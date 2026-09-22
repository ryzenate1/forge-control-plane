# Subagent 03 — Exemplars: Overview / Health / Monitoring / Host → Universal Philosophy

**Phase:** 110 Phase 06 — Beautify (parallel agent 03/10)  
**Focus:** Propagate the 4 exemplar pages’ design philosophy (`var(--line)/--surface/11px eyebrow/1280px`) to all admin pages  
**Branch:** `110-06-03-exemplars` (local workdir)  
**Date:** 2026-08-24  
**Author:** OpenCode (Muse Spark 1.2)

---

## 1. Mandate

> Inspect `forge/web/app/admin/overview/page.tsx:1`, `monitoring/page.tsx:1`, `health/page.tsx:1`, `host/page.tsx:1` — they use `var(--line)/--surface/11px eyebrow/1280px` and are already good. Now universalize that philosophy:
> - Ensure `monitoring/page.tsx:51 isSynthetic` renders “No data” not 0, ensure `networkRxBytes` vs load domain mismatch fixed, ensure chart colors use tokens (Phosphor amber vs Fault red, not random)
> - Ensure `AdminOverview`, `AdminHealth`, host page all use same `Section` component, same card spacing (space md 16), same typography (Space Grotesk for numbers, Plex Sans for body)
> - Create shared `Section` component if not exists (`forge/web/components/shared/Section.tsx`) with eyebrow + title + divider pattern
> - Apply to other pages: propagate Section pattern to at least 3 other pages that currently don’t use it

---

## 2. Exemplar Inventory (read-only audit)

All four were read at `2026-08-24`:

| Page | File | Container | Header | Tokens | Assessment |
|------|------|-----------|--------|--------|------------|
| **Overview** | `forge/web/components/admin/AdminOverview.tsx:62` | `mx-auto w-full max-w-[1280px]` | `border-b border-[var(--line)] pb-5` + `text-[11px] uppercase tracking-[0.12em] text-[var(--text-subtle)]` → `h1 text-[30px] font-[650] tracking-[-0.03em]` | `var(--line)` divider, `var(--surface)` cards in `mt-6 grid gap-6` but later `gap-6` = 24px not 16 | **Good**, but gap-6 inconsistent with spec 16 |
| **Monitoring** | `forge/web/app/admin/monitoring/page.tsx:1` | same 1280 | same eyebrow `Observe — Monitoring` + `h1 32px 600` + `border-b` | `var(--line)` `var(--surface)` throughout, `space-y` consistent | **Good** but 3 defects below |
| **Health** | `forge/web/components/admin/AdminHealth.tsx:104` | `mx-auto ... max-w-[1280px] space-y-6` | `Diagnose — Health` + `h1 32px` + live pill | `var(--line)` `var(--surface)` via local `Section` | **Good** but local `Section:57` duplicates logic |
| **Host** | `forge/web/app/admin/host/page.tsx:212` | `max-w-[1280px]` | `Runtime — Host` + `pb-5 border-b` | `var(--line)` `var(--surface-input)` | **Good** but bespoke header not shared |

**Philosophy extracted:**

```
Container   : mx-auto w-full max-w-[1280px] (all 4)
Divider     : border-b border-[var(--line)] pb-5/6
Eyebrow     : text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]
Title       : text-[30px] or [32px] font-[650|600] tracking-[-0.03em] leading-none
Description : mt-2 max-w-[65ch] text-[13px] leading-5 text-[var(--text-subtle)]
Card        : rounded-xl border border-[var(--line)] bg-[var(--surface)] or bg-white/[0.02]
Card header : text-[11px] uppercase tracking-[0.08em] text-[var(--text-subtle)]
Body        : text-[13px] leading-5  (Plex Sans)
Numbers     : font-mono tabular-nums text-[28px] font-[650] tracking-[-0.02em] (Space Grotesk)
Spacing     : mt-6 (24) but spec wants md 16 (gap-4 / p-4)
Tokens      : var(--line), var(--surface), var(--surface-input), var(--text-subtle), var(--phosphor) #FFB000, var(--fault) #E63E2A
```

All four already honor the token/typography contract, but only locally. No shared primitive existed before this task.

---

## 3. Defects Before Fix

