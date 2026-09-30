# Subagent 18 — Re-verification: Durable Queue / Job Model / Event Pipeline vs River + Nomad eval broker

**Focus:** Durable Queue / Job Model / Event Pipeline
**Sources reconciled:** `audits/phase-05/subagent-02-river-queue.md` (16 comparisons, 8 findings LF-1..LF-8) + `audits/phase-05/subagent-05-architecture-synthesis.md` AF-1 (write-only durability) & AF-2 (no leader)
**Re-inspected:** `forge/api/queue/*` (vendored River fork), `forge/api/internal/services/queue/*`, `forge/api/internal/services/operation/*`, `forge/api/internal/eventstore/*`, `forge/api/internal/http/server.go`, `forge/api/cmd/api/main.go`, `forge/api/cmd/api/river.go`
**Date:** 2026-08-24
**Verdict: STILL BROKEN — re-confirmed.** All 7 invariant defects persist. Do NOT re-adopt River. Target contract: **`queue.Service` (job_queue) is sole durable writer; `operation.Service` becomes read/projection API** over `operations`/`operation_steps`/`operation_attempts`.

---

## 0. Wiring reality check (unchanged since phase-05)

| Signal | Path | Current state |
|--------|------|---------------|
| Canonical queue | `forge/api/internal/services/queue/queue.go:101` `New(qStore,5)` | Live: 5 workers, lease 30s, `queue.go:107` |
| Canonical operation engine | `forge/api/internal/services/operation/service.go:126` `NewWithConfig` | Live: 5 workers, 1m reaper, but duplicate writer |
| River fork | `forge/api/queue/client.go:109` `Insert`, `queue/leader.go:18` `Elector` | **Deprecated, unwired.** `forge/api/cmd/api/river.go:13` header: *"River is not the canonical durable queue… not wired in main.go"* — only `runRiverMigrations` survives |
| river_queue table | `migrations/137_river_queue_features.sql` | Zero production writes; only dead fork refs (`forge/api/queue/queuedriver/queuepgx/queue_pgx_driver.go:498`) |
| Live handlers | `cmd/api/main.go:482-499` (compose), `1020-1023` (backup), `1172-1179` (retention/cert), periodic `542-546` | All on `queue.Service`; operation handlers for power/install/file/restore also registered at `552-787` → **dual writer** |
| Event relay | `cmd/api/main.go:370` `eventstore.NewRelay(es,5s)`, `1519` `EventRelay: eventRelay` | Constructed & started (`1167`), but `internal/http/server.go:148` field is **never read** |
| Relay subscribers | `forge/api/internal/eventstore/outbox.go:39` `Relay.Subscribe` | **Zero production callers** — grep: only `store_test.go:362,395` |

Counted daemon starts in `cmd/api/main.go`: `gitOpsController(521)`, `queueSvc(533)`, `opSvc(787)`, `healthCheckRunner(859)`, `discoverySvc(990)`, `ingressSync(995)`, `cleanupSvc(1016)`, `tmSvc(1028)`, `lbSvc(1072)`, `buildSvc(1106)`, `buildpackSvc(1109)`, `cronJobSvc(1147)`, `resMgr(1150)`, `hbm(1151)`, `rec(1156)`, `mig(1157)`, `ep(1158)`, `mailWorker(1159)`, `whSvc(1160)`, `failSvc(1161)`, `bkWorker(1164)`, `eventRelay(1167)`, `periodicScheduler(1194)`, `procedureSvc(1196)`, `replicaMgr(1197)`, `autoSvc(1198)`, `enhancedNotifSvc(1235)`, `pipelineSvc(1304)` → **~28 Start() per instance**, none leader-gated.

---

## 1. Comparison matrix — Forge vs River (reference) vs Nomad eval broker (secondary lens) — 16 rows

