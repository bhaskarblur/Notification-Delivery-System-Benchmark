# Distributed Notification Processing Design

## Problem Statement
Multiple notification service replicas must:
1. Pick notifications without conflicts (no duplicate processing)
2. Process by priority order (HIGH → MEDIUM → LOW)
3. Handle service crashes gracefully (allow re-picking)
4. Guarantee delivery reliability

## Solution: Lease-Based Distributed Locking

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Kafka Topic: notifications                │
│                     (event ingestion)                        │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│              Notification Service (Consumer)                 │
│  - Reads from Kafka                                         │
│  - Writes to DB with status='not_pushed'                    │
│  - No delivery logic (just persistence)                     │
└─────────────────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│              ClickHouse: notifications table                 │
│  Columns:                                                   │
│  - status: not_pushed | processing | pushed | failed        │
│  - priority: HIGH | MEDIUM | LOW                            │
│  - processing_by: instance_id (UUID)                        │
│  - processing_started_at: timestamp                         │
│  - processing_lease_expires_at: timestamp                   │
│  - retry_count: int                                         │
└─────────────────────┬───────────────────────────────────────┘
                      │
          ┌───────────┴───────────┬───────────────────┐
          ▼                       ▼                   ▼
┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
│ Task Picker #1   │   │ Task Picker #2   │   │ Task Picker #3   │
│ Instance: uuid-1 │   │ Instance: uuid-2 │   │ Instance: uuid-3 │
└──────────────────┘   └──────────────────┘   └──────────────────┘
     │                        │                       │
     └────────────────────────┴───────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │  SSE Delivery    │
                    │  + Status Update │
                    └──────────────────┘
```

### State Machine

```
not_pushed ──┐
             │ [Task Picker claims with lease]
             ▼
        processing ──┬─[Delivered successfully]──> pushed ──> delivered
                     │
                     ├─[Delivery failed]─────────> failed
                     │
                     └─[Lease expired]──────────> not_pushed (retry_count++)
                              │
                              └─[retry_count >= 3]──> failed
```

### Lease-Based Locking Algorithm

#### 1. **Claim Notifications (Atomic Operation)**

```sql
-- Task Picker polls with priority ordering
-- Claims up to BATCH_SIZE notifications atomically

UPDATE notifications.notifications
SET 
    status = 'processing',
    processing_by = '{instance_id}',
    processing_started_at = now(),
    processing_lease_expires_at = now() + INTERVAL 30 SECOND,
    updated_at = now()
WHERE notification_id IN (
    SELECT notification_id
    FROM notifications.notifications
    WHERE (
        -- Pick notifications that are available
        status = 'not_pushed' 
        OR 
        -- OR pick notifications with expired lease (crashed service)
        (status = 'processing' AND processing_lease_expires_at < now())
    )
    AND retry_count < 3  -- Max 3 retry attempts
    ORDER BY 
        priority ASC,  -- HIGH=1, MEDIUM=2, LOW=3 (ascending = HIGH first)
        notification_received_timestamp ASC  -- FIFO within priority
    LIMIT 100  -- Batch size
)
RETURNING notification_id, user_id, event_type, priority, payload;
```

**Key Points:**
- ClickHouse `ALTER UPDATE` with WHERE + LIMIT ensures **atomic claim**
- Only ONE service instance can claim a specific notification
- Expired leases (crashed services) are automatically re-claimable
- Priority ordering: HIGH → MEDIUM → LOW

#### 2. **Process & Deliver**

```go
// Task Picker processing loop
func (tp *TaskPicker) Run(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)  // Poll every 1s
    
    for {
        select {
        case <-ticker.C:
            // Claim batch
            notifications, err := tp.repo.ClaimNotifications(ctx, tp.instanceID, 100)
            if err != nil {
                tp.logger.Error("claim failed", zap.Error(err))
                continue
            }
            
            if len(notifications) == 0 {
                continue  // No work available
            }
            
            // Process batch concurrently
            var wg sync.WaitGroup
            for _, notif := range notifications {
                wg.Add(1)
                go func(n *Notification) {
                    defer wg.Done()
                    tp.processNotification(ctx, n)
                }(notif)
            }
            wg.Wait()
            
        case <-ctx.Done():
            return
        }
    }
}

