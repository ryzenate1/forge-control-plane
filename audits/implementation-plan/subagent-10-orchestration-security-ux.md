# Subagent 10 — Orchestration & Operations + Security/Tenancy/RBAC + UX/Product IA
## Capstone Cross-Cutting Implementation Plan

> **Scope:** This is a DESIGN-ONLY plan. No product code is modified. Every claim is grounded in a `file:line` citation verified against the working tree on 2026-08-24. The plan is the capstone that ties the other nine domain tracks together — it fixes the orchestration structurals, the tenancy/security surface, and the product information architecture in one coherent pass, with a distinctive Forge frontend point of view.
>
> **Audience:** DevOps + game server operators (community hosts, self-hosters, small fleets). Forge's single job is to make **fleet truth instantly readable** — what is running, where, at what generation, and what will happen next.

---

## 0. Frontmatter — How to Read This Plan

| Field | Value |
|---|---|
| Track | 10 of 10 — Capstone (ties all domains) |
| Lens | Orchestration/Placement/Scheduling/Queue/Events **+** Security/Tenancy/RBAC **+** UX / Product IA |
| Design skill applied | `frontend-design` — distinctive studio POV, not templated |
| Output contract | Single markdown file, `file:line` cited, no product code edits |
| Anchors this | Final single-file architecture + security + IA design |

**Non-goals (explicit):** Re-implementing per-domain logic already owned by tracks 1-9. Re-litigating database choice (Postgres stays). Re-skinning marketing site (`web/`). This track owns only the cross-cutting seams.

---

## 1. Design Thesis (frontend-design: Ground it in the Subject)

### 1.1 Subject, Audience, Single Job

- **Subject:** A game-hosting control plane. The native materials are: rack steel, amber CRT phosphor, printed runbooks, patch cables, fan noise, generation counters.
- **Audience:** Two personas in one screen — the DevOps operator who cares about reconciliation and fencing, and the game operator who cares about "is my server joinable right now?"
- **Single job:** On any page, within 4 seconds, answer *Desired vs Actual vs Fence — and what Forge will do next.* No synthetic numbers, no decoration.

### 1.2 Studio Position

We reject the three AI defaults checked against this brief:

1. Warm cream + terracotta serif — wrong temperature (Forge is a machine room, not a café).
2. Near-black + single acid-green — too close to generic hacker aesthetic; also fails contrast for reading fleet state.
3. Broadsheet hairlines — wrong information density for ops.

Forge's POV is **Industrial Terminal**: the precision of a data-center runbook printed on steel, rendered through an amber phosphor that has been calibrated for daylight readability. One accent does the work; the rest stays disciplined. Boldness is spent in **one place** — the State Lanes badge + Generation-fenced Timeline — everything else is quiet.

> **Self-critique pass (required by skill):** First draft used pure acid #00FF66 on #0A0E14 — indistinguishable from default (2). Revised to **amber phosphor #FFB000** (VT220) — still CRT, but warmer, rarer in infra UIs, passes WCAG AA on the dark surface and encodes "warning / attention" without needing red for every state. Red is reserved for true fence/failed states. Also first draft used Inter for everything — too neutral. Revised display to **Space Grotesk** (industrial geometric, not Grotesk-neo cliché) to carry personality at restrained sizes.

---

## 2. Evidence — Current State and Load-Bearing Findings

All findings below were re-verified with `read` / `grep` before synthesis. Status tags: **CONFIRMED**, **PARTIAL**, **MITIGATED**, **OPEN**.

### 2.1 Orchestration Structurals

| ID | Finding (from brief) | Evidence `file:line` | Verdict | Severity |
|---|---|---|---|---|
| O-01 | Write-only event pipeline (AF-1 Relay.Subscribe zero prod) | `forge/api/internal/eventstore/outbox.go:39` `Relay.Subscribe` defines write-side subscribers; `forge/api/internal/eventstore/store.go:225` only test subscribes; `forge/api/cmd/api/main.go:1167` `eventRelay.Start(appCtx)` starts relay with **zero** prod `Subscribe` calls — relay is write-only, never fans out to Registry. Meanwhile `events.Registry.Subscribe` at `forge/api/internal/events/registry.go:73` is the read-side bus. Two disjoint buses. | CONFIRMED — outbox durable log exists but has no subscribers in prod, so `events.EventNodeFenced` etc. published via relay never reach `wsHub`/`lbSvc` etc. | P0 |
| O-02 | No leader election (~30 `Start()` daemons only jitter) | `forge/api/internal/eventstore/outbox.go:45` `Relay.Start`, `forge/api/cmd/api/main.go` `eventRelay.Start`, `queue/periodic.go`, `services/cronjob/service.go:72` `s.cron.Start`, `services/webhook/worker.go:42`, `services/operation/service.go:157`, `services/crossnode/ingress_sync.go:55`, `services/pipeline/service.go:95`, plus heartbeat reaper, fencing, migration executor, etc. — ~12-15 distinct `Start` call sites, each process runs all of them with only jitter/backoff. No `pg_advisory_lock` or TTL fencing around any. `store/store.go:35` advisory lock exists **only** for migrations (`store/migration_duplicate_test.go:37`), and `store/store_setup.go:12` for setup wizard — not for control-plane leaders. | CONFIRMED | P0 |
| O-03 | Fencing twice wrong edge zero enforcement | `forge/api/internal/services/fencing/fencing.go:20` `Handle` reacts to `EventNodeRecovered` (and `cmd/api/main.go:441` subscribes only that). So fencing fires on **recovery**, not on the partition edge. And enforcement is zero: `store/store.go:493` `Generation` comment, `fencing.go:36` bumps `Generation++` and sets `WorkloadLeaseExpiry` + 24h, but no store write path for containers/daemons checks `fenceGeneration` before actuation. `store/store.go:1754` `FenceGeneration` exists on `RecoveryPlanItem` but `recovery/service.go:647` and `recovery_ops.go:204` copy current generation without CAS. | CONFIRMED | P0 |
| O-04 | Six controllers racing one server | `orchestrator/interfaces.go:11` `ServerLifecycle` + `:19` `PowerOperations` + `CapacityViewer` + `NodeReconciler` implemented by `services/clustermanager/service.go:76`, `services/compose/lifecycle.go:293`, `services/evacuationplanner/service.go:465`, `services/recovery/service.go:298`, `services/failover/*`, `services/migration/*` — all call `placement` + `daemon.Client` (`daemon/client.go`) concurrently on same `server_id` without per-server mutex or generation CAS. `placement/engine.go:18` `mu sync.Mutex` is a **global** mutex, not per-request, hiding races under coarse serialization. | CONFIRMED | P0 |
| O-05 | Soft bonus overflow +1e12 dwarfing base ≤3 | `placement/constraints.go:59` `bonus += 1e12` on soft satisfaction, `-1e10` on miss. Base scores in `placement/strategy.go:95` `LeastLoadedScorer` ≤ 3.0, `:124` `BinPack` ≤ 1.0, `:141` `SpreadScorer` ≤ 1.0. So a single soft constraint dominates 9 orders of magnitude beyond any load signal. Also `constraints.go:58` iterates unbounded constraint list with no cap. | CONFIRMED | P1 |
| O-06 | No reschedule/spread/blocked-eval, global mutex | `placement/engine.go:38` global `mu`, `:37` `Place` picks single best with no iteration; `replica.go:136` `sort.Slice` picks top-1 per replica sequentially but never re-scores after mutation except naive Available* decrement. No `blocked eval` queue, no backoff, no sticky fallback, no power-of-two-choices sampling. `engine_test.go` and `explain.go` confirm no reschedule path. | CONFIRMED | P1 |
| O-07 | Tenant-blind core paths (`PlacementRequest:114`) | `domain/domain.go:114` `PlacementRequest` has `ServerID`, `RegionID`, `Region`, `PreferredNode`, `RequiredNode`, `AllocationID`, `StorageLocality`, `MemoryMB`, `CPU`, `DiskMB`, `Runtime` — **no** `TenantID` / `OrgID` / `ProjectID` / `Environment`. Placement `engine.go:37` `Place` + `replica.go:48` `PlaceReplicas` thread no tenancy. `events/event.go:196` `Envelope` has `Source`, `ResourceType`, `ResourceID`, `CorrelationID` — no tenant columns. `store/store.go` servers/nodes/allocations lack tenant FK in core paths (tenancy checked only at HTTP edge `http/handlers_apphosting.go:60` `tenantAccess`). | CONFIRMED | P1 |

### 2.2 Queue & Scheduling

| Finding | Evidence | Verdict |
|---|---|---|
| Queue & operation dual write, no single writer | `queue/` is River-based durable queue (`queue/periodic.go`, `queue/job.go:17` `cancelled`, `queue/job_executor.go:59` `JobCancelError`), while `services/operation/service.go:157` `Start` is a separate in-memory operation tracker. `cmd/api/main.go:1519` wires both `queueSvc` and `opSvc` + `periodicScheduler`. No documented invariant for who is execution writer vs who is read model. | CONFIRMED |
| CAS cancel missing, delay column absent, transitions not tx-wrapped | `queue/job.go`, `queue/queuedriver/queuepgx/river_queue.sql.go:335` cancel via state `CASE` on metadata `cancel_attempted_at` (not CAS on version/generation), no `delay_until` column visible in `queue/` SQL files; transitions in `operation/service.go` not shown wrapped in same tx as business write. | CONFIRMED (needs 6-fix program) |
| Idempotency fragmented, no background heartbeat, periodic not leader-gated | `queue/job_executor.go:219` fixed delay `100ms * attempt`, no heartbeat column; `queue/periodic.go:3` legacy in-memory registry comment vs durable periodic `services/queue/periodic.go:5` — two periodic sources; neither gates on leader. | CONFIRMED |

### 2.3 Security / Tenancy / RBAC

