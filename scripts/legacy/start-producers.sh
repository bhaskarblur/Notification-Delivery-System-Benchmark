#!/bin/bash

# Start all producer services with configurable parameters

set -e

cd "$(dirname "$0")/.."

# Default configuration
EVENT_RATE=${EVENT_RATE:-10}
NUM_USERS=${NUM_USERS:-100}

echo "🚀 Starting Producer Services..."
echo ""
echo "Configuration:"
echo "  Event rate: $EVENT_RATE events/sec per service"
echo "  Number of users: $NUM_USERS"
echo "  Kafka topic: notifications"
echo ""

# Export environment variables
export KAFKA_BROKERS="localhost:9092"
export KAFKA_TOPIC="notifications"
export EVENT_RATE=$EVENT_RATE
export NUM_USERS=$NUM_USERS

# Start job-service
echo "📦 Starting job-service..."
./bin/job-service > /tmp/job-service.log 2>&1 &
JOB_PID=$!
echo "  PID: $JOB_PID"
echo "  Priority mapping: job.new=HIGH, job.update=HIGH, job.application_status=HIGH"
echo ""

# Start connections-service
echo "📦 Starting connections-service..."
./bin/connections-service > /tmp/connections-service.log 2>&1 &
CONN_PID=$!
echo "  PID: $CONN_PID"
echo "  Priority mapping: connection.request=MEDIUM, connection.accepted=MEDIUM"
echo ""

# Start followers-service
echo "📦 Starting followers-service..."
./bin/followers-service > /tmp/followers-service.log 2>&1 &
FOLLOW_PID=$!
echo "  PID: $FOLLOW_PID"
echo "  Priority mapping: follower.*=LOW"
echo ""

sleep 2

echo "✅ All producer services started!"
echo ""
echo "Services:"
echo "  job-service: PID $JOB_PID (logs: /tmp/job-service.log)"
echo "  connections-service: PID $CONN_PID (logs: /tmp/connections-service.log)"
echo "  followers-service: PID $FOLLOW_PID (logs: /tmp/followers-service.log)"
echo ""
echo "Total event rate: ~$((EVENT_RATE * 3)) events/sec"
echo ""
echo "Stop with:"
echo "  pkill -f 'bin/(job|connections|followers)-service'"
echo ""
echo "Monitor with:"
echo "  ./scripts/monitor.sh"
echo "  ./scripts/analyze-delays.sh"
