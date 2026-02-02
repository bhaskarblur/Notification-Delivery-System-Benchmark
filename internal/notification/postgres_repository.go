package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver (imported for side effects)
	"go.uber.org/zap"

	"notification-delivery-system/internal/models"
)

// PostgresRepository handles PostgreSQL operations with optimized batch inserts
type PostgresRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewPostgresRepository creates a new PostgreSQL repository
func NewPostgresRepository(host string, port int, database, user, password string, logger *zap.Logger) (*PostgresRepository, error) {
	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		host, port, database, user, password)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Connection pool settings for high throughput
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	logger.Info("postgres repository initialized",
		zap.String("host", host),
		zap.Int("port", port),
		zap.String("database", database))

	return &PostgresRepository{
		db:     db,
		logger: logger,
	}, nil
}

// Insert adds a notification (for compatibility, but prefer BatchInsert)
func (r *PostgresRepository) Insert(ctx context.Context, notification *models.Notification) error {
	return r.BatchInsert(ctx, []*models.Notification{notification})
}

// BatchInsert inserts multiple notifications using prepared statement for high performance
func (r *PostgresRepository) BatchInsert(ctx context.Context, notifications []*models.Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	// Use transaction with prepared statement for fast batch inserts
	txn, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer txn.Rollback()

	stmt, err := txn.PrepareContext(ctx, `
		INSERT INTO notifications (
			notification_id, user_id, event_type, priority, payload,
			status, event_timestamp, notification_received_timestamp,
			is_read, retry_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, notif := range notifications {
		// Convert payload to JSONB
		payloadJSON, err := json.Marshal(notif.Payload)
		if err != nil {
			r.logger.Warn("failed to marshal payload, using empty object",
				zap.Error(err),
				zap.String("notification_id", notif.NotificationID.String()))
			payloadJSON = []byte("{}")
		}

		status := notif.Status
		if status == "" {
			status = "not_pushed"
		}

		_, err = stmt.ExecContext(ctx,
			notif.NotificationID,
			notif.UserID,
			string(notif.EventType),
			string(notif.Priority),
			payloadJSON,
			status,
			notif.EventTimestamp,
			notif.NotificationReceivedTimestamp,
			notif.IsRead,
			notif.RetryCount,
			notif.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert notification: %w", err)
		}
	}

	if err := txn.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Debug("batch inserted to postgres",
		zap.Int("count", len(notifications)))

	return nil
}

// ClaimBatch claims a batch of notifications for processing
// Uses FOR UPDATE SKIP LOCKED for high concurrency without blocking
func (r *PostgresRepository) ClaimBatch(ctx context.Context, instanceID string, batchSize int, leaseDuration time.Duration) ([]*NotificationBatch, error) {
	query := `
		UPDATE notifications
		SET status = 'claimed',
		    instance_id = $1,
		    lease_timeout = $2
		FROM (
			SELECT notification_id
			FROM notifications
			WHERE status = 'not_pushed'
			ORDER BY priority DESC, created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		) AS batch
		WHERE notifications.notification_id = batch.notification_id
		RETURNING 
			notifications.notification_id,
			notifications.user_id,
			notifications.event_type,
			notifications.priority,
			notifications.event_timestamp,
			notifications.notification_received_timestamp,
			notifications.payload::text
	`

	leaseTimeout := time.Now().Add(leaseDuration)
	rows, err := r.db.QueryContext(ctx, query, instanceID, leaseTimeout, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to claim batch: %w", err)
	}
	defer rows.Close()

	var batch []*NotificationBatch
	for rows.Next() {
		var nb NotificationBatch
		var payloadStr string

		if err := rows.Scan(
			&nb.NotificationID,
			&nb.UserID,
			&nb.EventType,
			&nb.Priority,
			&nb.EventTimestamp,
			&nb.NotificationReceivedTimestamp,
			&payloadStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		nb.Payload = payloadStr
		batch = append(batch, &nb)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return batch, nil
}

// BatchUpdateStatus updates the status of multiple notifications
func (r *PostgresRepository) BatchUpdateStatus(ctx context.Context, updates []*StatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	txn, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer txn.Rollback()

	stmt, err := txn.PrepareContext(ctx, `
		UPDATE notifications
		SET status = $1,
		    delivered_at = CASE WHEN $1 = 'pushed' THEN NOW() ELSE delivered_at END,
		    error_message = $2,
		    instance_id = NULL,
		    lease_timeout = NULL
		WHERE notification_id = $3
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	for _, update := range updates {
		if _, err := stmt.ExecContext(ctx, update.Status, update.ErrorMsg, update.NotificationID); err != nil {
			r.logger.Warn("failed to update notification status",
				zap.Error(err),
				zap.String("notification_id", update.NotificationID.String()))
			// Continue with other updates
		}
	}

	if err := txn.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Debug("batch updated status",
		zap.Int("count", len(updates)))

	return nil
}

// ReclaimStaleTasks reclaims notifications with expired leases
func (r *PostgresRepository) ReclaimStaleTasks(ctx context.Context) (int, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE notifications
		SET status = 'not_pushed',
		    instance_id = NULL,
		    lease_timeout = NULL,
		    retry_count = retry_count + 1
		WHERE status = 'claimed'
		AND lease_timeout < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale tasks: %w", err)
	}

	count, _ := result.RowsAffected()
	if count > 0 {
		r.logger.Info("reclaimed stale tasks", zap.Int64("count", count))
	}

	return int(count), nil
}

