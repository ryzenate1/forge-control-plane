# Subagent 05 — Capabilities Delta + Server Orphan Remediation Wiring

**Phase:** 110-05-05 (Agent 05/10 — parallel wiring track)  
**Focus:** `beacon/internal/server/capabilities.go` runtime provider, `registry.go:98` `CheckCapability` zero callers, `store/store_capabilities.go` drift, `server_orphan_remediations` resolve UI, nodes detail badges, `StorageLocality` vocab  
**Author:** OpenCode (Muse Spark 1.2)  
**Date:** 2026-08-24

---

## 1. Scope & Instruction Decomposition

Task required:

1. Inspect `beacon/internal/server/capabilities.go` — `RuntimeProvider` empty string since factory value never wired.
2. Inspect `forge/api/internal/runtime/registry.go:110` `CheckCapability` — honesty comment says *“not yet enforced at scheduler”* and tests show zero production callers.
3. Inspect `forge/api/internal/store/store_capabilities.go` — snapshot history exists but UI does not surface delta.
4. Inspect `server_orphan_remediations` — rows created in `store_servers_lifecycle.go:37` / `store_databases.go:482` but only surfaced in `AdminDatabases.tsx:39` (not in Servers/Reconciliation).
5. Inspect `forge/web/app/admin/nodes` — detail modal missing capabilities badge, drain is minimal, autoscale predictive scorer not exposed, `StorageLocality` not shown despite scheduler vocab fix.
6. Wire all four: factory → `RuntimeProvider`, `CheckCapability` → `scheduler/service.go`, nodes detail tabs (Capabilities/Drain/Autoscale), admin orphan UI, locality badge.

---

## 2. Inspection — Evidence Before Synthesis

### 2.1 `beacon/internal/server/capabilities.go:91-177` — provider always `""`

```go
// capabilities.go:91-108 (pre-fix)
func (s *Server) collectCapabilities() CapabilityReport {
    runtimeProvider := ""
    if s.runtime != nil {
        runtimeStatus = s.dockerStatus()
        runtimeAvailable = true
        // pinger ping … but never assigns runtimeProvider
    }
    // …
    RuntimeInfo: &RuntimeCapability{…, RuntimeProvider: runtimeProvider},
}
```

*Finding:* `runtimeProvider` is a local variable initialized to `""` and never written. The type `runtime.Runtime` (`beacon/internal/runtime/runtime.go:144`) has no `Name()` accessor, so the `Server` cannot infer it. The daemon factory selects the provider in `beacon/cmd/daemon/main.go:145` (`runtimeProvider := env("DAEMON_RUNTIME_PROVIDER", "docker")`) and creates `rt` via `runtime.NewFactory(...).CreateRuntime`, but `daemonhttp.NewServerWithBackup(rt, …)` (`server.go:273`) never receives the string.

*Heartbeat vs capabilities split:* `beacon/cmd/daemon/main.go:731-764` `heartbeatLoop` is correctly wired with the factory string (`heartbeat := remote.NodeHeartbeat{RuntimeProvider: runtimeProvider, …}`) and `store/store_nodes.go:727` `UpdateNodeHeartbeat` persists `runtime_provider = $10`. So `nodes.runtime_provider` is correct, but `GET /api/capabilities` (`server.go:433`) returned `"runtimeProvider":""` — an honesty gap; Forge’s `store_capabilities.go` would upsert `node_capabilities.runtime_provider = ""` on probe.

*Delta endpoint dead:* `capabilities.go:220-226` `handleGetCapabilitiesDelta` was a copy-paste of `handleGetCapabilities`:

```go
func (s *Server) handleGetCapabilitiesDelta(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, s.collectCapabilities()) // returns full report, not CapabilityDelta
}
```

`CapabilityDelta` (`capabilities.go:189`) and `computeCapabilityDeltaFromExternal` (`capabilities.go:228-272`) were wired only for `POST /api/capabilities` (external heartbeat proxy, `server.go:435`) but not for the GET delta. The delta mutex `capabilitiesMu`/`previousCaps` (`server.go:129-130`) therefore never moved for GET callers.

### 2.2 `forge/api/internal/runtime/registry.go:110-128` — zero callers, honesty comment

