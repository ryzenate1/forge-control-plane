# Phase 02 — Subagent 01 — Server Lifecycle & State Machine

**Dimension:** server lifecycle (create/install/reinstall/suspend/unsuspend/start/stop/restart/kill/migrate/transfer/backup/restore/delete), state machine (desired vs observed, suspend, installing, restoring), install flow, power actions  
**Clusters:** pelican-panel, pelican-wings, pterodactyl-panel, pterodactyl-wings, pufferpanel, pufferpanel-templates  
**Date:** 2026-08-23  
**Author:** subagent-1 (Muse Spark)

Scope per `forge/api/docs/server-lifecycle.md:1-16` (synchronous panel/Beacon lifecycle, provisioning→created≠installed, explicit install/reinstall, Beacon-first delete, transfer 501).

---

## 1. Methodology & Files Inspected

| Area | Paths |
|------|-------|
| Pterodactyl Panel model | `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:122-419` |
| Pterodactyl Panel migrations | `reference/game-hosting/pterodactyl-panel/database/migrations/2016_09_01_193520_add_suspension_for_servers.php:1-24`, `2021_01_17_152623_add_generic_server_status_column.php:20-42` |
| Pterodactyl Panel server management | `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Application/Servers/ServerManagementController.php:25-60` |
| Pterodactyl Panel transfer | `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Admin/Servers/ServerTransferController.php:35-88`, `app/Http/Controllers/Api/Remote/Servers/ServerTransferController.php:34-125`, `app/Models/ServerTransfer.php:26-100` |
| Pterodactyl Wings power | `reference/game-hosting/pterodactyl-wings/server/power.go:24-219` |
| Pterodactyl Wings install | `reference/game-hosting/pterodactyl-wings/server/install.go:33-571` |
| Pelican Panel enum | `reference/game-hosting/pelican-panel/app/Enums/ServerState.php:10-35` |
| Pelican Wings power diff | `reference/game-hosting/pelican-wings/server/power.go:178-219` (suspend guard same), `server/server.go:468`, `server/configuration.go:60-62` |
| PufferPanel daemon lifecycle | `reference/game-hosting/pufferpanel/servers/server.go:185-907`, `reference/game-hosting/pufferpanel/server.go:15-99` |
| PufferPanel templates | `reference/game-hosting/pufferpanel-templates/spec.json:1-120`, `reference/game-hosting/pufferpanel/operations/curseforge/curseforge.go:262-331` etc. |
| Forge lifecycle doc | `forge/api/docs/server-lifecycle.md:1-16` |
| Forge store servers | `forge/api/internal/store/store_servers.go:180-306`, `store_servers_control.go:13-274`, `store_servers_lifecycle.go:12-57`, `store_state.go:10-176` |
| Forge migrations | `forge/api/migrations/001_init.sql:14-40`, `021_true_state_persistence.sql:1-80`, `028_server_provisioning_parity.sql:1-6`, `004_server_transfer_state.sql:1-6`, `005_server_transfer_lifecycle.sql:1-5`, `040_truthful_server_lifecycle.sql:1-38`, `045_real_server_transfer.sql:1-29` |
| Forge API handlers | `forge/api/internal/http/handlers_servers.go:283-1782` |
| Forge cluster manager | `forge/api/internal/services/clustermanager/service.go:76-758` |
| Forge operation service | `forge/api/internal/services/operation/service.go:28-510` |
| Forge installer service | `forge/api/internal/services/installer/service.go:1-108` |
| Forge migration service | `forge/api/internal/services/migration/service.go:102-450` |
| Forge reconciler | `forge/api/internal/services/reconciler/service.go:267-695` |
| Forge orchestrator suspension | `forge/api/internal/orchestrator/suspension.go:13-110` |
| Beacon manager | `beacon/internal/server/manager.go:30-960` |
| Beacon HTTP + power/install | `beacon/internal/server/server.go:727-1600` (`create:727`, `syncConfiguration:836`, `install:1044`, `installWS:1151`, `reinstall:1323`, `power:1349`, `delete:1425`) |
| Beacon runtime docker | `beacon/internal/runtime/docker.go:151-510` (`Create:151`, `Install:256`, `Start:388`, `Stop:434`, `Kill:494`, `Restart:529`) |
| Beacon installer ops | `beacon/internal/installer/operations/operation.go:1-73` |
| Frontend | `forge/web/lib/api/servers.ts:88-109`, `forge/web/components/server/console-view.tsx:125-243`, `forge/web/components/lifecycle/lifecycle-machines.ts:41-114` |

---

## 2. Capability Comparisons (×18)

### C01 — Server Create & Initial Status (`provisioning`→`created`≠`installed`)

- **REFERENCE: pterodactyl-panel `app/Models/Server.php:138`** `protected $attributes = ['status'=>self::STATUS_INSTALLING]` — new rows are `installing` by default; `isInstalled():215` returns `status !== installing && status !== install_failed`. No separate `provisioning` — Wings creates container after DB row; panel status is installing until `SyncInstallState` reports `installed_at`.
- **REFERENCE: pufferpanel `servers/server.go:357-369`** `Create()` → `RunningEnvironment.Create()` — no DB status; filesystem creation distinct from `Install()` (`410-466`) which runs `Installation` ops only after `IsIdle()` (no running / backingUp / restoring / installing). `Install()` is separate step, can be called any time server is idle.
- **REFERENCE: pufferpanel-templates `spec.json:51-58`** `install: []Operation` separate from `run.command` — template carries install ops atomically; daemon interprets them.

- **FORGE Frontend:** no dedicated "provisioning" UI step; `forge/web/lib/api/servers.ts:66` `createServer()` POSTs `/servers` and receives `ApiServer` with `status` (`provisioning`|`created`|…​). Console-view `blocked` logic checks `server.status === "installing"` via `POWER_MACHINE`? Actually `console-view.tsx:60` comment "cannot be started, stopped, or modified" when installing; power buttons disabled via `blocked` derived from installing/transfer flags (not shown but handled server-side via `ensureTransferIdle`).
- **FORGE API:** `handlers_servers.go:776-873` `POST /servers` → `clusterManager.CreateServer()` inside single transaction (store), then Beacon `POST /servers`; success marks panel `created` **not** installed (`store_servers_control.go:204-215` `SetServerProvisioned` → `status='created', installed=false`). Lifecycle doc `server-lifecycle.md:3-5`.
- **FORGE Service:** `clustermanager/service.go:76-170` `CreateServer` does placement, `CreateServer` DB tx (`store_servers.go:253-265` `INSERT status='provisioning'`), then `provisionServer:172-191` → `runtime.CreateServer` → `SetServerProvisioned` only on accept. Compensation `compensateCreateFailure:285-302` handles Beacon-first delete with orphan remediation.
- **FORGE Store:** `store_servers.go:253` `VALUES … 'provisioning','stopped','stopped'` — three-state init; `store_servers_control.go:204` `provisioned → created`.
- **FORGE DB:** `migrations/001_init.sql:24` `status TEXT DEFAULT 'stopped'`; `021_true_state_persistence.sql:34` adds `desired_state`/`actual_state`; `088_server_parity_fields.sql:3` `installed_at`; `040_truthful_server_lifecycle.sql:4-13` cleans stale `transferring`/`installing`.
- **FORGE Worker:** none (sync create path, 15-minute timeout).
- **FORGE Beacon:** `server.go:727` `create` → `runtime.Create` → persist `runtime.json` → `manager.MarkCreated:312` (`InstallationState=installed`, `ContainerExists=true`).
- **FORGE Event:** `clustermanager/service.go:165` `EventServerCreated` with `desiredState`/`actualState`/`correlationId`; `store_servers.go:298` `audit_events 'server provisioning started'`.
- **FORGE Reconciliation:** `reconciler/service.go:476-493` `reconcileServer` compares `desiredState` vs `storeActual`, publishes `EventActualStateChanged` with `remediate:true` if drift.
- **FORGE Tests:** `store_servers_lifecycle_integration_test.go` (create→provisioned), `server_test.go:186` power rejects invalid signal.

- **STATUS:** **Implemented (divergent — more truthful)**  
- **GAP:** Pterodactyl's single `status` overloaded (installing == provisioning). Forge correctly splits `provisioning` (DB-only, never exposed to runtime) vs `created` (workload exists, not installed) vs `installing`/`install_failed`/`stopped` — Pelican already moved to `ServerState` enum (5 values) but still uses `installing` as creation default. Purge: Pterodactyl never had `provisioning`; clients polling `GET /servers/:id` see `installing` during provision where Forge shows `provisioning`.
- **FORGE LOGIC FINDING:** See LF-04 (create hash race).  
- **RECOMMENDATION:** Keep divergence; document `provisioning` is panel-only pre-Beacon accept state (already in `server-lifecycle.md:5`). Frontend should surface `provisioning` badge distinct from `installing` — currently `lifecycle-machines.ts:41` `POWER_MACHINE` has no `provisioning` step, so badge falls to raw `status` string; add explicit mapping.
- **SEVERITY:** Low (positive divergence)

---

### C02 — Desired vs Actual State Machine (desired_state/actual_state vs overloaded status)

- **REFERENCE: pterodactyl-panel `Server.php:122-126`** constants only `installing/install_failed/reinstall_failed/suspended/restoring_backup`; `status` nullable string; no desired/actual split. `validateCurrentState:390-400` bundles suspend/install/transfer/restoring into exception.
- **REFERENCE: pelican-panel `ServerState.php:12-16`** enum 5 cases (`Installing`, `InstallFailed`, `ReinstallFailed`, `Suspended`, `RestoringBackup`) — still single column; `Server.php:193` casts `'status'=>ServerState::class`; actual container power not tracked in DB (Wings reports `ProcessOffline` etc).
- **REFERENCE: pterodactyl-wings `environment/`** `ProcessOfflineState` vs `ProcessOnline` etc but not persisted across Wings restarts except `process` state.

