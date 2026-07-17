# Gamepanel vs Pterodactyl Panel — Comprehensive Audit Comparison

**Date:** 2026-07-16
**Gamepanel:** Go (Fiber) + Next.js + Go daemon (Beacon)
**Pterodactyl:** Laravel PHP + React SPA + Wings (Go daemon)

---

## Executive Summary

Gamepanel is a **ground-up reimplementation** of the Pterodactyl panel concept with significant architectural divergence. While it covers the core Pterodactyl feature set (server lifecycle, nodes, allocations, users, backups, schedules, subusers, databases, mounts, nests/eggs, API keys, activity logging, SFTP auth, remote daemon communication), it introduces substantial **new infrastructure** not present in Pterodactyl: cloud provider orchestration, autoscaling, blue-green deployments, load balancing, failover policies, traffic management, predictive scheduling, observability/timeline, regions, plugin marketplace, OAuth2, WebAuthn, social auth, and webhooks.

**Key finding:** Gamepanel is not a fork — it is a **from-scratch reimplementation** in Go/Next.js that extends the Pterodactyl model with cloud-native infrastructure features. The core server/node/user/backup/schedule/subuser feature set has near-parity, but Gamepanel adds ~15+ major features Pterodactyl does not have.

---

## Directory/File Structure Comparison

| Area | Pterodactyl (PHP/Laravel) | Gamepanel (Go/Next.js) | Parity |
|------|--------------------------|----------------------|--------|
| **API Framework** | Laravel routes + Controllers | Go Fiber routes + handlers | Different stack, same pattern |
| **API Routes** | `routes/api-client.php`, `api-application.php`, `api-remote.php`, `auth.php`, `admin.php`, `base.php` | `server.go` (inline route registration) + handler files | Gamepanel uses single-file route registration vs Pterodactyl's file-per-route-group |
| **Controllers** | `app/Http/Controllers/Api/{Client,Application,Remote}/` | `forge/api/internal/http/handlers_*.go` | Same logical grouping, different naming |
| **Models** | `app/Models/*.php` (36 files) | `forge/api/internal/models/` (7 files) + `store/` (82 files) | Gamepanel uses store layer instead of ORM models |
| **Migrations** | `database/migrations/` (195 PHP files) | `forge/api/migrations/` (64 SQL files) | Pterodactyl has 3x more migrations (older project) |
| **Config** | `config/*.php` (29 files) | `forge/api/config/` (6 Go files) + `forge/api/internal/config/` (3 Go files) | Different format, similar concerns |
| **Frontend** | `resources/scripts/` (React SPA) + `resources/views/` (Blade) | `forge/web/` (Next.js App Router) | Different framework, same SPA concept |
| **Daemon** | `wings/` (separate Go project) | `beacon/` (Go daemon) | Same concept, different implementation |
| **Tests** | `tests/` (PHPUnit, Unit + Integration) | `forge/api/internal/http/*_test.go`, `forge/api/internal/store/*_test.go` | Go table-driven tests |
| **Infrastructure** | `Dockerfile`, `docker-compose.example.yml` | `infra/compose.yml`, `infra/grafana/`, `infra/prometheus/` | Gamepanel has richer infra |
| **i18n** | `resources/lang/` | `lang/` (8 JSON files) | Similar |
| **Config** | `config/*.php` (Laravel env-based) | `forge/api/config/*.go` + `config/config.yaml.example` | Different format, same env-driven pattern |

---

## API Routes Comparison

### Client API (`/api/client` in Pterodactyl, `/api/v1` in Gamepanel)

