# Subagent 15 — Design System & Shared Components (Phase 08)

**Agent:** 110-08-15 / 20  
**Focus:** spacing, typography, tokens — `forge/web/lib/design-tokens.ts`, `forge/web/app/globals.css:5`, `tailwind.config.ts`, `forge/web/components/shared/*`  
**Date:** 2026-08-24

---

## 1. Inspected Sources

| File | Key Findings |
|------|--------------|
| `forge/web/lib/design-tokens.ts:1` | 7 semantic groups + aggregate `tokens` — `brand:13`, `canvas:25`, `line:41`, `text:50`, `status:57`, `shadow:67`, `radius:73`, `tokens:81`, legacy `colors:93`, `type:108`, `space:114`, `motion:122`, `statusTones:128`. All `var(--*)` values mirror `globals.css:5`. |
| `forge/web/app/globals.css:5` | `:root:5` declares 32 vars (dark) + `[data-theme="light"]:52` 28 vars. Brand `#dc2626`, canvas `#0a0e16`/`#111722`, line `rgba(148,163,184,0.14)`, text `#f1f5f9`/`#94a3b8`, success/warning/danger, plus legacy `--ink`/`--steel`/`--concrete`/`--paper`/`--phosphor`/`--fault` aliases. Shadows/radius declared. `body:91` uses `var(--font-sans)`, `code:105` uses `var(--font-mono)`, `.font-numbers:108` uses `var(--font-display)`. |
| `forge/web/tailwind.config.ts:10` | `theme.extend.colors` maps every semantic token to `var(--*)` — `canvas:19`, `surface:21`, `border:36`, `line:40`, `text:44`, `brand:48`, `success:54`, `warning:58`, `danger:62`, legacy `ink:67`–`fault:72`. `fontFamily:12` prefers `var(--font-sans)`→IBM Plex Sans, `var(--font-display)`→Space Grotesk, `var(--font-mono)`→JetBrains Mono. `borderRadius:87`/`boxShadow:95` via `var(--radius*)`/`var(--shadow*)`. **Zero quoted hex** — no hardcoded hex. |
| `forge/web/app/fonts.ts:1` | `IBM_Plex_Sans:5` (`var(--font-sans)`), `Space_Grotesk:20` (`var(--font-display)`), `JetBrains_Mono:27` (`var(--font-mono)`), all `display:swap`, weights 400/500/600/700. `Manrope:13` retained as `--font-sans-manrope` fallback. `app/layout.tsx:5` imports `display/mono/sans` and applies `${sans.variable} ${display.variable} ${mono.variable}:41` to `<body>`. |
| `forge/web/components/shared/states-empty.tsx:1` | `EmptyCard:17` + 9 variants use `border-[var(--line)]`/`bg-[var(--surface)]`/`bg-[var(--surface-raised)]`/`text-[var(--text-subtle)]` — fully theme-aware. |
| `forge/web/components/shared/states-loading.tsx:1` | `Skeleton:6` `bg-[var(--surface-raised)] border-[var(--line)] animate-pulse`; `SkeletonList:10`/`SkeletonDetail:35`/`SkeletonForm:61` with `role=status`, `SpinnerInline:81`/`SpinnerPage:90`/`SpinnerButton:101` via `LoaderCircle`. |
| `forge/web/components/shared/generation-fenced-dot.tsx:1` | `stateToDotClass:19` maps states to `bg-[var(--success)]`/`bg-[var(--warning)]`/`bg-violet-500`/`bg-[var(--danger)]`/`bg-sky-500`/`bg-[var(--text-subtle)]`. `GenerationFencedDots:31` stacked `flex-col gap-[2px]` with two `rounded-full border` dots (`size` default 7), `fenced` when `generation < fenceGeneration` OR `isFenced`, second dot gets `ring-2 ring-[var(--danger)]/70` when fenced else `opacity-60` for `pending`. `StateLanesBadge:87` horizontal pill with `GenerationFencedDots` + `desired / actual` mono text + `g{n}→f{n} fenced`. `ServerStateLaneBadge:127`/`NodeStateLaneBadge:139` aliases. |
| `forge/web/DESIGN_TOKENS.md:1` / `forge/web/DESIGN_TOKENS.md:1` (project root) | Documents Industrial Terminal spec, token table, hardcode audit (311→1, only `bg-[#5865f2]` Discord). |

**Existing tests for design tokens/design system:** none dedicated; `test/app-ux-18.test.tsx:1` covers `statusTone`/`APP_TYPE_ICONS`/`useDeploymentSteps`/`restartComposeStack`/tab routing — no direct token/font/hardcode or `generation-fenced-dot` coverage. `test/ui-contracts.test.tsx:1`/`test/server-detail-layout.test.tsx` etc. likewise.

