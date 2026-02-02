#!/bin/bash

# Build all services for the new architecture
# Consumer writes to DB, Task Picker delivers with dual worker pools

set -e

echo "🔨 Building Notification Delivery System..."
echo ""

cd "$(dirname "$0")/.."

# Build notification service
echo "📦 Building notification-service..."
go build -o bin/notification-service ./cmd/notification-service/main.go
echo "✅ notification-service built"
echo ""

# Build producer services (unchanged)
echo "📦 Building job-service..."
go build -o bin/job-service ./cmd/job-service/main.go
echo "✅ job-service built"
echo ""

echo "📦 Building connections-service..."
go build -o bin/connections-service ./cmd/connections-service/main.go
echo "✅ connections-service built"
echo ""

echo "📦 Building followers-service..."
go build -o bin/followers-service ./cmd/followers-service/main.go
echo "✅ followers-service built"
echo ""

echo "🎉 All services built successfully!"
echo ""
echo "Binaries:"
ls -lh bin/
echo ""
echo "Next steps:"
echo "1. Start infrastructure: ./scripts/start-infra.sh"
echo "2. Initialize ClickHouse: ./scripts/init-clickhouse.sh"
echo "3. Start notification service: ./scripts/start-notification.sh"
echo "4. Start producers: ./scripts/start-producers.sh"
