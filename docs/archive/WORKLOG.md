# Worklog

Chronological history of project sessions.

Every development session must append:

- Date
- Changes
- Decisions
- Next steps

## 2026-06-14 - Documentation Foundation

Changes:

- Created long-term documentation foundation under `docs/`.
- Defined GamePanel as a cloud-native game workload orchestration platform.
- Documented current architecture, target architecture, domain model, roadmap, ADR system, competitor analysis, and development rules.

Decisions:

- Documentation is the project single source of truth.
- Future AI systems must read `AI_CONTEXT.md` first.
- Builds, tests, lint, typecheck, docker, and validation commands must not run unless explicitly requested.

Next steps:

- Add process files for task tracking, worklog history, and handoffs.
- Begin Phase 1 only after process foundation is in place.

## 2026-06-14 - Process Foundation

Changes:

- Updated `docs/AI_CONTEXT.md` with the required reading order for future AI systems.
- Added mandatory session closeout requirements.
- Created `docs/TASKS.md`.
- Created `docs/WORKLOG.md`.
- Created `docs/handoffs/TEMPLATE.md`.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-2030.md`.

Decisions:

- Every AI session must end by updating `WORKLOG.md`, updating `TASKS.md`, and creating a timestamped handoff file.
- `TASKS.md` is the active project-state tracker.
- `WORKLOG.md` is the chronological history.
- `docs/handoffs/` is the continuity layer between accounts, tools, and sessions.

Next steps:

- Future sessions should start by reading the required documents and latest handoff.
- Phase 1 architecture foundation may begin after this process foundation is accepted.

## 2026-06-14 - Phase 0 Consolidation

Changes:

- Consolidated Phase 0 documentation requirements against the requested structure.
- Updated `docs/AI_CONTEXT.md` with explicit project summary, architecture goals, and development workflow.
- Renamed Phase 5 in `docs/ROADMAP.md` to Runtime Layer.
- Added ADR-005, Modular Platform Architecture, to `docs/DECISIONS.md`.
- Updated `docs/TASKS.md` with the current Phase 0 state.
- Created timestamped handoff `docs/handoffs/HANDOFF-2026-06-14-2045.md`.

Decisions:

- Modular platform architecture is now an accepted ADR.
- Services may begin as internal modules before becoming separately deployable processes.
- Future work must respect documented module boundaries.

Next steps:

- Treat Phase 0 documentation foundation as ready for review.
- Begin Phase 1 - Architecture Foundation only when explicitly requested.

## 2026-06-14 - Phase 1 Multi-Node Foundation

Changes:

- Added shared API domain models under `apps/api/internal/domain`.
- Added first-class Region store repository and additive migration `020_regions_multi_node_foundation.sql`.
- Added Region API endpoints under `/api/v1/regions`.
- Extended Node model with `regionId`, `regionSlug`, and `draining` while preserving legacy `region`.
- Added Node Registry service under `apps/api/internal/services/noderegistry`.
- Added Cluster Manager skeleton under `apps/api/internal/services/clustermanager`.
- Added manual Scheduler interface under `apps/api/internal/services/scheduler`.
- Added preliminary shared contracts under `packages/contracts/models.go`.
- Refactored node CRUD, node heartbeat, remote node auth, remote node reset, server create, server power, and server delete paths to use new service seams.
- Added ADR-006 for manual placement compatibility.
- Updated `AI_CONTEXT.md`, `ROADMAP.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2115.md`.

Decisions:

- Manual node/allocation placement remains the only implemented placement behavior in Phase 1.
- Region support is additive and backward compatible with existing node `region` strings and location records.
- Node maintenance and draining are represented as node statuses but do not implement failover or orchestration.

Next steps:

- Review Phase 1 foundation changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Proceed to Phase 2 - Cluster Manager only after review.

## 2026-06-14 - Phase 2 Cluster Manager V1

Changes:

- Added computed node capacity snapshots from existing node/server tables.
- Added region capacity summaries exposed through Cluster Manager.
- Implemented Scheduler V1 with region-aware node filtering.
- Scheduler V1 filters out offline, maintenance, and draining nodes.
- Scheduler V1 checks available CPU, memory, and disk when capacity is known.
- Scheduler V1 scores by free memory, then CPU, then disk.
- Cluster Manager now owns server creation placement flow.
- Server creation can use region-based placement and still supports manual node/allocation selection.
- Cluster Manager selects the first available allocation on the placed node when no allocation is supplied.
- Added computed `desiredState` field to server responses using existing server status.
- Updated ADR-006 to state that users choose regions and scheduler chooses nodes.
- Updated `ROADMAP.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2145.md`.

Decisions:

- Capacity tracking is computed from existing tables because Phase 2 explicitly prohibits migrations.
- Desired state is exposed as a computed field from current server status until a future migration can add durable desired-state storage.
- Unknown node capacity is treated as unconstrained so existing workflows do not break before nodes report full capacity.

Next steps:

- Review Phase 2 changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Do not begin Event Bus, Failover, Reconciliation Loops, Runtime Abstraction, or AI features.

## 2026-06-14 - Phase 3 Reconciliation Engine V1

Changes:

- Added server desired-state and actual-state vocabulary to API domain, store models, and shared contracts.
- Added node desired-state and actual-state vocabulary to API domain, store models, and shared contracts.
- Exposed computed `actualState` on server responses using existing `servers.status`.
- Exposed computed node desired/actual state using existing `nodes.status`, `maintenance_mode`, and `draining`.
- Added `apps/api/internal/services/reconciler` with a default 30-second reconciliation loop.
- Reconciler refreshes node lists, node capacity snapshots, and server lists through existing store APIs.
- Cluster Manager can probe daemon stats as an actual-state refresh signal for running workloads.
- Reconciler compares simple desired/actual server states and requests start, stop, or restart actions through Cluster Manager.
- Server power requests now flow through `ClusterManager.RequestServerPower`.
- Added reconciliation counters to the existing API metrics endpoint.
- Added ADR-007 for desired-state architecture.
- Updated `AI_CONTEXT.md`, `roadmap.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2215.md`.

Decisions:

- Phase 3 does not add migrations, so dedicated desired-state and actual-state columns remain future work.
- Current desired/actual state is projected from existing status fields to preserve compatibility.
- Maintenance and draining continue to prevent new placement through Scheduler V1; existing servers are not migrated or failed over.

Next steps:

- Review Phase 3 Reconciliation Engine V1 changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Recommended Phase 5 is Event Bus foundation.

## 2026-06-14 - Phase 4 True State Persistence

Changes:

- Added migration `021_true_state_persistence.sql`.
- Added server desired-state and actual-state enum columns.
- Added node desired-state and actual-state enum columns.
- Backfilled server and node state from existing compatibility fields.
- Added `state_transitions` table with resource, state kind, from/to state, reason, and timestamp.
- Updated server and node repositories to read persisted desired/actual state directly.
- Added state mutation helpers that record transitions and keep legacy `status` synchronized.
- Updated Cluster Manager so power requests persist desired state before execution.
- Updated Cluster Manager actual-state refreshes to persist daemon stats observations.
- Updated reconciler metrics with server and node reconciliation totals.
- Added `state_transitions_total` metric from persisted transition records.
- Updated shared contracts with new reconciliation metric fields.
- Added ADR-008 for independent desired/actual state persistence.
- Updated `AI_CONTEXT.md`, `roadmap.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2245.md`.

Decisions:

- Desired and actual state are now durable state columns, not compatibility projections.
- Legacy `status` remains synchronized for existing frontend/API compatibility.
- State transition tracking is intentionally lightweight and database-backed; durable events are deferred to Event Bus.

Next steps:

- Review Phase 4 changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Recommended Phase 5 is Event Bus foundation.

## 2026-06-14 - Phase 5 Event Bus Foundation

Changes:

- Added `apps/api/internal/events`.
- Added event envelope with id, type, timestamp, source, resource type, resource id, and payload.
- Added core event definitions for server lifecycle, node health, placement, desired state, and actual state.
- Added publisher and subscriber interfaces.
- Added an in-memory event registry with wildcard subscription support.
- Added event metrics for published events, delivered events, and handler failures.
- Wired Event Bus into API startup.
- Integrated Cluster Manager event publication for placement decisions, server creation/deletion, server power changes, desired-state changes, and actual-state changes.
- Integrated Node Registry event publication for node desired-state and actual-state changes.
- Integrated Reconciler event publication for desired/actual state comparisons.
- Added Event Bus counters to the existing metrics endpoint.
- Added ADR-009 for Event Bus foundation.
- Updated `AI_CONTEXT.md`, `roadmap.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2315.md`.

Decisions:

- Event Bus is in-process only for Phase 5.
- No NATS, Kafka, Redis Streams, distributed delivery, durability, or replay was introduced.
- Event contracts are now internal API service contracts and can be adapted to durable messaging later.

Next steps:

- Review Phase 5 changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Recommended Phase 6 is Node Lifecycle and Draining.

## 2026-06-14 - Phase 6 Node Lifecycle and Draining

Changes:

- Added node lifecycle event types for draining, maintenance, and capacity exhaustion.
- Scheduler now rejects nodes using persisted actual and desired node state.
- Scheduler records placement rejection and capacity exhaustion metrics.
- Scheduler publishes `NodeCapacityExceeded` events when CPU, memory, or disk capacity blocks placement.
- Node Registry now publishes draining and maintenance lifecycle events when desired state changes.
- Node update requests now accept `desiredState` while preserving `maintenanceMode` and `draining` compatibility.
- Added node health scoring across CPU, memory, disk, heartbeat, and status.
- Added node lifecycle view with capacity, health, draining status, maintenance status, and placement eligibility.
- Added `/api/v1/nodes/:id/lifecycle`.
- Added `/api/v1/regions/:id/cluster`.
- Added metrics for draining nodes, placement rejections, and capacity exhaustion.
- Added ADR-010 for node lifecycle management.
- Updated `AI_CONTEXT.md`, `roadmap.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2345.md`.

Decisions:

- Draining blocks new placement only; existing workloads continue.
- Maintenance blocks new placement but does not migrate or stop workloads.
- Health scoring is an operator visibility signal, not a failover trigger.
- Capacity exhaustion remains scheduler-local and event-published in-process.

Next steps:

- Review Phase 6 changes.
- Run formatting/build/test/typecheck only if explicitly requested.
- Recommended Phase 7 is Runtime Layer foundation.

## 2026-06-14 - Architecture Review Phase 7 Checkpoint

Changes:

- Created `docs/ARCHITECTURE_REVIEW_PHASE7.md`.
- Reviewed current domains, services, stores, contracts, events, scheduler, Cluster Manager, Reconciler, and Evacuation Planner.
- Documented the current control-plane architecture diagram after Phase 7 foundations.
- Identified domain boundary issues, duplicated ownership, event gaps, data-model risks, technical debt, and future-readiness blockers.
- Updated `docs/TASKS.md` for the architecture review checkpoint.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-2355.md`.