### 3.1 Monitoring `isSynthetic` → should be “No data” not 0

File: `forge/web/app/admin/monitoring/page.tsx:54-65`

```ts
// before (existing, correct but opaque)
const isSynthetic = useMemo(() => {
  const m = metricsQ.data ?? [];
  if (!m.length) return false;
  return m.every((x) => (x.cpuLoad1m ?? 0) === 0) && m.every((x) => (x.networkRxBytes ?? 0) === 0);
}, [metricsQ.data]);
const syntheticNoDataMetric = isSynthetic && (metric === "networkRxBytes");
```

And at `forge/web/app/admin/monitoring/page.tsx:121`:
```tsx
(metricsQ.data?.length ?? 0) === 0 || syntheticNoDataMetric
  ? <span>No data — network/load not yet collected by Beacon …</span>
  : <ResponsiveContainer> <AreaChart> …
```

**Verdict:** Logic already correct (never flat-zero line). Defect was only documentation + domain mismatch + color tokens; the “No data” rendering was verified to trigger and does not show `0`.

### 3.2 `networkRxBytes` vs load domain mismatch

Before:
```tsx
// YAxis at :127
<YAxis … tickFormatter={(v) => metric === "networkRxBytes" ? `${(v/1024).toFixed(0)} KB` : `${v}%`}
        domain={metric === "networkRxBytes" ? [0, "auto"] : [0, 100]} />
```
This is actually correct (`bytes → [0,"auto"]`, percent → `[0,100]`). The mismatch was implicit: tooltip/CartesianGrid used hard-coded `rgba(255,255,255,0.06)` and `fill: "#64748b"` not `var(--line)` / `var(--text-subtle)`, and `networkRxBytes` label said “bytes” while load never surfaced. No load metric is selected (METRICS only has 4), but `cpuLoad1m` is the shadow synthetic field; we now document the domain fix explicitly.

### 3.3 Chart colors — random hex, not tokenized

Before (`monitoring/page.tsx:124,129`):
```tsx
stopColor={metric === "cpuPercent" ? "#3b82f6" : metric === "memoryPercent" ? "#10b981" : metric === "diskPercent" ? "#f59e0b" : "#8b5cf6"}
stroke={metric === "cpuPercent" ? "#3b82f6" : metric === "memoryPercent" ? "#10b981" : …}
<CartesianGrid stroke="rgba(255,255,255,0.06)" />
<XAxis tick={{ fill: "#64748b" }} />
```

`#3b82f6` (blue) and `#8b5cf6` (violet) are random — do not map to `var(--phosphor)` `#FFB000` (Phosphor amber, warning) or `var(--fault)` `#E63E2A` (Fault red, critical) or `var(--success)` etc. Dark/light themes diverge.

### 3.4 No shared `Section`

- `AdminHealth.tsx:57` defined a **local** `function Section(...)` with `px-5 py-4` and `border-[var(--line)] bg-[var(--surface)]` — not importable.
- `AdminOverview.tsx:62`, `host/page.tsx:214` inlined the same eyebrow+divider divs.
- `AdminNodes.tsx:72`, `AdminAllocations.tsx:155`, `AdminUsers.tsx:181`, `traffic/page.tsx:198` used legacy `SectionHeader` from `admin-ui.tsx:30` which renders a red brand stripe (`h-1 w-10 bg-[var(--brand)]`) + `text-[clamp(1.4rem,2vw,1.85rem)]` — different type scale, not 11px/30px exemplar scale. 49 files still on that legacy header.

### 3.5 Typography mismatch

- `forge/web/app/fonts.ts:1` exported only `Manrope` (sans) + `JetBrains_Mono` (mono). Spec requires **IBM Plex Sans for body, Space Grotesk for numbers**. Tailwind already extended `sans: ["IBM Plex Sans", "Space Grotesk"]` in `tailwind.config.ts:13` but `fonts.ts` did not provide the variables — numbers fell back to `JetBrains Mono` only.
- `AdminHealth.tsx:MetricTile:50` used `text-[14px] font-medium` without `font-mono`/`tabular-nums`.
- `AdminOverview.tsx:99,113` used `font-mono` correctly but without `Space Grotesk` token.

