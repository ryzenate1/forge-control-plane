# Subagent 05 — Runtime / Compose Confirmation (Phase 02 Agent 05/10)

> Focus: Confirm Runtime/Compose implementations vs phase-01 subagent-03 + reverification-08/09 + final-parity subagent-04 RT findings against LIVE.
> Date: 2026-08-24
> Auditor: subagent-05 (phase-02, parallel 05/10)
> Method: source inspection (Read/Bash/Grep). All citations `file:line` verified on current checkout. No product code modified.

---

## 1. Reconciliation Summary vs Prior Audits

| Prior ID | Capability | Prior Verdict | Live Verdict (this report) | Evidence |
|---|---|---|---|---|
| Phase-01 FL-01 `container_admin.go` create 404 | `POST /api/admin/containers` | BROKEN | **FIXED (lossy)** | `beacon/internal/server/container_admin.go:1776` + `beacon/internal/server/server.go:478` now exists; `forge/api/internal/daemon/client.go:1883` `AdminContainerCreate` resolves; `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` → `AdminContainerCreate` |
| Phase-01 FL-01 network create 404 | `POST /api/admin/networks` | BROKEN | **FIXED (lossy)** | `container_admin.go:1824` + `server.go:479` **FIXED**; `daemon/client.go:2038` `AdminNetworkCreate` |
| Phase-01 FL-01 volume create/delete 404 | `POST /api/admin/volumes` | BROKEN | **FIXED** | `container_admin.go:1896` + `server.go:481` + `1934/482`; `daemon/client.go:2064` |
| Phase-01 FL-02 / Re-08 F-03+ F-14 | volume prune 404 | BROKEN | **FIXED** | `container_admin.go:1961` + `server.go:483` `POST /api/admin/volumes/prune`; `daemon/client.go:2090` `AdminVolumePrune`; `handlers_docker.go:63` `POST /volumes/prune` → `dockerPruneVolumes:667` fan-out |
| Re-08 F-02 / Final-parity C03 F-11 | pause/unpause `client.go:1875→server.go:459` missing | BROKEN (still) | **STILL BROKEN** | `daemon/client.go:1875` `AdminContainerPause` + `1879` `AdminContainerUnpause` exist but `beacon/internal/server/server.go:459-461` only `start/stop/restart` — no `pause/unpause` handler; `container_admin.go` has no `handleContainerPause` |
| Re-08 F-03 / Final-parity C07 F-13 | image prune dead | HALF-FIXED | **STILL BROKEN (at Docker admin)** / **INTENTIONAL topology** | `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` + `daemon/client.go:1845` `AdminImagePrune` ready, but `handlers_docker.go:63` only mounts `POST /volumes/prune` — no `POST /docker/images/prune`; image prune only under `forge/api/internal/http/handlers_portainer.go:40/557` |
| Re-08 F-01 / Re-09 LF-06 / Final-parity C17 | `beacon/compose.go:213` `shortFormHostPort` `["80"]` bypass, `:214` `return ""` | BROKEN | **STILL BROKEN** | `beacon/internal/server/compose.go:214` `default: return ""` for `len==1`; `compose.go:190` `published=shortFormHostPort(v)` → `if published=="" continue` skips `<1024` check at `:206` |
| Re-08 F-06 / Re-09 LF-02 / Final-parity C14 F-03 | `lifecycle.go:280` per-replica ceilings | BROKEN | **STILL BROKEN** | `forge/api/internal/services/compose/lifecycle.go:280` checks only `req.MemoryMB/DiskMB/CPUShares` top-level `150-152`; never `sum(limits*replicas)` |
| Re-09 LF-03 / Final-parity C15 F-04 | `service.go:594` vs `compose.go:226` volume bifurcation | BROKEN | **STILL BROKEN** | `service.go:595` `checkVolumesSecurity` (warn/error + `isSensitiveHostPath:679` + `ValidateHostMountWithAllowlist:663`) vs `compose.go:226` `validateComposeVolumes` blanket reject `strings.HasPrefix(source,"/")` without allowlist |
| Phase-01 C11 / Re-09 LF-01 / Final-parity C11 F-01 | `service.go:92` `env_file` | BROKEN | **STILL BROKEN** | `service.go:92` `rawService` has no `EnvFile` field (only `rawInclude:134` has `EnvFile` for `include:`); `service.go:145` deploy path uses fallback `yaml.Unmarshal` not compose-go loader `parser.go:245` |
| `parser.go:197` / Re-09 C01 | compose-go parser not on deploy hot-path | MISSING/PARTIAL | **STILL BROKEN** | `parser.go:245` `ParseComposeYAML` via `compose-spec/compose-go/v2/loader` exists but `service.go:145` `ParseComposeYAML(content,workingDir,nil)` fallback + `lifecycle.go:264` `DeployComposeStack` never calls wrapper; `parser.go:364` `ParseComposeString` only used by import preview |
| Re-08 C05 / Final-parity C04 | exec air-gap | UNWIRED | **INTENTIONALLY_NOT_EXPOSED** | `beacon/internal/server/container_admin.go:792` `handleContainerExec` + `server.go:463` + `daemon/client.go:2095` healthy but `handlers_docker.go` has 0 exec route (verified `grep exec → 0`) — deliberate allowlist confinement |