Decisions:

- Runtime coupling is the highest-priority architectural blocker before execution-oriented migration or failover work.
- The recommended next phase is Runtime Abstraction.
- Migration Engine, Failover, NATS, and deeper Observability should wait until runtime boundaries are hardened.

Next steps:

- Review `docs/ARCHITECTURE_REVIEW_PHASE7.md`.
- Do not start the next implementation phase until explicitly requested.
- If approved, begin Runtime Abstraction as the next phase.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.

---

## 2026-06-16 — Master Handoff Gap Closure

What was done:
- Audited entire codebase against GamePanel_Master_Handoff.docx.
- Created `.github/workflows/ci.yml` — CI for api (go build+test), daemon (go build+test), frontend (npm ci + build + typecheck + lint). Triggers on push to main and pull_request.
- Added `POST /api/v1/servers/:id/suspend` and `POST /api/v1/servers/:id/unsuspend` in handlers_servers.go. Admin-only. Calls store.SetServerSuspended which already writes AppendAudit.
- Added AppendAudit call on failed login in server.go login handler — logs auth:login.failed with email and IP on every bad credential attempt.
- Rewrote infra/nginx/game-panel.conf: HTTP→HTTPS redirect block + HTTPS server block with TLS 1.2/1.3, ssl_session_cache, HSTS, X-Frame-Options, X-Content-Type-Options, WebSocket upgrade headers, 100MB client_max_body_size. Replace PANEL_FQDN placeholder before deploy.

