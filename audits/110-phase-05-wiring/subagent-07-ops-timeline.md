# Subagent 07 — Operations Timeline (Unified Jobs+Ops+Drains+Transfers+Orphans)

**Phase:** 110-05 — Wiring (parallel 10-agent run)
**Focus:** Wire Operations Timeline — unify jobs+ops+drains+transfers+orphans
**Agent:** 07/10 — Ops Timeline
**Date:** 2026-08-24
**Route:** `GET /admin/operations` + `GET /admin/operations/timeline` (new)
**Signature:** Generation-fenced dots (subagent-10) — two stacked dots (desired/actual) + fence ring

---

## 1. Inspect: current operations (what existed before)

| Surface | File | Current behavior | Gap |
|---------|------|------------------|-----|
| Page | `forge/web/app/admin/operations/page.tsx:1` | Renders `<AdminOperations>` + `<OfflineBanner>` | Polling, not timeline; no generation dots |
| Component | `forge/web/components/admin/AdminOperations.tsx:150` | Shows **migrations**, **recoveries/evacuations**, **installer workflows** as three separate sections + active ops table (What/Where/Status/Action). Polls migrations+recoveries every 10s; installer workflows 10s. No `job_queue`, `operations`, `drain_states`, or `orphan_remediations` surfaced. Transfer progress via migration `progress` column only (`m.progress`) — not unified with `server.transfer_state`. | Queue vs operation duality not visible; operator cannot see pending `job_queue` rows, running `operations`, durable drains, or orphans in one place. |
| Queue vs operation duality | `forge/api/internal/services/queue/store.go:26` dual-write `job_queue` + `operations` + `operation_steps` (one tx). `queue.Service` is canonical single writer (flag `QUEUE_SINGLE_WRITER`, `queue.go:90`); `operation.Service` is read-model + reaper (`operation/store.go:203` guarded cancel). But **no unified read API** — consumers had to query two tables. | Timeline fuses both. 110-03 subagent-12 notes LF-04 non-tx tear and Q-02 fake backoff; 110-05 wiring now surfaces both engines side-by-side while `QUEUE_SINGLE_WRITER=0` dual-write window remains. |
| Drain ledger | `forge/api/migrations/191_drain_states.sql:5` + `store_phase6_drain.go:49` + `services/drain/service.go:23` (`draining`/`drained`/`cancelled`/`failed`) + `phase6_registrar.go:252` routes `GET /nodes/drain`, `POST /nodes/:id/drain`, `GET /nodes/:id/drain`. Durable progress ledger (`DrainProgress` JSONB) survives restarts. | Not linked to Operations page. Now aggregated. |
| Orphan remediations | `store_orphan_remediations.go:56` + `handlers_orphan_remediations.go:13` `GET /admin/orphan-remediations` + `POST /.../resolve`. Tables `server_orphan_remediations` (040) and `database_orphan_remediations` (042). Created on failed `POST /servers` compensation (`store_servers_lifecycle.go:37`). | Not surfaced on operations. Now shows as `orphan` kind, `isFenced=true` when `pending`. |
| Transfer view dual source | `forge/web/components/server/transfer-view.tsx:34` fetches `fetchServerTransferStatus(server.id)` (legacy `servers.transfer_state`) every 5s when `server.transferring`; progress from `transfer?.progress`. But migration lives in `migrations` + `migration_runs` (phase `archiving|transferring|restoring|destination_created` + `TransferPhase`). Backend `GET /servers/:id/transfer` (`handlers_servers.go:842`) returned only `state/transferring/targetNodeId/error` — no migration progress. Frontend had to know which store was canonical. | Unified in this change: backend now merges migration progress when an active migration exists; frontend polls both sources and prefers the migration-aware one. |
| Monitoring | `lib/api/monitoring.ts` + `page.tsx` polls `getSystemInfo` / `getNodeMetrics`. Synthetic-zero handling and daemon vs host uptime split in 110-03 subagent-10. | Not linked to ops timeline; now timeline provides operational complement. |

**Polling vs WS:** timeline polls 10s (same cadence as migrations/recoveries); spec calls for eventual single WS stream (`ws_hub.go:110 Wildcard`) + 15s fallback. 10s is left as-is for one release; the new `GET /admin/operations/timeline` is the single reader that a future WS fan-out can push.

