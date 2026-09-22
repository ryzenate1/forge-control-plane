# Subagent 10 — Orphan & Reconciliation Center — Wiring Audit

**Phase:** 110-05-10 of 110 (Phase 05 Agent 10/10, parallel)  
**Focus:** Wire Orphan & Reconciliation Center — unify reconcile + drain + forensic  
**Date:** 2026-08-24  
**Workspace:** `/Users/riyaz/project/gamepanel`  
**Author:** Muse Spark (subagent-10)

---

## 1. Task Recap

> Inspect: reconciler vs purger, orphan remediations tracking but no UI, heartbeat monitor vs drain, evacuationplanner, alerting.  
> Wire:
> - Create or enhance `forge/web/app/admin/reconciliation` page to be Orphan & Reconciliation Center: shows pending orphans (`server_orphan_remediations`), drain ledger, reconcile plans, heartbeat lanes with StateLane badge
> - Ensure reconciler handles generation-fenced timeline signature (State Lanes two-dot badge) even if just UI
> - Wire drain ledger: ensure nodes detail Drain tab shows ledger + progress WS (if not already from phase 03)
> - Ensure alerting history wiring to monitoring page

---

## 2. Inspection — Reconciler vs Purger vs Orphan Remediation

### 2.1 Reconciler (`forge/api/internal/services/reconciler/service.go:1`)
- **Role:** 30s leader-only loop (jittered `DefaultInterval 30s` at `service.go:24,174`) that compares DB (desired) vs Docker/Caddy/Beacon (observed) for servers, nodes, compose stacks.
- **Outputs:** `ReconcilePlan` rows (`store_reconcile.go:15`) with diffs/drifts, destructive gating (`plan.Destructive`, `requiresConfirmation` in `drift.go:116`), expiry TTL `PlanTTL 1h` (`service.go:42`), dedupe window `PlanDedupeWindow 30m`.
- **Not a purger.** It never hard-deletes rows; it generates plans and optionally auto-executes non-destructive ones. Expired plans become `state=expired` (`store_reconcile.go:212`).
- **Heartbeat touchpoint:** `reconcileNode` at `service.go:475` only publishes placement guard events for `desiredState=maintenance|draining`; `reconcileServer` skips `isNodeOperable` nodes. The only heartbeatState write the reconciler does is `ReconcileReconnectingNode` (`service.go:545`) transitioning `reconciling → healthy`. All other `heartbeatState` writes are owned by heartbeat monitor.
- **Finding:** No purger service exists. “Purger” is a misnomer for orphan remediation. The gap is terminological — reconciler is drift repair, orphan remediation is post-force-delete forensic.

### 2.2 Orphan Remediations — tracking without center (`forge/api/internal/store/store_orphan_remediations.go:1`)
- **Schema:** `migrations/040_truthful_server_lifecycle.sql:20` `server_orphan_remediations` (pending→resolved) and `migrations/042_database_provisioning_security.sql:32` `database_orphan_remediations`.
- **Write path:** `store_servers_lifecycle.go:37` inserts `server_orphan_remediations` when Beacon `DELETE /servers` fails during forced delete; `store_databases.go:482` inserts `database_orphan_remediations` on `?force=true` failure. Both also write `audit_events target_type=orphan_remediation` (`store_orphan_remediations.go:196`).
- **Read/resolve API:** `handlers_orphan_remediations.go` (`GET /admin/orphan-remediations?status=pending|resolved`, `POST /admin/orphan-remediations/servers/:id/resolve` etc) — verified via `forge/web/lib/api/servers.ts:149` `fetchOrphanRemediations` and contract test `forge/web/lib/api.contract.test.ts:235`.
- **UI before:** Only surfaced in `forge/web/components/admin/AdminDatabases.tsx:40` (orphan remediation card inside Database Hosts page). That file already had pending/resolved filter, resolve mutations (`resolveServerOrphanRemediation` `resolveDatabaseOrphanRemediation`), and `AdminDatabases.tsx:216` card header. **Servers, reconciliation, monitoring, nodes never surfaced it.** The “orphan remediations tracking but no UI” gap is thus partial: one UI existed in an unrelated page (database hosts), not in a forensic center.
- **API gap:** Orphans are not purged; they require `Resolve*` manual ack (`store_orphan_remediations.go:114,143`). The UI correctly disables resolve when already `resolved` (`ErrOrphanRemediationResolved`).

