# Gamepanel vs Pelican Panel — Comprehensive Audit Comparison

**Generated:** 2026-07-16
**Gamepanel:** Go (Fiber) backend + Next.js frontend + Go daemon
**Pelican Panel:** PHP (Laravel) backend + Filament (Blade/Livewire) frontend
**Reference:** `/Users/riyaz/project/gamepanel/reference/pelicanpanel/`

---

## Executive Summary

Gamepanel is a **from-scratch reimplementation** of the Pelican Panel concept with significant architectural changes and feature additions. The most important difference is the language/stack shift (PHP → Go/TypeScript), which drives nearly every other architectural decision. Gamepanel is **not a line-for-line port** — it refactors and extends the Pelican feature set while introducing new infrastructure concepts (regions, deployment pipelines, autoscaling, load balancing, failover clusters, traffic management, predictive scheduling).

**Key stats:**

| Metric | Pelican Panel | Gamepanel |
|---|---|---|
| Backend language | PHP (Laravel 11) | Go (Fiber v2) |
| Frontend | Filament (Blade/Livewire) | Next.js 14 (App Router) |
| Daemon | Pelican Wings (separate) | Beacon (Go, integrated) |
| Database | SQLite/MySQL/PostgreSQL | PostgreSQL (+ SQLite/MySQL compat) |
| Tests | ~79 Pest/Unit tests | ~150 Go/TS tests |
| Admin pages | 124 Filament resource files | 57 React component files |
| Migrations | ~100+ Laravel migrations | ~58 SQL migrations |
| Languages | 20+ locale directories | 8 JSON locale files |
| Plugin system | Sushi-based, Composer packages | Go plugin service + DB manifests |
| Container orchestration | Standalone daemon per node | Cluster-aware with regions |

---

## Directory/File Structure Comparison

### Backend Application (Panel Core)