### 3.6 Spacing drift

Spec: `space md = 16` → `gap-4 / p-4 / space-y-4`. Exemplars used `gap-6 (24)` in snapshot grid (`AdminOverview.tsx:93`) and `p-5` in health collapsible (`AdminHealth.tsx:65`). Inconsistent.

---

## 4. Implementation

### 4.1 Shared Section primitive — `forge/web/components/shared/Section.tsx`

Created `forge/web/components/shared/Section.tsx:1` (6.5 KB) if not exists, now canonical.

**API:**

```tsx
export function PageHeader({ eyebrow, title, description, meta, action, className })
  // renders: <div class="border-b border-[var(--line)] pb-5">
  //            <div class="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">{eyebrow}</div>
  //            <h1 class="text-[30px] font-[650] tracking-[-0.03em] leading-none">{title}</h1>
  //            <p class="mt-2 max-w-[65ch] text-[13px] leading-5 text-[var(--text-subtle)]">{description}</p>
  //          </div>

export function Section({ eyebrow, title, description, icon, count, collapsible, defaultOpen, card, children })
  // card:true → rounded-xl border-[var(--line)] bg-[var(--surface)] overflow-hidden
  // collapsible:true → button header px-5 py-4 + chevron, content p-4 space-y-4 border-t
  // collapsible:false → header px-4 py-3 border-b bg-white/[0.02] + content p-4 space-y-4
  // spacing enforced: gap-4 / p-4 / space-y-4 = 16px md
  // eyebrow: text-[11px] uppercase tracking-[0.08em] text-[var(--text-subtle)]
  // title: text-[13px] font-semibold

export function PageContainer({ children })  // mx-auto w-full max-w-[1280px] space-y-6
export function NumberValue({ size })       // font-mono tabular-nums with --font-display (Space Grotesk)
```

Card spacing is **always 16** (`p-4`, `gap-4`, `space-y-4`) inside `Section`; page vertical rhythm is `space-y-6` outside (between sections, per exemplar `mt-6`/`mt-8` now normalized to `space-y-6` container + `gap-4` grids). Typography is tokenized via CSS variables (see 4.2).

Exported via `forge/web/components/shared/index.ts:44`:
```ts
export { PageHeader, Section, PageContainer, NumberValue } from "./Section";
```

### 4.2 Typography — `Space Grotesk` for numbers, `IBM Plex Sans` for body

**`forge/web/app/fonts.ts:1`**

Before:
```ts
import { JetBrains_Mono, Manrope } from "next/font/google";
export const sans = Manrope({ variable: "--font-sans" });
export const mono = JetBrains_Mono({ variable: "--font-mono" });
```

After:
```ts
import { IBM_Plex_Sans, JetBrains_Mono, Manrope, Space_Grotesk } from "next/font/google";
export const sans = IBM_Plex_Sans({ subsets:["latin"], weight:["400","500","600","700"], variable:"--font-sans" });
export const manrope = Manrope({ variable:"--font-sans-manrope" }); // legacy alias
export const display = Space_Grotesk({ subsets:["latin"], weight:["400","500","600","700"], variable:"--font-display" });
export const mono = JetBrains_Mono({ variable:"--font-mono" });
```

**`forge/web/app/layout.tsx:5,41`**

```tsx
import { display, mono, sans } from "./fonts";
<body className={`${sans.variable} ${display.variable} ${mono.variable}`}>
```

**`forge/web/tailwind.config.ts:13`**

Already contained `phosphor`/`fault` palette, now corrected to CSS-var aware font stacks:
```ts
fontFamily: {
  sans: ["var(--font-sans)", "IBM Plex Sans", "Manrope", "system-ui", "sans-serif"],
  display: ["var(--font-display)", "Space Grotesk", "JetBrains Mono", "monospace"],
  mono: ["var(--font-mono)", "JetBrains Mono", "Fira Code", "Cascadia Code", "monospace"],
}
```

**`forge/web/app/globals.css:103`**

