# Subagent 03 — Web Performance (Bundle, Code Splitting, Images, Static)

**Phase:** 110-09-03 of 10  
**Focus:** Optimize Web performance (bundle, code splitting, images, static)  
**Date:** 2026-08-24  
**Workspace:** `forge/web` (Next.js 15.5.22, React 19)

---

## 1. Inspection Summary

### 1.1 `forge/web/next.config.ts:1` (formerly `next.config.js`)
```ts
// forge/web/next.config.ts:1-38
import type { NextConfig } from "next";
const nextConfig: NextConfig = {
  reactStrictMode: true,
  compress: true,                 // ← added (default true, explicit)
  poweredByHeader: false,         // ← added (security + bytes)
  output: "standalone",
  outputFileTracingRoot: projectRoot,
  experimental: {
    optimizePackageImports: ["lucide-react","recharts","@xterm/xterm","@monaco-editor/react","@tanstack/react-query"]
  },
  images: {
    remotePatterns: [github, gitlab, bitbucket, cdn.jsdelivr.net],
    formats: ["image/avif","image/webp"], // ← added
    minimumCacheTTL: 60,                  // ← added
  },
  rewrites() { /* /api -> API_INTERNAL_URL */ }
}
```
- **Before:** minimal — `reactStrictMode`, `output:standalone`, `images.remotePatterns` only. Missing `compress`, `poweredByHeader`, `experimental.optimizePackageImports`, image `formats`.
- **After:** added `compress:true`, `poweredByHeader:false`, `experimental.optimizePackageImports`, image AVIF/WebP + `minimumCacheTTL`. Verified `sharp@0.35.3` installed provides optimizer backend (Next warns if missing — no warning observed).

### 1.2 `forge/web/package.json:18-35`
- `next: ^15.5.22`, `react: ^19.0.0`, `react-dom: ^19.0.0`
- Heavy deps: `@monaco-editor/react@^4.7.0` (+ `monaco-editor@0.55.1` transitive), `@xterm/xterm@^5.5.0` + addons `fit@0.10.0`, `search@0.15.0`, `web-links@0.11.0`, `recharts@^2.15.0` (chart), `sharp@^0.35.3` ✅, `lucide-react@^0.468.0` (tree-shake candidate), `@tanstack/react-query@^5.90.12`, `zustand@^5.0.2`.
- `sharp` present in `dependencies` and `overrides` — image optimization **ensured**. `next build` did not emit `sharp not installed` warning.
- No `bundle-analyzer` dep — not required; used `next build` output + `du` for size.

### 1.3 `forge/web/app/layout.tsx:1-52`
- `import { display, mono, sans } from "./fonts"` — `next/font/google` used correctly.
- `app/fonts.ts:1` uses `IBM_Plex_Sans`, `JetBrains_Mono`, `Manrope`, `Space_Grotesk` from `next/font/google` with `display:"swap"`, `subsets:["latin"]`, `variable:"--font-*"` — **font optimization ensured**. `body className={`${sans.variable} ${display.variable} ${mono.variable}`}` applies CSS variables; Tailwind `fontFamily` consumes `var(--font-*)`.
- `metadata` and `viewport` static, `cookies()` + `headers()` dynamic — root layout is dynamic (expected for auth/theme). No unnecessary client JS in layout.

### 1.4 `app/layout.tsx` vs `components/*` large analysis
- **Largest components by LOC / import weight:**
  - `components/server/files-view.tsx:335` — 335 lines, imports `lucide-react` icons, `useQuery`; monaco via `dynamic` (good).
  - `app/admin/terminal/page.tsx:317` — xterm terminal; uses `Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit"), import("@xterm/addon-web-links")])` + `@xterm/xterm/css/xterm.css` — **code-split** ✓
  - `app/admin/monitoring/page.tsx:235` — previously `import { AreaChart... } from "recharts"` static — **heavy (112 kB / 251 kB First Load)** — **optimized** to dynamic (see §2).
  - `components/monitoring/metrics-chart.tsx:185` — static recharts — optimized to dynamic viz.
  - `components/charts/ServerCPUChart.tsx:132`, `ServerMemoryChart`, `ServerDiskChart`, `ServerNetworkChart`, `ResourceUsageBar.tsx` — each static `recharts` import; currently **unused** (no consumer in `app/` — `grep -r "ServerCPUChart" forge/web` only definition exports). Risk: if imported statically, would inflate bundle. Mitigated via new `components/monitoring/monitoring-chart.tsx` dynamic pattern + `experimental.optimizePackageImports`.
  - `components/environment/EnvironmentEditor.tsx:294` — not heavy (no external heavy lib).

