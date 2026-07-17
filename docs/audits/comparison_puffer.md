# Puffer Panel vs Gamepanel — Comprehensive Audit

**Date:** 2026-07-16  
**Reference:** `/Users/riyaz/project/gamepanel/reference/pufferpanel/`  
**Target:** `/Users/riyaz/project/gamepanel`  

---

## Executive Summary

Gamepanel and Puffer Panel are both open-source game server management panels written in Go, but they diverge significantly in architecture, scope, and maturity.

| Dimension | Puffer Panel | Gamepanel |
|-----------|-------------|-----------|
| **Architecture** | Monolithic (panel + daemon in one binary) | Split (forge = panel API, beacon = daemon) |
| **HTTP Framework** | Gin | Fiber |
| **Frontend** | Embedded SPA in Go binary (`client/frontend/`) | Next.js app router (`forge/web/`) |
| **Database** | GORM auto-migrate + manual upgrade | GORM + 60+ sequential SQL migrations |
| **Container Runtime** | Docker + TTY (local process) | Docker, containerd, Podman, Firecracker, Kubernetes |
| **Auth** | OAuth2 tokens + session cookies | JWT + WebAuthn + OAuth2 |
| **Events** | WebSocket trackers | Event store + outbox pattern + pub/sub |
| **File serving** | Embedded `fs.ReadFileFS` merged with filesystem | Next.js static + API proxy |
| **i18n** | None (English only UI) | 8 languages (`lang/`) + i18n middleware |

Puffer Panel is a more mature, self-contained product with built-in operations for downloading Minecraft/Forge/Paper/Sponge servers, cron scheduling, TTY process management, RCON/Telnet console proxying, and a rich template/variable system. Gamepanel targets a more cloud-native architecture with multi-region clustering, container orchestration across multiple runtimes, autoscaling, load balancing, failover, live server migration, and encrypted secrets management.

---

## Directory / File Structure Comparison

### Top-Level Structure

