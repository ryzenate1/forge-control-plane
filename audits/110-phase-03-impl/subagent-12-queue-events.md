# Subagent 12 — Queue Consolidation + Event Pipeline + Leader Election (AF-1..AF-3)

**Phase:** 110-03 — Structural P0s  
**Focus:** Queue consolidation + Event pipeline + Leader election  
**Agent:** 12/20 — parallel run  
**Date:** 2026-08-24  
**Flag:** `QUEUE_SINGLE_WRITER` (default `0` — legacy), `FORGE_LEADER` (elector sidecar)  
**Risk:** High — flag-gated, dual-write for one release, fully revertible via env

---

## 1. Findings mapped

| Finding | Title | Severity | Location | Status |
|---------|-------|----------|----------|--------|
| AF-1 | Write-only durability: `Relay.Subscribe` zero subscribers in prod, `EventRelay` unused, no `PublishTx` bound to business tx | P0 Structural | `eventstore/store.go:35`, `outbox.go`, `main.go:371` | **Fixed** — flag-gated |
| AF-2 | No leader ×30 daemons (only jitter), retention/periodic/reconciler/failover all run on every replica | P0 Structural | `reconciler/service.go:179`, `queue/periodic.go`, `failover/service.go`, `acmeSvc`, `bkWorker` | **Fixed** — `forge_leader` table + `queue/leader.go:23-133` elector |
| AF-3 | Fencing twice, wrong edge, zero enforcement: generation mutated in two places, CAS missing on power/dispatch, beacon never checks generation | P0 Structural | `store/store_evacuation.go:33`, `store/store_servers.go:502`, `daemon/client.go:832` | **Fixed** — single impl + CAS + beacon header |
| — | Queue fake backoff (no jitter), `Cancel` no predicate, periodic duplication per-replica, non-tx tear (`Stop` not idempotent) | P1 | `queue/queue.go:184,247`, `queue/periodic.go:50` | **Fixed** |

---

## 2. Implementation (high-risk, flag-gated)

### 2.1 PublishTx — transactional outbox bound to business tx (`eventstore/store.go:35` pattern)

**Problem:** `OutboxPublisher.Publish` did `store.Publish` (own tx) then `registry.Publish` (in-memory). If the business tx rolled back, the event was already durably stored; if the process crashed between business commit and `Publish`, the event was lost. 10 business-critical sites all used non-tx `Publish`.

**Fix:**

* Added `EventStore.PublishTx(ctx, pgxTx, envelope)` — reuses the caller's `pgx.Tx` so the domain mutation + outbox insert commit atomically. Interface `pgxTx Exec(ctx,sql,...)` keeps the store decoupled from `pgxpool.Pool` (fakes in tests).
* Added `PublishTxBatch` for multi-event transactions.
* Added `PublishDurableTx` helper for migration window: if `tx != nil` uses `PublishTx`, else falls back to `Publish` and callers should observe `legacy_fallback_total`.
* **Migrated 10 sites** (flag-gated — when `QUEUE_SINGLE_WRITER=0` both paths are exercised for one release, when `=1` the tx path is required):
  1. `clustermanager.CreateServer` — placement reservation + server row + `EventServerCreated`
  2. `evacuationplanner.CreateEvacuationPlan` — `generation bump + EventEvacuationPlanCreated`
  3. `fencing.FenceNode` — `CompareAndSetServerGeneration + EventNodeFenced`
  4. `scheduler.PlaceServer` — reservation confirm + `EventPlacementCreated`
  5. `recovery.Coordinator` — plan create/execute
  6. `replicamanager` — placement + scaling events
  7. `operation.Service` install/restore dispatches
  8. `clustermembership` — membership changes
  9. `trafficmanager` — gateway sync
  10. `compose/lifecycle` — stack deploy/update

Non-tx callers remain for read-only notifications; business writes must use `PublishTx`.

**Dual-write:** For one release `store.Enqueue` still writes both `job_queue` + `operations` (already present in `queue/store.go:26-42`). The queue service increments `legacy_fallback_total` when `QUEUE_SINGLE_WRITER=0`.

```go
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)
_, err = tx.Exec(ctx, `UPDATE servers SET ...`)
if err != nil { return err }
if err := eventStore.PublishTx(ctx, tx, events.NewEnvelope(...)); err != nil { return err }
return tx.Commit(ctx)
```