func (tp *TaskPicker) processNotification(ctx context.Context, notif *Notification) {
    // Attempt SSE delivery
    err := tp.sseManager.Send(notif.UserID, notif)
    
    if err != nil {
        // Delivery failed - mark as failed or retry
        tp.repo.UpdateStatus(ctx, notif.NotificationID, "failed", tp.instanceID)
        tp.logger.Error("delivery failed", 
            zap.String("notification_id", notif.NotificationID.String()),
            zap.Error(err))
        return
    }
    
    // Success - mark as pushed
    tp.repo.UpdateStatus(ctx, notif.NotificationID, "pushed", tp.instanceID)
    tp.logger.Info("delivered", 
        zap.String("notification_id", notif.NotificationID.String()),
        zap.String("user_id", notif.UserID))
}
```

#### 3. **Status Update (Verify Ownership)**

```sql
-- Update status only if still owned by this instance
UPDATE notifications.notifications
SET 
    status = '{new_status}',  -- 'pushed' or 'failed'
    notification_pushed_timestamp = CASE WHEN '{new_status}' = 'pushed' THEN now() ELSE notification_pushed_timestamp END,
    processing_latency_ms = CASE WHEN '{new_status}' = 'pushed' THEN 
        dateDiff('millisecond', notification_received_timestamp, now()) 
    ELSE processing_latency_ms END,
    retry_count = CASE WHEN '{new_status}' = 'failed' THEN retry_count + 1 ELSE retry_count END,
    updated_at = now()
WHERE notification_id = '{notification_id}'
  AND processing_by = '{instance_id}'  -- Verify ownership
  AND status = 'processing';  -- Verify still in processing state
```

**Safety Check:**
- Only updates if `processing_by` matches (prevents other instances from updating)
- If lease expired and another instance claimed it, this UPDATE affects 0 rows (ignored)

#### 4. **Lease Expiry Handler (Background Job)**

```go
// Runs every 10 seconds to reset expired leases
func (tp *TaskPicker) LeaseExpiryHandler(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    
    for {
        select {
        case <-ticker.C:
            affected, err := tp.repo.ResetExpiredLeases(ctx)
            if err != nil {
                tp.logger.Error("lease reset failed", zap.Error(err))
                continue
            }
            
            if affected > 0 {
                tp.logger.Warn("reset expired leases", 
                    zap.Int("count", affected))
            }
            
        case <-ctx.Done():
            return
        }
    }
}

// SQL
UPDATE notifications.notifications
SET 
    status = 'not_pushed',
    processing_by = NULL,
    processing_started_at = NULL,
    processing_lease_expires_at = NULL,
    retry_count = retry_count + 1,
    updated_at = now()
WHERE status = 'processing'
  AND processing_lease_expires_at < now()
  AND retry_count < 3;
```

### Guarantees

✅ **No Duplicate Processing**: Atomic UPDATE with WHERE ensures only one instance claims  
✅ **Crash Recovery**: Expired leases automatically released after 30s  
✅ **Priority Ordering**: ORDER BY priority ASC in claim query  
✅ **Retry Limit**: Max 3 attempts, then marked as failed  
✅ **Idempotency**: Status updates verify ownership before applying  

### Configuration

```yaml
task_picker:
  instance_id: "auto-generate-uuid"  # Unique per service instance
  poll_interval: 1s                   # How often to claim work
  batch_size: 100                     # Notifications per claim
  lease_duration: 30s                 # Lease timeout
  max_retries: 3                      # Max retry attempts
  lease_cleanup_interval: 10s         # How often to reset expired leases
```

### Scaling

- **Horizontal**: Add more service replicas (each with unique instance_id)
- **Throughput**: Increase batch_size (100-1000)
- **Priority Tuning**: Adjust polling intervals per priority tier
- **Lease Duration**: 
  - Shorter (10s) = faster recovery but more overhead
  - Longer (60s) = less overhead but slower recovery

### ClickHouse Optimizations

```sql
-- Index for claim query
INDEX idx_claim (status, processing_lease_expires_at, retry_count, priority, notification_received_timestamp) TYPE minmax GRANULARITY 1

-- Partition by date for efficient cleanup
PARTITION BY toYYYYMMDD(notification_received_timestamp)

-- TTL for automatic cleanup
TTL created_at + INTERVAL 30 DAY
```

---

## Example Scenario

**3 Service Instances Running:**

1. Instance `A` claims 100 HIGH priority notifications at `T=0`
2. Instance `B` claims next 100 HIGH priority notifications at `T=0.5s`
3. Instance `C` finds no HIGH priority work, claims MEDIUM at `T=1s`

**Instance A crashes at T=15s:**
- 100 notifications stuck in `processing` state
- Lease expires at `T=30s`
- At `T=31s`, Instance `B` or `C` can reclaim them via expired lease query

**Result:**
- Total latency: ~31s for crashed batch (acceptable for reliability)
- No data loss
- Automatic recovery
- Priority maintained

---

This design provides **exactly-once delivery semantics** with **at-least-once guarantees** and **priority-based processing** in a distributed environment.
