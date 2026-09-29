# Phase 5 — Subagent 01: Placement / Scheduling Deep-Dive (Nomad reference vs Forge)

Cluster: `nomad` (primary), cross-checked against `incus`, `netbird`, `longhorn`, `rancher`, `river`.
Reference tree root: `reference/orchestration/nomad/`
Forge tree root: `forge/api/internal/{placement,scheduler,services}`

Scope: jobs/taskgroups/allocations model, constraints/affinity/spread, eval broker, plan/reconcile,
node draining/rescheduling, deployment strategies.

---

## 1. Reference map (what was inspected)

| Area | Nomad | Forge |
|---|---|---|
| Data model | `nomad/structs/structs.go` (`Job` :4388, `TaskGroup` :6877, `ReschedulePolicy` :6611, `UpdateStrategy` :5406, `Constraint` :9980, `Affinity` :10108, `Spread` :10204), `nomad/structs/alloc.go` (`Allocation` :47) | `forge/api/internal/placement/{strategy.go,constraints.go,replica.go}`, migrations `101_a_multi_node_replicas.sql`, `041_a_placement_intents.sql`, `026_placement_reservations.sql`, `190_add_env_affinity.sql` |
| Scheduler core | `scheduler/generic_sched.go`, `scheduler/util.go`, `scheduler/reconciler/*` | `forge/api/internal/services/scheduler/service.go`, `forge/api/internal/placement/engine.go` |
| Rank/filter stack | `scheduler/feasible/{stack.go,rank.go,spread.go,select.go}` | `placement/constraints.go`, `services/scheduler/constraints.go` |
| Eval broker / retries | `nomad/blocked_evals.go`, `retryMax` in `scheduler/util.go:67` | none (ticker-based `reconcile`) |
| Deployments | `nomad/deploymentwatcher/deployment_watcher.go` | status aggregation in `services/replicamanager/service.go` |
| Draining | `nomad/drainer/watch_jobs.go`, `structs.DrainStrategy` :1908 | `services/drain/service.go`, `services/evacuationplanner/service.go` |
| Autoscale | nomad external AP (out of repo scope) | `services/autoscaler/service.go`, `services/nodeautoscale/{service.go,worker.go}` |

Note on adapters: Forge's literal Nomad adapter (`api/internal/scheduler/scheduler_nomad.go:1-15`)
is explicitly **deprecated** — scheduling is owned by the internal placement engine. The comparison
below therefore pits the internal engine against Nomad proper, which is the right baseline since the
engine is what runs in production.

---

## 2. Comparisons (16)

### C1. Job/TaskGroup/Allocation vs App/Instance/PlacementDecision
Nomad models a declarative job (`Job` with `TaskGroups []*TaskGroup`, structs.go:4388/4452); each
desired replica materializes into an `Allocation` carrying `PreviousAllocation`/`NextAllocation`
chains (alloc.go:125-129), `DeploymentID` (:131), and `RescheduleTracker` (:140). The allocation is
the unit the reconciler diffs against desired count.
Forge splits this across three tables: `replica_applications` (desired state incl. `replicas`),
`instances` (per-replica rows, `UNIQUE (app_id, idx)` — migrations/101_a_multi_node_replicas.sql),
and append-only `placement_decisions`. Parity is decent: instance idx uniqueness matches Nomad's
alloc-name index dedupe (generic_sched.go:617-632). Gap: Forge instances have no predecessor chain
(no `PreviousAllocation` equivalent) — `placement_decisions` keeps history per instance but there is
no linkage that lets the system answer "which alloc replaced which" or carry reschedule history
forward, which Nomad uses both for penalties (C10) and for auditability.

### C2. Declarative reconcile loop vs imperative deploy methods
Nomad's `GenericScheduler.Process` (generic_sched.go:104) converges state on *every trigger*
(job register, node update, drain, deployment watcher, retry-failed-alloc, … :120-128) through the
reconciler diff (`result.Place/Stop/InplaceUpdate/DestructiveUpdate`). Any divergence self-heals.
Forge exposes imperative operations (`DeployApp` :143, `ScaleApp` :361, `ReplaceInstance` :561 in
replicamanager) plus a 60 s `reconcile` poller (:697-719) that only fixes *status labels* and calls
`RetryFailedPlacements` when failures exist (:779-783). There is no convergence pass comparing
desired `app.Replicas` to actual healthy instances — e.g. an instance deleted out-of-band stays
missing until someone calls ScaleApp. Nomad closes this within one eval.

