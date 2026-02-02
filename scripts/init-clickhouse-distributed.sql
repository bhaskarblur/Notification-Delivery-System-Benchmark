-- ClickHouse Initialization Script V2 - Distributed Processing
-- Database: notifications
-- Purpose: Store notifications with priority-based delivery tracking (30-day TTL)
-- Architecture: Two-phase (Consumer writes, Task Picker delivers with distributed locking)

-- Create database if not exists
CREATE DATABASE IF NOT EXISTS notifications;

-- Main notifications table with delivery status tracking & distributed locking
CREATE TABLE IF NOT EXISTS notifications.notifications (
    notification_id UUID DEFAULT generateUUIDv4(),
    user_id String,
    event_type String,
    -- Priority: Determines processing order (1=HIGH, 2=MEDIUM, 3=LOW)
    priority Enum8('HIGH' = 1, 'MEDIUM' = 2, 'LOW' = 3),
    -- Status: Tracks delivery lifecycle
    status Enum8('not_pushed' = 0, 'processing' = 1, 'pushed' = 2, 'delivered' = 3, 'failed' = 4),
    event_timestamp DateTime64(3),
    notification_received_timestamp DateTime64(3),
    notification_pushed_timestamp DateTime64(3) DEFAULT toDateTime64(0, 3),
    notification_delivered_timestamp DateTime64(3) DEFAULT toDateTime64(0, 3),
    -- Distributed locking fields
    processing_by Nullable(String),  -- Instance ID (UUID) of the service processing this notification
    processing_started_at Nullable(DateTime64(3)),  -- When processing started
    processing_lease_expires_at Nullable(DateTime64(3)),  -- Lease expiry time (30s from start)
    -- Time from received to pushed (task picker latency)
    processing_latency_ms Int32 DEFAULT 0,
    payload String,
    is_read UInt8 DEFAULT 0,
    retry_count UInt8 DEFAULT 0,
    last_error String DEFAULT '',
    created_at DateTime DEFAULT now(),
    updated_at DateTime DEFAULT now(),
    INDEX idx_user_id user_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_event_type event_type TYPE set(100) GRANULARITY 1,
    INDEX idx_status status TYPE set(5) GRANULARITY 1,
    INDEX idx_priority priority TYPE set(3) GRANULARITY 1,
    INDEX idx_lease_expiry processing_lease_expires_at TYPE minmax GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(event_timestamp)
ORDER BY (priority, notification_received_timestamp, user_id, notification_id)
TTL created_at + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Priority-based delivery queue view (for task picker monitoring)
-- NOTE: Task picker uses direct queries with locking, not this view
CREATE MATERIALIZED VIEW IF NOT EXISTS notifications.delivery_queue_mv
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (priority, notification_received_timestamp, notification_id)
AS SELECT
    notification_id,
    user_id,
    event_type,
    priority,
    status,
    notification_received_timestamp,
    processing_by,
    processing_lease_expires_at,
    payload,
    retry_count,
    updated_at
FROM notifications.notifications
WHERE (status = 'not_pushed' OR (status = 'processing' AND processing_lease_expires_at < now())) 
  AND retry_count < 3;

-- Notification stats by priority
CREATE MATERIALIZED VIEW IF NOT EXISTS notifications.notification_stats_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMMDD(event_date)
ORDER BY (event_date, priority, event_type, status)
AS SELECT
    toDate(event_timestamp) AS event_date,
    priority,
    event_type,
    status,
    count() AS notification_count,
    avg(processing_latency_ms) AS avg_processing_latency_ms,
    quantile(0.50)(processing_latency_ms) AS p50_latency_ms,
    quantile(0.95)(processing_latency_ms) AS p95_latency_ms,
    quantile(0.99)(processing_latency_ms) AS p99_latency_ms,
    countIf(status = 'delivered') AS delivered_count,
    countIf(status = 'failed') AS failed_count
FROM notifications.notifications
GROUP BY event_date, priority, event_type, status;

-- User notification summary with delivery metrics
CREATE MATERIALIZED VIEW IF NOT EXISTS notifications.user_notification_summary_mv
ENGINE = ReplacingMergeTree()
ORDER BY (user_id, event_date)
AS SELECT
    user_id,
    toDate(event_timestamp) AS event_date,
    count() AS total_notifications,
    countIf(status = 'delivered') AS delivered_notifications,
    countIf(status = 'failed') AS failed_notifications,
    countIf(is_read = 1) AS read_notifications,
    avg(processing_latency_ms) AS avg_latency_ms
FROM notifications.notifications
GROUP BY user_id, event_date;

-- Priority-based delivery performance view
CREATE MATERIALIZED VIEW IF NOT EXISTS notifications.priority_performance_mv
ENGINE = SummingMergeTree()
ORDER BY (hour, priority)
AS SELECT
    toStartOfHour(notification_received_timestamp) AS hour,
    priority,
    count() AS total_count,
    countIf(status = 'pushed' OR status = 'delivered') AS pushed_count,
    countIf(status = 'delivered') AS delivered_count,
    countIf(status = 'failed') AS failed_count,
    avg(processing_latency_ms) AS avg_latency_ms,
    quantile(0.95)(processing_latency_ms) AS p95_latency_ms
FROM notifications.notifications
GROUP BY hour, priority;

-- Instance processing metrics (for monitoring which instance is doing what)
CREATE MATERIALIZED VIEW IF NOT EXISTS notifications.instance_metrics_mv
ENGINE = SummingMergeTree()
ORDER BY (hour, processing_by)
AS SELECT
    toStartOfHour(notification_received_timestamp) AS hour,
    assumeNotNull(processing_by) AS processing_by,
    count() AS notifications_processed,
    countIf(status = 'pushed' OR status = 'delivered') AS successful_deliveries,
    countIf(status = 'failed') AS failed_deliveries,
    avg(processing_latency_ms) AS avg_latency_ms
FROM notifications.notifications
WHERE processing_by IS NOT NULL
GROUP BY hour, processing_by;

-- Event type priority mapping (for reference)
-- HIGH Priority: job.new, job.update, job.application_status
-- MEDIUM Priority: connection.request, connection.accepted, job.application_viewed
-- LOW Priority: follower.new, follower.content_liked, follower.content_commented, connection.endorsed

-- ========================================================================
-- Helper queries for task picker operations (copy to Go code):
-- ========================================================================

-- 1. Claim notifications (atomic batch claim with lease)
-- UPDATE notifications.notifications
-- SET status = 'processing',
--     processing_by = '{instance_id}',
--     processing_started_at = now(),
--     processing_lease_expires_at = now() + INTERVAL 30 SECOND,
--     updated_at = now()
-- WHERE notification_id IN (
--     SELECT notification_id FROM notifications.notifications
--     WHERE (status = 'not_pushed' OR (status = 'processing' AND processing_lease_expires_at < now()))
--       AND retry_count < 3
--     ORDER BY priority ASC, notification_received_timestamp ASC
--     LIMIT 100
-- );

-- 2. Update status after processing (verify ownership)
-- UPDATE notifications.notifications
-- SET status = '{new_status}',
--     notification_pushed_timestamp = CASE WHEN '{new_status}' = 'pushed' THEN now() ELSE notification_pushed_timestamp END,
--     processing_latency_ms = CASE WHEN '{new_status}' = 'pushed' THEN dateDiff('millisecond', notification_received_timestamp, now()) ELSE 0 END,
--     retry_count = CASE WHEN '{new_status}' = 'failed' THEN retry_count + 1 ELSE retry_count END,
--     last_error = '{error_message}',
--     updated_at = now()
-- WHERE notification_id = '{notification_id}'
--   AND processing_by = '{instance_id}'
--   AND status = 'processing';

-- 3. Reset expired leases (background cleanup)
-- UPDATE notifications.notifications
-- SET status = 'not_pushed',
--     processing_by = NULL,
--     processing_started_at = NULL,
--     processing_lease_expires_at = NULL,
--     retry_count = retry_count + 1,
--     last_error = 'Lease expired',
--     updated_at = now()
-- WHERE status = 'processing'
--   AND processing_lease_expires_at < now()
--   AND retry_count < 3;

-- 4. Query pending work (for monitoring)
-- SELECT 
--     priority,
--     status,
--     processing_by,
--     count() as count,
--     min(notification_received_timestamp) as oldest_notification
-- FROM notifications.notifications
-- WHERE status IN ('not_pushed', 'processing')
-- GROUP BY priority, status, processing_by
-- ORDER BY priority ASC;
