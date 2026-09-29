# Phase 5 — Subagent 05: Cross-Cutting Architecture / Reliability Synthesis (capstone)

Cluster: `nomad`, `incus`, `netbird`, `longhorn`, `rancher`, `river` (synthesis across all six).
Forge trees inspected this pass: `forge/api/internal/eventstore/*`, `forge/api/internal/events/*`,
`forge/api/internal/services/{reconciler,recovery,failover,fencing,heartbeatmonitor,scheduler}`,
`forge/api/cmd/api/main.go` wiring, `forge/api/internal/store/store_failover.go`, beacon grep-verification.
Sibling reports cited, not re-derived: [S1] subagent-01-nomad-placement.md,
[S2] subagent-02-river-queue.md, [S3] subagent-03-incus-netbird-runtime.md,
[S4] subagent-04-longhorn-rancher-storage.md (all in audits/phase-05/).

---

## 0. Executive verdict

Forge has quietly assembled the **right skeleton** for a single-writer, desired-state control plane:
a durable desired/actual state split on servers and nodes, an append-only decision/plan/event ledger
in Postgres, a lease-based outbox with dead-lettering, a hysteresis heartbeat classifier, and
generation+lease fencing tokens. Every one of these has a direct analogue in the reference systems,
and several are *better* than what Nomad CE or Rancher expose.

But the skeleton is not load-bearing yet. Three structural facts undercut it:

1. **The durable event pipeline is write-only in production.** The relay that gives the `events`
   table its purpose has zero production subscribers; all automation runs over the in-memory
   registry of whichever process happened to publish (AF-1).
2. **Nothing elects a leader.** ~30 background daemons start per API instance with only startup
   jitter as coordination; the only leader-election code in the repo is inside the deprecated River
   fork that subagent-02 confirmed is unwired ([S2] §0). Failover cooldowns live in process memory.
   The references solve this one way or another — Nomad via Raft leadership (leader.go:268,413-430),
   River via `river_leader` TTL rows, Longhorn via Kubernetes controller semantics, Incus via dqlite
   membership (AF-2).
3. **The fence token is written but never enforced.** Generation/lease bumps happen in recovery and
   fencing, but no action path — power ops, restarts, dispatches — validates them, and the beacon has
   zero occurrences of "generation" or "fence". Compare NetBird, where policy removal revokes mesh
   access on the next NetworkMap push, i.e., enforcement is the mechanism (AF-3, [S3] LF-5).

Everything else in this report is detail around those three facts plus the traps already surfaced by
the sibling reports (duplicated engines [S2], phantom providers [S3], locality vocabulary drift [S4]).

---

## 1. Cross-cutting comparison matrix (14)

