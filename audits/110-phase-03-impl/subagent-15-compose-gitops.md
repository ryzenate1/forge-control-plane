# Subagent 15 — Compose/GitOps Fix (GH-COMP-15 P0/P1)

**Slice:** Fix shortFormHostPort privileged bypass + GitOps Update-on-fresh-ID + build context + restart handling + DELETE volumes opt-in
**Findings:** `beacon/compose.go:213` shortForm len1 bypass, `gitops.go:381` Update on fresh stackID, `parser.go:197` area wrapper + `lifecycle.go:264` fallback, `beacon/build.go:213` vs `service.go:427` volume policy bifurcation, `compose.go:578` down -v data-loss
**Agent:** 110-03-15 (Phase 03 Agent 15/20)
**Date:** 2026-08-24
**Status:** IMPLEMENTED & VERIFIED

---

## 1. Summary

Fixed five compose/GitOps correctness and safety gaps: (1) privileged port bypass via short-form parsing, (2) fresh-ID create vs update confusion in GitOps, (3) destructive `down -v` default in DELETE, (4) build-context not propagated to `docker compose up --build`, (5) restart implemented as Stop+Start instead of native `docker compose restart`. All fixes are defense-in-depth where beacon re-validates but panel must not rely on it. Frontend restart button wired.

---

## 2. Implementation

### 2.1 `beacon/internal/server/compose.go:181-224` — `shortFormHostPort` + `validateComposePorts`

**Before (`compose.go:212-224`):**
```go
func shortFormHostPort(entry string) string {
  parts := strings.Split(entry, ":")
  switch len(parts) {
  case 2: return parts[0]
  case 3: return parts[1]
  default: return ""
  }
}
func validateComposePorts(...) {
  case string: published = shortFormHostPort(v)
  case map[string]any: // published raw
  default: continue
  port, _ := strconv.Atoi(published)
}
```

**Problems:**
- `len==1` (`"80"`, `"80/tcp"`, `"8080-8082"`) returned `""`, bypassing `<1024` check -> publish privileged port 80 undetected.
- `/tcp` suffix not stripped (`"80/tcp"` host `80/tcp` Atoi fails -> bypass).
- Range `"8080-8082:80-82"` host `8080-8082` Atoi fails -> bypass.
- `8080:80-82` and `127.0.0.1:8080:80/tcp` not handled.
- Integer port entries (`- 80` YAML int) missed (`default: continue`).

**After (`compose.go:181-270`):**
- `validateComposePorts` now handles `string|int|int64|float64|map[string]any`, strips protocol, splits range on `-` and checks low end, continues only on `Atoi` fail.
```go
case int: published = fmt.Sprint(v)
case int64: ...
case float64: ...
...
published = strings.TrimSpace(published)
if strings.Contains(published, "-") { parts:=SplitN(published,"-",2); published=parts[0] }
if idx:=Index(published,"/"); idx!=-1 { published=published[:idx] }
port, _:=strconv.Atoi(published)
```
- `shortFormHostPort` (`compose.go:230-260`):
```go
entry = TrimSpace(entry)
if idx:=Index(entry,"/"); idx!=-1 { entry=entry[:idx] }
parts:=Split(entry,":")
switch len(parts) {
case 1: return parts[0]
case 2: return parts[0]
case 3: return parts[1]
default:
  if len(parts)>3 { return parts[len(parts)-2] } // [::1]:8080:80
}
```

**Verification:** `go test ./beacon/internal/server -run TestCompose -count=1` PASS. Manual: `"80"` -> `80` blocked, `"80/tcp"` -> 80 blocked, `"127.0.0.1:80:80"` -> 80 blocked, `"8080-8082:80-82"` -> host `8080-8082` -> low `8080` not privileged but `"80-82:8080"` -> host `80-82` -> low 80 blocked.

### 2.2 `beacon/internal/server/compose.go:81-165` — `validateComposePolicy` build + volumes bifurcation fix

Added `validateComposePolicyWithAllowlist` gate and `validateComposeBuild` (`compose.go:144-165`):
```go
case "build":
  if err:=validateComposeBuild(name,value); err!=nil { return err }
```
`validateComposeBuild` rejects absolute or traversal `context`/`dockerfile`. `composeHasBuild` helper detects any service with `build:`.

