# Tasks

## Current Phase

Phase 13 - Recovery Coordinator Foundation.

## Current Task

Complete the Recovery Coordinator Foundation without starting Failover Engine, Recovery Execution, Migration Execution, Runtime Actions, or Auto Recovery.

## Completed Work

- Created core documentation foundation:
  - `docs/AI_CONTEXT.md`
  - `docs/VISION.md`
  - `docs/CURRENT_ARCHITECTURE.md`
  - `docs/TARGET_ARCHITECTURE.md`
  - `docs/DOMAIN_MODEL.md`
  - `docs/ROADMAP.md`
  - `docs/DECISIONS.md`
  - `docs/COMPETITOR_ANALYSIS.md`
  - `docs/DEVELOPMENT_RULES.md`
- Added required future-AI reading order to `docs/AI_CONTEXT.md`.
- Added mandatory session closeout rule to `docs/AI_CONTEXT.md`.
- Created process tracking files:
  - `docs/TASKS.md`
  - `docs/WORKLOG.md`
  - `docs/handoffs/TEMPLATE.md`
- Created first timestamped handoff for this process foundation session.
- Added ADR-005 for modular platform architecture.
- Aligned Phase 5 roadmap naming with Runtime Layer.
- Reinforced future-AI startup and closeout workflows in `docs/AI_CONTEXT.md`.
- Created timestamped handoff for this Phase 0 consolidation session.
- Added first-class Region persistence and API endpoints.
- Added additive node-to-region migration.
- Added Node Registry service wrapper.
- Added Cluster Manager skeleton.
- Added manual Scheduler interface.
- Added shared domain models.
- Added preliminary contracts models.
- Refactored node heartbeat, node CRUD, remote node auth, server create, server power, and server delete paths through new service seams.
- Updated Phase 1 ADR and worklog.
- Added computed node capacity snapshots.
- Added region capacity summary.
- Implemented Scheduler V1 filtering by online, draining, maintenance, region, and available resources.
- Implemented Scheduler V1 scoring by free memory, CPU, and disk.
- Moved server creation placement ownership into Cluster Manager.
- Added automatic allocation selection from the placed node when allocation is not supplied.
- Added computed `desiredState` on server responses.
- Updated ADR-006 to define scheduler-owned node selection.
- Added server desired-state and actual-state domain vocabulary.
- Added node desired-state and actual-state domain vocabulary.
- Added Reconciliation Engine service under `apps/api/internal/services/reconciler`.
- Added a default 30-second reconciliation loop.
- Added simple server reconciliation actions for running/stopped/crashed drift.
- Added node refresh and capacity refresh hooks using existing store APIs.
- Added reconciliation observability counters to API metrics.
- Routed server power requests through Cluster Manager's request-oriented power method.
- Added ADR-007 for desired-state architecture.
- Added migration `021_true_state_persistence.sql`.
- Added enum-backed server and node desired/actual state columns.
- Added `state_transitions` table.
- Updated repositories to read persisted desired/actual state.
- Added store helpers for state mutations and transition tracking.
- Updated Cluster Manager to persist desired state before power execution.
- Updated reconciler actual-state refreshes to persist observations.
- Added state transition and server/node reconciliation metrics.
- Added ADR-008 for independent desired/actual state persistence.
- Added `apps/api/internal/events`.
- Added event envelope and core event definitions.
- Added publisher and subscriber interfaces.
- Added in-memory event registry with wildcard subscription support.
- Added event metrics for published, delivered, and handler failures.
- Integrated Cluster Manager event publication for placements, server creation/deletion, power changes, desired state, and actual state.
- Integrated Node Registry event publication for node desired/actual state changes.
- Integrated Reconciler event publication for desired/actual state comparisons.
- Added Event Bus metrics to `/api/v1/metrics`.
- Added ADR-009 for Event Bus foundation.
- Added scheduler placement rejection metrics.
- Added scheduler capacity-exceeded metrics and events.
- Added node lifecycle events for draining and maintenance transitions.
- Added node lifecycle view with health score and placement eligibility.
- Added region cluster view API with capacity and node lifecycle visibility.
- Added draining-node metric.
- Added ADR-010 for node lifecycle management.
- Created `docs/ARCHITECTURE_REVIEW_PHASE7.md`.
- Reviewed domains, services, stores, contracts, events, scheduler, Cluster Manager, Reconciler, and Evacuation Planner.
- Recommended Runtime Abstraction as the next phase.
- Updated `docs/WORKLOG.md` for the review checkpoint.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-2355.md`.
- Created `apps/api/internal/runtime`.
- Added Runtime interface, runtime capability model, runtime registry, Docker adapter, runtime events, and runtime metrics.
- Updated Cluster Manager runtime operations to depend on the Runtime interface.
- Updated Reconciler-facing actual-state refreshes through Cluster Manager runtime abstraction.
- Updated scheduled power tasks to use the Runtime interface.
- Added `docs/RUNTIME_COUPLING_REPORT.md`.
- Added ADR-012 for the Runtime Abstraction Layer.
- Updated roadmap and AI context for Phase 8.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-0030.md`.
- Added migration persistence with `migrations` and `migration_history`.
- Added Migration Store methods for create, list, get, status transitions, history, source lookup, and totals.
- Added Migration Service with create, validate, prepare, execute, cancel, and failure marking.
- Added state-machine-only migration execution with no workload movement.
- Added migration API routes:
  - `POST /api/v1/migrations`
  - `GET /api/v1/migrations`
  - `GET /api/v1/migrations/:id`
  - `PATCH /api/v1/migrations/:id/cancel`
