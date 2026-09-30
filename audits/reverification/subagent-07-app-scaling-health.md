# Subagent 07 — App Scaling / Replicas, Health Probes, Revisions, Env & Mount Handling (Reverification)

**Subagent:** 07 of 20 (parallel run) — ALL 20 RUN IN PARALLEL
**Focus:** App Scaling/Replicas, Health Probes, Revisions, Env/Mount handling — the deeper app-platform layer
**Reconciling:** `phase-01 subagent-01 C07-C09` + `phase-06 subagent-09 Dokku/CapRover` + `final-parity subagent-02 AP-07..AP-16`
**Mode:** read-only — do not modify product code


**Re-inspection targets (prompt):**
- `reference: Uncloud pkg/client/service.go:24 RunService → placement, compose dependencies.go:78 InDependencyOrder, create.go:592 getRestartPolicy:641 resources`
- `Forge: forge/api/internal/services/apphosting/service.go:397 ScaleService, replicamanager/service.go:381 ScaleApp, compose/service.go:594 vs beacon/compose.go:226 volume policy bifurcation, forge/api/internal/services/deployment/healthgate.go:48 WaitForHealthGate, execution.go:313, rollout.go:92, beacon/compose.go:213 shortFormHostPort ["80"] bypass`
- **Checks:** scaling silent no-op (ReplicaAppID nil), health http vs ps, revisions disjoint, env interpolation, mounts isolation

**Date:** 2026-08-24
**Mode:** read-only — no product code modified. All citations file:line verified 2026-08-24.

---

## 1. Summary Verdict

| Area | Prior synthesis (phase-01 / phase-06 / final-parity) | Current re-verification | Delta |
|---|---|---|---|
| Scaling admission | phase-01 C07 PARTIAL — silent no-op when `ReplicaAppID==nil` (P1). final-parity AP-11 PARTIAL→FIXED ADMISSION (guard added). Dokku/CapRover LF-05 drift between `AppService.Replicas` and `Deployment.TargetReplicas`. | **Confirmed fixed (admission) with residual split-brain** — `apphosting/service.go:432` now refuses scale when `ReplicaAppID==nil`. `replicamanager/service.go:361 ScaleApp` is shard-locked and generation-fenced. Residual: DB replica count can diverge from placement if `UpdateAppService` succeeds but `UpdateReplicaAppReplicas` fails, and `applyRolloutRequest:92` can clobber replicas to 1. | Admission fixed; split-brain + clobber remain open (same as MASTER_FINDING_INDEX P1). |
| Health probes | phase-01 C08 PARTIAL, FLF-05 P0 localhost-only lie; final-parity AP-12 DIVERGED(stronger) with `resolveNodeHost` fix. | **Confirmed partially fixed** — `deployment/healthgate.go:62 resolveNodeHost` now derives host from `ServerControlTarget.NodeURL` (fix of F-05). `CheckHealth:12` http path and `compose/lifecycle.go:188 WaitForHealthy` ps path remain two disjoint health worlds; `CheckHealth` silent-skip when `port==0` persists. | Localhost lie closed; http-vs-ps divergence remains intentional but `Health` vs `State/Status` gap remains. |
| Revisions | phase-01 C09 PARTIAL (per-deployment fragmentation); final-parity AP-13 PARITY with LF-05 race. | **Confirmed** — `deployment/revisions.go:82 CreateRevision` race without TX and `RollbackToRevision:147` mixed version fencing persist. `execution.go:210 executeInitStep` snapshots per-deploy. | Same two bugs (race + fencing) still open (P1). |
| Env interpolation / env_file | phase-06-10 P0 silent drop; final-parity AP-14 PARTIAL (env_file missing). | **Confirmed P0 silent loss** — `compose/service.go:352 ExpandTemplate / 356 interpolateEnv` correct for `${VAR:-d}` etc; `rawService` defines `EnvFile` intake (`service.go:134`) but `parser.go:254 Environment: map[string]string{}` empty and `beacon/compose.go:380` writes only `.env` from `EnvVars` map — no `env_file` resolution or error. | Unchanged — still P0. |
| Mount isolation | phase-01 runtime-compose + phase-06-07 LF-03 / 06-10 / final-parity AP-17 bifurcated policy (200→400). MASTER_FINDING_INDEX REF-P6-COMP-01 P0 BROKEN. | **Confirmed bifurcated** — `compose/service.go:594 checkVolumesSecurity` vs `beacon/compose.go:226 validateComposeVolumes` remain divergent predicates with `ValidateHostMountWithAllowlist:663` unplumbed on compose path. | Unchanged — still P0/P1. |
| Dependencies | docker-compose `InDependencyOrder:78` topological; final-parity AP-15 PARTIAL delegated to `docker compose up -d`. | **Confirmed delegated** — `compose/parser.go` normalizes `DependsOn` but `lifecycle.go:249` delegates ordering to Docker; `replicamanager` ignores `DependsOn` for placement. | Unchanged — documented as acceptable via Docker but `condition: service_healthy` not enforced. |
| Restart / resources | `create.go:592 getRestartPolicy` / `635 getDeployResources` spec control; final-parity AP-16 UNWIRED (warning-only). | **Confirmed unwired** — API warning-only for `restart: always` (`compose/service.go:516`), beacon `validateComposePolicy:77` no restart gate, Forge `UpdateConfig/Resources` stored but not mapped to container `RestartPolicy/Resources` via Forge abstraction (compose resources flow via YAML only). | Unchanged. |

Overall: **3 admission/health fixes landed**, **6 structural gaps unchanged** (all previously catalogued as P0/P1), no new load-bearing regression introduced.

