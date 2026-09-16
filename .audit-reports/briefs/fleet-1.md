# Scope 1 — Nodes, Regions, Locations: BACKEND (audit and FIX, write code directly)

Read `.audit-reports/briefs/fleet-common.md` first for tier boundaries, the three invariants and the shared-file
and reporting rules. You are scope 1; report to `.audit-reports/reports/fleet-1.md`.

## You own (edit only these)

- `forge/api/internal/http/handlers_admin.go` — 2086 lines, ~116 routes. Your routes in it: `/nodes`, `/nodes/:id`,
  `/nodes/:id/health`, `/capacity`, `/lifecycle`, `/rotate-token`, `/configuration`, `/deployment`, `/allocations`,
  `/servers`, `/evacuation-preview`, `/evacuation-plan`, `/nodes/deployable`; `/regions`, `/regions/:id`,
  `/regions/:id/cluster`, `/regions/:id/capacity`; `/locations`; and the base `/mounts`, `/mounts/:id`,
  `/mounts/:id/servers|nodes|eggs` CRUD (scope 6 owns only `handlers_mount_manage.go`'s extra mount operations, so
  the `/mounts` routes living in `handlers_admin.go` are yours). Read this file in chunks top to bottom — do not
  skim it.
- `forge/api/internal/http/handlers_admin_extras.go`
- `forge/api/internal/http/handlers_regions.go` (97 lines, ~5 routes)
- `forge/api/internal/http/handlers_locations.go` (98 lines, ~5 routes)
- `forge/api/internal/store/store_nodes.go` (1389 lines), `store_nodes_extra.go`, `store_regions.go`,
  `store_locations.go`, `store_capacity.go`, `store_node_metrics.go`, `store_heartbeat.go`, `store_evacuation.go`,
  `store_placement_intents.go`
- The node/region/location query paths in `forge/api/internal/placement/` and `internal/scheduler/scheduler.go`
  ONLY where placement reads node state. Do not touch `scheduler_nomad.go` (scope 10).

## Hunt for, with extreme depth

1. **Health/honesty**: a node whose heartbeat row is stale or missing reported as healthy/online. `store_heartbeat.go`
   + `store_node_metrics.go` — find every place a zero value can mean "never reported", and make the distinction
   explicit in the response (a `reported`/`last_seen` field or an unknown state), not silently `0`.
2. **Capacity arithmetic**: `store_capacity.go` and `/nodes/:id/capacity` — capacity computed as 0 when a metric was
   not reported; overcommit derived from a stale reading; allocatable minus requested allowing negative or
   oversubscribed placement.
3. **Ambiguous target**: any handler that resolves a node by iterating credentials or calling `SetDefaultNode`
   instead of requiring the caller to name the node. Reject with 400.
4. **Tenancy/authz holes**: node/region/location read or write routes missing `requireAdminScope`, missing org
   scoping in the store `WHERE` clause, or leaking another org's node in a list or in `/nodes/deployable`.
   `/nodes/:id/rotate-token` must not be callable by a caller who cannot administer that node.
5. **Double/shadow registration**: the same path registered twice with different guards. Note
   `handlers_admin.go:2522`-area comment in `server.go` claims `GET /mounts/:id` is registered by
   `registerAdminRoutes` — verify that claim is true and that it is not also registered elsewhere with weaker auth.
6. **Param-name mismatches**: `c.Params("id")` read where the route declares `:nodeId` (or vice versa), and path
   segments silently ignored so a handler returns fleet-wide data for a scoped-looking URL.
7. **Store correctness**: `sql.ErrNoRows` swallowed into a zero-value struct indistinguishable from "not found";
   JSONB columns written as TEXT (see the known tags text-vs-jsonb failure mode); multi-row writes not wrapped in a
   transaction; node/region lists with no pagination bound; deletes of a region or location that still has nodes
   attached (FK or orphan) succeeding instead of refusing.
8. **Lifecycle state machine**: `/nodes/:id/lifecycle` allowing invalid transitions (e.g. draining→online without
   finishing, or marking ready while drain is incomplete); evacuation-preview claiming it is safe without checking
   that a destination with capacity exists; `rotate-token` leaving the previous token still valid on the Beacon.
9. **Input validation**: negative or absurd CPU/memory/disk values accepted; blank names; unknown `scheduler_type`
   accepted and persisted (must be validated against the supported set without renaming the persisted values —
   `docker`/`k3s`/`nomad` are frozen for scope 10's reason).

## Report requirements

Beyond the standard finding lines, include a short `## Route inventory` table listing every node/region/location
route you verified: path, guard(s) applied, store method, and whether the response can distinguish unknown from
zero. End with `## Cannot verify`.