- Added migration events and metrics.
- Added runtime migration contracts with Docker returning `ErrNotImplemented`.
- Added ADR-013 and `docs/MIGRATION_READINESS_REPORT.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-0100.md`.
- Created `docs/ORCHESTRATION_AUDIT_PHASE9.md`.
- Reviewed domains, services, runtime layer, state management, events, and failover readiness.
- Produced an ownership matrix, dependency diagram, technical debt report, and recommended Phase 10 direction.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-0115.md`.
- Added `024_observability_foundation.sql` with timeline, heartbeat history, and health history tables.
- Added durable timeline event storage and correlation queries.
- Added event envelope correlation IDs and context propagation helpers.
- Added Observability Service as an Event Bus wildcard subscriber.
- Added heartbeat history and health history persistence.
- Added observability APIs:
  - `GET /api/v1/timeline`
  - `GET /api/v1/timeline/:resourceType/:resourceId`
  - `GET /api/v1/correlations/:id`
  - `GET /api/v1/nodes/:id/heartbeats`
  - `GET /api/v1/nodes/:id/health-history`
- Added observability metrics:
  - `game_panel_timeline_events_total`
  - `game_panel_correlation_groups_total`
  - `game_panel_heartbeat_failures_total`
  - `game_panel_health_snapshots_total`
- Added correlation propagation for server lifecycle, reconciliation, evacuation, and migration events.
- Added ADR-014 for the Observability Foundation.
- Created `docs/FAILOVER_READINESS_REPORT.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-0145.md`.
- Created `docs/FAILOVER_READINESS_AUDIT.md`.
- Audited heartbeat expiry, placement reservations, migration coordination, restore ownership, event reliability, and recovery-flow readiness.
- Recommended Heartbeat Expiry Engine as the next foundation before failover execution.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-0200.md`.
- Added migration `apps/api/migrations/025_heartbeat_expiry_engine.sql`.
- Added persisted node heartbeat state fields and store read/update helpers.
- Added Heartbeat Monitor service with configurable warning, offline, recovery, and evaluation interval thresholds.
- Implemented heartbeat classification states:
  - `healthy`
  - `suspected`
  - `unreachable`
  - `offline`
  - `recovering`
- Mapped heartbeat classifications to compatible node actual states:
  - `online`
  - `degraded`
  - `offline`
