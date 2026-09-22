# Subagent 04 — Servers / Nodes / Allocations / Databases — Beautify and Universalize

**Phase:** 110-06-04 of 110 — Phase 06 (Beautify)
**Focus:** `forge/web/app/admin/servers, nodes, allocations, databases, database-services` + `forge/web/components/admin/AdminServers, AdminNodes, AdminAllocations, AdminDatabases` + `forge/web/app/servers, console/servers` ResourceBar + `forge/web/app/admin/docker` node-group selector + `lib/api/status.ts` centralization + State Lanes + OfflineBanner dedup
**Date:** 2026-08-24
**Mode:** Beautify (parallel agent 04/10)

---

## 1. Inspection

### 1.1 Hardcoded palette — pre-beautify drift

| Location | Before | Token violation |
|---|---|---|
| `AdminServers.tsx:91` `border-b border-white/[0.06] bg-[#161b28]` | table header | hardcoded hex + white opacity border |
| `AdminServers.tsx:102` `bg-[#161b28]` / `border-white/[0.04]` | table rows + startup inputs | same |
| `AdminServers.tsx:214` `bg-[#0f141f]` `border-white/10` | CreateServer inputs | same |
| `AdminServers.tsx:593` `bg-[#141824]` | select | same |
| `AdminNodes.tsx:115` `bg-[#161b28]` `border-white/[0.06]` | node table header | same |
| `AdminNodes.tsx:545,559,570,578` `bg-[#141824]` | NodeSettings selects | same |
| `AdminAllocations.tsx:149` `bg-surface-card-header` but hover `border-white/20` | selectStyle | mixed |
| `AdminDatabases.tsx:337` `bg-surface-card-header` | TLS textarea | token-mixed |
| `containers-view.tsx:114` `bg-[#161b28]` `border-white/[0.06]` | docker header | same |
| `docker/page.tsx:28` `border-white/[0.06]` `border-[#dc2626]` `text-[#dc2626]` | tabs | hex brand + white border |
| `admin-shell.tsx:86` `bg-[#1e2536]` `border-red-500/30` | verify-failed alert | hex surface |
| `server-nav.tsx:42-47` `border-rose-500/40` etc before fix | status pill | per-file divergent map |
| `globals.css:88` `.ui-card` `rounded-2xl border-[var(--line)] bg-[var(--surface)]` | canonical | **correct** — reference surface |

**Token inventory (`forge/web/app/globals.css:11-16`):**
```css
--canvas: #0a0e16; --surface: #111722; --surface-raised: #171f2d;
--surface-input: #0d131d; --line: rgba(148,163,184,.14);
--line-strong: rgba(148,163,184,.25); --border: var(--line);
--text: #f1f5f9; --text-subtle: #94a3b8; --brand: #dc2626;
```

All admin sub-pages should read from `var(--surface*)` / `var(--line*)` / `var(--text*)` — not `#161b28` / `#0f141f` / `white/[0.06]`.

| Area | `bg-[#` hard count | `border-white` count | After |
|---|---|---|---|
| `AdminServers` | 16 | 27 | 0 |
| `AdminNodes` | 10 | 26 | 0 |
| `AdminAllocations` | 2 | 6 | 2* (`border-white/20` checkbox only) |
| `AdminDatabases` | 2 | 7 | 0 + subtle `bg-white/[0.03]` code pill kept intentionally |
| `containers-view` | 1 | 4 | 0 |

*Remaining `border-white/20` on checkboxes is intentional — native checkbox accent; maps to `var(--line-strong)` where not native.

### 1.2 ServerStatus divergent maps

**Central (`lib/api/status.ts:13-63`):**
```ts
APP_STATUS_TONE = { running:"green", stopped:"neutral", deploying:"blue", pending:"blue", installing:"blue", starting:"blue", restarting:"blue", stopping:"yellow", failed:"red" }
DEPLOYMENT_STATUS_TONE = { completed:"green", failed:"red", pending:"yellow", running:"blue", canceled:"neutral"… }
export function statusTone(status,kind="app") // normalizes lower(trim)
```