| Puffer Panel | Gamepanel (Our Project) | Notes |
|---|---|---|
| `server.go`, `engine.go` | — | Puffer's core server definition & Gin engine singleton |
| `environment.go` | `beacon/internal/server/` | Puffer's environment (process management) lives in the monolith; Gamepanel splits this to beacon |
| `cmd/` (8 files) | `forge/api/cmd/api/` (2 files) | Puffer: cobra CLI (run, version, user, db). Gamepanel: single binary entry point |
| `models/` (26 files) | `forge/api/internal/models/` (7 files) + `forge/api/internal/domain/` | Puffer: GORM models for all entities. Gamepanel: thin models + rich domain types |
| `services/` (13 files) | `forge/api/internal/services/` (33 dirs) | Puffer: flat service files. Gamepanel: heavily modularized per concern |
| `database/` (2 files) | `forge/api/migrations/` (60+) + `forge/api/internal/store/` (82 files) | Puffer: GORM auto-migration. Gamepanel: versioned SQL + repository layer |
| `middleware/` (5 files) | `forge/api/internal/http/middleware_*.go` (8 files) | Similar concepts, different implementation |
| `web/` (7 subdirs) | `forge/web/` (Next.js) | Puffer: Go-based route registration + embedded frontend. Gamepanel: separate Next.js app |
| `web/api/` (9 files) | `forge/api/internal/http/handlers_*.go` (70 files) | Puffer: compact API handlers. Gamepanel: many specialized handler files |
| `servers/` (9 files + docker/ + tty/) | `beacon/internal/server/` (18 files) + `forge/api/internal/orchestrator/` | Puffer: servers live in monolith. Gamepanel: split between forge (orchestration) and beacon (execution) |
| `operations/` (24 subdirs) | — | Puffer: rich operation system (download, forge installer, etc.). Gamepanel: delegates to beacon runtime |
| `scopes/` (2 files) | `forge/api/internal/auth/scopes.go` | Puffer: 40+ scopes. Gamepanel: 14 scopes |
| `oauth2/` (3 files) | `forge/api/internal/http/handlers_oauth2.go` + store files | Both support OAuth2 |
| `sftp/` (2 files) | `beacon/internal/sftpserver/` (2 files) | Puffer: SFTP in monolith. Gamepanel: SFTP split to beacon |
| `files/` (5 files) | — | Puffer: file server abstraction (compression, merged FS). Gamepanel: handled in beacon |
| `email/` (6 files) | `forge/api/internal/services/mail/` | Puffer: 5 providers (debug, mailgun, mailjet, sendgrid, smtp). Gamepanel: built-in mail worker |
| `config/` (3 files) | `forge/api/config/` (6 files) + `forge/api/internal/config/` | Both use config packages |
| `logging/` (3 files) | `forge/api/internal/services/logger/` | Puffer: logger + rotator. Gamepanel: structured slog-based |
| `connections/` (3 files) | — | Puffer: RCON, RCONWS, Telnet. Gamepanel: not present |
| `conditions/` (4 files) | — | Puffer: CEL-based condition engine. Gamepanel: not present |
| `groups/` (2 files) | — | Puffer: OS permission groups. Gamepanel: not present |
| `utils/` (15 files) | — | Puffer: kernel, JVM, arguments, mappings, wildcard. Gamepanel: not centralized |
| `client/` (4 entries) | `packages/sdk/`, `packages/shared-types/`, `packages/ui/` | Puffer: embedded frontend. Gamepanel: separate packages workspace |
| `systemd/` | `infra/` | Puffer: systemd service files. Gamepanel: infra (CI, Grafana, Prometheus) |
| `query/` (1 file) | — | Puffer: Minecraft server query protocol. Gamepanel: not present |
| `assets/` | — | Puffer: static assets. Gamepanel: not present |
| `download.go`, `cache.go`, `console.go`, `tracker.go`, `message.go`, `version.go` | `forge/api/internal/version/`, `forge/api/internal/events/`, etc. | Puffer: top-level utility files. Gamepanel: distributed across internal packages |
| `requirements.go`, `variable.go`, `task.go`, `operation.go` | — | Puffer: server definition types. Gamepanel: equivalent in migration SQL + store types |
| `authorization.go` | — | Puffer: SFTP auth interface. Gamepanel: embedded in beacon |

---

## Feature Comparison

