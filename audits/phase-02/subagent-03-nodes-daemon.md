# Subagent 03 — NODE / DAEMON / BEACON vs WINGS

**Dimension:** Node lifecycle (register / heartbeat / health / metrics), daemon file operations, console, SFTP, installer, backups, transfers, runtime isolation

**Scope:** `pelican-wings` + `pterodactyl-wings` (+ panel node/daemon interaction) vs Forge `beacon` + `forge/api` daemon/client, heartbeatmonitor, nodeprobe, noderegistry, clustermanager, crossnode; `beacon/internal/{runtime,backup,sftpserver,transfer,system}`; `forge/api/store` nodes / heartbeat history / capabilities

**Date:** 2026-08-23
**Reference roots:**
- `reference/game-hosting/pterodactyl-wings` (Wings)
- `reference/game-hosting/pelican-wings` (Pelican, fork of Wings)
- `reference/game-hosting/pterodactyl-panel` / `pelican-panel`
- `beacon/{cmd/daemon,internal/server,internal/runtime,internal/backup,internal/sftpserver,internal/transfer,internal/system,config}`
- `forge/api/{internal/daemon,internal/store,internal/services/heartbeatmonitor,internal/services/nodeprobe,internal/services/noderegistry,internal/services/clustermanager,internal/services/crossnode,internal/http}`

---

## 1. Executive Summary

Forge's Beacon is a superset of Wings/Pelican in almost every daemon dimension: multi-runtime, signed host-file API, enrollment, Prometheus metrics, capability inventory, transfer-v1, and a 6-state heartbeat monitor. Wings/Pelican remain lean Docker-only daemons with Gin routing, a single `GET /api/system` and a 2-field daemon health model. The cost is complexity: Forge carries ~3.9 kLOC in `beacon/internal/server/server.go` alone (`beacon/internal/server/server.go:1`) versus Wings `router/router.go:19` + `server/*.go`, duplicates much of the store state machine, and introduces new failure modes around heartbeat history sorting, quota enforcement, capability marshaling, and dev-version bypass.

Overall gap vs Wings: **Beacon exceeds Wings functionally; parity is covered and extended. No major Wings capability is missing.** PufferPanel's daemon (`reference/game-hosting/pufferpanel`) is architecturally unrelated (Go daemon + `engine.go`/`files` handling via its own REST surface, no per-node Wings-style auth) and intentionally excluded from the Wings-family comparison below — see §6.

3 logic findings are raised (overcommit scoring, dev-version bypass fragility, and heartbeat history truncation / replay coverage), plus several observations.

---

## 2. Methodology & Files Inspected

**Wings/Pelican daemon:**

| File | What was checked |
|------|------------------|
| `reference/game-hosting/pterodactyl-wings/router/router.go:1` | Route topology, middleware stack, Gin setup, token regex |
| `reference/game-hosting/pelican-wings/router/router.go:1` (diff) | Added `router_server_files_search.go`, extra diagnostics/system routes |
| `reference/game-hosting/pterodactyl-wings/router/router_server_files.go:1` | File handlers: contents/list/rename/copy/delete/write/compress/decompress/chmod/pull/upload |
| `reference/game-hosting/pterodactyl-wings/router/router_server_ws.go` | `/api/servers/:server/ws` JWT upgrade |
| `reference/game-hosting/pterodactyl-wings/router/router_server_backup.go` | `POST /backup`, `POST /:backup/restore`, `DELETE` |
| `reference/game-hosting/pterodactyl-wings/server/filesystem/*.go` | `filesystem.go`, `archive.go`, `compress.go`, `disk_space.go`, `path.go` |
| `reference/game-hosting/pterodactyl-wings/server/console.go` | Throttle/Rate+Locker, publisher |
| `reference/game-hosting/pterodactyl-wings/server/backup/*.go` | Backup adapters (wings tardumps + restic/S3) |
| `reference/game-hosting/pterodactyl-wings/sftp/{server.go,handler.go}` | SFTP listener, handler, activity logging |
| `reference/game-hosting/pterodactyl-wings/server/transfer` | Legacy daemon→daemon archive push |
| `reference/game-hosting/pterodactyl-wings/config/config.go` | Daemon config (api, system.data/sftp) |
| `reference/game-hosting/pterodactyl-wings/server/manager.go` | Server registry, state cache |

**Forge Beacon / Panel:**

| File | What was checked |
|------|------------------|
| `beacon/internal/server/server.go:239` `NewServerWithBackup` | Full route table (~130 routes, lines 330-502), auth, replay, panic recovery, sanitize |
| `beacon/internal/server/secure_files.go:1` | `serverFilesystem`, staged archive extraction, pinned resolver, `securePullClient` |
| `beacon/internal/server/hostfiles.go:1` | `validateHostPath`, `resolveHostPath`, denylist vs allowlist |
| `beacon/internal/server/manager.go:103` | `ServerManager` state, `BeginInstall/EndInstall`, `HandlePower`, crash watcher |
| `beacon/internal/server/console.go:1` | `consoleManager`, bounded replay (128 entries / 256 KiB), `ConsoleThrottle` |
| `beacon/internal/server/capabilities.go:1` | `collectCapabilities`, `handleGetCapabilities`, delta/heartbeat |
| `beacon/internal/server/enrollment.go:1` | `EnrollmentManager`, hash-persisted tokens |
| `beacon/internal/server/container_admin.go:1` | Portainer-style `/api/admin/*` |
| `beacon/internal/runtime/runtime.go:1` + `docker.go`, `containerd.go`, `podman.go`, `firecracker.go`, `kubernetes.go`, `factory.go` | Provider interface, factory, multi-runtime |
| `beacon/internal/backup/backup.go:1`, `local.go`, `s3.go`, `store.go`, `retention.go` | `BackupInterface`, `BackupManager` metrics |
| `beacon/internal/sftpserver/server.go:86` | `Server.Run`, `handler` quota path, `idleConn`, `rate.Limiter` |
| `beacon/internal/transfer/protocol.go` + `transfer.go` | `forge-beacon-transfer/v1` dual-endpoint credential model |
| `beacon/internal/system/fs_linux.go` | `openat2`/`RESOLVE_BENEATH` rootfs |
| `beacon/internal/metrics/metrics.go:1` | `PrometheusCollector`, `normalizeMetricPath` |
| `beacon/cmd/daemon/main.go:730` | `heartbeatLoop`, `runtimeHeartbeatStatus` |
| `beacon/internal/remote/client.go:272` | `SendNodeHeartbeat`, HMAC+Bearer dual channel |
| `beacon/internal/remote/types.go:100` | `NodeHeartbeat` schema |
| `forge/api/internal/daemon/client.go:119` `Client` | Signed panel→beacon client (HMAC+nonce+Bearer), retry resign, transfer credentials |
| `forge/api/internal/store/store_nodes.go:727` `UpdateNodeHeartbeat` | Heartbeat persistence (`last_seen_at`, `docker_status`, etc.) |
| `forge/api/internal/store/store.go:381` `NodeHeartbeatRequest` + `store.go:1479` heartbeat states + `store_capabilities.go:239` | Capability inventory |
| `forge/api/internal/http/server.go:1593` heartbeat endpoint + `1694` capabilities webhook + `262` `nodeHeartbeatNonces` | Ingestion path |
| `forge/api/internal/services/heartbeatmonitor/service.go:205` | `evaluate` / `classify` 6-state machine |
| `forge/api/internal/services/noderegistry/service.go:130` | `RecordHeartbeat`, `LifecycleView`, `HealthScore`, `PlacementEligibility` |
| `forge/api/internal/services/nodeprobe/service.go:1` | `ProbeNode` live HMAC `GET /api/system` |
| `forge/api/internal/services/clustermanager/service.go:1` | `CreateServer` + scheduler + reservation + `compensateCreateFailure` |
| `forge/api/internal/services/crossnode/resolver.go:1` + `health_filter.go`, `routegroup.go` | Cross-node resolution caching `30s`, `SD` fallback |