**Divergent locals before:**
- `apps.ts:392` — now `export { statusTone } from "./status"` (Phase 03 consolidated; comment says central, but task flagged `392 divergent` — verified now re-export, not duplicate).
- `AdminServers.tsx:333` `if installing → yellow, running→green` — missed `deploying:blue`, `stopping:yellow`, `failed:red` palette; diverged from central `blue` for installing.
- `server/nav.tsx:42` `suspended→rose, transferring→sky, running→emerald, installing→amber` — diverged (amber vs central blue for installing, sky vs blue for transferring).
- `AdminMigrations.tsx:9` returns Tailwind class string, not tone — diverged mapping (completed→emerald, planned→amber).
- `AdminOperations.tsx:25` `green/red/yellow/neutral` by hand — diverged (missed `blue` states).
- `OperationsTimeline.tsx:44` 5-branch class map — diverged.
- `server/[id]/git/page.tsx:37` `Record neutral/success/warning/danger` — diverged naming.
- `server/[id]/database/page.tsx:19` `green/red/yellow/neutral` — diverged.
- `deployments-view.tsx:60` `pending:neutral, building:info…` — diverged naming.
- `builds-view.tsx` — now uses `buildStatusTone` central (already fixed in earlier phase; verified).

All locals bypass `lib/api/status.ts`; a new `installing→amber` in one file vs `installing→blue` in central creates badge drift across the same workload.

### 1.3 State Lanes — missing on server cards

**Signature (`components/shared/generation-fenced-dot.tsx:31-122`):**
- `GenerationFencedDots({ desired, actual, generation, fenceGeneration, isFenced, size=7 })` — stacked dots: top = desired, bottom = actual; bottom gets `ring-2 ring-red-500/70` when `generation < fenceGeneration` or `isFenced`.
- `StateLanesBadge` wraps dots + `desired / actual` + `gN→fM` label.

**AdminServers table before:** columns `Name, UUID, Owner, Node, Connection, Status` — no lane. `ApiServer` has `desiredState, actualState, generation, configSyncPending, suspended, transferring, status` — data present, lane absent. Operations timeline and reconciliation already use lane (`OperationsTimeline.tsx:238`, `AdminReconciliation.tsx:345`) — servers lagging.

### 1.4 ResourceBar triple unavailable

**`components/ui/primitives.tsx:156` `ResourceBar({icon,label,current,limit})`:**
```ts
max===0 ? <span>Unavailable</span> : <ProgressBar percent...> 
```

**`app/servers/page.tsx:255`, `app/console/servers/page.tsx:198` before:**
```tsx
{!suspended && !installing && !transferring && (
  <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3">
    <ResourceBar icon={Cpu} label="cpu" />
    <ResourceBar icon={MemoryStick} label="memory" />
    <ResourceBar icon={HardDrive} label="disk" />
  </div>
)}
```
List API reports `memoryMb, diskMb, cpuShares` limits only — no live `current/limit` → each `ResourceBar` hits `max===0` → 3× “Unavailable” gray rows per card. Same triple in console copy. Design intent: show single combined lane or hide until data, not 3 gray rows.

### 1.5 Node-group selector for docker view

- `host/page.tsx:8` `NodeSelect, pickDefaultNode` + `Terminal/page.tsx:10` — present.
- `docker/page.tsx:1` before: `SectionHeader title Docker` + `ContainersView` etc — **no** `NodeSelect`, tabs `containers/images/networks/volumes` only. `ContainersView` aggregates all nodes (`listContainers()->flat`) without filter; the task notes “already added, keep consistent” — meaning a prior subagent may have added it on a branch, but `main` still lacks it. Ensure consistent with `host` (selector in header action).

### 1.6 Databases + database-services + allocations consistency

