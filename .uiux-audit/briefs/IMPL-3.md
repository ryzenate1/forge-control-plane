# Impl scope 3 — Workloads B (nests, templates, registries, forgefile, mounts, tags)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-3.md`, then
implement. Report to `.uiux-audit/impl/scope-3.md`.

You own: `app/admin/nests/**`, `app/admin/app-templates/**`, `app/admin/compatibility-templates/**`,
`app/admin/registries/**`, `app/admin/forgefile/**`, `app/admin/app-mounts/**`, `app/admin/tags/**`,
`app/admin/templates/**` (alias stub → compatibility-templates),
`components/admin/AdminNestsEggs.tsx`, `AdminTemplates.tsx`, `AdminEggVariables.tsx`,
`forgefile-manager.tsx`, `app-mounts-manager.tsx`, `tags-manager.tsx`,
`lib/api/tags.ts`, `lib/app-templates-data.ts`.

This is the **unwired-features** slice. The rule from the shared brief applies hardest here: a control
that cannot do its job gets removed or honestly disabled, never decorated.

1. **Tags is a catalogue for a capability that does not exist.** `assignTag`, `fetchTagResources`,
   `bulkActionByTag` in `lib/api/tags.ts` have zero callers, and `TagBadge` renders only inside
   `tags-manager.tsx` — tags never appear on a server, app or database. `tags-manager.tsx:124`
   promises "restart every server tagged X". Read the Go handlers (`grep -rn tags
   forge/api/internal/http/`) to find what the API genuinely supports; wire the attach flow if it
   exists, otherwise delete the promises and the bulk-action UI and report what you removed. Do not
   leave buttons that toast success over no-ops.
2. **App Templates is a localStorage island.** The page writes via
   `lib/app-templates-data.ts:68` (`forge.app-templates.v1`) while consumers read
   `fetchAppTemplates()` (`lib/api/apps.ts:536`) from `GET /admin/app-templates`, falling back to
   localStorage only on `TypeError`. So browser-created templates are invisible to the product.
   Check whether a create/update endpoint exists for app templates; if it does, write through the API
   and drop the localStorage path; if not, make the page say plainly that templates are local to this
   browser.
3. **Legacy Templates "New Template" POSTs a different resource than the list reads**, dropping 8 of
   12 fields and filing the egg into a hardcoded "Games" nest (`AdminTemplates.tsx:116-125`,
   `store_templates.go:47-63`). Delete uses a 3-second double-click — replace with `useConfirm`.
   Read the handler and match the payload.
4. **Image Registries leaks secrets and lies about verification.** Token field is `type="text"`
   behind a `••••` placeholder (`registries/page.tsx:106`); verify discards the per-node `results` and
   toasts fleet success (`:41` vs `handlers_registries.go:140-176`); a failed list renders "No
   registries configured".
5. **Nest-delete copy promises a cascade the API refuses** (`store_nests.go:173`). Match the copy to
   the contract.
6. Naming authority is split three ways: `lang/en.json:222,224` vs registry vs hardcoded `<h1>`
   ("App Mounts" vs "App Storage", "Forgefile (env-as-code)"). Registry wins; delete local strings.
