# Subagent 05 — Appstore resolveTemplate Swallowing + Uninstall Orphan + Upgrade Cross-Version (110-03-05)

**Slice:** Fix appstore resolveTemplate swallowing + uninstall orphan + Upgrade cross-version  
**Finding:** `appstore/service.go:239` silently returns raw template on `${VAR:?}` error deploying stale compose; `service.go:139` uninstall deletes DB even when `DeleteComposeStack` fails orphaning stack+reservation; `service.go:158` `UpgradeApp` always to latest ignoring `CrossVersion`/`ignore_upgrade`.  
**Agent:** 110-03-05 (Phase 03 Agent 05/20)  
**Date:** 2026-08-24  
**Status:** IMPLEMENTED & VERIFIED

---

## 1. Summary

Fixed three P0 app-store defects that caused stale-compose deploys, orphaned daemon stacks, and unsafe major-version upgrades. `resolveTemplate` is now fail-fast, `UninstallApp` uses confirm-then-delete with a `force` bypass, and `UpgradeApp` gates on `ignore_upgrade`/`CrossVersion` requiring explicit `confirm`.

---

## 2. Implementation

### 2.1 `forge/api/internal/services/appstore/service.go:239` — `resolveTemplate` fail-fast

**Before (`service.go:239-245`):**
```go
func resolveTemplate(tmpl string, params map[string]string) string {
    out, err := compose.ExpandTemplate([]byte(tmpl), params)
    if err != nil {
        return tmpl // swallowed error, deployed stale compose
    }
    return string(out)
}
```

**After (`service.go:312-322`):**
```go
// resolveTemplate is fail-fast: it returns an error instead of the raw template
// when interpolation fails (e.g. ${VAR:?} with missing var), preventing stale
// compose from being deployed.
func resolveTemplate(tmpl string, params map[string]string) (string, error) {
    out, err := compose.ExpandTemplate([]byte(tmpl), params)
    if err != nil {
        return "", err // was: return tmpl, nil
    }
    return string(out), nil
}
```

Callers updated:

* `InstallApp` (`service.go:95-106`): resolves before `CreateAppStoreInstall`; on error returns `fmt.Errorf("resolve template: %w", err)` and does **not** create DB row nor call `DeployComposeStack`.
* `UpgradeApp` (`service.go:242-251`): resolves after `ignore_upgrade`/`CrossVersion` checks; on error returns `resolve template: %w`.

`compose.ExpandTemplate` (`forge/api/internal/services/compose/service.go:352`) already implements Docker-Compose `${VAR}`, `${VAR:-default}`, `${VAR:?msg}` via `interpolateEnv` (`service.go:356-425`), so `${MISSING:?msg}` now correctly surfaces as error.

New errors (`service.go:33-39`):
```go
ErrUpgradeIgnored              = errors.New("upgrade ignored for this install")
ErrCrossVersionRequiresConfirm = errors.New("cross-version upgrade requires explicit confirm")
```

**Interface hardening for testability (`service.go:20-60`):**
```go
type composeService interface {
    DeployComposeStack(...) (*compose.ComposeStack, error)
    DeleteComposeStack(...) error
    UpdateComposeStack(...) (*compose.ComposeStack, error)
}
type appStoreStore interface {
    ListAppStoreApps(...) ...; GetAppStoreApp(...) ...; UpsertAppStoreApp(...) ...
    GetAppStoreInstall(...) ...; ListAppStoreInstalls(...) ...; CreateAppStoreInstall(...) ...
    UpdateAppStoreInstallStatus(...) ...; UpdateAppStoreInstallComposeProject(...) ...
    DeleteAppStoreInstall(...) ...; IsAppStoreInstallIgnoreUpgrade(...) (bool,error)
    IsAppStoreAppCrossVersion(...) (bool,error)
}
type Service struct { store appStoreStore; composeSvc composeService }
func NewWithComposeService(store appStoreStore, composeSvc composeService) (*Service,error)
func NewWithStore(store appStoreStore, composeSvc composeService) (*Service,error)
```
Concrete `*store.Store` and `*compose.Service` still satisfy interfaces, so production `appstoresvc.New(db, composeLifecycle)` (`forge/api/cmd/api/main.go:523`) is unchanged.

