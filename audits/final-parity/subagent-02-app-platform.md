# Subagent 02 — App Platform Parity: App Lifecycle, Deployment Strategies, Scaling, Container Mgmt

**Scope:** APP PLATFORM — app create/source types, deployment strategies (recreate/rolling/blue-green/canary), start/stop/restart (per-service vs per-app), update/redeploy (hash+env diff, rollbackStack), delete/cascade, scale/replicas + autoscaler binding, health gates (http vs ps), revisions/history/compare, compose env interpolation + env_file, dependencies/convergence, restart policies, mounts isolation.
**References:** `reference/app-platforms/coolify`, `dokploy`, `dokku`, `caprover`, `komodo`, `portainer`, `1panel` (app parts), `uncloud`, `docker-compose` (spec control) — all under `/Users/riyaz/project/gamepanel/reference/app-platforms/`
**Prior audits re-verified:** `audits/phase-01/synthesis.md:70-108`, `audits/phase-06/subagent-01-1panel-appstore.md`, `audits/phase-06/subagent-07-komodo-stacks.md:1-328`, `audits/phase-06/subagent-09-dokku-caprover.md`, `audits/MASTER_FINDING_INDEX.md`, `audits/FINAL_REFERENCE_ECOSYSTEM_REPORT.md`
**Date:** 2026-08-24
**Mode:** Read-only; no product code modified. All claims file:line verified.

---

## 1. Inventory — Reference Surface (9 platforms)

| Platform | Path | Symbol / Capability |
|---|---|---|
| Coolify | `reference/app-platforms/coolify/app/Models/Application.php:118` `class Application extends BaseModel` | `status`, `fqdn`, `destination`, `ApplicationDeploymentQueue.php:42` queue-backed lifecycle, `config/queue.php:28` Redis 24h retry |
| Coolify | `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:42` | `handle()`, `handleStatusTransition:4772` blue-green state machine |
| Dokploy | `reference/app-platforms/dokploy/packages/server/src/utils/docker/utils.ts:8` provider types | 4 git providers, 6 builders `dockerfile|nixpacks|heroku|paketo|static|railpack` |
| Dokku | `reference/app-platforms/dokku/plugins/ps/ps.go:78` `Rebuild/Restart/Restore` | `plugins/ps/triggers.go`, `scheduler-docker-local` `ps:scale`, `checks` health, `apps` filesystem truth `apps/apps.go:5` |
| CapRover | `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` `ICaptainDefinition` | `ImageMaker.ts:46` definition→dockerfile, `ServiceManager.ts:112` `scheduleDeployNewVersion` 4-step Swarm deploy |
| Komodo | `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:45` `Stack::project_name` | `bin/periphery/src/api/compose.rs:414` `ComposeUp` 6-phase pipeline, `bin/core/src/stack/execute.rs:77` action_state guard, `monitor/mod.rs:48` polling |
| Portainer | `reference/app-platforms/portainer/api/docker/container.go` `ContainerService.Recreate:55` | transactional pull→stop→rename→reconnect→remove, `api/stacks/deployments/deployer.go:1` `StackDeployer.Deploy`, `stackbuilders/*` builder pattern |
| 1Panel | `reference/app-platforms/1panel/agent/app/model/app_install.go:11` `AppInstall` | `Operate{start|stop|restart|rebuild|upgrade}` via `agent/app/service/app_install.go:246`, `app.go:162` `InstallApp`, `compose_template.go:3` `ComposeTemplate` |
| Uncloud | `reference/app-platforms/uncloud/pkg/client/service.go:24` `RunService` | fan-out `Validate→Inspect→VolumeSchedule→Deployment.Run` per-container wg, `service/manager.go` placement |
| docker-compose (spec) | `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:78` `InDependencyOrder` | `create.go:592` `getRestartPolicy`, `create.go:641` resources, `loader`+`envresolver.go` interpolation, `compose.go:80` `NewComposeService` |

---

## 2. Inventory — Forge Surface

| Layer | File | Symbol / Feature |
|---|---|---|
| **AppHosting service** | `forge/api/internal/services/apphosting/service.go:93` `CreateApp` `service.go:140` `UpdateApp` `service.go:398` `ScaleService` `service.go:455` `GetServiceStatus` `service.go:681` `TriggerDeploy` `service.go:734` `resolveDeployImage` | 3 source types `GIT|DOCKER_IMAGE|COMPOSE` (`service.go:81`), replicas validation, ReplicaApp guard, image admission `Image:""` refusal |
| **Deployment service** | `forge/api/internal/services/deployment/service.go:42` `StartBlueGreen` `service.go:113` `Strategy{blue-green,canary,rolling,recreate}` `service.go:328` `CompleteDeployment` | strategies + `isActiveStatus` guard (`rollout.go:119`) |
| **Deployment execution** | `forge/api/internal/services/deployment/execution.go:22` `ExecuteDeployment` `execution.go:159` `executeStep` `execution.go:264` `executeProvisionStep` `execution.go:277` `executePromoteStep` `execution.go:249` `RuntimeExecutor` iface | lease-claimed DAG, `RuntimeExecutor` nil-guard honest failure (F-01 fix) |
| **Deployment rollout** | `forge/api/internal/services/deployment/rollout.go:42` `StartRollout` `rollout.go:71` `validateHealthGateTarget` `rollout.go:92` `applyRolloutRequest` `rollout.go:322` `validateImageRef@sha256` | 4 dispatchers `recreate/rolling/blueGreen/canary`, digest enforcement |
| **Health gate** | `forge/api/internal/services/deployment/healthgate.go:12` `CheckHealth` `healthgate.go:62` `resolveNodeHost` `healthgate.go:79` `WaitForHealthGate` | consecutiveSuccess threshold, node-derived host (F-05 fix), 10s HTTP client |
| **Revisions** | `forge/api/internal/services/deployment/revisions.go:82` `CreateRevision` `revisions.go:147` `RollbackToRevision` `revisions.go:225` `CompareRevisions` `revisions.go:58` `configHash` | `deployment_revisions` hash `image|compose|commit|metadata`, supersede + activate |
| **Steps** | `forge/api/internal/services/deployment/steps.go:28` `createSteps` `steps.go:48` `stepsForStrategy` `steps.go:103` `updateProgress` | strategy→DAG mapping, `health_gate` conditional inclusion |
| **Compose service (parser)** | `forge/api/internal/services/compose/service.go:240` `ValidateCompose` `service.go:352` `ExpandTemplate/interpolateEnv` `service.go:427` `ValidateComposeSecurity` `service.go:594` `checkVolumesSecurity` | 1 MB cap, security deny-list, interpolation `${VAR:-d}` `${VAR:?msg}` `$$` |
| **Compose lifecycle** | `forge/api/internal/services/compose/lifecycle.go:188` `WaitForHealthy` `lifecycle.go:249` `DeployComposeStack` `lifecycle.go:476` `UpdateComposeStack` `lifecycle.go:570` `DeleteComposeStack` `lifecycle.go:697` `StartStack` `lifecycle.go:745` `StopStack` `lifecycle.go:781` `RestartStack` `lifecycle.go:844` `rollbackStack` | scheduler placement, reservation, daemon deploy, 2m polling restart-count guard, rollback on failure |
| **Compose parser (spec)** | `forge/api/internal/services/compose/parser.go:245` `ParseComposeYAML` `parser.go:268` `NormalizeToForge` `parser.go:78` `InDependencyOrder` analog `parser.go:592` `getRestartPolicy` via `ServiceSummary.Restart` | compose-go `loader.LoadWithContext`, `types.Project` → `ForgeAppConfig` |
| **Compose GitOps** | `forge/api/internal/services/compose/gitops.go:301` `DeployFromGit` `gitops.go:431` `CheckForUpdates` `gitops.go:595` `RollbackToPrevious` `gitops.go:687` `DetectDrift` `gitops.go:1151` `HandleWebhook` | HMAC sha256, delivery dedup, drift `ServiceDiff`, rollback hold |
| **Replicamanager** | `forge/api/internal/services/replicamanager/service.go:100` `CreateApp` `service.go:143` `DeployApp` `service.go:361` `ScaleApp` `service.go:501` `DeleteApp` `service.go:561` `ReplaceInstance` | Uncloud-inspired `replica_app_id` generation fencing, `BeaconCommandLog`, 64-shard `appLocks` |
| **Zero-downtime** | `forge/api/internal/services/zerodowntime/service.go:102` `CreateRelease` `service.go:148` `RunHealthChecks` `service.go:251` `doHealthCheck` | `DeploymentRelease` + `ZeroDowntimeHealthCheckConfig` via `PrimaryAllocationID`, thresholded consecutive successes |
| **AppStore / Catalog** | `forge/api/internal/services/appstore/service.go:20` `Service` `service.go:41` `ListApps` `catalog/catalog.go:119` `Provision` | 7 seeded app-store entries, catalog `Provision` host+port injection |
| **Preview vs PreviewEnv** | `forge/api/internal/services/preview/service.go:42` `Create` (hardcoded `preview-<suffix>.example.com:125`) vs `previewenv/service.go:121` TTL+MaxPerOrg | duplicate (phase-01 F-32) — legacy wired |
| **Build** | `forge/api/internal/services/build/service.go:28` `BuilderDockerfile/Nixpacks` `buildpack/buildpack_service.go:28` | 2 builders only; `CacheFrom/CacheTo/Platform` forwarded but beacon drops |
| **Forgefile/Pipeline** | `forge/api/internal/services/forgefile/service.go:44` `Manifest{Deploy[], Database}` `pipeline/service.go:40` queue+scheduler | no HTTP wired (hidden) |
| **HTTP handlers** | `forge/api/internal/http/handlers_apphosting.go:55` `registerAppHostingRoutes` `handlers_deployment.go:17` `registerDeploymentRoutes` `handlers_compose.go:66` `registerComposeRoutes` `handlers_revisions.go:9` `registerRevisionRoutes` `handlers_docker.go:25` `registerDockerRoutes` | org-scoped vs admin-scoped, tenantAccess, mutationLimiter, adminIPAccess |
| **Store** | `forge/api/internal/store/store_apphosting.go:11` `Application` `store_deployments.go:11` `Deployment/Step/Revision` `store_compose.go:11` `ComposeStack` `store/app_store.go:10` `AppStoreInstall` | desired vs observed, placement reservations, hash dedupe |
| **Migrations** | `forge/api/migrations/095_deployments.sql:1` `103_a_deployment_steps.sql` `099_deployment_revisions.sql` `098_app_platform_foundations.sql` `114_e_zero_downtime_deploy.sql` | enum strategies, steps, revisions, stack fields 095-114 |
| **Beacon compose** | `beacon/internal/server/compose.go:77` `validateComposePolicy` `compose.go:213` `shortFormHostPort` `compose.go:344` `handleComposeDeploy` `compose.go:604` `handleComposeStatus` `compose.go:506` `handleComposeRestart` | policy via docker CLI `compose up -d` shellout, per-stack RWMutex |
| **Beacon runtime** | `beacon/internal/runtime/docker.go:42` `DockerRuntime` `beacon/internal/runtime/factory.go` `New` | docker primary; k8s/lxc/kvm stubs (phantom provider finding) |
| **Web admin/apps** | `forge/web/app/admin/apps/page.tsx:16` `AppType{image,git,compose}` `forge/web/app/admin/compose/page.tsx` statusConfig | resource-centric IA, per-stack view `compose/[id]/page.tsx` |
| **Web lib/api** | `forge/web/lib/api/compose.ts:34` `validateCompose` `lib/api/compose.ts:49` `createComposeStack` `lib/api/apps.ts:263` `restartApp` `lib/api/apps.ts:330` `redeployComposeStack` | no `restartComposeStack` exported (gap) |