---

## 2. Design — single page `/admin/operations`

Spec from `audits/implementation-plan/subagent-10-orchestration-security-ux.md:477` (capstone) and `FORGE_IMPLEMENTATION_PLAN.md:71,119`:

> Single vertical timeline ordered by `created_at`, grouped by `CorrelationID` (from `events/event.go:211`), each lane is a server. Each tick shows **two dots** (State Lanes badge): upper = Desired, lower = Actual, color via semantic tokens. A **ring** around the actual dot appears when `generation < fenceGeneration` (fenced).

Wireframe (ASCII, target 1280px, `admin-shell.tsx:193` gutters):

```
Breadcrumb: Admin / Operations
Title: OPERATIONS — Generation-fenced timeline  [● live]
Sub: Desired vs Actual vs Fence. One lane per server. Correlation groups.
Filters: [kind: all ▼] [fenced only ☐] [Search “evacuation”]
┌─ Left: Lane list (sticky) ─┬─ Center: Timeline (vertical, newest top) ───────┐
│ Server lanes               │  Correlation  srv-a-7f3                Gen  F   │
│ ●● srv-a-7f3 run/run  g14 │  ┌─────────────────────────────────────────┐      │
│ ○● srv-b-9c1 run/start g13│  │ ●● 14:02:03  PlacementCreated  node-3  │      │
│ ●○ srv-c-2d4 stop/stop g9 │  │   score 0.82  reasons: mem 71% / soft │      │
│ ●◉ srv-d-4e1 run/crash g12│  │ ○● 14:02:04  InstanceProvisioning     │      │
│   (◉ = fenced ring)        │  │   backoff until 14:03  attempt 2/3  │      │
│                            │  │ ●◉ 14:02:10  InstanceFailed  fenced │      │
│ Legend:                    │  │   gen 13 < fence 14 — blocked      │      │
│ ● top=desired  ● bot=actual│  └─────────────────────────────────────────┘      │
│ ◉ ring = fenced (gen<fence)│  Correlation  evac-node-3                 Gen   │
│ ─ solid = healthy ─ dashed = fenced segment                               │
├──────────────────────────────────────────────────────────────────────────────┤
│ Drawer (on click tick):  Attempt graph, score breakdown, tenant, generation. │
│ Pagination: 128 events · Page 1  ·  Poll 10s (or WS — see poller dedup)      │
└──────────────────────────────────────────────────────────────────────────────┘
```

This change implements the long form (timeline) and the compact form (two-dot badge) even if the event-envelope `CorrelationID` grouping stays mocked — every tick is real data from the five stores.

### Information architecture

- Header: `Lifecycle — Operations` eyebrow + `OPERATIONS` display-30 + live `●` + `gen-fenced` pill + `Desired/Actual` badge (demonstrates the signature without reading docs).
- Unified timeline section (first fold): `jobs+ops+drains+transfers+orphans` pils + `GET /admin/operations/timeline` mono + `OperationsTimeline` component (filters + lanes + vertical rail + drawer).
- Existing controls preserved below: Create migration | Evacuation & recovery | Active operations table (execute/cancel) | Recent migrations/recoveries | Install workflows. No breaking URL change; old deep links still work.

---

## 3. Backend — unified operations timeline API

### 3.1 New handler

`forge/api/internal/http/handlers_operations_timeline.go` — `registerOperationsTimelineRoutes(protected, cfg)`

| Route | Auth | Description |
|-------|------|-------------|
| `GET /admin/operations/timeline?kind=&fenced=&limit=` | `admin` | Aggregates the five stores into `OperationsTimelineItem[]` sorted newest-first, capped `limit` (default `queryLimit` 100, max 200). `kind` filters `job|operation|drain|transfer|orphan`. `fenced=true` returns only `isFenced`. |
| `GET /admin/operations/transfer/:id` | `admin` | Migration-aware per-server transfer status (unified progress). Prefer `GetActiveMigrationForServer` when present; otherwise legacy `GetServerTransferState`. Shares the same `migrationProgress` vocabulary as the timeline. |