// GetUserNotifications retrieves recent notifications for a user
func (r *PostgresRepository) GetUserNotifications(ctx context.Context, userID string, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			notification_id,
			user_id,
			event_type,
			priority,
			status,
			event_timestamp,
			notification_received_timestamp,
			delivered_at,
			EXTRACT(EPOCH FROM (delivered_at - event_timestamp)) as delay_seconds
		FROM notifications
		WHERE user_id = $1
		ORDER BY event_timestamp DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var (
			notificationID                uuid.UUID
			userIDVal                     string
			eventType                     string
			priority                      string
			status                        string
			eventTimestamp                time.Time
			notificationReceivedTimestamp time.Time
			deliveredAt                   sql.NullTime
			delaySeconds                  sql.NullFloat64
		)

		if err := rows.Scan(
			&notificationID,
			&userIDVal,
			&eventType,
			&priority,
			&status,
			&eventTimestamp,
			&notificationReceivedTimestamp,
			&deliveredAt,
			&delaySeconds,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		result := map[string]interface{}{
			"notification_id":                 notificationID.String(),
			"user_id":                         userIDVal,
			"event_type":                      eventType,
			"priority":                        priority,
			"status":                          status,
			"event_timestamp":                 eventTimestamp,
			"notification_received_timestamp": notificationReceivedTimestamp,
		}

		if deliveredAt.Valid {
			result["notification_delivered_timestamp"] = deliveredAt.Time
		}
		if delaySeconds.Valid {
			result["delay_seconds"] = delaySeconds.Float64
		}

		results = append(results, result)
	}

	return results, nil
}

// GetStats retrieves notification statistics
func (r *PostgresRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'not_pushed') as pending,
			COUNT(*) FILTER (WHERE status = 'pushed') as delivered,
			COUNT(*) FILTER (WHERE status = 'claimed') as claimed,
			COUNT(*) FILTER (WHERE status = 'failed') as failed,
			COUNT(*) as total
		FROM notifications
	`

	var stats struct {
		Pending   int64
		Delivered int64
		Claimed   int64
		Failed    int64
		Total     int64
	}

	if err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.Pending,
		&stats.Delivered,
		&stats.Claimed,
		&stats.Failed,
		&stats.Total,
	); err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return map[string]interface{}{
		"pending":   stats.Pending,
		"delivered": stats.Delivered,
		"claimed":   stats.Claimed,
		"failed":    stats.Failed,
		"total":     stats.Total,
	}, nil
}

// Close closes the database connection
func (r *PostgresRepository) Close(ctx context.Context) error {
	return r.db.Close()
}

// Flush is a no-op for PostgreSQL (kept for interface compatibility)
func (r *PostgresRepository) Flush(ctx context.Context) error {
	return nil
}
