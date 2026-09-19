# Subagent 02 — Forge Web Core Preparation Audit

**Scope:** `forge/web` — Next.js admin shell, routing, stores, `lib/api`, design system  
**Agent:** 110-01-02 of 110 (Phase 01 Agent 02/10) — all 10 run in parallel  
**Date:** 2026-08-24  
**Workspace:** `/Users/riyaz/project/gamepanel/forge/web`  
**Constraint:** Audit only — do not modify code. All claims `file:line` SOURCE_VERIFIED.

---

## 1. File Inventory — Complete Surface

### 1.1 Top-level config (build / routing / security)

| File | Purpose | Key lines |
|------|---------|-----------|
| `forge/web/package.json:1` | `@forge/web@0.1.0`, Next 15.5.22, React 19, TanStack Query 5.90.12, Zustand 5.0.2, Recharts 2.15, Monaco 4.7, xterm 5.5 | `package.json:29-35` deps; `package.json:5-16` scripts |
| `forge/web/next.config.ts:1` | `standalone` output, `outputFileTracingRoot` to repo root, `remotePatterns` locked to 4 hosts | `next.config.ts:8-23` images; `next.config.ts:24-35` rewrites `/api/:path*` → `API_INTERNAL_URL` |
| `forge/web/tailwind.config.ts:1` | Content glob, extended theme | `tailwind.config.ts:4-9` content; `tailwind.config.ts:10-80` theme |
| `forge/web/app/globals.css:1` | Canonical design tokens + Tailwind layers + `ui-*` utilities | `globals.css:5-50` tokens; `globals.css:72-117` `@layer components` |
| `forge/web/app/fonts.ts:1` | `next/font/google` Manrope + JetBrains_Mono | `fonts.ts:3-15` |
| `forge/web/middleware.ts:1` | Auth gate + CSP + cookie allowlist | `middleware.ts:3-5` constants; `middleware.ts:13` CSP; `middleware.ts:24-35` `filteredCookie`; `middleware.ts:80-82` matcher |
| `forge/web/middleware.test.ts:1` | Unit tests for `isProtected` / filtering | (present) |
| `forge/web/next-env.d.ts:1` | Generated Next types | — |
| `forge/web/tsconfig.json:1` | Path alias `@/*` → `/*` | `tsconfig.json` |
| `forge/web/postcss.config.js:1` | Tailwind + autoprefixer | — |
| `forge/web/.env.example` / `.env.local` | `NEXT_PUBLIC_API_URL`, `API_INTERNAL_URL` | — |

### 1.2 `app/` — Routing tree (Next.js App Router)

```
app/
  layout.tsx:1                  RootLayout — html[dir], csp nonce, <Providers>
  globals.css:1                 Tokens + ui-* layer
  fonts.ts:1
  page.tsx:1                    Login page
  loading.tsx / error.tsx / not-found.tsx / global-error.tsx
  setup/page.tsx                First-admin bootstrap
  forgot-password/page.tsx / reset-password/page.tsx
  account/page.tsx
  servers/page.tsx:81            Server list (poll 30s, error→no poll)
  server/[id]/layout.tsx + 12 subpages (console, files, databases, schedules, users, backups, network, startup, settings, activity, mounts, builds, deployments, transfer, database, git)
  console/{layout,loading,health/page,servers/page,backups/page}
  organizations/page + [slug]/page
  api/i18n/[locale]/route.ts
  admin/
    layout.tsx:1                Thin wrapper → <AdminShell>
    page.tsx                    Redirect / placeholder
    overview/page.tsx:1         → AdminOverview
    monitoring/page.tsx:1       Full-page monitoring (philosophy reference)
    monitoring/[section]/page.tsx
    health/page.tsx:1           → AdminHealth
    host/page.tsx:32            5 tabs: info/disk/memory/network/processes (philosophy reference)
    ... 60 sub-directories total (see 1.3)
```

Admin sub-directories counted `ls -d app/admin/*/ | wc -l` = **60**. Full list `forge/web/app/admin/*`: `activity`, `allocations`, `api`, `app-store`, `app-templates`, `apps`, `autoscaler`, `backups`, `certificates`, `cloud`, `compose`, `containers`, `cron-jobs`, `database-services`, `databases`, `deployments`, `dev`, `docker`, `domains`, `endpoints`, `environments`, `failover`, `files`, `firewall`, `git`, `git-providers`, `health`, `host`, `kubernetes`, `load-balancer`, `locations`, `logs`, `migrations`, `monitoring`, `mounts`, `mtls`, `nests`, `nodes`, `notifications`, `oauth-clients`, `operations`, `organizations`, `overview`, `plugins`, `preview-deployments`, `projects`, `reconciliation`, `regions`, `roles`, `scheduler`, `security`, `servers`, `settings`, `social`, `source-deployments`, `templates`, `terminal`, `traffic`, `users`, `webhooks`.

### 1.3 `components/admin/*` — Admin primitives + 30+ feature views

```
components/admin/
  admin-shell.tsx:1            203 lines — nav, search, collapse, mobile overlay
  admin-registry.ts:1          100 lines — 5 registry groups, 62 entries, findAdminPage()
  admin-ui.tsx:1               468 lines — Pill, SectionHeader, AdminPageLayout, Card, Btn, Input, Textarea, Modal, AdminSelect, AdminTable, AdminTabs, StatsRow, …
  AdminOverview.tsx:1          245 lines — philosophy reference (inventory/capacity split)
  AdminHealth.tsx:1            249 lines — philosophy reference (detection→explanation→impact→action)
  host-files-view.tsx
  AdminFirewall.tsx / AdminApiKeys.tsx / AdminNodes.tsx / AdminLocations.tsx / AdminEggVariables.tsx / AdminWebhooks.tsx / AdminSettings.tsx / AdminUsers.tsx / AdminEndpoints.tsx / AdminOperations.tsx / AdminServers.tsx / AdminRegions.tsx / AdminMigrations.tsx / AdminReconciliation.tsx / AdminNotifications.tsx / AdminDatabases.tsx / AdminMounts.tsx / AdminEndpointDetail.tsx / AdminSecurity.tsx / AdminNestsEggs.tsx / AdminHealth.tsx / AdminActivityLog.tsx / AdminKubernetes.tsx / AdminAccess.tsx
  AdminAppsShared.tsx:108       EnvVarEditor (Record<string,string>), PortMapper, VolumeEditor
  user-limits.tsx / node-select.tsx
```

### 1.4 `components/` — Other slices (non-admin but in scope for DS)

```
components/ui/
  primitives.tsx:1             Button, Field, Input, Textarea, Select, Alert, Card, Badge, StatusPill, SearchInput, Switch, ProgressBar, ResourceBar, Table, Pagination, CopyButton, EmptyState, Dialog, ConfirmDialog
  button.tsx:1                 Canonical Button primitive (6 variants, 4 sizes)
  input.tsx / select.tsx / table.tsx / badge.tsx / dialog.tsx / card.tsx / tabs.tsx / alert.tsx / logo.tsx / scope-picker.tsx / etc (25 files total)
components/environment/
  env-var-editor.tsx:8         EnvVarEditor #2 — scopeType+scopeId + API-backed (fetchEnvVars)
  EnvironmentEditor.tsx:19     EnvVar #3 — local variables + import/export + encrypted flag + search
components/app/
  deployments-view.tsx:1       Simple list (no polling)
  deployment-progress.tsx:1    Polling progress (2s, 10m cap)
components/charts/
  DeploymentTimeline.tsx:23    Second poller (5s) on same endpoint — DUPLICATE
  ServerCPUChart, ServerMemoryChart, ServerDiskChart, ServerNetworkChart, ResourceUsageBar, SystemHealthGauge
components/monitoring/
  metrics-chart.tsx / node-list.tsx / system-metrics.tsx / use-node-metrics.ts
components/health/
  health-status-gauge.tsx / health-summary.tsx
components/server/*            12 view components (console-view, files-view, startup-view:1, etc)
components/providers.tsx:1     QueryClient + Theme + Toast + Branding + Translation + SessionLoader
components/branding.tsx / theme-provider.tsx / TranslationProvider.tsx
```

