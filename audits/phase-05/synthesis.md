# Phase 05 Synthesis — Orchestration & Operations Cluster (Nomad · River · Incus · NetBird · Longhorn · Rancher)

**Cluster:** nomad, river, incus, netbird, longhorn, rancher
**Why this cluster last:** Deepest architecture phase. Phases 1–4 established Forge's app/game/backup/networking surfaces and surfaced the single-writer/desired-state lessons; this phase tests the control-plane core itself — scheduling, durable jobs, runtime honesty, storage/fleet concepts — against systems built for exactly these problems at scale.

**Evidence:** 5 subagent reports — Nomad placement (16 comparisons, 12 findings), River queue (16 comparisons, 8 findings), Incus+NetBird runtime/clustering (17 comparisons, 7 findings), Longhorn+Rancher storage/fleet (15 comparisons, 6 findings), architecture synthesis capstone (14 comparisons, 8 findings) — all file:line verified.

---

## 1. The three structural facts (capstone verdict)

Forge has quietly assembled the **right skeleton**: durable desired/actual state split, append-only decision/event ledger in Postgres, lease-based outbox with DLQ, hysteresis heartbeat classifier, generation+lease fencing tokens. Several pieces are *better* than reference analogues. But three facts undercut it:

1. **AF-1 — The durable event pipeline is write-only.** `Relay.Subscribe` has zero production callers (`eventstore/outbox.go:39`, test-only); `EventRelay` unused on `http/server.go:148`; no tx-bound publish API exists. All automation rides best-effort in-memory delivery; cross-instance automation is impossible today.
2. **AF-2 — No leader election anywhere.** ~30 background daemons start per instance with startup jitter as the only "coordination" (`main.go` ~20 Start() sites). The only elector code lives inside the **unwired River fork** (`queue/leader.go:23-133`). References solve it: Nomad leader-gated subsystems (`leader.go:413-430`), River TTL rows (`elector.go:26-31`).
3. **AF-3 — Fencing writes tokens nothing enforces.** Generation bumps written twice by two implementations (fencing + recovery, different leases 24h vs 1h), fire on `EventNodeRecovered` (the wrong edge — the partition window is unfenced), enforced by zero action paths; beacon has zero occurrences of "generation"/"fence". Contrast NetBird where policy removal revokes mesh access via next NetworkMap push — enforcement *is* the mechanism.

## 2. Key findings per dimension

### Scheduling (Nomad vs placement/)
- **S-01 Soft-constraint bonuses dwarf base scores.** ±1e10/1e12 bonuses (`placement/constraints.go:59-63`) vs base scores ≤3 — outcomes dominated by constraint count, not fit. Nomad composes bounded iterators then normalizes (`funcs.go:257-277`, `rank.go:967-991`).
- **S-02 No reschedule policy.** Failed instances retried every 60s forever, no backoff, no attempt cap, no failing-node penalty. Nomad `ReschedulePolicy` delay functions + `NodeReschedulingPenaltyIterator`.
- **S-03 Spread is anti-co-location only** — no attribute-target spread across zones (Nomad `Spread{Attribute,SpreadTarget[]}`).
- **S-04 No blocked-eval equivalent** — freed capacity picked up only on next poll tick.
- **S-05 Sticky semantics wrong:** `requiredNode` hard-fails instead of try-preferred-then-fallback (Nomad `findPreferredNode` degrades gracefully, `generic_sched.go:872-903`).
- **S-06 nodeautoscale scale-in hardcodes AWS provider and deprovisions before drain completes** (`F10`).

