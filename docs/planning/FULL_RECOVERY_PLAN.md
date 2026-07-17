# GamePanel Full Recovery & Parity Plan

**Date:** 2026-06-17
**Status:** Plan mode
**Goal:** Execute all 12 phases in dependency order, fixing bugs before building new features.

---

## Current Reality (from code audit)

| Component | Status |
|---|---|
| **Daemon (Beacon)** | Strongest piece. All routes work. Docker, SFTP, backups, console WS, auto-restart. |
| **Frontend (Plane)** | All pages have `page.tsx`. API client covers all endpoints. No mock data. |
| **API (Forge)** | 6 services are nil → ~15 endpoints panic or return errors. |
| **Deploy** | compose.yml paths broken. Dockerfiles use non-existent Go image. |
| **Competitor docs** | `docs/recovery/02_COMPETITOR_BEHAVIOR_MAP.md` (32KB) exists. |
| **Workspace** | Clean. `apps/api`, `apps/daemon`, `apps/panel`. Builds pass. |

---

## Phase Dependency Map

```
Phase 1 (behavior map) ───────────── DONE
Phase 2 (workspace cleanup) ──────── DONE

STAGE 1 — Fix immediate bugs (blocks everything)
STAGE 2 — API contracts doc (Phase 6)
STAGE 3 — Backend conversion (Phase 4)
STAGE 4 — Daemon hardening (Phase 5)
STAGE 5 — Core features parity (Phase 7)
STAGE 6 — Frontend rebuild (Phase 8)
STAGE 7 — Advanced orchestration (Phase 9) ← ONLY after core works
STAGE 8 — Multi-DB (Phase 10)
STAGE 9 — Deploy hardening (Phase 11)
STAGE 10 — Competitor-beating (Phase 12)
```

---

## STAGE 1: FIX IMMEDIATE BUGS

### Bug 1 — Wire nil services in server.go

**File:** `apps/api/internal/http/server.go` lines ~350-365

These services have complete implementations but are `nil`:
- `clusterManager` (382 lines — clustermanager/service.go)
- `evacuationPlanner` (325 lines — evacuationplanner/service.go)
- `migrationService` (312 lines — migration/service.go)
- `reservationManager` (189 lines — reservations/manager.go)
- `recoveryCoordinator` (349 lines — recovery/coordinator.go)
- `observability` (179 lines — observability/service.go)
- `heartbeatMonitor` (319 lines — heartbeatmonitor/service.go)

**Fix:** Initialize services + event bus when `cfg.Store != nil`. Pass daemon client and scheduler where needed.

**Endpoints that will start working:** server create/delete/power, evacuations, migrations, recovery plans, reservations, timeline, heartbeat history, health history, capacity queries (~15 endpoints).

### Bug 2 — Fix compose.yml context paths

**File:** `deploy/compose.yml`
Fix: `../../forge` → `../apps/api`, `../../beacon` → `../apps/daemon`

### Bug 3 — Fix Dockerfile Go versions

**Files:** `apps/api/Dockerfile`, `apps/daemon/Dockerfile`
Fix: `golang:1.26-alpine` → `golang:1.24-alpine` (actual available version)

### Bug 4 — Guard admin seed behind dev mode

**File:** `apps/api/internal/store/store.go`
Fix: Only seed `admin@example.com`/`admin123` when `APP_ENV=development`.

### Bug 5 — Graceful degradation for nil services

**File:** `apps/api/internal/http/server.go`
Fix: Routes with nil dependencies should return 503, not panic.

---

## STAGE 2: API CONTRACTS DOC (Phase 6)

Create `docs/api/API_CONTRACTS.md` — document every endpoint with request/response/errors/auth.

Organize into Pterodactyl's three-API pattern:
- `/api/client/` — user-facing (console, files, backups, schedules, databases, subusers)
- `/api/application/` — admin CRUD (users, nodes, servers, eggs, allocations, etc.)
- `/api/remote/` — daemon communication (sftp auth, server configs, heartbeats)

---

## STAGE 3: BACKEND CONVERSION (Phase 4)

### Route reorganization

Split handlers into Pterodactyl's pattern:
- `handlers_client.go` — user-facing server operations
- `handlers_admin.go` — admin CRUD
- `handlers_remote.go` — daemon communication (keep)
- `handlers_auth.go` — auth (keep)

### Service patterns to convert

| Missing | Action |
|---|---|
| Server suspension with rollback | Add suspension handler |
| Build modification with Wings sync | Add to clusterManager |
| Variable validation (Rules parsing) | Add validator service |
| Env map building | Parity check daemon template rendering |