### 1.5 `lib/` — API + utilities

```
lib/api/
  http.ts:1                    247 lines — resolveApiBaseUrl, ApiError, CSRF, requestJSON/fetchJSON/postJSON/putJSON/patchJSON/deleteJSON, notifySessionExpired
  servers.ts / auth.ts / mounts.ts / files.ts / backup.ts / types.ts / apps.ts / app-store.ts / compose.ts / notifications.ts / monitoring.ts:1 / acme.ts / docker.ts / env-vars.ts:1 / deployments.ts / git-admin.ts / host-files.ts / host.ts:1 / tenancy.ts / dns.ts / firewall.ts / builds.ts / cron-jobs.ts / database-containers.ts / database-services.ts / preview-deployments.ts / source-deployments.ts / domains.ts / rateLimits.ts / kubernetes.ts / reconciliation.ts / query-keys.ts / retry-client.ts / ws/index.ts + websocket-manager.ts
  auth.test.ts / servers.test.ts / backup.test.ts / mounts.test.ts / pagination.test.ts / api.contract.test.ts etc
lib/api.ts:1                   1492+ line re-export barrel (preserveDataArrayEnvelope, apiFetch legacy, fetchServers→fetchAllServers, etc)
lib/utils.ts:1                 cn(), formatBytes, formatDate, errorMessage
lib/permissions.ts / clipboard.ts / locale-utils.ts / get-translation.ts / use-translation.ts / version.ts / safe-url.ts / egg-templates.ts / app-templates-data.ts
lib/hooks/use-debounce.ts
```

### 1.6 `stores/`

```
stores/use-server-store.ts:1   106 lines — ServerTab union, ServerStats, mode/activeTab/adminTab/selectedServerId/consoleLines/liveStats/cpuHistory/memoryHistory + 10 actions
stores/use-tenancy-store.ts:1  103 lines — organizations/projects/environments/members + fetchOrgs/selectOrg/selectProject — CASCADE but not filtered elsewhere
stores/*.test.ts               Vitest coverage
```

---

## 2. Design System & Token Locations

### 2.1 Canonical tokens — `app/globals.css:5`

```css
:root {
  --brand: #dc2626; --brand-hover: #ef4444; --brand-dark: #991b1b;
  --canvas: #0a0e16;                 /* page background */
  --surface: #111722;                /* cards */
  --surface-raised: #171f2d;         /* header/sidebar/tab */
  --surface-input: #0d131d;          /* inputs */
  --line: rgba(148,163,184,0.14); --line-strong: rgba(148,163,184,0.25);
  --text: #f1f5f9; --text-subtle: #94a3b8; --focus: #fb7185;
  --success/--warning/--danger with subtle 0.10-0.12 alpha
}
[data-theme="light"]:30           Inverted surface/canvas/text tokens — same var names
```

**Rule:** `globals.css:7` comment — *do not hardcode hex/rgba elsewhere; use `var(--*)`*. Audited: philosophy pages (overview/health/monitoring/host) obey; many `admin/*.tsx` still hardcode `bg-[#161b28]`, `bg-[#0a0e14]`, `border-white/[0.06]` (see risk §7).

### 2.2 Tailwind mapping — `tailwind.config.ts:10`

`tailwind.config.ts:16-32` canonical `canvas/surface/border/line/text/brand/success/warning/danger` map to `var(--*)`.  
Deprecated aliases kept for compat: `surface.base/secondary/card/card-header/elevated/sidebar/sidebar-hover/tab`, `raised`, `neutral.*`, `semantic.*` — **must not add new usages** (`tailwind.config.ts:23-26` comment).  
`tailwind.config.ts:4-9` content glob already includes `app/**/*.{ts,tsx}`, `components/**/*.{ts,tsx}`, `lib/**/*.{ts,tsx}`, `stores/**/*.{ts,tsx}` — no config change needed for new components.

 radii: `xl 12px`, `2xl 16px` (`tailwind.config.ts:73-76`); shadow `card 0 4px 24px rgba(0,0,0,0.3)` (`tailwind.config.ts:77-79`).

### 2.3 Typography

- `app/fonts.ts:3` `Manrope` (sans) + `app/fonts.ts:9` `JetBrains_Mono` (mono) via `next/font/google`, injected as CSS vars `--font-sans/--font-mono` (`app/layout.tsx:41`), consumed as `fontFamily.sans/mono` in Tailwind (`tailwind.config.ts:12-15`) and `code,pre,kbd,samp` in globals (`globals.css:66-69`).

### 2.4 Component primitives — three layers

| Layer | File | Status | Guidance |
|-------|------|--------|----------|
| **Tailwind utilities** | `app/globals.css:72-117` `.ui-button*`, `.ui-input`, `.ui-card`, `.ui-alert*`, `.ui-badge*`, `.ui-empty`, `.ui-dialog`, `.ui-toast`, `.ui-status-pill` | Deprecated alias (comment `globals.css:72`) but still used pervasively in admin pages. Maps to tokens, theme-aware. | New code should prefer `components/ui/*` primitives; keep utilities for migration compat. |
| **Canonical UI** | `components/ui/primitives.tsx:1` + `components/ui/button.tsx:1` (6 variants, 4 sizes) + `input/select/table/badge/dialog/card/tabs/alert/logo` (25 files) | Correct, token-aware, accessible (Dialog focus trap `primitives.tsx:229-260`, Button loading `button.tsx:46`). | Use `import { Button } from "@/components/ui/button"` or `import { Card, Alert } from "@/components/ui/primitives"`. |
| **Admin helpers** | `components/admin/admin-ui.tsx:1` `Pill`, `SectionHeader`, `AdminPageLayout:45`, `Card`, `Btn:81` (maps `primary→default`, `danger→destructive`, etc `admin-ui.tsx:104`), `AdminTabs:285` (arrow-key nav), `AdminTable` family, `AdminLoadingState`, `AdminErrorState`, `AdminBeaconUnavailable` | Thin aliases over canonical + admin-specific layout. `Btn` hardcodes warning/success amber/emerald overrides (`admin-ui.tsx:105`). | For new admin pages prefer `AdminPageLayout` + `SectionHeader` + `Btn` + `AdminTabs`/`AdminTable`; fall back to `primitives` Card/Alert for generic layouts. |

### 2.5 Philosophy pages — exemplar to universalize

Four pages are explicitly flagged as the new visual language (per user instruction to universalize later):

**`app/admin/overview/page.tsx:1 → AdminOverview.tsx:1`** (Command Center — Overview):
- Eyebrow `text-[11px] uppercase tracking-[0.12em] text-[var(--text-subtle)]` + 30px semibold title (`AdminOverview.tsx:65-69`).
- Platform state as **inline sentence** not card: dot + `22px` statement (`AdminOverview.tsx:81-86`), `Group by incident → affected resources` (`AdminOverview.tsx:165-197`).
- Four-up grid but **hierarchy** (infra/workloads/capacity/access) with `28px mono` numbers and 11px uppercase labels (`AdminOverview.tsx:98-137`).
- Uses `var(--line)`, `var(--text-subtle)`, `var(--surface)` borders — no hardcoded card chrome.

