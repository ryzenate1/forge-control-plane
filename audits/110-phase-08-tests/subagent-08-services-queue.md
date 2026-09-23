# Subagent 08 — Services Queue / Operation / EventStore Reverifikation

**Agent:** 08/20 Phase 08 — Services Queue/Operation/EventStore test files  
**Date:** 2026-08-24  
**Focus:** `queue/periodic.go:142`, `queue/store.go:113`, `eventstore/store.go:267`, `eventstore/outbox.go:39`, `queue/leader.go:23`

---

## 1. Inspection Findings

### 1.1 `forge/api/internal/services/queue/periodic.go:142` — RFC3339 deterministic

**Line 142** (actual file 163):
```go
idempotencyKey := fmt.Sprintf("periodic:%s:%s", job.id, scheduledFor.UTC().Format(time.RFC3339))
```

- **Uses `time.RFC3339` (no nanos), NOT `RFC3339Nano`.** Comment at 161–162 explicitly states: _"Use RFC3339 without nanos so minor truncation jitter never defeats the unique index."_
- **Deterministic grid:** `periodicIntervalSchedule.Next()` (58–73) truncates via `((ns/ivr)+1)*ivr` — epoch-aligned grid anchored at Unix epoch, so two replicas that boot 4s apart converge to same `scheduledFor`.
- **Tick path** (140): `scheduledFor := j.nextRun.UTC().Truncate(time.Second)` — truncates before formatting, ensuring replica-invariant key even with ticker jitter.
- **Verified:** `TestPeriodic_DeterministicGrid` and `TestPeriodic_RFC3339_NotNano` confirm `RFC3339` yields same key for `11:00:00.123456789` vs `11:00:00.987654321` same-second, while `RFC3339Nano` would diverge. Existing `periodic_dedup_test.go:48–63` already covers this; reverification test cross-checks.

**Verdict:** ✅ Correct. No regression — key is replica-invariant.

---

### 1.2 `forge/api/internal/services/queue/store.go:113` / `queue.go:303` — Cancel predicate

- **Store layer `store.go:113` `Fail()`** is unconditional `UPDATE ... SET status='failed'`. It intentionally has no predicate — the service layer gates.
- **Service layer `queue.go:303` `Cancel()`** implements the predicate guard (AF-3):
  ```go
  j, err := s.store.GetJob(ctx, id)
  switch j.Status {
  case JobStatusCompleted, JobStatusFailed, JobStatusCancelled:
      return fmt.Errorf("job %s already terminal (%s) — not cancelling", id, j.Status)
  }
  // then cancel active context + store.Fail with "job cancelled"
  ```
- Terminal states are **completed / failed / cancelled** (all `JobStatus*` constants defined at `queue.go:49–55`). `pending` and `running` are cancellable; `Cancel` cancels active worker context via `activeMu` map.
- **No store-level predicate needed** because `Fail` is also called by normal failure paths; the guard belongs to the explicit `Cancel` API, not to `Fail`/`Retry`.

**Verdict:** ✅ Predicate correctly at service layer; store `Fail` is not predicate-guarded (by design). Reverification tests confirm `TestCancel_NoRegressCompleted/Failed/Cancelled` all fail to flip terminal → failed.

---

### 1.3 `forge/api/internal/eventstore/store.go:267` — `PublishTx`

```go
func (s *EventStore) PublishTx(ctx context.Context, tx pgxTx, envelope events.Envelope) error {
    if tx == nil { return fmt.Errorf("eventstore PublishTx: tx is required (use Publish for non-tx path)") }
    // auto-gen ID/Timestamp if zero, Marshal payload, INSERT 12 columns in tx
}
```