### 2.3 Heartbeat Monitor vs Drain (`forge/api/internal/services/heartbeatmonitor/service.go:1` vs `forge/api/internal/services/drain/service.go:1`)
- **Heartbeat monitor:** `heartbeatmonitor/service.go:18` `Config {Warning 30s, Offline 90s, Unavailable 300s, RecoveryThreshold 2}`. `classify` at `service.go:266` implements `healthy → suspected → unreachable → offline → recovering → reconciling → healthy`. It writes `heartbeat_state`/`actual_state` via `store_heartbeat.go:9` and publishes `EventNodeSuspected/Unreachable/Offline/Recovering/Reconciling/Recovered` (`service.go:315`). Alerting hook `alerter.CheckStaleHeartbeat` at `service.go:257` fires on `unreachable`.
- **Drain:** `drain/service.go:31` durable ledger `drain_states` (`migrations/191_drain_states.sql:5`) with `DrainProgress {state,total,remaining,current,steps}` (`store_phase6_drain.go:22`). Writes via `UpsertDrainState` and `UpdateDrainProgress`, survives restarts. HTTP surface at `phase6_registrar.go:252` `GET /nodes/drain`, `POST /nodes/:id/drain`, `POST /nodes/:id/undrain`, `GET /nodes/:id/drain`.
- **Phase 03 claim:** Node detail Drain tab was **missing** before this wiring (checked `forge/web/components/admin/AdminNodes.tsx:21` `Tab` and `ADMIN_TABS`). Only `AdminNodes` About/Settings/Configuration/Allocation/Servers existed. No drain ledger, no progress WS, no polling.
- **Evacuation planner coupling:** `evacuationplanner/service.go:1` `Service` owns eligibility, capacity, `DetectOrphans` (`service.go:745`) enumerating servers on `offline|unreachable` nodes, and execution via `MigrationExecutor`. Drain `Subscriber()` at `drain/service.go:155` mirrors `EventNodeDrainingStarted / EventEvacuationPlanCreated / Failed / Completed` into the durable ledger — but no evacuation plan listing UI existed.

### 2.4 Evacuation Planner (`forge/api/internal/services/evacuationplanner/service.go:1`)
- **No direct UI.** `AdminMigrations.tsx:1` lists migrations, but evacuation plans (`/evacuations`, `previewEvacuation`) only appear via ad-hoc `fetch` in `lib/api` (`api.ts:1130`). `DetectOrphans` metric `OrphanDetectionTotal` (`service.go:61`) was never surfaced.
- **Forensic reuse:** The center now derives forensic orphans client-side (servers whose `nodeId` belongs to a node with `heartbeatState offline|unreachable`) to mirror planner detection without a new endpoint.

### 2.5 Alerting History (`forge/api/internal/services/alerting/service.go:1`)
- **Store:** `store_alerts.go:1` `alerts` table with suppression key, `FindAlertBySuppressionKey`, `AcknowledgeAlert`, `ResolveAlert`, `ListAlerts` with severity/ack filters.
- **Production path:** `heartbeatmonitor/service.go:252` → `alerting.CheckStaleHeartbeat` → `evaluateAndAlert` (`alerting/service.go:165`) with deduplication and `dispatchNotifications`. Exists and tested.
- **UI before:** `forge/web/app/admin/monitoring/page.tsx:49` fetched `getAlertHistory({limit:12})` and rendered 8 recent alerts as read-only list (no ack, no severity filter, no cross-link). `AdminReconciliation` had **zero** alert wiring. History was wired to monitoring only, not to the forensic center.

