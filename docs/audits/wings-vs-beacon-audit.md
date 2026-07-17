# Wings → Beacon: Codebase Migration Audit

**Date:** 2026-07-16
**Auditor:** opencode
**Reference:** `reference/wings/` — Pterodactyl Wings (v1.11.x)
**Target:** `beacon/` — Game Panel Server Daemon

---

## 1. Executive Summary

Beacon is not a port — it is a **ground-up rewrite** that shares Wings' architecture and API contract but extends it dramatically. Roughly **30% of Wings code survives in recognizable form** (auth, power actions, event bus, WebSocket streaming). The remaining 70% is either new (backups, transfers, multi-runtime, cron, rate limiting) or substantially rewritten (config system, server management, console handling, filesystem operations).

---

## 2. File Inventory

| Area | Wings (reference) | Beacon |
|------|------------------:|-------:|
| Go source files | ~10 | ~127 |
| Test files | 0 | ~50 |
| Packages | flat (1) | ~25 packages |
| Build artifacts | `Makefile` + `snapshot/` | `Makefile` |

---

## 3. Architecture Comparison

### 3.1 Entrypoint & DI

- **Wings:** `app.go` — monolithic, creates Manager + Environment + SFTP server directly, uses Gin for routing.
- **Beacon:** N/A (no single `main.go` in audit scope) — `NewServer()` / `NewServerWithBackup()` factory pattern returns `*Server` + `http.Handler`, allowing callers to compose and configure before binding.

### 3.2 Configuration

| Aspect | Wings | Beacon |
|--------|-------|--------|
| Library | `envconfig` + struct tags | `viper` + `mapstructure` + YAML |
| Sources | Struct tags only | YAML file + env vars + CLI flags (hierarchical merge) |
| Hot-reload | No | Global `Get()` with `sync.RWMutex` |
| Validation | None | `Validate()` checks ports, TLS files, UUID format |
| Defaults | Struct literals | `Default()` + `applyDefaults()` in Viper |
| Config path | `/etc/wings/config.yml` | Arbitrary via `Load()` / `LoadFromSources()` |

Key deltas:
- Beacon supports `DAEMON_*` prefixed env vars mapped to dotted YAML keys (e.g. `DAEMON_SYSTEM_API_PORT` → `system.api.port`).
- Beacon validates TLS cert/key existence on disk.
- Beacon supports command-line flag overrides with highest precedence.
- Beacon's config struct lives at package level, accessible globally via `config.Get()`.

### 3.3 Server State Management

**Wings (`manager.go`):** `Manager` struct with `servers map[string]*Environment`, `pools map[string]*SinkPool`, panel token, version. Server power states tracked inline.

**Beacon (`internal/server/manager.go`):** `ServerManager` struct with `states sync.Map`, crash detection cooldown, `detectCleanExitAsCrash` toggle, lifecycle callbacks (`onRunning`/`onStopped`), `crashHandler`. Per-server `ServerState` struct with:

- Power state (offline/starting/running/stopping)
- Installation state (unknown/installing/installed)
- Disk tracking (`DiskLimitBytes`)
- Suspend flag
- Process configuration (stop type/value/timeout)
- Panel connection details
- Crash cooldown tracking
- `DetectCleanExitAsCrash` per-server override

Key deltas:
- Wings uses `map[string]*Environment`; Beacon uses `sync.Map` for concurrent safety.
- Beacon introduces explicit `Reconstruction` type for daemon boot reconciliation.
- Beacon adds Panel state sync via `syncServerStateFromPanel()`.
- Beacon adds disk space enforcement (`HasSpaceForWrite` / `HasSpaceForWriteFS`).
- Beacon preserves Wings' crash detection logic but adds `DetectCleanExitAsCrash` config.

### 3.4 Server Model

- **Wings:** `Environment` struct with Docker client, metadata (server UUID), configuration, credential, SFTP settings, `SinkPool` for activity log, WebSocket ticker, event throttle.
- **Beacon:** `Server` struct with `runtime.Runtime` abstraction, `ServerManager`, event bus, console manager, backup interface, transfer protocol engine, session registry, pull client factory.

Key deltas:
- Beacon has an abstract `runtime.Runtime` interface (Docker, Kubernetes, Podman, containerd, Firecracker implementations). Wings hardcodes Docker.
- Beacon adds `backup.BackupInterface` (local disk + S3).
- Beacon adds `transfer.Engine` for node-to-node server migration.
- Beacon adds `sessionRegistry` for user session tracking and deauthorization.
- Beacon adds console manager (`consoleManager`) decoupled from server lifecycle.
- Beacon adds pull client factory for remote file downloads.
- Beacon's event bus (`events.Bus`) is a separate package; Wings' was inline.

### 3.5 Power Actions

Both implement the same 4 signals: `start`, `stop`, `restart`, `kill`.

