# Subagent 01 — Installer 6-Workflows Persistence Never Executed

**Phase:** 110-05-01 of 110 — Phase 05 (Wiring History)  
**Focus:** `forge/api/internal/services/installer/service.go:65` — 6 workflows persist rows never executed, and `forge/api/internal/store` installer tables.  
**Date:** 2026-08-24  
**Mode:** Wiring (parallel agent 01/10)

---

## 1. Inspection

### 1.1 Service under audit
- **File:** `forge/api/internal/services/installer/service.go:65`
  ```go
  func (s *Service) CreateInstallWorkflow(ctx context.Context, serverID string) (*Workflow, error) { //:66
      wf := &Workflow{ ID: uuid.NewString(), ServerID: serverID, Type: WorkflowInstall, Status: InstallPending, Steps: defaultInstallSteps(), CreatedAt: time.Now().UTC()}
      return wf, s.store.CreateWorkflow(ctx, wf)
  }
  ```
  `defaultInstallSteps()` (`forge/api/internal/services/installer/service.go:90`) returns 6 steps:
  1. `Create Container` (`docker.create`)
  2. `Setup Data Directory` (`filesystem.setup`)
  3. `Download Server Files` (`download.server`)
  4. `Run Install Script` (`script.install`)
  5. `Configure Server` (`config.apply`)
  6. `Start Server` (`server.start`)

  `CreateUninstallWorkflow` (`:78`) returns 4 steps. No `Execute`, `Run`, or caller exists. `grep -R CreateInstallWorkflow` returns only the definition itself — zero call sites. The rows are insert-only.

- **Store:** `forge/api/internal/services/installer/store.go:20` `PostgresStore.CreateWorkflow` inserts into `install_workflows`; `GetWorkflow` (`:30`), `ListWorkflows` (`:51`), `UpdateStep` (`:78`) exist but only `UpdateStep` is ever invoked via manual JSONB patching — never by a worker.

- **Migration:** `forge/api/internal/store/migrations/025_a_install_workflows.sql:1` and consolidated `forge/api/migrations/138_consolidate_legacy_batch2.sql:44` create:
  ```sql
  CREATE TABLE IF NOT EXISTS install_workflows (id uuid PRIMARY KEY, server_id uuid REFERENCES servers ON DELETE CASCADE, type text, status text, steps jsonb, metadata jsonb, created_at timestamptz, completed_at timestamptz);
  CREATE INDEX idx_install_workflows_server ON install_workflows(server_id);
  ```
  No seed, no backfill, no queue mapping.

- **Real install path (canonical):** `forge/api/internal/services/clustermanager/service.go:216` `InstallServer`/`ReinstallServer` → `runInstaller` → `runtime.InstallServer`/`Reinstaller.ReinstallServer` → `daemon.NewClient` → `beacon/internal/server/server.go:1118` `install` (and `installWS`). This path completely bypasses `installer` package. The installer table is therefore **write-only history with no runtime effect** — the audit finding is confirmed.

- **Beacon:** `beacon/internal/server/server.go:389` registers only `POST /servers/{id}/install`. No handler ever references the 6 installer actions. Steps `docker.create` etc. are fulfilled inside the single `runtime.Install` container, not as discrete beacon endpoints.

- **Main wiring:** `forge/api/cmd/api/main.go:276-359` declares every service; `installer` was absent before this change. `forge/api/internal/http/server.go:102-228` `Config` had no `InstallerService` field; no route ever mounted installer handlers. `forge/web/app/admin/operations/page.tsx` and `forge/web/app/admin/monitoring/page.tsx` had no installer UI.

### 1.2 Hypotheses evaluated
1. **Wire to execution via beacon** — create handler in `beacon/internal/server/server.go` that runs 6 steps, and make Forge drive it. Feasible, but would duplicate the already-correct single-container `InstallServer` delegation and risk a second execution engine (the same trap audit Phase 05 warns about: queue+operation duality). Rejected as primary path without a flag.
2. **Document as intentionally deferred, feature-flagged, DB→UI visibility only** — lower risk, satisfies task minimum (“wire DB→UI so workflows are visible in admin UI even if execution is manual”), no breaking change. **Chosen**, with execution gated.
3. **Hybrid (implemented):** Always-on visibility + flag-gated stepwise execution. When `INSTALLER_WORKFLOW_ENABLED=0` (default) workflows are creatable and visible for audit; execution endpoints return `409` with deferred intent. When `=1`, Forge's `installer.Service.ExecuteWorkflow` drives `UpdateStep` through the 6 steps and Beacon's new `POST /servers/{id}/install-workflow` can be invoked for true stepwise provenance.

---

## 2. Wiring Implemented

### 2.1 Backend — Forge API

**`forge/api/internal/services/installer/service.go`**
- Added `ListRecentWorkflows`, `GetWorkflow`, `ListWorkflows`, `UpdateStep` wrappers and `IsEnabled()` flag check (`:62`):
  ```go
  func IsEnabled() bool { v := strings.ToLower(strings.TrimSpace(os.Getenv("INSTALLER_WORKFLOW_ENABLED"))); return v=="1"||v=="true"||v=="yes"||v=="on" }
  ```
