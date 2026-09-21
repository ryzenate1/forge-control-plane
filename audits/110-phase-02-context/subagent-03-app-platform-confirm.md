# Subagent 03 — App Platform Confirm (AP-02 / AP-03 / AP-05 / AP-06 / AP-07 / AP-08 / AP-10 + Reverif-06/07)

**Scope:** Confirm FINAL_PARITY AP-02, AP-03, AP-05, AP-06, AP-07, AP-08, AP-10 + reverification-06/07 findings against LIVE code (post-Phase-06 checkout 2026-08-24).
**Mode:** Read-only. No product code modified. All claims `file:line` SOURCE_VERIFIED.
**Prompt targets:** `deployment/execution.go:249,249-311` stubs vs honest provision, `healthgate.go:62` resolveNodeHost, `rollout.go:92` applyRolloutRequest, `revisions.go:147` RollbackToRevision, `replicamanager/service.go:432` (actually `apphosting/service.go:432`), `apphosting/service.go:175`, `compose/service.go:594` vs `beacon/compose.go:226`.

---

## 1. Direct Target File Inspection (prompt-mandated lines)

| Target | LIVE content | Verdict vs prompt question |
|---|---|---|
| `forge/api/internal/services/deployment/execution.go:249` `type RuntimeExecutor` | `execution.go:249-262` defines `RuntimeExecutor` iface with doc `When nil, provision steps fail honestly instead of reporting success ... (Phase-1 F-01 / FORGE-LOGIC-001: stubs reported completed with zero containers)` | **Existence confirmed** — honest-failure gate introduced |
| `execution.go:249-311` former stubs `executeProvision/Promote/Drain/Scale/Cleanup` `return nil` | `execution.go:264` `executeProvisionStep` now `if s.runtime==nil → error "no runtime executor wired; refusing to report success"` else `s.runtime.ApplyDeployment(ctx, ServerID, Image)` ; `execution.go:277` `executePromoteStep` still only `UpdateDeploymentConfig` + publish `deployment_promoted` (no trafficmanager) ; `execution.go:299` `executeDrainOldStep` + `306` `DrainCanary` + `313` `ScaleUp` + `320` `ScaleDown` all `→ verifyObservedRunning(ctx, ServerID) bool` ; `execution.go:327` `executeCleanupStep` pure `publish→nil` | **Recreate honest, others still stub/false-complete** — provision gate added, other dimensions not wired |
| `deployment/healthgate.go:62` `resolveNodeHost` | `healthgate.go:62-77` `resolveNodeHost(ctx, serverID) string` parses `Store.ServerControlTarget → NodeURL → url.Parse → Hostname()` with 10s timeout; caller `healthgate.go:23` `host = s.resolveNodeHost(ctx, ServerID)` when `HealthCheckHost==""`; empty → `Passed:false host unresolved` | **Fixed for default path** — no longer probes `localhost` control plane |
| `deployment/rollout.go:92` `applyRolloutRequest` | `rollout.go:92-117` `d.TargetReplicas = req.TargetReplicas; if d.TargetReplicas<=0 {d.TargetReplicas=1}` called from `recreateRollout:153` `rollingRollout:205` `blueGreenRollout:239` `canaryRollout:293` ; no seeding from `AppService.Replicas` ; `RolloutRequest:33` `TargetReplicas omitempty` usually 0 via `handlers_revisions.go:48` | **STILL clobbers** — silent downscale 10→1 without warning |
| `deployment/revisions.go:147` `RollbackToRevision` | `revisions.go:147-199` does `UpdateDeploymentStatusVersioned(V→V+1):167` then **non-versioned** `UpdateDeployment(toStoreDeployment):174` with stale `deployment.Version` ; `revisions.go:82` `CreateRevision` `GetLatest→nextNum+1` without TX (`revisions.go:87-91`) | **Race both ends still present** |
| `replicamanager/service.go:432` (prompt) → LIVE `apphosting/service.go:432` guard | `apphosting/service.go:432-434` `if targetReplicas>0 && existing.ReplicaAppID==nil → "scale refused: service has no replica placement"` — Phase-1 F-13 admission fix ; `replicamanager/service.go:361` `ScaleApp` itself shard-locked + generation-fenced, `361-444` honest | **Admission FIXED** ; split-brain remains (see AP-07 detail) |
| `apphosting/service.go:175` `DeleteApp` | `apphosting/service.go:175-183` only `AppBelongsToOrg` → `store.DeleteApplication(id)` ; `store/store_apphosting.go:374` `DELETE FROM applications WHERE id=$1` unconstrained ; no `isActiveStatus` guard (`rollout.go:119`), no `CancelReservation`, no `replicamanager.DeleteApp:501` sweep, no `docker compose down` | **STILL unconstrained** — orphan containers/reservations/instances |
| `compose/service.go:594` `checkVolumesSecurity` vs `beacon/compose.go:226` `validateComposeVolumes` | `compose/service.go:594-657` API blocks `/proc\|/sys\|docker.sock` hard error, `/etc\|/` error, else `/root\|/home` `warning+slog` (`service.go:634-654`); `service.go:679` `sensitiveHostPaths=["/","/root","/etc","/home"]` ; `service.go:663` `ValidateHostMountWithAllowlist` gate **never called** on compose deploy path ; Beacon `compose.go:226-244` rejects **any** absolute `source "/"` or `..` traversal or `type:bind` (`compose.go:234,244`) | **Bifurcated — STILL BROKEN** : `/data:/data` passes API `200 valid` then beacon `400 host bind mount "/data" is not allowed`; allowlist never plumbed |