---

## 2. Live Chain Inventory (file:line)

### 2.1 Beacon server multiplex — `beacon/internal/server/server.go:408-489`

| Line | Route | Handler | Status |
|---|---|---|---|
| `server.go:456` | `GET /api/admin/containers` | `handleContainerList:29` | HEALTHY |
| `server.go:457` | `GET /api/admin/containers/{id}` | `handleContainerInspect:92` | HEALTHY |
| `server.go:458` | `GET /api/admin/containers/{id}/logs` | `handleContainerLogs:141` | HEALTHY |
| `server.go:459` | `POST /api/admin/containers/{id}/start` | `handleContainerStart:193` | HEALTHY |
| `server.go:460` | `POST /api/admin/containers/{id}/stop` | `handleContainerStop:196` | HEALTHY |
| `server.go:461` | `POST /api/admin/containers/{id}/restart` | `handleContainerRestart:199` | HEALTHY |
| `server.go:462` | `DELETE /api/admin/containers/{id}` | `handleContainerDelete:276` | HEALTHY |
| `server.go:463` | `POST /api/admin/containers/{id}/exec` | `handleContainerExec:792` | HEALTHY but air-gapped at API |
| `server.go:476` | `POST /api/admin/images/prune` | `handleImagePrune:531` | HEALTHY but UNWIRED at `handlers_docker.go` |
| `server.go:478` | `POST /api/admin/containers` | `handleContainerCreate:1776` | **FIXED** (was 404 in phase-01 `server.go:440`) |
| `server.go:479` | `POST /api/admin/networks` | `handleNetworkCreate:1824` | **FIXED** (was `GET`-only `462-463`) |
| `server.go:481` | `POST /api/admin/volumes` | `handleVolumeCreate:1896` | **FIXED** |
| `server.go:483` | `POST /api/admin/volumes/prune` | `handleVolumePrune:1961` | **FIXED** |
| `server.go:459-461` | `POST /api/admin/containers/{id}/pause` | **none** | **STILL BROKEN** — no handler, `grep -n pause server.go` 0 |

### 2.2 Daemon client — `forge/api/internal/daemon/client.go`

| Line | Symbol | Wire |
|---|---|---|
| `client.go:1718` | `AdminContainerList` | `GET /api/admin/containers?all=` |
| `client.go:1756` | `AdminContainerStart` | `POST /api/admin/containers/{id}/start` |
| `client.go:1760` | `AdminContainerStop` | `POST .../stop` |
| `client.go:1764` | `AdminContainerRestart` | `POST .../restart` |
| `client.go:1875` | `AdminContainerPause` | `POST /api/admin/containers/{id}/pause` via `adminContainerAction:1768` → **404** |
| `client.go:1879` | `AdminContainerUnpause` | `POST .../unpause` → **404** |
| `client.go:1883` | `AdminContainerCreate` | `POST /api/admin/containers` (`adminPostJSON`) → **201** |
| `client.go:1845` | `AdminImagePrune` | `POST /api/admin/images/prune` → beacon ready, API dead under `/docker` |
| `client.go:2038` | `AdminNetworkCreate` | `POST /api/admin/networks` → **201** |
| `client.go:2064` | `AdminVolumeCreate` | `POST /api/admin/volumes` → **201** |
| `client.go:2090` | `AdminVolumePrune` | `POST /api/admin/volumes/prune` → **200** |
| `client.go:2095` | `AdminContainerExec` | `POST /api/admin/containers/{id}/exec` → healthy but API never calls |

### 2.3 API HTTP — `forge/api/internal/http/handlers_docker.go`

| Line | Route | Evidence |
|---|---|---|
| `handlers_docker.go:30` | `protected.Group("/docker", adminIPAccess, requireRole("admin"))` | admin only |
| `handlers_docker.go:34` | `POST /containers → dockerCreateContainer:136` | `BodyParser map[string]any → AdminContainerCreate:1883` + `recordAudit container:create` — **FIXED** end-to-end |
| `handlers_docker.go:36` | `POST /containers/:id/operate → dockerOperateContainer:155` | switch `start/stop/restart/pause:177/unpause:179` → `AdminContainerPause/Unpause` — UI→API→Daemon wired, Beacon 404 |
| `handlers_docker.go:63` | `POST /volumes/prune → dockerPruneVolumes:667` | fan-out `AdminVolumePrune:2090` + `recordAudit volume:prune` — **FIXED** |
| `handlers_docker.go:46-52` | images pull/build/push/tag/search/delete | all wired to `AdminImage*` |
| `handlers_docker.go:46-52` | `POST /docker/images/prune` | **MISSING** — only `handlers_portainer.go:40` `POST /portainer/images/prune → AdminImagePrune:1845` exists; `grep -n "POST /docker/images/prune" handlers_docker.go` 0 |