### C3. Eval broker / blocked evals vs nothing
When placement fails, Nomad creates a **blocked eval** (generic_sched.go:180-199) parked in
`BlockedEvals`; any capacity change unblocks exactly the matching classes/quota/nodes
(blocked_evals.go:176-206 Block/Reblock, Unblock :478, UnblockNode :578, `missedUnblock` index
tracking :352-406). Failed placements are retried the moment resources free up.
Forge has **no equivalent**: a failed replica is only retried by `RetryFailedPlacements`
(replicamanager/service.go:883-910) on the next 60 s tick, regardless of whether capacity became
free 1 s later. There is also no queued-work visibility analogous to `QueuedAllocations`
(util.go:561-563) surfaced to operators. Finding F7.

### C4. Two-phase plan/apply with optimistic concurrency vs DB-reservation gating
Nomad separates *plan computation* from *commit*: the scheduler builds a `Plan`, submits it
(`SubmitPlan`, generic_sched.go:291), and the leader's plan applier rejects conflicting plans;
on partial commit the scheduler retries (`FullCommit` check :309-316) up to `retryMax(5)` with
progress reset (:137-159, util.go:67-89). Capacity races between schedulers are arbitrated centrally.
Forge arbitrates via Postgres instead: `CreatePlacementReservation` validates available capacity
inside the store (`store_reservations.go:87-93` "reserved cpu exceeds available capacity") with
row locking, and `PlaceServer` walks scored nodes creating reservations until one succeeds
(services/scheduler/service.go:126-165). This is a legitimate design (single-writer DB instead of
leader plan queue) and arguably simpler; but unlike plan-apply there is no *atomicity between
reservation and dispatch* — see F3 for where that seam leaks.

### C5. Constraint feasibility: operand algebra vs enum types
Nomad `Constraint{LTarget, RTarget, Operand}` (structs.go:9980-9984) supports `=,!=,<,<=,>,>=`,
set-contains variants, regex, version/semver, `distinct_hosts`, `distinct_property`,
attribute is-set/is-not-set (:10020-10058), validated at registration. Constraints compose at
job/task-group/task level.
Forge has two disjoint systems: (a) per-request `Constraint` types {affinity, anti-affinity,
region, node, label} with operators {in, not-in, exists, not-exists}
(placement/constraints.go:11-17,148-178); (b) a global `ConstraintScheduler` with its own types
{required, preferred, forbidden} and eq/neq/in/notin/exists evaluated against only three keys —
region, node_id, name (services/scheduler/constraints.go:14-25,117-131). No version/regex/set
operators, no distinct-hosts analogue, and notably (b)'s "preferred"/"forbidden" types are dead
weight: `meetsAllConstraints` (:76-85) treats everything except `required` as advisory-noop
(failure of a preferred constraint neither scores nor filters).

### C6. Scoring pipeline normalization vs raw additive bonuses
Nomad composes scores as separate rank iterators — binpack fit (`ScoreFitBinPack`, bounded [0,18],
funcs.go:257-277), node affinity (weight-normalized, rank.go:967-991), spread boost (spread.go:204),
anti-affinity penalty (rank.go:855-863) — then averages them via `ScoreNormalizationIterator`
(rank.go:1024-1038) and emits per-factor breakdown into `AllocMetric.ScoreMetaData`
(generic_sched.go:582-583 `PopulateScoreMetaData`).
Forge adds a base strategy score plus `CheckSoft` bonuses: satisfied soft constraint ⇒ `+1e12`,
unsatisfied ⇒ `−1e10` (placement/constraints.go:59-63), then further ±1e9/±1e10/±1e8 tweaks in
`ScoreNodes` (preferred node, storage locality — services/scheduler/service.go:306-323). Because the
factors differ by ~20 orders of magnitude, one unsatisfied soft constraint overrides everything else,
and the averaged "Reasons" string mixes incommensurable numbers. Finding F1.