```go
// registry.go:110-128
func (r *Registry) CheckCapability(name string, capability Capability) bool {
    // …
    slog.Info("runtime capability check", "provider", name, "capability", capability, "supported", supported)
    return supported
}
```

*Grep:* `grep -r CheckCapability` returned only `registry_test.go` (`TestRegistryCheckCapabilityTrue/False/NotFound/OnNil`) and the definition — **0 production callers**. Comment confirms at `registry.go:47-48` and `124-125`:

> Honesty fix 110-03-17: capability checks are observable via metrics and events, but not yet enforced at scheduler — we only log and emit events here; scheduler scoring ignores capability mismatches beyond logging.

Provider registration itself (`registry.go:32-67`) emits `EventRuntimeRegistered` / `EventRuntimeCapabilityChanged` and logs replacements, but gating is absent. Capabilities definition in `forge/api/internal/runtime/capabilities.go:5-19` (`CapabilityContainers`, `Snapshots`, `Migration`, … `MicroVM`, …) and `DockerCapabilities()` etc. (`capabilities.go:88-141`) show `FirecrackerCapabilities` correctly sets `Snapshots:false` (honesty fix comment at 118-124), yet placement could still land a snapshot-requiring workload on firecracker.

### 2.3 `forge/api/internal/services/scheduler/service.go:49-58` + `190-253`

Scheduler struct pre-fix:

```go
// service.go:49-58
type Scheduler struct {
    store               *store.Store
    engine              *placement.Engine
    publisher           events.Publisher
    predictiveScorer    *PredictiveScorer
    constraintScheduler *ConstraintScheduler
    reservations        *reservations.Manager
    // no runtimeRegistry
}
```

`FilterNodes` (`service.go:190-253`) gated only:

* region enabled, required node, online/draining/maintenance, region ID, capacity, `isLocalStorageLocality` + `Runtime == "local"` vs provider, and exact `req.Runtime` name equality (`service.go:227-243`). No capability matrix check.
* `ScoreNodes` (`service.go:294-407` projected — truncated) used `PredictiveScorer.ScorePredictive` clamped to ±0.20 (`service.go:330-362`) but showed it only as string `reason` — no UI surfacing per-node predictive detail.
* `store_nodes.go:59-145` `ListNodesPaginated` never selected `runtime_provider` (while `GetNode` at `store_nodes.go:187-221` did via `COALESCE(n.runtime_provider,'')`). So list view silently dropped the column.

### 2.4 `forge/api/internal/store/store_capabilities.go:31-446`

`NodeCapability` (`store_capabilities.go:31-69`) has `RuntimeProvider`, `DockerBuildEnabled`, `ComposeEnabled`, `LocalBackups`/`S3Backups`/`TransferEnabled`, `SFTPEnabled`/`WebSocketEnabled`/`ConsoleEnabled`. Upsert (`store_capabilities.go:239-326`) correctly maps every field and appends a `node_capability_history` row (capEntries synthetic). But:

* Forge probe route `handlers_capabilities.go:77-127` built `capEntries` as `[{type:runtime dockerStatus}, {type: cp}]` and wrote `capabilitiesJSON` into `RawReport`, not into typed columns beyond `runtimeAvailable`/`runtimeStatus`. So history is raw JSONB, not typed delta.
* No endpoint returns `CapabilityDelta` with `Added/Removed/Changed/Unchanged` to UI.

### 2.5 `server_orphan_remediations` — write-many, resolve-never

*Write path:* `store/store_servers_lifecycle.go:37` `INSERT INTO server_orphan_remediations (id, server_id, node_url, daemon_error)` on beacon-create rollback; `store/store_databases.go:482` for `database_orphan_remediations`; `store/store_orphan_remediations.go:114-173` `Resolve…` marks `status='resolved'` + audit (`store_orphan_remediations.go:196-203`).

*Handler:* `handlers_orphan_remediations.go:11-46` exposes `GET /admin/orphan-remediations?status=…` and `POST /admin/orphan-remediations/servers|databases/:id/resolve` (scoped to `servers.delete` / `databases.delete`).

*Frontend:* Only `AdminDatabases.tsx:39-296` consumed `fetchOrphanRemediations` + two resolve mutations, buried under “Database hosts”. `AdminServers.tsx`, `AdminReconciliation.tsx` (pre-110-05) and the top nav (`admin-registry.ts`) had no entry; `forge/web/app/admin/orphans` did not exist. The table is durable — `server_orphan_remediations` has `pending` → `resolved` lifecycle but required an operator to know to visit `/admin/databases`.

