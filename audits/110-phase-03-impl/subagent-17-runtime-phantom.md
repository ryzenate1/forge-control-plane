# Subagent 17 — Runtime Phantom Providers Honesty Fix (110-03-17)

**Agent:** 110-03-17 / Phase 03 — Agent 17/20  
**Date:** 2026-08-24  
**Focus:** Fix phantom providers LXC/KVM silently → Docker, capability dead, StorageLocality vocab drift  
**Mode:** Low-risk honesty fix — make phantom visible (HTTP 400), do NOT implement real LXC/KVM  
**Flag:** `ENABLE_EXPERIMENTAL_RUNTIMES=true` gates future LXC/KVM work

---

## 1. Findings (as filed)

| # | Location | Finding |
|---|----------|---------|
| 1 | `forge/api/internal/runtime/runtime.go:9` 7 providers vs `beacon/internal/runtime/runtime.go:10` + `beacon/internal/runtime/factory.go:19-31` 5 | Forge advertises `lxc`/`kvm` (7 constants) but Beacon factory only handles 5. Phantom providers have adapters (`forge/api/internal/runtime/lxc.go:17`, `kvm.go:17`) that proxy to `daemon.CreateRequest{Provider: LXCProvider}` but Beacon `POST /servers` body drops `Provider` → always `mode:docker` (silent lie). |
| 2 | `beacon/internal/server/server.go:740` handler body struct | `Provider` field missing; decoded JSON silently discards it, returns `{"mode":"docker"}` for any provider. |
| 3 | `forge/api/internal/runtime/multiruntime.go:45` `getRuntimeForTarget` | Silent fallback to `defaultRuntime` when `target.Provider` unknown → phantom maps to docker without error. |
| 4 | `forge/api/internal/runtime/capabilities.go:118` `FirecrackerCapabilities()` | `Snapshots:true` but no snapshot RPC exists in `beacon/internal/runtime/firecracker.go` (nor in `FirecrackerAdapter`). Dead capability could mislead placement. |
| 5 | `forge/api/internal/runtime/registry.go:98` `CheckCapability` | Zero callers in production, no enforcement at scheduler — capability is declared but never gates scheduling. |
| 6 | `forge/api/internal/services/scheduler/service.go:212,317,320` `StorageLocality` | Dead vocab / drift: request `StorageLocality=="local_only"` vs candidate `StorageLocality=="local"`/`"shared"` (`nodeToCandidate:780`). `FilterNodes` penalty/bonus never fires because strings never match; `handlers_servers.go:860` never wires `StorageLocality` into `domain.PlacementRequest`. |

---

## 2. Implemented fix (low-risk, 110-03-17)

### 2.1 Beacon: `beacon/internal/server/server.go:740` — provider field + 400 guard

**file_path:line_number:** `beacon/internal/server/server.go:52-84`, `beacon/internal/server/server.go:795-845`