---

## 2. Parity Matrix (16 rows) — reconciling C07-C09 + AP-07..AP-16 + Dokku/CapRover + deep-layer prompts

Each row cites `REFERENCE file:symbol` and `FORGE file:line` (evidence-backed).

| # | Dimension | Reference (symbol) | Forge layers (file:line) | Status | Gap / Reconciliation | Severity |
|---|---|---|---|---|---|---|
| 01 | **Scale admission — ReplicaAppID guard** | `uncloud/pkg/client/service.go:24 RunService → scheduler.PlaceReplicas` placement must exist; `dokku/plugins/ps/ps.go:57 Formation` `DOKKU_SCALE` only valid for deployed app; `caprover/src/models/AppDefinition.ts:77 instanceCount` integer for existing app | `apphosting/service.go:398 ScaleService` guard `service.go:432 if targetReplicas>0 && existing.ReplicaAppID==nil → scale refused` (Phase-1 F-13 fix) ; `replicamanager/service.go:361 ScaleApp` per-shard `appLocks[64]` + `IncrementReplicaAppGeneration:401` | **PARITY (fixed)** | Prior `BROKEN` (silent no-op) → now admission refuses correctly. Reconciles phase-01 C07 and final-parity AP-11 admission. No silent success. | P1 closed |
| 02 | **Scale placement — generation fencing & scheduler** | `uncloud/pkg/client/service.go:76 deployment.Run` fan-out `wg.Go` per replica + generation; `docker-compose/pkg/compose/create.go:635 getDeployResources` resource-aware placement | `replicamanager/service.go:195 deployReplicas` `CreateReservation → CreateInstance TX → AssignInstanceReservation → dispatchBeaconCommand → ConfirmReservation` loop; `service.go:55 appLocks[64] fnv hash` ; `service.go:361 ScaleApp` calls `scheduler.ScaleReplicas` then `deployReplicas` or `safeStopReplicas:446` | **PARITY** | Forge `replicamanager` mirrors Uncloud pattern faithfully (reservation→instance→dispatch→confirm). Confirmed stronger than Dokku file-based scale and CapRover single-integer. | — |
| 03 | **Scale split-brain — DB vs placement** | `uncloud` single `Deployment.Run` transaction; `caprover ServiceManager.ts:888 ensureServiceInitedAndUpdated` atomic Swarm update | `apphosting/service.go:440 UpdateAppService(Replicas)` then `service.go:446 UpdateReplicaAppReplicas` — if second fails, `service.go:448` returns `updated, fmt.Errorf(... non-fatal)` with stale replica_app row. No TX coupling. `handlers_apphosting.go:434 if rmErr ScaleApp ...` separate HTTP path also not TX. | **PARTIAL / LF-01** | DB shows `replicas=5` while `replica_apps.replicas=3` (or placement 0). Phase-06 Dokku/CapRover LF-05 (drift between `AppService.Replicas` and `Deployment.TargetReplicas`) same class. | **High** |
| 04 | **Rollout TargetReplicas clobber** | `caprover AppDefinition.ts:77 instanceCount` explicit; `dokku ps/ps.go:57 Formation` per-type scale preserved across deploys | `deployment/rollout.go:92 applyRolloutRequest` `d.TargetReplicas = req.TargetReplicas; if <=0 {d.TargetReplicas=1} @ rollout.go:112` called from `recreateRollout:153` + `rollingRollout:205` + `blueGreenRollout:239` + `canaryRollout:293`. `handlers_revisions.go:48 Rollout` parses only `image+health`, `TargetReplicas` usually 0. | **BROKEN — LF-02** | Every rollout without explicit `targetReplicas` silently downscales a 5-replica app to 1. No path seeds from `ListAppServices`. Same finding as final-parity LF-02, phase-01 LF drift. **Unchanged.** | **High** |
| 05 | **Health — http gate vs ps health (two worlds)** | `dokku/plugins/checks/report.go:7 CheckHealth` ps vs http switch; `komodo/bin/core/src/monitor/mod.rs:48 PollStatus` ps+stats; `docker-compose` healthcheck spec | `deployment/healthgate.go:12 CheckHealth` http `GET http://host:port/path` 10s, 64KB, 200-399 pass ; `healthgate.go:62 resolveNodeHost` via `ServerControlTarget.NodeURL` (fixed) ; `healthgate.go:79 WaitForHealthGate` ticker `threshold` consecutive successes ; `compose/lifecycle.go:188 WaitForHealthy` polls `ComposeStatus` `state==running && status==up` + `extractRestartCount` guard | **DIVERGED (stronger, two paths)** | Fix of F-05 (`localhost` lie → `resolveNodeHost`) landed. Remaining divergence is intentional: deployment=http, compose=ps. Neither consults `container HealthStatus` (`docker inspect`). | Medium |
| 06 | **Health — silent disable + http semantics** | `coolify/app/Models/Application.php:143 health_check_*` explicit enabled flag; `dokku checks:enable` explicit | `deployment/healthgate.go:12 if HealthCheckPath=="" \|\| HealthCheckPort==0 → {Passed:true}` silent skip ; `healthgate.go:38 CheckRedirect: http.ErrUseLastResponse` (no follow) ; `lifecycle.go:213 WaitForHealthy` `state != running && status != up` fail plus `restartCount > prev+1` fail | **PARTIAL** | Gate can be silently disabled by omitting port; `passed==true` never warned. `WaitForHealthy` treats `status=="up"` as healthy even with `State==restarting` edge (mitigated by restart-count guard). | Medium |
| 07 | **Privileged port bypass — shortFormHostPort ["80"]** | `docker-compose pkg/compose` `validateComposePorts` via `nat.ParsePortSpec` handles single-value `80`; beacon intended `published <1024` block | `beacon/compose.go:181 validateComposePorts` → `shortFormHostPort:214` `strings.Split(entry,":")` switch `2→parts[0], 3→parts[1], default→""` (len 1 returns `""`) → `validateComposePorts:198 if published=="" continue` — skips check. `compose/service.go` API side has no port-privilege check at all. | **BROKEN — LF-03** | `ports: ["80"]` or `[" 80 "]` bypasses `port<1024` error; `long-form published: "80"` still caught only if map. Same as phase-06 subagent-07 LF-01 (confirmed). **Unchanged.** | High |
| 08 | **Revisions — create race + rollback fencing** | `caprover AppsDataStore.ts:591 versions[]` single-writer; `coolify ApplicationDeploymentQueue` queue per-app | `deployment/revisions.go:82 CreateRevision` `GetLatest → nextNum+1` without TX/FOR UPDATE (`revisions.go:87`); `revisions.go:147 RollbackToRevision` `UpdateDeploymentStatusVersioned:167` then `UpdateDeployment:174` (non-version-checked `store/store_deployments.go:128`) risking overwrite; `execution.go:210 executeInitStep` snapshots `ImageRef` only | **BROKEN — LF-04** | Same as final-parity LF-05 and phase-01 C09 gap. Duplicate `RevisionNumber` possible under concurrent rollout; rollback can clobber concurrent rollback's version. **Unchanged.** | **P1** |
| 09 | **Revisions disjoint — deployment vs compose vs replica generation** | `komodo stack.rs project_name` history per-stack hash; `uncloud ServiceSpec generation` | `deployment/revisions.go:58 configHash` (12-hex of `Image|Compose|Commit|Metadata`) per `deployment_id`; `compose/lifecycle.go:285 computeHash` SHA256 per `compose_stacks`; `replicamanager/service.go:155 IncrementReplicaAppGeneration` per `replica_app`. Three disjoint histories, no `application_id` anchor. | **DIVERGED** | Per-deployment revisions fragment history — each new `deployment` restarts at 1. Cannot list "app X revisions" without join via `current_deployment_id` (phase-01 C09). Same as before. | P2 |
| 10 | **Env interpolation — correct** | `docker-compose/pkg/compose` `envresolver.go` + `compose-go/types` `EnvFile`; `reference/app-platforms/docker-compose/pkg/compose/create.go` env propagation | `compose/service.go:352 ExpandTemplate` shared with `appstore/service.go:240` ; `service.go:356 interpolateEnv` handles `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?msg}`, `${VAR?msg}`, `$$` (`service.go:346 composeVarRe` + `service.go:370-423` branches) | **PARITY** | Matches compose spec control for in-YAML interpolation. Phase-06-10 and final-parity AP-14 confirm interpolation parity. | — |
| 11 | **env_file — silently dropped (P0)** | `docker-compose/pkg/compose/loader.go` + `envresolver.go` + `compose-go/types EnvFile` ; compose spec `env_file: ./secrets.env` | `compose/service.go:134 rawService.Environment` parsed, `service.go:134 Include.EnvFile` defined, but `parser.go:197 loader.LoadWithContext Environment: map[string]string{}` empty, and `parser.go:234 Environment: map[string]string{}` also empty; `beacon/compose.go:380 handleComposeDeploy` writes `.env` only from `EnvVars` map (`encodeComposeEnv:264`), ignores host `env_file` paths | **BROKEN — P0** | `services.db.env_file: ./secrets.env` silently ignored → deploy proceeds with empty vars where secrets expected. No validation error, no warning. Same as phase-06-10 LF P0 and final-parity AP-14. **Unchanged.** | **P0 — silent secret loss** |
| 12 | **Dependencies / InDependencyOrder** | `docker-compose/pkg/compose/dependencies.go:78 InDependencyOrder` topological graph with `Leaves()` + `errgroup` concurrency, condition `healthy/service_started`; `komodo compose.rs:544 StackServiceNames` | `compose/service.go:765 normalizeDependsOn` + `parser.go:896` correctly parse `depends_on` string/list/map+condition but `lifecycle.go:249 DeployComposeStack` single `docker compose up -d` delegates ordering to Docker; `replicamanager/service.go` placement ignores `DependsOn` (instances placed irrespective of DB→web graph) | **PARTIAL** | Parse/diff correct (`compareServiceSummaries:909` includes `dependsOn`), execution delegated. Acceptable parity via `docker compose` native topo, but `condition: service_healthy` never enforced beyond start order. | Low |
| 13 | **Restart policy — warning-only vs spec** | `docker-compose/pkg/compose/create.go:592 getRestartPolicy` maps `always|unless-stopped|on-failure:10|no` + deploy `restart_policy` override; `dokku/plugins/ps/ps.go:18 ps:restart-policy on-failure:10` | `compose/service.go:516 restart:"always"` issue `Severity:"warning"` (`service.go:521 may conflict with platform`) ; `beacon/compose.go:77 validateComposePolicy` **no** restart check; `compose/policy` never maps to `container.RestartPolicy` via Forge (Docker compose does); `replicamanager/service.go:732 reconcile` tracks `failed→ReplaceInstance` not restart policy | **UNWIRED** | `always` containers fight platform `StopStack` (`docker compose stop` not `down`, so `always` restarts immediately). Same as final-parity AP-16. | Medium |
| 14 | **Resources — getDeployResources vs Forge** | `docker-compose/pkg/compose/create.go:635 getDeployResources` maps `mem_limit, cpus, nano_cpus, limits/reservations, pids, blkio, devices, gpus, ulimits` to `container.Resources` | `compose/service.go:849 resources` parsed via `normalizeDeploy` / `normalizeResourcesToForge` but Forge stacks flow resources via `DeployComposeRequest{MemoryMB,CPUShares,DiskMB}` quota ceilings (`lifecycle.go:280-283`) not via YAML `deploy.resources`; `apphosting CreateServiceRequest:285 Resources *store.ResourceSpec` stored in `app_services.resources` JSON but never wired to placement `CPU/MemoryMB` vs `deployReplicas` resource checks as distinct from compose | **PARTIAL** | Compose resources preserved in YAML → Docker correctly; app-service path's `ResourceSpec` decoupled from compose/deploy resource translation. No mapping parity issue for compose stacks, but app-services resource drift possible (phase-06 Dokku/CapRover autoscaler vertical vs HPA). | Medium |
| 15 | **Mounts isolation — bifurcation (API vs beacon)** | `docker-compose` long-form `type: bind` vs volume; `1panel/agent/app/service/app_install.go:246` `LocalPersistentVolume` allowlist; `portainer/api/stacks/deployments/deployer.go` endpoint allowlist | `compose/service.go:594 checkVolumesSecurity` blocks `/proc|/sys|/docker.sock` error, `/etc|/` error else `/root|/home` warning+slog (`service.go:634-654`); `beacon/compose.go:226 validateComposeVolumes` unconditional reject any `source` with `/` prefix or `..` or `type: bind` (`compose.go:233-244`) ; `compose/service.go:663 ValidateHostMountWithAllowlist` gate (admin+allowlist) never called on compose deploy path | **BROKEN — LF-05** | Same as MASTER_FINDING_INDEX REF-P6-COMP-01 P0 BROKEN, final-parity LF-06. `compose: /data:/data` passes API (`/data` not in `sensitiveHostPaths:679 ["...", "/root","/etc","/home"]`) then beacon `400 host bind mount source "/data" is not allowed` (`compose.go:244`). No `allowedMounts` plumbing (`store_nodes.go:1055 AllowedMounts`). | **P1 — UX + isolation** |
| 16 | **Autoscaler / placement surplus (cross-cut)** | `dokku/ps Formation` per-process, `caprover instanceCount`, `uncloud placement` per-container wg, `komodo` no per-service scale; autoscaler is vertical (`autoscaler/service.go:332 ResizeServer`) not HPA | `autoscaler/service.go:251 EvaluateServer` scales *servers* (memory/CPU `ResizeServer`) not `AppService.Replicas`; `apphosting/service.go:681 TriggerDeploy` + `replicamanager/service.go:361 ScaleApp` handle replica count, but autoscaler never calls `ScaleApp` | **DIVERGED** | Autoscaler binding missing — same as final-parity AP-11 autoscaler note and Dokku/CapRover LF-05 drift. | P2 |