Verified already implemented (no code changes needed):
- POST /database-hosts/:id/test handler in handlers_admin.go.
- GetDatabaseHost and TestDatabaseHostConnection in store_databases.go.
- DatabaseHost.Password field (json:"-") in store.go.
- limiter middleware on /auth/login in server.go.
- go.mod go 1.23.0 on both api and daemon.
- RotateNodeToken calls AppendAudit in store_nodes.go.
- DeleteUser and UpdateUser call AppendAudit in store_users.go.
- SetServerSuspended calls AppendAudit in store_servers.go.
- JWT exp claim set and enforced in auth.go (parseToken checks claims.Exp <= time.Now().Unix()).
- Production secret guard in cmd/api/main.go.
- BodyLimit: 100MB in Fiber config in server.go.

Blocked (do not implement without explicit phase approval):
- SFTP permission enforcement (section 6.1) — large feature.
- dbprovisioner service (section 4.1) — Phase 16.
- Migration execution (section 4.6) — Phase 15, explicitly blocked by doc.

Build/test commands not run (prohibited unless user requests).

## 2026-06-14 - Phase 8 Runtime Abstraction Foundation

Changes:

- Created `apps/api/internal/runtime`.
- Added provider-neutral Runtime interface, Target, create/power/stats/inspect contracts, and provider constants.
- Added runtime capability model for containers, snapshots, migration, live migration, checkpoints, and resource limits.
- Added Docker adapter that wraps existing daemon client behavior.
- Added Runtime Registry with runtime lookup, capability checks, registration events, unavailable events, capability-change events, and in-memory runtime metrics.
- Added runtime event types: `RuntimeRegistered`, `RuntimeUnavailable`, and `RuntimeCapabilityChanged`.
- Updated API startup to register the Docker runtime provider and pass Runtime interface into Cluster Manager and schedule runner.
- Updated Cluster Manager power/delete/stats paths to use Runtime interface instead of daemon client directly.
- Updated scheduled power tasks to use Runtime interface while leaving backup tasks on the daemon client for later migration.
- Added runtime operation metrics to the metrics endpoint.
- Created `docs/RUNTIME_COUPLING_REPORT.md`.
- Added ADR-012 for the Runtime Abstraction Layer.
- Updated `AI_CONTEXT.md`, `ROADMAP.md`, `roadmap.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-0030.md`.

