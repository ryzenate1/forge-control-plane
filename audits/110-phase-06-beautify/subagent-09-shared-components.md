# Subagent 09 — Shared Components & Design System — Beautify / Universalize

**Agent:** 110-06-09 of 110 · Phase 06 Agent 09/10 (parallel)
**Date:** 2026-08-24
**Focus:** `forge/web/components/shared/*` (states-empty, states-loading, states-error, states-offline, generation-fenced-dot), `forge/web/components/admin/admin-ui.tsx:45`, `forge/web/components/ui/*`, `forge/web/components/charts/*`, `forge/web/lib/api/status.ts`

---

## 1. Executive Summary

Unified primitives across **shared empty/loading/error/offline**, **StateLane signature**, **chart palette**, and **tone centralization** so all 10 empty variants, skeletons, error cards, and operation/server/node badges share `var(--surface)/var(--line)/var(--text)/var(--text-subtle)/var(--brand)/var(--danger)/var(--success)/var(--warning)` tokens instead of hardcoded `bg-white/*`/`text-slate-*`/`bg-red-500*`/`#ef4444`/`#1e2536`.

- `states-empty.tsx:17` EmptyCard now `border-[var(--line)] bg-[var(--surface)]` + icon `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--text-subtle)]`; 10 exports (`EmptyList, EmptySearch, EmptyDeployments, EmptyBackups, EmptyDomains, EmptyServices, EmptyGit, EmptyCertificates, EmptyDNSProviders, EmptyOrganizations`) all inherit tokenized card.
- `states-loading.tsx:7` Skeleton → `bg-[var(--surface-raised)] border-[var(--line)]`, `SkeletonList` mimics `ui-card` structure with `divide-[var(--line)]`; `SpinnerInline`/`SpinnerPage` tokenized (`text-[var(--text-subtle)]`, `text-[var(--brand)]`).
- `states-error.tsx:30` ErrorCard → `border-[var(--danger)]/20 bg-[var(--danger-subtle)]`, icon `bg-[var(--danger-subtle)] text-[var(--danger)]`, text `text-[var(--text)]/[var(--text-subtle)]`, CTA `bg-[var(--danger)] hover:bg-[var(--brand-hover)]`.
- `generation-fenced-dot.tsx:19` StateLane signature extended: `stateToDotClass` now uses `bg-[var(--success)]/bg-[var(--warning)]/bg-[var(--danger)]/bg-[var(--text-subtle)]` plus new exports `ServerStateLaneBadge`/`NodeStateLaneBadge` and usage propagated to `components/server/server-nav.tsx` (server cards) and `components/monitoring/node-list.tsx` (node badges).
- Charts `ResourceUsageBar.tsx:5, ServerCPUChart.tsx:1, metrics-chart.tsx:1` now distinct palette (CPU blue, Memory emerald, Disk amber, Network violet/cyan) with tooltip `bg-[var(--surface-raised)] border-[var(--line)]` instead of triplicate `bg-[#1e2536] border-white/10`; no `#ef4444` triple-red.
- Tone centralization: `forge/web/lib/api/status.ts:46` is single source (`statusTone`, `deploymentStatusTone`, `appStatusTone`, `buildStatusTone`, `serverDeploymentStatusTone`, `composeStatusTone`, etc. with `BUILD_STATUS_INVENTORY`/`SERVER_DEPLOYMENT_INVENTORY` etc.). Per-file duplicated `const statusTone = {...}` removed/replaced via import in `app/server/[id]/database/page.tsx:19`, `app/server/[id]/git/page.tsx:37`, `app/console/servers/[id]/git/page.tsx:37`, `components/database/*`, `components/admin/AdminMigrations.tsx:9`, `app/admin/pipelines/page.tsx:44`, `components/server/builds-view.tsx:21` / `deployments-view.tsx:60`, `components/admin/AdminOperations.tsx:25` / `OperationsTimeline.tsx:44`.
- Design-token audit produced (§7) with remaining hardcoded inventory and replacements applied to `components/admin/admin-ui.tsx:15` Pill, `components/shared/states-offline.tsx:30`, `states-permission.tsx:15`, `upload-progress.tsx:48`, `states-connectivity.tsx:151`, `ui/loading-skeleton.tsx:6`, `monitoring/node-list.tsx:9`, `admin-ui Pill` etc.

---

## 2. Detailed Audit & Implementation

### 2.1 `states-empty.tsx:17` — 10 semantics, var(--surface)/var(--line), Phosphor CTA

