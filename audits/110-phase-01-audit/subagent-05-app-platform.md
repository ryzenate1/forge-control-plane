# Subagent 05 — App Platform Preparation Audit (Phase 01, Agent 05/10)

**Scope:** `apphosting`, `deployment`, `compose`, `catalog`, `build`, `replicamanager`, `preview`/`previewenv` + handlers + web admin
**Date:** 2026-08-24
**Mode:** Read-only. No code modified. All claims file:line verified on current checkout.
**Predecessors reconciled:** `audits/FINAL_PARITY_AUDIT.md §3 AP-01..AP-18`, `audits/final-parity/subagent-02-app-platform.md`, `audits/implementation-plan/subagent-03-app-platform.md`, `audits/reverification/subagent-06-app-lifecycle.md`, `audits/reverification/subagent-07-app-scaling-health.md`, `audits/MASTER_FINDING_INDEX.md REF-APP-*`, GH findings (`FINAL_PARITY §2 GH-01..GH-19`) where they intersect orchestration.
**Task lines inspected:** `forge/api/internal/services/apphosting/service.go:92`, `deployment/service.go:268`, `execution.go:249`, `rollout.go:42`, `healthgate.go:11`, `revisions.go:82`, `replicamanager/service.go:100`, `compose/service.go:14`, `lifecycle.go:249`, `parser.go`, `catalog`, `appstore`, `preview/previewenv`, `build/service.go:28`, `forgefile/pipeline`, `handlers_apphosting.go:173,450`, `handlers_deployment.go`, `handlers_compose.go:413`, `handlers_revisions.go`, `forge/web app/admin/apps/deployments/compose`

---

## 0. Executive Summary

App Platform **model is sound, execution is half-honest**. Since Phase-01, 4 P1/P0 fixes landed honestly:

* `deployment/execution.go:268-274` provision now fails closed when `runtime==nil` (`provision_regression_test.go:13` enforces) — `recreate` (`FINAL_PARITY AP-02`) is now `PARITY`.
* `deployment/healthgate.go:23` + `healthgate.go:62` `resolveNodeHost` no longer probes `localhost` — default gate derives `NodeURL.hostname` via `store.ServerControlTarget`.
* `handlers_apphosting.go:500-537` `POST /apps/:id/restart` now routes via `daemon.SendPower(..."restart")` 60s, honest `502` on failure — previously `TriggerDeploy` empty image (`FINAL_PARITY AP-03` P1 fixed).
* `handlers_compose.go:413-438` `POST /compose/:id/deploy` now calls `UpdateComposeStack` in-place (was `DeployComposeStack` fresh `cps-` leak) — `AP-06` row-leak closed; `apphosting/service.go:432` scale admission now refuses `ReplicaAppID==nil`.

**Remaining P0/P1 load-bearing defects still require Phase-01 activation:**

1. **Stubs lie complete for zd/rolling/scale** — `execution.go:277` `executePromoteStep` flips `active_target` DB only; `execution.go:299`/`306`/`313`/`320` drain/scale only `verifyObservedRunning` boolean, no `PlacementExecutor`/`TrafficExecutor`, no replica-count verification (`MASTER_FINDING REF-APP-C02-FLF01 / FORGE-LOGIC-001` refined but not closed).
2. **Health gate vantage wrong** — `healthgate.go:37-42` still dials from API pod (not beacon-local loopback) and `rollout.go:71-89` `validateHealthGateTarget` loopback clamp blocks explicit gateway VIP use; `health_check_configs` disjoint from `deployments.health_gate_*`.
3. **Scale split-brain** — `apphosting/service.go:440-450` does `UpdateAppService` then `UpdateReplicaAppReplicas` + `handlers_apphosting.go:416-437` `ScaleApp` after DB write; placement failure leaves DB `replicas` != real placement. No HPA.
4. **Rollback race** — `deployment/revisions.go:82-124` `CreateRevision` non-tx `GetLatest+1`; `revisions.go:147-199` `RollbackToRevision` does versioned `UpdateDeploymentStatusVersioned:167` then **non-versioned** `UpdateDeployment:174` with stale `deployment.Version` (`REF-APP-C05-FLF04`, `FINAL_PARITY AP-05`, `AP-09`).
5. **Start/stop desired_state lie** — `handlers_apphosting.go:450`/`475` set `desired_state` only (`store_apphosting.go:313`) with no reconciler, no `UpdateApplicationStatus:379`, no beacon actuation (`REF-APP-C03-FLF02` half-fixed).
6. **Delete unconstrained** — `apphosting/service.go:175-183` `DeleteApp` unconstrained `DELETE` (`store_apphosting.go:374`) with no `isActiveStatus` guard, no `CancelReservation`, no `replicamanager.DeleteApp:501` sweep, no beacon `ComposeDelete` for app path (`REF-APP-C06`, `FINAL_PARITY AP-06`).
7. **Env / mounts dishonesty** — `compose/service.go:14`/`134` `env_file` parsed but `parser.go:197` `loader.LoadWithContext Environment:map{}` empty + `beacon/compose.go:380` only `.env` from `EnvVars` → `AP-10` P0 silent secret loss; `compose/service.go:594` vs `beacon/compose.go:226` volume predicates diverge 200-valid→400-violation (`REF-P6-COMP-01/02`).

No `GatewayRouter` migration overlap was found — App Platform does not own gateway tables. Gateway work (`GatewayRouter`, `caddy_proxy.go`, `traefik_proxy.go`) is required only for promote/traffic wiring via a new `TrafficExecutor` interface, not schema changes.

---

## 1. Inventory — Files Inspected (file:line source of truth)

### 1.1 Core services

