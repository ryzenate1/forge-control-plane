# Subagent 08 — Container Management Re-verification (containers / images / networks / volumes / exec / logs / stats + prune / registry)

> Focus: Container Management parity vs Portainer / 1Panel / Docker upstream
> Reconciles: `audits/phase-01/subagent-03-runtime-compose.md` (18 rows, 8 traces) + `audits/final-parity/subagent-04-runtime-compose.md` re-verification (3 of 4 BROKEN chains claimed FIXED) + `audits/phase-06/subagent-07-komodo-stacks.md` + `audits/phase-06/subagent-04-1panel-infra-monitor.md`
> Auditor: subagent-08 (reverification, parallel 08/20)
> Date: 2026-08-24
> Method: source inspection (Read/Grep/Bash). All citations `file:line` verifiable. No product code modified. Traces are UI→API→daemon→Beacon→Docker with hop tables.

---

## 1. Reconciliation Summary

### 1.1 Phase-01 baseline (`phase-01/subagent-03-runtime-compose.md`)

18 comparisons (§4) + 7 traces (§5: T1-T7). Statuses: COMPLETE/PARTIAL/UNWIRED/BROKEN.

Key BROKEN at Phase-01:

| ID | Capability | Break location | Evidence (then) |
|---|---|---|---|
| FL-01 | `POST /api/admin/containers` create | Beacon no handler | `forge/api/internal/daemon/client.go:1892` `AdminContainerCreate` → `beacon/internal/server/server.go:440` mux table had only `GET/DELETE` |
| FL-01 | `POST /api/admin/networks` create | Beacon 404 | `client.go:2047` → `server.go:462-463` only `GET` |
| FL-01 | `POST /api/admin/volumes` create/delete | Beacon 404 | `client.go:2073` → `server.go:464-466` only `GET` |
| FL-02 | Prune crossed wires | Volume prune 404 at Beacon; image prune dead code at API | `handlers_docker.go:63` only volumes prune vs `container_admin.go:531` `handleImagePrune` + `server.go:460` |
| T4/T6 | `exec` UNWIRED at API | API+UI never mount exec | `handlers_docker.go` zero exec route, `docker.ts` no export, Beacon `792` + Daemon `2104` healthy but air-gapped |
| T2 | `pause`/`unpause` wired to Daemon but Beacon missing | Beacon mux only `start/stop/restart` | UI `containers-view.tsx:96` exposes pause, `client.go:1875` exists, Beacon no handler |

### 1.2 Final-parity re-verification (`final-parity/subagent-04-runtime-compose.md`)

Claims **FIXED since Phase-1** ( §1): `POST /api/admin/containers`, `POST /api/admin/networks`, `POST /api/admin/volumes`, `POST /api/admin/volumes/prune` now have Beacon handlers; `POST /compose/:id/restart` now mounted. Verdict: wiring debt largely resolved, spec-fidelity debt remains (env_file, shortFormHostPort, per-service resources, volume allowlist trifurcation, phantom providers, build.context).

### 1.3 This re-inspection — verdict

- **FIXED claims CONFIRMED** for 4 routes examined: `POST /api/admin/containers` (`beacon/internal/server/container_admin.go:1776` + `server.go:478`), `POST /api/admin/networks` (`1824` + `479`), `POST /api/admin/volumes` (`1896` + `481`), `POST /api/admin/volumes/prune` (`1961` + `483`). Daemon clients at `forge/api/internal/daemon/client.go:1883/2038/2064/2090` now resolve to real Beacon handlers (previously deterministic 404). Traces T2-T4 below prove HEALTHY.
- **Not FIXED / still BROKEN**: `pause`/`unpause` remains 404 (Daemon `1875/1879` → Beacon no `POST /api/admin/containers/{id}/pause`); `image prune` remains API-UNWIRED on Docker admin path (Beacon `531` + `server.go:476` ready, Daemon `1845` ready, but `handlers_docker.go` never exposes `POST /docker/images/prune` — only `handlers_portainer.go:557` does under `/portainer`); `shortFormHostPort` (`beacon/internal/server/compose.go:214`) still returns `""` for `len==1` (`"80"` privileged port bypass, confirmed via local python split test); per-replica resource ceiling (`compose/lifecycle.go:280`) still checks `req.MemoryMB` only, not `sum(service.limits * replicas)`; `exec` still correctly UNWIRED at API (intentional).
- **Lossy FIXED** (subtle regression from FIXED wiring): `handleContainerCreate` request struct narrows UI `CreateContainerRequest` (drops `ports/volumes/network/restartPolicy`); `handleNetworkCreate` drops UI `subnet` (no IPAMConfig). Wire is no longer 404 but silently discards fields — classified PARTIAL.

Counts this report: 18 parity rows (§4), 6 hop-traced chains (§5) with file:line per hop, 6 findings (§6) each broken chain has hop table.

---

## 2. Forge Chain Inventory (current, file:line)

### 2.1 UI — `forge/web`

| File | Symbol / Route | Notes |
|---|---|---|
| `forge/web/lib/api/docker.ts:77` | `listContainers({all})` | `GET /docker/containers?all=` → flatten `nodeId/nodeName/containers` (handles `State` vs `state` casing) |
| `forge/web/lib/api/docker.ts:109` | `getContainer(id)` | `GET /docker/containers/:id?node=` |
| `forge/web/lib/api/docker.ts:113` | `createContainer(cfg)` | `POST /docker/containers?node=` body `CreateContainerRequest{image,name,ports,env,volumes,network,restartPolicy}` |
| `forge/web/lib/api/docker.ts:118` | `operateContainer(id,action)` | `POST /docker/containers/:id/operate {action: start\|stop\|restart\|pause\|unpause}` |
| `forge/web/lib/api/docker.ts:126` | `deleteContainer(id,force,nodeId)` | `DELETE /docker/containers/:id?force&node=` |
| `forge/web/lib/api/docker.ts:134` | `getContainerLogs` | `GET /docker/containers/:id/logs?tail&node=` → `text/plain` |
| `forge/web/lib/api/docker.ts:143` | `getContainerStats` | `GET /docker/containers/:id/stats?node=` one-shot |
| `forge/web/lib/api/docker.ts:147` | `listImages` | `GET /docker/images` fan-out |
| `forge/web/lib/api/docker.ts:151` | `pullImage(image,tag,nodeId)` | `POST /docker/images/pull {image,tag,nodeId}` |
| `forge/web/lib/api/docker.ts:155` | `deleteImage` | `DELETE /docker/images/:id?node=` |
| `forge/web/lib/api/docker.ts:159` | `listNetworks/createNetwork/deleteNetwork` | networks CRUD |
| `forge/web/lib/api/docker.ts:172` | `listVolumes/createVolume/deleteVolume/pruneVolumes` | `POST /docker/volumes/prune` |
| `forge/web/lib/api/docker.ts:189` | `buildImage/pushImage/tagImage/searchImages` | `POST /docker/images/build {dockerfile,tag}` etc. |
| `forge/web/lib/api/docker.ts:207` | `listContainerFiles/readContainerFile/uploadContainerFile/deleteContainerFile` | `GET /docker/containers/:id/files?path=` + POST triad |
| `forge/web/app/admin/docker/page.tsx:14` | `DockerPage` tab container | `containers/images/networks/volumes` |
| `forge/web/components/docker/containers-view.tsx:56` | `ContainersView` | `useQuery ["docker","containers"]` 15s poll, per-row `fetchStats` on demand, pause/stop/restart/unpause/start buttons at `142-151` |
| `forge/web/components/docker/container-create-modal.tsx:14` | `ContainerCreateModal` | collects `CreateContainerRequest` ports/env/volumes/network/restartPolicy, maps to `createContainer` at `57` |
| `forge/web/components/docker/images-view.tsx:30` | `ImagesView` | pull modal at `141` + delete |
| `forge/web/components/docker/networks-view.tsx:18` | `NetworksView` | driver+subnet form, `createNetwork` |
| `forge/web/components/docker/volumes-view.tsx:18` | `VolumesView` | list 30s + create+delete+prune button at `44` |