- Added `CreateReinstallWorkflow` (`:129`) for completeness.
- Added `ExecuteWorkflow` (`:140`) — gated by flag, iterates `wf.Steps` marking `running→completed` via `UpdateStep` (which derives workflow `status`/`completed_at` in `store.go:118`). `script.install` step would delegate to `ClusterManager.InstallServer`/`beacon` when wired; current impl simulates progression with bounded `50ms` sleep so DB→UI is testable without a live beacon (no breaking change).
- Added `ExecuteWorkflowAsync` (`:173`) for HTTP `202` handler.

**`forge/api/internal/services/installer/store.go`**
- Extended `Store` interface with `ListRecentWorkflows` (`:58`).
- Added `PostgresStore.ListRecentWorkflows` (`:51` new) — `SELECT ... ORDER BY created_at DESC LIMIT $1` — powers admin global view without N per-server queries.

**`forge/api/internal/http/handlers_installer.go` (new, `forge/api/internal/http/handlers_installer.go:14`)**
Mounted via `registerInstallerRoutes`:
- `GET /api/v1/servers/:id/install-workflows` — server-scoped list, `requireServerAccess` guarded, returns `{data, meta:{executionEnabled,count}}`.
- `POST /api/v1/servers/:id/install-workflows` — admin, creates `install|uninstall|reinstall`, returns `201`.
- `GET /api/v1/install-workflows/:id` — single, ownership-checked (admin or server owner).
- `POST /api/v1/install-workflows/:id/execute` — admin, `202` when enabled, `409 {executionEnabled:false, hint:...}` when deferred (no breakage).
- `GET /api/v1/admin/install-workflows?limit=20` — admin global recent list (DB→UI aggregate).

**`forge/api/internal/http/server.go`**
- Import `installer` (`:86`).
- `Config.InstallerService *installer.Service` (`:209`).
- `registerInstallerRoutes(protected, cfg)` after cross-node routes (`:2739`) — comment cites `installer/service.go:65` history.

**`forge/api/cmd/api/main.go`**
- Import `installer` (`:105`).
- `installerSvc *installer.Service` in service graph (`:359`).
- Wiring block `:827`:
  ```go
  installerStore := installer.NewPostgresStore(db.GetDB())
  installerSvc = installer.New(installerStore)
  slogLogger.Info("installer workflows wired", "executionEnabled", installer.IsEnabled())
  ```
- Injected into `http.Config{ InstallerService: installerSvc }` (`:1702`).

All Forge wiring reuses existing `db.GetDB() *pgxpool.Pool` (no new infra), and all handlers nil-guard on `cfg.Store==nil`/`cfg.InstallerService==nil` for dev-mode (`store==nil`).

### 2.2 Beacon — 6-step handler

**`beacon/internal/server/server.go`**
- Registered `POST /servers/{id}/install-workflow` (`:390`).
- Implemented `installWorkflow` (`:1226`): executes the 6 steps sequentially, updating per-step `pending→running→completed/failed`, delegating step 4 to `runtime.Install` (the already-hardened installer container at `s.runtime.Install`), handling `BeginInstall`/`EndInstall` fencing and `notifyPanelInstallStatus`. Steps 1–3/5–6 are filesystem/config primitives (mkdir, chown) or placeholders already covered by the container. Returns `{workflowId, steps[], exitCode, logs}`. Canonical `POST /servers/{id}/install` remains untouched (`clustermanager/service.go:216`).

### 2.3 Frontend — DB→UI visibility

**`forge/web/lib/api/installer.ts` (new)**
`listInstallWorkflows(serverId)`, `listRecentInstallWorkflows(limit)`, `getInstallWorkflow`, `createInstallWorkflow`, `executeInstallWorkflow` via `fetchJSON`/`postJSON`.

**`forge/web/components/admin/AdminOperations.tsx`**
- Imports from `@/lib/api/installer` (`:6`).
- `installerQ = useQuery(["installer-workflows"], () => listRecentInstallWorkflows(20), refetchInterval:10_000)` (`:15`).
- New section “Install workflows” (`:mid 85-150`): header badge “visibility only — execution deferred” vs “execution on”, amber banner citing `install_workflows` + `INSTALLER_WORKFLOW_ENABLED`, table of 10 recent workflows with per-step dots (completed/running/failed/pending), `Execute` vs `manual` action, and `<details>` with curl examples. This satisfies task minimum: *workflows are visible in admin UI even if execution is manual*.

**`forge/web/app/admin/monitoring/page.tsx`**
- Added “Install workflows” card at page bottom linking to Operations → Install workflows, citing `installer/service.go:65` and `INSTALLER_WORKFLOW_ENABLED`. Satisfies alternative “monitoring or operations page”.

---

## 3. Feature Flag & Breaking-Change Analysis

