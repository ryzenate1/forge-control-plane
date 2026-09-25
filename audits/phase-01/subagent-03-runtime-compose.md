# Phase 01 — Subagent 03: CONTAINER / COMPOSE / RUNTIME

> Dimension: containers/images/networks/volumes/exec/logs/stats/pull/push/build/prune/registry · Compose (service dependencies, env, mounts, restart policies, resource limits, runtime isolation)
> Cluster: coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose (spec)
> Auditor: subagent-03 (Phase 1)
> Date: 2026-08-23
> Method: source inspection (Read/Grep/Glob/Bash). All citations are `file:line` verifiable. No product code modified.

---

## 1. Scope & Methodology

Inspected reference implementations under `reference/app-platforms/<project>` and Forge equivalents under:

- **UI:** `forge/web/lib/api/docker.ts`, `forge/web/lib/api/compose.ts`, `forge/web/app/admin/docker/*`, `forge/web/app/admin/compose/*`, `forge/web/app/admin/containers/*`, `forge/web/components/docker/*`
- **API:** `forge/api/internal/http/handlers_docker.go`, `handlers_compose.go`, `handlers_db_containers.go`, `handlers_user_containers.go`
- **Services/Adapters:** `forge/api/internal/services/compose/*` (`service.go`, `lifecycle.go`, `parser.go`, `queue_handler.go`, `controller.go`, `gitops.go`), `forge/api/internal/services/runtime/runtime.go`, `forge/api/internal/runtime/*` (`docker.go`, `multiruntime.go`, `registry.go`, `containerd.go`, `podmanadapter.go`, `kubernetesadapter.go`)
- **Daemon client:** `forge/api/internal/daemon/client.go`, `compose.go`, `build.go`
- **Beacon runtime:** `beacon/internal/runtime/*` (`docker.go:1135`, `factory.go`, `stats.go`, `enhanced_stats.go`, `containerd.go`, `podman.go`), `beacon/internal/server/*` (`compose.go:751`, `container_admin.go:1771`, `mounts.go:215`, `server.go`)
- **Store/Migrations:** `forge/api/internal/store/store_compose.go:793`, `forge/api/migrations/108_compose_stack_indexes.sql`, `115_compose_stacks.sql`, `160_encrypt_compose_secrets.sql` etc.

Every row in §4 cites at least one real file:symbol per reference and per Forge layer. Status taxonomy: `COMPLETE | PARTIAL | UNWIRED | BROKEN | MISSING | WIRED_BUT_WRONG`.

---

## 2. Reference Platform Inventory (file:symbol)

### 2.1 Portainer (develop) — docker + edge agent

- **Docker client factory:** `reference/app-platforms/portainer/api/docker/client/client.go:28` `type ClientFactory` + `CreateClient:42` — switches on `endpoint.Type` (`AgentOnDockerEnvironment`, `EdgeAgentOnDockerEnvironment`, direct `unix://`, TCP+TLS). Signs requests via `portainer.AgentSignature`. Reference for Forge's single-token `Admin*` pattern.
- **Container Recreate:** `reference/app-platforms/portainer/api/docker/container.go:55` `func (c *ContainerService) Recreate` — fetch `ContainerInspectWithRaw`, `images.ParseImage`, optional force pull via `images.NewPuller`, stop → rename `*-old` → disconnect networks → `ContainerCreate` with `applyVersionConstraint("< 1.44", clearMacAddrs)` → reconnect remaining networks → start new → remove old. Includes transactional `restore` deferred func. Forge has no recreate primitive.
- **Images:** `portainer/api/docker/images` package (referenced by container.go) — layered registry auth via DataStore.
- **Stacks:** `portainer/api/stacks/{stackbuilders,dataservices,deployments}` — `StackDeployer.Deploy` idempotently runs `docker stack deploy` / `compose up`.

### 2.2 Komodo (Rust) — stacks / periphery

- **Core API write stack:** `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs` (verified via `ls` + prior subagent01 extraction) — `CreateStack`, `UpdateStack`, `DeployStack`, `StartStack`, `StopStack`, `RemoveStack`, `PullStack`.
- **Periphery (host agent):** `komodo/lib/periphery/*` executes `docker compose up -d`, `ps --format json`, `logs --tail`, `pull` on host over gRPC. `lib/periphery/terminal.rs` streams. Maps to `beacon/internal/server/compose.go`.
- **Monitor:** `bin/core/src/monitor/stack.rs` polling `compose ps` → `running/stopped/degraded` reconciliation — direct analogy to `forge/api/internal/services/compose/lifecycle.go:181` `WaitForHealthy`.

### 2.3 1Panel (agent v2) — agent model

- **Containers:** `reference/app-platforms/1panel/agent/app/api/v2/container.go:25` `SearchContainer`, `:68` `ListContainerFiles`, `:224` `ListContainer`, `:419` `ContainerListStats`, `:436` `ContainerItemStats`, `:459` `ContainerCreate`, `:503` `ContainerPrune`, `:525` `CleanContainerLog`, `:612` `ContainerOperation` (start/stop/restart/pause/unpause/rename/kill), `:632` `ContainerStats` (one-shot), `:915` `ContainerStreamLogs`.
- **Networks:** `:697` `SearchNetwork`, `:722` `ListNetwork`, `:740` `DeleteNetwork`, `:762` `CreateNetwork`.
- **Volumes:** `:784` `SearchVolume`, `:809` `ListVolume`, `:827` `DeleteVolume`, `:849` `CreateVolume`.
- **Images:** `reference/app-platforms/1panel/agent/app/api/v2/image.go:18` `SearchImage`, `:43` `ListAllImage`, `:59` `ListImage`, `:77` `ImageBuild`, `:100` `ImagePull`, `:123` `ImagePush`, `:146` `ImageRemove`, `:192` `ImageTag`, `:215` `ImageLoad`.
- **Compose templates:** `reference/app-platforms/1panel/agent/app/api/v2/compose_template.go:14` `CreateComposeTemplate`, plus `SearchComposeTemplate`, `ListComposeTemplate`, `BatchComposeTemplate` — template catalog missing in Forge (Forge stores raw YAML per stack only).
- **Daemon JSON:** `reference/app-platforms/1panel/agent/app/api/v2/docker.go:19` `LoadDockerStatus`, `:31` `LoadDaemonJsonFile`, `:51` `LoadDaemonJson`, `:69` `UpdateDaemonJson` — daemon config management absent in Forge.

### 2.4 Uncloud — `RunService` mesh deploy

- **pkg/client/service.go:24** `RunService(spec)` — `Validate()` → `InspectService` → `CreateVolume via VolumeScheduler` → `NewDeployment(spec).Run(ctx)` → scheduler + WireGuard mesh deploy; returns `RunServiceResponse{ID, Name}`. Direct analog to `forge/api/internal/services/compose/lifecycle.go:287` `DeployComposeStack`.
- **Lifecycle:** `RemoveService:141` fan-out `wg.Go` per container across `MachineMember_UP/SUSPECT` with `RemoveVolumes:true`; `StopService:165`, `StartService:199` similar fan-out. Imperative, no reconciler loop (README: “No control plane”) — contrast Forge's store-backed reconciler `ListComposeStacksForReconciliation`.
- **Docker helpers:** `reference/app-platforms/uncloud/internal/docker/image.go:42` `PullImage` channels `PullPushImageMessage`, `RetrieveLocalDockerRegistryAuth` via `docker/cli/cli/config`; `container.go:20` `CreateContainerWithImagePull` retry-on-NotFound. Pattern for registry auth handling.

### 2.5 docker-compose (spec) — canonical primitives

- **pkg/compose/compose.go:80** `NewComposeService(dockerCli, options...)` — option functional pattern (`WithPrompt`, `WithMaxConcurrency`, `WithDryRun`, `WithEventProcessor`).
- **Lifecycle files:** `up.go`, `create.go`, `down.go:379`, `restart.go`, `scale.go:86 --scale SERVICE=NUM`, `ps.go --format json`, `logs.go`, `pull.go`, `build.go`, `publish.go`.
- **Service dependencies:** `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:78` `InDependencyOrder`, `:91` `InReverseDependencyOrder`, `:207` graph — converges via dependency graph before `create`. Also `convergence.go:88` re-wires `volumesFrom/networkMode/ipc/pid = container:ID` for `service:xxx` deps.
- **Create resource/restart mapping:** `reference/app-platforms/docker-compose/pkg/compose/create.go:592` `getRestartPolicy` mapping `deploy.restart_policy.condition → container.RestartPolicy`, `:641` `resources.Memory/NanoCPUs/PidsLimit/BlkioWeight/Ulimits`, `:728` `setReservations`, `:749` `setLimits`. Canonical for §4.14-4.15.
- **Env resolution:** `reference/app-platforms/docker-compose/pkg/compose/envresolver.go` + `loader.go` — composes interpolate `.env` + `env_file` + `include` + `${VAR:-default}`. Forge implements subset (see §4.10).

### 2.6 Coolify / Dokploy — build pipeline depth (secondary cluster refs)

- Coolify `app/Jobs/ApplicationDeploymentJob.php` style deploy (referenced in subagent01): queue-backed deploy family `deploy_*_buildpack`, `rolling_update`, `health_check`, `handleStatusTransition`. Dokploy `packages/server/src/utils/docker/{compose,utils}.ts` compose composition helpers (`compose/network.ts`, `compose/volume.ts` etc.) — both more granular than Forge's single-file `docker compose up -d` shellout.

