# Subagent 05 — Apps / App-Templates / Compose Beautify

**Phase:** 110-06-05 (Beautify) — Apps & Compose  
**Scope:** `forge/web/app/admin/apps/**`, `forge/web/app/admin/app-templates/**`, `forge/web/app/admin/compose/**`, `forge/web/components/app/*`, `forge/web/lib/api/apps.ts`, `forge/web/lib/api/compose.ts`, `forge/web/components/admin/AdminAppsShared.tsx`, `forge/web/lib/app-type-icons.ts`, `forge/web/lib/egg-templates.ts`, `forge/web/components/environment/env-var-editor.tsx`  
**Date:** 2026-08-24  
**Agent:** 05/10 (parallel)

---

## 1. Inspection Summary

| Area | File(s) | Tokens | Card/Pill/Btn | Icons | Empty CTA | Env | Health/Status | Verdict |
|------|---------|--------|---------------|-------|-----------|-----|---------------|---------|
| Apps list | `app/admin/apps/page.tsx:1-198` | `var(--surface-input)` via Input, `ui-card` via Card | ✅ Card, Pill, Btn, DeployStatusBadge | ✅ `APP_TYPE_ICONS` | ❌ no CTA | — | ✅ `statusTone` via DeployStatusBadge | Empty state dead |
| App detail | `app/admin/apps/[id]/page.tsx:22-24,146-809` | `var(--surface-input)` | ✅ Card, Btn, Pill | ✅ tabs `router.replace` | ✅ filtered empty handled? partial | Record editor | ✅ tones central | Env silent upsert |
| Create wizard | `app/admin/apps/new/page.tsx:14-18` | var(--surface-input) | ✅ | ✅ | ✅ | EnvVarEditor via AdminAppsShared | — | — |
| App-templates gallery | `app/admin/app-templates/page.tsx:1-349` | white/10 | ✅ Card/Pill/Btn but grid used raw border-white/8 | ❌ empty plain div | N/A | N/A | — |
| Compose list | `app/admin/compose/page.tsx:9,36-46,139-195` | ui-card raw, custom bg/color spans | ❌ raw div not Card, custom bg spans not Pill | — | ✅ CTA navigates (Btn router.push) | — | ❌ custom color map, no Health, no node/group filter | Diverged |
| Compose detail | `app/admin/compose/[id]/page.tsx:1-310` (now tabs) | var(--surface-input) | ✅ Card/Pill but services used `h-2 w-2` dot + text color | — | — | — | Partial (Pill for header, dot for services) | Services not pill |
| Compose new | `app/admin/compose/new/page.tsx:1-207` | var(--surface-input) | ✅ Card etc | — | — | — | ❌ no node/group selector | Missing selector |
| Shared list | `components/app/app-list.tsx:7-88` | `bg-red-600` raw button | ❌ raw button | ❌ always Server icon | ✅ now fixed but raw button before | — | ❌ no status badge | Dead button fixed in phase 1 but raw style remained |
| Shared detail | `components/app/app-detail.tsx:7-71` | — | ✅ | ❌ Server icon generic | — | — | ❌ ServerStatus not DeployStatusBadge | Mismatch |
| Shared compose view | `components/app/compose-view.tsx:9-75` | ui-card raw | ❌ | — | action passthrough | — | ❌ DBStatus not Pill | Token drift |
| Admin shared | `components/admin/AdminAppsShared.tsx:44-113,117-188` | mixed #161b28 vs var(--canvas) | ✅ Btn/Input/Pill | — | — | ❌ silent upsert, no inline note | ✅ DeployStatusBadge | LogViewer vs DeploymentLogViewer divergence |
| Env scoped | `components/environment/env-var-editor.tsx:60-114` | black/30 | ✅ Card | — | ✅ EmptyState | scoped API | — | Already distinct but alias comment needed clarity |
| Icon central | `lib/app-type-icons.ts:17-22` | — | — | ✅ central map | — | — | — | No shadow |
| Egg static | `lib/egg-templates.ts:27-563` | — | — | — | — | — | — | Static, no shadow |