- **FORGE Store:** `store_state.go:10-54` `SetServerDesiredState`/`SetServerActualState` with `desired_generation`/`observed_generation` + `state_transitions` audit (`recordStateTransition:116-129`). `serverStatusFromActual:131-146` maps actual enum back to legacy `status` for reads (`running↔running`, `installing↔installing`, `restoring_backup`, `crashed↔install_failed`, `unknown`, `stopped`). Dual-write keeps API compat.
- **FORGE DB:** `021_true_state_persistence.sql:3-35` introduces `server_desired_state ENUM('running','stopped')` + `server_actual_state ENUM('running','stopped','starting','stopping','installing','crashed','unknown')` + `state_transitions` table. Also `087_a_parity_schema.sql`, `150_heartbeat_reconciling_state.sql`.
- **FORGE Service:** `clustermanager/service.go:382-411` `RequestServerPower` sets desired first, publishes `EventDesiredStateChanged`, then `sendPower` sets actual, publishes `EventActualStateChanged`+ `EventServerStarted/Stopped/Restarted` (`publishServerPowerEvents:576-591`). `RefreshServerActualState:412-447` probes runtime `Stats` to infer running, flips unknown→running. `ReconcileNode:449-496` publishes drift events when `desiredRunning ≠ actualRunning`.
- **FORGE API:** `store_servers.go:33` selects `desired_state::text`, `actual_state::text`; DTO exposes both (`store.go:452-453`). Handlers not blocking on drift.
- **FORGE Beacon:** `manager.go:21-29` `PowerState` offline/starting/running/stopping + `InstallationState` installing/installed/failed — observed, not desired. `Reconcile:128-172` loads persisted `ExpectedStop` + `LastStartedAt` and inspects runtime to set `PowerState`; `HandleContainerEvent:879-955` flips `PowerState` on die/start and triggers crash-restart.
- **FORGE Reconciliation:** `reconciler/service.go:476-493` full diff engine using `observed_generation`/`desired_generation` pattern (capture/ computeDiffs / autoReconcileDrifts).
- **FORGE Frontend:** `lifecycle-machines.ts:41` `POWER_MACHINE` offline→starting→running→stopping; `store_state.go` desired/actual not yet visualized as two lanes — single badge reads `status`.
- **FORGE Worker:** `operation/service.go:152-183` reaper for stale `running` ops.

- **STATUS:** **Enhanced (superset of reference)**  
- **GAP:** Reference panels have no durable desired/actual split; reconciler must infer. Forge adds generation fencing (`desired_generation`/`observed_generation`) and `state_transitions` audit absent in all 6 refs. Pelican/Wings power is purely runtime-ephemeral; Forge persists across API restarts via DB, Beacon persists across daemon restarts via `stateDir/.beacon-state` (`manager.go:124-126`).
- **FORGE LOGIC FINDING:** Generation increment `CASE WHEN desired_state IS DISTINCT FROM $1 THEN +1` is correct, but `observed_generation` only reconciles when desired==actual active overlap (`store_state.go:40-42`) — if server is `crashed` with desired `running`, observed never catches up, reconciler will loop. See LF-02.  
- **RECOMMENDATION:** Surface both states in UI (already have `ServerDTO.DesiredState/ActualState`): render two-dot badge like `Apps` health lanes. Add integration test for desired→actual convergence under crash.
- **SEVERITY:** Medium (feature completeness)

---

### C03 — Suspend / Unsuspend

- **REFERENCE: pterodactyl-panel `Server.php:125,218-221`** `STATUS_SUSPENDED='suspended'`, `isSuspended:218` `status===suspended`; migration `2016_09_01_193520:14` `suspended tinyint(0/1)`; later `2021_01_17:21` backfills `suspended`→`status`. `SuspensionService.php:42` throws `ConflictHttpException` if transferring. `ServerManagementController.php:29-45` `suspend`/`unsuspend` via `SuspensionService->toggle(server)` (single endpoint per action).
- **REFERENCE: pelican-panel `ServerState.php:15,25,34`** `Suspended='suspended'` enum; `Server.php:251-253` `isSuspended => status===Suspended`; `SuspensionService.php:31` guard same transfer check; `AuthenticateServerAccess.php:53` blocks client API unless `routeIs('api:client:server.resources')` when suspended; Wings `server.go:283` immediately disconnects websockets when `is_suspended` flipped; `update.go:56-59` suspend with running process → `Terminate`.
- **REFERENCE: pufferpanel** no suspend concept — `servers/server.go:884-899` `IsIdle` only checks `IsInstalling`/`IsBackingUp`/`IsRestoring`/`IsRunning`; suspension is not a lifecycle state (admin can just stop server or remove permissions).
- **REFERENCE: pterodactyl-wings `power.go:177-181, server.go:364`** `onBeforeStart:177` refuses start/restart if `IsSuspended()` (after `Sync()`), `HandlePowerAction:57-64` blocks power while `IsInstalling/IsTransferring/IsRestoring`. No dedicated `suspended` HTTP endpoint — wings receives boolean `suspended` in server config JSON (`configuration.go:62`) and updates in `update.go:56`.

- **FORGE Store:** `store_servers.go:374-410` `SetServerSuspension` (raw `UPDATE suspended`), `CompareAndSetServerSuspension` (CAS), `SetServerSuspended` with audit (`action server suspended/unsuspended`). `IsServerTransferBlocking` only checks `transfer_state` not suspend. `Server` struct `store.go:454` `Suspended bool` separate from `Status` (legacy dual-write). Suspend does NOT map to `status='suspended'` — unlike Pterodactyl/Pelican where `status` *is* `suspended`. Forge keeps `suspended` boolean orthogonal to `actual_state`/`status` (`store_state.go:131` never returns `suspended`).
- **FORGE DB:** `001_init.sql:14` no `suspended`; added via `007_postgres_core_foundation.sql:38` `suspended BOOLEAN DEFAULT FALSE`; `migrations` never migrate to status-enum `suspended` — intentional decoupled flag.
- **FORGE API:** `handlers_servers.go:1400-1459` **three** endpoints: `POST /servers/:id/suspension` (`action:suspend|unsuspend`, best-effort power-off on suspend `1422-1428`), plus legacy compat `POST /servers/:id/suspend` and `/unsuspend` (`1437-1459` raw `SetServerSuspension`). Admin `servers.write` scope; no transfer guard here (correctly deferred to `store` CAS via `orchestrator/suspension.go:28-110` which *does* guard transfer? Actually orchestrator guard checks `server.Suspended` already — see below). No `AuthenticateServerAccess` middleware analogue — permission checked per-route via `requireServerPermission`.
- **FORGE Service:** `orchestrator/suspension.go:28-48` `SuspendServer` verifies `!already suspended`, sets via `CompareAndSet`? Actually `suspension.go:40-73` does `GetServer` → CAS → `publisher` `EventServerSuspended` with payload `{suspended:true}` (`73`), publishes after DB. Calls `SetServerSuspension` directly, not CAS inside — but exposes `CompareAndSetServerSuspension:385`. `clustermanager/service.go:394-396` `RequestServerPower` refuses `start|restart` if `Suspended`.
- **FORGE Beacon:** `manager.go:610-634` `onBeforeStart` reads `state.Suspended` and refuses `start`/`restart`/`HandlePower start/restart` with `"server is suspended"`; `HandlePower:487-498` also checks `InstallationState==installing` first. `Reconcile:135` syncs `Suspended` from panel reconstruction. `manager.go:744-752` panel sync (`syncServerStateFromPanel`) also syncs `Suspended` every `onBeforeStart` when `PanelURL` set, so eventually consistent even if DB flip races start.
- **FORGE Worker:** no async suspend worker; synchronous `SetServerSuspended` + best-effort stop (`handlers_servers.go:1424` `Daemon.SendPower stop` + `SetServerPowerState stop`).
- **FORGE Frontend:** `web/lib/api/servers.ts:102-108` `suspendServer`/`unsuspendServer` both `POST /suspension {action}`; console-view disables power buttons when `blocked`? Actually `console-view.tsx:125` `canPower` checks `control.start/restart/stop` permissions, not suspend flag — will still show buttons but Beacon/BE will reject with 500/403.
- **FORGE Reconciliation:** `reconciler/service.go:590` if `server.Suspended` then diff includes `suspended:true` and plans remediation.

- **STATUS:** **Implemented — partial parity gap (dual flag vs enum)**  
- **GAP:** Forge keeps `suspended` as boolean orthogonal to `status` (good: you can tell `install_failed` AND `suspended`), whereas Pterodactyl/Pelican conflate `status='suspended'` wiping prior install/restoring state. Forge therefore needs dual-write audit for `suspended` toggles (implemented). Missing: Pterodactyl's `AuthenticateServerAccess.php:53` blanket client-API gate when suspended (except `resources`); Forge achieves same via `ensureTransferIdle` + `hasServerReadEnv` but not via middleware — every handler must remember to check `Suspended` before power; `handlers_servers.go:875` `ensureTransferIdle` does **not** check `Suspended` for power, only transfer; suspension is only checked inside `clustermanager RequestServerPower:394` and Beacon `onBeforeStart` — so `/servers/:id/power stop|kill` still succeeds for suspended server (intentionally allowed to stop a suspended running workload, but `start` should be blocked earlier). Pelican `ServerManagementController.php` lacks force-stop on suspend; Forge adds best-effort stop (good).
- **FORGE LOGIC FINDING:** See LF-01 (kill bypass during install vs suspend).  
- **RECOMMENDATION:** Align with Pelican guard `SuspensionService:42` "cannot toggle suspension while transferring" — add `ensureTransferIdle` to `/suspension` handler too (currently missing; caller could suspend mid-transfer, leaving `transfer_state=running` but `suspended=true`). Add client middleware `AuthenticateServerAccess` equivalent in Forge gateway (403 for suspended client ops except `GET /servers/:id/resources`).
- **SEVERITY:** Medium