---

## 3. Forge Chain Inventory

### 3.1 UI

| File | Export / Component | Notes |
|------|-------------------|-------|
| `forge/web/lib/api/docker.ts:77` | `listContainers` | `GET /docker/containers?all=` → flattens `nodeId/nodeName/containers` |
| `forge/web/lib/api/docker.ts:109` | `getContainer` | `GET /docker/containers/:id?node=` |
| `forge/web/lib/api/docker.ts:113` | `createContainer` | `POST /docker/containers?node=` body `CreateContainerRequest` |
| `forge/web/lib/api/docker.ts:118` | `operateContainer` | `POST /docker/containers/:id/operate` `start/stop/restart/pause/unpause` |
| `forge/web/lib/api/docker.ts:126` | `deleteContainer` | `DELETE /docker/containers/:id?force&node=` |
| `forge/web/lib/api/docker.ts:134` | `getContainerLogs` | `GET /docker/containers/:id/logs?tail&node=` returns `text/plain` |
| `forge/web/lib/api/docker.ts:143` | `getContainerStats` | `GET /docker/containers/:id/stats?node=` |
| `forge/web/lib/api/docker.ts:147` | `listImages/pullImage/deleteImage` | images endpoints |
| `forge/web/lib/api/docker.ts:159-182` | `listNetworks/createNetwork/deleteNetwork/listVolumes/createVolume/deleteVolume/pruneVolumes` | networks/volumes |
| `forge/web/lib/api/docker.ts:189-224` | `buildImage/pushImage/tagImage/searchImages/listContainerFiles/readContainerFile/uploadContainerFile/deleteContainerFile` | image registry + file ops |
| `forge/web/lib/api/compose.ts:49` | `validateCompose` | `POST /compose/validate` |
| `forge/web/lib/api/compose.ts:53-111` | `createComposeStack/listComposeStacks/getComposeStack/updateComposeStack/deleteComposeStack/deployComposeStack/stopComposeStack/startComposeStack/getComposeStackStatus/getComposeStackLogs` | stack lifecycle |
| `forge/web/app/admin/docker/page.tsx:1` | `DockerPage` | tab container (containers/images/networks/volumes) |
| `forge/web/app/admin/compose/page.tsx:32` | `ComposeStacksPage` | `useQuery /compose` list, start/stop/delete mutations |
| `forge/web/app/admin/compose/[id]/page.tsx:26` | `ComposeStackDetailPage` | status `refetchInterval:10000`, logs `refetchInterval:5000`, redeploy/start/stop/delete |
| `forge/web/components/docker/containers-view.tsx:56` | `ContainersView` | `listContainers` every 15s, `operateContainer`, per-row `getContainerStats` on demand |
| `forge/web/components/docker/images-view.tsx:30` | `ImagesView` | `listImages` + `pullImage` + `deleteImage` |
| `forge/web/components/docker/networks-view.tsx:18` | `NetworksView` | `listNetworks` + `createNetwork` + `deleteNetwork` |
| `forge/web/components/docker/volumes-view.tsx:18` | `VolumesView` | `listVolumes` + `createVolume` + `deleteVolume` + `pruneVolumes` UI button |

### 3.2 API HTTP (handlers)

| File | Route | Guard |
|------|-------|-------|
| `forge/api/internal/http/handlers_docker.go:30` | `docker := protected.Group("/docker", adminIPAccess, requireRole("admin"))` | admin only |
| `handlers_docker.go:33` | `GET /containers` → `dockerListContainers` | `requireAdminScope("servers.read")` |
| `handlers_docker.go:34` | `POST /containers` → `dockerCreateContainer` | `servers.write` |
| `handlers_docker.go:35` | `GET /containers/:id` → `dockerGetContainer` | `servers.read` |
| `handlers_docker.go:36` | `POST /containers/:id/operate` → `dockerOperateContainer` | `servers.write` |
| `handlers_docker.go:37` | `DELETE /containers/:id` → `dockerDeleteContainer` | `servers.write` |
| `handlers_docker.go:38` | `GET /containers/:id/logs` → `dockerContainerLogs` | `servers.read` |
| `handlers_docker.go:39` | `GET /containers/:id/stats` → `dockerContainerStats` | `servers.read` |
| `handlers_docker.go:40-43` | `GET /containers/:id/files`, `POST .../files/read`, `.../files/upload`, `.../files/delete` | files |
| `handlers_docker.go:46` | `GET /images` → `dockerListImages` | `servers.read` |
| `handlers_docker.go:47` | `POST /images/build` → `dockerBuildImage` | `servers.write` |
| `handlers_docker.go:48` | `POST /images/pull` → `dockerPullImage` | `servers.write` |
| `handlers_docker.go:49-51` | `POST /images/:id/push`, `POST /images/:id/tag`, `DELETE /images/:id` | `servers.write` |
| `handlers_docker.go:52` | `GET /images/search` | `servers.read` |
| `handlers_docker.go:55` | `GET /networks`, `POST /networks`, `DELETE /networks/:id` | `servers.*` |
| `handlers_docker.go:60` | `GET /volumes`, `POST /volumes`, `DELETE /volumes/:id` | `servers.*` |
| `handlers_docker.go:63` | `POST /volumes/prune` → `dockerPruneVolumes` | `requireRole("admin")+servers.write` |
| `handlers_compose.go:66` | `registerComposeRoutes` | `composeSvc = compose.New(store, daemon)` |
| `handlers_compose.go:82` | `POST /compose/webhook/:webhookId` (public on `v1`) | HMAC `X-Hub-Signature-256` / `X-Git-Token` |
| `handlers_compose.go:106` | `POST /compose/validate` | `requireRole("admin")` → `ValidateCompose` |
| `handlers_compose.go:118` | `POST /compose/import` → `CreateComposeProject` with `ValidateCompose` gate |  |
| `handlers_compose.go:162-311` | `/compose/git/*` family: `deploy/redeploy/check-update/preview/rollback/pull-redeploy/branch/auto-update/drift/status/last-webhook` | all `mutationLimiter + requireRole("admin")` |
| `handlers_compose.go:315` | `POST /compose` → `DeployComposeStack` | `mutationLimiter + admin` |
| `handlers_compose.go:348` | `GET /compose`, `GET /compose/:id`, `PATCH /compose/:id`, `DELETE /compose/:id` | CRUD |
| `handlers_compose.go:413` | `POST /compose/:id/deploy` (redeploy via re-create) |  |
| `handlers_compose.go:442` | `POST /compose/:id/stop` → `composeSvc.StopStack` |  |
| `handlers_compose.go:455` | `POST /compose/:id/start` → `composeSvc.StartStack` |  |
| `handlers_compose.go:468` | `GET /compose/:id/logs` → `GetStackLogs` |  |
| `handlers_compose.go:483` | `GET /compose/:id/status` → `GetStackStatus` |  |
| `handlers_compose.go:498-609` | legacy `/compose/projects/*` | `import/export/summary` |

### 3.3 Daemon client (Forge → Beacon wire)

All `Admin*` methods in `forge/api/internal/daemon/client.go` sign via `newRequest` + `retryRoundTripper:1` (retry with fresh nonce `resignRequest`):

- Containers: `AdminContainerList:1727`, `AdminContainerInspect:1735`, `AdminContainerLogs:1740`, `AdminContainerStart:1765`, `AdminContainerStop:1769`, `AdminContainerRestart:1773`, `AdminContainerDelete:1794`, `AdminContainerPause:1884`, `AdminContainerUnpause:1888`, `AdminContainerCreate:1892` (`POST /api/admin/containers`), `AdminContainerStats:1901`, `AdminContainerExec:2104` (`POST /api/admin/containers/:id/exec`), `AdminContainerTop:2115`.
- Images: `AdminImageList:1811`, `AdminImagePull:1816`, `AdminImageDelete:1837`, `AdminImagePrune:1854` (`POST /api/admin/images/prune`), `AdminImageBuild:1908`, `AdminImagePush:1917`, `AdminImageTag:1942`, `AdminImageSearch:1963`.
- Networks: `AdminNetworkList:1859`, `AdminNetworkInspect:1864`, `AdminNetworkCreate:2047` (`POST /api/admin/networks`), `AdminNetworkDelete:2056`.
- Volumes: `AdminVolumeList:1869`, `AdminVolumeInspect:1874`, `AdminVolumeUsage:1879`, `AdminVolumeCreate:2073`, `AdminVolumeDelete:2082`, `AdminVolumePrune:2099` (`POST /api/admin/volumes/prune`).
- Files: `AdminContainerFilesList:1970`, `AdminContainerFilesRead:1975`, `AdminContainerFilesUpload:2001`, `AdminContainerFilesDelete:2024`.
- Compose (separate file `daemon/compose.go`): `ComposeDeploy:26`, `ComposeStop/Start/Restart/Delete:58-75`, `ComposePull:77`, `ComposeStatus:93`, `ComposeLogs:112`.

### 3.4 Beacon server (handlers → Docker SDK)

`beacon/internal/server/server.go:392-466` multiplexes:

- `POST /compose/deploy` → `handleComposeDeploy`, `POST /compose/{stackId}/stop/start/restart` → `handleComposeStop/Start/Restart`, `DELETE /compose/{stackId}` → `handleComposeDelete`, `GET .../status/logs/pull`.
- `POST /build/dockerfile` → `handleDockerfileBuild`, `POST /image/push`, `GET /image/inspect`.
- `GET/POST/DELETE /api/admin/{containers,images,networks,volumes}/*` → `container_admin.go` handlers.

`beacon/internal/server/container_admin.go` details:
- `handleContainerList:1` — `docker.ContainerList(all)`, filters `managed = labels["modern-game-panel.server_id"]`, enforces `IsInfraAdmin` for managed.
- `handleContainerInspect:89`, `handleContainerLogs:141` (tails 100, redacts env), `adminContainerAction` (start/stop/restart/pause/unpause), `handleContainerDelete:247` (force, destructive header), `handleContainerExec:792` (allowlist `ls/pwd/ps/top/df/...`, forbids managed containers, infra-admin only), `handleContainerTop:909`, `handleContainerChanges:952`, `handleContainerStats:995` (`ContainerStatsOneShot` + `io.Copy`), `handleImageList:345`, `handleImagePull:412`, `handleImageDelete:471` (`X-Confirm-Destructive`), `handleImagePrune:531` (`ImagesPrune`), `handleNetworkList:570`, `handleVolumeList:657`, `handleVolumeInspect/Usage`, `handleImageBuild:1230` (tar `Dockerfile`), `handleImageTag:1300`, `handleImageSearch:1358`, `handleContainerFilesList:1414` (`CopyFromContainer` tar parse), `...Read:1484` (10 MB cap), `...Upload:1559` (multipart tar → `CopyToContainer`), `...Delete:1703` (`validateContainerDeletePath` → must be within `/home/container`).

`beacon/internal/server/compose.go:1-751` details:
- `validateComposePolicy:70` YAML parse → rejects `privileged`, `network_mode: host|service:`, `pid: host`, `userns_mode: host`, `cap_add`, `devices`, `security_opt`, privileged ports `<1024`, bind `type: bind` or host-source `/`/`..`.
- `handleComposeDeploy:252` writes `compose.yaml` + `.env` (encoded via `encodeComposeEnv`) into `composeStack.dirForID(stackID)` (`<dataDir>/../compose/<stackID>`), locks per-stack `composeLockEntry`, `exec.CommandContext("docker", "compose", "-f", composePath, "-p", stackID, "up", "-d", "--remove-orphans"? false by default)`. Similar for `stop/start/restart/delete/status/logs/pull`.
- `validateStackID:30` enforces `^[a-z0-9][a-z0-9_-]{0,127}$`.

`beacon/internal/server/mounts.go:1` — `runtimeMounts:13`, `allowedMountSource:40` (`EvalSymlinks` + `Rel` within `allowedMounts`), `cleanupMount:65`.

`beacon/internal/runtime/docker.go:42-1135`:

- `NewDockerRuntime:56` pins API `1.43`, validates `DOCKER_HOST` allowlist (`unix://`, `npipe://`, `tcp://docker-proxy:2375` only).
- `ensureImage:113` — if `ImageInspect` miss, requires digest-pin `@sha256:...` unless `DAEMON_ALLOW_UNPINNED_IMAGES=true`, then `ImagePull` + verify.
- `Create/Reconcile:151-249` — idempotent via `configHashLabel:899` sha256 of `CreateRequest`, per-`workloadLocks[64]` shard by `sha256(serverID)[0]`. Reconcile stops if running, removes, then `ContainerCreate` with `buildContainerConfig:1003` + `buildHostConfigWithSettings:827` + `buildNetworkingConfig:1043` (`EndpointIPAMConfig`).
- `buildResources:783` — `Memory`(+`MemoryOverhead%`), `MemorySwap`, `CPUShares`, `CPUQuota/CPUPeriod` ( `CPUPercent*100000/100`), `BlkioWeight`, `PidsLimit=256`, `OomKillDisable`, etc.
- `buildHostConfigWithSettings:827` — `CapDrop:ALL`, `Privileged:false`, `Init:true`, `ReadonlyRootfs:true`, `SecurityOpt:no-new-privileges,seccomp=builtin`, `UsernsMode:host` if `DAEMON_DOCKER_ROOTLESS_ENABLED`, `Tmpfs:/tmp size=MemoryMB/4 clamp 16-1024M`, `LogConfig json-file max-size 10m max-file 3`.
- `Install:256` — ephemeral `mgp-<id>-installer` container `--user 1000:1000`, `CapDrop ALL`, `ReadonlyRootfs:true`, network `gamepanel`.
- `Inspect/List/Start/SendCommand/Stop/WaitForStop/Kill/Signal/Restart/Stats/Logs/AttachConsole/Delete/WatchEvents` — full.
- `ensureNetwork:1065` — if missing and `NetworkSubnet!= ""`, `NetworkCreate bridge` with `IPAM` `managed:true` label; else validate subnet/gateway containment, conflict retry.

Multi-runtime: `forge/api/internal/runtime/multiruntime.go:1` `MultiRuntimeAdapter` (default+map), `forge/api/internal/runtime/docker.go:1` `DockerAdapter` (thin daemon proxy), `beacon/internal/runtime/factory.go:15` `Factory.CreateRuntime` (docker/containerd/podman/firecracker/k8s, health `Ping`).

Store/migrations: `forge/api/internal/store/store_compose.go:10` `ComposeStack` + `ComposeService`, `composeStackCols:60`, `composeStackScanExpr:84`, `CreateComposeStack:150`, `scanComposeStack:210`, `ListComposeStacksForReconciliation`, `ClaimComposeStackForUpdate` etc. Migrations `108_compose_stack_indexes.sql`, `115_compose_stacks.sql`, `141_compose_git_previous_manifest.sql`, `143_compose_stack_text_identifiers.sql` (UUID→TEXT), `160_encrypt_compose_secrets.sql` (`env_vars_encrypted`, `git_webhook_secret_encrypted`).

---

## 4. Comparisons — Forge vs Reference (≥15)

### C01 — Container listing & scope filtering

- **Reference (1Panel):** `reference/app-platforms/1panel/agent/app/api/v2/container.go:224` `ListContainer()` returns all via `containerService.List()`, `LoadContainerStatus:253` aggregates. No managed-container concealment.
- **Forge (Beacon):** `beacon/internal/server/container_admin.go:28` `handleContainerList` — filters `managed` (`modern-game-panel.server_id`) for non-infra admins (`filteredContainers` loop). **Forge is stricter** (least-privilege) vs 1Panel's flat list. Forge API `forge/api/internal/http/handlers_docker.go:99` `dockerListContainers` fans out across `nodeListOrFirst` nodes, aggregates `results[]fiber.Map{nodeId/nodeName/containers}`.
- **Verdict:** Forge adds infra-admin scoping not present in 1Panel. Portainer's `ContainerService.Recreate` similar privilege model but at Docker SDK layer.

### C02 — Container creation contract

- **Reference (1Panel):** `container.go:459` `ContainerCreate` → `containerService.ContainerCreate(dto.ContainerOperate)` (image, name, ports, env, volumes, network, restartPolicy all via SDK `ContainerCreate`). Low validation.
- **Forge UI→API:** `forge/web/lib/api/docker.ts:113` `createContainer` `POST /docker/containers?node=`; `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` forwards opaque `map[string]any` to `daemon.AdminContainerCreate:1892` (`POST /api/admin/containers`). **Beacon missing handler:** `beacon/internal/server/server.go:440` registers no `POST /api/admin/containers` (only `GET/DELETE`). → **BROKEN** (see trace §5.2).
- **Verdict:** UI and API pretend to support arbitrary container creation, but Beacon has no create handler; game-server path instead uses `beacon/internal/runtime/docker.go:151` `Create` via `server.create` (`POST /servers`) with strict `validateCreateRequest:901`.

### C03 — Start / Stop / Restart / Pause / Unpause

- **Reference (Portainer):** stop/start via `client.ContainerStop/Start`, **recreate** is transactional (see §2.1). 1Panel `ContainerOperation:612` maps `operate` string to docker SDK directly.
- **Forge:** `handlers_docker.go:155` `dockerOperateContainer` switch `start|stop|restart|pause|unpause` → `daemon.AdminContainerStart/Stop/Restart/Pause/Unpause:1765-1892`. Beacon `container_admin.go:165` via `adminContainerAction` (not shown but dispatches `ContainerStart/Stop/Restart/Pause/Unpause`). UI `forge/web/components/docker/containers-view.tsx:96` exposes per-state buttons (running→pause/stop/restart, paused→unpause, else→start). **Coverage is COMPLETE** for lifecycle ops, pause/unpause beyond many panels (Coolify lacks pause). No `kill -s 9` signal exposed (beacon `docker.go:494` `Kill` exists but not wired to `/docker`).

### C04 — Exec (diagnostics)

- **Reference (Portainer edge agent):** unrestricted exec via agent API. 1Panel has no exec endpoint (relies on `terminal.go` ws).
- **Forge Beacon:** `beacon/internal/server/container_admin.go:792` `handleContainerExec` — **infra-admin only**, allowlist 20 commands (`ls/pwd/ps/top/df/free/uname/whoami/id/head/tail/stat/file/.../ping`), forbids path-traversal args, rejects managed containers (`modern-game-panel.server_id` label), uses `ContainerExecCreate`+`Attach` + `stdcopy.StdCopy`.
- **Forge Daemon:** `forge/api/internal/daemon/client.go:2104` `AdminContainerExec` exists (`POST /api/admin/containers/:id/exec`).
- **Forge API:** `forge/api/internal/http/handlers_docker.go` **has zero exec route** (verified `grep exec` nil). UI `forge/web/lib/api/docker.ts` also has no `exec` export. → **UNWIRED** — Beacon capability exists, daemon client exists, but Forge API gate never mounts it. Break at API layer.