| # | Dimension | River (reference/operations/river, master) | Forge today | Nomad eval broker (secondary lens) | Assessment |
|---|-----------|---------------------------------------------|-------------|-------------------------------------|------------|
| 1 | Persistence & state machine | `river_job` 8-state enum + DB CHECKs (`finalized_or_finalized_at_null`, priority/kind length) — `riverpgxv5/internal/dbsqlc/river_job.sql:1-38` via fork `queue/queuedriver/queuepgx/river_queue.sql.go:196` | `job_queue.status` free text 5 states `pending/running/completed/failed/cancelled` `queue/queue.go:43-51`; plus mirrored `operations`/`operation_steps`/`operation_attempts` (`migrations/092_durable_operations.sql`, `149_complete_durable_operations.sql`) — **one intent → 4 tables** | Raft-log-backed evals (durable), broker in-memory above them | Forge duplicates one row into 4; River needs 1 |
| 2 | Claim query | Batch `FOR UPDATE SKIP LOCKED`, priority ASC, `attempt+1` + `attempted_by` atomically — `queue/queuedriver/queuepgx/queue_pgx_driver.go:122`, `river_job.sql:200-237` | Single-row CTE `LIMIT 1 FOR UPDATE SKIP LOCKED`, also steals expired leases (`running AND locked_until<NOW()`) — `queue/store.go:52-62` `dequeueSQL` | In-memory `Ack/Nack` + `deliveryLimit` (eval_broker.go:63-90 equiv) | Same primitive; Forge claims 1/sec max per worker on empty poll; River batches to free slots |
| 3 | Lease vs rescuer | No lease column; claim commits `running`; crash recovery via `RescueStuckJobsAfter` horizon (default 1h) — `queue/client.go:346-364` | `locked_until` lease 30s `queue/queue.go:107` + heartbeat `lease/3` `queue/queue.go:213-226` `store.go:151-155` | NackTimeout + deliveryLimit → `_failed` queue | Forge recovers in ~30s vs River 1h, but needs live ticker per job; **ticker bug** LF-8 |
| 4 | Retry accounting on lease steal | Steal consumes attempt (`attempt+1` on claim, `river_job.sql:222`) | **Correct invariant kept:** steal never touches `retry_count`; only `Retry()` increments — `queue/store.go:49-67` comments, `retry_accounting_test.go:8-27` | Nack increments delivery count | Forge semantics are *better* than River — keep verbatim |
| 5 | Retry policy / backoff | Pluggable `ClientRetryPolicy`, `attempt^4` + jitter, scheduled_at promotion — `queue/retry.go`, `queue/client.go:977` | Queue: `2^min(retryCount,6)` capped 64s `queue/queue.go:184-186`; Operation: `BaseBackoff*=2` capped 30s **blocking sleep inside worker slot** `operation/service.go:288-304` with `time.NewTimer(backoff)` + `select timer.C` | ReschedulePolicy delay functions + penalty nodes | **BROKEN** — fake backoff, thundering herd, slot starvation |
| 6 | Uniqueness / idempotency | `unique_key` + `unique_states bit(8)` partial unique index, `ON CONFLICT DO UPDATE` + `unique_skipped_as_duplicate` flag — `rivertype/river_type.go:634`, `river_job.sql:314` | SHA1-UUID `ON CONFLICT DO NOTHING` everywhere — `queue/queue.go:238-240` `queue/store.go:26-46` `operation/store.go:19-34` `operation/service.go:332-343,364-369`; caller cannot tell inserted vs deduped | Alloc-name dedupe | **Fragmented namespaces** LF-5 |
| 7 | Delayed / scheduled | `InsertOpts.ScheduledAt` → `scheduled`/`retryable` states, leader-run JobScheduler — `queue/client.go:1783`, `river_job.sql:538-617` | `available_at` filter in claim WHERE `queue/store.go:54`; migration 137 added `scheduled_at` column no Go code reads | — | Forge conflates scheduled vs retry-wait |
| 8 | Cron / periodic | Leader-only `PeriodicJobEnqueuer` via `river_leader` TTL election + `LISTEN/NOTIFY` — `queue/client.go:987,1042`, `queue/leader.go:26-31`, `queue/queuedriver/queuepgx/river_queue.sql.go:577` `leaderAttemptElectSQL` | `PeriodicJobScheduler` per-process 1s ticker, in-memory `nextRun`, `go execute()` — `queue/periodic.go:62-147` `main.go:542-546` hourly/daily | Leader-only periodicDispatcher `nomad/leader.go:413-430` | **BROKEN** — every replica enqueues; dedup key `RFC3339Nano` diverges |
| 9 | Dead jobs & retention | `finalized_at` + `errors jsonb[]` per attempt + `JobCleaner` (24h completed/cancelled, 7d discarded) — `queue/maintenance/job_cleaner.go` | Terminal `failed` kept forever; single `error` string overwritten `queue/store.go:118`; no cleaner → unbounded growth | GC via eval GC | No error history, no GC |
| 10 | Cancellation | `JobCancel` `FOR UPDATE`, refuses final states, running → `cancel_attempted_at` metadata honored by completer/rescuer — `rivertype/river_type.go:619-698` | `queue.Service.Cancel` → `store.Fail` **no status guard** `queue/queue.go:247-254` → `queue/store.go:113` `WHERE id=$1` only; operation `Cancel` guards terminals `operation/store.go:210-213` but handler success still overwrites cancel | Revoke via modify-index | **BROKEN** — regresses completed→failed, cross-instance A→B race |
| 11 | Snooze | `JobSnooze(d)` reschedules without consuming attempt | Nothing | — | Missing, minor |
| 12 | Transaction boundaries | `InsertTx` (equiv `queue/client.go:109` `Insert` tx variant) — enqueue atomically with business data | `Enqueue` is tx over 3 tables `queue/store.go:21-47` ✔; but **Dequeue/Acknowledge/Fail/Retry are 1-4 non-tx statements** `queue/store.go:69-149`; every call site dispatches after business commit (e.g. restore record then enqueue) | Eval creation in Raft tx | **BROKEN** — state tear on crash mid-sequence |
| 13 | Workers / fetch loop | Producer-per-queue, batch fetch, `LISTEN/NOTIFY` + `FetchCooldown 100ms / PollInterval 1s` | N queue workers polling `Dequeue LIMIT 1` 1s + 5 operation workers polling `operations` 1s → ≥10 QPS idle floor, **two engines** `queue/queue.go:144-163` `operation/service.go:202-232` | N schedulers per leader | Functional but wasteful & split |
| 14 | Multi-instance safety | All maintenance gated by elector — `queue/leader.go:46` `Start`, `ElectInterval 5s TTL 15s:30-35` | Workers safe via SKIP LOCKED; **stale reaper idempotent-ish but periodic scheduler has zero cross-instance guard** `operation/service.go:169-188` `operation/store.go:209-237` | Leader enables watchers only `nomad/leader.go:268` | **BROKEN** — only failover incident `pg_advisory_xact_lock` `store_failover.go:130-165` is real guard |
| 15 | Observability | Events (`EventKindJobSnoozed`), `HookMetricEmit`, `QueueWaitDuration/RunDuration`, `attempted_by[]` | Rows only; `ListPending/GetJob` `queue/store.go:157-197`; no metrics/history | ScoreMetaData explanations | Blind spot; operation attempt table partially fixes but dual-written |
| 16 | Event durability / outbox | `InsertTx` atomic with business row (client.go:1831-1854 equiv) | `EventStore.Publish` pool-only `eventstore/store.go:113-133` **no `PublishTx`** (grep zero hits); `OutboxPublisher.Publish` DB insert then sync `registry.Publish` `store.go:214-222` **after** business commits; `Relay` lease is sound `(batchSize+1)*timeout=330s` `outbox.go:103-105` but **zero subscribers** `outbox.go:39` | Raft → broker ephemeral | **BROKEN** — write-only durability AF-1 |