### 2.2 API HTTP

| File:line | Route | Guard | Handler |
|---|---|---|---|
| `forge/api/internal/http/handlers_docker.go:30` | `protected.Group("/docker", adminIPAccess, requireRole("admin"))` | admin | — |
| `handlers_docker.go:33` | `GET /containers → dockerListContainers` | `servers.read` | fans out `nodeListOrFirst` at `98` |
| `handlers_docker.go:34` | `POST /containers → dockerCreateContainer` | `servers.write` + `mutationLimiter` | at `136` → `AdminContainerCreate` |
| `handlers_docker.go:36` | `POST /containers/:id/operate → dockerOperateContainer` | `servers.write` | switch `155` start/stop/restart/pause/unpause |
| `handlers_docker.go:37` | `DELETE /containers/:id → dockerDeleteContainer` | `servers.write` | `191` force+v (but API only forwards force, v false) |
| `handlers_docker.go:38-39` | `GET /containers/:id/logs\|stats` | `servers.read` | `207/223` |
| `handlers_docker.go:40-43` | `GET /containers/:id/files` + `POST .../files/{read,upload,delete}` | mixed | `493/510/531/548` |
| `handlers_docker.go:46` | `GET /images → dockerListImages` | `servers.read` | `240` RepoTags/Size/Created decode |
| `handlers_docker.go:47` | `POST /images/build → dockerBuildImage` | `servers.write` | `570` validates tag |
| `handlers_docker.go:48` | `POST /images/pull → dockerPullImage` | `servers.write` | `280` `validateContainerImageReference` |
| `handlers_docker.go:49-51` | `POST /images/:id/{push,tag} DELETE /images/:id` | `servers.write` | `595/619/319` |
| `handlers_docker.go:52` | `GET /images/search` | `servers.read` | `644` fan-out |
| `handlers_docker.go:55` | `GET /networks POST /networks DELETE /networks/:id` | `servers.*` | `340/377/396` |
| `handlers_docker.go:60` | `GET /volumes POST /volumes DELETE /volumes/:id` | `servers.*` | `415/454/473` |
| `handlers_docker.go:63` | `POST /volumes/prune → dockerPruneVolumes` | `requireRole("admin")+servers.write` | `667` fan-out `AdminVolumePrune:2090` |
| `handlers_docker.go:14` | `validateContainerImageReference` | — | 512 limit via `distribution/reference` |
| `forge/api/internal/http/handlers_portainer.go:40` | `POST /portainer/images/prune → pruneImages` | admin | `557` → `AdminImagePrune:1845` (Docker admin image prune lives here, not under `/docker`) |

Notable **absences**: no `POST /docker/containers/:id/exec`, no `POST /docker/images/prune`, no `POST /api/admin/containers/:id/pause` mount at Beacon (see below).

### 2.3 Daemon client — `forge/api/internal/daemon/client.go`

All `Admin*` sign via `newRequest` + `retryRoundTripper` with `resignRequest` fresh nonce. Current line numbers (shifted vs Phase-01 cited 1892/2047/2073 but semantically identical):

- Containers: `AdminContainerList:1718`, `AdminContainerInspect:1726`, `AdminContainerLogs:1731`, `AdminContainerStart:1756`, `AdminContainerStop:1760`, `AdminContainerRestart:1764`, `AdminContainerDelete:1785`, `AdminContainerPause:1875`, `AdminContainerUnpause:1879`, `AdminContainerCreate:1883` (`POST /api/admin/containers`), `AdminContainerStats:1892`, `AdminContainerExec:2095` (`POST /api/admin/containers/:id/exec`), `AdminContainerTop:2106`.
- Images: `AdminImageList:1802`, `AdminImagePull:1807`, `AdminImageDelete:1828`, `AdminImagePrune:1845` (`POST /api/admin/images/prune`), `AdminImageBuild:1899`, `AdminImagePush:1908`, `AdminImageTag:1933`, `AdminImageSearch:1954`.
- Networks: `AdminNetworkList:1850`, `AdminNetworkInspect:1855`, `AdminNetworkCreate:2038` (`POST /api/admin/networks`), `AdminNetworkDelete:2047`.
- Volumes: `AdminVolumeList:1860`, `AdminVolumeInspect:1865`, `AdminVolumeUsage:1870`, `AdminVolumeCreate:2064` (`POST /api/admin/volumes`), `AdminVolumeDelete:2073`, `AdminVolumePrune:2090` (`POST /api/admin/volumes/prune`).
- Files: `AdminContainerFilesList:1961`, `...Read:1966`, `...Upload:1992`, `...Delete:2015`.
- Compose: `daemon/compose.go:48` `ComposeDeploy`, `73-86` `ComposeStop/Start/Restart/Delete`, `89` `ComposePull`, `110` `ComposeStatus`, `131` `ComposeLogs`.

### 2.4 Beacon server → Docker SDK

`beacon/internal/server/server.go:408-489` multiplex (post-fix). Extract verified via bash (`grep -n POST /api/admin/...`):

- `POST /compose/deploy → handleComposeDeploy`, `POST /compose/{stackId}/{stop,start,restart} → handleComposeStop/Start/Restart:409-411`, `DELETE /compose/{stackId} → handleComposeDelete:412`, `GET .../status|logs|pull:413-415`.
- `POST /build/dockerfile → handleDockerfileBuild:429`, `POST /image/push` etc., `GET /image/inspect`.
- `GET/POST/DELETE /api/admin/* → container_admin.go` — FIXED set:
  - Containers: `GET /api/admin/containers:456 → handleContainerList`, `GET .../{id}:457`, `GET .../logs:458`, `POST .../start:459`, `POST .../stop:460`, `POST .../restart:461`, `DELETE .../{id}:462`, `POST .../exec:463 → handleContainerExec:792`, `GET .../top:464`, `GET .../changes:465`, `GET .../stats:466`, `GET/POST .../files*:467-470`, **`POST /api/admin/containers:478 → handleContainerCreate:1776` (FIXED)**
  - Images: `GET /api/admin/images:471`, `POST .../build:472`, `POST .../pull:473`, `DELETE .../{id}:474`, `POST .../{id}/tag:475`, **`POST /api/admin/images/prune:476 → handleImagePrune:531` (FIXED at Beacon, UNWIRED at Docker API)**, `GET .../search:477`.
  - Networks: `GET /api/admin/networks:484`, `GET .../{id}:485`, **`POST /api/admin/networks:479 → handleNetworkCreate:1824` (FIXED)**, **`DELETE .../{id}:480 → handleNetworkDelete:1870` (FIXED)**
  - Volumes: `GET /api/admin/volumes:486`, `GET .../{id}:487`, `GET .../usage:488`, **`POST /api/admin/volumes:481 → handleVolumeCreate:1896` (FIXED)**, **`DELETE .../{id}:482 → handleVolumeDelete:1934` (FIXED)**, **`POST .../prune:483 → handleVolumePrune:1961` (FIXED)**

`beacon/internal/server/container_admin.go`:

