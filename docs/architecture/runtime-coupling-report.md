# Runtime Coupling Report

Date: 2026-06-14

Scope: Phase 8 Runtime Abstraction Foundation.

This report identifies runtime-specific and daemon-specific coupling that remains after introducing the API-side Runtime abstraction.

## Summary

Phase 8 introduced:

- `apps/api/internal/runtime`
- Provider-neutral Runtime interface
- Docker adapter wrapping existing daemon client behavior
- Runtime Registry
- Runtime capability model
- Runtime events
- Runtime operation metrics

Cluster Manager no longer depends directly on the daemon client for server power, delete, or stats refresh paths. Reconciler reaches runtime behavior through Cluster Manager. Scheduled power tasks now use the Runtime interface.

Docker remains the only runtime provider. No Firecracker, containerd, Podman, Kubernetes, Nomad, Migration Engine, Failover, or AI features were introduced.

## Coupling Removed From Orchestration Paths

Cluster Manager:

- Before: held `*daemon.Client`.
- After: holds `runtime.Runtime`.
- Server delete uses `Runtime.DeleteServer`.
- Server start/stop/restart/kill use Runtime methods.
- Server actual-state refresh uses `Runtime.Stats`.

Reconciler:

- Continues to call Cluster Manager for actual-state refresh and corrective actions.
- Runtime access is now indirect through Cluster Manager.

Schedule Runner:

- Scheduled power tasks now use Runtime interface.
- Scheduled backup tasks still use daemon backup APIs.

## Intentional Docker Adapter Coupling

`apps/api/internal/runtime/docker.go` is intentionally coupled to:

- `apps/api/internal/daemon.Client`
- daemon create/delete/power/stats request and response types
- Docker provider name `docker`

This is the intended boundary for Phase 8. Future providers should add new adapters without changing Cluster Manager or Reconciler.

## Remaining API Daemon Coupling

The API still depends directly on daemon APIs for these workflows:

- Server install/provisioning in `apps/api/internal/http/handlers_servers.go`
- Server config sync in `apps/api/internal/http/handlers_servers.go`
- Server stats endpoint in `apps/api/internal/http/handlers_servers.go`
- Server logs endpoint in `apps/api/internal/http/handlers_servers.go`
- Realtime WebSocket proxy in `apps/api/internal/http/realtime.go`
- Backup create/list/download/restore in `apps/api/internal/http/handlers_servers.go`
- File list/read/write/upload/delete/archive/decompress/mkdir/rename in `apps/api/internal/http/handlers_servers.go`
- Scheduled backup task in `apps/api/internal/http/schedule_runner.go`

These are not fully orchestration-layer paths yet, but Migration Engine and Failover will need runtime-neutral equivalents for archive, restore, stats, and lifecycle inspection.

## Remaining Docker-Shaped Data Fields

The API and contracts still expose Docker-shaped or daemon-shaped fields:

- `servers.cpu_shares`
- `CreateServerRequest.cpuShares`
- `NodeHeartbeatRequest.dockerStatus`
- `nodes.docker_status`
- template/egg `docker_images`
- remote payload `container_image`
- file denylist entries such as `/.dockerenv`

These fields remain for compatibility. They should not be expanded as platform primitives.

## Remaining Daemon Agent Coupling

Node and remote-agent semantics still use daemon vocabulary:

- `daemon_base`
- `daemon_listen`
- `daemon_sftp`
- `daemon_token_id`
- `daemon_token`
- remote daemon authentication headers and token checks
- daemon heartbeat payloads

This is acceptable while Daemon Agent remains the host-local execution component, but runtime provider details should move out of daemon-specific fields over time.

## Remaining Daemon Runtime Coupling

The daemon service remains Docker-backed:

- `apps/daemon/internal/runtime/docker.go`
- `apps/daemon/cmd/daemon/main.go`
- Docker SDK dependencies in `apps/daemon/go.mod`
- Docker container events and Docker stats decoding

Phase 8 does not change daemon internals. Future provider work should either add daemon-side provider adapters or split runtime execution into a provider-specific agent layer.

## Metrics Added

Phase 8 adds these API metrics:

- `game_panel_runtime_operations_total`
- `game_panel_runtime_operation_failures_total`
- `game_panel_runtime_capability_checks_total`

These metrics are in-memory and process-local. They are not durable and reset on restart.

## Events Added

Phase 8 adds these in-process events:

- `RuntimeRegistered`
- `RuntimeUnavailable`
- `RuntimeCapabilityChanged`

These events are not durable, distributed, replayable, or cross-process.

## Recommended Cleanup Before Migration Engine

- Add runtime-neutral archive/restore or snapshot contracts.
- Add runtime-neutral create/provision/install flow for server activation.
- Replace direct handler stats/logs calls with runtime-backed read operations where appropriate.
- Define runtime instance identity separate from server ID.
- Normalize CPU/resource units away from Docker `cpu_shares`.
- Add provider capability checks to placement and migration planning.
- Add durable placement/reservation records before executing movement.

## Known Limitations

- `Runtime.Exists` and `Runtime.Inspect` in the Docker adapter currently rely on daemon stats as the only available daemon-backed existence signal.
- Runtime capability checks exist but are not yet used by Scheduler placement.
- Runtime metrics are process-local.
- Runtime registry is in-process only.
- No external runtime provider is implemented.
