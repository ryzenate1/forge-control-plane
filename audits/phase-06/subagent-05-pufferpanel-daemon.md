# Subagent 05 — PufferPanel Daemon & Server Operations vs Forge Beacon + Server Lifecycle

**Date:** 2026-08-23
**Scope:** `reference/game-hosting/pufferpanel` daemon/server logic, operations (panel vs daemon), server states, permissions — compared to Forge Beacon (`beacon/internal/server/*`, `beacon/internal/runtime/docker.go`) and Forge panel (`forge/api/internal/services/clustermanager`, `forge/api/internal/store/store_servers*.go`, `forge/api/internal/http/handlers_servers.go`, `forge/api/docs/server-lifecycle.md`)
**Previously excluded in Phase 2 Subagent-03 as "architecturally unrelated" — re-included for deep compare**

---

## 1. Executive Summary

PufferPanel and Forge/Beacon solve the same problem — lifecycle a fleet of game servers on Docker — with divergent architectures. PufferPanel is a **monolithic daemon** that embeds panel API, server definitions, and Docker/host runtime in one Go process. Forge is a **split control/data plane**: Forge panel owns placement, allocations, and desired state in Postgres; Beacon agents are thin, stateless (but state-persisting) workload executors that receive imperative commands over a reconciled `runtime` abstraction.

The split gives Forge stronger correctness (transactional placement + reservation, explicit orphan remediation, idempotent create, circuit-broken crash restart, per-workload persisted power state) at the cost of a more elaborate provisioning handshake. PufferPanel is simpler to run single-node, richer in declarative install primitives (22 operation types with CEL-gated pre/post hooks + cron scheduler), but weaker on isolation, concurrency control, and container reconcilability.

Neither is a superset; Forge intentionally defers PufferPanel's broad mod-loader download catalog and KeepAlive/tty proxy features.

---

## 2. Methodology

- Inspected PufferPanel daemon roots: `server.go`, `environment.go`, `environmentfactory.go`, `operation.go`, `servers/server.go`, `servers/server_loader.go`, `servers/env_loader.go`, `servers/operation_process.go`, `servers/operation_functions.go`, `servers/docker/*`, `servers/tty/*`, `servers/scheduler.go`, `operations/*`, `scopes/scopes.go`, `config/entries.go`, `files/`.
- Inspected Beacon: `beacon/internal/server/server.go`, `manager.go`, `console.go`, `queue.go`, `secure_files.go`, `hostfiles.go`, `beacon/internal/runtime/{runtime,docker}.go`.
- Inspected Forge panel: `forge/api/docs/server-lifecycle.md`, `forge/api/internal/services/clustermanager/service.go`, `forge/api/internal/store/store_servers*.go`, `forge/api/internal/http/handlers_servers.go`.
- All citations are `file_path:line_number`. Line numbers verified by `Read`.

---

## 3. Comparison Matrix (16 comparisons — requirement >=12)