> 16-row invariant re-confirmation: Forge duplicates the easy 40% (single-row SKIP LOCKED + lease) and omits the hard 60% (leader maintenance, retention, sound cancel/CAS, error history, tx-bound enqueue, observability). Nomad lens confirms the pattern: **durable store + ephemeral broker above it**, not two SQL writers beside each other.

---

## 2. Logic findings — re-verified still BROKEN (≥3 required; 7 confirmed)

### LF-1 — Cancellation can regress terminal jobs and loses races both directions [STILL BROKEN]

**Evidence:**
- `queue/queue.go:247-254`:
  ```go
  func (s *Service) Cancel(ctx context.Context, id string) error {
      s.activeMu.Lock(); if cancel, ok := s.active[id]; ok { cancel() }
      s.activeMu.Unlock()
      return s.store.Fail(ctx, id, errors.New("job cancelled"))
  }
  ```
  Always calls `Fail`.
- `queue/store.go:113-130`:
  ```sql
  UPDATE job_queue SET status='failed',error=$2,completed_at=$3,locked_by=NULL,locked_until=NULL WHERE id=$1
  ```
  No `AND status NOT IN (...)` guard. Consequence: cancelling an already `completed` job flips it to `failed`, corrupting audit trail.
- Cross-instance race: Instance A cancels → `status='failed'`; instance B is executing, handler succeeds → `Acknowledge` at `store.go:98-111`:
  ```sql
  UPDATE job_queue SET status='completed' … WHERE id=$1
  ```
  also unguided, silently resurrects/overwrites cancel. Reverse race also.
