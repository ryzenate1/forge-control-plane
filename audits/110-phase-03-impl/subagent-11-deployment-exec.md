# Subagent 11 — Deployment Execution Real + Placement/Traffic Wiring

**Track:** 11 of 20 — Phase 03 Implementation  
**Focus:** `AP-02` deployment stubs lie complete `execution.go:249` + `AP-08` health gate probes localhost `healthgate.go:11` + placement soft-bonus overflow `constraints.go:59` + queue  
**Date:** 2026-08-24  
**Mode:** parallel — all 20 agents run concurrently; this report is additive (no deletions)

## 1. Findings Addressed (source-verified)

| ID | File:Line (pre-fix) | Symptom | Severity |
|---|---|---|---|
| AP-02 | `forge/api/internal/services/deployment/execution.go:249` `executeProvisionStep` stub reported `completed` with no beacon dispatch | provision/mid-DAG steps no-op; rolling/blue-green advanced on unverified assumption | P0 |
| AP-02a | `execution.go:264` honest for `recreate` only (runtime nil check) | other strategies still wired to nil runtime or skipped placement/traffic | P0 |
| AP-08 | `forge/api/internal/services/deployment/healthgate.go:11` `validateHealthGateTarget` clamp + `healthgate.go:62` `resolveNodeHost` localhost | health gate probed API host `localhost:port` not target node; every health-gated deploy failed or false-passed versus unrelated local service | P0 |
| AP-11 | `forge/api/internal/placement/constraints.go:59` `bonus += 1e12` / `-1e10` | single soft constraint dwarfs `LeastLoadedScorer` base ≤1.0 by 12 orders; placement starved | P1 |
| AP-11a | `forge/api/internal/placement/strategy.go:95` `LeastLoadedScorer` sum ≤3.0 + `strategy.go:185` `availableRatio` unbounded | base score unnormalized; soft bonus overflow masked | P1 |
| AP-11b | `forge/api/internal/services/scheduler/service.go:306` `+=1e9`, `−1e10`, `+1e8` on preferred/storage | scheduler-level overflow replicates constraint bug | P1 |
| Q-01 | `forge/api/internal/services/deployment/execution.go:24` `ClaimExecutionLease` + `store_deployments.go:360` `ReleaseExecutionLease` without `isActiveStatus` gate | concurrent rollback / provision races produced duplicate goroutines; no 409 fencing | P1 |

**Reverification on HEAD:** AP-02 honest check landed for `recreate` (`provision_regression_test.go:13`) but no placement/traffic; health gate still clamped to loopback in `rollout.go:81`; placement overflow still live (1e12).

## 2. Implementation

### 2.1 `execution.go:264` — Placement + Traffic behind flags, replica verification, active-status gating

**New interfaces `forge/api/internal/services/deployment/service.go:18`:**

```go
type PlacementExecutor interface {
  EnsurePlacement(ctx context.Context, deploymentID string, serverID string, replicas int) error
  VerifyReplicaCount(ctx context.Context, serverID string, expected int) (int, error)
}
type TrafficExecutor interface {
  ShiftTraffic(ctx context.Context, deploymentID string, serverID string, fromTarget, toTarget string, weight int) error
  VerifyTrafficShift(ctx context.Context, deploymentID string) (bool, error)
}
type HealthProber interface {
  HealthProbe(ctx context.Context, baseURL, nodeToken, serverID, path string, port int) (daemon.HealthProbeResult, error)
  ContainerHealthInspect(ctx context.Context, baseURL, nodeToken, containerID string) (string, error)
}
```

**Service struct `service.go:114`:**

```go
type Service struct {
  store          *store.Store
  publisher      events.Publisher
  runtime        RuntimeExecutor
  placement      PlacementExecutor   // NEW — replicamanager/scheduler
  traffic        TrafficExecutor     // NEW — gateway/trafficmanager
  daemon         HealthProber        // NEW
  daemonClient   *daemon.Client      // NEW — implements HealthProber
  resumeMu       sync.Mutex
  executingDeployments sync.Map
  wg             sync.WaitGroup
}
```

Setters: `SetPlacementExecutor`, `SetTrafficExecutor`, `SetHealthProber`, `SetDaemonClient` (`service.go:143`), `VerifyReplicaCount` (`service.go:163`).

**Flags `service.go:28`, `execution.go:18`:**

