# Scope 9 — Operations B: reconciliation, autoscaling, placement
Audited: 10 registry routes, 12 page files (incl. `app/admin/autoscaler/policy/[id]/page.tsx`), 10 component files, 4 API clients (`lib/api/reconciliation.ts`, `cleanup.ts`, `drain.ts`, `envaffinity.ts`, `procedures.ts`, `nodeautoscale.ts` partial).
Frames in use: DashHeader 0 · AdminPageHeader→SectionHeader 6 (Reconciliation, Drain, Failover, Scheduler, Autoscaler list, Procedures) · SectionHeader direct 3 (Orphans, Cleanup, autoscaler policy detail) · admin-layout shim→SectionHeader 1 (Node Autoscaling) · ForgePage/canonical 0.

**Audit-volatility warning:** a concurrent fixer is editing source during this audit. `AdminReconciliation.tsx` changed between reads (title "Reconciliation Center"→"Reconciliation" was fixed mid-pass; a new `info={adminPageGuides.reconciliation}` prop at :238 has no matching `info` prop on `AdminPageHeader` (`admin-ui.tsx:280-313`) — likely in-flight type error). `app/admin/autoscaler/page.tsx` also changed between reads and the current working tree does not parse (see S1). HEAD-vs-worktree is noted where it matters.

## Per-page findings

### Reconciliation — /admin/reconciliation
- files: app/admin/reconciliation/page.tsx (10), components/admin/AdminReconciliation.tsx (364, mid-edit)
- frame: AdminPageHeader (legacy admin-ui)
- title: "Reconciliation" (AdminReconciliation.tsx:236) vs registry "Reconciliation" → MATCH (was "Reconciliation Center" ~10 min ago; fixed concurrently)
- description: now matches registry (admin-registry.ts:194); was free text
- icon: FlaskConical used on trigger Btn (AdminReconciliation.tsx:255) and CardHeader (:314); registry icon FlaskConical → MATCH, but FlaskConical is shared with Preview Environments (admin-registry.ts:143) and State Components (:231) — 3-way nav glyph collision
- **S1** · "Drifts 0 · Unresolved 0" KPIs render from DB counters with no way to distinguish *scan never ran* from *fleet in sync* — `ReconcileSummary` (lib/api/reconciliation.ts:3-8) has no lastRun/hasScanned field; KPIs at AdminReconciliation.tsx:279-303. A fresh or broken cluster shows a calm all-zero board: exactly the "never-run check rendered as healthy" failure the project rule forbids
- S1 · Silent staleness: summary/plans poll `refetchInterval: 15_000` (:196, :202) but there is no FreshnessBadge bound to `dataUpdatedAt`; after first success, failed background refetches keep showing cached numbers with no stale indicator (react-query keeps `data`, `isError` stays false)
- S2 · Debug strip hardcoded into operator UI: `…via store_reconcile.go:13…` — a Go file:line citation shown to users (:262-265); a structured `adminPageGuides` entry already exists (components/admin/admin-page-guides.ts:167-175) but is unused by this strip
- S2 · Transient/mid-edit: `info={adminPageGuides.reconciliation}` (:238) passes a prop `AdminPageHeader` does not declare (admin-ui.tsx:280) — type error unless header is being extended right now
- S3 · `select` for trigger kind has no accessible name (:240-248); icon-only refresh `Btn` (RefreshCw, :259-261) has no aria-label
- S3 · Confirm dialog copy "It will be queued for execution." (:170) while the endpoint is confirm-**and**-execute (`handlers_reconcile` → `ConfirmAndExecute`, forge/api/internal/services/reconciler/trigger.go:383) — "queued" is wrong
- S3 · KPI cards still use text-amber-400/red-400/yellow-400/blue-400 and `bg-white/[0.015]`, `text-[11px]` (:262, :285-303) instead of tokens

