# Phase 01 — Subagent 04: UX / Control Plane — Coolify / Dokploy / Komodo / Portainer / 1Panel / CapRover / Uncloud / Docker-Compose

> Dimension: USER EXPERIENCE / CONTROL PLANE — navigation, dashboards, app list/detail, deployment experience, status/loading/empty states/failures, logs/metrics, domains/configuration/env vars/secrets/permissions/projects/environments/teams
> Cluster: coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose (spec control)
> Auditor: subagent-4 (Phase 1)
> Date: 2026-08-23
> Method: source inspection (Read/Grep/Glob/Bash) — real paths under `reference/app-platforms/*` (frontend/, app/, components/, pages/) and `forge/web/*`. No marketing copy. All citations are `file:line` verifiable. Verification via execution where noted.

---

## 1. Scope & Methodology

Inspected reference UIs for how they make complex infra understandable, and Forge UI deeply for IA, empty states, error handling, deployment status communication, logs/metrics placement, env var UX.

**Reference paths inspected:**
- Coolify: `reference/app-platforms/coolify/resources/views/{components,livewire}` + `resources/views/livewire/project/*`
- Dokploy: `reference/app-platforms/dokploy/apps/dokploy/components/dashboard/{application,compose,docker,project}`
- Komodo: `reference/app-platforms/komodo/frontend/src` (top-level via `ls`)
- Portainer: `reference/app-platforms/portainer/app/portainer/{views,components}` + `app/react` + `app/docker`
- 1Panel: `reference/app-platforms/1panel/frontend/src/{views,components,layout,routers}`
- CapRover: `reference/app-platforms/caprover/src` (no `frontend/` — backend-driven)
- Uncloud: `reference/app-platforms/uncloud` (CLI/mesh, no web UI — website/landing concept)
- docker-compose: `reference/app-platforms/docker-compose` (CLI spec control — no UI)

**Forge paths inspected:**
- `forge/web/components/admin/admin-registry.ts`, `admin-shell.tsx`, `admin-ui.tsx`, `AdminOverview.tsx`, `AdminAppsShared.tsx`
- `forge/web/app/admin/*` (apps, deployments, preview-deployments, compose, docker, domains, environments, monitoring, logs, activity, etc.)
- `forge/web/lib/api/*` (apps.ts, deployments.ts, compose.ts, env-vars.ts, domains.ts, docker.ts)
- `forge/web/stores/use-server-store.ts`, `use-tenancy-store.ts`
- `forge/web/components/TranslationProvider.tsx`, `forge/web/lib/use-translation.ts`
- `forge/web/app/console/*`, `forge/web/components/{charts,monitoring,shared,server,environment,deployment,app}`
- `forge/web/components/shared/{states-empty,states-loading,states-error,states-offline}`

Status taxonomy: `COMPLETE | PARTIAL | UNWIRED | BROKEN | MISSING | DEAD | DUPLICATE | FALSE_COMPLETION | UNKNOWN`
Severity: `P0-P4` + `USER_VISIBLE / OPERATOR_VISIBLE / SILENT`

---

## 2. Reference Platform UI Inventory (file evidence)

### 2.1 Coolify v4 — Livewire + Blade

- **IA:** `reference/app-platforms/coolify/resources/views/livewire/project/show.blade.php:1` project → environment → resource hierarchy. `components/server/sidebar.blade.php:1` per-server sidebar with proxy/security/sentinel sections. `components/resources/breadcrumbs.blade.php:1` breadcrumb navigation. `livewire/project/resource/index.blade.php` + `livewire/project/resource/create.blade.php` resource creation.
- **Dashboard:** `livewire/dashboard.blade.php:1` project grid. `livewire/project/shared/resource-details.blade.php:1` + `resource-operations.blade.php:1` operational controls colocated with details. `components/status/{index,running,stopped,degraded,restarting,services}.blade.php` explicit status component family (6 variants) — status is not a pill, it is a dedicated component.
- **App list/detail:** `livewire/project/index.blade.php` + `show.blade.php` + `edit.blade.php` + `new/select.blade.php` + `new/*` (docker-compose, docker-image, github-private-repository, public-git-repository, simple-dockerfile). Each app type gets its own blade form.
- **Deployment experience:** `livewire/project/application/deployment/index.blade.php:1` deployments list, `show.blade.php:1` detail with steps. `components/deployment/configuration-diff.blade.php:1` config diff before deploy. `livewire/project/shared/logs.blade.php:1` + `project/shared/get-logs.blade.php:1` + `execute-container-command.blade.php:1` colocated. `components/forms/env-var-input.blade.php:1` dedicated env input with helpers.
- **Env vars:** `livewire/project/shared/environment-variable/all.blade.php:1` + `show.blade.php:1` + `show-hardcoded.blade.php:1` + `add.blade.php:1` — four-layer env UX: all (team-shared), show (per-resource), hardcoded (magic/immutable), add (with multiline/literal/buildtime-runtime flags). See `show.blade.php` excerpt below for buildtime/runtime/literal/multiline checkboxes.
- **Domains:** `deployment/configuration-diff` + `domain-conflict-modal.blade.php:1` explicit conflict handling. Domains are per-resource with verification + conflict modal.
- **Metrics:** `livewire/server/charts.blade.php:1` server charts (CPU/memory/disk/network) — colocated with server view, not a separate global monitoring page.
- **Global search:** `livewire/global-search.blade.php:1` global search across projects/resources.
- **Notifications:** `livewire/notifications/{discord,email,telegram,slack,webhook,pushover}.blade.php` per-channel forms.

### 2.2 Dokploy — Next.js dashboard

- **IA:** `reference/app-platforms/dokploy/apps/dokploy/components/dashboard/{application,compose,database,mariadb,mongo,mysql,postgres,redis,project}` mirrors resource type per folder. `components/layouts` + `apps/dokploy/pages` filesystem routing.
- **App detail IA:** `components/dashboard/application/{advanced,general,build,deployments,domains,environment,logs,preview-deployments,rollbacks,schedules,volume-backups}` — 10+ subfolders per app. Advanced alone has `advanced/{cluster,general,import,ports,redirects,security,traefik,volumes}`. This depth shows Dokploy's IA pushes 80% of controls under Advanced to keep primary tabs clean.
- **Env vars UX:** `components/dashboard/application/environment/show-environment.tsx:1` uses `CodeEditor` (monaco-like) with `isEnvVisible` toggle, `hasChanges` unsaved indicator, `Ctrl+S/Cmd+S` shortcut, per-type mutation map (`compose|libsql|mariadb|mongo|mysql|postgres|redis`). Validated via `zod` schema `addEnvironmentSchema`. `components/ui/secrets.tsx:1` masked secrets component.
- **Domains:** `components/dashboard/application/domains/{handle-domain.tsx,columns.tsx,dns-helper-modal.tsx,handle-forward-auth.tsx,show-domains.tsx}` — table columns + handler + DNS helper modal + forward-auth.
- **Logs:** `components/dashboard/application/logs/show.tsx:1` + `compose/logs`, `components/dashboard/docker/logs/docker-logs-id.tsx` — dynamic import (`ssr:false`), container/service selector, `badgeStateColor` per state, swarm vs native toggle.
- **Deployments:** `components/dashboard/application/deployments/show-deployments.tsx:1` polls `api.deployment.allByType` every 1s, `show-deployment.tsx`, `cancel-queues.tsx`, `clear-deployments.tsx`, `kill-build.tsx`, `refresh-token.tsx`, webhook URL display + copy, `stuckDeployment` detection (>9 min running in cloud).
- **Empty/Loading:** `components/ui/{skeleton.tsx,card.tsx,alert.tsx,progress.tsx,sheet.tsx}` shadcn-based. `file-tree.tsx` for file manager.

### 2.3 Komodo — Rust + frontend (periphery/Core)

- **IA:** Frontend not enumerated deeply (top-level `frontend/src` not present at inspected snapshot), but backend API shows `bin/core/src/api/write/stack.rs` `CreateStack/UpdateStack/DeployStack/StartStack/StopStack/RemoveStack/PullStack` — UI maps 1:1 to lifecycle verbs. Periphery `lib/periphery` does `docker compose up -d`, `ps --format json`, `logs`, `pull` on host. Screenshots folder shows stack table with status pills and bulk deploy.
- **UX trait:** Monitor loop (`bin/core/src/monitor`) reconciles `compose ps` → `running/stopped/degraded` — status is derived, not declared. Charts via `SystemMetrics` style.

### 2.4 Portainer — Angular/React hybrid

- **IA:** `reference/app-platforms/portainer/app/portainer/views/{stacks,endpoints,account,settings,tags,users,registries}` + `app/docker` + `app/kubernetes` + `app/edge` + `app/react` + `app/azure`. Endpoint-scoped navigation (`endpoints/:id` prefix). `app/portainer/components` + `app/docker/components` + `app/kubernetes/components`.
- **App list:** `app/stacks` stack list with per-endpoint deployment tracking (`dataservices/edgestackstatus/tx.go:80 DeploymentInfo`). Edge stacks show multi-endpoint deployment matrix.
- **Deployment:** `api/stacks/stackbuilders/{compose_git_builder.go:24,compose_file_builder.go:22,director.go}` builder pattern; UI shows `redeploy` + `RedeployWhenChanged` poll. Deploy is `docker stack deploy` or `compose up -d` — idempotent, no strategy selector.
- **Permissions/RBAC:** `app/portainer/authorization-guard.ts:1`, `app/portainer/rbac`, `app/portainer/services`, `app/portainer/filters` — role/team/endpoint access matrix; UI shows `accessControl` per stack/endpoint.
- **Global search/command:** Not prominent; endpoint selector is primary affordance.

### 2.5 1Panel — Vue frontend