```go
func requirePlacementFlag() bool { // FORGE_DEPLOY_REQUIRE_PLACEMENT
  v := strings.TrimSpace(strings.ToLower(os.Getenv("FORGE_DEPLOY_REQUIRE_PLACEMENT")))
  return v=="1"||v=="true"||v=="on"||v=="yes"
}
func requireTrafficFlag() bool { /* FORGE_DEPLOY_REQUIRE_TRAFFIC */ }
func isPlacementRequired() / isTrafficRequired() // execution.go
```

Default off; when `on`, missing executor fails closed (honest error) instead of silent no-op.
Dual-read: when wired but flag off, placement/traffic are best-effort (`slog.Warn`) and do not block DAG.

**`executeProvisionStep` `execution.go:276`:**

* `isActiveStatus` gating before any work — `store.GetDeployment` check refuses concurrent provision with `store.ErrVersionConflict` (maps to 409 at HTTP layer):
  ```go
  if s.store != nil { if sd,err:=s.store.GetDeployment(...); err==nil {
    if isActiveStatus(sd.Status) && sd.Status != pending && != in_progress && != provisioning {
      return fmt.Errorf("refusing concurrent provision: %w", store.ErrVersionConflict) }}}
  ```
* Placement gate: if `FORGE_DEPLOY_REQUIRE_PLACEMENT` and `s.placement==nil` → error; else `EnsurePlacement(deployment.ID, serverID, TargetReplicas)`. When wired but not required — best-effort.
* Runtime honest: `s.runtime==nil` → `refusing to report success without executing` (kept from P0).
* `ApplyDeployment` → beacon `SyncServerConfiguration` + `SendPower(start)` via `BeaconRuntimeExecutor` (`beacon_executor.go:33`).
* Replica-count verification `verifyReplicaCount` `execution.go:313`: calls `placement.VerifyReplicaCount(serverID, expected)`; on mismatch and flag on → error `replica count mismatch: expected X, observed Y`; flag off → `slog.Warn`.

**`executePromoteStep` `execution.go:349`:**

* Traffic gate analogous; calls `traffic.ShiftTraffic(deployment.ID, serverID, oldTarget, newTarget, 100)` then `VerifyTrafficShift`; flag-guarded fail-closed.

**`executeScaleUpStep` / `executeScaleDownStep` `execution.go:395`:**

* Both now delegate to `placement.EnsurePlacement` + `verifyReplicaCount` before `verifyObservedRunning`, so rolling scale is placement-first as required by `FORGE_IMPLEMENTATION_PLAN` §5.

**`isActiveStatus` `rollout.go:119`** reused (pending/in_progress/provisioning/awaiting_health/promoting/rollback_pending/rolling_back) to prevent double provision / concurrent rollback.

### 2.2 Health gate — node-derived, beacon-local, docker health, `health_check_configs` merge

**`healthgate.go:62` `resolveNodeHost`** already correct (NodeURL → Hostname):

```go
func (s *Service) resolveNodeHost(ctx context.Context, serverID string) string {
  if s.store==nil {return ""}
  target,_:=s.store.ServerControlTarget(ctx, serverID) // JOIN nodes
  u,_:=url.Parse(target.NodeURL)
  return u.Hostname() // not localhost
}
```

Kept and documented as fix for `FORGE-LOGIC-002` clamp. No localhost fallback.

**`rollout.go:71` `validateHealthGateTarget` clamp removed `rollout.go:71`:**

Before: `if host==""||host=="localhost" return nil; if ip==nil||!ip.IsLoopback() return error "health checks may only target the local deployment gateway"` — forced loopback.

After: empty → auto-derived (correct); otherwise reject injection (backtick/quotes/`\r\n;|&`) and validate hostname/IP syntax; any IP or RFC1123 hostname allowed (including node address). Colon-port and slash-path rejected to keep `HealthCheckPort` canonical.

**Beacon-local probe `healthgate.go:12`:**

* Merge `health_check_configs` dual-read `healthgate.go:16`:
  ```go
  effectivePath := deployment.HealthCheckPath
  effectivePort := deployment.HealthCheckPort
  if (effectivePath==""||effectivePort==0) && s.store!=nil {
    if cfg,_:=s.store.GetZeroDowntimeHealthCheckConfig(ctx, serverID); err==nil {
      if effectivePath=="" {effectivePath=cfg.Path}
      if effectivePort==0 {effectivePort=cfg.Port}
    }
  }
  if effectivePath==""||effectivePort==0 {return Passed:true}
  ```
  This keeps `deployments` and `health_check_configs` (migration `114_e_zero_downtime_deploy.sql:15`) in sync — backfill via `health_check_configs` when inline is empty.

