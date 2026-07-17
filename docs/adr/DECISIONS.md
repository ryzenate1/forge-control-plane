# Architecture Decisions

## ADR System

All significant architecture decisions must be recorded here.

Use this format:

```text
## ADR-000 Title

Status: Proposed | Accepted | Superseded
Date: YYYY-MM-DD

Context:
Decision:
Rationale:
Consequences:
```

Do not make architecture changes without adding or updating an ADR.

## ADR-001 Regions Are First-Class Domains

Status: Accepted
Date: 2026-06-14

Context:
Traditional hosting panels often expose nodes directly. GamePanel needs to become a platform where users select desired geography or market location without understanding infrastructure topology.

Decision:
Regions are first-class customer-facing domains.

Rationale:
Regions provide a stable product and UX abstraction. They let the platform change node inventory, capacity, runtime providers, and failover behavior without forcing customers to choose specific machines.

Consequences:
Users choose regions. Scheduler chooses nodes. UI and API should increasingly expose region selection instead of node selection for customer workflows.

## ADR-002 Nodes Are Infrastructure Resources

Status: Accepted
Date: 2026-06-14

Context:
Nodes currently exist in the API and admin UI, but server creation can require manual node selection. That is useful for admin control but not ideal as the long-term customer model.

Decision:
Nodes are infrastructure resources owned by the platform.

Rationale:
Nodes represent daemon agents, runtime capacity, network allocations, health, labels, and operational state. They should be managed by operators and consumed by schedulers.

Consequences:
Customer-facing flows should not require node selection. Admin flows may still expose nodes for operations, debugging, and capacity management.

## ADR-003 Runtime Abstraction Required

Status: Accepted
Date: 2026-06-14

Context:
Docker is the current runtime. The platform must support future runtime providers such as containerd, Podman, and Firecracker.

Decision:
GamePanel requires a provider-neutral runtime abstraction.

Rationale:
Runtime-specific assumptions in control-plane code would block future providers. Docker-specific behavior should live in a Docker adapter.

Consequences:
New runtime-facing code must use provider-neutral concepts such as instance, resource limits, mounts, network bindings, stats, logs, and runtime events. Docker fields may remain for compatibility but should not be expanded as platform primitives.

## ADR-004 Event-Driven Architecture

Status: Accepted
Date: 2026-06-14

Context:
The current system relies on direct calls, audit rows, schedule notifications, and local Docker events. Long-term features need durable event flow.

Decision:
GamePanel will use an event-driven architecture for platform lifecycle facts.

Rationale:
Events decouple the API, cluster manager, scheduler, node registry, observability, billing, AI operations, and future modules. Events also create an auditable timeline of platform behavior.

Consequences:
Core lifecycle actions must publish typed events. Future modules should subscribe to events rather than reaching directly into daemon agents or internal tables.

## ADR-005 Modular Platform Architecture

Status: Accepted
Date: 2026-06-14

Context:
The current repository is organized as frontend, API, daemon, contracts, and infrastructure. This is sufficient for the MVP, but long-term development requires clearer platform boundaries so multiple AI systems and contributors can work safely without collapsing all logic into the API or daemon.

Decision:
GamePanel will use a modular platform architecture centered on API Gateway, Control Plane, Cluster Manager, Scheduler, Node Registry, Event Bus, Daemon Agents, and Runtime Layer.

Rationale:
Separate modules make ownership clear. They allow the project to preserve API compatibility while extracting orchestration, scheduling, node health, eventing, and runtime behavior into focused boundaries. This also improves context preservation because future contributors can reason from documented module responsibilities.

Consequences:
New work should align with module boundaries even when implemented inside existing deployables at first. Services may start as internal packages before becoming separately deployable processes. Contributors must not add new cross-cutting orchestration logic directly into HTTP handlers when a documented module boundary exists.

## ADR-006 Scheduler Chooses Nodes

