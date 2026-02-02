.PHONY: help init build test docker-up docker-down clean

# Variables
BINARY_DIR := bin
SERVICES := notification-service job-service connections-service followers-service
EVENT_RATE ?= 10
NUM_USERS ?= 100

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
RED := \033[0;31m
NC := \033[0m

help: ## Show this help message
	@echo "$(GREEN)╔════════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║   Notification Delivery System - Makefile Commands            ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(BLUE)🚀 Quick Start:$(NC)"
	@echo "  make setup           - Complete setup (build + start infra + init DB + start all)"
	@echo "  make start           - Start all services (EVENT_RATE=50 NUM_USERS=500 for custom)"
	@echo "  make monitor         - Live monitoring dashboard"
	@echo "  make stop            - Stop all services"
	@echo ""
	@echo "$(BLUE)📋 All Commands:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-25s$(NC) %s\n", $$1, $$2}'

init: ## Initialize project (create directories, download dependencies)
	@echo "$(GREEN)Initializing project...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@mkdir -p results/benchmarks results/charts
	@mkdir -p internal/{notification,producer,models,metrics,config}
	@mkdir -p pkg/{clickhouse,kafka}
	@mkdir -p cmd/{notification-service,job-service,connections-service,followers-service}
	@go mod download
	@echo "$(GREEN)✓ Project initialized$(NC)"

setup: init build infra-start db-init ## Complete setup: init + build + start infra + init DB
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║              ✓ SETUP COMPLETE - READY TO START!               ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(BLUE)Next step:$(NC) make start"

deps: ## Download Go dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	go mod download
	go mod tidy
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

build: ## Build all services
	@echo "$(GREEN)Building all services...$(NC)"
	@for service in $(SERVICES); do \
		echo "  Building $$service..."; \
		go build -o $(BINARY_DIR)/$$service ./cmd/$$service/main.go || exit 1; \
		echo "  $(GREEN)✓$(NC) $$service"; \
	done
	@echo "$(GREEN)✓ All services built$(NC)"

build-notification: ## Build notification service only
	@echo "$(GREEN)Building notification service...$(NC)"
	go build -o $(BINARY_DIR)/notification-service ./cmd/notification-service/main.go
	@echo "$(GREEN)✓ Notification service built$(NC)"

build-producers: ## Build all producer services
	@echo "$(GREEN)Building producer services...$(NC)"
	@go build -o $(BINARY_DIR)/job-service ./cmd/job-service/main.go
	@go build -o $(BINARY_DIR)/connections-service ./cmd/connections-service/main.go
	@go build -o $(BINARY_DIR)/followers-service ./cmd/followers-service/main.go
	@echo "$(GREEN)✓ Producer services built$(NC)"

build-sse-bench: ## Build SSE benchmark tool
	@echo "$(GREEN)Building SSE benchmark tool...$(NC)"
	@go build -o $(BINARY_DIR)/sse-bench ./cmd/sse-bench/main.go
	@echo "$(GREEN)✓ SSE benchmark tool built$(NC)"

infra-start: ## Start infrastructure (Kafka, ClickHouse, Zookeeper)
	@echo "$(GREEN)🚀 Starting infrastructure services...$(NC)"
	@docker compose up -d zookeeper kafka clickhouse
	@echo ""
	@echo "$(YELLOW)⏳ Waiting for services to be healthy...$(NC)"
	@echo "  Waiting for ClickHouse..."
	@for i in $$(seq 1 30); do \
		if docker exec notif-clickhouse clickhouse-client --query "SELECT 1" > /dev/null 2>&1; then \
			echo "  $(GREEN)✓$(NC) ClickHouse is ready"; \
			break; \
		fi; \
		sleep 2; \
	done
	@echo "  Waiting for Kafka..."
	@for i in $$(seq 1 30); do \
		if docker exec notif-kafka kafka-topics --bootstrap-server localhost:9092 --list > /dev/null 2>&1; then \
			echo "  $(GREEN)✓$(NC) Kafka is ready"; \
			break; \
		fi; \
		sleep 2; \
	done
	@echo ""
	@echo "$(GREEN)✓ Infrastructure is ready!$(NC)"

db-init: ## Initialize ClickHouse database schema
	@echo "$(GREEN)Initializing ClickHouse schema...$(NC)"
	@docker exec -i notif-clickhouse clickhouse-client --user admin --password admin123 --multiquery < scripts/init-clickhouse-distributed.sql
	@echo "$(GREEN)✓ Database schema initialized$(NC)"

start: ## Start all services (use EVENT_RATE=50 NUM_USERS=500 to customize)
	@echo "$(GREEN)╔════════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║   Starting Notification Delivery System                       ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(BLUE)Configuration:$(NC)"
	@echo "  Event rate: $(EVENT_RATE) events/sec per service"
	@echo "  Number of users: $(NUM_USERS)"
	@echo ""
	@$(MAKE) -s start-notification
	@sleep 5
	@$(MAKE) -s start-producers
	@sleep 10
	@echo ""
	@echo "$(GREEN)╔════════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║                    🎉 SYSTEM READY! 🎉                         ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════════════════════════════╝$(NC)"
	@$(MAKE) -s status
	@echo ""
	@echo "$(BLUE)Quick Commands:$(NC)"
	@echo "  make monitor         - Live monitoring dashboard"
	@echo "  make logs            - View all logs"
	@echo "  make analyze         - Analyze notification delays"
	@echo "  make stop            - Stop all services"

start-notification: ## Start notification service only
	@echo "$(BLUE)🚀 Starting Notification Service...$(NC)"
	@KAFKA_BROKERS=localhost:9092 \
	 CLICKHOUSE_HOST=localhost:9000 \
	 CLICKHOUSE_USER=admin \
	 CLICKHOUSE_PASSWORD=admin123 \
	 CLICKHOUSE_DATABASE=notifications \
	 ./$(BINARY_DIR)/notification-service > /tmp/notification-service.log 2>&1 &
	@echo "  $(GREEN)✓$(NC) Notification service started (PID: $$!)"
	@echo "  Logs: /tmp/notification-service.log"
	@echo "  Health: http://localhost:8080/health"
	@echo "  pprof: http://localhost:6060/debug/pprof/"

start-producers: ## Start all producer services
	@echo "$(BLUE)🚀 Starting Producer Services...$(NC)"
	@KAFKA_BROKERS=localhost:9092 \
	 KAFKA_TOPIC=notifications \
	 EVENT_RATE=$(EVENT_RATE) \
	 NUM_USERS=$(NUM_USERS) \
	 ./$(BINARY_DIR)/job-service > /tmp/job-service.log 2>&1 &
	@echo "  $(GREEN)✓$(NC) job-service started (PID: $$!)"
	@KAFKA_BROKERS=localhost:9092 \
	 KAFKA_TOPIC=notifications \
	 EVENT_RATE=$(EVENT_RATE) \
	 NUM_USERS=$(NUM_USERS) \
	 ./$(BINARY_DIR)/connections-service > /tmp/connections-service.log 2>&1 &
	@echo "  $(GREEN)✓$(NC) connections-service started (PID: $$!)"
	@KAFKA_BROKERS=localhost:9092 \
	 KAFKA_TOPIC=notifications \
	 EVENT_RATE=$(EVENT_RATE) \
	 NUM_USERS=$(NUM_USERS) \
	 ./$(BINARY_DIR)/followers-service > /tmp/followers-service.log 2>&1 &
	@echo "  $(GREEN)✓$(NC) followers-service started (PID: $$!)"

test: ## Run unit tests
	@echo "$(GREEN)Running unit tests...$(NC)"
	go test ./internal/... -v -race -coverprofile=coverage.out
	@echo "$(GREEN)✓ Tests completed$(NC)"

test-integration: ## Run integration tests
	@echo "$(GREEN)Running integration tests...$(NC)"
	go test ./internal/... -tags=integration -v -race
	@echo "$(GREEN)✓ Integration tests completed$(NC)"

test-coverage: ## Generate test coverage report
	@echo "$(GREEN)Generating coverage report...$(NC)"
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report: coverage.html$(NC)"

stop: ## Stop all services
	@echo "$(YELLOW)Stopping all services...$(NC)"
	@-pkill -f "notification-service" || true
	@-pkill -f "job-service" || true
	@-pkill -f "connections-service" || true
	@-pkill -f "followers-service" || true
	@docker compose down
	@echo "$(GREEN)✓ All services stopped$(NC)"

status: ## Show service status
	@echo "$(BLUE)Infrastructure:$(NC)"
	@docker ps --filter "name=notif-" --format "  $(GREEN)✓$(NC) {{.Names}}: {{.Status}}"
	@echo ""
	@echo "$(BLUE)Services:$(NC)"
	@ps aux | grep -E "bin/(notification-service|job|connections|followers)" | grep -v grep | awk '{print "  $(GREEN)✓$(NC) " $$11 " (PID: " $$2 ")"}' || echo "  $(RED)✗$(NC) No services running"

logs: ## View all service logs
	@echo "$(BLUE)Recent logs from all services:$(NC)"
	@echo ""
	@echo "$(YELLOW)━━━ Notification Service ━━━$(NC)"
	@tail -20 /tmp/notification-service.log 2>/dev/null || echo "No logs yet"
	@echo ""
	@echo "$(YELLOW)━━━ Job Service ━━━$(NC)"
	@tail -20 /tmp/job-service.log 2>/dev/null || echo "No logs yet"
	@echo ""
	@echo "$(YELLOW)━━━ Connections Service ━━━$(NC)"
	@tail -20 /tmp/connections-service.log 2>/dev/null || echo "No logs yet"
	@echo ""
	@echo "$(YELLOW)━━━ Followers Service ━━━$(NC)"
	@tail -20 /tmp/followers-service.log 2>/dev/null || echo "No logs yet"

logs-follow: ## Follow notification service logs
	@tail -f /tmp/notification-service.log

monitor: ## Live monitoring dashboard
	@clear
	@echo "$(GREEN)╔════════════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(GREEN)║     NOTIFICATION DELIVERY SYSTEM - LIVE MONITOR                ║$(NC)"
	@echo "$(GREEN)╚════════════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(BLUE)━━━ Docker Containers ━━━$(NC)"
	@docker ps --filter "name=notif" --format "  {{.Names}}: {{.Status}}"
	@echo ""
	@echo "$(BLUE)━━━ Go Services ━━━$(NC)"
	@ps aux | grep -E "(notification-service|job-service|connections-service|followers-service)" | grep -v grep | awk '{printf "  %-30s PID: %-8s CPU: %-5s MEM: %-5s\n", $$11, $$2, $$3"%", $$4"%"}' || echo "  No services running"
	@echo ""
	@echo "$(BLUE)━━━ Kafka Statistics ━━━$(NC)"
	@TOTAL=$$(docker exec notif-kafka kafka-run-class kafka.tools.GetOffsetShell --broker-list localhost:9092 --topic notifications 2>/dev/null | awk -F: '{sum+=$$3} END{print sum}'); \
	echo "  Total Messages: $$TOTAL"
	@echo ""
	@echo "$(BLUE)━━━ ClickHouse Statistics ━━━$(NC)"
	@TOTAL=$$(docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "SELECT COUNT(*) FROM notifications.notifications" 2>/dev/null); \
	echo "  Total Notifications: $$TOTAL"
	@echo ""
	@echo "$(BLUE)━━━ Priority Distribution ━━━$(NC)"
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "\
	SELECT priority, COUNT(*) as count, \
	       countIf(status='pushed') as pushed, \
	       countIf(status='delivered') as delivered \
	FROM notifications.notifications GROUP BY priority ORDER BY priority" 2>/dev/null | \
	awk '{print "  " $$0}'
	@echo ""
	@echo "$(YELLOW)Press Ctrl+C to exit. Run 'make monitor-loop' for auto-refresh.$(NC)"

monitor-loop: ## Live monitoring dashboard (auto-refresh every 5s)
	@watch -n 5 -c make monitor

analyze: ## Analyze notification delays and performance
	@echo "$(GREEN)Analyzing notification delays...$(NC)"
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "\
	SELECT \
	    priority, \
	    COUNT(*) as total, \
	    AVG(delay_seconds) as avg_delay_sec, \
	    quantile(0.5)(delay_seconds) as p50_delay_sec, \
	    quantile(0.95)(delay_seconds) as p95_delay_sec, \
	    quantile(0.99)(delay_seconds) as p99_delay_sec, \
	    MAX(delay_seconds) as max_delay_sec \
	FROM notifications.notifications \
	WHERE status IN ('pushed', 'delivered') \
	GROUP BY priority \
	ORDER BY priority \
	FORMAT Pretty"

# Kafka Management
kafka-topics: ## List Kafka topics
	@docker exec notif-kafka kafka-topics --list --bootstrap-server localhost:9092

kafka-create-topic: ## Create notifications topic (if needed)
	@docker exec notif-kafka kafka-topics --create --topic notifications --partitions 10 --replication-factor 1 --bootstrap-server localhost:9092 --if-not-exists
	@echo "$(GREEN)✓ Topic created/verified$(NC)"

kafka-consumer-groups: ## Show Kafka consumer groups
	@docker exec notif-kafka kafka-consumer-groups \
		--bootstrap-server localhost:9092 \
		--list

kafka-lag: ## Show consumer group lag
	@docker exec notif-kafka kafka-consumer-groups \
		--bootstrap-server localhost:9092 \
		--group notification-service-group \
		--describe

# ClickHouse Management
clickhouse-cli: ## Open ClickHouse CLI
	@docker exec -it notif-clickhouse clickhouse-client --user admin --password admin123 --database notifications

clickhouse-query: ## Run a ClickHouse query (use QUERY="..." make clickhouse-query)
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --database notifications --query "$(QUERY)"

clickhouse-tables: ## Show ClickHouse tables
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --database notifications --query "SHOW TABLES"

clickhouse-stats: ## Show notification statistics
	@echo "$(GREEN)Notification Statistics:$(NC)"
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --database notifications --query "\
	SELECT \
	    priority, \
	    COUNT(*) as total, \
	    AVG(delay_seconds) as avg_delay_sec, \
	    countIf(status='not_pushed') as not_pushed, \
	    countIf(status='processing') as processing, \
	    countIf(status='pushed') as pushed, \
	    countIf(status='delivered') as delivered, \
	    countIf(status='failed') as failed \
	FROM notifications \
	GROUP BY priority \
	ORDER BY priority \
	FORMAT Pretty"

clickhouse-reset: ## Reset ClickHouse (drop and recreate schema)
	@echo "$(RED)⚠️  This will delete all data! Press Ctrl+C to cancel...$(NC)"
	@sleep 5
	@docker exec notif-clickhouse clickhouse-client --user admin --password admin123 --query "DROP DATABASE IF EXISTS notifications"
	@$(MAKE) -s db-init
	@echo "$(GREEN)✓ Database reset complete$(NC)"

# Performance Profiling
pprof: ## Open pprof UI in browser
	@echo "$(GREEN)Opening pprof UI...$(NC)"
	@open http://localhost:6060/debug/pprof/

pprof-heap: ## Capture and analyze heap profile
	@echo "$(GREEN)Capturing heap profile...$(NC)"
	@curl -s http://localhost:6060/debug/pprof/heap -o heap.prof
	@go tool pprof -http=:8081 heap.prof

pprof-cpu: ## Capture and analyze CPU profile (30 seconds)
	@echo "$(GREEN)Capturing CPU profile (30 seconds)...$(NC)"
	@curl -s http://localhost:6060/debug/pprof/profile?seconds=30 -o cpu.prof
	@go tool pprof -http=:8081 cpu.prof

pprof-goroutine: ## Capture and analyze goroutine profile
	@echo "$(GREEN)Capturing goroutine profile...$(NC)"
	@curl -s http://localhost:6060/debug/pprof/goroutine -o goroutine.prof
	@go tool pprof -http=:8081 goroutine.prof

pprof-allocs: ## Capture and analyze allocation profile
	@echo "$(GREEN)Capturing allocation profile...$(NC)"
	@curl -s http://localhost:6060/debug/pprof/allocs -o allocs.prof
	@go tool pprof -http=:8081 allocs.prof

# Development
fmt: ## Format Go code
	@echo "$(GREEN)Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

lint: ## Run linter (requires golangci-lint)
	@echo "$(GREEN)Running linter...$(NC)"
	@golangci-lint run ./... || echo "$(YELLOW)Install golangci-lint: make install-tools$(NC)"

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	@go vet ./...
	@echo "$(GREEN)✓ Vet completed$(NC)"

clean: ## Clean build artifacts and logs
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out coverage.html
	@rm -f *.prof
	@rm -f /tmp/notification-service.log
	@rm -f /tmp/job-service.log
	@rm -f /tmp/connections-service.log
	@rm -f /tmp/followers-service.log
	@echo "$(GREEN)✓ Clean completed$(NC)"

clean-all: clean ## Clean everything (artifacts + Docker volumes)
	@echo "$(YELLOW)Stopping services and removing volumes...$(NC)"
	@docker compose down -v
	@echo "$(GREEN)✓ Everything cleaned$(NC)"

install-tools: ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "$(GREEN)✓ Tools installed$(NC)"

# Quick Access URLs
urls: ## Show all service URLs
	@echo "$(GREEN)Service URLs:$(NC)"
	@echo "  Health Check:        http://localhost:8080/health"
	@echo "  SSE Stream:          http://localhost:8080/notifications/stream?user_id=user_1"
	@echo "  Metrics:             http://localhost:8080/metrics"
	@echo "  pprof:               http://localhost:6060/debug/pprof/"

# ╔═══════════════════════════════════════════════════════════════════╗
# ║                    SSE BENCHMARK COMMANDS                         ║
# ╚═══════════════════════════════════════════════════════════════════╝

sse-bench: build-sse-bench ## Run SSE benchmark (default: 1000 users, 5min)
	@echo "$(GREEN)🚀 Starting SSE benchmark...$(NC)"
	@echo "  Users: 1000, Duration: 5min, Report Interval: 10s"
	@./$(BINARY_DIR)/sse-bench \
		-server=http://localhost:8080 \
		-users=1000 \
		-duration=5m \
		-report=10s \
		-reconnect=true \
		-ramp-up=10s \
		-log=info

sse-bench-quick: build-sse-bench ## Quick SSE benchmark (100 users, 1min)
	@echo "$(GREEN)🚀 Running quick SSE benchmark...$(NC)"
	@echo "  Users: 100, Duration: 1min"
	@./$(BINARY_DIR)/sse-bench \
		-server=http://localhost:8080 \
		-users=100 \
		-duration=1m \
		-report=10s \
		-reconnect=true \
		-ramp-up=5s \
		-log=info

sse-bench-stress: build-sse-bench ## Stress test (10k users, 10min)
	@echo "$(GREEN)🚀 Starting SSE stress test...$(NC)"
	@echo "  Users: 10,000, Duration: 10min"
	@./$(BINARY_DIR)/sse-bench \
		-server=http://localhost:8080 \
		-users=10000 \
		-duration=10m \
		-report=10s \
		-reconnect=true \
		-ramp-up=30s \
		-log=warn

sse-bench-custom: build-sse-bench ## Custom SSE benchmark (use USERS, DURATION, SERVER vars)
	@echo "$(GREEN)🚀 Starting custom SSE benchmark...$(NC)"
	@./$(BINARY_DIR)/sse-bench \
		-server=$(or $(SERVER),http://localhost:8080) \
		-users=$(or $(USERS),1000) \
		-duration=$(or $(DURATION),5m) \
		-report=$(or $(REPORT),10s) \
		-reconnect=true \
		-ramp-up=$(or $(RAMPUP),10s) \
		-log=$(or $(LOG),info)

sse-bench-debug: build-sse-bench ## Debug SSE benchmark (10 users, verbose logging)
	@echo "$(GREEN)🚀 Running SSE benchmark in debug mode...$(NC)"
	@./$(BINARY_DIR)/sse-bench \
		-server=http://localhost:8080 \
		-users=10 \
		-duration=1m \
		-report=5s \
		-reconnect=true \
		-ramp-up=2s \
		-log=debug \
		-detailed=true
	@echo "  pprof Heap:          http://localhost:6060/debug/pprof/heap"
	@echo "  pprof Goroutines:    http://localhost:6060/debug/pprof/goroutine"

.DEFAULT_GOAL := help
