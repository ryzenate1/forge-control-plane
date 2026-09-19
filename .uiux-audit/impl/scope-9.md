# Impl scope 9 — Operations B (reconciliation, orphans, drain, cleanup, failover, procedures, scheduler, autoscalers, affinity)

Work is appended page by page; this file is the complete record up to the moment of writing.

## Done

### Reconciliation — `/admin/reconciliation`
- `lib/api/reconciliation.ts`: documented that `ReconcileSummary` is a count over `reconcile_plans`
  with **no** `lastRunAt`/`hasScanned` (Go is frozen, so the ambiguity is resolved in the UI); added
  `readReconcileSummary` (rejects a short/non-numeric payload instead of yielding `undefined` fields)
  and `latestReconcileActivityAt(plans, events)` — the only honest evidence that a run ever happened;
  `fetchReconcileSummary` now **throws** when counters are unreadable rather than resolving blank data.
- `components/admin/AdminReconciliation.tsx`:
  - S1 never-run-as-clean: KPIs are gated on `evidenceSettled` + `lastRecordedAt`. With no plan or event
    ever recorded, Pending/Failed/Drifts/Unresolved render `—` in the `unknown` tone plus a warning strip
    ("zero counters mean nothing has been checked, not that the fleet is in sync"). Previously an all-zero
    board looked calm. Followed the `AdminCleanup.tsx:58` pattern.
  - S1 silent staleness: `status={<FreshnessBadge state={sourceState(summary, 15_000)} />}` — failed 15 s
    background refetches now show `Stale`/`Unavailable` instead of cached numbers.
  - S2 deleted the hardcoded debug strip (`…via store_reconcile.go:13…`, `/admin/migrations` name-dropping);
    the structured `adminPageGuides.reconciliation` disclosure is now the only explainer, and the
    hand-passed `title`/`description` that restated `admin-registry.ts:194` are gone so the frame derives
    title + glyph from the registry.
  - S3 confirm copy fixed: `POST /plans/:id/confirm` is `ConfirmAndExecute`
    (`handlers_reconcile.go:50`), so the dialog no longer says "queued for execution".
  - S3 a11y: scope `<select>` gets `aria-label` + `htmlFor` label; icon-only refresh `Btn` gets `ariaLabel`;
    disclosure button gets `aria-expanded`; `h4` under `h2` replaced by `h3` (no heading jump).
  - S3 tokens: KPI tiles now use `AdminStatCard` (delegates to `ForgeMetric`, absent → `—`), raw
    `text-amber-400/red-400/yellow-400/blue-400`, `bg-white/[0.015]`, `text-[11px]` and the `h-24` skeleton
    blocks replaced with token/`ui-alert` classes.
  - `retry: false` on all three queries so a failed read surfaces instead of silently retrying into a blank;
    event/plan timestamps switched to shared `formatDate`.

### Orphaned Resources — `/admin/orphans`
- `components/admin/AdminOrphans.tsx`:
  - S1 relabelled honestly **in the page copy** (registry is frozen): header `sub` and a standing
    `ui-alert-warning` strip state that this is a failed-deletion queue, that nothing scans for orphans, and
    that an empty queue ≠ no orphans. The empty state says the same. `lib/api/drain.ts:60`
    (`fetchEvacuationOrphanCandidates` returning a hardcoded `[]`) is cited in the file comment as proof no
    discovery path exists.
  - A failed read now renders `AdminErrorState` with the explicit line "the queue state is unknown — this is
    not an empty queue"; loading uses `AdminLoadingState` (was an ad-hoc string) and the hand-rolled red error
    `div` is gone.
  - S2 frame: `max-w-[1280px]` custom container → canonical `AdminPageLayout` + `SectionHeader`.
  - S2 freshness: `FreshnessBadge` bound to `sourceState(q)`.
  - S3 a11y: `AdminSelect label=""` → `label="Queue status"`; refresh button gets `ariaLabel`.
  - S3 toast: `sonner` → the slice's `useToast` (one toast vocabulary across Operations B).
  - Copy: dropped the duplicated `title` (registry-derived now) and the DB table names
    (`server_orphan_remediations`, `audit_events`, `target_type=…`) from operator-facing text;
    timestamps via `formatDate` with explicit "not reported" fallbacks instead of an empty string.