### C7. Spread: attribute-target percentages vs inverse-count heuristic
Nomad `Spread{Attribute, Weight, SpreadTarget[]%}` (structs.go:10204-10217) spreads allocs across a
node attribute with per-value desired counts and an implicit `*` remainder bucket
(spread.go:165-201,266-293), plus an even-spread mode when no targets are given
(:214-264). Spread participates in normalized scoring with a configurable weight.
Forge's `SpreadScorer` is `1/(1+ServerCount)` (placement/strategy.go:137-147) — i.e. "fewest servers
on node", which is really anti-co-location, not distribution over an attribute. There is no notion
of spreading across regions/zones/racks, no weight, no percentage targets. For a game panel whose
main HA goal is "don't put all replicas of a shard in one zone", the Nomad feature is materially
absent. Additionally the replica path applies a *separate*, additive spread penalty
(`count*0.1`, replica.go:170-175), so two spread mechanisms coexist with different semantics. Findings F2/F4.

### C8. Affinity: weighted preferences vs binary pins
Nomad affinities take `Weight ∈ [-100,100] \ {0}` (structs.go:10149-10195 validation), merge
job+group+task levels (rank.go:936-955), normalize by total weight, and allow negative weights
(soft anti-preference). Forge's `checkAffinity` is existence-only: candidate passes iff the node
hosts one of the listed servers (constraints.go:86-97), and as a *soft* constraint contributes the
same blunt ±1e12/−1e10 as everything else. There is no graded preference ("prefer same rack") and
no way to express strength.

### C9. Anti-affinity: proportional penalty vs flat count penalty
Nomad penalizes co-placement with same job/group allocs by
`-(collisions+1)/desiredCount` using *proposed* allocations of this very evaluation
(JobAntiAffinityIterator, rank.go:833-866), so the penalty grows relative to how many replicas you're
placing and naturally saturates. Forge computes collisions from a caller-supplied
`ExistingNodeMap` plus running tally (`usedNodeCount`), penalty `count*0.1` (replica.go:57-74,170-175).
Functional parity for the common case, but: (i) the penalty is absolute, so with the least-loaded
base score (~0–3) four co-located replicas (−0.4) can be outweighed by trivial resource-ratio noise,
while with soft bonuses present it is irrelevant (see F1); (ii) `ExistingNodeMap` capacity discounting
uses the *max* replica size times count instead of the actual per-node mix — F5.

### C10. Reschedule-on-failure policy vs unconditional instant retry
Nomad `ReschedulePolicy` (structs.go:6611-6632): attempts/interval, delay functions
exponential/constant/fibonacci with `MaxDelay`, unlimited mode; delayed reschedules become
follow-up evals with `WaitUntil` (generic_sched.go:272-285, reconciler/filters.go:395-447,
createRescheduleLaterEvals reconcile_cluster.go:1408+). Failed nodes accumulate into
`penaltyNodes` from `RescheduleTracker.Events` so retries prefer other nodes
(generic_sched.go:797-818, rank.go:875-910).
Forge has **no reschedule policy**: `RetryFailedPlacements` (replicamanager/service.go:883-910)
replaces every failed instance once a minute, forever, with no backoff, no attempt cap, no
delay function — and does so synchronously inside the shared reconcile loop with a
`time.Sleep(100ms)` per app (:904). `ReplaceFailedInstance` filters nodes purely on
capacity/state (services/scheduler/service.go:616-643); nothing prevents re-placing onto the exact
node that just failed the workload. Findings F6/F8.

