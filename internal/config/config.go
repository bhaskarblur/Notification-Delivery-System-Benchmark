package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	NotificationService NotificationServiceConfig
	TaskPicker          TaskPickerConfig
	Kafka               KafkaConfig
	PostgreSQL          PostgreSQLConfig
	PriorityDelays      PriorityDelaysConfig
}

type NotificationServiceConfig struct {
	Port                    int
	PprofPort               int
	MaxSSEConnections       int
	SSEHeartbeatInterval    time.Duration
	GracefulShutdownTimeout time.Duration
}

type TaskPickerConfig struct {
	InstanceID         string
	NumPickerWorkers   int
	NumDeliveryWorkers int
	BatchSize          int
	PollInterval       time.Duration
	LeaseDuration      time.Duration
	ChannelBufferSize  int
}

type KafkaConfig struct {
	Brokers       []string
	ConsumerGroup string
	Topic         string
}

type PostgreSQLConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type PriorityDelaysConfig struct {
	High   DelayConfig
	Medium DelayConfig
	Low    DelayConfig
}

type DelayConfig struct {
	MinDelay      time.Duration
	MaxDelay      time.Duration
	JitterPercent int
}

func Load(configPath string) (*Config, error) {
	v := viper.New()

	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
	}

	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	v.AutomaticEnv()
	// Database type selection
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		v.Set("databasetype", dbType)
	}

	// PostgreSQL environment variables
	if pgHost := os.Getenv("POSTGRES_HOST"); pgHost != "" {
		v.Set("postgresql.host", pgHost)
	}
	if pgPort := os.Getenv("POSTGRES_PORT"); pgPort != "" {
		v.Set("postgresql.port", pgPort)
	}
	if pgDB := os.Getenv("POSTGRES_DATABASE"); pgDB != "" {
		v.Set("postgresql.database", pgDB)
	}
	if pgUser := os.Getenv("POSTGRES_USER"); pgUser != "" {
		v.Set("postgresql.user", pgUser)
	}
	if pgPass := os.Getenv("POSTGRES_PASSWORD"); pgPass != "" {
		v.Set("postgresql.password", pgPass)
	}

	// Kafka environment variables
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		v.Set("kafka.brokers", []string{brokers})
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// PostgreSQL defaults
	if config.PostgreSQL.Host == "" {
		config.PostgreSQL.Host = "localhost"
	}
	if config.PostgreSQL.Port == 0 {
		config.PostgreSQL.Port = 5432
	}
	if config.PostgreSQL.Database == "" {
		config.PostgreSQL.Database = "notifications"
	}
	if config.PostgreSQL.User == "" {
		config.PostgreSQL.User = "admin"
	}
	if config.PostgreSQL.Password == "" {
		config.PostgreSQL.Password = "admin123"
	}

	// Service defaults
	if config.NotificationService.Port == 0 {
		config.NotificationService.Port = 8080
	}
	if config.NotificationService.PprofPort == 0 {
		config.NotificationService.PprofPort = 6060
	}
	if config.NotificationService.MaxSSEConnections == 0 {
		config.NotificationService.MaxSSEConnections = 10000
	}
	if config.NotificationService.SSEHeartbeatInterval == 0 {
		config.NotificationService.SSEHeartbeatInterval = 30 * time.Second
	}
	if config.NotificationService.GracefulShutdownTimeout == 0 {
		config.NotificationService.GracefulShutdownTimeout = 30 * time.Second
	}
	
	// Task Picker defaults - Optimized for high throughput
	if config.TaskPicker.InstanceID == "" {
		config.TaskPicker.InstanceID = fmt.Sprintf("notif-service-%d", time.Now().Unix())
	}
	if config.TaskPicker.NumPickerWorkers == 0 {
		config.TaskPicker.NumPickerWorkers = 10 // Increased from 5
	}
	if config.TaskPicker.NumDeliveryWorkers == 0 {
		config.TaskPicker.NumDeliveryWorkers = 50 // Increased from 20
	}
	if config.TaskPicker.BatchSize == 0 {
		config.TaskPicker.BatchSize = 500 // Keep at 500 (already good)
	}
	if config.TaskPicker.PollInterval == 0 {
		config.TaskPicker.PollInterval = 100 * time.Millisecond // Reduced from 1s for faster pickup
	}
	if config.TaskPicker.LeaseDuration == 0 {
		config.TaskPicker.LeaseDuration = 30 * time.Second
	}
	if config.TaskPicker.ChannelBufferSize == 0 {
		config.TaskPicker.ChannelBufferSize = 5000 // Increased from 2000 for higher throughput
	}

	return &config, nil
}