| Area | File:line | Symbol / Current behavior |
|---|---|---|
| **apphosting create** | `forge/api/internal/services/apphosting/service.go:81` | `validSourceTypes {GIT,DOCKER_IMAGE,COMPOSE}` |
| | `service.go:92-123` | `CreateApp` — validates `SourceType` case-insensitive, defaults `""→DOCKER_IMAGE` (masks caller error), `ServerID` nullable, `SourceConfig` defaults `{}` |
| | `service.go:140-173` | `UpdateApp` — dynamic `UpdateApplicationInput`, `DesiredState` validated `running\|stopped\|removed` |
| | `service.go:175-183` | `DeleteApp` — **only** `AppBelongsToOrg` check then `store.DeleteApplication` unconstrained cross-ref `REF-APP-C06` |
| | `service.go:398-453` | `ScaleService` — **admission fixed** `432` `ReplicaAppID==nil → scale refused`, but `440` `UpdateAppService` before `446` `UpdateReplicaAppReplicas` split-brain |
| | `service.go:455-592` | `GetServiceStatus`/`GetServiceOverview`/`ComputeServiceHealth` — instance → `ServiceEndpoint` aggregation |
| | `service.go:681-754` | `TriggerDeploy` — requires `ServerID!=nil` else `409`, `resolveDeployImage` image-or-server-fallback, creates `StrategyRecreate` `pending` deployment |
| **deployment model** | `forge/api/internal/services/deployment/service.go:17-24` | `Strategy{blue-green,canary,rolling,recreate}`, `Status{pending→completed,failed,...}` |
| | `service.go:59-91` | `Deployment` struct — `BlueTargetID,GreenTargetID,ActiveTarget,HealthCheck*,TargetReplicas,Timeout*,Version` |
| | `service.go:113-120` | `Service{store,publisher,runtime,resumeMu,executingDeployments,wg}` — no `PlacementExecutor`/`TrafficExecutor` |
| | `service.go:276-326` | `StartBlueGreen` — creates `blue=server-blue`, `green=server-green-%d`, `ActiveTarget=blue`; `CompleteDeployment/Cancel` versioned |
| **execution** | `forge/api/internal/services/deployment/execution.go:22-35` | `ExecuteDeployment` — `ClaimExecutionLease:24` 5m + **unconditional** `defer ReleaseExecutionLease:32` (lease fence concern) |
| | `execution.go:249-262` | `RuntimeExecutor{ApplyDeployment,VerifyRunning}` — honest interface, docs cite `F-01` |
| | `execution.go:264-275` | `executeProvisionStep` — **fixed** `if runtime==nil→error` else `runtime.ApplyDeployment` |
| | `execution.go:277-297` | `executePromoteStep` — **P0 remaining** flips `active_target` via `UpdateDeploymentConfig` versioned, **no gateway call** (`FORGE-LOGIC-001` remainder) |
| | `execution.go:299-325` | `executeDrainOld/DrainCanary/ScaleUp/ScaleDown` — **stubs** only `verifyObservedRunning` boolean |
| | `execution.go:338-350` | `verifyObservedRunning` — `VerifyRunning(ctx,ServerID) bool`, fails if false |
| | `execution.go:389-490` | `ResumeDeployments`/`resumeFromStep` — lease + step restart, also unconditional release |
| | `execution.go:159-207` | `executeStep` DAG dispatch via `stepStatusMapping` |
| **rollout** | `forge/api/internal/services/deployment/rollout.go:42-68` | `StartRollout` dispatch `recreate\|rolling\|blueGreen\|canary` |
| | `rollout.go:71-90` | `validateHealthGateTarget` — allows `""\|localhost` else requires `IsLoopback()` → **rejects gateway VIP** `10.0.0.5` with `health checks may only target the local deployment gateway` `87` |
| | `rollout.go:92-117` | `applyRolloutRequest` — **LF-02** `d.TargetReplicas = req.TargetReplicas; if <=0 {=1}` clobbers `Replicas=5→1` silently |
| | `rollout.go:119-127` | `isActiveStatus` — `pending\|in_progress\|provisioning\|awaiting_health\|promoting\|rollback_pending\|rolling_back` |
| | `rollout.go:322-330` | `validateImageRef` `@sha256:` enforcement |
| **healthgate** | `forge/api/internal/services/deployment/healthgate.go:12-57` | `CheckHealth` — skips when `Path==""\|\|Port==0 → Passed:true`, else `host = HealthCheckHost \|\| resolveNodeHost`, `http://host:port/path` 10s client 200-399 pass, 64KB cap |
| | `healthgate.go:62-77` | `resolveNodeHost` — `ServerControlTarget → url.Parse NodeURL → Hostname()` **fix of localhost lie** |
| | `healthgate.go:79-126` | `WaitForHealthGate` — threshold `DefaultHealthGateThreshold:3`, interval `5000ms`, timeout `300s`, ticker consecutive-success |
| **revisions** | `forge/api/internal/services/deployment/revisions.go:58-62` | `configHash` `SHA256(json)[:12]` — **truncated** 12-hex, excludes env/ports |
| | `revisions.go:82-124` | `CreateRevision` — `GetLatest→nextNum+1` **no TX/SELECT FOR UPDATE** race `REF-APP-C05` |
| | `revisions.go:147-199` | `RollbackToRevision` — `UpdateDeploymentStatusVersioned` bumps `V→V+1` then **ungated** `UpdateDeployment` with stale `deployment.Version` → overwrites `V+2` |
| | `revisions.go:225-269` | `CompareRevisions` — diffs 5 fields correctly |
| **replicamanager** | `forge/api/internal/services/replicamanager/service.go:100-141` | `CreateApp` — generation-gated, `appLocks[64]` shard |
| | `service.go:143-193` | `DeployApp` — `PlaceReplicas` + `deployReplicas` reservation→instance→`BeaconCommandLog` |
| | `service.go:195-359` | `deployReplicas` — per-index `CreateReservation→CreateInstance(tx)→AssignReservation→dispatchBeaconCommand` |
| | `service.go:361-444` | `ScaleApp` — generation bump, `ScaleReplicas`/`deployReplicas`/`safeStopReplicas` |
| | `service.go:501-551` | `DeleteApp` — iterates instances `StopInstance+CancelReservation+DeleteInstance`, correct but **never called from `apphosting.DeleteApp`** |
| | `service.go:732-808` | `reconcile` — polls `ListReplicaApps` 1m, verifies `Beacon.VerifyInstance`, repairs `failed→ReplaceInstance`, updates `ReplicaApplications` status only (not `applications`) |
| **compose service** | `forge/api/internal/services/compose/service.go:14-15` | `MaxComposeYAMLBytes=1MB` |
| | `service.go:50-89` | `rawCompose/rawService` — no `env_file` field at top level (compose-go `types` has it, raw does not consume) |
| | `service.go:130-135` | `rawInclude{EnvFile}` — defined, never read |
| | `service.go:240-330` | `ValidateCompose` — parses, checks `image\|build`, warns `profiles/healthcheck/deploy`, runs `ValidateComposeSecurity` |
| | `service.go:352-425` | `interpolateEnv` — handles `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?err}`, `$$` correctly |
| | `service.go:427-528` | `ValidateComposeSecurity` — `privileged,network_mode host,pid host,cap_add dangerous→error`, `restart always→warning` |
| | `service.go:594-677` | `checkVolumesSecurity` — blocks `/var/run/docker.sock`, `/proc\|/sys→error`, `isSensitiveHostPath` `/, /etc, /root, /home` (only `/etc\|/→error`, else `warning+slog.Warn`) — **bifurcation source** |
| | `service.go:663-677` | `ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts)` — correct gate, never called on compose path |
| | `service.go:679-688` | `sensitiveHostPaths = {/,/root,/etc,/home}` |
| **compose lifecycle** | `forge/api/internal/services/compose/lifecycle.go:188-231` | `WaitForHealthy` — polls `daemon.ComposeStatus` every 5s, `State==running\|Status==up`, restart-count `+1` guard |
| | `lifecycle.go:249-434` | `DeployComposeStack` — `ValidateCompose` pre-check, quota `20/stack`, ceilings, scheduler `PlaceServer`, reservation, `CreateComposeStack cps-*`, `daemon.ComposeDeploy`, `awaiting_health→running\|degraded` 2m |
| | `lifecycle.go:476-568` | `UpdateComposeStack` — dedup `computeHash:988` + `mapsEqual(env)` `492-494`, captures `rollbackHash/YAML/Env:497`, `StatusUpdating→ComposeDeploy→WaitForHealthy→running\|degraded`, rollback on deploy error via `rollbackStack:844` |
| | `lifecycle.go:570-623` | `DeleteComposeStack` — `deleting→deleted`, `daemon.ComposeDelete` (default `removeOrphans=false`), `cancelReservation:608`, but beacon delete failure → `markFailed` retains `deleting` |
| | `lifecycle.go:697-786` | `StartStack/StopStack/RestartStack` — `RestartStack:781` is `StopStack→StartStack` sequential **no reservation hold**, non-atomic |
| **parser** | `forge/api/internal/services/compose/parser.go:245-265` | `ParseComposeYAML(yamlContent)` — `loader.LoadWithContext Environment:map{}` empty — **drops env_file/include** |
| | `parser.go:268-324` | `NormalizeToForge` — `types.Project→ForgeAppConfig` |
| | `parser.go:327-362` | `ValidateCompose` per-service `image\|build` check |
| **catalog/appstore** | `forge/api/internal/services/catalog/catalog.go:119` (via `main.go:1273`) | `Provision` host+port injection, isolated `store_catalog.go:14` |
| | `forge/api/internal/services/appstore/service.go:41-52` | `ListApps/GetApp` — 7 seeded entries |
| | `appstore/service.go:74-137` | `InstallApp` — `resolveTemplate:81` `ExpandTemplate` then `DeployComposeStack`, swallows interpolation error `239-244` `return tmpl` on fail → **stale compose deployed** `REF-P6-APP-01` |
| | `appstore/service.go:139-156` | `UninstallApp` — `DeleteComposeStack` warning but deletes DB row even on daemon error `REF-P6-APP-02` |
| | `appstore/service.go:158-202` | `UpgradeApp` — always upgrades to latest, ignores `app_ignore_upgrade` / cross-version guard `AP-05` |
| **preview dualism** | `forge/api/internal/services/preview/service.go:42-79` | `Create` — `IsIsolated=true`, `UniqueSuffix 8char`, `Deploy:125` synthesizes `https://preview-<suffix>.example.com` **without `proxy_domains` record** |
| | `previewenv/service.go:121-197` | `previewenv.Create` — `TTL 24h`, `MaxPerOrg 10→5`, per-PR dedup `ListActivePreviewDeployments`, `expires_at`, commit-status pending |
| | `http/phase4_registrar.go:37-40` | `previewenv.New(BaseDomain PREVIEW_DOMAIN, TTL 24h, MaxPerOrg 5)` — legacy `preview/service.go:820` still wired on admin namespace `AP-15` DUPLICATE |
| **build** | `forge/api/internal/services/build/service.go:28-31` | `BuilderDockerfile/Nixpacks` only — 2 builders vs Dokploy 6 |
| | `build/service.go:54-138` | `DockerfileBuildRequest{CacheFrom,CacheTo,Platform}` forwarded — was dropped now fixed |
| | `build/service.go:684-735` | Remote build via beacon `GitClone→LoginRegistry→DockerfileBuild/NixpacksBuild→BuildLogs→InspectDigest`, pollution risk `beacon/build.go:54-56` |
| **forgefile/pipeline** | `forge/api/internal/services/forgefile/service.go:44-97` | `Manifest{Deploy[],Database}` `Validate` max 256KiB, `Apply` materializes `apphosting.CreateApp/CreateService` per `deploy[]` |
| | `forge/api/internal/services/forgefile/service.go:374-392` | `persist` double-encodes `content` as JSON `REF-P6-PLUG-01` note |
| | `forge/api/internal/services/pipeline/service.go:40-114` | `queueLoop` poll 900ms concurrency 2 + `scheduleLoop` 1m cron `fireDueSchedules:194` — **no HTTP/UI** `AP-15` UNWIRED |
| **handlers** | `forge/api/internal/http/handlers_apphosting.go:173` | `POST /apps` — `SourceType` validation, `resolveDefaultOrg` `default` uuid fix |
| | `handlers_apphosting.go:450-472` | `POST /apps/:id/start` — **BROKEN** `UpdateApp{DesiredState:"running"}→{ok:true}` no beacon |
| | `handlers_apphosting.go:475-498` | `stop` symmetric lie |
| | `handlers_apphosting.go:500-537` | `restart` — **FIXED** `ServerControlTarget→SendPower restart 60s →502` |
| | `handlers_apphosting.go:416-439` | `PATCH .../services/:id` — `UpdateService` then `if ReplicaAppID!=nil → ScaleApp` (correct order in handler, inverted in service) |
| | `handlers_apphosting.go:444-446` | `POST .../instances/:id/{start,stop,restart}` → `instanceLifecycleHandler:1080` `AdminContainerStart/Stop/Restart` (`service.go:455` `GetServiceStatus` guard) |
| | `handlers_apphosting.go:1046-1077` | `GET /admin/git-branches` `git ls-remote --heads` on API host, not credential-aware |
| | `forge/api/internal/http/handlers_deployment.go:24-127` | `Group /admin/deployments` `blue-green/rollback/complete/cancel/execute/steps/resume` — adminIP + deployments.write/read |
| | `handlers_deployment.go:17-50` | `blue-green` only image+health, no `TargetReplicas` passthrough |
| | `forge/api/internal/http/handlers_revisions.go:16-32` | `GET /:id/revisions`, `POST /:id/revisions/:revId/rollback`, `POST /:id/rollback-previous` — **no 409 mapping for version conflict** |
| | `handlers_revisions.go:48-62` | `POST /:id/rollout` — parses `RolloutRequest{Image,Strategy,Health*,CanaryPercent,TargetReplicas}` → `StartRollout` |
| | `forge/api/internal/http/handlers_compose.go:83,413,470,483` | `POST /compose/webhook/:id` HMAC `X-Hub-Signature-256` + `POST /compose/:id/deploy` fixed to `UpdateComposeStack:427` + `POST /compose/:id/restart:470` newly wired → `RestartStack:781` but non-atomic |
| **web** | `forge/web/lib/api/compose.ts:34-111` | `validateCompose,createCompose,deploy,stop,start,getStatus,getLogs` — **no `restartComposeStack`** export (`AP-03` remainder) |
| | `forge/web/lib/api/apps.ts:255-265` | `startApp/stopApp/restartApp` — `restartApp` hits `/apps/:id/restart` (power) correctly |
| | `forge/web/app/admin/apps/page.tsx:16` | `AppType{image,git,compose}` IA, `AdminAppsShared.tsx:9` `DeployStatusBadge` but `compose/page.tsx` has own `statusConfig` (9 tones) drift |
| | `forge/web/app/admin/compose/page.tsx:44` | actions `stop/start/delete` only — no `restart` button though API now supports it |
| | `forge/web/app/admin/compose/[id]/page.tsx:28` | `useState(tab)` query `?service` but `deployment-progress.tsx:41` 2s poll vs `DeploymentTimeline:27` 5s duplicate pollers same `GET /admin/deployments/:id/steps` |
| | `forge/web/app/admin/deployments/*` | `deployments.ts:88` `fetchDeploymentSteps`, `98 fetchDeployment`, `103 fetchRevisions`, `113 rollbackToRevision` — correct but `compare?from&to` disjoint stores |

