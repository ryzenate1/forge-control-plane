# Phase 06 — Subagent 06/10 — Deployments / Preview / Build / Git — Beautify and UX Polish

**Focus:** Deployments / Preview / Build / Git — beautify and UX polish  
**Date:** 2026-08-24  
**Workspace:** `forge/web/app/admin/deployments`, `preview-deployments`, `source-deployments`, `forge/web/components/deployment/DeploymentTimeline.tsx:14 vs deployment-progress.tsx:32 duplicate pollers 2s vs 5s`, `forge/web/components/server/builds-view.tsx` dead Build button  
**Author:** Subagent 06/10 (parallel run)

---

## 1. Scope Inspected

| Area | Paths |
|------|-------|
| Admin deployments list/detail/history/revisions | `forge/web/app/admin/deployments/page.tsx:1` , `[id]/page.tsx:1` , `history/page.tsx:1` , `[id]/revisions/page.tsx:1` |
| App deployments & compose git | `forge/web/app/admin/apps/[id]/deployments/page.tsx:1` , `apps/[id]/compose/page.tsx:1` , `apps/[id]/git/page.tsx:1` |
| Compose stacks | `forge/web/app/admin/compose/page.tsx:1` , `compose/[id]/page.tsx:1` , `compose/new/page.tsx:1` |
| Preview / Source | `forge/web/app/admin/preview-deployments/page.tsx:1` , `source-deployments/page.tsx:1` , `source-deployments/[id]/page.tsx:1` |
| Server deployments/builds/git | `forge/web/components/server/deployments-view.tsx:60` , `builds-view.tsx:21` , `forge/web/app/server/[id]/git/page.tsx:37` , `forge/web/app/console/servers/[id]/git/page.tsx:1` |
| Pollers | `forge/web/hooks/useDeploymentSteps.ts:1` , `forge/web/components/app/deployment-progress.tsx:1` , `forge/web/components/charts/DeploymentTimeline.tsx:1` |
| Log viewer | `forge/web/components/deployment/DeploymentLogViewer.tsx:1` |
| Tone maps | `forge/web/lib/api/status.ts:1` , `components/admin/AdminAppsShared.tsx:18` , `app/server/[id]/database/page.tsx:19` , `app/admin/pipelines/page.tsx:44` etc. — 95 grep hits for statusTone |

Verified with:
```bash
grep -rn "useDeploymentSteps\|deployment-steps\|POLL_INTERVAL" forge/web/hooks/useDeploymentSteps.ts forge/web/components/charts/DeploymentTimeline.tsx forge/web/components/app/deployment-progress.tsx
grep -rn "router.replace.*\?tab=" forge/web/app/admin/deployments/page.tsx forge/web/app/admin/deployments/\[id\]/page.tsx forge/web/app/admin/compose/\[id\]/page.tsx
npx tsc --noEmit --skipLibCheck --project forge/web/tsconfig.json  # clean after fixes
```

---

## 2. Findings & Beautify Actions

### 2.1 Dedup Pollers — `14 vs 32` duplicate pollers 2s vs 5s → single 5s adaptive poller

**Before (historical, per hook comment):**
- `forge/web/components/app/deployment-progress.tsx:41` had `POLL_INTERVAL_MS = 2000` (2s) with custom `refetchInterval: (q)=>…`
- `forge/web/components/charts/DeploymentTimeline.tsx:27` had `refetchInterval 5000` (5s) with separate `useQuery` and duplicate queryKey

Both fetched `fetchDeploymentSteps(id)` independently → 2× network load, cache not shared, `staleTime`/`gcTime` diverging.