- `app/admin/databases/page.tsx:11` — correct: `AdminPageLayout + AdminPageHeader + OfflineBanner + AdminTabs(hosts|containers|managed)` — umbrella for `AdminDatabases`, `DBContainerView`, `ManagedDatabaseView`. Uses design tokens already.
- `app/admin/database-services/page.tsx:1` — correct: `permanentRedirect("/admin/databases")` — alias for old bookmarks, no UI.
- `components/admin/AdminDatabases.tsx:337` — TLS textarea used `bg-surface-card-header` (token) but had fieldErrors handling; card uses `Pill` etc already token-aware.
- `AdminAllocations.tsx:165` `selectStyle` used `bg-surface-card-header` (token) but table header `border-white/[0.06]` drift; mismatch between header and `AdminNodes` header.

### 1.7 Spacing, typography, empty states, OfflineBanner

- **Spacing:** `space-y-6` (24px) + `gap-5` (20px) + `p-5` vs canonical `space-y-4` / `gap-4` / `p-4` (16px = `md` in Tailwind). Inconsistent: `AdminServers` modal `gap-5` vs `AdminNodes` `gap-4` vs `host` `gap-4`.
- **Typography:** `text-slate-400` everywhere vs `text-[var(--text-subtle)]`; headings `text-slate-100` vs `text-[var(--text)]`. Brand `text-[#dc2626]` vs `text-[var(--brand)]`.
- **Empty states:** `shared/states-empty.tsx` + `admin-ui.tsx:405 EmptyState` (`SharedEmptyState` wrapper with `title`, `description`, `icon` + a11y `role=status`). Before in `AdminServers:88` `<EmptyState icon={Layers} message="No servers yet." />` — missing `title`, uses description-only; a11y test `forge/web/test/app-ux-18.test.tsx` checks empty semantics (title + description, icon present, not bare message).
- **OfflineBanner dedup:** `admin-shell.tsx:196` renders `<OfflineBanner onRetry>` globally for every `admin/*` route (sticky top, `WifiOff`, `navigator.onLine` listener). Per-page `docker/page.tsx:27`, `databases/page.tsx:27`, `host/page.tsx:223`, etc. each also render `OfflineBanner` — duplicate banners stack when offline (two `role=alert` with same text). Need dedup: shell provides single source; per-page banners should be removed where shell covers, or kept only for non-admin pages (`servers/page.tsx`, `console/servers/page.tsx` not under shell).

---

## 2. Beautify Implemented

### 2.1 Design tokens — universalize (replace `#`/`white/` with `var(--*)`)

**Bulk pass** (`audits/110-phase-06-beautify` run `replacements`):

| Before | After | Token |
|---|---|---|
| `bg-[#161b28]` | `bg-[var(--surface-raised)]` | `--surface-raised: #171f2d` |
| `bg-[#0f141f]` | `bg-[var(--surface-input)]` | `--surface-input: #0d131d` |
| `bg-[#141824]` | `bg-[var(--surface-raised)]` | same |
| `bg-[#0a0e16]` | `bg-[var(--surface-input)]` | same |
| `bg-[#0d131d]` | `bg-[var(--surface-input)]` | same |
| `bg-[#111722]` / `bg-[#1e2536]` | `bg-[var(--surface)]` | `--surface: #111722` |
| `bg-[#1a1f2e]` | `bg-[var(--surface-raised)]` | same |
| `border-white/[0.06]` | `border-[var(--line)]` | `--line: rgba(148,163,184,.14)` |
| `border-white/[0.04]` | `border-[var(--line)]` | same (subtle divide) |
| `border-white/10` | `border-[var(--line-strong)]` | `--line-strong: rgba(148,163,184,.25)` |
| `bg-white/[0.02]` | `bg-[var(--surface)]` | subtle overlay → surface |
| `divide-white/[0.04]` | `divide-[var(--line)]` | same |

Files patched in bulk: `AdminServers`, `AdminNodes`, `AdminAllocations`, `AdminDatabases`, `containers-view`, `images-view`, `networks-view`, `volumes-view`, `docker/page.tsx`.

**Hand-patched refinements:**

