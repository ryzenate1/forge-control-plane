# MASTER CONSOLIDATED AUDIT REPORT

**Generated:** 2026-07-16
**Sources:** 4 audit reports
- `comparison_petrodactyl.md` (461 lines) — Pterodactyl Panel
- `comparison_pelican.md` (514 lines) — Pelican Panel
- `comparison_puffer.md` (351 lines) — Puffer Panel
- `wings-vs-beacon-audit.md` (359 lines) — Wings (daemon)

---

## 1. Executive Summary

Gamepanel is a **ground-up reimplementation** of the game server panel concept, not a fork of any single project. It draws influence from Pterodactyl (API contract, server lifecycle), Pelican (plugin system), Puffer Panel (OAuth2, Go stack), and Wings (daemon architecture), but introduces substantial new infrastructure absent in all references.

### Stack Comparison

| Layer | Pterodactyl | Pelican | Puffer Panel | Our Project |
|-------|------------|--------|-------------|-------------|
| **Backend** | PHP 8 (Laravel) | PHP 8.2 (Laravel 11) | Go (Gin) | Go (Fiber) |
| **Frontend** | React SPA (Webpack) | Filament (Blade/Livewire) | Embedded SPA (Go binary) | Next.js 14 (App Router) |
| **Daemon** | Wings (Go, separate) | Wings (Go, separate) | Monolithic (same binary) | Beacon (Go, monorepo) |
| **Database** | MySQL/MariaDB | SQLite/MySQL/PostgreSQL | GORM auto-migrate | PostgreSQL (+ MySQL/SQLite) |
| **Container Runtime** | Docker | Docker | Docker + TTY | Docker, containerd, Podman, Firecracker, K8s |

### Key Numbers

| Metric | Pterodactyl | Pelican | Puffer Panel | Our Project |
|--------|------------|--------|-------------|-------------|
| Backend source files | ~195 migrations, ~36 models | ~100+ migrations | ~26 models, 13 services | ~60 migrations, 82 store files |
| Test files | PHPUnit tests | ~79 tests | Minimal | ~150 Go tests + ~50 beacon tests |
| API endpoints (client) | ~40 | ~40 | ~20 | ~45 (with extras) |
| API endpoints (admin) | ~30 | ~35 | ~15 | ~35 |
| Missing features vs this ref | 15 | 14 (actually 7 upon verification) | 16 | N/A |

### Overall Finding

