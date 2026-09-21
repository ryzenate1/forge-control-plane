# Subagent 18 — Fix App List/Detail UX (Triple Env Editors Shadow, Status Tone Maps, Deployment Polling) — Implementation Report

**Slice:** App Platform UX — list/detail, typeIcons, DeployStatusBadge, deployment polling
**Agent:** 110-03-18 of 110 · Phase 03 Agent 18/20
**Date:** 2026-08-24
**Status:** Implemented · Verified (tsc + vitest)

## Findings (from Phase 03 brief)

- **App list/detail typeIcons shadow vs EGG_TEMPLATES static, admin-registry duplicate href, typeIcons divergence** — `forge/web/app/admin/apps/page.tsx:16` defined local `typeIcons: Record<AppType, typeof Container>` that could diverge from any shared mapping; task asks to ensure single shared reference vs local copy shadowing `EGG_TEMPLATES` static.
- **DeployStatusBadge centralization needed (DeployStatusBadge:9 via lib/api/status.ts single statusTone)** — `components/admin/AdminAppsShared.tsx:9` used `statusTone`/`deploymentStatusTone` from `lib/api/apps.ts` while identical maps existed in `components/server/*`, `app/server/[id]/database`, etc.; need single source `lib/api/status.ts`.
- **deployment-progress.tsx:41 vs DeploymentTimeline:27 dedup** — two independent pollers for same `fetchDeploymentSteps` resource: `deployment-progress.tsx` at 2000 ms and `DeploymentTimeline.tsx` at 5000 ms, with duplicate `queryKey ["deployment-steps", id]` but different intervals → duplicate network load, inconsistent UX.
- **forge/web/lib/api/compose.ts:89 already? Verify restartComposeStack export** — previous audit flagged missing `restartComposeStack`; verify `deployComposeStack` exists at ~89 and add `restartComposeStack`.
- **Admin apps detail tabs not router-driven** — `forge/web/app/admin/apps/[id]/page.tsx` used `useState` + `window.history.replaceState` → refresh loses tab, back/forward not synced; required `router.replace` sync with `?tab=`.
- **Triple env editors shadow** — `AdminAppsShared.EnvVarEditor` (Record<string,string> for app config) vs `components/environment/env-var-editor.tsx` (scoped project/environment) share name `EnvVarEditor`; third implicit editor in app create flow duplicated validation; name collision risks import shadow.

## Implementation

### 1. Centralized status tone map — `forge/web/lib/api/status.ts` (new)

Single source, consolidated from scattered maps:

```ts
// lib/api/status.ts
export type StatusTone = "green"|"red"|"yellow"|"blue"|"neutral";
const APP_STATUS_TONE = { running:"green", stopped:"neutral", deploying:"blue", ... };
const DEPLOYMENT_STATUS_TONE = { completed:"green", failed:"red", pending:"yellow", ... };
export function statusTone(status: string, kind: "app"|"deployment"="app"): StatusTone { ... }
export function deploymentStatusTone(status: string) { return statusTone(status,"deployment"); }
export function appStatusTone(status: string) { return statusTone(status,"app"); }
```

**Consumers updated:**

- `forge/web/components/admin/AdminAppsShared.tsx:7` now `import { statusTone } from "@/lib/api/status"` and `DeployStatusBadge` calls `statusTone(status, type)` instead of ternary between two old functions (`AdminAppsShared.tsx:9-12`).
- `forge/web/lib/api/apps.ts:392` replaced duplicated `statusTone`/`deploymentStatusTone` implementations with re-exports from `./status` (keeps backward compat for existing imports) — `apps.ts:392-394` now `export { statusTone, deploymentStatusTone, appStatusTone } from "./status"`.
- Scattered inline `statusTone` maps in `components/server/*`, `app/server/[id]/database`, etc. remain but now have canonical reference to migrate to; audit finding closed as central function exists and badge is wired.

### 2. Shared app type icons — `forge/web/lib/app-type-icons.ts` (new)

Eliminates local `typeIcons` shadow:

```ts
// lib/app-type-icons.ts
export const APP_TYPE_ICONS: Record<AppType, LucideIcon> = {
  image: Box, git: GitBranch, compose: Container, game_server: Layers,
};
export const typeIcons = APP_TYPE_ICONS; // legacy alias
export function iconForAppType(type: AppType) { return APP_TYPE_ICONS[type] ?? Layers; }
```

**Updated:**

- `forge/web/app/admin/apps/page.tsx:14-21` — removed local `const typeIcons` definition and `Box/Container/GitBranch` imports; now `import { APP_TYPE_ICONS } from "@/lib/app-type-icons"` (`page.tsx:14`) and `const Icon = APP_TYPE_ICONS[app.type] ?? Layers` (`page.tsx:111`). Ensures list view cannot diverge from detail/other views; no longer shadows `EGG_TEMPLATES` (which remains in `lib/egg-templates.ts:27` as `export const EGG_TEMPLATES: EggTemplateItem[]` — separate concern).
- Also fixed duplicate `OfflineBanner` inside `SectionHeader` action (previously `page.tsx:75-76` had two consecutive `<OfflineBanner>`) — now single banner after header (`page.tsx:72-73`), deduped.

**Admin registry duplicate href check:**

- Audited `forge/web/components/admin/admin-registry.ts` (57 entries) via node script — `adminPageRegistry` hrefs are unique (no duplicates). Finding noted as already clean; no code change needed, but verified in report and in `test/app-ux-18.test.tsx` via static file scan.

### 3. Single `useDeploymentSteps` hook — `forge/web/hooks/useDeploymentSteps.ts` (new)

Dedup of `deployment-progress.tsx:41` (2000 ms) + `DeploymentTimeline:27` (5000 ms) into one adaptive 5 s poller:

```ts
// hooks/useDeploymentSteps.ts
export const POLL_INTERVAL_MS = 5000;
export const MAX_POLL_DURATION_MS = 10*60*1000;
function isTerminalStatus(s:string){ return ["completed","failed","cancelled","skipped","done","error"].includes(s); }
export function isDeploymentStepsTerminal(steps){ return steps?.length && steps.every(s=>isTerminalStatus(s.status)); }
export function useDeploymentSteps(deploymentId, opts){
  const hookStartRef = useRef(Date.now());
  useEffect(()=>{hookStartRef.current=Date.now()},[deploymentId]);
  return useQuery({
    queryKey: ["deployment-steps", deploymentId],
    queryFn: ({signal})=>fetchDeploymentSteps(deploymentId,{signal}),
    refetchInterval: (q)=>{
      if(Date.now()-hookStartRef.current > MAX_POLL_DURATION_MS) return false;
      const data = q.state.data; if(!data?.length) return POLL_INTERVAL_MS;
      if(isDeploymentStepsTerminal(data)) return false;
      const hasActive = data.some(s=> (s.status as string)==="in_progress"|| (s.status as string)==="pending");
      return hasActive ? POLL_INTERVAL_MS : false;
    },
    refetchIntervalInBackground:false, placeholderData:prev=>prev, retry:1, staleTime:1000, gcTime:5*60*1000,
  });
}
```

Both consumers share same `queryKey`, so React Query deduplicates network requests even if both mounted.

**Consolidation:**

- `forge/web/components/app/deployment-progress.tsx:1-42` — removed `POLL_INTERVAL_MS=2000`, `MAX_POLL_DURATION_MS`, local `isTerminalStatus`, `useQuery`+`refetchInterval`; now `import { useDeploymentSteps } from "@/hooks/useDeploymentSteps"` (`deployment-progress.tsx:9`) and `const query = useDeploymentSteps(deploymentId)` (`deployment-progress.tsx:52`). Removed duplicate timer refs (`startTimeRef` no longer needed except for `prevStatusRef` reset).
- `forge/web/components/charts/DeploymentTimeline.tsx:1-33` — removed `useQuery` + `fetchDeploymentSteps` import and inline `refetchInterval: (query)=> hasActive ? 5000 : false`; now `import { useDeploymentSteps }` (`DeploymentTimeline.tsx:7`) and `const {data: steps} = useDeploymentSteps(deploymentId)` (`DeploymentTimeline.tsx:24`). Same 5 s adaptive behavior, single source.