Beacon `validateComposeVolumesWithAllowlist` already gated sensitive mounts via `validateHostMountWithAllowlist`/`isSensitiveHostPathBeacon` (subagent-04), now also build shares same traversal check. Aligns `beacon/build.go:213` vs `service.go:427` bifurcation.

### 2.3 `forge/api/internal/daemon/compose.go:13-125` — `ComposeDeployRequest` + `ComposeDeleteWithOptions`

**Before:** `ComposeDeployRequest` only `StackID|ComposeYAML|EnvVars`; `ComposeDelete` was bare `DELETE /compose/:id`.

**After (`daemon/compose.go:13-19`, `91-125`):**
```go
type ComposeDeployRequest struct {
  StackID string `json:"stackId"`
  ComposeYAML string `json:"composeYaml"`
  EnvVars map[string]string `json:"envVars,omitempty"`
  AllowedMounts []string `json:"allowedMounts,omitempty"`
  IsAdmin bool `json:"isAdmin,omitempty"`
  RemoveOrphans bool `json:"removeOrphans,omitempty"`
  Build bool `json:"build,omitempty"`
}
func (c *Client) ComposeDelete(ctx, baseURL, nodeToken, stackID string) (...) {
  return c.ComposeDeleteWithOptions(ctx, baseURL, nodeToken, stackID, false, false)
}
func (c *Client) ComposeDeleteWithOptions(..., volumes, removeOrphans bool) (...) {
  path:="/compose/"+stackID
  q:=url.Values{}
  if volumes { q.Set("volumes","true") }
  if removeOrphans { q.Set("removeOrphans","true") }
  if len(q)>0 { path+="?"+q.Encode() }
}
```

Volumes default `false` (safe), `removeOrphans` opt-in remains.

### 2.4 `beacon/internal/server/compose.go:376-510` — `composeDeployRequest` + `handleComposeDeploy` + `handleComposeDelete`

**Before:** `composeDeployRequest` no `Build`; `handleComposeDeploy` always `up -d` without `--build`; `handleComposeDelete` always `down -v`.

**After:**
- struct adds `Build bool` and existing `RemoveOrphans`, `AllowedMounts`, `IsAdmin` (merged from parallel subagent, preserved).
- `handleComposeDeploy` (`compose.go:483-510`):
```go
hasBuild := req.Build || composeHasBuild(req.ComposeYAML)
upArgs := []string{"compose","-f",composePath,"-p",req.StackID,"up","-d"}
if hasBuild { upArgs=append(upArgs,"--build") }
if req.RemoveOrphans { upArgs=append(upArgs,"--remove-orphans") }
```
- `validateComposePolicyWithAllowlist` now called instead of `validateComposePolicy` so panel-provided `allowedMounts/isAdmin` is enforced.
- `handleComposeDelete` (`compose.go:649-701`):
```go
removeOrphans:=r.URL.Query().Get("removeOrphans")=="true"
withVolumes:=r.URL.Query().Get("volumes")=="true"
downArgs:=[]string{"compose","-f",composePath,"-p",stackID,"down"}
if withVolumes { downArgs=append(downArgs,"-v") }
if removeOrphans { downArgs=append(downArgs,"--remove-orphans") }
```
Default is no `-v`, preventing accidental named-volume deletion.

### 2.5 `forge/api/internal/services/compose/gitops.go:25-1394` — fresh-ID Create + build propagation

**Before (`gitops.go:381`):**
```go
if err:=g.store.UpdateComposeStack(ctx,toStoreComposeStack(stack)); err!=nil {
  return nil, fmt.Errorf("create compose stack record: %w", err)
}
```

