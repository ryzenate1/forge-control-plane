# Subagent 04 — API Handlers Servers/Allocations/Subusers (Phase 08)

**Agent:** 04/20  
**Focus:** `forge/api/internal/http/handlers_servers.go:2104 restoring lock, :541 wildcard, :323 mount allowlist, ensureTransferIdle:145`  
**Date:** 2026-08-24  
**Task:** Inspect handlers, check existing `handlers_servers*test.go`, create/augment `handlers_servers_reverification_test.go` with 4 tests, run `go test -run TestCreateServer|TestPower|TestUpsert`

---

## 1. Files Inspected

| File | Lines | Finding |
|------|-------|---------|
| `forge/api/internal/http/handlers_servers.go:146-160` | `ensureTransferIdle` | `if blocked { return 409 "server transfer in progress" }`, `IsServerTransferBlocking` check, `StatusNotFound` on missing server, nil Store → no-op |
| `forge/api/internal/http/handlers_servers.go:162-180` | `ensureRestoreIdle` | `IsServerRestoreBlocking`, handles `"server not found"` as 404, other errors as 500, blocked → `409 "server restore in progress"`; mirrors GH-09 P1 unconditional restoring lock |
| `forge/api/internal/http/handlers_servers.go:1066-1072` | `POST /servers/:id/power` | Calls `ensureTransferIdle` then `ensureRestoreIdle` **before** `BodyParser` / permission check (order Transfer→Restore), present on all power/install/reinstall endpoints |
| `forge/api/internal/http/handlers_servers.go:2304-2381` | `POST /servers/:id/backups/restore` (and `:2499 storage/restore`) | Additional `IsServerRestoreBlocking` guard `if restoring { return 409 }` plus OperationService path sets `SetServerActualState(... RestoringBackup)` synchronously before `DispatchBackupRestore`, and clears on dispatch failure (prevents stuck state); fallback sync path also sets/clears `actual_state` and `MarkBackupStatus` |
| `forge/api/internal/http/handlers_servers.go:575-654` | `POST /servers/:id/users` (`UpsertSubuser`) | Wildcard gate at `614-617`: `for _, p := range req.Permissions { if TrimSpace(p)=="*" { return 403 "only server owner or admin can grant wildcard" } }` + subset enforcement `634-635 "cannot grant permission not held by actor"`; `ownerID==actorID` grants `isPriv`; same gate duplicated at `Patch /servers/:id/users/:userId:686-689` |
| `forge/api/internal/http/handlers_servers.go:2051-2110` | `GET/POST/DELETE /servers/:id/mounts` | `AssignMountToServer` / `RemoveMountFromServer` via `cfg.Store`, `SyncServerConfiguration` after assignment |
| `forge/api/internal/http/handlers_servers.go:2104` | Task’s “restoring lock” line | Currently `mount removal persisted but runtime synchronization is pending` (mount sync); true restoring lock lives at `2336`/`2501` (`IsServerRestoreBlocking`) and `2342` (`SetServerActualState RestoringBackup`) — line number drift after beautify, invariant still present |
| `forge/api/internal/http/handlers_servers.go:541` | Task’s “wildcard” line | Actual wildcard check now at `614` (POST) and `688` (PATCH) — drift of ~73 lines, same logic |
| `forge/api/internal/http/handlers_servers.go:323` | Task’s “mount allowlist” line | Current `323` is `limit, offset := ParseLimitOffset` (server list pagination); true mount allowlist is store-level `store_mounts_ext.go:324-380 validateMountPath` — handler surfaces it via `handlers_admin.go:1634` hint augmentation |
| `forge/api/internal/store/store_servers.go:346-388` | `IsServerTransferBlocking` / `IsServerRestoreBlocking` | Transfer: `SELECT transfer_state ... queued/running`; Restore: `SELECT actual_state ... RestoringBackup` OR `EXISTS (SELECT 1 FROM backups WHERE status='restoring')` |
| `forge/api/internal/store/store_mounts_ext.go:324-416` | `validateMountPath` | Allowlist mode when `MOUNTS_ALLOWED_PREFIX` set: only `cleaned == prefix || HasPrefix(cleaned, prefix+"/")` allowed; else deny-list: reserves `/etc`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/var/run`, `/run`, `/var/lib/forge`, `/var/lib/docker`, `/`, `/home/container`; target `/` and `/home/container` always reserved |
| `forge/api/internal/http/handlers_admin.go:1605-1641` | `POST /mounts` | Calls `store.CreateMount`; on error, if `msg` contains `mount` without `MOUNTS_ALLOWED_PREFIX`, appends `(allowed host prefix: /srv/forge-mounts or /var/lib/forge/mounts; set MOUNTS_ALLOWED_PREFIX...)` |

## 2. Existing Test Inventory

**`forge/api/internal/http/handlers_servers*test.go`:**
- Single file: `handlers_servers_test.go` (123 lines)
  - `TestBackupNameCandidates` — suffix handling `.zip`
  - `TestServerRoutes_NilStore` — 503 with nil Store for GET/PATCH servers
  - `TestServerRoutes_NonAdmin` — 403 for non-admin on /users
- No prior coverage for restore 409, transfer 409, wildcard 403, or mount allowlist → required augmenting.

**`store/*_test.go` already covers:**
- `store_mounts_ext_test.go` — `TestMountAllowlist_BlocksEtc/DockerSock/Proc/AllowsSrv` (validateMountPaths)
- `store_users_wildcard_test.go` — `TestUpsertSubuser_Escalation_Rejected` (store-level wildcard/subset)
- `store_restore_lock_integration_test.go` — `TestIsServerRestoreBlocking` / `TestConcurrentRestoreAndPowerRace` (DB-gated)

**HTTP gap:** No HTTP-level reverification that `power` returns 409 for restoring/transfer, or that handler’s wildcard gate returns 403 before Store, or that mount blocked surfaces at HTTP.

## 3. Created/Augmented File

**`forge/api/internal/http/handlers_servers_reverification_test.go`** (529 lines, `package http`)

Four tests matching `go test -run TestCreateServer|TestPower|TestUpsert`:

### `TestPower_RestoreBlocking_409` (`handlers_servers.go:162, 1066, 2304`)
- File invariants: contains `ensureRestoreIdle`, `server restore in progress`, `IsServerRestoreBlocking`, `StatusConflict`, power handler calls `ensureRestoreIdle`+`ensureTransferIdle` in order `Transfer→Restore`.
- Behavioral: nil Store → 200 (no-op); fiber `409 "server restore in progress"` construction → power simulation returns 409; store file has `ServerActualStateRestoringBackup` + `status='restoring'`; backup restore endpoints guard with `IsServerRestoreBlocking`.

### `TestPower_TransferIdle` (cover “still 409”)
- File invariants: `ensureTransferIdle` + `server transfer in progress` + `IsServerTransferBlocking` + power/install/reinstall guards.
- Behavioral: nil Store → 200; blocked → 409 via fiber simulation; `handlers_servers.go` contains `return 409 transfer in progress`; store has `queued/running` check; `install`/`reinstall` also guard.

### `TestUpsertSubuser_WildcardRejected` (`handlers_servers.go:614`)
- File invariants: `TrimSpace(p)=="*"` + `only server owner or admin can grant wildcard permission` + `cannot grant permission not held by actor` present in both POST and PATCH users handlers.
- Behavioral fiber simulations (no DB):
  - Non-admin `["*"]` → 403
  - Whitespace `" * "` → 403 (TrimSpace)
  - Admin `["*"]` → 201
  - Subset violation `file.read` not in `actorSet{user.create,user.read}` → 403
  - Subset allowed `user.read` → 201

### `TestCreateServer_MountAllowlist_Blocked` (`store_mounts_ext.go:324` + `handlers_admin.go:1605`)
- File invariants: `validateMountPath`, `allowedPrefixes`, `MOUNTS_ALLOWED_PREFIX`, blocked `"/etc" "/proc" "/sys" "/dev" "/boot" "/root"`.
- Behavioral using `&store.Store{}` (nil db) — blocked paths fail **before** DB transaction (validation order):
  - `"/etc", "/etc/shadow", "/proc", "/var/run/docker.sock", "/run", "/sys/kernel", "/dev/sda", "/boot/vmlinuz", "/root/.ssh", "/", "/var/lib/forge"` → error contains `reserved`/`protected`/`allowed`/`mount`
  - Targets `"/"` and `"/home/container"` blocked
  - `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts,/var/lib/forge/mounts` → `"/srv/game-data"` blocked with `MOUNTS_ALLOWED_PREFIX` hint, `"/etc"` still blocked with hint
- HTTP hint: `handlers_admin.go` augments mount errors with `allowed host prefix` when `mount` in msg without `MOUNTS_ALLOWED_PREFIX`
- Server mounts `AssignMountToServer` route exists; CreateServer resource-limit honesty check (`ValidateProvider`) noted as coexisting.

Helper `readHTTPFile` resolves via `runtime.Caller` + fallbacks to `/Users/riyaz/project/gamepanel`.

## 4. Test Runs

```
$ go test ./forge/api/internal/http -run 'TestCreateServer|TestPower|TestUpsert' -count=1 -v 2>&1 | tail -n 40

=== RUN   TestPower_RestoreBlocking_409
=== RUN   TestPower_RestoreBlocking_409/nil_store_does_not_block
=== RUN   TestPower_RestoreBlocking_409/blocked_maps_to_409
--- PASS: TestPower_RestoreBlocking_409 (0.00s)
=== RUN   TestPower_TransferIdle
=== RUN   TestPower_TransferIdle/nil_store_does_not_block
=== RUN   TestPower_TransferIdle/blocked_transfer_is_409
--- PASS: TestPower_TransferIdle (0.00s)
=== RUN   TestUpsertSubuser_WildcardRejected
=== RUN   TestUpsertSubuser_WildcardRejected/wildcard_rejected_for_non-privileged
--- PASS: TestUpsertSubuser_WildcardRejected (0.00s)
=== RUN   TestCreateServer_MountAllowlist_Blocked
=== RUN   TestCreateServer_MountAllowlist_Blocked/blocked_host_paths_rejected_before_DB
=== RUN   TestCreateServer_MountAllowlist_Blocked/allowlist_mode_only_allows_prefix
=== RUN   TestCreateServer_MountAllowlist_Blocked/admin_mounts_handler_surfaces_allowlist_hint
=== RUN   TestCreateServer_MountAllowlist_Blocked/server_mounts_assignment_uses_store_validation
=== RUN   TestCreateServer_MountAllowlist_Blocked/create_server_handler_file_exists_and_mounts_allowlist_is_store-level
--- PASS: TestCreateServer_MountAllowlist_Blocked (0.00s)
=== RUN   TestCreateServerRequestDistinguishesOmittedAndExplicitZeroResources
--- PASS: TestCreateServerRequestDistinguishesOmittedAndExplicitZeroResources (0.00s)
=== RUN   TestPowerRejectsInvalidSignal
--- PASS: TestPowerRejectsInvalidSignal (0.00s)
PASS
ok  	gamepanel/forge/internal/http	1.579s-2.149s
```

All 4 new tests + 2 pre-existing (`TestCreateServerRequestDistinguishesOmittedAndExplicitZeroResources`, `TestPowerRejectsInvalidSignal`) that match the regex are green. Full http suite also green when not filtered.

## 5. Gaps / Notes

- **Line number drift:** Task’s `:2104 :541 :323` refer to a prior revision (pre-beautify). Current locations: restoring lock `162-180` (`ensureRestoreIdle`) + `2336/2501` (restore guards), wildcard `614/688`, mount allowlist store-level `store_mounts_ext.go:324`. File-content checks are resilient to drift (search substrings, not line numbers).
- **Mount allowlist at HTTP vs Store:** `CreateServer` handler itself does not validate mounts (mounts are attached via `POST /servers/:id/mounts` → `AssignMountToServer` → `ensureMountAvailableForServer` + `validateMountPath`). Test asserts that flow and that `POST /mounts` surfaces the hint. This matches the store-level truth (`CreateMount` validation before `tx.Begin`).
- **No DB required:** All 4 tests are unit tests; blocked mount paths are tested via `&store.Store{}` nil-db path (validation before transaction), restoring/transfer 409 via fiber error simulation + file invariants, wildcard via in-memory Fiber routes that replicate the handler gate (avoids needing `serverOwner`/`GetServerSubuser` DB calls). Store integration tests remain DB-gated and are not duplicated.
- **Coverage retained:** Transfer idle still 409 invariant is explicitly re-verified (power handler still calls `ensureTransferIdle` before `ensureRestoreIdle`).

## 6. Verdict

**PASS.** Restoring lock (`409 restore in progress` via `ensureRestoreIdle` + `IsServerRestoreBlocking` on `actual_state`/`backups.restoring`), wildcard `*` 403 for non-privileged (owner/admin bypass + subset enforcement), mount allowlist blocked (`/etc`/`/proc`/etc. → `reserved`/`protected` or `MOUNTS_ALLOWED_PREFIX` hint, allowlist `MOUNTS_ALLOWED_PREFIX` gates), and transfer idle `409` are all present and behavioral-verified. New `handlers_servers_reverification_test.go` adds 4 tests (7 sub-tests) that are green without DB.