### C11. Sticky/migrate volumes & preferred-node fallback vs hard required node
Nomad honors `EphemeralDisk.Sticky|Migrate`: `findPreferredNode` returns the previous alloc's node
and `Select` *tries it first, falling back to the full node set* if infeasible
(generic_sched.go:872-903, feasible/stack.go:136-151). Preference degrades gracefully.
Forge `PreferredNode` is just a score bump (+1 in replica.go:177-179; +1e9 in
services/scheduler/service.go:306-308) — fine — but `RequiredNode` is a hard filter that yields
"required node %s not found or incompatible" (replica.go:108-115) with no fallback, and in
`placeSingleReplica` the RequiredNode branch bypasses `FilterByConstraints` entirely (it runs
`CheckHard` afterwards in `buildPlacement` :185-188, so hard constraints still apply, but the error
message will claim incompatibility rather than naming the violated constraint). Also the preferred
bonus magnitude differs by 9 orders of magnitude between the two paths — F4.

### C12. Deployment strategy/canaries vs fire-and-forget deploys
Nomad's `UpdateStrategy` (structs.go:5406-5449) gives per-group rolling updates: `MaxParallel`,
health checks, `MinHealthyTime`, `HealthyDeadline`, `ProgressDeadline`, `Canary`,
`AutoPromote`, `AutoRevert`; the deploymentwatcher drives promotions/rollbacks
(deployment_watcher.go:158-242 unhealthy→fail+autorevert with rollback-validity guard :246-257,
autoPromoteDeployment :283-317).
Forge `deployReplicas` dispatches all placements immediately with no concurrency bound, no health
window, no canary concept, and marks the app `running` the moment dispatch succeeded
(replicamanager/service.go:346-356, `updateAppStatus` :810-820 counts *provisioned* replicas).
The only post-hoc correction is the 60 s reconciler flipping statuses to degraded/failed
(:785-806). There is no automatic rollback of any kind — closest analogues (env manifests /
zero-downtime migrations) live outside the placement dimension. For game servers (crash-looping
images are routine) the absence of `ProgressDeadline`+`AutoRevert` is the largest operational gap.

### C13. Node draining: deadline-driven migrator vs plan-ledger + bounded evacuator
Nomad drain is a first-class subsystem: `DrainStrategy{Deadline, ForceDeadline, StartedAt}`
(structs.go:1908-1951) with force-on-deadline semantics, and the drainer migrates allocs only while
healthy count stays above `tg.Count - tg.Migrate.MaxParallel`, keeping service availability
(drainer/watch_jobs.go:415-425). Migration pacing uses the group's `MigrateStrategy`
(structs.go:6821-6872: MaxParallel, HealthCheck, MinHealthyTime, HealthyDeadline).
Forge splits this into `drain.Service` (a durable *ledger*: Begin/Progress/End/Cancel +
event subscriber mirroring membership events, drain/service.go:47-184) and
`evacuationplanner.Service` (candidate search + executor). Pacing is a compile-time constant
`maxConcurrentEvacuationMigrations = 2` (evacuationplanner/service.go:188) applied globally, not
per workload criticality; there is no health-gating before killing the next item, no deadline/force
semantics (an item stuck `preparing` blocks a slot until its migration terminalizes — mitigated by
the stale-operation reaper elsewhere). Storage-locality awareness (`StorageLocalOnly` items are
marked ineligible, service.go:561-576,720-728) is genuinely *ahead* of Nomad CE, which delegates
that to host-volume policies. Capacity reservation during planning (`reserved` impact map
:507-516) parallels Nomad's plan-applier discounting.

### C14. In-place updates: diff-driven reuse vs always-recreate
Nomad distinguishes in-place vs destructive updates via `tasksUpdated` field-diff
(util.go:167-297) and reuses the alloc (new resources only) whenever drivers/config unchanged
(inplaceUpdate :569-701, genericAllocUpdateFn :805-940). Forge `ReplaceInstance` always recreates:
mark `removing`, place fresh, re-dispatch (replicamanager/service.go:561-652); `ScaleApp` likewise
never reuses. Acceptable for game workloads (image change ≈ recreate anyway), but env-only changes
also force recreation and full re-provision latency.

### C15. Scheduling throughput: iterator stacks + power-of-two choices vs serialized engine
Nomad evaluates candidates through lazy iterator chains with a log₂(n) cap for services and 2 for
batch (power-of-two choices, feasible/stack.go:79-100), shuffles nodes for tie-breaking
(`ShuffleNodes` :82), and runs many workers concurrently with optimistic plans.
Forge `Engine.Place/PlaceAll` hold one process-wide mutex for filter+score+select
(engine.go:37-48,79-90), and `ScoreNodes` fetches a capacity snapshot per node sequentially
(services/scheduler/service.go:282-294). At fleet sizes this is a serialization point (F9), and
tie-breaking depends on input order because `sort.Slice` is unstable (strategy.go via
engine.go:107-109).

