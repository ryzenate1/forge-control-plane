# Subagent 08 — Orchestration Preparation Audit (Placement / Scheduler / Queue / Operation / Eventstore / Reconciler / Recovery / Heartbeat)

**Phase:** 110 Phase 01 — Agent 08/10 (Orchestration slice)  
**Date:** 2026-08-24  
**Scope:** `forge/api/internal/placement/*`, `forge/api/internal/scheduler/*`, `forge/api/internal/services/scheduler/*`, `forge/api/internal/services/reservations/*`, `forge/api/internal/services/queue/*`, `forge/api/internal/services/operation/*`, `forge/api/internal/eventstore/*`, `forge/api/internal/events/*`, `forge/api/internal/services/{fencing,recovery,heartbeatmonitor,reconciler,evacuationplanner,replicamanager,drain,nodeautoscale}`, `forge/api/cmd/api/{main.go,river.go}`, `forge/api/queue/leader.go`, `forge/api/internal/runtime/runtime.go`, `beacon/internal/runtime/factory.go`, `forge/api/internal/http/server.go`  
**Method:** file:line inspection, `grep -rn` wiring checks, SQL `FOR UPDATE SKIP LOCKED` / `ON CONFLICT DO NOTHING` review, `main.go` `Start()` inventory, cross-parity check against `audits/FINAL_PARITY_AUDIT.md §10` (19 rows) + AF-1..AF-3 + `audits/phase-05/synthesis.md`. No code modified.

---

## 0. Intent for Phase 01 readers

This is a **preparation-only** inventory. Every row is preparation to consolidate execution to a single writer, make the lease-outbox real, elect a leader before duplicating work, and tag tenant before data volume makes migration painful. File:line citations are load-bearing for the execution plan §7-§9.

---

## 1. Inventory — what exists where

### 1.1 Placement

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/placement/engine.go:14-19` | `Engine{scorer, checker, logger, mu sync.Mutex}` | Orchestrator. `mu` serializes all placements on this process. |
| `forge/api/internal/placement/engine.go:38-39,79-81` | `Place` + `PlaceAll` both `mu.Lock()` | Single global mutex — throughput ceiling (Nomad uses lazy iterators + power-of-two, not a lock). |
| `forge/api/internal/placement/constraints.go:59-63` | `bonus += 1e12` / `bonus -= 1e10` | Soft-constraint bonus dwarfs any base score. |
| `forge/api/internal/placement/strategy.go:91-104` | `LeastLoadedScorer.Score = sum availRatio (≤3)` | Base score bounded ≤3. Same bound used under `+1e12` / `-1e10` → constraint count dominates. `BinPackScorer:106-131` [0,1], `SpreadScorer:137-147` `1/(1+ServerCount)` [0,1], `RandomScorer:149-173` `sync.Mutex+rng` non-deterministic. |
| `forge/api/internal/placement/strategy.go:175-183` | `ensureCapacity` | Hard capacity gate; negative request rejected. |
| `forge/api/internal/placement/replica.go:48-100` | `PlaceReplicas` | Per-replica capacity deduction via `workingCandidates` copy; anti-affinity `count*0.1` penalty `replica.go:171-175` second spread mechanism distinct from `SpreadScorer`. |
| `forge/api/internal/placement/replica.go:102-148` | `placeSingleReplica` | Hard `RequiredNode` short-circuit, else `FilterByConstraints` then `scoreReplicaCandidate`. |
| `forge/api/internal/placement/explain.go:28-87` | `ExplainPlacement` | Re-derives filter+score for UI — uses `checkSingle` per constraint, not same `Score()+CheckSoft` path (minor drift). |

**Placement verdict vs FINAL_PARITY §10 rows 1-3:** soft-bonus overflow (P1), spread misnamed (anti-colocation only, row 2), no blocked-eval/reschedule policy (row 3/4). All three open since phase-05.

### 1.2 Scheduler (control-plane wrapper over `placement.Engine`)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/scheduler/service.go:34-43,56-77` | `Scheduler{store, engine, publisher, predictiveScorer, constraintScheduler, reservations, mu, metrics}` | Single scheduler instance constructed at `main.go:375-391`. |
| `forge/api/internal/services/scheduler/service.go:88-166` | `PlaceServer` | Normalizes request, resolves region slug → ID (`resolveRegionID:790-802`), `FilterNodes` → `ScoreNodes` → sorted, then **reservation loop** trying nodes in score order until one reserves. |
| `forge/api/internal/services/scheduler/service.go:168-231` | `FilterNodes` | Region-enabled check, online/draining/maintenance gate, `NodeCapacitySnapshot` zero-capacity reject, `HasCapacity`, `StorageLocality`/`Runtime` filters, then `ConstraintScheduler.EvaluateConstraints` delegation. |
| `forge/api/internal/services/scheduler/service.go:267-327` | `ScoreNodes` | Builds `placement.Candidate` per node via `nodeToCandidate:703-737`, calls `engine.PlaceAll`, then **post-hoc locality & preferred tweaks:** `+1e9` preferred `service.go:307`, `±1e10` locality mismatch `318`, `+1e8` locality match `321`, predictive `TrendScore/Affinity` `313`. Same overflow family as placement soft bonus. |
| `forge/api/internal/services/scheduler/constraints.go:62-74` | `EvaluateConstraints` | Extra scheduler-local constraints (region/node_id/name) — separate constraint system from `placement.ConstraintChecker` → two constraint vocabularies. |
| `forge/api/internal/scheduler/scheduler.go:96-108` | `Scheduler` interface (K3s/Nomad/Docker) | Container-orchestrator abstraction — **not wired into main `PlaceServer` path** (K3s/Nomad adapters exist but instance traffic uses `placement.Engine` + `daemon.Client`). Mark as future seam, not current debt. |