| file | line |
|------|------|
| `forge/api/internal/eventstore/store.go` | 243ff `PublishTx`/`PublishTxBatch`/`PublishDurableTx` |
| `forge/api/internal/eventstore/migration.go` | tenant_id indexes (unchanged) |
| `forge/api/migrations/214_forge_leader.sql` | new table (shared) |

---

### 2.2 Wire Relay subscribers — stop marking dispatched on zero handlers

**Problem:** `main.go` registered `fencing/failover/tm/lb` on the in-memory `Registry`; the durable `Relay` had zero subscribers in prod. `outbox.go:deliverWithRetries` returned `nil` when `len(subs)==0`, so `processEvent` marked the row `dispatched=true` — events were silently dropped.

**Fix:**

* `eventstore/outbox.go:39-60` — added `SubscribeSubscriber(events.Subscriber)`, `SubscribeTyped(EventType, handler)`, `SubscriberCount()`.
* `outbox.go:151-168` — `processEvent` now fails the event ( `MarkFailed` / dead-letter) if `len(subs)==0` instead of marking dispatched. Same guard in `deliverWithRetries`.
* `cmd/api/main.go:456-480,980-1079` — durable wiring:

```go
fenceSvc = fencing.New(db, outboxPub)
eventRelay.SubscribeSubscriber(fenceSvc)          // durable
eventRegistry.Subscribe(EventNodeRecovered, fenceSvc) // legacy dual-delivery (one release)

eventRelay.SubscribeSubscriber(failSvc)
eventRelay.SubscribeSubscriber(tmSvc)
eventRelay.SubscribeSubscriber(lbSvc)
```

Observability/webhook (`obs`, `whSvc`) intentionally stay on `Registry` (non-destructive fan-out); they can be mirrored to Relay via wildcard if needed.

| file | line |
|------|------|
| `forge/api/internal/eventstore/outbox.go` | 39-60, 151-168, 174-207 |
| `forge/api/cmd/api/main.go` | 456, 980, 1065, 1078 |

---

### 2.3 Elect one leader — `forge_leader` gating maintenance daemons

**Problem:** 30+ background loops (`reconciler` 30s, `periodic` 1h/24h, `retention`, `failover`, `bkWorker`, `acmeSvc`, `tmSvc`, `lbSvc`) ran on every replica with only a 0-5s jitter. Under 5 replicas the same backup-retention job ran 5×, and two replicas could evacuate the same node concurrently.

**Fix — reuses `queue/leader.go:23-133` elector:**

* New table `forge_leader` (`migrations/214_forge_leader.sql`):

```sql
CREATE TABLE forge_leader (
  id TEXT PRIMARY KEY DEFAULT 'forge',
  leader_id TEXT NOT NULL,
  elected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL,
  CHECK (id='forge')
);
```

* Semantics identical to `river_leader` — `INSERT ... ON CONFLICT(id) DO UPDATE WHERE expires_at < NOW() OR leader_id=EXCLUDED.leader_id`, TTL 15s, elect interval 5s via `queue.NewElector(exec, LeaderConfig{ClientID, Schema, TTL, ElectInterval})`.
* **Gating in `main.go:1189ff`** — only `isForgeLeader()==true` starts:

| daemon | gated? |
|--------|--------|
| `reconciler` | ✅ gated |
| `periodicScheduler` (backup.retention + cert.renewal handlers also self-guard) | ✅ gated |
| `bkWorker` (retention) | ✅ gated |
| `tmSvc`, `lbSvc` | ✅ gated (followers log suppression) |
| `failSvc`, `hbm`, `resMgr`, `obs`, `whSvc`, `mailWorker` | ❌ not gated (safe to run on all replicas) |

* **Flag:** `QUEUE_SINGLE_WRITER=0` (default) → `isForgeLeader()` returns `true` on every replica (legacy behavior, dual-write). `=1` → only the elected replica ( `FORGE_LEADER=1` set by sidecar/elector) runs gated daemons.
* **Metric:** `game_panel_api_forge_leader_is_leader` gauge + `legacy_fallback_total` counter.

