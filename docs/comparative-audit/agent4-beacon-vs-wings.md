# Comparative Audit: Beacon (GamePanel) vs Wings (Pterodactyl)

**Agent:** Agent 4 — Architecture & Systems Auditor  
**Date:** 2026-07-15  
**Scope:** Complete file-by-file audit of `beacon/` (55 Go source files) against `reference/wings/` (134 Go source files)

---

## Executive Summary

Beacon is a purpose-built, minimal Go daemon that implements the core Pterodactyl daemon contract for managing game servers via Docker containers, SFTP, backups, transfers, and panel communication. It achieves feature parity with Wings across all critical subsystems while using approximately 40% of the codebase size and zero heavy third-party web frameworks. Wings is a mature, production-hardened daemon with deeper Docker orchestration, richer configuration, a full cron subsystem, a database layer, and significant operational polish accumulated over years of community deployment.

| Metric | Beacon | Wings |
|--------|--------|-------|
| Total Go files | 55 | 134 |
| Approx. LoC (Go) | ~8,500 | ~22,000+ |
| Third-party deps | ~8 direct | ~25+ direct |
| HTTP framework | `net/http` stdlib | `gin-gonic/gin` |
| CLI framework | None (env-only) | `spf13/cobra` |
| Config format | YAML + env vars | YAML + env vars + CLI flags |
| Database | None | BoltDB (internal) |
| Logging | `log` stdlib | `apex/log` |
| Error library | stdlib `errors` + `fmt` | `emperror.dev/errors` |

---

## 1. Main Entry Point

### Beacon (`cmd/daemon/main.go`)

- **500 lines**, single `main()` function — no CLI framework
- All configuration via environment variables (`DAEMON_*`, `PANEL_API_URL`, `WINGS_*` compatibility aliases)
- Inline healthcheck mode: `--healthcheck` flag exits immediately with HTTP probe
- Inline server sync: `syncServersFromPanel()` and `recoverServersFromDisk()` for boot-time recovery
- Inline heartbeat loop with 30s ticker
- Graceful shutdown via `signal.Notify(SIGINT, SIGTERM)` → `httpServer.Shutdown()`
- Backup adapter factory: `buildBackupAdapter()` switches between local and S3 based on `BACKUP_ADAPTER` env

### Wings (`cmd/root.go` + `wings.go`)

- **472 lines** in `root.go` + minimal `wings.go` bootstrap
- Full Cobra CLI with `--config`, `--debug`, `--pprof`, `--auto-tls`, `--tls-hostname`, `--ignore-certificate-errors` flags
- `rootCmdRun()` orchestrates 12+ initialization steps in a defined order:
  1. Timezone configuration
  2. Directory creation
  3. System user creation (`pterodactyl` user)
  4. Passwd file generation
  5. Log rotation setup
  6. Panel client creation
  7. Database initialization
  8. Server manager creation
  9. Docker environment configuration
  10. Worker pool bootstrap (4 concurrent)
  11. Cron scheduler
  12. SFTP server
  13. HTTP server with TLS/autocert support
- Parallel server boot using `gammazero/workerpool` (4 workers)
- Persistent state via JSON file (`PersistStates`/`ReadStates`)

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Startup complexity | Simple, fast | Thorough, robust | Wings (production readiness) |
| CLI ergonomics | Env vars only | Cobra flags | Wings |
| Boot-time recovery | Disk cache fallback | State file + Docker inspect | Wings |
| Healthcheck | Built-in HTTP probe | Not built-in (needs external) | Beacon |
| Graceful shutdown | Yes | Yes | Tie |

---

## 2. Configuration

### Beacon (`config/config.go`)

- **113 lines**, flat `Configuration` struct
- Fields: `Debug`, `UUID`, `TokenID`, `Token`, `PanelURL`, `Remote`, `System`, `AllowedMounts`, `AllowedOrigins`, `RemoteQuery`, `Docker`, `CrashDetection`
- `SystemConfiguration`: `DataDirectory`, `TempDirectory`, `Sftp`, `API`
- Thread-safe singleton with `sync.RWMutex`
- Load/Save via `gopkg.in/yaml.v2`
- `Default()` factory function for programmatic defaults
- No Docker configuration beyond `Network.Interface` and `Timezone`

### Wings (`config/config.go` + `config_docker.go`)

- **832 lines** across two files — 7× the code
- Deeply nested: `SystemConfiguration` alone has 30+ fields covering:
  - Root/Data/Archive/Backup/Log/Tmp directories
  - User management (UID/GID/passwd/machine-id)
  - Disk check intervals, activity send intervals, permission checks
  - Log rotation, WebSocket log count
  - Crash detection with timeout
  - Backup write limits, compression levels
  - Transfer download limits
  - Openat2 mode selection
- `DockerConfiguration` (185 lines): Network (IPv4/IPv6 subnets, MTU, ICC, DNS), registries with base64 auth, tmpfs size, PID limits, installer limits, memory overhead multipliers, user namespace mode, log config
- `ConsoleThrottles`: configurable line/period limits
- `ApiConfiguration`: SSL, upload limits, trusted proxies
- Runtime helper: `EnsurePterodactylUser()`, `ConfigureDirectories()`, `EnableLogRotation()`, `ConfigurePasswd()`, `ConfigureTimezone()`
- Openat2 kernel capability detection

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Config depth | Minimal, env-driven | Exhaustive, file-driven | Wings |
| Docker config | Network name + timezone | Full network/registry/PID/overhead | Wings |
| Security config | Basic mounts/origins | TLS, user namespace, openat2 | Wings |
| Maintainability | Simple, auditable | Complex, feature-rich | Beacon |
| Runtime setup | Delegated to main() | Self-contained helpers | Wings |

---

