Repository index — key files and responsibilities

Top-level folders
- `apps/frontend` — Next.js 15 frontend (TypeScript, Tailwind). Entry: `apps/frontend/app` routes. Key files: `lib/api.ts` (API client), `components/dashboard.tsx`, `components/file-editor.tsx`, `components/server-console.tsx`, `app/server/[id]/*` pages.
- `apps/api` — Go API (Fiber). Entry: `apps/api/cmd/api/main.go`. Router: `apps/api/internal/http/server.go`. Data layer: `apps/api/internal/store/store.go`. Daemon client: `apps/api/internal/daemon/client.go`. Migrations: `apps/api/migrations/*.sql` (001_init.sql ... 007_postgres_core_foundation.sql).
- `apps/daemon` — Go daemon controlling containers. Entry: `apps/daemon/cmd/daemon/main.go`. HTTP runtime handler: `apps/daemon/internal/server/server.go`. Docker runtime: `apps/daemon/internal/runtime/docker.go` implementing `runtime.Runtime` interface.
- `refs/pterodactyl-panel` — Reference copy of legacy Pterodactyl panel (PHP/Laravel). Useful for feature mapping and behavior reference only. Do not copy proprietary code directly.
- `infra` — Docker compose, Nginx, monitoring (Prometheus/Grafana) and scripts for local environment. Key file: `infra/docker/docker-compose.yml`.
- `docs` — design docs, migration notes, conversion mapping. Files added: `conversion-mapping.md`, `api-endpoint-coverage.md`, `PROJECT_STATUS.md`.

Important runtime flows
- Auth: frontend posts to `POST /api/v1/auth/login` → API issues JWT and frontend stores in `localStorage` (`modern-game-panel-token`). Protected API routes use `authMiddleware` and `X-Panel-Signature` for daemon requests.
- API ↔ Daemon: API uses `apps/api/internal/daemon/client.go` to talk to daemon base URLs (node baseUrl) using HMAC-signed headers; daemon exposes endpoints in `apps/daemon/internal/server/server.go` which call `runtime.Runtime`.
- Container runtime: `DockerRuntime` in `apps/daemon/internal/runtime/docker.go` uses Docker Engine API to create, start, stop, attach to containers and manage volumes.
- Files: chunked uploads handled by frontend `uploadFileChunked` and API's `/servers/:id/files/upload` which forwards to daemon `UploadFileChunk`.
- Websockets: API proxies websocket endpoints to daemon; daemon exposes websocket endpoints `ws/console`, `ws/logs`, `ws/stats`.

Database / Migrations
- Core migrations exist in `apps/api/migrations`. They provide baseline tables (`users`, `nodes`, `servers`, `allocations`, `audit_events`) and further additive migration `007_postgres_core_foundation.sql` adds roles, nests, eggs, server_transfers, subusers, and other fields.
- Legacy Pterodactyl migrations in `refs/pterodactyl-panel/database/migrations` are far more extensive; map them incrementally rather than copy wholesale.

Frontend responsibilities
- `apps/frontend/lib/api.ts` implements all API calls used by UI. Update `NEXT_PUBLIC_API_URL` or `NEXT_PUBLIC_DEMO_MODE` in `.env` as needed.
- `components/dashboard.tsx` is the central UI — large and contains admin + server UI; consider splitting into subcomponents per tab.

Suggested next investigative steps (I can run these):
1. Create a complete per-endpoint mapping between `lib/api.ts` calls and `server.go` handlers (done in `api-endpoint-coverage.md`, can generate CSV).
2. Verify `apps/daemon` implements all client methods invoked by `apps/api/internal/daemon/client.go` (CreateServer, DeleteServer, Logs, Stats, CreateBackup, ListBackups, DownloadBackup, ListFiles, ReadFile, WriteFile, UploadFileChunk, DeleteFile, MakeDir, RenameFile, AttachConsole, etc.).
3. Run smoke tests exercising key frontend flows (login, list servers, create allocation, create server, file ops).

Files I inspected during this pass
- `apps/api/internal/http/server.go`
- `apps/api/internal/store/store.go`
- `apps/api/internal/daemon/client.go`
- `apps/api/cmd/api/main.go`
- `apps/daemon/internal/server/server.go`
- `apps/daemon/internal/runtime/docker.go`
- `apps/daemon/cmd/daemon/main.go`
- `apps/frontend/lib/api.ts`
- `apps/frontend/components/dashboard.tsx`
- `infra/docker/docker-compose.yml`

If you want, I will now:
- run the smoke tests (login → fetch servers → create allocation → create server)
- or produce a per-file TODO mapping for porting the rest of Pterodactyl features and start implementing a prioritized subset.
