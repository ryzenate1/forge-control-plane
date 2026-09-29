# Impl scope 2 — Workloads A (servers, apps, databases, catalog, store)

Working record. Appended after each page; every section below is finished work.

Shared helpers adopted for the "unknown is not zero" class: `isAvailable`, `sourceState`,
`countLabel` from `lib/admin/telemetry.ts` (imported, not edited), `FreshnessBadge` from
`telemetry-ui.tsx`, and `formatDate` from `lib/utils`. Frame copy is now derived from
`admin-registry.ts` by `SectionHeader`, so hand-passed `title`/`sub` are being deleted.

## Done

### `app/admin/apps/page.tsx` (Applications list)
- **S1 failed read → "No applications found."**: destructured `isError`/`error` and added a
  precedence chain loading → error → empty. A failed `fetchApps` now renders `AdminErrorState`
  with a Retry; the empty state is only reachable when the query actually returned [].
- **S1 filtered count as total**: card header count is gated behind `isAvailable` (no count
  asserted before the query settles) and now reads `N of M applications (filtered)`, matching
  the Servers grammar, instead of `${filtered.length} applications`.
- **S1 seven icon-only buttons with no accessible name**: every row control passes `ariaLabel`
  (and `title`) naming the action and the resource — stop/restart/start/logs/console/delete.
- **S1 delete confirm rendered as a primary action**: replaced the bespoke `Modal` +
  `ModalFooter` (which defaulted to `destructive = false` → `tone="primary"`) with
  `useConfirm({ danger: true })`, the grammar the rest of the slice uses.
- **S1 lifecycle actions hidden for unmeasured status**: stop/start/restart now always render,
  disabled with a one-line reason instead of vanishing. `actionsFor()` covers all eleven
  `AppStatus` members; `unknown`/`pending`/`starting`/`deploying`/… explain why nothing can be
  pressed, and an `unknown` row says so in the Status cell.
- **S2 page-wide `isPending`**: each button reads its own mutation's `variables`, so the row in
  flight shows `loading` and its siblings' controls stay truthful.
- **S2 frame**: root is now `AdminPageLayout`; hand-passed `title`/`sub` deleted so the heading,
  sidebar row and breadcrumb all come from the registry (the old `sub` was a 47-word paragraph
  using a *Build/Deploy* vocabulary the registry never uses). Passing `info={adminPageGuides.applications}`
  — the guide existed and was dead.
- **S2 deleted the BUILD → DEPLOY banner**: it restated the registry vocabulary as body copy with
  four inline navigation links to routes outside this scope. The `info` disclosure now carries
  the same "define, then deploy" explanation.
- **S2 no-match vs no-data**: empty state distinguishes the two and offers Clear filters.
- **S2 unlabelled controls**: the search `Input` gets a real `label`, the type `<select>` gets
  `aria-label`. Added a Refresh button and `FreshnessBadge` on the header `status` slot — the
  page polls at 15 s and previously never said so.
- **S2 pagination**: added the shared `Pagination` primitive + page size, matching Servers; the
  whole list no longer renders at once.
- **Tokens/a11y/time**: `text-slate-*`, `bg-white/[0.0x]`, `text-[10px]`, `text-red-300` and the
  raw table markup moved to `border-line` / `text-text*` / `divide-line` / `AdminTable`; the
  hand-rolled `new Date(…).toLocaleDateString()` replaced with the shared `formatDate`.
- **S3 truncated id in prose**: dropped (the subtitle that baked `app.id.slice(0, 8)…` is gone).

### `app/admin/databases/page.tsx` (Databases frame)
- **S1 two `<h1>`s**: deleted the hand-rolled `<h1>Databases</h1>`, its raw lucide
  `Database` chip and its hardcoded `Workloads / Databases` breadcrumb row; the page
  is now one `SectionHeader`, which derives title, description **and** icon from
  `admin-registry.ts` (`Databases` / the 3-part line / `DatabaseCylinderIcon`). The
  tab views no longer render a second page title (see below). `OfflineBanner` kept.
- **S1 `?tab=` was never parsed**: the active tab is now read from the URL and
  written with `router.replace`, ported from the `apps/[id]` pattern. Every one of
  the five database views is now deep-linkable and survives a reload, and the
  `/admin/database-services` stub's `?tab=services` forward actually lands on
  Services instead of silently showing Overview.
- **S1 "Import" did not import**: removed. Its `onClick` was
  `setActiveTab("hosts")`, and the tab strip immediately below it already offered
  "Database Hosts", so the control was both a mislabel and a duplicate.