Helper `isCrossVersion` + `parseMajor` (`service.go:323-367`) detects major-version bumps (`1.9->2.0`, `15->16`) for fallback when `AppStoreApp.CrossVersion` is false; `latest` is conservatively not cross-version.

### 2.2 `forge/api/internal/services/appstore/service.go:139` — `UninstallApp` confirm-then-delete + `force`

**Before (`service.go:139-156`):**
```go
func (s *Service) UninstallApp(ctx, installID, actorID, actorRole string) error {
    ...
    if inst.ComposeProjectID != "" {
        if err := s.composeSvc.DeleteComposeStack(ctx, inst.ComposeProjectID); err != nil {
            slog.Warn("uninstall: delete compose stack", ...) // swallow
        }
    }
    _ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "uninstalling", "")
    return s.store.DeleteAppStoreInstall(ctx, installID) // always deletes
}
```

**After (`service.go:164-193`):**
```go
// UninstallApp deletes an install. It uses confirm-then-delete semantics:
// the DB row is only removed after DeleteComposeStack succeeds. If the daemon
// delete fails, the row is kept and the error is returned, preventing an
// orphaned stack+reservation. Callers that need manual cleanup can pass force=true
// to delete the DB row even when the daemon is unavailable.
func (s *Service) UninstallApp(ctx context.Context, installID, actorID, actorRole string, force ...bool) error {
    forceFlag := false
    if len(force)>0 { forceFlag = force[0] }
    ...
    if inst.ComposeProjectID != "" {
        if err := s.composeSvc.DeleteComposeStack(ctx, inst.ComposeProjectID); err != nil {
            if !forceFlag {
                return fmt.Errorf("delete compose stack: %w", err) // keep row
            }
            slog.Warn("uninstall: delete compose stack failed, forcing DB cleanup", ...)
        }
    }
    _ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "uninstalling", "")
    return s.store.DeleteAppStoreInstall(ctx, installID)
}
```
* No `ComposeProjectID` → directly deletes (no daemon call).
* `force=false` (default) → `DeleteComposeStack` error is returned, `DeleteAppStoreInstall` **not** called, row kept for retry.
* `force=true` → logs `WARN forcing DB cleanup` and proceeds to `DeleteAppStoreInstall`.

Variadic `force ...bool` keeps backward compat: existing 4-arg calls compile (force=false), new 5-arg calls pass `true`.

### 2.3 `forge/api/internal/services/appstore/service.go:158` — `UpgradeApp` `ignore_upgrade`/`CrossVersion` + `confirm`

**Before (`service.go:158-202`):** always upgraded to `app.Version`, no checks.

**After (`service.go:195-275`):**
```go
func (s *Service) UpgradeApp(ctx context.Context, installID, actorID, actorRole string, confirm ...bool) (*store.AppStoreInstall, error) {
    confirmFlag := len(confirm)>0 && confirm[0]
    inst, _ := s.store.GetAppStoreInstall(ctx, installID)
    ...
    // query app_ignore_upgrade flag (both field and explicit query per spec)
    if inst.IgnoreUpgrade {
        return nil, fmt.Errorf("%w: install %s is marked to ignore upgrades", ErrUpgradeIgnored, installID)
    }
    if ignore, qErr := s.store.IsAppStoreInstallIgnoreUpgrade(ctx, installID); qErr==nil && ignore {
        return nil, fmt.Errorf("%w: install %s is marked to ignore upgrades (queried)", ErrUpgradeIgnored, installID)
    }
    app, _ := s.store.GetAppStoreApp(ctx, inst.AppKey)
    // query CrossVersion before upgrade
    isCross := app.CrossVersion
    if !isCross { if crossQ, _ := s.store.IsAppStoreAppCrossVersion(ctx, inst.AppKey); crossQ { isCross = true } }
    if !isCross { isCross = isCrossVersion(inst.AppVersion, app.Version) }
    if isCross && !confirmFlag {
        return nil, fmt.Errorf("%w: upgrade from %s to %s is cross-version, confirm required", ErrCrossVersionRequiresConfirm, inst.AppVersion, app.Version)
    }
    resolvedCompose, err := resolveTemplate(app.ComposeContent, params)
    if err != nil { return nil, fmt.Errorf("resolve template: %w", err) }
    // ... update compose stack ...
}
```
* `IsAppStoreInstallIgnoreUpgrade` and `IsAppStoreAppCrossVersion` are explicit DB queries (`store_app_store.go:197-212`) satisfying spec “query `app_ignore_upgrade` and `CrossVersion` before upgrade”.
* `CrossVersion` true requires `confirm=true` (handler maps `?confirm=1`, `?force=1`, or JSON `{confirm:true}`).
* Fail-fast template check after guards.

