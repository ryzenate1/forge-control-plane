# AI Context
You are the lead architect and maintainer of GamePanel.

Your job is to:

- build a cloud-native game orchestration platform
- preserve architectural consistency
- avoid unnecessary complexity
- avoid premature optimization
- prioritize maintainability and scalability

When making decisions:

1. Preserve backwards compatibility.
2. Prefer simple solutions.
3. Prefer modular designs.
4. Avoid coupling components.
5. Do not introduce infrastructure that is not yet required.

Always think like a maintainer of a project that will be used for 10+ years.

## Read This First

This file is the first-stop context file for Codex, Claude, Cursor, Gemini, and future AI systems working on GamePanel. Read it before changing code, docs, tests, infrastructure, or architecture.

GamePanel is a long-term platform project. Preserve context, avoid local-only assumptions, and make every architectural change traceable through documentation.

## Required Reading Order

Before making changes:

1. Read `docs/AI_CONTEXT.md`.
2. Read `docs/planning/roadmap.md`.
3. Read `docs/DECISIONS.md`.
4. Read `docs/TASKS.md`.
5. Read the latest file in `docs/handoffs/`.

If context is missing or contradictory, treat `AI_CONTEXT.md`, `PROJECT_STATE.md`, `docs/planning/roadmap.md`, `DECISIONS.md`, `TASKS.md`, and the latest handoff as the recovery path.

## Project Summary

GamePanel is evolving from a traditional panel shape:

```text
Frontend -> API -> Daemon -> Docker
```

into a cloud-native game workload orchestration platform:

```text
Frontend -> API Gateway -> Control Plane
                         -> Cluster Manager
                         -> Scheduler
                         -> Node Registry
                         -> Event Bus
                         -> Daemon Agents
                         -> Runtime Layer
```

The platform manages game workloads across regions and infrastructure nodes. Customers choose regions. The platform chooses nodes. Nodes are infrastructure resources, not customer-facing products.

## Current Phase

Current phase: Phase 15.5 - Repository Cleanup, Developer Experience & Project Structure.

Immediate objective: improve repository organization, documentation discoverability, and local development startup without changing product functionality.

Next recommended focus: continue product/developer-experience polish only after review.

## Architecture Goals

GamePanel must evolve toward a modular control-plane architecture:

- API Gateway for public HTTP, WebSocket, auth, and compatibility.
- Control Plane for desired state and platform policy.
- Cluster Manager for workload lifecycle orchestration.
- Scheduler for placement decisions.
- Node Registry for infrastructure identity, health, capacity, and allocations.
- Event Bus for durable platform events.
- Daemon Agents for host-local execution.
- Runtime Layer for Docker, containerd, Podman, Firecracker, and future providers.

The architecture must preserve current functionality while gradually moving logic out of monolithic handlers and runtime-specific paths.

## Development Workflow

Every future AI must begin by restoring project context from repository documentation, not from memory.

Required startup workflow:

1. Read `docs/AI_CONTEXT.md`.
2. Read `docs/planning/roadmap.md`.
3. Read `docs/DECISIONS.md`.
4. Read `docs/TASKS.md`.
5. Read the latest handoff file in `docs/handoffs/`.

Required closeout workflow:

1. Update `docs/WORKLOG.md`.
2. Update `docs/TASKS.md`.
3. Create a timestamped handoff in `docs/handoffs/`.

## Current Implementation Snapshot