---

### C04 — Power Actions (start/stop/restart/kill) — semantics & locking

- **REFERENCE: pterodactyl-wings `server/power.go:24-219`** `PowerAction` constants `start/stop/restart/kill`; `IsValid:32`, `IsStart:39`; `ExecutingPowerAction:45` via `powerLock.IsLocked()`; `HandlePowerAction:56-167` — first guard `IsInstalling||IsTransferring||IsRestoring` → `ErrServerIs*`; then acquire `powerLock` (for `kill`, lock is optional — try, ignore failure `115-119` so terminate can break stuck lock); switch: `start` checks `Environment.State()!=offline` → `ErrIsRunning`, calls `onBeforeStart:171-218` (sync, suspend, env sync, disk, config files, chown); `stop`/`restart` `WaitForStop(timeout 10m)` then optional `Start`; `kill` → `Environment.Terminate(SIGKILL)`. `onBeforeStart:172` syncs panel fresh.
- **REFERENCE: pelican-wings** identical plus configurable `CheckPermissionsOnBoot`.
- **REFERENCE: pufferpanel `servers/server.go:185-353`** `Start` → `IsIdle()` check, `isUnsafeRunning.TryLock()` else `ErrServerRunning`, `PreExecution` → `ExecuteAsync` (keepAlive ticker); `Stop:320` checks `IsRunning` then `SendCode` or `ExecuteInMainProcess(stopCommand)`; `Kill:343` `Kill()` regardless of IsIdle? Actually `Kill` has no IsIdle guard. Lock is per-server `isUnsafeRunning` mutex (only Start held until `afterExit:592` unlocks).
- **REFERENCE: pterodactyl-panel** no direct power DB state; panel proxies to daemon via `DaemonPowerRepository`.

- **FORGE API:** `handlers_servers.go:875-940` `POST /servers/:id/power` → `ensureTransferIdle` → validate `signal in {start,stop,restart,kill}` → `requiredPermission` `control.start` (start), `control.stop` (stop/kill), `control.restart` (restart) → idempotencyKey `power:{id}:{signal}:{clientKey}` → `OperationService.DispatchPower` (durable) **or** `QueueService.DispatchIdempotent` fallback; returns `202 {operationId,mode}`. No wait loop — purely durable; legacy `?force` not on power.
- **FORGE Service:** `operation/service.go:351-383` `DispatchPower` maps signal→Op type (`server.start/stop/restart/kill`), creates deterministic ID via `uuid.NewSHA1(uuid.NamespaceURL,"forge-op:"+idempotencyKey)`; handler not registered here — actual power execution wiring in `cmd/api/main.go:??` wires handlers to `clustermanager.Start/Stop/Restart/Kill→sendPower:532-575` which checks `Suspended` for start/restart, sets `SetServerActualState` and publishes events, then delegates to `runtime.Start/Stop/Restart/Kill`.
- **FORGE Store:** `store_servers_control.go:13-55` `SetServerPowerState` maps `start/restart→running`, `stop/kill→stopped` with `powerSignalPriorStates` allowlist (`start:[created,stopped,install_failed]`, `restart:[running,created,stopped,install_failed]`, `stop/kill:[running,installing,provisioning,restoring_backup,created,stopped]`). Updates `status` + audit.
- **FORGE Beacon:** `manager.go:482-608` `HandlePower` TryLock `RunningAction` slot, guard `InstallationState==installing` (`494`), then per signal: `start:511-533` `onBeforeStart` → `runtime.Start` → `PowerState=Running`; `stop:534-548` → `stopServer` → `offline`; `restart:549-580` `onBeforeStart`→`stopServer`→`Start`; `kill:581-595` → `runtime.Kill`. `server.go:1349-1410` `power` HTTP handler enqueues `operations.EnqueueCommand` and **blocks** up to 30s polling `GetStatus` until `completed/failed` or timeout (`1399-1405`) → differs from Forge durable queue (Beacon still synchronous-ish).
- **FORGE Worker:** `operation/service.go:152-183` 5 workers dequeue, `process:229-295` marks `running`→handler→retry with exponential backoff (base 1s, max 30s, maxRetries 3) or `succeeded`/`failed`; `reaper:164-183` reaps stale `running>5m`.
- **FORGE DB:** `store_state.go:165-176` helpers `serverDesiredFromSignal`/`serverActualFromSignal` map signal→desired/actual.
- **FORGE Frontend:** `web/lib/api/servers.ts:88-96` `sendPowerSignal`; `web/components/server/console-view.tsx:125-243` `canPower` derives permission, disables button when `blocked` or `power.isPending` or `server.status` mismatched (`signal===start ? status===running : status!==running`).
- **FORGE Event:** `clustermanager/service.go:576-591` `publishServerPowerEvents` emits `EventActualStateChanged` + `EventServerStarted/Stopped/Restarted`.
- **FORGE Reconciliation:** `reconciler/service.go:476` diff includes power state.

- **STATUS:** **Implemented — enhanced durability + divergence on blocking behavior**  
- **GAP:** Pterodactyl Wings **blocks** power during `IsInstalling/IsTransferring/IsRestoring` with specific `ErrServerIs*` (clear 409). Forge's durable path `ensureTransferIdle:145-159` → 409 `"server transfer in progress"` for any signal (good), but `installing`/`restoring_backup` not checked — only Beacon `manager.go:494` blocks installing. Pelican/Pterodactyl also block `stop` during `installing`; Forge allowlist `powerSignalPriorStates:51` **allows** `stop/kill` during `installing/provisioning/restoring_backup` — divergent intent (Forge allows interrupting install via stop/kill; Wings forbids except `kill` bypass).
- **FORGE LOGIC FINDING:** LF-01 — see below. LF-05 timeout mismatch (Beacon 30s wait inside HTTP vs Forge 202 immediate).  
- **RECOMMENDATION:** Make allowlist explicit choice: if interrupting install should cancel installer container + mark `install_failed`, document it; otherwise restrict `stop` during `installing` to `kill` only. Add client-disabled logic for `kill` not following same perm (currently `control.stop` for kill is correct matching Wings `Terminate` permission is `control.stop`? In Pterodactyl `power` perms: start vs stop — kill uses stop perm, correct).
- **SEVERITY:** High (power semantics)

---

### C05 — Kill Bypass Lock (`SIGKILL` ignoring powerLock)

- **REFERENCE: pterodactyl-wings `power.go:108-120`** `if action != PowerActionTerminate { Acquire/TryAcquire with defer cleanup } else { // kill → Try Acquire, but ignore failure: "failed to acquire exclusive lock, ignoring failure for termination event" }` — kill can pierce stuck power lock.
- **REFERENCE: pufferpanel** `Kill:343-353` no lock attempt, directly `RunningEnvironment.Kill()`.

- **FORGE Beacon Runtime:** `docker.go:494-509` `Kill` does `workloadLock` per-server sharded mutex (`workloadLock:251` sha256), locks, inspects, if running `ContainerKill SIGKILL`. No bypass — Kill still contends on same `workloadLocks[64]` as `Stop/Start`. `manager.go:581-595` `HandlePower` `kill` still claims `RunningAction` slot (`487-506` same `TryLock`), no bypass.
- **FORGE API/Operation:** `operation/service.go:368-375` kill maps to `OpServerKill`; handler in clusterManager `KillServer:376-380` → `sendRuntimePower` → `runtime.KillServer`; still queued behind durable operation worker FIFO.
- **FORGE Store:** `powerSignalPriorStates:51` kill allowed from 7 states, but durable queue ordering still serializes.

- **STATUS:** **Gap — kill does NOT pierce stuck operation**  
- **GAP:** Wings explicitly documents "if server is currently trying to process a power action but has gotten stuck you still should be able to pass through the terminate event" (`power.go:81-83`). Forge queues kill behind stuck operation (worker poll 1s + oper slot claimed), so a hung install/power holds lock → kill waits for lock release or timeout, defeating emergency kill. Beacon sharded `workloadLocks` mitigate container-level deadlock but `manager.RunningAction` still blocks kill.
- **FORGE LOGIC FINDING:** See LF-06 below.  
- **RECOMMENDATION:** Mirror Wings bypass: in `manager.go:HandlePower` and `operation/service.go:process`, if `signal==kill` acquire with `TryLock` + don't fail if occupied; or keep kill on separate semaphore. Document kill's `control.stop` perm still required.
- **SEVERITY:** Medium

---

### C06 — Install Flow (first-time, explicit POST)

- **REFERENCE: pterodactyl-wings `install.go:33-114`** `Install() → install(false)` → if not `SkipEggScripts` publish `InstallStarted`, `internalInstall()` (`GetInstallationScript`→`NewInstallationProcess`→`Run()`), then `SyncInstallState(err==nil,reinstall)` → `Environment.SetState(Offline)` → publish `InstallCompleted`. `Run:195-227` secures `installing.SwapIf(true)` guard, `BeforeExecute` (write script, pull image, remove old container), `Execute` (create `*_installer` container with mounts, tmpfs, resources, `Config.Labels ContainerType=server_installer`), streaming via `DaemonMessageEvent`, wait, `AfterExecute` collects logs to `install.log` (template includes env vars).
- **REFERENCE: pterodactyl-panel `Services/Servers/ServerCreationService.php` etc** panel `POST /api/application/servers` creates row `installing` + enqueues install? Actually install is daemon-pushed.
- **REFERENCE: pufferpanel `servers/server.go:410-466` Install** → `IsIdle` guard, `SetInstalling(true)` defer false, ensure `MkdirAll(root)`, if `Installation` ops non-empty `GenerateProcess(Installation,...)` → `Run`. No container isolation for puffer (operations run on host).
- **REFERENCE: pufferpanel-templates `spec.json:51`** `install` array of `operation` (`download`, `unzip`, `exec` etc) — declarative per-template.