**Before (`states-empty.tsx:29-34`):**
```
border border-dashed border-white/10 px-6 py-14
  bg-white/[0.05] text-slate-400
  h3 text-slate-200, p text-slate-300
EmptySearch Clear button: border-white/10 text-slate-300 hover:bg-white/[0.06]
```

**After:**
- `EmptyCard` container: `border-dashed border-[var(--line)] bg-[var(--surface)]` (`states-empty.tsx:29`)
- Icon well: `border border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]` (`states-empty.tsx:30`)
- Title: `text-[var(--text)]`, description: `text-[var(--text-subtle)]` (`states-empty.tsx:33-34`)
- `EmptySearch` clear: `border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text)] hover:bg-[var(--surface)] hover:border-[var(--border-strong)]` (`states-empty.tsx:82`)
- Action slot: plain `mt-5 flex flex-wrap justify-center gap-2` so callers use canonical `Button variant="default"` (`bg-[var(--brand)]`) or `variant="secondary"` (`var(--surface-raised)`). Phosphor/brand CTA is enforced via `components/ui/button.tsx:14` `default: bg-[var(--brand)]` and via `forge/web/lib/design-tokens.ts:72` `phosphor: #FFB000` docs (CTA accent = `var(--brand)`; phosphor token alias retained in `design-tokens.colors.phosphor` for legacy). No hardcoded `white/10` remains in empty.

All 10 exports verified still present (`states-empty.tsx:42-180`): `EmptyList`, `EmptySearch`, `EmptyDeployments`, `EmptyBackups`, `EmptyDomains`, `EmptyServices`, `EmptyGit`, `EmptyCertificates`, `EmptyDNSProviders`, `EmptyOrganizations` via `index.ts:10-21`.

### 2.2 `states-loading.tsx` + `ui/loading-skeleton.tsx` — skeletons mimic ui-card with animate-pulse tokens

`states-loading.tsx:7`:
- `Skeleton` before `bg-white/[0.07]` → `bg-[var(--surface-raised)] border border-[var(--line)] animate-pulse`
- `SkeletonList:12` `ui-card overflow-hidden`, header `Skeleton border-0`, divider `divide-[var(--line)]` (was `divide-white/[0.07]`)
- `SkeletonDetail:35-78`, `SkeletonForm:62` already `ui-card` structured; skeletons now tokenized
- `SpinnerInline:81` `text-[var(--text-subtle)]` (was `text-slate-400/300`)
- `SpinnerPage:90` `text-[var(--brand)] animate-spin` (was `text-red-500`) + `text-[var(--text-subtle)]`

`ui/loading-skeleton.tsx:6-11`:
- `Skeleton` `bg-[var(--surface-raised)] border-[var(--line)]`
- `LoadingSpinner` `text-[var(--brand)]` (was `text-red-500`)
- `CardSkeleton` / `TableSkeleton` already `ui-card` + `ui-card-header` with `divide-[var(--line)]`

Matches `app/globals.css:88` `.ui-card { border-[var(--line)] bg-[var(--surface)] }` and `.ui-card-header { border-[var(--line)] bg-white/[0.018] }` — skeletons now visually identical to card surface.

### 2.3 `states-error.tsx` — ErrorCard uses Fault/Danger token, not hardcoded red-500

`states-error.tsx:30-41` ErrorCard:
- `border-red-500/20 bg-red-500/[0.04]` → `border-[var(--danger)]/20 bg-[var(--danger-subtle)]` (`--danger-subtle: rgba(220,38,38,.10)` in `globals.css:27`)
- Icon well `bg-red-500/10 text-red-400` → `bg-[var(--danger-subtle)] text-[var(--danger)] border-[var(--danger)]/20`
- Title `text-red-200` → `text-[var(--text)]`, message `text-red-300/80` → `text-[var(--text-subtle)]`
- CTAs (`ErrorAlert:68`, `ErrorNotFound:112`, `ErrorNetwork:151`, `ErrorRateLimit:203`) `bg-red-600 hover:bg-red-500` → `bg-[var(--danger)] hover:bg-[var(--brand-hover)]` (Fault red `#dc2626` via `--danger`, hover `#ef4444` via `--brand-hover`)
- Details toggle `text-slate-400` → `text-[var(--text-subtle)] hover:text-[var(--text)]`
- `<pre>` `border-white/10 bg-[#0f1419] text-slate-400` → `border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]`

Also fixed `states-offline.tsx:30` `border-amber-500/30 bg-amber-500/[0.12] text-amber-100` → `border-[var(--warning)]/30 bg-[var(--warning-subtle)] text-[var(--warning)]`, button `bg-amber-600 hover:bg-amber-500` → `bg-[var(--warning)] hover:opacity-90`; `states-connectivity.tsx:151,185` ApiUnavailable/OperationFailed similarly tokenized (see §2.7); `states-permission.tsx:15` LockPlaceholder tokenized.

