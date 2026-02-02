package notification

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NotificationBatch represents a batch of notifications claimed from DB
type NotificationBatch struct {
	NotificationID                uuid.UUID
	UserID                        string
	EventType                     string
	Priority                      string
	EventTimestamp                time.Time
	NotificationReceivedTimestamp time.Time
	Payload                       string
}

// StatusUpdate represents a status update to be batched
type StatusUpdate struct {
	NotificationID uuid.UUID
	Status         string
	ErrorMsg       string
}

// TaskPicker manages dual worker pools for maximum throughput
// Pool 1: Picker workers claim from DB
// Pool 2: Delivery workers send via SSE
type TaskPicker struct {
	instanceID string
	repository *PostgresRepository
	sseManager *SSEManager
	logger     *zap.Logger

	// Configuration
	numPickerWorkers   int
	numDeliveryWorkers int
	batchSize          int
	pollInterval       time.Duration
	leaseDuration      time.Duration

	// Channels for worker communication
	notificationChan chan *NotificationBatch
	statusUpdateChan chan *StatusUpdate

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// TaskPickerConfig holds configuration for the task picker
type TaskPickerConfig struct {
	InstanceID         string
	NumPickerWorkers   int           // Number of workers claiming from DB
	NumDeliveryWorkers int           // Number of workers delivering via SSE
	BatchSize          int           // Notifications per claim (500)
	PollInterval       time.Duration // How often pickers poll DB
	LeaseDuration      time.Duration // Lease timeout (30s)
	ChannelBufferSize  int           // Buffer between picker and delivery workers
}

// NewTaskPicker creates a new task picker with dual worker pools
func NewTaskPicker(cfg TaskPickerConfig, repo *PostgresRepository, sseManager *SSEManager, logger *zap.Logger) *TaskPicker {
	ctx, cancel := context.WithCancel(context.Background())

	return &TaskPicker{
		instanceID:         cfg.InstanceID,
		repository:         repo,
		sseManager:         sseManager,
		logger:             logger,
		numPickerWorkers:   cfg.NumPickerWorkers,
		numDeliveryWorkers: cfg.NumDeliveryWorkers,
		batchSize:          cfg.BatchSize,
		pollInterval:       cfg.PollInterval,
		leaseDuration:      cfg.LeaseDuration,
		notificationChan:   make(chan *NotificationBatch, cfg.ChannelBufferSize),
		statusUpdateChan:   make(chan *StatusUpdate, cfg.ChannelBufferSize),
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start starts all worker pools and background tasks
func (tp *TaskPicker) Start() {
	tp.logger.Info("starting task picker",
		zap.String("instance_id", tp.instanceID),
		zap.Int("picker_workers", tp.numPickerWorkers),
		zap.Int("delivery_workers", tp.numDeliveryWorkers),
		zap.Int("batch_size", tp.batchSize))

	// Start picker workers (claim from DB)
	for i := 0; i < tp.numPickerWorkers; i++ {
		tp.wg.Add(1)
		go tp.pickerWorker(i)
	}

	// Start delivery workers (send via SSE)
	for i := 0; i < tp.numDeliveryWorkers; i++ {
		tp.wg.Add(1)
		go tp.deliveryWorker(i)
	}

	// Start batch status updater (flushes every 1 second)
	tp.wg.Add(1)
	go tp.batchStatusUpdater()

	// Start lease cleanup background job
	tp.wg.Add(1)
	go tp.leaseCleanupWorker()

	// Start metrics reporter
	tp.wg.Add(1)
	go tp.metricsReporter()
}

// Stop gracefully stops all workers
func (tp *TaskPicker) Stop() {
	tp.logger.Info("stopping task picker")
	tp.cancel()

	// Close channels to signal workers
	close(tp.notificationChan)
	close(tp.statusUpdateChan)

	tp.wg.Wait()
	tp.logger.Info("task picker stopped")
}

// pickerWorker claims notifications from DB and sends to channel
func (tp *TaskPicker) pickerWorker(workerID int) {
	defer tp.wg.Done()

	ticker := time.NewTicker(tp.pollInterval)
	defer ticker.Stop()

	tp.logger.Info("picker worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case <-ticker.C:
			// Claim batch from DB
			notifications, err := tp.repository.ClaimBatch(
				tp.ctx,
				tp.instanceID,
				tp.batchSize,
				tp.leaseDuration,
			)

			if err != nil {
				tp.logger.Error("failed to claim notifications",
					zap.Int("worker_id", workerID),
					zap.Error(err))
				continue
			}

			if len(notifications) == 0 {
				// No work available
				continue
			}

			tp.logger.Debug("claimed notifications",
				zap.Int("worker_id", workerID),
				zap.Int("count", len(notifications)))

			// Send to delivery workers via channel
			for _, notif := range notifications {
				select {
				case tp.notificationChan <- notif:
					// Sent successfully
				case <-tp.ctx.Done():
					return
				}
			}

		case <-tp.ctx.Done():
			tp.logger.Info("picker worker stopped", zap.Int("worker_id", workerID))
			return
		}
	}
}

// deliveryWorker receives notifications from channel and delivers via SSE
func (tp *TaskPicker) deliveryWorker(workerID int) {
	defer tp.wg.Done()

	tp.logger.Info("delivery worker started", zap.Int("worker_id", workerID))

	for {
		select {
		case notif, ok := <-tp.notificationChan:
			if !ok {
				// Channel closed, shutdown
				tp.logger.Info("delivery worker stopped", zap.Int("worker_id", workerID))
				return
			}

			// Deliver notification
			tp.deliverNotification(workerID, notif)

		case <-tp.ctx.Done():
			tp.logger.Info("delivery worker stopped", zap.Int("worker_id", workerID))
			return
		}
	}
}

// deliverNotification attempts to deliver a single notification
func (tp *TaskPicker) deliverNotification(workerID int, notif *NotificationBatch) {
	startTime := time.Now()

	// Attempt SSE delivery
	err := tp.sseManager.Send(notif.UserID, map[string]interface{}{
		"notification_id": notif.NotificationID.String(),
		"event_type":      notif.EventType,
		"priority":        notif.Priority,
		"event_timestamp": notif.EventTimestamp,
		"payload":         notif.Payload,
	})

	deliveryLatency := time.Since(startTime)

	// Queue status update (batched)
	statusUpdate := &StatusUpdate{
		NotificationID: notif.NotificationID,
		Status:         "pushed",
		ErrorMsg:       "",
	}

	if err != nil {
		// Delivery failed - queue failed status
		statusUpdate.Status = "failed"
		statusUpdate.ErrorMsg = err.Error()

		tp.logger.Warn("delivery failed",
			zap.Int("worker_id", workerID),
			zap.String("notification_id", notif.NotificationID.String()),
			zap.String("user_id", notif.UserID),
			zap.String("priority", notif.Priority),
			zap.Duration("latency", deliveryLatency),
			zap.Error(err))
	} else {
		tp.logger.Debug("delivered notification",
			zap.Int("worker_id", workerID),
			zap.String("notification_id", notif.NotificationID.String()),
			zap.String("user_id", notif.UserID),
			zap.String("priority", notif.Priority),
			zap.Duration("delivery_latency", deliveryLatency))
	}

	// Send to batch status updater
	select {
	case tp.statusUpdateChan <- statusUpdate:
		// Queued successfully
	case <-tp.ctx.Done():
		return
	}
}

// leaseCleanupWorker periodically resets expired leases
func (tp *TaskPicker) leaseCleanupWorker() {
	defer tp.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	tp.logger.Info("lease cleanup worker started")

	for {
		select {
		case <-ticker.C:
			affected, err := tp.repository.ReclaimStaleTasks(tp.ctx)
			if err != nil {
				tp.logger.Error("failed to reset expired leases", zap.Error(err))
				continue
			}

			if affected > 0 {
				tp.logger.Warn("reset expired leases",
					zap.Int("count", affected))
			}

		case <-tp.ctx.Done():
			tp.logger.Info("lease cleanup worker stopped")
			return
		}
	}
}

// metricsReporter periodically reports metrics
func (tp *TaskPicker) metricsReporter() {
	defer tp.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	tp.logger.Info("metrics reporter started")

	for {
		select {
		case <-ticker.C:
			metrics, err := tp.repository.GetStats(tp.ctx)
			if err != nil {
				tp.logger.Error("failed to get metrics", zap.Error(err))
				continue
			}

			tp.logger.Info("task picker metrics",
				zap.String("instance_id", tp.instanceID),
				zap.Int("notification_channel_size", len(tp.notificationChan)),
				zap.Int("notification_channel_cap", cap(tp.notificationChan)),
				zap.Int("status_update_channel_size", len(tp.statusUpdateChan)),
				zap.Int("status_update_channel_cap", cap(tp.statusUpdateChan)),
				zap.Any("pending_work", metrics))

		case <-tp.ctx.Done():
			tp.logger.Info("metrics reporter stopped")
			return
		}
	}
}

// batchStatusUpdater collects status updates and flushes every 1 second
func (tp *TaskPicker) batchStatusUpdater() {
	defer tp.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var statusBatch []*StatusUpdate

	tp.logger.Info("batch status updater started")

	for {
		select {
		case update, ok := <-tp.statusUpdateChan:
			if !ok {
				// Channel closed, flush remaining and exit
				if len(statusBatch) > 0 {
					tp.flushStatusBatch(statusBatch)
				}
				tp.logger.Info("batch status updater stopped")
				return
			}

			// Accumulate updates
			statusBatch = append(statusBatch, update)

		case <-ticker.C:
			// Flush batch every 1 second
			if len(statusBatch) > 0 {
				tp.flushStatusBatch(statusBatch)
				statusBatch = statusBatch[:0] // Reset slice
			}

		case <-tp.ctx.Done():
			// Flush remaining updates before exiting
			if len(statusBatch) > 0 {
				tp.flushStatusBatch(statusBatch)
			}
			tp.logger.Info("batch status updater stopped")
			return
		}
	}
}

// flushStatusBatch executes a batch status update to DB
func (tp *TaskPicker) flushStatusBatch(batch []*StatusUpdate) {
	if len(batch) == 0 {
		return
	}

	startTime := time.Now()

	err := tp.repository.BatchUpdateStatus(tp.ctx, batch)
	if err != nil {
		tp.logger.Error("failed to batch update status",
			zap.Int("batch_size", len(batch)),
			zap.Error(err))
		return
	}

	flushDuration := time.Since(startTime)

	tp.logger.Info("batch status update completed",
		zap.Int("batch_size", len(batch)),
		zap.Duration("duration", flushDuration))
}

// metricsReporter periodically reports metrics
