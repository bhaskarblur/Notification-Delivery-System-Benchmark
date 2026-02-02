#!/bin/bash

# Notification Delivery System - Monitoring Dashboard
# Real-time monitoring of services, logs, and metrics

clear
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║     NOTIFICATION DELIVERY SYSTEM - LIVE MONITOR                ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Function to display section
section() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  $1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# Docker Containers Status
section "🐳 DOCKER CONTAINERS"
docker ps --filter "name=notif" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -v "^NAMES"

# Go Services Status
section "🚀 GO SERVICES"
ps aux | grep -E "(notification-service|job-service|connections-service|followers-service)" | grep -v grep | awk '{printf "%-30s PID: %-8s CPU: %-5s MEM: %-5s\n", $11, $2, $3"%", $4"%"}'

# Kafka Stats
section "📨 KAFKA STATISTICS"
TOTAL_MESSAGES=$(docker exec notif-kafka kafka-run-class kafka.tools.GetOffsetShell --broker-list localhost:9092 --topic notifications 2>/dev/null | awk -F: '{sum+=$3} END{print sum}')
echo "Total Messages Produced: $TOTAL_MESSAGES"
echo ""
echo "Partition Distribution:"
docker exec notif-kafka kafka-run-class kafka.tools.GetOffsetShell --broker-list localhost:9092 --topic notifications 2>/dev/null | awk -F: '{printf "  Partition %s: %s messages\n", $2, $3}'

# Consumer Group Lag
echo ""
echo "Consumer Group Status:"
docker exec notif-kafka kafka-consumer-groups --bootstrap-server localhost:9092 --describe --group notification-service-group 2>/dev/null | grep -E "(GROUP|notifications)" | head -5

# ClickHouse Stats
section "💾 CLICKHOUSE STATISTICS"
TOTAL_NOTIFS=$(docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "SELECT COUNT(*) FROM notifications.notifications" 2>/dev/null)
echo "Total Notifications Stored: $TOTAL_NOTIFS"
echo ""

echo "Priority Distribution:"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    COUNT(*) as count,
    ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*) FROM notifications.notifications), 2) as percentage
FROM notifications.notifications
GROUP BY priority
ORDER BY priority
FORMAT PrettyCompact
" 2>/dev/null

echo ""
echo "Event Type Distribution (Top 10):"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    event_type,
    COUNT(*) as count
FROM notifications.notifications
GROUP BY event_type
ORDER BY count DESC
LIMIT 10
FORMAT PrettyCompact
" 2>/dev/null

# Delay Analysis
section "⏱️  NOTIFICATION DELAY ANALYSIS"
echo "Recent Notifications (Last 5 Minutes):"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    COUNT(*) as notifications,
    ROUND(AVG(delay_seconds), 2) as avg_delay_sec,
    MIN(delay_seconds) as min_delay,
    MAX(delay_seconds) as max_delay,
    quantile(0.50)(delay_seconds) as p50,
    quantile(0.95)(delay_seconds) as p95
FROM notifications.notifications
WHERE event_timestamp > now() - INTERVAL 5 MINUTE
GROUP BY priority
ORDER BY priority
FORMAT PrettyCompact
" 2>/dev/null

echo ""
echo "Delay Distribution by Priority (All Time):"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    COUNT(*) as total,
    ROUND(AVG(delay_seconds), 2) as avg_sec,
    MIN(delay_seconds) as min_sec,
    MAX(delay_seconds) as max_sec,
    quantile(0.50)(delay_seconds) as p50,
    quantile(0.95)(delay_seconds) as p95,
    quantile(0.99)(delay_seconds) as p99
FROM notifications.notifications
GROUP BY priority
ORDER BY priority
FORMAT PrettyCompact
" 2>/dev/null

# Throughput
section "📊 THROUGHPUT METRICS"
echo "Messages per Minute (Last 10 Minutes):"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    toStartOfMinute(event_timestamp) as minute,
    COUNT(*) as messages
FROM notifications.notifications
WHERE event_timestamp > now() - INTERVAL 10 MINUTE
GROUP BY minute
ORDER BY minute DESC
FORMAT PrettyCompact
" 2>/dev/null

# Health Check
section "🏥 HEALTH STATUS"
HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null)
if [ $? -eq 0 ]; then
    echo "✅ Notification Service: HEALTHY"
    echo "$HEALTH" | jq .
else
    echo "❌ Notification Service: UNHEALTHY"
fi

section "📝 RECENT SERVICE LOGS"
echo ""
echo "Notification Service (Last 5 lines):"
tail -5 /tmp/notification-service.log 2>/dev/null | jq -r '"\(.ts | strftime("%H:%M:%S")) [\(.level | ascii_upcase)] \(.msg)"' 2>/dev/null || tail -5 /tmp/notification-service.log

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Last Updated: $(date '+%Y-%m-%d %H:%M:%S')"
echo "  Refresh: Run this script again or use 'watch -n 5 ./scripts/monitor.sh'"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
