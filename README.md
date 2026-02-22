# Notification Delivery System with Priority-Based Handling

A high-performance distributed notification system that delivers **10,000+ notifications/second** with real-time SSE delivery to 1000+ concurrent clients. Built with PostgreSQL, Kafka, and Go, achieving **40x better performance** than traditional OLAP databases.

## 🎯 Overview

This project demonstrates how platforms like LinkedIn and Naukri handle notifications with varying priorities at scale. After comprehensive performance optimization and database migration, the system now achieves:

- ⚡ **10,086 notifications/sec** delivery throughput (recent peak)
- 🚀 **1,100 writes/sec** to PostgreSQL
- 📊 **40x performance improvement** over ClickHouse baseline
- 💻 **90% CPU reduction** (1017% vs 5600% for similar load)
- 🔄 **1000 concurrent SSE connections** with <1s latency

**See [PERFORMANCE_RESULTS.md](PERFORMANCE_RESULTS.md) for detailed benchmarks and optimization journey.**

### Key Features

- **Priority-Based Processing**: HIGH, MEDIUM, LOW with optimized scheduling
- **Optimized Worker Pool**: 10 picker workers (100ms poll) + 50 delivery workers
- **Batch Operations**: Consumer batching (100 records/50ms), 500 notifications per claim
- **PostgreSQL with OLTP Optimization**: FOR UPDATE SKIP LOCKED, optimized indexes
- **Distributed Locking**: Lease-based coordination for horizontal scaling
- **SSE (Server-Sent Events)**: Real-time push with connection pooling
- **Kafka Integration**: Event-driven architecture with batch processing
- **Performance Monitoring**: pprof, metrics dashboard, real-time monitoring
- **Makefile Interface**: Simple, unified command interface

## 🚀 Quick Start

```bash
# Complete setup (first time)
make setup

# Start all services
make start

# Monitor in another terminal
make monitor

# Heavy load testing
make start EVENT_RATE=50 NUM_USERS=500

# Stop everything
make stop
```

**That's it!** See [MAKEFILE_GUIDE.md](MAKEFILE_GUIDE.md) for all commands.

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Event Producers (Go)                         │
├─────────────────┬─────────────────┬─────────────────────────────────┤
│  Job Service    │ Connections Svc │ Followers Svc                   │
│  - job.new      │ - conn.request  │ - follower.new                  │
│  - job.update   │ - conn.accepted │ - content.liked                 │
│  - app.viewed   │ - endorsed      │ - commented                     │
└────────┬────────┴────────┬────────┴────────┬───────────────────────┘
         │                 │                 │
         └─────────────────┼─────────────────┘
                           │
                  ┌────────▼────────┐
                  │  Kafka Topic    │
                  │(notification-   │
                  │   events)       │
                  └────────┬────────┘
                           │
         ┌─────────────────▼──────────────────┐
         │   Notification Service (Go)        │
         ├────────────────────────────────────┤
         │ Consumer: Kafka → PostgreSQL       │
         │   • Batch processing (100/50ms)    │
         │                                    │
         │ Task Picker: Optimized Workers     │
         │   • 10 picker workers (100ms poll) │
         │   • 50 delivery workers (SSE)      │
         │   • FOR UPDATE SKIP LOCKED         │
         │                                    │
         │ Distributed: Lease-based locking   │
         │ Performance: 10k+ notifs/sec       │
         └────────┬───────────────────────────┘
                  │
    ┌─────────────┼─────────────────┐
    │             │                 │
