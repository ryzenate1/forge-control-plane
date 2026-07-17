# MASTER REFERENCE Part 2 — Pterodactyl Conversion Map

> Continue from MASTER_REFERENCE.md. Feed both files to any AI agent.

## 7. PTERODACTYL vs MODERN PANEL — 1:1 CONVERSION STATUS

### Panel Controllers → Our API handlers

| Pterodactyl PHP Controller | Our Go Equivalent | Status |
|---|---|---|
| `Api/Client/AccountController.php` | `auth.go` → `/auth/me` | ✅ Basic (missing update profile) |
| `Api/Client/ApiKeyController.php` | — | ❌ NOT BUILT |
| `Api/Client/SSHKeyController.php` | — | ❌ NOT BUILT |
| `Api/Client/TwoFactorController.php` | — | ❌ NOT BUILT |
| `Api/Client/ActivityLogController.php` | `server.go` → `/servers/:id/activity` | ✅ API only, no UI |
| `Api/Client/ClientController.php` (server list) | `server.go` → `/servers` | ✅ |
| `Api/Client/Servers/PowerController.php` | `server.go` → `/servers/:id/power` | ✅ |
| `Api/Client/Servers/CommandController.php` | via WebSocket console | ✅ Different approach |
| `Api/Client/Servers/ResourceUtilizationController.php` | `server.go` → `/servers/:id/stats` | ✅ |
| `Api/Client/Servers/WebsocketController.php` | `realtime.go` → `/servers/:id/ws/*` | ✅ |
| `Api/Client/Servers/FileController.php` | `server.go` → `/servers/:id/files/*` | ✅ |
| `Api/Client/Servers/FileUploadController.php` | `server.go` → `/servers/:id/files/upload` | ✅ Chunked |
| `Api/Client/Servers/BackupController.php` | `server.go` → `/servers/:id/backups/*` | ✅ (no restore) |
| `Api/Client/Servers/DatabaseController.php` | `server.go` → `/servers/:id/databases/*` | ✅ |
| `Api/Client/Servers/ScheduleController.php` | `server.go` → `/servers/:id/schedules/*` | ✅ |
| `Api/Client/Servers/ScheduleTaskController.php` | `server.go` → `/servers/:id/schedules/:sid/tasks/*` | ✅ |
| `Api/Client/Servers/NetworkAllocationController.php` | `server.go` → `/servers/:id/allocations/*` | ✅ |
| `Api/Client/Servers/StartupController.php` | `server.go` → `/servers/:id/startup` | ✅ |
| `Api/Client/Servers/SettingsController.php` | `server.go` → `PATCH /servers/:id` | ✅ Partial |
| `Api/Client/Servers/SubuserController.php` | `server.go` → `/servers/:id/users/*` | ✅ |
| `Api/Client/Servers/ServerController.php` | `server.go` → `GET /servers/:id` | ✅ |
| `Api/Client/Servers/ActivityLogController.php` | `server.go` → `/servers/:id/activity` | ✅ |
| `Api/Remote/SftpAuthenticationController.php` | `server.go` → `/api/remote/sftp/auth` | ✅ |
| `Api/Remote/EggInstallController.php` | `server.go` → `/api/remote/servers/:id/install` | ✅ |
| `Api/Remote/Servers/*` | `server.go` → `/api/remote/servers/*` | ✅ |
| `Admin/ServersController.php` | `server.go` → admin routes | ✅ API, ⚠️ UI partial |
| `Admin/NodesController.php` | `server.go` → `/nodes/*` | ✅ |
| `Admin/UserController.php` | `server.go` → `/users/*` | ✅ API, ❌ UI stub |
| `Admin/DatabaseController.php` | `server.go` → `/database-hosts/*` | ✅ |
| `Admin/MountController.php` | `server.go` → `/mounts/*` | ✅ |
| `Admin/LocationController.php` | — | ❌ NOT BUILT |
| `Admin/Nests/*` | — | ❌ Nests table exists, no CRUD API |
| `Admin/Settings/*` | — | ❌ NOT BUILT |
| `Admin/NodeAutoDeployController.php` | — | ❌ NOT BUILT |

### Panel Models → Our Store Types