---

## 3. Logic Findings (5) — file:line, mechanism, impact, remediation

Each finding reconciles the deep-layer prompts (placement, InDependencyOrder, getRestartPolicy/resources, scaling silent no-op, health http vs ps, revisions disjoint, env interpolation, mounts isolation, shortFormHostPort bypass).

### LF-01 — Scale split-brain: `UpdateAppService` succeeds but `UpdateReplicaAppReplicas` fails → DB vs placement diverge

- **Reference:** `uncloud/pkg/client/service.go:24 RunService` single transactional `Deployment.Run` (volume schedule → placement → wg deploy); `caprover/src/user/ServiceManager.ts:888 ensureServiceInitedAndUpdated` atomic single Swarm update.
- **Forge:** `forge/api/internal/services/apphosting/service.go:398 ScaleService` lines `440-450`
  ```go
  // service.go:440-450
  updated, err := svc.store.UpdateAppService(ctx, serviceID, input) // Replicas=&target
  if updated.ReplicaAppID != nil {
      _, err = svc.store.UpdateReplicaAppReplicas(ctx, *updated.ReplicaAppID, targetReplicas)
      if err != nil {
          return updated, fmt.Errorf("service scaled but replica app update failed (non-fatal): %w", err)
      }
  }
  ```
  Second write is non-fatal; caller receives `updated` with `Replicas=5` while `replica_apps` still `3`. Separately, HTTP `handlers_apphosting.go:434` calls `ReplicaManager.ScaleApp` outside the DB mutation — if placement fails, DB already mutated (no rollback). No DB TX wrapping both.