---

## 2. Per-Finding Status

### AP-02 — Execution stubs lie complete (`deployment/execution.go:249-311`) — `FALSE_COMPLETION P0`

**Prior:** All steps `executeProvision/Promote/Drain/Scale/Cleanup` at `execution.go:249-311` returned `nil` after publishing event; `ExecuteDeployment:86-121` loop + `UpdateDeploymentCompletion:135` marked `completed 100%` with zero provisioning. Test asserted success without runtime. (FINAL_PARITY_AUDIT §3 AP-02 ; MASTER `REF-APP-C02-FLF01 P0` ; `final-parity/subagent-02` rows 02-05)

**LIVE:**

- `execution.go:264-274` `executeProvisionStep` **now honest**: `if runtime==nil → error "no runtime executor wired; refusing to report success without executing"` ; then `runtime.ApplyDeployment(ctx, ServerID, Image)` ; failure bubbles to `handleStepFailure:112→360` (`UpdateDeploymentFailure` + auto-rollback). Regression test `provision_regression_test.go:13` enforces. `recreateRollout` DAG `steps.go:50` `[init,provision,(health_gate),complete]` therefore honest.

- `execution.go:277-296` `executePromoteStep` still only `GetDeployment → UpdateDeploymentConfig(Version, "", "", "", newTarget) → publish deployment_promoted` with **no** `trafficmanager / loadbalancer / service_endpoints / target_groups / docker stop old` wiring. Both blue+green containers collide same host port on compose. Comment-free DB flag flip.

- `execution.go:299-325` `executeDrainOldStep:299`, `executeDrainCanaryStep:306`, `executeScaleUpStep:313`, `executeScaleDownStep:320` each only `verifyObservedRunning(ctx, ServerID) bool` (`execution.go:338-350` `VerifyRunning(ctx, ServerID) (bool,error)` ignoring `TargetReplicas`/`Replicas` count). Rolling 10 where 9 running passes; 1 of 10 after `ScaleUp` also passes. `canaryRollout:261` `CanaryPercent` stored and event-published (`rollout.go:305`) but `steps.go:75` `[init,provision,(health_gate),drain_canary,promote,complete]` ignores it.

- `execution.go:327-332` `executeCleanupStep` pure `publish → nil` event.

- `execution.go:32` and `execution.go:490` `defer ReleaseExecutionLease(ctx, deploymentID)` **unconditional** — lease can be stolen after 5m and deleter wipes successor's lease (no `ReleaseExecutionLeaseIfOwner:342` used) — same fencing bug carried to resume path (`reverification-06 LF-07`). No heartbeat renewal.

**Status:** **PARTIAL**

- `recreate` : **FIXED** (honest provision, no traffic concern)
- `rolling / blue-green / canary / drain / scale / cleanup` : **STILL BROKEN / FALSE_COMPLETION** for traffic-weight and replica-count dimensions. AP-02 P0 narrowed but **not closed** for ZD paths.

**Evidence:**
- `forge/api/internal/services/deployment/execution.go:249` `RuntimeExecutor` iface doc and nil-guard
- `forge/api/internal/services/deployment/execution.go:264` honest gate
- `forge/api/internal/services/deployment/execution.go:277` promote stub
- `forge/api/internal/services/deployment/execution.go:299,306,313,320,327` drain/scale/cleanup stubs
- `forge/api/internal/services/deployment/execution.go:338` `verifyObservedRunning` boolean mismatch
- `forge/api/internal/services/deployment/rollout.go:261` `canaryPct` stored but DAG ignores
- `forge/api/internal/services/deployment/execution.go:32,490` unconditional `ReleaseExecutionLease` (fence violation)

---

### AP-03 — Per-app start/stop lie, restart was `TriggerDeploy` (`handlers_apphosting.go:450,475,500`) — `BROKEN P1`