- **S1 "Create Database" created nothing**: the header button is now
  "Add database…" — which is what it does, since it opens a six-path directory
  rather than a form. The modal's own copy now states that plainly and each entry
  carries a `destination` line saying whether it opens a form here, switches tab, or
  leaves Databases (three of six do). The real create form in the Managed tab keeps
  the "Create Database" label, so there is now one button with that promise.
- **S2 duplicate import**: `admin-ui` was imported twice (`:10`, `:12`); merged.
- Added a Refresh that invalidates the six database list queries rather than
  remounting the tab, so filters and page position survive it.
- `info={adminPageGuides.databases}`.

### `app/admin/database-services/page.tsx` (alias stub)
- **S1 stub rendered full product chrome / S1 no loading state**: dropped the
  bespoke `title="Database Services"` + invented description + hand-styled brand
  `<Link>`. It now renders `SectionHeader` with no props — which resolves through
  `ADMIN_ALIAS_ROUTES` to the **Databases** entry, so the heading during the redirect
  is the truth of where you are going — plus `AdminLoadingState`. The `useEffect`
  → `router.replace` stays (the filesystem route is what answers the old URL;
  deleting it is reported below).
- **S1/S2 broken forward**: resolved by the Databases `?tab=` work; the copy that
  promised a destination the destination could not reach by URL is gone.

### `components/database/databases-overview.tsx` (Overview tab)
- **S1 KPI counts reported 0 for a source that failed to load** — the brief's top
  priority. `counts` was `hostsQ.data … : 0` / `managedQ.data?.length ?? 0` with an
  error gate that only fired when *every* source failed *and* the merged list was
  empty, so one `retry: false` source erroring rendered "Catalog 0" and an
  under-counted "Total Databases" as measured facts. Each source now resolves
  through `isAvailable()` to `number | undefined`; `undefined` renders `—` with a
  reason, the total refuses to render a number until all six have reported, and
  while any source is unreachable the inventory banner names which ones.
- **S1 failed read → empty list**: added the loading → error → empty precedence
  chain (`AdminLoadingState` / `AdminErrorState` with a retry that re-runs all six
  sources). Before, a total failure rendered the toolbar and filters over an empty
  array.
- **S2 local `statusTone()` word list**: deleted. Status is rendered as reported and
  coloured by `resolveTone`, so `unknown` is `unknown` rather than this file's
  `neutral`, and the `PillTone` type is no longer imported from the legacy
  `dashboard-cards` frame. `resourcesLabel()` replaces `${memoryMb}MB / ${cpuShares} CPU`,
  which rendered "undefinedMB" as a configured figure.
- **S2 filtered count as total**: `Databases ({sorted.length})` and
  `Showing 1–20 of {sorted.length}` now name the denominator the Servers way —
  `N of M databases (filtered)`.
- **S3 raw `‹`/`›` glyph pagination** → the shared `Pagination` primitive; the
  red-brand active page chip is gone with it.
- **S3 truncated name only in a `title`**: `max-w-44 truncate` + `title={r.name}`
  was the only place the whole name existed; rendered in full instead. Same for the
  cards view.
- **S3 dismissible banner that returned on every navigation**: dismissal persists to
  `localStorage`. Copy rewritten to drop the "one-click" claim the registry also
  makes and cannot honour (reported centrally).
- **S3 `DbStatCards` passed 7 KPIs into a 5-column grid** (two tiles orphaned onto a
  third row) → `xl:grid-cols-4`, and the seven tiles now each have their own glyph:
  `Box` labelled both "Managed DBs" and "DB Containers", `Database` both "Total"
  and "Server DBs".
- **Tokens**: `text-slate-*`, `bg-white/[0.0x]`, `bg-black/20`, `text-[10px]`,
  `text-[11px]`, `border-red-500/40`, `bg-sky-500/*`, `bg-emerald-*`, `bg-amber-*`
  and the raw table markup → `border-line` / `bg-overlay-*` / `text-text*` / `t-title`
  / `ui-table` / `ui-th` / `brand-*` for the active view toggle. Status colour now
  comes only from `resolveTone`.
- Cards view items are `<article>`s and their bare "Open" buttons say *where* they
  open (`openLabel`), matching Catalog's card shape.

### `components/admin/AdminDatabases.tsx` (Database Hosts tab)
- **S1 double `<h1>` (half 2)**: this tab's `SectionHeader title="Database Hosts"` was the
  second `<h1>` on the route. Replaced with an `<h2 className="t-title">` sub-heading plus its
  description and the action slot, so the heading order is h1 (Databases) → h2 (Database Hosts) →
  h3 (Hosts list). Same demotion applied to the other three tab views.