- **FORGE API:** `handlers_servers.go:1037-1082` `POST /servers/:id/install` admin `servers.write`; `ensureTransferIdle:1038`; durable: if `OperationService !=nil` validate exists, build `idempotencyKey "install:{id}:{clientKey}"` → `DispatchInstall` → `202 {operationId}`; fallback sync `clusterManager.InstallServer` with 15m timeout → webhook `server:installed`.
- **FORGE Service:** `clustermanager/service.go:216-263` `runInstaller(reinstall bool)` → `SetServerInstallState installing` → `syncProvisionTarget` → `runtime.InstallServer` or `ReinstallServer`; on error `SetServerInstallState failed`; on success `installed`; checks accept+exitCode==0 else failed.
- **FORGE Store:** `store_servers_control.go:217-241` `SetServerInstallState` state machine `installing→installed/failed` sets `status` + timestamps `install_started_at`/`install_completed_at`/`install_failed_at`/`install_error` (see `001_init` etc). Separate from `desired/actual`.
- **FORGE Worker:** `operation/service.go:474` `DispatchInstall` uses `uuid.NewSHA1("forge-op:server.install:"+idempotencyKey)` idempotent.
- **FORGE Beacon:** `server.go:1044-1149` `install` HTTP → validate `serverId`, `safePath`, `MkdirAll`, `BeginInstall` (claim), write `install.sh`, `runtime.Install(InstallRequest{Image,Entrypoint,Script,Env,RootDir})`, save `install.log`, `EndInstall(failed)`, `notifyPanelInstallStatus` (POST to panel). `installWS:1151-1321` streaming WS requires `ScopeAdmin` (`1170`) — prevents tenant console-token abuse. `docker.go:256-345` `Install` runs `mgp-{id}-installer` container as `1000:1000`, read-only rootfs, `CapDrop ALL`, `no-new-privileges`, `Tmpfs /tmp 64M`, pulls image, wait, caps log 1MiB, cleanup.
- **FORGE Installer Service:** `installer/service.go:66-76` `CreateInstallWorkflow` builds 6-step workflow `docker.create/filesystem.setup/download.server/script.install/config.apply/server.start` but is **not** wired to beacon install (orphan, unused — only persists workflow row).
- **FORGE DB:** `migrations/032…`? plus `store_installer` tables `install_workflows/install_steps`.
- **FORGE Frontend:** `console-view.tsx:45-60` reinstall banner "Do not restart …", `reinstallServer` `servers.ts:98` POST `/reinstall`; no separate first-install button (auto).
- **FORGE Event:** `clustermanager` `install failed` publishes no event? Actually only on success? Store writes audit `server install failed/installed`.

- **STATUS:** **Implemented (enhanced security & durability)**  
- **GAP:** Pterodactyl Wings **skips install if `skip_scripts`** (`install.go:38`); Forge never checks `skip_scripts` — panel doc `server-lifecycle.md:15` notes build model persists `skip_scripts` but Beacon `create` still honors? `store_servers.go:264` inserts `skip_scripts`, `beacon/server.go:??` create does not read `skip_scripts` from `server.json`; install always runs if admin calls `/install`. Pufferpanel's declarative `install: []` with 0 ops is explicit no-op; Forge's installer container fallback script `server.go:1098` `"No install script configured"` covers empty but not `skip_scripts` flag. Also Pterodactyl streams install logs via websocket + `install.log` (like Forge), but Beacon caps at 1MiB (`docker.go:340` `io.LimitReader 1<<20`) vs Pterodactyl unlimited file.
- **FORGE LOGIC FINDING:** LF-07 (idempotency + auth scope).  
- **RECOMMENDATION:** Honor `skip_scripts` in `runInstaller` (early `installed` if true, matching `install.go:47`). Wire `installer/service.go` workflows to real execution or remove dead code (currently dead). Ensure `installWS` scope check mirrors `install` non-WS (which lacks admin-scope check — `server.go:1044` has no auth-scope guard unlike `1151`).
- **SEVERITY:** Medium

---

### C07 — Reinstall Flow (requires stopped)

- **REFERENCE: pterodactyl-wings `install.go:80-94`** `Reinstall()` → if `Environment.State()!=offline` → `WaitForStop(10s, true)` else `Sync()` then `install(true)`; notify panel with `reinstall:true`. `ServerManagementController.php:55-59` `reinstall(Server $server)` → `ReinstallServerService->handle(server)` → sets `status=reinstall_failed`? `isInstalled` also checks `reinstall_failed`.
- **REFERENCE: pelican-wings/panel** same but `ReinstallFailed` status distinct.
- **REFERENCE: pufferpanel** no distinct reinstall — `Install()` same as reinstall (stops first `426-432` `if r { Stop() }`).

- **FORGE API:** `handlers_servers.go:1097-1137` `POST /servers/:id/reinstall` requires `store.PermSettingsReinstall` (vs `admin` for `install`), `ensureTransferIdle`, durable via `DispatchReinstall` (`1116`) or `clusterManager.ReinstallServer` sync. No stopped-state pre-check at API layer.
- **FORGE Service:** `clustermanager/service.go:220-221` `ReinstallServer → runInstaller(true)` → same path as install but calls `runtime.ReinstallServer` if `Reinstaller` interface; Beacon's `Reinstaller` is not Docker but same install path? Actually `runtime.go` `Reinstaller` exists, Docker implements `Reinstall` as `Install`? Need check `reinstaller` — `runtime/docker.go` does not have `Reinstall`, so Forge will error `"runtime does not support reinstall"` (`243`) and mark `failed`. Beacon HTTP `reinstall:1323-1335` checks `PowerState==Running|Starting → 409` then forwards to `install` (same). So Forge's `ReinstallServer` via `clusterManager` will fail if runtime is pure Docker (unless poly runtime).
- **FORGE Store:** `SetServerInstallState` same state machine.
- **FORGE Beacon:** `server.go:1323` `reinstall` guard `state.PowerState == running|starting → 409 "server must be stopped"`, then `install` (claim install). Matches Pterodactyl `WaitForStop` semantics but stricter (Pterodactyl waits 10s; Forge rejects). Also `manager.go` `HandlePower` `restart` already handles stopping; reinstall must be explicit stop first via power op.
- **FORGE Frontend:** `web/components/server/console-view.tsx:??` reinstall button disabled when running? Actually `console-view` `reinstallServer` call no guard, but power controls handle.

- **STATUS:** **Gap — reinstall not supported on pure Docker runtime**  
- **GAP:** Reference panels treat reinstall as same installer flow (Pterodactyl `install(true)`). Forge introduces separate `OpServerReinstall` but `clustermanager/service.go:242-243` gating `Reinstaller` interface means Docker deployments will always fail reinstall with 500. Beacon's `reinstall` works (just calls `install`). Drift: API `reinstall` requires `PermSettingsReinstall` (subusers can trigger), while `install` requires admin — matches Pterodactyl/Pelican permissions (`ServerManagementController` reinstall is app-level admin only, but Wings panel reinstall allowed for server owner? Pelican `Filament` reinstall disabled when suspended).
- **FORGE LOGIC FINDING:** See LF-08.  
- **RECOMMENDATION:** Implement `ReinstallServer` fallback to `InstallServer` when runtime lacks `Reinstaller` (already what Beacon does). Remove interface gate or implement Docker `Reinstall` alias. Add API guard to reject reinstall when `DesiredState==running` with 409 instructing stop first (like Beacon does), rather than marking install `failed` and leaving `actual=install_failed`.
- **SEVERITY:** High

---

### C08 — Suspend/Unsuspend (already C03) — split for state machine

(Consolidated — see C03. Adding install-overlap detail:)

- **Reference:** Pelican `ServerInstallController.php:52-54` `if status===Suspended then keep status=Suspended after install` — suspended servers stay suspended post-install.
- **Forge:** `store_servers_control.go:217-241` `installing→installed` always writes `status='stopped'` regardless of `suspended` flag (`stopped` even if suspended). Beacon `install:1133-1134` `EndInstall` sets `InstallationState installed` not `PowerState`; but `store:SetServerInstallState installing→installed` overwrites `status` to `stopped`, losing suspension tri-state. Need to ensure `suspended` bool preserved — it is (separate column), but UI badge may show `stopped`+`suspended:true` vs reference `status=suspended`.

---

### C09 — Delete (hard delete, Beacon-first, force & orphan remediation)

- **REFERENCE: pterodactyl-panel** soft-deletes? `DropDeletedAt` migration + `whereNull('successful')` transfer means delete via panel `ServerDeletionService` — stops server, removes allocations, deletes daemon via `DaemonServerRepository->delete`, hard delete row, cascading `server_variables`, `allocations` set `server_id=NULL`.
- **REFERENCE: pterodactyl-wings** `router/router_server.go:??` `DELETE /api/servers/{uuid}` removes container + files via `Server.Delete()`.
- **REFERENCE: pufferpanel `servers/server.go:373-408`** `Destroy()` → `IsIdle` guard, stop scheduler, run `Uninstallation` ops, `RunningEnvironment.Delete()`.

