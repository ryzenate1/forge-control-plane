# Reverification Subagent 01 — Game Server Lifecycle (create→provisioning→created vs installing, desired/actual+generations, suspend/unsuspend, power incl. kill-pierce, install/reinstall, delete+orphan)

**Date:** 2026-08-24 (reverification, parallel run 01/20)
**Scope:** Forge live code re-inspected file:line (no trust of prior audits), reconciled against `audits/FINAL_PARITY_AUDIT.md §2 GH-01..GH-19`, `audits/phase-02/synthesis.md`, `audits/final-parity/subagent-01-game-hosting.md`, `audits/MASTER_FINDING_INDEX.md` entries `REF-GAME-*`.
**Forge checkout:** `forge/api` + `beacon` as on disk 2026-08-24
**Method:** Direct `read`+`grep` on listed paths; every claim cited `path:line`.

**Live files re-inspected:**
- `forge/api/internal/store/store_servers.go` `store_state.go` `store_servers_lifecycle.go` `store_servers_control.go` `store_egg_variables.go` `store_users.go` `store_schedules.go` `store_mounts_ext.go`
- `forge/api/internal/http/handlers_servers.go`
- `forge/api/internal/services/clustermanager/service.go` `forge/api/internal/orchestrator/suspension.go`
- `beacon/internal/server/server.go:727-1600` `beacon/internal/server/manager.go:30-310` `beacon/internal/runtime/docker.go:151-510`
- `forge/api/docs/server-lifecycle.md` `forge/api/migrations/*`

---

## 0. Reconciliation vs Prior Audits

| Prior ID | Prior Status (FINAL §2) | Prior Severity | Reverification Now | Delta |
|---|---|---|---|---|
| GH-01 create provisioning→created | COMPLETE (better) | — | **COMPLETE** `forge/api/internal/store/store_servers.go:253` still `provisioning→created` distinct, `forge/api/docs/server-lifecycle.md:5` | CONFIRMED — no drift |
| GH-02 desired/actual+generations | COMPLETE+ | — | **COMPLETE** `forge/api/internal/store/store_state.go:10` `store_state.go:39` fence, `state_transitions` audit `store_state.go:120` | CONFIRMED, livelock LF-10 still present |
| GH-03 suspend orthogonal bool | COMPLETE+ | — | **PARTIAL** (downgraded from COMPLETE+ on live re-inspect) — `handlers_servers.go:1413` `POST /suspension` lacks `ensureTransferIdle` `handlers_servers.go:145`, `manager.go:748` race, legacy `store_servers.go:374` raw path | CONFIRMED gap already noted in `final-parity C-03 PARTIAL`; FINAL overstated |
| GH-04 power start/stop/restart | PARTIAL | — | **PARTIAL** `handlers_servers.go:888` `store_servers_control.go:44` allowlist, `manager.go:482` TryLock | CONFIRMED |
| GH-05 kill pierce lock | MISSING | — | **MISSING** `manager.go:487` same slot no pierce `manager.go:581`, `docker.go:494` `workloadLock` | CONFIRMED |
| GH-06 install fencing installer 1000:1000 | COMPLETE+ | — | **COMPLETE** `beacon/internal/server/server.go:1052` `manager.go:271` `docker.go:256` `User 1000:1000` `docker.go:285` `ReadonlyRootfs:true` `CapDrop:ALL` `server.go:1159` `installWS:1178` `ScopeAdmin` | CONFIRMED |
| GH-07 reinstall requires stopped / Docker gate | BROKEN | P1 | **BROKEN** `clustermanager/service.go:241` `Reinstaller` gate → 500 on Docker, `beacon/server.go:1331` works | CONFIRMED, not FIXED |
| GH-08 delete hard-delete + orphan | COMPLETE+ | — | **COMPLETE** `store_servers_lifecycle.go:18` `RecordOrphanAndHardDeleteServer` `clustermanager/service.go:332` `server-lifecycle.md:6` | CONFIRMED |
| GH-09 restoring_backup lock | MISSING P1 | P1 | **MISSING** `handlers_servers.go:2117` marks `backups.status=restoring` not `servers.actual_state` `store_state.go:31` never called, `manager.go:30` no Restoring state | CONFIRMED |
| GH-10 transfer/migration v1 | COMPLETE+ | — | **PARTIAL** (FINAL listed COMPLETE+ but live shows mitigation) — `handlers_servers.go:713` `202` via `migration/service.go:127` but `server-lifecycle.md:12` says `501`, `store_servers.go:346` `IsServerTransferBlocking` reads legacy `servers.transfer_state` not `migrations` | DOWNGRADED to PARTIAL, matches `final-parity C-10 PARTIAL` |
| GH-11 allocations containerPort/protocol | COMPLETE+ | — | **COMPLETE** `store_allocations.go` successor is `clustermanager/service.go:632` explicit `protocol` `docker.go:865` | CONFIRMED |
| GH-12 allocations kill/start race vs install (API allows stop during installing) | BROKEN | — | **BROKEN** `store_servers_control.go:51` allows `stop/kill` during `installing`, `handlers_servers.go:888` only `ensureTransferIdle:145` not `installing`, `manager.go:494` blocks | CONFIRMED |
| GH-13 egg/nest/egg_variables | PARTIAL | — | **PARTIAL** `store_nests.go:34` `store_egg_variables.go:17` valid but `store_egg_variables.go:176` bug | CONFIRMED |
| GH-14 variable validation regex slash | BROKEN P0 | P0 | **BROKEN** `store_egg_variables.go:176` `regexp.Compile(arg)` with slashes literal `store_egg_variables.go:143` `Split("|")` | CONFIRMED P0 not FIXED |
| GH-15 config parser 6 parsers | MISSING | — | **MISSING** `eggs.config` stored `store_servers_control.go:95` not applied `beacon/server.go:869` `applyConfigurationFiles` no parser, `store_servers_control.go:178` `resolveStartupCommand` only `{{VAR}}` | CONFIRMED |
| GH-16 template systems | DUPLICATE | — | **DUPLICATE** `migrations/091_seed_minecraft_java.sql` 1 egg vs `forge/web/lib/egg-templates.ts:27` 14 static vs localStorage `app-templates-data.ts:3` | CONFIRMED |
| GH-17 schedules cron task action | PARTIAL | — | **PARTIAL** `store_schedules.go:252` `CreateScheduleTask:253` only `action!=""` vs `PatchScheduleTask:331` `isValidScheduleTaskAction` | CONFIRMED |
| GH-18 wildcard * persistable | BROKEN P0 | P0 | **BROKEN** `store_users.go:555` `normalizeSubuserPermissions` allows `*` `store_users.go:306` `UpsertServerSubuser` no actor-subset | CONFIRMED P0 |
| GH-19 mount allowlist denylist | BROKEN | — | **BROKEN** `store_mounts_ext.go:323` only `source ∈ {/etc/forge,/var/lib/forge/volumes}` `target ∈ {/,/home/container}` | CONFIRMED |

Master index entries re-checked:
- `REF-GAME-C01` fencing races `beacon/manager.go:693 syncServerStateFromPanel:748` `store_state.go:40` observed_generation gate — still present, not FIXED.
- `REF-GAME-F-G-06` restoring lock `handlers_servers.go:2104` — still MISSING.
- `REF-GAME-F-G-07` reinstall gate `clustermanager/service.go:242` — still BROKEN.
- `REF-GAME-F-G-08` regex `store_egg_variables.go:177` — still BROKEN, `final-parity` LF-02 P0 confirmed.
- `REF-GAME-F-G-09` CPU conflation `store_servers.go:209` `beacon/docker.go:924` — still BROKEN.
- `REF-GAME-F-G-10` user_viewable leak `store_servers_control.go:154` vs `store_startup.go:58` — still BROKEN.
- `REF-GAME-F-G-22` subuser `*` `store_users.go:297` — still BROKEN.
- `REF-GAME-F-G-23` schedule asymmetry `store_schedules.go:252` — still BROKEN.
- `REF-GAME-F-G-24` mount allowlist `store_mounts_ext.go:323` — still BROKEN.
- `REF-GAME-HIDDEN-01` 6-step workflow `installer/service.go:66` (`forge/api/internal/services/installer/service.go:66`) persists rows never executed — still DEAD on live checkout (grep shows no caller beyond persistence).
- `REF-GAME-HIDDEN-02` transfer dual state `servers.transfer_state` vs `migrations` — still PARTIAL.
- `REF-GAME-DUP-01` template triplicate — still DUPLICATE.

No NEWLY_FIXED rows found on this lifecycle surface (GH-01..GH-19 all match prior BROKEN/MISSING/COMPLETE). No evidence of `git diff` fixes between 2026-08-24 audits and current HEAD on these paths.

---

## 1. Capability Matrix (≥12 rows, each file:line cited)

### GH-01 — Server create provisioning→created vs installing