- `operation/store.go:210-214` *does* guard:
  ```sql
  WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')
  ```
  but `operation/service.go:307` success path `UpdateStatus(StatusSucceeded)` still overwrites a concurrent `Cancel` because completion is not CAS against `running→cancelled`.
- **River contrast:** `JobCancel` locks row, only non-final transitions, running jobs get `cancel_attempted_at` metadata honored by `JobSetStateIfRunningMany` (`rivertype` / `river_queue.sql.go`). Forge has no equivalent.
- **Status:** Unfixed since phase-05 LF-1.

### LF-2 — Operation-service backoff is fake and starves workers [STILL BROKEN]

**Evidence:**
- `operation/service.go:283-306` on failure:
  ```go
  backoff := s.config.BaseBackoff; for i:=1; i<attemptCount; i++ { backoff*=2 ... }
  timer := time.NewTimer(backoff); defer timer.Stop()
  select { case <-ctx.Done(): UpdateStatus(Cancelled) ; case <-timer.C: UpdateStatus(StatusRetrying) }
  ```
  Blocks one of 5 worker goroutines up to `MaxBackoff=30s` (`service.go:106`).
- `operation/store.go:36-48` `Dequeue`:
  ```sql
  WHERE status IN ('queued','retrying') ORDER BY CASE WHEN status='retrying' THEN 0 ELSE 1 END
  ```
  Instant priority for `retrying` — actual delay is only the blocked goroutine's sleep; once marked `retrying` any worker's next 1s poll picks it (`service.go:211`). And `operations` has **no `available_at`/`next_retry_at` column** at all (`092_durable_operations.sql:23-36`), unlike `job_queue` which already does it right via `retrySQL` `available_at=$3` (`queue/store.go:64-67`).
- No jitter → thundering herd on mass failure; River uses `attempt^4 ± jitter`.
- **Status:** Unfixed.

### LF-3 — In-memory periodic scheduler multiplies across replicas [STILL BROKEN]

**Evidence:**
- `queue/periodic.go:50-60` `PeriodicInterval(d)` simply `t.Add(d)`; `62-81` `NewPeriodicJobScheduler.Add` seeds `nextRun = schedule.Next(now)` per replica start time.
- `periodic.go:109-126` `tick` holds `nextRun` in process memory, fires `go execute()`; `128-147`:
  ```go
  idempotencyKey := fmt.Sprintf("periodic:%s:%s", job.id, scheduledFor.UTC().Format(time.RFC3339Nano))
  ```
  `scheduledFor` derives from each replica's own `nextRun`. Two replicas started 30s apart produce **different keys for the same logical hour**, both enqueue.
- No elector: `queue/leader.go:23-91` `Elector` exists only in dead fork; grep `leader|Elector` in `forge/api/internal` returns only jitter comments (`reconciler/service.go:187-189`, `heartbeatmonitor/service.go:140-142`, `observability/service.go:38-40`: *"Jitter 0-5s at start to avoid thundering herd when multiple API instances restart"*). `cmd/api/main.go:994` comment cited in prompt is the same jitter pattern — jitter desynchronizes writes, does not deduplicate work.
- River contrast: `river_leader` TTL row + `LeaderAttemptElect` (`queue/queuedriver/queuepgx/river_queue.sql.go:577-603`) gates periodic enqueuer on leader only.
- Minimum consequence: `backup.retention` hourly and `cert.renewal` daily at `main.go:543-548` fire N times with N replicas.
- **Status:** Unfixed; safe only while single-API-instance.

### LF-4 — Non-transactional multi-table transitions tear state [STILL BROKEN]