| Feature | Puffer Panel | Gamepanel | Notes |
|---|---|---|---|
| **Server Types** | Docker + TTY (local) | Docker, containerd, Podman, Firecracker, K8s | Gamepanel has far more runtime options |
| **Server Installation** | 24 built-in operations (forge, paper, fabric, steam, etc.) | Delegates to daemon runtime | Puffer has richer installation templates |
| **Conditions/Logic** | CEL-based `if` conditions on operations | Not present | Puffer's conditional execution is powerful |
| **Variables** | Typed variables (string, int, bool) with user-edit flag | Stored in DB via egg/template system | Similar concept |
| **Templates** | Local repos + remote template repos | Egg-based template system | Different naming, similar concept |
| **Cron Scheduling** | Built-in task scheduler | `forge/api/internal/services/scheduler/` | Both have scheduling |
| **Backups** | Basic file-based (tar.gz) | Extended with S3 backup config, status tracking, locking | Gamepanel has richer backup features |
| **SFTP** | Embedded SSH server with key auth | `beacon/internal/sftpserver/` | Both have SFTP |
| **OAuth2** | Built-in OAuth2 endpoint + token validation | OAuth2 clients + social auth | Both support OAuth2 |
| **WebAuthn** | Not present | Full WebAuthn passwordless auth | Gamepanel only |
| **2FA** | Not present | 2FA enhancements in migration store | Gamepanel only |
| **Audit Logging** | Not present | Full audit event system with store | Gamepanel only |
| **Activity Events** | Not present | `store_activity.go`, activity service | Gamepanel only |
| **Live Server Migration** | Not present | Migration engine + transfer protocol | Gamepanel only |
| **Server Transfer** | Not present | Planned migration state machine | Gamepanel only |
| **Evacuation Planner** | Not present | Full evacuation plan system | Gamepanel only |
| **Autoscaler** | Not present | Auto-scaling by node usage | Gamepanel only |
| **Load Balancer** | Not present | Load balancing across nodes | Gamepanel only |
| **Failover** | Not present | Failover service | Gamepanel only |
| **Traffic Manager** | Not present | Traffic management service | Gamepanel only |
| **Multi-Region** | Single node only | Regions, clusters, multi-node | Gamepanel only |
| **Placement Engine** | Manual node assignment | Scoring + constraint-based placement | Gamepanel only |
| **Reservation System** | Not present | Resource reservations | Gamepanel only |
| **Cloud Provider** | Not present | AWS integration | Gamepanel only |
| **Secrets Encryption** | Not present | Keyring-based encryption at rest | Gamepanel only |
| **Webhooks** | Not present | Full webhook store + delivery dispatcher | Gamepanel only |
| **Plugin System** | Not present | Plugin store and loader | Gamepanel only |
| **Job Queue** | Not present | PostgreSQL-backed job queue | Gamepanel only |
| **Rate Limiting** | Not present | Rate limit config store + middleware | Gamepanel only |
| **Observability** | Not present | Observability event persistence | Gamepanel only |
| **Health Checks** | Not present | Health check service + endpoint | Gamepanel only |
| **i18n** | Not present | 8 languages, i18n middleware | Gamepanel only |
| **Databases (server)** | Not present | Database provisioning service (dbprovisioner) | Gamepanel only |
| **Notifications** | Not present | Notification store + mail triggers | Gamepanel only |
| **Console Connections** | RCON, Telnet, RCONWS | WebSocket from beacon | Different approach |
| **Keep-Alive** | Built-in keep-alive system | Not present | Puffer only |
| **Crash Detection** | Crash counter + auto-restart | Crash detector in beacon | Both |
| **Console Forwarding** | Configurable stdout forwarding | Not present | Puffer only |
| **Logger with Rotator** | Custom rotator | slog-based | Different approaches |
| **OS Permission Groups** | `pufferpanel` group check | Not present | Puffer only |
| **Minecraft Query** | Built-in query protocol | Not present | Puffer only |
| **Email Providers** | Mailgun, Mailjet, SendGrid, SMTP, Debug | Built-in mail worker | Different provider models |
| **Systemd** | Service files included | In `infra/` directory | Both have deployment files |
| **API Documentation** | Swagger/OpenAPI via swaggo | Swagger in handlers | Both documented |

---

## Missing Features from Puffer (Not in Gamepanel)

### 1. Operation System (Install/Configure)
Puffer's 24 built-in operations (`operations/`) provide a modular install system: forge download, paper download, fabric download, neoforge download, sponge download, steam game download, curseforge modpack, archive, extract, write file, move file, mkdir, sleep, command execution, console commands, docker pull, etc. Gamepanel delegates this entirely to the daemon runtime (the beacon's `installer/`), but has none of the Minecraft-specific downloader operations.

### 2. CEL-Based Condition Engine
Puffer uses Google's CEL (Common Expression Language) for conditional operation execution (`conditions/`). Operations can have `if` expressions evaluated against server variables, environment type, file existence, and custom CEL functions. Gamepanel has no equivalent.

### 3. TTY / Local Process Environment
Puffer's `servers/tty/` provides a local non-Docker process execution environment. Gamepanel only supports container-based runtimes.

### 4. RCON / Telnet Console Connections
Puffer's `connections/` package provides RCON, Telnet, and RCON WebSocket-based console integration for game servers that support these protocols. Gamepanel only uses WebSocket streaming from the daemon.

### 5. Minecraft Query Protocol
Puffer's `query/minecraft.go` implements the Minecraft server query protocol for retrieving server status (player count, MOTD, etc.) without a WebSocket connection.

### 6. Template Repository System
Puffer supports multiple template repositories (`services/templates.go`, `models/templaterepo.go`) for pulling server definitions from remote sources. Gamepanel has an egg system but no remote template repository concept.