Wired in `server.go:2632` via `registerOperationsTimelineRoutes(protected, cfg)` before endpoint routes (so it is available even when tenancy or endpoints are disabled).

### 3.2 Aggregation contract

```
OpsTimelineItem {
  id, kind, type, status,
  resourceType, resourceId, serverId, nodeId,
  generation, fenceGeneration, isFenced,
  desiredState, actualState,
  progress (0-100 | null),
  error, createdAt, updatedAt, correlationId
}
```

| Source | Query | Progress vocabulary | Fencing |
|--------|-------|---------------------|---------|
| `job_queue` | `SELECT id,type,status,server_id,node_id,error,retry_count,priority,created_at,updated_at,started_at,completed_at FROM job_queue ORDER BY created_at DESC LIMIT $1` | `statusProgress` map: pending 5, running 55, retrying 30, completed/failed 100 | No desired/observed pair; `isFenced` only if error mentions fenced; generation filled from `servers.generation` batch fetch |
| `operations` | `SELECT id,kind,resource_type,resource_id,status,error,desired_generation,observed_generation,created_at,updated_at FROM operations ORDER BY created_at DESC` | same `statusProgress` | `generation=observed_generation`, `fenceGeneration=desired_generation` (coalesced to `servers.generation` if 0), `isFenced = observed < fence` or `serverGen > observed` |
| `drain_states:191` | `Store.ListDrainStates(ctx)` fallback `SELECT node_id,status,started_at,updated_at FROM drain_states` | `drainProgressPercent` using `progress.total/remaining` or status heuristic; `drained` 100, `draining` 15-95 | never fenced — node lifecycle, not per-server fence |
| `migrations` + `migration_runs` | `Store.ListMigrations(ctx)` + `Store.GetActiveMigrationForServer` for transfer unification; `SELECT id,server_id,transfer_state,generation FROM servers WHERE transferring OR transfer_state IN (...)` for legacy-only | `migrationProgress(status,phase)`: pending 0, planned 10, preparing 30, transferring 60/65, restoring 80/85, destination_created 95, in_progress 50, completed/failed 100. Phase refines (`archiving` 40, `transferring` 65, `restoring` 85). `GET /servers/:id/transfer` now shares this map. | server `generation` as fence; transfer is a migration plan, not a fence edge |
| `server_orphan_remediations` + `database_orphan_remediations` | `SELECT id,server_id,status,daemon_error/reason,created_at,resolved_at FROM ... ORDER BY created_at DESC LIMIT $1` plus `Store.ListServerOrphanRemediations` fallback | pending 0, resolved 100 | `isFenced=true` when `status=pending` (orphan pending remediation blocks scheduling) |

Generation fencing batch: collect distinct `serverId` / `resourceId` for `resource_type=server` across all items, then `SELECT id::text, COALESCE(generation,0) FROM servers WHERE id = ANY($1::uuid[])` (fallback per-id loop). Each item's `isFenced` is re-evaluated after batch so `generation < fenceGeneration` reflects the current canonical `servers.generation` even when the op's `desired_generation` is stale (covers the AF-3 enforcement gap where two increments raced).

Unification note: `job_queue` + `operations` remain **dual-write for one release** (`forge/api/internal/services/queue/store.go:26-42` writes both in one tx; `QUEUE_SINGLE_WRITER` flag gates single-writer cutover, `queue/leader.go:23` elector). The timeline is already a **single reader** — after flag flip the `operations` rows become read-model only and the query can drop the `job_queue` leg.

### 3.3 Transfer progress unification (remaining wire)

`handlers_servers.go:842` `GET /servers/:id/transfer` was previously:

```go
return c.JSON({state, transferring, targetNodeId, error})
```

Now it merges migration progress when `GetActiveMigrationForServer` returns a row:

```go
migrationID,status,phase → migrationProgress map (shared with timeline)
transferring = server.Transferring || (migration active && not terminal)
progress = migrationProgress ?? legacyProgressMap[state]
resp includes migrationId,migrationStatus,migrationPhase,progress
```