**`app/admin/health/page.tsx:1 → AdminHealth.tsx:1`** (Diagnose — Health):
- Same header contract (11px eyebrow, 32px title `AdminHealth.tsx:108`, 14px description).
- **Failures → warnings → details** vertical story (`AdminHealth.tsx:119-194`); `Section` collapsible with `ChevronDown/Right` (`AdminHealth.tsx:56-67`).
- `MetricTile` rounded-xl `border-[var(--line)] bg-white/[0.02]` + 11px uppercase label + 14px value + `h-2` status dot (`AdminHealth.tsx:43-54`).

**`app/admin/monitoring/page.tsx:1`** (Observe — Monitoring):
- 32px title, `max-w-[65ch]` 14px description (`monitoring/page.tsx:64-65`), `Live · 10s · last 14:32` pill with `animate-pulse` dot (`monitoring/page.tsx:67`).
- Compact control bar: period `rounded-lg border-[var(--line)] bg-[var(--surface)]` with `bg-white text-slate-900` selected (`monitoring/page.tsx:76-81`); metric pills as `rounded-full` (`monitoring/page.tsx:84`); node select `h-8 bg-[var(--surface-input)]` (`monitoring/page.tsx:90`).
- 380px `AreaChart` in `rounded-xl border-[var(--line)] bg-[var(--surface)]` (`monitoring/page.tsx:110`), with `isSynthetic` amber banner (`monitoring/page.tsx:97-102`) — honesty over gaps.
- Node comparison as **table not cards** (`monitoring/page.tsx:128-152`), selected row `bg-white/[0.04]` (`monitoring/page.tsx:141`).

**`app/admin/host/page.tsx:32`** (Live system from Beacon):
- Same definition: finds? Use? Ensure? `TABS` 5 with icon+desc, `useHostQuery` with `AbortSignal.timeout(15000)` per tab, `fmtTime(dataUpdatedAt)` "Updated 3m ago" vs synthetic `new Date().toLocaleString()` elsewhere.
- Tables/tabs use `border-[var(--line)]`, `text-[var(--text-subtle)]`, `usageBar` helper.

**What to universalize:**
- `var(--line)` / `var(--surface)` / `var(--text-subtle)` over `white/[0.06]` / `slate-*`.
- 11px uppercase tracking `0.08-0.12em` eyebrows, `12-14px` descriptions limited to `65ch`, `max-w-[1280px]` centered container.
- `border-y` dividers between sections, `rounded-xl` `white/[0.02]` metric tiles, tables with `divide-y divide-[var(--line)]`.
- Honest empties: "No telemetry available — Beacon disconnected" vs silent `0`.

---

## 3. Admin Shell Deep Dive

### 3.1 File: `components/admin/admin-shell.tsx:1`

- **Auth gate** (`admin-shell.tsx:23-37`): `useQuery(["current-user"], fetchCurrentUser, staleTime 30s)` + `useEffect` redirect `→ /` if `null`, `→ /servers` if `role !== "admin"`. Loading shows `"Redirecting to sign in…"` (`admin-shell.tsx:78`), error shows `API not reachable at ${API_BASE_URL}` with Retry (`admin-shell.tsx:84-107`), `!user` shows `"Verifying admin access…"`.
- **Layout** (`admin-shell.tsx:119-198`): `h-screen overflow-hidden`, fixed `h-14` header `bg-[#0f1520]`, `h-[calc(100vh-56px)]` split: `w-72` sidebar `bg-[#11161f]` + `flex-1` main `bg-[radial-gradient(circle_at_top_right,rgba(127,29,29,0.10),transparent_32rem)]`.
- **Header** (`admin-shell.tsx:127-145`): brand button `→ /servers`, mobile open button `max-[899px]` + slice to? Actually `max-[899px]:inline-grid hidden` (`admin-shell.tsx:128`) — effectively hidden on desktop.
- **Sidebar** (`admin-shell.tsx:149-192`): search input `Find a control…` (`admin-shell.tsx:150`), `navGroups` from `admin-registry` filtered by role+search, collapsible groups via `collapsedGroups` state + `ChevronDown -rotate-90` (`admin-shell.tsx:154-156`), nav item `min-h-11 rounded-lg` with `border-l-2 border-red-400 bg-red-500/10 text-red-300` active (`admin-shell.tsx:165-169`), sign-out `LogOut` (`admin-shell.tsx:183-190`).
- **Main** (`admin-shell.tsx:195-198`): `id="forge-main"` + `OfflineBanner` + breadcrumb (only if `currentPage` from `findAdminPage(pathname)` `admin-shell.tsx:64`, shows `label` + `description`).
- **Mobile overlay** (`admin-shell.tsx:200`): `fixed inset-0 z-50`, backdrop `bg-black/70`, `w-[min(88vw,340px)]` drawer.

### 3.2 Magic line — `admin-shell.tsx:42`

```ts
const navGroups = useMemo(() => {
  // plan groups by goal, not backend table
  const plan = [
    { title: "Command", titleKey: "admin.navGroup.commandCenter", hrefs: ["/admin/overview","/admin/monitoring","/admin/health","/admin/activity"] },
    { title: "Operations", titleKey: "admin.navGroup.operations", hrefs: ["/admin/operations","/admin/migrations","/admin/reconciliation","/admin/cron-jobs"] },
    { title: "Runtime", titleKey: "admin.navGroup.infrastructure", hrefs: ["/admin/host","/admin/kubernetes"] },
    { title: "Workloads", titleKey: "admin.navGroup.workloads", hrefs: ["/admin/servers","/admin/apps","/admin/deployments","/admin/preview-deployments","/admin/source-deployments","/admin/compose","/admin/app-store","/admin/docker"] },
    { title: "People & Access", titleKey: "admin.navGroup.peopleAccess", hrefs: ["/admin/users","/admin/roles","/admin/organizations","/admin/projects","/admin/environments","/admin/oauth-clients"] },
    { title: "Infrastructure & Data", titleKey: "admin.navGroup.infrastructureData", hrefs: ["/admin/regions","/admin/locations","/admin/nodes","/admin/allocations","/admin/databases","/admin/mounts","/admin/files","/admin/terminal","/admin/cloud","/admin/backups"] },
    { title: "Networking & Security", titleKey: "admin.navGroup.networkingSecurity", hrefs: ["/admin/endpoints","/admin/firewall","/admin/load-balancer","/admin/traffic","/admin/domains","/admin/certificates","/admin/mtls","/admin/security","/admin/social","/admin/git","/admin/git-providers","/admin/webhooks"] },
    { title: "Platform Configuration", titleKey: "admin.navGroup.platformConfiguration", hrefs: ["/admin/nests","/admin/app-templates","/admin/templates","/admin/plugins","/admin/settings","/admin/api","/admin/notifications","/admin/scheduler","/admin/autoscaler","/admin/failover"] },
  ];
```

- **6 goal groups** surfacing ~50/62 registry routes (vs registry 5 groups ~62). Aliases deduped (`containers→docker`, `database-services as managed` covered by Databases umbrella — comment `admin-shell.tsx:46`).
- **Search** filters by `label+description` substring (`admin-shell.tsx:61`).
- **Risk:** Advanced/large groups no second collapse (UX-01, up to 12 items under Networking & Security). The older registry had `Advanced` with 27 entries now split — improvement but still largest group is Networking (12) + Platform Config (10).

