# Development Rules

## Scope Rules

- No feature work outside the current phase.
- No architecture changes without an ADR in `docs/DECISIONS.md`.
- No documentation drift. If behavior changes, update docs in the same phase.
- No source-code changes during documentation-only tasks.

## Command Rules

Do not run these unless explicitly requested by the user:

- `npm run build`
- `npm run lint`
- `npm run typecheck`
- `npm run test`
- `go test ./...`
- docker builds
- validation commands

When the user requests no commands, do not use shell commands.

## Architecture Rules

- Frontend talks to API Gateway only.
- API Gateway must not become the long-term home for orchestration logic.
- Control Plane owns desired state.
- Cluster Manager owns lifecycle orchestration.
- Scheduler owns placement.
- Node Registry owns node health, capacity, and allocation inventory.
- Event Bus owns durable platform events.
- Daemon Agents execute host-local commands.
- Runtime Layer hides provider-specific behavior.

## Runtime Rules

- No runtime-specific assumptions in control-plane code.
- Docker is a provider, not the platform.
- Prefer provider-neutral terms:
  - workload
  - instance
  - resource limits
  - network binding
  - mount
  - runtime event
  - runtime capabilities
- Keep Docker-specific fields behind adapters or compatibility layers.

## Node And Region Rules

- Users choose regions.
- Schedulers choose nodes.
- Nodes are infrastructure resources.
- Regions are customer-facing resources.
- Admin UI may expose nodes for operations.
- Customer flows should move toward region selection.

## Coupling Rules

- No direct frontend-to-daemon coupling.
- No optional module should call daemon agents directly.
- Avoid direct daemon coupling in API handlers.
- Prefer service interfaces between gateway, control plane, scheduler, cluster manager, node registry, event bus, and runtime layer.

## Contract Rules

- Shared contracts belong in `packages/contracts`.
- Event schemas belong in `packages/events`.
- Runtime contracts belong in `packages/runtime`.
- Telemetry conventions belong in `packages/telemetry`.
- Do not duplicate request/response models across frontend, API, and daemon without a migration plan.

## Implementation Rules

- Preserve existing functionality while introducing architecture boundaries.
- Prefer additive migrations.
- Keep compatibility routes stable.
- Make mock/demo behavior explicit.
- Keep changes small enough to review.
- Document risks and migration steps before large refactors.