* Added `Provider string `json:"provider,omitempty"` to the `POST /servers` body struct (was missing).
* Added allowlist `supportedBeaconProviders` (docker, containerd, podman, firecracker, kubernetes) and helpers `isExperimentalRuntimeEnabled()` / `isSupportedBeaconProvider()`:
  ```go
  var supportedBeaconProviders = map[string]struct{}{
      runtime.ProviderDocker: {}, runtime.ProviderContainerd: {}, ...
  }
  func isExperimentalRuntimeEnabled() bool {
      v := strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_EXPERIMENTAL_RUNTIMES")))
      return v=="true"||v=="1"||v=="yes"
  }
  ```
* Handler now normalizes `provider := strings.ToLower(strings.TrimSpace(body.Provider))` and returns `400 unsupported provider: <x>` when `!isSupportedBeaconProvider(provider)` and experimental flag off. Phantom `lxc`/`kvm` are gated; `unknown-*` is rejected. When flag on, `lxc`/`kvm` pass validation (future wiring point).
* Response `mode` now reflects the validated provider (`mode=provider` if supplied, else `docker`) instead of always `docker`, so the lie is visible.

**Verification:** `beacon/internal/server/phantom_test.go:TestCreatePhantomProvider_Rejected` (400 for lxc/kvm/unknown, 202 for docker etc.), `TestCreatePhantomProvider_AllowedWithExperimentalFlag` (202 when `ENABLE_EXPERIMENTAL_RUNTIMES=true`), `go test ./beacon/internal/server -run TestCreatePhantomProvider -count=1` → `ok`.

### 2.2 Forge: `forge/api/internal/runtime/multiruntime.go:45` — supportedProviders + fallback guard

**file_path:line_number:** `forge/api/internal/runtime/multiruntime.go:10-45`, `forge/api/internal/runtime/multiruntime.go:107-121`

* Added `ErrUnsupportedProvider`, `supportedProviders` map (5 core), `isExperimentalRuntimesEnabled()`, `IsSupportedProvider()` / `ValidateProvider()`.
* `Register()` and `GetRuntime()` now normalize provider (`strings.ToLower(strings.TrimSpace(...))`).
* `getRuntimeForTarget(target Target) (Runtime, error)` now returns error for phantom:
  ```go
  if (normalized=="lxc"||normalized=="kvm") && !isExperimentalRuntimesEnabled() {
      return nil, fmt.Errorf("unsupported provider: %s: %w", target.Provider, ErrUnsupportedProvider)
  }
  if runtime, ok := m.GetRuntime(normalized); ok { return runtime, nil }
  if !IsSupportedProvider(normalized) {
      return nil, fmt.Errorf("unsupported provider: %s: %w", ..., ErrUnsupportedProvider)
  }
  // else fallback to default (supported but not registered)
  ```
  Previously returned `defaultRuntime` silently for any unknown → docker. Now phantoms surface as `ErrUnsupportedProvider` (HTTP 400).
* All `CreateServer`/`InstallServer`/`*Server`/`Stats`/`Inspect` etc. updated to `rt, err := m.getRuntimeForTarget(target); if err!=nil {return ..., err}`.
* `Prepare/Execute/CancelMigration` preserve `ErrMigrationManagedByControlPlane` when no default (maps `ErrRuntimeUnavailable`).

**Verification:** `forge/api/internal/runtime/phantom_test.go:TestCreatePhantomProvider_Rejected` (lxc/kvm/unknown → `ErrUnsupportedProvider`, docker/containerd → success, case-insensitive), `TestCreatePhantomProvider_AllowedWithExperimentalFlag`, `go test ./forge/api/internal/runtime -count=1` → `ok` (3 prior migration tests fixed to map `ErrRuntimeUnavailable` → `ErrMigrationManagedByControlPlane`).

### 2.3 HTTP validation: `forge/api/internal/http/handlers_servers.go:860` / `server.go:592`

**file_path:line_number:** `forge/api/internal/http/server.go:592`, `forge/api/internal/http/handlers_servers.go:21`, `forge/api/internal/http/handlers_servers.go:920-960`

* Added `StorageLocality string `json:"storageLocality"`` to `CreateServerRequest` (was missing → placement hint never reached scheduler).
* `POST /servers` handler now validates `Runtime` before scheduling:
  ```go
  if strings.TrimSpace(req.Runtime)!="" {
      if err:= gpruntime.ValidateProvider(req.Runtime); errors.Is(err, gpruntime.ErrUnsupportedProvider) {
          return fiber.NewError(fiber.StatusBadRequest, err.Error())
      }
  }
  ```
  and maps `clusterManager.CreateServer` `ErrUnsupportedProvider` → `400` (otherwise `502`).
* Wired `StorageLocality` into `domain.PlacementRequest` with normalization (`local_only` → `local`, lowercased, trimmed) at `handlers_servers.go:945-955`:
  ```go
  StorageLocality: func() string {
      s:=strings.ToLower(strings.TrimSpace(req.StorageLocality))
      if s=="local_only" {return "local"}
      return s
  }(),
  Runtime: strings.ToLower(strings.TrimSpace(req.Runtime)),
  ```

**Note:** `ENABLE_EXPERIMENTAL_RUNTIMES` gate is read via `os.Getenv` in both beacon and forge, no code change needed beyond validation.

### 2.4 Capabilities honesty: `forge/api/internal/runtime/capabilities.go:118`

**file_path:line_number:** `forge/api/internal/runtime/capabilities.go:118-125`

* `FirecrackerCapabilities()` now returns `Snapshots:false` (was `true`):
  ```go
  // Honesty fix 110-03-17: no snapshot op wired → advertising Snapshots:true was dead
  return Capabilities{MicroVM:true, ResourceLimits:true, Snapshots:false, Seccomp:true}
  ```
* Updated `forge/api/internal/runtime/capabilities_test.go:187-192` `TestFirecrackerCapabilities` to assert `!Snapshots`.

**file_path:line_number:** `forge/api/internal/runtime/registry.go:31-55`, `forge/api/internal/runtime/registry.go:98-115`

* `Register()` now logs `slog.Info("runtime registered", ...)` and increments `RuntimeOperationsTotal` metric, publishes `EventRuntimeRegistered` + `EventRuntimeCapabilityChanged` (already existed, now also logs). Comment: *not yet enforced at scheduler — just log*.
* `CheckCapability()` increments `RuntimeCapabilityChecksTotal` (already), now also `slog.Info` with provider/capability/supported and logs “provider not found” path. Stays observable via metrics+events, not gating placement (future enforcement point).

### 2.5 StorageLocality single vocabulary

**Decision:** Canonical vocabulary = **`local`** (matches `scheduler/service.go:780` candidate default). Legacy `local_only` (evacuationplanner) is aliased via normalization — both sides normalize to `local`.

**file_path:line_number:** `forge/api/internal/services/scheduler/service.go:817-851`

* Already present (pre-existing in this branch, verified): `canonicalStorageLocality()`, `storageLocalityEqual()`, `isLocalStorageLocality()`, and `normalizeRequest()` which now does `req.StorageLocality = canonicalStorageLocality(req.StorageLocality)`.
* `FilterNodes:227` now uses `isLocalStorageLocality(req.StorageLocality)` (was `req.StorageLocality=="local_only"`).
* `ScoreNodes:364,372` now uses `storageLocalityEqual(r.StorageLocality, req.StorageLocality)` with `canonicalStorageLocality` normalization (was raw `!=`/`==` → never matched `local` vs `local_only`).
* `nodeToCandidate:780` already emits `StorageLocality:"local"` / `"shared"` (consistent with canonical).
* `toWorkloadRequest:813` passes normalized `req.StorageLocality` through.
* `forge/api/internal/http/server.go:592` + `handlers_servers.go:945-955` (above) wire the placement hint from API into scheduler with same normalization (`local_only` → `local`).
* `evacuationplanner/service.go:31` still defines `StorageLocalOnly="local_only"` but scheduler canonicalization makes the comparison honest; no DB migration required because locality is computed from mounts, not stored string. (Future cleanup could change constant to `"local"`; aliased for now.)

**Placeholder wiring:** `handlers_servers.go:860` is the assignment block for `domain.PlacementRequest`; storage locality is now included there (previously omitted → dead code).

---

## 3. Tests

### 3.1 `TestCreatePhantomProvider_Rejected` (required)

**Forge adapter:** `forge/api/internal/runtime/phantom_test.go:9-72`

* `t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES","")` → `IsSupportedProvider(lxc)==false`, `ValidateProvider(lxc)` → `ErrUnsupportedProvider`, `M.CreateServer(Target{Provider:"lxc"})` → `ErrUnsupportedProvider` (not silent docker), same for `kvm`, `unknown-phantom`. Supported providers (`docker`, `containerd`, `podman`, `firecracker`, `kubernetes`, `""`) pass. Other ops (`SyncServerConfiguration`, `Stats`) also reject phantoms. `IsSupportedProvider` case/whitespace tolerant. `go test ./forge/api/internal/runtime -run TestCreatePhantomProvider_Rejected` → pass.