- **S2 raw API route in operator copy**: the `INFRA · Storage` banner ended in
  `POST /database-hosts/:id/test`. Rewritten as prose about what a host is, that credentials are
  stored encrypted, that `verify-full` is the TLS default, and that an untested host can still be
  created but will fail at provisioning — no route strings.
- **S1 unknown as zero in the tiles**: `totalDatabases` was
  `hosts.reduce((sum, h) => sum + (h.databases ?? h.maxDatabases ?? 0), 0)`, so every host that
  reported neither silently contributed `0` to a figure presented as measured. Now uses the
  canonical `reportedTotal`/`isPartial`: the tile shows the sum of what answered and marks it
  `partial` with the reported/total count when some hosts did not answer. All four tiles are gated
  on `isAvailable(hostsQuery)` and render `—` rather than `…`/`0` before the query settles.
- **S2 filtered count as total**: `Hosts ({sortedHosts.length})` and
  `Showing X–Y of {sortedHosts.length} hosts` reported a filtered subset as the population. Now
  `N of M hosts (filtered)`, and the two duplicated range strings share one `hostsRangeLabel`.
- Added a Refresh on the sub-heading (the tab previously had none).
- **Tokens**: 44 palette-literal sites → 0 (`text-slate-*`, `text-white`,
  `border-white/[0.06-0.2]`, `bg-white/[0.0x]`, `bg-black/20`, `divide-white/[0.04]`,
  `bg-[var(--surface)]`, `border-red-500/*`, `bg-red-950/10`, `border-emerald-500/20`,
  `text-[10px]`/`text-[11px]`), now `border-line`/`border-line-strong`, `bg-overlay*`,
  `text-text*`, `text-meta`, `text-danger`/`border-danger-line`/`bg-danger-subtle`,
  `text-ok`/`border-ok-line`/`bg-ok-subtle`, and `brand-*` for the active view toggle instead of
  red. The hand-written hosts error panel is now `AdminErrorState`.
- `DbStatCards` call sites updated for the new `DbStat` shape (`hint`, no `tile`), with distinct
  glyphs per tile.

### `components/database/container-view.tsx` (DB Containers tab)
- **S1 double `<h1>` (half 3)**: `SectionHeader title="DB Containers"` → `<h2 className="t-title">`
  sub-heading, so the tab no longer puts a second page title on the route.
- **S1 unknown as zero**: `memorySum` was `containers.reduce((sum, c) => sum + (c.memoryMb ?? 0), 0)` —
  a container that never reported memory was added as a measured 0. Now `reportedTotal`/`isPartial`,
  rendering `—` when nothing reported and marking the figure `partial` when only some did. All four
  tiles gated on `isAvailable`. The per-row `{db.memoryMb}MB / {db.cpuShares} CPU` (which rendered
  `undefinedMB / undefined CPU`) now uses the shared `resourcesLabel`, which says
  "memory not reported" instead of inventing a number.
- **S2 word-list status buckets**: `["running","ready"].includes(...)` replaced by
  `resolveTone(status) === "ok"`, so the tile and the pill cannot disagree about which statuses count
  as running.
- **S2 the list error was a bespoke red panel and loading was the string "Loading"**: now
  `AdminErrorState` (with retry) and `AdminLoadingState`.
- **A11y — three icon-only row buttons had `title` but no accessible name**: restart/backup/delete now
  carry `aria-label` naming the container, plus `focus-visible` rings. The bare "Pending" cell says
  what is pending ("Credentials pending", with the reason in `title`).
- **S2 page-wide `isPending`**: added `rowPendingAction(id)` so the button actually in flight shows a
  spinner and the other rows read as idle; `AdminConfirmDialog` shows progress for *its* row.
- Delete now states what is affected (container + stored data, removed from the node) and the delete
  control explains why it is disabled while the container is provisioning.
- **S3 raw `‹`/`›` glyph pagination** → shared `Pagination`, in both the cards and table views, and the
  `of {sorted.length} containers` total now names the filtered denominator.
- **Tokens**: `text-slate-*`, `bg-black/20`, `border-white/*`, `bg-white/[0.0x]`, `divide-white/*`,
  `border-red-500/*`, `bg-red-950/10`, `hover:text-amber-200`, `text-[10px]`/`text-[11px]` → tokens;
  `text-[11px]`→`text-meta`. Local `en-GB` date format → shared `formatDate`.
- Added a Refresh control (this tab had none) and the copy that a container may also back a database
  created through another surface.

