# Scope 1 — Reference pattern spec + Access group

Audited: 11 registry routes (Access group), reference set Overview/Monitoring/Activity (+Game Servers/Nodes as pattern sources), plus alias/dead-code surface.
Frames in use (Access slice, observed): SectionHeader+AdminPageLayout (partial, migrating) · DashHeader 1 (AdminUsers) · hand-rolled hero 2 (AdminOverview, pre-migration AdminMonitoring) · bare inline headers 8+.

> **Snapshot caveat — the tree is moving under this audit.** HEAD is `501cc90` with a dirty working tree; `git status` shows ~20 admin pages modified while this audit ran. Concrete mid-audit drift: `components/admin/AdminMonitoring.tsx` changed between two reads from a hand-rolled "Command / Monitoring" hero (1176 lines) to `AdminPageLayout` + `SectionHeader` + `FreshnessBadge` (1108 lines); `app/admin/activity/page.tsx` lost its `AdminPageLayout` wrapper mid-session. Findings below cite the state observed at the time noted; a remediation pass must re-verify each line anchor against the then-current tree.

---

## 1a. What makes the reference pages right (and where they still disagree)

### Render order actually emitted, per page

**Overview** — `app/admin/overview/page.tsx` (7-line wrapper) → `components/admin/AdminOverview.tsx` (2034 lines):
1. `AdminPageLayout className="space-y-6"` — `AdminOverview.tsx:796` (overrides the layout default `space-y-5`, `admin-ui.tsx:273`)
2. Hand-rolled breadcrumb row "Command / Overview" — `AdminOverview.tsx:800-806`
3. Hand-rolled freshness dot (custom markup, not `FreshnessBadge`) — `:806-825`, derived from `newestUpdate` = max of five queries' `dataUpdatedAt` — `:787-793`; `dataUpdatedAt === 0` correctly renders "Waiting for first read" — `:819-820`
4. Hand-rolled `<h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-100">` + `PageInfoDisclosure` — `:830-863`
5. Hand-rolled subtitle `<p>` — `:864-866`
6. Hand-rolled toolbar row (range `select` w/ `aria-label` `:873-885`, icon refresh `:888-895`, "Deploy" CTA routing to `/admin/servers` `:898-905`)
7. ZONE 2 KPI row (three hand-rolled cards, `grid-cols-1 md:grid-cols-3` — `:912-1044`), each card is a clickable `div` with `onClick` nav and no `role`/`tabIndex`/keyboard handler — `:914-916`
8. ZONE 3-5 content — `:1044, :1743, :1835`
9. ZONE 6 hand-rolled "Sources" ribbon — `:1921-1975` (a duplicate of `SourceRibbon`, `telemetry-ui.tsx:221-239`, with its own tone logic), ending in a **static, unconditional** pulsing emerald dot + "Live Telemetry Polling 30s" — `:1971-1974`.

**Monitoring** — `app/admin/monitoring/page.tsx` (7 lines, bare) → `AdminMonitoring.tsx` (state observed at 1108 lines): `AdminPageLayout` → `FreshnessBadge state={pageState}` (`:385-388`; `pageState` aggregates `sourceState(query, REFRESH.x)` across four queries, `:367-373`) → `SectionHeader title="Monitoring" sub="Platform, node and workload health dashboards"` (`:390-392`) → KPI/`MetricKey` cards (`:382-386`) → charts. Earlier snapshot (pre-migration, same session) had the same hand-rolled "Command / Monitoring" hero + custom dot as Overview.

**Activity** — `app/admin/activity/page.tsx` (10 lines, no layout wrapper as observed) → `AdminActivityLog.tsx` (640 lines): bare `<div className="space-y-6">` — `:257` → `FreshnessBadge` row — `:267-269` → hand-rolled `<h1>` "Activity" + `PageInfoDisclosure` — `:273-294` → hand-rolled toolbar (range select/refresh/export) — `:299-368` → four hand-rolled KPI tiles — `:370-388` → "By level" bar — `:389-414` → filter strip → table card with custom skeleton rows `:499-509`, error-as-`EmptyState` `:510`, real `EmptyState` `:512`, `AdminTable` `:519`.

**Game Servers** (Scope 2 owns fixes; used here as pattern evidence) — `AdminServers.tsx`: `SectionHeader` with `title={<span>Servers</span> + PageInfoDisclosure}` — `:178-202`, `sub="Game server workloads running across your Forge infrastructure."` — `:203`, `action` = primary `Btn` "Create Server" — `:204-208`; hand-rolled KPI tile row `:211-219`; per-server detail uses `DashHeader icon={Box} eyebrow="Game server"` — `:902-934`.

**Nodes** — `AdminNodes.tsx`: `AdminPageLayout` → `SectionHeader title="Nodes" sub="Beacon hosts, heartbeat status and capacity"` — `:76-78` (sub is the registry description verbatim, `admin-registry.ts:150`); per-source `AdminErrorState` rows — `:91-99`; search-in-card — `:101-106`; `AdminLoadingState` — `:108`; error — `:110-113`; `EmptyState` — `:114-115`; `AdminTable` — `:117`. Node detail (in a `Modal`) uses `DashHeader icon={Server} eyebrow="Compute node"` + `KpiGrid`/`InfoCard`/`QuickActionsCard` — `:279-295, :367, :370, :441`.

### Primitives they compose

| Concern | Primitive | Definition |
| --- | --- | --- |
| Page column | `AdminPageLayout` (`max-w-page`, `space-y-5`) | `admin-ui.tsx:272-274`; a second name-exported wrapper (shim) in `admin-layout.tsx:23-75` used by *-manager.tsx files |
| Page header | `SectionHeader` → emits **h1** `.t-page` (`text-2xl sm:text-display font-bold text-text`, `app/globals.css:240`) | `admin-ui.tsx:163-266` (h1 at `:249`) |
| Detail/entity header | `DashHeader` → emits **h2** `text-xl font-bold text-white`, icon tile, eyebrow hardcoded `text-red-400` | `dashboard-cards.tsx:20-65` (h2 `:39`, red eyebrow `:37`, truncated description `:42`) |
| Freshness | `FreshnessBadge` (loading/restricted/error/stale/read) | `telemetry-ui.tsx:157-212` |
| Provenance | `SourceRibbon` | `telemetry-ui.tsx:221-239` (Overview clones it by hand) |
| Truth atoms | `NotReported` / `Reading` / `MetricTile` / `AggregateTile` / `DataState` | `telemetry-ui.tsx:83-105, 345-422, 428-466, 251-321` |
| Info disclosure | `PageInfoDisclosure` (icon button + Dialog) | `components/ui/page-info-disclosure.tsx:25-104`; copy store `admin-page-guides.ts` |
| Table/loading/empty/error | `AdminTable/…, AdminLoadingState/Rows, AdminErrorState, EmptyState` | `admin-ui.tsx:471-477, 502-517, 616-618` |
| Toolbar | `AdminPageToolbar` (aria-labelled select + refresh Btn) | `admin-page-toolbar.tsx:8-34` — **zero consumers repo-wide** |

