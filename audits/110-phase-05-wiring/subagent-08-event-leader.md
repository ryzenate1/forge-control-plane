# Subagent 08 — Event Pipeline + Leader Election + Orphan Center Wiring (Phase 05 Wiring)

**Slice:** Wire `eventstore` outbox + `queue/leader.go` elector + orphan remediation center
**Re-verifies:** AF-1 (zero prod subs, EventRelay unused, no PublishTx), AF-2 (unwired elector), ~30 Start() daemons
**Status:** Partial wiring verified — 4 Relay subscribers OK, zero-handler guard OK, leader gate stub present, orphan center OK. Two leader-gated daemons + dual-read metrics wired in this slice. PublishTx still has zero production callers (P0 gap). Elector remains ENV stub (P0 gap).

---

## 1. Task Checklist

| Required wiring | Expected state | Actual state (post-check) | Verdict |
|---|---|---|---|
| `PublishTx` used ≥10 critical paths (03-12 claim) | `EventStore.PublishTx(ctx, tx, env)` at 10+ sites | `forge/api/internal/eventstore/store.go:267` `PublishTx` + `store.go:314` `PublishDurableTx` exist but `grep -r PublishTx forge --include=*.go` = **11 hits only definition/comments** — zero prod callers | **FAIL — gap** |
| Relay subscribers: fencing/failover/tm/lb on Relay | `eventRelay.SubscribeSubscriber(fenceSvc/failSvc/tmSvc/lbSvc)` + dual Registry | `forge/api/cmd/api/main.go:461` fenceSvc, `982` failSvc, `1082` tmSvc, `1096` lbSvc → 4 prod subscribers | **PASS** |
| `forge_leader` election gating maintenance daemons | `isForgeLeader()` checks `queue.NewElector(...).IsLeader()` against `forge_leader` table | `forge/api/cmd/api/main.go:1220` `isForgeLeader` exists but ENV stub: `QUEUE_SINGLE_WRITER`+`FORGE_LEADER` env, comment notes `// Production leader check would use pg_try_advisory_lock` | **PARTIAL — stub** |
| Gated daemons via `isForgeLeader()` | reconciler + retention + failover + periodic gated | Before this slice: `rec:1235`, `bkWorker:1248`, `periodicScheduler:1290` gated; `failSvc:1244`, `cleanupSvc:1072` **ungated**. After this slice: `failSvc` gated, `cleanupSvc` deferred-gated | **FIXED in slice (2 daemons)** |
| Orphan center wiring | `/admin/orphan-remediations` + store + lifecycle | `handlers_orphan_remediations.go:13` route, `server.go:2635` register, `store_orphan_remediations.go:56` List/Resolve, `store_servers_lifecycle.go:37` insert | **PASS** |
| Dual-read metrics | `legacy_fallback_total`, `forge_leader_is_leader`, `fencing_generation` + relay vs registry | Before: 3 gauges at `server.go:1558-1591`. After: + `event_relay_subscribers`, `event_registry_published_total`, `event_store_pending/dispatched/dead_letter` | **FIXED in slice** |
| ~30 Start() daemons inventoried | ≤30 starts gated appropriately | 28 Start() counted — see §6 | **PASS with gaps noted** |

---

## 2. Event Pipeline — `eventstore/outbox.go:39` & `store.go:267`

### 2.1 Relay primitive (sound)

- `forge/api/internal/eventstore/outbox.go:18` `Relay{store, pollInterval, subscribers, maxRetries:3, eventTimeout:30s}` — `NewRelay` at `forge/api/cmd/api/main.go:387` `eventstore.NewRelay(es, 5*time.Second)` with comment `// 5s poll, 330s lease (via (batchSize+1)*eventTimeout=11*30s)` `main.go:1253`.
- `outbox.go:39` `Subscribe`, `48` `SubscribeSubscriber(events.Subscriber)`, `61` `SubscribeTyped(EventType, handler)`, `71` `SubscriberCount()`. All three are production-ready adapters.
- `outbox.go:77` `Start`, `91` `Stop`, `106` `pollLoop` with `Ticker(5s)` + `retentionTicker(1h)` pruning `Prune(7d,30d)` at `126`.
- `outbox.go:134` `processBatch`: `ClaimPending(limit=10, claimToken=uuid, lease=330s)` at `137` — lease = `(10+1)*30s` ensures crashed relay reclaims.
- `outbox.go:192` **zero-handler guard** (AF-1 fix):
  ```go
  if len(subs)==0 { err:=fmt.Errorf("relay has zero subscribers... AF-1");
    if markErr:=r.markFailedOrDeadLetter(...); ... }
  // deliverWithRetries second guard at 221, markFailedOrDeadLetter 262
  ```
  Prevents silent `MarkDispatched` with zero delivery. `store.go:266` + `maxFailureCount=5` + `MoveToDeadLetter` after 5.
