# MASTER REFERENCE — Modern Game Panel

> **PURPOSE**: This is the single document any AI agent must read to understand the entire project. Do NOT waste tokens scanning files one by one. Read this first, then go directly to the file you need.

> **CRITICAL RULE FOR ALL AGENTS**: The Pterodactyl reference repos are LOCAL at `refs/pterodactyl-panel` and `refs/pterodactyl-wings`. ALWAYS read these local files. NEVER try to fetch from GitHub. NEVER hallucinate Pterodactyl behavior — read the actual source.

## 1. PROJECT IDENTITY

- **Name**: Modern Game Panel
- **Goal**: 1:1 conversion of Pterodactyl (PHP LAMP stack) to modern stack (Go + Next.js + PostgreSQL)
- **Stage**: MVP functional, needs code quality + full Pterodactyl parity
- **Stack**: Go Fiber API, Go stdlib Daemon, Next.js 15 frontend, PostgreSQL 16, Redis 7, Docker

## 2. ARCHITECTURE (How Everything Connects)

```
User Browser (port 3002)
    │
    ├── REST API calls ──► Go Fiber API (port 8080)
    │                         ├── PostgreSQL 16 (port 5432) — all data
    │                         ├── Redis 7 (port 6379) — cache/queue
    │                         └── HTTP to Daemon (signed HMAC-SHA256)
    │                                │
    │                                ▼
    │                         Go Daemon (port 9090)
    │                         ├── Docker Engine API — container lifecycle
    │                         ├── Jailed filesystem — server files
    │                         └── WebSocket — console/stats/logs
    │
    └── WebSocket ──► API ──► proxied to Daemon WebSocket
```

**Auth flow**: Login → API issues HMAC-SHA256 token → stored in localStorage → sent as Bearer header
**Daemon trust**: API signs requests with node token + timestamp (5-min window)

## 3. COMPLETE FILE MAP

### API Service (`apps/api/`)
```
apps/api/
├── cmd/api/main.go              — Entry point (115 lines). DB connect, Redis, healthcheck mode
├── internal/
│   ├── http/
│   │   ├── server.go            — ALL API routes (2652 lines) ⚠️ NEEDS SPLITTING
│   │   ├── auth.go              — JWT-like token issue/parse/middleware (121 lines)
│   │   ├── realtime.go          — WebSocket proxy to daemon (113 lines)
│   │   └── schedule_runner.go   — Cron-based schedule executor (186 lines)
│   ├── store/
│   │   └── store.go             — ALL database queries (3262 lines) ⚠️ NEEDS SPLITTING
│   └── daemon/
│       └── client.go            — HTTP client to daemon with HMAC signing (585 lines)
├── migrations/                  — 16 SQL migration files (001-016)
├── go.mod                       — Go 1.25, Fiber v2, pgx v5, go-redis v9
└── Dockerfile
```

### Daemon Service (`apps/daemon/`)
```
apps/daemon/
├── cmd/daemon/main.go           — Entry point + heartbeat loop + panel sync (231 lines)
├── internal/
│   ├── runtime/
│   │   ├── runtime.go           — Runtime interface (73 lines) — Create/Start/Stop/Kill/Stats/Logs/Console/Delete
│   │   ├── docker.go            — Docker SDK implementation (274 lines)
│   │   ├── stats.go             — Docker stats decoder (52 lines)
│   │   └── stats_test.go        — Stats tests
│   └── server/
│       ├── server.go            — All 20 daemon HTTP handlers (1160 lines)
│       └── server_test.go       — Handler tests
├── go.mod                       — Go 1.25, Docker SDK v28, gorilla/websocket
└── Dockerfile
```

### Frontend (`apps/frontend/`)
```
apps/frontend/
├── app/
│   ├── layout.tsx, page.tsx, globals.css
│   ├── admin/
│   │   ├── page.tsx             — Admin dashboard entry
│   │   ├── nodes/, servers/, nests/  — Route stubs
│   └── server/[id]/             — Dynamic server route (stub)
├── components/
│   ├── dashboard.tsx            — ENTIRE UI (162KB) ⚠️ NEEDS SPLITTING into ~15 components
│   ├── server-console.tsx       — WebSocket console component
│   ├── file-editor.tsx          — Monaco editor wrapper
│   ├── backups-panel.tsx        — Backup management UI
│   ├── transfer-panel.tsx       — Server transfer UI
│   └── pterodactyl/route-screen.tsx
├── lib/
│   ├── api.ts                   — ALL API client functions (~60 functions, 1104 lines)
│   ├── mock-data.ts             — Demo fallback data
│   └── utils.ts
├── stores/use-panel-store.ts    — Zustand store
└── package.json                 — Next.js 15, React 19, TailwindCSS 3, Monaco, Zustand, TanStack Query
```

### Infrastructure
```
infra/
├── docker/docker-compose.yml    — 7 services: postgres, redis, api, daemon, prometheus, grafana, sftp
├── monitoring/prometheus.yml    — Scrape config for API + daemon /metrics
├── nginx/                       — Scaffold (not configured)
└── scripts/                     — Production env generator, smoke tests

packages/contracts/
└── openapi.yaml                 — OpenAPI 3.1 spec (490 lines)

scripts/
└── ws-smoke.js                  — WebSocket smoke test
```

## 4. DATABASE SCHEMA (16 migrations)

