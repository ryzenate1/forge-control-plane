# Forge Control Plane: Operations and Infrastructure Audit

**Date:** 2026-07-17  
**Scope:** Admin **Operations** and **Infrastructure** sections, their Forge API/Beacon dependencies, and targeted comparison with `reference/pufferpanel`.  
**Method:** Ten independent read-only code audits. No application, configuration, migration, or generated files were changed. This report is the only file created.

## Executive summary

The reported pages are **not generally frontend mocks**: standard CRUD paths for regions, locations, nodes, allocations, database hosts, and mounts call real authenticated APIs and persist to the database. However, multiple broken UI/API contracts, suppressed query errors, and incomplete daemon reconciliation make real backend failures appear as empty data or false success.

The panel is **not production-ready** for node onboarding, live mount changes, evacuation, or disaster recovery:

- **Migration** has a substantial real transfer engine, but needs worker/reconciliation hardening and end-to-end validation.
- **Evacuation** and **recovery** persist plans only; their execution endpoints deliberately return **HTTP 501**.
- The legacy server-transfer route is state/queue-only and must not be presented as execution.
- Node creation persists a node, but the supplied Beacon onboarding material is incompatible with the Beacon process configuration and signing contract. A saved node may therefore never connect.
- Several dashboards turn `401`, `403`, `404`, `503`, and network failures into empty arrays or green `0` counts.

> Do not describe these areas as fully live until the P0/P1 work below is complete and integration-tested.

## Confirmed user-facing defects

| Priority | Area | Confirmed defect | Evidence |
|---|---|---|---|
| P0 | Activity / monitoring | Web requests nonexistent `GET /account/activity`; activity failures are suppressed. | `forge/web/lib/api.ts:876-878`; `forge/api/internal/http/handlers_auth.go:391-406`; `forge/web/components/admin/AdminActivityLog.tsx:101` |
| P0 | Activity | Two `GET /activity` routes are registered; intended service-backed global activity route is shadowed. Screen and CSV/JSON export use different stores/datasets. | `forge/api/internal/http/handlers_auth.go:391-406`; `handlers_activity.go:42-84,187-267`; `server.go:1335,1344` |
| P0 | Node onboarding | Node UI discards returned complete credential, later shows only the secret and an unsupported `beacon configure`/YAML flow. Beacon uses environment variables and validates the complete credential. | `forge/web/components/admin/AdminNodes.tsx:386-434,568-586`; `forge/api/internal/store/store_nodes.go:220`; `beacon/cmd/daemon/main.go:53-74,212-230` |
| P0 | Mounts | Assigning a mount to an existing server changes only DB state; runtime sync ignores mounts and attempts duplicate container creation. “Mounted” is not runtime truth. | `forge/api/internal/http/handlers_servers.go:1549-1567`; `beacon/internal/server/server.go:493-505,529-590`; `forge/web/components/admin/AdminServers.tsx:676-710` |
| P0 | Evacuation / recovery | Evacuation and recovery are planning-only. Their start endpoints return HTTP 501. | `forge/api/internal/http/handlers_admin.go:1833-1835,1899-1919`; `forge/api/internal/services/evacuationplanner/service.go:277-286`; `recovery/service.go:274-297` |
| P1 | Overview | Counts/failures aggregate only the first server page (15 records). Resource usage reads fields the node API does not provide, so usage is fabricated as zero. | `forge/web/lib/api.ts:434-436`; `forge/api/internal/http/handlers_servers.go:182-220`; `AdminOverview.tsx:88-110` |
| P1 | Monitoring | Orchestration/activity query failures render as green `0`; `api`/`memory` checks queried by the UI are not registered by the API. | `AdminHealth.tsx:148-165,208-211,296`; `forge/api/cmd/api/main.go:272-347` |
| P1 | Regions | Region create/update is broken: slug is lowercased then validated as uppercase-only. “Enabled for placement” is not used by scheduler/recovery/evacuation filtering. | `forge/api/internal/store/store_regions.go:99-109,122-131`; `forge/web/components/admin/AdminRegions.tsx:27-38`; `scheduler/service.go:104-138` |
| P1 | Node location/region | UI uses a Location UUID as both `regionId` and `locationId`; node updates cannot update `location_id`. Placement filtering uses incomplete list data. | `AdminNodes.tsx:569-584`; `handlers_admin.go:135-153,204-265`; `store_nodes.go:16-85,174-211,236-275` |
| P1 | Allocations | Multi-port allocation create is not transactional; partial ranges can persist even though the request returns an error. Reload/configuration drops secondary allocations. | `handlers_admin.go:1452-1495`; `store_allocations.go:69-79`; `handlers_remote.go:243-260` |
| P1 | Database hosts | No connection test exists before save; UI defaults TLS to plaintext `disable`; failed provisioning and orphan remediation are not available in UI. | `AdminDatabases.tsx:46-90`; `store_databases.go:451-457`; `dbprovisioner/service.go:85-125` |
| P1 | API contracts | Operations recovery/evacuation calls target nonexistent `/admin/...` routes. Node allocation alias/bulk delete payloads do not match server contracts. | `forge/web/lib/api.ts:923-944,1217-1242`; `handlers_admin.go:318-344,574-615`; `handlers_admin_extras.go:138-188` |