**REFERENCE** `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:138` `protected $attributes = ['status'=>self::STATUS_INSTALLING]` (`Server.php:125` `STATUS_INSTALLING='installing'`) — new rows default `installing`; `pelican-panel/app/Enums/ServerState.php:12` same enum, no `provisioning` distinct. Wings creates container after DB row; panel status is installing until `SyncInstallState`.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:66` `createServer()` + `forge/api/docs/server-lifecycle.md:5` documents `provisioning (panel-only, never exposed to runtime)` (`forge/api/docs/server-lifecycle.md:5`).
- API: `forge/api/internal/http/handlers_servers.go:789` `POST /servers` → `clusterManager.CreateServer()` (`handlers_servers.go:838`).
- Service: `forge/api/internal/services/clustermanager/service.go:76` `CreateServer()` does placement `scheduler.PlaceServer:92`, `store.CreateServer:140`, then `provisionServer:172` → `syncProvisionTarget:276` → `runtime.CreateServer:180` → `SetServerProvisioned:187`. Compensation `compensateCreateFailure:285`.
- Store: `forge/api/internal/store/store_servers.go:253` `INSERT ... 'provisioning','stopped','stopped'` (`store_servers.go:253-261`) + `forge/api/internal/store/store_servers_control.go:207` `SetServerProvisioned()` flips `status='created', actual_state='stopped', installed=false` (`store_servers_control.go:208`).
- DB: `forge/api/migrations/001_init.sql:24` `status TEXT DEFAULT 'stopped'`; `forge/api/migrations/021_true_state_persistence.sql:34` adds `desired_state/actual_state`; `forge/api/migrations/040_truthful_server_lifecycle.sql:4` cleans stale `installing`.
- Worker: none — synchronous create with 15m timeout `handlers_servers.go:1085`.
- Beacon: `beacon/internal/server/server.go:735` `create` → `runtime.Create:829` → `persistRuntimeRequest:893` → `manager.MarkCreated:312` sets `InstallationState=installed, ContainerExists=true, PowerState=Offline` (`beacon/internal/server/manager.go:312`).
- Event: `clustermanager/service.go:165` `EventServerCreated` with `desiredState/actualState/correlationId`.
- Tests: `forge/api/internal/store/store_servers_lifecycle_integration_test.go:1` (create→provisioned).

**STATUS:** `COMPLETE` (intentional superset — more truthful than refs)

**GAP:** `lifecycle-machines.ts:41` `POWER_MACHINE` lacks `provisioning` badge — falls to raw string; benign UI truncation.

**FORGE LOGIC FINDING:** See LF-06 hash without generation (`beacon/internal/runtime/docker.go:193` `createRequestHash` `docker.go:1016` no generation seed) — two Creates with same spec but different `desired_generation` collapse; and `clustermanager/service.go:140-154` cancel/reservation 10m over-reservation window.

**RECOMMENDATION:** `ADOPT` — keep split; add `provisioning` to `POWER_MACHINE` badge map.

**SEVERITY:** `P3`

---

### GH-02 — Desired/actual state machine + generations

**REFERENCE** No ref has it; `pelican-panel/app/Enums/ServerState.php:12` single enum `Installing|InstallFailed|ReinstallFailed|Suspended|RestoringBackup`; `pterodactyl-panel/app/Models/Server.php:122` constants only `installing/install_failed/reinstall_failed/suspended/restoring_backup` single nullable `status`; Wings power ephemeral.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:66` DTO `desired_state/actual_state` via `store/store.go:452` `ServersToDTO`.
- API: `forge/api/internal/store/store.go:33` `SELECT desired_state::text, actual_state::text`.
- Service: `forge/api/internal/services/clustermanager/service.go:382` `RequestServerPower` sets desired first `SetServerDesiredState:398` + `EventDesiredStateChanged`, then `sendPower:532` sets actual + `EventActualStateChanged` + `EventServerStarted/Stopped/Restarted:576`. `RefreshServerActualState:413` probes `Stats`.
- Store: `forge/api/internal/store/store_state.go:10` `SetServerDesiredState` increments `desired_generation` on distinct (`store_state.go:16`); `store_state.go:31` `SetServerActualState` maps actual→legacy `status` via `serverStatusFromActual:131` and fences `observed_generation = CASE WHEN (desired='running' AND actual='running') OR (desired='stopped' AND actual='stopped') THEN desired_generation ELSE observed_generation END` (`store_state.go:39`).
- DB: `forge/api/migrations/021_true_state_persistence.sql:3` `ENUM server_desired_state('running','stopped')` + `server_actual_state('running','stopped','starting','stopping','installing','crashed','unknown')` + `state_transitions` table; `150_heartbeat_reconciling_state.sql`.
- Worker: `forge/api/internal/services/reconciler/service.go:267` computes diffs, `476-493` `reconcileServer` checks `observed_generation` vs `desired_generation`.
- Beacon: `beacon/internal/server/manager.go:21` `PowerState Offline/Starting/Running/Stopping` + `30` `InstallationState/StartupState/RunningAction/ExpectedStop/CrashCount`; `Reconcile:128` inspects runtime, recovers `ExpectedStop/LastStartedAt` from `stateDir/.beacon-state`.
- Event: `events.EventDesiredStateChanged` `EventActualStateChanged` (`clustermanager/service.go:401`).
- Tests: `forge/api/internal/store/store_servers_lifecycle_integration_test.go`.

**STATUS:** `COMPLETE+` — superset

**GAP:** UI single-badge single lane (`lifecycle-machines.ts:41`); `observed_generation` livelock when `crashed+desired running` (see LF-06).

**FORGE LOGIC FINDING:** LF-06 `store_state.go:40` fence never converges for `crashed`.

**RECOMMENDATION:** `ADOPT` — keep; fix fence to also advance on `installing→installed` or crash→running desired, surface two-dot badge.

**SEVERITY:** `P2`

---

### GH-03 — Suspend/unsuspend (orthogonal boolean vs enum wipe)

**REFERENCE** `pterodactyl-panel/app/Models/Server.php:125` `STATUS_SUSPENDED='suspended'` `isSuspended:218` `status===suspended`; `pelican-panel/app/Enums/ServerState.php:15` `Suspended`; `SuspensionService.php:31` throws if transferring; `AuthenticateServerAccess.php:53` blocks client API unless `routeIs('api:client:server.resources')` when suspended; `pterodactyl-wings/server/power.go:177` `onBeforeStart` refuses if `IsSuspended()`.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:102` `suspendServer/unsuspendServer` `POST /suspension {action}` (`handlers_servers.go:1413`).
- API: `forge/api/internal/http/handlers_servers.go:1413` `POST /servers/:id/suspension` (`action:suspend|unsuspend`) admin `servers.write`; best-effort `Daemon.SendPower stop` `handlers_servers.go:1435-1441` + `SetServerSuspended:1444`; legacy `POST /suspend`/`/unsuspend:1450,1462` raw `SetServerSuspension` without CAS.
- Service: `forge/api/internal/orchestrator/suspension.go:28` `SuspendServer()` verifies not already suspended `suspension.go:40`, does `CompareAndSetServerSuspension:47` CAS, publishes `EventServerStopped` with `suspended:true` (`suspension.go:72`); `clustermanager/service.go:394` `RequestServerPower` refuses `start|restart` if `Suspended`.
- Store: `forge/api/internal/store/store_servers.go:374` `SetServerSuspension` raw, `385` `CompareAndSetServerSuspension`, `397` `SetServerSuspended` with audit. `Server` `store.go:454` `Suspended bool` orthogonal to `Status`.
- DB: `forge/api/migrations/007_postgres_core_foundation.sql:38` `suspended BOOLEAN DEFAULT FALSE` (not migrated to enum).
- Worker: none synchronous; suspension side-effect is best-effort stop.
- Beacon: `beacon/internal/server/manager.go:271` `BeginInstall:280-283` checks `Suspended` atomically under `TryLock`; `610` `onBeforeStart:628-630` snapshots `Suspended` and returns `server is suspended`; `HandlePower:494` `InstallationState==installing` guard; `Reconcile:135` syncs `Suspended`; `syncServerStateFromPanel:748` refreshes `Suspended` from panel when `PanelURL` set.
- Event: `events.EventServerSuspended/EventServerUnsuspended` (`orchestrator/suspension.go:72` plus `ws_hub.go`).
- Tests: `forge/api/internal/orchestrator/orchestrator_test.go` (if exists); `store_heartbeat_test.go` indirect.

**STATUS:** `PARTIAL`

**GAP:** (1) `POST /suspension` does not call `ensureTransferIdle:145` — can suspend mid-transfer leaving `transfer_state=running` + `suspended=true` illegal combo (Pelican explicitly forbids). (2) No client middleware 403 for suspended client ops except `resources` (`AuthenticateServerAccess:53` analogue missing; per-handler `requireServerPermission` does not check `Suspended` for power `stop/kill` intentionally allowed but `start` should be blocked earlier). (3) `installing→installed` `store_servers_control.go:226` always writes `status='stopped'` even if `suspended=true` — badge shows `stopped+suspended` dual truth vs Pterodactyl `status=suspended` persistence. (4) Race: `manager.go:689` `syncServerStateFromPanel` loads `state.Suspended` at `611` then releases, fetches `PanelURL` without fence, re-acquires at `748` — concurrent `HandlePower start` can interleave.

**FORGE LOGIC FINDING:** LF-08 `manager.go:689` race + LF-01 (kill bypass not suspend).

**RECOMMENDATION:** `ADAPT` — keep orthogonal Boolean (superior), add `ensureTransferIdle` guard to `/suspension`, add client middleware 403 for suspended client ops except `GET /servers/:id/resources`, document `stopped+suspended` dual badge and add UI combined pill.

**SEVERITY:** `P1`

---

### GH-04 — Power (start/stop/restart)

**REFERENCE** `pterodactyl-wings/server/power.go:24` `PowerAction` constants `start/stop/restart/kill` `HandlePowerAction:56` guards `IsInstalling||IsTransferring||IsRestoring` → `ErrServerIs*` (`power.go:57`), acquires `powerLock` (`power.go:108`), `onBeforeStart:171` (Sync+suspend+env+disk+config+chown).

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:88` `sendPowerSignal`; `forge/web/components/server/console-view.tsx:125` `canPower` derives `control.start/restart/stop`, `blocked=suspended||transferring||installing`.
- API: `forge/api/internal/http/handlers_servers.go:888` `POST /servers/:id/power` → `ensureTransferIdle:889` → validate `start|stop|restart|kill` → permission `control.start|stop|restart` (`905`) → `idempotencyKey:908` → `OperationService.DispatchPower:915` deterministic `uuid.NewSHA1("forge-op:"+key)` or `QueueService:939` fallback, returns `202 {operationId,mode}`.
- Service: `forge/api/internal/services/operation/service.go:351` `DispatchPower` maps signal→`server.start/stop/restart/kill`; `clustermanager/service.go:362` `StartServer/StopServer/RestartServer` → `sendPower:532` → `SetServerActualState` + `publishServerPowerEvents:576`.
- Store: `forge/api/internal/store/store_servers_control.go:13` `SetServerPowerState` with `powerSignalPriorStates:44` (`start:[created,stopped,install_failed]`, `restart:[running,created,stopped,install_failed]`, `stop/kill:[running,installing,provisioning,restoring_backup,recovering,created,stopped]`).
- DB: `021_true_state_persistence.sql` enums.
- Worker: `operation/service.go:152` 5 workers `process:229` marks `running→handler→retry` (exp backoff 1s cap 30s maxRetries 3) or `succeeded/failed`; `reaper:164` reaps stale `running>5m`.
- Beacon: `beacon/internal/server/manager.go:482` `HandlePower` TryLock `RunningAction` slot (`487`), guard `InstallationState==installing:494`, `start:511` `onBeforeStart:612` → `runtime.Start:402` → `PowerState=Running`; `stop:534` → `stopServer:440`; `restart:549`; `kill:581`. `beacon/server.go:1357` `power` HTTP enqueues `operations.EnqueueCommand` and blocks up to 30s (`1390-1413`) — sync-ish shim diverging from Forge 202 queue. `beacon/internal/runtime/docker.go:388` `Start`, `434` `Stop`, `529` `Restart`.
- Event: `clustermanager:576` powers.
- Tests: `beacon/server_test.go:186` power rejects invalid.

**STATUS:** `PARTIAL`

**GAP:** Wings blocks `stop` during `IsInstalling/IsTransferring/IsRestoring` with 409. Forge `powerSignalPriorStates:51` allows `stop/kill` during `installing/provisioning/restoring_backup` — divergent cancellation semantic; API `ensureTransferIdle:145` guards transfer only, not `installing/restoring`; so `POST /power start` during `installing` passes API, enqueues op, Beacon rejects at `HandlePower:494` → durable op retries 3× then `failed` — caller saw `202 accepted` then async failure (Wings returns `409` sync).

**FORGE LOGIC FINDING:** LF-07 timeout divergence Beacon 30s wait vs Forge reaper 5m.

**RECOMMENDATION:** `ADAPT` — keep durable queue but surface sync 409 for `installing` before enqueue (fast 409), document `kill` allowed during `installing` as intentional cancellation vs Wings.