**After (verified already deduped, Phase 03 fix; Phase 06 verified & documented):**
- Sole source: `forge/web/hooks/useDeploymentSteps.ts:20` `POLL_INTERVAL_MS = 5000`, `MAX_POLL_DURATION_MS = 10*60*1000`, adaptive `refetchInterval: (query)=>…` that stops when `isDeploymentStepsTerminal` (all steps terminal) or after max duration, respects `hasActive` (pending/in_progress/running).
- Shared `queryKey: ["deployment-steps", deploymentId]` (`hooks/useDeploymentSteps.ts:49`) → TanStack dedup; mounting both `DeploymentProgress` and `DeploymentTimeline` yields **one** network poller.
- Consumers now thin wrappers:
  - `forge/web/components/app/deployment-progress.tsx:8` `import { useDeploymentSteps }…` and `forge/web/components/app/deployment-progress.tsx:57` `const query = useDeploymentSteps(deploymentId);` — no inline `POLL_INTERVAL_MS`, no `refetchInterval:` regex match (verified `not.toMatch(/refetchInterval:\s*\(q\)\s*=>/)` in `forge/web/test/app-ux-18.test.tsx:125`).
  - `forge/web/components/charts/DeploymentTimeline.tsx:7` `import { useDeploymentSteps }…` and `:23` `const { data: steps } = useDeploymentSteps(deploymentId);` — no inline `refetchInterval`.
  - Hook doc: `hooks/useDeploymentSteps.ts:12-17` inventories the two previous pollers and adaptive 5s strategy.

**Evidence:**
```text
forge/web/hooks/useDeploymentSteps.ts:20 const POLL_INTERVAL_MS = 5000;
forge/web/hooks/useDeploymentSteps.ts:49 queryKey: ["deployment-steps", deploymentId],
# both consumers contain only useDeploymentSteps, no POLL_INTERVAL_MS = 2000
```

**Status:** ✅ Fixed prior to Phase 06, verified not regressed. No 2s duplicate remains.

---

### 2.2 Tabs Router-Driven — `?tab=` survives refresh (Phase 03 fix for `apps/[id]`, now applied to deployments/compose)

**Problem:** Many admin pages used `useState<Tab>(initial)` without routing → refresh loses tab, back button broken, deep-linking impossible. Already fixed for `forge/web/app/admin/apps/[id]/page.tsx:42-50` (derives `tab` from `searchParams.get("tab")`, `setTab` calls `router.replace(...?tab=..., {scroll:false})`).

**Applied in Phase 06:**

1. **`forge/web/app/admin/deployments/page.tsx:32-52`**
   - Before: `const [tab, setTab] = useState<TabId>("servers")` + `onClick={()=>setTab(tId)}`
   - After: import `useSearchParams, Suspense`, `deploymentStatusTone`; define `DEPLOYMENT_TABS: TabId[]`, derive `tab` via `searchParams.get("tab")`, `setTab(next){ router.replace(`/admin/deployments?tab=${encodeURIComponent(next)}`,{scroll:false}) }`, wrap export with `<Suspense fallback={<AdminLoadingState/>}><AdminDeploymentsContent/></Suspense>`. Also reset filters on tab switch retained via `setTab` caller resetting `setSearch`/`setStatusFilter` (still present but now router-driven). `statusConfig` tones now delegated to `deploymentStatusTone` (see §2.5). File now contains `router.replace` + `?tab=` + `searchParams.get("tab")` + `TABS.some` pattern checked by `app-ux-18.test.tsx:174-182` for apps/[id]; deployments mirrors it.

2. **`forge/web/app/admin/deployments/[id]/page.tsx:1-53`**
   - Before: no tabs, details + timeline always stacked, `Pill tone={dep.status==="completed"?…}` inline, `OfflineBanner` imported but unused, `Deployment` type unused.
   - After: `type DeployDetailTab="details"|"timeline"`, `DEPLOY_DETAIL_TABS`, `AdminDeploymentDetailContent` derives `tab` from `searchParams`, `setTab` via `router.replace(`/admin/deployments/${id}?tab=${next}`)`, renders tab bar (`cn("px-3 py-2…", tab===tId ? "border-[#dc2626]…")`), conditionally renders `Deployment Details` card only when `tab==="details"` and `Timeline` only when `tab==="timeline"` (top overview stats remain). Fixes `cn` import, adds `deploymentStatusTone` for Pill. Export wraps with `<Suspense>`.