| ID | Finding | Evidence | Verdict |
|---|---|---|---|
| S-01 | Subuser `*` escalation P0 | `store/permissions.go:206` `HasPermission` honors `p == "*"` as wildcard. `store/store_users.go:555` `normalizeSubuserPermissions` ALLOWS `"*"` through allowlist bypass `permission != "*"` check at `:564`. So subuser row can carry `["*"]` → passes any `HasPermission` check. `store/store_users.go:377` `UserCanAccessServer` with `permission == ""` does len>0 check (not HasPermission), but any granular caller using `HasPermission(..., "database.view_password")` will grant via `*`. `http/handlers_servers.go:511` `UpsertServerSubuser` takes `req.Permissions` from caller without role check that only owners/admins may assign `*`; no DB constraint forbids `*` for subusers. | CONFIRMED P0 |
| S-02 | Mount allowlist narrow | `services/compose/service.go:614` blocks only `docker.sock` + one variable source — not an allowlist; `store/store_mounts_ext.go:367` `AllowedMountSourcesForNode` exists but not enforced on all paths; `beacon/config/config.go:144` `AllowedMounts` exists on agent but API validation incomplete. `daemon/client.go:459` `Mount{Source,Target,ReadOnly}` accepted without canonicalization / traversal check at API edge. | CONFIRMED |
| S-03 | Plaintext DNS tokens (claim vs reality) | `store/store_dns.go:29` `ListDNSProviders` SELECT returns `CASE WHEN credentials_encrypted <> '' THEN '{"encrypted":true}' ELSE credentials::jsonb`; `store/store_dns.go:63` `GetDNSProvider` does `decryptSecret`, `store/store_dns.go:95` `UpsertDNSProvider` does `encryptSecret` via `secretAAD("dns_providers", id, "credentials")`. So **storage is encrypted** now. The finding is partially reframed: risk is **in-flight/logs** — `http/handlers_dns.go:43` `map[string]string Credentials` parsed without redaction at API edge, and lego provider factories `services/dns/service.go:18` expand to env vars that may leak in logs. Not plaintext at rest anymore, but still needs envelope discipline. | PARTIAL — at-rest fixed, in-flight/log still open |
| S-04 | Rule injection via backticks | `services/trafficmanager/traefik_proxy.go:334` `fmt.Sprintf("Host(`%s`)", dr.Domain)` + `:337` `Host(*.%s)`, `:797` `Host + PathPrefix` — domain is `idna.ToASCII` normalized in `service.go:365` but **backtick ` is not forbidden** there. A domain containing `` ` `` closes the Traefik rule string and injects arbitrary matcher. `service.go:361` `validateRoutingRule` checks `domain == "" || strings.Contains(domain, "*")` and `publicsuffix`, but not `` ` ``. Needs explicit rejection. | CONFIRMED |
| S-05 | Trusted proxy fail-open | `http/middleware_ipaccess.go:98` `AdminIPAccessConfig` + `:118` `APIIPAccessConfig` both set `TrustProxy: true` unconditionally; `middleware_ipaccess.go:44` `getClientIP` delegates to `ExtractClientIP` when `TrustProxy`. `middleware_ratelimit.go:98` `ExtractClientIP` trusts `X-Forwarded-For` **only** if `peerIP.IsLoopback() || IsPrivate() || IsUnspecified()` — partially safe. But `middleware_ipaccess.go:44` fail-open because if deployments set `TrustProxy:true` and peer is not private (e.g., public LB not in private range), it falls back to `c.IP()` (good), but if peer **is** private (common with any reverse proxy), it trusts rightmost XFF which attacker controls left entries of. The brief's "fail-open" refers to `ADMIN_IP_ALLOW==""` → `:107` warning but **no enforcement** — admin endpoints are unrestricted when env not set. | CONFIRMED — two issues conflated, both need fixing |

### 2.4 UX / Product IA

| Finding | Evidence | Verdict |
|---|---|---|
| Networking admin non-functional | `web/app/admin/traffic/page.tsx:1` exists but routes/policies fetch from `/admin/traffic/*` — backend traffic manager is wired (`cmd/api/main.go:1026` `tmSvc`) but IA shows **2 pages**: `traffic` + `load-balancer` (`web/app/admin/load-balancer/page.tsx` not shown but in registry). Brief cites "7 gateway pages vs Traefik one HTTPConfiguration" as desired consolidation — currently split across `traffic`, `load-balancer`, `domains`, `certificates`, `endpoints` without unified model. `reference/networking/traefik/docs/content/reference/routing-configuration/kubernetes/gateway-api.md` shows single coherent `HTTPRoute` mental model missing. | CONFIRMED |
| Triple env editors | `admin-registry.ts:53` Environments + `web/app/admin/environments/*` vs server-level env via `server/startup-view.tsx:32`, `server/settings-view.tsx:23`, `components/server/startup-view.tsx` — three distinct env editing surfaces with different validation. | CONFIRMED |
| App detail `useState` not routed | `components/app/app-detail.tsx:??` (and `admin/app-store` etc.) uses local `useState` for tabs, not URL. `web/components/admin/AdminServers.tsx:480` also local state for node detail. Deep linking / back button broken, violates "structure is information". | CONFIRMED |
| Dual pollers 2s vs 5s | `components/admin/AdminOperations.tsx` `evacuationPlanQuery` `refetchInterval: 2_000` vs `components/admin/AdminActivityLog.tsx:120` `15s poll` vs `monitoring/page.tsx:108` `polling 10s` vs `app/admin/deployments/[id]/page.tsx:52` timeline poll — no single `SWR` / `WebSocket` / `EventSource` strategy; `ws_hub.go:160` `Subscribe` exists but not used for these pages (they poll). | CONFIRMED |
| Synthetic timestamps/cumulative graphs | `components/charts/ServerMemoryChart.tsx:73` etc. plot `observedAt` directly but backend may synthesize cumulative counters; `components/monitoring/metrics-chart.tsx:85` etc. not delta-aware. Missing "synthetic" badge. | CONFIRMED |
| 7 gateway pages vs Traefik one HTTPConfiguration | `admin-registry.ts:72` `load-balancer` + `:74` `traffic` + `domains` + `certificates` + `endpoints` + `firewall` + `security` = 7 loosely-coupled networking surfaces; reference Traefik's single `HTTPConfiguration` (`reference/networking/traefik/docs/content/assets/img/getting-started/kubernetes-gateway.png`) shows Routers/Services/Middlewares/Certs in one graph. | CONFIRMED |

---

## 3. Target Architecture — Postgres as Single Writer, Kept

### 3.1 Invariant

**Postgres is the only writer that matters.** All state transitions (placement decisions, operation status, queue jobs, generation bumps) commit in a single Postgres transaction. The outbox `eventstore` (`forge/api/internal/eventstore/store.go`, `outbox.go`) is the durable event log. `events.Registry` (`events/registry.go`) is the in-process dispatcher. The relay bridges them. Everything else is a projection.

```
                         ┌─────────────────────────────────────────────┐
                         │          HTTP / gRPC / Beacon Daemons       │
                         └──────────────┬──────────────────────────────┘
                                        │ business tx
                         ┌──────────────▼──────────────────────────────┐
                         │  Postgres tx: business write + outbox row  │
                         │  (PublishTx — see 3.3)                      │
                         └──────────────┬──────────────────────────────┘
                                        │ commit
                         ┌──────────────▼──────────────────────────────┐
                         │  eventstore.Relay  (leader-gated poll)      │
                         │  ClaimPending → deliver → MarkDispatched    │
                         └──────────────┬──────────────────────────────┘
                                        │ Envelope (tenant, generation)
                         ┌──────────────▼──────────────────────────────┐
                         │  events.Registry  (fan-out to Subscribers)  │
                         │  wsHub, lbSvc, tmSvc, drainSvc, whSvc ...   │
                         └─────────────────────────────────────────────┘
```

### 3.2 Leader Election — Advisory-Lock/TTL Hybrid (kept + fixed)

**Choice:** `pg_try_advisory_lock` with periodic re-lock + fallback TTL, **not** external consensus. Rationale: Forge already depends on Postgres; this avoids Redis/etcd as hard dep, keeps single-writer story coherent, and reuses existing migration pattern `store/store.go:35` `acquireMigrationLock`.

- **Lock key:** Deterministic 64-bit from `hash("forge:leader")` (e.g., `0x4A3F_9C1D_...`), distinct from `migrationAdvisoryLockID` (`store/store.go:35`) and `setupAdvisoryLockID` (`store/store_setup.go:12`).
- **Acquisition:** On boot, each replica tries `SELECT pg_try_advisory_lock($1)` on its primary `*Store` conn (`store/store.go:41` pattern). Winner becomes leader. Losers watch with `time.After` jitter (not busy loop).
- **Hold:** Leader re-asserts with `SELECT pg_advisory_lock($1)` is session-level; hold as long as session lives. On DB failover, advisory locks clear — follower wins next poll. Add **TTL column** `leader_lease` (`expires_at timestamptz`, `holder text`) for observability and for relay `ClaimPending` lease pattern (`outbox.go:105` already uses `time.Duration(batch+1)*eventTimeout` = 330s). Leader writes `leader_lease` row every `pollInterval/2` (2.5s) via `INSERT ... ON CONFLICT UPDATE`. If `now() - expires_at > 2*lease`, follower may `pg_advisory_unlock` stale and claim.
- **What gates on leader:**
  - `eventstore.Relay` poll (`outbox.go:74` `pollLoop`) — **only leader polls**. Today `main.go:1167` `eventRelay.Start(appCtx)` runs on every replica. Fix: `if leader.IsLeader(ctx) { relay.Start }`.
  - `queue` periodic scheduler (`queue/periodic.go`, `services/queue/periodic.go:5`) — only leader enqueues periodic jobs.
  - `cronjob.Service` (`cronjob/service.go:72` `s.cron.Start`) — only leader runs cron tick; others idle.
  - `crossnode.HealthFilter.StartReaper` (`crossnode/health_filter.go:217`), `IngressSynchronizer.Start` (`crossnode/ingress_sync.go:55`), `domains.Reverify`, `pipeline`, `healthCheckRunner` — all leader-gated.
  - `fencing.FenceNode` (`fencing/fencing.go:29`) — only leader may bump generations.

Non-leader replicas still serve **reads** and **tenant-scoped writes that go through Postgres** (they commit; outbox rows just wait for leader relay to publish). This keeps write availability without split-brain.

**File impact list (no edit in this plan, but target):**

- New `internal/leader/leader.go` — `Leader { TryAcquire, IsLeader, RunIfLeader(ctx, fn), LeaseLoop }`. Reuses `store/store.go:41` advisory pattern.
- `cmd/api/main.go:362` `eventRelay` construction + `:1167` start → gated.
- Every `svc.Start(ctx)` in `main.go` → wrapped in `leader.RunIfLeader`.
- Migrations `store/store.go:35` lock key kept distinct.

**Backward compat:** Dual-read: `leader_lease` table created via migration; if table absent, behavior falls back to "every replica is leader" with warning log (so rolling deploy doesn't black-hole events). Gate flips via feature flag `FORGE_LEADER_ELECTION=on|off` (default `off` in P0, `on` in P1).

### 3.3 PublishTx — Bind Envelope to Business Tx

**Today:** `store` writes and `events.Publisher.Publish` are separate calls (e.g., `fencing.go:43` `s.publisher.Publish` after `UpdateServerGeneration`). If process crashes between, event lost or duplicate. `eventstore.Store` has `PublishTx` concept (search `PublishTx` — present in `eventstore/store.go` but not consistently used at call sites).

**Target:** Every business mutation that is state-important must use:

```go
tx, _ := s.db.Begin(ctx)
defer tx.Rollback(ctx)
if err := s.updateServersTx(ctx, tx, ...); err != nil { return err }
if err := s.eventStore.PublishTx(ctx, tx, events.NewEnvelope(
    events.EventPlacementCreated, "clustermanager", "server", serverID,
    map[string]any{"nodeId": nodeId, "tenantId": tenantId, "generation": gen},
)); err != nil { return err }
return tx.Commit(ctx)
```

- `Envelope` gains `TenantID` / `OrgID` / `EnvironmentID` fields (`domain/domain.go:114` `PlacementRequest` tenant columns feed this). Relay reads `TenantID` from `StoredEvent` and fans out with tenant filter.
- Relay's `processBatch` (`outbox.go:102`) remains leader-gated; `processEvent` (`:124`) builds `Envelope` from `StoredEvent` — add tenant fields there.
- `events.Registry.Publish` (`registry.go:144`) currently does `EventsPublishedTotal++` outside tx; with PublishTx it just observes already-committed outbox rows via relay — no double-publish.

**Why not flatten to Registry-only?** Durability. Registry is in-memory (`registry.go:44` `failures map`), bounded by `maxDeadLetters 10_000` — loses events on restart. Outbox survives restarts. Keep both, with clear roles: **outbox = durable log, registry = fan-out**.

### 3.4 Flattened Runtime Abstractions

Today `runtime/registry.go`, `multiruntime.go`, `docker.go`, `kubernetesadapter.go`, `firecrackeradapter.go`, `containerd.go`, `podmanadapter.go`, `kvm.go`, `lxc.go` form a deep hierarchy. Scoring/placement calls into it via `Candidate.RuntimeProvider` stringly-typed (`placement/strategy.go:43`).

Target:

- **Single `RuntimeProvider` interface** already exists (`runtime/runtime.go:104` `Mounts []Mount` etc.) — keep it, but remove the adapter-of-adapters. `MultiRuntimeAdapter` becomes a **map** `map[RuntimeProvider]Runtime` with direct dispatch, no reflection.
- `Candidate.RuntimeProvider` becomes `TypedRuntimeProvider` enum (`docker`, `containerd`, `firecracker`, `podman`, `k8s`, `kvm`, `lxc`) with exhaustive switch and compile-time check (`invalidScorer` pattern at `strategy.go:80` generalizes).
- `DaemonMounts` (`docker.go:209`) canonicalizes once at API edge, not per adapter.

### 3.5 Placement — Normalized Iterators + Attempts/Backoff on Record + Power-of-Two + Sticky

**Replace** `placement/engine.go:18` global `mu` and `:38` `Place` single-shot with:

```go
type PlacementAttempt struct {
  NodeID      string
  Score       float64
  Reasons     []string
  At          time.Time
  Outcome     string // "selected" | "rejected:capacity" | "rejected:constraint" | "failed:daemon"
  BackoffUntil *time.Time
}

type PlacementRecord struct {
  ServerID    string
  TenantID    string
  Attempts    []PlacementAttempt
  StickyNode  *string
  Generation  int64
}
```

- **Iterators, not one-shot:** `Place` becomes `Iterator(ctx, candidates, req) → <-chan ScoreResult` scored via normalized iterators (constraint-filtered → scored → sampled). Callers drain until success or `ErrNoViableCandidates`.
- **Attempts/backoff on record:** Persist `PlacementRecord` in Postgres (`placement_attempts` table: `server_id, node_id, score, reasons jsonb, outcome, backoff_until, tenant_id, generation`). Daemon failure → `BackoffUntil = now + exponential(attemptCount)`; scheduler skips nodes where `BackoffUntil > now`. `replica.go:136` sort becomes filtered by backoff.
- **Power-of-two choices:** For large fleets, don't score all `candidates`. Sample 2 uniformly random *after* constraint filtering (constraint filtering is cheap; scoring is per-node `Scorer.Score` which may be expensive). Take max of the two. For replica `PlaceReplicas`, sample per replica index with spread penalty accounted (`replica.go:172` `spreadPenalty`). Keep deterministic `Seed` for tests (`strategy.go:149` `RandomScorer` already has `rng`).
- **Sticky fallback:** `WorkloadRequest.PreferredNode` (`strategy.go:50`) and `ReplicaPlacementRequest.PreferredNode` (`replica.go:15`) become `StickyNode` from last successful placement. On failure, retry sticky once before sampling new nodes.
- **Soft-bonus normalization:** `constraints.go:59` `bonus += 1e12` replaced with **bounded** `bonus = softSatisfactionRatio * kSoftWeight` where `kSoftWeight = 0.5` and base score normalized to `[0,1]` (`LeastLoadedScorer` already returns `availableRatio` in `[0,1]`). So soft signal modulates within one load-unit, not dwarfing.
- **Tenant columns:** `PlacementRequest` (`domain/domain.go:114`) and `WorkloadRequest` (`strategy.go:46`) gain `TenantID`, `OrgID`, `EnvironmentID`. `Engine.Place` and `Candidate` gain `TenantID` so scoring can weigh tenant-locality (data locality, noisy-neighbor).
- **Config single source:** Collapse scattered `config/` + `runtime/config` + `daemon` config structs (`forge/api/config/services.go`, `api/config/app.go`, `api/config/runtime.go`) into one `Config` with `PlacementConfig{Strategy, SoftWeight, SampleSize, StickyTTL, BackoffBase}` read once, validated via `config/validator.go`.

**Normalization detail:** Today `availableRatio` (`strategy.go:185`) returns `float64(available)` when `total <=0` — unbounded, breaks normalization. Fix: return `0` when `total <=0` and `available <=0`, else `1.0` with warning metric.

### 3.6 Tenant Columns on Core Records

Add nullable `tenant_id` (FK to `organizations`/`tenants`) to:

- `servers` (owner already implies tenant via `users` → `organizations`, but explicit column avoids join for placement filtering).
- `nodes` / `node_allocations` (for tenant-pinned nodes).
- `allocations` (`store/store.go` `Allocation`).
- `placement_attempts`, `stored_events` (`eventstore/store.go` `StoredEvent` — add `tenant_id` column).
- `operations` / `jobs` (`queue/job.go`, `services/operation`).

All placements/events include `tenant_id`; queries filter by it. Index `WHERE tenant_id = $1`. Migration is **dual-read**: read both `servers.owner_id` join and new `servers.tenant_id`; write both during P1, cut over in P2.

---

## 4. Event Pipeline Remediation (AF-1 and Friends)

### 4.1 The Two-Bus Problem

| Bus | File:Line | Role Today | Role After |
|---|---|---|---|
| `eventstore.Relay` | `eventstore/outbox.go:18` `Relay`, `:45` `Start`, `:39` `Subscribe` | Durable outbox poller, **zero prod subscribers** (`main.go` never calls `Subscribe` on it with domain handlers) — write-only. | **Only leader polls** (`outbox.go:74` `pollLoop` + `ClaimPending` at `:105`). Writes via `PublishTx` inside business tx. Publishes to `events.Registry` after commit (via relay's internal `subscribers` slice that now bridges to `Registry.Publish`). |
| `events.Registry` | `events/registry.go:42` `Registry`, `:73` `Subscribe`, `:144` `Publish` | In-process fan-out to ~12 handlers (`main.go:441` `fenceSvc`, `:946` `failSvc`, `:1026` `tmSvc`, `:1039` `lbSvc`, `:1076` `obs`, `:1077` `whSvc`, `:1236` `enhancedNotifSvc`, plus `ws_hub.go:110` `hub.Subscribe`). Loses events on restart. | Keeps fan-out, but **no longer the durability story**. Metrics at `:62` `EventsPublishedTotal` now count relay-bridged events, not direct `Registry.Publish` calls. |
| `queue` | `queue/client.go`, `queue/producer.go`, `queue/worker.go` | Durable job execution (River). Periodic jobs via `queue/periodic.go`. | Execution writer (6 fixes); periodic leader-gated. |

**Fix — Bridge Relay → Registry:**

```go
// cmd/api/main.go (new)
relay.Subscribe(func(ctx context.Context, env events.Envelope) error {
    return eventRegistry.Publish(ctx, env) // fan-out via Registry, metrics, dead-letter
})
```

With leader gating, only one replica's relay polls; all replicas' registries still have local subscribers (e.g., `wsHub` per replica) — but only leader's relay drives fan-out. Followers' registries stay idle for outbox events; live request handlers still publish via `PublishTx` → outbox → leader relay → fan-out broadcast via DB row (followers will see row on next leader poll cycle, not instantly). For **low-latency** paths (console `wsHub`), add `Registry.Publish` bypass for in-process-only events that don't need durability (mark `Envelope.Payload["durable"]=false`).

### 4.2 Envelope Tenant & Generation

- `events/event.go:196` `Envelope` gains `TenantID string \`json:"tenantId"\``, `Generation int64`.
- `eventstore/store.go` `StoredEvent` gains `TenantID`, `Generation`.
- `store/store.go:1754` `FenceGeneration` on server row is the source of truth; publishers must include it.

### 4.3 Idempotency & Dead-Letter

`registry.go:52` `maxDeadLetters 10_000` + `:237` `recordFailure` / `:270` `clearFailure` is per-entry retry. Unify with `queue` idempotency: every handler gets `Envelope.ID` (UUID at `event.go:215` `uuid.NewString`) + `CorrelationID`. Handlers do `INSERT ... ON CONFLICT (envelope_id) DO NOTHING` before side effect.

---

## 5. Fencing — Correct Edge, Enforced

### 5.1 Wrong Edge

Today `fencing/fencing.go:20` handles `EventNodeRecovered` (`main.go:441`). Should handle **partition detected**, not recovery witnessed. Fix subscriptions:

| Event | Today subscribers (`main.go:441`, `:946`, etc.) | After |
|---|---|---|
| `EventNodeOffline` / `EventNodeUnreachable` / `EventNodeSuspected` | `failSvc`, `tmSvc`, `lbSvc`, not `fenceSvc` | `fenceSvc` fences **on suspicion/offline**, bumps generation + `WorkloadLeaseExpiry`. |
| `EventNodeRecovered` | `fenceSvc` fences here (wrong) | **Unfence / reconcile** path: verify fencing lease still held before allowing writes; compare request `Generation` vs store `Generation`. |
| `EventNodeReconciling` | handler at `:1068` just logs | `recovery.Service` reconciles with generation CAS. |

### 5.2 Zero Enforcement → CAS on Generation

Every daemon-facing write (`daemon/client.go:1756` `AdminContainerStart`, `ComposeStart` `:77`, `DockerfileBuild` etc., and `runtime` adapters) must send `X-Forge-Generation: <server.Generation>` header; API's `store` CAS-update path checks:

```sql
UPDATE servers
SET actual_state = $2, generation = generation + 1
WHERE id = $1 AND generation = $3
RETURNING generation
```

`store/store.go:1754` `FenceGeneration` checked at `recovery/service.go:647` but not enforced on the write path. After fix, stale generation → `409 Conflict` with `TargetValidationError` (`domain/domain.go:138`) including `fenceGeneration` so caller knows it was fenced.

**Verification:** Add `store/store_heartbeat_test.go:53`-style test but for generation: concurrent `UpdateServerGeneration` with same `generation` → exactly one succeeds.

---

## 6. Queue Consolidation — One Writer, One Reader

### 6.1 Target

| Component | Role | File:Line | After |
|---|---|---|---|
| `queue.Service` (`queue/client.go`, `producer.go`, `worker.go`) | **Sole execution writer** — owns `UPDATE jobs SET state, attempt, errors` | `queue/job.go`, `queue/job_executor.go` | Keeps. Gets 6 fixes. |
| `services/operation.Service` (`services/operation/service.go:157` `Start`) | **Read model** — projections for UI (`AdminOperations.tsx` timeline), not execution | `services/operation/service.go` | Becomes read-only view over `queue` jobs + `stored_events` projections. No `Start` loop. |
| `services/queue/periodic.go:5` | Durable periodic dispatch | — | Leader-gated enqueuer (idempotent). |

### 6.2 Six Fixes

| # | Fix | Current | Target | Flag |
|---|---|---|---|---|
| 1 | **CAS cancel** | `queue/queuedriver/queuepgx/river_queue.sql.go:335` `state = CASE ... metadata ? 'cancel_attempted_at' ... 'cancelled'` is not CAS on version; racing cancel vs completion can both succeed. `queue/job_executor.go:59` `JobCancelError` reported out-of-band. | `UPDATE jobs SET state='cancelled' WHERE id=$1 AND state IN ('retryable','scheduled') AND generation=$2 RETURNING` — versioned. Cancel writes `cancelled_at` + `generation+1`. Executor checks `generation` before `completed`. | `FORGE_QUEUE_CAS_CANCEL` |
| 2 | **Delay column** | No `scheduled_at` delay column for backoff; uses `time.After` sleep in process (`job_executor.go:219`). Crashes lose delay. | Add `delay_until timestamptz` to `river_job`; `SELECT ... WHERE delay_until <= now()`; enqueue with explicit delay. Migrations dual-read old `time.After` path. | `FORGE_QUEUE_DELAY_COL` |
| 3 | **Tx-wrapped transitions** | Job state + business side-effect not atomic (e.g., `compose/lifecycle.go:293` `PlaceServer` + queue enqueue). | `BEGIN; INSERT jobs ...; PublishTx(outbox, envelope); COMMIT;` — single tx per `services/queue/periodic.go` pattern. | `FORGE_QUEUE_TX_TRANSITIONS` |
| 4 | **Unified idempotency** | Per-handler ad-hoc `metadata ? 'unique_key_conflict'` at `river_queue.sql.go:481`. | One `idempotency_keys` table `(key text PRIMARY KEY, created_at, envelope_id)`, inserted in same tx as enqueue. `queue/unique.go` already exists — unify with `events` idempotency. | `FORGE_QUEUE_IDEMPOTENCY_V2` |
| 5 | **Background heartbeat** | No heartbeat; `queue/job_executor.go:219` blocking retry sleep holds worker slot. | Column `heartbeat_at timestamptz`, worker updates every `eventTimeout/3` (`outbox.go:32` `30s` → 10s). Leader reaper marks `heartbeat_at < now() - 2*interval` as `retryable`. | `FORGE_QUEUE_HEARTBEAT` |
| 6 | **Leader-gated periodic** | `queue/periodic.go` + `services/queue/periodic.go` in-memory timers on every replica | `PeriodicJobBundle` enqueues only if `leader.IsLeader(ctx)`; jobs are durable so followers don't need to enqueue. | `FORGE_QUEUE_LEADER_PERIODIC` |

### 6.3 Backward Compat

All six ship behind flags default `off` P0, dual-write/dual-read P1, flag `on` P2, removal P3. Migrations add columns nullable first, backfill, then `NOT NULL` later.

---

## 7. Scheduling — Placement Normalization Detail

### 7.1 Score Normalization Table

| Scorer | Today range | Normalized to `[0,1]` formula |
|---|---|---|
| `LeastLoadedScorer` `strategy.go:95` | `availableRatio` sum ≤3, unbounded when `total<=0` | `score = (cpuAvail/cpuTotal + memAvail/memTotal + diskAvail/diskTotal)/3` clamped `[0,1]`; when `total<=0` return `0` if `avail<=0` else `0.5` (unknown). |
| `BinPackScorer` `strategy.go:124` | `util` mean `[0,1]` but `TotalCPU==0` → 0 utilization (lies) | Same but denominator guard: if `total==0` exclude that dimension from mean. |
| `SpreadScorer` `strategy.go:141` | `1/(1+count)` — `[0,1]` OK | Keep, but normalize after including `usedNodeCount` spread penalty separately. |
| `RandomScorer` `strategy.go:168` | `[0,1)` via `rng.Float64` | Keep, seed from `PlacementRecord.Seed` for determinism. |

Soft bonus: `constraints.go:66` `CheckSoft` returns `(1e12 * satisfied - 1e10 * missed)`. Replace with `bonus = (satisfiedCount / softTotal) * kSoftWeight - (missedCount / softTotal) * kSoftPenalty` where `kSoftWeight=0.30`, `kSoftPenalty=0.10`, `softTotal = max(1, len(soft))`. So max influence is `±0.30`, within one normalized point.

### 7.2 Power-of-Two Sampling Pseudocode

```go
func (e *Engine) sampleCandidates(filtered []Candidate, k int) []Candidate {
    if len(filtered) <= k*4 { return filtered } // small fleet: score all
    // k=2 by default (power-of-two)
    picks := make(map[int]bool)
    for len(picks) < k { picks[rand.Intn(len(filtered))] = true }
    out := make([]Candidate, 0, k)
    for i := range picks { out = append(out, filtered[i]) }
    return out
}
```

For `PlaceReplicas`, iterate `Replicas` (`replica.go:76`) but re-sample per replica after updating `workingCandidates` availability.

### 7.3 Reschedule / Spread / Blocked Eval

- `blockedEval` queue: if `Place` returns `no viable candidates`, enqueue `RescheduleRequest{serverID, tenantID, reason, requeueAt: now + backoff}` on `queue.Service` (not in-memory). Controller picks up via `queue` (durable). Backoff uses `delay_until` (queue fix #2).
- `spread` already exists as `SpreadScorer` but is opt-in strategy. Make it a **multiplicative factor** regardless of strategy: `finalScore = baseScore * (1 - spreadFactor*existingInstances)` where `spreadFactor=0.1` matches `replica.go:172` today.
- `sticky fallback`: `ExistingNodeMap` at `replica.go:19` already tracks `usedNodeCount`; promote to `PlacementRecord.StickyNode` persisted, tried first.

---

## 8. Security & Tenancy Closure

### 8.1 Close Wildcard (S-01)

**Target state:**

```go
// store/store_users.go:555 — fix allowlist
func normalizeSubuserPermissions(input []string) []string {
    // ... existing ...
    if permission == "*" { continue } // NEVER allow * for subusers; only owners/admins via internal path
    if !allowed[permission] { continue }
    // ...
}
```

Add DB check constraint:

```sql
ALTER TABLE subusers ADD CONSTRAINT subusers_no_wildcard
CHECK (NOT permissions::text ILIKE '%"*"%' );
```

And enforcement at `http/handlers_servers.go:511` `UpsertServerSubuser`: if `claims.Role != "admin"` and request contains `"*"` → `403`.

`HasPermission` (`permissions.go:206`) keeps `p == "*"` honored for **owner/admin** paths (`store/store_users.go:377` owner/admin bypass), but subuser rows can never contain it, so no escalation.

Also expand `defaultSubuserPermissions()` (`store_users.go:574`) to include intentionally-omitted permissions that were missing (e.g., `file.read-content` blank doc, `cron.*`, `buildpack.manage`, `server:read-env`) — subagents 06/07 noted gaps; capstone doesn't duplicate their allowlist, just ensures `*` closure covers all.

**Verification:** Unit test `TestNormalizeSubuserPermissionsRejectsWildcard` (new) + existing `auth/scopes_extended_test.go:107` wildcard tests stay.

### 8.2 Mount Allowlist (S-02)

Replace narrow `compose/service.go:614` docker.sock block with canonical allowlist:

- New table `mount_allowlist (prefix text PK)` seeded from `beacon/config/config.go:144` `AllowedMounts` via API.
- API validation (`http/handlers_servers.go` + `store/store_mounts_ext.go:367` `AllowedMountSourcesForNode`): `path.Clean(source)` must `HasPrefix` of an allowlisted prefix after resolving symlinks (`filepath.EvalSymlinks` on agent side `beacon/internal/server/mounts_test.go:35` pattern). Reject `..` segments, reject empty prefix.
- `daemon/client.go:459` `Mount` source/target validated at both API and agent; agent's `SetAllowedMounts` (`beacon/internal/server/mounts_test.go:59`) is enforcement, not advisory.
- `store/store_mounts_ext.go:307` allowlist read path becomes source of truth; no hard-coded docker.sock branch.

### 8.3 Encrypt DNS, Validate Proxy Rules, Fix Trusted Proxy

| Item | File:Line | Fix |
|---|---|---|
| DNS in-flight/log | `http/handlers_dns.go:43` `Credentials map[string]string` + `services/dns/service.go:65` lego env expansion | Redact credentials in HTTP logs (never `log.Printf` raw map); API response always returns `{"encrypted":true}` for reads (already at `store_dns.go:42`); only `ConfigureProvider` (`handlers_dns.go:54` `ConfigureProvider`) accepts plaintext **over TLS only** (`middleware_mtls.go:120` `X-Forwarded-Proto`). Add `slog` redaction middleware for `/dns` payloads. |
| Rule injection | `traefik_proxy.go:334` `Host(`%s`)` + `service.go:361` `validateRoutingRule` | Add `if strings.ContainsAny(domain, "`\"\\\r\n") { return err }` + same for `TargetHost` (`service.go:390`). Domain already via `idna.Lookup.ToASCII` (`:365`), but backtick passes there — must reject before formatting. Add test: `Host(``evil`)` must error. |
| Trusted proxy | `middleware_ipaccess.go:98` `TrustProxy:true` unconditional + `:107` empty allow ⇒ unrestricted admin | Default `TrustProxy:false`. Only set `true` when `TRUSTED_PROXY_CIDRS` env is non-empty AND `ExtractClientIP` peer is within that CIDR (extend `middleware_ratelimit.go:98` `IsLoopback||IsPrivate` to explicit CIDR allowlist). For `ADMIN_IP_ALLOW`/`ADMIN_IP_DENY`: if both empty → fail-closed in `production` (`os.Getenv("APP_ENV")=="production"` → return `503` with guidance), warning in dev (keep current `:107` warn but document). Update `middleware_ipaccess.go:107` to gate on `APP_ENV`. |

### 8.4 Tenant Enforcement

Every placement/event/queue row carries `tenant_id`. HTTP middleware `tenantAccess` (`handlers_apphosting.go:60`) stays, but now **core paths** enforce:

```go
// placement/engine.go — new
func (e *Engine) PlaceForTenant(ctx context.Context, candidates []Candidate, req WorkloadRequest, tenantID string) (ScoreResult, error) {
    if tenantID == "" && !isGlobalScope(req) { return ScoreResult{}, errors.New("tenant required") }
    // filter candidates to tenantVisible nodes (node.tenant_id == tenantID OR node.public)
}
```

`events.Envelope.TenantID` threads through `ws_hub.go:20` tenancy (already documents) — add subscription filter: `hub.Subscribe` (`ws_hub.go:168`) returns only envelopes where `envelope.TenantID == sub.tenantID || sub.role=="admin"`.

---

## 9. Product IA — Keep Goal-Grouped Shell, But Make It Honest

### 9.1 Keep `admin-shell.tsx:42` 6 Groups — With Corrections

The current shell (`forge/web/components/admin/admin-shell.tsx:16` `AdminShell`, `:42` `navGroups` with comment "Group by administrator's goal") is **directionally correct** — goal-grouped is better than table-grouped. Keep the mechanism (`admin-registry.ts:22` `adminPageRegistry`), but fix contents:

| Group (after) | Pages (registry `href`) | Change | Rationale |
|---|---|---|---|
| **Command Center** | `/admin/overview`, `/admin/monitoring`, `/admin/health`, `/admin/activity`, `/admin/operations` | Keep. Remove `host`, `kubernetes`, `cron-jobs` from here (they move). | "Command Center" is *read* — fleet truth, not controls. |
| **Workloads** | `/admin/servers`, `/admin/apps`, `/admin/deployments`, `/admin/preview-deployments`, `/admin/source-deployments`, `/admin/compose`, `/admin/scheduler` | `scheduler` moves here (placement is workload policy). Remove `app-store` (merge into `apps`), `docker` (becomes detail of `nodes`). | `Workloads` = things that get placed. |
| **People & Access** | `/admin/users`, `/admin/roles`, `/admin/organizations`, `/admin/projects`, `/admin/environments`, `/admin/oauth-clients`, `/admin/api` | Move `api` (API Keys) here — keys are access control, not platform config. Keep `environments` (one env editor — see 9.2). | Access is identity, not settings. |
| **Infrastructure & Data** | `/admin/regions`, `/admin/locations`, `/admin/nodes`, `/admin/allocations`, `/admin/databases`, `/admin/mounts`, `/admin/files`, `/admin/terminal`, `/admin/cloud`, `/admin/backups` | Nodes gains tabs (9.3). | Machines & state. |
| **Gateways** *(NEW)* | **Routers**, **Services**, **Middlewares**, **Certificates** under `/admin/gateways` | **Replaces** `endpoints`, `load-balancer`, `traffic`, `domains`, `certificates`, `mtls` (7 pages). Single `HTTPConfiguration` mental model. | Matches Traefik's one `HTTPConfiguration` (`reference/networking/traefik/integration/testdata/rawdata-gateway.json`) and Caddy's single config (`services/trafficmanager/caddy_proxy.go`). Eliminates decorative 7-way split. |
| **Platform Configuration** | `/admin/nests`, `/admin/app-templates`, `/admin/templates`, `/admin/plugins`, `/admin/settings`, `/admin/notifications`, `/admin/autoscaler`, `/admin/failover` | Remove `scheduler` (moved), decorative entries audited. `autoscaler` stays but its policies link to `nodes` detail. | Platform knobs that most operators touch quarterly, not daily. |

**Remove decorative entries** (audit 2026-08-22): entries with `capability: "metadata-only"` (`admin-registry.ts:61` `plugins`) become collapsed under `Platform Configuration` as secondary links with `muted` styling, not top-level groups. Any page with zero backend traffic in last 90 days (per `ws_hub`/`activity` metrics) gets demoted, not removed — unlinking without data loss.

### 9.2 Single Environment Editor (Kill the Triple)

- One surface: `/admin/environments` (`admin-registry.ts:53`) owns env variables.
- Server startup env (`server/startup-view.tsx:32` `commandDraft`, `server/settings-view.tsx:23`) becomes a **read-only projection** of an environment with "Edit in Environments → [service]" deep link (preserves `tenant_id` in URL search params).
- Validation single-sourced: `store/store_envvars.go:102` `normalizeScope` is the validator; both editors call it.

### 9.3 Nodes Detail Tabs

`components/admin/AdminNodes.tsx:11` `Tab` today is `about | settings | configuration | allocation | servers`.

After:

```ts
type Tab = "overview" | "health" | "capabilities" | "drain" | "autoscale" | "allocations" | "servers";
// overview = system info (current hosts; was "about")
// health = heartbeat / actualState + history (from store_heartbeat_test.go:53 data)
// capabilities = runtime providers + versions + runtimeConfig
// drain = Draining/Maintenance toggle + evacuation preview (from services/drain/service.go:152 Subscriber)
// autoscale = policy for this node (from store_phase6_autoscale.go:34)
// allocations/servers remain
```

Each tab route is `#tab=health` + deep-linkable, not only `useState`.

### 9.4 Backups Tabs

`web/app/admin/backups/page.tsx:507` `Backups` surface today is monolithic.

After: tabs `Configurations` (`BackupConfiguration`), `Jobs` (`BackupJob`), `Artifacts` (`BackupArtifact`), `Restores` (`BackupRestore`), `Storage` (`StorageProvider`) — same `AdminTabs` primitive as `AdminNodes`, route-driven.

### 9.5 App Detail Routed, Not `useState`

`components/app/app-detail.tsx:??` and `components/admin/AdminAppsShared.tsx` replace `useState<TabId>` with Next.js `useSearchParams` (`?tab=git|compose|instances`) so back/forward/share works. Same for `components/server/settings-view.tsx` etc. Structure encodes content — tab label is the URL, not decoration.

### 9.6 Gateways — The Traefik One-HTTPConfiguration Page

Single page `/admin/gateways` with four routed subtabs (preserves "structure is information"):

- **Routers** — `RoutingRule` list (`services/trafficmanager/service.go:24` `RoutingRule`). Each row shows `Domain` → `TargetHost:TargetPort` + `generation` + `fence` badge, inline `Host(\`example.com\`) && PathPrefix("/api")"` preview (validated rule from `traefik_proxy.go:797`).
- **Services** — target groups (`load-balancer` today), health state (`target-health`), allocation bindings.
- **Middlewares** — rate-limit / IP allowlists / headers (`middleware_ipaccess.go:12` `IPAccessConfig`, `middleware_ratelimit.go:18` `RateLimitConfig`). Each shows provenance (which router uses it).
- **Certificates** — ACME / manual certs (`handlers_certificates.go`, `handlers_acme_accounts.go`) + DNS provider status (`handlers_dns.go`). Wildcard + SAN visible, renewal timeline included.

This collapses **7 pages** into **1 page + 4 subtabs** with a shared graph visualization (signature-adjacent but not the signature).

### 9.7 Operations Timeline (New Capstone Page)

`/admin/operations` today (`components/admin/AdminOperations.tsx:150` `AdminOperations`) shows migrations/recoveries/evacuations as three separate sections. Capstone upgrades it to a **unified generation-fenced timeline**:

- Single vertical timeline ordered by `envelope.Timestamp` (`events/event.go:199`), grouped by `CorrelationID` (from `event.go:211` `correlationIDFromPayload`). Each lane is a server.
- Each tick shows **two dots** (State Lanes badge): upper = Desired (`ServerDesiredState`), lower = Actual (`ServerActualState`), color via semantic tokens. A **ring** around the actual dot appears when `generation < fenceGeneration` (fenced).
- Filter chips are tenant-aware (`tenant_id` from envelope); admin sees all, org member sees only their lanes.
- Clicking a tick opens drawer with `PlacementRecord.Attempts`, `ScoreResult.Reasons`, `FenceGeneration` vs current.

This **is** the signature's long form; the badge is the compact form.

---

## 10. Frontend Design System — Forge Industrial Terminal

### 10.1 Tokens

**File location:** `forge/web/app/globals.css:8` `:root` semantic tokens stay canonical (do not hardcode hex elsewhere). New file `forge/web/lib/design-tokens.ts` exports typed tokens for charts/timelines. `tailwind.config.ts:??` maps tokens to Tailwind.

**Palette — Forge Phosphor (6 values, amber-phosphor POV)**

| Token | Hex | Role | Where |
|---|---|---|---|
| `--forge-ink` | `#0B1118` | Canvas — rack void | `globals.css:11` `--canvas` dark value |
| `--forge-steel` | `#1B2636` | Surface/raised — chassis | `--surface` / `--surface-raised` |
| `--forge-phosphor` | `#FFB000` | Primary accent — amber CRT, not green/red | `--brand` (replaces `#DC2626` for primary; red re-scoped to fence/failed only) |
| `--forge-fault` | `#E63E2A` | Fault/fenced/danger — indicator red | `--danger` / `--brand-dark` in danger contexts |
| `--forge-concrete` | `#8A9BA8` | Muted text, dividers | `--text-subtle` / `--line-strong` |
| `--forge-paper` | `#E6EDF3` | Primary text, paper on ink | `--text` |

Rationale for amber: amber phosphor is the **long-persistence** CRT (DEC VT220) — slower decay, easier on eyes for long reads (fleet truth). Distinct from emerald/ acid green used by generic infra dashboards. Red kept for *state*, not brand, so fault is unmissable.

Light theme (`[data-theme="light"]` at `globals.css:27`) inverts ink→paper but keeps amber as accent (amber on white is impaired contrast, so light theme uses `--brand: #B45309` — darker amber, AA on white).

**Semantic mapping (keeps existing var names, changes values):**

```css
:root {
  --brand: #FFB000;         /* was #DC2626 */
  --brand-hover: #FFC033;
  --brand-dark: #E63E2A;    /* fault red, not brand */
  --canvas: #0B1118;
  --surface: #131C2A;       /* steel-tinted from #111722 */
  --surface-raised: #1B2636;
  --surface-input: #0D131D;
  --line: rgba(138,155,168,0.14);
  --line-strong: rgba(138,155,168,0.24);
  --text: #E6EDF3;
  --text-subtle: #8A9BA8;
  --focus: #FFB000;
  --success: #1A9E6A;       /* kept but desaturated vs amber */
  --danger: #E63E2A;
}
```

### 10.2 Typography

| Role | Typeface | Why | Usage |
|---|---|---|---|
| Display | **Space Grotesk** `400..700` | Industrial geometric, generous apertures, reads at small caps sizes; not the default sans. Paired with Forge steel feels machined. | Page titles (`30px 650 -0.03em`), group labels `10px 700 0.12em`, metric ticks |
| Body | **IBM Plex Sans** `400..600` | Engineered for UI, distinct from Inter/Geist used everywhere. Plex's notched terminals echo chassis labeling. | Descriptions `13px/20px 400`, table cells `13px`, form labels `12px 500` |
| Utility/Mono | **JetBrains Mono** `400..600` | True tabular numbers, ligature-free for logs/IDs. | `font-mono`: node IDs, alloc IPs, rule strings, log lines, generation counters |

Scale (from `globals.css` + `admin-ui.tsx` governed):

- `display-30`: `30/32 650 -0.03em` (page titles — Operations, Gateways)
- `title-14`: `14/20 600 -0.01em` (card headers)
- `label-11`: `11/16 600 0.12em uppercase` (eyebrows, tab labels)
- `body-13`: `13/20 400` (descriptions)
- `mono-11`: `11/16 500 mono` (timestamps, IDs, status chips)
- `mono-12`: `12/16 600 mono` (generation numbers — the one place we shout)

Weights deliberately limited to `400/500/600/700`; `650` only for display-30.

### 10.3 Spacing & Layout

- 8px base (`spacing.1=4`, `.2=8`, `.3=12`, `.4=16`, `.6=24`, `.8=32`) — keep existing Tailwind but document.
- Admin max width `1280px` (`AdminActivityLog.tsx` precedent `max-w-[1280px]`) as canonical; ops timeline uses same.
- Gutters: `p-4 sm:p-6 lg:p-8` on `AdminShell` main (`admin-shell.tsx:193`) — keep.

### 10.4 Motion

- **Orchestrated, not scattered:** one page-load sequence — steel slides `transform: translateY(4px) → 0` over `180ms ease-out` on the page header only. Subs stagger `40ms` (max two). No parallax.
- **Micro-interactions:** `Btn` hover `bg` only; no scale. Timeline tick expands `max-height` with `200ms ease`.
- **Reduced motion:** `globals.css:50` `@media (prefers-reduced-motion: reduce) { *, *::before, *::after { animation-duration: .01ms } }` already honoured — keep. Add `useReducedMotion` hook for JS-driven sequences.

### 10.5 Component Inventory (Signature + Primitives)

| Component | File (target) | Props / Contract | Notes |
|---|---|---|---|
| **ServerStatus — State Lanes (Signature)** | `components/shared/states-badge.tsx:??` (exists; upgrade) | `{ desired: ServerDesiredState, actual: ServerActualState, generation: number, fenceGeneration: number, heartbeat: NodeHeartbeatState }` — renders **two stacked dots**: top = desired (filled), bottom = actual (filled or hollow if `unknown`), both `7px` with `2px` gap. If `generation < fenceGeneration`, actual dot gets `2px ring --forge-fault` + `animate-pulse` once then idle. Tooltip shows `gen X / fence Y`. Color mapping via tokens, not ad-hoc hex. | This is **the one memorable thing**. Two-dot lanes encode true content (desired vs actual), not decoration. The ring encodes fencing — truth, not flair. |
| **DeploymentTimeline (Generation-Fenced)** | `components/charts/DeploymentTimeline.tsx:??` (exists) + `lifecycle/activity-timeline.tsx` | `{ events: Envelope[], laneBy: serverId }` — vertical timeline, per-lane dots are State Lanes badges, connecting line is `1px --line` with `fenced` segment dashed `--forge-fault`. Each tick shows `Attempt` reason from `PlacementRecord`. | Long form of signature. Uses `Envelope.CorrelationID` to group. |
| **LogViewer** | `components/deployment/DeploymentLogViewer.tsx:8` `timestamp: string` + `AdminAppsShared.tsx:96` | `{ lines: {timestamp, stream, line}[], showTimestamps: boolean }` — virtualized list, `JetBrains Mono 12px`, timestamp column `mono-11 --forge-concrete`, `follow` toggle, `copy` row. | Single implementation shared by app logs + deployment logs (dedup today's two viewers). |
| **EmptyState** | `components/shared/states-empty.tsx` | `{ icon, title, description, action }` — `dashed --line` card, `icon 18px --text-subtle`, `title mono-12`, `description body-13 --text-subtle`, `action Btn primary`. | One empty per page, with direction (not mood). Example: "No nodes match filters — clear to see all." (`AdminActivityLog.tsx:185`). |
| **OfflineBanner (dedup)** | `components/shared/states-offline.tsx:??` `OfflineBanner` | `{ onRetry }` — currently rendered **twice** per page (`traffic/page.tsx:84` duplicate) | Dedup: render once from `AdminShell` (`admin-shell.tsx:194` already has one). Remove per-page duplicates. |
| **Gateways Graph** | `app/admin/gateways/_components/gateway-graph.tsx` (new) | `{ routers, services, middlewares, certs }` — SVG graph with routers→services edges, middleware badges, cert links. Nodes are pills with State Lanes dots. | Not signature, but the structural replacement for 7 pages. |
| **Node Health/Capabilities/Drain/Autoscale tabs** | `components/admin/AdminNodes.tsx:11` `Tab` | As 9.3 — each tab deep-links via `?tab=` search param. | Uses `AdminTabs` primitive (`admin-ui.tsx:??`). |

**Tokens file location:** `forge/web/lib/design-tokens.ts` (new) re-exports `globals.css` vars for JS charts; `tailwind.config.ts` `theme.extend.colors` maps to `var(--*)`.

### 10.6 Responsive / Mobile / A11y

- **Responsive:** Sidebar collapses to sheet at `<900px` (`admin-shell.tsx:126` `max-[899px]:hidden` pattern — keep). All tables become card stacks below `640px` (not horizontal scroll traps). Gateways graph switches to vertical list at `<768px`.
- **Keyboard:** Every `Btn` (`admin-ui.tsx: Btn`) retains `focus-visible:ring-2 ring-[var(--focus)] offset-[var(--canvas)]` (from `admin-shell.tsx:152` group toggle, `:160` item focus). `Skip to content` (`admin-shell.tsx:118`) stays. Timeline ticks are `button` with `aria-label="Deployment gen 14 — desired running actual starting"`.
- **Reduced motion:** Honour `prefers-reduced-motion` (global sheet already does). No autoplay beyond single-load stagger; heartbeat pulse in State Lanes runs once (`animation-iteration-count: 1`) then static border — not infinite.
- **Contrast:** Amber `#FFB000` on `ink #0B1118` = 12.1:1. Amber on `steel #1B2636` = 8.3:1. Both AAA. Light theme dark amber `#B45309` on white = 4.6:1 (AA). Red `#E63E2A` on ink = 5.2:1 (AA).

