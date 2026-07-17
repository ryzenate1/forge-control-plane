# Comparative Audit: GamePanel (Our Implementation) vs. Pterodactyl Panel (Reference)

**Audit Date:** 2026-07-15
**Auditor:** Agent 1 — File-by-file comparative analysis
**Scope:** `gamepanel/beacon/`, `gamepanel/forge/api/`, `gamepanel/forge/web/` vs. `reference/petrodactylpanel/` (Pterodactyl Panel)

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Authentication & Session Management](#2-authentication--session-management)
3. [Server Management](#3-server-management)
4. [Database Design](#4-database-design)
5. [API Design](#5-api-design)
6. [WebSocket / Real-time Handling](#6-websocket--real-time-handling)
7. [File Management](#7-file-management)
8. [Backups](#8-backups)
9. [Scheduling](#9-scheduling)
10. [Security](#10-security)
11. [Frontend Architecture](#11-frontend-architecture)
12. [Feature Comparison Matrix](#12-feature-comparison-matrix)
13. [Gaps & Recommendations](#13-gaps--recommendations)

---

## 1. Architecture Overview

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Backend Language | Go (API: `forge/api/`, Daemon: `beacon/`) | PHP (Laravel framework) |
| Frontend Language | TypeScript (Next.js / React) | Vue.js 2 + TypeScript |
| Database | PostgreSQL (raw SQL migrations, pgx/v5) | MySQL/MariaDB (Eloquent ORM) |
| API Framework | Go Fiber (`gofiber/fiber/v2`) | Laravel routing + Fractal transformers |
| Daemon | Go binary (`beacon/`) — Docker container mgmt | Wings (Go) — Docker container mgmt |
| Real-time | WebSocket proxy via panel to daemon | WebSocket proxy via panel to Wings |
| Process Model | Single Go binary + separate daemon binary | PHP-FPM + Go binary (Wings) |
| Total Files | 55 Go (beacon) + 141 Go (forge/api) + 116 TSX (forge/web) | 563 PHP + 215 DB + 100+ Vue/TS |

### Key Architectural Differences

**GamePanel** uses a monolithic Go API server with direct PostgreSQL access via `pgx/v5`. The frontend is a Next.js React app with server-side rendering. The daemon (`beacon/`) runs on each node as a separate Go binary managing Docker containers.

**Pterodactyl** uses Laravel (PHP) with Eloquent ORM for the panel, and a separate Go binary (Wings) for the daemon. The frontend is a Vue.js 2 SPA with webpack.

### File Path Comparison

| Concern | GamePanel (Go) | Pterodactyl (PHP) |
|---------|----------------|-------------------|
| Core HTTP server | `forge/api/internal/http/server.go` | `routes/api-client.php` + Controllers |
| Auth middleware | `forge/api/internal/http/auth.go` | `app/Http/Middleware/Authenticate.php` |
| Server model | `forge/api/internal/store/store_servers.go` | `app/Models/Server.php` |
| User model | `forge/api/internal/store/store_users.go` | `app/Models/User.php` |
| Backup store | `forge/api/internal/store/store_backups.go` | `app/Services/Backup/` |
| Schedule runner | `forge/api/internal/http/schedule_runner.go` | `app/Console/Commands/Schedule/` |
| WebSocket proxy | `forge/api/internal/http/realtime.go` | `app/Http/.../WebsocketController.php` |
| File management | `beacon/internal/server/server.go` | `app/Http/.../FileController.php` |
| Security | `forge/api/internal/http/middleware_*.go` | Laravel CSRF + Middleware stack |

---

## 2. Authentication & Session Management

### 2.1 Token System Comparison

| Feature | GamePanel | Pterodactyl |
|---------|-----------|-------------|
| Token format | Custom HMAC-SHA256 signed JSON (base64.payload.base64.sig) | Laravel session cookies + bcrypt API keys |
| Session storage | PostgreSQL with `session_version` for instant revocation | Laravel sessions table + `remember_token` |
| JWT-like tokens | Custom `tokenClaims`: sub, email, role, jti, ver, exp | N/A (session-based) |
| Token TTL | 24 hours (`tokenTTL`) | Configurable session lifetime |
| Token revocation | `session_version` counter + JTI blacklist (`jwt_revocations`) | Session deletion + token rotation |
| Cookie name | `__Host-forge_session` (host-only prefix) | `laravel_session` |
| CSRF protection | Double-submit cookie: `__Host-forge_csrf` + `X-CSRF-Token` | Laravel `VerifyCsrfToken` middleware |
| 2FA | TOTP with encrypted-at-rest secrets, recovery tokens (bcrypt), configurable policy (none/admin/all) | TOTP via google2fa, plaintext TOTP secret, recovery codes |
| API keys | SHA-256 hash lookup, constant-time comparison, IP whitelist | bcrypt-hashed tokens with permission scopes |
| OAuth2 | Built-in client_credentials flow (server-scoped + account-scoped) | Not built-in (package-dependent) |

### 2.2 Authentication Flow

| Step | GamePanel | Pterodactyl |
|------|-----------|-------------|
| Login | POST /api/v1/auth/login, bcrypt verify, issue HMAC token + Set-Cookie | POST /api/auth/login, bcrypt verify, create session |
| 2FA checkpoint | POST /api/v1/auth/login/checkpoint, TOTP or recovery token | POST /api/auth/login/checkpoint, TOTP verify |
| Auth middleware | Checks Bearer (session/OAuth/API key) or session cookie | Laravel auth:api + middleware stack |
| Password reset | Custom token, bcrypt verify, `session_version` increment | Laravel CanResetPassword trait |
| Session invalidation | Increment `session_version` — instant global revoke | Manual session deletion |

### 2.3 GamePanel Auth Files

| File | Purpose | Lines |
|------|---------|-------|
| `forge/api/internal/http/auth.go` | Token create/parse, HMAC, middleware, session cookies, 2FA enforcement | 514 |
| `forge/api/internal/http/session_cookie.go` | Cookie config, CSRF token gen, `__Host-` prefix | 124 |
| `forge/api/internal/http/middleware_csrf.go` | Double-submit CSRF, Origin/Sec-Fetch-Site checks | 91 |
| `forge/api/internal/http/handlers_auth.go` | Login, logout, API keys, 2FA, password/email change, sessions | ~496 |
| `forge/api/internal/store/store_2fa.go` | TOTP gen/verify, recovery tokens, encrypted secrets | 232 |
| `forge/api/internal/store/store_users.go` | User CRUD, SFTP auth, session mgmt, password reset | ~1000 |

### 2.4 Strengths and Weaknesses

**GamePanel Advantages:**
- `session_version` provides instant global session invalidation (Pterodactyl lacks this)
- TOTP secrets encrypted at rest (Pterodactyl stores plaintext)
- Built-in OAuth2 with server-scoped tokens
- Configurable 2FA enforcement policy (none/admin/all)
- Double-submit CSRF with Origin + Sec-Fetch-Site validation
- `__Host-` prefixed cookies (prevents MITM on non-HTTPS)

**GamePanel Weaknesses:**
- Custom token format is non-standard (not RFC 7519 JWT) — no library ecosystem interop
- No rate limiting on token parsing (Pterodactyl benefits from Laravel's throttle)
- No "remember me" long-lived token option

---

## 3. Server Management

### 3.1 Server Model Comparison

| Field | GamePanel | Pterodactyl |
|-------|-----------|-------------|
| ID format | UUID (PostgreSQL native) | Auto-increment int + UUID + uuid_short |
| Ownership | `owner_id UUID` | `owner_id INTEGER` |
| Resources | memory_mb, cpu_shares, cpu, disk_mb, swap_mb, io_weight, threads, oom_disabled | memory, swap, disk, io, cpu, threads, oom_disabled |
| Limits | database_limit, backup_limit, allocation_limit | database_limit, backup_limit, allocation_limit |
| Service model | `template_id` (simple: name, image, startup, default_memory) | `nest_id` + `egg_id` (rich: install scripts, config, denylists, variables) |
| Node relation | `node_id UUID` | `node_id INTEGER` |
| Transfer | `server_transfers` table with state machine | `server_transfers` table with similar states |
| Multi-region | RequiredNode, PreferredNode, RegionID support | Single node_id |
| Docker Image | `docker_image TEXT` | `image VARCHAR` |
| User resource limits | Per-user: cpu_limit, memory_mb_limit, disk_mb_limit, backup_limit, database_limit, allocation_limit, subuser_limit, schedule_limit, server_limit | Not built-in (global settings only) |

### 3.2 Server Creation

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Request type | `CreateServerRequest` with 20+ fields | ServerRequest form request validation |
| Auto-placement | Region-based node selection | Manual node selection |
| Egg variables | `startup_variables` map in request | ServerVariable pivot table |
| Install trigger | Daemon `install()` via signed request | Daemon install via signed request |
| Post-install | Notify panel of install status | Mark server as installed |

### 3.3 Daemon (Beacon) Server Handling

The `beacon/internal/server/server.go` file (2728 lines) is the largest single file in the codebase. It handles:

| Operation | Method | Lines |
|-----------|--------|-------|
| Container create | `create()` | 355-462 |
| Install | `install()` / `installWS()` | 638-861 |
| Power control | `power()` | 889-909 |
| Stats streaming | `statsWS()` | 1115-1162 |
| Log streaming | `logsWS()` | 1164-1196 |
| Console WebSocket | `consoleWS()` | 1221-1286 |
| File operations | `listFiles()`, `readFile()`, `writeFile()`, `uploadFileChunk()` | 1288-1568 |
| Backup operations | `createBackup()`, `listBackups()`, `downloadBackup()`, `restoreBackup()` | 959-1113 |
| Transfer | `startTransfer()`, `receiveTransferArchive()` | 1990-2129 |
| Path validation | `safePath()` | 2141-2199 |

### 3.4 Key Differences

1. **UUID primary keys** — GamePanel uses UUIDs everywhere. Pterodactyl uses auto-increment integers. UUIDs prevent sequential enumeration attacks.

2. **Simpler template model** — GamePanel has `server_templates` (name, image, startup, default_memory). Pterodactyl has nests/eggs/variables/install-scripts/config-parsing/file-denylists — much richer but more complex.

3. **User resource limits** — GamePanel has per-user limits (CPU, memory, disk, backups, databases, allocations, subusers, schedules, servers). Pterodactyl lacks this; limits are global settings.

4. **Multi-region placement** — GamePanel supports region-based node selection with `RequiredNode`/`PreferredNode`. Pterodactyl requires manual node selection.

5. **IO weight** — GamePanel uses `io_weight` (Docker blkio weight). Pterodactyl uses `io` (10-1000 range, maps to device read/write bps).

---

## 4. Database Design

### 4.1 Migration Strategy

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Count | 50 numbered SQL files (001-050) | 100+ PHP migration files (2016-2024) |
| Tool | Raw SQL, custom migrator | Laravel `artisan migrate` |
| Engine | PostgreSQL 14+ (UUID, JSONB, TSTAMPTZ, LISTEN/NOTIFY) | MySQL 5.7+ / MariaDB 10.2+ |
| Approach | Forward-only, additive | Forward-only with some destructive |

### 4.2 Table Mapping

| Table | GamePanel | Pterodactyl |
|-------|-----------|-------------|
| users | UUID PK, email, password_hash, role TEXT, session_version, disabled, use_totp, totp_secret_encrypted | int PK, uuid, username, email, password, root_admin, use_totp, totp_secret (plaintext) |
| servers | UUID PK, node_id, owner_id, template_id, name, status, memory_mb, cpu, disk_mb, docker_image, startup_command | int PK, uuid, uuid_short, node_id, owner_id, nest_id, egg_id, name, memory, swap, disk, cpu, image, startup |
| nodes | UUID PK, name, region, base_url, status, token_hash, last_seen_at | int PK, name, fqdn, scheme, behind_proxy, maintenance_mode, memory, disk |
| allocations | UUID PK, node_id, server_id, ip, port, alias, notes | int PK, node_id, ip, port, server_id |
| schedules | UUID PK, server_id, name, cron fields, only_when_online, enabled, last_run_at, next_run_at | int PK, server_id, name, cron fields, only_when_online, enabled |
| schedule_tasks | UUID PK, schedule_id, sequence, action, payload JSONB, time_offset_seconds, continue_on_failure | int PK, schedule_id, action, payload JSON, time_offset, continue_on_failure |
| backups | UUID PK, server_id, name, checksum, size, status, is_locked, retry_count | int PK, server_id, name, checksum, size, status, completed_at |
| api_keys | UUID PK, user_id, token_hash, token_prefix, last_used_at, expires_at | int PK, user_id, token (bcrypt), token_type, allowed_ips, memo |
| activity_logs | UUID PK, actor_id, event, target_type, target_id, properties JSONB, timestamp | int PK, user_id, event, subject_type, subject_id, properties JSON |
| database_hosts | UUID PK, node_id, engine, name, host, port, username, password, max_databases | int PK, host, port, name, username, password, max_databases |
| server_databases | UUID PK, server_id, database_host_id, database_name, username, password, remote | int PK, server_id, database_host_id, database, username, password, remote |
| subusers | (via UserCanAccessServer query) | int PK, server_id, user_id, permissions JSON |
| nests/eggs | Simple `server_templates` table | Complex nests/eggs/egg_variables hierarchy |
| mounts | UUID PK, name, description, source, target, read_only, user_mountable | int PK, name, description, source, target, read_only, user_mountable |
| locations | UUID PK, short_name, long_name | int PK, short, long |
| regions | UUID PK, name, slug, description, enabled | N/A (locations serve this purpose) |

### 4.3 GamePanel-Specific Tables

These tables exist in GamePanel but have no direct Pterodactyl equivalent:

| Table | Purpose | Migration |
|-------|---------|-----------|
| `sessions` | Session tracking with IP, user agent, expiry, revocation | 041 |
| `jwt_revocations` | JWT JTI blacklist for instant token revocation | 041 |
| `recovery_tokens` | 2FA recovery tokens (bcrypt-hashed) | 018 |
| `oauth_clients` | OAuth2 client registrations | 035 |
| `oauth_revoked_tokens` | OAuth2 token revocation list | 035 |
| `password_reset_tokens` | Password reset tokens with expiry | 038 |
| `panel_settings` | Global panel configuration (JSONB) | 032-037 |
| `node_heartbeats` | Node heartbeat tracking | 010 |
| `server_state_events` | Server state persistence | 021 |
| `evacuation_plans` | Node evacuation planning | 022 |
| `migration_engine` | Server migration tracking | 023 |
| `observability_metrics` | System metrics storage | 024 |
| `placement_reservations` | Server placement reservations | 026 |
| `recovery_coordinators` | Node recovery coordination | 027 |
| `webhooks` / `webhook_deliveries` | Webhook system | 023/039 |
| `plugins` | Plugin system | 036 |
| `async_deliveries` | Async message delivery | 044 |

### 4.4 Pterodactyl-Specific Tables

| Table | Purpose |
|-------|---------|
| `nests` | Service category hierarchy |
| `eggs` | Service definitions with install scripts, config parsing |
| `egg_variables` | Configurable variables per egg |
| `server_variables` | Per-server variable values |
| `service_options` | Legacy service options |
| `service_variables` | Legacy service variables |
| `tasks` / `tasks_log` | Legacy scheduling (pre-schedule migration) |
| `jobs` / `failed_jobs` | Laravel queue system |
| `notifications` | Laravel notification system |

---

## 5. API Design

### 5.1 Route Structure

| Route Group | GamePanel | Pterodactyl |
|-------------|-----------|-------------|
| Auth | `/api/v1/auth/*` | `/api/auth/*` |
| Account | `/account/*` | `/api/client/account/*` |
| Server list | `GET /api/v1/servers` | `GET /api/client/servers` |
| Server detail | `GET /api/v1/servers/:id` | `GET /api/client/servers/:server` |
| Server power | `POST /api/v1/servers/:id/power` | `POST /api/client/servers/:server/power` |
| Server files | `GET /api/v1/servers/:id/files` | `GET /api/client/servers/:server/files` |
| Server backups | `GET /api/v1/servers/:id/backups` | `GET /api/client/servers/:server/backups` |
| Server schedules | `GET /api/v1/servers/:id/schedules` | `GET /api/client/servers/:server/schedules` |
| Admin servers | `GET /api/v1/admin/servers` | `GET /api/application/servers` |
| Admin users | `GET /api/v1/admin/users` | `GET /api/application/users` |
| Admin nodes | `GET /api/v1/admin/nodes` | `GET /api/application/nodes` |
| Admin locations | `GET /api/v1/admin/locations` | `GET /api/application/locations` |
| Daemon comms | Signed headers (HMAC) | Signed headers (HMAC) |

### 5.2 API Response Format

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Success format | Direct JSON (`{"key": "value"}`) | Fractal-wrapped (`{"object": "list", "data": [...]}`) |
| Error format | `{"message": "error"}` | `{"errors": [{"detail": "error"}]}` |
| Pagination | Offset-based (`?page=1&per_page=25`) | Cursor-based via Fractal |
| Filtering | Query parameters | Query parameters |
| Sorting | Query parameters | Query parameters |

### 5.3 Middleware Stack

| Middleware | GamePanel | Pterodactyl |
|-----------|-----------|-------------|
| Rate limiting | Redis-backed (auth: 5/min, mutation: 30/min, read: 120/min) | Laravel `throttle` middleware |
| CSRF | Double-submit cookie pattern | Laravel `VerifyCsrfToken` |
| Security headers | Custom `SecurityHeaders()` (CSP, X-Frame-Options, HSTS, etc.) | Laravel `TrustProxies` + custom |
| IP access control | CIDR-aware allow/deny lists | Not built-in |
| Auth | `authMiddleware()` — session/OAuth/API key | `auth:api` + custom |
| Role check | `requireRole()` middleware | `$user->root_admin` checks |
| Scope check | `requireAdminScope()` for API keys | `ApiKey` permission checks |
| Server access | `requireServerPermission()` — owner/subuser/admin check | Custom `SubuserAccess` middleware |

### 5.4 Key Differences

1. **GamePanel uses a single `server.go` file** (1291 lines) with all route registration in `NewServer()`. Pterodactyl splits across many controller classes. GamePanel is more compact but harder to navigate.

2. **GamePanel has built-in rate limiting** with Redis backend and endpoint-type differentiation. Pterodactyl relies on Laravel's basic throttle middleware.

3. **GamePanel's API is versioned** (`/api/v1/`). Pterodactyl's API is unversioned.

4. **GamePanel includes IP access control middleware** with CIDR support. Pterodactyl has per-API-key IP whitelists but no global IP control.

---

## 6. WebSocket / Real-time Handling

### 6.1 Architecture

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Panel-side proxy | `realtime.go` — Fiber WebSocket → gorilla WebSocket to daemon | `WebsocketController` — Laravel pusher/WebSocket → Wings |
| Auth method | WS ticket (single-use, 60s expiry, HMAC-signed) or long-lived JWT | Token query parameter |
| Ticket store | In-memory map with Redis fallback (`wsTicketStore`) | N/A (direct token auth) |
| Origin validation | Configurable allowed origins (`API_WS_ALLOWED_ORIGINS` env var) | Configurable origins |
| Heartbeat | Read limit (1MB) + 60s read deadline + pong handler | Similar timeout-based |
| Streams | `console`, `stats`, `logs` | `console`, `stats`, `logs` |

### 6.2 GamePanel WS Ticket System

GamePanel implements a sophisticated WebSocket ticket system (`handlers_ws_ticket.go`, 228 lines):

1. Frontend calls `POST /api/v1/servers/:id/ws-ticket?stream=console`
2. Backend validates server permission (`websocket.connect`) and generates a short-lived (60s) single-use ticket
3. Ticket is HMAC-signed: `subject.HMAC-SHA256(secret, subject)`
4. On WS upgrade, the ticket is peeked (not consumed yet), then the bearer token is verified for identity binding
5. Only after successful identity binding is the ticket consumed atomically
6. The WebSocket connection is then proxied to the daemon

### 6.3 Pterodactyl WebSocket Approach

Pterodactyl uses a simpler approach:
1. Frontend connects with `?token=<jwt>` query parameter
2. Backend validates the JWT and checks server access
3. Connection is proxied to Wings

### 6.4 Key Differences

1. **GamePanel's ticket system prevents token exposure** — the JWT is never sent in the WebSocket URL query string during the actual upgrade (it's in the Authorization header or Sec-WebSocket-Protocol). Pterodactyl sends the JWT in the URL.

2. **GamePanel supports Redis-backed ticket storage** for multi-instance deployments. Pterodactyl's approach works with a single panel instance.

3. **GamePanel validates user identity on WS upgrade** — the ticket is bound to a specific user, and the bearer token must match. Pterodactyl only validates the token.

---

## 7. File Management

### 7.1 Panel-side File Operations

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| File listing | Proxied to daemon via signed request | Proxied to Wings via signed request |
| File read | Proxied to daemon | Proxied to Wings |
| File write | Proxied to daemon | Proxied to Wings |
| File upload | Chunked upload via `uploadFileChunk()` | Chunked upload via Wings |
| File download | Ticket-based: issue ticket → consume on download | Direct proxy to Wings |
| File rename | `renameFile()` with `From`/`To` | Rename endpoint |
| File delete | `deleteFile()` + `batchDeleteFiles()` | Delete endpoint |
| File copy | `copyFile()` | N/A (must download+upload) |
| File chmod | `chmodFiles()` | N/A |
| Archive | `archiveFiles()` + `decompressFile()` | N/A |
| Remote file pull | `pullRemoteFile()` — download URL to server path | N/A |

### 7.2 Daemon-side File Operations

GamePanel's beacon implements these in `server.go`:

| Operation | Method | Notes |
|-----------|--------|-------|
| List files | `listFiles()` | Reads directory, detects text vs binary |
| Read file | `readFile()` | Reads file content, returns as text |
| Write file | `writeFile()` | Writes content to file |
| Upload chunk | `uploadFileChunk()` | Handles chunked uploads with temp file assembly |
| Download | `downloadFile()` | Streams file to daemon |
| Make directory | `makeDir()` | Creates directory |
| Rename | `renameFile()` | Atomic rename |
| Delete | `deleteFile()` | Single file delete |
| Batch delete | `batchDeleteFiles()` | Multiple file delete |
| Batch rename | `batchRenameFiles()` | Multiple file rename |
| Chmod | `chmodFiles()` | Permission change |
| Copy | `copyFile()` | File copy |
| Archive | `archiveFiles()` | Create archive |
| Decompress | `decompressFile()` | Extract archive |
| Remote pull | `pullRemoteFile()` | Download URL to server |

### 7.3 File Download Ticket System

GamePanel implements a ticket-based file download system (`handlers_file_download.go`):

1. Frontend requests a download ticket: `POST /api/v1/servers/:id/files/download-ticket` with `{"path": "file.txt"}`
2. Backend issues a 60-second single-use ticket (32-byte random hex token)
3. Frontend redirects to `GET /api/v1/servers/:id/files/download?token=<ticket>`
4. Backend consumes the ticket, proxies the file from daemon with security headers

Security headers set on download:
- `Content-Disposition: attachment` (prevents inline rendering)
- `X-Content-Type-Options: nosniff`
- `Referrer-Policy: no-referrer`
- `Cache-Control: private, no-store`

### 7.4 Key Differences

1. **GamePanel has more file operations** — copy, chmod, archive, decompress, remote pull, batch rename. Pterodactyl has basic CRUD only.

2. **GamePanel's ticket-based download** prevents URL leakage and provides audit logging. Pterodactyl proxies directly.

3. **GamePanel has path validation** (`safePath()` in beacon) to prevent directory traversal. Pterodactyl relies on Laravel's path validation.

---

## 8. Backups

### 8.1 Backup Architecture

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Storage adapters | Local + S3 (`backup/local.go`, `backup/s3.go`) | Local + S3 (via Wings config) |
| Backup format | ZIP archives | ZIP archives |
| Checksum | SHA-256 (`calculateChecksum()`) | SHA-256 |
| Backup locking | `is_locked` boolean with lock/unlock API | N/A |
| Retention policy | Configurable `retention_days` + `auto_cleanup` | Manual deletion only |
| Auto cleanup | Runs in schedule runner tick (every minute) | Manual admin action |
| Retry logic | `retry_count`, `last_retry_at` fields | N/A |
| Status callbacks | `status_callback` URL for async notification | N/A |
| Per-server limits | `backup_limit` on server model | `backup_limit` on server model |
| Daemon operations | create, list, download, restore, delete | create, list, download, restore, delete |

### 8.2 Backup Table Schema

| Column | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| ID | `uuid UUID PK` | `id INT PK` |
| Server | `server_id UUID FK` | `server_id INT FK` |
| Name | `name TEXT` | `name VARCHAR` |
| Checksum | `checksum TEXT` | `checksum VARCHAR` |
| Size | `size BIGINT` | `size BIGINT` |
| Status | `status TEXT` (pending/completed/failed) | `status TEXT` (building/completed/failed) |
| Upload ID | `upload_id TEXT` (S3 multipart) | N/A |
| Completed | `completed_at TIMESTAMPTZ` | `completed_at TIMESTAMP` |
| Locked | `is_locked BOOLEAN` | N/A |
| Retry count | `retry_count INTEGER` | N/A |
| Last retry | `last_retry_at TIMESTAMPTZ` | N/A |
| Status message | `status_message TEXT` | N/A |
| Status callback | `status_callback TEXT` | N/A |

### 8.3 Key Differences

1. **GamePanel has backup locking** — prevents accidental deletion of important backups. Pterodactyl has no equivalent.

2. **GamePanel has automatic retention cleanup** — configurable per-panel (`retention_days`, `auto_cleanup`). Pterodactyl requires manual cleanup.

3. **GamePanel tracks retry attempts** — `retry_count` and `last_retry_at` for backup reliability monitoring.

4. **GamePanel supports S3 multipart uploads** — `upload_id` tracks S3 multipart upload state. Pterodactyl handles this in Wings but doesn't track it in the database.

5. **GamePanel has backup status callbacks** — async notification when backup completes. Pterodactyl relies on polling.

---

## 9. Scheduling

### 9.1 Schedule Architecture

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Runner | In-process `scheduleRunner` (Go) | Laravel `ProcessRunnableCommand` (artisan) |
| Cron format | 5-field (minute, hour, dom, month, dow) | 5-field (same) |
| Task actions | power, backup, command | power, backup, command, notify, script |
| Lease-based | Yes — `ClaimDueSchedule()` with worker ID | No (lock-based) |
| Lease renewal | `ExtendScheduleLease()` every minute during execution | N/A |
| Wake optimization | `nextWakeDelay()` calculates precise next wake time | Polling every minute |
| Event-driven | LISTEN/NOTIFY on schedule changes | artisan schedule:run (1-minute cron) |
| Backup cleanup | Integrated into schedule tick | Not integrated |
| History | `schedule_runs` + `schedule_task_runs` tables | `tasks_log` table |
| Worker ID | `schedule-<uuid>` per instance | Single artisan process |
| Claim limit | 64 per tick | 1 per run |

### 9.2 Schedule Table Schema

| Column | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| ID | `uuid UUID PK` | `id INT PK` |
| Server | `server_id UUID FK` | `server_id INT FK` |
| Name | `name TEXT` | `name VARCHAR` |
| Cron | 5 TEXT columns (minute, hour, dom, month, dow) | 5 VARCHAR columns (same) |
| Only when online | `only_when_online BOOLEAN` | `only_when_online BOOLEAN` |
| Enabled | `enabled BOOLEAN` | `enabled BOOLEAN` |
| Last run | `last_run_at TIMESTAMPTZ` | `last_run_at TIMESTAMP` |
| Next run | `next_run_at TIMESTAMPTZ` | N/A |

### 9.3 Key Differences

1. **GamePanel uses lease-based execution** — prevents duplicate execution in multi-instance deployments. Pterodactyl uses simple locking.

2. **GamePanel has event-driven scheduling** — PostgreSQL LISTEN/NOTIFY triggers immediate execution when schedules change. Pterodactyl polls every minute.

3. **GamePanel optimizes wake time** — calculates the exact next wake delay instead of fixed 1-minute intervals. Reduces unnecessary polling.

4. **GamePanel lacks script/notify tasks** — Pterodactyl supports `script` (run server install script) and `notify` (send notification) tasks. GamePanel only has `power`, `backup`, and `command`.

5. **GamePanel has richer history** — separate `schedule_runs` (overall run status) and `schedule_task_runs` (per-task status with error messages). Pterodactyl has a simpler `tasks_log`.

---

## 10. Security

### 10.1 Security Headers

GamePanel implements comprehensive security headers in `middleware_security.go`:

| Header | Value | Purpose |
|--------|-------|---------|
| Content-Security-Policy | `default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; ...` | XSS prevention |
| X-Frame-Options | `DENY` | Clickjacking prevention |
| X-Content-Type-Options | `nosniff` | MIME sniffing prevention |
| X-XSS-Protection | `1; mode=block` | Legacy XSS filtering |
| Referrer-Policy | `strict-origin-when-cross-origin` | Referrer control |
| Permissions-Policy | `geolocation=(), microphone=(), ...` | Feature restriction |
| Strict-Transport-Security | `max-age=31536000; includeSubDomains` | HTTPS enforcement |

Pterodactyl relies on Laravel's default headers + custom middleware.

### 10.2 CSRF Protection

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Pattern | Double-submit cookie | Synchronizer token |
| Cookie | `__Host-forge_csrf` (HttpOnly=false, Secure=true) | `XSRF-TOKEN` (encrypted) |
| Header | `X-CSRF-Token` must match cookie value | `X-XSRF-TOKEN` header |
| Validation | `subtle.ConstantTimeCompare` | Laravel built-in |
| Origin check | `Origin` header + `Sec-Fetch-Site: cross-site` block | Laravel VerifyCsrfToken |
| Safe methods | GET, HEAD, OPTIONS exempted | Same |

### 10.3 Rate Limiting

| Endpoint Type | GamePanel | Pterodactyl |
|---------------|-----------|-------------|
| Auth | 5 requests/minute per IP | Laravel throttle (configurable) |
| Mutations | 30 requests/minute per IP | Laravel throttle |
| Reads | 120 requests/minute per IP | Laravel throttle |
| Login failures | Dedicated `checkLoginRateLimit()` + `recordLoginFailure()` | Laravel `throttle` |
| Backend | Redis-backed | File/session-backed |

### 10.4 IP Access Control

GamePanel has built-in IP access control (`middleware_ipaccess.go`):
- CIDR range support
- Allow list and deny list
- Trust proxy headers (X-Forwarded-For, X-Real-IP)
- Per-endpoint configuration (admin, API)

Pterodactyl has per-API-key IP whitelists but no global IP access control.

### 10.5 Path Traversal Prevention

GamePanel's beacon implements `safePath()` (57 lines):
- Resolves symlinks via `filepath.EvalSymlinks`
- Validates resolved path is within server root
- Prevents `../` traversal
- Handles symlink race conditions

Pterodactyl relies on Laravel's `realpath()` checks.

### 10.6 Security Headers on Downloads

GamePanel sets security headers on all file downloads:
```
Content-Disposition: attachment
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cache-Control: private, no-store
```

### 10.7 Key Security Differences

**GamePanel Advantages:**
- Double-submit CSRF with Origin validation (stronger than synchronizer token alone)
- IP access control with CIDR support
- Rate limiting differentiation by endpoint type
- Security headers middleware (CSP, HSTS, Permissions-Policy)
- Login failure tracking with dedicated rate limiting
- `__Host-` prefixed cookies (prevents domain downgrade)

**GamePanel Weaknesses:**
- CSP includes `'unsafe-eval'` (required for Monaco editor but weakens XSS protection)
- No Content-Security-Policy report-uri/report-to directive
- No automatic security header testing
- Custom HMAC tokens lack standard JWT validation libraries

---

## 11. Frontend Architecture

### 11.1 Technology Stack

| Aspect | GamePanel | Pterodactyl |
|--------|-----------|-------------|
| Framework | Next.js (React) | Vue.js 2 + Inertia.js |
| Language | TypeScript | TypeScript + JavaScript |
| Styling | Tailwind CSS | Tailwind CSS |
| State | React Query / SWR | Vue reactive + Vuex |
| Build | Next.js (Turbopack) | Webpack |
| Testing | Jest + React Testing Library | Jest + Vue Test Utils |
| Components | 116 TSX/TS files | ~100+ Vue/TS files |

### 11.2 Page Structure

| Page | GamePanel | Pterodactyl |
|------|-----------|-------------|
| Dashboard | `/servers` | `/` |
| Server console | `/server/[id]` | `/server/:id` |
| Server files | `/server/[id]/files` | `/server/:id/files` |
| Server backups | `/server/[id]/backups` | `/server/:id/backups` |
| Server databases | `/server/[id]/databases` | `/server/:id/databases` |
| Server schedules | `/server/[id]/schedules` | `/server/:id/schedules` |
| Server network | `/server/[id]/network` | `/server/:id/allocations` |
| Server users | `/server/[id]/users` | `/server/:id/users` |
| Server settings | `/server/[id]/settings` | `/server/:id/settings` |
| Admin overview | `/admin/overview` | `/admin/dashboard` |
| Admin servers | `/admin/servers` | `/admin/servers` |
| Admin users | `/admin/users` | `/admin/users` |
| Admin nodes | `/admin/nodes` | `/admin/nodes` |
| Account | `/account` | `/account` |
| Login | `/` (root) | `/auth/login` |

### 11.3 Frontend Component Organization

| Directory | GamePanel | Pterodactyl |
|-----------|-----------|-------------|
| Admin panels | `components/admin/Admin*.tsx` (15 components) | `resources/scripts/components/admin/` |
| Server views | `components/server/*-view.tsx` (12 components) | `resources/scripts/components/server/` |
| UI primitives | `components/ui/*.tsx` | `resources/scripts/components/core/` |
| API layer | `lib/api/*.ts` (auth, files, http) | `resources/scripts/api/` |
| Auth utils | `components/ui/auth-shell.tsx`, `auth-utils.ts` | `resources/scripts/routers/middleware/` |

### 11.4 Key Frontend Differences

1. **GamePanel uses Next.js App Router** — file-based routing with `page.tsx` files. Pterodactyl uses Laravel routes + Inertia.js.

2. **GamePanel has a dedicated `console-charts.tsx`** — real-time CPU/memory/network charts in the console view. Pterodactyl has basic stats display.

3. **GamePanel's `power-controls.tsx`** — dedicated component for server power management. Pterodactyl embeds this in the server console.

4. **GamePanel has `server-console-layout.tsx`** — a specialized layout for the server console page with sidebar navigation. Pterodactyl uses a generic layout.

---

## 12. Feature Comparison Matrix

| Feature | GamePanel | Pterodactyl | Advantage |
|---------|-----------|-------------|-----------|
| Multi-node support | Yes | Yes | Tie |
| Multi-region | Yes (Region + Location) | Yes (Location only) | GamePanel |
| UUID primary keys | Yes | No (int + UUID) | GamePanel |
| Session versioning | Yes (instant global revoke) | No | GamePanel |
| TOTP encrypted at rest | Yes | No | GamePanel |
| OAuth2 built-in | Yes (client_credentials) | No | GamePanel |
| 2FA enforcement policy | Yes (none/admin/all) | No | GamePanel |
| Per-user resource limits | Yes | No | GamePanel |
| Backup locking | Yes | No | GamePanel |
| Backup retention cleanup | Yes (auto) | No (manual) | GamePanel |
| Backup retry tracking | Yes | No | GamePanel |
| WS ticket system | Yes (single-use, HMAC-signed) | No | GamePanel |
| CSRF double-submit | Yes | No (synchronizer token) | GamePanel |
| IP access control (CIDR) | Yes | No (per-key only) | GamePanel |
| Rate limiting (tiered) | Yes (Redis-backed) | Basic (Laravel throttle) | GamePanel |
| Security headers middleware | Yes (CSP, HSTS, etc.) | Partial | GamePanel |
| Lease-based scheduling | Yes | No | GamePanel |
| Event-driven scheduling | Yes (LISTEN/NOTIFY) | No | GamePanel |
| File copy/chmod/archive | Yes | No | GamePanel |
| Remote file pull | Yes | No | GamePanel |
| Plugin system | Yes | No | GamePanel |
| Webhook system | Yes | No | GamePanel |
| Node evacuation | Yes | No | GamePanel |
| Observability metrics | Yes | No | GamePanel |
| User SSH keys | Yes | Yes | Tie |
| SFTP | Yes (custom SFTP server) | Yes (Wings SFTP) | Tie |
| Egg/variable system | Simple (templates) | Rich (nests/eggs/variables/install scripts) | Pterodactyl |
| Install scripts | Basic | Rich (with config parsing, file denylists) | Pterodactyl |
| Community ecosystem | Growing | Mature (years of plugins, eggs) | Pterodactyl |
| Documentation | Minimal | Extensive (docs.pterodactyl.io) | Pterodactyl |
| Task types | power, backup, command | power, backup, command, notify, script | Pterodactyl |
| Soft deletes | No | Yes (`deleted_at` on servers) | Pterodactyl |
| Eloquent/ORM | Raw SQL (pgx) | Eloquent ORM | Preference |
| Migration count | 50 | 100+ | Pterodactyl (maturity) |

---

## 13. Gaps & Recommendations

### 13.1 GamePanel Gaps (Missing from Pterodactyl)

| Gap | Impact | Recommendation |
|-----|--------|----------------|
| No egg/variable system | Cannot support complex game server configurations | Implement nest/egg/variable hierarchy or expand template model |
| No install scripts | Cannot run custom installation logic per game | Add install script support to templates |
| No file denylist | Users can modify critical server files | Add file denylist per template/egg |
| No soft deletes | Deleted servers cannot be recovered | Add `deleted_at` column to servers table |
| No notify/script tasks | Limited scheduling capabilities | Add `notify` and `script` task actions |
| No `username` field | Users identified only by email | Add username field to users table |
| Minimal documentation | Harder for new contributors | Write API docs, architecture docs |

### 13.2 GamePanel Improvements over Pterodactyl

These are features GamePanel already has that Pterodactyl lacks:

| Improvement | Description |
|-------------|-------------|
| Session versioning | Instant global session invalidation |
| Encrypted TOTP secrets | 2FA secrets encrypted at rest |
| Built-in OAuth2 | Client credentials with server/account scoping |
| Per-user resource limits | CPU, memory, disk, backup, database, allocation, subuser, schedule, server limits |
| Backup locking | Prevent accidental deletion |
| Auto backup retention | Configurable cleanup policy |
| WS ticket system | Single-use, HMAC-signed, identity-bound |
| IP access control | CIDR-aware allow/deny lists |
| Tiered rate limiting | Different limits for auth, mutation, read |
| File copy/chmod/archive | Extended file operations |
| Remote file pull | Download URL directly to server |
| Plugin system | Extensible via plugins |
| Webhook system | Event-driven notifications |
| Node evacuation | Planned node maintenance |
| Observability metrics | Built-in monitoring |
| Multi-region placement | Automatic node selection by region |

### 13.3 Priority Recommendations

1. **High Priority:** Implement egg/variable system — critical for game server support
2. **High Priority:** Add install script support — required for server provisioning
3. **Medium Priority:** Add soft deletes for servers
4. **Medium Priority:** Add notify/script task types to scheduler
5. **Medium Priority:** Write comprehensive API documentation
6. **Low Priority:** Add username field to users
7. **Low Priority:** Add file denylist per template