## 3. Server Lifecycle Management

### Beacon (`server/server.go` + `server/manager.go`)

- **ServerManager** with `sync.Map` for concurrent server states
- **ServerState** struct: power state, installation state, memory, allocation, stop config, crash detection, suspension, disk limits
- **Reconcile()**: restores panel-returned servers, inspects Docker container state
- **HandlePower()**: `start`, `stop`, `restart`, `kill` — uses `TryLock()` to prevent concurrent actions
- **stopServer()**: supports command-based, signal-based (SIGTERM/SIGINT/SIGKILL), and timeout-based stops
- **onBeforeStart()**: pre-flight checks — installing, suspended, config synced, disk usage
- **syncServerStateFromPanel()**: fetches latest config from panel on each start
- **StartEventWatcher()**: listens to Docker events via `EventWatcher` interface
- **HandleContainerEvent()**: reacts to Docker `start`/`die` events
- **Chown on boot**: optional recursive chown before start
- **Disk usage**: `diskUsageBytes()` via `filepath.WalkDir`
- No persistent state file — relies on panel sync or disk cache on boot

### Wings (`server/server.go` + `server/manager.go` + `server/power.go` + `server/connections.go`)

- **Manager** with `[]*Server` slice and `sync.RWMutex`
- **Server** struct: embeds `sync.RWMutex`, has context/cancel, crash handler, environment, filesystem, event bus, WebSocket bag, SFTP context bag, atomic booleans for installing/transferring/restoring
- **State persistence**: `PersistStates()`/`ReadStates()` writes to disk JSON
- **Power actions**: `PowerActionStart`, `PowerActionStop`, `PowerActionKill`, `PowerActionRestart`
- **Console attachment**: `Attach()` hijacks Docker stream, scans output lines for startup completion
- **Crash detection**: `CrashHandler` with configurable timeout, auto-restart logic
- **Server sync**: `Sync()` fetches latest panel config, updates disk limits, process config
- **Environment abstraction**: `ProcessEnvironment` interface with Docker implementation
- **Worker pool**: 4 concurrent server boot with 30s timeout per server
- **Disk space**: `filesystem.DiskSpace()` with configurable check interval caching

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| State management | Panel-only + disk cache | Persistent state file + panel | Wings |
| Power control | TryLock mutex | Locker with full state machine | Wings |
| Crash detection | Basic cooldown | Full handler with configurable timeout | Wings |
| Startup detection | None (console only) | Output line matching (regex) | Wings |
| Event system | Docker event watcher | Docker events + environment events | Wings |
| Disk usage | Real-time walk | Cached with interval | Wings |

---

## 4. Docker Runtime

### Beacon (`runtime/docker.go` + `runtime/runtime.go`)

- **Runtime interface** (20 methods): `Create`, `Install`, `Inspect`, `List`, `Start`, `Stop`, `Kill`, `Signal`, `Restart`, `Stats`, `Logs`, `LogsStream`, `StatsStream`, `AttachConsole`, `Delete`, `SendCommand`, `WaitForStop`
- **Optional interfaces**: `Pinger`, `EventWatcher`, `ConsoleSession`
- **Container naming**: `mgp-{serverID}` prefix
- **Container root**: `/home/container`
- **Security defaults**: `CapDrop: ALL`, `Privileged: false`, `Init: true`, `ReadonlyRootfs: true`, `SecurityOpt: no-new-privileges:true`, tmpfs `/tmp` with exec
- **Image pull**: auto-pulls if not found locally
- **Config hash label**: `configHashLabel` — skips recreation if hash matches
- **Live update**: attempts `ContainerUpdate` for resource changes on running containers
- **Event watcher**: Docker event stream with exponential backoff reconnect
- **Console attach**: handles TTY vs non-TTY via `stdcopy.StdCopy`
- **Install**: creates temporary container with `--cap-drop ALL --read-only`, collects logs, removes after completion
- **Network**: single default network name from env

### Wings (`environment/docker/` — 5 files)

- **Environment struct**: wraps Docker client with hijacked response stream, stats stream, event bus
- **Container lifecycle**: Create, Start, Stop, Restart, Kill, Remove, Inspect
- **Power management** (`power.go`): StartStop, SendCommand (attach-based), WaitForStop with configurable signal
- **Docker API** (`api.go`): container creation with full resource spec, network management, image pulling
- **Network management**: auto-creates Docker network with configurable interface, subnet, MTU, ICC
- **Stats collection** (`stats.go`): polling-based with Docker API stats endpoint
- **Environment abstraction**: `ProcessEnvironment` interface allows future non-Docker implementations
- **Container configuration**: full mount, port, DNS, tmpfs, PID limit, memory overhead, user namespace
- **Shared Docker client**: `environment.Docker()` singleton with API version negotiation

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Runtime abstraction | Interface + single impl | Interface + Docker impl + env wrapper | Wings |
| Network management | Basic env-based | Full auto-creation with IPv4/IPv6 | Wings |
| Container security | Same defaults | Same + user namespace, passwd, machine-id | Wings |
| Image management | Auto-pull | Auto-pull + registry auth | Wings |
| Event handling | Backoff reconnect | Event bus integration | Tie |
| Live update | Resource-only update | Full Sync + config update | Wings |
| Complexity | ~500 lines | ~800+ lines across 5 files | Beacon |

---

## 5. Console & WebSocket

### Beacon (`server/console.go`)