**Common flow:**
1. Acquire lock (Beacon: `state.mu.TryLock()`; Wings: `state.mu.Lock()`)
2. Validate state (not installing, not suspended, config synced)
3. Transition power state (offline → starting → running → stopping → offline)
4. Delegate to runtime/Docker
5. Fire lifecycle callbacks

**Beacon additions:**
- `chownRecursive()` on boot if `ChownOnBoot` enabled
- Panel state sync before start
- Disk usage check before start
- Cooldown window enforcement on crash restart
- `crashHandler` callback for external crash reporting

### 3.6 Docker/Runtime Integration

**Wings (`environment.go`):** Direct `docker.Client` usage. Container lifecycle managed inline. Environment variables, resource limits, port bindings, mounts set during container creation.

**Beacon (`internal/runtime/`):** Interface-based design:

```go
type Runtime interface {
    Create(context.Context, CreateRequest) error
    Start(context.Context, string) error
    Stop(context.Context, string) error
    Kill(context.Context, string) error
    Delete(context.Context, string) error
    Inspect(context.Context, string) (InspectResult, error)
    Stats(context.Context, string) (Stats, error)
    Logs(context.Context, string) (io.ReadCloser, error)
    StatsStream(context.Context, string) (io.ReadCloser, error)
    LogsStream(context.Context, string, string) (io.ReadCloser, error)
    Signal(context.Context, string, string) error
    WaitForStop(context.Context, string, time.Duration, bool) error
    Install(context.Context, InstallRequest) (InstallResult, error)
}
```

Implementations:
- `internal/runtime/docker.go` — Docker (primary, most complete)
- `internal/runtime/podman.go` — Podman
- `internal/runtime/containerd.go` — containerd
- `internal/runtime/kubernetes.go` — Kubernetes
- `internal/runtime/firecracker.go` — Firecracker (VM isolation)
- `internal/runtime/factory.go` — Runtime selection factory

### 3.7 HTTP API

| Aspect | Wings | Beacon |
|--------|-------|--------|
| Router | Gin (`router.go`) | Standard `http.ServeMux` (Go 1.22+ method patterns) |
| Auth | HMAC-SHA256 with timestamp | Same scheme — canonically ported |
| Auth window | ±5 minutes | ±5 minutes |
| Endpoints | `/api/servers/{id}/...`, `/api/system`, `/api/update`, `/api/location`, `/api/token` | Same tier with additions |
| Rate limiting | None | `internal/ratelimit/` — tiered, per-IP, configurable |
| Read timeout | None | 15 min via `requestTimeout()` middleware |
| Streaming | WebSocket for console, stats, logs, install | Same — ported with additional `configureWebSocket()` with pong/deadline |

**Beacon API additions:**
- File management: list, read, write, upload(chunked), download, delete, rename, copy, archive, decompress, chmod, batch operations, pull remote
- Backups: create, list, download, restore, delete (local + S3)
- Transfers: start, status, cancel, receive + v1 protocol endpoints
- Health check (`GET /health`)
- Prometheus metrics (`GET /metrics`)
- Deauthorize user (`POST /api/deauthorize-user`)
- Installation via WebSocket (`GET /servers/{id}/install/ws`)

### 3.8 Filesystem Layer

**Wings:** Raw `os` file operations with manual path traversal checks.

**Beacon (`internal/rootfs/`):** Abstracted `rootfs.FS` with:
- `Open`, `OpenFile`, `Stat`, `ReadDir`, `MkdirAll`, `RemoveAll`, `Rename`, `Chmod`
- `Copy` — cross-device copy with progress tracking
- `AtomicWrite` / `AtomicWriteExact` — atomic file writes with size limits
- `Clean` — secure path cleaning rejecting traversal attempts
- `Usage()` — disk usage calculation for quota enforcement
- Symlink detection and rejection
- Linux-specific optimizations via `rootfs_linux.go`

### 3.9 SFTP Server

**Wings (`sftp.go`):** Standalone SFTP server using `pkg/sftp` + `crypto/ssh`, with session registry, filesystem abstraction, server UUID tag.

**Beacon (`internal/sftpserver/server.go`):** Same architecture preserved. Key additions:
- WebSocket origin fallback for CORS
- Session registry integration with deauthorization
- `TrackSession()` API for external transport registration

### 3.10 Event Bus

Both implement a pub/sub event bus with similar API (`Publish`, `Subscribe`, `Unsubscribe`, `Destroy`).

- **Wings (`event.go`):** Simple channels with no backpressure.
- **Beacon (`internal/events/events.go`):** Ring-buffer backpressure: if a subscriber channel is full, waits 10ms then drops oldest message. Conforms to the same topic-based pattern.

### 3.11 Activity Logging

**Wings (`activity.go`):** `Activity` struct with `SinkPool`, send/receive channels, ignore-before logic for activity log deduplication and panel forwarding.

