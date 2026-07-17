# Gamepanel vs PufferPanel — Deep Comparison

> Generated: July 2026
> Comparison of our Gamepanel (Next.js + Go/Fiber) against PufferPanel (Vue.js + Go/Gin) reference at `/reference/pufferpanel/`

---

## 1. Frontend Framework Comparison: Vue.js vs Next.js Patterns

### PufferPanel (Vue.js 3 + Vite + Vue Router)
- **Single-page app** served from a Go binary via `embed.FS`
- Uses **dependency injection** (`inject`/`provide`) for API, toast, events, theme, validation — no global state library
- **i18n** with `vue-i18n` — every string is a lookup key
- **Web Worker** for console rendering (`consoleWorker.js`) — offloads ANSI parsing to a background thread
- **28 reusable UI components** (`Btn.vue`, `TextField.vue`, `Toggle.vue`, `Overlay.vue`, `Tab.vue`, etc.)
- **Server-type-specific components** loaded dynamically (`import(`../components/serverTypes/${type}.vue`)`)
- **Event bus** (`nanoevents`) for confirm dialogs, refresh triggers
- **Keybinding system** (`@github/hotkey`) — `<div v-hotkey="'c c'">` everywhere

### Our Gamepanel (Next.js 15 + React + TanStack Query)
- **Hybrid SSR/CSR app** — pages are statically rendered with `"use client"` directives
- **Zustand store** for auth (`use-server-store.ts`)
- **TanStack Query** for server state (cache, refetch, invalidation)
- **Tailwind CSS** utility classes — no component library abstraction
- **Lucide icons** — direct imports
- **Monaco Editor** (code editor) via `@monaco-editor/react`
- **xterm.js** for console terminal emulation
- **Inline UI patterns** — buttons, inputs, modals are inline Tailwind classes, not abstracted components

### Key Differences

| Aspect | Puffer (Vue) | Gamepanel (Next.js) |
|--------|-------------|-------------------|
| **UI component abstraction** | 28 reusable components | Tailwind utility classes inline |
| **State management** | DI + event bus | Zustand + TanStack Query |
| **i18n** | vue-i18n (every string) | None (hardcoded strings) |
| **Keyboard shortcuts** | `@github/hotkey` on every view | None |
| **Console rendering** | Web Worker (ANSI parsing offloaded) | Main thread (xterm.js) |
| **Code editor** | Ace editor | Monaco Editor |
| **Build** | Vite → static files → embed in Go | Next.js SSR |
| **Dynamic component loading** | `import(`.../${type}.vue`)` | Not done (static imports) |
| **Breadcrumbs** | Server-type-specific components | Generic layout |

---

## 2. Console / Terminal Implementation Comparison

### PufferPanel Console (`Console.vue`)
- **Event-driven**: subscribes to `server.on('console')` — the server object is a reactive wrapper
- **Polling fallback**: `server.startTask(() => getConsole(lastMessageTime), 5000)` — polls every 5s
- **Web Worker**: passes console data to a `ConsoleWorker` that handles ANSI parsing + HTML generation
- **DOM elements**: creates `<div>` elements for each line, caps at 1000 elements
- **Command history**: array of strings, max 100 entries, ArrowUp/Down navigation
- **Scope-gated UI**: `v-if="server.hasScope('server.console')"` and `server.console.send`
- **Startup**: fetches initial console log via `getConsole()` on mount

### Our Gamepanel Console (`console-view.tsx` + `console.tsx`)
- **WebSocket-based**: `connectServerWebSocket(serverId, "console")` — real-time, no polling
- **xterm.js terminal emulator**: full terminal emulation with themes, cursor, scrollback
- **Auto-reconnect**: exponential backoff (1s, 2s, 4s…up to 30s) via `nonce` state
- **Stats WebSocket**: separate connection to `stats` endpoint for CPU/Memory/Network sparklines
- **Search**: filter console output by text
- **Timestamp toggle**: show/hide timestamps on each line
- **Exponential backoff reconnect**: implemented manually
- **LocalStorage history**: persists command history per server via `HISTORY_KEY`
- **Sparkline charts**: SVG-based mini charts for CPU, memory, network
- **Uptime display**: live epoch → human-readable
- **No Web Worker**: xterm.js renders on main thread