## Operations assessment

### Overview, infrastructure health, orchestration, action cues

**Real data, misleading presentation.** The overview makes live API calls, but is not reliable under pagination or request failure:

1. `/servers` defaults to `per_page=15`; the web client discards `meta`, then computes the whole-panel server count, state distribution, recent list, and failures from page one only.
2. `memory`/`disk` usage displayed in overview and monitoring does not exist on the node response. These bars are not Beacon telemetry.
3. Health failures and user-query failures can still produce “No failures reported” and `Users: 0`.
4. Reservations/recovery query errors are hidden and displayed as green `0 active / 0 failed` orchestration.
5. “Daemon Connectivity” reports persisted heartbeat/status inference, not an active connectivity probe. With no nodes, the backend warning is truthful, but surrounding UI can still look healthy/green.
6. UI looks for health check names `api` and `memory`; the registered checks are `database`, `cache`, `daemon`, `system`, and queue. “API Runtime” is therefore permanently unknown.

**Required direction:** provide server-side aggregate/read-model endpoints, distinguish `unavailable` from `zero`, align check names/semantics, render setup-required/no-node state neutrally, and add actual daemon telemetry rather than showing configuration capacity as usage.

### Activity and exports

There are distinct sources: `audit_events`, `audit_logs`, and `activity_events`. The current UI mixes sources while CSV/JSON exports only `activity_events`; filters do not apply to export. The `/account/activity` call is nonexistent, and duplicate `/activity` route registration makes the intended route unreachable.

**Required direction:** establish a single canonical admin activity query with provenance/deduplication and use it for page rows, filters, totals, CSV, and JSON. Use separate unambiguous paths for account and platform activity. Escape cells beginning `=`, `+`, `-`, or `@` during CSV export to avoid spreadsheet formula injection.

### Duplicate logs/activity

The two screens look identical because they attempt to represent overlapping audit/runtime concepts without a defined product distinction. Keep one canonical **Activity** history. If a separate **Logs** page is needed, it should be an operational diagnostics stream with different schema, retention, filtering, and access controls—not another audit list.

### Migration, evacuation, recovery

| Capability | Actual state |
|---|---|
| Migration | Real durable transfer protocol exists between Forge and Beacon, including reservations, archive, transfer, checksum, restore, ownership switch, and compensation. It is not yet production complete. |
| Evacuation | Candidate selection and persisted plan are real; no executor exists. Workloads are untouched. Start is HTTP 501. |
| Recovery | Offline-node planning/reservations/migration records are real; no restore/execution exists. Start is HTTP 501. |
| Legacy server transfer | Creates queue/state records but does not invoke migration engine. Treat as unavailable/misleading. |

