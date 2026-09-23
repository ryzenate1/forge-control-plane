# Subagent 12 — Web App/Compose/Deployments Tests (Phase 08)

**Agent:** 110-08-12 of 110 — Phase 08 Agent 12/20  
**Focus:** Web App/Compose/Deployments test files  
**Date:** 2026-08-24  
**Workspace:** `/Users/riyaz/project/gamepanel`

---

## 1. Task Checklist (as assigned)

| # | Task | Status |
|---|------|--------|
| 1 | Inspect `forge/web/app/admin/apps`, `compose`, `deployments`, `preview-deployments`, `lib/api/apps.ts`, `lib/api/compose.ts`, `lib/api/status.ts`, `hooks/useDeploymentSteps.ts` | ✅ Done |
| 2 | Verify existing `forge/web/test/app-ux-18.test.tsx` (21 tests) already covers `statusTone`, `APP_TYPE_ICONS`, `useDeploymentSteps` | ✅ Done |
| 3 | Create/augment `forge/web/test/compose-fidelity.test.tsx` covering `TestEnvFile_NotSupported`, `TestShortFormHostPort`, `TestComposeCreate with nodeId` | ✅ Done (37 tests) |
| 4 | Run `npm --workspace @forge/web run test -- test/app-ux-18.test.tsx` and full `npm --workspace @forge/web run test` and report tails | ✅ Done |

---

## 2. Inspected Files

### 2.1 `forge/web/app/admin/apps/page.tsx:1`
- **Purpose:** Admin Apps list. Uses `APP_TYPE_ICONS` from `lib/app-type-icons.ts:1` (no local `const typeIcons` shadow). `OfflineBanner` imported once — audit check `OfflineBanner` count ≤2 passes.
- **Key wiring:** `fetchApps`, `startApp`, `stopApp`, `restartApp`, `deleteApp` from `lib/api/apps.ts:235`; `DeployStatusBadge` via centralized `statusTone`; `typeLabel` for pills; TanStack Query `refetchInterval: 15_000`.

### 2.2 `forge/web/app/admin/apps/new/page.tsx:1`
- **Purpose:** 4-step wizard (source → template → configure → review). `SOURCE_TYPES` = image/git/compose (game_server filtered out of selector).
- **Validation:** `validateAll()` checks name regex, image regex, git URL `VALID_GIT_URL`, compose `services:` presence; resources numeric; domain regex.
- **NodeId path (CreateApp):** `const [nodeId,setNodeId]=useState("")` :38, `fetchNodes` query :112, `<select value={nodeId}>` :591-599, `createApp` payload `nodeId: nodeId || undefined` :244 — mirrors `CreateAppInput.nodeId?: string` :139-140. Also `regionId`. Review step renders node name via `nodes.find(n=>n.id===nodeId)`.
- **Compose handling:** file upload → `setComposeContent`; textarea + `composeValidation` (checks `services:` regex); `validateCompose` local (not backend) but `createApp` forwards `composeContent` to backend where `env_file` strict gate lives.

### 2.3 `forge/web/app/admin/apps/[id]/page.tsx:1`
- **Purpose:** Detail with router-driven tabs. `useSearchParams().get("tab")` :43, `TABS.some` validation :44, `router.replace(...?tab=...,{scroll:false})` :49 — no `window.history.replaceState` divergence (checked in `app-ux-18.test.tsx:174`).
- **Tabs:** overview, deployments, configuration, logs, console, domains, backups. Each tab uses dedicated fetchers (`fetchAppDeployments`, `fetchAppLogs`, `fetchAppDomains`, `fetchAppBackups`, `fetchApp`).

### 2.4 `forge/web/app/admin/compose/page.tsx:1`
- **Purpose:** Compose stacks list. 9 inventoried states via `composeStatusTone` (`lib/api/status.ts:15`) — `statusConfig` maps each to `composeStatusTone("running")` etc. `stackTone()` and `healthTone()` both delegate to single source.
- **Lifecycle:** `fetchJSON("/compose")`, node/type filters, degraded banner (`degraded`/`failed`), actions `stop/start/restart/delete` via `deleteJSON`/`postJSON`.
- **Node pill:** `stack.nodeId.slice(0,8)` :268 — fidelity checked.