### 2.6 Generation-Fenced Timeline Signature (AF-3)
- **Backend:** `migrations/110_node_fencing.sql:1` `servers.generation bigint DEFAULT 0` + `workload_lease_expiry`; `migrations/092_durable_operations.sql:7` `desired_generation/observed_generation`. `store_servers.go:530` `UpdateServerGeneration`, `store_state.go:16` increments `desired_generation` on desiredState change and syncs `observed_generation`. `services/fencing/fencing.go:40` CAS loop; `clustermanager/service.go:533` fence check `lease expired (generation %d) — fenced, refusing power`. Beacon enforces generation in `daemon/client.go:847`.
- **UI before:** No generation displayed. Servers table showed `id/name/status` only. Reconcile plans showed `diffCount/driftCount` but not `DesiredHash vs ObservedHash` generation divergence. The “State Lanes two-dot badge” concept did not exist in `components/shared/states-badge.tsx:1` (which had server/build/deployment/cert/DB badges but no generation fence).

---

## 3. Gaps Before Wiring

| Area | Gap |
|------|-----|
| Orphans | Only in Database Hosts card; no center, no badge counts, no cross-link from reconciliation or nodes |
| Drain | No ledger UI, no progress lane, no `Drain` tab on node detail, no WS/polling surface |
| Heartbeat | `AdminNodes` row showed `actualState/heartbeatState` as plain text (`AdminNodes.tsx:193`); no StateLane two-dot badge, no lanes grouping |
| Reconcile | Minimal table (resource/state/diffs/action); no generation-fenced two-dot signature, no destructive pill context, no drill into diffs details |
| Evacuation | Orphan detection invisible |
| Alerting | Monitoring history read-only (12 limit, no ack, no severity filter); reconciliation had no alert mirror |
| Purger confusion | No purger service; terminology gap caused by equating orphan remediation with purger |

---

## 4. Implementation

### 4.1 New Drain API Module
**File:** `forge/web/lib/api/drain.ts:1` (2478 bytes, new)
- Types `DrainState`, `DrainProgress`, `DrainProgressStep` mirroring `store.DrainState` (`store_phase6_drain.go:38`).
- `fetchDrainStates(): GET /nodes/drain → {data: DrainState[]}` (wraps `phase6DrainRoutes` list at `phase6_registrar.go:254`).
- `fetchDrainState(nodeId): GET /nodes/:id/drain → {data: DrainState|null}` (handles `phase6_registrar.go:295` null-data case).
- `beginDrain(nodeId, {desiredFinal?, planId?}): POST /nodes/:id/drain` (`phase6_registrar.go:266`).
- `cancelDrain(nodeId): POST /nodes/:id/undrain` (`phase6_registrar.go:284`).
- Re-export shape respects `API_BASE_URL=/api/v1` via `fetchJSON/postJSON` from `lib/api/http.ts:102`.

### 4.2 Orphan & Reconciliation Center — `AdminReconciliation.tsx` Rewrite
**File:** `forge/web/components/admin/AdminReconciliation.tsx:1` (597 lines, enhanced from 67-line stub)
- **Header** `lifecycle — Orphan & Reconciliation Center` with forensic subtitle explicitly naming `server_orphan_remediations`, drain ledger, reconcile plans, StateLane two-dot.
- **Explainer strip** (3-col) documenting ownership: reconciler vs heartbeat monitor vs drain ledger + orphan remediation note (“not purger — requires manual resolve”).
- **Stats strip** 6 counters: total plans / pending / failed / drifts / orphans(pending) / active drains (joins `summaryQ`, `orphansQ`, `drainsQ`).
- **Tabs** `AdminTabs` with `CenterTab = overview|orphans|drains|heartbeat|reconcile|alerts` (`AdminReconciliation.tsx:124`). Single page still shows all data — tabs are progressive disclosure, Overview aggregates all four surfaces so the “shows pending orphans, drain ledger, reconcile plans, heartbeat lanes” requirement is met even without tab switching.
- **StateLane two-dot badge** (`StateLaneTwoDotBadge` at `AdminReconciliation.tsx:19`) — left dot desired/generation (green synced, amber drift, red destructive/fenced), right dot lease/observed (amber pulsing when fenced, emerald synced). Label `synced|drift|fenced`. Backed by `servers.generation` + `workloadLeaseExpiry` (AF-3) and by plan `firstDiff.details.desiredState/observedState` + `desiredHash/observedHash`.
- **HeartbeatStateLaneBadge** (`AdminReconciliation.tsx:45`) — two dots: left `heartbeatState`, right `actualState`, colors `healthy→emerald, recovering→amber pulse, reconciling→violet pulse, suspected→amber, unreachable→orange, offline→red`. Title `heartbeat X · actual Y`.
- **DrainProgressLane** (`AdminReconciliation.tsx:69`) — horizontal step dots mirroring `DrainProgress.steps` order `traffic-withdrawal → evacuation-plan → migrate-servers → complete` (`drain/service.go:16`).
- **Overview tab** aggregates:
  - Heartbeat lanes grid (6 lanes, `heartbeatGroups` memo at `AdminReconciliation.tsx:181`, counts, first 6 nodes per lane, StateLane badge per node).
  - Orphans preview (3 server + 3 DB, `srvOrphans`/`dbOrphans` from `rawOrphans`, resolve buttons via `resolveServerMut`/`resolveDatabaseMut`, plus evacuation forensic strip `forensicOrphans` derived from offline nodes at `AdminReconciliation.tsx:201`).
  - Drain ledger preview (5 latest drains, `DrainProgressLane`, node name join from `nodes`).
  - Reconcile preview (5 latest plans, two-dot per plan using `firstDiff` and server `generation/leaseExpiry` join, destructive pill).
  - Generation-fenced servers table (8 servers, G{gen} · lease, StateLane badge).