- **consoleManager**: owns at most one live console session per server
- **Producer-subscriber model**: producer reads from `ConsoleSession`, fans out to subscriber channels
- **Replay buffer**: last 128 entries / 256KB — new subscribers receive history
- **Bounded delivery**: slow subscribers get drop-oldest ring-buffer behavior (10ms timeout)
- **Write support**: `Write()` sends commands with newline appending
- **Lifecycle**: `Ensure()` creates producer on demand, `Stop()` tears down cleanly
- **WebSocket endpoint**: `GET /servers/{id}/ws/console` — uses gorilla/websocket
- **Origin validation**: configurable via `DAEMON_WS_ALLOWED_ORIGINS` or auto-derived from `PANEL_API_URL`
- **Stats WebSocket**: `GET /servers/{id}/ws/stats`
- **Logs WebSocket**: `GET /servers/{id}/ws/logs`

### Wings

- **Server-level event bus**: `events.Bus` wraps `system.SinkPool`
- **Console output**: `PublishConsoleOutputFromDaemon()` formats with app name color
- **WebSocket connections**: `WebsocketBag` tracks UUID-keyed connections with cancel functions
- **Output scanning**: `system.ScanReader()` — 64KB max line buffer, carriage return normalization
- **Startup detection**: regex-based `OutputLineMatcher` for "server ready" signals
- **WebSocket server** (`router/websocket/`): full implementation with rate limiting, message types, listener management
- **Log replay**: configurable `WebsocketLogCount` (default 150 lines)

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Console model | Producer-subscriber | SinkPool + event bus | Tie |
| Replay buffer | 128 entries / 256KB | Configurable line count | Wings |
| Startup detection | None | Regex output matching | Wings |
| WebSocket origin | Configurable whitelist | Configurable + auto | Tie |
| Rate limiting | Console throttle | WebSocket-specific limiter | Wings |
| Simplicity | Clean, focused | Distributed across packages | Beacon |

---

## 6. File Management

### Beacon (`server/server_wings_extras.go` + `server/secure_files.go`)

- **RootFS**: custom `rootfs.FS` with Linux `openat2` syscall for traversal protection
  - `RESOLVE_BENEATH | RESOLVE_NO_MAGICLINKS | RESOLVE_NO_SYMLINKS`
  - Atomic writes via temp file + rename
  - Platform fallback for non-Linux
- **File operations**: list, delete, mkdir, rename, archive, decompress, batch delete, batch rename, chmod, copy, pull, download, read, write, upload chunk
- **Archive extraction**: staged extraction with validation, rollback on failure
- **ZIP/TAR support**: `validateZip()` and `validateTar()` with size/entry limits
- **Path validation**: `rootfs.Clean()` rejects `..`, absolute paths, null bytes, drive prefixes
- **Disk quota**: `HasSpaceForWrite()` / `HasSpaceForWriteFS()` checked before writes
- **Pull downloads**: `securePullClient()` with pinned DNS resolver, no-redirect-to-private-IPs

### Wings (`server/filesystem/` — 10+ files)

- **Filesystem struct**: wraps `rootfs.FS` (or openat2), disk space tracking with interval caching
- **Archive creation**: `mholt/archives` library for tar.gz
- **Compression**: configurable gzip level (none/speed/compression)
- **Disk space**: `DiskSpace()` with configurable `DiskCheckInterval` caching
- **File denylist**: per-egg file deny patterns
- **Permissions**: `Chown()`, `Touch()` with proper ownership
- **Transfer archive**: streaming tar.gz with progress tracking
- **Path validation**: openat2 with `RESOLVE_BENEATH`
- **UFS layer** (`internal/ufs/`): full Unix filesystem abstraction with quota writer, walk, stat, mkdir, removeall

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Path security | openat2 + validation | openat2 + UFS abstraction | Wings |
| Archive support | ZIP + TAR.GZ | TAR.GZ (via archives lib) | Beacon |
| Disk quota | Real-time walk | Cached interval | Wings |
| Atomic writes | RootFS atomic | UFS atomic | Tie |
| Compression config | None (always deflate) | Configurable gzip level | Wings |
| Error recovery | Staged + rollback | Direct extraction | Beacon |

---

## 7. SFTP

### Beacon (`sftpserver/server.go`)

- **Self-contained**: 700+ lines in single file
- **Native SSH server**: `golang.org/x/crypto/ssh` + `github.com/pkg/sftp`
- **Authentication**: delegates to panel `/api/remote/sftp/auth` with username/password or public key
- **Username validation**: regex `^(?i)(.+)\.([a-z0-9-]{8,36})$` — fast-fail before API call
- **Connection limits**: configurable `MaxConnections` (128) and `MaxSessionsPerUser` (8)
- **Idle timeout**: configurable per-connection deadline
- **Host key**: auto-generates ED25519 key, persists to `.sftp/id_ed25519`
- **File operations**: all via `rootfs.FS` — Fileread, Filewrite, Filecmd, FileList
- **Disk quota**: per-write quota check with `quotaWriter` wrapper
- **Write locking**: per-server `sync.Mutex` to serialize writes
- **Activity logging**: `ActivityDedup` with detailed event recording
- **Session tracking**: integrated with `sessionRegistry` for deauthorization
- **SSH hardening**: `MaxAuthTries: 6`, restricted ciphers/MACs in config

### Wings (`sftp/` — 4 files)

- **SFTPServer struct**: manager-backed, config-driven
- **Authentication**: same panel delegation (`/api/remote/sftp/auth`)
- **Handler**: permission-aware, event-recording, filesystem-bound
- **Event handler**: SFTP activity events published to event bus
- **Context-based sessions**: per-user context bags for cancellation
- **SSH config**: explicit key exchange, cipher, and MAC algorithm lists
- **Host key**: auto-generates ED25519, persists to `data/.sftp/id_ed25519`

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Self-containedness | Single file, no manager dependency | Manager-integrated | Beacon |
| Connection limits | Configurable max + per-user | Not configurable | Beacon |
| Idle timeout | Configurable | Not configurable | Beacon |
| Activity dedup | Built-in batched dedup | Event bus push | Beacon |
| SSH hardening | Basic | Explicit algorithm lists | Wings |
| Permission model | Rootfs-based | Permission enum + filesystem | Wings |
| Error handling | Structured SSH error codes | Stack-traced errors | Wings |