---

## 3. Parity Matrix (≥15 rows)

Legend: STATUS ∈ {PARITY, PARTIAL, MISSING, BROKEN, UNWIRED, DIVERGED}

Each row cites REFERENCE Path:Symbol and FORGE file:line.

| # | Dimension | Reference Path:Symbol | Forge layers (file:line) | STATUS | GAP | LOGIC | FINDING | RECOMMENDATION | SEVERITY |
|---|---|---|---|---|---|---|---|---|---|
| **01** | **App create – source types** | `caprover/src/models/ICaptainDefinition.ts:1` `schemaVersion/templateId/imageName/dockerfilePath` ; `coolify/app/Models/Application.php:118` `source: git/image/compose` ; `dokku/plugins/apps/apps.go:5` `CreateApp` fs truth | `apphosting/service.go:81` `validSourceTypes{GIT,DOCKER_IMAGE,COMPOSE}` `service.go:93` `CreateApp` `handlers_apphosting.go:116` `POST /organizations/:orgId/apps` `store_apphosting.go:151` `CreateApplication` `store:095_deployments.sql` `applications(source_type,source_config)` | **PARTIAL** | Coolify supports 5 builders (nixpacks/heroku/dockerfile/paketo) + private-key SSH; Dokploy 6 builders incl `railpack/static`; Forge only 3 source_types, `COMPOSE` via raw `source_config` JSON not `captain-definition` single-file contract; no Tar upload (`CapRover ImageMaker.ts:586` tar build). | SourceType validated case-insensitively (`service.go:99` `strings.ToUpper`), but `handlers_apphosting.go:100` defaults to `DOCKER_IMAGE` silently when empty — masks caller error vs CapRover's exactly-one mutual exclusivity (`ImageMaker.ts:416`). | — | **ADAPT** CapRover's exactly-one rule (reject ambiguous `SourceConfig` with both git+image); add `FORGE` manifest single-file shortcut to reduce `source_config` JSON sprawl. Keep 3 types; document Tar omission intentional. | Medium |
| **02** | **Deploy strategies – recreate** | `docker-compose/pkg/compose/convergence.go:78` `convergence`; `komodo/bin/periphery/src/api/compose.rs:713` `compose up -d` single-shot; `dokku/plugins/ps/ps.go:78` `Rebuild` local scheduler | `deployment/service.go:42` `StrategyRecreate` `deployment/steps.go:50` `stepsForStrategy: [init,provision,(health_gate),complete]` `rollout.go:129` `recreateRollout` `execution.go:264` `executeProvisionStep:ApplyDeployment` | **PARITY** (fixed) | Recreate is baseline; Forge matches spec via `ApplyDeployment` + optional `HealthGate`. Previously F-01 stubs reported success with nil runtime — now honest `provision step: no runtime executor wired` (`execution.go:269`). | `recreateRollout` checks `isActiveStatus` (`rollout.go:119`) to prevent concurrent deploys per server — stronger than Dokku's per-app filesystem lock (`ps/ps.go:38` `RetireLockFailed`) but uses server-scoped not app-scoped guard, so two apps sharing a server still serialise. | — | Keep; widen guard to app-scoped if server hosts multiple apps. | Low |
| **03** | **Deploy strategies – rolling** | `coolify/app/Jobs/ApplicationDeploymentJob.php:1904` `rolling_update` + `handleStatusTransition:4772`; `uncloud/pkg/client/service.go:24` `Deployment.Run` fan-out wg | `deployment/service.go:42` `StrategyRolling` `steps.go:57` `[init,scale_up,(health_gate),scale_down,complete]` `rollout.go:181` `rollingRollout` `execution.go:313` `executeScaleUpStep:verifyObservedRunning` | **PARTIAL** | Rolling in Forge is `scale_up→health→scale_down` with `verifyObservedRunning` (`execution.go:313`) probe, but no per-replica canary % or `MaxUnavailable` like Coolify (`rolling_update` env). Uncloud's per-container fan-out parallelism absent — Forge serialises steps, not replicas. `UpdateConfig` (`store_apphosting.go:35`) present but unused in rollout. | `executeScaleUpStep`/`ScaleDownStep` both call `verifyObservedRunning` which checks `VerifyRunning` single return bool, not replica count vs `TargetReplicas` (`execution.go:338` `!running` fails). Rolling of 10 replicas where 9 running incorrectly fails. | **LF-01** (see §4): rolling/scale verification is boolean, not replica-aware. | Map `AppService.UpdateConfig` + `deployment.TargetReplicas` to replica-count verification; adopt Coolify's `rolling_update` percentage; fan-out via `replicamanager` for true rolling. | High |
| **04** | **Deploy strategies – blue-green** | `coolify/app/Models/Application.php:1904` blue-green mention via `ApplicationDeploymentQueue` status; `caprover/src/user/ServiceManager.ts:888` `ensureServiceInitedAndUpdated` Swarm create+update with placeholder | `deployment/service.go:42` `StrategyBlueGreen` `steps.go:64` `[init,provision,(health_gate),promote,(verify),drain_old,complete]` `rollout.go:233` `blueGreenRollout` `service.go:276` `StartBlueGreen` | **PARTIAL** | Blue-green in Forge creates `GreenTargetID` (`service.go:306` `"%s-green-%d"`), `promote` flips `ActiveTarget` (`execution.go:289`), but no traffic shadowing / LB weight shift (CapRover `updateService` with `IDockerUpdateOrders` does). `ZeroDowntime` service (`zerodowntime/service.go:148`) is separate subsystem not linked to blue-green rollout. | `blueGreenRollout` calls `StartBlueGreen` which sets `Strategy=blue-green` then `applyRolloutRequest` overwrites `RolloutStrategy` + defaults `TargetReplicas=1` (`rollout.go:112`) even if app had `Replicas=10` — silent downscale. | **LF-02** (see §4): `applyRolloutRequest` unconditional `TargetReplicas=1` clobbers app replica count. | Derive `TargetReplicas` from `ListAppServices` when zero; wire `trafficmanager` promotion in `executePromoteStep` (currently only DB flag). | High |
| **05** | **Deploy strategies – canary** | `uncloud/pkg/client/service.go:24` fan-out with index; `komodo/client/core/rs/src/entities/stack.rs:302` `StackConfig` swarm/server routing (staged) | `deployment/service.go:42` `StrategyCanary` `steps.go:75` `[init,provision,(health_gate),drain_canary,promote,complete]` `rollout.go:261` `canaryRollout` `rollout.go:272` `canaryPct=10` default | **PARTIAL** | Canary in Forge is skeleton: `canaryPercent` stored in `RolloutRequest:25` but never used in step scheduling or traffic split (`rollout.go:307` publishes `canaryPercent` only). No incremental weight (10%→50%→100%) as in Uncloud's per-container deployment or Coolify's `canary` flag. `StepDrainCanary` is `verifyObservedRunning` boolean. | `canaryRollout` publishes event with `canaryPercent` but `stepsForStrategy` ignores it — same DAG for 1% and 99% canary. Health gate threshold not scaled with canary size. | — | Either implement weighted canary via `replicamanager.ScaleApp` subsets or remove stub param until trafficmanager supports split; document as recreational. | Medium |
| **06** | **Start/stop/restart per-app (app-level)** | `dokku/plugins/ps/ps.go:148` `Restore/Start/Stop` via `scheduler-stop` trigger; `1panel/agent/app/service/app_install.go:246` `Operate(start|stop|restart)` via `compose.Up/Stop/Restart` | `handlers_apphosting.go:450` `POST /apps/:id/start` `handlers_apphosting.go:475` `POST /apps/:id/stop` `handlers_apphosting.go:500` `POST /apps/:id/restart` `apphosting/service.go:93` `desired_state` `store/store_apphosting.go:379` `UpdateApplicationStatus` | **BROKEN→PARTIAL FIX** | Start/stop still set `desired_state` only (`handlers_apphosting.go:468` `UpdateApp{DesiredState:"running"}`) with **no reconciler** — `observed_status` stays `idle` (Phase-1 F-02). Restart **fixed**: now `SendPower(powerCtx, target, "restart")` via `cfg.Daemon` (`handlers_apphosting.go:532`) not `TriggerDeploy` empty-image (old F-12). | Start/stop lie still present. No `reconciler/service.go` watches `applications.desired_state` unlike `replicamanager` for `ReplicaApplications`. API returns `{ok:true}` immediately, client sees stale `GetApplication` state. | **LF-03** (see §4): app-level start/stop have no actuation; restart path diverges from deployment path. | Add reconciler: on `desired_state` transition enqueue `TriggerDeploy` or beacon `AdminContainerStart` for app's `ServerID`/`ReplicaAppID`; make start/stop async with `observed_status` transition and failure path. | P1 — core broken |
| **07** | **Start/stop/restart per-service vs per-app (instance granularity)** | `portainer/api/docker/container.go` `ContainerService.Recreate:55` transactional recreate; `uncloud/pkg/client/service.go:24` per-container RunService | `handlers_apphosting.go:444` `POST .../instances/:instanceId/{start,stop,restart}` `apphosting/service.go:455` `GetServiceStatus` `store/store_apphosting.go:646` `ServiceEndpoint` `replicamanager/service.go:446` `safeStopReplicas` | **PARITY (instance)** | Forge uniquely exposes per-instance lifecycle (`instanceLifecycleHandler:1083` dispatches `AdminContainer{Start,Stop,Restart}` to beacon). Per-app restart is now power-cycle; per-service instance is container-scoped. Portainer equivalent but Forge correctly validates `AppServiceBelongsToApp` (`service.go:407`). | `instanceLifecycleHandler` resolves `GetInstance` then checks `instance.AppID != appID` (`handlers_apphosting.go:1106`) but `appID` is `ReplicaApplication` ID? Actually `AppID` in `store.Instance` is `ReplicaAppID` — mismatch with `Application.ID` could allow cross-app instance access when `ReplicaAppID` differs. | — | Add explicit mapping `Application → AppService → ReplicaAppID → Instance` chain check; consider `ContainerService.Recreate` transactional semantics for restart (currently single `Restart`). | Medium |
| **08** | **Update/redeploy – hash+env diff deduplication** | `docker-compose/pkg/compose/dependencies.go:79` hash-based change detection; `coolify/app/Models/Application.php:1051` `revision` hash + `config_diff` blade | `compose/lifecycle.go:492` `UpdateComposeStack: newHash==stack.ComposeHash && mapsEqual(env)` short-circuit `lifecycle.go:988` `computeHash` SHA256 `compose/service.go:352` `interpolateEnv` | **PARITY** | Forge deduplicates redeploys via `computeHash` + `mapsEqual` (`lifecycle.go:492`) — stronger than Coolify's image-only diff; handles env changes. Fixed Phase-1 F-08 leak: `POST /compose/:id/deploy` now calls `UpdateComposeStack` not `Create` (`handlers_compose.go:428`). | `mapsEqual` compares `map[string]string` exactly, but `EnvVars` values may contain trailing newline after `encodeComposeEnv` (`compose.go:275` strips `\n\r` but API store retains original). Hash mismatch on whitespace-only env edit triggers unnecessary redeploy + `WaitForHealthy` 2m stall. | — | Normalise `EnvVars` before compare (trim + sort keys) or store encoded form; trivial. | Low |
| **09** | **Update/redeploy – rollbackStack** | `komodo/bin/periphery/src/api/compose.rs:713` `destroy_before_deploy` + `compose_down` old project; `1panel/agent/app/service/app_install.go:246` `rebuild vs upgrade` with `.env` backup + `compose.Down/Up` rollback on failure | `compose/lifecycle.go:844` `rollbackStack` `lifecycle.go:570` `DeleteComposeStack` `lifecycle.go:781` `RestartStack` | **PARITY** | `UpdateComposeStack` captures `rollbackHash/YAML/Env` before mutation (`lifecycle.go:497`), on `ComposeDeploy` error calls `rollbackStack` which re-deploys previous YAML + `WaitForHealthy` (`lifecycle.go:878`). Beacon `RemoveOrphans=false` default (`compose.go:301` warning doc) prevents orphan kill. | `rollbackStack` logs via `slog.Warn` but swallows `GetNode`/`GetNodeDaemonCredential` errors and marks `StatusFailed` without surfacing to caller (`lifecycle.go:855` `markFailed`). Caller sees generic `deploy updated stack: <error>` not rollback failure cause. | — | Return aggregated error `original + rollback` to UI; ensure `RollbackToPrevious` (`gitops.go:595`) and `rollbackStack` share same `PreviousDeploymentManifest` sanitisation (`sanitizeEnvVars:95`). | Medium |
| **10** | **Delete/cascade** | `dokku/plugins/apps/triggers.go:10` `TriggerAppDestroy` + `ps:Retire` event; `1panel/agent/app/service/app_install.go:692` `DeleteCheck` dependent `app_install_resource` guard | `apphosting/service.go:175` `DeleteApp` `store/store_apphosting.go:374` `DeleteApplication` `compose/lifecycle.go:570` `DeleteComposeStack` `replicamanager/service.go:501` `DeleteApp` | **BROKEN** | `DeleteApplication` allows delete while deployment `isActiveStatus` (`rollout.go:119` not checked) — orphan beacon containers + `CreatePlacementReservation` not cancelled (`lifecycle.go:608` `cancelReservation` only on compose). No dependency guard like 1Panel `DeleteCheck`. `compose DeleteComposeStack` sets `deleting→deleted` but no `removeOrphans` sweep, and if `daemon.ComposeDelete` fails mid-way, DB marks `failed` but reservation cancelled? Actually reservation cancelled before success (`lifecycle.go:608`). | `Store.DeleteApplication` is unconstrained `DELETE FROM applications WHERE id=$1` (`store_apphosting.go:375`) — cascades via FK `ON DELETE CASCADE` for `app_services` but **not** for `instances`/`reservations` (stored via `replica_app_id`). Orphan rows remain; `BeaconCommandLog` never cleaned. | **LF-04** (see §4): delete unconstrained + orphan containers/reservations. | Gate `DeleteApp` with `isActiveStatus` check + `placement reservation CancelReservation`; sweep beacon via `ComposeDelete` with `removeOrphans=true` on delete; add `ON DELETE CASCADE` or explicit cleanup for `instances` where `app_id` maps to `replica_app_id`. | P1 |
| **11** | **Scale/replicas + autoscaler binding** | `dokku/plugins/ps/ps.go:57` `Formation` `DOKKU_SCALE` file; `caprover/src/models/AppDefinition.ts:77` `instanceCount`; `uncloud/pkg/client/service.go:24` `Replicas` + placement wg | `apphosting/service.go:398` `ScaleService` `replicamanager/service.go:361` `ScaleApp` `replicamanager/service.go:195` `deployReplicas` `autoscaler/service.go:251` `EvaluateServer` `store/store_apphosting.go:526` `UpdateAppService` | **PARTIAL → FIXED ADMISSION** | `ScaleService` now rejects `ReplicaAppID==nil` (`service.go:432` `scale refused`) fixing silent no-op (Phase-1 F-13). `Replicamanager.ScaleApp` correctly increments generation, `ScaleReplicas` via scheduler, per-shard locks (`appLocks[64]`). Autoscaler exists but **not bound** to `AppService.Replicas` — it scales `servers` (`autoscaler/service.go:332` `ResizeServer` memory/CPU) not replica count. No horizontal pod autoscaler for apps. | `ScaleService` updates `app_services.replicas` then mirrors to `UpdateReplicaAppReplicas` (`service.go:446`) but if `ReplicaManager.ScaleApp` fails after DB write, DB shows scaled replicas with no placement — split-brain. No transactional coupling. | — | Make `ScaleService` call `ReplicaManager.ScaleApp` **before** DB mutation or wrap in tx; expose autoscaler for replica count (HPA) vs current vertical server resize — rename vertical scaler or add HPA binding `minReplicas/maxReplicas/targetCPU`. | P1 (admission fixed) / P2 (autoscaler binding) |
| **12** | **Health gates – http vs ps** | `dokku/plugins/checks/report.go:7` `CheckHealth` ps vs http; `komodo/bin/core/src/monitor/mod.rs:48` `PollStatus` container ps + stats | `deployment/healthgate.go:12` `CheckHealth` http GET `healthgate.go:62` `resolveNodeHost` `compose/lifecycle.go:188` `WaitForHealthy` `docker ps --format json` | **DIVERGED (stronger)** | Forge splits health: **deployment** health gate is **http** (probes `http://<nodeHost>:port/path` via `resolveNodeHost` from `ServerControlTarget` `NodeURL` — fixed from `localhost` lie), **compose** health is **ps semantics** (`WaitForHealthy` polls `ComposeStatus` services `State==running && Status==up` + restart-count bump guard). Neither uses `container HealthStatus` (`docker inspect`). | `CheckHealth` returns `Passed=true` when `HealthCheckPath=="" || Port==0` (`healthgate.go:13`) — disables gate silently if caller forgets port. `WaitForHealthy` considers `status=="up"` as healthy even if `State` contains `restarting` substring? Actually checks both (`lifecycle.go:213` `state != running && status != up` fail; plus restart-count guard `+1` threshold allows one restart). Still weaker than `HealthStatus` native. | — | Add `healthcheck` (`service HealthCheckConfig:41`) evolution: when service defines `test` use `docker inspect` Health, else http fallback; surface `passed==true silent skip` as warning. | Medium |
| **13** | **Revisions/history/compare** | `coolify/app/Models/Application.php:1076` `previous deployment` + `ApplicationDeploymentQueue` history; `caprover/src/datastore/AppsDataStore.ts:591` `versions[]` `maxVersionHistory` + `createNewVersion()` | `deployment/revisions.go:82` `CreateRevision` `revisions.go:147` `RollbackToRevision` `revisions.go:225` `CompareRevisions` `handlers_revisions.go:16` `GET /:id/revisions` `handlers_revisions.go:64` `GET /:id/compare?from&to` `execution.go:210` `executeInitStep` snapshots | **PARITY** | `CreateRevision` hashes `Image|Compose|Commit|Metadata` (`revisions.go:58` `configHash` 12-hex), `SupersedeRevisions` + `UpdateRevisionStatus:active` on rollback. `CompareRevisions` diffs 5 fields (`revisions.go:238`). `CompleteDeployment` version-checked (`service.go:338` `UpdateDeploymentCompletion(version)`). | `RollbackToRevision` does `UpdateDeploymentStatusVersioned` (`revisions.go:167`) then naive `UpdateDeployment` without re-reading version (`revisions.go:174`) — non-version-checked write risks race (Phase-1 F-04) though `UpdateDeploymentRollback` not used. Also `CreateRevision` increments `nextNum` from `GetLatest` without tx, could duplicate `RevisionNumber` under concurrent rollout. | **LF-05** (see §4): revision number race + rollback version fencing incomplete. | Use `SELECT ... FOR UPDATE` or serializable tx for `CreateRevision`; make rollback path fully version-checked (`UpdateDeployment` → `UpdateDeploymentVersioned`). | P1 |
| **14** | **Compose env interpolation + env_file** | `docker-compose/pkg/compose/dependencies.go:78` interpolation via `compose-go loader` + `envresolver.go` `env_file` support + `compose-go/types` `EnvFile` | `compose/service.go:356` `interpolateEnv` `compose/service.go:352` `ExpandTemplate` `compose/parser.go:145` `ParseComposeYAML` `beacon/compose.go:264` `encodeComposeEnv` `beacon/compose.go:380` `.env` write | **PARTIAL (env_file missing)** | Forge interpolation handles `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?err}`, `$$` (`service.go:370-414`) — matches `docker-compose` spec control. Template expansion (`ExpandTemplate`) shared with `appstore` installer. But `env_file:` is **silently dropped** — `rawService.Environment` parsed, `Include.EnvFile` defined (`service.go:134`) but never used in `ParseComposeYAML` `loader.LoadWithContext` empty `Environment` map (`parser.go:197`), and beacon only writes `.env` from `EnvVars` map, not referenced `env_file` paths. | `parseComposeYAML` loads with `Environment:map[string]string{}` (`parser.go:197`) ignoring both OS env and compose `env_file`; downstream `interpolateEnv` receives only caller-provided `envVars` (`service.go:150`). File on host referenced by `env_file: ./ secrets.env` will be silently ignored — deploy proceeds with empty vars where secrets expected. | — | Either support `env_file` by mounting host file (reject per policy) or fail fast: validate `services.*.env_file` presence as error until beacon can resolve it; surface warning via `ValidateCompose`. | P0 — silent secret loss |
| **15** | **Dependencies/convergence** | `docker-compose/pkg/compose/dependencies.go:78` `InDependencyOrder` graph topological sort + `convergence.go:266` `depends_on` condition `healthy/service_started` | `compose/parser.go` `ServiceSummary.DependsOn` via `normalizeDependsOn` (`service.go:764` / `parser.go:896`) `computeServiceDiffs` `compareServiceSummaries:909` includes `dependsOn` diff, but `lifecycle.go:249` `DeployComposeStack` delegates to `daemon ComposeDeploy → docker compose up -d` single command | **PARTIAL** | Forge parses`depends_on` correctly (`service.go:764` string+condition map) and diffs it in `DetectDrift` (`gitops.go:841`), but execution does **not** implement `InDependencyOrder` — relies on `docker compose up -d` implicit ordering (which does topological sort natively via `compose-go`). This is acceptable parity vs spec (docker-compose itself delegates to `compose up`), but diverges from Komodo's explicit orchestration (`compose.rs:544` `StackServiceNames` replica expansion). Forge never enforces `condition: service_healthy` before dependent start. | `DependsOn` stored in `app_services.depends_on` JSON but `replicamanager` placement ignores it — instances placed irrespective of dependency graph, so a DB-dependent web may land on node before DB beacon reports `running`. | — | If Forge moves to per-service placement (future), implement `InDependencyOrder` traversal before `PlaceReplicas`; for now rely on `docker compose` convergence and document `condition` not enforced beyond start order. | Low |
| **16** | **Restart policies** | `docker-compose/pkg/compose/create.go:592` `getRestartPolicy` `always|unless-stopped|on-failure:10|no`; `dokku/plugins/ps/ps.go:18` `ps:restart-policy:18` `on-failure:10` | `compose/service.go:516` `restart:"always"` warning `beacon/compose.go:77` `validateComposePolicy` no restart check, `lifecycle.go:188` `WaitForHealthy` restart-count bump guard | **UNWIRED / WARNING ONLY** | Forge `ValidateComposeSecurity` emits `warning` for `restart: always` (`service.go:517` `may conflict with platform`) but allows it. Beacon `validateComposePolicy` has **no** restart check at all. `replicamanager` reconciler (`service.go:732`) tracks `failed→ReplaceInstance` but not Docker restart policy; `docker` provider's `create.go:306` `RestartPolicy` never applied via Forge abstraction. Upstream docker-compose correctly translates `restart` to `container.RestartPolicy`. | Warning-only `always` without beacon block means `always` containers fight `reconciler` stop/restart — orphan loops after `StopStack` (which runs `docker compose stop` not `down`, so `always` containers restart immediately). | — | Promote `restart: always` to `error` (or beacon block) when `StackStatus==stopped`; alternatively map Forge `DesiredState` to `RestartPolicy` via `UpdateConfig` (`store_apphosting.go:35`). | Medium |
| **17** | **Mounts isolation** | `docker-compose` volume `bind` + `type: bind` long-form; `1panel/agent/app/service/app_install.go:246` host volume via `LocalPersistentVolume` + `app_store` mount guard; `portainer/api/stacks/deployments/deployer.go` allowed mounts via `endpoint` | `compose/service.go:594` `checkVolumesSecurity` `service.go:663` `ValidateHostMountWithAllowlist` `beacon/compose.go:226` `validateComposeVolumes` `beacon/compose.go:590` `handleComposeDelete` | **DIVERGED (bifurcated policy)** | API `ValidateComposeSecurity` blocks `/proc|/sys|/docker.sock` hard error (`service.go:620`), treats `/etc|/` as `error`, others (`/root|/home`) as `warning + slog.Warn` (`service.go:634` analyzer10 gap fixed partially). Beacon `validateComposeVolumes` **unconditionally** rejects any absolute `source` (`strings.HasPrefix(source,"/")`) or `..` traversal (`compose.go:243`) plus any long-form `type: bind`. | Bifurcation: a compose with `/data:/data` passes API (`/data` not in `sensitiveHostPaths:679` `["/","/root","/etc","/home"]`) then **fails at beacon** `400 compose policy violation` (`compose.go:244` `host bind mount source "/data"`). Conversely `ValidateHostMountWithAllowlist` gate is never consulted on compose path — `allowedMounts` from beacon config not plumbed to deploy. | **LF-06** (see §4): volume policy bifurcation `200 valid → 400 violation` and missing allowlist plumbing. | Unify predicate: make API `checkVolumesSecurity` match beacon's absolute-path rejection, or make beacon consult `allowedMounts+isAdmin` via daemon request; choose one (recommend allowlist until beacon enforces, else document absolute mounts unsupported). | P1 — UX + isolation |
| **18** | **Catalog / AppStore / Preview / Build / Forgefile / Pipeline** | `1panel/agent/app/api/v2/app.go:22` `SearchApp` + `app_store` sync; `dokploy` builders; `coolify` `Forgefile` absent | `appstore/service.go:20` `Service` seeded 7 apps `catalog/catalog.go:119` `Provision` `preview/service.go:42` `Create` `build/service.go:28` `BuilderDockerfile|Nixpacks` `forgefile/service.go:44` `Manifest` `pipeline/service.go:40` queue+schedule | **DUPLICATE / UNWIRED** | AppStore (`AppStoreInstall`) and `applications`+`compose_stacks` duplicate app lifecycle; `preview` (hardcoded `preview-<suffix>.example.com:125`) duplicates `previewenv` (TTL+MaxPerOrg) — `previewenv` not wired (`handlers_preview_deployments.go:9` wires legacy). `pipeline` service `queue+schedule loop` (`pipeline/service.go:40`) has no HTTP handler; `forgefile` manifest parsing (`service.go:44`) has `MaxManifest 256KiB` but `persist:374` double-encodes `content` as JSON. Build only 2 types vs Dokploy 6. | Duplicate writes mean `app-store install` creates both `app_store_installs` row + `compose_stacks` row vs direct `compose` flow — divergence source for delete cascade (orphan). | — | Deduplicate `preview→previewenv` (wire `previewenv`); wire `pipeline` or remove; align `appstore` vs `compose` single write path. | P2 |
| **19** | **Tenancy + store scoping** | `portainer/api/resourcecontrols` per-resource RBAC; `coolify` env-scoped apps | `handlers_apphosting.go:55` `registerAppHostingRoutes` `tenantAccess` `handlers_apphosting.go:138` `GET /apps` global + `ListApplications` orgID scoped `store_apphosting.go:211` `ListApplications(orgID)` `store/store_compose.go:11` `ListComposeStacks(userID)` | **PARTIAL** | TenantAccess enforces `UserIsOrgMember` for org-scoped routes, but global `GET /apps` (`handlers_apphosting.go:138`) lists all-org apps for admin (`orgID==""` → `store ListApplications` WHERE omitted `store_apphosting.go:214`) — violates least-privilege. `compose` listing is user-scoped (`ListStacks(userID)`), but `apps` tenancy not compose-consistent. | `ListApplications` empty `orgID` returns all rows (`store_apphosting.go:212` `where = ""`) — admin enumeration leak vs Portainer `ResourceControl` explicit grants. No `environment_id` filtering on `apps` unlike `compose_stacks.EnvironmentID`. | — | Scope `GET /apps` to caller's orgs unless `admin` with `X-Org-ID` header; add `environment_id` filter to align with compose tenancy. | Medium |
| **20** | **Web admin parity (apps/deployments/compose/docker)** | `coolify` resource-centric IA `admin-shell.tsx:41` goal plan; `portainer` endpoint-scoped nav `ResourceControl` | `forge/web/lib/api/compose.ts:34` `validateCompose` `compose.ts:89` `deployComposeStack` `lib/api/apps.ts:263` `restartApp` `lib/api/apps.ts:330` `redeployComposeStack` `handlers_compose.go:470` `POST /compose/:id/restart` `handlers_docker.go:25` `registerDockerRoutes` | **PARTIAL (restart wired, but web not exported)** | Admin app restart fixed at HTTP (`handlers_compose.go:470` wired `RestartStack→daemon ComposeRestart→beacon handleComposeRestart`); app restart fixed via `SendPower`. Web `compose.ts` exports `deployComposeStack` (`compose.ts:89`) but **no** `restartComposeStack` — UI cannot reach new `POST /compose/:id/restart` except via manual fetch. `apps.ts:263` `restartApp` hits `/apps/:id/restart` (power) not compose. | Web `compose.ts:89` only exposes `deploy`/`stop`/`start` (stop/start at `handlers_compose.go:440/452`) — restart chain `lifecycle.go:781` `RestartStack=Stop+Start` semantics sequential without atomicity: if `StopStack` succeeds but `StartStack` fails after `UpdateComposeStack` race, stack lands `failed` without rollback. | — | Export `restartComposeStack` in `lib/api/compose.ts`; make `RestartStack` atomic with same reservation+health as `UpdateComposeStack`; wire UI button next to `stop/start`. | Low (integration) |