- `handleContainerList:29` — `ContainerList(All:all)` + filter `modern-game-panel.server_id` for non-infra admins.
- `handleContainerInspect:92`, `handleContainerLogs:141` (tail 100, 512 KiB `LimitReader`, redacts env), `adminContainerAction:203` (start/stop/restart with `State.Running` guards), `handleContainerDelete:276` (force + `X-Confirm-Destructive` + managed block), **`handleContainerCreate:1776` validates `name+image required`, maps `ContainerCreate` with `Config{Image,Cmd,Env,Labels}` only**, `handleContainerExec:792` (infra-admin only, allowlist 20 cmds `ls/pwd/ps/top/df/.../ping`, rejects managed containers, forbids `path.Base` escape, `StdCopy`), `handleContainerTop:909`, `handleContainerChanges:952`, `handleContainerStats:995` (`ContainerStatsOneShot` → `io.Copy` JSON), `handleImageList:365`, `handleImagePull:412` (`ImagePull+Discard`), `handleImageDelete:471` (`X-Confirm-Destructive`), `handleImagePrune:531` (`ImagesPrune` + audit), `handleNetworkList:570`, `handleNetworkCreate:1824` (driver+labels+internal, default bridge, no IPAM), `handleVolumeCreate:1896`, `handleImageBuild:1230` (tar Dockerfile), `handleImageTag:1300`, `handleImageSearch:1358`, `handleContainerFilesList:1414` (`CopyFromContainer` tar parse), `...Read:1484` (10 MiB cap), `...Upload:1559` (multipart tar → `CopyToContainer`), `...Delete:1703` (`validateContainerDeletePath` must be within `/home/container`).

`beacon/internal/server/compose.go:1-751`:

- `validStackID:59` `^[a-z0-9][a-z0-9_-]{0,127}$`
- `validateComposePolicy:77` denies `privileged`, `network_mode: host|service:`, `pid: host`, `userns_mode: host`, `cap_add`, `devices`, `security_opt`, privileged ports `<1024` via `validateComposePorts:181`, bind mounts via `validateComposeVolumes:226`.
- **`shortFormHostPort:214` still `case 2→parts[0] case 3→parts[1] default→""` — single-part `"80"` returns `""` (bypass, re-verified with python split test).**
- `handleComposeDeploy:344` writes `compose.yaml` + `.env` (`encodeComposeEnv:264`) into `composeStack.dirForID(stackID)` (`<dataDir>/../compose/<stackID>`), per-stack mutex, `exec.CommandContext("docker", "compose", "-f", composePath, "-p", stackID, "up", "-d", ["--remove-orphans"]?)` 10m timeout.
- `handleComposeRestart:506` native `docker compose restart` exists (Beacon correct); API `RestartStack:781` still does `StopStack+StartStack` two-phase (see C16).

`beacon/internal/runtime/docker.go:42-1135`: pins API `1.43`, allowlist `DOCKER_HOST` allowlist, `ensureImage:113` digest-pin gate, `Create/Reconcile:151` idempotent via `configHashLabel:899` + `workloadLocks[64]`, `buildResources:783`, `buildHostConfigWithSettings:827` (`CapDrop ALL`, `Privileged false`, `Init true`, `ReadonlyRootfs true`, `SecurityOpt no-new-privileges:seccomp=builtin`, etc.), `ensureNetwork:1065` managed bridge with `managed:true` label.

---

## 3. Reference Inventory (file:symbol)

| Ref | Path:Symbol | Proves |
|-----|-------------|--------|
| Portainer | `reference/app-platforms/portainer/api/docker/client/client.go:28` `ClientFactory` + `CreateClient:42` | Docker client factory switching on endpoint.Type + AgentSignature — Forge analog single-token Admin* |
| Portainer | `reference/app-platforms/portainer/api/docker/container.go:55` `Recreate` | Transactional inspect→pull→stop→rename→recreate→reconnect — Forge has no recreate |
| 1Panel containers | `reference/app-platforms/1panel/agent/app/api/v2/container.go:224` `ListContainer`, `:459` `ContainerCreate`, `:612` `ContainerOperation` (start/stop/restart/pause/unpause/kill), `:915` `ContainerStreamLogs` | Flat list/no concealment, full create, file ops |
| 1Panel networks/volumes/images | `container.go:722` `ListNetwork`, `:740` `DeleteNetwork`, `:762` `CreateNetwork`; `:784` `SearchVolume` `:849` `CreateVolume`; `image.go:100` `ImagePull`, `:77` `ImageBuild` | CRUD matrix |
| Uncloud | `reference/app-platforms/uncloud/pkg/client/service.go:24` `RunService` + `internal/docker/image.go:42` `PullImage` | Validate+VolumeScheduler+mesh deploy; PullImage auth via docker/cli config channel |
| docker-compose | `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:78` `InDependencyOrder` | DAG toposort + condition waits — Forge delegates to engine |
| docker-compose | `create.go:592` `getRestartPolicy` + `:635` `getDeployResources` | Restart + resources mapping into `container.Resources` |
| docker-compose | `envresolver.go` + `loader.go` | `.env`+`env_file`+`${VAR:-default}` etc. |
| Komodo periphery | `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:414` `ComposeUp` 6-phase | write_stack→registry login→pre_deploy→config→build→pull→up -d → post_deploy vs Forge single `up -d` |
| Dokploy | `packages/server/src/utils/docker/compose/volume.ts` + `collision.ts` | Named-volume collision detection — Forge has none |

---

## 4. Parity Matrix — Forge vs Reference (18 rows)

Legend STATUS: `COMPLETE` parity-or-better, `PARTIAL` subset, `UNWIRED` built but not exposed, `BROKEN` wired but fails, `WIRED_BUT_WRONG` semantic divergence, `INTENTIONAL` deliberately not exposed.

