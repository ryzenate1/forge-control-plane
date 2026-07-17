# File-by-File Comparison

> See agent task reports for the full 800-line detailed comparison. This document consolidates key findings per subsystem.

## Subsystem 1: Authentication & Authorization

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `auth.go` | Pterodactyl | `config/auth.php`, `Middleware/ApiAuthenticate.php` | JWT + session auth | HMAC-SHA256 stateless tokens, session cookie + CSRF cookie, 2FA middleware (none/admin/all), API key + IP allowlist | Laravel guard/provider, Sanctum token auth, middleware-based 2FA levels | Custom HMAC token (not RFC 9068 JWT), no OAuth2 refresh, no reCAPTCHA | Admin password-reset, session table not used, per-request rate limit tokens |
| `handlers_auth.go` | Pterodactyl | `routes/api-client.php` | Login, 2FA, API/SSH keys | 2FA checkpoint flow, full API key CRUD, SSH key CRUD, OAuth2 client management, session revoke-all | Dedicated AccountController with Fractal transformers, separate updateEmail/updatePassword | No 2FA "remember device", no admin password reset, no email-verified flag | `/auth/oauth` token introspection, SSO providers (Discord, GitHub) |
| `middleware_security.go` | Pterodactyl | `SetSecurityHeaders.php` | Security headers | Full CSP, HSTS, Permissions-Policy, X-Frame-Options DENY | Basic 4-header approach | CSP is restrictive (no dynamic generation) | SRI for assets, configurable HSTS max-age |
| `server.go:1117` | Pterodactyl | `routes/api-client.php:24` | Middleware chain | Auth + 2FA + CSRF + rate limit + IP access stacked per group | Route-level middleware groups with named middleware, Acl middleware | Auth all-or-nothing per v1 group, CSRF only on protected routes | Per-route middleware customization, maintenance mode |

## Subsystem 2: Server Management

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `handlers_servers.go:71-200` | Pterodactyl | `Controllers/Api/Client/Servers/` (16 controllers) | Server CRUD | Paginated search, user resource limits, force-delete | Dedicated PowerController/SettingsController/StartupController, Fractal transformers | Power routes duplicated, no batch ops | Activity count endpoint, reinstall trigger |
| `handlers_servers.go:661-900` | Pterodactyl | `ServerCreationService.php` | Server creation | Placement scheduler, region/node constraints, egg variable injection | Proper service layer with FormRequest validation | Creation logic in handler, inconsistent error handling | ServerConfigurationStructureService |
| `store/store_servers.go` | Pterodactyl | `Services/Servers/` (12 services) | DB operations | Paginated queries, subuser join, full scan JOINs | Dedicated service classes (BuildModificationService, SuspensionService, etc.) | SQL inline, no cursor-based pagination | Activity query builder, soft-delete |
| `handlers_servers.go:1084-1099` | Wings | `server/install.go` | Install/Reinstall | toggle-install endpoint, egg install scripts | Installer runs container, tracks process, cancels on delete | Basic install status tracking | Install container orchestration via daemon, egg-change reinstall |

## Subsystem 3: Node Management & Daemon Communication

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `handlers_admin.go:28-240` | Pterodactyl | `NodeController` | Node CRUD + config | Full CRUD, rotate-token, health, lifecycle, evacuation preview, capacity snapshot | NodeConfigurationController, config download endpoint, node token encryption | No Wings-format YAML config endpoint | `/nodes/deployable` endpoint, node JWT signing |
| `daemon/client.go` | Wings | `remote/http.go` | Daemon HTTP client | 15-min timeout, CreateRequest with full Docker config (ports, mounts, env, registry auth) | Backoff retries, Client interface, SetArchiveStatus, SendActivityLogs | No retry/backoff, no Client interface for testability | SetArchiveStatus, ValidateSftpCredentials, SendActivityLogs |
| `handlers_remote.go` | Pterodactyl | `routes/api-remote.php` | Remote (daemon→panel) API | Bearer auth, full server config, SFTP auth, backup status, activity logging | Token ID + encrypted token parts with hash_equals, backup remote upload URL | Simpler auth (just bearer), no backup upload URL | Backup remote upload URL endpoint, compressed download |

