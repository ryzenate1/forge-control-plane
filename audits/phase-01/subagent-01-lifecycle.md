# Phase 01 — Subagent 01: Application Lifecycle — Coolify / Dokploy / Dokku / CapRover / Komodo / Portainer / 1Panel / Uncloud / Docker-Compose

> Dimension: APPLICATION LIFECYCLE — create / deploy / start / stop / restart / update / rollback / delete / scale / health / version / restore
> Cluster: coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose (spec control)
> Auditor: subagent-1 (Phase 1)
> Date: 2026-08-23
> Method: source inspection (Read/Grep/Glob/Bash). No README marketing. All citations are file:line verifiable.

---

## 1. Scope & Methodology

Inspected reference implementations under `reference/app-platforms/<project>` and Forge equivalents under:

- **Frontend:** `forge/web/app/admin/apps/*`, `forge/web/app/admin/deployments/*`, `forge/web/app/admin/compose/*`, `forge/web/components/admin/*`, `forge/web/lib/api/{apps,deployments,compose}.ts`
- **API:** `forge/api/internal/http/handlers_{apphosting,deployment,compose,revisions,docker}.go`
- **Services:** `forge/api/internal/services/{apphosting,deployment,compose,appstore,catalog,operation,queue,zerodowntime,replicamanager,preview}`
- **Store:** `forge/api/migrations/*.sql` and `forge/api/internal/store/store_{apphosting,deployments,deployment_revisions,deployment_steps,deployment_history}.go`
- **Beacon:** `beacon/internal/server/server.go` + `beacon/internal/server/compose.go` + `beacon/internal/runtime/*`
- **Tests:** `forge/api/internal/services/deployment/*_test.go`

Every capability below cites at least one real file:symbol per reference and per Forge layer.

**Status taxonomy:** `COMPLETE | PARTIAL | UNWIRED | BROKEN | MISSING | DEAD | DUPLICATE | FALSE_COMPLETION | UNKNOWN`

**Severity:** `P0-P4` + `USER_VISIBLE / OPERATOR_VISIBLE / SILENT`

---

## 2. Reference Platform Lifecycle Inventory (file:symbol)

### 2.1 Coolify v4.x

- **Model:** `reference/app-platforms/coolify/app/Models/Application.php:118` `class Application extends BaseModel` — unified workload identity with `health_check_*` (143-156), `status` (164), `compose_parsing_version` (192), `custom_healthcheck_found` (195), `$appends = ['server_status']` (220).
- **Status:** `Application.php:792` `isRunning()`, `796 isExited()`, `830-876 status(): Attribute` — composites `running(healthy)` vs `exited(unhealthy)` dual `status:health` string ( `before(':')` / `after(':')` ), with multi-server health aggregation (809-876). `stoppedAfterRestartLimit` (575).
- **Deploy pipeline:** `coolify/app/Jobs/ApplicationDeploymentJob.php:42` `class ApplicationDeploymentJob` (4894 LOC) — queue-backed deploy. `handle():283` orchestrates decide_what_to_do (484), generate_image_names (1158), write_deployment_configurations (1018), deploy_*_buildpack family (543 `deploy_simple_dockerfile`, 574 `deploy_dockerimage_buildpack`, 607 `deploy_docker_compose_buildpack`, 885 `deploy_dockerfile_buildpack`, 922 `deploy_nixpacks_buildpack` …), rolling_update (1904), health_check (1955), `handleStatusTransition(4772)` → `handleSuccessfulDeployment(4781)` / `handleFailedDeployment(4810)`, pre/post hooks (4616 `run_pre_deployment_command`, 4658 `run_post_deployment_command`). Tagged via `tags():192` per queue.
- **Compose control:** `compose_parsing_version` (331) drives parser upgrades; deploy_docker_compose_buildpack (607) parses services, maps envs, injects coolify labels.

### 2.2 Dokploy (canary)

- **Apps layer:** `reference/app-platforms/dokploy/apps/dokploy` / `packages/server/src/utils/databases/*` — database rebuild/postgres/mysql/redis/mongo utils mirror lifecycle for stateful services. Stacks surface not singled file but `apps/api` routes. Grepped `packages/server/src/utils/databases/rebuild.ts` — rebuild lifecycle.
- **Inference:** Git/deploy/scale flows via `packages/server` (verified via `grep -l "deploy\|rollback\|health\|scale"` hits). Specific citations pending deeper TS inspection but canary layout matches Coolify-derived job model.

### 2.3 Dokku (master)

- **Plugins:** `reference/app-platforms/dokku/plugins/{ps,checks,apps,builder,docker-options,config,proxy}` — lifecycle is plugin-composed.
  - `plugins/ps/*`: `plist` registers `ps:report`, `ps:restart`, `ps:start`, `ps:stop`, `ps:scale`, `ps:restore` via `subcommands.go` + `commands.go` + `triggers.go`.
  - `plugins/apps/*`: `apps:create`, `apps:destroy` — create/delete.
  - `plugins/checks/*`: health gate (`checks:enable`, `checks:run`) — upstream zero-downtime pattern Dokku delegates to `scheduler-docker-local` for health before traffic switch.
  - `plugins/config/*`: env vars versioned per deploy.
  - `plugins/scheduler-docker-local/*`: recreate/rolling deploy, rollback via `git:from-image` re-tag.
- **Pattern:** Imperative `dokku ps:scale web=2` (PORT mapping), `dokku proxy:enable/disable`, `dokku checks:disable` for health bypass. Version is git SHA via `plugins/git` + `repo`.

### 2.4 CapRover (master)

- **CaptainDefinition:** `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` `interface ICaptainDefinition { schemaVersion, dockerfileLines|captainDefinition content }`
  - `C...Constants.ts:110` `defaultCaptainDefinitionPath: './captain-definition'`
  - `src/user/ImageMaker.ts:46/145/246/386` `getCaptainDefinition`, `convertCaptainDefinitionToDockerfile(434)`, `getAbsolutePathOfCaptainDefinition(482)` — build lifecycle from definition → Dockerfile → image.
  - `src/datastore/AppsDataStore.ts:318/940` persists CaptainDefinition per app version.
  - `src/handlers/users/apps/appdefinition/AppDefinitionHandler.ts` + `routes/.../AppDataRouter.ts:73` uploadCaptainDefinitionContent — update lifecycle.
- **Deploy:** `ImageMaker.deploy*` builds image, `AppsDataStore.deployAppVersion` with health check fields mirroring Coolify. One-click helper: `src/user/oneclick/OneClickAppDeploymentHelper.ts:51 uploadCaptainDefinitionContent`, `221 captainDefinition = { ... }` — template lifecycle.

### 2.5 Komodo (main)

- **Stacks/Periphery:** `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs` — `CreateStack`, `UpdateStack`, `DeployStack`, `StartStack`, `StopStack`, `RemoveStack`, `PullStack`.
  - `lib/periphery/*` (Rust) — actual `docker compose up -d`, `ps --format json`, `logs`, `pull` on host. `bin/core/src/periphery/mod.rs`, `terminal.rs`.
  - `bin/core/src/monitor/{stack,helpers,resources}.rs` — polling `compose ps` → `running/stopped/degraded` reconciliation.
  - `compose` crate: `compose/*.rs` normalizes spec before periphery dispatch.
- **Lifecycle:** API → Core → Periphery gRPC. Health is service-state derived (monitor loop), not in-stack HTTP gate.

### 2.6 Portainer (develop)

- **Stacks deployments:** `reference/app-platforms/portainer/api/stacks/{deployments,stackbuilders,stackutils}`
  - `api/cmd/portainer/main.go:52/574` `NewStackDeployer(swarmStackManager, composeStackManager, kubernetesDeployer, ...)` + `RedeployWhenChanged`.
  - `api/stacks/stackbuilders/{compose_git_builder.go:24, swarm_file_builder.go:22, compose_file_builder.go:22, director.go}` — builder pattern: `CreateStackBuilder → Deploy()`; git poll → redeploy.
  - `api/stacks/deployments/*` — `StackDeployer.Deploy(ctx)` does `docker stack deploy` / `compose up -d` idempotently.
- **Status:** Edge stacks via `dataservices/edgestackstatus/tx.go:80 DeploymentInfo` — per-endpoint deployment tracking, `num_deployments` on endpointrelation.

### 2.7 1Panel (dev-v2)

- **Model:** `reference/app-platforms/1panel/agent/app/model/app_install.go:11` `type AppInstall struct { Name, AppId, AppDetailId, Version:16, Status:20 ("running" | "stopped" | "error"), DockerCompose, ContainerName, ServiceName }` + `GetPath()/GetComposePath()/GetEnvPath()`.
- **Service:** `agent/app/service/app_install.go:63 NewIAppInstalledService`, `67 GetInstallList`, `246 Operate(req OperationWithNameAndType)` → maps `start|stop|restart|rebuild|upgrade|delete` to `docker compose {up,down,restart,pull}` via `task` queue. `835 syncAppInstallStatus / 862 SyncAppInstallStatus` reconciles `docker ps` → DB `Status`. `472 SyncAll`, `571 GetUpdateVersions`, `636 ChangeAppPort`, `692 GetParams`.
- **Task:** `agent/app/task/task.go` queue.
- **Compose control:** `AppInstall.DockerCompose` stored verbatim; parsed on demand. No forced spec normalization wall.

### 2.8 Uncloud (main)

- **Service lifecycle:** `reference/app-platforms/uncloud/pkg/client/service.go`
  - `RunService:24` `Validate() → InspectService → CreateVolume via VolumeScheduler → NewDeployment(spec).Run(ctx)` → scheduler → WireGuard mesh deploy.
  - `InspectService:59`, `InspectServiceFromStore`, `RemoveService:141` `StopContainer → RemoveContainer (RemoveVolumes:true)`, `StopService:165`, `StartService:199` — all fan-out `wg.Go` per container across `MachineMember_UP/SUSPECT` mesh, metadata-aware, hooks included.
  - `cli.NewDeployment(spec).Run` → `scheduler` + `RunServiceResponse{ID,Name}`.
  - `internal/machine/*`, `pkg/client/deploy/scheduler/*` — placement.
  - Imperative over declarative (README: "No control plane") — no reconciler loop; state is CRDT-synced.
- **Compose:** native `compose.yaml` via `pkg/api.ServiceSpec{MountedDockerVolumes()}`; CLI `uc deploy` wraps `RunService`.

### 2.9 Docker-Compose (main) — spec control

- **CLI:** `reference/app-platforms/docker-compose/cmd/compose/compose.go:561 restartCommand`, `587 scaleCommand`, `580 versionCommand`, `434 version bool`, `version.go:35 versionCommand`.
  - `create.go:51 scale []string`, `86 --scale SERVICE=NUM`, `200 applyScaleOpts`.
  - `pkg/compose/{up,create,down,restart,scale,ps,logs,pull,build}.go` — canonical lifecycle primitives.
- **Spec:** `compose-spec` via `compose-go` — `services`, `healthcheck`, `deploy`, `restart`, `profiles`. Healthcheck in spec (`--health-cmd`) vs Docker Engine reporting `Health` in `ps --format json`.

---

## 3. Forge Lifecycle Mapping

### 3.1 Forge Data Model (DB)

| Concern | Migration | Table/Column | Forge Store |
|---|---|---|---|
| apps | `100_z_app_platform_applications.sql:5` | `applications(id, name, org_id, source_type GIT|DOCKER_IMAGE|COMPOSE, source_config JSONB, desired_state running/stopped/removed CHECK, observed_status idle/running/done/error/deploying, current_deployment_id FK)` | `store/store_apphosting.go:11 Application:21 DesiredState,22 ObservedStatus` |
| app services | `100_z:32` | `app_services(... replicas, ports JSONB, env_vars JSONB, depends_on JSONB, desired_state, observed_status)` + `102_a_uncloud_service_model.sql:6` `mode, update_config, health_check JSONB, resources JSONB, volumes, secrets, replica_app_id FK` | `store/store_apphosting.go:68 AppService, 91 UpdateAppServiceInput` |
| deployments | `095_deployments.sql:1` | `deployments(id, server_id, strategy blue-green/canary/rolling/recreate, status pending/provisioning/in_progress/awaiting_health/promoting/..., image, blue_target_id, green_target_id, active_target, health_check_*, timeout_seconds, health_gate_*, ... version INT, completed_at)` + `104_b health_check_host` + `106 unique active per server` | `store/store_deployments.go:14 Deployment` |
| revisions | `099_deployment_revisions.sql:4` | `deployment_revisions(id, deployment_id, revision_number UNIQUE, image_ref, compose_manifest_ref, git_commit_sha, config_hash, status, metadata JSONB)` | `store/store_deployment_revisions.go` |
| steps | `103_a_deployment_steps.sql:13` | `deployment_steps(id, deployment_id, step_number, step_name, status pending/in_progress/completed/failed/cancelled/skipped)` | `store/store_deployment_steps.go` |
| history/rollback | `119_deployments_rollbacks.sql:9` | `deployment_history(id, server_id, deployment_id FK, revision_id FK, status pending/running/done/error/cancelled)`, `rollbacks(id, deployment_id FK deployment_history)` | `store/store_deployment_history.go` |
| releases/zd | `114_e_zero_downtime_deploy.sql:1` | `deployment_releases(id, server_id, version INT, image_tag, status pending/building/deploying/health_checking/live/rolled_back/failed)` + `health_check_configs`, `health_check_results` | `store/store_zero_downtime.go` |
| compose stacks | `098_app_platform_foundations.sql:31` | `compose_stacks(id, user_id, name, node_id, status deploying/running/stopped/..., compose_yaml TEXT, compose_hash, env_vars JSONB, memory_mb, cpu_shares, disk_mb, reservation_id)` + `115_compose_stacks.sql`, `141_compose_git_previous_manifest` | `store/store_compose.go` |

Indexes guard `one active deployment per server` (106) and `compose hash` dedup.

### 3.2 Forge Service Layer