### 7. Variable System with User-Editable Flags
Puffer's `variable.go` defines typed variables (string, integer, boolean) with `UserEditable` and `Internal` flags, and `Options` for dropdown selections. Variables are organized by `Group` with display names and descriptions. Gamepanel stores variables in the database but lacks the rich UI grouping.

### 8. OS Permission Groups
Puffer's `groups/` package checks for OS-level group membership (`pufferpanel` group) before allowing CLI access — a security hardening feature not present in Gamepanel.

### 9. Console Output to Stdout
Puffer's `config.ConsoleForward` allows echoing server console output to the daemon's stdout — useful for debugging and monitoring. Not present in Gamepanel.

### 10. Keep-Alive System
Puffer's server definition includes a `KeepAlive` field with configurable frequency and command to prevent server idle timeout. Gamepanel doesn't have this.

### 11. Logger with File Rotator
Puffer's `logging/rotator.go` provides a custom log rotation implementation. Gamepanel uses Go's `slog` with no built-in rotation.

### 12. Crash Limit Configuration
Puffer reads `config.CrashLimit` to limit auto-restart attempts. Gamepanel has crash detection but the equivalent is less configurable.

### 13. Multiple Email Providers
Puffer supports 5 email providers out of the box (Mailgun, Mailjet, SendGrid, SMTP, Debug). Gamepanel has a mail worker but fewer provider integrations.

### 14. Command-Line User Management
Puffer's `cmd/user.go` provides CLI-based user creation and management (useful for initial setup without the web UI). Gamepanel's CLI is minimal (just API binary).

### 15. Database Version Command
Puffer has `cmd/db.go`, `cmd/dbmigrate.go`, `cmd/dbupgrade.go` for database management from CLI. Gamepanel runs migrations on startup in code.

### 16. Embedded Frontend
Puffer embeds the frontend SPA in the Go binary via `client/frontend/` and `fs.ReadFileFS`, allowing single-binary deployment. Gamepanel requires separate Next.js deployment.

---

## Extra Features in Our Project (Not in Puffer)

### 1. Multi-Container Runtime Support
Gamepanel supports Docker, containerd, Podman, Firecracker microVMs, and Kubernetes as server runtimes (`forge/api/internal/runtime/`). Puffer only runs Docker containers or local TTY processes.

### 2. Multi-Region / Clustering
Gamepanel has a full `domain.Cluster`, `domain.Region` model with region-aware placement. Servers can be provisioned across regions with automatic node selection via the placement engine.

### 3. Placement Engine with Constraints & Scoring
Gamepanel's `forge/api/internal/placement/` provides a sophisticated placement engine that filters candidates by constraints and scores remaining candidates by strategy (e.g., Least Loaded). Supports explainability (`explain.go`).

### 4. Live Server Migration
Gamepanel has a full migration engine (`migration.Service`) with planned states (planned → preparing → transferring → restoring → completed/failed), plus transfer protocol in the beacon (`transfer/`).

### 5. Evacuation Planner
Gamepanel can plan and execute node evacuations (`evacuationplanner.Service`), draining servers from a node before maintenance.

### 6. Autoscaler
Gamepanel can auto-scale node capacity based on utilization (`autoscaler.Service`).

### 7. Load Balancer
Gamepanel has a load balancer service for distributing server traffic across nodes.

### 8. Failover Service
Gamepanel detects node failures and can redistribute servers (`failover.Service`).

### 9. Traffic Manager
Gamepanel's `trafficmanager.Service` manages traffic routing between servers and nodes.

### 10. Encrypted Secrets at Rest
Gamepanel uses a `secrets.Keyring` with master key encryption for operational secrets, supporting key rotation and multiple key IDs. Puffer stores secrets in plaintext.

### 11. WebAuthn Passwordless Auth
Gamepanel has full WebAuthn support with credential store and Redis-backed session state. Not present in Puffer.

### 12. Two-Factor Authentication (2FA)
Gamepanel has 2FA enhancements and TOTP support via `store_2fa.go`.

### 13. Webhook System
Gamepanel has a full webhook delivery system (`webhook.Service`) with PostgreSQL-backed deliveries, retry logic, and Discord webhook formatting.