### How title/description/eyebrow/icon are bound — the crux

Nothing reads the registry. Every reference page **restates by hand**:

| Page | Rendered title | Registry label | Rendered description | Registry description |
| --- | --- | --- | --- | --- |
| Overview | "Overview" `AdminOverview.tsx:831` | "Overview" `admin-registry.ts:114` | "Your infrastructure at a glance. Live state, workloads, capacity and recent activity." `:865` | "Live control-plane summary and fleet health at a glance" `:114` — **differs** |
| Monitoring (pre-migration snapshot) | "Monitoring" | "Monitoring" `:115` | "Allocation trends across your nodes and workloads…" `AdminMonitoring.tsx:446 (old)` | "Platform, node and workload health dashboards" — **differs**; new snapshot copies it verbatim into `sub` `:392` |
| Activity | "Activity" `AdminActivityLog.tsx:274` | "Activity" `:117` | "Platform-wide audit history. Filters apply to the event count, table, and export." `:296` | "Human-readable audit and activity history" — **differs** |
| Game Servers | "Servers" `AdminServers.tsx:181` | **"Game Servers"** `:121` | "Game server workloads running across your Forge infrastructure." `:203` | "Game server instances and their lifecycle" — **differs**, and title disagrees with the sidebar row |
| Nodes | "Nodes" `AdminNodes.tsx:77` | "Nodes" `:150` | sub = registry verbatim `:78` | match |

Icons: registry icons (`OverviewDashboardIcon` etc., `admin-registry.ts:114-121`) are rendered **only in the sidebar** (`admin-shell.tsx:110`). No reference page header renders the registry icon; `DashHeader` call sites hardcode `Box` (`AdminServers.tsx:903`) / `Server` (`AdminNodes.tsx:280`). Eyebrow: `SectionHeader` has no eyebrow slot at all; `DashHeader` requires one and forces its color to `text-red-400` (`dashboard-cards.tsx:37`) — detail views pass free text ("Game server", "Compute node"), never the group title.

### Heading levels actually emitted

- `SectionHeader` → h1 (`admin-ui.tsx:249`). `AdminSection` → h2 (`:322`). Overview/Monitoring/Activity hand-roll h1 + raw `<p>` sub, then **bare text rows** for section labels; `AdminOverview` cards use uppercase span labels, not headings — so h1 is followed by non-heading section titles.
- `DashHeader` → h2 (`dashboard-cards.tsx:39`); `InfoCard`/`QuickActionsCard`/`TrendChart` → h3 (`:200, :237, :306`); `CardHeader`/`PanelCard` → non-heading div / h3 (`admin-ui.tsx:349-357`; `telemetry-ui.tsx:491-494`). A Servers/Nodes detail view opens with an **h2 and no h1** anywhere in the page subtree, and h2→h3 inside a `Card` under unnamed zones is untraceable hierarchy.

### Loading / empty / error / freshness handling

- Good: Activity's KPIs show "…" only while loading and "—" for missing (`AdminActivityLog.tsx:382`); Monitoring/Nodes use `FreshnessBadge`/`AdminLoadingState`/`AdminErrorState`/`EmptyState` (`AdminMonitoring.tsx:387`; `AdminNodes.tsx:108-115`); `DataState` encodes the correct precedence loading→restricted→error→empty (`telemetry-ui.tsx:251-321`).
- Bad (even among the four): Activity renders a query **error through `EmptyState`** ("Failed to Load") instead of `AdminErrorState` (`AdminActivityLog.tsx:510`); Overview bypasses all of it with a hand-rolled ribbon and a static "Live Telemetry Polling 30s" claim (`AdminOverview.tsx:1971-1974`) — S1 by the project's own rule (a claim not derived from any query state); two different staleness rules coexist (Overview: per-query 2× interval `AdminOverview.tsx:767-781`; Monitoring: page-level 75 s window `AdminMonitoring.tsx:404` old snapshot vs. `sourceState` now; `AdminActivityLog` uses `sourceState(query, 15_000)` `:268`); `KpiCard` renders missing values as "— —" and **draws a decorative synthetic upward sparkline whenever `trend` has <2 samples** (`dashboard-cards.tsx:108-120, 161`) — fabricated telemetry in the reference set's own KPI primitive.

### Dimensions that cannot be standards (reference set disagrees with itself)

1. **Breadcrumb**: shell renders the registry trail on every admin page (`admin-shell.tsx:400-425`); Overview and Monitoring pre-migration render a *second*, shorter, wrong-named one ("Command"); Activity renders none and documents why (`AdminActivityLog.tsx:258-266`). Dimension settled: **shell only**.
2. **Header primitive**: hand-rolled hero (Overview, Activity) vs `SectionHeader` (Monitoring now, Nodes, Servers list) vs `DashHeader` (Servers/Nodes detail). No majority.
3. **Freshness source**: `FreshnessBadge` (Activity, Monitoring) vs bespoke dot (Overview) vs hand-rolled ribbon pill logic (Overview zone 6).
4. **KPI primitive**: hand tiles (Overview/Servers/Activity) vs `KpiGrid` (Nodes/Servers detail) vs `MetricTile` (`telemetry-ui`) — the latter used by none of them.
5. **Width container**: `AdminPageLayout` (`admin-ui.tsx:272`, `max-w-page`) vs bare `div` under the shell's own `max-w-[1440px]` (`admin-shell.tsx:641`) — Overview vs Activity/Servers/Monitoring(old).
6. **Colors**: the "good" pages are the heaviest raw-token offenders: slate/hex-alpha matches (approx.): Overview 161, Servers 163, Nodes 111, Monitoring 104, Activity 63, `dashboard-cards.tsx` 39. `AdminOverview`'s h1 uses `text-slate-100` where `SectionHeader` uses `text-text` — the reference set cannot be copy-pasted wholesale; its *structure* is the standard, its *tokens* are not.