Before enabling execution: add a durable periodic migration worker, cleanup reconciler, concurrency/lease controls, full source/target Beacon + PostgreSQL test environment, explicit evacuation executor, and a source-aware recovery executor that fences failed nodes and restores from verified backup/replica/source data.

## Infrastructure assessment

### Regions and Locations

- **Regions:** creation/update currently fails due to a normalization/validator contradiction. Seeded/backfilled lowercase slugs also cannot be updated. `enabled` is persisted but ignored by placement/recovery/evacuation selection.
- **Locations:** create wiring is real. Empty fields disable the button with a small inline message; populated fields POST to `/locations`, and backend errors should be rendered. The observed apparent no-op is likely disabled-button UX, request rejection (authorization, CSRF, IP access, rate limit), or a visually hidden error—not missing submit code.
- Location API/UI drift: UI expects `serverCount` and `updatedAt` not returned by the backend, so location server count is always zero.
- Remove user-facing legacy controller branding. This is content cleanup, but do not make it obscure the functional requirements above.

### Nodes — release blocker

The node record CRUD path is real but unsafe/incomplete as a daemon onboarding workflow.

1. **Fix registration credential semantics first.** Generate a supported artifact using `DAEMON_NODE_ID`, `PANEL_API_URL`, and the complete token. Implement either a real Beacon `configure` command or a documented systemd/Docker environment-file workflow. Show the credential exactly once.
2. **Stop destructive update behavior.** Basic node save currently risks resetting advanced persisted configuration because updates write a broad record from a sparse DTO. Use PATCH pointers or server-side merge.
3. **Separate Region and Location cleanly.** Pick a canonical scheduling grouping and return/filter the correct IDs in list/detail/deployable views.
4. **Validate endpoints/resources server-side.** Parse URLs, validate FQDN/IP and port ranges/collisions, capacity values, uniqueness, and referenced region/location.
5. **Make TLS/proxy/daemon endpoint authoritative.** The UI defaults to port 8080 while Beacon defaults to 9090; proxy/TLS configuration conflicts across layers.
6. **Introduce lifecycle gates:** registered → credential delivered → connected → runtime verified → placement eligible. Use heartbeat state in lists; avoid probing every listed node every 10 seconds.

PufferPanel is useful here for its explicit host/port validation and node deletion safeguards, but Forge needs its own explicit dual endpoint/proxy/daemon model.

### Allocations

Allocation CRUD and server consumption are real, not mocked. Key fixes:

- make range creation atomic;
- return meaningful list/dependency errors instead of empty state;
- ensure all assigned allocations are included in reload/configuration, not only the primary mapping;
- check `RowsAffected` on updates;
- register static bulk routes before dynamic `/:id` routes;
- match frontend payload casing/contracts and expose mutation errors;
- explain the operational implications of `0.0.0.0` default binding.

### Database hosts

Database provisioning has real PostgreSQL/MySQL connection code and encrypted credential persistence. It is not mock-backed. Current product gaps:

- create a privileged connection-test endpoint/UI before persistence;
- default browser-created hosts to verified TLS, not `disable`;
- fix custom CA edit semantics: blank must preserve current value, and API/UI must agree whether system trust is permitted;
- enforce non-negative capacity and define `blank/unlimited` semantics explicitly;
- expose provisioning state/error, retry, host history, and orphan remediation queue;
- make ordinary and force-delete behavior understandable and auditable.

### Mounts — release blocker

Mount records, associations, and fresh provisioning are real. The claim of a mount being active for an existing workload is not real:

- Mount assignment only records a DB pivot; it does not update/recreate the running workload.
- Beacon config sync ignores mount payload and calls create against an existing container.
- Node `allowed_mounts` configuration is not enforced.
- Path validation is insufficient (absolute/canonical paths, traversal, symlinks, sensitive paths, duplicate targets, runtime parity).
- A created mount is ineligible by default because UI does not attach nodes/eggs, and server selector lists ineligible mounts without error feedback.