### Orphaned Resources — /admin/orphans
- files: app/admin/orphans/page.tsx (5), components/admin/AdminOrphans.tsx (138)
- frame: SectionHeader direct (admin-ui)
- title: "Orphan Remediation" (AdminOrphans.tsx:32) vs registry "Orphaned Resources" (admin-registry.ts:195) → **MISMATCH**
- description: page promises a *remediation queue* ("Force-deleted servers and databases… tracked here", :33); registry promises "Servers and databases with no owning record" → **CONTRADICTS**
- icon: none rendered; registry `Bug` (:195) — no in-page echo
- **S1** · The page is a failure-queue viewer, not an orphan scanner: it lists only `server_orphan_remediations`/`database_orphan_remediations` rows reported after daemon deletion failed (AdminOrphans.tsx:15, :64, :67 "No pending server orphan remediation tasks."). No discovery API exists (`grep orphan` in forge/api/internal/http finds only metric counters and the remediation endpoints), so "0" here means "no *reported* failed deletions", **not** "no orphans exist". An operator reading the sidebar label ("Orphaned Resources") will take an empty list as "fleet has no orphans" — deceptive
- S2 · Ad-hoc loading string "Loading remediation tasks…" (:53) and hand-rolled red error div (:55-58) instead of AdminLoadingState/AdminErrorState used elsewhere in the slice
- S2 · Renders `max-w-[1280px]` own container (:30) — only page in slice off the canonical `max-w-page` AdminPageLayout width (and its wrapper page.tsx:5 adds no frame)
- S2 · Heavy raw palette: text-slate-200/300/400/500, text-red-200, bg-amber-950/10, text-[11px] (:63-64, :76-87, :129-131)
- S3 · Status filter `AdminSelect label=""` (:36-44) — filter has no visible or accessible caption; filter state not persisted across nav
- S3 · Toast system is `sonner` here (:7) while the rest of the slice uses `useToast` (components/ui/toast) — two toast vocabularies in one group

### Node Drain — /admin/drain
- files: app/admin/drain/page.tsx (7), components/admin/AdminDrain.tsx (286)
- frame: AdminPageHeader
- title: "Node Drain" (AdminDrain.tsx:165) vs registry "Node Drain" → MATCH
- description: matches registry intent, page adds ledger-vs-orchestration nuance (:166) → MATCH (longer)
- icon: Droplets on registry (:196); in-page Droplets used on empty state + start card (:179, :215) → MATCH (no header icon)
- **S1** · A **cancelled** drain renders every step as done: `if (overallStatus === "drained" || overallStatus === "cancelled") return "done";` (:66) — the stepper's four green check circles (:88-98) show "traffic withdrawn → evacuation planned → servers migrated → complete" for a drain that was aborted mid-way. Header Pill says Cancelled, but the progress graphic — the thing operators scan — lies
- **S1** · `{s.progress?.remaining ?? 0} remaining / of {s.progress?.total ?? 0} workloads` (:245-246): unreported progress renders as "0 remaining of 0 workloads" — looks complete/no-op. Type says `progress` is required (lib/api/drain.ts:22) yet the UI defends with `?.` — the mismatch is exactly the not-reported-as-zero case
- S2 · Stepper ignores the backend's authoritative per-step states: `DrainProgressStep.state` exists (lib/api/drain.ts:6-10) but `ProgressStepper` re-derives done/active/pending from step ordering + `current` string (:78-116) — a failed drain shows all steps "pending", including ones that completed
- S2 · Node picker silently empty on `nodesQ` failure: no error branch for nodes query (:130-133, :157-159); with nodes fetch failed, `drainableNodes` is empty and the help line (:201-204) only explains emptiness when `nodes.length > 0`
- S2 · Cancel drain has no confirmation (:249-256) while Cleanup/Failover/Orphans confirm their writes; inconsistent destructive-weighting across the slice
- S3 · No FreshnessBadge though active drains poll 5 s (:127); token drift: `text-[10px]`, `title`-only step tooltip (:95, :99), hardcoded shadow `rgba(245,158,11,0.08)` (:92)
- cross-ref (Scope 5): drained state *is* visible on Nodes page (AdminNodes.tsx:263, :367) — gap is only that Nodes also offers a separate desiredState="draining" control (AdminNodes.tsx:542-544) whose relationship to this ledger's beginDrain is unexplained in either UI