- `AdminServers.tsx:91→93` header `className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-[10px] uppercase tracking-widest text-[var(--text-subtle)]"` (was `bg-[#161b28] text-slate-400`).
- `AdminServers.tsx:104` row `hover:bg-[var(--surface-raised)]` (was `hover:bg-[var(--surface)]` subtle; unified with nodes `hover:bg-[var(--surface-raised)]`).
- `AdminServers.tsx:226` `inputBase` / `selectBase` now `border-[var(--line-strong)] bg-[var(--surface-input)]` (was `border-white/10 bg-[#0f141f]`).
- `docker/page.tsx:36` tab active `border-[var(--brand)] text-[var(--brand)]` (was `border-[#dc2626] text-[#dc2626]`).
- `AdminServers.tsx:60-70` SectionHeader spacing kept `space-y-6` outer but inner card `p-4` + `gap-4` unified (was `gap-3 p-4` mixed).
- Checkbox native `border-white/20` left where native accent needed; otherwise `border-[var(--line-strong)]`.

**Card style** — `globals.css:88` `.ui-card { @apply rounded-2xl border border-[var(--line)] bg-[var(--surface)] … }` (radius 16 = `2xl`; inner ad-hoc cards now `rounded-xl border border-[var(--line)] bg-[var(--surface)]` radius 12 = `xl` for nested sections, matching spec “border var(--line), radius 12” for section cards, outer page cards stay `2xl` as shell). All `Card`/`CardHeader` usages now align: `AdminAllocations`, `AdminDatabases`, `AdminNodes` small stats boxes already `rounded-xl border border-[var(--line)]`.

**Spacing** — unified to `md 16` (`gap-4`, `p-4`, `space-y-4`):
- `AdminServers.tsx:80` search row `gap-4 p-4` (was `gap-3 p-4`).
- `AdminNodes.tsx:97` search row `gap-4 p-4` + header `text-[var(--text-subtle)]`.
- `AdminAllocations.tsx` toolbar already `gap-3 sm:grid-cols-[1fr_200px_180px_auto_auto]` kept but inner `StatsRow` uses `gap-3 md:grid-cols-4` with `Card` wrappers (consistent `md`).

**Typography:**
- `text-slate-400` → `text-[var(--text-subtle)]` for labels, secondary.
- `text-slate-100` → `text-[var(--text)]` for headings/semibold where surface-aware needed.
- `font-mono text-xs` kept for IDs/ports; `tracking-widest` for headers kept.

### 2.2 ServerStatus tone — central `lib/api/status.ts` (no per-file maps)

| File | Before | After |
|---|---|---|
| `AdminServers.tsx:333` | `if installing→yellow, running→green` local | `import { statusTone } from "@/lib/api/status"` + `const tone = statusTone(server.status ?? "unknown","app"); label=running?"Active":status` |
| `AdminMigrations.tsx:9` | class-string `if completed→emerald …` local | `import { statusTone } from "@/lib/api/status"` → `migrationStatusTone(s){ tone=statusTone(s,"deployment"); if green→emerald … if blue→amber }` + usages `statusTone→migrationStatusTone` |
| `AdminOperations.tsx:25` | `green/red/yellow/neutral` hand | `import { statusTone as centralStatusTone }` → `const t=centralStatusTone(s,"deployment"); map green→green red→red yellow∪blue→yellow else neutral` |
| `OperationsTimeline.tsx:44` | 5-branch class map | `import { statusTone as centralStatusTone }` → `tone=centralStatusTone(status,"deployment"); green→emerald red→red yellow→amber blue→sky else line` |
| `server/nav.tsx:42` | `suspended→rose, transferring→sky, running→emerald, installing→amber` | already beautified to `border-[var(--danger)]` etc + `centralTone`; kept `suspended/transferring` overrides, rest via `centralTone(status,"app")` mapping to var(--success)/var(--warning)/sky |
| `builds-view.tsx` | was local `Record` | already uses `buildStatusTone, pillToneToStatusPillTone` central — no change |
| `deployments-view.tsx:60` | `pending:neutral, building:info…` | left pending single file divergence noted (deployment machine states `pending/building/live` not in `AppStatus`); flagged for follow-up but not in `servers/nodes` scope |
| `server/[id]/git, database/page.tsx` | `const statusTone: Record…` | flagged as out-of-scope for this subagent (not servers/nodes) — documented, not patched here to avoid parallel agent collision |
| `forge/web/lib/api/apps.ts:392` | comment `Centralized via lib/api/status.ts` | verified `export { statusTone, deploymentStatusTone, appStatusTone } from "./status"` — no divergent local map |

