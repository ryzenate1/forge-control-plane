# Forge Design Tokens — Industrial Terminal

Single source: `forge/web/app/globals.css:12` (`:root` + `[data-theme="light"]`).  
Typed JS re-export: `forge/web/lib/design-tokens.ts:1`.  
Tailwind mapping: `forge/web/tailwind.config.ts` (`theme.extend`).

> **Rule:** Do not hardcode hex/rgba in CSS or TSX. Use `var(--*)` in CSS/Tailwind or `tokens.*` in JS (charts, canvas, timelines). All raw `#161b28` etc. must map to a token.

---

## 1. Palette — 6-spec + canonical dark

Spec from `audits/FORGE_IMPLEMENTATION_PLAN.md:25` §1 (Industrial Terminal):

| Name | Hex | Role | Token |
|------|-----|------|-------|
| **Ink** | `#0B1118` | Deep navy-black canvas (not pure black) | `--ink` → `var(--canvas)` dark `#0a0e16` (close, WCAG) |
| **Steel** | `#1B2636` | Panel / card surface | `--steel` → `var(--surface)` `#111722`, `var(--surface-raised)` `#171f2d` |
| **Concrete** | `#8A9BA8` | Secondary text, borders at low opacity | `--concrete` → `var(--line)` `rgba(148,163,184,0.14)` / `--line-strong` |
| **Paper** | `#E6EDF3` | Primary text on dark (warm white) | `--paper` → `var(--text)` `#f1f5f9` |
| **Phosphor** | `#FFB000` | Terminal CRT amber — `running` / live / chart amber | `--phosphor` → alias, charts use `#FFB000` directly; active brand remains `--brand` `#dc2626` |
| **Fault** | `#E63E2A` | Errors, `suspended`/`failed` | `--fault` → alias, runtime `--danger` `#dc2626` (orange-leaning red) |
| Light fallback Amber-700 | `#B45309` | Light-mode warning | `--warning` light `#b45309` |

### Canonical dark tokens (`:root`)

```css
--brand: #dc2626; --brand-hover: #ef4444; --brand-dark: #991b1b; --brand-subtle: rgba(220,38,38,0.10); --brand-line: rgba(220,38,38,0.32);
--canvas: #0a0e16; --surface: #111722; --surface-raised: #171f2d; --surface-input: #0d131d; --surface-well: #080c13; --surface-hover: #1a2233; --nav: #0f141f;
--overlay-subtle: rgba(148,163,184,0.035); --overlay: rgba(148,163,184,0.06); --overlay-strong: rgba(148,163,184,0.10);
--line: rgba(148,163,184,0.14); --line-strong: rgba(148,163,184,0.26); --border: var(--line); --border-strong: var(--line-strong);
--text: #f1f5f9; --text-subtle: #94a3b8; --text-muted: #64748b; --focus: #fb7185;
--ok: #10b981 (+subtle/line); --warn: #f59e0b (+subtle/line); --danger: #f43f5e (+subtle/line); --info: #38bdf8 (+subtle/line); --unknown: #64748b (+subtle/line);
--success/--warning (+subtle) /* legacy aliases of --ok/--warn */;
--ink / --steel / --concrete / --paper / --phosphor / --fault  /* spec aliases */
```

Light (`[data-theme="light"]`): `--canvas: #f4f7fb`, `--surface: #ffffff`, `--surface-raised: #eef2f7`, `--surface-input: #ffffff`, `--surface-well: #f1f5f9`, `--surface-hover: #eef2f7`, `--nav: #ffffff`, `--line: rgba(15,23,42,0.11)` etc., `--brand-subtle: rgba(220,38,38,0.07)`, status `--ok: #047857` / `--warn: #b45309` / `--danger: #be123c` / `--info: #0369a1` / `--unknown: #64748b` (each +subtle/line).

---

## 2. Type

| Role | Family | Weight | Usage |
|------|--------|--------|-------|
| **Body / UI** | `Manrope` | 400/500/600/700 | Everything, including headings (`globals.css:99`); cards, tables, forms, descriptions (12/14 body, 1.5 line-height) |
| **Display** | `Space Grotesk` | 400/500/600/700 | Opt-in only via the `.font-display` utility (`globals.css:102`) |
| **Mono** | `JetBrains Mono` | 400 (variable) | Badges, log lines, server IDs, env keys (`app/fonts.ts` `var(--font-mono)`, `globals.css:106`) |