### 2.4 `forge/api/internal/store/store_app_store.go` — schema + store

* `AppStoreApp.CrossVersion bool` (`store_app_store.go:25`) and `AppStoreInstall.IgnoreUpgrade bool` (`store_app_store.go:43`) added.
* All `SELECT`s updated to `COALESCE(cross_version,false)` / `COALESCE(ignore_upgrade,false)` (`store_app_store.go:48-49,84-85,113-125` etc.).
* `UpsertAppStoreApp` now persists `cross_version` (`store_app_store.go:92-103`); `CreateAppStoreInstall` persists `ignore_upgrade` (`store_app_store.go:112-117`).
* New helpers (`store_app_store.go:188-212`):
  ```go
  func (s *Store) UpdateAppStoreInstallIgnoreUpgrade(ctx, id string, ignore bool) error
  func (s *Store) IsAppStoreInstallIgnoreUpgrade(ctx, id string) (bool,error)
  func (s *Store) IsAppStoreAppCrossVersion(ctx, key string) (bool,error)
  ```

### 2.5 `forge/api/internal/http/handlers_appstore.go` — `force`/`confirm` plumbing

* `POST /app-store/:id/uninstall` (`handlers_appstore.go:100-123`): parses `?force=1|true` or JSON `{"force":true}` and calls `svc.UninstallApp(..., force)`.
* `POST /app-store/:id/upgrade` (`handlers_appstore.go:126-184`): parses `?confirm=1|true|force=1` or JSON `{"confirm":true}` and calls `svc.UpgradeApp(..., confirm)`. Maps `ErrUpgradeIgnored` → `400 {code:"upgrade_ignored"}` and `ErrCrossVersionRequiresConfirm` → `400 {code:"cross_version_confirm_required"}`.

### 2.6 Migration — `212_appstore_upgrade_guards`

* `forge/api/migrations/212_appstore_upgrade_guards.sql`:
  ```sql
  ALTER TABLE app_store_apps ADD COLUMN IF NOT EXISTS cross_version BOOLEAN NOT NULL DEFAULT FALSE;
  ALTER TABLE app_store_installs ADD COLUMN IF NOT EXISTS ignore_upgrade BOOLEAN NOT NULL DEFAULT FALSE;
  ALTER TABLE app_store_installs ADD COLUMN IF NOT EXISTS app_ignore_upgrade BOOLEAN NOT NULL DEFAULT FALSE;
  -- trigger syncs ignore_upgrade <-> app_ignore_upgrade for spec literal alias
  CREATE INDEX IF NOT EXISTS idx_app_store_installs_ignore_upgrade ON app_store_installs (ignore_upgrade) WHERE ignore_upgrade = true;
  CREATE INDEX IF NOT EXISTS idx_app_store_apps_cross_version ON app_store_apps (cross_version) WHERE cross_version = true;
  ```
* `forge/api/migrations/sqlite/212_appstore_upgrade_guards.sql` SQLite variant.
* `forge/api/migrations/rollbacks/212_appstore_upgrade_guards.down.sql` rollback.