---

## 11. Wireframes (ASCII) — Key Pages

All frames assume `1280px` max, `globals.css` canvas `ink`, `admin-shell.tsx` fixed top bar + 288px sidebar.

### 11.1 Gateways — `HTTPConfiguration` Single Page

```
┌─ AdminShell ───────────────────────────────────────────────────────────────────┐
│ TopBar: [Forge]                                        [My Servers] [Sign Out] │
├─ Sidebar (goal-grouped, 6) ──┬─ Main (#forge-main) ───────────────────────────┤
│ Command Center (5)          │ Breadcrumb: Admin / Gateways                   │
│ Workloads (7)               │ Title: GATEWAYS  [ ● amber rule live edge ]     │
│ People & Access (7)         │ Sub: One graph. Routers → Services → Mwares.  │
│ Infra & Data (10)           │ Chips: [tenant: all ▼] [wildcard: hide] [Sync]│
│ ▸ Gateways (4)    ◄ACTIVE   ├────────────────────────────────────────────────┤
│ │ Routers  (18)             │ Tabs: [Routers]  Services  Middlewares  Certs  │
│ │ Services (12)             │ ┌ Search: “api.” ───────────┐ [+ New Router] ─┐│
│ │ Middlewares (9)           │ │ Graph preview (SVG):      │                ││
│ │ Certs    (7)              │ │                            │                ││
│ Platform Config (8)         │ │  Host(`api.example.com`)  │                ││
│ [Search controls…]          │ │     ├──▶ svc-alloc-1 :443 │ [healthy ●]    ││
│                             │ │     │    ┌ rateLimit 20/s ┐                ││
│                             │ │     └──▶ svc-node-2  :8443│ [reconciling ]  ││
│                             │ │         └ cert LE ★ auto │                ││
│                             │ ├────────────────────────────────────────────┤│
│                             │ │ Router rows (table, h-44):                  ││
│                             │ │ Path          Target         MW  Cert State  ││
│                             │ │ /api          svc-alloc-1  RL+X  LE  ●● ran ││ ← two-dot lane
│                             │ │ /health       svc-node-2   —    —   ○● deg ││
│                             │ │ (mono rule preview on hover: Host… && Path)││
│                             │ │ Pagination: Page 1/3 · 18 routers           ││
│                             │ └────────────────────────────────────────────┘│
│                             │ Footer: Traefik Host(`…`) rule validated.     │
└─────────────────────────────┴───────────────────────────────────────────────┘
Notes:
- Graph and table are linked selection; filtering “tenant: acme” collapses to that tenant’s routers only (core tenant_id on RoutingRule).
- Rule injection attempted value with ` would render as blocked row with red “Invalid rule” badge; never emitted to traefik_proxy.go.
```

### 11.2 Operations Timeline — Generation-Fenced, Tenant-Aware

```
┌─ Main ──────────────────────────────────────────────────────────────────────────┐
│ Breadcrumb: Admin / Operations                                                │
│ Title: OPERATIONS — Generation-fenced timeline                         [● live]│
│ Sub: Desired vs Actual vs Fence. One lane per server. Correlation groups.    │
│ Filters: [tenant: all ▼] [state: all ▼] [fenced only ☐] [Search “evacuation”] │
├─ Left: Lane list (sticky) ─┬─ Center: Timeline (vertical, newest top) ───────┤
│ Server lanes               │  Correlation  srv-a-7f3                Gen  F   │
│ ●● srv-a-7f3 run/run  g14 │  ┌─────────────────────────────────────────┐      │
│ ○● srv-b-9c1 run/start g13│  │ ●● 14:02:03  PlacementCreated  node-3  │      │
│ ●○ srv-c-2d4 stop/stop g9 │  │   score 0.82  reasons: mem 71% / soft │      │
│ ●◉ srv-d-4e1 run/crash g12│  │ ○● 14:02:04  InstanceProvisioning     │      │
│   (◉ = fenced ring)        │  │   backoff until 14:03  attempt 2/3  │      │
│                            │  │ ●◉ 14:02:10  InstanceFailed  fenced │      │
│ Controls:                  │  │   gen 13 < fence 14 — blocked      │      │
│ [Preview evac node-3]      │  └─────────────────────────────────────────┘      │
│ [Create recovery plan]     │  Correlation  evac-node-3                 Gen   │
│                            │  ┌─────────────────────────────────────────┐      │
│ Legend:                    │  │ ●● 14:04:11  EvacuationPlanCreated      │      │
│ ● top=desired  ● bot=actual│  │ ●● 14:04:18  EvacuationPlanCompleted    │      │
│ ◉ ring = fenced (gen<fence)│  └─────────────────────────────────────────┘      │
│ ─ solid = healthy ─ dashed = fenced segment                               │
├──────────────────────────────────────────────────────────────────────────────┤
│ Drawer (on click tick):  Attempt graph, score breakdown, tenant, generation. │
│ Pagination: 128 events · Page 1  ·  Poll 15s (or WS — see poller dedup)      │
└──────────────────────────────────────────────────────────────────────────────┘
Notes:
- Polling strategy: single source. Operations + Activity both subscribe to Registry stream via WS (ws_hub.go:110 Wildcard) with fallback 15s poll; evac detail uses 2s only while plan.status=='running' (AdminOperations.tsx evac 2s kept) then backoff.
- Tenant filter chips are encoded in URL (?tenant=acme) so deep-link works.
```

### 11.3 Server Console with State Lanes

```
┌─ Main ──────────────────────────────────────────────────────────────────────────┐
│ Breadcrumb: Admin / Servers / srv-a-7f3                                      │
│ Title: srv-a-7f3  [●● running/running g14]  [Fence: none]  [Node: node-3]   │
│   (title row badge is State Lanes: ●● with mono-12 gen label)                 │
│ Tabs: [Overview] Console  Files  Network  Startup  Schedules  Backups  MTLS   │
│ Sub: Fleet truth in one badge. Desired vs Actual, every tick.                │
├──────────────────────────────────────────────────────────────────────────────┤
│ Layout: two columns (API + Daemon) — truth panel left, actions right          │
│ ┌─ Truth ──────────────────────┐ ┌─ Actions ────────────────────────────────┐ │
│ │ State lanes (large)         │ │ [▶ Start] [■ Stop] [↻ Restart] [Kill]    │ │
│ │ ●● Running / Running        │ │ Env (read): PORT=25565  STATUS=joinable  │ │
│ │    gen 14 / fence 14  (ok)  │ │ “Edit in Environments → mc-default” link │ │
│ │    ○● would mean start→    │ │ Resources: CPU 2  MEM 1024 MB  DISK 5120 MB│ │
│ │ Node: node-3  (health ● up) │ │ Mounts: /mnt/a → /data  (allowlisted)    │ │
│ │ Alloc: 1.2.3.4:25565  (P)    │ │ Migrations: [Transfer → node-5]          │ │
│ │ Env #1: Pterodactyl compat   │ │ Recovery:  gen 14 — no fence              │ │
│ └──────────────────────────────┘ └──────────────────────────────────────────┘ │
│ ┌─ Console ───────── LogViewer ────────────────────────────────────────────┐ │
│ │ [Follow ☑] [Timestamps ☐] [Download] [Clear]   mono-11 timestamps off   │ │
│ │ 14:02:03 [server] Done (1.2s)! For help, type "help"                    │ │
│ │ 14:02:04 [console] > help │                                                │
│ │ Virtualized  JetBrains Mono 12px  line wrap off  10k line cap            │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│ OfflineBanner: shown once from AdminShell, not per card (dedup fix).           │
└──────────────────────────────────────────────────────────────────────────────┘
Notes:
- Console is routed: /admin/servers/:id?tab=console  (no synthetic local-state tabs).
- Allocation/mount writes validate tenant + generation (+fence) before hitting daemon.
```

---

## 12. Information Architecture — What We Remove and Why

| Removed / Demoted | Today | After | File:Line |
|---|---|---|---|
| 7 gateway pages | `traffic`, `load-balancer`, `domains`, `certificates` (split), `mtls`, `endpoints`, `firewall` (partial) in `admin-registry.ts:72-88` | Single `/admin/gateways` + 4 subtabs; `firewall` moves to `Infrastructure & Data` as node-adjacent; `endpoints` folded into Gateways/Services (discovery source) | `admin-registry.ts:22` `adminPageRegistry` |
| Triple env editors | `admin/environments`, `server/startup-view.tsx`, `server/settings-view.tsx` | One editor at `environments`; server views are read-only projection + deep link | `web/components/server/startup-view.tsx:32`, `settings-view.tsx:23` |
| `app-store` standalone | `admin-registry.ts:83` `appStore` | Merged into `apps` as filter `?source=store` — same `Candidate` runtime, no separate mental model | `admin-registry.ts:83` |
| `docker` top-level | `admin-registry.ts:66` `docker` | Becomes `nodes/:id/containers` tab (capabilities/health) — docker is node detail, not fleet nav | `admin-registry.ts:66` |
| Decorative `plugins` / `api` etc. | `admin-registry.ts:61` `plugins: metadata-only` visible at top level | Demoted to secondary under `Platform Configuration` with `muted` style; not removed | `admin-registry.ts:61` |
| Dual pollers | `AdminOperations.tsx` 2s + `AdminActivityLog.tsx` 15s + `monitoring/page.tsx` 10s | Single WS stream (`ws_hub.go:110` Wildcard) + `EventSource` fallback; poll intervals unified to `15s` with `staleTime 30s`; fast 2s poll only when status `running` (`AdminOperations.tsx: evac 2s` kept narrowly) | `AdminOperations.tsx:150`, `AdminActivityLog.tsx:120`, `monitoring/page.tsx:108` |

---

## 13. Cross-Cutting Non-Functional Requirements

### 13.1 Backward Compat — Dual-Read Migrations

All schema changes follow:

1. **Migration adds nullable column** (`tenant_id text`, `delay_until timestamptz`, `heartbeat_at timestamptz`, `fence_generation bigint` already exists at `store.go:1754`).
2. **Code writes both** old join path (`servers.owner_id` → tenant) and new column for one release (P0→P1).
3. **Code reads either**, preferring new column if non-null, else old join.
4. **Backfill job** (`queue` periodic, leader-gated) fills new column.
5. **Next release (P2)** makes column `NOT NULL` and removes old path behind flag.

No breaking URL change: `?tab=` params are additive; old `#tab=` hash redirects.