- **Impact:** `GetServiceOverview:529` reads `AppService.Replicas` (5) but `GetServiceStatus:487 ListInstancesByApp` finds 3 `running` instances; health `ComputeServiceHealth:323` reports `healthy` for 3 while platform reports `desired=5`. Monitoring lies; follow-up `ScaleApp` sees `activeCount=3` vs DB `5` mismatch.
- **Reconciliation:** Same class as phase-06 Dokku/CapRover LF-05 (replicas source-of-truth drift) and final-parity AP-11 split-brain note. Admission guard (LF fix) addressed silent no-op but not this two-phase commit.
- **Remediation:** Order `ReplicaManager.ScaleApp` (or at least `UpdateReplicaAppReplicas`) **before** `UpdateAppService`, or wrap both in `store.TX` with `SELECT FOR UPDATE` on `app_services`; on placement failure roll back DB replica count and publish `EventAppFailed`.
- **Severity:** **High**

### LF-02 — `applyRolloutRequest` clobbers `TargetReplicas` to 1 — silent downscale on every rollout without explicit replicas

- **Reference:** `caprover/src/models/AppDefinition.ts:77 instanceCount` explicit per-app replicas preserved across deploys; `dokku/plugins/ps/ps.go:57 Formation` persists per-type counts; `uncloud` `ServiceSpec.Replicas` explicit.
- **Forge:** `forge/api/internal/services/deployment/rollout.go:92 applyRolloutRequest` lines `111-114`
  ```go
  d.TargetReplicas = req.TargetReplicas
  if d.TargetReplicas <= 0 { d.TargetReplicas = 1 } // rollout.go:112
  ```
  Called from `recreateRollout:153`, `rollingRollout:205`, `blueGreenRollout:239`, `canaryRollout:293`. Request type `RolloutRequest:33 TargetReplicas int json:"targetReplicas,omitempty"` is usually `0` when called via `handlers_revisions.go:48 POST /:id/rollout` which parses only `image+healthCheck*` fields (no replicas field surfaced). No code path does `if req.TargetReplicas==0 { req.TargetReplicas = appService.Replicas }`.