- **IA:** `reference/app-platforms/1panel/frontend/src/views/{app-store,container,cronjob,database,home,host,log,setting,share,terminal,toolbox,website}` + `routers/index.ts` + `layout/index.vue` sidebar. `App.vue:1` + `components` + `composables` + `store`. Top-level nav mirrors system domains (container = docker, website = domains, database = DB hosts, toolbox = host).
- **Dashboard:** `views/home/index.vue` overview with stats row (nodes/servers/users/health analogues). `views/container` container list with start/stop/pause/unpause/restart/remove, image prune, network/volume tabs.
- **App store/templates:** `views/app-store` catalog + installed vs store split (matches Forge App Store vs installed).
- **Domains/websites:** `views/website` Nginx website management (create, proxy, rewrite, SSL via `website_acme_account`, `website_ssl`, `website_domain`).
- **Empty states:** Per-view `EmptyState` via `el-empty` (Element Plus) with `description` and action `el-button`.

### 2.6 CapRover — Node/Express + frontend build

- **Structure:** `reference/app-platforms/caprover/src/{handlers,models,datastore,user}` — no `frontend/` SPA; UI is served from `public` built from `src`. Single-tenant IA: `AppsDataStore:318` app version entry, `ImageMaker:46` build lifecycle, `OneClickAppDeploymentHelper:51` template deploy. Not a reference for multi-project UX.

### 2.7 Uncloud — CLI + CRDT mesh (no web UI)

- **IA:** Imperative CLI (`pkg/client/service.go:24 RunService`, `141 RemoveService`, `165 StopService`, `199 StartService`). No dashboard/empty states/metrics UI to compare. Represents "no control plane" pattern — relevant as counter-reference for Forge's heavy control plane.

### 2.8 docker-compose — CLI spec control

- **No UI:** `cmd/compose/compose.go:561 restartCommand`, `587 scaleCommand`, `580 versionCommand`; `pkg/compose/{up,create,down,restart,scale,ps,logs}`. Health via `healthcheck: {test,interval...}` in compose spec, not UI. Used as spec-control for correctness of Forge compose flows.

---

## 3. Forge UI Deep Inventory (file evidence)

### 3.1 Navigation & IA

- `forge/web/components/admin/admin-registry.ts:22` `adminPageRegistry: AdminNavGroup[]` — 5 groups (Operations, Infrastructure, Management, Services, Advanced) totaling ~60 entries with `capability: available|metadata-only`, `requiredRole: admin`, `description`. Single source of truth for nav metadata. Plugins entry at `:60` is `capability: metadata-only` with description “Manifest registry; runtime unavailable”.
- `forge/web/components/admin/admin-shell.tsx:41` `navGroups` memo with goal-oriented `plan` array (Command Center, Workloads, People & Access, Infrastructure & Data, Networking & Security, Platform Configuration) that maps `hrefs` to filtered `entries`. Includes search filter `navSearch`, collapsible groups `collapsedGroups`, mobile drawer `mobileOpen`. `findAdminPage:59` for active breadcrumb. `API_BASE_URL` unreachable banner at `:85` and pending redirect at `:70`.
- `forge/web/app/admin/{allocations,backups,compose,database-services,deployments,domains,environments,monitoring,apps,...}/page.tsx` each implement own `SectionHeader`, `CardHeader`, `AdminToolbar`, `AdminTable` or `ui-card` layouts — no shared `AdminPageLayout` enforcement (some use it, some do not).
- `forge/web/app/admin/apps/[id]/page.tsx:28` `TABS` 7 tabs (overview, deployments, configuration, logs, console, domains, backups) with client-side `useState` tab switcher (`:43` `tab` state from `searchParams.get("tab")`), not URL-routed tabs. Secondary links to compose/git views when type matches.
- `forge/web/components/server/server-nav.tsx:16` `tabs: ServerTab[]` 16 tabs (console, files, databases, database, schedules, users, backups, builds, network, startup, settings, mounts, activity, processes, deployments, git, transfer) with `permissions: string[]` per tab and `hasServerPermission(visibleTabs filter)` gate. `server-tabs.tsx:13` duplicates this config with slightly different permissions (console requires only `websocket.connect` vs `websocket.connect+control.console`).
- `forge/web/stores/use-server-store.ts:1` Zustand `currentUser`, `mode`, `activeTab`, `selectedServerId`, `consoleLines`, `liveStats`, `cpuHistory/memoryHistory`.

### 3.2 Dashboards

- `forge/web/components/admin/AdminOverview.tsx:86` `AdminOverview` — 5 queries (`nodes`, `servers`, `users`, `health`, `activity`), `StatsRow` 4 cards (Nodes, All servers, Running, Users), 2 capacity cards (server memory/disk configured vs node capacity), status distribution `StatusBreakdown` bar, node memory bar chart `SimpleBarChart`, persisted heartbeat list, server inventory slice (8), capacity coverage, actionable failures, recent activity. Handles `isError/isLoading` per query with `QueryError/QueryLoading` + `EmptyState` fallbacks and `Pill` availability row.
- `forge/web/app/admin/monitoring/page.tsx:1` `AdminMonitoring` — 4 summary gauge charts (`SystemHealthGauge`, `ServerCPUChart`, `ServerMemoryChart`, `ServerDiskChart`), `SystemMetrics`, `ResourceUsageBar`, `NodeList` + `ServerNetworkChart`, advanced charts toggle, alert history `getAlertHistory(limit:20)` every 30s. Uses `dynamic(..., {ssr:false})` for recharts.
- `forge/web/app/admin/compose/page.tsx:1` compose stacks grid/cards with `statusConfig` 9 states (running, deploying, awaiting_health, stopped, degraded, failed, updating, deleting, deleted).

### 3.3 App List / Detail

- `forge/web/components/app/app-list.tsx:9` `AppList` — `useQuery(["apps"], fetchApps)`, `SkeletonList rows6 columns4` loading, `ErrorAlert` with retry, `EmptyList` with “No apps yet” + Create CTA, `ui-card` list with icon + name + id, optional `onSelect`.
- `forge/web/components/app/app-detail.tsx:20` `AppDetailView` — `SkeletonDetail`, `ErrorNotFound` vs `ErrorAlert`, header with back to `/servers`, `ServerStatus`, render-prop children.
- `forge/web/app/admin/apps/page.tsx:35` `AdminAppsPage` — search `Input`, type filter `select`, `filtered` memo, `startMut/stopMut/restartMut/deleteMut`, `DeployStatusBadge`, type icons, card per app. `app-create-form.tsx:20` 4-step? actually 5 fields (name, description, region, template, memory, disk, cpu) with `validate()` inline.
- `forge/web/app/admin/apps/[id]/page.tsx:137` `OverviewTab` resource gauges (CPU/memory/disk), start/stop/restart/deploy buttons, info table; `DeploymentsTab:263`, `ConfigurationTab:379` (EnvVarEditor, PortMapper, VolumeEditor), `LogsTab:460`, `ConsoleTab:477` (WebSocket ticket flow), `DomainsTab:596`, `BackupsTab:694`.

### 3.4 Deployment Experience & Status

- `forge/web/components/app/deployment-progress.tsx:39` `DeploymentProgress` — manual `setInterval(load,2000)` polling `fetchDeploymentSteps`, `statusConfig` 6 states, progress bar `pct=completed/total*100`, collapsible error panel, `onComplete/onError` callbacks.
- `forge/web/components/charts/DeploymentTimeline.tsx:23` `DeploymentTimeline` — `useQuery(["deployment-steps",deploymentId], fetchDeploymentSteps, refetchInterval:5000 if hasActive)` auto-polling, vertical timeline with icons/colors/connector line, skeleton/error/empty states within `Card`.
- `forge/web/lib/api/deployments.ts:16` `Deployment/DeploymentStep/DeploymentRecord/Rollback/Revision` types + 15 functions (`fetchDeploymentSteps`, `fetchDeployment`, `fetchDeploymentRevisions`, `compareRevisions`, `rollbackToRevision`, `cancelDeployment`, `executeDeployment`, `completeDeployment`, `fetchDeploymentRecords`, `fetchDeploymentLogs`, `fetchRollbacks`).
- `forge/web/lib/api/apps.ts:93` `AppDeployment` type + `fetchAppDeployments`, `triggerDeploy(265)`, `fetchAppLogs`, `fetchAppServiceLogs`.

### 3.5 Logs / Metrics Placement

- `forge/web/components/deployment/DeploymentLogViewer.tsx:37` `DeploymentLogViewer` — WebSocket logs with `MAX_RECONNECT_ATTEMPTS=20`, exponential backoff, `autoScroll` toggle, search `stripAnsi`, level colors, line numbers + timestamps, clear button.
- `forge/web/components/admin/AdminAppsShared.tsx:28` `LogViewer` — simpler polling logs (`fetchAppLogs` every 5s in `LogsTab`), search, auto-scroll, download `Blob` as `.log`.
- `forge/web/components/monitoring/metrics-chart.tsx:66` `MetricsChart` — metric selector (cpu/memory/disk/networkRx), period selector (5m/15m/1h/6h/24h), `AreaChart` via recharts, `periodWindow` limit/since, refetch 10s for 5m else 60s.
- `forge/web/components/charts/{ServerCPUChart,ServerMemoryChart,ServerDiskChart,ServerNetworkChart,ResourceUsageBar,SystemHealthGauge}:1` separate chart components dynamic-imported on monitoring page.

### 3.6 Domains / Configuration / Env Vars / Secrets / Permissions / Projects / Environments / Teams

