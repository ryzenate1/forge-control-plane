# Phase 5 — Subagent 2: Durable Queue / Job Model (River vs Forge's queue+operation duality)

Cluster: `river` (reference/operations/river, master) · secondary lens: Nomad eval broker
Dimension: job model — persistence, queues, retries/backoff, uniqueness, scheduling/cron, dead jobs, cancellation/snooze, heartbeat, workers, transaction boundaries, advisory locks, idempotency, observability.

**Verdict up front:** Forge does **not** need River. Forge *has* a River-shaped queue (`internal/services/queue` over `job_queue`) plus a second engine (`internal/services/operation` over `operations`) that together duplicate roughly the easy 40% of River (insert / SKIP LOCKED claim / retry) while omitting nearly all of the hard 60% (leader-elected maintenance, retention/cleanup, sound cancellation, error history, uniqueness feedback, tx-bound enqueue, observability). The single-writer contract should be: **`queue.Service` is the only execution engine; `operation.Service` becomes a read/projection API.** Details and evidence below.

---

## 0. Wiring reality check (what is actually live)

- `forge/api/cmd/api/river.go:13-20` — the file is a deprecated stub: *"River is not the canonical durable queue… not wired in main.go."* Only `runRiverMigrations` survives; no River client exists.
- A full vendored River fork exists at `forge/api/queue/` (client.go, producer.go, leader.go, queuedriver/queuepgx…) with `doc.go:1-5`: *"Deprecated… not wired in cmd/api/main.go."*
- `forge/api/migrations/137_river_queue_features.sql:25-35` created a `river_queue` table; it receives **no writes** from any wired code path (only references are in the dead fork, e.g. `forge/api/queue/queuedriver/queuepgx/queue_pgx_driver.go:498-581`). Migration header at 137:1-5 marks it backward-compat only.
- Live wiring is `cmd/api/main.go:466-467` (`queue.NewPostgresStore` + `queue.New(qStore, 5)`), handlers registered at main.go:482-497 (compose), 1020-1023 (backup create/restore), 1172-1179 (retention/cert-renewal); periodic scheduler at main.go:542-546; shutdown at main.go:1656-1657.
- The power HTTP endpoint prefers `OperationService` and falls back to `QueueService` (`internal/http/handlers_servers.go:914-949`) — two engines behind one endpoint.

So the question "does river_queue get used?" → **no**; the live duality is `job_queue`+`operations`, both hand-rolled.

---

## 1. Comparison matrix