## Subsystem 4: File Management

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `handlers_servers.go:1829-2310` | Pterodactyl | `FileController.php` | File CRUD | Full ops: list, read, write (multipart + raw), upload chunked, delete, mkdir, rename, copy, chmod, archive, decompress, batch ops, URL pull | JWT-signed file access tokens, FormRequest validation, Wings disk quota integration, .pteroignore | 100% proxied to daemon, no server-side file validation | .pteroignore support, disk space check, signed download URLs |
| `handlers_servers.go:1829-2310` | Wings | `filesystem/filesystem.go` | File operations | — | Disk quota via `ufs.Quota`, ReadDir/ReadDirStat, Touch, Writefile, openat2 | No local FS abstraction, all ops proxied | Local quota enforcement |

## Subsystem 5: Backups

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `handlers_servers.go:1462-1711` | Pterodactyl | `BackupController.php`, Wings `server/backup.go` | Backup CRUD | Full ops, user + server backup caps, retention-based cleanup | Async backup initiation (InitiateBackupService), signed download URLs, .pteroignore | Synchronous (blocking for 15 min), no .pteroignore forwarding | .pteroignore support, async backup, signed download URLs, S3 backup config |
| `store_backups.go` | Wings | `server/backup/` | Backup storage | Locking, retention cleanup, retry count, status callbacks | S3 + local drivers, ArchiveDetails (checksum/size), Generate/Remove interface | No S3 driver, no integrity verification | S3 backup storage, BackupInterface abstraction |

## Subsystem 6: Schedules

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `services/scheduler/service.go` | PufferPanel | `servers/scheduler.go` | Placement scheduling | Multi-factor scoring, region-aware placement, NOTIFY for real-time updates | Capacity-based placement | Only placement, not cron task execution | Metrics (rejections/capacity exceeded), event publishing |
| `store_schedules.go` | Pterodactyl | `ProcessScheduleService.php` | Schedule execution | Full CRUD, run history, LISTEN/NOTIFY, timezone, task ordering | Proper service layer, dedicated ScheduleController + TaskController | No cron validation, missing task actions | Task action types, formatted cron output, task queue |
| `handlers_servers.go:910-1082` | PufferPanel | `servers/scheduler.go` | Schedule API | Full REST CRUD, run-now trigger | gocron v2 with JSON file persistence, per-server scheduler | In-process execution (no durable queue) | Durable execution queue, daemon-side runner |

## Subsystem 7: Databases

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `dbprovisioner/service.go` | Pterodactyl | `DatabaseController.php` | DB provisioning | Dual-engine (MySQL + PostgreSQL), TLS config, password rotation with rollback, full lifecycle | DeployServerDatabaseService, DatabasePasswordService, connection pooling | No connection pooling for admin connections, no host health checks | Host health check, auto-selection by node, max DB enforcement |
| `handlers_servers.go:1134-1411` | Pterodactyl | `DatabaseController.php:36-50` | DB CRUD API | Full CRUD with force orphan remediation, password rotation, Pterodactyl-shaped aliases | Fractal transforms, FormRequest validation, random host selection | No host auto-selection | Random host assignment per DeployServerDatabaseService |

## Subsystem 8: Migrations

| Our file | Reference | Ref file | Responsibility | Our Strengths | Ref Strengths | Our Weaknesses | Missing |
|---|---|---|---|---|---|---|---|
| `migrations/001_init.sql` | Pterodactyl | `database/migrations/2016_01_01_*.php` | Schema foundation | Clean schema with UUID PKs, nodes/servers/allocations/eggs | Laravel Schema::create(), 195 reversible migrations, timestamps | SQL-only, no Go migration runner, no down-migrations | Reversible down-migrateons, migration version tracking, seed data |
| `migrations/023_migration_engine.sql` | Pterodactyl | Database migration | Transfer system | Custom migration_status ENUM, migration_history audit trail | Tracked via server model fields | — | — |
| `migrations/025_heartbeat_expiry_engine.sql` | — | — | Heartbeat tracking | 5-state ENUM, recovery count tracking | None exists in references | State not consumed by service layer | State machine transitions, automatic recovery |