| # | Dimension | Reference pattern(s) | Forge today | Assessment |
|---|---|---|---|---|
| C1 | State model | Nomad: declarative Job→Allocation diff-reconciled every eval (structs.go:4388, generic_sched.go:104; [S1] C1-C2). Longhorn: spec/status CRDs reconciled by controllers (crds.yaml:4424+; [S4] C1) | `servers.desired_state` vs `actual_state` columns; nodes carry both heartbeat and actual state (reconciler/service.go:486-535); compose stacks diffed via config hashes (diff.go:40-117, trigger.go:70-175) | **Genuinely right shape.** Forge's spec/status split is real, persisted, and hash-diffed — closer to Longhorn's CRD model than to Pterodactyl-style imperative panels. Gap shared with [S1] C2: convergence only fixes status labels + restarts; it does not re-place missing replicas |
| C2 | Event model / durability | River: enqueue-in-tx (`InsertTx`, client.go:1831-1854; [S2] row 12). Nomad: raft-log-backed evals, broker ephemeral above them ([S2] row 16) | `events` table w/ claim leases + DLQ + retention (eventstore/store.go:67-93,157-199; migration.go:19-53); `OutboxPublisher` = DB insert then in-memory fan-out (store.go:214-222); **no tx-bound publish API** (grep: no `PublishTx` anywhere) | Half-right primitive, wrong integration. The store is a competent lease-outbox (see §3 credit), but events are published *after* business commits, so the classic outbox guarantee (no event without committed state, no state without event) does not hold — same defect class as [S2] LF-7 |
| C3 | Event consumption | All six references route side-effects through their durable substrate (Nomad evals, River jobs, Longhorn CRs, NetBird NetworkMap pushes) | Relay polls every 5 s, claims batches, marks dispatched (outbox.go:74-122)... to **zero** subscribers: `Relay.Subscribe` has test-only callers (outbox.go:39-43; grep confirms store_test.go:362,395 only); `EventRelay` sits unused on http/server.go:148; real consumers register on the in-memory `Registry` (main.go:441,946,1026-1077,1236-1243) | **Broken loop** — AF-1. The system pays write amplification (INSERT per publish + UPDATE per relay tick) for durability it never reads |
| C4 | Consistency / concurrency | Nomad: plan-apply arbitration + retryMax (generic_sched.go:291-316; [S1] C4). River: SKIP LOCKED claim, guarded CAS completion (`JobSetStateIfRunningMany`; [S2] rows 2,10) | DB-as-arbiter: reservation validation under row locks ([S1] C4), failover incidents serialized by `pg_advisory_xact_lock` + window check (store_failover.go:130-165), reconcile-plan dedupe/TTL (reconciler/service.go:226-230, 680-702) | Forge's Postgres-as-single-writer is a legitimate simplification of Nomad's plan queue and is applied inconsistently: the best guard (advisory-lock incident dedupe) exists only on the node-offline path; crash/failure paths use in-memory cooldowns (failover/service.go:99,446-453) |
| C5 | Retry & backoff | River: attempt^4 jittered backoff, scheduled_at promotion, snooze ([S2] rows 5,7,11). Nomad: ReschedulePolicy delay functions + penalty nodes (structs.go:6611; [S1] C10) | Registry subscriber retries 3× with doubling+jitter then in-memory failure record (registry.go:193-215,237-268); relay retries ≤5 failures then dead-letters (store.go:15, outbox.go:209-218); reconciler restart caps/cooldowns/decay constants (service.go:26-43, 619-674) | Per-mechanism retry exists everywhere but **policy lives nowhere**: caps/backoff constants are scattered per package, and placement retries have no policy at all ([S1] F6). References centralize retry policy on the record; Forge centralizes it nowhere |
| C6 | Failure detection | Rancher: active 15 s tunnel ping + pushed health sync ([S4] C11-C12). Incus: offlineThreshold heartbeat (heartbeat.go:113; [S3] row 8). NetBird: continuous client-side probes | Push heartbeats → history-table classifier with warning/unreachable/offline/recovering/reconciling states and consecutive-success hysteresis (heartbeatmonitor/service.go:266-313), alerting hook (:252-259), future-timestamp guard (:274-276) | **Richer than all six** on classification; weaker than Rancher on proof (no active round-trip). Keep the state machine, add Rancher's echo ([S4] rec 7) |
| C7 | Failure recovery orchestration | Longhorn: replica rebuild-with-existing-data, replenishment wait ([S4] C3). Nomad: reschedule evals w/ penalties. Incus: evacuation on member loss | Recovery Coordinator: plan→items→verify-backup→restore→await-beacon→health-gate lifecycle with 10-min ack timeout, reservation release on timeout, migration-executor reuse instead of duplication (recovery/service.go:335-372, 403-445, 100-110) | The lifecycle and the ack-timeout reconciliation are more explicit than anything in Rancher/Longhorn charts. Two flaws: recovery planning duplicates fencing logic inline (service.go:642-650 vs fencing.go:34-41 — two writers of generation/lease), and `RecoverFromUnavailable` (recovery_ops.go:139-255) bypasses the incident dedupe the failover path relies on (AF-6) |
| C8 | Idempotency | River: unique keys w/ `UniqueSkippedAsDuplicate` signal ([S2] row 6). Nomad: alloc-name dedupe ([S1] C1) | SHA-idempotent job inserts ([S2] LF-5 fragmentation); failover incident CAS (store_failover.go:150-152 returns created=false); reconcile plan hash-dedupe window (reconciler/service.go:680-702); destructive actions require explicit confirm (trigger.go:211-213; recovery_ops.go:105-137) | Intent is present at each layer; namespaces and signals are fragmented ([S2] LF-5). Notably Forge's "destructive plan needs confirm" gate is *stricter* than Nomad's AutoRevert-less CE default — worth keeping as a first-class abstraction |
| C9 | Scalability of the loop | Nomad: leader-only subsystems + parallel schedulers ([S1] C15). River: leader-elected maintenance ([S2] rows 8,14) | ~30 `.Start()` daemons per instance (main.go:521,533,787,859,990,995,1016,1028,1072,1106,1109,1147,1150-1167,1194-1198,1235,1304), each with own ticker; multi-instance safety = startup jitter comments (reconciler/service.go:187-189 "avoid thundering herd when multiple API instances"; heartbeatmonitor/service.go:140-142) | Correct only while deployment stays single-instance, which the jitter comments show is *known*. The moment main.go runs twice: duplicate periodic jobs ([S2] LF-3), N× reconcile/evacuation/failover evaluation, racing restart loops (AF-2) |
| C10 | Security boundaries | NetBird: enrollment→identity→policy→NetworkMap enforcement (account.go:1935,457; [S3] rows 10-11). Incus: project namespaces ([S3] row 17) | Org/project/env tenancy tables exist (store_tenancy.go:91-469); but placement lists **all** nodes globally (scheduler/service.go:88-108 `ListNodes`), `domain.PlacementRequest` carries no tenant field, and the event Envelope has no tenant/resource-version (events/event.go:196-205); events table schema likewise tenantless (migration.go:19-31) | Tenancy stops at the HTTP layer. Scheduling path, event bus, and recovery planner are tenant-blind — any compromise or bug in one tenant's workload decisioning is fleet-global (AF-7) |
| C11 | Plugin/provider models | Incus: drivers registered by name with per-create support checks (load.go:28, instance_utils.go:722-725). Longhorn: storage driver registry. River: driver abstraction | Control plane registers 7 providers incl. phantom LXC/KVM (main.go:392-399; [S3] LF-1); beacon factory implements 5 (factory.go:19-31); capability registry is dead code ([S3] LF-3); backup providers registered globally by string (main.go:973-976) — a second, *working* mini-plugin system nobody unified | The trap here is not extensibility but **honesty**: two registries disagree about reality, and nothing gates scheduling on declared capability ([S3] rec 6). Backup provider registry shows Forge can do this correctly when the abstraction matches one clear seam |
| C12 | Configuration model | Nomad: agent config + job spec validation at registration. River: ClientConfig w/ documented defaults. Longhorn: values.yaml + per-volume overrides | Four competing homes: package constants (reconciler/service.go:26-43), normalizeConfig defaults (heartbeatmonitor/service.go:89-97,399-423), env vars read ad hoc in main.go (PANEL_URL :455, GITOPS_WORKER_ID :516, BACKUP_RETENTION_DAYS :972), and DB policy rows (failover policies, failover/service.go:62-75) — plus in-memory fallback logic that *overrides operator intent* by defaulting no-policy nodes to Evacuate (failover/service.go:199-210) | Config sprawl is survivable; the failover default-Evacuate fallback is not — silence should mean Notify, not auto-evacuation of local-only data (AF-8, interacts with [S4] F5) |
| C13 | Observability | River: events/metrics hooks/error history ([S2] row 15). Nomad: ScoreMetaData explanations ([S1] C6) | Metrics snapshots per service (reconciler/service.go:56-69; heartbeatmonitor/service.go:26-34; failover/service.go:89-94); persisted plans/events/diffs (trigger.go:340-362); placement explainability ([S1] §4); wildcard observer + webhook subscribers (main.go:1076-1077) | Strong for a panel: decisions/plans/drifts are queryable artifacts, not log lines. Weakness: no error *history* per job ([S2] row 9), metrics are pull-from-process rather than exported counters, and correlation IDs propagate well (context.go:12-27) but terminate at the in-memory bus |
| C14 | Single-writer discipline | Nomad: leader runs all watchers/dispatchers under leadership (leader.go:268,413-430 establish; :1445-1454 revoke). River: elector TTL table (elector.go:26-31). Longhorn: one controller per CR | Leader election code exists **only** in the unwired River fork (queue/leader.go:23-133; [S2] §0). Live guards: advisory-lock incident dedupe (store_failover.go:136), SKIP LOCKED claims (queue store, eventstore/store.go:71-78), reservation row locks ([S1] C4) | Row-level primitives are right; the missing piece is exactly what Nomad/River add on top — one *process-level* arbiter deciding who runs periodic/maintenance/reconciliation. Postgres can provide it (advisory lock or TTL row) without adopting Raft |