Status: Accepted
Date: 2026-06-14

Context:
GamePanel currently supports explicit `nodeId` and `allocationId` server creation. The target architecture requires customer-facing region selection while infrastructure node choice belongs to the platform scheduler.

Decision:
Users choose regions. Scheduler chooses nodes. Manual node selection remains supported as an admin/backwards-compatibility override during migration.

Rationale:
Regions are the stable customer-facing abstraction. Nodes are infrastructure resources whose health, capacity, draining state, and maintenance state must influence placement. Keeping manual selection as an override protects existing workflows while the scheduler matures.

Consequences:
Cluster Manager owns server creation placement flow and asks Scheduler for a placement decision. Scheduler V1 filters nodes by region, online status, maintenance/draining flags, and available resources, then scores nodes by free memory, CPU, and disk. Later phases may replace scoring without changing the API route shape.

## ADR-007 GamePanel Uses Desired-State Architecture

Status: Accepted
Date: 2026-06-14

Context:
GamePanel historically performs imperative actions from handlers to daemon agents. Long-term orchestration needs a control-plane model where user intent and observed infrastructure state can diverge temporarily, then converge through reconciliation.

Decision:
GamePanel will use desired-state architecture for workload and node lifecycle management.

Rationale:
Desired state lets handlers request intent while Cluster Manager and Reconciliation Engine own execution. This enables retries, drift correction, future failover, event publication, and automated operations without coupling UI requests directly to daemon commands.

Consequences:
Server and node models now expose desired-state and actual-state vocabulary. The first reconciler compares simple running/stopped/crashed cases and routes corrective actions through Cluster Manager. Because Phase 3 prohibits migrations, the current implementation maps desired/actual state from existing status fields; dedicated persisted columns remain future work.

## ADR-008 Desired and Actual State Are Persisted Independently

Status: Accepted
Date: 2026-06-14

Context:
Phase 3 introduced desired/actual state vocabulary and a reconciler, but state was projected from legacy `status` fields for compatibility. That prevented durable divergence between requested intent and observed infrastructure state.

Decision:
Server and node desired state and actual state are persisted independently in dedicated enum-backed columns. State transitions are recorded in a lightweight transition table.

Rationale:
Independent persistence lets handlers record user intent without pretending infrastructure has already converged. The reconciler can compare durable desired and actual state, act through Cluster Manager, and persist updated observations. Transition records provide a foundation for auditability, debugging, event publication, and future failover.

Consequences:
Cluster Manager owns server desired-state mutations. Node desired and actual state are stored separately. Legacy `status` remains synchronized for compatibility, but new orchestration code reads `desired_state` and `actual_state` directly. Event Bus, failover, runtime abstraction, and auto migration remain future phases.

## ADR-009 Event-Driven Architecture Foundation

Status: Accepted
Date: 2026-06-14

Context:
GamePanel needs event-driven boundaries so future modules can react to lifecycle facts without direct coupling to HTTP handlers, store internals, daemon agents, or scheduler implementation details. The project is not ready for distributed messaging infrastructure.

Decision:
GamePanel will start with an in-process Event Bus inside the API service. The foundation includes an event envelope, core event types, publisher/subscriber interfaces, an in-memory registry, and event metrics.

Rationale:
An in-process bus establishes event contracts and publishing discipline without introducing NATS, Kafka, Redis Streams, or operational complexity. It lets Cluster Manager, Node Registry, Reconciler, and future internal modules communicate through events while preserving the option to replace the implementation later.

Consequences:
Core lifecycle paths now publish events such as server creation, power changes, node health changes, placement decisions, and state observations. Event delivery is synchronous and process-local. Events are not durable, distributed, replayable, or cross-process yet.

## ADR-010 Node Lifecycle Management

Status: Accepted
Date: 2026-06-14