**Prior:** `POST /apps/:id/start:450` and `stop:475` only wrote `applications.desired_state` via `appSvc.UpdateApp{DesiredState:"running"/"stopped"}` with no actuation, no reconciler, immediate `{ok:true}` while `observed_status` stayed `idle`; restart at `handlers_apphosting.go:500` was `TriggerDeploy{recreate,Image:""}` → empty-image `validateImageRef@sha256` fail + spurious history row. (FINAL_PARITY §3 AP-03 ; `final-parity/subagent-02` row 06 LF-03)

**LIVE:**

- `forge/api/internal/http/handlers_apphosting.go:450-472` `POST /apps/:id/start` still `running:="running"` → `appSvc.UpdateApp(ctx, id, orgID, UpdateAppRequest{DesiredState:&running})` (`handlers_apphosting.go:468`) → `200 {ok:true}`. No `daemon` call, no `UpdateApplicationStatus:379`, no `operation` queue. `forge/api/internal/store/store_apphosting.go:313` `UpdateApplication` only mutates `desired_state`. No watcher exists — `replicamanager/service.go:732` `reconcile` only watches `ReplicaApplications` not `applications`. Frontend `GET /apps/:id` badge stale `idle` (`reverification/subagent-06` C03/C11).

- `handlers_apphosting.go:475-498` `POST /apps/:id/stop` symmetric `stopped:="stopped"` same lie.

- `handlers_apphosting.go:500-537` `POST /apps/:id/restart` **FIXED**: now `ServerID==nil → 409`, `ServerControlTarget:527` → `Daemon.SendPower(powerCtx NodeURL NodeToken ServerID "restart") 60s:533` → `502 node restart failed` on error else `200 {ok:true serverId action:restart}`. Comment `517-520` cites prior `F-12` failure. No longer a deployment.

- `handlers_apphosting.go:444-446` + `handlers_apphosting.go:1080` `instanceLifecycleHandler` per-instance `POST .../instances/:instanceId/{start,stop,restart}` remains **PARITY**: validates `GetServiceStatus` + `GetInstance:1102` `instance.AppID==appID` guard + `GetNode/DaemonCredential` → `AdminContainer{Start,Stop,Restart}:1139-1143` + `UpdateInstanceStatus` + audit. Minor cross-check gap: `Instance.AppID` is `ReplicaAppID` vs `Application.ID` alias theoretical but mitigated by org guard.

- `handlers_compose.go:470` `POST /compose/:id/restart → RestartStack:781` and `handlers_user_console.go:231` `POST /user/compose/:id/restart` now exist (admin gap closed), but `forge/web/lib/api/compose.ts:34` still has **no** `restartComposeStack` export — `compose.ts:89` `deploy`, `93` `stop`, `98` `start` only — web unwired.

**Status:** **PARTIAL**

- `start` / `stop` (per-app) : **STILL BROKEN** (desired-state-only lie — `FALSE_COMPLETION`)
- `restart` (per-app) : **FIXED** (power channel honest)
- `instance restart/start/stop` : **FIXED / PARITY** (already)
- `compose restart web` : **UNWIRED at Web** (API wired, `compose.ts` missing export)

**Evidence:**
- `forge/api/internal/http/handlers_apphosting.go:450,468,475,493` start/stop lie
- `forge/api/internal/http/handlers_apphosting.go:500,533` restart fix (SendPower)
- `forge/api/internal/store/store_apphosting.go:313` `UpdateApplication` dynamic SET only desired
- `forge/api/internal/store/store_apphosting.go:379` `UpdateApplicationStatus` exists but no reconciler caller
- `forge/api/internal/services/replicamanager/service.go:732` reconcile watches ReplicaApplications only

---

### AP-05 — Rollback race / version guards (`revisions.go:147`, `store_deployments.go:128`) — `BROKEN`

**Prior:** `RollbackToRevision` non-versioned `UpdateDeployment`, `CreateRevision` `GetLatest→+1` without TX duplicates `RevisionNumber` under concurrency; `appstore/service.go:158` cross-version ignore. (FINAL_PARITY AP-05 ; `final-parity/subagent-02` LF-05 ; `reverification/subagent-06` C05/C09)

**LIVE — identical:**

- `forge/api/internal/services/deployment/revisions.go:82-91` `CreateRevision`: `latest, err := GetLatestDeploymentRevision(ctx, deploymentID)` → `nextNum = latest.RevisionNumber+1` with **no TX, no `SELECT … FOR UPDATE`**. Two concurrent `executeInitStep:210` both read `5 → both write 6`. `UNIQUE(deployment_id, revision_number)` (`099_deployment_revisions.sql`) may violate without retry.