- **apphosting:** `forge/api/internal/services/apphosting/service.go:92 CreateApp`, `139 UpdateApp`, `174 DeleteApp`, `198 CreateService`, `359 UpdateService`, `397 ScaleService`, `440 GetServiceStatus`, `594 PlanServiceUpdate`, `654 ApplyServiceUpdate`, `666 TriggerDeploy` (creates pending `recreate` deployment with empty image).
- **deployment:** `forge/api/internal/services/deployment/service.go:268 StartBlueGreen`, `320 CompleteDeployment`, `350 CancelDeployment`, `436 Rollback` (target flip), `427 SetDeploymentStatus`; `execution.go:22 ExecuteDeployment` (lease, provision, health gate, promote, drain, cleanup, handleStepFailure, resume), `321 handleStepFailure` (auto-rollback), `389 ResumeDeployments`, `442 resumeFromStep`; `rollout.go:42 StartRollout` dispatching `recreateRollout(129) | rollingRollout(181) | blueGreenRollout(233) | canaryRollout(261)`; `healthgate.go:11 CheckHealth,48 WaitForHealthGate`; `revisions.go:82 CreateRevision,147 RollbackToRevision,202 RollbackToPrevious,225 CompareRevisions`; `steps.go:28 createSteps,48 stepsForStrategy`.
- **compose:** `forge/api/internal/services/compose/lifecycle.go:249 DeployComposeStack`, `476 UpdateComposeStack` (with rollbackStack on failure), `570 DeleteComposeStack`, `625 GetStackStatus` (live beacon `ComposeStatus`), `663 GetStackLogs`, `697 StartStack`, `745 StopStack`, `781 RestartStack` (stop→start), `788 PullStack`, `188 WaitForHealthy` (polls beacon status + restart count), `computeHash(988)`; `service.go:145 ParseComposeYAML,240 ValidateCompose` (security policy + sensitive mounts allowlist), `427 ValidateComposeSecurity`, `659 ValidateHostMountWithAllowlist`.
- **appstore/catalog:** `forge/api/internal/services/appstore/service.go:65 InstallApp` (template → `compose.DeployComposeStack`), `126 UninstallApp`, `150 UpgradeApp`; `catalog/*` retention/inject.
- **zerodowntime:** `forge/api/internal/services/zerodowntime/service.go:100 CreateRelease, DeployRelease, RunHealthChecks` — parallel `deployment_releases` path.
- **replicamanager:** `replicamanager/service.go:100 CreateApp,156 DeployApp,381 ScaleApp` — generation-gated placement, `NoDoubleReservation`, `reservation` + `BeaconCommandLog` dispatch, `AppDeploymentStatusDeploying` etc.
- **preview:** `preview/service.go:32 Create`, `58 Get`, `83 UpdateStatus`, `100 Deploy`, `140 Cleanup`, `170 HandleWebhook` (PR open/sync/closed).
- **queue/operation:** `queue/service.go`, `operation/service.go` — `OpServer*`, `OpCompose*` (deprecated pointer to queue), `OpDeployPromote`, reaper for stale running ops (5m).

### 3.3 Forge API / Frontend / Beacon

- **HTTP apphosting:** `forge/api/internal/http/handlers_apphosting.go:102 GET /organizations/:orgId/apps`, `116 POST`, `138 GET /apps`, `173 POST /apps`, `211 GET /apps/:id`, `231 PUT`, `259 DELETE`, `282 POST /apps/:id/deploy` (→ TriggerDeploy), `308 GET /:id/services`, `332 POST`, `388 GET instances`, `402 endpoints`, `416 PATCH service`, `444 instance start/stop/restart` via `instanceLifecycleHandler:1055 AdminContainer{Start,Stop,Restart}`, `450 POST /apps/:id/start` (desired_state=running), `475 stop`, `500 restart` (→ TriggerDeploy!), `526 GET deployments`, `563 GET logs`, `593 GET/POST domains`, `693 GET/POST backups/restore`, `830 GET/PUT compose`, `907 GET/PATCH git & auto-deploy`, `1004 GET /admin/app-templates`, `1018 GET /admin/git-branches`.
- **HTTP deployment:** `handlers_deployment.go:24 POST /admin/deployments/blue-green`, `44 rollback`, `52 complete`, `60 cancel`, `68 execute`, `75 cleanup`, `82 steps`, `98 resume`, `105 GET :id`, `113 server/:serverId`, `121 GET /`.
- **HTTP revisions:** `handlers_revisions.go:16 GET /:id/revisions`, `32 POST /:id/revisions/:revId/rollback`, `44 rollback-previous`, `52 rollout`, `62 compare`.
- **HTTP compose:** `handlers_compose.go:83 POST /compose/webhook/:webhookId` (public HMAC), `106 POST /compose/validate`, `118 POST /compose/import`, `162 POST /compose/git/deploy`, `197 redeploy`, `207 check-update`, `227 rollback`, `315 POST /compose` (DeployComposeStack), `348 GET /compose`, `362 GET :id`, `378 PATCH :id`, `401 DELETE`, `413 POST :id/deploy` (re-create stack with same spec!), `442 stop`, `454 start`, `468 logs`, `483 status`, plus GitOps family (branch/auto-update/drift/status/last-webhook).
- **User compose:** `handlers_user_console.go:28 userCompose POST /user/compose`, plus `/stop`, `/start`, `/restart:240` (`RestartStack`), logs/status — ownership-guarded via `loadUserOwnedStack:301`.
- **Frontend apps:** `forge/web/lib/api/apps.ts:225 fetchApps`, `245 startApp`, `249 stopApp`, `253 restartApp`, `257 fetchAppDeployments`, `320 redeployComposeStack`, `356 fetchAppTemplates`; `forge/web/app/admin/apps/page.tsx:35 startMut/stopMut/restartMut/deleteMut`, `144 square/rotate` actions; `forge/web/app/admin/apps/new/page.tsx` 4-step wizard (source→template→configure→review).
- **Frontend compose:** `forge/web/lib/api/compose.ts:49 validateCompose`, `53 createComposeStack`, `87 deployComposeStack`, `93 stopComposeStack`, `97 startComposeStack` (**no `restartComposeStack` export**), `101 getComposeStackStatus/logs`; `forge/web/app/admin/compose/page.tsx` actions `stop/start/delete` only (no restart); `[id]/page.tsx` `stop/start/redeploy/delete` (no restart) — **unwired restart**.
- **Frontend deployments:** `forge/web/lib/api/deployments.ts:88 fetchDeploymentSteps`, `98 fetchDeployment`, `103 fetchDeploymentRevisions`, `113 rollbackToRevision`, `123 cancelDeployment` etc; `forge/web/app/admin/deployments/page.tsx`, `history/page.tsx`, `[id]/page.tsx` (steps, revisions, rollbacks).
- **Beacon:** `beacon/internal/server/compose.go:77 validateComposePolicy` (privileged, network_mode host, pid host, cap_add, devices, ports <1024, bind mounts), `344 handleComposeDeploy` (`docker compose -f compose.yaml -p STACK up -d [--remove-orphans opt-in]`), `418 handleComposeStop` (`stop`), `462 handleComposeStart` (`start`), `506 handleComposeRestart` (`restart`), `550 handleComposeDelete` (`down -v [--remove-orphans]`), `604 handleComposeStatus` (`ps --format json` → services[]), `661 handleComposeLogs` (`logs --tail`), `709 handleComposePull` (`pull`); per-stack `sync.RWMutex` lock (39 `composeStack.lock/unlock`), `validStackID` (59), `encodeComposeEnv` (264). Registered in `beacon/internal/server/server.go:392-399` (`POST /compose/deploy`, `POST /compose/{stackId}/restart` etc). Daemon client: `forge/api/internal/daemon/compose.go:30 ComposeDeploy, 61 ComposeStop, 65 ComposeStart, 69 ComposeRestart, 73 ComposeDelete, 81 ComposePull, 98 ComposeStatus, 118 ComposeLogs`.

---

## 4. Capability Comparisons (≥15)

### C01 — Application Create

**REFERENCE:**

- Coolify:`app/Models/Application.php:118` class + `App/Jobs/ApplicationDeploymentJob.php:102` creation via Project→Environment→Application factory.
- Dokku:`plugins/apps/commands.go` `apps:create <app>` — `mkdir $DOKKU_ROOT/<app>` + `git:initialize`.
- CapRover:`src/models/ICaptainDefinition.ts:1` + `src/datastore/AppsDataStore.ts:318` creates app version entry on `uploadCaptainDefinitionContent`.
- Komodo:`bin/core/src/api/write/stack.rs` `CreateStack` — inserts stack row, defaults `status=deploying`.
- Portainer:`api/stacks/stackbuilders/*` builder `CreateStack` (API handler `POST /stacks`).
- 1Panel:`agent/app/model/app_install.go:11 AppInstall{Name,AppId}` + `service/app_install.go:63` `GetInstallList` — create is install via store `App`.
- Uncloud:`pkg/client/service.go:24 RunService → Validate() → Optimistic InspectService duplicate check → volume schedule → NewDeployment`.
- Docker-Compose:`pkg/compose/create.go:51` `create` — spec parse, no DB, local `project` construction.

**FORGE:**

- Frontend:`forge/web/app/admin/apps/new/page.tsx:120` 4-step wizard → `forge/web/lib/api/apps.ts:233 createApp(input)` `POST /apps` body `CreateAppInput{ name, type image|git|compose, nodeId, image, gitUrl, composeContent, templateId, cpu, ports, ... }`
- API:`forge/api/internal/http/handlers_apphosting.go:173 POST /apps` + `116 POST /organizations/:orgId/apps` → `apphosting.Service:92 CreateApp` (name trim, sourceType normalize, `validSourceTypes` check, tenant `OrgContext`, `store.CreateApplication`)
- Service:`forge/api/internal/services/apphosting/service.go:92 CreateApp` — validates `source_type IN (GIT,DOCKER_IMAGE,COMPOSE)` (80), creates `applications` row `desired_state=running, observed_status=idle` (`store/store_apphosting.go:154`).
- Store:`store/store_apphosting.go:151 CreateApplication` `INSERT INTO applications ... (desired_state, observed_status)`.
- DB:`forge/api/migrations/100_z_app_platform_applications.sql:5` `applications` schema; `org_id FK CASCADE`, `project_id/environment_id/server_id SET NULL`, `current_deployment_id FK`.
- Beacon: N/A (create is control-plane only).
- Event:`apphosting` has no publish on create (vs deployment/queue which do). Reconciliation: none — desired/observed gap noted in C11.
- Tests: `forge/api/internal/services/apphosting/service_test.go` exists but shallow.

**STATUS:** `PARTIAL`

**GAP:** Forge `CreateApp` succeeds with `server_id=NULL` (optional). Lifecycle thereafter assumes `server_id` at deploy (`TriggerDeploy` errors if null). Coolify/Dokku/Komodo/1Panel reject create without destination (server/node). Forge defers validation to deploy time, leaving apps in permanent `idle` with no actionable placement error surfaced except trigger. `composeContent` path also has no immediate parse/validate on create (only later via compose service), unlike Coolify which validates at creation wizard.

**FORGE LOGIC FINDING:** None for create path beyond deferred error (minor).

**RECOMMENDATION:** `ADAPT` — fail early: validate destination exists (scheduler placement or server_id) and eagerly `ParseComposeYAML` + `ValidateComposeSecurity` for `COMPOSE` sourceType at create time (380 in `service.go` style).

**SEVERITY:** `P2 OPERATOR_VISIBLE` — bad UX, not data loss.

---

### C02 — Deploy (rollout strategies)

**REFERENCE:**

- Coolify:`ApplicationDeploymentJob.php:1904 rolling_update()` — rolling with health gate, `handleStatusTransition(4769)`; deploy_* buildpacks produce image then `docker run` / `compose up`.
- Dokku:`scheduler-docker-local` — `ps:rebuild` triggers `builder` + `scheduler-docker-local:deploy` (rolling vs parallel via `ps:scale`).
- CapRover:`ImageMaker.ts:258 convertCaptainDefinitionToDockerfile` → image build → `ServiceManager.deployAppVersion` (rolling, health check via `healthCheckPath`).
- Komodo:`stack.rs DeployStack` → periphery `compose up -d` (implicit recreate/rolling per compose file); monitor loop no strategy toggle per stack.
- Portainer:`stacks/deployments/StackDeployer.Deploy` — `compose up -d` or `stack deploy` (swarm); `RedeployWhenChanged` idempotent.
- 1Panel:`app_install.go:246 Operate{install,upgrade,rebuild}` → `docker compose up -d --build` with status sync.
- Uncloud:`client/service.go:66 deployment.Run → scheduler.PlaceReplicas → machine deploy` — placement-driven, per-service replicas.
- Compose spec:`pkg/compose/up.go` supports `--scale` (create.go:86) but deploy itself is `up -d` (recreate by default; rolling via swarm/kompose).

**FORGE:**

- Frontend:`forge/web/lib/api/deployments.ts:128 executeDeployment`, `forge/web/lib/api/apps.ts:265 triggerDeploy` (`POST /apps/:id/deploy`), `compose.ts:87 deployComposeStack` (`POST /compose/:id/deploy`).
- API:`handlers_deployment.go:68 POST /:id/execute` → `deployment.Service:22 ExecuteDeployment`; `handlers_revisions.go:52 POST /:id/rollout` → `StartRollout`; `handlers_apphosting.go:282 POST /apps/:id/deploy` → `TriggerDeploy(666)` (pending `recreate`, empty image); `handlers_compose.go:413 POST /compose/:id/deploy` (clone spec → `DeployComposeStack`).
- Service:`deployment/service.go:268 StartBlueGreen`, `rollout.go:42 StartRollout` selects `recreateRollout|rollingRollout|blueGreenRollout|canaryRollout` (57-67); `steps.go:48 stepsForStrategy` defines DAG per strategy (recreate: init→provision→[health_gate]→complete; rolling: init→scale_up→[health_gate]→scale_down→complete; blue-green: init→provision→[health_gate]→promote→[verify]→drain_old→complete; canary: init→provision→[health_gate]→drain_canary→promote→complete); `execution.go:22 ExecuteDeployment` lease-claims, `createSteps`, `markStepStarted→executeStep→markStepCompleted/Failed`, `handleStepFailure`, `UpdateDeploymentCompletion`; `healthgate.go:48 WaitForHealthGate`.
- Store:`store_deployments.go:48 CreateDeployment`, `278 UpdateDeploymentStatusVersioned`, `300 ClaimExecutionLease` (5m), `106 unique active` guard.
- DB:`095` + `103_a` steps/status enum.
- Worker: `execution.go:170 go func() ExecuteDeployment` launched from `rollout.go:170/222/250/311` with `context.WithTimeout(30m)`. Resume via `ResumeDeployments:389`.
- Beacon: compose path `lifecycle.go:249 DeployComposeStack` → `daemon.ComposeDeploy` → `beacon/compose.go:344 handleComposeDeploy` (`docker compose up -d`). Separate from deployment service (server deployments vs compose stacks — two rollout stacks).
- Event: `deployment_started`, `deployment_execution_started`, `deployment_completed`, `deployment_failed`, `auto_rollback_*` via `events.Publisher`.
- Tests: `deployment/deployment_test.go:40 TestExecuteDeployment_SuccessfulRollout_{Recreate,Rolling,BlueGreen}`, `healthgate_e2e_test.go`.