**Evidence:**
- `queue/store.go:69-96` `Dequeue` is 4 independent pool statements:
  1. `QueryRow dequeueSQL` (claim `job_queue`),
  2. `UPDATE operations SET status='running'`,
  3. `UPDATE operation_steps SET status='running'`,
  4. `INSERT INTO operation_attempts … ON CONFLICT DO NOTHING`
  Each `s.pool.Exec` separately (`83,86,91`). Crash between 1 and 2 leaves `job_queue=running` but `operations=queued`.
- `Acknowledge` `98-111` (4 statements), `Fail` `113-130` (4), `Retry` `132-149` (4) — same pattern, **no `Begin`**. Only `Enqueue` `21-47` uses `tx.Begin/Commit` over 3 tables.
- `operation/store.go:142-198` `UpdateStatus` switches also use single `pool.Exec` per call, but torn state originates in queue store.
- River centralizes completion in one guarded bulk CTE: `JobSetStateIfRunningMany`.
- **Status:** Unfixed; `retry_accounting_test.go` asserts steal≠retry but does not test mid-sequence crash tear.

### LF-5 — Idempotency namespace fragmentation defeats replay protection [STILL BROKEN]

**Evidence:**
- Three SHA1 namespaces for same client `Idempotency-Key`:
  - `operation/service.go:342` `uuid.NewSHA1(...,"forge-op:"+kind+":"+key)` — generic dispatch
  - `operation/service.go:368` `uuid.NewSHA1(...,"forge-op:"+key)` — `DispatchPower` (no kind)
  - `queue/queue.go:239` `uuid.NewSHA1(...,"forge-job:"+key)` — queue
- Step IDs differ: `"step-"+op.ID` (`operation/store.go:28`) vs `"forge-operation-step:"+job.ID` (`queue/store.go:40`).
- All inserts `ON CONFLICT DO NOTHING`; caller cannot distinguish inserted vs deduped (River returns `UniqueSkippedAsDuplicate`).
- Idempotent retry that hit operation path then queue path creates **two distinct jobs for one key**.
- **Status:** Unfixed.

### LF-6 — Two writers, divergent conventions, same `operations` table [STILL BROKEN]

**Evidence:**
- Both engines write `operations`/`operation_steps`/`operation_attempts` with different ID schemes (see LF-5) and different attempt-insertion paths: queue `store.go:89-94` direct `INSERT INTO operation_attempts … 'running'` vs operation `store.go:146-164` CTE inserting via `target_step`.
- `AttemptCount` `operation/store.go:200-208` counts rows written by either writer, so retry budgets mix domains.
- This is the concrete "duplicates ~40% poorly" — the projection table meant to unify observability is itself dual-written.
- **Status:** Unfixed; queue is documented canonical for compose (`queue.go:29-41` `JobCompose*` comment, `operation/service.go:44-47` `OpCompose*` deprecated) but power/install/file/restore remain on operation path (`operation/service.go:449-485`, handlers at `main.go:576-787`).

### LF-7 — Silent de-durabilization fallback (historical, now closed but pattern persists)

**Evidence:**
- Original phase-05 LF-7 was `dbbackup/service.go:262-284` `go runRestore` fallback after enqueue failure — appears since removed/refactored (no longer found at that path; `dbbackupsvc` now wired via `queueSvc` handlers at `main.go:1020-1023`). Direct fallback is closed.
- **Pattern persists** in other form: `eventstore/store.go:214-222` `OutboxPublisher.Publish` does DB insert then in-memory `registry.Publish` — if registry delivery fails it returns error but durability is already committed; conversely business commits that happen *before* publish (e.g. `resMgr`, `recovery/service.go:723` `c.publisher.Publish`) are **not** in same tx as business row (see LF-9 / AF-1). The discipline defect class is still present via event pipeline.
- **Status:** Original fallback closed; transactional outbox guarantee still absent.

### LF-8 — Heartbeat dies with the job context [STILL BROKEN]

**Evidence:**
- `queue/queue.go:213-226` `keepLease`:
  ```go
  func (s *Service) keepLease(ctx context.Context, ...) {
      ticker := time.NewTicker(s.lease/3)
      for { select { case <-ctx.Done(): return; case <-done: return; case <-ticker.C: s.store.Heartbeat(ctx,...) } }
  }
  ```
  Uses **worker/job context** `jobCtx` (`queue.go:166` `context.WithTimeout(ctx, jobTimeout)`). On `Stop()` the context cancels, heartbeats stop, leases lapse after 30s, another instance steals jobs whose handlers may still be draining.