---

## 3. Comparison Matrix (18 comparisons; ≥15 required)

| # | Dimension | Wings / Pelican | Forge Beacon + Panel | Parity / Gap / Exceeds |
|---|-----------|-----------------|----------------------|------------------------|
| 1 | **Node registration & credential** | Panel creates node; wings stores `token` + `token_id` in `config.yml` (`pterodactyl-wings/config/config.go`). No enrollment. Rotation via `POST /api/update` (`router/router.go:64`). | `Store.CreateNode` / `RotateNodeToken` (`forge/api/internal/store/store_nodes.go`), `store_nodes.go:1113` `newDaemonToken`. Beacon `config/beacon.yaml` + env `DAEMON_NODE_TOKEN`/`DAEMON_NODE_ID` (`beacon/cmd/daemon/main.go`). `EnrollmentManager` (`beacon/internal/server/enrollment.go:53`) hash-persisted tokens, `Pending→Approved→Consumed` (bcrypt in `store_capabilities.go:76`). `POST /api/enroll` / `GET /api/enroll/status` (`beacon/internal/server/server.go:402`). Store tokens never leak plaintext (`NodeConfiguration` `Token:""` comment `store_nodes.go:1049`). | **Exceeds** — Enrollment is net-new; Wings has no consumption lifecycle. |
| 2 | **Daemon route topology** | Gin `router.Configure` (`router/router.go:19`): ~30 routes, grouped under `/api/servers/:server` + `/files`, `/backup`, `/transfer`, `/ws`. Middleware: `RequireAuthorization` (Bearer), `ServerExists`. Signed-url trio: `GET /download/backup`, `GET /download/file`, `POST /upload/file`. | `Server.NewServerWithBackup` (`beacon/internal/server/server.go:239`) registers ~130 routes in `net/http` (`ServeMux` with method+path patterns). Groups: `/servers/{id}/*` (power/stats/logs/install/console/files/backups), `/api/*` (system/capabilities/enroll/update/diagnostics/upgrade/commands), `/v1/*` (host files), `/api/v1/transfers/*` (protocol v1), `/api/admin/*` (Portainer), `/database/*`, `/compose/*`, `/build/*`, `/git/*`, `/image/*`. Middleware chain: `requestTimeout`→`authenticate`→`securityHeaders`→`recoverPanics`→`sanitizeInternalErrors`. `isScopedTokenRoute` carve-out for JWT-download. | **Exceeds** — Beacon routes are ~4× Wings; explicit legacy-transfer removal comment `server.go:360`. |
| 3 | **Heartbeat transport** | **Wings has no heartbeat push.** Liveness is inferred from panel→wings `GET /api/system` polling (and on-demand node checks). `config.yml` `remote.query.timeout` etc. | Beacon **pushes** `POST /api/v1/nodes/:id/heartbeat` every 30 s (`beacon/cmd/daemon/main.go:737` `Ticker 30s`) via `remote.Client.SendNodeHeartbeat` (`beacon/internal/remote/client.go:272` → `postAPI /api/v1/nodes/:id/heartbeat`). Payload `NodeHeartbeat` (`beacon/internal/remote/types.go:101`) includes `Version/OS/Arch/CPUThreads/MemoryMB/DiskMB/RuntimeStatus/RuntimeProvider/Error/Uptime/LoadAverage`. Panel validates **Bearer + HMAC + nonce + `X-Beacon-Version`** (`forge/api/internal/http/server.go:1593-1631`) before `Store.UpdateNodeHeartbeat` (`store_nodes.go:727`). | **Exceeds** — Concept absent in Wings. |
| 4 | **Heartbeat storage model** | Not applicable (panel derives liveness from last successful API call). No `last_seen_at` or heartbeat table. | Panel `nodes` columns: `last_seen_at`, `heartbeat_state`, `heartbeat_error`, `heartbeat_recovery_count`, `status/actual_state/desired_state`, `docker_status/runtime_status`, `node_memory_mb/node_disk_mb` (`store.go:142-166`). Heartbeat history `node_heartbeat_history` + `node_heartbeat_state` enum (`healthy/suspected/unreachable/offline/recovering/reconciling` `store.go:1479`) and `node_capability_history`. `ListNodeHeartbeatHistory` + `SetNodeHeartbeatClassification` (`store_heartbeat.go:9`). | **Exceeds** — Entire state machine is net-new. |
| 5 | **Health classification** | Wings: no daemon-side classification. Panel exposes `remote.Request` error ⇒ node shown offline. `server/resources.go:Stats` returns live container stats if reachable. | `heartbeatmonitor.Service.classify` (`heartbeatmonitor/service.go:266`): 6 states, 4 thresholds (`Warning 30s`, `Offline 90s`, `Unavailable 300s`, `Recovery 2` `DefaultConfig` `service.go:89`), uses `age=now-LastSeen`, `consecutiveSuccessfulHeartbeats(history)`, future-skew guard (`-age>30s → suspected`), last-entry `Success` check. `evaluate` (`service.go:205`) sorts history `ObservedAt DESC`, calls `SetNodeHeartbeatClassification` when `persist`, publishes `EventNodeSuspected/Unreachable/Offline/Recovered/Reconciling` + `EventActualStateChanged`. Metrics counters per state. | **Exceeds** — No Wings equivalent. See logic finding LF-1. |
| 6 | **System / capabilities endpoint** | `GET /api/system` (`router/router_system.go` → `getSystemInformation`): returns live daemon system info (memory, disk, CPU, Docker disk, IPs, utilization). `GET /api/system/utilization` etc. (Pelican adds `GET /api/diagnostics`, disk prune). Minimal Docker status string. | Beacon `GET /api/system` (`server.go:398` `getSystem`) returns `CapabilityReport`-adjacent payload; dedicated `GET /api/capabilities` + `GET /api/capabilities/delta` + `POST /api/capabilities` heartbeat (`server.go:399-401` `capabilities.go:1`). `collectCapabilities` (`capabilities.go:1`) composes 6 capability types (runtime/build/compose/storage/gateway/database), `MemStats`, `NumCPU`, `LookPath("docker"/"nixpacks")`, Compose stack count, backup adapters, SFTP/WS/console flags. Panel persists via `POST /api/v1/nodes/capabilities` → `UpsertNodeCapability` (`store_capabilities.go:239`) + `node_capability_history`; `GET /api/capabilities` inventory. NodeProbe (`nodeprobe/service.go:1`) provides **second** live path: panel→beacon `GET /api/system` HMAC-signed, `DockerStatus=="ok"→DockerAvailable`,-error handling. | **Exceeds** — Forge splits diagnostics (cached), heartbeat (push), capability (inventory+history), and probe (live). |
| 7 | **File operation surface** | Wings `files` group (`router/router.go:92`): `GET /contents`, `GET /list-directory`, `PUT /rename`, `POST /copy`, `POST /write`, `POST /create-directory`, `POST /delete`, `POST /compress`, `POST /decompress`, `POST /chmod`, plus `GET|POST|DELETE /pull` (remote download) and top-level `GET /download/file`, `POST /upload/file`. Pelican adds `GET /search` (`router/router.go:114`). 13 file ops. | Beacon `/servers/{id}/files/*` (`server.go:360-376`): `GET /list`, `POST /mkdir`, `POST /remove`, `POST /rename`, `POST /copy`, `POST /chmod`, `POST /upload`, `GET /download`, `GET /content` (`readFile`), `PUT /content` (`writeFile`), `PUT /upload` (chunked), `POST /archive`, `POST /decompress`, `POST /pull`, plus batch variants `batchDeleteFiles/batchRenameFiles` (`server.go:2526`/`2568`), `copyFile` (`2672`), `pullRemoteFile` (`2715`). Host-files API `/v1/files/*` (`server.go:378`) is separate privileged surface (allowlist/denylist `hostfiles.go:1`). | **Parity + exceeds** — All Wings ops covered; chunked upload, batch renames/copies, host files are extras. Mapping table in §5. |
| 8 | **File security / isolation** | Wings `Filesystem` (`server/filesystem/filesystem.go`): `SafePath` validation, `IsIgnored` denylist, `HasSpaceErr`, `CompressFiles` with temp staging. Single-layer path cleaning; no kernel enforcement. | Beacon double-layer: (a) `serverFilesystem` (`secure_files.go:60`) returns `rootfs.FS` backed by `beacon/internal/rootfs` (`system/fs_linux.go`) using `openat2(RESOLVE_BENEATH)` on Linux 5.6+ with `O_NOFOLLOW`; (b) `hostfiles.go:resolveHostPath` with explicit `DAEMON_HOST_FILES_ALLOWLIST` allowlist vs `/etc//proc//sys/...` denylist + `dataDir` always denied. Upload staging: `lockUpload` (`secure_files.go:37` `sync.Map` per `serverID/uploadID`), `archivePathTracker` (`160`) for zip-slip, staged extraction `extractZipStaged/extractTarStaged`→`commitStaging`→`rollbackArchiveCommit` (`secure_files.go:269`). Limits: `maxFileWriteBytes 16 MiB`, `maxUpload 2 GiB`, `maxPull 512 MiB`, `maxArchive 4 GiB`, `100k entries` (`secure_files.go:1`). Signed pull with `pinnedResolver` + SSRF `restrictedIP` (`545`). | **Exceeds** — Kernel-enforced traversal defense is absent in Wings. |
| 9 | **Console transport** | Wings `server/console.go:1` + `listeners.go`: per-server `Console` struct, `PublishConsoleOutputFromDaemon`, `ConsoleThrottle` (`Rate` per `system` + `Locker`). WebSocket via `router/router_server_ws.go: GET /api/servers/:server/ws` with `ServerExists` + JWT. Single fan-out. | Beacon `consoleManager` (`beacon/internal/server/console.go:26`): 1 `consoleProducer` per server, demand-attached via `Ensure` (`server.go:118`), bounded replay (`consoleReplayEntries=128`, `consoleReplayBytes=256 KiB` `console.go:1`), `sync.Mutex` + copy-on-subscribe. WS at `GET /servers/{id}/ws/console` + `ws/logs` + `ws/stats` + `ws/backup` + `ws/install` (`server.go:348-351`). Auth via `authenticateWebSocket` (`server.go:3504`) validating `tokens.Claims.Scope==ScopeWebsocket` (`server.go:1954`), `trackWebSocket` session registry (`server.go:647`) tied to `deauthorize-user`. Binary+text handling, `30 s` read deadline, `pingWebSocket`. Install WS separately gates `ScopeAdmin` (`server.go:1170`). | **Parity + exceeds** — Core console WS parity; Beacon adds bounded replay, scope-separated install WS, deauthorize, multi-stream fan-out. |
| 10 | **SFTP** | Wings `sftp/server.go:Run` generates ED25519, `ssh.ServerConfig` with 5 KEX/3 ciphers/2 MACs, `PasswordCallback`+`PublicKeyCallback`→`remote.SftpAuthPassword/SftpAuthPublicKey` via panel; `SftpHandler` (`sftp/handler.go:87`) with `Fileread/Filewrite/Filecmd/Filelist`, `ReadOnly` flag, per-event `MustLog(ActivitySftp*)`. No quota/idle/max-session controls beyond panel config. | Beacon `sftpserver.Server.Run` (`sftpserver/server.go:86`) mirrors KEX/cipher set, `loadOrCreateHostKey` (`438`) with optional passphrase, `IdleTimeout 15m` + `MaxSessionLifetime 24h` via `idleConn` (`518`), `MaxConnections 128` + `MaxSessionsPerUser 8` (`sftpserver/server.go:62`), `rate.Limiter` (`35`) per-IP `authVisitor` (`337` `rate.Every 5s burst 5`). Handler (`handler.Filewrite` `586`) enforces `quotaBytes = MbToBytes(diskMB)` (`414`) via `quotaWriter` (`643`) with `WriteAt`/`Close`, `readOnly` flag, `Filecmd/Filelist` with `cleanSFTPPath/rootfs.Clean`, `activity` dedup `system.ActivityDedup` and `record` (`565`). Mirrors Wings API auth flow (`AuthenticateSFTP`/`AuthorizeSFTPSession` via `/api/remote/sftp/auth` `forge/api/internal/http/server.go:2095`). | **Parity + exceeds** — Adds quota, idle, per-user/IP rate limits, passphrase. |
| 11 | **Backups** | Wings `server/backup.go` + `backup/*.go`: `POST /backup` (archive), `POST /:backup/restore`, `DELETE /:backup`, `DELETE deleteAllBackups` (Pelican), `GET /download/backup` signed URL, `restic`/`s3` adapters. Denylist applied via `.pteroignore`. | Beacon `BackupInterface` (`beacon/internal/backup/backup.go:1`) with `LocalAdapter` + `S3Adapter` (`local.go`/`s3.go`), `BackupManager` metrics `RecordBackupDuration`, `SetProgressCallback`→`eventBus BackupProgressEvent` (`server.go:1542`), `createBackup` (`server.go:1495`) honors `.pteroignore` via `ignore.LoadIgnoreReader` + `reqBody.ignored_files` + sanitized `name`, idempotency via `Get` before `Create`, disk-space `HasSpaceAvailable` check on `DecompressFile`; `restoreBackup` (`1691`) creates `pre-restore-*.zip` snapshot + serialized `backupMu` + rollback on failure. Panel side `handlers_backup_extended.go` + `services/backup` with `UpsertBackup`/`MarkBackupStatus` (`http/server.go:2223`). Chunked upload and `downloadBackupWithToken` (`server.go:1642`) with `ScopeBackupDownload`. | **Parity + exceeds** — Adds S3 encryption/compression knobs, retention, scheduler, verified restore journal (`store.go`), metrics. |
| 12 | **Installer / bootstrap** | Wings `server/install.go` + `installer`: runs `install_script` inside Docker container `installContainer` with `installEntrypoint`, streams logs, binds server dir. `POST /install`, `POST /reinstall`, `GET /install-logs` (Pelican). | Beacon `install` (`server.go:1044`) + `installWS` (`1151`): validates `serverId` vs path, `os.MkdirAll(root/installDir)` with `os.Chown 1000:1000` when `geteuid==0` (`1088`), writes `install.sh 0750`, `BeginInstall`/`EndInstall` fencing (`manager BeginInstall 271`) rejecting concurrent installs/suspended servers, delegates `runtime.Install` (`InstallRequest` with `ServerID/Image/Entrypoint/Script/Env/RootDir`), `notifyPanelInstallStatus` (`1338`) via `remote.Client`, WebSocket variant enforces **admin scope only** (`claims.Scope!=ScopeAdmin→403` `1170`) and `defer EndInstall(!success)`. Reinstall guards `PowerStateRunning/Starting` (`1323`). | **Parity + exceeds** — Ownership fix, fencing, dual HTTP/WS + admin-scope WS gate are Beacon extras. |
| 13 | **Transfers / migrations** | Wings: **daemon-to-daemon** `POST /api/transfers` (`router/router.go:59` `postTransfers`) accepting panel-issued JWT, plus `POST /transfer`, `DELETE /transfer`, panel-managed system transfers `router/router_transfer.go` (`POST /api/transfers`, `DELETE /api/transfers/:server`). Ships master token to target-defined URL and extracts into live root — flagged retired in Beacon comment. | Beacon: `POST /api/v1/transfers/credentials` (`server.go:375` `registerTransferCredential`) + source/destination v1 endpoints (`server.go:387-396` `prepareTransferSource/pushTransferSource/.../restoreTransferDestination` in `transfer_protocol.go`). Engine `transfer.Engine` (`beacon/internal/transfer/protocol.go` `ProtocolVersion="forge-beacon-transfer/v1"`, `Register/Authorize` with `HashCredential` SHA-256, binding validation, TTL, `ConsumedAt`, `ArchiveSize/Offset/Checksum`). Panel `daemon.Client.RegisterTransferCredential/PrepareTransferSource/...` (`forge/api/internal/daemon/client.go:574`) sign via HMAC; panel `MigrationService` owns lifecycle — `legacyServerTransferCallbackUnavailable` (`http/server.go:2218`) explicitly disables Wings-style callbacks. `crossnode.Resolver` (`resolver.go:1`) and migration creds scoped per-direction (`DirectionSourceControl`/`DirectionDestinationUpload`). | **Intentional divergent design** — Beacon retired Wings' direct JWT transfer;Forge uses control-plane-mediated v1. Not a parity loss. |
| 14 | **Metrics** | Wings: no `/metrics` before daemon rewrite; `GET /api/system/utilization` ad-hoc JSON via `server/resources.go`. Pelican adds similar. Prometheus not wired. | Beacon `GET /metrics` (`server.go:666`): `text/plain v0.0.4` exposition, per-server `CPU/Memory/Rx/Tx` with labels `server_id`, `DefaultGatherer` embedded, `CollectProcess` CPU/goroutines/GC. Access: loopback bypass or `Bearer metricsToken` (`authenticate 3366` `isLoopbackRequest`). Panel `handlers_metrics.go` + `observability.RecordNodeMetric` sampling 1/6 heartbeats (`http/server.go:1660`), `metrics_collector.go`. | **Exceeds** — Wings has no Prometheus surface. |
| 15 | **Runtime isolation** | Wings: Docker only (single `internal/remote` docker adapter). `config/docker.go` `network` settings. Buildpack not present. | Beacon `runtime.Runtime` (`beacon/internal/runtime/runtime.go:1`) interface with `Create/Install/Inspect/List/Start/.../AttachConsole/Delete` + `Pinger` + `ConsoleSession`; implementations: `docker.go` (full), `containerd.go`, `podman.go`, `firecracker.go`, `kubernetes.go` (`runtime/*.go`); `factory.go` `NewFactory(RuntimeConfig).CreateRuntime`; panel `forge/api/internal/runtime` maps to `Daemon.Client`-backed providers (`containerd/docker/firecracker/kubernetesadapter.go`) + `capabilities.go`. Env `DAEMON_RUNTIME_PROVIDER` (`beacon/cmd/daemon/main.go`). Firecracker `FirecrackerConfig`, Podman `PodmanConfig`, K8s `KubernetesConfig`. Mock `Unavailable` runtime for tests. | **Exceeds** — Multi-runtime is net-new. |
| 16 | **Daemon auth model** | Wings: `Authorization: Bearer <token>` (`RequireAuthorization`) validated against node `daemon_token`; optional `Authorization` on WS via JWT; unsigned `/download/*` via JWT `tokens`. | Beacon `authenticate` (`server.go:3366`): (a) `/health`→public, `/metrics`→loopback or `Bearer metricsToken`, `isScopedTokenRoute`/`/download/backup` passthrough; (b) otherwise `X-Panel-Timestamp/Nonce/Signature` HMAC over `METHOD\nURI\nTimestamp\nNonce\nBody` keyed with `nodeToken` (`signedHeaders` `daemon/client.go:119`), `validRequestNonce 32-hex`, `maxSeenNonces 4096` + 5-min skew + replay (`acceptRequestNonce` `3448`). `daemon.Client` resigns on retry (`resignRequest`). Panel beacon→panel uses dual: `Bearer nodeToken` + `VerifyRemoteHMAC` (`http/server.go:263` `nodeHeartbeatNonces` Redis-backed `SetRedis`). | **Hardened** — Replay/nonce, dual-auth intent (`Dual-auth model (intentional)` comment `handlers_remote.go:1`), and metrics-token separation have no Wings equivalent. |
| 17 | **Node heartbeat history & scoring** | Not applicable. | Store: `CreateNodeHeartbeatHistory`, `ListNodeHeartbeatHistory(limit)` (`store_heartbeat.go`), `store_nodes.go` heartbeat path used by `heartbeatmonitor` to compute `successfulHeartbeats`. `Noderegistry.HealthScore` (`noderegistry/service.go:196`) composites `cpu/memory/disk/heartbeat/status` each 0-100, total avg. `heartbeatScoreAge` thresholds 2m→100,5m→75,15m→40, else 0; `resourceScore(total,available)` clamps `used<0→0→100`. Exposed via `GET /api/v1/nodes/:id/heartbeat/history` and capability endpoints. | **Exceeds** — Wings has no history. |
| 18 | **Cross-node / service discovery** | Wings: none (single-node panel). | Panel `crossnode.Resolver.ResolveTargetHost` (`crossnode/resolver.go:27`) with `30s` cache + `ClearCache/SetCacheTTL`, `servicediscovery.Service` (`ResolveAll` healthy-only), `ListEndpoints` per-node fallback, `health_filter.go` + `ingress_sync.go` + `routegroup.go` for LB / failover. | **Exceeds** — Cluster-aware routing is Forge-only. |