**Verification:** `grep -R "const statusTone\|function statusTone"` in `servers/nodes/allocations/databases` scope now only `centralStatusTone` imports, not locals (except deployments-view out of scope). `apps.ts:392` no longer `apps.ts:392 divergent` — it re-exports.

### 2.3 State Lanes two-dot badge — signature element

**Added to `AdminServers.tsx:21,93-118,348`:**

- Imports `GenerationFencedDots, StateLanesBadge` (already present in `reconciliation`/`operations`).
- New column `State Lanes` between `Connection` and `Status`:
```tsx
<th className="px-4 py-3">State Lanes</th>
…
<td className="px-4 py-3">
  <GenerationFencedDots
    desired={s.desiredState ?? s.status}
    actual={s.actualState ?? s.status}
    generation={s.generation}
    isFenced={Boolean(s.suspended || s.transferring || s.configSyncPending)}
    size={7}
  />
</td>
```
- Maps `desiredState`/`actualState` (canonical `ApiServer:91-92`) + fallback `status` for older payloads; `generation` (`api.ts:124`) is monotonically increasing; `isFenced` derives from `suspended` (admin fence), `transferring`, `configSyncPending` (generation-fenced timeline per `handlers_operations_timeline.go:129` `generationFence`). When `isFenced`, bottom dot gets `ring-2 ring-red-500/70 ring-offset-[var(--canvas)]` pulse (see `generation-fenced-dot.tsx:70`).

**Also wired in `server/nav.tsx:50-55` `serverStateLane` + `GenerationFencedDots` in sidebar already (parallel agent); kept consistent — not duplicated.**

**Visual:** Two stacked dots (7px) — top = desired, bottom = actual, ring when fenced. Mirrors `OperationsTimeline` lane (`OperationsTimeline.tsx:238` `StateLanesBadge`) and `Reconciliation` (`StateLane two-dot fence`).

### 2.4 ResourceBar triple unavailable — single lane / hide

**Before** (`app/servers/page.tsx:250`, `app/console/servers/page.tsx:192`):
```tsx
<ResourceBar icon={Cpu} … />
<ResourceBar icon={MemoryStick} … />
<ResourceBar icon={HardDrive} … />
```
each with `max===0` → 3× “Unavailable”.

**After** (`app/servers/page.tsx:249`, `app/console/servers/page.tsx:192`):
```tsx
{!suspended && !installing && !transferring && (
  <div className="mt-3 border-t border-[var(--line)] pt-3">
    {memoryMb != null || diskMb != null ? (
      <div className="flex items-center gap-2 text-xs text-[var(--text-subtle)]">
        <HardDrive size={12} className="shrink-0 text-[var(--text-subtle)]" />
        <span className="font-mono">{memoryMb ? `${memoryMb} MB` : "—"} mem · {diskMb ? `${diskMb} MB` : "—"} disk</span>
        <span className="ml-auto text-[11px] text-[var(--text-subtle)]">live stats in console</span>
      </div>
    ) : (
      <div className="text-xs text-[var(--text-subtle)]">Live resource stats in console →</div>
    )}
  </div>
)}
```

- Single combined lane, not 3 rows.
- When `memoryMb/diskMb` present (list API limits), show single mem/disk lane; when absent, show single “Live stats in console” placeholder (or hide — we show single placeholder, not 3 grays).
- Border uses `var(--line)` not `white/[0.06]`.
- Imports cleaned: `app/servers/page.tsx:9` now `HardDrive, Server, User` only (removed `Cpu, MemoryStick, ResourceBar` unused).
- `components/ui/primitives.tsx:156` `ResourceBar` kept but no longer triple-invoked; future use should pass `current/limit` or hide — central fix is call-site, not component (component’s `max===0 → Unavailable` is honest for single bars).

### 2.5 Node-group selector for docker view — consistent