- `forge/api/internal/eventstore/store.go:68` `ClaimPending` uses `FOR UPDATE SKIP LOCKED`, `claimed_until < NOW()` reclaim, lease via `claimed_by + claimed_until`.

### 2.2 Publish paths

- `store.go:114` `Publish(ctx, Envelope)` — non-tx pool insert.
- `store.go:211` `OutboxPublisher{store, registry}` — `Publish` does `store.Publish` then `registry.Publish`. **Not tx-bound**: business row commits, then outbox insert, then in-memory publish — crash between commits loses causality.
- `store.go:267` `PublishTx(ctx, pgxTx, Envelope)` — tx-bound insert, requires caller's `pgx.Tx`. Validates `tx!=nil`, marshals payload, inserts with `tenant_id` null handling. `PublishTxBatch:297` and `PublishDurableTx:314` (flag-gated helper: `if tx!=nil PublishTx else Publish` + counts `legacy_fallback_total`).
- **Verification:** `grep -rn PublishTx forge --include=*.go` = 11 hits — 8 definition/comment/doc, 3 in `store.go` itself. **Zero production callers.** The `store.go:313` comment claims 10 migrated sites (`clustermanager.CreateServer, evacuation planner, fencing.FenceNode, scheduler.PlaceServer, recovery.Coordinator, replicamanager, operation dispatch, cluster membership, traffic manager, compose lifecycle`) — **disproved by grep**. All current publishers remain `publisher.Publish` (outbox non-tx) or `eventRegistry.Publish`:
  - `heartbeatmonitor:353` `Publish(EventActualStateChanged)`
  - `reservations:177` `Publish(EventReservationCreated)`
  - `scheduler:274` `Publish(EventNodeCapacityExceeded)`
  - `failover:304,493,506,519` `Publish(failover_policy_created / evacuation / restart / notified)`
  - `loadbalancer:233` `Publish(target_group_created)`
  - `clustermanager:629`, `evacuationplanner:808`, `migration:879`, `deployment:*`, `compose:460,621,683`, `trafficmanager:810,891`, `replicamanager:871`, `clustermembership:366`, `crossnode/ingress_sync:201`, `runtimesvc:158`, `cleanup:117`, `reconciler:542`, `fencing:61` (via `FenceNode` publisher), `enhancednotif` etc.
  - `cmd/api/main.go:612,625,703,986,998` `outboxPub.Publish(...)` — all non-tx.
- **Impact:** AF-1 half-fixed (Relay guards against zero-handler dispatch; `PublishTx` primitive built) but **durability guarantee does not exist in production** — every domain mutation can commit without its event, and events can publish without their causal state. Cross-instance automation impossible on crash.

### 2.3 Relay subscribers — verified wired