| Pterodactyl Route | Gamepanel Route | Status |
|---|---|---|
| `GET /api/client` | `GET /api/v1/` (panel settings) | Equivalent |
| `GET /api/client/permissions` | `GET /api/v1/permissions` | Equivalent |
| `GET /api/client/account` | `GET /api/v1/auth/me` | Equivalent |
| `PUT /api/client/account/email` | `PUT /api/v1/auth/me/email` | Equivalent |
| `PUT /api/client/account/password` | `PUT /api/v1/auth/me/password` | Equivalent |
| `GET /api/client/account/activity` | `GET /api/v1/activity` | Equivalent |
| `GET /api/client/account/two-factor` | `GET /api/v1/auth/2fa/setup` | Equivalent |
| `POST /api/client/account/two-factor` | `POST /api/v1/auth/2fa/verify` | Equivalent |
| `DELETE /api/client/account/two-factor/disable` | `POST /api/v1/auth/2fa/disable` | Equivalent |
| `GET /api/client/account/api-keys` | `GET /api/v1/api-keys` | Equivalent |
| `POST /api/client/account/api-keys` | `POST /api/v1/api-keys` | Equivalent |
| `DELETE /api/client/account/api-keys/{id}` | `DELETE /api/v1/api-keys/:id` | Equivalent |
| `GET /api/client/account/ssh-keys` | `GET /api/v1/ssh-keys` | Equivalent |
| `POST /api/client/account/ssh-keys` | `POST /api/v1/ssh-keys` | Equivalent |
| `POST /api/client/account/ssh-keys/remove` | `DELETE /api/v1/ssh-keys/:id` | Equivalent |
| `GET /api/client/servers/{server}` | `GET /api/v1/servers/:id` | Equivalent |
| `GET /api/client/servers/{server}/websocket` | `GET /api/v1/servers/:id/ws/*` | Equivalent |
| `GET /api/client/servers/{server}/resources` | `GET /api/v1/servers/:id/resources` | Equivalent |
| `GET /api/client/servers/{server}/activity` | `GET /api/v1/servers/:id/activity` | Equivalent |
| `POST /api/client/servers/{server}/command` | `POST /api/v1/servers/:id/command` | Equivalent |
| `POST /api/client/servers/{server}/power` | `POST /api/v1/servers/:id/power` | Equivalent |
| `GET /api/client/servers/{server}/databases` | `GET /api/v1/servers/:id/databases` | Equivalent |
| `POST /api/client/servers/{server}/databases` | `POST /api/v1/servers/:id/databases` | Equivalent |
| `DELETE /api/client/servers/{server}/databases/{db}` | `DELETE /api/v1/servers/:id/databases/:dbId` | Equivalent |
| `POST /api/client/servers/{server}/databases/{db}/rotate-password` | `POST /api/v1/servers/:id/databases/:dbId/rotate-password` | Equivalent |
| `GET /api/client/servers/{server}/files/list` | `GET /api/v1/servers/:id/files/list` | Equivalent |
| `GET /api/client/servers/{server}/files/contents` | `GET /api/v1/servers/:id/files/read` | Equivalent |
| `GET /api/client/servers/{server}/files/download` | `GET /api/v1/servers/:id/files/download-url` | Equivalent |
| `PUT /api/client/servers/{server}/files/rename` | `POST /api/v1/servers/:id/files/rename` | Equivalent |
| `POST /api/client/servers/{server}/files/copy` | `POST /api/v1/servers/:id/files/copy` | Equivalent |
| `POST /api/client/servers/{server}/files/write` | `POST /api/v1/servers/:id/files/write` | Equivalent |
| `POST /api/client/servers/{server}/files/compress` | `POST /api/v1/servers/:id/files/compress` | Equivalent |
| `POST /api/client/servers/{server}/files/decompress` | `POST /api/v1/servers/:id/files/decompress` | Equivalent |
| `POST /api/client/servers/{server}/files/delete` | `POST /api/v1/servers/:id/files/delete` | Equivalent |
| `POST /api/client/servers/{server}/files/create-folder` | `POST /api/v1/servers/:id/files/create-folder` | Equivalent |
| `POST /api/client/servers/{server}/files/chmod` | `POST /api/v1/servers/:id/files/chmod` | Equivalent |
| `POST /api/client/servers/{server}/files/pull` | `POST /api/v1/servers/:id/files/pull` | Equivalent |
| `GET /api/client/servers/{server}/files/upload` | `POST /api/v1/servers/:id/files/upload` | Equivalent |
| `GET /api/client/servers/{server}/schedules` | `GET /api/v1/servers/:id/schedules` | Equivalent |
| `POST /api/client/servers/{server}/schedules` | `POST /api/v1/servers/:id/schedules` | Equivalent |
| `GET /api/client/servers/{server}/schedules/{s}` | `GET /api/v1/servers/:id/schedules/:schedId` | Equivalent |
| `POST /api/client/servers/{server}/schedules/{s}` | `PUT /api/v1/servers/:id/schedules/:schedId` | Equivalent |
| `POST /api/client/servers/{server}/schedules/{s}/execute` | `POST /api/v1/servers/:id/schedules/:schedId/execute` | Equivalent |
| `DELETE /api/client/servers/{server}/schedules/{s}` | `DELETE /api/v1/servers/:id/schedules/:schedId` | Equivalent |
| `POST /api/client/servers/{server}/schedules/{s}/tasks` | `POST /api/v1/servers/:id/schedules/:schedId/tasks` | Equivalent |
| `POST /api/client/servers/{server}/schedules/{s}/tasks/{t}` | `PUT /api/v1/servers/:id/schedules/:schedId/tasks/:taskId` | Equivalent |
| `DELETE /api/client/servers/{server}/schedules/{s}/tasks/{t}` | `DELETE /api/v1/servers/:id/schedules/:schedId/tasks/:taskId` | Equivalent |
| `GET /api/client/servers/{server}/network/allocations` | `GET /api/v1/servers/:id/allocations` | Equivalent |
| `POST /api/client/servers/{server}/network/allocations` | `POST /api/v1/servers/:id/allocations` | Equivalent |
| `POST /api/client/servers/{server}/network/allocations/{a}` | `PUT /api/v1/servers/:id/allocations/:allocId` | Equivalent |
| `POST /api/client/servers/{server}/network/allocations/{a}/primary` | `POST /api/v1/servers/:id/allocations/:allocId/primary` | Equivalent |
| `DELETE /api/client/servers/{server}/network/allocations/{a}` | `DELETE /api/v1/servers/:id/allocations/:allocId` | Equivalent |
| `GET /api/client/servers/{server}/users` | `GET /api/v1/servers/:id/users` | Equivalent |
| `POST /api/client/servers/{server}/users` | `POST /api/v1/servers/:id/users` | Equivalent |
| `GET /api/client/servers/{server}/users/{u}` | `GET /api/v1/servers/:id/users/:userId` | Equivalent |
| `POST /api/client/servers/{server}/users/{u}` | `PUT /api/v1/servers/:id/users/:userId` | Equivalent |
| `DELETE /api/client/servers/{server}/users/{u}` | `DELETE /api/v1/servers/:id/users/:userId` | Equivalent |
| `GET /api/client/servers/{server}/backups` | `GET /api/v1/servers/:id/backups` | Equivalent |
| `POST /api/client/servers/{server}/backups` | `POST /api/v1/servers/:id/backups` | Equivalent |
| `GET /api/client/servers/{server}/backups/{b}` | `GET /api/v1/servers/:id/backups/:backupId` | Equivalent |
| `GET /api/client/servers/{server}/backups/{b}/download` | `GET /api/v1/servers/:id/backups/:backupId/download` | Equivalent |
| `POST /api/client/servers/{server}/backups/{b}/lock` | `POST /api/v1/servers/:id/backups/:backupId/lock` | Equivalent |
| `POST /api/client/servers/{server}/backups/{b}/restore` | `POST /api/v1/servers/:id/backups/:backupId/restore` | Equivalent |
| `DELETE /api/client/servers/{server}/backups/{b}` | `DELETE /api/v1/servers/:id/backups/:backupId` | Equivalent |
| `GET /api/client/servers/{server}/startup` | `GET /api/v1/servers/:id/startup` | Equivalent |
| `PUT /api/client/servers/{server}/startup/variable` | `PUT /api/v1/servers/:id/startup/variable` | Equivalent |
| `POST /api/client/servers/{server}/settings/rename` | `POST /api/v1/servers/:id/settings/rename` | Equivalent |
| `POST /api/client/servers/{server}/settings/reinstall` | `POST /api/v1/servers/:id/settings/reinstall` | Equivalent |
| `PUT /api/client/servers/{server}/settings/docker-image` | `PUT /api/v1/servers/:id/settings/docker-image` | Equivalent |