### 1.3 Reservations (the good single-writer primitive)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/reservations/service.go:21-27,29-45` | `Manager{store, publisher, mu, metrics, cancel}` | Reservation lifecycle + metrics + `Start` sweeper. |
| `forge/api/internal/services/reservations/service.go:46-70` | `Start` 60s `ExpireReservations` ticker | Per-process ticker — no leader gate (part of AF-2). Startup sweep also in `main.go:379-384` (`ExpirePlacementReservations` + `RecoverPendingPlacementIntents`). |
| `forge/api/internal/services/reservations/service.go:78-137` | `Create/Confirm/Cancel/Expire` | All emit `OutboxPublisher` events; reservation row is the real capacity lock before instance row exists. Keep verbatim. |

### 1.4 Queue — the intended single writer

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/queue/queue.go:15-41` | `JobType` constants — canonical durable lifecycle is `JobCompose*` 6 types (deploy/update/delete/start/stop/restart) + periodic `backup.retention`/`cert.renewal` | Power ops explicitly **not** queue-canonical (deprecated comment `JobServerStart:19-20`). |
| `forge/api/internal/services/queue/queue.go:101-110` | `New(store,5)` → `lease 30s, jobTimeout 30m, workerID uuid` | `queue.go:107` lease 30 s → heartbeat `lease/3 =10s` at `queue.go:214`. |
| `forge/api/internal/services/queue/queue.go:124-142,144-163` | `Start` 5 workers `worker()` loop `Dequeue("")` every 1s on empty | Idle QPS = 5 workers × 1 Hz = 5 polls/s per replica; sibling `operation` adds another 5 (total ≥10). |
| `forge/api/internal/services/queue/queue.go:165-211` | `process` + panic recovery | Single backoff formula `1<<min(rc,6) s` = 1,2,4,8,16,32,64 s cap `queue.go:184-185,205`. Correct via `available_at`. Heartbeat bug: `keepLease:213-226` uses `jobCtx` (which is `ctx.WithTimeout(handlerTimeout)`) — on `Stop()` `ctx.Done()` also cancels heartbeats while handler may still drain → lease lapses 30 s then stolen (sibling `operation/service.go:500-504` fixed with `context.WithoutCancel`). |
| `forge/api/internal/services/queue/store.go:17-47` | `Enqueue` **in Tx** (`BEGIN; job_queue + operations + operation_steps; COMMIT`) | Correct — only `Enqueue` is tx-bound. |
| `forge/api/internal/services/queue/store.go:52-62` | `dequeueSQL` CTE `LIMIT 1 FOR UPDATE SKIP LOCKED` steals `pending+available_at<=NOW()` + `running+locked_until<NOW()` | Single-row batch vs River `batch LIMIT $2`; comment `store.go:49-51` documents steal≠retry contract (better than River). `available_at` correctly gates retry backoff. |
| `forge/api/internal/services/queue/store.go:69-96` | `Dequeue` 4 independent `Exec`s (`job_queue` + `operations` → `running` + `operation_steps` + `operation_attempts INSERT`) | Non-tx tear — crash mid-sequence leaves `job_queue=running` while projection still `queued`. |
| `forge/api/internal/services/queue/store.go:98-130` | `Acknowledge/Fail` multi-statement + `Retry` sole `retry_count+1` writer `retrySQL:66-67` | `Fail:113-130` has **no `WHERE status IN (pending,running,retrying)`** guard → cancel regresses `completed→failed`. `Retry` is sole legitimate `retry_count` writer — keep. |
| `forge/api/internal/services/queue/store.go:151-155` | `Heartbeat` `UPDATE ... WHERE status='running' AND locked_by=$2` | Correct CAS on heartbeat. |
| `forge/api/internal/services/queue/periodic.go:62-80,82-107,109-126` | `PeriodicJobScheduler` in-memory `nextRun` + 1 s ticker | `periodic.go:142` key `periodic:<id>:<RFC3339Nano>` derived from **each replica's own `nextRun`** (seeded `schedule.Next(now)` at own start `periodic.go:77-80`). Two replicas produce different keys for same logical period → dedupe fails (LF-3). `queue/leader.go:23-133` elector exists to gate this but is **unwired** (`river.go:13` deprecated). `main.go:543-548` two periods (`hourly-backup-retention:1h`, `daily-cert-renewal:24h`) run duplicated. |
| `forge/api/internal/services/queue/queue_test.go:77-94,96-110` | Regression tests | Steal≠retry + idempotent stable ID. Keep. |

### 1.5 Operation — the second execution engine (to be collapsed to read-model)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/operation/service.go:17-54` | `Status` + `OperationType` — `OpServer*` + `OpBackup*` + `OpFile*` + deprecated `OpCompose*` | Compose ops already stubbed `service.go:398-405` ("dead dispatcher was second engine"). |
| `forge/api/internal/services/operation/service.go:73-88,100-108` | `Store` iface + `DefaultConfig: MaxWorkers 5, Poll 1s, MaxRetries 3, BaseBackoff 1s, MaxBackoff 30s` | |
| `forge/api/internal/services/operation/service.go:157-188` | `Start` 5 workers + `reaper` ticker 60 s `ReapStale 5m` | Same idle QPS problem as queue; reaper threshold vs install `15m` `main.go:579-601` leaves 5-15 m window for duplicate work. |
| `forge/api/internal/services/operation/service.go:202-232` | `worker` `Dequeue` loop | Identical 1 s poll. |
| `forge/api/internal/services/operation/service.go:234-308` | `process` — heartbeat `touchLoop:490-507`, panic recovery, `UpdateStatus(running)`, `AttemptCount`, handler, then **`blocking sleep` backoff `283-304`** `for i<attemptCount: backoff*=2; NewTimer(backoff); select ctx.Done / timer.C → UpdateStatus(retrying)` | **Fake backoff.** `store.go:42-45` `Dequeue` orders `retrying FIRST` (`CASE WHEN status='retrying' THEN 0 ELSE 1 END`) so "delay" is ~1 poll interval while one of 5 slots is parked up to 30 s. No `next_retry_at` column (queue has `available_at`). Thundering herd, no jitter (`operation/service.go:282-295` vs River `attempt^4+jitter`). |
| `forge/api/internal/services/operation/service.go:332-396` | `dispatch` + `DispatchPower` | Two idempotency namespaces: `dispatch:342` `"forge-op:"+kind+":"+key` vs `DispatchPower:368` `"forge-op:"+key` (no kind) vs queue `"forge-job:"+key` `queue.go:239` → same client key creates two jobs across engines; `ON CONFLICT DO NOTHING` silent dedupe (no `UniqueSkippedAsDuplicate` signal). |
| `forge/api/internal/services/operation/store.go:19-34,36-65` | `Create` + `Dequeue` `LIMIT 1 FOR UPDATE SKIP LOCKED` `retrying FIRST` | `Create:22 ON CONFLICT DO NOTHING`; `Dequeue:40-48` single-row claim. |
| `forge/api/internal/services/operation/store.go:142-198,216-244,246-251` | `UpdateStatus` switch + `ReapStale` + `Touch` | `ReapStale:223-239` `LIMIT 100` + `Touch:249-251` 30 s `touchLoop` — correctly `WithoutCancel` `service.go:500`. Keep pattern, port to queue heartbeat. |