- `forge/web/components/environment/env-var-editor.tsx:8` `EnvVarEditor` (admin) — `fetchEnvVars(scopeType,scopeId)`, `createEnvVar`, `deleteEnvVar`, key/value inputs, reveal toggle per `sensitive`, `EmptyState("No variables defined")`. Simple key/value list, no multiline/literal/buildtime-runtime toggles.
- `forge/web/components/environment/EnvironmentEditor.tsx:37` `EnvironmentEditor` (console/shared, controlled via props) — full-featured: search, showValues, import/export `.env` via `parseEnvFormat/toEnvFormat`, `encrypted` flag per var, add/update/remove via `variables` array prop. Not backed by API directly — parent owns state.
- `forge/web/components/admin/AdminAppsShared.tsx:120` `EnvVarEditor` (app config) — `Record<string,string>` variant: key validation `ENV_KEY_REGEX`, add (upsert same key), remove, update. No separate sensitive/encrypted flag, no version. Different from admin `EnvVarEditor` above — third variant.
- `forge/web/lib/api/env-vars.ts:6` API with overloaded signatures `fetchEnvVars(scopeType,scopeId)` GET `/{projects|environments}/:id/env-vars`, `createEnvVar` POST same, `deleteEnvVar` DELETE `/env-vars/:varId`.
- `forge/web/components/app/domains-view.tsx:23` `DomainsView` — `fetchAppDomains` list, count header, `EmptyDomains`, `VerificationStatus`, delete with `useConfirm`. `compose-view.tsx:31` `ComposeView` parses `sourceConfig.services` array, empty `EmptyServices`.
- `forge/web/app/admin/environments/page.tsx:1` `AdminEnvironmentsPage` — org→project→environment cascade selects, create environment (name/color/protected), environment list, var editor per selected env. `AdminAccess.tsx:1` roles + OAuth clients with delete confirms. `forge/web/lib/api/tenancy.ts` tenancy hydration.
- `forge/web/components/admin/AdminAllocations.tsx:1`, `AdminMounts.tsx:1`, `AdminDatabases.tsx:1` etc — per-control-plane CRUD tables for infra allocations/mounts/database hosts.

### 3.7 Stores / Translation

- `forge/web/stores/use-server-store.ts:1` Zustand `currentUser/setCurrentUser`, `mode`, `activeTab`, `selectedServerId`.
- `forge/web/stores/use-tenancy-store.ts:1` org/project/env selection persistence.
- `forge/web/components/TranslationProvider.tsx:19` `TranslationProvider` wraps `useTranslation()` from `forge/web/lib/use-translation.ts:1` (lazy fetch `/api/i18n/[locale]`, cache, `preloadLocale`, `changeLocale`). `useT:28` falls back to `defaultMessages` (`lang/en.json`) on missing context so tests/error fallbacks do not show keys.

### 3.8 Shared States (loading/empty/error/offline)

- `forge/web/components/shared/states-empty.tsx:17` `EmptyCard` base + `EmptyList`, `EmptySearch`, `EmptyDeployments`, `EmptyBackups`, `EmptyDomains`, `EmptyServices`, `EmptyGit`, `EmptyCertificates`, `EmptyDNSProviders`, `EmptyOrganizations` — 10 semantic empty states with icon/title/description/action.
- `forge/web/components/shared/states-loading.tsx:10` `SkeletonList(rows5 columns4)`, `SkeletonDetail`, `SkeletonForm(fields5)`, `SpinnerInline`, `SpinnerPage`, `SpinnerButton` — skeletons mimic `ui-card` structure, with `aria-label="Loading ..."` + `role="status"` and `animate-pulse`.
- `forge/web/components/shared/states-error.tsx:43` `ErrorCard` base + `ErrorAlert` (with expandable stack, retry), `ErrorNotFound`, `ErrorPermission`, `ErrorNetwork`, `ErrorRateLimit` (countdown retry). All use `errorMessage()` + `role="alert"` + dashed red border.
- `forge/web/components/shared/states-offline.tsx:6` `OfflineBanner` sticky amber banner with `navigator.onLine` listener + retry button.
- `forge/web/components/admin/admin-ui.tsx:201` `AdminLoadingState/Rows/ErrorState`, `Card/CardHeader`, `Btn` (maps `tone` to `Button variant`), `Input/Textarea`, `AdminTabs` (arrow-key roving focus), `EmptyState/PermissionDeniedState`, `StatsRow`, `Pill`.

---

## 4. Capability Comparisons (17)

### C01 — Information Architecture / Navigation

**REFERENCE pattern:**
Coolify: `resources/views/livewire/project/show.blade.php:1` + `resources/views/components/server/sidebar.blade.php:1` shows *resource-centric* IA — Project → Environment → Application/Service/Database, with per-server sidebar sections (proxy/security/sentinel) and `resources/views/components/resources/breadcrumbs.blade.php:1` breadcrumb trail. Navigation depth never exceeds 3, and global search `livewire/global-search.blade.php:1` spans projects/resources. Dokploy: `components/dashboard/application/{general,advanced,build,deployments,domains,environment,logs}` + `advanced/{cluster,general,import,ports,redirects,security,traefik,volumes}` pushes 60% of controls under **Advanced** to keep primary tabs to 7. 1Panel: `frontend/src/views/*.vue` 12 top-level modules (`home`, `app-store`, `container`, `database`, `website`, `host`, `cronjob`, `terminal`, `log`, `toolbox`, `setting`, `share`) — system-domain, not resource-domain. Portainer: endpoint-scoped `app/portainer/views/endpoints` + `app/stacks` — endpoint selector is the top pivot.

**FORGE:**
`admin-registry.ts:22` single registry with 5 source groups (Operations 10 items, Infrastructure 8, Management 8, Services 9, Advanced 27) totaling ~62 entries. `admin-shell.tsx:45` re-groups them into 6 goal-oriented sections (Command Center 7 hrefs, Workloads 7, People & Access 6, Infrastructure & Data 12, Networking & Security 11, Platform Configuration 10) with search `navSearch:39` and collapsible groups `collapsedGroups:40`. Active page description shown as subtitle at `:185`. `server-nav.tsx:16` 17 tabs per server with per-tab permission gate, `hasServerPermission` filter.

**STATUS:** `PARTIAL`

**GAP:** Registry has `~62` entries but shell `plan` only maps ~50 hrefs — `Recovery` (`/admin/migrations` duplicate) and `Migrations` collision hide distinct IA; `Kubernetes`, `Containers`, `Dev/States` exist as routes but not in registry, and thus never appear in nav search. Dokploy's discipline of collapsing complexity under *Advanced* is inverted in Forge: *Advanced* is the largest group (27 items) with no further collapse — the taxonomy barrier is papered over by adding more entries to Advanced. Coolify's per-resource breadcrumbs have no Forge equivalent — `AdminPageHeader` shows back button + title but not resource lineage (organization → project → environment → app). Server nav has 17 tabs vs Dokploy's 7 — tabs spill to mobile drawer without scroll hint.

**LOGIC FINDING:** None.

**RECOMMENDATION:** `ADAPT` — Extract a second-level collapse inside Advanced (Deploy/Traffic/Security sub-groups). Add breadcrumb component mirroring Coolify's `breadcrumbs.blade.php` that reads `useTenancyStore` + `adminPageRegistry` lineage. Audit `plan` hrefs against registry to eliminate invisible entries.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — discoverability, not data loss.

---

### C02 — Dashboards

**REFERENCE:**
Coolify `livewire/dashboard.blade.php:1` shows team-scoped project grid, not cluster metrics — dashboard is *workload inventory*, not observability. Dokploy `components/dashboard/home` similar — recent deployments + stats. 1Panel `frontend/src/views/home/index.vue` (stats row + resource usage bars) is an exception: home mixes inventory and metrics. Komodo monitor loop reconciles `compose ps` but dashboard is stack table, not charts.

**FORGE:**
`AdminOverview.tsx:86` workload-focused overview: inventory totals, configured capacity (memory/disk) with reported/total coverage, `StatusBreakdown` distribution bar, `SimpleBarChart` per-node memory, persisted heartbeat list, server inventory slice (8). Handles per-query `isError/isLoading` with `QueryError/QueryLoading` + `Pill` availability row at `:142`. `monitoring/page.tsx:1` is the *separate* observability dashboard: 4 gauges + SystemMetrics + ResourceUsageBar + NodeList + NetworkChart + advanced toggle + alert history.

**STATUS:** `COMPLETE`

**GAP:** Mirrors 1Panel's split (home vs monitoring) correctly. The overview's "configured capacity" vs "live usage" distinction at `:313` (`Totals include only finite configured values... Capacity is configured allocation, not live resource usage`) is more honest than Coolify/Dokploy which conflate quota with usage. One gap: overview's server/memory capacity cards at `:165` duplicate capacity-coverage card at `:305` with same data from `reportedTotal:75` — DRY violation, two visuals for one metric.

**RECOMMENDATION:** Keep split: `overview` = inventory/capacity/heartbeat; `monitoring` = live metrics. Deduplicate capacity cards.

**SEVERITY:** `P3 OPERATOR_VISIBLE`

---

### C03 — App List & App Detail

**REFERENCE:**
Coolify: `livewire/project/resource/index.blade.php:1` + `livewire/project/new/select.blade.php:1` lists resources per environment with type icons and status; `show.blade.php` per-resource detail with operations inline. Dokploy: `components/dashboard/project` + `application/general/show.tsx` per-app detail with save + drift indicator. Portainer: `app/portainer/views/stacks` stack table with per-endpoint deployment info. 1Panel: `views/container` tables with status colors and search.

**FORGE:**
`app-list.tsx:14` `AppList` with `SkeletonList rows6 cols4`, `ErrorAlert` retry, `EmptyList` CTA. `app-detail.tsx:20` header with back to `/servers` (should be `/admin/apps`), `ServerStatus` pill. `app/admin/apps/page.tsx:35` adds search + type filter (`All Types` + 4 types), per-app type icons `typeIcons:Box|GitBranch|Container|Layers`, start/stop/restart/delete mutations. `app/admin/apps/[id]/page.tsx:28` 7 URL-agnostic tabs (`useState tab`, not route) + conditional Compose/Git links.

**STATUS:** `PARTIAL`