---

## 2. AP-01..AP-18 Parity Reconciliation

| # | AP | FINAL_PARITY | final-parity subagent-02 | Reverified (06) | Implementation-plan (03) | **Preparation verdict & evidence** |
|---|---|---|---|---|---|---|
| AP-01 | App create (image/git/compose) | COMPLETE `handlers_apphosting.go:173` | PARTIAL (3 types vs 5, no Tar) | PARTIAL — defaults `DOCKER_IMAGE` silently `service.go:100` | PARTIAL table 1.1 | **PARTIAL (honest)** keep 3 types — add exactly-one `SourceConfig` mutual exclusivity (`caprover ImageMaker.ts:416`); add server_id non-null constraint or eager `ValidateCompose` at create; document Tar intentional. |
| AP-02 | Deploy strategies (recreate/rolling/blue-green/canary) | FALSE_COMPLETION P0 `execution.go:249` stubs | 02 PARITY (recreate fixed) 03/04/05 PARTIAL | 02 PARITY, 03/04/05 PARTIAL/FALSE traffic `execution.go:277` | §1.1 still BROKEN except provision | **P0 split:** `recreate` FIXED (honest fail-closed `execution.go:269` + `provision_regression_test.go:13`); `rolling/blue-green/canary` still FALSE-COMPLETE — no placement/traffic, boolean verify, `canaryPercent` ignored `steps.go:48` `rollout.go:25,272` — Phase-01 workstream. |
| AP-03 | Per-app vs per-service start/stop/restart | BROKEN P1 `handlers_apphosting.go:450` desired_state lie | BROKEN→PARTIAL FIX (restart fixed) | PARTIAL (restart fixed, start/stop still BROKEN) | §1.2 BROKEN, restart FIXED | **BROKEN P1:** `restart` FIXED `handlers_apphosting.go:500`→`SendPower:533`; `start:450`+`stop:475` still lie `store_apphosting.go:313` no `UpdateApplicationStatus:379`, no `appreconciler`. Per-instance `handlers_apphosting.go:444` PARITY. Web `restartComposeStack` missing. |
| AP-04 | Hash+env diff dedup | COMPLETE `service.go:594` hash | PARITY `lifecycle.go:492` | FIXED (F-08 closed) but no `ValidateCompose` on `UpdateComposeStack` | PARTIAL note | **COMPLETE for compose; partial for app** `lifecycle.go:988` SHA256 + `mapsEqual:993` correct; `UpdateComposeStack` now live but without re-validation → privileged YAML can sit until beacon 400 after `updating`. |
| AP-05 | RollbackStack / version guards | BROKEN `appstore/service.go:158` always latest | BROKEN | STILL BROKEN race+scope `revisions.go:147` | §1.3 race | **BROKEN P1:** `appstore UpgradeApp` ignores `app_ignore_upgrade`/cross-version; `RollbackToRevision` non-atomic version (`revisions.go:167→174`). Separate findings, both P1. |
| AP-06 | Delete cascade / orphan | BROKEN `apphosting/service.go:175` unconstrained + `handlers_compose.go:413` leak | BROKEN `DeleteApplication` unconstrained | STILL BROKEN fence+orphan `service.go:175` | §1.4 BROKEN | **BROKEN P1:** no `isActiveStatus` gate `rollout.go:119`, no `CancelReservation`/`replicamanager.DeleteApp:501`, no beacon `down -v` for app path; `DeleteComposeStack:608` now cancels reservation but `apphosting DeleteApp` never calls. |
| AP-07 | Scale / replicas + autoscaler | BROKEN P1 silent no-op | PARTIAL→FIXED ADMISSION `ScaleService:432` | ADMISSION FIXED, split-brain remains | §1.5 split-brain | **BROKEN P1 remainder:** `ReplicaAppID==nil` admission fixed `service.go:432`; remaining DB-before-placement `service.go:440-446` vs handler `handlers_apphosting.go:433` divergence, `autoscaler ResizeServer` vertical only, no HPA binding, no `POST /compose/:id/scale`. |
| AP-08 | Health gates (http vs ps) | BROKEN P0 localhost + clamp | DIVERGED (stronger) `resolveNodeHost` | CORE FIX landed, clamp+dual-world remains `healthgate.go:62` | §1.6 default fixed | **BROKEN P2 remainder:** default path fixed `healthgate.go:23` via `NodeURL`; explicit `HealthCheckHost` still `rollout.go:87` loopback-only; probe is API→node not beacon-local; `health_check_configs:15` disjoint; `WaitForHealthy:188` ps not `docker HealthStatus`. |
| AP-09 | Revisions/history/compare | PARTIAL disjoint | PARITY with LF-05 race | PARTIAL race+fragment | §1.3 race | **PARTIAL:** `store_deployment_revisions.go:21` `UNIQUE(deployment, num)` + `execution.go:210` `executeInitStep` snapshots `ImageRef` correctly; but `CreateRevision:88` race, `configHash:58` 12-hex truncation, per-deployment fragmentation no app-level timeline. |
| AP-10 | Env interpolation + env_file | BROKEN P0 secrets dropped | PARTIAL (env_file missing) | BROKEN P0 `env_file/include` silently dropped `parser.go:197` | §1.7 dropped | **BROKEN P0:** `$$/${VAR:-d}` ported `compose/service.go:370-414`; `env_file/include` never consumed `parser.go:197` `Environment:map{}`, beacon `compose.go:380` only `EnvVars` → deploy empty vars where secrets expected. Must fail fast. |
| AP-11 | Service dependencies / convergence | PARTIAL parser strings | PARTIAL delegates to docker compose | PARTIAL | parsed not ordered | **PARTIAL (acceptable):** `parser.go:764` `normalizeDependsOn` correct, diffed `gitops.go:841`, runtime delegates to `docker compose up -d` topological; `replicamanager` app-services ordering not enforced. |
| AP-12 | Restart policies | PARTIAL warning | UNWIRED/WARNING ONLY | UNWIRED | warning only | **PARTIAL:** `service.go:517` `restart:always→warning`, beacon `compose.go:77` no restart check — `always` fights `StopStack` (compose `stop` not `down` so container restarts immediately). Promote to error when `StatusStopped`. |
| AP-13 | Mount isolation | PARTIAL two tiers | DIVERGED bifurcated LF-06 | DIVERGED `checkVolumesSecurity` vs `validateComposeVolumes:226` | §1.7 bifurc | **DIVERGED P1:** API allows `/data:/data` `service.go:634`, beacon rejects any absolute `source "/"` `compose.go:238` + `type:bind` ban — 200→400 after reservation. `ValidateHostMountWithAllowlist:663` never plumbed. |
| AP-14 | Catalog/AppStore pipeline | BROKEN swallows | DUPLICATE/UNWIRED | BROKEN `resolveTemplate:239` swallows | §1.7 stale compose | **BROKEN P1:** `appstore/service.go:239` returns `tmpl` on `ExpandTemplate` failure → stale compose deployed; `store_catalog.go:14` isolated ok but dual write paths diverge. |
| AP-15 | Preview envs (per-PR) | DUPLICATE | DUPLICATE | DUPLICATE | duplicate | **DUPLICATE HIGH:** `preview/service.go:42` (hardcoded `preview-<suffix>.example.com:125` no `proxy_domains` route) vs `previewenv/service.go:121` TTL+MaxPerOrg+commit-status at `phase4_registrar.go:37` not on admin namespace — TTL never expires in exposed impl. Winner: `previewenv`. |
| AP-16 | Provider registry honesty | DUPLICATE | DUPLICATE | DUPLICATE | dedup | **DUPLICATE P2:** Forge `capabilities.go:7` dead delta vs `nodeprobe/service.go:1` second probe path; not app-platform load-bearing but co-locate fix with Incus/LXC cleanup. |
| AP-17 | Multi-runtime dispatch | FALSE_COMPLETION phantom 7 vs 5 | FALSE_COMPLETION | FALSE beacon drops Provider `server.go:740→841` always docker | — | **FALSE_COMPLETION P0 (phantom):** control 7 names vs beacon 5, `beacon/server.go:740` drops `Provider` → always Docker — shared with Runtime audit, not app-platform P0 but blocks replicated `RuntimeProvider` honesty. |
| AP-18 | One-click catalog | COMPLETE 7 apps | DUPLICATE `preview/pipeline` UNWIRED | DUPLICATE | — | **PARTIAL:** 7 seeded appstore entries `COMPLETE` comparable to CapRover; unwired `pipeline` queue+schedule with no HTTP/UI `pipeline/service.go:40` is the `AP-18` `UNWIRED` remainder. |