Decisions:

- Docker remains the only implemented runtime provider.
- The Docker adapter wraps the existing daemon client to preserve behavior.
- Cluster Manager and Reconciler-facing runtime operations now go through the Runtime interface.
- Firecracker, containerd, Podman, Kubernetes, Nomad, Migration Engine, Failover, and AI features remain out of scope.

Next steps:

- Review Phase 8 Runtime Abstraction Foundation.
- Review remaining runtime coupling in `docs/RUNTIME_COUPLING_REPORT.md`.
- Run formatting/build/test/typecheck only if explicitly requested.
- Recommended next phase is Migration Engine foundation after review.

## 2026-06-14 - Phase 9 Migration Engine Foundation

Changes:

- Added migration persistence through `023_migration_engine.sql`.
- Added migration history records for state transitions.
- Added Migration Store methods for create, list, get, status updates, history, server source-node lookup, and totals.
- Added Migration Service with create, validate, prepare, execute, cancel, and failure marking.
- Reused Scheduler and Evacuation Planner capacity validation for migration target selection and validation.
- Added state-machine-only `ExecuteMigration`; it does not move workloads, transfer backups, restore data, or call runtime execution.
- Added runtime migration contracts and kept Docker adapter migration methods returning `ErrNotImplemented`.
- Added migration events and metrics.
- Added migration API routes for create, list, get, and cancel.
- Added ADR-013 for migration architecture.
- Created `docs/MIGRATION_READINESS_REPORT.md`.
- Updated `AI_CONTEXT.md`, `TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-0100.md`.

Decisions:

- Migration is now a durable control-plane resource.
- Migration history records every state transition.
- Phase 9 execution advances state only and intentionally does not perform server movement.
- Automatic failover, recovery, live migration, backup transfer, runtime enhancements, and AI features remain out of scope.

Next steps:

- Review Phase 9 Migration Engine Foundation.
- Do not start Failover or Recovery until explicitly requested.
- Recommended next phase is an architecture review or Failover Readiness checkpoint before any automatic recovery work.

## 2026-06-14 - Phase 9.5 Orchestration Architecture Audit