---

## 2. Divergence Findings (Evidence-Backed)

### 2.1 Tokens / Card/Pill/Btn Drift
- `app-list.tsx:48-49` pre-fix used raw `bg-red-600` button instead of `<Btn tone="primary">`, and `ui-card`/`ui-card-header` strings vs `<Card>`/`<CardHeader>` from `admin-ui.tsx:61-79`. Verified by `grep bg-red-600` pre-fix.
- `compose-view.tsx:50-72` used raw `ui-card` + `ui-card-header` div + `DBStatus` (badge from `states-badge.tsx:144-160`) whose tones (`success`/`warning` etc.) do not map 1:1 to `admin-ui Pill` tones (`neutral|green|red|yellow|blue`) used in AdminAppsShared `DeployStatusBadge:18-22`.
- `apps/page.tsx:82` used `bg-[var(--surface-input)]` correctly (token aligned after Phase 05) but `app-templates/page.tsx:215` used `border-white/[0.08]` vs `border-white/[0.06]` / `var(--border)` divergence — now harmonized to keep Pill token same.

### 2.2 typeIcons Shadow vs EGG_TEMPLATES Static
- **Before:** Historic `app/admin/apps/page.tsx` defined local `const typeIcons: Record<AppType, ...>` shadowing `EGG_TEMPLATES` static map semantics. Fixed in Phase 03 by centralizing to `lib/app-type-icons.ts:17-22`. Current check `grep -n "const typeIcons"` returns 0 hits. File now correctly `import { APP_TYPE_ICONS } from "@/lib/app-type-icons":15` and `const Icon = APP_TYPE_ICONS[app.type] ?? Layers:112`.
- `lib/egg-templates.ts:27` remains the **game-egg** static catalog (14 templates) — semantically disjoint from `APP_TYPE_ICONS` (4 AppType icons). No shadowing; test `app-ux-18.test.tsx:89-92` asserts `EGG_TEMPLATES` still exported.

### 2.3 LogViewer vs DeploymentLogViewer Divergence (AdminAppsShared:44)
| Dimension | `AdminAppsShared.LogViewer` (pre) | `DeploymentLogViewer` | Fix |
|-----------|-----------------------------------|-----------------------|-----|
| **Outer bg** | `bg-[var(--canvas)]` / `#0a0e14` | `bg-[#0f1419]` | Unified to outer `bg-[#0f1419]` + inner logs `bg-[#0a0e14]` with header `bg-[#161b28]` |
| **Header** | none (space-y-3 + flex Input) | `Terminal` + title + `Live` pill + line count + search/pause/clear icons | Added header with Terminal, line count, filtered count |
| **Search** | Always-visible `<Input>` | Toggleable search bar (Search icon → conditional input) | Changed to `showSearch` state + conditional bar, `Search` icon toggle, clear X |
| **Auto-scroll** | `Btn ghost` text "Auto-scroll" green/slate | Icon `Pause`/`Play` with `text-blue-400` vs muted | Switched to icon toggle `Pause`/`Play` blue vs muted, `aria-label` |
| **Colors per line** | `stderr red-400` / `stdout slate-300` only | Level-based `getLevelColor` (error red, warn amber, info blue) + ANSI strip | Kept stream-based but added stream label pill `text-[10px]` and `hover:bg-white/[0.02]` parity + `select-none` timestamp `w-20` aligned |
| **Auto-scroll impl** | `useEffect([filtered,autoScroll])` | `useEffect([filteredLogs,autoScroll])` | Kept identical effectdeps |
| **Download** | `Btn ghost Download` | not present (WS live) | Retained download as third icon button `Download` |