- `revisions.go:147-175` `RollbackToRevision`: `UpdateDeploymentStatusVersioned(ctx, id, sd.Version, "in_progress"):167` bumps `V→V+1`, then in-memory `deployment.Version` still `V` and `UpdateDeployment(ctx, toStoreDeployment(deployment)):174` is **non-versioned** `UPDATE deployments SET image=$.., version=version+1 WHERE id=$1` (`store/store_deployments.go:128`) — no `WHERE version=`, so second rollback clobbers first. `RollbackToPrevious:202` delegates to same race.

- `revisions.go:58` `configHash` truncated `hex[:12]` still collisionable, excludes `EnvVars/ports/resources`.

- `appstore/service.go:158` still `always upgrade to latest` ignoring `app_ignore_upgrade` / cross-version guard (not in this path but same finding family).

**Status:** **STILL BROKEN** (both ends race + fencing incomplete)

**Evidence:**
- `forge/api/internal/services/deployment/revisions.go:82,87,90,119` CreateRevision race
- `forge/api/internal/services/deployment/revisions.go:147,167,174` rollback mixed fencing
- `forge/api/internal/store/store_deployments.go:128` `UpdateDeployment` non-versioned vs `store_deployments.go:195` versioned variant unused here
- `forge/api/internal/services/deployment/revisions.go:58` 12-hex hash

---

### AP-06 — Delete cascade / orphan (`apphosting/service.go:175`) — `BROKEN`

**Prior:** `DeleteApplication` allows delete while `isActiveStatus` deployment in flight, no reservation cancel, no beacon sweep, orphans `instances`/`reservations`/`BeaconCommandLog`. (FINAL_PARITY AP-06 ; `final-parity/subagent-02` LF-04)

**LIVE — unchanged:**

- `forge/api/internal/services/apphosting/service.go:175-183` `DeleteApp` checks only `AppBelongsToOrg` then `Store.DeleteApplication(id)` — no `isActiveStatus:119` (`rollout.go:119` `pending|in_progress|provisioning|awaiting_health|promoting|rollback_pending|rolling_back`) guard, no `placement reservation CancelReservation`, no `replicamanager.DeleteApp:501` iteration (`StopInstance+CancelReservation+DeleteInstance`), no beacon `ComposeDelete`.

- `forge/api/internal/store/store_apphosting.go:374` `DELETE FROM applications WHERE id=$1` FK `ON DELETE CASCADE` for `app_services` remains, but `instances` indexed by `replica_app_id` + `placement_reservations` + `BeaconCommandLog` orphaned plus `deployment_history` dangling.

- `forge/api/internal/services/compose/lifecycle.go:570` `DeleteComposeStack` does `deleting→deleted`, calls `daemon.ComposeDelete:599` (`docker compose down -v`) conditionally `--remove-orphans` only if `?removeOrphans=true` default false (`compose.go:574`), and cancels reservation at `lifecycle.go:608` — but `apphosting DeleteApp` **never** calls this path, so app delete never tells beacon. Beacon containers remain; old subnet/VOL `removeOrphans=false` leaves orphans if caller omits flag.

- Composition of above: `in_progress` deployment continues after app row deleted; no idempotency.

**Status:** **STILL BROKEN** (fence + orphan sweep + beacon notification all missing)

**Evidence:**
- `forge/api/internal/services/apphosting/service.go:175` unconstrained delete
- `forge/api/internal/store/store_apphosting.go:374` raw `DELETE`
- `forge/api/internal/services/deployment/rollout.go:119` `isActiveStatus` guard exists but **not** checked by delete
- `forge/api/internal/services/replicamanager/service.go:501` `DeleteApp` correct iteration but never invoked from apphosting delete
- `forge/api/internal/services/compose/lifecycle.go:570,608,574` compose delete path not shared
- `beacon/internal/server/compose.go:550,574` `removeOrphans` opt-in default false

---

### AP-07 — Scale / replicas + autoscaler binding (`replicamanager/service.go:432` / `apphosting/service.go:398`) — `PARTIAL → FIXED admission`

**Prior:** `PATCH .../services/:serviceId` silent no-op when `ReplicaAppID==nil` — DB `replicas` mutated with zero placement. (FINAL_PARITY AP-07 `BROKEN P1` ; `final-parity/subagent-02` row 11)

**LIVE:**

- `forge/api/internal/services/apphosting/service.go:398-433` `ScaleService` now `if targetReplicas>0 && existing.ReplicaAppID==nil → "scale refused: service has no replica placement (ReplicaAppID is unset)"` (`service.go:432-433`). Documented as Phase-1 F-13 fix. `handlers_apphosting.go:416-439` `PATCH .../services/:serviceId` after `UpdateService` checks `if Replicas!=nil && ReplicaAppID!=nil && cfg.ReplicaManager!=nil → ReplicaManager.ScaleApp:434`. `replicamanager/service.go:361` `ScaleApp` correctly increments generation (`401 IncrementReplicaAppGeneration`), calls `scheduler.ScaleReplicas/PlaceReplicas` + per-shard `appLocks[64]` (`service.go:55` fnv hash) + reservation→TX→dispatch→confirm loop `deployReplicas:195`.