| Pterodactyl Model | Our Go struct in `store.go` | Status |
|---|---|---|
| `User.php` (10.4KB) | `store.User` | ✅ (missing: username, 2FA, SSH keys) |
| `Server.php` (16.6KB) | `store.Server` | ✅ (has all key fields) |
| `Node.php` (8.5KB) | `store.Node` | ✅ (has all fields including heartbeat) |
| `Allocation.php` (4.3KB) | `store.Allocation` | ✅ |
| `Egg.php` (10.3KB) | `store.Template` | ⚠️ Simplified (no variables, no install script editor) |
| `EggVariable.php` (3KB) | `store.StartupVariable` | ✅ |
| `Nest.php` (1.8KB) | DB table exists, no Go struct | ⚠️ Partial |
| `Schedule.php` (4.5KB) | `store.Schedule` | ✅ |
| `Task.php` (3.7KB) | `store.ScheduleTask` | ✅ |
| `Backup.php` (2.6KB) | No Go struct (daemon handles) | ⚠️ No DB persistence |
| `Database.php` (3.5KB) | `store.ServerDatabase` | ✅ |
| `DatabaseHost.php` (2.5KB) | `store.DatabaseHost` | ✅ |
| `Mount.php` (3.6KB) | `store.Mount` | ✅ |
| `Subuser.php` (2.6KB) | `store.ServerSubuser` | ✅ |
| `Permission.php` (11KB) | No equivalent | ❌ Granular permissions not implemented |
| `ServerTransfer.php` (3.2KB) | Transfer fields on Server struct | ✅ |
| `Location.php` (2KB) | DB table exists, no API | ⚠️ |
| `ApiKey.php` (8.5KB) | — | ❌ NOT BUILT |
| `UserSSHKey.php` (2.7KB) | — | ❌ NOT BUILT |
| `AuditLog.php` / `ActivityLog.php` | `store.AuditEvent` | ✅ |
| `RecoveryToken.php` | — | ❌ NOT BUILT |
| `Setting.php` | — | ❌ NOT BUILT |

### Wings (Daemon) → Our Daemon

| Wings File | Our Equivalent | Status |
|---|---|---|
| `router/router.go` (route setup) | `server/server.go` lines 40-71 | ✅ |
| `router/router_server.go` (power, logs, stats) | `server/server.go` | ✅ |
| `router/router_server_files.go` (18KB file ops) | `server/server.go` | ✅ (simpler, no archive/decompress) |
| `router/router_server_backup.go` (7.5KB) | `server/server.go` | ✅ (zip only, no S3) |
| `router/router_server_ws.go` (WebSocket) | `server/server.go` | ✅ |
| `router/router_server_transfer.go` | Not in daemon | ⚠️ Transfer handled by API |
| `router/router_system.go` (system info) | `/health`, `/metrics` | ✅ |
| `environment/docker/container.go` (15.7KB) | `runtime/docker.go` (274 lines) | ⚠️ Ours is simpler |
| `environment/docker/power.go` (12.3KB) | `runtime/docker.go` Start/Stop/Kill/Restart | ⚠️ Missing power state machine |
| `environment/docker/stats.go` (4.7KB) | `runtime/stats.go` | ✅ |
| `environment/docker/environment.go` (6.8KB) | `runtime/docker.go` | ⚠️ Missing log config, OOM handling |
| `server/filesystem/filesystem.go` (15.2KB) | Daemon uses raw `os` package | ⚠️ Missing: disk limits, archive, decompress |
| `server/install.go` (19.6KB) | `server/server.go` install handler | ⚠️ Much simpler |
| `server/power.go` (8.9KB) | Power via Docker SDK directly | ⚠️ No state machine |
| `server/manager.go` (8.6KB) | No equivalent (daemon is stateless) | ❌ |
| `server/crash.go` (3.5KB) | — | ❌ No crash detection |
| `server/console.go` (2.4KB) | WebSocket console handler | ✅ |
| `server/mounts.go` (3.3KB) | Mounts passed in create request | ✅ |
| `server/backup/` (backup adapters) | Zip-only backup | ⚠️ No S3 adapter |
| `server/transfer/` (transfer logic) | — | ❌ Transfer is API-only |
| `sftp/server.go` (8.2KB) | Docker sidecar (atmoz/sftp) | ⚠️ Not Go-native |
| `sftp/handler.go` (10.5KB) | — | ❌ No panel-auth SFTP |
| `remote/http.go` (10.6KB, panel client) | `cmd/daemon/main.go` sync + heartbeat | ✅ Basic |
| `remote/servers.go` (6.4KB) | Panel sync in main.go | ⚠️ Simplified |

## 8. WHAT'S NOT BUILT (Prioritized)

### Must-Have for Pterodactyl Parity
1. **Granular permissions system** — Pterodactyl has 40+ permission constants (see `Permission.php`). We have admin/user only.
2. **API keys** — `ApiKey.php` model, CRUD + token generation for programmatic access
3. **Locations** — `Location.php`, admin CRUD for geographic node grouping
4. **Nests/Eggs admin CRUD** — Tables exist, no API or UI for managing them
5. **Settings admin panel** — Mail config, general settings
6. **Crash detection** — Wings `crash.go` auto-restarts crashed servers
7. **Power state machine** — Wings tracks `starting`, `running`, `stopping`, `offline` states properly
8. **File archive/decompress** — Wings supports tar.gz compression in file manager
9. **Backup restore** — We create backups but can't restore them
10. **Node auto-deploy** — Generate Wings config YAML for new nodes