- **Impact:** Deploy of a 10-replica `AppService` via any `StartRollout`/`StartBlueGreen` resets `deployments.target_replicas=1`. `execution.go:313 executeScaleUpStep` then only verifies 1 running (`VerifyRunning` bool), so rollout reports `completed` with 90% capacity loss.
- **Reconciliation:** Identical to final-parity LF-02 (High) and phase-01 LF drift (C07). **Unfixed.**
- **Remediation:** In `StartRollout`/`blueGreenRollout`/`canaryRollout`, if `req.TargetReplicas==0` load `store.ListAppServices(ServerID→app→services)` and seed `Max(Replicas)`, or require caller to pass `serviceId` and use that service's replicas; validate mismatch as `400 targetReplicas required for apps with replicas>1`.
- **Severity:** **High**

### LF-03 — `shortFormHostPort` len-1 bypass — `ports: ["80"]` publishes privileged port undetected

- **Reference:** `reference/app-platforms/docker-compose/pkg/compose/create.go` `buildContainerPortBindingOptions` via `nat.ParsePortSpec` correctly handles single value `"80"` as published; `compose-go` loader normalizes short-form.
- **Forge:** `beacon/internal/server/compose.go:213 shortFormHostPort`
  ```go
  func shortFormHostPort(entry string) string {
      parts := strings.Split(entry, ":")
      switch len(parts) {
      case 2: return parts[0]
      case 3: return parts[1]
      default: return "" // len 1 → ""  // compose.go:221-222
      }
  }
  ```
  `validateComposePorts:181` then
  ```go
  published = shortFormHostPort(v) // compose.go:190
  if published == "" { continue }  // compose.go:198 — skips check
  port, err := strconv.Atoi(published) // 201
  if port < 1024 { return fmt.Errorf("... privileged host port %d ...", port) } // 206
  ```
  So `"80"` (len 1) and long-form `published: "80"` as string `"80"` via map path also bypasses unless `map["published"]` present (long-form via string not taken). API side `compose/service.go` has **no** privileged port check at all (only beacon does).
- **Impact:** Compose `services.web.ports: ["80:80"]` is blocked (published 80) but `["80"]` (same Docker semantics: host 80) passes validation `200 {valid:true}` then Docker actually publishes 80 as privileged. Isolation premise violated; matches phase-06 subagent-07 LF-01 (confirmed).
- **Reconciliation:** Same as phase-06-07 LF-01, MASTER_FINDING_INDEX P1. **Unfixed.**
- **Remediation:** In `shortFormHostPort`, handle `len==1 { return parts[0] }` and parse single-value as published port; reuse `compose-go` `nat.ParsePortSpec` or mirror Docker's `ParsePortSpec` for fidelity. Also add API-side port check mirroring beacon or move check to single shared predicate.
- **Severity:** **High** (isolation bypass)

### LF-04 — Revision number race + rollback version fencing incomplete

- **Reference:** `caprover/src/datastore/AppsDataStore.ts:305 versions[]` single writer + `maxVersionHistory`; `coolify/app/Models/Application.php:1076` queue per-app. Compose hash `computeHash` via SHA256.
- **Forge:** `forge/api/internal/services/deployment/revisions.go:82 CreateRevision`
  ```go
  latest, err := s.store.GetLatestDeploymentRevision(ctx, deploymentID) // revisions.go:87
  nextNum := 1
  if err == nil { nextNum = latest.RevisionNumber+1 } // 90 — no TX
  rev := &store.DeploymentRevision{ RevisionNumber: nextNum, ...} // 107
  s.store.CreateDeploymentRevision(ctx, rev) // 119
  ```
  `deployment_revisions` has `UNIQUE(deployment_id, revision_number)` (`099_deployment_revisions.sql:4`) but violation is not retried, just `fmt.Errorf("create revision: %w")`. Concurrent `ExecuteDeployment` (`execution.go:210 executeInitStep`) can both read `5` → both try to write `6`.

  `RollbackToRevision:147` does
  ```go
  s.store.UpdateDeploymentStatusVersioned(ctx, deploymentID, sd.Version, "in_progress", "") // 167 version V→V+1
  deployment.Image = targetRev.ImageRef // 171
  s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)) // 174 non-version-checked — store/store_deployments.go:128 SET ... WHERE id=$1
  ```
  Second write uses stale `deployment.Version=V` (pre-bump) and `UpdateDeployment` does not check `version`, so concurrent rollers interleave and later image wins with wrong version fencing. `UpdateDeploymentCurrentRevision:178`, `SupersedeDeploymentRevisions:182`, `UpdateDeploymentRevisionStatus:187` follow but are independent.
