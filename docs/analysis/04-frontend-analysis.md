# 04 — Frontend Analysis (`forge/web`)

## Tech Stack

| Library | Version | Role |
|---|---|---|
| Next.js | 15.3 | App router, SSR/SSG framework |
| React | 19 | UI rendering |
| TypeScript | 5.7 | Static typing |
| TanStack Query | v5.90 | Server-state fetching & caching |
| Zustand | v5.0 | Client-side global state |
| xterm.js | v5.5 | Terminal emulator (console page) |
| Monaco Editor | v0.55 | Code/file editor |
| Tailwind CSS | v3.4 | Utility-first styling |
| lucide-react | latest | Icon set |

---

## Page Inventory

| Route | File | Component | API Calls | Status |
|---|---|---|---|---|
| `/` | `page.tsx` | Login | polls `/setup/status`, calls `login()` | ✅ |
| `/setup` | `setup/page.tsx` | Setup wizard | `fetchSetupStatus`, `runSetup` | ✅ |
| `/admin` | `admin/page.tsx` | AdminOverview | GET `/nodes` `/servers` `/users` | ✅ |
| `/admin/overview` | `admin/overview/page.tsx` | AdminOverview | GET `/nodes` `/servers` `/users` | ✅ |
| `/admin/nodes` | `admin/nodes/page.tsx` | AdminNodes | Full 5-tab CRUD | ✅ |
| `/admin/servers` | `admin/servers/page.tsx` | AdminServers | Full 8-tab CRUD | ✅ |
| `/admin/users` | `admin/users/page.tsx` | AdminUsers | Table + bulk + resource limits | ✅ |
| `/admin/allocations` | `admin/allocations/page.tsx` | AdminAllocations | CRUD | ✅ |
| `/admin/locations` | `admin/locations/page.tsx` | AdminLocations | CRUD | ✅ |
| `/admin/databases` | `admin/databases/page.tsx` | AdminDatabases | Database hosts CRUD | ✅ |
| `/admin/nests` | `admin/nests/page.tsx` | AdminNests | Nests + eggs two-pane | ✅ |
| `/admin/mounts` | `admin/mounts/page.tsx` | AdminMounts | Mounts + attach | ✅ |
| `/admin/api` | `admin/api/page.tsx` | AdminAPIKeys | API key management with scope picker | ✅ |
| `/admin/settings` | `admin/settings/page.tsx` | AdminSettings | 3-tab (general / mail / advanced) | ✅ |
| `/admin/activity` | `admin/activity/page.tsx` | AdminActivity | Audit log, 30s poll | ✅ |
| `/admin/health` | `admin/health/page.tsx` | AdminHealth | Health check cards | ✅ |
| `/admin/logs` | `admin/logs/page.tsx` | AdminLogs | Calls wrong endpoint `/activity` instead of logs endpoint | ⚠️ |
| `/admin/webhooks` | `admin/webhooks/page.tsx` | AdminWebhooks | CRUD + Discord preview | ✅ |
| `/admin/templates` | `admin/templates/page.tsx` | AdminTemplates | Create only — no edit/delete | ⚠️ |
| `/server/[id]` | `server/[id]/page.tsx` | ServerConsole | xterm.js + WS + power controls + charts | ⚠️ |
| `/server/[id]/files` | `server/[id]/files/page.tsx` | FileManager stub | Renders `FileManager` with `alert('not implemented')` instead of `FilesView` | ❌ |
| `/server/[id]/databases` | `server/[id]/databases/page.tsx` | ServerDatabases | TanStack Query | ✅ |
| `/server/[id]/schedules` | `server/[id]/schedules/page.tsx` | ServerSchedules | Full CRUD + tasks + run history | ✅ |
| `/server/[id]/backups` | `server/[id]/backups/page.tsx` | ServerBackups | Full CRUD + download + restore | ✅ |
| `/server/[id]/users` | `server/[id]/users/page.tsx` | ServerUsers | Permission editor | ✅ |
| `/server/[id]/network` | `server/[id]/network/page.tsx` | ServerNetwork | Allocations + set-primary | ✅ |
| `/server/[id]/startup` | `server/[id]/startup/page.tsx` | ServerStartup | Variable editor | ✅ |
| `/server/[id]/settings` | `server/[id]/settings/page.tsx` | ServerSettings | Rename + SFTP + reinstall — missing `node` prop | ⚠️ |
| `/server/[id]/activity` | `server/[id]/activity/page.tsx` | ServerActivity | Filterable audit log | ✅ |