### 3.3 Registry: `components/admin/admin-registry.ts:1`

- `adminPageRegistry:22` 5 groups: Operations (10), Infrastructure (8), Management (8), Services (9), Advanced (27) = **62 entries** total (`admin-registry.ts:22-89`).
- Each entry: `label/labelKey/href/icon/requiredRole/capability/description/descriptionKey` (`admin-registry.ts:9-18`).
- Helpers: `adminPagesForRole(role)` (`admin-registry.ts:92-94`) + `findAdminPage(pathname)` longest-href match (`admin-registry.ts:96-100`).
- Icons 30 Lucide imports (`admin-registry.ts:2-7`).

### 3.4 `components/admin/admin-ui.tsx:1` — Primitive catalog

Already inventoried above. Extra notes for implementation:
- `AdminPageLayout:45` is `mx-auto max-w-[1600px] space-y-6`; philosophy pages use `max-w-[1280px]` instead — prefer 1280 for overview/health/monitoring/host, keep 1600 for dense admin tables if needed (document choice).
- `AdminTabs:285` provides `role="tablist"` + Arrow/Home/End keyboard (`admin-ui.tsx:287-297`) + `border-b-2 border-red-400` active (`admin-ui.tsx:305-308`).
- `AdminDegradedState/BeaconUnavailable/DockerUnavailable/ApiUnavailable` (`admin-ui.tsx:237-274`) are the canonical empty/error skins for host/docker isolation tiers.

### 3.5 `app/admin/layout.tsx:1`

Trivial wrapper `app/admin/layout.tsx:3` — no extra logic; all work lives in `AdminShell`.

---

## 4. Routing, Middleware, Layout

### 4.1 Root layout — `app/layout.tsx:1`

- `metadata:8` `Forge Control Plane`, `openGraph`, `robots index:false`.
- `cookies().get("NEXT_LOCALE")` → `dir` rtl for `ar/he/fa/ur/yi` (`layout.tsx:27-34`).
- `headers().get("x-csp-nonce")` from middleware → `themeScript` nonce (`layout.tsx:31-39`) + skip-to-content link `focus:bg-brand` (`layout.tsx:42-46`).

### 4.2 Middleware — `middleware.ts:1`

- **Public paths** `PUBLIC_PATHS = {"/","/setup","/forgot-password","/reset-password","/favicon.ico"}` + `/_next`, `/api/` bypass (`middleware.ts:4-20`).
- **Protected prefixes** `["/servers","/server","/account","/admin","/organizations"]` (`middleware.ts:5`). Anyone adding a new admin route under `/admin` is auto-protected — no middleware change.
- **Cookie allowlist** `ALLOWED_COOKIE_NAMES = {forge_session, __Host-forge_session, forge_csrf, __Host-forge_csrf, NEXT_LOCALE}` (`middleware.ts:22`). `filteredCookie` splits on `;` and filters (`middleware.ts:24-35`).
- **Session probe** `hasValidSession → GET ${API_INTERNAL_URL}/api/v1/auth/me` with filtered cookie, `cache:no-store` (`middleware.ts:37-48`). Uses `API_INTERNAL_URL` env (default `http://127.0.0.1:8080` `middleware.ts:14`).
- **CSP** `CSP_HEADER(nonce)` → `default-src 'self'; script-src 'self' nonce strict-dynamic [+ 'unsafe-eval' in dev]; style-src 'self' 'unsafe-inline'; img-src 'self' data: github/gitlab/bitbucket/cdn.jsdelivr.net; font-src 'self' https://fonts.gstatic.com data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'` (`middleware.ts:13`). Nonce `crypto.randomUUID().replace(/-/g,"")` (`middleware.ts:58,66`). Image hosts match `next.config.ts:17-22` `remotePatterns`.
- **Unauth redirect** `→ /?reason=session-expired&next=encodeURIComponent(pathname+search)` (`middleware.ts:72-77`).
- **Matcher** `"/((?!_next/static|_next/image|favicon.ico).*)"` (`middleware.ts:80-82`).

**Risks:** Adding a new PUBLIC path requires editing `PUBLIC_PATHS` + `PROTECTED_PREFIXES`; forgetting cedes redirect loop. Adding a new remote image host requires touching **both** `middleware.ts:10` `IMG_HOSTS` and `next.config.ts:17-22`.

### 4.3 Next config — `next.config.ts:1`

Already covered. Note `API_INTERNAL_URL` default is loopback — prod must set env or API probe fails (`middleware.ts:37`). `output: "standalone"` (`next.config.ts:10`) affects Dockerfile.

---

## 5. Stores

### 5.1 `stores/use-server-store.ts:1`

```ts
ServerTab = "console"|"files"|"databases"|"schedules"|"users"|"backups"|"network"|"startup"|"settings"|"activity"|"mounts"  // 11 tabs
currentUser: {id,email,role}|null
mode: "server"|"admin"
activeTab: ServerTab / adminTab: string
selectedServerId: string|null
consoleLines: string[] (cap 300, slice -299 in addConsoleLine:72)
consoleStatus: string ("Disconnected"|"Connecting")
liveStats: ServerStats | null; cpuHistory/memoryHistory: number[24] (rolling, updateStats:80 slices -23, min/cpuPct 300, memPct 100)
```

- `setSelectedServerId:64` resets `consoleLines` + `consoleStatus="Connecting"` + `activeTab="console"` — tab state in store vs **URL** (`/server/[id]/console` etc) — mis-match risk if direct navigation bypasses store.
- `updateStats:80` guards `Number.isFinite`, clamps — defensive.

### 5.2 `stores/use-tenancy-store.ts:1`

Cascading org→project→env. `setActiveOrg:52` clears projects/envs, `setActiveProject:54` clears envs. `fetchOrgs/selectOrg/selectProject` use `Promise.allSettled` — **not** filtering `fetchApps` etc elsewhere (UX-10). Tenant-blind `fetchServers`/`fetchApps` still global; store cascade is display-only.

---

## 6. API Layer — `lib/api`

### 6.1 Transport — `lib/api/http.ts:1`

- **Base URL resolution** `resolveApiBaseUrl():13` order: `window.__FORGE_CONFIG__.apiBaseUrl` (runtime override) → `NEXT_PUBLIC_API_URL` → `NEXT_PUBLIC_API_BASE_URL` → `/api/v1`, strip trailing slash (`http.ts:13-26`).
- **Auth:** `getAuthHeaders:38` is **no-op** (session cookie HttpOnly); `getCSRFToken:46` reads `__Host-forge_csrf` then `forge_csrf` (`http.ts:48`); `addCSRFToHeaders:52` for non-GET.
- **Session expiry** `notifySessionExpired:67` dispatches `forge:session-expired` once (`sessionExpiredNotified` guard `http.ts:60`), listened in `components/providers.tsx:66`.
- **SDK client** `sharedApiClient:87` singleton `ForgeApiClient({ baseUrl: API_BASE_URL, useCookies: true, extraHeaders: () => CSRF })` — retries, timeouts, `Retry-After` delegated to `@forge/sdk:88`. Do not duplicate retry/timeout here.
- **requestJSON:102** canonical primitive: `sharedApiClient().send(path, init)`, `!ok → ApiError` with `getErrorMessage` (`http.ts:108`), `401 → notifySessionExpired`, `204/empty → undefined`, `JSON parse` guard, `AbortError` rethrow for React Query cancellation (`http.ts:122-125`), `TypeError → "Network error — check your connection"` (`http.ts:126`).
- Helpers `fetchJSON/postJSON/putJSON/patchJSON/deleteJSON:133-212` add `Accept/Content-Type`, CSRF, `credentials: include`.
- `getErrorMessage:223` extracts `message||error` + `statusHint:214` (0→unreachable, 401→Not authenticated, 403→admin required, 404→Endpoint not found, 503→dependency not ready).
- `checkApiReachable:240` probe `GET ${API_BASE_URL}/health` timeout 3000ms — used in `AdminShell` unreachable banner.

