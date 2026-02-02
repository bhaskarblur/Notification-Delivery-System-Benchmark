package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
)

type NotificationEvent struct {
	NotificationID string    `json:"notification_id"`
	UserID         string    `json:"user_id"`
	Priority       string    `json:"priority"`
	Message        string    `json:"message"`
	EventTimestamp time.Time `json:"event_timestamp"`
	ReceivedAt     time.Time `json:"received_at"`
}

type LatencyStats struct {
	Min   time.Duration
	Max   time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Avg   time.Duration
	Count int64
	Total time.Duration
}

type BenchmarkMetrics struct {
	mu                    sync.RWMutex
	activeConnections     int64
	totalConnections      int64
	failedConnections     int64
	reconnections         int64
	notificationsReceived int64
	latencies             []time.Duration
	connectionDurations   []time.Duration
	startTime             time.Time
	lastReportTime        time.Time
	notificationsByUser   map[string]int64
	errorsByType          map[string]int64
	connectionStartTimes  map[string]time.Time
}

func NewBenchmarkMetrics() *BenchmarkMetrics {
	return &BenchmarkMetrics{
		notificationsByUser:  make(map[string]int64),
		errorsByType:         make(map[string]int64),
		connectionStartTimes: make(map[string]time.Time),
		startTime:            time.Now(),
		lastReportTime:       time.Now(),
	}
}

func (m *BenchmarkMetrics) RecordConnection(userID string) {
	atomic.AddInt64(&m.activeConnections, 1)
	atomic.AddInt64(&m.totalConnections, 1)
	m.mu.Lock()
	m.connectionStartTimes[userID] = time.Now()
	m.mu.Unlock()
}

func (m *BenchmarkMetrics) RecordDisconnection(userID string) {
	atomic.AddInt64(&m.activeConnections, -1)
	m.mu.Lock()
	if startTime, exists := m.connectionStartTimes[userID]; exists {
		duration := time.Since(startTime)
		m.connectionDurations = append(m.connectionDurations, duration)
		delete(m.connectionStartTimes, userID)
	}
	m.mu.Unlock()
}

func (m *BenchmarkMetrics) RecordReconnection() {
	atomic.AddInt64(&m.reconnections, 1)
}

func (m *BenchmarkMetrics) RecordFailedConnection() {
	atomic.AddInt64(&m.failedConnections, 1)
}

func (m *BenchmarkMetrics) RecordNotification(userID string, latency time.Duration) {
	atomic.AddInt64(&m.notificationsReceived, 1)
	m.mu.Lock()
	m.latencies = append(m.latencies, latency)
	m.notificationsByUser[userID]++
	m.mu.Unlock()
}

func (m *BenchmarkMetrics) RecordError(errorType string) {
	m.mu.Lock()
	m.errorsByType[errorType]++
	m.mu.Unlock()
}

func (m *BenchmarkMetrics) GetLatencyStats() LatencyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.latencies) == 0 {
		return LatencyStats{}
	}

	// Sort latencies for percentile calculation
	sortedLatencies := make([]time.Duration, len(m.latencies))
	copy(sortedLatencies, m.latencies)
	sort.Slice(sortedLatencies, func(i, j int) bool {
		return sortedLatencies[i] < sortedLatencies[j]
	})

	var total time.Duration
	for _, l := range sortedLatencies {
		total += l
	}

	stats := LatencyStats{
		Min:   sortedLatencies[0],
		Max:   sortedLatencies[len(sortedLatencies)-1],
		P50:   sortedLatencies[len(sortedLatencies)*50/100],
		P95:   sortedLatencies[len(sortedLatencies)*95/100],
		P99:   sortedLatencies[len(sortedLatencies)*99/100],
		Avg:   total / time.Duration(len(sortedLatencies)),
		Count: int64(len(sortedLatencies)),
		Total: total,
	}

	return stats
}

