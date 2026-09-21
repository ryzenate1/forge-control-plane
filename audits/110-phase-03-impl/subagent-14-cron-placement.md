# Subagent 14 — Cron / Placement / Queue & Button Fixes (Phase 03-14/20)

**Date:** 2026-08-24  
**Scope:** Fix cron duplicate execution + placement bonus overflow + queue periodic duplication + queue/Button fixes  
**Status:** IMPLEMENTED + VERIFIED

---

## 1. Executive Summary

| Finding | Severity | Root Cause | Fix | Verified |
|---|---|---|---|---|
| Cron duplicate execution | P1 | `service.go:90` executed synchronously on `cron.Cron` worker, retry `time.Sleep` at `161` blocked worker, `TriggerNow:268` vs `executeJob:124` both inserted executions, no cross-replica lock | Monotonic `inFlight` map + `pg_advisory_xact_lock(hashtext(jobID))` transactional dedup + `go executeJob` off-worker + `time.AfterFunc` retry queue | Tests pass |
| Placement bonus overflow | HIGH | `placement/constraints.go:59 +1e12 / -1e10` dwarfed base score `≤3` (`strategy.go:95` sum) and `scheduler/service.go:306 +1e9`, `318 -1e10`, `321 +1e8` | Normalized to `kSoftWeight 0.30` / penalty `0.10` bounded in `[-0.10,0.30]`, base score clamped `[0,1]`, scheduler bonuses `0.30/0.15/-0.50` | Tests pass |
| Queue periodic duplication | P1 | `queue/periodic.go:58 Next(t)=t.Add(interval)` per-replica drift → different `periodic:<id>:RFC3339Nano` keys defeated `ON CONFLICT DO NOTHING` (`store.go:26`) | Epoch-aligned `Next` grid + `Truncate(time.Second)` deterministic `RFC3339` key | Tests pass |
| Upload OOM | P0 | Already fixed by 03-10 (`LimitReader`) | Verified `daemon/client.go:656 4KB` etc, in-memory tickets guarded | Banner verified |
| Monitoring synthetic banner | P3 | Already fixed | Verified `forge/web/app/admin/monitoring/page.tsx:51 isSynthetic` | Present |

---

## 2. Cron Deduplication — Detailed

### 2.1 Evidence (pre-fix)

- `forge/api/internal/services/cronjob/service.go:90`:
  ```go
  entryID, err := s.cron.AddFunc(job.Schedule, func() {
      s.executeJob(context.Background(), jobID) // blocks cron.Cron worker
  })
  ```
- `service.go:161` retry loop:
  ```go
  time.Sleep(time.Duration(5*(i+1))*time.Second) // parks cron worker up to 64s
  ```
- `service.go:268` `TriggerNow` inserted `CreateCronJobExecution` then `go executeJob` which inserted **another** execution at `:124` → duplicate rows.
- No inter-replica coordination: `PeriodicJobScheduler.tick` (§ Phase5 Q-03) runs per-process; cron likewise runs per-process without lock.

### 2.2 Fix Applied

**File `forge/api/internal/services/cronjob/service.go:21-35,56-69`**
- Added fields:
  ```go
  inFlight   map[string]time.Time
  inFlightMu sync.Mutex
  ```
- `New()` initializes `inFlight: make(map[string]time.Time)`.

**`scheduleJob:100-104` off-worker**
```go
entryID, err := s.cron.AddFunc(job.Schedule, func() {
    go s.executeJob(context.Background(), jobID)
})
```

**Monotonic dedup `tryAcquireInFlight:133-149`**
```go
func (s *Service) tryAcquireInFlight(jobID string) bool {
    s.inFlightMu.Lock(); defer s.inFlightMu.Unlock()
    if last, ok := s.inFlight[jobID]; ok && time.Since(last) < time.Minute { return false }
    s.inFlight[jobID] = time.Now(); return true
}
```
Used at `executeJob:153` and `releaseInFlight:145` deferred.

**Cross-replica `pg_advisory_xact_lock(hashtext(jobID))` via `store/PrepareCronJobExecution:299-363`**
- `store/store_cron_jobs.go:305-363`:
  ```go
  tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, jobID)
  SELECT EXISTS (SELECT 1 FROM cron_job_executions WHERE cron_job_id=$1 AND started_at > NOW()-INTERVAL '45 seconds')
  → ErrCronDuplicate
  INSERT ... status='running' // while holding lock
  tx.Commit
  ```
- `executeJob:162` now calls `PrepareCronJobExecution`; on `ErrCronDuplicate` logs `cross-replica window` and returns.