---

## 2. Architecture findings

### AF-1 — The durable event pipeline is write-only; the outbox guarantee doesn't exist (HIGH)
Evidence chain:
- Production publishes go through `OutboxPublisher.Publish`: INSERT into `events`, then synchronous
  in-memory delivery to the local Registry (eventstore/store.go:214-222). If the process dies between
  business commit and publish, or mid-delivery, the durable copy either doesn't exist or has no reader.
- There is no transactional binding: `Publish` accepts only the pool executor; no call site passes a tx
  (grep for `PublishTx|WithTx` finds none). River solves this exact problem with `InsertTx`
  (client.go:1831-1854; [S2] row 12); Nomad keeps evals in raft so the broker can always replay.
- The relay — the component that would make the log useful — delivers to whatever subscribed via
  `Relay.Subscribe` (outbox.go:39-43). Grep across `forge/api` shows only tests subscribe
  (store_test.go:362,395). `http.Server.EventRelay` (server.go:148) is never dereferenced.
  So every event is claimed (outbox.go:105), "delivered" to an empty handler list
  (deliverWithRetries over zero subs returns nil), and marked dispatched (outbox.go:162) within 5 s.
Consequences: cross-instance automation is impossible today (a NodeOffline observed by instance A
cannot trigger failover logic living on instance B); the events table functions as a 7-day audit log
(prune at outbox.go:95) at the price of two writes per publish; and every consumer that *thinks* it has
durability (fencing, failover, traffic manager, LB, notifications — main.go:441-1243) actually has
best-effort in-memory delivery whose retry budget (registry.go:193-215) ends at the process boundary.
Fix direction: add `PublishTx(ctx, tx, envelope)` and migrate the ~10 business-critical publish sites;
register the existing consumers (fencing, failover, tm, lb) on the relay instead of (or in addition to)
the registry; treat registry delivery as a fast-path cache of the durable stream, not the source.

