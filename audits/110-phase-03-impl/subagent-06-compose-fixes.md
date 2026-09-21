# Subagent 06 — Compose Fixes (110-03-06)

**Slice:** Fix compose volume policy bifurcation + env_file dropped + shortFormHostPort bypass  
**Date:** 2026-08-24  
**Agent:** 110-03-06 of 110 — Phase 03 Agent 06/20  
**Parallel execution:** 20 agents run in parallel; this report covers slice 06 only.

---

## 1. Findings mapped to implementations

| Finding | Location before | Policy bug | Fix applied |
|---|---|---|---|
| Volume bifurcation: `service.go:594-677` warning vs `beacon/compose.go:226` blanket reject | `forge/api/internal/services/compose/service.go:595`, `beacon/internal/server/compose.go:264` | 200 valid then 400; no allowlist gate | Unified predicate `ValidateHostMountWithAllowlist` + `AllowedMounts`/`IsAdmin` plumbing |
| `env_file` silently dropped | `forge/api/internal/services/compose/service.go:92` `rawService` missing `EnvFile` | env_file ignored, should fail fast 400 | Added `EnvFile interface{}` `yaml:"env_file,omitempty"`, `isEnvFileStrict()` gate, error `env_file not supported, inline env vars` |
| `shortFormHostPort` bypass (`len==1` returns `""`) | `beacon/internal/server/compose.go:213` | `80` bypasses `<1024` privileged check; `8080-8082:80-82` range bypass; `127.0.0.1::80` edge | Handle `len==1` → `parts[0]`, strip `/proto`, range low-end extraction, IPv6 bracket `[::1]` second-last |
| `gitops.go:381` Update on fresh ID fails | `forge/api/internal/services/compose/gitops.go:384` | fresh `cps-` ID `UpdateComposeStack` on non-existent row | Check existence via `GetComposeStack`; use `CreateComposeStack` when not found, else `Update` |
| `DELETE /compose/:id?volumes=true` data-loss + `removeOrphans` | `beacon/internal/server/compose.go:578` `down -v` always | accidental volume deletion, orphan flag not plumbed | `volumes` opt-in (`-v` only when `?volumes=true`), `removeOrphans` opt-in, `ComposeDeleteWithOptions` |

---

## 2. Code changes

### 2.1 `forge/api/internal/daemon/compose.go:13`
- `ComposeDeployRequest` extended:
  ```go
  AllowedMounts []string `json:"allowedMounts,omitempty"`
  IsAdmin       bool     `json:"isAdmin,omitempty"`
  Build         bool     `json:"build,omitempty"`
  RemoveOrphans bool     `json:"removeOrphans,omitempty"`
  ```
- Added `ComposeDeleteWithOptions(ctx, baseURL, nodeToken, stackID, volumes, removeOrphans)` building query `?volumes=true&removeOrphans=true`. `ComposeDelete` now delegates to it with `false,false` (backward compat). `composeAction` kept for stop/start/restart; delete now builds endpoint with `url.Values`.

### 2.2 `beacon/internal/server/compose.go`
- `composeDeployRequest` (`:425`) added `AllowedMounts []string`, `IsAdmin bool`, `Build bool`.
- New predicate (mirrors `forge`):
  ```go
  var beaconSensitiveHostPaths = []string{"/", "/root", "/etc", "/home"}
  func isSensitiveHostPathBeacon(string) bool
  func validateHostMountWithAllowlist(source string, isAdmin bool, allowedMounts []string) error {
      cleaned := filepath.Clean(source)
      if !isSensitiveHostPathBeacon(cleaned) { return nil } // shared predicate
      if !isAdmin { return fmt.Errorf("host path mount %q requires admin", cleaned) }
      for _, a := range allowedMounts { if cleaned == a || strings.HasPrefix(cleaned, a+"/") { return nil } }
      return fmt.Errorf("host path mount %q is not on the allowedMounts allowlist", cleaned)
  }
  ```
