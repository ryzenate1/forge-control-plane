# Scope 4 — Docker + image/cache cleanup + docker events: BACKEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 4; report to `.audit-reports/reports/fleet-4.md`.

## You own (edit only these)

API:
- `forge/api/internal/http/handlers_docker.go` (760 lines, ~26 routes; `registerDockerRoutes` at server.go:2647)
- `forge/api/internal/http/handlers_docker_cleanup.go` (295 lines, ~10 routes; registrar `("docker-cleanup", 215, …)`)
- `forge/api/internal/http/handlers_docker_events.go` (519 lines; registrar `("docker-events", …)`)
- `forge/api/internal/http/handlers_cleanup.go`
- `forge/api/internal/http/cache.go`
- `forge/api/internal/services/dockerleanup/`, `internal/services/cleanup/`
- `forge/api/internal/store/store_docker_events.go`, `store_docker_registries.go`
- `forge/api/internal/daemon/client.go` — ONLY the `AdminContainer*`, `AdminImage*`, `AdminVolume*`,
  `AdminNetwork*` families plus the helpers they use (`adminContainerAction`, `adminGetJSON`, `adminPostJSON`).
  Other method families belong to scopes 6/8 — surgical edits to your lines only.
Beacon:
- `beacon/internal/server/container_admin.go`, `docker_cleanup.go`, `docker_events.go`
- `beacon/internal/runtime/docker.go` — read for the contract; runtime adapter files are shared, so if a change is
  needed there, report it rather than editing unless it is squarely about image/volume pruning.

Read every owned file fully first. `handlers_docker.go` is long; read it in chunks, not by grep.

## Hunt for, with extreme depth

1. **Prune honesty — the central risk in your slice.** A prune that returns 200 when the daemon errored;
   "space reclaimed" computed from a pre-prune estimate instead of a real post-prune delta; `0 B reclaimed`
   indistinguishable from "the node never answered". Both numbers (reclaimed, remaining) must come from actual
   readings, and a missing reading must be an error or an explicit unknown.
2. **Retention policy theatre**: a cleanup request that accepts `keep_last`/`older_than`/`exclude_in_use` parameters
   and then ignores them. Verify the filter is actually applied to the daemon call and that the response reflects
   what was enforced.
3. **Deleting something in use**: image or volume prune that can remove an image/volume still referenced by a
   running or scheduled workload. There must be a reference check against workloads before deletion, and `force`
   must not be a bypass for that check.
4. **Dangerous defaulting**: an empty filter map meaning "everything"; a missing node meaning "all nodes"; a missing
   `since`/`until` meaning unbounded; `all: true` defaults.
5. **Node targeting**: every container/image/volume/network call must be explicitly node-scoped, and the node id in
   the URL must provably be the node used to build the daemon request — look for a handler that validates one and
   then resolves another.
6. **Event stream integrity**: an unbounded event stream with no per-node cap or backpressure; events persisted with
   no retention (unbounded table growth); a reconnect that duplicates or silently drops events without recording the
   gap; a "connected" flag reported from a stale channel state after the peer went away.
7. **Timeouts/cancellation**: daemon calls built without a context deadline; a hung Beacon hanging the panel request;
   no cancellation propagation when the client disconnects mid-stream.
8. **Registration/authz**: docker cleanup and docker events both register via `init()` phase registrars — verify both
   actually fire and neither path is shadowed by another registrar at the same priority. Mutating routes must carry
   `mutationLimiter` and admin scoping; registry credential routes must not return secret material on read.
9. **Store**: `sql.ErrNoRows` collapsed into an empty slice; non-transactional multi-row writes; org scoping missing
   on event listing; unbounded `SELECT *` on the events table.

## Report requirements

Add a `## Prune safety` section listing each destructive endpoint you found, what it can delete, whether an
in-use check exists, and what a caller sees when nothing could be measured. End with `## Cannot verify`.