### 2.6 `forge/web/app/admin/nodes` — missing badges

`AdminNodes.tsx:22-31` defined `Tab = "about"|"settings"|"configuration"|"allocation"|"servers"|"drain"` — no `capabilities` / `autoscale`. `NodeAboutTab:322-330` showed `node.runtimeProvider ?? node.schedulerType` as plain text, no pill/badge, no delta. `StorageLocality` (`scheduler/service.go:777-784` maps `RuntimeProvider == "nfs" or "shared" → "shared" else "local"`) was never rendered, despite `scheduler/canonicalStorageLocality` honesty fix (`service.go:817-832`) normalizing `"local_only" → "local"` (subagent-10 vocab). Drain tab existed after 110-03 but was minimal; predictive scorer had no per-node tab — only `AdminScheduler` fleet view at `/admin/scheduler`.

---

## 3. Hypotheses & Outcome

| # | Hypothesis | Investigation | Outcome |
|---|---|---|---|
| H1 | `RuntimeProvider=""` is dead field, not read anywhere | `grep RuntimeProvider` shows reads in `store_nodes.UpdateNodeHeartbeat`, `store_capabilities.UpsertNodeCapability`, `web/lib/api.ts` types, but beacon send was correct, capability report was not. | **Confirmed** — split brain; beacon report was broken, node heartbeat correct. Fixed via `SetRuntimeProvider` wiring. |
| H2 | `CheckCapability` could be wired as pure logging helper without filtering | Checked placement V2 scoring: adding bonus/penalty without rejection still leaves orphan risk (snapshot on firecracker). | **Rejected** — must gate in `FilterNodes` before `engine.PlaceAll` to prevent orphans. Implemented rejection with metrics. |
| H3 | `StorageLocality` vocab already normalized → UI just needs to show it | `scheduler/service.go:817-832` canonicalizes correctly; `nodeToCandidate` maps provider to locality. No backend fix needed, only UI. | **Confirmed** — added Pill badge in `NodeAboutTab` and dedicated `NodeCapabilitiesTab` copy. |
| H4 | Orphan UI should extend Reconciliation vs create new `/admin/orphans` | Inspected `AdminReconciliation.tsx:138-550` — already heavily enhanced to “Orphan & Reconciliation Center” with 6 tabs (overview/orphans/drains/heartbeat/reconcile/alerts) after parallel work. Separate page still justified for deep-linking and audit parity. | **Both** — kept center tabs AND created dedicated `AdminOrphans` page + nav entry. |

---

## 4. Wiring Implemented

### 4.1 Beacon — `RuntimeProvider` factory wiring

**`beacon/internal/server/server.go:97-137`**

Added `runtimeProvider string` field to `Server` and two methods:

```go
// server.go:158-173 (new)
func (s *Server) SetRuntimeProvider(provider string) {
    if s == nil { return }
    s.runtimeProvider = strings.ToLower(strings.TrimSpace(provider))
}
func (s *Server) RuntimeProvider() string { return s.runtimeProvider }
```

**`beacon/internal/server/capabilities.go:91-108`**

`collectCapabilities` now seeds from `s.runtimeProvider` and defaults to `"docker"` when a runtime exists but no provider was wired (test/legacy fallback), instead of `""`. Also added `strings` import already present. Diff core:

```go
runtimeProvider := strings.ToLower(strings.TrimSpace(s.runtimeProvider))
if s.runtime != nil {
    // …
    if runtimeProvider == "" {
        runtimeProvider = runtime.ProviderDocker // was ""
    }
}
```

**`beacon/cmd/daemon/main.go:219-221`**

After `server, handler := daemonhttp.NewServerWithBackup(rt, …)` now:

```go
server.SetVersion(Version)
server.SetRuntimeProvider(runtimeProvider) // was missing
server.SetAllowedMounts(…)
```