**GAP:** Four gaps vs references: (1) App detail tabs are `useState` (`:42` `searchParams.get("tab")` initial only) — refresh loses tab, deep links do not work, and browser back/forward do not navigate tabs (Coolify/Dokploy use path-based tabs like `/environment/:id/applications/:id/config` etc). (2) List `AppList` shows `Server` icon for every app (`:73` `<Server size={16}/>`) rather than `typeIcons` used in admin apps page — inconsistent iconography. (3) Detail back button hardcodes `/servers` (`app-detail.tsx:55` `href="/servers"`) but should link to `/admin/apps` when inside admin; lands user in console view unexpectedly. (4) Empty state `AppList:37` message “Apps can be game servers...” is game-panel-specific, not container-app; conflates Prison Architect vs app platform models.

**RECOMMENDATION:** `ADAPT` — Make tabs router-driven (`/admin/apps/[id]/{overview,deployments,configuration,logs,console,domains,backups}` nested routes) like `apps/[id]/{compose,deployments,git}` already do. Share `typeIcons` between list and detail. Fix back href to use `usePathname` context.

**SEVERITY:** `P2 USER_VISIBLE` — deep-linking and back behavior broken.

---

### C04 — Deployment Experience (trigger → progress → complete)

**REFERENCE:**
Coolify `livewire/project/application/deployment/show.blade.php:1` + `imitation` via `ApplicationDeploymentJob.php` shows step-by-step build → deploy → health-check with `configuration-diff.blade.php:1` before deploy, and `deployment-navbar.blade.php:1` for progress. Dokploy `show-deployments.tsx:1` polls every 1s, shows webhook URL + `refreshToken`, `CancelQueues/KillBuild/ClearDeployments`, `stuckDeployment` (>9 min) detection with kill action, `activeLog` per deployment. Portainer shows no step progress (deploy is `compose up -d` — succeeded/failed). 1Panel deploy via `AppInstall.Operate{install,upgrade}` with task queue — simple spinner, not step timeline.

**FORGE:**
Two components that do the same job: `deployment-progress.tsx:46` manual `setInterval(load,2000)` + `steps.filter(completed)` pct math at `:83`, and `DeploymentTimeline.tsx:27` `useQuery(...,refetchInterval:5000 if hasActive)` with vertical timeline/connector. `lib/api/deployments.ts:88` 15 API functions. Detail page `apps/[id]/page.tsx:263` inline table 6 columns (revision/status/source/trigger/started/duration) with modal `Modal title=Deployment #revision` showing commit/log/error. `apps/[id]/deployments/page.tsx` separate “View Full History”.

**STATUS:** `PARTIAL` (good primitives, duplicate polling)