| # | Dimension | PufferPanel | Forge / Beacon | Verdict |
|---|-----------|-------------|----------------|---------|
| 1 | **Control/data-plane split** | Single binary serves panel API + daemon; `servers/server.go:59-64 InitService/processQueue/processStats` run in same process. Panel vs daemon is a config flag (`config/entries.go:42 DaemonEnabled/12 PanelEnabled`), not a deployment boundary. | Strict split: Forge panel (`forge/api/internal/services/clustermanager/service.go:76 CreateServer`) owns scheduling + Postgres; Beacon (`beacon/internal/server/server.go:223 NewServer`, `240 NewServerWithBackup`) is an agent that exposes `POST /servers`, `/power`, `/install` over mTLS/HMAC. Panel never touches Docker directly. | PufferPanel simpler single-node; Forge scales horizontally and survives panel restarts without stopping workloads. |
| 2 | **Server definition / template model** | JSON file per server (`servers/server_loader.go:52 Load`, `143 Save`) + embedded variables (`server.go:8-23 Server` with `Variables map[string]Variable`, `Installation []ConditionalMetadataType`, `Execution Execution`). Templates are the same JSON shape, copied via `CopyFrom` (`server.go:67`). Groups/variables drive UI generation. | Egg-centric: `store.CreateServer` (`forge/api/internal/store/store_servers.go:180-306`) inserts a `servers` row referencing `eggs` + `egg_variables`; overrides stored in `server_variables`. Beacon receives a **rendered** `CreateRequest` (`beacon/internal/runtime/runtime.go:55-82`) or a canonical `ServerConfiguration` payload (`clustermanager/service.go:689 runtimeConfiguration`). No raw template file on the agent. | PufferPanel templates are file-driven and self-contained; Forge templates are relational and versioned in DB, enabling `config_sync_pending` tracking but requiring `MarkServerConfigSynced` round-trip. |
| 3 | **Runtime abstraction** | `EnvironmentImpl` interface (`environment.go:17-31` ExecuteAsyncImpl/KillImpl/GetStatsImpl/SendCodeImpl/GetUidImpl/IsRunningImpl) with two implementations: `servers/docker/docker.go:35 Docker` and `servers/tty/tty.go` (host). Selected by `environmentType` string (`servers/env_loader.go:14-21`). | `Runtime` interface (`beacon/internal/runtime/runtime.go:144-163` + `Reconciler:171`) with **5 providers** (`runtime.go:10-16` docker/containerd/podman/firecracker/kubernetes) behind `factory.go`. Docker is one provider, not the only one. | Forge abstraction is wider (multi-runtime) and symmetric (`Create`/`Reconcile`/`Install`/`Inspect`), while PufferPanel's env split is Docker-vs-host only. |
| 4 | **Server lifecycle state machine** | Flags + process lock: `servers/server.go:28-44 Server` stores `CrashCounter`, `isUnsafeRunning *sync.Mutex`, `Installing bool` (`environment.go:45`), `BackingUp/Restoring`. `IsIdle()` (`servers/server.go:884-899`) blocks all mutating ops if running/backingUp/installing. Auto-restart flags on `Execution` (`server.go:34-36 AutoStart/AutoRestartFromCrash/AutoRestartFromGraceful`). | Explicit FSM: `beacon/internal/server/manager.go:21-28 PowerState` (Offline/Starting/Running/Stopping) + `ServerState:30-65` fields `PowerState`, `InstallationState`, `StartupState`, `RunningAction`, `ExpectedStop`, `CrashCount`, etc. Panel adds `desired_state/actual_state` (`store_servers.go:310-312`) and `status` (`provisioning/created/installing/installed/stopped/running/install_failed`) — see `store_servers_control.go:207-242`. Single `RunningAction` string enforces at-most-one action. | Forge state is richer and dual-layer (panel desired vs agent actual + reconciliation); PufferPanel's boolean flags are simpler but cannot represent `install_failed` or desired/actual drift. |
| 5 | **Startup path: pre/post hooks & command selection** | `servers/server.go:205-213 GenerateProcess(PreExecution)` → `ExecuteAsync(ExecutionData{Callback:afterExit})` → `562 afterExit` posts `GenerateProcess(PostExecution)`. Command can be a single string or a CEL-gated list (`222-264` iterates `Execution.Command []Command` picking first `If`-passing entry, with default fallback). Variables token-replaced via `utils.ReplaceTokens`. | No pre/post hook arrays. Single `CreateRequest.Command` (`runtime.go:58`) rendered by Forge (`clustermanager/service.go:650 command:= []string{"/bin/sh","-lc", StartupCommand}`) and passed to `buildContainerConfig` (`beacon/internal/runtime/docker.go:1003`). Post-exit handling is event-driven (`manager.go:879 HandleContainerEvent`) not templated. | PufferPanel declarative lifecycle hooks are more expressive (per-template file tweaks, env-specific setup); Beacon pushes that richness into Beacon's install script or Forge's egg, not per-start. |
| 6 | **Installation subsystem** | Declarative: `Installation []ConditionalMetadataType` executed as an `OperationProcess` (`servers/server.go:445-461` via `GenerateProcess`). 22 factories (`servers/operation_process.go:36-61`) cover `mojangdl/paperdl/fabricdl/forgedl/neoforgedl/spongedl/javadl/nodejsdl/steamgamedl/curseforge/download/extract/archive/mkdir/move/writefile/alterfile/dockerpull/...` with `OperationFactory.Create` (`operation.go:7-12`). Supports `VariableOverrides` (`operation.go:24-27`) to feed outputs back into `DataMap`. | Imperative script-in-container: `beacon/internal/runtime/docker.go:256-345 Install` creates `${ServerID}-installer` bind-mounting `RootDir→/mnt/server`, running installer as `1000:1000` with `ReadonlyRootfs:true`, `CapDrop:ALL`, `no-new-privileges:true` (`docker.go:304-306`). Forge supplies `Image/Entrypoint/Script/Env` from `egg.install_*` (`clustermanager/service.go:681 runtimeInstallRequest`). Result is `InstallResult{ExitCode,Logs}` installed atomically under `serverWs.install` (`server.go:1052-1157`). | PufferPanel's operation catalog is vastly broader (mod-loader specific) but runs unsandboxed on the daemon host unless Docker env; Beacon isolates installs in hardened containers — stronger containment, fewer built-in primitives (download/extraction done by the install script). |
| 7 | **Operation execution model & conditionals** | Generic `OperationFactory` plugin per `type` key (`servers/operation_process.go:63-67 commandMapping` + `150-156 factory.Create`). Each step CEL-gated (`142-148 RunCondition`). Failure short-circuits the process (`162-172 return result.Error`). TODO on line `165` notes success tracking is incomplete. | No factory/steps on the agent. Operations are **power or install** only, dispatched through a bounded `OperationQueue` (`beacon/internal/server/queue.go:18-26 OpStart/Stop/Restart/Kill/Install/Reinstall`, `63 OperationQueue` with `concurrency 1-4`). TTL/expiry (`ExpireExpired:339`), SQLite persistence (`NewPersistentOperationQueue:82`), idempotent `CommandID` dedupe (`364-377`), `Acknowledged/Progress/ResultData` fields. | PufferPanel is a workflow engine (steps, conditions, variable outputs); Beacon's queue is a durable, idempotent command journal for the handful of signals Forge actually needs. |
| 8 | **Crash / auto-restart & KeepAlive** | `servers/server.go:568-601 afterExit`: `graceful := exitCode==ExpectedExitCode`; crash increments `CrashCounter++` and `StartViaService(p)` if `< config.CrashLimit` (`config/entries.go:55` default 3). Graceful path restarts only if `AutoRestartFromGraceful` true. Separate **KeepAlive** ticker (`server.go:289-313`, `562-566` stop) that `ExecuteInMainProcess(KeepAlive.Command)` every `KeepAlive.Frequency`. Global `processStats/processQueue` ticker 1s/5s (` servers/server.go:92-112`). | `beacon/internal/server/manager.go:879-955 HandleContainerEvent` implements: `DetectCleanExitAsCrash` flag (`52`), `CrashCooldown` (`47`, default 1m `93`), `crashLoopWindow` 30m (`100`) + `maxConsecutiveCrashRestarts` 5 (`97`) circuit breaker, `crashHandler` callback, and `persistPowerState` (`387-428`) so restarts survive daemon restarts. `autoRestartCrashed` (`177-206`) with `crashAutoRestartWindow` 24h (`211`) after boot. `CrashCount` **not** reset on `start` by design (`525`). No KeepAlive — liveness via stats stream / `WaitForStop`. | PufferPanel KeepAlive is a unique self-healing ping (absent in Beacon); Forge crash logic is more sophisticated — persisted across restarts, with cooldown + breaker, and explicit clean-exit/oom semantics. |
| 9 | **Scheduling (cron tasks)** | Per-server `Scheduler` (`servers/scheduler.go:15-22` with `Tasks map[string]Task`, `15-52 Init/Save`) persisted as `${id}.cron` JSON (`27-33 LoadScheduler`). Backed by `gocron` (`67-113 Init` with location, concurrency limit, reschedule/wait mode). Task = `pufferpanel/task.go:3-8` (`Name,CronSchedule,Operations []ConditionalMetadataType`). Executed via `_executeTask` (`178-205`) → `GenerateProcess`+`Run`, logging to console buffer. | No agent scheduler. Scheduling is Forge-side (`forge/api/internal/http/handlers_servers.go:1152+ /servers/:id/schedules`CRUD` mapped to `store.CreateSchedule` etc., run via `scheduleRunner`). Beacon only executes commands Forge dispatches; schedule evaluation never lives on the agent. | PufferPanel schedules run locally even if panel is down; Forge schedules are panel-centralized, enabling RBAC + audit on runs but adding panel availability dependency. |
| 10 | **Permissions / access control** | OAuth2 scopes (`scopes/scopes.go:11-64`): `nodes.view/create/edit/delete/deploy`, `server.create`, per-server `server.start/stop/kill/install/files.edit/console/...`, plus `ScopeServerAdmin` inheritance (`134-152 ContainsScope` auto-grants admin → all server scopes). Server-user assignments stored per-server; `EditData` (`servers/server.go:522-539`) gates on `UserEditable` flag. | Dual layer: (a) global `requireAdminScope`/`requireRole("admin")` on panel API (`handlers_servers.go:165-341`, `789 POST /servers` etc.), (b) per-server RBAC via `UserCanAccessServer` + fine-grained `store.Perm*` (`PermControlStart/Stop/Restart`, `PermSettingsReinstall`, `PermAllocation*`, `PermUser*`, `PermSchedule*`) and `server:subusers` table (`store_servers.go`). Beacon trusts **panel-issued bearer/HMAC** plus JWT `ScopeAdmin` vs `ScopeServer` enforcement (`beacon/internal/server/server.go:1178-1181 installWS rejects non-admin scopes`). | PufferPanel scopes are coarse but cover many server actions; Forge permissions are more granular per CRUD axis and enforced at DB layer, with redaction (`handlers_servers.go:52-85 redactStartupSecrets`) for secrets. |
| 11 | **File / backup / transfer subsystems** | `DaemonServer` iface (`server.go:91-99` + `servers/server.go:604-829`): `GetItem`, `ArchiveItems` (archiver), `Extract`, `StartBackup/StartRestore` async with `chan bool` + `DisplayToConsole`. Configurable `BackupsFolder` per-server (`config/entries.go:53`). Binaries folder bind-mounted (`servers/docker/docker.go:487-497`). Global `files.Compress/Extract` with glob and Host/Container path translation (`servers/docker/container_mount_source.go:20-118` host path discovery). | `beacon/internal/server/secure_files.go:60 serverFilesystem` returns a `rootfs.FS` confined under `dataDir/serverID`; archive limits (`27-32 maxFileWriteBytes=16MB, archive 4GB, 100k entries`), staged extraction with rollback (`269-480 extractZipStaged/extractTarStaged/commitStaging`), `HasSpaceForWriteFS` (`manager.go:784-812`), chunked upload `.uploads/*.part` + reaper (`80-147 startUploadCleanupLoop`). Backups via `backup.BackupInterface` S3/local (`server.go:224-230 NewServerWithBackup`). Transfers are **protocol v1 HMAC-scoped credentials** (`server.go:373-398` legacy removal comment). | PufferPanel backup is a direct `tar.gz` under a flat folder with no quota/staging/rollback; Beacon layers isolation, validated archives, quotas, staging+rollback, and a dedicated transfer protocol — markedly more defensive. |
| 12 | **Console / stats** | `Environment` wraps `MemoryCache` (`environment.go:37`) + `Tracker` broadcasters (`42-44`) + `Wrapper io.Writer` (`41`, `211-218 CreateWrapper` multiwrites to stdout/buffer/tracker). `CreateConsoleStdinProxy` creates Telnet/RCON/WS proxies (`95-119`). Docker attach copies to `Wrapper` (`servers/docker/docker.go:98-106`). Stats via `ContainerStats` every 5s (`servers/docker/docker.go:187-267`) cached (`191-194` 5s throttle) with JVM special case via `jcmd` exec (`226-261`). | `consoleManager` (`beacon/internal/server/console.go:62-84` per-server `consoleProducer` with replay ring `128 entries /256KB`, bounded fan-out, drop-if-slow `149-161`). `events.Bus` publish (`server.go:265 eventBus`). Stats via `runtime.Stats` per-demand + `StatsStream` and `/metrics` Prometheus families (`server.go:674-733`) plus `StatsStream`/`LogsStream` websocket. No proxy stdin — `SendCommand` writes directly (`beacon/internal/runtime/docker.go:405-432`). | PufferPanel multiplexes stdout to both cache and optional stdout forward plus protocol-specific stdin; Beacon uses an isolated replay-buffered producer per server — better fan-out isolation but no built-in RCON/telnet gateway. |
| 13 | **Create idempotency & reconciliation** | `servers/docker/docker.go:74-76 doesContainerExist` → hard error `docker container already exists` if present; no reconcile. `Create` path (`servers/server_loader.go:120-161`) does `os.Mkdir + Save + Load + CreateEnvironment` with deferred cleanup on error. `Reload` (`228-261`) re-loads JSON, swaps `RunningEnvironment/Server`, restarts scheduler — but does not remove/recreate a running Docker container atomically. | `beacon/internal/runtime/docker.go:151-166 Create` idempotent (returns nil if `Inspect.Exists`), `171-249 Reconcile` hashes request (`198-201 createRequestHash`), preserves `restartAfterCreate` flag, stop→remove→create→restart if needed, with cleanup context `context.WithoutCancel` fallback. Forge `compensateCreateFailure` (`clustermanager/service.go:285-302`) + `RecordOrphanAndHardDeleteServer` (`store_servers_lifecycle.go:18`) handle partial create. | Beacon is safe to `Create` retry after panel crash; PufferPanel aborts on existence and relies on manual `Delete→Create`, risking orphans without remediation records. |
| 14 | **Configuration synchronization** | Configuration is the JSON file itself. `Save` (`servers/server.go:500-520`) marshals whole `Server` atomically; `Reload` picks up on-disk edits. No explicit "config sync pending" — absence implies sync. | Explicit sync handshake: `ServerProvisionTarget` (`store_servers_control.go:83-203`) + `runtimeConfiguration` (`clustermanager/service.go:689-715`) produce canonical payload; `SyncServerConfiguration` (`265-283`) writes then calls `MarkServerConfigSynced/Failed` (`244-264`). Panel surfaces `config_sync_pending/config_sync_error` (`store_servers.go:310-312`). `handlers_servers.go:1097-1108 GET /servers/:id/configuration` exposes the canonical form. | Forge makes drift observable and retryable; PufferPanel config drift is silent unless an operator reloads. |
| 15 | **Deletion / uninstall lifecycle** | `servers/server.go:373-408 Destroy` runs `GenerateProcess(Uninstallation)` then `RunningEnvironment.Delete()` (`environment.go:172 os.RemoveAll`). `servers/server_loader.go:163-207 Delete` stops if running (`Kill+WaitForMainProcess`), stops scheduler, then `Destroy` + removes `.json` + splices `allServers`. No orphan table; failure leaves FS state. | `clustermanager/service.go:332-360 DeleteServer`: calls `runtime.DeleteServer`; on failure without `force` surfaces daemon error; with `force` records `server_orphan_remediations` (`store_servers_lifecycle.go:36-40` no FK to `servers`) before `HardDeleteServer` (`12-56` transactional `FOR UPDATE` + allocation release + `server_orphan_remediations` insert). `HardDeleteServer` is the only path that hard-deletes the row (`DELETE FROM servers`). | Forge deletion is stronger — failure is never silent and is audited; PufferPanel deletion is best-effort filesystem removal with no remediation queue. |
| 16 | **Hardening / validation surfaces** | Docker host config merges user `HostConfig` verbatim (`servers/docker/docker.go:507 HostConfig.AutoRemove=true` + appends `Binds/PortBindings`). User (`Config.User`) defaults to `os.Getuid():Getgid()` (`461`). Image pulled without digest pinning (`301-370 PullImage` checks `ImageList` by reference, no SHA). Custom mount source resolution via `CopyFromContainer` probe (`container_mount_source.go:65`). | `beacon/internal/runtime/docker.go:901-980 validateCreateRequest` enforces: `serverID` charset/len (`990`), `MemoryMB/Swap/CPUShares/CPUSet/IOWeight/PIDLimit/StopSignal` ranges, `NetworkName` required, subnet/gateway/IP validation, DNS IP check, duplicate port detection (`897`). `ensureImage` (`113`) requires `@sha256:...` pinning unless `DAEMON_ALLOW_UNPINNED_IMAGES=true`. Installer runs as `1000:1000`, `ReadonlyRootfs:true`, `CapDrop:ALL`, `no-new-privileges:true`, `Init:true` (`281-304`). Host path validation via `validateRootDir` + `buildContainerMounts` symlink resolution. | Beacon hardens each runtime knob at admission and at pull time; PufferPanel largely trusts the template author and reuses host identity, a weaker posture. |

---

## 4. Logic Findings (5 findings — requirement >=3)

### LF-01 — PufferPanel `afterExit` crash-restart is unbounded-survivable and races `isUnsafeRunning` — Beacon circuit-breaks

**Files:**
- `reference/game-hosting/pufferpanel/servers/server.go:562-602` `afterExit`
- `reference/game-hosting/pufferpanel/servers/server.go:190 TryLock / 194-200 deferred Unlock on early error / 592 Unlock at end`
- `reference/game-hosting/pufferpanel/config/entries.go:55` `CrashLimit` default 3
- `beacon/internal/server/manager.go:92-101` `crashCooldownDefault=1m`, `maxConsecutiveCrashRestarts=5`, `crashLoopWindow=30m`
- `beacon/internal/server/manager.go:915-955` `HandleContainerEvent` breaker

**Observation:**
PufferPanel determines `graceful := exitCode == ExpectedExitCode` (`568`), resets counter only on graceful (`570`), and on crash checks `CrashCounter < CrashLimit` (default 3) then does `CrashCounter++ ; StartViaService(p)` (`598-600`). `CrashCounter` is never persisted — a daemon restart resets it to 0. The guard is `isUnsafeRunning.TryLock` at `Start` entry (`190`) and `Unlock` only in `afterExit` (`592`). Between `afterExit` deciding to `StartViaService` (which just enqueues to a 1s `processQueue` ticker) and the actual next `Start`, the mutex is *unlocked* but `CrashCounter` is already incremented — a concurrent `Start` call can interleave and observe a stale counter. No cooldown is enforced — a tight crash loop (e.g., bad `startupCommand` segfault) will restart as fast as the 1s queue allows until `CrashLimit` then stays down silently with no persistence: operator must manually `Start` to reset. There is no `DetectCleanExitAsCrash` parity — exit 0 with `AutoRestartFromGraceful=false` never restarts, even if the operator expects Wings-like `detectCleanExitAsCrash`.

Beacon instead: `CrashCount` is `sync.Mutex`-protected under `state.mu`, increments only after checking `CrashCooldown` (`925`) and loop-window decay (`935-938` resets if last crash outside window), then checks `loopBroken := CrashCount > 5` (`939`) before deciding. `persistPowerState` (`387-428`) writes `LastStartedAt/ExpectedStop` to `dataDir/.beacon-state/<id>.json` atomically, so `Reconcile` + `autoRestartCrashed` (`177-206` window 24h) preserve crash semantics across daemon restarts. `CrashCount` is intentionally *not* reset on `start` (`525` comment) so boot-loop workloads are caught, but a clean `ExpectedStop` resets the breaker (`909`). The single `RunningAction` slot (`manager.go:482-507 HandlePower` TryLock + `RunningAction != ""` check) prevents the TryLock race that PufferPanel's split `isUnsafeRunning` + queue introduces.

**Impact:** PufferPanel operators with short-crash workloads will see silent permanent offline after 3 crashes with no observability or daemon-restart resilience; Beacon's breaker + persistence keeps semantics correct across node reboots while limiting log spam.

**Recommendation:** If retaining PufferPanel's model, persist `CrashCounter`/`LastCrash` to disk, add a cooldown timer, and replace `isUnsafeRunning` + queue with a single `RunningAction`-style slot so crash-restart enqueue is atomic.

---

### LF-02 — PufferPanel `OperationProcess.Run` success-conditional gating is dead code — Beacon queues are TTL/ack-aware

**Files:**
- `reference/game-hosting/pufferpanel/servers/operation_process.go:132-186` `Run` (`135-138 extraData[success]=true`, `142 shouldRun := RunCondition`, `162-172 error branch with commented-out `firstError` logic`)
- `reference/game-hosting/pufferpanel/servers/operation_functions.go:15-35` CEL `file_exists/in_path/is_server_running`
- `beacon/internal/server/queue.go:339-354 ExpireExpired`, `364-405 EnqueueCommandWithTTL`, `314-337 Ack/SetError/SetProgress`

**Observation:**
PufferPanel operations support CEL `if:` (e.g., `file_exists("/data/server.jar")`) and the `extraData` map seeds `success:true` for subsequent steps (`137`), but the error path (lines `162-172`) immediately `return result.Error` and the commented TODO at `165-171` makes clear the `success=false` propagation was never finished. Steps that should skip on prior failure currently never run because the process aborts entirely — and steps that *should* run on failure (`if: "!success"`) can never fire because the process short-circuits. `GenerateProcess` (`69-122`) snapshots `DataMap` per step with token replacement, but `VariableOverrides` (`177-183`) mutate `server.Variables` *mid-process* while later steps' `DataMap` snapshots were already frozen — a step that yields a download URL into a variable cannot feed it to the next step's `if:` without re-reading.

Beacon's `OperationQueue` (`queue.go:364 EnqueueCommandWithTTL`) explicitly persists `TTL`/`progress`/`acknowledged`/`result_data` to SQLite (`91-100 schema`) and reaps expired `Pending` ops via `ExpireExpired` (`339` called every 60s at `server.go:312`). `processOp` checks `TTL` before promotion to `Running` (`234`). PufferPanel has no TTL, no journal recovery (a daemon crash mid-`Install` leaves `Installing=true` in memory only — `environment.go:45` not persisted — so reboot marks the server as installable again without signaling failure), and no `acknowledged` handshake.

**Impact:** PufferPanel install graphs with conditional fallback steps (e.g., "if download failed, retry with other mirror") cannot be expressed correctly — the whole install fails eagerly. Beacon's durable queue semantics (idempotent `CommandID` dedupe, TTL, persisted pending→running promotion) are missing.

**Recommendation:** Either complete `extraData[success]` propagation and move to per-step `DataMap` live-reference (not snapshot), or document that `success`-gating is unsupported. For the journal gap, persist `Installing`/`RunningAction` similar to Beacon's `persistPowerState`.

---

### LF-03 — PufferPanel Docker create is non-idempotent and leaks containers — Beacon create is idempotent + reconciling

**Files:**
- `reference/game-hosting/pufferpanel/servers/docker/docker.go:55-128` `ExecuteAsyncImpl` (`74-76 doesContainerExist → error "docker container already exists"`)
- `reference/game-hosting/pufferpanel/servers/docker/docker.go:301-370` `PullImage` (no hash check)
- `reference/game-hosting/pufferpanel/servers/server_loader.go:120-161` `Create` (mkdir+Save+Load, no container)
- `reference/game-hosting/pufferpanel/servers/server_loader.go:228-261` `Reload` (no container recreate)
- `beacon/internal/runtime/docker.go:151-166` `Create` (idempotent early return `160-164`), `171-249` `Reconcile` (hash compare `200-202`, `restartAfterCreate` flag, `WithCancel` cleanup)
- `beacon/internal/runtime/docker.go:198-212` `createRequestHash` (credential-redacted hash)
- `forge/api/internal/services/clustermanager/service.go:285-302` `compensateCreateFailure`

**Observation:**
PufferPanel's Docker `ExecuteAsyncImpl` decides at `74-76` that an existing container of the same `ServerId` is a hard `error "docker container already exists"` rather than inspecting whether its config differs and reconciling. The only caller that creates the container is a *start*, not an explicit provision step — so the panel's `Create` (`server_loader.go:120-161`) never creates a container; containers are ephemeral per-`Start` with `AutoRemove:true` (`servers/docker/docker.go:506`). A panel retry after a partial create (panel row written, beacon equivalent crash before `Start`) leaves no container but does leave `servers/<id>/` and `<id>.json`; a second `Create` is rejected as "server already exists" (`server_loader.go:121`). `Reload` does not stop/diff/recreate a running container — an operator edit to `docker.ImageName` takes effect only after the workload stops naturally.

Beacon's `Create` is intentionally idempotent (`beacon/internal/runtime/docker.go:159-164` inspect→no-op if exists) precisely so Forge's `provisionServer` (`clustermanager/service.go:172-191` `syncProvisionTarget` then `CreateServer`) can safely retry after any crash — the panel crash right after the first successful `Create` but before `SetServerProvisioned` still results in a single workload. `Reconcile` (`171`) goes further: it hashes the *full* desired `CreateRequest` minus secrets (`createRequestHash`) and, if mismatched, does `stop(if running) → remove → create → start(if was running)` atomically under a per-server `workloadLocks[sha256[0]%64]` (`251-253`), with orphan cleanup via `context.WithoutCancel` (`234-235`).

**Impact:** PufferPanel cannot be driven safely by a remote orchestrator that retries creates; it also cannot atomically apply a config change to a running container — a `Reload` during `Running` silently desynchronizes desired vs actual config.

**Recommendation:** For Forge-client use, wrap PufferPanel provision in an idempotent path (pre-check `IsRunning` + `ContainerInspect` + conditional `Remove`), or adopt Beacon's hash-gated reconcile. The existing `compensateCreateFailure` + `RecordOrphanAndHardDeleteServer` pattern is the compensating transaction that PufferPanel lacks.

---

### LF-04 — PufferPanel console/stats streams are unbounded and unthrottled — Beacon isolates per-workload and sanitizes errors

**Files:**
- `reference/game-hosting/pufferpanel/environment.go:37-47` `ConsoleBuffer *MemoryCache`, `41 Wrapper io.Writer`, `211-218 CreateWrapper` (MultiWriter to `logging.OriginalStdOut` when `ConsoleForward` true)
- `reference/game-hosting/pufferpanel/servers/docker/docker.go:98-106` `io.Copy(environment.Wrapper, connection.Reader)` (unbounded copy), `126 handleClose` (blocks on `ContainerWait` with `container.WaitConditionRemoved`)
- `reference/game-hosting/pufferpanel/servers/docker/docker.go:187-194` stats cache `5*time.Second` throttle (single-process global, not per-server), `202-215` `ContainerStats` with `utils.Close` defer on possibly nil `res.Body`
- `beacon/internal/server/console.go:56-84` `consoleManager` (`57-59 Replay 128/256KB`, `64 producers map[string]*consoleProducer`, `79-84 subs map`)
- `beacon/internal/server/console.go:134-163` `publish` / fan-out drop-if-slow + replay
- `beacon/internal/server/server.go:516-583` `recoverPanics` + `sanitizeInternalErrors` (WebSocket bypass)

**Observation:**
PufferPanel's `Environment.Wrapper` (`environment.go:41`) is shared; `ExecuteAsyncImpl:99-101` starts `go io.Copy(Wrapper, connection.Reader)` with **no limit** — a runaway process logging GB/s will copy into the in-memory `MemoryCache` (fixed `ConsoleBuffer=50` entries by default — `config/entries.go:43` — but the `Tracker` broadcast fans out without backpressure). The goroutine also captures `environment` by reference; `Server.Destroy` (`373`) deletes the directory while the copy may still be reading, a TOCTOU on `RootDirectory`. `handleClose:581` waits for `container.WaitConditionRemoved` but `HostConfig.AutoRemove:true` (`servers/docker/docker.go:506`) means the container disappears on stop — the wait can return `404` if the runtime pruned the state between `ContainerStop` and the wait, leaving `Wait.Unlock` (`610`) to release a lock no longer associated with a known state. The stats path (`187-267`) caches per-env but the `5s` gate (`191-193`) is checked without holding a global lock across the actual fetch — concurrent `GetStats` from two HTTP handlers for the *same* server can double-fetch.

Beacon's `consoleManager` (`console.go:62-89` `Ensure`/`detach`) holds exactly one `producer` per server, each with its own `replay` ring (`57-59` 128 entries, 256KB cap) and bounded selective fan-out (`149-161` drains stale before enqueue — slow websockets never block the runtime reader, and the reader is an `io.Pipe` fed by `stdcopy.StdCopy` for non-TTY with a `/demo console` limiter). `sanitizeInternalErrors` (`575-583`) intercepts `>=500` to avoid leaking stack/exit reasons to subusers, with a direct `IsWebSocketUpgrade` bypass (`577`). `WatchEvents` (`beacon/internal/runtime/docker.go:639-705` exponential backoff, buffered 128/1, `panic` recovery per-event at `manager.go:859-864`) ensures one bad handler cannot kill supervision for all servers — PufferPanel's `processQueue/processStats` goroutines (`servers/server.go:92-119` nil-ticker deref on `ShutdownService` without guard beyond `!running`) lack that isolation.

**Impact:** Low — PufferPanel works for single-operator, few-server deploys; under Forge-scale (dozens of concurrent consoles + burst logs) the copy/replay/broadcast costs and the missing per-consumer drop behavior create observability loss and potential goroutine leaks.

---

### LF-05 — PufferPanel suspend/install guards are racy and incomplete — Beacon enforces suspended + installing as first-class pre-start gates

**Files:**
- `reference/game-hosting/pufferpanel/servers/server.go:884-899` `IsIdle` (checks `Running()` + `IsInstalling()` + backingUp, no lock)
- `reference/game-hosting/pufferpanel/servers/server.go:410-434` `Install` (checks `IsIdle`, sets `SetInstalling(true)` deferred `false`, checks `IsRunning` again)
- `reference/game-hosting/pufferpanel/servers/server.go:373-408` `Destroy` (delegated to `IsIdle`)
- `beacon/internal/server/manager.go:610-673` `onBeforeStart` (snapshot under lock `610-620`, checks `installing 622`, `suspended 628`, `synced 633`, `root 638`, disk limit `659-672`, plus optional chown/panel sync)
- `beacon/internal/server/manager.go:271-292` `BeginInstall` (`TryLock`, checks `RunningAction/suspended/installing` atomically)
- `beacon/internal/server/manager.go:842-960` `StartEventWatcher` / `HandleContainerEvent`
- `forge/api/internal/store/store_servers_control.go:388-411` `RequestServerPower` pre-checks `Suspended` before `SetServerDesiredState`

**Observation:**
PufferPanel `IsIdle` at `884` calls `GetEnvironment().IsRunning()` and `IsInstalling()` each time — `IsInstalling()` reads `environment.Installing` (`environment.go:233`) with no synchronization, while `Install` sets it at `415` via `SetInstalling(true)` which writes two fields and a tracker message uncaptured by the caller. The gap between `IsIdle` and `SetInstalling(true)` is unsynchronized, so two concurrent `Install` calls can both pass `IsIdle` and both proceed to `MkdirAll` + `GenerateProcess` interleaving on the same `RootDirectory`. The `Destroy` guard inherits the same race — an `Install` in progress and a `Delete` (`server_loader.go:163 Delete` calls `IsRunning` then `Kill+Wait` then `Destroy`) can interleave, with `Destroy` running `Uninstallation` operations while the install's `MkdirAll` is still populating files.

Beacon serializes with a single `RunningAction` slot (`manager.go:488-506 HandlePower`: `TryLock` then `RunningAction != ""` check under the *same* lock, slot set to `signal`). `BeginInstall` (`271-292`) enforces the same slot plus explicit `Suspended` and `installing` checks atomically — concurrent installs or power ops are rejected with `another server action is already running`. Panel-side `RequestServerPower` (`clustermanager/service.go:388` checks `server.Suspended` before `SetServerDesiredState`) and `onBeforeStart` (`610` snapshots under lock) add defense-in-depth: a mid-flight suspend is observed before `Start`. `SetStateDir` persistence (`manager.go:124-126`, `server.go:249` wired to `dataDir/.beacon-state`) means the `ExpectedStop` + `LastStartedAt` gate survives daemon crashes — PufferPanel's `Installing` never survives a crash, so post-crash state is falsely idle.

**Impact:** Low–medium: single-node PufferPanel rarely triggers concurrent `Install`/`Delete`, but a remote panel driving it (Forge overlay) can — the race can corrupt server roots or leak `Uninstallation` work onto partially installed trees. Beacon's `RunningAction` design is the reference for any future multi-tenant driver.

---

## 5. Deep Dives

### 5.1 Daemon/Server Ownership & Hosting Split

PufferPanel `Server` **is** the hosting resource — it owns `RunningEnvironment` (`servers/server.go:33` pointer) + `fileServer` (`37`) + `Scheduler` (`34`) and the in-memory `allServers` slice (`server_loader.go:15`). Data lives on disk as `${ServersFolder}/<id>` directory + `<id>.json` + optional `<id>.cron`. The panel's `GetFromCache(id)` (`209`) is the lookup for every API handler.

Forge owns a `servers` row + `allocations` assignment + `server_variables` rows in a single transaction (`store_servers.go:180-306`), then delegates a **pure Docker/Runc job** to Beacon (`clustermanager/service.go:172-191`). Beacon keeps no DB, but persists power state to `dataDir/.beacon-state` so `Reconcile` can honor crash-restart without the panel. The doc contract (`forge/api/docs/server-lifecycle.md:1-16`) is explicit: panel row = `provisioning`, Beacon accept = `created` (not `installed`), `install` is a separate explicit step, `delete` is beacon-first unless `force`.

Takeaway for any PufferPanel replacement/migration: map `Server.Identifier`→Forge `server.id`, `Execution`→rendered `CreateRequest`, `Installation`→egg `install_script`/Beason `InstallRequest`, `Groups/Variables`→`egg_variables`/`server_variables`. The `supportedEnvironments` → runtime provider.

### 5.2 Filesystem & Security

PufferPanel filesystem path is `RunningEnvironment.RootDirectory` (`environment.go:35`) defaulting to `${ServersFolder}/<id>` (`servers/env_loader.go:53`). File serving uses `files.NewFileServer(RootDirectory, Uid, Gid)` (`server_loader.go:109`) which does internal uid/gid checks but is host-path-adjacent. Archive via `archiver` with `OverwriteExisting=true` (`servers/server.go:55-57`) has no expanded-byte cap — a zip bomb can fill the host.

Beacon's `rootfs.FS` (`beacon/internal/server/secure_files.go:60`) is instantiated per `serverFilesystem(serverID, create)` confined to `dataDir/serverID` after `serverid.Validate`. Archive extraction is staged: `validateZip/validateTar` pre-checks `4GB/100k entries` (`492`), `HasSpaceForWriteFS` quota (`518`), then `extract*Staged` into a `.extract-<suffix>` dir, then atomic `mergeStaging` with backup + rollback (`389-480`). Cross-node transfer is HMAC-scoped single-use credentials over `POST /api/v1/transfers/credentials` (`server.go:373` comment notes legacy removal precisely for credential-forwarding risk). This is the defensive baseline to preserve in any migration.

### 5.3 Install & Provisioning Comparison to Forge `clusterManager.provisionServer`

PufferPanel provision is "create the `Server` struct and save JSON" — no container yet. Install later runs `Installation` ops sequentially, each able to pull binaries (`javadl`/`paperdl`/… call Adoptium/Mojang APIs directly), write files, and consult `DataToMap()` (`servers/server.go:148-155` injects `rootDir/core:os/arch`). Forge `provisionServer` (`clustermanager/service.go:172-191`) instead assembles `ServerProvisionTarget` (full row join `ServerProvisionTarget:88-137`), `syncProvisionTarget` (`276` → `SyncServerConfiguration`), then `CreateServer` on the runtime — all with a 10m placement reservation (`109` `expiresAt`) that is `Confirm`ed (`155`) only on success.

The Forge path is stronger for at-scale relay: the reservation prevents placement races, `compensateCreateFailure` handles partials, and `installed=false` stays `false` until a successful `InstallServer` (`216-263` only `installed` on `ExitCode==0` and `Accepted`). PufferPanel's install result is observed only via console buffer, not a DB state — `Install` returns `error` directly (`servers/server.go:465`), panel must infer outcome.

---

## 6. Feature Delta — What Forge Defers

- **Mod-loader operations catalog** (`operations/curseforge/javadl/mojangdl/fabricdl/forgedl/...`): Forge eggs script this instead of shipping per-loader SDKs.
- **KeepAlive** (`server.go:62-65`) and **per-template `Stdin` proxy types** (`environment.go:95-119` `telnet/rcon/rconws`): Beacon assumes the container's `OpenStdin+AttachStdin` + `SendCommand` is sufficient; external KeepAlive can be added as a panel schedule.
- **Per-server cron inside daemon** (`servers/scheduler.go` + `task.go:3-8`): Forge schedules live in the panel; trade-off is daemon-offline execution.
- **Variables Groups UI model** (`server.go:54-60 Group`): Forge's `egg_variables` is functionally similar but rendered by Forge web, not the agent.
- **`AutoStart` on daemon boot** (`server.go:34 Execution.AutoStart`) — Beacon instead reconciles panel `desiredState=running` via `ListServersForNode` + `ReconcileNode` and `Reconcile` per-server; `AutoStart` semantics are expressed as `desiredState`.

None of these are correct-by-omission gaps; they are deliberate scope choices.

---

## 7. Risk Assessment

| Risk | Severity | Where | Note |
|------|----------|-------|------|
| Silent infinite-off offline after 3 crashes | Medium | `servers/server.go:598-601` | No persistence, no cooldown, no observability; operator sees `Stopped` with no error surface |
| `IsIdle`/`BeginInstall` race corrupts install tree | Low | `servers/server.go:410-434`, `884` | Requires concurrent `Install`/`Delete` — unlikely single-tenant, plausible when Forge drives PufferPanel |
| Zip bomb / unbounded archive expansion | Medium | `servers/server.go:55-57, 664-666` | `OverwriteExisting=true`, no expanded cap; Beacon caps at 4GB + staged rollback |
| `WaitConditionRemoved` vs `AutoRemove:true` | Low | `servers/docker/docker.go:506, 581-596` | Wait can 404 after auto-remove, leaving `Wait.Unlock` mismatched |
| Unpinned Docker pull | Medium | `servers/docker/docker.go:301-370` | No digest pinning — Forge requires `@sha256:` by default |

---

## 8. Recommendations

1. **Keep PufferPanel scope narrow** — treat it as reference for operation-catalog ergonomics (variable-gated steps, CEL helpers) and scheduler authoring, not for multi-tenant hosting.
2. **If bridging PufferPanel templates into Forge**, translate each `Installation` op to a line in the Forge egg `install_script` rather than re-implementing per-loader fetchers on the agent; retain PufferPanel's `resolve*Version` patterns as version-resolution helpers in the egg build.
3. **Port KeepAlive as an opt-in panel schedule**, not as a daemon ticker — avoids per-container goroutine while honoring operator UX for keep-alive pings.
4. **Adopt Beacon's reconciling, hash-gated create** if any PufferPanel-like daemon adds remote provision retries.

---

## 9. Files Inspected

**PufferPanel (`reference/game-hosting/pufferpanel`):**
`server.go`, `environment.go`, `environmentfactory.go`, `operation.go`, `conditions/`, `config/config.go`, `config/entries.go`, `scopes/scopes.go`, `servers/server.go`, `servers/server_loader.go`, `servers/env_loader.go`, `servers/operation_process.go`, `servers/operation_functions.go`, `servers/scheduler.go`, `servers/docker/docker.go`, `servers/docker/factory.go`, `servers/docker/imagewriter.go`, `servers/docker/container_mount_source.go`, `servers/tty/*`, `task.go`, `operations/*` (22 factories), `files/`

**Beacon:**
`beacon/internal/server/server.go`, `beacon/internal/server/manager.go`, `beacon/internal/server/console.go`, `beacon/internal/server/queue.go`, `beacon/internal/server/secure_files.go`, `beacon/internal/server/hostfiles.go`, `beacon/internal/server/stats_collector.go`, `beacon/internal/server/enrollment.go`, `beacon/internal/runtime/runtime.go`, `beacon/internal/runtime/docker.go`, `beacon/internal/runtime/factory.go`

**Forge:**
`forge/api/docs/server-lifecycle.md`, `forge/api/internal/services/clustermanager/service.go`, `forge/api/internal/store/store_servers.go`, `forge/api/internal/store/store_servers_lifecycle.go`, `forge/api/internal/store/store_servers_control.go`, `forge/api/internal/http/handlers_servers.go`

---

## 10. Appendix — PufferPanel Is Not Architecturally Unrelated (Correction of Prior Exclusion)

Phase 2 Subagent-03 excluded PufferPanel as "architecturally unrelated." This audit shows that, while deployments differ, they overlap on every lifecycle axis (provision → create → install → power → delete → reconcile). The operation-process CEL patterns, variable-overrides pipeline, and scheduler ergonomics are directly informative for Forge egg and schedule design, and the comparison uncovers concrete gaps (idempotent create, persisted power state, archive staging) that inform forward remediation. The prior exclusion is corrected; PufferPanel remains reference material.