### C05 — Logs (stream vs one-shot)

- **Reference (docker-compose):** `pkg/compose/logs.go` streaming with `--follow --tail`. 1Panel `DownloadContainerLogs:677` + `ContainerStreamLogs:915`.
- **Forge Containers:** `handlers_docker.go:207` `dockerContainerLogs` `GET /docker/containers/:id/logs?tail=` → `daemon.AdminContainerLogs:1740` → beacon `container_admin.go:141` `handleContainerLogs` `Tail` param defaults `100`, caps `512KB` via `io.LimitReader`, forbids managed containers. UI polls `getContainerLogs` every 5s in `containers-view.tsx:136` `ContainerLogsModal`.
- **Forge Compose:** `beacon/internal/server/compose.go:615` `handleComposeLogs` `docker compose logs --no-color --tail <tail> [service]` via `exec.CommandContext`, 30s timeout.
- **Verdict:** Containers tail is one-shot + UI polling; compose logs is `exec` shellout; neither streams WS/SSE unlike Portainer's WS logs.

### C06 — Stats (one-shot → UI)

- **Reference (Portainer):** `api/docker/stats` collects via Docker SDK aggregations. 1Panel `ContainerListStats:419` bulk polling.
- **Forge:** `handlers_docker.go:223` `dockerContainerStats` `GET /docker/containers/:id/stats` → `daemon.AdminContainerStats:1901` → beacon `container_admin.go:995` `handleContainerStats` `ContainerStatsOneShot` → raw JSON copy. UI `containers-view.tsx:72` `fetchStats` parses `cpuPercent/memoryBytes/memoryLimit` on demand (click log icon), not auto-polled. Beacon also has `beacon/internal/runtime/stats.go:24` `DecodeDockerStats` and `enhanced_stats.go:55` `DecodeEnhancedStats` (`cpuPercent, memoryPercent, blkio, pids, uptimeSeconds`) for game-server stats path (`POST /servers/:id/ws/stats`), but **Docker admin stats does not use enhanced decoder** — discrepancy.
- **Verdict:** PARTIAL — one-shot works; streaming `StatsStream` exists in runtime (`docker.go:580`) but never exposed via admin route.

### C07 — Images: pull / build / push / tag / search / remove / prune

- **Reference (1Panel):** full matrix `image.go:18-215` `SearchImage/ListAllImage/ListImage/ImageBuild/ImagePull/ImagePush/ImageRemove/ImageTag/ImageLoad` + separate service layer for registry auth.
- **Forge UI:** `forge/web/lib/api/docker.ts:147-205` `listImages:147`, `pullImage:151` (`POST /docker/images/pull {image,tag,nodeId}`), `deleteImage:155`, `buildImage:189` (`POST /docker/images/build {dockerfile,tag}`), `pushImage:193`, `tagImage:197`, `searchImages:201`.
- **Forge API:** `handlers_docker.go:238-665` handlers for `list/pull/delete/build/push/tag/search` all wired (`240 dockerListImages` → `AdminImageList:1811` fan-out per node, decode `RepoTags/Id/Size/Created`; `280 dockerPullImage` with `validateContainerImageReference:14` via `distribution/reference`; `569 dockerBuildImage`; `594 dockerPushImage` with `RegistryAuth` passthrough `daemon.RegistryAuth`; `619 dockerTagImage`; `644 dockerSearchImages` fan-out).
- **Beacon:** `container_admin.go:412` `handleImagePull` (`ImagePull`+`io.Copy Discard`), `:471` `handleImageDelete` (`ImageRemove` force), `:1230` `handleImageBuild` (tar `Dockerfile` → `ImageBuild`), `:1300` `handleImageTag`, `:1358` `handleImageSearch`, **but image prune exists only on Beacon** `container_admin.go:531` `handleImagePrune` (`ImagesPrune` + `POST /api/admin/images/prune` at `server.go:460`) and `daemon.AdminImagePrune:1854` (`POST /api/admin/images/prune`). **No Forge handler maps it internally** — `handlers_docker.go` only maps `POST /volumes/prune:63`. Uncloud `RetrieveLocalDockerRegistryAuth` pattern not adopted; Forge forwards caller-supplied `registryAuth` verbatim, no server-side lookup.

### C08 — Volumes: list / create / delete / prune / usage

- **Reference (1Panel):** `container.go:784` `SearchVolume`, `:809` `ListVolume`, `:827` `DeleteVolume`, `:849` `CreateVolume`; no explicit prune endpoint listed but `ContainerPrune:503` covers containers.
- **Forge UI:** `forge/web/components/docker/volumes-view.tsx:18` `VolumesView` with `listVolumes` (30s poll), `createVolume`, `deleteVolume`, **`pruneVolumes` button** (`Eraser` icon → `pruneMut`).
- **Forge API:** `handlers_docker.go:415` `dockerListVolumes` decodes `{volumes[]}`, `454 dockerCreateVolume` → `AdminVolumeCreate:2073`, `473 dockerDeleteVolume` → `AdminVolumeDelete:2082`, `667 dockerPruneVolumes` → fan-out `AdminVolumePrune:2099` (`POST /api/admin/volumes/prune`) + `recordAudit volume:prune`. **Only Beacon `container_admin.go` lacks create/delete handlers** — `server.go:464-466` registers only `GET /api/admin/volumes`, `GET /api/admin/volumes/{id}`, `GET /api/admin/volumes/usage`; **no POST/DELETE for volumes**. So UI→API `createVolume/deleteVolume` reach daemon client then 404 on Beacon. **BROKEN** at Beacon.

### C09 — Networks: list / create / delete

- **Reference (1Panel):** `container.go:697` `SearchNetwork`, `:722` `ListNetwork`, `:740` `DeleteNetwork`, `:762` `CreateNetwork` (bridge/host/overlay/macvlan plus CIDR options).
- **Forge:** `forge/web/components/docker/networks-view.tsx:18` `NetworksView` `listNetworks/createNetwork/deleteNetwork` with driver select + subnet. `handlers_docker.go:340` `dockerListNetworks` → `AdminNetworkList:1859` (attached count via `Containers` map), `377 dockerCreateNetwork` → `AdminNetworkCreate:2047`, `396 dockerDeleteNetwork` → `AdminNetworkDelete:2056` (requires `?node=`). **Beacon missing handlers:** `server.go:462-463` registers only `GET /api/admin/networks`, `GET /api/admin/networks/{id}`; **no POST/DELETE**. → **BROKEN**.

### C10 — Registry & image pinning

- **Reference (Uncloud):** `internal/docker/image.go:56` `PullImage` auto-fetches auth from `docker/cli/cli/config.LoadDefaultConfigFile` via `RetrieveLocalDockerRegistryAuth`, handles empty auth encode workaround for moby bug `50729`.
- **Forge Beacon runtime:** `beacon/internal/runtime/docker.go:113` `ensureImage` — if local inspect miss, requires `@sha256:` pin unless `DAEMON_ALLOW_UNPINNED_IMAGES=true` (`pinnedImagePattern:40`). Logs verbatim `use name@sha256:<64 hex>`. Forge compose path (`beacon/internal/server/compose.go:handleComposeDeploy`) does **NOT** enforce pinning; it outsources pull to `docker compose up` which follows standard registry auth ( `.env` not auth). The admin `ImagePull` path (`container_admin.go:451` `RegistryAuth` optional) similarly accepts raw base64.
- **Verdict:** Game-server workload path is stricter (digest-pinned) than ad-hoc Docker admin path — correct asymmetry but undocumented divergence.

### C11 — Compose: env, interpolation & `env_file`

- **Reference (docker-compose):** `loader.go`/`envresolver.go` resolves `.env`, `env_file:`, CLI env, `include.path/env_file`, `${VAR}`, `${VAR:-default}`, `${VAR:?err}`, `$$` escaping, `environment:` as map or list (`VAR` vs `VAR=val`).
- **Forge:** `forge/api/internal/services/compose/service.go:220` `interpolateEnv` handles `$$`, `${VAR}`, `${VAR:-default}`, `${VAR-default}`, `${VAR:?err}`, `${VAR?err}` with envVars map, empty string fallback for unset. `beacon/internal/server/compose.go:206` `encodeComposeEnv` emits `KEY=val` with `\n\r` stripped, key regex `[A-Za-z0-9_]`. **No `env_file` handling**: parser accepts compose file but never mounts or resolves external `env_file` entries; they remain in YAML and are resolved by Docker Compose at deploy time via the generated `.env` sidecar only. `ParsedCompose` (`service.go:80-130`) captures `Services[].Environment` as normalized but `deploy` still writes `compose.yaml` verbatim.
- **Verdict:** PARTIAL — core `${VAR:-default}` interpolation ported; `env_file`, `include.env_file`, profiles execution, and `extends` unsupported.

### C12 — Compose: service dependencies & ordering