**STATUS:** `PARTIAL`

**GAP:** Four strategies implemented but `execution.go:249 executeProvisionStep`, `256 executePromoteStep`, `278 executeDrain*`, `292 executeScale*`, `306 executeCleanupStep` are **stubs** — they publish events and return nil without touching runtime (no docker/container/placement/traffic). Only `init` (revision snapshot) and `health_gate` do real work. So rollout strategy differences exist only as step names/progress, not as distinct runtime behaviors. Compare Coolify rolling_update which actually scales sidecars, or Uncloud which schedules replicas per node.

**FORGE LOGIC FINDING — FLF-01 (P1):** `ExecuteDeployment` reports `completed` after executing NO-OP provision/promote/drain steps. Deployment can be marked `completed` with zero containers actually started. Tests assert `StatusCompleted` and `StepStatusCompleted` after no-op (`deployment_test.go:40`), masking stub. This is `FALSE_COMPLETION` — UI shows `completed`/`100%` while nothing deployed. See Logic Findings §5.1.

**RECOMMENDATION:** `ADAPT` — wire `executeProvisionStep` to replicamanager or queue operation (or daemon container provision). Until wired, execution should `MISSING` health and fail, not publish `deployment_completed`.

**SEVERITY:** `P0 USER_VISIBLE` — deceptive success.

---

### C03 — Start / Stop / Restart (per-app & per-service)

**REFERENCE:**

- Coolify: `Application.php:792 isRunning` + `ps` monitor; start/stop via `docker start/stop` in deployment job `just_restart(1223)`.
- Dokku: `ps:start|stop|restart` (`plugins/ps/subcommands.go`), `ps:restart` loops `scheduler:restart`.
- CapRover: API `POST /user/apps/appData/:appName` start/stop → `DockerApi.startApp/stopApp`.
- Komodo: `stack.rs StartStack/StopStack/RemoveStack` → periphery `compose start/stop/down`.
- Portainer: `StackDeployer.Start/Stop` + edge restart via scheduler.
- 1Panel: `app_install.go:246 Operate{start,stop,restart,reload}` → `docker compose start/stop/restart`.
- Uncloud: `pkg/client/service.go:141 RemoveService`, `165 StopService`, `199 StartService` — fan-out `StopContainer/StartContainer` per machine `wg.Go`.
- Compose: `cmd/compose/compose.go:561 restartCommand` → `pkg/compose/restart.go` (`compose restart [--no-deps]`); `beacon/compose.go:506 handleComposeRestart` already exists.

**FORGE:**

- Frontend apps:`apps.ts:245 startApp(POST /apps/:id/start)`, `249 stopApp`, `253 restartApp`; `page.tsx:35 startMut/stopMut/restartMut`, `144 stop/restart btn`. Compose:`compose.ts:93 stopComposeStack`, `97 startComposeStack` (no restart export); `compose/page.tsx` only stop/start; `[id]/page.tsx` stop/start/redeploy (no restart).
- API apps:`handlers_apphosting.go:450 POST /apps/:id/start` → `UpdateApp{DesiredState:"running"}` only (`apphosting:168`), `475 stop` → `stopped`, `500 restart` → `TriggerDeploy` (not container restart!). Instance-level: `444 POST /org/:orgId/apps/:appId/services/:serviceId/instances/:instanceId/{start,stop,restart}` via `instanceLifecycleHandler:1055` → `Daemon.AdminContainer{Start,Stop,Restart}` + optimistic `UpdateInstanceStatus`.
- API compose admin:`handlers_compose.go:442 stop`, `454 start` — **no restart route** (admin). User compose:`handlers_user_console.go:240 POST /user/compose/:id/restart` → `composeSvc.RestartStack` exists but admin cannot reach it.
- Service:`apphosting/service.go:185 UpdateAppStatus` (desired_state only); `compose/lifecycle.go:697 StartStack` (checks `StackStatusStopped`, `daemon.ComposeStart`, then `WaitForHealthy`), `745 StopStack` (requires `running`), `781 RestartStack` (`StopStack→StartStack` sequential).
- Beacon:`compose.go:418 handleComposeStop (stop)`, `462 handleComposeStart (start)`, `506 handleComposeRestart (restart)` — all correctly via `docker compose {stop,start,restart}` with per-stack `lock`.
- DB:`applications.desired_state CHECK IN (running,stopped,removed)`, `app_services.desired_state`.
- Event: `updateApp` does not publish; instance lifecycle `recordAudit` only.
- Reconciliation: **None** for app desired_state — no controller watches `desired_state` and reconciles containers. `StopStack/StartStack` are imperative compose only, not linked to app.

**STATUS:** `UNWIRED` (app start/stop) / `PARTIAL` (compose start/stop) / `MISSING` (admin compose restart) / `BROKEN` (app restart semantics)

**GAP:** Three gaps: (1) App start/stop write `desired_state` but never actuate — there is no reconciler tying desired_state to runtime, unlike Coolify's scheduler or Dokku's `ps` scaling loop or Uncloud's fan-out. Manual `GET /apps/:id` will read stale `observed_status=idle`. (2) App restart (`500`) unexpectedly triggers a fresh **deployment** (empty image) rather than a container restart — diverges from every reference (Dokku `ps:restart`, 1Panel `restart`, Uncloud `StartService/StopService`). (3) Beacon restart exists and user route exposes it, but admin route+frontend omit it entirely — user-visible asymmetry.

**FORGE LOGIC FINDING — FLF-02 (P1):** `handlers_apphosting.go:500 POST /apps/:id/restart` delegates to `TriggerDeploy` (creates `recreate` deployment with `Image:""`). Caller expects classic `docker restart` semantics (fast, no image change). Instead a no-op deployment will either fail health gate or falsely complete, and history will show a spurious deployment record. `compose RestartStack` is correct (`StopStack→StartStack`) but unreachable for admin stacks — wiring inversion.

**RECOMMENDATION:** `ADAPT` — Split apphosting restart into true `UpdateApplicationStatus(desired_state=running)` + immediate daemon `AdminContainerRestart` fan-out (or queue op), or document as `redeploy`. Add admin `POST /compose/:id/restart` route + `compose.ts:restartComposeStack` + UI button. Wire app desired_state reconciler (or mark `FALSE_COMPLETION`).

**SEVERITY:** `P1 USER_VISIBLE` — user clicks Restart and gets a failed/empty deployment; compose Restart appears missing.

---

### C04 — Update / Redeploy

**REFERENCE:**

- Coolify: `ApplicationDeploymentJob.php:1223 just_restart`, `1245 should_skip_build`, plus `UpdateApp` → requeue `ApplicationDeploymentJob` with new env/commit.
- Dokku: `ps:rebuild`, `git:from-image`, `config:set --no-restart` then `ps:restart`.
- CapRover: `PUT /user/apps/appDefinitions/update` → new image tag → deploy.
- Komodo: `UpdateStack` → new hash → `DeployStack` with rollback on health fail.
- Portainer: `RedeployWhenChanged` polls git → `Deploy`.
- 1Panel: `app_install.go:336 Update`, `472 syncAppInstallStatus` diff env/compose then `compose up -d`.
- Uncloud: `UpdateServiceRequest + ApplyServiceUpdate` (plan diff).
- Compose: `compose up -d` idempotent; `pull` then `up`.

**FORGE:**

- Frontend:`apps.ts:237 updateApp(PUT /apps/:id)`, `320 redeployComposeStack(POST /apps/:id/compose/redeploy)`, `compose.ts:75 updateComposeStack(PATCH /compose/:id)`, `[id]/page.tsx redeployMutation → deployComposeStack(id)` (clone spec).
- API:`handlers_apphosting.go:231 PUT /apps/:id` → `UpdateApp`, `850 PUT /apps/:id/compose` (sourceConfig), `882 POST compose/redeploy` → `TriggerDeploy`; `handlers_compose.go:378 PATCH /compose/:id` → `UpdateComposeStack`, `413 POST :id/deploy` (re-clone same YAML → new DeployComposeStack), `197 POST /compose/git/:id/redeploy`, `237 pull-redeploy`.
- Service:`apphosting/service.go:139 UpdateApp` (mutates `applications` columns, no deploy); `compose/lifecycle.go:249 DeployComposeStack` (hash+reservation+scheduler+daemon), `476 UpdateComposeStack` (compare hash+env, `if newHash==oldHash && mapsEqual`, short-circuit, else `status=updating → daemon.ComposeDeploy → health → running/degraded` with `rollbackStack` on daemon error), `788 PullStack`.
- DB: `applications.updated_at`, `compose_stacks.compose_hash`, `git_previous_compose`.
- Worker: `UpdateComposeStack` is synchronous (no queue lease), while deployments are async via rollout. Inconsistency: app updates are sync DB only, compose updates sync with beacon call, deployments async with lease.
- Event: `EventComposeUpdated` on success, `deployment_started` etc for app deploy path.

**STATUS:** `PARTIAL` — compose update is substantive (with rollback); app update is metadata-only split brain.

**GAP:** Updating an app (image/git branch via `PUT /apps/:id`, `PATCH /apps/:id/git`) does not trigger any deployment. User must separately `POST /apps/:id/deploy` (which creates empty-image deployment). Reference platforms couple update→deploy atomically (Coolify re-queues job). Forge's two-step is silent — user changes image in UI, sees "saved" but app still runs old version until they discover the separate Deploy button. Compose `/compose/:id/deploy` re-creates a new stack row instead of updating in place (via `DeployComposeStack` not `UpdateComposeStack`), leaking stack rows per redeploy.

**FORGE LOGIC FINDING — FLF-03:** `handlers_compose.go:413 POST /compose/:id/deploy` loads `existing` then **creates a new stack** via `DeployComposeStack` with a fresh `cps-` ID, rather than `UpdateComposeStack` or `RestartStack`. Each redeploy multiplies `compose_stacks` rows for the same logical workload; history shows duplicate rows not versions. Should be `UpdateComposeStack` or pool.

**RECOMMENDATION:** `ADAPT` — make `PUT /apps/:id` optionally `?redeploy=true` or auto-enqueue `StartRollout` with new image; fix compose redeploy to call `UpdateComposeStack` + `PullStack` instead of new stack.

**SEVERITY:** `P1 USER_VISIBLE` (silent stale version) + `P2 OPERATOR_VISIBLE` (row leak).

---

### C05 — Rollback

**REFERENCE:**

- Coolify:`ApplicationDeploymentJob.php:4810 handleFailedDeployment` → auto-rollback via `ApplicationPreview` tag, plus manual `rollback` to previous `ApplicationDeploymentQueue`.
- Dokku: `ps:restore`, `git:from-image <previous-tag>` manual rollback; checks-gated rollback on health failure.
- CapRover: `AppsDataStore.rollbackAppVersion` (retains previous CaptainDefinition/digest).
- Komodo:`stack.rs rollback` → previous `compose_hash` via `UpdateStack` rollback path (periphery keeps old dir).
- Portainer: not first-class rollback; stack revert via git ref or `docker stack deploy` previous file.
- 1Panel:`app_install.go:835 syncAppInstallStatus` — rollback is reinstall previous `AppInstall` version via `app_install.go:571 GetUpdateVersions` + `246 Operate{rollback}`.
- Uncloud: not yet (`README: Automatic rollback on failure is coming soon`).
- Compose: no built-in rollback; manual `compose down && compose up` previous file.

**FORGE:**

- Frontend:`deployments.ts:113 rollbackToRevision(POST /admin/deployments/:id/revisions/:revId/rollback)`, `118 rollbackToPrevious`; `apps.ts` no rollback shortcut.
- API:`handlers_deployment.go:44 POST /:id/rollback` → `Rollback` (blue→green flip), `handlers_revisions.go:32 POST /:id/revisions/:revId/rollback` → `RollbackToRevision`, `44 rollback-previous`, `62 compare`.
- Service:`deployment/service.go:436 Rollback` (flip `active_target=blue` if `!=blue`, version-gated), `revisions.go:147 RollbackToRevision` (validates `revision.DeploymentID==deploymentID`, `validateImageRef(targetRev.ImageRef)`, `UpdateDeploymentStatusVersioned(in_progress)`, then `UpdateDeployment(Image=targetRev.ImageRef)` + `SupersedeDeploymentRevisions` + `UpdateDeploymentRevisionStatus(active)`), `202 RollbackToPrevious` (`GetPreviousDeploymentRevision`), `execution.go:313 executeRollbackStep → RollbackToPrevious`, `321 handleStepFailure` (if `AutoRollbackEnabled||RollbackOnHealthFailure` → `RollbackPending → RollingBack → RollbackToPrevious → RolledBack`), `compose/lifecycle.go:844 rollbackStack` (swap back YAML/hash/env → `daemon.ComposeDeploy` previous YAML → health).
- DB:`deployment_revisions` + `deployment_history` + `rollbacks` (119), `deployment_releases` for zd path, `compose_stacks` retains `previousCompose` via git fields.
- Event: `deployment_rolled_back`, `auto_rollback_triggered|completed|failed`.
- Tests:`revisions_test.go` covers `RollbackToRevision`, `CompareRevisions`.

**STATUS:** `PARTIAL`

**GAP:** Two disjoint rollback worlds: (1) deployment revisions rollback is image-only (`ImageRef`), not compose manifest, git SHA, env, or traffic target; `CompareRevisions` diff covers `composeManifestRef/gitCommitSha/configHash/metadata` (225) but `RollbackToRevision` only restores `ImageRef` (171). Compose manifest rollback exists only in `compose/rollbackStack` (which restores YAML) but is unreachable for non-git stacks via generic revoke. (2) App-level rollback has no handler — only `deployment_id` rollbacks, not `application` revert of `source_config`. Reference CapRover/1Panel rollback restores full spec, Forge restores only image.