**Duality verdict (phase-05 `§2 Queue`):** River fork dead (`forge/api/cmd/api/river.go:13-20` stub, `migrations/137_river_queue_features.sql:25-35` orphan `river_queue`), live engines dual-write `operations/operation_steps/operation_attempts` (`queue/store.go:34-46` vs `operation/store.go:19-34`) with divergent ID schemes (`"forge-operation-step:"+jobID` vs `"step-"+opID`). Idle QPS split, attempt counts diverge, idempotency fragmented.

### 1.6 Eventstore / Outbox / Relay (AF-1)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/eventstore/store.go:95-111,109` | `New(pool)`, `pendingSQL` default `WHERE dispatched=false AND failure_count<5 ORDER BY created_at ASC LIMIT $2 FOR UPDATE SKIP LOCKED` |  |
| `forge/api/internal/eventstore/store.go:67-93` | `ClaimPending(limit, claimToken, lease)` `WITH candidates SELECT ... FOR UPDATE SKIP LOCKED UPDATE ... SET claimed_by=token:e.id, claimed_until=NOW()+lease` | Lease math documented `outbox.go:103-105` `(batchSize+1)*eventTimeout = 11*30s = 330s`. Retention indexes `migration.go:32-39` correct. |
| `forge/api/internal/eventstore/store.go:113-133` | `Publish` via `pool.Exec INSERT events dispatched=false` | **No `PublishTx`** (`grep PublishTx` zero hits in `forge/api`). Business commits first, publish after → classic outbox violation. |
| `forge/api/internal/eventstore/store.go:157-189,191-199` | `MarkDispatched/MarkFailed/MoveToDeadLetter/Prune` | `maxFailureCount=5` `store.go:15`. Atomic DLQ move `store.go:175-189`. |
| `forge/api/internal/eventstore/store.go:205-222` | `OutboxPublisher{store, registry}` `Publish: INSERT then registry.Publish` | Write (+1) then synchronous fan-out `events/registry.go:144-191` (`go` per subscriber, `wg.Wait` 30 s). No tx binding; cross-instance delivery impossible. |
| `forge/api/internal/eventstore/outbox.go:39-43` | `Relay.Subscribe(handler)` | **Zero production callers** — only tests `store_test.go:362,395`. |
| `forge/api/internal/eventstore/outbox.go:74-122,124-172,174-218` | `Relay.Start(5s)` → `processBatch → ClaimPending(10)` → `processEvent → deliverWithRetries(maxRetries 3, wait 100ms*2)` → `MarkDispatched` | `outbox.go:102-162` empty handler list returns `nil` → `MarkDispatched` within 5 s. Payload is `map[string]any` decode without schema. Prune `outbox.go:95` 7 d / 30 d. |
| `forge/api/internal/eventstore/migration.go:18-53` | `events` + `events_dead_letter` + `claimed_by/claimed_until` + indexes `pending_created` | Solid primitive, unused end-to-end. |
| `forge/api/internal/events/event.go:18-172,196-205` | `EventType` 60+ constants, `Envelope{ID,Type,Timestamp,Source,ResourceType,ResourceID,CorrelationID,Payload}` | **No `tenant_id`/`project_id` field** — AF-7 tenant-blind. `CorrelationID` plumbing `event.go:207-236` healthy. |
| `forge/api/internal/http/server.go:148` | `EventRelay *eventstore.Relay` wired at `main.go:370,1519` | **Never dereferenced** in server (grep zero). |
| `forge/api/cmd/api/main.go:362,370,1167,1519,1612,1668` | Relay lifecycle | `NewRelay(es,5s)`, `eventRelay.Start(appCtx)`, injected into server Config, `Stop()` on shutdown — but **no `Subscribe`** call in live code. |

**AF-1 CONFIRMED OPEN (HIGH)** — structurally River-quality primitive, functionally write-only audit log at 2× write amp.

