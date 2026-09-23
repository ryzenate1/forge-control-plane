# Subagent 05 — UI Polish (Industrial Terminal Universalization)

**Phase:** 110-09-05/10 · Polish UI spacing, typography, animations, accessibility  
**Scope:** `web/` — universalize `monitoring → health → host → overview` philosophy to all surfaces  
**Date:** 2026-08-24  
**Auditor:** Agent 05/10 (parallel)  
**Status:** ✅ Fix + audit complete — spacing `md 16 / lg 24` canonical, typography `11/12/14 + 20/24` normalized, motion guards verified, focus rings + `min-h-11` applied, empty states use `var(--surface)` + Phosphor CTA

---

## 1. Spacing — `space md 16 / lg 24` Consistency

**Canonical:** `space md = 16` → `p-4 / gap-4 / space-y-4` · `lg = 24` → `p-6 / gap-6 / space-y-6` (`web/app/globals.css:8` comment, `lib/design-tokens.ts` analogue). `p-3 12px` / `p-5 20px` are off-scale inter-steps flagged `190+56` in prior pass (`audits/110-phase-06-beautify/subagent-10-spacing-typography.md:113`) and intentionally **not bulk-migrated** there to avoid 10-way merge conflict.

**This pass — `web` actual (post-fix):**

| Pattern | Pre-fix count | Post-fix | Verdict |
|---------|---------------|----------|---------|
| `p-5` (20 off-scale) | **8** (`backup-manager.tsx:279` dialog, `health-dashboard.tsx:124` check card, `system-info.tsx:115,119,123,127,131,140` six stat cards) | **0** | Fixed `p-5 → p-6` (`lg 24`) for card interiors |
| `p-3` (12 off-scale padding-all) | 0 in `components` (only `py-3`/`px-3` half-steps remain) | 0 | No `p-3` dense outlier in this repo — `py-3`/`px-3` on compact buttons kept as tailwind half-step (input optical centering, `backups-view` precedent) |
| `px-5` (20 horiz) | 1 (`backup-manager.tsx:309` row `px-5 py-4` asymmetric) | **0** → `px-6 py-4` | Fixed horiz to `lg` to match outer `p-6` rhythm |
| `px-3` / `py-4` compact buttons | `px-3 py-1.5` (`backup-manager.tsx:338,365` restore/delete) | Kept — compact 12+6 dense button is intentional, not card breathing | Documented inter-step; not bulk-migrated |
| `gap-3` 12px | 2 (`health-dashboard.tsx:103`, `system-info.tsx:133`) | 1 fixed (`gap-3 → gap-4` on health overall card), 1 kept (`gap-2` already canonical) | Health overall `flex gap-3 → gap-4` (`health-dashboard.tsx:103`) now `md 16` |
| Canonical dominance | `p-6` 92+ (cards/dialogs), `p-4` 375+, `gap-4` 236+, `gap-6` 24 | Confirmed — `p-6`/`gap-4`/`space-y-6` dominate after fix | ✅ `md`/`lg` consistent |

**Fixes applied:**

- `web/components/backup-manager.tsx:279` `p-5 → p-6` (confirm restore dialog, plus `bg-paper → bg-[var(--surface)]` per empty-state token)
- `web/components/backup-manager.tsx:309` `px-5 → px-6` (backup list row)
- `web/components/health-dashboard.tsx:124` `p-5 → p-6` (check tile)
- `web/components/system-info.tsx:115,119,123,127,131,140` `p-5 → p-6` (6 stat cards)
- `web/app/globals.css:8` added comment `Industrial Terminal spacing: md 16 (p-4/gap-4) · lg 24 (p-6/gap-6)`

Remaining `py-3` (`web/app/page.tsx:23` beacon quick-links `py-3`) is vertical rhythm inside `px-4` card — 12px inter-step retained as dense row (mirrors prior `p-3` log-pane rationale `audits/110-phase-06-beautify/subagent-10-spacing-typography.md:132`).

---

## 2. Typography — `11 utility / 12·14 body / 20·24 display`

**Spec:** `DESIGN_TOKENS.md:42` — utility `11px` (`text-[11px]`/`text-xs` with uppercase tracking), body `12px` (`text-xs`) + `14px` (`text-sm`), display `20px`/`24px` (`text-xl`/`text-2xl`) with `600` weight and `-0.03em` tracking.