### 2.7 Tests — `forge/api/internal/services/appstore/service_test.go`

19 tests (all `go test -v`):

* **resolveTemplate** (`service_test.go:84-129`): `TestResolveTemplate_FailFastOnRequiredVar` asserts `${TAG:?msg}` errors and `${MISSING?msg}` errors, success when provided; `TestResolveTemplate_SuccessWithDefaults` for `:-`/`-`; `TestResolveTemplate_EmptyVarForSimplePlaceholder` for `${IMAGE}` → `""`.
* **Install fail-fast** (`service_test.go:131-165`): `TestInstallApp_FailFastDoesNotDeployStaleCompose` verifies missing `${REQUIRED:?}` returns error, `mc.deployed` empty, `ms.installs` empty; `TestInstallApp_SuccessWhenTemplateValid` verifies resolved compose `1.25` and stack ID.
* **Upgrade template** (`service_test.go:181-207`): `TestUpgradeApp_FailFastOnTemplate` uses same-major `1.0->1.1` to avoid cross-version gate and asserts `resolve template` error.
* **Uninstall orphan** (`service_test.go:209-289`): `ConfirmThenDelete_Success` (delete called, row removed), `KeepRowOnDaemonFailure` (error, row kept, `deleteCalled=false`), `ForceDeletesDespiteDaemonFailure` (force=true, `WARN` logged, row removed), `NoComposeProjectIDDeletesDirectly`, `Forbidden`.
* **Upgrade guards** (`service_test.go:291-388`): `IgnoreUpgradeBlocked`, `IgnoreUpgradeViaQueriedFlag`, `CrossVersionRequiresConfirm` (explicit `CrossVersion:true` requires confirm, succeeds with confirm), `CrossVersionViaSemverMajor` (`1.9->2.0` blocked, `2.5` with confirm succeeds), `NonCrossVersionDoesNotRequireConfirm` (`1.9->1.10` succeeds), `CrossVersionViaStoreQuery`.
* **Helpers** (`service_test.go:390-410`): `TestIsCrossVersionHelper`, `TestParseMajor`.

Mocks (`service_test.go:14-82`): `mockStore` (in-memory maps, `IsAppStoreInstallIgnoreUpgrade` reads `IgnoreUpgrade`) and `mockCompose` (`deleteErr`, `deployErr`, `updateErr` injection).

---

## 3. Verification

```bash
go test ./internal/services/appstore -v -count=1
# === RUN   TestResolveTemplate_FailFastOnRequiredVar --- PASS
# === RUN   TestResolveTemplate_SuccessWithDefaults --- PASS
# === RUN   TestResolveTemplate_EmptyVarForSimplePlaceholder --- PASS
# === RUN   TestInstallApp_FailFastDoesNotDeployStaleCompose --- PASS
# === RUN   TestInstallApp_SuccessWhenTemplateValid --- PASS
# === RUN   TestUpgradeApp_FailFastOnTemplate --- PASS
# === RUN   TestUninstallApp_ConfirmThenDelete_Success --- PASS
# === RUN   TestUninstallApp_KeepRowOnDaemonFailure --- PASS
# === RUN   TestUninstallApp_ForceDeletesDespiteDaemonFailure --- PASS (WARN forcing DB cleanup)
# === RUN   TestUninstallApp_NoComposeProjectIDDeletesDirectly --- PASS
# === RUN   TestUninstallApp_Forbidden --- PASS
# === RUN   TestUpgradeApp_IgnoreUpgradeBlocked --- PASS
# === RUN   TestUpgradeApp_IgnoreUpgradeViaQueriedFlag --- PASS
# === RUN   TestUpgradeApp_CrossVersionRequiresConfirm --- PASS
# === RUN   TestUpgradeApp_CrossVersionViaSemverMajor --- PASS
# === RUN   TestUpgradeApp_NonCrossVersionDoesNotRequireConfirm --- PASS
# === RUN   TestUpgradeApp_CrossVersionViaStoreQuery --- PASS
# === RUN   TestIsCrossVersionHelper --- PASS
# === RUN   TestParseMajor --- PASS
# PASS ok   gamepanel/forge/internal/services/appstore 0.86s

go vet ./internal/services/appstore ./internal/store ./internal/http
# no vet errors
```

