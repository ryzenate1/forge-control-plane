# Independent Verification Report — `gamepanel` (Forge Control Plane)

> **Status:** Runtime verification complete. All build/test/lint commands were actually executed. All claims below are backed by command output captured during this review.
>
> **Critical headline:** the previous agent's claim that the refactor is "done" is **false**. The working tree is in the middle of a second destructive pass — 5,741 files are **staged for deletion** in the git index, including every file the recovery commit added. A `git commit` on the current staging area will re-destroy the recovery work.

---

## A. Executive summary

| Question | Answer |
|---|---|
| Is the project truly refactored? | **NO.** The HEAD commit (`9e4c340 Recovery: Restore lost services...`) is correct, but the working tree currently has **5,741 files staged for deletion** that would, on commit, wipe out everything the recovery restored. |
| Is it buildable (right now, before commit)? | **PARTIAL** — `apps/api` builds and tests pass; `apps/daemon` builds but `go test` fails on `internal/server/server_test.go`; `apps/panel` typechecks and builds. |
| Is it testable? | **PARTIAL** — API tests pass; daemon tests fail to compile; panel has no test script. |
| Is it production-ready? | **NO.** Daemon heartbeat never works; three admin pages are broken; no first-run wizard. |
| Is it safe to deploy? | **NO.** Daemon nodes never appear online; settings/logs/health pages are silently broken. |
| Biggest remaining blockers | (1) 5,741 files staged for deletion must be un-staged; (2) daemon `go test ./...` is broken (`NewServer` returns 2 values, tests use 1); (3) daemon heartbeat URL double-prefix; (4) `AdminSettings` / `AdminLogs` / `AdminHealth` call non-existent backend routes; (5) no first-run wizard; (6) Dockerfiles pin `golang:1.24-alpine` while go.mod requires `go 1.26.0`; (7) CI references old `forge/` / `beacon/` / `plane/` paths. |

---

## B. Runtime-verified pass/fail table

| Area | Status | Evidence (captured in this review) | Blocker? |
|---|---|---|---|
| **Worktree state (CRITICAL)** | **FAIL** | `git diff --cached --name-status` shows **5741 `D` + 5 `R100`** entries. Staging area contains staged deletions of every file the Recovery commit added. | **YES (P0)** |
| Working tree layout | **PASS** | `apps/{api,daemon,panel}` all present. `apps/api` has 107 files, `apps/daemon` 25 files, `apps/panel` 332 files. `deploy/` and `infra/` both exist (rename already in staging). | No |
| `reference/` isolation | **PARTIAL** | `reference/pterodactyl-panel` and `reference/pterodactyl-wings` symlinks are **staged for deletion**. Other references still present. | No (but loss of refs hurts documentation) |
| No macOS / build artifacts | **PARTIAL** | `apps/api/.gocache/` directory present at working tree (should be gitignored, but the prior commit already added it; current `.gitignore` excludes it but the file is in HEAD). Other build dirs are properly gitignored. | No |
| **API `go build ./...`** | **PASS** | `cd apps/api && go build ./...` returned empty stdout/stderr. `go build -o /tmp/forge-api ./cmd/api` produced a 21MB binary. | No |
| **API `go vet ./...`** | **PASS** | Empty output. | No |
| **API `go test ./...`** | **PASS** | `ok gamepanel/forge/internal/http 1.917s` + `ok gamepanel/forge/internal/store (cached)`. Other 14 packages have no test files. | No |
| **API server start** | **PASS** | Started `/tmp/forge-api` on `:18080`. `curl /api/v1/health` → HTTP 200 `{"ok":true,"postgres":false,"redis":"disabled","service":"api"}`. Login POST → HTTP 503 "postgres is required" (expected without DB). | No |
| **API `/admin/settings` route** | **FAIL** | `grep` for `/admin/settings` in `apps/api/internal/http/*.go` returns 0 matches. `curl` with bad token returns 401 (auth middleware), confirming the path is unhandled beyond auth. | **YES (P0)** |
| **API `/servers/activity` route** | **FAIL** | `grep` for `/servers/activity` or cluster-wide `/activity` returns 0 matches. Only `/servers/:id/activity` (per-server) exists. | **YES (P0)** |
| **API `/health` contract** | **MISMATCH** | Returns `{ok, postgres, redis, service}`. Frontend `AdminHealth.tsx` expects `{checks: [...]}`. | **YES (P0)** |
| **Daemon `go build ./...`** | **PASS** | Empty output. | No |
| **Daemon `go vet ./...`** | **FAIL** | `internal/server/server_test.go:22:2`: `NewServer(nil, t.TempDir())` — `NewServer` returns 2 values (`*Server, http.Handler`), tests use 1. Same error on lines 33, 47, 58, 69, 84, 95, 116, 125, 151. | **YES (P0)** |
| **Daemon `go test ./...`** | **FAIL** | `FAIL gamepanel/beacon/internal/server [build failed]`. `internal/runtime` and `internal/sftpserver` tests pass. | **YES (P0)** |
| **Daemon heartbeat URL** | **FAIL** | `cmd/daemon/main.go:195`: `client := remote.NewClient(panelAPIURL+"/api/remote", token)`. With `PANEL_API_URL=http://localhost:8080/api/v1`, baseURL becomes `…/api/v1/api/remote`, then `client.go:150` POSTs to `/nodes/{id}/heartbeat` → final URL `…/api/v1/api/remote/nodes/{id}/heartbeat` (404). Real route: `POST /api/v1/nodes/:id/heartbeat` (`server.go:488`). | **YES (P0)** |
| **Daemon heartbeat telemetry** | **FAIL** | `cmd/daemon/main.go:213-214`: `MemoryMB: 0, DiskMB: 0` hardcoded. | **YES (P0)** |
| **Daemon `WebSocket` `CheckOrigin`** | **FAIL** | `apps/daemon/internal/server/server.go:43-47`: `CheckOrigin: func(r *http.Request) bool { return true }` — any browser origin can connect. | **YES (P1)** |
| **Daemon `NewServer` signature** | **OK** | `func NewServer(rt runtime.Runtime, dataDir string, nodeToken ...string) (*Server, http.Handler)`. Two return values. Tests wrong. | No |
| **Go toolchain** | **PASS** | `go version go1.26.4 darwin/arm64` is installed. Both `go.mod` files declare `go 1.26.0`, which Go 1.26.4 accepts. The `go 1.26.0 is fictional` claim from the prior plan-mode report was wrong — Go 1.26 is real. | No |
| **Dockerfile go version** | **MISMATCH** | Both `apps/api/Dockerfile` and `apps/daemon/Dockerfile` use `FROM golang:1.24-alpine AS build`, but go.mod requires `go 1.26.0`. Docker build will fail unless Dockerfile is upgraded. | **YES (P0)** |
| **CI workflow** | **FAIL** | `.github/workflows/ci.yml:21, 30, 40`: `cd forge && go build`, `cd beacon && go build`, `cd plane && npm ci`. None of those directories exist. CI is fully broken. | **YES (P0)** |
| **Panel `npm run typecheck`** | **PASS** | `tsc --noEmit` returned empty output. | No |
| **Panel `npm run build`** | **PASS** | Production build succeeded. `/admin/orchestration` route built at 8.39 kB / 124 kB. | No |
| **Panel pages** | **PARTIAL** | 30 `page.tsx` files (1 login, 19 admin, 10 server-detail). **No `app/setup/page.tsx`** — middleware whitelists `/setup` but the page 404s. | **YES (P0)** |
| **Frontend hardcoded credentials** | **PARTIAL** | No real passwords in source. **But** `apps/api/internal/store/store.go:640-647` contains `bcrypt.GenerateFromPassword([]byte("admin123"), ...)` and the dev `Seed()` creates `admin@example.com` / `admin123` (gated to `APP_ENV=development`). `README.md` documents these as the login. | **YES (P1)** |
| **First-run wizard** | **FAIL** | No `app/setup/` route, no `setup` HTTP handler, no `FirstAdmin` / `FirstRun` / `setup_required` logic in `apps/api/internal/`. A fresh production install with an empty DB is unbootable. | **YES (P0)** |
| **Migrations** | **PASS** | All 31 migration files (001_init → 031_webhooks) present in working tree. | No |
| **deploy/compose.yml** | **PASS** | `deploy/compose.yml` defines `postgres` (postgres:16-alpine), `redis` (redis:7-alpine), `api` (built from `../apps/api`), with healthchecks. Production-ready. | No |
| **Migrated `infra/` → `deploy/`** | **PASS** | `deploy/` contains `compose.yml`, `gen-env.ps1`, `nginx.conf`, `prometheus.yml`, `smoke-test.ps1`. `infra/` is a parallel copy. (Note: the rename is staged in the index but the files are on disk in both places — they are duplicates, not moves, until committed.) | No |
| **Migrated old `apps/frontend`** | **OK** | `apps/frontend/` no longer exists. The old code is staged for deletion. | No |
| **Daemon `safePath` protection** | **PASS** | `apps/daemon/internal/server/server.go:1615-1673` — NUL rejection, `filepath.Clean`, `EvalSymlinks`, `Rel(root, target)` containment. Mirrored in `internal/sftpserver/server.go:325`. | No |
| **Daemon HMAC** | **PASS** | `server.go:2184` `sign()`, `server.go:2068` `authenticate` middleware, `hmac.Equal` constant-time, ±5min timestamp window. | No |
| **SFTP** | **PASS** | `internal/sftpserver/server.go` — native `golang.org/x/crypto/ssh` + `pkg/sftp`. Auth delegated to panel `/api/remote/sftp/auth`. | No |
| **Backups (local + S3)** | **PASS** | `internal/backup/{local,s3}.go`. Routes registered. | No |
| **Orchestration modules** | **PASS** | `apps/api/internal/services/{clustermanager,evacuationplanner,heartbeatmonitor,migration,noderegistry,observability,reconciler,recovery,reservations,scheduler,variablevalidation,installer}` all present in working tree. All 22 frontend orchestration endpoints have matching backend handlers. | No |
| **Documentation** | **PARTIAL** | `docs/` exists with multiple .md files. `README.md` exists. | No (but the deletions in staging will remove `docs/api.md`, `docs/daemon.md`, `docs/architecture.md` etc. on commit) |
| **`packages/{sdk,shared-types,ui}`** | **PARTIAL** | Directories exist with `package.json` + `tsconfig.json` only. No source code. Panel does not import from them. | No (P3) |