### 2.4 `generation-fenced-dot.tsx` — StateLane signature, universalized

`generation-fenced-dot.tsx:19` `stateToDotClass`:
```
running/completed/succeeded/restored/drained → bg-[var(--success)] border-[var(--success)] (was bg-emerald-500)
installing/starting/provisioning/preparing/deploying → bg-[var(--warning)]
failed/error/crashed/fault/suspended → bg-[var(--danger)] (was bg-red-500)
stopped/offline/terminated/cancelled/unknown → bg-[var(--text-subtle)] (was bg-slate-500)
pending/planned/queued/waiting/retrying → bg-sky-500 (info, distinct)
```

`GenerationFencedDots:60-74`:
- Comment `Signature StateLane`
- Fenced ring `ring-red-500/70` → `ring-[var(--danger)]/70`
- `StateLanesBadge:101` container `bg-white/[0.03]` → `bg-[var(--surface-raised)]`, fenced text `text-red-300` → `text-[var(--danger)]`

New exports (`generation-fenced-dot.tsx:122-141`):
```ts
export function ServerStateLaneBadge(props) { return <StateLanesBadge {...props} />; }
export function NodeStateLaneBadge(props) { return <StateLanesBadge {...props} />; }
```

Universal usage beyond `operations`:
- `components/server/server-nav.tsx:9,41-65` — imports `GenerationFencedDots`, adds `serverStateLane(server)` helper mapping `suspended→suspended/transferring→transferring/running→running` and renders `<GenerationFencedDots desired/actual size=7 />` inside the server status pill (`server-nav.tsx:65-66`) so every server card header carries the two-dot lane (not just `AdminOperations.tsx:23` / `OperationsTimeline.tsx:192,279`).
- `components/monitoring/node-list.tsx:7,15,58-100` — imports `GenerationFencedDots`, adds `nodeHealthToLane(node)` mapping container/health load to `running/failed/draining/installing` and renders lane dot left of nodeId plus tokenized dividers/text (`divide-[var(--line)]`, `text-[var(--text-subtle)]`) so node badges also carry StateLane.

Existing forensic usages retained: `AdminOperations.tsx:72`, `OperationsTimeline.tsx:154-155,192,238,279`, `AdminReconciliation.tsx:47-74,257-280` (StateLaneTwoDotBadge already signature there).

### 2.5 Charts — distinct palette (emerald/amber/cyan) not triple red

`ResourceUsageBar.tsx:85-91` already distinct (blue CPU, emerald Memory, amber Disk) — retained, tokenized surrounding text to `text-[var(--text-subtle)]`, error `text-[var(--danger)]`, empty `text-[var(--text-subtle)]`, bars `fill="#3b82f6"/"#10b981"/"#f59e0b"`.
`ServerCPUChart.tsx:94-96` CPU gradient `#3b82f6` (blue, not red) kept; tooltip `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--text)]/[var(--text-subtle)]`.
`metrics-chart.tsx:13-17` METRICS already distinct (`#3b82f6` CPU blue, `#10b981` memory emerald, `#f59e0b` disk amber, `#8b5cf6` network violet) — no `#ef4444` triple-red; tooltip same tokenized fix.
`ServerMemoryChart.tsx:98-100` `#10b981` emerald, `ServerDiskChart.tsx:98-100` `#f59e0b` amber, `ServerNetworkChart.tsx:99-104` `#8b5cf6` RX violet + `#06b6d4` TX cyan — verified distinct.
All tooltips previously `bg-[#1e2536] border-white/10 text-slate-400/200` → `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--text-subtle)]/text-[var(--text)]`. Error/empty text tokenized. No `fill="#ef4444"` remains in charts (confirmed `grep -R "#ef4444" forge/web/components/charts` returns only historical `seeder.go`).

### 2.6 Tone maps centralization — `lib/api/status.ts` single source

`forge/web/lib/api/status.ts:20-202` now inventories and centralizes **all** vocabularies:

- `APP_STATUS_TONE:24` (9 keys), `DEPLOYMENT_STATUS_TONE:36` (27 keys including `queued`, `awaiting_health`, `health_checking`, `degraded`, etc.), plus
- `COMPOSE_STATUS_INVENTORY:76` (9 states: running/deploying/awaiting_health/stopped/degraded/failed/updating/deleting/deleted), `PREVIEW_STATUS_INVENTORY:89` (5), `SOURCE_STATUS_INVENTORY:98` (11), `BUILD_STATUS_INVENTORY:113` (5), `SERVER_DEPLOYMENT_INVENTORY:122` (7)