### 14. Job Queue
Gamepanel has a PostgreSQL-backed job queue (`queue.Service`) for async task processing.

### 15. Plugin System
Gamepanel can load and manage plugins at runtime (`plugins.Service` with `store_plugins.go`).

### 16. Audit Logging
Gamepanel has a complete audit event system (`store_audit_logs.go`, `handlers_activity.go`) tracking all user actions with metadata.

### 17. Observability / Event Timeline
Gamepanel persists all domain events to an event store (`eventstore/`) for timeline viewing and replay.

### 18. Rate Limiting
Gamepanel has configurable rate limiting (`rate_limit_settings.go`) with per-endpoint configuration.

### 19. Health Check System
Gamepanel has a composable health check service with database, cache, daemon, and system checks exposed at a health endpoint.

### 20. Database Provisioning
Gamepanel's `dbprovisioner.Service` can provision databases for game servers (e.g., MySQL/MariaDB for certain games).

### 21. Mail Notification Triggers
Gamepanel can trigger email notifications based on server events (`mail_notification_triggers.go`).

### 22. Social Auth
Gamepanel supports social authentication providers (`handlers_social_auth.go`, `store_social_auth.go`).

### 23. Password Reset
Gamepanel has a password reset flow with tokens (`handlers_password_reset.go`, `store_recovery.go`).

### 24. SSO / Cloud Provider
Gamepanel has cloud provider integration (AWS) and cloud service abstraction (`cloud/`).

### 25. Server Crashes Tracking
Gamepanel persists server crash events to the database (`store_server_crashes.go`).

### 26. SSH Keys Management
Gamepanel supports SSH key storage and management (`store_sshkeys.go`).

### 27. Internationalization (i18n)
Gamepanel supports 8 languages (en, de, es, fr, ja, pt, ru, zh) with a middleware that negotiates language from the `Accept-Language` header.

### 28. Next.js Frontend
Gamepanel's frontend is a modern Next.js app router application with TypeScript, Tailwind CSS, and a component library. Puffer's frontend is an older embedded SPA.

### 29. SDK & Shared Types
Gamepanel has a TypeScript SDK and shared-types package for programmatic API access. Puffer has no equivalent.

### 30. Shared UI Component Library
Gamepanel has `packages/ui/` with reusable React components. Puffer's UI is monolithic.

### 31. Predictive Scheduling
Gamepanel's `scheduler.PredictiveScorer` uses historical data to predict optimal scheduling decisions.

---

## Architectural Differences

### 1. Monolithic vs Split Architecture

**Puffer Panel** is a single Go binary that acts as both panel and daemon:
```
pufferpanel run   # Starts HTTP server + daemon + SFTP + all servers
```
A single process handles API requests, manages server processes, SFTP, and the web UI. The `config.PanelEnabled` and `config.DaemonEnabled` flags conditionally enable parts, but everything compiles into one binary.

**Gamepanel** has two separate binaries:
```
forge/api     # Panel server (API + orchestration)
beacon/       # Daemon (server execution on each node)
```
The forge API communicates with beacon daemons over HTTP/gRPC. This allows horizontal scaling — multiple beacon nodes managed by a single forge panel.

### 2. HTTP Framework

- **Puffer:** Uses Gin (`github.com/gin-gonic/gin`) with `github.com/gin-contrib/sessions` for cookie-based sessions.
- **Gamepanel:** Uses Fiber (`github.com/gofiber/fiber/v2`) — a faster, Express-like framework. Uses JWT-based auth (HMAC-signed tokens) rather than Gin sessions.

### 3. Database Layer

- **Puffer:** Uses GORM with automatic migration (auto-migrate) supplemented by manual upgrade functions in `database/upgrade.go`. Simple but error-prone.
- **Gamepanel:** Uses GORM with explicit versioned SQL migrations (`migrations/001_init.sql` through `migrations/059_server_crash_events.sql`), each idempotent (`CREATE TABLE IF NOT EXISTS`). The `store/` package provides a full repository abstraction layer with 80+ store files.

### 4. Frontend Delivery