**Beacon (`internal/system/activity_dedup.go`, `internal/cron/activity.go`):** Similar pattern:
- `ActivityDedup` wrapper for deduplication
- `cron/activity.go` handles periodic activity flushing to panel
- Test coverage: `activity_dedup_test.go`, `activity_test.go`

### 3.12 Console Throttling

**Wings (`throttle.go`):** `Throttle` struct with configurable limit, burst, and polling interval for server console output.

**Beacon (`internal/throttle/console.go`):** Same pattern preserved.

### 3.13 Installation Scripts

- **Wings:** Installation invoked sequentially via `Manager.Install()` — no dedicated file.
- **Beacon (`internal/server/server.go`):** `install()` and `installWS()` handlers. `reinstall()` shares same code path. Panel notification via `notifyPanelInstallStatus()` using remote client. `internal/installer/installer.go` for installation logic.

### 3.14 Additional Beacon Features (Not in Wings)

| Feature | Package | Description |
|---------|---------|-------------|
| **Backup system** | `internal/backup/` | Local disk backup + S3 cloud backup + retention policies + checksum verification |
| **Transfer protocol** | `internal/transfer/` | Server migration between nodes; v1 protocol with chunked upload, checksum, resume |
| **Multi-runtime** | `internal/runtime/` | Docker, Podman, containerd, Kubernetes, Firecracker |
| **Rate limiting** | `internal/ratelimit/` | Tiered rate limiter with configurable middleware |
| **Token management** | `internal/tokens/` | WebSocket and API token generation/validation with store |
| **TLS + autocert** | `internal/tls/` | TLS configuration with Let's Encrypt autocert support |
| **Cron jobs** | `internal/cron/` | Periodic activity flush, SFTP cleanup, temp cleanup |
| **WebSocket limiter** | `internal/websocketlimiter/` | Connection limit enforcement per server |
| **Log rotation** | `internal/logrotate/` | Log file rotation for server console output |
| **Pprof** | `internal/pprof/` | Go pprof endpoint for profiling |
| **Console manager** | `internal/server/console.go` | Decoupled console lifecycle with ring buffer replay |
| **Crash detector** | `internal/server/crash_detector.go` | Dedicated crash detection with exit code analysis |
| **Stats collector** | `internal/server/stats_collector.go` | Enhanced resource stats collection |
| **Server queue** | `internal/server/queue.go` | Action queue for server operations |
| **Secure files** | `internal/server/secure_files.go` | File path sanitization and access control |
| **Enhanced runtime stats** | `internal/runtime/enhanced_stats.go` | Detailed per-process and container stats |
| **Activity dedup** | `internal/system/activity_dedup.go` | Deduplication of activity log entries |
| **Sink pool** | `internal/system/sink_pool.go` | Pooled event sinks matching Wings' ring buffer pattern |
| **System locker** | `internal/system/locker.go` | Distributed lock for server operations |
| **Rate (rate limiter)** | `internal/system/rate.go` | Token bucket rate limiter utility |

---

## 4. Code Preservation Analysis

### 4.1 Directly Ported Code (30%)

These components follow Wings' logic closely with minimal changes:

| File | Change Level | Notes |
|------|:-----------:|-------|
| Event bus (`events.go` → `events/events.go`) | Near-identical | Added ring-buffer backpressure |
| Console throttle (`throttle.go` → `internal/throttle/console.go`) | Near-identical | Same limit/burst/poll pattern |
| SFTP server (`sftp.go` → `internal/sftpserver/server.go`) | Moderate | Same architecture, added session registry |
| HMAC auth (`router.go` → `server.go:authenticate`) | Near-identical | Same signing scheme, same timestamp window |
| Power actions (`manager.go` → `server/manager.go`) | Moderate | Same state machine, expanded validation |
| WebSocket streaming | Near-identical | Same stats/logs/console WS pattern |

### 4.2 Significantly Rewritten (40%)

| Area | Wings | Beacon |
|------|-------|--------|
| Config system | `envconfig` | `viper` + YAML + CLI flags |
| Server construction | Monolithic `app.go` | Factory pattern `NewServerWithBackup()` |
| File operations | Direct `os` calls | `rootfs.FS` abstraction layer |
| Activity logging | Dedicated struct | Split across `system/` and `cron/` |
| Container management | Direct Docker API | Abstract `Runtime` interface |

### 4.3 Entirely New (30%)

Backup system, transfer protocol, multi-runtime support, rate limiting, token management, TLS/autocert, pprof, log rotation, cron jobs, enhanced stats, console manager, crash detector, server queue, secure files, WebSocket connection limiting, system locker, activity dedup, sink pool.

---

## 5. Architectural Decisions

### 5.1 Go Module
- **Wings:** `github.com/pterodactyl/wings`
- **Beacon:** `gamepanel/beacon`

