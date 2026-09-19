# Impl scope 3 — Workloads B (nests/eggs, templates, registries, forgefile, app storage, tags)

In progress. Appended per page; this file is the complete record of work done so far.

## Done

### Tags — /admin/tags (`components/admin/tags-manager.tsx`, `app/admin/tags/page.tsx`)
- Read the real contract first: `forge/api/internal/http/handlers_tags.go` + `internal/services/tags/service.go`
  + `internal/store/store_tags.go`. The backend is **complete and real**: catalog CRUD, `GET
  /tags/:id/resources` (mixed kinds with display names), `POST /tags/:id/bulk` (servers via
  `OperationService.DispatchPower`, apps via `appLifecycleAction`, environments fail closed), and
  `POST/DELETE /applications|servers|environments/:id/tags` with tag+resource existence checks.
  So the attach flow exists and was **wired**, not deleted.
- `tags-manager.tsx`: new "Attachments and bulk actions" panel — explicit tag selection (no default),
  tagged-resource table from `fetchTagResources` with per-row Detach (`unassignTag`), an attach form
  (`assignTag`) whose resource picker is fed by `fetchApps` / `fetchAllServers` lazily per kind, and a
  bulk runner (`bulkActionByTag`) that renders the server's **per-resource** `data[]` result table
  ("Bulk restart: N of M resource(s) accepted") instead of one fleet-wide toast.
- `tags-manager.tsx`: environments are **not** offered in the attach picker, with a one-line reason on
  screen — `fetchEnvironments(projectId)` is project-scoped, so offering the kind would require guessing
  a project (AGENTS.md: never resolve an ambiguous target silently). Server copy states that only
  servers and applications have a runtime action.
- `tags-manager.tsx`: deleted the false dashboard promise. Grep confirms `TagBadge` has no consumer
  outside this file, so the sub now comes from the registry and the panel says plainly that tags do not
  appear on server/app screens and no list filters by tag.
- `tags-manager.tsx`: delete confirm now reads real usage — `fetchTagResources` runs before the dialog
  and the copy states the measured count and names (first 3); if the read fails it says "usage could not
  be read, so the number of affected resources is unknown" instead of asserting a blast radius.
- `tags-manager.tsx`: fixed the stale success lie — `saveMut.reset()` on every dialog open, and the
  green "Saved." panel is gone (the modal closes on success, so it could only ever show as a lie).
- `tags-manager.tsx`: loading → error → empty precedence with `AdminLoadingRows` / `AdminErrorState`
  (retry) for the catalog, the resource list and the picker; header count only renders once data lands
  (`tagsQuery.data ? "…n tags" : "Tag catalog"`), no more "0" during load.
- `tags-manager.tsx`: token/labels/a11y pass — `text-slate-*`/`border-white/10`/`bg-red-950/10` → `text-text-*`,
  `border-line`, `ui-alert ui-alert-*`, `ui-field-error`, `ui-hint`; hex field got a `label`; swatches are
  `aria-pressed` buttons with colour names and a ✓ so selection is not colour-only; icon-free buttons are
  labelled (`ariaLabel`); `formatDate` from `lib/utils` replaces the page-local date formatting; the
  manager returns a fragment so `app/admin/tags/page.tsx`'s `AdminPageLayout` is the only container;
  hand-passed `title`/`sub` deleted (registry drives the `<h1>`) and `info={adminPageGuides.tags}` added.