- **Remaining split-brain** (`service.go:440-452`): DB mutation order is `UpdateAppService(replicas):440` → `UpdateReplicaAppReplicas(replicas):446` where second failure returns `updated, fmt.Errorf("service scaled but replica app update failed (non-fatal)")` (`service.go:448`) — DB shows `replicas=5` while `replica_apps.replicas=3` or placement `0`. No TX coupling, no rollback. `handlers_apphosting.go:434` `ReplicaManager.ScaleApp` failure after DB similarly split.

- `deployment/rollout.go:92` TargetReplicas clobber compounds scale (every rollout resets to 1, so rollout of scaled service downscales).

- `autoscaler/service.go:251` `EvaluateServer` vs `332` `ResizeServer` is **vertical** (memory/CPU) not horizontal `ScaleApp` — HPA binding for `AppService` absent (`reverification/subagent-07` row 16 DIVERGED).

- Compose per-service `POST /compose/:id/scale` absent despite `create.go:86` `--scale SERVICE=NUM` (row 19 MISSING).

**Status:** **PARTIAL**

- Admission (`ReplicaAppID==nil` refusal) : **FIXED**
- Placement/scale fencing + TX : **STILL BROKEN** (split-brain)
- HPA binding : **STILL UNWIRED** (vertical-only scaler)
- Rollout clobber interaction : **STILL BROKEN** (cross-cut)

**Evidence:**
- `forge/api/internal/services/apphosting/service.go:432` admission guard (FIXED)
- `forge/api/internal/services/apphosting/service.go:440-450` split-brain order
- `forge/api/internal/services/replicamanager/service.go:361,401,55,195,446` ScaleApp generation + locks correct
- `forge/api/internal/services/deployment/rollout.go:111` `TargetReplicas=1` clobber
- `forge/api/internal/services/autoscaler/service.go:332` `ResizeServer` vertical only

---

### AP-08 — Health gates `localhost` vs `ps` (`healthgate.go:11`, `validateHealthGateTarget:71`, `resolveNodeHost:62`) — `BROKEN P0 → FIXED default`

**Prior:** `CheckHealth` probed `localhost` + `validateHealthGateTarget:71` clamped to loopback so node container unreachable; compose `WaitForHealthy` polled `localhost` too. (REF-APP-C08-FLF05 ; MASTER `REF-APP-C08-FLF05 P0` → `VERIFIED_FIXED node-derived health target`)

**LIVE:**

- `forge/api/internal/services/deployment/healthgate.go:11-30` `CheckHealth`: early `if HealthCheckPath==""||Port==0 → {Passed:true}` still silently disables gate (`healthgate.go:13`) — no warning. Otherwise `host = HealthCheckHost`; if empty (`healthgate.go:18`), `host = resolveNodeHost(ctx, ServerID):23` (not loopback). `resolveNodeHost:62-77` is now correct: `Store.ServerControlTarget → NodeURL → url.Parse → Hostname()` with 10s timeout (`healthgate.go:68`). Empty `Hostname` → `Passed:false "host unresolved"`. Then `target := "http://%s:%d%s"` (`healthgate.go:31`) + `http.Client 10s` + `CheckRedirect ErrUseLastResponse` (`healthgate.go:39`). So default path (`HealthCheckHost==""`) now probes node host, not API loopback.

- `forge/api/internal/services/deployment/rollout.go:71-89` `validateHealthGateTarget`: `port` range check, `path` must be local absolute (`/…`) not `//` nor abs URL, then `host = TrimSuffix(Lower(TrimSpace(host)),".")` ; if `""||"localhost"` → `nil` (allow default); else `net.ParseIP(host)` must be `IsLoopback()` else error `health checks may only target the local deployment gateway` (`rollout.go:87`). So explicitly passing node IP `10.0.0.5` via `HealthCheckHost` is **blocked** — caller must leave `HealthCheckHost==""` to get `resolveNodeHost` derived host. This clamp is intentional for default but prevents explicit host override by design.

- `forge/api/internal/services/compose/lifecycle.go:188` `WaitForHealthy` remains **ps semantics**: polls `daemon.ComposeStatus` `State==running && Status==up` (`lifecycle.go:213`) + `restartCount+1` guard (`lifecycle.go:213-223`) over 2m; not `docker inspect Health` native.

- `healthgate.go:79` `WaitForHealthGate` ticker `intervalMs` default 5000 threshold 3 consecutive successes over `TimeoutSeconds` default 300 — still `http 200-399` only (not 3xx clamp beyond redirect check).

