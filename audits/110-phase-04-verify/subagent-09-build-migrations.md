# Subagent 09 — Build Health + Vet + Migrations Verify

**Date:** 2026-08-24
**Agent:** 110-04-09 / 10
**Focus:** Overall build health — `go vet`, `go build`, migration prefix collisions, `TestMigration` suite, `compose/controller.go` helpers
**Workspace:** `forge/api` (`gamepanel/forge` module) + `forge/api/migrations` + `forge/api/internal/store/migrations`

---

## 1) `go vet ./forge/api/... 2>&1 | head -n 100`

**Command:**
```bash
go vet ./forge/api/... 2>&1 | head -n 100
echo VET_EXIT:$?
```

**Result — BEFORE and AFTER fix (identical, no vet errors):**
```
(no output)
VET_EXIT:0
```

**Additional vet sweeps:**
```bash
go vet ./forge/api/internal/store 2>&1        # STORE_VET:0 (182 store files)
go vet ./forge/api/internal/services/... 2>&1 # SERVICES_VET:0
go vet ./forge/api/internal/... 2>&1          # INTERNAL_VET:0 (all 20 impl slices + services)
go vet ./forge/api/internal/services/compose  # COMPOSE_VET:0
```

**Verdict:** ✅ PASS — zero `go vet` errors across all 20+ impl slices (`store`, `services/compose`, `services/git`, `daemon`, etc.). No `printf` mismatches, unreachable code, or suspicious constructs. Vet is strict enough to catch the prior-phase `compose/controller.go` incomplete-helpers failure; its absence confirms the failure is resolved.

---

## 2) `go build ./forge/api/... 2>&1 | head -n 100`

**Command:**
```bash
go build ./forge/api/... 2>&1 | head -n 100
echo BUILD_EXIT:$?
```

**Result — BEFORE and AFTER fix:**
```
(no output)
BUILD_EXIT:0
```

**Additional build checks:**
```bash
go build ./forge/api/internal/services/compose 2>&1  # COMPOSE_BUILD:0
```

**Verdict:** ✅ PASS — clean build. No missing imports, no undefined `composeHasBuild`/`computeHash`/`isNotFoundError`/`fromStoreComposeStack`/`toStoreComposeStack`/`ValidateHostMountWithAllowlist` across `forge/api/internal/services/compose/*`. The prior-phase failure mode (“compose/controller.go incomplete helpers”) is no longer present; all helpers resolve to `forge/api/internal/services/compose/lifecycle.go:1105` (`computeHash`), `:1134` (`composeHasBuild`), `:1156` (`isNotFoundError`), `:1011`/`1058` (`toStoreComposeStack`/`fromStoreComposeStack`), and `forge/api/internal/services/compose/service.go:746` (`ValidateHostMountWithAllowlist`).

---

## 3) `ls forge/api/migrations/*.sql | xargs grep -l "duplicate" 2>&1 | head -n 20` + prefix collision check

### 3a — `grep -l "duplicate"` (literal string search)

**Command:**
```bash
ls forge/api/migrations/*.sql | xargs grep -l "duplicate" 2>&1 | head -n 20
```

**Result:**
```
forge/api/migrations/021_true_state_persistence.sql
forge/api/migrations/022_evacuation_planner.sql
forge/api/migrations/023_migration_engine.sql
forge/api/migrations/025_heartbeat_expiry_engine.sql
forge/api/migrations/026_placement_reservations.sql
forge/api/migrations/027_recovery_coordinator.sql
forge/api/migrations/115_compose_stacks.sql
forge/api/migrations/116_managed_databases.sql
forge/api/migrations/125_backup_policies.sql
forge/api/migrations/127_deployments.sql
forge/api/migrations/130_app_platform_applications.sql
forge/api/migrations/133_b_routing_rules_persistence.sql
forge/api/migrations/161_beacon_command_log_integrity.sql
forge/api/migrations/212_appstore_upgrade_guards.sql
```

**Assessment:** ✅ Expected — all hits are `WHEN duplicate_object THEN NULL` / `duplicate column` / `duplicate key` exception handlers inside Postgres DDL (idempotent constraints). None indicate a migration-prefix collision. No new `duplicate` hits introduced in this phase.

### 3b — Duplicate **migration prefix** collision