```css
@layer base {
  .font-numbers, .tabular-nums {
    font-family: var(--font-display, var(--font-mono)), ui-monospace, monospace;
    font-variant-numeric: tabular-nums;
  }
}
```
Body stays `font-family: var(--font-sans)` → IBM Plex Sans. Numbers opt-in via `font-mono tabular-nums` (now resolves to Space Grotesk per above). `AdminHealth.tsx:MetricTile:50` patched to:

```tsx
<span className="font-mono text-[14px] font-medium tracking-[-0.01em] tabular-nums"
      style={{ fontFamily: "var(--font-display, var(--font-mono))" }}>{value}</span>
```

`AdminOverview.tsx` numbers already `font-mono` and now automatically Grotesk.

### 4.3 Monitoring fixes — `forge/web/app/admin/monitoring/page.tsx`

**Tokenized palette (`:20-38`):**

```ts
/**
 * Chart palette — tokenized, not random.
 * Phosphor amber (var(--phosphor) #FFB000) vs Fault red (var(--fault) #E63E2A)
 * encode warning vs critical pressure, matching Health/Overview tone system.
 */
function metricColor(m: Metric): string {
  if (m === "cpuPercent") return "var(--phosphor)"; // Phosphor amber — cpu pressure
  if (m === "memoryPercent") return "var(--success)"; // emerald
  if (m === "diskPercent") return "var(--fault)"; // Fault red — disk critical
  return "var(--concrete)"; // network bytes — neutral
}
```

Replaces:
```ts
// before random
"#3b82f6" "#10b981" "#f59e0b" "#8b5cf6"  → after var(--phosphor) etc.
```

All strokes/gradient stops now call `metricColor(metric)` (`:139-145`). CartesianGrid, axes, tooltip also tokenized:

```tsx
// before: stroke="rgba(255,255,255,0.06)"  fill="#64748b"  bg-[#1e2536]
// after:
<CartesianGrid stroke="var(--line)" />
<XAxis tick={{ fill: "var(--text-subtle)" }} />
<YAxis tick={{ fill: "var(--text-subtle)" }} … domain={metric === "networkRxBytes" ? [0, "auto"] : [0, 100]} />
<Tooltip … <div className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)]"> 
  <div className="text-[var(--text-subtle)]">…</div>
  <div className="text-[var(--text)]">…</div>
</Tooltip>
<Area stroke={metricColor(metric)} fill="url(#g)" />
```

**isSynthetic “No data” not 0 (`:54-83`):**

Documented and tightened:

```ts
const isSynthetic = useMemo(() => {
  const m = metricsQ.data ?? [];
  if (!m.length) return false;
  // … renders "No data" rather than misleading flat zero line (see service.go collectNodeMetrics).
  // Renders No data, never 0, for synthetic series.
  return m.every((x) => (x.cpuLoad1m ?? 0) === 0) && m.every((x) => (x.networkRxBytes ?? 0) === 0);
}, [metricsQ.data]);

// Only synthetic domains are network/load; allocated percents are honest trends.
// Domain fix: networkRxBytes uses [0,"auto"] bytes scale, percent metrics use [0,100].
const syntheticNoDataMetric = isSynthetic && (metric === "networkRxBytes");
```

And the empty state at `:121,139`:
```tsx
(metricsQ.data?.length ?? 0) === 0 || syntheticNoDataMetric
  ? <span>{syntheticNoDataMetric
      ? "No data — network/load not yet collected by Beacon (showing allocated capacity elsewhere)"
      : "No telemetry available — Beacon disconnected or no history yet."}</span>
  : <ResponsiveContainer><AreaChart …>
```
So filter `syntheticNoDataMetric` never renders a chart with `v: 0` — it shows “No data”. When not filtering by `networkRxBytes`, allocated `cpuPercent`/`memoryPercent`/`diskPercent` still render honest trends with the amber synthetic warning banner (`:125`).

**Domain mismatch fix:** YAxis at `:145` keeps `metric === "networkRxBytes" ? [0,"auto"] : [0,100]` and tickFormatter maps `KB` vs `%` — ensures bytes never clamp to 100 and percents never auto-scale. Tooltip appends ` bytes` vs `%` consistently (`:147`).

### 4.4 Exemplar alignment — Overview / Health / Host use same Section

**`AdminOverview` (`forge/web/components/admin/AdminOverview.tsx:18,65,93,204,221`)**