- `main.go:457` `fenceSvc = fencing.New(db, outboxPub)` → `461` `eventRelay.SubscribeSubscriber(fenceSvc)` + `462` `eventRegistry.Subscribe(EventNodeRecovered, fenceSvc)` — **dual delivery** comment `AF-1: durable fencing via Relay, not in-memory Registry`.
- `failSvc` at `895` `failover.New(db, outboxPub)` → `982` `eventRelay.SubscribeSubscriber(failSvc)` + `983` `eventRegistry.Subscribe(EventNodeOffline, failSvc)` — same dual pattern.
- `tmSvc` at `1081` `trafficmanager.NewWithPersistence(..., outboxPub)` → `1082` `eventRelay.SubscribeSubscriber(tmSvc)` + `1083-1084` Registry `EventNodeOffline/Recovered`.
- `lbSvc` at `855` `loadbalancer.New(db,outboxPub)` → `1096` `eventRelay.SubscribeSubscriber(lbSvc)` + `1097-1099` Registry `EventNodeOffline/Recovered/Online`.
- Total **4 prod subscribers** on Relay. `grep -n eventRelay.SubscribeSubscriber forge/api/cmd/api/main.go` confirms exactly those 4.
- Other consumers remain **Registry-only** (intentional per comments — non-destructive): `obs:1137` `WildcardEventType`, `whSvc:1138` `Wildcard`, `enhancedNotifSvc:1336-1343` (`EventServerCrashed/InstallCompleted/BackupCreated/...`), `crossnode resolver clear` handlers at `1099,1108,1117`, `reconciler` via `EventDesiredStateChanged` publish (not subscribe). These are deliberately not Relay-leader-gated until one-release dual-read window closes — documented at `main.go:458-462,980-983`.
- `Relay.Start(appCtx)` at `1255` after `isForgeLeader` block, with lease verification comment.

### 2.4 Dual-read metrics — fixed in this slice

Before: `server.go:1558-1591` only 3 gauges — `legacy_fallback_total` (always 0 placeholder `v uint64`), `forge_leader_is_leader` (env-based), `fencing_generation` (MAX generation).

After (this slice `server.go:1592-1630`):

```go
game_panel_api_event_relay_subscribers gauge // cfg.EventRelay.SubscriberCount() expected ≥4
game_panel_api_event_registry_published_total counter // cfg.EventRegistry.Metrics().EventsPublishedTotal
game_panel_api_event_store_pending gauge // SELECT COUNT(*) FROM events WHERE dispatched=false
game_panel_api_event_store_dispatched gauge // WHERE dispatched=true
game_panel_api_event_store_dead_letter gauge // COUNT(*) FROM events_dead_letter
```

Zero-relay-subscribers with pending >0 now surfaces as alert. `Pending` growth + `dead_letter` growth distinguishes AF-1 mis-wiring (events claimed then DLQ) from normal drain.

---

## 3. Leader Election — `queue/leader.go:23` & `migrations/214_forge_leader.sql`

### 3.1 The unwired elector (still)

- `forge/api/queue/leader.go:18` `Elector{exec Executor, config *LeaderConfig{ClientID, Schema, ElectInterval:5s, TTL:15s}, isLeader atomic.Bool, leaderCh, stopCh, doneCh, logger}`.
- `leader.go:29` `NewElector`, `46` `Start(ctx)` spawns goroutine looping `LeaderAttemptElect` → `runLeaderLoop` ticker `ElectInterval` → `LeaderAttemptReelect` with `timeNowPtr()`, `notify(true/false)`, `resign` on `stopCh`.
- `migrations/214_forge_leader.sql:6` `CREATE TABLE forge_leader(id PK 'forge', leader_id, elected_at, expires_at, CHECK id='forge')` + `idx_forge_leader_expires`.
- **Dead-fork reference:** `forge/api/queue/queuedriver/queuepgx/queue_pgx_driver.go:401` `LeaderAttemptElect` SQL targets `river_leader` table (River fork), not `forge_leader`. The `queue` elector against `forge_leader` was never wired to a `forge_leader`-aware executor — the only existing executor writes `river_leader`. New table requires new SQL or `pg_try_advisory_lock` path noted in comment.
- **Production references:** `grep -rn NewElector forge/api --include=*.go` = only `queue/leader.go:29` definition, one comment in `main.go:1224` `// via queue.NewElector(exec, cfg).IsLeader()`. Zero instantiations in `forge/api/internal` or `cmd/api/main.go`. Confirms `audits/reverification/subagent-18-queue-events.md:192` and `audits/phase-05/subagent-02-river-queue.md`.

### 3.2 `isForgeLeader` stub

`main.go:1220`:

```go
isForgeLeader := func() bool {
  if os.Getenv("QUEUE_SINGLE_WRITER")!="1" && os.Getenv("QUEUE_SINGLE_WRITER")!="true" { return true } // legacy fallback
  return os.Getenv("FORGE_LEADER")=="1" // elector sidecar sets env
}
```

- Flag `QUEUE_SINGLE_WRITER` gates rollout: 0 → every replica is leader (legacy, counts toward `legacy_fallback_total`); 1 → `FORGE_LEADER=1` only leader. `main.go:1219` comment and `store.go:309` `PublishDurableTx` doc agree.
- True leader check would be `pg_try_advisory_lock(hashtext('forge_leader'))` or `queue.NewElector(exec, cfg).IsLeader()` against `forge_leader` — **not yet implemented**. `FORGE_LEADER` env requires external sidecar/elector process to set it; no sidecar shipped in `infra/` or `ops/`.
- `http/server.go:1569-1580` `forge_leader_is_leader` gauge mirrors same env logic.

### 3.3 Daemon gating — this slice remediation

**Inventory 28 Start() per instance (`main.go`):**

| # | Daemon | Line | Before | After | Correct? |
|---|---|---|---|---|---|
| 1 | `gitOpsController.Start` | 542 | always | always | OK (per-repo GitOps, needs per-instance? debatable — keep always) |
| 2 | `queueSvc.Start` | 560 | always | always | OK (SKIP LOCKED work queue — all replicas may claim) |
| 3 | `opSvc.Start` | 821 | always | always | OK (same, but dual-writer; keep always until consolidation) |
| 4 | `healthCheckRunner.Start` | 893 | always | always | OK (per-node health, needs all replicas to observe) |
| 5 | `discoverySvc.Start` | 1046 | always | always | OK (service discovery, eventual) |
| 6 | `ingressSync.Start` | 1051 | always | always | OK (Caddy sync via db poll) |
| 7 | `cleanupSvc.Start` | 1072→**deferred** | **always** | **leader-gated** | **FIXED this slice** — moved from `New` site to post-`isForgeLeader` block |
| 8 | `tmSvc.Start` | 1085 | always | always | OK (event-driven, Relay ensures dedup) |
| 9 | `lbSvc.Start` | 1130 | always | always | OK (same) |
| 10 | `buildSvc.Start` | 1167 | always | always | OK (build recovery) |
| 11 | `buildpackSvc.Start` | 1170 | always | always | OK |
| 12 | `cronJobSvc.Start` | 1208 | always | always | Should be leader-gated? Cron uses DB idempotency but still duplicates on multi-replica; kept always for now — note P1 |
| 13 | `resMgr.Start` | 1211 | always | always | OK (lease expiry needs all?) |
| 14 | `hbm.Start` | 1212 | always | always | OK (heartbeat classification from API poll) |
| 15 | `rec.Start` | 1235 | **gated** | gated | OK |
| 16 | `mig.Start` | 1240 | always | always | OK (migration executor, on-demand) |
| 17 | `ep.Start` | 1241 | always | always | OK (on-demand) |
| 18 | `mailWorker.Start` | 1242 | always | always | OK (mail queue, SKIP LOCKED) |
| 19 | `whSvc.Start` | 1243 | always | always | OK (webhook retries) |
| 20 | `failSvc.Start` | 1244 | **always** | **gated** | **FIXED this slice** |
| 21 | `bkWorker.Start` | 1248 | **gated** | gated | OK |
| 22 | `eventRelay.Start` | 1255 | always | always | OK (all replicas may poll Relay; CLAIM SKIP LOCKED ensures single delivery, but retention prune duplicates — acceptable) |
| 23 | `periodicScheduler.Start` | 1290 | **gated** | gated | OK |
| 24 | `procedureSvc.Start` | 1296 | always | always | OK |
| 25 | `replicaMgr.Start` | 1297 | always | always | OK |
| 26 | `autoSvc.Start` | 1298 | always | always | OK (autoscaler, on-demand) |
| 27 | `enhancedNotifSvc.Start` | 1335 | always | always | OK |
| 28 | `pipelineSvc.Start` | 1404 | always | always | OK |

