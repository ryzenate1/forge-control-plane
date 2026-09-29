# Scope 4 — Git-triggered deploy + GitOps (audit and FIX, write code directly)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering, route
registration, the two invariants (never report success for work not performed; never resolve an ambiguous node
target silently), and that Panel->Beacon is HTTP via internal/daemon.Client — websockets never carry commands.

## A previous pass was interrupted mid-edit
`forge/api/internal/services/git/deployment_service.go` currently shows as modified in git. Re-read CURRENT
content of every file first; repair half-applied changes (unused or missing imports, declared-but-unused
variables, a helper called that does not exist, a signature changed at only some call sites, an error now
returned where a caller still ignores it).

## Files you own — edit ONLY these
- forge/api/internal/services/git/deploy.go
- forge/api/internal/services/git/deploy_service.go
- forge/api/internal/services/git/deployment_service.go
- forge/api/internal/services/compose/gitops.go
- forge/api/internal/services/compose/git_deploy_adapter.go
- forge/api/internal/http/handlers_git_deploy.go
- forge/api/internal/store/store_git_deployments.go
Read-only, do NOT edit: internal/http/server.go, internal/daemon/client.go, beacon/internal/server/git.go,
forge/web/lib/api/git-deployments.ts, services/git/service.go, handlers_git.go.

## Trace this path, then hunt
push/webhook event -> deployment record created -> panel calls Beacon -> Beacon pulls and builds -> status
returns -> deployment marked finished.
1. Trigger correctness — branch/ref filters inverted or ignored so a push to a protected branch deploys;
   `[skip ci]` not honoured; webhook signature verification absent, using `==` instead of constant-time, or
   SKIPPED when the secret is unconfigured (fail-open on a missing secret is a bug — it must reject); duplicate
   deliveries creating two concurrent deployments of the same commit with no dedupe or lock.
2. Concurrency — two triggers deploying the same stack/server with no per-target lock; a running deployment
   silently overwritten instead of cancelled or queued; a status row overwritten by an older job finishing last
   (needs check-and-set on revision or ID).
3. State honesty — a deployment recorded success when the Beacon call errored, timed out, or never happened; a
   commit SHA persisted without a confirmed fetch; progress/replica counts defaulted to 0 when the real value is
   unknown; a failed step's error swallowed so the parent reports healthy; a `running` state with no terminal
   transition on timeout or node loss.
4. Dead or unwired code — a gitops poller or trigger service that nothing constructs or starts: grep for its
   constructor and its start method and PROVE whether it runs. Confirm every handler in handlers_git_deploy.go
   is mounted and that the git-deploy route group is registered from server.go.
5. Tenancy and RBAC — a trigger, redeploy, log-read or teardown route missing an org filter or
   requireServerPermission; ID-only lookups; cross-org repo access by guessed ID.
6. Injection — refs, branch names, env values and commit messages interpolated into shell commands, clone paths
   or generated compose files. Repository content is hostile.
7. Retry and timeout — a Beacon HTTP call with no timeout or context deadline; unbounded retries without
   backoff; a retry that re-triggers a non-idempotent deploy.

Fix everything in place, matching existing style. No new dependencies; no premature abstraction. Never delete a
signature check or widen a permission to make code work. No comments explaining what code does; one short line
only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-4-git-deploy-gitops.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Dead / unwired code` (with grep evidence),
`## Security findings`, `## Needs elsewhere`, `## Checked clean`.
Return a summary under 250 words, leading with the gitops/trigger wiring verdict.