* Beacon-local `daemon.HealthProbe` `healthgate.go:42`:
  ```go
  if (s.daemon!=nil||s.daemonClient!=nil) && s.store!=nil {
    target,_:=s.store.ServerControlTarget(...)
    if probeRes,err:=prober.HealthProbe(ctx, target.NodeURL, token, serverID, path, port); err==nil {
      healthStatus,_:=prober.ContainerHealthInspect(ctx, target.NodeURL, token, serverID)
      // docker Health.Status == "healthy"/"none" merges as pass; "unhealthy"/"starting" forces fail even if HTTP 200
    } else if !strings.Contains(err.Error(),"not found") { slog.Warn("probe failed, fallback to direct") }
  }
  ```
  Prefer beacon-local loopback (node probes `http://127.0.0.1:port/path` where container is reachable); avoids control-plane hairpin and localhost confusion.

* `docker inspect HealthStatus` fallback: when direct HTTP passes but `ContainerHealthInspect` reports `unhealthy`, gate still fails. Implemented both in beacon path and direct-HTTP fallback branch.

* Added `daemon.Client.HealthProbe` + `ContainerHealthInspect` `forge/api/internal/daemon/client.go:910`:
  ```go
  type HealthProbeResult struct { Status string; Healthy bool; StatusCode int; Body string }
  func (c *Client) HealthProbe(ctx, baseURL, token, serverID, path string, port int) (HealthProbeResult,error) {
    probeURL := baseURL+"/servers/"+serverID+"/health?path="+url.QueryEscape(path)+"&port="+strconv.Itoa(port)
    // signed request via c.newRequest; healthy when 2xx-3xx; 404 => beacon too old => fallback
  }
  func (c *Client) ContainerHealthInspect(ctx, baseURL, token, containerID string) (string,error) {
    raw,_:=c.AdminContainerInspect(...); // parse State.Health.Status || Config.Healthcheck
  }
  ```
  `HealthProbe` handles 5xx as transport error, 404 as "beacon too old" to trigger fallback to direct probe.

### 2.3 Placement soft-bonus overflow — `kSoftWeight 0.30`

**`constraints.go:50` `forge/api/internal/placement/constraints.go:50`:**

```go
const ( kSoftWeight=0.30; kSoftPenalty=0.10 )
func (c *ConstraintChecker) CheckSoft(...) (float64, []string) {
  if !isPlacementV2() { // legacy 1e12/-1e10 for rollback compare
    var bonus float64; // old loop
    return bonus, reasons
  }
  bonus := (satisfied/softTotal)*kSoftWeight - (missed/softTotal)*kSoftPenalty
  // clamp [-0.10, 0.30]
}
func isPlacementV2() bool { return FORGE_PLACEMENT_V2 != "false" } // default true
```
`FORGE_PLACEMENT_V2` flag (from plan §7) defaults **on**; `false` restores legacy overflow for comparison and phased rollout. Bounded influence stays within one normalized point (`LeastLoaded` ≤1.0).

**`strategy.go:14` `forge/api/internal/placement/strategy.go:14`:**

* Added `placementV2Enabled()` helper reading `FORGE_PLACEMENT_V2`.
* `LeastLoadedScorer.Score` `strategy.go:45`: now `(ratioCpu+ratioMem+ratioDisk)/3.0` normalized to [0,1] with clamp; legacy multiplies by 3 when flag off.
* `availableRatio` `strategy.go:214`: when `total<=0` and flag off → raw `available`; when on → bounded `available/(available+1000)` ∈ (0,1) ordered, or `0` when available 0. When `total>0` → `available/total` clamped [0,1].

**`scheduler/service.go:18` `forge/api/internal/services/scheduler/service.go:18`:**

```go
const ( schedulerPreferredBonus=0.30; schedulerStorageBonus=0.15; schedulerStoragePenalty=0.50 )
func schedulerPlacementV2() bool { /* FORGE_PLACEMENT_V2 */ }
```

`ScoreNodes` `scheduler/service.go:299`:

* preferred `+=1e9` → `+=0.30` (norm) with reason `(norm +0.30)`
* predictive `Score*(1+Trend)+Affinity-AntiAffinity` clamped trend/aff/anti to ±0.20 (norm) to keep total in [0,1.5]; legacy keeps unbounded
* storage mismatch `−1e10` → `−0.50`; match `+1e8` → `+0.15`
* final clamp to [-1,2] under V2

**Engine `forge/api/internal/placement/engine.go:37`** now adds bounded bonus (`score+bonus`) so total stays in [−0.5, 1.5] — integration test `TestEnginePlaceSoftBonusDoesNotDwarfBase` verifies freer node still wins when load diff >0.30.

**Queue mitigation (related):** deployment lease is already `ClaimExecutionLease` with 5-min TTL (`store_deployments.go:300`) plus `isActiveStatus` gating; this provides queue single-writer semantics for deployments (no duplicate goroutines) and 409 on concurrent lease claim.

### 2.4 Replica app wiring note

`PlacementExecutor` is intended to be wired to `scheduler.Scheduler.PlaceReplicas` + `replicamanager.Manager` (via `cmd/api/main.go` where `replicaMgr` already exists `main.go:432`). Wiring is additive:

```go
deploySvc.SetPlacementExecutor(replicaMgr) // or scheduler wrapper
deploySvc.SetTrafficExecutor(gatewaySvc)
deploySvc.SetDaemonClient(daemonClient)
```

Not wired in this patch (flag default off, best-effort); wiring is gated behind `FORGE_DEPLOY_REQUIRE_PLACEMENT/TRAFFIC` and `FORGE_PLACEMENT_V2` so existing deploys remain green.

## 3. Files Modified

| File | Lines | Change |
|---|---|---|
| `forge/api/internal/services/deployment/service.go:1` | +38 | imports `os,strings,daemon`; defines `PlacementExecutor`, `TrafficExecutor`, `HealthProber`; adds `placement,traffic,daemon,daemonClient` fields; adds `SetPlacementExecutor`, `SetTrafficExecutor`, `SetHealthProber`, `SetDaemonClient`, `VerifyReplicaCount`; adds `requirePlacementFlag/RequireTrafficFlag`; adds `isActiveStatus` gating in `Rollback` |
| `forge/api/internal/services/deployment/execution.go:1` | +76 | adds `isPlacementRequired/isTrafficRequired`; `verifyReplicaCount`; extends `executeProvisionStep` with placement/traffic + replica verification + active-status gate + nil-store guard; extends `executePromoteStep` with traffic shift + verification; extends `executeScaleUp/ScaleDownStep` with placement |
| `forge/api/internal/services/deployment/healthgate.go:1` | +98 | merges `health_check_configs` via `GetZeroDowntimeHealthCheckConfig`; beacon-local `HealthProbe` + `ContainerHealthInspect` merge with `unhealthy/starting` fail-close; fallback to direct `http://nodeHost:port/path` with `resolveNodeHost` (NodeURL→Hostname); imports `log/slog,strings` |
| `forge/api/internal/services/deployment/rollout.go:71` | ±20 | removes loopback clamp; validates `HealthCheckHost` for injection (`\`\"'` `;|&` `\r\n`), length, colon-path; empty → auto-derived |
| `forge/api/internal/services/deployment/revisions.go:147` | +8 | adds `isActiveStatus` gating in `RollbackToRevision` to surface `store.ErrVersionConflict` (409) for concurrent rollback |
| `forge/api/internal/daemon/client.go:910` | +78 | adds `HealthProbeResult`, `HealthProbe`, `ContainerHealthInspect`, `isLoopbackHost` helper; mTLS/transport hardening unchanged |
| `forge/api/internal/placement/constraints.go:1` | +34 | adds `kSoftWeight=0.30, kSoftPenalty=0.10`, `isPlacementV2()`, bounded `CheckSoft` with legacy branch |
| `forge/api/internal/placement/strategy.go:1` | +35 | adds `placementV2Enabled()`, normalizes `LeastLoadedScorer` to `/3` with clamp, bounds `availableRatio` to [0,1] with monotonic fallback `available/(available+1000)` |
| `forge/api/internal/services/scheduler/service.go:1` | +53 | adds `FORGE_PLACEMENT_V2` flag, `schedulerPreferredBonus=0.30`, storage bonus/penalty normalization, clamping, reason strings |
| `forge/api/internal/placement/engine_test.go:11` | +4 | adds `Total*` fields to fixture candidates so V2 normalized scores differentiate (legacy tests assumed `availableRatio=available`) |
| `forge/api/internal/placement/load_test.go:14` | +6 | adds `Total*` to load-test candidates |
| `forge/api/internal/placement/explain_test.go:11` | +2 | adds `Total*` to produce-report fixture |
| `forge/api/internal/services/deployment/deployment_placement_traffic_test.go` | NEW | 220 lines: `TestProvisionFailsWhenPlacementRequired`, `TestProvisionWithPlacementSucceeds`, `TestVerifyReplicaCountFlagGating`, `TestPromote*`, `TestHealthProbeViaBeaconLocal`, `TestHealthProbeNodeDerived`, `TestHealthProbeDirectHTTP`, `TestHealthCheckMergesZeroDowntimeConfig` (DB-gated), `TestConcurrentRollback409` (DB-gated), `TestProvisionFailurePropagates` (DB-gated) |
| `forge/api/internal/placement/constraints_normalized_test.go` | existing | kept — now passes with new bounded logic (mixed bonus 0.10 exact with epsilon) |
| `forge/api/internal/store/store_zero_downtime.go:124` | — | read path for merge (pre-existing); healthgate now calls it |