Remediation in this slice: `failSvc` and `cleanupSvc` now leader-gated, matching `214_forge_leader.sql:22` intent (`reconciler, periodic scheduler, retention pruners, and failover timers`). `server.go` metrics now distinguish leader vs follower via `forge_leader_is_leader`.

**Remaining not-yet-gated but flagged P1:** `cronJobSvc` (periodic cron still per-replica beyond the 2 durable periodic jobs — `queue/periodic.go:50` `PeriodicInterval` with `RFC3339Nano` dedup key diverges per replica, noted in `reverification/subagent-18:103-115`), `hbm` reaper, `replicaMgr` stale lease reaper. Recommend extending `isForgeLeader` gate to `cronJobSvc` in next slice.

**Elector proper fix remains P0:** instantiate `queue.NewElector(exec, &queue.LeaderConfig{ClientID:hostname, Schema:"public", ElectInterval:5s, TTL:15s})` against `forge_leader` (requires new executor SQL or `pg_advisory_xact_lock` path). Until then, multi-replica requires external `FORGE_LEADER=1` env orchestration.

---

## 4. Orphan Center Wiring — Verified No Gap

- `migrations/040_truthful_server_lifecycle.sql:20` `server_orphan_remediations(id, server_id, node_url, daemon_error, status pending, created_at)` + index `pending_idx`.
- `migrations/042_database_provisioning_security.sql:32` `database_orphan_remediations(id, server_database_id, server_id, database_host_id, engine, host, port, database_name, username, remote, reason, status, created_at)`.
- `migrations/145_preserve_database_orphan_remediations.sql:1` drops orphan FKs for retention.
- `forge/api/internal/store/store_orphan_remediations.go:56` `ListServerOrphanRemediations(status)`, `82` `ListDatabaseOrphanRemediations`, `114` `ResolveServerOrphanRemediation` (CAS `status=pending` → `resolved`, audit insert), `143` `ResolveDatabaseOrphanRemediation`.
- `forge/api/internal/store/store_servers_lifecycle.go:37` `INSERT INTO server_orphan_remediations` on `force=true` delete when `daemon Delete` fails; `store_databases.go:482` equivalent for database hosts.
- `forge/api/internal/http/handlers_orphan_remediations.go:13` `registerOrphanRemediationRoutes(protected.Group("/admin/orphan-remediations", requireRole("admin")))`:
  - `GET /` `requireAdminScope("servers.read","databases.read")` → lists both tables filtered by `status=pending|resolved` via `orphanRemediationStatusFromRequest:49`.
  - `POST /servers/:id/resolve` `requireAdminScope("servers.delete")`, `POST /databases/:id/resolve` `requireAdminScope("databases.delete")`, both via `remediationActorID(c)` + `remediationResolutionError` handling `ErrOrphanRemediationNotFound/Resolved` → `404/409`.
- `forge/api/internal/http/server.go:2635` `registerOrphanRemediationRoutes(protected, cfg, mutationLimiter, adminIPAccess)` — wired.
- Tests: `handlers_orphan_remediations_test.go:14` `TestOrphanRemediationStatusFromRequest`, `:45` `TestOrphanRemediationRoutesRequireAdmin`, `:58` `TestOrphanRemediationRoutesRequireResourceScopes`, `store_servers_lifecycle_integration_test.go:65,106,231` counts audit.
- **Verdict:** No gap. Force-delete audit path → orphan row → admin center → scoped resolve is end-to-end. No wiring added in this slice.

---

## 5. PublishTx Critical Paths — Gap Detail & Recommended 10 Sites

`store.go:307-313` claims migrated sites but none wired. Critical 10 that must use `PublishTx(ctx, tx, ...)` (business tx holds row lock):