### AF-2 — No leader election; ~30 uncoordinated daemons per instance (HIGH)
Every reliability subsystem starts unconditionally per process (main.go lines listed in C9): two
queue-like engines ([S2] LF-3/LF-6), the reconciler, heartbeat monitor, failover loop, evacuation
planner, migration watcher, replicamanager reconcile, periodic scheduler, mail/webhook workers,
gitops controller, observability samplers. Coordination consists of:
- startup jitter explicitly rationalized for multi-instance restart storms (reconciler/service.go:187-189;
  heartbeatmonitor/service.go:140-142) — jitter desynchronizes writes but does not deduplicate *work*;
- in-memory cooldown maps in failover (failover/service.go:99, 446-453) which protect only the
  process that saw the event;
- one genuine cross-instance guard: `CreateFailoverIncident`'s advisory lock + active-window check
  (store_failover.go:130-165) — proof the team knows the pattern, applied once.
Meanwhile both references that faced this problem solved it with a tiny elected-leader gate: Nomad
enables deploymentWatcher/volumeWatcher/periodicDispatcher only under leadership (nomad/leader.go:413-430,
revoked at :1445-1454), River gates all maintenance on the `river_leader` TTL row (elector.go:26-31;
[S2] row 14). Forge even *contains* a portable implementation — the unwired fork's `Elector`
(queue/leader.go:23-133) built on the same TTL-row SQL.
Consequences scale with ambition: today duplicated periodic jobs and double reconcile loops ([S2] LF-3);
tomorrow two instances concurrently running `runFailoverAction` for different-but-overlapping node
events, each creating recovery plans against the same capacity pool. Minimum fix: one
`pg_try_advisory_lock(hashtext('forge-leader'))`-style election wrapper (or resurrect the fork's elector
against a small `forge_leader` table) gating the ~8 maintenance-class daemons; leave per-event work on
SKIP LOCKED claims.

