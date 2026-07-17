# MVP Audit (UI ↔ API ↔ Daemon)

> Superseded note (2026-06-16): this audit is retained for history, but parts of
> it are stale. The current frontend has dynamic `/server/[id]` routing and real
> admin screens for users, allocations, databases, mounts, nests, and eggs. Use
> `docs/PROJECT_STATE.md`, `docs/TASKS.md`, and the live source as the current
> implementation reference before making product decisions from this file.

This document maps the current UI implementation to the Go API and daemon surfaces and flags gaps for the **1:1 MVP slice**.

## Legend
- **OK**: implemented and wired to real API/daemon
- **Partial**: exists but has demo placeholders, missing selection flow, or incomplete wiring
- **Missing**: not present / only a placeholder screen

## Server area (Pterodactyl “Server”)

### Console (realtime)
- **UI**: `apps/frontend/components/dashboard.tsx` → `ServerConsoleView`
- **API**:
  - `GET /api/v1/servers/:id/logs` (initial text fetch)
  - `GET /api/v1/servers/:id/ws/console` (WebSocket)
  - `GET /api/v1/servers/:id/ws/stats` (WebSocket)
- **Daemon**:
  - `GET /servers/:id/logs`
  - `GET /servers/:id/ws/console`
  - `GET /servers/:id/ws/stats`
- **Status**: **Partial**
  - Console + stats WS are implemented in UI, but the UI always targets `servers[0]` (no server picker in the main “server mode” flow).

### Power + install
- **UI**: `apps/frontend/components/dashboard.tsx` → power buttons + “Install / Sync Container”
- **API**:
  - `POST /api/v1/servers/:id/power`
  - `POST /api/v1/servers/:id/install`
- **Daemon**:
  - `POST /servers/:id/power`
  - `POST /servers`
- **Status**: **Partial**
  - Wired, but depends on server selection problem above.

### File manager + Monaco + upload
- **UI**: `apps/frontend/components/dashboard.tsx` → `FilesView` + Monaco
- **API**:
  - `GET /api/v1/servers/:id/files`
  - `GET /api/v1/servers/:id/files/content`
  - `PUT /api/v1/servers/:id/files/content`
  - `PUT /api/v1/servers/:id/files/upload` (chunked)
  - `DELETE /api/v1/servers/:id/files`
  - `POST /api/v1/servers/:id/files/mkdir`
  - `PATCH /api/v1/servers/:id/files/rename`
- **Daemon**:
  - `GET /servers/:id/files`
  - `GET /servers/:id/files/content`
  - `PUT /servers/:id/files/content`
  - `PUT /servers/:id/files/upload`
  - `DELETE /servers/:id/files`
  - `POST /servers/:id/files/mkdir`
  - `PATCH /servers/:id/files/rename`
- **Status**: **OK** (feature-complete in UI; needs end-to-end verification against running stack)

### Backups (create/list/download)
- **UI**: `apps/frontend/components/dashboard.tsx` → `BackupsView`
- **API**:
  - `GET /api/v1/servers/:id/backups`
  - `POST /api/v1/servers/:id/backups`
  - `GET /api/v1/servers/:id/backups/download?name=...`
- **Daemon**:
  - `GET /servers/:id/backups`
  - `POST /servers/:id/backups`
  - `GET /servers/:id/backups/download?name=...`
- **Status**: **Partial**
  - UI shows a hardcoded “demo” list when API returns an empty list (even in non-demo situations). This should be gated or removed to avoid false data.

### Network + allocations
- **UI**: `apps/frontend/components/dashboard.tsx` → `NetworkView`
- **API**: `GET /api/v1/allocations`
- **Status**: **Partial**
  - The view is not per-server; it displays allocation rows with fallback dummy data when allocations are empty.

### Startup / settings / databases / schedules / users
- **UI**: startup is a static mock screen; others are placeholders (`EmptyServerTab`).
- **Status**: **Missing/Partial**
  - These are “later parity” items (beyond MVP unless explicitly required).

## Admin area (Pterodactyl “Admin”)

### Nodes list
- **UI**: `apps/frontend/components/dashboard.tsx` → `AdminNodes`
- **API**: `GET /api/v1/nodes`
- **Status**: **OK** (listing only)

### Servers list
- **UI**: `apps/frontend/components/dashboard.tsx` → `AdminServers`
- **API**: `GET /api/v1/servers`
- **Status**: **OK** (listing only)

### Users / roles
- **UI**: Admin nav includes “Users” but screen is placeholder.
- **API**: `GET /api/v1/users`, `POST /api/v1/users` (admin-only)
- **Status**: **Missing** (UI wiring)

### Templates (nests/eggs equivalent)
- **UI**: “Nests” exists but is a static script display; no real templates CRUD.
- **API**: `GET /api/v1/templates`, `POST /api/v1/templates` (admin-only)
- **Status**: **Missing/Partial** (real templates management)

### Allocations management
- **UI**: no admin allocations view (only server-side “network” tab).
- **API**: `GET /api/v1/allocations` (list)
- **Status**: **Missing** (admin UI)

## Cross-cutting gap: server selection
- The “server mode” experience should be able to select a server by route (`/server/[id]/...`) or a server list picker.\n+  Currently the dashboard uses `servers[0]` as “the server”.