---

## 8. Backup (Local + S3)

### Beacon (`backup/` — 4 files)

- **BackupInterface**: `Create`, `List`, `Get`, `Delete`, `Restore`, `Download`, `Type`
- **LocalBackup**: stores under `backupRoot/<namespace>/`, validates namespace/name
  - Atomic staging: writes to `.partial` then renames
  - `.pteroignore` support
  - SHA-256 checksums
  - Restore with truncate (staged rollback) and non-truncate modes
  - Interrupted restore journal recovery (`RecoverRestoreJournals`)
  - Legacy backup migration from `serverRoot/.backups/`
- **S3Backup**: full S3 API client (not pre-signed URLs)
  - Configurable endpoint (S3-compatible), region, bucket, prefix
  - Path-style bucket access option
  - Multipart upload support via `BackupPart`
  - Local staging before upload
  - Same checksum/integrity guarantees
- **Adapter factory**: `BACKUP_ADAPTER` env (`local`/`s3`)
- **Namespace validation**: strict alphanumeric + hyphen/underscore

### Wings (`server/backup/` — 3 files)

- **BackupInterface**: `SetClient`, `Identifier`, `Generate`, `Checksum`, `Size`, `Path`, `Details`, `Remove`, `Restore`
- **LocalBackup**: stores as `backupDir/{uuid}.tar.gz`
  - Uses `filesystem.Archive` with `mholt/archives` library
  - SHA-1 checksums
  - Rate-limited writes via `juju/ratelimit`
  - Gzip compression (configurable level)
- **S3Backup**: pre-signed URL upload via panel API
  - `GetBackupRemoteUploadURLs()` gets presigned URLs from panel
  - Multipart upload to S3 via presigned URLs
  - Rate-limited restore
- **No namespace validation** — UUIDs are assumed valid
- **No restore journal** — no interruption recovery
- **No legacy migration**

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| S3 implementation | Direct S3 API | Panel-proxied presigned URLs | Different models |
| Checksum | SHA-256 | SHA-1 | Beacon (stronger) |
| Restore recovery | Journal-based rollback | None | Beacon |
| Rate limiting | None | Configurable write limit | Wings |
| Compression config | ZIP (deflate) | TAR.GZ (configurable level) | Wings |
| Legacy migration | Yes | No | Beacon |
| Namespace security | Strict validation | None | Beacon |
| Archive format | ZIP | TAR.GZ | Wings (standard) |

---

## 9. Transfer Protocol

### Beacon (`transfer/` + `server/transfer_protocol.go`)

- **Two transfer systems**:
  1. **Legacy Manager** (`transfer.go`): tar.gz streaming with HTTP POST, resume offset support
  2. **Forge Protocol** (`protocol.go`): `forge-beacon-transfer/v1` — credential-based with SHA-256 HMAC
- **Forge Protocol features**:
  - Registration: panel pre-registers migration with credential hash
  - Authorization: constant-time credential comparison via `crypto/subtle`
  - Direction support: `source-control` and `destination-upload`
  - Offset-based resumption: HEAD returns current offset, PATCH appends
  - Checksum verification: SHA-256 upload checksum
  - Idempotency keys
  - Credential expiry and replay protection
  - State persistence on disk per migration
- **Source push**: source daemon pushes to destination via PATCH with offset negotiation
- **Destination endpoints**: offset, chunk receive, restore, finalize, cancel
- **Cleanup**: deletes container + state on source cleanup

### Wings (`server/transfer/` — 5 files)

- **Transfer struct**: context-based, status-tracked, with Archive and Manager
- **Manager**: server-keyed map with add/remove/get
- **Archive**: streaming tar.gz creation with progress tracking
- **Source**: generates archive, streams to target node via HTTP
- **Status tracking**: `Pending`, `Processing`, `Cancelling`, `Cancelled`, `Failed`, `Completed`
- **Rate limiting**: configurable download limit via `Transfers.DownloadLimit`
- **Integration**: Panel-mediated — source and destination coordinate through panel API calls

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Protocol sophistication | Custom credential-based v1 | Panel-mediated | Beacon (self-contained) |
| Security | SHA-256 + constant-time + replay protection | Panel JWT + bearer | Beacon |
| Resumption | Offset-based with retry | Archive-based | Beacon |
| Progress tracking | Basic percentage | Archive progress | Tie |
| Rate limiting | None | Configurable | Wings |
| Maturity | New protocol | Battle-tested | Wings |

---

## 10. Installer

### Beacon (`installer/installer.go` + `runtime/docker.go` Install)

- **Two install paths**:
  1. **Runtime.Install()**: creates temporary container with `--cap-drop ALL --read-only`, runs script, collects logs, removes container
  2. **Installer.Run()**: writes script to disk, creates container with network disabled, polls for completion, checks logs for `ERROR:`/`FATAL:`
- **Security**: `ReadonlyRootfs: true`, `SecurityOpt: no-new-privileges`, memory limit 512MB
- **Timeout**: 30-minute context deadline
- **Log capture**: 1MB limit on container logs
- **Panel notification**: `SetInstallationStatus()` reports success/failure

### Wings (`server/install.go` + `server/installer/installer.go`)