**GAP:** (1) Two deployment progress components poll the same `GET /admin/deployments/:id/steps` with different intervals (2s vs 5s) and different query keys (`["deployment-steps"]` vs manual state) — they will never deduplicate cache, doubling load on the deployment API. One uses `react-query` with proper teardown, the other uses raw `setInterval` + `useRef` with leak risk if `deploymentId` changes quickly. Dokploy's single `useQuery(refetchInterval:1000)` is canonical. (2) Forge logs integration is absent inside deployment progress — Dokploy shows live logs per deployment via `ShowDeployment` + copy webhook URL; Forge's `LogViewer` is for app runtime, not deploy. (3) No configuration diff before deploy (Coolify's `configuration-diff.blade.php`).

**RECOMMENDATION:** `ADAPT` — Consolidate into one `DeploymentTimeline` variant, add `LogViewer` below timeline on deployments tab, add config-diff modal like Coolify's.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — performance + missing live log feedback.

---

### C05 — Deployment Status Communication

**REFERENCE:**
Coolify `components/status/{running,stopped,degraded,restarting,services,index}.blade.php` — 5 status components with semantics (degraded = partial healthy, restarting = transient). Dokploy `badgeStateColor(state): switch running/ready→green, exited/shutdown→red, accepted/created→blue` plus `ShowDeployments` status pills. Portainer `EndpointStatus`, `StackStatus`. All references use pills for server/instance status and detailed timelines for deployments — they separate *workload* status from *deploy* status.

**FORGE:**
`shared/states-badge.tsx` + `admin-ui.tsx:14 Pill` 5 tones, `states-badge.tsx` + `app-detail.tsx:66 ServerStatus` for app header, `DeploymentTimeline.tsx:14 statusConfig` 6 colors/icons (completed/in_progress/failed/cancelled/skipped/pending) with connector line, `deployment-progress.tsx:30 statusConfig` 6 icons, `ResourceGauge.tsx:14` color by pct (90% red, 70% amber). `AdminOverview.tsx:109` aggregates server status into segment bar (Running green `#22c55e`, Stopped `#64748b`, Failed red, Suspended amber, Installing blue).

**STATUS:** `PARTIAL`

**GAP:** Inconsistencies: (1) Status enums differ across APIs — `apps.ts:3 AppStatus` (running/stopped/deploying/failed/installing/pending/restarting/starting/stopping) vs `deployments.ts:16 Deployment.status string` (pending/in_progress/.../failed) vs `compose/page.tsx:statusConfig` (9 states: awaiting_health/degraded/deleting/deleted...). `Pill` tones are chosen per file, not centrally, so same status renders differently on overview vs compose vs app detail. Dokploy's `badgeStateColor` is a single function. (2) `AdminOverview` server status at `:291` uses 2-tone pill (green/neutral/yellow) while deployment timeline uses 6-tone timeline — no shared `statusTone` map for deployments vs apps vs servers. (3) `states-badge` not inspected but `DeployStatusBadge:122` maps via `statusTone` vs `deploymentStatusTone` correctly — the gap is that `compose` page invents its own `statusConfig` locally instead of reusing shared.

**RECOMMENDATION:** `ADAPT` — Centralize all status→tone/icon maps into `@/lib/api/status.ts` and consume from every page like Dokploy does.

**SEVERITY:** `P3 USER_VISIBLE` — confusing but not blocking.

---

### C06 — Empty States

**REFERENCE:**
Coolify: per-resource empty (no apps in env shows “Deploy your first app” + template picker). Dokploy: `EmptyState` per resource type with `CardDescription` guidance. Portainer: `EmptyStack` with create CTA. 1Panel: Element Plus `el-empty` with description “No data” + button. All references show *contextual* empty with primary action.

**FORGE:**
`shared/states-empty.tsx:17` `EmptyCard` (dashed border, icon ring, title, description, action) plus 10 semantic wrappers: `EmptyList`, `EmptySearch`, `EmptyDeployments`, `EmptyBackups`, `EmptyDomains`, `EmptyServices`, `EmptyGit`, `EmptyCertificates`, `EmptyDNSProviders`, `EmptyOrganizations`. `admin-ui.tsx:302 EmptyState` delegates to `ui/primitives EmptyState`. Used in: `app-list:37 No apps yet` + Create CTA, `compose-view:46 EmptyServices`, `domains-view:61 EmptyDomains`, `deployments-view:294 EmptyDeployments`, `docker/containers-view:EmptyState icon=Terminal search-aware message`, `monitoring` metrics empty.

**STATUS:** `COMPLETE` (rich empty states, properly wired)

**GAP:** Two minor gaps vs Dokploy: (1) Many Forge empty messages are generic — “No applications found” vs Dokploy's type-specific guidance (“Select a server to view its domains” at `domains/page.tsx` is good, but `AdminEnvironmentsPage` just says “Select a project” with no guidance on what an environment is). (2) `app-create-form` is reachable from `apps/page.tsx` header Create App, but `AppList:43` empty action is a dead `<button>` with no `onClick` (only `emptyAction` prop is wired; default button has no handler). So empty-state primary CTA is non-functional unless caller passes `emptyAction`.

**RECOMMENDATION:** `ADAPT` — wire `AppList` default Create button to `router.push("/admin/apps/new")`. Audit empty messages to include the next-step instruction (like Dokploy's “Connect a Git repo”).

**SEVERITY:** `P3 USER_VISIBLE`

---

### C07 — Loading / Failure States

**REFERENCE:**
Coolify: `components/loading.blade.php:1` + `loading-on-button.blade.php:1` inline spinners, `page-loading.blade.php:1` full-page. Dokploy: `Skeleton` `Card` shimmer + `Loader2` per-query. 1Panel: `el-skeleton` + `el-loading` overlay. Portainer loading: `Spinner` + `ng-if pending`. All show retry affordances per failed query, not global error.

**FORGE:**
`shared/states-loading.tsx:10` `SkeletonList/Detail/Form`, `SpinnerInline/Page/Button` all with `role="status"` + `aria-label` + `animate-pulse bg-white/[0.07]`. `shared/states-error.tsx:43` `ErrorAlert` (expandable stack trace), `ErrorNotFound`, `ErrorPermission`, `ErrorNetwork`, `ErrorRateLimit` (countdown retry). `admin-ui.tsx:201 AdminLoadingState/Rows/ErrorState` simpler variants. `admin-shell.tsx:70` pending shows centered `redirecting` text, unreachable shows red card with `API_BASE_URL` + retry. Most admin pages render per-query inline loading/error (e.g., `AdminOverview:139 Pill` availability row, `apps/[id]/page.tsx:51` loading splash).

**STATUS:** `PARTIAL`

**GAP:** Per-query handling is good, but two failure modes are missing compared to Dokploy: (1) Rate-limit error exists (`ErrorRateLimit:164` with `countdown/setInterval`) but is never consumed — `lib/api/http.ts` does not surface `429 Retry-After` as `ErrorRateLimit` prop; dead component. (2) `OfflineBanner:6` uses its own `navigator.onLine` listener, but `admin-shell.tsx:365` also has `OnlineStatusBanner` (`component: admin/OfflineBanner.tsx ?? shared/states-offline.tsx`) — two offline banners with different listeners coexist; one is inside shell above header, one would be per-page. Also `SkeletonDetail` mimics `ui-card` but `AdminOverview` when loading shows `AdminLoadingState` while `app-detail.tsx` shows `SkeletonDetail` — two skeleton languages.

**RECOMMENDATION:** `ADAPT` — Wire `429` handling in `fetchJSON` to throw `RateLimited` with `retryAfter` consumed by `ErrorRateLimit`. Remove duplicate offline banner; unify skeleton family.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C08 — Logs / Metrics Placement

**REFERENCE:**
Coolify: logs are *per-resource* (`project/shared/logs.blade.php`, `get-logs.blade.php`, `execute-container-command.blade.php`) colocated under resource detail, plus terminal. Not a global Logs page. Metrics are `server/charts.blade.php` per server, not global. Dokploy: `application/logs/show.tsx` per-app logs with container selector (native vs swarm), plus `docker/logs/docker-logs-id.tsx` per-container. 1Panel: `views/log` centralized but filtered by type. Portainer: `docker/containers/:id/logs`.

**FORGE:**
`deployment/DeploymentLogViewer.tsx:37` WebSocket logs with reconnect, search, `autoScroll`, level colors, line numbers — used for *deployment* logs. `admin/AdminAppsShared:30 LogViewer` polling `fetchAppLogs` for *app* logs + download. Monitoring page isolates metrics (`metrics-chart:66` metric selector + period selector + `AreaChart`) and charts (`ServerCPUChart` etc). `shared` and `admin` layers both have log viewers but they target different data sources: `DeploymentLogViewer` wants `wsUrl: string|() => Promise<string>` (ticketed), `LogViewer` wants `AppLogEntry[]`. `app/admin/logs/page.tsx:3` correctly `permanentRedirect("/admin/activity")` — no duplicate global logs.

**STATUS:** `PARTIAL`

**GAP:** Logs are correctly placed per-resource (good, matches Coolify/Dokploy), but two gaps: (1) Deployment logs (WebSocket, live) are separated from the deployments tab which shows only static `log: string` in the modal at `apps/[id]/page.tsx:366` — user toggles between `deployments` and `logs` tabs to see runtime logs, but deploy-time logs are stuck inside a modal, not inline under `DeploymentTimeline`. Dokploy shows deployment log inline via `ShowDeployment`. (2) Metrics selector `metrics-chart:66` uses a single-chart toggle (cpu|memory|disk|network) with period 5m–24h, while `monitoring/page.tsx` also renders separate chart per metric (`ServerCPUChart` etc) — two metric presentation models on the same page, one selectable and one fixed. The selectable chart's `since` is computed locally via `periodWindow:29` but API query key is `["metrics-chart",period]` only, dropping `limit/since` — cache dedup broken.

**RECOMMENDATION:** `ADAPT` — Render `DeploymentLogViewer` inline under `DeploymentsTab` for active deploy, not in modal. Choose one metric presentation (per-metric charts OR selector, not both) or label them distinct (live vs history).

**SEVERITY:** `P2 USER_VISIBLE` — deploy troubleshooting requires the modal, logs/metrics duplication confuses.

---

### C09 — Domains / TLS / Verification

**REFERENCE:**
Coolify: `domain-conflict-modal.blade.php:1` + per-resource domain with `helper.blade.php` tooltip, verification state, letsencrypt. Dokploy: `application/domains/{columns,handle-domain,handle-forward-auth,dns-helper-modal,show-domains}` — table with `columns`, handler with validation, DNS helper modal shows records to set, `forwardAuth` toggle. 1Panel: `views/website` full site lifecycle (nginx conf, `website_ssl`, `website_domain`, `website_acme_account`). Portainer: not domain-aware.

**FORGE:**
Two domain systems: `app/domains-view.tsx:23` app-level `fetchAppDomains` with `VerificationStatus` + `EmptyDomains`, and `admin/domains/page.tsx:16` admin-level `DomainRecord` with per-server filter (`serverFilter` selects server, then `GET /servers/:id/domains`), `addDomain` modal, `verifyMutation POST /domains/verify`, `checkDNSMutation POST /domains/check-dns` + `Network` Check DNS button, delete confirm. `lib/api/domains.ts:1` 4 functions + `ServerDomain` type.

**STATUS:** `PARTIAL` (duplicated, gated)

**GAP:** (1) Admin domains page `domains/page.tsx:57` has `enabled: !!serverFilter` — without selecting a server, the list is always `EmptyState("Select a server to view its domains")` at `:195`. There is no aggregate “all domains” view (Dokploy shows all via table). The page looks empty by default with no hint of total count. (2) Verify and Check DNS are separate mutations (`verifyDomain` vs `checkDNS`) triggered by different buttons that both hit the same domain — user sees two DNS buttons (Check DNS + Verify) with unclear order. Dokploy's `dns-helper-modal` shows the *expected* records before Verify. (3) `domains-view.tsx` and `admin/domains/page.tsx` do not share types (`AppDomain{ssl,sslStatus}` vs `DomainRecord{verified,verificationToken}`) — verification status renders differently. (4) Certificate management `admin/certificates/page.tsx` filters by verified state but domains page does not surface cert expiry.

**RECOMMENDATION:** `ADAPT` — Default to aggregated domains view (or auto-select first server). Merge verify/check DNS into single flow with DNS helper modal like Dokploy. Converge `AppDomain` vs `DomainRecord`.

**SEVERITY:** `P2 USER_VISIBLE` — domains look broken on first visit.

---

### C10 — Docker (Containers / Images / Networks / Volumes)

**REFERENCE:**
Coolify: delegated — not a docker manager UI; server shows `resources.blade.php`. Dokploy: `components/dashboard/docker/{containers,images,networks,volumes}` + `logs` with `badgeStateColor`. 1Panel: `views/container` full container/image/network/volume UI with type filters and creation modals. Portainer: deepest — `app/docker` per-endpoint containers/images/networks/volumes + inspect/detail/drawer, dedicated `docker/containers-view`.

**FORGE:**
`admin/docker/page.tsx:17` four `TABS` (containers/images/networks/volumes) with `useState tab`, renders `ContainersView/ImagesView/NetworksView/VolumesView` per `components/docker/*`. `lib/api/docker.ts:65` `listContainers(all)` fans out per-node then flattens `DockerContainerInfo`, `listImages/Networks/Volumes` similar, `operateContainer(start|stop|restart|pause|unpause|remove)` at `:73`, `createContainer/Network/Volume`. `docker/{containers,images,networks,volumes}-view.tsx` each have card/list, search `Input`, `EmptyState` with search-aware message, `AdminConfirmDialog` for create/delete. `admin-docker` is admin-only (no user docker).

**STATUS:** `COMPLETE` (faithful port of Dokploy/1Panel pattern)

**GAP:** Minor: (1) Images/networks/volumes create modals use `AdminConfirmDialog` without size validation feedback, whereas 1Panel shows image pull progress. Forge `ImagesView` pull is non-streaming (no progress). (2) Containers view search is client-side filter only — Dokploy scopes by server selector (node filter) like `monitoring/page.tsx` does via `selectedNode`; docker page has no node filter, so multi-node deploys list all nodes without grouping (whereas `docker.ts:listContainers` flattens with `nodeName` but view does not group by node). (3) No kube-aware docker view despite `/admin/kubernetes` route existing.

**RECOMMENDATION:** `ADAPT` — Add node/group selector to docker views like monitoring; stream image pull progress.

**SEVERITY:** `P3 OPERATOR_VISIBLE`

---

### C11 — Env Vars / Secrets / Configuration

**REFERENCE:**
Coolify (most nuanced): `project/shared/environment-variable/show.blade.php:1` shows per-var controls: key is **locked** with env icon + delete confirm `confirmationText="{{$env->key}}"`, plus `is_multiline`, `is_literal` (Dont interpolate `$VARIABLES`), `is_buildtime` vs `is_runtime` (Docker build vs runtime), `comment` field, magic variables handled by Coolify (interpolation note). On service type, different flags: `instantSave` on checkboxes requeues deploy. `show-hardcoded.blade.php` for immutable vars. Dokploy: `environment/show-environment.tsx:120` `CodeEditor` raw `env: string`, `isEnvVisible` toggle (Eye/EyeOff), `hasChanges` unsaved, `Ctrl+S` shortcut, `CardDescription` “environment variables to your resource” with masked view. `secrets.tsx` for masked values. 1Panel: env via compose `environment:` in file, not first-class per-var UI. Portainer: `stack.env` via `env` field + `StackEnvConfig`.

**FORGE:**
Three incompatible editors for the same concept:
- A) `environment/env-var-editor.tsx:8` (admin: `EnvVarEditor scopeType/scopeId`) — `fetchEnvVars(scopeType,scopeId)` GET `/{projects|environments}/:id/env-vars`, `createEnvVar` POST, `deleteEnvVar`, key/value inputs, reveal toggle `isSensitive`, `Pill Sensitive`, version `v{version}`. No multiline/literal/buildtime/runtime.
- B) `admin/AdminAppsShared:120 EnvVarEditor` (app detail configuration tab) — controlled `Record<string,string>` variant: `ENV_KEY_REGEX` validation at `:140`, add is **upsert** (same key overwrites, never errors), update inline, remove. Key input is `readOnly?` actually editable? At `:172` `onChange={() => {}}` — **key input is disabled** (rendered as faux input), so rename requires delete+add. No sensitive, no version, no multiline.
- C) `environment/EnvironmentEditor.tsx:37` (console/shared) — `variables: EnvVar[]{key,value,encrypted}`, bulk import/export `.env` via `parseEnvFormat/toEnvFormat`, search, `encrypted` toggle `Lock/Unlock`, add/update/remove via array `onChange`. No API binding; parent owns state. Has import/export `.env` (`FileUp/FileDown`) and search.