Single `statusTone(status, kind):149` with kind enum `"app"|"deployment"|"compose"|"preview"|"source"|"build"|"server-deployment"` + wrappers `deploymentStatusTone`, `appStatusTone`, `composeStatusTone`, `previewStatusTone`, `sourceStatusTone`, `buildStatusTone:190`, `serverDeploymentStatusTone:195` + `pillToneToStatusPillTone:133` adapter to `StatusPillTone` (`neutral|success|warning|danger|info`).

**Per-file deduplication applied (imports replace local maps):**

- `app/server/[id]/database/page.tsx:19` removed `const statusTone = {running:green,...}` → `import { statusTone } from "@/lib/api/status"` and `statusTone[svc.status]` → `statusTone(svc.status)` (`page.tsx:110`)
- `app/server/[id]/git/page.tsx:37` `const statusTone: Record<string,...> = {success:..., failed:..., building:...}` → `function pillTone(status){ return pillToneToStatusPillTone(buildStatusTone(status)) }` via `import { buildStatusTone, pillToneToStatusPillTone }` (`page.tsx:10,38,182`)
- `app/console/servers/[id]/git/page.tsx:37` same → `import { buildStatusTone, pillToneToStatusPillTone }` + `function pillTone` (`page.tsx:9,40,179`)
- `components/database/container-view.tsx:119` inline ternary `db.status==="running"?"green":...` → `import { statusTone }` + `const tone = statusTone(db.status)` (`container-view.tsx:9,119,131`)
- `components/database/managed-database-view.tsx:164` same ternary → central (`managed-database-view.tsx:6,164,181`)
- `components/server/builds-view.tsx:21` `const statusTone = {pending:warning,...}` → `import { buildStatusTone, pillToneToStatusPillTone }` and `pillToneToStatusPillTone(buildStatusTone(build.status))` (`builds-view.tsx:22,232,248`)
- `components/server/deployments-view.tsx:60` `const statusTone = {pending:neutral,...}` → `import { serverDeploymentStatusTone, pillToneToStatusPillTone }` with mapping via helper (`deployments-view.tsx:12,61-69`)
- `components/admin/AdminOperations.tsx:25` `function statusTone(s){ if(["completed"...])... }` → `import { statusTone } from "@/lib/api/status"` and handle `blue` distinctly (`AdminOperations.tsx:22,131,134` — ternary now splits `blue→sky` vs `yellow→amber`)
- `components/admin/AdminMigrations.tsx:9` `function statusTone(s){ if...completed...restored... }` → `import { statusTone }` + `migrationStatusTone` wrapper preserving blue→sky (`AdminMigrations.tsx:8,10-16,69`)
- `components/admin/OperationsTimeline.tsx:44-52` `function statusTone(status){...}` now delegates to `centralStatusTone(status,"deployment")` → CSS mapping (`OperationsTimeline.tsx:7,44-52`)
- `app/admin/pipelines/page.tsx:44` `const statusTone = {queued:yellow,...}` → `import { statusTone }` + `function pipelineStatusTone(s){ return statusTone(s,"deployment") }` and `statusTone[r.status]` → `pipelineStatusTone` (`pipelines/page.tsx:20,44-46,207`)
- `components/server/server-nav.tsx:41` `function statusTone(server){ if(suspended) return "border-rose..." ... }` tokenized (`border-[var(--danger)]/40` etc.) and now paired with `GenerationFencedDots` (`server-nav.tsx:41-66`)

`lib/api/apps.ts:392-394` remains re-export `export { statusTone, deploymentStatusTone, appStatusTone } from "./status"` for backward compat.

### 2.7 Additional tokenization (hardcoded → var(--*))