### Application API (`/api/application` in Pterodactyl, `/api/v1` admin routes in Gamepanel)

| Pterodactyl Route | Gamepanel Route | Status |
|---|---|---|
| `GET /api/application/users` | `GET /api/v1/users` | Equivalent |
| `GET /api/application/users/{id}` | `GET /api/v1/users/:id` | Equivalent |
| `GET /api/application/users/external/{ext_id}` | Not implemented | **MISSING** |
| `POST /api/application/users` | `POST /api/v1/users` | Equivalent |
| `PATCH /api/application/users/{id}` | `PUT /api/v1/users/:id` | Equivalent |
| `DELETE /api/application/users/{id}` | `DELETE /api/v1/users/:id` | Equivalent |
| `GET /api/application/nodes` | `GET /api/v1/nodes` | Equivalent |
| `GET /api/application/nodes/deployable` | `GET /api/v1/nodes/deployable` | Equivalent |
| `GET /api/application/nodes/{id}` | `GET /api/v1/nodes/:id` | Equivalent |
| `GET /api/application/nodes/{id}/configuration` | `GET /api/v1/nodes/:id/configuration` | Equivalent |
| `POST /api/application/nodes` | `POST /api/v1/nodes` | Equivalent |
| `PATCH /api/application/nodes/{id}` | `PUT /api/v1/nodes/:id` | Equivalent |
| `DELETE /api/application/nodes/{id}` | `DELETE /api/v1/nodes/:id` | Equivalent |
| `GET /api/application/nodes/{id}/allocations` | `GET /api/v1/nodes/:id/allocations` | Equivalent |
| `POST /api/application/nodes/{id}/allocations` | `POST /api/v1/nodes/:id/allocations` | Equivalent |
| `DELETE /api/application/nodes/{id}/allocations/{a}` | `DELETE /api/v1/nodes/:id/allocations/:allocId` | Equivalent |
| `GET /api/application/locations` | `GET /api/v1/locations` | Equivalent |
| `GET /api/application/locations/{id}` | `GET /api/v1/locations/:id` | Equivalent |
| `POST /api/application/locations` | `POST /api/v1/locations` | Equivalent |
| `PATCH /api/application/locations/{id}` | `PUT /api/v1/locations/:id` | Equivalent |
| `DELETE /api/application/locations/{id}` | `DELETE /api/v1/locations/:id` | Equivalent |
| `GET /api/application/servers` | `GET /api/v1/servers` | Equivalent |
| `GET /api/application/servers/{id}` | `GET /api/v1/servers/:id` | Equivalent |
| `GET /api/application/servers/external/{ext_id}` | Not implemented | **MISSING** |
| `PATCH /api/application/servers/{id}/details` | `PUT /api/v1/servers/:id` | Equivalent |
| `PATCH /api/application/servers/{id}/build` | `PUT /api/v1/servers/:id/build` | Equivalent |
| `PATCH /api/application/servers/{id}/startup` | `PUT /api/v1/servers/:id/startup` | Equivalent |
| `POST /api/application/servers` | `POST /api/v1/servers` | Equivalent |
| `POST /api/application/servers/{id}/suspend` | `POST /api/v1/servers/:id/suspend` | Equivalent |
| `POST /api/application/servers/{id}/unsuspend` | `POST /api/v1/servers/:id/unsuspend` | Equivalent |
| `POST /api/application/servers/{id}/reinstall` | `POST /api/v1/servers/:id/reinstall` | Equivalent |
| `DELETE /api/application/servers/{id}` | `DELETE /api/v1/servers/:id` | Equivalent |
| `GET /api/application/servers/{id}/databases` | `GET /api/v1/servers/:id/databases` | Equivalent |
| `GET /api/application/servers/{id}/databases/{db}` | `GET /api/v1/servers/:id/databases/:dbId` | Equivalent |
| `POST /api/application/servers/{id}/databases` | `POST /api/v1/servers/:id/databases` | Equivalent |
| `POST /api/application/servers/{id}/databases/{db}/reset-password` | `POST /api/v1/servers/:id/databases/:dbId/rotate-password` | Equivalent |
| `DELETE /api/application/servers/{id}/databases/{db}` | `DELETE /api/v1/servers/:id/databases/:dbId` | Equivalent |
| `GET /api/application/nests` | `GET /api/v1/nests` | Equivalent |
| `GET /api/application/nests/{id}` | `GET /api/v1/nests/:id` | Equivalent |
| `GET /api/application/nests/{id}/eggs` | `GET /api/v1/nests/:id/eggs` | Equivalent |
| `GET /api/application/nests/{id}/eggs/{egg}` | `GET /api/v1/nests/:id/eggs/:eggId` | Equivalent |