- **Interface `pgxTx` (246–248):** minimal `Exec(ctx,sql,args...) (int64,error)` — caller must pass same `pgx.Tx` used for domain write; `Commit` after both succeed ⇒ atomic.
- **Column shape (285–289):** `INSERT INTO events (id, type, source, resource_type, resource_id, correlation_id, tenant_id, payload, created_at, dispatched, failure_count, last_error)` — 12 columns, `tenant_id` nullable via `any(nil)` when empty (sparse partial index).
- **Helpers:** `PublishTxBatch` (297–304) loops `PublishTx`; `PublishDurableTx` (314–320) flag-gated dual-write for `QUEUE_SINGLE_WRITER` migration (10 critical sites).
- **Tests created:** `TestPublishTx_Atomic` verifies single INSERT shape; `TestPublishTx_NilTxRejected`, `TestPublishTx_GeneratesIDAndTimestampWhenZero`, `TestPublishTx_InsertErrorPropagates`, `TestPublishTxBatch_Atomic` verify atomic all-or-nothing at tx level (mockTx).

**Verdict:** ✅ Correct transactional outbox pattern. Documented at `store.go:250–266` with example. No SQL drift vs `Publish` (same 12-column INSERT, one via `s.db.Exec`, one via `tx.Exec`).

---

### 1.4 `forge/api/internal/eventstore/outbox.go:39` — `Subscribe`

```go
func (r *Relay) Subscribe(handler func(context.Context, events.Envelope) error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.subscribers = append(r.subscribers, handler)
}
```

- **Three APIs:**
  - `Subscribe(handler)` — raw generic handler (line 39)
  - `SubscribeSubscriber(sub events.Subscriber)` — adapts `events.Subscriber` via `Handle(ctx,env)` (48–55), nil-safe
  - `SubscribeTyped(EventType, handler)` — filtered wrapper (61–68) checking `env.Type != eventType`
- **Zero-subscriber contract (AF-1):** `processEvent:192` and `deliverWithRetries:221` both guard `len(subs)==0` → `markFailedOrDeadLetter` with error `"relay has zero subscribers ... not marking dispatched"` — event NOT marked dispatched, failure_count++ so operators notice silent drop. `ClaimPending` lease `(batchSize+1)*eventTimeout = 330s` ensures crashed relay reclaimable.
- **Reverification:** `TestSubscribe_DurableRelay` verifies `SubscriberCount` increments via all 3 APIs and nil is no-op; `TestSubscribeTyped_Filters` verifies type filtering; `TestRelay_ZeroSubscriber_NotDispatched` verifies zero-sub leaves event not dispatched but failed.

**Verdict:** ✅ Durable relay contract correct. Zero-prod wiring is config error, not success.

---

### 1.5 `forge/api/queue/leader.go:23` — elector

**Note:** Task path `queue/leader.go:23` maps to `forge/api/queue/leader.go` (river-style driver), not `internal/services/queue` (which has no elector; leader gating is in `cmd/api/main.go:1252–1347` via `forge_leader` table).

**`forge/api/queue/leader.go:18–27` `Elector`:**
```go
type Elector struct {
    exec     Executor
    config   *LeaderConfig
    isLeader atomic.Bool
    leaderCh chan bool // buffered 1
    stopCh   chan struct{}
    ...
}
func NewElector(exec Executor, config *LeaderConfig) *Elector {
    if config.ElectInterval <= 0 { config.ElectInterval = 5*time.Second }
    if config.TTL <= 0 { config.TTL = 15*time.Second }
    ...
}
```

- **Loop:** `Start` goroutine `LeaderAttemptElect` → on success `notify(true)` + `runLeaderLoop` ticker `LeaderAttemptReelect` every `ElectInterval`; on error `isLeader.Store(false)` + `notify(false)` → retry after `ElectInterval`. Panic recovered with stack. `resign` called on `stopCh` via `LeaderResign`.
- **AF-2 gating (main.go):** Only leader runs reconciler, failover reaper, backup retention, cleanup reaper, periodic scheduler — `isLeader` metric `game_panel_api_forge_leader_is_leader` exposed at `http/server.go:1682`.

**Verdict:** ✅ Elector implements advisory-lock/TTL singleton correctly. `internal/services/queue.PeriodicJobScheduler` itself is not leader-gated — gating is at caller (main.go) which is intended.