### Where the documented canonical frame fits

`admin-page.tsx` (`AdminPage` → `ForgePage`+`ForgePageHeader`, zero consumers) already implements the right binding rule: title/description/eyebrow/trail default to the registry for the current pathname (`admin-page.tsx:88-103`), explicit `title` is reserved for detail routes (`:50-54`), and freshness is only ever a passed-in `FreshnessBadge` — never rendered by the frame itself (`:25-30`). `ForgePageHeader` emits a proper h1 `.t-page` with eyebrow (`layout.tsx:109-140`). The contract below therefore merges `AdminPage`'s **derivation rule** with the reference set's **composition order and state vocabulary**; `AdminPageToolbar` (`admin-page-toolbar.tsx`) is likewise consumer-less and should be adopted rather than abandoned.

---

## Target frame contract

Rules a page must satisfy to be considered consistent. `[ia]` marks product/IA decisions for the orchestrator.

1. **One frame.** Every `/admin` route renders through a single column: `AdminPageLayout` (`admin-ui.tsx:272`) for now (or `AdminPage` once it gains consumers — do not add a fourth). No bare `<div className="space-y-6">` page roots; no per-page `className` gap overrides.
2. **Exactly one header, exactly one h1.** First child is `SectionHeader` (or `ForgePageHeader`). A page never emits a second breadcrumb row — `AdminShell`'s registry trail (`admin-shell.tsx:400-425`) is the only trail; `SectionHeader`'s `breadcrumb` prop is retired inside `/admin`. DashHeader (or equivalent hero cards) is allowed only inside detail modals/drills *below* the list page's h1, and its `h2` must then be genuinely subordinate.
3. **Page language is derived, not restated.** `title` = registry `label`, `sub`/`description` = registry `description`, eyebrow = registry group title, header icon = registry `icon` — resolved from `usePathname()` + `findAdminPage()` (`admin-registry.ts:345`) as in `admin-page.tsx:100-102`. Call-site strings equal to the registry are forbidden duplication; explicit `title` only for detail routes (resource name) and wizard steps. Byte-equality of any interim hand-passed strings with the registry is enforced by the route-integrity test.
4. **No invented section names in content.** "Command" appears nowhere in the registry; any breadcrumb/eyebrow text must exist in `admin-registry.ts` (or `ADMIN_SEGMENT_LABELS` for trailing segments).
5. **Freshness is one component bound to one state source.** Any "Live/Stale/Read …" claim is `FreshnessBadge` fed by `sourceState(query, refetchMs)` (`lib/admin/telemetry.ts`); mixed-source pages use `SourceRibbon` instead of per-page pill logic. A frame/ribbon never renders a static live claim (`AdminOverview.tsx:1971-1974` is the anti-pattern). `dataUpdatedAt === 0` renders "waiting", not "just now".
6. **Unknown is never zero, and never decorative.** Values go through `Reading`/`NotReported` (reason mandatory), `MetricTile`/`AggregateTile` (`kind` required, no trend unless real series). Ban `KpiCard`'s synthetic sparkline path (`dashboard-cards.tsx:108-120`) and "— —" placeholders; a check that didn't report renders dashed-empty, not 0 (cf. `SubsystemHealthMeter`, `admin-ui.tsx:122-127`).
7. **State precedence loading → restricted → error → empty**, rendered by `DataState`/`AdminLoadingState`/`AdminLoadingRows`/`AdminErrorState`/`EmptyState`. An error is never an `EmptyState` (`AdminActivityLog.tsx:510` anti-pattern); empty lists are allowed only in `ready` state.
8. **One toolbar.** `AdminPageToolbar` owns range + refresh (`aria-label`s already baked in); page primary CTA lives in the header `action` slot; batch actions sit above the table they act on; every destructive action goes through `useConfirm`/`AdminConfirmDialog`; filter/search state survives navigation (URL or store, not component state alone).
9. **Section vocabulary: h1 → h2 → h3 with real heading elements.** Section titles are `AdminSection`/`ForgeSection` h2 (`t-title`, `app/globals.css:238`); card titles are `CardHeader`/`PanelCard`/`AdminCard` headers; bare `<h3>`/uppercase `<span>` zone labels are not section headings. No h1→h3 jumps.
10. **Tokens only.** No `text-slate-*`/`bg-white/[0.0x]`/`text-red-400`/`text-[10px]` in pages or in `dashboard-cards.tsx`; use `text-text*`, `border-line*`, `bg-overlay-*`, `var(--*)`, `t-*` classes from `app/globals.css`. DashHeader must stop hardcoding its eyebrow color (`dashboard-cards.tsx:37`). Status colors come only via `resolveTone`/`toneStyles` (`ui/forge/status.ts`) — call sites pass tone names, never classes [ia: which neutral gray replaces slate-100 titles to be decided once, centrally].
11. **Capability-gated affordances render disabled with a one-line reason** under/next to the control (pattern: `AdminServers.tsx:1123-1125`), never hidden, never enabled-and-lying. `capability: "metadata-only"` entries must state that the page is a directory, not a console (Plugins, `admin-registry.ts:228`).
12. **Detail routes keep the frame.** `<id>` pages pass the resource name via the `BreadcrumbProvider` override (`lib/nav/breadcrumb-context.tsx`, consumed at `admin-shell.tsx:263-267`) rather than drawing their own trail, and reuse the same h1/frame primitives; every alias in `ADMIN_ALIAS_ROUTES` that operators may have bookmarked gets a redirect stub page, never a blank flash or 404.
13. **Responsive floor.** Tables live in an `overflow-x-auto` container (`AdminTable` already does, `admin-ui.tsx:471`); no page-level fixed `min-w-[NNNpx]`; KPI grids collapse 4→2→1 (`ForgeGrid`, `layout.tsx:239-256`); icon-only buttons use `AdminIconButton`/`ForgeIconButton` (accessible label required) and clickable cards expose role + keyboard handler.
14. **Info disclosure: one guide per route, stored centrally.** Page guides live in `admin-page-guides.ts` keyed by route (`app-store/page.tsx:237` is the only correct consumer today); the header primitive owns the `info` prop — until `SectionHeader` actually accepts it, no page may pass `info={{…}}` and claim a disclosure exists.

---

## Per-page findings — Access group (snapshot: live working tree, mid-migration; re-verify anchors)