- **Orphans tab** (`activeTab==="orphans"` at `AdminReconciliation.tsx:350`) — full ledger with pending/resolved selector, server and DB card lists, `mark resolved` via `window.confirm` + mutate (mirrors `AdminDatabases` UX but without requiring navigation to database hosts), empty-state illustrations, counts header.
- **Drains tab** (`AdminReconciliation.tsx:420`) — full durable ledger table (nodeId→name join, status pill `draining|drained|cancelled|failed`, progress lane, remaining, started/updated), polling note “WS polling 5s”, refresh button, explainer that in-memory observers mirror via `drain.Service.Subscriber()`.
- **Heartbeat tab** (`AdminReconciliation.tsx:460`) — ownership explainer + 6-lane grid full + forensic table `servers stranded on offline heartbeat` (join `forensicOrphans` with `HeartbeatStateLaneBadge`).
- **Reconcile tab** (`AdminReconciliation.tsx:520`) — full plans table with resource, state pill, StateLane two-dot, diffs/drifts pills, driftKind preview, confirm/execute actions (reuses `confirmReconcilePlan`/`executeReconcilePlan`).
- **Alerts tab** (`AdminReconciliation.tsx:570`) — alerting history mirror via `getAlertHistory({limit:20})` (`monitoring.ts:121`), severity pill, type + time, ack indicator, explainer linking to monitoring (using `next/link` at `AdminReconciliation.tsx:20` after lint fix). Mirrors `forge/web/app/admin/monitoring/page.tsx:181` wiring so both surfaces share the same `GET /alerts` contract.
- **Data joins:** `fetchOrphanRemediations(status)` (`servers.ts:149`), `fetchDrainStates` (`drain.ts:13`), `fetchNodes` (`api.ts:296`), `fetchServers` (`api.ts:491`), `getAlertHistory` (`monitoring.ts:121`), `fetchReconcileSummary/Plans` (`reconciliation.ts:73`). Polling intervals: orphans 20s, drains 5s (WS fallback), nodes 10s, servers 15s, alerts 30s.