- **Puffer:** The frontend is compiled as an SPA (likely Angular/Vue) and embedded in the Go binary via `embed.FS`. The Go executable serves everything on a single port.
- **Gamepanel:** The frontend is a separate Next.js application running on its own port (typically `:3000`), communicating with the forge API (`:8080`) via REST + WebSocket. Requires a reverse proxy in production.

### 5. Server Execution Model

- **Puffer:** The `servers/` package loads server definitions from a folder on disk. Each server has a `RunningEnvironment` that wraps either a Docker container or a local TTY process. The environment is managed in-process.
- **Gamepanel:** Server execution is fully delegated to the beacon daemon. The forge panel maintains desired state in the database and reconciles via `reconciler.Service`, sending commands to beacon over HTTP. The beacon manages containers through its runtime adapters.

### 6. Event / Message System

- **Puffer:** Uses in-memory WebSocket trackers (`tracker.go`) for console, stats, and status. No persistence.
- **Gamepanel:** Has a sophisticated event system with `events.Registry` (pub/sub), `eventstore` (PostgreSQL-backed with outbox pattern), and persistent timeline for observability.

### 7. Configuration

- **Puffer:** Uses Viper with JSON config files. Environment variables are prefixed `PUFFER_`. Config entries are defined in `config/entries.go`.
- **Gamepanel:** Uses environment variables exclusively (no config files). `FORGE_*` prefix. Config structs passed around via dependency injection.

### 8. Lifecycle Management

- **Puffer:** Servers are loaded from disk, queued for start in a `container/list`, and processed by a ticker-based queue (1s interval). Stats are polled every 5s.
- **Gamepanel:** Uses a reconciliation loop (`reconciler.Service`) with event-driven updates. Desired state is stored in DB, actual state is checked against beacon, and drift is corrected.

### 9. Frontend Technology

- **Puffer:** Older embedded SPA with CSS/JS/fonts/wasm files served from Go.
- **Gamepanel:** Modern Next.js app router with TypeScript, Tailwind CSS, ESLint, Vitest, shared component library, and TypeScript SDK.

---

## Recommendations

### Priority 1 — Add Missing Core Features
1. **Operation System:** Port Puffer's 24 operation types (forge, paper, fabric, steam downloads, etc.) to Gamepanel's installer service. These are critical for a game panel that claims Minecraft/Steam support.
2. **CEL Conditions:** Adopt the CEL-based condition evaluation for install scripts and server variables. This would make Gamepanel's egg/template system more flexible.
3. **Console Connections:** Add support for RCON/Telnet console proxying for games that don't support WebSocket stdin.
4. **Minecraft Query:** Implement the server query protocol for game status display without requiring WebSocket.

### Priority 2 — Architectural Improvements
1. **Template Repository System:** Add remote template/egg repository support akin to Puffer's `templaterepo.go`.
2. **Variable System Enhancement:** Add typed variables with options, groups, descriptions, and user-editable flags — mirror Puffer's `variable.go`.
3. **Crash Limit Config:** Make crash detection limits configurable per-server or globally.
4. **Keep-Alive:** Add the keep-alive feature for servers that need periodic input to stay alive.

### Priority 3 — Deployment & Operations
1. **CLI User Management:** Add a `user` subcommand to the forge binary for headless user creation.
2. **CLI Database Management:** Add migration run/status/rollback commands.
3. **Email Provider Expansion:** Add SendGrid, Mailgun, and Mailjet providers alongside the custom mail worker.
4. **Single-Binary Mode:** Consider a mode where forge+beacon can run as a single process for small deployments (Puffer's model).

### Priority 4 — Security Hardening
1. **OS Group Checks:** Add optional OS permission group checks for CLI access.
2. **Console Forwarding:** Add optional console-to-stdout forwarding for debugging.

### Priority 5 — Keep Your Differentiators
Gamepanel's cloud-native features (multi-region, clustering, migration, autoscaling, load balancing, failover, secrets encryption, WebAuthn, event store, observability, rate limiting, webhooks, plugins, job queues) are significant advantages over Puffer. **Do not regress these** while adopting Puffer's missing features.