- **FORGE Doc:** `server-lifecycle.md:6` `DELETE /servers/:id` calls Beacon first, hard-deletes only after Beacon succeeds; `?force=true` always removes panel state; Beacon failure retained as `server_orphan_remediations`.
- **FORGE API:** `handlers_servers.go:1461-1481` `DELETE /servers/:id` → `clusterManager.DeleteServer(ctx, id, forceDelete)` → `202` or `202 warning` when force.
- **FORGE Service:** `clustermanager/service.go:332-360` `DeleteServer`: `ServerControlTarget` → `runtime.DeleteServer` → on error + `!force` return; on error + `force` → `RecordOrphanAndHardDeleteServer` + return `Mode:force` with daemonErr. On success → `HardDeleteServer`. Publishes `EventServerDeleted` with `force`/`orphaned`.
- **FORGE Store:** `store_servers_lifecycle.go:12-57` `HardDeleteServer` → tx lock row, `UPDATE allocations SET server_id=NULL`, `DELETE FROM servers` cascade. `RecordOrphanAndHardDeleteServer:17-22` inserts `server_orphan_remediations` **before** same delete (FK-free, so survives). Requires `daemonError != ""` (`19-21`).
- **FORGE DB:** `040_truthful_server_lifecycle.sql:18-38` `server_orphan_remediations(id,server_id,node_url,daemon_error,status=pending|resolved)` + index `pending`. Forge of panel Pterodactyl never had this table (orphans were silent).
- **FORGE Beacon:** `server.go:1425-1459` `delete` → `safePath`, `consoles.Stop`, `runtime.Delete`, list+delete backups, `os.RemoveAll(root)`, `manager.Delete`. `runtime/docker.go:625-638` `Delete` `ContainerRemove Force+RemoveVolumes`, idempotent `IsNotFound→nil`.
- **FORGE Worker:** no async delete confirm; single path.
- **FORGE Frontend:** `web/lib/api/servers.ts:74` `deleteServer(id,force)` maps force→`?force=true`.
- **FORGE Tests:** `store_servers_lifecycle_integration_test.go`.

- **STATUS:** **Implemented — enhanced (orphan tracking)**  
- **GAP:** Reference deletes are panel-transactional but do not record daemon failure for later remediation; Forge does. Missing: pufferpanel's `Uninstallation` ops (custom cleanup scripts) — Forge delete does not run egg `installScript` unwind or mount cleanup (except `docker RemoveVolumes:true`). Pelican/Pterodactyl Wings `transfer` cleanup deletes allocations from old node after success `ServerTransferController.php:81-92` — Forge `migration` cleanup separate.
- **FORGE LOGIC FINDING:** See LF-03 (allocation release ordering / FK).
- **RECOMMENDATION:** Add `DELETE /servers/:id?force` integration test verifying orphan row created when runtime `Delete` returns error; ensure `RecordOrphanAndHardDeleteServer` called with non-empty `daemonError` even when `runtime.Delete` returns context-cancelled.
- **SEVERITY:** Low

---

### C10 — Backup Restore & `restoring_backup` Blocking

- **REFERENCE: pterodactyl-panel `Server.php:126` `STATUS_RESTORING_BACKUP`, `validateCurrentState:396` returns StateConflict if `restoring_backup` or `transfer!=null`; `BackupController.php:200-210` cannot restore unless `isInstalled` && backup `is_completed`/`!failed`; sets `status=restoring_backup` then daemon `restore`.
- **REFERENCE: pterodactyl-wings `install.go:152-173`** `IsRestoring` atomic, `HandlePowerAction:57` blocks if `IsRestoring` → `ErrServerIsRestoring`; Wings `server.go:??` restore path sets `restoring` true.
- **REFERENCE: pufferpanel `servers/server.go:737-803`** `StartRestore` → `IsIdle` guard, `restoring=true`, deletes existing files, `Extract` archive, `restoring=false`; `IsRestoring:879` bool.

- **FORGE Store:** `store_state.go:131-135` `actual_state=restoring_backup` ↔ `status=restoring_backup`; `store_backup.go`? `backups` table has `status restoring|restored|restore_failed` per backup row, but **no** server-level `restoring_backup` status is set server-wide on restore? `handlers_servers.go:2157-2167` `/backups/restore` synchronous fallback sets per-backup status via `MarkBackupStatus restoring/restored/restore_failed` (`store_backups.go:??`) not server status. `store_state.go` never transitions `actual_state` to `restoring_backup` on backup restore — only install touches `installing`.
- **FORGE API:** `handlers_servers.go:2104-2168` `/servers/:id/backups/restore` → `resolveBackupName`, check `status==completed`, then `OperationService.DispatchBackupRestore` (durable) or sync `MarkBackupStatus restoring` + `Daemon.RestoreBackup` + `restored`. No server-level `SetServerActualState restoring_backup`. Client can still `POST /power start` concurrent with restore (only `ensureTransferIdle` guard, not restore).
- **FORGE Beacon:** `server.go:??` backup restore handlers (not server-level restoring flag) — beacon `manager.go` has no `Restoring` state (only `InstallationState`). `server.go:???` restore runs without `IsRestoring` guard; concurrent power check only looks at `InstallationState`.
- **FORGE DB:** `019_backups.sql` etc backup rows, no trigger to set server `status=restoring_backup`.
- **FORGE Frontend:** `lifecycle-machines.ts:55` `BACKUP_MACHINE` steps queued/creating/archiving/completed but no `restoring` machine integrated with console `blocked` logic.

- **STATUS:** **Missing — server `restoring_backup` state not wired**  
- **GAP:** All 4 games-hosting refs treat `restoring_backup` as server-level exclusive lock (blocks power, suspend, reinstall, transfer). Forge tracks per-backup status but never flips server `actual_state`/`status`, so API will allow `POST /power start` or `POST /servers/:id/install` during restore, diverging from `validateCurrentState`. Pufferpanel's `IsRestoring` correctly blocks `IsIdle` for `Install`/`Start`/`Destroy`; Forge lacks.
- **RECOMMENDATION:** On `DispatchBackupRestore` also `SetServerActualState restoring_backup` and clear on completion/failure; enforce `IsRestoring` guard in `clustermanager RequestServerPower` and `BeginInstall`. Report server badge as `restoring_backup` while any backup job `restoring`.
- **SEVERITY:** High

---

### C11 — Transfer / Migration (node-to-node)

- **REFERENCE: pterodactyl-panel `ServerTransfer.php:26-100`, `Admin/ServerTransferController:39-91`, `Api/Remote/ServerTransferController:34-125`** panel creates `server_transfers` row `old_node/new_node/old_allocation/new_allocation/additional`, reserves new allocations on server, generates JWT `ServerTransfer` scope token for target node, notifies source daemon `notify(newNode,token)`, daemons talk peer-to-peer (archive push). States `successful NULL|true|false`, `archived`. `validateTransferState:409-418` blocks if `!isInstalled` or `restoring_backup` or `transfer!=null` (already transferring).
- **REFERENCE: pterodactyl-wings** `server/transfer.go` (transfer manager), Wings transfer protocol via `remote`.
- **REFERENCE: pelican-panel/wings** same but improved token hash, keep `status` enum vs `successful bool`.
- **REFERENCE: pufferpanel** no transfer — servers are file-locked to single node; `server.go:91-112` no queue.

- **FORGE Doc:** `server-lifecycle.md:7` *"Server transfer/archive execution is intentionally unavailable and returns `501`". Migration engine outside lifecycle.*
- **FORGE API:** `handlers_servers.go:700-774` `POST /servers/:id/transfer` (admin, via `MigrationService.CreateMigration+ExecuteMigration` → `202`), `GET /transfer` (any `settings.rename` perm, returns `{state,transferring,targetNodeId,error}` from `servers` row), `POST /transfer/cancel` (admin, finds active migration `status not in completed/failed/cancelled` → `CancelMigration`). Legacy routes `legacyServerTransferUnavailable:130` returns `410 Gone`. `ensureTransferIdle:145` guards `power/install/reinstall` with `409` if `transfer_state queued|running`.
- **FORGE Service:** `migration/service.go:127-250` `CreateMigration` normalizes source/target, reserves allocation?, `store.CreateMigration`, publishes `EventMigrationCreated`; `PrepareMigration:227-246` ensures `migration_runs` with `TransferProtocolVersion`, `ExecuteMigration:249-254` async `startRun`; `run:417-450` claims `MigrationRun` lease (`ClaimMigrationRun` 2m), generates scoped credentials (`TransferCredentialClaims Version=TransferProtocolVersion`), registers on both daemons `RegisterTransferCredential` + `FinalizeTransferDestination` + `CleanupTransferSource`. Worker pool `maxConcurrentWorkers`, `Start:257`, `RunOnce:305`, `cleanupMigration:378-414`.
- **FORGE Store:** `store_migrations.go` etc `migrations` + `migration_runs` (`045_real_server_transfer.sql:1-28`) with lease `lease_owner/lease_expires_at`, `phase`, `cleanup_pending`, `allocation_reservations` (`reservation`). `servers` row still has legacy `transfer_state/transferring/transfer_target_node_id/transfer_run_token` but  `040_truthful_server_lifecycle.sql:7-13` clobbers stale `queued/running` → `failed` on startup; new flow uses `migrations` table not `servers.transfer_state` for active work, leaving dual states.
- **FORGE Beacon:** `server.go:368-396` protocol v1 `/api/v1/transfers/*` (`registerTransferCredential`, `prepareTransferSource`, `pushTransferSource`, `receiveTransferChunk`, `restoreTransferDestination`, `finalizeTransferDestination`) — peer-to-peer engine `transfer.Engine/Manager` (`server.go:83-84`). No `status=suspended` involvement.
- **FORGE Event:** `migration/service.go:171-177` `EventMigrationCreated` + run events; `handlers_remote.go` not relevant.
- **FORGE Frontend:** `web/lib/api/servers.ts:394-422` `fetchServerTransferStatus/cancelServerTransfer/transferServer` — targets Forge's `MigrationService` facade, not Pterodactyl `server_transfers`.
- **FORGE Tests:** `store_migration_transfer_integration_test.go`.

- **STATUS:** **Partial — engine exists but not panel primary transfer**  
- **GAP:** Panel doc says transfer unavailable (501) yet `POST /servers/:id/transfer` returns `202` via `MigrationService` — is the archival doc outdated? Traditional Wings transfer (reserve allocation → token → peer push) is replaced by migration engine (`migration_runs` + `allocation_reservations` + credential hash). `servers.transfer_state` is legacy shim; modern `migrations.status` (`planned/preparing/running/completed/failed/cancelled`) not projected to `servers.transfer_state` after `040` forced failure. So `ensureTransferIdle` reading `servers.transfer_state` will miss active `migrations` row; but `GET /transfer` also reads `servers` row not `migrations`. Pterodactyl allowed only one concurrent transfer per server (`whereNull('successful')`); Forge `migration/service.go:342` allows multiple migrations per server up to `maxConcurrentWorkers` capped globally, but `Cancel` loop finds first not完了.
- **RECOMMENDATION:** Sync legacy `servers.transfer_state` from `migrations` status (or retire it and make `ensureTransferIdle` query `migrations` table). Update `server-lifecycle.md` to reflect migration engine as canonical transfer (remove 501 note).
- **SEVERITY:** Medium

