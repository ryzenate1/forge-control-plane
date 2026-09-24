# Subagent 03 — App Platform Implementation Plan

## Deployments, Scaling, Health Gates, Revisions, Env/Mounts, Delete, Frontend Parity

**Owner:** App Platform (Deployments / Scaling / Health Gates / Revisions / Env-Mounts)
**Scope:** `forge/api/internal/services/{deployment,apphosting,compose,replicamanager,zerodowntime}` + `forge/api/internal/http/handlers_{apphosting,deployment,compose}` + `forge/api/internal/store/{store_apphosting,store_deployments,store_deployment_revisions,store_compose,store_zero_downtime}` + `forge/api/internal/daemon/{client,compose}` + `forge/web/{app/admin/apps,lib/api,components}`
**Reference:** `audits/FINAL_PARITY_AUDIT.md §3 AP-01..AP-18`, `audits/final-parity/subagent-02-app-platform.md`, `audits/MASTER_FINDING_INDEX.md REF-APP-C02..C08`, `audits/phase-01/synthesis.md:70-108`
**Status:** DESIGN — no product code modified
**Date:** 2026-08-24 (post-final-parity verification)

---

## 0. Executive Summary

Forge's app-platform **models are sound, execution is dishonest**. Four deployment strategies are modeled, DAG steps exist, health gates threshold, revisions hash correctly — but the step executors lie:

* `forge/api/internal/services/deployment/execution.go:264` `executeProvisionStep` was a stub until recently; now it has a `RuntimeExecutor` interface (`execution.go:254`) with honest fail-closed behavior (`execution.go:268-269`), but `executePromoteStep:277`, `executeDrainOldStep:299`, `executeScaleUpStep:313` still only call `verifyObservedRunning` boolean. No container is provisioned via `replicamanager`, no traffic target is flipped, no placement is invoked.
* `forge/api/internal/http/handlers_apphosting.go:450` / `475` `POST /apps/:id/start|stop` still set `desired_state` only (`handlers_apphosting.go:468` `UpdateApp{DesiredState:"running"}`) with no reconciler — stale `observed_status` forever `idle`. Restart is **now fixed** (`handlers_apphosting.go:532` `SendPower(...,"restart")`) but start/stop remain the lie.
* `forge/api/internal/services/deployment/revisions.go:147` `RollbackToRevision` does version-gated `UpdateDeploymentStatusVersioned:167` then **ungated** `UpdateDeployment:174` — second rook can clobber first.
* `forge/api/internal/services/apphosting/service.go:175` `DeleteApp` is unconstrained `DELETE` (`store_apphosting.go:374`) with no `isActiveStatus` gate, no `placement_reservations` cancel, no beacon sweep.
* `forge/api/internal/services/apphosting/service.go:398` `ScaleService` admission for `ReplicaAppID==nil` is **now fixed** (`service.go:432-433` `scale refused`), but DB is mutated **before** `ReplicaManager.ScaleApp` — split-brain on failure.
* `forge/api/internal/services/deployment/healthgate.go:12` default loopback probe **fixed** (`healthgate.go:23` `resolveNodeHost` from `ServerControlTarget`), but `forge/api/internal/services/deployment/rollout.go:71` `validateHealthGateTarget:81-88` still clamps any explicit `healthCheckHost` to loopback — operator cannot probe a gateway VIP.
* `forge/api/internal/services/compose/service.go:134` `rawService.EnvFile` / `rawInclude.EnvFile` parsed but `parser.go:197` `loader.LoadWithContext Environment:map{}` drops it; `compose/service.go:356` `interpolateEnv` receives only caller `envVars`. Secrets in `env_file:` silently vanish.
* `forge/api/internal/services/compose/service.go:594` vs `beacon/internal/server/compose.go:226` volume predicates diverge: API allows `/data:/data`, beacon hard-rejects any absolute `source` → `200 valid → 400 violation` after reservation.
* Frontend: `forge/web/lib/api/compose.ts:89` never exports `restartComposeStack` though `handlers_compose.go:470` wired it; `forge/web/components/app/deployment-progress.tsx:41` (2s poll) vs `forge/web/components/charts/DeploymentTimeline.tsx:27` (5s poll) duplicate same `GET /admin/deployments/:id/steps`.

**This plan makes no forward claims without file:line.** Every recommendation cites a current line and a target diff. Backward compatibility is explicit per change.

---

## 1. Current State — File:Line Verified

### 1.1 Deployment execution (AP-02)

| File | Line | Current | Gap |
|------|------|---------|-----|
| `deployment/service.go:42` | `Strategy{blue-green,canary,rolling,recreate}` | Model complete | — |
| `deployment/steps.go:48` | `stepsForStrategy` DAG | Model complete | — |
| `deployment/execution.go:22` | `ExecuteDeployment` lease-claimed loop | Correct | — |
| `deployment/execution.go:249` | `RuntimeExecutor` interface docs | **FIXED** — was stub, now honest | AP-02 P0 closed for provision |
| `deployment/execution.go:264` | `executeProvisionStep` calls `rt.ApplyDeployment` | **PARTIALLY FIXED** — now calls `beacon_executor.go:33` `SyncServerConfiguration`+`SendPower start` | Still single-server only (see §2.1) |
| `deployment/execution.go:277` | `executePromoteStep` flips `ActiveTarget` in DB | **STILL BROKEN** — no trafficmanager/LB call, no verify | AP-02 remainder |
| `deployment/execution.go:299` | `executeDrainOldStep` → `verifyObservedRunning` | **STUB** | AP-02 |
| `deployment/execution.go:313` | `executeScaleUpStep` → `verifyObservedRunning` | **STUB** boolean, not replica-count | AP-02 + LF-01 |
| `deployment/beacon_executor.go:33` | `BeaconRuntimeExecutor.ApplyDeployment` | Real: `SyncServerConfiguration` + `SendPower start` | Good, but `VerifyRunning` is `Stats` existence only |
| `deployment/beacon_executor.go:60` | `VerifyRunning` via `Stats` | Weak: cannot distinguish running vs exited, no replica count | Needs hardening §2.3 |

**Verdict:** Provision is now real for the single `serverID` model. Everything else still lies complete.

### 1.2 App start/stop/restart (AP-03)

| File | Line | Current |
|------|------|---------|
| `handlers_apphosting.go:450` | `POST /apps/:id/start` → `UpdateApp{DesiredState:"running"}` `468` + `{"ok":true}` `472` | **BROKEN** — no actuation, no reconciler |
| `handlers_apphosting.go:475` | `POST /apps/:id/stop` same | **BROKEN** |
| `handlers_apphosting.go:500` | `POST /apps/:id/restart` → `SendPower restart` `532-534` | **FIXED** — was `TriggerDeploy` empty image |
| `store_apphosting.go:313` | `UpdateApplication` dynamic SET | Correct primitive, but no `observed_status` transition |
| `replicamanager/service.go:732` | `reconcile()` for `ReplicaApplications` | Exists for replica apps, **not** for `applications` |

Instance-level `POST .../instances/:id/{start,stop,restart}` at `handlers_apphosting.go:444` is correct (dispatches `AdminContainer{Start,Stop,Restart}` via daemon, updates `UpdateInstanceStatus`).

### 1.3 Revisions / Rollback (AP-05 / AP-09)

| File | Line | Current |
|------|------|---------|
| `deployment/revisions.go:58` | `configHash` SHA12 `image|compose|commit|metadata` | Correct |
| `deployment/revisions.go:82` | `CreateRevision` reads `GetLatest` then `nextNum+1` with no lock | **RACE** LF-05 |
| `deployment/revisions.go:147` | `RollbackToRevision` → `UpdateDeploymentStatusVersioned:167` then **ungated** `UpdateDeployment:174` | **RACE** |
| `store_deployment_revisions.go:21` | `UNIQUE(deployment_id, revision_number)` | Correct, will surface duplicate as 23505 |
| `store_deployments.go:128` | `UpdateDeployment` is `version = version+1 WHERE id=$1` — no version predicate | **UNVERSIONED** |
| `store_deployments.go:195` | `UpdateDeploymentConfig ... WHERE id=$1 AND version=$2` | Versioned — correct pattern to reuse |

### 1.4 Delete (AP-06)

| File | Line | Current |
|------|------|---------|
| `apphosting/service.go:175` | `DeleteApp` → `store.DeleteApplication` only | **BROKEN** — no `isActiveStatus` guard, no reservation cancel, no `replicamanager.DeleteApp` |
| `store_apphosting.go:374` | `DELETE FROM applications WHERE id=$1` | Unconstrained; cascades `app_services` via FK but **not** `instances` where `app_id=replica_app_id` |
| `compose/lifecycle.go:570` | `DeleteComposeStack` has `deleting→deleted` + reservation cancel | Correct pattern, not reused |
| `replicamanager/service.go:501` | `DeleteApp` iterates instances, cancels reservations, deletes | Correct, never called from `apphosting.DeleteApp` |

### 1.5 Scale (AP-07)

| File | Line | Current |
|------|------|---------|
| `apphosting/service.go:432` | `if target>0 && ReplicaAppID==nil { return scale refused }` | **FIXED ADMISSION** |
| `apphosting/service.go:436-449` | `UpdateAppService` then `UpdateReplicaAppReplicas` | **SPLIT-BRAIN** — DB mutated before `ReplicaManager.ScaleApp`; handler `handlers_apphosting.go:433` does call `ScaleApp` after but without tx |
| `handlers_apphosting.go:416` | `PATCH .../services/:id` calls `ScaleApp` after DB write | Same ordering bug |

### 1.6 Health gate (AP-08)

| File | Line | Current |
|------|------|---------|
| `deployment/healthgate.go:12` | `CheckHealth` skips if `Path=="" || Port==0` | Silent disable — caller bug masked |
| `deployment/healthgate.go:23` | `resolveNodeHost` via `ServerControlTarget → NodeURL hostname` | **FIXED** default path |
| `deployment/rollout.go:71` | `validateHealthGateTarget` allows `""\|"localhost"` else requires loopback IP | **CLAMP** — explicit `10.0.1.20` or `app.internal` rejected with `health checks may only target local` |
| `deployment/healthgate.go:37` | `http.Client{Timeout:10s, CheckRedirect: UseLastResponse}` | Direct from API pod → node, not via beacon |
| `store_zero_downtime.go:15` | `health_check_configs` per-server with `healthy_threshold` etc | **Disjoint** from `deployments.health_gate_*` — two health models |

### 1.7 Env / mounts / dependencies / catalog (AP-10, AP-11, AP-14)

| File | Line | Current |
|------|------|---------|
| `compose/service.go:134` | `EnvFile` field parsed | Defined, never consumed |
| `compose/parser.go:197` | `Environment:map[string]string{}` empty | Drops env_file |
| `beacon/internal/server/compose.go:380` | `.env` written from `EnvVars` map only | Cannot resolve `env_file:` |
| `compose/service.go:594` | `checkVolumesSecurity` warns on `/data`, errors on `/etc` | Allows `/data:/data` |
| `beacon/internal/server/compose.go:226` | `validateComposeVolumes` rejects any `source` starting `/` | Rejects `/data:/data` → bifurcation |
| `compose/service.go:764` | `normalizeDependsOn` correct | Parsed, diffed in `gitops.go:841`, but `lifecycle.go:249` delegates ordering to `docker compose up -d` — Forge never orders |
| `appstore/service.go:239` | `resolveTemplate` swallows interpolation error, deploys raw | **BROKEN** — stale compose |
| `catalog/catalog.go:119` | `Provision` | Isolated, correct |