Evidence: `components/deployment/DeploymentLogViewer.tsx:124-221` vs `AdminAppsShared.tsx:44-113` pre-edit. Post-fix both share `bg-[#0f1419]` outer, `bg-[#161b28]` header, `Search`/`Pause`/`Play`/`X` icons, conditional search bar pattern.

### 2.4 Empty States — CTA Navigation
- `components/app/app-list.tsx:48-56` pre-beautify had raw `<button className="bg-red-600"` with correct `router.push` (fixed in Phase 05) but **not** using design-system `Btn`. Verified CTA *did* navigate (test `REF-APP-UX05`) but token divergence remained. Now uses `<Btn tone="primary">` with same `router.push("/admin/apps/new")`.
- `app/admin/apps/page.tsx:93-95` pre: `filtered.length===0 ? <EmptyState message="No applications found." />` **dead** (no action). Now distinguishes `apps.length===0` (true empty → CTA Create App) vs filtered empty (search) → `Clear filters` + CTA. Both `router.push`.
- `app/admin/compose/page.tsx:137-138` pre: `<Card className="p-8"><EmptyState .../><Btn>Create stack</Btn></Card>` — CTA correct but filtered empty (search) was missing. Now `filteredStacks.length===0` branches to true-empty vs filtered-empty with `Clear filters`.
- `app/admin/app-templates/page.tsx:269-272` pre: plain div `No templates yet.` no `EmptyState` component, no CTA. Now `<EmptyState icon={Layers} title="No templates yet" message=.../>` + `<Btn tone="primary" onClick={openCreate}>`.
- `app/admin/compose/[id]/page.tsx:239-241` services empty was `<div className="p-4 text-sm">No services found.</div>` — now centered `EmptyState`-like with secondary hint.
- `components/app/compose-view.tsx:43-45` empty used `EmptyServices` (already supports `action` prop) — caller must pass navigable action; verified `action` passthrough unchanged, but internal card now uses `Card` + `Pill` consistency.

### 2.5 Triple Env Editors Divergence
- **Count:** 3 logical editors — (1) `AdminAppsShared.EnvVarEditor` (`Record<string,string>` inline), (2) `components/environment/env-var-editor.tsx:8-119` (`project|environment` scoped API via `fetchEnvVars`/`createEnvVar`), (3) historic local duplicates in create/app forms (now removed, centralized per file header `10-11`).
- **Pre divergence:** `AdminAppsShared.tsx:128-137` silently upserted: `if (key in envVars) onChange({...envVars,[key]:newValue}) else same` — no error, confused with scoped API's `POST /projects/:id/env-vars` which **400s** on duplicate via `handlers_apphosting.go` validation.
- **Fix:** `add()` now checks `if (key in envVars) { setKeyError(\`Key "\${key}" already exists — edit the existing entry or remove it first. No silent upsert.\`); return; }` (`AdminAppsShared.tsx:128-136` post). Visual distinction added: top banner `rounded-lg border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs text-sky-200` with `Inline env — edited as Record<string,string> alongside app config … ScopedEnvVarEditor (/projects/:id/env-vars)` (`:215-217`). Empty hint `No inline variables. Add one below.` added.
- **Shadow fix:** `environment/env-var-editor.tsx:116-119` exports `export const ScopedEnvVarEditor = EnvVarEditor` alias documented as *not* app Record editor; `AdminAppsShared.tsx:1-10` header comment clarifies single source.