## Verified
- `npx tsc --noEmit` filtered to my files: **clean** for
  `AdminReconciliation.tsx`, `AdminOrphans.tsx`, `AdminCleanup.tsx`, `AdminDrain.tsx`,
  `lib/api/reconciliation.ts`, `lib/api/cleanup.ts`. Two errors found and fixed on the way
  (`TS1355` misapplied `as const` in the Drain `StatsRow` items, then a `TS2322` union-inference
  failure on the same array, resolved by annotating it with the `StatsRow` item shape and
  importing `AdminTone` / `LucideIcon` as types).

### Node Drain — `/admin/drain`
- `components/admin/AdminDrain.tsx`:
  - **S1 cancelled-as-complete**: `stepState()` (which returned `"done"` for every step when
    `overallStatus === "cancelled"`) is deleted. `ProgressStepper` now renders each
    `DrainProgressStep.state` from the ledger via `stepVisualFor()`, with six distinct visuals —
    `done | active | pending | failed | not-run | unknown`. In a terminal drain
    (drained/cancelled/failed) a still-`pending` step draws **not-run** (dashed, `Ban` glyph), so an
    aborted evacuation can no longer show four green checks. A state string this UI doesn't know
    falls to `unknown`, never to `done`.
  - S1 unknown-as-zero: `{s.progress?.remaining ?? 0} remaining / of {…total ?? 0} workloads` →
    unreported counts render "Progress not reported" with each side labelled unknown individually.
    Likewise `started/updated` go through `formatDate(…, "not reported")`, `planId` says
    "no evacuation plan recorded", and the four `StatsRow` counters render `—` until `ledgerReady`.
  - S2 node picker: `nodesQ` now has loading and error branches (`AdminLoadingState` /
    `AdminErrorState` stating "an empty picker here is not an empty fleet"); the help line explains
    ineligibility separately from emptiness.
  - S2 confirmation: **Cancel** goes through `useConfirm` and spells out that already-migrated
    workloads are not moved back and the node may stay out of traffic, quoting the remaining count
    when the ledger reported one. Begin-drain keeps its `AdminConfirmDialog`. Both mutations toast
    the real outcome instead of staying silent.
  - S3 freshness: `status={<FreshnessBadge state={sourceState(statesQ, 5_000)} />}` matching the
    5 s active-drain poll; `retry: false` on both queries; `OfflineBanner` retries the actual reads
    instead of `window.location.reload()`.
  - Tokens: raw `emerald/amber/slate/white-alpha`, `text-[10px]`/`text-[11px]` and the hardcoded
    `rgba(245,158,11,0.08)` shadow replaced with `ok-line/warn-line/danger-line/unknown-line`,
    `overlay-subtle`, `text-eyebrow`, `text-meta`; the `title`-only step tooltip became visible
    per-step text (state word + backend `detail`), so nothing is trapped in a tooltip.
  - Copy: dropped `title="Node Drain"` (registry-derived); kept the longer `description` because it
    carries the ledger-vs-orchestration distinction the registry line does not.

### Cleanup — `/admin/cleanup`
- `lib/api/cleanup.ts`: `unwrapInfo` no longer defaults an unrecognised payload to
  `{0, 0}` — it requires both counters to be finite numbers and **throws** otherwise, with the stale
  TODO comment replaced by a note recording that the Go handler returns a bare
  `cleanupsvc.CleanupInfo`. A contract change now surfaces as an error state, not a "clean" verdict.