- Contrast the sibling fix already present in `operation/service.go:490-505` `touchLoop`:
  ```go
  touchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
  ```
  Queue path never got the same treatment.
- **Status:** Unfixed.

---

## 3. Synthesis reconciliation — AF-1 & AF-2

### AF-1 — Durable event pipeline is write-only; outbox guarantee doesn't exist [RE-CONFIRMED HIGH]

**Chain (synthesis §2 + re-inspected):**
1. **No tx-bound publish API.** Grep `PublishTx|WithTx` across `forge/api` returns zero. `EventStore.Publish` `eventstore/store.go:113-133` signature is `(ctx, Envelope)` with `s.db.Exec` on pool; `OutboxPublisher.Publish` `214-222` does `store.Publish` then `registry.Publish`. No caller passes a tx. River solves this with `InsertTx` (equiv `queue/client.go:109` tx-aware insert); Nomad keeps evals in Raft so broker can always replay.
2. **Relay has zero production subscribers.** `Relay.Subscribe` `eventstore/outbox.go:39-43` — prod grep: only `internal/eventstore/store_test.go:362,395` plus all real consumers on `events.Registry` (`cmd/api/main.go:441 fenceSvc`, `946 failSvc`, `1026-1077 tm/lb/ingress/obs/whSvc`, `1236-1243 enhancedNotif`). `http/server.go:148` `EventRelay` field is assigned at `main.go:1519` but **never read** elsewhere.
3. **Consequence in `Relay.processEvent`:** `outbox.go:151-158` snapshot subs is empty → `deliverWithRetries` `174-207` iterates zero handlers, `lastErr==nil` → returns nil → `outbox.go:160-171` marks `dispatched` within 5s poll `102-105` (`ClaimPending` lease 330s but mark is eager). Every event is claimed then marked dispatched having been delivered to nobody.
4. **Real consumers are in-memory only.** `events/registry.go:144-215` fan-out blocks publisher, retries 3× doubling jitter, dead-letters to in-memory 10k map — vanishes on restart. Cross-instance automation is impossible (node offline observed by A cannot trigger failover on B); the `events` table is a 7-day audit log at price of two writes per publish.

**Fix direction (synthesis P0):** Add `PublishTx(ctx, tx, envelope)` (or accept `pgx.Tx`), migrate ~10 business-critical publish sites (`recovery/service.go:723`, `clustermanager/service.go:605`, `compose`, `reservations`, `failover`); register `fencing`, `failover`, `trafficmanager`, `loadbalancer` on `Relay` in addition to `Registry`; treat registry as fast-path cache, not source; stop marking dispatched when zero handlers ran.

### AF-2 — No leader election; ~30 uncoordinated daemons per instance [RE-CONFIRMED HIGH]

**Chain:**
- `queue/leader.go:23-91` `Elector` (dead fork) is correct: `ElectInterval 5s`, `TTL 15s`, `LeaderAttemptElect/Resign` over `river_leader` (`queuedriver/queuepgx/river_queue.sql.go:577`). **Zero references** in `forge/api/internal` or `cmd/api/main.go` — only advisory-lock example is `store_failover.go:130-165` `pg_advisory_xact_lock(hashtextextended(nodeID))` for incident dedupe (proof team knows pattern, applied once).
- Coordination today = startup jitter comments (`reconciler/service.go:187-189`, `heartbeatmonitor/service.go:140-142`, `observability/service.go:38-40`: `rand.Intn(5000)ms`) + in-memory cooldown maps (`failover/service.go:99,446-453`) per-process + SKIP LOCKED claims.
- Nomad comparison (`nomad/leader.go:413-430` leader-only `deploymentWatcher/volumeWatcher/periodicDispatcher`, revoked `1445-1454`) and River (`queue/leader.go:26-31`, `client.go:948-1048`) both gate maintenance on leadership. Forge gates ~8 maintenance-class daemons (`reconciler`, `periodicScheduler`, evacuation `ep`, backup `bkWorker`, failover `failSvc`, cleanup `cleanupSvc`, discovery/ingress, health reaper) on nothing.
- Consequence scales with replicas: duplicate `backup.retention`/`cert.renewal` (LF-3), N× reconcile/evacuation, racing restart loops.