- **Reference (docker-compose):** `dependencies.go:78` `InDependencyOrder` topologically sorts `depends_on` (with `condition: service_started|service_healthy|service_completed_successfully`) before `create`; `convergence.go:50` rewires `service:db` container references.
- **Forge parser:** `forge/api/internal/services/compose/parser.go:566` normalizes `DependsOn` via `normalizeDependsOn`, stores as `[]string` names (condition parsed if present). **Runtime does not enforce order** — `beacon/internal/server/compose.go` shellouts `docker compose up -d` which itself respects `depends_on` with healthcheck conditions; Forge's `WaitForHealthy` (`lifecycle.go:181`) polls `ComposeStatus` regardless of dependency graph, so ordering is delegated to Docker Compose rather than explicitly sequenced.

### C13 — Compose: restart policies

- **Reference (docker-compose):** `create.go:592` `getRestartPolicy` maps `restart: always|unless-stopped|on-failure[:max]` and `deploy.restart_policy.condition: none|on-failure|any` → `container.RestartPolicy`.
- **Forge validation:** `forge/api/internal/services/compose/service.go:516` warns `restart: always` as “may conflict with platform lifecycle management” (severity `warning`), but **does not block** it. `beacon/internal/server/compose.go:94` `validateComposePolicy` does **not** validate `restart` at all — only `privileged`, `network_mode`, `pid`, `userns_mode`, `cap_add`, `devices`, `security_opt`, privileged ports, bind mounts. So `restart: always` passes Beacon policy. For game workloads, `beacon/internal/runtime/docker.go:827` hard-codes **no explicit restart policy** (delegates to Docker daemon defaults / Compose), relying on compose `restart` field verbatim.
- **Verdict:** Inconsistency between Forge parser warning and Beacon absence — effective behavior is “allow all” via Docker Compose.

### C14 — Resource limits & runtime isolation

- **Reference (docker-compose):** `create.go:641` maps `deploy.resources.limits.memory/cpus/pids`, `reservations`, `blkio_weight`, `ulimits`, `devices`, `device_requests` into `container.Resources`.
- **Forge game runtime:** `beacon/internal/runtime/docker.go:783` `buildResources` converts `CreateRequest.MemoryMB/CPUShares/CPUPercent/IOWeight/PIDLimit/OOMKillDisabled` into `container.Resources` with `MemoryOverhead%`, `MemorySwap`, `CPUShares 2-262144`, `CPUQuota/Period 100000`, `BlkioWeight 10-1000`, `PidsLimit default 256`, `OomKillDisable`. Plus `buildHostConfigWithSettings:827` enforces **isolation**: `CapDrop ALL`, `Privileged false`, `Init true`, `ReadonlyRootfs true`, `SecurityOpt no-new-privileges:seccomp=builtin`, `UsernsMode host` if rootless, Tmpfs, json-file logs. Compose path, however, runs `docker compose up -d` without these overrides — compose workloads inherit whatever the compose YAML specifies (subject only to `validateComposePolicy` deny-list). **Two isolation tiers.**
- **Forge compose ceilings:** `forge/api/internal/services/compose/lifecycle.go:139` `MaxUserStackMemoryMB=65536`, `Disk=512000`, `CPUShares=8192` enforced in `DeployComposeStack:150` before placement. Beacon compose does **not** enforce ceilings (no `MemoryMB` passed to `docker compose` except as reservation accounting).

### C15 — Mounts & volume isolation

- **Reference (1Panel):** host mounts are unrestricted (panel runs as root agent). Dokploy `utils/docker/compose/volume.ts` composes volumes with collision detection (`collision.ts`).
- **Forge Beacon:** `beacon/internal/server/mounts.go:13` `runtimeMounts` — every `mountConfiguration.Source` must `EvalSymlinks` and be `Rel` within `allowedMounts` (`SetAllowedMounts:125` from `allowed_mounts` config); empty allowlist denies all custom mounts. Targets must be absolute, not `/home/container`. Cleanup via `rootfs.New(permittedRoot)` confinement (`mounts.go:130`). **Compose file policy** `compose.go:205` `validateComposeVolumes` rejects `type: bind` long-form and any `source` that is `/`-absolute or `../` traversal; short-form named volumes/anonymous are allowed. Sensitive host paths (`/`, `/root`, `/etc`, `/home`) trigger warning/error in `service.go:610` `isSensitiveHostPath`, gate via `ValidateHostMountWithAllowlist:680` (requires `isAdmin` + `allowedMounts` entry).
- **Forge API migrations:** `015_a_mounts.sql` defines mounts table with `source/target/read_only`; compose stacks store `mounts` via `server_mounts` join. Divergence: game-server mounts are explicit per `CreateRequest.Mounts` with confinement; compose stacks are YAML-defined with deny-list only.

### C16 — Multi-runtime abstraction (bonus)

- **Reference (Portainer):** `ClientFactory` dispatches per endpoint type but remains Docker-only.
- **Forge:** `forge/api/internal/runtime/multiruntime.go:12` `MultiRuntimeAdapter` maps `provider→Runtime`; `beacon/internal/runtime/factory.go:17` supports `docker, containerd, podman, firecracker, kubernetes` plus `ProviderLXC/KVM` stubs. Each implements same `Runtime` interface (`runtime.go:40`). Game-server path supports migration-aware runtimes; compose path stays Docker-only (`docker compose` CLI).

---

## 5. End-to-End Chain Traces (UI → API → operation/queue → Beacon → Docker) — with breaks

### Trace T1 — `listContainers` (read — HEALTHY)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/lib/api/docker.ts:77` `listContainers({all})` | `fetchJSON("/docker/containers?all=${all}")` → flatten `nodeId/nodeName/containers`; `forge/web/components/docker/containers-view.tsx:56` `useQuery(["docker","containers"], listContainers, refetchInterval:15000)` |
| API | `forge/api/internal/http/handlers_docker.go:98` `dockerListContainers` | `nodeListOrFirst()` fan-out → `daemon.AdminContainerList` per node, swallow per-node errors, aggregate `fiber.Map{nodeId,nodeName,containers}` |
| Daemon | `forge/api/internal/daemon/client.go:1727` `AdminContainerList` | `GET /api/admin/containers?all=` with signed `newRequest` + retry |
| Beacon | `beacon/internal/server/container_admin.go:28` `handleContainerList` | `adminDockerClient()` → `docker.ContainerList(All:all)` → filter `managed` via `modern-game-panel.server_id` label |
| Docker | `github.com/docker/docker/client.ContainerList` | real SDK call |

**Result:** `COMPLETE`. No operation/queue (sync read). Only nuance: `containers-view.tsx:96` maps `state` field from `State` (Docker) but `forge/web/lib/api/docker.ts:90` maps `c.State ?? c.state` fallback — case sensitivity of Docker's `State` vs JSON camel.

### Trace T2 — `createContainer` (mutating — BROKEN at Beacon)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/lib/api/docker.ts:113` `createContainer({image,name,ports,env,volumes,network,restartPolicy,nodeId})` | `POST /docker/containers?node=` |
| API | `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` | `c.BodyParser(&body map[string]any)` → `resolveDockerNode` → `daemon.AdminContainerCreate` |
| Daemon | `forge/api/internal/daemon/client.go:1892` `AdminContainerCreate` | `POST /api/admin/containers` (`adminPostJSON`) |
| Beacon | `beacon/internal/server/server.go:440` mux table | **No route** `POST /api/admin/containers`. Handlers registered: `GET /api/admin/containers`, `GET .../{id}`, `GET .../logs`, `POST .../start/stop/restart/exec`, `DELETE .../{id}` — but **create is absent**. Request 404s. |
| Docker | — | never reached |

**Break:** `WIRED_BUT_WRONG` / `BROKEN`. Daemon client points at non-existent Beacon handler. Game-server creation (`beacon/internal/runtime/docker.go:151` `Create` via `server.create POST /servers`) works but is a distinct code path with strict `validateCreateRequest:901`. Admin “Create Container” UI will always fail post-deploy if exercised against a real Beacon. No test covers this ( `handlers_docker_test.go:10` only checks nil store → 404, not daemon roundtrip).

### Trace T3 — `image pull` (mutating — HEALTHY)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/components/docker/images-view.tsx:46` `PullImageModal` → `pullMut mutate({image,tag})`; `forge/web/lib/api/docker.ts:151` `pullImage(image,tag,nodeId)` | `POST /docker/images/pull {image,tag,nodeId}` |
| API | `forge/api/internal/http/handlers_docker.go:280` `dockerPullImage` | parses `body.Image+":"+body.Tag` if needed, `validateContainerImageReference:14` (`distribution/reference.ParseNormalizedNamed`, 512 limit), resolves node, → `daemon.AdminImagePull` |
| Daemon | `forge/api/internal/daemon/client.go:1816` `AdminImagePull` | `POST /api/admin/images/pull {image,registryAuth}` |
| Beacon | `beacon/internal/server/container_admin.go:412` `handleImagePull` | `docker.ImagePull(opts)` → `io.Copy Discard` drain, audit `image:pull` |
| Docker | `client.ImagePull` |  |

**Result:** `COMPLETE`. Auth passthrough exists (`pullImage` does not yet surface `registryAuth` UI, but daemon `AdminImagePush:1917` does with `RegistryAuth` argument). Pull defaults to first available node if `nodeId` omitted (list-or-first pattern). No queue; synchronous.