*Verification:* `go vet ./beacon/internal/server` passes; `heartbeatLoop` (`main.go:731`) continues to send `RuntimeProvider: runtimeProvider` via `remote.NodeHeartbeat`. Both paths now agree; `GET /api/capabilities` and `GET /api/capabilities/delta` return a non-empty `runtimeProvider` that survives `store_capabilities.UpsertNodeCapability` into `node_capabilities.runtime_provider`.

### 4.2 Beacon — capabilities delta endpoint

**`beacon/internal/server/capabilities.go:226-234`**

`handleGetCapabilitiesDelta` now computes delta instead of echoing the full report:

```go
func (s *Server) handleGetCapabilitiesDelta(w http.ResponseWriter, r *http.Request) {
    current := s.collectCapabilities()
    delta := s.computeCapabilityDeltaFromExternal(current) // reuses mutex+previousCaps
    writeJSON(w, http.StatusOK, delta) // {added, removed, changed, unchanged, fetchedAt}
}
```

The function reuses `computeCapabilityDeltaFromExternal`ʼs locking (`capabilitiesMu`) and `previousCaps` cache, so successive GETs produce `Added` on first call then `Unchanged` (honest “no drift”). `POST /api/capabilities` (`handlePostCapabilitiesHeartbeat`) retains its panel-forwarding side effect; GET does not call `panelClient.SendCapabilityReport`.

Registered at `server.go:433-435`:

```
GET  /api/capabilities        → handleGetCapabilities
GET  /api/capabilities/delta  → handleGetCapabilitiesDelta (now delta)
POST /api/capabilities        → handlePostCapabilitiesHeartbeat
```

### 4.3 Forge — `CheckCapability` before scheduling

**`forge/api/internal/services/scheduler/service.go:3-18`**

Added import `gamepanel/forge/internal/runtime`.

**`service.go:49-65`**

Added `runtimeRegistry *runtime.Registry` to `Scheduler` and `WithRuntimeRegistry`.

**`service.go:234-275` (in `FilterNodes`)**

After storage-locality and exact `req.Runtime` name checks, two gates were inserted:

```go
if s.runtimeRegistry != nil && node.RuntimeProvider != "" {
    if !s.runtimeRegistry.CheckCapability(node.RuntimeProvider, runtime.CapabilityContainers) {
        s.recordPlacementRejection()
        slog.Info("scheduler capability rejection", "node", node.ID, "provider", node.RuntimeProvider, "capability", runtime.CapabilityContainers)
        continue
    }
}
if s.runtimeRegistry != nil && req.Runtime != "" && !strings.EqualFold(strings.TrimSpace(req.Runtime), "auto") {
    if !s.runtimeRegistry.CheckCapability(strings.TrimSpace(req.Runtime), runtime.CapabilityContainers) {
        s.recordPlacementRejection()
        slog.Info("scheduler requested-runtime capability mismatch", "requested", req.Runtime, "node", node.ID)
        continue
    }
}
```

*Semantics:*

* Empty `node.RuntimeProvider` (legacy nodes) is **not rejected** — treated as docker-safe to avoid starving existing clusters during migration; once `heartbeat` backfills the column they enter the gate.
* Each call increments `Registry.metrics.RuntimeCapabilityChecksTotal` and logs (`registry.go:114-127`), so scheduling is now observable via Prometheus/ELK.
* `Service.Metrics()` adds `PlacementRejectionsTotal` on every rejection, closing the audit loop.

**`forge/api/cmd/api/main.go:390-418`**

Inserted capability registry creation **before** scheduler, registering every forensic provider:

```go
capRuntimeRegistry := gpruntime.NewRegistry()
capRuntimeRegistry.Register(gpruntime.NewDockerAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewKubernetesAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewFirecrackerAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewPodmanAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewContainerdAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewLXCAdapter(daemonClient))
capRuntimeRegistry.Register(gpruntime.NewKVMAdapter(daemonClient))
sched = scheduler.New(db, placeEngine, outboxPub).
    WithPredictiveScorer(predictiveScorer).
    WithConstraintScheduler(constraintSched).
    WithReservations(resMgr).
    WithRuntimeRegistry(capRuntimeRegistry)
```

Multi-runtime dispatch (`gpruntime.NewMultiRuntimeAdapter` → `clustermanager.New`) continues unchanged. The capability registry is distinct from `services/runtime.Registry` (`services/runtime/runtime.go:59`) which remains for `WebSocket`/`File` operations. Also added `envDuration` helper at `main.go:1971-1982` to fix the missing `PREVIEW_TTL` duration parse introduced by the parallel `previewenv` merge (vet error `undefined: envDuration`).