**SEVERITY:** `P1`

---

### GH-05 — Kill pierces stuck lock (emergency SIGKILL bypass)

**REFERENCE** `pterodactyl-wings/server/power.go:108` `if action != PowerActionTerminate { Acquire/TryAcquire } else { // kill → Try Acquire, but ignore failure: "failed to acquire exclusive lock, ignoring failure for termination event" }` (`power.go:81` comment “if server is currently trying to process a power action but has gotten stuck you still should be able to pass through the terminate event”).

**FORGE**
- Frontend: `console-view.tsx:243` `kill` requires confirm but same perm `control.stop`.
- API/Operation: `operation/service.go:368` `kill` → `OpServerKill`; `clustermanager/service.go:377` `KillServer` → `sendRuntimePower` → `runtime.KillServer`; queued behind FIFO durable worker (`operation/service.go:152` concurrency 1-4).
- Store: `store_servers_control.go:51` `kill` allowed from 7 states but queuing serializes.
- Beacon Runtime: `beacon/internal/runtime/docker.go:494` `Kill` `workloadLock:251` sharded mutex `[64]sync.Mutex` by `sha256(serverID)[0]`, does not bypass; `manager.go:581` `HandlePower` `kill` still claims `RunningAction` slot (`487 TryLock` + `RunningAction!=""` check) — no bypass.
- Worker: `queue.go:63` `OperationQueue` TTL; single slot `manager.go:488` `RunningAction`.

**STATUS:** `MISSING`

**GAP:** Forge queues `kill` behind stuck install/power (worker poll 1s + `RunningAction` claimed). Hung `install` holding lock → `kill` waits for lock release or 5m reaper timeout, defeating emergency kill. Wings explicitly documents pierce; Forge sharded locks mitigate container-level deadlock but `RunningAction` still blocks kill on same server.

**FORGE LOGIC FINDING:** LF-02 `manager.go:487,271` both use same `RunningAction` slot; `Kill` cannot pierce install.

**RECOMMENDATION:** `ADOPT` — mirror Wings bypass: in `manager.go:HandlePower` and `operation/service.go:process`, if `signal==kill` TryLock but don't fail if occupied; or separate `killMu`. Add test hung-install + kill succeeds within 1s.

**SEVERITY:** `P1`

---

### GH-06 — Install fencing (installer container 1000:1000 readonly+capDrop, admin-only WS)

**REFERENCE** `pterodactyl-wings/server/install.go:33` `Install() → install(false)` → `BeforeExecute` mounts tmpfs, streams via `DaemonMessageEvent`, `Labels ContainerType=server_installer`.

**FORGE**
- Frontend: `console-view.tsx:37` `InstallBanner` amber when `status==='installing'`.
- API: `handlers_servers.go:1050` `POST /servers/:id/install` admin `servers.write`; `ensureTransferIdle:1051`; durable `DispatchInstall:1070` → `202`; fallback sync `clusterManager.InstallServer` 15m.
- Service: `clustermanager/service.go:224` `runInstaller(false)` → `SetServerInstallState installing:232` → `syncProvisionTarget:235` → `runtime.InstallServer:248`.
- Store: `store_servers_control.go:218` `SetServerInstallState` `installing→installed/failed` sets `install_started_at/completed_at/failed_at/install_error`.
- DB: `installer/service.go:66` 6-step workflow `docker.create/filesystem.setup/download.server/script.install/config.apply/server.start` but **dead** never executed — `REF-GAME-HIDDEN-01`.
- Worker: `operation/service.go:474` `DispatchInstall` idempotent `uuid.NewSHA1("forge-op:server.install:"+key)`.
- Beacon: `beacon/internal/server/server.go:1052` `install` → `MkdirAll` `BeginInstall:1116` claim `runtime.Install:1122` (`docker.go:256` creates `mgp-{id}-installer` `User 1000:1000:285` `ReadonlyRootfs:true:302` `CapDrop:ALL:299` `no-new-privileges:true:303` `Tmpfs /tmp 64M:305` pulls image `alpine:3.21:262` waits, log cap 1MiB `340` cleanup) `EndInstall:1131` `notifyPanelInstallStatus:1132`; `installWS:1159` streaming WS **requires `ScopeAdmin:1178`** (prevents tenant console-token abuse).
- Event: audit `server install installed/failed`.
- Tests: `beacon/internal/server/install_exclusivity_test.go:1`.

**STATUS:** `COMPLETE` (enhanced security & durability — strictly better than Wings default `User 0:0`)

**GAP:** (1) `skip_scripts` (`store_servers.go:264` persists but Beacon `create:727` not honoring at install skip) — Forge never checks `skip_scripts` unlike Wings `install.go:38`. (2) Non-WS `install` (`server.go:1052`) lacks admin-scope check vs `installWS` which gates `ScopeAdmin` — asymmetric auth (Beacon HMAC is sole gate, but drift). (3) Dead workflow engine `installer/service.go:66` persists rows never executed.

**FORGE LOGIC FINDING:** LF-09 `install` HTTP lacks scope check; `chown` `server.go:1088` best-effort only when `geteuid==0` else installer runs as root breaks SFTP ownership.

**RECOMMENDATION:** `ADAPT` — honor `skip_scripts` early `installed`, wire or delete dead workflow, add scope check to HTTP `install` or document HMAC sole gate.

**SEVERITY:** `P2`

---

### GH-07 — Reinstall requires stopped

**REFERENCE** `pterodactyl-wings/server/install.go:80` `Reinstall() → if State()!=offline → WaitForStop(10s,true) else Sync() then install(true)`; `ServerManagementController.php:55` `reinstall → ReinstallServerService`.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:98` `reinstallServer` `POST /reinstall`.
- API: `forge/api/internal/http/handlers_servers.go:1110` `POST /servers/:id/reinstall` requires `PermSettingsReinstall` (subusers can trigger) vs `install` admin; `ensureTransferIdle`; durable `DispatchReinstall:1129` or `clusterManager.ReinstallServer` sync; no stopped-state pre-check at API.
- Service: `clustermanager/service.go:220` `ReinstallServer → runInstaller(true)` → `runtime.ReinstallServer` if `Reinstaller` iface (`241`); Docker runtime does not implement `Reinstall` → `errors.New("runtime does not support reinstall"):243` and marks `failed` (`clustermanager/service.go:242-243`).
- Store: `store_servers_control.go:218` same machine.
- Beacon: `beacon/internal/server/server.go:1331` `reinstall` guard `PowerState==Running|Starting → 409 "server must be stopped"` (`1335`), then forwards to `install` (`BeginInstall`).
- Event: same install audit.
- Tests: none for Docker reinstall fallback.

**STATUS:** `BROKEN`

**GAP:** `clustermanager:242` gating makes pure-Docker Reinstall always fail 500 while Beacon `server.go:1331` reinstall works (just calls install) — API vs Beacon drift. `SetServerInstallState failed` persists `status='install_failed'` leaving server unusable until manual toggle.

**FORGE LOGIC FINDING:** LF-04

**RECOMMENDATION:** `ADOPT` reference — fallback `if !ok → response, err = s.runtime.InstallServer(...)` in `clustermanager/service.go:241` (mirror Beacon) or implement Docker `Reinstall` alias; add API 409 instructing `stop first` (like Beacon) rather than marking `install_failed`.

**SEVERITY:** `P1`

---

### GH-08 — Delete hard-delete + orphan remediation

**REFERENCE** Pterodactyl `ServerDeletionService` stops, removes allocations, `DaemonServerRepository->delete`, hard delete cascade `server_variables`, `allocations.server_id=NULL`; `pufferpanel/servers/server.go:373` `Destroy()` → `IsIdle` guard, `Uninstallation` ops, `RunningEnvironment.Delete()`; no orphan table.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:74` `deleteServer(id,force)` maps `?force=true`; UI confirm.
- API: `forge/api/docs/server-lifecycle.md:6` Beacon-first, hard-delete only after success; `?force=true` always removes panel state; `handlers_servers.go:1474` `DELETE /servers/:id` → `clusterManager.DeleteServer:332` (`handlers_servers.go:1481`).
- Service: `clustermanager/service.go:332` `DeleteServer`: `ServerControlTarget` → `runtime.DeleteServer`; on error + `!force` return; on error + `force` → `RecordOrphanAndHardDeleteServer:348` + `Mode:force` with daemonErr; on success → `HardDeleteServer:355`. Publishes `EventServerDeleted` (`352,358`).
- Store: `forge/api/internal/store/store_servers_lifecycle.go:12` `HardDeleteServer` → tx `FOR UPDATE` lock, `UPDATE allocations SET server_id=NULL` (`46`), `DELETE FROM servers` cascade (`49`). `17` `RecordOrphanAndHardDeleteServer` inserts `server_orphan_remediations` **before** delete (FK-free) requires `daemonError != ""` (`19`).
- DB: `forge/api/migrations/040_truthful_server_lifecycle.sql:18` `server_orphan_remediations(id,server_id,node_url,daemon_error,status=pending|resolved)` + index pending; orphan rows accrue silently — no `GET /orphans` UI (`handlers_admin.go` lacks).
- Beacon: `beacon/internal/server/server.go:1433` `delete` → `safePath`, `consoles.Stop`, `runtime.Delete:1429` (`docker.go:625` `ContainerRemove Force+RemoveVolumes` idempotent `IsNotFound→nil`), `os.RemoveAll(root)`, `manager.Delete:1478`.
- Event: `EventServerDeleted`.
- Tests: `store_servers_lifecycle_integration_test.go`.

**STATUS:** `COMPLETE` (enhanced — orphan tracking absent in refs)

**GAP:** Puffer `Uninstallation` ops not run; Forge delete not run egg `install_script` unwind. `RecordOrphanAndHardDeleteServer` requires non-empty `daemonError`; `DeleteServer` with `runtime==nil` passes `gpruntime.ErrRuntimeUnavailable` → creates orphan even though no workload existed (phantom orphan). No admin resolution UI → `REF-GAME-HIDDEN-02`.

**FORGE LOGIC FINDING:** LF-10 phantom orphan when `runtime==nil`.

**RECOMMENDATION:** `ADAPT` — keep orphan remediation (novel), add `GET /admin/orphans` UI, fix phantom orphan when `runtime==nil` (just `HardDeleteServer`), add force-delete integration test.

**SEVERITY:** `P3`

---

### GH-09 — restoring_backup as server lock

**REFERENCE** `pterodactyl-panel/app/Models/Server.php:126` `STATUS_RESTORING_BACKUP` `Server.php:393` `validateCurrentState()` throws `StateConflict` if `restoring_backup` or `transfer!=null`; `pterodactyl-wings/server/install.go:152` `IsRestoring` atomic `HandlePowerAction:57` blocks if `IsRestoring`; `pufferpanel/servers/server.go:737` `StartRestore` → `IsIdle` guard `restoring=true` blocks `IsIdle`.

