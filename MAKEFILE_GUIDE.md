# Makefile Guide - Notification Delivery System

## 🚀 Quick Start

The entire system is now managed through the Makefile. No need to remember individual scripts!

### First Time Setup
```bash
make setup          # Complete setup: init + build + start infra + init DB
make start          # Start all services
```

### Daily Usage
```bash
make start          # Start everything (default: 10 events/sec, 100 users)
make monitor        # Live monitoring dashboard
make stop           # Stop all services
```

### Heavy Load Testing
```bash
make start EVENT_RATE=50 NUM_USERS=500
```

---

## 📋 All Commands

### Core Operations

| Command | Description |
|---------|-------------|
| `make help` | Show all available commands with descriptions |
| `make setup` | Complete setup (init + build + infra + DB) |
| `make start` | Start all services (use EVENT_RATE and NUM_USERS vars) |
| `make stop` | Stop all services |
| `make status` | Show running services |

### Build Commands

| Command | Description |
|---------|-------------|
| `make build` | Build all services |
| `make build-notification` | Build notification service only |
| `make build-producers` | Build all producer services |

### Infrastructure Management

| Command | Description |
|---------|-------------|
| `make infra-start` | Start Docker infrastructure (Kafka, ClickHouse) |
| `make db-init` | Initialize ClickHouse database schema |
| `make start-notification` | Start notification service only |
| `make start-producers` | Start producer services only |

### Monitoring & Debugging

| Command | Description |
|---------|-------------|
| `make monitor` | Live monitoring dashboard (single view) |
| `make monitor-loop` | Auto-refreshing monitor (every 5 seconds) |
| `make logs` | View recent logs from all services |
| `make logs-follow` | Follow notification service logs in real-time |
| `make analyze` | Analyze notification delays with percentiles |
| `make status` | Show service status (Docker + Go processes) |
| `make urls` | Show all service URLs |

### Kafka Operations

| Command | Description |
|---------|-------------|
| `make kafka-topics` | List all Kafka topics |
| `make kafka-create-topic` | Create notifications topic (if needed) |
| `make kafka-consumer-groups` | Show consumer groups |
| `make kafka-lag` | Show consumer group lag |

### ClickHouse Operations

| Command | Description |
|---------|-------------|
| `make clickhouse-cli` | Open ClickHouse CLI |
| `make clickhouse-tables` | Show all tables |
| `make clickhouse-stats` | Show notification statistics |
| `make clickhouse-query QUERY="..."` | Run custom query |
| `make clickhouse-reset` | Reset database (⚠️ deletes all data) |

### Performance Profiling

| Command | Description |
|---------|-------------|
| `make pprof` | Open pprof UI in browser |
| `make pprof-heap` | Capture and analyze heap profile |
| `make pprof-cpu` | Capture CPU profile (30 seconds) |
| `make pprof-goroutine` | Analyze goroutines |
| `make pprof-allocs` | Analyze memory allocations |

### Testing & Development

| Command | Description |
|---------|-------------|
| `make test` | Run unit tests with race detector |
| `make test-integration` | Run integration tests |
| `make test-coverage` | Generate coverage report (coverage.html) |
| `make fmt` | Format Go code |
| `make lint` | Run linter (requires golangci-lint) |
| `make vet` | Run go vet |

### Cleanup

| Command | Description |
|---------|-------------|
| `make clean` | Clean build artifacts and logs |
| `make clean-all` | Clean everything (+ Docker volumes) |

### Tools

| Command | Description |
|---------|-------------|
| `make install-tools` | Install development tools (golangci-lint) |

---

## 🎯 Common Workflows

### 1. Fresh Start
```bash
# First time setup
make setup
make start

# Monitor system
make monitor

# When done
make stop
```

### 2. Development Workflow
```bash
# Make code changes
make fmt
make vet
make test

# Rebuild and restart
make stop
make build
make start
```

### 3. Heavy Load Testing
```bash
# Start with high load
make start EVENT_RATE=100 NUM_USERS=1000

# Monitor in another terminal
make monitor-loop

# Analyze performance
make analyze
make pprof-cpu
```

### 4. Debugging Issues
```bash
# Check service status
make status

# View logs
make logs

# Follow live logs
make logs-follow

# Check ClickHouse data
make clickhouse-stats
make clickhouse-cli

# Check Kafka lag
make kafka-lag
```

