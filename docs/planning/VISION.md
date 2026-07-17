# Vision

## GamePanel Is Not A Hosting Panel

GamePanel is not a traditional hosting panel whose primary job is to expose a single machine, a daemon, and a container runtime to users.

GamePanel is a cloud-native game workload orchestration platform.

The platform should feel simple to customers, but internally it should behave like an orchestration system: workloads are requested, scheduled, provisioned, observed, reconciled, and moved across infrastructure.

## Product Principle

Users choose regions.

Schedulers choose nodes.

Nodes are infrastructure resources.

Regions are customer-facing resources.

This distinction is foundational. A customer should not need to understand individual nodes, daemon URLs, runtime backends, or host capacity. They should choose where they want a workload to run, and the platform should decide the safest node.

## Core Platform

### Cluster Manager

Owns workload lifecycle orchestration.

Responsibilities:

- Create server workloads.
- Install workloads.
- Start, stop, restart, and delete workloads.
- Coordinate transfers and future failover.
- Reconcile desired state with daemon-reported state.
- Dispatch commands to daemon agents.

### Scheduler

Owns placement decisions.

Responsibilities:

- Choose nodes based on CPU, memory, disk, allocation availability, region, health, and policy.
- Support manual overrides during migration.
- Support predictive placement later.
- Produce explainable placement decisions.

### Runtime Layer

Owns runtime abstraction.

Responsibilities:

- Provide a common interface for workload lifecycle operations.
- Hide Docker, containerd, Podman, Firecracker, and future runtimes behind adapters.
- Normalize stats, logs, console sessions, lifecycle events, and resource limits.

### Node Registry

Owns infrastructure node identity and inventory.

Responsibilities:

- Track node identity, region, labels, status, health, capacity, runtime capabilities, and allocations.
- Accept heartbeats from daemon agents.
- Make node state available to scheduler and cluster manager.

### Event Bus

Owns platform events.

Responsibilities:

- Publish durable events such as `ServerCreated`, `ServerStarted`, `ServerStopped`, `ServerDeleted`, `BackupCreated`, `NodeOnline`, and `NodeOffline`.
- Decouple API requests from background orchestration.
- Enable future observability, automation, billing, and AI operations.

## Optional Modules

These are product modules built on top of the platform. They are not the platform core.

- Customer Portal
- Billing
- Monitoring
- AI Operations
- Ticketing

Optional modules must consume stable platform contracts instead of reaching directly into daemon or runtime internals.

## Long-Term Direction

GamePanel should become a control plane for multiplayer game infrastructure:

- Region-based workload provisioning.
- Automatic node placement.
- Health-aware orchestration.
- Runtime-provider flexibility.
- Durable event history.
- Predictive capacity planning.
- Automated failover.
- First-class observability.

