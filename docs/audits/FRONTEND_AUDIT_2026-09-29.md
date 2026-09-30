# Frontend health audit — `forge/web`

**Date:** 2026-09-29 · **Branch:** `mvp-4` (vs `mvp-3`) · **Scope:** `forge/web` only

## Verdict

The frontend is **structurally sound but not green**. Architectural discipline is
genuinely good — one HTTP primitive, no second component library, token rules
respected, locales complete, and the monitoring surface is honest about what its
data actually means. Three things are broken right now: one user-visible
rendering bug that prints `NaN` in the admin UI, a type error that makes
`make lint` fail, and two stale tests. Nothing found is architectural; all six
findings are local fixes.

**Counts:** 1 high · 3 medium · 2 low. Tests 572/574 pass. Typecheck fails (1).
ESLint 9 warnings, 0 errors.

---

## Status — 2026-09-29, after remediation

Five of six findings are fixed. **F4 is not, and cannot be from this
environment.**

| # | Severity | Status |
| --- | --- | --- |
| F1 | HIGH | **Fixed.** `ResourceGauge` now takes nullable `value`/`limit`, renders `NotReported` with a reason instead of a zero, and draws no bar when there is no ratio. Locked by 5 new cases in `forge/web/test/app-ux-18.test.tsx`. |
| F2 | MEDIUM | **Fixed by another session** in `b7df85e`, not by this work. The dead `Symbol.for("react.fragment")` comparison is gone; `primitives.tsx:54` now guards on `typeof child.type !== "symbol"`. |
| F3 | MEDIUM | **Fixed.** Both stale assertions updated to match the shipped source. |
| F4 | MEDIUM | **Open — blocked by the sandbox, not by the code.** See the finding. |
| F5 | LOW | **Fixed.** 4 unused symbols removed; 5 `exhaustive-deps` warnings resolved structurally. ESLint now emits no output at all. |
| F6 | LOW | **Fixed, and re-characterised — the original finding was wrong about the cause.** See the finding. |

**Gates after remediation**, exit codes captured directly rather than read off
condensed output:

| Gate | Before | After |
| --- | --- | --- |
| `npx tsc --noEmit` | exit 1 | **exit 0**, 0 `error TS` |
| `npx eslint .` | exit 0, 9 warnings | **exit 0, no output** |
| `npx vitest run` | exit 1, 572/574 | **exit 0, 579/579, 33/33 files** |

`next build` is still not verifiable here for the F4 reason below.

### F6 was mis-diagnosed

The audit filed F6 as zero-defaults hiding unknown scores. Checking the
backend rather than inferring from the frontend shows that is not what was
happening. `scheduler.PredictiveScore`
(`forge/api/internal/services/scheduler/predictive.go:51`) declares every
scoring field as a plain `float64` with **no `omitempty`**, and the handler
returns the struct directly (`handlers_scheduler.go:19`). Go therefore always
emits all of them: there is no absent-field state on this endpoint, so the
`?? 0` guards were unreachable, not dishonest.

Worse, the page's local `PredictiveScore` declared six fields the API never
sends — `score`, `cpuLoad`, `memoryUsage`, `diskUsage`, `networkLoad`,
`activeServers` — of which four were never read at all. They existed only to
make the dead fallbacks look load-bearing.

The fix is therefore deletion, not a `NotReported` branch: adding one would
have asserted an unreachable state and implied the endpoint can withhold
scores when it cannot. Uncertainty on this endpoint is already carried
honestly by `confidence`, which the UI displays. Fixing this the way the
original finding proposed would have made the code *less* accurate.

### New finding — F7 · LOW · `lib/api/scheduler.ts` is an orphan module

Found while fixing F6, not in the original audit. `forge/web/lib/api/scheduler.ts`
already defines `PredictiveScore` and `listPredictiveScores()` /
`getPredictiveScore()`, but it is **not re-exported from `lib/api.ts`**, so
`@/lib/api` cannot reach it. `app/admin/scheduler/page.tsx` consequently
hand-rolled a duplicate type and calls `fetchJSON` directly against the same
two URLs — which is how the phantom fields in F6 drifted in unnoticed.

Not fixed here, deliberately. Consolidating means either a deep import or
adding barrel exports for generic names (`Constraint`, `ResourceMetric`,
`BackendInfo`) that risk colliding with existing exports — a decision worth
making on its own rather than inside an unrelated fix. The duplicate type is
now at least accurate.

---

## Findings

### F1 — HIGH · App detail "Resource Usage" shows invented zeros against a `NaN` limit