### 1.7 Fencing / Recovery / Heartbeat / Reconciler (AF-3, §10 rows 10-11,15)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/fencing/fencing.go:11-18,20-27,29-49` | `Service{store,publisher}` `Handle(EventNodeRecovered → FenceNode)` | `FenceNode:29-49` loops `ListServersForNode` → `Generation++` + 24 h lease → `UpdateServerGeneration`. Fires on **recovery**, not failure (window unfenced). |
| `forge/api/internal/services/recovery/service.go:642-679` | `planServer` second fencing path | `PreviousGeneration` capture, `Generation++` + 1 h lease `645-646`, `CreateReservations(...,"")` `653`, `CreateRecoveryItem` with `FenceGeneration` `658-669`, rollback `errors.Join` `654-656,672-677`. Competes with `fencing.FenceNode` — last writer wins, leases differ 24 h vs 1 h. |
| `forge/api/internal/services/recovery/service.go:335-372,338-403,403-445` | `ExecutePlan` / `ReconcileRecoveryAcknowledgment` / `ReconcilePlan` | `ExecutePlan` requires `BackupRestoreExecutor` (`VerifyAndRestore` never contacts offline source — correct). `ReconcileRecoveryAcknowledgment:403-445` 10 m ack timeout `DefaultRecoveryAcknowledgmentTimeout:21`, releases reservation on timeout, compares `server.ActualState==running` to mark `Restored`. |
| `forge/api/internal/services/heartbeatmonitor/service.go:89-97,99-109,127-159` | `DefaultConfig: 30s warning, 90s offline, 300s unavailable, 2 recovery, 30s interval` | Hysteresis classifier **richer than all six references** (`classify:266-313` consecutive-success, future-timestamp guard, monotonic `normalizeConfig:399-423`). |
| `forge/api/internal/services/heartbeatmonitor/service.go:140-142,187-189` | Jitter comments | `"Jitter 0-5s at start to desynchronize heartbeat evaluation across instances"` / reconciler same — explicitly **not** a leader, only desynchronization. |
| `forge/api/internal/services/reconciler/service.go:174-206,214-275` | `Start` jitter 0-5 s + ticker `DefaultInterval 30s`, `RunOnce` | `RunOnce:226-230` dedupes stale `ReconcilePlan` TTL `PlanTTL=1h` `service.go:42`, hash `snapshotHash` dedupe window `PlanDedupeWindow=30m` `680-702`, destructive-confirm `trigger.go:211-213`. |
| `forge/api/internal/services/reconciler/service.go:486-536,541-580,581-676` | `reconcileNode/Server`, `recoverUnhealthyTargets` | `reconcileServer` diffs desired vs actual, emits `ActualStateChanged` only on transition (F-26 fix). `recoverUnhealthyTargets` caps `MaxRestartAttempts=3` `26`, `RestartCooldown 15m` `28`, decay `RestartAttemptResetAfter 24h` `32` — per-mechanism caps, no cross-controller lease. |

**AF-3 CONFIRMED OPEN (HIGH)** — generation bumps written twice, on wrong edge, enforced **zero** times (`grep generation|fence beacon/internal` zero, `clustermanager/service.go` power dispatch never sends generation, `beacon/server.go:735-842` create/power handlers lack generation field check). Six controllers race on same server (reconciler desired-vs-actual + health-recovery + replicamanager replace + crashdetector + failover + recovery) with only per-controller caps.

### 1.8 Drain / Evacuation / Replica / Autoscale

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/drain/service.go:1-40,47-184` | `Service` durable drain ledger `drain_states` | `BeginDrain:47`, `ProgressDrain:70`, `EndDrain:87`, `Subscriber:155` mirroring `EventNodeDrainingStarted/EvacuationPlanCreated` etc. Ahead of Nomad CE on locality guard. Pacing is global constant `maxConcurrent=2` `evacuationplanner/service.go:188`, not per-workload `MaxParallel`. |
| `forge/api/internal/services/evacuationplanner/service.go:65-96,101-144,188-297,298-394,546-648` | `Service{store,scheduler,publisher,executor,mountStore}` | `Start` resume `ListEvacuationPlansByStatus(Running)` `136-144`, `ExecutePlan` CAS `StartEvacuationPlan` `193-213`, `startAvailableItems` bounded slots, `observePlan` reconciliation `308-343`. `evaluateNode:546-636` storage-locality guard (`local_only → ineligible`) + `ReplacementPolicyProtect` guard + capacity via `findCandidates:460-522` with `reserved` map and `ValidateCapacity:524-544`. Interim until AF-3 serialization via generation. |
| `forge/api/internal/services/replicamanager/service.go:42-56,68-89,100-194,195-359,361-444,446-509,561-652,697-730` | `Manager{store, engine, scheduler, reservations, dispatcher, beaconClient, publisher, logger, mu, metrics, appLocks[64]}` | `DeployApp:143-194` `UpdateReplicaAppStatus(deploying)` → `IncrementReplicaAppGeneration` → `scheduler.PlaceReplicas` → `deployReplicas`. `deployReplicas:195-359` **double reservation** — scheduler already reserves per replica (`scheduler/service.go:403-424`) then `deployReplicas:219` reserves again with `ServerID: app.ID` (semantically an app, not a server). `beaconClient` vs `dispatcher` alternative-backend double-provision comment `service.go:312-329` conditional on `beaconClient==nil`. Per-app `fnv` sharding `service.go:553-559`, `Start` minute-tick `reconcile:697-730` with `RetryFailedPlacements:883-910` forever `+Sleep(100ms)` — no policy record. |
| `forge/api/internal/services/nodeautoscale/service.go:30-39,193-241,274-297,301-336,339-359,373-396` | `Service{store, cloud.Manager, membership, logger, bootstrap, mu, lastScan}` | `evaluate:193-241` `aboveTarget || minNodes` → `Deficit`; cooldown `lastScan` per-policy `222-231`; `leastLoadedNode:274-296`. **Hardcoded AWS** `service.go:391` `DeprovisionNode(ctx, ProviderKind("aws"), nodeID)` — ignores `policy.Provider` (contrast `ProvisionNode:358` which correctly uses `ProviderKind(policy.Provider)`). `worker.go:15-36` `Start` 60s scan for `AutoJoin` only; `ScaleIn:373-396` `StartDrain` then deprovision without drain-completion gate. All three are S-06 / phase-05 F10 still open. |

### 1.9 Runtime honesty seam (FINAL_PARITY §10 row 10.3)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/runtime/runtime.go:9-16` | Control plane lists 7 providers: `docker,containerd,podman,firecracker,kubernetes,lxc,kvm` | |
| `forge/api/cmd/api/main.go:392-399` | Registers all 7 via `NewDockerAdapter/KubernetesAdapter/Firecracker/Podman/Containerd/LXC/KVM` | |
| `beacon/internal/runtime/factory.go:19-31` | Beacon implements **5** — `docker,containerd,podman,firecracker,kubernetes` — **no `lxc/kvm` branches** | |
| `beacon/internal/runtime/` grep | No occurrence of `lxc` in factory; `Capabilities` test-only `firecrackerCapabilities{Snapshot:true}` etc. — `CheckCapability` zero callers | Phantom providers lie across trust boundary; always Docker `beacon/server.go:740-772 body lacks Provider field → mode:docker:841`. Therefore **R-01 open**. |

### 1.10 Daemon inventory (AF-2 prerequisite)