| # | Capability | Reference Path:Symbol | Forge UI file:line | Forge API file:line | Forge Daemon/Beacon file:line | STATUS | GAP / LOGIC | FINDING | SEVERITY |
|---|---|---|---|---|---|---|---|---|
| C01 | Container listing & scope filtering | `1panel/.../container.go:224` `ListContainer` flat | `forge/web/lib/api/docker.ts:77` `listContainers` + `containers-view.tsx:56` 15s poll | `handlers_docker.go:98` `dockerListContainers` fan-out `nodeListOrFirst` | `container_admin.go:29` `handleContainerList` — filter `modern-game-panel.server_id` for non-infra | **COMPLETE (stricter)** | Adds least-privilege concealment absent in 1Panel | — | — |
| C02 | Container creation contract | `1panel/container.go:459` `ContainerCreate` (image/name/ports/env/volumes/network/restartPolicy via SDK) | `docker.ts:113` `createContainer` + `container-create-modal.tsx:26` sends `ports/env/volumes/network/restartPolicy` | `handlers_docker.go:136` `dockerCreateContainer` → `daemon.AdminContainerCreate:1883` `POST /api/admin/containers` | **FIXED** `container_admin.go:1776` `handleContainerCreate` + `server.go:478` (was 404) | **PARTIAL (lossy FIXED)** | Beacon handler struct only `{name,image,cmd,env,labels}` — UI `ports/volumes/network/restartPolicy` silently dropped (HostConfig nil) | **F-01** | P2 |
| C03 | Lifecycle start/stop/restart | `portainer/container.go` stop/start; `1panel:612` `ContainerOperation` | `containers-view.tsx:96` per-state buttons; `docker.ts:118` `operateContainer` | `handlers_docker.go:155` switch start/stop/restart → `daemon.AdminContainer*` | `container_admin.go:193` `adminContainerAction` (state checks) | **COMPLETE** | stop/start/restart fully wired; 409 on bad state | — | — |
| C04 | Lifecycle pause/unpause | `1panel/container.go:612` includes pause/unpause | Same `operateContainer` type union includes `pause\|unpause` at `docker.ts:120` + buttons at `containers-view.tsx:142,148` | `handlers_docker.go:176` `case "pause" → AdminContainerPause:1875`, `178 → AdminContainerUnpause:1879` | **Beacon MISSING**: `server.go:459-461` only `start/stop/restart`; `client.go:1875` `POST /api/admin/containers/{id}/pause` → 404 | **BROKEN** | UI→API→Daemon wired, Beacon handler absent — 404 for pause/unpause; also `kill -s` not wired | **F-02** | P2 |
| C05 | Exec (diagnostics) | Portainer unrestricted via edge agent; 1Panel no exec (ws terminal.go) | **No exec export** in `docker.ts` (verified `grep exec → 0`) | **No exec route** in `handlers_docker.go` (verified nil) | `container_admin.go:792` `handleContainerExec` (infra-admin allowlist 20 read-only, managed-block, `StdCopy`) + `daemon.AdminContainerExec:2095` + `server.go:463` mounted | **INTENTIONALLY_UNWIRED** | Beacon+Daemon healthy, API/UI air-gapped — matches MASTER `INTENTIONALLY_NOT_EXPOSED`. No false completion. | — | P3 info |
| C06 | Logs (container one-shot + compose) | `compose/logs.go` streaming; `1panel:915` `ContainerStreamLogs` | `containers-view.tsx:186` `ContainerLogsModal` polls `getContainerLogs` 5s | `handlers_docker.go:207` `dockerContainerLogs` → `AdminContainerLogs:1731` | `container_admin.go:141` caps 512 KiB + `compose.go:661` `docker compose logs --tail` 30s timeout | **PARTIAL** | One-shot+polling only; no WS/SSE streaming (Portainer WS); compose lacks Since/Until/Timestamps | — | P3 |
| C07 | Stats (one-shot → UI) | `1panel:419` `ContainerListStats` bulk | `containers-view.tsx:64` `fetchStats` one-shot on logs open | `handlers_docker.go:223` `dockerContainerStats` → `AdminContainerStats:1892` raw JSON passthrough | `container_admin.go:995` `ContainerStatsOneShot` + `runtime/stats.go:32` `DecodeDockerStats` sibling path not used for admin | **PARTIAL** | Manual trigger, not auto-polled; bulk stats N+1; no decoded cpu/mem; `StatsStream:580` unused for admin | — | P2 |
| C08 | Images: pull/build/push/tag/search/remove | `1panel/image.go:100/77/123/192/146` | `docker.ts:147` `listImages/pullImage/deleteImage` + `:189` build/push/tag/search; `images-view.tsx:46` | `handlers_docker.go:240` `dockerListImages`, `:280` `dockerPullImage` (`validateContainerImageReference:14`, 512 limit), `:569` `dockerBuildImage`, `:595` `dockerPushImage`, `:619` `dockerTagImage`, `:644` `dockerSearchImages` | `container_admin.go:412` `handleImagePull`, `:471` `handleImageDelete`, `:1230` `handleImageBuild`, `:1300` `handleImageTag`, `:1358` `handleImageSearch` | **COMPLETE** | Pull forwards `registryAuth` verbatim (no Uncloud auto-fetch); build tars inline Dockerfile only (no compose build context) | — | — |
| C09 | Image prune | `1panel` no explicit prune (image prune via `ImagesPrune` Docker API) | **No UI** `pruneImages` in `docker.ts` (only `pruneVolumes:185`) | **No route** `POST /docker/images/prune` in `handlers_docker.go` (only `POST /volumes/prune:63`); `handlers_portainer.go:40/557` has `pruneImages` under `/portainer` → `AdminImagePrune:1845` | **Beacon READY**: `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune`, Daemon `1845` ready | **UNWIRED at Docker admin** | Beacon+Daemon wired but Docker admin API never exposes it — dead code under `/docker`; only `/portainer` exposes prune | **F-03** | P3 |
| C10 | Networks: list/inspect + attached count | `1panel:722` `ListNetwork` | `docker.ts:159` `listNetworks`; `networks-view.tsx` table | `handlers_docker.go:340` `dockerListNetworks` attachedCount via `Containers` map | `container_admin.go:570` `handleNetworkList` filters platform nets for non-infra | **COMPLETE** | Read parity | — | — |
| C11 | Networks: create/delete (+ IPAM) | `1panel:762` `CreateNetwork` (bridge/host/overlay/macvlan + CIDR) | `docker.ts:163` `createNetwork` `{name,driver,subnet,nodeId}` + `networks-view.tsx` subnet field | `handlers_docker.go:377` `dockerCreateNetwork` → `AdminNetworkCreate:2038`, `396 → AdminNetworkDelete:2047` | **FIXED** `container_admin.go:1824` `handleNetworkCreate` + `server.go:479`, `1870` delete + `480` | **COMPLETE (lossy)** | UI `subnet` field never forwarded — Beacon `CreateOptions` sets only `Driver/Labels/Internal`, no `IPAMConfig` (silent drop) | **F-04** | P3 |
| C12 | Volumes: list/inspect/usage | `1panel:809` `ListVolume` | `docker.ts:172` `listVolumes` | `handlers_docker.go:415` `dockerListVolumes` decodes `{volumes[]}` | `container_admin.go:657` `handleVolumeList` + `:705` inspect + `:752` usage (`DiskUsage`) | **COMPLETE** | Read parity with managed label flag | — | — |
| C13 | Volumes: create/delete | `1panel:849` `CreateVolume` | `docker.ts:176` `createVolume {name,driver}` | `handlers_docker.go:454` `dockerCreateVolume` → `AdminVolumeCreate:2064` | **FIXED** `container_admin.go:1896` `handleVolumeCreate` + `server.go:481`, `1934` delete + `482` | **COMPLETE** | Phase-01 404 now 201 | — | — |
| C14 | Volumes: prune | `1panel:503` `ContainerPrune`; Docker `VolumesPrune` | `docker.ts:185` `pruneVolumes` + `volumes-view.tsx:44` Eraser button | `handlers_docker.go:667` `dockerPruneVolumes` fan-out `AdminVolumePrune:2090` + `recordAudit` | **FIXED** `container_admin.go:1961` `handleVolumePrune` + `server.go:483` `VolumesPrune` (`filters.NewArgs()`) | **COMPLETE** | Was 404, now both ends wired | — | — |
| C15 | Container files (list/read/upload/delete) | `1panel/container.go:112` `UploadContainerFile` + `1panel/file.go` super-set | `docker.ts:207` `listContainerFiles/readContainerFile/uploadContainerFile/deleteContainerFile` | `handlers_docker.go:493/510/531/548` → `AdminContainerFiles*:1961/1966/1992/2015` | `container_admin.go:1414` `CopyFromContainer` tar parse, `:1484` read 10 MiB cap, `:1559` upload multipart tar → `CopyToContainer`, `:1703` delete via `validateContainerDeletePath` confined to `/home/container` | **COMPLETE** | Confinement hardened vs 1Panel unrestricted | — | — |
| C16 | Compose lifecycle (up/down/stop/start/restart/pull/ps/logs) incl. isolation | `docker-compose` `up.go/down.go/restart.go/scale.go` | `forge/web/lib/api/compose.ts:53` `createComposeStack` + `89` `deploy/stop/start`, `101` `getComposeStackStatus` 10s poll | `daemon/compose.go:48` `ComposeDeploy`, `73-86` stop/start/restart/delete, `110` status, `131` logs; `handlers_compose.go` composes | `compose.go:344` single `up -d` shellout; `:506` native `restart`; `:550` `down -v` hard-coded | **PARTIAL** | Fidelity: deploy/start/stop/pull/status/logs parity high; `RestartStack:781` is Stop+Start two-phase while Beacon has native restart; `down -v` unconditional (data-loss per final-parity); `scale`/`replicas` not exposed; `build.context` not shipped (MISSING). Resources/restart policy delegated to engine. | — | P2 (see finding) |
| C17 | Compose port policy — privileged port guard | `1panel` no guard (allows privileged); `komodo` delegates to Docker | — (compose UI) | `service.go:427` `ValidateComposeSecurity` has **no** port check | `compose.go:181` `validateComposePorts` + `shortFormHostPort:214` | **BROKEN (bypass)** | `shortFormHostPort` returns `""` for `len==1` (`"80"`) → `if published=="" continue` skips `<1024` check; attacker can publish `"80"` via short-form or `"127.0.0.1::80"` | **F-05** | P1 |
| C18 | Per-service resource limits × replicas vs per-stack ceilings | `compose/create.go:641` `getDeployResources` → `container.Resources{Memory,NanoCPUs,Pids}` | — (compose YAML holds `deploy.resources.limits`) | `lifecycle.go:280` ceilings check only `req.MemoryMB/DiskMB/CPUShares` (top-level) | Game `docker.go:783` `buildResources` + `827` `buildHostConfigWithSettings` enforces tier1 isolation; Compose path `compose.go:344` runs `up -d` without overrides, **no** Beacon ceiling check; `lifecycle.go:139` ceilings never multiplied by `deploy.replicas` | **BROKEN (quota bypass)** | 10-replica `memory: 8G` compose → `8G` per-stack check passes but host needs `80G`; `sum(limits*replicas)` unchecked; compose bypasses `ReadonlyRootfs/CapDrop` unless in YAML | **F-06** | P1 |