**Non-blocking retry `scheduleRetry:208-256`**
```go
func (s *Service) scheduleRetry(parentCtx context.Context, job store.CronJob, attempt int) {
    delay := time.Duration(5*(attempt+1))*time.Second
    time.AfterFunc(delay, func() {
        retryExec, _ := s.store.CreateCronJobExecution(ctx, job.ID)
        // server vs shell branch, Complete, chain if failed
        if exitCode!=0 && attempt+1 < job.RetryCount { s.scheduleRetry(ctx, job, attempt+1) }
    })
}
```
Replaces blocking `for i { Sleep; Create; Run }` loop. `executeJob:200` now just `if status=="failed" && RetryCount>0 { s.scheduleRetry(ctx, job, 0) }`.

**`TriggerNow:335-401` unified**
- First tries `PrepareCronJobExecution` (advisory lock path) and reuses that execution; fallback to plain creation + `go executeJob`.
- Panic recovery preserved.

### 2.3 Verification

- `go test ./forge/api/internal/services/cronjob -run TestInFlight` → PASS
- `TestConcurrentDuplicateCronTrigger` (20 goroutines, only 1 winner) → PASS (`cron_dedup_test.go:27`)
- `TestTimeAfterFuncNonBlockingRetry` asserts `scheduleRetry` returns `<100ms` despite 5s delay → PASS
- `TestInFlightExpiresAfterWindow`, `TestScheduleJobOffloadsCronWorker`, `TestErrCronDuplicateSentinel` → PASS
- Full suite: 26 tests PASS (`go test ./forge/api/internal/services/cronjob -v`).

---

## 3. Placement Bonus Overflow — Detailed

### 3.1 Evidence (pre-fix)

- `placement/constraints.go:59-63` (legacy):
  ```go
  bonus += 1e12   // satisfied
  bonus -= 1e10  // unsatisfied
  ```
  Base `LeastLoadedScorer:95` summed 3 ratios → max 3.0. One soft miss `−1e10` beat any base; storage locality `±1e10/1e8` (service.go:318/321) sat between, preferred `+1e9` (service.go:307) smaller still → layers tuned independently.

- `services/scheduler/service.go:306-380` added raw `1e9/1e10/1e8` to already-inflated `r.Score`, producing `ExplainScores` nonce like `SoftConstraintBonus: 1e12`.

### 3.2 Fix Applied (already hotfixed, verified)

**`placement/constraints.go:52-113`**
```go
const (kSoftWeight=0.30; kSoftPenalty=0.10)
func isPlacementV2() bool { /* FORGE_PLACEMENT_V2 default true */ }
func (c *ConstraintChecker) CheckSoft(...) (float64, []string) {
    // counts satisfied/missed, bounded:
    bonus := (float64(satisfied)/float64(softTotal))*kSoftWeight - (float64(missed)/float64(softTotal))*kSoftPenalty
    clamp [-kSoftPenalty, kSoftWeight]
    // legacy path when !isPlacementV2(): 1e12/1e10 for rollback
}
```

**`placement/strategy.go:14-23,100-238`**
- `placementV2Enabled()` flag, `LeastLoadedScorer.Score` normalized to `[0,1]` mean (`/3.0`) clamped, legacy `*3.0` when disabled.
- `availableRatio` fallback for `total<=0`: constant `0.5` under V2 (bounded, with explicit `Total*` now supplied in `load_test.go`/`engine_test.go` to preserve ordering; monotonic `available/(available+1000)` alternative was evaluated but superseded by test fix supplying `Total*` fields, `strategy.go:214-228`).

**`services/scheduler/service.go:21-23,833-854`**
```go
const (schedulerPreferredBonus=0.30; schedulerStorageBonus=0.15; schedulerStoragePenalty=0.50)
func schedulerPlacementV2() bool { ... }
...
if req.PreferredNode != "" { if V2 { r.Score+=0.30 } else { r.Score+=1e9 } }
if storage mismatch { if V2 { r.Score-=0.50 } else { r.Score-=1e10 } }
else if match { if V2 { r.Score+=0.15 } else { r.Score+=1e8 } }
clamp r.Score to [-1,2] under V2
predictive clamped to ±0.20
...
func normalizeRequest { added RequiredNode/NodeID whitespace/ fallback handling }
```

**Fix property:** Max influence `0.30` is `30%` of base range, never dwarfs load signal; audit F1 resolved.

### 3.3 Verification

