# Scope 2 — Workloads A: the three primaries + catalog/store

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-2.md`, appending after each page.

| Registry label | href | primary | files to open |
| --- | --- | --- | --- |
| Game Servers | `/admin/servers` | yes | `components/admin/AdminServers.tsx` (**1662 lines — the reference; audit it too**) |
| Applications | `/admin/apps` | yes | `app/admin/apps/page.tsx` (401 lines) + `components/admin/AdminAppsShared.tsx` + `app/admin/apps/new/page.tsx`, `app/admin/apps/[id]/**` |
| Databases | `/admin/databases` | yes | `components/admin/AdminDatabases.tsx`, `app/admin/databases/**` |
| Database Services (alias page) | `/admin/database-services` | — | `app/admin/database-services/page.tsx` — registry says this is *a tab of* `/admin/databases`, not a nav row. Verify whether it is still a live separate page; that is a finding. |
| Service Catalog | `/admin/catalog` | More | `components/admin/AdminCatalog.tsx` |
| App Store | `/admin/app-store` | More | `app/admin/app-store/**` |

Known lead: `AdminServers.tsx:23` and `:24` are duplicate `import { fetchWorkloadKinds } from
"@/lib/api/capabilities"` lines. Confirm and look for more of the same in your files.

Focus areas for this slice:
- **The three primaries are three different products.** Servers uses `DashHeader` + `KpiGrid` +
  `AdminTabs`; Apps is a fat inline `page.tsx` with `AdminPageLayout` + `SectionHeader`; Databases
  uses whatever it uses. Characterise each difference in the header, the KPI/summary band, the
  filter bar and the row affordances.
- Apps list: type filter vs status filter placement; `APP_TYPE_ICONS` per-row icons vs the registry
  icon; pagination (`ChevronLeft/Right` imported — check it works and reports totals honestly);
  start/stop/restart/delete actions and their confirm + disabled states.
- **Workload honesty**: rows showing a status the control plane never confirmed; containers counted
  from a stats stream rather than a state endpoint (project rule: judge lifecycle by the state
  endpoint, never by stats surface).
- Databases: hosts vs managed instances vs linked services — the registry description promises all
  three in one page. Are they actually three tabs? Is `Database Services` reachable both as a tab
  and as its own stale page?
- Catalog vs App Store: two "browse and provision" surfaces. Compare their card grids, detail
  modals, search and empty states. Do they look like the same feature? Do both promise one-click?
- Capability gating: do create/install buttons explain *why* they are unavailable, or silently
  disappear?
