# Plan (Working Notes)

This repo is a **Pterodactyl-inspired** game panel, rebuilt in a modern stack (Next.js + Go + Docker + Postgres/Redis).

## Goal
- **Ship older stack → newer stack** by porting behaviors/workflows from Pterodactyl *feature-by-feature*.
- Use Pterodactyl as a **behavioral reference**, not a codebase to copy.

## Reference repos (local)
- Panel reference: `refs/pterodactyl-panel` (remote `pterodactyl/panel`)
- Wings reference: `refs/pterodactyl-wings` (remote `pterodactyl/wings`)

## Porting rules (important)
- Do **not** copy Pterodactyl source code, assets, migrations, or UI strings.
- Copy the **concepts and workflows** and implement the smallest original version in our stack.

## MVP slice (1:1 behavior target)
- Auth/login + session
- Admin: nodes, allocations, templates, users/roles (basic RBAC)
- Servers: create → install → power (start/stop/restart/kill) → delete
- Realtime: console + stats + logs (WebSocket via API gateway)
- Files: jailed browse/read/write/rename/mkdir/delete + chunked upload + Monaco editor
- Backups: create/list/download
- Infra: compose stack + smoke test + metrics (Prometheus/Grafana)

## Current state (high level)
- Docs exist for API/daemon/security/integration/roadmap under `docs/`.
- API + daemon entrypoints exist under `apps/api/cmd/api` and `apps/daemon/cmd/daemon`.
- Frontend has API client with explicit demo fallback gating in `apps/frontend/lib/api.ts`.

## Next steps (execution)
- Audit existing UI routes vs real API behavior; remove accidental reliance on demo fallback for core flows.
- Run compose + smoke test until consistently green; treat that as the baseline for iteration.