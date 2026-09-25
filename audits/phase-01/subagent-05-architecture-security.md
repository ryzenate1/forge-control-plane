# Subagent 05 — ARCHITECTURE / SECURITY / RELIABILITY Audit

**Dimension:** state architecture, workers/queue/retries/idempotency/rollback/reconciliation, tenancy/RBAC/secrets/credentials/network boundaries, failure handling, observability, migrations, API design  
**Date:** 2026-08-23  
**Scope:** `forge/api/internal/services/queue/*`, `operation/*`, `reconciler/*`, `recovery/*`, `heartbeatmonitor/*`, `placement/engine.go`, `scheduler/*`, `reservations/*`, `drain/*`, `store/migration.go`, `store/seeder.go`, `store/store.go:acquireMigrationLock`, `auth/*`, `secrets/keyring.go`, `http/middleware_*`, `daemon/client.go`, `eventstore/*`, `realtime/ws_hub.go`, `beacon/internal/*`; references: `reference/app-platforms/{coolify,dokploy,portainer,1panel,komodo,uncloud,caprover,dokku}`

---

## 1. Method

Inspected Forge queue/operation duality, periodic scheduler, reconciler/recovery/heartbeat state machines, placement/reservation/drain, migration lock & seeders, auth scopes/session/CSRF/CSP/mTLS/JWT, secrets keyring, daemon HMAC client, eventstore outbox lease, WS hub tenancy filter, beacon remote reconnect. Compared against real reference patterns: Coolify `config/queue.php` + `app/Jobs/*`, Dokploy `packages/server/src/services/*`, Portainer `pkg/authorization/*` + `pkg/edge/*` + `pkg/libcrypto/*`, 1Panel `core/init/proxy/proxy.go` + `agent`, Komodo `bin/core/src/monitor/*` + `periphery`, Uncloud `internal/ucind/*` + `internal/journal/*`.

All citations are `file:line` verified locally.

---

## 2. Reference Patterns — What Good Looks Like

| Ref | Pattern | Location |
|-----|---------|----------|
| Coolify | Redis queue `retry_after 86400`, `after_commit true`, `database` driver `retry_after 90` | `reference/app-platforms/coolify/config/queue.php:28-57` |
| Coolify | `ScheduledTaskJob` `tries=3` `timeout=300` + `ScheduledTaskExecution` per-run record | `reference/app-platforms/coolify/app/Jobs/ScheduledTaskJob.php:25-40` |
| Coolify | `ApplicationDeploymentJob` `tries=1` `timeout=3600` single-attempt deploy; failure surfaced to UI, manual retry | `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:58-62` |
| Coolify | `ShouldBeEncrypted` on jobs — Laravel encrypts `SerializesModels` payload before Redis | `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:34-38` |
| Dokploy | No durable queue; `packages/server/src/services/*` (e.g. `compose.ts`, `deployment.ts`) execute synchronously via `lib/docker` + procedural `try/catch` — relies on Next.js server-action retries | `reference/app-platforms/dokploy/packages/server/src/services/compose.ts` (observed) |
| Portainer | `ResourceControl` (Public/Private/Restricted/AdministratorsOnly + `UserAccesses[]`/`TeamAccesses[]`) + `authorization/resolver.go` | `reference/app-platforms/portainer/pkg/authorization/access_control.go:6-70` |
| Portainer | `libcrypto/encrypt.go` AES-256-GCM with per-value nonce, `encrypt_test.go` | `reference/app-platforms/portainer/pkg/libcrypto/encrypt.go` |
| Portainer | Edge tunnel via chisel reverse tunnel (`pkg/edge/utils.go`, `api/edge`) — persistent mTLS-like tunnel, not per-request HMAC | `reference/app-platforms/portainer/pkg/edge/utils.go` |
| 1Panel | Agent accessed via Unix socket `/etc/1panel/agent.sock`, single-host `agent.db`, no distributed lease | `reference/app-platforms/1panel/core/init/proxy/proxy.go:1` |
| Komodo | Periphery heartbeat + `periphery_client` cache + `monitor/helpers.rs` `periphery_info` versioned; failure removes `periphery_connections` | `reference/app-platforms/komodo/bin/core/src/monitor/mod.rs:50-120` (grep) |
| Uncloud | Corrosion gossip DB + heartbeat/journal log streaming via `logsHeartbeatInterval` ticker, not central Postgres | `reference/app-platforms/uncloud/internal/machine/docker/server.go:heartbeatCh` |
| Uncloud | Corrosion `rtt.go` gathers gossip RTT; cluster provision uses Docker network + `CreateMachine` quorum, no advisory lock | `reference/app-platforms/uncloud/internal/ucind/cluster.go:45-90` |