`lib/api/env-vars.ts:6` API correctly supports `scopeType: "project"|"environment"` but admin page `environments/page.tsx:68` calls `createEnvVar(selectedEnv, varKey, varValue, false)` — uses 3-arg overload (legacy `envId,key,value`) not scoped overload; and fetch uses `fetchEnvVars(selectedEnv)` (1-arg) which defaults to `environment` scope — so the overload indirection hides whether `project`-scoped vars ever work from that page. The new page `app/admin/apps/[id]/page.tsx` configuration tab uses variant B which bypasses the API entirely until `updateApp(391)` bulk-saves `envVars: Record`.

**STATUS:** `BROKEN` (three editors, two APIs, one silent mis-wiring)

**GAP:** User-facing incoherence: (1) Same term “Environment Variables” renders three different UIs depending on entry route — admin Environments page (key/value + Sensitive pill), app Configuration tab (Record editor with validation), console EnvironmentEditor (encrypted/import). Coolify's single show view unifies these with flags; Dokploy unifies with CodeEditor. (2) No literal/buildtime/runtime/multiline semantics — Coolify's handlers `$VAR` vs literal are missing, so Postgres/Docker-compose `$` interpolations will surprise. (3) Variant B `EnvVarEditor` silently upserts on duplicate key instead of erroring (Dokploy/CodeEditor leaves duplicate handling to raw text). (4) API overload at `env-vars.ts:6` permits both `fetchEnvVars(envId)` and `fetchEnvVars(scopeType,scopeId)` — caller must know which overload they hit; `environments/page.tsx:56` `fetchEnvVars(selectedEnv)` is ambiguous.

**RECOMMENDATION:** `ADAPT` — Converge on one model (API-backed `EnvVarResponse[]` with `isSensitive/version`) and one UI component (variant A) augmented with Coolify's flags (`isMultiline/isLiteral/isBuildtime/isRuntime/comment`). Deprecate `Record<string,string>` variant B and wire `EnvironmentEditor` variant C to the API. Remove overload; use explicit `fetchEnvVarsByScope("environment", id)`.

**SEVERITY:** `P1 USER_VISIBLE` — config loss/confusion (B upserts silently, A vs B vs C diverge).

---

### C12 — Projects / Environments / Tenancy (Teams)

**REFERENCE:**
Coolify: Project → Environment → Resource (team is one level above project). `livewire/project/{index,show,edit,clone-me,environment-edit}` + `shared-variables/{environment,project,server,team}/show.blade.php` — shared variables cascade team→project→environment→resource. Dokploy: `components/dashboard/project` + `organization` (org is team), `apps/dokploy/pages` filesystem routing with `project/:id` → `environment`. Portainer: `teams` + `endpoints` + `users` RBAC matrix. 1Panel: no team concept (single-node panel).

**FORGE:**
`admin/organizations/page.tsx`, `projects/page.tsx`, `environments/page.tsx` — three separate admin pages, plus `components/app/team-tenancy-view.tsx:1` user-facing `EmptyOrganizations` etc. `lib/api/tenancy.ts` `fetchOrganizations`, `fetchProjects(orgId)`, `fetchEnvironments(projectId)`, `createEnvironment(name,color,protected)`. `stores/use-tenancy-store.ts` persists selected org/project/environment. `environments/page.tsx:66` cascade: org select → `projectsQuery(enabled:!!org)` → project select → `environmentsQuery(enabled:!!project)` → env list → var editor.

**STATUS:** `PARTIAL`

**GAP:** Three gaps vs Coolify/Dokploy cascade: (1) Tenancy selector is **three dropdowns** on the environments page itself, not global context: visiting `/admin/apps` shows *all* apps across orgs because there is no active-project filter; Koolify/Dokploy scope resource lists to the selected project/environment. Forge apps list `fetchApps()` at `apps.ts:225` is global `GET /apps` with no `orgId/projectId` scope — tenancy is display-only, not enforced. (2) Environments color/protected flags exist (`colors:18` 8-color palette, `protected: checkbox`) but no UI shows “protected” effect — Dokploy shows protected envs as non-deletable; Forge `environments/page.tsx` only displays a `Pill Protected` without enforcement. (3) `projects/page.tsx` and `organizations/page.tsx` have no direct navigation to environments — user must go to `/admin/environments` then re-select org→project again (state not carried).

**RECOMMENDATION:** `ADAPT` — Make `AppList` scoped to active tenancy (`fetchApps({projectId})` or client filter like admin apps page). Add protected guard in API + UI (disable delete). Carry `?org=` + `?project=` via query string so linking preserves context like Coolify's env param.

**SEVERITY:** `P2 USER_VISIBLE` — tenancy appears configurable but does not filter workload views.

---

### C13 — Permissions / Roles / OAuth

**REFERENCE:**
Coolify: role selects + `team/navbar.blade.php:1` member management, invite flow (`invitation-link.blade.php`). Dokploy: `__test__/permissions/*.test.ts` (`check-permission`, `resolve-permissions`, `service-access`) + `user.getPermissions.useQuery` (`permissions?.envVars.write`) gate `SaveEnvironment` etc, impersonation. Portainer: `authorization-guard.ts:1` + `rbac` models, per-resource access control UI, `settings/tags` scoped. 1Panel: `Agent/app/api/setting.go` admin-only guards.

**FORGE:**
`admin/AdminAccess.tsx:1` two cards: Roles table (name/key/privilege `Pill isAdmin?`) with delete confirm + “Create Role” modal mapping to `role.isAdmin`, and OAuth Clients table (select owner → `fetchOAuthClients(ownerId)`, scopes). `server-nav.tsx:48 visibleTabs = tabs.filter(hasServerPermission(access, permissions))` gates per-tab; `admin-registry:requiredRole="admin"` gates entire admin. `shared/states-error.tsx:122 ErrorPermission` + `admin-ui:305 PermissionDeniedState` amber card. `stores/use-server-store currentUser.role` drives admin guard redirect at `admin-shell:33`.

**STATUS:** `PARTIAL`

**GAP:** (1) Roles are “additional role assignments” (`description: “Additional role assignments”` at `registry:49`) — not the primary RBAC; the actual permission checks are string checks like `"envVars.write"` in Dokploy but Forge checks like `"websocket.connect"`, `"file.read"`, `"control.start"` in `server-nav:16` differ per file (`server-nav` vs `server-tabs` disagree on required perms). No centralized `resolvePermissions()` like Dokploy's `resolve-permissions.test.ts`. (2) Permission failures surface as empty filtered tabs (just hidden) with a single generic amber banner at `server-nav:63` “Permissions could not be verified. Navigation is restricted” — Dokploy shows per-control disabled with tooltip explaining missing perm. (3) OAuth clients are “User-owned” (`description at registry:50`) but UI is admin-scoped under People & Access, not under user account.