Exec absence verified: `grep -n exec handlers_docker.go` (only file `.../files` ops, no `AdminContainerExec`).

---

## 3. Per-Item Confirmation

### 3.1 `container_admin.go:1776` / `1824` / `1896` / `1961` + `server.go:478-483` vs `handlers_docker.go:136` — FIXED?

**Verdict: FIXED (with lossy nuance).**

- **Phase-01 baseline:** `forge/api/internal/daemon/client.go:1892` `AdminContainerCreate` (`POST /api/admin/containers`), `:2047` `AdminNetworkCreate`, `:2073` `AdminVolumeCreate` hit deterministic 404 because `beacon/internal/server/server.go:440` mux had only `GET/DELETE` for those prefixes. `handlers_docker.go:136` `dockerCreateContainer` pretended to support arbitrary container creation but never reached Docker.
- **Reverification-08 (§1.3) claim:** `POST /api/admin/containers:1776` + `server.go:478`, `POST /api/admin/networks:1824+479`, `POST /api/admin/volumes:1896+481`, `POST /api/admin/volumes/prune:1961+483` now have Beacon handlers — **CONFIRMED**.
- **Live evidence:**
  - `beacon/internal/server/container_admin.go:1776` `handleContainerCreate` validates `name+image required:1798`, builds `container.Config{Image,Cmd,Env,Labels}:1814` → `docker.ContainerCreate:1813` → `201 {id,warnings}`; `logAdminAction:1802` audits.
  - `container_admin.go:1824` `handleNetworkCreate` defaults `driver→bridge:1856`, `docker.NetworkCreate:1859` `{Driver,Labels,Internal}` → `201 {id,warning}`.
  - `container_admin.go:1896` `handleVolumeCreate` → `docker.VolumeCreate:1923` → `201` volume JSON.
  - `container_admin.go:1961` `handleVolumePrune` → `docker.VolumesPrune:1979` (`filters.NewArgs()`) → `200` report.
  - `server.go:478` `POST /api/admin/containers`, `479` `POST /api/admin/networks`, `481` `POST /api/admin/volumes`, `483` `POST /api/admin/volumes/prune` all registered.
  - `handlers_docker.go:136` `dockerCreateContainer` now resolves to real Beacon handler: `c.BodyParser(&body map[string]any)` → `resolveDockerNode` → `daemon.AdminContainerCreate:1883` (`POST /api/admin/containers` via `adminPostJSON`) → `201`.

**Trace T2 (now-healthy, was BROKEN):**

| Hop | File:line | Evidence |
|---|---|---|
| UI | `forge/web/lib/api/docker.ts:113` `createContainer` | `POST /docker/containers?node=` body `CreateContainerRequest{image,name,ports,env,volumes,network,restartPolicy}` |
| API | `handlers_docker.go:136` `dockerCreateContainer` | `AdminContainerCreate` |
| Daemon | `daemon/client.go:1883` | `POST /api/admin/containers` with fresh nonce `resignRequest` |
| Beacon | `container_admin.go:1776` + `server.go:478` | **FIXED** |
| Docker | `container_admin.go:1813` `client.ContainerCreate` | real SDK call |

**Remaining nuance — FIXED but lossy (Re-08 F-05 / Final-parity C02 F-10 reconfirmed):**

- `handleContainerCreate` request struct `container_admin.go:1787-1793` only binds `{name,image,cmd,env,labels}` and calls `docker.ContainerCreate(config, nil, nil, nil, name)` with nil `HostConfig`/`NetworkingConfig`. UI `CreateContainerRequest` (`docker.ts:113`) and `forge/web/components/docker/container-create-modal.tsx:26` supply `ports/volumes/network/restartPolicy` — all silently dropped. Not 404 but silent no-op.
- `handleNetworkCreate` struct `container_admin.go:1835-1839` only `{Name,Driver,Labels,Internal}` — UI `createNetwork` `docker.ts:163` `{name,driver,subnet,nodeId}` `subnet` field never forwarded; `network.CreateOptions` sets only `Driver/Labels/Internal`, no `IPAMConfig`. Subnet silently dropped (Re-08 F-04 / Final-parity C09).

Classification: **FIXED** for wiring debt; **PARTIAL (Wired but lossy)** for spec fidelity. Recommend extending struct to `ports/volumes/network/restartPolicy` via `nat.ParsePortSpec` + `mount.Mount` + `restart.Policy` or removing those UI fields until wired.

---

### 3.2 Pause / Unpause `client.go:1875 → server.go:459` missing — STILL BROKEN

**Verdict: STILL BROKEN (WIRED_BUT_WRONG → 404).**