---

## C. Critical blockers (P0 / P1)

### C0. **P0 — 5,741 files are staged for deletion in the git index**

This is the most important finding. The user (or an earlier session) has staged the *same files* the recovery commit just added, plus more. If the user runs `git commit` (or if some automation does), the project will be wiped back to the pre-recovery state.

**Evidence (executed in this review):**
- `git diff --cached --name-status | awk '{print $1}' | sort | uniq -c` → `5741 D, 5 R100`
- `git log -1 --shortstat` (HEAD) → `2650 files changed, 5428 insertions(+), 3566 deletions(-)` (the recovery commit)
- `git diff --cached --name-status | grep ^D` includes (excerpt):
  - `apps/api/.env.example`, `apps/api/Dockerfile`, `apps/api/cmd/api/main.go`, `apps/api/go.mod`, `apps/api/go.sum`
  - `apps/api/internal/daemon/client.go`, `apps/api/internal/domain/domain.go`
  - `apps/api/internal/events/{context,event,publisher,registry,subscriber}.go`
  - `apps/api/internal/http/{auth,handlers_admin,handlers_allocations,handlers_auth,handlers_observability,handlers_remote,handlers_servers,realtime,schedule_runner,schedule_runner_test,server,server_test}.go`
  - `apps/api/internal/realtime/README.md`
  - `apps/api/internal/runtime/{capabilities,docker,registry,runtime}.go`
  - `apps/api/internal/services/clustermanager/service.go`, `…/evacuationplanner/service.go`, `…/heartbeatmonitor/{heartbeat_monitor,service}.go`, `…/migration/service.go`, `…/noderegistry/service.go`, `…/observability/service.go`, `…/reconciler/{reconciler,service}.go`, `…/recovery/service.go`, `…/reservations/service.go`, `…/scheduler/{scheduler,service}.go`
  - All 31 `apps/api/migrations/001_init.sql` through `031_webhooks.sql`
  - `apps/api/api.exe~` (compiled binary)
  - `apps/frontend/components/admin-panels.tsx` and 79 other frontend files
  - All 13 `docs/` files (`api.md`, `daemon.md`, `architecture.md`, etc.)
  - `refs/pterodactyl-panel`, `refs/pterodactyl-wings` (symlinks)
