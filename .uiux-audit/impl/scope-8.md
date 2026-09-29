# Impl scope 8 — Operations A (operations, backups, engines, migrations, cron, docker events)

Working record — appended after each page.

## Done

### lib/api/admin-backups.ts (client for /admin/backups/*)
- Field names corrected against the Go structs (`progress` → `progressPercentage`, which the wire actually sends; the old name made every progress cell read `NaN%`).
- `BackupArtifact` extended with the fields the API already sends: `verificationAttempts`, `lastVerifiedAt`, `lockReason`, `expiresAt`, `hashAlgorithm`, `sourceServerId/AppId/DatabaseId/VolumeId`, `uploadedAt`.
- `BackupRestore` extended with `artifactId` (the provenance link that was being dropped), `errorMessage`, `verificationStatus`, `progressPercentage`, byte counters.
- New `CreateRestoreInput`/`RestoreOptions` mirroring `CreateRestoreRequest` — the API requires exactly one target (`restore.go:218-223`) and the old form sent none.
- New `verifyBackupArtifact()` for the existing but unused `POST /admin/backups/artifacts/:id/verify`.
- New `artifactVerification()` (verified / check-failed / never-checked three-way verdict) and `artifactSizeBytes()` (null for a size never measured). Documented why `attempts > 0 && !isVerified` is evidence of a *failed* check (`artifact.go:561-568`) and why `fileSize: 0` before upload is not "empty backup".

### app/admin/backups/engines/page.tsx
- Restic snapshot size and file count: local `formatBytes` deleted; `engineSizeCell`/`engineFilesCell` render **"Not reported by restic"** for non-Kopia repositories, with a comment naming the server lines that prove it (`service.go:1254-1262` vs `:1371-1372`). Kopia keeps real numbers, including a measured `0 B`.
- The Snapshot-created toast no longer interpolates a size the engine never reported.
- Un-initialised repository: the snapshots tab now has its own "Repository needs initialisation" state (instead of "No snapshots yet — run Snapshot now"), an inline **Initialise repository** button wired to the real init endpoint, and `Snapshot now` disabled with a visible one-line reason (no repo / not initialised / no paths). Refresh is disabled in the same case.
- Restore dialog now states the snapshot's verification verdict, size verdict, engine and repository, warns in amber when nothing has verified it, and requires a `useConfirm()` "Restore an unverified snapshot?" step before posting.
- Restore mutation no longer toasts "Restore failed" for work that merely hasn't finished: `failed` → error, `completed` → success, anything else → "Restore started", and the tab switches to the job list.
- Provenance and cross-link: `repo.artifactId` is now rendered as a link into Backups; both pages link each other and each says in one sentence that the other system's records are not listed here.
- Tabs renamed "Engine snapshots" / "Engine restores" so the word "Restores" no longer means two unrelated job tables in one sidebar group.
- Header: `title`/`description` deleted (registry derives them, which restores the "verification" the paraphrase dropped), `info={adminPageGuides.backupEngines}`, `status={<FreshnessBadge …>}` for the active tab's query so the on-demand snapshots tab cannot look as fresh as the 30s-polling ones.
- Node/Server selection: free-text UUID inputs replaced with selects over the node and server inventory (`useNodesQuery`/`useServersQuery`), preserving an unknown existing id as an explicit "not in the node list" option.
- Snapshots list got a labelled filter; truncated location/last-error/target-path values now wrap and break instead of living only in a `title` tooltip.
- Icon-only remove button got `ariaLabel`; sibling icon buttons labelled; every table has a `label`; progress bar only drawn for `running` jobs (clamped), with words for pending/finished; `startedAt`/`completedAt` use the shared `formatDate` with "Not started"/"Not finished" instead of a bare dash.
- Token pass: `slate-*`, `border-white/20`, `bg-white/[0.06]`, `bg-blue-500/60`, `text-amber-200`, `text-red-200` → `text-text*`, `border-line*`, `bg-overlay-*`, `bg-info`, `text-warn`/`border-warn-line`/`bg-warn-subtle`, `text-danger`/`border-danger-line`/`bg-danger-subtle`; local `statusTone` deleted in favour of `deploymentStatusTone`.

