# Subagent 07 — Web Slices 18, 06 Verify (App UX, Host Infra, Design Tokens)

**Date:** 2026-08-24
**Agent:** 110-04-07 / 10
**Focus:** `forge/web` slices 18 (App UX) + 06 (Host infra) + design tokens consolidation
**Workspace:** `forge/web` (`@forge/web@0.1.0`)

---

## 1) `npx tsc --noEmit --project forge/web/tsconfig.json`

**Command:**
```bash
npx tsc --noEmit --project forge/web/tsconfig.json 2>&1 | head -n 100
echo EXIT:$?
```

**Result — BEFORE fix:**
```
(no output)
EXIT:0
```

**Result — AFTER fix:**
```
(no output)
EXIT:0
```

**Verdict:** ✅ PASS — zero type errors. No missing exports or type mismatches across `lib/api/status.ts`, `lib/app-type-icons.ts`, `hooks/useDeploymentSteps.ts`, `lib/api/compose.ts` and all consumers (`components/app/deployment-progress.tsx:8`, `components/charts/DeploymentTimeline.tsx:7`, `app/admin/apps/page.tsx:15`, `app/admin/apps/[id]/page.tsx:23`).

---

## 2) `npm --workspace @forge/web run lint`

**Command:**
```bash
npm --workspace @forge/web run lint 2>&1 | head -n 100
npm --workspace @forge/web run lint 2>&1 | tail -n 100
```

**Result — BEFORE fix (95 problems: 3 errors, 92 warnings):**
```
✖ 95 problems (3 errors, 92 warnings)

Errors:
  components/admin/AdminHealth.tsx:243:9   error  Do not use an `<a>` element to navigate to `/admin/monitoring/`. Use `<Link />` from `next/link` instead.  @next/next/no-html-link-for-pages
  components/admin/AdminOverview.tsx:239:13 error  Do not use an `<a>` element to navigate to `/admin/monitoring/`. Use `<Link />` from `next/link` instead.  @next/next/no-html-link-for-pages
  test/app-ux-18.test.tsx:201:42   error    Unexpected any. Specify a different type        @typescript-eslint/no-explicit-any

Warnings sample (92 total, excerpt):
  app/admin/apps/[id]/compose/page.tsx:15:72  warning  'AdminLoadingState' is defined but never used  @typescript-eslint/no-unused-vars
  app/admin/host/page.tsx:3:20  warning  'useCallback' is defined but never used  @typescript-eslint/no-unused-vars
  components/admin/AdminHealth.tsx:3:10  warning  'useMemo' is defined but never used  @typescript-eslint/no-unused-vars
  components/app/deployment-progress.tsx:6:15  warning  'DeploymentStep' is defined but never used  @typescript-eslint/no-unused-vars
  test/app-ux-18.test.tsx:2:10  warning  'render' is defined but never used  @typescript-eslint/no-unused-vars
  ... (full list 92 warnings across ~30 files, see raw lint output)
```

**Result — AFTER fix (92 problems: 0 errors, 92 warnings):**
```
✖ 92 problems (0 errors, 92 warnings)
LINT_EXIT:0  (previously exit 1 due to 3 errors)
```

Full tail after fix:
```
/components/shared/states-connectivity.tsx
  4:10  warning  'cn' is defined but never used  @typescript-eslint/no-unused-vars
/e2e/nodes.spec.ts
  81:9  warning  'healthCalls' is assigned a value but never used  @typescript-eslint/no-unused-vars
/lib/api/console-backups.ts
  1:54  warning  'putJSON' is defined but never used  @typescript-eslint/no-unused-vars
/scripts/sync-locales.mjs
  24:10  warning  'countKeys' is defined but never used  @typescript-eslint/no-unused-vars
/test/app-ux-18.test.tsx
   2:10   warning  'render' is defined but never used              @typescript-eslint/no-unused-vars
   2:18   warning  'screen' is defined but never used              @typescript-eslint/no-unused-vars
   3:8    warning  'userEvent' is defined but never used           @typescript-eslint/no-unused-vars
  12:106  warning  'useDeploymentSteps' is defined but never used  @typescript-eslint/no-unused-vars

✖ 92 problems (0 errors, 92 warnings)
```