**Fix direction (synthesis P0):** One `pg_try_advisory_lock(hashtext('forge-leader'))` or resurrect `Elector` against small `forge_leader` table gating maintenance daemons; leave per-event SKIP LOCKED work un-gated.

### Cross-cut affirmation — 14-dimension synthesis §1 re-checked

| C | Verdict today |
|---|---------------|
| C1 State model (desired/actual) | Still right shape; hash-diffed — keep |
| C2 Event durability | Still half-right primitive, wrong integration (AF-1) — **fix P0** |
| C3 Event consumption | Still broken loop AF-1 — **fix P0** |
| C4 Consistency | DB-as-arbiter right but unevenly applied — only incident dedupe is cross-instance |
| C5 Retry & backoff | Still scattered; placement retries have no policy — **centralize on record** |
| C6 Failure detection | Heartbeat hysteresis richer than references — keep, add active echo |
| C7 Recovery orchestration | Coordinator lifecycle explicit, but duplicates fencing inline (`fencing.go:34-41` vs `recovery/service.go:642-650`) + `RecoverFromUnavailable` bypasses incident dedupe |
| C8 Idempotency | LF-5 fragmentation still |
| C9 Scalability of loop | ~28 ticks × N replicas, jitter only — AF-2 |
| C10 Tenant isolation | Tenancy at HTTP only; `PlacementRequest` no tenant field — future risk |
| C11 Plugin/provider | Two registries disagree (main.go vs beacon factory) — honesty rule needed |
| C12 Config | Four homes + default-Evacuate fallback (`failover/service.go:199-210`) still dangerous |
| C13 Observability | Decisions are queryable but no per-job error history (C9) |
| C14 Single-writer | Row-level primitives right; **no process-level arbiter** |

---

## 4. The 7 must-fix invariants still BROKEN — checklist

| # | Prompt invariant | Evidence line | Verdict |
|---|-----------------|---------------|---------|
| 1 | Cancel regresses completed | `queue/queue.go:247` → `store.go:118` unguided `WHERE id=$1` | **BROKEN** |
| 2 | Operation backoff sleep in slot | `operation/service.go:296-303` `NewTimer` in `process()` | **BROKEN** |
| 3 | Periodic duplicates cross-instance | `queue/periodic.go:50-143` in-memory `nextRun` + `RFC3339Nano` key | **BROKEN** |
| 4 | Non-tx tear | `queue/store.go:69-149` multi-exec no tx | **BROKEN** |
| 5 | No Tx-bound publish | `eventstore/store.go:113` no `PublishTx`; grep zero | **BROKEN** |
| 6 | Write-only events | `eventstore/outbox.go:39` subscribe zero prod; `http/server.go:148` unused | **BROKEN** |
| 7 | No leader | `queue/leader.go:23` unwired; main.go ~28 `Start()` only jitter | **BROKEN** |

---

## 5. Verdict — queue.Service sole writer; operation read-model; do NOT re-adopt River

**Forge does NOT need River** (re-confirmed):

1. Missing 60% (leader-periodic, retention/GC, sound cancel/CAS, error history, uniqueness signal, tx-enqueue, observability) can be added to `internal/services/queue` in ~hundreds of lines — no need for River's driver/sqlc/notifier/plugin surface.
2. Reviving vendored fork (`forge/api/queue/`, `river_queue` table) would add second schema line, second claim path, and a **third engine** to two already fighting over `operations` (LF-6).
3. Forge insight worth keeping verbatim: **steal ≠ retry accounting** (`queue/store.go:49-67`, `retry_accounting_test.go`) — better than River's uniform attempt cost.

**Target single-writer contract (synthesis §5 + phase-05 §3):**