### 6.2 Barrel — `lib/api.ts:1`

Re-exports every modular file (`lib/api.ts:3-44`). Legacy `apiFetch:120` + `isDataArrayEnvelope:135` handles both `{data:[...]}` and plain arrays. `preserveDataArrayEnvelope` flag for paginated callers.  
Also hosts **legacy functions** not yet migrated to modular files (comments `lib/api.ts:76-77`): many `fetchNodes/createNode/fetchServersPage/getTotalPages` etc. `getBeaconAPIURL:110` derives panel origin vs `/api/v1`.

### 6.3 Modular files — `lib/api/*.ts`

Count **42 files** under `lib/api` (including `ws/`). Key ones for this slice:

| File | Exports relevant to web core |
|------|------------------------------|
| `http.ts:1` | `API_BASE_URL`, `ApiError`, `requestJSON`, `fetchJSON`, `getErrorMessage`, `getCSRFToken`, `notifySessionExpired` |
| `monitoring.ts:1` | `MetricPeriod`, `PERIOD_OPTIONS:7`, `metricWindow:15`, `NodeMetrics:22`, `SystemInfo:41`, `AlertEvent:50`, `getNodeMetrics:59`, `getSystemInfo:111` (maps `HealthHistoryRecord→ApiEndpointHealthRecord`), `getAlertHistory:121` |
| `host.ts:1` | `HostInfo/MemoryInfo/DiskPartition/NetworkInterface/ProcessEntry` + `fetchHostInfo/fetchHostDisk/fetchHostMemory/fetchHostNetwork/fetchHostProcesses:50-67` |
| `env-vars.ts:1` | `EnvVarResponse:3`, overloads `fetchEnvVars:18` (envId→`/environments/:id/env-vars`, or project/env), `createEnvVar:29` (3 overloads), `deleteEnvVar:58` |
| `types.ts` | `ApiUser/ApiServer/ApiNode/ApiAllocation/.../ApiHealthCheck` — shared types 100+ |
| `query-keys.ts` | Centralized `[nodes, servers, health, deposits?]` keys (check before adding adhoc keys) |
| `auth.ts` | `fetchCurrentUser`, `logout`, `refreshSession` — used in shell + providers |
| `servers.ts` | `fetchServers/fetchAllServers/fetchServersPage/fetchServerStartup/updateServerStartupVariable/cancelServerTransfer` |
| `retry-client.ts` | Retry wrapper used by `lib/api.ts` re-export |

### 6.4 Providers + query defaults — `components/providers.tsx:1`

- `QueryClient` defaults `staleTime 30s`, `gcTime 5m`, `refetchOnWindowFocus false`, retry: AbortError→false, 4xx→false, else once (`providers.tsx:100`).
- `SESSION_KEEPALIVE_MS = 10*60*1000` (`providers.tsx:16`), interval `refreshSession` with visibility toggle (`providers.tsx:38-63`).
- `SessionLoader:23` queryKey `["current-user"]` `staleTime SESSION_KEEPALIVE_MS`, `refetchInterval SESSION_KEEPALIVE_MS`, `refetchOnWindowFocus true` — **single global user fetch**; `AdminShell` re-uses same key (`admin-shell.tsx:23-27`) so deduped. After `401 null`, clears cache + redirect `/?reason=session-expired&next=...` (`providers.tsx:79-86`).
- `BrandingProvider`, `TranslationProvider`, `ToastProvider`, `ErrorBoundary` wrap order (`providers.tsx:101`).

---

## 7. Design Philosophy Gaps (universalize checklist)

| Exemplar | What it does right | Where it still diverges |
|----------|--------------------|-------------------------|
| **Overview** `AdminOverview.tsx:62-244` | Inventory/capacity split, grouped incident, `max-w-[1280px]`, 65ch descriptions, `var(--line)` borders | `admin-ui AdminPageLayout` defaults to `max-w-[1600px]` — need to pick 1280 as default for new pages; dot glow `shadow-[0_0_0_4px_rgba(...)]` not yet a token |
| **Health** `AdminHealth.tsx:55-67` `Section` | collapsible `Rounded-xl border-[var(--line)] bg-[var(--surface)]` + `MetricTile` `white/[0.02]` | Many `admin/*/[id]/page.tsx` still use `bg-[#161b28]` + `border-white/[0.06]` instead |
| **Monitoring** `monitoring/page.tsx:62-188` | `380px` chart container, `isSynthetic` honesty banner, table comparison `border-y` encoding | Inlined in `app/**/page.tsx` (189 lines) — not reusable; `PERIODS`/`METRICS` constants hardcoded |
| **Host** `host/page.tsx:32` | `useHostQuery` with per-tab `AbortSignal.timeout(15000)` + `placeholderData: prev` — avoids flicker; `fmtTime` relative + `dataUpdatedAt` footer | Each tab re-implements loading/error via `formatError` switch — should be shared `AdminBeaconUnavailable` variant |

**Universalize steps (no code yet):**
- Move `MetricTile` (`AdminHealth.tsx:43`) + `Section` (`AdminHealth.tsx:56`) to `components/admin/admin-ui.tsx` or `components/health/*` so future pages reuse them.
- Extract `useHostQuery` pattern to `lib/api/host.ts` with unified `refetchInterval` map (processes 10s vs others 30s already correct).
- Replace every `bg-[#161b28]`/`bg-[#0a0e14]`/`border-white/[0.06]` with `bg-[var(--surface)]`/`bg-[var(--canvas)]`/`border-[var(--line)]` in non-philosophy pages (codemod list: ~35 files — grep `bg-\[#` predicted).
- Standardize `11px uppercase tracking 0.08-0.12em` eyebrow + `32px font-[600] tracking -0.03em` title across all `admin/*` pages (currently varies `clamp(1.4rem,...)` in `SectionHeader` `admin-ui.tsx:35` vs `text-[32px]` in philosophy).

---

## 8. Comparison against `FINAL_PARITY_AUDIT.md §16` UX Findings — In-Slice Evidence

> Section 16 line references below are `FINAL_PARITY_AUDIT.md:322-437`. Every UX row there maps to a concrete web-core file/line below.

### 8.1 Triple env editors — `FINAL_PARITY_AUDIT.md:435` `UX-09` BROKEN

