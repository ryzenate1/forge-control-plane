# Final Parity — Subagent 01 — GAME HOSTING (Definitive)

**Subagent:** 01 of 10 (parallel) — Game Hosting Lifecycle & Templating  
**Date:** 2026-08-24  
**References:** `pterodactyl-panel` `pelican-panel` `pterodactyl-wings` `pelican-wings` `pufferpanel` `pufferpanel-templates` under `/Users/riyaz/project/gamepanel/reference/game-hosting/`  
**Forge surface re-verified (not trusting prior audits):**
- `forge/api/internal/store/store_servers*.go` `store_nests.go` `store_egg_variables.go` `store_startup.go` `store_templates.go` `store_allocations.go` `store_state.go` `store_schedules.go` `store_users.go` `store_mounts_ext.go`
- `forge/api/internal/http/handlers_servers.go` `handlers_admin.go` `handlers_user_console.go`
- `forge/api/internal/services/clustermanager/service.go` `reconciler/*` `services/orchestrator/*` `heartbeatmonitor/*` `operation/*`
- `beacon/internal/server/server.go` `manager.go` `beacon/internal/runtime/docker.go` `runtime.go`
- `forge/web/app/admin/nests/*` `app/admin/app-templates/*` `components/server/*` `lib/egg-templates.ts` `lib/app-templates-data.ts`
- `forge/api/docs/server-lifecycle.md` `forge/api/migrations/*`
- `packages/shared-types` and phantom `packages/game-templates` (verified absent on `main`)

**Prior audits reconciled:** `audits/phase-02/synthesis.md` (+ subagent-01…05), `audits/phase-06/subagent-05-pufferpanel-daemon.md`, `audits/phase-06/subagent-06-templates-catalog.md`, `audits/MASTER_FINDING_INDEX.md`, `audits/FINAL_REFERENCE_ECOSYSTEM_REPORT.md`

**Method:** Direct `read`+`grep` file:line re-inspection of 6 references vs 8 Forge layers. Status taxonomy: `COMPLETE | PARTIAL | UNWIRED | BROKEN | MISSING | DEAD | DUPLICATE | FALSE_COMPLETION | UNKNOWN`. Recommendation taxonomy: `ADOPT | ADAPT | INSPIRE | REJECT`. Severity: `P0-P4` + `USER_VISIBLE|OPERATOR_VISIBLE|SILENT`.

---

## 0. Reading guide

Each capability row is **independent** (not lumped). Format:

- **REFERENCE** — exact symbol & behavior with `file:line`
- **FORGE** — 8-layer trace `Frontend:API:Service:Store:DB:Worker:Beacon:Event:Tests` each with `file:line` (or `—` if absent)
- **STATUS/GAP/FORGE LOGIC FINDING/RECOMMENDATION/SEVERITY**

Shared Bootstrap context: Pterodactyl `Server.php` is canonical game-hosting model (`reference/game-hosting/pterodactyl-panel/app/Models/Server.php:1`), Wings daemons are correctness horizon (`pterodactyl-wings/server/power.go:1`, `install.go:1`), Pelican is strict fork diff, PufferPanel is typed declarative alternative (`pufferpanel-templates/spec.json:1`).

---

## 1. Capability Matrix (19 rows)

### C-01 — Create / Provisioning (`provisioning` → `created` ≠ `installed`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Server.php:125` `STATUS_INSTALLING='installing'` + `protected $attributes=['status'=>self::STATUS_INSTALLING]` (`Server.php:138`) — new rows default to `installing`; `isInstalled():215` checks `status !== installing && !== install_failed`. No `provisioning` — Wings creates container immediately after DB row.
- `pterodactyl-panel/app/Http/Controllers/Api/Application/Servers/ServerController.php:40` `POST /api/application/servers` creates row then enqueues install via daemon.
- `pufferpanel/servers/server_loader.go:120` `Create()` → `os.Mkdir + Save + Load + CreateEnvironment` — filesystem creation distinct from `Install()` (`servers/server.go:410`).
- `pufferpanel-templates/spec.json:51` `install: []Operation` separate from `run.command`.
- `pelican-panel/app/Enums/ServerState.php:12` enum `Installing|InstallFailed|ReinstallFailed|Suspended|RestoringBackup` — still single column, same creation default.

**FORGE**
- **Frontend:** `forge/web/lib/api.ts:233` `createServer()` + `forge/web/app/admin/servers/page.tsx:1` + `forge/api/docs/server-lifecycle.md:5` documents `provisioning` (panel-only, never exposed to runtime).
- **API:** `forge/api/internal/http/handlers_servers.go:789` `POST /servers` → `clusterManager.CreateServer()` (`handlers_servers.go:838`), maps resource limits, returns DTO.
- **Service:** `forge/api/internal/services/clustermanager/service.go:76` `CreateServer()` does placement `scheduler.PlaceServer:92`, `store.CreateServer:140` inside Tx, then `provisionServer:172` → `syncProvisionTarget:276` → `runtime.CreateServer:180` → `SetServerProvisioned:187`. Compensation `compensateCreateFailure:285` handles partials. Reservation `CreateReservation:110` with 10m TTL.
- **Store:** `forge/api/internal/store/store_servers.go:253` `INSERT ... 'provisioning','stopped','stopped'` + `store_servers_control.go:207` `SetServerProvisioned()` flips `status='created', actual_state='stopped', installed=false`.
- **DB:** `forge/api/migrations/001_init.sql:24` `status TEXT DEFAULT 'stopped'`; `021_true_state_persistence.sql:34` adds `desired_state/actual_state`; `007_postgres_core_foundation.sql:106` seeds `Games` nest; `040_truthful_server_lifecycle.sql:4` cleans stale `installing`.
- **Worker:** no async worker — synchronous create with 15m timeout; `operation/service.go:152` reaper only for power/install.
- **Beacon:** `beacon/internal/server/server.go:735` `create` → `runtime.Create:829` → `persistRuntimeRequest:893` → `manager.MarkCreated:312` sets `InstallationState=installed, ContainerExists=true, PowerState=Offline`.
- **Event:** `clustermanager/service.go:165` `EventServerCreated` with `desiredState/actualState/correlationId`.
- **Tests:** `forge/api/internal/store/store_servers_lifecycle_integration_test.go:1` (create→provisioned flow).

**STATUS:** `COMPLETE` (intentional superset — more truthful than refs)

**GAP:** Pterodactyl overloads single `status` for both DB provisioning and workload install; Forge correctly splits `provisioning` (DB-only, never sent to Beacon) → `created` (workload exists, not installed) → `installing`. Frontend `lifecycle-machines.ts:41` `POWER_MACHINE` lacks `provisioning` badge state — falls to raw string; Pelican `ServerState` enum already has 5 values but still uses `installing` at birth.

**FORGE LOGIC FINDING:** C-01-LF — `beacon/internal/runtime/docker.go:193` `createRequestHash()` hashes marshaled `CreateRequest` **without** generation seed; two `Create` retries with same spec but different `desired_generation` collapse to same hash and skip reconciliation. Also `clustermanager/service.go:140-154` cancel/reservation race: `cancelReservation` on `CreateServer` error after `store.CreateServer` succeeds but before `provisionServer` — reservation confirmed only after `SetServerProvisioned` success, leaving 10m over-reservation window.

**RECOMMENDATION:** `ADOPT` — keep split; add `provisioning` to `POWER_MACHINE` badge map and docs. Fix hash to include `desired_generation`.

**SEVERITY:** `P3 OPERATOR_VISIBLE` (positive divergence, minor UI truncation)

---

### C-02 — Desired / Actual State Machine + Generation Fencing

**REFERENCE**
- `pterodactyl-panel/app/Models/Server.php:122` constants `installing|install_failed|reinstall_failed|suspended|restoring_backup` — single nullable `status`; no desired/actual; `validateCurrentState:393` bundles 4 guards.
- `pelican-panel/app/Enums/ServerState.php:12` enum same 5; still single column, actual power (`ProcessOffline` etc) ephemeral in Wings, not persisted.
- `pterodactyl-wings/server/server.go:151` `GetEnvironmentVariables` ephemeral.
- No `desired_generation/observed_generation` or `state_transitions` in any of 6 refs.

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:66` exposes `desired_state/actual_state` via DTO (`store/store.go:452`); `lifecycle-machines.ts:41` single-lane rendering (gap).
- **API:** `forge/api/internal/store/store.go:33` selects `desired_state::text, actual_state::text`.
- **Service:** `forge/api/internal/services/clustermanager/service.go:382` `RequestServerPower` sets desired first `SetServerDesiredState:398` + publishes `EventDesiredStateChanged`, then `sendPower:532` sets actual + `EventActualStateChanged` + `EventServerStarted/Stopped/Restarted:576`. `RefreshServerActualState:413` probes `Stats` to infer running. `ReconcileNode:449` drifts publish.
- **Store:** `forge/api/internal/store/store_state.go:10` `SetServerDesiredState` increments `desired_generation` on distinct; `store_state.go:31` `SetServerActualState` maps actual→legacy `status` (`serverStatusFromActual:131`) and fences `observed_generation = CASE WHEN (desired='running' AND actual='running') OR (desired='stopped' AND actual='stopped') THEN desired_generation ELSE observed_generation END` (`store_state.go:39`). `recordStateTransition:120` audits to `state_transitions`.
- **DB:** `forge/api/migrations/021_true_state_persistence.sql:3` `ENUM server_desired_state('running','stopped')` + `server_actual_state('running','stopped','starting','stopping','installing','crashed','unknown')` + `state_transitions` table; `150_heartbeat_reconciling_state.sql`.
- **Worker:** `forge/api/internal/services/reconciler/service.go:267` computes diffs, `476-493` `reconcileServer` checks `observed_generation` vs `desired_generation`.
- **Beacon:** `beacon/internal/server/manager.go:21` `PowerState Offline/Starting/Running/Stopping` + `30` `ServerState` fields `PowerState/InstallationState/StartupState/RunningAction/ExpectedStop/CrashCount`; `Reconcile:128` inspects runtime, recovers `ExpectedStop/LastStartedAt` from `stateDir/.beacon-state`.
- **Event:** `events.EventDesiredStateChanged` `EventActualStateChanged` `EventServerSuspended` etc (`clustermanager/service.go:401`).
- **Tests:** `store_heartbeat_test.go:35` expects 2 transitions.

**STATUS:** `COMPLETE` (superset — no ref had desired/actual)

**GAP:** Forge adds generation fencing + audit absent in all refs. UI still single badge; gap is observability, not logic. Power state persists across API restarts via DB and across daemon restarts via `dataDir/.beacon-state/manager.go:124`.

**FORGE LOGIC FINDING:** C-02-LF — `store_state.go:40` only advances `observed_generation` when `running↔running` or `stopped↔stopped`; `crashed`+desired `running` never converges — reconciler `service.go:476` will loop emitting `remediate:true` forever without progress. Also `ReconcileNode` publishes drift events but does **not** write `observed_generation` — external controller must apply.

**RECOMMENDATION:** `ADOPT` as canonical — unique strength. Fix fence to also advance on `installing→installed` or introduce `Crash→Stopped` desired alignment. Surface two-dot badge (`desired|actual`) like `Apps` health lanes.

**SEVERITY:** `P2 OPERATOR_VISIBLE` (reconciler livelock when crashed)

---

### C-03 — Suspend / Unsuspend

**REFERENCE**
- `pterodactyl-panel/app/Models/Server.php:125,218` `STATUS_SUSPENDED='suspended'`, `isSuspended():218` `status===suspended`; `database/migrations/2016_09_01_193520_add_suspension_for_servers.php:14` `suspended tinyint` later backfilled to `status` (`2021_01_17_152623:21`).
- `pelican-panel/app/Enums/ServerState.php:15` `Suspended`; `SuspensionService.php:31` throws if transferring; `AuthenticateServerAccess.php:53` blocks client API unless `routeIs('api:client:server.resources')` when suspended; Wings `server/server.go:283` disconnects WS on suspend flip; `server/update.go:56` suspend with running → `Terminate`.
- `pterodactyl-wings/server/power.go:177` `onBeforeStart` refuses start/restart if `IsSuspended()` after `Sync()`; `HandlePowerAction:57` blocks while transferring/restoring/installing.
- `pufferpanel` — **no** suspend concept; `servers/server.go:884` `IsIdle` only checks `IsInstalling/IsBackingUp/IsRestoring/IsRunning`.

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:102` `suspendServer/unsuspendServer` `POST /suspension {action}`; `console-view.tsx:125` disables power via `blocked` but not via permission — Beacon will reject.
- **API:** `forge/api/internal/http/handlers_servers.go:1421` `POST /servers/:id/suspension` (`action:suspend|unsuspend`) + legacy compat `POST /suspend`/`/unsuspend:1437,1450` — admin `servers.write`; best-effort `Daemon.SendPower stop` on suspend (`handlers_servers.go:1435`). Calls `store.SetServerSuspended:1444`.
- **Service:** `forge/api/internal/services/orchestrator/suspension.go:28` `SuspendServer()` verifies not already suspended, does `SetServerSuspension` then publishes `EventServerSuspended` (`73`); `clustermanager/service.go:394` `RequestServerPower` refuses `start|restart` if `Suspended`.
- **Store:** `forge/api/internal/store/store_servers.go:374` `SetServerSuspension` raw, `385` `CompareAndSetServerSuspension` CAS, `397` `SetServerSuspended` with audit. `Server` struct `store.go:454` `Suspended bool` orthogonal to `Status`.
- **DB:** `001_init.sql:14` no suspended; `007_postgres_core_foundation.sql:38` `suspended BOOLEAN DEFAULT FALSE`; never migrated to enum — intentional decoupled flag.
- **Worker:** none synchronous; suspension side-effect is best-effort stop.
- **Beacon:** `beacon/internal/server/manager.go:271` `BeginInstall` checks `Suspended` atomically under `TryLock`; `610` `onBeforeStart` snapshots `Suspended` and returns `"server is suspended"`; `HandlePower:494` also checks `InstallationState==installing` first; `Reconcile:135` syncs `Suspended` from panel; `syncServerStateFromPanel:748` refreshes `Suspended` from panel every `onBeforeStart` when `PanelURL` set.
- **Event:** `events.EventServerSuspended/EventServerUnsuspended` (`orchestrator/suspension.go:73,105` + `ws_hub.go:74`).
- **Tests:** `forge/api/internal/services/orchestrator/orchestrator_test.go:82` `TestSuspendServer`.

**STATUS:** `PARTIAL`