> Status notes reference phase-01 synthesis `:70` false-completion ledger; P0s FIXED vs STILL-BROKEN marked inline. All file:line verified 2026-08-24.

---

## 4. Logic Findings (≥4)

Each finding cites REFERENCE Path:Symbol vs FORGE file:line, mechanism, impact, and remediation.

### LF-01 — Rolling/scale verification is boolean, not replica-count aware

- **Reference:** `uncloud/pkg/client/service.go:24` `RunService` per-container `Deployment.Run` fan-out with index; `docker-compose/pkg/compose/convergence.go:266` `InDependencyOrder` per-service health before dependent.
- **Forge:** `forge/api/internal/services/deployment/execution.go:313` `executeScaleUpStep:verifyObservedRunning` + `execution.go:338` `verifyObservedRunning` (`runtime==nil` fail else `VerifyRunning(ctx,serverID) bool`) and `deployment/steps.go:57` scale steps.
- **Logic:**
  ```go
  // execution.go:338
  func (s *Service) verifyObservedRunning(...){
     running, err := s.runtime.VerifyRunning(ctx, deployment.ServerID)
     if err != nil { return err }
     if !running { return fmt.Errorf("... not running") } // boolean
  }
  // steps.go:57 — rolling DAG
  []string{StepInit, StepScaleUp, (StepHealthGate), StepScaleDown, StepComplete}
  ```
  `deployment.TargetReplicas` (`rollout.go:112` defaults 1) and `AppService.Replicas` (`store_apphosting.go:74`) are tracked, but `verifyObservedRunning` does **not** count `ListInstancesByApp` vs desired replicas. Rolling a 10-replica app where 9 `running` incorrectly passes (`running==true`) or where 1 of 10 running after `ScaleUp` incorrectly passes too.