func (m *BenchmarkMetrics) PrintReport(logger *zap.Logger, detailed bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	elapsed := time.Since(m.startTime)
	sinceLast := time.Since(m.lastReportTime)
	m.lastReportTime = time.Now()

	latencyStats := m.GetLatencyStats()
	throughput := float64(m.notificationsReceived) / elapsed.Seconds()
	recentThroughput := float64(m.notificationsReceived) / sinceLast.Seconds()

	logger.Info("=== SSE Benchmark Report ===",
		zap.Duration("elapsed", elapsed),
		zap.Int64("active_connections", atomic.LoadInt64(&m.activeConnections)),
		zap.Int64("total_connections", atomic.LoadInt64(&m.totalConnections)),
		zap.Int64("failed_connections", atomic.LoadInt64(&m.failedConnections)),
		zap.Int64("reconnections", atomic.LoadInt64(&m.reconnections)),
		zap.Int64("notifications_received", atomic.LoadInt64(&m.notificationsReceived)),
		zap.Float64("throughput_per_sec", throughput),
		zap.Float64("recent_throughput_per_sec", recentThroughput),
	)

	if latencyStats.Count > 0 {
		logger.Info("=== Latency Statistics (Event → Client) ===",
			zap.Duration("min", latencyStats.Min),
			zap.Duration("max", latencyStats.Max),
			zap.Duration("avg", latencyStats.Avg),
			zap.Duration("p50", latencyStats.P50),
			zap.Duration("p95", latencyStats.P95),
			zap.Duration("p99", latencyStats.P99),
		)
	}

	if detailed && len(m.errorsByType) > 0 {
		logger.Info("=== Errors by Type ===")
		for errType, count := range m.errorsByType {
			logger.Info("error", zap.String("type", errType), zap.Int64("count", count))
		}
	}

	if detailed && len(m.notificationsByUser) > 0 {
		// Get top 5 users by notification count
		type userCount struct {
			userID string
			count  int64
		}
		var userCounts []userCount
		for userID, count := range m.notificationsByUser {
			userCounts = append(userCounts, userCount{userID, count})
		}
		sort.Slice(userCounts, func(i, j int) bool {
			return userCounts[i].count > userCounts[j].count
		})

		logger.Info("=== Top 5 Users by Notification Count ===")
		for i := 0; i < 5 && i < len(userCounts); i++ {
			logger.Info("user",
				zap.String("user_id", userCounts[i].userID),
				zap.Int64("count", userCounts[i].count),
			)
		}
	}
}

type SSEClient struct {
	userID      string
	serverURL   string
	metrics     *BenchmarkMetrics
	logger      *zap.Logger
	stopChan    chan struct{}
	wg          *sync.WaitGroup
	maxRetries  int
	retryDelay  time.Duration
	reconnect   bool
	pingTimeout time.Duration
}

func NewSSEClient(userID, serverURL string, metrics *BenchmarkMetrics, logger *zap.Logger, reconnect bool) *SSEClient {
	return &SSEClient{
		userID:      userID,
		serverURL:   serverURL,
		metrics:     metrics,
		logger:      logger,
		stopChan:    make(chan struct{}),
		wg:          &sync.WaitGroup{},
		maxRetries:  10,
		retryDelay:  time.Second,
		reconnect:   reconnect,
		pingTimeout: 35 * time.Second, // Slightly longer than server's 30s ping interval
	}
}

func (c *SSEClient) Connect(ctx context.Context) {
	c.wg.Add(1)
	go c.connectLoop(ctx)
}

func (c *SSEClient) connectLoop(ctx context.Context) {
	defer c.wg.Done()

	retryCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
		}

		// Establish connection
		if err := c.stream(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}

			c.metrics.RecordError(fmt.Sprintf("stream_error: %s", err.Error()))
			c.logger.Warn("stream error",
				zap.String("user_id", c.userID),
				zap.Error(err),
				zap.Int("retry_count", retryCount),
			)

			if !c.reconnect {
				c.metrics.RecordFailedConnection()
				return
			}

			retryCount++
			if retryCount > c.maxRetries {
				c.logger.Error("max retries exceeded",
					zap.String("user_id", c.userID),
					zap.Int("retries", retryCount),
				)
				c.metrics.RecordFailedConnection()
				return
			}

			c.metrics.RecordReconnection()
			// Exponential backoff
			backoff := c.retryDelay * time.Duration(1<<uint(retryCount))
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			case <-c.stopChan:
				return
			}
		} else {
			// Clean disconnection
			return
		}
	}
}

