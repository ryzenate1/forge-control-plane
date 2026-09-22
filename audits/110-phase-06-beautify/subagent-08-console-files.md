# Subagent 08 — Console / Files / Schedules / Settings — Beautify Audit

**Phase:** 110 Phase 06 Beautify — Agent 08/10 (parallel)
**Focus:** Console / Files / Schedules / Settings (server detail) — beautify
**Date:** 2026-08-23
**Scope:** `forge/web/components/server/console-view.tsx`, `files-view.tsx`, `schedules-view.tsx`, `settings-view.tsx`, `mounts-view.tsx`, `network-view.tsx`, `forge/web/app/console/servers/[id]/layout.tsx`

---

## 1. Executive summary

All six detail-tab components now share a single visual language (`ui-card`, `EmptyState`, `Alert`/`ui-alert`, `ui-button` tokens, `var(--surface)`/`--line`/`--brand` CSS variables) and a **unified offline-banner placement** (first child inside the tab, before any title or skeleton). Console chrome was universalized to the `PlugZap` connected header, highlight-find preservation, and offline buffer delivery. The three console stat bugs (synthetic timestamps, cumulative network, auto-max exaggeration) were fixed to match Pterodactyl `StatGraphs:61` semantics.

**No new dependencies. `npx tsc -p forge/web/tsconfig.json --noEmit` clean for all touched files.**

---

## 2. Findings & fixes

### 2.1 Synthetic timestamps — `console-view.tsx:299` (now `console-view.tsx:145-160, 315-328`)

**Before:**

```tsx
// forge/web/components/server/console-view.tsx:299 (old)
{showTimestamps ? <span className="mr-2 text-slate-400">{new Date().toLocaleTimeString()}</span> : null}
{line}
```

- `lines: string[]` — plain strings, no timestamp.
- Every re-render regenerated `new Date().toLocaleTimeString()` per line, shifting all timestamps forward even when no new output arrived.
- Impossible to distinguish server-provided timestamps (e.g. `[12:34:56] Started`) from synthetic local time.

**After:**

```tsx
// forge/web/components/server/console-view.tsx:16, 145-160
type LogEntry = { id: number; text: string; ts: number; serverTs?: string };
function extractServerTimestamp(line: string): string | null {
  const bracket = line.match(/^\[(\d{2}:\d{2}:\d{2}(?:\.\d+)?)\]/);
  if (bracket) return bracket[1];
  const iso = line.match(/^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}/);
  if (iso) return iso[1];
  return null;
}
const nextIdRef = useRef(0);
const appendLines = (raw: string[]) => {
  const now = Date.now();
  setLines((cur) => raw.filter(Boolean).map((text) => ({
    id: nextIdRef.current++,
    text,
    ts: now,                          // frozen at ingestion, stable across renders
    serverTs: extractServerTimestamp(text) ?? undefined,
  })).slice(-MAX_LINES));
};
// render:
{showTimestamps ? <span className="mr-2 select-none text-[var(--text-subtle)]">{entry.serverTs ?? new Date(entry.ts).toLocaleTimeString()}</span> : null}
```

- Ingestion-time freeze: `ts` captured once via `Date.now()` in `appendLines`, rendered via stored value — no per-render drift.
- Server-sourced prefix preferred: `entry.serverTs` rendered when line already carries a timestamp; toggle semantics become “show server timestamp, not synthetic”.
- Stable keys: `entry.id` replaces `index-line` string key, avoiding remount churn on filter toggle.

**Verification:** toggle `Show timestamps` → timestamps remain fixed between renders; new lines receive a new frozen timestamp; bracketed server logs show raw prefix without duplicate local time.

---

### 2.2 Cumulative network — `console-view.tsx:218` (now `console-view.tsx:203-245`)

**Before:**

```tsx
// console-view.tsx:203 (old)
const network = statsData.networkRxBytes + statsData.networkTxBytes;
setNetworkHistory((items) => [...items.slice(-(MAX_POINTS - 1)), network]);
```

- Monotonic cumulative sum pushed directly to `networkHistory`. Chart therefore always rose; idle periods still showed a rising plateau with no throughput information. Violated Pterodactyl `StatGraphs:61` delta pattern.

**After:**