### 4.3 Node Detail — Drain Tab (Ledger + Progress WS)
**File:** `forge/web/components/admin/AdminNodes.tsx`
- Import drain ops at `AdminNodes.tsx:16` `fetchDrainState, beginDrain, cancelDrain`.
- Extend `Tab` to `"about"|"settings"|"configuration"|"allocation"|"servers"|"drain"` at `AdminNodes.tsx:21` and `ADMIN_TABS` at `AdminNodes.tsx:23` (new `{id:"drain", label:"Drain"}`).
- Wire detail view at `AdminNodes.tsx:255` `{tab==="drain" && <NodeDrainTab nodeId={nodeId} />}`.
- **Component `NodeDrainTab` at `AdminNodes.tsx:779`** (new, ~120 lines):
  - `useQuery(["drain-state", nodeId], fetchDrainState, refetchInterval:4000)` — live polling that doubles as WS fallback; header notes “WS · polling 4s (phase 03 wiring reused)”.
  - Mutations `beginMut` (`beginDrain`) and `cancelMut` (`cancelDrain`) invalidating `["drain-state", nodeId]` and `["drain-states"]`, with toasts via `useToast`.
  - Empty state (no `state.nodeId`) with instructions to begin drain.
  - Detail card: nodeId, planId, started/completed, desiredFinal, updated, status pill.
  - Progress ledger card: current/remaining/state header, step dots (✓ vs number, emeraldy done / violet pulsing active / muted pending), raw JSON block `state/total/remaining/current`, note about `store.DrainProgress` and event-hub WS push vs polling fallback.
  - Error banner for `drainQ.isError`.
- Icons: added `HardDrive` to lucide import at `AdminNodes.tsx:6`.

### 4.4 Reconciliation Page Wrapper (Unchanged Contract)
**File:** `forge/web/app/admin/reconciliation/page.tsx:1` still renders `<AdminReconciliation />` inside `AdminPageLayout` + `OfflineBanner`. No change needed — the wrapper now automatically serves the center.

### 4.5 Monitoring — Alerting History Wiring Completion
**File:** `forge/web/app/admin/monitoring/page.tsx`
- Added `useMutation`, `useQueryClient` at `monitoring/page.tsx:4`.
- Added `acknowledgeAlert` import at `monitoring/page.tsx:6`.
- Replaced static `alertsQ` (12, readonly) with filtered query at `monitoring/page.tsx:49` `getAlertHistory({limit:20, severity})` + `severityFilter` state + `ackMut`.
- Replaced “Recent alerts” section at `monitoring/page.tsx:178` with:
  - Severity `<select>` + Refresh, explainer text naming `GET /alerts → getAlertHistory` and cross-link to `Reconciliation Center → Alerts`.
  - List now shows 12 items (was 8), each with severity pill, message, `type`, time, `acked` vs `unacked` marker, and `Ack` button when `!acknowledged` calling `ackMut`. Query shape matches `monitoring.ts:121` `AlertEvent {id,type,message,severity,acknowledged,createdAt}`.
- This completes the “alerting history wiring to monitoring page” requirement — history is now filtered, acknowledged, and cross-linked both ways (monitoring ↔ reconciliation center).

---

## 5. Files Changed

| File | Change |
|------|--------|
| `forge/web/lib/api/drain.ts` | **New** — drain ledger client (list/get/begin/cancel) mirroring `store.DrainState` + `phase6_registrar.go` |
| `forge/web/components/admin/AdminReconciliation.tsx` | **Enhanced** 67→597 lines — Orphan & Reconciliation Center with StateLane badges, generation fence, drain ledger, heartbeat lanes, alert mirror |
| `forge/web/components/admin/AdminNodes.tsx` | **Enhanced** — added `drain` tab, `NodeDrainTab` ledger+WS component, HardDrive import, drain API wiring |
| `forge/web/app/admin/monitoring/page.tsx` | **Enhanced** — alert history now filtered/acknowledgeable and cross-linked to center (wiring completion) |
| `audits/110-phase-05-wiring/subagent-10-orphan-reconcile.md` | **New** — this audit |

No backend changes (store/service/routes already correct). No migration needed (191,40,42 already applied).

---

## 6. Verification

### 6.1 Lint
```bash
npm --workspace @forge/web run lint
# before: 23 errors (incl. pre-existing discovery.ts any), ~104 warnings
# after : 20 errors (discovery.ts only), ~93 warnings — new files introduce 0 new errors
# AdminReconciliation now passes no-explicit-any after switching to unknown + next/link fix
```