### C16. Priority & preemption vs FIFO-by-tick
Nomad jobs carry `Priority` (structs.go:4418-4420) enabling preemption of lower-priority allocs
when capacity is short (preemption plumbing in generic_sched.go:906-954 and
PreemptionScoringIterator rank.go:1042+). Forge has no priority anywhere in
`WorkloadRequest`/`Candidate` (strategy.go:46-56) — a game-server cannot preempt a staging batch,
and under contention placement is first-come-first-served by tick order.

---

## 3. Logic findings

### F1. Soft-constraint bonus magnitudes destroy the score space (HIGH)
`placement/constraints.go:59-63`: each satisfied soft constraint adds `+1e12`; each unsatisfied one
subtracts `−1e10`. Base scores are ≤ ~3 (`LeastLoadedScorer`, strategy.go:91-104), ≤ 1
(SpreadScorer :141), or ≤ 1 (RandomScorer :162-173). Consequences:
- One unsatisfied soft constraint (−1e10) beats any number of satisfied ones (+1e12 each) only when
  ≥1 satisfied — fine — but *two* unsatisfied (−2e10) versus one satisfied (+1e12) still prefers the
  latter; meanwhile storage-locality adjustments in `ScoreNodes` (±1e10/+1e8,
  services/scheduler/service.go:317-323) sit *between* these scales, so a locality penalty can be
  masked or amplified depending on soft-constraint count. The layers were clearly tuned independently
  (compare `PreferredNode +1e9` :307 < locality penalty 1e10 < soft bonus 1e12).
- `ExplainScores` reports `SoftConstraintBonus: 1e12` alongside base `1.7` (explain.go:89-106),
  producing nonsense explanations.
Nomad avoids this by normalizing every factor to roughly [−1, 18] and averaging
(rank.go:1004-1038). Recommend: clamp soft bonuses to a small multiple of the base-score range and
document factor precedence explicitly.

### F2. Spread strategy is misnamed anti-co-location; true spread absent (MEDIUM-HIGH)
`SpreadScorer` (strategy.go:137-147) scores `1/(1+ServerCount)` — minimizing per-node server count.
It cannot express "spread across datacenters/zones" (Nomad `Spread.Attribute`, structs.go:10204),
has no weights/target percentages, and is silently *doubly* applied in the replica path where
`scoreReplicaCandidate` subtracts another `0.1*count` (replica.go:170-175) — so choosing the
"spread" strategy yields different cumulative behavior than choosing least-loaded with the replica
penalty, with no documentation of intent. No Forge mechanism distributes replicas across regions;
`RegionID` filtering is equality-only (constraints.go:112-128).

### F3. Reservation lifecycle race: confirm-after-dispatch leaves double-book window (MEDIUM)
`deployReplicas` creates a reservation, inserts the instance row, dispatches, *then* confirms the
reservation (replicamanager/service.go:219-356). Between dispatch and confirm the capacity is
counted twice-safe (fine), but on dispatch failure the code cancels the reservation *after* having
already marked the instance failed (:301-310) while `safeStopReplicas`/`DeleteApp` cancel
reservations for instances whose rows they delete (:491-495,533-537) — however
`RetryFailedPlacements`→`ReplaceInstance` path never cancels the old reservation; it relies on the
instance status becoming `removing` and `existingNodeMap` excluding it (services/scheduler/
service.go:628-634), yet the *active reservation* row still holds capacity until manually expired
(reservations expire only via the 1-min `ExpireReservations` ticker, reservations/service.go:59-69).
A flapping instance can therefore pin phantom capacity indefinitely if `UpdateInstanceStatus` fails
silently (`_, _ =` throughout, e.g. :302). Nomad's plan-applier never has orphaned holds because
capacity is derived from committed allocs, not from a parallel ledger. Recommend: derive
available-capacity from instances ∪ active-reservations with reservation cancellation tied to
instance terminal states in one transaction (partially done in store_capacity.go per comments in
store_placement_intents.go:39-43 — verify the replace-path calls it).