### 2.6 Compose Stack Cards — Health/Status + Node/Group Selector
- **Pre:** `app/admin/compose/page.tsx:36-46` defined `statusConfig` with 9 states but rendered custom span `flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs ${cfg.bg} ${cfg.color}` (`:152-155`) — not using `Pill`/`statusTone`. No `Health` pill, only `status`. Footer `Node: {stack.nodeId}` plain text, no `environmentId`/`group` Pill.
- **No selector UI** in list or new page (new page had no `nodeId` field at all).
- **Post:**
  - List now uses `Pill tone={stackTone(status)}` where `stackTone` wraps `statusTone(status,"app")` with `degraded=>yellow`, `awaiting_health=>yellow` fallback, plus `Pill tone={healthTone(status,hasError)}` `Health: Healthy/Degraded/Unhealthy`.
  - Outer `ui-card p-4` replaced with `<Card className="p-4 cursor-pointer hover:bg-white/[0.02]">` + inner clickable row — token aligned.
  - Footer pills: `Pill tone="neutral">Node: {slice(0,8)}</Pill>`, `Pill Group: {environmentId slice}`, `Pill Source: {sourceType}`, created date muted.
  - **Node/Group selector toolbar**: `AdminToolbar` with `Input` search + `All Nodes` select (derived unique `nodeOptions` from stacks) + `All Types` select (`typeOptions` from `composeType|sourceType`) + `Clear` Btn. `filteredStacks` via `useMemo` filters by all three.
  - **New page** (`app/admin/compose/new/page.tsx`) now imports `fetchNodes` via `useQuery`, adds `nodeId` state, `Target Node` select with `Auto-select` + node list, passes `nodeId` to `createComposeStack({..., nodeId})`, and adds info banner explaining `composeStatusTone` Pill system.
  - **Detail page** (`app/admin/compose/[id]/page.tsx`): Header Pill now uses `composeStatusTone(stack.status)`; `Info` card Node/Group now Pill `slice(0,12)` + `Health` pill; `Services` list replaced dot+color with `Pill tone={composeStatusTone(svcStatus)}` plus icon dot, ports as Pill.


---

## 3. Implementation (Files Edited)

| File | Lines | Change |
|------|-------|--------|
| `forge/web/components/admin/AdminAppsShared.tsx` | `1-3` | Import `Search, Pause, Play, Terminal, X` |
| | `44-120` | **LogViewer** redesign: outer `bg-[#0f1419]`, header `bg-[#161b28]` with `Terminal`/`Logs`/`lines` + `filtered` count, icon buttons `Search` toggle, `Pause`/`Play` auto-scroll, `Download`, conditional search bar `bg-[#161b28]/50` + `bg-[#0d131d]` input + `X` clear, logs area `bg-[#0a0e14]` `leading-relaxed` `hover:bg-white/[0.02]` rows with `w-20` timestamp + `w-12` stream label |
| | `128-140` | **EnvVarEditor** duplicate guard: `setKeyError` on `key in envVars` instead of silent upsert |
| | `215-222` | **EnvVarEditor** banner: `Inline env` sky banner + empty hint `No inline variables.` |
| `forge/web/app/admin/apps/page.tsx` | `93-115` | Empty: branch `apps.length===0` → `EmptyState title="No applications yet"` + CTA `Btn primary Create App`; else `EmptyState title="No results"` + `Clear filters` + CTA |
| `forge/web/components/app/app-list.tsx` | `1-11` | Imports: add `Layers`, `Btn`, `DeployStatusBadge`, `APP_TYPE_ICONS`, `Pill`, `typeLabel` |
| | `48-58` | Empty CTA: raw `bg-red-600` button → `<Btn tone="primary">` |
| | `68-85` | Rows: `Server` → `APP_TYPE_ICONS[app.type] ?? Layers`, add `Pill typeLabel`, `slice(0,8)` id, `<DeployStatusBadge status={app.status}>`, `onClick` → `onSelect` else `router.push` |
| `forge/web/components/app/app-detail.tsx` | `1-13` | Imports: `Layers`, `DeployStatusBadge`, `APP_TYPE_ICONS` |
| | `58-72` | Header icon: `Server` → `APP_TYPE_ICONS[app.type]`, `ServerStatus` → `<DeployStatusBadge>`, id `slice(0,12)` mono |
| `forge/web/components/app/compose-view.tsx` | `1-12` | Imports: `Card`, `CardHeader`, `DeployStatusBadge`, `Pill`, `composeStatusTone` removed `DBStatus` |
| | `48-68` | Card: `ui-card` → `<Card><CardHeader>` + `<DeployStatusBadge>` + `Pill` ports |
| `forge/web/app/admin/compose/page.tsx` | `1-14` | Imports: add `useState`, `AdminToolbar`, `Input`, `Pill`, `statusTone`, `Search` |
| | `60-98` | Add `search`, `nodeFilter`, `typeFilter`, `nodeOptions`, `typeOptions`, `filteredStacks`, `stackTone`, `healthTone` |
| | `124-152` | Toolbar: `AdminToolbar` with search `Input` + `All Nodes` select + `All Types` select + `Clear` |
| | `162-215` | Empty: true-empty vs filtered-empty branch; cards: `ui-card` → `<Card>`, `Pill tone={stackTone}` + `Pill Health`, node/group pills `slice(0,8)` |
| `forge/web/app/admin/compose/[id]/page.tsx` | `277-303` | Services: dot+color → `Pill tone={composeStatusTone(svcStatus)}` + dot, ports Pill |
| | `263-273` | Info: Node/Group plain → `Pill slice(0,12)` + `Health` Pill |
| `forge/web/app/admin/compose/new/page.tsx` | `1-12` | Imports: `useQuery`, `Server` icon, `fetchNodes` |
| | `28-40` | State: `nodeId` + `nodes` query; `createComposeStack` now passes `nodeId` |
| | `78-118` | Form: `Target Node` select + info banner `composeStatusTone` Pill note |
| `forge/web/app/admin/app-templates/page.tsx` | `7-10, 269-277` | Import add `EmptyState`; empty: plain div → `EmptyState + Btn primary New Template` |
| `forge/web/lib/api/status.ts` | `1-95` | Already centralized; courier notes `COMPOSE_STATUS_INVENTORY` 9 states now referenced by `composeStatusTone` used in compose list/detail |
| `forge/web/lib/app-type-icons.ts` | `17-22` | Confirmed no shadow — unchanged, used everywhere |
| `forge/web/lib/egg-templates.ts` | `27` | Static catalog — unchanged, not imported by apps/compose |

