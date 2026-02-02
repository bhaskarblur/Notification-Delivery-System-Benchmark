#!/bin/bash

# Stop all services

echo "🛑 Stopping Notification Delivery System..."
echo ""

# Stop Go services
echo "Stopping Go services..."
pkill -f "bin/notification-service" && echo "  ✓ notification-service stopped"
pkill -f "bin/job-service" && echo "  ✓ job-service stopped"
pkill -f "bin/connections-service" && echo "  ✓ connections-service stopped"
pkill -f "bin/followers-service" && echo "  ✓ followers-service stopped"
echo ""

# Stop Docker containers
echo "Stopping Docker containers..."
cd "$(dirname "$0")/.."
docker compose -f docker-compose.infra.yml down -v
echo ""

echo "✅ All services stopped!"