### Remote API (`/api/remote`)

| Pterodactyl Route | Gamepanel Route | Status |
|---|---|---|
| `POST /api/remote/sftp/auth` | `POST /api/remote/sftp/auth` | Equivalent |
| `GET /api/remote/servers` | `GET /api/remote/servers` | Equivalent |
| `POST /api/remote/servers/reset` | `POST /api/remote/servers/reset` | Equivalent |
| `POST /api/remote/activity` | `POST /api/remote/servers/:id/activity` | Equivalent |
| `GET /api/remote/servers/{uuid}` | `GET /api/remote/servers/:id` | Equivalent |
| `GET /api/remote/servers/{uuid}/install` | `GET /api/remote/servers/:id/install` | Equivalent |
| `POST /api/remote/servers/{uuid}/install` | `POST /api/remote/servers/:id/install` | Equivalent |
| `POST /api/remote/servers/{uuid}/transfer/failure` | `POST /api/remote/servers/:id/transfer/failure` | Equivalent (stub) |
| `POST /api/remote/servers/{uuid}/transfer/success` | `POST /api/remote/servers/:id/transfer/success` | Equivalent (stub) |
| `GET /api/remote/backups/{backup}` | `GET /api/remote/backups/:id` | Equivalent |
| `POST /api/remote/backups/{backup}` | `POST /api/remote/backups/:id` | Equivalent |
| `POST /api/remote/backups/{backup}/restore` | `POST /api/remote/backups/:id/restore` | Equivalent |