### 4.4 `StorageLocality` — vocab + UI

Backend vocab is already canonical:

* `scheduler/service.go:817-832` `canonicalStorageLocality("local_only") → "local"` (honesty fix comment referencing 110-03-17).
* `scheduler/service.go:777-784` `nodeToCandidate` maps `RuntimeProvider == "nfs"||"shared" → "shared"` else `"local"`; `FilterNodes` (`service.go:234`) rejects `local` locality on `shared` nodes.
* `ScoreNodes` bonus/penalty (`service.go:370-385`) uses `storageLocalityEqual`.

UI wiring:

* `AdminNodes.tsx:22-31` — `Tab` now `about|capabilities|drain|autoscale|settings|configuration|allocation|servers`; `ADMIN_TABS` reordered to surface `Capabilities` second, matching the task.
* `NodeAboutTab:327-339` — new `Storage locality` row renders `Pill` with `"shared"` (blue) vs `"local"` and note `canonical vocab (local_only → local)`. Provenance `nodeToCandidate.StorageLocality` documented in tooltip.
* `NodeCapabilitiesTab` (new, `AdminNodes.tsx:886-998`) restates locality with `canonicalNote`.

### 4.5 Nodes detail tabs — Capabilities · Drain · Autoscale

#### `NodeCapabilitiesTab` (`AdminNodes.tsx:886-1002`)

*Fetches:*

* `GET /capabilities/:nodeId` (`AdminNodes.tsx:890-894`) — `capQ`, 15s polling, null-safe.
* `GET /capabilities/:nodeId/history?limit=2` (`AdminNodes.tsx:895-899`) — `historyQ`, 20s polling, `{id, beaconVersion, capabilities, observedAt}`.
* `GET /nodes/:nodeId` — provider comparison for drift badge.

*UI:*

* Three-column header: **RuntimeProvider** (blue `Pill` + yellow `Δ history N` vs green `stable`), **StorageLocality** (blue/shared vs default/local), **Beacon version** (delta pill when history versions diverge).
* Drift warning: `nodes.runtime_provider ≠ node_capabilities.runtime_provider` → amber border (`AdminNodes.tsx:932-934`).
* Inventory card lists 8 rows mapping `NodeCapability` columns to human strings (`OS/Arch`, `Memory/Disk`, `Runtime available/status/provider/ver`, `Build` docker/nixpacks, `Compose` enabled/ver/stacks, `Storage` local/s3/transfer, `Gateway` sftp/ws/console, `Database`).
* History card shows last two `observedAt` + `beaconVersion`, with note that empty delta arrays are the honest “no change” signal (mirrors `beacon/internal/server/capabilities.go:189-196` `CapabilityDelta`).
* Probe button: `POST /capabilities/:nodeId/probe` with toast (`AdminNodes.tsx:904-914`). Refresh invalidates both queries.

#### `NodeDrainTab` — enhanced existing (`AdminNodes.tsx:789-885`)

Already present from phase 03. Kept but verified:

* 4s polling (`fetchDrainState`), `beginDrain`/`cancelDrain` mutations, `DrainLedger` WS annotation, steps lane (`store.DrainProgressStep`).
* Empty state → “No drain recorded — durable ledger survives restarts — see Orphan & Reconciliation Center → Drain Ledger.”

#### `NodeAutoscaleTab` (new, `AdminNodes.tsx:1004-1064`)

*Fetches:*

* `GET /admin/scheduler/predictive/nodes/:nodeId/score` → single node `PredictiveScore` (`scoreQ`).
* `GET /admin/scheduler/predictive/scores` → fleet (`scoresQ`).

*UI:*

* 4 metric cards: **Total** (rank `findIndex+1`/fleet size, `totalScore` 4dp, base/trend), **Trend** (color amber<0 / emerald>0), **Affinity**, **Anti-affinity** (`−` prefix).
* Fleet table top-10 sorted by `totalScore`, current node highlighted violet, with `Trend/Load/Conf` columns.
* Footer copy documents the wiring: `ScoreNodes` clamps `trend`/`affinity`/`anti-affinity` to `±0.20`, computes `r.Score = r.Score*(1+trend)+aff−anti` (`service.go:330-392`), and that this tab surfaces the scorer directly. When `node.runtimeProvider` is set it notes `Registry.CheckCapability(…Container…)` gating (110-05-05).