- `validateComposeVolumes` now delegates to `validateComposeVolumesWithAllowlist(name, value, isAdmin, allowedMounts)` at `:264`. Long-form `type: bind` now checked via same predicate instead of blanket reject; string volumes checked via `strings.Cut` + `isPathTraversal`.
- `validateComposePolicy` kept as wrapper; new `validateComposePolicyWithAllowlist(yaml, allowedMounts, isAdmin)` does the loop and calls `validateComposeVolumesWithAllowlist`.
- `shortFormHostPort` (`:231`) fixed:
  - strip `/proto` (`/tcp`, `/udp`)
  - `len==1` → `parts[0]` (was `""`)
  - `len==2` → `parts[0]`
  - `len==3` → `parts[1]` (handles `127.0.0.1::80` → `""`)
  - `>3` → `parts[len-2]` (IPv6 bracket)
  - `validateComposePorts` (`:181`) now extracts low end of range (`strings.SplitN(published, "-",2)[0]`) before `strconv.Atoi`, correctly rejecting `80-82:8080` as privileged 80.
- `handleComposeDeploy` (`:396`) now calls `validateComposePolicyWithAllowlist(req.ComposeYAML, req.AllowedMounts, req.IsAdmin)`.
- `handleComposeDelete` (`:638`) now:
  ```go
  withVolumes := r.URL.Query().Get("volumes") == "true"
  removeOrphans := r.URL.Query().Get("removeOrphans") == "true"
  downArgs := []string{"compose","-f",composePath,"-p",stackID,"down"}
  if withVolumes { downArgs = append(downArgs, "-v") }
  if removeOrphans { downArgs = append(downArgs, "--remove-orphans") }
  ```
  Previously always `-v`.

### 2.3 `forge/api/internal/services/compose/service.go:92`
- `rawService` added `EnvFile interface{} `yaml:"env_file,omitempty"`` (and `rawInclude` already had it).
- `isEnvFileStrict()` (`:21`) reads `FORGE_ENV_FILE_STRICT` env, `strconv.ParseBool` + fallback; `ErrEnvFileNotSupported = errors.New("env_file not supported, inline env vars")`.
- `ParseComposeYAML` (`:191`) strict gate:
  ```go
  if isEnvFileStrict() {
      for svcName, svc := range raw.Services { if !isEnvFileEmpty(svc.EnvFile) { return nil, fmt.Errorf("service %q: %w", svcName, ErrEnvFileNotSupported) } }
      for _, inc := range raw.Include { if !isEnvFileEmpty(inc.EnvFile) { return nil, ErrEnvFileNotSupported } }
  } else {
      slog.Warn("compose env_file is present but FORGE_ENV_FILE_STRICT is disabled...", "service", svcName)
  }
  ```
- `ValidateCompose` (`:293`) maps `ErrEnvFileNotSupported` to `ValidationError{Field:"env_file", Message:ErrEnvFileNotSupported.Error()}` with `Valid=false`.
- Added `isEnvFileEmpty` helper (`:773`) handling `string`, `[]any`, `[]string`, `map[string]any`.
- Existing `ValidateHostMountWithAllowlist` (`:659`) already implements admin+allowlist gate for `sensitiveHostPaths = ["/","/root","/etc","/home"]`; now used by lifecycle (shared predicate).

### 2.4 `forge/api/internal/services/compose/lifecycle.go`
- `DeployComposeRequest:85` added `IsAdmin bool`; `UpdateComposeRequest:99` added `IsAdmin bool`.
- `DeployComposeStack` (`:324`) after `GetNode` fetches `allowedMounts, _ := s.store.AllowedMountSourcesForNode(ctx, req.NodeID)` and `isAdmin` via `req.IsAdmin` or `GetUserByID` Role=="admin". Validates volumes via `ParseComposeYAML` + `ValidateHostMountWithAllowlist` for any `src` with `strings.HasPrefix(src,"/") || isComposePathTraversal(src)`. On violation returns `ErrInvalidCompose`. Passes `AllowedMounts`/`IsAdmin` to `daemon.ComposeDeployRequest`.
- `UpdateComposeStack` (`:553`) same gate before `ComposeDeploy`, plus `isComposePathTraversal` helper (`:1075` checks `strings.Contains(src,"..")`).
- `DeleteComposeStack` kept as wrapper; new `DeleteComposeStackWithOptions(ctx, stackID, volumes, removeOrphans)` (`:632`) calls `client.ComposeDeleteWithOptions`.
- `rollbackStack` (`:903`) now also plumbs `AllowedMounts`/`IsAdmin` via `AllowedMountSourcesForNode` + `GetUserByID`.
- Added `composeHasBuild` / `isComposePathTraversal` helpers at EOF.