### Comparison

| Feature | Puffer | Gamepanel |
|---------|--------|----------|
| **Transport** | Polling (5s) + event push | WebSocket (real-time) |
| **Rendering** | DOM divs (custom ANSI parser in Worker) | xterm.js (full terminal emulation) |
| **Background parsing** | Web Worker (yes) | Main thread (xterm handles it) |
| **Reconnect** | Not explicit | Exponential backoff 1s→30s |
| **Command history** | In-memory (100 max) | In-memory (50) + localStorage |
| **Search** | No | Yes (client-side filter) |
| **Stats/charts** | Separate component (`Stats.vue`) | Inline SVG sparklines |
| **Timestamps** | No | Yes |
| **Auto-scroll** | Manual (newest at bottom) | Toggleable |
| **Permission gating** | `hasScope()` checks | `hasServerPermission()` + `canConsole` |
| **Power controls** | `Controls.vue` (separate component) | Inline in console view |

### Verdict
Our console is **more advanced** — xterm.js + WebSocket + sparklines + search + reconnect logic is superior to Puffer's polling + DOM approach. However, Puffer's **Web Worker offloading** is worth copying for large scrollback buffers.

---

## 3. File Manager Comparison

### PufferPanel Files (`Files.vue`)
- **Breadcrumb navigation**: clickable path segments
- **Selection**: checkbox per file, select all/deselect all
- **Context menu**: right-click actions per file (download, delete, archive, extract)
- **Upload**: single file + directory upload (`webkitdirectory`)
- **Editor**: Ace editor in an overlay modal
- **Archive/Extract**: built-in archive button, extract on archives
- **Large file warning**: 30MB threshold before opening
- **Auto-refresh**: 5-min interval polling
- **Icon mapping**: file extension → named icon (`icon.name`)
- **Overlay modals**: create file, create folder, archive selected, loading, editor, file size warning

### Our File Manager (`files-view.tsx`)
- **Dual view**: list (table) and grid (cards)
- **Sorting**: by name, size, date — ascending/descending
- **Search/filter**: text filter within current directory
- **Drag & drop upload**: overlay when dragging files
- **Chunked upload**: progress tracking per file
- **Bulk operations**: move, copy, chmod, delete (sticky toolbar)
- **Right-click context menu**: download, rename, archive, extract, preview (images)
- **Image preview modal**: inline image viewer
- **Monaco Editor**: full code editor (syntax highlighting, git indicators)
- **Permission gating**: `file.read`, `file.create`, `file.update`, `file.delete`, `file.archive`, `file.read-content`
- **Keyboard shortcuts**: Delete, Ctrl+A, Ctrl+F, Escape
- **"Pull URL"**: download from remote URL directly to server
- **Short-lived download tickets**: secure one-time download URLs

### Feature Comparison

| Feature | Puffer | Gamepanel |
|---------|--------|----------|
| **List/Grid view** | List only | Both (list + grid) |
| **Sorting** | Name only (asc) | Name, size, date (asc/desc) |
| **Search** | No | Text filter |
| **Drag & drop** | No | Yes (with overlay) |
| **Chunked upload** | No | Yes (with progress) |
| **Bulk operations** | Delete only | Move, copy, chmod, delete |
| **Context menu** | Yes | Yes |
| **Image preview** | No | Yes (modal) |
| **Code editor** | Ace (overlay) | Monaco (full-page) |
| **Pull from URL** | No | Yes |
| **Download tickets** | Direct URL | Short-lived tokens |
| **Directory upload** | Yes (`webkitdirectory`) | No |
| **Large file warning** | Yes (30MB) | No |
| **Keyboard shortcuts** | Hotkey system | Native (Delete, Ctrl+A/F) |
| **Auto-refresh** | 5 min interval | On mutation only |
| **Permission scopes** | `server.files.edit`/`view` | 6 granular scopes |