### Auth Routes

| Pterodactyl Route | Gamepanel Route | Status |
|---|---|---|
| `GET /auth/login` | SPA-rendered | Equivalent |
| `POST /auth/login` | `POST /api/v1/auth/login` | Equivalent |
| `POST /auth/login/checkpoint` | `POST /api/v1/auth/login/checkpoint` | Equivalent |
| `POST /auth/logout` | `POST /api/v1/auth/logout` | Equivalent |
| `GET /auth/password` | SPA-rendered | Equivalent |
| `POST /auth/password` | `POST /api/v1/auth/password/forgot` | Equivalent |
| `GET /auth/password/reset/{token}` | SPA-rendered | Equivalent |
| `POST /auth/password/reset` | `POST /api/v1/auth/password/reset` | Equivalent |

---

## Config/Feature Comparison

| Feature | Pterodactyl | Gamepanel | Notes |
|---------|------------|-----------|-------|
| **Database** | MySQL/MariaDB only | PostgreSQL, MySQL, SQLite | Gamepanel supports 3 drivers |
| **Cache** | Redis, file | Redis | Similar |
| **Session** | Redis (default), file, cookie, database | JWT + HttpOnly cookies | Different approach |
| **Mail** | SMTP, SES, Mailgun, Postmark, Sendmail, Log | SMTP (configurable) | Pterodactyl has more mail drivers |
| **Backup Storage** | Wings (local), S3 | Local, S3 | Equivalent |
| **2FA** | TOTP (Google Authenticator) | TOTP + WebAuthn (passkeys) | Gamepanel adds WebAuthn |
| **Social Auth** | None | Discord, Steam, Authentik | **EXTRA** |
| **OAuth2** | None (Sanctum tokens only) | RFC 6749 client_credentials | **EXTRA** |
| **Plugins** | None | Plugin marketplace (metadata only) | **EXTRA** |
| **Webhooks** | None | Webhook dispatch system | **EXTRA** |
| **Observability** | None | Timeline, correlations, heartbeat monitor | **EXTRA** |
| **Auto-scaling** | None | Auto-scaling policies | **EXTRA** |
| **Blue/Green Deploy** | None | Deployment service | **EXTRA** |
| **Load Balancer** | None | Target groups, routing | **EXTRA** |
| **Failover** | None | Failover policies | **EXTRA** |
| **Traffic Manager** | None | Routing rules | **EXTRA** |
| **Predictive Scheduling** | None | Predictive scoring, affinity rules | **EXTRA** |
| **Cloud Orchestration** | None | Cloud provider integration (multi-cloud) | **EXTRA** |
| **Regions** | None (locations only) | Regions (multi-region node grouping) | **EXTRA** |
| **Crash Detection** | None | Crash event tracking | **EXTRA** |
| **Observability** | None | Observability timeline, correlations | **EXTRA** |
| **Rate Limiting** | Throttle middleware | Per-endpoint rate limiting (Redis) | Gamepanel more granular |
| **IP Access Control** | None | IP whitelist/blacklist per endpoint | **EXTRA** |
| **CSRF Protection** | Laravel CSRF | Custom CSRF token middleware | Equivalent |
| **Security Headers** | `SetSecurityHeaders` middleware | `SecurityHeaders()` middleware | Equivalent |
| **i18n** | Laravel translation | Go i18n service + JSON files | Equivalent |
| **ReCAPTCHA** | `VerifyReCaptcha` middleware | Not implemented | **MISSING** |
| **Telemetry** | Telemetry service | Not implemented | **MISSING** |
| **Maintenance Mode** | `MaintenanceMiddleware` | Not implemented | **MISSING** |

---

## Missing Features (Pterodactyl has, Gamepanel does not)

1. **ReCAPTCHA integration** — Pterodactyl has `VerifyReCaptcha` middleware on login and forgot-password endpoints. Gamepanel has no captcha support.

2. **External user/server lookup by external_id** — Pterodactyl supports `GET /api/application/users/external/{external_id}` and `GET /api/application/servers/external/{external_id}`. Gamepanel does not.

3. **Maintenance mode** — Pterodactyl has `MaintenanceMiddleware` for putting the panel into maintenance mode. Gamepanel has no equivalent.

4. **Telemetry** — Pterodactyl has a telemetry service that reports anonymous usage data. Gamepanel does not.

5. **CDN version check** — Pterodactyl checks `cdn.pterodactyl.io` for latest release version. Gamepanel has no version check.

6. **Per-schedule task limit** — Pterodactyl enforces `PTERODACTYL_PER_SCHEDULE_TASK_LIMIT` (default 10). Gamepanel does not appear to enforce this.