### Phase 4 immediate checklist

- [ ] Guard admin seed behind `APP_ENV`
- [ ] Initialize all services
- [ ] Return 503 for routes with nil services
- [ ] Fix duplicate migration naming
- [ ] Move webhook migration to main migrations dir
- [ ] Remove plaintext token fallback in daemon auth

---

## STAGE 4: DAEMON HARDENING (Phase 5)

### Container security gaps vs Wings

| Gap | Fix |
|---|---|
| Only 3 caps dropped (Wings: ALL) | Drop ALL capabilities, add back only needed |
| No `no-new-privileges` | Set `NoNewPrivileges: true` |
| No read-only rootfs | Set `ReadonlyRootfs: true` |
| Runs as root | Add UID:GID mapping (non-root user) |
| No tmpfs /tmp | Add tmpfs with size limit |
| No log limits | Add json-file log driver with rotation |
| No PID limits | Set PidsLimit |
| Disk quota via filesystem walk (O(n)) | Use Docker volume size limits |

### Missing Wings features

- S3 backup storage (add adapter)
- Backup retention/cleanup (auto-delete on limit)
- Backup locking (lock flag)
- Structured logging (replace fmt.Fprintf)

---

## STAGE 5: CORE FEATURES PARITY (Phase 7)

Execute in Pterodactyl feature order:

1. **Auth + setup wizard** — no default admin, first-run wizard, auth guard
2. **Users + permissions** — already works
3. **Nodes + allocations** — already works
4. **Eggs/nests** — already works (add variable Rules validation)
5. **Server create** — broken (wire clusterManager)
6. **Server install** — works
7. **Power controls** — broken (wire clusterManager)
8. **Console WS** — works
9. **File manager** — works
10. **Startup variables** — works
11. **Databases** — CRUD works, provisioning returns 501 (must implement)
12. **Backups** — local works, missing: locking, S3
13. **Schedules** — works, missing: task chaining
14. **SFTP** — works
15. **Activity logs** — works

### Acceptance test

A user must be able to:
install panel → create admin → add node → register daemon → create allocation → create server → install server → start server → open console → send command → edit files → use SFTP → create backup → restore backup → stop/delete server

---

## STAGE 6: FRONTEND REBUILD (Phase 8)

### Immediate fixes
- Remove hardcoded admin login comment
- Add auth guard component
- Add role-aware navigation
- Consolidate duplicate components (file-manager, backup-list, database-list → their -view counterparts)
- Fix mixed theme (server nav uses light, admin uses dark)
- Fix SFTP page connection details

### Structure
```
apps/panel/
├── app/
│   ├── login/          # Already exists
│   ├── setup/          # NEW — first-run wizard
│   ├── server/[id]/    # 9 sub-pages — all exist
│   └── admin/          # All pages exist
├── components/
│   ├── layout/         # Auth guard + nav
│   ├── server/         # Conslidate
│   ├── admin/
│   └── ui/
├── lib/
│   ├── api/            # Split large api.ts
│   ├── auth/
│   └── websocket/
└── stores/
```

---

## STAGE 7: ADVANCED ORCHESTRATION (Phase 9)

**ONLY AFTER core panel works end-to-end.**

Bring back unique features behind feature flags:
```
ORCHESTRATION_SCHEDULER_ENABLED=true
ORCHESTRATION_MIGRATION_ENABLED=true
ORCHESTRATION_EVACUATION_ENABLED=true
ORCHESTRATION_RECOVERY_ENABLED=true
```

Services already written — just need wiring and flags.

---

## STAGE 8: MULTI-DB (Phase 10)

After PostgreSQL is stable:
1. Abstract repositories behind interfaces
2. Add MySQL migration folder
3. Add SQLite migration folder (dev only)
4. Config-driven driver selection: `DB_DRIVER=postgres|mysql|sqlite`

---

## STAGE 9: DEPLOYMENT HARDENING (Phase 11)

- Fix compose.yml + Dockerfiles
- Create panel Dockerfile
- Security checklist (hashed tokens, rate limiting, CSRF, audit logs, RBAC, backup encryption)
- Backup/restore docs, upgrade docs

---

## STAGE 10: COMPETITOR-BEATING (Phase 12)

Only after production stable:
bulk ops → node drain → capacity reservation → placement simulation → multi-region → health scores → alerting → cost dashboard → plugins → cluster expansion

---

## CRITICAL RULE

Do not add orchestration features until the 15-step acceptance test passes end-to-end.
