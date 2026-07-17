# Recovery Coordinator Readiness

Date: 2026-06-14
Phase: 12 - Placement Reservation Engine

## Summary

GamePanel is closer to recovery orchestration because node reachability, migration intent, observability, and placement reservations now exist as separate control-plane foundations. The platform is still not ready to execute automatic recovery because restore ownership, recovery planning, and migration execution remain intentionally unimplemented.

## Recovery Ownership

Current ownership:

- Heartbeat Monitor classifies node reachability.
- Scheduler chooses eligible target nodes using reservation-aware capacity.
- Reservation Manager owns capacity holds.
- Migration Service owns migration state.
- Cluster Manager owns normal server lifecycle actions.
- Runtime Layer owns provider-neutral runtime operations.

Missing ownership:

- No Recovery Coordinator owns recovery decisions.
- No service converts node-offline facts into recovery plans.
- No service owns restore execution or rollback policy.

Required next foundation:

- A Recovery Coordinator that consumes heartbeat state, server inventory, reservation decisions, migration state, and restore capability.
- Recovery plans that are dry-run capable before execution.
- Clear conflict rules between manual migration, evacuation, and automatic recovery.

## Reservation Integration

Placement reservations now provide the capacity hold that recovery would need before restoring a workload elsewhere.

Ready:

- Capacity snapshots subtract active reservations.
- Server creation creates short-lived placement reservations.
- Migration creation reserves target capacity.
- Reservation lifecycle events enter the timeline.

Still missing:

- Reservation ownership for recovery plans.
- Reservation expiry policy tuned for restore workflows.
- Reservation renewal while a long-running restore is pending.
- Reservation handoff from recovery plan to migration/restore execution.

## Migration Coordination

Migration Service now reserves target capacity and releases reservations on cancellation, failure, or state-machine completion. This prevents some conflicts, but migration still does not move workloads.

Ready:

- Migration target validation uses reservation-aware scheduler capacity.
- Active server-linked reservations block duplicate migration target holds.
- Migration lifecycle can release reservations on terminal states.

Still missing:

- Execution locks for a server under migration.
- Restore artifacts and transfer ownership.
- Runtime migration implementation.
- Rollback and partial-failure handling.

## Failover Dependencies

Before failover execution, GamePanel still needs:

- Recovery Coordinator.
- Recovery Plan domain and store.
- Restore ownership boundary.
- Server execution locks.
- Reservation renewal and release policy for recovery.
- Migration or restore execution implementation.
- Runtime backup/restore contracts beyond the current Docker adapter limitations.
- Event durability stronger than the current in-process Event Bus if recovery must survive API process crashes.

## Readiness

Recovery Coordinator readiness: 55%.

The core inputs now exist: heartbeat classification, timeline history, migration state, scheduler placement, and reservations. The missing work is orchestration ownership and execution safety, not basic data availability.

## Recommended Next Phase

Build Recovery Coordinator Foundation next.

Minimum scope:

- RecoveryPlan domain.
- Recovery Coordinator service.
- Dry-run recovery plan API.
- Recovery reservation integration.
- Conflict checks with active migrations and evacuations.
- No automatic execution yet.