### Should-Have
11. **2FA (TOTP)** — `TwoFactorController.php`, recovery tokens
12. **SSH keys** — `UserSSHKey.php`, key management
13. **Activity logs UI** — Data exists, no frontend view
14. **Admin UI screens** — Users, Templates, Allocations, Nests management
15. **Server selection UX** — Dashboard hardcodes `servers[0]`
16. **S3 backup adapter** — Wings supports S3, we only do local zip
17. **Go-native SFTP server** — Replace Docker sidecar with `pkg/sftp`
18. **Disk space tracking** — Wings `filesystem/disk_space.go` enforces limits

## 9. CONVERSION STRATEGY

### How to Convert a Pterodactyl Feature

```
STEP 1: Read the Pterodactyl source (LOCAL, not GitHub!)
  Panel: refs/pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/<Feature>Controller.php
  Model: refs/pterodactyl-panel/app/Models/<Feature>.php
  Service: refs/pterodactyl-panel/app/Services/<Feature>/
  Migration: refs/pterodactyl-panel/database/migrations/*<feature>*
  Routes: refs/pterodactyl-panel/routes/api-client.php
  Wings: refs/pterodactyl-wings/server/<feature>.go
  Wings router: refs/pterodactyl-wings/router/router_server_<feature>.go

STEP 2: Understand the BEHAVIOR
  - What does the controller validate?
  - What permissions does it check? (see Permission.php constants)
  - What service class does it call?
  - What events does it fire?
  - What does Wings do when it receives the request?

STEP 3: Implement in our stack
  - Add migration SQL in apps/api/migrations/0XX_<feature>.sql
  - Add store methods in apps/api/internal/store/ (split file by domain)
  - Add handler in apps/api/internal/http/ (split file by domain)
  - Add daemon endpoint if needed in apps/daemon/internal/server/
  - Add frontend API function in apps/frontend/lib/api.ts
  - Add frontend UI component in apps/frontend/components/

STEP 4: Wire it all together
  - API handler calls store for DB ops
  - API handler calls daemon client for container ops
  - Frontend component calls API client function
  - Test the full flow
```

### Parallel Development Strategy

```
Agent 1 (Backend/API):     apps/api/internal/http/ + apps/api/internal/store/
Agent 2 (Daemon):          apps/daemon/internal/server/ + apps/daemon/internal/runtime/
Agent 3 (Frontend):        apps/frontend/components/ + apps/frontend/lib/api.ts
Agent 4 (Review/Security): Review all changes, run CodeRabbit

All agents read: docs/MASTER_REFERENCE.md + docs/MASTER_REFERENCE_PART2.md
All agents refer: refs/pterodactyl-panel/ and refs/pterodactyl-wings/ (LOCAL)
```

## 10. DEVOPS & DEPLOYMENT INSTRUCTIONS

### Prerequisites for Testing
- **Ubuntu VM** (or WSL2) — daemon REQUIRES Linux + Docker
- Docker Engine installed on the Linux environment
- PostgreSQL 16 (via Docker or native)
- Redis 7 (via Docker or native)
- Go 1.25+ for building API and daemon
- Node.js 20+ for frontend

### Option A: Docker Compose (Easiest)
```bash
# From project root on a Linux machine with Docker:
docker compose -f infra/docker/docker-compose.yml up -d --build
# Starts: postgres, redis, api, daemon, prometheus, grafana, sftp
# API: http://localhost:8080
# Frontend: run separately with npm run dev (port 3002)
# Login: admin@example.com / admin123
```

### Option B: Local Development (Each service separately)
```bash
# Terminal 1: PostgreSQL + Redis (Docker)
docker run -d --name postgres -e POSTGRES_DB=gamepanel -e POSTGRES_USER=gamepanel -e POSTGRES_PASSWORD=gamepanel -p 5432:5432 postgres:16-alpine
docker run -d --name redis -p 6379:6379 redis:7-alpine

# Terminal 2: API
cd apps/api
DATABASE_URL="postgres://gamepanel:gamepanel@localhost:5432/gamepanel?sslmode=disable" REDIS_ADDR="localhost:6379" go run ./cmd/api

# Terminal 3: Daemon (MUST be on Linux with Docker)
cd apps/daemon
DAEMON_ALLOW_MOCK_RUNTIME=true go run ./cmd/daemon
# Set DAEMON_ALLOW_MOCK_RUNTIME=false when Docker is available

# Terminal 4: Frontend
npm install  # from project root
npm run dev  # starts on port 3002
```