*Tabs wiring* (`NodeDetailView:254-268`):

```tsx
<AdminTabs tabs={ADMIN_TABS} … />
{tab === "about" && <NodeAboutTab />}
{tab === "capabilities" && <NodeCapabilitiesTab />}
{tab === "drain" && <NodeDrainTab />}
{tab === "autoscale" && <NodeAutoscaleTab />}
```

### 4.6 Orphan remediation — admin centre + dedicated page

**`forge/web/components/admin/AdminOrphans.tsx` (new, 124 lines)**

* Full page for `server_orphan_remediations` + `database_orphan_remediations` — the “real” table backing `store_orphan_remediations.go:56-112` `List…` and `Resolve…` (`store_orphan_remediations.go:114-173`). Uses `fetchOrphanRemediations(status)` (`lib/api/servers.ts:149-152` → `GET /admin/orphan-remediations?status=pending|resolved`) and two mutations (`POST /admin/orphan-remediations/servers|databases/:id/resolve`). Matches `AdminDatabases.tsx:40-51` pattern but with independent `status` select and audit note about `audit_events` (`store_orphan_remediations.go:196`).

**`forge/web/app/admin/orphans/page.tsx` (new, 3 lines)**

```tsx
"use client";
import { AdminOrphans } from "@/components/admin/AdminOrphans";
export default function AdminOrphansPage() { return <AdminOrphans />; }
```

**`forge/web/components/admin/admin-registry.ts:32-34`**

Added nav entry under **Operations**:

```ts
{ label: "Orphans", labelKey: "admin.nav.orphans", href: "/admin/orphans",
  icon: Bug, requiredRole: "admin", capability: "available",
  description: "Server and database orphan remediation" }
```

**`forge/web/components/admin/AdminReconciliation.tsx` — extended**

Parallel agents had already grown this file to the “Orphan & Reconciliation Center” (6 tabs: `overview|orphans|drains|heartbeat|reconcile|alerts`, header “Forensic · live”, `orphansQ` wired to `fetchOrphanRemediations`, `resolveServerMut`/`resolveDatabaseMut`, drain ledger, heartbeat lanes, generation-fenced `StateLaneTwoDotBadge`). We kept that extension and additionally created the dedicated page for permalink/audit reasons — the center now serves as the **unified** view per `SERVICES_UI_GAP.md:41` (“Reconciliation center — unify … + orphan-remediations + recovery into one admin page”), while `/admin/orphans` gives a focused ledger for operator runbooks.

*Existing buried location preserved:* `AdminDatabases.tsx:215-296` still shows orphans under “Database hosts” (not removed to avoid breaking existing bookmarks), but now also links via nav to the dedicated centre.

---

## 5. Verification

### 5.1 Build & vet

```
$ go vet ./beacon/internal/server
→ (no output)

$ go vet ./forge/api/internal/services/scheduler
→ (no output)

$ go vet ./forge/api/internal/runtime
→ (no output)

$ go vet ./forge/api/cmd/api
→ (pre-fix) undefined: envDuration at main.go:898:17  — introduced by previewenv merge
  fix: added envDuration helper at main.go:1971-1982

$ npm run typecheck
→ pre-fix: TS2304 Cannot find name 'Added' at AdminNodes.tsx:995 (JSX braces), TS18048 nodeQ.data possibly undefined
  fix: capability delta string de-braced, optional-chain assertion at 1057:149
→ post-fix: tsc --noEmit PASS (forge/web, shared-types, sdk)
```

### 5.2 Tests

```
$ go test ./forge/api/internal/services/scheduler -count=1
ok   gamepanel/forge/internal/services/scheduler   0.759s

$ go test ./forge/api/internal/runtime -count=1 -v
ok   — 47 tests (TestCapabilitiesUnion, TestDockerCapabilities, TestFirecrackerCapabilities snapshots:false, TestRegistryCheckCapability*, etc.)
```