### Verdict
Our file manager is **significantly more feature-rich** — drag-and-drop, bulk operations, dual view, sorting, search, image preview, pull-from-URL, and secure download tickets. Puffer's "file size warning" and "directory upload" are worth adding.

---

## 4. Admin Sections

### PufferPanel Admin Features
| View | Features |
|------|----------|
| **Server Admin** (`Admin.vue`) | JSON definition editor (6 tabs: Variables, Install, Run, Hooks, Environment, JSON), delete server |
| **Node List** (`NodeList.vue`) | List all nodes, reachability, environment types |
| **Node View** (`NodeView.vue`) | Edit name/host/port/SFTP port, deployment config (JSON), 5-step deploy guide, delete |
| **Node Create** (`NodeCreate.vue`) | Name, public/private host/port, SFTP port |
| **User List** (`UserList.vue`) | CRUD users |
| **Template List** (`TemplateList.vue`) | CRUD templates (server definitions) |
| **Template Create/View** | Visual editor with tabs |
| **Settings** (`Settings.vue`) | Panel settings (master URL, company name, registration toggle, theme selection) + Email settings (provider picker: none/SMTP/mailgun/mailjet, test email) |
| **Self** (`Self.vue`) | Personal account settings |
| **Invite** (`Invite.vue`) | Invite registration |

### Our Gamepanel Admin Features
| View | Features |
|------|----------|
| **AdminOverview** | Dashboard stats |
| **AdminNodes** | Node CRUD, allocations, sysinfo, configuration, deploy token |
| **AdminServers** | Server management |
| **AdminUsers** | User management |
| **AdminLocations** | Location CRUD |
| **AdminRegions** | Region CRUD |
| **AdminTemplates** | Template management |
| **AdminNestsEggs** | Nest/egg structure |
| **AdminSettings** | Panel settings |
| **AdminApiKeys** | API key management |
| **AdminDatabases** | Database management |
| **AdminMounts** | Mount management |
| **AdminAllocations** | Allocation management |
| **AdminOperations** | Operation monitoring |
| **AdminWebhooks** | Webhook management |
| **AdminPlugins** | Plugin management |
| **AdminActivityLog** | Activity log |
| **AdminHealth** | Health checks |
| **AdminRateLimitSettings** | Rate limiting |
| **AdminAccess** | Access control |
| **AdminOperations** | Operation system monitoring |
| **user-limits.tsx** | User resource limits |

### What Puffer Has That We Don't
1. **Server definition JSON editor** — Puffer lets admins edit the raw server definition with a tabbed GUI (Variables, Install steps, Run config, Hooks, Environment, and raw JSON). This is powerful for debugging.
2. **Deployment wizard** — 5-step deployment guide with copy-paste config after creating a node.
3. **Email provider config** — SMTP, Mailgun, Mailjet with test email button.
4. **Invite system** — Registration via invite tokens.
5. **Theme switching** — Runtime theme selection.
6. **Visual template editor** — Template editing with tabs for each section.
7. **Registration toggle** — Enable/disable self-registration.
8. **i18n** — All admin UI is translated.

### What We Have That Puffer Doesn't
1. **Operations monitoring** — Our `AdminOperations` page.
2. **Webhooks** — Configurable webhooks.
3. **Plugins** — Plugin system.
4. **Nests/Eggs** — Hierarchical template structure.
5. **Databases** — Database management per server.
6. **Mounts** — Mount management.
7. **Health checks** — System health monitoring.
8. **Rate limiting** — Configurable rate limits.
9. **Activity log** — Audit trail.
10. **Access control** — Granular permission management.

---

## 5. Server Creation Flow