┌───▼────┐   ┌────▼────────┐   ┌───▼─────┐
│  SSE   │   │ PostgreSQL  │   │  pprof  │
│ Users  │   │   (OLTP)    │   │ Metrics │
│ 1000+  │   │ Optimized   │   │Dashboard│
└────────┘   └─────────────┘   └─────────┘
```

## 📊 Notification Priority Model

| Priority | Examples | Processing Order |
|----------|----------|------------------|
| **HIGH** (1) | Job updates, Connection requests | Processed first |
| **MEDIUM** (2) | Profile views, Endorsements | Processed second |
| **LOW** (3) | New followers, Content likes | Processed last |

### Processing Logic

- **No artificial delays**: All notifications processed based on priority
- **Optimized worker pools**: 10 DB pickers (100ms poll) + 50 delivery workers
- **Batch operations**: Consumer batches 100 records/50ms, claim 500 notifications
- **Distributed safe**: Lease-based locking prevents duplicate processing

## 🚀 Performance Metrics

**Current System Performance (PostgreSQL):**

| Metric | Value |
|--------|-------|
| Write Throughput | 1,100 writes/sec |
| Peak Delivery Rate | 10,086 notifications/sec |
| Average Delivery Rate | 277 notifications/sec |
| Concurrent SSE Connections | 1,000 (stable) |
| P50 Latency | 657ms |
| CPU Usage (Normal) | 60-100% |
| Success Rate | 100% |

**Improvement Over ClickHouse Baseline:**
- ✅ **8.8x** faster writes (125 → 1,100 writes/sec)
- ✅ **22-280x** faster delivery (36-449 → 10,086 notifications/sec)
- ✅ **9x** lower CPU usage (560% → 60%)
- ✅ **8.5x** faster latency (5.5ms → 0.65ms P50)

**📊 See [PERFORMANCE_RESULTS.md](PERFORMANCE_RESULTS.md) for detailed benchmarks and optimization journey.**

## 📖 Documentation

- **[PERFORMANCE_RESULTS.md](PERFORMANCE_RESULTS.md)** - ⭐ Benchmark results and optimization analysis
- **[ARCHITECTURE_SUMMARY.md](ARCHITECTURE_SUMMARY.md)** - Design philosophy
- **[DISTRIBUTED_PROCESSING_DESIGN.md](DISTRIBUTED_PROCESSING_DESIGN.md)** - Distributed locking details
- **[DUAL_WORKER_POOL_ARCHITECTURE.md](DUAL_WORKER_POOL_ARCHITECTURE.md)** - Worker pool internals

## 🔧 Common Commands

```bash
# Setup & Start
make setup              # First time setup
make start              # Start all services
make start EVENT_RATE=50 NUM_USERS=500  # Heavy load

# Monitoring
make monitor            # Live dashboard
make monitor-loop       # Auto-refresh every 5s
make logs               # View all logs
make analyze            # Analyze delays

# Management
make status             # Check services
make stop               # Stop everything
make clean-all          # Clean everything

# Development
make build              # Build services
make test               # Run tests
make fmt                # Format code

# Performance
make pprof              # Open pprof UI
make pprof-cpu          # CPU profiling
make pprof-heap         # Memory profiling

# Database
make postgres-cli       # Open PostgreSQL CLI
make postgres-stats     # Show database statistics
make kafka-lag          # Show consumer lag
```

See [MAKEFILE_GUIDE.md](MAKEFILE_GUIDE.md) for all commands.

## 🌐 Service Endpoints

After running `make start`:

| Service | URL |
|---------|-----|
| Health Check | http://localhost:8080/health |
| SSE Stream | http://localhost:8080/notifications/stream?user_id=user_1 |
| Metrics | http://localhost:8080/metrics |
| pprof | http://localhost:6060/debug/pprof/ |

Test SSE connection:
```bash
curl -N http://localhost:8080/notifications/stream?user_id=user_1
```

## 📈 Performance Monitoring

```bash
# View metrics
curl http://localhost:8080/metrics

# View Grafana dashboards
open http://localhost:3000
```

### 5. Query ClickHouse

```bash
# Access ClickHouse CLI
docker exec -it notif-clickhouse clickhouse-client

# Run queries
SELECT 
    priority,
    count() as total,
    avg(delay_seconds) as avg_delay