**Status:** **PARTIAL (core localhost lie FIXED, edge polish remains)**

- Default node-derived health target : **FIXED**
- Explicit `HealthCheckHost` loopback clamp : **Fixed-by-design** (must leave empty to get node host; documented behavior)
- Silent `Passed:true` when `Path==""||Port==0` : **STILL BROKEN** (honesty gap)
- `ps` vs `HealthStatus` divergence : **DIVERGED by design** (acceptable, but document `HealthStatus` not consulted)

**Evidence:**
- `forge/api/internal/services/deployment/healthgate.go:11` silent-pass gate
- `forge/api/internal/services/deployment/healthgate.go:62,68,76` `resolveNodeHost` fix
- `forge/api/internal/services/deployment/healthgate.go:23` node-derived host
- `forge/api/internal/services/deployment/rollout.go:71` `validateHealthGateTarget` loopback clamp
- `forge/api/internal/services/compose/lifecycle.go:188` `WaitForHealthy` ps check

---

### AP-10 — Env interpolation + `env_file` (`compose/service.go:14` / `beacon/compose.go:226`) — `BROKEN P0`

**Prior:** `service.go:14` `$$ / ${VAR:-default}` ported, but `env_file/include` unsupported, `rawService` missing `env_file` field, secrets silently dropped (FINAL_PARITY AP-10 `BROKEN P0` ; `reverification/subagent-07` row 11 `P0 silent secret loss`)

**LIVE — identical:**

- Interpolation: `compose/service.go:352` `ExpandTemplate` shared with `appstore/service.go:240` and `compose/service.go:356` `interpolateEnv` handles `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?err}`, `$$` (`service.go:370-423` via `composeVarRe:346`) — **PARITY** correct.

- `env_file` intake: `compose/service.go:92` `rawService` defines only `Environment interface{}` plus `Include []rawInclude` (`service.go:89`) and `rawInclude.Path/EnvFile interface{}` (`service.go:131`), but `ParseComposeYAML` (`parser.go:197` via `loader.LoadWithContext Environment:map[string]string{}` empty) **never populates** them from YAML; beacon `.env` write `beacon/compose.go:380` writes only `EnvVars` map via `encodeComposeEnv:264`, ignoring host `env_file` paths. `services.db.env_file: ./secrets.env` silently ignored → deploy proceeds with empty vars where secrets expected, runtime failure, no validation error. `ValidateCompose:240` does not emit `error` when `env_file` present.

- `healthcheck`/`deploy.resources` still `warning-only` at API (`service.go:278-289`) ignored at beacon (`compose.go:77` no `healthcheck` gate).

**Status:** **STILL BROKEN** (`P0 — silent secret loss`; interpolation PARITY but `env_file` false-completeness)

**Evidence:**
- `forge/api/internal/services/compose/service.go:134` `rawService.Environment` parsed but no `EnvFile` intake into deployment
- `forge/api/internal/services/compose/service.go:89,131` `Include.EnvFile` defined but unused
- `forge/api/internal/services/compose/parser.go:197` `Environment:map[string]string{}` empty — `env_file` dropped (second site `parser.go:254`)
- `beacon/internal/server/compose.go:380` `.env` only from `EnvVars`
- `forge/api/internal/services/compose/service.go:240` `ValidateCompose` no `env_file` error
- `forge/api/internal/services/compose/service.go:352,356` interpolation PARITY (for contrast)

---

## 3. Cross-Cut Checks (prompt line list + reverification-06/07 synthesis)

