#!/bin/bash

# Initialize ClickHouse with distributed processing schema (with locking)

set -e

cd "$(dirname "$0")/.."

echo "🔧 Initializing ClickHouse schema..."
echo ""

# Execute initialization script
docker exec -i notif-clickhouse clickhouse-client --user admin --password admin123 --multiquery < scripts/init-clickhouse-distributed.sql

echo ""
echo "✅ ClickHouse schema initialized!"
echo ""

# Verify tables
echo "📊 Verifying tables..."
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "SHOW TABLES FROM notifications FORMAT PrettyCompact"

echo ""
echo "🎉 Database ready for dual worker pool architecture!"
echo ""
echo "Next step: Start notification service"
echo "  ./scripts/start-notification.sh"