Changes:

- Created `docs/ORCHESTRATION_AUDIT_PHASE9.md`.
- Reviewed orchestration domains: Cluster, Region, Node, Server, Placement, EvacuationPlan, and Migration.
- Reviewed services: Cluster Manager, Scheduler, Node Registry, Reconciler, Event Bus, Evacuation Planner, and Migration Service.
- Reviewed remaining daemon and Docker coupling after the Runtime Abstraction and Migration Engine foundations.
- Reviewed desired state, actual state, transitions, and migration state ownership.
- Reviewed current event types, missing events, duplicate events, and unused or low-signal events.
- Produced an ownership matrix, dependency diagram, technical debt report, and failover readiness assessment.
- Updated `docs/TASKS.md` for the Phase 9.5 audit checkpoint.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-0115.md`.

Decisions:

- The current architecture is not ready for automatic failover.
- The main blockers are missing durable placement reservations, no heartbeat-expiry/offline detector, state-machine-only migration execution, restore paths outside the Runtime abstraction, process-local observability, and cross-service ownership gaps.
- Recommended Phase 10 is Observability Platform before Failover & Recovery.

Next steps:

- Review `docs/ORCHESTRATION_AUDIT_PHASE9.md`.
- Do not start Phase 10 until explicitly requested.
- If approved, begin Observability Platform as the next phase.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.

## 2026-06-14 - Phase 10 Observability Platform Foundation

Changes:

- Added `apps/api/migrations/024_observability_foundation.sql`.
- Added timeline event domain, store methods, correlation queries, heartbeat history models, and health history models.
- Added event envelope correlation IDs and context helpers for propagation through nested service calls.
- Added Observability Service under `apps/api/internal/services/observability`.
- Registered Observability Service as an Event Bus wildcard subscriber so major in-process events are persisted to the timeline.
- Added idempotent timeline persistence keyed by event ID to avoid duplicate timeline storage.
- Added heartbeat history persistence from node heartbeat API activity.
- Added health history persistence from node heartbeat and capacity snapshots.
- Added observability APIs for timeline, resource timeline, correlation groups, node heartbeat history, and node health history.
- Added observability metrics to `/api/v1/metrics`.
- Propagated correlation IDs through server lifecycle, reconciliation, evacuation, and migration operations.
- Added ADR-014 for the Observability Foundation.
- Created `docs/FAILOVER_READINESS_REPORT.md`.
- Updated `docs/AI_CONTEXT.md`, `docs/ROADMAP.md`, `docs/TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-0145.md`.

Decisions:

- Observability persists Event Bus facts as timeline entries without introducing external messaging or dashboards.
- Correlation IDs are carried through major orchestration flows and can be queried through `/api/v1/correlations/:id`.
- Heartbeat and health history are data collection foundations only; they do not trigger failover.
- Automatic failover, recovery, workload movement, runtime expansion, external messaging, and AI features remain out of scope.

Next steps:

- Review Phase 10 Observability Platform Foundation.
- Review `docs/FAILOVER_READINESS_REPORT.md`.
- Recommended next phase is Failover Readiness Foundations: heartbeat expiry evaluator, recovery trigger policy, placement reservations, migration/reconciliation coordination locks, restore ownership design, and failover dry-run planning.
- Do not start failover or recovery execution until explicitly requested.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.

## 2026-06-14 - Phase 10.5 Failover Readiness Audit

Changes:

- Created `docs/FAILOVER_READINESS_AUDIT.md`.
- Reviewed `docs/FAILOVER_READINESS_REPORT.md`, `docs/ORCHESTRATION_AUDIT_PHASE9.md`, `docs/MIGRATION_READINESS_REPORT.md`, and `docs/AI_CONTEXT.md`.
- Audited heartbeat expiry readiness, placement reservation safety, migration coordination, restore ownership, event reliability, and recovery-flow gaps.
- Assigned readiness scores for heartbeat, migration, recovery, and failover.
- Updated `docs/TASKS.md` for the audit checkpoint.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-0200.md`.

Decisions:

- GamePanel is not ready to proceed to Failover & Recovery execution.
- Current observability is sufficient to support the next design step, but not safe automation.
- Recommended next phase is Heartbeat Expiry Engine before placement reservations, recovery coordination, or failover execution.