- `forge/api/internal/daemon/client.go:1875` `AdminContainerPause` → `adminContainerAction(ctx, baseURL, nodeToken, containerID, "pause")` `1768` `POST /api/admin/containers/{id}/pause` (signed retry).
- `forge/api/internal/daemon/client.go:1879` `AdminContainerUnpause` similarly.
- `forge/api/internal/http/handlers_docker.go:176` `case "pause": opErr = AdminContainerPause`, `178` `case "unpause": opErr = AdminContainerUnpause` → API forwards correctly; `forge/web/lib/api/docker.ts:118` `operateContainer(id,"pause")` union includes `pause|unpause`; `forge/web/components/docker/containers-view.tsx:142` Pause button (running→pause) and `148` Unpause.
- **Break at Beacon:** `beacon/internal/server/server.go:459-461` registers only `POST /api/admin/containers/{id}/start|stop|restart` + `DELETE /api/admin/containers/{id}` + `POST .../exec` (`463`). **No** `POST .../pause` or `.../unpause`. `beacon/internal/server/container_admin.go:193` `adminContainerAction` switch handles only `start/stop/restart` (no `pause` case). `grep -n pause beacon/internal/server/server.go` 0 hits; `grep -n handleContainer.*Pause container_admin.go` 0.
- **Impact:** Admin clicks Pause/Unpause → `502 Bad Gateway` (Daemon 404 → API `StatusBadGateway`). Final-parity C03 F-11 and Re-08 F-02 still open, reconfirmed.
- **Recommendation:** Implement `handleContainerPause/Unpause` mirroring `adminContainerAction` with `client.ContainerPause/Unpause` + register `POST /api/admin/containers/{id}/pause|unpause` in `server.go`, or remove `pause|unpause` from `operateContainer` type union and UI buttons. No `kill -s SIGNAL` path either (`beacon/internal/runtime/docker.go:494` `Kill` exists but not wired).

**Trace T5 (pause):**

| Hop | File:line | Evidence |
|---|---|---|
| UI | `forge/web/lib/api/docker.ts:118` + `containers-view.tsx:142` | `operateContainer pause` |
| API | `handlers_docker.go:176` | `AdminContainerPause:1875` |
| Daemon | `daemon/client.go:1875` | `POST /api/admin/containers/{id}/pause` |
| Beacon | `server.go:459-461` | **404 — no route** |
| Docker | — | never reached |

---

### 3.3 Image prune `server.go:476` vs `handlers_docker.go:63` dead — STILL BROKEN (half-wired)

**Verdict: STILL BROKEN at Docker admin surface; Beacon+Daemon healthy; only Portainer surface exposes prune. Classified as `UNWIRED at Docker admin` (if Portainer topology intentional, document; else `BROKEN`).**

- **Beacon ready:** `beacon/internal/server/container_admin.go:531` `handleImagePrune` (`ImagesPrune` + audit `image:prune:563` → `report`); `beacon/internal/server/server.go:476` `POST /api/admin/images/prune` registered.
- **Daemon ready:** `forge/api/internal/daemon/client.go:1845` `AdminImagePrune` (`POST /api/admin/images/prune` via `adminPostJSON`).
- **API gap:** `forge/api/internal/http/handlers_docker.go:63` only `docker.Post("/volumes/prune", ... , dockerPruneVolumes)` — `grep -n "images/prune" handlers_docker.go` 0. No `POST /docker/images/prune` handler. `forge/web/lib/api/docker.ts:147` has no `pruneImages` export (only `pruneVolumes:185`).
- **Contrast — Portainer surface truth:** `forge/api/internal/http/handlers_portainer.go:40` `images.Post("/prune", ..., pruneImages)` → `handlers_portainer.go:557` `pruneImages → AdminImagePrune:1845` — so image prune is reachable only under `/portainer/images/prune`, not `/docker/images/prune`.
- **Phase-01 crossed wires half-fixed:** Re-08 §1.3 notes volume prune fixed both ends; image prune remains dead code under `/docker`. Final-parity C07 F-13 reconfirmed.
- **Impact:** `POST /docker/images/prune` 404; operator cannot prune images from Docker admin UI (requires Portainer prefix).
- **Recommendation:** Either add `docker.Post("/images/prune", ..., dockerPruneImages → AdminImagePrune)` mirroring `dockerPruneVolumes`, or delete `handleImagePrune`/`server.go:476` to avoid dead code and document that Docker admin image prune intentionally lives under `/portainer`.

---

### 3.4 `beacon/compose.go:213` `shortFormHostPort` `["80"]` bypass + `314?` / `:214` `return ""` — STILL BROKEN

**Verdict: STILL BROKEN (P1, OPERATOR_VISIBLE, policy bypass).**

- **Live code:**
  ```go
  // beacon/internal/server/compose.go:212-224
  func shortFormHostPort(entry string) string {
    parts := strings.Split(entry, ":")
    switch len(parts) {
    case 2: return parts[0]
    case 3: return parts[1]
    default: return "" // ← handles "80" (len1) and "127.0.0.1::80" len? → "" 
    }
  }
  // beacon/internal/server/compose.go:181-209
  func validateComposePorts(service string, value any) error {
    // ...
    case string: published = shortFormHostPort(v)
    // ...
    if published == "" { continue } // ← bypass
    port, _ := strconv.Atoi(strings.TrimSpace(published))
    if port < 1024 { return fmt.Errorf("publishing privileged host port %d ...") }
  }
  ```