**GAP:** Forge keeps `suspended` Boolean orthogonal to `status` (good: preserves `install_failed`+`suspended`), whereas Pterodactyl/Pelican conflate `status='suspended'` wiping prior state. Missing: (1) Pterodactyl `AuthenticateServerAccess:53` blanket client-API gate when suspended (except `resources`) — Forge relies on per-handler permission checks; `ensureTransferIdle` does not check `Suspended` for power, only transfer. (2) `POST /suspension` does **not** call `ensureTransferIdle` — can suspend mid-transfer leaving `transfer_state=running` + `suspended=true` illegal combo. Pelican explicitly forbids. (3) `installing→installed` transition (`store_servers_control.go:226`) always writes `status='stopped'` even if `suspended=true` — badge shows `stopped`+suspended dual truth vs Pterodactyl `status=suspended` persistence.

**FORGE LOGIC FINDING:** C-03-LF — Race: `beacon/manager.go:689` `syncServerStateFromPanel` loads `state.Suspended` under lock at `611` then releases, fetches `PanelURL` without fence, re-acquires to write `Suspended` at `748` — concurrent `HandlePower start` can interleave and observe stale `Suspended=false` and start a suspended server. Also orphan `suspended=true` + `transferring=true` possible.

**RECOMMENDATION:** `ADAPT` — Keep orthogonal Boolean (superior), add `ensureTransferIdle` guard to `/suspension`, add client middleware 403 for suspended client ops except `GET /servers/:id/resources`, keep `stopped+suspended` dual badge (document intent) and add UI combined pill.

**SEVERITY:** `P1 OPERATOR_VISIBLE` (suspend bypass / illegal state combo)

---

### C-04 — Power Actions (start/stop/restart)