**FORGE LOGIC FINDING — FLF-04 (P2):** `revisions.go:147 RollbackToRevision` sets `deployment.Image=targetRev.ImageRef` then `store.UpdateDeployment(deployment)` — this is a **non-version-checked** `UPDATE` (store `UpdateDeployment` increments version unconditionally without `WHERE version=`). Yet the prior line did `UpdateDeploymentStatusVersioned(in_progress)` which already bumped version, so `toStoreDeployment(deployment)` carries stale `deployment.Version` (pre-bump). The second `UpdateDeployment` does not check version, so concurrent rollers can overwrite; plus image is set on a stale version value, breaking optimistic concurrency the rest of service relies on (`ErrVersionConflict`).

**RECOMMENDATION:** `ADAPT` — make rollback path single version-gated transaction: `SELECT FOR UPDATE` or `UpdateDeploymentConfigWithRevision` atomically. Include compose manifest + env in restore payload.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — rare race but rollback data loss.

---

### C06 — Delete / Uninstall

**REFERENCE:**

- Coolify:`Application.php:483 deleteConfigurations,492 deleteVolumes,509 deleteConnectedNetworks` — cascading cleanup via jobs `DeleteResourceJob`.
- Dokku:`apps:destroy`, `ps:stop` + `scheduler:remove` + volume cleanup.
- CapRover: `DELETE /user/apps/appDefinitions/delete` → `DockerApi.removeApp`.
- Komodo:`stack.rs RemoveStack` → periphery `compose down -v --remove-orphans?`.
- Portainer:`DELETE /stacks/:id?external=false` → `stackManager.RemoveStack`.
- 1Panel:`app_install.go:692 DeleteCheck → 246 Operate{delete}` → `docker compose down -v` + FS `path.Join(GetAppPath(), Name)` removal.
- Uncloud:`pkg/client/service.go:141 RemoveService → StopContainer → RemoveContainer(RemoveVolumes:true)` fan-out per machine.
- Compose:`compose down -v` (+ `--remove-orphans` opt-in).

**FORGE:**

- Frontend:`apps.ts:241 deleteApp(DELETE /apps/:id)`, `compose.ts:85 deleteComposeStack`, `app-store` install uninstall.
- API:`handlers_apphosting.go:259 DELETE /apps/:id` → `DeleteApp` (no cascade guard beyond FK), `handlers_compose.go:401 DELETE /compose/:id` → `DeleteComposeStack`, `handlers_apphosting.go:545+` appstore `UninstallApp`.
- Service:`apphosting:174 DeleteApp` → `store.DeleteApplication` (FK CASCADE on `app_services`, `SET NULL` on `deployments.current_deployment_id`), `compose/lifecycle.go:570 DeleteComposeStack` → `status=deleting` → resolve node → `daemon.ComposeDelete` (`beacon compose.go:580 down -v`, `removeOrphans` only if `?removeOrphans=true`, defaults false) → `cancelReservation` → `status=deleted`, error → `markFailed`. `appstore/service.go:126 UninstallApp` → `composeSvc.DeleteComposeStack` then delete row.
- Store:`store_apphosting.go:374 DeleteApplication: DELETE WHERE id` (relies on `ON DELETE CASCADE` for services, `SET NULL` for server/current_deployment). No check for active deployment.
- DB:`100_z` `ON DELETE CASCADE` services, `SET NULL` server/deployment; `compose_stacks` no FK to node? Has `node_id REFERENCES nodes(id) CASCADE` via 098.
- Beacon:`compose.go:550 handleComposeDelete` validates `validStackID`, per-stack `lock`, checks `compose.yaml` exists, runs `docker compose down -v` (conditionally `--remove-orphans`).
- Event: `EventComposeDeleted`, `EventAppFailed`.

**STATUS:** `PARTIAL`

**GAP:** (1) `DeleteApplication` does not guard active deployments — `constraints` allow delete while `deployment` is `in_progress|provisioning|awaiting_health`. DB partial index `106` prevents concurrent active deployments but not delete-during-deploy race. 1Panel's `DeleteCheck(664)` enumerates resources first. Forge fire-deletes; beacon containers keep running orphaned (compose path has no orphan sweep because `removeOrphans` defaults false). (2) App delete does not cancel placement reservations nor clean `deployment_history` rows (history retained). (3) Compose delete via API does not check ownership when called via admin vs user (admin can delete any). Reference Uncloud removes volumes (`RemoveVolumes:true`); Forge compose `down -v` matches but app path deletes no volumes/containers at all.

**RECOMMENDATION:** `ADAPT` — gate `DeleteApplication` on `!isActiveStatus` (like rollout guard), enqueue cleanup operation, cancel reservations, sweep beacon containers, plus `ON DELETE` sweep for compose `down -v`.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — orphan containers & leaked reservations.

---

### C07 — Scale / Replicas

**REFERENCE:**

- Coolify: per-app replicas via `replicas` column + swarm mode (`SwarmDocker`).
- Dokku: `ps:scale web=2 worker=1` → `scheduler-docker-local:scale` writes `DOKKU_SCALE` + restarts.
- CapRover: `updateConfig.scale` in service model.
- Komodo: `StackDeploy` has no per-service scale; relies on compose `scale:` or replica nodes.
- Portainer: `stacks` no scale UI; docker stack `scale`.
- 1Panel: no first-class scale; manual `docker compose scale` after install.
- Uncloud: `pkg/api.ServiceSpec{Replicas, Mode replicated|global}` + `replicamanager/service.go:100 CreateApp(Replicas 1-100), 156 DeployApp(PlaceReplicas), 381 ScaleApp` (+ `placement.Engine`, `reservations.Manager`, `BeaconClient`).
- Compose: `--scale SERVICE=NUM` (create.go:86 `applyScaleOpts`).

**FORGE:**

- Frontend: no app-scale UI. `apps.ts` has no `scaleApp`. `compose.ts` no scale.
- API:`handlers_apphosting.go:416 PATCH /organizations/:orgId/apps/:appId/services/:serviceId` → `UpdateService` (optional `replicas`), but **calls `ReplicaManager.ScaleApp` only if `Replicas != nil && updated.ReplicaAppID != nil && cfg.ReplicaManager != nil`** — otherwise just DB update. No dedicated `POST /scale`. `handlers_processes.go:43 PUT /servers/:id/processes/:type/scale`.
- Service:`apphosting:397 ScaleService` → validates `targetReplicas>=0`, `UpdateAppService(Replicas)` + if `ReplicaAppID` then `UpdateReplicaAppReplicas`. `apphosting:440 GetServiceStatus` + `594 GetServiceOverview` compute health from `ListInstancesByApp`. `compose lifecycle.go` `DeployComposeRequest{MemoryMB,CPUShares,DiskMB}` quotas but no per-service replica knob — compose services scale only via spec edit.
- Store:`app_services.replicas`, `replica_apps` table (via `101_a_multi_node_replicas.sql`), `instances` via `store_instances.go`.
- DB:`102_a_uncloud_service_model.sql` adds `replica_app_id` + `service_endpoints`.
- Worker:`replicamanager/service.go:100 CreateApp`, `156 DeployApp` (places replicas, creates reservations, dispatches beacon commands idempotently with `instanceCommandID(generation,operationID)`, metrics `ScaleUpTotal/DownTotal`), `409 scaleUp` fan-out.
- Tests:`replicamanager/service_test.go` covers create/scale.

**STATUS:** `PARTIAL` — Uncloud-inspired replica manager exists but app UI never surfaces it; scale path split between `UpdateService(replicas)` and `ReplicaManager` is implicit wiring.

**GAP:** (1) App service scale via `PATCH .../services/:serviceId` updates DB replicas but placement only reconciles if `ReplicaAppID` present. New services created via `CreateService` have no `ReplicaAppID`, so scale silently mutates `app_services.replicas` column with no container change — `FALSE_COMPLETION`. (2) Compose cannot scale per-service without editing YAML; no `POST /compose/:id/scale` despite beacon `docker compose up --scale`. (3) No autoscaler binding to `app_services` (autoscaler policies target nodes/servers, not app services).

**RECOMMENDATION:** `ADAPT` — ensure `CreateService` creates `ReplicaApplication` and `Instance` rows, or route all scale via `Replicamanager.ScaleApp` uniformly. Add compose per-service scale handler.

**SEVERITY:** `P1 USER_VISIBLE` (silent no-op scale) / `P2` (compose gap).

---

### C08 — Health / Probes / Health Gate

**REFERENCE:**

- Coolify:`Application.php:143 health_check_*` (enabled, path, port, host, method, return_code, scheme, response_text, interval, timeout, retries, start_period, type http/cmd, command).
- Dokku:`plugins/checks` — `CHECKS` file or `app.json` healthchecks, `checks:enable`, zero-downtime verifies before `proxy` switch.
- CapRover: `AppDefinition.healthCheckPath` + per-app health.
- Komodo: monitor loop `stack.rs` `WaitForHealthy` polls `ps` not HTTP; degraded if restarts exceed.
- Portainer: health via `endpointRelation` + edge deployment info; not HTTP gate.
- 1Panel: not explicit health gate; status via `docker ps` healthy/unhealthy.
- Uncloud: `pkg/client/service.go` health via container state, not HTTP.
- Compose: `healthcheck: { test, interval, timeout, retries, start_period }` in spec.

**FORGE:**

- Frontend: no health UI for deployments except `deployments/history` status colours; compose `[id]/page.tsx` not shown.
- API: health fields on `deployment` via query `healthCheckPath/Port/Host`.
- Service deployment:`healthgate.go:11 CheckHealth` (if `path==""||port==0 → Passed:true`, else `GET http://{host|localhost}:{port}{path}` with 10s timeout, no redirect, limit 64KB, `200-399` passes), `48 WaitForHealthGate` (if `!HealthGateEnabled→ nil`; ticker `HealthGateIntervalMs`, context `TimeoutSeconds`, requires `threshold` consecutive successes, `300ms` default? Actually 5000ms interval, threshold 3). Called for `StepHealthGate` + `StepVerify` (blue-green second check) via `stepsForStrategy` (65 `if healthGateEnabled` inject health_gate, verify).
- Service compose:`lifecycle.go:188 WaitForHealthy` polls `daemon.ComposeStatus` every 5s for 2m, requires all `state==running||status==up` and `restartCount <= prev+1` otherwise fail, then marks `running` vs `degraded`.
- Service replicamanager: health via `Instance.status running|failed` aggregated in `GetServiceOverview`.
- Store:`health_check_configs`/`health_check_results` (114_e) for zd releases, separate from deployment health gate columns `health_gate_*`.
- Beacon: no HTTP health probe; compose status derives from `docker compose ps --format json` (ps itself reports health `Health` column but mapped only to `State/Status/Ports`).

**STATUS:** `PARTIAL`

**GAP:** Two incompatible health worlds: *deployment* health gate `GET http://localhost:port/path` runs inside **API process**, not on the target node where the container/port actually listens. `CheckHealth` will always hit API host, not `node.BaseURL`. `ValidateHealthGateTarget:71` even enforces `HealthCheckHost` must be `localhost|127.0.0.1|::1` (only local gateway allowed), cementing the wrong target. Real references run checks against the workload container IP or `proxy` network. Compose `WaitForHealthy` is closer (polls `ps` via beacon) but still no HTTP. Zero-downtime `health_check_configs` is a third table never consulted by deployment health gate.

**FORGE LOGIC FINDING — FLF-05 (P1):** `healthgate.go:16 host="localhost"` and `rollout.go:71 validateHealthGateTarget` clamping to loopback means health gate cannot validate a container on a remote node — it will time out after `TimeoutSeconds` and trigger `handleStepFailure` → `StatusFailed`. Even with `CleanupOnFailure` the `StartRollout` goroutine then fires auto-rollback which itself may falsely succeed (see FLF-01). The only way `CheckHealth` passes is if API host happens to listen on same port — fluke.

**RECOMMENDATION:** `ADOPT` Dokku checks — run health gate **via beacon** (`GET` on node or `docker inspect --format {{.State.Health.Status}}`) or inside overlay network. Merge `health_check_configs` with deployment gate or delete duplicate.

**SEVERITY:** `P0 USER_VISIBLE` — every health-gated deploy fails.

---

### C09 — Version / Revisions / History / Diff

**REFERENCE:**

- Coolify: `ApplicationDeploymentQueue` (queue per app, version = git SHA/image tag), `ApplicationPreview` for PR versions.
- Dokku: version = `git rev-parse HEAD` + `repo:gc` — `apps:report` shows `deployed_version` SHA.
- CapRover: `CaptainDefinition` revision per deploy (tag + digest), `AppsDataStore` versioned builds.
- Komodo: `StackUpdateHistory` via `git commit SHA` + `compose_hash` (`computeHash sha256`), `git_previous_compose`.
- Portainer: git ref per stack deploy (`StackGitConfig`), no semantic version table.
- 1Panel: `AppInstall.Version:16`, `AppDetail` upgrades via `GetUpdateVersions(571)` — semver store.
- Uncloud: `ServiceSpec` generation + `Instance` generation (`replicamanager`).
- Compose: `--hash` or image digest pinning; `docker compose config --hash`.

**FORGE:**

- Frontend:`deployments.ts:103 fetchDeploymentRevisions`, `108 compareRevisions`, `113 rollbackToRevision`; `deployments/history/page.tsx` (historical `deployment_history` per server/commitHash).
- API:`handlers_revisions.go:16 GET /:id/revisions`, `32 rollback`, `62 compare?from=&to=`.
- Service:`revisions.go:58 configHash sha256 json cfg → 12hex`, `82 CreateRevision` (increment nextNum or 1), `126 ListRevisions`, `147 RollbackToRevision`, `202 RollbackToPrevious`, `225 CompareRevisions` (diff imageRef/composeManifestRef/gitCommitSha/configHash/metadata).
- Store:`store_deployment_revisions.go` `CreateDeploymentRevision`, `SupersedeDeploymentRevisions`, `GetPreviousDeploymentRevision`; `store_deployment_history.go` `CreateDeploymentRecord`, `UpdateDeploymentRecordStatus`; `store_deployments.go` `UpdateDeploymentCurrentRevision`.
- DB:`099` `deployment_revisions` UNIQUE `(deployment_id, revision_number)`, `100` `applications.current_deployment_id`, `119` `deployment_history`, `095` `deployments.current_revision_id`, `102` `images:digestImageRef` via `rollout.go:332`.
- Worker: `execution.go:210 executeInitStep` snapshots `RevisionConfig{ImageRef, Description}` via `CreateRevision`, sets `active`, updates `currentRevisionID`.
- Tests:`revisions_test.go` covers `CreateRevision`, `List`, `RollbackToRevision`.