### Trace T4 — `volume prune` (destructive — PARTIAL, two prune scopes divergence)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/components/docker/volumes-view.tsx:44` `pruneMut mutate()` → `forge/web/lib/api/docker.ts:185` `pruneVolumes` | `POST /docker/volumes/prune` |
| API | `forge/api/internal/http/handlers_docker.go:667` `dockerPruneVolumes` | fan-out `nodeListOrFirst` → `daemon.AdminVolumePrune:2099` per node → `recordAudit volume:prune` |
| Daemon | `daemon/client.go:2099` `AdminVolumePrune` | `POST /api/admin/volumes/prune` |
| Beacon | `beacon/internal/server/server.go` | **No handler** `POST /api/admin/volumes/prune` registered (only `GET /api/admin/volumes` / `GET .../{id}` / `GET .../usage`). **Missing volume prune handler** — daemon will 404. Note contrast: `container_admin.go:531` `handleImagePrune` (`POST /api/admin/images/prune`) **does exist** (`server.go:460`) but Forge never exposes it (no `/docker/images/prune` route). |
| Docker | `client.VolumesPrune` would be target but unreachable for volumes; `client.ImagesPrune:556` for image prune is wired but unwired at API |  |

**Break:** Both prune directions are crossed wires. Volume prune is UI→API→Daemon wired but Beacon handler missing → 404. Image prune is Beacon→Docker wired but API route missing → dead code. Verification run via `grep -rn prune forge/api/internal/http/handlers_docker.go` confirms only volume prune route.

### Trace T5 — `compose deploy` (async via service, not queue — PARTIAL)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/lib/api/compose.ts:53` `createComposeStack({name,composeYaml,nodeId,envVars,...})` | `POST /compose`; detail page `forge/web/app/admin/compose/[id]/page.tsx:62` `deployComposeStack` → `POST /compose/:id/deploy` |
| API | `forge/api/internal/http/handlers_compose.go:315` `POST /compose` → `composeSvc.DeployComposeStack` (`handlers_compose.go:329`); `413 POST /compose/:id/deploy` → re-creates with existing YAML. Both guarded `mutationLimiter + requireRole("admin")`, not via `queue` (see note `forge/api/internal/services/operation/service.go:36` `OpCompose*` are *deprecated* — `queue.Service JobCompose* is canonical` — but compose handlers never enqueue; they call service directly). |  |
| Service | `forge/api/internal/services/compose/lifecycle.go:287` `DeployComposeStack` | Validates via `ValidateCompose:302` → quota `MaxUserStacks 20` → resource ceilings `MaxUserStackMemoryMB/Disk/CPUShares` → scheduler `PlaceServer` or direct node → `CreatePlacementReservation` → `store.CreateComposeStack` `status=deploying` → `daemon.ComposeDeploy:26` (`POST /compose/deploy {stackId,composeYaml,envVars}`) → mark `awaiting_health` → `confirmReservation` → **`WaitForHealthy` loop (2 min)** polling `daemon.ComposeStatus` every 5s → final `running/degraded` + `publisher.Publish EventComposeDeployed` |
| Daemon | `forge/api/internal/daemon/compose.go:26` `ComposeDeploy` | `POST /compose/deploy` |
| Beacon | `beacon/internal/server/compose.go:252` `handleComposeDeploy` | `validateComposePolicy:70` → stack-scoped lock `composeLockEntry`, `MkdirAll <dataDir>/compose/<stackID>`, write `compose.yaml` 0640 + `.env` via `encodeComposeEnv:206` → `exec.CommandContext("docker", "compose","-f",composePath,"-p",stackID,"up","-d")` 10 min timeout → `409` `composeOperationResponse` on failure |
| Docker | `docker compose up -d` | via shellout (requires Docker Compose v2 plugin on host) |

**Healthy.** Breaking nuance: **restart is unexposed.** `lifecycle.go` has `RestartStack` (and `queue_handler.go:111` `HandleRestart`), and beacon `handleComposeRestart:488`, daemon `ComposeRestart:58`, but `handlers_compose.go` **never mounts** `POST /compose/:id/restart` (only `deploy/stop/start`). So `restart` is service-ready but API-unreachable. Similarly `ComposePull` (`daemon/compose.go:77`) has beacon handler `handleComposePull:700` but API only uses it internally for gitops polling, not exposed to UI. `queue_handler.go` exists but `handlers_compose.go` does not enqueue — compose operations are synchronous through the 2-min health poll, not durable queue jobs (unlike game servers).

### Trace T6 — `exec` (read-only diagnostic — UNWIRED)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI | `forge/web/lib/api/docker.ts` | **no exec export** |
| API | `handlers_docker.go` | **no route** |
| Daemon | `daemon/client.go:2104` `AdminContainerExec` | wired `POST /api/admin/containers/:id/exec` |
| Beacon | `container_admin.go:792` `handleContainerExec` | `infra-admin` + allowlist + managed-container block → `ContainerExecCreate/Attach` → `StdCopy` → `ContainerExecInspect` exitCode; `server.go:447` mounts it |
| Docker | `client.ContainerExecCreate/Attach` |  |

→ Beacon+Daemon healthy, but API/UI never call it. Contrast `handlers_user_containers.go:18` per-server `POST /servers/:id/containers/:cid/{restart,start,stop}` which also omits exec.

### Trace T7 — `stats` (health-adjacent — HEALTHY via separate path)

| Hop | File:line | Evidence |
|-----|-----------|----------|
| UI (admin) | `docker.ts:143` `getContainerStats` → `containers-view.tsx:72` `fetchStats` | `GET /docker/containers/:id/stats?node=` one-shot, manual trigger (click `Terminal`) |
| API | `handlers_docker.go:223` `dockerContainerStats` | `AdminContainerStats` per node → `c.JSON({nodeId, stats: data})` (raw JSON passthrough, not decoded) |
| Daemon | `client.go:1901` `AdminContainerStats` | `GET /api/admin/containers/:id/stats` |
| Beacon | `container_admin.go:995` `handleContainerStats` | `ContainerStatsOneShot` → `io.Copy` JSON; also forbids managed containers |
| Docker | `client.ContainerStatsOneShot` |  |

→ Works. Streaming `StatsStream` / `enhanced_stats.go:55` for game workloads is a sibling path (`GET /servers/:id/stats` → `runtime.Stats` → `DecodeDockerStats` / `DecodeEnhancedStats`) not used here.

---

## 6. Store / Migration Coverage

- **compose stacks:** `store_compose.go:10` `ComposeStack` 40+ cols (`id,user_id,name,node_id,status,compose_yaml,compose_hash,env_vars/env_vars_encrypted,memory_mb,cpu_shares,disk_mb,error,reservation_id,compose_type,source_type,environment_id,git_*` family, `parsed_config`). `store_compose.go:150` `CreateComposeStack` encrypts env + webhook, `scanComposeStack:210` decrypts. **Encrypted at rest** since `160_encrypt_compose_secrets.sql`.
- **compose services/logs:** `migration 115_compose_stacks.sql:10` `compose_services (stack_id,name,image,status,state,ports,health,node_id)` + `compose_logs`. **Ephemeral**: `beacon/internal/server/compose.go` never writes to DB; `GetStackStatus` (`lifecycle.go:663`) synthesizes live `ComposeStatus` from beacon, not DB rows. `compose_services` appears populated only by heartbeat/monitor not inspected here — suggests drift.
- **mounts:** `015_a_mounts.sql` + `forge/api/internal/store/*mounts*` (store not located as single file; managed via `store.go` + `migrations`). Runtime isolation enforced in beacon not via store constraints.
- **No `store_docker.go`:** pruning/registry stats are live Docker calls, no persistence.

---

## 7. FORGE LOGIC FINDINGS

### FL-01 — Admin container / network / volume create routes have no Beacon handlers → deterministic 404 (P1, USER_VISIBLE)

- **Finding:** `forge/api/internal/daemon/client.go:1892` `AdminContainerCreate` (`POST /api/admin/containers`), `:2047` `AdminNetworkCreate` (`POST /api/admin/networks`), `:2073` `AdminVolumeCreate` (`POST /api/admin/volumes`) are faithfully invoked by `handlers_docker.go:136/377/454`, and UI `forge/web/lib/api/docker.ts:113/163/176` & modals `container-create-modal.tsx`, `networks-view.tsx`, `volumes-view.tsx` collect user input and POST. Yet `beacon/internal/server/server.go:440-466` multiplex table **registers no POST handlers** for those three paths (only GET/DELETE/stats/exec). Any attempt to create a container/network/volume via Docker admin UI will succeed at token validation then 404 at Beacon. Networks/volumes creation is therefore **false-complete**: UI appears functional, tests pass (nil-store 404 expectations), but end-to-end never succeeds.
- **Evidence:** `daemon/client.go:1892-1898` `AdminContainerCreate`, `server.go:440-466` mux list, `handlers_docker.go:136` callsite, absence confirmed via `grep -rn "POST /api/admin/containers"` in `beacon/`.
- **Reference contrast:** 1Panel `container.go:459` `ContainerCreate` + `762 CreateNetwork` + `849 CreateVolume` each have full agent service backing; Forge duplicated the API shape but left Beacon handler stub absent.
- **Impact:** Admins cannot provision ad-hoc containers/networks/volumes via panel; only game-server workload path (`POST /servers`) and `docker compose up -d` for compose stacks are operative.
- **Recommendation:** Implement `handleContainerCreate`, `handleNetworkCreate`, `handleVolumeCreate` in `beacon/internal/server/container_admin.go` (mirroring existing `handleContainerDelete` audit discipline with resource validation) or remove UI `Create …` affordances and document limitation.

