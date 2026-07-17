# Domain Model

## Principles

GamePanel domain names must remain stable across code, API contracts, docs, and future AI work.

Customer-facing concepts should not leak infrastructure details.

Infrastructure concepts should not become product SKUs by accident.

## Cluster

A cluster is a collection of regions, nodes, schedulers, daemon agents, and runtime capacity managed by one control plane.

Owner: Control Plane.

Boundaries:

- Cluster is the operational unit for orchestration.
- Cluster contains infrastructure.
- Cluster is not directly managed by end users in normal flows.

Relationships:

- Has many regions.
- Has many nodes.
- Emits events.
- Provides capacity to workloads.

## Region

A region is a customer-facing placement choice.

Owner: Control Plane and Scheduler.

Boundaries:

- Users choose regions.
- Regions map to one or more nodes.
- Regions may have labels, capacity policy, pricing policy, and availability state.

Relationships:

- Contains or maps to nodes.
- Used by scheduler as a placement constraint.
- Exposed in customer portal and admin UI.

## Node

A node is an infrastructure resource running a daemon agent.

Owner: Node Registry.

Boundaries:

- Nodes are not customer-facing products.
- Nodes report health, capacity, allocations, runtime capabilities, and heartbeat state.
- Nodes execute workloads only through daemon agent commands.

Relationships:

- Belongs to a region.
- Has allocations.
- Hosts workloads.
- Reports runtime state.

## Server

A server is the user-visible game server instance.

Owner: Control Plane.

Boundaries:

- Server is the customer-facing representation.
- Server has desired lifecycle state.
- Server maps to a workload under the hood.
- Server should not expose runtime-provider details directly.

Relationships:

- Owned by a user.
- Placed in a region.
- Assigned to a node by scheduler.
- Has one or more allocations.
- Produces events.

## Allocation

An allocation is a network endpoint reservation.

Owner: Node Registry and Scheduler.

Boundaries:

- Allocation belongs to node infrastructure.
- Allocation may be assigned to one server.
- Allocation selection should eventually be automatic.

Relationships:

- Belongs to a node.
- May be attached to a server.
- Used by scheduler for placement.

## Runtime

A runtime is a provider that executes workloads on a node.

Owner: Runtime Layer.

Boundaries:

- Runtime providers are implementation details.
- Docker is one provider.
- Runtime-specific assumptions must not leak into control-plane domain models.

Relationships:

- Runs on a node.
- Executes workload instances.
- Emits runtime events.
- Reports stats and capabilities.

## Event

An event is a durable fact that something happened.

Owner: Event Bus.

Boundaries:

- Events are immutable.
- Events describe facts, not requests.
- Events must include type, version, source, subject, time, and payload.

Relationships:

- Produced by API, control plane, scheduler, cluster manager, node registry, and daemon agents.
- Consumed by observability, billing, automation, AI operations, and future modules.

## Workload

A workload is the schedulable execution unit for a game server.

Owner: Cluster Manager and Scheduler.

Boundaries:

- Workload is infrastructure-facing.
- Server is user-facing.
- A server may map to one active workload.
- Future failover may create replacement workloads.

Relationships:

- Belongs to a server.
- Placed on a node.
- Executed by a runtime provider.
- Uses allocations, mounts, environment, limits, and configuration.