---

## 4. Forge Route Inventory (authoritative)

`beacon/internal/server/server.go:379` onward, in registration order:

```
/health, /ready, /metrics
POST /servers (+ syncConfiguration subresource)
POST /servers/{id}/sync  → syncConfiguration
GET  /servers/{id}/config → getConfiguration
POST /servers/{id}/install (+ WS /ws)
POST /servers/{id}/reinstall
POST /servers/{id}/power  (+ OperationQueue + Journal .beacon/journal/operations.db)
GET  /servers/{id}/operations (+ /operations/{id})
DELETE /servers/{id}
GET  /servers/{id}/stats (+ WS), GET /logs (+ WS), ws/console, ws/backup, POST /command, ws/install
POST /servers/{id}/backups (+ ListBackups), GET/POST backup download/restore/delete
FILES per-server: GET /files/list, POST /mkdir|/remove|/rename|/copy|/chmod|/upload, GET /download, GET /content, PUT /content, PUT /upload (chunked), POST /archive|/decompress|/pull
POST /api/v1/transfers/credentials (+ source/destination phase endpoints + DELETE /api/v1/transfers/{id})
GET  /v1/files/list|/read|/write|/mkdir|/remove|/rename|/copy|/chmod|/upload|/download
GET  /api/system, GET/GET delta/POST /api/capabilities, POST /api/enroll (+ status), POST /api/update, POST /api/deauthorize-user, GET /download/backup (+ token), /compose/* 7, /git/* 3, /database/* 7, /build/* 6, /image/* 3, /api/edge/* 3, /api/diagnostics* 3, /api/version, /api/upgrade* 4, /api/commands/* 3 + pending, /api/admin/* 20+, /v1/host/* 5, /v1/firewall/* 9
```