### AF-3 — Fencing writes tokens nothing enforces, fires on the wrong edge, and is implemented twice (HIGH)
- Wrong edge: `FenceNode` triggers on `EventNodeRecovered` (fencing.go:20-27) — i.e., the partition
  window Forge actually needs protection from is unfenced until the node comes back ([S3] LF-5 made
  this finding for the runtime dimension; architecturally it means the fence cannot prevent split-brain
  *during* the outage, only mark suspicion after it).
- Unenforced: generation/lease are written by fencing (fencing.go:36-38) and by recovery planning
  (recovery/service.go:642-650; recovery_ops.go:201-204), and read by exactly one non-store site —
  the stale-workload counter in `ReconcileReconnectingNode` (reconciler/service.go:559-565). No power
  op, dispatch, or migration checks the token before acting; grep of `beacon/internal` shows zero
  occurrences of generation/fence, so node-side rejection doesn't exist either. Contrast NetBird, where
  removing a peer's policy takes effect via the next NetworkMap push — the enforcement *is* the
  protocol (account.go:457; [S3] row 9) — and Longhorn, where attach ownership is decided from
  `currentNodeID` observed state, not advisory integers.
- Duplicated: two independent implementations of "bump generation + set lease" exist (fencing.go:34-41
  vs recovery/service.go:644-650), with different lease durations (24 h vs 1 h) and different rollback
  behavior (none vs full rollback with `errors.Join` at recovery/service.go:653-657,670-677). They also
  race: recovery's `planServer` bumps generation per server while fencing may bump the whole node's
  servers on recovery — last writer wins on lease expiry, silently.
Fix direction: single `fencing.Service.FenceServer/FenceNode` used by both callers; enforce by making
`clustermanager.RequestServerPower`/dispatch compare the server's current generation against the
caller's claimed generation (CAS in `UpdateServerGeneration`'s WHERE clause), and reject stale-token
commands at the beacon using a field the create/power handlers already parse.

### AF-4 — Six controllers, one server, no serialization on the act path (MEDIUM-HIGH)
For a given unhealthy server these independent loops can all decide to act within one interval window:
1. reconciler desired-vs-actual power switch (reconciler/service.go:523-535),
2. reconciler health-recovery restarts w/ caps (service.go:581-676),
3. replicamanager failed-instance replacement ([S1] F6),
4. crash detector auto-restart/suspend (main.go:947-970),
5. failover executor restart-all or evacuate (main.go:862-924),
6. recovery coordinator restore-to-new-node (recovery/service.go:335-372).
Each has *internal* safety (cooldowns, attempt caps, node-operable check service.go:710-727), and the
incident dedupe covers #5's node-offline entry. But there is no cross-controller lease: e.g. while a
recovery item is `AwaitingBeacon`, nothing stops controller #1 from issuing StartServer against the old
node's dead workload, or #4 from counting the crash and suspending the server the restore is targeting.
Nomad prevents this class by funneling every mutation through one scheduler worker per object
(generic_sched.go eval serialization); Kubernetes/Longhorn through per-object work queues with
generation preconditions. Forge's cheapest equivalent: reuse the existing generation column as a
per-server act lock — controllers read generation, act, and their status writes carry
`WHERE generation=$n`, turning AF-3's enforcement gap into the serialization mechanism.