- **Bypass proof:**
  - `"80"` → `strings.Split("80",":") = ["80"]` len1 → `""` → `published==""` → `continue` → **no error** → attacker publishes host `80` (Compose interprets `80` as `80:80` or random depending on engine).
  - `"127.0.0.1::80"` → `["127.0.0.1","","80"]` len3 → `parts[1]=""` → `""` → bypass.
  - `"8080:80"` → `["8080","80"]` len2 → `"8080"` → correctly checked.
  - Long-form `published: 80` (`map[string]any` branch `fmt.Sprint(raw)` at `193`) is correctly checked; bypass is short-form specific.
  - API `forge/api/internal/services/compose/service.go:427` `ValidateComposeSecurity` has **no** privileged-port check at all — only beacon does, and it skips.
- **Cross-refs:** Re-08 F-01 / Re-09 LF-06 / Phase-06 REF-P6-COMP-03 P1 — all reconfirmed still open (`git log` shows no patch to `compose.go:212-224`).
- **Impact:** Host-level port hijacking on shared nodes (bind 80/22/443 without error).
- **Fix (no code change per audit):** `case 1: return parts[0]` after stripping `/tcp`/`/udp` suffix + `strings.TrimSuffix(entry, "/tcp")` or reuse `docker/go-connections/nat.ParsePortSpec`; also add API-side parity check.

**Trace T7 (privileged port bypass):**

| Hop | File:line | Evidence |
|---|---|---|
| UI | `forge/web/lib/api/compose.ts:53` `createComposeStack` with `ports: ["80"]` | YAML supplied verbatim |
| API | `service.go:427` `ValidateComposeSecurity` | no port check |
| Daemon | `daemon/compose.go:48` `ComposeDeploy` | `POST /compose/deploy` |
| Beacon | `compose.go:181` `validateComposePorts` → `shortFormHostPort:214` | `published==""` → `continue` bypass |
| Docker | `compose.go:395` `docker compose up -d` | publishes privileged port |

---

### 3.5 `lifecycle.go:280` per-replica ceilings — STILL BROKEN

**Verdict: STILL BROKEN (WIRED_BUT_WRONG, P1 quota bypass).**

- **Live check:**
  ```go
  // forge/api/internal/services/compose/lifecycle.go:280-282
  if req.MemoryMB > MaxUserStackMemoryMB || req.DiskMB > MaxUserStackDiskMB || req.CPUShares > MaxUserStackCPUShares {
    return nil, fmt.Errorf("%w: memory<=%dMB disk<=%dMB cpuShares<=%d",
      ErrResourceLimitExceeded, MaxUserStackMemoryMB, MaxUserStackDiskMB, MaxUserStackCPUShares)
  }
  // ceilings: MaxUserStackMemoryMB=65536, Disk=512000, CPUShares=8192 at lifecycle.go:150-152
  ```
  Only top-level `DeployComposeRequest{MemoryMB,CPUShares,DiskMB}` checked. Never computes `sum(per-service deploy.resources.limits * replicas)`.

- **Parser vs placement divergence:**
  - `forge/api/internal/services/compose/parser.go:913` `normalizeResources` maps only `cpu` (`NanoCPUs`) + `memory` (`MemoryBytes`) into `map[string]string` — drops `Pids/BlkioWeight/Gpus/Ulimits/ShmSize/DeviceRequests`.
  - `service.go:846` stores `Deploy.Replicas` but never consumed in deployment; `lifecycle.go:293` `scheduler.PlaceServer{CPU: CPUShares, MemoryMB, DiskMB}` uses top-level only, not YAML `deploy.resources`.
  - `beacon/internal/server/compose.go:395` bypasses `beacon/internal/runtime/docker.go:783` `buildResources` isolation (game path) entirely — `docker compose up` honors YAML limits verbatim but Forge placement not fed.

- **Example bypass:**
  ```yaml
  services:
    api:
      image: api:latest
      deploy:
        resources: { limits: {memory: 8G, cpus: '4.0'} }
        replicas: 10
  ```
  Top-level `MemoryMB=512` passes ceiling `65536`, but host needs `80G` (10×8G) → over-commit / OOM hidden.

- **Cross-refs:** Re-08 F-06 / Re-09 LF-02 / Final-parity C14 F-03 — all reconfirmed.

---

### 3.6 `service.go:594` vs `compose.go:226` volume bifurcation (trifurcation) — STILL BROKEN

**Verdict: STILL BROKEN (P0, bifurcated policy → `200 valid` → `400` on deploy).**

- **API side (`service.go:595` `checkVolumesSecurity`):**
  ```go
  // service.go:595-657
  if strings.HasPrefix(lower, "/var/run/docker.sock") → error
  if strings.HasPrefix(lower, "/proc"|"/sys") → error
  if strings.HasPrefix(source, "/") && isSensitiveHostPath(source) { // sensitive: "/", "/root", "/etc", "/home" at 679
    severity := "warning"; if source=="/etc" or "/" → "error" else "warning" + slog.Warn
  }
  // service.go:663 ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts) — requires admin + allowlist entry
  ```
  Allows `/data` as warning → `ValidateCompose:240` still `Valid:true`.