### 2.5 `forge/web/app/admin/compose/new/page.tsx:1`
- **Purpose:** New stack wizard. `TEMPLATES` 3, upload, textarea, `validateCompose` mutation, `createComposeStack({name,composeYaml,composeType,sourceType:"raw",nodeId: nodeId || undefined})` :53.
- **NodeId path:** `const [nodeId,setNodeId]=useState("")` :38, `fetchNodes` :39, `<select value={nodeId}>` :109-119 with `Auto-select` option. `canDeploy = name.trim() && composeYaml.trim() && validateResult?.valid` :80 disables Deploy until Valid.
- **Validate UI:** `Validate` / `Deploy` buttons, `validateResult.valid ? Valid : Invalid` :179-187, `validateResult.errors[].field` and `warnings` rendering.

### 2.6 `forge/web/app/admin/compose/[id]/page.tsx:1`
- **Purpose:** Detail with router-driven tab (`COMPOSE_TABS` + `router.replace(...?tab=...)` :38-41), status via `composeStatusTone`, services via `status.services`, logs via `getComposeStackLogs`.

### 2.7 `forge/web/app/admin/deployments/page.tsx:1`
- **Purpose:** Server + App deployments unified. Tabs `servers|apps` router-driven :47-52, `deploymentStatusTone` single source for `statusConfig` :33-39, `DeployStatusBadge` for app deployments.
- **History link:** `/admin/deployments/history` (history page uses `deploymentStatusTone` for `pending|running|done|error|cancelled`).

### 2.8 `forge/web/app/admin/preview-deployments/page.tsx:1`
- **Purpose:** Preview PR environments. `previewStatusTone` single source for 5 states :46-52, TTL/limit/reaper lifecycle card, canonical `/preview` with fallback `/admin/preview-deployments`.

### 2.9 `forge/web/lib/api/apps.ts:1`
- **Types:** `AppType`, `AppStatus`, `DeploymentStatus`, `ApiApp`, `ApiAppDetail`, `CreateAppInput` with `nodeId?: string` :139, `sourceType`/`sourceConfig`.
- **Exports:** `fetchApps`, `fetchApp`, `createApp` :243 (`postJSON("/apps",input)`), `fetchAppDeployments`, `fetchAllDeployments` (`/admin/deployments`), `triggerDeploy`, `fetchAppCertificates` etc.
- **Centralization:** `export {statusTone,deploymentStatusTone,appStatusTone} from "./status"` :393 — no local tone map drift.

### 2.10 `forge/web/lib/api/compose.ts:1`
- **Interfaces:** `ComposeStack`, `ServiceState`, `StackStatusResponse`, `ComposeValidateResult` with `errors?: ComposeValidationError[]`, `warnings?`, `summary?`.
- **Functions:** `validateCompose(content)` :49 `postJSON('/compose/validate',{content})`, `createComposeStack(body)` :53 `postJSON('/compose',body)` with `nodeId?: string` :56, `list/get/update/delete/deploy/restart/stop/start/status/logs`.
- **Restart export:** `export function restartComposeStack(id)` :93 alongside `deploy/start/stop` — checked in `app-ux-18.test.tsx:157`.

### 2.11 `forge/web/lib/api/status.ts:1`
- **Single source:** `APP_STATUS_TONE` :24, `DEPLOYMENT_STATUS_TONE` :36 (72 entries covering compose/preview/source/build/server), `COMPOSE_STATUS_INVENTORY` :76 (9), `PREVIEW` :89 (5), `SOURCE` :98 (11), `BUILD` :113, `SERVER_DEPLOYMENT` :122.
- **Unified:** `statusTone(status,kind)` :149 with kind-specific inventories + fallthrough; helpers `composeStatusTone`, `previewStatusTone`, `sourceStatusTone`, `buildStatusTone`, `serverDeploymentStatusTone`; `pillToneToStatusPillTone` and `statusPillTone`.
- **Verified invariants (app-ux-18):** `statusTone("running","app")==="green"`, `deploymentStatusTone("completed")==="green"`, unknown→neutral, `COMPOSE_STATUS_INVENTORY` docs 9 states.