**Forge experimental flag:** `phantom_test.go:74-105` `TestCreatePhantomProvider_AllowedWithExperimentalFlag` with `ENABLE_EXPERIMENTAL_RUNTIMES=true` → `lxc`/`kvm` supported, `CreateServer` fallback to default or registered `lxc` runtime when present, case-insensitive.

**Beacon HTTP:** `beacon/internal/server/phantom_test.go:10-68` `TestCreatePhantomProvider_Rejected` posts `{"provider":"lxc"}` → `400 unsupported provider` and ensures `rt.createCalled==false`; `docker`/`""` → `202` with `mode:docker`; `TestCreatePhantomProvider_AllowedWithExperimentalFlag` verifies `202` when flag on; `TestCreatePhantomProvider_ModeReflectsProvider` checks response mode. `go test ./beacon/internal/server -run TestCreatePhantomProvider` → `ok`.

### 3.2 Existing tests updated

* `forge/api/internal/runtime/multiruntime_test.go:117-156` `GetRuntimeForTarget` suite now handles `(Runtime,error)` and distinguishes supported-unregistered fallback vs unsupported error.
* `forge/api/internal/runtime/capabilities_test.go:187` updated for `Snapshots:false`.
* Migration no-default tests (`multiruntime_test.go:479,496,513`) fixed to map `ErrRuntimeUnavailable` → `ErrMigrationManagedByControlPlane` (preserved expectation).
* `go test ./forge/api/internal/runtime -count=1` → `ok` (previously `FAIL` on miscounted capabilities & migration).
* `go test ./beacon/internal/server -run TestCreate -count=1` → `ok`.
* `go test ./forge/api/internal/services/scheduler -count=1` → `ok`.

---

## 4. Feature flag

`ENABLE_EXPERIMENTAL_RUNTIMES` (`true`/`1`/`yes`, case-insensitive, trimmed, `os.Getenv`) gates phantom providers in both planes:

* **Off (default):** `lxc`, `kvm` → `400 unsupported provider` at beacon `POST /servers` and forge `MultiRuntimeAdapter`/`POST /servers`. Silent docker fallback eliminated.
* **On:** `lxc`, `kvm` pass validation; if a runtime is registered for that provider, it is used; otherwise request falls back to default (same as other supported-but-not-registered providers). No real LXC/KVM implementation is added — they remain thin `daemon` proxies (`lxc.go:39`, `kvm.go:39` with `Provider:LXCProvider/KVMProvider`) that will now only be reachable when the flag is on, making the future implementation point explicit.

---

## 5. Files modified

```
beacon/internal/server/server.go                         — provider field, allowlist, 400 guard, mode fix
beacon/internal/server/phantom_test.go                   — NEW: beacon phantom 400 tests
forge/api/internal/runtime/runtime.go                    — (read-only; constants LXC/KVM remain but now gated)
forge/api/internal/runtime/capabilities.go               — Firecracker Snapshots:false
forge/api/internal/runtime/capabilities_test.go          — update expectation
forge/api/internal/runtime/registry.go                   — log + metric on Register, log on CheckCapability
forge/api/internal/runtime/multiruntime.go               — supportedProviders map, ValidateProvider, ErrUnsupportedProvider, normalized Register/GetRuntime, getRuntimeForTarget honesty guard, error propagation, migration ErrRuntimeUnavailable mapping
forge/api/internal/runtime/multiruntime_test.go          — update GetRuntimeForTarget expectations for new error path
forge/api/internal/runtime/phantom_test.go               — NEW: TestCreatePhantomProvider_Rejected + flag tests
forge/api/internal/http/server.go                        — CreateServerRequest.StorageLocality field
forge/api/internal/http/handlers_servers.go              — import gpruntime, Runtime validation 400, StorageLocality wiring + normalization, Runtime lowercasing, error mapping
forge/api/internal/services/scheduler/service.go         — (verified existing) canonicalStorageLocality, isLocalStorageLocality, storageLocalityEqual, normalizeRequest wiring; no additional change needed beyond handler wiring
```

---

## 6. Constraint compliance

* No real LXC/KVM runtime implemented — `lxc.go`/`kvm.go` remain daemon-proxy stubs; honesty fix only adds gating and 400 visibility.
* Change is low-risk: additive validation + normalization, no migration, no DB schema change, no beacon `DAEMON_RUNTIME_PROVIDER` startup path changed.
* All 400 paths include `unsupported provider: <value>` message (case preserved in error) for observability.
* Metrics (`RuntimeCapabilityChecksTotal`, `RuntimeOperationsTotal`) and events (`RuntimeRegistered`, `RuntimeCapabilityChanged`, `RuntimeUnavailable`) retained and augmented with `slog.Info` logging (not yet enforced at scheduler — scheduler scoring only logs, does not reject on capability mismatch).

---

## 7. How to verify

```bash
# forge phantom guard
go test ./forge/api/internal/runtime -run TestCreatePhantomProvider -count=1 -v

# beacon provider validation
go test ./beacon/internal/server -run TestCreatePhantomProvider -count=1 -v

# full runtime suite (includes updated firecracker & migration tests)
go test ./forge/api/internal/runtime -count=1

# scheduler storage locality canonicalization
go test ./forge/api/internal/services/scheduler -count=1

# manual smoke: phantom → 400, supported → 202
curl -X POST http://beacon:9090/servers -H 'Content-Type: application/json' \
  -d '{"serverId":"123e4567-e89b-12d3-a456-426614174000","image":"busybox","provider":"lxc"}' # → 400
ENABLE_EXPERIMENTAL_RUNTIMES=true go run ./beacon/cmd/daemon # then 202
```

---

## 8. Future work (not in this PR)

* Real LXC/KVM backends under `ENABLE_EXPERIMENTAL_RUNTIMES` — implement `beacon/internal/runtime/{lxc,kvm}.go` and extend `Factory.CreateRuntime` beyond 5 providers, add volume/snapshot handling, and flip `FirecrackerCapabilities.Snapshots` back to `true` when snapshot RPC exists.
* Enforce `CheckCapability` at scheduler (currently only logs). Add `scheduler.FilterNodes` capability gate behind a feature flag, using `registry.CheckCapability(node.RuntimeProvider, CapabilitySnapshots)` etc., with metrics `placement_rejected_capability_mismatch_total`.
* Migrate `evacuationplanner.StorageLocalOnly` constant from `"local_only"` to `"local"` (or add typed alias) and update persisted `EvacuationItem.Reason` strings for consistency.
* Expose `StorageLocality` in OpenAPI (`forge/api/docs/openapi.json:10222` `CreateServerRequest`) and regenerate clients.