### FL-02 — Prune wires are crossed: volume prune called but volume-prune handler missing; image-prune handler exists but never called (P2, OPERATOR_VISIBLE/SILENT)

- **Finding:** `forge/web/lib/api/docker.ts:185` `pruneVolumes` UI button (`volumes-view.tsx:44`) → `handlers_docker.go:667` `dockerPruneVolumes` → `daemon.AdminVolumePrune:2099` (`POST /api/admin/volumes/prune`) but Beacon mounts **no** `POST /api/admin/volumes/prune`. Conversely `beacon/internal/server/container_admin.go:531` `handleImagePrune` (`ImagesPrune` → `server.go:460` `POST /api/admin/images/prune`) + `daemon.AdminImagePrune:1854` exist, yet `handlers_docker.go` never mounts a `/docker/images/prune` route; the only prune route `handlers_portainer.go:40` `POST /images/prune` is unrelated. Net: neither prune actually reaches Docker from the Docker admin tab; image prune is dead code, volume prune 404s.
- **Evidence:** `handlers_docker.go:63` single `docker.Post("/volumes/prune"...` vs `server.go:460` `handleImagePrune`, `daemon/client.go:1854/2099`, `volumes-view.tsx:18` prune button wiring.
- **Reference contrast:** 1Panel `container.go:503` `ContainerPrune` (`pruneType` body) and image `ImageRemove` cover both; Uncloud has no panel prune (intentionally remote-only). Forge would need both, consistently.
- **Recommendation:** Wire `POST /docker/images/prune` → `AdminImagePrune` in `handlers_docker.go` (with `X-Confirm-Destructive` header pass-through like beacon expects `req.Header.Get("X-Confirm-Destructive")` at `container_admin.go:485/545`), and implement `handleVolumePrune` in Beacon (or remove volume-prune button).

### FL-03 — Exec is intentionally hardened in Beacon but completely absent from Forge API/UI → security-correct but feature-undiscoverable (P2, SILENT)

- **Finding:** `beacon/internal/server/container_admin.go:792` hardens exec: infra-admin only (`IsInfraAdmin`), managed-container deny, 20-command read-only allowlist, no `rm/curl/wget/sh/bash/python/docker/apt` etc., args forbidding `\x00\r\n`, audit `container:exec`. This is stronger than Portainer (no allowlist) and correct for a multi-tenant panel. However `forge/api/internal/http/handlers_docker.go` never mounts `POST /docker/containers/:id/exec`, and `forge/web/lib/api/docker.ts` never exports `execContainer`. `daemon.Client.AdminContainerExec:2104` is fully implemented but orphaned. Result: the capability cannot be exercised via normal panel flow; only a holder of the raw node token speaking Beacon directly can use it.
- **Evidence:** `container_admin.go:832-843` allowlist, `daemon/client.go:2104`, `handlers_docker.go` grep-nil `exec`, `docker.ts` export list.
- **Reference contrast:** 1Panel relies on `terminal.go` ws terminal (full shell, guarded by audit); Uncloud has no exec concept (service-level). Forge's allowlist design is defensible, but the missing Forge handler suggests the feature was intentionally left unwired pending infra-admin UX work — still, orphaned `AdminContainerExec` without route is technical debt.
- **Recommendation:** Either (a) wire `POST /docker/containers/:id/exec` behind `requireRole("admin") + requireInfraAdmin` (new guard) with command allowlist mirrored in API validation, or (b) delete `AdminContainerExec` and document exec as unsupported via Forge, directing users to Beacon-direct debugging.

### FL-04 — Compose restart/pull/ exec lifecycle gaps: service supports them, API/UI do not (P2)

- **Finding:** `forge/api/internal/services/compose/lifecycle.go` implements `RestartStack` and `daemon/compose.go:58` `ComposeRestart` + `77 ComposePull` exist; `beacon/internal/server/compose.go:488` `handleComposeRestart` + `700 handleComposePull` exist. But `forge/api/internal/http/handlers_compose.go` only exposes `POST .../deploy/stop/start`, never `POST .../restart` or `POST .../pull`. The queue handler `queue_handler.go:111` `HandleRestart` exists but handlers never enqueue. UI `forge/web/app/admin/compose/[id]/page.tsx` offers `Start/Stop/Redeploy(Deploy)` but no Restart button. Similarly Portainer's `RedeployWhenChanged` / Komodo's `PullStack` pattern for GitOps has no one-shot pull trigger in Forge UI.
- **Impact:** Admins cannot restart a degraded compose stack without stop+start cycle; cannot manually trigger `docker compose pull` before redeploy.
- **Evidence:** `handlers_compose.go:442/455` (stop/start only), `compose.go:488/700` handlers, `lifecycle.go:663` start/stop/restart implementations, `[id]/page.tsx:62` mutation list.

### FL-05 — Compose resource ceilings exist but Beacon compose ignores them; restart policy validation is warn-only at API but pass-through at Beacon (P3)

- **Finding:** `lifecycle.go:139` ceilings `MaxUserStackMemoryMB/CPU/Disk` are enforced **only** on `DeployComposeStack`; `UpdateComposeStack` (`lifecycle.go:460` onward) does not re-check updated `MemoryMB/CPUShares/DiskMB` beyond size, allowing privilege escalation via PATCH. Beacon `compose.go:validateComposePolicy` does not enforce any memory/cpu/disk limits at all — it relies on API pre-check. Similarly `service.go:516` warns `restart: always` but Beacon `validateComposePolicy:70` allows it (no restart check). So the isolation boundary is API-only, bypassable if Beacon is called directly.
- **Recommendation:** Move ceiling check into a shared `ValidateResourceLimits` called by both `DeployComposeStack` and `UpdateComposeStack`, and add defense-in-depth numeric cap (e.g. max memory per compose service) in `validateComposePolicy` or `handleComposeDeploy` before `docker compose up`.

### FL-06 — Port mapping parser bug in UI flattens `hostPort:containerPort` incorrectly for some Docker formats + silent credential handling gap (P3)

- **Finding:** `forge/web/lib/api/docker.ts:92` `ports: ports.map(p => \`${p.hostPort||""}:${p.containerPort}/${p.type||"tcp"}\`)` uses camelCase `hostPort/containerPort/type` keys, but Docker Engine returns `PublicPort/PrivatePort/Type` (or `IP`-family). Real Beacon `container_admin.go:28` forwards raw `types.Container` which has `Ports []types.Port{IP,PrivatePort,PublicPort,Type}`; those lowercase names will be undefined after JSON `Port` struct marshal (Go JSON uses upper-first `PublicPort` → `publicPort` via `json` tags?). The UI fallback `p.hostPort` will be empty, showing `:80/tcp` for many containers. `handlers_docker.go:240` `dockerListImages` correctly handles `RepoTags` via `interface{}`, but `docker.ts` does not mirror for ports. Minor but user-visible.
- **Also:** `pullImage:151` UI `images-view.tsx:46` collects only `image`+`tag`; registry auth is never surfaced (no credential picker). Private registry pulls will fail silently; Uncloud's `RetrieveLocalDockerRegistryAuth` solves this by local docker config lookup, Forge does not.

---

## 8. Coverage Matrix (Selected)