### 2.12 `forge/web/hooks/useDeploymentSteps.ts:1`
- **Single poller dedup:** `POLL_INTERVAL_MS=5000` :20, `MAX_POLL_DURATION_MS=10*60*1000` :21, `isTerminalStatus` includes `completed|failed|cancelled|skipped|done|error` :23, `isDeploymentStepsTerminal` every-terminal :34, `useDeploymentSteps` :39 with `queryKey ["deployment-steps",id]` :49, `refetchInterval` adaptive 5s :53-62 (returns false when terminal or elapsed), `refetchIntervalInBackground:false`, `placeholderData`, `retry:1`.
- **Exported constants:** `DEPLOYMENT_STEPS_POLL_INTERVAL_MS`, `DEPLOYMENT_STEPS_MAX_DURATION_MS`.
- **Phase 03 fix verified:** `deployment-progress.tsx` and `DeploymentTimeline.tsx` no longer define `POLL_INTERVAL_MS=2000` nor inline `refetchInterval: (q)=>`, both import `useDeploymentSteps`.

---

## 3. Existing Test – `forge/web/test/app-ux-18.test.tsx` (21 tests)

**Status:** ✅ Pass (21/21)

Verified via `npm --workspace @forge/web run test -- test/app-ux-18.test.tsx` :

```
✓ test/app-ux-18.test.tsx (21 tests) 23ms
Test Files 1 passed (1)
Tests 21 passed (21)
```

**Coverage already present:**

| Area | Tests | Assertions |
|------|-------|------------|
| `statusTone centralization` | 6 | app/deployment mapping, wrapper `deploymentStatusTone`, `appStatusTone`, `DeployStatusBadge`, unknown fallback |
| `APP_TYPE_ICONS` centralization | 3 | exports for all AppType, `page.tsx` imports `APP_TYPE_ICONS` not `const typeIcons`, `EGG_TEMPLATES` separate, OfflineBanner count ≤2 |
| `useDeploymentSteps` single poller | 5 | 5s not 2s constant, `isDeploymentStepsTerminal` logic, `deployment-progress.tsx` uses hook no 2s, `DeploymentTimeline.tsx` uses hook no inline refetch, adaptive terminal |
| `restartComposeStack` export | 1 | typeof function, file contains `deploy/start/stop/restart` |
| `tab routing router-driven` | 4 | `router.replace` with `?tab=`, no `window.history.replaceState`, validation fallback, `setTab` encodes, refresh restores via `searchParams.get("tab")` |
| `triple env editors shadow fix` | 2 | `EnvVarEditor` canonical `Record<string,string>`, `environment/env-var-editor.tsx` scoped with `scopeType/scopeId` |

**Conclusion:** No augmentation needed for app-ux-18 slice; all Phase 03/06 web UX consolidations intact.

---

## 4. New Test – `forge/web/test/compose-fidelity.test.tsx` (37 tests)

**Status:** ✅ Created, 37/37 pass

**Path:** `forge/web/test/compose-fidelity.test.tsx:1`

### 4.1 Design Rationale

Mirrors three Go reference suites into the web layer:

| Go Reference | Web Fidelity Test | Locus |
|--------------|-------------------|-------|
| `forge/api/internal/services/compose/compose_fixes_test.go:11 TestEnvFile_NotSupported` + `env_file_test.go:12` | `TestEnvFile_NotSupported` 10 tests | `lib/api/compose.ts:49 validateCompose`, `handlers_compose.go:106` strict 400 path |
| `beacon/internal/server/compose_fixes_test.go:12 TestShortFormHostPort_Fixes` + `beacon/internal/server/compose.go:286 shortFormHostPort` | `TestShortFormHostPort` 18 tests | TS replica `shortFormHostPort` + `isPrivilegedHostPort` + file-content check of `beacon/internal/server/compose.go` |
| `handlers_compose.go:45 CreateStackRequest` with `nodeId` + `lib/api/compose.ts:53 createComposeStack` | `TestComposeCreate with nodeId` 9 tests | `lib/api/compose.ts:56`, `app/admin/compose/new/page.tsx:38` nodeId wiring, integration render |

