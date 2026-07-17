# Implementation Audit - 2026-06-05

## Summary

This pass completed the requested production-blocker starting set:

1. Permission Enforcement
2. Server Manager / State Machine
3. Crash Detection & Auto Restart
4. Power State Machine

It also includes earlier hardening work from `implementation_plan.md`: empty auth-secret bypass removal, Redis-backed login rate limiting, WebSocket cleanup, migration tracking, and audit metadata normalization.

Phase 1 is complete. Phase 2 is partially complete: backup persistence/restore, archive/decompress, native SFTP, disk quota enforcement, and config parser foundations have been implemented. Phase 3 reliability work is complete for the requested scope: migrations are tracked, WebSocket proxy pumps share cancellation, and schedules now wake through PostgreSQL notifications with a polling fallback.

## Final Verification

Commands run successfully:

```powershell
cd E:\game\apps\api
$env:GOCACHE='E:\game\apps\api\.gocache'
go test ./...

cd E:\game\apps\daemon
$env:GOCACHE='E:\game\apps\daemon\.gocache'
go test ./...
```

Result:

- API tests: passed
- Daemon tests: passed

## Feature Status

| Feature | Status | Notes |
|---|---|---|
| Permission Enforcement | Complete | Server-scoped routes now enforce granular permissions with admin and owner bypass. Subusers are checked against stored server permissions. Server lists are filtered to owned/shared servers. WebSockets require `websocket.connect`. |
| Server Manager / State Machine | Complete for current daemon scope | Added in-memory `ServerManager` with `sync.Map`, per-server state, install state, startup state, running action, root directory, disk limit, config sync status, expected stop tracking, and per-server power locking. |
| Crash Detection & Auto Restart | Complete for Docker runtime | Docker event watcher handles labeled container exit events. Unexpected non-zero exits and OOM exits restart after cooldown. Expected stops do not restart. |
| Power State Machine | Complete for Phase 1 | Power actions are serialized per server. Start/restart run daemon-side `onBeforeStart` preflight for install state, configuration sync, and disk-limit validation. User permission validation remains API-owned before daemon calls. |
| Backup Persistence | Complete | Added `backups` table and DB-backed backup history with UUID, server, checksum, size, status, upload ID, and completion timestamp. |
| Backup Restore | Complete | API restore endpoint tracks restoring/restored/restore_failed status and daemon validates/extracts backup ZIPs into the jailed server root. |
| File Archive / Decompress | Complete | Added tar.gz archive download and zip/tar.gz decompress routes with jailed extraction. |
| Native SFTP | Complete | Added Go-native SSH/SFTP server with panel-authenticated password sessions and file permission enforcement. |
| Disk Space Enforcement | Partial beyond Phase 2 | Start/restart preflight, file write, upload, and decompression paths enforce disk limits. Backups and all future write paths should continue using the quota helper. |
| Config File Parser | Initial implementation | Daemon config sync renders configured files with environment substitution and basic plain/properties/JSON content support. |
| Migration Tracking | Complete | API migrations are recorded in `schema_migrations` and only pending SQL files run, each inside its own transaction. |
| WebSocket Leak Fix | Complete | Realtime proxy pumps share a cancellation context and close both downstream/upstream sockets before waiting for the second pump to exit. |
| Schedule Runner Reliability | Complete | Scheduler listens on PostgreSQL `schedule_events`, wakes immediately on schedule/task edits, uses the next `next_run_at` as a timer, and keeps minute polling as fallback. |

## Files Changed

### API

- `apps/api/internal/http/auth.go`
  - Removed empty-secret auth bypass.
  - Added `requireServerPermission` middleware and shared permission-check helper.

- `apps/api/internal/http/handlers_servers.go`
  - Applied granular permission checks to server-scoped routes.
  - Added dynamic permission checks for power signals.
  - Filtered server list through current user visibility.
  - Made global audit route admin-only.
  - Sends server disk limits to daemon create requests.

- `apps/api/internal/daemon/client.go`
  - Added `diskMb` to daemon create request payloads.
  - Added daemon client methods for backup restore, file archive, and file decompress.

- `apps/api/internal/http/realtime.go`
  - Enforced `websocket.connect` for server WebSocket proxy connections.
  - Improved WebSocket pump cleanup.

- `apps/api/internal/http/server.go`
  - Added Redis-backed login failure rate limiting by IP and email hash.

- `apps/api/internal/http/server_test.go`
  - Updated tests for required auth behavior.

- `apps/api/internal/http/schedule_runner.go`
  - Replaced fixed 30-second polling with PostgreSQL LISTEN/NOTIFY, next-run timers, and fallback polling.