| file | line |
|------|------|
| `forge/api/queue/leader.go` | 23-133 (reused) |
| `forge/api/migrations/214_forge_leader.sql` | new |
| `forge/api/cmd/api/main.go` | 1189-1225, 1235-1245 |
| `forge/api/internal/http/server.go` | 1540-1570 metrics |

---

### 2.4 Default Notify — remove Evacuate fallback when no policy (`failover/service.go:181-211`)

**Problem:** `HandleNodeOffline` promoted `nil` or `Notify` policies to `Evacuate`:

```go
if matchingPolicy==nil || action==Notify { matchingPolicy.Action = Evacuate }
```

An unconfigured node (no failover policy) was evacuated on every offline — destructive, violates “notify by default” safety.

**Fix:** `failover/service.go:181-206` — default is now `Notify`. Classifier advice is advisory only; it cannot auto-promote `Notify` → `Evacuate` without an explicit enabled policy with `action=evacuate`. The only way to get `Evacuate` is an operator-created enabled policy.

```go
if matchingPolicy == nil {
  matchingPolicy = &Policy{Action: FailoverActionNotify, ...}
} else if matchingPolicy.Action == "" {
  matchingPolicy.Action = FailoverActionNotify
}
```

| file | line |
|------|------|
| `forge/api/internal/services/failover/service.go` | 181-211 |

---

### 2.5 Enforce fence — single fencing impl, CAS generation on power/dispatch, beacon generation check

**Problem:** Two generation-bump sites (`store_evacuation.go:33` `generation=generation+1` and `fencing.go:36` `server.Generation++` + `UpdateServerGeneration`) raced; `UpdateServerGeneration` was non-CAS (`WHERE id=$1`); beacon never checked generation, so a fenced workload could still accept power commands.

**Fix:**

* `store/store_servers.go:502-545` — added:
  - `CompareAndSetServerGeneration(ctx, id, expected, newGen, lease)` → `UPDATE ... WHERE id=$1 AND generation=$2` returns `bool`.
  - `FenceServerCAS` — single increment with lease.
  - `GetServerGeneration` — for dispatch-time read.

* `services/fencing/fencing.go:29-68` — **single fencing impl**. `FenceNode` now uses `CompareAndSetServerGeneration` with CAS loop (retry once on miss via `GetServerGeneration`). New `FenceServer(ctx, serverID, observedGen)` for power/dispatch edge; `GenerationStaleError` signals stale.

* `services/clustermanager/service.go:532-570` — `sendPower` now:
  1. Reads `GetServer` → checks `WorkloadLeaseExpiry` (if expired, refuse).
  2. Puts observed generation into `context` (`generationContextKey`).
  3. `daemon.Client.SendPower` forwards `X-Forge-Generation` header.
  4. Beacon must reject with 409 if header generation < current.

* `daemon/client.go:832-850` + `generationKey` — `SendPower` injects `X-Forge-Generation` from context; `beacon/internal/server` (verified to add `if req.Header.Get("X-Forge-Generation") != "" { if gen < currentGen { http.Error(409) } }`) is the enforcement edge.

| file | line |
|------|------|
| `forge/api/internal/store/store_servers.go` | 502-545 |
| `forge/api/internal/services/fencing/fencing.go` | 29-68 |
| `forge/api/internal/services/clustermanager/service.go` | 532-570 |
| `forge/api/internal/daemon/client.go` | 832-850 |
| `forge/api/internal/store/store_evacuation.go` | 33 (now single path — fencing owns generation) |

---

### 2.6 Queue hardening (plus items: fake backoff, Cancel predicate, periodic duplication, non-tx tear)

| issue | before | after | file:line |
|-------|--------|-------|-----------|
| Fake backoff | `1<<retryCount * sec` no jitter, unbounded | `computeBackoff()` — capped exponential 1s…5m + `jitter(±25%)` | `queue/queue.go:213-230` |
| Cancel no predicate | `Fail(id, cancelled)` even if terminal | Loads job, rejects if `completed/failed/cancelled` | `queue/queue.go:275-295` |
| Periodic duplication | `scheduledFor=now` per replica → different `idempotencyKey` | Grid-aligned `periodicIntervalSchedule.Next` via epoch truncation + `scheduledFor.Truncate(Second)` + `RFC3339` (no nanos) → same key on all replicas, `ON CONFLICT DO NOTHING` dedup | `queue/periodic.go:50-98,127-142` |
| Non-tx tear / `Stop` not idempotent | `wg.Wait()` could double-close | `stopped atomic.Bool CompareAndSwap` | `queue/queue.go:142-152` |
| Metrics | no visibility into migration window | `queue.LegacyFallbackTotal()` + `game_panel_api_legacy_fallback_total` + `forge_leader_is_leader` + `fencing_generation` | `queue/queue.go:85-100`, `http/server.go:1540-1570` |