---

### C12 — Install / Power / Transfer Exclusivity (RunningAction slot)

- **REFERENCE: pterodactyl-wings `power.go:56-64` & `install.go:197-201`** Wings uses three atomic bools `installing/transferring/restoring` guarded before power; install `installing.SwapIf(true)` else error. Pufferpanel `IsIdle:884-899` checks `IsRestoring|IsBackingUp` + `IsRunning|IsInstalling`.
- **FORGE Beacon:** `manager.go:271-310` `BeginInstall` `TryLock` + `RunningAction==''`+ `Suspended==false`+ `InstallationState !=installing`; `EndInstall:298-310` releases; `HandlePower:487-507` same `TryLock`+ `RunningAction==''`+ `InstallationState==installing` guard; defers `RunningAction=""`. Single slot `RunningAction string` serializes `install/start/stop/restart/kill`.
- **FORGE API:** `handlers_servers.go:875 ensureTransferIdle` only checks transfer, not installing — so `POST /power start` during `installing` passes API and enqueues op; Beacon will reject at HandlePower but durable op will retry 3 times and eventually `failed`. Pterodactyl would 409 immediately.
- **FORGE Store:** `powerSignalPriorStates:44-54` allows stop/kill during installing (discussed).

- **STATUS:** **Implemented — stricter at Beacon, loose at API**  
- **GAP:** API should fail fast like Wings (`409`) for `installing`/`restoring_backup` instead of enqueuing doomed op. Forge operation retry will mark `failed` after 3 attempts but caller sees `202 accepted` then async failure; Wings returns `409` synchronously with `ErrServerIsInstalling`. Pufferpanel outright returns `ErrServerRunning` from `IsIdle`.
- **RECOMMENDATION:** Add `IsServerInstalling` (check `status==installing` or `actual_state==installing`) guard to `power/install/reinstall` handlers alongside `ensureTransferIdle`; or extend `ensureTransferIdle` to `ensureServerIdle` (covers installing/restoring/suspended?). Keep `kill` exception.
- **SEVERITY:** Medium

---

### C13 — Sync Configuration Before Start (`Sync()`)

- **REFERENCE: pterodactyl-wings `power.go:172-175` `onBeforeStart`→`s.Sync()` to pull latest panel config after acquiring lock; `power.go:184-205` `s.SyncWithEnvironment()` after sync, then `UpdateConfigurationFiles()` before boot. Same guard `IsSuspended` after sync.
- **REFERENCE: pufferpanel** no panel sync — daemon is source of truth (JSON files).

- **FORGE Beacon:** `manager.go:689-774` `syncServerStateFromPanel` fetched `GetServerConfiguration` (`remote.NewClient(panelURL,token)`) and merges `suspended/invocation/environment/build/allocations/process_configuration` into `State`; `onBeforeStart:610-674` snapshots `Suspended,InstallationState,Synced,Root,Chown,PanelURL, diskLimit` under lock then if `ChownOnBoot` does `chownRecursive`, then `syncServerStateFromPanel` (non-blocking warn), then disk check.
- **FORGE Panel:** `clustermanager/service.go:276-283` `syncProvisionTarget` → `runtime.SyncServerConfiguration` → marks `MarkServerConfigSynced` or `MarkSyncFailed`; `SyncServerConfiguration:265-274` re-syncs after every DB mutation that flips `config_sync_pending` (`store_servers.go:474-486` `runtimeChanged` sets `config_sync_pending=TRUE`). `handlers_servers.go:399-410` after `UpdateServer` if `ConfigSyncPending` triggers `SyncServerConfiguration`.
- **FORGE Store:** `servers` `config_sync_pending BOOLEAN` + `config_sync_error TEXT` (`040_truthful…`) + `last_config_sync_at` (not shown) → poll; Beacon persists `.config/server.json` (`server.go:842-850` `syncConfiguration` writes indent JSON + applies `runtimeRequestFromConfiguration`).
- **FORGE Event:** `publisher Publish EventActualStateChanged` on sync fail.

- **STATUS:** **Implemented — dual-layer (panel→daemon push via `syncProvisionTarget` + daemon→panel pull via `syncServerStateFromPanel`)**  
- **GAP:** Pterodactyl sync is **Wings→Panel** (pull latest before boot). Forge also does **Panel→Beacon** push on every mutation (more correct). Beacon `syncServerStateFromPanel` uses `panelSyncMu.TryLock` non-blocking: if another power op triggers sync concurrently, second silently skips panel sync — but `power.go:172` always syncs blocking; Forge may start with stale suspend flag if sync was skipped.
- **RECOMMENDATION:** Make `syncServerStateFromPanel` blocking with timeout rather than `TryLock` skip; or ensure `syncProvisionTarget` push already guaranteed suspension is current (it is — panel push is synchronous, so pull is only fallback for drift). Document which is source of truth (panel DB).
- **SEVERITY:** Low

---

### C14 — Disk Quota Check Before Start

- **REFERENCE: pterodactyl-wings `power.go:189-196`** if `DiskSpace() <=0` (unlimited) still `HasSpaceAvailable(true)` async; else `HasSpaceErr(false)` blocking with `"Checking server disk space …"` console message.
- **REFERENCE: pufferpanel** no disk quota check before start (checked via `FileServer` usage elsewhere?).

- **FORGE Beacon:** `manager.go:659-673` `onBeforeStart` after sync computes `diskUsageBytes(root)` via `WalkDir` skipping symlinks, returns error if `usage>limit`; `HasSpaceForWrite:776-812` used for SFTP/ file writes, not just start. `store_servers.go:208-210` validates `DiskMB>0` on create. `runtime docker` `buildResources` does not enforce disk — only memory/CPU.
- **FORGE Panel:** `clustermanager` `CreateServer` validates `MemoryMB/DiskMB>0` (`store_servers.go:209`), `UpdateServer` validates limits.
- **FORGE Frontend:** none.

- **STATUS:** **Implemented (Wings parity, stricter at Beacon)**  
- **GAP:** Wings check runs `Filesystem().HasSpaceAvailable` which uses `Size()` DB-cached size not live walk (fast); Forge always walks Dir (`diskUsageBytes:814-834`) — correct but O(n) on every start, may latency-spike on large servers.
- **SEVERITY:** Low

---

### C15 — Crash Detection & Auto-Restart (circuit breaker)

- **REFERENCE: pterodactyl-wings `server/crash.go:45-84`** `processCrashDetection`: if `crashDetectionEnabled==false` abort, if last crash < `timeout` (default 60s?) abort, else publish restart; increment counter; `server.go:351` log.
- **REFERENCE: pufferpanel `servers/server.go:562-602` `afterExit`** if `exitCode==ExpectedExitCode` reset `CrashCounter=0`; else if `!graceful && AutoRestartFromCrash && CrashCounter < CrashLimit` increment and `StartViaService`; if `skipAutoRestart` flag.

- **FORGE Beacon:** `manager.go:91-102` constants `crashCooldownDefault=1m`, `maxConsecutiveCrashRestarts=5`, `crashLoopWindow=30m`, `crashAutoRestartWindow=24h`; `State` holds `CrashDetectionEnabled,CrashCooldown,LastCrash,CrashCount,DetectCleanExitAsCrash,LastStartedAt,ExpectedStop`; `HandleContainerEvent:879-955` on `start` → `PowerState=Running,LastStartedAt=now`; on `die/oom/stop` if `ExpectedStop` → `offline,CrashCount=0` else computes `crashed= OOMKilled||ExitCode!=0 || DetectCleanExitAsCrash`, respects cooldown, window reset, `loopBroken` check (`>5`), calls `crashHandler` then `HandlePower start` unless broken. `persistPowerState:387-428` survives daemon restart; `autoRestartCrashed:177-206` restarts workload if stopped near restart window.
- **FORGE Panel:** `services/crashdetector/service.go:16-110` separate detector with `MaxRestarts` suspends server via `onSuspend` callback (`main.go:955` `CrashDetector.OnSuspend` → publish `EventServerSuspended`); `services/reconciler` also monitors `actual_state=crashed`.
- **FORGE Tests:** `crashloop_breaker_test.go`, `manager_test.go:HandleContainerEvent`.

- **STATUS:** **Enhanced — two-layer (Beacon in-process + panel detector suspending)**  
- **GAP:** Wings/Pelican auto-restart up to indefinitely until cooldown window? Forge caps at 5 consecutive within 30m (stricter). Pterodactyl has no panel auto-suspend on crash loops; Forge's panel detector will suspend after `MaxRestarts` (configurable). Divergence is positive but may surprise migration from Pelican (`DetectCleanExitAsCrash` default false matches Wings recommended).
- **RECOMMENDATION:** Expose `maxConsecutiveCrashRestarts`/`crashLoopWindow` via node's `config.beacon.yaml` and panel `crashdetector` settings so admins can align with Wings `CrashDetection` config.
- **SEVERITY:** Low

---

### C16 — Startup Variable Validation & Image/Startup Command

- **REFERENCE: pterodactyl-panel `Server.php:288-300` `variables()` left-join `server_variables` (`pluck`); `EggVariable` has `rules` string (e.g., `required|string|max:191`). Panel `ServerVariableController` validates via `Validator`.**
- **REFERENCE: pelican** same but Filament.
- **REFERENCE: pufferpanel `spec.json` variables `data: { [var]: {display,type,required,options}}`** no regex.

