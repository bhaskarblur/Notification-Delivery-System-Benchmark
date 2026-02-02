package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
	"go.uber.org/zap"

	"notification-delivery-system/internal/models"
)

type Producer struct {
	writer *kafka.Writer
	topic  string
	logger *zap.Logger
}

func NewProducer(brokers []string, topic string, logger *zap.Logger) (*Producer, error) {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{}, // Hash by key to ensure same user goes to same partition
		Compression:            compress.Snappy,
		RequiredAcks:           kafka.RequireOne, // Changed from RequireAll for better performance
		MaxAttempts:            3,
		BatchSize:              100,
		BatchTimeout:           10 * time.Millisecond,
		Async:                  false,
		ReadTimeout:            10 * time.Second,
		WriteTimeout:           10 * time.Second,
		AllowAutoTopicCreation: true,
	}

	logger.Info("kafka producer created", 
		zap.Strings("brokers", brokers),
		zap.String("topic", topic))

	return &Producer{
		writer: writer,
		topic:  topic,
		logger: logger,
	}, nil
}

// PublishNotification publishes a notification event to Kafka
// Uses user_id as partition key to ensure all events for a user go to the same partition
func (p *Producer) PublishNotification(ctx context.Context, msg *models.KafkaMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// IMPORTANT: Use user_id as partition key
	// This ensures all notifications for the same user go to the same partition maintaining order for that user
	kafkaMsg := kafka.Message{
		Key:   []byte(msg.UserID),
		Value: data,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(msg.EventType)},
			{Key: "priority", Value: []byte(msg.Priority)},
			{Key: "source_service", Value: []byte(msg.Metadata.SourceService)},
			{Key: "trace_id", Value: []byte(msg.Metadata.TraceID)},
		},
		Time: time.Now(),
	}

	// Write with timeout
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = p.writer.WriteMessages(writeCtx, kafkaMsg)
	if err != nil {
		p.logger.Error("delivery failed", zap.String("user_id", msg.UserID), zap.Error(err))
		return fmt.Errorf("failed to write message: %w", err)
	}

	p.logger.Debug("message delivered", 
		zap.String("user_id", msg.UserID), 
		zap.String("event_type", msg.EventType))

	return nil
}

func (p *Producer) Close() {
	p.logger.Info("closing producer")
	if err := p.writer.Close(); err != nil {
		p.logger.Error("failed to close producer", zap.Error(err))
	}
}