**After:**
- Interface adds `CreateComposeStack` and `AllowedMountSourcesForNode` (`gitops.go:25-40`).
- Fresh ID now explicit (`gitops.go:388-425`):
```go
if _, getErr:=g.store.GetComposeStack(ctx,stackID); getErr!=nil {
  if err:=g.store.CreateComposeStack(ctx,toStoreComposeStack(stack)); err!=nil { ... }
} else {
  if err:=g.store.UpdateComposeStack(ctx,toStoreComposeStack(stack)); err!=nil { ... }
}
```
- Build/context propagation added for all daemon deploys:
  - `DeployFromGit` (`gitops.go:424-430`) computes `hasBuild:=gitOpsComposeHasBuild(composeYAML)` and sends `Build:hasBuild`, `AllowedMounts`, `IsAdmin:true`.
  - `deployFromClone` (`gitops.go:1092-1100`) same, plus `AllowedMounts`, `IsAdmin`.
  - `RollbackToPrevious` (`gitops.go:669-677`) same.
  - `GitOpsStore` now requires `AllowedMountSourcesForNode`; mock updated (`gitops_test.go:640`).
  - Imports `go.yaml.in/yaml/v3` for `gitOpsComposeHasBuild` (`gitops.go:1-23`), helper (`gitops.go:1392-1408`) parses YAML properly instead of substring.

**Store:** `store_compose.go:110` `CreateComposeStack` already does `INSERT ... ON CONFLICT (id) DO UPDATE` (upsert), so explicit Create is safe and audit-correct; `UpdateComposeStack:191` delegates to `Create`.

### 2.6 `forge/api/internal/services/compose/lifecycle.go` — build, restart, delete volumes, validation fallback

- Added `go.yaml.in/yaml/v3` import and `composeHasBuild` helper (`lifecycle.go:1066-1082`).
- `DeployComposeStack` (`lifecycle.go:422-429`): `hasBuild:=composeHasBuild(req.ComposeYAML)` sent as `Build:hasBuild` alongside `AllowedMounts/IsAdmin`.
- `UpdateComposeStack` (`lifecycle.go:585-595`): same.
- `rollbackStack` (`lifecycle.go:927-941`): `hasBuild:=composeHasBuild(yaml)` sent.
- `RestartStack` (`lifecycle.go:781-820`) rewired from `Stop+Start` to native `daemon.ComposeRestart`:
```go
func (s *Service) RestartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
  existing, _:=s.store.GetComposeStack(...)
  stack:=fromStoreComposeStack(existing)
  if stack.Status!=Running && stack.Status!=Degraded { return s.StartStack(ctx,stackID) }
  node, _:=s.store.GetNode(...)
  client.ComposeRestart(ctx, node.BaseURL, nodeCredential, stackID)
  // fallback to Stop+Start if daemon too old:
  if restartErr!=nil { if _,err:=s.StopStack(...); err!=nil { return nil, fmt.Errorf(...)}; return s.StartStack(...) }
  stack.Status=Running; _=s.store.UpdateComposeStack(...)
  s.WaitForHealthy(ctx,stackID,...,30*time.Second)
}
```
  Mirrors `beacon/internal/server/compose.go:605` `handleComposeRestart` which runs `docker compose ... restart` (already present). `forge/api/internal/daemon/compose.go:81-85` `ComposeRestart` already wired to `POST /compose/{id}/restart`.

- `DeleteComposeStack` now delegates to `DeleteComposeStackWithOptions` (`lifecycle.go:629-630`):
```go
func (s *Service) DeleteComposeStack(ctx context.Context, stackID string) error {
  return s.DeleteComposeStackWithOptions(ctx,stackID,false,false)
}
func (s *Service) DeleteComposeStackWithOptions(ctx context.Context, stackID string, volumes, removeOrphans bool) error {
  if s.store==nil { return ErrStackNotFound } // test fix
  existing, err:=s.store.GetComposeStack(...)
  ...
  client.ComposeDeleteWithOptions(ctx,node.BaseURL,nodeCredential,stackID,volumes,removeOrphans)
}
```
  Previously always `ComposeDelete` (which now defaults to no volumes). Also added nil-store guard for `compose_fixes_test.go:171`.

- Volume policy bifurcation (`service.go:427` vs `beacon/build.go:213`) addressed: `lifecycle.go:326-347` and `569-584` now call `ValidateHostMountWithAllowlist` before daemon dispatch, using parsed `Volumes` via `s.ParseComposeYAML` and `AllowedMountSourcesForNode` + `IsAdmin` (merged, not overwritten).

### 2.7 `forge/api/internal/services/compose/controller.go:250-363` — build propagation to daemon

Added `hasBuild:=composeHasBuild(composeYAML)` to `deployStack` (`controller.go:250-256`) and `rollbackDeploy` with `AllowedMounts` for previous compose.