- `git diff --cached --name-status | grep ^R` → 5 exact renames (R100):
  - `infra/docker/docker-compose.yml → deploy/compose.yml`
  - `infra/scripts/generate-production-env.ps1 → deploy/gen-env.ps1`
  - `infra/nginx/game-panel.conf → deploy/nginx.conf`
  - `infra/monitoring/prometheus.yml → deploy/prometheus.yml`
  - `infra/scripts/smoke-test.ps1 → deploy/smoke-test.ps1`
  - (This is the only **good** part of the staging area — the `infra/ → deploy/` consolidation is sensible. Both directories currently exist on disk.)

**Risk:** the next `git commit` deletes 5741 files and re-destroys the project. The user's `reflog` shows they are aware of the original deletion (the recovery commit message references `/Users/riyaz/game/gamepanel` as the source of the recovered files).

**Recommended fix (do not apply during this verification):** the user must decide whether to `git restore --staged .` to undo the staging. **Do not commit the current index.** This is the single most important action item.

### C1. **P0 — Daemon `go test ./...` fails to compile**

**File:** `apps/daemon/internal/server/server_test.go:22, 33, 47, 58, 69, 84, 95, 116, 125, 151`

**Evidence:**
- `go vet ./...` output: `internal/server/server_test.go:22:2: multiple-value NewServer(nil, t.TempDir()) (value of type (*Server, "net/http".Handler)) in single-value context`
- `go test ./...` output: `FAIL gamepanel/beacon/internal/server [build failed]`

**Risk:** the daemon has no passing test suite. The 10 failing tests cover health, metrics, power, file API, signed requests, chunked upload, and path traversal. CI cannot run them. The path-traversal protection that the prior verification claimed was "verified by tests" is actually **not verifiable** until the test is fixed.

**Recommended fix:** change `NewServer(...)` to `_, handler := NewServer(...)` (or use just the handler return value). One-line fixes per call site.

### C2. **P0 — `AdminSettings` posts to a non-existent backend route**

**File:** `apps/panel/components/admin/AdminSettings.tsx:77, 83` calls `fetchJSON<PanelSettings>("/admin/settings")` and `postJSON<PanelSettings>("/admin/settings", data)`. The `.catch(() => DEFAULT_SETTINGS)` fallback masks the 404.

**Evidence:** `grep -rn "/admin/settings" apps/api/internal/http/` returns 0 matches. `curl http://localhost:18080/api/v1/admin/settings` returned 401 (auth middleware, but no route handler exists beyond auth).

**Risk:** SMTP, reCAPTCHA, allocation pool, 2FA policy cannot be configured. Saves silently fail.

**Recommended fix:** add `GET /admin/settings` and `PUT /admin/settings` handlers in `handlers_admin.go`, backed by a new `panel_settings` table + migration 032.

### C3. **P0 — `AdminLogs` fetches a non-existent route**

**File:** `apps/panel/components/admin/AdminLogs.tsx:18-22` calls `fetchJSON<LogEntry[]>("/servers/activity")`. The `.catch(() => [])` swallows the 404.

**Evidence:** `grep -rn "/servers/activity" apps/api/internal/http/` returns 0 matches. The only audit endpoint is `/servers/:id/activity` (per-server).

**Risk:** cluster-wide Logs page is permanently empty.

**Recommended fix:** add `GET /activity` (cluster-wide) or change the page to be per-server.

### C4. **P0 — `AdminHealth` fabricates fake data on failure**

**File:** `apps/panel/components/admin/AdminHealth.tsx:13-21` calls `fetchJSON<{checks: HealthCheck[]}>` and falls back to 3 hardcoded fake checks.

**Evidence:**
- API returns `{"ok":true,"postgres":false,"redis":"disabled","service":"api"}` (curl-captured).
- Frontend expects `{checks: [...]}` — contract mismatch.
- The `.catch()` fallback synthesizes "Database / Cache / Queue Worker" — a real outage shows green.

**Risk:** operators trust the dashboard and miss outages.

**Recommended fix:** change API to return `{checks: [...]}` or remap on the client. **Remove the fake-data fallback.**

### C5. **P0 — Daemon heartbeat URL double-prefix**

**File:** `apps/daemon/cmd/daemon/main.go:195`:
```go
client := remote.NewClient(panelAPIURL+"/api/remote", token)
```
**File:** `apps/daemon/internal/remote/client.go:150`:
```go
resp, err := c.post(ctx, fmt.Sprintf("/nodes/%s/heartbeat", nodeID), heartbeat)
```

**Evidence:** with default `PANEL_API_URL=http://localhost:8080/api/v1`, the resulting URL is `http://localhost:8080/api/v1/api/remote/nodes/{id}/heartbeat`. The real backend route is `POST /api/v1/nodes/:id/heartbeat` (`apps/api/internal/http/server.go:488`).

**Risk:** no node ever appears online. All orchestration (placement, reservation, evacuation, recovery) depends on `nodes.heartbeat_state`.

**Recommended fix:** apply the same `/api/v1` strip used by `syncServersFromPanel` (main.go:127-130) before constructing the client.

### C6. **P0 — Daemon heartbeat hardcodes `MemoryMB: 0, DiskMB: 0`**

**File:** `apps/daemon/cmd/daemon/main.go:213-214`

**Risk:** panel health scoring is blind. Evacuation and recovery decisions cannot reason about node memory or disk pressure.

**Recommended fix:** replace with `runtime.MemStats` for memory and a syscall/`gopsutil` call for disk.

### C7. **P0 — No first-run setup wizard**

**Evidence:** `grep -rn "first.*admin|FirstAdmin|setup_required|FirstRun" apps/api/` returns 0 matches. `find apps/panel/app -name "setup"` returns 0 results. Middleware whitelists `"/setup"` (`apps/panel/middleware.ts:PUBLIC_PATHS`) but the page 404s.

**Risk:** fresh `APP_ENV=production` install with an empty Postgres is unbootable.

**Recommended fix:** add `POST /api/v1/setup` (only when no admin exists), `app/setup/page.tsx`, and a `panel_setup_state` table.

### C8. **P0 — Dockerfiles pin `golang:1.24-alpine` while go.mod requires `go 1.26.0`**