`grep -n "Start(appCtx" forge/api/cmd/api/main.go` (verified):

```
gitOpsController.Start:521, queueSvc.Start:533, opSvc.Start:787, healthCheckRunner.Start:859,
discoverySvc.Start:990, ingressSync.Start:995, cleanupSvc.Start:1016, tmSvc.Start:1028,
lbSvc.Start:1072, buildSvc.Start:1106, buildpackSvc.Start:1109, cronJobSvc.Start:1147,
resMgr.Start:1150, hbm.Start:1151, rec.Start:1156, mig.Start:1157, ep.Start:1158,
mailWorker.Start:1159, whSvc.Start:1160, failSvc.Start:1161, bkWorker.Start:1164,
eventRelay.Start:1167, periodicScheduler.Start:1194, procedureSvc.Start:1196,
replicaMgr.Start:1197, autoSvc.Start:1198, enhancedNotifSvc.Start:1235,
pipelineSvc.Start:1304
= ~27 Start() sites + 2 inline goroutines (session-cleanup:1203, domain startup sync:1030) ≈ 29-30 loops/replica.
```

Coordination observed: jitter comments only (`heartbeatmonitor/service.go:140-142`, `reconciler/service.go:187-189`). No `pg_try_advisory_lock`, no TTL row, no `queue/leader.go:23-133` `LeaderAttemptElect` call in live `forge/api` (dead fork + `forge/api/cmd/api/river.go:13-20` stub). **AF-2 confirmed open.**

---

## 2. Known structurals — confirmation status

### AF-1 Write-only durable events — CONFIRMED OPEN, P0

Chain documented in §1.6. **Impact for Phase 01:** cross-instance automation impossible; fencing/failover/tm/lb observers all in-memory. Any second replica produces split-brain event delivery. **Preparation fix is §7.1 (PublishTx + Relay subscribers).**

### AF-2 No leader; ~30 duplicated daemons — CONFIRMED OPEN, P0

Inventory §1.10. **Impact for Phase 01:** enabling a second API replica before electing a leader duplicates every periodic job (hourly/daily×N), duplicates reconciler/evacuator/failover evaluations, duplicates advisory-lock incident race window beyond the one guarded path. **Preparation fix is §8 (resurrect `queue/leader.go:23-133`).**

### AF-3 Unenforced fencing, wrong edge, duplicated — CONFIRMED OPEN, P0

§1.7 dual writers, wrong edge `EventNodeRecovered`, zero beacon enforcement. **Impact for Phase 01:** a partitioned server can be power-acted from both sides inside the outage window; recovery generation bumps race fencing bumps. **Preparation fix is §7.2 (single Fence + CAS `WHERE generation=$old`).**

### Queue `fake backoff` (FINAL_PARITY §10.2 row 12 / operation) — CONFIRMED OPEN, P1

`operation/service.go:283-304` sleep-in-slot + `operation/store.go:42-45` `retrying FIRST` sort → ~1 s actual delay + parked worker. **Preparation fix is §7.3 (add `operations.next_retry_at` or delete operation dequeuer).**

### Additional structurals inherited

- **AF-4 six controllers, no per-server serialization** — CONFIRMED OPEN, P0/P1, collapses into AF-3 fix (generation as per-server act lock).
- **AF-7 tenant-blind core path** — CONFIRMED OPEN, P1/P2, preparation fix §9.
- **AF-8 dangerous default-Evacuate** `evacuationplanner` + `failover/service.go:181-211` pattern (synthetic policy fallback) — still present via locality default path; fix: default-Notify.

---

## 3. FINAL_PARITY §10 parity matrix — orchestration-only delta check

Re-verified against `audits/FINAL_PARITY_AUDIT.md:255-302`.

| FINAL_PARITY row | Reference vs Forge | This audit's file:line re-check | Change since FINAL_PARITY |
|---|---|---|---|
| §10.1 Scoring overflow | `constraints.go:59` `+1e12` vs Nomad bounded iterators | `constraints.go:59-63`, `scheduler/service.go:306-323` still `+1e9/±1e10/+1e8` | **Unchanged — open P1** |
| §10.1 Spread | attribute-target % vs `SpreadScorer:137-147` `1/(1+count)` + `replica.go:171` `count*0.1` | `placement/strategy.go:137-147`, `replica.go:170-175` unchanged | **Open P2** |
| §10.1 Reschedule | `ReschedulePolicy delay+penalty` vs 60s forever `replicamanager/service.go:883-910` | `replicamanager/service.go:697-719,883-910` unchanged | **Open P0** |
| §10.2 Claim query | River batch vs Forge single `queue/store.go:52-62` | `queue/store.go:52-62` `LIMIT1` still; `operation/store.go:42` `retrying FIRST` | **PARTIAL — open P3** |
| §10.2 Idempotency | River partial unique + signal vs `forge-job`/`forge-op` split | `queue/queue.go:238-240`, `operation/service.go:342,368`, `operation/store.go:28` vs `queue/store.go:40` ID schemes diverge, `ON CONFLICT DO NOTHING` silent | **Open P1** |
| §10.2 Delayed jobs | Leader `scheduled_at` promotion vs `queue available_at` correct / `operations` none | `queue/store.go:54,66`, `operation` none | **PARTIAL — operation P1** |
| §10.2 Periodic leader | `river_leader` TTL vs `queue/periodic.go:62-147` in-memory no guard | `periodic.go:142` diverging keys, `queue/leader.go:23` unwired | **Open P0** |
| §10.2 Cancel CAS | River guarded vs `queue/store.go:113-130` no predicate | `queue/store.go:113-130` `WHERE id=$1` only; `operation/store.go:203-207` guards terminals but `service.go:307` overwrites | **Open P0** |
| §10.2 Backoff | `attempt^4+jitter` via `scheduled_at` vs `queue 2^rc` correct / `operation` sleep | `queue/queue.go:184-205` correct via `available_at`; `operation/service.go:283-304` fake | **Open P1 (operation)** |
| §10.2 Outbox/PublishTx | `River InsertTx` vs Forge no Tx | `eventstore/store.go:113-133,214-222` no `PublishTx` | **UNWIRED — open P0 (AF-1)** |
| §10.2 Relay consumers | Registry vs Relay zero subs | `eventstore/outbox.go:39-43` test-only, `server.go:148` unused | **UNWIRED — open P0** |
| §10.2 Leader election | Nomad/River leader-gated vs jitter only | `queue/leader.go:23-133` dead, `main.go` 30 daemons | **Open P0 (AF-2)** |
| §10.3 Fencing enforcement | NetBird NetworkMap / Incus heartbeat vs write-only generation | `fencing/fencing.go:20-49`, `recovery/service.go:642-650`, `beacon` zero | **FALSE — open P0 (AF-3)** |
| §10.3 Runtime honesty | Incus strict driver check vs phantom LXC/KVM | `runtime/runtime.go:9-16` vs `beacon/factory.go:19-31`, `server.go:740-772` | **FALSE — open P0** |
| §10.3 Overlay mesh | NetBird WG tunnel vs public-only | `store.Store Node` no `TunnelIP`, `crossnode/resolver.go:98-106` public fallback | **MISSING — P1** |
| §10.3 Service discovery | NetBird fan-out vs write-only registry | `servicediscovery/registry.go:91,216-241` stale reaper | **UNWIRED — P1** |