**Verdict:** ✅ PASS (errors fixed, warnings remain but non-blocking). ESLint `next/core-web-vitals` treats warnings as non-failing; exit 0 achieved after fixes. Remaining 92 warnings are **pre-existing unused-import/variable warnings** across admin pages (`app/admin/*`), `components/admin/*`, `e2e/*`, `scripts/*` — mostly unused `AdminLoadingState`, `AdminErrorState`, `isAbortError`, icon imports. They do **not** block build and are **outside slice 18/06 core files**.

**Slice-relevant files are clean:**
- `forge/web/lib/api/status.ts:46` — no lint issues
- `forge/web/lib/app-type-icons.ts:16` — no lint issues
- `forge/web/hooks/useDeploymentSteps.ts:39` — no lint issues
- `forge/web/lib/api/compose.ts:93` — no lint issues
- `forge/web/components/app/deployment-progress.tsx:8` — 1 warning (`DeploymentStep` unused) but no error; hook now correctly imported via `useDeploymentSteps`
- `forge/web/components/charts/DeploymentTimeline.tsx:7` — clean
- `forge/web/app/admin/apps/page.tsx:15` — 1 warning unused import, no error
- `forge/web/app/admin/apps/[id]/page.tsx:43` — clean (uses `router.replace` + `searchParams.get("tab")`)

---

## 3) `npm --workspace @forge/web run test 2>&1 | tail -n 40`

**Command:**
```bash
npm --workspace @forge/web run test 2>&1 | tail -n 40
```

**Result — BEFORE and AFTER (identical, fix does not affect middleware test):**

Targeted run:
```
✓ test/app-ux-18.test.tsx (21 tests) 23ms
 Test Files  1 passed (1)
      Tests  21 passed (21)
```

Full run (`vitest run` 20 files):
```
 ✓ test/app-ux-18.test.tsx (21 tests) 22ms
 ✓ lib/api.nodes.contract.test.ts (4 tests) 13ms
 ✓ lib/api/mounts.test.ts (2 tests) 12ms
 ✓ lib/api.response.test.ts (3 tests) 14ms
 ...
 FAIL  middleware.test.ts > middleware > protected paths with a session cookie > forwards the cookie header to the validation request
AssertionError: expected '__Host-forge_session=abc' to be '__Host-forge_session=abc; other=1' // Object.is equality
  at middleware.test.ts:115:35
   113|       const [url, init] = (global.fetch as ReturnType<typeof vi.fn>).m…
   114|       expect(url.toString()).toBe("http://127.0.0.1:8080/api/v1/auth/m…
   115|       expect(init.headers.cookie).toBe(`${SESSION_COOKIE_VALUES[0]}=ab…
...

 Test Files  1 failed | 19 passed (20)
      Tests  1 failed | 229 passed (230)
   Duration  6.91s