- **Server.Install()**: fetches script from panel, creates Docker container, streams output to event bus
- **Reinstall()**: stops server, syncs state, runs install
- **Startup detection**: output line matching for completion signals
- **Panel integration**: `SyncInstallState()` notifies panel of completion
- **Event publishing**: `InstallStartedEvent`, `InstallCompletedEvent`
- **Installer struct**: manages server creation from panel data with UUID validation

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Security | Excellent (read-only, no-new-privs) | Good | Beacon |
| Output streaming | Log capture only | Event bus real-time | Wings |
| Startup detection | None | Regex matching | Wings |
| Timeout | 30 min | Configurable | Wings |
| Error detection | Log string matching | Exit code + output | Wings |

---

## 11. Remote Client Communication

### Beacon (`remote/client.go` + `remote/types.go`)

- **Client interface** (12 methods): `GetServerConfiguration`, `GetServers`, `ResetServersState`, `SendActivityLogs`, `SendServerStats`, `SendNodeHeartbeat`, `CreatePlacementReservation`, `ConfirmPlacementReservation`, `CancelPlacementReservation`, `TriggerServerBackup`, `ReportEvacuationProgress`, `SetInstallationStatus`
- **Dual URL roots**: `/api/remote` for daemon calls, `/api/v1` for Forge/panel calls
- **Token format**: single bearer token (not `id.token` format)
- **Error handling**: reads error body (16KB limit), formats descriptive messages
- **No retry**: single-attempt HTTP calls
- **Placement API**: reservation system for server placement

### Wings (`remote/` — 4 files)

- **Client interface** (12 methods): similar scope but different method signatures
- **Token format**: `Bearer {id}.{token}` (dual-token)
- **Retry**: exponential backoff via `cenkalti/backoff/v4`
- **Response wrapper**: typed `Response` struct with `BindJSON()`
- **Pagination**: parallel page fetching via `errgroup`
- **Error taxonomy**: `RequestError`, `SftpInvalidCredentialsError`
- **User-Agent**: `Pterodactyl Wings/v{version} (id:{tokenId})`

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Retry resilience | None | Exponential backoff | Wings |
| API coverage | Placement + evacuation | Standard panel APIs | Different |
| Token model | Single token | Dual token (id.token) | Wings (compatibility) |
| Error handling | Descriptive messages | Typed errors + stack traces | Wings |
| Pagination | Per-page limit | Parallel page fetching | Wings |

---

## 12. Event System

### Beacon (`events/events.go`)

- **Bus struct**: `sync.RWMutex` + topic-keyed channel map
- **Publish**: JSON-encodes event, fans out to subscribers concurrently
- **Ring buffer**: 10ms wait → drop oldest → retry pattern (identical to Wings SinkPool)
- **Subscribe**: returns buffered channel (32 capacity)
- **Unsubscribe**: safe removal with channel close
- **Destroy**: closes all channels, marks closed
- **Topic namespacing**: strips `:` suffix for routing

### Wings (`events/events.go` + `system/sink_pool.go`)

- **Bus struct**: wraps `system.SinkPool` (identical ring-buffer push logic)
- **SinkPool**: shared implementation used by both events and sinks
- **Publish**: same JSON encoding + ring-buffer fan-out
- **MustDecode/DecodeTo**: event deserialization helpers
- **panic on marshal error**: stricter error handling

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Implementation | Self-contained bus | SinkPool wrapper | Beacon |
| Ring buffer | Identical 10ms-drop pattern | Identical | Tie |
| Error handling | Silent drop on marshal fail | Panic on marshal fail | Different philosophy |
| Unsubscribe | Safe with close | Off (same) | Tie |

---

## 13. RootFS Handling

### Beacon (`rootfs/` — 6 files)

- **FS struct**: root-confined filesystem with platform-specific backends
- **Linux impl**: `openat2` with `RESOLVE_BENEATH | RESOLVE_NO_MAGICLINKS | RESOLVE_NO_SYMLINKS`
- **Fallback**: `openat` walk for kernels < 5.6
- **Operations**: Open, OpenFile, Stat, ReadDir, MkdirAll, RemoveAll, Rename, Chtimes, Chmod, WriteFile, AtomicWrite, AtomicWriteExact, Copy, Usage
- **AtomicFile**: temp file + `fsync` + rename for crash-safe writes
- **Path validation**: `Clean()` rejects traversal, null bytes, absolute paths, drive prefixes
- **Usage calculation**: recursive walk excluding symlinks with overflow protection

### Wings (`internal/ufs/` — 12+ files)

- **Full Unix Filesystem (UFS) layer**: file operations, filesystem, walk, stat, quota, errors
- **openat2 integration**: `RESOLVE_BENEATH` with fallback
- **QuotaWriter**: disk quota enforcement on writes
- **File operations**: FilePosix for POSIX-specific behavior
- **Walk**: custom directory traversal
- **RemoveAll**: recursive removal
- **Stat**: with Linux-specific extensions

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Scope | 6 files, focused | 12+ files, comprehensive | Wings |
| Security | openat2 + RESOLVE_BENEATH | openat2 + RESOLVE_BENEATH | Tie |
| Atomic writes | Built-in AtomicFile | Separate implementation | Tie |
| Disk quota | Usage calculation | QuotaWriter on writes | Wings |
| Platform support | Linux + fallback | Linux + fallback | Tie |

---

## 14. System Utilities

### Beacon (`system/` — 8 files)

| File | Purpose |
|------|---------|
| `activity_dedup.go` | Sliding-window SFTP activity dedup with batched flush |
| `atomic.go` | `AtomicString` and `AtomicBool` wrappers |
| `locker.go` | Channel-based non-blocking lock |
| `rate.go` | Time-window rate limiter |
| `sink_pool.go` | Ring-buffer channel fan-out (ported from Wings) |
| `fs_linux.go` | openat2 SafeOpen with RESOLVE_BENEATH |
| `fs_other.go` | Non-Linux fallback |

### Wings (`system/` — 10+ files)

