# Roadmap

## Phase 0 - Documentation Foundation

Goal: make documentation the single source of truth for long-term development.

Deliverables:

- `AI_CONTEXT.md`
- `VISION.md`
- `CURRENT_ARCHITECTURE.md`
- `TARGET_ARCHITECTURE.md`
- `DOMAIN_MODEL.md`
- `ROADMAP.md`
- `DECISIONS.md`
- `COMPETITOR_ANALYSIS.md`
- `DEVELOPMENT_RULES.md`

Exit criteria:

- Future AI systems know what to read first.
- Current and target architecture are documented.
- Domain language is stable.
- ADR process exists.

## Phase 1 - Architecture Foundation

Goal: create boundaries without breaking existing functionality.

Deliverables:

- Control-plane service interfaces.
- Cluster manager interface.
- Scheduler interface.
- Node registry interface.
- Event bus interface.
- Runtime-neutral interface design.
- Shared contract direction.
- First-class Region domain.
- Manual-placement compatibility layer.

Exit criteria:

- API compatibility remains intact.
- New boundaries exist as architecture, not feature rewrites.
- Existing manual node/allocation placement remains supported.

## Phase 2 - Cluster Manager

Goal: move lifecycle orchestration out of HTTP handlers.

Deliverables:

- Server lifecycle service.
- Daemon agent command abstraction.
- Desired-state and actual-state reconciliation design.
- Install, create, power, delete orchestration boundaries.
- Region-aware placement.
- Node capacity snapshots.
- Placement decisions.
- Desired-state foundation.

Exit criteria:

- API handlers delegate lifecycle work.
- Direct daemon coupling is isolated.
- Users can create servers by region while manual node selection remains compatible.
- Cluster Manager owns server creation placement flow.

## Phase 3 - Reconciliation Engine

Goal: introduce desired-state reconciliation as a first-class platform capability.

Deliverables:

- Server desired-state and actual-state models.
- Node desired-state and actual-state models.
- Reconciliation service under the API control-plane boundary.
- Configurable reconciliation loop with a 30-second default interval.
- Node health and capacity refresh hooks.
- Server status sync hooks.
- Observability counters for reconciliation and refresh failures.

Exit criteria:

- Cluster Manager owns power execution for handler requests.
- Reconciler can compare desired and actual state and request simple corrective actions.
- Maintenance and draining nodes remain excluded from new placement.
- Existing server power and placement behavior remains compatible.

## Phase 4 - True State Persistence

Goal: persist desired and actual state independently for servers and nodes.

Deliverables:

- Server `desired_state` and `actual_state` persistence.
- Node `desired_state` and `actual_state` persistence.
- Enum-backed state columns.
- Lightweight state transition records.
- Cluster Manager ownership of desired-state mutations.
- Reconciler reads desired/actual state from durable columns.
- Reconciler persists actual-state refreshes and corrective actions.
- Metrics for state transitions and server/node reconciliation.

Exit criteria:

- Compatibility mappings from legacy `status` are replaced by persisted state reads.
- Legacy `status` remains synchronized for existing behavior.
- State transitions are queryable from the database.

## Phase 5 - Event Bus Foundation

Goal: introduce internal event-driven architecture inside the API process.

Deliverables:

- Event envelope.
- Event schemas.
- Event publisher interface.
- Event subscriber interface.
- In-memory event registry.
- Core events for server, node, placement, desired-state, and actual-state changes.
- Event publication from Cluster Manager, Node Registry, and Reconciler.
- Event metrics.
- Initial events: `ServerCreated`, `ServerStarted`, `ServerStopped`, `ServerRestarted`, `ServerDeleted`, `NodeOnline`, `NodeOffline`, `NodeDegraded`, `PlacementCreated`, `DesiredStateChanged`, `ActualStateChanged`.

Exit criteria:

- Core platform actions publish in-process events.
- Future modules can subscribe inside the API process without direct DB or daemon coupling.
- No external messaging infrastructure is introduced.