Wings parity mapping: every top-level Wings group has a Beacon counterpart (see table row 2/7). Extra Beacon groups have no Wings analog and are additive.

---

## 5. UI → API → Beacon Traces

### 5.1 File Operations

```
Web (forge/web)  ──POST /api/v1/servers/:id/files/rename──▶  Forge API (fiber)
       │  file list/write/read/download-ticket flows pass requireServerPermission(PermFile*), validation, audit append
       ▼
Forge API ──daemon.Client.fileJSONMutation/fileJSONPayloadMutation──▶ Beacon
       │  HMAC signed: X-Panel-Timestamp/Nonce/Signature + X-Beacon-Version
       ▼
Beacon authenticate(3366) → de-dupe nonce → ServerExists → s.serverFilesystem(serverID,false) → rootfs.FS (openat2 beneath)
       │
       ├─ listFiles(2103)        → fsys.ReadDir → JSON (FileEntry with MIME/size)
       ├─ renameFile(2407)       → fsys.Rename guarded by IsIgnored/denylist
       ├─ batchRenameFiles(2568) → targets [] + errgroup-like serialization under backupMu
       ├─ chmodFiles(2637)       → fsys.Chmod(FileMode(parseOctal))
       ├─ writeFile/readFile(2243/2217) + uploadFileChunk(2285) → staged .uploads/*.part with lockUpload + uploadExpiry cleanup loop(131) + HasSpaceForWrite check
       ├─ archiveFiles/decompressFile(2456/2491) → archiveTree / extractZipStaged with archivePathTracker+validateArchiveName+maxArchiveBytes/Entries
       └─ pullRemoteFile(2715)   → securePullClient(pinnedResolver) + restrictedIP + maxPullBytes + SpaceAvailableForDecompression

Wings equivalent: router/router_server_files.go:* flows directly without the dual HMAC layer or chunked upload .part journal.
```

