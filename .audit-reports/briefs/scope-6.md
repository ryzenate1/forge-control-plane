# Scope 6 — Deployment engine + stores (COMPLETE an interrupted audit, and FIX)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root. Its invariants are your
spec: (1) never report success for work not performed — unknown is not zero, not-reported is not zero, a stale
reading is not healthy; (2) never resolve an ambiguous target silently; (3) discarded errors are bugs.

## A previous pass was INTERRUPTED MID-EDIT ON THESE VERY FILES — this is your first job
Evidence from timestamps: execution.go, healthgate.go, beacon_executor.go at 21:39; rollout.go at 21:43. That
agent neither finished nor verified anything.
Audit those files for half-applied changes and make every file internally consistent BEFORE adding new fixes:
an import added but unused, or removed while still referenced; a variable declared and never used; a helper
called that does not exist; a signature changed but not at every call site; an error now returned where a caller
still ignores it; a guard inserted mid-block leaving unreachable code.

## Files you own — edit ONLY these
- forge/api/internal/services/deployment/service.go
- forge/api/internal/services/deployment/execution.go
- forge/api/internal/services/deployment/rollout.go
- forge/api/internal/services/deployment/healthgate.go
- forge/api/internal/services/deployment/revisions.go
- forge/api/internal/services/deployment/steps.go
- forge/api/internal/services/deployment/beacon_executor.go
- forge/api/internal/store/store_deployments.go
- forge/api/internal/store/store_deployment_steps.go
- forge/api/internal/store/store_deployment_revisions.go
- forge/api/internal/store/store_deployment_history.go
Read-only, do NOT edit: internal/daemon/client.go (scope 10 owns it), http/handlers_deployment.go,
http/handlers_deployment_rollback.go (scope 7 owns those).

## Known leads — roughly 27 `_ =` discarded-error sites; verify against CURRENT source (lines may have moved,
or the killed agent may have already fixed some)
- execution.go:556 `_ = s.markStepCompleted(...)` — a step reported completed while the DB error is discarded.
  The clearest invariant-1 violation.
- execution.go:558 `_ = s.updateProgress(...)`
- execution.go:398,413,432 — auto-rollback events discarded; a failed auto-rollback must not be silent.
- healthgate.go:120 `_ = s.markStepFailed(...)` — the failure itself may not persist.
- revisions.go:200 `_ = s.store.UpdateDeployment(...)`
- service.go:454 `_ = s.store.UpdateDeploymentStepStatus(... cancelled ...)`
Judge each: publishing an event AFTER the state was durably written may stay best-effort; persisting a state
transition never may.

## Bug classes, after the consistency repair
1. State machine honesty — every status/progress/step transition must be persisted, and a persistence failure
   must surface (log at minimum, and change the outcome where the transition is load-bearing). A deployment must
   never reach success/healthy/completed without a confirmed round-trip. Timeout, cancellation and node-loss
   paths must land in a real terminal state rather than staying `running` forever.
2. Node targeting — especially beacon_executor.go: anywhere a node or client is chosen while the target is
   absent or ambiguous must reject, not take the first node with credentials.
3. Rollout safety (rolling / canary / blue-green) — a traffic percentage applied without a confirmed upstream
   write; proceeding to the next batch after a failed health gate; draining the old instance before the new one
   is actually ready (capacity loss); promoting on an UNKNOWN replica count instead of a confirmed one; a
   rollback failure escalating instead of being recorded as a critical unresolved state.
4. Health gate — unknown or unreachable counted as pass, or as a benign failure; the consecutive-failure counter
   reset incorrectly; no timeout path; off-by-one on the threshold.
5. Revisions and history — a revision recorded for a config never applied; history written as success on a
   partial failure; concurrent revisions for one server racing (check for optimistic locking); a rollback
   reusing stale fields from the revision it restores.
6. Concurrency/lifecycle — goroutines without cancellation; context deadline ignored; no per-deployment lock;
   executor DB writes racing the HTTP handler; timers and tickers not stopped.
7. Store — missing tenancy filter; sql.ErrNoRows collapsed into a zero value; an UPDATE affecting zero rows
   that returns nil; multi-row step writes without a transaction; unbounded history queries.

Fix everything in place, matching existing style. No new dependencies; no new abstraction layers. Never weaken a
check or lower a threshold to make code pass. No comments explaining what code does; one short line only when
the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. The user requires code-only shipping; verification
is central afterwards. Compensate by reading precisely — every call you make must match a real signature.

## Deliverable — write incrementally, not at the end
Report file: `.audit-reports/scope-6-deployment-engine.md`. After you finish EACH source file, append its
findings immediately so an interruption cannot erase the record.
Sections: `## Half-applied repairs` (FIRST — highest value), `## Fixed` (file:line — problem -> change),
`## Discarded errors reviewed`, `## State transitions that can still lie`, `## Needs elsewhere`,
`## Checked clean`.
Return a summary under 300 words. Do not overstate: if something is still broken, say so.