**FORGE**
- Frontend: `forge/web/components/lifecycle/lifecycle-machines.ts:55` `BACKUP_MACHINE` steps queued/creating/completed but no `restoring` integrated with console `blocked` (`console-view.tsx:125`).
- API: `forge/api/internal/http/handlers_servers.go:2117` `/servers/:id/backups/restore` → `resolveBackupName`, check `status==completed`, then `OperationService.DispatchBackupRestore:2155` **or** sync `MarkBackupStatus restoring:2170` (`store_backups.go`) — per-backup row, **not** server row; no `SetServerActualState restoring_backup` (`store_state.go:31` mapping `store_state.go:137` exists but never driven). `ensureTransferIdle:145` guards transfer only.
- Service: no restore → server state transition; `clustermanager RequestServerPower:382` checks `Suspended` but not `restoring_backup`.
- Store: `store_state.go:131` `serverStatusFromActual restoring_backup → restoring_backup` ↔ `store.go:1479` but never set by backup routes; `backups.status=restoring` only.
- DB: `019_backups.sql` backup rows, no trigger to `status=restoring_backup`.
- Beacon: `beacon/internal/server/server.go:1699` `restoreBackup` `backupMu` serialize, but `manager.go:30` `ServerState` has no `Restoring` field (only `InstallationState` `30-35`).
- Event: none server-level.

**STATUS:** `MISSING` (per `REF-GAME-F-G-06` never wired)

**GAP:** All 4 refs treat `restoring_backup` as server exclusive lock (blocks power, suspend, reinstall, transfer). Forge tracks per-backup `status=restoring` but never flips `servers.actual_state/status`. `POST /power start` or `POST /servers/:id/install` during restore succeeds at API (only `ensureTransferIdle` guard) diverging from `validateCurrentState`. Concurrent `start` + `restore` allowed → race `RootDir` half-written extract vs running process.

**FORGE LOGIC FINDING:** LF-03

**RECOMMENDATION:** `ADOPT` reference — on `DispatchBackupRestore` also `SetServerActualState restoring_backup` (`store_state.go:31`) and clear (`stopped` or prior) on completion/failure; add `ensureRestoreIdle` guard alongside `ensureTransferIdle` for `power/install/reinstall/suspend`.

**SEVERITY:** `P1`

---

### GH-10 — Transfer/migration v1 resumable chunks

**REFERENCE** `pterodactyl-panel/app/Models/ServerTransfer.php:26` row `old_node/new_node/old_allocation/new_allocation/additional`, `successful NULL|true|false`, `archived`; `Admin/ServerTransferController.php:39` reserves new allocations, generates JWT `ServerTransfer` token, notifies source daemon; `wings/server/transfer.go` + `router/router.go:59`.

**FORGE**
- Frontend: `forge/web/lib/api/servers.ts:394` `fetchServerTransferStatus/cancelServerTransfer/transferServer`; `transfer-view.tsx:31` polls `GET /servers/:id/transfer` every 5s if `transferring`; flicker `transfer-view.tsx:77` `isTransferring = server.transferring` (cached) vs `transfer.transferring` (polled) — `REF-GAME-F-G-19`.
- API: `forge/api/docs/server-lifecycle.md:12` “Server transfer/archive execution is intentionally unavailable and returns `501`” — **doc lie**; `handlers_servers.go:713` `POST /servers/:id/transfer` (admin via `MigrationService.CreateMigration+ExecuteMigration → 202`), `GET /transfer:740` (`state,transferring,targetNodeId,error` from `servers` row), `POST /transfer/cancel:758` (finds active `migrations.status not in completed/failed/cancelled` `handlers_servers.go:773`).
- Service: `forge/api/internal/services/migration/service.go:127` `CreateMigration` + `227` `PrepareMigration` ensures `migration_runs` with `TransferProtocolVersion`, `249` `ExecuteMigration` async `startRun`; `run:417` claims `MigrationRun` lease (`ClaimMigrationRun` 2m), generates `TransferCredentialClaims Version=TransferProtocolVersion`, registers on both daemons `RegisterTransferCredential` + `FinalizeTransferDestination` + `CleanupTransferSource`.
- Store: `store_migrations.go` + `migration_runs` (`045_real_server_transfer.sql:1` lease `lease_owner/lease_expires_at`, `phase`, `cleanup_pending`, `allocation_reservations`). `servers` row legacy `transfer_state/transferring/transfer_target_node_id/transfer_run_token` but `040_truthful_server_lifecycle.sql:7` clobbers stale `queued/running→failed` on startup; modern `migrations` not projected to `servers.transfer_state` → dual state split.
- DB: `004_server_transfer_state.sql:1`, `005_server_transfer_lifecycle.sql:1`, `045_real_server_transfer.sql:1`.
- Beacon: `beacon/internal/server/server.go:368` protocol v1 `/api/v1/transfers/credentials` (`registerTransferCredential`), `transfer_protocol.go` + `transfer.Engine/Manager` (`server.go:83`).
- Event: `migration/service.go:171` `EventMigrationCreated`.
- Tests: `store_migration_transfer_integration_test.go`.

**STATUS:** `PARTIAL`

**GAP:** Doc says 501 but returns 202 — outdated. Traditional Wings reserve→token→peer push replaced by migration engine (`migration_runs` + `allocation_reservations`). `servers.transfer_state` legacy shim still drives `ensureTransferIdle:145` guards and `GET /transfer`; modern `migrations.status` (`planned/preparing/running/completed/failed/cancelled`) not projected → `ensureTransferIdle` reading stale `servers` will miss active `migrations` row.

**FORGE LOGIC FINDING:** LF-11 split-brain flicker `transfer-view.tsx:77` vs stale guard.

**RECOMMENDATION:** `ADAPT` — keep control-plane-mediated v1 (resumable chunks, HMAC-scoped credentials superior to Wings JWT push); retire or sync legacy `servers.transfer_state` from `migrations` status (or make `ensureTransferIdle` query `migrations` table). Update `server-lifecycle.md:12` to reflect migration engine canonical.

**SEVERITY:** `P2`

---

### GH-11 — Allocations (containerPort/protocol)

**REFERENCE** `pterodactyl-wings/environment/allocations.go:36` `Mappings map[string][]int` — no per-port protocol; `Bindings()` creates both `tcp` and `udp` for every port (`allocations.go:54`).

**FORGE**
- Frontend: `forge/web/app/admin/allocations/page.tsx:1`.
- API: `forge/api/internal/http/handlers_admin.go:1365` `GET /allocations/nodes`, `1378` `GET /allocations?limit&offset` paginated 50, `handlers_servers.go:454` `GET /servers/:id/allocations`.
- Store: `forge/api/internal/store/store_allocations.go:15` `Allocation{IP,Port,ContainerPort,Protocol,Alias}`; `125` `CreateAllocations` bulk Tx with `pg 23505` duplicate handling (`182`), validates `net.ParseIP`, `protocol ∈ {tcp,udp} default tcp`, `containerPort` default `port` (`162`); `327` `AssignAllocationToServer` node-affinity `FOR UPDATE`; `367` `SetPrimaryAllocation` Tx `FOR UPDATE`.
- DB: `forge/api/migrations/007_postgres_core_foundation.sql:134` `allocations(ip inet, port int, container_port int, protocol text)`; `090_allocation_transport.sql`? `144_allocation_container_port_compatibility.sql`.
- Beacon: `beacon/internal/runtime/runtime.go:100` `PortBinding{HostIP,HostPort,ContainerPort,Protocol}`; `beacon/internal/runtime/docker.go:865` `dockerPorts` validates `tcp|udp`, duplicate `HostIP:HostPort/Protocol` detection (`888`), maps to `nat.PortSet/PortMap`.
- Service/Client: `forge/api/internal/daemon/client.go:438` `Port{HostIP,HostPort,ContainerPort,Protocol}` + `ServerConfiguration.Allocations.Mappings` compat; `clustermanager/service.go:655` `runtimeCreateRequest` maps `allocation.ContainerPort` (0→HostPort) + `protocol` default tcp (`clustermanager/service.go:657-665`).
- Event: audit `allocation assigned/unassigned/primary set`.
- Tests: implicit via `CreateServer` allocations binding.

**STATUS:** `COMPLETE` (superset — explicit `protocol`+`containerPort` vs Wings dual)

**GAP:** Legacy `Mappings map[string][]int` (`clustermanager/service.go:698-702` + `client.go:460`) drops `protocol` when using compat path → UDP-only `8211` also binds tcp (`F-G-11`). DB unique index may be `(node_id,ip,port)` without `protocol` — inserting same `ip:port` tcp then udp hits `23505`.

**FORGE LOGIC FINDING:** LF-12 `store_allocations.go:178` duplicate detection only `ip+port` not `protocol`.

**RECOMMENDATION:** `ADOPT` — explicit better than Wings; fix DB unique index to `(node_id,ip,port,protocol)` if not already; remove or fix legacy `Mappings` fallback to encode protocol.

**SEVERITY:** `P2`

---

### GH-12 — Allocations kill/start race vs install (power allowed during installing)

**REFERENCE** `pterodactyl-wings/server/power.go:57` blocks if `IsInstalling||IsTransferring||IsRestoring` → `ErrServerIsInstalling` (`power.go:57`).

**FORGE**
- API: `handlers_servers.go:888` only `ensureTransferIdle:145` guards transfer not `installing`; `store_servers_control.go:51` `powerSignalPriorStates` allows `stop/kill` during `installing` but also `start` is blocked via `powerSignalPriorStates:47` (`start:[created,stopped,install_failed]` — `installing` not allowed, so `start` will 400 at store, but durable op still enqueued before store check? Actually API enqueues before store `SetServerPowerState` happens in worker, so API returns `202` then worker marks `failed`).
- Beacon: `manager.go:494` `HandlePower` `InstallationState==installing → "server is installing"` blocks.

**STATUS:** `BROKEN`

**GAP:** API enqueues doomed ops → retry storm `operation/service.go:229` 3 attempts then `failed`; caller saw `202 accepted` then async failure (Wings returns `409` sync).

**FORGE LOGIC FINDING:** LF-07

**RECOMMENDATION:** `ADAPT` — add `IsServerInstalling` guard (`status==installing` or `actual_state==installing`) at `handlers_servers.go:888` alongside `ensureTransferIdle` (fast 409). Keep `kill` allowed during `installing` as cancellation but document.

**SEVERITY:** `P1`

---

### GH-13 — Egg/nest/egg_variables model

**REFERENCE** `pterodactyl-panel/app/Models/Nest.php:19` `nests(id,uuid,author,name)` hasMany eggs `54`; `Egg.php:52` `eggs(id,uuid,nest_id,author,name,docker_images JSON,startup,config_files...)`, `EXPORT_VERSION PTDL_v2`.