### PufferPanel Server Creation (3-step wizard in `ServerCreate.vue`)
1. **Environment step** (`Environment.vue`) — Select node, environment type, server name, users
2. **Template step** (`SelectTemplate.vue`) — Browse/search templates filtered by env/os/arch
3. **Settings step** (`Settings.vue`) — Display variable groups with categorized inputs

```
Step 1 (Environment) → Step 2 (Select Template) → Step 3 (Configure Variables)
     ↑                      ↑                          |
     └── Back ──────────────┘ ←── Back ────────────────┘ → Create server
```

- Progress indicator (`progress.on-step-environment/template/settings`)
- Server types are loaded dynamically by `server.type` → `components/serverTypes/${type}.vue`
- Templates include: `install` (operation steps), `groups` (variable groupings), `data` (variable definitions)

### Our Gamepanel Server Creation
- No multi-step wizard exists in the admin panels I reviewed
- We have `AdminNestsEggs.tsx` (nests/eggs setup) but no `ServerCreate` equivalent in the admin views
- Server creation appears to happen via API only, or via a simplified form

### Comparison

| Feature | Puffer | Gamepanel |
|---------|--------|----------|
| **Multi-step wizard** | Yes (3 steps) | Not visible |
| **Progress indicator** | Animated dots | Not visible |
| **Template selection** | Filtered by env/os/arch | Via nests/eggs |
| **Variable groups** | Categorized (`groups[]` with `order`) | Not visible |
| **User assignment** | Step 1 (multi-user) | Not visible |
| **Node selection** | Step 1 (dropdown) | Via nest/egg config |
| **Back navigation** | Yes (each step) | Not visible |

### Recommendation
Implement a **3-step server creation wizard** with:
1. Node + environment selection (reuse our location/nodes API)
2. Template/nest selection filtered by environment capabilities
3. Variable configuration with group categorization (use `groups` from server definition)

---

## 6. Auth Flows: OAuth2 vs Session Comparison

### PufferPanel Auth Architecture
```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  Vue.js SPA │────▶│  Go/Gin API  │────▶│  OAuth2     │
│  (no SSR)   │     │  Bearer token│     │  /oauth2/*  │
└─────────────┘     └──────────────┘     └─────────────┘
                           │
                    ┌──────┴──────┐
                    │  Cookie:    │
                    │ puffer_auth │
                    └─────────────┘
```

- **OAuth2** for API access (`/oauth2/token` with `client_credentials`, `password` grants)
- **Cookie session** (`puffer_auth`) for browser login (set on login, validated in `AuthMiddleware`)
- **WebAuthn/Passkey** support — `PasskeyStart`, `PasskeyFinish` endpoints
- **TOTP (OTP)** — 6-digit 2FA codes
- **Scope-based permissions** — ~60 scopes defined in `web/loader.go` (e.g., `server.start`, `server.files.view`)
- **Two-tier permission check**: global perms + server-specific perms
- **Session re-auth** — 15-minute intervals in `App.vue` (`api.auth.reauth()`)
- **No JWT** — uses DB-backed sessions with token strings
- **Login flow**: email → password → (optional: passkey/TOTP 2FA)

### Our Gamepanel Auth Architecture
```
┌────────────────┐     ┌──────────────────┐
│ Next.js (SSR)  │────▶│ Go/Fiber API     │
│ /web           │     │ Session cookie   │
└────────────────┘     └──────────────────┘
```

- **Session-cookie-based** auth
- **Login → cookie → authenticated requests**
- No OAuth2 support for third-party clients
- No 2FA (TOTP/WebAuthn)
- No scope-based API permissions (admin-only flag)
- No API token management for external access
- No session re-auth mechanism

### Comparison

