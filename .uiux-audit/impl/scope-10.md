# Impl scope 10 — Platform group + shared widgets

Working record. Appended after each page; earlier sections are complete work.

## Done

### Plugins — `/admin/plugins`
- `components/admin/AdminPlugins.tsx`: rewritten. Deleted the `PUT /:id/settings (example)`
  button and `updateSettingsMut` (it overwrote live plugin settings with `{note:"example"}`),
  deleted the Marketplace tab and the Discover tab and every `Install` control (see Removed),
  deleted the "Plugin lifecycle active — var(--brand) themed" banner and the two hand-rolled
  wayfinding bands (HTTP verbs / endpoint paths as copy).
  Loading no longer renders as "No plugin manifests registered" — the card goes through
  `DataState`, so loading → error → empty are decided in one place and a failed read cannot
  look like an empty directory.
  The status pill is now honest: it reads the plugin *service* `state` (joined by id from
  `fetchPluginRuntimeRecords`) instead of the `enabled` column that `/enable` and `/disable`
  never write, which is why every row used to read "Disabled". When that read fails the pill
  says "State not reported" and Enable/Disable render **disabled with a reason** rather than
  posting a blind transition and toasting "State updated".
  Enable/Disable now take an explicit target action instead of an inverted "current state"
  boolean; the icon-only delete button got `ariaLabel`; the search field is labelled; the
  two contradictory counts became one ("N of M match"); tables use `AdminTable`/`AdminTh`
  (accessible name) instead of a raw `<table>`; timestamps use `formatDate` behind `Reading`
  instead of `undefined` rendering blank; `text-slate-*` / `border-white/[0.06]` /
  `bg-red-950/10` / `bg-black/30` / `text-emerald-300` replaced with tokens and
  `AdminErrorState`/`DataState`.
- `lib/api/plugins.ts`: `asList` no longer coerces an unrecognised body into `[]` (a malformed
  read used to become a confident "no plugins"); it throws like the webhooks client does.
  `fetchMarketplacePlugins` → `fetchPluginRuntimeRecords` (the route returns `pluginSvc.List()`,
  i.e. registered rows, not a catalogue) with the old name kept as a deprecated alias so the
  frozen `lib/api.ts` barrel still resolves. `PluginRuntimeRecord` typed to what the service
  actually serialises (`state`, `error`, `source`; no `enabled`). `togglePluginLifecycle`
  signature changed from "current state, inverted internally" to an explicit
  `'enable' | 'disable'` target. `updatePluginSettings` and `fetchDiscoveredPlugins` kept but
  documented with why the UI cannot offer them (no settings read; discover self-registers).
- `app/admin/plugins/page.tsx`: wrapped in `AdminPageLayout` like the other Platform routes
  (the page previously rendered no frame at all and relied on the shell's raw width cap).

### Notifications — `/admin/notifications`
- `app/admin/notifications/page.tsx`: **one `<h1>` for the route** (registry "Notifications")
  rendered by the page frame, with both halves demoted to sections underneath. The route used to
  stack two `SectionHeader`s — "Notifications Engine" and "Notifications" — and disclosed the
  two-backend split only in a code comment no operator sees. The split is now in the subtitle and
  in the `info` disclosure: which endpoint each half reads, what a test actually proves, and that
  stored credentials are returned in cleartext.
- `components/admin/notifications-manager.tsx`: `SectionHeader` → `AdminSection` (h2, with
  `FreshnessBadge` and the actions in its action slot). Channels and the subscription matrix both
  go through `DataState`, so a failed channels read no longer renders "No channels yet" and a
  failed **event-catalogue** read no longer renders a permanent `TableSkeleton` (that tab had no
  error branch at all). The per-row "Events: 0" count is now `—` with the reason
  "Subscriptions could not be read" instead of a fabricated zero — `AggregateTile`'s rule applied
  locally because the count is per row, not an aggregate. The enable/disable affordance was a
  `<Pill>` inside a bare `<button title="Click to disable">`; it is now a real `Btn` with an
  `ariaLabel`, a pending state per row, and no reliance on a `title` for its only explanation.
  `channelScope` no longer invents "Global": `userId`/`orgId` are `omitempty` server-side
  (`store_notification_channels.go:33-35`), so an absent owner and an unreported owner arrive
  identically — the pill now says "Owner not reported" in the `unknown` tone with a one-line
  explanation rather than asserting a platform-wide policy from a missing field.
  Webhook URLs, bot tokens and endpoint URLs render as `type="password"` with an explicit
  reveal toggle; the modal's "Configuration is encrypted at rest" claim is deleted (nothing on the
  page can verify it) and replaced with the claim the server does back — destination validation on
  save and at send (`notifiers.go:26-27`, `validateWebhookURL`). Tables converted to
  `AdminTable`/`AdminTh` (accessible names), all raw tokens gone, delete button named.