Required: transactional assignment + runtime reconciliation; backend pending/failed/runtime state; canonical allowlist/path enforcement in every runtime; eligibility filtering; safe detach/edit/delete lifecycle; and integration tests across Docker, containerd/Kubernetes, and unsupported runtimes.

## Cross-cutting quality and readiness

### Error handling

These components default failed queries to empty arrays, so actual failures look mocked/unconfigured: Nodes, Allocations, Locations, Database hosts, Mounts, and several overview/monitoring subqueries. Render a visible error state with status/request ID, retry, and correct permission/setup actions. Never display a green zero result when its source query failed.

### Worktree status

The working tree already contains extensive unrelated/pre-existing modifications and deletions, including tracked docs, Forge, Beacon, migrations, and generated binaries. Audit observations:

- 159 modified tracked files; 49 deleted tracked files; 240 untracked paths at audit time.
- Unstaged tracked diff approximately `+8,984 / -22,008`; no staged changes.
- Root npm workspace lockfile does not match the declared Forge web workspace.
- Untracked/duplicate migration sequencing means a clean checkout may not contain or apply current schema work.

Do not clean, revert, move, or release from this worktree until intended changes are isolated and reviewed.

### Existing diagnostics

Workspace diagnostics report several Beacon and reference-PufferPanel errors plus web/API warnings. Audit agents independently verified `go mod verify`, `go vet ./...` in `forge/api`, focused health/API tests, and web typecheck in limited scopes; these passed where run. Full builds were intentionally not run because this is an audit-only pass on a dirty tree and can overwrite generated artifacts.

## Ordered remediation plan

### Phase 0 — prevent false success (first)

1. Add visible error/retry states for all admin queries and mutations; do not map failures to empty/green values.
2. Fix frontend/API endpoint and payload mismatches for activity, evacuation, recovery, allocation alias, and bulk allocation deletion.
3. Correct Regions slug normalization and enforce `regions.enabled` in placement/recovery/evacuation.
4. Fix Overview pagination and remove fake resource-usage metrics until Beacon telemetry is available.
5. Unify activity routes/data model/export and add CSV formula escaping.
6. Feature-flag or clearly label evacuation/recovery/legacy transfer as **plan only** until executors exist.

### Phase 1 — node and infrastructure correctness

1. Rebuild node onboarding around a single executable Beacon configuration contract.
2. Repair location/region domain model, list projections, and placement filters.
3. Implement safe node PATCH updates, validation, endpoint/proxy/TLS consistency, and lifecycle readiness gates.
4. Make allocation ranges transactional and preserve all mappings on reload/configuration.
5. Add database host test/retry/history/orphan workflows and secure TLS defaults.
6. Make mount assignment reflect real runtime reconciliation, enforce allowlists/path policy, and represent runtime state honestly.

### Phase 2 — operational execution and release hardening

1. Add migration worker/reconciliation plus full multi-daemon integration coverage.
2. Implement evacuation executor with draining, reservations, per-item progress, concurrency and cancellation.
3. Implement recovery executor with fencing/split-brain protection and verified restore-source policy.
4. Resolve root lockfile/migration provenance, add deployment topology, real CI deployment gates, migration lock, non-root containers, probes, image/action pinning, and security scanning.

## Definition of done

Do not mark the Operations/Infrastructure area complete until automated integration tests demonstrate:

- location/region/node creation with truthful validation/error feedback;
- Beacon configured from the generated node artifact successfully heartbeats and accepts signed commands;
- node updates preserve advanced fields and a node becomes placement eligible only after validation;
- allocation ranges are atomic and secondary allocations survive reload;
- database host TLS/test/retry/orphan workflows report real remote state;
- mount assignment/detach reports actual runtime success or a recoverable pending/error state;
- overview/monitoring gives correct full-panel counts and reports unavailable sources honestly;
- activity UI, CSV, and JSON export the same filtered canonical records;
- migration succeeds and recovers across restart/failure cases; evacuation/recovery execution is either fully validated or unavailable by feature flag.