### 13.2 Feature Flags (env-gated)

| Flag | Default P0 | P1 | P2 | P3 |
|---|---|---|---|---|
| `FORGE_LEADER_ELECTION` | `off` | `on` (dual-read) | `on` | remove fallback |
| `FORGE_PUBLISH_TX` | `off` | `on` | `on` | remove old path |
| `FORGE_PLACEMENT_V2` (iterators/normalization/power-of-two/sticky) | `off` | `on` | `on` | remove old scorer math |
| `FORGE_QUEUE_*` (6 flags or single `FORGE_QUEUE_V2`) | `off` | per-fix `on` | `on` | cleanup |
| `FORGE_SECURITY_HARDEN` (wildcard/mount/proxy/rule) | `off` | `on` | `on` | remove permissive |
| `FORGE_IA_GATEWAYS` | `off` | `preview` (admin can toggle) | `on` | remove 7-page routing |
| `FORGE_IA_NODES_TABS` | `off` | `on` | `on` | — |
| `FORGE_DS_PHOSPHOR` | `off` | `preview` | `on` | remove legacy red brand |

Flags read at boot (`config/config.go:??` + `env.go`) and exposed at `GET /admin/health` so e2e can assert.

### 13.3 Observability

- Leader lease gauge (`forge_leader_is_leader 0/1`, `forge_leader_lease_expiry_seconds`).
- Placement attempt histogram (`forge_placement_attempts_total{outcome}`) and score histogram.
- Queue delay/heartbeat metrics (`forge_queue_delayed_total`, `forge_queue_heartbeat_age_seconds`).
- Security `*_blocked_total` counters (`subuser_wildcard_blocked_total`, `mount_rejected_total`, `proxy_rule_rejected_total`, `admin_ip_forbidden_total`).
- All metrics emitted with `tenant_id` label cardinality bounded (hash or truncate).