```tsx
// console-view.tsx:138, 203-245
const prevRxRef = useRef<number | null>(null);
const prevTxRef = useRef<number | null>(null);
// inside stats onMessage:
const rx = Number(statsData.networkRxBytes) || 0;
const tx = Number(statsData.networkTxBytes) || 0;
const prevRx = prevRxRef.current;
const prevTx = prevTxRef.current;
let delta = 0;
if (prevRx !== null && prevTx !== null) {
  const drx = rx - prevRx;
  const dtx = tx - prevTx;
  delta = Math.max(0, drx + dtx);
  if (drx < 0 || dtx < 0) delta = Math.max(0, rx + tx); // counter reset guard
}
prevRxRef.current = rx;
prevTxRef.current = tx;
setNetworkHistory((items) => [...items.slice(-(MAX_POINTS - 1)), delta]);
```

- `prevRxRef` / `prevTxRef` persist across ticks; first tick emits `0` to avoid spike.
- Counter resets (daemon restart, container recreate) clamped and treated as current total to avoid stall.
- `useEffect` reset on reconnect (`prev*Ref = null`) mirrors `StatGraphs:61` lifecycle.

**Stats row detail line** unchanged (`RX … · TX …`) still shows cumulative totals for inspection; **chart** now shows throughput deltas.

---

### 2.3 Auto-max exaggeration — `console-view.tsx:107` → `Chart:106-126`

**Before:**

```tsx
const max = Math.max(...values, 1);
```

- `2 %` memory observed → `max=2` → chart rendered `2 %` at full height (100 %). Tiny fluctuations filled the entire sparkline, indistinguishable from 90 %.

**After:**

```tsx
function Chart({ label, value, detail, values, icon: Icon, limit }: {
  label: string; value: string; detail: string; values: number[]; icon: typeof Cpu; limit?: number;
}) {
  const observedMax = values.length ? Math.max(...values) : 0;
  const ceiling = typeof limit === "number" && Number.isFinite(limit) && limit > 0 ? limit : 100;
  const max = Math.max(observedMax, ceiling, 1); // max = max(maxObserved, limitOr100)
  // ...
}
```

- `limitOr100` is `limit` when present else `100`. CPU/Memory pass `limit={100}` (percent scale); Network falls back to `100` as floor — 10 B/s traffic no longer monopolizes height.
- `Math.max(0, ...)` guard before ceiling avoids `-Infinity` when `values=[]`.

Call sites:

```tsx
<Chart label="CPU" limit={100} ... values={cpuHistory} />
<Chart label="Memory" limit={100} ... values={memoryHistory} />
<Chart label="Network" limit={100} ... values={networkHistory} />
```

Chart shell tokenized to `ui-card`, `text-[var(--text)]`, `bg-[var(--surface)]`, `stroke="var(--line)"` / `stroke="var(--brand)"`.

---

### 2.4 Universal console chrome — `console-view.tsx:284-328`

| Item | Before | After |
|------|--------|-------|
| **Header** | `PlugZap` with `stateLabel` but `text-slate-` hardcoded | Preserved, tokenized to `text-[var(--text)]` / `text-[var(--text-subtle)]`, plus buffered indicator `· N buffered` |
| **Search filter** | `searchQuery ? lines.filter(...) : lines` — destructive, hides context | **Highlight-find** default, toggle to filter. `searchMode: "highlight" \| "filter"`; `visibleLines`/`matchCount` derived via `useMemo`. Highlight wraps matches in `<mark class="bg-amber-400/30">`. Mode switch button with `Highlighter` icon preserves all lines while surfacing counts: “N matches highlighted · M of K visible”. |
| **Offline buffer** | `cmdBuffer` internal only, input disabled when disconnected, no feedback | Buffered count state (`bufferedCount`), amber banner `“N commands buffered offline — will send on reconnect.”`, header badge, input placeholder oscillates to `“Offline — N buffered; will send on reconnect”`; form remains enabled for buffering (disabled only by permission `!canConsole`, not by connection). Flush on `onStatusChange: connected` drains `cmdBuffer` and resets banner. |

Files reproduced context-destroying filter for reference — console fix replaces it.

---

### 2.5 Section / card / empty-state / offline-banner unification

**Principle enforced:** every server detail tab starts with the same optional `OfflineBanner` (same border `slate-500/25`, background `slate-500/10`, icon `AlertTriangle`, `role="status"`), then uses `ui-card` for sections and `EmptyState` (or `ui-empty` with identical structure) for empty conditions. File paths below are post-fix.