| Prompt check | LIVE line | STATUS | Notes (reverif reconcil.) |
|---|---|---|---|
| `deployment/execution.go:249,249-311` stubs now honest provision | `execution.go:249` iface + `264` gate | **PARTIAL** | Recreate FIXED; blue-green/canary/rolling still false-complete on traffic & replica dimensions. `reverification/subagent-06` C02 same verdict narrowed. |
| `deployment/healthgate.go:62` `resolveNodeHost` | `healthgate.go:62` via `ServerControlTarget.NodeURL → Hostname` | **FIXED** | Reconciles `MASTER REF-APP-C08-FLF05 → VERIFIED_FIXED`; `reverification/subagent-06` C08 + `07` health rows DIVERGED(stronger) confirm. |
| `deployment/rollout.go:92` `applyRolloutRequest` | `rollout.go:92-114` `TargetReplicas=1` default | **STILL BROKEN** | `reverification/subagent-07` LF-02 High ; `07` row 04 BROKEN-LF-02 confirms identical. |
| `deployment/revisions.go:147` `RollbackToRevision` | `revisions.go:147,167,174` + `82` race | **STILL BROKEN** | `reverification/subagent-06` C05/C09 + `07` LF-04 P1 ; `final-parity/subagent-02` LF-05 all identical. |
| `replicamanager/service.go:432` scale admission (LIVE `apphosting/service.go:432`) | `apphosting/service.go:432` | **FIXED (admission)** | Reconciles `MASTER REF-APP-C07 → VERIFIED_FIXED`; `reverification/subagent-06` C07 + `07` row 01 PARITY confirm admission fix. |
| `apphosting/service.go:175` delete unconstrained | `apphosting/service.go:175` + `store/store_apphosting.go:374` | **STILL BROKEN** | `reverification/subagent-06` C06 + `final-parity/subagent-02` LF-04 unanimous. |
| `compose/service.go:594` vs `beacon/compose.go:226` mounts | `compose/service.go:594` allows `/data` ; `beacon/compose.go:226` rejects any `/` | **STILL BROKEN — bifurcation** | `MASTER REF-P6-COMP-01` ; `reverification/subagent-07` LF-05 + `final-parity/subagent-02` LF-06 identical. `ValidateHostMountWithAllowlist:663` still unplumbed. |
| AP-02 for blue-green/canary still false? | `execution.go:277` promote DB-only | **YES — STILL FALSE for ZD** | `reverification/subagent-06` rows 04-05 `UNWIRED / FALSE_COMPLETION (traffic)` . |
| Health default fixed? | `healthgate.go:23` default derived | **YES — DEFAULT FIXED** | Explicit-host clamp retained by design (must leave empty). |
| Scale admission fixed? | `apphosting/service.go:432` refusal | **YES — ADMISSION FIXED** | Split-brain & HPA binding remain (reverif-07 rows 03,16). |
| Delete still unconstrained? | `apphosting/service.go:175` | **YES — STILL UNCONSTRAINED** | No guard, no sweep, orphans leaked. |
| `env_file` (AP-10) | `parser.go:197` empty map | **STILL BROKEN P0** | All reverifs agree. |
| Lease fence (`execution.go:32`) | unconditional `ReleaseExecutionLease` | **STILL BROKEN** | `reverification/subagent-06` row 15 BROKEN lease fence LF-07 ; `ReleaseExecutionLeaseIfOwner:342` exists but unused. |
| Privileged port `["80"]` bypass (`beacon/compose.go:213`) | `shortFormHostPort:214` len-1 → `""` | **STILL BROKEN** | `reverification/subagent-07` LF-03 High ; not in prompt but cross-cut. |
| Volume privileged-port + env interpolation remainder | `compose/service.go:352` vs `compose.go:213` | **Interpolation FIXED, port/allowlist NOT** | `reverification/subagent-07` rows 07,10,15. |
| Compose restart wiring | `handlers_compose.go:470` + `beacon/compose.go:506` exists, web missing | **WIRED at API, UNWIRED at Web** | `reverification/subagent-06` row 18 same. |
| Autoscaler binding | `autoscaler/service.go:332` vertical only | **STILL UNWIRED** | `reverification/subagent-07` row 16 DIVERGED. |
| `hash+env diff dedup` | `lifecycle.go:492` `computeHash/sha256 + mapsEqual` | **FIXED** | `reverification/subagent-06` row 09 PARITY (row-leak closed `handlers_compose.go:413` now `UpdateComposeStack`). Minor user surface still `DeployComposeStack` fresh row. |

---

## 4. Summary Counts (this subagent, 8 findings + cross-cuts)

| Status | Count | IDs |
|---|---|---|
| **FIXED** | 2 (narrow) | `AP-08 default health` (node-derived via `resolveNodeHost:62`), `AP-07 admission` (`scale refused` at `apphosting/service.go:432`) — both also `MASTER VERIFIED_FIXED` |
| **PARTIAL** | 2 | `AP-02` (recreate honest, ZD still false-complete), `AP-03` (restart fixed to power, start/stop still lie; instance lifecycle parity, web compose restart unwired) |
| **STILL BROKEN** | 4 | `AP-06 delete` (fence+orphan+sweep), `AP-05 rollback` (CreateRevision race + mixed version fencing), `AP-10 env_file` (P0 silent loss), mounts bifurcation `compose/service.go:594` vs `beacon/compose.go:226` (200→400 + allowlist not plumbed); plus cross-cut `rollout.go:92` clobber, `execution.go:32` lease fence, `beacon/compose.go:213` `["80"]` bypass |