- **Beacon compose side (`compose.go:226` `validateComposeVolumes`):**
  ```go
  func validateComposeVolumes(service string, value any) error {
    // long-form type:bind → error
    // short-form source:hasSource && (strings.HasPrefix(source,"/") || isPathTraversal(source))
    //   → error "host bind mount source %q is not allowed"
  }
  ```
  **Without allowlist param.** Any absolute host bind (`/data`, `/opt/appdata`) unconditionally rejected. No `allowedMounts` plumbing.

- **Game path side (`beacon/internal/server/mounts.go:64` `allowedMountSource`):** `EvalSymlinks` + `Rel` within `allowedMounts` from `SetAllowedMounts:129` — empty allowlist denies all custom mounts; but compose path never consults it.

- **Result — trifurcation (Re-09 LF-03 / Final-parity C15 F-04 reconfirmed):**
  1. API warning vs beacon error mismatch: `POST /compose/validate` can return `valid:true` (warning) then `POST /compose/{id}/deploy` (`daemon/compose.go:48` → `beacon/compose.go:358` `validateComposePolicy:358`) fails `400 compose policy violation: host bind mount source "/data" not allowed`.
  2. Allowlist honoured on game path (`mounts.go:64`) but ignored on compose path (`compose.go:226` has no `allowed` param) — extending `allowed_mounts=/opt/appdata` enables game servers but not compose stacks.
  3. Long-form `type: bind/tmpfs/volume` details (`selinux`, `propagation`, `nocopy/subpath`, `tmpfs size/mode`, `image.subpath`) discarded at `parser.go:880` coercion `source:target[:ro]` only.

- **Fix:** Plumb `allowedMounts+isAdmin` through `composeDeployRequest` into `validateComposeVolumes(..., allowed, isAdmin)` or make API reject all absolute mounts (`error`) matching beacon until plumbing exists; add `?keepVolumes` for delete vs hard-coded `down -v`.

---

### 3.7 `service.go:92` `env_file` — STILL BROKEN

**Verdict: STILL BROKEN (P0, silent ignore → runtime misconfiguration).**

- **Live structs:**
  ```go
  // forge/api/internal/services/compose/service.go:92
  type rawService struct {
    Image       string                 `yaml:"image,omitempty"`
    Build       map[string]interface{} `yaml:"build,omitempty"`
    Ports       []interface{}          `yaml:"ports,omitempty"`
    Environment interface{}            `yaml:"environment,omitempty"`
    Volumes     []interface{}          `yaml:"volumes,omitempty"`
    // ← no EnvFile field
  }
  // service.go:134
  type rawInclude struct {
    Path       interface{} `yaml:"path,omitempty"`
    ProjectDir string      `yaml:"project_directory,omitempty"`
    EnvFile    interface{} `yaml:"env_file,omitempty"` // only for include:, not service env_file
  }
  ```
  `env_file: ./app.env` arrives, is unmarshalled into nothing, stripped from `ParsedCompose` summary, not written to beacon. `ValidateComposeSecurity:427` has no branch for it → no warning/error.

- **Beacon writes only `EnvVars` map:**
  - `beacon/internal/server/compose.go:380` `encodeComposeEnv:264` writes `Req.EnvVars` to `.env` (stripping `\n\r`, key regex `[A-Za-z0-9_]`) only; never mounts host `env_file` sources. `validateComposePolicy:77` omits `env_file` case.

- **Deploy path fallback vs compose-go:**
  - `service.go:145` `ParseComposeYAML(content,workingDir,nil)` uses `yaml.Unmarshal` fallback structs.
  - `parser.go:245` `ParseComposeYAML(yamlContent)` via `compose-spec/compose-go/v2/loader` **does** understand `types.ServiceConfig.EnvFiles`, but `service.go:145`/`lifecycle.go:264` never call wrapper. Import preview `parser.go:364` `ParseComposeString` does use loader, so import shows valid while deploy silently omits vars.

- **Repro:**
  ```yaml
  services:
    web:
      image: nginx:alpine
      env_file: [./frontend.env]
      environment: [PORT=3000]
  ```
  Expected merge from `frontend.env`; Forge: only `PORT=3000` + `EnvVars` map → app crashes (DB passwords missing).

- **Cross-refs:** REF-P6-COMP-02 P0 / Final-parity RT-11 / Re-09 LF-01 — all still open, no remediation detected.

- **Fix:** Either route `DeployComposeStack` through loader (`WithEnvFiles`) and copy referenced `env_file` hosts into `stackDir` before `handleComposeDeploy`, or fail-fast: reject `env_file` at `ValidateCompose:240` with `severity:error`.

---

### 3.8 `parser.go:197` — STILL BROKEN (wrapper exists but not on hot path)

**Verdict: STILL BROKEN (MISSING on deploy path; import vs deploy divergence).**