**GH overlap:** `FINAL_PARITY §2 GH-01..GH-19` do not directly overlap App Platform workload identity except via shared leases/orchestration (`store_deployments.go:300` `ClaimExecutionLease` vs `queue/store.go:113` cancel race, `fencing.go` wrong edge). No GH P0 maps to AP — separate lanes.

**Subagent-02 vs 03 reconciliation:** `final-parity subagent-02` upgraded `recreate` to PARITY pre-fix; `reverification subagent-06` confirmed 3 fixes honest but narrowed `AP-02` to traffic/weight stubs; `implementation-plan subagent-03` §§2.1-2.9 design remains authoritative for execution interfaces (`PlacementExecutor`/`TrafficExecutor`), reconciler (`appreconciler`), health via beacon, revision tx, delete gate, volume unification. This prep audit endorses that plan.

---

## 3. Known P0s — Deep Dives (stubs lie complete, health localhost, scale no-op, rollback race)

### 3.1 P0 — Stubs lie complete (AP-02 refining FLF-01 / FORGE-LOGIC-001)

* **Before:** `execution.go:249-311` all `execute*` steps emitted events + `return nil` regardless of `runtime==nil`. `ExecuteDeployment:131-148` marked `completed 100%` with zero containers; test `deployment_test.go:40` asserted success.
* **Now:** `execution.go:264-274` fails closed `no runtime executor wired` and `beacon_executor.go:33` `SyncServerConfiguration+SendPower start` make `recreate` honest (`provision_regression_test.go:13`). **Remainder:** `execution.go:277` `executePromoteStep` still `UpdateDeploymentConfig` DB-only `active_target green↔blue`  `UpdateDeploymentConfig:286` with **no `trafficmanager`/`loadbalancer`/`service_endpoints` call**, no `HealthGate` verify, no port-collision avoidance (both blue+green on same hostPort). `execution.go:313,320` `ScaleUp/Down` and `299,306` `Drain*` only `verifyObservedRunning:338` `VerifyRunning bool` not replica count (`replicamanager.ListInstancesByApp:374` never consulted, `TargetReplicas` ignored). `steps.go:48` `stepsForStrategy` DAG ignores `canaryPercent:25` (stored but unpublished except event `305`).
* **Impact:** Blue-green/rolling/canary report `completed` with one server provisioned or none for replicated plane; silent downscale via `applyRolloutRequest:112` `TargetReplicas==0 →1` clobbers `AppService.Replicas=10`.
* **Fix shape (no code now):** introduce `PlacementExecutor{EnsureReplicas(ctx,serverID,want) VerifyReplicaCount}` + `TrafficExecutor{Promote(ctx,serverID,from,to)}` (§5), gate with `FORGE_DEPLOY_REQUIRE_PLACEMENT/TRAFFIC` flags, wire `ReplicaPlacementExecutor` in `main.go:393-395`. See §§2.1, 2.2.