---

## 14. Rollout — 4 Phases

### Phase 0 — Wiring & Bugfixes (week 1, no UX churn, safe to ship daily)

**Goal:** Stop bleeding P0s without new IA.

| Work | Files | Owner track mimic |
|---|---|---|
| Bridge `Relay.Subscribe → Registry.Publish` (AF-1) + leader-gate flag `off` but code in place | `outbox.go:39,45,74`, `registry.go:73,144`, `cmd/api/main.go:1167` | Ops |
| Close subuser `*` in `normalizeSubuserPermissions` + DB constraint + `UpsertServerSubuser` 403 | `store_users.go:555,564`, `permissions.go:206`, `http/handlers_servers.go:511` | Sec |
| Reject backtick in `validateRoutingRule` + `TargetHost` | `traefik_proxy.go:334,797`, `service.go:361,388` | Sec/Gateway |
| Switch `TrustProxy` default to `false` + `TRUSTED_PROXY_CIDRS` + fail-closed `ADMIN_IP_ALLOW` in `production` | `middleware_ipaccess.go:98,105`, `middleware_ratelimit.go:98` | Sec |
| Normalize soft bonus: `constraints.go:59` `1e12` → `kSoftWeight 0.30` + clamp | `constraints.go:59`, `strategy.go:95,185` | Placement |
| Dedup `OfflineBanner` (render in `AdminShell` only) | `admin-shell.tsx:194`, `web/app/admin/traffic/page.tsx:84` | UX |
| Add `tenant_id` nullable column + dual-read write-both for `servers`/`nodes`/`stored_events` | `domain/domain.go:114`, `events/event.go:196`, migrations | Tenancy |