### 4.2 `TestEnvFile_NotSupported` – 10 tests

| # | Test | Assertion |
|---|------|-----------|
| 1 | `validateCompose: strict 400 path surfaces env_file error as ApiError` | `mockFetch(jsonResponse({valid:false,errors:[{field:"env_file",message:"env_file not supported, inline env vars"}]},400))` → `validateCompose` throws `ApiError` with `status===400` and message `/400|env_file/i` |
| 2 | `validateCompose: strict 200-invalid payload contains env_file field error` | Mock 200 `{valid:false, errors:[env_file]}` → `result.valid===false` and `errors[0].message==="env_file not supported, inline env vars"` |
| 3 | `env_file string form also rejected` | Same as above with `env_file: .env` string form |
| 4 | `clean compose without env_file passes strict` | Mock 200 `{valid:true, summary:{services:[...]}}` → `valid===true` |
| 5 | `non-strict warns but passes` | Mock 200 `{valid:true, warnings:[env_file]}` → `valid===true` and `warnings` contains `env_file` |
| 6 | `validateCompose sends { content } payload` | `mockFetch` + `requestJSON(calls[0])` asserts `url.contains("/compose/validate")` and `body.content===input` |
| 7 | `createComposeStack also rejects env_file via 400` | `mockFetch(400 {message:"env_file not supported..."})` → `createComposeStack` rejects with `/400|env_file/` |
| 8 | `compose new page wires validate button and renders Valid/Invalid` | `readFileSync("app/admin/compose/new/page.tsx")` contains `validateCompose`, `validateResult`, `Valid`, `Invalid`, `validateResult.errors`, import from `lib/api/compose` |
| 9 | `compose new page file does not swallow env_file – backend strict surfaces field env_file` | `readFileSync("lib/api/compose.ts")` contains `validateCompose`, `/compose/validate`, `ComposeValidationError` |
| 10 | `backend handler file correctly maps env_file to 400` | `readFileSync("../../api/internal/http/handlers_compose.go")` contains `env_file`, `StatusBadRequest`, count ≥3 (validate + create + patch) |

Fidelity notes:
- Web does not duplicate Go's `ParseComposeYAML` `FORGE_ENV_FILE_STRICT` branching; it delegates to backend via `validateCompose`/`createComposeStack`. Tests assert the HTTP contract (400 for strict, warning for non-strict) rather than re-implementing Go parser.

### 4.3 `TestShortFormHostPort` – 18 tests

**TS replica (file-local):**
```ts
function shortFormHostPort(entry: string): string {
  entry=entry.trim(); if(!entry) return "";
  const slash=entry.indexOf("/"); if(slash!==-1) entry=entry.slice(0,slash);
  entry=entry.trim(); if(!entry) return "";
  const parts=entry.split(":");
  switch(parts.length){ case 1: return parts[0]; case 2: return parts[0]; case 3: return parts[1]; default: if(parts.length>3) return parts[parts.length-2]; return ""; }
}
```

| # | Test |
|---|------|
| 1-9 | `it.each(vectors)` 9 Go reference vectors: `80→80`, `80/tcp→80`, `8080:80→8080`, `8080-8082:80-82→8080-8082`, `127.0.0.1:8080:80→8080`, `127.0.0.1::80→""`, `127.0.0.1:8080-8082:80-82→8080-8082`, `8080:80/udp→8080`, `[::1]:8080:80→8080` |
| 10 | strips `/tcp /udp /sctp` before split |
| 11 | range form extracts host side correctly + low-end `split("-",2)[0]==="8080"` |
| 12 | random host `ip::container` returns empty so privileged gate skipped |
| 13 | privileged detection: `80-82` and `shortFormHostPort("80-82:8080")` → privileged true; `8080-8082:80` not privileged |
| 14 | len1 privileged `80` rejected, `8080` passes |
| 15 | handles `int/float/proto` edge: trimmed spaces, empty, ` 8080:80/tcp ` |
| 16 | IPv6 bracket second-last heuristic `ports[len-2]` matches `beacon/internal/server/compose.go:310` |
| 17 | file-content check `../../../beacon/internal/server/compose.go` contains `func shortFormHostPort`, `8080-8082`, `validateComposePorts`, `case int:`, `case int64:`, `strings.Index(entry, "/")` |
| 18 | `app/admin/compose/[id]/page.tsx` shows `svc.ports` verbatim and does *not* define its own `function shortFormHostPort` shadow |