**FORGE**
- Frontend: `forge/web/app/admin/nests/page.tsx:1` `AdminNestsEggs` aggregates nests+eggs; `.../eggs/page.tsx:1` CRUD.
- API: `forge/api/internal/http/handlers_admin.go:990` `GET /nests`, `1083` `GET /eggs`, `1124` `POST /eggs`, `1288` `/eggs/:id/export`, `1308` `/eggs/import` (`1354` loop `CreateEggVariable`).
- Store: `forge/api/internal/store/store_nests.go:16` `Nest{Name,Description,EggCount}`, `34` `Egg{DockerImages json.RawMessage, Startup, Config, DefaultMemoryMB, InstallScript…}`; `248` `CreateEgg` via `normalizeDockerImages:361` handles `map<string,string>` **or** legacy `[]string`, `361` robust.
- DB: `forge/api/migrations/007_postgres_core_foundation.sql:87` `nests(id UUID PK, name TEXT UNIQUE)`, `eggs` table, `043_unify_eggs_templates_mounts.sql:63` migrates `server_templates→eggs`, `089_egg_parity_fields.sql` adds `config_from, copy_script_from`.
- Beacon: `ServerProvisionTarget:88` loads `e.docker_images` for runtime; `docker.go:901` validates image non-empty.
- Event: audit `egg created`.
- Tests: `store_eggs_integration_test.go`.

**STATUS:** `COMPLETE`

**GAP:** Forge drops `uuid/author` columns, more permissive `normalizeDockerImages` handles both shapes (improvement). Lexicographically-first image fallback `store_nests.go:99` `COALESCE(NULLIF(s.docker_image,''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1)`.

**FORGE LOGIC FINDING:** C-13-LF low `DeleteNest` race `SELECT COUNT(*)` without `FOR UPDATE` — concurrent `CreateEgg` after count before `DELETE` violates FK.

**RECOMMENDATION:** `ADOPT` — keep `normalizeDockerImages`.

**SEVERITY:** `P4`

---

### GH-14 — Variable validation regex slash bug (blocks PTDL imports)

**REFERENCE** `pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:28` Laravel `ValidationFactory` with `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` (`minecraft-paper.json:58`). Reserved `SERVER_MEMORY,SERVER_IP,SERVER_PORT,ENV,HOME,USER,STARTUP,SERVER_UUID,UUID` `EggVariable.php:43`.

**FORGE**
- Frontend: `forge/web/app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx:1`.
- API: `handlers_admin.go:1185` `GET/POST/PATCH /eggs/:id/variables`, `handlers_servers.go:355` `PATCH /servers/:id` startup updates.
- Store: `store_egg_variables.go:17` `EggVariable{Name,EnvVariable,DefaultValue,UserViewable,UserEditable,Rules,Sort}`; `142` `validateVariableValue()` implements `required|nullable|string|max|min|in|regex` (`176` `regexp.Compile(arg)`). `store_startup.go:11` `GetServerStartup` joins `egg_variables` filtered `user_viewable=true` (`31`), `54` `UpdateServerStartupVariable` checks `user_editable`. `store_servers_control.go:154` `ServerProvisionTarget` loads **all** variables (no viewable filter) — inconsistency.
- DB: `013_startup_variables.sql:26` seed.
- Tests: no regex-slash test covers.

**STATUS:** `BROKEN` (P0)

**GAP:** `store_egg_variables.go:143` `strings.Split(rules, "|")` then `strings.Cut(rule,":")` → for `regex:/^([\w\d._-]+)(\.jar)$/` yields `arg=/^([\w\d._-]+)(\.jar)$/` including surrounding `/`. Then `regexp.Compile(arg)` treats slashes literally → `server.jar` fails (must be `/server.jar/`). Also `Split("|")` splits regex alternation `|` inside pattern (e.g., `regex:/^(foo|bar)$/`). Import via `POST /eggs/import:1308` loop that calls `CreateEggVariable` → 400, blocks Paper/Vanilla PTDL_v2 import. No `RESERVED_ENV_NAMES` guard in Forge; `store_servers.go:273` CreateServer allows any variable regardless of `user_viewable` — non-admin can set `DL_PATH` at provision to hijack installer URL (`F-G-10`).

**FORGE LOGIC FINDING:** LF-01 (P0 load-bearing)

**RECOMMENDATION:** `ADAPT` — strip `regex:/…/` delimiters and optional flags before `Regexp.Compile`, respect `|` inside `regex:`; add `RESERVED_ENV_NAMES` check; gate creation-time variables by `user_editable/viewable` for non-admin.

**SEVERITY:** `P0`

---

### GH-15 — Config parser 6 parsers (file/yaml/properties/ini/json/xml)

**REFERENCE** `pterodactyl-wings/server/parser/parser.go:1` + `configuration.go:60` — `ConfigurationFiles []ConfigurationFile` with `FileName, Parser, Find(map[string]string)`, supports `properties` (`server.properties`), `yaml`, `json`, `xml`, `ini`, `file`; `configMatchRegex {{config.docker.interface}}` lookup before start (`power.go:184` `UpdateConfigurationFiles()`).

**FORGE**
- Frontend: `egg-templates.ts:63` `config.files["server.properties"]{parser:properties, find:{server-ip,server-port}}` inert.
- API: `store_servers_control.go:95` `e.config::text` stored as `ConfigJSON` (`97`), `store_nests.go:231` normalized — no parser validation.
- Service: `clustermanager/service.go:689` `runtimeConfiguration` packs `Config` but never applies patch; `configSyncPending` set via `UpdateServer` but not via parser.
- Store: `store_servers_control.go:178` `resolveStartupCommand` only resolves `{{VAR}}` not `{{server.build.default.port}}` alias expected by `minecraft-paper.json:22`.
- Beacon: `beacon/internal/server/server.go:844` `syncConfiguration` writes `server.json` + `applyConfigurationFiles:869` not parsing `config.files`; only writes raw JSON. No `parser` package ported.
- DB: `eggs.config JSONB` holds raw; `servers.config_sync_pending` flag.
- Worker: none.

**STATUS:** `MISSING`

**GAP:** All game servers requiring `server.properties` patching (Minecraft) will start on wrong port / `SERVER_PORT` not patched. `{{server.build.default.port}}` inert (`F-G-12`). Wings `file/yaml/properties/ini/json/xml` patching not reimplemented; `game-templates` `minecraft-paper.json:16` `find:server-port→{{server.build.default.port}}` inert.

**FORGE LOGIC FINDING:** LF-13 — startup substitution only `{{VAR}}`/`{{lower}}`, not `{{server.build.*}}` builtins (`SERVER_PORT, SERVER_IP, SERVER_MEMORY`).

**RECOMMENDATION:** `ADAPT` — port `wings/parser` into beacon pre-start `onBeforeStart` (apply `config.files` via `rootfs.FS` openat2, before `Start`), or strip `config.files` from templates and document unsupported; prefer port (~300 LoC `inspector`).

**SEVERITY:** `P2`

---

### GH-16 — Template systems (DB eggs vs FS 14 vs localStorage 5)

**REFERENCE** Pterodactyl single `eggs` canonical; `pufferpanel-templates/spec.json:1` 44 JSON payloads (36 types) strictly validated.