Implementation: `forge/web/app/fonts.ts` exports `sans` (Manrope → `var(--font-sans)`), `display` (Space Grotesk → `var(--font-display)`), `mono` (JetBrains Mono → `var(--font-mono)`), loaded with `next/font/google`; self-hosting via `next/font/local` is pending the vendored woff2 files. `lib/design-tokens.ts` `type.body` is `Manrope`, `type.display` is `Space Grotesk`, `type.mono` is `JetBrains Mono` — no legacy font names remain. Historical note: earlier revisions named IBM Plex Sans as the body face; the app now uses Manrope exclusively.
Tailwind: `tailwind.config.ts:13` `fontFamily.sans: ["var(--font-sans)","Manrope",...]`, `display: ["var(--font-display)","Space Grotesk",...]`, `mono: ["var(--font-mono)","JetBrains Mono",...]`. Root applies `font-family: var(--font-sans)` in `globals.css:89`.

Scale: 11 utility / 12-14 body / 20-24 display; tracking -0.02 display caps; `prefers-reduced-motion` disables animation (`globals.css:63`).

---

## 3. Spacing

`lib/design-tokens.ts:92`
```ts
export const space = { xs:4, sm:8, md:16, lg:24, xl:32 }
```
Usage: 4pt grid; `gap-3` (12px) and `gap-6` (24px) dominate `monitoring`, `health`, `overview` dense ops-wall grid.

---

## 4. Motion

`lib/design-tokens.ts:100`
```ts
export const motion = { duration:180, easing:"cubic-bezier(0.2,0,0,1)" }
```
CSS: `transition-[background-color,border-color,color,box-shadow,transform] duration-150` on `.ui-button` (`globals.css:77`); `prefers-reduced-motion` collapses to `0.01ms`.

---

## 5. Shadow / Radius

```css
--shadow-flat: 0 1px 2px rgba(0,0,0,0.20);      /* buttons, chips */
--shadow-card: 0 1px 2px rgba(0,0,0,0.20);      /* .ui-card — hairline carries hierarchy, not shadow */
--shadow-raised: 0 4px 16px rgba(0,0,0,0.28);   /* table heads, raised panels */
--shadow-elevated: 0 4px 16px rgba(0,0,0,0.28);
--shadow-popover: 0 12px 32px rgba(0,0,0,0.42);
--shadow-dialog: 0 24px 64px rgba(0,0,0,0.55);  /* .ui-dialog */
--radius-xs: 4px; --radius-sm: 6px; --radius: 8px; --radius-lg: 12px; --radius-full: 9999px;
```
Tailwind: `borderRadius: { sm:"var(--radius-xs)", DEFAULT:"var(--radius-sm)", md:"var(--radius-sm)", lg:"var(--radius)", xl:"var(--radius-lg)", "2xl":"var(--radius-lg)", full:"var(--radius-full)" }`, `boxShadow: { flat/card/raised/elevated/popover/dialog → var(--shadow-*) }` (`tailwind.config.ts`).

---

## 6. Semantic Coverage

All families required by audit must resolve via `var(--*)`:

- `brand / canvas / line / border / text / focus / ok / warn / danger / info / unknown` ✅ (`globals.css:12-70`)
- `nav / surface / surface-hover / surface-well / brand-subtle / brand-line / overlay` ✅
- `shadow / radius` ✅ (`--shadow-flat/card/raised/elevated/popover/dialog`, `--radius-xs/sm/*/lg/full`)

`design-tokens.ts` re-exports each group (`brand`, `canvas`, `line`, `overlay`, `text`, `status`, `shadow`, `radius`) and the `tokens` aggregate; `status` carries the canonical seven (`ok/warn/danger/info/unknown` +subtle/line) plus the `--success`/`--warning` legacy aliases. `colors` retains spec `ink/steel/concrete/paper/phosphor/fault` for chart/tooltip non-theme uses. Status meaning lives in `components/ui/forge/status.ts` (`forgeStatusTones`); `generation-fenced-dot` keeps its own lane hues (violet transfer, sky queued) as a documented exception.

---

## 7. Tailwind Mapping

`tailwind.config.ts:theme.extend`

```
colors.canvas → var(--canvas)
colors.nav → var(--nav)
colors.surface.{DEFAULT,base,secondary,card,card-header,elevated,raised,input,hover,well} → var(--surface*)
colors.well → var(--surface-well)
colors.overlay.{DEFAULT,subtle,strong} → var(--overlay*)
colors.border.{DEFAULT,strong} → var(--border*)
colors.line.{DEFAULT,strong} → var(--line*)
colors.text.{DEFAULT,subtle,muted,focus} → var(--text*)
colors.brand.{DEFAULT,hover,dark,subtle,line} → var(--brand*)
colors.ok/warn/info/unknown.{DEFAULT,subtle,line} → var(--*)
colors.success/warning.{DEFAULT,subtle} → var(--*)  /* legacy aliases */
colors.danger.{DEFAULT,subtle,line} → var(--danger*)
colors.ink/steel/concrete/paper/phosphor/fault → var(--*)
boxShadow.flat/card/raised/elevated/popover/dialog → var(--shadow-*)
borderRadius sm/DEFAULT/md/lg/xl/2xl/full → var(--radius-xs/sm/sm/radius/lg/lg/full)
```

