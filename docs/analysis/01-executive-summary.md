# 01 — Executive Summary

---

## What GamePanel Is

GamePanel is an ambitious reimagination of Pterodactyl Panel. It replaces the PHP/Laravel stack with:

- **`forge/api`** — Go + Fiber v2 REST API backed by PostgreSQL
- **`forge/web`** — Next.js 15 + React 19 frontend
- **`beacon`** — Go daemon (replacing Wings) managing Docker containers, SFTP, backups, and transfers

Beyond Pterodactyl parity, the project adds cloud-orchestration concepts found in no reference project: placement engines, capacity reservations, evacuation planners, migration state machines, a recovery coordinator, a heartbeat monitor, and a reconciliation loop.

---

## TL;DR Verdict

| Component | State |
|---|---|
| Database schema | ✅ Production-grade. 47 tables, 36 migrations, richer than all references |
| API store layer | ✅ Production-grade. 47 store files, complete SQL |
| API auth system | ✅ Strong. 3-method auth chain, OAuth2, node HMAC, 2FA backend |
| API route surface | ⚠️ 189 / 222 routes functional. 33 panic at runtime |
| Service implementations | ⚠️ All 12 services are well-written. Only 2 are wired into the running binary |
| Frontend — Admin | ✅ ~90% complete and correct |
| Frontend — Server UI | ⚠️ ~60% complete. Files page uses wrong component. No user dashboard. No logout |
| Daemon (beacon) | ❌ Will not compile. Core Docker + SFTP work. Backup/S3 broken. No TLS |
| Orchestration layer | ⚠️ All services implemented, none reachable through the API |

---

## The Single Biggest Problem

`forge/api/cmd/api/main.go` instantiates **2 of 12 services**. The other 10 are declared as `nil` and passed directly to route handlers:

```go
// main.go — what IS wired:
nodeRegistry := noderegistry.New(store)   ✅
nodeProbe    := nodeprobe.New(...)        ✅

// Everything else is nil — panics at runtime:
clusterManager      = nil  → POST /servers, POST /servers/:id/power, DELETE /servers/:id
evacuationPlanner   = nil  → POST /nodes/:id/evacuation-plan
migrationService    = nil  → POST /migrations
reservationManager  = nil  → GET /reservations
recoveryCoordinator = nil  → POST /recovery-plans
observabilitySvc    = nil  → GET /timeline, GET /nodes/:id/heartbeats

// Never started at all:
heartbeatMonitor.Start(ctx)   // nodes never classified offline
reconciler.Start(ctx)         // desired vs actual state never reconciled
reservations.Manager.Start()  // reservations never auto-expired
events.Registry               // event bus never created; zero event propagation
```

**Impact:** The most critical operations — creating a server, sending a power signal, deleting a server — hit a nil pointer and panic. The entire orchestration layer is unreachable dead code despite being fully implemented.

---

## Critical Issues (Must Fix Before Any Feature Work)

| # | Issue | Location | Impact |
|---|---|---|---|
| 1 | Missing `golang.org/x/sync` in `go.mod` | `beacon/go.mod` | Beacon will not compile |
| 2 | 10 service pointers are nil in `NewServer` | `forge/api/internal/http/server.go` | ~33 routes panic |
| 3 | `events.Registry` never instantiated | `forge/api/cmd/api/main.go` | Zero inter-service communication |
| 4 | `heartbeatMonitor`, `reconciler`, `reservations.Manager` never started | `main.go` | Node states never updated, servers never reconciled |
| 5 | `deleteBackup` handler written but not registered in HTTP mux | `beacon/internal/server/server.go` | Backup deletion silently unavailable |
| 6 | S3 backup client field commented out | `beacon/internal/backup/s3.go` | S3 adapter completely non-functional |
| 7 | `notifyPanelInstallStatus` is a stub | `beacon` | Panel never learns when installs complete |
| 8 | File manager page uses wrong component | `forge/web/app/server/[id]/files/page.tsx` | Monaco editor implemented but unreachable |
| 9 | No 2FA login checkpoint UI | `forge/web` | 2FA-enabled users are permanently locked out |
| 10 | No logout button anywhere in the UI | `forge/web` | Users cannot sign out |

---

## What Is Genuinely Excellent

1. **Store layer** — 47 store files with correct, complete SQL. This is production-quality work.
2. **Service architecture** — All 12 services are well-designed, use proper interfaces, and are testable in isolation.
3. **Wings API compatibility** — The remote API surface is Pterodactyl-compatible and correctly HMAC-signed.
4. **Orchestration concepts** — Evacuation planner, recovery coordinator, placement engine, and 5-state heartbeat classifier are sophisticated features that exceed all reference projects.
5. **Auth system** — Custom HMAC-JWT + OAuth2 `client_credentials` + API keys + node tokens is thorough.
6. **Admin frontend** — Nodes, servers, users, allocations, nests/eggs, mounts, webhooks, settings — all correct and complete.
7. **Schema** — 36 migrations covering orchestration concepts (regions, evacuations, migrations, recovery, reservations) that have no Pterodactyl equivalent.