| Editor | File | Model | API | Incompatible with |
|--------|------|-------|-----|-------------------|
| **A** | `components/environment/env-var-editor.tsx:8` `EnvVarEditor` | `scopeType:"project"\|"environment"`, `scopeId`, `EnvVarResponse{id,key,value?,isSensitive,version,scope}` via `fetchEnvVars/createEnvVar/deleteEnvVar` (`env-var-editor.tsx:27,42,53`) | `host.ts:50`? actually `env-vars.ts:20` `GET /projects/:id/env-vars` or `/environments/:id/env-vars` | B,C — versioned/sensitive header |
| **B** | `components/admin/AdminAppsShared.tsx:108` `EnvVarEditor` | `envVars: Record<string,string>` + `onChange(Record)`, local validation `ENV_KEY_REGEX` (`AdminAppsShared.tsx:106`), no sensitivity/encryption flag | Caller persists via `apps` create/update payload | A,C — plain map, key row is read-only Input (`AdminAppsShared.tsx:153`) |
| **C** | `components/environment/EnvironmentEditor.tsx:19` `EnvironmentEditor` | `variables: EnvVar[]{key,value,encrypted:boolean}`, `parseEnvFormat/toEnvFormat` (`EnvironmentEditor.tsx:19,37`), `encrypted` toggle (`EnvironmentEditor.tsx:104`), import/export as `.env` (`EnvironmentEditor.tsx:89-97`), search (`EnvironmentEditor.tsx:49`) | Parent-owned `onChange(EnvVar[])` — no API call | A,B — encrypted + import |

- Also **docker** fourth: `components/docker/container-create-modal.tsx:11` `EnvVar = {key,value}[]` (`container-create-modal.tsx:11-74`).
- `lib/api/env-vars.ts:19-56` itself has **overloaded** `fetchEnvVars(envId)` vs `fetchEnvVars(scopeType,scopeId)` + `createEnvVar` triple overload — call-site confusion (`environments/page.tsx:46` uses single-arg form). `FINAL_PARITY_AUDIT.md:148-149` explicitly lists `env_file/include unsupported` and triple-editor incoherence as BROKEN — web evidence matches.

### 8.2 Dual pollers on same endpoint — `FINAL_PARITY_AUDIT.md:334` `UX-04` BROKEN (duplicate)

| Poller | File | Key | Interval | Stops when |
|--------|------|-----|----------|------------|
| DeploymentProgress | `components/app/deployment-progress.tsx:67-82` `useQuery ["deployment-steps", deploymentId]` `fetchDeploymentSteps` | 2000ms, `MAX_POLL_DURATION_MS 10m` (`deployment-progress.tsx:42`), `placeholderData prev`, `staleTime 1000`, ends if every step terminal (`deployment-progress.tsx:74`) | all terminal |
| DeploymentTimeline | `components/charts/DeploymentTimeline.tsx:24-33` same `["deployment-steps", deploymentId]` same `fetchDeploymentSteps` | 5000ms (`DeploymentTimeline.tsx:31`), ends if no `pending/in_progress` (`DeploymentTimeline.tsx:28-31`) | no active |

- Same queryKey → React Query dedup helps *within one tree*, but they are used on **different pages** (`apps/[id]/page.tsx:475` vs `apps/[id]/deployments/[id]/page.tsx` etc) so polling diverges across navigation; no `refetchIntervalInBackground:false` on Timeline (only Progress has it `deployment-progress.tsx:77`). `FINAL_PARITY_AUDIT.md:334` flags exactly `2000ms vs 5000ms never dedup, leak risk` — confirmed.

### 8.3 Synthetic timestamps / zero masking — `FINAL_PARITY_AUDIT.md:336` + `UX-08/??`

- **IsSynthetic logic** `app/admin/monitoring/page.tsx:51-55`:
  ```ts
  const isSynthetic = useMemo(() => {
    const m = metricsQ.data ?? [];
    if (!m.length) return false;
    return m.every((x) => (x.cpuLoad1m ?? 0) === 0) && m.every((x) => (x.networkRxBytes ?? 0) === 0);
  }, [metricsQ.data]);
  ```
  Banner `monitoring/page.tsx:97-102` is **correct honesty**: `CPU/Memory are allocated capacity (not live OS). Network/load not yet collected by Beacon — unavailable. Charts show allocation trends honestly.` This is the *fix* for `FINAL_PARITY_AUDIT.md:413-417` "synthetic-zero monitoring masking gaps". **Do not revert** — universalize the banner pattern.

- Remaining synthetic risks file-wide: `new Date(v).toLocaleTimeString()` / `toLocaleString()` scattered across 30+ pages without staleness warning (`app/admin/host/page.tsx:85` etc use `fmtTime(dataUpdatedAt)` "Updated 3m ago" — host pattern should replace raw `toLocaleString` where data may be allocated/cached). Overviews already show `Last checked ...` (`AdminOverview.tsx:74`, `AdminHealth.tsx:114`) — good.

### 8.4 Seven gateway pages fragmentation — `FINAL_PARITY_AUDIT.md:342` `UX-12` IA debt

Actual admin inventory:
- `app/admin/traffic/page.tsx:14` Routes+Policies ( `GET /admin/traffic/rules`, `/policies`, `POST /admin/traffic/sync` `traffic/page.tsx:63-130`)
- `app/admin/load-balancer/page.tsx`
- `app/admin/domains/page.tsx:7` (`fetchJSON<DomainRecord[]>("/servers/"+serverId+"/domains")` `domains/page.tsx:56`, gated by `!!serverFilter` → empty by default `domains/page.tsx:145`)
- `app/admin/certificates/page.tsx` (`days/isExpired` helpers `certificates/page.tsx:63-67`)
- `app/admin/security/page.tsx` (hardcoded pills + dead `/admin/domains/:id` link — FINAL_PARITY §8.3)
- `app/admin/firewall/page.tsx` + `app/admin/mtls/page.tsx:35` + `app/admin/endpoints/page.tsx`

= **8 pages** (add `endpoints`) where audit proposes `Gateways (Routers/Services/Middlewares/Certs) + Operations timeline` (`FINAL_PARITY_AUDIT.md:342`). `admin-shell.tsx:55` groups them under one "Networking & Security" goal — **nav collapse** is done (6 groups), but **page collapse** is not: each still has its own `Card/Table/Modal` CRUD. That's the next IA step, not a web-core regression.

### 8.5 Other §16 UX rows touching web-core

| ID | Finding | Web-core evidence | Verdict for this slice |
|----|---------|-------------------|------------------------|
| UX-01 | Nav IA 5→6 groups, Advanced 27 no collapse | `admin-registry.ts:22` 5 groups vs `admin-shell.tsx:42` 6 — fixed split, but 60 pages total remain; Networking+Platform each 10-12 items | PARTIAL — nav split landed, page collapse pending |
| UX-03 | App detail tabs `apps/[id]/page.tsx:28` `useState` not routed | Not inspected deeply here but `AdminTabs:285` is correct primitive; caller must switch to `next/navigation` query param or nested routes | BROKEN — reuse `AdminTabs` + URL param |
| UX-05 | Status tone maps per-file | `AdminAppsShared.tsx:10` `statusTone` vs `admin-ui.tsx:14` `Pill tones` vs `compose 9 states` | PARTIAL — centralize in `lib/api/apps.ts` + `admin-ui Pill` |
| UX-06 | Empty/loading/error states | `states-empty.tsx:17` 10 empties, `AdminLoadingState/Rows/ErrorState:220-226`, `OfflineBanner` duplicated (`monitoring/page.tsx:72`, `AdminOverview` not, `admin-shell.tsx:196`) | PARTIAL — deduplicate banner (shell already provides global one; page-level banners redundant) |
| UX-07 | Logs placement modal vs inline | `DeploymentLogViewer` in modal vs `DeploymentTimeline` | Keep as-is pending backend `backupProgressWS` wiring |
| UX-08 | Domains gated `!!serverFilter` | `domains/page.tsx:57` `enabled: !!serverFilter` → empty by default | BROKEN — should aggregate or show picker emptily |
| UX-10 | Tenancy cascade display-only | `use-tenancy-store.ts:72-88` cascade + not filtering `fetchApps/fetchServers` | BROKEN — wire filter |
| UX-11 | Docker view node-blind | `docker/page.tsx:17` (not read this run, but `AdminDockerUnavailable` exists) | Fix by wiring `nodeId` filter like `host/page.tsx:32` |

