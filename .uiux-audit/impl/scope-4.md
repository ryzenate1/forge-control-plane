# Impl scope 4 — Deploy group

Working record. Appended after each page.

## Done

### `app/admin/deployments/page.tsx` (rewritten)
- **Collapsed the two tabs into one table.** Both tabs (`fetchJSON("/admin/deployments")` and
  `fetchAllDeployments()` → `lib/api/apps.ts:443`) are the *same* `GET /admin/deployments`; the App tab
  was a second rendering of the same records under fields the endpoint does not send. Verified against
  `forge/api/internal/http/handlers_deployment.go:268` (returns `fiber.Map{"data": all}` from
  `deployment.Service.ListDeployments`) and the `Deployment{}` projection in
  `forge/api/internal/services/deployment/service.go:60-92`. The page now fetches once, through
  `unwrapList`, and types rows as `Deployment` from `lib/api/deployments.ts`.
- **Found and fixed a second, worse defect the audit did not catch:** the route returns the
  `{ data: [...] }` envelope and both queries read it as a bare array, so `Array.isArray(...)` was false
  and the page rendered "No server deployments yet" **for every operator, always** — a successful read
  displayed as an empty list. Same envelope bug in `deployments/history/page.tsx` (fixed there too).
- **Removed the invented App columns.** `revision`, `trigger`, `source`, `commit`, `startedAt`,
  `duration` are not in the payload; the table now shows Server / Image / Strategy / Status / Active
  target / Progress / Created / Completed — all real fields of the Go projection.
- **One status source.** Deleted the page-local `statusConfig` map and the count-strip ternary; both
  the rows and the count chips use `deploymentStatusTone`. `rolled_back` is now `warn` (as
  `lib/api/status.ts:86` says) instead of grey, and `pending` reads `pending` rather than amber.
- **Modal deleted, and with it the `DashHeader`/`InfoCard` frame collision** (brief item 2). The App
  modal repeated the dialog title, the status, App/Source/Trigger/Duration/Started three times each.
  Rows are now `AdminTr` with a real click + keyboard target that navigates to `/admin/deployments/[id]`,
  which already renders these facts once; the `cursor-pointer` row that did nothing is gone.
- **Failed read no longer renders as empty.** `isPending → AdminLoadingRows`, `isError →
  AdminErrorState` with retry, then empty vs. no-matches, in that precedence.