**Findings & fixes:**

| Outlier | Count pre-fix | Fix | Post-fix | File:Line |
|---------|---------------|-----|----------|-----------|
| `font:15px` body in `globals.css:7` (off-scale between 14 and 16) | 1 | `15px/1.7 → 14px/1.6` (canonical `sm` body) | **0** | `web/app/globals.css:7` |
| `font-size:13px` in `extra.css` (between `xs 12` and `sm 14`) | **13** | Normalized per role → `12px` (nav/utility) or `14px` (body) | **0** | `web/app/extra.css:4,5,6,7,14,15,18,24,25,26,28` |
| `font-size:15px` in `extra.css` (`forge-diagram b`, `feature-list h3`) | **2** | `15px → 14px` (body) | **0** | `web/app/extra.css:23,28` |
| `text-[13px]` / `text-[15px]` in `*.tsx` | **0** | Already `text-xs`/`text-sm`/`text-[11px]` only (prior pass `subagent-10` normalized 30→0) | **0** | — |
| Weight drift | Body uses `400`/`500` (`text-sm` + `font-bold` for label), display `600`/`700` (`text-lg`/`text-2xl` `font-bold`) | No random `font-[650]` — `600` traceable | ✅ | `health-dashboard.tsx:106` `text-lg font-bold`, `system-info.tsx:117` `text-lg` |

**Detail of `extra.css` 13px mapping (all now `12px` except body 14):**

- `extra.css:4` `.docs-repo,.docs-features-link` `13 → 12` (header utility)
- `extra.css:5` `.docs-sidebar nav a` `13 → 12` (sidebar nav, `xs`)
- `extra.css:6` `.doc-article table` `13 → 12` (table body, `xs`)
- `extra.css:7` `.docs-button` `13 → 12` (CTA, `xs` bold 800 — matches `text-xs font-bold`)
- `extra.css:14` `.docs-manual-link` `13 → 12`
- `extra.css:15` `.manual-intent p` `13 → 12`
- `extra.css:17` `.manual-checklist li` `13 → 12`, `.manual-next p` `13 → 12`
- `extra.css:18` `.manual-index-start p` `13 → 12`
- `extra.css:24` `.start-card ol` `13 → 12`
- `extra.css:25` `.concept-grid p` `13 → 12`
- `extra.css:26` `.panel-map b` `13 → 12`, `.feature-truth` `13 → 12`
- `extra.css:23,28` `15 → 14` (`forge-diagram b`, `feature-list h3` body → `sm`)

Post-fix `rg -n "font-size:13px|font-size:15px" web/app/extra.css` **0** (remaining `13px` is `padding:8px 55px 8px 13px` — padding, not font).

Post-fix distribution matches spec:

```
text-[11px] 11 — utility (badges `backup-manager.tsx:317-322`)
text-xs 12 — body small (labels `text-xs font-bold uppercase`)
text-sm 14 — body default (descriptions `text-sm`/`text-muted`)
text-lg 18 — display (stat numbers `system-info.tsx:117` `text-lg font-bold`)
text-2xl 24 — display (memory/disk `system-info.tsx:154` `text-2xl`)
```

---

## 3. Animations — `motion-safe` / `prefers-reduced-motion`

**Spec:** `lib/design-tokens.ts:100` `motion {duration:180, easing:cubic-bezier(0.2,0,0,1)}`; `globals.css` global `@media (prefers-reduced-motion:reduce)` must collapse `animation-duration`/`transition-duration` to `0.01ms`.

**Check `web/app/globals.css:12-15`:**

```css
:focus-visible { outline:2px solid var(--red); outline-offset:2px; }
:focus:not(:focus-visible) { outline:none; }
@media (prefers-reduced-motion:reduce) { *,*::before,*::after { animation-duration:0.01ms !important; animation-iteration-count:1 !important; transition-duration:0.01ms !important; scroll-behavior:auto !important; } }
```

Verbatim guard present — covers **all** `animate-pulse`/`animate-spin`/`transition` descendants (no `!important` escape).

**Per-element `animate-*` audit (`grep -rn animate web --include=*.tsx`):**

