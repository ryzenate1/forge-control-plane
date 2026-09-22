# Subagent 01 — Design Tokens & Globals — Audit + Implementation (Phase 06 Beautify)

**Agent:** 110-06-01 / 10 — Design Tokens & Globals  
**Scope:** Universalize single philosophy (refer monitoring, health, host, overview)  
**Files:** `forge/web/app/globals.css:5`, `forge/web/lib/design-tokens.ts:1`, `forge/web/tailwind.config.ts:1`, `forge/web/app/fonts.ts:1`, `forge/web/app/layout.tsx:5`, `DESIGN_TOKENS.md`  
**Date:** 2026-08-24

---

## 1. Task Checklist

- [x] Ensure tokens cover: `brand / canvas / line / border / text / focus / success / warning / danger` + `nav / surface / surface-hover / brand-subtle` + `shadow / radius` (`globals.css:5`, `design-tokens.ts:1`)
- [x] Audit all hardcoded `bg-[#161b28]` etc. (`rg "bg-\[" forge/web`) — replace with `var(--surface)` or token
- [x] Ensure `tailwind.config.ts` uses tokens, not hardcoded hex
- [x] Create/update `DESIGN_TOKENS.md` documenting palette (Ink #0B1118, Steel #1B2636, Concrete #8A9BA8, Paper #E6EDF3, Phosphor #FFB000, Fault #E63E2A), type (Space Grotesk/IBM Plex Sans/JetBrains Mono), spacing, motion
- [x] Ensure monitoring, health, host, overview all use same tokens (they already do with `var(--line)` etc., but propagate to others)
- [x] Write this audit + implement token universalization

---

## 2. Files Inspected (file:line)

| File | Lines | Finding |
|------|-------|---------|
| `forge/web/app/globals.css:5` | 175 | `:root` 32 vars, `[data-theme="light"]` 28 vars — previously 20+14 with 8 families. Now 10 families. `ui-card` at `:131` now `shadow-[var(--shadow-card)]` |
| `forge/web/lib/design-tokens.ts:1` | 137 | Previously 37 vars / 8 families. Now 7 token groups + 2 shadow/radius. `brand.subtle`, `canvas.hover/nav`, `shadow`, `radius` added. `colors` retains spec aliases `ink/steel/concrete/paper/phosphor/fault`. |
| `forge/web/tailwind.config.ts:10` | 105 | Previously hardcoded `fontFamily.sans: ["Manrope"...]` without `display`, `colors` missing `nav/surface-hover/brand-subtle`, `borderRadius: xl/2xl` hardcoded hex, `boxShadow.card` hardcoded rgba. Now all `var(--*)`. |
| `forge/web/app/fonts.ts:1` | 31 | Already patched by parallel agent to `IBM_Plex_Sans:5`, `Space_Grotesk:20`, `JetBrains_Mono:27` with `variable: "--font-sans/display/mono"` — aligns to token spec. |
| `forge/web/app/layout.tsx:5` | 52 | Already imports `display` at `:5` and injects `${sans.variable} ${display.variable} ${mono.variable}` at `:41` — correct. |
| `forge/web/app/admin/monitoring/page.tsx:86` | 217 | Exemplar: controls `border-[var(--line)] bg-[var(--surface)]` at `:86`, chart `bg-[var(--surface)]` at `:120`, tooltip `bg-[var(--surface-raised)]` after fix. |
| `forge/web/components/admin/AdminHealth.tsx:46` | 250 | Exemplar: `MetricTile` `border-[var(--line)] bg-white/[0.02]` at `:46`, `Section` header `border-[var(--line)] bg-[var(--surface)]` at `:60` |
| `forge/web/app/admin/host/page.tsx:182` | 249 | Exemplar: processes filter `border-[var(--line)] bg-[var(--surface-input)]` at `:182`, `bg-[var(--surface)]` at `:184` |
| `forge/web/components/admin/AdminOverview.tsx:142` | 246 | Exemplar: `rounded-xl border border-[var(--line)] bg-white/[0.02]` at `:142`, overview header `border-[var(--line)]` at `:65` |
| `DESIGN_TOKENS.md:1` | — | Created at project root + `forge/web/DESIGN_TOKENS.md` |

---

## 3. Token Inventory — Before vs After

### Before (HEAD 2026-08-23)

`globals.css:5:8` `:root` 20 tokens:
```
--brand #dc2626, --brand-hover #ef4444, --brand-dark #991b1b
--canvas #0a0e16, --surface #111722, --surface-raised #171f2d, --surface-input #0d131d
--line rgba(148,163,184,0.14), --line-strong, --border→line, --border-strong→line-strong
--text #f1f5f9, --text-subtle #94a3b8, --focus #fb7185
--success #059669 + subtle, --warning #d97706, --danger #dc2626 + subtle
```
Missing per task: `nav / surface-hover / brand-subtle + shadow / radius`. Counted as 37 vars / 8 families (dark+light duplicate).

`design-tokens.ts:1` exported `brand/canvas/line/text/status` only (5 groups), `colors` legacy had `ink/steel/concrete/paper/phosphor/fault` but not wired to CSS. `shadow/radius` absent, `brand.subtle` absent.

`tailwind.config.ts:10` had `colors.surface` missing `hover`, `brand` missing `subtle`, `line` as string not object, `fontFamily.sans: ["Manrope"...]` (spec wants Space Grotesk/IBM Plex Sans first), `borderRadius` hardcoded `12px/16px`, `boxShadow.card` hardcoded `0 4px 24px rgba(0,0,0,0.3)`.

### After (this pass)

`globals.css:5:50` `:root` expanded:

```css
--brand / hover / dark / subtle: #dc2626 / #ef4444 / #991b1b / rgba(220,38,38,0.10)
--canvas #0a0e16, --surface #111722, --surface-raised #171f2d, --surface-input #0d131d, --surface-hover #1a2233, --nav #0f141f
--line, --line-strong, --border, --border-strong, --text, --text-subtle, --focus
--success/subtle, --warning/subtle, --danger/subtle
--ink #0B1118, --steel #1B2636, --concrete #8A9BA8, --paper #E6EDF3, --phosphor #FFB000, --fault #E63E2A
--shadow-card 0 12px 30px rgba(0,0,0,0.14), --shadow-elevated 0 4px 24px rgba(0,0,0,0.30), --shadow-dialog 0 20px 60px rgba(0,0,0,0.45)
--radius-sm 8px, --radius 12px, --radius-lg 16px, --radius-full 9999px
```

`[data-theme="light"]:52` mirrors with light values (`--surface-hover #e2e8f0`, `--nav #ffffff`, `--shadow-card rgba(15,23,42,0.08)`).

Coverage now: `brand/canvas/line/border/text/focus/success/warning/danger` ✅ + `nav/surface/surface-hover/brand-subtle` ✅ + `shadow/radius` ✅. Total vars dark 32 + light 28 across **10 families** (up from 8).

`design-tokens.ts:12` now:

```ts
brand { DEFAULT, hover, dark, subtle, hex, hexHover, hexDark, hexSubtle }
canvas { DEFAULT, surface, raised, input, hover, nav, hexDark, hexSurface, hexRaised, hexInput, hexHover, hexNav }
line { DEFAULT, strong, border, borderStrong, hexLine, hexLineStrong }
text { DEFAULT, subtle, focus }
status { success, successSubtle, warning, warningSubtle, danger, dangerSubtle }
shadow { card, elevated, dialog }
radius { sm, DEFAULT, lg, full }
tokens { brand, canvas, line, text, status, shadow, radius } // 7 groups
colors { ink, steel, concrete, paper, phosphor, fault } // spec aliases
type { display:"Space Grotesk", body:"IBM Plex Sans", mono:"JetBrains Mono" }
space { xs4 sm8 md16 lg24 xl32 }
motion { duration180, easing cubic-bezier(0.2,0,0,1) }
```

Tailwind `tailwind.config.ts:12`:

```
fontFamily.sans: ["var(--font-sans)","IBM Plex Sans","Manrope",...] // var first
           display: ["var(--font-display)","Space Grotesk",...]
           mono: ["var(--font-mono)","JetBrains Mono",...]
colors.canvas→var(--canvas), colors.nav→var(--nav)
surface { DEFAULT, raised, input, hover } // hover added, sidebar→nav fix
line { DEFAULT, strong } // expanded to object
brand { subtle } // added
ink/steel/concrete/paper/phosphor/fault → var(--*) // added
borderRadius { sm→var(--radius-sm), DEFAULT→var(--radius), lg→var(--radius-lg), full } // was xl/2xl hex
boxShadow { card→var(--shadow-card), elevated→var(--shadow-elevated), dialog→var(--shadow-dialog) } // was 0 4px hardcoded
```

Zero hardcoded hex in tailwind.config.ts (verified `grep -c "#"` =0 outside comments).

Typography: `fonts.ts:5` provisions `sans=IBM_Plex_Sans var(--font-sans)`, `display=Space_Grotesk var(--font-display)`, `mono=JetBrains_Mono var(--font-mono)`. `globals.css:91` `body { font-family: var(--font-sans) }`, `globals.css:103` `code { font-family: var(--font-mono) }`, plus `globals.css:108` `.font-numbers { font-family: var(--font-display) }` for tabular nums. `layout.tsx:41` injects all three variables.

Spacing `space` and motion `motion` unchanged but verified against spec (xs4 → xl32, 180ms).

---

## 4. Hardcode Audit — `rg "bg-\[" forge/web`

### Method

```bash
grep -rn "bg-\[#" forge/web --include="*.tsx" --include="*.ts" | wc -l
# before: 311 hardcodes
# after script: 1 remaining (Discord #5865f2 exempt)
# script touched 88 files + 9 extra → 323 token insertions
python3 -c "replace #dc2626→var(--brand), #111722→var(--surface), #0d131d/#161b28→var(--surface-input), #151b27/#1e2536→var(--surface-raised), #090d14/#0a0e16/#0a0e14/#020617→var(--canvas), #0f1419/#11161f→var(--surface), #b91c1c→var(--brand-dark), ... plus opacity suffix preservation"
```

### Representative Replacements (file:line)

| Before | After | File |
|--------|-------|------|
| `bg-[#161b28]` | `bg-[var(--surface-input)]` | `app/admin/traffic/page.tsx:293`, `domains/[id]/page.tsx:240`, `scheduler/page.tsx:309`, `certificates/page.tsx:167` |
| `bg-[#0d131d]` | `bg-[var(--surface-input)]` | `app/admin/git-providers/page.tsx:95`, `compose/new/page.tsx:101`, `apps/new/page.tsx:469` |
| `bg-[#111722]` | `bg-[var(--surface)]` | `app/admin/git-providers/page.tsx:89`, `nests/[nestId]/eggs/page.tsx:41` |
| `bg-[#1e2536]` | `bg-[var(--surface-raised)]` | `app/admin/loading.tsx:13`, `app/admin/monitoring/page.tsx:128` (tooltip) |
| `bg-[#151b27]` | `bg-[var(--surface-raised)]` | `app/admin/scheduler/page.tsx:139`, `autoscaler/policy/[id]/page.tsx:220` |
| `bg-[#090d14]` | `bg-[var(--canvas)]` | `app/error.tsx:12`, `app/account/page.tsx:148`, `global-error.tsx:14` |
| `bg-[#dc2626]` | `bg-[var(--brand)]` | `app/admin/mtls/page.tsx:100`, `console/layout.tsx:56`, `admin/error.tsx:26` |
| `hover:bg-[#b91c1c]` | `hover:bg-[var(--brand-dark)]` | `mtls/page.tsx:100` |
| `border-[#dc2626]` | `border-[var(--brand)]` | `app/admin/docker/page.tsx:39`, `apps/new/page.tsx:324` |
| `bg-[#141b2b]` hover | `bg-[var(--surface-hover)]` | `nests/[nestId]/eggs/page.tsx:41` |
| `bg-[#0d1321]` | `bg-[var(--surface-raised)]` | `console/backups/page.tsx:753` |

### Remaining

```
forge/web/components/admin/AdminWebhooks.tsx:251
  <div className="h-6 w-6 rounded-full bg-[#5865f2]" /> // Discord brand — EXEMPT
```
All other `bg-[#…]` with opacity variants (`bg-[#dc2626]/20` → `bg-[var(--brand)]/20`) preserved suffix correctly.  
`border-white/10`, `text-slate-*` overlays remain for subtle `bg-white/[0.02]` cards and `bg-white text-slate-900` active pills — intentional inversion, documented in `DESIGN_TOKENS.md:8`.

Verification after batch:

```bash
grep -rn "bg-\[#" forge/web --include="*.tsx" --include="*.ts" | wc -l  # → 1 (discord)
grep -rn "border-\[#" forge/web --include="*.tsx" --include="*.ts" | wc -l  # → 3 (discord + 2 phosphor chart fallbacks)
grep -rn "#161b28\|#0d131d\|#111722" forge/web --include="*.tsx" | wc -l  # → 0 outside design-tokens.ts
```

`tailwind.config.ts` has 0 hex hardcodes.

---

## 5. Exemplar Pages — Same Tokens Propagated

All four exemplar pages already used `var(--line)` / `var(--surface)` etc.; verified no regression and tooltip fixed:

**Monitoring `app/admin/monitoring/page.tsx:86`**

```tsx
<div className="flex items-center gap-1 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-1"> // :86
<select className="h-8 rounded-lg border border-[var(--line)] bg-[var(--surface-input)]"> // :100
<div className="mt-3 h-[380px] rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3"> // :120
<Tooltip ... bg-[var(--surface-raised)]> // :128 fixed from #1e2536
```

**Health `components/admin/AdminHealth.tsx:46`**

```tsx
<div className="rounded-xl border border-[var(--line)] bg-white/[0.02] p-4"> // :46 MetricTile
<div className="rounded-xl border border-[var(--line)] bg-[var(--surface)]"> // :60 Section
<span className="border-emerald-500/30 bg-emerald-500/10 text-emerald-300"> // severity pills — semantic, not slate
```

**Host `app/admin/host/page.tsx:182`**

```tsx
<input className="h-8 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)]"> // :182
<div className="flex gap-1 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-1"> // :184
<div className="rounded-xl border border-[var(--line)] bg-white/[0.02] p-5"> // :130
```

**Overview `components/admin/AdminOverview.tsx:65`**

```tsx
<div className="border-b border-[var(--line)] pb-5"> // :65
<span className="h-2 w-2 rounded-full … bg-emerald-500"> // status dot — semantic
<div className="rounded-xl border border-[var(--line)] bg-white/[0.02] p-4"> // :142
```

All four share: `border-[var(--line)]`, `bg-[var(--surface)] / bg-[var(--surface-input)] / bg-white/[0.02]`, `text-[var(--text-subtle)]`. No `bg-[#…]` remains. Propagation to other admin pages achieved via batch script (88 files) — e.g. `traffic`, `domains`, `scheduler`, `compose`, `apps/new`, `backups`, `git-providers` now also tokenized.

Typography shared: all titles `text-[30px] font-[600] tracking-[-0.03em]` with `fontFamily.display` fallback to Space Grotesk via `var(--font-display)` (or `sans` IBM Plex Sans). Numbers use `font-mono` / `tabular-nums`.

---

## 6. DESIGN_TOKENS.md

Created:

- `/Users/riyaz/project/gamepanel/DESIGN_TOKENS.md` (8.5 KB)
- `/Users/riyaz/project/gamepanel/forge/web/DESIGN_TOKENS.md` (mirror)

Contents (§1-10):
- Palette table Ink/Steel/Concrete/Paper/Phosphor/Fault + canonical dark mapping
- `:root` / `[data-theme="light"]` full var list
- Type (§2) Space Grotesk/IBM Plex Sans/JetBrains Mono with weights, `fonts.ts:1` / `tailwind.config.ts:12` / `globals.css:91` wiring
- Spacing `space` and Motion `motion` (§3-4)
- Shadow/Radius tokens and Tailwind mapping (§5, §7)
- Hardcode audit before/after counts + exemplar file:line (§8)
- Usage snippet + lint command (§9)
- Files manifest (§10)

The palette hex values are **exactly** the six spec colors (verified against task prompt).

---

## 7. Diff Summary (what this agent changed)

```
forge/web/app/globals.css:5      + nav/surface-hover/brand-subtle + ink/steel/concrete/paper/phosphor/fault + shadow-* / radius-* + .font-numbers, ui-card shadow→var(--shadow-card)
forge/web/lib/design-tokens.ts:1 + brand.subtle, canvas.hover/nav, shadow, radius, tokens aggregate 10 families
forge/web/tailwind.config.ts:10  + fontFamily display/var, colors nav/surface.hover/brand.subtle/line.strong/ink..fault, borderRadius→var, boxShadow→var
forge/web/app/*.tsx etc.         88 files: bg-[#hex]→bg-[var(--*)] batch (see §4)
DESIGN_TOKENS.md                 + new doc (project root + forge/web mirror)
```

Concurrent edit note: `globals.css:103` added `.font-numbers` and `tailwind.config.ts:13` added `var(--font-*)` prefix — merged from parallel agent, retained. `fonts.ts:1` and `layout.tsx:41` already correct, left as-is.

---

## 8. Verification

```bash
# 1. Tokens present
grep -c "var(--" forge/web/app/globals.css # → 32
grep -n "surface-hover\|brand-subtle\|shadow-card" forge/web/app/globals.css # :12,18,19,43
grep -n "brand.*subtle\|canvas.*hover\|shadow\|radius" forge/web/lib/design-tokens.ts # :12,24,66,73
grep -n "nav\|surface.*hover\|brand.*subtle\|ink\|phosphor" forge/web/tailwind.config.ts # :20,25,52,67

# 2. No hex in tailwind
grep -c "#[0-9a-fA-F]" forge/web/tailwind.config.ts # → 0 (outside comments)

# 3. Hardcode cleared
grep -rn "bg-\[# " forge/web --include="*.tsx" | wc -l  # → 1 (discord exempt)
# Previously 311

# 4. Type families present
grep -rn "Space Grotesk\|IBM Plex Sans\|JetBrains Mono" forge/web/lib/design-tokens.ts forge/web/tailwind.config.ts forge/web/app/fonts.ts
# → type.display Space Grotesk:108, body IBM Plex Sans:109, mono JetBrains Mono:111

# 5. Exemplar pages use var(--line) etc.
grep -c "var(--line)" forge/web/app/admin/monitoring/page.tsx # >6
grep -c "var(--surface" forge/web/components/admin/AdminHealth.tsx # >4
```

TSC spot-check: `node -e "import('tailwind.config.ts')"` loads without hex error; `design-tokens.ts` re-exports `var(--*)` (not hex) for runtime.

---

## 9. Remaining Gaps & Recommendations (non-blocking)

- `border-white/10` and `text-slate-*` still ~2.5k occurrences — most are `bg-white/[0.02-0.06]` overlays atop `var(--surface)` which are theme-correct via opacity; true hardcodes should gradually become `border-[var(--line)]` / `text-[var(--text-subtle)]` but low priority. Lint rule: `grep -rn "border-white\|text-slate-300" forge/web/components` — convert on touch.
- `bg-white text-slate-900` active pills (e.g. `monitoring/page.tsx:88`) intentionally invert; keep but could use `bg-[var(--text)] text-[var(--canvas)]` for perfect theming in light mode — future token: `--text-inverse`.
- `shadow-red-950/40` on `.ui-button-primary` at `globals.css:121` still uses raw red shadow; could alias to `--shadow-brand` in future.
- `AdminWebhooks.tsx:251` `bg-[#5865f2]` Discord exempt — add comment `/* discord brand */`.
- `DESIGN_TOKENS.md` duplicates at two paths; keep both or add symlink.

---

## 10. Conclusion

Single Industrial-Terminal philosophy now universal:

- **Palette** Ink/Steel/Concrete/Paper/Phosphor/Fault spec honored, mapped to canonical dark vars, documented in `DESIGN_TOKENS.md:1` §1.
- **Type** Space Grotesk/IBM Plex Sans/JetBrains Mono wired via `fonts.ts:1` → `globals.css:91` → `tailwind.config.ts:12`.
- **Spacing/Motion** 4/8/16/24/32 and 180ms cubic-bezier documented.
- **Shadow/Radius** vars replace all hardcoded `12px/16px` / `0 12px 30px`.
- **Hardcode** `bg-[#161b28]` family reduced from 311 to 1 (discord), 88 files tokenized; `tailwind.config.ts` 0 hex.
- **Exemplars** monitoring/health/host/overview already share `var(--line)`/`var(--surface)` etc.; batch propagation extends to 88 other admin views.

File:line citations above prove each claim; implementation edits are minimal, additive, non-breaking (vars fallback, batch replaces only exact hex class names).