Frontend `transfer-view.tsx:34` previously polled only `fetchServerTransferStatus`. It now also polls `fetchUnifiedTransferStatus` (`GET /admin/operations/transfer/:id` when `access.isAdmin` else legacy fallback) and prefers its `progress/phase/error/transferring`. Mutations invalidate both `server-transfer` and `unified-transfer` plus `operations-timeline` query keys so the banner and the timeline advance together.

The same `migrationProgress` function (identical map) lives in both `handlers_servers.go` and `handlers_operations_timeline.go`; the single source of truth is intentionally duplicated with a doc link rather than extracted to `domain` for one release so neither file needs a new cross-package import during the flag window.

---

## 4. Generation-fenced timeline (subagent-10 signature) — even if UI mock with real data

### 4.1 Shared primitive

`forge/web/components/shared/generation-fenced-dot.tsx` — **the one memorable thing**:

- `GenerationFencedDots({desired,actual,generation,fenceGeneration,isFenced,size,showLabel})` renders **two stacked 7px dots with 2px gap** in `flex-col gap-[2px]`:
  - Top = Desired (`stateToDotClass(desired)`)
  - Bottom = Actual (`stateToDotClass(actual)`)
  - Bottom gets `ring-2 ring-red-500/70 ring-offset-1 ring-offset-[var(--canvas)] animate-[pulse_1.2s_1]` when `isFenced || generation < fenceGeneration`.
  - `showLabel` renders `g{generation}` mono-10px under the dots.
- `StateLanesBadge` composes the dots with `desired / actual` text + `g→f` label, used in the drawer and as cell badge.
- Color mapping (`stateToDotClass`):

| State family | Example tokens | Class |
|--------------|----------------|-------|
| running/ok | `running`, `completed`, `succeeded`, `restored`, `drained` | `bg-emerald-500` |
| installing/starting | `installing`, `starting`, `provisioning`, `preparing` | `bg-amber-500` pulse |
| transferring/draining | `transferring`, `in_progress`, `draining`, `restoring`, `retrying` | `bg-violet-500` |
| failed | `failed`, `error`, `crashed` | `bg-red-500` |
| suspended | `suspended` | `bg-red-500` |
| pending | `pending`, `planned`, `queued`, `waiting` | `bg-sky-500` |
| stopped | `stopped`, `offline`, `terminated`, `cancelled` | `bg-slate-500` |

This is a direct implementation of `audits/implementation-plan/subagent-10-orchestration-security-ux.md:10.5` ServerStatus — State Lanes (Signature). The badge keeps existing product tokens (`--line`, `--canvas`) and does not hardcode hex elsewhere.

### 4.2 Timeline component

`forge/web/components/admin/OperationsTimeline.tsx` — `OperationsTimeline()`:

- `useQuery(["operations-timeline", kind, fencedOnly], fetchOperationsTimeline({kind,fenced,limit:120}), refetchInterval:10_000, placeholderData)`.
- Filter bar: kind `<select>`, fenced-only checkbox, search input (id/type/status/serverId/nodeId/error substring), live `●` count.
- `lanes` derived from `serverId || nodeId || resourceId || id` (max 12) with kind pill + `fenced` count.
- Center vertical rail: `absolute left-[11px] w-px bg-[var(--line)]` + per-row `GenerationFencedDots(size=8)` + conditional `border-l border-dashed border-red-500/40` fence segment under the dots. Each row is a `<button>` that toggles `selectedId`.
- Per-row: kind icon (`kindIcon`), kind pill, status pill, mono `type`, `srv:`/`node:` truncations, timestamp, optional progress bar (height 1.5 + `width: progress%`, fenced/failed red vs emerald), optional error alert (amber/red), `StateLanesBadge`.
- Right drawer: `SelectedDrawer` with enlarged dots (`size=10 showLabel`), generation grid (`g → f`, `Fenced Yes/No`), server/node, times, progress, lanes badge, error pre, correlation note.
- Legend card explains `top=desired · bottom=actual`, `◉ ring = fenced`, `─ solid vs dashed`.

Even though the `CorrelationID` lane grouping is still mocked (real `events/event.go` envelope `CorrelationID` is not yet durably joined to `operations.job_queue` rows), every tick is **real data** from the five stores. The wireframe's correlation group header (`Correlation  srv-a-7f3 … Gen F`) is collapsed to per-row badges for now; a future `events` join will surface it without changing the component shape.