---

## API Client Inventory (`lib/api.ts`)

~120 exported functions, organized by domain:

### Auth
`login`, `logout`, `fetchCurrentUser`, `fetchWSTicket`, `loginCheckpoint`, `fetchCSRFToken`

### Node (14 functions)
`fetchNodes`, `fetchNode`, `createNode`, `updateNode`, `deleteNode`, `fetchNodeAllocations`, `createAllocation`, `deleteAllocation`, `fetchNodeStats`, `fetchNodeHealth`, `fetchNodeConfiguration`, `deployNode`, `fetchRegions` *(no admin UI)*, `fetchNodeServers`

### Server (20+ functions)
`fetchServers`, `fetchServer`, `createServer`, `updateServer`, `deleteServer`, `suspendServer`, `unsuspendServer`, `reinstallServer`, `sendPowerSignal`, `fetchServerStats`, `fetchServerLogs`, `fetchServerConsoleToken`, `transferServer`, `fetchTransfers`, `cancelTransfer`, `completeTransfer`, `fetchServerActivity`, `updateServerStartup`, `fetchServerStartup`, `rebuildServer`

### File (11 functions)
`fetchFiles`, `createFile`, `writeFile`, `deleteFile`, `createDirectory`, `renameFile`, `copyFile`, `compressFiles`, `decompressFile`, `pullRemoteFile`, `fetchFileContent`

### Backup (5 functions)
`fetchBackups`, `createBackup`, `deleteBackup`, `downloadBackup`, `restoreBackup`

### Database (4 functions)
`fetchServerDatabases`, `createServerDatabase`, `deleteServerDatabase`, `rotateDatabasePassword`

### Schedule (9 functions)
`fetchSchedules`, `fetchSchedule`, `createSchedule`, `updateSchedule`, `deleteSchedule`, `triggerSchedule`, `fetchScheduleTasks`, `createScheduleTask`, `deleteScheduleTask`

### Startup (2 functions)
`fetchStartupVariables`, `updateStartupVariable`

### Allocation (6 functions)
`fetchAllocations`, `fetchAllAllocations`, `createAllocation`, `deleteAllocation`, `setPrimaryAllocation`, `fetchServerAllocations`

### Location (5 functions)
`fetchLocations`, `fetchLocation`, `createLocation`, `updateLocation`, `deleteLocation`

### Nest / Egg (10 functions)
`fetchNests`, `fetchNest`, `createNest`, `updateNest`, `deleteNest`, `fetchEggs`, `fetchEgg`, `createEgg`, `updateEgg`, `deleteEgg`

### User / Admin (6 functions)
`fetchUsers`, `fetchUser`, `createUser`, `updateUser`, `deleteUser`, `fetchServerUsers`

### Template (2 functions)
`fetchTemplates`, `createTemplate`
> No `updateTemplate` / `deleteTemplate` despite the API supporting them.

### API Key (4 functions)
`fetchAPIKeys`, `createAPIKey`, `deleteAPIKey`, `fetchAPIKeyScopes`

### SSH Key (3 functions) — **NO UI**
`fetchSSHKeys`, `createSSHKey`, `deleteSSHKey`

### 2FA (3 functions) — **NO UI**
`setupTwoFactor`, `enableTwoFactor`, `disableTwoFactor`

### Settings (7 functions)
`fetchSettings`, `updateGeneralSettings`, `updateMailSettings`, `updateAdvancedSettings`, `testMailSettings`, `fetchActivityLogs`, `fetchAdminLogs`

### Regions (1 function) — **NO UI**
`fetchRegions`