### 3.2 P0 — Health localhost lie → default fixed but vantage still wrong (AP-08)

* **Before:** `healthgate.go:11` probed `localhost`, `rollout.go:71` loopback clamp `127.0.0.1明顯` — every remote-node health gate failed or hit API's own port.
* **Now:** `healthgate.go:12-30` default `host = HealthCheckHost || resolveNodeHost(ctx,ServerID)` where `resolveNodeHost:62` parses `ServerControlTarget NodeURL` hostname — correct for API-routable nodes. `WaitForHealthGate:79` threshold 3 consecutive over `Timeout 300s` ticker 5s correct.
* **Remainder:** `rollout.go:71-89` still rejects explicit `HealthCheckHost` non-loopback with `health checks may only target the local deployment gateway:87` — operator cannot pass `app.internal` or `10.0.1.20` via that field (must leave `Host==""` to get node-derived host; correct post-fix but confusing). `healthgate.go:37-50` dials **from API pod**, not **from beacon** on-node loopback `127.0.0.1:port`; container port bound to `127.0.0.1` is unreachable from API VPC. `compose/lifecycle.go:188` `WaitForHealthy` is `ps` (`State==running||Status==up`) + restart-count `+1`, never `docker inspect HealthStatus`. `health_check_configs` (`114_e_zero_downtime_deploy.sql:15`) disjoint from deployment gate.
* **Fix shape:** relax `validateHealthGateTarget` (§2.3.3), add `BeaconHealthProbe` RPC (`daemon/client.go:690` `HealthProbeRequest{Host,Port,Path}` → beacon `127.0.0.1:port` GET) and try beacon-local first in `CheckHealth`, fallback to direct; optionally probe `AdminContainerInspect Health.Status`.

### 3.3 P0 — Scale no-op (AP-07 refining FLF-??)

* **Before:** `handlers_apphosting.go:416` silently no-oped when `ReplicaAppID==nil` — DB `replicas` updated, no `PlaceReplicas`.
* **Now:** `apphosting/service.go:432-433` refuses `targetReplicas>0 && ReplicaAppID==nil → "scale refused"` honest 422-equivalent; `replicamanager/service.go:361` `ScaleApp` honors generation + `appLocks[64]` shard locks correct.
* **Remainder:** `ScaleService:440-450` `UpdateAppService(replicas)` **before** `UpdateReplicaAppReplicas:446` → DB `replicas=10` while placement `ScaleApp` fails leaves split-brain (`service.go:448` returns `updated` with `non-fatal` replica error). `handlers_apphosting.go:416-437` does the opposite (`ScaleApp` after DB write but aborts with 500) — hand+service disagree on order. `autoscaler/service.go:332` scales `servers` vertical Memory/CPU, not horizontal replica HPA (`AP-07` P2). `compose` per-service `POST /compose/:id/scale` absent though `docker-compose create.go:86 --scale` exists.
* **Fix shape:** invert order — `ReplicaManager.ScaleApp` before DB mutation or wrapping tx; surface `Autoscaler` HPA `min/max/targetCPU` or rename vertical scaler; add compose scale route behind same replica endpoint.

### 3.4 P0 — Rollback race (AP-05 / AP-09 / LF-05)

* **Mechanism:** `revisions.go:82-91` reads `GetLatestDeploymentRevision` then `nextNum = latest.RevisionNumber+1` with **no `SELECT ... FOR UPDATE`** → concurrent `executeInitStep:219` `CreateRevision` can duplicate `RevisionNumber` (violates `UNIQUE(deployment_id,revision_number):21` as 23505 but swallowed). `Revisions.go:147-199` does `UpdateDeploymentStatusVersioned:167` (bumps `V→V+1`) then **ungated** `UpdateDeployment:174` `UPDATE deployments SET ... version=version+1 WHERE id=$1` with stale in-memory `deployment.Version==V` — second rook clobbers first. Fragmentation: revisions are per-`deployment_id` not per-`application`/`compose_stack`; each `TriggerDeploy` starts at 1, cannot list "app history" without joining `applications.current_deployment_id`.
* **Impact:** Duplicate `RevisionNumber` breaks `CompareRevisions:225` diff; concurrent rollback to different images last-writer-wins silently.
* **Fix shape:** add `store.CreateDeploymentRevisionTx` with `BEGIN; SELECT deployments WHERE id FOR UPDATE; SELECT COALESCE(MAX(revision_number),0)+1; INSERT; COMMIT;` plus single-tx `RollbackToRevision` CAS (`SELECT version FROM deployments FOR UPDATE` → `UPDATE ... WHERE id=$1 AND version=$2`). See `implementation-plan subagent-03 §2.4`.

---

## 4. Additional P1s (must not be left behind)