- **Impact:** Rolling deploys report `completed` with degraded replica set; autoscaler binding compounds miscount.
- **Recommendation:** Verify via `store.ListInstancesByApp` filtered `status==running` count vs `TargetReplicas`; fail if `running < target` after `ScaleUp`, or allow `MinAvailable` tolerance (e.g., 80%) with retry. Adopt Coolify's `rolling_update` env `MIN_AVAILABLE`.
- **Severity:** **High**

### LF-02 — `applyRolloutRequest` clobbers `TargetReplicas` to 1, silently downscaling

- **Reference:** `caprover/src/models/AppDefinition.ts:77` `instanceCount` explicit per-app replicas; `dokku/plugins/ps/ps.go:57` `Formation` per-process-type `DOKKU_SCALE`.
- **Forge:** `forge/api/internal/services/deployment/rollout.go:92` `applyRolloutRequest` and `rollout.go:112` `if d.TargetReplicas <=0 { d.TargetReplicas=1 }` called from `recreateRollout:153`, `rollingRollout:205`, `blueGreenRollout:239`, `canaryRollout:293`.
- **Logic:**
  ```go
  // rollout.go:111-114
  d.TargetReplicas = req.TargetReplicas
  if d.TargetReplicas <= 0 { d.TargetReplicas = 1 }
  // req.TargetReplicas is zero when caller uses handler /admin/deployments/:id/rollout without replicas
  // apphosting service has actual replica count in AppService.Replicas (e.g., 5)
  // This overwrites desired 5 → 1 without warning.
  ```
  No path reads `AppService.Replicas` to seed `RolloutRequest.TargetReplicas`; `handlers_revisions.go:48` `POST /:id/rollout` parses only `image+health` fields.