| # | Dimension | River | Forge | Assessment |
|---|-----------|-------|-------|------------|
| 1 | Persistence & state machine | `river_job` 8-state enum + DB-enforced invariants (`finalized_or_finalized_at_null`, priority/kind length CHECKs) — riverpgxv5/internal/dbsqlc/river_job.sql:1-38 | `job_queue.status` free text, 5 states — migrations/057_a_job_queue.sql:4; plus mirrored rows in `operations`/`operation_steps`/`operation_attempts` (092_durable_operations.sql:23-43, 149_complete_durable_operations.sql:6-37) | Forge mirrors one row into four tables; River needs exactly one |
| 2 | Claim query | Batch fetch sized to free worker slots, `FOR UPDATE SKIP LOCKED`, priority ASC ordering, `attempt=attempt+1` and `attempted_by` audit appended atomically — river_job.sql:200-237 | Single-row CTE `LIMIT 1 FOR UPDATE SKIP LOCKED`, also steals expired leases (`running AND locked_until<NOW()`) — internal/services/queue/store.go:52-62 | Same core primitive; River batches, Forge claims 1/job/sec max per worker on empty polls |
| 3 | Lease vs rescuer | No lease column. Claim commits state='running'; crash recovery via `RescueStuckJobsAfter` horizon (default 1h, must exceed JobTimeout) — client.go:346-364, maintenance/job_rescuer.go | `locked_until` lease (30s) + heartbeat every lease/3 — queue/queue.go:107,213-226; store.go:151-155; columns from 092_durable_operations.sql:3-5,21 | Forge's lease recovers crashes in ~30s vs River's 1h, but requires a live ticker per job |
| 4 | Retry accounting on lease steal | Steal = real attempt consumed (claim increments attempt, river_job.sql:222); rescuer appends "Stuck job rescued" error but does not refund — job_rescuer_test.go:182-185 | Explicit contract: dequeue must never touch retry_count; `Retry()` is sole writer (+1 per observed failure) — store.go:49-67, regression-tested in retry_accounting_test.go:8-27 (Phase-1 F-18) | Forge's semantics are *better* for infra-vs-job failure separation; keep this contract |
| 5 | Retry policy/backoff | Pluggable `ClientRetryPolicy`; default attempt^4 seconds ±10% jitter, overflow-safe to ~292y — retry_policy.go:49-103; reschedule via scheduled_at, workers never block | Fixed `2^min(retryCount,6)`s cap 64s (queue.go:184-186,205-207); operation svc doubles BaseBackoff capped MaxBackoff=30s **with a blocking sleep inside the worker slot** — operation/service.go:288-304 | Forge blocks a worker during backoff and has no jitter (thundering herd on mass failure) |
| 6 | Uniqueness/idempotency | `unique_key bytea` + `unique_states bit(8)` partial unique index enforced inside INSERT … ON CONFLICT DO UPDATE; returns `unique_skipped_as_duplicate` flag — river_job.sql:29,314-320; default states rivertype/river_type.go:634-643; advisory-lock fallback documented client.go:96-112 | Deterministic SHA1-UUID ID + `ON CONFLICT DO NOTHING` across three tables — queue.go:237-244, store.go:26-46; unique indexes idx_job_queue_idempotency (092:16-17) and partial (137:15-17). Caller cannot tell insert vs dedupe | River gives callers a signal; Forge silently returns the old row |
| 7 | Delayed/scheduled jobs | First-class `scheduled`/`retryable` states promoted by the leader-run JobScheduler when due — river_job.sql:538-617, client.go:977; `InsertOpts.ScheduledAt` sets state at insert (client.go:1783-1793) | `available_at` filter inside the claim WHERE clause (store.go:54). Migration 137 added `scheduled_at` (137:10) that no Go code reads | Forge works for delays but conflates "scheduled" and "retry-wait" |
| 8 | Cron / periodic | Periodic enqueuer runs **only on the elected leader** (`river_leader` unlogged table, TTL election, LISTEN/NOTIFY resign) — client.go:987,1042-1048; leadership/elector.go:26-31; dbsqlc/river_leader.sql:1-60; in-memory schedule honestly documented as approximate (periodic_job.go:65-73); durable periodic jobs exist as records (rivertype/river_type.go:583-596) | `PeriodicJobScheduler`: per-process 1s ticker, in-memory nextRun, fires `go execute()` — queue/periodic.go:62-147; registered hourly/daily at main.go:542-546 | **Every API replica runs its own scheduler** → duplicates; see LF-3 |
| 9 | Dead jobs & retention | Terminal states cancelled/completed/discarded carry `finalized_at`; per-attempt error history `errors jsonb[]` (rivertype/river_type.go:75-77,264-282); JobCleaner deletes past retention (24h completed/cancelled, 7d discarded) — client.go:114-136, maintenance/job_cleaner.go, JobDeleteBefore river_job.sql:155-175 | Terminal `failed` kept forever; single `error` string overwritten each attempt (store.go:118); no cleaner, table grows unbounded | Forge has no error history and no GC |
| 10 | Cancellation | `JobCancel`: `FOR UPDATE`, refuses final states; running jobs get metadata `cancel_attempted_at` so the rescuer finishes cancellation instead of resurrecting them — river_job.sql:40-81 and guarded completion JobSetStateIfRunningMany river_job.sql:619-698 | `Cancel` cancels local ctx if active, else `Fail("job cancelled")` — queue.go:247-254; `Fail` UPDATE has **no status guard** (store.go:113-130); operation svc guards terminal states (store.go:203-207) but its Cancel races the same way | See LF-1 — unsound |
| 11 | Snooze | `JobSnooze(d)` reschedules without consuming an attempt (attempt decremented) — error.go:43-44, jobexecutor tests :401-431 | Nothing equivalent | Missing feature, minor |
| 12 | Transaction boundaries | `InsertTx` documented contract: enqueue atomically with business data so jobs never run against uncommitted state — client.go:1831-1854 | Enqueue is internally transactional (store.go:21-47) but every call site dispatches *after* its own business commit (e.g. dbbackup/service.go:257-275 creates restore record then enqueues; on enqueue failure falls back to a naked goroutine :277-284). Worse: Dequeue/Acknowledge/Fail/Retry are multi-statement **non-tx** sequences (store.go:69-149) | See LF-4/LF-7 |
| 13 | Workers/fetch loop | Producer-per-queue, batch fetch to free slots, LISTEN/NOTIFY wake with FetchCooldown 100ms / FetchPollInterval 1s fallback, `PollOnly` for PgBouncer txn pooling — client.go:44-57,307-320,1061-1172 | N goroutines each polling `Dequeue(LIMIT 1)` on 1s tick (queue.go:144-163) **plus** 5 operation workers polling `operations` on 1s tick (service.go:202-232) → ≥10 QPS idle floor, two engines | Functional at Forge scale; wasteful and split |
| 14 | Multi-instance safety | All maintenance (cleaner, rescuer, scheduler, periodic, reindexer, queue cleaner) gated by leader election; election uses `pg_advisory_xact_lock` fallback + TTL row — client.go:948-1048, elector.go, pg_misc.sql:2 | Workers safe via SKIP LOCKED; stale reaper runs on **every** instance but is idempotent-ish (SKIP LOCKED batch, service.go:169-188, store.go:209-237); periodic scheduler has **no** cross-instance guard | See LF-3 |
| 15 | Observability | Events (EventKindJobSnoozed etc., event.go:26-43), metrics hooks `HookMetricEmit` incl. fetch duration/count (rivertype/river_type.go:238-249,352-401), job statistics QueueWaitDuration/RunDuration (jobexecutor/job_executor.go:150-152), `attempted_by[]` audit trail (river_job.sql:224-231) | Rows in tables; `ListPending`/`GetJob` accessors (store.go:157-197); nothing else | Blind spot |
| 16 | Secondary lens: Nomad | Nomad EvalBroker: entirely in-memory, leader-managed, Ack/Nack with nackTimeout + deliveryLimit → `_failed` queue, at-least-once — orchestration/nomad/nomad/eval_broker.go:43-53,63-90 | Forge's `operation.Service` is structurally the same animal (ephemeral engine, poll-based, in-process cancel map) except Nomad keeps the broker ephemeral *on top of* Raft-durable evals, while Forge persists ops to Postgres anyway | Confirms the pattern: an in-memory broker belongs above a durable store, not beside a second SQL writer |

