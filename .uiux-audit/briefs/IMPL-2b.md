# Impl scope 2 (continued) — Workloads A, remainder

Previous agent hit the turn limit. Read `.uiux-audit/impl/scope-2.md` first — it completed and
**currently compiles**: `app/admin/apps/page.tsx`, `app/admin/databases/page.tsx`,
`app/admin/database-services/page.tsx`, `components/database/databases-overview.tsx`,
`components/admin/AdminDatabases.tsx`, `components/database/container-view.tsx`. Verify, don't redo.

**DO NOT EDIT anything under `components/database/`** — another agent is in those files right now.

Your remaining work:

1. **`components/admin/AdminServers.tsx`** (1662 lines, a reference page — be surgical).
   - `:577-584` the row "▶" is an `<a>` to the console **labelled "Start"**. Either wire the real start
     mutation or label it truthfully. This is the S1 in your slice.
   - `:651`, `:863` "Create Server" stays enabled with zero available engines — disable with a reason line.
   - `:23` and `:24` duplicate `import { fetchWorkloadKinds } from "@/lib/api/capabilities"` — remove one.
   - Delete any hand-passed `title`/`sub`/`icon` that restate the registry so the frame derives them.
   - `:1123-1125` is the existing good capability-gating pattern; follow it elsewhere in the file.
2. **`app/admin/servers/**`** remaining pages — detail routes keep their resource-name title.
3. **`components/admin/AdminCatalog.tsx`** — `:651` asserts a "0" instance count while loading; a heading
   must not state a number the page has not read yet.
4. **`app/admin/app-store/**`** — check install actions report per-app results honestly and a failed
   catalogue read is not "The store is empty".
5. **Alias stubs you own:** `app/admin/workloads/**` (→ `/admin/servers`), `app/admin/game-servers/**`
   (→ `/admin/servers`), `app/admin/data/**` (→ `/admin/databases`), `app/admin/build/**` (→
   `/admin/catalog`), `app/admin/database-hosts/**` (→ `/admin/databases`). Copy the shape of an
   existing stub such as `app/admin/containers/page.tsx`. `test/route-integrity.test.ts` asserts every
   alias source is served by a real page.

Follow `.uiux-audit/briefs/IMPL-shared.md`. Append to `.uiux-audit/impl/scope-2.md` under `## Continued`.
Frozen: all shared primitives, `AdminAppsShared.tsx`, all Go. `rtk` is not installed.