**Carried cross-cut consensus (reverification-06/07 unanimous, unchanged in live):**
- `env_file` **P0** silent loss — fail-fast `ValidateCompose:240` must emit `Severity:"error"` when any `services.*.env_file`/`include` present until beacon resolves it.
- `volume bifurcation` **P0/P1** — unify predicate: add `AllowedMounts+IsAdmin` to `beacon/compose.go:290` `composeDeployRequest` and enforce `ValidateHostMountWithAllowlist:663` in beacon `validateComposeVolumes:226`; remove blanket `/` and blanket `type:bind` bans or gate via allowlist; promote `/root` warning→error unless admin+allowlist.
- `TargetReplicas clobber` **High** — in `rollout.go:92` derive from `store.ListAppServices` when `req.TargetReplicas<=0` or require explicit replicas for scaled apps.
- `revision race` **P1** — `CreateRevision:82` in `SERIALIZABLE` TX `SELECT … FOR UPDATE` on `deployments` + retry on `UNIQUE(deployment_id,revision_number)` ; `RollbackToRevision:147` single version-gated `UPDATE … WHERE version=$N`.
- `scale split-brain` **High** — `ScaleService:440` call `UpdateReplicaAppReplicas` **before** `UpdateAppService` or wrap both in TX with rollback on `ScaleApp` failure.
- `lease fence` **P1** — use `ReleaseExecutionLeaseIfOwner` not unconditional `ReleaseExecutionLease` + add heartbeat renewal for long steps (30s `handleStepFailure` + 5m default).
- `start/stop` **P0** — add reconciler like `replicamanager/reconcile:732` or route through `daemon.AdminContainerStart/Stop`.

---

## 5. File:Line Index (key evidence — all verified)

- `forge/api/internal/services/deployment/execution.go:22` `ExecuteDeployment` lease-claimed DAG
- `forge/api/internal/services/deployment/execution.go:32,490` unconditional `ReleaseExecutionLease` (vs `store/store_deployments.go:342` `ReleaseExecutionLeaseIfOwner` unused)
- `forge/api/internal/services/deployment/execution.go:210` `executeInitStep` snapshots `ImageRef` only
- `forge/api/internal/services/deployment/execution.go:249` `RuntimeExecutor` honest-failure gate (F-01 fix)
- `forge/api/internal/services/deployment/execution.go:264,277,299,306,313,320,327,338` provision/promote/drain/scale/cleanup + `verifyObservedRunning` boolean mismatch
- `forge/api/internal/services/deployment/healthgate.go:11,13,23,62,79` CheckHealth gate + `resolveNodeHost` fix + `WaitForHealthGate`
- `forge/api/internal/services/deployment/rollout.go:71,92,111,119,129,153,181,233,261` health target clamp + `applyRolloutRequest` clobber + `isActiveStatus` guard
- `forge/api/internal/services/deployment/revisions.go:58,82,87,147,167,174` configHash + CreateRevision race + Rollback mixed fencing
- `forge/api/internal/services/deployment/steps.go:48,57,64,75` DAG per strategy (canaryPercent ignored)
- `forge/api/internal/store/store_deployments.go:128,195,300,342` `UpdateDeployment` non-versioned vs versioned + lease claim
- `forge/api/internal/services/apphosting/service.go:81,93,175,374,398,432,440,681,734` source types + DeleteApp unconstrained + ScaleService admission gate + split-brain order
- `forge/api/internal/http/handlers_apphosting.go:138,173,416,444,450,475,500,533,1080` `/apps` global leak, instance lifecycle parity, start/stop lie, restart fix via `SendPower`
- `forge/api/internal/services/compose/service.go:134,240,352,356,516,594,663,679` interpolation parity + `env_file` drop + `restart:always` warning + `checkVolumesSecurity` + `ValidateHostMountWithAllowlist` unplumbed
- `forge/api/internal/services/compose/parser.go:197,254` `Environment:map[string]string{}` empty (env_file lost, two sites)
- `forge/api/internal/services/compose/lifecycle.go:188,249,476,570,781,988` `WaitForHealthy` ps semantics + deploy + Update + Delete + Restart + hash
- `beacon/internal/server/compose.go:77,181,213,226,264,344,506,550,574` policy checks, `shortFormHostPort` len-1 bypass, `validateComposeVolumes` absolute reject, deploy/start/stop/restart/delete/status
- `forge/api/internal/services/replicamanager/service.go:55,100,143,361,446,501,732` placement, locks, `ScaleApp`, `DeleteApp` sweep, reconcile
- Cross-refs: `audits/final-parity/subagent-02-app-platform.md` rows 02-05, 06 LF-03, LF-04, LF-05, LF-06 ; `audits/reverification/subagent-06-app-lifecycle.md` C02-C07, C08, rows 02-15 ; `audits/reverification/subagent-07-app-scaling-health.md` LF-01..LF-05 rows 01-16 ; `audits/MASTER_FINDING_INDEX.md` `REF-APP-C02..C08`, `REF-P6-COMP-01/02/03`

---

*Generated by subagent 03/10 (110-02 phase-02 parallel). No product files modified. Evidence: SOURCE_VERIFIED file:line unless marked.*
