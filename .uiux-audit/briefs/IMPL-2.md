# Impl scope 2 — Workloads A (servers, apps, databases, catalog, store)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-2.md`, then
implement. Report to `.uiux-audit/impl/scope-2.md`.

You own: `app/admin/servers/**`, `app/admin/apps/**`, `app/admin/databases/**`,
`app/admin/database-services/**`, `app/admin/catalog/**`, `app/admin/app-store/**`,
`app/admin/workloads/**` + `app/admin/game-servers/**` (alias stubs → `/admin/servers`),
`app/admin/data/**` (stub → `/admin/databases`), `app/admin/build/**` (stub → `/admin/catalog`),
`components/admin/AdminServers.tsx`, `AdminDatabases.tsx`, `AdminCatalog.tsx`,
`lib/app-type-icons.ts`, `app/admin/databases/databases-overview.tsx` if present.

**Frozen (report under "Needs central change", do not edit):** `AdminAppsShared.tsx` — apps and
deployments both import it.

Top priority:
1. **Apps reports failure as absence.** `app/admin/apps/page.tsx:26` never destructures `isError`,
   so a dead API renders "No applications found." (`:102`). Same class:
   `databases-overview.tsx:342-347`, `:480` `?? 0` per source; `AdminCatalog.tsx:651` asserts a "0"
   instance count while loading.
2. **Databases renders two `<h1>`s** — the page (`databases/page.tsx:35`) plus every tab's
   `SectionHeader`. One heading per page. Also `app/admin/database-services/page.tsx:18` forwards to
   `?tab=services` which the page never parses, so it silently lands on Overview — either read the
   param or point the link at the real tab.
3. **Servers' row "▶" is not a start action** — it's an `<a>` to the console labelled "Start"
   (`AdminServers.tsx:577-584`). Label it truthfully or wire the real start mutation. `Create
   Server` stays enabled with zero available engines (`:651`, `:863`) — disable with a reason line.
4. **Apps' 7 icon-only row buttons have no accessible name** (`apps/page.tsx:147-173`) and the delete
   confirm renders as a *primary* button (`:186-201`) — it must read as destructive.
5. `AdminServers.tsx:23` and `:24` are duplicate `import { fetchWorkloadKinds }` lines. Delete one.
6. Header icons matched the registry 0/5 — now automatic; strip the hand-passed `title`/`sub`.