No hardcoded hex remains in `tailwind.config.ts`.

> Tailwind v3 caveat: the `/opacity` modifier does not emit for colors backed
> by `var(--*)` (e.g. `bg-brand/10` compiles to nothing). Opacity over a token
> must use `bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]` (see
> `globals.css` scrollbar/focus treatments and `admin-shell.tsx` nav active
> state). Raw `border-white/10`, `bg-white/[0.0x]`, `bg-black/20` and
> `text-slate-*` are likewise banned outside data-driven color (tag pills,
> chart SVG via `lib/design-tokens.ts` `chart`): use `border-line`,
> `bg-overlay(-subtle/-strong)`, `bg-well` and `text-text(-subtle/-muted)`.

---

## 8. Hardcode Audit

Run: `grep -rn "bg-\[#" forge/web --include="*.tsx" --include="*.ts"` (excludes `.next`)

- **Before:** ~311 `bg-[#...]` hardcodes across `forge/web/app` + `components` (traffic `bg-[#161b28]`, mtls `bg-[#dc2626]`, git-providers `bg-[#111722]`, etc.).
- **After (this pass):** **1** remaining `bg-[#5865f2]` (Discord brand, intentionally exempt in `components/admin/AdminWebhooks.tsx:251`). All other `bg-[#161b28] / #0d131d → bg-[var(--surface-input)]`, `bg-[#111722] → bg-[var(--surface)]`, `bg-[#1e2536] / #151b27 → bg-[var(--surface-raised)]`, `bg-[#0a0e16]/#0a0e14/#090d14/#020617 → bg-[var(--canvas)]`, `bg-[#0f1419] → bg-[var(--surface)]`, `bg-[#11161f] → bg-[var(--surface)]`, `bg-[#dc2626] → bg-[var(--brand)]`, `hover:bg-[#b91c1c] → hover:bg-[var(--brand-dark)]` — batch replaced via script (88 files touched, `audits/110-phase-06-beautify/subagent-01-tokens.md:§3`).

Remaining `border-white/10`, `text-slate-*` are thematic overlays; exemplar pages use `border-[var(--line)]` / `text-[var(--text-subtle)]` where theme-aware (e.g. `monitoring/page.tsx:86` `border-[var(--line)] bg-[var(--surface)]`). `border-white/[0.04]` retained only for selected `bg-white` pills that must invert (e.g. `monitoring/page.tsx:88` `bg-white text-slate-900`).

Check exemplar propagation:
- `monitoring/page.tsx:86` ✅ controls use `border-[var(--line)] bg-[var(--surface)]`
- `components/admin/AdminHealth.tsx:46` ✅ `border-[var(--line)] bg-white/[0.02]`
- `app/admin/host/page.tsx:182` ✅ `border-[var(--line)] bg-[var(--surface-input)]`
- `components/admin/AdminOverview.tsx:142` ✅ `border-[var(--line)] bg-white/[0.02]`

Registry `tailwind.config.ts:13` enforces `Manrope` (body) + `Space Grotesk` (display utility) per §2; `app/fonts.ts` provisions Manrope, Space Grotesk and JetBrains Mono.

---

## 9. Usage

```tsx
// CSS / Tailwind (preferred)
<div className="border border-[var(--line)] bg-[var(--surface)] text-[var(--text)] rounded-[var(--radius)] shadow-[var(--shadow-card)]" />

// JS (charts, canvas)
import { tokens, colors, chart } from "@/lib/design-tokens";
<Area stroke={colors.phosphor} fill={tokens.brand.DEFAULT} />
<CartesianGrid stroke={chart.grid} />   // SVG attrs can't resolve var(--*)
```

Never: `bg-[#161b28]`, `border-white/10` for cards, `text-[#dc2626]`. Lint by `grep -rn "bg-\[#\|border-\[#\|#161b28" forge/web/lib forge/web/app forge/web/components` — should be 0 outside `globals.css`/`design-tokens.ts` (exception: `AdminWebhooks.tsx` Discord).

---

## 10. Files

- `forge/web/app/globals.css:12` — dark `:root` + `[data-theme="light"]` token blocks
- `forge/web/lib/design-tokens.ts:1` — `brand/canvas/line/overlay/text/status/shadow/radius` + `tokens`, `colors`, `type`, `space`, `motion`, plus `chart`/`terminalTheme` literal palettes for SVG/canvas contexts
- `forge/web/tailwind.config.ts` — theme.extend maps to `var(--*)`
- `forge/web/app/fonts.ts` — Manrope / Space Grotesk / JetBrains Mono via `next/font`