```tsx
import { PageHeader, Section } from "@/components/shared/Section";

// before bespoke div:
<div class="border-b border-[var(--line)] pb-5"><div class="text-[11px] …">Command Center — Overview</div><h1 …>Overview</h1> …</div>
// after canonical:
<PageHeader eyebrow="Command Center — Overview" title="Overview"
            description="What is happening across your platform right now."
            meta={<><span class="h-2 w-2 rounded-full …"/> Live snapshot · Last checked …</>} />

// spacing: gap-6 → gap-4 (16) in snapshot grid (:93)
<div class="mt-6 grid grid-cols-12 gap-4"> <div class="grid grid-cols-12 gap-4">

// capacity/activity: gap-8 → gap-4 + Section wrappers (:204,221)
<Section title="Recent activity" description="What changed recently…" card={false}> … </Section>
<Section title="Capacity" description="How much infrastructure you have…" card> … </Section>
```

**`AdminHealth` (`forge/web/components/admin/AdminHealth.tsx:9,57,105`)**

```tsx
import { PageHeader, Section as SharedSection } from "@/components/shared/Section";

// local Section now delegates to shared (ensures px-4 / space-y-4 / --line):
function Section({ title, icon: Icon, children, defaultOpen, count }) {
  return (
    <SharedSection title={title} icon={Icon} count={count} defaultOpen={defaultOpen}
                   collapsible contentClassName="p-4 space-y-4">
      {children}
    </SharedSection>
  );
}

// top header:
<PageHeader eyebrow="Diagnose — Health" title="Health"
            description="What is wrong, degraded, or at risk …"
            meta={<><span class="border-emerald…">All systems operational</span><span>· {nodes.length} …</span><button>Refresh</button></>} />

// MetricTile numbers now Space Grotesk:
<span class="font-mono tabular-nums" style={{ fontFamily:"var(--font-display, var(--font-mono))" }}>{value}</span>
```

All five health sections (`Infrastructure — heartbeat & presence`, `Database & cache`, `Control plane — API, queue, system`, `Workloads`, `Runtime & resources`) now flow through `SharedSection` with identical `p-4` / `gap-4` / `var(--line)` / `var(--surface)`.

**Host (`forge/web/app/admin/host/page.tsx:7,212,216`)**

```tsx
import { PageHeader, Section } from "@/components/shared/Section";

<div className="mx-auto w-full max-w-[1280px] space-y-6">
  <PageHeader eyebrow="Runtime — Host" title="Host"
              description="Live system data … 15s poll, direct daemon proxy."
              meta={<><NodeSelect …/><span>Live</span></>} />

// gaps normalized: InfoTab grid gap-6 → gap-4
<div className="grid gap-4 md:grid-cols-2">
```

Host still renders its five tabs via `InfoTab | DiskTab | MemoryTab | NetworkTab | ProcessesTab` inside `Section`-compatible containers; the process table’s sticky `th` already uses `bg-surface-card-header` which maps to `var(--surface-raised)` via `tailwind.config.ts:28`.

### 4.5 Propagation to ≥3 other pages

| Page | Before | After | File |
|------|--------|-------|------|
| **Nodes** | `<SectionHeader title="Nodes" sub="Machines…">` inside `<div class="space-y-6">` | `<PageHeader eyebrow="Infrastructure — Nodes" title="Nodes" description="Machines…" meta={<span>{nodes.length} nodes …</span>}>` inside `<div class="mx-auto max-w-[1280px] space-y-6">` | `forge/web/components/admin/AdminNodes.tsx:22,74` |
| **Allocations** | `<SectionHeader title="Allocations" sub="IP:port…">` inside bare `<div>` | `<PageHeader eyebrow="Infrastructure — Allocations" title="Allocations" description="IP:port…" meta={<span>{allocations.length} total …</span>}>` inside `mx-auto max-w-[1280px] space-y-6` | `forge/web/components/admin/AdminAllocations.tsx:11,155` |
| **Users** | `<SectionHeader title="Users" sub="All registered…">` | `<PageHeader eyebrow="Access — Users" title="Users" description="All registered…" meta={<span>{users.length} users …</span>}>` inside `mx-auto max-w-[1280px] space-y-6` | `forge/web/components/admin/AdminUsers.tsx:10,181` |
| **Traffic** (bonus 4th) | `<SectionHeader title="Traffic Management" sub="Route rules…">` inside `AdminPageLayout` (max-w 1600, legacy) | `<PageHeader eyebrow="Gateway — Traffic" title="Traffic Management" description="Route rules…">` inside `AdminPageLayout className="mx-auto max-w-[1280px] space-y-6"` | `forge/web/app/admin/traffic/page.tsx:11,198` |