| # | Site | Current non-tx publish | Required tx boundary |
|---|---|---|---|
| 1 | `clustermanager.CreateServer` | `store.go:629` `publisher.Publish(ServerCreated)` after `INSERT servers` | `BEGIN; INSERT servers; PublishTx(tx, ServerCreated); COMMIT` |
| 2 | `clustermanager.DeleteServer` (orphan) | `handlers_servers.go:1920` insert orphan after failed daemon | same tx as `DELETE servers` or orphan insert |
| 3 | `fencing.FenceNode` | `fencing.go:61` `Publish(EventNodeFenced)` after looped `CAS generation++` | tx wrapping generation bumps + fenced event |
| 4 | `scheduler.PlaceServer` | `scheduler/service.go:274` `Publish(NodeCapacityExceeded)` after placement row | placement reservation tx |
| 5 | `recovery.Coordinator` create/plan | `recovery/service.go:642-650` inline fencing duplicate + event | use `fencing.FenceNode` + tx |
| 6 | `replicamanager` instance lifecycle | `replicamanager:871` `Publish(EventInstance*)` | instance row tx |
| 7 | `operation.Service` dispatch (`install/restore/file*`) | `main.go:612,625,703` `outboxPub.Publish` outside tx | caller tx that inserts `operations` row |
| 8 | `clustermembership` join/leave | `clustermembership:366` | membership row tx |
| 9 | `trafficmanager` gateway route sync | `trafficmanager:810,891,1042` | gateway persistence tx |
| 10 | `compose lifecycle` deploy/update/delete | `compose/lifecycle.go:460,621,683` | stack state tx |

Pattern per `store.go:265` doc: `tx,_:=pool.Begin(ctx); defer tx.Rollback(ctx); Exec business; PublishTx(ctx, tx, NewEnvelope(...)); tx.Commit(ctx)`. Followed by async `registry.Publish` via Relay subscription (do not double-publish synchronously).

Only `Migration` guidance remains to reach one-release dual-write: gate `PublishDurableTx` on `QUEUE_SINGLE_WRITER` (already at `store.go:314`), then flip flag and remove `OutboxPublisher.Publish` dual path after Relay proves stable (one release per `main.go:458` comment).

---

## 6. Remaining Wiring & Activation Order

| Priority | Item | Action | Owner/file |
|---|---|---|---|
| **P0** | Wire `PublishTx` at 10 sites above | Migrate each `publisher.Publish` to `store.PublishTx(tx, ...)` inside business tx; add `PublishTxBatch` where multi-event | `internal/eventstore/store.go:267`, each service |
| **P0** | Instantiate real elector | `queue.NewElector(forgeLeaderExecutor, &LeaderConfig{ClientID:os.Hostname(), Schema:"public"})` + `elector.Start(appCtx)` + `isForgeLeader:=elector.IsLeader` + remove `FORGE_LEADER` env fallback | `cmd/api/main.go:1220`, new `internal/leader/forge_leader.go` |
| **P0** | Gate `cronJobSvc` on leader | `if isForgeLeader(){cronJobSvc.Start(appCtx)}` (avoid duplicate cron fires) | `main.go:1208` |
| P1 | EventRegistry dual-read removal | After one release, remove `eventRegistry.Subscribe(...fenceSvc/failSvc/tmSvc/lbSvc)` fallback leaves Relay as source of truth | `main.go:462,983,1083,1097` |
| P1 | `avi available_at` for operation retry | Add `available_at` column + `Retry` sets `available_at = NOW()+backoff` instead of `time.Sleep` in slot | `operation/service.go:288`, `operation/store.go:36` |
| P1 | Forge-leader metrics full | `elector.IsLeader` drives `forge_leader_is_leader` gauge directly (not env) | `server.go:1571` |
| P2 | Tenant-tag events | `Envelope.TenantID` already propagated (`event.go:216` `tenantIDFromPayload`) — persist via `PublishTx` `tenantID` col | `eventstore/store.go:126,283` |

---

## 7. Verification