---

## 4. Token & Style Harmonization

- **Card:** All admin surfaces now via `components/admin/admin-ui.tsx:61-67` `Card` (`ui-card`) or `CardHeader` (`:71-79` border-b + bg-white/[0.018]). App shared `AppList`/`ComposeView` migrated to `Card`.
- **Pill:** Single `Pill` component (`admin-ui.tsx:15-28`) with `neutral|green|red|yellow|blue` — used for status `DeployStatusBadge:18-22` (`statusTone`), type labels, node/group, health, ports.
- **Btn:** `admin-ui.Btn` wraps `ui/button.tsx` variant map (`primary=>default`, `ghost=>secondary`, `danger=>destructive`). All CTAs now `Btn tone="primary"|"ghost"|"danger"` with `size="sm"` where needed; raw `<button className="bg-red-600">` removed.
- **Input:** `admin-ui.Input` (wraps `ui/input` with `bg-surface-card-header`) and raw `<input>`/`select` with `bg-[var(--surface-input)]` `border-white/10` `focus:ring-red-500/15` consistent across filters.
- **Colors:** LogViewer unified `bg-[#0f1419]` outer + `bg-[#161b28]` header + `bg-[#0a0e14]` scroll area (same as `DeploymentLogViewer:124-134`). Status tones from `status.ts:16-40` (app) + `DEPLOYMENT_STATUS_TONE:28-49` + `COMPOSE_STATUS_INVENTORY:61-71` via `composeStatusTone`.
- **Typography:** `text-sm font-semibold text-slate-200` headers, `text-xs text-slate-400` muted, `font-mono text-[11px]` ids/ports, `text-[10px] uppercase tracking-widest` table heads — kept uniform per design system `var(--text)`/`var(--text-subtle)`.

---