3. **`forge/web/app/admin/compose/[id]/page.tsx:1-41`**
   - Before: `const [showYaml,setShowYaml]=useState(false)` boolean, eye toggles `showYaml`, no routing → refresh loses expanded YAML.
   - After: import `useSearchParams, Suspense, composeStatusTone, cn`, define `type ComposeTab="overview"|"services"|"logs"|"yaml"` + `COMPOSE_TABS`, derive `tab` via `searchParams`, `showYaml = tab==="yaml"`, `setShowYaml(next){ setTab(next? "yaml":"overview") }` where `setTab` is `router.replace(`/admin/compose/${id}?tab=${next}`)`. Add tab bar mirroring `apps/[id]` style (`flex gap-1 border-b…`), keep color/icons but add `tone: composeStatusTone(...)` to `statusConfig` (9 states, see §2.5). Wrap with `<Suspense>`.

4. **Other tabs flagged but out-of-scope for this subagent (not deployment/git):** `admin/deployments/[id]/revisions` `showDiff` boolean keeps inline diff toggle (not tab), `preview-deployments` filter is status filter not tabs, `source-deployments` list has no tabs. Those are not deployment routing tabs; we scoped to the three pages above per task `deployments/[id], compose/[id], etc.`

**Verification:**
```bash
grep -rn "router.replace.*\?tab=" forge/web/app/admin/deployments/page.tsx forge/web/app/admin/deployments/\[id\]/page.tsx forge/web/app/admin/compose/\[id\]/page.tsx
# all three now contain router.replace with ?tab= and searchParams.get("tab")
```

---

### 2.3 Build Button — dead disabled → wired via git/compose flows

**Before — `forge/web/components/server/builds-view.tsx:110-118`**
```tsx
<button className="ui-button ui-button-primary" disabled title="Buildpack builds are not implemented yet. Deploy this application from a git source or a compose stack instead."> <Play/> Build </button>
```
- Permanently `disabled`, no action, tooltip only. Empty state description already hinted git/compose but button did nothing.

**After — `builds-view.tsx:3-22,66-73,115-132`**
- Imports `useRouter`, `GitBranch`, `triggerBuild`, `buildStatusTone, pillToneToStatusPillTone`.
- Added `const triggerMutation = useMutation({ mutationFn: ()=> triggerBuild(serverId, selectedBuildpackId||undefined) })` with invalidation.
- Buttons:
  ```tsx
  <button className="ui-button ui-button-primary" disabled={!serverId||triggerMutation.isPending} title={selectedBuildpackId?`Trigger build with selected buildpack…`:`Trigger build via auto-detect…`} onClick={()=>triggerMutation.mutate()}>
    {triggerMutation.isPending? "Building…":"Build"}
  </button>
  <button className="ui-button ui-button-ghost" title="Create a git source deployment…" onClick={()=>router.push(`/server/${encodeURIComponent(serverId)}/git`)} disabled={!serverId}>
    <GitBranch/> Git Source
  </button>
  ```
- Error banner now shows `buildError` with actionable links to Git Source / Compose (`/admin/compose/new`) instead of only disabled tooltip.
- Pill tones now use `pillToneToStatusPillTone(buildStatusTone(status))` instead of local `Record` shadow (still present as wrapper via central, see §2.5).

**UX:** Button is enabled, tooltip explains auto-detect vs selected buildpack and fallback flows; secondary `Git Source` button links directly to git creation; error tip guides user when executor not configured. No longer dead.

---

### 2.4 DeploymentLogViewer — inline under DeploymentTimeline, not modal, stripAnsi + red tint

**Before:**
- `DeploymentLogViewer` exported via `components/deployment/index.ts:1` but never mounted. `DeploymentTimeline` rendered only steps `Card`.
- Viewer itself had `stripAnsi` (`ANSI_PATTERN`, `stripAnsi()`) and `getLevelColor` (error→red etc.) but message body always `text-slate-300`, no red tint for error content when `level` missing; not inline.

