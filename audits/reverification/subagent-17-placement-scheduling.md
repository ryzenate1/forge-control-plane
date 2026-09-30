# Subagent 17 — Reverification: Placement / Scheduling (Nomad vs Forge scheduler)

**Focus:** Constraints, scoring normalization, spread, affinity/anti-affinity, reschedule, drain, leader throughput  
**Reconciles:** `audits/phase-05/subagent-01-nomad-placement.md` (16 comparisons + 12 findings) + `audits/final-parity/subagent-09-orchestration-queue.md` placement rows 1-5  
**Method:** file:line re-inspection, no product code modified  
**Date:** 2026-08-24  
**Verdict:** **STILL BROKEN** — all 5 required gaps reproduce verbatim on current HEAD

---

## 0. Re-inspection scope (evidence completeness)

| Reference (Nomad @ `reference/orchestration/nomad/`) | Forge path:line | Checked |
|---|---|---|
| `nomad/structs/structs.go:4388` `Job` / `6877` `TaskGroup` / `nomad/structs/alloc.go:47` `Allocation` + `:125-129` `PreviousAllocation/NextAllocation` + `:140` `RescheduleTracker` | `forge/api/internal/placement/strategy.go` `Candidate`:`26-44` + `WorkloadRequest`:`46-56`, `migrations/101_a_multi_node_replicas.sql` instances `UNIQUE(app_id,idx)` | read |
| `nomad/scheduler/generic_sched.go:104` `GenericScheduler.Process` + `:120-128` trigger switch + `:291` `SubmitPlan` + `:137-159` `retryMax(5)` | `forge/api/internal/services/scheduler/service.go:88-166` `PlaceServer`, `forge/api/internal/services/replicamanager/service.go:732-808` `reconcile` poller | read |
| `nomad/scheduler/feasible/spread.go:165-293` `SpreadIterator` + `:214-264` even-spread + `structs.go:10204-10217` `Spread{Attribute,Weight,SpreadTarget[]%}` | `forge/api/internal/placement/strategy.go:137-147` `SpreadScorer = 1/(1+ServerCount)`, `replica.go:170-175` `count*0.1` penalty, `constraints.go:112-128` region eq only | read |
| `nomad/scheduler/feasible/rank.go:967-991` `NodeAffinityIterator` weight-normalized + `structs.go:10149-10195` `Affinity{Weight∈[-100,100]\{0}}` + `1024-1038` `ScoreNormalizationIterator` avg + `funcs.go:257-277` `ScoreFitBinPack [0,18]` | `forge/api/internal/placement/constraints.go:59-63` `+1e12/-1e10`, `strategy.go:91-104` `LeastLoadedScorer ≤3`, `services/scheduler/service.go:306-323` `+1e9 / ±1e10 / +1e8` tweaks, `placement/explain.go:89-106` | read |
| `nomad/structs/structs.go:6611-6632` `ReschedulePolicy{Attempts,Interval,Delay,DelayFunction,MaxDelay,Unlimited}` + `generic_sched.go:797-818` `penaltyNodes` + `rank.go:875-910` `NodeReschedulingPenaltyIterator` + `reconcile_cluster.go:1408+` `createRescheduleLaterEvals` + `generic_sched.go:272-285` `WaitUntil` | `forge/api/internal/services/replicamanager/service.go:883-910` `RetryFailedPlacements` + `services/scheduler/service.go:599-688` `ReplaceFailedInstance` (no policy) | read |
| `nomad/nomad/blocked_evals.go:176-206` `Block/Reblock` + `:478` `Unblock` + `:578` `UnblockNode` + `scheduler/util.go:67` `retryMax` | `replicamanager/service.go:697-719` 60s ticker `reconcile` + `:779-783` conditional `RetryFailedPlacements` | grep zero hits for `blocked_evals`/`Unblock` in forge |
| `nomad/scheduler/feasible/stack.go:79-100` `SetNodes` shuffle + `limit=log2(n)` power-of-two + `82` `ShuffleNodes` + `GenericStack:494` iterator chain | `forge/api/internal/placement/engine.go:18` `mu sync.Mutex` + `:38` `Place` + `:79` `PlaceAll` global mutex + `strategy.go:149-157` `RandomScorer.mu` (only inner lock) + `scheduler/service.go:282-294` sequential snapshot fetches | read |
| `forge/api/internal/scheduler/scheduler.go:1-108` type registry (`Docker/K3s/Nomad`) — parity surface | `forge/api/internal/services/scheduler/constraints.go:14-25` `required/preferred/forbidden` + `:117-131` key-limited eval | read |
| `forge/api/internal/services/reservations` + `services/drain` + `services/evacuationplanner/service.go:188` `maxConcurrent=2` + `replicamanager` + `services/nodeautoscale/service.go:380-394` hardcodes `cloud.ProviderKind("aws")` + `:288` unit-soup `leastLoadedNode` | all read |