| Finding | File:line | Status | One-line impact |
|---|---|---|---|
| **Delete unconstrained** | `apphosting/service.go:175-183` → `store_apphosting.go:374` + `handlers_apphosting.go:259` | BROKEN P1 | Orphan containers + `placement_reservations` leaked, `instances` via `replica_app_id` orphan FK, `BeaconCommandLog` never cleaned; `DeleteComposeStack:608` cancels reservation but app path bypasses it |
| **Env_file / include dropped** | `compose/parser.go:197` `Environment:map{}` + `compose/service.go:134` `EnvFile` unused + `beacon/compose.go:380` only `EnvVars` map | BROKEN P0 | Host `env_file: ./secrets.env` silently yields empty env; deploy proceeds with empty secrets |
| **Volume bifurcation** | `compose/service.go:594-655` vs `beacon/compose.go:226-248` + `service.go:663` allowlist never plumbed | P1 | `200 {valid:true}` for `/data:/data` → beacon `400 host bind mount "/data" not allowed` after 2m health wait + DB `updating` |
| **Catalog swallow** | `appstore/service.go:239-244` `resolveTemplate` returns `tmpl` on `ExpandTemplate` error | BROKEN P1 | Interpolation `${VAR:?err}` failure silently deploys stale compose |
| **App create admission** | `service.go:100` defaults `""→DOCKER_IMAGE` + `CreateApplication:151` server_id nullable | PARTIAL P2 | CapRover `ImageMaker:416` exactly-one rule bypassed; `idle` forever until `TriggerDeploy:695` `409 has no server assigned` |
| **Delete cascade 2nd edge** | `lifecycle.go:596-604` `ComposeDelete` → `isNotFoundError` swallow but `markFailed` retains `deleting` | P2 | Node gone → stack stuck `deleting` not sweepable without admin retry |
| **Compose update without re-validation** | `lifecycle.go:476` `UpdateComposeStack` no `ValidateCompose` call | P2 | `PATCH /compose/:id` stores privileged YAML until beacon 400, hash already mutated before `rollbackStack:844` |
| **Restart non-atomic** | `lifecycle.go:781` `RestartStack = Stop→Start` without reservation hold/health rollback | P1 | `Stop` success + `Start` fail after concurrent `UpdateComposeStack` race leaves `failed` without rollback |
| **Progress tone drift** | `AdminAppsShared:9` `DeployStatusBadge` vs `compose/page.tsx` `statusConfig` (9 states) | P2 UX | Badge colors diverge per page |
| **Duplicate pollers** | `deployment-progress.tsx:41` 2s vs `DeploymentTimeline:27` 5s same `GET /admin/deployments/:id/steps` | P2 | Never dedup via `queryKey ["deployment-steps",id]`, leak risk |

---

## 5. Files to Touch (Phase-01 activation plan, grouped by workstream)

### Stream A — Deployment execution honesty (AP-02 refining FLF-01)

| File:line | Change | Notes |
|---|---|---|
| `forge/api/internal/services/deployment/execution.go:249-350` | Introduce `PlacementExecutor`, `TrafficExecutor` interfaces + `executeProvision/Promote/Drain/Scale*` branching on `TargetReplicas>1` + replica-count `VerifyReplicaCount` | §2.1 design; keep single-server `RuntimeExecutor` path for game servers |
| `forge/api/internal/services/deployment/placement_executor.go` **NEW** | `ReplicaPlacementExecutor{Store,ReplicaManager}` mapping `deployments.server_id → app_services.replica_app_id` | Preferred column `deployments.replica_app_id` migration needed (see §6) |
| `forge/api/internal/services/deployment/traffic_executor.go` **NEW** | `GatewayTrafficExecutor{Store,TrafficManager}` `PromoteTarget` | Initially fail-closed unless `FORGE_DEPLOY_REQUIRE_TRAFFIC=off` |
| `forge/api/internal/services/deployment/beacon_executor.go:60` | Harden `VerifyRunning` → `AdminContainerInspect State==running` + replica-count fallback | §2.1 `verifyObservedRunning` hardening |
| `forge/api/internal/services/deployment/service.go:113-143` | Add `placement TrafficExecutor` fields + `SetPlacementExecutor/SetTrafficExecutor` | Wire in `main.go` |
| `forge/api/cmd/api/main.go:393-395` | `WirePlacementExecutor(svc,store,replicaMgr)` + `WireTrafficExecutor` + `SetBeaconHealth` | Already wires `BeaconRuntimeExecutor` at `deployment.WireBeaconExecutor` |
| `forge/api/internal/store/store_deployments.go:128-195` | Add `UpdateDeploymentImageVersioned` or use existing `UpdateDeploymentConfig` version-guarded for rollback | Reuse `103_a_deployment_steps` pattern |

### Stream B — Health vantage fix (AP-08 refining FLF-05)

| File:line | Change |
|---|---|
| `forge/api/internal/services/deployment/rollout.go:71-90` | Relax `validateHealthGateTarget` — allow any hostname/IP, only hygiene ` \t\n\r/?#@`, max 253, RFC1123 check (plan §2.3.3) |
| `forge/api/internal/services/deployment/healthgate.go:11-50` | Try `BeaconHealthProbe` first, fallback to direct `resolveNodeHost`; surface silent `Path==""\|\|Port==0 → Passed:true` as warning |
| `forge/api/internal/daemon/client.go` | Add `HealthProbeRequest/Response` + `HealthProbe(ctx,baseURL,token,serverID,req)` → `POST /servers/:id/health-probe` |
| `beacon/internal/server/server.go` | Implement `handleHealthProbe` (`127.0.0.1` loopback dial, `CheckRedirect UseLastResponse`, `Passed=200-399`) + `ContainerHealth` via Docker inspect |
| `forge/api/migrations/211_*` | Optional: merge `health_check_configs` `source='deployment'` + `deployment_id` FK (plan §2.3.2) |

### Stream C — Start/stop reconciler (AP-03 splitting FLF-02)

| File:line | Change |
|---|---|
| `forge/api/internal/http/handlers_apphosting.go:450-498` | Optimistic synchronous actuation: `UpdateApp desired_state` then if `ServerID!=nil && Daemon!=nil` `SendPower("start"/"stop")` with `60s` timeout, honest `502` on failure, else `202 Accepted` with `observed_status:"deploying"` |
| `forge/api/internal/services/appreconciler/service.go` **NEW** | 30s poll `ListApplicationsNeedingReconcile` where `desired!=observed`, skip when `HasActiveDeployment` true, actuate `start/stop/removed`, `UpdateApplicationStatus:379` |
| `forge/api/internal/store/store_apphosting.go:211-379` | Add query `ListApplicationsNeedingReconcile` + `HasActiveDeployment` helper, partial index `idx_applications_reconcile (desired,observed) WHERE desired!=observed` (§3 migration) |
| `forge/web/lib/api/compose.ts:89` | Export `restartComposeStack(id)` `POST /compose/:id/restart` |
| `forge/api/internal/services/compose/lifecycle.go:781-786` | Make `RestartStack` reservation-aware + health-verified (`WaitForHealthy` + rollback hint) |

### Stream D — Revisions/rollback fencing (AP-05/AP-09)

| File:line | Change |
|---|---|
| `forge/api/internal/store/store_deployment_revisions.go:21` | Add `CreateDeploymentRevisionTx` (`BEGIN; SELECT deployments FOR UPDATE; SELECT MAX(revision_number)+1; INSERT; COMMIT`) + `isUniqueViolation 23505` retry |
| `forge/api/internal/services/deployment/revisions.go:82-124` | Call `CreateDeploymentRevisionTx`; on `23505` retry once |
| `forge/api/internal/services/deployment/revisions.go:147-199` | Wrap `RollbackToRevision` in single tx `FOR UPDATE` + version CAS (`UpdateDeployment...WHERE id=$1 AND version=$2` rowsAffected==0→409) |
| `forge/api/internal/http/handlers_revisions.go:32-45` | Map `ErrVersionConflict→409` in `respondDeploymentError` |
| `forge/api/internal/services/deployment/service.go:58-62` | Document `configHash` truncation (keep 12-hex but include `EnvVars/ports` in future) |