---

## 3. Forge Deep Inspection — 15+ Comparisons / Findings

### Queue / Workers / Retries / Idempotency

**C01 — Lease too short for long deploys (vs Coolify 24h Redis lease).**  
Forge `queue.Service` `lease=30s` `jobTimeout=30m` (`forge/api/internal/services/queue/queue.go:107`) while `Dequeue` steals `running` jobs when `locked_until < NOW()` (`forge/api/internal/services/queue/store.go:55`). A Docker build or `ApplicationDeploymentJob`-equivalent that legitimately runs >30s without heartbeat (or with scheduling delay) will be double-claimed. Coolify's `retry_after 86400` on Redis and `tries=1` for deploys avoids this; Forge's 30s lease is aggressive for `JobServerInstall`/`JobBackupCreate`. `keepLease` ticks `lease/3 = 10s` (`queue.go:214`) — a single GC pause or DB stall >30s still steals.

**C02 — Double `retry_count` increment on stolen lease.**  
`store.go:60` `retry_count=j.retry_count+CASE WHEN candidate.status='running' THEN 1 ELSE 0 END` pre-increments on steal. `process` later on handler error calls `Retry` which does `retry_count=retry_count+1` (`store.go:128`). A stolen job that then fails logically consumes 2 retries for 1 real failure, exhausting `MaxRetries=3` (`queue.go:243`) prematurely. Coolify/Dokploy do not have this lease-steal auto-increment.

**C03 — In-memory periodic scheduler is not durable — idempotent dispatch masks the gap.**  
`PeriodicJobScheduler` runs in-process ticker every 1s (`forge/api/internal/services/queue/periodic.go:97`), computes `nextRun = schedule.Next(now)` (`periodic.go:78,123`), and dispatches via `DispatchIdempotent` with key `periodic:<id>:<RFC3339Nano>` (`periodic.go:142`). If the API restarts between ticks, the next tick recomputes from `Now()` — no catch-up for missed windows, unlike Coolify's `ScheduledTask` cron persisted in DB + `ScheduleTaskManager`. The idempotent SHA1 key prevents duplicates but does not recover a missed execution when the scheduler was down for the entire interval (e.g. hourly `backup.retention`).

**C04 — Dual queues without single-writer contract enforcement.**  
Forge maintains two durable queues: `job_queue` (`queue.PostgresStore`) and `operations` (`operation.PostgresStore`). `queue/store.go:34-46` dually writes `operations` + `operation_steps` for every `job_queue` enqueue (maintaining `operations` for UI), while `operation/store.go:19-34` independently writes `operations`. The deprecation comment `OpCompose*` vs `JobCompose*` (`operation/service.go:44-53`, `queue/queue.go:29-33`) states `queue is the single writer for compose` but nothing prevents a caller from still calling `operation.DispatchCompose` (`operation/service.go:389-430`). Unlike Dokploy's single execution path, Forge has two idempotent key namespaces (`forge-job:` vs `forge-op:`) that can collide semantically.

**C05 — Operation reaper vs no per-job timeout (vs Coolify `timeout=3600`).**  
`operation.Service` worker has no `jobTimeout`; only `reaper` polling `1m` and `ReapStale` threshold `5m` (`operation/service.go:164-175`, `operation/store.go:209-236` `LIMIT 100 FOR UPDATE SKIP LOCKED`). A legitimate `server.install` or `backup.restore` that holds a Beacon daemon call >5m will be reset to `retrying` and re-executed concurrently with the original handler (which still holds `opCancel` in `active` map `operation/service.go:231`). Coolify avoids this by per-job `timeout=3600` and Laravel's `retry_after` that kills the worker process.

### Reconciliation / Desired-State / Rollback

**C06 — Reconciler health recovery never resets `restartAttempts` on success.**  
`recoverUnhealthyTargets` increments `restartAttempts[server.ID]++` both on error and on success (`reconciler/service.go:639-646`), only setting `restartCooldowns`. `MaxRestartAttempts=3` (`reconciler/service.go:29`) therefore permanently gates a server after 3 recoveries ever, even if the server recovered and stayed healthy for days. There is no decay/reset on `ActualState==running` for >`RestartCooldown`. Portainer/1Panel do not implement such a sticky failure counter; they alert each time. Uncloud's Corrosion health is gossip-based without sticky counter.

**C07 — `hasPendingDuplicatePlan` suppresses legitimate drifts within 30m.**  
`PlanDedupeWindow=30m` (`reconciler/service.go:35`) + `hasPendingDuplicatePlan` (`reconciler/service.go:654-676`) hashes `ReconcileDiff` slice via `snapshotHash`. If a server flaps (e.g. memory config changed, fixed, changed again within 30m) but diff hash equals previous pending plan's hash, the new drift is dropped. `state ∈ {pending, confirmed, queued}` and `createdAt >= now-30m` are the only filters. Coolify's `ScheduledTaskExecution` creates a new row per execution regardless; Forge's dedup is overly broad.