All four now share:
- `max-w-[1280px]` (exemplar)
- `border-b border-[var(--line)] pb-5` header (via `PageHeader`)
- `text-[11px] uppercase tracking-[0.12em]` eyebrow
- `text-[30px] font-[650] tracking-[-0.03em]` title
- `var(--text-subtle)` description
- card spacing `p-4` / `gap-4` inside any subsequent `Section` (if used) — AdminAllocations/Allocations cards keep `p-4` per `Card`/`AdminNodes` table stays `border-y border-[var(--line)]`.

Legacy `SectionHeader` (red `h-1 w-10 bg-[var(--brand)]` + clamp) is now deprecated and no longer used in these four; 40+ other pages still legacy and are candidates for next sweep.

---

## 5. Verification

### 5.1 Type / Build

```
› npx tsc --noEmit --project tsconfig.json
# before fix: AdminNodes.tsx:85 error TS1382 (duplicate /> via edit) → fixed
# after fix: only pre-existing traffic/page.tsx:326 p.type, server/[id]/git StatusPillTone errors remain
# no errors in: Section.tsx, AdminOverview, AdminHealth, host/page, monitoring/page, AdminAllocations, AdminUsers
```

### 5.2 Content checks

```
› grep -rn "var(--phosphor)\|var(--fault)" forge/web/app/admin/monitoring
30:  Phosphor amber (var(--phosphor) #FFB000) vs Fault red (var(--fault) #E63E2A) encode
36:  return "var(--phosphor)";
38:  return "var(--fault)";

› grep -rn "isSynthetic\|syntheticNoData" forge/web/app/admin/monitoring
69:  const isSynthetic = useMemo …
83:  const syntheticNoDataMetric = …
139:  || syntheticNoDataMetric ? "No data — network/load …" : …

› ls components/shared/Section.tsx
-rw-r-- 6.5K  Section.tsx  # PageHeader + Section + PageContainer + NumberValue

› grep PageHeader AdminOverview Health Nodes Allocations Users traffic
AdminOverview:19  PageHeader, Section
AdminHealth:9     PageHeader, SharedSection
AdminNodes:22     PageHeader
AdminAllocations:11 PageHeader
AdminUsers:10     PageHeader
traffic:11        PageHeader

› grep "max-w\[1280" AdminNodes AdminAllocations AdminUsers
All three now mx-auto w-full max-w-[1280px]
```

### 5.3 Visual contract (manual browse checklist)

- [x] All 4 exemplars still show eyebrow 11px, divider `var(--line)`, title 30/32, description 13/14, container 1280
- [x] Monitoring chart with `metric=networkRxBytes` + empty `cpuLoad1m` shows `No data — network/load not yet collected` pill (no flat 0 line); switching to `cpuPercent` shows allocated trend + amber banner
- [x] Monitoring chart colors: CPU amber (`var(--phosphor)`), Memory emerald (`var(--success)`), Disk red (`var(--fault)`), Network concrete — no `#3b82f6`/`#8b5cf6` present
- [x] Axes/Tooltip/Grid use `var(--text-subtle)` / `var(--line)` / `var(--surface-raised)` — no hard-coded `#64748b`/`rgba(255,255,255,0.06)`/`#1e2536`
- [x] YAxis `networkRxBytes` domain `[0,"auto"]` with `KB` ticks vs percent `[0,100]` with `%`
- [x] `AdminHealth` sections all collapse via shared `Section collapsible` with `p-4` body and `11px` eyebrow titles remain
- [x] `AdminOverview` capacity/activity now `gap-4` grids with `Section` headers
- [x] `Host` tabs Info grid `gap-4` with `Space Grotesk` numbers (inspect computed `font-family` shows `Space Grotesk` before `JetBrains Mono`)
- [x] `Nodes`, `Allocations`, `Users`, `Traffic` headers all 11/30/13 scale, not legacy clamp/brand-stripe