- `components/admin/AdminNotifications.tsx`: same treatment — `AdminSection` (no second h1),
  `DataState` for channels **and for the delivery log**, which used to answer
  "No delivery logs yet." for a failed read (the screen an operator opens *after* an alert
  failed to arrive), and `DataState` for the per-channel subscription read, which used to show a
  stale/empty checkbox grid. Malformed headers JSON is now pre-validated with a field-level error
  instead of a raw `SyntaxError` surfacing from inside the mutation. `testMut` gained the
  `onSuccess` it never had (a successful test used to produce no feedback at all) and its message
  says "sent … the control plane accepted the send", not "delivered". The view-switch button lost
  its `RefreshCw` glyph (a refresh icon on a navigation control) and got a real Reload button;
  subscription checkboxes are disabled while a write is in flight and carry `aria-label`s; the
  unnamed raw delete `<button className="text-red-400">` is a `Btn tone="danger"` with
  `ariaLabel` going through `useConfirm`; timestamps use `formatDate`/`Reading` instead of
  `new Date(undefined).toLocaleString()` → "Invalid Date", and `log.error ?? "-"` became an
  explained dash.### Mail — `/admin/mail`
- `components/admin/mail-manager.tsx`: rewritten on the canonical frame
  (`admin-ui` `AdminPageLayout` + `SectionHeader`, no hand-typed title) instead of the
  `admin-layout` shim, which structurally forces a re-typed registry title/description — that is
  why this page read "Mail Settings" while the sidebar says "Mail", and why its description was an
  API contract (`GET/PUT /admin/mail/settings`, "Driver 'log' bypasses SMTP validation") in the
  page subtitle.
  **Cache-key collision fixed**: this page keeps `["panel-mail-settings"]` for
  `/admin/mail/settings` and the Settings Mail tab moved to `["panel-settings-mail"]`; before, two
  different endpoints with different field spellings (`driver` here, `host`+`smtpHost` doubled
  there) filled each other's form and the second-mounted form PUT the other's document back.
  The placeholder sender address is gone: `withFallbacks` seeded
  `mailFromAddress: "noreply@example.com"` / `mailFromName: "Forge"` when the query had not
  answered and Save posted that object, so a load failure followed by Save wrote a fake sender.
  Now fields hydrate from the read only, Save is disabled until a read succeeds, and the masked
  sentinel is echoed back deliberately (`mail.MAIL_MASKED_SECRET`) because that is what the
  handler reads as "keep the stored password".
  Triggers no longer render "No triggers" mid-flight (`DataState`), and the file's existing
  error wording — "the list below is not known to be empty" — is kept as the pattern.
  All eight unlabelled inputs (whose only label was a `placeholder`, and whose accessible name was
  therefore empty until a value was typed) are now real `Input`/`AdminSelect` labels; the
  hand-rolled `<button>` class strings became `Btn`; the driver switch gained an inline warning
  line explaining that `log` means nothing is delivered, which used to be a parenthetical in the
  page description; test results say **queued**, not sent, and the "Queued is not delivered"
  distinction is repeated next to the button. Transport trivia ("multipart alternative",
  "TLS 1.2+", "15s timeout", "MailTriggerService worker") is gone from the two places a first-run
  user reads most. `text-red-200`/`emerald-*`/`bg-black/10`/`tracking-[0.12em]` replaced with
  `AdminErrorState`, `ui-alert-success` and tokens. `FreshnessBadge` added.