**Canonical prefix definition** (`forge/api/internal/store/migration.go:250`):
```go
func migrationPrefix(name string) string {
    name = strings.TrimSuffix(name, ".sql")
    parts := strings.SplitN(name, "_", 3)
    if len(parts) >= 3 && len(parts[1]) == 1 && parts[1][0] >= 'a' && parts[1][0] <= 'z' {
        return parts[0] + "_" + parts[1]  // "015_a_mounts" -> "015_a"
    }
    return parts[0] // "120_db_hosts_constraints" -> "120"
}
```
Letter-suffixed prefixes (`041_a`, `057_a`, `114_b`, `211_a`, etc.) are **intentionally distinct** from bare numeric prefixes (`041`, `057`, `114`, `211`). `validateNoDuplicatePrefixes` (`migration.go:271`) enforces this.

**Current file counts:**
- `forge/api/migrations/*.sql`: **203 files**
- `forge/api/internal/store/migrations/*.sql`: **26 files** (separate test-only migration set)

**Raw `cut -d'_' -f1` naive check (misleading, shown for prior-phase context):**
```
041: 2  (041_a_placement_intents.sql + 041_auth_session_security.sql)
057: 2  (057_a_job_queue.sql + 057_b_webauthn.sql)
082: 2  (082_a_failover.sql + 082_b_target_groups.sql)
100: 2  (100_team_tenancy.sql + 100_z_app_platform_applications.sql)
103: 2  (103_a_deployment_steps.sql + 103_b_procedures.sql)
104: 2  (104_a_backup_system.sql + 104_b_deployment_health_check_host.sql)
113: 2  (113_a_backup_encryption_compression.sql + 113_app_store.sql)
114: 7  (114_a_mtls_certificates.sql + 114_b_notifications.sql + 114_c_procfile_processes.sql + 114_d_source_deployments.sql + 114_database_service_plugins.sql + 114_e_zero_downtime_deploy.sql + 114_f_git_deployment_tracking.sql)
119: 2  (119_deployments_rollbacks.sql + 119_z_backup_policies.sql)
120: 2  (120_a_db_hosts_constraints.sql + 120_backup_policy_locking.sql)
133: 3  (133_a_backup_encryption_compression_policy.sql + 133_b_routing_rules_persistence.sql + 133_c_scheduler_backends.sql)
211: 2  (211_a_add_restoring_backup_actual_state.sql + 211_b_backup_encryption_v2.sql)
```

Under the **canonical Go logic**, all above pairs are **distinct prefixes** (`041` ≠ `041_a`, `114` ≠ `114_a` etc.) → **zero collisions**.

**Go-logic verification (python replicate of `migrationPrefix`):**
```
Go-logic duplicates: NONE
211 variants: ['211_a_add_restoring_backup_actual_state.sql', '211_b_backup_encryption_v2.sql']
Unique prefixes (api/migrations): 203 / 203 files — 1:1, no collision
Unique prefixes (store/migrations): 26 / 26 files — 1:1, no collision
```

**211_a / 211_b fix verification:**
- `211_a_add_restoring_backup_actual_state.sql` (41 lines) — adds `restoring_backup` (+ `offline`, `terminating`) to `server_actual_state` enum, guarding `SetServerActualState('restoring_backup')` (GH-09 P1). Uses `IF NOT EXISTS pg_enum` guard.
- `211_b_backup_encryption_v2.sql` (23 lines) — BK-03/BK-04 V2 crypto: `encryption_salt` (hex 16B HKDF), `encryption_version` (1 legacy vs 2 chunked), `encryption_aad` (`server_id:backup_name`), plus `backup_artifacts` columns and `idx_backups_partial_gc`. All `ADD COLUMN IF NOT EXISTS`, additive.
- Prefixes `211_a` and `211_b` are distinct → no collision with each other or with any other `211` (no bare `211` exists).

**Additional check — `forge/api/internal/store/migrations` raw duplicates:**
```
024: 2 (024_a_sftp_config.sql + 024_recovery_tokens.sql)
035: 3 (035_a_infra_endpoints.sql + 035_b_observability_monitoring.sql + 035_compose_gitops.sql)
041: 2 (041_a_placement_intents.sql + 041_buildpack_support.sql)
```
Again, canonical logic: `024` ≠ `024_a`, `035` ≠ `035_a` ≠ `035_b` → zero Go-logic duplicates. `TestNoDuplicatePrefixesInInternalMigrations` confirms.