7. **File max edit size** — Pterodactyl enforces `PTERODACTYL_FILES_MAX_EDIT_SIZE` (default 4MB). Gamepanel does not appear to enforce this.

8. **Client allocation range** — Pterodactyl allows configuring `PTERODACTYL_CLIENT_ALLOCATIONS_RANGE_START/END`. Gamepanel does not have this client-facing allocation range feature.

9. **Backup throttle** — Pterodactyl enforces backup creation throttle (2 per 10 min). Gamepanel does not appear to throttle backup creation.

10. **Backup prune age** — Pterodactyl auto-fails backups older than `BACKUP_PRUNE_AGE` (default 6h). Gamepanel does not have this.

11. **S3 backup accelerate endpoint** — Pterodactyl supports S3 transfer acceleration. Gamepanel S3 backup config does not include this.

12. **Password reset throttle** — Pterodactyl enforces `auth.throttle` on password reset. Gamepanel has rate limiting but not specifically on password reset.

13. **Login lockout** — Pterodactyl locks out after 3 failed attempts for 2 minutes. Gamepanel uses Redis-based rate limiting (5 attempts/min) — different approach.

14. **Server identifier (short UUID)** — Pterodactyl uses short server identifiers (e.g., `abc12345`). Gamepanel uses full UUIDs.

15. **Egg export/import** — Pterodactyl supports egg sharing via JSON export/import. Gamepanel does not have egg export/import.

16. **Node auto-deploy** — Pterodactyl has `NodeAutoDeployController` for generating auto-deploy tokens. Gamepanel has node registration but no auto-deploy workflow.

17. **Backup remote upload (presigned URL)** — Pterodactyl has `BackupRemoteUploadController` for S3 presigned URL uploads. Gamepanel backup system may not support this.

18. **Activity log actors table** — Pterodactyl has a dedicated `activity_log_actors` table. Gamepanel stores actor info inline.

19. **Server transfer execution** — Pterodactyl has server transfer workflow. Gamepanel has `legacyServerTransferUnavailable` stub returning 501.

20. **Per-server database host management** — Pterodactyl allows managing database hosts per server. Gamepanel has database hosts but may not have the same granularity.

---

## Extra Features in Our Project (not in Pterodactyl)

1. **Cloud Provider Orchestration** — `handlers_cloud.go` + `cloud.Manager` — Integrates with cloud providers (AWS, GCP, etc.) to provision/deprovision nodes automatically.

2. **Auto-scaling** — `handlers_autoscaler.go` + `autoscaler.Service` — Auto-scaling policies that can scale node resources based on demand.

3. **Blue/Green Deployments** — `handlers_deployment.go` + `deployment.Service` — Zero-downtime deployment with health checks and rollback.

4. **Load Balancer** — `handlers_loadbalancer.go` + `loadbalancer.Service` — Target groups and routing for game server traffic.

5. **Failover Policies** — `handlers_failover.go` + `failover.Service` — Automatic failover when nodes go down.

6. **Traffic Manager** — `handlers_trafficmanager.go` + `trafficmanager.Service` — Routing rules for directing traffic across nodes/regions.

7. **Predictive Scheduling** — `handlers_scheduler.go` + `scheduler.PredictiveScorer` + `scheduler.ConstraintScheduler` — ML-based node scoring and affinity rules for optimal server placement.