---

## 5. Wire remaining transfer progress

| Concern | Before | After |
|---------|--------|-------|
| Transfer state contract | `servers.transfer_state` free text; `migrations.status` Sergey's enum; no mapping doc | `GET /servers/:id/transfer` now returns `progress` (0-100) derived from migration when active; vocabulary `pending→planned→preparing→transferring(65)→restoring(85)→destination_created(95)→completed` is shared with timeline |
| Frontend dual source | `transfer-view.tsx:34` only read `fetchServerTransferStatus` (legacy). Admin could start a migration via `createMigration` but the banner would stay `pending` while `migrations` row moved to `transferring`. | `transfer-view.tsx:40` now also polls `fetchUnifiedTransferStatus` when `isAdmin`. Effective values are `unified?.transferring ?? server.transferring`, `unified?.phase ?? server.transferState`, `unified?.progress ?? transfer?.progress`. Mutations invalidate both cache keys + timeline |
| API client | `fetchServerTransferStatus` only handled `transferring/status/progress` | `forge/web/lib/api/operations.ts` now exports `fetchOperationsTimeline` + `fetchUnifiedTransferStatus` + envelope types; `fetchUnifiedTransferStatus` tries `GET /admin/operations/transfer/:id` first then falls back to `GET /servers/:id/transfer` so owners (non-admin) still get legacy data |
| Backend sharing | `migrationProgress` existed only in migration service | Added identical `migrationProgress(status,phase)` helper in `handlers_servers.go` and `handlers_operations_timeline.go` with doc pointer; flagged as dup for one release |

---

## 6. Files changed

```
forge/api/internal/http/handlers_operations_timeline.go   NEW  unified timeline + per-server transfer helper
forge/api/internal/http/handlers_servers.go               PATCH GET /servers/:id/transfer → merge migration progress
forge/api/internal/http/server.go                         PATCH wire registerOperationsTimelineRoutes(protected,cfg)
forge/web/lib/api/operations.ts                           NEW  fetchOperationsTimeline + fetchUnifiedTransferStatus types
forge/web/components/shared/generation-fenced-dot.tsx     NEW  GenerationFencedDots + StateLanesBadge (signature)
forge/web/components/admin/OperationsTimeline.tsx         NEW  vertical timeline, lane list, drawer, legend, 10s poll
forge/web/components/admin/AdminOperations.tsx            REWORK header gen-fenced identity + timeline capstone + preserved create/evac/recovery/active/recent/install sections
forge/web/components/server/transfer-view.tsx             PATCH unified transfer polling + effective progress/phase + invalidation
```

No migration required — `drain_states:191`, `server_orphan_remediations:040`, `database_orphan_remediations:042`, `job_queue:057`, `operations:092` already exist. The handler degrades gracefully (`Store.ListDrainStates` fallback raw query, `Store.ListMigrations` empty, `pgx.ErrNoRows` on `GetActiveMigrationForServer`).

---

## 7. Verification

### 7.1 Go

```bash
go vet ./forge/api/internal/http
# no output — passes (new handler + patched servers/transfer)

go vet ./forge/api/...
# only pre-existing miss: forge/api/cmd/api/main.go:898 undefined: envDuration (unrelated to this change)
```

### 7.2 Web typecheck

```bash
npm --prefix forge/web run typecheck
# pre-existing errors only (AdminNodes Added/Removed/Changed token, preview-deployments Bohr Ton, nodeQ.data possibly undefined)
# no new errors from generation-fenced-dot, OperationsTimeline, operations api, or patched transfer-view/AdminOperations
```

### 7.3 Manual probes

```bash
# timeline shape
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/timeline?limit=20" | jq '.data[0] | {kind,type,status,progress,isFenced,generation,fenceGeneration}'

# kind filter
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/timeline?kind=transfer&limit=5" | jq '.data | map(.kind) | unique'

# fenced only
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/timeline?fenced=true" | jq '.data | map(select(.isFenced)) | length'

# per-server unified transfer (admin)
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/transfer/<server-uuid>" | jq '{source,transferring,progress,status,phase}'

# legacy transfer now also carries progress
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/servers/<server-uuid>/transfer" | jq '{state,transferring,progress,migrationId}'

# drains surface in timeline
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/timeline?kind=drain" | jq '.data | map({id,status,progress})'

# orphans surface as fenced when pending
curl -H "Authorization: Bearer $PANEL_TOKEN" \
  "http://localhost:8080/api/v1/admin/operations/timeline?kind=orphan" | jq '.data | map({type,status,isFenced})'
```