Previous commit diff (`git diff HEAD` on `forge/api/internal/placement/**` / `scheduler/**`) shows only `replicamanager/service.go:780` added `slog.Warn` wrapping and `nodeautoscale/service.go` whitespace — **no semantic fix** to any finding below.

---

## 1. Definitive parity matrix (16 rows — meets >=12 requirement)

Legend: `GAP`=missing/mostly absent, `PARTIAL`=half-baked, `PARITY`=adequate. All STATUS re-verified 2026-08-24 on current HEAD.

| # | Capability | REFERENCE Path:Symbol | FORGE Layers file:line | STATUS | GAP (one-line) | Prior | Still BROKEN? |
|---|---|---|---|---|---|---|---|
| 1 | Job/TaskGroup/Allocation model with predecessor chain + RescheduleTracker | `nomad/structs/structs.go:4388` `Job{TaskGroups}`, `:6877` `TaskGroup`, `alloc.go:47,125-129,131,140` `Allocation{Previous/Next,DeploymentID,RescheduleTracker}` | `placement/strategy.go:46-56` `WorkloadRequest`, `placement/replica.go:10-20` `ReplicaSpec/Request`, `migrations/101_a_multi_node_replicas.sql` `instances UNIQ(app_id,idx)` + `placement_decisions` append-only | **PARTIAL** | No `PreviousAllocation` chain → no reschedule-history carry-forward, no penalty feeding; index dedupe matches (`generic_sched.go:617-632`) but linkage/audit absent. | C1 | Yes — unchanged |
| 2 | Declarative reconcile loop on every trigger vs 60s status-only poller | `scheduler/generic_sched.go:104` `Process` + `:120-128` triggers (job, node, drain, retry, …) + `reconciler/*` diff `Place/Stop/Inplace` | `services/replicamanager/service.go:697-719` 60s `reconcile()` + `:779-783` `if hasFailed>0 RetryFailedPlacements` + status label fixes `:785-806` | **GAP** | No convergence pass `desired Replicas vs healthy instances`; out-of-band delete stays missing until `ScaleApp`. | C2 | Yes |
| 3 | Two-phase plan/apply with optimistic concurrency vs DB-reservation gating | `generic_sched.go:291` `SubmitPlan` + `:309-316` `FullCommit` + `util.go:67` `retryMax=5` | `services/scheduler/service.go:126-165` `PlaceServer` reservation loop + `store_reservations.go:87-93` capacity check | **PARTIAL** | DB gating is simpler/valid, but reservation→dispatch not atomic; orphan-hold window until 1-min `ExpireReservations` (`services/reservations` ticker). | C4 | Yes |
| 4 | Scoring normalization pipeline (bounded iterators + avg) vs raw additive bonuses | `scheduler/feasible/rank.go:967-991` affinity normalized `/sumWeight` + `funcs.go:257-277` `[0,18]` + `:1024-1038` `ScoreNormalizationIterator` avg + `generic_sched.go:582-583` `ScoreMetaData` | `placement/constraints.go:59-63` `+1e12/-1e10` + `strategy.go:91-104` base `≤3` + `services/scheduler/service.go:306-323` `+1e9/-1e10/+1e8` + `placement/explain.go:89-106` | **GAP** | **F1 re-verified** — soft bonus dwarfs base by 9-12 OOM; locality inserts between scales. See §2 F1. | C6 / final-09 row1 | **BROKEN** |
| 5 | Constraint feasibility operand algebra vs enum types + dead preferred | `structs.go:9980-10058` `Constraint{LTarget,RTarget,Operand}` `=,!=,<,<=,>,>=, regex, version, distinct_hosts` | `placement/constraints.go:11-17` 5 types + `services/scheduler/constraints.go:14-25,76-85` `meetsAllConstraints` treats `preferred/forbidden` as advisory-NOOP | **GAP** | No version/regex/set operators, no `distinct_hosts`; global scheduler's `preferred` neither filters nor scores. | C5 | Yes |
| 6 | Spread across attribute + %Targets + weight vs inverse-count heuristic | `structs.go:10204-10217` `Spread{Attribute,Weight,SpreadTarget[]%}` + `spread.go:165-293` (`*` bucket, `:214-264` even-spread) | `placement/strategy.go:137-147` `1/(1+ServerCount)` + `replica.go:170-175` `0.1*count` second penalty + region equality-only `constraints.go:112-128` | **GAP** | **F2 re-verified** — misnamed anti-co-location only; cannot express zone/rack %Targets; two penalties undocumented. | C7 / row2 | **BROKEN** |
| 7 | Affinity: weighted `[-100,100]\{0}` normalized vs binary existence pin | `structs.go:10149-10195` affinity weight + `rank.go:936-955` merge + `:967-991` normalize (negative allowed) | `placement/constraints.go:86-97` `checkAffinity` existence-only + soft `±1e12/-1e10` | **GAP** | No graded preference, no strength; same blunt scale as all soft constraints. | C8 | Yes |
| 8 | Anti-affinity: proportional `-(coll+1)/desiredCount` (proposed allocs) vs flat `0.1*count` | `rank.go:833-866` `JobAntiAffinityIterator` with `desiredCount` + `generic_sched.go:797-818` penalty nodes | `placement/replica.go:57-74,170-175` `count*0.1` over `ExistingNodeMap` + `max(existingCPU/Mem/Disk)` pre-debit | **GAP** | Absolute penalty buried under soft bonuses or lost in least-loaded noise (4×0.4 vs base 0-3); `ExistingNodeMap` max-size overshoot F5. | C9 | Yes |
| 9 | Reschedule policy: attempts/interval/delayFn/MaxDelay/unlimited+WaitUntil+penalty vs unconditional 60s forever | `structs.go:6611` `ReschedulePolicy` + `generic_sched.go:272-285` `WaitUntil` + `reconciler/filters.go:395-447` + `rank.go:875-910` penalty iterator | `services/replicamanager/service.go:883-910` `RetryFailedPlacements` every `1*time.Minute` (`:706` ticker) forever, `Sleep(100ms)` inline `:904`, `services/scheduler/service.go:616-643` no `PenaltyNodeIDs` | **GAP** | **F6 re-verified** — no cap/backoff/delayFn; fleet-wide bad image = 1/min replace-storm churning Beacon commands + `placement_decisions`. Blocks shared reconcile goroutine. | C10 / row3 | **BROKEN** |
| 10 | Sticky/migrate volumes: preferred-node fallback to full set vs hard `RequiredNode` fail | `generic_sched.go:872-903` `findPreferredNode` + `feasible/stack.go:136-151` `Select` tries preferred then fallback | `placement/replica.go:108-115` `required node %s not found or incompatible` hard fail + `services/scheduler/service.go:306-308` `+1e9` vs `replica.go:177-179` `+1` (9 OOM gap) | **GAP** | No graceful fallback; preferred magnitudes inconsistent between paths; `ExplainReplicaPlacement:241-245` swallows scorer error. | C11 | Yes (F4) |
| 11 | Deployment strategy: `MaxParallel/MinHealthyTime/HealthyDeadline/Canary/AutoPromote/AutoRevert` vs fire-and-forget | `structs.go:5406-5449` `UpdateStrategy` + `deploymentwatcher/deployment_watcher.go:158-317` health-gated promotion/rollback | `services/replicamanager/service.go:346-356` `deployReplicas` immediate fan-out + `810-820` `running` counts *provisioned* + reconciler only flips to `degraded/failed` `:785-806` | **GAP** | No concurrency bound/health window/canary; no `AutoRevert`/`ProgressDeadline` — crash-looping images remain largest operational gap (C12). | C12 | Yes |
| 12 | Node draining: deadline/force + `Migrate.MaxParallel` health-gated vs global const ledger | `structs.go:1908-1951` `DrainStrategy{Deadline,ForceDeadline}` + `drainer/watch_jobs.go:415-425` `tg.Migrate.MaxParallel` + `structs.go:6821-6872` `MigrateStrategy` | `services/drain/service.go:47-184` durable ledger + `services/evacuationplanner/service.go:188` `maxConcurrent=2` global const + `:561-576` `StorageLocalOnly` guard (ahead of Nomad CE) | **PARTIAL** | Deadline/force absent; pacing not per-workload-criticality; stuck `preparing` blocks slot until terminalized (reaper mitigates). | C13 / row5 | Yes |
| 13 | In-place updates (field-diff reuse) vs always-recreate | `scheduler/util.go:167-297` `tasksUpdated` + `:569-701` `inplaceUpdate` | `services/replicamanager/service.go:561-652` `ReplaceInstance` always `removing→fresh→dispatch` | **GAP** | Env-only changes force full re-provision latency; acceptable for game servers but wasteful. | C14 | Yes |
| 14 | Scheduling throughput: lazy iterators + power-of-two `log2(n)` + `ShuffleNodes` vs global mutex serialization | `feasible/stack.go:79-100` `ShuffleNodes` + `:92-99` `limit=log2(n)` cap (2 for batch) + string of iterators (`NewGenericStack:494`) | `placement/engine.go:18` `mu sync.Mutex` held across filter+score+select `:38-40,79-81` + `services/scheduler/service.go:282-294` sequential `NodeCapacitySnapshot` per node + `strategy.go:107-109` unstable `sort.Slice` tie-break | **GAP** | **Still BROKEN** — single mutex serializes whole cluster; per-node queries sequential; throughput collapses with fleet size. See §2 F9. | C15 | **BROKEN** |
| 15 | Priority & preemption vs FIFO-by-tick | `structs.go:4418-4420` `Priority` + `generic_sched.go:906-954` preemption + `rank.go:1042+` `PreemptionScoringIterator` | `placement/strategy.go:46-56` `WorkloadRequest`/`Candidate` no `Priority`; `services/scheduler` no preemption | **GAP** | Game server cannot preempt staging batch; contention is FIFO-by-tick. | C16 | Yes |
| 16 | Node-autoscale / provider routing vs hard-coded AWS | Nomad AP external (out-of-repo) | `services/nodeautoscale/service.go:380-394` `DeprovisionNode(ctx, ProviderKind("aws"), nodeID)` hardcoded; `:288` `leastLoadedNode` unit-soup `AllocatedCPU+AllocatedMemory+AllocatedDisk`; `replicamanager` drain-then-deprovision races | **GAP** | Hetzner/DO node hits wrong adapter; deprovision fires sync despite doc "when drain completed"; disk-MB dominates ranking→degenerates to least-disk. | F10 from S1 | Yes |