Context:
Nodes are infrastructure resources that need operational lifecycle controls. Operators must be able to prevent new placement on a node without disrupting existing workloads, place nodes into maintenance, and understand health and placement eligibility.

Decision:
GamePanel will manage nodes through persisted desired state and actual state. `active`, `maintenance`, and `draining` desired states control placement eligibility. Actual state reflects observed node health. Scheduler must reject offline, maintenance, draining, and capacity-exhausted nodes.

Rationale:
Draining and maintenance are foundational operations for multi-node orchestration. They allow safe capacity management and future failover/migration planning while preserving existing workloads during this phase.

Consequences:
Nodes in draining or maintenance receive no new workloads. Existing workloads are not migrated or failed over. Health scoring and cluster visibility are exposed through API surfaces. Lifecycle events and metrics create an operator-facing foundation without external messaging or runtime abstraction changes.

## ADR-012 Runtime Abstraction Layer

Status: Accepted
Date: 2026-06-14

Context:
The architecture review after Phase 7 identified runtime coupling as the highest-priority blocker. Cluster Manager and Reconciler still depended on daemon/Docker-shaped behavior for power operations and runtime stats. Migration Engine and Failover would amplify that coupling if implemented before a provider-neutral boundary.

Decision:
GamePanel will introduce an API-side Runtime abstraction under `apps/api/internal/runtime`. Docker remains the only implemented provider for now, represented by a Docker adapter that wraps existing daemon behavior. Cluster Manager and Reconciler-facing execution paths must depend on the Runtime interface instead of Docker or daemon implementation details.

Rationale:
Runtime-neutral orchestration is required before safe migration or failover execution. The interface lets future providers such as containerd, Podman, and Firecracker implement create, delete, start, stop, restart, stats, exists, and inspect operations without rewriting control-plane services.

Consequences:
The runtime registry owns provider lookup, capability checks, registration events, unavailable events, capability-change events, and runtime operation metrics. Docker-specific behavior is isolated to the Docker adapter and daemon internals. Remaining daemon/file/backup/console coupling is documented in `docs/RUNTIME_COUPLING_REPORT.md` and should be migrated deliberately in later phases.

## ADR-013 Migration Architecture

Status: Accepted
Date: 2026-06-14

Context:
Evacuation planning can identify workloads and candidate target nodes, but the platform needs a durable migration object before it can safely execute workload movement in later phases. Runtime migration contracts also need to exist before provider-specific movement is implemented.

Decision:
GamePanel will represent migration as a durable control-plane resource with source node, target node, status, and history. Phase 9 introduces a Migration Service, state machine, events, metrics, and APIs. Migration execution in this phase advances state only and does not move workloads, transfer backups, restore data, or perform failover.

Rationale:
Persisting migration intent and history creates a safe foundation for future execution. Keeping execution non-operational avoids accidental failover or data movement while still proving API, service, store, event, and runtime boundaries.

Consequences:
Future phases can attach archive/restore, runtime execution, placement reservation, and rollback behavior to the migration state machine. Docker migration methods currently return `ErrNotImplemented`, and cross-runtime/live migration remains out of scope.

## ADR-014 Observability Foundation

Status: Accepted
Date: 2026-06-14

Context:
Phase 9.5 found that GamePanel is not ready for automatic failover because orchestration facts are difficult to trace across placement, reconciliation, migration, evacuation, runtime operations, and node health. Events and metrics existed, but event delivery was process-local and there was no durable timeline or correlation model.

Decision:
GamePanel will persist a control-plane observability foundation before implementing failover or recovery. The API service records timeline events from the in-process Event Bus, carries correlation IDs across major orchestration operations, stores node heartbeat history, stores node health history, and exposes timeline/correlation APIs.

Rationale:
Failover and recovery require explainable decisions. Operators and future automation need to understand why a node was considered unhealthy, which reconciliation run acted on a server, which migration or evacuation plan selected a target, and how related events fit together. A durable observability layer makes future automation safer without introducing dashboards or external telemetry infrastructure in this phase.