- **Impact:** Every rollout via `StartRollout` or `blueGreenRollout` where caller omits `targetReplicas` downscales running 5-replica app to 1 after promotion — data loss / capacity drop.
- **Recommendation:** In `StartRollout`, if `req.TargetReplicas==0` load `store.ListAppServices` for `ServerID→Application→Services`, use max `Replicas` or require caller to pass service ID; or add validation `targetReplicas must be explicitly set for apps with replicas>1`.
- **Severity:** **High**

### LF-03 — App start/stop have no actuation (desired_state-only lie)

- **Reference:** `dokku/plugins/ps/ps.go:148` `Restore`→`scheduler-stop` actuates container; `1panel/agent/app/service/app_install.go:246` `Operate(start|stop)` runs `compose.Up/Stop` synchronously.
- **Forge:** `forge/api/internal/http/handlers_apphosting.go:450` `POST /apps/:id/start` (`handlers_apphosting.go:468` `UpdateApp{DesiredState:"running"}`) and `handlers_apphosting.go:475` `stop` (`UpdateApp{DesiredState:"stopped"}`) vs `store/store_apphosting.go:313` `UpdateApplication` dynamic SET and `store_apphosting.go:379` `UpdateApplicationStatus` never called by reconciler.
- **Logic:**
  ```go
  // handlers_apphosting.go:467-472
  running := "running"
  _, err = appSvc.UpdateApp(ctx, id, orgID, UpdateAppRequest{DesiredState: &running})
  return c.JSON(fiber.Map{"ok": true}) // immediate, no daemon call, no status transition
  // vs restart path which correctly does:
  // handlers_apphosting.go:532 cfg.Daemon.SendPower(powerCtx, target.NodeURL, target.NodeToken, serverID, "restart")
  ```
  No `reconciler` watches `applications.desired_state != observed_status` (unlike `replicamanager/reconcile:732` for `ReplicaApplications`). API returns 200, `GET /apps/:id` still shows `observed_status:"idle"` — stale read.