| Feature | Puffer | Gamepanel |
|---------|--------|----------|
| **Auth standard** | OAuth2 + Cookie session | Cookie session only |
| **API tokens** | OAuth2 (client_credentials) | Not implemented |
| **2FA** | TOTP + WebAuthn/Passkey | Not implemented |
| **Scopes** | ~60 granular scopes | Basic admin flag |
| **Permission inheritance** | Global + server-specific scopes | Not implemented |
| **Session re-auth** | 15-min interval | No |
| **Passwordless** | Passkey/WebAuthn | No |
| **SFTP auth** | OAuth2 password grant (user#server) | Not implemented |
| **Registration** | Toggleable + invite-only | Basic |

### Recommendation
Adopt Puffer's scope-based permission model. It's the single most impactful architectural improvement we could make:
1. Define ~40 scopes as Go constants (matching our existing `permissions` table concept)
2. Middleware checks scopes on every protected route (Puffer's `middleware/auth.go` + `middleware/middleware.go` pattern)
3. Expose scopes in API responses so the frontend can gate UI elements

---

## 7. Backend Architecture Comparison

### Server Lifecycle

| Phase | Puffer | Gamepanel |
|-------|--------|----------|
| **Pre-execution** | `PreExecution` operations (command list) | Not implemented |
| **Start** | `Start()` → pre-exec → run → keepalive | `POST /power` → daemon signal |
| **Stop** | `Stop()` → stop code or stop command | `POST /power` → daemon signal |
| **Kill** | `Kill()` → SIGKILL | `POST /power` → daemon signal |
| **Install** | `Install()` → full operation pipeline | `POST /install` → daemon signal |
| **Post-execution** | `afterExit()` → post-exec operations, auto-restart logic | Not implemented |
| **Auto-restart** | Configurable: `autorecover` (crash), `autorestart` (graceful) | Not implemented |
| **Keepalive** | `KeepAlive.Frequency` + `KeepAlive.Command` | Not implemented |
| **Scheduler** | `Scheduler` goroutine for timed tasks | Not implemented |
| **Stats collection** | 5-second interval, gopsutil, optional JVM stats | WebSocket stats stream |
| **Queue** | FIFO process queue with 1-second ticker | Not implemented |

### Operation System

| Aspect | Puffer (24 operations) | Gamepanel (our new system) |
|--------|----------------------|---------------------------|
| **Installation steps** | `install[]` array of operations | ServerDefinition operations |
| **Uninstall steps** | `uninstall[]` array of operations | Not implemented |
| **Pre/post hooks** | `pre[]`/`post[]` on Execution | Not implemented |
| **Conditional execution** | `if` field using CEL expressions | Not implemented |
| **Variable overrides** | Operations can return new variable values | Not implemented |
| **Condition functions** | `file_exists`, `in_path`, `is_server_running` (CEL) | Not implemented |
| **Dynamic dispatch** | Factory pattern with `Key()` + `Create()` | Factory pattern (similar) |
| **Token replacement** | `utils.ReplaceTokens()` in all string values | Not implemented |

### Console / WebSocket Architecture

| Aspect | Puffer | Gamepanel |
|--------|--------|----------|
| **Daemon→Panel** | HTTP proxy through panel to daemon | Direct WebSocket from frontend to daemon |
| **WebSocket proxy** | Panel proxies WS to daemon via `OpenSocket()` | Frontend connects directly |
| **Auth for WS** | Panel generates JWT token, passes to daemon | Session cookie + dedicated endpoint |
| **Log retrieval** | `getConsole(lastTimestamp)` — polling | `fetchServerLogs()` on mount |
| **Stats streaming** | Polled every 5s, sent via WS `MessageTypeStats` | Separate WebSocket for stats |
| **Console transport** | DB-backed message queue with epochs | WebSocket (real-time) |

### Database

| Aspect | Puffer (GORM) | Gamepanel (raw SQL) |
|--------|--------------|---------------------|
| **ORM** | GORM (auto-migrate, associations, preload) | Raw SQL queries |
| **Migrations** | Auto (GORM) | Manual SQL files |
| **Associations** | GORM `Preload(clause.Associations)` | Manual JOINs |
| **Transactions** | GORM transactions with middleware | Manual BEGIN/COMMIT |

---

## 8. Recommendations — Top Changes to Adopt from Puffer

### P0 — High Impact, Low Effort

1. **Scope-based permission model** (`middleware/auth.go` pattern)
   - Define Go constants for scopes matching our DB permission system
   - Add middleware that checks scopes on protected routes
   - Return permission scopes in API responses so frontend can gate UI

2. **Keyboard shortcuts** (`@github/hotkey` pattern)
   - Add a hotkey system using `useEffect` + keydown in Next.js
   - Map `r s` = stop, `r r` = restart, `c c` = command focus, `f l` = file list focus
   - This dramatically improves power-user experience

3. **Command history persistence to localStorage**
   - We already do this for console commands — extend to other input fields

### P1 — Medium Impact, Medium Effort

4. **Server creation wizard** (3-step: Environment → Template → Settings)
   - Implement in `AdminNestsEggs.tsx` or as a new `ServerCreateWizard` component
   - Progress indicator, back navigation, variable grouping

5. **Server definition JSON editor**
   - Add a tabbed editor for `ServerDefinition` (variables, install steps, run config, raw JSON)
   - Reuse our `MonacoEditor` component with JSON mode
   - Essential for advanced users debugging server configs

6. **WebAuthn/Passkey + TOTP 2FA**
   - Add to `/auth/*` endpoints using `github.com/go-webauthn/webauthn`
   - Login flow: email → password → (optional 2FA)
   - This is table-stakes for production hosting

### P2 — High Impact, Higher Effort

7. **Conditional operation execution (CEL expressions)**
   - Add `if` field support to our operation system
   - Use CEL (like Puffer) or a simpler expression language
   - Operations run conditionally based on `file_exists`, `is_server_running`, variable values

8. **Pre/post execution hooks + auto-restart**
   - Add `PreExecution`/`PostExecution` arrays to server execution config
   - Auto-restart on crash (`autorecover`) with crash counter and limit
   - Keepalive heartbeat command with configurable frequency

9. **i18n system**
   - Adopt `next-intl` or `react-i18next`
   - Extract all hardcoded strings into locale files
   - This is important if aiming for multi-tenant hosting

10. **Web Worker for console rendering**
    - Offload ANSI parsing and DOM manipulation to a Web Worker
    - Prevents main-thread jank with 1000s of console lines
    - Keep xterm.js but pipe output through a worker for filtering/searching

### P3 — Nice to Have

11. **Email provider configuration UI** — SMTP/Mailgun/Mailjet with test button
12. **Invite registration system** — invite tokens for new users
13. **Node deploy wizard** — 5-step setup guide after node creation
14. **File size warning** — warn before opening files >30MB in editor
15. **Theme switching** — runtime theme selection
16. **Scheduler for server tasks** — timed commands (restart at 4am, etc.)
17. **Session re-auth** — periodic re-validation of auth session
18. **Directory upload** — `webkitdirectory` support in file manager

---

## Summary

| Area | PufferStrength | GamepanelStrength |
|------|--------------|-------------------|
| **Console** | Web Worker, event-driven | xterm.js, WebSocket, sparklines, reconnect, search |
| **File Manager** | Large file warning, directory upload | Drag-drop, bulk ops, dual view, sorting, search |
| **Admin** | Definition editor, deploy wizard | More pages (webhooks, plugins, ops, activity log) |
| **Server Creation** | 3-step wizard with template filtering | Nests/eggs hierarchy |
| **Auth** | OAuth2, scopes, 2FA, WebAuthn | Simpler session model |
| **Operations** | 24 ops, CEL conditions, pre/post hooks | New system (can adopt Puffer patterns) |
| **Console WS** | HTTP proxy through panel | Direct WebSocket (simpler) |
| **Database** | GORM auto-migrate | Raw SQL (more control) |
| **Frontend** | 28 UI components, i18n, hotkeys | Monaco, xterm, Tailwind, charts |
| **i18n** | Full `vue-i18n` | None |
