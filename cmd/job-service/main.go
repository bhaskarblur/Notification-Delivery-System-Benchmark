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

var jobTitles = []string{
	"Senior Backend Engineer",
	"Data Scientist",
	"Product Manager",
	"Frontend Developer",
	"DevOps Engineer",
	"ML Engineer",
	"QA Engineer",
	"Technical Lead",
}

var companies = []string{
	"TechCorp",
	"DataCo",
	"StartupXYZ",
	"BigTech Inc",
	"InnovateAI",
	"CloudSystems",
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("starting job service")

	// Get config from environment
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
	eventRate := 100
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

	// Initialize producer
	prod, err := producer.NewProducer(brokers, topic, logger)
	if err != nil {
		logger.Fatal("failed to create producer", zap.Error(err))
	}
	defer prod.Close()

	logger.Info("job service started",
		zap.Int("event_rate", eventRate),
		zap.Int("num_users", numUsers))

	// Start generating events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(time.Second / time.Duration(eventRate))
	defer ticker.Stop()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-quit:
			logger.Info("shutting down job service")
			return
		case <-ticker.C:
			// Generate random job event
			eventType := randomJobEventType()
			userID := fmt.Sprintf("user_%d", rand.Intn(numUsers)+1)
			priority := models.GetPriorityForEventType(eventType)

			msg := &models.KafkaMessage{
				EventID:        uuid.New().String(),
				EventType:      string(eventType),
				Priority:       string(priority),
				UserID:         userID,
				EventTimestamp: time.Now(),
				Payload:        generateJobPayload(eventType),
				Metadata: models.Metadata{
					SourceService: "job-service",
					TraceID:       uuid.New().String(),
				},
			}

			if err := prod.PublishNotification(ctx, msg); err != nil {
				logger.Error("failed to publish event", zap.Error(err))
			}
		}
	}
}

func randomJobEventType() models.EventType {
	events := []models.EventType{
		models.EventJobNew,
		models.EventJobUpdate,
		models.EventJobApplicationViewed,
		models.EventJobApplicationStatus,
	}
	return events[rand.Intn(len(events))]
}

func generateJobPayload(eventType models.EventType) map[string]string {
	payload := make(map[string]string)

	switch eventType {
	case models.EventJobNew:
		payload["job_title"] = jobTitles[rand.Intn(len(jobTitles))]
		payload["company_name"] = companies[rand.Intn(len(companies))]
		payload["location"] = "Remote"
	case models.EventJobUpdate:
		payload["job_title"] = jobTitles[rand.Intn(len(jobTitles))]
		payload["company_name"] = companies[rand.Intn(len(companies))]
		payload["update_type"] = "salary_updated"
	case models.EventJobApplicationViewed:
		payload["company_name"] = companies[rand.Intn(len(companies))]
		payload["job_title"] = jobTitles[rand.Intn(len(jobTitles))]
		payload["recruiter_name"] = "John Doe"
	case models.EventJobApplicationStatus:
		payload["company_name"] = companies[rand.Intn(len(companies))]
		payload["status"] = "interview_scheduled"
	}

	return payload
}