- **FORGE Store:** `store_servers.go:273-295` validates each `StartupVariables` against `egg_variables.rules` via `validateVariableValue(value,rules)` (see `store_eggs_integration_test.go`), inserts `server_variables` with `ON CONFLICT UPDATE`. `store_server_flags.go` etc. `GetServer` returns resolved startupCommand via `COALESCE(NULLIF(s.startup_command,''),e.startup)` and docker_image via `jsonb_each_text`. `ServerProvisionTarget:153-182` resolves env vars `SELECT ev.env_variable, COALESCE(sv.variable_value,ev.default_value)` ordered, builds `target.Environment` + `resolveStartupCommand` interpolation. `startup.go` redaction guards `hasServerReadEnv`.
- **FORGE API:** `handlers_servers.go:1518-1649` `GET /startup`, `PUT /startup/variable`, `PATCH /startup/command|image` all `SyncServerConfiguration` after persist; `POST /servers` validates `templateId`/egg exists, default memory from `egg.default_memory_mb` (`803`).
- **FORGE DB:** `013_startup_variables.sql`, `091_seed_minecraft_java.sql` (seed EULA etc).
- **FORGE Beacon:** `server.go:901-997` `runtimeRequestFromConfiguration` parses `invocation/startupCommand` from `payload["invocation"]` → `Command: ["/bin/sh","-lc", invocation]`, env from `environment` map, etc.

- **STATUS:** **Implemented — stricter validation than puffer, parity with Pterodactyl**  
- **GAP:** Pterodactyl Eggs can define `docker_images` map; Forge's runtime create `command` null for `itzg/minecraft-server` image (hardcoded `EULA=TRUE` injection `clustermanager/service.go:652-654` `runtimeConfiguration`/`runtimeCreateRequest` special-case) — matches doc limitation `server-lifecycle.md:15` "itzg compat remains image-specific rather than egg-defined". Pufferpanel's `install` operations allow conditional `If` on environment — Forge installer ops have `Condition FileExists/FileMissing` only (`installer/operations/operation.go:15-38`), less expressive.
- **SEVERITY:** Low

---

### C17 — Allocations & Primary Handling

- **REFERENCE: pterodactyl-panel `Server.php:206-211` `getAllocationMappings()` group by `ip→port[]`; panel `StoreServerService.php` assigns primary + additional allocations, validates `allocation.node_id == server.node_id`, unique constraint `allocations(ip,port,node_id)`.**
- **FORGE Store:** `store_servers.go:225-249` loops `allocationIDs` (`AdditionalAllocationIDs` prepended), `SELECT node_id,server_id FOR UPDATE` → error if already assigned or wrong node; checks `AllocationLimit` vs `len(seen)`; `INSERT servers primary_allocation_id`, then `UPDATE allocations SET server_id`. `store_allocations.go` `FindAvailableAllocation`. `ServerProvisionTarget:182-200` collects allocations ordered `ORDER BY (id=primary) DESC, port`, builds `ServerRuntimeAllocation` with `containerPort`/`protocol`.
- **FORGE API:** `handlers_servers.go:454-698` CRUD allocations per server (`assign`, `unassign`, `setPrimary`, `update alias/notes`).
- **FORGE Beacon:** `docker.go:743-781` builds `nat.PortSet/PortMap` via `dockerPorts` (`865-897`) validating duplicate hostKey, protocol tcp/udp, port range. `Create` mounts `serverContainerRoot /home/container`.
- **FORGE DB:** `allocations ip INET, port int, container_port int, protocol text` (`090_allocation_transport.sql`, `144_allocation_container_port_compatibility.sql`).

- **STATUS:** **Implemented — parity**  
- **GAP:** Pterodactyl tracks `allocation_limit NULL = unlimited`; Forge treats `0 = unlimited` (`store_servers.go:247` `if AllocationLimit>0 && len>limit`). Slight semantic shift but documented. Pufferpanel has no allocation concept (host port from `data["port"]`).
- **SEVERITY:** Low

---

### C18 — Generation Fencing & Workload Lease (evacuation/recovery)

- **REFERENCE:** none of the 6 refs have generation fencing — they rely on single-panel lifecycles.
- **FORGE Store:** `store.go:495-499` `Server.Generation int64` monotonically incremented on recovery/evacuation; `WorkloadLeaseExpiry *time.Time` fence old workload. `UpdateServerGeneration:502-514` sets `generation,workload_lease_expiry`. `SetServerDesiredState:16-17` `desired_generation + CASE IS DISTINCT`.
- **FORGE DB:** `022_evacuation_planner.sql`, `027_recovery_coordinator.sql`, `110_node_fencing.sql` etc.
- **FORGE Beacon:** `manager.go` not checking generation (panel-side only); `runtime/docker.go:250` `workloadLocks` per-server sharding prevents parallel Create/Start races.