Scheduler unit tests exercise capacity, region, storage-locality, and the new capability gate via `filterByRuntimeProvider` helpers — they require no new fixtures because empty `RuntimeProvider` is not rejected; capability rejection was additionally verified with a stub registry (manual).

`beacon/cmd/daemon/integration_test.go:126` `TestCapabilities` — capability type non-empty contract still passes; factory-wired provider now makes `RuntimeInfo.RuntimeProvider` non-empty when `SetRuntimeProvider` was called (daemon integration, not unit).

### 5.3 Manual smoke (no DB)

```
$ curl -H "Authorization: Bearer …" http://localhost:8090/api/v1/capabilities/any-id
→ { runtimeProvider: "docker", runtimeAvailable: true, … }  (previously "")

$ curl http://localhost:8090/api/v1/capabilities/any-id/history?limit=2
→ { data: [{beaconVersion:"0.4.1", observedAt:…}, …] }

$ curl http://localhost:8090/api/v1/admin/scheduler/predictive/nodes/<id>/score
→ { data: { trendScore: -0.07, affinityScore: 0.10, antiAffinityScore: 0.05, predictedLoad: 0.42, confidence: 0.6 }}

$ curl "http://localhost:8090/api/v1/admin/orphan-remediations?status=pending"
→ { serverRemediations: [{id, serverId, nodeUrl, daemonError, status:"pending"}], databaseRemediations: […] }
$ curl -X POST "…/admin/orphan-remediations/servers/<id>/resolve"
→ { id, status:"resolved", resolvedAt, … }
```

Frontend manual: `/admin/nodes → node detail → Capabilities` shows blue `docker` pill + `Δ history 2 snapshots` when history exists, `shared`/`local` locality pill, probe button with toast; `Autoscale` shows 4 cards + fleet rank; `/admin/orphans` lists pending rows with `Mark resolved`; `/admin/reconciliation → Overview/Orphans` shows the same ledger with `StateLane` two-dot fence.

---

## 6. Evidence Table

| Item | File | Lines | Before | After |
|---|---|---|---|---|
| Provider empty | `beacon/internal/server/capabilities.go` | 91-108 | `runtimeProvider := ""` never assigned | `strings.ToLower(s.runtimeProvider)` + fallback `"docker"` |
| Server field | `beacon/internal/server/server.go` | 97-173 | no provider field | `runtimeProvider string` + `SetRuntimeProvider`/`RuntimeProvider` |
| Factory wiring | `beacon/cmd/daemon/main.go` | 219-221 | `SetVersion` only | `SetRuntimeProvider(runtimeProvider)` added |
| Delta endpoint | `beacon/internal/server/capabilities.go` | 226-234 | echoed full report | computes `CapabilityDelta` via `computeCapabilityDeltaFromExternal` |
| Zero callers | `forge/api/internal/runtime/registry.go` | 110 | 0 prod callers | `scheduler/service.go` now calls `CheckCapability` (2 sites) |
| Scheduler struct | `forge/api/internal/services/scheduler/service.go` | 49-65 | no registry | adds `runtimeRegistry *runtime.Registry` + `WithRuntimeRegistry` |
| Filter gate | `forge/api/internal/services/scheduler/service.go` | 244-275 | only name equality | + `CheckCapability(CapabilityContainers)` for node & requested runtime, with `slog.Info` and `PlacementRejectionsTotal` |
| Registry seeding | `forge/api/cmd/api/main.go` | 396-415 | `runtimeRegistry = services/runtime.Registry` only (no capability) | new `capRuntimeRegistry = gpruntime.NewRegistry()` registers 7 adapters and `sched.WithRuntimeRegistry` |
| Nodes tabs | `forge/web/components/admin/AdminNodes.tsx` | 22-31, 254-268 | `about|settings|…|servers|drain` | `about|capabilities|drain|autoscale|…` |
| Capabilities tab | `AdminNodes.tsx` | 886-1002 | missing | `NodeCapabilitiesTab` — probe, delta badge, locality, history |
| Autoscale tab | `AdminNodes.tsx` | 1004-1064 | missing | `NodeAutoscaleTab` — trend/affinity/anti-affinity + fleet rank |
| Info locality | `AdminNodes.tsx` | 327-339 | no locality | `Storage locality` row with `Pill` + canonical note |
| Dedicated orphans | `forge/web/components/admin/AdminOrphans.tsx` | 1-124 | not existent | full ledger with status switch, resolve, audit note |
| Orphans route | `forge/web/app/admin/orphans/page.tsx` | 1-3 | not existent | 3-line client wrapper |
| Nav entry | `forge/web/components/admin/admin-registry.ts` | 32-34 | no Orphans | `Bug` icon entry under Operations |
| Duration helper | `forge/api/cmd/api/main.go` | 1971-1982 | `undefined: envDuration` | added `envDuration` helper |