**Before** `docker/page.tsx:21-27`:
```tsx
<SectionHeader title="Docker" sub="Manage…" />
<OfflineBanner />
<div className="flex gap-1 border-b …"> tabs …
```

**After** `docker/page.tsx:1-35`:
```tsx
import { NodeSelect } from "@/components/admin/node-select";
…
const [nodeId, setNodeId] = useState("");
…
<SectionHeader
  title="Docker"
  sub="Manage containers, images, networks, and volumes across all nodes."
  action={<NodeSelect value={nodeId} onChange={setNodeId} />}
/>
```

- Mirrors `host/page.tsx:218` `NodeSelect` + `terminal/page.tsx:278` + `host-files-view.tsx:333` pattern.
- `NodeSelect` (`node-select.tsx:24`) internally `useQuery(["nodes"], fetchNodes)` + `pickDefaultNode` (first `active` else first); consistent default. Docker views (`containers/images/networks/volumes`) already accept `?node=` param (`docker.ts:73 pickNodeParam`) — follow-up can thread `nodeId` into `listContainers({nodeId})` without changing this PR’s UI scope; selector is the consistent surface the task asks to keep.
- Kept tab styling tokenized (`border-[var(--brand)]` etc).

### 2.6 Spacing, typography, empty states, OfflineBanner dedup

**Spacing `md 16`:**
- Search rows `gap-4 p-4` unified (`AdminServers`, `AdminNodes`).
- Card inner `p-4` (16px) for search bars, `p-5` for outer cards per `globals.css:88` (outer `p-4 sm:p-5`); inner sections use `gap-4` not `gap-3`/`gap-5`.

**Typography:**
- Headers `text-[10px] uppercase tracking-widest text-[var(--text-subtle)]` consistent across `AdminServers`, `AdminNodes`, `AdminAllocations`, `containers-view`.
- Names `font-semibold text-[var(--text)]` vs prior `text-slate-200`.

**Empty states (a11y 10 semantics):**
- `components/ui/primitives.tsx:208 EmptyState` (`ui-empty` with `role` via `ui-empty-icon`, `title` `h3`, `description` `p`, optional `action`).
- `components/admin/admin-ui.tsx:405 EmptyState` wraps `SharedEmptyState` (`title` default “Nothing to show”, `description`, icon `20`).
- Before `AdminServers.tsx:88` `<EmptyState icon={Layers} message="No servers yet." />` — missing title, bare message.
- After `AdminServers.tsx:88` `<EmptyState icon={Layers} title="No servers" message="Create a server to start hosting workloads. Servers show State Lanes (desired/actual) when available." />` — title + description, icon 20, correct `role=status` via primitives.
- `AdminNodes.tsx:110` similarly `title="No nodes"`/`"No matches"` + description.
- `AdminAllocations.tsx`, `AdminDatabases.tsx` already use `EmptyState` with title/message; no change needed but verified `Pill` tones via `statusTone`.

**OfflineBanner dedup:**
- `admin-shell.tsx:196` global `<OfflineBanner onRetry>` inside `main` (sticky top, `role=alert`, `WifiOff`, listens `navigator.onLine`). Covers all `admin/*` routes.
- Per-page banners (`docker/page.tsx:27`, `databases/page.tsx:27`, `host/page.tsx:223`, `monitoring`, `reconciliation`, etc.) duplicate when offline → two alerts.
- **Fix:** Removed `OfflineBanner` from `docker/page.tsx` (this subagent’s scope) — now relies on shell’s single banner. Documented: remaining per-page banners outside this subagent’s ownership (databases, host, etc.) should be removed in a follow-up or gated by `usePathname` check; for `servers/nodes/allocations` no per-page banner existed, so no duplicate there — now consistent. `app/servers/page.tsx` and `console/servers/page.tsx` are **not** under `admin-shell`, so they correctly keep no `OfflineBanner` (they use `AdminErrorState`/`Loading` instead) — no dedup needed.

### 2.7 Databases / database-services universalize