### Queue (River vs queue+operation duality)
- **Verdict: Forge does NOT need River.** `cmd/api/river.go` is a stub; vendored fork dead; `river_queue` table orphaned since migration 137. Live engines are `job_queue` + `operations` dual-writing the same projection tables.
- **Q-01 Cancellation unsound:** `Fail()` has no status predicate → cancel regresses completed jobs, loses races both directions (`store.go:113-130`). River uses FOR UPDATE + guarded CAS completion.
- **Q-02 Operation backoff is fake:** worker sleeps inside slot up to 30s but `retrying` sorts FIRST in Dequeue → ~1 poll-interval actual delay, parked goroutine cost; no `available_at` column on operations.
- **Q-03 Per-replica periodic scheduler duplicates work cross-instance** — nano-timestamp idempotency keys derived from each replica's own start time produce different keys for same logical run.
- **Q-04 Non-tx multi-statement transitions tear job_queue/operations/attempts apart** (Dequeue/Ack/Fail/Retry each run 4 independent statements).
- **Q-05 Idempotency namespace fragmentation:** `"forge-op:"+kind+":"+key` vs `"forge-op:"+key` vs `"forge-job:"+key` — same client key hashes differently per engine path; silent dedupe (no `UniqueSkippedAsDuplicate` signal).
- **Q-06 Heartbeat dies with job context on Stop()** — leases lapse while handlers drain (contrast operation.Service's correct `context.WithoutCancel` touchLoop).

### Runtime (Incus vs adapters)
- **R-01 LXC/KVM are phantom providers.** Control plane registers 7 names; beacon factory has no lxc/kvm branch; beacon create handler has NO `provider` field in body struct — silently dropped; always executes Docker and replies `"mode": "docker"` hardcoded (`server.go:740-772,841`). A silent lie across a trust boundary.
- **R-02 Firecracker unsafe-by-construction:** every VM boots the SAME RW rootfs image; no network-interface setup ever issued; install exit code hardcoded 0; stats decode fields FC never emits.
- **R-03 Capability system dead code overstating reality:** `Snapshots:true` claimed with no snapshot op in any runtime interface; `CheckCapability` zero callers; scheduler never gates on capability.
- **R-04 No overlay option whatsoever:** Node schema lacks tunnel/private IP; gateway resolves public hostnames only; sole "WireGuard" mention is a test string. Cross-node private traffic has no path and no plan.
- **R-05 servicediscovery write-only:** admin-API registration only, beacons never self-register; LastHeartbeat stamped once, never refreshed → reaper marks EVERYTHING unhealthy 3 minutes after registration.
- **R-06 Profile stacking missing:** eggs = single object with one optional parent pointer; Incus composes N ordered profiles with deterministic last-wins merge — ~40 lines of Go to adopt conceptually.

### Storage/Fleet (Longhorn/Rancher vs mounts/volumes)
- **ST-01 StorageLocality scoring is dead code** — no production caller sets it; vocabularies disagree (`"local"` vs `"local_only"/"replicated"`) so could never match even if wired.
- **ST-02 volumes table orphaned FK stub** with zero inserts; real volume IDs are free-text docker names.
- **ST-03 Mount eligibility enforced AFTER placement** (`ensureMountAvailableForServer`) instead of schedule-time like Longhorn diskSelector/nodeSelector.
- **ST-04 Evacuator defaults servers without extra mounts to "replicated" → AutoReplace**, overstating durability; combined with AF-8's default-Evacuate failover fallback, a dead node of ordinary local-data game servers auto-evacuates.

### Failover/config
- **AF-8 Dangerous default:** when no policy matches, classifier override or synthetic policy defaults to **Evacuate** (`failover/service.go:181-211`). Every reference errs the other way (Longhorn `do-nothing`, Rancher opt-in policies). Absence-of-policy must mean Notify.

## 3. What Forge gets RIGHT (keep & strengthen)

1. **Postgres-as-single-writer** instead of bespoke consensus — reservation row locks, advisory-lock incident CAS, SKIP LOCKED claims. Correct scaling-down of Nomad's plan-applier for panel scale.
2. **Lease-based outbox primitive itself** — claim tokens, expiry reclaim, bounded failures, atomic DLQ move, retention pruning. Structurally River-quality; needs producers/consumers attached.
3. **Desired/actual split with persisted diffs + destructive-confirm gates** — stricter than any reference; recovery keeps needsConfirm visible even mid-execution.
4. **Hysteresis heartbeat classifier with history** — richer than all six references on classification.
5. **Generation+lease as a concept** — right idea, wrong wiring.
6. **Executor reuse over reimplementation** in recovery (injects migration service rather than duplicating transfer logic).
7. **Steal≠retry accounting in queue** — regression-tested, better than River's uniform attempts. Keep verbatim.
8. **Correlation-ID plumbing end-to-end** — none of the references do better.

## 4. Traps to dismantle

| Trap | Disposition |
|---|---|
| Duplicated execution engines (queue+operation, dead River fork, river_queue table) | queue.Service sole execution writer; operation.Service read-model; delete fork |
| Phantom providers (lxc/kvm/firecracker claims vs reality) | Registry honesty rule: listed only if factory+capability+routing agree; beacon rejects unknown provider values |
| Write-only durability (events/outbox) | Add PublishTx; register consumers on relay; stop marking dispatched with zero handlers |
| Tenant-blind core paths (placement/events/plans carry no tenant) | Add tenant columns NOW while tables small |
| Config sprawl + unsafe defaults (4 config regimes; Evacuate fallback) | Notify-by-default; typed config surface per service |
| Scattered retry policy | Move attempts/backoff/penalty onto records (Nomad ReschedulePolicy lesson) |

## 5. Recommended activation order (control plane)

1. **P0 Make the event loop real (AF-1):** PublishTx + relay consumers for fencing/failover/tm/lb; don't mark dispatched when zero handlers ran.
2. **P0 Elect one leader (AF-2):** advisory-lock/TTL-row election gating maintenance daemons; resurrect unwired elector.
3. **P0 Default Notify (AF-8):** remove failover Evacuate fallback before locality-default ever meets a real outage.
4. **P0 Stop lying about providers (R-01/02):** remove lxc/kvm until implemented or make beacon reject unknown provider; quarantine Firecracker behind build tag.
5. **P1 Enforce the fence (AF-3/4):** single fencing impl; CAS generation checks on power/dispatch; per-server controller serialization via generation preconditions.
6. **P1 Queue consolidation fixes (Q-01..06)** regardless of engine choice: state-guarded cancel, retryable delay column, tx-wrapped transitions, unified idempotency namespace, background-context heartbeat, drop goroutine fallback.
7. **P1 Mesh decision (R-04):** tunnel IP + mesh pubkey on Node schema minimum; prefer in resolveTargetHost.
8. **P1 Self-registering servicediscovery (R-05):** beacons carry endpoint liveness in heartbeats.
9. **P2 Tenant-tag core records (AF-7).**
10. **P2 Schedule-time mount eligibility (ST-03) + honest locality vocabulary (ST-01).**

## 6. Handoff to Final Report

All five phases complete. Phase 5 confirms the audit-wide thesis: Forge's architecture skeleton matches or exceeds references where it matters (state model, outbox primitive, heartbeat classification, correlation IDs), while integration debt concentrates in wiring (events unreadable, leaders unelected, fences unenforced, certs undelivered, progress unproxied, queues doubled, providers phantom). The final report aggregates all phases into capability parity, activation roadmap, and the missing-vs-disconnected verdict.
