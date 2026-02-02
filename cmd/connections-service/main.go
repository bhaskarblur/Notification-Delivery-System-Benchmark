package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"notification-delivery-system/internal/models"
	"notification-delivery-system/internal/producer"
)

var names = []string{
	"John Doe", "Jane Smith", "Michael Brown", "Emily Davis",
	"David Wilson", "Sarah Johnson", "Robert Lee", "Lisa Anderson",
}

var skills = []string{
	"Go", "Python", "JavaScript", "React", "Docker", "Kubernetes",
	"System Design", "Machine Learning", "Cloud Architecture",
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("starting connections service")

	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "localhost:9092"
	}
	brokers := []string{brokersEnv}

	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "notification-events"
	}

	eventRateStr := os.Getenv("EVENT_RATE")
	eventRate := 50
	if eventRateStr != "" {
		if rate, err := strconv.Atoi(eventRateStr); err == nil {
			eventRate = rate
		}
	}

	numUsersStr := os.Getenv("NUM_USERS")
	numUsers := 10000
	if numUsersStr != "" {
		if users, err := strconv.Atoi(numUsersStr); err == nil {
			numUsers = users
		}
	}

	prod, err := producer.NewProducer(brokers, topic, logger)
	if err != nil {
		logger.Fatal("failed to create producer", zap.Error(err))
	}
	defer prod.Close()

	logger.Info("connections service started",
		zap.Int("event_rate", eventRate),
		zap.Int("num_users", numUsers))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(time.Second / time.Duration(eventRate))
	defer ticker.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-quit:
			logger.Info("shutting down connections service")
			return
		case <-ticker.C:
			eventType := randomConnectionEventType()
			userID := fmt.Sprintf("user_%d", rand.Intn(numUsers)+1)
			priority := models.GetPriorityForEventType(eventType)

			msg := &models.KafkaMessage{
				EventID:        uuid.New().String(),
				EventType:      string(eventType),
				Priority:       string(priority),
				UserID:         userID,
				EventTimestamp: time.Now(),
				Payload:        generateConnectionPayload(eventType),
				Metadata: models.Metadata{
					SourceService: "connections-service",
					TraceID:       uuid.New().String(),
				},
			}

			if err := prod.PublishNotification(ctx, msg); err != nil {
				logger.Error("failed to publish event", zap.Error(err))
			}
		}
	}
}

func randomConnectionEventType() models.EventType {
	events := []models.EventType{
		models.EventConnectionRequest,
		models.EventConnectionAccepted,
		models.EventConnectionEndorsed,
	}
	return events[rand.Intn(len(events))]
}

func generateConnectionPayload(eventType models.EventType) map[string]string {
	payload := make(map[string]string)

	switch eventType {
	case models.EventConnectionRequest:
		payload["from"] = names[rand.Intn(len(names))]
		payload["headline"] = "Software Engineer at TechCorp"
	case models.EventConnectionAccepted:
		payload["from"] = names[rand.Intn(len(names))]
	case models.EventConnectionEndorsed:
		payload["from"] = names[rand.Intn(len(names))]
		payload["skill"] = skills[rand.Intn(len(skills))]
	}

	return payload
}