**Where:** [app/admin/apps/[id]/page.tsx:181](../../forge/web/app/admin/apps/%5Bid%5D/page.tsx#L181)-186,
[lib/api/apps.ts:385](../../forge/web/lib/api/apps.ts#L385)-389, 410-414

Three defects compound into one permanently-fake panel:

1. **Usage is never populated.** `mapApplication` hard-codes
   `cpuUsage: undefined`, `memoryUsage: undefined`, `diskUsage: undefined`
   (`lib/api/apps.ts:385,387,389`). There is no app stats/metrics endpoint in
   `lib/api/apps.ts` at all, and the page's only data source is
   `fetchApp(id)` → `mapApplicationDetail`, which spreads `mapApplication`.
   So these fields are `undefined` on every render, for every app, always.
2. **Unknown is coerced to zero.** The page does `app.cpuUsage ?? 0` (and the
   same for memory and disk), turning "never reported" into a confident `0.0`.
3. **The limit is `NaN`.** `mapApplicationDetail` always sets
   `resourceLimits` to an object of **strings**, using `""` when nothing is
   configured (`lib/api/apps.ts:410-414`). So the page's guard
   `typeof app.resourceLimits?.cpu === "string"` is **always true**, and the
   code takes `parseFloat("")` → `NaN`. The `?? 1` / `?? 1024` / `?? 10240`
   fallbacks on those same lines are unreachable dead code.

**Reproduced by execution** (temporary vitest harness, since removed) against
the real `mapApplicationDetail` and the real `ResourceGauge`:

```
COMPUTED:      { cpuUsage: 0, cpuLimit: NaN, memUsage: 0, memLimit: NaN }
RENDERED CPU:  0.0 / NaN cores
RENDERED MEM:  0.0 / NaN MiB
```

**Impact.** Every app's Resource Usage card renders `0.0 / NaN`. The bar is
also *green*: `ResourceGauge` computes `pct = limit > 0 ? … : 0`, and `NaN > 0`
is false, so `pct = 0` selects `bg-emerald-500`
([components/admin/AdminAppsShared.tsx:17-18](../../forge/web/components/admin/AdminAppsShared.tsx#L17)).
An operator sees a healthy-looking zero-load reading for a workload the panel
has no telemetry for whatsoever.

This is squarely the AGENTS.md pitfall — *"Unknown is not zero, not-reported is
not zero … this holds in the API, in Beacon and in the UI."* It is also
inconsistent with the rest of the admin surface, which already does this
correctly (see *Healthy*, below).

**Fix.** The codebase already has the right primitive:
`Reading` / `NotReported` in
[components/admin/telemetry-ui.tsx:92](../../forge/web/components/admin/telemetry-ui.tsx#L92)
renders a "Not reported" state for `null | undefined | ""`.

- Give `ResourceGauge` an unknown state (`value`/`limit` accepting
  `number | null | undefined`) that renders `Reading` instead of a bar.
- Pass `app.cpuUsage` through unchanged — drop the `?? 0`.
- Delete the always-true `typeof … === "string"` ternary; parse
  `resourceLimits.*` only when non-empty, and treat an unset limit as unknown
  rather than inventing `1` / `1024` / `10240`.
- Longer term the panel needs a real stats source, or it should not claim to be
  a usage panel.

---

### F2 — MEDIUM · Type error fails `tsc` and therefore `make lint`

**Where:** [components/ui/primitives.tsx:48](../../forge/web/components/ui/primitives.tsx#L48)

```
components/ui/primitives.tsx(48,50): error TS2367: This comparison appears to be
unintentional because the types 'string | JSXElementConstructor<any>' and
'symbol' have no overlap.
```

`npx tsc --noEmit` exits **1**. `scripts/dev/lint.sh:22` runs
`npm run typecheck`, so `make lint` is red. Introduced by the most recent
commit, `bce7085`.

**No behavioural bug.** The line-48 clause
`(children as React.ReactElement).type !== Symbol.for("react.fragment")` is a
fragment guard that TypeScript considers always-true, and the trailing
`(children.type as unknown) !== undefined` is always true as well. The guard
that actually works at runtime is the duplicate three lines below —
`typeof child.type !== "symbol"` (line 51). So fragments are already handled;
line 48 is a dead, type-invalid restatement.

**Fix.** Delete both clauses from the line-48 condition and rely on the line-51
symbol check, or compare against the imported `Fragment` via an `unknown` cast.
Either removes the error without changing behaviour.

---

### F3 — MEDIUM · Two stale tests keep the suite red (no product change needed)

`npx vitest run` → **2 failed / 572 passed (574)**. Both are assertions that
were not updated when the code deliberately changed.

**a) WebSocket URL contract inverted.**
[lib/api.contract.test.ts:98](../../forge/web/lib/api.contract.test.ts#L98) —
*"constructs relative API websocket URLs"* — asserts
`/api/v1/servers/srv/ws/stats`, but production now returns
`ws://localhost:3000/api/v1/servers/srv/ws/stats`.

Production is correct and documents why:
[lib/api/http.ts:636-648](../../forge/web/lib/api/http.ts#L636) explains that a
relative base makes `new WebSocket` throw, so the URL is resolved against
`window.location.origin` with the scheme swapped, and only SSR (no `window`)
returns the relative path. Timeline confirms drift: `lib/api/http.ts` last
changed **2026-09-27**, the test last changed **2026-07-26**.

→ Update the assertion to the absolute URL and rename the test; it asserts the
opposite of the current intended contract.

**b) Fenced-ring token softened.**
[test/design-system.test.tsx:441](../../forge/web/test/design-system.test.tsx#L441)
expects `ring-[var(--danger)]`; the component uses
`ring-[color-mix(in_srgb,var(--danger)_70%,transparent)]`
([generation-fenced-dot.tsx:79](../../forge/web/components/shared/generation-fenced-dot.tsx#L79)).
Both files changed in the same commit (`7389900`) — the value was softened and
the assertion was not updated with it. The sibling `ring-2` assertion on line
440 still passes.

→ Assert on the `color-mix(...)` value, or loosen to `ring-[` + `--danger`.

Both failures live in commits titled *"chore: commit working tree …"*, which is
the likely mechanism: bulk commits of in-progress work without a test run.

---

### F4 — MEDIUM · Production build requires egress to Google Fonts

**Where:** [app/fonts.ts:15](../../forge/web/app/fonts.ts#L15)

`next build` fails when `fonts.googleapis.com` is unreachable:

```
Failed to compile.
app/fonts.ts  `next/font` error: Failed to fetch `Manrope` from Google Fonts.
app/fonts.ts  `next/font` error: Failed to fetch `Space Grotesk` from Google Fonts.
app/fonts.ts  `next/font` error: Failed to fetch `JetBrains Mono` from Google Fonts.
> Build failed because of webpack errors
```

(Reproduced here because this sandbox denies that host; the same applies to any
air-gapped or egress-restricted builder.) There are **no** `woff2`/`woff`/`ttf`
files anywhere in the tree — `public/` holds only `favicon.svg` and `og.svg`.

**Nuance, stated precisely.** AGENTS.md says the fonts are *self-hosted*. That
is true at **runtime** — `next/font/google` downloads at build time and the app
serves the files itself, so no page-load request reaches Google. It is not true
at **build** time. `app/fonts.ts:5-9` already documents this gap and carries a
`TODO` to vendor the woff2 files and migrate to `next/font/local`, so this is a
known shortcut, not a hidden one.

**Why it matters here:** this repo ships `forge/install/install.sh` for
production hosts, so a build that needs third-party egress is a real
constraint on offline installs and on locked-down CI.

**Fix.** Execute the existing TODO: vendor the three families and switch to
`next/font/local`. Note `app/fonts.ts` also loads **Space Grotesk**, a third
family beyond the two AGENTS.md names — deliberate and documented in-file
(display headings only), but AGENTS.md should say so too.

**Status: not fixed. This environment cannot do it.** Fixing F4 requires
downloading the woff2 files, and the only source is the host this sandbox
denies:

```
$ curl https://fonts.googleapis.com/css2?family=Manrope:wght@400
curl: (56) CONNECT tunnel failed, response 403
deny network-outbound fonts.googleapis.com:443 (user denied)
```

The egress allowlist here is fixed and cannot be widened per-command. The two
ways to "finish" F4 without that access would both be worse than leaving it
open: pointing `next/font/local` at font files that do not exist would break
the build outright — precisely what the in-file TODO warns about — and
substituting different fonts would silently change the product's typography.

So F4 needs either network access to those hosts or the woff2 files supplied
into `forge/web/app/fonts/`. Whether to open that egress is the repo owner's
call, not something to route around. Everything else about the finding stands
as written, including that the *runtime* really is self-hosted.

---

### F5 — LOW · 9 ESLint warnings

`npx eslint .` → 9 warnings, **0 errors** (so `npm run lint` exits 0 and does
not block).

- Unused (4): `unwrapList` in `app/admin/load-balancer/page.tsx:6` and
  `app/admin/preview-deployments/page.tsx:9`; `useQuery` in
  `app/console/layout.tsx:6`; `ForgeRequestOptions` in
  `lib/api/retry-client.ts:3`. Safe deletions.
- `react-hooks/exhaustive-deps` missing `router` (5): `app/page.tsx:56,61`,
  `app/organizations/page.tsx:35`, `app/organizations/[slug]/page.tsx:43`,
  `components/server/server-console-layout.tsx:85`. All are redirect effects;
  the App Router `router` object is stable, so these are noise rather than
  bugs — but they are the kind of noise that hides a real dep warning later.

---

### F6 — LOW (lower confidence) · Scheduler score zero-defaults

**Where:** [app/admin/scheduler/page.tsx:339](../../forge/web/app/admin/scheduler/page.tsx#L339)-340

```ts
const scoreVal = node.totalScore ?? node.score ?? 0;
const cpuVal = node.predictedLoad ?? node.cpuLoad ?? 0;
```

Same class as F1 — a missing score would render as a confident `0.0`. I did
**not** confirm that `/scheduler/scores` ever omits these fields, so this may
be harmless in practice; flagging it as the remaining instance of the pattern
rather than a proven defect. The page otherwise handles loading, error and
empty states properly.

---

## Healthy — verified, not assumed

- **One HTTP primitive.** Zero bare `fetch(` calls in `app/`, `components/`,
  `hooks/`, `stores/`. All traffic goes through `lib/api/*`.
- **No second primitive set.** No Radix, no shadcn in `package.json`; UI
  primitives are the hand-rolled `components/ui/`.
- **Design tokens.** The project's own documented check
  (`DESIGN_TOKENS.md:154`) returns exactly one hit — the
  `AdminWebhooks.tsx` Discord brand colour that the doc itself exempts. Raw hex
  elsewhere (`AdminCatalog.tsx` database logo colours, `tags-manager.tsx`
  user-facing colour palette) is third-party/user-chosen identity colour in
  inline styles, not theme colour, so it is outside the rule's intent.
  *Trivial doc drift:* `DESIGN_TOKENS.md:128` cites `AdminWebhooks.tsx:251`; the
  occurrences are now at lines 261 and 266.
- **Locales complete.** `npm run sync:locales:check` → 8 locales on disk match
  `supportedLocales`, **0 missing keys**.
- **No status defaults to healthy.** A sweep for
  `?? "healthy" | "online" | "running" | "ok" | "up" | "active" | "success"`
  returns nothing — the work in `1b02638` / `bb7ee7c` held.
- **Monitoring is honest.** `AdminMonitoring.tsx` labels the series
  `CPU/MEMORY/STORAGE ALLOCATED`, states that rows derive from capacity
  snapshots and are "not measured host load", keeps charts *empty rather than
  zero* when no rows exist, and deliberately omits a network series because the
  collector writes zero bytes (`AdminMonitoring.tsx:68-71`). This matches the
  actual `node_metrics` semantics. F1 is the one place the admin UI departs from
  this standard.

---

## Method

Every command below was run from `forge/web` on `mvp-4` with a clean tree.

```bash
npx tsc --noEmit          # exit 1 — 1 error (F2)
npx eslint .              # exit 0 — 9 warnings (F5)
npx vitest run            # exit 1 — 2 failed / 572 passed (F3)
npx next build            # exit 1 — blocked at Google Fonts fetch (F4)
npm run sync:locales:check # exit 0 — 0 missing keys
```

**⚠ Tooling caveat worth acting on.** Under RTK's condensed output, `tsc`,
`vitest` and `next build` all reported **`[exited with code 0]`** while their
own text showed failures — the real exit codes are `1`, `1`, `1`. Every result
in this report comes from a re-run via `rtk proxy`. Anyone reading the condensed
summaries alone would conclude the frontend is green when it is not; worth
checking before these commands are trusted in CI or a pre-commit gate.

**Not covered:** Playwright e2e (`npx playwright test`), runtime verification
against a live API, `packages/*` internals, and the `forge/web` half of the
1365-file `mvp-3…mvp-4` diff as a review (this is a current-state audit, not a
diff review).

**Unrelated observation:** the working tree was clean at session start but now
shows modifications to `forge/api/internal/placement/replica.go`,
`forge/api/internal/scheduler/scheduler_nomad.go`,
`forge/api/internal/services/crossnode/ingress_sync.go` and a new
`docs/audits/BACKEND_BUILD_BREAKAGE_2026-09-29.md`. These are backend files and
not mine — likely a concurrent session. I did not touch them.

## Suggested order

1. **F2** — one-line deletion, unblocks `make lint`.
2. **F3** — two assertion updates, turns the suite green.
3. **F1** — the only user-visible bug; needs a small `ResourceGauge` API change.
4. **F4** — vendor the woff2 files (the TODO already specifies it).
5. **F5 / F6** — cleanup.

Steps 1–3 are small and independent, and together take the frontend from
"red on three gates" to green.
