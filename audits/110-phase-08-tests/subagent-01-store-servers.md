# Subagent 01 — Store Servers / Allocations / Eggs Reverification (110-08-01)

**Date:** 2026-08-24  
**Slice:** Store Servers/Allocations/Eggs (200k LOC coverage scope)  
**Agent:** 01/20 - Phase 08 Final Verifications + Test Files

---

## 1. Files Inspected

| File | LOC | Key Areas Inspected | Notes |
|------|-----|---------------------|-------|
| `forge/api/internal/store/store_servers.go:1` | 599 | `ListServers`, `CreateServer`, `IsServerRestoreBlocking` (line 366), `CompareAndSetServerGeneration`, `UpdateServer`, `FenceServerCAS` | Central server CRUD + restoring lock (GH-09 P1). Alias `IsServerRestoring` at line 386. |
| `forge/api/internal/store/store_servers_control.go` | ~260 | `ServerControlTarget`, `ServerProvisionTarget` | Provisioning without extra alloc logic |
| `forge/api/internal/store/store_allocations.go:15` | 391 | `AllocationNode` at line 15, `CreateAllocations` validation at lines 134-170 (protocol tcp/udp, containerPort defaults, IP validation) | Migration 090 adds `protocol` + `container_port` + unique `(node_id, ip, port, protocol)`. Migration 144 adds trigger `default_allocation_container_port`. |
| `forge/api/internal/store/store_nests.go` | 425 | `Nest`/`Egg` CRUD, `normalizeDockerImages` | Nest/Egg lifecycle, Games nest seeding |
| `forge/api/internal/store/store_egg_variables.go:176` | 340 | `validateVariableValue` integer handling at line 176, regex slash-strip at lines 213-239, `findRegexTokenEnd` + `splitValidationRules` (slash fix) | Slash-delim fix is the 200k LOC regression: PTDL paper regex `/^...$/` with flags `i/m/s`, char-class `|` and alternation handling |
| `forge/api/internal/store/store_startup.go` | 100 | `GetServerStartup`, `UpdateServerStartupVariable` (calls `validateVariableValue` at line 73) | Startup variable validation reuses same validator |
| `forge/api/internal/store/seed_game_templates.go` | 333 | `fallbackGameTemplates` (14 templates), `SeedGameTemplates` + `validateVariableValue` at line 323 | 93% deficit closure check |
| `forge/api/migrations/090_allocation_transport.sql` | — | Add `protocol` + `container_port`, checks, unique key | |
| `forge/api/migrations/144_allocation_container_port_compatibility.sql` | — | Trigger default `container_port := port` | |
| `forge/api/migrations/211_a_add_restoring_backup_actual_state.sql` | — | Add enum values `restoring_backup`, `offline`, `terminating`, `terminated` | Unconditional security lock GH-09 P1 |
| `forge/api/internal/store/store_state.go:31` | 177 | `SetServerActualState`, `serverStatusFromActual` mapping `restoring_backup` | |

---

## 2. Existing Tests (ls)

```
forge/api/internal/store/*test*.go (45 files):
- store_egg_variables_test.go              -> TestValidateVariableValue_RegexSlash, TestSplitValidationRules_NoSplitInsideRegex, TestValidateVariableValue_PaperImport, TestSeedGameTemplates_CountAndValidation
- store_mounts_ext_test.go                 -> TestValidateMountPaths, TestMountAllowlist_BlocksEtc, TestMountAllowlist_BlocksDockerSock, TestMountAllowlist_BlocksProc, TestMountAllowlist_AllowsSrv
- store_restore_lock_integration_test.go   -> TestIsServerRestoreBlocking, TestConcurrentRestoreAndPowerRace (build: integration)
- store_seed_eggs_test.go                  -> TestEggUpsert_Idempotent, TestSeedDefaultEggs_StoreAvailable
- store_servers_lifecycle_integration_test.go -> TestServerOrphanRemediationListAndResolve, TestServerInventoriesSupportEmailSearchAndNewestFirstOrdering, TestServerPatchAndHardDeleteAreTransactional
- store_eggs_integration_test.go           -> migrationTestStore helper + TestMigration043...
- 40 other files (capacity, tenancy, apikeys, etc.)
```

**Coverage check before change:**

- `grep TestValidateVariableValue_RegexSlashDelim` → 0 hits (only `RegexSlash` existed)
- `grep TestCreateServer_AllocationProtocol` → 0 hits
- `grep TestMountAllowlist_BlocksSensitive` → 0 hits (only BlocksEtc/BlocksDockerSock/BlocksProc/AllowsSrv existed)
- `grep TestIsServerRestoreBlocking` → 1 hit in `store_restore_lock_integration_test.go:18` (integration-only, skipped without TEST_DATABASE_URL)
- Existing file `store_servers_reverification_test.go` → **missing** (confirmed `ls` → no such file)

**Conclusion:** Three of four required reverification tests were missing as exact names; the fourth (`IsServerRestoreBlocking`) existed only as integration (no unit coverage). Created new file to ensure 200k-LOC slice is covered both unit and integration.

---

## 3. New Test File Created

