# Failover Readiness Report

Date: 2026-06-14

Scope: Phase 10 Observability Platform Foundation.

## Summary

Phase 10 improves failover readiness by making orchestration behavior visible and durable. Timeline events, correlation IDs, heartbeat history, health history, and observability APIs now provide the evidence future failover automation will need.

GamePanel is still not ready to execute automatic failover. The remaining blockers are intentional because this phase only creates data collection and visibility.

## Heartbeat Expiry

Current status: Partially ready.

What exists:

- Node heartbeat submissions are persisted into `node_heartbeat_history`.
- Heartbeat success/failure and heartbeat gaps are stored.
- Invalid node-token heartbeat attempts can be recorded as heartbeat failures.
- Node health snapshots include heartbeat score.

What is missing:

- No heartbeat-expiry evaluator marks nodes offline after a timeout.
- No configurable failure threshold exists.
- No recovery trigger is emitted when a heartbeat gap crosses policy.

Readiness verdict: Observability is ready; failure detection is not automated.

## Recovery Triggers

Current status: Not ready.

What exists:

- Node actual state can represent online, offline, and degraded.
- Timeline events can show NodeOnline, NodeOffline, NodeDegraded, state changes, and reconciliation activity.

What is missing:

- No failover trigger service.
- No recovery plan domain.
- No policy for degraded vs offline vs maintenance/draining.
- No operator approval workflow.

Readiness verdict: Future recovery triggers now have data to inspect, but no trigger engine exists.

## Migration Coordination

Current status: Foundation only.

What exists:

- Migration is durable and has status history.
- Migration events are timeline-captured.
- Migration events use migration ID as the correlation ID.
- Scheduler and Evacuation Planner can validate candidate targets.

What is missing:

- No migration execution.
- No migration lock on servers.
- Reconciler does not pause or coordinate with active migrations.
- No rollback model.
- No server node reassignment or placement commit.

Readiness verdict: Migration intent is traceable, but migration execution coordination is not ready.

## Placement Reservations

Current status: Not ready.

What exists:

- Scheduler filters and scores nodes.
- PlacementCreated events are captured into the timeline.
- Capacity snapshots are queryable and health snapshots are persisted.

What is missing:

- No durable placement reservation table.
- No reservation expiry.
- No reservation commit/rollback.
- No protection against concurrent placements overcommitting the same node.

Readiness verdict: Placement decisions are visible, but not yet safe for automated failover execution.

## Restore Ownership

Current status: Not ready.

What exists:

- Runtime abstraction exists for create/delete/power/stats/inspect.
- Docker adapter exposes migration methods that intentionally return `ErrNotImplemented`.

What is missing:

- Backup and restore remain daemon-coupled.
- No runtime-neutral archive/restore contract exists.
- No restore orchestration service exists.
- No restore events or restore timeline semantics exist.

Readiness verdict: Restore ownership must be designed before failover can execute.

## Phase 10 Improvements

- Durable timeline table for event-derived operational history.
- Correlation IDs across server lifecycle, reconciliation, evacuation, and migration operations.
- Heartbeat history table with failure and gap tracking.
- Health history table with health score and capacity snapshot fields.
- API endpoints:
  - `GET /api/v1/timeline`
  - `GET /api/v1/timeline/:resourceType/:resourceId`
  - `GET /api/v1/correlations/:id`
  - `GET /api/v1/nodes/:id/heartbeats`
  - `GET /api/v1/nodes/:id/health-history`
- Metrics:
  - `game_panel_timeline_events_total`
  - `game_panel_correlation_groups_total`
  - `game_panel_heartbeat_failures_total`
  - `game_panel_health_snapshots_total`

## Recommended Next Phase

Recommended next phase: Failover Readiness Foundations.

Scope should remain pre-execution:

- Heartbeat expiry evaluator.
- Recovery trigger policy.
- Placement reservations.
- Migration/reconciliation coordination locks.
- Restore ownership design.
- Failover dry-run plans.

Do not implement automatic failover, recovery execution, workload movement, runtime expansion, live migration, or AI features until those foundations are reviewed.