**No new parity row introduced beyond FINAL_PARITY; no formerly-open row fixes itself by re-inspection.** Phase-05 synthesis `§4 Traps` + `§5 activation order` remains the correct ladder (§7).

---

## 4. Files to touch — minimal, ordered by structural fix

**Do not confuse this list with "change now" — it is preparation scope for Phase 01 execution.**

### P0 — before second replica or real evacuation

| Fix | Files |
|---|---|
| **AF-1 PublishTx + Relay** | `forge/api/internal/eventstore/store.go:113-133,205-222` add `PublishTx(ctx, pgx.Tx, Envelope) error`; `forge/api/internal/eventstore/migration.go` add `tenant_id` iff needed; migrate call sites in `services/{fencing,failover,trafficmanager,loadbalancer,recovery,scheduler,reservations}` to `PublishTx`; `forge/api/internal/eventstore/outbox.go:39-43,102-162` add `if len(subs)==0 return nil` guard **not** to `MarkDispatched`; wire `Relay.Subscribe` in `main.go:440-441,946,1026-1077,1236-1243` alongside existing `Registry.Subscribe` |
| **AF-2 Leader election** | Resurrect `forge/api/queue/leader.go:18-148` (+ `Executor` impl `store TryAdvisoryLock` or `forge_leader` TTL table mirroring `reference/operations/river/riverdriver/riverdrivertest/leader.go`); `forge/api/cmd/api/main.go:1150-1198` wrap maintenance daemons (`rec,hbm,ep,mig,resMgr,failSvc,bkWorker,periodicScheduler,replicaMgr`) with `if elector.IsLeader() / advisory lock` guard; keep `SKIP LOCKED` claims leaderless per §C9 |
| **AF-3 Single fencing + CAS** | `forge/api/internal/store/*` add `generation` param to `UpdateServerGeneration` `WHERE id=$1 AND generation=$old` (today unconditional `WHERE id=$1`); `forge/api/internal/services/fencing/fencing.go:29-49` → `Fence{Node,Server}(ctx,id)(newGen,error)` single writer; delete duplicate bump `recovery/service.go:642-650` delegate to fencing; `forge/api/internal/services/clustermanager` + `forge/api/internal/runtime/runtime.go:18-24` thread `generation` into `Target`/`MigrationRequest:136-141`; `beacon/internal/server/server.go:740-772` reject `generation < stored` with `409 stale_generation` |
| **Cancel CAS** | `forge/api/internal/services/queue/store.go:113-130` `Fail/Cancel` `WHERE status IN ('pending','running','retrying') RETURNING` + rowcount; `forge/api/internal/services/operation/store.go:203-207` extend guard to `service.go:307 succeeded` path |
| **Periodic leader gate** | `forge/api/internal/services/queue/periodic.go:62-147` gate `tick:109` on `IsLeader`/advisory lock; fix key rounding `periodic.go:142` `scheduledFor.Truncate(period)` or counter so replicas converge |

### P1 — with or immediately after P0

| Fix | Files |
|---|---|
| **Single-writer consolidation** | `forge/api/internal/services/queue/queue.go:15-41,228-245` port `Dispatch*` for `OpServer*/OpFile*/OpBackup*`; `main.go:552-784` move 6 operation handler registrations (`registerPowerOp` + `OpServerInstall/Reinstall/BackupRestore/File*`) onto `queueSvc`; delete `operation/service.go:157-232,283-308` `Dequeue/worker/process` + blocking sleep; keep `operation/service.go:309-317,73-88,216-244,246-251` read-model + `ReapStale/Touch/Get/ListByResource` (projection `queue/store.go:83-94` already writes) |
| **Fake backoff → delay column** | `forge/api/internal/store/migrations` add `operations.next_retry_at TIMESTAMPTZ`; `store.go:40-48` filter `next_retry_at<=NOW()`; delete `operation/service.go:288-304` timer |
| **Heartbeat `WithoutCancel`** | `forge/api/internal/services/queue/queue.go:213-226` → `context.WithoutCancel(ctx)` like `operation/service.go:500` |
| **Tx-wrap lifecycle** | `forge/api/internal/services/queue/store.go:69-149` `BEGIN/COMMIT` per `Dequeue/Ack/Fail/Retry` |
| **Unified idempotency** | `queue/queue.go:238`, `operation/service.go:342,368`, `queue/store.go:40`, `operation/store.go:28` namespace `forge:{engine}:{kind}:{key}` + `RETURNING (xmax=0)` `created bool` |
| **Placement scoring bounds** | `placement/constraints.go:59-63`, `services/scheduler/service.go:306-323,155-163` clamp `soft ≤±2`, `locality ≤±1`, `preferred ≤+0.5`, expose `ScoreBreakdown` per factor (`placement/explain.go:21-26` already has shape) |
| **Runtime honesty** | `beacon/internal/runtime/factory.go:19-31` **reject** unknown provider `lxc/kvm` with `400 unsupported provider`; or remove `lxc/kvm` from `forge/api/internal/runtime/runtime.go:9-16` until implemented |
| **Nodeautoscale AWS hardcode** | `forge/api/internal/services/nodeautoscale/service.go:391` `ProviderKind("aws")` → `ProviderKind(policy.Provider)`; gate `ScaleIn:373-396` on drain completion before deprovision |
| **Tenant columns (prepare)** | §9 — `events.Envelope:196-205` + `eventstore/migration.go:19-31` + `domain.PlacementRequest:114-129` + `store.RecoveryPlan/Reservation` add `tenant_id` now while small |