### Stream E — Delete + scale + env/mounts + catalog

| File:line | Change |
|---|---|
| `forge/api/internal/services/apphosting/service.go:175-183` | Gate `DeleteApp` with `isActiveStatus` over `ListDeployments` + `CurrentDeploymentID`; cancel reservations; delegate to `replicamanager.DeleteApp:501` when `ReplicaAppID!=nil`; sweep beacon `ComposeDelete?removeOrphans=true` |
| `forge/api/internal/services/apphosting/service.go:398-453` | Invert order `ScaleService` `ReplicaManager.ScaleApp` **before** `UpdateAppService` or tx-wrapped; handler `handlers_apphosting.go:416` simplified |
| `forge/api/internal/services/compose/service.go:594-677` | Unify predicate: make API `checkVolumesSecurity` match beacon `strings.HasPrefix(source,"/")` error **or** plumb `allowedMounts+isAdmin` via `composeDeployRequest` to beacon `validateComposeVolumes` — choose allowlist path |
| `forge/api/internal/services/compose/parser.go:197` | Fail-fast `env_file/include` presence → `errors.New("env_file is not supported; inline via EnvVars")` surfaced via `ValidateCompose` |
| `forge/api/internal/services/appstore/service.go:239-244` | Fail-fast `resolveTemplate` error returns error, do not swallow (`return "", err`) |
| `forge/api/internal/services/compose/lifecycle.go:476` | Add `ValidateComposeSecurity` re-check at `UpdateComposeStack` entry before `updating` |

---

## 6. Migrations — GatewayRouter & Overlap Assessment

**GatewayRouter does not overlap App Platform.** No app-platform table references `caddy_proxy`, `traefik_proxy`, `crossnode/routegroup`, `loadbalancer`, or `target_groups` directly. The only seam is `TrafficExecutor.Promote` which will call `trafficmanager.Service` at runtime, not schema.

| Migration | Needed? | File:line / DDL | Reason |
|---|---|---|---|
| `211_app_reconciler_index.sql` | **YES** App Platform | `CREATE INDEX idx_applications_reconcile ON applications(desired_state,observed_status) WHERE desired_state!=observed_status` | Backs `appreconciler` poll; scan would else seq-scan `applications` every 30s |
| `212_deployment_revision_tx_guard.sql` | **NO DDL** | None — tx via `SELECT ... FOR UPDATE` on existing `deployments` + existing `UNIQUE(deployment_id,revision_number)` `099` is sufficient; add helper only | Code-only fencing, no migration |
| `213_deployment_replica_app_link.sql` | **OPTIONAL** App Platform | `ALTER TABLE deployments ADD COLUMN replica_app_id uuid REFERENCES replica_applications(id) ON DELETE SET NULL; CREATE INDEX idx_deployments_replica_app ON deployments(replica_app_id)` | Preferred for `PlacementExecutor.resolveReplicaAppID` (direct join vs `Application→AppService` scan); otherwise resolve via service lookup — no gateway overlap |
| `214_health_check_configs_merge.sql` | **OPTIONAL** Health | `ALTER TABLE health_check_configs ADD COLUMN source text DEFAULT 'deployment', ADD COLUMN deployment_id uuid REFERENCES deployments(id)` + index | Unifies `deployments.health_gate_*` + `health_check_configs` dual truth — not required for P0 but prevents drift |
| `GatewayRouter` / `caddy_proxy.go:707` / `traefik_proxy.go:725` | **NO** for App Platform | — | Gateway single-writer, five-writers conflict (`FINAL_PARITY §8`) is Network lane; app promote would *consume* gateway service but not modify its tables in Phase-01. No migration to touch. |
| Existing 197 migrations (`095_deployments`, `099_deployment_revisions`, `098_app_platform_foundations`, `100_z_app_platform_applications`, `103_a_deployment_steps`, `106_deployment_unique_active`, `114_e_zero_downtime_deploy`) | **AS-IS** | Verify `idx_unique_active_deployment_per_server:106` still guards one active per server (used by `StartRollout:129` `isActiveStatus`) — not app-scoped but sufficient until app-scoped lease added | No change unless app-scoped guard desired |

**Message to parallel agents:** Do not add `GatewayRouter` migrations for App Platform in Phase-01. If Gateway team introduces `routegroup`/`middlewares` joins, App Platform's `TrafficExecutor` must write through their service (not direct table writes) to respect single-writer gate.

---

## 7. Risks

| Risk | Trigger | Likelihood | Blast radius | Mitigation |
|---|---|---|---|---|
| **Lease liveness tear** | `execution.go:32` unconditional `ReleaseExecutionLease` after 5m `ClaimExecutionLease:24` + 30min `ExecuteDeployment:171` timeout; long step exceeds lease → second worker claims, first defers wipe → third claims → double `Promote` | Medium until heartbeat added | P1 double-promote / double-provision | Use `ReleaseExecutionLeaseIfOwner:342` not `Release:353`; add heartbeat `TouchExecutionLease` every 60s; defer `context.WithoutCancel` for release |
| **Split-brain scale** | `ScaleService:440` before `ScaleReplicas` succeeds | High if scheduler contended | DB replicas != placement | Invert order, fail closed on placement error with `409` |
| **Restart storm after `StopStack`** | `compose/service.go:517` `restart:always` containers restart immediately after `StopStack:745` did `docker compose stop` (not `down`) | Medium docker host | Runaway containers fighting reconciler | Promote `restart:always` to error when `StackStatusStopped` or block at beacon `validateComposePolicy` |
| **Stale hash after beacon 400** | `lifecycle.go:501-517` `UpdateComposeStack` mutates `ComposeHash→updating` before beacon `ComposeDeploy:532` 400 | Medium | User sees `updating` then rollback, but DB briefly divergent | Validate at `UpdateComposeStack` entry same as `DeployComposeStack:264`; or TX the mutation |
| **Revision number 23505 surfacing** | Concurrent `CreateRevision:119` duplicate key | Medium during rolling blue-green burst | UI generic 500 unless mapped | Map `23505 → 409 retry` in service, add tx |
| **Health gate silent pass** | `healthgate.go:13` `Port==0\|\|Path==""→Passed:true` | High if caller omits fields | Unhealthy deploy marked completed | Emit warning `validateHealthGateTarget` requires `port+path` if `HealthGateEnabled` |
| **Env_file secret loss masquerading as runtime failure** | `parser.go:197` drop + beacon `.env` empty | High for hosts with `.env` secrets files | Service starts but `DB_PASSWORD=""` → auth fail | `ValidateCompose` error until allowlist mount plumbed |
| **Cross-app instance access** | `handlers_apphosting.go:1106` `instance.AppID != appID` where `instance.AppID` is `ReplicaAppID` not `Application.ID` | Low (guarded by `GetServiceStatus` org) | Phantom 200 vs 404 confusion | Chain check `Application→AppService→ReplicaAppID→Instance` |
| **Frontend stale idle** | No `UpdateApplicationStatus` from start/stop | High UX | Badge `idle` forever, user retries trigger `TriggerDeploy` duplicate | Ship `appreconciler` or make `POST /start` 502 when `SendPower` would fail |
| **Tenant leak** | `handlers_apphosting.go:138` `GET /apps` admin `ListApplications("", )` with `where=""` `store_apphosting.go:214` | Medium | Admin enumeration leak vs `ResourceControl` | Scope `GET /apps` to `X-Org-ID` or caller's orgs |

---