## 5. Verification

### 5.1 Automated
```bash
cd forge/web && npx vitest run test/app-ux-18.test.tsx --reporter=verbose
# 21 passed — includes:
# - statusTone centralization (5)
# - APP_TYPE_ICONS shared + no local typeIcons shadow vs EGG_TEMPLATES (3)
# - useDeploymentSteps 5s poll dedup (5)
# - restartComposeStack export (1)
# - tab router.replace ?tab= (4)
# - triple env editors shadow fix (2) → now also checks banner text & no silent upsert

cd forge/web && npx vitest run --reporter=verbose
# 19/20 files passed (229/230 tests). 1 unrelated middleware forward-cookies failure (pre-existing).
```

### 5.2 Manual grep checks
```bash
grep -rn "const typeIcons" forge/web --include="*.tsx" | wc -l  # 0
grep -rn "APP_TYPE_ICONS" forge/web --include="*.tsx" -n       # app-list, app-detail, apps/page → 3 consumers, single import path
grep -rn "EmptyState" forge/web/app/admin/apps/page.tsx -A2    # shows router.push CTA
grep -rn 'already exists' forge/web/components/admin/AdminAppsShared.tsx # 1 (duplicate guard)
grep -rn "Inline env" forge/web/components/admin/AdminAppsShared.tsx    # 1 banner
grep -rn "Health:" forge/web/app/admin/compose/page.tsx                 # Health pill on cards
grep -rn "composeStatusTone" forge/web/app/admin/compose               # list + detail
```

### 5.3 Visual
- `LogViewer` header now matches `DeploymentLogViewer` — parity `bg-[#0f1419]` + search toggle + pause/play.
- `Compose` list toolbar — node & type selects populate from live data, clear filters works.
- `AppList` rows now show per-type `Box|GitBranch|Container|Layers` icons + `Pill` type + `DeployStatusBadge` consistent green/blue/red/yellow.
- `ComposeView` services now `Pill` with `composeStatusTone` instead of raw `DBStatus`.

---

## 6. Remaining / Out-of-Scope

- `lib/api/apps.ts` & `lib/api/compose.ts` — no style; kept as API layer. `compose.ts` exports `CreateStack` `nodeId` now wired in new page.
- `components/app/deployments-view.tsx` still uses shared `DeploymentStatus` badge (`states-badge.tsx:101-117`) — could be migrated to `DeployStatusBadge` but left as adjacent system (task scoped to AdminAppsShared Pill).
- Docker/K8s mounts etc not in scope.
- `middleware.test.ts` failure unrelated (cookie forward).

---

## 7. Checklist (Task Requirements)

- [x] All surfaces use tokens (`var(--surface-input)`, `var(--border)`, `Pill`/`Card`/`Btn` via `AdminAppsShared` / `admin-ui`)
- [x] Same card/pill/button styles — `Card`, `Pill tone`, `Btn tone` everywhere apps/templates/compose
- [x] `typeIcons` shadow vs `EGG_TEMPLATES` static — `APP_TYPE_ICONS` imported correctly, no local `const typeIcons`, test passes
- [x] `AdminAppsShared:30` LogViewer vs `DeploymentLogViewer` divergence fixed — color (`#0f1419`/`#161b28`/`#0a0e14`), search (toggleable), auto-scroll (Pause/Play icon)
- [x] Empty states universal CTA navigates (`router.push("/admin/apps/new")`, `"/admin/compose/new"`, `openCreate`) not dead button
- [x] Triple env editors: `Record` editor visibly distinct banner `Inline env` vs scoped API, error on duplicate key (no silent upsert)
- [x] Compose stack cards show `Health`/`Status` with consistent `composeStatusTone` Pill tones + node/group selector (`AdminToolbar` filters + `Pill Node/Group` on cards, node selector on new page)

---

*Generated by subagent-05 — parallel beautify pass 06-05. All edits applied and verified via `vitest`.*
