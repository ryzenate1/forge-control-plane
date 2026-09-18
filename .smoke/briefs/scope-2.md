# Scope 2 — Servers lifecycle, node administration, locations, health/maintenance

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 2.

## Owned files (`forge/api/`)

`internal/http/handlers_servers.go`, `handlers_admin.go` (nodes, reservations, allocation-pool admin
sections), `handlers_admin_extras.go`, `handlers_locations.go`, `handlers_regions.go`,
`handlers_nests.go`, `handlers_eggs*.go` if present, `handlers_tags.go`, `handlers_maintenance.go`,
`handlers_health_checks.go`, `handlers_resource_limits.go`, `handlers_health*.go`, `handlers_crashdetect.go`,
`internal/services/noderegistry/`, `internal/services/nodeprobe/`, `internal/services/heartbeatmonitor/`,
`internal/services/health/`, `internal/services/healthcheckrunner/`, `internal/services/crashdetector/`,
`internal/services/tags/`, `internal/services/registrations/` — plus the matching `_test.go` files.
Frontend (read-only for contract checks; edits go to scope 10's files only if you stay read-only):
`forge/web/lib/api/servers*.ts`, `forge/web/lib/api/nodes*.ts`, `forge/web/lib/api/locations*.ts`.

## Smoke checklist

Node CRUD through the real API: create, read, list, patch, delete, `POST /nodes/:id/rotate-token`,
`POST /nodes/deployable`. Two nodes exist: A `22222222-…2222` online, B `e43bb9aa-…55ad` offline.
Prove the offline node is correctly excluded from `deployable`, and that a server pinned to an offline
node fails with a clear error rather than hanging or reporting healthy.

Server lifecycle on node A with real Docker: create `smoke2-*` server, start, stop, restart, kill,
suspend/unsuspend, transfer, delete. Watch the **state endpoint**, never infer lifecycle from a stats
number — a stats stream that shows CPU does not prove the workload is in the state the panel claims.
Verify `status`, `desiredState`, `actualState`, `suspended`, `transferState` are honest at each step and
agree with `docker ps` and with Postgres.

Sizing/limits: memory, disk, CPU shares/limit, swap, io weight, overallocate percentages — set them, then
verify they reach the container (`docker inspect`) and that an impossible request (more RAM than the node
has, over the overallocate cap) is refused with the right status class.

Allocations/ports: allocate, list, free, and prove port ranges per node are respected (node A uses
25565-26565, node B 26566-27565). Verify an allocation cannot be double-booked and that `autoAllocate`
behaves when the range is exhausted.

Health/maintenance: node heartbeat → status transition, maintenance mode blocks new placement,
`crash-detection` read + reset, health-check runner actually runs HTTP/TCP/command checks and reports
failure honestly (not "healthy" when it could not reach the target).

## Two known bugs you own — root-cause and fix them

1. `POST /api/v1/nodes` answers **500** for validation failures that should be 4xx. Reproduce exactly:
   missing/invalid `baseUrl` and a loopback `fqdn` both return `{"error":"…"}` with `HTTP=500`, while a
   missing `LocationID` correctly returns 422. So one validation class is routed through a generic error
   path (`respondInternalError` or a bare `fiber.NewError(500)`), losing the 422. Fix the mapping so
   client errors are 400/422 and only true server faults are 500. Prove it with the same three bodies.
2. `GET /api/v1/locations` returns `nodeCount: 0` for the Local Lab location even though node A
   (`region: local-lab`, `locationID: cccccccc-…`) and node B both belong to it. Either the aggregate is
   unimplemented, computed from the wrong key, or never updated on node create. Find which, fix, and
   prove the count changes when you create/delete a node.

Also check `serverCount` on the same endpoint for the same class of staleness.

## Report

`.smoke/reports/scope-2.md`.