**Core tables**: `users`, `nodes`, `servers`, `server_templates`, `allocations`, `audit_events`
**RBAC**: `roles`, `role_rules`, `user_roles`
**Hierarchy**: `locations`, `nests`, `eggs`
**Server features**: `schedules`, `schedule_tasks`, `schedule_runs`, `schedule_task_runs`
**Server data**: `startup_variables`, `server_startup_values`, `server_databases`, `database_hosts`
**Sharing**: `subusers` (with JSONB permissions)
**Infrastructure**: `mounts`, `mount_nodes`, `mount_templates`, `server_mounts`
**Transfers**: `server_transfers`
**Server fields**: `primary_allocation_id`, `suspended`, `transfer_state`, `transfer_target_node_id`, `description`, `installed_at`, `database_limit`, `backup_limit`, `allocation_limit`, `io_weight`, `swap_mb`, `threads`, `oom_disabled`

**Seed data**: Admin user `admin@example.com` / `admin123`, one node, one template (Minecraft Java), one server, two allocations.

## 5. API ENDPOINTS (What's implemented)

### Auth
- `POST /api/v1/auth/login` — email/password → token ✅
- `GET /api/v1/auth/me` — current user from token ✅

### Nodes (full CRUD)
- `GET/POST /api/v1/nodes` ✅
- `GET/PATCH/DELETE /api/v1/nodes/:id` ✅
- `POST /api/v1/nodes/:id/heartbeat` ✅
- `POST /api/v1/nodes/:id/rotate-token` ✅
- `GET /api/v1/nodes/:id/configuration` ✅
- `GET /api/v1/nodes/:id/allocations` ✅
- `GET /api/v1/nodes/:id/servers` ✅

### Servers (full lifecycle)
- `GET/POST /api/v1/servers` ✅
- `GET/PATCH/DELETE /api/v1/servers/:id` ✅
- `POST /api/v1/servers/:id/power` — start/stop/restart/kill ✅
- `POST /api/v1/servers/:id/install` ✅
- `POST /api/v1/servers/:id/suspension` ✅
- `POST /api/v1/servers/:id/toggle-install` ✅
- `POST/GET /api/v1/servers/:id/transfer` ✅
- `POST /api/v1/servers/:id/transfer/cancel` ✅
- `GET /api/v1/servers/:id/stats` ✅
- `GET /api/v1/servers/:id/logs` ✅
- `GET /api/v1/servers/:id/activity` ✅

### Files (complete)
- `GET /api/v1/servers/:id/files` ✅
- `GET/PUT /api/v1/servers/:id/files/content` ✅
- `PUT /api/v1/servers/:id/files/upload` — chunked ✅
- `DELETE /api/v1/servers/:id/files` ✅
- `POST /api/v1/servers/:id/files/mkdir` ✅
- `PATCH /api/v1/servers/:id/files/rename` ✅

### Backups, Allocations, Templates, Users, Schedules, Databases, Mounts, Subusers — ALL ✅
### WebSocket: `/ws/stats`, `/ws/logs`, `/ws/console` — ALL ✅
### Remote API (Wings-compatible): `/api/remote/servers`, `/api/remote/sftp/auth` — ALL ✅

### Daemon Endpoints (20 routes, all ✅)
`/health`, `/metrics`, `POST /servers`, `DELETE /servers/:id`, `GET/PUT /servers/:id/configuration`, `POST /servers/:id/install`, `POST /servers/:id/power`, `GET /servers/:id/stats`, `GET /servers/:id/logs`, backups (list/create/download), WebSocket (stats/logs/console), files (list/read/write/upload/delete/mkdir/rename)

## 6. FRONTEND API CLIENT FUNCTIONS (lib/api.ts)

All 60 functions with types: `login`, `logout`, `fetchCurrentUser`, `fetchUsers`, `createUser`, `fetchNodes`, `fetchNode`, `createNode`, `updateNode`, `deleteNode`, `rotateNodeToken`, `fetchNodeConfiguration`, `fetchNodeAllocations`, `fetchNodeServers`, `fetchServers`, `fetchServer`, `createServer`, `updateServer`, `deleteServer`, `installServer`, `reinstallServer`, `sendPowerSignal`, `suspendServer`, `unsuspendServer`, `toggleServerInstallStatus`, `transferServer`, `fetchServerTransferStatus`, `cancelServerTransfer`, `fetchAllocations`, `createAllocation`, `updateAllocation`, `deleteAllocation`, `fetchServerAllocations`, `assignServerAllocation`, `unassignServerAllocation`, `setPrimaryServerAllocation`, `fetchTemplates`, `createTemplate`, `fetchAuditEvents`, `fetchServerStats`, `fetchServerLogs`, `fetchServerStartup`, `updateServerStartupVariable`, `fetchServerFiles`, `readServerFile`, `writeServerFile`, `deleteServerFile`, `createServerDirectory`, `renameServerFile`, `uploadFileChunked`, `fetchBackups`, `createBackup`, `downloadBackup`, `fetchServerSchedules`, `createServerSchedule`, `updateServerSchedule`, `deleteServerSchedule`, `createServerScheduleTask`, `deleteServerScheduleTask`, `fetchServerScheduleRuns`, `runServerSchedule`, `fetchServerDatabases`, `createServerDatabase`, `rotateServerDatabasePassword`, `deleteServerDatabase`, `fetchDatabaseHosts`, `createDatabaseHost`, `deleteDatabaseHost`, `fetchMounts`, `createMount`, `deleteMount`, `fetchServerMounts`, `assignServerMount`, `removeServerMount`, `fetchServerUsers`, `upsertServerUser`, `updateServerUser`, `deleteServerUser`, `fetchServerActivity`, `serverWebSocketURL`