---

## 2. Logic findings

### LF-1 — Cancellation can regress terminal jobs and loses races both directions
`queue.Service.Cancel` → `store.Fail` issues `UPDATE job_queue SET status='failed' … WHERE id=$1` with **no status predicate** (queue/queue.go:247-254 → store/store.go:113-130). Consequences:
- Cancelling an already-completed/failed job flips it to failed (audit trail corrupted).
- Cross-instance race: instance B is executing; instance A cancels → status='failed'; B's handler succeeds → `Acknowledge` unconditionally sets 'completed' (store.go:99). The cancel silently evaporates.
Compare River: `JobCancel` locks the row, only transitions non-final states, and for running jobs writes `cancel_attempted_at` metadata which the completer/rescuer honor later (river_job.sql:40-81,666-681). Operation.Service's Cancel at least guards terminals (`AND status NOT IN ('succeeded','failed','cancelled')`, operation/store.go:203-207) — but its handler-success write (`UpdateStatus(StatusSucceeded)` service.go:307) still overwrites a concurrent cancel because there is no compare-and-set between running→cancelled and running→succeeded.

### LF-2 — Operation-service backoff is fake and starves workers
On failure, the worker computes backoff and **sleeps inside `process()`**, blocking one of 5 worker slots up to MaxBackoff=30s, then marks the op `retrying` (service.go:283-306). But `Dequeue` selects `status IN ('queued','retrying')` and sorts `'retrying' FIRST` (operation/store.go:42-45) — the instant the op is marked retrying it is the highest-priority candidate for any worker's next 1s poll. Net effect: ≈one poll-interval of actual delay, paid for with a parked goroutine. There is also no `available_at`/`next_retry_at` column on `operations` at all (092_durable_operations.sql:23-36). River solves this with state+scheduled_at promoted by the scheduler (river_job.sql:538-558); Forge's own `job_queue.retrySQL` already does it right with `available_at=$3` (store.go:64-67).

### LF-3 — In-memory periodic scheduler multiplies across replicas
`PeriodicJobScheduler.tick` keeps `nextRun` in process memory and spawns `go execute()` per due job (periodic.go:109-126). The dedup key is `periodic:<id>:<scheduledFor RFC3339Nano>` (periodic.go:142) where `scheduledFor` derives from each replica's own `nextRun` seeded at its own start time — two replicas therefore produce different keys for the same logical run and both enqueue. There is no leader election anywhere in Forge (contrast River's elector + `river_leader` TTL row, elector.go:26-31, river_leader.sql:1-60; and Nomad's leader-only broker, eval_broker.go:47-48). Safe today only because deployments are single-API-instance; the moment main.go runs twice, backup.retention fires N times hourly.

### LF-4 — Non-transactional multi-table transitions tear state
Each lifecycle method in the queue store runs 1–4 independent statements on the pool:
- `Dequeue`: update job_queue, then operations, then operation_steps, then insert operation_attempts (store.go:73-94).
- `Acknowledge`: four statements (store.go:98-111); `Fail`: four (113-130); `Retry`: four (132-149).

A crash or error mid-sequence leaves `job_queue` advanced while `operations` lags (e.g. step stuck 'running' with no attempt row). Only `Enqueue` uses a transaction (store.go:21-47). River centralizes completion in one guarded bulk statement (`JobSetStateIfRunningMany`, river_job.sql:619-698) precisely so state can't tear.