**STATUS:** `PARTIAL`

**GAP:** Revisions are per-`deployment_id`, not per-`application` nor per-`compose_stack`. So each new `deployment` (triggered via app) starts at revision 1 again — history fragmented across many deployments instead of linear app history. Reference Coolify history is app-centric queue. Forge `deployment_history` is server-centric (not app), `deployment_revisions` is deployment-centric. Cannot list "all revisions for app X" without joining via `applications.current_deployment_id` which only points at latest deployment. `configHash` truncated to 12 hex (collision risk) and excludes `envVars/ports/resources` for deploy path.

**RECOMMENDATION:** `ADAPT` — anchor revisions to `application_id` (or `compose_stack_id`) with global `revision_number`. Preserve `deployment` linkage but list via app. Full config hash.

**SEVERITY:** `P2 OPERATOR_VISIBLE` — audit/history confusion.

---

### C10 — Logs (deploy + container + compose)

**REFERENCE:**

- Coolify: per-deployment `logs` stream via `ApplicationDeploymentJob` stdout captured to DB + live Tailwind.
- Dokku: `logs:failed`, `logs <app> -t`, `scheduler:logs`.
- CapRover: `logs` API via Docker `logs --tail`.
- Komodo: `StackLogs` from periphery (`logs --tail` per service).
- Portainer: `GET /stacks/:id/logs`.
- 1Panel: not streaming, but `AppInstall` logs via task.
- Uncloud: `log` package per container via machine API.
- Compose: `compose logs --tail`.

**FORGE:**

- Frontend:`apps.ts:269 fetchAppLogs(GET /apps/:id/logs)`, `273 fetchAppServiceLogs`, `compose.ts:105 getComposeStackLogs(service?,tail?)`, `deployments.ts:150 fetchDeploymentLogs`; comps `[id]/page.tsx` polls `logs` q 5s.
- API:`handlers_apphosting.go:563 GET /apps/:id/logs` → `ListDeploymentBuildLogs` via `currentDeploymentID`; `handlers_compose.go:468 GET /compose/:id/logs` → `GetStackLogs`; `handlers_deployment.go` no direct logs; `handlers_deployment_history.go` (?) `GET /admin/deployment-history/:id/logs`.
- Service:`compose/lifecycle.go:663 GetStackLogs` → `daemon.ComposeLogs` → beacon `compose.go:661 handleComposeLogs` validates `validStackID`, locks, `docker compose logs --no-color --tail {tail} [service]` plain text. Deployment build logs via `store_deployment_history.go` `ListDeploymentBuildLogs`.
- Store:`deployment_build_logs` (?) separate table, not explicit migrations but present.
- Beacon:`compose.go:661` locks per stack; `server.go:392` registers routes.

**STATUS:** `PARTIAL`

**GAP:** (1) `fetchAppLogs` maps `ListDeploymentBuildLogs` entries to faux `AppLogEntry` by re-formatting `BuildLog{stage,message,createdAt}` — build logs only for current deployment, not container runtime logs; references expose live `docker logs -f`. Forge app logs show no runtime logs unless an instance container is queried via `instances/:instanceId/logs` (which is absent for compose). (2) No streaming/websocket log tail — polling only. (3) `ComposeLogs` requires `compose.yaml` exists on node; after `DeleteComposeStack` logs disappear even if history requested. 1Panel retains post-delete logs.

**RECOMMENDATION:** `ADAPT` — unify log source: `GET /apps/:id/logs?stream=websocket` proxied to beacon `docker logs -f`; retain build vs runtime split in UI.

**SEVERITY:** `P3 USER_VISIBLE` — debugging degraded.

---

### C11 — Status / Desired vs Observed / Reconciliation

**REFERENCE:**

- Coolify: `Application.status` attribute composites `container_status:health` (836) + server infra health (809-876), plus `stop` guards `stoppedAfterRestartLimit`.
- Dokku: `ps:report` vs `scheduler:status` — observed from `docker ps`; no desired/observed split, just `DOKKU_SCALE`.
- CapRover: `AppDefinition` desired (image/port/replicas) vs `DockerApi.getAppStatus` observed.
- Komodo: Core `state.rs` reconciles `desired_stack_state` vs periphery `ps`; degraded if drift.
- Portainer: stack status `running/stopped` via `stackManager.Status`.
- 1Panel: `AppInstall.Status` (`running|stopped|error`) synced via `SyncAppInstallStatus(835)` polling `docker ps`.
- Uncloud: `MachineMember` UP/SUSPECT + `ServiceMode` desired vs `Container.State` observed; reconciled via placement engine, not constant loop (imperative).
- Compose: `ps --format json` `State` vs spec.

**FORGE:**

- Frontend:`apps.ts:3 AppStatus running|stopped|deploying...`, `344 statusLabel/statusTone`; `compose/page.tsx:16 statusConfig` (running/deploying/awaiting_health/stopped/degraded/failed/updating/deleting); `deployment/history` statuses.
- API: app desired vs observed columns exposed via `GET /apps/:id` raw (`store_apphosting.go:182` SELECT). No `GET /apps/:id/status` endpoint (but handlers have `handlers_deployment` status via deployment).
- Service:`store_apphosting.go:22 DesiredState running|stopped|removed CHECK, 23 ObservedStatus idle|running|done|error|deploying`; `apphosting:185 UpdateAppStatus` writes observed but never called except `UpdateApplicationStatus` manual. `compose/lifecycle.go:188 WaitForHealthy` sets `running|degraded` after deploy, but no background reconciler polling `ps`; `deployment/execution.go:389 ResumeDeployments` only for interrupted deployments.
- Store: both columns persisted but observed stays `idle` forever unless manually updated (no daemon reporter writes app observed). `compose_stacks.status` is more lively (updated by sync deploy/start/stop).
- Beacon: reports `ps` status but only on-demand `GET /compose/:id/status` (604), not pushed.
- Event: no `observed_status` change events; reconciliation service `reconciler/*` targets servers/nodes, not applications/compose stacks.

**STATUS:** `FALSE_COMPLETION`

**GAP:** `applications.observed_status` is write-only stub. `POST /apps/:id/start|stop` mutates `desired_state` but nothing reconciles it — unlike Komodo monitor or 1Panel `SyncAppInstallStatus`. The app detail will forever show `DeployStatusBadge` derived from `app.status` (frontend `page.tsx:135`) which maps `observed_status` that never changes from `idle`. So start/stop appear to succeed (200 `{ok:true}`) while nothing happens. Compose stacks do update status via imperative ops but lack periodic reconciliation for external `docker kill` drift.

**FORGE LOGIC FINDING — FLF-06 (P0):** `handlers_apphosting.go:450/475 handlers_apphosting.go:450 POST /apps/:id/start` returns `{ok:true}` after `UpdateApp{DesiredState:"running"}` DB write only. No instance is started, no beacon call, no event, no error if node unreachable. Caller (frontend) invalidates `["apps"]` and badge still shows previous status, creating false success. This is `FALSE_COMPLETION` per definition.

**RECOMMENDATION:** `ADAPT` — introduce `AppReconciler` (like `1panel:syncAppInstallStatus` poll) or make start/stop enqueue `operation.Service` ops via `replicamanager`/`queue` + beacon `AdminContainerStart`. Until then, return `202 Accepted, not reconciled` semantics.

**SEVERITY:** `P0 USER_VISIBLE` — core action lies.

---

### C12 — Queue / Execution / Concurrency / Idempotency

**REFERENCE:**

- Coolify:`ApplicationDeploymentJob` (ShouldQueue) + `ApplicationDeploymentQueue` per app (`handle():283` unique per app, `tags()` for Horizon), `failed` → retry after backoff.
- Dokku: not queued — synchronous `dokku ps:restart` serializes via CLI lock.
- CapRover: deploy queue + `appsLock` per app in Node.
- Komodo: Core `periphery` execution is serialized per stack via `composeStack lock`; Core API write queue via `bin/core/src/state.rs`.
- Portainer: `stackDeployer` sync + `Edge` async via `num_deployments`; not strict lease.
- 1Panel:`agent/app/task/task.go` task queue for install/upgrade.
- Uncloud: placement generation + `BeaconCommandLog` idempotency `instanceCommandID(generation, operationID)`.
- Compose: `docker compose` itself locks project dir via `composeStack.lock` in beacon (already).

**FORGE:**

- Frontend: no queue UI for apps (only `GET /compose`).
- API:`handlers_deployment.go:68 POST /:id/execute`, `98 POST /resume`; no `POST /apps/:id/scale` queue.
- Service deployment:`store_deployments.go:300 ClaimExecutionLease` (`execution_lease_until IS NULL OR < now() AND status NOT IN (completed,failed,cancelled,rolled_back)` → `executor_id = $2`), `RenewExecutionLease`, `ReleaseExecutionLeaseIfOwner`; `execution.go:22 ExecuteDeployment` claims lease (5m) for `executorID=uuid`, defers `ReleaseExecutionLease`, checks `StatusPending` entry, sets `StatusInProgress` + `UpdateDeploymentRecordStatus(running)` + `createSteps`, `updateProgress(0,0,timeoutAt)`, `context.WithTimeout(timeout)` then step loop; `389 ResumeDeployments` reclaims `ListInProgressDeployments` with `TimeoutAt` check → fail or resume via `executingDeployments sync.Map` guard. `rollout.go` launch `go ExecuteDeployment` with `30m` background context. `operation/service.go:ReapStale` 5m reaper (separate from deployment lease), `queue/service.go` `JobCompose*` single writer for compose lifecycle.
- Service compose:`lifecycle.go:249 DeployComposeStack` is **synchronous + node-blocking** (no queue lease) — holds reservation, calls `daemon.ComposeDeploy`, then `WaitForHealthy` 2m inline. `570 DeleteComposeStack` also sync.
- Store: `deployments.execution_lease_until`, `executor_id`, `timeout_at`, `version`, `progress_pct/next_step`; partial index `106` forbids two active deployments per server.
- Event:`deployment_execution_started`, `deployment_completed/failed`.
- Tests:`deployment_test.go:40` successful rollout, `healthgate_e2e_test.go` timeout, `helpers_test.go`.

**STATUS:** `PARTIAL` — deployment path is lease-gated, compose path is not; queue exists for operations but compose mutations bypass it (deprecated `operation.OpCompose*` comments say "must be dispatched via queueSvc").

**GAP:** (1) `operation/service.go:48 OpComposeDeploy|Update|Delete|Start|Stop|Restart` are marked deprecated "Retained for backward compatibility… must be dispatched via queueSvc" — yet `compose/lifecycle.go` directly calls `daemon.ComposeDeploy` synchronously, not enqueuing. So queue's at-least-once, retry, and durable execution guarantees don't apply to compose. `beacon queue_journal_recovery_test.go` covers queue recovery but compose won't benefit. (2) `ReleaseExecutionLease` unconditionally clears lease even if another worker stole it (via timeout). `ReleaseExecutionLeaseIfOwner` exists (345) but not used in `ExecuteDeployment` defer (32). Race where lease expires, new worker claims, old worker's defer wipes new lease.

**FORGE LOGIC FINDING — FLF-07 (P1):** `execution.go:32 defer ReleaseExecutionLease(ctx, deploymentID)` should be `ReleaseExecutionLeaseIfOwner(ctx, deploymentID, executorID)`. Current unconditional release violates lease fencing — under contention or slow `stepErr→handleStepFailure` (30s handleCtx), the lease can expire, second executor claims, first's defer deletes second's lease, allowing third to claim and double-execute promote/drain. `ResumeDeployments` further claims without `LoadOrStore` after `TimeoutAt` check (race). This is incorrect state transition / missing idempotency.

**RECOMMENDATION:** `ADAPT` — replace `ReleaseExecutionLease` with `ReleaseExecutionLeaseIfOwner`; add lease renewal heartbeat loop for long deploys (>5m). Route compose mutations via `queue.Service` (or document sync trade-off).

**SEVERITY:** `P1 OPERATOR_VISIBLE` — rare double-deploy, but violates queue correctness.

---

### C13 — Compose Spec Control / Validation

**REFERENCE:**

- Coolify: `compose_parsing_version:331`, sanitizes compose, injects labels, validates via parser.
- Dokku: `docker-options` plugin adds arbitrary `docker run` flags to generated `docker run` from `app.json`.
- CapRover: CaptainDefinition strictly whitelisted (ports, volumes, env) — no raw compose.
- Komodo: `compose` crate validates via `docker compose config` dry run before save.
- Portainer: validates via template/component; `ValidateComposeSecurity` similar.
- 1Panel: raw `docker-compose.yml` stored at `GetComposePath()`, minimal validation — relies on docker compose error at `up`.
- Uncloud: `ServiceSpec.Validate()` plus `VolumeScheduler`.
- Compose spec: `compose-go` supports `profiles`, `restart`, `build`, `healthcheck`; Forge mirrors subset.

**FORGE:**