---

## 5. Single-writer consolidation plan (detailed)

**Thesis (FINAL_PARITY §12.6 + phase-05 synthesis §4):** two engines dual-writing `operations` is not a missing feature but a double writer. Adding River would make a **third**.

### 5.1 Target contract

| Concern | Winner | Action |
|---|---|---|
| Durable execution (compose, backup, power, install, file, transfer) | `queue.Service` (`job_queue` leased via `locked_until`) | Port every `operation.Dispatch*` (`OpServerStart/Stop/Restart/Kill/Install/Reinstall/BackupRestore/FileArchive/Pull/Download/Upload` at `operation/service.go:28-44,449-485`) onto `queue.Service.DispatchIdempotent("forge-job:"+key, ...)` |
| `operations` read/UX | `operation.Service` as **read-model + reaper** | Delete `operation.Dequeue/worker/process/Cancel/sleep` (`service.go:157-308`); keep `Get/ListByResource/Touch/ReapStale/Cancel` as projection. Queue already writes `operations/operation_steps/operation_attempts` at `queue/store.go:34-46,83-94`; keep that write path authoritative. |
| Power endpoint `handlers_servers.go:875` | Single branch | Remove fallback `OperationService first / QueueService fallback` — call one writer. |
| Periodic | `PeriodicJobScheduler` + leader gate | No new River feature; gate existing `queue/periodic.go:62-147`. |

**Why queue wins over operation:** steal≠retry contract `queue/store.go:49-67` regression-tested, better than River's uniform attempts; `RetrySQL:66-67` delay column already correct; enqueue tx-bound `queue/store.go:21-47`; operation path lacks `available_at` and blocks slots.

### 5.2 Files that must change for consolidation (and only these)

- Remove dequeuer: `operation/service.go:157-232` `Start/worker`, `234-308` `process+touchLoop`, `73` `Dequeue` Store method, `store.go:36-65` `Dequeue` query.
- Port handlers: `main.go:552-784` six `opSvc.RegisterHandler` blocks → `queueSvc.RegisterHandler(queue.JobType(...), adaptPayload)`.
- Fix projection: ensure `queue/store.go:83-94` `UPDATE operations/operation_steps/operation_attempts` covers every new kind with correct `resource_type`.
- Fix idempotency: collapse `forge-job` / `forge-op:{kind}` / `forge-op` into one `forge:{engine}:{kind}:{key}` namespace, return `(id, created bool)`.
- Tests: `operation/service_test.go:100-127` `TestProcessStopsAfterConfiguredAttempts` becomes `TestQueueRetryWithDelayColumn`; `queue/queue_test.go:77-94` extend to cover lease-steal re-claim (already tested `retry_accounting_test.go:8-27`).

### 5.3 Forbidden move

Do **not** introduce River (`cmd/api/river.go:13-20` deprecated fork, `forge/api/queue/*` 20+ files, `137_river_queue_features.sql` orphan). That adds a third engine, a second schema line (`river_job/river_leader`), and re-introduces uniform attempt accounting Forge intentionally fixed at `queue/store.go:49-67`.

---

## 6. Known structurals — preparation summaries for execution (no code changed here)

**Write-only events (§1.6):** paying `INSERT + UPDATE dispatched` per publish for audit-only; automation lives on `events.Registry` worst-case 30 s blocked (`registry.go:167-189`). Fix is PublishTx + Relay as real consumer.

**No leader (§1.10):** all 30 daemons run duplicated; in-memory `lastAction` cooldowns (`failover/service.go:99,446`) process-local. Fix is single `forge_leader` election wrapping maintenance daemons only (keep `SKIP LOCKED` claims leaderless).

**Unenforced fences (§1.7):** fence token generated but never validated end-to-end; wrong edge (recovery not failure). Fix is CAS fence + per-server generation preconditions doubling as controller serialization (AF-4 collapses into AF-3).

**Fake backoff (§1.5):** operation worker starvation + thundering herd (no jitter). Fix is delay column + jitter (River `retry_policy.go:49-103` analogue on record, not in slot).

---

## 7. Leader election reuse — concrete plan

**Source:** `forge/api/queue/leader.go:11-148` (+ `Executor` interface `LeaderAttemptElect/LeaderAttemptReelect/LeaderResign` funded by `river_leader` table `reference/operations/river` — see `audits/final-parity/subagent-09-orchestration-queue.md:14` + synthesis §2 AF-2).