### AF-5 — In-memory registry semantics leak into durability-critical paths (MEDIUM)
`Registry.Publish` fans out one goroutine per subscriber and **blocks the publisher** on all of them
(registry.go:167-189), with a self-imposed 30 s deadline (registry.go:151-155) and per-subscriber retry
sleeps up to ~700 ms × 3 (registry.go:193-215). Because `OutboxPublisher.Publish` invokes this
synchronously after the DB insert (store.go:218), any code that publishes inside a request or worker
path inherits worst-case latency from its slowest subscriber — e.g. wildcard observers and webhook
dispatch (main.go:1076-1077) sit on every event. Additionally the "dead letter" for failed subscribers
is an in-memory map capped at 10 k entries with arbitrary oldest-eviction (registry.go:51,237-268) —
failed deliveries vanish on restart, indistinguishable from success. This is acceptable for UI-grade
notifications; it is not acceptable as the transport for fencing/failover triggers (AF-1/AF-3 depend on
it today). Fix direction follows from AF-1: durable-first delivery, registry demoted to best-effort
fan-out with `go` per subscriber and no publisher blocking.

### AF-6 — Idempotency is per-entry-point, not per-intent (MEDIUM)
The same operator intent — "recover node X" — has three entries with different guarantees:
- automatic: `HandleNodeOffline` → incident CAS (deduped cross-instance, store_failover.go:130-165);
- manual API: `RecoverFromUnavailable` (recovery_ops.go:139-255) creates a fresh plan every call with
  only the node-state precondition (must be non-healthy, :147-153) — two clicks produce two plans,
  two sets of reservations and generation bumps;
- `CreatePlan` (recovery/service.go:148-256) similarly lacks an intent key; it is safe from the
  automatic path only because the incident lock upstream happens to fire once.