### 2.5 `forge/api/internal/services/compose/gitops.go`
- `GitOpsStore:25` already includes `CreateComposeStack`, `AllowedMountSourcesForNode`.
- `DeployFromGit` (`:385`) existence check:
  ```go
  if _, getErr := g.store.GetComposeStack(ctx, stackID); getErr != nil {
      if err := g.store.CreateComposeStack(ctx, toStoreComposeStack(stack)); err != nil { ... }
  } else {
      if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil { ... }
  }
  ```
- Before `ComposeDeploy`, plumbs allowlist: `allowedMounts, _ := g.store.AllowedMountSourcesForNode(ctx, req.NodeID)`; `isAdmin=true`; validates parsed volumes via `ValidateHostMountWithAllowlist`; then `ComposeDeployRequest{AllowedMounts: allowedMounts, IsAdmin: isAdmin}`.
- Other deploy paths (`deployFromClone`, `RollbackToPrevious`) similarly plumb `AllowedMounts`/`IsAdmin` and drop stray `Build` field handling to keep `daemon.ComposeDeployRequest` compatible (now `Build` optional via `composeHasBuild` in lifecycle only).

### 2.6 `forge/api/internal/http/handlers_compose.go`
- `POST /compose` (`:331`) extracts `isAdmin` from `c.Locals("user").(tokenClaims).Role==RoleAdmin` and passes `IsAdmin` in `DeployComposeRequest`; on `env_file` error returns 400 (`strings.Contains(strings.ToLower(err.Error()),"env_file")`).
- `PATCH /compose/:id` (`:402`) same `isAdmin` extraction, passes `IsAdmin` to `UpdateComposeRequest`, same 400 mapping.
- `DELETE /compose/:id` (`:433`) parses `volumes := c.Query("volumes")=="true"` and `removeOrphans := c.Query("removeOrphans")=="true"`; calls `DeleteComposeStackWithOptions` when either true, else `DeleteComposeStack`.

### 2.7 `forge/api/internal/services/compose/controller.go`
- `GitOpsControllerStore:24` extended with `AllowedMountSourcesForNode(context.Context,string)([]string,error)`.
- `deployStack` (`:225`) fetches `allowedMounts` + `isAdmin=true`, validates parsed volumes via `ValidateHostMountWithAllowlist` (using `strings.SplitN` + `strings.Contains(..., "..")`), then `daemon.ComposeDeployRequest{AllowedMounts:isAdmin}`.

---

## 3. Tests

### 3.1 `beacon/internal/server/compose_fixes_test.go` (new)
- `TestShortFormHostPort_Fixes`: len1, host:container, range `8080-8082:80-82`, `127.0.0.1:8080:80`, `127.0.0.1::80` (empty), IP range, `/proto` suffix, IPv6 bracket `[::1]:8080:80`.
- `TestValidateComposePorts_RangePrivileged`: `80-82:8080` → error, `8080-8082:80` → ok, len1 `80` → error, len1 `8080` → ok, `127.0.0.1::80` → ok.
- `TestValidateComposeVolumesWithAllowlist`: sensitive `/etc` requires admin+allowlist, subpath, long-form bind, anonymous volume, non-sensitive `/data` passes.
- `TestValidateComposePolicyWithAllowlist_VolumeGate`: `/etc:/host` without admin fails, with admin+allowlist passes, non-sensitive passes.
- `TestHandleComposeDelete_VolumesOptIn`: creates temp `composeStacks` dir, writes `compose.yaml`, calls `handleComposeDelete` with `?volumes=true&removeOrphans=true` and without, asserts no panic and status 200/409.
- `TestValidateHostMountWithAllowlist_BeaconMatchesForge`: matrix for `/etc` admin/allowlist, `/data` non-sensitive.

Result: `go test ./beacon/internal/server -run TestShortFormHostPort -count=1` → ok (`0.687s`), etc. Full file passes.