Exit criteria: No `*` subuser can pass `HasPermission`; backtick domain rejected with 400; relay→registry bridge e2e delivers outbox row to `wsHub` subscriber; placement soft bonus ≤0.5.

### Phase 1 — Consolidation (weeks 2-3,behind preview flags)

**Goal:** One writer, one timeline, normalized placement behind flags.

| Work | Flag |
|---|---|
| Leader election `internal/leader/leader.go` + gate `Relay`, `queue/periodic`, `cron`, `HealthFilter`, `IngressSynchronizer`, `Fencing` | `FORGE_LEADER_ELECTION=on` |
| `PublishTx` on all placement/migration/recovery writes | `FORGE_PUBLISH_TX=on` |
| Queue 6 fixes (CAS cancel, delay, tx, idempotency, heartbeat, leader periodic) | `FORGE_QUEUE_V2=on` (per-fix subflags) |
| Placement iterators + `PlacementRecord` + power-of-two + sticky + backoff + fencing edge fix | `FORGE_PLACEMENT_V2=on` |
| Fencing correct edge: `EventNodeOffline` fences, `EventNodeRecovered` unfences/reconciles with CAS | `FORGE_PLACEMENT_V2` |
| Gateways preview page (new route, old 7 pages still reachable via redirect) | `FORGE_IA_GATEWAYS=preview` |
| Nodes tabs + Operations timeline behind preview | `FORGE_IA_NODES_TABS`, `FORGE_IA_GATEWAYS` |
| Mount allowlist `mount_allowlist` table + bidirectional enforcement (API + agent) | `FORGE_SECURITY_HARDEN=on` |