- `apps/api/internal/http/schedule_runner_test.go`
  - Added tests for scheduler wake-delay behavior.

- `apps/api/internal/store/store.go`
  - Added `schema_migrations` tracking and transactional migration application.

- `apps/api/internal/store/store_audit.go`
  - Normalizes audit metadata JSON before persistence.

- `apps/api/internal/store/store_users.go`
  - Added `UserCanAccessServer`.

- `apps/api/internal/store/store_servers.go`
  - Added `ListServersForUser`.

- `apps/api/internal/store/store_backups.go`
  - Added DB-backed backup list/count/limit/upsert/status methods.

- `apps/api/internal/store/store_schedules.go`
  - Added schedule change notifications, LISTEN support, and next-run lookup for the runner.

- `apps/api/migrations/019_backups.sql`
  - Added backup persistence table and indexes.

### Daemon

- `apps/daemon/internal/server/manager.go`
  - Added `ServerManager`, `ServerState`, power state tracking, per-server power lock, expected stop handling, and crash handling.
  - Added `onBeforeStart` preflight for install state, config sync, and disk limit validation.

- `apps/daemon/internal/server/server.go`
  - Wired create/install/delete/power handlers through `ServerManager`.
  - Starts event watcher when the runtime supports it.
  - Records daemon config sync and disk limits in server state.
  - Added backup restore, tar.gz archive, zip/tar.gz decompress, quota-aware writes, and config file rendering.

- `apps/daemon/internal/server/manager_test.go`
  - Added tests for concurrent power rejection, unexpected crash restart, expected stop behavior, cooldown suppression, config-sync preflight, and disk-limit preflight.

- `apps/daemon/internal/runtime/runtime.go`
  - Added `ContainerEvent` and optional `EventWatcher` interface.

- `apps/daemon/internal/runtime/docker.go`
  - Added Docker event watching for `modern-game-panel.server_id` labeled containers.

- `apps/daemon/internal/sftpserver/server.go`
  - Added native panel-authenticated SSH/SFTP server and jailed file handlers.

- `apps/daemon/internal/sftpserver/server_test.go`
  - Added SFTP auth, path jail, permission, write, and host-key persistence tests.

- `apps/daemon/go.mod`
- `apps/daemon/go.sum`
  - Added `github.com/pkg/sftp` and `golang.org/x/crypto` for the native SFTP server.

### Documentation

- `docs/architecture.md`
- `docs/api.md`
- `docs/daemon.md`
- `docs/security.md`

## Architecture Decisions

### Permission Enforcement

The panel API remains the source of identity, role, ownership, and subuser permission truth. Admins and owners bypass server permission checks. Subusers are authorized per server using the JSON permission grants already stored in the database.

### Server Manager

The daemon now has an in-memory state layer in front of the runtime interface. This preserves the project architecture: the API still owns orchestration intent, while the daemon owns node-local lifecycle safety.

### Power State Machine

The daemon `ServerManager` owns runtime state transitions and local preflight. Permission validation remains in the API because user identity and subuser grants live there. Before start or restart, the daemon verifies the server is not installing, has synced configuration, has a known data root, and is not over its disk limit.

### Crash Detection

Crash detection is runtime-optional. Docker implements event watching; other future runtimes can implement the same `EventWatcher` interface without changing API handlers. Clean exits are not treated as crashes. Non-zero exits and OOM exits restart only when not expected and outside cooldown.

### Native SFTP

The daemon owns the SFTP listener because it owns node-local filesystem access. Login is not local-only: the daemon calls the panel remote SFTP auth endpoint and receives the target server plus granted file permissions. File operations are jailed to that server root and mapped to the same Pterodactyl-style permissions used by the API.

### Schedule Runner Reliability

The API remains the schedule owner because schedules, tasks, runs, and daemon targets live in panel storage. Schedule and task mutations emit `NOTIFY schedule_events`; the runner listens on that channel and immediately rechecks due schedules. A next-run timer wakes exactly when the nearest known schedule is due, while minute polling remains as a resilience fallback if notification delivery is interrupted.

## Known Follow-Up Work

- Add panel-visible state sync after daemon-side crash restart.
- Persist crash detection settings per server instead of using the current default-enabled in-memory state.
- Add async progress streams for backup restore and large archive/decompress jobs.
- Add UI controls for backup restore, file archive, and file decompress.
- Replace any remaining deployment references to the old SFTP sidecar.
- Expand config parser compatibility with Pterodactyl egg parser variants.