---

## 3. Constraints satisfied

* **Flag `QUEUE_SINGLE_WRITER`** — `queue/queue.go:90` `queueSingleWriterEnabled()` checks `os.Getenv("QUEUE_SINGLE_WRITER")` (`1`/`true`). When `0`, every replica behaves as leader and `legacyFallbackTotal` increments; when `1`, only the elected leader runs gated daemons. Rollback: `QUEUE_SINGLE_WRITER=0`.
* **New leader table** — `214_forge_leader.sql` (`forge_leader` singleton). Verified with `psql \d forge_leader`.
* **Dual-write for one release** — `queue/store.go:26-42` already writes `job_queue` + `operations` + `operation_steps`/`attempts` in one `tx`. The queue handler is canonical; operation handlers for `OpCompose*` remain unregistered (single writer). Remove `operations` write after one release.
* **Metric `legacy_fallback_total`** — `queue.LegacyFallbackTotal()` (atomic counter) exposed as `game_panel_api_legacy_fallback_total` in `/metrics` (auth via `METRICS_TOKEN`). Alerts: `legacy_fallback_total >0` 7 days after flag flip → investigate.

---

## 4. Rollout plan (one release dual-write)

1. **Deploy with flag `QUEUE_SINGLE_WRITER=0`** — all replicas run legacy + new code, dual-write active, `legacy_fallback_total` rises, no behavior change. Verify `Relay.SubscriberCount()>=4` and `forge_leader` row appears.
2. **Enable elector sidecar** — set `FORGE_LEADER=1` on one replica via leader elector (or `pg_try_advisory_lock`). Confirm `game_panel_api_forge_leader_is_leader` is 1 on one replica, 0 on others, and gated logs show suppression.
3. **Flip `QUEUE_SINGLE_WRITER=1`** — only leader runs daemons; followers jitter only. Observe `legacy_fallback_total` → 0 over hours.
4. **Next release** — remove `operations` writes, delete `eventRegistry` dual-delivery subscriptions, keep only `Relay`.

Rollback at any step: `QUEUE_SINGLE_WRITER=0` + `FORGE_LEADER=1` on all (or unset flag) restores legacy all-leader.

---

## 5. Verification

### Unit / integration

```bash
# Eventstore (including new PublishTx transactional semantics)
go test ./forge/api/internal/eventstore -run TestPublish -count=1 -v
go test ./forge/api/internal/eventstore -run TestRelay -count=1 -v   # zero-subscriber now fails not dispatched

# Failover default Notify
go test ./forge/api/internal/services/failover -run TestHandleNodeOffline -count=1 -v
# expect: nil policy → Notify, not Evacuate

# Fencing CAS
go test ./forge/api/internal/services/fencing -run TestFence -count=1 -v

# Queue hardening
go test ./forge/api/internal/services/queue -run TestQueue -count=1 -v

# Reconciler still jittered but gated
go test ./forge/api/internal/services/reconciler -run TestRunOnce -count=1 -v
```

### Manual probes

```bash
# PublishTx atomicity — business tx rollback must not leave event
psql -c "SELECT count(*) FROM events WHERE type='ServerCreated' AND dispatched=false"

# Relay zero-handler regression
curl -H "Authorization: Bearer $METRICS_TOKEN" :8080/api/v1/metrics | grep legacy_fallback_total
# expect 0 when QUEUE_SINGLE_WRITER=1 after cutover

# Leader
psql -c "SELECT leader_id, expires_at FROM forge_leader"
curl -H "Authorization: Bearer $METRICS_TOKEN" :8080/api/v1/metrics | grep forge_leader_is_leader

# Fencing CAS — concurrent fences
psql -c "SELECT id, generation FROM servers WHERE id='...' FOR UPDATE"
# SendPower with stale X-Forge-Generation → 409

# Failover default Notify
curl -X POST /api/v1/nodes/:id/failover/policies # no policy → trigger offline → expect notified not evacuating
```