| Concern | Winner | Action |
|---------|--------|--------|
| Durable execution (compose, backup, power, install, file ops, periodic) | `queue.Service` (`job_queue`, leased) | Port remaining `operation.Dispatch*` kinds onto `queue.DispatchIdempotent`; register handlers once — queue is already canonical for compose (`queue.go:29-41` + `operation/service.go:44-47` deprecated), keep that boundary |
| `operations` reads / desired-vs-observed UX | `operation.Service` reduced to **read-model + `Touch`/reaper** of projections queue already writes (`queue/store.go:83-109`) | Delete `operation.Dequeue/worker/retry` machinery (`operation/service.go:157-308`); unify step IDs per LF-5 |
| Power endpoint | One branch | Remove `OperationService`-first/`Queue`-fallback split if still present (`phase-05: handlers_servers.go:914-949` pattern) |
| Periodic | `PeriodicJobScheduler` + one guard | Add advisory-lock or `forge_leader` TTL election gating `tick` so one replica schedules (fixes LF-3) without adopting full River |
| Event pipeline | Tx-bound outbox | Add `PublishTx`, wire consumers to Relay, stop relay marking dispatched on zero handlers (fixes AF-1) |
| Fixes owed regardless | — | CAS cancel `WHERE status NOT IN (...)` + `JobSetStateIfRunning` equiv (LF-1); delay column `available_at` not sleep (LF-2); wrap store transitions in one tx (LF-4); unify `forge-op:` vs `forge-job:` namespace + return `UniqueSkippedAsDuplicate` signal (LF-5); `context.WithoutCancel` heartbeat (LF-8); drop goroutine de-durabilization; elect one leader gating maintenance (AF-2) |

Minimum P0 before any scale-out: **LF-1 + LF-3 + LF-4 + AF-1 + AF-2 + AF-3 fence enforcement** — all are correctness bugs today that only survive behind single-instance deployment and silent 5s jitter.

---

## 6. Coverage checklist

- [x] `forge/api/internal/services/queue/queue.go` (lease 30s:107, keepLease:213-226, Cancel:247-254, idempotency:237-244, workers:144-163)
- [x] `forge/api/internal/services/queue/store.go` (SKIP LOCKED:52-62, retrySQL:64-67, non-tx Dequeue/Ack/Fail/Retry:69-149, Enqueue tx:21-47, heartbeat:151-155)
- [x] `forge/api/internal/services/queue/periodic.go` (RFC3339Nano key:142, per-replica ticker:62-147, in-memory nextRun)
- [x] `forge/api/internal/services/operation/service.go` (164-283 fake backoff:288-304, touchLoop WithWithoutCancel:490-505, Dequeue worker/reaper:157-232)
- [x] `forge/api/internal/services/operation/store.go` (SKIP LOCKED Dequeue:36-48, UpdateStatus CTEs:142-198, Cancel guard:210-214, ReapStale/Touch:216-252)
- [x] `forge/api/internal/eventstore/store.go` (Publish no Tx:113-133, OutboxPublisher:205-222)
- [x] `forge/api/internal/eventstore/outbox.go` (Subscribe:39-43, Relay lease:103-105, zero-handler deliver:174-207, mark dispatched:160-171)
- [x] `forge/api/internal/http/server.go:148` EventRelay unused
- [x] `forge/api/cmd/api/main.go` (30 Start() daemons:521-1304, eventRelay wiring:370/1167/1519, periodicScheduler:542-548, queue 5 workers:467)
- [x] `forge/api/cmd/api/river.go:13` deprecated stub
- [x] `forge/api/queue/leader.go:23` Elector unwired (only jitter comments in reconciler/heartbeatmonitor/observability)
- [x] `forge/api/queue/queuedriver/queuepgx/*` (riverpgxv5 SKIP LOCKED:122, leaderAttemptElect:577)
- [x] No product code modified

---

## 7. Reference links (reconciliation deltas)

- `phase-05/subagent-02-river-queue.md` 16-row matrix → re-verified row-for-row; only change is LF-7 fallback closed but transactional guarantee still absent — treat as **pattern-persists**.
- `phase-05/subagent-05-architecture-synthesis.md` 14-row matrix C1-C14 → all rows re-confirmed; AF-1/AF-2 evidence chains lengthened with exact line numbers above; AF-3 (fence) out of scope for this subagent but noted as remaining P1.
- Grep deltas: `PublishTx|WithTx` still zero; `Relay.Subscribe` still test-only; `EventRelay` still write-only struct field; `Elector|pg_advisory` still only in dead fork + `store_failover.go`.

*Generated by subagent 18/20 — parallel reverification lane. No files under `forge/` were modified.*
