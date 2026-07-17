# Failover Engine Readiness

Date: 2026-06-14
Phase: 13 - Recovery Coordinator Foundation

## Summary

GamePanel is closer to failover readiness, but Phase 13 remains planning-only. The platform can now classify offline nodes, identify affected workloads, reserve target capacity, create planned migration records, and expose recovery plans through API surfaces.

The architecture is not yet allowed to execute recovery. No service should move workloads, restore backups, call runtime migration execution, or perform automatic failover until a dedicated Failover Engine phase is approved.

## Reservation Integration

Recovery planning uses placement reservations with `reservation_type = recovery`.

Current behavior:

- Target capacity is reserved after Scheduler selects a recovery target.
- Reservations are linked to server and migration IDs.
- Failed or cancelled recovery plans release reservation intent through ReservationManager cancellation.
- Scheduler-facing capacity snapshots subtract active reservations.

Remaining risks:

- Reservation expiration is process-local and has no distributed lease owner.
- Reservation and migration creation are separate steps, so full multi-resource atomicity is not guaranteed.
- Recovery execution will need explicit reservation confirmation or release semantics.

## Migration Integration

Recovery planning creates planned migration records only.

Current behavior:

- Recovery items link to planned migrations.
- Migration status starts as `planned`.
- No transfer, restore, runtime execution, or state-machine execution occurs during recovery planning.

Remaining risks:

- Migration records do not yet carry an explicit recovery plan ID.
- Migration execution is still a state-machine-only foundation.
- Backup/archive/restore dependencies are not owned by the migration layer yet.
- Cross-runtime and live migration remain unsupported.

## Runtime Dependencies

Recovery Coordinator does not call runtime interfaces.

Future Failover Engine runtime needs:

- restore/archive execution ownership
- runtime availability checks before execution
- target runtime compatibility validation
- post-restore server inspection
- rollback behavior for partially completed recovery

Current blockers:

- Docker migration methods still return `ErrNotImplemented`.
- Runtime abstraction does not yet own backup transfer, restore, file movement, or install state.
- Daemon-local runtime internals remain Docker-shaped in several non-orchestration paths.

## Execution Blockers

Before Failover Engine can safely execute recovery:

- Define Failover Engine ownership and interfaces.
- Decide whether recovery execution consumes RecoveryPlan directly or creates a separate FailoverRun.
- Add execution locks so one server cannot be recovered by multiple active plans.
- Add restore ownership and target allocation rules.
- Define reservation confirmation/release rules after recovery success or failure.
- Define migration execution contract for recovery-specific migrations.
- Decide how failed execution rolls back migration, reservation, desired state, and actual state.

## Readiness Assessment

- Heartbeat classification: ready for planning.
- Placement reservations: ready for planning, not yet distributed.
- Recovery planning: ready.
- Migration execution: not ready.
- Restore execution: not ready.
- Runtime execution: not ready for recovery.
- Automatic failover: not ready.

## Recommendation

Proceed next with a dedicated Failover Engine Foundation only if it remains planning/execution-coordination focused and does not perform live migration or restore until runtime and backup ownership are defined.

If execution must be deferred further, run a Phase 13.5 audit focused on failover execution ownership, recovery locks, restore boundaries, and migration-to-runtime contracts.