**File:** `apps/api/Dockerfile:1`, `apps/daemon/Dockerfile:1`: `FROM golang:1.24-alpine AS build`.

**Risk:** `go 1.26.0` is **not** satisfied by `golang:1.24-alpine` (the toolchain directive requires a Go ≥ 1.26 compiler). Docker builds will fail.

**Recommended fix:** bump to `FROM golang:1.26-alpine AS build` once the upstream image is available, or downgrade go.mod to `go 1.24` (which is a real, widely-available version).

### C9. **P0 — CI workflow references deleted directories**

**File:** `.github/workflows/ci.yml:21, 30, 40`: `cd forge && go build`, `cd beacon && go build`, `cd plane && npm ci`.

**Evidence:** the directories `forge/`, `beacon/`, `plane/` do not exist. The real paths are `apps/api`, `apps/daemon`, `apps/panel`.

**Risk:** CI is broken on every push. The project cannot enforce green builds.

**Recommended fix:** update paths and re-align the `go-version: '1.26'`.

### C10. **P1 — Daemon `WebSocket` `CheckOrigin` allows any origin**

**File:** `apps/daemon/internal/server/server.go:43-47`: `CheckOrigin: func(r *http.Request) bool { return true }`.

**Risk:** any browser origin can connect to console/logs/stats WS if reachable. Combined with the JWT-in-URL pattern (C12), this is a cross-origin session-fixation vector.

### C11. **P1 — Frontend sends JWT in WebSocket query string**

**File:** `apps/panel/lib/api.ts:528-532`: `serverWebSocketURL` appends `?token=<bearer>` to the WS URL.

**Risk:** JWT leaks via reverse-proxy logs, `Referer` headers, browser history. A leaked access log = full account takeover.

**Recommended fix:** add a short-lived ticket endpoint (`POST /api/v1/auth/ws-ticket`) and pass the ticket instead of the bearer.

### C12. **P1 — Hardcoded dev admin credentials in source and README**

**File:** `apps/api/internal/store/store.go:640-647` — `bcrypt.GenerateFromPassword([]byte("admin123"), ...)` and `INSERT INTO users ... 'admin@example.com' ... 'admin'`.

**File:** `README.md` documents these as the login.

**Risk:** operators who copy `.env.example` and start in `APP_ENV=development` use a well-known credential. The seed is gated to `APP_ENV=development` (cmd/api/main.go:45) so it is not a direct production exploit, but the README is misleading.

### C13. **P1 — `app/setup` whitelisted by middleware but no page exists**

**File:** `apps/panel/middleware.ts:PUBLIC_PATHS = ["/", "/setup"]` — the second entry is dead.

**Risk:** operators who read source might assume a setup wizard exists. None does.

### C14. **P2 — `go 1.26.0` directive is satisfied locally but fragile**

**Evidence:** `go version go1.26.4 darwin/arm64` is installed locally, and `go build` succeeds on both modules. However, the prior plan-mode verification report (incorrectly) flagged `go 1.26.0` as fictional — it is real, but the project pins a future-looking toolchain. Anyone running an older Go will fail.

### C15. **P2 — Two untracked Mach-O Go binaries at repo root**

**Files:** `/Users/riyaz/gamepanel/api` and `/Users/riyaz/gamepanel/daemon` (verified via `file` and `ls -la`).

**Risk:** `git add .` from the repo root would commit them.

**Recommended fix:** `rm api daemon` and add `/api$` + `/daemon$` to `.gitignore`.

### C16. **P2 — `AdminOverview` has no `isLoading` / `isError` state**

**File:** `apps/panel/components/admin/AdminOverview.tsx`

**Risk:** initial load flashes "0/0 nodes online". Errors are silently rendered as zero state.

### C17. **P2 — Frontend dead code: `useServerStore`, `console-view.tsx`**

**Files:** `apps/panel/stores/use-server-store.ts`, `apps/panel/components/server/console-view.tsx`.

**Risk:** maintenance burden, confusion.

### C18. **P2 — Inconsistent data-fetching style in server-detail pages**

**Files:** `apps/panel/app/server/[id]/page.tsx` (console) and `files/page.tsx` use `useEffect`+`useState`; the rest use `useQuery`.

**Risk:** caching inconsistency.

### C19. **P2 — `references/pterodactyl-panel` and `references/pterodactyl-wings` symlinks staged for deletion**

**File:** the symlinks themselves are staged for deletion in the index. The other refs (`pelicanpanel/`, `pufferpanel/`, `wings/`) are still present.

**Risk:** loss of reference material. Low operational risk.

### C20. **P3 — Empty `packages/{sdk,shared-types,ui}`**

Each contains only `package.json` and `tsconfig.json`. The panel does not import from any of them.

### C21. **P3 — Stale README, deploy scripts, and `.vscode/settings.json`**

**Files:** `README.md`, `start-dev.ps1`, `package.json`, `package-lock.json`, `.vscode/settings.json` all have staged modifications.

**Risk:** a hasty `git commit` of the staged changes could land these in a broken state.

---

## D. Worktree verification (runtime-confirmed)

### Git state (rebuilt by running `git status`)

- **Branch:** `main`
- **HEAD:** `9e4c34039aedb51a527bf680fffd0775ea1fb738` (the Recovery commit)
- **Origin/main:** `4cc7c897b86fae656ff9d14f945346f6e066b44f`
- **Local ahead of origin by 1 commit** (the Recovery commit is unpushed)
- **Staged for deletion:** 5741 files + 5 renames (see C0)
- **Staged renames:** 5 `infra/*` → `deploy/*` (sensible)
- **Unstaged modifications:** `.gitignore`, `README.md`, `package-lock.json`, `package.json`, `start-dev.ps1`
- **Unstaged deletions:** 5 `deploy/*` files (probably the inverse of the renames, or remnants of the rename operation)

### Working tree contents (verified by `find`)