---

## 2. Existing Test Inventory (pre-fix)

```
forge/api/internal/services/queue/*test.go
  - periodic_dedup_test.go  (TestPeriodicIntervalDeterministic, TestPeriodicIdempotencyKeyDeterministic, TestPeriodicNextMonotonic)
  - queue_test.go           (TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert, TestDispatchIdempotentUsesStableOperationID) + memoryStore harness
  - retry_accounting_test.go (TestDequeueDoesNotAccountRetries, TestRetryIsSoleRetryAccountant — F-18)

forge/api/internal/eventstore/*test.go
  - store_test.go           (TestPublish, TestPending*, TestMarkDispatched, TestOutboxPublisher*, TestRelay*)
  - store_tenant_test.go    (TestEventStore_TenantColumn_PublishAndRoundTrip, TestEventStore_TenantNull_BackwardsCompatible, TestEventStore_Migration_File_ContainsTenantColumn)
```

**Pre-fix failure:** `store_test.go:setupTestDB` created `events` without `tenant_id` column → `Publish` inserts with `tenant_id` failed with `table events has no column named tenant_id`. All `store_test.go` DB-backed tests failed. Fixed by adding `tenant_id TEXT` column and updating `newTestStore.pendingQuery` to include `COALESCE(tenant_id,'')` as 7th column to match `Pending` scan shape (see §4).

---

## 3. Created / Augmented Tests

### 3.1 `forge/api/internal/services/queue/queue_reverification_test.go` (new, 260 lines)

**Cancel predicate:**
- `TestCancel_NoRegressCompleted` — dispatches then `Acknowledge` → completed → `Cancel` must error `"already terminal (completed)"`, status stays `completed`.
- `TestCancel_NoRegressFailed` — `Fail("original failure")` → failed → `Cancel` must error, error string preserved (`"original failure"` not overwritten).
- `TestCancel_NoRegressCancelled` — pending → first `Cancel` succeeds (→ `failed:"job cancelled"`), second `Cancel` must error (terminal).
- `TestCancel_AcceptsPending` — pending → `Cancel` succeeds → `failed` + `"job cancelled"`.
- `TestCancel_AcceptsRunning` — `Dequeue` → running + inject `active[job.ID]` context → `Cancel` succeeds and cancels job context (`<-jobCtx.Done()`).
- `TestCancel_NotFound` — random UUID → `"job not found"`.

**Periodic deterministic grid:**
- `TestPeriodic_DeterministicGrid` — hourly: `10:00:03.123ns` vs `10:00:07.987ns` → same `11:00:00` grid next; on-boundary `10:00:00` → `11:00:00`; daily: `05:23:11` vs `05:23:45` → same `00:00:00` next day; RFC3339 keys for same grid second identical, contain no `"."`; `RFC3339Nano` would diverge (harness self-check).
- `TestPeriodic_RFC3339_NotNano` — verifies `time.RFC3339` format has no fractional seconds even with `500ms` nanos input; isolates time part before checking `"."`.
- `TestPeriodic_Tick_GridAlignment` — two ticks `+10ms` vs `+200ms` after grid `next` map to same `Truncate(time.Second).Format(time.RFC3339)` key.
- `TestPeriodic_ZeroInterval_Guarded` / `TestPeriodic_NegativeInterval_Guarded` — edge cases for `PeriodicInterval(0)` and `-1h`.

**Predicate SQL shape:**
- `TestStorePredicate_DequeueDoesNotIncrementRetry` — `dequeueSQL` must NOT contain `retry_count=`.
- `TestStorePredicate_RetryIsSoleAccountant` — `retrySQL` must contain `retry_count=retry_count+1`.

### 3.2 `forge/api/internal/eventstore/eventstore_reverification_test.go` (new, 341 lines)