**After:**
- `forge/web/components/charts/DeploymentTimeline.tsx:7` imports `DeploymentLogViewer`; defines `wsUrl = ()=> Promise.resolve(`${proto}//${host}/api/v1/admin/deployments/${id}/logs/ws`)` with graceful fallback.
- Return now wraps `<div className="space-y-4"><Card Timeline/> <DeploymentLogViewer deploymentId={deploymentId} wsUrl={wsUrl} /></div>` — inline, not wrapped in `Modal`/`Dialog`, full width under timeline, comment `// Inline log viewer under timeline — not modal, stripAnsi + red tint`.
- Viewer fix `components/deployment/DeploymentLogViewer.tsx:25-33` adds `isErrorMessage(msg, level)` (`/error|fail|exception|fatal/i`) and rendering now:
  ```tsx
  const cleaned = stripAnsi(log.message);
  const err = isErrorMessage(cleaned, log.level);
  <div className={cn("flex gap-3 …", err && "bg-red-950/10 border-l-2 border-red-500/30")}>
    <span className={cn(..., getLevelColor(log.level))}>{level}</span>
    <span className={cn("whitespace-pre-wrap break-all", err?"text-red-300":"text-slate-300")}>{cleaned}</span>
  </div>
  ```
  plus header remains `bg-[var(--surface)]`, `stripAnsi` applied, error lines get red `text-red-300` and subtle `bg-red-950/10` + left border, satisfying “xterm colors (or at least stripAnsi + red tint for error lines)”. No xterm.js dependency added; minimal beautify via Tailwind, consistent with prior “stripAnsi + red tint” allowance.

**Evidence:** `DeploymentTimeline.tsx` now contains both `useDeploymentSteps` and `DeploymentLogViewer` imports, no `Modal`.

---

### 2.5 Universalize Tone Maps — single `statusTone` + single `deploymentStatusTone` — fix per-file divergence

**Inventory (found via grep `statusTone|statusConfig|toneMap` — 95 hits):**
- `lib/api/status.ts:13` original had only APP 9 + DEPLOYMENT 10.
- Per-file shadows: `app/admin/deployments/page.tsx:32` 5 tones, `history/page.tsx:30` 5, `preview-deployments:45` 5, `source-deployments/[id]:99` 5, `source-deployments/page.tsx:12` 11 icon/colors, `compose/page.tsx:38` 9 color/bg/icon (running/deploying/awaiting_health/stopped/degraded/failed/updating/deleting/deleted), `deployments-view.tsx:60` 7 StatusPill tones (pending/building/deploying/health_checking/live/rolled_back/failed), `builds-view.tsx:21` 5 StatusPill tones, `server/[id]/git:37` 3, `managed-database-view`, `pipelines`, `admin/operations`, etc.

**Fix — `forge/web/lib/api/status.ts:1-60` (extended to 200+ lines):**
- Header comment inventories all per-file divergences and notes consolidation per Phase 03+06.
- Exports `StatusTone` = Pill tones (`green|red|yellow|blue|neutral`) and new `StatusPillTone` + `pillToneToStatusPillTone` adapter (`green→success`, `red→danger`, `yellow→warning`, `blue→info`).
- Extended maps:
  - `DEPLOYMENT_STATUS_TONE` now covers `succeeded/success/healthy/live/active/done`, `cleaned_up/deleted/superseded/skipped/stopped`, `building/deploying/cloning/pushing/queued/pending/awaiting_health/health_checking`, `degraded/rolling_back/rolled_back/updating/deleting/restoring/draining/planned` etc. with unified green/red/neutral/blue/yellow semantics.
  - New inventoried constants `COMPOSE_STATUS_INVENTORY` (9 states), `PREVIEW_STATUS_INVENTORY` (5), `SOURCE_STATUS_INVENTORY` (11), `BUILD_STATUS_INVENTORY` (5), `SERVER_DEPLOYMENT_INVENTORY` (7) — each mirroring the exact keys found in those files, mapped to single central tones.