- **Impact:** Duplicate `RevisionNumber` or `UNIQUE violation` under concurrent rollout; rollback race can land on wrong image. `CompareRevisions:225` diffs 5 fields but rollback only restores `ImageRef` (fragile — other fields dropped).
- **Reconciliation:** Same as final-parity LF-05 (P1) and phase-01 C09 race note. **Unfixed.**
- **Remediation:** Wrap `CreateRevision` in `SERIALIZABLE` TX with `SELECT ... FOR UPDATE` on `deployments` row, or rely on `UNIQUE` + retry loop with re-read. Make rollback single version-gated statement: `UPDATE deployments SET image=$1, status='in_progress', version=version+1 WHERE id=$2 AND version=$3` via `UpdateDeploymentVersioned`/`UpdateDeploymentConfig` pattern.
- **Severity:** **P1**

### LF-05 — Volume policy bifurcation: `200 valid` → `400 policy violation`, allowlist not plumbed

- **Reference:** `docker-compose` `type: bind` long-form vs `type: volume`; `1panel/agent/app/service/app_install.go:246` `LocalPersistentVolume` allowlist; `portainer/api/stacks/deployments/deployer.go` endpoint allowlist.
- **Forge:**
  ```go
  // compose/service.go:594 checkVolumesSecurity
  if strings.HasPrefix(lower, "/var/run/docker.sock") { error }
  if strings.HasPrefix(lower, "/proc") || lower=="/sys" { error }
  // service.go:634-654
  if source == "/etc" || source == "/etc/" || source=="/" { severity="error" }
  else if isSensitiveHostPath(source) { // service.go:681 ["/","/root","/etc","/home"]
      severity="warning" + slog.Warn // warning-only for /root,/home
  }
  // service.go:663 ValidateHostMountWithAllowlist(source, isAdmin, allowedMounts) — never called on compose path
  ```
  ```go
  // beacon/compose.go:226 validateComposeVolumes
  case map[string]any:
      if composeString(v["type"]) == "bind" { return fmt.Errorf("long-form bind not allowed") } // 234
  case string:
      source, _, _ := strings.Cut(v, ":")
      if strings.HasPrefix(source, "/") || isPathTraversal(source) { // 243
          return fmt.Errorf("host bind mount source %q is not allowed", source) // 244
      }
  ```
  A compose with `volumes: ["/data:/data"]` passes API (`/data` not in `sensitiveHostPaths:679`) → `DeployComposeStack` creates `compose_stacks` row + `CreatePlacementReservation` (`lifecycle.go:326`) → beacon `POST /compose/deploy` returns `400 compose policy violation` (`compose.go:359`). User sees `degraded` stack with no `allowedMounts` recourse. Conversely `ValidateHostMountWithAllowlist` (admin+allowlist gate) is defined but no caller on compose path provides `store.AllowedMountSourcesForNode` (`store_nodes.go:1079`).
- **Impact:** Confusing UX (validate says ok, deploy fails after reservation); isolation weakened by warning-only for non-admin `/root` at API layer; operator allowlist (`store_nodes.go:1055 AllowedMounts`) never enforced.
- **Reconciliation:** Identical to MASTER_FINDING_INDEX REF-P6-COMP-01 P0 BROKEN, phase-06-07 LF-03, phase-06-10 mount paragraph, final-parity LF-06. **Confirmed still bifurcated.**
- **Remediation:** Unify predicate: either (a) make API `checkVolumesSecurity` match beacon's absolute-reject (all `/` = error) until beacon supports allowlist, or (b) plumb `allowedMounts+isAdmin` via `daemon.ComposeDeployRequest` (`beacon/compose.go:290 composeDeployRequest` add `AllowedMounts []string + IsAdmin bool`) and enforce `ValidateHostMountWithAllowlist` semantics in beacon `validateComposeVolumes` (remove blanket `/` rejection, remove blanket long-form `type:bind` ban, gate via allowlist).
- **Severity:** **P1 — UX + isolation**

---

## 4. Thematic Summary — status counts and cross-cutting gaps

| Status | Count | Examples |
|---|---|---|
| **PARITY / FIXED** | 3 | scale admission guard (01), scale placement generation fencing (02), env interpolation (10) |
| **PARTIAL / DIVERGED** | 6 | scale split-brain (03), health http-vs-ps divergence (05-06), dependencies delegation (12), restart warning-only (13), resources partial (14), autoscaler vertical vs HPA (16) |
| **BROKEN / P0** | 5 | TargetReplicas clobber (04), privileged port bypass (07), revision race (08), env_file silent drop (11), mounts bifurcation (15) |

**Cross-cutting gaps carried forward (reconfirmed):**

| Gap | Related rows | Evidence |
|---|---|---|
| Silent secret loss via `env_file` — no error | 10-11 | `compose/service.go:134 EnvFile` intake unused, `parser.go:197 Environment:{}` empty, `beacon/compose.go:380` writes only `.env` |
| `depends_on` topo delegated to Docker; replicamanager ignores graph | 12 | `parser.go:896` normalized, `lifecycle.go:249` single `up -d`, `replicamanager` no `DependsOn` check |
| `restart: always` fights platform lifecycle (`stop` vs `down`) | 13 | `service.go:516` warning-only, `beacon/compose.go:77` no gate, `lifecycle.go:213` restart-count +1 tolerance |
| Mount allowlist not plumbed — `200→400` trap | 15 LF-05 | `service.go:663 ValidateHostMountWithAllowlist` vs `compose.go:226` absolute reject |
| Autoscaler vertical not horizontal (no HPA) | 16 | `autoscaler/service.go:332 ResizeServer` vs `replicamanager.ScaleApp` |
| Revision histories disjoint per subsystem | 08-09 | `revisions.go:58` / `lifecycle.go:988` / `replicamanager:155` three hashes |