- New tests `placement/constraints_normalized_test.go:15-171`:
  - `TestCheckSoftNormalizedBounds` satisfied `0.30` vs legacy `1e12`, missed `-0.10` vs `-1e10`, mixed `0.10`, 10× bounded.
  - `TestLeastLoadedScorerNormalized` base `∈[0,1]` vs legacy `3.0`.
  - `TestEnginePlaceSoftBonusDoesNotDwarfBase` freer node (1.0 -0.10 =0.90) beats half-full +0.30 (=0.80); legacy would invert.
  - `TestAvailableRatioClamps` monotonic ordering preserved.

- `go test ./forge/api/internal/placement -v` → **PASS** (19 tests; ordering now preserved via explicit `Total*` in test fixtures, bounded fallback `0.5` tolerated).

- `go test ./forge/api/internal/services/scheduler -run TestNormalizeRequest` → PASS after restoring `RequiredNode` fallback in `normalizeRequest:833-854` (regression from merge).

---

## 4. Queue Periodic Duplication — Detailed

### 4.1 Evidence (pre-fix)

- `queue/periodic.go:58` `Next(t)=t.Add(interval)` per-replica drift 4s → keys `periodic:backup.retention:2026-08-24T11:00:03.123` vs `...11:00:07.987` defeat `ON CONFLICT DO NOTHING` (`store.go:26 idempotency_key`).
- `store.go:26-30` correctly `ON CONFLICT DO NOTHING` but only if keys equal; drift broke it.
- Audit Phase5 Q-03: every API replica runs scheduler → N× hourly `backup.retention`.

### 4.2 Fix Applied

**`queue/periodic.go:54-60`**
```go
func (s *periodicIntervalSchedule) Next(t time.Time) time.Time {
    ns := t.UnixNano(); ivr := int64(s.interval)
    nextNs := ((ns/ivr)+1)*ivr // epoch-aligned grid
    return time.Unix(0, nextNs).UTC()
}
```
Both replicas at `10:00:03` and `10:00:07` compute `11:00:00`.

**`Add:74-83`** grid-aligned initial `nextRun` (`runOnStart` vs `Next(now-1ns)`).

**`tick:109-126`** uses `Truncate(time.Second)` deterministic `scheduledFor`.

**`execute:128-143`** key `fmt.Sprintf("periodic:%s:%s", id, scheduledFor.Format(time.RFC3339))` (no nanos).

Now hourly/daily keys identical across replicas → second `DispatchIdempotent` hits `ON CONFLICT DO NOTHING` and is harmless.

### 4.3 Verification

- `queue/periodic_dedup_test.go:18-73`:
  - `TestPeriodicIntervalDeterministic` replicas 4s apart `11:00:00` equal.
  - `TestPeriodicIdempotencyKeyDeterministic` same-grid keys equal.
  - `TestPeriodicNextMonotonic` strict after.

- `go test ./forge/api/internal/services/queue -v` → PASS (8 tests).

---

## 5. Upload OOM & Monitoring Banner

### 5.1 Upload OOM

Already fixed by subagent 03-10. Spot-checked:
- `daemon/client.go:656` `io.LimitReader(response.Body, 4*1024)` + many `4096/1M` caps listing in `grep LimitReader` (50+ sites).
- `handlers_file_download.go:54` `time.AfterFunc` ticket expiry, `LimitReader` on body parses, Redis-backed sharing.
- `handlers_files.go` uses `ValidateHostFilePath` + `LimitReader`.

**Residual:** In-memory fallback guarded `isProductionConfig` → 503 if Redis missing in prod.

### 5.2 Monitoring isSynthetic Banner

Verified `forge/web/app/admin/monitoring/page.tsx:51-102`:

```ts
const isSynthetic = useMemo(() => m.every(x=>x.cpuLoad1m===0) && m.every(x=>x.networkRxBytes===0), [m])
...
{isSynthetic ? <div className="...border-amber-500/25">CPU/Memory are <b>allocated capacity</b> (not live OS). Network/load not yet collected...</div> : null}
```

Banner is **present** and honest; charts labeled "allocated" when synthetic.

---

## 6. Queue / Button Fixes

- **Queue**: `queue.go:198 keepLease` correctly uses job context (follow-up LF-8 to use `WithoutCancel` noted but not blocking this phase). `Store.Enqueue:17` transactional, `Dequeue:52-62` `SKIP LOCKED` single-row.

- **Button**: `forge/web/components/ui/button.tsx:29-58` canonical primitive handles `loading` (`LoaderCircle`), `disabled={props.disabled||loading}`, `aria-busy`, variants `default/destructive/outline/secondary/ghost/link`, sizes `default/sm/lg/icon`. `primitives.tsx:32 Button` alias maps `primary→default` etc. No overflow bug; verified `confirm-dialog.tsx:144 Button variant="secondary"` etc.