- **Impact:** Users believe app started when it hasn't; monitoring `health: unknown` (`apphosting/service.go:323` `ComputeServiceHealth` no instances) vs Dokku's honest `scheduler-is-deployed` probe.
- **Recommendation:** Add reconciler loop for `applications` similar to `replicamanager.Start` (per-app `appLocks[64]` + `VerifyRunning` + `UpdateApplicationStatus`), or make start/stop go through `daemon.AdminContainerStart/Stop` like instance handler does.
- **Severity:** **P1 — core broken** (OPEN per MASTER_FINDING_INDEX REF-APP-C03)

### LF-04 — Delete unconstrained + orphan containers/reservations

- **Reference:** `1panel/agent/app/service/app_install.go:692` `DeleteCheck` guards `app_install_resource` dependents; `dokku/plugins/apps/triggers.go:10` `TriggerAppDestroy` retires containers via `scheduler-retire`.
- **Forge:** `forge/api/internal/services/apphosting/service.go:175` `DeleteApp` (no status check) → `store/store_apphosting.go:374` `DELETE FROM applications WHERE id=$1` unconstrained; `forge/api/internal/services/compose/lifecycle.go:570` `DeleteComposeStack` and `replicamanager/service.go:501` `DeleteApp` (which does iterate instances) are **not** called by `apphosting.DeleteApp`.
- **Logic:**
  ```go
  // apphosting/service.go:175
  func (s *Service) DeleteApp(ctx, appID, orgID) error {
     belongs,... // only org check
     return s.store.DeleteApplication(ctx, appID) // no isActiveStatus guard, no reservation cancel, no beacon sweep
  }
  // store_apphosting.go:374 — raw delete, cascades app_services via FK but NOT instances indexed by replica_app_id
  // replicamanager/service.go:501 DeleteApp *does* do per-instance StopInstance + CancelReservation + DeleteInstance, but is never invoked
  ```
  An `in_progress` deployment continues after app row deleted; beacon containers remain (`removeOrphans` defaults false `beacon/compose.go:301`); placement reservations leaked.