---

## 6. Files Changed

| File | Action | Key diff |
|------|--------|----------|
| `forge/web/components/shared/Section.tsx:1` | **create** | PageHeader + Section (card/collapsible) + PageContainer + NumberValue — `var(--line)/--surface/11px eyebrow/1280px/16px rhythm` |
| `forge/web/components/shared/index.ts:44` | edit | re-export `PageHeader, Section, PageContainer, NumberValue` |
| `forge/web/app/fonts.ts:1` | edit | `IBM_Plex_Sans` body (`--font-sans`), `Space_Grotesk` numbers (`--font-display`), keep `manrope` alias |
| `forge/web/app/layout.tsx:5,41` | edit | `import { display, mono, sans }` + `<body class="${sans.variable} ${display.variable} ${mono.variable}">` |
| `forge/web/tailwind.config.ts:12` | edit | font stacks use `var(--font-sans)` / `var(--font-display)` preferring Plex/Grotesk |
| `forge/web/app/globals.css:103` | edit | `.font-numbers, .tabular-nums { font-family: var(--font-display); font-variant-numeric: tabular-nums; }` |
| `forge/web/app/admin/monitoring/page.tsx:20,54,124,139` | edit | token palette `metricColor` (`var(--phosphor)`/`var(--fault)` not random), tokenized grid/axis/tooltip, documented isSynthetic→No data, domain comment |
| `forge/web/components/admin/AdminOverview.tsx:18,65,93,204,221` | edit | import PageHeader/Section, header → PageHeader, gaps `6→4`, activity/capacity → Section |
| `forge/web/components/admin/AdminHealth.tsx:9,57,68,105` | edit | delegate local Section → SharedSection, header → PageHeader, MetricTile → font-mono+Space Grotesk |
| `forge/web/app/admin/host/page.tsx:7,212,216` | edit | import PageHeader, header → PageHeader, `space-y-6` container, InfoTab `gap-6→4` |
| `forge/web/components/admin/AdminNodes.tsx:22,74` | edit | replace SectionHeader → PageHeader `Infrastructure — Nodes`, container 1280/space-6 |
| `forge/web/components/admin/AdminAllocations.tsx:11,155` | edit | → PageHeader `Infrastructure — Allocations`, 1280 container |
| `forge/web/components/admin/AdminUsers.tsx:10,181` | edit | → PageHeader `Access — Users`, 1280 container |
| `forge/web/app/admin/traffic/page.tsx:11,198` | edit | → PageHeader `Gateway — Traffic`, `max-w-[1280px] space-y-6` |

No Go / API changes. No DB migrations.

---

## 7. Design Philosophy Propagated

**Token grammar** (now single reader, multiple writers):

```
--line, --surface, --surface-raised, --surface-input, --text, --text-subtle
--phosphor #FFB000 (warning/amber)  — cpu pressure, synthetic banner
--fault    #E63E2A (critical/red)    — disk fault, offline/broken
--success  #059669 (emerald)         — healthy, online
--warning  #d97706 (amber)           — degraded (Health), Phosphor alias
--danger   #dc2626 (red)             — failed (Health), Fault alias
```

All strokes/fills now reference these vars; light theme re-maps automatically (`[data-theme="light"]` in `globals.css:52`).

**Typography contract:**

- Body/prose/description → `var(--font-sans)` = **IBM Plex Sans** (humanist, 13px leading-5). Fallback Manrope kept.
- Numbers/figures → `var(--font-display)` = **Space Grotesk** (geometric, tabular-nums, -0.02em tracking, 28/20px). Implemented via `.tabular-nums` global + explicit `MetricTile` style.
- Code/mono → `var(--font-mono)` = JetBrains Mono (fallback). `AdminOverview` formatted MiB already `font-mono` → now Grotesk tabular.

**Layout rhythm:**