| Pelican Panel | Gamepanel | Notes |
|---|---|---|
| `app/Http/Controllers/` | `forge/api/internal/http/` | Route handlers |
| `app/Models/` (Eloquent) | `forge/api/internal/store/` + `domain/` | Data models / storage layer |
| `app/Services/` | `forge/api/internal/services/` | Business logic services |
| `app/Enums/` | `forge/api/internal/domain/` + Go constants | Enums and constants |
| `app/Events/` + `app/Listeners/` | `forge/api/internal/events/` + `eventstore/` | Event system |
| `app/Policies/` | `forge/api/internal/policies/` | Authorization policies |
| `app/Jobs/` | `forge/api/internal/services/queue/` | Async job processing |
| `app/Notifications/` | `forge/api/internal/services/mail/` | Email notifications |
| `app/Providers/` | `forge/api/cmd/api/main.go` | Service registration/bootstrap |
| `app/Exceptions/` | Inline error handlers | Error handling |
| `app/Helpers/` | `forge/api/internal/http/helpers_*.go` | Utility functions |
| `app/Rules/` | `forge/api/internal/http/validation.go` | Validation rules |
| `app/Repositories/` | `forge/api/internal/daemon/` | Daemon communication layer |
| `app/Contracts/` | Go interfaces in various packages | Contracts/interfaces |
| `app/Extensions/` | N/A (different approach) | Extension framework |
| `app/Facades/` | N/A (Go doesn't use facades) | Static proxies |
| `app/Transformers/` | Handled in response construction | API response formatting |

### Frontend

| Pelican Panel | Gamepanel | Notes |
|---|---|---|
| `resources/views/` (Blade) | `forge/web/app/` (Next.js pages) | Template/pages |
| `app/Filament/Admin/Resources/` | `forge/web/app/admin/` | Admin pages |
| `app/Filament/Server/Resources/` | `forge/web/app/server/[id]/` | Server management pages |
| `app/Filament/Pages/Auth/` | `forge/web/app/login`, `forgot-password/` | Auth pages |
| `app/Livewire/` | `forge/web/components/` | Interactive components |
| `public/css/` + `public/js/` | `forge/web/components/ui/` | UI components |

### Daemon (Wings Equivalent)

| Pelican Panel | Gamepanel | Notes |
|---|---|---|
| Standalone `wings` binary | `beacon/` directory | Gamepanel owns the daemon in-repo |
| — | `beacon/internal/server/` | Container runtime management |
| — | `beacon/internal/sftpserver/` | SFTP server |
| — | `beacon/internal/backup/` | Backup orchestration |
| — | `beacon/internal/transfer/` | Server transfer |
| — | `beacon/internal/cron/` | Schedule execution |

### Infrastructure

| Pelican Panel | Gamepanel | Notes |
|---|---|---|
| `docker/` (Caddy, supervisord, crontab) | `infra/` (Compose, Grafana, Prometheus) | Deployment config |
| `Dockerfile` | `forge/api/Dockerfile` + Dockerfile (root) | Container build |
| `.env.example` | `infra/.env.example` (via compose) | Environment config |

### Configuration

| Pelican Panel | Gamepanel | Notes |
|---|---|---|
| `config/app.php` | `forge/api/config/app.go` | App config |
| `config/database.php` | `forge/api/config/database.go` | Database config |
| `config/mail.php` | `forge/api/config/mail.go` | Mail config |
| `config/auth.php` | `forge/api/config/auth.go` | Auth config |
| `config/services.php` | `forge/api/config/services.go` | OAuth/external services |
| `config/panel.php` | N/A (inline defaults) | Panel-specific settings |
| `config/backups.php` | N/A (inline defaults) | Backup configuration |
| `config/activity.php` | N/A (inline defaults) | Activity log settings |
| `config/http.php` | `handlers_rate_limit_config.go` | Rate limiting |
| `config/sanctum.php` | N/A (JWT-based auth) | API token auth |
| `config/cors.php` | `server.go` (inline CORS) | CORS configuration |
| `config/health.php` | `handlers_health.go` | Health checks |
| `config/permission.php` | `forge/api/internal/policies/` | Permission system |
| `config/cache.php` | N/A (Redis direct usage) | Caching config |
| `config/session.php` | `session_cookie.go` | Session handling |

---

## API Routes Comparison

### Application API (Admin) — `/api/application/*` vs `/api/v1/*`

| Endpoint | Pelican Panel | Gamepanel | Status |
|---|---|---|---|
| **Users** | | | |
| List users | `GET /users` | `GET /users` | ✅ Match |
| Get user | `GET /users/{id}` | `GET /users/:id` | ✅ Match |
| Get by external ID | `GET /users/external/{id}` | Not found | ❌ Missing |
| Create user | `POST /users` | `POST /users` | ✅ Match |
| Update user | `PATCH /users/{id}` | `PATCH /users/:id` | ✅ Match |
| Delete user | `DELETE /users/{id}` | `DELETE /users/:id` | ✅ Match |
| Assign roles | `PATCH /users/{id}/roles/assign` | Not found | ❌ Missing |
| Remove roles | `PATCH /users/{id}/roles/remove` | Not found | ❌ Missing |
| **Nodes** | | | |
| List nodes | `GET /nodes` | `GET /nodes` | ✅ Match |
| Get node | `GET /nodes/{id}` | `GET /nodes/:id` | ✅ Match |
| Get configuration | `GET /nodes/{id}/configuration` | Not found | ❌ Missing |
| Deployable nodes | `GET /nodes/deployable` | Not found | ❌ Missing |
| Create node | `POST /nodes` | `POST /nodes` | ✅ Match |
| Update node | `PATCH /nodes/{id}` | `PATCH /nodes/:id` | ✅ Match |
| Delete node | `DELETE /nodes/{id}` | `DELETE /nodes/:id` | ✅ Match |
| Node heartbeat | — | `POST /nodes/:id/heartbeat` | ➕ Extra |
| **Allocations** | | | |
| List allocations | `GET /nodes/{id}/allocations` | In allocation routes | ✅ Match |
| Create allocation | `POST /nodes/{id}/allocations` | In allocation routes | ✅ Match |
| Delete allocation | `DELETE /nodes/{id}/allocations/{id}` | In allocation routes | ✅ Match |
| **Servers** | | | |
| List servers | `GET /servers` | `GET /servers` | ✅ Match |
| Get server | `GET /servers/{id}` | `GET /servers/:id` | ✅ Match |
| External ID lookup | `GET /servers/external/{id}` | Not found | ❌ Missing |
| Create server | `POST /servers` | `POST /servers` | ✅ Match |
| Update details | `PATCH /servers/{id}/details` | `PATCH /servers/:id` | ✅ Match |
| Update build | `PATCH /servers/{id}/build` | Same update endpoint | ✅ Match |
| Update startup | `PATCH /servers/{id}/startup` | `PATCH /servers/:id` | ✅ Match |
| Suspend | `POST /servers/{id}/suspend` | `POST /servers/:id/suspend` | ✅ Match |
| Unsuspend | `POST /servers/{id}/unsuspend` | `POST /servers/:id/unsuspend` | ✅ Match |
| Reinstall | `POST /servers/{id}/reinstall` | `POST /servers/:id/reinstall` | ✅ Match |
| Transfer | `POST /servers/{id}/transfer` | `POST /servers/:id/transfer` | ✅ Match |
| Cancel transfer | `POST /servers/{id}/transfer/cancel` | Not found | ❌ Missing |
| Delete server | `DELETE /servers/{id}` | `DELETE /servers/:id` | ✅ Match |
| **Server Databases** | | | |
| List server databases | `GET /servers/{id}/databases` | `GET /servers/:id/databases` | ✅ Match |
| Get database | `GET /servers/{id}/databases/{id}` | `GET /servers/:id/databases/:id` | ✅ Match |
| Create database | `POST /servers/{id}/databases` | `POST /servers/:id/databases` | ✅ Match |
| Reset password | `POST /servers/{id}/databases/{id}/reset-password` | Not found | ❌ Missing |
| Delete database | `DELETE /servers/{id}/databases/{id}` | `DELETE /servers/:id/databases/:id` | ✅ Match |
| **Eggs/Templates** | | | |
| List | `GET /eggs` | `GET /templates` (renamed) | 🔄 Renamed |
| Get | `GET /eggs/{id}` | `GET /templates/:id` | 🔄 Renamed |
| Export | `GET /eggs/{id}/export` | Not found | ❌ Missing |
| Import | `POST /eggs/import` | `POST /templates/import` | 🔄 Renamed |
| Delete | `DELETE /eggs/{id}` | `DELETE /templates/:id` | 🔄 Renamed |
| **Database Hosts** | | | |
| List DB hosts | `GET /database-hosts` | `GET /database-hosts` | ✅ Match |
| Get DB host | `GET /database-hosts/{id}` | `GET /database-hosts/:id` | ✅ Match |
| Create DB host | `POST /database-hosts` | `POST /database-hosts` | ✅ Match |
| Update DB host | `PATCH /database-hosts/{id}` | `PATCH /database-hosts/:id` | ✅ Match |
| Delete DB host | `DELETE /database-hosts/{id}` | `DELETE /database-hosts/:id` | ✅ Match |
| **Mounts** | | | |
| List | `GET /mounts` | `GET /mounts` | ✅ Match |
| Get | `GET /mounts/{id}` | `GET /mounts/:id` | ✅ Match |
| Create | `POST /mounts` | `POST /mounts` | ✅ Match |
| Update | `PATCH /mounts/{id}` | `PATCH /mounts/:id` | ✅ Match |
| Delete | `DELETE /mounts/{id}` | `DELETE /mounts/:id` | ✅ Match |
| Sub-resource CRUD | Complex endpoints | Similar structure | ✅ Match |
| **Roles** | | | |
| List | `GET /roles` | `GET /roles` | ✅ Match |
| Get | `GET /roles/{id}` | `GET /roles/:id` | ✅ Match |
| Create | `POST /roles` | `POST /roles` | ✅ Match |
| Update | `PATCH /roles/{id}` | `PATCH /roles/:id` | ✅ Match |
| Delete | `DELETE /roles/{id}` | `DELETE /roles/:id` | ✅ Match |
| **Plugins** | | | |
| List | `GET /plugins` | `GET /plugins` | ✅ Match |
| Get | `GET /plugins/{id}` | `GET /plugins/:id` | ✅ Match |
| Import file | `POST /plugins/import/file` | `POST /plugins/import` | ✅ Match |
| Import URL | `POST /plugins/import/url` | Not separate endpoint | ❌ Missing |
| Install | `POST /plugins/{id}/install` | `POST /plugins/:id/install` | ✅ Match |
| Update | `POST /plugins/{id}/update` | `POST /plugins/:id/update` | ✅ Match |
| Uninstall | `POST /plugins/{id}/uninstall` | `POST /plugins/:id/uninstall` | ✅ Match |
| Enable/disable | `POST /plugins/{id}/enable|disable` | `POST /plugins/:id/toggle` | 🔄 Merged |

### Client API — `/api/client/*` vs `/api/v1/*`

| Endpoint | Pelican Panel | Gamepanel | Status |
|---|---|---|---|
| **Account** | | | |
| Get account | `GET /account` | `GET /account` | ✅ Match |
| Update username | `PUT /account/username` | `PUT /account/username` | ✅ Match |
| Update email | `PUT /account/email` | `PUT /account/email` | ✅ Match |
| Update password | `PUT /account/password` | `PUT /account/password` | ✅ Match |
| Activity log | `GET /account/activity` | `GET /account/activity` | ✅ Match |
| API keys CRUD | `GET/POST/DELETE /account/api-keys` | `GET/POST/DELETE /account/api-keys` | ✅ Match |
| SSH keys CRUD | `GET/POST/DELETE /account/ssh-keys` | Not found | ❌ Missing |
| **Servers** | | | |
| Get server | `GET /servers/{server}` | `GET /servers/:id` | ✅ Match |
| Websocket | `GET /servers/{server}/websocket` | `GET /servers/:id/ws/*` | ✅ Match |
| Resources | `GET /servers/{server}/resources` | `GET /servers/:id/resources` | ✅ Match |
| Activity | `GET /servers/{server}/activity` | `GET /servers/:id/activity` | ✅ Match |
| Send command | `POST /servers/{server}/command` | `POST /servers/:id/command` | ✅ Match |
| Power action | `POST /servers/{server}/power` | `POST /servers/:id/power` | ✅ Match |
| **Databases (client)** | | | |
| List | `GET /servers/{server}/databases` | `GET /servers/:id/databases` | ✅ Match |
| Create | `POST /servers/{server}/databases` | `POST /servers/:id/databases` | ✅ Match |
| Rotate password | `POST /servers/{server}/databases/{db}/rotate-password` | Not found | ❌ Missing |
| Delete | `DELETE /servers/{server}/databases/{db}` | `DELETE /servers/:id/databases/:id` | ✅ Match |
| **Files** | | | |
| List directory | `GET /servers/{server}/files/list` | `GET /servers/:id/files` | ✅ Match |
| File contents | `GET /servers/{server}/files/contents` | `GET /servers/:id/files/contents` | ✅ Match |
| Download | `GET /servers/{server}/files/download` | `GET /servers/:id/files/download-url` | ✅ Match |
| Rename | `PUT /servers/{server}/files/rename` | `PUT /servers/:id/files/rename` | ✅ Match |
| Copy | `POST /servers/{server}/files/copy` | `POST /servers/:id/files/copy` | ✅ Match |
| Write | `POST /servers/{server}/files/write` | `POST /servers/:id/files/write` | ✅ Match |
| Compress | `POST /servers/{server}/files/compress` | `POST /servers/:id/files/compress` | ✅ Match |
| Decompress | `POST /servers/{server}/files/decompress` | `POST /servers/:id/files/decompress` | ✅ Match |
| Delete | `POST /servers/{server}/files/delete` | `DELETE /servers/:id/files` | 🔄 Different verb |
| Create folder | `POST /servers/{server}/files/create-folder` | `POST /servers/:id/files/mkdir` | 🔄 Renamed |
| Chmod | `POST /servers/{server}/files/chmod` | `POST /servers/:id/files/chmod` | ✅ Match |
| Pull from URL | `POST /servers/{server}/files/pull` | Not found | ❌ Missing |
| Upload | `GET /servers/{server}/files/upload` | `POST /servers/:id/files/upload` | ✅ Match |
| **Schedules** | | | |
| List | `GET /servers/{server}/schedules` | `GET /servers/:id/schedules` | ✅ Match |
| Create | `POST /servers/{server}/schedules` | `POST /servers/:id/schedules` | ✅ Match |
| View | `GET /servers/{server}/schedules/{schedule}` | `GET /servers/:id/schedules/:id` | ✅ Match |
| Update | `POST /servers/{server}/schedules/{schedule}` | `PATCH /servers/:id/schedules/:id` | 🔄 Different verb |
| Execute | `POST /servers/{server}/schedules/{schedule}/execute` | `POST /servers/:id/schedules/:id/execute` | ✅ Match |
| Delete | `DELETE /servers/{server}/schedules/{schedule}` | `DELETE /servers/:id/schedules/:id` | ✅ Match |
| **Schedule Tasks** | | | |
| Create task | `POST /servers/{server}/schedules/{schedule}/tasks` | Similar | ✅ Match |
| Update task | `POST /servers/{server}/schedules/{schedule}/tasks/{task}` | Similar | ✅ Match |
| Delete task | `DELETE /servers/{server}/schedules/{schedule}/tasks/{task}` | Similar | ✅ Match |
| **Network/Allocations** | | | |
| List | `GET /servers/{server}/network/allocations` | `GET /servers/:id/network` | ✅ Match |
| Create | `POST /servers/{server}/network/allocations` | `POST /servers/:id/network` | ✅ Match |
| Update | `POST /servers/{server}/network/allocations/{alloc}` | `PATCH /servers/:id/network/:id` | 🔄 Different verb |
| Set primary | `POST /servers/{server}/network/allocations/{alloc}/primary` | `POST /servers/:id/network/:id/primary` | ✅ Match |
| Delete | `DELETE /servers/{server}/network/allocations/{alloc}` | `DELETE /servers/:id/network/:id` | ✅ Match |
| **Subusers** | | | |
| List | `GET /servers/{server}/users` | `GET /servers/:id/users` | ✅ Match |
| Create | `POST /servers/{server}/users` | `POST /servers/:id/users` | ✅ Match |
| View | `GET /servers/{server}/users/{user}` | `GET /servers/:id/users/:id` | ✅ Match |
| Update | `POST /servers/{server}/users/{user}` | `PATCH /servers/:id/users/:id` | 🔄 Different verb |
| Delete | `DELETE /servers/{server}/users/{user}` | `DELETE /servers/:id/users/:id` | ✅ Match |
| **Backups** | | | |
| List | `GET /servers/{server}/backups` | `GET /servers/:id/backups` | ✅ Match |
| Create | `POST /servers/{server}/backups` | `POST /servers/:id/backups` | ✅ Match |
| View | `GET /servers/{server}/backups/{backup}` | `GET /servers/:id/backups/:id` | ✅ Match |
| Download | `GET /servers/{server}/backups/{backup}/download` | `GET /servers/:id/backups/:id/download` | ✅ Match |
| Rename | `PUT /servers/{server}/backups/{backup}/rename` | Not found | ❌ Missing |
| Lock/unlock | `POST /servers/{server}/backups/{backup}/lock` | `POST /servers/:id/backups/:id/lock` | ✅ Match |
| Restore | `POST /servers/{server}/backups/{backup}/restore` | `POST /servers/:id/backups/:id/restore` | ✅ Match |
| Delete | `DELETE /servers/{server}/backups/{backup}` | `DELETE /servers/:id/backups/:id` | ✅ Match |
| **Startup** | | | |
| Get variables | `GET /servers/{server}/startup` | `GET /servers/:id/startup` | ✅ Match |
| Update variable | `PUT /servers/{server}/startup/variable` | `PATCH /servers/:id/startup` | ✅ Match |
| **Settings** | | | |
| Rename | `POST /servers/{server}/settings/rename` | `PATCH /servers/:id` | 🔄 Merged |
| Description | `POST /servers/{server}/settings/description` | `PATCH /servers/:id` | 🔄 Merged |
| Reinstall | `POST /servers/{server}/settings/reinstall` | `POST /servers/:id/reinstall` | ✅ Match |
| Docker image | `PUT /servers/{server}/settings/docker-image` | `PATCH /servers/:id` | 🔄 Merged |

### Remote API (Daemon Communication) — `/api/remote/*`

| Endpoint | Pelican Panel | Gamepanel | Status |
|---|---|---|---|
| SFTP auth | `POST /sftp/auth` | `POST /sftp/auth` | ✅ Match |
| List servers | `GET /servers` | `GET /servers` | ✅ Match |
| Reset state | `POST /servers/reset` | `POST /servers/reset` | ✅ Match |
| Activity push | `POST /activity` | `POST /servers/:id/activity` | ✅ Match |
| Get server | `GET /servers/{server}` | `GET /servers/:id` | ✅ Match |
| Get install | `GET /servers/{server}/install` | `GET /servers/:id/install` | ✅ Match |
| Post install | `POST /servers/{server}/install` | `POST /servers/:id/install` | ✅ Match |
| Transfer failure | `POST /servers/{server}/transfer/failure` | `POST /servers/:id/transfer/failure` | ✅ Match |
| Transfer success | `POST /servers/{server}/transfer/success` | `POST /servers/:id/transfer/success` | ✅ Match |
| Container status | `POST /servers/{server}/container/status` | `POST /servers/:id/status` | ✅ Match |
| Get backup | `GET /backups/{backup}` | Similar | ✅ Match |
| Backup status | `POST /backups/{backup}` | `POST /servers/:id/backups/status` | ✅ Match |
| Backup restore | `POST /backups/{backup}/restore` | Same pattern | ✅ Match |

---

## Config/Feature Comparison

| Feature | Pelican Panel | Gamepanel | Notes |
|---|---|---|---|
| **Authentication** | | | |
| Email/password login | ✅ | ✅ | |
| TOTP 2FA | ✅ | ✅ | |
| WebAuthn/Passkeys | ❌ | ✅ | Extra feature in Gamepanel |
| OAuth login | ✅ (stubs) | ✅ (Discord, Steam, Authentik) | Gamepanel has more |
| Password reset | ✅ | ✅ | |
| Account recovery | ❌ | ✅ | Extra feature |
| Session management | ✅ (Sanctum) | ✅ (JWT + cookies) | Different approach |
| API token auth | ✅ (Sanctum) | ✅ (JWT) | |
| **Server Management** | | | |
| CRUD | ✅ | ✅ | |
| Suspend/Unsuspend | ✅ | ✅ | |
| Reinstall | ✅ | ✅ | |
| Transfer between nodes | ✅ | ✅ | |
| Build config | ✅ | ✅ | |
| Startup management | ✅ | ✅ | |
| **Database Hosts** | | | |
| CRUD + TLS | ✅ | ✅ | |
| **Mounts** | | | |
| CRUD + associations | ✅ | ✅ | |
| **Templates/Eggs** | | | |
| CRUD + Export/Import | ✅ | ✅ (as "templates") | Renamed |
| Egg variables | ✅ | ✅ | |
| Docker images | ✅ | ✅ | |
| Install scripts | ✅ | ✅ | |
| **Backups** | | | |
| Create/Restore/Delete | ✅ | ✅ | |
| S3 storage | ✅ | ✅ | |
| Daemon-local | ✅ | ✅ | |
| Locking | ✅ | ✅ | |
| Throttling | ✅ | ✅ | |
| **Schedules** | | | |
| CRON + tasks | ✅ | ✅ | |
| **Files** | | | |
| Directory/Read/Write | ✅ | ✅ | |
| Upload/Download | ✅ | ✅ | |
| Compress/Decompress | ✅ | ✅ | |
| Copy/Rename/Delete | ✅ | ✅ | |
| SFTP | ✅ | ✅ | |
| File pull from URL | ✅ | ❌ | Missing |
| **Subusers** | | | |
| CRUD + permissions | ✅ | ✅ | |
| **Activity Logging** | | | |
| Per-server/per-user | ✅ | ✅ | |
| Pruning | ✅ | ✅ | |
| Admin hiding | ✅ | ❌ | Missing config |
| **Webhooks** | | | |
| Event-based | ✅ | ✅ | |
| **Plugins** | | | |
| Plugin system | ✅ (Sushi/Composer) | ✅ (Go service) | Different arch |
| Import/Install/Update/Uninstall | ✅ | ✅ | |
| Marketplace | ✅ (egg index) | ❌ | Missing |
| **Rate Limiting** | | | |
| Per-endpoint | ✅ | ✅ | Configurable via API |
| Login rate limiting | ✅ | ✅ | |
| **Health Checks** | | | |
| Health endpoints | ✅ | ✅ | Enhanced with history |
| Slack/mail/OhDear | ✅ | ❌ | Missing |
| **Notifications** | | | |
| Account created email | ✅ | ✅ | |
| Subuser added/removed | ✅ | ❌ | Missing |
| Install complete | ✅ | ❌ | Missing |
| Backup complete | ✅ | ❌ | Missing |
| **i18n** | | | |
| Languages | 20+ directories | 8 JSON files | Pelican more extensive |
| **Security** | | | |
| CSRF | ✅ | ✅ | |
| CORS | ✅ | ✅ | |
| Security headers | ✅ | ✅ | |
| IP access control | ✅ | ✅ | |
| Maintenance mode | ✅ | ❌ | Missing |

---

## Missing Features (Pelican has, Gamepanel does not)

### High Priority
1. **SSH Key Management** — Pelican has full SSH key CRUD for users (`/account/ssh-keys`). Gamepanel has no SSH key support.
2. **Pelican Egg Export** — `GET /eggs/{id}/export` — Gamepanel templates lack JSON export functionality.
3. **File Pull from URL** — Pelican allows pulling files from remote URLs directly into server filesystem.
4. **Backup Rename** — Pelican supports `PUT /backups/{backup}/rename` for renaming backups.
5. **Server Transfer Cancel** — Pelican supports canceling in-progress transfers.
6. **Database Password Rotation** — Pelican's `POST /databases/{db}/rotate-password` for client-side password reset.
7. **External ID Lookups** — Both users and servers can be looked up by external_id in Pelican.
8. **Email Notifications** — Pelican sends emails for: account creation, subuser added/removed, install complete, reinstall, backup complete. Gamepanel has the mail service but minimal template notifications.
9. **Node Configuration Endpoint** — Pelican exposes `GET /nodes/{id}/configuration` for daemon bootstrap config.

### Medium Priority
10. **Role Assign/Remove endpoints** — Pelican has dedicated `users/{id}/roles/assign` and `roles/remove` endpoints.
11. **Plugin URL Import** — Separate `POST /plugins/import/url` endpoint in Pelican (Gamepanel may merge with file import).
12. **Plugin Marketplace** — Pelican has egg index URL support (`PANEL_EGG_INDEX_URL`) for discovering community eggs/plugins.
13. **Activity Admin Hiding** — Pelican supports hiding admin activity from client API responses.
14. **Health Check Notifications** (Slack/mail/OhDear) — Pelican integrates with Spatie Health and OhDear.
15. **Maintenance Mode** — Pelican has a maintenance mode middleware.
16. **Backup Prune Age config** — Pelican has configurable auto-failure for stale backups.

### Low Priority
17. **Node Tags** — Pelican supports tags on nodes for organization.
18. **Binary Prefix Display** — Pelican can use binary (TiB/GiB) vs decimal (TB/GB) display.
19. **Client Allocation Creation Control** — Pelican allows admins to enable/disable client-side allocation creation.

---

## Extra Features in Gamepanel

### Infrastructure & Operations
1. **Cloud Manager** — Full cloud provider integration (`forge/api/internal/cloud/`) for managing cloud resources.
2. **AutoScaler** — Policy-based autoscaling for nodes/servers.
3. **Deployment Pipelines** — Multi-stage deployment system.
4. **Load Balancer** — Built-in load balancing across servers/nodes.
5. **Failover Clusters** — Automatic failover detection and recovery.
6. **Traffic Manager** — Traffic routing and management.
7. **Predictive Scheduler** — ML-assisted server scheduling (`PredictiveScorer`).
8. **Regions** — Multi-region support (beyond Pelican's simple locations).
9. **Observability Stack** — Integrated Grafana + Prometheus monitoring.
10. **WebAuthn/Passkeys** — Passwordless authentication.
11. **Social Auth** — Multiple OAuth providers with admin management UI.
12. **OAuth2 Token Endpoint** — PufferPanel-compatible OAuth2 token issuance.
13. **Crash Detection** — Server crash event tracking and alerting.
14. **Recovery Coordinator** — Automatic recovery from node failures.
15. **Evacuation Planner** — Planned node evacuation for maintenance.
16. **Maintenance Windows** — Node draining and maintenance mode management.
17. **Configurable Rate Limits via API** — Runtime rate limit configuration.
18. **File Download Tickets** — Signed, time-limited download URLs.
19. **S3 Backup Configuration UI** — Admin UI for S3 backup config.
20. **Account Recovery Flow** — Passwordless account recovery.
21. **Multi-DB Support** — PostgreSQL, MySQL, SQLite in one config.

### Tech Stack
22. **Shared TypeScript Types** — `packages/shared-types/` for type safety.
23. **UI Component Library** — `packages/ui/` with shared React components.
24. **SDK Package** — `packages/sdk/` for programmatic panel access.
25. **Integrated Daemon** — Beacon lives in the same repo for consistency.

---

## Architectural Differences

### 1. Language / Runtime
- **Pelican:** PHP 8.2+ with Laravel 11 — request lifecycle, Eloquent ORM, Blade templating, artisan CLI.
- **Gamepanel:** Go 1.22+ with Fiber v2 — compiled binary, goroutine-based concurrency, no ORM (direct SQL).

### 2. Frontend Architecture
- **Pelican:** Server-rendered Filament (Livewire + Blade) — PHP renders HTML on each request, state managed server-side.
- **Gamepanel:** Next.js 14 App Router with React — client-side rendering, API calls to separate Go backend, SPA-like experience.

### 3. Daemon Integration
- **Pelican:** Wings is a **separate** Go binary maintained as an independent project. Communication via REST API.
- **Gamepanel:** Beacon is a **monorepo component** (`beacon/`) sharing types and conventions with Forge. Still a separate binary but codebase is unified.

### 4. Database / Storage Layer
- **Pelican:** Uses Eloquent ORM with migrations in PHP. Supports SQLite (default), MySQL, PostgreSQL.
- **Gamepanel:** Uses raw SQL with Go `database/sql` and `pgx` driver. All queries in `forge/api/internal/store/`. SQL migrations only for PostgreSQL (with some MySQL compatibility).

### 5. Authentication
- **Pelican:** Laravel Sanctum for SPA auth + token-based API auth. Session cookies for web, Bearer tokens for API.
- **Gamepanel:** JWT-based auth with HttpOnly session cookies + CSRF tokens. Separate login/2FA checkpoint/WebAuthn flows.

### 6. Plugin System
- **Pelican:** PHP-based plugins as Composer packages with Sushi (SQLite in-memory) for storage. Plugins can register Filament pages, Livewire components, etc.
- **Gamepanel:** Go plugin service that manages manifests in the database. Plugins are tracked but there's no in-language plugin runtime (Go has no runtime plugin system like PHP).

### 7. Extensions Framework
- **Pelican:** Has a formal Extensions directory (`app/Extensions/`) with abstractions for backends (S3/wings backup adapters), captcha providers (Turnstile), avatar providers, OAuth providers, task runners, and feature flags.
- **Gamepanel:** No equivalent formal extension system. Features are integrated directly.

### 8. API Design
- **Pelican:** Three separate API groups: Application (admin), Client (user), Remote (daemon). Each with distinct auth middleware.
- **Gamepanel:** Single `/api/v1/` prefix with middleware chaining. Uses `requireRole("admin")` and `requireServerPermission()` middleware for access control.

### 9. Deployment
- **Pelican:** Docker with Caddy (reverse proxy), Supervisord (process manager), crontab for scheduled tasks.
- **Gamepanel:** Docker Compose with Nginx, Grafana + Prometheus monitoring stack. No process manager (container-native).

### 10. Permission/Policy System
- **Pelican:** Spatie Laravel Permission (roles/permissions) + Filament authorization. SubuserPermission enum with 24+ granular permission strings.
- **Gamepanel:** `policies/` package with role-based access + `requireServerPermission()` middleware. Permission constants like `PermControlConsole`, `PermFileReadContent`, etc.

### 11. Job Queue
- **Pelican:** Laravel queue with database driver. Jobs for egg installation, webhook dispatch, SFTP access revocation.
- **Gamepanel:** Custom schedule runner + queue service. No formal job persistence layer (uses PostgreSQL).

### 12. Testing Approach
- **Pelican:** Pest PHP framework with Integration, Feature, Unit, and Filament test suites. Uses fixtures, traits, and seeders.
- **Gamepanel:** Go standard testing with testify. Tests are concentrated in `handlers_*_test.go` files and `store_*_test.go` files. Uses integration tests with Docker.

### 13. Error Handling
- **Pelican:** Structured exception hierarchy (Http exceptions, Service exceptions, Model exceptions) with dedicated exception classes for each domain.
- **Gamepanel:** Inline error handling with Fiber's error types. No formal exception hierarchy.

### 14. Validation
- **Pelican:** Laravel validation rules + custom Rule classes (e.g., `Port.php`). Validation in FormRequests.
- **Gamepanel:** Manual request body parsing + validation in handler functions. Some validation in `validation.go`.

### 15. Event System
- **Pelican:** Laravel events/listeners with dedicated event classes for auth, server, and user events.
- **Gamepanel:** Event store + relay system with WebSocket broadcasting for real-time events.

---

## Recommendations

### Immediate (High Impact)
1. **Implement SSH Key management** — Missing API endpoints and storage for user SSH keys.
2. **Add Egg/Template export** — JSON export for templates is used for sharing and migration.
3. **Add file pull from URL** — Feature parity with Pelican, commonly used for plugin/mod installations.
4. **Add backup rename endpoint** — Simple missing CRUD operation.
5. **Add database password rotation for clients** — Essential for client workflow parity.

### Short-term (Medium Impact)
6. **Add email notification templates** — Account creation, subuser events, install completion.
7. **Add server transfer cancel endpoint** — Minor but useful for operations.
8. **Add external_id lookups** — For integrations that reference external systems.
9. **Add plugin URL import** — Separate endpoint for URL-based plugin import.
10. **Add node configuration endpoint** — Useful for daemon auto-configuration.

### Long-term (Architecture/Polish)
11. **Add more i18n locales** — Gamepanel has 8, Pelican has 20+.
12. **Consider formal extension framework** — Pelican's Extensions directory provides clean separation for pluggable backends.
13. **Add health check notification channels** — Slack/mail alerts for system health.
14. **Add maintenance mode middleware** — Useful for production deployments.
15. **Consider dedicated exception types** — Improves error handling and API consistency.