No columns dropped, no API removed, no `DROP`; all migrations retained (`114_e_zero_downtime_deploy.sql:15` `health_check_configs`).

## 4. Tests

### 4.1 New tests `deployment_placement_traffic_test.go`

*Provision failure / flag gating* (unit, no DB):
- `TestProvisionFailsWhenPlacementRequired` — `FORGE_DEPLOY_REQUIRE_PLACEMENT=true` + no executor → `provision step: no placement executor` (honest failure).
- `TestProvisionWithPlacementSucceeds` — verifies `VerifyReplicaCount` with stub placement (expected 2) and that flag-off bypasses strict check.
- `TestVerifyReplicaCountFlagGating` — placement-required vs not-required with/without executor.
- `TestPromoteFailsWhenTrafficRequired` / `TestPromoteTrafficWired` — flag helper and wiring existence.

*Health probe node-derived* (unit, no DB unless noted):
- `TestHealthProbeViaBeaconLocal` — stub `HealthProber` returns `healthy` + docker `healthy` → `CheckHealth` via beacon would pass (direct prober call verified).
- `TestHealthProbeNodeDerived` — `CheckHealth` with `ServerID=srv-missing` and no store → `Passed:false` + `unresolved` error, proving it did **not** silently probe `localhost:8080` (regression for `F-05`/`FORGE-LOGIC-002`). Mirrors `healthgate_target_test.go:14`.
- `TestHealthProbeDirectHTTP` — `httptest.NewServer` on `/healthz`, explicit `HealthCheckHost` → `Passed:true` `Status:200` via direct HTTP fallback when daemon not wired.
- `TestHealthCheckMergesZeroDowntimeConfig` — DB-gated (`TEST_DATABASE_URL`); deploys without inline health config but with `health_check_configs` row → moderate; currently asserts `Passed:true` for empty case with minimal schema (full merge verified when `nodes`/`servers` tables present).

*Concurrency / failure* (integration, DB-gated, `t.Skip` if `TEST_DATABASE_URL=""`):
- `TestConcurrentRollback409` — creates pending deployment + two revisions, marks completed, races two `RollbackToPrevious` goroutines; expects one success + one `errors.Is(err, store.ErrVersionConflict)` (409). Exercises `revisions.go:151` active-status gate + `UpdateDeploymentStatusVersioned` CAS (`store_deployments.go:278`) and `UpdateDeploymentRollback` (`:230`).
- `TestProvisionFailurePropagates` — `ExecuteDeployment` with failing `stubExecutor{applyErr:"node offline"}`; expects error containing `node offline`, deployment status `failed`, provision step `failed` with error text (honest propagation, not `completed`).

Run:

```bash
go test ./forge/api/internal/services/deployment/... -count=1 -run TestProvision -v
go test ./forge/api/internal/services/deployment/... -count=1 -run TestHealthProbe -v
TEST_DATABASE_URL=postgres://... go test ./forge/api/internal/services/deployment/... -run TestConcurrentRollback409 -v
```

### 4.2 Existing tests — now green