### 4.4 `TestComposeCreate with nodeId` – 9 tests

| # | Test |
|---|------|
| 1 | `createComposeStack forwards nodeId when provided` — `mockFetch(jsonResponse(stack))` → `requestJSON(calls[0]).nodeId==="node-abc"`, `url.contains("/compose")`, `method==="POST"` |
| 2 | `createComposeStack sends nodeId as undefined when auto-select` — body `nodeId===undefined` (JSON omits key, scheduler auto) |
| 3 | `also forwards composeType and sourceType alongside nodeId` — `memoryMb`/`composeType`/`sourceType` asserted |
| 4 | `lib/api/compose.ts correctly defines nodeId?: string optional` — file contains `nodeId?: string` and `postJSON<ComposeStack>('/compose'` |
| 5 | `NewComposeStackPage wires nodeId select to createComposeStack payload` — file contains `nodeId`, `fetchNodes`, `createComposeStack`, `nodeId: nodeId || undefined`, `Auto-select`, `value={nodeId}` |
| 6 | **Integration** `NewComposeStackPage deploy mutation posts to /compose with nodeId` — `mockFetchByUrl({"/nodes":nodes,"/compose/validate":valid,"/compose": (url,init)=>{ assert body.nodeId }})` → render page, type name, type yaml, `select node-2`, click Validate → `Valid`, click Deploy → `push` called with `/admin/compose/new-stack-id` |
| 7 | `Create App page also forwards nodeId for compose sourceType` — `apps/new/page.tsx` contains `nodeId`, `createApp`, `nodeId: nodeId`, `fetchNodes` |
| 8 | `backend handlers_compose.go CreateStackRequest correctly requires nodeId optional` — file contains `type CreateStackRequest`, `NodeID string`, `NodeID: req.NodeID`, checks `req.Name/composeYAML` but *not* `req.NodeID==""` |
| 9 | `compose list page shows Node pill sliced to 8` — `compose/page.tsx` contains `stack.nodeId` and `slice(0, 8)` |

**Mocking note:** `ForgeApiClient` singleton dispatch via `globalThis.fetch` is honored by `vi.stubGlobal("fetch",…)` from `test/fetch-mock.ts:26`. `mockFetchByUrl` would also work but `mockFetch` sequential is simpler for single-endpoint tests; integration test uses `mockFetchByUrl` to route `/nodes` vs `/compose/validate` vs `/compose`.

---

## 5. Test Runs

### 5.1 `npm --workspace @forge/web run test -- test/app-ux-18.test.tsx`

```
> @forge/web@0.1.0 test
> vitest run test/app-ux-18.test.tsx

 RUN  v3.2.7 /Users/riyaz/project/gamepanel/forge/web

 ✓ test/app-ux-18.test.tsx (21 tests) 23ms

 Test Files  1 passed (1)
      Tests  21 passed (21)
   Start at  07:34:21
   Duration  3.07s (transform 327ms, setup 214ms, collect 541ms, tests 32ms, environment 789ms, prepare 146ms)
```

### 5.2 `npm --workspace @forge/web run test -- test/compose-fidelity.test.tsx`

```
> @forge/web@0.1.0 test
> vitest run test/compose-fidelity.test.tsx

 ✓ test/compose-fidelity.test.tsx (37 tests) 928ms
   ✓ TestComposeCreate with nodeId (compose create fidelity) > NewComposeStackPage deploy mutation posts to /compose with nodeId (integration via mocked fetch)  899ms

 Test Files  1 passed (1)
      Tests  37 passed (37)
   Start at  07:33:53
   Duration  1.85s (transform 282ms, setup 89ms, collect 140ms, tests 928ms, environment 281ms, prepare 55ms)
```