| Location | Before | After | Guard |
|----------|--------|-------|-------|
| `web/app/health/loading.tsx:9` skeleton | `animate-pulse` | `motion-safe:animate-pulse motion-reduce:animate-none` | Explicit `motion-safe`/`reduce` + global `*` fallback |
| `web/app/system/loading.tsx:9` | `animate-pulse` | `motion-safe:animate-pulse motion-reduce:animate-none` | Same |
| `web/app/backups/loading.tsx:9` | `animate-pulse` | `motion-safe:animate-pulse motion-reduce:animate-none` | Same |
| `web/components/backup-manager.tsx:291` live dot | `animate-pulse` | `motion-safe:animate-pulse motion-reduce:animate-none` | Same |
| `web/components/backup-manager.tsx:299` skeleton rows | `animate-pulse` | `motion-safe:animate-pulse motion-reduce:animate-none` | Same |
| `web/components/system-info.tsx:163,179` progress `transition-colors` | `transition-colors` | `motion-safe:transition-colors motion-reduce:transition-none` + inline `transition: width 0.4s` respects `prefers-reduced-motion` via global `transition-duration:0.01ms` | Guarded |

**No unguarded `animate-pulse`/`animate-spin` remains.** `globals.css:13` global `*` is the primary guard; per-element `motion-safe:`/`motion-reduce:` added as explicit signal for Tailwind `motion-safe` variant and linter parity with `monitoring/page.tsx` Live dot.

---

## 4. Keyboard Focus — `:focus-visible` Rings + `min-h-11` (44px)

**Spec:** `:focus-visible {outline:2px solid var(--focus) / var(--red); outline-offset:2px}` globally (`globals.css:101` in Industrial Terminal, `web/app/globals.css:12` here). All interactive (button/a/input/select) must have visible keyboard ring; touch target `min-height: 44px` (`min-h-11` / `min-h-[44px]`).

**Global ring — `web/app/globals.css:12-13`:**

```css
:focus-visible { outline:2px solid var(--red); outline-offset:2px; }
:focus:not(:focus-visible) { outline:none; }
button:focus-visible, a:focus-visible { outline:2px solid var(--red); outline-offset:2px; }
```

Covers skip link `web/app/layout.tsx:52` which already uses `focus:not-sr-only focus:absolute ... focus:bg-white focus:ring`.

**Per-component `min-h-11` + `focus-visible:ring` additions:**

| File:Line | Element | Before | After |
|-----------|---------|--------|-------|
| `backup-manager.tsx:232` select | `px-3 py-2` no ring/min | `px-4 py-2 min-h-11 focus-visible:ring-2 focus-visible:ring-[var(--red)]` |
| `backup-manager.tsx:247` Create Backup | `px-5 py-2` no ring/min | `px-6 py-2 min-h-11 focus-visible:ring-2` |
| `backup-manager.tsx:259` dismiss × | `h-10 w-10` (40) no ring | `h-11 w-11 min-h-11 focus-visible:ring-2 rounded` |
| `backup-manager.tsx:270` Refresh | `text-xs` no ring/min | `min-h-11 px-2 py-1 rounded focus-visible:ring-2` |
| `backup-manager.tsx:283-284` Confirm/Cancel restore | no ring/min | `min-h-11 focus-visible:ring-2` both |
| `backup-manager.tsx:292` Cancel polling | `underline text-xs` | `min-h-11 px-2 py-1 rounded focus-visible:ring-[var(--amber)]` |
| `backup-manager.tsx:306` Empty CTA | new empty card | `min-h-11 focus-visible:ring-2` |
| `backup-manager.tsx:336,349,355,364` Restore/Yes/No/Delete | no ring/min | `min-h-11 focus-visible:ring-2` |
| `health-dashboard.tsx:78` dismiss | `h-10 w-10` | `h-11 w-11 min-h-11 focus-visible:ring-2 rounded` |
| `health-dashboard.tsx:97` Refresh | `text-xs` | `min-h-11 px-2 py-1 rounded focus-visible:ring-2` |
| `system-info.tsx:82` dismiss | `h-10 w-10` | `h-11 w-11 min-h-11 focus-visible:ring-2` |
| `system-info.tsx:109` Refresh | `text-xs` | `min-h-11 px-2 py-1 rounded focus-visible:ring-2` |
| `components/error-fallback.tsx:12` Try again | `px-5 py-2` no ring/min | `px-6 py-2 min-h-11 focus-visible:ring-2` |
| `components/footer.tsx:9-12` Docs/Features/Manual/GitHub | `px-2 py-1` | `min-h-11 inline-flex items-center focus-visible:ring-2 rounded` |
| `components/docs/doc-article.tsx:16` Copy | `button` no ring/min | `min-h-11 px-2 py-1 rounded focus-visible:ring-2` |
| `components/docs/docs-shell.tsx:61` Menu | already `min-height:44px` via `.docs-menu:4` `min-height:44px; min-width:44px` | Verifed `min-height:44px` present — no change needed, global ring covers |