River signals dedupe outcomes to callers (`UniqueSkippedAsDuplicate`; [S2] row 6) precisely so callers
can build idempotent flows; Forge's `ON CONFLICT DO NOTHING` job inserts return the old row silently
([S2] LF-5), and plan creation doesn't try. Fix direction: key recovery plans by `(node_id, reason,
active-window)` with the same advisory-lock CAS used for incidents, and return `created=false` so
API callers get the existing plan instead of a twin.

### AF-7 — Tenant isolation stops at the HTTP handler (MEDIUM-HIGH)
Org/project/environment hierarchy exists (store_tenancy.go:91-469) and HTTP handlers enforce roles,
but everything below is tenant-blind: `PlaceServer` ranks the global node list
(scheduler/service.go:104), `PlacementRequest` has no tenant field (domain.go; grep: zero hits),
recovery planning enumerates all servers of a node regardless of ownership (recovery/service.go:276-292),
and the event envelope/table carry no tenant attribute (events/event.go:196-205; migration.go:19-31) so
audit trails cannot be partitioned either. NetBird's model is the reference: account scoping is a
property of the *data*, and every push/route/policy derives isolation from it ([S3] rows 10-11,17);
Incus achieves the same with project-scoped profile namespaces ([S3] row 17). For Forge the near-term
risk is blast radius (a misbehaving tenant's affinity constraint or autoscaler policy influences
fleet-wide placement), and the long-term risk is that retrofitting tenant columns into events/plans/
decisions after they accumulate is a migration you don't want. Fix direction: add tenant/project ID to
PlacementRequest, PlacementDecision, ReconcilePlan, and Envelope now (cheap while volumes are small);
filter candidate nodes per project quotas later.

### AF-8 — Config sprawl plus a dangerous default in the failover classifier (MEDIUM)
Four configuration regimes coexist (C12). The acute issue: when no enabled policy matches, Forge
consults the workload classifier and, if that yields anything but Notify, **overrides toward Evacuate**
— and if the classifier errors or says Notify, HandleNodeOffline still substitutes a synthetic
Evacuate policy (failover/service.go:181-211, esp. the `matchingPolicy == nil → Action:
FailoverActionEvacuate` branch at :201-206). Combined with [S4] F5 (locality defaults to "replicated"
absent evidence), the composite default behavior for a dead node full of ordinary local-data game
servers is automatic evacuation. Every reference errs the other way: Nomad requires an explicit
reschedule stanza, Longhorn defaults `NodeDownPodDeletionPolicy: do-nothing` ([S4] C9), Rancher
policies must be enabled. Fix direction: delete the Evacuate fallback; absence-of-policy ⇒ Notify +
alert; make the classifier's evidence bar explicit (verified off-node artifact within RPO, per [S4]
rec on defaulting to local_only).

---

## 3. What Forge gets right (keep and strengthen)

These are architecturally sound relative to the cluster and should be treated as load-bearing:

1. **Postgres-as-single-writer instead of a bespoke consensus layer.** Reservation row-locks ([S1]
   C4), advisory-lock incident CAS (store_failover.go:130-165), SKIP LOCKED claims in both queue and
   event stores (eventstore/store.go:71-78). This is the correct scaling-down of Nomad's plan-applier
   for a panel-scale fleet; what's missing is uniform application (AF-2/AF-4), not a different mechanism.
2. **The lease-based outbox primitive itself.** `ClaimPending` with per-relay claim tokens, expiry-
   reclaimable leases sized from batch math ((batchSize+1)×timeout, outbox.go:103-105), bounded failure
   counts, atomic move-to-DLQ (store.go:175-189), retention pruning (store.go:191-199). This is
   structurally River-quality; it just needs producers and consumers attached (AF-1).
3. **Desired/actual state split with persisted diffs and destructive-action confirmation.** Servers and
   nodes both carry desired vs actual (reconciler/service.go:486-535), plans persist diff/drift JSON
   with hash dedupe and TTL (service.go:226-230,680-702), destructive plans require explicit confirm
   (trigger.go:211-213), and recovery keeps "needsConfirm" visible even after execution begins
   (recovery_ops.go:94-97). That last touch exceeds anything in the six references.
4. **Hysteresis heartbeat classifier with history.** Warning/unreachable/offline/recovering/
   reconciling transitions driven by consecutive-success thresholds over a persisted history table,
   future-timestamp guard, monotonic threshold normalization (heartbeatmonitor/service.go:266-313,
   399-423) — richer than Rancher's boolean condition ([S4] C11), comparable to Incus's
   offlineThreshold machinery, and the right place to hang Rancher-style active echo.
5. **Generation+lease as a concept.** The token idea is exactly right for a control plane that must
   invalidate in-flight authority (the lesson Nomad encodes in alloc modify-index chains and NetBird in
   NetworkMap epochs); it is the wiring that's wrong (AF-3), not the design.
6. **Executor reuse over reimplementation in recovery.** The coordinator deliberately injects the
   migration service rather than duplicating transfer logic (recovery/service.go:100-110 comment),
   and backup-restore recovery refuses to contact the dead source (:44-48 interface contract). Compare
   Forge's runtime layer, where three abstractions drifted apart ([S3] LF-7) — recovery shows the
   codebase can do dependency discipline when the seam is drawn once.
7. **Steal ≠ retry accounting in the queue** ([S2] row 4, regression-tested) — better than River's
   uniform attempt accounting for distinguishing infra failure from job failure. Keep this contract
   verbatim in whichever engine survives consolidation.
8. **Correlation-ID plumbing end-to-end** (events/context.go:12-27; RunOnce injects per-cycle
   correlation IDs, reconciler/service.go:218-219; recovery propagates into payloads,
   recovery/service.go:711-724). None of the references do better; most do worse.

## 4. Traps to dismantle (consolidated from all five reports)

| Trap | Evidence | Disposition |
|---|---|---|
| Duplicated execution engines | queue vs operation dual-writing `operations` ([S2] LF-5/LF-6); vendored River fork + dead `river_queue` table ([S2] §0); three runtime abstractions ([S3] LF-7) | Pick one writer per concern ([S2]'s target table); delete the fork; fold `services/runtime` into `internal/runtime` |
| Provider-specific abstractions ahead of implementations | 7 provider names vs 5 implemented vs 1 honest ([S3] LF-1/LF-2); capability registry dead ([S3] LF-3); nodeautoscale hardcodes `"aws"` ([S1] F10) | Registry honesty rule: a provider may be listed only if its factory + capability report + beacon routing all agree; schedule-time `CheckCapability` gate |
| Tenant-blind core path | AF-7 | Add tenant columns now while tables are small |
| Config sprawl + unsafe defaults | AF-8; four config regimes (C12) | One typed config surface per service, DB-backed where operators must change it at runtime; Notify-by-default for unpolicied failure |
| Scattered retry policy | C5; [S1] F6 placement retries | Move attempts/backoff/penalty onto the records themselves (instances, events, plans) — the Nomad ReschedulePolicy lesson |
| Write-only durability | AF-1 | Wire relay consumers or drop the table; do not keep paying for an unused guarantee |

## 5. Priority recommendations (control plane)

1. **P0 — Make the event loop real (AF-1):** add tx-bound publish; register fencing/failover/tm/lb on
   the relay; stop marking events dispatched when zero handlers ran.
2. **P0 — Elect one leader (AF-2):** advisory-lock or TTL-row election gating the maintenance-class
   daemons (reconciler, periodic scheduler, evacuator sweeps, retention prunes, failover loop). The
   unwired elector at queue/leader.go:23-133 is the starting point.
3. **P0 — Default Notify (AF-8):** remove the failover Evacuate fallback before the locality default
   ([S4] F5) ever meets a real outage.
4. **P1 — Enforce the fence (AF-3/AF-4):** single fencing implementation; CAS generation checks on
   power/dispatch; serialize controllers per server via generation preconditions.
5. **P1 — Tenant-tag the core records (AF-7):** PlacementRequest/Decision, plans, Envelope.
6. **P2 — Idempotent recovery intents (AF-6)** keyed like incidents; return dedupe signals.
7. **P2 — Demote the registry** to non-blocking best-effort fan-out once durable delivery exists (AF-5).
8. **Ongoing — keep** items §3.1-§3.8; they are the parts of Forge a rewrite should preserve.

## 6. Coverage checklist

- [x] eventstore/* (schema, claim/lease, DLQ, relay, outbox publisher) — read in full
- [x] events/* (envelope, registry, subscriber, context) — read in full
- [x] services/reconciler (service, diff, trigger, plan lifecycle) — read
- [x] services/recovery (coordinator, ops, ack timeout; tokens.go noted as account-recovery, out of scope)
- [x] services/heartbeatmonitor, services/failover, services/fencing — read in full
- [x] cmd/api/main.go daemon inventory + subscription graph + shutdown — grepped/read
- [x] beacon-side enforcement of generation/fencing — verified absent (grep)
- [x] nomad leader.go leadership-gated subsystems — spot-checked (leader.go:268,413-430,1445-1454)
- [x] sibling reports S1-S4 incorporated by citation; no product code modified