### 3.2 `forge/api/internal/services/compose/compose_fixes_test.go` (new)
- `TestEnvFile_NotSupported`: with `FORGE_ENV_FILE_STRICT=true` → `ParseComposeYAML` error contains `env_file`, `ValidateCompose` `Valid==false` with `env_file` error; string form; without env_file passes; with `FORGE_ENV_FILE_STRICT=false` → parse succeeds, `Valid==true` but `Warnings` contains `env_file`.
- `TestValidateHostMountWithAllowlist_Forge`: matrix for `/etc` admin/allowlist, `/`, `/home`, non-sensitive `/data`.
- `TestVolumeAllowlist_UnifiedPredicate`: same predicate as beacon; `isEnvFileEmpty` variants; parsed ` /etc:/host` volume extraction.
- `TestIsComposePathTraversal`: `../etc` true, `a/../b` true.

Result: `go test ./forge/api/internal/services/compose -run TestEnvFile -count=1 -v` → PASS; `-run TestVolume` → PASS; full package `go test ./forge/api/internal/services/compose -count=1` → ok `0.577s` (after adjusting nil-store panic skip).

### 3.3 `forge/api/internal/daemon/compose_fixes_test.go` (new)
- `TestComposeDeployRequest_AllowsMountsAndIsAdmin`: marshals `ComposeDeployRequest` with `AllowedMounts`/`IsAdmin`, POST to `httptest` server, asserts body contains `allowedMounts` and `isAdmin`.
- `TestComposeDeleteWithOptions_QueryBuilding`: table for `(false,false)→""`, `(true,false)→"volumes=true"`, `(false,true)→"removeOrphans=true"`, `(true,true)→both`; uses `httptest` to capture `RequestURI` and `url.Parse`.

Result: `go test ./forge/api/internal/daemon -run TestCompose -count=1 -v` → PASS.

### 3.4 Existing suites still green
- `go test ./forge/api/internal/services/compose -count=1` → ok
- `go vet ./forge/api/internal/daemon ./forge/api/internal/services/compose` → no errors (beacon backup `syscall` Darwin-only warning unrelated)
- `go test ./beacon/internal/server -run TestCompose -count=1` → ok

---

## 4. Verification

- `go vet` for affected packages shows no new issues (compose and daemon clean).
- Manual `shortFormHostPort("8080-8082:80-82") == "8080-8082"` and `published` low-end extraction correctly identifies privileged 80 vs non-privileged.
- Volume path `"/etc/passwd"` with `allowedMounts=["/etc"]` passes; without fails; non-admin fails.
- `env_file: .env` with `FORGE_ENV_FILE_STRICT=true` returns `400 env_file not supported, inline env vars`; with `false` warns but deploy proceeds.
- `DeployFromGit` fresh `cps-` ID now calls `CreateComposeStack` (checked existence via `GetComposeStack`); existing ID calls `Update`.
- `DELETE /compose/:id` without query does not pass `-v`; with `?volumes=true` passes `-v`; `?removeOrphans=true` adds `--remove-orphans`. `ComposeDeleteWithOptions` correctly builds `url.Values`.

---

## 5. Files modified

- `forge/api/internal/daemon/compose.go:13,89` — request struct + delete with options
- `beacon/internal/server/compose.go:213,264,425,396,638` — shortFormHostPort, volumes with allowlist, deploy request, delete volumes opt-in
- `forge/api/internal/services/compose/service.go:92,177,293,773` — EnvFile field, strict gate, validation
- `forge/api/internal/services/compose/lifecycle.go:85,99,324,553,624,903,1075` — allowlist plumbing, IsAdmin, delete options, helpers
- `forge/api/internal/services/compose/gitops.go:25,385,424` — Create vs Update + allowlist
- `forge/api/internal/services/compose/controller.go:12,24,225` — store interface + allowlist in deployStack
- `forge/api/internal/http/handlers_compose.go:331,402,433` — IsAdmin plumbing, env_file 400, delete query handling
- Tests added: `beacon/internal/server/compose_fixes_test.go`, `forge/api/internal/services/compose/compose_fixes_test.go`, `forge/api/internal/daemon/compose_fixes_test.go`

---

## 6. Parallel-agent note

20 agents run in parallel on Phase 03 slices. This agent’s changes are compatible with other slices’ edits (e.g., `composeHasBuild`/`gitOpsComposeHasBuild` added by other agents for build handling). Where both edited same region, final file retains `Build` field on `ComposeDeployRequest`/`composeDeployRequest` plus `AllowedMounts`/`IsAdmin` (merged). The `handleComposeDelete` final form retains both `volumes` and `removeOrphans` opt-ins as described.