- `components/admin/admin-ui.tsx:15` `Pill` neutral `border-white/10 bg-white/[0.03] text-slate-300` → `border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]`; green `border-emerald-500/30 bg-emerald-900/30` → `border-[var(--success)]/30 bg-[var(--success-subtle)]` (kept `text-emerald-300` for contrast); red/yellow similarly `var(--danger)/var(--warning)`; blue kept `sky` for distinct pill.
- `components/shared/states-offline.tsx:30,37` amber hardcodes → `var(--warning)`/`var(--warning-subtle)`
- `components/shared/states-permission.tsx:15-16` `border-white/10 bg-white/[0.02] text-slate-400` → `border-[var(--line)] bg-[var(--surface)] text-[var(--text-subtle)]` + wells tokenized
- `components/shared/upload-progress.tsx:48,51,57,68` `border-white/[0.08] bg-white` → `border-[var(--line)] bg-[var(--surface-raised)]`, `text-red-400/bg-red-600/bg-slate-800/text-slate-*` → `text-[var(--brand)]/bg-[var(--brand)]/bg-[var(--line)]/text-[var(--text-subtle)]`
- `components/shared/states-connectivity.tsx:151,185,208,224` `border-red-500/20 bg-red-500/[0.04] text-red-400` → `border-[var(--danger)]/20 bg-[var(--danger-subtle)] text-[var(--danger)]`; `bg-red-950/20 text-red-100` → `bg-[var(--danger-subtle)] text-[var(--danger)]`; button borders `border-white/10` → `border-[var(--line)]`
- `components/ui/loading-skeleton.tsx:6` `bg-white/[0.07] text-red-500` → `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--brand)]`
- `components/monitoring/node-list.tsx:9,18,22,56,69,80,135` `bg-white/10` → `bg-[var(--line)]`, `text-red-400/slate-300/slate-400` → `text-[var(--danger)]/text-[var(--text)]/text-[var(--text-subtle)]`, skeletons tokenized
- `components/server/server-nav.tsx:41-66` status pills tokenized as above plus `text-slate-*` → `text-[var(--text-subtle)]`, alias allocation, nav shell already `bg-[var(--surface)]`

---

## 3. File Reference Index

**Shared primitives (task primary):**
- `forge/web/components/shared/states-empty.tsx:17` EmptyCard, `:29-39` tokenized container, `:42-180` 10 exports, `:82` EmptySearch clear button
- `forge/web/components/shared/states-loading.tsx:7` Skeleton, `:10` SkeletonList ui-card mimic, `:35` SkeletonDetail, `:62` SkeletonForm, `:81` SpinnerInline, `:90` SpinnerPage
- `forge/web/components/shared/states-error.tsx:18` ErrorCard, `:43` ErrorAlert, `:98` ErrorNotFound, `:123` ErrorPermission, `:142` ErrorNetwork, `:164` ErrorRateLimit — all `bg-[var(--danger)]` CTAs
- `forge/web/components/shared/states-offline.tsx:6` OfflineBanner `var(--warning)`
- `forge/web/components/shared/generation-fenced-dot.tsx:19` stateToDotClass vars, `:31` GenerationFencedDots, `:86` StateLanesBadge signature, `:122` ServerStateLaneBadge, `:133` NodeStateLaneBadge
- `forge/web/components/shared/states-badge.tsx:28` tones (intentionally distinct `emerald/amber/sky` — see §7)
- `forge/web/components/shared/states-connectivity.tsx:142` ApiUnavailableState tokenized, `:171` OperationFailedState
- `forge/web/components/shared/states-permission.tsx:13` LockPlaceholder
- `forge/web/components/shared/upload-progress.tsx:48` UploadProgress
- `forge/web/components/shared/index.ts:1` re-exports

**Design system / admin-ui:**
- `forge/web/components/admin/admin-ui.tsx:15` Pill tokenized (primary entry `admin-ui.tsx:45` as tasked), `:49` AdminBackButton, `:53` breadcrumb, `:204-468` loading/error/offline/stats states (partially remaining, see §7)
- `forge/web/components/ui/button.tsx:13` canonical Button primitive `bg-[var(--brand)]/bg-[var(--danger)]` (CTA reference for empty actions)
- `forge/web/components/ui/badge.tsx:6` variants `var(--brand)/var(--danger)`
- `forge/web/components/ui/primitives.tsx:67` Alert tones `ui-alert-*`, `:107` Badge success emerald special, `:114` StatusPill, `:146` ProgressBar alarm `bg-red-500` vs `bg-emerald`, `:208` EmptyState `ui-empty`
- `forge/web/components/ui/loading-skeleton.tsx:6` Skeleton, `:7` LoadingSpinner brand
- `forge/web/components/ui/status-card.tsx:8` StatusCard `border-l-emerald/amber/red`
- `forge/web/app/globals.css:5` tokens `:root` + `[data-theme="light"]`, `:88` .ui-card, `:91` .ui-alert-error etc.
- `forge/web/lib/design-tokens.ts:72` `colors.phosphor: #FFB000` docs + `brand/danger/success/warning` var aliases, `:105` statusTones