### 5.3 Full `npm --workspace @forge/web run test` (tail -n 30)

Representative run (flaky baseline, 3-4 pre-existing failures, not introduced by this slice):

```
 FAIL  middleware.test.ts > middleware > protected paths with a session cookie > forwards the cookie header to the validation request
 FAIL  test/admin-overview.test.tsx > AdminMonitoring — isSynthetic > shows synthetic allocated-capacity banner and No data for synthetic networkRxBytes metric
 FAIL  test/admin-overview.test.tsx > AdminMonitoring — isSynthetic > period switches retain isSynthetic semantics
 Test Files  2 failed | 22 passed (24)
      Tests  3 failed | 345 passed (348)
```

**Baseline without `compose-fidelity.test.tsx`:**
```
 FAIL  ... same 3 (2 synthetic + middleware) ...
 Test Files  2 failed | 21 passed (23)
      Tests  3 failed | 308 passed (311)
```

**Interpretation:** No new deterministic failures introduced. The two `isSynthetic` failures and `middleware` failure are pre-existing and flaky (observed alternating with `ui-contracts.test.tsx > keeps file save disabled…` across 3 consecutive runs). `compose-fidelity.test.tsx` itself is stable (37/37 over 3 runs) and `app-ux-18.test.tsx` stable (21/21).

**Full duration:** ~7.88s (transform 2.97s, setup 4.39s, collect 9.08s, tests 29.43s, environment 15.60s, prepare 2.36s).

---

## 6. Verdict

| Slice | Result | Evidence |
|-------|--------|----------|
| **Existing app-ux-18** | ✅ PASS | 21/21, statusTone/APP_TYPE_ICONS/useDeploymentSteps intact |
| **New compose-fidelity** | ✅ PASS | 37/37, covers Go parity vectors + API contract + nodeId wiring |
| **Full web suite** | ⚠️ PASS with pre-existing flake | 345/348 (or 344/348) pass; 3 flaky failures unrelated to this slice (isSynthetic/middleware) |
| **Compose fidelity risk** | ✅ MITIGATED | env_file strict vs non-strict, host-port short-form + privileged range, nodeId auto-select vs pinned all have explicit tests; integration render proves Deploy posts nodeId |

**Overall Phase 08 Subagent 12:** **PASS** — Web App/Compose/Deployments files inspected, existing 21-test suite confirmed, new 37-test `compose-fidelity.test.tsx` augments coverage for `TestEnvFile_NotSupported` / `TestShortFormHostPort` / `TestComposeCreate with nodeId` per Go reference, two targeted runs green, full suite shows only pre-existing flaky failures.

---

## 7. Files Changed/Created

| Path | Action |
|------|--------|
| `forge/web/test/compose-fidelity.test.tsx:1` | **Created** (37 tests) |
| `audits/110-phase-08-tests/subagent-12-web-apps.md` | **Created** (this report) |

No production code changes required; fidelity is verification-only for Phase 08.

---

## 8. References

- `forge/web/lib/api/status.ts:1` single source (Phase 03 + 06)
- `forge/web/hooks/useDeploymentSteps.ts:1` 5s adaptive dedup
- `forge/web/lib/api/compose.ts:1` validate/create with nodeId
- `forge/api/internal/services/compose/service.go:19` `ErrEnvFileNotSupported`, `isEnvFileStrict` :21, `ValidateCompose` :293
- `forge/api/internal/http/handlers_compose.go:106` validate 400 on env_file, `CreateStackRequest:45` nodeId optional
- `beacon/internal/server/compose.go:286` `shortFormHostPort`, `validateComposePorts:234` privileged range + int/int64/float64
- `beacon/internal/server/compose_fixes_test.go:12` 9 vectors + `TestValidateComposePorts_RangePrivileged` :36
- `forge/web/test/app-ux-18.test.tsx:1` 21 tests (statusTone, APP_TYPE_ICONS, useDeploymentSteps, restartComposeStack, tab routing)