### Cleanup — /admin/cleanup
- files: app/admin/cleanup/page.tsx (7), components/admin/AdminCleanup.tsx (202)
- frame: SectionHeader direct + AdminPageLayout
- title: "Cleanup" (AdminCleanup.tsx:56) vs registry "Cleanup" → MATCH
- description: matches and extends registry line (:57) → MATCH
- icon: registry Trash2 (:197); in-page Trash2 on both cards and Run Btn (:80, :106, :141) → MATCH
- Best reference-compliance in the slice: `FreshnessBadge` bound to `sourceState(inspectQ, 30_000)` (:58), useConfirm on both run buttons (:70-77, :163-169), AdminLoadingState/AdminErrorState, dry-run "Inspect" explained (:134-136), and the page itself disambiguates from /admin/orphans (:187-195)
- **S1** · Counter client defaults an unrecognized success response to zeros: `unwrapInfo` returns `{staleReservations: 0, orphanedAllocations: 0}` on any unexpected shape (lib/api/cleanup.ts:11-27, `?? 0` chain + explicit TODO comment at :24) — a contract change would render Verdict "clean" (:131) and hide the failure; defense should error, not zero
- S2 · Naming-collision judgment (see scope patterns): title "Cleanup" alone is under-specified next to "Image & Cache Cleanup" (admin-registry.ts:162, page title confirmed at docker-cleanup-manager.tsx:258); registry keywords ["gc","prune"] overlap docker-cleanup's ["prune","disk","reclaim"]
- S3 · `StatsRow` "Total stale" tone `green` when 0 (:100) — a verdict color on a counter is fine here only because inspect ran; pair with the S1 above if unwrap is fixed
- S3 · Run card leaks internals: "the service also runs every 5 min via `Service.Start(ctx)`" (:180-182)