Access-group registry rows: `admin-registry.ts:206-219`. Wrappers audited + their delegate components. Frame counts for this slice: SectionHeader/AdminPageHeader 10 · DashHeader 0 (was 1 before mid-audit edit) · hand-rolled root-div 2 · ForgePage/AdminPage 0.

### Organizations — /admin/organizations
- files: `app/admin/organizations/page.tsx` (52 lines, self-contained), `components/admin/admin-ui.tsx` primitives
- frame: AdminPageLayout + AdminPageHeader — `app/admin/organizations/page.tsx:40-41`
- title: "Organizations" vs registry "Organizations" (`admin-registry.ts:207`) → MATCH (restated by hand at `page.tsx:41`)
- description: page "Create tenant boundaries, then group their projects, environments, and members." (`:41`) vs registry "Tenants: the top of the organization → project → environment hierarchy" (`:207`) → **contradicts** (restated)
- icon: registry `Building2` (`:207`); page uses Building2 in CardHeader/EmptyState (`:47`, `:48`) → match-by-duplication; no header icon exists in `SectionHeader`/`AdminPageHeader` at all
- S2 · Tenancy explainer band is per-page free text with raw tokens — `page.tsx:42-44` (`border-white/[0.06]`, `text-slate-400`); duplicated across Projects (`projects/page.tsx:49-51`) but missing from Environments — same hierarchy, three different treatments
- S2 · Row click leaves the admin frame: `router.push(`/organizations/${slug}`)` — `page.tsx:48` sends operators to the non-admin `/organizations/*` area; breadcrumb/active-state contract (shell trail) breaks mid-task; no `/admin/organizations/[id]` route exists
- S3 · `router.push("/admin/organizations")` after create (`:34`) is a no-op navigation that resets scroll/state; invalidate already happened (`:28`)
- S3 · Owner fallback "Unknown" (`:48`) is honest, but `memberCount !== undefined` gate (`:48`) is the correct not-reported pattern — keep, and note it as the slice's only example
- Good: loading/error/empty all use `AdminLoadingState`/`AdminErrorState`/`EmptyState` with create CTA in empty (`:45`, `:48`); create is a real `<form>` with submit + `ModalFooter`.

### Projects — /admin/projects
- files: `app/admin/projects/page.tsx` (103 lines)
- frame: AdminPageLayout + AdminPageHeader — `:47-48`
- title: "Projects" = registry "Projects" (`admin-registry.ts:208`) → MATCH (hand-restated `:48`)
- description: page copies the registry string verbatim plus a period ("Projects grouping workloads inside an organization." vs `:208`) → match-by-duplication — exactly the drift contract rule 3 forbids
- icon: registry `Layers` (`:208`) vs rendered `FolderKanban` (`:5`, `:62`, `:63`) → **differs**
- S2 · "New project" disabled while no org selected with **no reason line** — `:48`; contract rule 11 (disabled-with-reason), and `M` rubric item
- S2 · Raw tokens in explainer band and rows — `:49-51`, `:69`, `:73-76` (`text-slate-*`, `divide-white/[0.06]`)
- S3 · Org selector is local state (`:13`) — lost on nav; Projects/OAuth/Environments all re-implement an unpersisted "select an organization" gate (compare `environments/page.tsx:16`, `AdminAccess.tsx:36` for owner) — one shared tenancy-scope control is missing (`scope-switcher.tsx` exists in the sidebar but is not used here)
- S3 · Project rows render as inert divs (`:70-78`) with slug in mono but no detail route and no action; org→project hierarchy is a dead end: nothing can be done to a project from this page
- Good: full loading/error/empty cascade per query (`:57-59`, `:63-68`); duplicate Organization `AdminSelect` inside the create modal (`:85`) at least prevents blind submit.