### 1.8 Frontend (AP-03 remainder + UX)

| File | Line | Current |
|------|------|---------|
| `web/lib/api/compose.ts:89` | `deployComposeStack`, `stop`, `start`, `getStatus` exported | `restartComposeStack` **missing** though `handlers_compose.go:470` wired |
| `web/components/app/deployment-progress.tsx:41` | `POLL_INTERVAL 2000` against `GET /admin/deployments/:id/steps` | Correct component |
| `web/components/charts/DeploymentTimeline.tsx:27` | `refetchInterval 5000` same endpoint, same `queryKey ["deployment-steps", id]` | **DUPLICATE POLLER** — two components, same key, different intervals never dedup |
| `web/components/admin/AdminAppsShared.tsx:9` | `DeployStatusBadge` centralizes `statusTone` | Exists, but `web/app/admin/compose/page.tsx` uses own `statusConfig` (9 states) — tone drift |
| `web/app/admin/apps/page.tsx:16` | `typeIcons: Record<AppType, typeof Container>` bug | `Container` is lucide icon, shadows DOM `Container` — works but fragile |
| `web/app/admin/apps/[id]/page.tsx:42` | `useState(tab)` not synced to router on pop | **FIXED** — now `window.history.replaceState` on change, but initial `searchParams.get("tab")` only on mount, back button doesn't update `tab` state |

---

## 2. Detailed Design — Per Finding

### 2.1 AP-02 — Make Deployment Execution Real

#### 2.1.1 Problem restated

`ExecuteDeployment` claims lease, creates steps, then `executeStep` fans to:

* `executeProvisionStep:264` — **now real** via `RuntimeExecutor.ApplyDeployment`
* `executePromoteStep:277` — flips `active_target` in `deployments` row only
* `executeDrainOldStep:299`, `executeScaleUpStep:313`, `executeScaleDownStep:320` — boolean `verifyObservedRunning`

