# Placement Reservation Readiness

Date: 2026-06-14
Phase: 11 - Heartbeat Expiry Engine

## Summary

GamePanel is closer to safe failover because node reachability can now be classified independently from coarse node actual state. Placement reservations are still not implemented and should be the next foundation before recovery automation can safely allocate replacement capacity.

## Capacity Races

Current placement capacity is computed from stored node capacity and existing server allocations. This is sufficient for single-request placement decisions, but it does not reserve capacity before a migration, restore, or future failover operation begins.

Risk:

- Two concurrent creates or migrations can choose the same available capacity.
- Capacity can appear available during planning but be consumed before execution.
- Evacuation and migration planning can calculate feasible targets without holding those targets.

Required foundation:

- A durable reservation record with resource requirements, target node, owning workflow, expiry, and status.
- Atomic reservation creation against current capacity.
- Reservation release on completion, failure, cancellation, or timeout.

## Double Placement

Manual node selection and automatic scheduler placement both remain supported. Without reservations, two workflows can independently select the same node for the same scarce resources.

Risk:

- Server creation and migration planning can conflict.
- Future recovery plans can conflict with user-initiated migrations.
- A server could receive competing placement intent if migration/recovery ownership is not locked.

Required foundation:

- One active placement intent per server/workload.
- Workflow-level ownership for create, migration, evacuation, and recovery.
- Conflict checks before creating migration or recovery reservations.

## Reservation Ownership

The Scheduler currently chooses nodes. Cluster Manager owns server creation. Migration Service owns migration state. Evacuation Planner owns evacuation plans. No service owns durable capacity holds yet.

Recommended ownership:

- Scheduler evaluates candidates and returns placement decisions.
- Cluster Manager requests reservations for new server placement.
- Migration Service requests reservations for migration targets.
- Future Recovery Coordinator requests reservations for failover restores.
- A Placement Reservation Store should own persistence, expiry, and atomic capacity accounting.

## Scheduler Coordination

Scheduler V1 can filter by region, node state, draining/maintenance, online status, and capacity. Heartbeat classification now improves the `actual_state` input it already consumes.

Still missing:

- Reservation-aware available capacity.
- Reservation-aware scoring.
- Reservation expiry and cleanup.
- Coordination between scheduler decisions and later execution.

## Readiness

Placement reservation readiness: 35%.

The architecture has enough domain boundaries to add reservations cleanly, but current placement decisions are advisory and not protected against concurrent capacity races.

## Recommended Next Phase

Build Placement Reservations before Recovery Coordinator or Failover Engine.

Minimum scope:

- `PlacementReservation` domain.
- Store methods for create, confirm, release, expire, and list.
- Scheduler capacity calculations that subtract active reservations.
- Cluster Manager integration for server creation.
- Migration Service integration for migration target holds.
- Metrics and events for reservation lifecycle.
