package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"notification-delivery-system/internal/config"
	"notification-delivery-system/internal/notification"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("starting notification service - PostgreSQL optimized")

	cfg, err := config.Load("")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Initialize PostgreSQL repository
	repo, err := notification.NewPostgresRepository(
		cfg.PostgreSQL.Host,
		cfg.PostgreSQL.Port,
		cfg.PostgreSQL.Database,
		cfg.PostgreSQL.User,
		cfg.PostgreSQL.Password,
		logger,
	)
	if err != nil {
		logger.Fatal("failed to initialize postgres repository", zap.Error(err))
	}
	defer repo.Close(context.Background())

	// Initialize SSE Manager
	sseManager := notification.NewSSEManager(cfg.NotificationService.MaxSSEConnections, logger)

	// Get Kafka config from environment or use defaults
	kafkaBrokers := []string{os.Getenv("KAFKA_BROKERS")}
	if kafkaBrokers[0] == "" {
		kafkaBrokers = []string{"localhost:9092"}
	}
	kafkaTopic := os.Getenv("KAFKA_TOPIC")
	if kafkaTopic == "" {
		kafkaTopic = "notifications"
	}
	kafkaGroup := "notification-consumer"

	// Initialize Kafka Consumer (Phase 1: Kafka → ClickHouse persistence)
	consumer, err := notification.NewConsumer(
		kafkaBrokers,
		kafkaGroup,
		kafkaTopic,
		repo,
		logger,
	)
	if err != nil {
		logger.Fatal("failed to initialize consumer", zap.Error(err))
	}
	defer consumer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Kafka Consumer (writes to DB with status='not_pushed')
	go func() {
		logger.Info("starting kafka consumer - persistence layer")
		if err := consumer.Consume(ctx); err != nil {
			logger.Error("consumer error", zap.Error(err))
		}
	}()

	// Initialize Task Picker (Phase 2: DB → SSE delivery with dual worker pools)
	taskPickerCfg := notification.TaskPickerConfig{
		InstanceID:         cfg.TaskPicker.InstanceID,
		NumPickerWorkers:   cfg.TaskPicker.NumPickerWorkers,
		NumDeliveryWorkers: cfg.TaskPicker.NumDeliveryWorkers,
		BatchSize:          cfg.TaskPicker.BatchSize,
		PollInterval:       cfg.TaskPicker.PollInterval,
		LeaseDuration:      cfg.TaskPicker.LeaseDuration,
		ChannelBufferSize:  cfg.TaskPicker.ChannelBufferSize,
	}

	taskPicker := notification.NewTaskPicker(taskPickerCfg, repo, sseManager, logger)

	// Start Task Picker (claims from DB, delivers via SSE, batch status updates)
	logger.Info("starting task picker - delivery layer with dual worker pools")
	taskPicker.Start()
	defer taskPicker.Stop()

	// Start pprof server
	go func() {
		pprofAddr := fmt.Sprintf(":%d", cfg.NotificationService.PprofPort)
		logger.Info("starting pprof server", zap.String("addr", pprofAddr))
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("pprof server error", zap.Error(err))
		}
	}()

	// Setup HTTP router
	router := setupRouter(sseManager, repo, logger)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.NotificationService.Port),
		Handler: router,
	}

	go func() {
		logger.Info("starting HTTP server", zap.Int("port", cfg.NotificationService.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.NotificationService.GracefulShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server exited")
}

func setupRouter(sseManager *notification.SSEManager, repo *notification.PostgresRepository, logger *zap.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":             "ok",
			"active_connections": sseManager.GetActiveConnections(),
			"timestamp":          time.Now().Format(time.RFC3339),
		})
	})

	router.GET("/notifications/stream", func(c *gin.Context) {
		userID := c.Query("user_id")
		if userID == "" {
			c.JSON(400, gin.H{"error": "user_id is required"})
			return
		}

		logger.Info("SSE connection request", zap.String("user_id", userID))

		// Use the built-in StreamToClient method that handles everything
		sseManager.StreamToClient(c, userID)
	})

	router.GET("/notifications/:user_id", func(c *gin.Context) {
		userID := c.Param("user_id")

		notifications, err := repo.GetUserNotifications(c.Request.Context(), userID, 100)
		if err != nil {
			logger.Error("failed to query notifications", zap.Error(err))
			c.JSON(500, gin.H{"error": "failed to fetch notifications"})
			return
		}

		c.JSON(200, gin.H{
			"user_id":       userID,
			"notifications": notifications,
			"count":         len(notifications),
		})
	})

	return router
}