- `components/admin/AdminCleanup.tsx` (kept as the slice's reference pattern, then tightened):
  - `?? 0` on `staleReservations`/`orphanedAllocations` removed; counts flow through
    `readout()` → `—` when unreported, `measured` gates the Verdict pill, and the verdict now reads
    "nothing stale" (measured) vs "not measured" (`tone="unknown"`) vs "N to clean".
  - Both confirm dialogs state a real blast radius, and when counts are unreported they say so
    ("the blast radius of this run is unknown") instead of interpolating zeros. Run is disabled while
    inspect is loading or failed.
  - Inspect-failure copy: "This is not a clean platform — it is an unreadable one."
  - Tokens: `amber-500/20`, `red-500/10`, `text-amber-200/70`, `divide-white/[0.04]`,
    `text-[11px] uppercase tracking-widest` → `ui-alert`/`warn-*`/`danger-*`/`divide-line`/`t-eyebrow`.
  - S3 developer leak removed: "runs every 5 min via `Service.Start(ctx)`" → "runs on a 5 minute
    schedule in the background"; `EventReservationExpired` → "records a reservation expired event".
  - Header `sub` now disambiguates against Image & Cache Cleanup and the orphans queue (the naming
    triangle), and the closing note names all three surfaces.

### Failover — `/admin/failover`
- `app/admin/failover/page.tsx`:
  - **S1 mislabelled destructive write**: the card is "Report a server crash", carries a
    `Live write` pill and a one-line statement that this runs the real recovery path with no dry
    run (it POSTs `/admin/failover/crash/:serverId/:nodeId` → `HandleServerCrash`,
    `handlers_failover.go:82`). The button reads "Report crash & run failover" and is gated on both
    IDs, then goes through `useConfirm` with the actual blast radius: the enabled policies matching
    that node (action, threshold, window, cooldown) or the explicit "no enabled policy matches, so
    nothing will be recovered".
  - **S1 outcome honesty**: the hardcoded green "Crash handled" panel is gone. `describeOutcome()`
    reads `failover.Event` and keeps the three cases distinct — `null` data (accepted but
    *no action taken*: unmatched policy or cooldown) renders in the `unknown` tone, `failed` in red,
    `completed` in green, and in-flight statuses (`evacuating`/`restarting`/`notified`) in
    warn/info. `OutcomePanel` prints node/server/eventType/status/timestamp instead of
    `Object.entries(...).slice(0,6)` of the envelope, which used to render `data → [object]`.
  - S2 "Record failure" is now confirmed (naming the matched policy's threshold, or stating that no
    policy matches so nothing will trigger) and reports its outcome in the metrics card rather than
    only invalidating the cache.
  - S2 node targeting: policy create/edit and the crash node field use `NodeSelect` (no default
    selection) instead of a free-text Node ID a typo could silently orphan; the crash card live-
    reports how many enabled policies match the chosen node.
  - S3: hand-rolled `<table>` → `AdminTable`/`AdminTh`/`AdminTd`; unassociated `<label>`+`<select>` →
    `AdminSelect`; icon-only delete `Btn` gets `ariaLabel`; raw slate/emerald/red/white-alpha and
    `text-[10px] uppercase` swept to tokens; `retry: false` + `FreshnessBadge` over
    `worstSourceState([policies, metrics])`; counts render `—` while policies are loading or failed.
  - Copy: dropped the `title`/`description` that restated `admin-registry.ts:198`; the registry
    glyph (LifeBuoy) is now rendered by the frame automatically.
  - Metrics card states the counters are process-wide and reset on API restart, so a low number is
    not read as "no failures ever".

## In progress / not yet started
- Procedures, Scheduler, Autoscaler list + detail, Node Autoscaling, Placement Affinity

### Procedures — `/admin/procedures`
- `components/admin/procedures-manager.tsx`:
  - **S1 "No logs."**: `handleViewExec` used to swallow every `listStepLogs` failure
    (`// ignore per-step log failures`) and the box printed "No logs." Logs are now a
    `LogRead { rows, error, loaded }` per step, so the panel renders four distinct states:
    not fetched yet (dashed, "Logs not fetched for this step yet"), read-succeeded-with-
    zero-lines ("The log read succeeded and returned no lines"), failed (`AdminErrorState`
    + retry, "This is not 'no logs'"), and populated. The retry button is per-step and
    calls the real endpoint.
  - **S2 failed-vs-skipped now expressible**: `executionVerdict()` computes
    "succeeded with N failed steps" (yellow) when the runner reports `succeeded` under
    `continueOnFailure` (`procedure/service.go:300-301`) and "…· N steps never ran" when
    a terminal execution left `pending` steps behind (`:325-438` never writes `skipped`),
    with an explanatory strip. Step pills render "never ran" in the `unknown` tone instead
    of the yellow `pending`.
  - S2 execution staleness: while any execution is non-terminal the panel polls every 5 s
    and says so; `Cancel` is **disabled on terminal rows** (it was an accepted no-op write
    on a succeeded execution) with a `title` reason, and both Execute and Cancel now go
    through `useConfirm` with what will run / what will not be rolled back.
  - S2 no edit path: `api.updateProcedure` existed and was never called. `Enable`/`Disable`
    on the detail card now round-trips the full definition through `PUT /procedures/:id`
    (`handlers_procedures.go:54`), which the handler accepts as a whole-record replace.
  - Executions list got `AdminLoadingState`/`AdminErrorState` (`loadExecutions` previously
    set only a page-level error and the card still read "No executions" after a failure).
  - Copy: `title`/`description` deleted (frame derives from `admin-registry.ts:199`);
    "queued" is kept and explained — unlike Reconciliation, `POST /:id/execute` really is
    asynchronous (202 + goroutine).
  - Tokens/a11y: action `<select>` → labelled `AdminSelect`; `✕` config-removal button gets
    `ariaLabel`; step config values render `JSON.stringify` instead of `"[object]"`;
    `text-[11px]`, `border-white/10`, the emerald success banner and `text-red-300` →
    `t-eyebrow`, native checkbox, `ui-alert-success`, `text-danger`; timestamps via
    `formatDate`; attempt counts guard against unreported numbers.

### Scheduler — `/admin/scheduler`
- `app/admin/scheduler/page.tsx`:
  - **S1 capability lie**: the hardcoded `docker | k3s | nomad` `<select>` is gone. Runtime
    pin options are generated from `/admin/scheduler/backends` (the list the same tab already
    fetches); the control is **disabled with a reason** while that read is loading or failed,
    and the button additionally requires the chosen type to be in the reported set. Empty
    backend list → `EmptyState`, not an empty option list.
  - **S1 metric forge**: "Ingest predictive metrics" is labelled `Test hook · writes`, and the
    form no longer defaults every field to 0. `metricsForm` is now string-backed and
    empty-by-default; `metricReadiness` refuses to send while any of the six values is blank or
    non-numeric and names them ("would be written as a measured 0"), because
    `scheduler.ResourceMetric` (`predictive.go:23-31`) has no `omitempty` and cannot distinguish
    omitted from zero. Sending goes through `useConfirm` stating it becomes the node's latest
    measurement and shifts fleet-wide placement. A node must be named; nothing is auto-picked.
  - S2: all five ad-hoc loading strings → `AdminLoadingState`; the backends card got
    `AdminErrorState` + retry and explains that pinning is disabled as a result.
  - S2: `weight: {rule.weight ?? 0}` → `not reported` (creation defaults to 1, so an absent
    weight is unknown, not zero).
  - S3: `ScoreVerdict` no longer prints an unconditional green "scored node" pill — a score
    with confidence ≤ 0 or a non-positive total renders `unknown` with "scored with no
    confidence"/"score is not above zero" plus a strip saying to treat the node as unranked;
    ordinary scores are `neutral`, since a ranking is not a health claim.
  - Copy: `title` deleted; `description` overridden to what the page actually does
    (the registry line promises "strategies and explanations" that live elsewhere).
  - Tokens/a11y: hand-rolled constraints `<table>` → `AdminTable`; both modal `<select>`s →
    labelled `AdminSelect` with self-describing options; every icon-only delete `Btn` gets
    `ariaLabel`; `bg-emerald-500/sky-500/amber-500/red-500` bars → `bg-ok/info/warn/danger`;
    `text-slate-*`, `border-white/*`, `text-[10px]` swept (verified: zero matches remain).
    Constraint-delete confirm now names the rule being removed, and the full-set rewrite
    race is disclosed in the card copy.

### Workload Autoscaling — `/admin/autoscaler` + `/admin/autoscaler/policy/[id]`
- `app/admin/autoscaler/page.tsx`:
  - `policyDefects()` gates `Save` on min ≤ max (memory and CPU, with equality flagged as no
    headroom), `scaleUp > scaleDown` (hysteresis), threshold range and cooldown — previously
    gated only on a non-empty server ID, so an inverted policy persisted and silently never
    fired. Rows whose stored policy is degenerate get a `no headroom` / `no hysteresis`
    `unknown` pill, and the search field and delete confirm speak the real consequence.
  - Counts render `—` until their query settles (`policiesKnown` / `metricsKnown`); the
    `?? '—'` unknown-policy the audit praised is kept and extended to `Total policies`.
  - `Evaluate` is confirmed (it can change live allocations) and now reports its outcome and
    invalidates the whole `['admin','autoscaler']` key, not just metrics.
  - Rows link to the detail route (`ExternalLink` glyph) — the `[id]` page existed with no way
    to reach it from the list.
  - Frame: `title`/`description` deleted (registry-derived "Workload Autoscaling" + Scale glyph);
    `FreshnessBadge` over `worstSourceState([policies, metrics])`; hand-rolled table →
    `AdminTable`; `text-[10px]`/slate/white-alpha → tokens; icon-only Trash2 gets `ariaLabel`.
- `app/admin/autoscaler/policy/[id]/page.tsx`:
  - **S1 `?? 0` → unknown**: `readout()` + `metricsKnown` render `—` in the `unknown` tone while
    the metrics query is loading or failed, and the error branch says "The dashes above mean
    no reading, not zero events." The list page's correct policy is now the detail page's too.
  - S2 scoping: the card is retitled "Autoscaler counters (fleet-wide)" with an explicit note
    that `GET /admin/autoscaler/metrics` is process-wide, unscoped to this policy, and resets on
    restart (there is no per-policy metrics route).
  - S2 entity copy: delete says "Automatic scaling for **workload** <serverId> stops" (was
    "this node"); `backLabel` "Auto-Scaler" → "Workload Autoscaling", removing the fourth name
    for one feature.
  - S2 states: raw "Loading policy…" / "Policy not found." strings replaced with
    `AdminLoadingState`, `AdminErrorState` (with retry, and wording that separates a failed
    read from an absent record) and `EmptyState`.
  - S2 validation: the same `policyDefects` guard on the edit modal, plus a standing warning
    strip on the read view listing defects in the **stored** policy.
  - Honesty added: a line stating that current allocation and replica count are not reported by
    this API, so the page cannot show current-vs-desired (audit S2 asked for that readout — it
    needs a Go endpoint, see Needs central change).
  - Tokens: `text-[10px] uppercase text-slate-500` → `t-eyebrow`; `emerald/blue/red-400` →
    `text-ok/info/danger`; dates via `formatDate`; Evaluate confirmed before the write.

### Node Autoscaling — `/admin/node-autoscaler`
- `components/admin/node-autoscaler-manager.tsx`:
  - **S2 `window.confirm` ×3 deleted** (`:84`, `:106`, `:121`) → `useConfirm`, each with blast
    radius: create names provider/region/bounds, scale-out says it requests a real billable
    instance, scale-in says the node is drained and the instance destroyed and is not undoable.
  - S2 capability: Scale In is **disabled with the server's own reason** the moment a call
    returns the unwired-membership error (`nodeautoscale/service.go:378`) — `capabilityReasonFrom`
    recognises it and the control stays disabled with an explanation until a refresh succeeds,
    instead of looking available and failing repeatedly. A free-text node ID is required; the
    page never picks one. (A pre-flight capability endpoint is requested under Needs central
    change — until then there is no honest way to disable *before* first failure.)
  - S2 `enabled` is now changeable post-create via `api.updatePolicy` (`PUT /autoscale/policies/:id`
    exists and was uncalled); each policy row carries Enable/Disable + Delete.
  - S2 min/max validation: `formDefects` blocks create on min > max, negative counts,
    out-of-range targets, missing region/instance type; stored policies with min > max show a
    warning inline.
  - S2 a11y: every placeholder-only `<input>`/`<select>` gets a real `Input`/`AdminSelect`
    label; provider and evaluator options come from the Go constants (`cloud/provider.go:19-23`
    and `evaluatorCPU|Memory|Both`) instead of free text.
  - S2 events ledger: `eventsState` loading/ready/error, `AdminLoadingState`/`AdminErrorState`,
    a "Read <time>" freshness marker, a refresh button, and a failed read no longer renders
    "No events."
  - S3 developer vocabulary: "node_autoscale_policies · suggest + apply…" and
    "node_autoscale_events · ?policyId=&limit= ." replaced with operator language;
    `Evaluate (dryRun)` → "Dry-run evaluation", `Scale Out (confirm)` → "Scale out".
  - Token bugs: selected-row `border-red-400` (destructive colour as selection) → brand wash +
    `aria-pressed`; the dead class `bg-[var(--brand)]-wash` deleted; `bg-surface` →
    `bg-overlay-subtle`; `text-amber-600` on dark → `ui-alert-warning`; raw `<button>`s → `Btn`;
    scale-in candidate surfaced as actionable copy rather than a bare amber line.
  - Frame: dropped the `components/admin/admin-layout.tsx` shim (its last consumer in this
    slice) for `AdminPageLayout` + `SectionHeader` from `admin-ui`, and deleted the
    `title="Node Autoscaler"` override so the `<h1>` reads the registry label "Node Autoscaling"
    and picks up the `TrendingUp` glyph for the first time. The description no longer reads as an
    internal design note.

### Placement Affinity — `/admin/env-affinity`
- `components/admin/env-affinity-manager.tsx`:
  - **S2 shared-state bug fixed**: `serverId` was bound to both the viewer and the preview
    inputs, so typing in one silently changed the other. Split into `viewerServerId` /
    `previewServerId`; you can now explain with server A while previewing server B.
  - **S2 no-op retry removed**: `OfflineBanner onRetry={() => { /* per-action retry */ }}` did
    nothing. `lastAction` records the most recent action and `retryLast` re-runs it; with no
    action yet it says "Nothing to retry yet" rather than pretending, and the error surface
    only offers Retry when there is something to retry.
  - S2 fleet-wide write: "Patch placement constraints" → "Rebuild placement constraints", now
    behind `useConfirm` stating it re-resolves every server and replaces the scheduler's rules
    for all future placement. `Safe to re-run` became "idempotent, but it is a fleet-wide write
    and is confirmed first", and the button is `tone="danger"`.
  - Node targeting: the free-text node field uses `NodeSelect` (no default selection), and
    Explain is disabled until a node is chosen; handleExplain rejects an empty node with
    "the page will not pick one for you" instead of calling the API ambiguously.
  - S3 a11y: `✓`/`✗` glyphs no longer carry meaning alone — "Matched:"/"Missing:" prefixes,
    `aria-hidden` on the icons, an explicit line when the scorer reports neither, and the
    required/preferred distinction is rendered as a word in the chip instead of living only in a
    `title` tooltip. The selected node is labelled "(this node)" in the ranking.
  - Unknown handling: scores/counts guarded (`Number.isFinite`) with "no score", "an unknown
    number of", "no env groups reported", "No constraints reported…", and an empty-ranking note
    saying the node cannot be compared. Amber/emerald/white-alpha chips → `warn-*`/`overlay-*`/
    `ok`/`danger`; `text-[11px]` → `t-eyebrow`; the registry `Network` glyph now renders from the
    frame after the `title` override was deleted.
  - Copy: `title` deleted; `description` rewritten honestly — the registry row promises
    constraint/rule management that actually lives on Scheduler, so the page says where rule
    editing is rather than claiming to offer it.

### `lib/api/drain.ts`
- `DrainState.progress` and `DrainProgress.total/remaining/current/steps` are now optional,
  matching what the ledger can omit and what `AdminDrain` already defended against; the old
  "required" type plus `?? 0` at the call site was the type-level cause of the not-reported-as-zero
  render. Comments record that a missing step state is unknown, never `done`.
- **Deleted** `fetchEvacuationOrphanCandidates()` and `EvacuationPlanPreview`: the function
  returned a hardcoded `[]` behind its own comment admitting no endpoint exists, and had no
  callers. A discovery-shaped call that always answers "no orphans" is the defect class this
  slice exists to remove; the deletion note points at the honest surface instead.

## Verified
- Final `npx tsc --noEmit`, filtered to every file in this scope: **0 errors** across
  `AdminReconciliation.tsx`, `AdminOrphans.tsx`, `AdminDrain.tsx`, `AdminCleanup.tsx`,
  `procedures-manager.tsx`, `node-autoscaler-manager.tsx`, `env-affinity-manager.tsx`,
  `app/admin/failover/page.tsx`, `app/admin/scheduler/page.tsx`,
  `app/admin/autoscaler/page.tsx`, `app/admin/autoscaler/policy/[id]/page.tsx`,
  `lib/api/{cleanup,reconciliation,drain}.ts`.
- Whole-tree typecheck currently reports 28 errors in three files —
  `components/admin/AdminBilling.tsx`, `AdminMounts.tsx`, `AdminAllocations.tsx` — **none of
  which are in this scope** (other agents' concurrent edits; JSX parse errors at
  `AdminBilling.tsx:433`, `AdminMounts.tsx:344/650`). Not touched.
- Palette/vocabulary sweep verified by grep over my 11 files: no `text-slate-*`,
  `bg-white/[0.0x]`, `border-white/10`, `text-red-400`/`emerald-400`/`amber-400`,
  `text-[10px]`, `text-[11px]`, `window.confirm`, `store_reconcile`, `Service.Start` or
  DB-table names in operator-facing copy remain (the one remaining `*_remediations` match is
  inside a source comment, not rendered copy).
- Token classes used were checked against `tailwind.config.ts` / `app/globals.css`
  (`ok|warn|danger|info|unknown` with `-line`/`-subtle`, `overlay-subtle/strong`,
  `line-strong`, `t-eyebrow`/`t-meta`/`t-readout`, `ui-alert{,-danger,-warning,-success,-info}`);
  a non-existent `bg-overlay-selected` I introduced was caught and replaced with the brand wash
  the rest of the slice uses.
- Re-read the Go contracts I depend on: `handlers_reconcile.go:20/50/59` (summary shape,
  confirm-and-execute), `store_reconcile.go:242-263` (no run marker), `handlers_failover.go:75-90`
  + `failover/service.go:77-87,459-541` (`Event` JSON keys and the `nil,nil` "no action" answers),
  `handlers_cleanup.go` + `cleanup/service.go:17-25` (bare `CleanupInfo`, no envelope),
  `handlers_procedures.go:54-87` + `procedure/service.go:233-438` (PUT body, async execute,
  writable step statuses), `scheduler/predictive.go:23-31` (no `omitempty`),
  `nodeautoscale/service.go:148-150,373-412` and `cloud/provider.go:19-23` (evaluator and
  provider constants, unwired-membership error).

## Needs central change (do NOT edit these yourself)
- `components/admin/admin-registry.ts:195` — **Orphaned Resources** promises a scan no backend
  performs. Requested: `label: "Failed Deletions"`, `description: "Servers and databases whose remote deletion failed and await manual cleanup"`, and drop `"cleanup"` from `keywords` (it collides with `:197`). `AdminOrphans.tsx` already carries this copy as an override.
- `components/admin/admin-registry.ts:197` vs `:162` — "Cleanup" (placement/allocation GC) is not
  distinguishable from "Image & Cache Cleanup" (per-node disk pruning). Requested
  `label: "Allocation GC"` (or "Platform Cleanup") and remove `"prune"` from `:197` keywords.
- `components/admin/admin-registry.ts:200` — Scheduler's description promises "strategies and
  explanations" it has no UI for. Requested `"Placement scoring, affinity rules and constraints"`.
  `scheduler/page.tsx` currently overrides it honestly; delete the override once the registry matches.
- `components/admin/admin-registry.ts:203` — Placement Affinity's description promises
  constraint/rule management that lives on Scheduler. Requested
  `"Explain placement and re-sync environment affinity rules"`. Same override-then-delete plan.
- `components/admin/admin-registry.ts:199` vs `:166` — `Workflow` is used by both Procedures and
  Nomad; `FlaskConical` by Reconciliation, Preview Environments and State Components (`:143`, `:231`).
  Icon-collision de-duplication is a registry decision; not touched.
- Go: `store.ReconcileSummary` (`store_reconcile.go:242`) has no run marker, so the UI must derive
  "was reconciliation ever measured" from the plan/event lists. Requested
  `LastRunAt *time.Time` + `HasScanned bool` on `ReconcileSummary` (or on
  `GET /admin/reconcile/summary`), which would let `AdminReconciliation` drop the derivation and
  bind a `FreshnessBadge` to the scan itself rather than to the read.
- Go: no read-only autoscale capability surface. `GET /autoscale/capabilities` reporting
  `membershipWired` and `cloudProvisioning` would let Scale In render disabled-with-a-reason
  before a request fails, instead of after (`nodeautoscale/service.go:378`).
- Go: procedures never write a `skipped` step status (`procedure/service.go:325-438`) and complete
  an execution as `succeeded` with failed steps under `continueOnFailure` (`:300-301`). The UI now
  computes both, but a first-class `skipped`/`succeeded_with_failures` status is the correct fix.
- Go: `POST /admin/failover/crash/:serverId/:nodeId` is named as a simulation while running real
  failover (`handlers_failover.go:82` → `service.go:497`). The UI relabelled and confirmed it; the
  route should be renamed (e.g. `…/failover/trigger-crash`) or gain a `dryRun` flag. Its two
  `nil,nil` returns (no matching policy, in cooldown) are also worth distinguishing in the response.
- Go: the autoscaler has no per-policy metrics route (`handlers_autoscaler.go:80` is fleet-global)
  and no current-vs-desired replica readout, so `/admin/autoscaler/policy/[id]` can only disclaim
  both rather than show them.
- Not owned by this scope, reported only: `components/admin/AdminBilling.tsx`, `AdminMounts.tsx`
  and `AdminAllocations.tsx` currently fail to parse/typecheck (JSX errors), which will break the
  web build for everyone until their owners land.

## Removed
- `lib/api/drain.ts`: `fetchEvacuationOrphanCandidates()` (hardcoded `[]` behind a comment admitting
  no endpoint exists) and its `EvacuationPlanPreview` type — zero callers.
- `AdminReconciliation.tsx`: the `store_reconcile.go:13` operator-facing debug strip and the
  hand-rolled five-`div` KPI grid (replaced by `AdminStatCard`).
- `AdminDrain.tsx`: `stepState()`, the function that returned `"done"` for every step of a cancelled
  drain.
- `node-autoscaler-manager.tsx`: three `window.confirm()` calls; the dead
  `bg-[var(--brand)]-wash` class; the `admin-layout.tsx` frame import (that shim's last consumer in
  this slice); `border-red-400` as a selection colour.
- `env-affinity-manager.tsx`: the `onRetry` no-op closure on `OfflineBanner`.
- `failover/page.tsx`: the `CrashSummary` envelope dump (`Object.entries(…).slice(0,6)` rendering
  `data → [object]`) and the unconditional green "Crash handled" pill.
- `scheduler/page.tsx`: the hardcoded `docker | k3s | nomad` option list; the unconditional
  `?? 0` metric defaults; the always-green "scored node" pill.
- Every hand-passed `title`/`description`/`icon` that only restated `admin-registry.ts` across the
  ten pages (Reconciliation, Orphans, Drain, Cleanup, Failover, Procedures, Scheduler, Autoscaler
  list + detail, Node Autoscaling, Placement Affinity), so titles, glyphs and breadcrumbs now come
  from the registry — except where the registry text is itself false, which is listed above.

## Deferred
- **Scheduler + Placement Affinity IA merge** (audit remediation 2, pattern 3): a registry/nav
  restructure, not a page edit, and `admin-registry.ts` is frozen. Both pages now carry honest
  cross-referencing descriptions instead; consolidation needs an orchestrator decision.
- **Persisting Orphans' status filter across nav** and Failover's search throttling: local
  niceties with no honesty impact; left alone to keep this pass to defects.
- **Procedures create-form IA** (three-column single page, no step reordering): a redesign, and
  `position` is still derived correctly from list order on submit.
- **Disabling Scale In *before* the first failed call**: impossible without the capability endpoint
  requested above; the current behaviour is evidence-based (disable once the server has said so)
  rather than speculative.
- **`autoscaler` current-vs-desired replica readout**: no endpoint exists to serve it; the detail
  page states the limitation instead of approximating it.



## Needs central change (do NOT edit these yourself)
- `components/admin/admin-registry.ts:195` — **Orphaned Resources** row is a promise no backend keeps.
  Requested edit: `label: "Failed Deletions"`, `description: "Servers and databases whose remote deletion failed and await manual cleanup"`,
  and drop `"cleanup"` from `keywords` (it collides with the Cleanup row at `:197`). The page copy in
  `AdminOrphans.tsx` already says this; the sidebar still advertises a scan.
- `components/admin/admin-registry.ts:197` vs `:162` — naming triangle: "Cleanup" (allocation GC) is
  indistinguishable from "Image & Cache Cleanup" (per-node disk pruning). Requested: `label: "Allocation GC"`
  (or "Platform Cleanup") and remove `"prune"` from the `:197` keywords, which belongs to the docker row.
- `components/admin/admin-registry.ts:200` / `:203` — both **Scheduler** and **Placement Affinity** promise
  "explanations" in their descriptions, but only env-affinity renders explain; Scheduler additionally promises
  "strategies" it has no UI for. Requested: `:200` → "Placement scoring, affinity rules and constraints";
  `:203` → "Explain placement and re-sync environment affinity rules". Pages currently override with honest
  `description` props; deleting those overrides is the follow-up once the registry matches.
- `components/admin/admin-registry.ts:194/:202` — **Reconciliation** and **Node Autoscaling** titles/descs are
  fine; no change requested. `:201` Workload Autoscaling is fine (detail route uses it as its back label).
- Go (frozen, reported only): `/admin/reconcile/summary` has no run marker. Requested
  `ReconcileSummary{…, lastRunAt *time.Time, hasScanned bool}` (or expose newest
  `reconcile_plans.created_at`) so the UI can stop deriving "was it ever measured" from the plan list.
  Also `store_reconcile.go:242` fields are `int` and cannot express "not measured".
- Go (frozen, reported only): no read-only capability surface for the node autoscaler. A
  `GET /autoscale/capabilities` reporting `membershipWired` / `cloudProvisioning` would let Scale In render
  disabled-with-a-reason instead of failing at call time (`nodeautoscale/service.go:378`).
- Go (frozen, reported only): procedures never write a `skipped` step status
  (`procedure/service.go:325-438`), so steps after a halting failure stay `pending` forever and an execution
  can be `succeeded` while red `failed` steps sit under it (`:300-301`). The UI now computes and labels that
  ("succeeded with failures" / "not run"), but a first-class status would be the right fix.
- `forge/api/internal/http/handlers_failover.go` + `POST /admin/failover/crash/:serverId/:nodeId`: the route
  name advertises a simulation and the handler is real failover (`failover/service.go:497`). Renaming the
  control is done in the UI; the route itself should be renamed (`…/failover/trigger-crash`) or given a
  `dryRun` flag.