Summary: COMPLETE 6, FIXED 3 (C02 lossy, C11 lossy, C13/C14 clean), PARTIAL 5, BROKEN/UNWIRED/WIRED_BUT_WRONG 4 — total 18 rows (≥15 required). Every BROKEN/WIRED_BUT_WRONG has FINDING in §6.

---

## 5. End-to-End Chain Traces (UI → API → daemon → Beacon → Docker) — with hop tables

> Each hop is `file:line` verifiable. Broken chains have **Break** note with location. Verified against current checkout (2026-08-24) via `grep -n` and file reads.

### Trace T1 — `listContainers` (read — HEALTHY)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts:77` `listContainers({all})` → `forge/web/components/docker/containers-view.tsx:56` `useQuery(["docker","containers"], listContainers, refetchInterval:15000)` | `fetchJSON("/docker/containers?all=${all}")` → flatten `nodeId/nodeName/containers`; fallback `c.State ?? c.state` at `docker.ts:90` |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go:98` `dockerListContainers` | `nodeListOrFirst()` fan-out → `daemon.AdminContainerList` per node, swallow per-node errors, aggregate `fiber.Map{nodeId,nodeName,containers}` |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:1718` `AdminContainerList` | `GET /api/admin/containers?all=` with signed `newRequest` + `retryRoundTripper` `resignRequest` |
| 4 Beacon | Server | `beacon/internal/server/container_admin.go:29` `handleContainerList` | `adminDockerClient()` → `docker.ContainerList(All:all)` → filter `managed = labels["modern-game-panel.server_id"]` for non-infra admins (`getAdminUserInfo:1151` `IsInfraAdmin`) |
| 5 Docker | SDK | `github.com/docker/docker/client.ContainerList` | real SDK call |

**Result:** `COMPLETE`. Sync read, no queue. Platform nets filtered for containers via label; non-infra cannot see managed.

### Trace T2 — `createContainer` (mutating — NOW HEALTHY, was BROKEN 404 in Phase-1)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts:113` `createContainer({image,name,ports,env,volumes,network,restartPolicy,nodeId})` | `POST /docker/containers?node=` body `CreateContainerRequest`; modal at `forge/web/components/docker/container-create-modal.tsx:57` maps ports/env/volumes |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` | `c.BodyParser(&body map[string]any)` → `resolveDockerNode` → `daemon.AdminContainerCreate:1883` + `recordAudit container:create` |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:1883` `AdminContainerCreate` | `POST /api/admin/containers` (`adminPostJSON`) with fresh nonce `resignRequest` |
| 4 Beacon | Server | `beacon/internal/server/container_admin.go:1776` `handleContainerCreate` + `beacon/internal/server/server.go:478` `POST /api/admin/containers` | **FIXED**: previously `server.go:440` had no POST route. Now validates `name+image required` at `1798`, builds `container.Config{Image,Cmd,Env,Labels}` → `docker.ContainerCreate` at `1813` → returns `{id, warnings}`. `logAdminAction:1802` audits. |
| 5 Docker | SDK | `client.ContainerCreate` at `container_admin.go:1813` | real create with `container.Config` + nil HostConfig/NetworkingConfig |

**Result:** `COMPLETE` — remediation verified (`server.go:478` + `container_admin.go:1776`). **Remaining nuance:** handler's request struct at `1787-1793` only binds `{name,image,cmd,env,labels}` — UI `ports/volumes/network/restartPolicy` are in `map[string]any` at Daemon but dropped at Beacon (HostConfig nil). Not 404 but lossy success (see C02 / F-01). No test covers HostConfig forwarding ( `handlers_docker_test.go:10` only nil-store 404).

### Trace T3 — `network create` (mutating — NOW HEALTHY, was BROKEN)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts:163` `createNetwork({name,driver,subnet,nodeId})` | `POST /docker/networks?node=`; `forge/web/components/docker/networks-view.tsx:18` form includes driver+subnet |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go:377` `dockerCreateNetwork` | `c.BodyParser(&body map[string]any)` → `resolveDockerNode` → `daemon.AdminNetworkCreate:2038` |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:2038` `AdminNetworkCreate` | `POST /api/admin/networks` (`adminPostJSON`) |
| 4 Beacon | Server | `beacon/internal/server/container_admin.go:1824` `handleNetworkCreate` + `server.go:479` `POST /api/admin/networks` | **FIXED**: was `GET`-only at `server.go:462-463`. Now validates `name required` `1845`, defaults `driver→bridge` `1856`, `docker.NetworkCreate` `1859` with `{Driver,Labels,Internal}` → `{id,warning}` |
| 5 Docker | SDK | `client.NetworkCreate` at `1824:1859` |  |

**Result:** `COMPLETE` with caveat: `CreateNetworkRequest.subnet` from UI is never bound — Beacon struct at `1835-1839` has `Name/Driver/Labels/Internal` only, no `Subnet/IPAM`. Subnet silently dropped (see C11 / F-04).