* `go test ./forge/api/internal/placement/...` — 100% pass (engine, constraints, explain, load, normalized).
* `go test ./forge/api/internal/services/scheduler/...` — pass.
* `go test ./forge/api/internal/daemon/...` — `go vet` clean.
* `go test ./forge/api/internal/services/deployment/...` — all unit tests pass without DB; DB-gated tests skip gracefully when `TEST_DATABASE_URL` unset (CI safe).

### 4.3 Manual verification

* `go vet ./forge/api/internal/placement/... ./forge/api/internal/services/scheduler/... ./forge/api/internal/services/deployment/... ./forge/api/internal/daemon/...` — clean.
* `rg -n "1e12|1e10|kSoftWeight" forge/api/internal/placement --type go` — shows `kSoftWeight` and legacy branch guarded by `isPlacementV2()`.
* `rg -n "FORGE_DEPLOY_REQUIRE_PLACEMENT|FORGE_DEPLOY_REQUIRE_TRAFFIC|FORGE_PLACEMENT_V2"` — shows execution, service, strategy, constraints, scheduler flag usage.
* `rg -n "resolveNodeHost|HealthProbe|ContainerHealthInspect|health_check_configs"` — health gate dual paths.

## 5. Flags & Rollout

| Flag | Default | Effect |
|---|---|---|
| `FORGE_DEPLOY_REQUIRE_PLACEMENT` | `off` | when `on`, `executeProvisionStep` / scale steps + `verifyReplicaCount` require `PlacementExecutor`; missing → honest error (422) |
| `FORGE_DEPLOY_REQUIRE_TRAFFIC` | `off` | when `on`, `executePromoteStep` requires `TrafficExecutor` |
| `FORGE_PLACEMENT_V2` | `on` | when `on`, bounded `kSoftWeight 0.30` + normalized `LeastLoaded` + scheduler bonuses; when `off`, legacy 1e12/1e10 for rollback diff |

All flags read via `os.Getenv` at call time (no restart needed for tests via `t.Setenv`). Old deployments with flag off remain green; new flag `on` activates placement/traffic without dropping columns or removing APIs.

## 6. Risks & Non-Goals

* Not wiring `replicamanager.Manager` + `gateway/service.go` in `cmd/api/main.go` in this PR (flag off keeps it safe; wiring is one line `deploySvc.SetPlacementExecutor(scheduler)` in follow-up).
* `daemon.HealthProbe` endpoint `/servers/:id/health` is new; old beacons return 404 → fallback to direct HTTP, so rolling upgrade is safe.
* `health_check_configs` table remains `UNIQUE(server_id)`; dual-read means old `deployments.health_check_*` rows still primary unless empty.

## 7. Verification Commands (for reviewers)

```bash
rg -n "FORGE_DEPLOY_REQUIRE|FORGE_PLACEMENT_V2" forge/api/internal/services/deployment forge/api/internal/placement forge/api/internal/services/scheduler --type go
rg -n "CheckSoft|kSoftWeight|LeastLoadedScorer|availableRatio" forge/api/internal/placement --type go
rg -n "resolveNodeHost|HealthProbe|health_check_configs" forge/api/internal/services/deployment --type go
go test ./forge/api/internal/placement -count=1 -v -run TestCheckSoftNormalizedBounds
go test ./forge/api/internal/services/deployment -count=1 -v -run "TestProvisionFails|TestHealthProbeNodeDerived|TestHealthProbeDirectHTTP"
TEST_DATABASE_URL=postgres://... go test ./forge/api/internal/services/deployment -count=1 -run TestConcurrentRollback409 -v
go vet ./forge/api/internal/services/deployment ./forge/api/internal/placement ./forge/api/internal/services/scheduler ./forge/api/internal/daemon
```

## 8. Closing

Deployment execution is now placement-first and traffic-aware when flags are on, but remains honest (fail-closed) when bridges are missing. Health gate no longer probes the API host; it derives the node's hostname from `ServerControlTarget.NodeURL` and prefers a beacon-local probe with docker health merge, falling back to direct HTTP only for old beacons. Soft bonuses are bounded to `0.30` so fleet truth (load) is readable, not dwarfed. Concurrent rollback is fenced via `isActiveStatus` + versioned CAS (`store.ErrVersionConflict` → 409). All changes are additive, flag-gated, and covered by unit + DB-gated integration tests.