Counts: 16/16 comparisons re-verified (≥12 required). Rows 4,6,9,14,16 are the 5 "still BROKEN" categories demanded in the prompt; all five reproduce.

Final-parity `subagent-09` placement rows reconciliation:
- Row1 (scoring normalization GAP) → C4/F1 above — still GAP.
- Row2 (spread GAP) → C6/F2 — still GAP (anti-only).
- Row3 (reschedule GAP) → C9/F6/F8 — still GAP (60s forever).
- Row4 (blocked evals GAP) → C2 + §2 F7 — still GAP (60s poll only).
- Row5 (drain PARTIAL) → C12 — still PARTIAL (global const, no deadline).
All five rows unchanged since snapshot; status column matches this re-inspection.

---

## 2. Logic findings (≥3 required — 7 re-verified, severity preserved)

### F1 — Soft-constraint bonus magnitudes destroy the score space — **HIGH — STILL BROKEN**

- **Forge:** `forge/api/internal/placement/constraints.go:59-63` satisfied `+= 1e12`, unsatisfied `-= 1e10`; `forge/api/internal/placement/strategy.go:91-104` `LeastLoadedScorer` base `≤3` (sum of 3 ratios), `SpreadScorer:141` `≤1`, `RandomScorer:162-173` `≤1`; `forge/api/internal/services/scheduler/service.go:306-323` injects `+1e9` preferred, `-1e10` storage-locality mismatch, `+1e8` match, and `predictive.go:149-175` multiplies/adds `trend/affinity` atop already-scaled `r.Score`.
- **Math:** One unsatisfied soft (−1e10) vs one satisfied (+1e12) → satisfied wins by 990 B; two unsatisfied (−2e10) still loses to one +1e12. Storage mismatch penalty (−1e10) exactly cancels one soft penalty and sits *between* soft satisfier and penalty → tuned independently, not normalized. `ExplainScores:89-106` reports `SoftConstraintBonus: 1e12` next to `BaseScore: 1.7`.
- **Reference:** Nomad `scheduler/feasible/rank.go:1004-1038` averages every factor into `[-1, ~18]` then into final `[-1,1]` per factor; per-factor breakdown into `AllocMetric.ScoreMetaData` (`generic_sched.go:582-583`).
- **Effect:** Constraint count dominates binpack/fit; anti-affinity `0.1*count` never survives; operator cannot tune.
- **Fix (from S1 #1):** Bounded iterators (`base∈[0,1], soft≤±2, locality≤±1, preferred≤+0.5`) then normalize+avg; `Explain*` per-factor.

### F6 — No reschedule backoff / attempt ceiling ⇒ crash-loops amplify — **HIGH — STILL BROKEN**

- **Forge:** `forge/api/internal/services/replicamanager/service.go:705-719` `time.NewTicker(1*time.Minute)` drives `reconcile`; `:732-783` `if hasFailed>0 RetryFailedPlacements`; `RetryFailedPlacements:883-910` iterates **every** app, every `failed` instance, `ReplaceInstance:561-652` marks `removing→place→dispatch→provisioning` with **no** `ReschedulePolicy` analogue. Loop does `time.Sleep(100*time.Millisecond)` per app **inside** the shared reconcile goroutine (`:904`), stalling status reconciliation for all other apps.
- **Missing:** `structs.go:6611` `Attempts/Interval/Delay/DelayFunction(fibonacci/exponential)/MaxDelay/Unlimited`, `generic_sched.go:272-285` `WaitUntil` delayed follow-up evals, `reconcile_cluster.go:1408+` `createRescheduleLaterEvals`, `rank.go:875-910` `NodeReschedulingPenaltyIterator` feeding `generic_sched.go:797-818` `penaltyNodes`.
- **Exploit:** Deploy bad image fleet-wide → permanent 1/min `ReplaceFailedInstance` per failed instance, each dispatching Beacon commands (`dispatchBeaconCommand:834-845`) and appending `placement_decisions`. Inline sleep amplifies stall.
- **Also:** `ReplaceFailedInstance:599-688` builds candidates via `FilterNodes` with empty region/runtime, never excludes/penalizes `inst.NodeID` (flaky-but-online node retains high free capacity after workload died). Nomad feeds prior failures into `PenaltyNodeIDs`.
- **Fix (S1 #2):** Per-instance `failure_count,next_retry_at,last_failed_node_id` + exponential backoff; penalty iterator consulted by `ReplaceInstance`.

### F2 — Spread is misnamed anti-co-location; true spread absent — **MEDIUM-HIGH — STILL BROKEN**

- **Forge:** `placement/strategy.go:137-147` `1/(1+ServerCount)` minimizes per-node count; `replica.go:170-175` second penalty `count*0.1` additive → two mechanisms, different semantics, undocumented. Cannot express `Spread.Attribute="datacenter"` + `SpreadTarget{{Value:"dc1",Percent:50},…}` + `Weight` (`structs.go:10204-10217`, `spread.go:165-293` with `*` remainder bucket, `:214-264` even-spread).
- **Impact:** Game-panel HA goal "don't co-locate all replicas of a shard in one zone" unexpressible; `RegionID` filter is equality-only (`constraints.go:112-128`); repeated scale up/down (via `ScaleReplicas:578-595` positional victim selection) consolidates onto one node.
- **Credit retained:** `evacuationplanner/service.go:507-516` `reserved` map discounts during planning (≈ Nomad plan-applier).

### F7 — Missing blocked-eval equivalent ⇒ slow reaction to freed capacity — **MEDIUM — STILL BROKEN**

- **Reference:** `nomad/nomad/blocked_evals.go:176-206` `Block/Reblock` + `:478` `Unblock` + `:578` `UnblockNode` + `missedUnblock:352-406`; failed placements park and retry **instantly** when matching class/quota/node capacity frees.
- **Forge:** No `BlockedEvals`. Only `replicamanager/service.go:779-783` conditional on next 60s tick (or manual `Retry` call). Capacity free at T+1s waits up to 59s; no `QueuedAllocations` (`util.go:561-563`) visibility.
- **Cheap fix (S1 #3):** Publish event from `ExpireReservations` + node-state transitions to kick `RetryFailedPlacements` for affected apps (scoped, not full sweep).

### F9 — Global placement mutex serializes whole cluster — **LOW-MEDIUM — STILL BROKEN**

- **Forge:** `placement/engine.go:18` `mu sync.Mutex` + `:38-40` `Place` locks across `FilterByConstraints+Score+CheckSoft`, `:79-81` `PlaceAll` same, `replica.go` reuses `e.scorer/e.checker` under caller's locks. `services/scheduler/service.go:282-294` fetches `NodeCapacitySnapshot` **sequentially** per node.
- **Evidence of statelessness:** `ConstraintChecker struct{}` (`constraints.go:32`), `LeastLoaded/BinPack/SpreadScorer` stateless, `RandomScorer` owns its own `mu:149-157` — mutex protects nothing contended; likely removable outright (S1).
- **Reference throughput:** Nomad lazy iterator chains + `stack.go:79-100` `log2(n)` cap (services) / 2 (batch) power-of-two choices + `ShuffleNodes:82` tie-break + concurrent workers with optimistic `Plan` commits (`generic_sched.go:291,309`).
- **Consequence:** N concurrent deploys serialize 1-at-a-time; tail latency linear in fleet size; `sort.Slice:107-109` unstable → input-order tie-break, not shuffled.

### F10 — nodeautoscale scale-in deprovisions hard-coded AWS & races drain — **MEDIUM — STILL BROKEN**

- **Forge:** `services/nodeautoscale/service.go:380-394` `ScaleIn:380` starts drain via `membership.StartDrain:381`, then if `deprovision && node.SchedulerType!=""` immediately calls `s.cloud.DeprovisionNode(ctx, cloud.ProviderKind("aws"), nodeID)` — provider string is literal `"aws"` regardless of node's `Provider`; Hetzner/DO hits wrong adapter. Doc comment (§370-372) claims "when drain completed" but code `await`s nothing — evacuating workloads lose source mid-flight. Also `leastLoadedNode:288` ranks by `AllocatedCPU+AllocatedMemory+AllocatedDisk` (CPU shares + MB) — disk-MB dominates, degenerates to "least disk used".
- **Reference:** Nomad CP delegates autoscale to external AP; Forge's `evacuationplanner:561-576` `StorageLocalOnly` guard is *ahead* of Nomad CE but pacing is global constant `maxConcurrentEvacuationMigrations=2:188`, not per-workload `Migrate.MaxParallel`.

### F4 (supplemental) — Inconsistent preferred-node scale between paths — **MEDIUM — STILL BROKEN**

- `services/scheduler/service.go:306-308` single-server preferred `+1e9` (overwrites `reason="preferred node"`), vs `placement/replica.go:177-179` replica `+1` (negligible beside `±1e10`). Result: "prefer this node" wins in path A, loses to one soft constraint in path B. `ExplainReplicaPlacement:219-270` omits bonus and swallows scorer errors `241-245` (`_`), so explanation can recommend a node the real placer rejects for capacity.

Minor retained: F5 `replica.go:59-74` replica capacity pre-debit uses `count×max(existingCPU/Mem/Disk)` not actual per-node mix → overshoots, reduces packing density (safe but wasteful); F3 reservation confirm-after-dispatch double-book window + flapping instance pins phantom capacity until 1-min expiry if `UpdateInstanceStatus` silent-fails (`_, _ =`): verified present but less load-bearing than 5 must-haves.

---

## 3. Reconciliation to prior audit claims

| Prior claim | Re-verified | Delta since prior HEAD |
|---|---|---|
| S1 16 comparisons | All 16 re-opened above, same file:line except `replicamanager:780` warn-add | None semantic |
| S1 12 findings (F1 bonus overflow, F6 no reschedule, F2 spread anti-only, F7 blocked-eval, … F12) | F1,F2,F6,F7,F9,F10 reproduced; F3/F4/F5 still present; F11/F12 unchanged/deprecated | No fix landed |
| S1 "Forge does better" (durable decisions, restart recovery, storage-locality-aware evacuation, zero-capacity metric) | Still true — not placement-gaps | Retained |
| Final-09 placement rows 1-5 | Row1=F1, Row2=F2, Row3=F6, Row4=F7, Row5=C13 — all status unchanged | Confirmed open |
| "No leader throughput / power-of-two" | `engine.go:38` mutex + `stack.go:92-99` power-of-two absent — still BROKEN | Confirmed |
| "No reschedule (60s forever no cap)" | `replicamanager:706,883-910` 60s forever — still BROKEN | Confirmed |

---

## 4. Verdict

**All five mandated still-BROKEN signals reproduce on current HEAD:**

1. **Soft bonus overflow** (`constraints.go:59`) — `+1e12/-1e10` dwarfs base `≤3`.
2. **No reschedule policy** (`replicamanager:883-910`) — 60s forever, no cap/backoff (`structs.go:6611` absent).
3. **Anti-only spread** (`strategy.go:137-147`, `replica.go:170`) — `1/(1+n)` / `0.1*count`, no attribute/percent (`spread.go:165` absent).
4. **No blocked-eval** (`blocked_evals.go:176` absent) — capacity-free waits next tick.
5. **No leader throughput (power-of-two)** (`stack.go:79-100` / `generic_sched.go:104` iterator+log2 pattern absent) — `engine.go:38` global mutex serializes cluster.

The placement/scheduling dimension remains **GAP** for Nomad parity; recommendations from `phase-05/subagent-01-nomad-placement.md` §5 (1 unbounded-factor fix, 2 ReschedulePolicy, 3 event-triggered retries, 4 zone-aware spread, 6 lock-free Engine + parallel snapshot fetches) remain the minimal viable path and are **not yet applied**. No new product code is introduced by this reverification.

*Evidence files:* `forge/api/internal/placement/engine.go:38`, `constraints.go:59`, `strategy.go:91,137`, `replica.go:59,170`, `explain.go:89`, `forge/api/internal/services/scheduler/service.go:282,306,616`, `services/replicamanager/service.go:705,732,883`, `services/nodeautoscale/service.go:288,380-394`, `forge/api/internal/scheduler/scheduler.go`, `reference/orchestration/nomad/nomad/structs/structs.go:6611,10149,10204`, `scheduler/generic_sched.go:104`, `scheduler/feasible/spread.go:165`, `scheduler/feasible/rank.go:967`, `scheduler/feasible/stack.go:79-100`.