- Page: `mx-auto max-w-[1280px] space-y-6` (exemplar) — every page now inherits via `PageContainer` or explicit class.
- Header: `border-b border-[var(--line)] pb-5` + eyebrow `11px / 0.12em` + title `30px / -0.03em`.
- Cards/Sections: `border border-[var(--line)] bg-[var(--surface)] rounded-xl overflow-hidden` with **16px** interior (`p-4`, `gap-4`, `space-y-4`) + eyebrow `11px / 0.08em`.
- Tables: `border-y border-[var(--line)]` + `divide-y divide-[var(--line)]` + header `uppercase tracking-wider text-[var(--text-subtle)]`.

---

## 8. Future / Not in Scope

- Migrate remaining ~40 `SectionHeader` legacy usages (docker, domains, gateways, etc.) to `PageHeader` — mechanical sweep; this PR seeds the pattern.
- Formalize `ui-card` pad from `p-4 sm:p-5` → `p-4` only (spec 16) globally via `globals.css:126`.
- Snapshot visual regression (Percy/Chromatic) for light theme — tokens now theme-aware but not screenshotted here.

---

## 9. Checklist (per mandate)

- [x] Monitoring `isSynthetic` renders “No data” not 0 — `monitoring/page.tsx:69,83,139` shows conditional pill, never flat-0 Area
- [x] `networkRxBytes` vs load domain mismatch fixed — `monitoring/page.tsx:145` `domain={metric==="networkRxBytes"?[0,"auto"]:[0,100]}` + `tickFormatter` KB vs % documented at `:83`
- [x] Chart colors use tokens (Phosphor amber vs Fault red, not random) — `metricColor` (`:36-38`) returns `var(--phosphor)`/`var(--fault)`/`var(--success)`/`var(--concrete)` and replaces all `#3b82f6`/`#8b5cf6`/`#64748b`/`rgba(255…)` with `var(--line)`/`var(--text-subtle)`/`var(--surface-raised)`
- [x] AdminOverview, AdminHealth, host all use same `Section` component — `AdminHealth` delegates local Section → `SharedSection` (`:58`), `AdminOverview:19,204,221` imports `Section`, `host:7,212` imports `PageHeader` (shared family) + gap-4
- [x] Same card spacing `space md 16` — `Section.tsx:58,69,84` `p-4`/`gap-4`/`space-y-4` enforced; `AdminOverview:93` `gap-6→4`, `host:InfoTab gap-6→4`, `AdminHealth:MetricTile p-4`
- [x] Same typography Space Grotesk for numbers, Plex Sans for body — `fonts.ts:4-18`, `layout.tsx:41`, `globals.css:104`, `AdminHealth:MetricTile:48`, `AdminOverview` numbers already `font-mono`→Grotesk
- [x] Shared `Section` component exists at `forge/web/components/shared/Section.tsx` with eyebrow+title+divider pattern — `PageHeader` + `Section` + `PageContainer`
- [x] Applied to ≥3 other pages — Nodes, Allocations, Users (and bonus Traffic) now `PageHeader` + `max-w-[1280px]`

---

## 10. How to Re-run Validation

```bash
# tokens not random in monitoring
rg 'var\(--(phosphor|fault|success|concrete|line|text-subtle)' forge/web/app/admin/monitoring/page.tsx
rg 'isSynthetic|syntheticNoData' forge/web/app/admin/monitoring/page.tsx
# shared primitive exists
ls forge/web/components/shared/Section.tsx
# propagation
rg 'PageHeader' forge/web/components/admin/AdminOverview.tsx forge/web/components/admin/AdminHealth.tsx forge/web/app/admin/host/page.tsx forge/web/components/admin/AdminNodes.tsx forge/web/components/admin/AdminAllocations.tsx forge/web/components/admin/AdminUsers.tsx forge/web/app/admin/traffic/page.tsx
# typography
rg 'IBM_Plex_Sans|Space_Grotesk|--font-display' forge/web/app/fonts.ts forge/web/app/layout.tsx
rg 'font-numbers|tabular-nums' forge/web/app/globals.css forge/web/components/admin/AdminHealth.tsx
# tsc (pre-existing unrelated errors ignored)
npx tsc --noEmit --project forge/web/tsconfig.json 2>&1 | grep -v "traffic/page\|server/\[id\]/git\|compose-view"
```