Evidence: `forge/api/internal/daemon/client.go:1241` `fileJSONMutation`, `1225` `CopyFile`, `1233` `ChmodFile`, `1242` `fileJSONPayloadMutation`, `1202` `UploadFileChunk`; `beacon/internal/server/secure_files.go:37` `lockUpload`, `160` `archivePathTracker`, `545` `restrictedIP`, `597` `securePullClient`.

### 5.2 Console (WS)

```
Web WS ticket issuance:  POST /api/v1/servers/:id/ws/ticket ──requireServerPermission(PermControlConsole)──▶  wsTickets.issue({serverId, userId, stream:console, expires 15m})
       │
Web ──GET /api/v1/servers/:id/ws/console?ticket=…──▶ Forge API realtimeProxy(cfg,wsTickets,"console") ──CSRF-immune GET via wsOriginMiddleware──▶ daemon.Client.WebSocketURL(baseURL,serverID,"console") + ticket forwarding ──signed upgrade──▶ Beacon
       ▼
Beacon consoleWS(1954): authenticateWebSocket → ScopeWebsocket → trackWebSocket → consoles.Ensure(serverID)=AttachConsole → Subscribe(serverID) replay 128/256KiB → fan-out goroutines:
       client→daemon: ReadMessage → consoles.Write(serverID,cmd) → runtime.SendCommand
       daemon→client: ch (output) → writer.WriteJSON({type:output,data:…})
       + pingWebSocket + sessionRegistry deauthorize (“POST /api/deauthorize-user” evicts wsTickets+SFTP)

Wings equivalent: GET /api/servers/:server/ws (router/router.go:54) with ServerExists + token parsing; no ticket layer, no bounded replay, no admin-vs-websocket scope split.
```

Evidence: `forge/api/internal/http/server.go:1975` ws routes, `ws_hub.go`/`ws_ticket.go`/`handlers_ws_ticket.go`; `beacon/internal/server/server.go:1954` `consoleWS`, `647` `trackWebSocket`, `console.go:1` `ConsoleThrottle` & `consoleManager`.

### 5.3 Backup