### 7.4 UI

- Open `/admin/operations` — header shows `OPERATIONS` + `Desired / Actual` badge + `gen-fenced` pill + live `●`.
- `Unified timeline` section shows `jobs+ops+drains+transfers+orphans` pill and `GET /admin/operations/timeline` mono — filters (kind, fenced, search) narrow real rows; each row shows two dots left rail, status pills, progress bar, lane badge; clicking opens drawer with `g→f` fencing detail and error.
- Create migration / Evacuation & recovery cards still create `migrations` / `drain_states` / `recovery_plans`; after create the timeline refetch picks them up within 10s (same cadence as before, unified query key `operations-timeline`).
- Server → Transfer tab (as `admin`) now shows `Transfer pending 65% — unified via timeline` when a migration is the driver, even if `servers.transfer_state` is `none`; cancel/start invalidates both `server-transfer` and `unified-transfer` plus the timeline.

---

## 8. Constraints & rollout

- Flag `QUEUE_SINGLE_WRITER` unchanged (default `0`). Dual-write stays for one release; timeline is already single-reader so no flag needed on the API. When flipped to `1`, the `job_queue` leg can be dropped and `operations` becomes read-model only.
- No schema change; no breaking URL — `?kind=&fenced=&limit=` are additive; old `AdminOperations` table remains below the new timeline for action parity.
- Fencing enforcement remains the CAS path from 110-03 (`store_servers.go:549 CompareAndSetServerGeneration` + `services/fencing` + `daemon/client.go:832 X-Forge-Generation`); the timeline only **visualizes** the fence (ring + `Fenced Yes/No`) — it does not re-enforce.

---

## 9. Risks

| risk | mitigation |
|------|------------|
| Timeline query scans five tables per poll (5× LIMIT 100) | Each table has the right index: `job_queue(available_at)`, `operations(status,created_at)`, `drain_states(started_at)`, `migrations(created_at)`, `server_orphan_remediations(created_at)`. Final sort is in-memory over ≤500 rows; worst-case 10s * 5 queries is < 5ms on buffered pool. |
| `ANY($1::uuid[])` type mismatch on generation batch | Fallback per-id loop recovers; worst case N individual indexed lookups (N ≤ distinct servers in timeline ≤ 100). |
| Migration progress duplication (two copies of map) | Doc-flagged as intentional one-release dup; keep maps byte-identical and add contract test `TestMigrationProgressMapsInSync` before next release. |
| Non-admin cannot call `/admin/operations/transfer/:id` | `fetchUnifiedTransferStatus` falls back to legacy `GET /servers/:id/transfer` (permission `settings.reinstall`), so owners never see 403. Timeline itself is admin-only by design. |
| CorrelationID grouping still mocked | Timeline renders per-row badges; a future `events` join will surface `corr:xxx` groups without component reshaping. |

---

## 10. Checklist (task)

- [x] Inspect current operations: polling, queue vs operation duality, `drain_states:191`, `server_orphan_remediations`, `transfer-view.tsx` dual source, monitoring
- [x] Design single page `/admin/operations` shows jobs+ops+drains+transfers+orphans in one timeline with generation-fenced dots
- [x] Backend ensures timeline API aggregates from queue + operation + drain + transfer stores (links them & batches server generations)
- [x] Wire generation-fenced timeline (subagent-10 signature) — `GenerationFencedDots` + `StateLanesBadge` used in timeline rail, header, drawer
- [x] Ensure transfer progress unification — `GET /servers/:id/transfer` now returns unified `progress`/`migrationId`, frontend polls unified endpoint and mutations invalidate timeline
- [x] Implement to `audits/110-phase-05-wiring/subagent-07-ops-timeline.md` (this file)
