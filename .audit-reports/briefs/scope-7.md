# Scope 7 — Deployment HTTP: deployment, history, rollback, resource limits (audit and FIX)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering (handlers receive a
`Config` struct of service pointers), the route-registration mechanism, and the two invariants — never report
success for work not performed; never resolve an ambiguous node target silently.

## A previous pass was interrupted mid-edit
Re-read CURRENT content of every file first and repair half-applied changes: unused or missing imports,
declared-but-unused variables, a helper called but not defined, a signature changed at only some call sites.

## Files you own — edit ONLY these
- forge/api/internal/http/handlers_deployment.go
- forge/api/internal/http/handlers_deployment_history.go
- forge/api/internal/http/handlers_deployment_rollback.go
- forge/api/internal/http/handlers_resource_limits.go
Read-only, do NOT edit: internal/http/server.go, internal/http/phase_registry.go,
internal/services/deployment/*.go (scope 6), forge/web/lib/api/{deployments,deployment-rollbacks,resource-limits}.ts.

## Wiring facts to verify rather than assume
registerDeploymentRoutes is invoked at server.go:2589 and registerDeploymentHistoryRoutes at server.go:2590.
handlers_deployment_rollback.go:69 and handlers_resource_limits.go:34 each call RegisterPhaseRegistrar — confirm
those calls sit inside an `init()` in a file that is built, because a registrar defined but never registered
silently yields 404s.

## Bug classes, endpoint by endpoint
1. Unwired handlers — this repo's documented failure mode: a handler compiles and 404s because the route line
   was never added. Enumerate every handler function in your four files (`grep '^func'`), then confirm each has
   a matching `.<method>(` registration inside its register*Routes body. Add the missing ones. Flag any path
   registered twice where a later route shadows an earlier one.
2. Authorization — rollback is a mutating, high-privilege action: verify admin scope or requireServerPermission
   plus an org tenancy filter, and that a rollback cannot cross tenants. Check deployment-history reads and
   resource-limit writes for a missing tenancy filter. Node-targeted resource-limit writes must be explicitly
   node-scoped.
3. Boundary validation — parse errors ignored or answered with 200; a missing or blank nodeId/serverId defaulting
   to "first available" (violates invariant 2 — must be rejected); unbounded bodies; a rollback target revision
   verified to exist but not verified as belonging to the same server AND tenant; numeric limits (CPU, memory,
   replicas, timeouts) parsed with no range check so a negative or zero value later reads as unlimited.
4. Honest responses — 200 {"ok":true} after a swallowed service error; a nil service pointer bypassed with a
   silent empty result instead of a clear 503/501; a list endpoint returning [] on error, indistinguishable from
   "no data"; a cached or last-known value served as current status; wrong codes (500 for a 404/403/409
   condition, 201 without a created resource).
5. Streams — websocket/SSE handlers with no cleanup, no write deadline, unbounded goroutines per connection, or a
   stream that reports healthy when the upstream fetch failed.
6. Contract drift — compare paths, verbs and JSON field names AND casing (snake_case vs camelCase is a known trap)
   against lib/api/deployments.ts, deployment-rollbacks.ts, resource-limits.ts. Fix Go where Go is wrong; record
   web-side mismatches under "Needs elsewhere".

Fix everything in place, matching existing handler style. No new dependencies or abstraction layers. Never weaken
an authorization or validation check to make a path work. No comments explaining what code does; one short line
only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-7-deployment-http.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Unwired endpoints` (evidence: handler defined / registration
absent or present), `## Authorization gaps`, `## Needs elsewhere`, `## Checked clean`.
Return a summary under 250 words.