## Phase 6 - Node Lifecycle and Draining

Goal: make nodes manageable infrastructure resources.

Deliverables:

- Persisted node desired states: active, maintenance, draining.
- Scheduler placement blocking for offline, maintenance, and draining nodes.
- Draining behavior that prevents new placement while existing workloads continue.
- Capacity enforcement for CPU, memory, and disk exhaustion.
- Node health scoring across CPU, memory, disk, heartbeat, and status.
- Cluster view API exposing region capacity, node capacity, node health, draining status, and placement eligibility.
- Node lifecycle events for draining, maintenance, and capacity exhaustion.
- Metrics for draining nodes, placement rejections, and capacity exhaustion.

Exit criteria:

- Nodes can be placed into draining or maintenance without receiving new workloads.
- Scheduler refuses ineligible or capacity-exhausted nodes.
- Operators can inspect node health and placement eligibility through API responses.

## Phase 7 - Evacuation Planner Foundation

Goal: plan workload evacuation without moving workloads.

Deliverables:

- Evacuation plan and item domains.
- Evacuation Planner service.
- Candidate node selection using Scheduler.
- Capacity validation for planned targets.
- Preview/create/get evacuation APIs.
- Evacuation planning events and metrics.

Exit criteria:

- Operators can preview which workloads would need to move from a node.
- Plans identify eligible target nodes without executing migration.
- No workload movement, backup transfer, failover, or runtime change is performed.

## Phase 8 - Runtime Abstraction

Goal: move from Docker-shaped runtime to provider-neutral runtime.

Deliverables:

- Runtime interface.
- Docker adapter wrapping existing daemon behavior.
- Runtime registry.
- Runtime capabilities model.
- Runtime stats model.
- Runtime events.
- Runtime operation and capability metrics.
- Runtime coupling report.

Exit criteria:

- Docker remains supported.
- Cluster Manager and Reconciler depend on Runtime interface, not Docker or daemon implementation details.
- New providers can be added without rewriting orchestration services.

## Phase 9 - Migration Engine

Goal: execute planned workload movement between eligible nodes.

Deliverables:

- Migration execution state machine.
- Backup/archive transfer handoff.
- Target runtime creation.
- Placement reservation and commit.
- Migration progress events.

Exit criteria:

- Evacuation plans can become executable migrations.
- Runtime-neutral execution paths are used.
- Failover remains a later phase.

## Phase 10 - Observability Platform Foundation

Goal: persist the operational visibility required for future failover, recovery, migration coordination, and capacity planning.

Deliverables:

- Durable timeline events.
- Correlation IDs across orchestration operations.
- Node heartbeat history.
- Node health history.
- Timeline and correlation APIs.
- Observability metrics.
- Failover readiness report.

Exit criteria:

- Operators and future automation can trace server lifecycle, migration, evacuation, reconciliation, node heartbeat, and node health history.
- No failover, recovery, automatic migration, runtime expansion, or AI features are implemented.

## Phase 11 - Failover Readiness Foundations

Goal: close the remaining safety gaps before automatic failover execution.

Deliverables:

- Heartbeat expiry evaluator.
- Recovery trigger design.
- Placement reservations.
- Migration coordination locks.
- Restore ownership design.
- Failover dry-run planning.

Exit criteria:

- Platform can detect failed nodes and produce a safe recovery plan without executing automatic recovery.

## Phase 12 - Observability Expansion

Goal: provide platform-level visibility.

Deliverables:

- Standard telemetry package.
- Node health dashboards.
- Workload lifecycle metrics.
- Scheduler decision logs.
- Event stream inspection.

Exit criteria:

- Operators can understand system health and placement behavior.

## Phase 13 - Optional Modules

Goal: build product modules on top of stable platform contracts.

Candidate modules:

- Customer Portal
- Billing
- Monitoring
- AI Operations
- Ticketing

Exit criteria:

- Optional modules consume platform APIs/events.
- Optional modules do not couple directly to daemon or runtime internals.