## Verified

## Needs central change (do NOT edit these yourself)

## Removed

## Deferred

## Continued (agent 2 — remainder)

### Alias stubs created
- `app/admin/workloads/page.tsx` → redirect("/admin/servers")
- `app/admin/game-servers/page.tsx` → redirect("/admin/servers")
- `app/admin/data/page.tsx` → redirect("/admin/databases")
- `app/admin/build/page.tsx` → redirect("/admin/catalog")
- `app/admin/database-hosts/page.tsx` → redirect("/admin/databases?tab=hosts") — valid tab id per the `?tab=`
  parser the predecessor added; lands on Hosts, not Overview.
All copy the shape of `app/admin/containers/page.tsx` (server `redirect`, 307, no chrome flash). These close the
route-integrity gap "every alias source is served by a real page". Other ADMIN_ALIAS_ROUTES sources were not
assigned to this scope and were left alone.

### `components/admin/AdminServers.tsx`
- **S1 row "▶ Start" was a console link** (`:577-584`): replaced the `<a>` with a real button wired to
  `sendPowerSignal(id, "start")` — the same endpoint the modal's powerMut uses. A dispatch the control plane
  does *not* accept throws ("The control plane did not accept the start signal"); it never toasts success for
  work not performed. Per-row pending via `rowStartMut.variables?.id`. Blocked states render **disabled with a
  one-line reason** (suspended → "unsuspend from the Manage tab first", running → "already running",
  starting/installing → in-progress). Navigation stays available honestly via `RowMenu` ("Open overview" /
  "Open console"); the row control no longer claims a power action it does not send.
- **S1 KPI sub-lines asserted "0 running • 0 stopped • 0 error" during load** (`:213/:228`): value and sub now
  gate together — "…" while loading, "—" + "Not measured — the servers read failed" on error.
- **S1 `pct()` "0% of total" from a 0/0 division** (`:99`): returns "—" when there is no denominator.
- **S1 Create Server submittable with zero/available=false engines** (`:651/:673-687/:863`): `validationError`
  now refuses while engine availability is unverified, and refuses unless the selected engine is one the control
  plane reports available (`availableKinds.some(provider === runtimeProvider)`). ModalFooter gates on this, and
  the reason renders in the existing validation panel; the availability error panel gained a real **Retry**
  (`kindsQuery.refetch`) since it told the operator to retry rather than guess.
- **S2 refresh faked pending with `setTimeout(…, 500)`**: `isRefreshing` state deleted; the spinner now reads
  `serversQuery.isFetching`. Header gained `status={<FreshnessBadge state={sourceState(serversQuery)} />}`.
- **Copy/frame**: hand-passed `title`("Servers") + `sub`(paraphrase) deleted — the `<h1>` now derives
  "Game Servers" + registry description + GamepadIcon glyph from `admin-registry.ts` (was a 3-way disagree with
  the sidebar). The hand-built `PageInfoDisclosure` moved into the header's `info` slot (its content preserved;
  see Needs-central for the guide entry that should replace it).
- **S2 status vocabulary**: `statusSubtext` no longer calls a `starting` server "Installing…" (says
  "Starting…"), and the status filter option is "Installing / starting" — one field, one vocabulary.
- **S3 RowMenu a11y**: `aria-haspopup="menu"`, `role="menu"`, `role="menuitem"`, Escape closes.
- **S2 error panel** → shared `AdminErrorState` with retry.
- **Tokens**: 213 palette-literal sites → 0 (`text-slate-*`→`text-text*`, `border-white/[0.04-0.2]`→
  `border-line*`, `bg-white/[0.01-0.08]`+`bg-black/20`+`bg-slate-800/50`→`bg-overlay*`, `divide-white/*`→
  `divide-line`, `text-[10px]/[11px]`→`text-eyebrow`/`text-meta`, emerald/amber/red semantics→`*-ok/warn/danger`
  token families, `accent-red-500`→`accent-brand`, input focus→`focus:border-brand focus:ring-brand-subtle`).
  Brand-*active* states (pagination chip, selected engine tile) moved off raw red onto `border-brand-line
  bg-brand-subtle`; error panels onto `border-danger-line bg-danger-subtle text-danger`. Chart accents
  (sky/purple/orange series colours) left as-is — they pair with `lib/design-tokens` `chart.*`.
- **Verified**: `fetchWorkloadKinds` duplicate import (`:23`/`:24`) confirmed absent (line 24 is the
  SectionHeader import now); audit already flagged that lead as stale. `npx tsc --noEmit` filtered to this file: clean.
