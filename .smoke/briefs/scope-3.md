# Scope 3 — Placement, scheduling, reservations, capacity simulation, reconcile

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 3.

This is the scope the user named most explicitly ("placement, reservation simulation"). Treat the
simulation as a real feature to be proven, not an endpoint that returns 200.

## Owned files (`forge/api/`)

`internal/placement/**`, `internal/scheduler/**`, `internal/scheduling/**` (if present),
`internal/services/reservations/`, `internal/services/scheduler/`, `internal/services/reconciler/`,
`internal/services/deployment/` (placement/strategy parts), `internal/services/clustermanager/`,
`internal/services/replicamanager/`, `internal/services/autoscaler/`, `internal/services/nodeautoscale/`,
`internal/services/queue/`, `internal/services/pipeline/`,
`internal/http/handlers_placement.go`, `handlers_scheduler.go`, `handlers_scheduled_tasks.go`,
`handlers_cronjob.go`, `handlers_reconcile.go`, `handlers_autoscaler.go`, `handlers_nodeautoscale.go`,
`handlers_zerodowntime.go`, `handlers_orphan_remediations.go`, `handlers_capacity*.go` if present,
plus matching `_test.go`. The reservation sections of `handlers_admin.go` are **shared with scope 2**:
they own that file. Put reservation fixes you need in your own files, or list them under
NEEDS-ORCHESTRATOR with the exact edit and let the orchestrator apply them.

## Smoke checklist

Placement: for each strategy the code supports, place `smoke3-*` workloads and prove the chosen node is
the one the strategy's scoring says should win (spread vs binpack vs resource-affinity etc.). Read
`explain.go` and verify the **explanation matches the decision actually taken** — an explain endpoint
that narrates a different algorithm than the one that ran is an S2 honesty bug. `POST`
`/api/v1/placement/explain` returned 405 to the orchestrator when GET'd; find the real verb/path and
verify both directions work.

Reservation simulation — the full state machine, on both nodes:
`GET /api/v1/reservations`, `GET /reservations/:id`, `POST /reservations`,
`POST /reservations/:id/cancel`, `POST /reservations/:id/confirm`.
Prove: (a) a pending reservation reduces the node's available capacity for *other* placements;
(b) confirm converts it into real usage exactly once and a second confirm is refused;
(c) cancel releases the capacity and it becomes placeable again;
(d) an over-capacity reservation is refused with the right status, not created and later failing;
(e) reservation capacity math respects overallocate percentages and reserved-memory/disk;
(f) reservations expire/have TTL honesty — no reservation is reported as available capacity if the host
is offline (node B is offline; use it for this).
A "simulation" that only echoes the request is an S1 finding: say so plainly.

Scheduler: rules, constraints, `env-affinity`, leader-elected scheduler for cron/scheduled tasks — verify
leader election does not double-fire (look for two executions of the same tick) and that a manual trigger
and a scheduled trigger share one code path.

Reconcile + orphan remediation: create a deliberate orphan (a container in Docker with no panel record,
or a panel record with no container) and verify reconcile detects it, reports it honestly, and that the
remediation actually remediates instead of re-reporting. Zerotrust check: does `reconcile` ever mark a
stale node healthy?

Capacity: total/allocated/available/free consistency across `/nodes`, `/reservations`,
`/admin/capability*`, and the allocation endpoints — the same node must not report two different numbers
to two different endpoints. Prove arithmetic sums (allocated + free = total) and that overcommit is
represented honestly.

## Report

`.smoke/reports/scope-3.md`. Put the numeric before/after of every capacity calculation in the report —
that is what the orchestrator will re-check after restart.