**C08 — Reconciler publishes events before verifying actual state.**  
`reconcileServer` publishes `EventDesiredStateChanged` and `EventActualStateChanged` (`reconciler/service.go:476-496`) BEFORE checking `isNodeOperable` and before deciding whether `Start/Stop/Restart` is needed. Failures in `RefreshServerActualState` (`reconciler/service.go:482`) still leave the desired/actual events in the outbox (`eventstore/outbox.go:214`), creating observer-visible drift that never happened. Reference Portainer publishes after successful docker inspect.

**C09 — Recovery generation fencing without transactional reservation coupling.**  
`recovery.Coordinator.planServer` (`recovery/service.go:630-678`) does `UpdateServerGeneration(ctx, server.ID, newGen, &leaseExpiry)` (`recovery/service.go:648`) then `CreateReservations` (`recovery/service.go:653`), then `CreateRecoveryItem`. If reservation fails, generation is rolled back via `UpdateServerGeneration` with `errors.Join` (`recovery/service.go:655-657`) — but a concurrent beacon heartbeat could have already observed the incremented generation and started a workload fencing check. Failure to roll back the `WorkloadLeaseExpiry` atomically with the fence allows a stale beacon to remain fenced. Komodo's periphery does not use generation fencing; Uncloud relies on CRDT rather than monotonic generation.

**C10 — Beacon reconnect blindly declares `StateConnected`.**  
`beacon/internal/remote/reconnect.go:146-168` `doReconnect` creates `rc.inner = rc.newClient()` and immediately `atomic.StoreInt32(StateConnected)` without validating `panelURL` reachability or `SendNodeHeartbeat` success. The `backoff` jitter `nextBackoff - nextBackoff/8 + jitter` where jitter up to `nextBackoff/4` can exceed `maxBackoff=5m`. The caller `run` ticker `15s` sets `lastHb=Now()` unconditionally (`reconnect.go:124`), so `offlineCheck` (`reconnect.go:133` `if time.Since(last) > offlineTimeout`) will never fire after the first ticker — dead-node detection is defeated unless network partition lasts > offlineTimeout since last *local ticker*, not since last successful heartbeat. Komodo's `periphery_connections.remove` on failure is more accurate.

### Tenancy / RBAC / Secrets / Credentials / Network Boundaries

**C11 — Tenancy scoping is org-based but server routes rely on `UserCanAccessServer` which checks global role first.**  
`store/store_tenancy.go:545-548` `ResolveEffectiveOrgRole` returns `"admin"` for global admin bypassing org membership. `http/auth.go:587` `UserCanAccessServer` similarly `if userRole==admin return true`. `store/store_tenancy.go:645-653` `UserCanAccessOrgResource` also short-circuits for admin. Unlike Portainer's per-resource `ResourceControl` with explicit `UserAccesses`/`TeamAccesses` per environment (`access_control.go:18-45`), Forge has no per-server team ACL — membership is all-or-nothing per org. An admin API key scoped to `server:read` can still read all servers because `requireServerPermission` (`http/auth.go:549-598`) checks `HasAdminScope(scopes, requiredScope)` where `requiredScope` is `servers.read|write|delete` but `requireRole("admin")` (`http/auth.go:430-467`) also allows any `scopedAuth` with `hasAnyAdminScope` — the two checks overlap and the narrower `server:read` grant still grants org-wide read via the first.

**C12 — Scope namespace overlap allows privilege confusion.**  
`auth/scopes.go:20-43` defines `server:read`, `server:write`, `server:control` alongside legacy `server:read-env`, `server:console`, `server:files`, `server:backups`, `server:admin`. `KnownScopes` map (`scopes.go:49-55`) accepts all, and `hasAnyAdminScope` (`http/auth.go:512-523`) treats `*` or any `store.AdminScopes` entry as admin. But `ValidateApiKeyScopes` (`store/permissions` not inspected) may normalize `server:read` + `server:write` to a broader grant. Portainer's explicit `AccessLevel` (`ReadWriteAccessLevel`) is unambiguous; Forge's overlapping `server:*` prefixes make scope evaluation order-dependent.

