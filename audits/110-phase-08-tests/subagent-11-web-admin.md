# Subagent 11 — Web Admin Pages Test Files (Phase 08)

**Focus:** `overview`, `monitoring`, `health`, `host`, `servers` — `forge/web/app/admin/*`, `components/admin/AdminOverview.tsx`, `monitoring/page.tsx`, `host/page.tsx`, `servers/page.tsx`

**Date:** 2026-08-24

## Inspection

### Admin pages inspected
- `forge/web/app/admin/` — 68 entries (activity, allocations, ... overview, health, host, monitoring, servers, etc.) `forge/web/app/admin:68`
- `forge/web/components/admin/AdminOverview.tsx:240` — `AdminOverview()` with 5 queries (`fetchAllNodes`, `fetchAllServers`, `fetchUsers`, `fetchHealthStatus`, `fetchAdminAudit`), `overall` critical/degraded/operational derivation, `isAbortError`/`abortAwareRetry`, `attentionCount`, `formatMiB`, capacity aggregation, collapsible attention section.
- `forge/web/app/admin/monitoring/page.tsx:235` — `AdminMonitoring` with `isSynthetic` (`cpuLoad1m===0 && networkRxBytes===0` for all points), `syntheticNoDataMetric = isSynthetic && metric==="networkRxBytes"`, period/metric/nodeId controls, Recharts `AreaChart`, `metricColor` tokenized palette, alert history via `getAlertHistory`/`acknowledgeAlert`.
- `forge/web/app/admin/health/page.tsx:6` — thin wrapper re-exporting `AdminHealth`
- `forge/web/components/admin/AdminHealth.tsx:250` — `AdminHealth` with `fetchHealthStatus`/`fetchNodes`/`fetchServers`/`fetchReservations`/`fetchRecoveryPlans`, `failed`/`warn` filtering, `remediation()` per check name, `tone()`, `MetricTile`, infrastructure/DB/cache/control-plane/workload sections, refresh.
- `forge/web/app/admin/host/page.tsx:252` — `AdminHost` with `useHostQuery` (15s AbortSignal.timeout + placeholderData), 5 tabs `info/disk/memory/network/processes`, `NodeSelect`, `OfflineBanner`, per-tab fetchers `fetchHostInfo`/`fetchHostDisk`/`fetchHostMemory`/`fetchHostNetwork`/`fetchHostProcesses`.
- `forge/web/app/admin/servers/page.tsx:7` — re-export of `AdminServers`
- `forge/web/components/admin/AdminServers.tsx:809+` — `AdminServers` list + `CreateServerModal` (validation for allocation/node binding), detail modal with 8 tabs, `GenerationFencedDots`, `statusTone`.

### Existing tests (pre-task)
```sh
ls forge/web/test/*.test.tsx | head -n 20
# /Users/riyaz/project/gamepanel/forge/web/test/app-ux-18.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/auth-account.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/console-health.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/lifecycle.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/permission-gates.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/server-detail-layout.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/test/ui-contracts.test.tsx
# (7 files)

ls forge/web/components/**/*.test.tsx
# /Users/riyaz/project/gamepanel/forge/web/components/server/server-views.test.tsx
```

No `admin-overview.test.tsx` existed.

## Created / Augmented

**File:** `forge/web/test/admin-overview.test.tsx` (22 tests)

Mocks:
- `next/link` → `<a>`
- `recharts` → lightweight stubs (`ResponsiveContainer`, `AreaChart`, etc.) — avoids `ResizeObserver`/SVG measurement in jsdom
- `vi.stubGlobal("fetch", ...)` via `installFetch` routing by substring (metrics before generic `/nodes`), with `Response.clone()` handling for polling/re-fetch reuse (body can only be read once) and factory `() => jsonResponse(...)` for metrics/summary/alerts

Coverage:

**1. `AdminOverview — mocked API` (5 tests)**
- Operational overview with healthy nodes/running servers/users/health snapshot → asserts `All systems operational`, `Infrastructure`, `2 healthy`, `2 running`, calm attention, audit `server create`, `Capacity`.
- Critical attention when offline nodes + failed health checks → `nodes offline`, `Node heartbeat failure`, affected node `beta`, failed `Database`.
- Degraded via warning checks + degraded heartbeat → `Platform degraded`, `1 degraded`.
- Empty audit recent activity → `No recent changes.`
- Toggle `Needs attention` Hide/Show via button click.

**2. `AdminHealth — health checks` (5 tests)**
- Operational state meta `All systems operational`, `2 nodes`.
- Failures with remediation (`Check DB credentials …`), offline count, crashed workload.
- Warnings section for `memory` warning + degraded node.
- Empty-state `No failures`, heartbeat table `1/1 healthy`, `alpha`, `View` link.
- Detailed metric tiles `Database & cache`, `Latency`, `Control plane`, `Goroutines` via check details.

**3. `AdminMonitoring — isSynthetic` (5 tests)**
- Synthetic banner `allocated capacity` and `No data — network/load not yet collected by Beacon` when `cpuLoad1m===0 && networkRxBytes===0` for all points and metric=`networkRxBytes`; CPU metric does not show network No data.
- Non-synthetic live metrics (1.2 cpuLoad, 1024 bytes) → no synthetic banner, chart header `CPU — 5m trend`.
- Empty metrics → `No telemetry available`.
- Period switch `1h` retains synthetic semantics.
- Pure `isSynthetic` predicate unit test (empty→false, all-zero→true, mixed→false).

**4. `AdminHost page` (3 tests)**
- Default `Host` + `System` tab `Hostname` `host-1`.
- Tab switching Storage/Disk → `/`, Memory → `8.0 GB`+`50.0%`, Network → `eth0`, Processes → `init`.
- No-nodes empty state `No nodes`.

**5. `AdminServers page` (4 tests)**
- List with `alpha-mc`/`beta-mc`, `Servers` header, `State Lanes`.
- Search filter `beta` hides `alpha-mc`.
- Empty `No servers`.
- Create modal `Create Server` dialog, typing `My Game Server` input.

## Verification

```sh
npx tsc --noEmit --project forge/web/tsconfig.json
# (no output) exit code 0

npm --workspace @forge/web run test -- test/admin-overview.test.tsx
# ✓ test/admin-overview.test.tsx (22 tests) 703ms
# Test Files 1 passed (1)
# Tests 22 passed (22)

npm --workspace @forge/web run test
# Before: Test Files 1 failed | 19 passed (20), Tests 1 failed | 229 passed (230) — failure pre-existing in middleware.test.ts
# After (with parallel agents' additions): Test Files 1 failed | 23 passed (24), Tests 1 failed | 347 passed (348)
# The sole failure is middleware.test.ts > forwards the cookie header to the validation request (expected '__Host-forge_session=abc; other=1' got '__Host-forge_session=abc') — pre-existing, unrelated to admin pages.
```

- Polling `refetchInterval` edge: handled via `retry:false, gcTime:0` in `createTestQueryClient()` and fresh `Response` per fetch (factory/clone) so metric/period switches (new `queryKey`) do not hit "body already used".
- Tokenized palette `metricColor` and abort-aware retry `isAbortError` are exercised indirectly via the mocked fetch layer.

## Files

- Created: `forge/web/test/admin-overview.test.tsx:579`
- Inspected: `forge/web/app/admin/monitoring/page.tsx:235`, `forge/web/app/admin/host/page.tsx:252`, `forge/web/app/admin/health/page.tsx:6`, `forge/web/components/admin/AdminOverview.tsx:240`, `forge/web/components/admin/AdminHealth.tsx:250`, `forge/web/components/admin/AdminServers.tsx`, `forge/web/lib/api/monitoring.ts:135`, `forge/web/lib/api/host.ts:68`, `forge/web/test/render.tsx:104`, `forge/web/test/setup.ts:37`, `forge/web/test/fetch-mock.ts:108`