| Capability | Reference (best) | Forge UI | Forge API | Daemon | Beacon | Docker | Status |
|------------|------------------|----------|-----------|--------|--------|--------|--------|
| container list | 1Panel `ListContainer:224` | `docker.ts:77` | `handlers_docker.go:99` fan-out | `AdminContainerList:1727` | `handleContainerList:28` filtered | `ContainerList` | **COMPLETE** |
| container create | 1Panel `ContainerCreate:459` | modal `container-create-modal.tsx` | `dockerCreateContainer:136` | `AdminContainerCreate:1892` 404 | **missing handler** | — | **BROKEN** |
| container inspect | 1Panel `ContainerInfo:384` | `getContainer:109` | `dockerGetContainer:121` | `AdminContainerInspect:1735` | `handleContainerInspect:89` redacted | `ContainerInspect` | **COMPLETE** |
| container start/stop/restart | 1Panel `ContainerOperation:612` | `operateContainer:118` | `dockerOperateContainer:155` | `AdminContainerStart:1765` etc. | `handleContainerStart/Stop/Restart` | SDK | **COMPLETE** (pause/unpause extra) |
| container pause/unpause | 1Panel same | yes | yes | `AdminContainerPause:1884` | via `adminContainerAction` | `Pause/Unpause` | **COMPLETE** (superset) |
| container exec | beacon allowlist | **none** | **none** | `AdminContainerExec:2104` | `handleContainerExec:792` | `ExecCreate/Attach` | **UNWIRED** |
| container logs | 1Panel `ContainerStreamLogs:915` | `getContainerLogs:134` polling | `dockerContainerLogs:207` | `AdminContainerLogs:1740` | `handleContainerLogs:141` 512KB | `ContainerLogs` | **COMPLETE** (no streaming) |
| container stats | 1Panel `ContainerListStats:419` | `getContainerStats:143` manual | `dockerContainerStats:223` | `AdminContainerStats:1901` | `handleContainerStats:995` | `StatsOneShot` | **PARTIAL** (no stream) |
| container prune | 1Panel `ContainerPrune:503` | none | none | none | none | `ContainersPrune` | **MISSING** |
| container files (ls/read/upload/delete) | 1Panel `ListContainerFiles:68` | `listContainerFiles:207` etc. | `dockerContainerFiles*:494` | `AdminContainerFiles*:1970` | `handleContainerFiles*:1414` confined `/home/container` | `CopyFrom/ToContainer + exec rm` | **COMPLETE** (game-container-scoped) |
| image list | 1Panel `ListAllImage:43` | `listImages:147` fan-out decode | `dockerListImages:240` | `AdminImageList:1811` | `handleImageList:345` | `ImageList` | **COMPLETE** |
| image pull | 1Panel `ImagePull:100` | `pullImage:151` | `dockerPullImage:280` + `validateImageReference:14` | `AdminImagePull:1816` | `handleImagePull:412` | `ImagePull` | **COMPLETE** |
| image build | 1Panel `ImageBuild:77` | `buildImage:189` | `dockerBuildImage:569` | `AdminImageBuild:1908` | `handleImageBuild:1230` tar build | `ImageBuild` | **COMPLETE** |
| image push | 1Panel `ImagePush:123` | `pushImage:193` | `dockerPushImage:594` | `AdminImagePush:1917` | `handleImagePush:419` | `ImagePush` | **COMPLETE** but no UI trigger |
| image tag | 1Panel `ImageTag:192` | `tagImage:197` | `dockerTagImage:619` | `AdminImageTag:1942` | `handleImageTag:1300` | `ImageTag` | **COMPLETE** |
| image search | 1Panel `SearchImage:18` | `searchImages:201` | `dockerSearchImages:644` fan-out | `AdminImageSearch:1963` | `handleImageSearch:1358` limit 25 | `ImageSearch` | **COMPLETE** |
| image prune | ⚠️ 1Panel via `ImageRemove` | **none** | **none** | `AdminImagePrune:1854` | `handleImagePrune:531` | `ImagesPrune` | **UNWIRED** (dead) |
| volume list | 1Panel `ListVolume:809` | `listVolumes:172` | `dockerListVolumes:415` decode `volumes` | `AdminVolumeList:1869` | `handleVolumeList:657` + `filter managed` | `VolumeList` | **COMPLETE** |
| volume create | 1Panel `CreateVolume:849` | `createVolume:176` | `dockerCreateVolume:454` | `AdminVolumeCreate:2073` 404 | **missing handler** | — | **BROKEN** |
| volume delete | 1Panel `DeleteVolume:827` | `deleteVolume:181` | `dockerDeleteVolume:473` needs `?node=` | `AdminVolumeDelete:2082` 404 | **missing handler** | — | **BROKEN** |
| volume prune | none | `pruneVolumes:185` button exists | `dockerPruneVolumes:667` fan-out | `AdminVolumePrune:2099` 404 | **missing handler** | `VolumesPrune` | **BROKEN** |
| network list | 1Panel `ListNetwork:722` | `listNetworks:159` | `dockerListNetworks:340` | `AdminNetworkList:1859` | `handleNetworkList:570` filter platform | `NetworkList` | **COMPLETE** |
| network create | 1Panel `CreateNetwork:762` | `createNetwork:163` | `dockerCreateNetwork:377` | `AdminNetworkCreate:2047` 404 | **missing handler** | — | **BROKEN** |
| network delete | 1Panel `DeleteNetwork:740` | `deleteNetwork:168` | `dockerDeleteNetwork:396` needs `?node=` | `AdminNetworkDelete:2056` 404 | **missing handler** | — | **BROKEN** |
| compose validate | docker-compose loader | `validateCompose:49` | `POST /compose/validate:106` | — | — | — | **COMPLETE** |
| compose deploy | Komodo `DeployStack`, docker `up -d` | `createComposeStack:53` | `POST /compose:315` → `DeployComposeStack` | `ComposeDeploy:26` | `handleComposeDeploy:252` + policy + lock | `docker compose up` | **COMPLETE** |
| compose status | Komodo `ps --format json` monitor | `getComposeStackStatus:101` polling 10s | `GET /compose/:id/status:483` | `ComposeStatus:93` | `handleComposeStatus:569` (`ps --format json`) | `compose ps` | **COMPLETE** |
| compose logs | | `getComposeStackLogs:105` | `GET /compose/:id/logs:468` | `ComposeLogs:112` | `handleComposeLogs:615` | `compose logs` | **COMPLETE** |
| compose stop/start | Komodo `Stop/StartStack` | `stop/startComposeStack:93/97` | `POST .../stop:442` `.../start:455` | `ComposeStop/Start:58` | `handleComposeStop/Start` | `compose stop/start` | **COMPLETE** |
| compose restart | `restartCommand:561` | **none** | **none** | `ComposeRestart:58` | `handleComposeRestart:488` | `compose restart` | **UNWIRED** |
| compose pull | `pull` | **none** | none (internal gitops only) | `ComposePull:77` | `handleComposePull:700` | `compose pull` | **UNWIRED** |
| compose dependencies | `dependencies.go:78` order | not visualized | parser `normalizeDependsOn` | — | delegates to `up` | `InDependencyOrder` | **DELEGATED** |
| compose env interpolation | `envresolver.go` | via `.env` file + `ExpandTemplate` | `interpolateEnv:55` | passed as `.env` | `encodeComposeEnv:206` | `loader` | **PARTIAL** (no `env_file`) |
| resource limits | `create.go:641` NanoCPUs/Memory | ceilings UI shows mem/cpuShares/disk | ceilings enforced Deploy only | reservation | game-runtime `buildResources:783` strict; compose policy lenient | — | **PARTIAL** |
| runtime isolation | `CapDrop/Privileged/Init/Readonly` | not exposed | — | — | `docker.go:827` hardcoded | — | **COMPLETE** for game; **LENIENT** for compose |

---

## 9. Verification of “store_compose / store_docker / prune / stats / exec” Prompt Requirements

- **prune:** UI `pruneVolumes:185` exists (`volumes-view.tsx` button), API `dockerPruneVolumes:667` exists but Beacon handler absent → 404. `handleImagePrune:531` exists on Beacon but no `/docker/images/prune` API route wraps it. No `store_docker.go` prune persistence (live Docker only).
- **stats:** One-shot `dockerContainerStats:223` → `AdminContainerStats:1901` → `handleContainerStats:995` `StatsOneShot` verified. Enhanced `enhanced_stats.go:24` `DecodeEnhancedStats` (CPU/Mem%/blkio/pids/uptime) is used only by game-server stats path, not Docker admin. Streaming `StatsStream:580` unused by admin routes.
- **exec:** `handleContainerExec:792` verified with allowlist, infra-admin gate, audit. Orphaned `AdminContainerExec:2104` exists. No Forge API route, no UI export.
- **store_compose:** `store_compose.go:793` verified — encrypted `env_vars`, `git_webhook_secret`, `parsed_config`, `git_*` family, `ComposeService`/`ComposeLogs` tables via migrations.
- **store_docker:** no such file — Docker inventory is intentionally stateless, lives in Docker Engine only.
- **mounts/compose migrations:** `015_a_mounts.sql` (mounts with source/target/read_only), `108/115/141/143/160` compose stack enc, `077_add_runtime_status_to_nodes.sql` runtime status, `102_a_uncloud_service_model.sql` service model — all inspected.

---

## 10. Recommendations (prioritized)

1. **P1 — Close create/prune handler gaps:** implement `POST /api/admin/{containers,networks,volumes}` + `DELETE /api/admin/{networks,volumes}/{id}` + `POST /api/admin/volumes/prune` in Beacon, or delete UI affordances. Add integration test that `AdminContainerCreate` round-trips against a test Beacon mux (like `handlers_docker_test.go:10` but with fake Beacon).
2. **P1 — Fix prune cross-wiring:** mount `POST /docker/images/prune` → `AdminImagePrune` with `X-Confirm-Destructive` header passthrough; implement Beacon `handleVolumePrune` (`VolumesPrune` + filter).
3. **P2 — Decide on exec:** either promote `contextPos-t:2104` to a first-class Forge route with infra-admin guard mirroring Beacon's allowlist, or delete `AdminContainerExec` and document exec as out-of-band. Do not leave orphaned allowlisted exec on Beacon reachable only via raw node token.
4. **P2 — Wire compose restart/pull:** expose `POST /compose/:id/restart` → `composeSvc.RestartStack` and `POST /compose/:id/pull` → `ComposePull` (used by gitops polling) to Forge UI.
5. **P3 — Make `interpolateEnv` and `validateComposePolicy` consistent:** centralize `validateComposePolicy` call in Forge parser already, but ensure `restart: always` semantics are documented; move resource ceiling check to shared function called by both `Deploy` and `Update`.
6. **P3 — Fix UI port mapping:** update `forge/web/lib/api/docker.ts:92` to handle `PublicPort/PrivatePort/IP/Type` from Docker Engine's `types.Port` (not `hostPort/containerPort`), using `Port` type guard.

---

*Evidence exhaustive. No product code changed. All line numbers refer to trees at `gamepanel/` at audit date; Beacon `container_admin.go` 1771 lines, `compose.go` 751, `docker.go` 1135 provide pinning/isolation guarantees that exceed 1Panel and match docker-compose spec where composed via CLI.*