**Path:** `forge/api/internal/store/store_servers_reverification_test.go` (557 lines, no build tag — runs in `go test` without DB, integration sub-tests skip gracefully)

**Functions (5 top-level, 7 sub-suites):**

| Test | Lines | Covers | Spec Mapping |
|------|-------|--------|--------------|
| `TestValidateVariableValue_RegexSlashDelim` | store_servers_reverification_test.go:20 | Slash-delim fix: stripping `/.../` and `/.../flags`, flag handling `i/m/s`, invalid flag `g/x`, alternation `\|` not splitting, char-class `[\|]`, escaped slash `\/`, paper `server.jar` PTDL, palworld decimal, plain regex, integer/string/in rules | Task: `TestValidateVariableValue_RegexSlashDelim (from slash fix)` — replicates `store_egg_variables.go:213-239` + `splitValidationRules` |
| `TestValidateVariableValue_RegexSlashDelim_Split` | store_servers_reverification_test.go:102 | `splitValidationRules` interval logic for `regex:/.../` containing `\|` | Supplement for slash fix |
| `TestCreateServer_AllocationProtocol` | store_servers_reverification_test.go:131 | `store_allocations.go:148-162` protocol defaults `tcp`, case-insensitive, `containerPort` defaults to `port`, validation `tcp/udp` only, `1-65535` range, IP/port pre-tx checks | Task: `TestCreateServer_AllocationProtocol (containerPort/protocol)` — unit via nil Store (pre-tx validation) + integration via `migrationTestStore` (persistence, uniqueness per `protocol`, Get/List round-trip, duplicate error `already exists`). Uses migration 090/144. |
| `TestMountAllowlist_BlocksSensitive` | store_servers_reverification_test.go:289 | `store_mounts_ext.go:324` `validateMountPath` — all sensitive prefixes in deny-list (`/etc`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/var/run`, `/run`, `/var/lib/forge`, `/var/lib/docker`, `/`, `/home/container`) plus allowed `/srv/...`, `/mnt/...`, and allowlist mode `MOUNTS_ALLOWED_PREFIX` | Task: `TestMountAllowlist_BlocksSensitive (already exists but ensure coverage)` — supplements existing `BlocksEtc`/`BlocksDockerSock`/`BlocksProc` with single composite sensitive test |
| `TestIsServerRestoreBlocking_Reverification` | store_servers_reverification_test.go:380 | `store_servers.go:366` + `store_state.go:31` + `store.go:1869` constants: `ServerActualStateRestoringBackup == "restoring_backup"`, `serverStatusFromActual`, alias `IsServerRestoring`, plus integration cycle (not blocked → restoring_backup blocks → stopped unblocks → backup row `restoring` blocks → `restored` unblocks → not-found → race) | Task: `TestIsServerRestoreBlocking (from restoring lock 211 migration)` — avoids name collision with existing integration test (which is `//go:build integration`) by using suffix `_Reverification`; unit constants run without DB, integration skips if `TEST_DATABASE_URL` unset |

**Collision avoidance:** Existing `TestIsServerRestoreBlocking` is `//go:build integration` (only compiled with `-tags=integration`). New file defines `TestIsServerRestoreBlocking_Reverification` (no tag) to avoid duplicate definition when `-tags=integration` is used. Report documents that it covers the same unconditional lock.

**Verification:**
- `go vet ./forge/api/internal/store` → no output (clean)
- `grep -n "^func Test" store_servers_reverification_test.go` → 5 functions (see table)

---

## 4. Run Output (as requested)

### Command 1 (task-mandated):

```sh
go test ./forge/api/internal/store -run "TestValidateVariableValue|TestSeedGameTemplates|TestMountAllowlist" -count=1 -v 2>&1 | tail -n 40
```

**Output tail (PASS, 2.881s):**

```
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__boot_vmlinuz
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__root
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__root_.ssh
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__var_run
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__var_run_docker.sock
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__run
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__run_docker.sock
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__var_lib_forge
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__var_lib_forge_volumes
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__var_lib_docker
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__
=== RUN   TestMountAllowlist_BlocksSensitive/blocks__home_container
=== RUN   TestMountAllowlist_BlocksSensitive/allowlist_mode
--- PASS: TestMountAllowlist_BlocksSensitive (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__etc (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__etc_shadow (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__etc_passwd (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__etc_forge (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__proc (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__proc_self_environ (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__sys (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__sys_kernel (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__dev (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__dev_sda (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__boot (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__boot_vmlinuz (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__root (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__root_.ssh (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__var_run (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__var_run_docker.sock (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__run (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__run_docker.sock (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__var_lib_forge (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__var_lib_forge_volumes (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__var_lib_docker (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__ (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/blocks__home_container (0.00s)
    --- PASS: TestMountAllowlist_BlocksSensitive/allowlist_mode (0.00s)
PASS
ok  	gamepanel/forge/internal/store	2.881s
```

Full filter also passed for `TestValidateVariableValue_RegexSlash` (existing 28 sub-tests), `TestValidateVariableValue_PaperImport`, `TestSeedGameTemplates_CountAndValidation` (14 templates), and all `TestMountAllowlist_*` (BlocksEtc 5, BlocksDockerSock 4, BlocksProc 14, AllowsSrv) — no failures.

### Command 2 (new reverification tests):

```sh
go test ./forge/api/internal/store -run "TestCreateServer_AllocationProtocol|TestIsServerRestoreBlocking_Reverification" -count=1 -v
```

```
=== RUN   TestCreateServer_AllocationProtocol
=== RUN   TestCreateServer_AllocationProtocol/unit_protocol_and_containerPort_validation
=== RUN   TestCreateServer_AllocationProtocol/integration_persistence
    store_servers_reverification_test.go:200: TEST_DATABASE_URL not set — skipping allocation persistence check
--- PASS: TestCreateServer_AllocationProtocol (0.00s)
    --- PASS: TestCreateServer_AllocationProtocol/unit_protocol_and_containerPort_validation (0.00s)
    --- SKIP: TestCreateServer_AllocationProtocol/integration_persistence (0.00s)
=== RUN   TestIsServerRestoreBlocking_Reverification
=== RUN   TestIsServerRestoreBlocking_Reverification/unit_constants
=== RUN   TestIsServerRestoreBlocking_Reverification/integration_restoring_lock
    store_servers_reverification_test.go:410: TEST_DATABASE_URL not set — skipping restoring lock integration
=== RUN   TestIsServerRestoreBlocking_Reverification/integration_race
    store_servers_reverification_test.go:518: TEST_DATABASE_URL not set
--- PASS: TestIsServerRestoreBlocking_Reverification (0.00s)
    --- PASS: TestIsServerRestoreBlocking_Reverification/unit_constants (0.00s)
    --- SKIP: TestIsServerRestoreBlocking_Reverification/integration_restoring_lock (0.00s)
    --- SKIP: TestIsServerRestoreBlocking_Reverification/integration_race (0.00s)
PASS
ok  	gamepanel/forge/internal/store	0.980s
```

All unit sub-tests **PASS**; integration sub-tests **SKIP** (expected without `TEST_DATABASE_URL`, same as other integration tests like `TestIsServerRestoreBlocking` and `TestMigration043...`).

### Regression check:

```sh
go test ./forge/api/internal/store -run "TestValidateVariableValue|TestSeedGameTemplates|TestMountAllowlist|TestCreateServer_AllocationProtocol|TestIsServerRestoreBlocking" -count=1 -v | grep -E "PASS|FAIL"
# => PASS (0.938s), no FAIL
go vet ./forge/api/internal/store => no output
```

No existing tests broken.

---

## 5. Coverage Summary (Slice 200k LOC)

| Requirement | Before | After | Evidence |
|-------------|--------|-------|----------|
| `TestValidateVariableValue_RegexSlashDelim` (slash fix) | missing (only `RegexSlash` existed) | **added** 40 sub-cases + split helper | `store_egg_variables_test.go:7` existing + new `store_servers_reverification_test.go:20` with flags `i/m/s`, invalid `g/x`, escaped `\/`, etc. |
| `TestCreateServer_AllocationProtocol` (containerPort/protocol) | missing | **added** unit + integration | Validates `store_allocations.go:148-162` defaults, `090` unique key, `144` trigger; `GetAllocation`/`ListAllocations` round-trip |
| `TestMountAllowlist_BlocksSensitive` | missing (only granular) | **added** composite 23-sensitive + allowlist mode | Complements `store_mounts_ext_test.go:47,73,93` |
| `TestIsServerRestoreBlocking` (211 restoring lock) | integration-only, skipped | **supplemented** with unit constants `restoring_backup` + `serverStatusFromActual` + integration reverification | Existing `store_restore_lock_integration_test.go:18` preserved; new `Reverification` avoids name collision |

**Existing tests preserved:** All prior `TestValidateVariableValue_RegexSlash`, `TestValidateVariableValue_PaperImport`, `TestSeedGameTemplates_CountAndValidation`, `TestMountAllowlist_Blocks*`, `TestMountAllowlist_AllowsSrv` still **PASS**.

---

## 6. Files Changed / Created

- **Created:** `forge/api/internal/store/store_servers_reverification_test.go` (557 lines) — no modifications to existing source files; no breaking changes.
- **Created:** `audits/110-phase-08-tests/subagent-01-store-servers.md` (this report)

---

## 7. Notes / Caveats

- Integration persistence tests (`CreateAllocations` with `protocol`/`containerPort` uniqueness, `IsServerRestoreBlocking` full cycle) require `TEST_DATABASE_URL`; they `t.Skip` in CI without DB, matching prior integration tests. Unit portions always run and already cover the slash fix, mount allowlist, and protocol validation pre-tx checks.
- The 211 migration extends `server_actual_state` enum to include `restoring_backup`; unit test `TestIsServerRestoreBlocking_Reverification/unit_constants` asserts `ServerActualStateRestoringBackup == "restoring_backup"` and `serverStatusFromActual` mapping to guard future enum regressions.
- No `store_servers_reverification_test.go` previously existed; task's `ls ... | head` confirmed missing.
- Noted typo in original task glob: `ls forge/api/internal/store/*test*.go | head` — verified 45 test files exist.