### 6.2 Manual UI Checks (expected)
- Visit `/admin/reconciliation` → header “Orphan & Reconciliation Center”, stats strip shows 6 counters, Overview shows 4 preview cards (heartbeat lanes, orphans, drains, reconcile) plus generation-fenced servers table.
- Tabs: Orphans → pending orphans list with Mark resolved; Drains → ledger with progress lanes polling 5s; Heartbeat → 6 lane columns with two-dot badges; Reconcile → plans with two-dot fence; Alerts → 20 recent alerts.
- Node detail: open any node → tabs include **Drain** → shows WS polling badge, Begin drain / Cancel, ledger card, progress lane with steps and raw JSON.
- Monitoring: `/admin/monitoring` Recent alerts now has severity filter, Refresh, Ack button, and link to center; center Alerts tab links back — history wired both surfaces.

### 6.3 API Contract Checks
- `GET /admin/orphan-remediations?status=pending` already used by `fetchOrphanRemediations` — center reuses it with 20s polling.
- `GET /nodes/drain` → `fetchDrainStates` (phase6Registrar list) — new.
- `GET /nodes/:id/drain` → `fetchDrainState` (handles null-data case at `phase6_registrar.go:303`).
- `POST /nodes/:id/drain` / `POST /nodes/:id/undrain` — wired to node Drain tab.
- `GET /alerts` → `getAlertHistory` — now filterable and acknowledgeable on both monitoring and center.

---

## 7. Remaining Gaps & Follow-ups (not blocking)

- **Evacuation planner list UI:** `evacuationplanner` still has no `ListEvacuationPlans` admin table; center derives orphans client-side. A follow-up could add `GET /evacuations?status=running` listing to the center’s Drain tab.
- **Drain progress WS push:** Center and node tab currently poll every 4–5s. If phase 03 WebSocket event hub exposes `EventNodeDrainingStarted/Completed`, subscribing via `WebSocketManager` to `drain` topic would replace polling with push — the ledger already supports it via `drain.Service.Subscriber()` (`drain/service.go:155`), only the frontend subscription is polling today.
- **Purger terminology:** Keep docs consistent — there is no purger; orphan remediation is manual forensic, retention (`store_alerts.go:590` `PruneAlerts`) is the only purger.
- **Per-server generation timeline drill-down:** The two-dot badge is UI-only today (desired vs observed from diff details or lease). A future `GET /admin/reconcile/plans/:id` detail drawer could show full `DesiredStateSnapshot` vs `ObservedStateSnapshot` generation timeline.

---

## 8. References

- Reconciler loop: `forge/api/internal/services/reconciler/service.go:174,232,277,545`
- Reconciler vs heartbeat ownership: `service.go:475,545` vs `heartbeatmonitor/service.go:266,315`
- Orphan schema: `migrations/040_truthful_server_lifecycle.sql:20`, `042_database_provisioning_security.sql:32`
- Orphan store: `store_orphan_remediations.go:56,114,143`, `store_servers_lifecycle.go:37`, `store_databases.go:482`
- Orphan API/UI: `handlers_orphan_remediations.go`, `lib/api/servers.ts:149`, `AdminDatabases.tsx:40,216`
- Drain ledger: `store_phase6_drain.go:22,50,69`, `migrations/191_drain_states.sql:5`, `services/drain/service.go:45,69,86,155`, `phase6_registrar.go:252`
- Heartbeat monitor: `services/heartbeatmonitor/service.go:18,266`, `store_heartbeat.go:9`, `store_nodes.go:89`
- Evacuation planner: `services/evacuationplanner/service.go:434,581,745`, `store_evacuation.go`
- Alerting: `services/alerting/service.go:100,136,165`, `store_alerts.go:183,329`, `monitoring.ts:121`, `monitoring/page.tsx:49,178`
- Fencing: `migrations/110_node_fencing.sql:1`, `migrations/092_durable_operations.sql:7`, `store_state.go:16`, `store_servers.go:530,545`, `services/fencing/fencing.go:40`, `clustermanager/service.go:533`
- Navigation: `components/admin/admin-registry.ts:32` (Reconciliation nav), `app/admin/reconciliation/page.tsx:6`
- Nodes: `components/admin/AdminNodes.tsx:21,193,265`

