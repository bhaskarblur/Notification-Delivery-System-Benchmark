#!/bin/bash

# Complete System Startup Script
# Starts everything needed for the dual worker pool architecture

set -e

cd "$(dirname "$0")/.."

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║   Notification Delivery System - Complete Startup              ║"
echo "║   Architecture: Dual Worker Pool + Priority-based Processing   ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Parse arguments
EVENT_RATE=${1:-10}
NUM_USERS=${2:-100}

echo "Configuration:"
echo "  Event rate: $EVENT_RATE events/sec per service"
echo "  Number of users: $NUM_USERS"
echo ""

# Step 1: Build
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Step 1/5: Building services"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
./scripts/build-all.sh
echo ""

# Step 2: Start infrastructure
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Step 2/5: Starting infrastructure (Kafka, ClickHouse, Zookeeper)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
./scripts/start-infra.sh
echo ""

# Step 3: Initialize ClickHouse
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Step 3/5: Initializing ClickHouse schema"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
./scripts/init-clickhouse.sh
echo ""

# Step 4: Start notification service
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Step 4/5: Starting Notification Service"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
./scripts/start-notification.sh
sleep 5
echo ""

# Step 5: Start producers
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Step 5/5: Starting Producer Services"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
export EVENT_RATE=$EVENT_RATE
export NUM_USERS=$NUM_USERS
./scripts/start-producers.sh
echo ""

# Wait a bit for metrics
sleep 10

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                    🎉 SYSTEM READY! 🎉                          ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""
echo "System Status:"
docker ps --filter "name=notif-" --format "  ✓ {{.Names}}: {{.Status}}"
echo ""
ps aux | grep -E "bin/(notification-service|job|connections|followers)" | grep -v grep | awk '{print "  ✓ " $11 " (PID: " $2 ")"}'
echo ""
echo "Monitoring Commands:"
echo "  ./scripts/monitor.sh          - Live dashboard"
echo "  ./scripts/view-logs.sh        - View logs"
echo "  ./scripts/analyze-delays.sh   - Delay analysis"
echo ""
echo "Endpoints:"
echo "  Health: http://localhost:8080/health"
echo "  SSE Stream: http://localhost:8080/notifications/stream?user_id=user_1"
echo "  pprof: http://localhost:6060/debug/pprof/"
echo ""
echo "Stop Everything:"
echo "  ./scripts/stop-all.sh"