No placement is invoked (except provision's single-server path), no traffic weight is flipped, no replica count is verified. The DAG reports `completed` even though only 1 of 4 step kinds does real work.

#### 2.1.2 Design decision — two execution planes

Forge has **two disjoint workload planes**:

* **Single-server plane:** `deployments.server_id` → one `servers` row → one node via `ServerControlTarget` → `BeaconRuntimeExecutor` (current).
* **Replicated plane:** `applications` → `app_services` → `replica_app_id` → `instances` via `replicamanager.Manager` → scheduler + reservations + beacon dispatch.

`deployment.TargetReplicas` (`rollout.go:111`) is the seam. If `TargetReplicas > 1`, deployment should drive the **replicated plane**. Today it never does.

**Choice:** Keep both planes, but route step executors by `deployment.TargetReplicas` and whether `ServerID` maps to a replicated app. Do not delete the single-server path — game servers are single-server.

#### 2.1.3 Proposed interfaces

```go
// forge/api/internal/services/deployment/execution.go

// PlacementExecutor is the replicated-plane bridge. It is distinct from
// RuntimeExecutor (single-server container sync) so tests can fake one without
// the other and so the single-server path requires no placement DB.
type PlacementExecutor interface {
    // EnsureReplicas places and starts target replicas for (appID via serverID mapping).
    // Implementations must be idempotent for (deploymentID, attempt).
    EnsureReplicas(ctx context.Context, serverID string, targetReplicas int) error
    // DrainReplicas stops and removes replicas down to target.
    DrainReplicas(ctx context.Context, serverID string, targetReplicas int) error
    // VerifyReplicaCount checks running instances vs target.
    VerifyReplicaCount(ctx context.Context, serverID string, want int) (running int, err error)
}

// TrafficExecutor flips the gateway/ingress to the new target.
// It is intentionally tiny — trafficmanager owns the Caddy/Traefik write.
type TrafficExecutor interface {
    // Promote flips active target from blue→green (or reverse) for serverID.
    // Must verify the new target serves (health) before returning.
    Promote(ctx context.Context, serverID, fromTarget, toTarget string) error
}
```

```go
// forge/api/internal/services/deployment/service.go
type Service struct {
    store            *store.Store
    publisher        events.Publisher
    runtime          RuntimeExecutor      // single-server
    placement        PlacementExecutor    // replicated
    traffic          TrafficExecutor      // gateway
    resumeMu         sync.Mutex
    // ...
}
func (s *Service) SetPlacementExecutor(pe PlacementExecutor) { s.placement = pe }
func (s *Service) SetTrafficExecutor(te TrafficExecutor)    { s.traffic = te }
```

#### 2.1.4 Step-by-step wiring

**`executeProvisionStep`** — keep current `RuntimeExecutor` path for `TargetReplicas==1`, add replicated branch:

```go
func (s *Service) executeProvisionStep(ctx context.Context, deployment *Deployment) error {
    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_provisioning", deployment))
    }
    // Single-server path (game servers, single-replica apps)
    if deployment.TargetReplicas <= 1 {
        if s.runtime == nil {
            return fmt.Errorf("provision step: no runtime executor wired; refusing to report success without executing")
        }
        if err := s.runtime.ApplyDeployment(ctx, deployment.ServerID, deployment.Image); err != nil {
            return fmt.Errorf("provision step: apply to node failed: %w", err)
        }
        return nil
    }
    // Replicated path (app platform)
    if s.placement == nil {
        return fmt.Errorf("provision step: no placement executor wired for replicated deploy (want %d replicas)", deployment.TargetReplicas)
    }
    if err := s.placement.EnsureReplicas(ctx, deployment.ServerID, deployment.TargetReplicas); err != nil {
        return fmt.Errorf("provision step: place replicas failed: %w", err)
    }
    // Verify at least quorum before advancing (configurable; default 100%)
    if running, err := s.placement.VerifyReplicaCount(ctx, deployment.ServerID, deployment.TargetReplicas); err != nil {
        return fmt.Errorf("provision step: verify replicas failed: %w", err)
    } else if running < deployment.TargetReplicas {
        return fmt.Errorf("provision step: only %d/%d replicas running after placement", running, deployment.TargetReplicas)
    }
    return nil
}
```

**Production `PlacementExecutor` adapter** (wires `replicamanager`):

```go
// forge/api/internal/services/deployment/placement_executor.go
package deployment

import (
    "context"
    "fmt"
    "gamepanel/forge/internal/store"
)

type ReplicaPlacementExecutor struct {
    Store           *store.Store
    ReplicaManager  interface {
        ScaleApp(ctx context.Context, appID string, targetReplicas int) error
    }
    // Resolve which ReplicaApplication backs a serverID.
    // For app-platform, ServerID is the logical app's server binding; we need
    // to map deployments.server_id → applications.server_id → app_services.replica_app_id
    // or directly via a new column deployments.replica_app_id (migration §3.1).
}

func (p *ReplicaPlacementExecutor) EnsureReplicas(ctx context.Context, serverID string, want int) error {
    // Option A (no new column): lookup Application where server_id = $1
    // then its AppServices, then ReplicaAppID.
    // Option B (new column, preferred): deployments.replica_app_id FK.
    appID, err := p.resolveReplicaAppID(ctx, serverID)
    if err != nil {
        return err
    }
    return p.ReplicaManager.ScaleApp(ctx, appID, want)
}

func (p *ReplicaPlacementExecutor) VerifyReplicaCount(ctx context.Context, serverID string, want int) (int, error) {
    appID, err := p.resolveReplicaAppID(ctx, serverID)
    if err != nil {
        return 0, err
    }
    insts, err := p.Store.ListInstancesByApp(ctx, appID)
    if err != nil {
        return 0, err
    }
    running := 0
    for _, inst := range insts {
        if inst.Status == "running" { // align with replicamanager reconcile:732
            running++
        }
    }
    return running, nil
}
```

**`executePromoteStep`** — wire traffic:

```go
func (s *Service) executePromoteStep(ctx context.Context, deployment *Deployment) error {
    newTarget := "green"
    if deployment.ActiveTarget != "blue" {
        newTarget = "blue"
    }
    oldTarget := deployment.ActiveTarget
    fresh, err := s.store.GetDeployment(ctx, deployment.ID)
    if err != nil {
        return fmt.Errorf("re-fetch deployment: %w", err)
    }
    // If traffic executor present, flip gateway first, then record.
    if s.traffic != nil {
        if err := s.traffic.Promote(ctx, deployment.ServerID, oldTarget, newTarget); err != nil {
            return fmt.Errorf("promote step: traffic flip failed: %w", err)
        }
    } else {
        // Fail-closed unless explicitly opted out (feature flag)
        // For backward compat during rollout, allow DB-only promote behind flag.
        // See §5 feature flag.
    }
    if err := s.store.UpdateDeploymentConfig(ctx, deployment.ID, fresh.Version, "", "", "", newTarget); err != nil {
        return fmt.Errorf("update deployment config: %w", err)
    }
    deployment.ActiveTarget = newTarget
    deployment.UpdatedAt = time.Now().UTC()
    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_promoted", deployment))
    }
    return nil
}
```

```go
// forge/api/internal/services/deployment/traffic_executor.go
type GatewayTrafficExecutor struct {
    Store            *store.Store
    TrafficManager   interface {
        PromoteTarget(ctx context.Context, serverID, fromTarget, toTarget string) error
    }
}
func (g *GatewayTrafficExecutor) Promote(ctx context.Context, serverID, from, to string) error {
    return g.TrafficManager.PromoteTarget(ctx, serverID, from, to)
}
```

For the first milestone, `TrafficExecutor` may be a no-op that logs and returns error unless `FORGE_DEPLOY_TRAFFIC_PROMOTE=off` — see rollout §5.

**`executeDrainOldStep` / `ScaleUp/Down`** — wire placement + verify count:

```go
func (s *Service) executeDrainOldStep(ctx context.Context, deployment *Deployment) error {
    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_draining", deployment))
    }
    if deployment.TargetReplicas <= 1 {
        return s.verifyObservedRunning(ctx, deployment, "drain_old")
    }
    if s.placement == nil {
        return fmt.Errorf("drain_old step: no placement executor wired")
    }
    // For blue-green drain, the old color's replicas are the drain target.
    // For now, verify the active target is at desired count and old is 0.
    // Full implementation would track GreenTargetID vs BlueTargetID in
    // replica placement; initial cut just verifies active target.
    if running, err := s.placement.VerifyReplicaCount(ctx, deployment.ServerID, deployment.TargetReplicas); err != nil {
        return fmt.Errorf("drain_old step: verify failed: %w", err)
    } else if running < deployment.TargetReplicas {
        return fmt.Errorf("drain_old step: %d/%d replicas running after promote", running, deployment.TargetReplicas)
    }
    return nil
}

func (s *Service) executeScaleUpStep(ctx context.Context, deployment *Deployment) error {
    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, newDeploymentEvent("rolling_scale_up", deployment))
    }
    if s.placement != nil && deployment.TargetReplicas > 1 {
        if err := s.placement.EnsureReplicas(ctx, deployment.ServerID, deployment.TargetReplicas); err != nil {
            return fmt.Errorf("scale_up step: %w", err)
        }
        if running, err := s.placement.VerifyReplicaCount(ctx, deployment.ServerID, deployment.TargetReplicas); err != nil {
            return err
        } else if running < deployment.TargetReplicas {
            return fmt.Errorf("scale_up step: %d/%d running", running, deployment.TargetReplicas)
        }
        return nil
    }
    return s.verifyObservedRunning(ctx, deployment, "scale_up")
}
```

**`verifyObservedRunning` hardening** — move from `Stats` existence to `health_check_configs` or container `State`:

```go
// beacon_executor.go:60 current:
func (b *BeaconRuntimeExecutor) VerifyRunning(...) (bool, error) {
    // Today: Stats existence
    if _, err := b.Daemon.Stats(...); err != nil { return false, err }
    return true, nil
}
// Target: try AdminContainerInspect first (State==running), fallback to Stats per §2.3
```

#### 2.1.5 Wiring in `main.go`

```go
// forge/api/cmd/api/main.go (or wherever WireBeaconExecutor is called)
deploymentSvc := deployment.New(store, publisher)
deployment.WireBeaconExecutor(deploymentSvc, store, daemonClient)
deployment.WirePlacementExecutor(deploymentSvc, store, replicaManager) // new
deployment.WireTrafficExecutor(deploymentSvc, store, trafficManager)   // new, may be nil initially
```

Feature flag: `FORGE_DEPLOY_REQUIRE_PLACEMENT` (default true for `TargetReplicas>1`), `FORGE_DEPLOY_REQUIRE_TRAFFIC` (default false initially, true after §5.2).

Backward compatibility: when executors are nil, steps fail closed with `no runtime executor wired` — deployment goes `failed`, never `completed` falsely. This is the correct remedy for AP-02 P0.

---

### 2.2 AP-03 — App start/stop Reconciler (desired→actual)

#### 2.2.1 Options considered

| Option | Behavior | Tradeoff |
|--------|----------|----------|
| **A. Reconciler loop** (like `replicamanager/reconcile:732`) | API sets `desired_state`, background loop drives `observed_status` via beacon | Correct desired/actual semantics, eventually consistent; adds daemon |
| **B. Synchronous actuation** | `POST /start` directly calls `SendPower` or `AdminContainerStart`, returns 502 on beacon failure | Immediate feedback, no loop, but `desired_state` becomes redundant |
| **C. Admit no-reconciler** | Document `desired_state` as advisory, make API synchronous with 502 | Honest but defeats the `desired/actual+transitions sup` advantage (`FINAL_PARITY_AUDIT §2 GH-02`) |

**Recommendation: A + B hybrid** — add a thin reconciler for drift/retries, but make the HTTP handler **optimistically actuates synchronously** and surface beacon errors immediately. Reconciler catches crashes and retries.

This matches how 1Panel `Operate(start|stop)` runs `compose.Up/Stop` synchronously, and how Forge's `replicamanager` reconciles replica apps.

#### 2.2.2 API changes — `handlers_apphosting.go:450` / `475`

Current (`450-472`):

```go
running := "running"
_, err = appSvc.UpdateApp(ctx, c.Params("id"), app.OrgID, apphosting.UpdateAppRequest{DesiredState: &running})
return c.JSON(fiber.Map{"ok": true})
```

Target:

```go
protected.Post("/apps/:id/start", mutationLimiter, func(c *fiber.Ctx) error {
    // ... auth + load app ...
    running := "running"
    if _, err := appSvc.UpdateApp(ctx, c.Params("id"), app.OrgID, apphosting.UpdateAppRequest{DesiredState: &running}); err != nil {
        return respondStoreError(err)
    }
    // Optimistic actuation: try beacon now; reconciler will retry on failure.
    if app.ServerID != nil && *app.ServerID != "" && cfg.Daemon != nil && cfg.Store != nil {
        target, err := cfg.Store.ServerControlTarget(ctx, *app.ServerID)
        if err == nil {
            powerCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
            defer cancel()
            if _, err := cfg.Daemon.SendPower(powerCtx, target.NodeURL, target.NodeToken, target.ServerID, "start"); err != nil {
                // Mark observed_status failed so UI reflects reality; reconciler retries.
                _ = cfg.Store.UpdateApplicationStatus(ctx, c.Params("id"), "error")
                return fiber.NewError(fiber.StatusBadGateway, "desired_state set to running but node start failed: "+err.Error())
            }
            _ = cfg.Store.UpdateApplicationStatus(ctx, c.Params("id"), "running")
            return c.JSON(fiber.Map{"ok": true, "observed_status": "running"})
        }
    }
    // No server assigned or node offline — desired set, observed will converge via reconciler.
    return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
        "ok": true,
        "observed_status": "deploying",
        "message": "desired_state set to running; reconciler will actuate when node is available",
    })
})
```

Same for `POST /apps/:id/stop` with `signal:"stop"` and `observed_status:"stopped"`.

For **compose-based apps** (no `ServerID` but a `compose_stack`), reuse compose lifecycle:

```go
if app.SourceType == "COMPOSE" {
    // Resolve compose_stack for this app (lookup by app_id or server_id)
    stack, err := cfg.Store.FindComposeStackByAppID(ctx, c.Params("id"))
    if err == nil && stack != nil {
        node, _ := cfg.Store.GetNode(ctx, stack.NodeID)
        token, _ := cfg.Store.GetNodeDaemonCredential(ctx, node.ID)
        _, err = cfg.Daemon.ComposeStart(ctx, node.BaseURL, token, stack.ID)
        // set observed_status accordingly
    }
}
```

This covers `handlers_compose.go:442/452` parity.

#### 2.2.3 Reconciler service

New package `forge/api/internal/services/appreconciler/service.go`:

```go
package appreconciler

type Service struct {
    store  *store.Store
    daemon *daemon.Client
    mu     sync.Mutex
    stopCh chan struct{}
    started atomic.Bool
}

func (s *Service) Start(ctx context.Context) {
    if !s.started.CompareAndSwap(false, true) { return }
    s.stopCh = make(chan struct{})
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done(): return
            case <-s.stopCh: return
            case <-ticker.C: s.reconcile(ctx)
            }
        }
    }()
}

func (s *Service) reconcile(ctx context.Context) {
    apps, _ := s.store.ListApplicationsNeedingReconcile(ctx) // where desired_state != observed_status
    for _, app := range apps {
        // Skip if deployment is active (avoid fighting rollout)
        if active, _ := s.store.HasActiveDeployment(ctx, app.ServerID); active {
            continue
        }
        switch app.DesiredState {
        case "running":
            if app.ObservedStatus != "running" && app.ObservedStatus != "deploying" {
                s.actuateStart(ctx, app)
            }
        case "stopped":
            if app.ObservedStatus != "stopped" {
                s.actuateStop(ctx, app)
            }
        case "removed":
            // Delegate to DeleteApp flow with gate §2.5
        }
    }
}
```

Store helper:

```sql
-- new query in Store
SELECT id, desired_state, observed_status, server_id, source_type
FROM applications
WHERE desired_state != observed_status
  AND desired_state IN ('running','stopped')
  AND updated_at < now() - interval '5 seconds'
LIMIT 100;
```

Migration to add partial index for reconciler:

```sql
-- migrations/211_app_reconciler_index.sql
CREATE INDEX IF NOT EXISTS idx_applications_reconcile
ON applications(desired_state, observed_status)
WHERE desired_state != observed_status;
```

**Feature flag alternative:** If the team prefers no background loop, expose the gap explicitly: add `X-Forge-Reconciler: off` header and make start/stop return `502` when beacon is unreachable, documenting that `desired_state` requires `appreconciler` enabled. The flag check lives in handler — no new daemon.

#### 2.2.4 Compose admin restart

`handlers_compose.go:470` already wires `POST /compose/:id/restart` → `lifecycle.go:781` `RestartStack` (`Stop+Start`). The bug is that `web/lib/api/compose.ts:89` does not export it and lifecycle is not atomic.

Fix lifecycle to be reservation-aware + health-verified:

```go
// compose/lifecycle.go:781
func (s *Service) RestartStack(ctx context.Context, stackID string) error {
    stack, err := s.store.GetComposeStack(ctx, stackID)
    if err != nil { return err }
    // Hold reservation during restart (extend lease)
    if stack.ReservationID != "" {
        // no-op if no reservation; else RenewPlacementReservation
    }
    if err := s.stopStack(ctx, stack); err != nil {
        return fmt.Errorf("restart stop failed: %w", err)
    }
    if err := s.startStack(ctx, stack); err != nil {
        // Attempt to restore previous YAML? Or mark failed with rollback hint.
        return fmt.Errorf("restart start failed: %w", err)
    }
    if err := s.WaitForHealthy(ctx, stack.ID, 2*time.Minute); err != nil {
        return fmt.Errorf("restart unhealthy: %w", err)
    }
    return nil
}
```

Export in `compose.ts`:

```ts
export function restartComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
}
```

---

### 2.3 AP-08 — Health Gates: Beacon-Executed, Not API-Dialed

#### 2.3.1 Current vs target

* **Current default:** `healthgate.go:23` `resolveNodeHost` correctly derives `NodeURL.hostname` — no longer `localhost`. Good.
* **Current clamp:** `rollout.go:71` `validateHealthGateTarget` rejects any explicit `HealthCheckHost` that is not `""`, `"localhost"`, or loopback — so an operator cannot set `healthCheckHost:"10.0.1.20"` (node internal IP) or `healthCheckHost:"app.internal"` (Tailscale/NetBird).
* **Current probe:** `healthgate.go:37` `http.Client{Timeout:10s}` dials from the **API pod** to `http://<nodeHost>:<port><path>`. This fails when API↔node is not routable (private VPC, firewalled node). No beacon path, no `docker inspect HealthStatus`, no `health_check_configs`.

#### 2.3.2 Target architecture

Three health sources, merged:

1. **API-direct HTTP** — fast path when node is routable from API (current).
2. **Beacon-local HTTP** — API asks beacon to `GET http://127.0.0.1:<port><path>` **on the target node**; beacon dials loopback and returns `passed/status`.
3. **Docker `HealthStatus`** — `docker inspect` via beacon, for images with `HEALTHCHECK`.

Merge with `health_check_configs` (`114_e_zero_downtime_deploy.sql:15`):

```sql
-- health_check_configs currently keyed by server_id for zero-downtime releases.
-- Merge deployment-health-gate into same table so config is durable.
ALTER TABLE health_check_configs
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'deployment' -- 'deployment'|'zerodowntime'|'manual'
  , ADD COLUMN IF NOT EXISTS deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_health_check_configs_deployment
  ON health_check_configs(deployment_id);
```

#### 2.3.3 `validateHealthGateTarget` fix

Remove loopback clamp for explicit host; validate as hostname/IP but do not restrict to loopback.

```go
// deployment/rollout.go:71
func validateHealthGateTarget(req *RolloutRequest) error {
    if req.HealthCheckPort < 0 || req.HealthCheckPort > 65535 {
        return errors.New("health check port is out of range")
    }
    if req.HealthCheckPath != "" {
        parsed, err := url.ParseRequestURI(req.HealthCheckPath)
        if err != nil || !strings.HasPrefix(req.HealthCheckPath, "/") || parsed.IsAbs() || strings.HasPrefix(req.HealthCheckPath, "//") {
            return errors.New("health check path must be a local absolute path")
        }
    }
    host := strings.TrimSpace(req.HealthCheckHost)
    if host == "" {
        return nil // default → resolveNodeHost
    }
    host = strings.TrimSuffix(strings.ToLower(host), ".")
    // Allow any valid hostname/IP; no loopback clamp.
    // Basic hygiene: reject URL injection.
    if strings.ContainsAny(host, " \t\n\r/\\?#@") {
        return errors.New("health check host contains illegal characters")
    }
    if len(host) > 253 {
        return errors.New("health check host too long")
    }
    // If it looks like an IP, validate it.
    if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
        // Allow any IP — private, loopback, public — operator chooses.
        // Do not clamp.
        return nil
    }
    // Hostname: RFC 1123-ish check
    if !hostnameRegex.MatchString(host) {
        return errors.New("health check host is not a valid hostname or IP")
    }
    return nil
}
var hostnameRegex = regexp.MustCompile(`^([a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)
```

Keep existing `resolveNodeHost` as fallback when `HealthCheckHost==""`.

#### 2.3.4 Health gate probe via beacon

Extend `daemon.Client` with a health probe RPC:

```go
// daemon/client.go — new method
type HealthProbeRequest struct {
    Host string `json:"host"` // "" means 127.0.0.1 on node
    Port int    `json:"port"`
    Path string `json:"path"`
}
type HealthProbeResponse struct {
    Passed       bool   `json:"passed"`
    Status       int    `json:"status"`
    Body         string `json:"body,omitempty"`
    Error        string `json:"error,omitempty"`
    Via          string `json:"via"` // "direct"|"beacon"
    ResponseMs   int    `json:"responseMs"`
}

func (c *Client) HealthProbe(ctx context.Context, baseURL, nodeToken, serverID string, req HealthProbeRequest) (HealthProbeResponse, error) {
    body, _ := json.Marshal(req)
    endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/health-probe"
    // Wire beacon side to GET http://127.0.0.1:<port><path> if req.Host=="" else http://<host>:<port><path>
    // Return 200 with {passed:false} on probe failure (do not return HTTP error).
}
```

Beacon side (`beacon/internal/server/server.go`):

```go
func (s *Server) handleHealthProbe(w http.ResponseWriter, r *http.Request) {
    var req daemon.HealthProbeRequest
    _ = json.NewDecoder(r.Body).Decode(&req)
    host := req.Host
    if host == "" { host = "127.0.0.1" }
    target := fmt.Sprintf("http://%s:%d%s", host, req.Port, req.Path)
    client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
    start := time.Now()
    resp, err := client.Get(target)
    // map to HealthProbeResponse{Passed: status 200-399}
}
```

Wire `CheckHealth` to try beacon first when node is routable via daemon:

```go
// deployment/healthgate.go:12
func (s *Service) CheckHealth(ctx context.Context, deployment *Deployment) (*HealthCheckResult, error) {
    if deployment.HealthCheckPath == "" || deployment.HealthCheckPort == 0 {
        return &HealthCheckResult{Passed: true}, nil
    }
    // Try beacon-local probe first (most reliable — on-node loopback).
    if hostProbe := s.tryBeaconHealthProbe(ctx, deployment); hostProbe != nil {
        return hostProbe, nil
    }
    // Fallback: direct dial from API (legacy path for offline beacon or no daemon wiring).
    host := deployment.HealthCheckHost
    if host == "" {
        host = s.resolveNodeHost(ctx, deployment.ServerID)
        if host == "" {
            return &HealthCheckResult{Passed: false, Error: "health check host unresolved: target node offline or unknown"}, nil
        }
    }
    target := fmt.Sprintf("http://%s:%d%s", host, deployment.HealthCheckPort, deployment.HealthCheckPath)
    // ... existing http.Client path
}
```

`tryBeaconHealthProbe` helper:

```go
func (s *Service) tryBeaconHealthProbe(ctx context.Context, d *Deployment) *HealthCheckResult {
    if s.beaconHealth == nil { return nil } // not wired yet
    targetCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
    defer cancel()
    target, err := s.store.ServerControlTarget(targetCtx, d.ServerID)
    if err != nil { return nil }
    // Defer to daemon client
    res, err := s.beaconHealth.HealthProbe(targetCtx, target.NodeURL, target.NodeToken, d.ServerID, daemon.HealthProbeRequest{
        Host: d.HealthCheckHost, // "" → beacon uses 127.0.0.1
        Port: d.HealthCheckPort,
        Path: d.HealthCheckPath,
    })
    if err != nil { return nil } // fallback to direct
    return &HealthCheckResult{
        Passed: res.Passed,
        Status: res.Status,
        Body:   res.Body,
        Error:  res.Error,
    }
}
```

Add field to `Service`:

```go
type Service struct {
    store       *store.Store
    publisher   events.Publisher
    runtime     RuntimeExecutor
    beaconHealth interface {
        HealthProbe(ctx context.Context, baseURL, nodeToken, serverID string, req daemon.HealthProbeRequest) (daemon.HealthProbeResponse, error)
    }
}
```

Wire in `main.go`:

```go
deploymentSvc.SetBeaconHealth(daemonClient) // daemonClient already satisfies HealthProbe
```

**Why beacon-local first?** The container's `EXPOSE` port is typically only bound to `127.0.0.1` or node private IP, not reachable from API. And `docker inspect HealthStatus` is only visible on the node. Beacon-local is the only honest vantage — `FINAL_PARITY_AUDIT §3 AP-08` says `localhost` probe failed before, and `resolveNodeHost` is better but still wrong vantage (API→node, not beacon→container).

For `HEALTHCHECK` images, also probe via `docker inspect`:

```go
// daemon/client.go — second probe path
func (c *Client) ContainerHealth(ctx context.Context, baseURL, nodeToken, containerID string) (string, error) {
    // GET /admin/containers/<id>/json → .State.Health.Status
}
```

`CheckHealth` can then short-circuit: if `container HealthStatus == "healthy"` → `Passed:true` without HTTP, if `"unhealthy"` → `Passed:false`.

Merge `health_check_configs` usage:

```go
// In applyRolloutRequest, persist to health_check_configs as well:
if req.HealthGateEnabled {
    _ = s.store.UpsertZeroDowntimeHealthCheckConfig(ctx, &store.ZeroDowntimeHealthCheckConfig{
        ID: uuid.NewString(), ServerID: req.ServerID,
        Path: req.HealthCheckPath, Port: req.HealthCheckPort,
        IntervalSeconds: req.HealthGateIntervalMs/1000,
        HealthyThreshold: req.HealthGateThreshold,
        Source: "deployment", DeploymentID: &d.ID,
    })
}
```

---

### 2.4 AP-05 / AP-09 — Revisions: Version-Gated Transaction

#### 2.4.1 Race analysis

`revisions.go:82` `CreateRevision`:

```go
latest, _ := s.store.GetLatestDeploymentRevision(ctx, deploymentID)
nextNum := latest.RevisionNumber + 1 // race: two callers read 5, both write 6
// then CreateDeploymentRevision without lock → 23505 unique violation OR silent duplicate if violated expectation not checked
```

`revisions.go:147` `RollbackToRevision`:

```go
s.store.UpdateDeploymentStatusVersioned(ctx, id, sd.Version, "in_progress", "") // bumps version 7→8
deployment.Image = targetRev.ImageRef
s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)) // ungated UPDATE ... version=version+1 WHERE id=$1 → 8→9 regardless of concurrent 8→9 by other rollback
```

If two admins rollback to different revisions concurrently, last writer wins silently.

#### 2.4.2 Migration — already partially protected

`099_deployment_revisions.sql:21` `UNIQUE(deployment_id, revision_number)` will make the `CreateRevision` race surface as `23505` rather than duplicate numbers — but the service swallows it as generic error, no retry.

Add explicit tx helper and version-gated `UpdateDeployment`:

```sql
-- No new DDL required: idx_deployment_revisions_rev_number already exists.
-- Add check to prevent phantom revision_number gaps from breaking CompareRevisions:
-- already ORDER BY revision_number DESC handled.
```

#### 2.4.3 Service fix — `CreateRevision` with `FOR UPDATE`

```go
// store/store_deployment_revisions.go — new tx helper
func (s *Store) CreateDeploymentRevisionTx(ctx context.Context, deploymentID string, cfg *RevisionConfig, description string) (*DeploymentRevision, error) {
    tx, err := s.db.Begin(ctx)
    if err != nil { return nil, err }
    defer tx.Rollback(ctx)

    // Lock the deployment row to serialize revision numbering per deployment.
    var deploymentVersion int
    if err := tx.QueryRow(ctx, `SELECT version FROM deployments WHERE id = $1 FOR UPDATE`, deploymentID).Scan(&deploymentVersion); err != nil {
        return nil, fmt.Errorf("lock deployment for revision: %w", err)
    }
    var nextNum int
    if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number),0)+1 FROM deployment_revisions WHERE deployment_id = $1`, deploymentID).Scan(&nextNum); err != nil {
        return nil, err
    }
    now := time.Now().UTC()
    hash := configHash(cfg)
    metaRaw, _ := json.Marshal(cfg.Metadata)
    if len(metaRaw) == 0 { metaRaw = []byte("{}") }
    rev := &DeploymentRevision{
        ID: uuid.NewString(), DeploymentID: deploymentID,
        RevisionNumber: nextNum, ImageRef: cfg.ImageRef,
        ComposeManifestRef: cfg.ComposeManifestRef, GitCommitSHA: cfg.GitCommitSHA,
        ConfigHash: hash, Status: string(RevisionStatusPending),
        Description: description, Metadata: json.RawMessage(metaRaw),
        CreatedAt: now, UpdatedAt: now,
    }
    if _, err := tx.Exec(ctx, `
        INSERT INTO deployment_revisions (id, deployment_id, revision_number, image_ref, compose_manifest_ref,
            git_commit_sha, config_hash, status, deployed_at, description, metadata, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
    `, rev.ID, rev.DeploymentID, rev.RevisionNumber, rev.ImageRef, rev.ComposeManifestRef,
        rev.GitCommitSHA, rev.ConfigHash, rev.Status, rev.DeployedAt, rev.Description, rev.Metadata, rev.CreatedAt, rev.UpdatedAt); err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil { return nil, err }
    return rev, nil
}
```

Wire `deployment/revisions.go:82` to use it:

```go
func (s *Service) CreateRevision(ctx context.Context, deploymentID string, cfg *RevisionConfig) (*Revision, error) {
    if _, err := s.store.GetDeployment(ctx, deploymentID); err != nil {
        return nil, ErrNotFound
    }
    rev, err := s.store.CreateDeploymentRevisionTx(ctx, deploymentID, cfg, cfg.Description)
    if err != nil {
        // If 23505 still occurs (race across tx boundary in non-serializable mode), retry once.
        if isUniqueViolation(err) {
            // retry with fresh MAX
            rev, err = s.store.CreateDeploymentRevisionTx(ctx, deploymentID, cfg, cfg.Description)
        }
        if err != nil { return nil, fmt.Errorf("create revision: %w", err) }
    }
    return toRevision(*rev), nil
}
```

#### 2.4.4 `RollbackToRevision` — single CAS transaction

Option 1: wrap existing two writes in a tx with `FOR UPDATE`:

```go
func (s *Service) RollbackToRevision(ctx context.Context, deploymentID string, revisionID string) (*Deployment, error) {
    sd, err := s.store.GetDeployment(ctx, deploymentID)
    if err != nil { return nil, ErrNotFound }
    targetRev, err := s.store.GetDeploymentRevision(ctx, revisionID)
    if err != nil { return nil, ErrRevisionNotFound }
    if targetRev.DeploymentID != deploymentID {
        return nil, fmt.Errorf("revision does not belong to this deployment")
    }
    if err := validateImageRef(targetRev.ImageRef); err != nil {
        return nil, fmt.Errorf("rollback target revision: %w", err)
    }
    // Single transaction: lock deployment, bump status, set image+revision, supersede, activate.
    tx, err := s.store.DB().Begin(ctx)
    if err != nil { return nil, err }
    defer tx.Rollback(ctx)

    var curVersion int
    var curImage string
    var curStatus string
    if err := tx.QueryRow(ctx, `SELECT version, image, status FROM deployments WHERE id = $1 FOR UPDATE`, deploymentID).Scan(&curVersion, &curImage, &curStatus); err != nil {
        return nil, err
    }
    // Ensure we are not rolling back an already-in-progress deployment raced by another rollback.
    // Require that version still == sd.Version we read before tx.
    if curVersion != sd.Version {
        return nil, ErrVersionConflict // caller retries or surfaces 409
    }
    now := time.Now().UTC()
    // Claim ownership (in_progress) + set image + current_revision in one version-gated update.
    tag, err := tx.Exec(ctx, `
        UPDATE deployments
        SET status = 'in_progress',
            image = $3,
            current_revision_id = $4,
            version = version + 1,
            updated_at = now()
        WHERE id = $1 AND version = $2
    `, deploymentID, curVersion, targetRev.ImageRef, revisionID)
    if err != nil { return nil, err }
    if tag.RowsAffected() == 0 {
        return nil, ErrVersionConflict
    }
    // Supersede and activate within same tx
    if _, err := tx.Exec(ctx, `UPDATE deployment_revisions SET status='superseded', updated_at=now() WHERE deployment_id=$1 AND id!=$2 AND status='active'`, deploymentID, revisionID); err != nil {
        return nil, err
    }
    if _, err := tx.Exec(ctx, `UPDATE deployment_revisions SET status='active', deployed_at=$2, updated_at=now() WHERE id=$1`, revisionID, now); err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil { return nil, err }

    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_rolled_back", "deployment", "deployment", deploymentID, map[string]any{
            "serverId": sd.ServerID, "targetRev": targetRev.RevisionNumber, "targetImage": targetRev.ImageRef,
        }))
    }
    fresh, _ := s.store.GetDeployment(ctx, deploymentID)
    return toServiceDeployment(fresh), nil
}
```

Option 2 (if tx over `pgx.Tx` not desired): keep two `Update...Versioned` calls but make the second also version-checked. Add new store method `UpdateDeploymentImageVersioned`:

```go
func (s *Store) UpdateDeploymentImageVersioned(ctx context.Context, id string, expectedVersion int, image string, revisionID *string) error {
    tag, err := s.db.Exec(ctx, `
        UPDATE deployments
        SET image = $3, current_revision_id = $4, version = version + 1, updated_at = now()
        WHERE id = $1 AND version = $2
    `, id, expectedVersion, image, revisionID)
    if tag.RowsAffected()==0 { return ErrVersionConflict }
    return err
}
```

Then `RollbackToRevision` does:

```go
if err := s.store.UpdateDeploymentStatusVersioned(ctx, deploymentID, sd.Version, string(StatusInProgress), ""); err != nil { return err }
if err := s.store.UpdateDeploymentImageVersioned(ctx, deploymentID, sd.Version+1, targetRev.ImageRef, &revisionID); err != nil { return err }
```

But tx is strictly better (atomic status+image+revision). **Choose tx.**

Add helper `isUniqueViolation`:

```go
func isUniqueViolation(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

Expose `ErrVersionConflict` already exists at `store_deployments.go:193` — reuse.

#### 2.4.5 Frontend / API surface

`handlers_revisions.go:9` already routes `GET /:id/revisions` and `GET /:id/compare?from&to`. No change needed. Ensure `POST /:id/revisions/:revID/rollback` and `POST /:id/rollback-previous` return `409 Conflict` on `ErrVersionConflict` (map in `respondDeploymentError`):

```go
func respondDeploymentError(c *fiber.Ctx, err error) error {
    if errors.Is(err, deployment.ErrNotFound) { return fiber.NewError(404, err.Error()) }
    if errors.Is(err, deployment.ErrVersionConflict) { return fiber.NewError(409, "deployment modified concurrently; retry") }
    return respondInternalError(c, err)
}
```

---

### 2.5 AP-06 — Delete: Gate, Cancel, Sweep

#### 2.5.1 Current

```go
// apphosting/service.go:175
func (s *Service) DeleteApp(ctx, appID, orgID) error {
    belongs, _ := s.store.AppBelongsToOrg(ctx, appID, orgID)
    return s.store.DeleteApplication(ctx, appID) // no guard
}
// store_apphosting.go:374
DELETE FROM applications WHERE id = $1 // cascades app_services but not instances
```

While `compose/lifecycle.go:570` `DeleteComposeStack` correctly sets `deleting→deleted`, cancels reservation, calls `daemon.ComposeDelete` with `removeOrphans`, the app path skips all of it.

#### 2.5.2 Target

```go
func (s *Service) DeleteApp(ctx context.Context, appID, orgID string) error {
    belongs, err := s.store.AppBelongsToOrg(ctx, appID, orgID)
    if err != nil { return err }
    if !belongs { return errors.New("application not found") }

    app, err := s.store.GetApplication(ctx, appID)
    if err != nil { return err }

    // Gate 1: refuse while deployment active (AP-06 orphan guard)
    if app.ServerID != nil && *app.ServerID != "" {
        deployments, _ := s.store.ListDeployments(ctx, *app.ServerID)
        for _, d := range deployments {
            if isActiveStatus(d.Status) { // reuse deployment.isActiveStatus
                return fmt.Errorf("delete refused: deployment %s is %s; cancel it first", d.ID, d.Status)
            }
        }
        if app.CurrentDeploymentID != nil {
            if d, err := s.store.GetDeployment(ctx, *app.CurrentDeploymentID); err == nil && isActiveStatus(d.Status) {
                return fmt.Errorf("delete refused: current deployment is %s", d.Status)
            }
        }
    }
    // Gate 2: compose stack status gate (if app maps to compose)
    // Gate 3: app desired_state gate — require desired_state == "removed" or force flag
    // For now, gate only on active deployment; add isActiveStatus check is the P0.

    // Resolve backing resources before row deletion so we can sweep.
    var replicaAppIDs []string
    services, _ := s.store.ListAppServices(ctx, appID)
    for _, svc := range services {
        if svc.ReplicaAppID != nil {
            replicaAppIDs = append(replicaAppIDs, *svc.ReplicaAppID)
        }
    }
    // Also resolve compose stacks tied to this app (if mapping exists)
    // var composeStacks []store.ComposeStack // via app_id foreign or name convention

    // Sweep phase — best-effort, never block row delete on beacon failure? Choose: fail-closed for orphan safety.
    // Option: enqueue cleanup job instead of inline sweep so delete is async but durable.

    // Cancel placement reservations tied to this app's server
    if app.ServerID != nil {
        _ = s.store.CancelReservationsByServer(ctx, *app.ServerID) // new helper
    }

    // Dispatch replica deletions (reuse replicamanager logic but via injected interface to avoid import cycle)
    if s.replicaDeleter != nil {
        for _, rid := range replicaAppIDs {
            _ = s.replicaDeleter.DeleteApp(ctx, rid) // cancels reservations + beacon StopInstance
        }
    }

    // Beacon compose sweep if applicable
    if s.composeDeleter != nil {
        // For each compose stack tied to app, call ComposeDelete with removeOrphans=true
    }

    // Finally delete row
    if err := s.store.DeleteApplication(ctx, appID); err != nil {
        return err
    }
    // Enqueue orphan check (in case beacon sweep missed containers)
    _ = s.store.RecordOrphanCleanupRequest(ctx, appID, time.Now().UTC()) // optional table
    return nil
}
```

Store helper `CancelReservationsByServer`:

```go
func (s *Store) CancelReservationsByServer(ctx context.Context, serverID string) error {
    _, err := s.db.Exec(ctx, `
        UPDATE placement_reservations
        SET status = 'cancelled', updated_at = now()
        WHERE server_id = $1 AND status IN ('pending','active')
    `, serverID)
    return err
}
```

For the web handler `handlers_apphosting.go:259` `DELETE /apps/:id`, map the gate error to `409`:

```go
if err := appSvc.DeleteApp(ctx, c.Params("id"), app.OrgID); err != nil {
    if strings.Contains(err.Error(), "delete refused:") {
        return fiber.NewError(fiber.StatusConflict, err.Error())
    }
    return respondStoreError(err)
}
```

**Reservation + orphan wiring:** `replicamanager/service.go:501` `DeleteApp` already cancels reservations + deletes instances. The app platform should call it — but `apphosting` cannot import `replicamanager` due to cycle. Inject via interface `ReplicaDeleter` set in `main.go`:

```go
type ReplicaDeleter interface {
    DeleteApp(ctx context.Context, appID string) error
}
type Service struct {
    store          Store
    tenancySvc     *tenancy.Service
    replicaDeleter ReplicaDeleter // new
}
func (s *Service) SetReplicaDeleter(d ReplicaDeleter) { s.replicaDeleter = d }
```

Similarly `ComposeDeleter` for compose stacks.

**Backward compat:** Existing apps with no active deployment delete as before. Only active-deployment deletes now get `409` — correct, prevents beacon leak.

**Alternative async delete:** If operator wants delete to succeed even while deployment is running, enqueue a cleanup job (`operations` table) and return `202 Accepted` with `Retry-After`. Not recommended for P0 — synchronous gate is simpler and matches 1Panel `DeleteCheck`.

---

### 2.6 AP-07 — Scale: Make DB and Placement Atomic

#### 2.6.1 Current bug

```go
// apphosting/service.go:440-449
updated, err := svc.store.UpdateAppService(ctx, serviceID, input) // 1) DB mutated
if updated.ReplicaAppID != nil {
    _, err = svc.store.UpdateReplicaAppReplicas(ctx, *updated.ReplicaAppID, targetReplicas) // 2) second DB write
    // 3) BUT ReplicaManager.ScaleApp is called by HANDLER after, not here — split-brain if placement fails
}
```

Handler `handlers_apphosting.go:433`:

```go
updated, e := appSvc.UpdateService(...) // DB done
if req.Replicas != nil && updated.ReplicaAppID != nil && cfg.ReplicaManager != nil {
    if rmErr := cfg.ReplicaManager.ScaleApp(ctx, *updated.ReplicaAppID, *req.Replicas); rmErr != nil {
        return 500 // DB shows 5, placement still 3
    }
}
```

#### 2.6.2 Fix — transaction or placement-first

**Placement-first (simpler, no tx):**

```go
func (svc *Service) ScaleService(ctx context.Context, serviceID, appID, orgID string, targetReplicas int) (*store.AppService, error) {
    // ... belongs checks, load existing, ReplicaAppID nil guard ...
    // 1) Call placement first
    existing, _ := svc.store.GetAppService(ctx, serviceID)
    if targetReplicas > 0 && existing.ReplicaAppID == nil {
        return nil, errors.New("scale refused: service has no replica placement (ReplicaAppID is unset)")
    }
    if svc.replicaScaler != nil && existing.ReplicaAppID != nil {
        if err := svc.replicaScaler.ScaleApp(ctx, *existing.ReplicaAppID, targetReplicas); err != nil {
            return nil, fmt.Errorf("placement scale failed (db unchanged): %w", err)
        }
    }
    // 2) Only on placement success, mutate DB
    input := store.UpdateAppServiceInput{Replicas: &targetReplicas}
    updated, err := svc.store.UpdateAppService(ctx, serviceID, input)
    if err != nil { return nil, fmt.Errorf("scale service: %w", err) }
    if updated.ReplicaAppID != nil {
        if _, err := svc.store.UpdateReplicaAppReplicas(ctx, *updated.ReplicaAppID, targetReplicas); err != nil {
            // Non-fatal divergence: placement already scaled, DB mirror failed.
            // Log and return updated so caller sees desired replicas.
            // A reconciler should repair UpdateReplicaAppReplicas if needed.
            return updated, fmt.Errorf("service scaled but replica app update failed (non-fatal): %w", err)
        }
    }
    return updated, nil
}
```

This inverts order so DB is not ahead of reality. The handler then simplifies to not calling `ScaleApp` again:

```go
// handlers_apphosting.go:416 — handler no longer needs to call ScaleApp separately
protected.Patch("/organizations/:orgId/apps/:appId/services/:serviceId", tenantAccess, mutationLimiter, func(c *fiber.Ctx) error {
    // ...
    updated, e := appSvc.UpdateService(ctx, c.Params("serviceId"), c.Params("appId"), oc.OrgID, req)
    // ScaleService now drives placement internally; no second ScaleApp call.
    if e != nil { return fiber.NewError(500, e.Error()) }
    return c.JSON(updated)
})
```

**Transactional alternative** (if `UpdateAppService` + `UpdateReplicaAppReplicas` must be atomic):

```go
tx, _ := s.db.Begin(ctx)
updated, _ := s.store.UpdateAppServiceTx(ctx, tx, serviceID, input)
_, _ = s.store.UpdateReplicaAppReplicasTx(ctx, tx, *updated.ReplicaAppID, targetReplicas)
tx.Commit()
```

But placement call cannot be in the DB tx (network). So placement-first is preferred. Document remaining `UpdateReplicaAppReplicas` mirror as best-effort; add a future reconciler that heals `app_services.replicas` vs `replica_applications.replicas`.

Inject `ReplicaScaler` interface to avoid cycle:

```go
type ReplicaScaler interface {
    ScaleApp(ctx context.Context, appID string, targetReplicas int) error
}
```

**Zero-replica stop:** Allow `targetReplicas==0` to mean stop (already allowed `service.go:418-419`), but require explicit `desired_state:"stopped"` as well for clarity in next iteration.

---

### 2.7 AP-10 — env_file / Include / Template Expansion

#### 2.7.1 Current

* `compose/service.go:134` `EnvFile interface{}` parsed into `rawService` but never used.
* `compose/parser.go:197` `loader.LoadWithContext` empty env — drops `env_file`.
* `beacon/compose.go:380` `.env` only from `EnvVars` map — no `env_file` mount.

Impact: a compose referencing `env_file: ./secrets.env` deploys with empty secrets.

#### 2.7.2 Fix — fail-fast until wired, then expand

**Phase 1 (P0): fail-fast validation**

Make `ValidateCompose` reject `env_file` / `include` present, so silent drop becomes explicit error:

```go
// compose/service.go — new validation in ValidateCompose
func (s *Service) ValidateCompose(content []byte, workingDir string) *ValidateResult {
    result := &ValidateResult{Valid: true}
    // Early raw check before interpolation
    var raw rawCompose
    if err := yaml.Unmarshal(content, &raw); err == nil {
        for name, svc := range raw.Services {
            // Check raw yaml for env_file key via map trick
            var m map[string]any
            if err := yaml.Unmarshal(content, &m); err == nil {
                if svcs, ok := m["services"].(map[string]any); ok {
                    if sm, ok := svcs[name].(map[string]any); ok {
                        if _, has := sm["env_file"]; has {
                            result.Valid = false
                            result.Errors = append(result.Errors, ValidationError{
                                Field: fmt.Sprintf("services.%s.env_file", name),
                                Message: "env_file is not yet supported — inline vars via EnvVars or use secrets",
                            })
                        }
                    }
                }
            }
        }
        // Also check top-level include with env_file
        for _, inc := range raw.Include {
            if inc.EnvFile != nil {
                result.Valid = false
                result.Errors = append(result.Errors, ValidationError{
                    Field: "include.env_file",
                    Message: "include with env_file is not yet supported",
                })
            }
        }
    }
    // ... rest of ValidateCompose
}
```

This surfaces `AP-10` immediately at `POST /compose/validate` and `POST /compose` admission.

**Phase 2 (when beacon can mount):** Expand `env_file` by resolving relative to `workingDir`, reading files, merging with `EnvVars` (caller-provided wins), and writing to `.env` before `docker compose up`.

```go
// New helper
func (s *Service) resolveEnvFile(envFile interface{}, workingDir string, envVars map[string]string) (map[string]string, error) {
    // envFile can be string or []any of string|map
    // Resolve each path via filepath.Join(workingDir, path) + secure_files check
    // Parse dotenv format (key=val, ignore #)
    // Return merged map
}

// In ParseComposeYAML, after interpolateEnv but before yaml.Unmarshal:
// if raw.Include/EnvFile present, expand and fold into envVars, then re-interpolate.
```

Beacon's `handleComposeDeploy` would likewise need to accept `env_file` payload or be sent expanded env.

Template expansion (`compose/service.go:352` `ExpandTemplate`) already handles `${VAR:-default}` etc. Ensure `isSensitive` handling for `AP-14` catalog swallowing is fixed in next section.

---

### 2.8 AP-14 — Catalog Swallowing / env/mounts Template Expansion

#### 2.8.1 Current

`appstore/service.go:239` `resolveTemplate` interpolates template `composeYaml` with user vars via `ExpandTemplate`, but on interpolation error (`${VAR:?msg}` missing), it **swallows** the error and deploys the raw template (stale compose).

```go
// appstore/service.go:239 (approx)
expanded, err := compose.ExpandTemplate([]byte(tmpl.ComposeYAML), vars)
if err != nil {
    // BUG: swallows error, returns raw tmpl.ComposeYAML
    expanded = []byte(tmpl.ComposeYAML)
}
```

#### 2.8.2 Fix — fail-fast

```go
func (s *Service) resolveTemplate(tmpl *CatalogEntry, vars map[string]string) ([]byte, error) {
    if tmpl.ComposeYAML == "" {
        return nil, errors.New("template has no compose content")
    }
    expanded, err := s.composeSvc.ExpandTemplate([]byte(tmpl.ComposeYAML), vars)
    if err != nil {
        return nil, fmt.Errorf("template interpolation failed: %w", err)
    }
    // Validate after expansion — catch missing isSensitive vars that expanded to ""
    if result := s.composeSvc.ValidateCompose(expanded, ""); !result.Valid {
        return nil, fmt.Errorf("expanded template invalid: %s - %s", result.Errors[0].Field, result.Errors[0].Message)
    }
    return expanded, nil
}
```

Caller `Provision` must propagate error to API as `422`:

```go
content, err := s.resolveTemplate(entry, vars)
if err != nil {
    return nil, fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
}
```

**`isSensitive` handling:** Template env vars marked `isSensitive:true` must not be echoed in validation errors or logs. Ensure `ExpandTemplate` does not log values; API error messages reference only field names.

Also fix `appstore/service.go:158` `UpgradeApp` ignoring `app_ignore_upgrade` + cross-version:

```go
func (s *Service) UpgradeApp(ctx context.Context, appID string) error {
    app, _ := s.store.GetAppStoreInstall(ctx, appID)
    if app.IgnoreUpgrade { return errors.New("upgrade ignored by user preference") }
    if isCrossVersion(app.Version, app.TargetVersion) {
        return errors.New("cross-version upgrade requires manual migration")
    }
    // proceed
}
```

Where `isCrossVersion` mirrors `1panel/utils IsCrossVersion` (major version jump).

---

### 2.9 Dependencies / Convergence (AP-11) and Mount Isolation (AP-13)

#### AP-11 — Dependencies

Short-term: document that `depends_on` ordering is delegated to `docker compose up -d`. Add a warning in `ValidateCompose`:

```go
for _, svc := range parsed.Services {
    if len(svc.DependsOn) > 0 {
        result.Warnings = append(result.Warnings, ValidationError{
            Field: fmt.Sprintf("services.%s.depends_on", svc.Name),
            Message: "service ordering delegated to docker compose; condition: service_healthy not enforced beyond start order",
        })
    }
}
```

Long-term: implement `InDependencyOrder` traversal before `PlaceReplicas` when Forge moves to per-service placement.

#### AP-13 / AP-10 overlap — Mount isolation unification

See §2.7's bifurcation fix. Single predicate is `ValidateHostMountWithAllowlist` (`compose/service.go:663`):

```go
// API ValidateComposeSecurity should call ValidateHostMountWithAllowlist when allowedMounts available,
// else error on absolute. Beacon validateComposeVolumes should be relaxed to call same allowlist path.
// For now, make API match beacon's strictness: reject all absolute mounts at API as well.
func checkVolumesSecurity(svcName string, volumes interface{}, issues *[]ValidationIssue) {
    // ... existing ...
    if strings.HasPrefix(source, "/") {
        severity := "error" // was warning for /data; now error to match beacon
        // Unless source is on operator allowlist — but API has no allowlist yet.
        // So error, with message explaining to request allowlist entry.
    }
}
```

And plumb `allowedMounts+isAdmin` via `ComposeDeployRequest`:

```go
// daemon/compose.go:13
type ComposeDeployRequest struct {
    StackID      string            `json:"stackId"`
    ComposeYAML  string            `json:"composeYaml"`
    EnvVars      map[string]string `json:"envVars,omitempty"`
    AllowedMounts []string         `json:"allowedMounts,omitempty"` // new
    IsAdmin      bool              `json:"isAdmin,omitempty"`       // new
}
```

Beacon `validateComposeVolumes` then accepts `source` if `ValidateHostMountWithAllowlist(source, isAdmin, allowedMounts)==nil`.

---

### 2.10 Frontend — App List/Detail, TypeIcons, Router-Driven Tabs, DeployStatusBadge, Progress Consolidation

#### 2.10.1 `typeIcons` fix (`web/app/admin/apps/page.tsx:16`)

```ts
// Before
const typeIcons: Record<AppType, typeof Container> = {
  image: Box,
  git: GitBranch,
  compose: Container, // Container is lucide icon; also global DOM type
  game_server: Layers,
};
// After
import { Container as ContainerIcon } from "lucide-react";
const typeIcons: Record<AppType, typeof ContainerIcon> = {
  image: Box,
  git: GitBranch,
  compose: ContainerIcon,
  game_server: Layers,
};
```

Also dedupe `OfflineBanner` rendered twice `page.tsx:75-76`.

#### 2.10.2 Router-driven tabs (`web/app/admin/apps/[id]/page.tsx:42`)

Current `useState(tab)` initialized from `searchParams.get("tab")` once on mount. Navigating back or sharing URL with `?tab=deployments` after mount does not update state.

```tsx
// Fix: sync `tab` from searchParams reactively
const searchParams = useSearchParams();
const router = useRouter();
const initialTab = (searchParams.get("tab") as TabId) || "overview";
const [tab, setTab] = useState<TabId>(initialTab);

// Keep tab in sync when URL changes (back/forward, external link)
useEffect(() => {
  const next = (searchParams.get("tab") as TabId) || "overview";
  if (next !== tab) setTab(next);
}, [searchParams]);

// On tab click, use Next router rather than window.history
const handleTab = (tId: TabId) => {
  setTab(tId);
  router.replace(`/admin/apps/${id}?tab=${tId}` as any, { scroll: false });
};
```

Consider `useRouter` from `next/navigation` already imported — use `router.replace` not `window.history.replaceState`.

#### 2.10.3 `DeployStatusBadge` centralization

`AdminAppsShared.tsx:9` is canonical. Remove per-file `statusConfig` maps:

* `web/app/admin/compose/page.tsx` has 9-state `statusConfig` — map its states to `DeployStatusBadge` tones or extend `statusTone`:

```ts
// apps.ts — extend statusTone to cover compose states
export function statusTone(status: string): ... {
  switch (status) {
    case "idle": return "neutral";
    case "deploying": return "blue";
    case "deleting": return "yellow";
    case "deleted": return "neutral";
    case "failed": return "red";
    default: return statusToneBase(status);
  }
}
```

Then `compose/page.tsx` imports `DeployStatusBadge`.

#### 2.10.4 Deployment progress consolidation

Two components polling same `GET /admin/deployments/:id/steps`:

* `deployment-progress.tsx:41` poll 2000ms, auto-dismiss after 10m, `queryKey ["deployment-steps", id]`
* `DeploymentTimeline.tsx:27` poll 5000ms, same key, different interval

Since `queryKey` is identical, TanStack Query only runs one interval — but which one wins depends on mount order. Still wasteful and confusing.

**Plan:** Keep `deployment-progress.tsx` as canonical progress (richer: `RefreshCw` retry, `staleTime 1000`, `gcTime 5m`, `placeholderData`, error recovery). Make `DeploymentTimeline` a **presentational** wrapper around the same data, or deprecate it and reuse `DeploymentProgress` in its slot.

```tsx
// web/components/charts/DeploymentTimeline.tsx — change to accept steps prop
export function DeploymentTimeline({ steps }: { steps: DeploymentStep[] }) {
  // purely presentational: vertical timeline of steps
}

// caller chooses one or the other, not both:
import { DeploymentProgress } from "@/components/app/deployment-progress";
// and not DeploymentTimeline polling separately
```

If both must coexist, dedupe query:

```ts
// lib/api/deployments.ts — single source of interval constant
export const DEPLOYMENT_STEPS_POLL_MS = 2000;
export const DEPLOYMENT_STEPS_MAX_MS = 10 * 60 * 1000;

// both components import same key + interval via shared util
export function useDeploymentSteps(id: string) {
  return useQuery({
    queryKey: ["deployment-steps", id] as const,
    queryFn: ({ signal }) => fetchDeploymentSteps(id, { signal }),
    refetchInterval: (q) => {
      const data = q.state.data as DeploymentStep[] | undefined;
      if (data && data.every(s => isTerminal(s.status))) return false;
      return DEPLOYMENT_STEPS_POLL_MS;
    },
    staleTime: 1000,
  });
}
```

Update both components to `useDeploymentSteps`.

Also consolidate log placement: `DeploymentLogViewer` currently lives in a modal not under `DeploymentTimeline`. Per UX findings, inline it below timeline — trivial JSX move.

#### 2.10.5 Missing `restartComposeStack` export

```ts
// web/lib/api/compose.ts — add
export function restartComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
}
```

Wire UI button next to stop/start in `web/app/admin/compose/[id]/page.tsx`.

---

## 3. Migrations — SQL (Backward Compatible)

All migrations are additive, idempotent `IF NOT EXISTS`.

### 3.1 Deployments → ReplicaApp linkage

```sql
-- migrations/211_deployments_replica_link.sql
ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS replica_app_id UUID REFERENCES replica_applications(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS app_id UUID REFERENCES applications(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_deployments_replica_app ON deployments(replica_app_id);
CREATE INDEX IF NOT EXISTS idx_deployments_app ON deployments(app_id);
```

Needed for `PlacementExecutor.resolveReplicaAppID` without fuzzy `applications.server_id` lookup.

### 3.2 Health gate merge

```sql
-- migrations/212_health_gate_beacon.sql
ALTER TABLE health_check_configs
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'deployment',
  ADD COLUMN IF NOT EXISTS deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_health_check_configs_deployment
  ON health_check_configs(deployment_id);
```

### 3.3 Reconciler index

```sql
-- migrations/213_app_reconciler_index.sql
CREATE INDEX IF NOT EXISTS idx_applications_reconcile
  ON applications(desired_state, observed_status)
  WHERE desired_state != observed_status;

-- Applications gained 'removed' early; ensure observed_status can be 'error'
ALTER TABLE applications
  DROP CONSTRAINT IF EXISTS applications_observed_status_check;
ALTER TABLE applications
  ADD CONSTRAINT applications_observed_status_check
  CHECK (observed_status IN ('idle','running','done','error','deploying','stopped','failed'));
```

If native `ADD CONSTRAINT IF NOT EXISTS` not supported, wrap in `DO $$ BEGIN ... EXCEPTION WHEN duplicate_object THEN NULL; END $$`.

### 3.4 Deployment version helper (no DDL — app code only)

No migration needed; `deployments.version` already exists (`105_deployment_version.sql`). Add index if missing:

```sql
CREATE INDEX IF NOT EXISTS idx_deployments_version ON deployments(id, version);
```

---

## 4. Rollout Steps — Sequenced

### Phase 0 — Feature flags (day 0, no behavior change)

Add env vars (all default to safehonest):

```
FORGE_DEPLOY_REQUIRE_PLACEMENT=true     # fail-closed for TargetReplicas>1 when placement nil
FORGE_DEPLOY_REQUIRE_TRAFFIC=false      # off until trafficmanager wired (§4.2)
FORGE_RECONCILER_ENABLED=false          # off until reconciler deployed
FORGE_HEALTH_VIA_BEACON=true            # try beacon first, fallback to direct
FORGE_ENV_FILE_STRICT=true              # reject env_file with 422
```

Gate each new executor behind its flag so rollback is `env var flip`.

### Phase 1 — Safety gates (P0, 1-2 days, no new tables)

1. **AP-05 revisions** — land `CreateDeploymentRevisionTx` + `RollbackToRevision` CAS tx (§2.4). No flag. Immediate 409 on concurrent rollback. Risk low — tx only narrows window.
2. **AP-10 env_file strict** — add `ValidateCompose` rejection for `env_file/include` (§2.7 Phase 1). Flag `FORGE_ENV_FILE_STRICT`.
3. **AP-14 catalog swallow** — make `resolveTemplate` error fail-fast (§2.8). No flag — current behavior is data loss (stale compose).
4. **AP-06 delete gate** — add `isActiveStatus` guard in `DeleteApp` (§2.5). Flag `FORGE_DELETE_REQUIRE_IDLE=true` default.

Verify: `go test ./internal/services/deployment -run TestRevision` — add new `TestConcurrentRollback409` + `TestEnvFileRejected`.

### Phase 2 — Execution honesty (P0, 3-5 days)

1. **AP-02 provision** already honest; add `PlacementExecutor` + `TrafficExecutor` interfaces with flag-gated fail-closed (`§2.1`). Wire `BeaconRuntimeExecutor` health probe extension (`ContainerHealth`).
2. **AP-08 health** — land `validateHealthGateTarget` relaxation (§2.3.3) + beacon-local probe (§2.3.4) behind `FORGE_HEALTH_VIA_BEACON`.
3. **AP-07 scale** — invert DB↔placement order in `ScaleService` (§2.6), inject `ReplicaScaler` interface, simplify handler.
4. **Migrations** `211..213` deploy (idempotent, no downtime).

Verify: `go test ./internal/services/deployment -run TestExecuteProvision -run TestHealthGate`, plus manual `curl POST /admin/deployments/blue-green` with `TargetReplicas=3` against a test node — expect placement calls in logs.

### Phase 3 — Reconciler (P1, 1 week)

1. **AP-03 start/stop** — make handler optimistic-sync (§2.2.2) + add `appreconciler` service (§2.2.3) behind `FORGE_RECONCILER_ENABLED`. Register in `cmd/api/main.go` `Start/Stop`.
2. **Compose restart atomicity** — make `RestartStack` reservation-aware + health-verified (§2.2.4), export `restartComposeStack` in `compose.ts`.

Verify: `POST /apps/:id/stop` with node offline → `202 Accepted` + reconciler retries when node returns. `POST /apps/:id/start` with node online → `200 {observed_status:running}` immediately.

### Phase 4 — Frontend consolidation (P2, 2-3 days, independent of backend)

1. Fix `typeIcons` shadow, dedupe `OfflineBanner`, router-driven tabs `replace` (§2.10.1-2.10.2).
2. Centralize `DeployStatusBadge` — remove per-page `statusConfig`.
3. Consolidate `DeploymentProgress` vs `DeploymentTimeline` via `useDeploymentSteps` (§2.10.4).
4. Export `restartComposeStack` + UI button; inline log viewer under timeline.

Verify: manual UX crawl — refresh on `?tab=deployments` preserves tab via router, back button works, single network poll for steps.

### Phase 5 — Optional follow-ups (P2/P3, not blocking P0)

* Plumb `allowedMounts+isAdmin` via `ComposeDeployRequest` (§2.9).
* Expand `env_file` for real via `resolveEnvFile` + beacon mount (§2.7 Phase 2).
* Merge `health_check_configs` fully with deployment gate (dropdown in app detail "Health Probe" section).
* Autoscaler HPA binding for replica count (`autoscaler/service.go:332` currently vertical only).

---

## 5. Testing & Verification

### 5.1 Unit tests (new)

| Test | Covers | File |
|------|--------|------|
| `TestCreateRevision_Concurrent` (2 goroutines, same deploymentID) | §2.4 race → one 23505 retry → unique revision_numbers monotonic | `deployment/revisions_test.go:NEW` |
| `TestRollbackToRevision_Concurrent409` | two rollbacks same version → second 409 `ErrVersionConflict` | `deployment/revisions_test.go:NEW` |
| `TestDeleteApp_ActiveDeployment409` | delete with `pending` deployment → 409 | `apphosting/service_test.go:NEW` |
| `TestScaleService_PlacementFirst` | placement fails → DB unchanged | `apphosting/service_test.go:NEW` |
| `TestValidateHealthGateTarget_ExplicitHostAllowed` | `healthCheckHost:"10.0.1.5"` accepted, not clamped | `deployment/healthgate_target_test.go:MOD` |
| `TestValidateCompose_EnvFileRejected` | compose with `env_file:` → `Valid:false` | `compose/service_test.go:NEW` |
| `TestExpandTemplate_RequiredVarError` | `${VAR:?missing}` → error propagated, not swallowed | `compose/service_test.go:MOD` |
| `TestExecuteProvision_ReplicatedRequiresPlacement` | `TargetReplicas=3` without executor → honest failure not completed | `deployment/execution_test.go:NEW` |

Existing relevant: `deployment/provision_regression_test.go`, `deployment/healthgate_target_test.go:1`, `deployment/healthgate_e2e_test.go:1`, `apphosting/service_test.go:1`.

### 5.2 Integration / E2E

* `e2e/health_gated_deployment_test.go:1` — extend to cover beacon-local probe path (mock daemon `HealthProbe` returns `passed:true`).
* `integration/e2e_test.go` — add `TestAppStartStopReconcile` (start with node offline, bring node online, assert `observed_status→running` within 60s).
* Beacon compose E2E `beacon/internal/server/compose.go:344` — validate `/data:/data` now rejected at API pre-check (422) not after reservation (400).

### 5.3 Manual smoke checklist

```
# 1. Deployment stub honesty
curl -X POST /admin/deployments/blue-green -d '{"serverId":"$SID","image":"nginx@sha256:...","healthCheckPath":"/health","healthCheckPort":80,"healthGateEnabled":true}'
→ {"data":{"id":"...","status":"pending"}}
curl /admin/deployments/$ID/steps
→ init completed, provision in_progress → if no RuntimeExecutor wired → failed "no runtime executor wired" (honest), not completed.

# 2. Health gate explicit host
curl -X POST /admin/deployments/recreate -d '{"serverId":"$SID","image":"nginx@sha256:...","healthCheckHost":"10.0.1.20","healthCheckPort":80,"healthCheckPath":"/"}'
→ 201 (was 400 "may only target local" before fix)

# 3. Delete gate
curl -X DELETE /apps/$APP  # while deployment pending
→ 409 "delete refused: deployment ... is in_progress"

# 4. Scale split-brain
curl -X PATCH /organizations/$ORG/apps/$APP/services/$SVC -d '{"replicas":5}' # placement down → 500, DB still 3
curl /apps/$APP/services/$SVC → replicas 3 not 5

# 5. Env_file
curl -X POST /compose/validate -d '{"content":"services:\n  web:\n    image: nginx\n    env_file: [./.env]"}'
→ {"valid":false,"errors":[{"field":"services.web.env_file","message":"env_file is not yet supported"}]}

# 6. Frontend
# Open /admin/apps/$ID?tab=deployments → refresh → stays on deployments tab (router-driven)
# Open deployment steps page → single fetch for steps (Network tab shows 1 poller)
```

---

## 6. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `RollbackToRevision` tx locks deployment row, blocking concurrent step executors | Medium (rollback rare) | p95 latency +50ms during rollback | `FOR UPDATE NOWAIT` alternative; or keep existing two-step with `ErrVersionConflict` retry — caller sees 409 and retries |
| `reconciler` fights active rollout (both drive `observed_status`) | Medium | Flapping status | Reconciler skips apps with active deployment (`HasActiveDeployment` check §2.2.3) |
| Beacon-local health probe requires new beacon endpoint → old beacons return 404, health gate fails | Medium | All health-gated deploys fail after API upgrade until beacons roll | Fallback to direct dial on 404 (`tryBeaconHealthProbe` returns nil on error); mark beacon version in `store_capabilities.go` capability check |
| `allowedMounts` not yet plumbed — strict API rejection for all absolute mounts breaks existing `/data` users | High if flipped now | Compose stacks with `/data:/data` start failing at validate (was deploy) | Keep Phase 0 flag off; Phase 1 strictness is intentional — operators must request allowlist; document migration path (use named volumes `postgres-data:/var/lib/db` instead of bind) |
| TX helper `CreateDeploymentRevisionTx` needs `pgx.Tx` — `Store.db` is `pgxpool.Pool` not `DBTX` interface | Low | Compile error | Add `DB() pgxpool.Pool` already exists (`replicamanager/service.go:240` `DB().Begin`), use pool's `Begin` — no new plumbing |
| Frontend `DeploymentTimeline` consumers expect polling timeline — making it presentational breaks them | Low | Stale timeline | Keep `useDeploymentSteps` polling in hook; timeline becomes presentational but still shows live data via parent poll |

---

## 7. Backward Compatibility Notes

* **Deployments:** When `PlacementExecutor`/`TrafficExecutor` nil, behavior is **more strict** (steps fail with `no ... executor wired`) not looser. Existing single-server `TargetReplicas=1` path unchanged. Rollout flag `FORGE_DEPLOY_REQUIRE_PLACEMENT=false` restores old "verifyOnly" for emergency downgrade.
* **Health gate validation:** Relaxing `validateHealthGateTarget` from loopback clamp to any hostname is **strictness relax** — old invalid configs remain invalid, new configs that were rejected become accepted. No breakage.
* **Revisions tx:** New tx serializes `revision_number` correctly; existing revisions unaffected. Duplicate `revision_number` now surfaces as honest 23505 retry, not silently skipped.
* **Delete gate:** New 409 is a strictness **increase** — clients that deleted during deploy will now need to cancel first. Documented as breaking but correct; add `force=true` query param if emergency delete must bypass (logs warning).
* **Scale order inversion:** No API shape change; only internal order `placement→db`. Clients see more correct 500 on placement failure vs silent lie.
* **Env_file strict:** New 422 for `env_file:` is strictness increase — affected users must inline vars. Provide migration guidance (convert `env_file: .env` → `EnvVars` map).
* **Frontend:** `typeIcons` import rename is type-only. Router-driven tabs change `window.history.replaceState` to `next/navigation router.replace` — same URL shape.

---

## 8. File:Line Index — All Citations

| Symbol | File:Line | Used In |
|--------|-----------|---------|
| `ExecuteDeployment` DAG | `deployment/execution.go:22` | §1.1, §2.1 |
| `RuntimeExecutor` honest fail-closed | `deployment/execution.go:249,268` | §1.1, §2.1 |
| `executeProvisionStep` real | `deployment/execution.go:264` | §1.1, §2.1.4 |
| `executePromoteStep` DB-only | `deployment/execution.go:277` | §1.1, §2.1.4 |
| `verifyObservedRunning` boolean | `deployment/execution.go:338,313` | §1.1, §2.1.4 |
| `BeaconRuntimeExecutor.ApplyDeployment` | `deployment/beacon_executor.go:33` | §1.1, §2.1.4 |
| `BeaconRuntimeExecutor.VerifyRunning` Stats | `deployment/beacon_executor.go:60` | §1.1, §2.3 |
| `resolveNodeHost` fix | `deployment/healthgate.go:62` | §1.6, §2.3.1 |
| `CheckHealth` direct dial | `deployment/healthgate.go:37` | §1.6, §2.3.2 |
| `validateHealthGateTarget` clamp | `deployment/rollout.go:71,81` | §1.6, §2.3.3 |
| `applyRolloutRequest` replica clobber | `deployment/rollout.go:92,111` | MASTER FINDING, §2.1 future fix also needed |
| `CreateRevision` race | `deployment/revisions.go:82,87` | §1.3, §2.4.1 |
| `RollbackToRevision` ungated write | `deployment/revisions.go:147,167,174` | §1.3, §2.4.1 |
| `UpdateDeployment` unversioned | `store_deployments.go:128` | §1.3, §2.4.2 |
| `UpdateDeploymentConfig` versioned | `store_deployments.go:195` | §1.3, §2.4.4 |
| `ErrVersionConflict` | `store_deployments.go:193` | §2.4.4 |
| `UNIQUE(deployment_id, revision_number)` | `099_deployment_revisions.sql:21` | §1.3, §2.4.2 |
| `POST /apps/:id/start` desired-only | `handlers_apphosting.go:450,468` | §1.2, §2.2.2 |
| `POST /apps/:id/stop` desired-only | `handlers_apphosting.go:475` | §1.2, §2.2.2 |
| `POST /apps/:id/restart` SendPower | `handlers_apphosting.go:500,532` | §1.2, §2.2.2 |
| `instanceLifecycleHandler` correct | `handlers_apphosting.go:1083,1138` | §1.2 |
| `UpdateApplication` dynamic SET | `store_apphosting.go:313` | §1.2 |
| `DeleteApplication` unconstrained | `store_apphosting.go:374` | §1.4, §2.5 |
| `DeleteApp` app platform | `apphosting/service.go:175` | §1.4, §2.5 |
| `ScaleService` admission fixed, DB-before-placement | `apphosting/service.go:398,432,440` | §1.5, §2.6 |
| `PATCH .../services/:id` handler double-scale | `handlers_apphosting.go:416,433` | §1.5, §2.6 |
| `replicamanager/reconcile` exists | `replicamanager/service.go:732` | §1.2, §2.2.3 |
| `ScaleApp` placement | `replicamanager/service.go:361` | §1.5, §2.1 |
| `DeleteApp` replicamanager sweep | `replicamanager/service.go:501` | §1.4, §2.5 |
| `EnvFile` dropped | `compose/service.go:134` | §1.7, §2.7 |
| `Environment:map{}` empty | `compose/parser.go:197` | §1.7 |
| `interpolateEnv` | `compose/service.go:352,356` | §1.7, §2.8 |
| `checkVolumesSecurity` warns on /data | `compose/service.go:594,634` | §1.7, §2.9 |
| `validateComposeVolumes` rejects absolute | `beacon/internal/server/compose.go:226,238` | §1.7, §2.9 |
| `ValidateHostMountWithAllowlist` gate | `compose/service.go:663` | §1.7, §2.9 |
| `ComposeDeployRequest` no allowlist | `daemon/compose.go:13` | §2.9 |
| `health_check_configs` disjoint | `114_e_zero_downtime_deploy.sql:15` | §1.6, §2.3.2 |
| `appStore resolveTemplate swallow` | `appstore/service.go:239` | §1.7, §2.8 |
| `compose.ts` missing restart export | `web/lib/api/compose.ts:89` | §1.8, §2.10.5 |
| `deployment-progress.tsx` 2s poll | `web/components/app/deployment-progress.tsx:41` | §1.8, §2.10.4 |
| `DeploymentTimeline` 5s poll same key | `web/components/charts/DeploymentTimeline.tsx:27` | §1.8, §2.10.4 |
| `DeployStatusBadge` canonical | `web/components/admin/AdminAppsShared.tsx:9` | §1.8, §2.10.3 |
| `typeIcons` shadow | `web/app/admin/apps/page.tsx:16` | §1.8, §2.10.1 |
| `useState(tab)` not router-synced | `web/app/admin/apps/[id]/page.tsx:42` | §1.8, §2.10.2 |

---

## 9. Cross-Reference to Other Subagents

* **Subagent 01 (Game Hosting)** — shares `ServerControlTarget` / `SendPower` pattern; align `appreconciler` with `heartbeatmonitor` 6-state logic if app maps to a server.
* **Subagent 04 (Networking/Gateway)** — `TrafficExecutor.Promote` must coordinate with single-writer gateway reconciler (NG-01 five writers on one Caddy). Do not wire traffic promote until NG single-writer lands.
* **Subagent 05 (Orchestration)** — `PlacementExecutor` honors scheduler's `ScaleReplicas` reservation row-locks; do not bypass with direct `placement_reservations` writes.
* **Subagent 06 (Security)** — `ValidateHostMountWithAllowlist` is the SE-04 mount allowlist gate; API→beacon `allowedMounts` plumbing is security boundary.
* **Subagent 02 (Final-Parity App Platform)** itself — this plan directly addresses its LF-01..LF-06; LF-02 `TargetReplicas` clobber remains in `rollout.go:112` and should be fixed alongside §2.1 (seed from `AppService.Replicas` when `req.TargetReplicas==0`).

---

## 10. Immediate Action Checklist (for next sprint)

- [ ] Land `211..213` migrations (idempotent, no code dep).
- [ ] Ship `revisions.go` CAS tx + `validateHealthGateTarget` relax + `env_file` strict 422 — `apphosting/service.go:432` style small diffs, no new service.
- [ ] Ship `DeleteApp` gate + `ScaleService` placement-first — both single-file, high orphan/split-brain payoff.
- [ ] Introduce `PlacementExecutor` / `TrafficExecutor` interfaces behind flags, wire `HealthProbe` beacon path, add `appreconciler` behind flag off — merging without flag-on means fail-closed only.
- [ ] Frontend `compose.ts` restart export + `DeploymentProgress` dedupe — independent PR, no backend dep.
- [ ] Flip `FORGE_RECONCILER_ENABLED` + `FORGE_DEPLOY_REQUIRE_TRAFFIC` after NG single-writer and beacon health probe rolls to all nodes.

*Generated by subagent 03 (parallel). No product files modified. Evidence: SOURCE_VERIFIED file:line unless marked.*

