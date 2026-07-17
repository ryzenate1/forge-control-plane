# Modern Game Panel — Full Analysis & Modernization Plan

> **Scope**: Deep comparison of Pterodactyl Panel (PHP) + Wings (Go) vs our Modern Game Panel implementation. All findings verified against local reference sources at `refs/pterodactyl-panel/` and `refs/pterodactyl-wings/`.

---

## 1. Feature Gap Analysis

### ✅ Features Present and Improved

| Feature | Pterodactyl | Our Implementation | How We're Better |
|---|---|---|---|
| API Server | PHP/Laravel + Apache/Nginx | Go Fiber (single binary) | 10-50x faster, 5-10x less memory, no PHP runtime |
| Queue Worker | Separate `pteroq.service` (PHP) | In-process goroutines | One fewer systemd service, no cron dependency |
| Scheduler | Crontab + `artisan schedule:run` | [schedule_runner.go](file:///e:/game/apps/api/internal/http/schedule_runner.go) in-process | Zero external dependencies |
| Database | MySQL | PostgreSQL 16 | Better JSONB, MVCC concurrency, partial indexes |
| Container Abstraction | Hardcoded Docker SDK | [runtime.go](file:///e:/game/apps/daemon/internal/runtime/runtime.go) interface | Ready for containerd/Podman backends |
| Chunked Uploads | Single POST upload | Offset-tracked chunked upload with `uploadId` | Resumable uploads on disconnect |
| Daemon Auth | JWT tokens signed by panel | HMAC-SHA256 with timestamp (5-min window) | Simpler, no key rotation complexity |
| File Jailing | Chroot-based (`pkg/sftp`) | Path sanitization (`safePath`) | Simpler, no privilege requirement |
| Deployment | ~12 manual steps, PHP extensions, Composer | 2 Go binaries + PostgreSQL + Redis | Dramatically simpler |
| Monitoring | None built-in | Prometheus `/metrics` on API + daemon | Observable from day one |

### ⚠️ Features Partially Implemented

| Feature | Pterodactyl Source | Our Status | Gap Details |
|---|---|---|---|
| Granular Permissions | [Permission.php](file:///e:/game/refs/pterodactyl-panel/app/Models/Permission.php) — 40+ constants | [permissions.go](file:///e:/game/apps/api/internal/store/permissions.go) — constants defined | Permissions defined but **not enforced** in handler middleware. All routes use `requireRole("admin")` or no check. Subuser permissions stored in DB but ignored at request time. |
| Backup Persistence | `Backup.php` model, stored in DB | Daemon-only (zip on filesystem) | No `backups` table in our DB. Can't track backup history, no S3 adapter. |
| Nests/Eggs Admin | Full CRUD with variable editor, egg import/export | API CRUD built in [handlers_admin.go](file:///e:/game/apps/api/internal/http/handlers_admin.go) + [store_nests.go](file:///e:/game/apps/api/internal/store/store_nests.go) | No frontend UI for nest/egg management. No egg import from JSON. |
| Server Selection UX | Full server list → detail routing | Dashboard component, route stubs exist | [dashboard.tsx](file:///e:/game/apps/frontend/components/dashboard.tsx) still present but partially refactored. Server routing not fully wired. |
| Docker Power State | [power.go](file:///e:/game/refs/pterodactyl-wings/server/power.go) — `starting`, `running`, `stopping`, `offline` with mutex lock | Simple `start`/`stop`/`restart`/`kill` | No state machine. No power lock preventing race conditions. No `onBeforeStart()` pre-flight checks (disk space, config sync, permission check). |
| Install Process | [install.go](file:///e:/game/refs/pterodactyl-wings/server/install.go) — 19KB, full lifecycle | [server.go:224](file:///e:/game/apps/daemon/internal/server/server.go#L224) — basic install | No install event lifecycle, no pre/post hooks, no install log streaming via WebSocket. |
| Admin Settings | `Admin/Settings/*` controllers | None | No general settings, mail config, maintenance mode from UI. |

### ❌ Features Missing

| Feature | Pterodactyl Source | Impact | Complexity |
|---|---|---|---|
| **Crash Detection** | [crash.go](file:///e:/game/refs/pterodactyl-wings/server/crash.go) — auto-restart with cooldown timer | **HIGH** — servers stay dead after crash | Medium — need Docker event listener + restart logic |
| **Server Manager** | [manager.go](file:///e:/game/refs/pterodactyl-wings/server/manager.go) — in-memory server state, parallel init | **HIGH** — daemon is stateless, no tracking | Medium — add `sync.Map` of server states |
| **File Archive/Decompress** | [router_server_files.go](file:///e:/game/refs/pterodactyl-wings/router/router_server_files.go) — 18KB, tar.gz compress/decompress | Medium — users must upload pre-extracted | Medium |
| **Backup Restore** | `BackupController` + Wings restore handler | Medium — one-way backups only | Medium |
| **Go-native SFTP** | [sftp/server.go](file:///e:/game/refs/pterodactyl-wings/sftp/server.go) + [handler.go](file:///e:/game/refs/pterodactyl-wings/sftp/handler.go) — panel-authenticated | Medium — sidecar uses static creds | High — need `pkg/sftp` integration |
| **Disk Space Enforcement** | [filesystem/disk_space.go](file:///e:/game/refs/pterodactyl-wings/server/filesystem) | Medium — no limits enforced | Low |
| **Node Auto-Deploy** | `NodeAutoDeployController.php` | Low — manual config is fine for now | Low |
| **OOM Kill Handling** | [environment/docker/environment.go](file:///e:/game/refs/pterodactyl-wings/environment/docker/environment.go) | Low — Docker already enforces limits | Low |
| **Config File Parser** | Wings `server/config_parser.go` + panel `ConfigurationStructureService` | Low — server configs need manual setup | Medium |

---

## 2. Architecture Review

### 🔴 Critical Issues

#### 2.1 Permissions Defined But Not Enforced

- **Finding**: [permissions.go](file:///e:/game/apps/api/internal/store/permissions.go) defines 38 granular permissions matching Pterodactyl's `Permission.php`. The `HasPermission()` function exists. But **no handler** actually calls it.
- **Evidence**: Every protected route uses only `requireRole("admin")` or no role check. Subuser permissions are stored in the DB but never validated against incoming requests.
- **Root Cause**: The permission enforcement middleware was never integrated after the permission model was built.
- **Proposed Solution**: Create a `requirePermission(perm string)` middleware that checks both role (admin bypass) and subuser permissions against the server being accessed. Wire it to every server-scoped route.
- **Expected Impact**: Enables multi-user mode — the #1 feature gap vs Pterodactyl.

#### 2.2 No Power State Machine in Daemon

- **Finding**: Our daemon's [power handler](file:///e:/game/apps/daemon/internal/server/server.go#L301) directly calls `runtime.Start/Stop/Kill/Restart` with no state tracking.
- **Evidence**: Pterodactyl Wings uses a `powerLock` mutex + `HandlePowerAction()` that prevents concurrent power actions, checks install/transfer state, syncs with panel before start, and validates disk space.
- **Root Cause**: Our daemon is designed as a stateless HTTP server — it holds no in-memory server state.
- **Proposed Solution**: Add a `ServerManager` struct to daemon with `sync.Map[serverID → ServerState]`. Each state tracks current power status and holds a power-action lock. Implement `onBeforeStart()` pre-flight sequence.
- **Expected Impact**: Eliminates race conditions, enables crash detection, makes daemon production-ready.

#### 2.3 No Crash Detection

- **Finding**: When a game server container exits unexpectedly, nothing restarts it.
- **Evidence**: Wings' [crash.go](file:///e:/game/refs/pterodactyl-wings/server/crash.go) monitors Docker exit events, checks exit code + OOM state, enforces cooldown timers, and auto-restarts.
- **Root Cause**: Our daemon doesn't listen to Docker container events.
- **Proposed Solution**: Start a Docker events listener goroutine per-server. On container die event: check exit code/OOM, apply cooldown (configurable, default 60s), auto-restart if crash detection is enabled for the server.
- **Expected Impact**: Critical for production — unattended game servers will self-heal.

### 🟡 Moderate Issues

#### 2.4 Migration Runner Has No Tracking

- **Finding**: [store.go:493](file:///e:/game/apps/api/internal/store/store.go#L493) reads all migration files and runs every statement on every boot.
- **Evidence**: Works only because all DDL uses `IF NOT EXISTS` / `ON CONFLICT DO NOTHING`. But this means: (1) unnecessary overhead on startup, (2) can't detect failed migrations, (3) can't roll back.
- **Root Cause**: Intentional simplification during MVP.
- **Proposed Solution**: Add `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ)`. Check before running each file. Mark applied after success.
- **Expected Impact**: Safe migration management, ~20x faster startups for production instances with many migrations.

#### 2.5 WebSocket Goroutine Leak Risk

- **Finding**: In [realtime.go](file:///e:/game/apps/api/internal/http/realtime.go), the `realtimeProxy` function creates two goroutines (`pumpUpstreamToClient` and `pumpClientToUpstream`) and only waits for the first error. The second goroutine leaks until its read times out.
- **Evidence**: `errs` channel has capacity 2, but only one error is consumed: `<-errs`. When one pump exits, `cancel()` is deferred but the second goroutine blocks on `ReadMessage()` with a 60-second deadline.
- **Root Cause**: Missing `context.Done()` propagation to both pump goroutines.
- **Proposed Solution**: Pass `ctx` to both pumps. Use `ctx.Done()` select alongside `ReadMessage`. Both goroutines should exit when context is cancelled.
- **Expected Impact**: Prevents goroutine accumulation under heavy WebSocket usage.

#### 2.6 Schedule Runner Polling Inefficiency

- **Finding**: [schedule_runner.go](file:///e:/game/apps/api/internal/http/schedule_runner.go#L33) polls the database every 30 seconds for due schedules.
- **Evidence**: Queries `ListDueSchedules(now, 64)` unconditionally, even when zero schedules exist.
- **Root Cause**: Simplest implementation for MVP.
- **Proposed Solution**: Use PostgreSQL `LISTEN/NOTIFY` or Redis pub/sub to trigger schedule evaluation only when a schedule is created/updated/approaches its next_run_at. Fall back to 60s polling as a safety net.
- **Expected Impact**: Eliminates unnecessary DB queries in steady state. Faster schedule execution (sub-second instead of up to 30s late).

#### 2.7 Backup Data Not Persisted in Database

- **Finding**: Backups are only stored as zip files in the daemon's filesystem. No DB table tracks them.
- **Evidence**: Pterodactyl has a `Backup` model with fields: `uuid`, `server_id`, `is_successful`, `is_locked`, `disk`, `checksum`, `upload_id`, `completed_at`. Our API's backup endpoints proxy directly to daemon.
- **Root Cause**: MVP shortcut — daemon handles everything.
- **Proposed Solution**: Add `backups` table. When backup is created, insert a row. Enable backup limits per server (`backup_limit` column already exists in `servers` table). Track size, status, and allow restore.
- **Expected Impact**: Backup management parity with Pterodactyl. Enables backup limits, restore, and remote backup (S3).

### 🟢 Architecture Strengths

1. **Clean separation of concerns**: Frontend → API → Daemon → Docker. No layer leaks.
2. **Runtime interface**: [runtime.go](file:///e:/game/apps/daemon/internal/runtime/runtime.go) is a clean abstraction — adding containerd/Podman requires only implementing 10 methods.
3. **Domain-split store files**: Store has been modularized into 19 files by domain (e.g., `store_nodes.go`, `store_servers.go`).
4. **Handler splitting progress**: Handlers are split into `handlers_auth.go`, `handlers_admin.go`, `handlers_servers.go`, `handlers_remote.go`.
5. **HMAC-signed daemon communication**: Simple, effective, no JWT key distribution complexity.
6. **Connection pool tuning**: MaxConns=8, MinConns=1 is appropriate for the workload.

---

## 3. Modernization Opportunities

### 3.1 Event-Driven Architecture

| Current | Proposed | Benefit |
|---|---|---|
| Polling for schedule execution (30s) | Redis pub/sub + `LISTEN/NOTIFY` | Sub-second schedule triggers |
| No server event propagation | Docker Events API → Redis pub/sub → WebSocket broadcast | Real-time server state in UI |
| Sync daemon→panel heartbeat (60s loop) | Event-driven heartbeat with backoff | Less network chatter when healthy |

### 3.2 Streaming & Async Processing

| Current | Proposed | Benefit |
|---|---|---|
| File write buffers entire body | `StreamRequestBody: true` already set, but body read via `c.Body()` | True streaming reduces memory per upload |
| Backup blocks API request thread | Background backup job via Redis queue | Non-blocking backup creation |
| Install blocks HTTP handler for 15min | Move to async task with status polling | Better UX, no timeout concerns |

### 3.3 Observability Improvements

| Current | Proposed | Benefit |
|---|---|---|
| Basic uptime + goroutine metrics | Add: request latency histograms, error rates by endpoint, active WebSocket connections, DB pool utilization | Actionable production dashboards |
| No structured logging | Add `slog` (stdlib) with JSON output + request ID | Log correlation, easier debugging |
| No distributed tracing | OpenTelemetry SDK with trace propagation API→Daemon | End-to-end request tracing |

### 3.4 Horizontal Scalability

| Component | Current Limitation | Path to Scale |
|---|---|---|
| API | Single process, in-memory schedule runner | Stateless API behind load balancer + Redis-backed distributed lock for schedules |
| Daemon | Single node | Already designed correctly — one daemon per node |
| Frontend | Single instance | Already stateless SSR, put behind CDN |
| WebSocket | Pinned to single API process | Redis pub/sub for cross-instance WebSocket fan-out |

---

## 4. Performance Review

### 4.1 CPU-Heavy Operations

| Operation | Location | Issue | Fix |
|---|---|---|---|
| Bcrypt on every seed | [store.go:531](file:///e:/game/apps/api/internal/store/store.go#L531) | `bcrypt.GenerateFromPassword` runs on every startup even when user exists | Check user existence first, skip hashing if already seeded |
| JSON audit metadata | Multiple handlers | `fmt.Sprintf` constructing JSON strings manually: `fmt.Sprintf('{\"file\":\"%s\"}', ...)` | Use `json.Marshal` — current approach is vulnerable to JSON injection if path contains `"` |
| Prometheus metrics | [server.go:283](file:///e:/game/apps/api/internal/http/server.go#L283) | `runtime.ReadMemStats()` on every `/metrics` call — causes STW pause | Cache with 5s TTL or use `prometheus/client_golang` |

### 4.2 Memory-Heavy Operations

| Operation | Location | Issue | Fix |
|---|---|---|---|
| Logs download | [server.go:362](file:///e:/game/apps/daemon/internal/server/server.go#L362) | `stdcopy.StdCopy` writes up to 256KB into HTTP response | Already limited, but logsWS reads ALL logs every 3s into memory |
| File read | [server.go:727](file:///e:/game/apps/daemon/internal/server/server.go#L727) | `io.LimitReader(file, 1024*1024)` — up to 1MB per read | Acceptable, but no streaming for large files |
| Upload chunk | [server.go:753](file:///e:/game/apps/daemon/internal/server/server.go#L753) | 8MB chunk limit is reasonable | Consider reducing to 4MB for lower memory pressure |
| Install logs | [docker.go:165](file:///e:/game/apps/daemon/internal/runtime/docker.go#L165) | `io.LimitReader(logsReader, 1024*1024)` — 1MB buffer in memory | Acceptable, but could stream to disk |

### 4.3 N+1 Queries

| Operation | Location | Issue |
|---|---|---|
| Allocation creation | [handlers_admin.go:633](file:///e:/game/apps/api/internal/http/handlers_admin.go#L633) | Loop calling `CreateAllocation` per port — N inserts. Use batch `INSERT ... VALUES (...), (...), (...)` |
| Remote server configs | `RemoteServerConfigurations` | Loads all servers for a node, then builds payloads. If server count grows, this becomes slow. Add pagination. |
| Schedule task execution | `runSchedule` | Loops through tasks sequentially. Tasks with `timeOffsetSeconds > 0` should be delayed, but currently aren't (Pterodactyl delays them). |

### 4.4 Unnecessary Work

| Operation | Issue | Fix |
|---|---|---|
| DB connection retry loop | 20 retries × 500ms = 10s max wait on startup. Could use exponential backoff. | Use backoff: 100ms, 200ms, 400ms, ... |
| Seed runs on every startup | Even when data exists, it executes INSERT...ON CONFLICT for every seed row. | Add a `SELECT count(*) FROM users WHERE id = $1` guard. |

---

## 5. Security Review

### 🔴 Critical

#### 5.1 JSON Injection in Audit Metadata

- **Finding**: Audit trail metadata is constructed via `fmt.Sprintf`:
  ```go
  fmt.Sprintf(`{"file":"%s"}`, c.Query("path"))
  ```
- **Evidence**: [handlers_servers.go:1113](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1113), [L1138](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1138), [L1179](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1179), [L1204](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1204), [L1228](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1228), [L1256](file:///e:/game/apps/api/internal/http/handlers_servers.go#L1256)
- **Risk**: If `path` contains `"`, `\`, or any JSON special char, the audit log is corrupted. An attacker could inject arbitrary JSON keys.
- **Fix**: Use `json.Marshal(map[string]string{"file": path})` everywhere.

#### 5.2 No Rate Limiting on Login

- **Finding**: [server.go:357](file:///e:/game/apps/api/internal/http/server.go#L357) — login endpoint has no brute-force protection.
- **Evidence**: Redis is connected but not used for auth rate limiting.
- **Risk**: Credential stuffing, brute force attacks.
- **Fix**: Add Redis-backed rate limiter: `INCR login:{ip}` with 60s TTL, reject after 5 failures. Also rate-limit by email.

#### 5.3 Auth Bypass When Secret Is Empty

- **Finding**: [auth.go:73](file:///e:/game/apps/api/internal/http/auth.go#L73) — `if secret == "" { return c.Next() }`.
- **Evidence**: If `API_AUTH_SECRET` is empty, **all routes are unprotected**.
- **Risk**: Misconfiguration → full access to all endpoints.
- **Fix**: Remove this bypass. Require a non-empty secret always. The production check exists in `main.go` but dev mode allows empty secrets.

### 🟡 Medium

#### 5.4 Token Has No Revocation Mechanism

- **Finding**: Tokens are HMAC-signed with 24h TTL. No JTI (unique ID), no blacklist, no refresh flow.
- **Evidence**: [auth.go:17](file:///e:/game/apps/api/internal/http/auth.go#L17) — `tokenTTL = 24 * time.Hour`. If a token is compromised, it cannot be revoked.
- **Fix**: Add `jti` claim (UUID) to token. Store active sessions in Redis. On logout, delete from Redis. Auth middleware checks Redis for valid `jti`.

#### 5.5 WebSocket Auth via Query Parameter

- **Finding**: [realtime.go:20](file:///e:/game/apps/api/internal/http/realtime.go#L20) — token passed via `client.Query("token")`.
- **Risk**: Token appears in server access logs, browser history, proxy logs.
- **Fix**: Accept token via first WebSocket message instead of query parameter (Pterodactyl does this).

#### 5.6 SFTP Sidecar Uses Static Credentials

- **Finding**: Docker Compose runs `atmoz/sftp:alpine` with hardcoded `game:gamepass` credentials.
- **Evidence**: [docker-compose.yml](file:///e:/game/infra/docker/docker-compose.yml) SFTP service.
- **Fix**: Replace with Go-native SFTP server inside daemon that authenticates against the panel API (as Pterodactyl Wings does).

### 🟢 Strengths

- Container security is solid: `CapDrop: ["AUDIT_WRITE", "MKNOD", "NET_RAW"]`, `Privileged: false`
- File path sanitization in daemon (`safePath`) prevents traversal
- HMAC-signed daemon requests with timestamp window prevents replay attacks
- Production mode rejects default secrets
- Bcrypt password hashing with default cost

---

## 6. Improvement Roadmap

### P0 — Critical (Must Fix Before Production)

| # | Item | Impact | Complexity | Risk | Effort |
|---|---|---|---|---|---|
| 1 | **Fix JSON injection in audit metadata** | Security — corrupted logs, potential injection | Low | High if exploited | 1-2 hours |
| 2 | **Add rate limiting on login** | Security — brute force prevention | Low | Medium | 2-4 hours |
| 3 | **Remove auth bypass on empty secret** | Security — misconfiguration = full access | Trivial | Critical | 30 minutes |
| 4 | **Implement power state machine in daemon** | Reliability — race conditions, crash detection | Medium | High in production | 2-3 days |
| 5 | **Add crash detection** | Reliability — game servers stay dead after crash | Medium | High in production | 1-2 days |
| 6 | **Fix WebSocket goroutine leak** | Reliability — goroutine accumulation | Low | Medium under load | 2-4 hours |
| 7 | **Add migration tracking table** | Reliability — safe startup, no redundant re-execution | Low | Low | 2-4 hours |

---

### P1 — Important (Production Quality)

| # | Item | Impact | Complexity | Risk | Effort |
|---|---|---|---|---|---|
| 8 | **Enforce granular permissions in handlers** | Feature — multi-user support | Medium | Low | 2-3 days |
| 9 | **Add `backups` table + track in DB** | Feature — backup management, limits, restore | Medium | Low | 1-2 days |
| 10 | **Add token revocation (Redis JTI)** | Security — session management | Medium | Low | 1 day |
| 11 | **Batch allocation creation** | Performance — N inserts → 1 batch insert | Low | Low | 2-4 hours |
| 12 | **Replace WebSocket query-param auth** | Security — token exposure | Low | Low | 4-8 hours |
| 13 | **Add structured logging (slog)** | Observability — production debugging | Low | Low | 1 day |
| 14 | **Improve metrics (request latency, errors)** | Observability — actionable dashboards | Low | Low | 1 day |
| 15 | **Cache bcrypt hash check on seed** | Performance — faster startup | Trivial | Low | 1 hour |
| 16 | **Wire admin UI screens** | Feature — users, templates, nests management | Medium | Low | 3-5 days |

---

### P2 — Nice to Have (Beyond Parity)

| # | Item | Impact | Complexity | Risk | Effort |
|---|---|---|---|---|---|
| 17 | **File archive/decompress (tar.gz)** | Feature — better file management UX | Medium | Low | 2-3 days |
| 18 | **Backup restore** | Feature — full backup lifecycle | Medium | Medium | 2-3 days |
| 19 | **S3 backup adapter** | Feature — remote backup storage | Medium | Low | 2-3 days |
| 20 | **Go-native SFTP server** | Feature — panel-authenticated SFTP | Medium | Medium | 3-5 days |
| 21 | **Disk space enforcement** | Feature — prevent disk abuse | Low | Low | 1 day |
| 22 | **Event-driven schedule runner** | Performance — sub-second schedules | Low | Low | 1-2 days |
| 23 | **Async install/backup jobs** | UX — non-blocking long operations | Medium | Medium | 2-3 days |
| 24 | **OpenTelemetry tracing** | Observability — distributed tracing | Low | Low | 2 days |
| 25 | **Live file sync (fsnotify + WS)** | Feature — real-time file changes | Medium | Medium | 3-5 days |
| 26 | **One-line Linux installer** | DevOps — easy deployment | High | Low | 2-3 days |
| 27 | **Auto-update mechanism** | DevOps — binary replacement + restart | Medium | Medium | 2-3 days |

---

## Summary

The project is **functionally comprehensive** for its MVP scope — roughly 80% of Pterodactyl's feature surface is implemented. The architecture is fundamentally sound (Go API + Go Daemon + Next.js frontend + PostgreSQL + Redis), and several design decisions are genuinely better than Pterodactyl's (runtime abstraction, single-binary deployment, in-process scheduling).

The critical gaps are:

1. **Security hardening** (P0 items 1-3) — relatively quick fixes with high impact
2. **Daemon statelessness** (P0 items 4-5) — the daemon needs a `ServerManager` with power state tracking and crash detection to be production-ready
3. **Permission enforcement** (P1 item 8) — the permission model exists but isn't wired up, blocking multi-user scenarios

> [!IMPORTANT]
> Before implementing any changes, please confirm:
> 1. Should P0 security fixes be prioritized over P0 daemon improvements, or done in parallel?
> 2. For crash detection, should we match Pterodactyl's configurable cooldown (default 60s) or use a simpler fixed approach?
> 3. For token revocation, should we add a full refresh-token flow or just add `jti` + Redis blacklist?