### Webhooks — `/admin/webhooks`
- `components/admin/AdminWebhooks.tsx`:
  **Test no longer claims a delivery it did not perform.** `POST /webhooks/:id/test` only inserts
  a delivery row and answers 201 with it (`forge/api/internal/http/handlers_admin.go:1773-1788`);
  the toast used to read "Test delivery fired" without inspecting the response and `testMut` had
  **no `onError` at all**, so a 403/404/503 test produced complete silence while the button
  returned to "Test". It now says *queued*, names the row's state when the response carries one,
  reports failures, invalidates the delivery list, and tracks pending state **per row**
  (`testMut.variables === wh.id`) — previously firing a test on row A greyed out and relabelled
  every other row's button "Testing…".
  The comment that contradicted the code ("No 'test' endpoint exists in the API … so the Test
  button was removed") is gone, since the endpoint exists and the button is rendered.
  Delete now goes through `useConfirm()` and states the blast radius (event subscription count and
  the URL that stops receiving); it previously used a `ModalFooter` **without** `destructive`, so
  the confirm button was the ordinary primary blue. Retry is likewise confirmed and says "queued".
  The deliveries modal no longer nests a `DashHeader` (a second `h2` with the red page eyebrow
  inside a dialog); counts are a plain `role="status"` line that says "Counts unavailable — the
  delivery list could not be read" instead of printing "…" while loading; the loading state is
  `AdminLoadingRows` with `role="status"` rather than a bare string; a failed read says
  "the list below is not known to be empty" instead of falling through to "No deliveries recorded",
  and the empty copy explains that no records is not the same as verified.
  Both tables are `AdminTable` with accessible names. The URL/Events/HTTP/Failure columns were
  `hidden sm:table-cell`/`md`/`lg` — on a narrow viewport the operator silently lost the endpoint
  URL; they are now always present inside the scroll container, and the two truncated cells
  (`wh.url`, `delivery.lastError`) no longer keep their only copy in a `title` attribute.
  Header copy no longer restates the registry and no longer leads with a formatting detail
  ("Discord embed support"); `Globe` (the registry icon for **Domains**, a nav collision) replaced
  by `Webhook`; every icon-only/unnamed control labelled; the fake "Today at 12:00" timestamp in
  the Discord preview removed (it rendered a made-up time in a preview); "No events selected yet,
  so nothing would trigger this webhook" added so an empty event list is not read as configured.
  Create/update now report failure (they had `onSuccess` only). Roughly 29 off-token classes
  (`text-slate-*`, `border-white/[0.06]`, `divide-white/[0.04]`, `red-500/20`, `bg-black/30`,
  `max-w-[120px]`) replaced with tokens and the shared error surface.
### Platform Upgrade — `/admin/upgrade`
- `components/admin/AdminUpgrade.tsx`: rewritten. This is the highest-stakes destructive action in
  the slice, and the old page asserted more than the backend can support.
  **`versions.length || 4` deleted** — the KPI claimed four components when the server had
  measured none, next to an empty card saying it could not determine versions
  (`service.go:127-138` drops a component whose version file is unreadable or invalid). It is now
  `AggregateTile` rendering "N of 4 components are read from local version files" with a `state`,
  so a failed read yields no number at all rather than a floor presented as a total.
  **"up to date" is gone.** `upgradable` is `currentVersion != latestVersion`
  (`service.go:137`) and `latest` comes from an env var or `<component>.latest.version` on this
  host (`service.go:167-177`) — there is no registry or network check anywhere. Pills now say
  "matches recorded target" / "differs from target: `<latest>`", name the source in a one-liner,
  and a `compareVersions` ordering renders a **warn pill "target is older"** when the recorded
  target is a downgrade (previously an arrow pointing forward plus a yellow pill invited exactly
  that click). "Re-check" became "Re-read versions", which is what it does.
  **The consent dialog no longer asserts an unverified backup.** The backup is created *inside*
  execute (`service.go:283-288`), needs `DATABASE_URL` (`:457-459`) and a working `pg_dump`
  (`:470-476`), and neither precondition is checked or surfaced before consent — so the text now
  states that the backup has not been taken or verified, that a backup failure stops the plan
  before any component changes, and that rollback is *attempted* automatically and depends on
  that backup. **The "health is verified after each step" claim is deleted**: `verifyComponentHealth`
  returns success without probing (`service.go:631-635`) and the API itself refuses the claim
  ("component health verification is not implemented in the upgrade service",
  `handlers_upgrade.go:165-166`); the page now says to check health yourself.
  `plan.backupPath` — returned by the server and previously dropped — is rendered as a "Restore
  point" column, with `Reading` explaining "Backup is taken when execution starts" instead of a
  blank. `executeMut`/`cancelMut`/`deleteMut` all gained `onError` (a Cancel that failed mid-flight
  used to show nothing while the row kept its Cancel button) and execute now inspects
  `UpgradeResult.success` rather than assuming it. `plansQ` gained a `refetchInterval` while any
  plan is non-terminal, so an upgrade no longer looks frozen at a percentage after the single
  post-execute invalidation.
  Progress: `totalSteps === 0` no longer renders a bare status pill indistinguishable from a
  one-step plan — it says "Progress not reported by this plan" / "No steps recorded", `pct` is
  clamped to 0-100 (a `progress > totalSteps` reading used to draw a bar wider than its track),
  and the step count is shown as "x of y" rather than a bare percentage.
  `confirmExec.components.join(", ")` — which could crash the consent dialog on a null
  `components` — now shares the guarded `componentsLabel` used by the list. Delete goes through
  `useConfirm` and says what is lost (the control plane's own change history and restore path),
  and the unnamed icon-only delete button is labelled. The hard-coded client `COMPONENTS` list is
  kept but labelled as the server's accepted set (`validComponent`), "Create plan" is disabled
  until a non-`full` scope has a component ticked (an empty list used to be posted), and the
  window-reload retry on `OfflineBanner` became a query refetch that does not discard the selected
  scope. `bg-blue-500/20`/`text-blue-300`/`accent-blue-500`/`bg-blue-400/60` — raw blues colouring
  the *selected upgrade target* and its progress bar — are now `Btn` tones, `accent-[var(--brand)]`
  and `bg-brand`; the plans list is an `AdminTable` with an accessible name; ~30 off-token
  occurrences replaced; `FreshnessBadge` on both reads; `formatDate` fallbacks instead of
  "Invalid Date".

## Removed
- `PUT /:id/settings (example)` — wrote `{note:"example"}` over a live plugin's settings.
- Marketplace tab and its per-card **Install (POST /install)** buttons: the endpoint returns
  the installed rows, so every click is rejected `plugin %q is already installed`
  (`forge/api/internal/services/plugins/plugin.go:185`).
- Discover tab and its Install buttons: `Service.Discover` calls `CreatePlugin` for every
  on-disk manifest during the GET, so nothing in its response is left to install; the payload
  also sent `manifest` as an object where the handler parses a string
  (`handlers_plugins.go:174-179`), which would have failed with 400 even for a new name.
- Settings mutation/editor with no read path — `GET /admin/plugins` does not return `settings`,
  so an editor could only open blind and overwrite.

### Platform Settings — `/admin/settings`
- `components/admin/AdminSettings.tsx`: rewritten around a read-then-edit draft.
  **The default-seeding hazard is gone**: `DEFAULT_GENERAL` (60 hand-typed values) is deleted;
  each tab hydrates a draft from the document it read and Save is disabled while the read is
  pending or failed, with an `AdminErrorState` naming what happened. This matters because
  `PUT /admin/settings`, `PATCH …/mail` and `PATCH …/advanced` each parse the body into one
  whole Go struct (`forge/api/internal/http/handlers_settings.go:44`,
  `handlers_settings_extras.go:41,103`), so what the old page did after a failed load was
  overwrite company name, `require2FA`, `passwordComplexity`, retention windows and strategies
  with defaults — a silent reconfiguration of the panel.
  `Number("")` → `0` is gone: numeric fields hold text, `null` means "the box is empty", and a
  cleared field **blocks Save with a named error** instead of writing `0` into
  session duration, password expiration, API token TTL, backup limit or capacity buffer.
  Payload is rebuilt over the loaded document, so fields this form does not render are sent
  back as read rather than zero-filled.
  Added: dirty tracking with a `useConfirm` guard on tab switch (previously switching tabs
  discarded edits silently and re-seeded defaults); `FreshnessBadge` per tab from
  `dataUpdatedAt`; save results now use `role="status"`/`role="alert"` with distinct tones
  (success and failure used to be the same grey paragraph below the fold); per-field help text
  for the semantic zeros (`backupLimit: 0` = no limit, `passwordExpirationDays: 0` = never) and
  for the fields the API masks (`********`, preserved on write —
  `handlers_settings.go:50-73`).
  The hand-rolled off-token explainer band that printed `PUT /admin/settings` and
  `/admin/security` paths into operator copy is replaced by the frame's `info` disclosure,
  which now states the read-before-write rule, the masked-secret behaviour and the scope
  boundaries. Title/sub no longer restate the registry (the `<h1>` and icon come from it).
  `SelectField`'s invented option values are replaced by `AdminSelect` (label ≠ value, and a
  stored value outside the list renders as its own labelled option instead of vanishing); the
  SMTP `""` option that posted an empty string is gone.
  Mail tab's cache key changed from `["panel-mail-settings"]` to `["panel-settings-mail"]`:
  this tab writes `/admin/settings/mail` while `/admin/mail` writes `/admin/mail/settings`
  (`handlers_mail_settings.go`), different records under different field spellings sharing one
  key, so whichever form mounted second rendered the other's document. The key split stops the
  cross-fill; the two-endpoint question itself is in "Needs central change".
  "Test email sent." now renders the handler's own `sent`/`message` in a real status/alert band
  rather than a hard-coded fallback string; the endpoint is synchronous here
  (`handlers_settings_extras.go:74-78`), unlike `/admin/mail/test`, which queues.
  All `text-slate-*`, `border-white/[0.06]`, `bg-white/[0.015]`, `emerald-*`/`amber-*` classes
  replaced with tokens/`ui-alert`/`AdminErrorState`.

### State components gallery — `/admin/dev/states`
- `app/admin/dev/states/page.tsx`: rebuilt as the reference the rest of the product is
  measured against. It used to demo exactly one of the four state vocabularies
  (`components/shared/states-*`). It now demos three, in the order a reviewer needs:
  **1** `DataState` driving the loading → restricted → error → empty → ready precedence from a
  switchable `SourceState`, so "a failed read rendered as an empty list" is visibly impossible;
  **2** `Reading`/`NotReported`, including a **measured `0`** next to an undefined value so the
  two cannot be confused; **3** `FreshnessBadge` in all six shapes, `SourceRibbon`,
  `MetricSeriesChart` with no points (draws no line) and `PartialFleetNotice`; **4**
  `MetricTile` (kind required, missing value, stale variant) and `AggregateTile` showing
  "N of M reported" as a floor; **5** `StatusIcon`/`StatusPill` across the seven tones plus
  `Pill` legacy colour words resolving through `resolveTone`; **6** `AdminLoadingState`,
  `AdminLoadingRows`, `AdminErrorState`, `EmptyState`, `PermissionDeniedState`; **7** the two
  components both named `PanelCard` side by side, which is what makes the collision visible at
  all; **8** the shared app-workspace set, kept; **9** a "known gaps" section naming
  `NodeTelemetryTable` (would need invented node rows), the route-level
  `app/admin/loading.tsx`/`error.tsx` fourth system and `ui/forge/feedback.tsx` as the fifth.
  Also fixed on this page: title/sub no longer restate the registry ("State Components Demo" →
  registry label), heading order is now h1 → h2 (`AdminSection`) → h3 instead of h1 → h3 with no
  h2, the five hand-rolled `bg-red-600`/`border-amber-500` buttons replaced with `Btn` (the page
  that exists to fix button inconsistency was showing five inconsistent buttons), the off-token
  `text-slate-*`/`bg-white/[0.06]` bands replaced with `border-line`/`bg-overlay-subtle`/`ui-label`,
  the denied-gate children relabelled from the self-referential "Should not render" to "This
  child must NOT appear" (a reviewer can now tell whether it rendered), the toggle buttons given
  `ariaLabel`, the `OfflineBanner` note corrected so it no longer claims to fake connectivity
  (it reads the live `navigator.onLine`; the button only mounts it), and the dead
  `statusIcons`/`lucide-react` block that sat after the default export is folded into the
  `CustomStatus` demo — which now also shows a status **missing from the mapping** instead of
  only the three that are present.