- **Old phase-01 citation `parser.go:197` mapped to `ParseComposeWithMultipleFiles:195` / `CheckComposeVersionCompatibility:213` area — indicating the compose-go loader wrapper existed but was unused on deploy.**
- **Live:**
  - `forge/api/internal/services/compose/parser.go:245` `ParseComposeYAML(yamlContent)` and `195` `ParseComposeWithMultipleFiles`, `364` `ParseComposeString` via `loader.LoadWithContext` all exist and support `EnvFiles`, `Include` expansion, `WithDotEnv`, etc.
  - **But** `forge/api/internal/services/compose/service.go:145` `ParseComposeYAML(content,workingDir,nil)` and `service.go:333` / `service.go:338` and `lifecycle.go:264` `DeployComposeStack` → `ValidateCompose` gate use **fallback** `yaml.Unmarshal` into `rawService`/`rawCompose` (no `EnvFile`), not the wrapper. `gitops.go:196` `readComposeFromDir` single-file read similarly bypasses.
  - `beacon/internal/server/compose.go:374` only writes single `compose.yaml` + `.env` into `compose/<id>`; `./web/Dockerfile` / sibling files never transferred. `build.context` not shipped (Final-parity C18 MISSING).

- **Result:** Multi-file `COMPOSE_FILE`, remote `include:` (`options.go:277` `RemoteLoader`), and `env_file` all work in compose-go import preview but fail or are ignored at deploy → UX divergence `valid` at import vs `409` / crash at deploy.

---

### 3.9 Exec air-gap — INTENTIONALLY_NOT_EXPOSED

**Verdict: INTENTIONALLY_NOT_EXPOSED (not a regression; matches MASTER `INTENTIONALLY_NOT_EXPOSED`).**

- **Beacon healthy:** `beacon/internal/server/container_admin.go:792` `handleContainerExec` — infra-admin only `800`, allowlist 20 read-only cmds `832` (`ls/pwd/ps/top/df/.../ping` + extended `lscpu/lsblk/lsof/ss/ip/...`), `path.Base` check `845`, `path` traversal arg check `851`, rejects managed containers `865`, `ContainerExecCreate:874` + `ContainerExecAttach:879` + `stdcopy.StdCopy:887`.
- **Daemon healthy:** `forge/api/internal/daemon/client.go:2095` `AdminContainerExec` `POST /api/admin/containers/:id/exec`.
- **API deliberately not exposing:** `forge/api/internal/http/handlers_docker.go` **no** `exec` route (`grep exec → 0` for files ops only); `forge/web/lib/api/docker.ts` **no** `exec` export.
- **Contrast:** `handlers_user_containers.go` also omits exec. No false completion — panel cannot become arbitrary RCE.
- **When to expose:** Gate via `requireAdminScope` + audit + allowlist disclosure if ever wired.

---

## 4. Verdict Matrix (requested taxonomy)

| Item | Requested Line | Live Location | Verdict | Evidence |
|---|---|---|---|---|
| Container create | `container_admin.go:1776` + `server.go:478` vs `handlers_docker.go:136` | `container_admin.go:1776` `handleContainerCreate` + `server.go:478` + `handlers_docker.go:136` + `daemon/client.go:1883` | **FIXED** (lossy) | See §3.1 |
| Network create | `container_admin.go:1824` + `server.go:479` | `1824` + `479` + `daemon/client.go:2038` | **FIXED** (lossy: subnet dropped) | §3.1 |
| Volume create | `container_admin.go:1896` + `server.go:481` | `1896` + `481` + `daemon/client.go:2064` | **FIXED** | §3.1 |
| Volume prune | `container_admin.go:1961` + `server.go:483` vs `handlers_docker.go:63` | `1961` + `483` + `handlers_docker.go:63` + `daemon/client.go:2090` | **FIXED** | §3.1 |
| Pause/unpause | `client.go:1875 → server.go:459` missing | `daemon/client.go:1875/1879` vs `server.go:459-461` (start/stop/restart only) | **STILL BROKEN** | §3.2 |
| Image prune dead | `server.go:476` vs `handlers_docker.go:63` | `server.go:476` + `daemon/client.go:1845` vs `handlers_docker.go:63` (volumes only) + `handlers_portainer.go:40/557` | **STILL BROKEN** at `/docker` / **UNWIRED** (Portainer topology) | §3.3 |
| `shortFormHostPort` `["80"]` bypass | `beacon/compose.go:213` + `214 return ""` | `compose.go:214` `default: return ""` + `compose.go:190` + `198` | **STILL BROKEN** | §3.4 |
| Per-replica ceilings | `lifecycle.go:280` | `lifecycle.go:280` `if req.MemoryMB > MaxUserStack...` only | **STILL BROKEN** | §3.5 |
| Volume bifurcation | `service.go:594` vs `compose.go:226` | `service.go:595` `checkVolumesSecurity` vs `compose.go:226` `validateComposeVolumes` | **STILL BROKEN** | §3.6 |
| `env_file` | `service.go:92` | `service.go:92` `rawService` no `EnvFile` | **STILL BROKEN** | §3.7 |
| `parser.go:197` (compose-go wrapper) | `parser.go:197` area | `parser.go:245` wrapper vs `service.go:145` fallback | **STILL BROKEN** (not on deploy hot-path) | §3.8 |
| Exec | `container_admin.go:792` / `server.go:463` / `daemon/client.go:2095` vs `handlers_docker.go` absent | `container_admin.go:792` + `server.go:463` + `daemon/client.go:2095` vs `handlers_docker.go` 0 exec | **INTENTIONALLY_NOT_EXPOSED** | §3.9 |