```
Web ──POST /api/v1/servers/:id/backups──▶ Forge API ──check limits, enqueue──▶ daemon.Client.CreateBackup(ctx,baseURL,nodeToken,serverID,ignoredFiles,name)
       │  ctx with Idempotency-Key backup:create:serverID:name  → newRequest + resign
       ▼
Beacon createBackup(1495): serverFilesystem + .pteroignore load + reqBody.ignored_files merge + sanitizeBackupName → Idempotency: Backups.Get(name) if exists return 200 → BackupInterface.Create(serverRoot,serverID,name,ignored) under backupMu + SetProgressCallback→eventBus BackupProgressEvent
       │  ──async──▶ panelClient.SendBackupStatus(BackupStatusRequest{backup_uuid,size,checksum,successful}) → POST /api/remote/servers/:id/backups/status → UpsertBackup/MarkBackupStatus
       └──────────────▶ DB backup record (size/checksum/status/completed_at)

Restore: POST /api/v1/servers/:id/backups/:name/restore ──▶ daemon.Client.RestoreBackup(name,truncate,commandID backup:restore:…) ──▶ Beacon restoreBackup(1691): pre-restore snapshot pre-restore-*.zip + backupMu → Restore with paths/truncate → on error rollback Restore(pre-restore snapshot,truncate=true)
Download: Web ──POST /servers/:id/backups/download-ticket──▶ issue token (ScopeBackupDownload, serverId+backupId) ──▶ Beacon GET /download/backup?token=… → downloadBackupWithToken(1642) Validate → Download(serverId,name)

Wings equivalent: router/router_server_backup.go: POST /backup etc. direct; no panel async status callback, no pre-restore snapshot, no progress WS.
```

Evidence: `forge/api/internal/daemon/client.go:919` `CreateBackup`, `1003` `RestoreBackup`; `beacon/internal/server/server.go:1495/1691/1642/2048` `backupProgressWS`, `beacon/internal/backup/backup.go:1` `BackupProgress`.

### 5.4 Transfer / Migration

```
Forge Web ──POST /api/v1/migrations (MigrationService.Create)──▶ Allocates TransferCredentialRegistration{ claims{version,migrationId,serverId,sourceNodeId,targetNodeId,direction,expiresAt}, credentialHash}
       │
       ├─▶ daemon.Client.RegisterTransferCredential(sourceBaseURL, cred) → Beacon POST /api/v1/transfers/credentials + HMAC → Engine.Register(reg) (protocol.go: HASH check, binding validation, TTL)
       └─▶ daemon.Client.RegisterTransferCredential(targetBaseURL, cred) → Engine.Register(target)

Data plane (per TransferEngine, no panel hop for bytes):
  source: PrepareTransferSource → PushTransferSource(offset,chunk,checksum)  →  target: ReceiveTransferChunk(HEAD offset/PATCH chunk) → RestoreTransferDestination → FinalizeTransferDestination
  Panel polls: GET /source/status, cleans up: POST /source/cleanup, DELETE /{id} cancels (server.go:387-396 + daemon/client.go:574-625). Bytes validated via OffsetMismatch/ChecksumMismatch/Bounds.

Wings equivalent: POST /api/transfers (router/router.go:59) handled in router/router_transfer.go as daemon→daemon JWT; panel posts archive hash to target URL directly. Beacon comment server.go:360 marks this “Legacy transfer endpoints removed …”.
```

Evidence: `beacon/internal/transfer/protocol.go:1` `ProtocolVersion`, `Engine.Register/Authorize`, errors `ErrOffsetMismatch`/`ErrChecksumMismatch`/`ErrTransferBounds`; `beacon/internal/server/transfer_protocol.go`; `forge/api/internal/services/migration` (referenced via `registerMigrationRoutes`) vs `daemon/client.go:574` `transferJSON`.

---

## 6. PufferPanel Note

`reference/game-hosting/pufferpanel` (`client.go`, `operation.go`, `server.go`, `sftp/*.go`, `files/*.go`) exposes a panel-owned daemon (REST + WebSocket) that **is not a Wings family member**. It uses `engine.go`/`files` directly on host paths, SFTP via Go `pkg/sftp`, and a `task.go` operation queue. There is no per-node Wings-style `daemon_token` or `GET /api/system` contract; comparisons to Forge are therefore category errors and PufferPanel is treated as an out-of-scope alternative implementation rather than a baseline for parity.

---

## 7. Logic / Correctness Findings (≥3 required)

### LF-1  [Medium] Resource/health scoring treats overcommit as perfectly healthy

**Location:** `forge/api/internal/services/noderegistry/service.go:302` `resourceScore` + `heartbeatScoreAge` (`330`) + `HealthScore` (`196`)

```go
used := total - available
if used < 0 { used = 0 }               // overcommit → used=0 → score=100
score := 100 - ((used*100)/total)
```

When `available > total` (memory/disk/cpu overcommitted — allowed by `MemoryOverallocate`/`DiskOverallocate` in `store_nodes.go:1219`), the node scores **100** for that resource, and `Total = (CPU+Mem+Disk+Heartbeat+Status)/5` is inflated. The scheduler’s `PlacementEligibility` (`208`) gates only on `AvailableCPU<=0` etc., so an overcommitted but under-pressured node and a correctly-utilized node are scored identically, defeating `HealthScore`-driven ranking. `heartbeatScoreAge`’s 40-point plateau for 5–15 min is similarly coarse and never negative.

**Impact:** placement and dashboard health mislead; overcommit silently optimizes rather than penalizes.

**Expected:** clamp `available = min(available,total)` before scoring, or penalize `overcommit = available-total` (e.g., `score = max(0, 100 - overcommit*weight)`), and smooth heartbeat decay (linear in `age`).

**Repro:** set `TotalMemory=16 GiB`, `AvailableMemory=20 GiB`, `AvailableCPU> TotalCPU`; call `HealthScore` → all resource legs 100 regardless of overcommit ratio.

---

### LF-2  [Low] Dev-version bypass for heartbeat `X-Beacon-Version` is substring-fragile

**Location:** `forge/api/internal/http/server.go:1621` + `beacon/internal/remote/client.go: header X-Beacon-Version`

```go
isDevVersion := beaconVersion == "beacon-dev" || strings.Contains(beaconVersion,"-dev") || strings.Contains(beaconVersion,"-test") || strings.Contains(beaconVersion,"dev-")
```

`strings.Contains(...,"dev-")` matches production semvers containing `dev-` anywhere (e.g., `1.2.3-dev-1` intended as release, or a vendor fork `acme-device-manager-1.0.0`), wrongly bypassing `daemon.CheckBeaconVersionCompatibility`. Conversely, a real dev build without `-dev`/`-test`/`dev-` in the string (e.g., `beacon-nightly-abc`) is incorrectly rejected.

**Impact:** compatibility gate is either overly permissive or overly strict; CI with custom prefixes can drift past enforcement.

**Expected:** match exact allowlist of build channels (`beacon-dev`, `*-dev`, `*-nightly`, `*-test`) via suffix/prefix rules, or gate on `APP_ENV!="production"` only, not string sniffing.