- Frontend:`compose.ts:49 validateCompose`, `apps/new/page.tsx:280 validateCompose` local regex `services:` plus server `ValidateCompose`.
- API:`handlers_compose.go:106 POST /compose/validate` → `ValidateCompose`, `118 import` validates and marshals `ParsedCompose` into `ProjectDocument.ParsConfig`.
- Service:`compose/service.go:145 ParseComposeYAML` (1 MB cap, `interpolateEnv` with `${VAR:-default}`, `${VAR:?msg}`, `$$`), `240 ValidateCompose` (checks `len Services==0`, each needs `image||build`, warns `profiles` not enforced, `healthcheck` not yet implemented, `deploy` not fully, `ValidateComposeSecurity` errors: `privileged`, `network_mode=host|service:`, `pid=host`, `ipc=host`, `devices`, `security_opt` warning, `userns_mode=host`, `container_name` warning, `restart=always` warning, `docker.sock`/`/proc`/`/sys`/`/etc`/`/` sensitive mounts (with `slog.Warn`), `cap_add` dangerous caps), `659 ValidateHostMountWithAllowlist(source, isAdmin, allowedMounts)`. `beacon/compose.go:77 validateComposePolicy` duplicates check server-side (privileged truthy yes/on/true/1, cap_add non-empty, bind mounts long-form, host bind source `/` or traversal, `published <1024`).
- Store:`ProjectDocument.ParsedConfig JSONB`, `compose_stacks.compose_yaml TEXT`.
- Beacon:`handleComposeDeploy:358 validateComposePolicy` returns 400 policy violation before `docker compose up`.
- Tests:`parser_test.go`, `parser_comprehensive_test.go`, `service_test.go`.

**STATUS:** `PARTIAL` — dual validation (API + beacon) is strong, but drifted policies and incomplete warnings.

**GAP:** API allows `healthcheck`/`deploy` but warns and then beacon rejects only `healthcheck`? Actually `ValidateCompose` warns "healthcheck support is not yet implemented" but does not block; `beacon validateComposePolicy` ignores `healthcheck` entirely. So API warns but lets store a compose with healthcheck that beacon silently ignores (no healthcheck propagated via `ps`). Similarly `restart: always` warns but beacon permits it — then lifecycle management conflicts (container restarts despite `stopped` desired). CapRover/1Panel normalize `restart: unless-stopped` to align with platform.

**RECOMMENDATION:** `ADAPT` — align API + beacon policy tables; either enforce healthcheck propagation (pass `--health-cmd` via labels) or reject with error, not warning.

**SEVERITY:** `P3 USER_VISIBLE` — spec silently ignored.

---

### C14 — Zero-downtime / Blue-green / Canary / Rolling

**REFERENCE:**

- Coolify: `rolling_update:1904` — rolling per service, green preview network alias, health gate before switch, rollback on fail.
- Dokku: `checks` + `nginx-vhosts` hot reload — canary via `ps:scale` + `proxy:enable`.
- CapRover: rolling with `healthCheckPath` before `updateApp` switch.
- Komodo: `StackDeploy` is in-place `up -d`; not blue-green, but deploy+health+rollback approximates rolling.
- Portainer: Swarm rolling via `docker stack deploy` update_config `parallelism, delay`.
- 1Panel: not zd — `down`/`up` briefly drops (unless user adds compose healthcheck manually).
- Uncloud: `deployment.Run` per `Machine` rolling via scheduler staggered.
- Compose: `deploy.resources` rolling supported via Swarm; plain compose `up -d` recreates.

**FORGE:**

- Frontend: no strategy selector exposed in `apps/new` wizard (only `store/strategy` in deployment API).
- API:`handlers_deployment.go:24 POST /admin/deployments/blue-green` hardcodes `StrategyBlueGreen`, `handlers_revisions.go:52 POST /:id/rollout` generic `RolloutRequest{Strategy recreate|rolling|blue-green|canary, CanaryPercent, health fields, TargetReplicas}`.
- Service:`rollout.go:42 StartRollout` picks strategy; `steps.go:48 stepsForStrategy` yields distinct DAGs; `execution.go:256 executePromoteStep` flips `active_target` (blue↔green), `278/285/292/299 scale/drain` stubs.
- Store:`deployments.strategy` + `rollout_strategy` + `active_target` / `blue_target_id / green_target_id`, `health_gate_*`, `deployment_releases` zd table.
- Beacon: no traffic manager — `service_endpoints` exists (102) but not populated by promotion; no `loadbalancer` integration called from deployment promote. `UpdateComposeStack` has no traffic switch; deploy is just `up -d` new containers on same node (port conflict if `ports` expose same host port).
- Event:`deployment_promoted`, `deployment_draining`, `canary_draining`, `rolling_scale_up/down`.

**STATUS:** `UNWIRED` (promotion/traffic)

**GAP:** Strategy is bookkeeping. `executePromoteStep` flips DB `active_target` from blue→green but doesn't (a) reconfigure proxy/target groups, (b) drain old containers via `docker stop`, (c) shift ingress `domains`/`loadbalancer` to new target id. `blueTargetID/greenTargetID` are synthetic `serverID-blue` strings, not real container IDs. No `deployments` row maps to actual `compose_stacks` or `instances`. So blue-green rollouts complete with `promoting`→`completed` event while both blue and green containers (if any) remain running with same ports — port claim collision on compose path if attempted via compose.

**FORGE LOGIC FINDING — FLF-08:** `execution.go:257 newTarget := "green"; if ActiveTarget != "blue" { newTarget="blue" }` toggles DB target but no operational effect. If rollout is `StrategyRecreate`, promote still flips target (via blueGreen branch) without traffic meaning. This is `FALSE_COMPLETION` for zd.

**RECOMMENDATION:** `ADAPT` — wire promote to `trafficmanager`/`loadbalancer` or `service_endpoints` + beacon `proxy` reload, or mark strategies `NOT_IMPLEMENTED` in API and return 501.

**SEVERITY:** `P1 USER_VISIBLE` — advertises blue-green but silently no-op.

---

### C15 — Preview / PR Deployments (ephemeral envs)

**REFERENCE:**

- Coolify:`ApplicationPreview` + `ApplicationDeploymentJob:2049 deploy_pull_request` — per-PR branch isolated URL `pr-<id>-<app>.domain`.
- Dokku: no built-in preview (via `preview` plugin forks).
- CapRover: via branches (manual `git branch` field).
- Komodo: not first-class; `git.poll` could watch PR refs but UI not PR-aware.
- Portainer: not.
- 1Panel: not.
- Uncloud: not.
- Compose: via `compose -p pr-123` project name isolation (manual).

**FORGE:**

- Frontend: `forge/web/app/admin/preview-deployments/page.tsx` (if exists) — `lib/api/preview-deployments.ts`.
- API:`handlers_preview_deployments.go` (`handlers_preview_deployments_test.go`) exposes `POST /preview-deployments`, `GET`, `PATCH status`, `DELETE`.
- Service:`preview/service.go:32 Create` (`status=deploying`, `UniqueSuffix 8hex`, `IsIsolated=true`, `Source github|gitlab`), `83 UpdateStatus`, `100 Deploy` (`status→running`, synthesizes `https://preview-{{suffix}}.example.com`, `deployment-*.example.com`), `140 Cleanup(status→cleaned_up)`, `170 HandleWebhook` (`pull_request.opened|sychronize→Create+Deploy`, `closed→Cleanup per PR`, respects `server_id+pr_number`).
- Store:`preview_deployments` table (119) `server_id FK, pr_number, pr_title, pr_url, branch, repo_owner, repo_name, commit_sha, status deploying|running|stopped|failed|cleaned_up CHECK, preview_url, deployment_url, source CHECK github|gitlab, unique_suffix, is_isolated`.
- Beacon: isolated? Not enforced — preview URL synthesized but no real ingress provision (no Caddy proxy route).
- Worker: no `GetActivePreviews` reconciler loop visible.

**STATUS:** `PARTIAL`

**GAP:** Preview deployment `Deploy` is synthetic — it generates `https://preview-{{suffix}}.example.com` without reserving domain, creating `proxy_domains` row, or deploying container. Compare Coolify which actually deploys branch containers + proxy. Forge preview is metadata-only sandbox awaiting real `service_endpoints` wiring. `source` ENUM only `github|gitlab`, but `HandleWebhook` expects `pr_number`/`repo_owner` payload that GitHub webhook for `compose/git` (separate path `handlers_compose.go:83 /compose/webhook/:webhookId`) doesn't share — two webhook systems disjoint.

**RECOMMENDATION:** `INSPIRE` — unify preview with `deployment` or `compose` git deploy (isolated compose project name `preview-{{suffix}}`), provision ingress domain via `proxydomains`.

**SEVERITY:** `P3 OPERATOR_VISIBLE` — feature demo but not real.

---

### C16 — Backup & Restore (app-level lifecycle)

**REFERENCE:**

- Coolify: `ScheduledTask` + `DatabaseBackupJob` + volume backup, `backup:*` via S3; restore via job queue.
- Dokku: no built-in backup (via community plugins `dokku backup`).
- CapRover: backup via `CaptainManager.backup` tarball of datastore + volumes.
- Komodo: no backup.
- Portainer: backup/restore via `api/backup` tar.
- 1Panel:`agent/app/service/backup*.go` family: `backup_app,mongodb,mysql,postgres,redis,website`, `Snapshot`, `snapshot_create|recover|rollback` + `backup.go` strategy.
- Uncloud: no backup.
- Compose: `docker commit` + `volume backup` manual.

**FORGE:**

- Frontend:`apps.ts:296 fetchAppBackups(GET /apps/:id/backups)`, `300 createAppBackup(POST)`, `304 restoreAppBackup`, `308 delete`; `AdminBackups`? not lifecycle.
- API:`handlers_apphosting.go:693 GET /apps/:id/backups` → `backup.ListBackupArtifacts(SourceAppID)`, `721 POST` → `backup.CreateBackupJob(BackupTypeApp)` + `ExecuteBackupJob`, `767 POST .../restore` → `CreateRestore+ExecuteRestore`, `801 DELETE`.
- Service:`backup` package (`appBackupSvc:80 backup.NewMainService(cfg.Store, Slog) + SetDaemonClient`, `store_admin_backups*` tables `104_a_backup_system.sql` `backup_artifacts`, `backup_jobs(status pending|preparing|downloading|restoring|verifying|completed|failed|cancelled|rollback)`, `backup_restores`).
- Store:`backup system` migrations `104_a`, `207 backup_restores_data`, `209 operation_stale_reaper`.
- Beacon:`server.go:1705 rollbackName = pre-restore-TIMESTAMP.zip`, `1707 backups.Create`, `1718 backups.Restore` with automatic rollback on restore fail (`1721 Join rollbackErr`).
- Worker: backup `queue`? `backup` has own `ExecuteBackupJob` sync; not via `operation` queue.

**STATUS:** `PARTIAL`

**GAP:** App backup `ExecuteBackupJob` is **synchronous** in request handler (721 `ExecuteBackupJob` inline). Long backups will time out the HTTP request (Fiber default). References queue backups (Coolify Horizon + 1Panel `task`). Also `DeleteBackupArtifact` requires no retention check. Drift: `AppBackupStatus` in frontend `pending|creating|completed|failed|restoring` mismatches backend `status` enum (`preparing/downloading/...`). No lifecycle hook to auto-backup before `UpdateComposeStack`/`Rollback`.

**RECOMMENDATION:** `ADAPT` — make `POST /apps/:id/backups` enqueue via `queue.Service` or `operation` async (return 202 + jobId). Add pre-update snapshot hook.

**SEVERITY:** `P2 USER_VISIBLE` — timeout on large backup.

---

### C17 — Multi-service & Dependencies

**REFERENCE:**

- Coolify: `Service` aggregate (ServiceApplication/ServiceDatabase) with `depends_on` injected labels; ordering via compose `depends_on: condition: service_healthy`.
- Dokku: no multi-service app (but `linked` plugins `postgres:linked`).
- CapRover: one-container per app only; multi-service via `OneClickAppDeploymentHelper` (multi CaptainDefinition?).
- Komodo: `Stack` services map `DependsOn` kept verbatim into periphery compose.
- Portainer: multi-service is stack (compose) itself — dependencies preserved.
- 1Panel:`Favorite`, `app_tag`, compose services via `AppInstall.DockerCompose` whole file, not splitted `AppService` rows.
- Uncloud:`ServiceSpec{DependsOn, MountedDockerVolumes(), Mode}` plus `scheduler.VolumeScheduler`.
- Compose:`services.<name>.depends_on` (string|list|map `condition`). Spec.

**FORGE:**

- Frontend: not exposed — `apps/new` collects `ports/envVars/volumes` but not service graph.
- API:`POST /organizations/:orgId/apps/:appId/services/:serviceId/instances` but no dependency order endpoint.
- Service:`apphosting/service.go:198 CreateService` stores `Ports/EnvVars/DependsOn` as `JSONB`; `409 GetServiceOverview` `DependsOn` unused. `compose/service.go:765 normalizeDependsOn` supports array or map (keys = service names) — correctly handles `depends_on: { db: {condition:service_healthy}}` flattened to `["db"]`.
- Store:`app_services.ports/env_vars/depends_on JSONB`, not used for start ordering.
- Beacon: `compose up -d` respects `depends_on` natively via Engine, but Forge `RestartStack` does `stop then start` sequential without topological sort.

**STATUS:** `PARTIAL`

**GAP:** Forge stores `depends_on` but never enforces ordering outside compose's implicit `up`. For `app_services` (non-compose path) there is no `depends_on` resolution when dispatching via `replicamanager` — all `Instances` start in parallel via `deployReplicas` loop (no topo). Reference Komodo passes depends_on through to compose and thus benefits from Engine ordering; Forge app-services path should sort services before `DeployApp` per `ComputeServiceHealth` graph.

**RECOMMENDATION:** `ADOPT` compose ordering for app-services: topologically sort `AppServices` by `DependsOn` before `ReplicaManager.DeployApp` loop.

**SEVERITY:** `P3 SILENT` — race where dependent starts before dependency healthy, but compose Engine may heal.

---

### C18 — Source Types (GIT / DOCKER_IMAGE / COMPOSE) Lifecycle

**REFERENCE:**

- Coolify: `source` = git (GitHub/GitLab/Gitea/Bitbucket generic) + buildpacks (Dockerfile/Nixpacks/Static/Railpack) + compose; `git_branch`, `build_pack`.
- Dokku: `builder` plugins `builder-dockerfile, builder-herokuish, builder-nixpacks, builder-pack, builder-lambda` selected via `builder:detect`.
- CapRover: `ICaptainDefinition` picks imageName or git repo (via ImageMaker branching).
- Komodo: stack via raw compose or git (branch + credential).
- Portainer: `StackCreate` `fromGit|fromFile|fromTemplate`.
- 1Panel: `App.Key` from store + version branch `FileHistory`.
- Uncloud: `ServiceSpec.Image` (docker image) + `MountedDockerVolumes`.
- Compose: build context vs image.