### 5. Performance Analysis
```bash
# Start system
make start

# Generate load
make start-producers EVENT_RATE=50 NUM_USERS=500

# Capture profiles
make pprof-cpu      # CPU profiling
make pprof-heap     # Memory profiling
make pprof-goroutine # Goroutine analysis

# Analyze delays
make analyze
```

---

## 🔧 Configuration

### Environment Variables

The Makefile supports these configuration variables:

- `EVENT_RATE`: Events per second per producer (default: 10)
- `NUM_USERS`: Number of simulated users (default: 100)
- `QUERY`: Custom ClickHouse query for `clickhouse-query` command

Examples:
```bash
# Custom configuration
make start EVENT_RATE=50 NUM_USERS=500

# Run custom query
make clickhouse-query QUERY="SELECT COUNT(*) FROM notifications WHERE priority=1"
```

### Service Endpoints

After running `make start`, services are available at:

- **Health Check**: http://localhost:8080/health
- **SSE Stream**: http://localhost:8080/notifications/stream?user_id=user_1
- **Metrics**: http://localhost:8080/metrics
- **pprof**: http://localhost:6060/debug/pprof/

View all URLs:
```bash
make urls
```

---

## 📊 Monitoring Examples

### Live Dashboard
```bash
make monitor-loop
```

Shows:
- Docker container status
- Go service processes (CPU, Memory)
- Kafka message counts
- ClickHouse notification counts
- Priority distribution

### Delay Analysis
```bash
make analyze
```

Shows per-priority:
- Total notifications
- Average delay
- P50, P95, P99 percentiles
- Maximum delay

### Database Statistics
```bash
make clickhouse-stats
```

Shows per-priority:
- Total count
- Status breakdown (not_pushed, processing, pushed, delivered, failed)
- Average delay

---

## 🐛 Troubleshooting

### Services Not Starting

```bash
# Check what's running
make status

# Check infrastructure
docker ps

# View logs
make logs

# Restart from scratch
make stop
make clean-all
make setup
make start
```

### High Consumer Lag

```bash
# Check lag
make kafka-lag

# Analyze bottlenecks
make pprof-cpu
make pprof-goroutine

# Check ClickHouse performance
make clickhouse-stats
```

### Database Issues

```bash
# Check tables
make clickhouse-tables

# View data
make clickhouse-cli

# Reset (⚠️ deletes all data)
make clickhouse-reset
```

---

## 📝 Notes

### Simplified Architecture

All functionality previously in individual shell scripts (`start-all.sh`, `monitor.sh`, etc.) is now integrated into the Makefile for:

- **Easier discovery**: `make help` shows all commands
- **Consistent interface**: One tool for everything
- **Better maintainability**: Single source of truth
- **IDE integration**: Most IDEs support Makefile targets

### Legacy Scripts

Old scripts are preserved in `scripts/legacy/` if you need to reference them, but the Makefile is now the recommended way to interact with the system.

### Color Output

The Makefile uses ANSI colors for better readability:
- 🟢 Green: Success messages
- 🟡 Yellow: Warnings
- 🔵 Blue: Section headers
- 🔴 Red: Errors or destructive operations

---

## 🎓 Learning Resources

### Understanding the System

1. Read [ARCHITECTURE_SUMMARY.md](ARCHITECTURE_SUMMARY.md) for high-level design
2. Read [DISTRIBUTED_PROCESSING_DESIGN.md](DISTRIBUTED_PROCESSING_DESIGN.md) for distributed locking
3. Read [DUAL_WORKER_POOL_ARCHITECTURE.md](DUAL_WORKER_POOL_ARCHITECTURE.md) for worker pool details

### Hands-on Learning

```bash
# 1. Start system and watch it work
make start
make monitor-loop

# 2. Increase load gradually
make stop
make start EVENT_RATE=20 NUM_USERS=200
make start EVENT_RATE=50 NUM_USERS=500

# 3. Analyze performance at each level
make analyze
make pprof-cpu
```

---

## 🎉 Summary

**One command to rule them all:**

```bash
make setup    # First time only
make start    # Daily usage
make monitor  # Watch it work
make stop     # When done
```

All scripts have been simplified into the Makefile. Use `make help` to see all available commands.