```bash
# Relay subscribers (expected 4)
grep -n "eventRelay.SubscribeSubscriber" forge/api/cmd/api/main.go
# → 461 fenceSvc, 982 failSvc, 1082 tmSvc, 1096 lbSvc

# Zero-handler guard (AF-1)
grep -n "zero subscribers" forge/api/internal/eventstore/outbox.go
# → 192, 221

# PublishTx primitive exists but zero prod callers (P0 gap)
grep -rn "PublishTx" forge --include="*.go" | wc -l
# → 11 (definition + doc only)

# Leader elector stub (P0 gap)
grep -n "isForgeLeader\|FORGE_LEADER\|QUEUE_SINGLE_WRITER" forge/api/cmd/api/main.go
# → 1220 isForgeLeader, 1221 QUEUE_SINGLE_WRITER, 1227 FORGE_LEADER

# Gated daemons after this slice
grep -n "isForgeLeader()" forge/api/cmd/api/main.go
# → 1235 rec, 1245 failSvc (new), 1249 bkWorker, 1261 cleanupSvc (new), 1292 periodicScheduler
# + periodic handler predicates 1263,1273 (QUEUE_SINGLE_WRITER && FORGE_LEADER check)

# Orphan center
grep -n "registerOrphanRemediationRoutes" forge/api/internal/http/server.go forge/api/internal/http/handlers_orphan_remediations.go
# → server.go:2635, handlers_orphan_remediations.go:13

# Dual-read metrics after this slice
grep -n "event_relay_subscribers\|event_store_pending\|event_registry_published" forge/api/internal/http/server.go
# → 1597, 1607, 1612, 1625 (new)

# Orphan migrations
ls forge/api/migrations/*orphan* forge/api/migrations/*leader* forge/api/migrations/*eventstore*
# → 040_truthful_server_lifecycle.sql, 042_database_provisioning_security.sql, 214_forge_leader.sql, 139_eventstore.sql
```

`go vet ./forge/api/internal/eventstore ./forge/api/cmd/api ./forge/api/internal/http` — clean (new metrics use existing `cfg.EventRelay`/`cfg.EventRegistry`/`cfg.Store.GetDB()`).

---

## 8. Code Changes in This Slice

- `forge/api/cmd/api/main.go:1071` — defer `cleanupSvc.Start` (remove unconditional, add gated start at 1260).
- `forge/api/cmd/api/main.go:1244` — gate `failSvc.Start` on `isForgeLeader()` (was always).
- `forge/api/cmd/api/main.go:1259` — add gated `cleanupSvc.Start` with `AF-2` comment.
- `forge/api/internal/http/server.go:1558` — extend metrics block: `game_panel_api_event_relay_subscribers`, `game_panel_api_event_registry_published_total`, `game_panel_api_event_store_pending/dispatched/dead_letter` (dual-read observability).

No `PublishTx` site wired in this slice — documented as P0 gap for next lane (requires per-service tx threading, not safe as sharded parallel edit). No `FORGE_LEADER` elector instantiation wired — documented as P0 with design notes.

---

## 9. Files Referenced

- `forge/api/internal/eventstore/outbox.go:39,48,61,71,77,106,134,192,221,262`
- `forge/api/internal/eventstore/store.go:68,114,211,267,297,314`
- `forge/api/queue/leader.go:23,29,46,93,118,123`
- `forge/api/migrations/214_forge_leader.sql:6,22`
- `forge/api/migrations/139_eventstore.sql` (via `eventstore/migration.go`)
- `forge/api/cmd/api/main.go:387,461,462,542,560,821,893,982,983,1046,1051,1071,1082,1096,1220,1235,1244,1248,1255,1260,1290`
- `forge/api/internal/http/server.go:150,1558,1597,1607,2635`
- `forge/api/internal/http/handlers_orphan_remediations.go:13,49`
- `forge/api/internal/store/store_orphan_remediations.go:56,114,143`
- `forge/api/internal/store/store_servers_lifecycle.go:37`
- `forge/api/internal/services/fencing/fencing.go:33,61`
- `forge/api/internal/services/failover/service.go:165,181,552`
- `forge/api/internal/services/cleanup/service.go:59,91`
- `audits/phase-05/subagent-02-river-queue.md` + `audits/phase-05/subagent-05-architecture-synthesis.md` + `audits/reverification/subagent-18-queue-events.md`
- `audits/reverification/REVERIFICATION_REPORT.md` (not updated — no new global verdict; this slice is wiring delta)

---

*Generated by subagent 08/10 — parallel wiring lane. Two daemons gated + dual-read metrics wired; PublishTx (10 sites) + real elector remain P0 for next lane. No 03-12 history rewritten.*