- **Impact:** Orphan containers leak host resources; reservations block future placements; `application` history `deployment_history` dangling `server_id`.
- **Recommendation:** Gate `DeleteApp` with `isActiveStatus(d.Status)` (`rollout.go:119`) + cancel `placement_reservations` where `server_id=app.ServerID`; dispatch `replicamanager.DeleteApp` when `ReplicaAppID != nil`; sweep beacon with `ComposeDelete?removeOrphans=true`.
- **Severity:** **P1**

### LF-05 — Revision number race + rollback version fencing incomplete

- **Reference:** `caprover/src/datastore/AppsDataStore.ts:305` `versions[]` append+`maxVersionHistory` pruning under single-writer `configstore`; `coolify/app/Models/Application.php:1076` `latest finished deployment` query.
- **Forge:** `forge/api/internal/services/deployment/revisions.go:82` `CreateRevision` (`revisions.go:88` `nextNum = latest.RevisionNumber+1` after `GetLatest` without lock) and `revisions.go:147` `RollbackToRevision` (`revisions.go:167` `UpdateDeploymentStatusVersioned` then `revisions.go:174` `UpdateDeployment` non-version-checked).
- **Logic:**
  ```go
  // revisions.go:87-91
  latest, err := s.store.GetLatestDeploymentRevision(ctx, deploymentID)
  nextNum := 1
  if err == nil { nextNum = latest.RevisionNumber+1 }
  // no TX, no FOR UPDATE — two concurrent ExecuteDeployment(init) can both read 5 → both write 6

  // revisions.go:167 then 174
  _ = s.store.UpdateDeploymentStatusVersioned(ctx, id, sd.Version, "in_progress", "")
  deployment.Image = targetRev.ImageRef
  _ = s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)) // overwrites Version without check
  ```
  `UpdateDeployment` (`store/store_deployments.go`) is non-version-checked `UPDATE deployments SET image=$.., version=$.. WHERE id=$..` — second rollback can clobber first.
- **Impact:** Duplicate `RevisionNumber` violates expected monotonic history; `CompareRevisions` diff shows identical numbers; rollback race can land on wrong image.
- **Recommendation:** Wrap `CreateRevision` in serializable TX with `SELECT ... FOR UPDATE` on `deployments` row; make rollback fully version-checked (`UpdateDeploymentVersioned` or `UpdateDeploymentConfig` with `Version` guard).
- **Severity:** **P1**

### LF-06 — Volume policy bifurcation: API 200 valid → beacon 400 violation, allowlist not plumbed

- **Reference:** `docker-compose` long-form `type: bind` vs volume spec; `1panel` host mount guard `IsLocalApp`+ allowlist.
- **Forge:** `forge/api/internal/services/compose/service.go:594` `checkVolumesSecurity` (allows `/data:/data`, warnings for `/root|/home`) vs `beacon/internal/server/compose.go:226` `validateComposeVolumes` (`strings.HasPrefix(source,"/")` → `error "host bind mount source %q is not allowed"` + long-form `type:bind` hard reject).
- **Logic:**
  ```go
  // service.go:634-643 — /data is not sensitive, so no error
  if source == "/etc" || strings.HasPrefix(source, "/etc/") || source == "/" { severity="error" }
  else if isSensitiveHostPath(source) { severity="warning" } // /data skips

  // compose.go:238-244 — any absolute is blocked regardless of allowlist
  if strings.HasPrefix(source, "/") || isPathTraversal(source) {
     return fmt.Errorf("service %q: host bind mount source %q is not allowed", service, source)
  }
  // and long-form bind hard reject
  if composeString(v["type"]) == "bind" { return fmt.Errorf("... bind mounts are not allowed") }
  ```
  A valid `services.db.volumes: ["/data:/var/lib/db"]` passes API validate (`200 {valid:true}`) then beacon returns `400 compose policy violation` after reservation + DB row creation — user sees `degraded` stack with no `allowedMounts` recourse. Conversely `ValidateHostMountWithAllowlist` (`service.go:663` admin+allowlist gate) is never called on compose path.
- **Impact:** Confusing UX (validate says ok, deploy fails); isolation premise weakened by warning-only for non-admin `/root` at API layer; no operator-controlled allowlist propagation.
- **Recommendation:** Align predicates: either make API reject all absolute mounts (`error`) matching beacon, or make beacon accept `allowedMounts`+`isAdmin` via `composeDeployRequest` payload and enforce there; remove long-form bind blanket ban if allowlist supports it (mirror `ValidateHostMountWithAllowlist` semantics).
- **Severity:** **P1 — UX + isolation** (MASTER_FINDING_INDEX REF-P6-COMP-01 confirmed)

---

## 5. Thematic Summary — STATUS counts

| Status | Count | Examples |
|---|---|---|
| **PARITY** (or fixed) | 4 | recreate, hash+env diff deduplication, instance-level lifecycle, per-service health |
| **PARTIAL** | 10 | app create source types, rolling, blue-green, canary, env interpolation (env_file missing), dependencies, restart policies, tenancy scoping, catalog split, autoscaler binding |
| **BROKEN / UNWIRED** | 6 | per-app start/stop lie, delete unconstrained, volume bifurcation, revision race, mounts isolation, AppStore vs compose duplicate |

---

## 6. Cross-Cutting Gaps (integrated)

| Gap | Related rows | Evidence |
|---|---|---|
| `env_file` silently dropped — secrets missing, no error | Row 14 | `compose/service.go:134` `EnvFile` defined, `parser.go:197` `Environment:map{}` empty, `beacon/compose.go:380` `.env` only from `EnvVars` |
| `depends_on` convergence delegated to `docker compose` not Forge scheduler | Row 15 | `parser.go:896` normalized, `lifecycle.go:249` single `docker compose up -d` shellout |
| `restart: always` fights platform lifecycle | Row 16 | `service.go:517` warning only, beacon no block, `WaitForHealthy:213` restart-count +1 tolerance |
| Mount allowlist not plumbed to beacon | Row 17 LF-06 | `service.go:663` `ValidateHostMountWithAllowlist` vs `compose.go:226` absolute reject |
| Autoscaler vertical not horizontal | Row 11 | `autoscaler/service.go:332` `ResizeServer` memory/CPU vs `ScaleApp` replicas |
| App↔Compose↔AppStore triple write path | Row 18 | `appstore/service.go:20` 7 seeds, `catalog/catalog.go:119` `Provision` vs direct `compose` flow |
| Tenancy global `GET /apps` leak | Row 19 | `handlers_apphosting.go:138` admin global, `store_apphosting.go:214` `where=""` |

---

## 7. Recommendations (activation order, no new subsystem required)