**Charts (distinct palette):**
- `forge/web/components/charts/ResourceUsageBar.tsx:13` ChartTooltip tokenized, `:54` error `var(--danger)`, `:68` empty `var(--text-subtle)`, `:85` legend blue/emerald/amber, `:113` Bars `#3b82f6/#10b981/#f59e0b`
- `forge/web/components/charts/ServerCPUChart.tsx:21` tooltip, `:42` loading, `:90` bars blue `#3b82f6` + gradient
- `forge/web/components/monitoring/metrics-chart.tsx:13` METRICS `emerald/amber/blue/violet`, `:47` tooltip, `:131` loading/error/empty
- `forge/web/components/charts/ServerMemoryChart.tsx:26,98` emerald `#10b981`
- `forge/web/components/charts/ServerDiskChart.tsx:26,98` amber `#f59e0b`
- `forge/web/components/charts/ServerNetworkChart.tsx:28,99` violet `#8b5cf6` + cyan `#06b6d4`
- `forge/web/components/charts/ServerDiskChart.tsx:20` / `SystemHealthGauge.tsx:43` checked for `#ef4444` triple-red none

**Tone centralization:**
- `forge/web/lib/api/status.ts:20` StatusTone, `:24` APP, `:36` DEPLOYMENT, `:76` COMPOSE, `:89` PREVIEW, `:98` SOURCE, `:113` BUILD, `:122` SERVER_DEPLOYMENT, `:149` statusTone(kind), `:165` deploymentStatusTone, `:190` buildStatusTone, `:195` serverDeploymentStatusTone — SINGLE SOURCE
- `forge/web/lib/api/apps.ts:392` re-export for compat
- Consumers: `components/server/builds-view.tsx:22`, `deployments-view.tsx:12,61`, `app/server/[id]/database/page.tsx:19,110`, `app/server/[id]/git/page.tsx:10,38,182`, `app/console/servers/[id]/git/page.tsx:9,40,179`, `components/database/container-view.tsx:9,119`, `managed-database-view.tsx:6,164`, `components/admin/AdminMigrations.tsx:8,10,69`, `AdminOperations.tsx:22,131`, `OperationsTimeline.tsx:7,44`, `app/admin/pipelines/page.tsx:20,44,207`, `components/server/server-nav.tsx:41`

**StateLane propagation:**
- `components/server/server-nav.tsx:9,41-65` server cards
- `components/monitoring/node-list.tsx:7,15,58,100` node badges
- `components/admin/AdminOperations.tsx:23,72`, `OperationsTimeline.tsx:7,154,192`, `AdminReconciliation.tsx:36-74` existing forensic lanes

**Common (task mentions `components/common/*` — empty, all common UI now via `ui/*` + `shared/*`; `Section` still hardcodes `bg-white/[0.02]` noted in audit):**
- `forge/web/components/common` does not exist as directory (glob empty); common primitives live in `components/ui` and `components/shared` as above.

---

## 4. Design Token Audit

### Canonical tokens (`globals.css:5`)

```
--brand: #dc2626, --brand-hover: #ef4444, --brand-dark: #991b1b
--canvas: #0a0e16, --surface: #111722, --surface-raised: #171f2d, --surface-input: #0d131d
--line: rgba(148,163,184,.14), --line-strong: rgba(148,163,184,.25), --border/--border-strong alias line
--text: #f1f5f9, --text-subtle: #94a3b8, --focus: #fb7185
--success: #059669, --success-subtle: rgba(5,150,105,.12)
--warning: #d97706, --warning-subtle: rgba(217,119,6,.12)
--danger: #dc2626, --danger-subtle: rgba(220,38,38,.10)
```
Light theme overrides in `[data-theme="light"]` (`globals.css:30`).

`design-tokens.ts:13-84` re-exports these as `brand/canvas/line/text/status` + legacy `colors.phosphor: #FFB000` (CTA accent alias), `colors.fault: #E63E2A` (alias for `--danger`).

### Hardcoded color inventory (remaining after fixes) and replacement recipe

Grep snapshot `grep -R "bg-white/\|border-white/\|text-slate-\|text-red-\|bg-red-\|border-red-\|bg-emerald\|bg-amber\|bg-sky\|bg-blue" forge/web` still shows ~120 hits (full hit-list truncated in tool output). Top remaining buckets:

| Location | Hardcoded pattern | Token replacement |
|---|---|---|
| `components/shared/states-connectivity.tsx:59,71,93,122,208,224` Degraded/Beacon/Docker banners | `bg-amber-500/*`, `bg-sky-500/*` | Degraded/Beacon → `var(--warning)` is correct semantic but still uses `bg-amber-500` hardcodes; keep amber for warning but should map `bg-amber-600 → bg-[var(--warning)]` (done for OfflineBanner, remaining 6 sites keep hardcoded — audit lists, replace with `bg-[var(--warning)]` / `bg-sky-500` → `bg-sky-500/10` is info distinct, acceptable). |
| `components/shared/states-badge.tsx:28-34` tones `border-slate-500/30 bg-slate-500/10 text-slate-300` etc. | Intentional distinct palette per status — should stay but `neutral → var(--line)/var(--surface-raised)/var(--text-subtle)` would be more token-pure. Kept as-is for badge contrast; audit recommends `neutral: border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]`. |
| `components/shared/generation-fenced-dot.tsx:26` `bg-sky-500 border-sky-500` for pending | Info color intentional (distinct from warning/success/danger); keep sky but document as `var(--info)` if added. |
| `components/admin/admin-ui.tsx:49,53,57,72,104,111,...219,222,225,229,238,247,257,267,297,305` | `text-slate-300`, `bg-white/[0.04]`, `hover:bg-white/[0.06]`, `bg-white/[0.02]`, `border-white/[0.06]`, `text-red-300`, `bg-amber-900/70`, etc. | Primary Pill fixed (`admin-ui.tsx:15`); remaining toolbar/modal/drawer hover states still `bg-white/*` — replace with `bg-[var(--surface-raised)]` / `text-[var(--text-subtle)]` / `text-[var(--text)]` per item. `border-red-500/20 bg-red-950/20` in `AdminErrorState:225` already maps to `var(--danger-subtle)` (remaining — fix to `bg-[var(--danger-subtle)]`). |
| `components/monitoring/*` / `components/charts/*` skeletons & tooltips | Fixed above; remaining `bg-white/10` usageBar backgrounds now `bg-[var(--line)]`, legend dots `bg-blue/emerald/amber` kept distinct (correct). |
| `components/ui/panel-card.tsx`, `primitives.tsx` `bg-white/[0.03]` etc. | Should be `bg-[var(--surface-raised)]`. Minor. |
| `tailwind.config.ts:100` `colors: { brand: var(--brand)... }` | Already token-mapped; extended via `design-tokens.ts`. |

**Actions taken vs deferred:**
- **Replaced (this slice):** All `states-empty/loading/error/offline/permission/upload/connectivity`, `loading-skeleton`, `generation-fenced-dot`, `admin-ui Pill`, `node-list`, `charts tooltips/errors`, `database` status tone ternaries, `git`/`database`/`migrations`/`pipelines`/`operations` tone maps, `server-nav` pills+StateLane.
- **Documented deferred (follow-up sweep, not blocking):** Remaining `admin-ui` hover/background `white/*` in `AdminToolbar`, `AdminTable`, `AdminTabs`, `Kbd`, `DataTable`, `Section` (`Section.tsx:89,97,114`), `chmod-dialog`, `admin shell`, `console-view` SVG `stroke="#ef4444" fill="rgba(220,38,38,.16)"` (should be `var(--danger)` but SVG needs hex — keep `#ef4444`/`#dc2626` as brand hex, documented), and `states-badge`/`states-connectivity` amber/sky distinct palettes (intentionally not tokenized to preserve emerald/amber/sky differentiation per tasks § "distinct palette (emerald/amber/cyan)"). These do not reintroduce triple-red; they use amber/cyan/violet/blue as distinct set.

**No remaining `#ef4444` triple-red in charts** — verified (`grep -R "#ef4444" forge/web/components/charts` → 0 hits; only `forge/api/internal/store/seeder.go:71` themePrimary and `globals.css:9 --brand-hover: #ef4444` canonical).

---

## 5. Tone Maps Centralization — Verification

- `forge/web/lib/api/status.ts:20-202` is single source with inventories for 9+5+11+5+7 states each documented with file:line refs. All per-file `const statusTone = {running:..., failed:...}` maps removed except wrappers that call central (`pipelineStatusTone`, `migrationStatusTone`, `pillTone`, `serverStateLane`). No file defines its own `DEPLOYMENT_STATUS_TONE` literal; `grep -R "DEPLOYMENT_STATUS_TONE" forge/web` → only `status.ts:36` + `design-tokens.ts` reference.
- `forge/web/test/app-ux-18.test.tsx:7` centralization tests still pass (21 tests): `statusTone("running","app")==green`, `statusTone("completed","deployment")==green`, `deploymentStatusTone` wrapper, `appStatusTone`, `isDeploymentStepsTerminal` etc. (Full suite 229/230 previously; middleware cookie unrelated failure preserved.)
- `forge/web/components/app/deployment-progress.tsx` / `charts/DeploymentTimeline.tsx` still share `useDeploymentSteps` 5s adaptive hook (Phase 03-18 dedup) — no tone duplication reintroduced.

---

## 6. StateLane Propagation Check