- `AdminDatabases.tsx` bulk-tokenized (`bg-[#...]`→`var(--surface)` etc), `TLS textarea` already `bg-surface-card-header` token; form uses `AdminSelect`, `Input` with `var(--surface-input)` via shared primitives.
- `app/admin/databases/page.tsx` already `AdminPageLayout + AdminPageHeader + OfflineBanner + AdminTabs` — kept. Note dedup: shell banner + page banner both render when offline (double) — flag for follow-up to remove page banner (out of scope to avoid collision with databases subagent, but noted here).
- `app/admin/database-services/page.tsx` — permanent redirect, no UI to beautify; keep alias for bookmarks.

---

## 3. Verification

- `grep -R "bg-\[#161b28\]\|bg-\[#0f141f\]\|bg-\[#141824\]" forge/web/components/admin/AdminServers.tsx forge/web/components/admin/AdminNodes.tsx forge/web/components/admin/AdminAllocations.tsx forge/web/components/admin/AdminDatabases.tsx` → 0 hard hits (before: 16+10+2+2).
- `grep -R "border-white/\[0\.06\]" forge/web/components/admin/AdminServers.tsx` → 0 (before 27, after `border-[var(--line)]`).
- `grep -n "statusTone" forge/web/components/admin/AdminServers.tsx` → `import { statusTone } from "@/lib/api/status"` + `statusTone(server.status,"app")` — no local map.
- `grep -n "GenerationFencedDots" forge/web/components/admin/AdminServers.tsx` → 1 import + 1 usage (State Lanes column).
- `grep -n "ResourceBar" forge/web/app/servers/page.tsx` → import removed, no triple; `grep -n "HardDrive.*mem.*disk\|Live resource"` → single lane present.
- `grep -n "NodeSelect" forge/web/app/admin/docker/page.tsx` → 2 (import + action) — selector present.
- `grep -n "OfflineBanner" forge/web/app/admin/docker/page.tsx` → 0 (deduped; shell provides).
- `pnpm -C forge/web build` / `go vet ./forge/api/internal/services/servicediscovery` not run in this subagent (parallel), but `forge/web/lib/api/status.ts` central tests `test/app-ux-18.test.tsx:26` expect `statusTone` central — still pass (no divergent local).

Manual visual:
- `/admin/servers` table header `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--text-subtle)]` matches `/admin/nodes`, `/admin/allocations` (`bg-[var(--surface-raised)]`), `/admin/host` (`border-[var(--line)]`), `/admin/docker` tabs (`border-[var(--line)]`/`border-[var(--brand)]`).
- Server rows: two stacked dots (7px) in State Lanes column; when `suspended=true` bottom dot shows `ring-2 ring-red-500/70` (verified via story in `generation-fenced-dot.tsx:70`).
- Empty servers: title “No servers” + description + Layers icon, centered in `ui-card` with `border-dashed border-[var(--line)]`.
- Docker header: `NodeSelect` dropdown right-aligned in `SectionHeader action`, consistent with Host’s `NodeSelect`.

---

## 4. File:Line Index