Consequences:
Timeline data is now persisted in PostgreSQL through additive tables. Events remain in-process, but an observability subscriber records each event once using event IDs for idempotency. Correlation IDs are carried through server lifecycle, reconciliation, evacuation, and migration flows. Heartbeat and health history are queryable through API endpoints. This phase does not implement failover, recovery, automatic migration, runtime expansion, external messaging, or AI features.

## ADR-015 Heartbeat Expiry Engine

Status: Accepted
Date: 2026-06-14

Context:
Phase 10.5 found that failover cannot safely begin until the platform can distinguish a healthy node from a delayed, unreachable, offline, or recovering node. Existing node actual state only supports `online`, `degraded`, and `offline`, which is useful for compatibility but too coarse for recovery policy.

Decision:
GamePanel will persist heartbeat state separately from node actual state. Heartbeat state uses `healthy`, `suspected`, `unreachable`, `offline`, and `recovering`. Node actual state remains compatible with `online`, `degraded`, and `offline`, and is derived from heartbeat evaluation.

Rationale:
Separating heartbeat classification from node actual state gives future failover logic a richer signal without breaking existing API consumers or scheduler behavior. The heartbeat monitor can use warning, offline, and recovery thresholds to classify node reachability before any recovery execution exists.

Consequences:
Node Registry records heartbeat facts, while the Heartbeat Monitor owns heartbeat classification and compatibility mapping to node actual state. Heartbeat transitions publish events and are persisted to the timeline through the existing Event Bus observer. This phase does not implement placement reservations, recovery coordination, failover execution, or migration execution.

## ADR-016 Placement Reservation Engine

Status: Accepted
Date: 2026-06-14

Context:
Scheduler V1 could filter and score nodes by apparent capacity, but capacity was advisory. Concurrent server creation, migration planning, evacuation planning, and future recovery work could choose the same remaining resources before any workload existed on the target node.

Decision:
GamePanel will introduce durable placement reservations. Capacity calculations must be Actual Capacity minus Active Reservations. Reservations are persisted with node, optional server, optional migration, type, CPU, memory, disk, status, expiry, and lifecycle timestamps.

Rationale:
Reservations create an atomic control-plane hold between scheduling and execution. They prevent double placement, capacity races, migration conflicts, and future recovery conflicts without implementing failover or workload movement in this phase.

Consequences:
Scheduler-facing capacity snapshots subtract active reservations. Cluster Manager creates and completes short-lived placement reservations around server creation. Migration Service creates target-node reservations for migration intent and releases them on cancellation, failure, or state-machine completion. Reservation lifecycle events are published and persisted by the existing observability subscriber. This phase does not implement Recovery Coordinator, Failover Engine, Auto Recovery, or migration execution.

## ADR-017 Recovery Coordinator

Status: Accepted
Date: 2026-06-14

Context:
After heartbeat expiry and placement reservations, GamePanel can classify offline nodes and reserve target capacity safely. The platform still needs a control-plane owner for recovery planning before any failover or restore execution can exist.

Decision:
GamePanel will introduce a Recovery Coordinator that creates durable recovery plans and recovery items for offline nodes. The coordinator identifies affected servers, asks the Scheduler for target nodes, creates recovery reservations, and creates planned migration records. It does not execute migrations, restore data, move workloads, call runtime interfaces, or perform automatic failover.

Rationale:
Recovery planning must be explicit and auditable before execution is allowed. Keeping planning separate from execution prevents accidental workload movement while giving operators and future automation a durable plan, capacity holds, migration intent, events, metrics, and timeline history.

Consequences:
Recovery plans become the handoff point between node failure detection and future failover execution. The Recovery Coordinator depends on Store, Scheduler, ReservationManager, and Event Bus only. Future failover work must consume recovery plans rather than re-discovering failed-node workloads independently.