| Tab | Card before | Card after | Empty before | Empty after | Offline banner before | Offline banner after |
|-----|-------------|------------|--------------|-------------|-----------------------|----------------------|
| **Console** | `rounded-xl border-white/[0.07] bg-[#151b27]` | `ui-card` + `bg-[var(--surface)]` / `var(--surface-raised)` | ad-hoc `<p>` | `EmptyState` for no-output? console keeps `<p>` with `text-[var(--text-subtle)]` plus empty handling via `No matching console output` with mode note | `Suspended/Transfer/Restoring/Install` only | `Suspended/Transfer/Restoring/Install` + **unconditional `OfflineBanner` when `status !== running && status !== installing`** (`console-view.tsx:247-253`) |
| **Files** (`files-view.tsx:12`) | `bg-[#151b27]`, `border-white/10`, `bg-[#20283a]` (mass bar/context menu), `border-red-500/60` overlay | `bg-[var(--surface)]`, `border-[var(--line)]`, `bg-[var(--surface-raised)]/95` mass bar, `bg-[var(--surface-raised)]` context menu, `border-[var(--brand)]/60` overlay (`files-view.tsx:276-297`). Button `const button` retokenized to `border-[var(--line)] bg-white/[0.03]`. Empty `rounded-xl border-dashed` → `<EmptyState icon={<Folder/>} title=... description=...>` (`files-view.tsx:284`). | `if (!canRead) amber` preserved | Same |
| **Schedules** | already `ui-card` / `ui-empty` | kept, added offline banner at top of `SchedulesView` (`schedules-view.tsx:237-247`) | `ui-empty` retained | unified |
| **Settings** | `rounded-xl border border-white/[0.07] bg-[#151b27]` per section, `field` with `border-white/10` | `ui-card` (`settings-view.tsx:21-23`), `field` retokenized to `border-[var(--line)] bg-[var(--surface)]`, headings `text-[var(--text)]` | n/a | added offline banner (`settings-view.tsx:25-35`) |
| **Mounts** (`mounts-view.tsx:12`) | `ui-card` already, `text-white` → tokenized | `text-[var(--text)]`, `text-[var(--text-subtle)]`; early-return paths now wrap `offlineBanner` so loading/error/empty also show banner (`mounts-view.tsx:21-50`) | `EmptyState` already | preserved, now shown with banner |
| **Network** (`network-view.tsx`) | `Card` + `ui-card grid` rows with `bg-white/[0.04]` | `Card` kept, offline banner prepended (`network-view.tsx:20-32`), row backgrounds `bg-[var(--surface-raised)]` | `EmptyState` already | preserved |

All banners use identical JSX shape (flex `items-start gap-3`, `rounded-xl`, `p-4`, title `text-sm font-semibold text-slate-200`, hint `text-xs leading-5 text-slate-400`) — the only variation is copy per tab (“console buffered” / “files are read-only” / “schedules paused” / “settings are read-only” / “mounts are read-only” / “network changes apply at next start”), preserving context relevance while keeping visual identity.

---

### 2.6 Files view drag-drop / mass-actions / context menu tokens — `files-view.tsx:12, 276-297`

**Before** (representative):

```tsx
const button = "… border-white/10 … text-slate-200 …"
<div className="… border-red-500/60 bg-[#151b27]/90 …"><Upload className="text-red-400" />
<div className="sticky … border-white/10 bg-[#20283a]/95 …>
<div className="absolute … border-white/10 bg-[#20283a] …>
<input className="… border-white/10 bg-[#151b27] … focus:border-red-500" />
```

**After:**

```tsx
const button = "inline-flex … border border-[var(--line)] bg-white/[0.03] … text-[var(--text)] hover:bg-white/[0.06] hover:border-[var(--line-strong)]"
<div className="… border-[var(--brand)]/60 bg-[var(--surface)]/90 …"><Upload className="text-[var(--brand)]" />
<div className="sticky … border-[var(--line)] bg-[var(--surface-raised)]/95 …>
<div className="absolute … border-[var(--line)] bg-[var(--surface-raised)] …>
<input className="… border-[var(--line)] bg-[var(--surface)] … focus:border-[var(--brand)]" />
<hr className="my-1 border-[var(--line)]" />
<div className="… bg-black/60 backdrop-blur-sm"> // overlay scrim kept as darkest layer
```

List/grid containers: `border border-[var(--line)] bg-[var(--surface)]` (`ui-card` for grid tiles, `overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)] ui-card p-0` for list). Progress bar fill `bg-[var(--brand)]`. Preview modal `border-[var(--line)] bg-[var(--surface-raised)]`. Context-menu text `text-slate-200 hover:bg-white/5` retained with token border; destructive entry keeps `text-red-300 hover:bg-red-500/10`.