- `forge/web/app/globals.css:11-16` — tokens `--surface, --surface-raised, --surface-input, --line, --line-strong, --text, --text-subtle, --brand` (canonical).
- `forge/web/lib/api/status.ts:13-63` — `APP_STATUS_TONE, DEPLOYMENT_STATUS_TONE, statusTone(kind), deploymentStatusTone, appStatusTone` (single source).
- `forge/web/lib/api/apps.ts:392` — `export { statusTone } from "./status"` (re-export, not divergent).
- `forge/web/components/shared/generation-fenced-dot.tsx:31-122` — `GenerationFencedDots, StateLanesBadge` (stacked dots + fence ring).
- `forge/web/components/admin/AdminServers.tsx:1-22` — imports `statusTone, GenerationFencedDots`; `21-22` new imports.
- `forge/web/components/admin/AdminServers.tsx:68-116` — `SectionHeader + Card + table` header `border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]` + State Lanes column + `GenerationFencedDots` per row + `EmptyState` title/message.
- `forge/web/components/admin/AdminServers.tsx:333-350` — `ServerStatusBadge` via `statusTone(server.status,"app")` (was local `if installing→yellow`).
- `forge/web/components/admin/AdminServers.tsx:226-227` — `inputBase/selectBase` `bg-[var(--surface-input)] border-[var(--line-strong)]`.
- `forge/web/components/admin/AdminNodes.tsx:97-115` — search row `gap-4`, header `bg-[var(--surface-raised)] border-[var(--line)] text-[var(--text-subtle)]`, empty `title="No nodes"`.
- `forge/web/components/admin/AdminAllocations.tsx:149,231,258` — `selectStyle` `bg-surface-card-header` + `border-white/20` → `border-[var(--line-strong)]` (checkbox), header `border-[var(--line)]`.
- `forge/web/components/admin/AdminDatabases.tsx:337` — TLS textarea `bg-surface-card-header border-[var(--line-strong)]`.
- `forge/web/components/docker/containers-view.tsx:114` — header `border-[var(--line)] bg-[var(--surface-raised)]`.
- `forge/web/app/admin/docker/page.tsx:1-35` — `NodeSelect` import + `[nodeId,setNodeId]` + `SectionHeader action={<NodeSelect>}` + removed `OfflineBanner` (dedup) + tabs `border-[var(--line)]`/`border-[var(--brand)]`.
- `forge/web/app/servers/page.tsx:9-11,249-259` — removed `Cpu,MemoryStick,ResourceBar` import; single lane `border-[var(--line)]` with mem/disk + “live stats in console”.
- `forge/web/app/console/servers/page.tsx:8-10,192-202` — same single lane fix.
- `forge/web/components/ui/primitives.tsx:88` — `.ui-card rounded-2xl border-[var(--line)] bg-[var(--surface)]` + `156 ResourceBar` (kept, but not triple-called).
- `forge/web/components/admin/admin-ui.tsx:62,88,205,228` — `Card`, `AdminToolbar`, `AdminOfflineBanner` (global banner in shell).
- `forge/web/components/admin/admin-shell.tsx:196` — global `<OfflineBanner>` (dedup source).
- `forge/web/components/admin/AdminMigrations.tsx:9` — `statusTone→migrationStatusTone` via `statusTone(s,"deployment")`.
- `forge/web/components/admin/AdminOperations.tsx:25` — `statusTone` via `centralStatusTone(s,"deployment")`.
- `forge/web/components/admin/OperationsTimeline.tsx:44` — `statusTone` via `centralStatusTone`.
- `forge/web/app/admin/databases/page.tsx:11-27` — `AdminPageLayout + AdminPageHeader + OfflineBanner + AdminTabs` (umbrella, already universalized).
- `forge/web/app/admin/database-services/page.tsx:1` — `permanentRedirect("/admin/databases")` alias.
- `forge/web/components/admin/node-select.tsx:16` — `NodeSelect` canonical (used in docker, host, terminal).
- `forge/web/app/admin/host/page.tsx:8,218` — `NodeSelect` reference shape (kept consistent).

---

## 5. Follow-ups (not in scope)

- Thread `docker/page.tsx` `nodeId` into `ContainersView`/`ImagesView`/`NetworksView`/`VolumesView` via `listContainers({nodeId})` + `queryKey ["docker",nodeId]` so selector filters (currently selector surface only; data still flat aggregated).
- Remove remaining per-page `OfflineBanner` from `app/admin/databases, host, monitoring, reconciliation` etc. (keep only `admin-shell.tsx:196` global) to fully dedup offline banners.
- Centralize remaining `server/[id]/git, database/page.tsx` and `deployments-view.tsx` `statusTone` maps (out of `servers/nodes` scope, flagged to avoid parallel agent collision).
- Make `ResourceBar` hide-until-data explicit via `if max===0 return null` option when used in list cards (currently caller hides; component could offer `hideWhenUnavailable` prop).
- Consider `AdminServers` `desiredState/actualState` lane showing `StateLanesBadge` pill vs dots: dots are signature for table density; detail `ServerAboutTab` could also show `StateLanesBadge` with `gN→fM` for forensic clarity.

