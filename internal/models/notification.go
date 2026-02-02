package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Priority represents notification priority level
type Priority string

const (
	PriorityHigh   Priority = "HIGH"
	PriorityMedium Priority = "MEDIUM"
	PriorityLow    Priority = "LOW"
)

// EventType represents the type of notification event
type EventType string

const (
	// Job events
	EventJobNew               EventType = "job.new"
	EventJobUpdate            EventType = "job.update"
	EventJobApplicationViewed EventType = "job.application_viewed"
	EventJobApplicationStatus EventType = "job.application_status"

	// Connection events
	EventConnectionRequest  EventType = "connection.request"
	EventConnectionAccepted EventType = "connection.accepted"
	EventConnectionEndorsed EventType = "connection.endorsed"

	// Follower events
	EventFollowerNew            EventType = "follower.new"
	EventFollowerContentLiked   EventType = "follower.content_liked"
	EventFollowerContentComment EventType = "follower.content_commented"
)

// Notification represents a notification in the system
type Notification struct {
	NotificationID                 uuid.UUID         `json:"notification_id"`
	UserID                         string            `json:"user_id"`
	EventType                      EventType         `json:"event_type"`
	Priority                       Priority          `json:"priority"`
	Status                         string            `json:"status"` // not_pushed, processing, pushed, delivered, failed
	EventTimestamp                 time.Time         `json:"event_timestamp"`
	NotificationReceivedTimestamp  time.Time         `json:"notification_received_timestamp"`
	NotificationDeliveredTimestamp time.Time         `json:"notification_delivered_timestamp"`
	DelaySeconds                   int32             `json:"delay_seconds"`
	Payload                        map[string]string `json:"payload"`
	IsRead                         bool              `json:"is_read"`
	RetryCount                     int               `json:"retry_count"`
	CreatedAt                      time.Time         `json:"created_at"`
}

// KafkaMessage represents the message format in Kafka
type KafkaMessage struct {
	EventID        string            `json:"event_id"`
	EventType      string            `json:"event_type"`
	Priority       string            `json:"priority"`
	UserID         string            `json:"user_id"`
	EventTimestamp time.Time         `json:"event_timestamp"`
	Payload        map[string]string `json:"payload"`
	Metadata       Metadata          `json:"metadata"`
}

// Metadata contains additional event metadata
type Metadata struct {
	SourceService string `json:"source_service"`
	TraceID       string `json:"trace_id"`
}

// ToJSON converts notification to JSON
func (n *Notification) ToJSON() ([]byte, error) {
	return json.Marshal(n)
}

// FromJSON creates notification from JSON
func (n *Notification) FromJSON(data []byte) error {
	return json.Unmarshal(data, n)
}

// GetPriorityForEventType returns the priority for a given event type
// HIGH: Job updates - most critical for user's career
// MEDIUM: Connection updates - important social interactions
// LOW: Follower activity - nice to have, not urgent
func GetPriorityForEventType(eventType EventType) Priority {
	switch eventType {
	// HIGH priority - process first (job-related, urgent)
	case EventJobNew:
		return PriorityHigh
	case EventJobUpdate:
		return PriorityHigh
	case EventJobApplicationStatus:
		return PriorityHigh

	// MEDIUM priority - process second (connections, moderately important)
	case EventConnectionRequest:
		return PriorityMedium
	case EventConnectionAccepted:
		return PriorityMedium
	case EventJobApplicationViewed:
		return PriorityMedium

	// LOW priority - process last (social activity, not urgent)
	case EventFollowerNew:
		return PriorityLow
	case EventFollowerContentLiked:
		return PriorityLow
	case EventFollowerContentComment:
		return PriorityLow
	case EventConnectionEndorsed:
		return PriorityLow

	default:
		return PriorityMedium
	}
}

// SSEMessage represents a message sent over SSE
type SSEMessage struct {
	NotificationID uuid.UUID `json:"notification_id"`
	Type           string    `json:"type"`
	Priority       string    `json:"priority"`
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
}