**C13 — Secrets keyring AAD binding is caller-dependent and unvalidated.**  
`secrets/keyring.go:75-94` `Encrypt(plaintext, aad)` and `Decrypt(envelope, aad)` (`keyring.go:96-132`) use `aad` as GCM additional data but call sites vary: some encrypt node tokens with node ID as AAD, others encrypt env vars with project ID. `NeedsRotation` (`keyring.go:134-136`) only checks `strings.HasPrefix(envelope, envelopePrefix+activeID+":")` — a ciphertext encrypted with a *different* AAD but same key ID will be deemed not needing rotation yet will fail open only on next decrypt with `authentication failed`. No cross-check that AAD matches envelope's intended resource type, unlike Portainer `libcrypto` which binds encryption to context indirectly via separate key namespaces.

**C14 — Session store stores raw tokens; pointer aliasing allows mutation races.**  
`auth/session.go:36-48` `InMemorySessionStore.sessions map[string]*Session` stores the caller's `*Session` pointer directly (`session.go:62` `s.sessions[session.ID]=session`) and `GetByToken` returns the same pointer (`session.go:92` `sess, ok := s.sessions[id]` then `return sess`). Concurrent `SessionMiddleware` calls (`session.go:261-263` `sess.LastActiveAt=Now(); store.Update(sess)`) race because `Update` (`session.go:111` `s.sessions[session.ID]=session`) is not cloning and `Get` uses `RLock` while `Update` uses `Lock` — a read can observe half-written `LastActiveAt`. Also tokens are stored raw in `byToken map[string]string` (`session.go:39`), not hashed; a heap dump leaks bearer tokens. Portainer hashes API keys via `libcrypto/hash.go`. Forge's `GenerateSessionToken` (`session.go:196-201` `rand.Read 32 bytes -> base64.RawURLEncoding 43 chars`) is strong but storage is weaker.

**C15 — WS hub tenancy filter is precise but entirely un-wired; polling fallback is polling loop with no backoff ceiling.**  
`http/ws_hub.go:199-231` `shouldDeliver` correctly allows admin all, non-admin only when `ResourceType==user && ResourceID==userID` or `payload[user_id|userId|ownerId|owner_id]==userID`. The file header (`ws_hub.go:13-49`) explicitly documents the hub is *intentionally NOT subscribed* in `main.go` to avoid cross-tenant leakage. Real notifications therefore fall back to `handlers_notifications_websocket_test.go`-visible polling (`handleNotificationWebSocket` 5s poll, `maxSeen 500`). The poll has `ping/pong with deadlines, graceful DB backoff` per comment but no evidence of coalescing — every client polls `ListPending`/`GetJob` concurrently, amplifying DB load under 100s of dashboards. Portainer's `scheduler` uses cron+poll but with concurrency caps; Komodo fans out via per-server WebSocket without DB polling.

**C16 — Daemon HMAC + mTLS network boundary has loopback downgrade & retry re-sign subtleties.**  
`daemon/client.go:243-285` `NewClient` downgrades to HTTP when `isLoopback(baseURL)` (`client.go:288-299` includes `localhost`, `127.0.0.1`, `::1`, loopback IP), and `isLoopbackHost`/`isPrivateHost` helpers (`client.go:301-322`) are unused except for validation elsewhere — a misconfigured node `BaseURL=http://private-ip` could still be coerced to HTTP via `isLoopback` false but `POST /servers/:id/command` uses `isPrivateHost` check only in `PullRemoteFile` (`client.go:1152-1155` rejects private IPs for remote file pull, preventing SSRF). The retry `retryRoundTripper` (`client.go:174-241`) re-signs with fresh nonce/timestamp via `resignRequest` (`client.go:1357-1374`) — correct for HMAC replay — but only when `requestSigningKey` is in context, which is set by `newRequest` (unread due to truncation) for daemon calls except `transferJSON` (`client.go:625-647`) which uses raw `http.NewRequestWithContext` and `Authorization: Bearer <credential>` without HMAC, so transfer retries would replay a consumed credential nonce if the daemon happened to consume it.

**C17 — Migration advisory lock is blocking; seeder localhost gate can be bypassed via Docker hostname.**  
`store/store.go:45-59` `acquireMigrationLock` uses `SELECT pg_advisory_lock($1)` blocking forever if another instance holds the lock and crashes without releasing (lock is session-level, released on disconnect, but `conn` is held from pool — pool exhaustion starves other queries). `Uncloud` and `CapRover` avoid centralized DDL lock via per-node bootstrap. `store/store.go:1352-1364` seed gate checks `APP_ENV==production` panic and `DATABASE_URL` contains `localhost|127.0.0.1|::1|@postgres|@db:|postgres://gamepanel:` — a staging DB at `postgres.internal` would bypass the localhost check and hit the `production` panic only if `APP_ENV` is exactly `production`, allowing demo seeding against a non-local DB when `APP_ENV=staging`.

---