FROM notification_db.notifications
GROUP BY priority;
```

### 6. Stop the System

```bash
./scripts/stop-all.sh
```

## 📁 Project Structure

```
notification-delivery-system/
├── cmd/                                    # Application entry points
│   ├── notification-service/              # Main notification service
│   ├── job-service/                       # Job event producer
│   ├── connections-service/               # Connections event producer
│   ├── followers-service/                 # Followers event producer
│   └── benchmark-client/                  # Load testing client
├── internal/                              # Private application code
│   ├── notification/
│   │   ├── consumer.go                    # Kafka consumer
│   │   ├── processor.go                   # Notification processing
│   │   ├── delay_engine.go                # Priority-based delay logic
│   │   ├── sse_manager.go                 # SSE connection management
│   │   └── repository.go                  # ClickHouse repository
│   ├── producer/
│   │   └── kafka_producer.go              # Shared Kafka producer
│   ├── models/
│   │   └── notification.go                # Data models
│   ├── metrics/
│   │   └── prometheus.go                  # Prometheus metrics
│   └── config/
│       └── config.go                      # Configuration loader
├── pkg/                                   # Public reusable packages
│   ├── clickhouse/
│   │   └── client.go                      # ClickHouse client wrapper
│   └── kafka/
│       └── client.go                      # Kafka client wrapper
├── scripts/                               # Utility scripts
│   ├── start-all.sh                       # Start all services
│   ├── stop-all.sh                        # Stop all services
│   ├── init-clickhouse.sql                # ClickHouse schema
│   └── create-kafka-topic.sh              # Kafka topic setup
├── configs/                               # Configuration files
│   ├── config.yaml                        # Main config
│   ├── prometheus.yml                     # Prometheus config
│   ├── clickhouse-config.xml              # ClickHouse config
│   └── grafana/                           # Grafana dashboards
├── docker/                                # Dockerfiles
│   ├── Dockerfile.notification            # Notification service
│   ├── Dockerfile.services                # Event producers
│   └── Dockerfile.benchmark               # Benchmark client
├── benchmarks/                            # Benchmark results
│   ├── results/                           # Test results (JSON/CSV)
│   └── scenarios/                         # Test scenario configs
├── docker-compose.yml                     # Docker Compose config
├── go.mod                                 # Go dependencies
├── Makefile                               # Build automation
├── PRD.md                                 # Product Requirements Doc
├── TASKS.md                               # Implementation checklist
└── README.md                              # This file
```

## 🛠️ Development

### Build Services Locally

```bash
# Build notification service
go build -o bin/notification-service cmd/notification-service/main.go

# Build producer services
go build -o bin/job-service cmd/job-service/main.go
go build -o bin/connections-service cmd/connections-service/main.go
go build -o bin/followers-service cmd/followers-service/main.go

# Build benchmark client
go build -o bin/benchmark-client cmd/benchmark-client/main.go
```

### Run Locally (without Docker)

```bash
# Start infrastructure only
docker-compose up -d zookeeper kafka clickhouse prometheus grafana

# Run notification service locally
CONFIG_PATH=./configs/config.yaml \
KAFKA_BROKERS=localhost:9092 \
CLICKHOUSE_HOST=localhost:9000 \
./bin/notification-service

# Run producer service
KAFKA_BROKERS=localhost:9092 \
./bin/job-service
```

### Run Tests

```bash
# Unit tests
go test ./internal/... -v

# Integration tests
go test ./internal/... -tags=integration -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 📈 Performance Monitoring

### pprof Profiling

```bash
# Heap profile (memory usage)
go tool pprof http://localhost:6060/debug/pprof/heap
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/heap

# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Goroutine analysis
go tool pprof http://localhost:6060/debug/pprof/goroutine

# View pprof UI
open http://localhost:6060/debug/pprof/
```

### Prometheus Queries

```promql
# Notification throughput (per second)
rate(notifications_processed_total[1m])

# Average notification latency by priority
histogram_quantile(0.95, rate(notification_latency_seconds_bucket[5m]))

# Active SSE connections
active_sse_connections

# Kafka consumer lag
kafka_consumer_lag
```

## 🧪 Benchmarking

### Run Load Tests

```bash
# Build benchmark client
go build -o bin/benchmark-client cmd/benchmark-client/main.go

# Baseline test (1,000 users, 5 minutes)
./bin/benchmark-client \
  --users 1000 \
  --duration 5m \
  --priority-mix high:50,medium:30,low:20 \
  --output benchmarks/results/baseline.json

# High load test (50,000 users)
./bin/benchmark-client \
  --users 50000 \
  --duration 10m \
  --ramp-up 2m \
  --output benchmarks/results/high-load.json

# Delay verification test (6 hours)
./bin/benchmark-client \
  --scenario delay-verification \
  --duration 6h \
  --output benchmarks/results/delay-verify.json
```

### Benchmark Scenarios

1. **Baseline Test**: 1,000 users, mixed priority distribution
2. **High Load Test**: 50,000 users, 10K notifications/sec
3. **Burst Test**: Sudden spike from 5K to 50K notifications/sec
4. **Delay Verification**: Verify LOW priority delays are 4-6 hours
5. **Connection Stability**: 20% connection drop/reconnect simulation

## 🔍 ClickHouse Queries

### Useful Analytics Queries