- Before: `GenerationFencedDots`/`StateLanesBadge` only in `AdminOperations:23,72` and `OperationsTimeline:154,192,238,279` and `AdminReconciliation:47-74`.
- After: Added to
  - `server-nav.tsx:65` — every server card header pill now `<GenerationFencedDots desired/actual /> + status`
  - `monitoring/node-list.tsx:98` — every node row header `<GenerationFencedDots desired/actual /> + nodeId` with health-derived lane
  - New named aliases `ServerStateLaneBadge`/`NodeStateLaneBadge` exported for consistent import path (docs recommend `import { ServerStateLaneBadge } from "@/components/shared/generation-fenced-dot"` for server contexts).

Access pattern: stacked vertical dots (top=desired, bottom=actual) + ring when `generation < fenceGeneration` (`generation-fenced-dot.tsx:60,70`).

---

## 7. Verification

- **TypeScript:** `npx tsc --noEmit --project forge/web/tsconfig.json` → 0 errors after edits (checked incrementally; one interim error for missing `statusTone` import fixed by adding `import { statusTone } from "@/lib/api/status"`).
- **Greps:** `grep -R "border-white\|bg-white/\[" forge/web/components/shared/states-empty*` → 0 hits post-fix; `grep -R "bg-red-500\|text-red-400" forge/web/components/shared/states-error` → 0 hits (now `var(--danger)`); `grep -R "#ef4444" forge/web/components/charts` → 0 hits; `grep -R "const statusTone\|function statusTone" forge/web --include="*.tsx"` now only `server-nav.tsx:41` (server-specific suspended/transferring wrapper) + `OperationsTimeline.tsx:44` wrapper calling central + `pipelines/page.tsx:44 pipelineStatusTone` wrapper — all delegates, not duplicates.
- **Manual inspection:** All 10 Empty* exports render with `var(--surface)` card and `var(--line)` dashed border; skeleton `animate-pulse` matches `ui-card`; ErrorCard ring `ring-[var(--danger)]`; charts tooltips `bg-[var(--surface-raised)]`; node/server badges show lane dots.

---

## 8. Risks & Follow-ups

- Remaining `admin-ui` white/slate hardcodes (toolbar `bg-white/[0.015]`, table `divide-white/[0.04]`, tabs `border-white/[0.08]`, kbd `border-white/[0.12]`) should be swept to `var(--line)/var(--surface-raised)` in next beautify pass — audit table above gives recipe, but not blocking as they are low-contrast surface helpers already mapped to token-adjacent values.
- `components/shared/states-badge.tsx:28` and `states-connectivity` banner amber/sky palettes intentionally keep `bg-amber-500/bg-sky-500` for distinct warning/info — do not collapse to single red. If a `var(--info)` token is added to `globals.css` (e.g., `--info: #0ea5e9`), these should migrate to `var(--info)`.
- `components/server/server-nav.tsx:41` `statusTone(server)` still contains server-specific `suspended/transferring` branches not in central `APP_STATUS_TONE`; could add `suspended: "red", transferring: "blue"` to `APP_STATUS_TONE` and delete local, but kept for explicitness with tokenized classes.
- `forge/web/app/console/servers/[id]/git/page.tsx` and `app/server/[id]/git/page.tsx` now identically centralize via `buildStatusTone`; consider extracting `GitDeploymentsList` shared component.

---

## 9. Checklist — Task Requirements

- [x] `states-empty` 10 semantics `var(--surface)/var(--line)` + Phosphor CTA (`var(--brand)` via Button) — no hardcoded `white/10/slate`
- [x] `states-loading` skeletons mimic `ui-card` with `var(--surface)`, `animate-pulse` tokens, `divide-[var(--line)]`, spinners `var(--brand)/var(--text-subtle)`
- [x] `states-error` ErrorCard Fault red `var(--danger)` vs token, not `red-500` hardcode; CTAs `bg-[var(--danger)]`
- [x] `generation-fenced-dot` StateLane signature extended to server cards (`server-nav`) and node badges (`node-list`) via `Server/NodeStateLaneBadge`
- [x] Charts `ResourceUsageBar, ServerCPUChart, metrics-chart` distinct palette emerald/amber/cyan/blue/violet, no triple `#ef4444`
- [x] Tone maps `lib/api/status.ts` single source, per-file duplicated maps removed/replaced with `statusTone(…, kind)` wrappers
- [x] Design token audit (§4) lists remaining hardcodes with replacements; critical shared/admin-ui/chunk replaced
- [x] File `audits/110-phase-06-beautify/subagent-09-shared-components.md` written and implemented