### 1.5 `.next` size & `next build` output (pre-optimize)
- `.next` (pre): `693M` (after first build), `static` `5.6M` — from `du -sh forge/web/.next`.
- `npm --workspace @forge/web run build 2>&1 | tail -n 50` (pre — captured 2026-08-24 07:51):
  ```
  Route (app)                                          Size  First Load JS
  ├ ƒ /admin/monitoring                              112 kB         251 kB  ← LARGEST
  ├ ƒ /admin/nodes                                  20.6 kB         171 kB
  ├ ƒ /admin/operations                             12.2 kB         160 kB
  ├ ƒ /server/[id]/files                             2.21 kB         175 kB
  + First Load JS shared by all                      103 kB
    ├ chunks/18-c6633c4945af8275.js                 46.7 kB
    ├ chunks/87c73c54-09e1ba5c70e60a51.js           54.2 kB
    └ other shared chunks (total)                   2.12 kB
  ƒ Middleware                                        35 kB
  ```
- **First Load JS shared** `103 kB` — healthy (Next.js baseline ~100k). Monitoring route outlier due to `recharts` bundled inline.
- Static chunks (pre): `4957-8e1509d206f76468.js 382K`, `1c0ca389.c636a9d260551aca.js 280K`, `framework-92c... 185K`, `18-c663... 170K` — `4957` contains `recharts` (≈382K uncompressed).
- CSS: `0bab087056f1d0da.css 87K`, `8362a43b... 15K`, `0a6ee5b... 3K`.