### Failover — /admin/failover
- files: app/admin/failover/page.tsx (340, inline page — no component), none
- frame: AdminPageHeader
- title: "Failover" (page.tsx:158) vs registry → MATCH; description matches registry line (:159) → MATCH
- icon: registry LifeBuoy (admin-registry.ts:198) — never rendered in-page (header has no icon; cards use Shield/ShieldAlert/BarChart3, :174, :210, :230)
- **S1** · "Simulate crash" card is not a simulation: `Trigger Crash` (:273) POSTs `/admin/failover/crash/:serverId/:nodeId`, which runs the real `HandleServerCrash` recovery path with `failover.write` scope (forge/api/internal/http/handlers_failover.go:82-88; internal/services/failover/service.go:497). No `useConfirm`, no dry-run flag, and the success panel is a hardcoded green "Crash handled" pill (page.tsx:319, :331) shown for *any* 2xx regardless of what the action did — mislabeled destructive write rendered as success
- S2 · Crash "result" panel dumps the raw envelope: `CrashSummary` prints `Object.entries(...).slice(0,6)` of `{data: event}` → renders `data → [object]` (:314-327); the operator-visible payload is destroyed by `String(value)`/`"[object]"`
- S2 · "Record failure" (:199) mutates the failure ledger per row-click with no confirmation and no result feedback (invalidate only, :104-107)
- S2 · Policy create/edit takes a free-text Node ID (:282) — no node picker (contrast Drain's AdminSelect), so a typo'd policy silently matches nothing; `metrics?.failuresDetected ?? "—"` (:166-168) is the *correct* unknown pattern — keep it
- S3 · Hand-rolled `<table>` with `text-[10px] uppercase` headers (:186-189) instead of AdminTable used two pages over; raw slate/emerald/white-alpha palette throughout (:187-201, :318); `<select>` with unassociated `<label>` (:287); search unthrottled/unpersisted (:51, :77)

### Procedures — /admin/procedures
- files: app/admin/procedures/page.tsx (5), components/admin/procedures-manager.tsx (517)
- frame: AdminPageHeader
- title: "Procedures" (procedures-manager.tsx:279) vs registry → MATCH; description matches (:280) → MATCH
- icon: registry Workflow (:199) — used on list CardHeader (:294) → MATCH; Workflow also on Nomad (admin-registry.ts:166) — 2-way collision
- **S1-adjacent (filed S1)** · Per-step log fetch failures are rendered as "No logs.": `handleViewExec` swallows each `listStepLogs` error ("ignore per-step log failures", :253-259) and the log box then prints "No logs." (:497) — unknown reported as empty. Violates the same rule as the stepper: not-reported is not empty
- S2 · **Failed vs skipped is unrepresentable**: the backend never writes a `skipped` step status (forge/api/internal/services/procedure/service.go:325-438 covers only failed/waiting_approval/succeeded), steps after a halting failure stay `pending` forever, and the UI colors `pending` yellow (:101) — an operator cannot tell "will run" from "will never run". Worse: with `continueOnFailure`, the *execution* is completed "succeeded" (service.go:300-301) while red `failed` steps sit underneath — the list pill (procedures-manager.tsx:389) is green while steps are red, no "succeeded with failures" state
- S2 · No edit path: create (:160-182) and delete (:184-200) only; no PUT is ever called — fixing a runbook means retyping it; `enabled` of an existing procedure cannot be toggled
- S2 · Execution progress is static: after Execute the toast says "queued" (:206) and the last-20 list (:381-399) refreshes once; a `running` execution never auto-updates and there is no per-row refresh — the panel silently shows a frozen status
- S2 · "Cancel" is offered on every execution row regardless of terminal status (:394) — cancel-after-succeeded is an accepted no-op write
- S3 · Three-column single-page IA (list | detail | 470-line create form) with a 520px scrollbox (:301) — dense, but the create form duplicates step fields per step with no drag/keyboard reorder despite `position` being semantic (:85)
- S3 · Token drift inside an otherwise tokenised file: action `select` uses `border-white/10 … text-slate-100` (:428), success banner uses emerald-500 literals (:287)

### Scheduler — /admin/scheduler
- files: app/admin/scheduler/page.tsx (693, inline), none
- frame: AdminPageHeader + AdminTabs (Scores / Affinity / Constraints)
- title: "Scheduler" (page.tsx:298) vs registry → MATCH
- description: "Placement scoring, strategies and explanations." (:299) — matches registry text but **over-promises**: there is no explanation UI on this page; every "why" affordance links away to /admin/env-affinity (:386-389, :456-459, :543). "strategies" is also unrepresented — there is no strategy editor; the only strategy-ish control is the per-node runtime pin card
- icon: registry BarChart3 (admin-registry.ts:200) — rendered as CardHeader icon on scores card (:308) and autoscaler StatsRow icon over there; MATCH-ish
- S2 · Ad-hoc loading strings ×5 instead of AdminLoadingState: "Loading scores..." (:310), "Loading backends..." (:417), "Loading rules..." (:471), "Loading anti-affinity..." (:502), "Loading constraints..." (:545)
- **S1** · "Node scheduler config" hardcodes runtime options `docker | k3s | nomad` (:438-442) independently of the live `Scheduler Backends` list the same tab fetches (:146-149, :421-427) and against capability M ("render disabled with a reason, never enabled-and-lying"): pinning a node to `nomad` is offered even when the backend doesn't report it — enabled-and-lying control
- S1 · "Ingest predictive metrics" is a manual metric-forge defaulting every field to 0 (:114-121, :248-265): one click with only Node ID filled writes a zero-load sample into the scheduler's prediction store, quietly skewing placement scoring fleet-wide. No confirmation, no "this is a test hook" framing beyond the title
- S2 · Affinity rule rows show `weight: {rule.weight ?? 0}` (:483, :514) — unknown weight rendered as 0, while creation defaults weight to 1 (:91); the display contradicts the write contract for the same field
- S2 · Constraints CRUD rewrites the whole set (`putJSON(…[...existing, newConstraint])`, :209-226) with an index-addressed DELETE fallback (:229-241) — last-writer-wins races between two admins are silent; the page does disclose the full-set semantics (:543)
- S3 · Raw-palette saturation: text-slate-100..500, bg-emerald-500/sky-500/amber-500/red-500 bars (:332-334), text-[10px] eyebrows (:341-364, :552), border-white/* throughout; unassociated `<label>`s on selects (:587, :619, :628)
- S3 · `ScoreVerdict` always prints green "scored node" pill (:660) even for a node with score 0 / confidence 0 — the pill asserts outcome, not quality

### Workload Autoscaling — /admin/autoscaler (+ policy/[id])
- files: app/admin/autoscaler/page.tsx (343, inline), app/admin/autoscaler/policy/[id]/page.tsx (317)
- frame: AdminPageHeader (list), SectionHeader (detail)
- title: "Workload Autoscaling" (page.tsx:121) vs registry → MATCH; description matches (:122). Detail page backLabel "Auto-Scaler" ([id]:125) — a **fourth name** for the same feature (sidebar: Workload Autoscaling; route: autoscaler; comment "service autoscaler" in node-autoscaler-manager.tsx:134; back button "Auto-Scaler")
- icon: registry Scale (admin-registry.ts:201) — never rendered; page uses Activity/BarChart3/Zap
- **S1** · List page does not compile in the working tree: line 108 `postJSON(`/admin/autoscaler/evaluate/${encodeURIComponent(serverId}`)` is a SyntaxError (unterminated call inside template substitution; verified by parsing the snippet with node). HEAD (line 105) is correct — a concurrent edit introduced this; if committed, the whole route and build break. Fixer must restore `${encodeURIComponent(serverId)}` (as [id]/page.tsx:101 does)
- **S1** · Detail page fabricates zeros for unmeasured metrics: `metrics?.scaleUpEventsTotal ?? 0` / `?? 0` ([id]:217, :224, :231) render big green "0 Scale Ups" while the metrics query is loading or failed — the sibling list page correctly uses `?? '—'` (page.tsx:133-136). Same feature, two opposite unknown-policies; the detail page violates the project rule
- S2 · Policy form (both create modal page.tsx:274-331 and detail edit [id]:247-311) has **no min≤max validation**: minMemoryMb/maxMemoryMb, minCpu/maxCpu, and scaleUpThreshold/scaleDownThreshold can be saved inverted or equal (up ≤ down defeats hysteresis), and Save is only gated on `!form.serverId` (page.tsx:337). No server picker — free-text Server ID; no current-vs-desired replica readout anywhere; a policy that can never fire is never flagged
- S2 · Detail page's "Autoscaler Metric Summary" ([id]:207-234) shows **fleet-global counters** in a per-policy page with no scoping note — reads as this server's scaling history
- S2 · Detail delete copy says "Automatic scaling for this node will stop" ([id]:131) on a *workload/server* policy — wrong entity, echoes the workload/node conflation the whole slice suffers
- S2 · Detail loading/error are raw strings ("Loading policy..." [id]:102, "Policy not found." :106) — no AdminLoadingState/EmptyState/error.tsx
- S3 · "Evaluate" click on list page (page.tsx:198-205) invalidates metrics only (:109-111) — the per-policy effect of evaluating is invisible; no success toast either
- S3 · icon-only Trash2 Btns without accessible names (page.tsx:213, [id] row actions); `text-[10px] uppercase` headers (:155, [id]:160+)

### Node Autoscaling — /admin/node-autoscaler
- files: app/admin/node-autoscaler/page.tsx (5), components/admin/node-autoscaler-manager.tsx (269)
- frame: `AdminPageLayout(title=…)` from **components/admin/admin-layout.tsx** — a third frame module, now a compatibility shim over admin-ui (admin-layout.tsx:9-20) delegating to SectionHeader; the *only* consumer left in this slice is this file (plus forgefile/zerodowntime/mail/billing/webauthn/onboarding managers)
- title: **"Node Autoscaler"** (node-autoscaler-manager.tsx:133) vs registry "Node Autoscaling" (:202) → MISMATCH
- description: page text (:134) matches registry promise but reads as an internal design note ("suggest + explicit confirm", "(which scales replicas)") → partial
- icon: registry TrendingUp (:202) — no icon anywhere in-page
- S2 · **All destructive/confirm flows use native `window.confirm()`** (:84, :106, :121) while every other page in the slice uses `useConfirm`/`AdminConfirmDialog` — browser chrome in a tokenised dashboard; scale-in/out also proceeds with zero context (which node, current fleet size)
- S2 · Capability gating inverted (rule M): Scale In is always enabled with a free-text nodeId (:184-190), and the card documents that it may fail server-side ("otherwise returns 'membership service not wired for scale-in'") — enabled-and-lying instead of disabled-with-reason
- S2 · No min/max validation: minNodes/maxNodes are independent number inputs (:203-204) — min 5 / max 1 creates and persists
- S2 · Every input is placeholder-labelled (:186, :195-209) — no `<label>`, no accessible names, placeholder vanishes on typing; `evaluator` select (:214) unlabeled
- S2 · Policies are create-or-delete only — no edit/update call exists in the component; `enabled` can never change post-create (:212)
- S2 · Token bugs: `border-red-400` for the *selected* policy row (:160) — red = destructive color vocabulary used as selection; `bg-[var(--brand)]-wash` (:160) is not a valid Tailwind construct (class never generated, dead style); `bg-surface` (:160, :176, :183) used as if a utility; `text-amber-600` on dark (:179)
- S3 · AdminCard descriptions leak DB table names to operators: "node_autoscale_policies · suggest + apply…" (:149), "node_autoscale_events · ?policyId=&limit= ." (:239) — same genre as the Reconciliation strip
- S3 · Events ledger has no auto-refresh and no freshness marker (:239-266) — for a page about an *automatic* process

### Placement Affinity — /admin/env-affinity
- files: app/admin/env-affinity/page.tsx (5), components/admin/env-affinity-manager.tsx (199)
- frame: AdminPageHeader
- title: "Placement Affinity" (env-affinity-manager.tsx:82) vs registry → MATCH
- description: :83-84 differs from registry "Placement constraints, affinity rules and explanations" — the registry row *promises constraint/rule management*, the page offers explain/preview/resync only; actual rule editing lives on **Scheduler**'s Affinity tab. Cross-page description contradiction → CONTRADICTS
- icon: registry Network (:203); page renders MapIcon/Sparkles/Wand2 (:95, :100, :157) and Network is actually painted on Scheduler's cards (scheduler page.tsx:500, :533) — icon is on the wrong page
- S2 · **Both cards share one `serverId` state**: viewer input "Server (optional)" (:98) and preview input "Server" (:161) bind the same `useState` (:43) — typing in one changes the other; you cannot explain with server A while previewing server B. Broken, visible-on-first-use
- S2 · `OfflineBanner onRetry` is a **no-op closure** (:80: `onRetry={() => { /* per-action retry */ }}`) — the banner's Retry button does nothing; and the page loads no data on mount, so it is all action-driven with no idle summary of existing rules
- S2 · Fleet-wide write "Patch placement constraints" (:183-185) rebuilds scheduler affinity rules for every server with no confirmation step, leaning on the copy "Safe to re-run" (:181) — compare Cleanup which double-confirms
- S3 · Free-text node/server IDs (:97-98) — no picker, same as Failover/Scheduler; constraint chips are `title`-attribute-only tooltips (:32); "✓/✗" glyphs (:125, :128) carry the semantic load without SR text
- S3 · Mixed tokens again: amber/white-alpha chips (:30, :119), emerald success panel (:187)
- Positive: explain results are honestly derived — eligible/not-a-candidate pill (:106-110), ranking from API (:136-150); this page does *surface* explain.go (POST /placement/explain, lib/api/envaffinity.ts:61-63) as structured data, not raw text

## Scope-level patterns
1. **Unknown→zero/healthy is the dominant S1**: Reconciliation KPIs have no last-scan signal (lib/api/reconciliation.ts:3-8); Cleanup client defaults a malformed response to 0/0 (lib/api/cleanup.ts:11-27); autoscaler detail renders `?? 0` (app/admin/autoscaler/policy/[id]/page.tsx:217-231); Drain shows "0 remaining of 0" (AdminDrain.tsx:245-246) and paints cancelled drains all-done (:66); Orphans' empty queue reads as "no orphans exist" because no discovery endpoint exists at all. One shared fix: every drift/health counter needs a `lastRunAt`/`measured` field and a FreshnessBadge or explicit "never run" state — Cleanup already shows how (AdminCleanup.tsx:58).
2. **Naming triangle (Cleanup / Image & Cache Cleanup / Orphaned Resources) fails on all three axes**: "Cleanup" is the GC of placement reservations/allocations, "Image & Cache Cleanup" is per-node disk pruning, "Orphaned Resources" is a failed-deletion queue (nothing scans orphans). Registry keywords overlap on "prune"/"cleanup" (admin-registry.ts:162, :195, :197). Only Cleanup disambiguates in-page. Judge: titles are not tellable apart; "Orphaned Resources" is actively mislabeled.
3. **Four placement pages could be one "Placement" surface with tabs** (Scoring/Explain | Rules/Constraints | Workload scaling | Node scaling). Scheduler and Placement Affinity both promise "explanations" in the registry (:200, :203) while only env-affinity delivers explain; Scheduler hosts affinity/constraint CRUD that env-affinity's description also promises; Node Autoscaling's own description says it is "distinct from the service autoscaler" (:134) — the grouping into 4 secondary "More" rows is doing negative work. `explain.go` itself is *not* surfaced as raw text (structured result in env-affinity-manager.tsx:103-151), so consolidation is a IA-only change.
4. **Frame count for the slice**: AdminPageHeader 6, SectionHeader-direct 3, admin-layout shim 1, DashHeader 0, canonical ForgePage 0. Three file-naming conventions in one group: `Admin*.tsx` ×4, `*-manager.tsx` ×3, inline `page.tsx` ×3 (+1 detail route). Loading vocabulary is 50/50 between AdminLoadingState and raw "Loading…" strings (10 occurrences).
5. **Confirmation asymmetry**: useConfirm guards deletion nearly everywhere, but crash-simulation writes, drain cancel, metric ingest, constraint/rule deletes mid-case, and fleet-wide resync skip it (Failover :273, Drain :249-256, Scheduler :407, env-affinity :183) — and node-autoscaler uses native `window.confirm` (:84, :106, :121).
6. **Capability-M violation in two forms**: hardcoded docker/k3s/nomad pin (scheduler page.tsx:438-442) and always-enabled scale-in with a documented server-side failure (node-autoscaler-manager.tsx:184-190).
7. **Developer vocabulary leaks to operators**: `store_reconcile.go:13` strip (AdminReconciliation.tsx:262-265), table names in card descriptions (node-autoscaler-manager.tsx:149, :239), query-string syntax in a heading (:239).
8. Concurrency note: the tree is being edited mid-audit (Reconciliation fixed live; autoscaler list regression introduced live). Findings cite working-tree state as of this run.

## Proposed remediation for this slice
1. [state] Add "was this measured, when" to every drift/count surface: extend `/admin/reconcile/summary` payload with lastRunAt (or derive from newest plan/event), add FreshnessBadge rows to Reconciliation + Drain + node-autoscaler ledger, and make all `?? 0` metric renders in autoscaler detail and lib/api/cleanup.ts fail to "unknown" instead of zero. Fix `AdminDrain.tsx:66` cancelled-stepper render and use `DrainProgressStep.state` directly.
2. [ia] Merge Scheduler + Placement Affinity into one "Placement" page (tabs: Explain | Scores | Rules | Constraints) and re-file both autoscalers as children of a "Scaling" pair; update `admin-registry.ts:200, :203` descriptions so "explanations" is promised on exactly one row. [copy]
3. [copy] Rename to break the cleanup triangle: "Cleanup" → "Allocation GC" (or "Platform Cleanup"), "Orphaned Resources" → "Failed Deletions" (the registry description must match: queue, not scanner). Keep the /admin/cleanup→/admin/orphans cross-link pattern (AdminCleanup.tsx:187-195) on all three.
4. [frame] Migrate the 5 raw "Loading…" strings (scheduler) and Orphans' hand-rolled error box to AdminLoadingState/AdminErrorState; adopt AdminPageToolbar+useConfirm for node-autoscaler (delete admin-layout shim's last consumers in this slice); restore `encodeURIComponent(serverId)` in autoscaler list.
5. [state] [a11y] Confirmation policy: every POST that mutates fleet state (crash trigger, resync, metric ingest, drain cancel, scale-in/out) goes through useConfirm with a summary of blast radius; replace all `window.confirm` in node-autoscaler-manager.tsx.
6. [tokens] Sweep `text-slate-*`/emerald/amber/red literals, `text-[10px]`, `border-white/[0.0x]` in AdminReconciliation KPIs, AdminOrphans, Scheduler, Failover, Autoscaler, Drain stepper (replace with `var(--token)`/`ui-*` classes as AdminCleanup/procedures-manager already do).
7. [a11y] Label every bare `<select>`/`<input>` (scheduler :438/:587/:619/:628, autoscaler forms, node-autoscaler placeholders-only inputs, Reconciliation :240); add aria-labels to icon-only Btns (Drain refresh, row Trash2).
8. [capability] Drive scheduler-type pin options from `/admin/scheduler/backends` and disable Scale In with a reason line when membership wiring is absent.

## Open questions for the orchestrator
- Orphaned Resources: does a real orphan *discovery* scan exist anywhere (evacuation planner's DetectOrphans is hinted at in lib/api/drain.ts:52-60 returning `[]`)? If yes, the page should show scan results with last-run; if no, registry description ("servers and databases with no owning record") is a promise no backend keeps — product decision, not a rename.
- Should execution status include a "succeeded with failures" state (backend writes `succeeded` even with continueOnFailure red steps, service.go:288-301), or should the UI compute and show it? Crosses API contract.
- The concurrent fixer is actively changing this slice (Reconciliation frame migration mid-flight with an `info` prop that `AdminPageHeader` doesn't declare; autoscaler list SyntaxError in worktree, correct at HEAD). Should the remediation pass freeze these files first?