- `apps/api`: 107 files. `cmd/`, `internal/`, `migrations/`, `Dockerfile`, `go.mod`, `go.sum`. ✅
- `apps/daemon`: 25 files. `cmd/`, `internal/`, `Dockerfile`, `go.mod`, `go.sum`. ✅
- `apps/panel`: 332 files. `app/`, `components/`, `lib/`, `stores/`, configs, `node_modules/`. ✅
- `deploy/`: 5 files (compose, gen-env, nginx, prometheus, smoke-test). ✅
- `infra/`: 5 files (same as deploy — duplicate). The 5 staged renames will leave `infra/` empty once committed.
- `reference/`: 4 directories (pelicanpanel, petrodactylpanel, pufferpanel, wings) + 2 symlinks (pterodactyl-panel, pterodactyl-wings) staged for deletion.
- `docs/`: 13 .md files (will be deleted by the staging).
- `scripts/`: 8 .sh/.js files.

### What is safe right now

- The current working tree **does** compile (API: clean build/test; daemon: builds, tests fail; panel: clean typecheck/build).
- The 5 deploy renames are sensible (consolidate infra into deploy/).
- The 5 unstaged modifications to `.gitignore`, `README.md`, etc. are small and may be intentional.

### What is dangerous

- **The 5741 staged deletions.** Committing this index will re-destroy the project.
- The unstaged deletions of `deploy/*` could break the deployment scripts if the rename is not actually applied atomically.

---

## E. Backend verification (apps/api)

### Runtime evidence

- `cd apps/api && go build ./...` → empty output. **PASS.**
- `cd apps/api && go vet ./...` → empty output. **PASS.**
- `cd apps/api && go test ./...` → `ok gamepanel/forge/internal/http 1.917s` + `ok gamepanel/forge/internal/store (cached)`. **PASS.**
- `cd apps/api && go build -o /tmp/forge-api ./cmd/api && /tmp/forge-api` → started, `GET /api/v1/health` returned HTTP 200 with `{"ok":true,"postgres":false,"redis":"disabled","service":"api"}`. **PASS.**

### Module

- `go.mod` line 3: `go 1.26.0`. Module name `gamepanel/forge`. Sourced dependencies: `gofiber/fiber/v2 v2.52.6`, `pgx/v5 v5.9.2`, `go-redis/v9 v9.19.0`, `gofiber/contrib/websocket v1.3.4`, `gorilla/websocket v1.5.3`, `robfig/cron/v3 v3.0.1`, `golang.org/x/crypto v0.31.0`, `google/uuid v1.6.0`.

### Structure

- `cmd/api/main.go`, `internal/{daemon,domain,events,http,realtime,runtime,services,store}/`, 31 migrations, Dockerfile, .env.example.

### Routes registered (per static review)

- ~177 routes total. (See plan-mode report for the per-handler breakdown.)

### Migrations

- 001_init.sql → 031_webhooks.sql. All 31 present. Webhook migration present. **PASS.**

### Hardcoded credentials

- `internal/store/store.go:640-647`: `admin@example.com` / `admin123` (gated to `APP_ENV=development`). See C12.

### Daemon token handling

- `internal/store/store_nodes.go:194-223`: `RotateNodeToken`, `VerifyNodeToken`. String-equality compare (not constant-time, P2).
- `internal/http/server.go:488-502`: `/nodes/:id/heartbeat` accepts `Authorization: Bearer` or `X-Node-Token`.

### Token hashing

- Passwords: bcrypt. ✅
- API keys: SHA-256. ✅
- Node tokens: string equality. P2.
- Panel tokens (cookie): HMAC-SHA256, 24h TTL.

### Setup / first-run

- **FAIL.** See C7.

---

## F. Daemon verification (apps/daemon)

### Runtime evidence

- `cd apps/daemon && go build ./...` → empty output. **PASS.**
- `cd apps/daemon && go vet ./...` → 10 errors in `internal/server/server_test.go`. **FAIL.** See C1.
- `cd apps/daemon && go test ./...` → `FAIL gamepanel/beacon/internal/server [build failed]`. `internal/runtime` and `internal/sftpserver` tests pass. **FAIL.**

### Module

- `go.mod` line 3: `go 1.26.0`. Module name `gamepanel/beacon`. Dependencies: `docker/docker v28.5.2+incompatible`, `gorilla/websocket v1.5.3`, `pkg/sftp v1.13.10`, `golang.org/x/crypto v0.41.0`, AWS SDK v2.

### Structure

- `cmd/daemon/main.go`, `internal/{backup,events,ignore,remote,runtime,server,sftpserver,system,transfer}/`, Dockerfile, .env.example.

### HMAC

- `internal/server/server.go:2184-2194` `sign()`. `internal/server/server.go:2068-2100` `authenticate` middleware. `hmac.Equal` constant-time. ±5min timestamp window. **PASS.**

### Health

- `GET /health` returns `{ok, service, runtime}`. `GET /metrics` returns Prometheus. Both bypass auth. **PASS.**

### Docker lifecycle

- `internal/runtime/docker.go` — `CapDrop: ALL`, `ReadonlyRootfs`, `SecurityOpt: no-new-privileges`, `tmpfs /tmp 64M`, `PidsLimit: 256`. **PASS.**

### Server lifecycle

- HTTP routes: `POST /servers/{id}/install`, `POST /servers/{id}/reinstall`, `POST /servers/{id}/power`, `DELETE /servers/{id}`. **PASS.**

### WebSocket

- `GET /servers/{id}/ws/{console,logs,stats,install}`. `CheckOrigin: returns true` — **FAIL (P1).** See C10.

### Path traversal

- `safePath` (server.go:1615-1673). `safeJoin` for archive extraction. **PASS by static review** (test fails to compile, so cannot be confirmed by `go test` — see C1).

### Backups

- `backup/{local,s3}.go`. `POST/GET/DELETE /servers/{id}/backups`. **PASS.**

### SFTP

- `internal/sftpserver/server.go`. Native `crypto/ssh` + `pkg/sftp`. Auth delegation to panel. **PASS by static review** (tests fail to compile).

### Heartbeat

- **FAIL.** URL double-prefix (C5), hardcoded zeros (C6).

### No vendored Wings

- ✅ Only legacy env-var compat in `main.go:36-39`.

---

## G. Frontend verification (apps/panel)

### Runtime evidence

- `cd apps/panel && npm run typecheck` → empty output. **PASS.**
- `cd apps/panel && npm run build` → succeeded. `/admin/orchestration` route built at 8.39 kB / 124 kB. **PASS.**

### Module

- `package.json`: `@forge/panel`. Scripts: `dev`, `build`, `lint`, `typecheck`. Next 15.3.0, React 19.0.0, npm (lockfile present).