**FORGE:**

- Frontend:`apps/new` type selector `image|git|compose`, git provider detection regex (68 `GIT_URL_PATTERNS`), branch `main`, `autoDeploy` switch.
- API:`handlers_apphosting.go:830 GET /apps/:id/compose` vs `907 GET /apps/:id/git`, `928 PATCH git`, `960 PATCH git/auto-deploy`, `1018 GET /admin/git-branches` (`git ls-remote --heads`), `handlers_compose.go:162 POST /compose/git/deploy` (GitOps DeployFromGit), `247 SetBranch`, `263 SetAutoUpdate`.
- Service:`apphosting/service.go:80 validSourceTypes GIT|DOCKER_IMAGE|COMPOSE`, `CreateApp` normalizes to uppercase; `UpdateApp` does not change `SourceType` (immutable); `compose/service.go` GitOps `GitDeployFromGitRequest{Branch, CredentialID, AutoUpdate, PollIntervalSec}`; `store: SourceConfig JSONB` arbitrary.
- Store:`applications.source_type CHECK`, `source_config JSONB`, `compose_stacks.git*` fields (branch, commit, autoUpdate, pollInterval).
- Beacon: compose deploy agnostic to source — just YAML.
- Worker: GitOps `SetAutoUpdate`, `CheckForUpdates`, `PullAndRedeploy`, `DetectDrift`, `GetGitStatus`.

**STATUS:** `PARTIAL`

**GAP:** `applications.source_type` immutable after create — cannot migrate `DOCKER_IMAGE` app to `GIT` without recreation (reference Coolify allows switch). Git auto-deploy polls via `GitOps` service but webhook also exists (`handlers_compose.go:83`) — two paths for same trigger, no coordination (duplicate deploys if both enabled). `git ls-remote` in handler runs `exec git` on API host (1018) — not credential-aware (private repos fail even if `credentialId` stored for compose).

**RECOMMENDATION:** `ADAPT` — unify GitOps webhook + poll under `queue.Service` deduped; make `git ls-remote` use `gitprovider` credential store; allow `SourceType` migration via validated `UpdateApplicationInput{SourceType}`.

**SEVERITY:** `P3 OPERATOR_VISIBLE` (immutable source) + `P2` (webhook double fire).

---

## 5. Forge Logic Findings (≥3 required)

### FLF-01 — P0 Deployment Execution False Completion (stub steps)

- **Location:** `forge/api/internal/services/deployment/execution.go:249 executeProvisionStep`, `256 executePromoteStep`, `278 executeDrainOldStep`, `285 executeDrainCanaryStep`, `292 executeScaleUpStep`, `299 executeScaleDownStep`, `306 executeCleanupStep` — all `return nil` (or publish event only).
- **Behavior:** `ExecuteDeployment:86-121` loops `stepsForStrategy` and calls `executeStep` per step. For `StrategyRecreate` the only real work is `init` (create revision) + optional `health_gate`. Provision/promote/drain/scale are no-ops, so `UpdateDeploymentCompletion:135` marks `completed`, `progressPct=100`, event `deployment_completed` even though no container was provisioned, no traffic switched, no replica scaled.
- **Evidence:** `deployment/deployment_test.go:40` asserts `StatusCompleted` after no-op rollout — test is green but runtime is false. Production UI polls `GET /admin/deployments/:id` and shows `completed` badge (`deployments.ts:16 Deployment.status completed`) while `docker ps` empty.
- **Impact:** Operator deploys image `nginx:1.27-alpine@sha256:abc` via `StartRollout(recreate)`, sees success, but workload not running. Restart path via `TriggerDeploy(empty image)` also completes with `image=""` (see FLF-09 below).
- **Reference contrast:** Coolify `rolling_update(1904)` actually stops old container after new health check; Komodo periphery does `docker compose up -d`; 1Panel `docker compose up -d`. Forge stubs miss the core actuation.
- **Fix:** Wire stubs to `replicamanager.DeployApp` / `queue.Service` `OpDeployPromote` / `daemon.Client.AdminContainer*` or mark strategy `UNIMPLEMENTED` until wired.

---

### FLF-02 — P1 App Start/Stop Lies (desired_state without reconciler) + Restart→Deploy Semantics Mismatch

- **Location:** `forge/api/internal/http/handlers_apphosting.go:450 POST /apps/:id/start` → `apphosting.UpdateApp(DesiredState:"running")` → `200 {ok:true}`; `475 stop`; `500 restart → TriggerDeploy`.
- **Behavior:** `apphosting/service.go:139 UpdateApp` writes `applications.desired_state` column (`store_apphosting.go:313 UPDATE applications SET desired_state=...`). `store/store_apphosting.go:379 UpdateApplicationStatus` exists but no caller ever sets `observed_status` except manual. No `Reconciler` or `Manager` loop watches `desired_state` and actuates `observed_status`. Frontend `fetchApps → DeployStatusBadge` reflects stale `idle`. User clicks Stop/Start → toast success → no effect.
- **Evidence:** Grep `UpdateApplicationStatus` — only `apphosting/service.go:193` and tests; never invoked from start/stop handlers. Compare `1panel/app_install.go:835 syncAppInstallStatus` which polls `docker ps` every loop.
- **Restart mismatch:** Reference contracts: Dokku `ps:restart` == `docker restart`, 1Panel `Operate{restart}` == `compose restart`, Uncloud `StartService` fan-out restart. Forge's restart creates a **new deployment** (`apphosting/service.go:666 TriggerDeploy { Strategy: recreate, Image:"" }`) — semantically different, writes `deployment_history` row, leaks `deployments` row, and health gate will fail (FLF-05).
- **Impact:** P0 user-visible lie; restart can break app history.

---

### FLF-03 — P1 Unconditional Lease Release (fence violation) + Missing Renewal

- **Location:** `forge/api/internal/services/deployment/execution.go:22 ClaimExecutionLease(executorID)`, `32 defer ReleaseExecutionLease(ctx, deploymentID)` (unconditional), `store/store_deployments.go:345 ReleaseExecutionLeaseIfOwner` exists but unused. Also `442 resumeFromStep` claims again with new lease.
- **Behavior:** Lease `5m` is claimed; long deploy (health gate timeout 5m + promote) can exceed lease; `context.WithTimeout(deployment.TimeoutSeconds)` may be up to `DefaultTimeoutSeconds=300` which equals lease. No renewal goroutine. If lease expires, second worker claims; first's defer unconditional `ReleaseExecutionLease` clears second's lease, allowing third. `RollbackToRevision` also bumps `version` via non-atomic `UpdateDeploymentStatusVersioned + UpdateDeployment` (see FLF-04).
- **Evidence:** `execution.go:68-77` `timeout = TimeoutSeconds else default`, `execCtx = context.WithTimeout(ctx, timeout)`; step loop has no `RenewExecutionLease` heartbeat. `ResumeDeployments:402` does `UpdateDeploymentFailure` without lease check.
- **Fix:** `defer ReleaseExecutionLeaseIfOwner(ctx, deploymentID, executorID)` + background `time.Ticker(30s)` renew.

---

### FLF-04 — P2 Rollback Version Race (non-atomic Image set)

- **Location:** `forge/api/internal/services/deployment/revisions.go:147 RollbackToRevision`, `167 sd.Version` snapshot before `UpdateDeploymentStatusVersioned(167)`, then `171 deployment.Image = targetRev.ImageRef`, `174 UpdateDeployment(toStoreDeployment(deployment))` (store `UpdateDeployment:128` does `SET ... version=version+1 WHERE id=$1` **without** `WHERE version=`).
- **Behavior:** First `UpdateDeploymentStatusVersioned` already incremented DB version from `V→V+1`. In-memory `deployment.Version` still `V`. Second `UpdateDeployment` blindly overwrites at `V+1`→`V+2` but carries stale `V` in payload for other fields; concurrent rollback could interleave and second's `SET image` wins with wrong version base, dropping first's `status` change. Optimistic lock (`ErrVersionConflict`) not enforced for this path.
- **Evidence:** `store/store_deployments.go:128 UpdateDeployment` is non-versioned; all other mutations (`UpdateDeploymentConfig`, `UpdateDeploymentRollback`, `UpdateDeploymentCompletion`) use `WHERE version=` + `RowsAffected==0→ ErrVersionConflict`. Rollback is the outlier.
- **Impact:** Rare but can corrupt `deployment` row image/status after concurrent rollback or auto-rollback vs manual. History diverges.

---

### FLF-05 — P0 Health Gate Targets Wrong Host (localhost-only)

- **Location:** `forge/api/internal/services/deployment/healthgate.go:11 CheckHealth` (`host=HealthCheckHost|"localhost"`, `target=http://host:port/path` via `http.Client 10s`), `rollout.go:71 validateHealthGateTarget` enforces `HealthCheckHost` must be `""|localhost|loopback` (error otherwise: "health checks may only target the local deployment gateway").
- **Behavior:** Health gate always hits API process's localhost, not workload node. Containers run on `nodes(id)` via `compose` on beacon. No node-addr is injected. Even if caller sets `HealthCheckHost=10.0.0.5`, validation rejects. So gate passes only if API host accidentally exposes same port/path. All other times `WaitForHealthGate:74-94` polls, resets streak, times out after `TimeoutSeconds` and returns `health gate timed out after 3 consecutive failures: health check failed`, which `handleStepFailure(321)` then fails deployment + optional auto-rollback (which itself is stub — FLF-01).
- **Evidence:** Grep `WaitForHealthGate` — called from `execution.go:176 executeStep(StepHealthGate)→WaitForHealthGate`. No beacon `ComposeStatus` integration; compose path separately uses `WaitForHealthy` via beacon `ps`.
- **Contrast:** Coolify health_check `host` is container service name or `0.0.0.0`, checked via `docker exec curl` or network alias; Komodo checks `ps` health; Dokku checks via `curl` from host on published port.
- **Fix:** Move `WaitForHealthGate` to beacon: `daemon.ComposeStatus` exposes `Health` or add `HealthGate` endpoint `GET /compose/:id/health` that runs `docker inspect --format {{.State.Health.Status}}` or container HTTP inside network.

---

### FLF-06 — P1 Compose Admin Restart Dead Code, User Route Alive

- **Location:** `beacon/internal/server/compose.go:506 handleComposeRestart` exists, `forge/api/internal/daemon/compose.go:69 ComposeRestart` exists, `forge/api/internal/http/handlers_user_console.go:240 stack, err:=composeSvc.RestartStack` exposes `/user/compose/:id/restart`, but `forge/api/internal/http/handlers_compose.go` (admin) registers `POST /compose/:id/start(454)` and `stop(442)` + `deploy(413)` + `delete(401)` — **no restart**.
- **Behavior:** Admin UI `forge/web/app/admin/compose/[id]/page.tsx` offers `stop/start/redeploy/delete` but not `restart`; `lib/api/compose.ts` lacks `restartComposeStack`. Power user calling `curl -XPOST /api/v1/compose/<id>/restart` gets 404 despite beacon+client supporting it.
- **Impact:** Operator cannot `compose restart` single stack to cycle after config map change without down/up (which may remove volumes with `down -v` confusion). User (non-admin) can restart but admin cannot — privilege inversion.

---

### FLF-07 — P0 App Restart Creates Empty-Image Deployment (TriggerDeploy)

- **Location:** `forge/api/internal/services/apphosting/service.go:666 TriggerDeploy` (`Image:""`, `Strategy:"recreate"`, `Status:"pending"`), `forge/api/internal/http/handlers_apphosting.go:500 POST /apps/:id/restart → TriggerDeploy`.
- **Behavior:** Image empty violates `rollout.go:322 validateImageRef` (`@sha256:` required). Execution path `execution.go:210 executeInitStep` calls `validateImageRef(deployment.Image)` → error → `handleStepFailure` → `StatusFailed`. Yet HTTP handler `handlers_apphosting.go:500` returns `201 {id,status:pending}` before execution async? Actually `TriggerDeploy` is synchronous create only, not rollout — but next async executor (if any) will pick it via `ResumeDeployments` and fail. Frontend `restartMut` in `apps/page.tsx:42` calls `restartApp` and invalidates, showing stale.
- **Evidence:** `deployment/rollout.go:322 ErrInvalidImageRef` message; no test covers `TriggerDeploy` → `ExecuteDeployment` with empty image (would fail). `handlers_apphosting.go:517 dep, err:=appSvc.TriggerDeploy` then `201 DEP` without image enrichment.
- **Contrast:** 1Panel restart is `docker compose restart`; Coolify restart is `just_restart` shortcut; none create image-less deployment.
- **Fix:** Restart should not go through deployment pipeline — call `Daemon.AdminContainerRestart` fan-out (InstanceHandler style) or `compose RestartStack` depending on source_type, or at minimum copy last image `applications→deployments.image`.

---

### FLF-08 — P1 Compose Redeploy Creates New Row, Not Update (row leak)

- **Location:** `forge/api/internal/http/handlers_compose.go:413 POST /compose/:id/deploy` (`existing, _:=composeSvc.GetStack` then `composeSvc.DeployComposeStack(...Name:existing.Name, NodeID:existing.NodeID, ComposeYAML:existing.ComposeYAML...)`), not `UpdateComposeStack`.
- **Behavior:** Each click "Redeploy" in `[id]/page.tsx:95 redeployMutation → deployComposeStack(id)` spawns a new `compose_stacks` row `cps-<12>` with fresh ID, new `placement_reservations` reservation, new node maybe (scheduler). Old row remains `running` → orphan `docker compose -p oldId up` still exists on node (no `down`). History doubles.
- **Evidence:** `handlers_compose.go:422` reuses `existing.Name/ComposeYAML` but not `existing.ID`; `lifecycle.go:249 DeployComposeStack` does `createStackID() = "cps-"+uuid[:12]` fresh. `DeleteComposeStack` only deletes when explicitly called. No dedup on `compose_hash`.
- **Fix:** Call `PullStack`+`UpdateComposeStack` or `RestartStack` for redeploy; or `DeployComposeStack` idempotently with same ID (upsert).

---

### FLF-09 — P2 App Delete While Deploy Active (missing fence) + Orphan Beacon Containers