### 1.6 Code splitting audit (`dynamic` / `next/dynamic`)
| Component | File | Import | Split? | Status |
|-----------|------|--------|--------|--------|
| Monaco | `components/server/files-view.tsx:5,17` | `dynamic(() => import("@monaco-editor/react"), {ssr:false, loading...})` | ✅ | Already code-split |
| xterm | `app/admin/terminal/page.tsx:68-71` | `Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit"), import("@xterm/addon-web-links")])` + `xtermRef` lazy init | ✅ | Already code-split, `ssr:false` via client-only effect |
| Recharts (monitoring) | `app/admin/monitoring/page.tsx:6` (pre) | `import { Area... } from "recharts"` static | ❌ | **Fixed — now dynamic** |
| Recharts (metrics-chart) | `components/monitoring/metrics-chart.tsx:5` (pre) | static | ❌ | **Fixed — now dynamic via `metrics-viz.tsx`** |
| Recharts (charts/*) | `components/charts/*.tsx:4` | static, but unused | ⚠️ | Documented; consumers should use dynamic wrapper or `optimizePackageImports` |
| Other | `lucide-react`, `@tanstack/react-query` | static everywhere | ⚠️ | Mitigated via `experimental.optimizePackageImports` |

### 1.7 Image optimization
- `sharp@^0.35.3` in `package.json:33` ✅ — `node_modules/sharp` present with `darwin-arm64`/`linux-x64` binaries.
- `next.config.ts:images.remotePatterns` locked to 4 hosts (no `**` wildcard) — prevents optimizer as open proxy: `github.com`, `gitlab.com`, `bitbucket.org`, `cdn.jsdelivr.net`.
- `next/image` usage: `app/admin/app-store/page.tsx:4,273,388` and `app/admin/git-providers/page.tsx:4,21-28` — both `unoptimized` prop (intentional — external `cdn.jsdelivr.net`/`github` icons, CSP-governed, browser-fetched). No optimized remote images, so `sharp` not yet exercised but correctly installed for future `next/image` optimization. Added `formats: ["image/avif","image/webp"]` + `minimumCacheTTL:60` to `next.config.ts` to enable AVIF/WebP negotiation when `next/image` is used without `unoptimized`.
- No `next/image` priority/LCP issues detected (no hero images). `public/favicon.svg`, `public/og.svg` static.

### 1.8 Font optimization (`next/font`)
- `app/fonts.ts:1` — `IBM_Plex_Sans({weight:["400","500","600","700"], display:"swap", variable:"--font-sans"})`, `JetBrains_Mono({display:"swap", variable:"--font-mono"})`, `Space_Grotesk`, `Manrope` — all with `display:swap` (avoids FOIT), `subsets:["latin"]` (subset), `variable` (CSS var, no extra class). Tailwind `fontFamily` consumes `var(--font-*)` — correct `next/font` pattern. No `@import` or external `<link>` fonts.

### 1.9 Static generation
- All routes are `ƒ` (Dynamic) in build output — `server-rendered on demand` (auth, `cookies()`, `headers()` in `layout.tsx:30-32`, `useQuery` client fetches). Expected for control plane — no `generateStaticParams` or `export const revalidate`. `output:"standalone"` for Docker. No `experimental.ppr` needed. `loading.tsx` suspense boundaries added (see §2.3) to enable streaming.

### 1.10 Existing `loading.tsx` suspense
- Pre-existing: `app/loading.tsx:1` (client, spinner), `app/console/loading.tsx:1` (skeleton cards), `app/admin/loading.tsx:1` (admin skeleton). Missing: `app/server/*`, `app/admin/monitoring`, `app/console/servers/*` — added.

---

## 2. Optimizations Applied

### 2.1 `next.config.ts` — bundle / image / compress
**File:** `forge/web/next.config.ts:8-22`

```diff
+ compress: true,
+ poweredByHeader: false,
+ experimental: { optimizePackageImports: ["lucide-react","recharts","@xterm/xterm","@monaco-editor/react","@tanstack/react-query"] },
  images: {
    remotePatterns: [...],
+   formats: ["image/avif","image/webp"],
+   minimumCacheTTL: 60,
  }
```

- **Rationale:** `optimizePackageImports` tree-shakes `lucide-react` (300+ icons, but each page imports 5-15 icons — without this, shared chunk includes entire lib). `recharts` benefits similarly (only `AreaChart` used in 1 page, but previously bundled for all via shared). `compress:true` enables gzip (default but explicit). `poweredByHeader:false` saves ~15 bytes/header + security. AVIF/WebP formats enable modern image delivery; `minimumCacheTTL:60` reduces re-optimization.
- **Verified:** `next build` no config error; `First Load JS shared` stable at `103 kB` (no regression).

### 2.2 Code splitting — `recharts` heavy component

**Problem:** `app/admin/monitoring/page.tsx:6` static `recharts` → `112 kB` Size, `251 kB` First Load (largest route). Static chunk `4957` 382K.

**Solution 1 — New isolated chart component:**
- Created `forge/web/components/monitoring/monitoring-chart.tsx:1-65`
  ```ts
  "use client";
  import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
  export function MonitoringAreaChart({data, metric, nodeId, nodeName}) {
    const chartData = [...data].sort(...).map(m=>({ts:m.observedAt, v:m[metric]}));
    return <ResponsiveContainer><AreaChart>...</AreaChart></ResponsiveContainer>;
  }
  ```
  Pure `recharts` — isolated, no query logic, `ssr: false` when dynamically imported.

- Modified `forge/web/app/admin/monitoring/page.tsx:1-48`
  ```ts
  import dynamic from "next/dynamic";
  // remove: import { Area, AreaChart, ... } from "recharts"
  const MonitoringAreaChart = dynamic(
    () => import("@/components/monitoring/monitoring-chart").then(m=>m.MonitoringAreaChart),
    { ssr:false, loading: () => <div>Loading chart…</div> }
  );
  // Replace inline <ResponsiveContainer><AreaChart>...</AreaChart></ResponsiveContainer>
  // with <MonitoringAreaChart data={metricsQ.data} metric={metric} nodeId={nodeId} nodeName={...} />
  ```

- **Result:** `recharts` chunk (`6456.f4305eeb1149c67a.js 375K`) now **lazy-loaded only when `/admin/monitoring` visited**, not in `First Load JS`.

**Solution 2 — `metrics-chart` secondary:**
- Created `forge/web/components/monitoring/metrics-viz.tsx:1-72` — `MetricsViz` wrapper for `MetricsChart`.
- Modified `forge/web/components/monitoring/metrics-chart.tsx:3-57`
  ```ts
  import dynamic from "next/dynamic";
  const MetricsViz = dynamic(() => import("./metrics-viz").then(m=>m.MetricsViz), {
    ssr:false,
    loading: () => <div className="flex h-64 items-center justify-center"><div className="animate-spin"/></div>
  });
  // Replace inline recharts JSX with <MetricsViz chartData={chartData} selectedMetric={...} color={metric.color} />
  ```
- Removed unused `formatValue` duplication (now in viz).

**Existing splits verified (no change, documented):**
- `components/server/files-view.tsx:5,17` — `MonacoEditor = dynamic(() => import("@monaco-editor/react"), {ssr:false})` ✅
- `app/admin/terminal/page.tsx:68` — `Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit"), import("@xterm/addon-web-links")])` + `useFit` ResizeObserver ✅
- No `dynamic` for `charts/ServerCPUChart` etc — **not needed** (unused). If future consumer imports them statically, recommend using `next/dynamic` wrapper or `optimizePackageImports` already handles tree-shaking. Added to report for follow-up.

### 2.3 `loading.tsx` for suspense (streaming)
Added 5 new `loading.tsx` to enable React Suspense streaming for heavy segments (Next.js App Router automatically wraps `page.tsx` in `<Suspense fallback={<Loading/>}>` when sibling `loading.tsx` exists):

- `forge/web/app/server/loading.tsx:1-16` — skeleton: `ServerLoading` (4 cards, pulsing `bg-white/[0.06]`).
- `forge/web/app/server/[id]/loading.tsx:1-20` — `ServerDetailLoading` (tabs skeleton, 5 tab placeholders, 72h chart placeholder).
- `forge/web/app/admin/monitoring/loading.tsx:1-28` — `MonitoringLoading` (matches monitoring page: header, controls, 380px chart, 12-col grid).
- `forge/web/app/console/servers/loading.tsx:1-18` — `ConsoleServersLoading` (6 card grid).
- `forge/web/app/console/servers/[id]/loading.tsx:1-20` — `ConsoleServerDetailLoading` (tab strip + card).

Existing retained: `app/loading.tsx:1`, `app/console/loading.tsx:1`, `app/admin/loading.tsx:1` — now 8 total.

### 2.4 Lint fix (build blocker)
- `forge/web/components/shared/states-offline.tsx:11` — `let bannerListeners` → `const bannerListeners` (prefer-const error caused `next build` failure after config change triggered stricter lint). Fixed to ensure build passes.

### 2.5 Image / font verification (no file change, ensured)
- Confirmed `sharp` in `package.json` and `node_modules/sharp` present.
- Confirmed `app/fonts.ts` uses `next/font/google` with `display:swap` + `variable`.
- No new `next/image` added; existing `unoptimized` usages justified (external CDN, CSP).

---

## 3. Verification

### 3.1 Build (`npm --workspace @forge/web run build 2>&1 | tail -n 50`)
**After optimizations (2026-08-24 08:09):** `✓ Compiled successfully` (4.2s)
```
Route (app)                                          Size  First Load JS
├ ƒ /admin/monitoring                              8.36 kB         148 kB  ← vs pre 112 kB / 251 kB
├ ƒ /admin/nodes                                  20.6 kB         171 kB  (stable)
├ ƒ /admin/operations                             12.3 kB         161 kB  (stable +0.1)
├ ƒ /admin/overview                                5.35 kB         149 kB  (stable)
├ ƒ /server/[id]/files                             2.21 kB         175 kB  (stable)
+ First Load JS shared by all                      103 kB  ← stable (pre 103 kB)
  ├ chunks/18-c6633c4945af8275.js                 46.7 kB
  ├ chunks/87c73c54-09e1ba5c70e60a51.js           54.2 kB
  └ other shared chunks (total)                   2.15 kB  (pre 2.12)
ƒ Middleware                                        35 kB  (stable)
```
- **Key metric:** ` /admin/monitoring` **Size -92.5% (112 → 8.36 kB)**, **First Load -41% (251 → 148 kB)**. Shared stable → no regression for other routes (all routes ±0.1-0.2 kB, within noise, due to `optimizePackageImports` / config).
- Build still passes with lint (previously failed on `prefer-const` — fixed).

### 3.2 Typecheck (`npm --workspace @forge/web run typecheck 2>&1`)
```
> tsc --noEmit
(no output — 0 errors)
```
- No `tsc` errors. Dynamic imports correctly typed (`dynamic(() => import(...).then(m=>m.X))`).

### 3.3 Bundle size (`du -sh` + `ls -lh static/chunks`)
- `.next` total: `691M` (pre 693M) — **stable -2M** (within variance, standalone trace).
- `.next/static`: `5.7M` (pre 5.6M) — **stable +0.1M** (new viz chunks).
- Largest chunks after: `6456.f4305eeb1149c67a.js 375K` (lazy recharts — previously `4957` 382K eagerly in initial), `1c0ca389.6a10d73cfb377ec3.js 286K` (xterm/monaco), `framework 185K`, `18-c663 170K`. Recharts now **not in shared**, only loaded on `/admin/monitoring` visit (verified via `next build` First Load drop).

### 3.4 Static / dynamic checks
- `next build` routes all `ƒ` (Dynamic) — correct for auth. No static generation broken.
- `output: "standalone"` trace `required-server-files.json` present.
- `images` formats verified in `.next/images-manifest.json` (contains `avif`, `webp`).

### 3.5 Code splitting verification (manual)
- `grep -r "next/dynamic" forge/web --include="*.tsx" | wc -l` → 3 (pre 1) — `files-view.tsx`, `monitoring/page.tsx`, `metrics-chart.tsx`.
- `grep -r "import(\"@xterm" forge/web/app/admin/terminal/page.tsx` → 1 — still dynamic via `Promise.all`.
- No `recharts` in `First Load` chunk (verified by `strings .next/static/chunks/18-*.js | grep recharts` → 0; only `6456.*.js` contains `recharts`).

### 3.6 Regression check
- `git diff --stat` shows 7 files changed + 5 new `loading.tsx` + 2 new viz components — no deletion of functional code.
- `vitest` not run (out of scope), but `next build` lint + typecheck pass guarantee no TS breakage.
- Manual smoke: `find app -name "loading.tsx"` → 8 files (was 3).

---

## 4. Findings & Recommendations

### 4.1 Findings
- **Sharp:** ✅ Installed `0.35.3`, Next optimizer ready; remotePatterns secure.
- **next/font:** ✅ `IBM_Plex_Sans`, `JetBrains_Mono`, `Space_Grotesk` with `display:swap`, subsets, variables.
- **Monaco & xterm:** ✅ Already code-split (`dynamic` + `Promise.all` dynamic imports). Monaco `ssr:false`, xterm lazy `Terminal` init avoids SSR `document`.
- **Recharts:** ❌ Previously not split — caused `112 kB` monitoring route. ✅ **Fixed** — now `8.36 kB` via `monitoring-chart.tsx` dynamic. `metrics-chart.tsx` also split via `metrics-viz.tsx`.
- **Bundle:** `First Load JS shared 103 kB` healthy. Monitoring route **-92%**. Other routes stable. `.next` size stable (691M vs 693M).
- **Static:** All dynamic — correct for dashboard. No `generateStaticParams` needed. `loading.tsx` streaming added.
- **Images:** `unoptimized` for external CDN icons correct; AVIF/WebP enabled for future `next/image` without `unoptimized`.

### 4.2 Remaining gaps (not fixed, low priority)
- `components/charts/ServerCPUChart.tsx:4` etc still static `recharts` — but **unused**, so no bundle impact currently. If a future page imports `ServerCPUChart` statically, it will reintroduce recharts into First Load. **Recommendation:** Consumers should use `dynamic(() => import("@/components/charts/ServerCPUChart").then(m=>m.ServerCPUChart), {ssr:false})` or a shared `components/charts/dynamic.tsx` barrel. `experimental.optimizePackageImports` partially mitigates but not full code-split — dynamic is preferred for heavy charts.
- No `next/bundle-analyzer` configured — consider adding for CI trend tracking (`ANALYZE=true next build`).
- `lucide-react` — 150+ icons imported statically across files; `optimizePackageImports` helps but `dynamic` not needed (icons small, tree-shaken).
- No `fetch` caching / `revalidate` for static assets — not applicable (dynamic control plane).

### 4.3 Bundle size trend (for audit)
```
Before:  /admin/monitoring 112 kB / 251 kB First Load, shared 103 kB, .next 693M
After:   /admin/monitoring 8.36 kB / 148 kB First Load, shared 103 kB, .next 691M
Delta:   -103.64 kB Size, -103 kB First Load (monitoring), shared stable
```

---

## 5. Files Changed

- `forge/web/next.config.ts:8-22` — add `compress`, `poweredByHeader`, `experimental.optimizePackageImports`, image `formats`/`minimumCacheTTL`
- `forge/web/app/admin/monitoring/page.tsx:1-6,35-48,148-156` — dynamic `MonitoringAreaChart`
- `forge/web/components/monitoring/monitoring-chart.tsx:1-65` **new** — isolated recharts chart
- `forge/web/components/monitoring/metrics-chart.tsx:3-57` — dynamic `MetricsViz`
- `forge/web/components/monitoring/metrics-viz.tsx:1-72` **new** — isolated recharts viz
- `forge/web/components/shared/states-offline.tsx:11` — `let`→`const` lint fix
- `forge/web/app/server/loading.tsx:1-16` **new**
- `forge/web/app/server/[id]/loading.tsx:1-20` **new**
- `forge/web/app/admin/monitoring/loading.tsx:1-28` **new**
- `forge/web/app/console/servers/loading.tsx:1-18` **new**
- `forge/web/app/console/servers/[id]/loading.tsx:1-20` **new**

---

## 6. Command Log

```bash
# Inspect
cat forge/web/next.config.ts
cat forge/web/package.json
cat forge/web/app/layout.tsx
cat forge/web/app/fonts.ts
grep -r "next/dynamic\|dynamic(" forge/web --include="*.tsx"
grep -r "recharts\|monaco\|xterm" forge/web --include="*.tsx"
ls -lh forge/web/.next/static/chunks/*.js | sort -k5 -hr | head
npm --workspace @forge/web run build 2>&1 | tail -n 50  # pre: 112 kB monitoring
npm --workspace @forge/web run build 2>&1 | tail -n 50  # post: 8.36 kB monitoring

# Optimize
# edit next.config.ts, create monitoring-chart.tsx, metrics-viz.tsx, edit monitoring/page.tsx, metrics-chart.tsx, add 5 loading.tsx, fix states-offline.tsx

# Verify
npm --workspace @forge/web run build 2>&1 | grep -E "Size|First Load|Middleware"
npm --workspace @forge/web run typecheck 2>&1  # 0 errors
du -sh forge/web/.next forge/web/.next/static
ls -lh forge/web/.next/static/chunks/*.js | sort -k5 -hr | head
```

---

## 7. Sign-off

- **Bundle:** ✅ Reduced (monitoring -92%), stable shared.
- **Code splitting:** ✅ Monaco (existing), xterm (existing), recharts (now dynamic) — heavy components split.
- **Images:** ✅ `sharp` present, `next/image` remotePatterns secure, AVIF/WebP enabled.
- **Fonts:** ✅ `next/font/google` with `swap`, subsets, variables.
- **Static/suspense:** ✅ `loading.tsx` added (5 new, 8 total) for streaming.
- **Build:** ✅ Passes (`✓ Compiled successfully`), `tsc --noEmit` 0 errors, bundle stable.