| File | Purpose |
|------|---------|
| `system.go` | Docker system info collection |
| `sink_pool.go` | Ring-buffer channel fan-out (original) |
| `locker.go` | Channel-based non-blocking lock |
| `rate.go` | Time-window rate limiter |
| `utils.go` | ScanReader, FirstNotEmpty, MustInt |
| `context_bag.go` | Context-keyed bag for SFTP sessions |
| `const.go` | Version constant |
| `locker_test.go` | Tests |
| `rate_test.go` | Tests |
| `sink_pool_test.go` | Tests |

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Original code | 2 unique (dedup, atomic) | All original | Wings |
| Ported code | SinkPool, Locker, Rate | — | — |
| Test coverage | Some tests | Comprehensive tests | Wings |
| Docker info | Not collected | Full system info | Wings |
| Activity dedup | Built-in with batch flush | Separate cron-based | Beacon |

---

## 15. Security

### Beacon

| Feature | Implementation |
|---------|---------------|
| Container isolation | `CapDrop: ALL`, `ReadonlyRootfs`, `no-new-privileges` |
| Path traversal | `openat2` RESOLVE_BENEATH + NO_SYMLINKS + NO_MAGICLINKS |
| UUID validation | Strict 8-4-4-4-12 hex pattern |
| Namespace validation | Alphanumeric + hyphen/underscore only |
| Backup separation | Server root and backup root must be distinct |
| Archive validation | Entry count limits, size limits, symlink rejection, duplicate path detection |
| Transfer auth | SHA-256 credential hash + constant-time compare + replay protection |
| Pull downloads | Pinned DNS resolver, no redirect to private IPs, proxy disabled |
| Auth mode | Required in production, explicit opt-out for dev |
| File permissions | `0o750` directories, `0o640` files |
| Config file | `0o600` permissions on save |

### Wings

| Feature | Implementation |
|---------|---------------|
| Container isolation | Same + user namespace, passwd files |
| Path traversal | openat2 + UFS abstraction |
| TLS | Full TLS config with cipher suite selection |
| Auto-TLS | Let's Encrypt autocert integration |
| SSH hardening | Explicit KEX, cipher, MAC algorithm lists |
| Trusted proxies | Configurable proxy list |
| CORS | Private network header support |
| Openat mode | Configurable (auto/on/off) |
| File permissions | User-owned (pterodactyl) |
| Config file | YAML with structured tags |

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Container security | Excellent | Excellent + user namespace | Wings |
| Network security | Pull pinning, SSRF protection | TLS, autocert, CORS | Different |
| Path security | openat2 + validation | openat2 + UFS | Tie |
| Transfer security | Custom crypto protocol | Panel JWT | Beacon |
| Auth enforcement | Hard fail in production | Depends on panel | Beacon |
| SSH hardening | Basic | Explicit algorithms | Wings |

---

## 16. Error Handling

### Beacon

- **stdlib errors**: `errors.New()`, `errors.Is()`, `errors.Join()`, `fmt.Errorf()` with `%w` wrapping
- **No stack traces**: errors propagated with context strings
- **Silent drops**: backup marshal errors, event publish failures logged but not fatal
- **Structured HTTP errors**: `writeTransferError()` maps error types to HTTP status codes
- **Graceful degradation**: Docker unavailable → mock mode, panel sync fails → disk recovery

### Wings

- **`emperror.dev/errors`**: stack traces on all wrapped errors
- **`errors.WithStackIf()`**: conditional stack trace attachment
- **`errors.WrapIf()`**: conditional wrapping with context
- **Structured logging**: `apex/log` with field-based context
- **Panic on invariant violations**: event bus marshal failure panics
- **HTTP error taxonomy**: typed `RequestError` with status code access

### Verdict

| Aspect | Beacon | Wings | Winner |
|--------|--------|-------|--------|
| Stack traces | None | Full stack traces | Wings |
| Error context | String formatting | Structured wrapping | Wings |
| Debuggability | Requires reproduction | Stack traces available | Wings |
| Resilience | Graceful fallback | Fail-fast with panics | Different |
| HTTP errors | Structured mapping | Typed errors | Tie |

---

## 17. Architecture Patterns

### Beacon

```
cmd/daemon/main.go          — Entry point, env config, orchestration
config/config.go            — YAML config, thread-safe singleton
internal/
  server/                   — HTTP handlers + server manager + console
  runtime/                  — Docker runtime interface + implementation
  backup/                   — Backup interface + local + S3
  transfer/                 — Transfer protocol (legacy + Forge v1)
  remote/                   — Panel API client
  sftpserver/               — Native SFTP server
  installer/                — Installation runner
  rootfs/                   — Confined filesystem (openat2)
  serverid/                 — UUID validation
  events/                   — Event bus
  system/                   — Utilities (dedup, rate, locker, sink)
  ignore/                   — .pteroignore parser
```

**Pattern**: Flat internal packages, each owning a domain. No external web framework. Single-process, single-binary with env-only configuration. No database.

### Wings

```
cmd/                        — Cobra CLI (root, configure, diagnostics)
config/                     — YAML config + runtime helpers
server/                     — Server struct, manager, power, install, backup, transfer, filesystem, console, events, websockets, crash
  filesystem/               — File operations, archive, compression, disk space
  backup/                   — Backup adapters (local, S3)
  transfer/                 — Transfer protocol
  installer/                — Installation orchestrator
environment/                — Process environment abstraction
  docker/                   — Docker implementation
remote/                     — Panel API client
router/                     — Gin HTTP router + middleware
  middleware/               — Auth, CORS, request ID
  tokens/                   — JWT token management
  websocket/                — WebSocket server
  downloader/               — File download handler
sftp/                       — SFTP server
events/                     — Event bus
system/                     — Utilities
internal/
  cron/                     — Scheduled tasks
  database/                 — BoltDB persistence
  models/                   — Activity models
  progress/                 — Progress tracking
  ufs/                      — Unix filesystem abstraction
parser/                     — Config parser for egg files
loggers/                    — Log formatters
```

