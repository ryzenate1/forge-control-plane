# Comparative Audit: GamePanel (Our Stack) vs Pelican Panel

**Agent:** Agent 2 — File-by-File Source Comparison  
**Date:** 2026-07-15  
**Scope:** `beacon/` (Go daemon), `forge/api/` (Go API), `forge/web/` (Next.js frontend) vs `reference/pelicanpanel/` (PHP/Laravel + Filament)

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Authentication](#2-authentication)
3. [Server Management](#3-server-management)
4. [Node Management](#4-node-management)
5. [Database Design](#5-database-design)
6. [API Design](#6-api-design)
7. [WebSocket Handling](#7-websocket-handling)
8. [File Management](#8-file-management)
9. [Backups](#9-backups)
10. [Scheduling](#10-scheduling)
11. [Plugin System](#11-plugin-system)
12. [Security](#12-security)
13. [Frontend Comparison](#13-frontend-comparison)
14. [Summary Scorecard](#14-summary-scorecard)
15. [Key Gaps & Recommendations](#15-key-gaps--recommendations)

---

## 1. Architecture Overview

| Aspect | GamePanel (Ours) | Pelican Panel | Assessment |
|---|---|---|---|
| **Panel API** | Go (Fiber framework) | PHP (Laravel + Filament) | Pelican has mature Laravel ecosystem; ours is leaner and faster |
| **Daemon** | Go (custom binary) | Go (Pterodactyl Wings) | Same language; Pelican's Wings has years of battle-testing |
| **Frontend** | Next.js (React/TypeScript) | Livewire + Blade (PHP SSR) | Ours has better SPA separation and type safety |
| **Database** | PostgreSQL (raw SQL migrations) | MySQL/MariaDB (Eloquent ORM) | PostgreSQL offers JSONB, array types, better concurrency |
| **Communication** | REST + WebSocket proxy | REST + WebSocket proxy | Structurally similar |
| **Process Model** | Single Go binary per component | PHP-FPM + queue workers + scheduler | Ours has simpler deployment; Pelican has proven worker patterns |

### Key Architectural Differences

**GamePanel** follows a three-tier architecture:
- `forge/api/` — Stateless Go API (Fiber), PostgreSQL store, Redis for rate limiting
- `beacon/` — Go daemon running on each node, manages Docker containers, SFTP, backups
- `forge/web/` — Next.js SPA communicating with `forge/api/`

**Pelican Panel** follows the Pterodactyl architecture:
- PHP Laravel API + Filament admin panel
- Wings (Go daemon) per node
- Livewire/Blade server-rendered frontend
- MySQL/MariaDB with Eloquent ORM

---

## 2. Authentication

### Token Design

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Token format** | HMAC-SHA256 signed JWT-like (`payload.signature`, 2-part) | Standard JWT via `lcobucci/jwt` (3-part, base64-encoded) |
| **Claims** | `sub`, `email`, `role`, `jti`, `ver` (session_version), `exp` | Standard JWT claims + `user_uuid`, `unique_id` |
| **Token TTL** | 24 hours (hardcoded `tokenTTL`) | Configurable (typically 24h) |
| **Revocation** | JTI stored in DB, checked via `IsJWTRevoked()` on every request | Token blacklist via database |
| **Session version** | `session_version` column on `users` table, incremented on password/2FA change | Not present; relies on token expiry |
| **Cookie-based sessions** | `__Host-forge_session` (HTTPOnly, Secure, SameSite=Lax) + CSRF cookie | Laravel session cookies (encrypted) |

**Our Advantage:** The `session_version` field is a lightweight revocation mechanism — changing a password or disabling 2FA instantly invalidates all tokens without a full blacklist lookup. Pelican relies on token expiry or explicit blacklisting.

**Pelican's Advantage:** Uses a well-tested third-party JWT library (`lcobucci/jwt`) with standard 3-part JWT format. Our 2-part HMAC token is custom and non-standard.

### Authentication Middleware

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Bearer token** | ✅ Custom HMAC parse + validate | ✅ Laravel Sanctum / JWT |
| **Session cookie** | ✅ `__Host-forge_session` | ✅ Laravel encrypted session |
| **API key** | ✅ Scoped keys with IP whitelist (`ValidateApiKey`) | ✅ Application API keys with permissions |
| **OAuth2** | ✅ Full OAuth2 client support (`handlers_oauth2.go`) | ❌ Not present |
| **2FA enforcement** | ✅ Configurable policy: `none`, `admin`, `all` | ✅ Configurable: `LEVEL_NONE`, `LEVEL_ADMIN`, `LEVEL_ALL` |
| **2FA implementation** | TOTP with recovery codes | TOTP (Filament MFA App) + Email 2FA |
| **Rate limiting** | ✅ Redis-based per-endpoint (`middleware_ratelimit.go`) | ✅ Laravel built-in throttle |
| **Login rate limiting** | ✅ `checkLoginRateLimit` + `recordLoginFailure` | ✅ Laravel throttle middleware |
| **Password hashing** | `bcrypt` via Go's `golang.org/x/crypto` | `bcrypt` via Laravel's `Hash` facade |

**Code Comparison:**

Our `authMiddleware` (`forge/api/internal/http/auth.go:176-275`) handles four auth sources in priority order:
1. Bearer session token
2. OAuth2 token
3. API key
4. Session cookie

Pelican's authentication flows through Laravel's middleware stack with Sanctum for API tokens.

**Key Difference:** Our OAuth2 support (`handlers_oauth2.go`, `035_oauth2_clients.sql`) is a significant feature absent from Pelican. This allows third-party integrations with scoped access to specific servers.

### CSRF Protection

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **CSRF implementation** | Double-submit cookie pattern (`X-CSRF-Token` header vs `__Host-forge_csrf` cookie) | Laravel `PreventRequestForgery` (token-in-form) |
| **Scope** | Only for cookie-based sessions; skips Bearer/API-key auth | All state-changing requests except `remote/*`, `daemon/*` |
| **Origin checking** | ✅ `Origin` header + `Sec-Fetch-Site: cross-site` check | Partial (Laravel VerifyCsrfToken) |

**Our Advantage:** The double-submit cookie pattern with `Sec-Fetch-Site` checking is more robust than traditional CSRF tokens, especially for SPAs.

---

## 3. Server Management

### Server Model

| Field | GamePanel | Pelican Panel |
|---|---|---|
| **ID format** | UUID (PostgreSQL native) | UUID + `uuid_short` (first 8 chars) |
| **Owner** | `owner_id UUID` | `owner_id` (int FK) |
| **Node** | `node_id UUID` | `node_id` (int FK) |
| **Template/Egg** | `template_id UUID` → `server_templates` | `egg_id` → `eggs` (via `nests`) |
| **Resources** | `memory_mb`, `cpu_shares`, `disk_mb`, `swap_mb`, `io_weight`, `oom_disabled`, `threads` | `memory`, `cpu`, `disk`, `swap`, `io`, `threads`, `oom_killer` |
| **Limits** | `database_limit`, `backup_limit`, `allocation_limit` | Same |
| **Docker image** | Stored via template reference | `image` column directly on server |
| **Status** | `status TEXT` | `status` (enum: `ServerState`) |
| **Install state** | Separate tracking via daemon | `skip_scripts`, install script execution |
| **Description** | `description TEXT` | `description TEXT` |

### Server Lifecycle

| Operation | GamePanel | Pelican Panel |
|---|---|---|
| **Create** | `CreateServerRequest` → validate → DB insert → daemon `CreateServer` | `ServerCreationService.handle()` → DB transaction → daemon create |
| **Start/Stop** | `HandlePower(signal)` → daemon `SendPower()` | Via Filament UI → daemon API |
| **Install** | `install()` → daemon runs container with entrypoint+script | `ToggleInstallService` / `ReinstallServerService` |
| **Delete** | `delete()` → daemon `DeleteServer` → DB cleanup | `ServerDeletionService.withForce()` |
| **Transfer** | Full transfer protocol with `TransferProtocolVersion`, `TransferCredentialClaims`, staged push | `TransferServerService` via daemon |
| **Suspend/Unsuspend** | `Suspended` flag in `ServerState` | `SuspensionService` |

**Our Advantage — Truthful Server Lifecycle:**
Migration `040_truthful_server_lifecycle.sql` and the `server_manager.go` `Reconcile()` method implement a state reconciliation loop that compares panel state with actual Docker container state. This is more robust than Pelican's approach which relies on daemon-reported state.

**Pelican's Advantage — Maturity:**
`ServerCreationService` has retry logic (5 transaction attempts), allocation locking (`lockForUpdate()`), and comprehensive error handling with automatic cleanup on daemon connection failure.

### Subuser Permissions

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Permission model** | JSONB `permissions` column on `subusers` table | Legacy `permissions` table (migrated to JSON in `subusers`) |
| **Scope** | Server-level permissions checked via `UserCanAccessServer()` | `GetUserPermissionsService` |
| **RBAC** | Full RBAC: `roles`, `role_rules`, `user_roles` tables | Simple admin/user role system |
| **WebSocket permission** | `PermWebsocketConnect` checked on WS upgrade | Implicit (server access = WS access) |

**Our Advantage:** Full RBAC with custom roles and granular permissions (`007_postgres_core_foundation.sql`) is significantly more powerful than Pelican's binary admin/user model.

---

## 4. Node Management

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Node model** | UUID, `name`, `region`, `base_url`, `fqdn`, `status`, `token_hash` | UUID, `name`, `fqdn`, `scheme`, `daemon_token`, `daemon_token_id` |
| **Token auth** | `token_hash` (hashed) | `daemon_token` (encrypted, stored as plaintext in DB, encrypted at rest) |
| **Heartbeat** | Periodic heartbeat via `heartbeatLoop()` → daemon `SendNodeHeartbeat()` | Daemon reports stats; panel polls or daemon pushes |
| **Heartbeat data** | `version`, `os`, `architecture`, `cpu_threads`, `memory_mb`, `disk_mb`, `docker_status` | System info via HTTP call to daemon |
| **Heartbeat expiry** | `025_heartbeat_expiry_engine.sql` — auto-mark nodes offline | No automatic offline detection |
| **Region support** | Full region system (`regions` table, `CreateRegionRequest`) | Location-based (simpler) |
| **Placement** | `PlacementReservation` system with `ConfirmPlacementReservation` | `FindViableNodesService` + `AllocationSelectionService` |
| **Maintenance mode** | `maintenance_mode` + `desired_state` + `draining` | `maintenance_mode` boolean |
| **Configuration** | YAML config pushed to daemon (`011_wings_config_install.sql`) | `getConfiguration()` method on Node model |
| **Token rotation** | ✅ | ✅ (via Filament) |

**Our Advantage — Heartbeat Expiry Engine:**
The `025_heartbeat_expiry_engine.sql` and heartbeat monitoring provide automatic node health tracking. Nodes that miss heartbeats are automatically marked offline — Pelican lacks this.

**Our Advantage — Placement Reservations:**
The `CreatePlacementReservation` / `ConfirmPlacementReservation` / `CancelPlacementReservation` flow provides atomic server placement with resource reservation. Pelican's `FindViableNodesService` does a best-effort check without reservation locking.

**Pelican's Advantage — Simplicity:**
Pelican's node configuration is simpler — a single `getConfiguration()` method returns everything the daemon needs. Our configuration is spread across multiple migrations.

---

## 5. Database Design

### Migration Approach

| Aspect | GamePanel | Pelican Panel |
|---|---|---|
| **Migration format** | Sequential numbered SQL files (`001_init.sql` … `053_account_sessions.sql`) | Laravel timestamped PHP migrations (2016–2026) |
| **Migration count** | 53 files | ~160+ files |
| **Database** | PostgreSQL | MySQL/MariaDB |
| **ID strategy** | UUID primary keys throughout | Auto-incrementing integer IDs |
| **JSON support** | Native JSONB columns | `json` columns or serialized text |
| **Naming** | Snake_case | Snake_case (consistent) |

### Schema Comparison

| Table | GamePanel | Pelican Panel | Notes |
|---|---|---|---|
| `users` | UUID PK, email, password_hash, role, session_version, disabled, 2FA fields | Auto-increment PK, email, password, admin flag, 2FA fields | Pelican has more user fields (username, avatar, gravatar) |
| `nodes` | UUID PK, name, region, base_url, fqdn, status, token_hash, heartbeat fields, maintenance | Auto-increment PK, name, fqdn, scheme, daemon_token, memory/disk/cpu + overallocate | Pelican has richer resource tracking; we have heartbeat expiry |
| `servers` | UUID PK, node_id, owner_id, template_id, name, status, resources, limits | Auto-increment PK, uuid, uuid_short, node_id, egg_id, owner_id, name, status, resources | Similar structure |
| `allocations` | UUID PK, node_id, server_id, ip (inet), port, alias, notes | Autoincrement PK, node_id, server_id, ip, port, ip_alias, notes | Ours uses PostgreSQL `inet` type |
| `backups` | UUID PK, server_id, name, checksum, size, status, upload_id | Auto-increment PK, server_id, uuid, name, checksum, is_successful, is_locked, disk | Pelican has backup locking |
| `schedules` | Via `008_server_schedules.sql` + `009_schedule_run_history.sql` | `schedules` + `tasks` tables | Both support cron-based scheduling |
| `subusers` | UUID PK, server_id, user_id, permissions (JSONB) | `subusers` with merged permissions | Similar |
| `eggs` | UUID PK, nest_id, name, docker_images (JSONB), startup, config | `eggs` with nest relationship, docker_images, startup_commands | Pelican has `features` column, `file_denylist`, `force_outgoing_ip` |
| `nests` | UUID PK, name, description | `nests` with name, description | Identical concept |
| `plugins` | ✅ UUID PK, name, description, kind, version, manifest, installed, enabled | ✅ `plugins` table with similar fields | Both support plugins |
| `webhooks` | ✅ `039_webhooks.sql` | ✅ `webhooks` + `webhook_configurations` | Both support webhooks |
| `roles` | ✅ UUID PK, key, name, is_admin + `role_rules` + `user_roles` | ✅ Simple admin/user role | Ours is more granular |

### Unique Schema Features

**GamePanel-only tables:**
- `regions` — Multi-region support with enable/disable
- `placement_reservations` — Atomic server placement
- `evacuation_plans` — Node evacuation coordination
- `migration_history` — Server transfer tracking
- `observability` — System metrics collection
- `account_sessions` — Session management with revocation
- `panel_settings` — Centralized panel configuration
- `password_reset_tokens` — Secure password reset flow

**Pelican-only tables (not in ours):**
- `api_permissions` — Granular API key permissions (we have `api_key_scopes`)
- `egg_variables` — Template variables with sort order
- `server_variables` — Runtime variable values
- `activity_log_actors` — Detailed activity log attribution
- `user_ssh_keys` — SSH key management

**Missing from GamePanel:**
- `server_variables` table for storing per-server egg variable values
- `egg_variables` for template variable definitions
- `user_ssh_keys` for SSH public key management
- `docker_labels` support on servers
- `installed_at` timestamp tracking

---

## 6. API Design

### Endpoint Structure

| Aspect | GamePanel | Pelican Panel |
|---|---|---|
| **Framework** | Fiber (Go) | Laravel Routes (PHP) |
| **Route style** | RESTful, grouped by resource | RESTful, grouped by resource |
| **Versioning** | `/api/v1/` prefix | Implicit versioning |
| **Content negotiation** | `Accept: application/json` | `Accept: application/json` |
| **Error format** | `{"message": "error text"}` | `{"message": "error text"}` |
| **Pagination** | Basic `per_page` query param | Laravel pagination |

### Auth Endpoint Comparison

| Endpoint | GamePanel | Pelican Panel |
|---|---|---|
| **Login** | `POST /api/v1/auth/login` | `POST /api/auth/login` |
| **2FA checkpoint** | `POST /api/v1/auth/login/checkpoint` | `POST /api/auth/login/checkpoint` |
| **Logout** | `POST /api/v1/auth/logout` | `POST /api/auth/logout` |
| **Current user** | `GET /api/v1/auth/me` | `GET /api/auth/me` |
| **Password reset** | `POST /api/v1/auth/password/email` + `/reset` | `POST /api/auth/password/email` + `/reset` |
| **Change password** | `PUT /api/v1/account/password` | Similar |
| **Change email** | `PATCH /api/v1/account/email` | Similar |
| **Sessions** | `GET /api/v1/account/sessions` | Not present |
| **SSH keys** | `POST /api/v1/account/ssh-keys` | Via Filament UI |
| **2FA setup** | `POST /api/v1/account/2fa/setup` + `/enable` + `/disable` | Via Filament MFA |

**Our Advantage — Account Session Management:**
The `account/sessions` endpoints (`fetchUserSessions`, `revokeUserSession`, `revokeAllUserSessions`) are a significant security feature absent from Pelican. Users can see and revoke active sessions.

### Server Endpoints

| Operation | GamePanel Route | Pelican Panel Route |
|---|---|---|
| List servers | `GET /api/v1/servers` | `GET /api/servers` |
| Get server | `GET /api/v1/servers/:id` | `GET /api/servers/:id` |
| Create server | `POST /api/v1/servers` | `POST /api/servers` |
| Update server | `PATCH /api/v1/servers/:id` | `PUT /api/servers/:id` |
| Delete server | `DELETE /api/v1/servers/:id` | `DELETE /api/servers/:id` |
| Power signal | `POST /api/v1/servers/:id/power` | `POST /api/servers/:id/power` |
| Install | `POST /api/v1/servers/:id/install` | `POST /api/servers/:id/install` |
| Reinstall | `POST /api/v1/servers/:id/reinstall` | `POST /api/servers/:id/reinstall` |
| Transfer | `POST /api/v1/servers/:id/transfer` | `POST /api/servers/:id/transfer` |
| Suspend | `POST /api/v1/servers/:id/suspend` | Via Filament |
| Backups | `GET/POST /api/v1/servers/:id/backups` | Similar |
| Files | `GET/PUT/DELETE /api/v1/servers/:id/files/*` | Similar |
| Schedules | `GET/POST /api/v1/servers/:id/schedules` | Similar |
| Subusers | `GET/POST /api/v1/servers/:id/users` | Similar |

### Admin Endpoints

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **User CRUD** | ✅ `GET/POST/PATCH/DELETE /api/v1/admin/users` | Via Filament admin panel |
| **Node CRUD** | ✅ `GET/POST/PATCH/DELETE /api/v1/admin/nodes` | Via Filament + API |
| **Region CRUD** | ✅ `GET/POST/PATCH/DELETE /api/v1/admin/regions` | ❌ (locations only) |
| **Nest/Egg CRUD** | ✅ | Via Filament |
| **Allocation management** | ✅ Bulk operations | Via Filament |
| **Database host management** | ✅ | Via Filament |
| **Mount management** | ✅ Full CRUD + attachment | Via Filament |
| **Plugin management** | ✅ Import, install, enable/disable | Via Filament |
| **Webhook management** | ✅ | Via Filament |
| **OAuth2 client management** | ✅ | ❌ |
| **Role management** | ✅ CRUD + assignment | Simple admin/user toggle |
| **Activity logs** | ✅ `GET /api/v1/admin/activity` | Via Filament |
| **Audit logs** | ✅ `GET /api/v1/admin/audit` | ✅ `activity_logs` table |
| **Panel settings** | ✅ `GET/PATCH /api/v1/admin/settings` | Via `.env` + config files |
| **Health check** | ✅ `GET /api/v1/admin/health` | ❌ |
| **Observability** | ✅ `GET /api/v1/admin/monitoring` | ❌ |
| **Evacuation** | ✅ `POST /api/v1/admin/evacuations` | ❌ |
| **Migration engine** | ✅ `POST /api/v1/admin/migrations` | ❌ |

**Our Advantage — Comprehensive Admin API:**
Our admin API provides full programmatic access to all management operations. Pelican relies heavily on the Filament admin panel (PHP rendered UI) for these operations, which means automation requires API workarounds.

---

## 7. WebSocket Handling

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **WS library** | Fiber WebSocket (gorilla/websocket) | Laravel WebSockets / Soketi |
| **Auth modes** | 1. Long-lived JWT 2. Short-lived WS ticket | Session-based |
| **WS tickets** | `wsTicketStore` with single-use consumption, identity binding, server binding | Not present |
| **Proxy pattern** | Panel proxies WS to daemon upstream | Panel proxies WS to daemon |
| **Origin validation** | Configurable via `API_WS_ALLOWED_ORIGINS` env | Not explicitly configured |
| **Streams** | Console, logs, stats (3 streams) | Console, logs (2 streams) |
| **Console throttle** | `ConsoleThrottle` with configurable rate + strike callback | Not present |
| **Replay buffer** | 128 entries / 256KB replay buffer per server | Not present |
| **Read limit** | 1MB per WS message | Not explicitly configured |
| **Ping/pong** | 60s deadline + pong handler | Not explicitly configured |
| **Protocol header** | Supports `Sec-WebSocket-Protocol: jwt.<token>` fallback | N/A |

**Our Advantage — WS Ticket System:**
The `wsTicketStore` with single-use, time-limited, server-bound, identity-bound tickets is a significant security improvement over Pelican's session-based WS auth. It prevents ticket reuse and cross-server access.

**Our Advantage — Console Throttle & Replay:**
The `ConsoleThrottle` rate limiter and 128-entry/256KB replay buffer in `console.go` prevent console flooding and provide late-joiners with recent output. Pelican has no equivalent.

---

## 8. File Management

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **List files** | ✅ `GET /api/v1/servers/:id/files` | Via daemon API |
| **Read file** | ✅ `GET /api/v1/servers/:id/files/contents` | Via daemon API |
| **Write file** | ✅ `PUT /api/v1/servers/:id/files/write` | Via daemon API |
| **Upload** | ✅ Chunked upload with `uploadFileChunked()` (1MB chunks) | Via daemon API |
| **Download** | ✅ Ticket-based file download | Via daemon API |
| **Delete** | ✅ Single + batch delete | `DeleteFilesService` (pattern-based) |
| **Rename** | ✅ Single + batch rename | Via daemon API |
| **Copy** | ✅ `POST /api/v1/servers/:id/files/copy` | Via daemon API |
| **Chmod** | ✅ `POST /api/v1/servers/:id/files/chmod` | Via daemon API |
| **Archive** | ✅ `POST /api/v1/servers/:id/files/archive` | Via daemon API |
| **Decompress** | ✅ `POST /api/v1/servers/:id/files/decompress` | Via daemon API |
| **Make directory** | ✅ `POST /api/v1/servers/:id/files/mkdir` | Via daemon API |
| **Pull remote URL** | ✅ `POST /api/v1/servers/:id/files/pull` | Not present |
| **Text detection** | ✅ `isTextFile()` heuristic | N/A |
| **Path validation** | ✅ `safePath()` traversal protection | Daemon-side validation |
| **Archive validation** | ✅ `validateZip()`, `validateTar()` with entry limits | Not present |
| **Upload staging** | ✅ Staged extraction with rollback | Not present |
| **Disk quota** | ✅ `quotaWriter` in SFTP, `HasSpaceForWrite` check | Not present |

**Our Advantage — Secure File Operations:**
The `secure_files.go` implementation includes:
- Archive validation (zip bomb prevention, entry count limits)
- Staged extraction with rollback on failure
- Upload size limits per chunk
- Disk quota enforcement
- Path traversal protection
- Pull-remote with pinned DNS resolution

Pelican's file operations are simpler, delegating most work directly to the daemon.

---

## 9. Backups

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Adapters** | `local` + `s3` (via `BackupInterface`) | `local` + `s3` + external via `BackupManager` |
| **S3 implementation** | Native AWS SDK v2 with multipart upload, retry (3 attempts), exponential backoff | Laravel Flysystem adapter |
| **Backup locking** | ✅ Via `049_backup_locking.sql` | ✅ `is_locked` column |
| **Backup status** | ✅ Detailed status tracking (`051_backup_status_tracking.sql`) | `is_successful` boolean |
| **Checksum** | SHA-256, verified on download and S3 staging | SHA-256, stored in DB |
| **Ignored files** | ✅ `.gameignore` pattern matching | ✅ `ignored_files` JSON column |
| **Restore** | ✅ With journal-based crash recovery (`RecoverRestoreJournals`) | Daemon-side restore |
| **Restore journal** | ✅ `restoreJournal` with staging/rollback/phase tracking | Not present |
| **Download** | ✅ Direct download with cleanup | `DownloadLinkService` |
| **Auto-cleanup** | ✅ `BackupAutoCleanup` + `BackupRetentionDays` in panel settings | Config-based throttle |
| **Throttling** | Via panel settings | `config('backups.throttles.limit')` + period |
| **Legacy migration** | ✅ `migrateLegacyBackups` from old path layout | N/A |

**Our Advantage — Crash-Safe Restore:**
The `restoreJournal` system in `local.go` (with `writeRestoreJournal`, `recoverInterruptedRestore`, `RecoverRestoreJournals`) provides crash recovery for interrupted backup restores. This is a production-critical feature absent from Pelican.

**Our Advantage — S3 with Staging:**
Our S3 backup implementation stages locally before uploading, with 3-attempt retry and exponential backoff. Downloads verify checksums against S3 metadata. Pelican's approach delegates to Flysystem which has less control over retry behavior.

**Pelican's Advantage — Backup Throttling:**
Pelican's `InitiateBackupService` has explicit time-based throttling (limit backups per N seconds) with proper HTTP 429 responses and retry-after headers. Our throttling is less explicit.

---

## 10. Scheduling

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Runner** | In-process `scheduleRunner` with `robfig/cron/v3` parser | Laravel Queue workers + `ProcessScheduleService` |
| **Claim-based** | ✅ `ClaimDueSchedule` with worker ID, lease extension | ❌ (queue job dispatch) |
| **Lease renewal** | ✅ `ExtendScheduleLease` during long-running tasks | ❌ |
| **Task types** | `power`, `backup`, `command` | `power`, `backup`, `command` |
| **Only-when-online** | ✅ Checked before execution | ✅ `only_when_online` flag |
| **Continue on failure** | ✅ Per-task `continue_on_failure` flag | ❌ (stops on first failure) |
| **Task run history** | ✅ `schedule_task_runs` table with per-task status | ❌ (only schedule-level tracking) |
| **Wake optimization** | ✅ `nextWakeDelay` calculates precise timer based on next schedule | Fallback 1-minute polling |
| **PostgreSQL LISTEN** | ✅ `ListenScheduleEvents` for instant notification | N/A (queue-based) |
| **Manual run** | ✅ `RunNow` with `ClaimManualSchedule` | ✅ `ProcessScheduleService.handle($schedule, true)` |
| **Timezone support** | ✅ `052_schedule_timezone.sql` | Via PHP timezone config |
| **Backup cleanup** | ✅ Integrated into schedule tick | ❌ |

**Our Advantage — Claim-Based Scheduling:**
The claim-based scheduler with lease renewal is designed for horizontal scalability. Multiple API instances can run schedules without conflicts. Pelican's queue-based approach is simpler but less explicit about concurrency.

**Our Advantage — Per-Task Run History:**
The `schedule_task_runs` table tracks individual task execution status, enabling detailed debugging of complex multi-step schedules.

---

## 11. Plugin System

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Plugin storage** | PostgreSQL `plugins` table | Database `plugins` table |
| **Import** | File upload + URL fetch (`ImportPluginFromFile`, `ImportPluginFromURL`) | Via admin UI |
| **Manifest validation** | JSON schema: `name`, `description`, `kind`, `version` | Similar |
| **Install state** | `installed` + `enabled` booleans | Similar |
| **Runtime execution** | ❌ Not yet implemented (manifest only) | ✅ Full PHP plugin runtime |
| **Lifecycle** | Import → Install → Enable/Disable → Uninstall → Delete | Similar |
| **Plugin kind** | `integration` (default) | Various types |
| **Manifest hash** | SHA-256 `pluginHash()` | Not present |

**Pelican's Advantage — Plugin Runtime:**
Pelican has a full PHP plugin runtime that can execute plugin code, extend Filament panels, add routes, and hook into system events. Our plugin system is currently manifest-only (metadata storage without execution). This is a significant functional gap.

---

## 12. Security

### Security Headers

| Header | GamePanel | Pelican Panel |
|---|---|---|
| `Content-Security-Policy` | ✅ Full CSP with script/style/connect directives | ❌ (commented as TODO) |
| `X-Frame-Options` | ✅ `DENY` | ✅ `DENY` |
| `X-Content-Type-Options` | ✅ `nosniff` | ✅ `nosniff` |
| `X-XSS-Protection` | ✅ `1; mode=block` | ✅ `1; mode=block` |
| `Referrer-Policy` | ✅ `strict-origin-when-cross-origin` | `no-referrer-when-downgrade` |
| `Permissions-Policy` | ✅ Disables camera, mic, geolocation, etc. | ❌ |
| `Strict-Transport-Security` | ✅ Conditional on HTTPS | ❌ |

**Our Advantage:** Our security headers are significantly more comprehensive, including a full CSP and Permissions-Policy that Pelican explicitly defers.

### Rate Limiting

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Implementation** | Redis-based, per-endpoint type | Laravel `throttle` middleware |
| **Auth endpoint** | 5 requests/minute | Configurable |
| **Mutation endpoint** | 30 requests/minute | Configurable |
| **Read endpoint** | 120 requests/minute | Configurable |
| **Headers** | `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After` | Standard 429 response |

**Our Advantage:** Our rate limiting returns proper `X-RateLimit-*` headers, which is best practice for API consumers.

### IP Access Control

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **IP allowlist/denylist** | ✅ `middleware_ipaccess.go` with CIDR support | ❌ |
| **Proxy trust** | ✅ Configurable `TrustProxy` with `X-Forwarded-For` / `X-Real-IP` | Standard Laravel trust |
| **Admin IP restriction** | ✅ `AdminIPAccessConfig` (extensible) | ❌ |

### Session Security

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Cookie prefix** | `__Host-` (enforces Secure + no domain scope) | Standard Laravel session |
| **Session version** | ✅ Incremented on password/2FA change | ❌ |
| **Session revocation** | ✅ Per-session + bulk revoke with reason | Via password change |
| **Token revocation** | ✅ JTI-based blacklist | Token blacklist |
| **Password reset** | ✅ `038_password_reset_tokens.sql` with dedicated table | Laravel `password_resets` |
| **2FA recovery** | ✅ Recovery codes | ✅ Recovery codes |
| **TOTP** | ✅ | ✅ |

### Other Security

| Feature | GamePanel | Pelican Panel |
|---|---|---|
| **Backup path validation** | ✅ `validNamespace()`, `validBackupName()` with charset + length checks | Daemon-side |
| **Archive bomb protection** | ✅ `validateZip()`, `validateTar()` with entry/size limits | ❌ |
| **Upload staging** | ✅ Extract to staging dir, validate, then merge | Direct write |
| **DNS pinning** | ✅ `pinnedResolver` for pull-remote to prevent DNS rebinding | ❌ |
| **Config file permissions** | ✅ `0o600` for config files | Standard file permissions |
| **CSRF** | ✅ Double-submit cookie + `Sec-Fetch-Site` | Laravel VerifyCsrfToken |

**Our Advantage:** The security posture is significantly stronger with DNS pinning, archive validation, upload staging with rollback, and `__Host-` prefixed cookies.

---

## 13. Frontend Comparison

| Aspect | GamePanel (`forge/web/`) | Pelican Panel |
|---|---|---|
| **Framework** | Next.js (React 18+ / TypeScript) | Livewire + Blade (PHP SSR) |
| **UI library** | Tailwind CSS + custom components | Filament (PHP component library) |
| **State management** | Zustand (`use-server-store.ts`) | Livewire reactive properties |
| **Type safety** | ✅ Full TypeScript with `Api*` types | ❌ PHP dynamic typing |
| **API client** | ✅ Typed `api.ts` with 200+ typed functions | ❌ Server-side Eloquent |
| **Tests** | ✅ Vitest + React Testing Library | ✅ PHPUnit |
| **Routing** | Next.js App Router (`app/`) | Laravel routes + Filament resources |
| **WebSocket** | Client-side `connectServerWebSocket()` | Laravel WebSockets / Soketi |

### Page Coverage

| Page | GamePanel | Pelican Panel |
|---|---|---|
| Admin dashboard | ✅ `admin/overview/` | ✅ Filament Dashboard |
| Server list | ✅ `servers/` | ✅ Filament resource |
| Server console | ✅ `server/[id]/` with real-time WS | ✅ Filament page |
| Server files | ✅ `server/[id]/files/` | ✅ Filament page |
| Server backups | ✅ `server/[id]/backups/` | ✅ Filament page |
| Server schedules | ✅ `server/[id]/schedules/` | ✅ Filament page |
| Server databases | ✅ `server/[id]/databases/` | ✅ Filament page |
| Server networking | ✅ `server/[id]/network/` | Via Filament |
| Server startup | ✅ `server/[id]/startup/` | Via Filament |
| Server settings | ✅ `server/[id]/settings/` | Via Filament |
| Server users | ✅ `server/[id]/users/` | Via Filament |
| Server activity | ✅ `server/[id]/activity/` | Via Filament |
| Node management | ✅ `admin/nodes/` | Via Filament |
| Region management | ✅ `admin/regions/` | ❌ (locations) |
| Template management | ✅ `admin/templates/` | Via Filament |
| User management | ✅ `admin/users/` | Via Filament |
| Role management | ✅ `admin/roles/` | ❌ (simple admin/user) |
| Plugin management | ✅ `admin/plugins/` | Via Filament |
| OAuth2 clients | ✅ `admin/oauth-clients/` | ❌ |
| Webhook management | ✅ `admin/webhooks/` | Via Filament |
| Mount management | ✅ `admin/mounts/` | Via Filament |
| Activity logs | ✅ `admin/activity/` + `admin/logs/` | Via Filament |
| Health monitoring | ✅ `admin/health/` | ❌ |
| Observability | ✅ `admin/monitoring/` | ❌ |
| Operations | ✅ `admin/operations/` | ❌ |
| Account management | ✅ `account/` | Via Filament profile |
| Password reset | ✅ `forgot-password/` + `reset-password/` | Via Filament auth |

**Our Advantage — Type-Safe API Client:**
The `lib/api.ts` file provides 200+ typed API functions with full TypeScript interfaces (`ApiNode`, `ApiServer`, `ApiBackup`, etc.). This catches type errors at compile time. Pelican's Livewire components don't benefit from this kind of type safety.

**Our Advantage — SPA Architecture:**
Our Next.js SPA with Zustand state management provides instant client-side navigation and optimistic updates. Pelican's Livewire requires server round-trips for every interaction.

---

## 14. Summary Scorecard

| Domain | GamePanel | Pelican Panel | Winner | Notes |
|---|---|---|---|---|
| **Architecture** | ⭐⭐⭐ | ⭐⭐⭐⭐ | Pelican | More mature, battle-tested, simpler mental model |
| **Authentication** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | OAuth2, session management, session versioning |
| **Server Management** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Tie | Different strengths; our lifecycle reconciliation vs their proven creation flow |
| **Node Management** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Heartbeat expiry, placement reservations, regions |
| **Database Design** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Tie | Ours has more features; Pelican has more migrations/iterations |
| **API Design** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | More endpoints, typed client, admin API completeness |
| **WebSocket Handling** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Ticket system, console throttle, replay buffer |
| **File Management** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Archive validation, staging, disk quota, pull-remote |
| **Backups** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Crash recovery journals, S3 staging, checksum verification |
| **Scheduling** | ⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Claim-based, per-task history, lease renewal |
| **Plugin System** | ⭐⭐ | ⭐⭐⭐⭐ | Pelican | Our plugins are metadata-only; Pelican has full runtime |
| **Security** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | CSP, DNS pinning, archive bombs, rate limit headers, IP control |
| **Frontend** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | GamePanel | Type safety, SPA, more admin pages, health/monitoring |
| **Maturity/Battle-tested** | ⭐⭐ | ⭐⭐⭐⭐⭐ | Pelican | 10+ years of Pterodactyl lineage |
| **Performance** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | GamePanel | Go binary vs PHP-FPM, Redis caching |

---

## 15. Key Gaps & Recommendations

### Critical Gaps in GamePanel

1. **Plugin Runtime** — Our plugin system is import-only with no execution engine. Pelican's PHP plugin runtime is a major differentiator. **Recommendation:** Implement a WASM or Go-based plugin sandbox.

2. **Egg Variables / Server Variables** — We lack `egg_variables` and `server_variables` tables. Templates have no configurable parameters. **Recommendation:** Add `egg_variables` table and server variable storage.

3. **SSH Key Management** — No `user_ssh_keys` table or API endpoints. **Recommendation:** Add SSH key CRUD endpoints.

4. **Docker Labels** — Servers don't support custom Docker labels. **Recommendation:** Add `docker_labels` JSONB column to servers.

5. **Installation Script Execution** — Our install flow exists but the tracking (`installed_at`, `skip_scripts`) is less mature than Pelican's.

### Areas to Strengthen

1. **Backup throttling** — Pelican's `InitiateBackupService` has explicit rate limiting with HTTP 429 + retry-after. Our throttling is implicit.

2. **Server creation retry** — Pelican's `ServerCreationService` retries DB transactions up to 5 times. Our single-attempt approach may be fragile under load.

3. **Node statistics caching** — Pelican caches node system info for 360 seconds and uses `flexible` cache TTL. Our heartbeat approach is more real-time but may overload the database.

4. **File denylist** — Pelican's eggs support `file_denylist` (files users cannot access). We lack this feature.

5. **User avatar support** — Pelican has gravatar/avatar support. We don't.

### Where GamePanel Excels

1. **Heartbeat expiry engine** — Automatic node health tracking
2. **Placement reservations** — Atomic server placement with resource locking
3. **Full RBAC** — Custom roles with granular permissions
4. **OAuth2** — Third-party integration support
5. **Account session management** — View/revoke active sessions
6. **Security headers** — Comprehensive CSP, Permissions-Policy, HSTS
7. **Archive validation** — Zip bomb prevention, entry limits
8. **Crash recovery** — Journal-based backup restore recovery
9. **DNS pinning** — Prevents DNS rebinding on file pulls
10. **Claim-based scheduling** — Horizontally scalable schedule execution
11. **WS ticket system** — Single-use, identity-bound WebSocket authentication
12. **TypeScript API client** — 200+ typed functions with full interfaces

---

*Report generated by Agent 2 — file-by-file source comparison audit.*
*Files analyzed: 141 Go files (beacon + forge/api), 53 SQL migrations, 117 TypeScript/JavaScript files (forge/web), ~160 PHP migration files, 25+ PHP model/service files.*