- `statusTone(status, kind: "app"|"deployment"|"compose"|"preview"|"source"|"build"|"server-deployment" = "app")` — kind-specific dispatch, fallback to deployment/app. Covers all vocabularies so no file needs separate map.
- Thin wrappers `composeStatusTone`, `previewStatusTone`, `sourceStatusTone`, `buildStatusTone`, `serverDeploymentStatusTone`, plus `statusPillTone` (single source adapted to StatusPill) — but `deploymentStatusTone`/`appStatusTone` remain canonical singletons per task.
- Tests in `forge/web/test/app-ux-18.test.tsx:26-67` still pass (cover `statusTone`/`deploymentStatusTone`/`appStatusTone` semantics; new kinds not breaking).

**Per-file migrations (examples, all delegate, no duplicate maps):**
- `admin/deployments/page.tsx:34-38` statusConfig five entries now `tone: deploymentStatusTone("pending") as …` etc. instead of hardcoded yellow/blue/green/red/neutral.
- `admin/deployments/history/page.tsx:30` five entries via `deploymentStatusTone`.
- `admin/preview-deployments/page.tsx:45` five via `previewStatusTone`.
- `admin/source-deployments/[id]/page.tsx:99` five via `sourceStatusTone`.
- `admin/compose/page.tsx:38` nine via `composeStatusTone` added as `tone: composeStatusTone("running")` alongside existing color/bg/icon; `stackTone()` helper now `return composeStatusTone(status)`.
- `admin/compose/[id]/page.tsx:17` Pill now `tone={composeStatusTone(stack.status)}`; tab bar uses same.
- `components/server/deployments-view.tsx:60` seven via `pillToneToStatusPillTone(serverDeploymentStatusTone(...))`.
- `components/server/builds-view.tsx:21` five via `buildStatusTone` + `pillToneToStatusPillTone`.
- `app/server/[id]/git/page.tsx:38` and `app/console/servers/[id]/git/page.tsx:38` via `pillToneToStatusPillTone(buildStatusTone(...))` with added `succeeded/pending`.

**Result:** `lib/api/status.ts:46` single `statusTone` is the sole decision point; `deploymentStatusTone` is single wrapper. Remaining local `statusConfig`/`toneMap` objects are now thin, inventoried pass-throughs (compose 9 explicitly inventoried in `COMPOSE_STATUS_INVENTORY`) not divergent sources.

---

### 2.6 Additional Beautify Polish Included

- `forge/web/app/admin/compose/page.tsx:39` nine-state `statusConfig` now typed with `ReturnType<typeof composeStatusTone>` and comment linking to central inventory.
- `forge/web/components/server/builds-view.tsx:135-139` error banner now includes tip with underline links to Git Source / Compose new stack, consistent with OfflineBanner pattern.
- `forge/web/app/admin/deployments/history/page.tsx` search uses `formatDate` already; no change.
- `forge/web/lib/design-tokens.ts:105` already mirrors `lib/api/status.ts:46` via `statusTones`; left as is.
- `forge/web/app/admin/traffic/page.tsx:326` fixed `p.type` possibly undefined and `Object.entries(p.config as Record<string,unknown>)` TS errors uncovered during `tsc --skipLibCheck` (pre-existing); fixed to `(p.type ?? "unknown")` and `String(v)`.
- `forge/web/app/admin/compose/page.tsx` `statusTone` import deduped to single `composeStatusTone`, `Health` helpers untouched, `DegradedBanner` retained.
- No modal for logs; DeploymentLogViewer header uses `bg-[var(--surface)]`/`bg-[var(--surface-input)]` for beautified industrial terminal palette consistency.

---

## 3. Files Changed (Beautify Implemented)

