-- PostgreSQL Schema for Notification Delivery System
-- Optimized for OLTP workload with high write throughput

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create notifications table with optimized indexes
CREATE TABLE IF NOT EXISTS notifications (
    notification_id UUID PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    priority VARCHAR(10) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'not_pushed',
    event_timestamp TIMESTAMPTZ NOT NULL,
    notification_received_timestamp TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    lease_timeout TIMESTAMPTZ,
    instance_id VARCHAR(255)
);

-- Index for Task Picker: Find pending notifications by user, ordered by priority
-- This is the MOST CRITICAL index for performance
CREATE INDEX idx_user_status_priority ON notifications (user_id, status, priority DESC, created_at)
WHERE status IN ('not_pushed', 'claimed');

-- Index for pending notifications (faster picker queries)
CREATE INDEX idx_status_created ON notifications (status, created_at)
WHERE status = 'not_pushed';

-- Index for lease expiry (reclaiming stale tasks)
CREATE INDEX idx_lease_timeout ON notifications (lease_timeout)
WHERE status = 'claimed' AND lease_timeout IS NOT NULL;

-- Index for delivered notifications lookup
CREATE INDEX idx_user_delivered ON notifications (user_id, delivered_at DESC)
WHERE status = 'pushed';

-- Index for JSONB payload queries (if needed)
CREATE INDEX idx_payload_gin ON notifications USING gin (payload);

-- Partial index for error tracking
CREATE INDEX idx_error_tracking ON notifications (status, retry_count, created_at)
WHERE status = 'failed';

-- Create table for performance metrics tracking
CREATE TABLE IF NOT EXISTS notification_metrics (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metric_type VARCHAR(50) NOT NULL,
    metric_value BIGINT NOT NULL,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_metrics_timestamp ON notification_metrics (timestamp DESC);
CREATE INDEX idx_metrics_type ON notification_metrics (metric_type, timestamp DESC);

-- Function to clean up old delivered notifications (optional)
CREATE OR REPLACE FUNCTION cleanup_old_notifications(retention_days INTEGER DEFAULT 30)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM notifications
    WHERE status = 'pushed'
    AND delivered_at < NOW() - (retention_days || ' days')::INTERVAL;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to reclaim stale tasks
CREATE OR REPLACE FUNCTION reclaim_stale_tasks()
RETURNS INTEGER AS $$
DECLARE
    reclaimed_count INTEGER;
BEGIN
    UPDATE notifications
    SET status = 'not_pushed',
        instance_id = NULL,
        lease_timeout = NULL,
        retry_count = retry_count + 1
    WHERE status = 'claimed'
    AND lease_timeout < NOW();
    
    GET DIAGNOSTICS reclaimed_count = ROW_COUNT;
    RETURN reclaimed_count;
END;
$$ LANGUAGE plpgsql;

-- Performance tuning: Increase statistics target for better query planning
ALTER TABLE notifications ALTER COLUMN user_id SET STATISTICS 1000;
ALTER TABLE notifications ALTER COLUMN status SET STATISTICS 1000;
ALTER TABLE notifications ALTER COLUMN priority SET STATISTICS 1000;

-- Create view for monitoring
CREATE OR REPLACE VIEW notification_stats AS
SELECT
    status,
    priority,
    COUNT(*) as count,
    AVG(EXTRACT(EPOCH FROM (NOW() - created_at))) as avg_age_seconds,
    MAX(created_at) as latest,
    MIN(created_at) as oldest
FROM notifications
GROUP BY status, priority;

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO admin;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO admin;

-- Log schema creation
DO $$
BEGIN
    RAISE NOTICE 'PostgreSQL schema created successfully';
    RAISE NOTICE 'Optimized for high-throughput OLTP workload';
    RAISE NOTICE 'Expected performance: 5,000-10,000 writes/sec';
END $$;