- **Frame + copy**: `AdminPageLayout` + `AdminPageHeader` with **no** `title`/`sub` (registry-derived,
  so the registry glyph now renders beside the `<h1>`), the table sits under a real `AdminSection` `<h2>`
  (previously the section's only "heading" was a `CardHeader` `<div>`), and the `target-ia.md §3`
  architecture dump banner is deleted.
- **Unknown ≠ measured**: count chip renders "Count not loaded" until the query resolves;
  `activeTarget` absent reads "Not reported"; `progressPct` absent reads `—`; `completedAt` absent reads
  `—` rather than `formatDate`'s "Never" ("Completed: Never" was claiming an outcome for a live release).
- **Tokens/a11y/time**: no `text-slate-*`, `bg-white/[0.0x]`, `text-red-400`/`emerald-400`, `text-[10px]`
  left; the two unlabelled raw `<select>`s are now labelled `AdminSelect`s and the search `Input` has a
  real `<label>`; filters persist in the URL (`?q=&status=&strategy=`); filter options are derived from
  the data instead of a hard-coded vocabulary; one time format (`formatDate`);
  `<FreshnessBadge state={sourceState(query, 15_000)} />` in the header `status` slot replaces the
  silent 15 s poll.

### `app/admin/deployments/history/page.tsx` (rewritten)
- **Same envelope bug, worse consequence**: `fetchJSON<DeploymentRecord[]>("/admin/deployment-history")`
  reads `{data:[…]}` as an array, so `records.filter` threw a `TypeError` on every load — the route
  crashed rather than rendering. Now unwrapped through `unwrapList`
  (`GET /admin/deployment-history` → `handlers_deployment_history.go:30`).
- Added the missing `isError` branch (loading → error → empty), `AdminLoadingRows`, retry.
- Deleted the page-local `statusConfig` colour map and the hand-written status icon colour ladder;
  status now comes from `deploymentStatusTone`. `done`/`completed`/`error`/`failed`/`cancelled`/`running`
  are all in the shared table; anything else renders `unknown` instead of defaulting to amber "pending".
- The failure reason existed only as a hover `title` on an icon (`:132-136`); it is now a visible
  **Error** column, with "None reported" when there is no error (rather than a blank cell that could be
  read as "no error recorded" for a status that was never read).
- Frame: `AdminPageLayout` + `AdminPageHeader` + `AdminSection` `<h2>`; `AdminTable`/`AdminTd` replace
  the raw table with `text-[10px] uppercase tracking-widest text-slate-500` headers; the two unlabelled
  `<select>`s are labelled `AdminSelect`/`Input`; status filter options are derived from the data
  (`DeploymentRecord.status` is a free string in Go, so a hard-coded list was a filter that could not match).
- Counts render "Count not loaded" until the query resolves; timestamps only `formatDate`, absent → `—`
  (not `formatDate`'s "Never"); `<FreshnessBadge state={sourceState(q, 10_000)} />` in the header.
- Kept an explicit `title="Deployment History"` — `/admin/deployments/history` is not a registry row and
  would otherwise inherit "Deployments" from the longest-prefix match. Logged under Needs central change.

### `app/admin/deployments/[id]/page.tsx` (rewritten)
- **Failed read ≠ missing record**: `if (!dep) "Deployment not found."` fired for every 500, timeout and
  403. Now `isPending → AdminLoadingState`, `isError → AdminErrorState` with retry and the real message.
- Rollback / Complete / Cancel: **all three now go through `useConfirm`** and say what is affected; they
  are rendered *always*, `disabled` with a reason when the state does not permit it, and the same reason
  is printed as visible text under the header (the `title` alone was mouse-only).
- Rollback gating follows the server, not a guess: `rollback-previous` requires a
  `currentRevisionId` (`revisions.go:224-242`), so the control is disabled with that sentence when it is
  absent instead of firing a request that returns `ErrNoRevisions`.
- Title keeps the resource name but prints the **full id** in mono, wrapping, rather than
  `Deployment: 8f3a2b1c...` — an operator can now read and search it. Single `<h1>` from the frame.
- Mutations now report outcome: Rollback/Complete/Cancel each toast on success *and* failure
  (`complete`/`cancel` previously invalidated silently on success with no confirmation to the operator).
- `Complete` posts via `completeDeployment()` from `lib/api/deployments.ts` instead of a hand-built path.
- Timeline got loading/error/empty branches, an `AdminTable` with `formatDate` timestamps, and
  `unwrapList`. Facts are a real `<dl>` under an `AdminSection` `<h2>`; raw seconds/`toLocaleString`
  dropped; `text-slate-*`, `text-[10px]`, `border-red-700/30 bg-red-900/10` → tokens (`ui-alert-danger`).
- Adds a 10 s poll + `FreshnessBadge` (the detail page had no freshness signal for an object that changes).

### `app/admin/deployments/[id]/revisions/page.tsx` (rewritten)
- **"Rollback to Previous" no-op fixed honestly**: it did client-side index arithmetic
  (`findIndex(active) + 1`) and silently returned when the active revision was the last array element —
  an enabled button that did nothing. It now calls `POST /:id/rollback-previous`
  (`rollbackToPrevious()`), which is where the server decides what "previous" means, and is disabled with
  a visible reason when fewer than two revisions exist.
- The per-row gate `canRollback = !isActive && idx > 0` invented an ordering rule the API does not have
  (`RollbackToRevision` accepts any revision of the deployment, `revisions.go:147-164`); it is now
  `!isActive`, with "Already serving" said in the row. Both rollbacks go through `useConfirm` naming the
  target revision, image and the fact that newer revisions are superseded.
- Per-row busy state (`rollbackMutation.variables === rev.id`) replaces one flag disabling every row.
- Missing success toast added (a successful rollback was silent). Failed list read rendered as
  "No revisions recorded yet"; now loading → error (retry) → empty.
- Frame → `AdminPageLayout`/`AdminPageHeader`/`AdminSection`; bespoke rail timeline → `AdminTable`
  (revision / status / image / commit / config hash / created / action); two unlabelled `<select>`s →
  labelled `AdminSelect`s; `toLocaleString` → `formatDate`; diff rows no longer use `text-red-400` /
  `text-emerald-400` / `text-indigo-300`; the "Rolling back…" band uses `ui-alert ui-alert-warning`.
- Diff "no differences" copy now says the comparison *ran* and found none, distinct from the error
  branch, which was already correct here — preserved.
- Status tones were already from `deploymentStatusTone` (the one shared-module consumer in the slice);
  the `Pill tone="green"` on the active revision chip now goes through the same helper.
- Verified `RevisionDiff.fromRevisionId/toRevisionId` really are revision *numbers*
  (`revisions.go` sets them from `RevisionNumber`), so "Changes from Rev #N" is accurate — kept.

### `app/admin/compose/page.tsx` (rewritten)
- **Deleted the nine-entry `statusConfig`** of `text-emerald-400`/`bg-red-500/10` class strings, the
  bogus `composeStatusTone(status) as "green"|"yellow"|"red"|"blue"|"neutral"` cast (`stackTone`) and
  `healthTone`. One status source (`composeStatusTone`) now colours one pill per row.
- **S1 unknown-as-failed fixed**: `statusConfig[stack.status] || statusConfig.failed` gave every state
  the backend adds next a red failure icon next to a correctly-dashed `unknown` pill.
- **S1 "Health: Healthy" removed.** It was derived from `stack.status`, so a `running` stack with
  crashed services claimed Healthy, and it re-rendered the same string as a second coloured judgement
  on the same row. Nothing on this route reports a health check, so the word is gone (audit open
  question 5 — logged under Needs central change in case the compose API does expose service health).
- **S1 failure reason no longer clipped**: `truncate` with no `title` and no click target → full text in
  a `ui-alert-danger` block on the card (the detail page already showed it in full).
- Rows: `role="button"` `<div>` handling only Enter, with four `<Btn>`s nested inside it behind a
  `stopPropagation` band-aid → the stack **name is a real `<Link>`**, actions sit outside it.
- **Start/Stop/Restart are now visible-and-disabled with a stated reason**
  (`Cannot start while the stack is “deploying”.`) instead of vanishing for
  `awaiting_health`/`deploying`/`updating`/`deleting`/`deleted`.
- One shared `isPending` used to disable every card's buttons while any one stack acted; now
  `actionMutation.variables?.id === stack.id`. The three hand-built `postJSON("/compose/:id/stop")`
  paths are replaced with `start/stop/restartComposeStack` from `lib/api/compose.ts`.
- **Endpoint strings stripped from operator copy**: toast titles
  "Stack deleted … — DELETE /compose/:id?volumes=true" / "Compose imported — POST /compose/import",
  the confirm description, the `GET /compose …` banner and the "Wires listComposeProjects / …" sentence
  are all gone, replaced by the outcome.
- "volumes on delete" moved out of the **filter toolbar** (where it silently changed what every row's
  Delete did) to a labelled line beside the list, and the confirm text now states whether volumes go.
- Import panel relocated from **below the pagination control** to directly under the header, with
  labelled fields, an error branch, and a hint that says what it does *not* do (no validation —
  New stack does).
- Failed list read → `AdminErrorState` with retry (was already partly right; precedence made explicit);
  added the missing `projectsQ` error branch, which used to render "no projects" by omission.
- Frame: registry-derived title/description (hand-passed "Multi-service Compose workloads as
  first-class deployments with lifecycle, logs and rollback." deleted), `AdminSection` `<h2>`s instead
  of `<h3>`s + `CardHeader` `<div>`s, `text-slate-*`/`border-white/[0.0x]`/`bg-sky-500/*`/`text-[11px]`
  → tokens, `AdminSelect`s replace the two unlabelled `<select>`s, search `Input` gets a `<label>`,
  icon-only buttons get per-row `ariaLabel`s ("Delete" eleven times used to be "Delete" eleven times).
- `toLocaleDateString()` → `formatDate`; added a 15 s poll + `FreshnessBadge` (the list never polled
  while the detail did, so a stack sat at "deploying" forever); "Retrying…" strip re-worded to
  "Refreshing…" (a normal poll is not a retry) and tokenised; counts render "Count not loaded" until
  the read lands; project `status` emitted as a bare lowercase string is now a tone pill.
- Local duplicate `interface ComposeStack` deleted in favour of the one in `lib/api/compose.ts`;
  `useToast` missing semicolon and `useConfirm` double semicolon fixed.

### `app/admin/deployments/new/page.tsx` (rewritten)
- **The server picker could never populate**: `fetchJSON<Server[]>("/servers")` reads a paginated
  `{data:[…], meta}` envelope (`handlers_servers.go:278`) as an array, so `Array.isArray` was false and
  the only strategy form in the group had a permanently empty target list. Now uses
  `fetchServers()` from `lib/api/servers.ts`, with loading → error (retry) → empty branches; an empty
  inventory says "no server is registered" rather than offering a blank select.
- No silent target: the select has a placeholder, the Start button is disabled until a server is chosen,
  and the reason ("Select a server to continue" / "Enter the image to deploy" / port range) is rendered
  as a visible chip instead of a dead button.
- **Canary is real, and `blue-green` is hyphenated on the wire** — confirmed
  `service.go:20-24` (`StrategyBlueGreen = "blue-green"`, `StrategyCanary`) and `rollout.go:57-68`
  switches on all four. The payload endpoint `POST /admin/deployments/:serverId/rollout` accepts
  `{strategy, image, healthCheckPath, healthCheckPort, canaryPercent}` (`RolloutRequest`,
  `rollout.go:18-33`), so the mutation contract was already right and is kept. This answers the audit's
  open question 3: canary is implemented server-side; the *list* page was the thing that had dropped it.
- Strategy picker is a real `role="radiogroup"` with `aria-checked`/roving `tabIndex` (it was four
  unlabelled buttons with a `Pill tone="red">Selected` — a status colour on a non-status).
- Canary slider has an associated `<label htmlFor>`; it is a form control the API reads, so it kept its
  place, now with its wire name in the hint.
- Mutation gained an `onError` toast; the failure `Alert` keeps the server message. Success copy no
  longer concatenates ("The " + strategy + " deployment has been created").
- `AdminFormSection` + duplicate `CardHeader` both titled "Deployment Configuration" → replaced by three
  `AdminSection`s with real `<h2>`s (Target / Rollout strategy / Workload and health gate). Tokens:
  `text-slate-*`, `border-white/[0.06]`, `text-red-400`, `bg-[var(--surface-input)]` ladder →
  `border-line`, `bg-overlay-*`, `ui-field-error`, `var(--brand)`.


### `app/admin/compose/[id]/page.tsx` (rewritten)
- **S1 failed read → "Stack not found." fixed**: `isLoading` was the only flag read from
  `getComposeStackStatus`; every 500/403/timeout reported the stack as missing. Now
  `isPending → AdminLoadingState` **under a header with an `<h1>`** (the old loading branch was a bare
  `Loader2` spinner with no heading at all), `isError → AdminErrorState` + retry, and a distinct message
  when the control plane answers with no record.
- **Three of the five tabs were visual no-ops** (Services/Logs rewrote `?tab=` and rendered the same
  everything-always body). The tab strip now genuinely partitions: Overview (identity + recorded limits +
  delete), Services, Logs, Compose file, GitOps. `AdminTabs` replaces the hand-rolled button strip
  (arrow keys, `role="tablist"`, `aria-selected`). Log polling is now `enabled: tab === "logs"`, so a
  hidden 5 s log poll no longer runs.
- **S1 YAML icon buttons fixed**: `title="Edit YAML — PATCH /compose/:id"` on an unlabelled raw button
  whose glyph was a *floppy*, next to a second unlabelled button with no `title` at all, while help text
  told the operator to "click the pencil" — there was no pencil. Now a labelled **Edit** button (`Pencil`)
  and the file is always shown in the Compose-file tab (the eye-toggle tab is redundant once tabs work).
- **S1 save-blind fixed**: the edit modal used to `PATCH` unvalidated. `updateMutation` now runs
  `POST /compose/validate` first and refuses to write an invalid document, surfacing the server's first
  field error (`ComposeValidateResult.errors`) as the failure message. The modal description states this.
- **S1 `stack.memoryMb || "—"`** drew a legitimate `0` as "not reported" in three cells; replaced by
  `numberOrNotReported()`, which prints `0 MB` and reserves "Not reported" for a value that is not a number.
- **The Health chip derived from status is gone here too** (fourth copy of the same claim); status is said
  once, as the header pill. The header no longer puts the raw status in `description` *and* a pill in
  `action` — `description` now names the node and source type.
- **`composeStatusTone(...) as "green"|"yellow"|"red"|"blue"|"neutral"`** bogus casts deleted (two sites);
  the `"blue"` in that union is produced by nothing in the shared module.
- **Endpoint dumps stripped from visible copy**: "GitOps — 9 endpoints wired", the 11-route `<p>`,
  "Compose YAML — PATCH /compose/:id", "Wires updateComposeStack — PATCH /compose/:id with composeYaml",
  "Drift — GET /compose/git/:id/drift", the `GET /compose/git/:id/last-webhook` suffix and the four toast
  titles carrying `POST /compose/git/:id/…`. Replaced by outcomes; the update-check payload is rendered in
  the panel rather than JSON-stringified into a toast that scrolls away.
- **Drift is now a verdict, not a payload**: `DriftCheckResult.hasDrift`
  (`compose/gitops.go:77-85`) drives a "Drifted / Not drifted" line naming the deployed and repository
  SHAs, with the `servicesDiff` rows in a table; the raw response is behind an explicit
  show/hide toggle (kept, because the operator may still need it) and the panel distinguishes
  checking / failed / answered.
- Services table: the `bg-current` dot that was always the pill's own colour is gone, and the image is no
  longer printed twice with two different truncations (`truncate` + `slice(0,24)`). `svc.status || svc.state`
  stays as one reading labelled "Not reported" when neither was populated.
- Logs: `select`s are labelled `AdminSelect`s; a failed log read used to render "No logs available." —
  now loading / error+retry / and an explicit "the request completed and returned nothing".
  The `_all` fallback no longer silently claims "no logs" when a service filter is selected.
- Delete: the `volumes` flag moved out of the header action cluster (where it sat, labelled only
  "volumes", between Redeploy and Delete) into a **Remove this stack** section adjacent to the button it
  modifies, with its consequence written out; the confirm text says which way it is set. Endpoint suffix
  removed from the confirm.
- Redeploy: was unconditionally enabled and shared `RotateCcw` with Restart 40 px away; now `Rocket`,
  disabled for `deploying`/`deleting`/`deleted` with the state named, and confirmed. All four lifecycle
  controls are visible-and-disabled with a reason, plus one visible line explaining the gate (they used
  to be hidden per-status). Git controls are disabled when the stack is not recorded as git-backed, with
  the recorded `sourceType` quoted — and `isGitBacked` deliberately reads only `sourceType`, the one git
  signal the stored stack carries.
- GitOps Rollback / Pull-and-redeploy / Redeploy-from-git all now go through `useConfirm` naming what moves
  (Rollback in particular redeploys the stack; it was a one-click ghost button).
- Time: three `toLocaleString()` sites → `formatDate`; `stack.environmentId` read directly instead of two
  `(stack as unknown as {environmentId?})` casts (the field is on `ComposeStack`); `reservationId` surfaced
  or stated as "No reservation recorded".
- Tokens: `text-slate-*`, `border-white/[0.06]`, `bg-red-500/10 text-red-400`, `text-amber-400`,
  `bg-[var(--surface-input)]` + `color-mix` focus rings → `ui-alert-*`, `border-line`, `bg-overlay-*`,
  `ui-input`, `text-text-*`; `<dl>`/`<dt>`/`<dd>` used for the fact rows; identity facts are an
  `AdminSection` `<h2>`, not a `CardHeader` `<div>`.
- **Facts vs limits distinguished**: the limits card says explicitly "these are the limits stored for the
  stack, not live usage", because nothing on this route measures use.

## Verified
- pending final `npx tsc --noEmit` pass.

## Needs central change (do NOT edit these myself)
- (none yet)

## Removed
- `deployments/page.tsx`: the "App Deployments" tab, `DeploymentDetailModal`, `DashHeader` + `InfoCard`
  imports, the page-local `statusConfig`, the `target-ia.md §3` explainer band, `DeployStatusBadge` usage.

## Deferred
- (none yet)