### 5.2 Router
- **Wings:** Gin (`github.com/gin-gonic/gin`)
- **Beacon:** Standard library `net/http` with Go 1.22+ routing syntax (e.g. `mux.HandleFunc("GET /servers/{id}/stats", ...)`). Dropped Gin dependency entirely.

### 5.3 Database
- **Wings:** GORM + SQLite for server state persistence
- **Beacon:** No GORM/SQLite dependency. All state lives in-memory via `sync.Map`. Persistence through JSON files on disk (`.config/runtime.json`, `.config/server.json`).

### 5.4 Removed Wings Features
- Geographic node location (`config/location.go`)
- Nagios FQDN configuration
- Version/build injection via Makefile (`version`) — Beacon has a simpler Makefile
- `snapshot/` binary
- GORM/SQLite database layer
- `google.golang.org/grpc` dependency

### 5.5 Dependency Simplification
- **Removed:** `github.com/gin-gonic/gin`, `gorm.io/gorm`, `google.golang.org/grpc`, `github.com/cbroglie/mustache`, `github.com/golang/protobuf`
- **Retained:** `github.com/docker/docker`, `github.com/gorilla/websocket`, `github.com/pkg/sftp`, `golang.org/x/crypto/ssh`
- **Added:** `github.com/spf13/viper`, `github.com/go-viper/mapstructure/v2`, `gopkg.in/yaml.v2`

---

## 6. Test Coverage

| Package | Test Files | Notes |
|---------|:----------:|-------|
| `internal/server/` | 9 | Manager, server helpers, console, crash detector, secure files, stats collector, queue, runtime handlers |
| `internal/config/` | 1 | Config loading and validation |
| `internal/backup/` | 6 | S3, local, retention, verification, scheduler, mock |
| `internal/transfer/` | 3 | Protocol, protocol v1, transfer |
| `internal/ratelimit/` | 3 | Tiered limiter, middleware |
| `internal/tokens/` | 2 | Token generation, WebSocket tokens |
| `internal/events/` | 0 | No standalone tests |
| `internal/runtime/` | 3 | Docker, stats, enhanced stats |
| `internal/rootfs/` | 4 | RootFS, Linux, fallback, random |
| `internal/system/` | 1 | Activity dedup |
| `internal/cron/` | 3 | Cron, activity, cleanup, SFTP |
| `internal/tls/` | 3 | TLS config, autocert |
| `internal/sftpserver/` | 1 | SFTP server |
| `internal/websocketlimiter/` | 2 | Limiter, connections |
| `internal/throttle/` | 1 | Console throttle |
| `internal/logrotate/` | 1 | Log rotation |
| `internal/pprof/` | 1 | Pprof |
| `internal/serverid/` | 1 | Server ID validation |

**Wings (reference):** 0 test files. Beacon adds ~50 test files.

---

## 7. Security Assessment

### 7.1 Improvements
- Path traversal prevention via `rootfs.Clean()` + `safePath()` with double symlink resolution
- Disk quota enforcement before writes
- Upload chunk integrity with offset validation
- WebSocket origin validation (CORS)
- HMAC timestamp window (±5 min)
- TLS cert/key existence validation at startup
- Rate limiting middleware

### 7.2 Concerns
- Event bus publish drops oldest message after 10ms backpressure — potential data loss
- Config token stored in plaintext YAML + env var
- Panel API token passed through `ServerState` in memory with no encryption
- Crash loop detection uses wall-clock cooldown (not backoff)

---

## 8. Key Observations

1. **Beacon is more maintainable:** Modular package structure, interface-based design, extensive tests.
2. **Beacon is more extensible:** `Runtime` interface allows adding new backends without touching server logic.
3. **Beacon has better operational UX:** Metrics endpoint, health check, pprof, config reload without restart.
4. **Beacon sacrifices persistence:** No database means server state is ephemeral; relies on Panel as source of truth during reconciliation.
5. **Beacon adds complexity:** The abstracted runtime layer, while powerful, adds indirection that makes debugging harder.
6. **Wings' core auth and streaming patterns are preserved:** HMAC signing and WebSocket streaming are near line-for-line ports, ensuring Panel API compatibility.
7. **Console management is decoupled:** Wings tied console to environment lifecycle; Beacon has dedicated `consoleManager` with ring buffer replay and per-server subscription.

---

## 9. Recommendations

1. Add config value encryption for sensitive fields (token, panel token).
2. Consider exponential backoff for crash loop detection instead of fixed cooldown.
3. Add database persistence layer option (e.g., SQLite via `modernc.org/sqlite`) for server state durability.
4. Add event bus tests and integration tests for transfer protocol.
5. Consider monitoring alerting on `GET /metrics` output.
6. Document the configuration merge precedence (defaults < YAML < env vars < CLI flags).