**Pattern**: Deep package hierarchy, one package per concern. Framework-based routing. Optional database. Multi-command CLI. Extensive configuration options.

### Architectural Comparison

| Aspect | Beacon | Wings |
|--------|--------|-------|
| Package depth | 2 levels (internal/*) | 4 levels (server/filesystem/*) |
| Coupling | Loose (interfaces) | Loose (interfaces + DI) |
| Framework | net/http stdlib | gin-gonic/gin |
| DI | Constructor injection | Constructor + middleware injection |
| State management | sync.Map per-server | Mutex-protected slice |
| Configuration | Env vars | File + env + CLI flags |
| Extensibility | Add internal package | Add package + route + middleware |
| Testing | Unit tests present | Unit + integration tests |
| Documentation | Minimal inline | Extensive comments + godoc |

---

## 18. Feature Gap Analysis

### Features in Wings but missing/incomplete in Beacon

| Feature | Wings | Beacon | Impact |
|---------|-------|--------|--------|
| Cobra CLI | Full CLI with subcommands | Env vars only | Low — container-friendly |
| Cron scheduler | Activity + SFTP crons | None | Medium — no periodic tasks |
| Database | BoltDB for persistence | None | Low — panel is source of truth |
| Auto-TLS | Let's Encrypt autocert | None | Low — typically behind proxy |
| Pprof profiling | Built-in with flags | None | Medium — debugging |
| Startup detection | Regex output matching | None | Medium — no auto-detect ready |
| Log rotation | Auto-configured | None | Low — container handles |
| Config parser | Egg file parser | None | Low — panel provides config |
| Trusted proxies | Configurable | None | Medium — reverse proxy setups |
| TLS config | Full cipher suite control | None | Low — typically behind proxy |
| User namespace | Docker userns remapping | None | Medium — rootless containers |
| Backup rate limiting | Configurable MiB/s | None | Medium — I/O impact |
| Transfer rate limiting | Configurable download limit | None | Medium — bandwidth control |
| Disk space caching | Configurable interval | Real-time walk | Low — trade-off |
| Activity cron | Periodic batch send | Dedup + flush | Different approaches |
| Compression levels | Configurable gzip | ZIP deflate only | Low |

### Features in Beacon but missing/incomplete in Wings

| Feature | Beacon | Wings | Impact |
|---------|--------|--------|--------|
| Healthcheck probe | Built-in `--healthcheck` | None | High — container orchestration |
| Forge transfer protocol | Custom credential-based v1 | Panel-mediated | High — self-contained |
| Interrupted restore recovery | Journal-based rollback | None | High — data safety |
| Legacy backup migration | Auto-migrates `.backups/` | None | Medium — upgrades |
| Backup namespace validation | Strict alphanumeric | None | High — path traversal |
| Activity deduplication | Sliding-window with batch flush | Cron-based send | Medium — efficiency |
| Pull download security | Pinned DNS + no-SSRF | Basic download | High — SSRF prevention |
| Placement API | Reservation system | None | Medium — server placement |
| Evacuation API | Progress reporting | None | Medium — node evacuation |
| Archive validation | Entry count + size + symlink + dedup | Basic validation | High — zip bomb protection |
| Staged archive extraction | Atomic staging + rollback | Direct extraction | Medium — crash safety |
| Session deauthorization | WS + SFTP session registry | WS only | Medium — SFTP security |
| Config persistence on update | Writes panel push to disk | No-op or reload | Medium — audit trail |

---

## 19. Test Coverage

### Beacon Tests

| File | Tests |
|------|-------|
| `cmd/daemon/main_test.go` | Main package tests |
| `cmd/daemon/mem_linux.go` | Linux memory reading |
| `backup/local_test.go` | Local backup tests |
| `backup/s3_test.go` | S3 backup tests |
| `remote/client_test.go` | Remote client tests |
| `rootfs/rootfs_test.go` | RootFS tests |
| `rootfs/rootfs_linux_test.go` | Linux rootfs tests |
| `runtime/docker_test.go` | Docker runtime tests |
| `runtime/stats_test.go` | Stats decoding tests |
| `server/console_test.go` | Console throttle tests |
| `server/manager_test.go` | Server manager tests |
| `server/runtime_handlers_test.go` | Runtime handler tests |
| `server/secure_files_test.go` | File security tests |
| `server/server_test.go` | Server tests |
| `serverid/serverid_test.go` | UUID validation tests |
| `sftpserver/server_test.go` | SFTP server tests |
| `system/activity_dedup_test.go` | Activity dedup tests |
| `transfer/transfer_test.go` | Transfer tests |
| `transfer/protocol_v1_test.go` | Protocol v1 tests |

**19 test files** covering core subsystems.

### Wings Tests

| File | Tests |
|------|-------|
| `events/events_test.go` | Event bus tests |
| `internal/progress/progress_test.go` | Progress tracking |
| `internal/ufs/fs_unix_test.go` | Unix filesystem |
| `remote/http_test.go` | HTTP client |
| `server/console_test.go` | Console tests |
| `server/filesystem/archive_test.go` | Archive tests |
| `server/filesystem/compress_test.go` | Compression |
| `server/filesystem/errors_test.go` | Error types |
| `server/filesystem/filesystem_test.go` | Filesystem |
| `server/filesystem/path_test.go` | Path validation |
| `server/power_test.go` | Power actions |
| `system/locker_test.go` | Locker |
| `system/rate_test.go` | Rate limiter |
| `system/sink_pool_test.go` | Sink pool |
| `system/utils_test.go` | Utilities |

**15+ test files** (plus many more in subpackages).

---

## 20. Dependencies Comparison

### Beacon Direct Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/docker/docker` | Docker API client |
| `github.com/docker/go-connections` | NAT port parsing |
| `github.com/gorilla/websocket` | WebSocket connections |
| `github.com/pkg/sftp` | SFTP server |
| `golang.org/x/crypto` | SSH server |
| `golang.org/x/sys` | Linux syscalls (openat2) |
| `gopkg.in/yaml.v2` | YAML parsing |
| AWS S3 SDK | S3 backup adapter |

**~8 direct dependencies**

### Wings Direct Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/docker/docker` | Docker API client |
| `github.com/gin-gonic/gin` | HTTP framework |
| `github.com/gorilla/websocket` | WebSocket |
| `github.com/pkg/sftp` | SFTP server |
| `golang.org/x/crypto` | SSH, ACME |
| `golang.org/x/sys` | Linux syscalls |
| `gopkg.in/yaml.v2` | YAML parsing |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/apex/log` | Structured logging |
| `emperror.dev/errors` | Error wrapping |
| `github.com/mholt/archives` | Archive handling |
| `github.com/cenkalti/backoff/v4` | Retry logic |
| `github.com/juju/ratelimit` | Rate limiting |
| `github.com/gammazero/workerpool` | Worker pools |
| `github.com/creasty/defaults` | Struct defaults |
| `github.com/gbrlsnchs/jwt/v3` | JWT tokens |
| `github.com/google/uuid` | UUID generation |
| `github.com/NYTimes/logrotate` | Log rotation |
| `github.com/mitchellh/colorstring` | Terminal colors |
| `github.com/asaskevich/govalidator` | Validation |
| `github.com/acobaugh/osrelease` | OS detection |
| `github.com/cenkalti/backoff/v4` | Retry |
| `go.etcd.io/bbolt` | BoltDB |
| `golang.org/x/sync` | errgroup |

**~25 direct dependencies**

---

## 21. Recommendations

### For Beacon

1. **Add startup detection**: Implement regex-based output line matching for server-ready signals
2. **Add cron scheduler**: Even a minimal ticker-based scheduler for periodic tasks
3. **Add retry to remote client**: Exponential backoff for panel API calls
4. **Add transfer rate limiting**: Configurable bandwidth limits
5. **Add backup compression config**: Support gzip level selection
6. **Add structured logging**: Migrate from `log` to `apex/log` or `slog`
7. **Add stack-trace errors**: Consider `emperror.dev/errors` for debuggability
8. **Add config file support**: While env vars work, YAML config supports complex nested settings
9. **Expand test coverage**: Add integration tests for Docker runtime, SFTP, backup round-trips
10. **Add pprof support**: Even without CLI flags, expose via `/debug/pprof/` endpoint

### For Wings Parity

Beacon already achieves functional parity across all critical subsystems. The gaps are primarily operational maturity (retry, cron, logging) rather than feature gaps. The areas where Beacon **exceeds** Wings are notable:

- **Healthcheck**: Essential for container orchestration, completely absent in Wings
- **Transfer protocol**: Self-contained credential-based protocol with replay protection
- **Restore recovery**: Journal-based rollback prevents data loss
- **Security validation**: Strict namespace/UUID/archive validation throughout
- **SSRF protection**: Pull download security with DNS pinning

---

## Appendix: File-by-File Mapping

| Domain | Beacon Files | Wings Files |
|--------|-------------|-------------|
| Entry point | `cmd/daemon/main.go` | `wings.go`, `cmd/root.go` |
| Config | `config/config.go` | `config/config.go`, `config/config_docker.go` |
| Server model | `server/server.go`, `server/manager.go` | `server/server.go`, `server/manager.go`, `server/configuration.go` |
| Power control | `server/manager.go` (HandlePower) | `server/power.go` |
| Console | `server/console.go` | `server/console.go`, `server/connections.go` |
| WebSocket | `server/server.go` (ws endpoints) | `server/websockets.go`, `router/websocket/` |
| Docker runtime | `runtime/docker.go`, `runtime/runtime.go`, `runtime/stats.go` | `environment/docker/` (5 files) |
| Backup | `backup/backup.go`, `backup/local.go`, `backup/s3.go` | `server/backup/backup.go`, `server/backup/backup_local.go`, `server/backup/backup_s3.go` |
| Transfer | `transfer/transfer.go`, `transfer/protocol.go`, `server/transfer_protocol.go` | `server/transfer/` (5 files) |
| SFTP | `sftpserver/server.go` | `sftp/server.go`, `sftp/handler.go`, `sftp/event.go`, `sftp/utils.go` |
| Filesystem | `rootfs/rootfs.go`, `rootfs/rootfs_linux.go`, `server/secure_files.go` | `server/filesystem/` (10+ files), `internal/ufs/` (12 files) |
| Remote client | `remote/client.go`, `remote/types.go` | `remote/http.go`, `remote/servers.go`, `remote/types.go`, `remote/errors.go` |
| Events | `events/events.go` | `events/events.go` |
| System utils | `system/` (8 files) | `system/` (10+ files) |
| Installer | `installer/installer.go` | `server/install.go`, `server/installer/installer.go` |
| Server ID | `serverid/serverid.go` | (embedded in validation) |
| Ignore patterns | `ignore/ignore.go` | (embedded in filesystem) |
| Router | (net/http mux in server.go) | `router/router.go` + 12 route files + middleware |
| Cron | — | `internal/cron/` (3 files) |
| Database | — | `internal/database/database.go` |
| Models | — | `internal/models/` (2 files) |
| Parser | — | `parser/parser.go`, `parser/helpers.go` |

---

*End of Agent 4 Comparative Audit*