**Hardcode audit (`grep -rn "bg-\[#"`):**
- Only 1 hit outside `.next`/`DESIGN_TOKENS.md`: `forge/web/components/admin/AdminWebhooks.tsx:251` `bg-[#5865f2]` (Discord Blurple, intentionally exempt per `DESIGN_TOKENS.md:128`). Verified collapses to 0 when that hex is allowed.

---

## 2. Test File Created/Augmented

**New:** `forge/web/test/design-system.test.tsx` — 41 tests in 8 suites.

### Coverage matrix (task → tests)

| Task bullet | Tests | File refs |
|-------------|-------|-----------|
| **Tokens exist (brand/canvas/line/text etc.)** | `brand`/`canvas`/`line`/`text`/`status`/`shadow`/`radius`/`tokens`/`colors`/`statusTones`/`space`/`motion`/`type`; `globals.css:5` var declarations; `tailwind.config.ts` var mapping + zero hex; `states-*` theme-aware spot-check | `forge/web/test/design-system.test.tsx:24-120`, `lib/design-tokens.ts:13-137` |
| **No hardcoded `bg-[#...]` (except blurple)** | filesystem walk over `app/`, `components/`, `lib/`, `stores/`, `hooks/` — asserts 0 `bg-[#...]` outside `#5865f2`; Discord isolation (≤1 file, must be `AdminWebhooks`); doc exemption | `forge/web/test/design-system.test.tsx:122-165` |
| **Space Grotesk/IBM Plex Sans/JetBrains Mono via `next/font`** | `fonts.ts` imports + variable exports + weights; `layout.tsx` applies `sans.variable display.variable mono.variable`; `globals.css` `var(--font-*)` usage; `tailwind.config.ts` `fontFamily` prefers vars; doc scale | `forge/web/test/design-system.test.tsx:167-220` |
| **generation-fenced-dot renders two-dot badge correctly** | 2 dots stacked `flex-col`, top=desired/bottom=actual, `aria-label`/`title` with `g{n}/f{n}`, default pending, `size` prop, fenced ring vs `opacity-60`, `isFenced` override, `showLabel` `◉`, full `stateToDotClass` tone table, `StateLanesBadge` horizontal pill + `g→f fenced`, null `— / —`, `ServerStateLaneBadge`/`NodeStateLaneBadge` aliases, plus `states-empty`/`states-loading` exports & `role=status` & theme-aware EmptyList | `forge/web/test/design-system.test.tsx:222-549`, `components/shared/generation-fenced-dot.tsx:31-147` |

All 41 design-system tests **pass** in isolation (`vitest run test/design-system.test.tsx`).

---

## 3. Verification Commands (as specified)

### `npm --workspace @forge/web run test 2>&1 | tail -n 30`
```
 FAIL  middleware.test.ts > middleware > protected paths with a session cookie > forwards the cookie header to the validation request
AssertionError: expected '__Host-forge_session=abc' to be '__Host-forge_session=abc; other=1'
   → middleware.test.ts:115:35
 Test Files  1 failed | 20 passed (21)
      Tests  1 failed | 270 passed (271)
   Duration  8.71s
```
- **1 pre-existing failure** in `middleware.test.ts:115` (cookie forwarding — unrelated to design system, existed before this task; not introduced by `design-system.test.tsx`).
- **270 passing** includes all 41 new design-system tests + 21 existing `app-ux-18` suite + others. No regression from new file.

Snippet of design-system isolate (`npm --workspace @forge/web run test -- test/design-system.test.tsx 2>&1 | tail -n 20`):
```
✓ test/design-system.test.tsx (41 tests) 170ms
 Tests  41 passed (41)
```

### `npx tsc --noEmit --project forge/web/tsconfig.json 2>&1 | head -n 20`
```
(exit 0 — no output)
```
Typecheck clean — new `test/design-system.test.tsx` type-correct against `forge/web/tsconfig.json:1` (`paths: @/*`, `vitest` globals, `next/font/google` correctly excluded via `skipLibCheck`).

---

## 4. Notes / Residual Risks

- The single `bg-[#5865f2]` in `components/admin/AdminWebhooks.tsx:251` is **intentional** (Discord brand color, not part of the semantic palette); `DESIGN_TOKENS.md:128` explicitly exempts it. Lint rule should allowlist `#5865f2`.
- `generation-fenced-dot.tsx:19` still contains `bg-violet-500`/`bg-sky-500` hardcoded Tailwind semantic colors for `transferring`/`pending` states — these are palette extensions, not canvas/brand hardcodes, and are consistent with `DESIGN_TOKENS.md` §6 status coverage (`blue` tone). Future pass could map to `var(--info)` if the palette adds it.
- Pre-existing `middleware.test.ts:115` cookie failure is out of scope; tracks to session cookie `other=1` forwarding logic, not design tokens.

---

## 5. Files Changed

- **Created:** `forge/web/test/design-system.test.tsx` (549 lines, 41 tests, 8 suites) — no production code modified; `lib/design-tokens.ts`, `app/globals.css:5`, `tailwind.config.ts`, `components/shared/*` verified unchanged and compliant.