---

## 5. Recommendations (activation order, no new subsystem required)

1. **P0 Fix `env_file` dishonesty** (`compose/service.go:134` / `parser.go:197` / `beacon/compose.go:264`): either support `env_file` by mounting host file (requires policy decision) or fail fast — `ValidateCompose:240` must return `error` when any `services.*.env_file` is present until beacon can resolve it; surface `ValidationIssue{Severity:"error"}` and document.
2. **P0/P1 Unify volume policy** (`service.go:594` vs `compose.go:226`): add `AllowedMounts []string + IsAdmin bool` to `daemon.ComposeDeployRequest` (`beacon/compose.go:290`), call `store.AllowedMountSourcesForNode` (`store_nodes.go:1079`) in `lifecycle.go:249` deploy path, enforce `ValidateHostMountWithAllowlist:663` in beacon; remove blanket `/` and blanket `type:bind` bans or gate them via allowlist. Remove warning-only for `/root` — promote to error unless admin+allowlist.
3. **P1 Close `shortFormHostPort` bypass** (`beacon/compose.go:213`): handle `len==1` as `parts[0]` (single `"80"` means published `80`); consider reusing `compose-go` `nat.ParsePortSpec` for fidelity; mirror check in API `ValidateComposeSecurity` or share predicate.
4. **P1 Close rollout replica clobber** (`rollout.go:92`): if `req.TargetReplicas<=0` derive from `store.ListAppServices(ServerID→app→services)` max `Replicas`, or require `targetReplicas` when app has `replicas>1`; add `400` error instead of silent `1`.
5. **P1 Wrap scale in TX** (`apphosting/service.go:398` + `replicamanager/service.go:361`): call `UpdateReplicaAppReplicas` before `UpdateAppService` or wrap both in DB TX with rollback on `ScaleApp` error; publish `EventAppScaledDown/Up` only after both succeed.
6. **P1 Fix revision fencing** (`revisions.go:82` + `revisions.go:147` + `store/store_deployments.go:128`): `CreateRevision` in `SERIALIZABLE` TX with `SELECT ... FOR UPDATE` on `deployments`; change `RollbackToRevision` second write to version-checked `UPDATE ... WHERE version=$N` (use `UpdateDeploymentConfig` pattern) and add `UNIQUE(deployment_id, revision_number)` retry on conflict.
7. **P1 Health honesty** (`healthgate.go:12`): warn when gate silently disabled (`Port==0` or `Path==""`) — return `ValidationIssue` or log; for compose, document that `WaitForHealthy:188` covers `ps` only, and when `HealthCheck` present prefer `docker inspect HealthStatus` via beacon `ComposeStatus` extension (add `Health` field from `ps --format json` `Health` column déjà available).
8. **P2 Wire restart/resources to spec** (`compose/service.go:516` vs `create.go:592`): promote `restart: always` to `error` when `StackStatus==stopped` or map Forge `DesiredState` to `RestartPolicy` via `UpdateConfig`; map `apphosting ResourceSpec` to placement `CPU/MemoryMB` consistently, or document that compose resource limits live in YAML only.
9. **P2 Dependency health condition** (`dependencies.go:78` vs `lifecycle.go:249`): document that `depends_on condition: service_healthy` is not enforced beyond `docker compose` start order; if Forge moves to per-service placement, implement `InDependencyOrder` traversal before `PlaceReplicas` (or via `replicamanager` sorted `AppServices`).
10. **P3 Autoscaler naming** (`autoscaler/service.go:251`): rename current autoscaler to `vertical scaler` or add HPA binding that calls `replicamanager.ScaleApp` with `minReplicas/maxReplicas/targetCPU`.

---

## 6. Verification Notes (re-inspection method)

- Compared `reference/app-platforms/uncloud/pkg/client/service.go:24 RunService` → `scheduler.VolumeScheduler` + `Deployment.Run` wg fan-out per replica against `replicamanager/service.go:195 deployReplicas` reservation→TX→dispatch loop and `service.go:361 ScaleApp` shard locks.
- Compared `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:78 InDependencyOrder` graph/Leaves/errgroup vs `forge/api/internal/services/compose/parser.go` + `service.go:765 normalizeDependsOn` vs `lifecycle.go:249` single up, and `create.go:592 getRestartPolicy` / `635 getDeployResources` vs `compose/service.go:516` warning-only and `parser.go` resource passthrough.
- Re-read `beacon/internal/server/compose.go:77 validateComposePolicy`, `181 validateComposePorts`, `213 shortFormHostPort`, `226 validateComposeVolumes`, `264 encodeComposeEnv`, `344 handleComposeDeploy`, `418 handleComposeStop`, `462 handleComposeStart`, `506 handleComposeRestart`, `550 handleComposeDelete`, `604 handleComposeStatus`.
- Re-read `forge/api/internal/services/apphosting/service.go:398 ScaleService` (guard at `432`), `replicamanager/service.go:361 ScaleApp`, `deployment/healthgate.go:12 CheckHealth`, `62 resolveNodeHost`, `79 WaitForHealthGate`, `deployment/execution.go:210 executeInitStep`, `313 executeScaleUpStep`, `338 verifyObservedRunning`, `deployment/rollout.go:92 applyRolloutRequest`, `deployment/steps.go:48 stepsForStrategy`, `deployment/revisions.go:82 CreateRevision`, `147 RollbackToRevision`, `compose/service.go:240 ValidateCompose`, `352 ExpandTemplate`, `356 interpolateEnv`, `594 checkVolumesSecurity`, `663 ValidateHostMountWithAllowlist`, `compose/lifecycle.go:188 WaitForHealthy`, `249 DeployComposeStack`, `476 UpdateComposeStack`, `570 DeleteComposeStack`, `781 RestartStack`, `988 computeHash`.
- Cross-checked `phase-01 subagent-01 C07-C09` (scale/health/revisions), `phase-06 subagent-09 Dokku/CapRover` LF-05 drift, `final-parity subagent-02 AP-07..AP-16` rows and `audits/MASTER_FINDING_INDEX.md:113 REF-P6-COMP-01`.