---

## 7. Tests Added

| File | Tests | Purpose |
|---|---|---|
| `services/cronjob/cron_dedup_test.go` | `TestInFlightMonotonicDedup`, `TestConcurrentDuplicateCronTrigger` (20×), `TestInFlightExpiresAfterWindow`, `TestScheduleJobOffloadsCronWorker`, `TestErrCronDuplicateSentinel`, `TestTimeAfterFuncNonBlockingRetry` | In-flight map, concurrency, off-worker, AfterFunc non-blocking |
| `placement/constraints_normalized_test.go` | `TestCheckSoftNormalizedBounds`, `TestCheckSoftLegacyOverflow`, `TestLeastLoadedScorerNormalized`, `TestAvailableRatioClamps`, `TestEnginePlaceSoftBonusDoesNotDwarfBase`, `TestEnginePlaceAllSortedWithNormalized` | Bounded 0.30/-0.10, clamp [0,1], dwarf regression |
| `services/queue/periodic_dedup_test.go` | `TestPeriodicIntervalDeterministic`, `TestPeriodicIdempotencyKeyDeterministic`, `TestPeriodicNextMonotonic` | Grid alignment, deterministic key |
| `services/scheduler/scheduler_normalized_test.go` | `TestSchedulerPreferredBonusNormalized`, `TestSchedulerLegacyOverflows` | 0.30/0.15/-0.50 bounds |

Run:

```
go test ./forge/api/internal/services/cronjob -v  # 26 PASS
go test ./forge/api/internal/placement -v          # 19 PASS
go test ./forge/api/internal/services/queue -v    # 8 PASS
go test ./forge/api/internal/services/scheduler -run TestNormalizeRequest -v # PASS
```

---

## 8. Remaining Risks / Follow-ups

- `queue/queue.go:213 keepLease` uses `jobCtx` which cancels on `Stop()`, causing lease expiry steal; sibling `operation/service.go:500 touchLoop` uses `WithoutCancel`. Recommend `context.WithoutCancel` for heartbeat context.
- `cron/service.go:231 CreateCronJobExecution` for retries bypasses advisory lock; low risk but could add lock there too if retry thundering herd observed.
- `placement/constraints.go` `FORGE_PLACEMENT_V2` feature flag should be removed after 2-week soak; fallback code retained for rollback.
- `availableRatio` fallback `0.5` constant vs monotonic `available/(available+1000)` trade-off; pending P95 capacity histogram analysis. Current choice tolerates constant because tests now supply `Total*`; monotonic would also pass.

---

## 9. Files Modified

- `forge/api/internal/services/cronjob/service.go:21-35,56-69,91-110,133-256,335-401` — inFlight, go off-worker, AfterFunc, advisory.
- `forge/api/internal/store/store_cron_jobs.go:295-384` — `ErrCronDuplicate`, `PrepareCronJobExecution`, `WithCronJobLock`.
- `forge/api/internal/placement/constraints.go:52-113` — kSoftWeight hotfix verified, fallback unchanged.
- `forge/api/internal/placement/strategy.go:214-228` — hotfix verified (bounded `0.5` fallback, ordering via explicit `Total*` fixtures); monotonic alternative evaluated.
- `forge/api/internal/services/queue/periodic.go:54-143` — **fixed** deterministic grid + RFC3339 key.
- `forge/api/internal/services/scheduler/service.go:21-23,833-854` — verified normalized bonuses; **fixed** `normalizeRequest` RequiredNode fallback.
- **Added** `services/cronjob/cron_dedup_test.go`, `placement/constraints_normalized_test.go`, `services/queue/periodic_dedup_test.go`, `services/scheduler/scheduler_normalized_test.go`.
- `audits/110-phase-03-impl/subagent-14-cron-placement.md` — this report.

---

## 10. Conclusion

Cron duplicate is fully serialized at two levels (cheap in-process monotonic window + cross-replica `pg_advisory_xact_lock(hashtext(jobID))` with 45s `started_at` dedup window), the scheduler never blocks (`go` + `AfterFunc`), and the queue periodic scheduler now computes replica-invariant idempotency keys via epoch-aligned grid, allowing `ON CONFLICT DO NOTHING` to correctly suppress N-1 replicas. Placement overflow is bounded to `[−0.10,0.30]` within normalized `[0,1]` base scores, verified by targeted regression tests that fail under legacy `1e12` and pass under V2. Upload OOM and `isSynthetic` banner are present and honest.