### Environments — /admin/environments
- files: `app/admin/environments/page.tsx` (237 lines)
- frame: AdminPageLayout + AdminPageHeader — `:118-119`
- title: "Environments" = registry "Environments" (`admin-registry.ts:209`) → MATCH (restated `:119`)
- description: "Manage deployment environments and environment variables" (`:119`) vs registry "Environment stages within a project, and their variables" (`:209`) → contradicts (restated)
- icon: registry `Globe` (`:209`); page renders Globe in both CardHeaders (`:156`, `:180`) → match-by-duplication; registry-side collision: `Globe` is also Domains (`:171`) — the very pages this one cross-links
- S1 · Reveal button is enabled-and-lying: `Eye`/`EyeOff` toggle sets `revealed[v.id]` (`:200-202`) but **no code ever renders `v.value` anywhere on the page** — clicking "Show" discloses nothing; operator believes secrets are being masked/unmasked when the affordance is dead
- S1 · Variable deletion has **no confirmation**: `handleDeleteVar` → `deleteVarMutation.mutate(varId)` directly (`:95-97`, `:206`) — destructive action without `useConfirm`, unlike every other Access page
- S1 · Revision-history failure renders as "No revision history available." — `:105-107` swallows the error into `setRevisions([])`, then `:216` shows the empty branch; unknown ≠ zero (AGENTS.md rule)
- S2 · Ad-hoc loading/error strings instead of primitives: `"Loading..."` `:158`, `:188`, `:214`; un-retried red text errors `:159`, `:189`
- S2 · Cascade selects are bare `<select>` with no `aria-label` and placeholder-option only — `:122`, `:126` (Projects' equivalent uses labelled `AdminSelect`, `projects/page.tsx:54` — same hierarchy, two grammars)
- S2 · Row selection: clickable `div` with `onClick` and no role/tabIndex/keyboard (`:163-167`); selected-state highlight is hardcoded **`bg-red-500/10`** (`:165`) — selection expressed in a danger colour, raw token
- S2 · Raw token inputs everywhere: `border-white/10 bg-black/30 text-white`, `focus:border-red-400/70` (`:122`, `:136`, `:184-185`); color-swatch buttons have no accessible name (`:141-143`)
- S2 · No tenancy explainer band (Organizations/Projects have it) — the three pages of one hierarchy look like three products, exactly the scope-1.md hypothesis, confirmed
- S3 · `revealed` state map (`:25`) is write-only dead state; password create-flow conventions differ from `AdminUsers` (no autofill hints)

### Users — /admin/users
- files: `app/admin/users/page.tsx` (7 lines), `components/admin/AdminUsers.tsx` (361 lines), `components/admin/user-limits.tsx` (62 lines)
- frame: AdminPageLayout + SectionHeader — `AdminUsers.tsx:179-184` (was DashHeader + raw hero before this audit's mid-session edit; drift noted)
- title: "Users" (`:181`) = registry "Users" (`admin-registry.ts:210`) → MATCH (restated)
- description: "All registered user accounts, administrative privileges, and resource limits on this panel." (`:182`) vs registry "Accounts, limits and status" (`:210`) → contradicts (restated)
- icon: registry `Users` (`:210`) rendered only inside CardHeader/StatsRow (`:190`, `:192`, `:212`) → match-by-duplication; header has no icon slot
- S1 · `StatsRow` renders `users.length` / `admins.length` **while the query is still loading and after it errors** — `:189-193` with `users = usersQuery.data ?? []` (`:17`): a failing users list shows "Total Users 0", not an error; contract rule 6 (unknown ≠ 0). Same pattern for `Servers` column `ownedCount` (0 when servers query failed, `:19-20`, `:144`)
- S2 · Bulk role change "Set User" bypasses confirm (`:203`) while the *less* dangerous "Set Admin" has one (`:204`) and deletion has one (`:205`) — inconsistent destructive gradient; demoting admins silently can lock operators out
- S2 · Raw tokens: ACCESS·Identity band `:185-187`; batch bar `:200`; modal footer reimplemented as a raw `-mx-6 -mb-5 … bg-white/[0.015]` div instead of `ModalFooter` — `:301`
- S2 · Row affordance mismatch: trailing `ChevronRight` (`:250`) promises the row opens, but only the email text is a button (`:242`); no full-row click/hover target
- S3 · Create-user submit disabled on `password.length < 8` with **no visible reason** (`:338`); "Delete" disabled in detail modal (`:302`) has its reason only inside the confirm dialog that never opens while disabled — contract rule 11
- S3 · Search/role filter/page state are component-local (`:27-30`) — lost on every navigation; filter persistence rubric item H
- S3 · Mixed indentation (single-space body, `:14-16`…) — hygiene visible to no one but signals the file was regenerated, not maintained
- Good: `useConfirm` wired (`:15`, `:343`); per-row checkboxes have accessible names (`:223`, `:235`); `ownershipKnown` (`:143`) blocks "Delete Safe" when ownership data is incomplete — the honest pattern the rest of the slice lacks.
- `user-limits.tsx` (shared by create+edit): S2 · unlabelled inputs — `<label>` not associated with `<input>` (`:10-17`), no `htmlFor`; numeric fields with no `inputMode`; S3 · expand toggle shows `▾/▸` glyphs (`:46`) with no `aria-expanded` (`:40-47`); raw tokens `:16`, `:39`, `:43`, `:49`.

### Roles & Permissions — /admin/roles
- files: `app/admin/roles/page.tsx` (2 lines), `components/admin/AdminAccess.tsx` (`AdminRoles`, `:14-28`)
- frame: AdminPageLayout + SectionHeader — `AdminAccess.tsx:23`
- title: "Roles & Permissions" (`:23`) = registry (`admin-registry.ts:211`) → MATCH (restated)
- description: "Additional Forge Control Plane roles. User assignment is available from the Users page." (`:23`) vs registry "Role definitions, scopes and permission sets" (`:211`) → **contradicts** and the page never delivers: no permission set exists on this page; the only acknowledgement is a modal footnote "the backend … not editing role permissions" (`:25`) — rubric N "metadata promises, UI doesn't ship"
- icon: registry `ShieldCheck` (`:211`) vs CardHeader `ShieldCheck` (`:24`) → match-by-duplication; registry-side collision: `ShieldCheck` is also Zero-Downtime Releases (`:146`)
- S2 · Delete-role button is icon-only with no accessible name: `<Btn tone="danger"…><Trash2/></Btn>` — `AdminAccess.tsx:24` (no `ariaLabel`/`title` passed; contrast mTLS revoke which at least uses `title`)
- S3 · Long single-line JSX (`:24-25`) — entire table + modal on one line; unmaintainable, but not user-visible
- S3 · `roles.length` baked into CardHeader title (`:24`) — acceptable (h2-level) but shows 0 while loading since CardHeader renders above the loading branch — same "count before measure" as Security below; the loading state is inside the same Card (`:24`) so the header says "0 roles" while `AdminLoadingState` shows below it
- Good: `useConfirm` on delete (`:24`); loading/error/empty complete (`:24`); no fake permission matrix.

### Single Sign-On — /admin/social
- files: `app/admin/social/page.tsx` (168 lines)
- frame: AdminPageLayout + AdminPageHeader — `:69-70`
- title: "Single Sign-On" (`:70`) = registry (`admin-registry.ts:213`) → MATCH (restated)
- description: starts with the registry string then appends "Discord OAuth, Steam OpenID and Authentik…" + an honesty disclaimer (`:70`) vs registry "Social and enterprise SSO providers" (`:213`) → drifted superset; the "does not test or claim provider connectivity" caveat (`:70`) is exactly what the registry description should carry instead of the page
- icon: registry `Fingerprint` (`:213`) vs rendered `Globe` (`:72`) → **differs** (and Globe is Domains/Environments' glyph — third-way collision)
- S2 · CardHeader count "0 providers" renders during loading: `providers.length` (`:72`) evaluated before the `query.isLoading` branch (`:73-74`) — same count-before-measure pattern
- S3 · Row icons are a private map `{discord: MessageCircle, steam: Gamepad2, authentik: KeyRound}` (`:119`) — fine as entity icons but `KeyRound` re-collides with ACME/OAuth/Vault
- S3 · Raw tokens: `divide-white/[0.04]` `:80`, secret-box classes `:109`–`:118` (mtls pattern) — here `text-slate-*` `:127-134`, `bg-black` hover on save `:162` (Btn is used — good)
- Good: **best secret hygiene in the slice** — server returns `hasClientSecret` only, input is `type=password` w/ eye toggle carrying `aria-label` (`:148-158`), placeholder honestly says "Stored securely — enter a new value to replace it" (`:153`); save toast explicitly disclaims unverified credentials (`:62`); loading/error/empty complete (`:73-79`).

### OAuth Clients — /admin/oauth-clients
- files: `app/admin/oauth-clients/page.tsx` (7 lines), `AdminAccess.tsx` (`AdminOAuthClients`, `:30-49`)
- frame: AdminPageLayout + SectionHeader — `:42`
- title: "OAuth Clients" = registry (`admin-registry.ts:214`) → MATCH (restated `:42`)
- description: "OAuth clients are owner-scoped. Select an owner to inspect credentials…" (`:42`) vs registry "OAuth clients authorised against the Forge API" (`:214`) → contradicts (restated; the *workflow* copy is arguably better but belongs in the registry)
- icon: registry `KeyRound` (`:214`) = rendered `KeyRound` (`:44`) → match-by-duplication; **intra-slice collision**: KeyRound is also Vault (`:218`) and ACME (`:177`) — three nav entries, one glyph, two of them in Access
- S2 · Clients table has **no `<thead>`**: `<AdminTable …><AdminTBody>` (`:44`) — four columns rendered under a `CardHeader` with zero column labels; the roles table beside it (same file, `:24`) has a full head
- S2 · Owner selector required before any data loads (`:37`, `:43`), local state, not persisted — same missing shared tenancy/owner picker as Projects/Environments
- S3 · Revoke button icon-only, no accessible name (`:44`) — same as Roles
- S3 · Secret-shown-once modal: plaintext secret in a `<pre>` (`:46`) — appropriate for one-time reveal; but **no copy button** (contrast API keys which has `copySecret`), hardcoded `bg-black/30 text-emerald-300` tokens, and no "re-generate" path if dismissed un-copied (client must re-create)
- Good: `useConfirm` on revoke (`:44`); amber notice when creating without owner (`:45`) — disabled-with-reason pattern nearly satisfied (Create stays enabled until fields complete, `:45`, with explanation).

### API Keys — /admin/api
- files: `app/admin/api/page.tsx` (7 lines), `components/admin/AdminApiKeys.tsx` (275 lines)
- frame: AdminPageLayout + SectionHeader — `:133-134`
- title: "API Keys" = registry (`admin-registry.ts:215`) → MATCH (restated `:134`)
- description: "Scoped API tokens with granular permissions." (`:134`) vs registry "API keys and programmatic access" (`:215`) → contradicts (restated)
- icon: registry `Code2` (`:215`) vs rendered `KeyRound` (`:139`) + `Shield` (`:238`) → **differs** (and KeyRound collides twice, above)
- S2 · Auto-verify on create (`:90-93`) fires a second network call whose failure text is rendered as a *success-box sibling* (`:230`) — "Verifying token authentication…" persists in the emerald success panel even if verification fails; wrong-state-in-right-container
- S2 · Revoke action icon-only `Trash2` (`:264`) — `Btn` receives no `title`/`ariaLabel` here (Vault at least passes `title`, `vault-provider-manager.tsx:200`)
- S2 · Scope chips render only the action fragment (`{scope.split(".")[1]}`, `:196`) with the meaning living in a `title` tooltip (`:195`) — title-only affordance, rubric K; group toggle draws a fake checkbox span (`:187-189`) with no `role="checkbox"`/`aria-checked`, chips are buttons with no `aria-pressed` (`:195`) — permission matrix is unscannable by AT
- S2 · Raw tokens throughout: `bg-white/5`, `border-white/[0.06]`, `text-slate-*`, amber/emerald hardcoded panels (`:153`, `:166-232`, `:246`)
- S3 · Header `sub` and `CardHeader` titles duplicate the same facts twice (`:134-138`); no `FreshnessBadge` on a page listing revocation-sensitive credentials (list never auto-refreshes — fine, but staleness is unlabelled)
- Good: best destructive/secret flow: `useConfirm` (`:115-119`), masked-on-blur token (`:121-130`), real `aria-expanded`/`aria-controls` on the scope disclosure (`:150-151`), copy with `aria-label` (`:222-227`) backed by a real 15 s clipboard wipe (`lib/clipboard.ts:20`), honest "Scopes are enforced by the API…" explainer (`:231`).

### Security Headers — /admin/security
- files: `app/admin/security/page.tsx` (7 lines), `components/admin/AdminSecurity.tsx` (151 lines)
- frame: **none** — root is bare `<div className="space-y-6">` (`AdminSecurity.tsx:43`), no `AdminPageLayout`; header is `AdminPageHeader` (`:44-47`)
- title: "Security Headers" = registry (`admin-registry.ts:216`) → MATCH (restated `:45`)
- description: "Global middleware headers and per-domain overrides for HTTP responses." (`:46`) vs registry "Security headers and per-domain policies" (`:216`) → restated superset
- icon: registry `ShieldAlert` (`:216`) vs rendered `Shield` (`:53`) and `Globe` (`:76`) → **differs** (Shield = Firewall's glyph `:174`)
- S1 · The whole "Global Middleware Headers" card is **hardcoded static copy claiming "active"** — `GLOBAL_HEADERS` constant (`:8-39`) + `Pill tone="blue">active"` (`:63`): the page never reads the deployed middleware config, and the "value" strings contain editorial fragments inside header syntax (`"script-src 'self' [unsafe-eval on monaco routes]"` `:11`, `"… preload (production)"` `:16`) presented as measured configuration. Registry already models the honest alternative (`capability: "metadata-only"`, `admin-registry.ts:228`) — this page ships product chrome for a read-the-file-at-runtime fact it did not measure
- S2 · Banner line invents a group: "PLATFORM · Security" (`:49`) while the registry files this page under **Access** (`:206-216`) — same invented-eyebrow class as Overview's "Command"
- S2 · Copy references dead navigation: "Advanced → Domains" (`:113`) — no "Advanced" group exists in the registry; and stale project identity "middleware_security.go" cross-reference (`:49`, `:56`) belongs in the page guide, not chrome
- S2 · Custom lazy-import fetch wrapper `function fetchJSON … import("@/lib/api/http")` (`:149-151`) — bypasses the "one HTTP primitive in `lib/api/*`" convention (AGENTS.md frontend rule), inside a component
- S3 · Hand-rolled loading string (`:93`), hand-rolled empty state (`:107-117`) instead of `EmptyState`; per-domain "policies" promised by title but the table only links to `/admin/domains/[id]` (`:137`)
- Also: no `useConfirm` needed (read-only) — but there is **no freshness affordance either**: static claims + no "as-of" anything.

### Private CA & mTLS — /admin/mtls
- files: `app/admin/mtls/page.tsx` (345 lines, self-contained)
- frame: bare `<div className="space-y-6 p-6">` (`:94`) — no `AdminPageLayout` **plus** its own padding, double-padding vs every other Access page; header `SectionHeader` (`:95-108`)
- title: "Private CA & mTLS" = registry (`admin-registry.ts:217`) → MATCH (restated `:96`)
- description: "The private certificate authority and the mutual-TLS identities it issues…" (`:97`) vs registry "Private certificate authority and mutual TLS identities" (`:217`) → restated
- icon: registry `Lock` (`:217`) vs rendered Shield/ShieldCheck/ShieldX (`:5`, `:163-167`) → **differs**
- S1 · "Node Certificates" card has **no loading/error gate**: `certs = certsData?.data || []` (`:90`) feeds the card title count ("(0)", `:237`) and the `EmptyState "No certificates"` (`:238-239`) while the query is in flight or failed — an operator can be told "no certificates" by a page that hasn't loaded; error of that query is never surfaced anywhere
- S2 · `status.*Count ?? 0` in `StatsRow` (`:164-167`) — unreported counts render as measured zeros (contract rule 6)
- S2 · Two raw `<table>`s (`:187-233`, `:241-292`) bypass `AdminTable` — no `overflow-x-auto`, no `aria-label`, six-column rows that will crush at `md` (rubric J); all cell styling is `text-slate-*`/`border-white/[0.0x]`
- S2 · Hand-rolled buttons ignore `Btn`: Generate CA (`:99-107`), modal buttons (`:133-147`), revoke icon-buttons (`:215-227`, `:274-286`) with `title` as the only name; `hover:bg-[var(--brand-dark)]` (`:101`) exists (`globals.css:17/109`) but differs from the `--brand-hover` used by Overview CTA (`AdminOverview.tsx:901`) — two hover vocabularies
- S2 · CA expiry card shows an **AlertTriangle warning for any expiry date** (`:172-182`) — years out or days out, same alarm; day math on client clock (`:178`)
- S3 · Defaults embed the legacy product name: `caOrg = "GamePanel"`, `caCN = "GamePanel mTLS CA"` (`:33-34`) — new certs ship with the wrong identity unless the operator notices
- S3 · "Loading status…" div (`:157`) and un-retried error card (`:159`); migration phase tiles hardcode `text-green-400` (`:307`)
- Good: revokes and Run Migration all pass through `useConfirm` (`:29`, `:217`, `:276`); mutations check `result.ok` instead of trusting HTTP 200 (`:69`, `:79`).

### Vault — /admin/vault
- files: `app/admin/vault/page.tsx` (5 lines), `components/admin/vault-provider-manager.tsx` (388 lines); `components/admin/webauthn-manager.tsx` (188 lines, **zero consumers**)
- frame: AdminPageLayout + SectionHeader — `vault-provider-manager.tsx:134-148`
- title: "Vault" = registry (`admin-registry.ts:218`) → MATCH (restated `:136`)
- description: long sub (`:137`) vs registry "HashiCorp Vault connections for live secret references" (`:218`) → restated superset
- icon: registry `KeyRound` (`:218`) = rendered CardHeader `KeyRound` (`:151`) → match-by-duplication (collision with OAuth Clients `:214` and ACME `:177` — three KeyRound nav entries, two in this slice)
- S1-adjacent (dead feature) · `WebAuthnManager` is orphaned: exports at `webauthn-manager.tsx:26`, imported by no page/component (grep across `app/`+`components/` finds zero consumers); it is also written in the **legacy client palette** — `text-ink`, `bg-ink`, `text-muted`, `border-red-300 bg-red-wash text-red-dark`, `bg-green-50` (`:131-150`) — classes that do not exist in the admin token vocabulary, and its intro copy is an API-routes dump ("POST /auth/webauthn/register/begin → … 6 routes total", `:132-133`). Passkeys have no home in the IA: not in the registry, not mounted. Either it belongs under Access (Single Sign-On?) or its code should be deleted; report as IA gap + dead code (rubric N)
- S2 · Refresh lives as a `Btn` in the header actions (`:140-142`) instead of `AdminPageToolbar` (which has zero consumers repo-wide); no `FreshnessBadge` despite `isFetching` being right there — a page that *tests* connections has no "last test" timestamp
- S3 · `text-slate-*` cell tokens (`:174-189`); icon-only delete `Btn` relies on `title` alone (`:200-202`)
- Good: the **reference-quality secret surface** — credentials never returned, masked hints only (`:44-47` comment + `:170`), test result honesty (`:108-112`), blank-means-keep-current edit semantics (`:251-253`, `:333`), full loading/error/empty cascade (`:152-157`), `useConfirm` with concrete blast radius (`:123-131`), and disabled-save with a visible reason line (`:361-368`) — this page demonstrates contract rule 11 done right; the Environments variable list should look like this page.

### Alias stub — /admin/access → /admin/users
- `app/admin/access/` **does not exist**; `ADMIN_ALIAS_ROUTES` maps `/admin/access` (`admin-registry.ts:264`) but only 4 of 33 aliases have redirect stub pages (`app/admin/{containers,logs,git-providers,database-services}/page.tsx`; e.g. `containers/page.tsx:3` `permanentRedirect`). The Access-relevant aliases `/admin/access`, `/admin/tenancy` (`:270`), `/admin/security-headers` (`:271`), `/admin/mtls-ca` (`:272`) **404 at the router** while the nav layer claims they resolve. S2 (rubric L): bookmarked operators hit not-found; registry comment `:236-241` concedes the split without closing it.

---

## Scope-level patterns

1. **Copy duplication is the root cause of every title/description finding.** All 11 Access pages restate registry `label`/`description` at the call site; 8 of 11 render a description that differs from the registry text the sidebar row promised. The fix is one primitive edit (derive from `findAdminPage(pathname)` as `admin-page.tsx:100-102` already does), not 11 page edits.
2. **Header-frame convergence is happening but is uneven**: 10/11 pages use `SectionHeader`/`AdminPageHeader` (good), but `AdminSecurity.tsx:43` and `mtls/page.tsx:94` root in bare divs (no width column, mTLS even adds rogue `p-6`). One sweep for page roots.
3. **The `info` prop is being passed to a primitive that ignores it.** During this audit the parallel migration added `info={{…}}` to `SectionHeader` call sites (`AdminNodes.tsx:78`, `AdminMonitoring.tsx:393`) while `admin-ui.tsx` (659 lines at observation) has no `info` in `SectionHeaderProps` (`admin-ui.tsx:140-161`) — disclosures silently drop and `tsc` should fail. The Access pages don't pass `info` at all. Contract rule 14; needs one coordinated primitive+call-sites change.
4. **"Count before measure" recurs**: `social/page.tsx:72`, `AdminAccess.tsx:24`, `mtls/page.tsx:90/237-239`, `AdminUsers.tsx:189-193` all render counts (0!) above their own loading branches. A `CountedCardHeader` that defers to `isLoading` kills this class once.
5. **Icon-only danger buttons without accessible names** (`AdminAccess.tsx:24`, `:44`; `AdminApiKeys.tsx:264`; `mtls/page.tsx:215-227/274-286`) vs. correct ones (`vault-provider-manager.tsx:196` uses `title`+labelled text). Should be `AdminIconButton` (which requires `label`, `admin-ui.tsx:588-590`) — 6 call sites.
6. **Destructive-action confirmation is inconsistent within a single file family**: roles/OAuth/API/mTLS/vault/users use `useConfirm`; **Environments deletes env vars with one click** (`environments/page.tsx:206`); Users "Set User" bulk demotion skips it (`AdminUsers.tsx:203`).
7. **Tenancy scoping is re-implemented four times** (org select in Projects, org+project cascade in Environments, owner select in OAuth Clients, sidebar `scope-switcher.tsx` unused by all) with none persisting selection across navigation. One `TenancyScopeSelect` primitive.
8. **KeyRound/Globe/Shield*/Layers collisions live inside Access itself**: OAuth Clients + Vault + ACME share `KeyRound`; Environments + Domains share `Globe`; Roles + Zero-Downtime share `ShieldCheck`; Security-Headers renders Shield (Firewall's) while registry gives it ShieldAlert; Projects renders FolderKanban while registry gives it Layers. Nav legibility fix belongs in the registry, one edit, eight rows.
9. **The slice's own best-practice samples already exist** — `vault-provider-manager.tsx` (masked credentials, disabled-with-reason, confirm with blast radius) and `social/page.tsx` (unverified-credentials honesty). Remediation should point pages at these, not invent new vocabulary.
10. **Raw-token saturation extends past `dashboard-cards.tsx`** (39 matches, incl. the fabricated sparkline `:108-120` and `text-red-400` eyebrow `:37`): every Access page body mixes `text-slate-*`/`border-white/[0.0x]`/`bg-black/30` with token classes in the same file.

## Proposed remediation for this scope

1. [frame] Land the `info` prop on `SectionHeader` (`admin-ui.tsx:140-161`, render `PageInfoDisclosure` beside the h1) **and** wire guides from `admin-page-guides.ts`; then batch-convert Access pages' hand-passed title/sub to registry-derived values (or adopt `AdminPage` as the one frame — orchestrator choice, see open questions).
2. [copy] Registry-sync sweep: fix the 8 divergent descriptions (or update the registry where the page copy is better — Environments, Projects, OAuth, API Keys, Users), and move the SSO "does not claim connectivity" and Security "these are facts as-of middleware build" disclaimers into registry `description`/guides so the sidebar stops over-promising.
3. [state] Replace count-before-measure headers: gate `CardHeader` counts on `isLoading`; add loading/error gates to `mtls/page.tsx:236-239`; render revision-history failure as `AdminErrorState`, not empty (`environments/page.tsx:105-107/216`).
4. [state] Environments: delete `revealed` dead affordance or actually render `v.value` (with sensitive masking like Vault's hints); route env-var delete through `useConfirm`.
5. [frame] Convert `AdminSecurity.tsx` and `app/admin/mtls/page.tsx` roots to `AdminPageLayout`; strip mTLS's `p-6`; move its two raw tables to `AdminTable`.
6. [a11y] Swap icon-only `Btn` trash buttons to `AdminIconButton` (6 sites); add `htmlFor`/`id` to `user-limits.tsx` inputs and `aria-expanded` to its disclosure; add `aria-label` to Environments cascade selects; `role=checkbox`/`aria-pressed` on API-key scope chips.
7. [tokens] One sweep: Access pages + `dashboard-cards.tsx` to `text-text*`/`border-line*`/`bg-overlay-*`; delete the fabricated sparkline branch (`dashboard-cards.tsx:108-120`); make `DashHeader` eyebrow use a token instead of `text-red-400` (`:37`).
8. [ia] Registry icon de-collision for the eight rows listed in pattern 8; add a WebAuthn/passkeys entry (under Single Sign-On as secondary?) or delete `webauthn-manager.tsx` — currently an unmaintained orphan in a dead palette.
9. [ia] Shared `TenancyScopeSelect` (org → project → env, URL-persisted) used by Projects, Environments, OAuth Clients; persist Users' search/filter/pagination to the query string.
10. [frame] Add the four missing Access alias stub pages (`access`, `tenancy`, `security-headers`, `mtls-ca`) as `permanentRedirect`, matching `containers/page.tsx`.
11. [state] Security Headers: mark the page `capability: "metadata-only"` in the registry OR fetch actual header config; remove the "active" pills while unmeasured (`AdminSecurity.tsx:63`); fix the "PLATFORM · Security" band to the registry group and "Advanced → Domains" copy.

## Open questions for the orchestrator

1. Which frame wins for the rewrite: fix `admin-page.tsx`/`ForgePageHeader` to carry guides + freshness and migrate, or finish the in-flight `SectionHeader + info` migration (callers already passing `info` suggest the latter is underway by another agent — two agents converging on this primitive must coordinate)?
2. Organizations detail: keep deep-linking out of `/admin` to `/organizations/[slug]` (current behaviour, `organizations/page.tsx:48`) or add `/admin/organizations/[id]`? Contract rule 12 needs an answer before breadcrumb derivation can be trusted for tenancy drill-ins.
3. Roles & Permissions: the page delivers less than its registry label promises ("…Permission sets") because the backend cannot edit permissions. Change the registry label/description, or is a permission matrix coming? UI shouldn't keep a title the platform can't fill.
4. Should `AdminPageToolbar` (zero consumers) be deleted, or adopted as contract rule 8's primitive — if adopted, Overview's/Activity's custom toolbars and every Access refresh `Btn` are the conversion set.
5. Clipboard claim verified TRUE — `lib/clipboard.ts:20` implements the 15 s wipe promised at `AdminApiKeys.tsx:229`; it should be adopted by the OAuth secret modal (`AdminAccess.tsx:46`, no copy affordance at all) instead of each page inventing secret handling. No orchestrator decision needed; noted so the remediation pass doesn't re-litigate it.