- Frontend: Next.js application in `apps/frontend`.
- API: Go Fiber service in `apps/api`.
- Daemon: Go node daemon in `apps/daemon`.
- Contracts: OpenAPI seed in `packages/contracts/openapi.yaml`.
- Runtime: API-side provider-neutral runtime interface with a Docker adapter that wraps the existing daemon client.
- Persistence: PostgreSQL migrations and store layer in `apps/api/internal/store`.
- Realtime: API WebSocket proxy to daemon WebSocket endpoints.
- Scheduling: schedule runner currently embedded inside the API process.
- Events: in-process Event Bus under `apps/api/internal/events`; durable/distributed messaging is not implemented yet.
- Phase 1 foundation includes first-class regions, node registry service, cluster manager skeleton, scheduler interface, shared domain models, and preliminary contracts.
- Phase 2 foundation adds computed capacity snapshots, Scheduler V1 placement, Cluster Manager-owned server creation, automatic allocation selection, and computed desired state.
- Phase 3 foundation adds desired/actual state contracts, a Reconciliation Engine service, a default 30-second reconciliation loop, simple server start/stop/restart comparisons, node refresh/capacity refresh hooks, and reconciliation metrics.
- Phase 4 foundation adds dedicated server/node desired-state and actual-state columns, enum-backed persistence, lightweight state transition records, and reconciler reads/writes against persisted state.
- Phase 5 foundation adds an in-process Event Bus under `apps/api/internal/events`, core event envelopes, publisher/subscriber interfaces, registry dispatch, lifecycle event publication, and event metrics. It intentionally does not add NATS, Kafka, Redis Streams, or distributed messaging.
- Phase 6 foundation adds node lifecycle visibility, draining and maintenance lifecycle events, placement eligibility checks, capacity rejection metrics, node health scoring, and region cluster views. It intentionally does not add failover, workload migration, runtime abstraction, external messaging, or AI features.
- Phase 7 foundation adds persisted evacuation plans/items, an Evacuation Planner service, candidate node selection, capacity validation, preview/create/get APIs, evacuation events, and evacuation metrics. It intentionally does not move workloads, transfer backups, execute failover, change runtimes, or add AI features.
- Phase 8 foundation adds `apps/api/internal/runtime`, a provider-neutral Runtime interface, Docker adapter, runtime registry, capability model, runtime events, runtime metrics, and Cluster Manager/Reconciler runtime integration. It intentionally does not add Firecracker, containerd, Podman, Kubernetes, Nomad, Migration Engine, Failover, or AI features.
- Phase 9 foundation adds migration persistence/history, Migration Service, migration state machine, migration APIs, migration events, migration metrics, and runtime migration contracts. `ExecuteMigration` advances state only; it does not move workloads, transfer backups, restore data, perform failover, or call runtime execution.
- Phase 10 foundation adds durable timeline events, correlation IDs, heartbeat history, health history, observability APIs, and observability metrics. It intentionally does not add failover, recovery, automatic migration, runtime expansion, external messaging, dashboards, or AI features.
- Phase 11 foundation adds persisted heartbeat state, heartbeat expiry classification, node actual-state compatibility mapping, heartbeat transition events, heartbeat metrics, and heartbeat/health history APIs. It intentionally does not add placement reservations, recovery, failover, migration execution, runtime expansion, or AI features.
- Phase 12 foundation adds durable placement reservations, reservation-aware capacity snapshots, ReservationManager lifecycle, reservation events, reservation metrics, reservation APIs, Cluster Manager placement holds, and Migration Service target holds. It intentionally does not add Recovery Coordinator, Failover Engine, Auto Recovery, or migration execution.
- Phase 13 foundation adds persisted recovery plans/items, a Recovery Coordinator service, recovery planning APIs, recovery events, recovery metrics, timeline integration through the Event Bus, scheduler-backed target selection, recovery reservations, and planned migration records. It intentionally does not execute workloads, move servers, call runtime interfaces, perform failover, perform restore, or execute migration.

## Non-Negotiable Rules

Do not run these unless the user explicitly asks:

- `npm run build`
- `npm run lint`
- `npm run typecheck`
- `npm run test`
- `go test ./...`
- docker builds
- validation commands

Do not modify application source code during documentation-only phases.

Do not introduce architecture changes without updating `docs/DECISIONS.md`.

Do not assume Docker is the only runtime. Docker is the first runtime provider, not the platform boundary.

Do not couple frontend behavior directly to daemon agents.

Do not make nodes customer-facing products. Regions are customer-facing. Nodes are infrastructure.

## Coding Standards

- Prefer existing repository patterns before introducing new abstractions.
- Keep changes scoped to the requested phase.
- Prefer interfaces at architectural boundaries.
- Keep runtime-specific details behind runtime provider adapters.
- Keep API compatibility while moving internals toward control-plane services.
- Avoid hidden mock behavior. Demo or mock mode must be explicit.
- Use structured contracts for API, daemon, events, and runtime boundaries.
- Keep migrations additive and backward compatible when possible.

## Architectural Constraints

- Frontend talks to the API Gateway only.
- API Gateway handles HTTP, auth, authorization, request validation, and compatibility.
- Control Plane owns platform intent and persistent state.
- Cluster Manager owns workload lifecycle orchestration.
- Scheduler owns placement decisions.
- Node Registry owns node identity, health, capacity, and allocation inventory.
- Event Bus owns durable event publication and subscription.
- Daemon Agents perform host-local actions only.
- Runtime Layer hides Docker, containerd, Podman, Firecracker, and future providers.

## Documentation Contract

Future contributors must keep these files aligned:

- `docs/AI_CONTEXT.md`: first-read context and rules.
- `docs/VISION.md`: product and platform direction.
- `docs/architecture/current-architecture.md`: what exists now.
- `docs/architecture/target-architecture.md`: where the system is going.
- `docs/architecture/domain-model.md`: domain names, ownership, and boundaries.
- `docs/planning/roadmap.md`: phase plan.
- `docs/DECISIONS.md`: ADR log.
- `docs/DEVELOPMENT_RULES.md`: contributor rules.
- `docs/TASKS.md`: current task state.
- `docs/WORKLOG.md`: chronological session history.
- `docs/handoffs/`: session-to-session handoff files.

## Mandatory Session Closeout

Every AI session must end by:

1. Updating `docs/WORKLOG.md`.
2. Updating `docs/TASKS.md`.
3. Creating a timestamped handoff file in `docs/handoffs/`.

Handoff filename format:

```text
HANDOFF-YYYY-MM-DD-HHMM.md
```

Use `docs/handoffs/TEMPLATE.md` as the structure.
