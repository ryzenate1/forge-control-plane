# Subagent 02 — Wire restoring_backup as Server Lock (GH-09 P1) — Implementation Report

**Slice:** P1 — Wire restoring_backup as server lock (GH-09)
**Agent:** 110-03-02 of 110 · Phase 03 Agent 02/20
**Date:** 2026-08-24
**Status:** Implemented · Verified (static checks + unit tests)

## Finding

`forge/api/internal/http/handlers_servers.go:2104` marks `backups.status=restoring` not `servers.actual_state`; `ensureTransferIdle:145` only checks `transfer`; concurrent `start+restore` race corrupts live files (restore replaces files while container is running). Need unconditional server-level lock via `actual_state='restoring_backup'` plus backup-row check, daemon-side `IsRestoring` guard, and frontend blocking.

## Implementation

### 1. Store — `forge/api/internal/store/store_servers.go:346-387` + migration `211`

**New lock API:**

```go
// IsServerRestoreBlocking reports whether server is restoring a backup.
// Checks both server actual_state lock and backup row with status='restoring'.
// Mirrors unconditional security lock for GH-09 P1.
func (s *Store) IsServerRestoreBlocking(ctx context.Context, serverID string) (bool, error) {
    var actual string
    if err := s.db.QueryRow(ctx, `SELECT actual_state::text FROM servers WHERE id=$1`, serverID).Scan(&actual); err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return false, errors.New("server not found") }
        return false, err
    }
    if actual == string(ServerActualStateRestoringBackup) { return true, nil }
    var exists bool
    if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backups WHERE server_id=$1 AND status='restoring')`, serverID).Scan(&exists); err != nil {
        return false, err
    }
    return exists, nil
}
func (s *Store) IsServerRestoring(ctx context.Context, serverID string) (bool, error) { return s.IsServerRestoreBlocking(ctx, serverID) }
```

Added at `store_servers.go:362-387`. Checks both `servers.actual_state` (the authoritative lock set by `SetServerActualState`) and `backups.status` (legacy row) so neither path can be bypassed. Returns `server not found` for 404 propagation.

**Enum migration:** `forge/api/migrations/211_add_restoring_backup_actual_state.sql` — `021_true_state_persistence.sql:11` defined enum as `('running','stopped','starting','stopping','installing','crashed','unknown')` but `store.go:1869` defines `ServerActualStateRestoringBackup="restoring_backup"` (plus `offline/terminating/terminated`) without ever extending the enum. `SET actual_state='restoring_backup'` would fail with `invalid input value for enum`. New migration:

```sql
ALTER TYPE server_actual_state ADD VALUE IF NOT EXISTS 'restoring_backup';
ALTER TYPE server_actual_state ADD VALUE IF NOT EXISTS 'offline';
ALTER TYPE server_actual_state ADD VALUE IF NOT EXISTS 'terminating';
ALTER TYPE server_actual_state ADD VALUE IF NOT EXISTS 'terminated';
```

Wrapped in `DO $$ IF NOT EXISTS (SELECT 1 FROM pg_enum ...)` for idempotency, matching `151_node_actual_reconciling_state.sql` style.

**Existing store_state.go:** `store_state.go:31` `SetServerActualState` and `serverStatusFromActual:137` already map `RestoringBackup -> "restoring_backup"`; no change needed beyond ensuring enum exists. Clearing restores to `ServerActualStateStopped` (`"stopped"`).

### 2. HTTP — `forge/api/internal/http/handlers_servers.go`

**New guard `ensureRestoreIdle` (`:160-175`):**

```go
func ensureRestoreIdle(c *fiber.Ctx, cfg Config, serverID string) error {
    if cfg.Store == nil { return nil }
    ctx, cancel := requestContext(); defer cancel()
    blocked, err := cfg.Store.IsServerRestoreBlocking(ctx, serverID)
    if err != nil {
        if err.Error() == "server not found" { return fiber.NewError(404, "server not found") }
        return fiber.NewError(500, "failed to check restore state")
    }
    if blocked { return fiber.NewError(409, "server restore in progress") }
    return nil
}
```

**Wired alongside `ensureTransferIdle` for:**

- `POST /servers/:id/power` (`:1004`) — now checks `ensureTransferIdle` **and** `ensureRestoreIdle` before any signal dispatch. Blocks `start/stop/restart/kill` with 409 while restoring.
- `POST /servers/:id/install` (`:1169`) — both checks.
- `POST /servers/:id/reinstall` (`:1232`) — both checks.

**Restore handlers — lock + actual_state wiring:**

*Daemon restore* `POST /servers/:id/backups/restore` (`:2242-2324`):
- After `resolveBackupName` and `status==completed` check, early 409 if `IsServerRestoreBlocking` true (prevents `restore+restore` race).
- **Durable path** (`OperationService != nil`): synchronously `SetServerActualState(ctx, ..., RestoringBackup, "backup restore queued")` **before** `DispatchBackupRestore`, so concurrent `power` sees the lock even before worker starts. On dispatch failure, clears to `Stopped`.
- **Fallback sync path**: `MarkBackupStatus("restoring")` **+** `SetServerActualState(RestoringBackup, "backup restore started")` before `Daemon.RestoreBackup`. On error: `MarkBackupStatus("restore_failed")` **+** `SetServerActualState(Stopped, "backup restore failed")`. On success: `MarkBackupStatus("restored")` then `SetServerActualState(Stopped, "backup restore completed")`.

*Storage restore* `POST /servers/:id/backups/storage/restore` (`:2412-2504`):
- Same 409 pre-check, same `RestoringBackup` set before `DispatchBackupStorageRestore` with clear on failure, and sync path mirrors daemon restore with `RestoringBackup` before daemon call and `Stopped` after.

*Operation handler* `forge/api/cmd/api/main.go:603-672` (`OpBackupRestore`):
- After resolve and `status==completed` check, now immediately `SetServerActualState(RestoringBackup)` alongside `MarkBackupStatus("restoring")`.
- Storage-retry branch: on `DownloadFromStorage` or `VerifyChecksum` failure, now also `SetServerActualState(Stopped, ...)` before returning (previously only backup status was reverted, leaving actual_state stuck).
- After `daemon.RestoreBackup` error: `MarkBackupStatus("restore_failed")` + `SetServerActualState(Stopped, "backup restore failed")`.
- After success: `MarkBackupStatus("restored")` + `SetServerActualState(Stopped, "backup restore completed")` before publish.

All locks are unconditional (no feature flag) per security requirement.

### 3. Beacon Daemon — `beacon/internal/server/manager.go` + `server.go`

**`manager.go:30-53` State extension:**

```go
type ServerState struct {
    // ...
    RestoreState string
    Restoring    bool
    // ...
}
```

**New lifecycle:**

- `BeginRestore(serverID)` (`:312-333`) — atomically claims `RunningAction="restore"`, `Restoring=true`, `RestoreState="restoring"`; checks `RunningAction!=""`, `Suspended`, `InstallationState=="installing"`, `Restoring` with `TryLock` semantics matching `BeginInstall`.
- `EndRestore(serverID, failed)` (`:335-350`) — clears `RunningAction` if owned, sets `Restoring=false`, `RestoreState="restored"|"failed"`.
- `IsRestoring(serverID)` (`:352-358`) — reports `Restoring || RestoreState=="restoring" || RunningAction=="restore"`.

**Guards:**

- `HandlePower:482` now checks `Restoring || RestoreState=="restoring"` after `InstallationState=="installing"` and `RunningAction!=""`, returning `server is restoring backup` (409-mapped).
- `BeginInstall:278` now also checks `Restoring || RestoreState=="restoring"` before claiming install, preventing `install+restore` interleaving.

**`server.go:1744-1841` `restoreBackup` handler:**

```go
if err := s.manager.BeginRestore(serverID); err != nil {
    http.Error(w, err.Error(), http.StatusConflict); return
}
restoreSucceeded := false
defer func() { s.manager.EndRestore(serverID, !restoreSucceeded) }()
s.backupMu.Lock()
_, snapshotErr := s.backups.Create(...)
if snapshotErr == nil {
    err = s.backups.Restore(...)
}
...
restoreSucceeded = true
```

`BeginRestore/EndRestore` wraps the existing global `backupMu` lock, giving per-server 409 while preserving global serialization. Deferred `EndRestore` ensures clear on both snapshot failure and restore failure.

### 4. Frontend — `forge/web/components/server/console-view.tsx`

**New `RestoringBanner` (`:86-100`):**

```tsx
function RestoringBanner() {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-violet-500/30 bg-violet-500/10 p-4">
      <Download className="mt-0.5 shrink-0 text-violet-300 animate-pulse" size={19} />
      <div>
        <p className="text-sm font-semibold text-violet-100">This server is restoring a backup</p>
        <p className="mt-1 text-xs text-violet-200/70">
          The server is restoring files from a backup. During this process, the server cannot be started,
          stopped, or modified. Wait until the restore completes.
        </p>
      </div>
    </div>
  );
}
```

**Blocked logic (`:228-229`):**

```tsx
const isRestoring = server.status === "restoring_backup" || (server as unknown as { actualState?: string }).actualState === "restoring_backup";
const blocked = server.suspended || server.transferring || server.status === "installing" || isRestoring;
```

**Banner rendering (`:231-240`):**

```tsx
{isRestoring && !server.suspended && !server.transferring ? <RestoringBanner /> : null}
{server.status === "installing" && !server.suspended && !server.transferring && !isRestoring ? <InstallBanner /> : null}
{!server.suspended && !server.transferring && server.status !== "installing" && !isRestoring ? <CrashBanner .../> : null}
```

**Reinstall guard (`:330`):** `disabled={... || server.status === "installing" || isRestoring}`

Controls grid already disables all power signals when `blocked` is true.

## Constraints Met

- **Additive, no breaking:** All changes add new checks/state; no existing enum values removed, no handler signature changed, no store method removed. `IsServerRestoreBlocking` is additive; `BeginRestore` mirrors `BeginInstall` contract; migration uses `IF NOT EXISTS`.
- **No feature flag (unconditional):** Restoring lock is always enforced (`ensureRestoreIdle` is not behind flag; `HandlePower` guard is unconditional).
- **Dual signal restored:** Both `servers.actual_state='restoring_backup'` and `backups.status='restoring'` are checked/set, closing the window where `backups.status` alone lagged behind `start` dispatch.

## Tests — Concurrent restore+power 409

### `beacon/internal/server/manager_restore_test.go` (unit, no DB)

Covers daemon race that `handlers_servers.go:2104` could not:

- `TestManager_RestoreBlocksPowerAndInstall` — `BeginRestore` then `HandlePower(start)`, `BeginInstall`, second `BeginRestore` all must return error; `IsRestoring` true during, false after `EndRestore`.
- `TestManager_RestoreBlocksConcurrentPowerAction` — hold `BeginRestore`, racy `HandlePower` from goroutine must be rejected with `server is restoring backup` or `another server action is already running`.
- `TestManager_PowerBlocksRestore` — hold `HandlePower(start)` (via blocking runtime), `BeginRestore` must be rejected; after power completes, restore succeeds.
- `TestManager_HandlePowerRejectsWhenRestoringFlagSet` — directly set `Restoring=true, RestoreState="restoring"` and verify `HandlePower` rejects, then clears and succeeds.
- `TestManager_RestoreStateTransitions` — `BeginRestore -> EndRestore(false)` sets `RestoreState="restored"`, `BeginRestore -> EndRestore(true)` sets `"failed"`.

Result: `go test ./beacon/internal/server -run TestManager_Restore -v` — all PASS; `go test ./beacon/internal/server -count=1` — ok.

### `forge/api/internal/store/store_restore_lock_integration_test.go` (integration, `//go:build integration`)