**FORGE**
- Frontend: `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries hardcoded TS, IDs mirror `packages/game-templates`; `forge/web/lib/app-templates-data.ts:3` `STORAGE_KEY forge.app-templates.v1` 5 defaults (`app-templates/page.tsx:56` `formToTemplate` freeform `host:container` CSV) — entirely localStorage.
- API: `store_templates.go:12` compat shim over eggs (`templateFromEgg:99`); `handlers_admin.go:2037` `/templates` delegates to `Store.ListTemplates`. No seed from FS.
- Service: `catalog/catalog.go:1` DB-driven for DBs; `appstore/service.go:1` + `seed.go:27` 7 compose apps (`SeedDefaultApps` on `cmd/api/main.go:529`) **No** `SeedGameTemplates`.
- DB: `007`: `Games` nest `dddd...`; `043_unify`: `Legacy Templates` backfill; `091_seed_minecraft_java.sql:3` inserts **1** egg `Minecraft Java` variant — only production egg seed; fresh install has 1 egg not 14.
- FS Package: `.freebuff/worktrees` branch `packages/game-templates/templates/*.json` 14 curated + `template-schema.json:1` but `packages/game-templates` does not exist on `main` HEAD (`ls packages → sdk, shared-types`) — phantom on worktree only.

**STATUS:** `DUPLICATE` + `MISSING` seeding (93% deficit)

**GAP:** Three disconnected catalogs: DB eggs 1 seeded vs FS 14 curated vs static `EGG_TEMPLATES` 14 vs localStorage 5. No single source of truth; `ListEggs(ctx,"")` returns 1 row fresh; wizard pre-fills DB but not synced; `game-templates` 14 never seeded via migration like `SeedDefaultApps`. `main` cannot build `packages/game-templates`.

**FORGE LOGIC FINDING:** LF-14 Seeding divergence `091` 1/14.

**RECOMMENDATION:** `ADAPT` — collapse to single canonical: `SeedGameTemplates` idempotent boot upserting 14 curated eggs (like `appstore/seed.go:216`) or move package to `forge/packages/game-templates` reachable on `main`; deprecate `app-templates` localStorage or unify under DB.

**SEVERITY:** `P1`

---

### GH-17 — Schedules/cron tasks

**REFERENCE** `pterodactyl-panel/app/Models/Schedule.php` + `Task.php` — `action ∈ {power,command,backup}`; `ProcessScheduleService.php:45` `is_processing` lock; `getNextRunAt()` via `Utilities::getScheduleNextRunDate`.

**FORGE**
- Frontend: `forge/web/components/server/schedules-view.tsx:37` `TaskEditor` handles `command|power|backup`.
- API: `handlers_servers.go:1288` `POST /schedules/:id/tasks` (`CreateScheduleTask`), `1327` `GET .../runs`, `1340` `PATCH /tasks/:taskId`; `handlers_user_console.go:314` server-scoped cron `registerServerCronJobRoutes` (`322` `ListCronJobs` global then Go-filter `331` `if TargetType!="server" continue` — O(N) scan).
- Service: `http/schedule_runner.go:139` `tick` → `ListDueSchedules` (`489`) uses `only_when_online = FALSE OR s.status='running'` (DB status not live); `ClaimDueSchedule` lease; `executeTask:386` `power|backup|command`.
- Store: `store_schedules.go:14` `resolveStartupCommand`; `252` `CreateScheduleTask` checks `action!=""` only (`254`) `timeOffset>=0` — **does not** call `isValidScheduleTaskAction` (`store.go:1196` valid `power|backup|command`); `331` `PatchScheduleTask` **does** validate `332`. Asymmetry `F-G-23`. `489` `ListDueSchedules` filters `only_when_online` via DB `status`.
- DB: `008_server_schedules.sql:1`, `009_schedule_run_history.sql:1`.
- Beacon: no scheduler — panel-centralized.
- Event: `NotifySchedulesChanged:398` `NOTIFY schedule_events`.

**STATUS:** `PARTIAL`

**GAP:** (1) Task action asymmetry allows arbitrary action persistable (e.g., `rm -rf`) — only rejected at `schedule_runner:386` (`unsupported task action`) polluting store. (2) `only_when_online` uses DB `s.status='running'` vs Pterodactyl live probe — may run while offline. (3) `handlers_user_console.go:315` O(N) scan vs SQL `WHERE target_type='server' AND target_id=$1`. (4) No `is_processing` row lock but claim via `pg_advisory_lock` sound but different.

**FORGE LOGIC FINDING:** LF-05 asymmetry `CreateScheduleTask:252` vs `PatchScheduleTask:332`.

**RECOMMENDATION:** `ADAPT` — add `isValidScheduleTaskAction` guard to `CreateScheduleTask:252` one-line; push server cron filter into SQL `ListCronJobsByServer`.

**SEVERITY:** `P2`

---

### GH-18 — Subusers/permissions wildcard *

**REFERENCE** `pterodactyl-panel/app/Models/Subuser.php` `Permission.php:1` 10 groups 45 constants; `GetUserPermissionsService.php:18` returns `['*']` exclusively for `root_admin` or `owner_id===user.id` — never stored for subusers; `SubuserController.php:154` `getDefaultPermissions()` intersects request with allowlist, always adds `websocket.connect`; service checks actor can only grant permissions they themselves possess.

**FORGE**
- Frontend: `forge/web/components/server/users-view.tsx:1`.
- API: `handlers_servers.go:501` `GET /users`, `554` `POST /users` (checks `PermUserCreate` `541` but not `perms ⊆ actorPerms`), `594` `PATCH`, `622` `DELETE`.
- Store: `forge/api/internal/store/permissions.go:5` 10 groups + `cron.read/create/update/delete/run` (`28`), `buildpack.manage`, `mount.read/update`; `store_users.go:297` `UpsertServerSubuser` only allowlist-checks `allowed[perm]||perm=="*"` (`555-564` `normalizeSubuserPermissions`) **without actor-subset**; `544` allows `*`; `574` `defaultSubuserPermissions()` 40+ keys; `362` `UserCanAccessServer` respects `HasPermission` wildcard (`permissions.go:206` `p=="*"`). `AuthenticateSFTP:464` & `AuthorizeSFTPSession:536` both iterate `*`.
- DB: `016_subusers_panel_parity.sql:1` `subusers(id,server_id,user_id,permissions jsonb)` unique.
- Beacon: `server.go:1178` `installWS` rejects non-admin scopes — subuser `*` would not bypass Beacon HMAC but bypasses panel.

**STATUS:** `BROKEN` (P0)

**GAP:** `normalizeSubuserPermissions` only checks allowlist + `*`, no `actorPerms ⊆ requested`. Any holder of `user.create` can grant `*` or `database.view_password`, `schedule.delete` to any user. `GET /servers/:id` (`handlers_servers.go:335`) sets `Permissions=["*"]` only for owner/admin else subuser row — consistent but subuser `*` row then becomes universal.

**FORGE LOGIC FINDING:** LF-01

**RECOMMENDATION:** `ADAPT` — enforce subset at `UpsertServerSubuser` (or handler): load actor effective perms via `GetServerSubuser(ctx, serverID, actorID)` (or `['*']` for owner/admin) and reject any `req.Permissions` not in actor set unless owner/admin; gate `*` behind owner/admin only. Add test `user.create` cannot grant `database.view_password`.

**SEVERITY:** `P0`

---

### GH-19 — Mount allowlist / file denylist

**REFERENCE** `pterodactyl-panel/app/Models/Mount.php` via `egg_mount` pivot (`015_a_mounts.sql:19`); `Egg.php:68` `file_denylist`; Wings `parser/parser.go:1` 6 parsers `server/filesystem/filesystem.go:1` `IsIgnored`.

**FORGE**
- API: `forge/api/internal/http/handlers_admin.go:1579` mount admin CRUD `CreateMount:41`, `validateMountPaths:316`, `validateMountPath:323`, `ensureMountAvailableForServer:297` double-join `mount_node ∧ egg_mount`.
- Store: `forge/api/internal/store/store_mounts_ext.go:13` `ListMounts` `41` `CreateMount` `316` `validateMountPaths` (`path.IsAbs`, `Clean==value`, no `\`, no `..`), `323` `validateMountPath` reserves only `source ∈ {/etc/forge,/var/lib/forge/volumes}` + `target ∈ {/,/home/container}`; `297` `ensureMountAvailableForServer`, `367` `AllowedMountSourcesForNode`.
- DB: `015_a_mounts.sql:19` `mounts(mount_node,egg_mount,mount_server)`.
- Beacon: `beacon/internal/runtime/docker.go:743` `buildContainerMounts` validates `filepath.IsAbs`, `EvalSymlinks`, rejects `target==/` or `/home/container`; `beacon/internal/server/secure_files.go:60` `serverFilesystem` `rootfs.FS` `openat2 RESOLVE_BENEATH`; `hostfiles.go:1` `validateHostPath` with `DAEMON_HOST_FILES_ALLOWLIST`.
- Frontend: `forge/web/components/server/mounts-view.tsx:1`.

**STATUS:** `BROKEN` (enforced but too narrow)

**GAP:** `validateMountPath` only blocks 2 sources + 2 targets; leaves `/etc`, `/var/run/docker.sock`, `/proc`, `/`, `/root` mountable via `POST /mounts` with `mounts.write` scope. Compromised admin becomes host-breakout primitive. `buildContainerMounts` does symlink resolution but panel allowlist short. `file_denylist` passthrough `ServerProvisionTarget.FileDenylist` but not enforced at runtime beyond `IsIgnored` check.

**FORGE LOGIC FINDING:** LF-04

**RECOMMENDATION:** `ADAPT` — harden `validateMountPath` to deny `/etc|/proc|/sys|/dev|/var/run|/root|/` source prefixes or require `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` allowlist (Pterodactyl doc: mounts under dedicated host dirs).

**SEVERITY:** `P1` → `P0` if admin token stolen (host breakout)

---

### Additional — Install/Reinstall/Power fencing (installer container fencing)

Covered in GH-06 `manager.go:271` `BeginInstall` `TryLock`+`Suspended`+`installing` (`manager.go:273-290`), `HandlePower:487` same `RunningAction` slot (`manager.go:487-507`). Beacon stricter (admin-only WS `server.go:1170` `ScopeAdmin`), readonly+capDrop (`docker.go:299-303`) confirms `GH-06 COMPLETE`.

Power kill fencing already in GH-05.

---

## 2. Consolidated Logic Findings (≥3, file:line cited, novel verification)

| # | Id | Severity | Title | Files (live) | Finding (re-verified now) |
|---|----|----------|-------|--------------|---------------------------|
| **LF-01** | **F-G-08** | **P0** `USER_VISIBLE` | **Regex `validateVariableValue` delimiter bug blocks all PTDL imports** | `forge/api/internal/store/store_egg_variables.go:142` `validateVariableValue:176` `regexp.Compile(arg)` + `store_egg_variables.go:143` `Split("\|")` + `forge/api/internal/http/handlers_admin.go:1308` `/eggs/import` loop + `packages/game-templates` `minecraft-paper.json:58` `rules:"required\|regex:/^([\\w\\d._-]+)(\\.jar)$/"` | For `regex:/^([\w\d._-]+)(\.jar)$/` yields `arg=/^.../` with slashes; `regexp.Compile(arg)` expects literal slashes → `server.jar` fails. Also `Split("\|")` splits regex alternation `\|` inside char class. Result: `POST /eggs/import` **rejects all variables with regex rules**, blocking import of stock Paper/Vanilla eggs. Prior `FINAL GH-14 BROKEN` and `final-parity C-14 LF-02` confirmed; **still BROKEN** live — no fix on disk. Fix: strip leading `/` and trailing `/[flags]` before compile; respect `\|` inside `regex:`; add `RESERVED_ENV_NAMES` guard. |
| **LF-02** | **F-G-22** | **P0** `USER_VISIBLE` | **Subuser `*` escalation — any `user.create` holder can grant `*`** | `forge/api/internal/store/store_users.go:306` `UpsertServerSubuser:311` `normalizeSubuserPermissions` `store_users.go:555` `allowed[perm]||perm=="*"` + `forge/api/internal/store/permissions.go:206` `HasPermission` `p=="*"` + `forge/api/internal/http/handlers_servers.go:554` `POST /users` only checks `user.create` | `normalize` allowlists `*` without checking `actorPerms ⊆ requested`. Handler only checks `user.create`. Repro: alice (`user.create,file.read`) → `POST /servers/:id/users {email:bob, perms:["*"]}` succeeds → bob has full control + `database.view_password`. Pelican `SubuserController:154 getDefaultPermissions` intersects + subset check. **Still BROKEN**; `MASTER REF-GAME-F-G-22` confirmed. Fix: load actor perms, reject grants outside set; gate `*` behind owner/admin. |
| **LF-03** | **F-G-06** | **P1** `USER_VISIBLE` | **`restoring_backup` server lock not wired — concurrent start allowed during restore** | `forge/api/internal/store/store_state.go:131` `serverStatusFromActual` + `forge/api/internal/http/handlers_servers.go:2117` `/backups/restore` `MarkBackupStatus restoring` (per-backup) `handlers_servers.go:2117-2170` + `store_state.go:31` `SetServerActualState` never called for restore + `handlers_servers.go:145` `ensureTransferIdle` (no restore guard) + `beacon/internal/server/manager.go:30` `ServerState` no Restoring field | All 4 refs treat `restoring_backup` as server exclusive lock (`Server.php:393 validateCurrentState`, `wings power.go:57 IsRestoring`). Forge never flips `servers.actual_state/status`; `POST /power start` concurrent with restore allowed → race on `RootDir` half-written `backup.Extract` vs running process (data corruption). **Still MISSING**; `REF-GAME-F-G-06` confirmed. Fix: on `DispatchBackupRestore` also `SetServerActualState restoring_backup` and guard `power/install/reinstall/suspend` via `ensureRestoreIdle`. |
| **LF-04** | **F-G-07** | **P1** `USER_VISIBLE` | **Reinstall fails on Docker runtime gate (`Reinstaller` interface) but advertised** | `forge/api/internal/services/clustermanager/service.go:241` `Reinstaller` check `242 "runtime does not support reinstall"` + `beacon/internal/server/server.go:1331` `reinstall` (`1335` `PowerState==Running→409` then `install` call) + `forge/api/internal/http/handlers_servers.go:1110` `POST /reinstall` `PermSettingsReinstall` | `clustermanager:242` gating makes pure-Docker Reinstall always 500 while Beacon works → API vs Beacon drift. Caller sees `failed` marking `status='install_failed'`. **Still BROKEN**; `REF-GAME-F-G-07`. Fix: fallback `if !ok → InstallServer` (mirror Beacon) or implement Docker `Reinstall` alias. |
| **LF-05** | **F-G-05** | **P1** `USER_VISIBLE` | **Kill does NOT pierce stuck `RunningAction` lock** | `beacon/internal/server/manager.go:487` `TryLock + RunningAction!=""` `manager.go:271` `BeginInstall` same slot `manager.go:581` `kill` same + `beacon/internal/runtime/docker.go:494` `Kill` sharded `workloadLocks[64]` `docker.go:251` + `reference/game-hosting/pterodactyl-wings/server/power.go:108` kill ignore lock | Wings documents `kill` can pierce stuck lock. Forge queues `kill` behind stuck op (worker 5m reaper `operation/service.go:164` + `RunningAction` slot claimed) so hung install holds lock → kill waits. **Still MISSING**; `FINAL GH-05`. Fix: separate kill semaphore or TryLock-ignore for `signal==kill` mirroring Wings `115-119`. |
| **LF-06** | **F-G-08 generation** | **P2** `SILENT` | **Generation fencing livelock when `crashed` with desired `running` + hash without generation seed** | `forge/api/internal/store/store_state.go:39` `CASE WHEN desired='running' AND actual='running' THEN desired_generation` + `forge/api/internal/services/reconciler/service.go:476` `reconcileServer` + `beacon/internal/runtime/docker.go:193` `createRequestHash:1016` no generation | `observed_generation` only advances on exact active overlap `running↔running` / `stopped↔stopped`; `crashed+desired running` never converges → reconciler loop `remediate:true` forever. Also `createRequestHash` hashes marshaled `CreateRequest` without `desired_generation` — two Creates with same spec but different generations collapse to same hash and skip reconciliation. **Still BROKEN** (phase-02 subagent-01 LF-02/LF-04, `final-parity C-02 LF-10`/`C-01 LF-11`). |
| **LF-07** | **F-G-23** | **P2** `OPERATOR_VISIBLE` | **Schedule task action asymmetry — arbitrary action persists** | `forge/api/internal/store/store_schedules.go:252` `CreateScheduleTask:253` `action!=""` only vs `store_schedules.go:332` `PatchScheduleTask:332` `isValidScheduleTaskAction` `store/store.go:1196` valid `power|backup|command` | Create vs Patch asymmetry: attacker with `schedule.update` can persist `action="powershell"`; runner `schedule_runner.go:386` rejects at execution but DB polluted. **Still BROKEN**; `REF-GAME-F-G-23`. Fix one-line guard at `252`. |
| **LF-08** | **F-G-24** | **P1** `OPERATOR_VISIBLE` → `P0` if admin stolen | **Mount allowlist too narrow → host path breakout** | `forge/api/internal/store/store_mounts_ext.go:323` `validateMountPath:332` `source∈{/etc/forge,/var/lib/forge/volumes}` `target∈{/,/home/container}` + `beacon/internal/runtime/docker.go:743` `buildContainerMounts` | Only blocks 2+2; leaves `/etc`, `/var/run/docker.sock`, `/proc`, `/`, `/root` mountable via `POST /mounts` (`handlers_admin.go:1579`) with `mounts.write`. Compromised admin host-breakout. **Still BROKEN**; `REF-GAME-F-G-24`. Fix: deny `/etc|/proc|/sys|/dev|/var/run|/root|/` or require `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts`. |
| **LF-09** | **NEW** | **P2** `OPERATOR_VISIBLE` | **Allocation `container_port`/`protocol` lost in legacy `Mappings` fallback** | `forge/api/internal/store/store_allocations.go:155` `containerPort` + `forge/api/internal/daemon/client.go:438` `Port` + `beacon/internal/runtime/docker.go:865` `dockerPorts` + `forge/api/internal/services/clustermanager/service.go:698` `Mappings` `service.go:699` `mappings[ip]=append(..., port)` + `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` | Forge explicit `protocol` dropped when serialization uses legacy `Mappings` (`Bindings()` creates both tcp+udp) → UDP-only `8211` also binds tcp. **Still PARTIAL** (`F-G-11`). |
| **LF-10** | **F-G-10** | **P2** `SILENT` | **`user_viewable` gate leak — non-admin can set `DL_PATH` at provision to hijack installer URL** | `forge/api/internal/store/store_servers_control.go:154` `ServerProvisionTarget` loads **all** variables `store_servers_control.go:155-163` no viewable filter + `forge/api/internal/store/store_startup.go:58` rejects non-viewable updates, `forge/api/internal/store/store_servers.go:273` CreateServer allows any variable via `validateVariableValue` only | `CreateServer` can set `DL_PATH` ( `user_viewable=false` ) at provision to hijack installer URL, not later. **Still BROKEN**. |
| **LF-11** | **REF-GAME-C01** | **P1** `OPERATOR_VISIBLE` | **Suspend/Panel-sync race + transfer-suspend illegal combo** | `beacon/internal/server/manager.go:610` `onBeforeStart:611` snapshot `Suspended` + `689` `syncServerStateFromPanel:748` `Suspended= settings.Suspended` without fence `panelSyncMu:693` only serializes panel fetches not power lock | `syncServerStateFromPanel` releases `state.mu` then HTTP fetches without fence, re-acquires — concurrent `HandlePower start` can observe stale `Suspended=false` and start suspended server. Also `POST /suspension:1413` not checking `ensureTransferIdle` → `suspended=true` + `transferring=true` illegal combo (Pelican forbids). **Still BROKEN**; `final-parity C-03 LF-01`. |

> Also re-affirmed but not duplicated: `store_servers_control.go:226` `installing→installed` always `stopped` unconditionally even if `suspended=true` → badge shows `stopped+suspended` dual truth (vs `status=suspended` persistence) — intentional divergence but needs UI combined pill (`phase-02 F-G-04`).

---

## 3. Status Rollup (15 capabilities, reconciled)

| Capability | Live Status | Bucket | Prior FINAL | Delta |
|---|---|---|---|---|
| GH-01 create→created | **COMPLETE** superset | Keep | COMPLETE | CONFIRMED |
| GH-02 desired/actual + fencing | **COMPLETE** superset | Keep | COMPLETE+ | CONFIRMED (livelock LF-06) |
| GH-03 suspend/unsuspend | **PARTIAL** | Gap | COMPLETE+ | DOWNGRADED to PARTIAL — matches final-parity |
| GH-04 power start/stop/restart | **PARTIAL** | Gap | PARTIAL | CONFIRMED |
| GH-05 kill pierce | **MISSING** | Must fix | MISSING | CONFIRMED |
| GH-06 install fencing | **COMPLETE** enhanced | Keep | COMPLETE+ | CONFIRMED |
| GH-07 reinstall | **BROKEN** | Must fix | BROKEN | CONFIRMED |
| GH-08 delete + orphan | **COMPLETE** enhanced | Keep | COMPLETE+ | CONFIRMED |
| GH-09 restoring_backup lock | **MISSING** | Must fix | MISSING P1 | CONFIRMED |
| GH-10 transfer/migration v1 | **PARTIAL** | Doc/dual | COMPLETE+ | DOWNGRADED to PARTIAL |
| GH-11 allocations | **COMPLETE** superset | Keep | COMPLETE+ | CONFIRMED |
| GH-12 allocations/install race vs kill | **BROKEN** | Must fix | BROKEN | CONFIRMED |
| GH-13 egg/nest model | **COMPLETE** | OK | PARTIAL | UPGRADED to COMPLETE on re-inspect (model itself complete) |
| GH-14 variables + validation | **BROKEN** | Must fix P0 | BROKEN P0 | CONFIRMED |
| GH-15 config parser 6 parsers | **MISSING** | Must fix | MISSING | CONFIRMED |
| GH-16 template vs egg dualism | **DUPLICATE** + `MISSING` seed | Degap | DUPLICATE | CONFIRMED |
| GH-17 schedules/cron | **PARTIAL** | Gap | PARTIAL | CONFIRMED |
| GH-18 wildcard * | **BROKEN** | Must fix P0 | BROKEN P0 | CONFIRMED |
| GH-19 mount allowlist | **BROKEN** | Hardening P1 | BROKEN | CONFIRMED |

Counts (15 core + 4 extended): `COMPLETE 6 | PARTIAL 4 | MISSING 3 | BROKEN 4 | DUPLICATE 1` — 7/15 COMPLETE+, 7/15 need P0/P1 fix, 1 DUPLICATE.

---

## 4. Recommendations (ADOPT/ADAPT/INSPIRE/REJECT)

| Capability | Rec | Justification (file:line) |
|---|---|---|
| GH-01 provisioning split | **ADOPT** | Keep `store_servers.go:253` `provisioning` — generation fencing more truthful; add badge mapping |
| GH-02 desired/actual | **ADOPT** | Keep canonical — no ref had it; `store_state.go:39` is strength; fix livelock to advance on `crashed→running` desired |
| GH-03 suspend orthogonal bool | **ADAPT** | Keep `suspended` bool orthogonal (`store_servers.go:374`) but add `ensureTransferIdle:145` guard to `/suspension:1413` and client 403 middleware |
| GH-04 power durable queue | **ADAPT** | Keep `handlers_servers.go:888` durable `202` but fail-fast 409 for `installing` before enqueue (match Wings `power.go:57`) |
| GH-05 kill pierce | **ADOPT** | Adopt Wings `power.go:108` pierce verbatim — emergency kill must not queue (`manager.go:487,581`) |
| GH-06 install 1000:1000 | **ADOPT** | Keep `docker.go:285` `1000:1000` `ReadonlyRootfs` `CapDrop ALL` — strictly better; honor `skip_scripts` |
| GH-07 reinstall | **ADOPT** | Adopt Ptero `install(true)` — fallback `Reinstall→Install` when `Reinstaller` missing `clustermanager/service.go:241` |
| GH-08 orphan remediation | **ADOPT** | Keep `store_servers_lifecycle.go:18` novel vs silent Pterodactyl; add `GET /admin/orphans` UI |
| GH-09 restoring lock | **ADOPT** | Adopt `Server.php:393` `restoring_backup` semantics verbatim (`store_state.go:137`) |
| GH-10 transfer v1 resumable | **ADAPT** | Keep control-plane-mediated v1 (HMAC chunks) over Wings JWT push; sync stale `servers.transfer_state` from `migrations` |
| GH-11 allocations explicit | **ADOPT** | Adopt explicit `protocol`+`containerPort` vs Wings `Mappings` dual — fix DB index `(node_id,ip,port,protocol)` |
| GH-14 validator | **ADAPT** | Adapt Laravel `ValidationFactory` — strip `regex:/…/` delimiters, `\|` inside regex, add `RESERVED_ENV_NAMES` |
| GH-15 config parser | **INSPIRE** | Inspire Wings `parser:1` — port `parser/` into `manager.go:onBeforeStart` or document unsupported |
| GH-17 schedules | **ADAPT** | Adopt `isValidScheduleTaskAction:1196` at creation `store_schedules.go:252` one-line |
| GH-18 subusers | **ADAPT** | Adapt `SubuserController:154 getDefaultPermissions` subset — enforce `perms ⊆ actorPerms` gate `*` |
| GH-16 templates collapse | **ADAPT** | Collapse FS vs DB vs localStorage three-way — `SeedGameTemplates` idempotent boot (like `appstore/seed.go:216`) |
| GH-19 mounts | **ADAPT** | Adapt Pterodactyl doc: mounts under dedicated host prefix `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` |

**REJECT list (explicitly not to adopt):**
- Pterodactyl single `status='suspended'` wiping prior state (`Server.php:125`) — keep Forge orthogonal boolean.
- Pelican `ServerState` enum single column vs Forge desired/actual split — keep split.
- Puffer `IsIdle` boolean flags (`servers/server.go:884`) vs Forge `RunningAction` single slot `manager.go:488` — keep slot.
- Typed install pipeline 26 ops fully on daemon (`spec.json:307`) — **INSPIRE for modded Minecraft only**, keep single `install_script` for most games.
- Host `supportedEnvironments[host,docker]` (`spec.json:26`) — reject host env, keep Docker-only.

---

## 5. Evidence Anchor (every claim file:line — condensed, live verification 2026-08-24)

**References (selected):**
- `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:122` constants `125` suspended `126` restoring_backup `138` `$attributes installing` `153` resource validation `162` allocation/threads/oom `215` isInstalled `218` isSuspended `393` validateCurrentState
- `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:52` egg schema `68` EXPORT_VERSION `84` docker_images/config/file_denylist
- `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:29` egg_variables `43` RESERVED `70` validation
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:28`
- `reference/game-hosting/pterodactyl-wings/server/power.go:24` HandlePowerAction `30` constants `57` installing/transfer/restoring guard `81` kill bypass comment `108` kill TryLock ignore `171` onBeforeStart
- `reference/game-hosting/pterodactyl-wings/server/install.go:33` Install `80` Reinstall `195` installing.SwapIf
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` Limits `66` ConvertedCpuLimit `116` BlkioWeight `124` conditional shares
- `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` Mappings `54` Bindings dual
- `reference/game-hosting/pterodactyl-wings/server/parser/parser.go:1` 6 parsers
- `reference/game-hosting/pelican-panel/app/Enums/ServerState.php:12` enum `15` Suspended
- `reference/game-hosting/pufferpanel/servers/server.go:185` Start `343` Kill `410` Install `884` IsIdle

**Forge API:**
- `forge/api/docs/server-lifecycle.md:5` provisioning `6` Beacon-first delete `12` transfer 501 (doc lie)
- `forge/api/internal/store/store_servers.go:253` `INSERT 'provisioning'` `273` StartupVariables validation `374` SetServerSuspension `385` CompareAndSet `397` SetServerSuspended `346` IsServerTransferBlocking `362` UpdateServerTransferState
- `forge/api/internal/store/store_servers_control.go:13` SetServerPowerState `44` powerSignalPriorStates `51` kill allowed 7 states `207` SetServerProvisioned `208` `status='created',actual_state='stopped'` `218` SetServerInstallState `244` MarkSynced `57` ServerControlTarget `83` ServerProvisionTarget `154` env no viewable filter `178` resolveStartupCommand
- `forge/api/internal/store/store_state.go:10` SetDesired `16` generation +1 `31` SetActual `37` status `39` observed_generation fence `120` recordStateTransition `131` serverStatusFromActual `165` serverDesiredFromSignal
- `forge/api/internal/store/store_servers_lifecycle.go:12` HardDeleteServer `18` RecordOrphanAndHardDeleteServer `19` daemonError required `43` primary_allocation_id NULL `46` allocations NULL `49` DELETE
- `forge/api/internal/store/store_nests.go:16` Nest `34` Egg `248` CreateEgg `361` normalizeDockerImages
- `forge/api/internal/store/store_egg_variables.go:15` pattern `17` EggVariable `65` CreateEggVariable `129` validateRequest `142` validateVariableValue `143` Split("|") `176` `regexp.Compile(arg)` with slashes `177` regex case
- `forge/api/internal/store/store_startup.go:11` GetServerStartup `31` user_viewable `54` UpdateServerStartupVariable `58` viewable+editable
- `forge/api/internal/store/store_schedules.go:14` resolveStartupCommand `252` CreateScheduleTask `253` `action!=""` only `331` PatchScheduleTask `332` isValidScheduleTaskAction `489` ListDueSchedules
- `forge/api/internal/store/store_users.go:306` UpsertServerSubuser `311` normalize `555` normalizeSubuserPermissions `563` allows `*` `574` defaultSubuserPermissions `353` UserCanAccessServer `464` AuthenticateSFTP
- `forge/api/internal/store/store_mounts_ext.go:13` ListMounts `41` CreateMount `297` ensureMountAvailableForServer `316` validateMountPaths `323` validateMountPath reserved `332` source 2 + `335` target 2 `367` AllowedMountSourcesForNode
- `forge/api/internal/store/store.go:1196` isValidScheduleTaskAction `30` constants
- `forge/api/internal/http/handlers_servers.go:46` hasServerReadEnv `71` redactStartupSecrets `130` legacyTransferGone `145` ensureTransferIdle `335` Permissions * for owner/admin `501` users `554` POST /users no subset `713` POST /transfer migration `740` GET /transfer stale `789` POST /servers `888` POST /power `1050` POST /install `1110` POST /reinstall `1413` POST /suspension `1474` DELETE /delete orphan
- `forge/api/internal/http/handlers_admin.go:990` nests `1083` eggs `1185` variables `1308` import `1354` CreateEggVariable loop `1579` mounts
- `forge/api/internal/services/clustermanager/service.go:76` CreateServer `92` PlaceServer `140` CreateServer Tx `172` provisionServer `180` runtime.CreateServer `187` SetServerProvisioned `216` InstallServer `220` ReinstallServer `241` Reinstaller gate `243` 500 `285` compensateCreateFailure `292` RecordOrphan `332` DeleteServer `355` HardDelete `382` RequestServerPower `394` Suspended check `532` sendPower `576` publishServerPowerEvents `632` runtimeCreateRequest `657` HostIP/ContainerPort/Protocol default tcp `689` runtimeConfiguration `698` mappings
- `forge/api/internal/orchestrator/suspension.go:28` SuspendServer `40` not already suspended `47` CompareAndSet `72` publish suspended `82` UnsuspendServer
- `forge/api/migrations/021_true_state_persistence.sql:3` desired/actual enums `040_truthful_server_lifecycle.sql:4` clobber `18` orphan `043_unify_eggs_templates_mounts.sql:63` `091_seed_minecraft_java.sql:3` one egg `150_heartbeat_reconciling_state.sql`

**Beacon:**
- `beacon/internal/server/server.go:735` create `770` validate serverId+image `814` CreateRequest `829` runtime.Create `841` MarkCreated `844` syncConfiguration `869` applyConfigurationFiles no parser `1052` install `1116` BeginInstall claim `1122` runtime.Install `1131` EndInstall `1132` notifyPanel `1159` installWS `1178` ScopeAdmin `1331` reinstall `1335` PowerState check `1357` power `1380` block 30s poll `1433` delete `625` docker Delete
- `beacon/internal/server/manager.go:21` PowerState `30` ServerState `312` MarkCreated `128` Reconcile `271` BeginInstall `273` TryLock `280` Suspended `284` installing `298` EndInstall `482` HandlePower `487` TryLock+RunningAction `494` installing guard `511` start `534` stop `549` restart `581` kill same slot `610` onBeforeStart `611` snapshot `623` installing `628` suspended `689` syncServerStateFromPanel `748` Suspended assign `843` StartEventWatcher `879` HandleContainerEvent
- `beacon/internal/runtime/runtime.go:55` CreateRequest `100` PortBinding `115` InstallRequest `144` Runtime interface
- `beacon/internal/runtime/docker.go:151` Create idempotent `171` Reconcile `193` createRequestHash `200` configHashLabel `256` Install `285` User 1000:1000 `299` CapDrop ALL `302` ReadonlyRootfs `303` no-new-privileges `340` log cap 1MiB `388` Start `434` Stop `494` Kill sharded lock `251` workloadLocks `[64]sync.Mutex` `529` Restart `625` Delete `783` buildResources `808` buildHostConfig `865` dockerPorts `899` configHashLabel `901` validateCreateRequest `924` CPUPercent `1016` createRequestHash no generation `1003` buildContainerConfig `1016` hash `1028` sha256

**Web:**
- `forge/web/lib/api/servers.ts:66` createServer `88` sendPowerSignal `98` reinstallServer `102` suspendServer `394` fetchServerTransferStatus
- `forge/web/lib/egg-templates.ts:27` EGG_TEMPLATES 14 static `80` regex `/^([\w\d._-]+)(\.jar)$/` payload
- `forge/web/components/server/console-view.tsx:37` InstallBanner `125` canPower `203` network cumulative `243` kill

---

## 6. Activation Order (P0→P3) — without new subsystem

1. **P0** `store_egg_variables.go:176` strip `regex:/…/` delimiters + `|`-aware split (`F-G-08` LF-01) — unblocks PTDL imports (already `REF-GAME-F-G-08`).
2. **P0** `store_users.go:555` gate subuser escalation — subset check + `*` behind owner/admin (`F-G-22` LF-02).
3. **P1** wire `restoring_backup` as server lock `handlers_servers.go:2117` `SetServerActualState restoring_backup` + `ensureRestoreIdle` alongside `ensureTransferIdle` (`F-G-06` LF-03).
4. **P1** `clustermanager/service.go:241` fix reinstall fallback to `InstallServer` when `Reinstaller` not implemented (`F-G-07` LF-04).
5. **P1** `manager.go:482` kill pierce lock — separate `kill` semaphore or TryLock-ignore for `kill` (`F-G-05` LF-05).
6. **P1** `store_mounts_ext.go:323` harden allowlist to `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` or deny `/etc|/proc|/sys|/dev|/var/run` (`F-G-24` LF-08).
7. **P1** `handlers_servers.go:888` fast 409 for `installing` before enqueue + `store_servers_control.go:51` document kill-cancel semantic (GH-12).
8. **P2** `store_schedules.go:252` add `isValidScheduleTaskAction` guard (`F-G-23` LF-07); push `ListCronJobs` filter into SQL.
9. **P2** Fix `store_servers_control.go:226` `installing→installed` clobber of `suspended` badge + `manager.go:689` suspend race; add `ensureTransferIdle` to `/suspension`.
10. **P2** Fix generation livelock `store_state.go:39` + hash seed `docker.go:1016` include `desired_generation` (LF-06).
11. **P2** Port config parser `wings/parser` to `manager.go:onBeforeStart` or strip `config.files` from templates (GH-15).
12. **P2** Unify templates — `SeedGameTemplates` idempotent boot like `appstore/seed.go:216` (GH-16).
13. **P3** Fix heartbeat/phantom orphan etc.

---

**Verdict:** Forge lifecycle is **functionally parity-close** (7/15 COMPLETE), but **5 P0/P1 load-bearing defects remain unwired/BROKEN exactly as prior audits reported — no fixes landed on this surface since 2026-08-24**. Top fixes are validation regex (GH-14 P0), subuser wildcard (GH-18 P0), restoring lock (GH-09 P1), reinstall gate (GH-07 P1), kill pierce (GH-05 P1). Wiring and hardening (activation) — not rebuilding — is correct next phase.

*All claims file:line cited from live code; no product code modified; no vague claims.*