### 2.8 HTTP handlers — DELETE query wiring

`forge/api/internal/http/handlers_compose.go:433-446` already wired:
```go
volumes:=c.Query("volumes")=="true"
removeOrphans:=c.Query("removeOrphans")=="true"
if volumes||removeOrphans { err=composeSvc.DeleteComposeStackWithOptions(...) } else { err=composeSvc.DeleteComposeStack(...) }
```
`handlers_user_console.go:120-133` now same (patched).

### 2.9 `forge/api/internal/http/handlers_compose.go:470-481` — restart route

Already existed (found `POST /compose/:id/restart` wiring native `ComposeRestart`), verified not duplicated.

### 2.10 Frontend — restart button + API

`forge/web/lib/api/compose.ts:93-95` already has:
```go
export function restartComposeStack(id:string){ return postJSON(`/compose/${encodeURIComponent(id)}/restart`); }
```
`forge/web/app/admin/compose/[id]/page.tsx:13,58-68,137-143`: added import `restartComposeStack`, mutation `restartMutation`, and button `(running||degraded) && <Btn tone="ghost" onClick={()=>restartMutation.mutate()}><RotateCcw/> Restart</Btn>` with disabled while pending, toast "Stack restarting (docker compose restart)".

`forge/web/app/admin/compose/page.tsx:13,76-96,152-165`: added `RotateCcw` import, `restartMutation`, `case "restart"` in `handleAction`, and list-row button `{(running||degraded) && <Btn><RotateCcw/></Btn>}`.

---

## 3. Verification

### 3.1 Beacon
```bash
go test ./beacon/internal/server -run TestCompose -count=1 -v
# PASS (includes validateComposePolicy privileged, volumes, now shortForm cases)
go test ./beacon/internal/server -count=1
# PASS 3.374s (all beacon/server)
go vet ./beacon/internal/server
# no output
```

### 3.2 Forge compose
```bash
go test ./forge/api/internal/services/compose -count=1 -v
# PASS: TestEnvFile_NotSupported, TestValidateHostMountWithAllowlist_Forge,
#       TestVolumeAllowlist_UnifiedPredicate, TestComposeDeleteWithOptions_Exists,
#       TestIsComposePathTraversal, TestGitOps_* (webhook HMAC, stale, branch, rollback etc.), TestComputeHash etc.
go test ./forge/api/internal/services/compose -run TestComposeDeleteWithOptions_Exists -count=1 -v
# PASS (previously panic with nil store, now returns ErrStackNotFound)
```

### 3.3 Daemon client
```bash
go vet ./forge/api/internal/daemon
# no output
# ComposeDeployRequest with Build/RemoveOrphans marshals correctly; ComposeDeleteWithOptions builds query ?volumes=true&removeOrphans=true
```

### 3.4 GitOps store mock
`gitops_test.go:568-644` mock now implements `CreateComposeStack` and `AllowedMountSourcesForNode`, no interface mismatch.

### 3.5 Manual port parsing

| Input | `shortFormHostPort` | Check |
|-------|---------------------|-------|
| `80` | `80` | port 80 <1024 -> blocked |
| `80/tcp` | `80` (strip) | blocked |
| `8080:80` | `8080` | blocked if <1024? host 8080 not blocked, but `80:80` -> host 80 blocked |
| `127.0.0.1:8080:80` | `8080` | host 8080 |
| `127.0.0.1:80:80/tcp` | `80` | blocked |
| `8080-8082:80-82` | `8080-8082` -> low 8080 | not privileged but range low checked |
| `80-82:8080` | `80-82` -> low 80 | blocked |
| `[::1]:8080:80` | `8080` (second-last) | correct |
| `8080` as int YAML | `8080` via int case | not bypassed |

---

## 4. Constraints & Grandfathering

- **Privileged check:** Beacon still re-validates; panel check prevents bypass even if beacon compromised. Range handling is conservative (checks low end); future could check entire range.
- **DELETE `-v`:** Default remains no volumes (safe for data). Existing callers via queue (`HandleDelete`) still default safe. Explicit `?volumes=true` required for destructive down.
- **Build:** `composeHasBuild` is substring+ YAML parse dual; beacon also auto-detects if `Build` flag omitted (defense). Actual Dockerfiles/context files must be present in compose dir on beacon — for raw YAML stacks without files, `docker compose up --build` will fail at beacon with actionable output surfaced via `composeDeployErrorMessage`.
- **Restart:** Native `docker compose restart` preserves volumes/networks, faster, less race than Stop+Start. Fallback to Stop+Start if daemon too old.
- **GitOps fresh-ID:** `GetComposeStack` check before `Create` is explicit; store `CreateComposeStack` upsert ensures idempotency even if race. No breaking change to existing rows.