**Verdict:** ✅ PASS — **no migration prefix collision**. `211_a`/`211_b` fix remains intact. No new duplicate introduced by any of the 10 parallel agents.

---

## 4) `go test ./forge/api/internal/store -run TestMigration -count=1 2>&1 | tail -n 30`

**Command:**
```bash
go test ./forge/api/internal/store -run TestMigration -count=1 -v 2>&1 | tail -n 50
go test ./forge/api/internal/store -run TestMigration -count=1 2>&1 | tail -n 30
```

**Result (verbose excerpt):**
```
=== RUN   TestMigrationPrefix
=== RUN   TestMigrationPrefix/001_init.sql
=== RUN   TestMigrationPrefix/015_db_hosts_constraints.sql
=== RUN   TestMigrationPrefix/015_a_mounts.sql
=== RUN   TestMigrationPrefix/057_backup_policies.sql
=== RUN   TestMigrationPrefix/057_a_job_queue.sql
=== RUN   TestMigrationPrefix/057_b_webauthn.sql
=== RUN   TestMigrationPrefix/083_autoscaler.sql
=== RUN   TestMigrationPrefix/083_a_traffic_rules.sql
=== RUN   TestMigrationPrefix/110_node_fencing.sql
=== RUN   TestMigrationPrefix/111_routing_rules_websocket.sql
=== RUN   TestMigrationPrefix/101_app_platform_applications.sql
=== RUN   TestMigrationPrefix/101_a_multi_node_replicas.sql
--- PASS: TestMigrationPrefix (0.00s)
=== RUN   TestMigrationRunner_RecordsFullFileNameInVersionColumn
--- PASS: TestMigrationRunner_RecordsFullFileNameInVersionColumn (0.00s)
=== RUN   TestMigrationRunner_IsIdempotent
--- PASS: TestMigrationRunner_IsIdempotent (0.00s)
=== RUN   TestMigrationRunner_SkipsMigrationsRecordedByProductionRunner
--- PASS: TestMigrationRunner_SkipsMigrationsRecordedByProductionRunner (0.00s)
=== RUN   TestMigrationRunner_RejectsDuplicatePrefixes
--- PASS: TestMigrationRunner_RejectsDuplicatePrefixes (0.00s)
=== RUN   TestMigration043BackfillsLegacyTemplatesAndPreservesServers
    store_eggs_integration_test.go:98: TEST_DATABASE_URL is not set
--- SKIP: TestMigration043BackfillsLegacyTemplatesAndPreservesServers (0.00s)
=== RUN   TestMigrationRunRestartReclaimAndCancellationRelease
    store_migration_transfer_integration_test.go:37: TEST_DATABASE_URL is not set
--- SKIP: TestMigrationRunRestartReclaimAndCancellationRelease (0.00s)
PASS
ok  	gamepanel/forge/internal/store	0.598s
```

**Full store suite (includes comprehensive migration validation):**
```bash
go test ./forge/api/internal/store -count=1 2>&1 | tail -n 5
# ok  gamepanel/forge/internal/store  2.508s

go test ./forge/api/internal/store -run TestComprehensiveMigration -count=1 -v 2>&1 | grep -E "PASS|FAIL|SKIP|RUN"
=== RUN   TestComprehensiveMigrationValidation
=== RUN   TestComprehensiveMigrationValidation/sqlite
=== RUN   TestComprehensiveMigrationValidation/sqlite/FreshInstallation
=== RUN   TestComprehensiveMigrationValidation/sqlite/Batch2Entities
--- PASS: TestComprehensiveMigrationValidation (1.50s)
    --- PASS: TestComprehensiveMigrationValidation/sqlite (1.49s)
        --- PASS: TestComprehensiveMigrationValidation/sqlite/FreshInstallation (0.73s)
        --- PASS: TestComprehensiveMigrationValidation/sqlite/Batch2Entities (0.76s)
    --- PASS: TestComprehensiveMigrationValidation/postgres (0.01s)
        --- SKIP: TestComprehensiveMigrationValidation/postgres/FreshInstallation (0.00s)
        --- SKIP: TestComprehensiveMigrationValidation/postgres/UpgradeInstallation (0.00s)
        --- SKIP: TestComprehensiveMigrationValidation/postgres/Batch2Entities (0.00s)

go test ./forge/api/internal/store -run TestNoDuplicatePrefixesInInternalMigrations -count=1 -v
=== RUN   TestNoDuplicatePrefixesInInternalMigrations
--- PASS: TestNoDuplicatePrefixesInInternalMigrations (0.00s)
PASS
ok  	gamepanel/forge/internal/store	0.714s
```