---

### LF-3  [Medium] Heartbeat history window truncation + unsorted history assumption can mis-classify recovery

**Locations:** `forge/api/internal/services/heartbeatmonitor/service.go:205` `evaluate`, `history, err := s.store.ListNodeHeartbeatHistory(ctx, node.ID, s.config.RecoveryThreshold+3)`; `sort.SliceStable(history[i].ObservedAt.After(...))`; `consecutiveSuccessfulHeartbeats` (`388`) breaking on first `!Success`; `classify` (`266`) using `history[0].Success` guard.

`ListNodeHeartbeatHistory(..., RecoveryThreshold+3)` (default `2+3=5`) truncates history to 5 entries. `classify` then counts `consecutiveSuccessfulHeartbeats(history)` assuming most-recent-first order (index 0 newest). Two issues:

1. **Truncation starves recovery:** `RecoveryThreshold=2` requires 2 successes, but if the last 2 of the 5 are successes preceded by 3 failures, `consecutiveSuccessfulHeartbeats` counts correctly; however if the node flapped `success,fail,success,success` within 5 entries, classification can mis-fire. With a longer `RecoveryThreshold` (custom config up to e.g. 5), limit `+3 = 8` is still small under high write frequency; under replicated placement churn the newest 8 may all be `failure` even when older successes would have completed recovery, keeping the node `recovering` indefinitely. Conversely the `len(history)>0 && !history[0].Success && age<OfflineThreshold → suspected` line (`classify:280`) means a single stale `failure` entry at index 0 overrides age-based health even when newer in-flight heartbeats haven’t yet been persisted (race with `UpdateNodeHeartbeat` vs `ListNodeHeartbeatHistory`).

2. **Sort + truncation interplay:** `evaluate` sorts the returned 5 entries every call, but `store.ListNodeHeartbeatHistory` already `ORDER BY observed_at DESC` at the DB; sorting is redundant but hides the fact that truncation happened pre-sort. Under pagination jitter, the window can skip the row containing the true newest timestamp, yielding an `ageSeconds` computed from `node.LastSeenAt` (separate column, updated by `UpdateNodeHeartbeat`) that disagrees with the history window’s recency.

**Evidence:** `store_heartbeat_test.go` only tests synthetic sorted histories; no test covers DB truncation vs `RecoveryThreshold>limit`.

**Expected:** make history limit `max(RecoveryThreshold*3, 20)` or fetch `WHERE observed_at >= now()-OfflineThreshold-UnavailableAfter`; and compute `consecutiveSuccessfulHeartbeats` on DB-ordered result without re-sorting; align `LastSeenAt` validation with `history[0].ObservedAt` (reject when drift >90s).

---

### LF-4  [Info] Backup pre-restore snapshot failure abandons the restore with the live FS already exported

**Location:** `beacon/internal/server/server.go:1722-1744` `restoreBackup`

```go
_, snapshotErr := s.backups.Create(..., rollbackName, nil)
if snapshotErr == nil { err = s.backups.Restore(...) }
...
if snapshotErr != nil { http.Error "pre-restore snapshot failed" }
```

If `Create(rollbackName)` fails (disk full, S3 token expired), the handler returns 500 **without** touching `backupMu`-protected restore, which is correct. However the backup directory is still exported to the caller as if the operation never happened, yet `panelClient.SendRestoreStatus(Successful:false)` is only sent in the `err!=nil` branch after a successful snapshot. A client retry with the same `name` will re-enter the `Create` attempt indefinitely until disk is freed; there is no `429`/retry-after or storage-health hint, and `BackupManager.Create`’s histogram `(RecordBackupDuration)` is not observed for the failed snapshot, hiding the failure in metrics.

**Not a data-loss bug, but an operator-visibility gap.** Minor severity; kept for completeness.

---

### LF-5  [Info] SFTP quotaWriter permits sparse-file quota escape via `WriteAt` with gaps

**Location:** `beacon/internal/sftpserver/server.go:620` `maxFinal = h.quotaBytes - usage + oldSize` and `quotaWriter.WriteAt` (`652`) checking `off + len(p) > w.max`

`quotaWriter` caps the **final logical size** per open, but SFTP `WriteAt` allows writing at offset beyond current size (sparse/hole). An `oldSize` computed at open time plus a sparse write past `maxFinal` is still rejected, but two concurrent SFTP sessions on the same server each compute `usage` independently (outside a shared quota lock) — the panel-reported `usage` can go stale between `Filewrite` calls, and the per-handler `diskUsageBytes` check happens before `WriteAt`, not atomically with it. Under burst, the sum of two sparse regions across sessions can exceed `quotaBytes` before the per-file close reconciles usage.

**Matches Wings behavior (same race)**, so not a Forge regression; logged as info because the `writeLock` (`sftpserver/handler:414`) serializes **per-file** writes but not **cross-file** quota accounting, same as Wings `disk_space.go`.

---

## 8. Parity Verdict & Recommendation

| Category | Verdict |
|----------|---------|
| **Node lifecycle** | Exceeds Wings — enrollment/bcrypt, token rotation, heartbeat push+history+monitor integration are additive and coherent. |
| **System / metrics** | Exceeds — Prometheus, capabilities (+history), diagnostics, utilization, Docker disk, IPs have explicit parity tests. |
| **File ops** | Parity met — 13 Wings ops all present; Beacon adds chunked upload journal, batch ops, host-file allowlist/denylist. Kernel `RESOLVE_BENEATH` defense is the standout hardening delta. |
| **Console** | Parity met — Bounded replay, scope-separated install WS, deauthorization eviction are improvements over Wings single-channel throttle. |
| **SFTP** | Parity met — Quota/idle/rate-limit/per-user caps tighten Wings’ unconstrained handler. |
| **Backups** | Parity met — `.pteroignore` + ignored_files contract honored; pre-restore snapshot+rollback and progress WS are improvements. |
| **Installer** | Parity met — Chown fix and `BeginInstall` fencing close a Wings TOCTOU on concurrent installs. |
| **Transfers** | **Intentional divergence** — Wings daemon→daemon JWT transfer retired (`server.go:360` comment); Forge uses control-plane-mediated `forge-beacon-transfer/v1` dual credentials. Not a gap; migration service should be considered authoritative. |
| **Runtime isolation** | Exceeds — Multi-runtime (docker/podman/containerd/firecracker/k8s) vs Wings docker-only. |

**Overall:** Forge has **no material Wings parity gaps**; every Wings daemon capability maps to a Beacon counterpart. The 4 logic/info issues above are soft correctness/visibility concerns, not missing-feature gaps. The two areas worth teeing up before GA are: (a) tightening LF-2’s dev bypass to suffix/prefix allowlist, and (b) expanding LF-3’s heartbeat history window and aligning `LastSeenAt` vs `ObservedAt` to prevent recovery stalling.

---

## 9. File Reference Index (for `file_path:line_number` navigation)