### Trace T4 — `volume prune` (destructive — NOW HEALTHY, was BROKEN / crossed)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts:185` `pruneVolumes` → `forge/web/components/docker/volumes-view.tsx:44` `pruneMut` Eraser button | `POST /docker/volumes/prune` |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go:667` `dockerPruneVolumes` | `nodeListOrFirst` fan-out → `daemon.AdminVolumePrune:2090` per node + `recordAudit volume:prune:680` |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:2090` `AdminVolumePrune` | `POST /api/admin/volumes/prune` via `adminPostJSON` |
| 4 Beacon | Server | `beacon/internal/server/container_admin.go:1961` `handleVolumePrune` + `server.go:483` `POST /api/admin/volumes/prune` | **FIXED**: previously `server.go` only `GET /api/admin/volumes`. Now `VolumesPrune(ctx, filters.NewArgs())` `1979` → report |
| 5 Docker | SDK | `client.VolumesPrune` at `1979` |  |

**Contrast** — image prune uses sibling handler `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` + Daemon `1845`, but Docker admin API `handlers_docker.go:63` never mounts `POST /docker/images/prune` (only volumes prune). Image prune is thus HEALTHY at Beacon/Daemon but **UNWIRED at Docker API** — exposed only under `handlers_portainer.go:40` `/portainer/images/prune` (see C09 / F-03). Phase-01 "crossed wires" is half-fixed: volume prune fixed both ends, image prune still half-wired.

### Trace T5 — `pause`/`unpause` (operate — STILL BROKEN)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts:118` `operateContainer(id,"pause")` + `containers-view.tsx:142` Pause button (running→pause) and `148` Unpause | Union type includes `pause\|unpause` `docker.ts:120` |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go:176` `case "pause": opErr = AdminContainerPause`, `178 → AdminContainerUnpause` | forwards to Daemon |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:1875` `AdminContainerPause` → `adminContainerAction(...,"pause")` `1768` `POST /api/admin/containers/{id}/pause` | signed retry |
| 4 Beacon | Server | `beacon/internal/server/server.go:459-461` mux table | **Break**: registers `POST /api/admin/containers/{id}/start/stop/restart` but **no** `.../pause` or `.../unpause`. Daemon 404. No `handleContainerPause/Unpause` exists in `container_admin.go` (grep `handleContainer.*Pause` 0). |
| 5 Docker | SDK | `client.ContainerPause` (would be target) | never reached |

**Break:** `WIRED_BUT_WRONG` / `BROKEN` at Beacon. Final-parity C03 already flagged `F-11` — re-verified still present via `grep -n pause beacon/internal/server/server.go` (empty). `dockerPause` at Beacon `container_admin.go:203` only handles start/stop/restart switch, no pause case.

### Trace T6 — `exec` diagnostic (INTENTIONALLY_UNWIRED — not a regression)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/docker.ts` | **no exec export** (verified `grep exec → 0`) |
| 2 API | HTTP | `forge/api/internal/http/handlers_docker.go` | **no exec route** (verified `grep exec → 0`) |
| 3 Daemon | Client | `forge/api/internal/daemon/client.go:2095` `AdminContainerExec` | `POST /api/admin/containers/:id/exec` `{cmd,attachStdout,attachStderr,tty}` |
| 4 Beacon | Server | `beacon/internal/server/container_admin.go:792` `handleContainerExec` + `server.go:463` `POST .../exec` | infra-admin only `800`, allowlist 20 read-only cmds `832` (`ls/pwd/ps/top/df/.../ping`), `path.Base` check `845`, rejects managed containers `865`, `ContainerExecCreate/Attach` + `StdCopy` |
| 5 Docker | SDK | `client.ContainerExecCreate/Attach` at `874/879` |  |

**Result:** `INTENTIONAL`. Beacon+Daemon healthy, API/UI deliberately air-gapped (MASTER `INTENTIONALLY_NOT_EXPOSED`). Matches final-parity C04 judgment.