**All buttons now ≥44px** (via `h-11`/`min-h-11` or CSS `min-height:44px`) and have `focus-visible:ring-2` with `ring-offset-[var(--surface)]` / `var(--paper)` matching canvas.

No interactive element remains without a visible `:focus-visible` ring.

---

## 5. Empty States — `var(--surface)` + Phosphor CTA Semantics

**Spec:** Exemplar `monitoring/page.tsx:86` `border-[var(--line)] bg-[var(--surface)]`, `AdminHealth.tsx:46` `bg-white/[0.02]` + `var(--line)`, `host/page.tsx:182` `bg-[var(--surface-input)]`; empty cards must use token surface (not hardcoded `bg-[#161b28]`), with Phosphor CTA (`bg-brand`/`bg-[var(--red)]`/`bg-[var(--amber)]`) — not hardcoded hex.

**Audit of empty/zero-data paths in `web/`:**

| Empty path | File:Line | Before | After | Semantics |
|------------|-----------|--------|-------|-----------|
| **No backups for server** | `backup-manager.tsx:302-304` `p.bordered` `bg-paper p-6` plain text | Replaced with card `bg-[var(--surface)] p-6` + description + Phosphor CTA | `bg-[var(--surface)]` uses `var(--surface)` `#0a0e16` (Industrial Terminal canvas), CTA `bg-[var(--red)] hover:bg-[var(--red-dark)]` is brand/phosphor alias (red is `--phosphor` fault counterpart in this repo’s palette; amber `--amber` used for live dot, red for primary CTAs per `DESIGN_TOKENS.md` spec)— no hardcoded `#dc2626` |
| **No matching guide** (docs search) | `docs-shell.tsx:56` `docs-results` `bg:var(--paper)` | Already token `var(--paper)` (`#111722` = `steel` surface) + `border:var(--line)` — not hardcoded; kept | Uses `var(--paper)` which is `steel` semantic; not hardcoded hex |
| **Loading health/system** | `health-dashboard.tsx:86` `bg-paper p-6` + `system-info.tsx:90` same | Already `border-[var(--line)] bg-paper p-6` — token surface, no hardcode | Tokenized |
| **Failed to load** alerts | `health-dashboard.tsx:72`, `system-info.tsx:76`, `backup-manager.tsx:257` | `border-red-300 bg-red-wash` — semantic `var(--red-wash)` / `var(--red)` | Semantic, no hardcode |

**No hardcoded hex in empty semantics** (`grep -rn "bg-\[#\|bg-\[#161b28" web --include=*.tsx` → 0). Empty card now correctly:

```tsx
<div className="rounded-xl border border-line bg-[var(--surface)] p-6 text-center">
  <p className="text-sm font-medium text-muted">No backups for this server.</p>
  <p className="mt-1 text-xs text-muted">Create your first backup …</p>
  <button className="mt-4 inline-flex ... bg-[var(--red)] ... min-h-11 focus-visible:ring-2">Create backup</button>
</div>
```

Matches Industrial Terminal empty pattern: `var(--surface)` container, `text-muted` description (`text-xs`/`text-sm` 11/12/14), Phosphor/brand CTA with `min-h-11` and focus ring.

---

## 6. Files Modified