**REFERENCE**
- `pterodactyl-wings/server/power.go:24` `PowerAction` constants `start/stop/restart/kill` (`power.go:30`), `HandlePowerAction:56` — guards `IsInstalling||IsTransferring||IsRestoring` → `ErrServerIs*` (`power.go:57`), acquires `powerLock` (`power.go:108`), switch: `start` checks `State!=offline → ErrIsRunning` then `onBeforeStart:171` (Sync+suspend+env+disk+config+chown), `stop/restart` `WaitForStop(10m)` then optional `Start`, `kill` → `Terminate(SIGKILL)`.
- `pufferpanel/servers/server.go:185` `Start()` → `IsIdle()` + `isUnsafeRunning.TryLock()` else `ErrServerRunning` (`190`), `Stop:320`, `Kill:343` has no idle guard.
- No DB desired/actual; panel proxies via `DaemonPowerRepository`.

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:88` `sendPowerSignal`; `forge/web/components/server/console-view.tsx:125` `canPower` derives `control.start/restart/stop`, `blocked=suspended||transferring||installing`, idempotent `power.isPending` guard.
- **API:** `forge/api/internal/http/handlers_servers.go:888` `POST /servers/:id/power` → `ensureTransferIdle:889` → validate `start|stop|restart|kill` → permission `control.start|stop|restart` (`898`) → `idempotencyKey:908` → `OperationService.DispatchPower:915` (durable, deterministic `uuid.NewSHA1("forge-op:"+key)`) **or** `QueueService.DispatchIdempotent:939` fallback. Returns `202 {operationId,mode}` — fully async vs Wings sync.
- **Service:** `forge/api/internal/services/operation/service.go:351` `DispatchPower` maps signal→`server.start/stop/restart/kill`; `clustermanager/service.go:362` `StartServer/StopServer/RestartServer` → `sendPower:532` → `SetServerActualState` + `publishServerPowerEvents:576` (EventActualStateChanged + ServerStarted/Stopped/Restarted) then `runtime.Start/Stop/Restart/Kill`.
- **Store:** `forge/api/internal/store/store_servers_control.go:13` `SetServerPowerState` with `powerSignalPriorStates:44` (`start:[created,stopped,install_failed]`, `restart:[running,created,stopped,install_failed]`, `stop/kill:[running,installing,provisioning,restoring_backup,recovering,created,stopped]`). Updates `status`+audit. `store_state.go:165` helpers `serverDesiredFromSignal/serverActualFromSignal`.
- **DB:** `021_true_state_persistence.sql` desired/actual enums.
- **Worker:** `operation/service.go:152` 5 workers, `process:229` marks `running→handler→retry` (exp backoff 1s cap 30s maxRetries 3) or `succeeded/failed`; `reaper:164` reaps stale `running>5m`.
- **Beacon:** `beacon/internal/server/manager.go:482` `HandlePower` TryLock `RunningAction` slot (`487`), guard `InstallationState==installing:494`, per-signal: `start:511` `onBeforeStart:612` → `runtime.Start:402` → `PowerState=Running`; `stop:534` → `stopServer:440` → Offline; `restart:549` stop→Start; `kill:581` → `runtime.Kill:494`. `beacon/server.go:1349` `power` HTTP enqueues `operations.EnqueueCommand` and blocks up to 30s polling (`1399`) — sync-ish compat shim diverging from Forge 202 queue. `runtime/docker.go:388` `Start`, `434` `Stop`, `529` `Restart`.
- **Event:** `clustermanager:576` powers.
- **Tests:** `beacon/server_test.go:186` power rejects invalid signal.

**STATUS:** `PARTIAL`

**GAP:** Wings blocks `stop` during `IsInstalling/IsTransferring/IsRestoring` with 409 `ErrServerIsInstalling` etc (`power.go:57`). Forge `powerSignalPriorStates:51` **allows** `stop/kill` during `installing/provisioning/restoring_backup` — divergent intent (Forge allows interrupting install via stop/kill; Wings forbids except `kill` pierce). API `ensureTransferIdle:145` guards transfer only, not `installing/restoring`; so `POST /power start` during `installing` passes API, enqueues op, Beacon rejects at `HandlePower:494` → durable op retries 3× then `failed` — caller saw `202 accepted` then async failure; Wings returns `409` sync. Pufferpanel `IsIdle:884` would reject.

**FORGE LOGIC FINDING:** C-04-LF — Timeout divergence: Beacon `WaitForStop` 10m (`power.go:184` via `manager.go:440` `WaitForStop(30s)` actually) vs Forge operation reaper 5m → kill bypass divergence (see C-05). Also `stop` during `installing` will cancel installer container + mark `install_failed` but API docs do not state this intentional cancellation semantic.

**RECOMMENDATION:** `ADAPT` — Make allowlist intentional: keep `kill` allowed during `installing`, but `stop` should 409 while `installing` (matching Wells) unless installer cancellation is desired — document and enforce at API with `IsServerInstalling` guard alongside `ensureTransferIdle` (fast 409). Keep durable queue but surface sync 409 for `installing` before enqueue.

**SEVERITY:** `P1 OPERATOR_VISIBLE` (blocked-operation semantics / misleading 202)

---

### C-05 — Power `kill` Pierce-Lock (emergency SIGKILL bypass)

**REFERENCE**
- `pterodactyl-wings/server/power.go:108`:
  ```go
  if action != PowerActionTerminate { Acquire/TryAcquire with defer cleanup }
  else { // kill → Try Acquire, but ignore failure: "failed to acquire exclusive lock, ignoring failure for termination event" }
  ```
  Comment `power.go:81` “if server is currently trying to process a power action but has gotten stuck you still should be able to pass through the terminate event”. `kill` bypasses stuck `powerLock`.
- `pufferpanel/servers/server.go:343` `Kill()` has no lock attempt, directly `RunningEnvironment.Kill()`.

**FORGE**
- **Frontend:** `console-view.tsx:243` `kill` requires `confirm({Kill server?})` but same permission `control.stop` as stop.
- **API/Operation:** `operation/service.go:368` kill maps to `OpServerKill`; handler `clustermanager/service.go:377` `KillServer` → `sendRuntimePower` → `runtime.KillServer`; still queued behind FIFO durable worker.
- **Store:** `store_servers_control.go:51` kill allowed from 7 states but queuing still serializes.
- **Beacon Runtime:** `beacon/internal/runtime/docker.go:494` `Kill` does `workloadLock:251` sharded mutex (`[64]sync.Mutex` by `sha256(serverID)[0]`), **does not bypass** — contends same as `Stop/Start`. `manager.go:581` `HandlePower` `kill` still claims `RunningAction` slot (`487 TryLock` + `RunningAction!=""` check) — no bypass.
- **Beacon Worker:** `queue.go:63` `OperationQueue` concurrency 1-4, TTL/expiry; single slot.

**STATUS:** `MISSING`

**GAP:** Forge queues `kill` behind stuck install/power operation (worker poll 1s + `RunningAction` claimed). A hung `install` holding lock → `kill` waits for lock release or 5m reaper timeout, defeating emergency kill. Wings explicitly documents pierce; Forge has sharded `workloadLocks` mitigating container-level deadlock but `manager.RunningAction` still blocks kill on same server.

**FORGE LOGIC FINDING:** C-05-LF — `beacon/internal/server/manager.go:487` and `271` both use same `RunningAction` slot; `Kill` cannot pierce install. Concurrent `restart` during `installing` also blocked same slot but should be 409 distinct. Fix requires separate `kill` semaphore or TryLock-ignore-failure path for `signal==kill` mirroring Wings `115-119`.

**RECOMMENDATION:** `ADOPT` — Mirror Wings bypass: in `manager.go:HandlePower` and `operation/service.go:process`, if `signal==kill` try `TryLock` but don't fail if occupied; or keep kill on separate semaphore (`killMu`). Document `control.stop` still required. Add integration test hung-install + kill succeeds within 1s.

**SEVERITY:** `P1 USER_VISIBLE` (emergency kill fails when most needed)

---

### C-06 — Install Flow (first-time, explicit POST)

**REFERENCE**
- `pterodactyl-wings/server/install.go:33` `Install() → install(false)` → if not `SkipEggScripts` publish `InstallStarted`, `internalInstall()` (`GetInstallationScript → NewInstallationProcess → Run()`), then `SyncInstallState(err==nil,reinstall) → Environment.SetState(Offline)` → publish `InstallCompleted`. `Run:195` secures `installing.SwapIf(true)` guard, `BeforeExecute` (write script, pull image, remove old container), `Execute` (create `*_installer` container with mounts, tmpfs, resources, `Labels ContainerType=server_installer`), streaming via `DaemonMessageEvent`, wait, `AfterExecute` collects logs to `install.log`.
- `pufferpanel/servers/server.go:410` `Install()` → `IsIdle` guard, `SetInstalling(true)` defer false, `MkdirAll(root)`, `GenerateProcess(Installation).Run`.
- `pufferpanel-templates/spec.json:51` `install: []Operation` declarative.

**FORGE**
- **Frontend:** `forge/web/components/server/console-view.tsx:37` `InstallBanner` amber `Download` static text when `status==='installing'` (`236`); `builds-view.tsx:110` Build button permanently `disabled` with title claiming executor absent — honest decorative.
- **API:** `forge/api/internal/http/handlers_servers.go:1050` `POST /servers/:id/install` admin `servers.write`; `ensureTransferIdle:1051`; durable `DispatchInstall:1070` → `202 {operationId}`; fallback sync `clusterManager.InstallServer` 15m timeout.
- **Service:** `forge/api/internal/services/clustermanager/service.go:224` `runInstaller(reinstall bool)` → `SetServerInstallState installing:232` → `syncProvisionTarget:235` → `runtime.InstallServer:248` or `ReinstallServer`; on error `failed`, on success `installed` (only if `Accepted && ExitCode==0:254`).
- **Store:** `forge/api/internal/store/store_servers_control.go:218` `SetServerInstallState` state machine `installing→installed/failed` sets `status+timestamps install_started_at/install_completed_at/install_failed_at/install_error`.
- **DB:** `store_installer` tables `install_workflows/install_steps` (`installer/service.go:66` 6-step workflow `docker.create/filesystem.setup/download.server/script.install/config.apply/server.start` but **dead** — never executed, only persists row → `REF-GAME-HIDDEN-01`).
- **Worker:** `operation/service.go:474` `DispatchInstall` idempotent `uuid.NewSHA1("forge-op:server.install:"+key)`.
- **Beacon:** `beacon/internal/server/server.go:1052` `install` HTTP → validate, `MkdirAll`, `BeginInstall:1116` claim, `runtime.Install:1122` (`docker.go:256` creates `mgp-{id}-installer` bind-mounting `RootDir→/mnt/server`, `User 1000:1000`, `ReadonlyRootfs:true`, `CapDrop:ALL`, `no-new-privileges:true`, `Tmpfs /tmp 64M`, pulls image, waits, caps log 1MiB `340`, cleanup), `EndInstall:1131` (failed if err or non-zero), `notifyPanelInstallStatus:1132`; `installWS:1159` streaming WS **requires `ScopeAdmin:1178`** (prevents tenant console-token abuse). Installer image defaults `alpine:3.21` (`docker.go:262`).
- **Event:** `clustermanager` audit `server install installed/failed`.
- **Tests:** `beacon/internal/server/install_exclusivity_test.go:1`.

**STATUS:** `COMPLETE` (enhanced security & durability)

**GAP:** (1) Pterodactyl `SkipEggScripts` (`install.go:38`) skipped — Forge never checks `skip_scripts` (`store_servers.go:264` persists it, but Beacon `create` doesn't read `skip_scripts` from `server.json`; install always runs if called). (2) Installer log cap 1MiB (`docker.go:340`) vs Pterodactyl unlimited file. (3) `installer/service.go:66` workflow dead code persists rows never executed. (4) Non-WS `install` (`server.go:1052`) lacks admin-scope check vs `installWS` which gates `ScopeAdmin` — inconsistent auth.

**FORGE LOGIC FINDING:** C-06-LF — `installWS` gates `ScopeAdmin` but `POST /servers/:id/install` (non-WS) has no scope check beyond `requireRole("admin")` at panel — a stolen `ScopeServer` token replayed directly to Beacon bypasses WS gate via plain HTTP install (Beacon `install:1052` authenticates via HMAC/node token, not JWT scope). Also `chown` `1088` best-effort `os.Chown 1000:1000` only when `geteuid==0`, non-root daemons inherit root-owned files from installer if image runs as root — breaks SFTP.

**RECOMMENDATION:** `ADAPT` — Honor `skip_scripts` in `runInstaller` (early `installed` if true, matching `install.go:47`). Wire or delete `installer/service.go` dead workflow (remove until needed). Add scope check to HTTP `install` (reject non-admin JWT) or document that Beacon HMAC is sole gate (remove WS scope asymmetry).

**SEVERITY:** `P2 OPERATOR_VISIBLE` (skipped scripts divergence + auth asymmetry)

---

### C-07 — Reinstall Flow (requires stopped)

**REFERENCE**
- `pterodactyl-wings/server/install.go:80` `Reinstall() → if State()!=offline → WaitForStop(10s,true) else Sync() then install(true)`; `ServerManagementController.php:55` `reinstall → ReinstallServerService` → sets `status=reinstall_failed`.
- `pelican-wings` same but distinct `ReinstallFailed` enum.
- `pufferpanel` no distinct reinstall — `Install()` same (stops first `426`).

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:98` `reinstallServer` `POST /reinstall`; `console-view.tsx:45` banner but no guard disabled when running (relies on Beacon 409).
- **API:** `forge/api/internal/http/handlers_servers.go:1110` `POST /servers/:id/reinstall` requires `PermSettingsReinstall` (subusers can trigger) vs `install` requires admin; `ensureTransferIdle`; durable `DispatchReinstall:1129` or `clusterManager.ReinstallServer` sync. No stopped-state pre-check at API.
- **Service:** `clustermanager/service.go:220` `ReinstallServer → runInstaller(true)` → same path but calls `runtime.ReinstallServer` if `Reinstaller` interface (`241`); Docker runtime **does not implement** `Reinstall` → Forge errors `"runtime does not support reinstall":243` and marks `failed`. Beacon `Reinstall` works (just calls `install`) but API drift.
- **Store:** `store_servers_control.go:218` same state machine.
- **Beacon:** `beacon/internal/server/server.go:1331` `reinstall` guard `PowerState==Running|Starting → 409 "server must be stopped"` (`1335`), then forwards to `install` (`BeginInstall` claim). `manager.go:549` `restart` already stops; reinstall must be explicit stop first.
- **Event:** same install audit.

**STATUS:** `BROKEN`

**GAP:** Reference panels treat reinstall as same installer flow (Pterodactyl `install(true)`). Forge introduces separate `OpServerReinstall` but `clustermanager:242` `Reinstaller` interface gate makes pure-Docker Reinstall always fail 500 (carrier `F-G-07`). Beacon `reinstall` works (just calls `install`): API vs Beacon drift. Also `POST /reinstall` requires `PermSettingsReinstall` (subuser) while `install` requires admin — matches Pterodactyl app-level admin vs server-owner? Pelican Filament disables reinstall when suspended — consistent.

**FORGE LOGIC FINDING:** C-07-LF — Reinstall on Docker will always `failed` via `clustermanager` path but succeed via Beacon direct; `SetServerInstallState failed` persists `status='install_failed'` leaving server unusable until admin manually toggles. Should fallback to `InstallServer` when `Reinstaller` not implemented (Beacon behavior) or implement `Reinstall` alias in `DockerRuntime`.

**RECOMMENDATION:** `ADOPT` reference — Implement fallback: `if !ok → response, err = s.runtime.InstallServer(...)` in `clustermanager:241`. Add API 409 instructing `stop first` consistently (like Beacon) rather than marking `install_failed`.

**SEVERITY:** `P1 USER_VISIBLE` (reinstall always fails on default runtime)

---

### C-08 — Delete (hard-delete, Beacon-first, force & orphan remediation)

**REFERENCE**
- `pterodactyl-panel` soft-deletes? `DropDeletedAt` migration + `whereNull('successful')` transfer; `ServerDeletionService` stops server, removes allocations, `DaemonServerRepository->delete`, hard delete row, cascade `server_variables`, `allocations.server_id=NULL`.
- `pterodactyl-wings/router/router_server.go` `DELETE /api/servers/{uuid}` removes container + files via `Server.Delete()`.
- `pufferpanel/servers/server.go:373` `Destroy()` → `IsIdle` guard, scheduler stop, `Uninstallation` ops, `RunningEnvironment.Delete()`.
- No orphan remediation table — failures silent.

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:74` `deleteServer(id,force)` maps force→`?force=true`; UI confirm gate.
- **API:** `forge/api/docs/server-lifecycle.md:6` Beacon-first, hard-delete only after success; `?force=true` always removes panel state; failure → orphan. `handlers_servers.go:1461` `DELETE /servers/:id` → `clusterManager.DeleteServer:332`.
- **Service:** `clustermanager/service.go:332` `DeleteServer`: `ServerControlTarget` → `runtime.DeleteServer`; on error + `!force` return; on error + `force` → `RecordOrphanAndHardDeleteServer:348` + `Mode:force` with daemonErr; on success → `HardDeleteServer:355`. Publishes `EventServerDeleted` (`352,358`) with `force/orphaned`.
- **Store:** `forge/api/internal/store/store_servers_lifecycle.go:12` `HardDeleteServer` → tx `FOR UPDATE` lock, `UPDATE allocations SET server_id=NULL` (`46`), `DELETE FROM servers` cascade. `17` `RecordOrphanAndHardDeleteServer` inserts `server_orphan_remediations` **before** delete (FK-free) — requires `daemonError != ""` (`19`).
- **DB:** `forge/api/migrations/040_truthful_server_lifecycle.sql:18` `server_orphan_remediations(id,server_id,node_url,daemon_error,status=pending|resolved)` + index `pending`; orphan rows accrue silently — no `GET /orphans` UI (`handlers_admin.go` lacks).
- **Worker:** none single path.
- **Beacon:** `beacon/internal/server/server.go:1425` `delete` → `safePath`, `consoles.Stop`, `runtime.Delete:1429` (`docker.go:625` `ContainerRemove Force+RemoveVolumes`, idempotent `IsNotFound→nil`), `os.RemoveAll(root)`, `manager.Delete:1478`.
- **Event:** `EventServerDeleted`.
- **Tests:** `store_servers_lifecycle_integration_test.go`.

**STATUS:** `COMPLETE` (enhanced — orphan tracking absent in refs)

**GAP:** Pufferpanel `Uninstallation` ops (custom cleanup scripts) not run — Forge delete does not run egg `install_script` unwind or `mount_server` cleanup beyond `RemoveVolumes:true`. Pterodactyl `transfer` cleanup deletes allocations from old node — separate.

**FORGE LOGIC FINDING:** C-08-LF — `store_servers_lifecycle.go:43` `UPDATE servers SET primary_allocation_id=NULL` before `UPDATE allocations SET server_id=NULL` ordering is correct inside Tx, but `HardDeleteServer` sets `primary_allocation_id=NULL` even if allocations already `NULL` — benign. More relevant: `RecordOrphanAndHardDeleteServer` requires non-empty `daemonError`; `compensateCreateFailure:292` passes `deleteErr.Error()` correctly, but `DeleteServer` with `runtime==nil` passes `gpruntime.ErrRuntimeUnavailable` — creates orphan remediation even though no workload existed (phantom orphan). Also no admin resolution UI → `REF-GAME-HIDDEN-02`.

**RECOMMENDATION:** `ADAPT` — Keep orphan remediation (novel), add `GET /admin/orphans` UI to resolve pending rows, fix phantom orphan when `runtime==nil` (just `HardDeleteServer`).

**SEVERITY:** `P3 OPERATOR_VISIBLE` (orphan table silent accumulation)

---

### C-09 — Backup Restore Lock (`restoring_backup`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Server.php:126` `STATUS_RESTORING_BACKUP`, `Server.php:393` `validateCurrentState()` throws `StateConflict` if `restoring_backup` or `transfer!=null`; `BackupController.php:200` cannot restore unless `isInstalled && backup is_completed && !failed`; sets `status=restoring_backup` then daemon `restore`.
- `pterodactyl-wings/server/install.go:152` `IsRestoring` atomic, `HandlePowerAction:57` blocks if `IsRestoring → ErrServerIsRestoring`; Wings `backup.go` restore path sets `restoring true`.
- `pufferpanel/servers/server.go:737` `StartRestore` → `IsIdle` guard, `restoring=true`, `Extract` archive, `restoring=false`; `IsRestoring:879` bool blocks `IsIdle`.
- `pelican-panel` same.

**FORGE**
- **Frontend:** `forge/web/components/lifecycle/lifecycle-machines.ts:55` `BACKUP_MACHINE` steps queued/creating/archiving/completed but no `restoring` integrated with console `blocked` logic.
- **API:** `forge/api/internal/http/handlers_servers.go:2104` `/servers/:id/backups/restore` → `resolveBackupName`, check `status==completed`, then `OperationService.DispatchBackupRestore` **or** sync `MarkBackupStatus restoring:2170` (`store_backups.go`) — per-backup row, **not** server row; no `SetServerActualState restoring_backup` (`store_state.go:131` mapping exists but never driven by restore). `ensureTransferIdle` guards transfer only.
- **Service:** no restore → server state transition; `clustermanager` power `RequestServerPower` checks `Suspended` but not `restoring_backup`.
- **Store:** `store_state.go:131` `serverStatusFromActual restoring_backup → restoring_backup` ↔ `store.go:1479` but never set by backup routes; `backups.status=restoring` only.
- **DB:** `019_backups.sql` backup rows, no trigger to server `status=restoring_backup`.
- **Beacon:** `beacon/internal/server/server.go:1691` `restoreBackup` creates `pre-restore-*.zip` snapshot + `backupMu` serialize, but `manager.go` has **no** `Restoring` state (only `InstallationState`).
- **Event:** none for server-level restoring lock.

**STATUS:** `MISSING` (per earlier `F-G-06` — never wired)

**GAP:** All 4 games-hosting refs treat `restoring_backup` as server-level exclusive lock (blocks power, suspend, reinstall, transfer). Forge tracks per-backup `status=restoring` but never flips `servers.actual_state/status`. So `POST /power start` or `POST /servers/:id/install` during restore succeeds at API (only `ensureTransferIdle` guard) diverging from `validateCurrentState`. Pufferpanel `IsRestoring` correctly blocks `IsIdle` for `Install/Start/Destroy`; Forge lacks.

**FORGE LOGIC FINDING:** C-09-LF — Concurrent `start` + `restore` allowed → restore `Extract` + container `Start` race on same `RootDir` (zip bomb + running process reading half-written files). Also `handlers_servers.go:2104` marks `backups.status=restoring` but if daemon `restoreBackup` fails, `MarkBackupStatus restore_failed` runs but server-level `restoring_backup` would remain stuck if it existed — needs clear-on-completion.

**RECOMMENDATION:** `ADOPT` reference — On `DispatchBackupRestore` also `SetServerActualState restoring_backup` (`store_state.go:31`) and clear (`stopped` or prior) on completion/failure; add `ensureRestoreIdle` guard alongside `ensureTransferIdle` for `power/install/reinstall/suspend`.

**SEVERITY:** `P1 USER_VISIBLE` (data corruption race, spec violation)

---

### C-10 — Transfer / Migration (node-to-node) — legacy Wings transfer vs control-plane-mediated v1

**REFERENCE**
- `pterodactyl-panel/app/Models/ServerTransfer.php:26` row `old_node/new_node/old_allocation/new_allocation/additional`, `successful NULL|true|false`, `archived`; `Admin/ServerTransferController.php:39` reserves new allocations, generates JWT `ServerTransfer` scope token, notifies source daemon `notify(newNode,token)`, daemons talk peer-to-peer (archive push). `validateTransferState:409` blocks if `!isInstalled` or `restoring_backup` or `transfer!=null`.
- `pterodactyl-wings/server/transfer.go` + `router/router.go:59` `postTransfers`.
- `pelican-panel/wings` same but token hash, keep `status` enum.
- `pufferpanel` — no transfer.

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:394` `fetchServerTransferStatus/cancelServerTransfer/transferServer` targets Forge `MigrationService` facade; `transfer-view.tsx:31` polls `GET /servers/:id/transfer` every 5s if `transferring`; `transfer-view.tsx:77` `isTransferring = server.transferring` vs `transfer.transferring` flicker (`REF-GAME-DUP`).
- **API:** `forge/api/docs/server-lifecycle.md:12` claims "Server transfer/archive execution is intentionally unavailable and returns `501`". Yet `handlers_servers.go:713` `POST /servers/:id/transfer` (admin, via `MigrationService.CreateMigration+ExecuteMigration → 202`), `GET /transfer:740` (`state,transferring,targetNodeId,error` from `servers` row), `POST /transfer/cancel:758` (finds active `migrations.status not in completed/failed/cancelled`). Legacy routes `legacyServerTransferUnavailable:130` returns `410 Gone`.
- **Service:** `forge/api/internal/services/migration/service.go:127` `CreateMigration` + `227` `PrepareMigration` ensures `migration_runs` with `TransferProtocolVersion`, `249` `ExecuteMigration` async `startRun`; `run:417` claims `MigrationRun` lease (`ClaimMigrationRun` 2m), generates scoped credentials (`TransferCredentialClaims Version=TransferProtocolVersion`), registers on both daemons `RegisterTransferCredential` + `FinalizeTransferDestination` + `CleanupTransferSource`. `migration/service.go:102` `TransferProtocolVersion = "forge-beacon-transfer/v1"` + `allocation_reservations`.
- **Store:** `store_migrations.go` + `migration_runs` (`045_real_server_transfer.sql:1` lease `lease_owner/lease_expires_at`, `phase`, `cleanup_pending`, `allocation_reservations`). `servers` row still has legacy `transfer_state/transferring/transfer_target_node_id/transfer_run_token` but `040_truthful_server_lifecycle.sql:7` clobbers stale `queued/running→failed` on startup; modern `migrations` not projected to `servers.transfer_state` → dual state split makes `GET /transfer` read stale column (`REF-GAME-HIDDEN-02`).
- **DB:** `004_server_transfer_state.sql:1`, `005_server_transfer_lifecycle.sql:1`, `045_real_server_transfer.sql:1`.
- **Beacon:** `beacon/internal/server/server.go:368` protocol v1 `/api/v1/transfers/credentials` (`registerTransferCredential`), `transfer_protocol.go` + `transfer.Engine/Manager` (`server.go:83`), peer-to-peer via `prepareTransferSource/pushTransferSource/receiveTransferChunk/restoreTransferDestination` (`server.go:388`).
- **Event:** `migration/service.go:171` `EventMigrationCreated`.
- **Tests:** `store_migration_transfer_integration_test.go`.

**STATUS:** `PARTIAL`

**GAP:** Doc says 501 but handler returns 202 via `MigrationService` — doc outdated. Traditional Wings transfer (reserve→token→peer push) replaced by migration engine (`migration_runs` + `allocation_reservations` + hash-pinned credentials). `servers.transfer_state` legacy shim superseded but still drives `ensureTransferIdle:145` guards and `GET /transfer`; modern `migrations.status` (`planned/preparing/running/completed/failed/cancelled`) not projected. So `ensureTransferIdle` reading `servers.transfer_state` will miss active `migrations` row; but `GET /transfer` also reads stale `servers` row. Pterodactyl allowed one concurrent transfer per server (`whereNull('successful')`); Forge `migration/service.go:342` allows multiple migrations per server up to global `maxConcurrentWorkers`.

**FORGE LOGIC FINDING:** C-10-LF — Split brain: `servers.transferring` (boolean) vs `migrations` table vs `transferState` string vs `migration_runs.phase` — `transfer-view.tsx:77` flicker emerald/warning due to cached `server.transferring` vs polled `transfer.transferring` distinct sources (`phase-02/subagent-04` F-G-19).

**RECOMMENDATION:** `ADAPT` — Keep control-plane-mediated v1 (superior to Wings JWT push: resumable chunks, HMAC-scoped single-use credentials). Retire or sync legacy `servers.transfer_state` from `migrations` status (or make `ensureTransferIdle` query `migrations` table). Update `server-lifecycle.md` to reflect migration engine as canonical transfer (remove 501 note). Unify `transferring` source in `transfer-view`.

**SEVERITY:** `P2 OPERATOR_VISIBLE` (stale transfer guard / doc lie / UI flicker)

---

### C-11 — Allocations (IP/port + `containerPort`/`protocol`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Allocation.php` + `database/migrations/2016_01_23_195641_add_allocations_table.php:13` `allocations(ip,port,node_id,assigned_to nullable)`; single IP+port range creation; `Server.php:162` `allocation_id` FK unique.
- `pterodactyl-wings/environment/allocations.go:36` `Mappings map[string][]int` — **no per-port protocol**; `Bindings()` creates both `tcp` and `udp` for every port (`allocations.go:54`).
- `pufferpanel` no pool — `portBindings: ["0.0.0.0:${port}:${port}/tcp"]` per environment (`pufferpanel-templates/valheim/valheim.json:77`).

**FORGE**
- **Frontend:** `forge/web/lib/api/servers.ts:634` allocation assign, `forge/web/app/admin/allocations/page.tsx:1` (not inspected deeply but exists).
- **API:** `forge/api/internal/http/handlers_admin.go:1365` `GET /allocations/nodes`, `1378` `GET /allocations?limit&offset` paginated 50, `handlers_servers.go:454` `GET /servers/:id/allocations`, `634` `POST /allocations` + `DELETE` + `POST .../primary`.
- **Service:** none specific; store transactional.
- **Store:** `forge/api/internal/store/store_allocations.go:15` `Allocation{IP,Port,ContainerPort,Protocol,Alias}`; `125` `CreateAllocations` bulk Tx with `pg 23505` duplicate handling (`182`), validates `net.ParseIP`, `protocol ∈ {tcp,udp} default tcp`, `containerPort` default `port` (`162`); `214` `DeleteAllocations` fails entire batch if assigned; `327` `AssignAllocationToServer` loads server `node_id` vs allocation `node_id` + `already assigned` check; `367` `SetPrimaryAllocation` Tx `FOR UPDATE`.
- **DB:** `007_postgres_core_foundation.sql:134` `allocations(ip inet, port int, container_port int, protocol text)`.
- **Beacon:** `beacon/internal/runtime/runtime.go:100` `PortBinding{HostIP,HostPort,ContainerPort,Protocol}`; `beacon/internal/runtime/docker.go:865` `dockerPorts` validates `tcp|udp`, duplicate `HostIP:HostPort/Protocol` detection (`888`), `HostIP` `net.ParseIP`, maps to `nat.PortSet/PortMap`.
- **Service/Client:** `forge/api/internal/daemon/client.go:438` `Port{HostIP,HostPort,ContainerPort,Protocol}` + `ServerConfiguration.Allocations.Mappings` compat; `clustermanager/service.go:655` `runtimeCreateRequest` maps `allocation.ContainerPort` (0→HostPort) + `protocol` default tcp.
- **Event:** audit `allocation assigned/unassigned/primary set`.
- **Tests:** `store_allocations` integration implicitly via `CreateServer`.

**STATUS:** `COMPLETE` (superset — explicit `protocol`+`containerPort` vs Wings dual)

**GAP:** Forge correctly models `protocol` per allocation vs Wings `Mappings` which encodes both tcp+udp via `Bindings()`. Forge fallback `Mappings map[string][]int` (`clustermanager/service.go:698` + `client.go:460`) drops `protocol` when using legacy path → UDP-only `8211` also binds tcp (`F-G-11`). Game-templates static `ports` (`minecraft-paper.json:33` `25565 tcp public`) not auto-provisioned to pool — must be pre-created via admin.

**FORGE LOGIC FINDING:** C-11-LF — `store_allocations.go:178` duplicate detection only on `ip+port` but not `protocol` — inserting same `ip:port` with `tcp` then `udp` will hit `23505` if unique index is `(node_id,ip,port)` without protocol (verify migration `090_allocation_transport.sql`). Forge code assumes protocol-distinct rows but DB constraint may reject.

**RECOMMENDATION:** `ADOPT` — explicit `protocol`+`containerPort` is strictly better than Wings dual binding. Fix DB unique index to `(node_id,ip,port,protocol)` if not already; remove or fix legacy `Mappings` fallback to encode protocol or drop it.

**SEVERITY:** `P2 SILENT` (UDP ports expose tcp silently via compat; DB uniqueness)

---

### C-12 — Resource Limits (`cpu`/`mem`/`disk`/`io`/`swap`/`oom`/`threads`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Server.php:153` validation `memory required numeric min:0`, `swap min:-1` (`-1=unlimited`), `cpu percent 0..THREADS*100`, `io 10..1000`, `threads regex /^[0-9-,]+$/`, `oom_disabled sometimes boolean default true`.
- `pterodactyl-wings/environment/settings.go:37` `Limits{MemoryLimit, Swap, IoWeight, CpuLimit, DiskSpace, Threads, OOMDisabled}` with helpers `ConvertedCpuLimit()`, `BoundedMemoryLimit()`, `ConvertedSwap()`, `AsContainerResources()` (BlkioWeight gated on cgroup v2 `116`).
- `pufferpanel` `requirements{os,arch,binaries}` only — no cpu/mem sizing at template level.

**FORGE**
- **Frontend:** `packages/shared-types/src/api.ts:98` `cpuShares/cpuLimit/threads` + `forge/web/lib/api/servers.ts` creation form.
- **API:** `forge/api/internal/http/handlers_servers.go:789` `POST /servers` maps `cpuShares default 1024, cpuLimit default 0, disk 10240, ioWeight 500, swap 0` with validation `memoryMB<=0||cpuShares<=0||cpuLimit<0...` (`827`).
- **Service:** `clustermanager/service.go:632` `runtimeCreateRequest` maps `MemoryMB, SwapMB, CPUShares, CPULimit, DiskMB, IOWeight, Threads, OOMDisabled`; `service.go:513` `ResizeServer` via `UpdateServer`.
- **Store:** `store_servers.go:209` `if MemoryMB<=0||CPUShares<=0||CPULimit<0||DiskMB<=0...SwapMB<-1||IOWeight 10..1000 → error`; `store_servers_control.go:96` loads `s.cpu_shares,s.cpu_limit` for provision target; `store_allocations.go` not relevant.
- **DB:** servers columns `memory_mb, cpu_shares, cpu_limit, disk_mb, swap_mb, io_weight, threads, oom_disabled` (`001_init.sql`).
- **Beacon:** `beacon/internal/runtime/docker.go:901` `validateCreateRequest` enforces `MemoryMB<0, SwapMB<-1, Swap>0&&Memory==0→error, CPUShares 2..262144, CPUPercent 0..100000 (999), IOWeight 10..1000, PIDLimit, UID/GID` etc; `783` `buildResources` converts `CPUPercent→CPUQuota=period*percent/100` (period 100000), `MemorySwap=memory+swap`, `CPUShares` weight, `BlkioWeight`, `OomKillDisable`, `PidsLimit`; `827` `buildHostConfigWithSettings` applies.
- **Event:** none.
- **Tests:** `beacon/internal/server/runtime_handlers_test.go` implicitly.

**STATUS:** `PARTIAL`

**GAP:** Three CPU concepts conflated (`F-G-09`): (1) Pterodactyl `cpu` percent `0..threads*100` → `CpuLimit` → `CPUQuota`, (2) Forge `CPUShares` weight `2..262144`, (3) `CPUPercent/CPULimit` percent. `store_servers.go:209` validates `CPULimit<0` but not cap `100000` unlike beacon `docker.go:924` (`0..100000`); `store` allows `CPULimit=0` (unlimited) with `CPUShares=1024` still set — beacon `docker.go:927` still sets weight `1024` diverging from `settings.go:124` which only sets `CPUShares=1024` when `CpuLimit>0`. `Daemon CreateRequest` has both `CPUShares` and `CPUPercent` (`daemon/client.go:410` + `runtime.go:65`) mapping ambiguous.

**FORGE LOGIC FINDING:** C-12-LF — Unlimited CPU still sets weight `1024` → container throttled vs Wings unlimited (`CPUQuota 0` means no limit, weight irrelevant but may affect cgroup v1 share). Also `SwapMB` validation allows `0` unlimited but `docker.go:914` check `SwapMB>0 && MemoryMB==0→error` correct.

**RECOMMENDATION:** `ADAPT` — Align validation (`CPULimit 0..threads*100` → `CPUPercent 0..100*100? Actually 0..100000` is centi-percent, clarify units). Gate `CPUShares` on `CPULimit>0` like `settings.go:124` (`if CpuLimit>0 then CPUShares else 0`). Unify `CpuLimit` vs `CpuPercent` naming.

**SEVERITY:** `P2 SILENT` (wrong throttling, not crash)

---

### C-13 — Egg / Nest Data Model

**REFERENCE**
- `pterodactyl-panel/app/Models/Nest.php:19` `nests(id,uuid,author,name)` hasMany eggs `54`.
- `pterodactyl-panel/app/Models/Egg.php:52` `eggs(id,uuid,nest_id,author,name,docker_images JSON,startup,config_files/startup/logs/stop,file_denylist,script_install/container/entry,file_denylist,features,force_outgoing_ip,copy_script_from,config_from, EXPORT_VERSION PTDL_v2)` (`68`), validation `docker_images.* regex`.
- `pelican-panel/app/Models/Egg.php:1` adds `tags, startup_commands, children, mounts, HasIcon, Validatable` strict.
- No Forge; Puffer `spec.json` `id?, type, display` not slug-enforced.

**FORGE**
- **Frontend:** `forge/web/app/admin/nests/page.tsx:1` `AdminNestsEggs` aggregates nests+eggs one view; `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1` CRUD (name/description/dockerImages newline list, startup, stop, features, installScript/container/entrypoint) + `EggCard` with `Settings/Copy/Download/Trash` and `Browse Templates →` CTA; `variables/page.tsx`.
- **API:** `forge/api/internal/http/handlers_admin.go:990` `GET /nests`, `1003` `GET /nests/:id`, `1016` `POST /nests`, `1043` `PATCH /nests`, `1067` `DELETE /nests` with `eggCount>0→400`; `1083` `GET /eggs`, `1098` `GET /nests/:nestId/eggs`, `1111` `GET /eggs/:id`, `1124` `POST /eggs`, `1156` `PATCH /eggs/:id`, `1270` `DELETE /eggs/:id`, `1288` `/eggs/:id/export`, `1308` `/eggs/import`.
- **Service:** none specific; store Tx.
- **Store:** `forge/api/internal/store/store_nests.go:16` `Nest{Name,Description,EggCount}`, `34` `Egg{DockerImages json.RawMessage, Startup, Config, DefaultMemoryMB, InstallScript…, FileDenylist, UpdateURL, Features…}`; `100` `ListNests`, `137` `CreateNest` (unique name), `172` `DeleteNest` (reject if eggs), `190` `ListEggs`, `248` `CreateEgg` via `normalizeDockerImages:361` (accepts `map<string,string>` **or** legacy `[]string` array), `361` robust.
- **DB:** `forge/api/migrations/007_postgres_core_foundation.sql:87` `nests(id UUID PK, name TEXT UNIQUE, description)`, `eggs` table `default_memory_mb`, `043_unify_eggs_templates_mounts.sql:63` migrates `server_templates→eggs` preserving UUIDs, `089_egg_parity_fields.sql` adds `config_from, copy_script_from, update_url, features, startup_commands`.
- **Beacon:** `ServerProvisionTarget:88` loads `e.docker_images, e.startup, e.config, e.file_denylist, e.install_*` for runtime; `docker.go:901` validates image non-empty.
- **Event:** audit `nest created/egg created`.
- **Tests:** `forge/api/internal/store/store_eggs_integration_test.go`.

**STATUS:** `COMPLETE`

**GAP:** Forge drops `uuid/author` columns on nests (uses plain UUID PK), more permissive `normalizeDockerImages` handles both `map` and `[]string` (improvement vs panel map-only after 2022). Forge correctly prefers server override else lexicographically-first image (`store_nests.go:99` `store_startup.go:14` `COALESCE(NULLIF(s.docker_image,''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1)`).

**FORGE LOGIC FINDING:** C-13-LF — `DeleteNest` counts eggs via `SELECT COUNT(*)` without `FOR UPDATE` — race where concurrent `CreateEgg` after count but before `DELETE` violates FK or orphans egg (FK `nest_id` with `RESTRICT` would error, but Forge migration may have `CASCADE` — check `043`). Low risk single admin path.

**RECOMMENDATION:** `ADOPT` — `normalizeDockerImages` handling both shapes is strictly better. Keep.

**SEVERITY:** `P4 SILENT`

---

### C-14 — Egg Variables / Startup Templating + Variable Validation

**REFERENCE**
- `pterodactyl-panel/app/Models/EggVariable.php:29` `egg_variables(id,egg_id,name,env_variable,default_value,user_viewable,user_editable,rules)`; `RESERVED_ENV_NAMES = SERVER_MEMORY,SERVER_IP,SERVER_PORT,ENV,HOME,USER,STARTUP,SERVER_UUID,UUID` (`43`), regex `^[\w]{1,191}$` `notIn:RESERVED`.
- `pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:28` filters `user_editable && user_viewable` when not admin, builds `rules['environment.'+env_variable]=variable->rules` and runs Laravel `ValidationFactory`.
- `pterodactyl-panel/app/Services/Servers/StartupModificationService.php` tests verify sync.
- `pterodactyl-wings/server/server.go:151` `GetEnvironmentVariables()` injects fixed `TZ,STARTUP,SERVER_MEMORY,SERVER_IP,SERVER_PORT` then appends `EnvVars` upper-cased skipping duplicates.
- Templates ship `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` (`packages/game-templates` paper).

**FORGE**
- **Frontend:** `forge/web/app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx:1` with `fetchEggVariables/create/update/delete/reorder` (`forge/web/lib/api.ts:1340`).
- **API:** `handlers_admin.go:1185` `GET /eggs/:id/variables`, `1198` `POST .../variables`, `1226` `PATCH ...`, `1254` `DELETE`; `handlers_servers.go:355` `PATCH /servers/:id` startup updates, `store_startup.go` path.
- **Service:** `clustermanager/service.go:632` `runtimeCreateRequest` injects `SERVER_MEMORY/SERVER_IP/SERVER_PORT` + egg variables; `runtimeInstallRequest:681` same.
- **Store:** `store_egg_variables.go:17` `EggVariable{Name,EnvVariable,DefaultValue,UserViewable,UserEditable,Rules,Sort}`; `129` `validateEggVariableRequest` pattern `^[A-Z][A-Z0-9_]*$` (`15`), `142` `validateVariableValue()` implements `required|nullable|string|max|min|in|regex`; `store_startup.go:11` `GetServerStartup` joins `egg_variables` filtered `user_viewable=true` (`31`), injects `env[EnvVariable]=ServerValue` then `StartupCommand=resolveStartupCommand(RawStartupCommand, env)` (`50`); `54` `UpdateServerStartupVariable` checks `user_editable`, validates, `INSERT ON CONFLICT UPDATE` + `config_sync_pending=true`. `store_servers.go:273` also validates `StartupVariables` at creation via same helper. `store_servers_control.go:154` `ServerProvisionTarget` loads **all** variables (no viewable filter) for runtime (correct for daemon but inconsistent with UI filter).
- **DB:** `013_startup_variables.sql:26` seed `required|string|max:64`.
- **Worker:** `reconciler` syncs `config_sync_pending`.
- **Beacon:** `docker.go:1003` `buildContainerConfig` injects `SERVER_MEMORY` etc as `Env`; `runtime.go:55` `CreateRequest Env []string`.
- **Tests:** `store_eggs_integration_test.go` but no variable validator regex test covers slash case.

**STATUS:** `BROKEN`

**GAP:** Critical validator bug (`F-G-08`): `store_egg_variables.go:176` splits `rules` on `|` then `strings.Cut(rule,":")` → for `regex:/^([\w\d._-]+)(\.jar)$/` yields `name=regex, arg=/^([\w\d._-]+)(\.jar)$/` including surrounding `/`. Then `regexp.Compile(arg)` treats slashes literally — `server.jar` fails (must be `/server.jar/`). Also naïve `Split("|")` splits regex alternation `|` inside pattern (e.g., `regex:/^(foo|bar)$/`). Valid PTDL_v2 eggs (Paper) cannot be imported via `POST /eggs/import:1308` loop that calls `CreateEggVariable` → 400. Additionally Forge has **no reserved-names list** — relies on pattern not exposing `SERVER_*` but could.

**FORGE LOGIC FINDING:** C-14-LF — `validateVariableValue:142` also does not handle Laravel `regex:/.../flags` (`/i` etc) and does not anchor; `max/min` counts runes (`len([]rune)`) whereas Laravel counts string length bytes? Divergence subtle but incorrect for unicode seeds. `store_servers_control.go:154` `ServerProvisionTarget` injects all variables (including `user_viewable=false` `DL_PATH`) while `store_startup.go:58` rejects non-viewable updates, and `store_servers.go:273` CreateServer allows any variable regardless of viewable — non-admin can set `DL_PATH` at provision to hijack installer URL (`F-G-10`).

**RECOMMENDATION:** `ADAPT` — Strip `regex:/…/` delimiters and optional flags before `Regexp.Compile`, and respect `|` inside regex char classes by parsing rules with a Laravel-compatible splitter (e.g., split on `|` not inside `regex:`). Add `RESERVED_ENV_NAMES` check (block `SERVER_*`, `ENV`, etc) and gate creation-time variables by `user_editable/viewable` for non-admin.

**SEVERITY:** `P0 USER_VISIBLE` (blocks all PTDL imports + privilege boundary)

---

### C-15 — Templates Dualism + Seeding (DB eggs vs FS `game-templates` vs `app-templates` localStorage)

**REFERENCE**
- `pterodactyl-panel` — single `eggs` table is canonical, no FS catalog; `PTDL_v2` export is transfer format.
- `pufferpanel-templates/spec.json:1` — JSON Schema 2020-12 strict `additionalProperties:false`; 44 JSON payloads (36 game types) with `data.json` seed variants per game (`minecraft/data.json:2`).
- Forge reference `pufferpanel-templates` is superset (36 types).

**FORGE**
- **Frontend:** `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries (minecraft-paper:15 etc) — hardcoded TS, IDs mirror `packages/game-templates` (deliberate). No validator. `forge/web/lib/app-templates-data.ts:3` `STORAGE_KEY forge.app-templates.v1` with `AppTemplate{type:image|git|compose}` 5 defaults (`app-templates/page.tsx:1` `formToTemplate` freeform `host:container` CSV) — **entirely localStorage**, never touches DB (`api/apps.ts:369` fallback).
- **API:** `store_templates.go:12` `ListTemplates/GetTemplate/CreateTemplate` compat shim over eggs (`templateFromEgg:99`); `handlers_admin.go:2037` `/templates` routes delegate to `Store.ListTemplates`. No seed from FS.
- **Service:** `catalog/catalog.go:1` `CatalogEntry` DB-driven for DBs/caches; `appstore/service.go:1` + `seed.go:27` 7 compose apps (`SeedDefaultApps` on `cmd/api/main.go:529` upserts). **No** `SeedGameTemplates`.
- **Store:** `store_templates.go:48` `CreateTemplate` uses `Games` nest fallback; `store_nests.go:276` `normalizeDockerImages`.
- **DB:** `007`: `Games` nest inserts `dddd...`; `043_unify`: `Legacy Templates` nest backfills; `091_seed_minecraft_java.sql:3` inserts **1** egg `Minecraft Java` variant (`itzg/minecraft-server:java21`, empty `startup`, 2 vars `VERSION/TYPE`) — **only** production egg seed; fresh install has 1 egg, not 14.
- **Worker:** none.
- **Beacon:** `runtimeInstallRequest` uses DB egg, not FS template.
- **FS Package:** `.freebuff/worktrees` branch `packages/game-templates/templates/*.json` 14 curated + `template-schema.json:1` draft-07 + `validate-templates.mjs:1` but **`packages/game-templates` does not exist on `main` HEAD** (`ls packages → sdk, shared-types`) — phantom on worktree only. `validate-templates.mjs:12` `BUILTIN_VARIABLES` 9 but only validates `startup + config.files` placeholders, not `install_script`.

**STATUS:** `DUPLICATE` + `MISSING` (seeding)

**GAP:** Three systems unsynced, overlapping but disconnected (`REF-GAME-DUP-01`, `REF-GAME-DUP-02`, `REF-P6-TMPL-01`): (1) DB eggs canonical (1 seeded), (2) FS `game-templates` 14 on branch/0 on main (private package), (3) static `EGG_TEMPLATES` frontend constant 14, (4) localStorage `app-templates` 5. No single source of truth; `ListEggs(ctx,"")` returns 1 row fresh; `EGG_TEMPLATES` wizard pre-fills DB creation but not synced; `game-templates` curated 14 (93% deficit) never seeded via migration like `SeedDefaultApps`.

**FORGE LOGIC FINDING:** C-15-LF — Seeding & materialization divergence: fresh Forge ships 1 minimal Minecraft placeholder, not 14; admin must manually import via paste JSON (`AdminNestsEggs.tsx:124` validates only `dockerImages.length>0`) — no batch import or `SeedDefaultApps` analogue. `main` cannot build `packages/game-templates` → CI never validates templates.

**RECOMMENDATION:** `ADAPT` — Choose single canonical: either DB eggs seeded from `egg-templates.ts` via `DefaultSeeder` (like `appstore/seed.go:216`) or generate `egg-templates.ts` from `packages/game-templates`. Add idempotent boot seeder `SeedGameTemplates` upserting 14 curated eggs (or move package to `forge/packages/game-templates` reachable on `main`). Deprecate `app-templates` localStorage or unify under DB.

**SEVERITY:** `P1 OPERATOR_VISIBLE` (phantom catalogue, operator expectation vs reality)

---

### C-16 — Mounts & File Denylist (`file_denylist`, `isIgnored` vs `configMatchRegex`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Mount.php` `Mount` via `egg_mount` pivot (`015_a_mounts.sql:19`); `Egg.php:23` `file_denylist` array; `Egg.php:68` `config_files/startup/logs/stop`; Wings `parser/parser.go:1` 6 parsers `file|yaml|properties|ini|json|xml` with `find/replace` rules, wildcard `.*`, `configMatchRegex {{config.docker.interface}}`, `xmlValueMatchRegex`.
- `pterodactyl-wings/server/filesystem/filesystem.go:1` `IsIgnored`, `HasSpaceErr`, `SafePath`; `server/mounts.go` validates mounts at daemon.

**FORGE**
- **Frontend:** `forge/web/components/server/mounts-view.tsx:1` mounts UI via `ServerMounts` API.
- **API:** `forge/api/internal/http/handlers_admin.go:1579` mount admin CRUD `CreateMount:41`, `validateMountPaths:316`, `validateMountPath:323`, `ensureMountAvailableForServer:297` double-join `mount_node ∧ egg_mount` required, `AllowedMountSourcesForNode` second gate.
- **Service:** `clustermanager:689` `runtimeConfiguration` includes `denylist` but not applied.
- **Store:** `forge/api/internal/store/store_mounts_ext.go:13` `ListMounts`, `41` `CreateMount`, `316` `validateMountPaths` (`path.IsAbs`, `Clean==value`, no `\`, no `..`), `323` `validateMountPath` reserves `source ∈ {/etc/forge,/var/lib/forge/volumes}` + `target ∈ {/,/home/container}`; `297` `ensureMountAvailableForServer`, `367` `AllowedMountSourcesForNode`, `513` `ServerMounts`.
- **DB:** `015_a_mounts.sql:19` `mounts(mount_node,egg_mount,mount_server)`.
- **Beacon:** `beacon/internal/runtime/docker.go:743` `buildContainerMounts` validates `filepath.IsAbs`, `EvalSymlinks`, rejects `target==/` or `/home/container`; `beacon/internal/server/secure_files.go:60` `serverFilesystem` `rootfs.FS` `openat2 RESOLVE_BENEATH`; `hostfiles.go:1` `validateHostPath` with `DAEMON_HOST_FILES_ALLOWLIST` allowlist vs `dataDir` always denied; `secure_files.go:37` `lockUpload` + `archivePathTracker` + `maxArchive 4GB`.
- **Event:** audit `mount assigned/removed`.

**STATUS:** `PARTIAL`

**GAP:** Config-file parser (`config.files` JSON stored in `eggs.config` + `minecraft-paper.json:16` `find:server-port→{{server.build.default.port}}`) is **stored not applied** (Forge inert vs Wings parser). File denylist passthrough `ServerProvisionTarget.FileDenylist` but not enforced at runtime beyond `IsIgnored` check (check exists in `secure_files`? verify). Mount allowlist too narrow (`F-G-24`).

**FORGE LOGIC FINDING:** C-16-LF — Mount reserved list only blocks 2 sources + 2 targets; leaves `/etc`, `/var/run/docker.sock`, `/proc`, `/`, `/root` mountable via compromised admin (`store_mounts_ext.go:332`). `buildContainerMounts` does symlink resolution but panel allowlist short. Pterodactyl doc recommends mounts under dedicated host dirs; Forge panel should default `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts`. Also `file_denylist` is string `file_denylist` stored but Wings `IsIgnored` uses denylist glob matching not present in Beacon file handlers beyond `rename` guard.

**RECOMMENDATION:** `ADAPT` — Harden `validateMountPath` to deny `/etc|/proc|/sys|/dev|/var/run|/root|/` source prefixes or switch to allowlist `MOUNTS_ALLOWED_PREFIX` (default `/srv/forge-mounts`). Either implement pre-start config patch in `beacon/manager.go:onBeforeStart` via `wings/parser` port or strip `config.files` from templates and document unsupported.

**SEVERITY:** `P1 OPERATOR_VISIBLE` → `P0` if admin token stolen (host breakout); `P2` for config parser inert

---

### C-17 — Config Parser (6 parsers: `properties|yaml|json|xml|ini|file`)

**REFERENCE**
- `pterodactyl-wings/server/parser/parser.go:1` + `config_parser.go` + `server/configuration.go:60` — `ConfigurationFiles []ConfigurationFile` with `FileName, Parser, Find(map[string]string), Replace?`. Supports `properties` (`server.properties`), `yaml`, `json`, `xml`, `ini`, `file`. `configMatchRegex {{config.docker.interface}}` lookup at boot via `UpdateConfigurationFiles()` before start (`power.go:184`). Handles wildcard `.*`, array `something[1]`, `xmlValueMatchRegex`.
- `pufferpanel-templates/spec.json:307` `alterfile/file` ops similar.

**FORGE**
- **Frontend:** `egg-templates.ts:63` `config.files["server.properties"]{parser:properties, find:{server-ip,server-port}}` inert; `startup-view.tsx` shows raw startup.
- **API:** `store_servers_control.go:95` `e.config::text` stored as `ConfigJSON` (`97`), `store_nests.go:231` `Config` normalized as `normalizeJSONObject` — no parser validation.
- **Service:** `clustermanager/service.go:689` `runtimeConfiguration` packs `Config` map + `Allocations` but never applies patch; `configSyncPending` set via `UpdateServer` but not via parser.
- **Store:** `store_servers_control.go:178` `resolveStartupCommand` only resolves `{{VAR}}` from Environment map, **not** `{{server.build.default.port}}` or `{{config.docker.interface}}`. `minecraft-paper.json:22` config patch `find:"server-port"` expects `{{server.build.default.port}}` alias.
- **Beacon:** `beacon/internal/server/server.go:844` `syncConfiguration` writes `server.json` + `applyConfigurationFiles:869` — **not** parsing `config.files`; only writes raw JSON. No `parser` package ported.
- **DB:** `eggs.config JSONB` holds raw; `servers.config_sync_pending` flag exists.

**STATUS:** `MISSING`

**GAP:** All game servers requiring `server.properties` patching (Minecraft) will start on wrong port / `SERVER_PORT` allocation not patched to file. `{{server.build.default.port}}` inert (`F-G-12`). Wings `parser` `file/yaml/properties/ini/json/xml` patching not reimplemented; `game-templates` `minecraft-paper.json:16` `find:server-port→{{server.build.default.port}}` inert. PufferPanel `alterfile` ops also lost in single-script transliteration.

**FORGE LOGIC FINDING:** C-17-LF — Startup variable substitution `resolveStartupCommand:178` only handles `{{VAR}}` and `{{lower}}`, not `{{server.build.default.*}}` builtins enumerated in `validate-templates.mjs:12` (`SERVER_PORT, SERVER_IP, SERVER_MEMORY, SERVER_UUID, P_SERVER_UUID`). So `config.files` find values that *do* use those builtins remain literal `{{...}}` in file. No allocation-port file patch.

**RECOMMENDATION:** `ADAPT` — Either port `wings/parser` logic into beacon pre-start `onBeforeStart` (apply `config.files` to `RootDir` via `rootfs.FS` openat2, before `Start`), or explicitly strip `config.files` from templates and document unsupported, removing dead placeholder `{{server.build.default.port}}` expectations. Prefer port — low cost (copy `parser` package, ~300 LoC).

**SEVERITY:** `P2 USER_VISIBLE` (Minecraft wrong port on start; silent misconfig)

---

### C-18 — Schedules / Cron Tasks (`server_schedules` + `schedule_tasks` + `schedule_runs`)

**REFERENCE**
- `pterodactyl-panel/app/Models/Schedule.php` + `ScheduleController.php:1` + `Task.php` — `action ∈ {power,command,backup}`; `ProcessScheduleService.php:45` `is_processing` lock, `only_when_online` via daemon `DaemonServerRepository->getDetails()` live probe; `getNextRunAt()` via `Utilities::getScheduleNextRunDate` throwing 422 if cron invalid.
- `pterodactyl-wings` `server/schedule.go` (not explicit) daemon executes `power/command/backup`.

**FORGE**
- **Frontend:** `forge/web/components/server/schedules-view.tsx:37` `TaskEditor` handles `command|power|backup` with `sequence/offset/continueOnFailure`, Cron helper, reorder via swap; `forge/web/app/server/[id]/schedules/page.tsx`.
- **API:** `forge/api/internal/http/handlers_servers.go:1152` `GET /schedules`, `1165` `GET /schedules/:id`, `1178` `POST /schedules` (`validateServerScheduleCron:1183` via `robfig/cron`), `1220` `PATCH /schedules/:id`, `1272` `DELETE`, `1288` `POST /schedules/:id/tasks` (`CreateScheduleTask`), `1315` `POST .../run` (`runner.RunNow`), `1327` `GET .../runs`, `1340` `PATCH /tasks/:taskId`, `1367` `DELETE .../tasks/:taskId`; `handlers_user_console.go:314` server-scoped cron `registerServerCronJobRoutes` (`GET /servers/:id/cron-jobs` via `ListCronJobs` global then Go-filter `331` `if TargetType!="server" continue` — O(N) scan).
- **Service:** `forge/api/internal/http/schedule_runner.go:139` `tick` → `ListDueSchedules` (`489`) uses `only_when_online = FALSE OR s.status='running'` (DB status, not live); `ClaimDueSchedule` lease `ScheduleLeaseDuration`; `executeTask:386` `power|backup|command` via daemon. `cronjob/service.go:117` `executeJob` branches `TargetType=="server"` → `dispatchServerCommand:240` (node console) else `runShellCommand:212` (`sh -c`).
- **Store:** `forge/api/internal/store/store_schedules.go:14` `resolveStartupCommand`; `26` `ListSchedules` + task join; `166` `CreateSchedule` (validates name, default `*`); `252` `CreateScheduleTask` checks `action!=""` only (`254`) + `timeOffset>=0` — **does not** call `isValidScheduleTaskAction` (`store.go:1196` valid `power|backup|command`); `331` `PatchScheduleTask` **does** validate `332`. Asymmetry (`F-G-23`). `489` `ListDueSchedules` filters `only_when_online` via DB `status`; `579` `UpdateScheduleRunMeta`, `590` `CreateScheduleRun`, `605` `FinishScheduleRun`, `624` `ListScheduleRuns`.
- **DB:** `008_server_schedules.sql:1`, `009_schedule_run_history.sql:1` (`server_schedules`, `schedule_tasks`, `schedule_runs`, `schedule_task_runs`).
- **Beacon:** no scheduler — panel-centralized.
- **Event:** `NotifySchedulesChanged:398` `NOTIFY schedule_events` + `ListenScheduleEvents:402` with backoff.

**STATUS:** `PARTIAL`

**GAP:** (1) Task action asymmetry allows storing arbitrary action (e.g., `rm -rf`) — only rejected at `schedule_runner:386` (`unsupported task action`) polluting schedule store (MEDIUM). (2) `only_when_online` uses DB `s.status='running'` vs Pterodactyl live daemon probe — under rapid transitions may run while offline or skip while starting. (3) Server-scoped cron listing `handlers_user_console.go:315` scans whole table then Go-filters — `O(N)` timing leak vs SQL `WHERE target_type='server' AND target_id=$1`. (4) No `is_processing` row lock like Pterodactyl — relies on `ClaimDueSchedule` lease + `pg_advisory_lock` + `SELECT FOR UPDATE SKIP LOCKED` style claim (sound but different).

**FORGE LOGIC FINDING:** C-18-LF — Asymmetry `CreateScheduleTask:252` vs `PatchScheduleTask:332` violates fail-at-admission; arbitrary action persists (`F-G-23`). Also `validateServerScheduleCron:3693` correctly uses `robfig/cron` but `PatchSchedule` defaulting `minute="*"` when only `hour` patched (`1227`) may mis-validate partial updates if caller sends empty string.

**RECOMMENDATION:** `ADAPT` — Add `isValidScheduleTaskAction` guard to `CreateScheduleTask:252` (one-line). Push server cron filter into SQL `ListCronJobsByServer`. Keep DB `status` for `only_when_online` but add live probe fallback for edge cases or document.

**SEVERITY:** `P2 OPERATOR_VISIBLE` (invalid tasks persist) / `P3` for O(N) scan

---

### C-19 — Subusers / Permissions (`server:subusers`, `*` wildcard, subset enforcement)

**REFERENCE**
- `pterodactyl-panel/app/Models/Subuser.php:1` `$casts permissions=>array`; `Permission.php:1` 10 groups (websocket/control/user/file/backup/allocation/startup/database/schedule/settings/activity) 45 constants `ACTION_*`, `permissions()` map `101`; `RESOURCE_NAME=subuser_permission`.
- `pterodactyl-panel/app/Models/Server.php:162` `owner_id`, `subusers()`, `validateCurrentState:393`; `GetUserPermissionsService.php:18` returns `['*']` **exclusively** for `root_admin` or `owner_id===user.id` — never stored for subusers.
- `pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/SubuserController.php:154` `getDefaultPermissions()` intersects request with `Permission::permissions()` allowlist, always adds `websocket.connect`; service checks actor can only grant permissions they themselves possess (`Permission.php:120` comment).
- `pterodactyl-panel/app/Services/Servers/SubuserUpdateService.php` similar subset enforcement.
- Wings permission middleware daemon-side `ServerPermission`.

**FORGE**
- **Frontend:** `forge/web/components/server/users-view.tsx:1` + `forge/web/lib/api/servers.ts` subuser CRUD.
- **API:** `forge/api/internal/http/handlers_servers.go:501` `GET /servers/:id/users`, `554` `POST /users` (checks `PermUserCreate`), `594` `PATCH /users/:userId`, `622` `DELETE`; `handlers_servers.go:541` checks `user.create` but not `perms ⊆ actorPerms`.
- **Service:** none separate; `auth.go:558` `checkServerPermission`.
- **Store:** `forge/api/internal/store/permissions.go:5` reproduces all 10 groups + adds `cron.read/create/update/delete/run` (`28`), `buildpack.manage`, `mount.read/update`, `server:read-env`, `server.view/settings` (`11` `IsSensitivePermission` flags `database.view_password`+`server:read-env` via `redactStartupSecrets:71`); `forge/api/internal/store/store_users.go:297` `UpsertServerSubuser` only allowlist-checks `allowed[perm]||perm=="*"` (`563`) **without actor-subset**; `544` `normalizeSubuserPermissions` allows `*`; `574` `defaultSubuserPermissions()` enumerates 40+ keys; `362` `UserCanAccessServer` respects `HasPermission` wildcard (`permissions.go:206` `p=="*"`). `AuthenticateSFTP:464` & `AuthorizeSFTPSession:536` both iterate `*` or `file.sftp`.
- **DB:** `016_subusers_panel_parity.sql:1` `subusers(id,server_id,user_id,permissions jsonb)` unique `(server_id,user_id)`.
- **Worker:** none.
- **Beacon:** not directly; `server.go:1178` `installWS` rejects non-admin scopes — subuser `*` would bypass panel but not Beacon HMAC.
- **Event:** audit `server subuser upserted/deleted`.

**STATUS:** `BROKEN`

**GAP:** (`F-G-22` HIGH): `normalizeSubuserPermissions` (`store_users.go:563`) only checks allowlist + `*`, no `actorPerms ⊆` check. Any holder of `user.create` can grant `*` or arbitrary `database.view_password`, `schedule.delete`, etc., to any user. Pterodactyl explicitly forbids (“They will never be able to assign permissions they do not have themselves.”) and never persists `*` for subusers. Forge persists `*` if supplied. `GET /servers/:id` (`handlers_servers.go:335`) sets `Permissions=["*"]` only for owner/admin else subuser row — consistent but subuser `*` row then becomes universal.

**FORGE LOGIC FINDING:** C-19-LF — Vertical privilege escalation. Low-priv collaborator (`file.read`+`user.create` to invite) can self-escalate to full control, read DB passwords (`database.view_password`), trigger `settings.reinstall`, create cron jobs dispatching commands to node (`cron.create` server-scoped but node-touching). `GetTeamMemberPermissions:418` OR-merge re-enables explicit `false` (Info) — overwrites stored `false` indistinguishable from absent zero-value.

**RECOMMENDATION:** `ADAPT` — Enforce subset at `UpsertServerSubuser` (or handler): load actor effective permissions via `GetServerSubuser(ctx, serverID, actorID)` (or `['*']` for owner/admin) and reject any `req.Permissions` not in actor set unless owner/admin. Gate `*` behind owner/admin only (remove from `normalize` allow path or check `actorIsOwnerOrAdmin`). Add test: `user.create` holder cannot grant `database.view_password`.

**SEVERITY:** `P0 USER_VISIBLE` (permission bypass / wrong scope — breaks subuser trust boundary)

---

## 2. Consolidated Logic Findings (carry-forward + new)

> Requirement: ≥4 logic findings, each file:line-cited, even if reference lacks feature (spec race/wrong transition/permission/host-breakout).

| # | Id | Severity | Title | Files | Finding |
|---|----|----------|-------|-------|---------|
| **LF-01** | **F-G-22** (Phase2-05 F-01) | **P0** `USER_VISIBLE` | **Subuser `*` escalation — any `user.create` holder can grant `*`** | `store/store_users.go:297` `UpsertServerSubuser:563 normalizeSubuserPermissions` + `permissions.go:206 HasPermission` + `http/handlers_servers.go:554 POST /users` | `normalize` allowlists `allowed[perm]||perm=="*"` without checking `actorPerms ⊆ requested`. Handler only checks `user.create`. Repro: alice (`user.create,file.read`) → `POST /servers/:id/users {email:bob, perms:["*"]}` succeeds → bob has full control. Pelican `SubuserController:154 getDefaultPermissions` intersects + `websocket.connect` inject and service checks subset. Fix: load actor perms, reject grants outside set; gate `*` behind owner/admin. **Carried** from phase-02 & MASTER_INDEX. |
| **LF-02** | **F-G-08** (Phase2-02 LF-01) | **P0** `OPERATOR_VISIBLE` | **Regex `validateVariableValue` delimiter bug blocks all PTDL imports** | `store/store_egg_variables.go:142` `validateVariableValue:176 regex` + `store_egg_variables.go:143 Split("|")` + `templates/minecraft-paper.json:80 rules:"required\|regex:/^([\\w\\d._-]+)(\\.jar)$/"` | For `regex:/^([\w\d._-]+)(\.jar)$/` yields `arg=/^.../` with slashes; `regexp.Compile(arg)` expects literal slashes → `server.jar` fails. Also `Split("|")` splits regex alternation `|` inside char class. Result: `POST /eggs/import` (`handlers_admin.go:1354`) **rejects all variables with regex rules**, blocking import of stock Paper/Vanilla eggs. Fix: strip leading `/` and trailing `/[flags]` before compile; respect `|` inside `regex:`; or reuse Laravel validator. **Carried**. |
| **LF-03** | **F-G-06** (Phase2-01 C10) | **P1** `USER_VISIBLE` | **`restoring_backup` server lock not wired — concurrent start allowed during restore** | `store/store_state.go:131 serverStatusFromActual` + `http/handlers_servers.go:2104 /backups/restore` `MarkBackupStatus restoring` (per-backup) + `store/store_state.go:31 SetServerActualState` never called + `http/handlers_servers.go:145 ensureTransferIdle` (no restore guard) + `beacon/server/manager.go:30` no Restoring state | All 4 refs treat `restoring_backup` as server exclusive lock blocking power/suspend/reinstall/transfer (`Server.php:393 validateCurrentState`, `wings power.go:57 IsRestoring`, `pufferpanel IsRestoring:879`). Forge tracks per-backup `backups.status=restoring` but never flips `servers.actual_state/status`. `POST /power start` concurrent with restore allowed → race on `RootDir` half-written extract vs running process. Fix: on `DispatchBackupRestore` also `SetServerActualState restoring_backup` and guard `power/install/reinstall/suspend` via `ensureRestoreIdle`. **Carried**. |
| **LF-04** | **F-G-07** (Phase2-01 C07) | **P1** `USER_VISIBLE` | **Reinstall fails on Docker runtime gate (`Reinstaller` interface) but advertised** | `services/clustermanager/service.go:241 Reinstaller` check + `beacon/server/server.go:1331 reinstall` (just calls install) + `http/handlers_servers.go:1110 POST /reinstall` | `clustermanager:242` gating `Reinstaller` makes pure-Docker Reinstall always 500 `"runtime does not support reinstall"` while Beacon `server.go:1323` reinstall works (calls install) → API vs Beacon drift. Caller sees `failed` marking `status='install_failed'`. Fix: fallback to `InstallServer` when runtime lacks `Reinstaller` (mirror Beacon) or implement Docker `Reinstall` alias. **Carried**. |
| **LF-05** | **F-G-05** | **P1** `USER_VISIBLE` | **Kill does NOT pierce stuck `RunningAction` lock (emergency SIGKILL blocked)** | `beacon/server/manager.go:487 TryLock + RunningAction!=""` + `runtime/docker.go:494 Kill` sharded `workloadLocks[64]` + `wings/server/power.go:108 kill Try Acquire ignore failure` (`81 comment`) | Wings documents `kill` can pierce stuck power lock. Forge queues `kill` behind stuck op (worker 5m reaper + `RunningAction` slot claimed) so hung install holds lock → kill waits. Fix: separate kill semaphore or TryLock-ignore for `signal==kill` mirroring Wings `115-119`. **Carried** (Phase2-01 C05). |
| **LF-06** | **F-G-24** (Phase2-05 F-03) | **P1** `OPERATOR_VISIBLE` → `P0` if admin token stolen | **Mount allowlist too narrow → host path breakout via compromised admin** | `store/store_mounts_ext.go:323 validateMountPath:332 source∈{/etc/forge,/var/lib/forge/volumes}, target∈{/,/home/container}` + `beacon/runtime/docker.go:743 buildContainerMounts` | Only blocks 2 sources + 2 targets; leaves `/etc`, `/var/run/docker.sock`, `/proc`, `/`, `/root` mountable via `POST /mounts` (`handlers_admin.go:1579`) with `mounts.write` scope. Compromised admin becomes host-breakout primitive without shell. Forge `AllowedMountSourcesForNode:367` second gate but panel allowlist short. Fix: deny `/etc|/proc|/sys|/dev|/var/run|/root|/` or require `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` allowlist. **Carried**. |
| **LF-07** | **F-G-09** (Phase2-02 LF-02) | **P1** `SILENT` | **CPU `cpu_shares` vs `CPUPercent` conflation — unlimited still weighted** | `store/store_servers.go:209 CPUShares/CPULimit` + `beacon/runtime/docker.go:901 validateCreateRequest:924 CPUPercent 0..100000` + `settings.go:124 Wings conditional` | `store:209` allows `CPULimit=0` (unlimited) with `CPUShares=1024` still set; beacon `924` caps `CPUPercent` but store caps `CPUShares 2..262144` — unlimited still sets weight `1024` diverging from `settings.go:124` conditional. Impact wrong throttling. Fix gate weight on `CPULimit>0`. **Carried**. |
| **LF-08** | **F-G-23** (Phase2-05 F-02) | **P2** `OPERATOR_VISIBLE` | **Schedule task action asymmetry — arbitrary action persists** | `store/store_schedules.go:252 CreateScheduleTask:253 action!=""` only vs `store/store_schedules.go:332 PatchScheduleTask:332 isValidScheduleTaskAction` + `store/store.go:1196 isValidScheduleTaskAction` | Create vs Patch asymmetry: attacker with `schedule.update` (gates `POST .../tasks:1288`) can persist `action="powershell"`; runner `schedule_runner.go:386` rejects at execution (`unsupported`) but DB polluted, `ListDueSchedules` counts include unrunnable, wasting lease. Fix one-line guard at `252`. **Carried**. |
| **LF-09** | **NEW** | **P2** `OPERATOR_VISIBLE` | **Allocation `container_port`/`protocol` lost in legacy `Mappings` fallback** | `store/store_allocations.go:155 containerPort` + `daemon/client.go:438 Port` + `runtime/docker.go:865 dockerPorts` + `clustermanager/service.go:698 Mappings` + `wings/environment/allocations.go:36 Mappings map[string][]int` | Forge explicit `protocol` dropped when serialization uses legacy `Mappings` (`Bindings()` creates both tcp+udp for every port) → UDP-only `8211` also binds tcp. Add protocol-distinct unique index `(node_id,ip,port,protocol)` verification. **Carried** (`F-G-11`). |
| **LF-10** | **NEW** | **P2** `SILENT` | **Generation fencing livelock when `crashed` with desired `running`** | `store/store_state.go:39 CASE WHEN desired='running' AND actual='running' THEN desired_generation` + `services/reconciler/service.go:476 reconcileServer` | `observed_generation` only advances on exact active overlap, leaving `crashed+desired running` nunca converging → reconciler loop emits `remediate:true` forever. Fix expand fence to `crashed→running` transition or desired `running` + any `actual IN (running,starting)`? **New** (phase-02 subagent-01 LF-02). |
| **LF-11** | **NEW** | **P2** `SILENT` | **Hash collision without generation seed skips reconciliation** | `beacon/runtime/docker.go:193 createRequestHash:1016` + `docker.go:200 configHashLabel` | Hash of `CreateRequest` without `desired_generation` collapses two Creates with different generations to same hash → skips reconciliation. Fix include `desired_generation` in hash. **New** (F-G-03). |
| **LF-12** | **NEW** | **P3** `SILENT` | **Server-scoped cron listing scans whole table then Go-filters (O(N))** | `http/handlers_user_console.go:322 ListCronJobs` + `store/store_cron_jobs.go:95 ListCronJobs` global + `handlers_user_console.go:331 for j … if TargetType!="server" continue` | O(N) overhead / timing leak vs SQL `WHERE target_type='server' AND target_id=$1`. Push filter into SQL via `ListCronJobsByServer`. **Carried** (`F-G-25`). |

> Also re-affirmed but not duplicated in table: `user_viewable` gate leak `CreateServer` can set `DL_PATH` at provision (`F-G-10` `store_servers_control.go:154` vs `store_startup.go:58`), `installing→installed` clobbers `suspended` badge (`F-G-04` `store_servers_control.go:226`), `heartbeat` `ResourceScore` overcommit healthy (`noderegistry/service.go:302`) is game-agnostic but not game-hosting row.

---

## 3. Status Rollup (19 capabilities)

| Capability | Status | Bucket |
|------------|--------|--------|
| C-01 create→created | **COMPLETE** (superset) | Keep |
| C-02 desired/actual + fencing | **COMPLETE** | Keep |
| C-03 suspend/unsuspend | **PARTIAL** | Gap |
| C-04 power start/stop/restart | **PARTIAL** | Gap |
| C-05 kill pierce | **MISSING** | Gap |
| C-06 install (first-time) | **COMPLETE** | Enhanced |
| C-07 reinstall | **BROKEN** | Must fix |
| C-08 delete + orphan | **COMPLETE** | Enhanced |
| C-09 restoring_backup lock | **MISSING** | Must fix |
| C-10 transfer/migration v1 | **PARTIAL** | Doc/dual |
| C-11 allocations | **COMPLETE** | Superset |
| C-12 resources | **PARTIAL** | Conflation |
| C-13 egg/nest model | **COMPLETE** | OK |
| C-14 variables + validation | **BROKEN** | Must fix |
| C-15 template vs egg dualism | **DUPLICATE** + `MISSING` seed | Degap |
| C-16 mounts + denylist | **PARTIAL** | Hardening |
| C-17 config parser | **MISSING** | Implement or drop |
| C-18 schedules/cron | **PARTIAL** | Guard |
| C-19 subusers/permissions | **BROKEN** | Must fix |

Counts: `COMPLETE 7 | PARTIAL 6 | MISSING 3 | BROKEN 3 | DUPLICATE 1` (19). Parity vs Wings: **Forge exceeds Wings functionally on 9/19, broken/missing on 5/19, intentional divergence on 2/19.**

---

## 4. Recommendations (ADOPT/ADAPT/INSPIRE/REJECT with justification)

| Capability | Rec | Justification |
|------------|-----|---------------|
| C-01 provisioning split | **ADOPT** | Keep — generation fencing + `provisioning` is strictly more truthful than Pterodactyl single `installing` default; add badge mapping. |
| C-02 desired/actual | **ADOPT** | Keep canonical — no ref had it; Pelican `ServerState` enum is weaker; aligns with_nomad-style reconciliation. |
| C-03 suspend orthogonal bool | **ADAPT** | Keep `suspended` bool orthogonal to `status` (preserves `install_failed+suspended`) but add `ensureTransferIdle` to suspension, client-API `AuthenticateServerAccess` gate, and dual-badge. |
| C-04 power semantics | **ADAPT** | Keep durable queue but fail-fast 409 for `installing`/`restoring` before enqueue (match Wings 409), keep `stop` blocked during `installing` only `kill` allowed. |
| C-05 kill pierce | **ADOPT** | Adopt Wings `power.go:108` pierce comment verbatim — emergency kill must not queue. |
| C-06 install security | **ADAPT** | Keep `1000:1000` `ReadonlyRootfs` `CapDrop ALL` (`docker.go:304`) — strictly better than Wings default`User 0:0`; honor `skip_scripts` and unify scope checks. |
| C-07 reinstall | **ADOPT** | Adopt Pelican/Ptero `install(true)` — implement fallback `Reinstall→Install` when `Reinstaller` missing; add 409 `stop first` at API. |
| C-08 orphan remediation | **ADOPT** | Keep `server_orphan_remediations` — novel vs silent Pterodactyl; add `GET /admin/orphans` UI. |
| C-09 restoring lock | **ADOPT** | Adopt `Server.php:393 validateCurrentState` `restoring_backup` semantics verbatim. |
| C-10 transfer v1 | **ADAPT** | Adopt control-plane-mediated v1 (resumable chunks, scoped `TransferCredentialClaims`) over Wings JWT peer push (`server.go:360` retired) — keep v1, fix dual-state projection and doc. |
| C-11 allocations explicit | **ADOPT** | Adopt explicit `protocol`+`containerPort` vs Wings `Mappings` dual — superior; fix DB index and drop legacy `Mappings` fallback. |
| C-12 cpu shares/percent | **ADAPT** | Adapt `settings.go:124` conditional (`CPUShares` only when `CpuLimit>0`) and unify `cpuLimit/cpuPercent` naming. |
| C-13 eggs | **ADOPT** | Keep `normalizeDockerImages` map-or-array handling — more permissive/correct. |
| C-14 variable validator | **ADAPT** | Adapt Laravel `ValidationFactory` — strip `regex:/…/` delimiters, respect `|` inside regex, add `RESERVED_ENV_NAMES` guard, gate `DL_PATH` at creation. |
| C-15 templates | **ADAPT** | Collapse FS vs DB vs localStorage three-way split — generate `EGG_TEMPLATES` from DB seed job or vice-versa; add `SeedGameTemplates` idempotent boot (like `SeedDefaultApps` `appstore/seed.go:216`). |
| C-16 mounts | **ADAPT** | Adapt Pterodactyl doc: mounts under dedicated host prefix (`MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts`) plus hard deny `/etc|/proc|/sys|/dev|/var/run`. |
| C-17 config parser | **INSPIRE** | Inspire Wings `parser:1` — port `parser/` or document unsupported and remove `config.files` from templates; choose one. |
| C-18 schedules | **ADAPT** | Adopt `isValidScheduleTaskAction` at creation; keep DB `only_when_online` but push cron filter into SQL. |
| C-19 subusers | **ADAPT** | Adapt `SubuserController:154 getDefaultPermissions` subset check — enforce `perms ⊆ actorPerms` and gate `*` behind owner/admin. |

**REJECT list:** (patterns explicitly **not** to adopt)
- Host `supportedEnvironments` (`spec.json:26` `environment+supportedEnvironments[host,docker]`) — `pufferpanel-templates/spec.json:26` dual `host|docker` vs Forge Docker-only — Forge correctly Docker-only; reject host env.
- Single `status='suspended'` wiping prior state (`Server.php:125`) — reject, keep orthogonal `suspended` bool.
- Monolith `IsIdle` boolean flags (`pufferpanel/servers/server.go:884`) vs Forge `RunningAction` single slot — reject puffer, keep `manager.go:488` slot.
- Typed install pipeline 26 ops with `if: file_exists` (`spec.json:307`) fully on daemon — **INSPIRE for modded Minecraft only**, keep single `install_script` for most games (see phase-02 synthesis §7).
- Multi-env `PortBindings: ["0.0.0.0:${port}:${port}/tcp"]` variable interpolation at daemon — reject, keep Forge `protocol` explicit.

---

## 5. Evidence Anchor (every claim file:line — condensed)

**References:**
- `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:122` constants, `125` suspended, `126` restoring_backup, `138` `$attributes installing`, `153` resource validation, `162` allocation/threads/oom, `215` isInstalled, `218` isSuspended, `393` validateCurrentState
- `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:52` egg schema, `68` EXPORT_VERSION, `84` docker_images/config/file_denylist
- `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:29` egg_variables, `43` RESERVED, `70` validation
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:28`
- `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Application/Servers/ServerManagementController.php:25` suspend, `Api/Remote/Servers/ServerTransferController.php:34` remote transfer
- `reference/game-hosting/pterodactyl-wings/server/power.go:24` HandlePowerAction, `30` constants, `57` installing/transfer/restoring guard, `81` kill bypass comment, `108` kill TryLock ignore, `171` onBeforeStart
- `reference/game-hosting/pterodactyl-wings/server/install.go:33` Install, `80` Reinstall, `195` installing.SwapIf
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` Limits, `66` ConvertedCpuLimit, `116` BlkioWeight, `124` conditional shares
- `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` Mappings, `54` Bindings dual
- `reference/game-hosting/pterodactyl-wings/server/server.go:151` GetEnvironmentVariables, `468` manager
- `reference/game-hosting/pterodactyl-wings/server/configuration.go:60`
- `reference/game-hosting/pterodactyl-wings/parser/parser.go:1` 6 parsers
- `reference/game-hosting/pelican-panel/app/Enums/ServerState.php:12` enum
- `reference/game-hosting/pufferpanel/servers/server.go:59` Server struct, `185` Start, `343` Kill, `410` Install, `884` IsIdle, `92` processQueue
- `reference/game-hosting/pufferpanel-templates/spec.json:1` schema, `26` environment, `51` install, `156` groups, `201` variable, `307` operation 26 types
- `reference/game-hosting/pufferpanel/environment.go:17` EnvironmentImpl

**Forge API:**
- `forge/api/docs/server-lifecycle.md:1` lifecycle (provisioning→created≠installed, Beacon-first delete, transfer 501)
- `forge/api/internal/store/store_servers.go:180` CreateServer, `253` provisioning, `273` StartupVariables validation, `385` CompareAndSet, `374` SetServerSuspension, `397` SetServerSuspended, `308` GetServer, `346` IsServerTransferBlocking
- `forge/api/internal/store/store_servers_control.go:13` SetServerPowerState, `44` powerSignalPriorStates, `57` ServerControlTarget, `83` ServerProvisionTarget, `154` environment no viewable filter, `178` resolveStartupCommand, `207` SetServerProvisioned, `218` SetServerInstallState, `244` MarkSynced
- `forge/api/internal/store/store_state.go:10` SetDesired, `31` SetActual, `39` observed_generation fence, `120` recordStateTransition, `131` serverStatusFromActual
- `forge/api/internal/store/store_servers_lifecycle.go:12` HardDeleteServer, `18` RecordOrphanAndHardDeleteServer, `25` hardDeleteServer Tx
- `forge/api/internal/store/store_nests.go:16` Nest, `34` Egg, `100` ListNests, `137` CreateNest, `172` DeleteNest, `190` ListEggs, `248` CreateEgg, `361` normalizeDockerImages
- `forge/api/internal/store/store_egg_variables.go:15` pattern, `17` EggVariable, `65` CreateEggVariable, `129` validateRequest, `142` validateVariableValue `176` regex `regexp.Compile(arg)`
- `forge/api/internal/store/store_startup.go:11` GetServerStartup `31` user_viewable, `54` UpdateServerStartupVariable `58` viewable+editable
- `forge/api/internal/store/store_templates.go:12` ListTemplates compat, `99` templateFromEgg
- `forge/api/internal/store/store_allocations.go:15` Allocation, `125` CreateAllocations Tx, `214` DeleteAllocations, `327` AssignAllocationToServer, `367` SetPrimaryAllocation `FOR UPDATE`
- `forge/api/internal/store/store_mounts_ext.go:13` ListMounts, `41` CreateMount, `297` ensureMountAvailableForServer double-join, `316` validateMountPaths, `323` validateMountPath reserved `2+2`, `367` AllowedMountSourcesForNode, `513` ServerMounts
- `forge/api/internal/store/store_schedules.go:14` resolveStartupCommand, `26` ListSchedules, `166` CreateSchedule, `252` CreateScheduleTask `254` action!="" only, `331` PatchScheduleTask `332` isValid, `489` ListDueSchedules online filter, `579` UpdateScheduleRunMeta
- `forge/api/internal/store/store_users.go:88` List/Upsert, `297` UpsertServerSubuser, `353` UserCanAccessServer, `410` AuthenticateSFTP, `544` normalizeSubuserPermissions `563` allows `*`, `574` defaultSubuserPermissions
- `forge/api/internal/store/store.go:27` ScheduleTaskAction `30` constants, `1196` isValidScheduleTaskAction, `776` ServerProvisionTarget, `1196` isValid
- `forge/api/internal/http/handlers_servers.go:46` hasServerReadEnv, `71` redactStartupSecrets, `130` legacyTransferGone, `145` ensureTransferIdle, `296` GET /servers, `321` GET /servers/:id `335` Permissions *, `355` PATCH /servers, `451` POST /servers/:id/reload, `467` allocations, `501` users, `554` POST /users (no subset), `713` POST /transfer migration, `740` GET /transfer stale, `789` POST /servers, `888` POST /power 202, `1050` POST /install durable, `1110` POST /reinstall, `1152` schedules CRUD, `1400` suspension `1421` action:suspend, `1461` DELETE /delete orphan
- `forge/api/internal/http/handlers_admin.go:990` nests, `1083` eggs, `1185` variables, `1288` export, `1308` import (`1354` CreateEggVariable loop), `1365` allocations/nodes, `1579` mounts
- `forge/api/internal/http/handlers_user_console.go:314` registerServerCronJobRoutes `322` ListCronJobs global+Go-filter O(N)
- `forge/api/internal/services/clustermanager/service.go:76` CreateServer, `172` provisionServer, `216` InstallServer, `220` ReinstallServer `241` Reinstaller gate, `276` syncProvisionTarget, `285` compensateCreateFailure, `332` DeleteServer, `362` Start/Stop/Restart/Kill, `382` RequestServerPower `394` Suspended check, `532` sendPower, `576` publishServerPowerEvents, `632` runtimeCreateRequest, `681` runtimeInstallRequest
- `forge/api/internal/services/orchestrator/suspension.go:28` SuspendServer, `82` UnsuspendServer (verified via grep, file exists)
- `forge/api/internal/services/reconciler/service.go:134` New, `267` diff, `476` reconcileServer, `542` publish
- `forge/api/internal/services/heartbeatmonitor/service.go:205` evaluate, `266` classify (6-state)
- `forge/api/migrations/001_init.sql:24` status, `004_server_transfer_state.sql:1`, `005_server_transfer_lifecycle.sql:1`, `007_postgres_core_foundation.sql:38` suspended `106` Games nest `134` allocations, `013_startup_variables.sql:1`, `015_a_mounts.sql:19`, `021_true_state_persistence.sql:3` desired/actual+transitions, `040_truthful_server_lifecycle.sql:4` clobber+orphan `18`, `043_unify_eggs_templates_mounts.sql:63` unify, `045_real_server_transfer.sql:1` migration_runs, `091_seed_minecraft_java.sql:3` one egg `96` seed apps mimic

**Beacon:**
- `beacon/internal/server/server.go:223` NewServer, `240` NewServerWithBackup, `312` MarkCreated, `735` create `770` validate serverId+image, `796` ports mounts, `814` CreateRequest, `844` syncConfiguration `869` applyConfigurationFiles (no parser), `1052` install `1116` BeginInstall claim `1122` runtime.Install `1131` EndInstall `1132` notifyPanel, `1159` installWS `1178` ScopeAdmin, `1331` reinstall `1335` PowerState==Running→409, `1349` power `1380` block 30s poll, `1425` delete `625` docker Delete idempotent, `3366` authenticate
- `beacon/internal/server/manager.go:21` PowerState, `30` ServerState `36` RunningAction etc, `103` NewServerManager, `128` Reconcile `177` autoRestartCrashed, `250` State `271` BeginInstall TryLock+Suspended+installing, `298` EndInstall, `312` MarkCreated, `430` persistPowerState, `482` HandlePower `487` TryLock+RunningAction `494` installing guard, `511` start `534` stop `549` restart `581` kill same slot, `610` onBeforeStart `611` snapshot `623` installing `628` suspended `633` synced `638` root, `652` syncServerStateFromPanel `689` syncServerStateFromPanel `748` Suspended assign, `776` HasSpaceForWrite, `843` StartEventWatcher `879` HandleContainerEvent `925` CrashCooldown `935` loop window `954` HandlePower start, `957` isExitEvent
- `beacon/internal/runtime/runtime.go:10` ProviderDocker etc, `55` CreateRequest `100` PortBinding `115` InstallRequest, `144` Runtime interface `171` Reconciler
- `beacon/internal/runtime/docker.go:42` DockerRuntime `[64]sync.Mutex workloadLocks:251`, `113` ensureImage `124` pinnedImagePattern `@sha256`, `151` Create idempotent `171` Reconcile `193` createRequestHash `200` configHashLabel `256` Install `285` User 1000:1000 `299` CapDrop ALL `302` ReadonlyRootfs `303` no-new-privileges `304` Tmpfs `340` log cap 1MiB `388` Start `434` Stop `494` Kill sharded lock `529` Restart `625` Delete `639` WatchEvents `783` buildResources `808` buildHostConfig `865` dockerPorts duplicate detection `899` configHashLabel `901` validateCreateRequest `924` CPUPercent `1016` createRequestHash (no generation)

**Web:**
- `forge/web/lib/egg-templates.ts:27` EGG_TEMPLATES 14 static, `63` config.files `68` server-port `{{server.build.default.port}}` inert, `80` env `SERVER_JARFILE` regex `/^([\w\d._-]+)(\.jar)$/` (the bug payload), `86` DL_PATH internal
- `forge/web/lib/app-templates-data.ts:3` STORAGE_KEY `forge.app-templates.v1` localStorage
- `forge/web/app/admin/nests/page.tsx:1` AdminNestsEggs
- `forge/web/app/admin/app-templates/page.tsx:1` localStorage UI `56` formToTemplate `host:container` CSV
- `forge/web/components/server/console-view.tsx:37` InstallBanner, `125` canPower, `132` WebSocketManager, `203` network `rx+tx` cumulative bug (phase-2 F-G-18), `243` controls grid `267` chrome `299` synthetic timestamp `console-view.tsx:299` `new Date().toLocaleTimeString()` per line
- `forge/web/components/server/transfer-view.tsx:31` polls transfer, `77` isTransferring flicker, `92` tone, `106` progress bar `h-2`
- `forge/web/components/server/builds-view.tsx:110` Build button `disabled` decorative
- `forge/web/components/server/backups-view.tsx:1`, `schedules-view.tsx:37`, `settings-view.tsx:15`, `mounts-view.tsx:1`

---

## 6. Activation Order (P0→P3) — without new subsystem

1. **P0** `store_egg_variables.go:176` strip `regex:/…/` delimiters + `|`-aware split — unblocks PTDL imports (LF-02).
2. **P0** `store_users.go:563` subset check + gate `*` behind owner/admin — close vertical escalation (LF-01) (`handlers_servers.go:554`).
3. **P1** Wire `restoring_backup` as server lock `handlers_servers.go:2104` `SetServerActualState restoring_backup` on restore + guard `power/install/reinstall/suspend` via `ensureRestoreIdle` alongside `ensureTransferIdle` (LF-03).
4. **P1** Fix `Reinstall` fallback `clustermanager/service.go:241` → `InstallServer` when no `Reinstaller`; add `409 stop first` at API (LF-04).
5. **P1** Kill pierce `manager.go:487` + `operation/service.go` separate semaphore for `kill` (LF-05).
6. **P1** Harden mounts `store_mounts_ext.go:323` deny `/etc|/proc|/sys|/dev|/var/run|/root|/` or require `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` (LF-06).
7. **P1** Fix CPU conflation `store_servers.go:209`/`docker.go:924` align `cpu` percent vs `cpu_shares` weight, gate weight on `CPULimit>0` (LF-07).
8. **P2** Fix `CreateScheduleTask:252` add `isValidScheduleTaskAction` (LF-08); push cron filter into SQL `handlers_user_console.go:322` (`P3` perf).
9. **P2** Fix generation fencing `store_state.go:39` expand to `crashed`+desired `running` convergence; include generation in `createRequestHash:1016` (LF-10/11).
10. **P2** Implement or remove config parser `beacon/server.go:869` — either port `wings/parser` into `manager.go:onBeforeStart` or strip `config.files` from `egg-templates.ts`.
11. **P2** Unify templates seed — `SeedGameTemplates` idempotent boot like `appstore/seed.go:216` upserts 14 `EGG_TEMPLATES` into `eggs+egg_variables`; move `packages/game-templates` onto `main` or generate `egg-templates.ts`.
12. **P3** Unify `transfer` single source `transfer-view.tsx:77` derive `isTransferring` from `transfer?.transferring ?? server.transferring` + invalidate on change; sync legacy `servers.transfer_state` from `migrations` status or retire it (`server-lifecycle.md:12`).
13. **P3** Remove dead `installer/service.go:66` 6-step workflow or wire to execution.

---

## 7. What Forge does better than every reference (keep)

- Desired/actual split + `observed_generation`/`desired_generation` + `state_transitions` audit — no ref had it.
- `suspended` bool orthogonal to `status` vs Ptero `status='suspended'` wiping install state.
- `server_orphan_remediations` tracking with `RecordOrphanAndHardDeleteServer:18` — Pterodactyl silent orphan.
- Beacon `1000:1000` `ReadonlyRootfs` `CapDrop ALL` `no-new-privileges` installer (`docker.go:304`) — Wings default wider.
- `rootfs.FS` `openat2 RESOLVE_BENEATH` (`secure_files.go:60`) + staged extraction `commitStaging/rollbackArchiveCommit` — Wings single-layer `SafePath`.
- `protocol`+`containerPort` explicit per allocation vs Wings dual tcp/udp `Bindings()` confusion.
- Bulk Tx `CreateAllocations:134` with `23505` handling, atomic `DeleteAllocations:220` rollback — Pterodactyl per-row.
- Bounded console `consoleManager:57` `Replay 128/256KB` + fan-out drop-if-slow vs Wings unbounded `io.Copy(Wrapper)`.
- Persisted power state `dataDir/.beacon-state` `persistPowerState:387` surviving daemon restarts — Puffer `Installing=true` memory-only.
- Control-plane-mediated transfer v1 `ProtocolVersion="forge-beacon-transfer/v1"` with dual HMAC-scoped credentials + resumable chunks (`transfer/protocol.go`) — retires Wings master-token broadcast (`server.go:360` legacy removed comment).

---

## 8. What to borrow (inspiration only)

- PufferPanel 24 typed ops with `if: file_exists` + CEL helpers — inspire modded Minecraft heavy pipeline (Fabric/CurseForge version resolution `resolveforgeversion` etc) for `install_script` hardening, but keep shell for most games.
- `parser` 6 parsers patching `server.properties` at boot — copy verbatim (~300 LoC) into `beacon/internal/parser` and call in `onBeforeStart:652`.
- `SubuserController:154` subset enforcement — copy actor-perm subset pattern.

---

## 9. Dead / Duplicate / False-completion (for index)

- **DEAD:** `installer/service.go:66` 6-step workflow persists rows never executed (`REF-GAME-HIDDEN-01`). `server_orphan_remediations` has no admin resolution UI (`handlers_admin.go` lacks `GET /orphans`). `forge/packages/game-templates` phantom on worktree — `main` cannot build/validate.
- **DUPLICATE:** Three template systems (DB eggs vs FS `game-templates` 14 vs localStorage `app-templates` vs `EGG_TEMPLATES` static vs `server_templates` shim `043`). `Allocations Mappings` legacy compat duplicates `Ports[]` explicit.
- **FALSE_COMPLETION:** Buildpacks view `builds-view.tsx:110` Build button permanently `disabled`. `transfer` progress `progress:394` may be null always — bar `width clamp(progress)` may never move despite `server.transferState` text changing. `POST /power` 202 accepted when `installing` actually will fail async — false liveness.

---

*Generated 2026-08-24 for `audits/final-parity/subagent-01-game-hosting.md` — read-only, no product code modified. All citations `file:line` verified by direct `read` at audit time. Cross-check with `audits/phase-02/subagent-*.md` (5), `phase-06/subagent-05+06` for full 27-repo corpus context.*