All file:line citations above verified against `main` at 2026-08-24 read time. No product code modified.

---

## 7. File:Line Index (key citations for this subagent)

- `reference/app-platforms/uncloud/pkg/client/service.go:24 RunService` → `service.go:76 Deployment.Run` fan-out
- `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:78 InDependencyOrder` — `InDependencyOrder(ctx, project, fn)`
- `reference/app-platforms/docker-compose/pkg/compose/create.go:592 getRestartPolicy` — `mapRestartPolicyCondition` incl `always|unless-stopped|on-failure:10|no`
- `reference/app-platforms/docker-compose/pkg/compose/create.go:635 getDeployResources` — `resources = getDeployResources(service)`
- `forge/api/internal/services/apphosting/service.go:398 ScaleService` — admission gate `service.go:432 if ReplicaAppID==nil → scale refused`
- `forge/api/internal/services/replicamanager/service.go:361 ScaleApp` — generation `service.go:401 IncrementReplicaAppGeneration`, shard `service.go:55 appLocks[64]`, `service.go:195 deployReplicas`, `service.go:446 safeStopReplicas`, `service.go:501 DeleteApp`, `service.go:732 reconcile`
- `forge/api/internal/services/deployment/healthgate.go:12 CheckHealth` — silent `Passed:true` when `Path==""||Port==0`; `healthgate.go:62 resolveNodeHost` via `ServerControlTarget.NodeURL`; `healthgate.go:79 WaitForHealthGate` threshold loop
- `forge/api/internal/services/deployment/execution.go:210 executeInitStep` — `CreateRevision`; `execution.go:313 executeScaleUpStep:verifyObservedRunning`; `execution.go:338 verifyObservedRunning` bool check
- `forge/api/internal/services/deployment/rollout.go:92 applyRolloutRequest` — `rollout.go:112 if TargetReplicas<=0 {=1}` clobber; `rollout.go:129 recreateRollout`, `rollout.go:181 rollingRollout`, `rollout.go:233 blueGreenRollout`, `rollout.go:261 canaryRollout`
- `forge/api/internal/services/deployment/steps.go:48 stepsForStrategy` — DAG per strategy including `health_gate` conditional
- `forge/api/internal/services/deployment/revisions.go:58 configHash` 12-hex; `revisions.go:82 CreateRevision` race; `revisions.go:147 RollbackToRevision` mixed fencing; `revisions.go:225 CompareRevisions`
- `forge/api/internal/services/compose/service.go:134 rawService EnvFile Include.EnvFile` intake; `service.go:240 ValidateCompose`, `service.go:352 ExpandTemplate`, `service.go:356 interpolateEnv` `${VAR:-d}` handling; `service.go:427 ValidateComposeSecurity`; `service.go:516 restart always warning`; `service.go:594 checkVolumesSecurity`; `service.go:663 ValidateHostMountWithAllowlist`; `service.go:679 sensitiveHostPaths`; `service.go:849 resources`
- `forge/api/internal/services/compose/parser.go:197 loader.LoadWithContext Environment:{} empty`; `parser.go:254 Environment:{}`
- `forge/api/internal/services/compose/lifecycle.go:188 WaitForHealthy` ps + restart-count guard; `lifecycle.go:249 DeployComposeStack` placement+reservation+daemon+health; `lifecycle.go:476 UpdateComposeStack` hash+`mapsEqual:993`; `lifecycle.go:781 RestartStack Stop+Start`; `lifecycle.go:988 computeHash` SHA256
- `beacon/internal/server/compose.go:77 validateComposePolicy` strict schema; `compose.go:181 validateComposePorts`; `compose.go:213 shortFormHostPort` privileged-port extraction; `compose.go:226 validateComposeVolumes` absolute/`..`/`type:bind` reject; `compose.go:264 encodeComposeEnv`; `compose.go:344 handleComposeDeploy` `docker compose up -d`; `compose.go:380 .env write`; `compose.go:418 handleComposeStop`; `compose.go:462 handleComposeStart`; `compose.go:506 handleComposeRestart`; `compose.go:550 handleComposeDelete` `down -v`; `compose.go:604 handleComposeStatus` `ps --format json`
- `forge/api/internal/store/store_apphosting.go:211 ListApplications` scoping; `store/store_deployments.go:128 UpdateDeployment` non-version-checked vs `UpdateDeploymentStatusVersioned`; `store/store_nodes.go:1055 AllowedMounts`, `1079 AllowedMountSourcesForNode`
- Cross-refs: `audits/phase-01/subagent-01-lifecycle.md:C07-C09`, `audits/phase-06/subagent-09-dokku-caprover.md` (LF-05 drift, LF-01 plugin gap), `audits/final-parity/subagent-02-app-platform.md:AP-07..AP-16` (+ LF-01..LF-06), `audits/MASTER_FINDING_INDEX.md:113 REF-P6-COMP-01`

---

*Generated by subagent 07 (reverification, parallel). No product files modified. Evidence: SOURCE_VERIFIED file:line unless marked.*