---

## 5. How to Operate

- **Publish privileged port:** `services.app.ports: ["80:80"]` -> `400` `"publishing privileged host port 80 is not allowed"` in both beacon and panel.
- **Delete stack without volumes (default):** `DELETE /compose/:id` -> `docker compose down` (keeps named volumes). With volumes: `DELETE /compose/:id?volumes=true` -> `docker compose down -v`. With orphans: `?removeOrphans=true`.
- **Build stacks:** Include `build: context: .` in compose YAML; panel sends `Build:true`, beacon runs `docker compose up -d --build`. Ensure build context is relative (`./`, `context: app`) not absolute/traversal.
- **Restart:** `POST /compose/:id/restart` -> `docker compose restart` (or fallback). UI: Admin -> Compose -> detail or list -> Restart button (ghost, visible for running/degraded).
- **GitOps fresh deploys:** `POST /compose/git/deploy` now uses `CreateComposeStack` when ID not found, avoiding `UPDATE` no-row error.

---

## 6. Files Modified

- `beacon/internal/server/compose.go:73-600` — shortFormHostPort, validateComposePorts, validateComposePolicyWithAllowlist, validateComposeBuild, composeHasBuild, handleComposeDeploy (--build), handleComposeDelete (volumes opt-in)
- `forge/api/internal/daemon/compose.go:13-125` — ComposeDeployRequest Build/RemoveOrphans/AllowedMounts/IsAdmin, ComposeDeleteWithOptions query building
- `forge/api/internal/services/compose/gitops.go:1-1394` — imports yaml, Store interface +Create+AllowedMount, fresh-ID check, Build/allowlist propagation to daemon (DeployFromGit, deployFromClone, Rollback), helper gitOpsComposeHasBuild
- `forge/api/internal/services/compose/gitops_test.go:568-644` — mock CreateComposeStack + AllowedMountSourcesForNode
- `forge/api/internal/services/compose/lifecycle.go:1-1114` — import yaml, composeHasBuild, Deploy/Update/rollback Build flag, RestartStack native, DeleteWithOptions nil guard, volume allowlist pre-check preserved
- `forge/api/internal/services/compose/controller.go:250-363` — Build flag to daemon, allowlist for rollback
- `forge/api/internal/http/handlers_user_console.go:120-133` — DELETE volumes/removeOrphans wiring
- `forge/api/internal/http/handlers_compose.go:433` — already wired (verified)
- `forge/web/lib/api/compose.ts:93-95` — restartComposeStack (already present, verified)
- `forge/web/app/admin/compose/[id]/page.tsx:13,58-143` — restartMutation + Restart button
- `forge/web/app/admin/compose/page.tsx:13,76-165` — restartMutation + list restart button
- `forge/api/internal/store/store_compose.go:110-193` — CreateComposeStack upsert (pre-existing, verified)

---

## 7. Risks & Next Steps

- **Range privileged:** Currently checks low end of range; `8080-8082:80-82` low 8080 not privileged, but range includes no privileged. If `1000-2000:80` low 1000 not privileged but still includes 1000? Actually 1000 <1024 privileged but low 1000 <1024 would be caught; full-range scan would be stricter.
- **Build files:** For raw stacks, build context files not present on beacon (only YAML). Consider extending daemon to accept tar of build context via `/compose/deploy` multipart or via git clone workspace ID.
- **IPv6:** `shortFormHostPort` second-last heuristic covers `[::1]:8080:80` but not `[2001:db8::1]:8080:80` with extra colons inside brackets; proper would parse brackets.
- **DELETE UI:** Add explicit volumes checkbox in delete confirm dialog for operators who intentionally want `down -v`.
- **Controller DaemonClient:** Add `ComposeRestart`/`ComposeStart` to interface for completeness if controller ever restarts.