Next steps:

- Review `docs/FAILOVER_READINESS_AUDIT.md`.
- If approved, build the Heartbeat Expiry Engine as the next foundation.
- Do not start Failover & Recovery execution, runtime expansion, live migration, or AI features until explicitly requested.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.

## 2026-06-14 - Phase 11 Heartbeat Expiry Engine

Changes:

- Added persisted node heartbeat state through `apps/api/migrations/025_heartbeat_expiry_engine.sql`.
- Added heartbeat state fields to node store models and node read queries.
- Added `apps/api/internal/services/heartbeatmonitor` with configurable warning, offline, recovery, and evaluation interval thresholds.
- Implemented heartbeat classification states: `healthy`, `suspected`, `unreachable`, `offline`, and `recovering`.
- Mapped heartbeat classifications to compatible node actual states: `online`, `degraded`, and `offline`.
- Moved heartbeat-driven node actual-state ownership to the Heartbeat Monitor; Node Registry now records heartbeat facts without directly deriving infrastructure state.
- Added heartbeat transition store updates with state-transition records.
- Added events for `NodeSuspected`, `NodeUnreachable`, `NodeOffline`, and `NodeRecovered`.
- Wired heartbeat transition events through the existing Event Bus so timeline persistence captures heartbeat transitions.
- Recorded heartbeat health snapshots after monitor classification so health history reflects derived node actual state.
- Added heartbeat monitor metrics to `/api/v1/metrics`.
- Added `GET /api/v1/nodes/:id/heartbeat` for current heartbeat inspection and recent heartbeat history.
- Confirmed existing `GET /api/v1/nodes/:id/health-history` remains the health-history API.
- Added ADR-015 for the Heartbeat Expiry Engine.
- Created `docs/PLACEMENT_RESERVATION_READINESS.md`.
- Updated `docs/TASKS.md` and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-1452.md`.

Decisions:

- Heartbeat state remains separate from node actual state.
- Heartbeat Monitor owns classification and compatibility mapping.
- GET heartbeat inspection is read-only; heartbeat POST and the periodic monitor perform persisted evaluations.
- Phase 11 does not implement placement reservations, recovery coordination, failover execution, migration execution, runtime expansion, or AI features.

Next steps:

- Review Phase 11 Heartbeat Expiry Engine.
- Review `docs/PLACEMENT_RESERVATION_READINESS.md`.
- Recommended next foundation is Placement Reservations before Recovery Coordinator or Failover Engine.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.
- `gofmt` was attempted for formatting only, but the command is not installed in this environment.

## 2026-06-14 - Phase 12 Placement Reservation Engine

Changes:

- Added `apps/api/migrations/026_placement_reservations.sql`.
- Added placement reservation domain types, statuses, and request models.
- Added store methods to create, list, get, confirm, cancel, expire, and release migration-linked reservations.
- Added ReservationManager service with lifecycle methods, lifecycle events, metrics, and periodic expiration.
- Updated node capacity snapshots to subtract active reservations from available CPU, memory, and disk.
- Preserved Scheduler ownership of placement while making all scheduler capacity checks reservation-aware through `NodeCapacitySnapshot`.
- Added Cluster Manager placement reservation hooks around server creation.
- Added Migration Service target reservations and release on cancellation, failure, or current state-machine completion.
- Added reservation events: `ReservationCreated`, `ReservationConfirmed`, `ReservationExpired`, and `ReservationCancelled`.
- Added reservation metrics to `/api/v1/metrics`.
- Added read-only reservation APIs:
  - `GET /api/v1/reservations`
  - `GET /api/v1/reservations/:id`
- Added ADR-016 for Placement Reservation Engine.
- Created `docs/RECOVERY_COORDINATOR_READINESS.md`.
- Updated `docs/AI_CONTEXT.md`, `docs/TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-1512.md`.

Decisions:

- Capacity calculations are now Actual Capacity minus Active Reservations.
- Reservation creation is the atomic conflict gate after Scheduler scoring.
- Migration reservations are linked to both migration and server to prevent duplicate active holds for the same workload.
- Reservation lifecycle events are persisted to the timeline through the existing Event Bus observability subscriber.
- Phase 12 does not implement Recovery Coordinator, Failover Engine, Auto Recovery, or migration execution.

Next steps:

- Review Phase 12 Placement Reservation Engine.
- Review `docs/RECOVERY_COORDINATOR_READINESS.md`.
- Recommended next foundation is Recovery Coordinator Foundation with dry-run planning only.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.
- Go files were not automatically formatted because `gofmt` is not installed in this environment.

## 2026-06-14 - Phase 13 Recovery Coordinator Foundation

Changes:

- Added `apps/api/migrations/027_recovery_coordinator.sql`.
- Added RecoveryPlan and RecoveryItem persistence, statuses, store methods, and plan/item listing.
- Added Recovery Coordinator service with:
  - offline-node evaluation
  - affected-server discovery
  - scheduler-backed target selection
  - recovery reservation creation
  - planned migration record creation
  - plan cancellation and cleanup
- Wired Recovery Coordinator into API service construction.
- Added recovery planning APIs:
  - `POST /api/v1/recovery-plans`
  - `GET /api/v1/recovery-plans`
  - `GET /api/v1/recovery-plans/:id`
- Added recovery events: `RecoveryPlanCreated`, `RecoveryPlanPlanned`, `RecoveryPlanFailed`, `RecoveryPlanCancelled`, and `RecoveryItemCreated`.
- Added recovery metrics to `/api/v1/metrics`.
- Recovery events flow through the existing Event Bus and observability subscriber for timeline persistence.
- Added ADR-017 for the Recovery Coordinator.
- Created `docs/FAILOVER_ENGINE_READINESS.md`.
- Updated `docs/AI_CONTEXT.md`, `docs/TASKS.md`, and this worklog.
- Created handoff `docs/handoffs/HANDOFF-2026-06-14-1531.md`.

Decisions:

- Recovery Coordinator creates plans only.
- Recovery Coordinator does not execute recovery, execute migrations, move workloads, restore data, call runtime interfaces, or perform failover.
- Planned migration records created by recovery plans are intent records for future execution phases.
- Recovery reservations hold target capacity so future execution can avoid double placement.

Next steps:

- Review Phase 13 Recovery Coordinator Foundation.
- Review `docs/FAILOVER_ENGINE_READINESS.md`.
- Recommended next phase is a Failover Engine readiness checkpoint or Failover Engine Foundation, but only after explicit approval.
- Build, test, lint, typecheck, docker, and validation commands were intentionally not run.

---

## 2026-06-16 — §7.9 Audit Log Gaps + Security Hardening (New Account Continuation)

Changes:
- `handlers_servers.go` — Added `AppendAudit` calls on:
  - `POST /servers/:id/suspend` → logs `server:suspended`
  - `POST /servers/:id/unsuspend` → logs `server:unsuspended`
  - `POST /servers/:id/suspension` (combined route) → logs `server:suspended` or `server:unsuspended`
  - `PATCH /users/:id` (password change) → logs `user:password-changed` when password is changed
  - `DELETE /users/:id` → logs `user:deleted`
- `handlers_admin.go` — Added `AppendAudit` on `POST /nodes/:id/rotate-token` → logs `node:token-rotated`. Added `encoding/json` import.

Verified already complete from prior sessions (not re-implemented):
- `ServerAccessChecker` interface + `accessChecker()` in `server.go`
- `checkServerPermission` uses `cfg.accessChecker()` in `auth.go`
- 14 subuser permission tests + 3 cross-user/admin/404 tests in `server_test.go`
- `POST /database-hosts/:id/test` wired in `handlers_admin.go`
- DB provisioner fully wired (CreateDatabase/RotatePassword/DeleteDatabase)
- Rate limiting on login (fiber limiter + Redis)
- BodyLimit 100MB in Fiber config
- Startup secret guard in `cmd/api/main.go`
- JSON injection fix on `auth:login.failed`

Blocked (do not implement without explicit phase approval):
- SFTP permission enforcement (§6.1) — Phase 17
- Migration execution (§4.6) — Phase 15, explicitly blocked
- S3/cross-node backup (§6.6) — blocked on Phase 15
- Failover Engine (§4.7) — Phase 14, explicit approval required

Next steps:
- Frontend workflow smoke testing (§5 of handoff doc) — UI needs live verification.
- `npm audit fix` for frontend vulnerabilities (§3.1 Issue 1).
- Daemon security section (§6) verification.