- Moved node actual-state derivation for heartbeat activity out of raw heartbeat storage and into the Heartbeat Monitor.
- Added heartbeat transition events:
  - `NodeSuspected`
  - `NodeUnreachable`
  - `NodeOffline`
  - `NodeRecovered`
- Added heartbeat monitor metrics:
  - `game_panel_heartbeat_evaluations_total`
  - `game_panel_nodes_suspected_total`
  - `game_panel_nodes_offline_total`
  - `game_panel_nodes_recovered_total`
- Recorded health history after heartbeat monitor classification so snapshots reflect derived node actual state.
- Added API endpoint `GET /api/v1/nodes/:id/heartbeat`.
- Confirmed existing `GET /api/v1/nodes/:id/health-history` remains the health-history query surface.
- Added ADR-015 for the Heartbeat Expiry Engine.
- Created `docs/PLACEMENT_RESERVATION_READINESS.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-1452.md`.
- Added migration `apps/api/migrations/026_placement_reservations.sql`.
- Added PlacementReservation domain types, statuses, and request models.
- Added reservation store methods for create, get, list, confirm, cancel, expire, and migration-linked release.
- Added ReservationManager service with lifecycle methods, events, metrics, and periodic expiration.
- Made node capacity snapshots subtract active reservations.
- Made Scheduler placement paths reservation-aware through existing `NodeCapacitySnapshot` usage.
- Added Cluster Manager placement reservation hooks around server creation.
- Added Migration Service target reservations and terminal-state release.
- Added reservation lifecycle events:
  - `ReservationCreated`
  - `ReservationConfirmed`
  - `ReservationExpired`
  - `ReservationCancelled`
- Added reservation metrics:
  - `game_panel_placement_reservations_total`
  - `game_panel_reservation_conflicts_total`
  - `game_panel_reservation_expirations_total`
- Added reservation APIs:
  - `GET /api/v1/reservations`
  - `GET /api/v1/reservations/:id`
- Added ADR-016 for the Placement Reservation Engine.
- Created `docs/RECOVERY_COORDINATOR_READINESS.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-1512.md`.
- Added migration `apps/api/migrations/027_recovery_coordinator.sql`.
- Added RecoveryPlan and RecoveryItem domain models, statuses, and store methods.
- Added Recovery Coordinator service with plan creation, offline-node evaluation, affected-server discovery, scheduler-backed target selection, recovery reservation creation, planned migration record creation, and plan cancellation.
- Added recovery events:
  - `RecoveryPlanCreated`
  - `RecoveryPlanPlanned`
  - `RecoveryPlanFailed`
  - `RecoveryPlanCancelled`
  - `RecoveryItemCreated`
- Added recovery metrics:
  - `game_panel_recovery_plans_total`
  - `game_panel_recovery_items_total`
  - `game_panel_recovery_failures_total`
- Added recovery planning APIs:
  - `POST /api/v1/recovery-plans`
  - `GET /api/v1/recovery-plans`
  - `GET /api/v1/recovery-plans/:id`
- Added ADR-017 for the Recovery Coordinator.
- Created `docs/FAILOVER_ENGINE_READINESS.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-1531.md`.

## Remaining Work

- Keep `TASKS.md` current at the end of every session.
- Append to `WORKLOG.md` at the end of every session.
- Create a new timestamped handoff file at the end of every session.
- Review Phase 13 Recovery Coordinator Foundation.
- Review `docs/FAILOVER_ENGINE_READINESS.md`.
- Preserve Failover Engine, Recovery Execution, Auto Recovery, Runtime Expansion, and AI features for future phases.
- Preserve failover, automatic recovery, live migration, backup transfer, runtime enhancements, NATS, external messaging, deep observability, and AI features for future phases.
- Run formatting/build/test/typecheck only if explicitly requested later.
- Begin the next implementation phase only after explicit approval.

## Blockers

