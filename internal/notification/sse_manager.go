package notification

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"notification-delivery-system/internal/models"
)

// SSEConnection represents a client SSE connection
type SSEConnection struct {
	UserID     string
	ClientChan chan []byte
	LastPing   time.Time
}

// SSEManager manages SSE connections for all users
type SSEManager struct {
	connections map[string][]*SSEConnection
	mu          sync.RWMutex
	logger      *zap.Logger
	maxConns    int
}

// NewSSEManager creates a new SSE manager
func NewSSEManager(maxConns int, logger *zap.Logger) *SSEManager {
	manager := &SSEManager{
		connections: make(map[string][]*SSEConnection),
		logger:      logger,
		maxConns:    maxConns,
	}

	// Start cleanup goroutine
	go manager.cleanupStaleConnections()

	return manager
}

// AddConnection adds a new SSE connection for a user
func (m *SSEManager) AddConnection(userID string) (*SSEConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check max connections
	totalConns := 0
	for _, conns := range m.connections {
		totalConns += len(conns)
	}

	if totalConns >= m.maxConns {
		return nil, fmt.Errorf("max connections reached: %d", m.maxConns)
	}

	conn := &SSEConnection{
		UserID:     userID,
		ClientChan: make(chan []byte, 100), // Buffer for 100 messages
		LastPing:   time.Now(),
	}

	m.connections[userID] = append(m.connections[userID], conn)

	m.logger.Info("SSE connection added",
		zap.String("user_id", userID),
		zap.Int("user_connections", len(m.connections[userID])),
		zap.Int("total_connections", totalConns+1))

	return conn, nil
}

// RemoveConnection removes an SSE connection
func (m *SSEManager) RemoveConnection(userID string, conn *SSEConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()

	connections := m.connections[userID]
	for i, c := range connections {
		if c == conn {
			close(c.ClientChan)
			m.connections[userID] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	// Remove user entry if no more connections
	if len(m.connections[userID]) == 0 {
		delete(m.connections, userID)
	}

	m.logger.Info("SSE connection removed",
		zap.String("user_id", userID),
		zap.Int("remaining_connections", len(m.connections[userID])))
}

// BroadcastToUser sends a notification to all connections of a user
func (m *SSEManager) BroadcastToUser(userID string, notification *models.Notification) {
	m.mu.RLock()
	connections := m.connections[userID]
	m.mu.RUnlock()

	if len(connections) == 0 {
		m.logger.Debug("no active connections for user", zap.String("user_id", userID))
		return
	}

	// Create SSE message
	sseMsg := models.SSEMessage{
		NotificationID: notification.NotificationID,
		Type:           string(notification.EventType),
		Priority:       string(notification.Priority),
		Title:          m.generateTitle(notification),
		Message:        m.generateMessage(notification),
		Timestamp:      notification.NotificationDeliveredTimestamp,
	}

	data, err := json.Marshal(sseMsg)
	if err != nil {
		m.logger.Error("failed to marshal SSE message", zap.Error(err))
		return
	}

	// Format SSE message
	sseData := fmt.Sprintf("event: notification\ndata: %s\n\n", data)

	// Send to all user connections
	for _, conn := range connections {
		select {
		case conn.ClientChan <- []byte(sseData):
			m.logger.Debug("notification sent to connection",
				zap.String("user_id", userID),
				zap.String("event_type", string(notification.EventType)))
		default:
			m.logger.Warn("connection buffer full, skipping",
				zap.String("user_id", userID))
		}
	}
}

// Send sends a generic message to all connections of a user
func (m *SSEManager) Send(userID string, data map[string]interface{}) error {
	m.mu.RLock()
	connections := m.connections[userID]
	m.mu.RUnlock()

	if len(connections) == 0 {
		return fmt.Errorf("no active connections for user: %s", userID)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Format SSE message
	sseData := fmt.Sprintf("event: notification\ndata: %s\n\n", jsonData)

	// Send to all user connections
	for _, conn := range connections {
		select {
		case conn.ClientChan <- []byte(sseData):
			// Sent successfully
		default:
			m.logger.Warn("connection buffer full, skipping",
				zap.String("user_id", userID))
		}
	}

	return nil
}

// StreamToClient handles the SSE streaming to a gin context
func (m *SSEManager) StreamToClient(c *gin.Context, userID string) {
	conn, err := m.AddConnection(userID)
	if err != nil {
		c.JSON(503, gin.H{"error": err.Error()})
		return
	}
	defer m.RemoveConnection(userID, conn)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Send initial connection message
	c.Writer.Write([]byte("event: connected\ndata: {\"status\":\"connected\"}\n\n"))
	c.Writer.Flush()

	// Start heartbeat
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			m.logger.Info("client disconnected", zap.String("user_id", userID))
			return
		case msg := <-conn.ClientChan:
			_, err := c.Writer.Write(msg)
			if err != nil {
				m.logger.Error("failed to write to client", zap.Error(err))
				return
			}
			c.Writer.Flush()
			conn.LastPing = time.Now()
		case <-ticker.C:
			// Send heartbeat
			heartbeat := fmt.Sprintf("event: heartbeat\ndata: {\"timestamp\":\"%s\"}\n\n",
				time.Now().Format(time.RFC3339))
			_, err := c.Writer.Write([]byte(heartbeat))
			if err != nil {
				m.logger.Error("failed to send heartbeat", zap.Error(err))
				return
			}
			c.Writer.Flush()
			conn.LastPing = time.Now()
		}
	}
}

// cleanupStaleConnections removes stale connections
func (m *SSEManager) cleanupStaleConnections() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		staleTimeout := 5 * time.Minute
		now := time.Now()

		for userID, connections := range m.connections {
			var activeConns []*SSEConnection
			for _, conn := range connections {
				if now.Sub(conn.LastPing) < staleTimeout {
					activeConns = append(activeConns, conn)
				} else {
					close(conn.ClientChan)
					m.logger.Info("removed stale connection",
						zap.String("user_id", userID),
						zap.Duration("idle_time", now.Sub(conn.LastPing)))
				}
			}

			if len(activeConns) > 0 {
				m.connections[userID] = activeConns
			} else {
				delete(m.connections, userID)
			}
		}
		m.mu.Unlock()
	}
}

// GetActiveConnections returns the count of active connections
func (m *SSEManager) GetActiveConnections() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for _, conns := range m.connections {
		total += len(conns)
	}
	return total
}

// generateTitle generates a title for the notification
func (m *SSEManager) generateTitle(notif *models.Notification) string {
	switch notif.EventType {
	case models.EventJobApplicationViewed:
		return "Application Viewed"
	case models.EventJobNew:
		return "New Job Recommendation"
	case models.EventConnectionRequest:
		return "New Connection Request"
	case models.EventFollowerNew:
		return "New Follower"
	default:
		return "New Notification"
	}
}

// generateMessage generates a message for the notification
func (m *SSEManager) generateMessage(notif *models.Notification) string {
	switch notif.EventType {
	case models.EventJobApplicationViewed:
		company := notif.Payload["company_name"]
		return fmt.Sprintf("%s viewed your application", company)
	case models.EventJobNew:
		title := notif.Payload["job_title"]
		return fmt.Sprintf("New job: %s", title)
	case models.EventConnectionRequest:
		name := notif.Payload["from"]
		return fmt.Sprintf("%s sent you a connection request", name)
	case models.EventFollowerNew:
		name := notif.Payload["follower_name"]
		return fmt.Sprintf("%s started following you", name)
	default:
		return "You have a new notification"
	}
}