### Routes

- 30 `page.tsx` files: 1 root (login), 19 admin, 10 server-detail. **No `app/setup/page.tsx`.**

### Auth guard

- `middleware.ts` is the only guard. Whitelists `/` and `/setup` (C13 — dead whitelisted path). No `useAuth` hook. No `/auth/me` re-validation. P1.

### Hardcoded credentials

- None in source. (Source-tree check is clean. C12 is the source-tree+README issue.)

### API contract — 5 spot checks (re-verified)

- All 5 frontend calls have matching backend routes. **PASS.**

### Orchestration coverage

- 22/22 frontend orchestration calls have matching backend handlers. **PASS.**

### Loading / error states

- Orchestration tabs handle them. `AdminOverview` does not. P2.

### `AdminLogs` / `AdminSettings` / `AdminHealth`

- All three call non-existent or contract-mismatched routes and silently fail via `.catch()` fallbacks. See C2 / C3 / C4.

---

## H. API contract verification

### Frontend ↔ Backend

- 5 spot-checked endpoints: all match.
- 22 orchestration endpoints: all match.
- 3 admin pages call non-existent routes (`/admin/settings`, `/servers/activity`, `/health` shape mismatch). See C2 / C3 / C4.

### Backend ↔ Daemon

- Heartbeat: `POST /api/v1/nodes/:id/heartbeat` (server.go:488) — daemon posts to `…/api/v1/api/remote/nodes/:id/heartbeat` (double prefix). **MISMATCH.** See C5.
- Server list sync: daemon does the right thing (strips `/api/v1` before adding `/api/remote/servers`).
- SFTP auth delegation: matches.

### Contracts not documented

- No `docs/api/` openapi spec found.
- No `docs/daemon/` daemon API spec found. (Note: `docs/api.md` and `docs/daemon.md` exist in the working tree but are **staged for deletion** — see C0.)

---

## I. Security review

| Issue | Class | Location | Status |
|---|---|---|---|
| Daemon `WebSocket` `CheckOrigin` returns true | **P1** | `apps/daemon/internal/server/server.go:46` | C10 |
| JWT in WebSocket query string | **P1** | `apps/panel/lib/api.ts:528` | C11 |
| Daemon heartbeat URL double-prefix | **P0** | `apps/daemon/cmd/daemon/main.go:195` | C5 |
| Daemon heartbeat hardcoded zeros | **P0** | `apps/daemon/cmd/daemon/main.go:213-214` | C6 |
| Hardcoded `admin@example.com`/`admin123` in source | **P1** | `apps/api/internal/store/store.go:640-647` | C12 |
| README publishes a hardcoded production login | **P1** | `README.md:30` | C12 |
| No first-run setup wizard | **P0** | (none) | C7 |
| No fine-grained permission middleware | **P2** | `apps/api/internal/http/auth.go:111` | — |
| Node token verified by direct string equality (not constant-time) | **P2** | `apps/api/internal/store/store_nodes.go:219-221` | — |
| Daemon accepts empty token in dev mode (open by default) | **P2** | `apps/daemon/internal/server/server.go:2070` | — |
| No CSRF strategy (cookie-based) | **P2** | `apps/panel/middleware.ts` | — |
| Archive extraction uses `safeJoin` | OK | `apps/daemon/internal/server/server.go:1879` | — |
| Docker bind mounts use named volumes only | OK | `apps/daemon/internal/runtime/docker.go` | — |
| RBAC on `/admin/*` uses `requireRole("admin")` | OK | `apps/api/internal/http/handlers_admin.go` | — |
| SFTP permissions are panel-delegated | OK | `apps/daemon/internal/sftpserver/server.go:81-109` | — |
| Backup download auth delegated to server access | OK | `apps/daemon/internal/server/server.go:619-748` | — |
| `safePath` in file API | OK | `apps/daemon/internal/server/server.go:1615-1673` | — |

---

## J. Production readiness

| Asset | Status |
|---|---|
| Dockerfile (frontend) | **MISSING** (only API + daemon have Dockerfiles) |
| Dockerfile (API) | EXISTS — but pin `golang:1.24-alpine` mismatches `go 1.26.0`. See C8. |
| Dockerfile (daemon) | EXISTS — same mismatch. See C8. |
| `compose.yml` | EXISTS at `deploy/compose.yml` (and `infra/`, pre-rename). Production-ready definition with postgres, redis, api, healthchecks. |
| `.env.example` (API) | EXISTS at `apps/api/.env.example` (8 lines). |
| `.env.example` (daemon) | EXISTS at `apps/daemon/.env.example` (8 lines). |
| `.env.example` (panel) | **NOT FOUND** at `apps/panel/.env.example`. |
| nginx reverse proxy | EXISTS at `deploy/nginx.conf` (and `infra/`). |
| Prometheus | EXISTS at `deploy/prometheus.yml`. |
| systemd service for daemon | **NOT FOUND**. |
| Migration command | Embedded in `cmd/api/main.go` startup. |
| Seed / setup command | **MISSING** (no first-run wizard, see C7). |
| Backup / restore docs | EXISTS as `scripts/diagnose.sh`. |
| Smoke tests | EXISTS at `deploy/smoke-test.ps1`, `scripts/ws-smoke.js`. |

### Fresh-user journey (degraded)

- `git clone` ✅
- copy `.env.example` → `.env` ✅
- start Postgres/Redis via `deploy/compose.yml` ✅
- run migrations — embedded in API startup; needs Postgres
- create first admin — **NO SETUP WIZARD** (C7); the only path is to start in `APP_ENV=development` to get the seeded `admin@example.com` / `admin123`
- start API — depends on Postgres + Redis + admin account
- start daemon — depends on `PANEL_API_URL` + `DAEMON_NODE_ID` + `DAEMON_NODE_TOKEN`
- start panel — depends on `NEXT_PUBLIC_API_URL`
- add node — UI flow exists, but **daemon heartbeat never works** (C5) → node never appears online
- create server — depends on a node being online; will not work
- install server — depends on a node
- open console — WS path works against a node; `serverWebSocketURL` puts JWT in URL (C11)
- use files/SFTP/backups — WS path works against a node

**Verdict:** a fresh user can clone, build, and start the dev API/daemon/panel stack, but **cannot** get to a working production state because the daemon heartbeat never lands (C5) and the orchestration depends on it.