Gamepanel is **ahead of all references** in cloud-native infrastructure (multi-region, autoscaling, load balancing, failover, predictive scheduling, observability, encrypted secrets). It is **behind** in:
- **Installation automation** (Puffer's 24 operation types, Minecraft-specific downloaders)
- **Console connectivity** (RCON, Telnet, Minecraft Query — Puffer only)
- **Conditional logic** (Puffer's CEL-based conditions)
- **Email notifications** (Pelican has more notification templates)
- **i18n completeness** (8 locales vs Pelican's 20+)
- **Extension framework** (Pelican's formal Extensions directory)

---

## 2. Cross-Reference Verification

### 2.1 Contradictions Found and Resolved

The Pelican agent incorrectly flagged **7 items** as missing that actually exist in Gamepanel. These are verification errors in the Pelican audit, confirmed via source code inspection:

| # | Claimed Missing (Pelican) | Actual Status | Evidence |
|---|---|---|---|
| 1 | SSH Key Management | ✅ EXISTS | `handlers_auth.go:194-238` — `/ssh-keys` GET/POST/DELETE + `store_sshkeys.go` |
| 2 | File Pull from URL | ✅ EXISTS | `handlers_servers.go:2545` — `POST /servers/:id/files/pull` |
| 3 | Database Password Rotation | ✅ EXISTS | `handlers_servers.go:1400` — `POST /servers/:id/databases/:databaseId/rotate-password` |
| 4 | Node Configuration Endpoint | ✅ EXISTS | `handlers_admin.go:108` — `GET /nodes/:id/configuration` |
| 5 | Deployable Nodes | ✅ EXISTS | `handlers_admin.go:204` — `POST /nodes/deployable` |
| 6 | Backup Rename | ✅ EXISTS | `handlers_servers.go:1842` — `POST /servers/:id/backups/:name/rename` |
| 7 | Server Transfer Cancel | ✅ EXISTS | `handlers_servers.go:637` — `POST /servers/:id/transfer/cancel` |

### 2.2 Minor Discrepancies

| Item | Pterodactyl Report | Pelican Report | Resolution |
|------|-------------------|----------------|------------|
| File delete verb | `POST /files/delete` | `DELETE /files` | Actual: `DELETE /servers/:id/files/delete` — Pterodactyl wrong verb, Pelican wrong path |
| Server transfer | Stub (501) | `POST /transfer` ✅ | Both correct — old pterodactyl transfer stub exists; new transfer endpoint also exists |

### 2.3 Consistent Findings (All Reference Projects)

These items are consistently confirmed as genuinely missing:

| Missing Feature | Pterodactyl | Pelican | Puffer | Consensus |
|----------------|:-----------:|:-------:|:------:|:---------:|
| External ID lookups | ✅ Miss | ✅ Miss | N/A | **Confirmed missing** |
| Maintenance mode | ✅ Miss | ✅ Miss | N/A | **Confirmed missing** |
| Egg/template export | ✅ Miss | ✅ Miss | N/A | **Confirmed missing** |
| Email notification variety | ✅ Less | ✅ Miss | ✅ Less | **Confirmed gap** |
| i18n completeness | ✅ Less | ✅ Less | N/A | **Confirmed gap** |
| ReCAPTCHA | ✅ Miss | N/A | N/A | **Confirmed missing** |

### 2.4 Overlapping Strengths (All Reports)

All 4 reports independently confirm Gamepanel's unique features: multi-region, autoscaling, load balancing, failover, WebAuthn, social auth, observability, webhooks, plugin system, encrypted secrets, predictive scheduling.

---

## 3. Consolidated Feature Matrix

| # | Feature | Our Project | Pterodactyl | Pelican | Puffer Panel | Wings |
|---|---------|:-----------:|:-----------:|:-------:|:------------:|:-----:|
| | **Server Management** | | | | | |
| 1 | Server CRUD (create/read/update/delete) | ✅ | ✅ | ✅ | ✅ | — |
| 2 | Server suspend/unsuspend | ✅ | ✅ | ✅ | ✅ | — |
| 3 | Server reinstall | ✅ | ✅ | ✅ | — | — |
| 4 | Server transfer between nodes | ✅ | ✅ (transfer) | ✅ | ❌ | — |
| 5 | Server transfer cancel | ✅ | ❌ | ✅ | ❌ | — |
| 6 | Server build config (CPU/RAM/disk) | ✅ | ✅ | ✅ | ✅ | — |
| 7 | Server startup management | ✅ | ✅ | ✅ | ✅ | — |
| 8 | Server power actions (start/stop/restart/kill) | ✅ | ✅ | ✅ | ✅ | ✅ |
| 9 | Server identifier (short UUID / full UUID) | Full UUID | Short UUID | Full UUID | N/A | — |
| 10 | Predictive server placement | ✅ | ❌ | ❌ | ❌ | — |
| | **API & Endpoints** | | | | | |
| 11 | Client API (user-facing) | `/api/v1` | `/api/client` | `/api/client` | Custom | — |
| 12 | Application API (admin) | `/api/v1` | `/api/application` | `/api/application` | Custom | — |
| 13 | Remote API (daemon communication) | `/api/remote` | `/api/remote` | `/api/remote` | N/A (monolith) | ✅ |
| 14 | API versioning | Explicit v1 | None | None | None | — |
| 15 | External ID lookup (users/servers) | ❌ | ✅ | ✅ | ❌ | — |
| 16 | OAuth2 token endpoint (client_credentials) | ✅ | ❌ | ❌ | ✅ | — |
| 17 | Webhook dispatch system | ✅ | ❌ | ✅ | ❌ | — |
| 18 | Swagger/OpenAPI docs | ✅ (swagger.go) | ❌ | ❌ | ✅ | — |
| | **Authentication** | | | | | |
| 19 | Email/password login | ✅ | ✅ | ✅ | ✅ | — |
| 20 | TOTP 2FA | ✅ | ✅ | ✅ | ❌ | — |
| 21 | WebAuthn/Passkeys | ✅ | ❌ | ❌ | ❌ | — |
| 22 | Social auth (Discord, Steam, Authentik) | ✅ | ❌ | ✅ (stubs) | ❌ | — |
| 23 | SSH key management | ✅ | ✅ | ❌ (error in audit) | ✅ | — |
| 24 | API token auth (JWT/Bearer) | ✅ | ✅ (Sanctum) | ✅ (Sanctum) | ✅ | ✅ |
| 25 | Password reset flow | ✅ | ✅ | ✅ | ✅ | — |
| 26 | Account recovery | ✅ | ❌ | ❌ | ❌ | — |
| 27 | Rate limiting (per-endpoint, configurable) | ✅ | ✅ (basic) | ✅ | ❌ | ✅ |
| 28 | IP access control | ✅ | ❌ | ✅ | ❌ | — |
| | **Container Runtimes** | | | | | |
| 29 | Docker runtime | ✅ | ✅ | ✅ | ✅ | ✅ |
| 30 | containerd runtime | ✅ | ❌ | ❌ | ❌ | ❌ |
| 31 | Podman runtime | ✅ | ❌ | ❌ | ❌ | ❌ |
| 32 | Firecracker microVM runtime | ✅ | ❌ | ❌ | ❌ | ❌ |
| 33 | Kubernetes runtime | ✅ | ❌ | ❌ | ❌ | ❌ |
| 34 | Local TTY process runtime | ❌ | ❌ | ❌ | ✅ | ❌ |
| | **Backup Systems** | | | | | |
| 35 | Backup create/restore/delete | ✅ | ✅ | ✅ | ✅ | ✅ |
| 36 | S3 backup storage | ✅ | ✅ | ✅ | ❌ | ✅ |
| 37 | Backup locking | ✅ | ✅ | ✅ | ❌ | — |
| 38 | Backup rename | ✅ | ❌ | ✅ | ❌ | — |
| 39 | Backup throttling (rate limit) | ❌ | ✅ | ✅ | ❌ | — |
| 40 | Backup prune age (auto-fail stale) | ❌ | ✅ | ✅ | ❌ | — |
| 41 | Backup checksum verification | ✅ | ❌ | ❌ | ❌ | ✅ |
| | **Database Features** | | | | | |
| 42 | Server database provisioning | ✅ | ✅ | ✅ | ❌ | — |
| 43 | Database host CRUD | ✅ | ✅ | ✅ | ❌ | — |
| 44 | Database password rotation | ✅ | ✅ | ❌ (error) | ❌ | — |
| 45 | Database TLS support | ✅ | ✅ | ✅ | ❌ | — |
| | **File Management** | | | | | |
| 46 | File list/read/write/delete | ✅ | ✅ | ✅ | ✅ | — |
| 47 | File upload/download | ✅ | ✅ | ✅ | ✅ | — |
| 48 | File compress/decompress | ✅ | ✅ | ✅ | ✅ | ✅ |
| 49 | File copy/rename/chmod | ✅ | ✅ | ✅ | — | — |
| 50 | File pull from URL | ✅ | ✅ | ❌ (error) | ❌ | — |
| 51 | SFTP server | ✅ | ✅ | ✅ | ✅ | ✅ |
| 52 | File max edit size enforcement | ❌ | ✅ | — | — | — |
| | **Scheduling** | | | | | |
| 53 | CRON schedule management | ✅ | ✅ | ✅ | ✅ | — |
| 54 | Schedule tasks (commands) | ✅ | ✅ | ✅ | ✅ | — |
| 55 | Per-schedule task limit | ❌ | ✅ | — | — | — |
| | **Subusers & Permissions** | | | | | |
| 56 | Subuser CRUD | ✅ | ✅ | ✅ | ❌ | — |
| 57 | Granular subuser permissions | ✅ | ✅ | ✅ | ✅ | — |
| 58 | Role-based access control (RBAC) | ✅ | ❌ | ✅ | ✅ | — |
| 59 | Subuser invitations | ✅ | ❌ | — | — | — |
| | **Admin Features** | | | | | |
| 60 | Node CRUD | ✅ | ✅ | ✅ | ✅ | — |
| 61 | Allocation management | ✅ | ✅ | ✅ | ✅ | — |
| 62 | Location/Region management | ✅ (regions) | ✅ (locations) | ✅ (locations) | ❌ | — |
| 63 | Multi-region clustering | ✅ | ❌ | ❌ | ❌ | — |
| 64 | Nest/Egg (template) management | ✅ (templates) | ✅ (eggs) | ✅ (eggs) | ✅ (templates) | — |
| 65 | Egg/template export/import | ❌ (no export) | ✅ | ✅ | — | — |
| 66 | Plugin system | ✅ | ❌ | ✅ | ❌ | — |
| 67 | Plugin marketplace | ✅ (metadata) | ❌ | ✅ | ❌ | — |
| 68 | ReCAPTCHA | ❌ | ✅ | ❌ | ❌ | — |
| 69 | Maintenance mode | ❌ | ✅ | ✅ | ❌ | — |
| 70 | Telemetry | ❌ | ✅ | — | — | — |
| | **Monitoring & Observability** | | | | | |
| 71 | Activity/audit logging | ✅ | ✅ | ✅ | ❌ | ✅ |
| 72 | Observability event timeline | ✅ | ❌ | ❌ | ❌ | — |
| 73 | Health check endpoints | ✅ | ❌ | ✅ (OhDear) | ✅ | ✅ |
| 74 | Grafana dashboards | ✅ | ❌ | ❌ | ❌ | — |
| 75 | Prometheus metrics | ✅ | ❌ | ❌ | ❌ | ✅ |
| 76 | Crash detection & tracking | ✅ | ❌ | ❌ | ✅ | ✅ |
| 77 | Heartbeat monitoring | ✅ | ❌ | ❌ | ❌ | — |
| | **Daemon Features** | | | | | |
| 78 | HMAC auth for panel↔daemon | ✅ | ✅ | ✅ | N/A (monolith) | ✅ |
| 79 | WebSocket streaming (console/stats) | ✅ | ✅ | ✅ | ✅ | ✅ |
| 80 | Console throttle | ✅ | ✅ | — | — | ✅ |
| 81 | Disk quota enforcement | ✅ | ✅ | — | — | ✅ |
| 82 | S3 backup in daemon | ✅ | ✅ | — | — | ✅ |
| 83 | Server transfer protocol | ✅ | ✅ | — | ❌ | ✅ |
| 84 | TLS/autocert support | ✅ | ❌ | — | ❌ | ✅ |
| 85 | Rate limiting (daemon) | ✅ | ❌ | — | ❌ | ✅ |
| 86 | Pprof profiling | ✅ | ❌ | — | ❌ | — |
| 87 | Log rotation (daemon) | ✅ | ❌ | — | ✅ | ✅ |
| | **Infrastructure** | | | | | |
| 88 | Multiple database drivers (PG/MySQL/SQLite) | ✅ | ❌ (MySQL only) | ✅ | — | — |
| 89 | Docker Compose deployment | ✅ | ✅ | ✅ | ✅ | — |
| 90 | Cloud provider integration | ✅ | ❌ | ❌ | ❌ | — |
| 91 | Auto-scaling | ✅ | ❌ | ❌ | ❌ | — |
| 92 | Load balancing | ✅ | ❌ | ❌ | ❌ | — |
| 93 | Failover clusters | ✅ | ❌ | ❌ | ❌ | — |
| 94 | Traffic manager | ✅ | ❌ | ❌ | ❌ | — |
| 95 | Blue/green deployments | ✅ | ❌ | ❌ | ❌ | — |
| 96 | Encrypted secrets at rest | ✅ | ❌ | ❌ | ❌ | — |
| 97 | Job queue (async processing) | ✅ | ❌ | ✅ | ❌ | — |
| 98 | i18n (language support) | ✅ (8) | ❌ | ✅ (20+) | ❌ | — |
| 99 | RCON/Telnet console proxy | ❌ | ❌ | ❌ | ✅ | — |
| 100 | Minecraft query protocol | ❌ | ❌ | ❌ | ✅ | — |
| 101 | Operation system (forge/paper/steam downloads) | ❌ | ❌ | ❌ | ✅ (24 ops) | — |
| 102 | CEL-based condition engine | ❌ | ❌ | ❌ | ✅ | — |

---

## 4. Consolidated Gaps

### 4.1 HIGH Priority

| # | Gap | Affected Reference | Description |
|---|-----|:------------------:|-------------|
| 1 | **Server transfer execution** | Pterodactyl | Legacy pterodactyl transfer stub returns 501. New transfer endpoint exists but old compatibility is broken. |
| 2 | **ReCAPTCHA / Captcha** | Pterodactyl, Pelican | Login and password-reset endpoints lack captcha verification. Pterodactyl has `VerifyReCaptcha` middleware. |
| 3 | **Backup throttling & prune age** | Pterodactyl, Pelican | No rate limiting on backup creation (2 per 10 min in Pterodactyl). No auto-fail for stale backups (6h in Pterodactyl). |
| 4 | **Operation system (game-specific installers)** | Puffer Panel | Puffer has 24 built-in operations (forge, paper, fabric, steam, curseforge, etc.). Gamepanel delegates entirely to daemon with no game-specific installers. |
| 5 | **Email notification templates** | Pelican | Missing email notifications for account creation, subuser events, install completion, backup completion. |
| 6 | **External ID lookup endpoints** | Pterodactyl, Pelican | No `GET /users/external/{id}` or `GET /servers/external/{id}`. Needed for external system integration. |

### 4.2 MEDIUM Priority

| # | Gap | Affected Reference | Description |
|---|-----|:------------------:|-------------|
| 7 | **Maintenance mode** | Pterodactyl, Pelican | No middleware to put panel in maintenance mode during updates. |
| 8 | **CEL-based condition engine** | Puffer Panel | Puffer uses Google's CEL for conditional operation execution. Gamepanel has no equivalent, making install scripts less flexible. |
| 9 | **RCON/Telnet console proxying** | Puffer Panel | Some games don't support WebSocket stdin. Puffer provides RCON, Telnet, RCONWS alternatives. |
| 10 | **Minecraft query protocol** | Puffer Panel | Built-in Minecraft server query for status display without WebSocket. |
| 11 | **Egg/template export** | Pterodactyl, Pelican | Cannot export game server templates as JSON for sharing between installations. |
| 12 | **Telemetry** | Pterodactyl | No anonymous usage reporting. Low priority but useful for the project maintainers. |
| 13 | **Node auto-deploy token** | Pterodactyl | No auto-deploy workflow for nodes. Pterodactyl generates auto-deploy tokens. |
| 14 | **Crash limit configuration** | Puffer Panel | Crash detection exists but limit is not per-server configurable like Puffer's `CrashLimit`. |
| 15 | **File max edit size enforcement** | Pterodactyl | No limit on file edit size (Pterodactyl defaults to 4MB). |
| 16 | **Per-schedule task limit** | Pterodactyl | No limit on tasks per schedule (Pterodactyl defaults to 10). |

### 4.3 LOW Priority

| # | Gap | Affected Reference | Description |
|---|-----|:------------------:|-------------|
| 17 | **i18n expansion** | Pelican | 8 languages vs Pelican's 20+. Need more locales and translations. |
| 18 | **Health check notification channels** | Pelican | No Slack/mail/OhDear integration for health check alerts. |
| 19 | **CLI user management** | Puffer Panel | No command-line user creation for headless setup. |
| 20 | **CLI database management** | Puffer Panel | Migration run/status commands would help operations. |
| 21 | **Keep-alive system** | Puffer Panel | No keep-alive for servers needing periodic input. |
| 22 | **TTY/local process runtime** | Puffer Panel | Docker-only runtime. No local process execution. |
| 23 | **Console forwarding to stdout** | Puffer Panel | No optional console-to-stdout for debugging. |
| 24 | **OS permission group checks** | Puffer Panel | No OS-level group membership check for CLI access. |
| 25 | **Activity admin hiding** | Pelican | Cannot hide admin activity from client API responses. |
| 26 | **Client allocation range config** | Pterodactyl | Cannot restrict client allocation port ranges. |
| 27 | **S3 accelerate endpoint** | Pterodactyl | No S3 transfer acceleration support in backup config. |
| 28 | **Single-binary deployment mode** | Puffer Panel | Forge+Beacon as single binary for small deployments. |
| 29 | **Multiple email providers** | Puffer Panel | Only SMTP; no Mailgun/SendGrid/Mailjet providers. |
| 30 | **Extension framework** | Pelican | No formal extension system like Pelican's `app/Extensions/`. |

---

## 5. Consolidated Strengths

Features Gamepanel has that NONE of the reference projects have:

| # | Feature | Pterodactyl | Pelican | Puffer | Wings |
|---|---------|:-----------:|:-------:|:------:|:-----:|
| 1 | **Multi-container runtime** (Docker + containerd + Podman + Firecracker + K8s) | ❌ | ❌ | ❌ | ❌ |
| 2 | **Multi-region clustering** | ❌ | ❌ | ❌ | ❌ |
| 3 | **Placement engine** (constraints + scoring + explainability) | ❌ | ❌ | ❌ | ❌ |
| 4 | **Predictive scheduling** (ML-based scoring) | ❌ | ❌ | ❌ | ❌ |
| 5 | **Cloud provider integration** (AWS, multi-cloud) | ❌ | ❌ | ❌ | ❌ |
| 6 | **Auto-scaling** (policy-based node scaling) | ❌ | ❌ | ❌ | ❌ |
| 7 | **Load balancer** (target groups, routing) | ❌ | ❌ | ❌ | ❌ |
| 8 | **Failover clusters** (automatic detection + redistribution) | ❌ | ❌ | ❌ | ❌ |
| 9 | **Traffic manager** (routing rules) | ❌ | ❌ | ❌ | ❌ |
| 10 | **Blue/green deployments** (zero-downtime) | ❌ | ❌ | ❌ | ❌ |
| 11 | **Evacuation planner** (maintenance draining) | ❌ | ❌ | ❌ | ❌ |
| 12 | **Reservation system** (resource reservations) | ❌ | ❌ | ❌ | ❌ |
| 13 | **Recovery coordinator** (account/node recovery) | ❌ | ❌ | ❌ | ❌ |
| 14 | **WebAuthn passwordless auth** | ❌ | ❌ | ❌ | ❌ |
| 15 | **Social authentication** (Discord, Steam, Authentik) | ❌ | ✅ (stubs) | ❌ | ❌ |
| 16 | **OAuth2 RFC 6749 support** (client_credentials) | ❌ | ❌ | ✅ | ❌ |
| 17 | **Observability event timeline** | ❌ | ❌ | ❌ | ❌ |
| 18 | **Grafana + Prometheus infrastructure** | ❌ | ❌ | ❌ | ❌ |
| 19 | **Encrypted secrets at rest** (keyring-based) | ❌ | ❌ | ❌ | ❌ |
| 20 | **Job queue** (PostgreSQL-backed async processing) | ❌ | ✅ | ❌ | ❌ |
| 21 | **Configurable rate limits via API** | ❌ | ❌ | ❌ | ❌ |
| 22 | **IP access control whitelist/blacklist** | ❌ | ❌ | ❌ | ❌ |
| 23 | **Plugin marketplace** (metadata import/query) | ❌ | ❌ | ❌ | ❌ |
| 24 | **Webhook dispatch** (outbound event-driven webhooks) | ❌ | ✅ | ❌ | ❌ |
| 25 | **Subuser invitations** | ❌ | ❌ | ❌ | ❌ |
| 26 | **Database provisioner** (automated DB provisioning) | ❌ | ❌ | ❌ | ❌ |
| 27 | **Crash event tracking** (DB-persisted crash events) | ❌ | ❌ | ❌ | ❌ |
| 28 | **Node heartbeat + probe** | ❌ | ❌ | ❌ | ❌ |
| 29 | **Shared TypeScript SDK + types + UI components** | ❌ | ❌ | ❌ | ❌ |
| 30 | **SQLite support** (dev/testing) | ❌ | ✅ | ❌ | ❌ |
| 31 | **Multiple database drivers** (PG + MySQL + SQLite) | ❌ (MySQL only) | ✅ | ❌ | ❌ |
| 32 | **Next.js modern frontend** (App Router, SSR, RSC) | ❌ | ❌ | ❌ | ❌ |

---

## 6. Audit Quality Assessment

### 6.1 Did Each Agent Actually Examine File Contents?

| Agent | File Contents Evidence | Verdict |
|-------|----------------------|---------|
| **Pterodactyl** | References 42+ specific Go handler files (`handlers_cloud.go`, `handlers_autoscaler.go`, etc.), specific store files (`store_webhooks.go`, `store_roles.go`), specific migrations (`057_job_queue.sql`, `059_server_crash_events.sql`), test file patterns (`*_test.go`). | ✅ **Excellent** — Deep file-level analysis |
| **Pelican** | References specific Pelican PHP files and Gamepanel Go equivalents. Notes file counts (57 React components, 58 SQL migrations). Maps directory structures. | ✅ **Good** — Surface-level mapping, API-focused |
| **Puffer** | References specific Puffer packages (`operations/`, `conditions/`, `servers/tty/`, `connections/`), specific Gamepanel packages (`runtime/`, `placement/`, `migration.Service`). Detailed architectural comparison. | ✅ **Very Good** — Package-level analysis |
| **Wings/Beacon** | Shows actual Go interface definitions (`Runtime` interface), specific struct fields (`ServerState`), counts test files per package, lists dependency changes (removed Gin, GORM, gRPC; added Viper, YAML). | ✅ **Excellent** — Deepest code-level analysis |

**All 4 agents examined actual file contents.** None relied solely on filenames.

### 6.2 Blind Spots & Errors

| Issue | Affected Agent | Impact |
|-------|---------------|--------|
| **7 false missing flags** — SSH keys, file pull, DB password rotation, node config, deployable nodes, backup rename, transfer cancel all falsely flagged as missing | Pelican | **HIGH** — 7 out of 14 claimed missing features (50%) were verification errors. The Pelican audit's "missing" list is unreliable. |
| **No performance/scalability testing** | All | **MEDIUM** — No agent benchmarked or profiled Gamepanel vs references |
| **No deep security audit** | All | **HIGH** — No XSS, SQLi, auth bypass, CSRF, or session fixation testing |
| **No frontend code analysis** | All | **MEDIUM** — No audit of React component quality, accessibility, bundle size, or Next.js patterns |
| **No database schema completeness check** | All | **MEDIUM** — No verification that all relationships, indexes, and constraints from references exist |
| **No CI/CD pipeline analysis** | All | **LOW** — Build/deploy pipeline not compared |
| **No error handling consistency review** | All (mentioned briefly by Pelican) | **LOW** — Exception hierarchy vs inline errors |
| **No WebSocket/realtime infrastructure deep dive** | All (partially in Wings) | **LOW** — Event relay, outbox pattern, pub/sub not deeply compared |

### 6.3 What Needs Deeper Analysis

1. **Security audit** — Penetration testing, auth bypass checks, token validation review
2. **Database schema migration comparison** — Map each of the 60 migrations against Pterodactyl's 195 to find missing tables/indexes/constraints
3. **Frontend audit** — Accessibility (a11y), bundle size, client/server component split, SEO, error boundaries
4. **Performance benchmark** — API response times, daemon startup time, concurrent server management capacity
5. **Configuration validation audit** — All `FORGE_*` env vars documented? All config options validated at startup?
6. **Error handling audit** — Are all API errors properly typed and returned? Panic recovery coverage?

---

## 7. Recommendations

### Top 10 Priority Actions (All Audits Combined)

| # | Action | Priority | Source | Effort | Impact |
|---|--------|----------|--------|--------|--------|
| 1 | **Implement game-specific operation system** — Port Puffer's 24 install operations (forge, paper, fabric, steam, curseforge downloads) to Beacon's installer | **HIGH** | Puffer | Large | Critical for game panel credibility |
| 2 | **Add ReCAPTCHA/Turnstile** to login and password reset endpoints | **HIGH** | Pterodactyl | Small | Security hardening for production |
| 3 | **Add backup throttling and prune age** — Implement rate limiting (2 per 10 min) and auto-fail for stale backups (6h) | **HIGH** | Pterodactyl, Pelican | Medium | Operational safeguard |
| 4 | **Fix server transfer stub** — Replace `legacyServerTransferUnavailable` (501) with working implementation for Pterodactyl-compatible transfers | **HIGH** | Pterodactyl | Medium | Legacy compatibility |
| 5 | **Add email notification templates** — Account creation, subuser events, install complete, backup complete | **HIGH** | Pelican | Medium | User experience |
| 6 | **Add CEL-based condition engine** for install scripts and server variables | **MEDIUM** | Puffer | Large | Template flexibility |
| 7 | **Implement RCON/Telnet console proxying** in Beacon for games without WebSocket support | **MEDIUM** | Puffer | Large | Game compatibility |
| 8 | **Add external ID lookup endpoints** — `/users/external/{id}`, `/servers/external/{id}` | **MEDIUM** | Pterodactyl, Pelican | Small | API integration |
| 9 | **Implement egg/template export** — JSON export for game server templates | **MEDIUM** | Pterodactyl, Pelican | Small | Template sharing |
| 10 | **Add maintenance mode middleware** for production deployments | **MEDIUM** | Pterodactyl, Pelican | Small | Operations |

### Architectural Recommendations

- **Extract route groups from `server.go`** — The 1300+ line file is harder to navigate than Pterodactyl's separate route files
- **Consider formal extension framework** — Pelican's `app/Extensions/` provides clean separation for pluggable backends
- **Add exponential backoff for crash loop detection** — Currently uses fixed cooldown (Wings audit finding)
- **Expand i18n to 20+ locales** — Pelican has more comprehensive localization
- **Document config merge precedence** — Defaults < YAML < env vars < CLI flags (Wings audit finding)
- **Review all 60 SQL migrations** against Pterodactyl's 195 to ensure no missing indexes, foreign keys, or constraints
- **Clean up empty directories** — `forge/web/components/pterodactyl/` is empty

---

*Report generated by AUDIT COLLECTOR/VERIFIER. Verification methodology: cross-referenced all 4 audit reports against each other and against actual source code where contradictions were found.*