func (c *SSEClient) stream(ctx context.Context) error {
	url := fmt.Sprintf("%s/notifications/stream?user_id=%s", c.serverURL, c.userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	client := &http.Client{
		Timeout: 0, // No timeout for streaming
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	c.metrics.RecordConnection(c.userID)
	c.logger.Debug("connected", zap.String("user_id", c.userID))

	defer func() {
		c.metrics.RecordDisconnection(c.userID)
		c.logger.Debug("disconnected", zap.String("user_id", c.userID))
	}()

	reader := bufio.NewReader(resp.Body)
	lastActivity := time.Now()

	// Ping timeout checker
	pingTimeoutChan := time.After(c.pingTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.stopChan:
			return nil
		case <-pingTimeoutChan:
			if time.Since(lastActivity) > c.pingTimeout {
				return fmt.Errorf("ping timeout: no activity for %v", c.pingTimeout)
			}
			pingTimeoutChan = time.After(c.pingTimeout)
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		lastActivity = time.Now()
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Handle SSE event
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)

			// Skip ping messages
			if data == "ping" {
				continue
			}

			// Parse notification
			var event NotificationEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				c.logger.Warn("failed to parse notification",
					zap.String("user_id", c.userID),
					zap.String("data", data),
					zap.Error(err),
				)
				c.metrics.RecordError("parse_error")
				continue
			}

			// Calculate end-to-end latency (event creation to client receipt)
			event.ReceivedAt = time.Now()
			latency := event.ReceivedAt.Sub(event.EventTimestamp)

			c.metrics.RecordNotification(c.userID, latency)

			c.logger.Debug("notification received",
				zap.String("user_id", c.userID),
				zap.String("notification_id", event.NotificationID),
				zap.Duration("latency", latency),
			)
		}
	}
}

func (c *SSEClient) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}

func main() {
	var (
		serverURL       = flag.String("server", "http://localhost:8080", "Notification service URL")
		numUsers        = flag.Int("users", 1000, "Number of concurrent users")
		userPrefix      = flag.String("prefix", "user_", "User ID prefix")
		duration        = flag.Duration("duration", 5*time.Minute, "Benchmark duration (0 for infinite)")
		reportInterval  = flag.Duration("report", 10*time.Second, "Report interval")
		reconnect       = flag.Bool("reconnect", true, "Auto-reconnect on disconnect")
		detailedReports = flag.Bool("detailed", false, "Show detailed reports")
		rampUp          = flag.Duration("ramp-up", 10*time.Second, "Ramp-up duration for connections")
		logLevel        = flag.String("log", "info", "Log level (debug, info, warn, error)")
	)

	flag.Parse()

	// Setup logger
	var logger *zap.Logger
	var err error
	switch *logLevel {
	case "debug":
		logger, err = zap.NewDevelopment()
	default:
		logger, err = zap.NewProduction()
	}
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Info("starting SSE benchmark",
		zap.String("server", *serverURL),
		zap.Int("users", *numUsers),
		zap.Duration("duration", *duration),
		zap.Duration("ramp_up", *rampUp),
		zap.Bool("reconnect", *reconnect),
	)

	metrics := NewBenchmarkMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create clients
	clients := make([]*SSEClient, *numUsers)
	for i := 0; i < *numUsers; i++ {
		userID := fmt.Sprintf("%s%d", *userPrefix, i)
		clients[i] = NewSSEClient(userID, *serverURL, metrics, logger, *reconnect)
	}

	// Start clients with ramp-up
	rampUpDelay := *rampUp / time.Duration(*numUsers)
	logger.Info("ramping up connections",
		zap.Duration("delay_per_connection", rampUpDelay),
		zap.Duration("total_ramp_up", *rampUp),
	)

	for i, client := range clients {
		client.Connect(ctx)
		if i < *numUsers-1 {
			time.Sleep(rampUpDelay)
		}
	}

	logger.Info("all connections initiated", zap.Int("count", *numUsers))

	// Periodic reporting
	reportTicker := time.NewTicker(*reportInterval)
	defer reportTicker.Stop()

	var benchmarkTimeout <-chan time.Time
	if *duration > 0 {
		benchmarkTimeout = time.After(*duration)
	}

	// Report loop
	for {
		select {
		case <-reportTicker.C:
			metrics.PrintReport(logger, *detailedReports)

		case <-benchmarkTimeout:
			logger.Info("benchmark duration reached, shutting down...")
			cancel()
			goto cleanup

		case sig := <-sigChan:
			logger.Info("received signal, shutting down...", zap.String("signal", sig.String()))
			cancel()
			goto cleanup
		}
	}

cleanup:
	// Stop all clients
	logger.Info("stopping all clients...")
	for _, client := range clients {
		client.Stop()
	}

	// Final report
	logger.Info("=== FINAL REPORT ===")
	metrics.PrintReport(logger, true)

	logger.Info("benchmark completed")
}