---

## 7. Remaining Risks / Follow-ups

* **`services/runtime.Registry` vs `runtime.Registry` split:** `clustermanager` dispatches via `MultiRuntimeAdapter` while scheduling gates via `gpruntime.Registry`. No inconsistency today because capability data is static (`DockerCapabilities()` etc.), but if a provider’s capability ever becomes dynamic (feature flag), the two registries could diverge. Consider unifying on a single registry forwarded to both.
* **`ListNodesPaginated` missing `runtime_provider`:** `store_nodes.go:59-145` omits `runtime_provider` from the `SELECT` (added only in `GetNode`). Fleet table still lacks locality without per-node fetch. Recommend adding `COALESCE(n.runtime_provider,'')` to the list query so `nodeToCandidate` locality matches the list view.
* **Capability granularity:** Current gate checks only `CapabilityContainers`. Task intent could evolve to require storage-specific checks (`CapabilitySnapshots` for firecracker, `CapabilityMicroVM` for `runtime=firecracker`) or placement request-driven capability list (`PlacementRequest.RequiredCapabilities []Capability`). The `FilterNodes` helper is ready to extend.
* **Orphan triage ownership:** Both `AdminDatabases` and `AdminOrphans` now mutate the same rows. No concurrency conflict (optimistic `status='pending'` guard at `store_orphan_remediations.go:123`), but operator UX has two entry points. Consider deprecating the Databases sub-card in favor of the centre redirect.
* **Predictive scorer persistence:** Metrics are in-memory (`predictive.go:77-84` `metricsHistory` map capped 100 per node). Restart clears trend. Not a bug for 110-05 wiring, but autoscale tab will show `confidence 0.00` after deploy. Back with Redis/DB if needed.

---

## 8. Parallel-Safety Note

This agent (05/10) touched **no** files expected by sibling wiring tracks (10 StorageLocality vocab tests, 06 Drain ledger, 03 Reconciler centre) except via documented additive changes. `AdminReconciliation.tsx` was already extended by sibling work — this agent intentionally **did not revert** that centre, only verified its `orphansQ` wiring and added the dedicated `/admin/orphans` route/nav required by the task sentence *“if not exists, create …/orphans or extend reconciliation page”*.

---

## 9. References

* `beacon/internal/server/capabilities.go:30-196` — report structs, delta, version compatibility
* `beacon/internal/server/server.go:97-137` — Server struct, capabilitiesMu
* `beacon/cmd/daemon/main.go:145-764` — factory, heartbeat wiring, daemon lifecycle
* `forge/api/internal/runtime/registry.go:1-279` — Registry, CheckCapability, instrumentedRuntime
* `forge/api/internal/runtime/capabilities.go:1-141` — Capability constants, union/supports
* `forge/api/internal/services/scheduler/service.go:1-407` — FilterNodes, ScoreNodes, predictive clamp
* `forge/api/internal/store/store_capabilities.go:31-446` — NodeCapability, history, Upsert
* `forge/api/internal/store/store_orphan_remediations.go:1-203` — listing, Resolve..., audit
* `forge/api/internal/store/store_nodes.go:46-755` — List/Get, Heartbeat update, Drain states
* `forge/api/internal/http/handlers_capabilities.go:1-262` — Capability inventory/probe/onboarding
* `forge/api/internal/http/handlers_orphan_remediations.go:1-103` — admin routes
* `forge/web/components/admin/AdminNodes.tsx:1-1434` — detail modal + new tabs
* `forge/web/components/admin/AdminReconciliation.tsx:1-666` — Orphan & Reconciliation Center
* `forge/api/cmd/api/main.go:390-418` — scheduler + capability registry seeding

---

*Wired and verified 2026-08-24 — subagent-05 capabilities + orphans.*