### Trace T7 — `compose privileged port` policy (BROKEN bypass — shortFormHostPort)

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/compose.ts:53` `createComposeStack` + direct `fetch POST /compose/:id/deploy` | compose YAML supplied by user |
| 2 API | HTTP | `forge/api/internal/services/compose/service.go:427` `ValidateComposeSecurity` | has **no** privileged-port check |
| 3 Daemon | Client | `forge/api/internal/daemon/compose.go:48` `ComposeDeploy` | `POST /compose/deploy {stackId,composeYaml,envVars}` |
| 4 Beacon | Server | `beacon/internal/server/compose.go:181` `validateComposePorts` → `shortFormHostPort:214` at `190` | `shortFormHostPort` splits on `:`; `len 2→parts[0]`, `len 3→parts[1]`, **default→""**. For `"80"` (len 1) returns `""`, then `if published=="" continue` skips `<1024` check at `198` |
| 5 Docker | CLI | `docker compose up -d` shellout `compose.go:399` | Docker publishes privileged port despite policy |

**Bypass proof** (python split test): `"80"→""`, `"127.0.0.1::80"→""` both bypass; `"8080:80"→"8080"` correctly checked. Long-form `published: 22` is handled correctly (`fmt.Sprint(raw)` path). Still BROKEN as originally reported in `phase-06/subagent-07-komodo-stacks.md` LF-01 and `final-parity` C17.

### Trace T8 — `compose per-replica resources` (BROKEN quota bypass) — extra 5+ trace

| Hop | Layer | File:line | Evidence |
|-----|-------|-----------|----------|
| 1 UI | Web | `forge/web/lib/api/compose.ts:53` `createComposeStack` with `composeYaml` containing `deploy.replicas` + `resources.limits` | parsed via `service.go:92` `rawService.Deploy` |
| 2 API | HTTP | `forge/api/internal/services/compose/lifecycle.go:280` per-stack ceiling check | `if req.MemoryMB > MaxUserStackMemoryMB` only — top-level `MemoryMB/CPUShares/DiskMB` (ceilings `65536/512000/8192` at `150-152`). Never computes `sum(per-service limits × replicas)` |
| 3 Daemon | Client | `daemon/compose.go:48` `ComposeDeploy` | forwards YAML verbatim |
| 4 Beacon | Server | `beacon/internal/server/compose.go:344` `handleComposeDeploy` | `validateComposePolicy` denies `privileged/cap_add` but not resource quotas; `up -d` inherits YAML limits verbatim |
| 5 Docker | CLI | `docker compose up -d` | deploys 10×8 GiB replicas bypassing 64 GiB per-stack ceiling |

**Result:** `BROKEN` / `WIRED_BUT_WRONG` — quota is per-stack request, not per-service multiplied. Affects C18. Final-parity already flagged as `F-03` P1, still present.

---

## 6. Re-verification of FIXED Claims (vs prior 404)

| Claimed FIX | Prior 404 location | Current handler | Daemon client | Route mounted? | Verified HTTP semantic | Still gaps? | Verdict |
|---|---|---|---|---|---|---|---|
| `POST /api/admin/containers:1776` now exists | `server.go:440` no POST (only GET/DELETE) → 404 | `container_admin.go:1776` `handleContainerCreate` validates `name+image required`, `ContainerCreate` at `1813` | `client.go:1883` `AdminContainerCreate` `POST /api/admin/containers` | `server.go:478` `POST /api/admin/containers` **YES** | `201 {id,warnings}` on success, `400` on missing fields, `409` on conflict; `logAdminAction` audits | Lossy: `ports/volumes/network/restartPolicy` dropped (HostConfig nil) | **FIXED (lossy)** |
| `POST /api/admin/networks:1824` | `server.go:462-463` only GET → 404 | `container_admin.go:1824` `handleNetworkCreate` (driver default bridge) | `client.go:2038` `AdminNetworkCreate` | `server.go:479` **YES** | `201 {id,warning}` | Subnet/IPAM silently dropped; UI field no-op | **FIXED (lossy)** |
| `POST /api/admin/volumes:1896` + `prune:1961` | `server.go:464-466` only `GET /api/admin/volumes` etc. → 404 | `container_admin.go:1896` `handleVolumeCreate` (Name/Driver/Labels) → `VolumeCreate` `1923`; `1961` `handleVolumePrune` `VolumesPrune` `1979` | `client.go:2064` `AdminVolumeCreate`, `2090` `AdminVolumePrune` | `server.go:481` + `483` **YES** | `201` volume JSON; `200` prune report | Clean FIXED | **FIXED** |
| Image prune (sibling of volume prune) | Beacon `container_admin.go:531` existed but API had no `/docker/images/prune` → dead code | `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` ready | `client.go:1845` `AdminImagePrune` ready | **No** `handlers_docker.go` mount (only `handlers_portainer.go:40` under `/portainer/images/prune` `557`) | Docker admin path 404; Portainer path 200 | API UNWIRED at Docker surface | **HALF-FIXED → still BROKEN for Docker admin** (carry-forward from FL-02) |

Bash verification used: `grep -n "POST /api/admin/containers\|POST /api/admin/networks\|POST /api/admin/volumes"` on `server.go` confirms 478/479/481/483 present; `grep -n "handleContainerCreate\|handleNetworkCreate\|handleVolumeCreate\|handleVolumePrune"` confirms 1776/1824/1896/1961; `grep -n pause beacon/internal/server/server.go` confirms absence.

---

## 7. Logic Findings (≥4 required, each broken chain has hop table file:line)

### F-01 — `shortFormHostPort` privileged-port bypass for `ports: ["80"]` / `["127.0.0.1::80"]` (P1, OPERATOR_VISIBLE, WIRING/POLICY)

- **Finding:** `beacon/internal/server/compose.go:214` returns `""` for `len(parts)!=2,3` (single-part `"80"` → `["80"]` len 1 → `""`). Caller `validateComposePorts:190` does `published = shortFormHostPort(v)` then `if published=="" continue` — skips the `<1024` error at `205`. Result: `ports: ["80"]` (Compose interprets as `80:80` or random host port depending on engine) or `"127.0.0.1::80"` (empty host port part) publishes a privileged host port undetected. Long-form `published: 80` is correctly checked via `fmt.Sprint(raw)` branch `192`, so bypass is short-form specific. API `ValidateComposeSecurity:427` has no port check at all, so neither layer catches it.
- **Evidence:** `compose.go:214-223` function + `181-209` loop; python split test confirming `"80"→""`, `"127.0.0.1::80"→""` vs `"8080:80"→"8080"` correct. Reference `phase-06/subagent-07-komodo-stacks.md:LF-01` identical logic already reported — still present on 2026-08-24 checkout (no patch found via `git log` search or code diff).
- **Reference contrast:** 1Panel has no privileged-port guard; Komodo delegates to Docker; docker-compose has no guard. Forge intended to harden — bypass defeats hardening.
- **Impact:** Host-level port hijacking on shared nodes (bind 80/22/443 without error).
- **Recommendation (no code change per audit, but record):** Fix `shortFormHostPort` to `case 1: return parts[0]` (treat bare `"80"` as published port) and strip `/tcp` suffix + `strings.TrimSuffix(entry, "/tcp")` + handle `"80:80/tcp"` (already partly handled via `Atoi` ignoring suffix? actually `"80/tcp"` fails Atoi → silent continue — also bypass for `"/tcp"` suffix). Prefer reuse `docker/go-connections/nat.ParsePortSpec` for fidelity.

### F-02 — `pause`/`unpause` operate still 404 — UI→API→Daemon wired, Beacon handler missing (P2, USER_VISIBLE)

- **Finding:** UI `forge/web/lib/api/docker.ts:118` includes `pause|unpause` in action union; `containers-view.tsx:142,148` exposes Pause/Unpause buttons; API `handlers_docker.go:176` forwards `pause` → `AdminContainerPause:1875` (`POST /api/admin/containers/{id}/pause` via `adminContainerAction:1768`); Beacon `server.go:459-461` only mounts `start/stop/restart`, no pause/unpause handler in `container_admin.go` (no `handleContainerPause`).
- **Trace:** T5 hop table above — break at `server.go:459-461` mux. Daemon will 404. `grep -n pause beacon/internal/server/server.go` (0 hits) confirms.
- **Impact:** Admins see Pause/Unpause buttons but operation always fails `502 Bad Gateway` (Daemon 404 → API `StatusBadGateway`).
- **Recommendation:** Implement `handleContainerPause/Unpause` in `container_admin.go` mirroring `adminContainerAction` with `ContainerPause/Unpause` + register `POST /api/admin/containers/{id}/pause|unpause` in `server.go`, or remove `pause|unpause` from `operateContainer` type and UI buttons. No `kill -s SIGNAL` path either (`beacon/internal/runtime/docker.go:511` `Signal` exists but not wired to admin).

### F-03 — Image prune UNWIRED at Docker admin API — dead Beacon code (P3, SILENT)

- **Finding:** Beacon `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` + Daemon `client.go:1845` `AdminImagePrune` (`POST /api/admin/images/prune`) are healthy. But `forge/api/internal/http/handlers_docker.go` never mounts `POST /docker/images/prune` (only `POST /volumes/prune:63`). Volum prune trace T4 is fully wired both ends; image prune is only reachable under `handlers_portainer.go:40/557` `POST /portainer/images/prune` (Portainer surface). Docker admin UI `docker.ts:159` has no `pruneImages` export, but a caller using Daemon client directly or future UI would 404.
- **Impact:** Prune asymmetry: volumes prune works, images prune dead under Docker tab (requires Portainer prefix).
- **Recommendation:** Add `docker.Post("/images/prune", ...)` in `registerDockerRoutes` mirroring `dockerPruneVolumes` → `AdminImagePrune`, or delete Beacon `handleImagePrune`/`server.go:476` to avoid dead code. Document that Docker admin image prune intentionally lives under `/portainer` if that is the intended topology.

### F-04 — Volume `/down -v` hard-coded + orphan data-loss + resource/per-replica quota gaps carried from Phase-06 (P1-P2)

- **Finding A — data-loss:** `beacon/internal/server/compose.go:578` always runs `down -v` even when stack has `postgres` named volumes; upstream `docker-compose` opts-in via `options.Volumes`. Final-parity C16 already flagged as P1 data-loss. No condition checks `up` `RemoveOrphans` gate.
- **Finding B — per-replica bypass:** `lifecycle.go:280` ceilings check `req.MemoryMB/DiskMB/CPUShares` only, never `sum(service Deploy.Resources.Limits + Replicas)`. Beacon `handleComposeDeploy:344` never checks at all. Example: `deploy: {replicas:10, resources:{limits:{memory:8G}}}` passes per-stack 64 GiB but needs 80 GiB. Verified via `grep -n "MaxUserStack" lifecycle.go` and service.go `normalizeDeploy:842` storing `Replicas` but never consumed in deployment.
- **Evidence:** `compose.go:550-596` delete path; `lifecycle.go:143-152` ceilings; `service.go:846` replicas parsed.
- **Impact:** Operator can bypass quota via replicas; `down -v` can wipe production DB volumes on delete.
- **Recommendation:** Compute `totalMemory = sum(parseMemory(deploy.resources.limits.memory) * max(1,replicas))` at `ValidateCompose` / placement time; enforce ≤ ceiling; make `down -v` opt-in (`?volumes=true` query).

### F-05 — Container/Network create FIXED but lossy — silent field drops (P2, USER_VISIBLE)

- **Finding:** `handleContainerCreate:1787-1813` only binds `name/image/cmd/env/labels` and calls `docker.ContainerCreate(config, nil, nil, nil, name)` with nil HostConfig/NetworkingConfig. UI `container-create-modal.tsx:26-57` and `docker.ts:49-58` `CreateContainerRequest` supply `ports/volumes/network/restartPolicy` — all discarded. Similarly `handleNetworkCreate:1835-1839` ignores `CreateNetworkRequest.subnet` (UI `networks-view.tsx` subnet field) — `network.CreateOptions` sets `Driver/Labels/Internal` only, no `IPAM`. Both wires no longer 404 but silently do less than the UI promises.
- **Trace:** T2 and T3 hop tables above — Hop 4 Beacon struct narrowness proven.
- **Impact:** Admin creates container expecting port 8080→80 to fail open (container starts with no ports); network expects subnet CIDR but gets default bridge IPAM — misleading success.
- **Recommendation:** Extend `handleContainerCreate` struct to accept `ports/volumes/network/restartPolicy` and build `container.HostConfig`/`NetworkingConfig` via `nat.ParsePortSpec` + `mount.Mount` + `restart.Policy`; extend `handleNetworkCreate` to forward `IPAMConfig` from `subnet` (or remove `subnet` from UI/types until wired, to avoid false expectation). Parity check vs 1Panel `container.go:459` low validation but full SDK pass-through — Forge dropped fields.

### F-06 — Exec allowlist correctly air-gapped but host file allowlist and privileged port policy remain bifurcated (P3 info + carried)

- **Finding:** Exec is correctly `INTENTIONALLY_UNWIRED` at API (see T6) with strong Beacon allowlist (`container_admin.go:832` 20 read-only cmds, `path.Base` check, `managed` block, infra-admin only). This is good. However sibling `allowedMounts`/`hostfiles` policy shows same bifurcation pattern: game path `beacon/internal/server/mounts.go:64` `allowedMountSource` (`EvalSymlinks`+`Rel` within `allowedMounts`, empty→deny all) is confinement-correct; **compose path** `compose.go:226` `validateComposeVolumes` rejects **all** absolute host mounts unconditionally without consulting allowlist (even `allowed_mounts=/opt/appdata`). API `service.go:663` `ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts)` has admin+allowlist gate but compose path never plumbs `allowedMounts`/`isAdmin` into `composeDeployRequest`. Result: validation `200 valid` → deploy `400 policy violation` divergence (final-parity F-04, 1Panel-infra-monitor LF-02 trifurcation) still present.
- **Impact:** Compose stacks cannot use allowed host paths even when operator configured them; no way to convey allowlist to Beacon.
- **Recommendation:** Document `exec` as intentional (`INTENTIONALLY_NOT_EXPOSED` with audit+allowlist disclosure) and, if compose host mounts are desired, plumb `allowedMounts+isAdmin` through `ComposeDeployRequest` and enforce in `validateComposeVolumes` with allowlist rather than blanket reject; otherwise make API reject absolute mounts (`error` not `warning`) to match Beacon and remove allowlist feature until Beacon can enforce it.

---

## 8. File:Line Index (key citations)

- `reference/app-platforms/portainer/api/docker/client/client.go:28,42` factory; `portainer/api/docker/container.go:55` Recreate
- `reference/app-platforms/1panel/agent/app/api/v2/container.go:224,459,612,915` containers; `:697,722,740,762,784,809,827,849` networks/volumes; `image.go:77,100,123,146,192` images
- `forge/web/lib/api/docker.ts:77,109,113,118,126,134,143,147,151,155,159,163,172,185,189,201,207` UI contracts; `forge/web/components/docker/containers-view.tsx:56,96,142,148,186` view; `container-create-modal.tsx:14,26,57` create modal; `networks-view.tsx:18`, `volumes-view.tsx:18,44`, `app/admin/docker/page.tsx:14` tabs
- `forge/api/internal/http/handlers_docker.go:14,30,33,34,36,37,38,39,40,46,48,49,52,55,60,63,98,136,155,176,178,191,207,223,240,280,340,377,396,415,454,473,570,595,619,644,667` docker admin routes; `handlers_portainer.go:40,557` image prune alt path; `daemon/client.go:1718,1726,1731,1756,1760,1764,1785,1802,1807,1828,1845,1850,1855,1860,1865,1870,1875,1879,1883,1892,1899,1908,1933,1954,1961,1966,1992,2015,2038,2047,2064,2073,2090,2095,2106` Admin* clients; `daemon/compose.go:48,73,81,89,110,131` Compose clients
- `beacon/internal/server/server.go:408,456,459,463,464,471,472,476,478,479,481,483,484,486` mux table; `container_admin.go:29,92,141,193,276,365,412,471,531,570,608,657,705,752,792,832,865,909,952,995,1124,1230,1300,1358,1414,1484,1559,1703,1776,1824,1870,1896,1934,1961` handlers; `compose.go:59,77,181,212,226,251,264,283,344,399,506,550,604,661,709` compose policy/lifecycle; `runtime/docker.go:56,113,151,783,827,1065,511,580` runtime isolation/resources
- `forge/api/internal/services/compose/service.go:92,427,530,594,663,765,846,850` parser/validation; `lifecycle.go:138,150,188,280,781` ceilings/WaitForHealthy/RestartStack; `gitops.go:841` DriftCheck (companion, not this subagent's primary)
- `beacon/internal/runtime/stats.go:32` `DecodeDockerStats`; `enhanced_stats.go:59` `DecodeEnhancedStats`; `factory.go:16` Factory
- Phase-01 synthesis: `audits/phase-01/subagent-03-runtime-compose.md:1,98,132,150,178,277,339` baseline; `phase-01-remediation/*.md` (tracked but not re-read in depth)
- Final-parity: `audits/final-parity/subagent-04-runtime-compose.md:15,102,112,153,180` claimed FIXED rows; `audits/phase-06/subagent-07-komodo-stacks.md:181,213` shortFormHostPort LF-01 (still open)
- Infra: `audits/phase-06/subagent-04-1panel-infra-monitor.md:19,68` file/CPU/monitor gaps for context (not primary for this dimension)

---

## 9. Checklist

- [x] `forge/web/lib/api/docker.ts`, `forge/web/app/admin/docker/*`, `forge/web/components/docker/*` re-inspected
- [x] `forge/api/internal/http/handlers_docker.go`, `forge/api/internal/daemon/client.go` AdminContainer*/AdminImage*/AdminNetwork*/AdminVolume* (**1883/2038/2064/2090** current lines, 1892/2047/2073 in Phase-01 refs) verified
- [x] `beacon/internal/server/container_admin.go:28,1776,1824,1896,1961,792` + `beacon/internal/server/server.go:440-483` mux table verified — FIXED claims `POST /api/admin/containers|networks|volumes|prune` now exist
- [x] Exec allowlist verified intentionally unwired at API (`handlers_docker.go` 0 exec routes, `daemon:2095` + `container_admin.go:792` + `server.go:463` healthy)
- [x] `shortFormHostPort ["80"]` bypass verified still present via python split + `compose.go:214` code read
- [x] Per-replica resources verified still unchecked (`lifecycle.go:280` only `req.MemoryMB`)
- [x] ≥15 parity rows (18) + ≥4 findings (6) + 6 hop-traced chains (T1-T8) with file:line per hop, each broken chain has hop table
- [x] No product code modified

---

*Generated by subagent 08 (reverification, parallel 08/20). No product files modified. All 20 subagents run in parallel; this audit is one slice.*