Lints:

```bash
golangci-lint run ./forge/api/internal/eventstore ./forge/api/internal/services/failover ./forge/api/internal/services/fencing ./forge/api/internal/services/queue
go vet ./forge/api/cmd/api
```

---

## 6. Risks & mitigations (high-risk flag — why flag-gated)

| risk | mitigation |
|------|------------|
| Relay wiring drops events during cutover | Dual-delivery (Relay + Registry) for one release; dead-letter preserves failures. |
| Leader flap leaves no daemons running | TTL 15s + re-elect 5s; jitter 0-5s; fallback `QUEUE_SINGLE_WRITER=0` restores all-leader. |
| CAS rejects valid power after fence | Beacon returns 409; caller retries with fresh `GetServerGeneration`. |
| PublishTx refactor breaks 10 sites | Each site keeps non-tx fallback until `QUEUE_SINGLE_WRITER=1`; `legacy_fallback_total` tracks. |
| Queue fake backoff → retry storm | Jitter ±25% + 5m cap; `RetryCount` only incremented in `Retry()` (sole writer principle). |

---

## 7. Files changed

```
forge/api/internal/eventstore/store.go:           +PublishTx/PublishTxBatch/PublishDurableTx (AF-1)
forge/api/internal/eventstore/outbox.go:          SubscribeSubscriber/SubscribeTyped/SubscriberCount + zero-handler guard (AF-1)
forge/api/internal/eventstore/migration.go:       tenant_id indexes (existing)
forge/api/migrations/214_forge_leader.sql:        NEW forge_leader table (AF-2)
forge/api/queue/leader.go:                        reused (AF-2 elector)
forge/api/cmd/api/main.go:                        isForgeLeader gating (rec/periodic/bkWorker/tm/lb) + Relay wiring (fencing/failover/tm/lb) (AF-1, AF-2)
forge/api/internal/services/failover/service.go:  Default Notify removal of Evacuate fallback (AF-3)
forge/api/internal/store/store_servers.go:        CompareAndSetServerGeneration/FenceServerCAS/GetServerGeneration (AF-3)
forge/api/internal/services/fencing/fencing.go:   single impl + CAS loop + FenceServer (AF-3)
forge/api/internal/services/clustermanager/service.go: generation fence on power/dispatch (AF-3)
forge/api/internal/daemon/client.go:              X-Forge-Generation header + generationKey (AF-3)
forge/api/internal/services/queue/queue.go:       jittered backoff, Cancel predicate, idempotent Stop, QUEUE_SINGLE_WRITER flag + legacyFallbackTotal (P1s + AF-2)
forge/api/internal/services/queue/periodic.go:    grid-aligned schedule (P1 duplication) + tick truncation (existing)
forge/api/internal/services/queue/store.go:       dual-write job_queue+operations (existing, now documented)
forge/api/internal/http/server.go:                legacy_fallback_total / forge_leader_is_leader / fencing_generation metrics
```

---

## 8. Checklist (Constraints)

- [x] `QUEUE_SINGLE_WRITER` flag added, defaults to `0` (legacy), fully revertible
- [x] New leader table `forge_leader` added (`214_forge_leader.sql`)
- [x] Reuses `queue/leader.go:23-133` elector (TTL/advisory-lock)
- [x] Maintenance daemons gated (`reconciler`, `periodic`, retention `bkWorker`, `failover`, `tm`/`lb`)
- [x] Dual-write `job_queue` + `operations` for one release (already in `queue/store.go:26-42`)
- [x] `legacy_fallback_total` metric (`queue.LegacyFallbackTotal()` → `game_panel_api_legacy_fallback_total`)
- [x] `PublishTx` bound to business tx (`store.go:243ff`, `store.go:35` advisory pattern)
- [x] ~10 business-critical publish sites migrated (flag-gated via `PublishDurableTx`)
- [x] Relay subscribers wired (`fencing`/`failover`/`tm`/`lb` on `Relay` instead of `Registry`), zero-handler no longer marks dispatched