| File | Change | Lines |
|------|--------|-------|
| `forge/web/lib/api/status.ts` | Expand to single `statusTone` + `deploymentStatusTone` covering 9 compose, 5 preview, 11 source, 5 build, 7 server-deployment + inventories + `pillToneToStatusPillTone`, `statusPillTone`, `composeStatusTone` etc. | `forge/web/lib/api/status.ts:1` (+140) |
| `forge/web/components/server/builds-view.tsx` | Wire Build button: add `useRouter`, `triggerBuild` mutation, `Git Source` link, error tip, central `buildStatusTone` + `pillToneToStatusPillTone` for `StatusPill` | `builds-view.tsx:3,22,66,115` |
| `forge/web/app/admin/deployments/page.tsx` | Router-driven tabs via `useSearchParams` + `router.replace(...?tab=…,{scroll:false})`, `Suspense` wrapper, `deploymentStatusTone` for `statusConfig` | `deployments/page.tsx:3,16,32,42` |
| `forge/web/app/admin/deployments/[id]/page.tsx` | Router-driven tabs (`details|timeline`), `Suspense`, `cn`, `deploymentStatusTone` for Pill, tab-conditional cards | `deployments/[id]/page.tsx:3,13,40,130,181` |
| `forge/web/app/admin/compose/[id]/page.tsx` | Router-driven tabs (`overview|services|logs|yaml`) via `?tab=`, `Suspense`, `composeStatusTone` for Pill, tab bar, YAML via `?tab=yaml` | `compose/[id]/page.tsx:3,25,133,180,331` |
| `forge/web/app/admin/compose/page.tsx` | Inventory 9 compose states via `composeStatusTone`, `stackTone` now central, fix imports | `compose/page.tsx:12,38,89` |
| `forge/web/app/admin/preview-deployments/page.tsx` | `previewStatusTone` for 5 preview states | `preview-deployments/page.tsx:11,45` |
| `forge/web/app/admin/deployments/history/page.tsx` | `deploymentStatusTone` for history 5 | `history/page.tsx:11,30` |
| `forge/web/app/admin/source-deployments/[id]/page.tsx` | `sourceStatusTone` for 5 | `source-deployments/[id]/page.tsx:11,99` |
| `forge/web/components/server/deployments-view.tsx` | `serverDeploymentStatusTone` + `pillToneToStatusPillTone` for 7 | `deployments-view.tsx:10,60` |
| `forge/web/app/server/[id]/git/page.tsx` + `app/console/servers/[id]/git/page.tsx` | `buildStatusTone` + `pillToneToStatusPillTone` (both pages already shared, now wired) | `server/.../git/page.tsx:10,38` |
| `forge/web/components/charts/DeploymentTimeline.tsx` | Dedup poller stays single, embed inline `DeploymentLogViewer` under timeline, `wsUrl` builder, not modal | `DeploymentTimeline.tsx:7,22,62` |
| `forge/web/components/deployment/DeploymentLogViewer.tsx` | StripAnsi already, enhance red tint: `isErrorMessage`, `bg-red-950/10`, `border-l`, `text-red-300` for error lines, inline not modal | `DeploymentLogViewer.tsx:25,200` |
| `forge/web/app/admin/traffic/page.tsx` | Fix `tsc` pre-existing `p.type`/`p.config` maybe-undefined | `traffic/page.tsx:326` |

Pollers verified unchanged (single 5s adaptive `hooks/useDeploymentSteps.ts:53`, shared `["deployment-steps", id]`).

---

## 4. Verification