### F4. Inconsistent preferred-node semantics and scales between paths (MEDIUM)
Single-server placement: `PreferredNode` ⇒ `+1e9` and reason overwrite (services/scheduler/
service.go:306-309). Replica placement: `PreferredNode` ⇒ `+1` (placement/replica.go:177-179),
which is negligible against soft-constraint bonuses (±1e10+) and comparable to the spread penalty
(0.1·count). Result: "prefer this node" is nearly guaranteed to win in path A and routinely loses
to a single soft constraint in path B. Also `ExplainReplicaPlacement` omits the preferred bonus and
capacity checks entirely (it swallows scorer errors with `_` at replica.go:241-245), so the
explanation view can recommend a node the real placer would reject for capacity, and vice versa.

### F5. Replica capacity pre-debit uses max-size × count, not actual mix (LOW-MEDIUM)
`PlaceReplicas` discounts each already-occupied node by `count × max(existingCPU/Mem/Disk)`
(replica.go:59-74). If existing instances are heterogeneous (possible after resize features), the
discount overshoots, shrinking usable capacity and causing spurious "no viable node for replica"
failures. Under-debiting is impossible, so this errs safe, but it silently reduces packing density.
Nomad computes proposed usage from actual allocs (ProposedAllocs, rank.go:222).

### F6. No reschedule backoff / attempt ceiling ⇒ crash-loops amplify (HIGH)
As detailed in C10: `RetryFailedPlacements` (replicamanager/service.go:883-910) has no
`ReschedulePolicy` analogue — no attempts/interval, no exponential/fibonacci delay, no
`Unlimited=false` cap. A bad image deployed fleet-wide produces a permanent 1-per-minute
replace-storm per failed instance, each iteration re-dispatching Beacon commands and churning
placement_decisions rows. It also runs inline in the shared reconcile goroutine with
`time.Sleep(100ms)` per app (:904), stalling status reconciliation for all other apps. Minimum
viable fix: per-instance failure counter + exponential backoff timestamp consulted by
`ReplaceInstance`.

### F7. Missing blocked-eval equivalent ⇒ slow reaction to freed capacity (MEDIUM)
Capacity freeing (server stop, node join, reservation expiry) does not trigger pending placements;
they wait for the next reconcile tick or manual action. Nomad's `BlockedEvals` + `UnblockNode`
(blocked_evals.go:478-604) makes retry latency ~immediate and scoped to eligible classes. Cheap
approximation for Forge: publish an event from `ExpireReservations` and node-state transitions that
kicks `RetryFailedPlacements` for affected apps.

### F8. Replacement ignores the failing node (MEDIUM)
`ReplaceFailedInstance` builds candidates from `FilterNodes(...)` with empty region/runtime
constraints (services/scheduler/service.go:620) — it does not exclude or penalize
`inst.NodeID`, the node that just failed the workload (only implicit exclusion happens if the node
is offline/draining; a *flaky-but-online* node remains the top scorer thanks to its high free
capacity after the workload died). Nomad feeds prior failures into `PenaltyNodeIDs`
(generic_sched.go:800-813) and the `NodeReschedulingPenaltyIterator` (rank.go:875-910).