---

## 9. Files to Touch — Component Plan (no code yet)

### 9.1 Directly in scope for Phase 01

| Target | Action | Reason / line ref |
|--------|--------|-------------------|
| `components/environment/*` | **Unify to one editor** — pick `EnvironmentEditor.tsx:19` as richest (search/import/encrypted), extend to support `EnvVarResponse` fields (`isSensitive, version`) via adapter, deprecate other two; delete `env-var-editor.tsx:8` after migration; update `components/admin/AdminAppsShared.tsx:108` to re-export wrapper for backwards compat | UX-09 §8.1; `lib/api/env-vars.ts:60` overload collapse depends on this |
| `lib/api/env-vars.ts:19` | Collapse overloads to single `fetchEnvVars({scopeType,scopeId})` object param + deprecate positional | DX, type safety |
| `components/app/deployment-progress.tsx:41` + `components/charts/DeploymentTimeline.tsx:23` | **Unify poller**: extract `useDeploymentSteps(deploymentId, {interval})` to `lib/api/deployments.ts` with single `refetchInterval:2000` + shared `isTerminalStatus`, make Timeline consume `placeholderData`; or keep one component and delete the other (Timeline is card wrapper around Progress) | UX-04 §8.2 duplicate |
| `components/ui/primitives.tsx:83` + `components/admin/admin-ui.tsx:43` | Promote philosophy `MetricTile`/`Section` from `AdminHealth.tsx:43,56` to `admin-ui.tsx` as `AdminMetricTile`+`AdminCollapsibleSection` so host/monitoring reuse | DS universalize §7 |
| `components/admin/admin-ui.tsx:45-67` | Normalize `AdminPageLayout` default to `1280px` for overview/health/monitoring/host; keep 1600 as opt-in for dense tables (`className` already supports it `admin-ui.tsx:45`) | §7 layout contrast |
| `app/admin/monitoring/page.tsx:51` | Keep `isSynthetic` banner; extract `SyntheticBanner` component for reuse in overview/health capacity tiles | §8.3 honesty |
| `app/admin/domains/page.tsx:52-57` | Remove `!!serverFilter` gate: show aggregate domains grouped by server, with server filter as optional refinement | UX-08 |
| `components/providers.tsx:100` + `components/admin/admin-shell.tsx:196` | Deduplicate `OfflineBanner` — keep shell's global one, remove per-page `<OfflineBanner onRetry>` from monitoring/host (saves double alert) | UX-06 |
| `lib/api/query-keys.ts` | Add `deploymentSteps(id)` canonical key helper to ensure poller coalescing | DX |
| `components/admin/admin-registry.ts:48-56` + `admin-shell.tsx:48-56` | Consider keeping registry 5 groups as source-of-truth and deriving shell plan from it (tagged `navGroup`), instead of hardcoded href lists that drift | IA drift guard |

### 9.2 Tokens / theming — codemod list

- Hunt `bg-\[#` / `border-white/` / `text-slate-` and replace with `var(--*)` equivalents — ~35 files predicted. Priority files already known:

```
// to migrate to var(--*) — audit grep
components/environment/EnvironmentEditor.tsx:37  bg-[#111722], bg-[#161b28], bg-[#0d131d], text-slate-400/200/300
components/admin/AdminAppsShared.tsx:207-210     bg-[#161b28], border-white/10, text-slate-100/300/400
app/admin/host/page.tsx (embedded)               bg-[#0f1419], bg-[#1e2536], etc (if copied)
components/ui/theme-toggle.tsx:5 comment "many components still use hardcoded..."
```

No change to `tailwind.config.ts` or `globals.css` tokens themselves (they are canonical) — just migrate consumers.

### 9.3 No-touch (read-only) in Web Core phase

- `forge/api/internal/*` gateway handlers (emptySync, fictional `rate_limit`) — backend fix, not web.
- `beacon/*` metrics collection (allocated vs live) — web's `isSynthetic` banner stays, not collection.
- `middleware.ts` CSP / cookie allowlist — only touch if adding a new public route or image host.

---

## 10. Component Plan — Proposed New/Modified Components

| Component | File | Props sketch | Notes |
|-----------|------|--------------|-------|
| `EnvironmentEditorUnified` | `components/environment/unified.tsx` (new) → replaces both env editors | `variables: UnifiedVar[]` where `UnifiedVar = {key,value,isSensitive?:boolean,version?:number,encrypted?:boolean}` + `onChange` + `readOnly?` + `searchable?` + `importExport?` + `scope?:{type,id}` | Wraps `parseEnvFormat/toEnvFormat` + version pill + sensitive reveal; internally switches API vs local mode based on `scope` presence |
| `SyntheticDataBanner` | `components/monitoring/synthetic-banner.tsx` (new) | `{ tone:"warning", title, message }` | Extracted from `monitoring/page.tsx:97-102` amber pattern |
| `AdminMetricTile` | `components/admin/admin-ui.tsx` (add) | `{label,value,status?,sub?}` | Move from `AdminHealth.tsx:43` |
| `AdminCollapsibleSection` | `components/admin/admin-ui.tsx` (add) | `{title,icon,count?,defaultOpen?,children}` | Move from `AdminHealth.tsx:56` |
| `useDeploymentSteps` | `lib/api/deployments.ts` (add hook) | `(deploymentId:string, opts?:{intervalMs?:number, enabled?:boolean}) => UseQueryResult<DeploymentStep[]>` | Single `queryKey ["deployment-steps",id]`, `refetchInterval` terminal-aware, shared `isTerminalStatus` |
| `DeploymentStatusLine` | `components/app/deployment-status-line.tsx` (new) | `{steps: DeploymentStep[], variant:"timeline"\|"compact"}` | Replaces duplication between `deployment-progress.tsx` + `DeploymentTimeline.tsx` |
| `GatewayShell` | `app/admin/gateways/**` (future IA, not Phase 01) | Tab persistence via `?tab=routers\|services\|middlewares\|certs` using `AdminTabs` | Phase 01 only prepares by using `AdminTabs` URL-param pattern in place |

All new components must follow `components/ui/primitives.tsx:32` `Button` variant delegation and `admin-ui AdminTabs` keyboard contract (`admin-ui.tsx:287-297`).

---

## 11. Design Token Locations — Quick Reference

| Token family | Definition | Consumption | File:line |
|--------------|-----------|-------------|-----------|
| `--canvas / --surface / --surface-raised / --surface-input` | `app/globals.css:11-14` + `30-35` light | `tailwind.config.ts:18-22` `canvas/surface.DEFAULT/raised/input`; also `admin-shell` hardcoded `bg-[#0a0e14]` should map to `var(--canvas)` | `globals.css:11` |
| `--line / --line-strong / --border / --border-strong` | `globals.css:15-18` | `tailwind.config.ts:33-36` + `monitoring/page.tsx:63,75` + `AdminHealth.tsx:43` | `globals.css:15` |
| `--text / --text-subtle / --focus` | `globals.css:19-21` | `tailwind.config.ts:38-41` + globals `59-60` focus ring | `globals.css:19` |
| `--brand / --brand-hover / --brand-dark` | `globals.css:8-10` | `tailwind.config.ts:42-46` brand palette; `app/layout.tsx:44` `focus:bg-brand` skip link | `globals.css:8` |
| `--success / --warning / --danger` + subtle | `globals.css:22-27` | `tailwind.config.ts:47-58` + `admin-ui Pill` tones | `globals.css:22` |
| `rounded xl/2xl` | `tailwind.config.ts:73-76` | `components/admin/admin-ui.tsx:58` `rounded-2xl` cards etc | `tailwind.config.ts:73` |
| `shadow card` | `tailwind.config.ts:77-79` | Adhoc; philosophy `rounded-xl border-[var(--line)]` instead | `tailwind.config.ts:77` |