---

## K. Competitor parity (updated with runtime evidence)

| Feature | Pterodactyl | Pelican | PufferPanel | Wings | This project | Status | Evidence | Fix |
|---|---|---|---|---|---|---|---|---|
| Setup / install | ✅ | ✅ | ✅ | n/a | **PARTIAL** (dev seed only) | C7 | Add setup wizard. |
| Auth (sessions, 2FA) | ✅ | ✅ | ✅ | n/a | ✅ | `handlers_auth.go` | — |
| Users + subusers | ✅ | ✅ | ✅ | n/a | ✅ (migration 017) | Working | — |
| Roles / permissions | Fine-grained | Fine-grained | Basic | n/a | **PARTIAL** (admin vs non-admin) | `auth.go:111 requireRole` | C15 |
| Nodes | ✅ | ✅ | ✅ | ✅ | ✅ | But heartbeat broken (C5) | Fix C5 |
| Allocations | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Servers | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Eggs / nests / templates | ✅ | ✅ | ✅ | n/a | ✅ | — | — |
| Startup variables | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Console (WS) | ✅ | ✅ | ✅ | ✅ | ✅ (but JWT in URL, permissive origin) | C10, C11 | — |
| Power controls | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Files (CRUD) | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| SFTP | ✅ | ✅ | ✅ | ✅ | ✅ | Static review OK; tests fail (C1) | Fix C1 |
| Backups (local + S3) | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Schedules | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Databases | ✅ | ✅ | ✅ | n/a | ✅ | — | — |
| Mounts | ✅ | ✅ | ❌ | ✅ | ✅ | — | — |
| Activity logs (audit) | ✅ | ✅ | ✅ | ✅ | ✅ per-server; cluster-wide missing | C3 | Add `GET /activity` |
| API keys | ✅ | ✅ | ✅ | ✅ | ✅ | — | — |
| Webhooks | ✅ | ✅ | ❌ | n/a | ✅ (migration 031) | — | — |
| Settings (panel-wide) | ✅ | ✅ | ✅ | n/a | **FAIL** | C2 | Add `/admin/settings` |
| Daemon heartbeat | ✅ | ✅ | ✅ | ✅ | **FAIL** (URL bug) | C5 | Strip `/api/v1` prefix |
| Daemon stats | ✅ | ✅ | ✅ | ✅ | **FAIL** (always zero mem/disk) | C6 | Use MemStats + syscall |
| Docker lifecycle | n/a | n/a | n/a | ✅ | ✅ | — | — |
| Server install | n/a | n/a | n/a | ✅ | ✅ | — | — |
| Deployment | PHP / Docker | PHP / Docker | Go binary | Go binary | Docker + compose | Dockerfile mismatch (C8) | Bump Dockerfile to 1.26 |
| Plugin / extension | ✅ | ✅ | ❌ | ❌ | **MISSING** | No extension system | Future work |
| **Orchestration** | Limited | Limited | ❌ | ❌ | **YES** (differentiator) | Neutralized by C5/C6 | Fix heartbeat |
| Bulk operations | Limited | Limited | ❌ | n/a | ✅ | — | — |
| Placement / reservation | ❌ | ❌ | ❌ | n/a | ✅ (028 + UI) | — | — |
| Node evacuation | ❌ | ❌ | ❌ | n/a | ✅ (024 + UI) | — | — |
| Recovery coordinator | ❌ | ❌ | ❌ | n/a | ✅ (029 + UI) | — | — |
| Hardening engine | ❌ | ❌ | ❌ | n/a | **PARTIAL** (Docker hardening only) | App-layer missing | Future work |
| Health dashboard | ✅ | ✅ | ✅ | ✅ | **FAIL** (fabricated) | C4 | Remap contract |
| Logs viewer | ✅ | ✅ | ✅ | ✅ | **FAIL** (404) | C3 | Add `/activity` |

---

## L. Orchestration readiness

| Module | Present | Half-wired | Missing | Risk |
|---|---|---|---|---|
| Server hardening engine | Docker hardening only | App-layer hardening | ✅ | Low — Docker-side hardening is on. |
| Node health engine | ✅ (`observability` + `heartbeatmonitor`) | — | — | Broken by C5 (heartbeat never lands). |
| Heartbeat engine | ✅ (API) | Daemon side | — | Broken by C5 + C6. |
| Placement planner | ✅ (`clustermanager`, `noderegistry`) | — | — | Depends on heartbeat. |
| Reservation planner | ✅ (`reservations` service + migration 028) | — | — | Depends on heartbeat. |
| Migration planner | ✅ (`migration` service + migration 025 + UI) | — | — | Depends on heartbeat. |
| Evacuation planner | ✅ (`evacuationplanner` service + migration 024 + UI) | — | — | Depends on heartbeat. |
| Recovery coordinator | ✅ (`recovery` service + migration 029 + UI) | — | — | Depends on heartbeat. |
| Bulk server operations | ✅ (orchestration tabs) | — | — | UI-only; underlying migration path is real. |
| Capacity planning | ✅ (`capacity` tab) | — | — | Depends on heartbeat. |
| Multi-region awareness | ✅ (`regions` + `RegionCapacity`) | — | — | Depends on heartbeat. |

**Verdict:** every orchestration module is structurally present and wired through the UI. **They are all neutralized by the daemon heartbeat URL bug (C5) and the hardcoded zeros (C6)**.

---

## M. Final verdict

**Broken / not safely runnable.**

- The Recovery commit is correct, but the working tree has 5,741 files staged for deletion that will re-destroy it on commit.
- The daemon `go test ./...` does not compile.
- The daemon heartbeat never works.
- Three admin pages are silently broken.
- No first-run wizard exists.

The previous agent's "all green" claim is **factually wrong** on at least: daemon test compilation, the existence of staging-area deletions, the working state of `/admin/settings` and `/servers/activity`, the working state of the heartbeat.

What the previous agent *did* get right: the API builds, vets, and tests; the panel typechecks and builds; the directory restructure is complete; the orchestration module wiring is correct.

---

## N. Next patch plan (in order — do not skip)

### 1. STOP. Do not commit. Un-stage the deletions.
- **Action:** `git restore --staged .` to undo all 5,741 staged deletions. Review the diff to ensure the 5 `infra/* → deploy/*` renames are still what you want.
- **Why first:** committing the current index re-destroys the project. This is the single most important action.