### Activity (3 functions)
`fetchActivityLogs`, `fetchServerActivity`, `fetchAdminActivity`

---

## Component Inventory

### `components/admin/`

| Component | Purpose | Complete |
|---|---|---|
| `AdminOverview` | Summary cards for nodes/servers/users | ✅ |
| `AdminNodes` | 5-tab node CRUD (details, allocations, servers, config, delete) | ✅ |
| `AdminServers` | 8-tab server CRUD | ✅ |
| `AdminUsers` | User table with bulk actions and resource limits | ✅ |
| `AdminAllocations` | Allocation list and CRUD | ✅ |
| `AdminLocations` | Location CRUD | ✅ |
| `AdminDatabases` | Database host CRUD | ✅ |
| `AdminNests` | Nests + eggs two-pane editor | ✅ |
| `AdminMounts` | Mount list + server attachment | ✅ |
| `AdminAPIKeys` | API key management with scope picker | ✅ |
| `AdminSettings` | 3-tab settings form | ✅ |
| `AdminActivity` | Audit log with 30s polling | ✅ |
| `AdminHealth` | Per-node health cards | ✅ |
| `AdminLogs` | Log viewer — calls wrong endpoint | ⚠️ |
| `AdminWebhooks` | Webhook CRUD + Discord preview | ✅ |
| `AdminTemplates` | Template creation only — no edit/delete | ⚠️ |

### `components/server/`

| Component | Purpose | Complete |
|---|---|---|
| `ServerConsoleLayout` | Shell wrapper; mounts all server sub-views | ⚠️ |
| `ConsoleView` (active) | xterm.js terminal + WebSocket | ⚠️ |
| `PowerControls` | Start/stop/restart/kill buttons | ⚠️ (raw fetch) |
| `ConsoleCharts` | CPU/memory chart (opens separate WS) | ⚠️ |
| `ServerStats` | Stats sidebar (opens third WS) | ⚠️ |
| `FilesView` | Monaco-based file manager | ✅ (not mounted) |
| `ServerDatabases` | Database list + rotate password | ✅ |
| `ServerSchedules` | Schedule CRUD + task editor | ✅ |
| `ServerBackups` | Backup list + download + restore | ✅ |
| `ServerUsers` | Sub-user permission editor | ✅ |
| `ServerNetwork` | Allocation list + set primary | ✅ |
| `ServerStartup` | Startup variable editor | ✅ |
| `ServerSettingsView` | Rename + SFTP info + reinstall | ⚠️ (missing node prop) |
| `ServerActivity` | Filterable activity log | ✅ |

### `components/ui/`

Standard shadcn/ui primitives: `Button`, `Card`, `Dialog`, `DropdownMenu`, `Input`, `Label`, `Select`, `Tabs`, `Table`, `Toast`, `Badge`, `Checkbox`, `Switch`, `Textarea`, `Tooltip` — all present and functioning.

---

## Dead Code Components

| Component | File | Superseded By |
|---|---|---|
| `FileManager` | `components/server/file-manager.tsx` | `FilesView` (Monaco, fully implemented) |
| `BackupList` | `components/server/backup-list.tsx` | `ServerBackups` (TanStack Query) |
| `DatabaseList` | `components/server/database-list.tsx` | `ServerDatabases` (TanStack Query) |
| `ConsoleView` (old) | `components/server/console-view.tsx` | Active console inside `ServerConsoleLayout` |
| `AdminLayout` | `components/admin/admin-layout.tsx` | Next.js `admin/layout.tsx` (root layout) |

All five components are imported nowhere and can be deleted safely.

---

## State Management Analysis

### Zustand Store Fields

| Field | Type | Written By | Read By |
|---|---|---|---|
| `token` | `string \| null` | `login()` action | API client headers |
| `currentUser` | `User \| null` | **Never written** | `useStore` consumers |
| `serverId` | `string \| null` | `ServerConsoleLayout` | Console sub-components |
| `addConsoleLine` | `(line) => void` | **Never called** | — |
| `updateStats` | `(stats) => void` | **Never called** | — |
| `liveStats` | `ServerStats \| null` | **Never written** | Stats sidebar |
| `cpuHistory` | `number[]` | **Never written** | ConsoleCharts |
| `memoryHistory` | `number[]` | **Never written** | ConsoleCharts |

