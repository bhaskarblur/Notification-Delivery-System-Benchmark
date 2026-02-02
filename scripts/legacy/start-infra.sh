#!/bin/bash

# Start Docker infrastructure (Kafka, ClickHouse, Zookeeper)

set -e

cd "$(dirname "$0")/.."

echo "🚀 Starting infrastructure services..."
echo ""

docker compose -f docker-compose.infra.yml up -d

echo ""
echo "⏳ Waiting for services to be healthy..."
echo ""

# Wait for ClickHouse
echo "Waiting for ClickHouse..."
for i in {1..30}; do
    if docker exec notif-clickhouse clickhouse-client --query "SELECT 1" > /dev/null 2>&1; then
        echo "✅ ClickHouse is ready"
        break
    fi
    sleep 2
done

# Wait for Kafka
echo "Waiting for Kafka..."
for i in {1..30}; do
    if docker exec notif-kafka kafka-topics --bootstrap-server localhost:9092 --list > /dev/null 2>&1; then
        echo "✅ Kafka is ready"
        break
    fi
    sleep 2
done

echo ""
echo "🎉 Infrastructure is ready!"
echo ""
echo "Services running:"
docker ps --filter "name=notif-" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo ""
echo "Next step: Initialize ClickHouse schema"
echo "  ./scripts/init-clickhouse.sh"
