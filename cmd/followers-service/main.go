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

var followerNames = []string{
	"Alice Williams", "Bob Martin", "Carol White", "Daniel Harris",
	"Eva Thompson", "Frank Garcia", "Grace Martinez", "Henry Robinson",
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("starting followers service")

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
	eventRate := 75
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

	logger.Info("followers service started",
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
			logger.Info("shutting down followers service")
			return
		case <-ticker.C:
			eventType := randomFollowerEventType()
			userID := fmt.Sprintf("user_%d", rand.Intn(numUsers)+1)
			priority := models.GetPriorityForEventType(eventType)

			msg := &models.KafkaMessage{
				EventID:        uuid.New().String(),
				EventType:      string(eventType),
				Priority:       string(priority),
				UserID:         userID,
				EventTimestamp: time.Now(),
				Payload:        generateFollowerPayload(eventType),
				Metadata: models.Metadata{
					SourceService: "followers-service",
					TraceID:       uuid.New().String(),
				},
			}

			if err := prod.PublishNotification(ctx, msg); err != nil {
				logger.Error("failed to publish event", zap.Error(err))
			}
		}
	}
}

func randomFollowerEventType() models.EventType {
	events := []models.EventType{
		models.EventFollowerNew,
		models.EventFollowerContentLiked,
		models.EventFollowerContentComment,
	}
	return events[rand.Intn(len(events))]
}

func generateFollowerPayload(eventType models.EventType) map[string]string {
	payload := make(map[string]string)

	switch eventType {
	case models.EventFollowerNew:
		payload["follower_id"] = fmt.Sprintf("user_%d", rand.Intn(10000)+1)
		payload["follower_name"] = followerNames[rand.Intn(len(followerNames))]
		payload["follower_headline"] = "Product Manager at StartupXYZ"
	case models.EventFollowerContentLiked:
		payload["liker_name"] = followerNames[rand.Intn(len(followerNames))]
		payload["content_title"] = "My thoughts on distributed systems"
	case models.EventFollowerContentComment:
		payload["commenter_name"] = followerNames[rand.Intn(len(followerNames))]
		payload["content_title"] = "Building scalable services with Go"
		payload["comment_preview"] = "Great insights!"
	}

	return payload
}