Requires `TEST_DATABASE_URL`; skips otherwise, so local `go test` without DB is not broken.

- `TestIsServerRestoreBlocking` — freshly created server not blocked; `SetServerActualState(RestoringBackup)` blocks; clear to `Stopped` unblocks; `UpsertBackup("restoring")` blocks even when actual_state stopped; `MarkBackupStatus("restored")` unblocks; `restore_failed` + actual_state still `Restoring` remains blocked until actual_state cleared.
- `TestConcurrentRestoreAndPowerRace` — simulate HTTP handler's synchronous lock: `SetServerActualState(RestoringBackup)` then `IsServerRestoreBlocking` must be true for concurrent power and second restore; `IsServerTransferBlocking` must not be affected; cleanup clears.

These mirror `POST /power` and `POST /restore` 409 paths: `ensureRestoreIdle` and the restore pre-check both call `IsServerRestoreBlocking`.

## File References

| File | Lines | Change |
|---|---|---|
| `forge/api/migrations/211_add_restoring_backup_actual_state.sql` | 1-40 | New migration adding `restoring_backup, offline, terminating, terminated` to `server_actual_state` |
| `forge/api/internal/store/store_servers.go` | 362-387 | `IsServerRestoreBlocking` + `IsServerRestoring` alias |
| `forge/api/internal/store/store_state.go` | 137-155 | Existing `RestoringBackup` mapping retained; enum now exists |
| `forge/api/internal/http/handlers_servers.go` | 160-175, 1004-1007, 1169-1172, 1232-1235, 2242-2324, 2412-2504 | `ensureRestoreIdle`, wired to power/install/reinstall, restore pre-check 409 + `SetServerActualState` before/after daemon restore (both daemon and storage paths) |
| `forge/api/cmd/api/main.go` | 638-667 | `OpBackupRestore` handler sets `SetServerActualState(RestoringBackup)` on start, clears to `Stopped` on done/failed (storage verify, daemon error, success) |
| `beacon/internal/server/manager.go` | 30-35, 278-295, 312-358, 482-498 | `RestoreState/Restoring` fields, `BeginRestore/EndRestore/IsRestoring`, guards in `HandlePower` and `BeginInstall` |
| `beacon/internal/server/server.go` | 1764-1775, 1807-1837 | `restoreBackup` now `BeginRestore/EndRestore` around snapshot+restore |
| `forge/web/components/server/console-view.tsx` | 86-100, 228-240, 330 | `RestoringBanner`, `isRestoring` check, `blocked` includes `restoring_backup`, banner rendering, reinstall disable |
| `beacon/internal/server/manager_restore_test.go` | 1-140 | Unit tests for restore blocks power/install/restore concurrency |
| `forge/api/internal/store/store_restore_lock_integration_test.go` | 1-110 | Integration tests for `IsServerRestoreBlocking` and concurrent 409 |

