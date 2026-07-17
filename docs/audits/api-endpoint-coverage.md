API endpoint coverage — frontend → apps/api

> Superseded note (2026-06-16): this audit is retained for history, but parts of
> it are stale. The current code includes additional admin/server endpoints and
> live database-host connection testing. Use the live source plus
> `docs/PROJECT_STATE.md` and `docs/TASKS.md` as the current reference before
> making implementation decisions from this file.

Summary: scanned `apps/frontend/lib/api.ts` and `apps/api/internal/http/server.go`. Most frontend endpoints have corresponding handlers in the API; some require `postgres` and `daemon` to be enabled.

Endpoints referenced by frontend and coverage status

- `POST /api/v1/auth/login` — implemented (v1.Post "/auth/login")
- `GET /api/v1/auth/me` — implemented (protected GET)
- `GET /api/v1/health` — implemented
- `GET /api/v1/metrics` — implemented

- Users
  - `GET /api/v1/users` — implemented (admin)
  - `POST /api/v1/users` — implemented (admin)

- Nodes
  - `GET /api/v1/nodes` — implemented
  - `POST /api/v1/nodes` — implemented (admin)
  - `GET /api/v1/nodes/:id` — implemented
  - `PATCH /api/v1/nodes/:id` — implemented (admin)
  - `DELETE /api/v1/nodes/:id` — implemented (admin)
  - `POST /api/v1/nodes/:id/rotate-token` — implemented (admin)
  - `GET /api/v1/nodes/:id/allocations` — implemented
  - `GET /api/v1/nodes/:id/servers` — implemented

- Allocations
  - `GET /api/v1/allocations` — implemented
  - `POST /api/v1/allocations` — implemented (admin)
  - `PATCH /api/v1/allocations/:id` — implemented (admin)
  - `DELETE /api/v1/allocations/:id` — implemented (admin)

- Templates / Nests / Eggs
  - `GET /api/v1/templates` — implemented
  - `POST /api/v1/templates` — implemented (admin)
  - nests/eggs data persisted in `nests` / `eggs` migrations (present)

- Servers
  - `GET /api/v1/servers` — implemented
  - `GET /api/v1/servers/:id` — implemented
  - `POST /api/v1/servers` — implemented (admin)
  - `PATCH /api/v1/servers/:id` — implemented (admin)
  - `DELETE /api/v1/servers/:id` — implemented (admin)
  - `POST /api/v1/servers/:id/install` — implemented (calls daemon)
  - `POST /api/v1/servers/:id/reinstall` — redirects to install
  - `POST /api/v1/servers/:id/power` — implemented (daemon proxy)
  - `POST /api/v1/servers/:id/transfer` — implemented
  - `GET /api/v1/servers/:id/transfer` — implemented
  - `POST /api/v1/servers/:id/transfer/cancel` — implemented
  - `POST /api/v1/servers/:id/toggle-install` — implemented
  - `POST /api/v1/servers/:id/suspension` — implemented

- Server allocations
  - `GET /api/v1/servers/:id/allocations` — implemented
  - `POST /api/v1/servers/:id/allocations` — implemented (admin)
  - `DELETE /api/v1/servers/:id/allocations/:allocationId` — implemented (admin)
  - `POST /api/v1/servers/:id/allocations/:allocationId/primary` — implemented (admin)

- Files & editor
  - `GET /api/v1/servers/:id/files` — implemented (daemon)
  - `GET /api/v1/servers/:id/files/content` — implemented (daemon)
  - `PUT /api/v1/servers/:id/files/content` — implemented (daemon)
  - `PUT /api/v1/servers/:id/files/upload` — implemented (chunked upload via daemon)
  - `DELETE /api/v1/servers/:id/files` — implemented (daemon)
  - `POST /api/v1/servers/:id/files/mkdir` — implemented (daemon)
  - `PATCH /api/v1/servers/:id/files/rename` — implemented (daemon)

- Backups
  - `GET /api/v1/servers/:id/backups` — implemented (daemon)
  - `POST /api/v1/servers/:id/backups` — implemented (daemon)
  - `GET /api/v1/servers/:id/backups/download` — implemented (stream)

- Logs & stats
  - `GET /api/v1/servers/:id/logs` — implemented (daemon)
  - `GET /api/v1/servers/:id/ws/console` — implemented (websocket proxy)
  - `GET /api/v1/servers/:id/ws/logs` — implemented (websocket proxy)
  - `GET /api/v1/servers/:id/ws/stats` — implemented (websocket proxy)
  - `GET /api/v1/servers/:id/stats` — implemented (daemon)

- Audit
  - `GET /api/v1/audit` — implemented

Notes & gaps
- Database: `apps/api/migrations` provides a solid core (users, nodes, servers, allocations, templates, transfers, roles, nests/eggs). The legacy Pterodactyl migrations are far more extensive; full parity requires incrementally porting missing tables (backups metadata, schedules/tasks, mounts, databases, API keys, SSH keys, user recovery tokens, activity logs, etc.).
- Daemon: API delegates runtime operations to `apps/daemon`. Ensure `daemon` implements file operations, backup stream, upload chunking, create/delete server, stats, logs, and websocket proxy. In docker-compose the daemon container is present but verify each daemon method in `apps/api/internal/daemon` maps to implemented functions in `apps/daemon`.
- Auth/CORS: already adjusted for frontend port; auth token flows exist.

Recommended immediate next tasks
1. Run a sync: for each function call in `apps/api/internal/daemon` ensure corresponding method exists in `apps/daemon` (CreateServer, DeleteServer, UploadFileChunk, ReadFile, WriteFile, ListFiles, Logs, Stats, CreateBackup, ListBackups, DownloadBackup, MakeDir, RenameFile, DeleteFile, SendPower, etc.).
2. Create migration backlog: prioritize `backups` metadata, `user_ssh_keys`, `schedules`/`tasks`, `api_keys`, and `mounts`.
3. Add smoke tests that perform: login, fetch servers, create allocation (admin), create server (admin), install server (requires daemon), read files (requires daemon).