## 8. Checklist — Preparation Gate for Phase-01 Activation

**Evidence gate (must be file:line citable before merge):**

- [ ] `execution.go:264` `executeProvisionStep` provision honesty retained (run `provision_regression_test.go:13` green)
- [ ] `healthgate.go:62` `resolveNodeHost` still parses `ServerControlTarget.NodeURL` not external Host header
- [ ] `handlers_apphosting.go:500` restart still `SendPower restart` not `TriggerDeploy`
- [ ] `handlers_compose.go:413` deploy still `UpdateComposeStack` not `DeployComposeStack`
- [ ] `apphosting/service.go:432` scale admission still `ReplicaAppID==nil` refused

**P0 activators (in order, each flag-gated, each behind `FENCE` migration or nil-fail-closed):**

1. **Revision/rollback fence** — land `CreateDeploymentRevisionTx` + `RollbackToRevision` CAS tx (`§5 Stream D`) — no feature flag; immediate `409` on concurrent rollback; risk low — tx only narrows window. Verify with `pgbench` 20 concurrent `CreateRevision` against same `deployment_id` → zero `23505` leak, exactly one `409` on competing rollback.
2. **Volume predicate unification** — align `checkVolumesSecurity` vs `validateComposeVolumes:226` (`§5 Stream E`) — behind `FORGE_VOLUME_PREDICATE=unified` flag; default allowlist-until-beacon. Verify `POST /compose/validate {"content":"services.db.volumes: [\"/data:/data\"]"}` returns consistent error/warning at both layers before `CreateComposeStack`.
3. **Delete gate** — add `isActiveStatus` check in `DeleteApp` (`§5 Stream E`) — `FORGE_DELETE_REQUIRE_IDLE=true` default; verify `DELETE /apps/:id` while `ListDeployments isActiveStatus==true` returns `409 deployment is awaiting_health`.
4. **Env_file strict** — add `ValidateCompose` rejection for `env_file/include` (`§5 Stream E`) — `FORGE_ENV_FILE_STRICT=true`; verify `compose/validate` flags `env_file` as `error` not silent pass.
5. **AppStart reconciler (hybrid)** — handler optimistic `SendPower` + `appreconciler` loop (`§5 Stream C`) — `FORGE_RECONCILER_ENABLED=true`; verify `POST /apps/:id/start` when beacon `VerifyRunning=false` flips `observed_status` to `running` within 35s, and stale `idle` corrects after 30s tick.
6. **Placement/traffic executors** — `PlacementExecutor` + `TrafficExecutor` interfaces flag-gated (`§5 Stream A`) — `FORGE_DEPLOY_REQUIRE_PLACEMENT=true` for `TargetReplicas>1`, `REQUIRE_TRAFFIC=false` initially; verify `StartRollout` with `targetReplicas=5` creates 5 `instances` via `replicamanager/ScaleApp` + `PlacementReservation` and `Promote` flips only via traffic service (no DB-only `active_target` lie).
7. **Health via beacon** — `validateHealthGateTarget` relaxation + `HealthProbe` RPC (`§5 Stream B`) — `FORGE_HEALTH_VIA_BEACON=true`; verify `CheckHealth` via `beacon 127.0.0.1:port/path` passes when container is bound to loopback but API VPC cannot reach node IP; add `HealthStatus` docker probe for images with `HEALTHCHECK`.
8. **Scale atomicity** — invert DB↔placement order (`§5 Stream E`) + HPA story — verify `PATCH .../services/:id {replicas:10}` where `ScaleApp` mock fails → `GET /apps/:id/services/:id` still shows `replicas=old` (no split-brain).
9. **Catalog swallow fix** — `resolveTemplate` fail-fast (`§5 Stream E`) — verify missing `${VAR:?err}` returns `422` not `200` with stale compose.
10. **Restart + volume + frontend wiring** — `restartComposeStack` export in `compose.ts:89` + `RestartStack` reservation/health, `AllowedMounts` plumbing, dual poller dedup via single `useDeploymentSteps(id)` hook keyed `["deployment-steps",id]` 5s.

**Web parity gate:**

- [ ] Export `restartComposeStack` in `forge/web/lib/api/compose.ts:34` — compiled, not just typed; `app/admin/compose/[id]/page.tsx` button wired next to `stop/start` with disabled state when `StackStatus` not `running|stopped`
- [ ] Collapse `deployment-progress.tsx:41` vs `DeploymentTimeline:27` into one hook; tones via `AdminAppsShared:9` `statusTone` only (remove per-file `statusConfig` drift)
- [ ] `apps/[id]/page.tsx:28` tab router: `searchParams.get("tab")` synced via `useEffect` on `popstate`, not only `replaceState` on change

**Quality gate (before declaring AP-02 closed):**

- [ ] `rec` strategy integration test: `StartRollout{Strategy:Recreate,Image:"nginx:1.27@sha256:...",HealthCheck{Port:80,Path:"/"}}` → `ExecuteDeployment` → `GetDeployment.Status==completed` only if `runtime.ApplyDeployment`+`WaitForHealthGate` 3 consecutive succeed — must fail with `502` mock if VerifyRunning false (no false-completion)
- [ ] `blue-green` integration test: `StartBlueGreen` → `ExecuteDeployment` → `trafficmanager.PromoteTarget` mock called exactly once with `from=blue,to=green` before `UpdateDeploymentConfig active_target` flips — assert order via `pgx.Tx` log
- [ ] `rollback` race test: 10 goroutines `RollbackToRevision(same deployment, different rev)` → exactly one `409 version conflict`, others either 409 or eventual `in_progress` — no overwrite

**Operational gate:**

- [ ] No new `GatewayRouter` migrations introduced by App Platform lanes (verified by `grep -r GatewayRouter migrations/` after plan — must remain zero for this slice)
- [ ] `FORGE_DEPLOY_*`, `FORGE_HEALTH_VIA_BEACON`, `FORGE_RECONCILER_ENABLED` documented in `docs/env.md` with defaults and kill-switch (`off` reverts to honest-failure, never silent success)

---

## 9. References (source ∩ prep parity)

* `FINAL_PARITY_AUDIT.md §3 AP-01..AP-18` (18 rows, verdict line 91 core model sound, execution dishonest)
* `final-parity/subagent-02-app-platform.md` (20 rows §§3-4 LF-01..LF-06, §5 counts PARITY 4/PARTIAL 10/BROKEN 6, §7 activation order)
* `implementation-plan/subagent-03-app-platform.md` (§§1.1-1.8 current state tables, §§2.1-2.9 designs with code targets, migrations §3, risks §4, rollout §5)
* `reverification/subagent-06-app-lifecycle.md` (20 row parity, §4 focus questions, LF-03..LF-07 confirming 3 fixes landed, 5 remains)
* `reverification/subagent-07-app-scaling-health.md` (scaling admission fixed, http-vs-ps divergence intentional note, revisions race still open)
* `MASTER_FINDING_INDEX.md` `REF-APP-C02-FLF01`, `REF-APP-C03-FLF02`, `REF-APP-C05-FLF04`, `REF-APP-C06`, `REF-APP-C07` (admission fixed), `REF-APP-C08-FLF05` (health fixed), `REF-P6-COMP-01/02`, `REF-P6-APP-01/02/03`, `FORGE-LOGIC-001/002`
* Forge live code file:line catalog in §1 (all forge/api + beacon + web lines listed)

---

*Prepared without code modification. Next step: Phase-01 workstream owners pull their Stream (§5) and land behind flags in checklist order §8, keeping `recreate` honest under test (`provision_regression_test.go:13`) at every commit.*