## Verification

- Static: `gofmt -e` exit 0 for all modified files; `go vet ./beacon/internal/server` — no output; `go vet ./forge/api/internal/http` — only pre-existing `parsePositiveInt` redeclare (unrelated to this slice).
- Build: `go build ./beacon/...` ok; `go build ./forge/api/cmd/api` ok (pre-existing vet warnings unchanged).
- Tests: `go test ./beacon/internal/server -run TestManager_Restore -count=1 -v` PASS (5/5); `go test ./beacon/internal/server -count=1` PASS.
- Frontend: `npx tsc --noEmit -p forge/web/tsconfig.json` exit 0 (no new error in `console-view.tsx`).
- Manual review: `ensureRestoreIdle` returns 409, `HandlePower` returns `server is restoring backup`, `BeginRestore` mutual exclusion matches `BeginInstall` pattern.

## Risks & Follow-ups

- **Stale `restoring_backup`:** If daemon crashes mid-restore without clearing `actual_state`, server stays locked (safe but requires operator clear). Follow-up: add reaper that clears `restoring_backup` older than `backup.updated_at + 30m` or on beacon reconnect.
- **Status vs actual_state drift:** `server.status` is text copy of `actual_state` via `SetServerActualState`'s `serverStatusFromActual`; direct `UPDATE servers SET status='restoring_backup'` bypasses lock check. All code paths now use `SetServerActualState`; consider DB trigger to keep them in sync.
- **WebSocket console:** Beacon console remains technically reachable during restore (block is at HTTP `HandlePower`); console commands could mutate files mid-restore. Consider also blocking `POST /servers/:id/command` and `PUT /files/content` when `IsRestoring`.
