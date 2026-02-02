#!/bin/bash

# Monitor Notification Delivery System Containers
# Shows real-time performance metrics for all services

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

clear

echo -e "${GREEN}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     NOTIFICATION DELIVERY SYSTEM - CONTAINER MONITOR           ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Container Stats
echo -e "${BLUE}━━━ Container Resource Usage ━━━${NC}"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" | grep notif
echo ""

# Kafka Topics
echo -e "${BLUE}━━━ Kafka Topics ━━━${NC}"
docker exec notif-kafka kafka-topics --bootstrap-server localhost:9092 --list 2>/dev/null
echo ""

# Kafka Consumer Groups
echo -e "${BLUE}━━━ Kafka Consumer Lag ━━━${NC}"
docker exec notif-kafka kafka-consumer-groups --bootstrap-server localhost:9092 --list 2>/dev/null | while read -r group; do
    if [ ! -z "$group" ]; then
        echo "Consumer Group: $group"
        docker exec notif-kafka kafka-consumer-groups --bootstrap-server localhost:9092 --describe --group "$group" 2>/dev/null | head -5
        echo ""
    fi
done

# ClickHouse Stats
echo -e "${BLUE}━━━ ClickHouse Statistics ━━━${NC}"
echo "Total Notifications:"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "SELECT count() FROM notifications.notifications" 2>/dev/null

echo ""
echo "By Priority and Status:"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    priority,
    status,
    count() as count
FROM notifications.notifications 
GROUP BY priority, status 
ORDER BY priority, status
FORMAT PrettyCompact" 2>/dev/null

echo ""
echo "Events per second (last minute):"
docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "
SELECT 
    toStartOfMinute(event_timestamp) as minute,
    count() as events_per_minute,
    round(count() / 60, 2) as events_per_second
FROM notifications.notifications 
WHERE event_timestamp >= now() - INTERVAL 2 MINUTE
GROUP BY minute 
ORDER BY minute DESC
LIMIT 5
FORMAT PrettyCompact" 2>/dev/null

# Service Health
echo ""
echo -e "${BLUE}━━━ Service Health Checks ━━━${NC}"
echo -n "Notification Service API: "
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Down${NC}"
fi

echo -n "ClickHouse: "
if docker exec notif-clickhouse clickhouse-client --query "SELECT 1" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Down${NC}"
fi

echo -n "Kafka: "
if docker exec notif-kafka kafka-broker-api-versions --bootstrap-server localhost:9092 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Healthy${NC}"
else
    echo -e "${RED}✗ Down${NC}"
fi

# Service Logs Summary
echo ""
echo -e "${BLUE}━━━ Recent Service Activity ━━━${NC}"
echo "Producer Services (last 2 log lines each):"
echo -e "${YELLOW}Job Service:${NC}"
docker compose logs --tail=2 job-service 2>/dev/null | tail -2

echo -e "${YELLOW}Connections Service:${NC}"
docker compose logs --tail=2 connections-service 2>/dev/null | tail -2

echo -e "${YELLOW}Followers Service:${NC}"
docker compose logs --tail=2 followers-service 2>/dev/null | tail -2

echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Load Configuration:${NC}"
echo "  • Job Service: 500 events/sec, 100,000 users"
echo "  • Connections Service: 300 events/sec, 100,000 users"
echo "  • Followers Service: 400 events/sec, 100,000 users"
echo "  • ${YELLOW}TOTAL: 1,200 events/sec${NC}"
echo ""
echo "Run: ${BLUE}watch -n 5 ./scripts/monitor-containers.sh${NC} for live updates"
echo "Or:  ${BLUE}docker compose logs -f [service-name]${NC} for detailed logs"
echo ""