- `beacon/internal/server/server.go:239` `NewServerWithBackup`
- `beacon/internal/server/server.go:330` route table start, `379` host/transfer globals, `360` legacy-transfer retirement note
- `beacon/internal/server/server.go:647` `trackWebSocket`, `666` `metrics`, `1495` `createBackup`, `1642` `downloadBackupWithToken`, `1691` `restoreBackup`, `1954` `consoleWS`, `2048` `backupProgressWS`, `3366` `authenticate`, `3448` `acceptRequestNonce`, `3504` `authenticateWebSocket`
- `beacon/internal/server/secure_files.go:37` `lockUpload`, `60` `serverFilesystem`, `160` `archivePathTracker`, `545` `restrictedIP`, `597` `securePullClient`
- `beacon/internal/server/hostfiles.go:1` `validateHostPath`/`resolveHostPath`/`SetHostFileAllowlist`
- `beacon/internal/server/manager.go:103` `NewServerManager`, `271` `BeginInstall`, `482` `HandlePower`, `776` `HasSpaceForWrite`
- `beacon/internal/server/capabilities.go:1` `CapabilityReport`/`collectCapabilities`/`handleGetCapabilities`
- `beacon/internal/server/console.go:1` `ConsoleThrottle`/`consoleManager`/`Subscribe`
- `beacon/internal/runtime/runtime.go:1` `Runtime` interface, `184` `Stats`; `runtime/factory.go:53` `NewFactory`; `docker.go`, `containerd.go`, `podman.go`, `firecracker.go`, `kubernetes.go`
- `beacon/internal/backup/backup.go:1` `BackupInterface`, `local.go`, `s3.go`, `store.go`, `scheduler.go`
- `beacon/internal/sftpserver/server.go:62` `Config`, `86` `Run`, `207` `passwordCallback`, `414` `handler`, `518` `idleConn`, `643` `quotaWriter`
- `beacon/internal/transfer/protocol.go:1` `ProtocolVersion`/`Register`/`Authorize`/`HashCredential`
- `beacon/internal/system/fs_linux.go:1` `IsOpenat2Supported`/`SafeOpen`
- `beacon/internal/metrics/metrics.go:1` `PrometheusCollector`
- `beacon/cmd/daemon/main.go:730` `heartbeatLoop`, `757` `SendNodeHeartbeat` call, `743` `NodeHeartbeat` construction
- `beacon/internal/remote/client.go:54` base URL derivation, `272` `SendNodeHeartbeat`, `101` `NodeHeartbeat` type
- `forge/api/internal/daemon/client.go:119` `SetDefaultNode`, `124` `Get/Post`, `174` `retryRoundTripper`, `288` `isLoopback`, `574` `RegisterTransferCredential`, `919` `CreateBackup`, `1202` `UploadFileChunk`, `1241` `fileJSONMutation`
- `forge/api/internal/store/store.go:142` `Node`, `381` `NodeHeartbeatRequest`, `1479` `NodeHeartbeatState`, `1603` `NodeHeartbeatHistory`
- `forge/api/internal/store/store_nodes.go:727` `UpdateNodeHeartbeat`, `1045` `NodeConfiguration`, `1321` `ListServersForNode`
- `forge/api/internal/store/store_capabilities.go:76` `CreateOnboardingToken`, `239` `UpsertNodeCapability`, `408` `GetCapabilityHistory`
- `forge/api/internal/http/server.go:262` `nodeHeartbeatNonces`, `283` `VerifyRemoteHMAC`, `905` `NodeHeartbeatRequest` (HTTP), `1593` `POST /nodes/:id/heartbeat`, `1694` `POST /nodes/capabilities`
- `forge/api/internal/services/heartbeatmonitor/service.go:89` `DefaultConfig`, `205` `evaluate`, `266` `classify`, `315` `publishTransitions`
- `forge/api/internal/services/noderegistry/service.go:47` `RegisterNode`, `130` `RecordHeartbeat`, `196` `HealthScore`, `208` `PlacementEligibility`
- `forge/api/internal/services/nodeprobe/service.go:1` `ProbeNode`
- `forge/api/internal/services/clustermanager/service.go:1` `CreateServer`/`compensateCreateFailure`
- `forge/api/internal/services/crossnode/resolver.go:27` `ResolveTargetHost`, `101` `resolveFromDiscovery`
- `reference/game-hosting/pterodactyl-wings/router/router.go:19` `Configure`, `54` `GET /ws`, `59` `POST /transfers`, `70` `/files`
- `reference/game-hosting/pelican-wings/router/router.go:67` `diagnostics`/`docker/disk`/`utilization` extensions, `114` `GET /search`
- `reference/game-hosting/pterodactyl-wings/router/router_server_files.go:1` file handlers; `server/filesystem/filesystem.go` `Filesystem`; `sftp/server.go:1` `SFTPServer`/`Run`; `server/console.go:59` `Allow`

---

## 10. Appendix — Beacon vs Wings Route Crosswalk (file ops detail)

| Operation | Wings | Beacon |
|-----------|-------|--------|
| List directory | `GET /list-directory` (`router_server_files.go:78`) | `GET /files/list` (`server.go:360` `listFiles`) |
| Get file contents | `GET /contents?file=` (`30`) | `GET /files/content?path=` |
| Download (stream) | `GET /download/file` (signed) | `GET /files/download?path=` |
| Write small file | `POST /write?file=` (`231`) | `PUT /files/content?path=` + streaming `fileJSONPayload` |
| Chunked upload | n/a (multipart only `/upload/file`) | `PUT /files/upload?uploadId=&offset=&final=` (`2285`) |
| Delete | `POST /delete {root,files}` (`186`) | `POST /files/remove` + `batchDeleteFiles` |
| Rename/Move | `PUT /rename {root,files:[from,to]}` (`94`) | `POST /files/rename` + `batchRenameFiles` |
| Copy | `POST /copy {location}` (`162`) | `POST /files/copy` |
| Chmod | `POST /chmod {root,files:[file,mode]}` (`504`) | `POST /files/chmod` |
| Create dir | `POST /create-directory {name,path}` (`388`) | `POST /files/mkdir?path=` |
| Compress | `POST /compress {root,files}` (`415`) | `POST /files/archive` |
| Decompress | `POST /decompress {root,file}` (`456`) | `POST /files/decompress` |
| Pull remote | `POST /pull {root,url,file_name}` (`277`) + `GET/DELETE /pull` | `POST /files/pull` + `securePullClient` |
| Search | `GET /search` (Pelican only) | n/a — gap is Pelican-only additive; Forge hostfiles `list?path=` + panel search covers; not a regression |
| Host file list/read/write | n/a | `GET/POST /v1/files/list|read|write|mkdir|…` (host allowlist) |

Search parity note: Wings base lacks search; Pelican added it (`router/router_server_files_search.go`). Forge has no server-files search endpoint (panel-side filtering instead); acceptable given hostfiles extension.