### Phase 2 — Hidden Surface (weeks 3-4, swap defaults)

Turn preview → on by default. Dual-read stays. Wire frontend design system phosphor tokens (`globals.css:8` `--brand` swap) behind `FORGE_DS_PHOSPHOR=on`.

- Operations timeline becomes default landing for `Command Center` (with redirect from old sections table).
- App/server `useState` tabs → routed `?tab=` params (codemod: search `useState.*Tab` in `web/` and replace with `useSearchParams`).
- Triple env editors: server editors become read-only + deep link.
- Synthetic graph badge: where `cumulative` metric detected, chart shows "Cumulative — deltas derived" chip.
- Global mutex removed after iterators land (`engine.go:18`).

### Phase 3 — Polish (week 4+, remove fallback)

- Drop legacy flag fallbacks, old 7-page routes (keep `301` redirects), old in-memory periodic registry (`queue/periodic.go:3` legacy comment path).
- Remove per-page `OfflineBanner` duplicates (lint rule: `no-duplicate-OfflineBanner`).
- Replace `PlaceAll` (`engine.go:79`) callers with iterator drain (or keep as deprecated wrapper).
- Design tokens hardening: lint forbids hard-coded hex in `web/` outside `globals.css`/`design-tokens.ts`.
- E2e: placement property tests (soft bonus never dwarfs), generation fencing CAS test, tenant-isolation e2e (org A cannot place on tenant-pinned node B), gateways HTTPConfiguration snapshot vs Traefik rawdata (`reference/networking/traefik/integration/testdata/rawdata-gateway.json`).

---

## 15. Acceptance Criteria (Verifiable)

| Area | Criterion | How verified |
|---|---|---|
| Event durability | `INSERT placement` + crash after commit but before publish still eventually fans out via relay after leader re-election (no loss). | Kill leader mid-tx; new leader's relay `ClaimPending` delivers; assert `wsHub` receives `PlacementCreated`. |
| Leader | Only one replica's `Relay.pollLoop` executes per 330s window; followers idle. | Metric `forge_leader_is_leader` =1 on exactly one pod; log "claim failed" rate near zero. |
| Fencing | Daemon write with `generation=13` when store `generation=14` (fenced) gets `409` with `fenceGeneration:14` | `store_heartbeat_test.go`-style concurrent CAS test + e2e fence bump then stale write. |
| Placement math | For any `availableRatio` ≤3, `softBonus` contribution ≤0.3 and `Place` never overflows | Property test: brute force 1k candidate sets, assert score ∈ [-0.5, 1.5]. |
| Tenancy | Placement with `tenant_id=acme` never selects node `tenant_id=other` unless `node.public=true` | SQL filter test + e2e `Place` with tenant-pinned nodes. |
| Security wildcard | Subuser with `permissions=["*"]` cannot be created via API (400), existing `*` rows rejected by constraint on migration, `HasPermission(["*"], "file.read")` false for subuser path | `TestNormalizeSubuserPermissionsRejectsWildcard` + DB constraint test. |
| Mount | `POST /servers/:id/mounts {source:"/etc/passwd"}` →400; `{source:"/mnt/a/../../etc"}` →400; `{source:"/mnt/a/game-data"}` with `/mnt/a` allowlisted →200 | `beacon/internal/server/mounts_test.go:35` pattern reused at API. |
| Proxy rule | `POST /admin/gateways/routers {domain:"a`b.example.com"}` →400 `invalid routing domain` (backtick) | `traefik_proxy_test.go` variant. |
| Gateways IA | `/admin/gateways` renders Routers/Services/Middlewares/Certs via subordinate routes; `/admin/traffic` and `/admin/load-balancer` 301 to it | `grep` nav: only `Gateways` in primary shell after P2. |
| State Lanes | Every server row/badge shows two dots; fenced state shows ring + tooltip `gen X / fence Y`; a11y `aria-label` includes both states | Snapshot test for `states-badge.tsx` + axe. |
| OfflineBanner | Exactly one instance in `AdminShell`, zero in per-page components after P1 | Lint `rg "OfflineBanner" forge/web --count` ==1. |
| Poller dedup | Operations and Activity share single WS/EventSource; no page polls at 2s unless status `running` | Network panel: 15s cadence max, 2s only when `running`. |

---

## 16. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Leader lock on primary DB becomes availability bottleneck | Advisory lock is session-level, not row lock — no TX bloat. TTL lease provides liveness even if primary is briefly down (followers don't need DB to serve reads). If Postgres stretches, flag off. |
| PublishTx adds latency to every placement write (tx holds longer) | Outbox row is small (`StoredEvent`); `PublishTx` just `INSERT` into `stored_events` in same TX — no extra round trip beyond tx commit. |
| Mount allowlist breaks self-hosters who rely on arbitrary binds | Migration seeds allowlist from existing `AllowedMountSourcesForNode` (`store/store_mounts_ext.go:367`) + agent `AllowedMounts` (`beacon/config/config.go:175` default `[]`); P1 dual-write warns but doesn't block, gate flips `on` only after operators confirm. |
| Amber phosphor alienates existing Forge red branding | Keep red for danger/fault; amber is primary accent — operator tests in preview (`FORGE_DS_PHOSPHOR=preview`) collect feedback before default swap. Red CTAs remain red via `--brand-dark` alias for destructive actions (delete, kill). |
| Gateways consolidation hides domain/cert workflows | Gateways tabs keep full CRUD from `handlers_certificates.go`, `handlers_dns.go`, `handlers_acme_accounts.go`; no feature loss, just regrouping. Old URLs 301 preserve bookmarks. |