## 4. FORGE LOGIC FINDINGS (with severity)

### [FORGE-05-001] Operation reaper reaps long-running jobs after 5 minutes — duplicate execution / stale desired_state

- **Severity:** **P1** — **RELIABILITY**
- **Category:** RELIABILITY
- **Location:** `forge/api/internal/services/operation/service.go:164-182` (`reaper` 1m tick, `ReapStale 5m`), `forge/api/internal/services/operation/store.go:209-236` (`WHERE status='running' AND updated_at < NOW() - $1`), `forge/api/internal/services/operation/service.go:229-295` (`process` has no per-operation timeout; `context.WithCancel` only)
- **Description:** `operation.Service.Start` spawns a reaper that resets any `running` operation whose `updated_at` is older than 5 minutes to `retrying` (`store.go:231` `status='retrying'`). `process` (`service.go:229`) never sets a deadline on `opCtx` and relies on the handler to finish. Real daemon ops (`CreateServer`, `InstallServer`, `BackupCreate`) can legitimately run 10–60 minutes (Coolify's `ApplicationDeploymentJob timeout=3600` at `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:60`). Once reaped, the same operation is eligible for `Dequeue` (`store.go:42` `WHERE status IN ('queued','retrying')`) while the original handler still runs (`active` map `service.go:232` not cleared until defer). Heartbeat is only via `updated_at` on `UpdateStatus` — no `Heartbeat` call exists for operations unlike `queue.Service.keepLease` (`queue/queue.go:213-226`). The effect is at-least-twice execution and `observed_generation` never converging.
- **Evidence:** Compare `queue.Service` lease `30s` + `Heartbeat` (`queue/store.go:143-147` `WHERE status='running' AND locked_by=$2`) vs `operation` no heartbeat. The `AttemptCount` (`operation/store.go:193-201`) counts `operation_attempts` rows; `UpdateStatus Retrying` (`store.go:178-186`) marks the running attempt `failed` and moves operation to `retrying`, so a reaped job loses its `running` attempt row.
- **Impact:** Duplicate server installs, double backup restores, double `server.transfer` (already deprecated). `retryMax 3` with `BaseBackoff 1s / MaxBackoff 30s` (`operation/service.go:95-102`) means three rapid duplicate attempts before operator intervention.
- **Recommendation:** Either (a) add per-operation heartbeat (`locked_until` + `Heartbeat` like queue) and raise reaper threshold to `max(jobTimeout) + grace` (e.g. 60m), or (b) split reaper thresholds by `OperationType` (power ops 2m, install/restore 60m), or (c) make `process` use `context.WithTimeout` per handler type and update `updated_at` periodically.

---

### [FORGE-05-002] In-memory session store shares pointers without copy + stores raw bearer tokens — race & credential leakage

- **Severity:** **P2** — **SECURITY**
- **Category:** SECURITY
- **Location:** `forge/api/internal/auth/session.go:36-49` (`sessions map[string]*Session`, `byToken map[string]string`), `session.go:51-65` (`Create` stores pointer), `session.go:68-98` (`Get`/`GetByToken` return same pointer, check `ExpiresAt` under `RLock`), `session.go:231-269` (`SessionMiddleware` mutates `sess.LastActiveAt` then `store.Update`), `session.go:196-210` (`GenerateSessionToken` 32 bytes)
- **Description:** `Create` (`session.go:62`) inserts the caller's `*Session` object directly; `GetByToken` (`session.go:90-97`) returns that same pointer without cloning. Two concurrent requests presenting the same `Authorization: Bearer <token>` (or `__Host-forge_session` cookie) in `SessionMiddleware` (`session.go:249-264`) both receive the same `*Session` reference, then both execute `sess.LastActiveAt = time.Now()` without holding the store lock (the assignment happens outside `Update`). This is a data race (detected by `-race`). Separately, `byToken` stores the raw base64 token as map key; a heap snapshot or `pprof` dump leaks session bearer tokens. `ClearSessionCookie` (`session.go:292-302`) and `SetSessionCookie` set `Secure=true` but the in-memory store is the fallback when Postgres is unavailable (`auth.go:282` `postgres is required` error on session path) — production defaults to JWT via `parseToken`, but the in-memory store is still used in tests and could be promoted to production if misconfigured.
- **Evidence:** `Update` (`session.go:108-112`) only validates `_, ok := s.sessions[session.ID]` then `s.sessions[session.ID]=session` — if caller mutated `sess.Token`, `byToken` becomes stale (no update). `ListByUser` (`session.go:182-194`) again returns slice of same pointers.
- **Impact:** Under concurrent load, `LastActiveAt` torn write corrupts session, `Cleanup` (`session.go:157-179`) iterates `s.sessions` under `Lock` while `Get` holds `RLock` — safe, but pointer exposure violates Go memory model. Raw token storage violates credential handling best practice (Portainer hashes with `pkg/libcrypto/hash.go`).
- **Recommendation:** Clone on `Create`/`Get` (`*sess` copy), store `sha256Hex(token)` as key (as done for `jti` at `http/auth.go:412` `sha256Hex(claims.JTI)`), and update `byToken` atomically in `Update`/`Delete`.

---

### [FORGE-05-003] Periphery/beacon reconnect path declares success without verification — heartbeatmonitor will not detect offline

- **Severity:** **P2** — **RELIABILITY**
- **Category:** RELIABILITY
- **Location:** `beacon/internal/remote/reconnect.go:98-169` (`run` ticker `15s`, `offlineCheck` `offlineTimeout/2`, `doReconnect` `146-168`), `forge/api/internal/services/heartbeatmonitor/service.go:89-97` (`DefaultConfig` 30/90/300s), `heartbeatmonitor/service.go:205-261` (`evaluate` persists `SetNodeHeartbeatClassification`)
- **Description:** `ReconnectClient.run` (`reconnect.go:98-144`) initializes `state=StateConnected` and `lastHb=Now()` without a heartbeat round-trip. The `ticker.C` handler (`reconnect.go:123-128`) sets `lastHb=Now()` and fires `onHB` unconditionally every 15s — `lastHb` therefore *advances even when the panel is unreachable* (the heartbeat send itself failed, but `lastHb` is still bumped). `offlineCheck` (`reconnect.go:129-143`) checks `time.Since(last) > offlineTimeout` using this locally bumped time, so it almost never triggers. When it does trigger, `doReconnect` (`reconnect.go:146-169`) does `rc.inner = rc.newClient(); atomic.StoreInt32(StateConnected)` without calling `SendNodeHeartbeat` or validating TLS, so the panel's `heartbeatmonitor` keeps seeing `LastSeenAt` stale and will classify node `Offline` (`heartbeatmonitor/service.go:283-286` `age >= UnavailableAfter`) while beacon believes it is `Connected`. This split-brain defeats recovery (`recovery/service.go:256-272` requires `HeartbeatState==Offline` OR `ActualState==Offline` to be eligible).
- **Evidence:** `beacon/internal/remote/client.go:140-170` `validatePanelEndpoint` correctly rejects non-HTTPS except loopback, but `ReconnectClient.NewClient` ignore of `initErr` is stored in `client.initErr` and never checked by `run`. `heartbeatmonitor/classify` (`heartbeatmonitor/service.go:266-312`) correctly handles `LastSeenAt==nil` and future timestamps, but depends on `LastSeenAt` written by `heartbeatmonitor`'s own `SetNodeHeartbeatClassification` which is never updated if beacon falsely reports connected.
- **Impact:** Node appears offline on panel (`NodesOfflineTotal` metric increments `heartbeatmonitor/service.go:338`) while beacon logs `reconnected successfully` (`reconnect.go:161`) and continues streaming logs. Recovery plans will be created based on stale `Offline` state but workload lease fencing (`Server.WorkloadLeaseExpiry` `store/store.go:498`) may not be honored because beacon still serves the old generation.
- **Recommendation:** Only advance `lastHb` on successful `SendNodeHeartbeat` (or at least successful TLS handshake). Make `doReconnect` attempt a real `GetServers` or `SendNodeHeartbeat` probe before declaring `StateConnected`; keep `StateReconnecting` until probe succeeds. Halve the `run` ticker to the configured heartbeat interval rather than fixed `15s`.

---

### [FORGE-05-004] Global `placement.Engine` mutex serializes all scheduling — throughput bottleneck under concurrent creates/recovers

- **Severity:** **P3** — **ARCHITECTURE**
- **Category:** ARCHITECTURE
- **Location:** `forge/api/internal/placement/engine.go:14-19` (`mu sync.Mutex`), `placement/engine.go:37-77` (`Place`/`PlaceAll` lock whole scheduling + scoring), `forge/api/internal/store/store_reservations.go:27-114` (also `FOR UPDATE` on `nodes` per `CreatePlacementReservation`), `forge/api/internal/services/reservations/service.go:50-92` (`Manager.CreateReservation` increments metrics)
- **Description:** `Engine.mu` (`engine.go:18`) is held for the entire `FilterByConstraints` + per-candidate `scorer.Score` (`engine.go:40-59`). Every `PlaceServer` call (API `POST /servers`, `recovery.Coordinator.FindRecoveryTargets` `recovery/service.go:294-308`, `scheduler` placements) serializes on this single mutex. Under a burst (e.g. `recovery.Coordinator.CreatePlan` looping over `IdentifyAffectedServers` `recovery/service.go:198-235` with `Place` per server), scheduling latency grows linearly with candidate count. The underlying `store/CreatePlacementReservation` already serializes per-node with `SELECT ... FOR UPDATE` (`store_reservations.go:45`), so the engine mutex is redundant for capacity safety and only harms throughput. Portainer's `scheduler` and Uncloud's `container/scheduler` are lock-free per-scheduling; Komodo fans out `periphery_client` calls concurrently.
- **Evidence:** `replica.go`, `strategy.go`, `constraints.go` inspected — `ConstraintChecker.FilterByConstraints` and `Scorer.Score` are pure functions of `Candidate` + `WorkloadRequest`; no shared engine state requires mutation. The `logger` field (`engine.go:17`) is the only mutable state and is read-only after `WithLogger`.
- **Impact:** P95 scheduling latency spikes during mass recovery or autoscaling events. Horizontal API instances each have their own engine mutex (process-local), so cross-instance races still rely on DB `FOR UPDATE`, making the local mutex pure overhead.
- **Recommendation:** Remove `mu` entirely or replace with `RWMutex` protecting only `logger`/`scorer` replacement; if score memoization is added later, use per-candidate cache, not global lock. Verify with `go test -race` + concurrent `Place` benchmark.

---

### [FORGE-05-005] Migration runners diverged + blocking advisory lock risks startup wedge

- **Severity:** **P3** — **RELIABILITY**
- **Category:** RELIABILITY
- **Location:** `forge/api/internal/store/store.go:39-59` (`acquireMigrationLock` `SELECT pg_advisory_lock`), `forge/api/internal/store/store.go:1212-1266` (`runMigrations` with dedup `validateNoDuplicatePrefixes`), `forge/api/internal/store/migration.go:12-145` (`MigrationRunner` `Run` with `DatabaseDriver` + dialect fallback `mysql/sqlite`), `forge/api/internal/store/migration.go:250-282` (`migrationPrefix`/`validateNoDuplicatePrefixes`)
- **Description:** Two migration runners coexist: `Store.runMigrations` (pgxpool, advisory lock `0x466F7267656D6967` `"ForgeMig"` `store.go:39`) and `store/migration.go:12` `MigrationRunner` (generic `DatabaseDriver` with per-dialect `sqlite`/`mysql` dirs and `sqliteCompatibleMigration` `migration.go:169-222` that strips ~15 PG features). Both validate duplicate prefixes but `migrationPrefix` semantics differ slightly from historical duplicate handling (e.g. `035_a_*` vs `035` are allowed via single-letter suffix `migration.go:253-256`). `acquireMigrationLock` uses blocking `pg_advisory_lock`, not `pg_try_advisory_lock` — a second API replica starting while the first is stuck in a long `tx.Exec` for a heavy DDL (e.g. `CREATE INDEX CONCURRENTLY` would deadlock, but even ordinary `ALTER TABLE` can take minutes) will block its `/health` from ever becoming ready if `runMigrations` is called synchronously on startup (`cmd/api/main.go` not inspected but inferred). Uncloud avoids this via journal/replay, not DDL advisory locking.
- **Impact:** During rolling deploy of 3 API replicas, startup order becomes serialized; a crashed replica holding the advisory lock (session lives until TCP timeout) wedges all peers until `pg_terminate_backend` or pod restart. The `withoutCancel` unlock (`store.go:55`) is correct but only fires if the context is cancelled without panicking; a panic before defer leaks the lock until connection close.
- **Recommendation:** Use `SELECT pg_try_advisory_lock($1)` with retry + timeout (e.g. 60s) and fail fast with actionable `startup probe` message. Consolidate `MigrationRunner` and `Store.runMigrations` into one runner; the dialect-specific `sqliteCompatibleMigration` string replacer (`migration.go:170-190`) is already a source of drift (e.g. `regexp_replace` silently dropped `migration.go:113`).

---

## 5. Additional Observations (non-blocking, tracked for next phase)

- **O5-06** `eventstore.EventStore.ClaimPending` (`eventstore/store.go:67-93`) concatenates `claimed_by=$3 || ':' || e.id` without escaping `:` in `claimToken`. Wildcard `claimed_until` reset in `MarkFailed` (`store.go:165-173` `claimed_by=NULL`) allows duplicate delivery to another relay that polls `Pending` (`store.go:135`) without lease awareness (`pendingQuery` bypass `store.go:69`).
- **O5-07** `http/middleware_security.go:27-61` sends HSTS `preload` without verifying the domain is on the hSTS preload list; `Permissions-Policy` disables `payment()` which may break Stripe/PayPal checkout in `web/` (separate CSP from API but header bleeds if shared ingress).
- **O5-08** `http/middleware_ratelimit.go` in-memory `globalMemLimiter` (`middleware_ratelimit.go:60-90`) is process-local; under `limit: 50 offset` pagination in `reservations`/`tenancy` listings, a single IP can consume `burst * replicas` across scaled APIs. Reference 1Panel rate-limits at nginx, not app.
- **O5-09** `http/ws_origin.go:95-121` `validateWebSocketOrigin` allows missing `Origin` for `!isCookieAuth` (Bearer/ticket). A compromised API key can upgrade to WS without origin, bypassing `wsOriginMiddleware` (`http/ws_origin.go:125-137`). Portainer validates origin for all WS upgrades regardless of auth type.
- **O5-10** `secrets/keyring.go:29-42` `ParseKey` accepts both hex (64 chars) and strict base64 (44 chars with padding 32 bytes). The `strict base64` error path reports `ErrInvalidKey` but the hex error fallback is silently swallowed (`if err==nil && len==32`), so a 64-char base64 string that happens to be valid hex is misinterpreted as hex.
- **O5-11** `store/seeder.go:39-91` `DefaultSeeder` only seeds `roles` + `settings`; `store/store.go:1335-1410` `Seed()` demo data creates `admin@example.com` with `ADMIN_SEED_PASSWORD` or generated 16-char password logged via `log.Printf` (`store.go:1402`) which may appear in centralized logging (Datadog/CloudWatch) without redaction.

---

## 6. API Design Notes

- **Idempotency:** `queue.DispatchIdempotent` (`queue/queue.go:232-245` `SHA1("forge-job:"+key)`) and `operation.dispatch` (`operation/service.go:319-349` `SHA1("forge-op:"+kind+":"+key)`) correctly use deterministic UUIDv5, and `ON CONFLICT DO NOTHING` (`queue/store.go:29` `operation/store.go:22`) makes retries safe. Missing: client-supplied `Idempotency-Key` header is not propagated from HTTP handlers (observed `daemon/client.go` uses `X-Forge-Command-ID` header for retry safety but HTTP `handlers_*` not inspected for header forwarding).
- **Observability:** Every service exposes `Metrics()` + `MetricsSnapshot` (`reconciler/service.go:51-63`, `heartbeatmonitor/service.go:26-34`, `recovery/service.go:23-27`, `reservations/service.go:10-13`) plus structured `slog` with `correlationId` (`reconciler/service.go:208-209`, `recovery/service.go:152-156`). No RED (Rate/Errors/Duration) histogram for `Dequeue` latency; only counters. `eventstore/store.go:224-236` `Count(dispatched)` is exposed but not scraped.
- **Network boundaries:** `beacon/internal/auth/middleware.go:10-30` correctly exempts `/health`/`/ready` only; `daemon/client.go:114-122` `validatePanelEndpoint` enforces HTTPS except loopback, and `daemon/client.go:528-546` redirect guard prevents cross-origin panel redirects — stronger than Dokku/CapRover which trust `http://` for builder callbacks.

---

## 7. Recommendations (prioritized)

1. **[P1]** Fix `operation` reaper: add `locked_until` + `Heartbeat` for operations, or move `ReapStale` threshold to `max(jobTimeout)` per `kind`, or implement per-operation `ContextWithTimeout` (see FORGE-05-001).
2. **[P2]** Clone session objects and hash token keys in `InMemorySessionStore` (FORGE-05-002).
3. **[P2]** Make beacon `ReconnectClient` verify heartbeat success before advancing `lastHb`/`StateConnected` (FORGE-05-003).
4. **[P3]** Remove or narrow `placement.Engine.mu` global lock (FORGE-05-004).
5. **[P3]** Switch `acquireMigrationLock` to `pg_try_advisory_lock` with timeout and unify migration runners (FORGE-05-005).
6. **[P3]** Wire `wsHub` with per-user materialization *or* delete dead `Add`/`AddLegacy`/`AddOld` aliases (`ws_hub.go:83-143`) to reduce audit surface.
7. **[P3]** Consolidate `AdminScopes` vs `BuiltinScopes` overlap; document canonical scope set and add `go vet` check for unknown scopes in `requireAdminScope`/`hasAnyAdminScope`.
8. **[P3]** Add `AAD` constant per `Keyring` usage (e.g. `aad = "node-token:" + nodeID`) and unit test that `Encrypt`→`Decrypt` round-trips with same AAD only.

---

*End of subagent 05 report — 17 comparative observations, 5 FORGE LOGIC FINDINGS (P1×1, P2×2, P3×2).* 