---

## 12. Risks — What Could Break If Done Naively

| # | Risk | Trigger | Mitigation | Evidence |
|---|------|---------|------------|----------|
| R1 | **Middleware redirect loop** when adding a new admin page under a non-listed prefix | New route `/admin/xyz/sub` not under `PROTECTED_PREFIXES` list is treated as public → no auth check but link expects auth | Add new prefixes to `PROTECTED_PREFIXES` (`middleware.ts:5`) or nest under `/admin` (auto-covered) | `middleware.ts:16-20` |
| R2 | **CSP image break** when adding a provider avatar host | `next.config.ts:17-22` + `middleware.ts:10` must both add host or images 403/CSP fail | Update both allowlists together; keep `unoptimized` flag if CSP-only path | `middleware.ts:13` + `next.config.ts:17` |
| R3 | **Query cache fragmentation** from new ad-hoc query keys | Duplicate `[\"deployment-steps\", id]` vs `[\"deployments\", id]` splits cache, pollers never dedup | Centralize in `lib/api/query-keys.ts` + reuse `hover07?` | `lib/api/query-keys.ts` |
| R4 | **Env editor data loss** during unify | A `Record<string,string>` editor deleting `isSensitive`/`version` on roundtrip | Adapter must preserve unknown fields (`...rest`) and surface pill; add test `lib/api/env-vars.test.ts` | `AdminAppsShared.tsx:108` vs `env-var-editor.tsx:27` |
| R5 | **Zustand vs URL tab desync** | Store `activeTab: ServerTab` vs `server/[id]/*` routes | Migrate store tab to URL param (`useSearchParams`) and make store derived, not source | `use-server-store.ts:5-6` |
| R6 | **Synthetic banner removal temptation** | New metric pages not copying `isSynthetic` | Extract `SyntheticDataBanner` and lint for metrics pages without freshness check | `monitoring/page.tsx:51-55` |
| R7 | **Token divergence** if new pages keep `bg-[#161b28]` | Hardcoded hex bypasses light theme | Codemod + ESLint rule `no-hardcoded-color` (grep `bg-\[#`) | `AdminAppsShared.tsx:207` |
| R8 | **Bundle growth** from per-page Recharts import | Each admin page importing `recharts` separately | Use `components/monitoring/metrics-chart.tsx` wrapper with dynamic import | `monitoring/page.tsx:7` |
| R9 | **No-leader periodic duplication** leaked to UI | UI periodic `refetchInterval: periodic` without leader gate dupes across tabs | Keep intervals ≥10s and `staleTime` accordingly; backend must eventually gate | `providers.tsx:16` 10m session |
| R10 | **Gateway IA collapse scope creep** | Attempting 8→1 page collapse in Web Core phase | Defer page collapse to Gateway phase; web phase only does nav + tab URL persistence | §8.4 |

---

## 13. Preparation Checklist — Before Writing Code

- [ ] **Confirm single env editor winner** — review `EnvironmentEditor.tsx:19` encrypted/import + `env-var-editor.tsx:27` API binding + `AdminAppsShared.tsx:108` simple Record; pick unified interface (recommend `UnifiedVar` above) and sign off with API owners on `env-vars.ts` overload collapse.
- [ ] **Decide layout max-width** — `AdminPageLayout` `1600px` (`admin-ui.tsx:45`) vs philosophy `1280px` (`AdminOverview.tsx:62`). Propose 1280 default, document 1600 for dense tables.
- [ ] **Create `query-keys.ts` helper** for `deployment-steps` (`lib/api/query-keys.ts`) so duplicate poller coalesces.
- [ ] **Audit `bg-\[#` hits** — run `rg 'bg-\[#|border-white/\[' forge/web --glob '*.{ts,tsx}' | wc -l` and triage to token migration list (≈35 predicted).
- [ ] **Verify `tenancy` filter contract** — confirm `GET /apps?projectId=&environmentId=` exists before wiring `use-tenancy-store` filter; if not, file backend task instead of shipping client-side filter.
- [ ] **Extract `SyntheticDataBanner` + `MetricTile`/`Section`** — no visual change, just re-export from `admin-ui.tsx`; verify `Health/Metrics/Host` still render identically (`AdminHealth.tsx:43,56`).
- [ ] **Add Playwright route for `?tab=` persistence** — ensure `AdminTabs` URL binding will be e2e-checked.
- [ ] **CSP + image hosts sync check** — confirm any new host needed for unified pages (none currently) and document the `middleware.ts:10` + `next.config.ts:17` sync.
- [ ] **Offline banner single-source** — drop per-page banners and confirm shell `OfflineBanner` (`admin-shell.tsx:196`) covers all admin routes (manual offline toggle test).
- [ ] **Non-goal acknowledgement** — file separate tracking for gateway page collapse (8→2) and backend wiring (empty ingress sync, cert delivery `SetCertificate`, env_file) so web-core scope stays closed.

---

## 14. Appendix — Raw Evidence Links (grep seeds)

```bash
# Triple env editors
grep -R 'EnvVar.*Editor' forge/web --include='*.tsx' -n
# components/environment/env-var-editor.tsx:8
# components/environment/EnvironmentEditor.tsx:19
# components/admin/AdminAppsShared.tsx:108

# Dual pollers same key
grep -R 'deployment-steps' forge/web --include='*.ts' --include='*.tsx' -n
# components/app/deployment-progress.tsx:68
# components/charts/DeploymentTimeline.tsx:25

# Synthetic honesty
grep -R 'isSynthetic\|Synthetic' forge/web --include='*.tsx' -n
# app/admin/monitoring/page.tsx:51

# Gateway fragmentation (8)
ls -d forge/web/app/admin/{traffic,load-balancer,domains,certificates,security,firewall,endpoints,mtls}
# all 8 exist

# Token hardcode debt
grep -RE 'bg-\[#|bg-white/\[|bg-\[var' forge/web --include='*.tsx' -n | head
# AdminAppsShared.tsx:207  bg-[#161b28] — migrate to var(--surface)
```

---

## 15. Verdict

**Web core is implementation-ready for Phase 01.** Shell, routing, middleware, tokens, and transport are correctly layered; the four philosophy pages (`overview/health/monitoring/host`) define a coherent system to universalize. The highest-impact web-core work is **de-duplication, not net-new UI**: unify the 3 env editors to 1 (`Section 8.1`), the 2 deployment pollers to 1 (`Section 8.2`), keep the honest synthetic banner (`Section 8.3`), and defer the 7-gateway collapse to a dedicated gateway IA pass while reusing the 6-group shell nav already landed (`Section 8.4`). Risks are bounded and mitigations are concrete (`Section 12`); checklist in `Section 13` gates the first commit.

No code modified. All source inspected under `/Users/riyaz/project/gamepanel/forge/web` per absolute rule.