### LF-5 — Idempotency namespace fragmentation defeats replay protection
The same client `Idempotency-Key` header hashes to different UUIDs depending on engine:
- `uuid.NewSHA1(...,"forge-op:"+kind+":"+key)` — generic dispatch (operation/service.go:332-343)
- `uuid.NewSHA1(...,"forge-op:"+key)` — DispatchPower, no kind in hash (service.go:364-369)
- `uuid.NewSHA1(...,"forge-job:"+key)` — queue dispatch (queue.go:238-240)
- derived step IDs differ too: `"step-"+op.ID` (operation/store.go:28) vs `"forge-operation-step:"+job.ID` (queue/store.go:40)

A client retrying a power op that landed on the queue path after landing on the operation path creates two distinct jobs for one key. Additionally `ON CONFLICT DO NOTHING` everywhere means the caller can't distinguish "inserted" from "deduped" — River returns `UniqueSkippedAsDuplicate` for exactly this (river_job.sql:320, rivertype/river_type.go:41-45).

### LF-6 — Two writers, divergent conventions, same `operations` table
Both engines write `operations`/`operation_steps`/`operation_attempts` with different ID schemes and different attempt-insertion paths (queue store Dequeue direct-insert at store.go:89-94 vs operation UpdateStatus CTE at operation/store.go:146-158; step IDs per LF-5). `AttemptCount` (operation/store.go:193-201) counts rows written by either writer, so retry budgets mix domains. This is the concrete sense in which Forge "duplicates ~40% poorly": the projection table that was supposed to unify observability is itself dual-written.

### LF-7 — Silent de-durabilization fallback
dbbackup `Restore` creates the restore record, tries the queue, and on enqueue error falls back to `go runRestore(...)` (service.go:262-284) — losing retries, heartbeats, and visibility with no operator-facing signal beyond a log line. River-style discipline would either fail the request or use an outbox row.

### LF-8 — Heartbeat dies with the job context
`keepLease` ticks `s.store.Heartbeat(ctx, …)` using the **worker/job context** (queue.go:198,213-226). On graceful `Stop()` the context cancels, heartbeats stop, and leases lapse 30s later — another instance steals jobs whose handlers may still be draining. Contrast the sibling fix already present in operation.Service's `touchLoop`, which deliberately uses `context.WithoutCancel` (service.go:500-504): the queue path never got the same treatment.

---

## 3. Does Forge need River? Which single-writer contract wins?

**Forge does not need River** as a dependency:
1. The hard parts Forge actually lacks (leader-elected periodic scheduling, retention cleanup, sound cancellation, error history) could be added to `internal/services/queue` in a few hundred lines — they do not require River's driver abstraction, sqlc layer, notifier, or plugin system.
2. Re-introducing the already-vendored fork (`forge/api/queue/`) would add a second schema line (`river_job`/`river_leader` migrations), a second claim path, and a third engine to the two that already fight over `operations`.
3. Forge's best idea — steal ≠ retry (LF-free, store.go:49-67, regression-tested) — diverges from River's uniform attempt accounting (river_job.sql:222) and is worth keeping.

**But the current contract is not "single-writer" yet.** Recommended target:

| Concern | Winner | Action |
|---|---|---|
| Durable execution (compose, backup, power, install, file ops, periodic) | `queue.Service` (job_queue) | Port remaining `operation.Dispatch*` kinds (power/install/file/restore, operation/service.go:449-485) onto `DispatchIdempotent`; register their handlers once |
| `operations` reads / desired-vs-observed generation UX | `operation.Service` reduced to read-model + `Touch`/reaper ownership of the projections the queue already writes (store.go:83-109) | Delete `operation.Dequeue`/worker/retry machinery (service.go:157-308) |
| Power endpoint | One branch | Remove the OperationService-first/Queue-fallback split at handlers_servers.go:914-949 |
| Periodic | `PeriodicJobScheduler` + one guard | Add a Postgres advisory-lock or `river_leader`-style TTL election so one replica schedules (fixes LF-3) without adopting full River |
| Fixes owed regardless | — | State-guarded CAS cancel (LF-1), retryable delay column instead of sleeping workers (LF-2), wrap store transitions in one tx (LF-4), unify idempotency namespace (LF-5), background-context heartbeat (LF-8), drop goroutine fallback (LF-7) |

That is the honest reading of "duplicates ~40% poorly": the duplicated 40% should win because it is already canonical, regression-tested, and simpler than reviving River; the missing 60% should be ported piecemeal, prioritizing LF-1/LF-3/LF-4 which are correctness bugs today.