- `tags-manager.tsx`: dropped the unused `CheckCircle2` import and the American/British split ("Pick a
  valid colour") so the page agrees with the registry sentence.
- Verified: `npx tsc --noEmit` filtered to `tags-manager|app/admin/tags` → no output (clean).

### App Templates — /admin/app-templates (`app/admin/app-templates/page.tsx`, `lib/app-templates-data.ts`)
- Read the contract: `handlers_apphosting.go:963-973` registers **only** `GET /admin/app-templates`
  (admin-gated, returns `defaultAppTemplates()`), and `handlers_appstore.go:34` notes the same. There is
  **no** create/update/delete route for app templates, so the API island cannot be wired — the page now
  says plainly what each source is instead.
- `app-templates/page.tsx`: split into two honestly-labelled sections. "Deployment catalog" reads the API
  through `fetchAppTemplates()` (`queryKey: ["app-templates"]`, the wizard's own key) and is **read-only**;
  "Saved in this browser" holds the localStorage entries with Edit/Delete and states in the section
  description that other admins never see them and that the wizard only reads them when the API cannot be
  reached. Browser entries are filtered out of the catalog card so the offline fallback cannot show the
  same template twice under two sources.
- `app-templates/page.tsx`: the `Branch` input is **deleted** — `AppTemplate` (`lib/api/apps.ts:139-151`)
  has no `gitBranch` field, so the value could never be persisted; previously it was also *prefilled from
  the CPU value* (`gitBranch: tpl.defaultResources.cpu`). Filed as a central change instead.
- `app-templates/page.tsx`: lossy port parsing replaced with validation. `parseInt(x) || 8080` silently
  turned a typo into a plausible port; `parsePorts`/`parseEnvVars` now reject a malformed entry and name it
  in the dialog, and save refuses to proceed. Added required-field checks per template type and a catch
  around `saveUserTemplates` so a quota/private-mode failure surfaces (`Could not save in this browser: …`)
  instead of the dialog sitting there with no message.
- `app-templates/page.tsx`: failed read no longer renders "No templates yet"; loading → error → empty
  precedence via `AdminLoadingRows` / `AdminErrorState` (retry) / `EmptyState`, and the card count only
  appears once data has landed.
- `app-templates/page.tsx`: hand-passed `title`/`sub`/`icon` and the `backAction`/`backLabel="Apps"` link
  (registry declares no parent) deleted; `<h1>` comes from `admin-registry.ts`, `info={adminPageGuides.appTemplates}`
  added, local `typeLabel` re-implementation deleted in favour of the one in `lib/api/apps.ts`, raw
  `<input>/<select>/<textarea>` replaced with `Input`/`AdminSelect`/`Textarea` so every control is labelled,
  icon-only edit/delete replaced with `AdminIconButton` + accessible names, `text-slate-*`/`border-white/10`/
  `focus:border-red-400/70` replaced with tokens, `Layers` glyph (Service Definitions' icon) removed, disk now
  displayed, ports shown instead of dropped.
- `lib/app-templates-data.ts`: added `readStoredTemplates()` returning `{ templates, error }` so corrupted or
  foreign localStorage content reports itself instead of silently reading as "you have no templates";
  `loadUserTemplates()` still exists (used by `lib/api/apps.ts`) and delegates to it.

### Image Registries — /admin/registries (`app/admin/registries/page.tsx`)
- Secret no longer leaks: the credential field is `type="password"` with `autoComplete="new-password"`
  (create) / `"off"` (edit), an explicit Show/Hide toggle, and the misleading `••••••••` placeholder replaced
  by copy that says what is happening. `Input` defaulted to `type="text"`, so the token was plaintext.
- Edit path wired (`updateRegistry` was exported and unused): opening Edit sends no credential and the label
  says "blank keeps the stored one" — matching `store_docker_registries.go:176`, which preserves the existing
  ciphertext for `""` or `"********"`. Rotating no longer means delete-and-recreate.
- Verify is now honest about the fleet: the per-node `results` the endpoint returns
  (`handlers_registries.go:150-176`) are rendered as a "Verification — <registry>" table with
  "Verified on N of M node(s)" and each failure's message; the toast says "succeeded on every node" only when
  `okCount === results.length`. A 502 (the API's all-nodes-failed answer, whose body the client cannot read)
  is reported as "failed on every node … no node reported a result" instead of a bare "login failed".
- A failed list renders `AdminErrorState` with retry plus the line "Credentials may still exist and still be
  used for pulls" — no more "No registries configured" over an outage.
- Wired the real consumer so the copy stops guessing: `store_servers_control.go:316-348` resolves a
  credential by **exact registry-host match** on the image. That is now what the form explains. The `isGlobal`
  flag is stored but read by nothing (no `is_global` reference outside its own INSERT/UPDATE), so the checkbox
  copy says it is recorded on the entry and that resolution matches on address, not on the flag.
- Remove is labelled (`AdminIconButton`), and its confirm now states the consequence (next pull/push from that
  address has no credentials; running workloads keep going). Verify is pending per row (`variables.id`) instead
  of greying out every row. Header title/sub come from the registry, `info={adminPageGuides.registries}`, count
  only after data lands, tokens swapped (`divide-line`, `text-text-*`, `ui-alert-danger`), rows wrap on mobile.

### Legacy Templates — /admin/templates + /admin/compatibility-templates (`components/admin/AdminTemplates.tsx`, `app/admin/templates/page.tsx`)
- **Create now writes the resource the list reads.** Read `handlers_templates.go` +
  `store_templates.go:37-65`: `POST /templates` accepts only `name/image/startupCommand/defaultMemoryMb`,
  hardcodes `installContainer: "alpine:3.21"` / `installEntrypoint: "sh"`, files the egg in the nest literally
  named `"Games"` and 500s if that nest does not exist — so 8 of the 12 collected fields evaporated. The page
  now posts `createEgg` (`POST /eggs`, `handlers_nests.go:147-181`, `nestId` required) with name, nest,
  description, all images, startup, `config.stop`, `config.features`, memory, install container/entrypoint/
  script and the file denylist, and the nest comes from a required `AdminSelect` with a placeholder.
- Delete uses `useConfirm` with the real contract (`migrations/043_unify_eggs_templates_mounts.sql:85-87`:
  `servers.egg_id` is `ON DELETE RESTRICT`): "servers already created keep running; a definition still
  referenced by a server cannot be deleted". The 3-second `setTimeout` double-click is gone.
- Failed reads no longer render as "No templates configured": `isError` → `AdminErrorState` with retry for
  both the definitions list and the nest list (create/import controls are disabled with a stated reason when
  no nest could be confirmed). The KPI `StatsRow` "0 Templates" tile is deleted — during load there is no count
  at all, so the card title reads "Compatibility definitions" until data lands, then "N definitions".
- One `<h1>`: the second `SectionHeader` ("Game Template Catalog") is now `AdminSection` (h2) and wraps its
  card. The component owns `AdminPageLayout` and `app/admin/templates/page.tsx` no longer wraps it again, so
  both routes render one frame.
- Nest targeting is no longer auto-picked: `ImportTemplateModal` defaults to "" unless `?nestId=` names a real
  nest, and says so; `preselectedNestId` is explained on screen instead of being silently applied.
- Legacy framing: a warning banner links to Service Definitions; the bundled catalog section states the entries
  ship with the build rather than being inventory; `createTemplate` import removed.
- Copy/tokens: hand-passed title/description deleted (registry + `info={adminPageGuides.compatibility}`),
  `HardDrive`-means-image and `Cpu`-means-memory glyphs replaced with `FileCode`/text, `text-slate-*`,
  `border-white/[0.0x]`, `bg-amber-500/10 text-amber-300` and `focus:border-red-400/70` swapped for
  `text-text-*` / `border-line` / `bg-overlay-subtle` / `AdminSelect`, truncated image+startup strings now print
  in full (`break-all`) instead of living only in a `title` attribute, every icon-only button is labelled,
  `<h4>`/`<p class=font-semibold>` headings normalised to h3/h2.

### Service Definitions — /admin/nests (`components/admin/AdminNestsEggs.tsx`)
- **Nest-delete copy now matches the contract** (brief item 5). `store_nests.go:172-177` refuses while the
  nest holds eggs, so the dialog no longer promises "the nest and its eggs will be permanently removed"; it
  reads the nest's own `eggCount` and says the delete will be refused until those eggs are moved or deleted.
  Egg-delete copy likewise states the `servers.egg_id ON DELETE RESTRICT` consequence rather than implying a
  free removal. A refused delete now also closes the dialog (it used to sit open over a failed mutation).
- Egg count honesty: `nest.eggCount ?? nest.eggs ?? 0` replaced by `eggCountOf()` which returns `null` for an
  unreported count, rendered as "Egg count not reported" instead of a measured "0 eggs".
- Hover-only `opacity-0 group-hover:opacity-100` nest actions made always visible (they were invisible to
  touch and keyboard-only focus while still in the tab order); icon-only clone/export/delete/edit controls
  now carry `ariaLabel`s through `AdminIconButton`.
- Bare "Loading" strings → `AdminLoadingRows`; the two off-token `bg-red-950/10` panels → `AdminErrorState`
  with retry; header count appears only after data lands.
- The raw `<table>` (5 unlabelled header cells, two empty `<th>`) is now `AdminTable` with a label and named
  columns; truncated `Docker image(s)`/`Startup` cells print in full (`break-all`) instead of living only in a
  `title` attribute.
- Action hierarchy: New Egg is primary, Import/Export subtle, and the mislabelled "Browse Templates →" is now
  "Game template catalog" with a `title` stating what the `?nestId=` handoff actually does (it preselects the
  import target; the list is not filtered).
- Hand-passed `title`/`sub` deleted (registry drives the `<h1>`), `info={adminPageGuides.nests}` added, and
  the egg `Tag` glyph (the Tags page's glyph) replaced with `Layers`/`Box`; tokens swept
  (`divide-line`, `text-text-*`, `bg-overlay-subtle`, `focus-visible:ring-[var(--focus)]` instead of
  `ring-red-500/40`/`hover:text-sky-400`). Mutations now toast on success and invalidate `templates` too, so
  the legacy page stops showing stale rows.

### Eggs (detail) — /admin/nests/[nestId]/eggs (`app/admin/nests/[nestId]/eggs/page.tsx`)
- The "0 eggs" tile next to "Loading eggs…" is gone: the card title reads "Egg definitions" until
  `eggsQuery.data` exists, then "N eggs" — no unmeasured count and no two contradictory claims at once.
- `nestQuery` now has an error branch: if the nest read fails, an `AdminErrorState` says the operator may not
  be looking at the nest they think they are, with a retry. Previously the page silently titled itself "Eggs".
- Removed the duplicated hover-only variables affordance (`opacity-0` icon tile whose only name was a `title`)
  and the third "Browse Templates →" link; one labelled catalog action remains in the header, one in the empty
  state. Icon-only edit/clone/export/delete buttons are `AdminIconButton`s with names.
- Loading/error → `AdminLoadingRows`/`AdminErrorState` (retry); image and startup previews no longer truncate
  at 30 characters behind a bare ellipsis — full values, `break-all`; `Tag` glyph replaced with `Layers`;
  `border-white/[0.0x]`, `hover:bg-sky-500/10`, `text-[10px] text-slate-500` replaced with tokens; `sub`
  deleted and `backLabel` now says "Service Definitions" (the registry label) instead of "Nests".

### Egg Variables — /admin/nests/[nestId]/eggs/[eggId]/variables (`variables/page.tsx`, `components/admin/AdminEggVariables.tsx`)
- **Fixed the literal escape garbage** (S1): `Loading variables\u2026` and `\u2014` were raw JSX text, so the
  browser printed the backslash sequences. Both states are now real components.
- **Stopped fabricating install metadata** (S1): `egg.installContainer || "alpine:3.21"` and
  `egg.installEntrypoint || "sh"` presented values the definition does not have; they read "Not set" now, the
  same way the memory row already did. Memory unit corrected from "MB" to "MiB" to match the field's meaning.
- **Reorder no longer fires a request per `dragover`** (S2, and a real data hazard): dragging across N rows
  issued N `POST /eggs/:id/variables/reorder` calls and the last writer won. Dragging now updates a local
  staged order and commits exactly once on drop; `move()` up/down buttons give a keyboard and touch path and
  each issues one request; the footer states whether the new order is staged, saving, or settled.
- Second `<h1>` removed: the component's `SectionHeader` "Environment Variables" is now `AdminSection` (h2),
  so the page has exactly one `<h1>` from the frame and a valid h2/h3 outline; the summary card's duplicate
  `<h2>{egg.name}</h2>` became a "Definition summary" heading.
- Loading and error branches of the *page* now render inside `AdminPageLayout` + `SectionHeader`, so the
  deepest drill-down no longer changes geometry per query state; the error branch gets a retry.
- "0 variables" while loading fixed (count only when `varsQuery.data` exists); access flags state their
  consequence ("Hidden from users", "Users cannot change it") through `Pill tone=` instead of raw
  `emerald-400`/`blue-400`/`slate-500` hues at `text-[10px]`; the dead `onMouseDown` expression and the
  unnamed `<span className="cursor-grab">` grip are gone (grip is `aria-hidden`, real controls are buttons);
  empty `<th>`s named; `text-slate-*`/`bg-white/[0.0x]`/`bg-amber-500/10 text-amber-300` swept to tokens;
  mutation failures surface in the dialog instead of only a toast.

### Forgefile — /admin/forgefile (`components/admin/forgefile-manager.tsx`)
- **Apply is gated behind `useConfirm`** and the editor **starts empty**. It used to boot pre-filled with a
  working `demo` manifest (project + app + postgres db) with a live Apply button, so one mis-click
  materialised resources. "Load example" is now an explicit action, and a Clear action exists.
- The confirm states the real scope and its limit: apply materialises the project, its apps and its database
  and stores a new manifest version, and — because the API has only `POST /forgefile/apply` and no dry-run —
  "the exact changes cannot be listed before they happen". Filed as a central change request.
- **Success copy no longer over-claims**: the panel reads "Apply reported: <slug> · vN · <appliedAt>" and
  states explicitly that `ApplyResult` carries no created/updated/unchanged flag, so re-applying an identical
  document still returns a version. Apps/links render "none reported" rather than empty.
- **Unknown is not zero**: `{manifests.length} manifest(s)` (which printed "0 manifest(s)" during load and
  after failure) is gone; the card title shows the count only when data exists, `isLoading` renders
  `AdminLoadingRows`, and a failed list renders `AdminErrorState`. `listManifests()` returning `[]` for an
  unrecognised envelope is a client-side lie that cannot be fixed from the page — filed as a central change.
- One collapsed `actionError` replaced with four separate panels (list / validate / apply / detail), each with
  its own retry, so a validate failure can no longer be mistaken for an apply failure after dismissal.
- Validate now shows a pending label and a `loading` state; the byte limit is enforced in the UI with the real
  256 KiB number and a stated reason for disabled actions; the detail row has `aria-pressed`, a stated
  re-click-to-close behaviour and a "No rules"/"No default" reading instead of a bare em-dash.
- Copy rewritten from HTTP documentation to operator language: the endpoint/verb/envelope sentences
  ("GET /forgefile/validate?manifest=… also supported", "requires ownership", `FORGEFILE_BASE_DOMAIN`) are
  gone; the schema reference moved into a `<details>`; hand-rolled `<button className="rounded bg-[var(--brand)]
  text-white">` replaced with `Btn`; `border-red-500/25 bg-red-500/[0.09]`/emerald equivalents and
  `text-amber-700` replaced with `ui-alert ui-alert-*`; `bg-surface` vs `bg-[var(--surface-input)]` mixed
  spellings unified on the overlay tokens; the unlabelled `<textarea rows={20}>` has an `aria-label`; the
  `admin-layout` shim is no longer imported (the page uses `admin-ui` directly like the rest of the slice).

### App Storage — /admin/app-mounts (`components/admin/app-mounts-manager.tsx`)
- **Naming (brief item 6): the registry wins.** The `<h1>` said "App Mounts" while the sidebar, breadcrumb and
  command palette said "App Storage"; the hand-passed `title`/`sub` are deleted so the frame renders the
  registry label, and `info={adminPageGuides.appMounts}` carries the disambiguation from Storage Mounts.
  Hand-passed strings were also dropped from Tags, Registries, Forgefile, App Templates, Legacy Templates and
  Service Definitions, so six of the slice's ten headers now derive their language from one source.
- **The aggregate count is no longer a fleet claim built on a partial read**: it renders only once the fan-out
  has resolved and reads "N mount(s) on M of K application(s)", appending "· J could not be read, so this is a
  lower bound" when any app failed, plus a line above the banners saying how many were read.
- `openCreate` no longer prefills `apps[0]?.id`: the application must be chosen, matching the project rule
  against silently resolving an ambiguous target. `New Mount` when there are no applications is disabled with
  a stated reason (`title`), as is the empty-app case in the form.
- The list no longer prints the raw enum (`seed-file`) while the form says "Seed file": rows use
  `MOUNT_TYPE_META[...].label`, and the per-type colour moved off raw hues (`bg-sky-500/15 text-sky-300`) into
  `Pill tone=`; Mode is a `Pill` rather than coloured text with no legend.
- The decisive fields are no longer truncated behind a `title` attribute: target and source render in full with
  `break-all`; the application cell links to `/admin/apps/<id>`; the empty `<th>` is named "Actions"; failed
  validation vs failed create on the two-step save are distinguished in the dialog; field errors use the
  primitive's `ui-field-error` instead of hand-rolled `text-xs text-red-400`; `text-white`,
  `text-[10px] uppercase tracking-wider text-slate-500` and `text-amber-300` swept to tokens; the component
  returns a fragment so the route's `AdminPageLayout` is the only container (same fix as Tags).

## Verified
- `npx tsc --noEmit` filtered to every file in this slice → **0 lines** (no errors, no drift):
  `tags-manager`, `AdminNestsEggs`, `AdminTemplates`, `AdminEggVariables`, `forgefile-manager`,
  `app-mounts-manager`, `app-templates/page`, `registries/page`, `templates/page`,
  `nests/[nestId]/eggs/page`, `nests/[nestId]/eggs/[eggId]/variables/page`, `app-templates-data`.
- `npx eslint` on the same twelve files → **0 errors, 0 warnings**.
- Off-token pattern sweep across the ten edited components/pages: `text-slate-*`, `bg-white/[…]`,
  `red-950`, `border-white/*`, `text-[10px]`, `text-red-400`, `focus:border-red-400`, `text-amber-300`,
  `text-emerald-400` → 0 matches in every file.
- Escape-literal check (`\u2026`, `\u2014` in JSX text): none left in this slice (remaining hits are in
  `AdminFirewall.tsx` / `AdminServers.tsx`, owned by another scope).
- Remaining `title=`/`description=` props in the slice are on `CardHeader`, `EmptyState`, `Modal` and
  `AdminSection` only; no page header passes a `title`/`sub` except the two legitimate detail routes
  (`Eggs: <nest>`, `Egg: <name>`).
- No builds, vitest or dev server run, per the brief.





## Needs central change (do NOT edit these myself)
1. `components/admin/admin-registry.ts:129` — the Legacy Templates row is `href: "/admin/templates"`, but
   `/admin/compatibility-templates` is the URL that renders the same component and is **not registered**, so
   `findAdminPage` misses it: no sidebar row is highlighted and the breadcrumb collapses to the two-crumb
   fallback. Exact edit: change that `href` to `/admin/compatibility-templates` (and rename the row once, so
   "Legacy"/"Compatibility" is not said twice), then make `app/admin/templates/page.tsx` a redirect stub like
   `app/admin/containers/page.tsx`. I could only stop the two routes rendering two different frames.
2. `lang/en.json:222,224` (+ the other seven locales, re-sync with
   `npm --workspace @forge/web run sync:locales`) — `admin.nav.nestsEggs` = "Nests & Eggs" and
   `admin.nav.compatibilityTemplates` = "Compatibility Templates" override the registry labels
   ("Service Definitions", "Legacy Templates") in the sidebar (`admin-shell.tsx` `navLabel` → catalog wins)
   while the breadcrumb and now the `<h1>` come from `admin-registry.ts`. Delete those two label keys (and the
   matching `admin.navDesc.*` at `:302-304`) so one destination has one name. Same question for
   `admin.nav.appMounts` if it disagrees with "App Storage". This is brief item 6 and it cannot be finished
   from page files.
3. `components/admin/admin-page-guides.ts:14` (nests guide) — "Deleting a nest removes its eggs" is the same
   false cascade the confirm dialog carried; `store_nests.go:172-177` refuses. Change to "a nest can only be
   deleted once its eggs have been moved or deleted; existing servers keep running."
4. `components/admin/admin-page-guides.ts:25` (appTemplates guide) — "Templates seed the Create Application
   wizard" overstates it for browser-local templates: the wizard reads `GET /admin/app-templates` and only
   reaches localStorage when that request fails with a `TypeError`.
5. `lib/api/apps.ts:139-151` — `AppTemplate` has no `gitBranch` field, so a branch preset cannot exist; either
   add `gitBranch?: string` (and thread it through `app/admin/apps/new/page.tsx`) or accept that the wizard
   never sets a branch from a template. I deleted the lying input instead. `lib/api/apps.ts:536-546` also
   merges two different stores into one list on `TypeError`; that fallback is why the page now separates the
   API catalog from browser entries.
6. `lib/api/forgefile.ts:55-65` — `listManifests()` returns `[]` for any envelope it does not recognise
   (`:64`), so a contract change reads as "no manifests" and the page cannot tell the difference. Use
   `unwrapList` (`lib/api/http.ts:92-103`), which already throws for exactly this case.
7. `lib/api/forgefile.ts:33-48` — the validate verdict is decided by `err.message.includes("valid")` and
   re-parsing JSON out of an error string; the 422 body should be read from `ApiError.details` instead, or the
   endpoint should answer 200 with `{ valid: false }`.
8. `forge/api/internal/http/handlers_registries.go:173-175` (frozen) — when every node fails, verify answers
   502 with `{ ok: false, results }`, and the web client keeps only `message`/`error` from an error body, so
   the per-node list is lost precisely when it matters. Either return 200 with `ok:false` plus `results`, or
   put `results` under `details` so `ApiError.details` carries it and the UI can show the same table for
   every outcome.
9. Forgefile has no dry-run/plan route (`POST /forgefile/apply` is the only surface), so "what will change"
   cannot be stated before applying. The confirm dialog now says that plainly; the honest fix is a plan
   endpoint returning the resources it would create or update.
10. Product call for App Templates (audit open question 3): the server exposes only
    `protected.Get("/admin/app-templates")` (`handlers_apphosting.go:963-973`) with no write route. Either add
    the write path and delete `lib/app-templates-data.ts`, or accept browser-local presets as a scratchpad that
    only the offline wizard reads. I labelled it honestly rather than choosing.
11. Tags display on other pages: the tag **attach/bulk/usage** flow is now real inside `/admin/tags`, but
    `TagBadge` is still rendered on no other screen, so "tags appear on servers/apps" remains false. Adding the
    chip to server/app/database rows belongs to those scopes' files.
12. `admin-registry.ts` icon vocabulary (`Boxes` = Image Registries + Kubernetes + Incus; `HardDrive` =
    App Storage + Image & Cache Cleanup + Backups; `Layers` = Service Definitions + Projects; `FileText` =
    Forgefile + Host Files) is frozen here. I fixed only the wrong glyphs pages used locally.

## Removed (controls/copies that could not do their job)
- **Tags**: the green "Saved." panel (it could only ever render as a lie — the modal unmounts on success, so it
  reappeared stale in the next create dialog); the "filter the dashboard" / "colour the dashboard" promise in
  the header copy (no other screen renders a tag chip).
- **App Templates**: the `Branch` input (no field to persist it; it was prefilled from the CPU value); the
  local `typeLabel` copy; the `backLabel="Apps"` link to a parent the registry does not declare; the
  `Layers` glyph (Service Definitions' icon); the unreachable "No templates yet" empty state.
- **Legacy Templates**: the `POST /templates` create path from the UI (`createTemplate` import deleted — that
  endpoint accepted 4 of the 12 collected fields and filed everything into a nest literally named "Games");
  the 3-second double-click `setTimeout` delete; the `StatsRow` "0 Templates" KPI tile; the second `<h1>`;
  the duplicate `AdminPageLayout` wrapper; the silent `nests[0]` import target.
- **Image Registries**: the `••••` masked-looking plaintext token field; the unqualified "Registry login
  succeeded" toast for a partial fleet result; "No registries configured" as the rendering of a failed list.
- **Service Definitions / Eggs**: the nest-delete cascade promise; the hover-only `opacity-0` action groups
  (invisible to touch and keyboard while still focusable); `?? 0` egg-count fallbacks.
- **Egg Variables**: fabricated install defaults (`alpine:3.21`, `sh`); the dead
  `onMouseDown={(e) => e.currentTarget.parentElement?.draggable && void 0}` expression; the
  per-`dragover` POST reorder storm (N requests per drag); the second `<h1>`.
- **Forgefile**: the pre-filled working demo manifest as the editor's initial content (Apply was live over it
  on first render); endpoint verbs, response envelopes, the internal `checkKeys` name and
  `FORGEFILE_BASE_DOMAIN` as operator copy; the collapsed single `actionError` that made three different
  failures indistinguishable.
- **App Storage**: the `apps[0]` default application in the create form; the raw-enum type badge with
  off-token hues; truncated target/source paths whose full value existed only in a `title`; the second layout
  container.

## Deferred
- Search, per-nest/per-app filtering, pagination and `AdminPageToolbar`/`FreshnessBadge` adoption across the
  slice's lists (S2 findings). Additive features with no honesty impact; each needs query-key + URL-state
  design that is out of a correctness pass.
- Merging the two egg editors (`AdminNestsEggs.tsx` modal vs the detail page modal) into one shared component
  (audit pattern 11). Both copies are now contract-correct and labelled; the consolidation is a refactor.
- Replacing the desktop-table/mobile-card pair in `AdminEggVariables` with one accessible row implementation —
  both variants now have named controls and a keyboard reorder path.
- Environments in the tag attach picker: `fetchEnvironments(projectId)` is project-scoped, so offering the kind
  here would need a project chooser. Not offered, with a one-line reason on screen.
- Deleting the App Templates create/edit flow outright (pending central-change item 10).
- The `lang/*` and registry-side naming fixes themselves (central-change items 1 and 2).