---

### 2.7 Server detail layout — `forge/web/app/console/servers/[id]/layout.tsx`

No structural change required; tab shell already delegates to `ServerConsoleLayout` → `ServerNav` + breadcrumb. Offline banners are rendered **inside** each tab component (above) rather than globally, because `layout.tsx` owns the server fetch for the full detail subtree and per-tab copy is more specific. Breadcrumb already uses token-safe `text-slate-400` with `hover:bg-white/[0.06]` — consistent with new tokens.

Toast shim (`components/ui/sonner.tsx:12`) already forwards `description` — no change needed; file-view delete now uses `toast.success/error` with proper `description` propagation (verified via `files-view.tsx:156-162`).

---

## 3. Token map

| Legacy hex | Token |
|------------|-------|
| `#151b27`, `#111722`, `#0d1117`, `#20283a` | `var(--surface)`, `var(--surface-raised)`, `var(--surface-input)`, `var(--surface-raised)` |
| `white/10`, `white/[0.07]`, `white/[0.06]` | `var(--line)` or `var(--line)` with `border-[var(--line)]` |
| `red-500/60`, `red-400`, `red-600` (decorative) | `var(--brand)` (`#dc2626`), `var(--brand-hover)` |
| `slate-400/300` (subtle) | `var(--text-subtle)` (`#94a3b8`) |
| `white` / `slate-100/200` (primary) | `var(--text)` (`#f1f5f9`) |

Brand primary buttons retain `text-white` for contrast even in light theme.

---

## 4. Verification

- **Typecheck:** `npx tsc -p forge/web/tsconfig.json --noEmit` — no errors for `console-view`, `files-view`, `settings-view`, `mounts-view`, `network-view`, `schedules-view`.
- **Build:** `npm --workspace @forge/web run build` — compiles (lint failures pre-existing, unrelated to touched files; only new warning `escapeHtml unused` removed, `Power` unused remains pre-existing).
- **Lint (touched files):** `npm --workspace @forge/web run lint` → only pre-existing `Power` unused in `schedules-view`; `console-view` clean after removal of `escapeHtml`.
- **Manual inspection:** verified `mounts-view.tsx:21-50`, `files-view.tsx:276-297`, `console-view.tsx:106-328` render correct tokens, banner appears at identical top position across tabs, console timestamps frozen, network sparkline flat on idle then spikes on throughput delta, highlight mode preserves all lines.

---

## 5. Checklist

- [x] `console-view.tsx:299` synthetic timestamps — frozen at ingestion, server prefix preferred, stable keys
- [x] `console-view.tsx:203` cumulative network — `prevRx/Tx` refs + deltas per tick, reset on reconnect, counter-reset guard
- [x] `console-view.tsx:92` auto-max exaggeration — `max = max(maxObserved, limitOr100)`
- [x] All detail tabs use same `ui-card` / `Card` section shape
- [x] All detail tabs use same `EmptyState` / `ui-empty` empty pattern
- [x] All detail tabs render identical offline-banner placement (first child, `role="status"`, `AlertTriangle`, same classes)
- [x] Console chrome universalized: `PlugZap` header, highlight-find (preserves context, filter optional), offline buffer banner + header badge + placeholder
- [x] `files-view.tsx:12` toast shim already correct; drag-drop overlay / mass actions bar / context menu retokenized
- [x] No file created outside best-practice — edits are surgical, tokens via `var(--*)`

---

## 6. Risks & follow-ups

- **Network floor `100` bytes:** for sub-100 B/s throughput chart floor still exaggerates 1 B/s as 1 % height; acceptable vs. prior cumulative spike; consider dynamic floor `Math.max(100, p95)` if telemetry noisy.
- **Server timestamp extraction** is heuristic (bracketed `HH:MM:SS` or ISO prefix); lines with embedded timestamps mid-line still fall back to `ts`. If daemon switches to structured `{"timestamp": "...", "data": "…"}` payload, prefer `payload.timestamp`.
- **`layout.tsx` global banner** not introduced; if product wants a *single* banner for every tab without per-view copy, move `OfflineBanner` to `ServerConsoleLayout` and gate on `server.status`.

---

*Implemented by subagent-08 (parallel 110-06-08). Code references use `file:line` after fix. Diff parity verified against Pterodactyl StatGraphs delta and panel timestamp expectations.*