### What Agents CANNOT Do (You Must Handle)
1. **Start Docker containers** — agents cannot run Docker
2. **Test WebSocket connections** — needs a running browser + server
3. **Test daemon container operations** — needs Docker Engine on Linux
4. **Deploy to production** — needs Linux server access
5. **Run integration tests** — needs full stack running
6. **Network configuration** — firewall, ports, DNS
7. **SSL certificates** — Let's Encrypt setup

### Smoke Test After Deployment
```bash
# Health checks
curl http://localhost:8080/api/v1/health
curl http://localhost:9090/health

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin123"}'

# Use returned token for authenticated requests
curl http://localhost:8080/api/v1/servers \
  -H "Authorization: Bearer <TOKEN>"
```

## 11. PTERODACTYL REFERENCE PATHS (For Agent Prompting)

```
PANEL CONTROLLERS:
  refs/pterodactyl-panel/app/Http/Controllers/Api/Client/          — Client API (user-facing)
  refs/pterodactyl-panel/app/Http/Controllers/Api/Application/     — Application API (admin)
  refs/pterodactyl-panel/app/Http/Controllers/Api/Remote/          — Remote API (daemon→panel)
  refs/pterodactyl-panel/app/Http/Controllers/Admin/               — Admin web UI controllers

PANEL MODELS:
  refs/pterodactyl-panel/app/Models/                               — All Eloquent models (32 files)

PANEL SERVICES:
  refs/pterodactyl-panel/app/Services/Servers/                     — Server creation, deletion, suspension, etc.
  refs/pterodactyl-panel/app/Services/Eggs/                        — Egg CRUD + parsing
  refs/pterodactyl-panel/app/Services/Nodes/                       — Node CRUD + JWT
  refs/pterodactyl-panel/app/Services/Schedules/                   — Schedule processing
  refs/pterodactyl-panel/app/Services/Allocations/                 — Allocation management
  refs/pterodactyl-panel/app/Services/Backups/                     — Backup management
  refs/pterodactyl-panel/app/Services/Databases/                   — Database management
  refs/pterodactyl-panel/app/Services/Subusers/                    — Subuser management

PANEL ROUTES:
  refs/pterodactyl-panel/routes/api-client.php                     — Client API routes
  refs/pterodactyl-panel/routes/api-application.php                — Application API routes
  refs/pterodactyl-panel/routes/api-remote.php                     — Remote API routes
  refs/pterodactyl-panel/routes/admin.php                          — Admin routes

PANEL MIGRATIONS:
  refs/pterodactyl-panel/database/migrations/                      — 195 migration files

PANEL PERMISSIONS:
  refs/pterodactyl-panel/app/Models/Permission.php                 — 40+ permission constants

WINGS (DAEMON):
  refs/pterodactyl-wings/router/                                   — All HTTP handlers
  refs/pterodactyl-wings/server/                                   — Server lifecycle, install, power, events
  refs/pterodactyl-wings/server/filesystem/                        — Jailed file operations
  refs/pterodactyl-wings/server/backup/                            — Backup adapters
  refs/pterodactyl-wings/server/transfer/                          — Server transfer logic
  refs/pterodactyl-wings/environment/docker/                       — Docker runtime
  refs/pterodactyl-wings/sftp/                                     — Go-native SFTP server
  refs/pterodactyl-wings/remote/                                   — Panel API client
  refs/pterodactyl-wings/config/                                   — Configuration structs
```

## 12. KNOWN ISSUES TO FIX

1. **server.go (API)** is 2652 lines — split into domain files
2. **store.go** is 3262 lines — split into domain files
3. **dashboard.tsx** is 162KB — split into ~15 components
4. **Server selection** hardcodes `servers[0]` — needs route-based selection
5. **Demo fallbacks** in `fetchNodes`/`fetchServers` inject fake CPU/memory values
6. **No test coverage** for frontend, minimal for backend
7. **WebSocket goroutine leaks** possible (no timeout/cancel in WS handlers)
8. **No rate limiting** on login (Redis available but not used)
9. **Migration runner** has no tracking table (re-runs all migrations every startup)
10. **SFTP sidecar** uses static credentials, not panel auth

## 13. STACK DECISIONS (FINAL)

- **API Framework**: Fiber (KEEP — already 2652 lines deep, faster than Gin)
- **Daemon HTTP**: stdlib net/http (KEEP — needs raw hijacked connections)
- **Database**: PostgreSQL 16 (KEEP)
- **Cache**: Redis 7 (KEEP)
- **Frontend**: Next.js 15 + React 19 + TailwindCSS (KEEP)
- **Container Runtime**: Docker SDK behind `runtime.Runtime` interface (KEEP)
- **Python**: FastAPI for NON-CRITICAL microservices only (analytics, egg import, AI helpers)
- **SFTP**: Replace Docker sidecar with Go-native `pkg/sftp` (TODO)
