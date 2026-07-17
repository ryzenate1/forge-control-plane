Conversion mapping: Pterodactyl -> modern-game-panel

Summary
- Legacy reference: refs/pterodactyl-panel (Laravel PHP panel copy present in repo)
- Target stack: `apps/api` (Go + Fiber), `apps/daemon` (Go), `apps/frontend` (Next.js + React)

Key domains to port
- Auth & users: `/api/v1/auth/*`, `users`, `sessions` -> implement in `apps/api` (handlers + store). Seed user present in `store.Seed`.
- Servers & templates (eggs/nests): server model, templates, allocations -> map to `servers`, `server_templates`, `allocations` migrations in `apps/api/migrations`.
- Nodes/Daemon: node registration, node tokens, daemon runtime APIs -> `apps/daemon` responsibilities; API node endpoints in `apps/api`.
- Files, backups, SFTP: file APIs, backups -> daemon + API file endpoints; integrate SFTP/rsync later.
- Schedules, tasks, power control: server lifecycle endpoints -> API + daemon.
- Admin UI, server UI, console, file editor, backups -> `apps/frontend` pages/components (see `components/` and `app/` routes).

Immediate actionable tasks
1. Complete API endpoint coverage for core features used by frontend: auth, /nodes, /servers, /allocations, /templates. (Progress: partial — many handlers exist; implement missing ones.)
2. Ensure migrations in `apps/api/migrations` cover required schema from `refs/pterodactyl-panel/database` (review and add missing fields).
3. Wire daemon endpoints expected by API (node base URL, websocket proxies). Confirm tokens and auth flows.
4. Frontend: reconcile components under `apps/frontend/app` with API routes; update `API_BASE_URL` if needed and test flows (login, server list, console).
5. Add automated smoke tests that use seeded admin (`admin@example.com` / `admin123`) to exercise login and key endpoints.

Next steps I'll perform (pick one and run):
- Run a quick diff of `refs/pterodactyl-panel/database` vs `apps/api/migrations` to list missing DB objects.
- Enumerate currently unimplemented API endpoints referenced by the frontend.

Deep pass findings (2026-05-08)
- Daemon client/server parity check passed for core methods used by API: create/delete/power/stats/logs/files/backups/websockets are all implemented in `apps/daemon/internal/server/server.go` and backed by `apps/daemon/internal/runtime/docker.go`.
- Smoke test flow (login -> servers -> allocation -> server create) works with seeded admin credentials.
- Install failures were reproduced and root-caused to allocation IP formatting from Postgres `inet` values (returned as CIDR, e.g. `0.0.0.0/32`) when provisioning containers.

Fixes applied in this pass
1. Fixed server-allocation listing scan issue:
	- `apps/api/internal/store/store.go`
	- `ListServerAllocations` now selects `host(a.ip)` to safely scan/render host IP as plain text.
2. Fixed daemon provisioning IP normalization path:
	- `apps/api/internal/store/store.go`
	- `ServerProvisionTarget` now selects `host(a.ip)` to avoid passing CIDR host IPs into Docker port bindings.
3. Improved daemon create error detail propagation:
	- `apps/api/internal/daemon/client.go`
	- `CreateServer` now includes daemon response body (up to 4KB) in returned errors to improve diagnostics.

Verification after fixes
- API module compiles/tests clean: `go test ./...` in `apps/api`.
- End-to-end install path succeeds through API:
  - `POST /api/v1/servers/:id/install` returns `{"accepted":true,"mode":"docker"...}`.
  - Docker shows created container `mgp-<serverId>` for installed server.
  - `GET /api/v1/servers/:id/allocations` now returns host IP values like `0.0.0.0` (not CIDR suffixes).

Still pending for full Pterodactyl parity
- Schedules/tasks execution lifecycle (worker/runner semantics) and advanced action catalog.
- API keys/application keys and scoped auth.
- SSH keys and richer user-security profile.
- Mounts/database hosts and advanced startup/environment controls.
- Frontend decomposition of `components/dashboard.tsx` and per-tab E2E checks.

Frontend admin pass completed (next slice)
- `apps/frontend/components/dashboard.tsx`
	- `AdminAllocations` now uses a node dropdown (instead of manual UUID entry), pending state, and create status feedback.
	- Allocation creation is now routed through a dedicated React Query mutation in `Dashboard`, with cache invalidation for `allocations`.
	- `AdminView` prop wiring expanded to pass allocation create handlers/pending state cleanly.
- Result: templates and allocations creation flows are both wired through mutation-driven admin flows with refresh behavior.

Schedules/tasks baseline implemented (2026-05-08)
- DB migration added:
	- `apps/api/migrations/008_server_schedules.sql`
	- Adds `server_schedules` and `schedule_tasks` tables with constraints and indexes.
- Store layer added in `apps/api/internal/store/store.go`:
	- `ListSchedules`, `GetSchedule`, `CreateSchedule`, `PatchSchedule`, `DeleteSchedule`
	- `CreateScheduleTask`, `PatchScheduleTask`, `DeleteScheduleTask`
- API routes added in `apps/api/internal/http/server.go`:
	- `GET/POST /servers/:id/schedules`
	- `PATCH/DELETE /servers/:id/schedules/:scheduleId`
	- `POST /servers/:id/schedules/:scheduleId/tasks`
	- `PATCH/DELETE /servers/:id/schedules/:scheduleId/tasks/:taskId`
- Frontend client and UI:
	- `apps/frontend/lib/api.ts`: added schedule/task types and API helpers.
	- `apps/frontend/components/dashboard.tsx`: server `Schedules` tab now supports create/list/toggle/delete schedules and add/delete tasks.

Verified in this pass
- API compile/test: `go test ./...` (apps/api) passes.
- Frontend typecheck: `npm run typecheck` passes.
- Runtime smoke test:
	- Created schedule and task on server `44444444-4444-4444-4444-444444444444` via API.
	- Listed schedule with task payload successfully.

Schedule execution runner added (2026-05-08, next continuation)
- DB migration added:
	- `apps/api/migrations/009_schedule_run_history.sql`
	- Adds `schedule_runs` and `schedule_task_runs` for execution auditing.
- API runtime worker:
	- `apps/api/internal/http/schedule_runner.go`
	- Background loop polls due schedules, computes next run with cron expression parser, and records run/task outcomes.
	- Supported task actions currently: `power`, `backup`.
	- `command` action intentionally returns unsupported until daemon command API is added.
- Store extensions:
	- Due selection + run metadata update + run history persistence/listing in `apps/api/internal/store/store.go`.
- API endpoint added:
	- `GET /servers/:id/schedules/:scheduleId/runs`
	- `POST /servers/:id/schedules/:scheduleId/run` for manual trigger/testing.

Execution semantics currently
- First runner pass initializes `next_run_at` for schedules that do not have one yet.
- Subsequent due passes execute tasks in sequence and honor `continueOnFailure`.
- Run status is stored as `success`, `failed`, `partial`, or `skipped` with task-level status entries.
- Manual trigger is available for immediate verification and records a run with trigger `manual`.

Still missing for full schedule parity
- No retry/backoff policy or distributed lock semantics for multi-instance API deployments.
- Task action set is still limited and not yet validated against a strict action catalog.