```

**Verdict:** ✅ Slice 18/06 PASS, ⚠️ 1 pre-existing failure unrelated to this slice.

- **Slice 18/06 contract (`test/app-ux-18.test.tsx:1`): 21/21 PASS** — covers:
  - `statusTone` centralization (`lib/api/status.ts:46` — `statusTone(status, kind)` single source)
  - `APP_TYPE_ICONS` shadow fix (`lib/app-type-icons.ts:16`)
  - `useDeploymentSteps` dedup 5s polling (`hooks/useDeploymentSteps.ts:39`, `DEPLOYMENT_STEPS_POLL_INTERVAL_MS = 5000`)
  - `restartComposeStack` export (`lib/api/compose.ts:93`)
  - Router-driven tab routing (`app/admin/apps/[id]/page.tsx:43`, `router.replace` with `?tab=`)
  - Triple env editors scope check
- **Unrelated failure:** `middleware.test.ts:115` expects cookie forwarding `"__Host-forge_session=abc; other=1"` but receives `"__Host-forge_session=abc"` — belongs to auth/middleware slice, not 18/06. Does not block slice verification. Recommend follow-up in middleware (cookie header merging in `forge/web/middleware.ts`).

---

## 4) File Existence & Export Checks (Step 4)

| Check | Path | Result | Evidence |
|-------|------|--------|----------|
| **Single `statusTone` exists** | `forge/web/lib/api/status.ts:46` | ✅ PASS | `export function statusTone(status: string, kind: "app" \| "deployment" = "app"): StatusTone` at line 46. Aliases `deploymentStatusTone` (line 56) and `appStatusTone` (line 61) wrap it. Consolidates prior duplicates in `lib/api/apps.ts` and inline maps. |
| **`forge/web/lib/app-type-icons.ts` exists** | `forge/web/lib/app-type-icons.ts:1` | ✅ PASS | `"use client"` + `export const APP_TYPE_ICONS: Record<AppType, LucideIcon>` (line 16) + `export const typeIcons = APP_TYPE_ICONS` alias (line 25) + `iconForAppType` helper (line 27). Imports converge: `app/admin/apps/page.tsx:15` now imports `APP_TYPE_ICONS` (no local `const typeIcons`). |
| **`forge/web/hooks/useDeploymentSteps.ts` exists** | `forge/web/hooks/useDeploymentSteps.ts:1` | ✅ PASS | Exports `useDeploymentSteps` (line 39), `isDeploymentStepsTerminal` (line 34), `DEPLOYMENT_STEPS_POLL_INTERVAL_MS = 5000` (line 20), `DEPLOYMENT_STEPS_MAX_DURATION_MS = 10*60*1000` (line 21). Consumers `components/app/deployment-progress.tsx:8` and `components/charts/DeploymentTimeline.tsx:7` both import hook, share `queryKey ["deployment-steps", id]` (line 49), no longer define `POLL_INTERVAL_MS = 2000` or inline `refetchInterval: (query)=>`. |
| **`forge/web/lib/api/compose.ts` `restartComposeStack` export exists** | `forge/web/lib/api/compose.ts:93` | ✅ PASS | `export function restartComposeStack(id: string) { return postJSON(\`/compose/${encodeURIComponent(id)}/restart\`); }` alongside `deployComposeStack` (line 89), `stopComposeStack` (line 97), `startComposeStack` (line 101). Verified imported in `test/app-ux-18.test.tsx:13`. |

Additional host-infra / design-token checks observed while verifying 06:
- `components/admin/AdminHealth.tsx:70` — health page uses shared `fetchHealthStatus`/`fetchNodes` etc., metric tiles and Section layout tokens (`--line`, `--surface`, `--text-subtle`) — design token usage consistent.
- `components/admin/AdminOverview.tsx:65` — command-center overview uses same token set and `Node`/`Server`/`Health` queries; no duplicated `typeIcons`/`statusTone`.
- Both admin components now use `next/link` for internal navigation (post-fix).

---

## 5) Fixes Applied (Step 5)

### 5.1 `forge/web/components/admin/AdminHealth.tsx:1` — `<Link />` migration
**Issue:** `@next/next/no-html-link-for-pages` error at `AdminHealth.tsx:243:9` (and related dynamic link at `:187:39`)
```tsx
// before
import { Activity, ... } from "lucide-react";
import { fetchHealthStatus, ... } from "@/lib/api";
...
<a href={`/admin/nodes/${n.id}`} ...>View</a>
<a href="/admin/monitoring" ...>Observe metrics →</a>
<a href="/admin/overview" ...>Back to overview →</a>
```

**Fix:**
```diff
+import Link from "next/link";
 import { fetchHealthStatus, ... } from "@/lib/api";
 
-<a href={`/admin/nodes/${n.id}`} ...>View</a>
+<Link href={`/admin/nodes/${n.id}`} ...>View</Link>
 
-<a href="/admin/monitoring" ...>Observe metrics →</a>
-<a href="/admin/overview" ...>Back to overview →</a>
+<Link href="/admin/monitoring" ...>Observe metrics →</Link>
+<Link href="/admin/overview" ...>Back to overview →</Link>
```

**File:** `forge/web/components/admin/AdminHealth.tsx:6` (import), `187`, `243-245` (usages)

---

### 5.2 `forge/web/components/admin/AdminOverview.tsx:1` — `<Link />` migration (6 links)
**Issue:** `@next/next/no-html-link-for-pages` error at `AdminOverview.tsx:239:13`, plus 5 other internal `<a>` not yet flagged but violating same rule.

**Fix:**
```diff
+import Link from "next/link";
 import { AlertTriangle, ... } from "lucide-react";
 
-<a href="/admin/health" ...>View health ...</a>
+<Link href="/admin/health" ...>View health ...</Link>
 
-<a href={`/admin/nodes/${n.id}`} ...>View</a>
+<Link href={`/admin/nodes/${n.id}`} ...>View</Link>
 
-<a href="/admin/health" ...>View all health issues →</a>
+<Link href="/admin/health" ...>View all health issues →</Link>
 
-<a href="/admin/activity" ...>View activity log →</a>
+<Link href="/admin/activity" ...>View activity log →</Link>
 
-<a href="/admin/nodes" ...>View nodes →</a>
-<a href="/admin/monitoring" ...>Observe metrics →</a>
+<Link href="/admin/nodes" ...>View nodes →</Link>
+<Link href="/admin/monitoring" ...>Observe metrics →</Link>
```

**File:** `forge/web/components/admin/AdminOverview.tsx:4` (import), `91`, `183`, `196`, `222`, `238-240`

---

### 5.3 `forge/web/test/app-ux-18.test.tsx:201` — `no-explicit-any` fix
**Issue:** `@typescript-eslint/no-explicit-any` error at `test/app-ux-18.test.tsx:201:42`
```ts
const router = { replace: replace as any };
```

**Fix:**
```diff
-    const router = { replace: replace as any };
+    const router = { replace: replace as unknown as (url: string, opts?: { scroll: boolean }) => void };
     router.replace(expected, { scroll: false });
```

**File:** `forge/web/test/app-ux-18.test.tsx:201`

---

### 5.4 Re-verification
```bash
npx tsc --noEmit --project forge/web/tsconfig.json   # EXIT:0
npm --workspace @forge/web run lint                   # ✖ 92 problems (0 errors, 92 warnings) — EXIT:0
npm --workspace @forge/web run test -- test/app-ux-18.test.tsx  # 21/21 pass
```

No new `tsc` errors introduced. Lint now passes (0 errors). All 21 slice contract tests still pass.

---

## 6) Remaining Risks / Follow-ups

- **92 warnings remain** — all `no-unused-vars` warnings. They are not errors and do not block `next build`, but indicate dead imports (`AdminLoadingState`, `AdminErrorState`, `isAbortError`, icons, etc.) across ~30 files. Recommend `eslint --fix` pass or `// eslint-disable-next-line @typescript-eslint/no-unused-vars` with `_` prefix for intentionally unused destructured values (e.g., `app-ux-18.test.tsx:2 render, screen, userEvent` — these are imported for future assertions but currently only used for source-read checks).
- **Middleware test failure** (`middleware.test.ts:115`) — unrelated to slice 18/06. Cookie forwarding logic in `forge/web/middleware.ts` drops secondary cookies (`other=1`). Needs fix: merge `request.headers.get("cookie")` instead of only `__Host-forge_session`.
- **`components/app/deployment-progress.tsx:59`** still has `react-hooks/exhaustive-deps` warning about `steps` dependency in `useEffect` at line 76 — acceptable because `steps` is query data derived from `useDeploymentSteps`. Could be suppressed or memoized, but not an error.
- Design tokens: no missing token exports detected; `tailwind.config.ts` and `app/globals.css` not re-checked in this pass — host infra tokens appear consistent via `var(--line)`/`var(--surface)` usage.

---

## 7) Summary

| Task | Status | Notes |
|------|--------|-------|
| `tsc --noEmit` | ✅ PASS | 0 errors |
| `eslint` (core) | ✅ PASS (after fix) | 3 errors → 0 errors; 92 warnings remain (non-blocking) |
| `vitest run` | ⚠️ PASS with 1 unrelated fail | `test/app-ux-18.test.tsx` 21/21 pass; `middleware.test.ts` 1 fail pre-existing |
| `lib/api/status.ts` single `statusTone` | ✅ EXISTS | `statusTone` at `:46`, aliases at `:56` `:61` |
| `lib/app-type-icons.ts` | ✅ EXISTS | `APP_TYPE_ICONS` at `:16` |
| `hooks/useDeploymentSteps.ts` | ✅ EXISTS | `useDeploymentSteps` at `:39`, 5s interval at `:20` |
| `lib/api/compose.ts` `restartComposeStack` | ✅ EXISTS | at `:93` |
| Fixes applied | ✅ 3 errors fixed | Links + `no-explicit-any` |

**Overall slice 18/06 verification: PASS** — web UX consolidation (status tones, app icons, deployment poller dedup, compose restart, router-driven tabs) is intact; toolchain is green except for unrelated middleware test and pre-existing unused-var warnings.