### app/admin/backups/page.tsx (classic backup pipeline)
- KPI tiles: replaced the four hand-rolled `Card` + `|| 0` tiles with `MetricTile` fed by `sourceState(systemStatusQuery, 30s)`. All eight figures now render a loading skeleton, then a value, then `—` with "The status read failed" — never `0`.
- Artifact size: local `formatBytes` deleted; shared one from `lib/utils`, with `sizeCell()` rendering "Not measured" for an unmeasured artifact.
- "Verified Yes/No" column replaced by a three-way Integrity column: `Verified <timestamp>` / `Check failed (N attempts)` / `Not checked` (unknown tone), so a checked-and-bad backup no longer looks identical to an unchecked one.
- Restore: now gated and confirmed. The menu item is disabled with a visible one-line reason when the last integrity check failed; the dialog states the real verdict, size, the fact that the control plane re-hashes before writing (`restore.go:378-388`), collects the mandatory target (prefilled from the artifact's own source id), and maps its checkboxes onto the fields the server reads (`restoreOptions.overwriteExisting`, `createBackupBeforeRestore`) with consequence text for each. Submitting runs `useConfirm()` stating what is overwritten and whether rollback exists.
- The previous Restore form posted `overwrite` and no target, so every restore it started was rejected by the API with 422 — that is now fixed by the payload correction, not by decoration.
- Progress: `ProgressCell` renders "Not started" for pending, a clamped % + phase + `x of y` byte denominator for running, and "Stopped by failure"/"Cancelled" instead of a frozen percent.
- Job/restore `errorMessage`, config `lastStatus`/`lastError`, `lockReason` surfaced; `bytesProcessed`/`totalBytes` used as the denominator.
- Header: `title`/`description` deleted (registry derives them), `info={adminPageGuides.backups}`, `status={<FreshnessBadge …>}` over the worst of all six queries.
- Aggregate error banner moved above the tabs (it used to render only on Overview); every tab now has loading → error → empty branches, and empty states distinguish "no records" from "no match for this search".
- Vocabulary unified on "backup policy" (was configurations/policies/Configuration in five places).
- Search is cleared on tab change (one shared string had been filtering all four lists at once); every `<Input label="">` got a real label; every `AdminTable` got a `label`.
- Download reports the byte count it actually received; verify/lock/unlock/cancel/report-deleted all report success or failure.
- Duplicate "Quick Actions" Refresh control deleted (the header owns refresh); "Storage Providers" list given loading/error/empty branches.
- Token pass: `text-slate-*`, `border-white/10`, `divide-white/[0.04]`, `bg-[var(--surface-input)]` selects → `text-text*`/`border-line*`/`divide-line`/`AdminSelect`; status colour comes from `deploymentStatusTone`/`Pill tone=`, local `getStatusTone` deleted.

### app/admin/cron-jobs/page.tsx
- Failure ≠ empty: the job list now destructures the query and renders `AdminErrorState` with retry. Previously a 401/500 rendered "No cron jobs configured. Create one to automate tasks." Same fix in `ExecutionLog`, which turned a failed history read into "No executions yet".
- Real schedule validation: `parseCronField`/`validateCronSchedule` check all five fields against the ranges robfig/cron uses (minute 0-59, hour 0-23, dom 1-31, month 1-12, dow 0-6, names, ranges, steps, lists, backwards ranges, zero steps) plus the control plane's own rule (`handlers_cronjob.go:19-21`) that `* *` is rejected. The old check counted fields.
- Next-run preview: `computeNextRuns` evaluates the expression (with the standard dom/dow OR rule) and lists the next three matches, labelled as computed in the browser's time zone. The unschedulable-claim copy "Runs … on a custom schedule" is gone.
- Row state: an enabled job with no `nextRun` now says which case it is — "No next run — the scheduler has not registered this job" versus "Not scheduled — the expression is invalid" — instead of rendering nothing at all. Disabled jobs say "Disabled — no next run".
- Last run: derived from the newest execution already fetched and shown in the expanded history header ("Last run … finished … (exit N)"), so no fabricated value. `exitCode`, `finishedAt` and `durationMs` are now rendered; `exit code not reported` where absent.
- Row actions: icon-only buttons got `aria-label`s; Edit moved off the `RefreshCw` glyph onto `Pencil`; the clickable row `div` became a real `button` with `aria-expanded`/`aria-controls`; `delete`/`toggle`/`trigger` all toast success with what happened, and "Trigger now" invalidates the job list as well as execution history (it previously invalidated a key that matched nothing unless a row was open).
- Target selection: `targetType`/`targetId` free-text UUIDs replaced with a type select plus a node/server picker fed from the same inventory the rest of the admin reads, with validation that a target type needs an ID.
- Loading uses `AdminLoadingRows`; the `&nbsp;` filler form field is deleted; headings no longer bake counts in before data exists (count now sits in the card action and only renders post-load); header gets `info={adminPageGuides.cronJobs}` and a real `FreshnessBadge` for both the list and the history panel.
- Token pass: the hand-rolled `inputBase` string replaced by `ui-input`, `slate-*`/`bg-black/30`/`bg-red-900/20`/`bg-white/10`/`bg-emerald-500` → `text-text*`, `border-line*`, `bg-overlay-*`, `bg-ok`, `bg-danger-*`; all tables/lists use `divide-line`.


### components/admin/docker-events-feed.tsx
- The pulsing green `live 5s` badge is gone. Connection state is now derived from `sourceState(query, 5s)` and reads one of: `Connecting — first read pending`, `Polling every 5s`, `Stale — last poll did not return`, `Offline — the feed is not reading`, `Permission restricted`, `Paused — not polling`, with the dot colour following the same verdict. It sits beside a real `FreshnessBadge`, and the error banner can no longer appear under a "live" claim.
- `${visible.length} shown · ${total ?? 0} stored` replaced with three separately-labelled scopes: "N shown of M loaded" (client filter) and "K matched on the server", with "server count not read" before the first response instead of `0`.
- Node liveness added: nodes whose inventory says they are not active are named ("2 of 5 nodes are not active (helix-2, atlas) and reporting no events"), and a failed node inventory is reported as such rather than silently hiding the fact that the page cannot attribute silence to anyone.
- Hardcoded "retained 30 days" / "prunes anything older than 30 days" claims removed — the value lives in `handlers_docker_events.go` and is not on the wire. The copy now states what the page actually knows (the newest 200 matching rows) and that retention is enforced server-side and not reported here.
- Filters (node, event type, search, paused) are mirrored into the URL query string via `history.replaceState` and restored after mount, so a filtered feed is shareable and survives a reload without an SSR/hydration mismatch.
- Loading skeleton is now `role="status"` with a label (the rows themselves stay `aria-hidden`); arbitrary type sizes (`text-[10px]`, `text-[11px]`) replaced with the named scale `text-eyebrow`/`text-meta`.
- Dead `import {  } from "@/lib/api";` deleted; doc comment corrected to say the feed polls.

### app/admin/docker-events/page.tsx
- Hand-passed `title` deleted (registry derives it); `sub` overridden to say "polled … every 5 seconds" instead of the registry's "streamed"; `info={adminPageGuides.dockerEvents}` wired.

## Verified

- In progress; final `npx tsc --noEmit` filtered to scope-8 files recorded at the end of the pass.

## Needs central change (do NOT edit these yourself)

## Removed

## Deferred