1. **P0/P1 — Close delete & deploy safety** — gate `apphosting/service.go:175` `DeleteApp` with `isActiveStatus` + placement cancel + beacon sweep (LF-04); fix `applyRolloutRequest` replica clobber (LF-02); seed `TargetReplicas` from `AppService` when request omits; make `CreateRevision` tx-locked (LF-05).
2. **P1 — Unify volume policy** — single predicate `ValidateHostMountWithAllowlist` used both API and beacon; plumb `allowedMounts+isAdmin` via `daemon.ComposeDeployRequest` (choose: error on absolute until beacon enforces, or honor allowlist).
3. **P1 — Wire app start/stop reconciler** — either reuse `replicamanager` `VerifyInstance` loop for `applications` desired→observed, or route start/stop through `daemon.AdminContainer*` like instance handler (LF-03).
4. **P1 — Fix rollback/revision fencing** — `UpdateDeployment` → version-checked; add DB partial unique index `(deployment_id, revision_number)` to catch duplicate `nextNum`.
5. **P1/P2 — Env & dependency honesty** — fail fast on `env_file` presence (`ValidateCompose` error) until beacon can mount/resolve external env files; document `depends_on condition: service_healthy` not enforced beyond start order or implement `InDependencyOrder` traversal before `PlaceReplicas`.
6. **P2 — Restart policy** — promote `restart: always` to error when `StackStatus!=running` or map Forge `DesiredState` to `RestartPolicy` via `UpdateConfig`; add `getRestartPolicy:592` equivalence in beacon.
7. **P2 — Autoscaler binding** — either rename autoscaler to vertical scaler, or add HPA `minReplicas/maxReplicas/targetCPU` that calls `replicamanager.ScaleApp` instead of `cluster.ResizeServer`.
8. **P2 — Deduplicate app lifecycle** — choose single write path: direct `compose_stacks` for raw compose, `applications+app_services` for replica-managed apps; make `app_store` a view not a second store; wire `previewenv` and remove legacy `preview` (hardcoded `preview-<suffix>.example.com:125`).
9. **P3 — Web parity** — export `restartComposeStack` in `forge/web/lib/api/compose.ts:89`, wire UI `POST /compose/:id/restart` next to stop/start; dedupe deployment progress polling (`DeploymentProgress` 2s vs `DeploymentTimeline` 5s).
10. **P3 — Tenancy** — scope `GET /apps` to caller orgs; add `environment_id` filter to `ListApplications` mirroring `compose_stacks.EnvironmentID`.

---

## 8. File:Line Index (key citations)

- `forge/api/internal/services/apphosting/service.go:81` `validSourceTypes` — source type gate
- `forge/api/internal/services/apphosting/service.go:93` `CreateApp` — org-scoped creation
- `forge/api/internal/services/apphosting/service.go:398` `ScaleService` — ReplicaApp guard (fixed)
- `forge/api/internal/services/apphosting/service.go:681` `TriggerDeploy` — `Image:""` admission `service.go:701`
- `forge/api/internal/services/deployment/service.go:42` strategies enum
- `forge/api/internal/services/deployment/rollout.go:42` `StartRollout` dispatch
- `forge/api/internal/services/deployment/rollout.go:71` `validateHealthGateTarget` — loopback allowance
- `forge/api/internal/services/deployment/rollout.go:92` `applyRolloutRequest` — replica clobber site
- `forge/api/internal/services/deployment/execution.go:22` `ExecuteDeployment` lease-claimed DAG
- `forge/api/internal/services/deployment/execution.go:249` `RuntimeExecutor` honest failure iface
- `forge/api/internal/services/deployment/execution.go:313` `executeScaleUpStep` boolean verification
- `forge/api/internal/services/deployment/healthgate.go:12` `CheckHealth` http with `resolveNodeHost:62`
- `forge/api/internal/services/deployment/healthgate.go:79` `WaitForHealthGate` threshold loop
- `forge/api/internal/services/deployment/revisions.go:58` `configHash` SHA12
- `forge/api/internal/services/deployment/revisions.go:82` `CreateRevision` race site
- `forge/api/internal/services/deployment/revisions.go:225` `CompareRevisions` 5-field diff
- `forge/api/internal/services/deployment/steps.go:48` `stepsForStrategy` — strategy→DAG
- `forge/api/internal/services/compose/service.go:240` `ValidateCompose` + `service.go:352` `interpolateEnv`
- `forge/api/internal/services/compose/service.go:427` `ValidateComposeSecurity` deny-list
- `forge/api/internal/services/compose/service.go:594` `checkVolumesSecurity` sensitive paths
- `forge/api/internal/services/compose/service.go:663` `ValidateHostMountWithAllowlist` allowlist gate (unplumbed)
- `forge/api/internal/services/compose/lifecycle.go:188` `WaitForHealthy` ps semantics + restart-count guard
- `forge/api/internal/services/compose/lifecycle.go:249` `DeployComposeStack` placement+reservation+daemon+health
- `forge/api/internal/services/compose/lifecycle.go:476` `UpdateComposeStack` hash+mapsEqual dedupe
- `forge/api/internal/services/compose/lifecycle.go:570` `DeleteComposeStack`
- `forge/api/internal/services/compose/lifecycle.go:781` `RestartStack` `Stop+Start`
- `forge/api/internal/services/compose/lifecycle.go:844` `rollbackStack` with health
- `forge/api/internal/services/compose/parser.go:245` `ParseComposeYAML` compose-go loader
- `forge/api/internal/services/compose/gitops.go:301` `DeployFromGit` `Update` vs `Create` bug `gitops.go:384`
- `forge/api/internal/services/compose/gitops.go:431` `CheckForUpdates` drift preview
- `forge/api/internal/services/compose/gitops.go:1151` `HandleWebhook` HMAC+DeliveryID dedup
- `forge/api/internal/services/replicamanager/service.go:100` `CreateApp` Uncloud-inspired
- `forge/api/internal/services/replicamanager/service.go:195` `deployReplicas` generation fencing
- `forge/api/internal/services/replicamanager/service.go:361` `ScaleApp` + `appLocks[64]`
- `forge/api/internal/services/zerodowntime/service.go:102` `CreateRelease` + `service.go:251` `doHealthCheck` via `PrimaryAllocation`
- `forge/api/internal/services/autoscaler/service.go:251` `EvaluateServer` vertical resize
- `forge/api/internal/http/handlers_apphosting.go:55` `registerAppHostingRoutes` `tenantAccess`
- `forge/api/internal/http/handlers_apphosting.go:450` start, `475` stop (desired-only), `500` restart via `SendPower`
- `forge/api/internal/http/handlers_deployment.go:17` `registerDeploymentRoutes` `/admin/deployments`
- `forge/api/internal/http/handlers_compose.go:66` `registerComposeRoutes` `470` restart wired but web not exported
- `forge/api/internal/http/handlers_revisions.go:9` `registerRevisionRoutes` `/revisions`+`compare`
- `forge/api/internal/http/handlers_docker.go:25` `registerDockerRoutes` containers/images/networks/volumes + prune
- `forge/api/internal/store/store_apphosting.go:11` `Application/AppService` models
- `forge/api/internal/store/store_apphosting.go:211` `ListApplications` org scoping
- `forge/api/internal/store/store_apphosting.go:374` `DeleteApplication` unconstrained
- `beacon/internal/server/compose.go:77` `validateComposePolicy` strict schema
- `beacon/internal/server/compose.go:213` `shortFormHostPort` privileged-port extraction
- `beacon/internal/server/compose.go:226` `validateComposeVolumes` absolute bind rejection
- `beacon/internal/server/compose.go:344` `handleComposeDeploy` `docker compose up -d`
- `beacon/internal/server/compose.go:506` `handleComposeRestart`
- `beacon/internal/server/compose.go:604` `handleComposeStatus` `ps --format json`
- `forge/web/lib/api/compose.ts:34` `validateCompose` `compose.ts:89` `deployComposeStack`
- `forge/web/lib/api/apps.ts:263` `restartApp` `apps.ts:330` `redeployComposeStack`

---

*Generated by subagent 02 (final-parity, parallel). No product files modified. Evidence: SOURCE_VERIFIED file:line unless marked.*