- `gofmt` is not available in the current environment, so touched Go files were manually inspected but not automatically formatted.
- Build, test, lint, typecheck, docker, and validation commands remain prohibited unless explicitly requested.
- Phase 7 implementation exists, but prior process documentation was still reflecting Phase 6 before this checkpoint; confirm Phase 7 closeout documentation if continuity gaps matter before new implementation.
- Runtime abstraction does not yet cover server install, file operations, backup operations, realtime console/logs/stats proxy, daemon heartbeat payloads, or daemon-local Docker runtime internals.
- Migration execution is state-machine-only and does not move servers between nodes.
- Docker migration runtime methods intentionally return `ErrNotImplemented`.
- Observability is persisted in PostgreSQL but Event Bus dispatch is still in-process.
- Restore ownership remains daemon-coupled and out of scope.
- Event Bus remains in-process and should not drive failover execution as the source of truth.
- Reservation expiration runs inside the API process and has no leader election.
- Reservation metrics are process-local counters.
- A small scheduling window remains between scheduler scoring and reservation creation; reservation creation is the atomic conflict gate and may reject stale decisions.
- Recovery Coordinator creates planned migration records and recovery reservations only; it does not execute migrations, restore data, move workloads, or call runtime interfaces.
- Recovery plans are manually/API-created in this phase. Automatic recovery triggering remains out of scope.

## Next Action

Review Phase 13 Recovery Coordinator Foundation. Recommended next step is a Failover Engine readiness review or Failover Engine Foundation, but do not start recovery execution, migration execution, runtime actions, auto recovery, or AI features until explicitly requested.

---

## Session 2026-06-16 — §7.9 Audit Log Gaps Closed

Completed:
- `handlers_servers.go` — `AppendAudit` on suspend, unsuspend, suspension (combined), user delete, user password change.
- `handlers_admin.go` — `AppendAudit` on node token rotation. Added `encoding/json` import.

All §7.9 audit gaps from GamePanel_Master_Handoff.docx are now closed:
- ✅ Node token rotation — `node:token-rotated`
- ✅ User password change — `user:password-changed`
- ✅ Server suspension/unsuspension — `server:suspended` / `server:unsuspended`
- ✅ Admin delete user — `user:deleted`
- ✅ Failed authentication — `auth:login.failed` (prior session)

Remaining from GamePanel_Master_Handoff.docx (do not start without explicit approval):
- §4.6 Migration execution — Phase 15, BLOCKED
- §4.7 Auto recovery trigger — Phase 20, BLOCKED
- §5.x Frontend smoke tests — needs live running environment
- §6.1 SFTP permission enforcement — Phase 17
- §6.6 S3/cross-node backup — Phase 19, blocked on Phase 15
- §3.1 npm audit fix — safe to run when user requests build


---

## Session 2026-06-16 — Master Handoff Doc Gap Closure

Completed:
- Created `.github/workflows/ci.yml` — CI pipeline for api, daemon, frontend (build + test + lint).
- Added `POST /servers/:id/suspend` and `POST /servers/:id/unsuspend` routes in `handlers_servers.go` (admin only, calls `SetServerSuspended`).
- Added failed auth `AppendAudit` call in `server.go` login handler — logs `auth:login.failed` with email and IP on every bad credential attempt.
- Rewrote `infra/nginx/game-panel.conf` with HTTPS server block (TLS 1.2/1.3, HSTS, security headers) and HTTP→HTTPS redirect. Replace `PANEL_FQDN` with actual domain.

Verified already complete (no changes needed):
- `POST /database-hosts/:id/test` — already in `handlers_admin.go`.
- `GetDatabaseHost` + `TestDatabaseHostConnection` — already in `store_databases.go`.
- `DatabaseHost.Password` field — already in `store.go`.
- Rate limiter on `/auth/login` — already in `server.go`.
- `go.mod` Go version — already `1.23.0` on both api and daemon.
- `RotateNodeToken` audit — already in `store_nodes.go`.
- `DeleteUser` audit — already in `store_users.go`.
- `SetServerSuspended` audit — already in `store_servers.go`.
- JWT expiry enforcement — already in `auth.go`.
- Startup secret guard — already in `cmd/api/main.go`.
- `BodyLimit: 100MB` — already in Fiber config in `server.go`.
