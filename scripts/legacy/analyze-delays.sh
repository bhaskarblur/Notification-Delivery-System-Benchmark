#!/bin/bash

# Notification Delay Analysis Tool
# Detailed analysis of notification delivery delays

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║     NOTIFICATION DELAY ANALYSIS - DETAILED VIEW                ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

section() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  $1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

section "📊 OVERALL DELAY STATISTICS"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    COUNT(*) as total_notifications,
    ROUND(AVG(delay_seconds), 3) as avg_delay_sec,
    ROUND(MIN(delay_seconds), 3) as min_delay_sec,
    ROUND(MAX(delay_seconds), 3) as max_delay_sec,
    ROUND(quantile(0.25)(delay_seconds), 3) as p25,
    ROUND(quantile(0.50)(delay_seconds), 3) as p50_median,
    ROUND(quantile(0.75)(delay_seconds), 3) as p75,
    ROUND(quantile(0.95)(delay_seconds), 3) as p95,
    ROUND(quantile(0.99)(delay_seconds), 3) as p99
FROM notifications.notifications
GROUP BY priority
ORDER BY priority
FORMAT PrettyCompact
" 2>/dev/null

section "⏱️  DELAY RANGES BY PRIORITY"
echo "Target Delays:"
echo "  HIGH:   0-1 second (instant delivery)"
echo "  MEDIUM: 15-30 minutes (900-1800 seconds)"
echo "  LOW:    4-6 hours (14400-21600 seconds)"
echo ""
echo "Actual Distribution:"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    countIf(delay_seconds < 1) as instant_lt_1s,
    countIf(delay_seconds >= 1 AND delay_seconds < 10) as fast_1_10s,
    countIf(delay_seconds >= 10 AND delay_seconds < 60) as medium_10_60s,
    countIf(delay_seconds >= 60 AND delay_seconds < 900) as slow_1_15min,
    countIf(delay_seconds >= 900 AND delay_seconds < 1800) as delayed_15_30min,
    countIf(delay_seconds >= 1800) as very_delayed_gt_30min,
    COUNT(*) as total
FROM notifications.notifications
GROUP BY priority
ORDER BY priority
FORMAT PrettyCompact
" 2>/dev/null

section "🔥 RECENT NOTIFICATIONS (Last 5 Minutes)"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    event_type,
    COUNT(*) as count,
    ROUND(AVG(delay_seconds), 3) as avg_delay,
    ROUND(MIN(delay_seconds), 3) as min_delay,
    ROUND(MAX(delay_seconds), 3) as max_delay
FROM notifications.notifications
WHERE event_timestamp > now() - INTERVAL 5 MINUTE
GROUP BY priority, event_type
ORDER BY priority, count DESC
FORMAT PrettyCompact
" 2>/dev/null

section "📈 HOURLY DELAY TRENDS (Last 24 Hours)"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    toStartOfHour(event_timestamp) as hour,
    priority,
    COUNT(*) as notifications,
    ROUND(AVG(delay_seconds), 2) as avg_delay
FROM notifications.notifications
WHERE event_timestamp > now() - INTERVAL 24 HOUR
GROUP BY hour, priority
ORDER BY hour DESC, priority
LIMIT 20
FORMAT PrettyCompact
" 2>/dev/null

section "🎯 PRIORITY COMPLIANCE CHECK"
echo "Checking if delays meet target SLAs..."
echo ""
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
WITH 
    high_target AS (SELECT 1 as threshold_sec),
    medium_target AS (SELECT 1800 as threshold_sec),
    low_target AS (SELECT 21600 as threshold_sec)
SELECT 
    'HIGH' as priority,
    COUNT(*) as total,
    countIf(delay_seconds <= 1) as within_sla,
    ROUND(countIf(delay_seconds <= 1) * 100.0 / COUNT(*), 2) as sla_compliance_pct,
    '< 1s' as target
FROM notifications.notifications
WHERE priority = 'HIGH'

UNION ALL

SELECT 
    'MEDIUM' as priority,
    COUNT(*) as total,
    countIf(delay_seconds <= 1800) as within_sla,
    ROUND(countIf(delay_seconds <= 1800) * 100.0 / COUNT(*), 2) as sla_compliance_pct,
    '< 30min' as target
FROM notifications.notifications
WHERE priority = 'MEDIUM'

UNION ALL

SELECT 
    'LOW' as priority,
    COUNT(*) as total,
    countIf(delay_seconds <= 21600) as within_sla,
    ROUND(countIf(delay_seconds <= 21600) * 100.0 / COUNT(*), 2) as sla_compliance_pct,
    '< 6h' as target
FROM notifications.notifications
WHERE priority = 'LOW'
FORMAT PrettyCompact
" 2>/dev/null

section "🔍 SAMPLE RECENT NOTIFICATIONS WITH TIMESTAMPS"
echo "Showing 10 most recent notifications with full timing details:"
echo ""
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    event_type,
    priority,
    user_id,
    event_timestamp,
    notification_received_timestamp,
    delay_seconds,
    CONCAT(
        CASE 
            WHEN delay_seconds < 1 THEN '✅ INSTANT'
            WHEN delay_seconds < 60 THEN '⚡ FAST'
            WHEN delay_seconds < 900 THEN '⏱️  DELAYED'
            ELSE '🐌 SLOW'
        END
    ) as status
FROM notifications.notifications
ORDER BY event_timestamp DESC
LIMIT 10
FORMAT PrettyCompact
" 2>/dev/null

section "📊 DELAY HISTOGRAM"
echo "Distribution of delay_seconds across notifications:"
echo ""
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    CASE 
        WHEN delay_seconds < 0.1 THEN '0-100ms'
        WHEN delay_seconds < 1 THEN '100ms-1s'
        WHEN delay_seconds < 10 THEN '1-10s'
        WHEN delay_seconds < 60 THEN '10-60s'
        WHEN delay_seconds < 300 THEN '1-5min'
        WHEN delay_seconds < 900 THEN '5-15min'
        WHEN delay_seconds < 1800 THEN '15-30min'
        WHEN delay_seconds < 3600 THEN '30-60min'
        WHEN delay_seconds < 14400 THEN '1-4h'
        WHEN delay_seconds < 21600 THEN '4-6h'
        ELSE '>6h'
    END as delay_bucket,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER (PARTITION BY priority), 2) as pct
FROM notifications.notifications
GROUP BY priority, delay_bucket
ORDER BY priority, 
    CASE delay_bucket
        WHEN '0-100ms' THEN 1
        WHEN '100ms-1s' THEN 2
        WHEN '1-10s' THEN 3
        WHEN '10-60s' THEN 4
        WHEN '1-5min' THEN 5
        WHEN '5-15min' THEN 6
        WHEN '15-30min' THEN 7
        WHEN '30-60min' THEN 8
        WHEN '1-4h' THEN 9
        WHEN '4-6h' THEN 10
        ELSE 11
    END
FORMAT PrettyCompact
" 2>/dev/null

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Analysis Complete - $(date '+%Y-%m-%d %H:%M:%S')"
echo "  For live monitoring: ./scripts/monitor.sh"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