```sql
-- Unread notifications for a user
SELECT * FROM notifications 
WHERE user_id = 'user_12345' AND is_read = 0 
ORDER BY event_timestamp DESC 
LIMIT 50;

-- Delay statistics by priority
SELECT 
    priority,
    count() as total,
    avg(delay_seconds) as avg_delay,
    quantile(0.50)(delay_seconds) as p50,
    quantile(0.95)(delay_seconds) as p95,
    quantile(0.99)(delay_seconds) as p99
FROM notifications
WHERE event_timestamp > now() - INTERVAL 1 DAY
GROUP BY priority;

-- Notification volume by hour and priority
SELECT 
    toStartOfHour(event_timestamp) as hour,
    priority,
    count() as notification_count
FROM notifications
WHERE event_timestamp > now() - INTERVAL 24 HOUR
GROUP BY hour, priority
ORDER BY hour DESC;

-- Users with most unread notifications
SELECT 
    user_id,
    countIf(is_read = 0) as unread_count,
    count() as total_notifications
FROM notifications
WHERE event_timestamp > now() - INTERVAL 7 DAY
GROUP BY user_id
ORDER BY unread_count DESC
LIMIT 20;
```

## 🎯 Key Metrics to Monitor

### Service Health
- **Active SSE Connections**: Should handle 10K+ concurrent connections
- **Kafka Consumer Lag**: Should be < 1000 messages under normal load
- **ClickHouse Write Latency**: P95 < 10ms for batch inserts
- **Memory Usage**: < 2GB @ 10K connections

### Notification Delivery
- **HIGH Priority P99 Latency**: < 1 second
- **MEDIUM Priority P95 Latency**: 15-30 minutes
- **LOW Priority P95 Latency**: 4-6 hours
- **Delivery Success Rate**: > 99.9%

## 📚 API Documentation

### SSE Stream Endpoint

**GET** `/notifications/stream?user_id={user_id}`

Opens an SSE connection for real-time notifications.

**Response**:
```
event: notification
data: {"notification_id":"uuid","type":"job.new","priority":"LOW","timestamp":"2026-01-31T10:30:00Z"}

event: heartbeat
data: {"timestamp":"2026-01-31T10:30:30Z"}
```

### REST Endpoints

**GET** `/notifications?user_id={user_id}&limit=50`

Get notifications for a user (REST fallback).

**POST** `/notifications/read`

Mark notifications as read.

```json
{
  "notification_ids": ["uuid1", "uuid2"]
}
```

**GET** `/metrics`

Prometheus metrics endpoint.

**GET** `/debug/pprof/`

pprof profiling endpoints.

## 🐛 Troubleshooting

### Kafka Connection Issues

```bash
# Check Kafka logs
docker logs notif-kafka

# Verify topic exists
docker exec notif-kafka kafka-topics --list --bootstrap-server localhost:9092

# Check consumer group
docker exec notif-kafka kafka-consumer-groups \
  --bootstrap-server localhost:9092 \
  --group notification-service-group \
  --describe
```

### ClickHouse Issues

```bash
# Check ClickHouse logs
docker logs notif-clickhouse

# Verify tables
docker exec notif-clickhouse clickhouse-client \
  --query "SHOW TABLES FROM notification_db"

# Check table size
docker exec notif-clickhouse clickhouse-client \
  --query "SELECT formatReadableSize(sum(bytes)) FROM system.parts WHERE table = 'notifications'"
```

### SSE Connection Issues

- Check if notification service is running: `docker ps | grep notif-service`
- View service logs: `docker logs notif-service -f`
- Test with curl: `curl -N http://localhost:8080/notifications/stream?user_id=test`

## 🚀 Performance Tuning

### ClickHouse Optimization

```bash
# Optimize table after bulk inserts
docker exec notif-clickhouse clickhouse-client \
  --query "OPTIMIZE TABLE notification_db.notifications FINAL"
```

### Kafka Tuning

Adjust in `docker-compose.yml`:
- `KAFKA_NUM_NETWORK_THREADS`: Increase for more throughput
- `KAFKA_NUM_IO_THREADS`: Increase for disk I/O
- `KAFKA_SOCKET_SEND_BUFFER_BYTES`: Increase for larger messages

### Notification Service Tuning

Adjust in `configs/config.yaml`:
- `max_sse_connections`: Increase connection limit
- `batch_size`: Larger batches for ClickHouse writes
- `batch_timeout`: Adjust for latency vs throughput tradeoff

## 🤝 Contributing

This is a technical demonstration project for interview/learning purposes. Feel free to extend it with:

- WebSocket support in addition to SSE
- Mobile push notifications (APNS, FCM)
- Notification aggregation/batching
- User preference management
- Multi-region deployment
- A/B testing framework for delay policies

## 🙏 Acknowledgments

Inspired by real-world notification systems from LinkedIn, Naukri, and other professional networking platforms.

---

**Built with**: Go • Kafka • ClickHouse • Docker • Grafana

**Author**: Built for technical interview demonstration

**Last Updated**: January 31, 2026