**Counts:** FIXED 4 (2 lossy), STILL BROKEN 7, INTENTIONALLY_NOT_EXPOSED 1 — total 12 items.

---

## 5. File:Line Index (key citations, current checkout)

- `beacon/internal/server/container_admin.go:29` `handleContainerList`; `92` `handleContainerInspect`; `141` `handleContainerLogs`; `193` `adminContainerAction` (start/stop/restart only); `276` `handleContainerDelete`; `365` `handleImageList`; `412` `handleImagePull`; `471` `handleImageDelete`; `531` `handleImagePrune`; `570` `handleNetworkList`; `657/705/752` volume list/inspect/usage; `792` `handleContainerExec` (allowlist `832`); `909` `handleContainerTop`; `995` `handleContainerStats`; `1230` `handleImageBuild`; `1414/1484/1559/1703` container files; `1776` `handleContainerCreate` (struct `1787-1793` narrow); `1824` `handleNetworkCreate` (`1835-1839` no IPAM); `1896` `handleVolumeCreate`; `1961` `handleVolumePrune`
- `beacon/internal/server/server.go:456-483` multiplex; `459-461` start/stop/restart only (no pause); `463` exec mounted but API air-gapped; `476` `POST /api/admin/images/prune`; `478` `POST /api/admin/containers`; `479` `POST /api/admin/networks`; `481` `POST /api/admin/volumes`; `483` `POST /api/admin/volumes/prune`
- `beacon/internal/server/compose.go:59` `validStackID`; `77` `validateComposePolicy`; `181` `validateComposePorts`; `212` `shortFormHostPort` (`214` `default: return ""` bypass); `226` `validateComposeVolumes` (no allowlist); `264` `encodeComposeEnv`; `344` `handleComposeDeploy` (`395` `docker compose up -d`); `506` `handleComposeRestart` native but `lifecycle.go:781` `RestartStack` uses stop+start; `550` `handleComposeDelete` `578` hard-coded `down -v`
- `forge/api/internal/daemon/client.go:1718` `AdminContainerList`; `1756/1760/1764` start/stop/restart; `1875` `AdminContainerPause`; `1879` `AdminContainerUnpause`; `1883` `AdminContainerCreate`; `1845` `AdminImagePrune`; `2038` `AdminNetworkCreate`; `2064` `AdminVolumeCreate`; `2090` `AdminVolumePrune`; `2095` `AdminContainerExec`
- `forge/api/internal/http/handlers_docker.go:30` `protected.Group("/docker")`; `33` `GET /containers`; `34` `POST /containers → dockerCreateContainer:136`; `36` `POST /containers/:id/operate` (`176` pause, `178` unpause); `63` `POST /volumes/prune`; `280` `dockerPullImage` (`validateContainerImageReference:14` 512 limit); `667` `dockerPruneVolumes`
- `forge/api/internal/http/handlers_portainer.go:40/557` `pruneImages` (`POST /portainer/images/prune` → `AdminImagePrune`)
- `forge/api/internal/services/compose/service.go:14` `MaxComposeYAMLBytes`; `92` `rawService` (no `EnvFile`); `134` `rawInclude.EnvFile` (only include); `145` `ParseComposeYAML` fallback; `356` `interpolateEnv` (`composeVarRe:346`); `427` `ValidateComposeSecurity`; `595` `checkVolumesSecurity`; `663` `ValidateHostMountWithAllowlist`; `679` `sensitiveHostPaths`
- `forge/api/internal/services/compose/parser.go:195` `ParseComposeWithMultipleFiles`; `245` `ParseComposeYAML` loader wrapper; `364` `ParseComposeString` (import only); `913` `normalizeResources` (cpu+memory only)
- `forge/api/internal/services/compose/lifecycle.go:139-152` ceilings (`MaxUserStackMemoryMB 65536/Disk500G/CPUShares8192`); `249` `DeployComposeStack`; `264` security gate; `280` ceiling check (top-level only); `293` `PlaceServer`; `781` `RestartStack` stop+start; `188` `WaitForHealthy` 5 s×2 m
- `forge/api/internal/daemon/compose.go:48` `ComposeDeploy`; `73-86` stop/start/restart/delete; `110` status; `131` logs

---

*Generated by subagent 05 (phase-02, parallel 05/10). No product files modified. All 10 phase-02 subagents run in parallel — this report is independently verifiable via `grep -n` on listed files.*