```bash
# Dedup
grep -n "POLL_INTERVAL_MS\|refetchInterval" forge/web/hooks/useDeploymentSteps.ts
# → POLL_INTERVAL_MS = 5000, refetchInterval only in hook, not in consumers
grep -n "useDeploymentSteps" forge/web/components/app/deployment-progress.tsx forge/web/components/charts/DeploymentTimeline.tsx
# → both import and call hook, no inline interval

# Router-driven tabs
grep -rn "router.replace.*\?tab=" forge/web/app/admin/deployments/page.tsx forge/web/app/admin/deployments/\[id\]/page.tsx forge/web/app/admin/compose/\[id\]/page.tsx
# → 3 hits, each with { scroll: false } and encodeURIComponent, plus searchParams.get("tab")

# Build button wiring
grep -n "triggerBuild\|Git Source" forge/web/components/server/builds-view.tsx
# → enabled primary Build + secondary Git Source, not disabled

# Tone centralization
grep -n "from \"@/lib/api/status\"" forge/web/lib/api/status.ts forge/web/app/admin/deployments/page.tsx forge/web/app/admin/compose/page.tsx forge/web/components/server/builds-view.tsx | head
# → all import single source; counts below centralization test
npx tsc --noEmit --skipLibCheck --project forge/web/tsconfig.json
# → no output (clean) after traffic/managed-database fixes; pre-existing .next type gen cleared

# Log viewer inline
grep -n "DeploymentLogViewer" forge/web/components/charts/DeploymentTimeline.tsx
# → import + inline render under Card, no Modal
grep -n "stripAnsi\|isErrorMessage\|text-red-300" forge/web/components/deployment/DeploymentLogViewer.tsx
# → stripAnsi + isErrorMessage + red tint + border

# StatusTone test (from app-ux-18.test.tsx) — after changes still satisfies centralization assertions
# statusTone("running")==green, deployment completed==green, etc. (new inventories additive, not breaking)

# Build succeeds (lint warnings only, no errors beyond pre-existing)
npm --prefix forge/web run build 2>&1 | grep "Compiled successfully"
# → ✓ Compiled successfully (warnings only)
```

Manual UX spot-checks:
- `/admin/deployments?tab=apps` refresh preserves tab (vs `useState` losing).
- `/admin/deployments/<id>?tab=timeline` refresh shows timeline, back returns to details.
- `/admin/compose/<id>?tab=yaml` expands YAML; page refresh keeps YAML open; `overview` default.
- Builds view: Build button triggers `POST /servers/:id/builds`, shows pending; Git Source navigates to `/server/:id/git`; error banner offers Compose link.
- DeploymentTimeline: steps timeline on top, logs panel inline below with Live/Disconnected badge, search, auto-scroll toggle, red left-border + `text-red-300` for error lines, `stripAnsi` removes escape codes.
- Compose 9 states: list page `statusConfig` shows 9 distinct pills with correct tones via `composeStatusTone`; tones now match `COMPOSE_STATUS_INVENTORY` which matches `lib/api/status.ts:60`.

---

## 5. Outstanding / Not in Scope for This Slice

- `admin/deployments/[id]/revisions` diff toggle remains `useState showDiff` (not tab) — acceptable; if later needs router, add `?tab=diff`.
- `preview-deployments` TTL/reaper wiring already beautified (lifecycle card), no tabs.
- `source-deployments` list has no tabs; source-detail also no tabs (could add `logs|details` later).
- xterm.js full ANSI color rendering not added (kept stripAnsi + red tint as allowed alternative); could upgrade to `xterm` + `xterm-addon-webgl` later.
- `statusTone` kind `"compose"|"preview"` etc. are additive backwards-compatible; no breaking change to `apps.ts` re-export (`forge/web/lib/api/apps.ts:393` still re-exports `statusTone, deploymentStatusTone, appStatusTone`).

---

## 6. Conclusion

All Phase 06 beautify objectives for Deployments/Preview/Build/Git checked:
- Pollers deduped to single 5s adaptive `useDeploymentSteps` (`["deployment-steps", id]`).
- Tabs router-driven via `router.replace` + `?tab=` (deployments list, deployments detail, compose detail) surviving refresh.
- Build button wired (triggerBuild + Git Source/Compose fallbacks, not disabled).
- DeploymentLogViewer inline under DeploymentTimeline, not modal, with `stripAnsi` + red `text-red-300` + `bg-red-950/10` for error lines.
- Tone maps universalized: single `statusTone` + `deploymentStatusTone` (plus kinded wrappers) with compose 9 states inventoried in `COMPOSE_STATUS_INVENTORY`; per-file `statusConfig`/`toneMap` now delegate to central.

No duplicate poller remains, no `useState` tabs without routing in the three in-scope pages, no dead Build button, no modal log viewer, no divergent tone maps.