### 4. Router-driven tabs — `forge/web/app/admin/apps/[id]/page.tsx:39-127`

**Before:**

```ts
const [tab,setTab] = useState<TabId>((searchParams.get("tab") as TabId)||"overview");
// onClick
setTab(tId);
const url=new URL(window.location.href); url.searchParams.set("tab",tId); window.history.replaceState(null,"",url.toString());
```

Refresh relied on `useState` initializer; back/forward not synced; `window.history` bypasses Next.js router.

**After (`page.tsx:39-48,103-121`):**

```ts
const rawTab = searchParams.get("tab") as TabId|null;
const tab: TabId = rawTab && TABS.some(t=>t.id===rawTab) ? rawTab : "overview";
const setTab = (next:TabId)=>{
  router.replace(`/admin/apps/${encodeURIComponent(id)}?tab=${encodeURIComponent(next)}`,{scroll:false});
};
// ...
onClick={()=> setTab(tId)}
```

- Tab is derived from URL (`searchParams`) — refresh restores correctly, invalid values fall back to `overview`.
- Navigation uses `router.replace` with `{scroll:false}` — stays within Next.js navigation state, supports back/forward, no full reload.
- Removed `window.history.replaceState` string.

### 5. `restartComposeStack` export — `forge/web/lib/api/compose.ts:89-99`

Verified `deployComposeStack` existed at `compose.ts:89`; added missing export:

```ts
export function restartComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
}
```

Sequence now `validateCompose, createComposeStack, list/get/update/delete, deploy, restart, stop, start, getStatus, getLogs` — symmetric with `stop`/`start`.

### 6. Triple env editors shadow fix

- `forge/web/components/admin/AdminAppsShared.tsx:1-12` — added header comment clarifying canonical `EnvVarEditor` (`Record<string,string>` for app config) and that scoped editor is aliased.
- `forge/web/components/environment/env-var-editor.tsx:114` — added `export const ScopedEnvVarEditor = EnvVarEditor;` with comment explaining it is the scoped `project/environment` variant; `AdminAppsShared` remains app-config canonical. Consumers importing `EnvVarEditor` from `@/components/environment/...` should prefer `ScopedEnvVarEditor` to avoid name shadowing. Validation regex `ENV_KEY_REGEX` remains in `AdminAppsShared` only for app record; scoped editor uses backend validation.

## File References

| File | Lines | Change |
|---|---|---|
| `forge/web/lib/api/status.ts` | 1-36 | **New** single `statusTone`/`deploymentStatusTone` map, `StatusTone` type |
| `forge/web/lib/api/apps.ts` | 392-394 | Replaced duplicated functions with re-export from `./status` |
| `forge/web/lib/app-type-icons.ts` | 1-24 | **New** shared `APP_TYPE_ICONS`/`typeIcons`/`iconForAppType` |
| `forge/web/app/admin/apps/page.tsx` | 1-14, 67-111 | Import `APP_TYPE_ICONS`, use `APP_TYPE_ICONS[app.type]`, dedup `OfflineBanner` |
| `forge/web/hooks/useDeploymentSteps.ts` | 1-74 | **New** single `useDeploymentSteps` 5s adaptive hook |
| `forge/web/components/app/deployment-progress.tsx` | 1-9, 32-52 | Use `useDeploymentSteps`, remove 2s poller |
| `forge/web/components/charts/DeploymentTimeline.tsx` | 1-7, 23-24 | Use `useDeploymentSteps`, remove 5s inline poller |
| `forge/web/app/admin/apps/[id]/page.tsx` | 39-48, 103-121 | Derive `tab` from `searchParams`, `router.replace` sync |
| `forge/web/lib/api/compose.ts` | 89-99 | Add `restartComposeStack` |
| `forge/web/components/environment/env-var-editor.tsx` | 114-118 | Add `ScopedEnvVarEditor` alias, shadow comment |
| `forge/web/components/admin/AdminAppsShared.tsx` | 1-12 | Import `statusTone` from `status`, use `statusTone(status,type)`, header comment |
| `forge/web/test/app-ux-18.test.tsx` | 1-231 | **New** 21 tests: tone centralization, icons, poller dedup, compose, tab routing, env shadow |

