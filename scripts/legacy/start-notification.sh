#!/bin/bash

# Start Notification Service (Consumer + Task Picker with dual worker pools)

set -e

cd "$(dirname "$0")/.."

# Configuration
export KAFKA_BROKERS="localhost:9092"
export CLICKHOUSE_HOST="localhost:9000"
export CLICKHOUSE_USER="admin"
export CLICKHOUSE_PASSWORD="admin123"
export CLICKHOUSE_DATABASE="notifications"

echo "🚀 Starting Notification Service..."
echo ""
echo "Architecture:"
echo "  ✓ Kafka Consumer → ClickHouse (status=not_pushed)"
echo "  ✓ Task Picker: 5 picker workers + 20 delivery workers"
echo "  ✓ Batch size: 500 notifications"
echo "  ✓ Batch status updates: every 1 second"
echo ""

# Start service
./bin/notification-service 2>&1 | tee /tmp/notification-service.log &

echo "✅ Notification Service started!"
echo ""
echo "PID: $!"
echo "Logs: /tmp/notification-service.log"
echo ""
echo "Endpoints:"
echo "  Health: http://localhost:8080/health"
echo "  SSE Stream: http://localhost:8080/notifications/stream?user_id=user_1"
echo "  pprof: http://localhost:6060/debug/pprof/"
echo ""
echo "Monitor with:"
echo "  tail -f /tmp/notification-service.log"
echo "  ./scripts/monitor.sh"