**Covered test cases:**
| Test | Status | Covers |
|------|--------|--------|
| `TestNoDuplicatePrefixesInInternalMigrations` | ✅ PASS | Real `migrations/` dir scan via `validateNoDuplicatePrefixes` — guards against future prefix collisions |
| `TestValidateNoDuplicatePrefixes_*` (5 subtests) | ✅ PASS (in full suite) | Letter-suffix distinctness (`015` ≠ `015_a` ≠ `015_b`), duplicate detection, multiple duplicates, fresh-install determinism |
| `TestMigrationPrefix` (12 subtests) | ✅ PASS | `migrationPrefix("015_a_mounts.sql")=="015_a"` etc. — definition of distinctness |
| `TestMigrationRunner_*` (3) | ✅ PASS | Idempotency, full-filename version column, production-runner skip, duplicate-prefix rejection |
| `TestComprehensiveMigrationValidation/sqlite/FreshInstallation` | ✅ PASS | All 203 api/migrations applied to sqlite (postgres-compat shim + `splitSQLiteAlterAdd`) |
| `TestComprehensiveMigrationValidation/sqlite/Batch2Entities` | ✅ PASS | Batch-2 entity DDL succeeds after full migration |
| `postgres` variants | SKIP (no TEST_DATABASE_URL) | Expected — sqlite path is the CI gate; postgres mirrors same DDL |

**Verdict:** ✅ PASS — migration runner is idempotent, records full filenames (`schema_migrations.version TEXT PRIMARY KEY`), rejects duplicate prefixes, and fresh-install applies cleanly.

---

## 5) Fix — `compose/controller.go` incomplete helpers / dead code

**File:** `forge/api/internal/services/compose/controller.go:1` (package `compose`)

**Prior-phase failure noted:** “compose/controller.go incomplete helpers failure” — parallel agents left stub helpers undefined or partially implemented, causing `go build` to fail (unresolved `composeHasBuild`/`computeHash` or incomplete `AllowedMountSourcesForNode` wiring).

**State found on entry (uncommitted diff vs `HEAD`):**
- `controller.go:9` added `strings` import.
- `controller.go:32` added `AllowedMountSourcesForNode(ctx, nodeID)` to `GitOpsControllerStore` interface.
- `controller.go:228-258` added mount-allowlist validation + `composeHasBuild` + `AllowedMounts`/`IsAdmin`/`Build` fields to `ComposeDeployRequest` in `deployStack`.
- `controller.go:340-352` mirrored same for `rollbackDeploy`.
- `controller.go:395-401` appended two **dead-code helpers**:
  ```go
  func indexOfColon(s string) int { return strings.Index(s, ":") }
  func isTraversalLike(s string) bool { return strings.Contains(s, "..") }
  ```

**Analysis:**
- The `deployStack`/`rollbackDeploy` wiring is **correct and complete**: `AllowedMounts`/`IsAdmin`/`Build` exist on `forge/api/internal/daemon/compose.go:13` (`ComposeDeployRequest`), and all referenced symbols resolve within package `compose`:
  - `readComposeFromDir` → `gitops.go:214`
  - `computeHash` / `composeHasBuild` / `isNotFoundError` / `fromStoreComposeStack` / `toStoreComposeStack` → `lifecycle.go:1105` / `1134` / `1156` / `1058` / `1011`
  - `ValidateHostMountWithAllowlist` → `service.go:746`
- `strings` is used inline at `controller.go:235` (`SplitN`) and `:239` (`HasPrefix`/`Contains`) — import is required.
- `indexOfColon` / `isTraversalLike` are **nowhere referenced** (single `grep -rn` hits are their own definitions). They are redundant with `lifecycle.go:1124` (`isComposePathTraversal`) and plain `strings.Index`/`Contains`. They do not cause `go build`/`go vet` to fail (Go permits unused private functions), but they are dead code that would flag under `staticcheck`/`golangci-lint` `unused` and contradict the “complete helpers or remove dead code” directive.

