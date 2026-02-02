I recently experimented with notification delivery systems using Go, PostgreSQL, and Kafka. Built a real-time event processor to stress-test architecture decisions at scale.

The Problem:
Initial implementation with ClickHouse at 1,200 events/sec = system collapse
- 125 writes/sec throughput
- 36-449 deliveries/sec (unreliable)
- 560% CPU usage
- 70-96% throughput loss

The Root Causes:
ClickHouse (OLAP database) was the wrong tool. It performs table-level locks on UPDATE operations. Our workload needed row-level locking and high-frequency small transactions - classic OLTP patterns. We were optimizing code when we should have optimized architecture.

The Optimizations:
1. Database migration - Switched to PostgreSQL with FOR UPDATE SKIP LOCKED for distributed task claiming
2. Consumer batch processing - 100 records/50ms timeout instead of single-message writes (100x fewer round-trips)
3. Worker pool tuning - 5→10 picker workers (100ms poll), 20→50 delivery workers
4. Connection pooling - Prepared statements with 50 max open connections

The Results at 1,000 concurrent SSE connections:
✅ 1,100 writes/sec (8.8x improvement)
✅ 10,086 notifications/sec peak delivery (22x improvement)
✅ 0.65ms P50 latency (8.5x faster)
✅ 60% CPU usage (9x reduction)
✅ 100% reliability over 18+ minutes continuous load

Key Takeaway:
Database selection is architecture. The biggest performance gain came from choosing the right database for the workload, not from clever code optimizations. OLTP vs OLAP matters.

GitHub: [Add your repo link]

#DistributedSystems #SystemDesign #Golang #PostgreSQL #Kafka #Performance #SoftwareArchitecture