8. **Regions** — Multi-region support (beyond Pterodactyl's flat "locations") with region-aware node grouping.

9. **Observability Timeline** — `handlers_observability.go` + `observability.Service` — Correlated event timeline across resources.

10. **Heartbeat Monitor** — `heartbeatmonitor.Service` — Tracks node health with expiry detection.

11. **Crash Detection** — `handlers_crashdetect.go` + `crashdetector.Service` — Tracks server crash events with exit codes and OOM detection.

12. **WebAuthn (Passkeys)** — `handlers_webauthn.go` + `webauthn.Service` — FIDO2 WebAuthn support for passwordless authentication.

13. **Social Authentication** — `handlers_social_auth.go` — Discord, Steam, and Authentik OAuth login.

14. **OAuth2 Token Endpoint** — `handlers_oauth2.go` — RFC 6749 `client_credentials` grant for external integrations (PufferPanel parity).

15. **Plugin Marketplace** — `handlers_plugins.go` — Plugin metadata import/query system.

16. **Webhooks** — `store_webhooks.go` + `webhook_dispatcher.go` — Outbound webhook dispatch on resource events.

17. **Roles & Permissions System** — `handlers_roles.go` + `store_roles.go` — Role-based access control beyond Pterodactyl's subuser permissions.

18. **SFTP Global Config** — `handlers_sftp.go` — Admin-configurable SFTP settings per-node and globally.

19. **Rate Limit Configuration** — `handlers_rate_limit_config.go` — Dynamically configurable rate limits per endpoint type.

20. **Mail Notification Triggers** — `store_mail_notification_triggers.go` — Configurable email notification triggers.

21. **Observability Timeline** — Correlated event timeline across resources with filtering.

22. **Evacuation Planner** — `evacuationplanner.Service` — Planned node evacuation for maintenance.

23. **Reservation Manager** — `reservations.Manager` — Resource reservation system.

24. **Recovery Coordinator** — `recovery.Coordinator` + `recovery.TokenService` — Account recovery workflow.

25. **Node Probe** — `nodeprobe.Service` — Active node health probing.

26. **Cluster Manager** — `clustermanager.Service` — Multi-node cluster management.

27. **Migration Service** — `migration.Service` — Server migration across nodes.

28. **Queue System** — `queue.Service` — Async job queue for background tasks.

29. **Runtime Registry** — `runtime.Registry` — Tracks server runtime states.

30. **Config Validator** — `configvalidator.Service` — Validates node/daemon configuration.

31. **Database Provisioner** — `dbprovisioner.Service` — Automated database host provisioning.

32. **Infrastructure** — Grafana dashboards, Prometheus monitoring, alertmanager, nginx config — Pterodactyl has none of these.

33. **SQLite support** — Gamepanel supports SQLite for development/testing.

34. **Multiple database drivers** — PostgreSQL, MySQL, SQLite (Pterodactyl is MySQL/MariaDB only).

35. **OAuth2 clients** — `store_oauth2.go` — OAuth2 client management for API integrations.

36. **WebAuthn** — Passkey/security key support beyond TOTP.

37. **Subuser invitations** — `store_invitations.go` — Invitation-based subuser onboarding.

38. **Mail notification triggers** — Configurable triggers for email notifications.

39. **Backup policies** — Per-server backup policy management.

40. **Job queue** — `057_job_queue.sql` — Async job processing.

41. **Rate limit settings** — `058_rate_limit_settings.sql` — Dynamically configurable rate limits.

42. **Server crash events** — `059_server_crash_events.sql` — Crash event tracking.

---

## Architectural Differences

| Aspect | Pterodactyl | Gamepanel |
|--------|------------|-----------|
| **Language** | PHP 8.x (Laravel) | Go 1.x (Fiber) |
| **Frontend** | React SPA (Webpack) | Next.js 14+ (App Router) |
| **Daemon** | Wings (Go, separate repo) | Beacon (Go, monorepo) |
| **API Style** | RESTful controllers + FormRequests | Fiber handlers with inline route registration |
| **Database** | MySQL/MariaDB only (Eloquent ORM) | PostgreSQL, MySQL, SQLite (raw SQL + pgx) |
| **Auth** | Laravel session + Sanctum tokens | JWT + HttpOnly cookies + CSRF tokens |
| **2FA** | TOTP only | TOTP + WebAuthn (passkeys) |
| **Rate Limiting** | Laravel throttle middleware | Redis-backed per-endpoint rate limiters |
| **Caching** | Laravel cache (Redis/file) | Redis (optional) |
| **Queue** | Laravel queue (database/redis) | Custom in-process schedule runner + queue service |
| **Migrations** | PHP migration classes (Laravel) | Raw SQL files (numbered) |
| **ORM** | Eloquent ORM | Raw SQL + store layer (repository pattern) |
| **Validation** | FormRequest classes | Inline validation in handlers |
| **Testing** | PHPUnit (Unit + Integration) | Go `testing` package (table-driven) |
| **Package Mgmt** | Composer (PHP) + Yarn (JS) | Go modules + npm |
| **Monorepo** | No (panel + wings separate) | Yes (forge + beacon + packages) |
| **API Versioning** | Implicit (no version prefix) | Explicit `/api/v1` prefix |
| **WebSockets** | Laravel WebSockets | Fiber WebSocket middleware |
| **i18n** | Laravel translation files | Go i18n service + JSON files |
| **Infrastructure** | Basic Dockerfile | Full compose + Grafana + Prometheus + Alertmanager |

---

## Missing Features Summary

| # | Feature | Pterodactyl | Gamepanel | Priority |
|---|---------|------------|-----------|----------|
| 1 | ReCAPTCHA | `VerifyReCaptcha` middleware | Not implemented | Medium |
| 2 | External ID lookup | `/users/external/{id}`, `/servers/external/{id}` | Not implemented | Low |
| 3 | Maintenance mode | `MaintenanceMiddleware` | Not implemented | Low |
| 4 | Telemetry | Telemetry service | Not implemented | Low |
| 5 | CDN version check | `cdn.pterodactyl.io` check | Not implemented | Low |
| 6 | Per-schedule task limit | 10 task limit | Not enforced | Low |
| 7 | File max edit size | 4MB limit | Not enforced | Low |
| 8 | Client allocation range | Configurable range | Not implemented | Low |
| 9 | Backup throttle | 2 per 10 min | Not implemented | Medium |
| 10 | Backup prune age | 6h auto-fail | Not implemented | Medium |
| 11 | S3 accelerate endpoint | Configurable | Not in config | Low |
| 12 | Egg export/import | JSON share | Not implemented | Low |
| 13 | Node auto-deploy | Auto-deploy token | Not implemented | Low |
| 14 | Server transfer execution | Full workflow | Stub (501) | **High** |
| 15 | Server short identifiers | `abc12345` format | Full UUID only | Low |

---

## Architectural Differences

### 1. Monorepo vs Separate Repos
Pterodactyl keeps the panel and Wings daemon in separate repositories. Gamepanel uses a monorepo with `forge/api/`, `forge/web/`, and `beacon/` under one roof, plus shared `packages/` for SDK, types, and UI components.

### 2. API Versioning
Pterodactyl does not version its API (routes are at `/api/client`, `/api/application`, `/api/remote`). Gamepanel uses explicit `/api/v1` prefix, allowing future API evolution.

### 3. Route Registration
Pterodactyl uses separate route files per domain (`api-client.php`, `api-application.php`, etc.) with dedicated controller classes. Gamepanel registers all routes inline in `server.go` with handler functions spread across `handlers_*.go` files. This makes Gamepanel's routing harder to navigate but keeps related logic co-located.

### 4. Data Layer
Pterodactyl uses Eloquent ORM with Models, Repositories, and Services layers. Gamepanel uses a raw SQL store layer (`store/`) with repository interfaces and implementations. Gamepanel's approach gives more control over SQL but loses ORM conveniences.

### 5. Auth Architecture
Pterodactyl uses Laravel's session-based auth with Sanctum tokens for API access. Gamepanel uses JWT tokens with HttpOnly cookies, CSRF tokens, and supports multiple auth sources (Bearer, Cookie, Session).

### 6. Daemon Communication
Pterodactyl's Wings communicates via HMAC-signed requests. Gamepanel's Beacon uses bearer tokens with optional HMAC verification (backward-compatible). Gamepanel also adds a heartbeat protocol and node registry.

### 7. Frontend Architecture
Pterodactyl uses a React SPA bootstrapped via Laravel Blade views with Webpack. Gamepanel uses Next.js 14 App Router with server components, client components, and a more modern React architecture.

### 8. Service Layer
Pterodactyl has a dedicated `Services/` directory with service classes per domain. Gamepanel has `services/` with more granular services (33 sub-packages) that are more infrastructure-focused (autoscaler, failover, loadbalancer, etc.).

### 9. Event System
Pterodactyl uses Laravel events + listeners. Gamepanel has a custom event system (`events/`, `eventstore/`) with publishers, subscribers, and a relay for real-time updates.

### 10. Testing Approach
Pterodactyl uses PHPUnit with separate Unit and Integration test directories. Gamepanel uses Go's built-in `testing` package with table-driven tests co-located with source files (`*_test.go`).

---

## Recommendations

1. **Implement server transfer execution** — The `legacyServerTransferUnavailable` stub (returning 501) is the most significant gap. Pterodactyl has a complete transfer workflow that Gamepanel should implement.

2. **Add ReCAPTCHA** — For production security, add captcha verification on login and password reset endpoints.

3. **Add backup throttling and prune age** — These are important operational safeguards that Pterodactyl provides.

4. **Consider egg export/import** — Useful for sharing game server templates between installations.

5. **Add external ID lookup endpoints** — Low effort, useful for external system integration.

6. **Document the API route structure** — Gamepanel's inline route registration in `server.go` (1300+ lines) is harder to navigate than Pterodactyl's separate route files. Consider extracting route groups into separate files.

7. **Add API documentation** — Pterodactyl has no formal API docs. Gamepanel has a `swagger.go` file — ensure it is complete and up-to-date.

8. **Consider removing or implementing the `pterodactyl/` empty component directory** — `forge/web/components/pterodactyl/` is empty and should be cleaned up.

9. **Add maintenance mode** — Useful for production deployments during updates.

10. **Review the 64 SQL migration files** against Pterodactyl's 195 migrations to ensure no schema features are missing (particularly around indexes, foreign keys, and constraints).