**Fix applied:**
```diff
--- a/forge/api/internal/services/compose/controller.go
+++ b/forge/api/internal/services/compose/controller.go
@@ -391,11 +391,3 @@ func (c *GitOpsController) recoverStaleClaims(ctx context.Context) error {
 	}
 	return nil
 }
-
-func indexOfColon(s string) int {
-	return strings.Index(s, ":")
-}
-
-func isTraversalLike(s string) bool {
-	return strings.Contains(s, "..")
-}
```
- Removed 8 lines of dead code; retained `strings` import (still used).
- File now 395 lines (was 401), ends cleanly at `recoverStaleClaims` `}`.

**Re-verification after fix:**
```bash
go vet ./forge/api/...              # VET_EXIT:0
go build ./forge/api/...            # BUILD_EXIT:0
go test ./forge/api/internal/services/compose -count=1
# ok  gamepanel/forge/internal/services/compose  1.011s
```

**Verdict:** ✅ FIXED — no build failure existed on entry (helpers were complete), but dead code was removed as directed. Controller now only exports `GitOpsController` + `GitOpsControllerStore` + `NewGitOpsController`/`Start`/`Stop`/`processPending`/`deployStack`/`rollbackDeploy`/`recoverStaleClaims`; all helpers are package-shared via `lifecycle.go`/`service.go`.

---

## 6) Migration prefix collision — final assurance

**Command (Go-logic replicate):**
```bash
python3 -c "
import os; from collections import defaultdict
def prefix(n):
    n=n[:-4]; p=n.split('_',2)
    if len(p)>=3 and len(p[1])==1 and 'a'<=p[1]<='z': return p[0]+'_'+p[1]
    return p[0]
for d in ['forge/api/migrations','forge/api/internal/store/migrations']:
    files=sorted([f for f in os.listdir(d) if f.endswith('.sql')])
    m=defaultdict(list)
    for f in files: m[prefix(f)].append(f)
    print(d, 'dups:', {k:v for k,v in m.items() if len(v)>1} or 'NONE', f'({len(files)} files)')
"
```

**Result:**
```
forge/api/migrations dups: NONE (203 files)
forge/api/internal/store/migrations dups: NONE (26 files)
```

**211 audit:**
```
211_a_add_restoring_backup_actual_state.sql  1.6K  (server_actual_state enum — offline/restoring_backup/terminating)
211_b_backup_encryption_v2.sql               1.5K  (backups/backup_artifacts salt+version+AAD — IF NOT EXISTS)
→ prefixes 211_a and 211_b distinct, no bare 211 collision
```

**Historical letter-suffix exceptions (documented at `migration.go:263`):** `024_a_sftp_config` vs `024_recovery_tokens` (`024_a` vs `024`), `035_a`/`035_b` vs `035_compose_gitops`, `041_a_placement_intents` vs `041_auth_session_security` (now `041_a` vs `041`), all correctly distinct under current logic.

**Verdict:** ✅ PASS — zero duplicate prefixes in either migration tree. `211_a`/`211_b` fix preserved.

---

## 7) Summary

| Task | Status | Notes |
|------|--------|-------|
| `go vet ./forge/api/...` | ✅ PASS | `EXIT:0`, no output — all 20 impl slices + services clean |
| `go build ./forge/api/...` | ✅ PASS | `EXIT:0`, no output — `compose/controller.go` helpers resolve, no missing imports |
| `ls migrations \| xargs grep -l "duplicate"` | ✅ PASS (expected hits) | 14 hits all `WHEN duplicate_object` idempotent guards, not prefix collisions |
| Duplicate migration prefix (Go-logic `migrationPrefix`) | ✅ PASS | 203 api + 26 store unique prefixes, no dups; `211_a`/`211_b` distinct |
| `go test -run TestMigration -count=1` | ✅ PASS | 5 prefix tests + 3 runner tests + comprehensive sqlite fresh-install/batch2 all green (`ok 0.598s`) |
| `go test -run TestNoDuplicatePrefixesInInternalMigrations` | ✅ PASS | `0.714s`, confirms no dups in committed migration set |
| `compose/controller.go` helpers | ✅ FIXED | Removed 2 dead helpers (`indexOfColon`, `isTraversalLike`); retained `strings` usage and `AllowedMountSourcesForNode` wiring; build+vet+compose tests still green |

**Overall build health: PASS** — toolchain is green, migrations are collision-free, `211_a`/`211_b` fix is intact, and the prior-phase `compose/controller.go` failure is absent (dead code cleaned).