### F9. Global placement mutex serializes the whole cluster (LOW-MEDIUM)
`Engine.Place`/`PlaceAll`/`PlaceReplicas` all funnel through `e.mu` held across scoring
(engine.go:38-40,79-81; replica.go calls `e.scorer`/`e.checker` under callers' locks). With N
concurrent deploys, placement throughput is 1-at-a-time regardless of cores; combined with per-node
sequential snapshot queries in `ScoreNodes` (services/scheduler/service.go:282-294), tail latency
grows linearly with fleet size. The mutex appears to protect scorer/checker state, but both are
stateless (`ConstraintChecker struct{}`, constraints.go:32; RandomScorer has its own mutex
strategy.go:149-157) — likely removable outright.

### F10. nodeautoscale scale-in deprovisions hardcoded AWS provider and races the drain (MEDIUM)
`ScaleIn` starts the drain and, when `deprovision && node.SchedulerType != ""`, immediately calls
`s.cloud.DeprovisionNode(ctx, cloud.ProviderKind("aws"), nodeID)` (nodeautoscale/service.go:380-394):
(i) provider is hardcoded `"aws"` regardless of the node's actual provider — an Hetzner/DO node
would hit the wrong adapter; (ii) deprovision fires synchronously although the doc comment claims
"finally deprovisions ... when the node's drain completed" — the code awaits nothing; evacuating
workloads lose their source mid-flight. Also `leastLoadedNode` ranks by
`AllocatedCPU + AllocatedMemory + AllocatedDisk` (:288) — adding CPU shares to MB is unit soup;
with typical values disk-MB dominates and the choice degenerates to "least disk used".

### F11. Scale-down victim selection is positional, not policy-based (LOW)
`ScaleReplicas` scale-down removes the last N active instances by list order
(services/scheduler/service.go:578-595), ignoring node balance, newest/oldest, or per-instance load.
Combined with F2 (no true spread), repeated scale up/down cycles can consolidate an app onto one
node. Nomad tracks alloc-name indexes and lets the reconciler pick deterministically with spread
scoring still active (reconciler computeStop, reconcile_cluster.go:1101+).

### F12. Deprecated Nomad adapter writes HCL via fmt and shells out to CLI (INFO, hygiene)
Even though deprecated, `scheduler_nomad.go` remains compiled-in: job specs built by string
formatting (:299-382) — env values are `%q`-escaped but injection via crafted env keys/values is
fragile; `Restart` stops every alloc via `alloc stop` (:140-168) which triggers Nomad's reschedule
logic rather than a clean job restart; `GetResources` returns requested resources, not usage, and
drops CPU (:268-297). If the type survives, route it through the Nomad HTTP API; otherwise delete
per the file's own instruction (:11-14).

---

## 4. What Forge does better (credit where due)

- **Durable placement decisions + explainability**: `placement_decisions` with per-candidate reasons
  and `ExplainPlacement`/`ExplainScores` (explain.go) exceed Nomad's ephemeral
  `AllocMetric.ScoreMetaData` retention (explain API exists but metrics aren't persisted per decision).
- **Reservation intents with restart recovery**: pending intents/expired reservations are swept on
  boot (`RecoverPendingPlacementIntents`, store_placement_intents.go:185-198) — Nomad relies on raft
  replay for similar guarantees.
- **Storage-locality-aware evacuation** (evacuationplanner/service.go:561-576,691-728) — refuses to
  migrate local-only volumes instead of stranding them; Nomad CE pushes this to volume ACLs/policies.
- **Safe-mode node autoscaling**: suggest-then-confirm with an audited event ledger
  (nodeautoscale/service.go:155-191) is more conservative than most cluster-autoscaler defaults.
- **Zero-capacity node rejection metric** (`ZeroCapacityRejectionsTotal`,
  services/scheduler/service.go:196-199) catches misconfigured nodes Nomad silently treats as
  exhausted.

## 5. Priority recommendations

1. Fix score-scale layering (F1) — introduce a bounded factor model (normalize base ∈ [0,1], soft
   bonus ≤ ±2, locality ≤ ±1, preferred ≤ +0.5), keep reasons per factor.
2. Add a minimal ReschedulePolicy (attempts + exponential backoff + penalty-node memory) to
   instances (F6, F8) — schema: `failure_count`, `next_retry_at`, `last_failed_node_id`.
3. Event-trigger retries on capacity change instead of pure 60 s polling (F7).
4. Implement region/zone-aware spread for replicas (F2) — even a simple "max distinct RegionID
   coverage" tie-break would capture most of the value.
5. Bound deploy concurrency and gate `running` on a health probe window (C12-lite).
6. Make `Engine` lock-free (stateless members) and parallelize snapshot fetches (F9).
7. Fix nodeautoscale provider hardcoding and await drain completion before deprovision (F10).