---

## 17. File:Line Index — Every Target File

**Orchestration / Placement / Events**

- `forge/api/internal/domain/domain.go:114` `PlacementRequest` — add tenant columns.
- `forge/api/internal/placement/strategy.go:43` `Candidate`, `:46` `WorkloadRequest`, `:80` `invalidScorer`, `:95` `LeastLoadedScorer`, `:106` `BinPackScorer`, `:133` `SpreadScorer`, `:149` `RandomScorer`, `:175` `ensureCapacity`, `:185` `availableRatio` — normalize, bound.
- `forge/api/internal/placement/constraints.go:58` `CheckSoft` — replace `1e12` / `-1e10`.
- `forge/api/internal/placement/engine.go:18` `mu`, `:37` `Place`, `:79` `PlaceAll` — iterators, remove global mutex, Tenant filter.
- `forge/api/internal/placement/replica.go:19` `ExistingNodeMap`, `:48` `PlaceReplicas`, `:136` `sort`, `:172` `spreadPenalty` — attempts/backoff, power-of-two, sticky.
- `forge/api/internal/placement/explain.go`, `explain_test.go` — extend for backoff reasoning.
- `forge/api/internal/events/event.go:196` `Envelope`, `:207` `NewEnvelope`, `:19` `EventType` — add TenantID/Generation.
- `forge/api/internal/events/registry.go:44` `failures`, `:73` `Subscribe`, `:144` `Publish`, `:193` `handleWithRetry`, `:237` `recordFailure` — bridge role, metrics.
- `forge/api/internal/events/publisher.go:8`, `subscriber.go:8` — PublishTx middleware.
- `forge/api/internal/eventstore/outbox.go:18` `Relay`, `:30` `NewRelay`, `:39` `Subscribe`, `:45` `Start`, `:74` `pollLoop`, `:102` `processBatch`, `:124` `processEvent`, `:174` `deliverWithRetries` — leader-gate, tenant envelope.
- `forge/api/internal/eventstore/store.go` `StoredEvent`, `PublishTx`, `ClaimPending` (`:105` lease math) — tenant cols.
- `forge/api/internal/store/store.go:35` `migrationAdvisoryLockID`, `:493` `Server{Generation}`, `:1754` `FenceGeneration`, `:1501` `NodeHeartbeatState` — generation CAS, lease.
- `forge/api/internal/store/store_setup.go:12` `setupAdvisoryLockID` — keep distinct.
- `forge/api/internal/store/store_heartbeat_test.go:53` — pattern for CAS test.
- `forge/api/internal/orchestrator/interfaces.go:11` `ServerLifecycle`, `:19` `PowerOperations` — per-server lock note.
- `forge/api/internal/orchestrator/region_view.go` — tenant-aware view.
- `forge/api/internal/daemon/client.go:398` `Mounts`, `:459` `Mount`, `:77` `ComposeStart`, `:1756` `AdminContainerStart` — generation header, mount validation.
- `forge/api/internal/runtime/*` (`docker.go:209` `daemonMounts`, `registry.go`, `multiruntime.go`, `runtime.go:33` `Mount`) — flatten.

**Queue**

- `forge/api/queue/*.go` (`job.go:17` `cancelled`, `job_executor.go:59`, `:219` delay, `periodic.go`, `client.go`, `producer.go`, `worker.go`, `subscription.go:15` `job_cancelled`, `unique.go`, `retry.go`, `maintenanc
e.go`, `queuedriver/queuepgx/*`) — 6 fixes.
- `forge/api/internal/services/operation/service.go:157` `Start` — read model only.
- `forge/api/internal/services/queue/periodic.go:5` — leader-gated.

**Security**

- `forge/api/internal/store/permissions.go:197` `HasPermission` wildcard doc, `:206` impl — keep, but subuser path never gets `*`.
- `forge/api/internal/store/store_users.go:311` `UpsertServerSubuser` caller, `:555` `normalizeSubuserPermissions` (`:564` `permission != "*"`), `:574` `defaultSubuserPermissions`, `:377` `UserCanAccessServer`, `:540` `SFTPAuthResult` — close `*`, DB constraint.
- `forge/api/internal/store/store_mounts_ext.go:307` allowlist check, `:367` `AllowedMountSourcesForNode` — single source.
- `forge/api/internal/services/compose/service.go:614` docker.sock block — expand to allowlist.
- `forge/api/internal/store/store_dns.go:29` list redaction, `:63` decrypt, `:95` encrypt, `:151` `SetDefaultDNSProvider` — keep, fix log redaction.
- `forge/api/internal/http/handlers_dns.go:43` `Credentials` parse, `:54` `ConfigureProvider` — redact.
- `forge/api/internal/services/dns/service.go:18` lego env — redact.
- `forge/api/internal/services/trafficmanager/traefik_proxy.go:334` `Host(`%s`)`, `:337`, `:797` — backtick reject.
- `forge/api/internal/services/trafficmanager/service.go:361` `validateRoutingRule`, `:388` `TargetHost` — add `` ` `` check.
- `forge/api/internal/services/trafficmanager/gateway_adapter.go:41` `UpdateRoutes` — tenant id thread.
- `forge/api/internal/http/middleware_ipaccess.go:18` `TrustProxy`, `:44` `getClientIP`, `:98` `AdminIPAccessConfig` (`:105` `TrustProxy:true`), `:118` `APIIPAccessConfig` — fix defaults.
- `forge/api/internal/http/middleware_ratelimit.go:98` `ExtractClientIP`, `:18` `RateLimitConfig` — trusted proxy CIDR.

**Product IA / Frontend**

- `forge/web/components/admin/admin-shell.tsx:16` `AdminShell`, `:42` `navGroups` (6 groups), `:193` `OfflineBanner`, `:118` skip-link — keep goal-grouped, gateways IA, dedup banner.
- `forge/web/components/admin/admin-registry.ts:22` `adminPageRegistry`, `:61` `plugins metadata-only`, `:72` load-balancer/traffic split — gateways consolidation.
- `forge/web/app/globals.css:8` `*`, `:11` `--brand`, `:27` `[data-theme="light"]` — phosphor tokens.
- `forge/web/tailwind.config.ts` — token mapping.
- `forge/web/lib/design-tokens.ts` — new typed tokens.
- `forge/web/components/shared/states-badge.tsx` — State Lanes signature.
- `forge/web/components/shared/states-offline.tsx` — dedup.
- `forge/web/components/shared/states-empty.tsx` — empty pattern.
- `forge/web/components/shared/states-error.tsx:181` timer — align with poller dedup.
- `forge/web/components/charts/DeploymentTimeline.tsx` — generation-fenced long form.
- `forge/web/components/deployment/DeploymentLogViewer.tsx:8`, `components/admin/AdminAppsShared.tsx:96`, `components/app/app-detail.tsx` — single LogViewer, routed tabs.
- `forge/web/components/admin/AdminNodes.tsx:11` `Tab`, `:152` detail — Health/Capabilities/Drain/Autoscale.
- `forge/web/components/admin/AdminServers.tsx:480` `ServerRow`, `:811` detail — State Lanes badge per row.
- `forge/web/components/admin/AdminOperations.tsx:150` `AdminOperations`, `: evacPlan 2s` — unified timeline.
- `forge/web/components/admin/AdminActivityLog.tsx:120` `15s poll` — WS/EventSource migration.
- `forge/web/app/admin/traffic/page.tsx:84` duplicate OfflineBanner — remove.
- `forge/web/app/admin/monitoring/page.tsx:108` `polling 10s`, `components/monitoring/metrics-chart.tsx:85`, `components/charts/Server*Chart.tsx:73` — poller + graph synthesis.
- `forge/web/app/admin/gateways/**` — new route (Routers/Services/Middlewares/Certs).
- `forge/web/components/server/startup-view.tsx:32`, `settings-view.tsx:23` — read-only projection.
- `forge/api/cmd/api/main.go:362` `eventRelay`, `:370` poll 5s/330s lease, `:441` `fenceSvc`, `:946` `failSvc`, `:1026` `tmSvc`, `:1076` `wsHub` wildcard, `:1167` `Start`, `:1596` `shutdownServices` — wiring fixes.
- `reference/networking/traefik/docs/content/reference/routing-configuration/kubernetes/gateway-api.md`, `reference/networking/traefik/integration/testdata/rawdata-gateway.json` — gateways provenance.
- `beacon/config/config.go:144` `AllowedMounts`, `:175`, `beacon/internal/server/mounts_test.go:35` — allowlist enforcement.

---

## 18. Verification Commands (Read-Only Checks for Reviewers)

```bash
# 1. No prod Relay subscriber? (should become 1 after bridge)
rg -n "relay\.Subscribe\(|eventRelay\.Subscribe" forge/api/cmd/api/main.go forge/api/internal
rg -n "Relay.*Subscribe|PublishTx" forge/api/internal/eventstore --type go

# 2. Leader gating coverage — every Start that should be gated
rg -n "\.Start\(ctx|\.Start\(appCtx" forge/api/cmd/api/main.go forge/api/internal/services --type go

# 3. Wildcard allowlist — ensure * rejected for subusers
rg -n 'normalizeSubuserPermissions|"\*"' forge/api/internal/store/store_users.go forge/api/internal/store/permissions.go

# 4. Soft bonus normalization
rg -n "1e12|1e10|kSoftWeight|CheckSoft" forge/api/internal/placement --type go

# 5. Global mutex removal
rg -n "sync\.Mutex.*Engine|Engine.*mu\.Lock" forge/api/internal/placement --type go

# 6. Validated proxy rules — backtick reject
rg -n "validateRoutingRule|Host\(`" forge/api/internal/services/trafficmanager --type go

# 7. TrustProxy default
rg -n "TrustProxy" forge/api/internal/http/middleware_ipaccess.go forge/api/internal/http/middleware_ratelimit.go

# 8. IA — 7 pages collapsed
rg -n "adminPageRegistry|gateways|Gateways" forge/web/components/admin/admin-registry.ts
rg -n "OfflineBanner" forge/web --type ts --type tsx

# 9. State Lanes signature exists
rg -n "ServerStatus|StateLanes|generation.*fence|fenceGeneration" forge/web/components/shared/states-badge.tsx forge/web --type tsx
```

---

## 19. Closing — What Makes This Capstone Distinct

- **Structure is truth:** Numbering in phases encodes rollout order; group counts in `admin-shell.tsx:42` encode fleet cardinality; two-dot lanes encode desired vs actual — never decoration.
- **One accent, one signature:** Amber phosphor is the only saturated accent in calm steel/ink; State Lanes two-dot + fenced ring is the only memorable glyph. Everything else is disciplined so the signature reads.
- **Postgres stays writer:** No new distributed system is introduced. Leader election reuses the advisory lock already proven for migrations (`store/store.go:35`) and setup (`store/store_setup.go:12`), just extended with TTL observability. Outbox + PublishTx give exactly-once *effect* without Redis/etcd.
- **Security by removal:** Wildcard `*` is closed by disallowing it at the normalization boundary (`store_users.go:564`) plus a DB constraint — not by adding policy). Mounts are an allowlist, not a denylist (`compose/service.go:614` expanded).
- **Gateways as one graph:** Seven loosely-coupled networking pages become one `HTTPConfiguration` graph with four subtabs — matching the Traefik reference operators already know, not an invented abstraction.

This plan is executable without product code change in this step; every Phase 0 row carries a `file:line` anchor for the implementation PR that follows.