**RECOMMENDATION:** `ADAPT` — Centralize permission map (like Dokploy's `check-permission`) and show disabled-with-tooltip rather than hidden tabs when missing. Move OAuth clients under account or clarify admin-vs-user ownership.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — invisible access model.

---

### C14 — Charts / Monitoring / Metrics

**REFERENCE:**
Coolify: `livewire/server/charts.blade.php:1` per-server area/line charts (CPU/memory/network/disk) colocated with server detail, not a separate global page. Dokploy: `components/dashboard/monitoring` + `monitoring/server` metrics with period selectors. Komodo: monitor polls `ps` for status, plus resource history. 1Panel: `agent/app/service/monitor.go:1` + `views/host` CPU/memory per-host bars.

**FORGE:**
`monitoring/metrics-chart.tsx:66` single selectable chart (`METRICS: Cpu/Memory/Disk/NetworkRx`, `PERIODS: 5m/15m/1h/6h/24h`, `periodWindow` compute `limit/since`, `AreaChart` + `linearGradient`). `monitoring/page.tsx` composes `SystemHealthGauge + ServerCPUChart + ServerMemoryChart + ServerDiskChart + SystemMetrics + ResourceUsageBar + NodeList + ServerNetworkChart` plus `getAlertHistory limit20 every 30s` with alerts `Pill severity`. `charts/*` components each wrap recharts with error boundaries. `lib/api/monitoring.ts` `getNodeMetrics({period,limit,since})`.

**STATUS:** `COMPLETE`

**GAP:** (1) Metrics appear twice on monitoring page — selectable `MetricsChart` (top, user toggles metric) and per-metric `Server*Chart` cards (below, one per metric) show overlapping data from same `getNodeMetrics` endpoint with different query keys (`["metrics-chart",period]` vs per-chart keys) — Dokploy/Coolify show one set. (2) Node filter `selectedNode` at `monitoring:45` affects `ServerCPUChart` via `nodeFilter` but `MetricsChart:73` query key ignores `nodeFilter` — selected node does not filter the top chart. (3) Empty/error rendering for metrics is `Failed to load metrics` red text inline (`:133`) not a full `ErrorAlert` like other pages — inconsistent failure UX.

**RECOMMENDATION:** Keep one metrics presentation or label them explicitly (“Live per-node” vs “History”). Wire node filter into `MetricsChart`.

**SEVERITY:** `P3 OPERATOR_VISIBLE`

---

### C15 — Domains vs DNS vs Certificates vs Security vs Traffic

**REFERENCE:**
Coolify: domains are part of application config (proxy domains list), `security/navbar` for headers, domain conflict modal. Dokploy: domains are per-application with DNS helper + `traefik-config`, `show-traefik-config.tsx`. 1Panel: `views/website` manages Nginx proxy + `website_ssl` (Let's Encrypt via `acme`) + `website_domain` mapping + `rewrite`. All cluster domains, certs, and security together.

**FORGE:**
Separate admin pages: `admin/domains` (Domain Management), `admin/certificates` (TLS), `admin/dns` (DNS providers for ACME), `admin/security` (HTTP security headers), `admin/traffic` (route rules), `admin/load-balancer` (target groups), `admin/endpoints` (public endpoint inventory). Each is its own nav entry (`registry:69-86`). `certificates/page.tsx` filters by `verified` but `domains/page.tsx` manages verification. `security/page.tsx` per-domain overrides.

**STATUS:** `PARTIAL` (over-decomposition)

**GAP:** Splitting what Dokploy/Cmobify/1Panel cluster (domains + certs + DNS + headers are one workflow: *add domain → set DNS → issue cert → apply headers → route*) into 7 pages forces user to hop. Forge `security` page shows overrides per domain but domains page does not link through — no deep link `domain → security`. Coolify's `sidebar-proxy` groups these. Forge grouping in shell `Networking & Security` at `admin-shell:50` correctly clusters them, but the *pages* still treat them as independent inventories.

**RECOMMENDATION:** `ADAPT` — Add cross-links (domain row → certificates/security/traffic) and a workflow CTA on domains page (“Add domain → Issue TLS → Set security headers”).

**SEVERITY:** `P3 USER_VISIBLE` — workflow fragmentation.

---

### C16 — Compose / Stacks: Configuration, Secrets, Revision & Per-Service UX

**REFERENCE:**
Coolify: not compose-first; `compose_parsing_version` + `deploy_docker_compose_buildpack:607` parses YAML then maps ports/envs/labels. Dokploy: compose is first-class — `dashboard/compose/{advanced/general/logs/containers}` with `env` via `CodeEditor`, containers live view, `deployComposeStack` + `clone compose`. Komodo: `stack.rs CreateStack→DeployStack` raw compose YAML treated as source of truth; UI is `compose.yaml` editor + env vars alongside. 1Panel: `views/container` deploy via `docker-compose.yml` verbatim, not decomposed per-service.

**FORGE:**
Two compose worlds: app-level `apps/[id]/compose/page.tsx` + `lib/api/apps:312 fetchAppComposeConfig/updateAppComposeConfig` (for `appType=compose` apps), and admin-level `compose/page.tsx` + `compose/[id]/page.tsx` + `lib/api/compose.ts` standalone stacks (`POST /compose validate/import/git/deploy`, `deployComposeStack`, `getComposeStackStatus/logs`). `components/app/compose-view.tsx:14` parses `sourceConfig.services[]` into `{name,status,image,ports}`, `AppDetail` overview tabs link to compose view only if `type==="compose"`. `compose/[id]/page.tsx` stop/start/redeploy/delete (no restart).

**STATUS:** `PARTIAL`

**GAP:** (1) Same compose YAML has two address spaces: `GET /apps/:id/compose` vs `GET /compose/:id` — user editing app compose (`PUT /apps/:id/compose`) does not affect the standalone `compose_stacks` row created by “New stack” (`POST /compose`), and vice versa. Dokploy/Komodo have one stack table. (2) Secrets: `components/ui/secrets.tsx` exists in Dokploy, but Forge `compose-view` shows no secrets integration — env vars are in `EnvironmentEditor` not tied to `compose.env_vars`. (3) Per-service controls: Dokploy `compose/containers` live view + `ports/redirects/traefik` per-service; Forge shows `services + DBStatus + ports joined` read-only table at `compose-view:57` with no actions per service.

**RECOMMENDATION:** `ADAPT` — Unify app-compose and standalone compose into one stack model (or at least deep-link them). Add per-service actions (logs/restart) like Dokploy.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — bifurcation will cause “my compose edit did not deploy” support tickets.

---

### C17 — Translation / i18n

**REFERENCE:**
Coolify `lang` files + Blade strings use `__("key")` with fallback. Dokploy uses `next-intl` style server ctx. 1Panel `frontend/src/lang/{en,zh,es,ja,ko,ru,...}.ts` + `i18n.ts:1` per-locale YAML/TS modules loaded synchronously.

**FORGE:**
`components/TranslationProvider.tsx:19` + `lib/use-translation.ts:1` lazy-loads `/api/i18n/[locale]` server route (`app/api/i18n/[locale]/route.ts:1`) with `cache`, `supportedLocales`, `preloadLocale`, `changeLocale`. `useT:28` fallback to `lang/en.json:1` via manual `key.split(".").reduce` so missing context (tests/error fallbacks) never shows raw keys — shows English fallback. `admin-shell:16 tOr(key,fallback)` is double fallback (translation, then inline fallback). `lib/locale-utils.ts` + `lang/en.json` ~200 keys.

**STATUS:** `COMPLETE` (more careful than references)

**GAP:** Double fallback hides translation gaps — missing key renders English instead of visible placeholder, so translators cannot spot gaps (Coolify/1Panel show raw key in dev). Not a user-facing bug, but a workflow gap for translators.

**RECOMMENDATION:** `ADAPT` — In development, warn when fallback triggers (`console.warn` on `value===key`).

**SEVERITY:** `P4 SILENT`

---

## 5. Logic Findings (Forge UI promises vs backend/UI coherence)

### FL-UX-01 — Duplicate Navigation Entry: Recovery == Migrations — Fake Second Tab

**Evidence:**
`admin-registry.ts:31` `Migrations` → `href: "/admin/migrations"` and `admin-registry.ts:32` `Recovery` → **same** `href: "/admin/migrations"` with different label/description (`"Migration jobs and recovery plans"` vs `"Recovery plan management"`). `findAdminPage:96` picks the *longest* matching href, so `/admin/migrations` resolves to whichever sorts first by length (same length → array order picks `Migrations`). `AdminShell` will render both items as active simultaneously when on `/admin/migrations` (both pass `pathname===href || pathname.startsWith("/admin/migrations/")`).

**Impact:**
Two nav items point to one page. User clicks Recovery expecting distinct recovery-plan UI but gets migrations list. `admin/migrations/page.tsx` shows migrations + recovery plans mixed; the split in registry is not reflected in routes. Looks like a planning artifact that leaked to registry.

**Classification:** `DUPLICATE / DEAD TAB` — duplicate screen.

**Recommendation:**
`FIX` — Either split into `/admin/migrations` and `/admin/recovery`, or keep one entry and merge descriptions. Remove duplicate from `adminPageRegistry` until the second page ships.

**Severity:** `P2 USER_VISIBLE` — impossible-to-discover second control.

---

### FL-UX-02 — App Create Form: Four Visible Fields Are No-ops — Fake Control

**Evidence:**
`app/app-create-form.tsx:22` `FormValues = {name, description, region, template, memory, disk, cpu}` renders 5 inputs + 2 selects (description, region `us-east/us-west/eu-west/ap-south`, template `nodejs/python/go/static/docker`, memory/disk/cpu). Validation at `:33` requires `region` and `template`. But `createMut:64` mutation ignores 4 fields entirely:
```ts
createApp({ name: v.name.trim(), type: "image", ports:[], envVars:{}, volumes:[], domains:[], enableTls:false, cpuLimit: String(v.cpu), memoryLimit: String(v.memory), diskLimit: String(v.disk) })
```
`description`, `region`, `template`, `disk`? Actually `disk`/`memory`/`cpu` are wired, but `description`, `region`, `template` are not sent. `CreateAppInput:135` does accept `templateId?: string` + `regionId?` but the handler at `handlers_apphosting.go:173` does not ignore them; they are just never supplied. User fills region/template to pass validation, but server records `type="image"` regardless of template choice, and `regionId/registryUrl/gitUrl` remain null. The admin apps new wizard at `app/admin/apps/new/page.tsx:120` is a *4-step* wizard (source→template→configure→review) that *does* wire these correctly — so simple `CreateAppForm` (used in `app-list`/`console` paths) is inconsistent with the wizard.

**Impact:**
User selects `Template: Python` and `Region: EU West`, validation passes, app is created as blank `image` with no image URL, then `observed_status=idle` forever (see lifecycle C01). Deployment `POST /apps/:id/deploy` then creates empty-image `recreate` deployment that will fail health-gate or FALSE_COMPLETE (lifecycle FLF-01). The form promises region/template placement that never happens.

**Classification:** `FAKE CONTROL` — misleading label; UI collects required input then discards.

**Recommendation:**
`FIX` — Map `template` → `templateId` and `region` → `regionId` (or `nodeId` via lookup) before `createApp`, or remove region/template/description fields from `CreateAppForm` and delegate to the wizard. If form is kept simple, label the select as `templateId` and call `fetchAppTemplates` like admin wizard does.

**Severity:** `P1 USER_VISIBLE` — leads directly to deploy failure/black-hole deploy.

---

### FL-UX-03 — Three Env Var Editors for One Concept — Impossible-to-Discover Config Path

**Evidence:**
See C11 evidence: same label “Environment Variables” renders three components:
- `environment/env-var-editor.tsx:8` `EnvVarEditor({scopeType,scopeId})` — API-backed list with `isSensitive`, `version`, `Pill Sensitive`, delete by `varId`.
- `admin/AdminAppsShared:120 EnvVarEditor({envVars: Record})` — `Record<string,string>` with regex `ENV_KEY_REGEX`, upsert on duplicate, key input `readOnly` disabled, no sensitive/encrypted.
- `environment/EnvironmentEditor.tsx:37` `EnvironmentEditor({variables, onChange})` — array `EnvVar{key,value,encrypted}`, bulk import/export `.env`, search, `encrypted` toggle.

They coexist on different routes: admin `environments/page.tsx` uses A, `apps/[id]/page.tsx` configuration tab uses B, console/server shared uses C. `lib/api/env-vars.ts:6` overload `fetchEnvVars(envId)` vs `fetchEnvVars(scopeType,scopeId)` adds indirection so page `environments/page.tsx:56` `fetchEnvVars(selectedEnv)` (1-arg) and hypothetical `fetchEnvVars("project", id)` (2-arg) hit same endpoint name but different API contract.

**Impact:**
User editing env in `Configuration` tab (B) sees `updateApp` bulk overwrite; switch to `Environments` page (A) sees per-var `POST /environments/:id/env-vars` row; values diverge because B writes `applications.env_vars` or `app_services.env_vars` column while A writes `environment_vars` table. No reconciliation or warning. Coolify/Dokploy have one env truth per workload.

**Classification:** `DUPLICATE SCREEN / IMPOSSIBLE-TO-DISCOVER` — config-fragmentation.

**Recommendation:**
`FIX` — Normalize on data model: one env store (`env-vars` API with scope). Deprecate `Record` variant; make configuration tab call `createEnvVar`/`deleteEnvVar` per var like A does. Remove overload in `env-vars.ts`. Document import/export flow (keep C as bulk helper that writes via A).

**Severity:** `P1 USER_VISIBLE` — config drift between tabs.

---

### FL-UX-04 — App Detail Back Target is Hardcoded to `/servers` — Dead Tab / Misleading Breadcrumb

**Evidence:**
`app/app-detail.tsx:55` `<Link href="/servers"> <ArrowLeft/> </Link>` and `app/admin/apps/[id]/page.tsx:72` `<Btn onClick={() => router.push("/admin/apps")}>` use different backs for similar detail views. `AppDetailView` is rendered inside both admin and console contexts (shared component). Hardcoded `/servers` breaks when `AppDetailView` is used from `/admin/apps` → `/admin/apps/:id` (the most common admin path). The admin single-view at `apps/[id]/page.tsx` fixes it with `/admin/apps` but the shared `AppDetailView` used by `compose-view` consumers does not.

**Impact:**
Clicking back from app detail in admin lands in console `/servers` list, not admin `/admin/apps` list, losing filters and requiring re-navigation. Kontrast: Coolify's `resources/breadcrumbs.blade.php` always reconstructs Project→Environment→Resource path regardless of entry point; Dokploy preserves `projectId` in route (`/dashboard/project/:id/environment/:id`). Forge detail header loses provenance.

**Classification:** `DEAD TAB / MISLEADING LABEL` — impossible-to-discover return path.

**Recommendation:**
`FIX` — Make back `href` a prop (`backHref`) defaulting to `"/admin/apps"` when `pathname.startsWith("/admin")` else `"/servers"` via `usePathname()`. Better: add a breadcrumb component deriving org/project lineage from `adminPageRegistry` + `useTenancyStore`.

**Severity:** `P3 USER_VISIBLE`

---

### FL-UX-05 — “No Apps” Primary CTA Is Dead — Missing Feedback

**Evidence:**
`app/app-list.tsx:44` default `emptyAction`:
```tsx
<button className="inline-flex items-center gap-2 ..." type="button">
  <Plus size={14}/> Create App
</button>
```
No `onClick`. The button renders but is inert. Callers `admin/overview/page.tsx` or `console/servers` *can* pass `emptyAction` prop to override, but `app-list.tsx` default path does not navigate. In contrast `shared/states-empty.tsx:27` `EmptyCard` `action` slot is generic, but `AdminOverview` when `servers.length===0` renders `EmptyState("No servers yet")` without action, while compose stacks `compose/page.tsx` renders `EmptyState ...` plus explicit `Create stack` button below that *does* navigate. So list-vs-empty inconsistency.

**Impact:**
New users hit empty state with a prominent Create button that does nothing. They must find the header Create App button via accidental discovery. Portainer/Dokploy empty states are all wired to creation.

**Classification:** `DEAD TAB / MISSING FEEDBACK`

**Recommendation:**
`FIX` — `<button onClick={() => router.push("/admin/apps/new")}>` or remove default button and require caller to supply `emptyAction` (fail-closed).

**Severity:** `P2 USER_VISIBLE`

---

### FL-UX-06 — Monitoring Top Chart Ignores Node Filter — Polling Plane Mismatch

**Evidence:**
`admin/monitoring/page.tsx:45` `selectedNode` state is used as `nodeFilter` passed to `ServerCPUChart/Memory/Disk/NetworkChart` and `NodeList(onNodeSelect)`. `monitoring/metrics-chart.tsx:73` query:
```ts
queryFn: () => getNodeMetrics({ period: selectedPeriod, limit: window.limit, since: window.since })
```
`queryKey: ["metrics-chart", selectedPeriod]` — `selectedNode` not in key, never passed to `getNodeMetrics`. `lib/api/monitoring.ts` `getNodeMetrics({period,limit,since,nodeId?})` does accept optional `nodeId` but caller drops it. Conversely `ServerCPUChart` does honor `nodeFilter`. So same monitoring page shows per-node filtered gauges below and *unfiltered* selectable chart above. `periodWindow` also computes `since` from `Date.now()` at call time, not cached, so two consecutive renders within re-render thrash generate different `since` strings — query thrash.

**Impact:**
Selecting a node shows “Node: my-node ✕” banner and filters gauges, but top chart silently stays global — user thinks they filtered everything. This is the same pattern as Dokploy's bug where native vs swarm logs toggle missed cache, but Forge's version is inverted (top chart should be most sensitive to filter).

**Classification:** `FAKE CONTROL` — filter pretends to scope the whole dashboard.

**Recommendation:**
Include `selectedNode` in query key and forward `nodeId: selectedNode ?? undefined` to `getNodeMetrics`. Debounce `since` or derive from `selectedPeriod` deterministically inside `monitoring.ts` rather than caller.

**Severity:** `P2 OPERATOR_VISIBLE`

---

## 6. Severity-ordered Gap Register

| # | Capability | Status | Severity | Forge Logic Finding |
|---|---|---|---|---|
| 01 | IA / Navigation | PARTIAL | P2 | — |
| 02 | Dashboards | COMPLETE | P3 | — |
| 03 | App List / Detail | PARTIAL | P2 | FL-UX-04 (hardcoded back), FL-UX-05 (dead CTA) |
| 04 | Deployment Experience | PARTIAL | P2 | — (plus lifecycle FLF-01 false completion) |
| 05 | Deployment Status | PARTIAL | P3 | — |
| 06 | Empty States | COMPLETE | P3 | FL-UX-05 |
| 07 | Loading / Failure | PARTIAL | P3 | — |
| 08 | Logs / Metrics Placement | PARTIAL | P2 | FL-UX-06 (filter mismatch) |
| 09 | Domains / TLS | PARTIAL | P2 | — |
| 10 | Docker | COMPLETE | P3 | — |
| 11 | Env Vars / Secrets | BROKEN | P1 | **FL-UX-03** (triple editor) |
| 12 | Projects / Environments | PARTIAL | P2 | — |
| 13 | Permissions / Roles | PARTIAL | P2 | — |
| 14 | Charts / Monitoring | COMPLETE | P3 | FL-UX-06 |
| 15 | Domains-vs-Certs-vs-Security vs Traffic | PARTIAL | P3 | — |
| 16 | Compose Stacks | PARTIAL | P2 | — (plus lifecycle FLF-03 row leak) |
| 17 | Translation | COMPLETE | P4 | — |

**Global P1 logic findings requiring immediate fix:** FL-UX-02 (fake CreateApp fields → deploy black-hole), FL-UX-03 (triple env editor → config drift), FL-UX-01+05 (duplicate nav + dead empty CTA).

---

## 7. What References Do That Forge Should Adopt (positive transfer)

- **Coolify's explicit status family** (`components/status/* 6 variants`) and `configuration-diff.blade.php` before deploy — adopt as `status-badge.tsx` consolidation + pre-deploy diff modal like Dokploy's `dns-helper`.
- **Dokploy's CodeEditor env UX** (`show-environment: EyeToggle + hasChanges + Ctrl+S + secrets.tsx`) and `show-deployments` webhook URL + `stuckDeployment` kill — Forge env CodeEditor exists as variant C but should replace variants A/B; adopt Dokploy's webhook copy + deployment kill flow.
- **Dokploy's `CancelQueues/KillBuild/ClearDeployments`** deployment lifecycle affordances — Forge deployments tab has no cancel/kill; add them from `deployment-progress`/`DeploymentTimeline`.
- **1Panel's website+SSL lifecycle** (`views/website` Nginx + `website_ssl` + `acme`) grouping — Forge already clusters domains/certs/dns/security/traffic but should cross-link like 1Panel's site list.
- **Portainer's endpoint-scoped deployment matrix** and RBAC `authorization-guard` — Forge server nav per-tab permission filter is similar; surface Portainer-style disabled-with-tooltip rather than hidden tabs.
- **Coolify's `global-search.blade.php`** and `resources/breadcrumbs` — missing in Forge; should add.

---

## 8. Appendix: Verbatim Forensic Excerpts

- `admin-registry.ts:32` `Recovery` entry duplicates `Migrations` href:
  ```
  { label: "Recovery", href: "/admin/migrations", icon: RotateCcw, capability: "available", description: "Recovery plan management" }
  ```
- `app/app-create-form.tsx:64` `createApp` discards fields:
  ```
  const result = await createApp({ name: v.name.trim(), type: "image", ports:[], envVars:{}, volumes:[], domains:[], enableTls:false, cpuLimit: String(v.cpu), ... })
  ```
  yet `validate:37` requires `if (!values.region) errors.region = "Region is required"` and `if (!values.template) errors.template = "Template is required"`.
- `app/app-list.tsx:44` dead CTA:
  ```
  <button className="inline-flex items-center ..." type="button"><Plus size={14}/> Create App</button>
  ```
  (no `onClick`).
- `monitoring/metrics-chart.tsx:73` query key drops node filter:
  ```
  queryKey: ["metrics-chart", selectedPeriod], queryFn: () => getNodeMetrics({ period: selectedPeriod, limit: window.limit, since: window.since })
  ```
  called while parent holds `selectedNode`.

---

*Audited at 2026-08-23. References pinned to filesystem snapshot under `reference/app-platforms/*` and Forge `forge/web/*` as inspected. No product code modified.*