- Preferred reuse: **`pg_try_advisory_lock(hashtext('forge-leader'))`** wrapper gating `main.go:1150-1198,990-995,859,521,533` maintenance starts (no new table, no TTL races, verified pattern at `store_failover.go:136` `pg_advisory_xact_lock`).
- Alternative reuse: resurrect `queue.Elector` against `forge_leader` table (TTL 15 s `leader.go:34`, ElectInterval 5 s `31`) — closer to River `elector.go:26-31` but needs `LeaderAttemptElect` executor wired to a real `pgxpool.Pool` implementation (not the dead `forge/api/queue` fork's `Executor` stub).
- Scope gate **only** maintenance class: reconciler, heartbeat `EvaluateAll`, evacuation `resumeRunningPlans`, reservation sweeper, backup retention `bkWorker`, periodic `PeriodicJobScheduler`, replicamanager `reconcile`, failover `lastAction` loop, procedure/pipeline schedulers. Leave per-event work (`Dequeue`, claim, `PrepareMigration`) on `FOR UPDATE SKIP LOCKED` without leader.

**Do not** run a split deployment with two API replicas before one of the two is wired — current duplicated-key bug `queue/periodic.go:76-80,142` guarantees hourly/daily spam on day 1 of second replica.

---

## 8. Tenant columns — add now while tables are small (AF-7)

**Current gap:** `domain.PlacementRequest:114-129` no tenant field, every `FilterNodes/ScoreNodes` scans global `ListNodes:104`; `events.Envelope:196-205` no tenant; `eventstore` `events` schema `migration.go:19-31` tenantless; `ReconcilePlan`/`RecoveryPlan`/`Reservation` rows tenant-blind. NetBird/Incus analogue cited in `audits/phase-05/subagent-05-architecture-synthesis.md §C10` shows blast-radius risk (tenant A's affinity shapes fleet capacity for all).

**Minimal additive change before data grows:**

```
events{tenant_id uuid FK tenants, resource_tenant_id?} migration.go:19-31 + index
eventstore.Store{ClaimPending per-tenant still global — keep global claim, filter on read if needed}
domain.PlacementRequest{tenantID, projectID} domain.go:114-129
placement.WorkloadRequest{TenantID} strategy.go:46-56 + Place/PlaceAll filters once quotas exist
store.RecoveryPlan{tenant_id} + CreateRecoveryPlan(ctx, tenantID, ...)
store.PlacementReservation{tenant_id}
Envelope{TenantID} events/event.go:196-205 + NewEnvelope tenant param (keep compat: "" = legacy)
```

**Cost of delay:** retrofitting tenant onto populated `events/reconcile_plans/recovery_items` after volume accumulates is the migration to avoid. Phase-05 synthesis `§5 #5 + §4` already ordered this P1; preparation fix is to land the columns nullable-defaulted before second tenant ships.

**Hardening note for later (not Phase 01):** filter `ListNodes` per project quotas in `scheduler/service.go:88-108` once tenant plumbing lands — not a Phase-01 blocker.

---

## 9. Preparation checklist — what Phase 01 execution must do first (tracked)

- [ ] Land `eventstore.Store.PublishTx(ctx, pgx.Tx, Envelope)` (`store.go:113-133` sibling) and migrate ≥1 fence/failover producer to prove outbox guarantee.
- [ ] Wire **one** Relay consumer (choose `fencing.Service` — smallest handler `fencing.go:20-27`) onto `Relay` in `main.go:441`; assert `MarkDispatched` not called with zero handlers (`outbox.go:156-158` guard).
- [ ] Gate **one** maintenance daemon (choose `periodicScheduler` or `hbm`) behind `queue/leader.go:23-133` elect or advisory lock; prove second replica no longer duplicates `periodic.go:142` keys.
- [ ] Collapse fencing to one CAS `FenceServer/Node` and thread `generation` through `Target:18-24` to beacon `server.go:740-772` stale-reject; use same CAS as per-server serialization (AF-4).
- [ ] Consolidate one durable kind from `operation` → `queue` (choose `OpServerStart` — simplest `DispatchPower:364-396` → `queue.DispatchIdempotent`) and delete `operation` dequeuer for that kind only to prove read-model path.
- [ ] Fix `nodeautoscale/service.go:391` hardcoded `"aws"` and gate `ScaleIn:373-396` on drain completion before deprovision (otherwise S-06 reopens under autoscale).
- [ ] Add nullable `tenant_id` columns to `events`, `Envelope`, `PlacementRequest` with zero behavior change (preparation-only is acceptable to prove migration path).

---

## 10. Risks if preparation is skipped

- **Second replica** without AF-2/AF-1/AF-3 fixes → duplicated backups/certs, double failover plans racing capacity, split-brain power ops on same server, write-only ledger at 2× cost for no cross-instance durability.
- **Real evacuation** without fence enforcement → workload resurrected on old node after successful migration.
- **Another tenant** without §9 → tenant-blind scheduling and untaggable audit trail requiring late painful backfill.
- **River introduction** without §5 → third engine, third `attempt_count` semantic, third idempotency namespace — opposite of single-writer goal.

---

## 11. Evidence completeness — what was read (non-reliance)

Read in full with citations above: `placement/engine.go:1-112`, `constraints.go:1-193`, `strategy.go:1-190`, `replica.go:1-300`, `explain.go:1-106`; `services/scheduler/service.go:1-814`, `constraints.go:1-135`; `services/reservations/service.go:1-205`; `services/queue/queue.go:1-256`, `store.go:1-197`, `periodic.go:1-147`; `services/operation/service.go:1-507`, `store.go:1-252`; `eventstore/store.go:1-237`, `outbox.go:1-219`, `migration.go:1-80`; `services/fencing/fencing.go:1-49`, `services/recovery/service.go:1-730`, `services/heartbeatmonitor/service.go:1-429`, `services/reconciler/service.go:1-743`, `services/evacuationplanner/service.go:1-794`, `services/replicamanager/service.go:1-910`, `services/drain/service.go:1-209`, `services/nodeautoscale/service.go:1-412,worker.go:1-105`; `forge/api/cmd/api/{main.go:362-1304,river.go:1-92}`, `queue/leader.go:1-148`; `forge/api/internal/runtime/runtime.go:1-178`, `beacon/internal/runtime/factory.go:1-53`, `forge/api/internal/http/server.go:1-200`; plus `domain/domain.go:114-129` and `events/event.go:196-205`. Grep-verified absent: `PublishTx`, `Relay.Subscribe` production callers, beacon `generation` enforcement, `TunnelIP` column, `TenantID` in placement/events.

---

*Phase 01 orchestration will close AF-1..AF-3 plus queue duality with the smallest viable, single-writer, lease-faithful, leader-gated, tenant-tagged delta — not a new subsystem. Prior audits* `audits/phase-05/synthesis.md:§1-§5` *and* `audits/final-parity/subagent-09-orchestration-queue.md` *contain the 19-row parity matrix and structural derivations incorporated here; this report adds only the daemon inventory, the `queue.go:107,213` vs `operation/service.go:164,283` execution comparison, and the preparation file map.*