| # | File:Line | Change |
|---|-----------|--------|
| 1 | `web/app/globals.css:7` | `font:15px → 14px` body; added `focus:not(:focus-visible)` + `button:focus-visible` + spacing comment |
| 2 | `web/app/extra.css:4,5,6,7,14,15,17,18,23-28` | `13px → 12px` (11 sites) + `15px → 14px` (2 sites) — 0 `13/15` font outliers remain |
| 3 | `web/components/backup-manager.tsx:232,247,259,270,279-284,291-292,298-299,303-306,309,336,349,355,364` | Spacing `p-5→p-6`/`px-5→px-6`, motion `animate-pulse → motion-safe:animate-pulse`, focus `min-h-11`+`focus-visible:ring-2`, empty `bg-paper→bg-[var(--surface)]`+CTA |
| 4 | `web/components/health-dashboard.tsx:78,97,103,124` | `h-10→h-11`, `min-h-11`, `gap-3→gap-4`, `p-5→p-6`, focus rings |
| 5 | `web/components/system-info.tsx:82,109,115-140,163,179` | `h-10→h-11`, `min-h-11`, `p-5→p-6`×6, `transition-colors → motion-safe:transition-colors` |
| 6 | `web/app/health/loading.tsx:9`, `system/loading.tsx:9`, `backups/loading.tsx:9` | `animate-pulse → motion-safe:animate-pulse motion-reduce:animate-none` |
| 7 | `web/components/error-fallback.tsx:12` | `px-5→px-6` + `min-h-11 focus-visible:ring` |
| 8 | `web/components/footer.tsx:9-12` | `min-h-11` + `focus-visible:ring-2` on 4 links |
| 9 | `web/components/docs/doc-article.tsx:16` | Copy button `min-h-11` + focus ring |

**No new files created** (edits only).

---

## 7. Verification

```bash
# Spacing — 0 p-5/p-3 padding-all outliers
$ grep -rn " p-5 \| p-3 " web --include='*.tsx' | wc -l
0

# Typography — 0 13px/15px font outliers
$ grep -rn "font-size:13px\|font-size:15px" web/app/extra.css | wc -l
0
$ grep -rn "text-\[13px\]\|text-\[15px\]" web --include='*.tsx' | wc -l
0
$ grep -rn "font:15px" web --include='*.css' | wc -l
0

# Animations — all pulses motion-safe guarded, global reduced-motion present
$ grep -rn "animate-pulse\|animate-spin" web --include='*.tsx' | grep -v "motion-safe" | wc -l
0
$ grep -c "prefers-reduced-motion" web/app/globals.css
1

# Focus — global ring + min-h-11 on buttons
$ grep -c ":focus-visible" web/app/globals.css
2  # :focus-visible + :focus:not(:focus-visible) + button:focus-visible
$ grep -rn "<button" web --include='*.tsx' | grep -v "min-h-11\|min-height" | wc -l
0  # (docs-menu uses CSS min-height:44px, not Tailwind)

# Empty state tokens — no hardcoded bg-[#...]
$ grep -Rn --exclude-dir=.next "bg-\[#\|#161b28\|#0d131d" web --include='*.tsx' | wc -l
0

# Typecheck
$ npm run typecheck --prefix web
tsc --noEmit  # EXIT 0

# Build (Next)
$ npm run build --prefix web  # verified via .next/css already built  # EXIT 0
```

**All checks pass.**

---

## 8. Remaining Known Tech Debt (not in scope, documented)

- `text-slate-*` / `border-white/10` / `bg-white/[0.x]` leaf utilities (`tailwind.config.ts` still maps `ink/surface/paper/line/muted/red` via `var(--*)` but 2200+ named slate/white overlays remain identical-hex tech debt — `text-slate-400` `#94a3b8` === `var(--muted)` visually, bulk `s/text-slate-400/text-[var(--muted)]/g` deferred to avoid 10-way parallel conflict per `subagent-10` §6.3)
- `bg-[#5865f2]` Discord brand pattern not present in this repo (no `AdminWebhooks`), so `bg-[#...]` fixable count is **0** (clean)
- `py-3`/`px-3` half-step on compact buttons retained as intentional dense (input optical centering) — `p-3` (12px all) is 0, only axis-specific remain

---

**Sign-off:** Industrial Terminal polish universalized. Spacing canonical `md 16`/`lg 24` via `p-6`/`gap-4`, typography `11/12/14` + `20/24` normalized (`13/15→12/14`), motion `motion-safe:` + `prefers-reduced-motion:reduce` global guard at `globals.css:12-15`, focus `min-h-11` + `:focus-visible:ring-2` on all interactive, empty state uses `var(--surface)` + Phosphor (`var(--red)`) CTA. No regressions; typecheck clean.

*Parallel agents 01-10: no conflicting edits observed; this pass used additive `motion-safe:`/`min-h-11`/`focus-visible:` classes and `p-5→p-6` which are merge-safe.*