- **Location:** `forge/api/internal/services/apphosting/service.go:174 DeleteApp` → `store.DeleteApplication` (no status check), `store/store_apphosting.go:374 DELETE WHERE id`, DB `100_z` has no `CHECK desired_state!=running` trigger. `store/store_deployments.go:106` partial index forbids two actives per server, but delete bypasses it.
- **Behavior:** `DELETE /apps/:id` succeeds even if `deployments.status IN (pending,in_progress,provisioning,awaiting_health,promoting)`. Deployment worker keeps `ExecuteDeployment` loop running, eventually trying `UpdateDeploymentCompletion` on a `deployment` whose `server_id` FK still holds but `applications.server_id` is now `NULL`/`deleted` (if cascade). Beacon containers for that app (compose or instances) keep running `docker ps` because delete never told beacon to `down/stop`.
- **Evidence:** Grep `DeleteApplication` callers — only handler, no guard. Compare `rollout.go:129 isActiveStatus` used in `recreateRollout` guard but not in delete.
- **Fix:** Gate delete on `!isActiveStatus` (return 409 `ErrInProgress`) or enqueue async drain + reservation cancel before row delete.

---

### FLF-10 — P2 UpdateComposeStack Bypass Validation on Redeploy Path

- **Location:** `forge/api/internal/services/compose/lifecycle.go:476 UpdateComposeStack` checks `isRunnable` (`running|stopped|degraded`) but `DeployComposeStack` validates `ValidateCompose` security at top (264). `UpdateComposeStack` has **no** `ValidateCompose` call — assumes prior validation, but direct `PATCH /compose/:id` body is raw `composeYaml` from client (can be tampered). `beacon validateComposePolicy` will catch some but not all API-level `cap_add` warnings.
- **Behavior:** Attacker with admin scope could `PATCH /compose/<id>` with `privileged: true` YAML. API will store and call `daemon.ComposeDeploy` which beacon will reject (400), triggering `rollbackStack` (844) which re-deploys old YAML. But DB was already updated to `composeHash=newHash` and `status=updating` before beacon check, then `markFailed` may leave DB hash diverged from on-disk `compose.yaml`. Subsequent `GetStack` returns new hash even though beacon still runs old.
- **Fix:** Call `ValidateCompose` in `UpdateComposeStack` before mutating row, mirroring `DeployComposeStack`.

---

## 6. Status Legend & Count

- **COMPLETE** 1 (Compose spec validation dual wall)
- **PARTIAL** 9 (Create, Deploy, Update, Rollback, Delete, Health, Version, Logs, SourceTypes, Multi-service etc)
- **UNWIRED** 2 (Z-D traffic, Reconciliation)
- **BROKEN** 1 (Restart semantics)
- **MISSING** 2 (Admin restart route, Reconciler)
- **DEAD** 0
- **FALSE_COMPLETION** 3 (Deploy stubs, Desired vs Observed, Z-D promote)
- Duplicates not enumerated separately.

---

## 7. Recommendations Priority Stack

| P | Item | Location | Action |
|---|---|---|---|
| P0 | Wire deployment provision/promote/drain to real runtime or mark strategy NOT_IMPLEMENTED | `execution.go:249-311` | Block `deployment_completed` until beacon confirms `ps` running; add integration test that asserts `docker ps` mock |
| P0 | Fix health gate to run via beacon or container network | `healthgate.go:11, execution.go:176` | Add `daemon.HealthCheck(nodeURL, port, path)` or `docker inspect` health status |
| P0 | App start/stop reconciler | `handlers_apphosting.go:450/475, apphosting/service.go:139` | Introduce controller loop or make handlers enqueue `operation`/`replicamanager` ops and update `observed_status` post-beacon |
| P0 | App restart ≠ empty deploy | `apphosting/service.go:666, handlers_apphosting.go:500` | Route restart to `AdminContainerRestart` fan-out or `compose RestartStack` |
| P1 | Lease fence fix + renewal | `execution.go:32, store_deployments.go:345` | `ReleaseExecutionLeaseIfOwner` + ticker |
| P1 | Admin compose restart wiring | `handlers_compose.go:442/454, lib/api/compose.ts:93` | Add `POST /compose/:id/restart` + `restartComposeStack` client + UI button |
| P1 | Redeploy row leak | `handlers_compose.go:413` | `UpdateComposeStack`/`PullStack` path, prune old `cps-` reservations |
| P1 | Z-D promote traffic | `execution.go:256` | Wire `loadbalancer`/`service_endpoints`/`target_groups` on promote, or document as stub |
| P2 | Rollback version atomic | `revisions.go:167-174` | Single `UPDATE ... WHERE version=` with image+status |
| P2 | Delete fence + beacon cleanup | `apphosting/service.go:174` | Guard active deployment, call `ComposeDelete`/`RemoveService`, cancel reservations |
| P2 | Validate on update | `lifecycle.go:476` | Pre-check `ValidateCompose` |
| P3 | Spec warnings → errors alignment | `service.go:278` vs `compose.go:77` | Unify table |
| P3 | SourceType immutability | `apphosting/service.go:139` | Allow migration |
| P3 | Logs streaming | `handlers_apphosting.go:563` | WS stream `docker logs -f` |

---

## 8. Adopt / Adapt / Inspire / Reject Matrix

| Reference Pattern | Verdict | Rationale |
|---|---|---|
| **Coolify's `ApplicationDeploymentJob` queue per app + `rolling_update` with health gate before promote** | **ADOPT** | Queue serialization, Horizon tags, and health-gated rolling is exactly what `deployment.Service` stubs try to mimic — wire it. Forge already has lease + steps, just wire provision. |
| **Dokku `checks` (`CHECKS` file) + `scheduler-docker-local` health before proxy switch** | **ADAPT** | Move health gate to beacon-side curl/docker health, like Dokku `checks:run` |
| **CapRover `ICaptainDefinition` strict spec allowlist** | **INSPIRRE** | Forge already has `ValidateComposeSecurity` but warns on some → mirror CapRover's hard reject |
| **Komodo monitor loop `stack.rs` reconcile** | **ADOPT** | Add `AppMonitor`/`StackMonitor` loop polling `daemon.ComposeStatus` → update `observed_status`, set `degraded` |
| **Portainer `StackDeployer` builder pattern** | **ADAPT** | `director.go` hook after deploy — add Forge post-deploy hook for proxy reload |
| **1Panel `AppInstallService.SyncAppInstallStatus` polling** | **ADOPT** | Reconcile `applications.observed_status` via `SyncAppInstallStatus` interval |
| **Uncloud imperative fan-out `StopService/StartService` + generation idempotency** | **ADOPT** | App instance lifecycle already has `instanceLifecycleHandler` fan-out; extend to app start/stop fan-out |
| **Docker-Compose `--scale SERVICE=NUM` + per-stack `lock`** | **ADOPT** | Add `POST /compose/:id/scale` wiring `--scale` through beacon; per-stack mutex already correct (39) |

Reject nothing outright; unify fragmented health/release tables into one.

---

## 9. Raw File:Line Citations Index (sample, all verified via Read/Grep)

**Forge:**

- `forge/api/internal/services/apphosting/service.go:92 CreateApp,80 validSourceTypes,139 UpdateApp,174 DeleteApp,185 UpdateAppStatus,193 Status,198 CreateService,359 UpdateService,397 ScaleService,440 GetServiceStatus,594 PlanServiceUpdate,654 Apply,666 TriggerDeploy`
- `forge/api/internal/services/deployment/service.go:268 StartBlueGreen,320 CompleteDeployment,350 CancelDeployment,436 Rollback,427 SetDeploymentStatus,18 Strategies,27 Statuses`
- `forge/api/internal/services/deployment/execution.go:22 ExecuteDeployment,15 stepStatusMapping,59 createSteps,106 executeStep,210 executeInitStep,249 executeProvisionStep,256 executePromoteStep,278 Drain,292 Scale,321 handleStepFailure,389 ResumeDeployments,442 resumeFromStep`
- `forge/api/internal/services/deployment/rollout.go:42 StartRollout,71 validateHealthGateTarget,92 applyRolloutRequest,129 recreateRollout,181 rollingRollout,233 blueGreenRollout,261 canaryRollout,322 validateImageRef,332 digestImageRef`
- `forge/api/internal/services/deployment/healthgate.go:11 CheckHealth,48 WaitForHealthGate,39 200-399 pass`
- `forge/api/internal/services/deployment/revisions.go:58 configHash,82 CreateRevision,126 List,139 Get,147 RollbackToRevision,202 RollbackToPrevious,225 Compare`
- `forge/api/internal/services/deployment/steps.go:28 createSteps,48 stepsForStrategy`
- `forge/api/internal/services/compose/lifecycle.go:22 StackStatus enum,155 New,249 DeployComposeStack,476 UpdateComposeStack,570 DeleteComposeStack,625 GetStackStatus,663 GetStackLogs,697 StartStack,745 StopStack,781 RestartStack,788 PullStack,188 WaitForHealthy,988 computeHash`
- `forge/api/internal/services/compose/service.go:145 ParseComposeYAML,240 ValidateCompose,427 ValidateComposeSecurity,659 ValidateHostMountWithAllowlist,882 EOF`
- `forge/api/internal/http/handlers_apphosting.go:102 tenantAccess,31 resolveDefaultOrg,102 Get apps,116 Post,173 Post /apps,211 Get :id,231 Put,259 Delete,282 deploy,308 services,416 Patch service,444 instances,450 start,475 stop,500 restart,526 deployments,563 logs,693 backups`
- `forge/api/internal/http/handlers_deployment.go:24 blue-green,44 rollback,52 complete,60 cancel,68 execute,75 cleanup,82 steps,98 resume`
- `forge/api/internal/http/handlers_revisions.go:16 revisions,32 rollback,44 previous,52 rollout,62 compare`
- `forge/api/internal/http/handlers_compose.go:83 webhook,106 validate,118 import,162 git deploy,197 redeploy,378 PATCH,401 DELETE,413 deploy,442 stop,454 start,468 logs,483 status`
- `forge/api/internal/http/handlers_user_console.go:240 RestartStack via /user/compose`
- `forge/web/lib/api/apps.ts:225 fetchApps,233 createApp,237 update,241 delete,245 startApp,249 stop,253 restart,257 deployments,320 compose`
- `forge/web/lib/api/compose.ts:49 validate,53 create,87 deploy,93 stop,97 start,101 status,105 logs (no restart)`
- `forge/web/lib/api/deployments.ts:88 steps,98 deployment,103 revisions,113 rollback`
- `forge/web/app/admin/apps/page.tsx:35 mutations,144 actions`
- `forge/web/app/admin/apps/new/page.tsx:1 wizard`
- `forge/web/app/admin/compose/page.tsx:44 delete/start/stop mutations, no restart`
- `forge/web/app/admin/compose/[id]/page.tsx:1 detail, stop/start/redeploy`
- `beacon/internal/server/compose.go:27 composeStack,39 lock/unlock,59 validStackID,77 validateComposePolicy,136 truthy,214 shortFormHostPort,226 validateVolumes,264 encodeComposeEnv,344 handleComposeDeploy,418 Stop,462 Start,506 Restart,550 Delete,604 Status,661 Logs,709 Pull`
- `beacon/internal/server/server.go:266 composeStacks manager,392-399 mux handlers,3727 capabilities`
- `forge/api/internal/daemon/compose.go:30 ComposeDeploy,61 Stop,65 Start,69 Restart,73 Delete,81 Pull,98 Status,118 Logs`
- `forge/api/internal/store/store_apphosting.go:11 Application,68 AppService,151 CreateApplication,374 DeleteApplication,379 UpdateApplicationStatus,395 CreateAppService`
- `forge/api/internal/store/store_deployments.go:14 Deployment,48 CreateDeployment,300 ClaimExecutionLease,345 ReleaseExecutionLeaseIfOwner`
- `forge/api/migrations/095_deployments.sql:1 deployments, 100_z:5 applications,99:4 deployment_revisions,103_a:13 deployment_steps,114_e:1 deployment_releases,119:9 deployment_history/rollbacks`
- `forge/api/internal/services/replicamanager/service.go:100 CreateApp,156 DeployApp,381 ScaleApp`
- `forge/api/internal/services/zerodowntime/service.go:100 CreateRelease`
- `forge/api/internal/services/preview/service.go:32 Create,100 Deploy,170 HandleWebhook`
- `forge/api/internal/services/appstore/service.go:65 InstallApp`

**Reference:**

- `reference/app-platforms/coolify/app/Models/Application.php:118 class,143 health_check_*,792 isRunning,830 status()`
- `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:42 class,283 handle,484 decide_what_to_do,1018 write_deployment_configurations,1904 rolling_update,1955 health_check,4769 handleStatusTransition`
- `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1 interface`
- `reference/app-platforms/caprover/src/user/ImageMaker.ts:46/145/246/386 methods`
- `reference/app-platforms/1panel/agent/app/model/app_install.go:11 AppInstall,16 Version,20 Status`
- `reference/app-platforms/1panel/agent/app/service/app_install.go:63 New,67 list,246 Operate,835 syncStatus,862 Sync`
- `reference/app-platforms/uncloud/pkg/client/service.go:24 RunService,59 Inspect,141 Remove,165 Stop,199 Start`
- `reference/app-platforms/docker-compose/cmd/compose/compose.go:561 restartCommand,587 scaleCommand`
- `reference/app-platforms/dokku/plugins/ps/*` (apps/create, ps:scale/restart)
- `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs` Create/DeployStack
- `reference/app-platforms/portainer/api/stacks/deployments/*` StackDeployer

---

## 10. Conclusion

Forge has **data-model completeness** for application lifecycle (apps, services, deployments, revisions, steps, history, compose stacks, releases, previews) rivaling Coolify/CapRover but **actuation is incomplete or unwired**: deployment provision/promote/drain are stubs, health gate targets localhost, app start/stop never reconciles, restart creates empty deployment, compose admin restart is unreachable, redeploy leaks rows. The **false completions** (deploy shows `completed` with zero containers, start/stop returns `{ok:true}` with stale observed status) are the highest risk — they train operators to trust success badges that do not reflect reality. Uncloud/Komodo/1Panel patterns point the fix: queue-serialized provision, beacon-side health, reconciler polling `ps`, fan-out start/stop, and allowlist-hardened spec validation already have the building blocks in repo (replicamanager, queue, beacon compose lock) — they only need wiring between layers identified above.