### Critical State Issues

- **`addConsoleLine` / `updateStats` / `liveStats` / `cpuHistory` / `memoryHistory`** are defined in the store but never populated by any component. Charts and stats displays always show empty/zero data.
- **`currentUser` is always `null`** — no component calls a fetch-and-store action after login. Any feature gated on the current user silently breaks.
- **Token storage inconsistency** — the Zustand store holds `token`, but `PowerControls` reads `localStorage.getItem('token')` directly via a raw `fetch()`. If Zustand ever switches to a non-localStorage persistence adapter, power actions will break silently.

---

## Critical Bugs

1. **Files page renders a stub** — `/server/[id]/files` mounts `FileManager` which calls `alert('not implemented')`. The fully-featured `FilesView` (Monaco editor, directory tree, upload, rename, delete) exists in `components/server/files-view.tsx` but is never imported by the page.

2. **Undefined Tailwind CSS variables** — `text-panel-ink`, `bg-panel-brand`, `border-panel-line`, and `text-panel-muted` are referenced across at least 6 components but are not defined in `tailwind.config.ts` or any CSS file. Affected elements render with no color/border.

3. **AdminLogs calls the wrong endpoint** — `fetchAdminLogs` in `AdminLogs` hits `/activity`, which returns audit event objects `{id, event, user, …}`. The component maps the response as `LogEntry{level, message, source}`, so every field is `undefined` and the log table is empty.

4. **PowerControls bypasses the API client** — start/stop/restart/kill use a raw `fetch('/api/servers/{id}/power', { headers: { Authorization: localStorage.getItem('token') } })` instead of calling `sendPowerSignal()`. This skips interceptors, ignores the Zustand token, and will silently fail in any environment that sets cookies-only auth.

5. **Three separate WebSocket connections on the console page** — `ServerConsoleLayout` opens a console WS, `ConsoleCharts` opens a stats WS, and `ServerStats` opens a third stats WS independently. All three run concurrently with no coordination or shared connection.

6. **`ServerSettingsView` missing `node` prop** — `ServerConsoleLayout` does not pass the `node` object when mounting `ServerSettingsView`. The SFTP hostname and port fields fall back to placeholder values, so every user sees incorrect SFTP credentials.

7. **No user-facing server dashboard** — non-admin users have no landing page after login. There is no `/servers` route that lists the servers a user has access to. Non-admins land on the login page or a 404.

8. **No logout button** — `logout()` exists in `lib/api.ts` but is not wired to any navigation element in any layout, header, or menu.

9. **No 2FA login checkpoint UI** — `loginCheckpoint()` exists in the API client but there is no UI to collect the TOTP code after a successful password check. Any user with 2FA enabled is permanently locked out.

10. **Duplicate admin routes** — `admin/page.tsx` and `admin/overview/page.tsx` both render `<AdminOverview />` with identical data fetches. Navigating to either route makes two separate sets of API calls.

---

## Missing Features Summary

| Feature | API Functions Available | UI Status |
|---|---|---|
| 2FA enrollment | `setupTwoFactor`, `enableTwoFactor`, `disableTwoFactor` | ❌ No account/profile page |
| 2FA login challenge | `loginCheckpoint` | ❌ No checkpoint step in login flow |
| SSH key management | `fetchSSHKeys`, `createSSHKey`, `deleteSSHKey` | ❌ No page |
| Server transfer | `transferServer`, `fetchTransfers`, `cancelTransfer`, `completeTransfer` | ❌ No UI |
| User server dashboard | `fetchServers` | ❌ No `/servers` page |
| Logout | `logout()` | ❌ No button in any nav |
| Regions admin | `fetchRegions` | ❌ No admin UI |
| Template edit / delete | *(panel API supports it)* | ❌ `AdminTemplates` create-only |
