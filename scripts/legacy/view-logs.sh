#!/bin/bash

# Docker Container Logs Viewer
# View and tail logs from all Docker containers

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║     NOTIFICATION SYSTEM - DOCKER LOGS VIEWER                   ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""
echo "Select log to view:"
echo ""
echo "  1) Kafka Logs"
echo "  2) ClickHouse Logs"
echo "  3) Zookeeper Logs"
echo "  4) All Docker Logs (Combined)"
echo "  5) Tail All Logs (Live)"
echo ""
echo "  6) Notification Service (Go Binary)"
echo "  7) Job Service (Go Binary)"
echo "  8) Connections Service (Go Binary)"
echo "  9) Followers Service (Go Binary)"
echo " 10) All Go Service Logs"
echo ""
read -p "Enter choice [1-10]: " choice

case $choice in
    1)
        echo "📨 Kafka Logs (Last 50 lines, Ctrl+C to exit):"
        docker logs --tail 50 -f notif-kafka
        ;;
    2)
        echo "💾 ClickHouse Logs (Last 50 lines, Ctrl+C to exit):"
        docker logs --tail 50 -f notif-clickhouse
        ;;
    3)
        echo "🔧 Zookeeper Logs (Last 50 lines, Ctrl+C to exit):"
        docker logs --tail 50 -f notif-zookeeper
        ;;
    4)
        echo "🐳 All Docker Container Logs (Last 20 lines each):"
        echo ""
        echo "━━━━ KAFKA ━━━━"
        docker logs --tail 20 notif-kafka
        echo ""
        echo "━━━━ CLICKHOUSE ━━━━"
        docker logs --tail 20 notif-clickhouse
        echo ""
        echo "━━━━ ZOOKEEPER ━━━━"
        docker logs --tail 20 notif-zookeeper
        ;;
    5)
        echo "📡 Tailing All Docker Logs (Ctrl+C to exit):"
        docker-compose -f docker-compose.infra.yml logs -f
        ;;
    6)
        echo "🔔 Notification Service Logs (Ctrl+C to exit):"
        tail -f /tmp/notification-service.log | jq -r '"\(.ts | strftime("%H:%M:%S")) [\(.level | ascii_upcase)] \(.msg) \(.user_id // "") \(.event_type // "")"'
        ;;
    7)
        echo "💼 Job Service Logs (Ctrl+C to exit):"
        tail -f /tmp/job-service.log | jq -r '"\(.ts | strftime("%H:%M:%S")) [\(.level | ascii_upcase)] \(.msg)"'
        ;;
    8)
        echo "🤝 Connections Service Logs (Ctrl+C to exit):"
        tail -f /tmp/connections-service.log | jq -r '"\(.ts | strftime("%H:%M:%S")) [\(.level | ascii_upcase)] \(.msg)"'
        ;;
    9)
        echo "👥 Followers Service Logs (Ctrl+C to exit):"
        tail -f /tmp/followers-service.log | jq -r '"\(.ts | strftime("%H:%M:%S")) [\(.level | ascii_upcase)] \(.msg)"'
        ;;
    10)
        echo "📋 All Go Service Logs (Ctrl+C to exit):"
        echo "Tailing all service logs..."
        tail -f /tmp/notification-service.log /tmp/job-service.log /tmp/connections-service.log /tmp/followers-service.log
        ;;
    *)
        echo "❌ Invalid choice"
        exit 1
        ;;
esac