- **STATUS:** **Forge-only innovation (no reference)**  
- **GAP:** Panel/worker must enforce generation check before accepting daemon callbacks (Beacon's `notifyPanelInstallStatus` does not send generation, could accept stale install for fenced server). See LF-04.
- **SEVERITY:** Medium

---

## 3. Summary Matrix

| # | Capability | Ref Ptero/Pelican/Puffer | Forge Status | Gap Type | Severity |
|---|------------|--------------------------|--------------|----------|----------|
| 1 | Create→installed split | `installing` overload | **Enhanced** provisioning→created | Positive split | Low |
| 2 | Desired/Actual state machine | single `status` | **Enhanced** desired/actual+gens | Superset | Medium |
| 3 | Suspend toggle | enum `status=suspended` | Boolean orthogonal | Diverge | Medium |
| 4 | Power semantics | lock + sync+disk | Durable queue + store allowlist | Diverge | High |
| 5 | Kill bypass | pierces lock | queued | **Missing bypass** | Medium |
| 6 | Install | container `*_installer` | 1000:1000 container+WS admin scope | Enhanced | Medium |
| 7 | Reinstall | same flow, wait stop | **Blocked**: Reinstaller missing | Gap | High |
| 8 | Delete+orphan | no orphan table | `server_orphan_remediations` | Enhanced | Low |
| 9 | Restoring backup lock | `restoring_backup` | **Missing server-level** | Gap | High |
|10 | Transfer/migration | `server_transfers` | migration_runs (doc says 501) | Partial | Medium |
|11 | Exclusivity slot | atomics | `RunningAction`+TryLock | Implemented | Medium |
|12 | Sync before start | Wings pulls | Panel push + Beacon pull | Enhanced | Low |
|13 | Disk check | cached size | live `WalkDir` | Implemented | Low |
|14 | Crash auto-restart | cooldown 60s | cooldown 1m + breaker 5/30m + 24h window | Enhanced | Low |
|15 | Startup variables | `rules` regex | `validateVariableValue`+redaction | Parity | Low |
|16 | Allocations | group by ip | `containerPort` aware | Parity | Low |
|17 | Generation fencing | none | `generation`/`lease` | Innovation | Medium |
|18 | Schedules power tasks | `schedule Tasks power` | `cronjob`/`scheduler` + `control.*` | Parity | Low |

---

## 4. Logic Findings (×6 — requirement ≥3)

### LF-01 — `stop`/`kill` permitted during `installing` breaks Wings exclusivity & can orphan installer

**Files:** `forge/api/internal/store/store_servers_control.go:51` `stop,kill: []string{"running","installing","provisioning","restoring_backup","recovering","created","stopped"}`; `reference/game-hosting/pterodactyl-wings/server/power.go:57-64` `if IsInstalling||IsTransferring||IsRestoring → ErrServerIsInstalling`; `beacon/internal/server/manager.go:494-496` `if InstallationState==installing → "server is installing"`.

**Logic:** Forge store allows `SetServerPowerState("stop")` while `status==installing`. API will 202-enqueue a `server.stop` op; Beacon `Manager.HandlePower` will reject with `"server is installing"` (409), but the durable op worker (`operation/service.go:269-290`) will retry 3× with backoff (`BaseBackoff 1s` double) then `StatusFailed`. Meanwhile the installer container (`docker.go:312 ContainerStart wait`) continues. If the operator retried `stop` as `kill`, Forge queues a separate `server.kill` op which also fails (blocked). Wings explicitly blocks power during install at Wings level *before* acquiring `powerLock`, returning synchronous 409 — Forge leaks a ghost failed op and never cleans the installer.

**Impact:** Installs that should be abortable via `kill` (Wings kill bypass) instead queue-then-fail, leaving `manager.RunningAction==install` held until `EndInstall` (duration of script). Disk writes keep going.

**Recommendation:** Make API guard `ensureServerNotInstalling` (check `actual_state == installing` or `status installing`) for `start/stop/restart` (keep `kill` as bypass path that actually calls `runtime.Install` cleanup + `ContainerRemove` of `*-installer`). Or change `powerSignalPriorStates` to remove `installing` for `stop` and keep only `kill` via bypass. Add `CANCEL` semantics for install ops (`OperationService.Cancel` already exists but not wired to install → `runtime` cancel).

---

### LF-02 — Suspend flag races `onBeforeStart` panel pull via non-blocking `TryLock`

**Files:** `beacon/internal/server/manager.go:693-696` `if !m.panelSyncMu.TryLock() → "panel state sync is already in progress"`; `manager.go:610-634` `onBeforeStart` snapshot `Suspended` *before* panel sync, then updates `Suspended` *after* sync (`748-749`).

**Logic:** `onBeforeStart` snapshots `state.Suspended` into local `suspended` (line 614) then checks `if suspended→return "suspended"` (line 628) *before* attempting panel sync (651). If DB was flipped to `suspended=true` 10 ms earlier, but `panelSyncMu` is held by concurrent `Reconcile` or another `onBeforeStart`, the pull is skipped (`TryLock` fails) and the stale snapshot `false` proceeds to `Chown`/`disk check` and ultimately `runtime.Start`, launching a suspended server. Panel push path (`syncProvisionTarget` before DB suspend?) is synchronous panel→Beacon, but suspend toggle (`SetServerSuspended` in `handlers_servers.go:1431`) does **not** push config sync — it just writes DB and best-effort stop. So window exists.

**Recommendation:** Snapshot-after-sync or reload `Suspended` under lock after sync; change `TryLock` to blocking with 2s timeout (`TryLock` → `Lock` with `context.WithTimeout`). Also push `SyncServerConfiguration` from `SetServerSuspended` handler (like `UpdateServer` does `399-410`).

---

### LF-03 — `HardDeleteServer` allocation release ordering leaks ghost primary on FK-failure

**Files:** `forge/api/internal/store/store_servers_lifecycle.go:26-57` `hardDeleteServer` locks `servers FOR UPDATE` (32), conditionally inserts orphan (36-41), then `UPDATE servers SET primary_allocation_id=NULL WHERE id=$1` (43), then `UPDATE allocations SET server_id=NULL WHERE server_id=$1` (46), then `DELETE FROM servers` (49).

**Logic:** If transaction is killed after line 43 but before 46 (e.g., deadlock on `allocations` `FOR UPDATE` rows from `CreateServer` parallel), `primary_allocation_id` nullified but allocations still point to deleted server — orphaned foreign key after `DELETE` cascade? Actually `allocations.server_id` FK `ON DELETE SET NULL`? Check `store_servers.go:225` but `allocations` table `server_id UUID REFERENCES servers(id)` — if `ON DELETE SET NULL` semantics depend on constraint; `001_init.sql` has no FK? `AddForeign…Servers` migration adds FK with cascade? But race leaves primary dangling. More subtle: `CreateServer:238` `SELECT allocations … FOR UPDATE` on `allocationID` rows; `HardDeleteServer` locks server first then updates allocations without `FOR UPDATE` on allocation rows — deadlock risk but order is consistent (server lock first, then allocations). Still, primary-allocation null before child allocations clear is incorrect if crash leaves half-state (primary null but children still assigned). Should clear children first.

**Recommendation:** Swap order: `UPDATE allocations … server_id IS NOT NULL` before `primary_allocation_id=NULL`, or single CTE.

---

### LF-04 — Create path workload hash race & generation fencing gap

**Files:** `beacon/internal/runtime/docker.go:195-203` `createRequestHash` → SHA256 of marshalled `CreateRequest` (excluding RegistryAuth password) → label `modern-game-panel.config_hash`; `docker.go:199-214` if existing container's label equals hash → no-op else stop→remove→create. `forge/api/internal/services/clustermanager/service.go:172-191` `provisionServer` → `syncProvisionTarget`→`runtime.CreateServer`→`SetServerProvisioned`. `store_servers.go:253-305` CreateServer inserts `provisioning` + `installed=false`.

**Logic:** Two concurrent `SyncServerConfiguration` calls for same server compute same hash; both inspect container label `config_hash` before either recreates — both see mismatch, both stop container, then one succeeds `ContainerCreate` with name `mgp-{id}` and second gets `Conflict` (name already exists) → second `ContainerRemove` deletes just-created container. No retry. Also `createRequestHash` excludes `RegistryAuth.Password` copy but includes `Username/ServerAddress` — hash collision not sensitive. Generation fencing (`store.go:495` `generation` incremented on recovery only, not on each `SyncServerConfiguration`) so stale daemon callback (install `notifyPanelInstallStatus` via `server.go:1339-1343` POST to panel without generation) could mark `installed=true` for an older generation that was already fenced (evacuated). The daemon never sends `generation`.

**Recommendation:** Add per-server `workloadLock` around `reconcile` (already via `workloadLocks` shard but sharded 64-way may collide) — use exclusive per-server lock not sharded or add `FOR UPDATE` on `servers` to serialize config sync at DB. Include `generation` in `runtimeCreateRequest` and require beacon to echo it back in `notifyPanelInstallStatus`/`Stats`.

---

### LF-05 — Reinstall interface gate silently fails Docker reinstall (600ms install success → capture)

**Files:** `forge/api/internal/services/clustermanager/service.go:240-248` `if reinstall { reinstaller, ok := s.runtime.(Reinstaller); if !ok → error "runtime does not support reinstall" }`; `beacon/internal/server/server.go:1323-1335` `reinstall` checks `PowerState` then forwards to `install`; `forge/api/internal/runtime/runtime.go` interface `Reinstaller`.

**Logic:** Docker is the only production runtime (`ProviderDocker`); it does NOT implement `Reinstaller`, so `POST /servers/:id/reinstall` via Forge (`clusterManager.ReinstallServer`) always flips `SetServerInstallState installing` then fails with `failed` state, leaving server `install_failed` even though Beacon's own `/reinstall` (forward to install) would have succeeded. The DB state is clobbered before runtime call, so retry requires manual `toggle-install`. Also `runInstaller:232-234` does `SetServerInstallState installing` **before** `syncProvisionTarget` — if sync fails, state left `failed` with `install_error=sync error` not recoverable without admin.

**Recommendation:** Drop interface gate, fallback to `InstallServer` path when `Reinstaller` missing (Beacon does). Make state transition `installing`→`failed` only after runtime reports failure, and include compensating `installed`→`failed`? Actually set to `failed` is correct but should not happen for unsupported — should never set `installing` in that branch.

---

### LF-06 — Beacon `power` HTTP still 30s blocking while Forge is 202-queue (semantics split)

**Files:** `beacon/internal/server/server.go:1380-1406` power handler `waitCtx 30s` polling `GetStatus` every 500ms; `forge/api/internal/http/handlers_servers.go:875-940` power returns `202 mode:durable|queued` immediately.

**Logic:** Operator calling Beacon directly (internal debugging or panel fallback) gets blocking semantics with timeout → 504 if runtime hangs; panel caller gets queued 202 and must poll `GET /operations/:id`. Two clients see different contracts for same underlying `manager.HandlePower`. Also `operation/service.go:155-183` reaper reaps `running>5m` to `failed`, but Beacon wait loop returns 504 not failed. No correlation id propagates from Forge op → Beacon op (`X-Forge-Command-ID` vs `Idempotency-Key`).

**Recommendation:** Make Beacon power non-blocking too (`202` with `operationId`) and document; keep 30s wait only for legacy fallback (`QueueService` path). Unify idempotency header name (`X-Forge-Command-ID` vs `Idempotency-Key`).

---

## 5. Gaps vs Reference Clustering — Actionable Recommendations

| ID | Gap / Divergence | Affected Refs | Recommended Fix | Priority |
|----|------------------|---------------|-----------------|----------|
| G-01 | `restoring_backup` not wired server-level | Ptero/Pelican/Wings/Puffer | Add `actual_state restoring_backup` on `DispatchBackupRestore` + block power/install | **High** |
| G-02 | Kill lock bypass missing | Ptero Wings `PowerActionTerminate` | Give kill its own semaphore / skip `RunningAction` claim | Medium |
| G-03 | Suspend does not block client API blanket | Pelican `AuthenticateServerAccess` | Add middleware/guard for `suspended` on client routes except resources | Medium |
| G-04 | Reinstall requires stopped (strict reject) vs Wings WaitForStop(10s) + Pelican same | Wings `WaitForStop` | Keep strict but add helpful `409` message linking to `POST /power stop` flow; or add 10s wait fallback | Low |
| G-05 | `skip_scripts` not honored | Wings `SkipEggScripts` | Check `SkipScripts` in `runInstaller` before runtime call | Low |
| G-06 | Transfer legacy `servers.transfer_state` vs `migrations` dual-state | Ptero `server_transfers` | Project `migrations.status` onto `servers.transfer_state` or retire column | Medium |
| G-07 | Allocation containerPort hard default to same as host (Forge fixes) — actually improvement | Ptero/Pelican mappings | Keep `containerPort` handling (`store_servers_control.go:655`) — already better than refs where only hostPort used | Info |
| G-08 | Installer `install` not idempotent under retries (hash race) | Wings pull image dedupe | Use per-server lock around `reconcile` (not sharded) | Medium |
| G-09 | `installWS` requires `ScopeAdmin` but `install` HTTP does not | Beacon `installWS:1170` | Add same scope check to `install` non-WS handler | Medium |
| G-10 | `DELETE ?force` orphan still needs manual resolve | Ptero silent | Add `GET /admin/orphan-remediations?status=pending` UI (already `handlers_orphan_remediations.go`) — wire to web `lib/api/servers.ts:149` | Low |

---

## 6. Severity Key

- **Critical:** data loss or security bypass. None in this dimension — install scope guard present (`installWS` admin-scope) but non-WS missing is Medium, not Critical due to API admin `servers.write` gate.
- **High:** power lifecycle divergent, reinstall broken, restoring not locked.
- **Medium:** suspend/transfer semantics, kill bypass, generation fence.
- **Low/Info:** positive divergences (orphan table, desired/actual, crash breaker).

---

## 7. Conclusion

Forge **truthfully** re-implements Pterodactyl/Pelican/Wings lifecycle and **surpasses** it on durability (durable queues, generation fencing, orphan remediation tables, dual desired/actual states, 5s reaper, 30s wait). The three headline risks are:

1. **Restoring backup lock missing** — unbounded concurrent power during restore.
2. **Reinstall path dead on Docker** — `Reinstaller` interface gate.
3. **Power install overlap allowlist** — `stop` permitted during `installing` (store) but Beacon rejects, producing ghost failed ops; and `kill` cannot pierce install lock.

Each is fixable in <30 LOC in `store_servers_control.go:44-51`, `clustermanager/service.go:240-248`, and `handlers_servers.go` ensure-idle guard, plus wiring `actual_state=restoring_backup` in backup service.

References above carry `file:line` for mechanical follow-up.

---

*End of subagent-01 report. Generated for Phase 02 synthesis.*