**PublishTx atomic:**
- `mockTx` harness — records `execs`/`args`, injectable `failOn` failure.
- `TestPublishTx_Atomic` — verifies 12-column INSERT shape, `dispatched=false`, `failure_count=0`, JSON payload, ID preserved.
- `TestPublishTx_NilTxRejected` — nil tx → `"tx is required"`.
- `TestPublishTx_GeneratesIDAndTimestampWhenZero` — zero ID/timestamp auto-generated UUID/time.
- `TestPublishTx_InsertErrorPropagates` — `unique violation` → `"publishtx insert"`.
- `TestPublishTxBatch_Atomic` — 3 envelopes → 3 Exec; second fail → error after 2 calls (tx rollback would discard).
- `TestPublishTxBatch_EmptyIsNoop` — nil batch → 0 Exec.
- `TestPublishTx_SQLiteRoundTrip` — column order matches `Publish` 12-col contract.
- `TestPublish_PersistsDispatchedFalse` — integration: `Publish` then SELECT dispatched/failure_count.

**Subscribe durable relay:**
- `TestSubscribe_DurableRelay` — `SubscriberCount` 0→1→2→3 via `Subscribe`/`SubscribeSubscriber`/`SubscribeTyped`; nil subscriber no-op.
- `TestSubscribeTyped_Filters` — `Created` event hits typed+generic (1,2), `Deleted` only generic → counts `1,2`; pending empty after dispatch.
- `TestRelay_ZeroSubscriber_NotDispatched` — zero handlers + 180ms/50ms relay → DB `dispatched=false`, `failure_count>=1`, `last_error` contains `"zero subscribers"`, `Count(dispatched=true)==0`, pending or DLQ contains AF-1 error. Also creates `events_dead_letter` table in test DB to support `MoveToDeadLetter` path.
- `TestRelay_DeliverWithRetries_ThenDLQ` — placeholder documenting DLQ path (requires PG).

---

## 4. Fixes Applied to Existing Code (to make suite green)

**`forge/api/internal/eventstore/store_test.go`**

- `setupTestDB` — added `tenant_id TEXT` column to `events` CREATE TABLE (additive, nullable).
- `setupTestDB` — added `events_dead_letter` DDL (needed for `MoveToDeadLetter` CTE path in zero-subscriber test).
- `newTestStore.pendingQuery` — changed from 11-col `... correlation_id, payload ...` to 12-col `... correlation_id, COALESCE(tenant_id,''), payload ...` to match `Pending`'s `Scan(&e.TenantID, &e.Payload, ...)`.

**Rationale:** Production `store.go:Publish` inserts 12 columns including `tenant_id`; test harness was stale after `215_tenant_scoping_additive.sql`. Without fix, *all* DB-backed eventstore tests failed (`go test ./forge/api/internal/eventstore` FAIL before fix). After fix: PASS (1.5–2s, 2 SKIPs for PG-only tenant tests).

---

## 5. Test Runs (final, after fixes)

### Queue

```
go test ./forge/api/internal/services/queue -count=1 -v 2>&1 | tail -n 30
=== RUN   TestCancel_AcceptsPending
--- PASS: TestCancel_AcceptsPending (0.00s)
=== RUN   TestCancel_AcceptsRunning
--- PASS: TestCancel_AcceptsRunning (0.00s)
=== RUN   TestCancel_NotFound
--- PASS: TestCancel_NotFound (0.00s)
=== RUN   TestPeriodic_DeterministicGrid
--- PASS: TestPeriodic_DeterministicGrid (0.00s)
=== RUN   TestPeriodic_RFC3339_NotNano
--- PASS: TestPeriodic_RFC3339_NotNano (0.00s)
=== RUN   TestPeriodic_Tick_GridAlignment
--- PASS: TestPeriodic_Tick_GridAlignment (0.00s)
=== RUN   TestStorePredicate_DequeueDoesNotIncrementRetry
--- PASS: TestStorePredicate_DequeueDoesNotIncrementRetry (0.00s)
=== RUN   TestStorePredicate_RetryIsSoleAccountant
--- PASS: TestStorePredicate_RetryIsSoleAccountant (0.00s)
=== RUN   TestPeriodic_ZeroInterval_Guarded
--- PASS: TestPeriodic_ZeroInterval_Guarded (0.00s)
=== RUN   TestPeriodic_NegativeInterval_Guarded
--- PASS: TestPeriodic_NegativeInterval_Guarded (0.00s)
=== RUN   TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert
--- PASS: TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert (0.00s)
=== RUN   TestDispatchIdempotentUsesStableOperationID
--- PASS: TestDispatchIdempotentUsesStableOperationID (0.00s)
=== RUN   TestDequeueDoesNotAccountRetries
--- PASS: TestDequeueDoesNotAccountRetries (0.00s)
=== RUN   TestRetryIsSoleRetryAccountant
--- PASS: TestRetryIsSoleRetryAccountant (0.00s)
PASS
ok  	gamepanel/forge/internal/services/queue	0.261s
```