### 2. P0 fixes (in this order)
1. **Fix daemon test compilation** (C1). One-line fix per call site in `apps/daemon/internal/server/server_test.go`. After: `go test ./...` should pass.
2. **Fix daemon heartbeat URL** (C5). Apply the `/api/v1` strip from `syncServersFromPanel` (main.go:127-130) to `heartbeatLoop` (main.go:195).
3. **Fix daemon heartbeat telemetry** (C6). Replace `MemoryMB: 0, DiskMB: 0` with real values from `runtime.MemStats` + syscall/`gopsutil`.
4. **Add `GET /admin/settings` and `PUT /admin/settings`** in `handlers_admin.go` (C2). Backed by a new `panel_settings` table + migration 032.
5. **Add `GET /activity` (cluster-wide)** in `handlers_admin.go` (C3). Update `AdminLogs.tsx` to call it.
6. **Fix `AdminHealth` contract** (C4). Either change the API to return `{checks: [...]}` or remap on the client. **Remove the fake-data `.catch()` fallback.**
7. **Add first-run setup wizard** (C7). `POST /api/v1/setup` handler gated to "no admin exists", `app/setup/page.tsx`, `panel_setup_state` table.
8. **Fix Dockerfiles** (C8). Bump to `FROM golang:1.26-alpine AS build` (once the upstream image is published) or downgrade go.mod to `go 1.24`.
9. **Fix CI** (C9). Update `.github/workflows/ci.yml` paths to `apps/api`, `apps/daemon`, `apps/panel`. Re-align the `go-version`.
10. **Remove stale dev seed** or keep it gated to `APP_ENV=development` (already done) and update the README to not advertise the credentials (C12).

### 3. P1 fixes
1. **Daemon `WebSocket` `CheckOrigin` allowlist** (C10).
2. **Frontend WS ticket endpoint** (C11).
3. **Frontend `useAuth` hook + `/auth/me` re-validation** on app mount. Redirect to `/` on 401.
4. **Remove `app/setup` from middleware whitelisting** (C13) or build the page (covered by P0#7).

### 4. Build / test fixes
1. **Run `go build ./...`** on both modules — already verified clean.
2. **Run `go vet ./...`** on both modules — daemon will be clean after C1 fix.
3. **Run `go test ./...`** on both modules — daemon will be clean after C1 fix.
4. **Run `npm run typecheck`** in `apps/panel` — already verified clean.
5. **Run `npm run build`** in `apps/panel` — already verified clean.
6. **Run `npm run lint`** in `apps/panel` — not yet executed; should pass.

### 5. Integration fixes
1. **End-to-end test:** `apps/api` + `apps/daemon` + `apps/panel` running together with a real Postgres. Verify node registration, heartbeat (after C5 fix), server install, console, file ops, backup, SFTP, and one orchestration flow.
2. **Verify `deploy/compose.yml`** content and add a production profile.
3. **Verify `deploy/nginx.conf`** TLS termination and proxy buffering for WebSocket.

### 6. Production hardening
1. **Add `requirePermission(perm)` middleware** and apply to settings, mounts, backups, databases, webhooks.
2. **Constant-time node-token comparison** in `store_nodes.go:219-221`.
3. **Daemon requires `DAEMON_NODE_TOKEN` in all envs** (not just production).
4. **Rate limiting** on auth + node-token endpoints.
5. **HttpOnly + Secure + SameSite=strict** cookie strategy for production.
6. **Audit log sensitive actions** (node creation, user role changes, settings changes, backup deletion).

### 7. Frontend polish
1. **`AdminOverview` loading + error states**.
2. **Delete dead code:** `useServerStore`, `console-view.tsx`.
3. **Migrate console and files pages to `useQuery`**.
4. **Populate `packages/{sdk,shared-types,ui}`** or delete the empty workspaces.
5. **Reconcile the 5 unstaged modifications** (`.gitignore`, `README.md`, `package-lock.json`, `package.json`, `start-dev.ps1`).

### 8. Orchestration re-enable
1. **Verify all orchestration features end-to-end** with the heartbeat fix.
2. **Feature-flag the orchestration surface** (settings toggle + environment variable) so it can be disabled until hardened.
3. **Add tests** for placement, reservation, evacuation, and recovery services.
4. **Add an "app-level hardening engine"** to complement Docker hardening (cgroups, ulimits, syscall filter, seccomp).

### Verification gate before declaring production-ready
- `go build ./...` on both Go modules on the chosen Go version.
- `go vet ./...` and `go test ./...` on both modules.
- `npm run typecheck`, `npm run lint`, `npm run build` in `apps/panel`.
- CI workflow green on a fresh push.
- Manual end-to-end test of: setup wizard, node registration, heartbeat, server install, console, files, SFTP, backup, schedules, databases, one orchestration flow.
- `grep -rn "TODO|FIXME|mock|fake|placeholder" apps/` returns only allowed matches.
- `git status` shows no stray binaries, no untracked secrets, no untracked build artifacts, and **no staged deletions**.

If any of these fail, the project is not production-ready.

---

## O. Open questions for the user (in priority order)

1. **CRITICAL: do you want to undo the 5,741 staged deletions?** The current git index will re-destroy the project on commit. `git restore --staged .` is the safe action. Confirm before any further work.
2. **Target Go version for Dockerfiles.** `golang:1.26-alpine` is the obvious match for `go 1.26.0` in go.mod, but the upstream image may not exist yet. Downgrade go.mod to `go 1.24` if you want stable Docker builds.
3. **Settings backend.** New table + migration, or remove the page from the admin shell?
4. **Logs scope.** Cluster-wide `/activity`, or per-server logs in the admin Logs page?
5. **Health contract.** API returns `checks[]`, or client remaps existing `{ok, service, redis, postgres}`?
6. **First-run wizard.** Add `POST /api/v1/setup` + `/setup` page, or document a `forge setup` CLI subcommand?
7. **Hardcoded dev credentials.** Remove the dev seed, or keep it (already gated) and have it print a random password?
8. **JWT in WebSocket URL.** Short-lived ticket endpoint now, or accept the leak for v1.0?
9. **Stale binaries at repo root.** `rm api daemon` and update `.gitignore`?

These answers unblock the patch plan in Section N.