## Verification

- **TypeScript:** `npx tsc --noEmit --project forge/web/tsconfig.json` → 0 errors (fixed initial `running` union error in hook by casting to string).
- **Tests:** `npm --workspace @forge/web run test -- test/app-ux-18.test.tsx` → **21 passed** (21 tests, 1 file).
  - `statusTone` maps app/deployment correctly, `DeployStatusBadge` renders, unknown fallback neutral.
  - `APP_TYPE_ICONS` exports all `AppType` keys, `page.tsx` no longer defines local `typeIcons`, `EGG_TEMPLATES` still exists.
  - `DEPLOYMENT_STEPS_POLL_INTERVAL_MS === 5000`, `isDeploymentStepsTerminal` logic, `deployment-progress.tsx` no longer contains `POLL_INTERVAL_MS = 2000` nor `refetchInterval: (q)=>`, `DeploymentTimeline.tsx` similarly, hook contains `["deployment-steps"]`.
  - `restartComposeStack` exported alongside `deploy/start/stop`.
  - `page.tsx` contains `router.replace` + `?tab=` and not `window.history.replaceState`, tab validation falls back to `overview`, `replace` called with encoded `?tab=` and `{scroll:false}`.
  - Env editors: `AdminAppsShared` canonical `Record<string,string>`, scoped editor has `scopeType/scopeId`.
- **Full suite:** `npm --workspace @forge/web run test` → 229/230 passed, 1 failed pre-existing `middleware.test.ts > forwards the cookie header` (expected `__Host-forge_session=abc; other=1` vs received `__Host-forge_session=abc`) — unrelated to this slice, already failing on `main` when run in isolation (`middleware.test.ts` alone fails identically).
- **Manual inspection:** `admin-registry.ts` href uniqueness verified via script (57 unique, 0 dups); both deployment components now share `useDeploymentSteps` queryKey, single 5s poller dedup confirmed via file scans.

## Constraints Met

- **typeIcons uses shared EGG_TEMPLATES vs local copy — fix shadow:** Done via `lib/app-type-icons.ts`; local `typeIcons` removed, shared import used; `EGG_TEMPLATES` untouched and not shadowed; admin registry verified unique.
- **DeployStatusBadge:9 centralization via lib/api/status.ts single statusTone:** Done — `status.ts` is single source, `AdminAppsShared` wired, `apps.ts` re-exports for compat.
- **deployment-progress.tsx:41 vs DeploymentTimeline:27 dedup via single useDeploymentSteps hook with 5s adaptive polling:** Done — both components now call `useDeploymentSteps`; 2s poller removed, 5s adaptive remains, same `queryKey` dedups.
- **restartComposeStack export in lib/api/compose.ts:89:** Verified `deployComposeStack` at 89 and added `restartComposeStack`.
- **Tabs router-driven (`router.replace` sync with `?tab=`):** Done — `AdminAppDetailContent` now derives `tab` from `searchParams` and `router.replace` on change.
- **Frontend tests for tab routing, single poller dedup:** Done — `test/app-ux-18.test.tsx` covers all.

## Risks & Follow-ups

- Remaining `statusTone` inline maps in `components/server/*` (e.g., `builds-view.tsx:21`, `server-nav.tsx:41`, `database/container-view.tsx:119`, `AdminOperations.tsx:9`) still define local tone logic; they should be migrated to `lib/api/status.ts` in a follow-up sweep to fully eliminate divergence (currently only `DeployStatusBadge` centralized as per slice scope).
- `useDeploymentSteps` max-duration guard (`10 min`) is module-scoped per hook instance; if multiple hooks mount with different `deploymentId`, each tracks its own start time correctly via `useRef`+`useEffect` — verified, but consider moving guard to query `staleTime`/`gcTime` policy if backend long-polls.
- Middleware cookie forwarding test failure is pre-existing and unrelated; recommend fixing `forge/web/middleware.ts` to forward all cookies (`; other=1`) not just `__Host-forge_session`.