20 tests total: 3 original `periodic_dedup`, 2 `queue_test`, 2 `retry_accounting` + 13 new reverification → all PASS.

### EventStore

```
go test ./forge/api/internal/eventstore -count=1 2>&1 | tail -n 30
ok  	gamepanel/forge/internal/eventstore	1.540s
```

Expanded `–v` run (prior):
```
TestPublishTx_Atomic, TestPublishTx_NilTxRejected, TestPublishTx_GeneratesIDAndTimestampWhenZero,
TestPublishTx_InsertErrorPropagates, TestPublishTxBatch_Atomic/EmptyIsNoop, TestPublishTx_SQLiteRoundTrip,
TestSubscribe_DurableRelay, TestSubscribeTyped_Filters, TestRelay_ZeroSubscriber_NotDispatched,
TestPublish_PersistsDispatchedFalse, TestPublish, TestPending*, TestMarkDispatched, TestOutboxPublisher*,
TestRelayPollsAndDispatches/StopsOnCancellation → PASS (2 SKIPs: PG tenant tests without DB)
```

Prior to fix: 8 FAIL due to missing `tenant_id`. After fix: 0 FAIL.

---

## 6. Coverage Summary

| Requirement | Test | Status |
|---|---|---|
| `TestCancel_NoRegressCompleted` — cancel must not flip `completed→failed` | `queue_reverification_test.go:10` | ✅ PASS |
| `TestCancel_NoRegressFailed/Cancelled/NotFound` — other terminals | `queue_reverification_test.go:42,71,127` | ✅ PASS |
| `TestPeriodic_DeterministicGrid` — keys same across replicas same epoch | `queue_reverification_test.go:154` | ✅ PASS |
| RFC3339 non-Nano invariant | `queue_reverification_test.go:228` | ✅ PASS |
| `TestPublishTx_Atomic` (if applicable) | `eventstore_reverification_test.go:27` + 5 more `PublishTx*` | ✅ PASS (mockTx + integration) |
| `Subscribe` durable (outbox.go:39) | `eventstore_reverification_test.go:180,225,281` | ✅ PASS |
| Elector (queue/leader.go:23) | Inspected `forge/api/queue/leader.go` — documented §1.5 | ✅ Inspected |
| Cancel predicate (queue/store.go:113) | `queue.go:303` predicate + `store.go:113` unconditional Fail | ✅ Verified |
| Periodic RFC3339 deterministic (periodic.go:142) | `periodic.go:163` `Format(time.RFC3339)` | ✅ Verified |

---

## 7. Paths

- New: `forge/api/internal/services/queue/queue_reverification_test.go`
- New: `forge/api/internal/eventstore/eventstore_reverification_test.go`
- Patched: `forge/api/internal/eventstore/store_test.go` (tenant_id + dead_letter + pendingQuery)
- Inspected: `forge/api/internal/services/queue/periodic.go:142`, `forge/api/internal/services/queue/store.go:113`, `forge/api/internal/services/queue/queue.go:303`, `forge/api/internal/eventstore/store.go:267`, `forge/api/internal/eventstore/outbox.go:39`, `forge/api/queue/leader.go:23`