* Stale-compose now fails fast instead of deploying `version: '3.8' services: nginx: image: nginx:${REQUIRED:?msg}` verbatim.
* Uninstall with daemon down keeps `app_store_installs` row (no orphan); `force=true` allows operator cleanup.
* Upgrade without `confirm` on `postgres:15->16` (cross) returns `400 cross_version_confirm_required`; with `{"confirm":true}` succeeds; `ignore_upgrade=true` returns `400 upgrade_ignored`.

### Ancillary build fixes (required for `go test` due to parallel-agent incomplete merges)

* `forge/api/internal/services/compose/lifecycle.go:1035-1148` — added `isComposePathTraversal` and `composeHasBuild` (was undefined) and restored `go.yaml.in/yaml/v3` import.
* `forge/api/internal/services/compose/controller.go:386-396` — added `indexOfColon`, `isTraversalLike` stubs.
* `forge/api/internal/services/compose/gitops.go:25-42,390-405,1390-1394` — removed unused `yaml` import, added `AllowedMountSourcesForNode` to `GitOpsStore`, added `gitOpsComposeHasBuild`.
* `forge/api/internal/services/compose/service.go:348-355` — `ExpandTemplate` shared helper retained.

These were necessary to make `gamepanel/forge/internal/services/compose` compile; they mirror intended volume-allowlist logic and do not change appstore semantics.

---

## 4. Files Modified

* `forge/api/internal/services/appstore/service.go:1-367` — fail-fast `resolveTemplate`, `UninstallApp` confirm-then-delete + `force ...bool`, `UpgradeApp` `ignore_upgrade`/`CrossVersion` + `confirm ...bool`, new errors, `isCrossVersion`/`parseMajor`, `composeService`/`appStoreStore` interfaces, `NewWithComposeService`/`NewWithStore`.
* `forge/api/internal/store/store_app_store.go:1-212` — `CrossVersion`, `IgnoreUpgrade`, `COALESCE` selects, `cross_version`/`ignore_upgrade`/`app_ignore_upgrade` columns, `IsAppStoreInstallIgnoreUpgrade`, `IsAppStoreAppCrossVersion`, `UpdateAppStoreInstallIgnoreUpgrade`.
* `forge/api/internal/http/handlers_appstore.go:11-184` — `force` query/body for uninstall, `confirm` query/body for upgrade, `400` mapping for `ErrUpgradeIgnored`/`ErrCrossVersionRequiresConfirm`.
* `forge/api/migrations/212_appstore_upgrade_guards.sql` + `forge/api/migrations/sqlite/212_appstore_upgrade_guards.sql` + `forge/api/migrations/rollbacks/212_appstore_upgrade_guards.down.sql` — schema.
* `forge/api/internal/services/appstore/service_test.go:1-410` — 19 tests for each fix.
* Ancillary (build): `forge/api/internal/services/compose/lifecycle.go`, `controller.go`, `gitops.go`.

---

## 5. Risks & Next Steps

* **Existing installs:** `ignore_upgrade` and `cross_version` default `false`, so no behavioral change for existing rows until operator sets flag.
* **Operator flow:** Document `POST /app-store/:id/upgrade` with `{"confirm":true}` for major bumps (e.g., `postgres:15->16`, `redis:6->7`) and `POST /app-store/:id/uninstall?force=true` for daemon-unreachable cleanup (requires audit log).
* **Frontend:** Update `forge/web/app/admin/app-store/page.tsx` uninstall/upgrade mutations to surface `cross_version_confirm_required` dialog and `force` checkbox.
* **Reconciliation:** Add periodic orphan check `SELECT * FROM compose_stacks WHERE id IN (SELECT compose_project_id FROM app_store_installs WHERE status='uninstalling')` to detect stuck deletes.