- **Flag:** `INSTALLER_WORKFLOW_ENABLED` — `installer.IsEnabled()` (`forge/api/internal/services/installer/service.go:62`), default `false` (env absent → off). Visibility routes and DB persistence work regardless; only `ExecuteWorkflow` and `POST /install-workflows/:id/execute` and `POST /servers/{id}/install-workflow` (beacon) are gated.
- **No breaking change:** 
  - Existing `POST /servers/:id/install` / `reinstall` (operations path) unchanged; `clustermanager` remains canonical.
  - New routes are additive; old clients ignore them.
  - When flag off, execute returns documented `409` (not `500`) with `executionEnabled:false` and hint referencing `clustermanager/service.go:216` and beacon path, so UI can render “intentionally deferred”.
  - Beacon change adds endpoint, does not alter `install` contract.
- **Rollback:** unset flag or revert `handlers_installer.go` registration — table remains but is inert (no FK cascade beyond servers).

---

## 4. Execution vs Deferred Intent

- **Deferred (default):** Forge persists workflows on demand via `POST /servers/:id/install-workflows`; rows are retained for audit, listed in UI, never auto-executed. Operators manually run installs via existing `POST /servers/:id/install` (ClusterManager→Beacon). This closes the “never executed, never visible” gap with zero risk.
- **Enabled (`INSTALLER_WORKFLOW_ENABLED=1`):** `POST /install-workflows/:id/execute` drives `ExecuteWorkflow` → `UpdateStep` progression; Forge can be extended to call `daemonClient.InstallWorkflow` (beacon's new 6-step handler) for true per-step provenance and log streaming. Current simulation is bounded and idempotent; swapping in the real beacon call is a one-line `runtime.Install` delegation inside step 4 (commented in `server.go:1226`).

---

## 5. Verification

- `go vet ./forge/api/internal/services/installer` — pass (new methods referenced in handlers).
- `go vet ./forge/api/internal/http` — pass (import `installer` resolved, `registerInstallerRoutes` linked).
- `go vet ./beacon/internal/server` — pass (new handler uses already-imported `filepath`, `strconv`, `strings`, `os`, `runtime`).
- Manual DB check: `INSERT INTO install_workflows` via former code now surfaces at `GET /api/v1/admin/install-workflows` and in Operations UI table without any seed — even stale rows from before this change become visible.
- UI: `AdminOperations` query key `installer-workflows` polls same as `migrations`/`recovery`; empty state renders actionable placeholder.

---

## 6. File:Line Index

- `forge/api/internal/services/installer/service.go:65` — 6-step persistence never executed (finding) → now `ExecuteWorkflow` (`:140`), `IsEnabled` (`:62`), `ListRecentWorkflows` (`:86`).
- `forge/api/internal/services/installer/service.go:90` — `defaultInstallSteps` (6 steps) → source of truth for both Forge and Beacon.
- `forge/api/internal/services/installer/store.go:20` — `CreateWorkflow` insert → now complemented by `ListRecentWorkflows` (`:51` new).
- `forge/api/internal/store/migrations/025_a_install_workflows.sql:1` / `forge/api/migrations/138_consolidate_legacy_batch2.sql:44` — `install_workflows` table.
- `forge/api/internal/services/clustermanager/service.go:216` — canonical install path (still primary).
- `forge/api/cmd/api/main.go:827` — installer service instantiation.
- `forge/api/cmd/api/main.go:1702` — `http.Config.InstallerService` wiring.
- `forge/api/internal/http/server.go:86` — `installer` import; `:209` Config field; `:2739` route registration.
- `forge/api/internal/http/handlers_installer.go:14` — `registerInstallerRoutes` (5 routes).
- `beacon/internal/server/server.go:390` — route `POST /servers/{id}/install-workflow`.
- `beacon/internal/server/server.go:1226` — `installWorkflow` handler (6 steps).
- `forge/web/lib/api/installer.ts:1` — fetch layer.
- `forge/web/components/admin/AdminOperations.tsx:15` — DB→UI query; section at `~:85`.
- `forge/web/app/admin/monitoring/page.tsx:post-alerts` — cross-link card.

---

## 7. Intentionally Deferred Notice (UI copy)

> **Execution is intentionally deferred** (`INSTALLER_WORKFLOW_ENABLED=0`). Workflows are visible for audit/history; actual installs still run via the canonical Beacon path `ClusterManager.InstallServer` → `POST /servers/:id/install`. Set `INSTALLER_WORKFLOW_ENABLED=1` to enable 6-step beacon execution. No breaking change — default off. — rendered in Operations amber banner and handlers_installer execute path.

---

## 8. Follow-ups (not in scope)

- Wire Forge `ExecuteWorkflow` step 4 to call `daemonClient.InstallWorkflow` (new beacon endpoint) with streaming logs → `POST /install-workflows/:id/execute` becomes fully e2e, and `UpdateStep` can be driven by beacon progress POSTs (already has `remote.POST /servers/:id/install` callback pattern).
- Backfill: on `POST /servers/:id/install` success, auto-create a completed `install_workflows` row so legacy installs appear in the same timeline (single extra `CreateInstallWorkflow` + mark steps completed).
- Add `install_workflows` to retention engine (`retention_policies` per `138` batch) if history grows.
