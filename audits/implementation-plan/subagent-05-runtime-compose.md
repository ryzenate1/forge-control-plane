# Subagent 05 — Runtime / Compose — Fix Broken Chains + Spec Fidelity Without Breaking Existing Stacks

> **Track:** Runtime / Compose — Reverification-08/09 + Final Parity RT-01..RT-17
> **Auditor synthesis:** subagent-05 (plan, parallel 05/10)
> **Date:** 2026-08-24
> **Mode:** DESIGN-ONLY — no product code modified. All citations `file:line` verified on current checkout (`f015a22` / reverification 2026-08-24).
> **Predecessors read:** `audits/reverification/subagent-08-containers.md` (§1-7, F-01..F-06, T1-T8), `audits/reverification/subagent-09-compose-spec.md` (§1-7, LF-01..LF-07, C01-C18), `audits/final-parity/subagent-04-runtime-compose.md` (§1-6, C01-C18, T1-T3, F-01..F-05), `audits/phase-01/subagent-03-runtime-compose.md`, `audits/phase-06/subagent-10-compose-builds.md`, `audits/phase-06/subagent-07-komodo-stacks.md`, `MASTER_FINDING_INDEX.md` (REF-APP-RT-*, REF-P6-COMP-01..04), `FINAL_REFERENCE_ECOSYSTEM_REPORT.md`
> **Reference inventory:** `reference/app-platforms/portainer/api/docker/client/client.go:28` + `portainer/api/docker/container.go:55` `Recreate`, `reference/app-platforms/1panel/agent/app/api/v2/container.go:224/459/612/915`, `reference/app-platforms/docker-compose/pkg/compose/{loader.go,dependencies.go:78,create.go:592/635,envresolver.go,convergence.go:88}`, `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:414` `ComposeUp` 6-phase, `reference/app-platforms/uncloud/pkg/client/service.go:24` `RunService`

---

## 0. Executive Summary — What This Plan Fixes And What It Preserves

### 0.1 Already FIXED — keep wired, add regression guards (T2-T4)

Reverification §6 confirms 4 Beacon routes that were deterministic 404 in Phase-01 are now Healthy:

| Chain | Beacon handler | Beacon mount | Daemon client | API handler | Status |
|---|---|---|---|---|---|
| `POST /api/admin/containers` create | `beacon/internal/server/container_admin.go:1776` `handleContainerCreate` | `beacon/internal/server/server.go:478` | `forge/api/internal/daemon/client.go:1883` `AdminContainerCreate` | `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` | **FIXED (lossy — see §2.5)** |
| `POST /api/admin/networks` create | `container_admin.go:1824` `handleNetworkCreate` | `server.go:479` | `daemon/client.go:2038` `AdminNetworkCreate` | `handlers_docker.go:377` `dockerCreateNetwork` | **FIXED (lossy — HostConfig nil / subnet dropped)** |
| `POST /api/admin/volumes` create | `container_admin.go:1896` `handleVolumeCreate` | `server.go:481` | `daemon/client.go:2064` `AdminVolumeCreate` | `handlers_docker.go:454` `dockerCreateVolume` | **FIXED clean** |
| `POST /api/admin/volumes/prune` | `container_admin.go:1961` `handleVolumePrune` | `server.go:483` | `daemon/client.go:2090` `AdminVolumePrune` | `handlers_docker.go:667` `dockerPruneVolumes` | **FIXED clean** |

Additional FIXED at HTTP layer: `POST /compose/:id/restart` now mounted at `forge/api/internal/http/handlers_compose.go:470` (was missing in Phase-01). See §3.4 for wiring gap remaining below it.

Inventory parity vs 1Panel flat list/no-concealment: `C01` COMPLETE (stricter — `container_admin.go:29` filters `modern-game-panel.server_id` for non-infra), `C08/C09` COMPLETE, `C06/C07` PARTIAL (polling vs WS streaming — deferred).

**This plan's Invariant #1:** No edit may re-introduce 404 for T2-T4. Every change below includes a regression test that would have caught the original 404. See §5.

### 0.2 Still BROKEN / WIRED_BUT_WRONG — this plan fixes (P0-P1)

Carried from reverification-08 F-01..F-06 + reverification-09 LF-01..LF-07 + final-parity RT-11..RT-15 + MASTER P0s:

| ID | Title | File:line (root cause) | Severity | User-visible failure mode |
|---|---|---|---|---|
| **F-01 / LF-06 / REF-P6-COMP-03 / C17** | `shortFormHostPort` privileged-port bypass for `ports: ["80"]` and `"127.0.0.1::80"` | `beacon/internal/server/compose.go:214` `default → ""` then `validateComposePorts:198` `if published=="" continue` skips `<1024` check; API `service.go:427` `ValidateComposeSecurity` has zero port check | **P1** | Host port hijack on shared nodes (`80/22/443` bind without error) |
| **F-02 / C04** | `pause`/`unpause` UI→API→Daemon wired, Beacon handler missing → 404 | `forge/api/internal/daemon/client.go:1875` `AdminContainerPause` → `POST /api/admin/containers/{id}/pause` but `beacon/internal/server/server.go:459-461` only mounts `start/stop/restart`; `container_admin.go` has no `handleContainerPause` (grep 0) | **P2 USER_VISIBLE** | Admins see Pause/Unpause buttons (`containers-view.tsx:142,148`) but operation always `502 Bad Gateway` (Daemon 404 → `handlers_docker.go:176` `StatusBadGateway`) |
| **F-03 / C09** | Image prune **UNWIRED at Docker admin** — dead Beacon code | Beacon `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` + Daemon `client.go:1845` `AdminImagePrune` healthy; but `handlers_docker.go:63` never mounts `POST /docker/images/prune` (only `POST /volumes/prune`); only `handlers_portainer.go:40/557` exposes it under `/portainer` | **P3 SILENT** | Volume prune works, image prune dead under Docker tab; future `pruneImages` UI would 404 |
| **F-04 / C12 / LF-03** | `down -v` hard-coded + volume policy trifurcation + `-v` data-loss | `compose.go:578` always `down -v` vs upstream `down.go:112` opt-in; API `service.go:595` `checkVolumesSecurity` (warn `/data`) vs `compose.go:226` `validateComposeVolumes` strictly blocks all absolute binds (no allowlist) vs game path `mounts.go:64` `allowedMountSource` (EvalSymlinks+Rel allowlist); result `200 valid` at `POST /compose/validate` → `400 policy violation` at deploy; allowlist permits portal servers but not compose stacks; `-v` wipes `pgdata:/var/lib/postgresql/data` | **P1 / P0 bifurcation** | `valid→400` confusion; `allowed_mounts=/opt/appdata` enables game but not compose; inadvertent DB wipe on `DELETE /compose/:id` |
| **F-05 / C11 / C02 lossy** | Container/Network create FIXED but lossy — silent field drops | `handleContainerCreate:1787-1813` only binds `{name,image,cmd,env,labels}` then `client.ContainerCreate(config,nil,nil,nil,name)` with nil HostConfig/NetworkingConfig — UI `container-create-modal.tsx:26` / `docker.ts:113` `CreateContainerRequest{ports,volumes,network,restartPolicy}` discarded; `handleNetworkCreate:1835-1859` ignores `subnet` → `network.CreateOptions{Driver,Labels,Internal}` no `IPAMConfig` | **P2 USER_VISIBLE** | Container starts with no ports/volumes despite UI success; network gets default bridge IPAM despite CIDR form |
| **LF-01 / C11 / REF-P6-COMP-02 / RT-11** | `env_file` silently dropped — runtime misconfiguration | `service.go:92` `rawService` has no `EnvFile` field (only `rawInclude.EnvFile:134` for `include:`), deploy path `service.go:145` / `lifecycle.go:264` uses fallback parser not `parser.go:245` compose-go loader; `validateComposePolicy:77` has no `env_file` case; `compose.go:380` only writes `Req.EnvVars` via `encodeComposeEnv:264` | **P0 BROKEN** | `env_file: ./app.env` (DB passwords) never mounted → `up -d` succeeds but app crashes; import via `parser.go:364` `ParseComposeString` (compose-go) shows valid while deploy silently omits vars |
| **LF-02 / C09 / C14 / RT-14** | `deploy.resources.limits`/`reservations` + `replicas` not multiplied — placement over-commit | `parser.go:913` `normalizeResources` only `cpu`/`memory` (drops `Pids/Blkio/Gpus/Ulimits/ShmSize`); `lifecycle.go:280` ceiling checks `req.MemoryMB/DiskMB/CPUShares` only (ceilings `MaxUserStackMemoryMB 65536/Disk 512000/CPUShares 8192` at `lifecycle.go:150-152`), placement `scheduler.PlaceServer:293` uses top-level `MemoryMB/CPUShares/DiskMB` not YAML `deploy.resources`; `compose.go:395` bypasses `docker.go:783` `buildResources` / `827` `buildHostConfigWithSettings` (CapDrop/ReadonlyRootfs/etc.); beacon `handleComposeDeploy:344` single `up -d` shellout | **P1** | `replicas:10 × memory:8G` passes 64G ceiling but needs 80G → host OOM; `memory:100TiB` or blocked `PidsLimit` never enforced; dual-tier isolation undocumented |
| **LF-04 / C18 / C14** | Build context not shipped for compose `build:` → opaque `409` | `service.go:92` `rawService.Build map[string]interface{}` → `normalizeBuild:787` stored as `BuildSummary` string; `lifecycle.go` never ships source tree; `compose.go:374` only writes `compose.yaml`+`.env`; `build/service.go:530` `executeRemoteBuild` clones for generic builds, not wired to `build.context`; `daemon/compose.go:13` `ComposeDeployRequest` lacks context tarball fields | **P2 MISSING** | `build: {context: ./web}` always fails `failed to read dockerfile: open .../compose/<id>/web/Dockerfile: no such file` → `ComposeOperationResponse{Error,Output}` |
| **LF-05 / C08 / RT-13 / C13** | Restart policy divergence + `RestartStack` lifecycle race | `service.go:516` warns `restart:always` only; `compose.go:92` `validateComposePolicy` has no `restart` case (allow-all); `lifecycle.go:781` `RestartStack` is `StopStack+StartStack` (`handlers_compose.go:470` calls it) vs beacon `compose.go:506` native `docker compose restart` (more faithful) + unused `daemon.ComposeRestart:81` (`daemon/compose.go:81` `ComposeRestart`); `on-failure:3` `MaximumRetryCount` lost; Swarm `deploy.restart_policy` never mapped | **P2 WIRED_BUT_WRONG** | `stop` races host auto-restart for `always`; `StartStack` flaps; stop+start loses `--no-deps` hook filtering vs native `restart` |
| **LF-07 / REF-P6-COMP-04** | `DeployFromGit` fresh-ID bug (also compose_service) | `forge/api/internal/services/compose/gitops.go:351/384` allocates fresh `stackID := g.compose.createStackID()` then `UpdateComposeStack` on non-existent row (should be `CreateComposeStack`); `lifecycle.go:390` `store.CreateComposeStack` is correct path | **P1** | Generic error `"create compose stack record"`; reservation leaked (`Cancelled`); retry double-creates |
| **C15 / LF-03 trifurcation** | Volume allowlist bifurcation (same root as F-04) | `service.go:663` `ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts)` (admin+allowlist gate) exists but compose path never plumbs `allowedMounts`/`isAdmin` into `composeDeployRequest` | **P0** | Duplicate but load-bearing: fix is unified predicate (§3.3) |
| **C15-C16 / §2.1** | Multi-runtime phantom LXC/KVM — silent Docker fallback | API `multiruntime.go:45` `getRuntimeForTarget` falls back to `defaultRuntime` (Docker); `forge/api/cmd/api/main.go:393` registers 6 providers (`docker` default + k8s/firecracker/podman/containerd/lxc/kvm) into `MultiRuntimeAdapter`; capabilities union advertises LXC/KVM; `beacon/internal/runtime/factory.go:16` `Factory.CreateRuntime` supports docker/containerd/podman/firecracker/k8s but not `ProviderLXC/KVM`; `server.go:735` `create` (`POST /servers`) drops `Provider` field entirely → every workload uses `s.runtime.Create` (node-configured runtime, almost always docker) | **P2 WIRED_BUT_WRONG** | Scheduler places on `lxc/kvm`; Beacon silently executes as Docker — placement honesty & safety claims false |
| **Exec (§2.1)** | `exec` air-gap — verify intentionally not exposed | `beacon/container_admin.go:792` `handleContainerExec` (infra-admin allowlist 20 read-only, `path.Base` check, managed-block, `StdCopy`) + `daemon/client.go:2095` + `server.go:463` healthy; `handlers_docker.go` deliberately has zero exec route; `forge/web/lib/api/docker.ts` has no exec export (grep 0) | **INTENTIONAL** | Not a regression — but must be documented as `INTENTIONALLY_NOT_EXPOSED` with audit disclosure; do NOT wire unless gated |

> **Scope guardrails for this plan:** No schema break for existing `compose_stacks` rows; no change to `mgp-*` single-container isolation tier (`docker.go:827` `buildHostConfigWithSettings` stays); no adoption of Rancher fleet/K8s control plane; no `docker.sock` mount; no arbitrary exec terminal.

---

## 1. Design Principles — Fidelity Without Breaking Existing Stacks

### 1.1 Backward-compat contract

- **Existing compose stacks continue to start/stop/status/logs without re-deployment.** New validation only applies on the `DeployComposeStack`/`UpdateComposeStack`/`DeployFromGit`/`RedeployFromGit` paths (`lifecycle.go:249`, `476`, `gitops.go:384`). Already-deployed `compose.yaml` on disk (`composeStack.dirForID:283` `<dataDir>/../compose/<stackID>/compose.yaml`) is never re-validated in place. Operators must explicitly redeploy to opt into new policy.
- **No automatic `-v` wipe change for stacks deployed before this release.** `DeleteComposeStack:570` hard-coded `down -v` is already destructive. The new `?volumes=true` flag (see §3.4) defaults to `false` for fresh deletes; old UI `DELETE /compose/:id` without query param gets `false` (safe). A one-time migration note warns operators whose volumes were previously wiped and offers a recovery audit (`compose_stacks.updated_at < <release>`).
- **`env_file:` fail-fast is opt-in first, hard-error second.** Phase 1: API `POST /compose/validate` returns `severity:error` but `DeployComposeStack` can still be called with `X-Allow-Unsupported-Env-File: true` header for 30 days. Phase 2: hard reject. This avoids breaking the (broken) deployments that accidentally rely on `env_file` being dropped (e.g., they duplicated vars into `environment:`).
- **Port policy grandfather:** If an existing stack already published a privileged host port (`80`) before this fix, `GetStackStatus` will not retroactively fail. Only new `up -d` is blocked. A background scan (`ListComposeStacksForReconciliation:416`) logs a warning for such stacks but does not stop them.

### 1.2 Single predicate, no trifurcation

The volume/host-mount bug is a three-way split: API (`service.go:595` `checkVolumesSecurity`) warns, Beacon compose (`compose.go:226` `validateComposeVolumes`) hard-rejects with no allowlist, game path (`mounts.go:64` `allowedMountSource`) uses EvalSymlinks+Rel within `allowedMounts`. Fix: one shared predicate `ValidateHostMountWithAllowlist` (`service.go:663`) is plumbed through `ComposeDeployRequest` (`daemon/compose.go:13`) into Beacon `validateComposeVolumesWithAllowlist` so `POST /compose/validate` and `POST /compose/deploy` agree for the same `allowedMounts` + `isAdmin` snapshot. See §3.3.

### 1.3 Minimal new state

Prefer deriving limits from `compose.yaml` on placement rather than persisting a `total_memory_mb` column. Where persistence is needed (scale generation, spec diff), reuse existing `compose_stacks.compose_hash` + `git_previous_compose_yaml` instead of new columns, except for `scale_generation` needed for optimistic concurrency. Migration is additive, nullable, no backfill required. See §6.

### 1.4 Keep `INTENTIONALLY_NOT_EXPOSED` honest

`exec` (`container_admin.go:792`) stays air-gapped at `handlers_docker.go` (verified zero exec route) despite being the only well-allowlisted mutating debug surface (20 read-only commands, `path.Base`, `managed` block, `infra-admin` only). `pause`/`unpause` is NOT in that category — it is a standard lifecycle operation users already see in `containers-view.tsx:142,148` — so it must be wired, not documented away. See §3.1.1 decision matrix.

---

## 2. Inventory — Wired vs Broken (with Traces)

### 2.1 Regression guard traces to keep wired (reverification-08 T1-T4, T6)

| Trace | Hop table (file:line) | Guard added (see §5) |
|---|---|---|
| **T1** `listContainers` HEALTHY | `docker.ts:77` `listContainers` → `handlers_docker.go:98` `dockerListContainers` fan-out `nodeListOrFirst` → `daemon/client.go:1718` `AdminContainerList` → `container_admin.go:29` `handleContainerList` (managed-filter) | `handlers_docker_test.go:10` contract: `GET /api/admin/containers?all=` 200 with filtered shape; add unit for `managed` flag |
| **T2** `createContainer` NOW HEALTHY (was 404) | `docker.ts:113` `createContainer` → `handlers_docker.go:136` `dockerCreateContainer` → `client.go:1883` `AdminContainerCreate` `POST /api/admin/containers` → `server.go:478` + `container_admin.go:1776` | §5.1: `TestDockerRoutes_AdminCreate_Mounted` asserts `POST /api/admin/containers` not 404; `TestBeacon_ContainerCreate_StillMounted` via `httptest.NewServer` |
| **T3** `network create` NOW HEALTHY | `docker.ts:163` `createNetwork` → `handlers_docker.go:377` → `client.go:2038` `POST /api/admin/networks` → `server.go:479` + `container_admin.go:1824` | §5.1 sibling |
| **T4** `volume prune` NOW HEALTHY | `docker.ts:185` `pruneVolumes` → `handlers_docker.go:667` `dockerPruneVolumes` → `client.go:2090` `POST /api/admin/volumes/prune` → `server.go:483` + `container_admin.go:1961` | §5.1 sibling |
| **T6** `exec` INTENTIONAL air-gap | `docker.ts` (no exec export) + `handlers_docker.go` (no exec route) → air-gapped; `daemon/client.go:2095` + `server.go:463` + `container_admin.go:792` remain healthy but unexposed | §5.1: `TestDockerRoutes_NoExecMounted` asserts 404 for `POST /docker/containers/:id/exec`; include `INTENTIONALLY_NOT_EXPOSED` doc comment |

### 2.2 Broken traces to fix

| Trace | Break location (file:line) | Failure |
|---|---|---|
| **T5** `pause`/`unpause` BROKEN | `daemon/client.go:1875` `AdminContainerPause` → `server.go:459-461` mux table has only `start/stop/restart`; `container_admin.go` has no `handleContainerPause` | 404 |
| **T7** `compose privileged port` BROKEN | `compose.go:214` `shortFormHostPort` `default→""` + `validateComposePorts:190` `if published=="" continue` | `"80"` → `""` → skip `<1024` check |
| **T8** `compose per-replica resources` BROKEN | `lifecycle.go:280` ceiling checks `req.MemoryMB` only, not `sum(service.limits*mul replicas)`; `service.go:846` `Replicas` never consumed | 10×8G bypasses 64G ceiling |
| **Image prune** HALF-FIXED | `handlers_docker.go` (no `POST /docker/images/prune`), only `handlers_portainer.go:40/557` under `/portainer` | Docker tab dead code |
| **Volume trifurcation** BROKEN | `service.go:595` vs `compose.go:226` vs `mounts.go:64` | `200 valid→400 violation` |
| **env_file** SILENT DROP | `service.go:92` `rawService` no `EnvFile`; `validateComposePolicy:77` no `env_file` case | Crash at runtime, not validate |
| **Build context** MISSING | `daemon/compose.go:13` lacks tarball; `compose.go:374` only `compose.yaml`+`.env` | `409` opaque |

---

## 3. Detailed Fix Design (by Finding)

### 3.1 Container Network / Volume admin surface — keep FIXED, fix lossy edges

#### 3.1.0 Regression guard (shared)

See §5 for tests that must pass before any change lands. The routes at `server.go:478/479/481-483` must remain; do not rename them. Any refactor must keep `POST /api/admin/containers`, `POST /api/admin/networks`, `POST /api/admin/volumes`, `POST /api/admin/volumes/prune` verbatim.

#### 3.1.1 `pause`/`unpause` — wire handler (do NOT document as air-gap)

**Decision:** Wire, not `INTENTIONALLY_NOT_EXPOSED`. Rationale: `exec` is intentionally air-gapped because it is an authenticated RCE surface with allowlist footnotes (20 commands, `path.Base`, managed-block). `pause`/`unpause` is a normal Docker lifecycle action already exposed by `1panel/agent/app/api/v2/container.go:612` `ContainerOperation` (start/stop/restart/pause/unpause/kill) and expected by `containers-view.tsx:142,148`. The Daemon client already expects it (`client.go:1875,1879`) and API already forwards it (`handlers_docker.go:176,178`). Leaving it 404 is USER_VISIBLE broken, not honest absence.

**Beacon:**

```go
// beacon/internal/server/container_admin.go — new, alongside handleContainerStart:193
func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
    s.adminContainerAction(w, r, "pause")
}
func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
    s.adminContainerAction(w, r, "unpause")
}

// extend adminContainerAction switch (container_admin.go:251):
func (s *Server) adminContainerAction(w http.ResponseWriter, r *http.Request, action string) {
    // ... existing auth + inspect + managed-container + state checks up to :249 ...
    switch action {
    case "start":
        err = docker.ContainerStart(r.Context(), id, container.StartOptions{})
    case "stop":
        timeout := 30
        err = docker.ContainerStop(r.Context(), id, container.StopOptions{Timeout: &timeout})
    case "restart":
        timeout := 30
        err = docker.ContainerRestart(r.Context(), id, container.StopOptions{Timeout: &timeout})
    case "pause":
        // Pause only makes sense for running containers. Mirror stop/restart guard.
        if inspectErr == nil && !inspect.State.Running {
            writeError(w, http.StatusConflict, "container is not running")
            return
        }
        if inspect.State.Paused {
            writeError(w, http.StatusConflict, "container is already paused")
            return
        }
        err = docker.ContainerPause(r.Context(), id)
    case "unpause":
        if inspectErr == nil && !inspect.State.Paused {
            writeError(w, http.StatusConflict, "container is not paused")
            return
        }
        err = docker.ContainerUnpause(r.Context(), id)
    }
    // ... existing error mapping (IsErrNotFound → 404, else 409) + audit log :271 ...
    s.logAdminAction(r.Context(), userInfo.UserID, "container:"+action, "container", &id, map[string]interface{}{"status": "success"})
    writeJSON(w, http.StatusOK, map[string]any{"id": id, "action": action, "status": "ok"})
}
```

**Mount** in `beacon/internal/server/server.go:456-483` (preserve existing order; insert after restart):

```go
mux.HandleFunc("POST /api/admin/containers/{id}/start", server.handleContainerStart)
mux.HandleFunc("POST /api/admin/containers/{id}/stop", server.handleContainerStop)
mux.HandleFunc("POST /api/admin/containers/{id}/restart", server.handleContainerRestart)
mux.HandleFunc("POST /api/admin/containers/{id}/pause", server.handleContainerPause)     // ADD
mux.HandleFunc("POST /api/admin/containers/{id}/unpause", server.handleContainerUnpause) // ADD
mux.HandleFunc("DELETE /api/admin/containers/{id}", server.handleContainerDelete)
```

No change to `server.go:478` `POST /api/admin/containers` — keep wired.

**Daemon** `forge/api/internal/daemon/client.go:1875/1879` already has `AdminContainerPause/Unpause` (`POST /api/admin/containers/{id}/pause`). Verify idempotency: signing via `newRequest` + `retryRoundTripper` with `resignRequest` fresh nonce already handles retries. No change needed.

**API** `handlers_docker.go:176-178` already forwards `pause`/`unpause` via `AdminContainerPause:1875`. No change. Ensure `docker_test` covers the new 200 path (see §5).

**Alternative explicitly rejected:** Documenting as `INTENTIONALLY_NOT_EXPOSED` and removing buttons from `containers-view.tsx:142,148` would hide a P2 USER_VISIBLE bug behind docs. Not accepted — wire instead. If an operator truly wants the air-gap, they can gate via `requireAdminScope("servers.write")` + audit already present, or set `DAEMON_PAUSE_DISABLED=true` env flag (future; not in this plan).

#### 3.1.2 Image prune — wire Docker admin path (keep Portainer path for compat)

**Beacon** `container_admin.go:531` `handleImagePrune` + `server.go:476` `POST /api/admin/images/prune` already correct. Keep.

**API** add missing mount in `forge/api/internal/http/handlers_docker.go:25-63`:

```go
func registerDockerRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
    // ...
    docker.Post("/images/prune", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), dockerPruneImages(cfg)) // ADD
    docker.Post("/volumes/prune", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), dockerPruneVolumes(cfg))
}

func dockerPruneImages(cfg Config) fiber.Handler {
    return func(c *fiber.Ctx) error {
        targets, err := nodeListOrFirst(cfg)
        if err != nil { return err }
        results := make([]fiber.Map, 0, len(targets))
        for _, t := range targets {
            data, err := cfg.Daemon.AdminImagePrune(c.Context(), t.NodeURL, t.NodeToken)
            if err != nil { continue }
            results = append(results, fiber.Map{"nodeId": t.NodeID, "nodeName": t.NodeName, "result": data})
            recordAudit(cfg, c, "image:prune", "node", &t.NodeID, nil)
        }
        return c.JSON(results)
    }
}
```

**Daemon** `client.go:1845` `AdminImagePrune` (`POST /api/admin/images/prune`) already exists; `handlers_portainer.go:40/557` `POST /portainer/images/prune` continues to proxy to same `AdminImagePrune` for backward compat — do not remove it. Both paths now succeed; Docker-tab callers no longer need Portainer prefix.

**Frontend** add export in `forge/web/lib/api/docker.ts:147` + `images-view.tsx:30`:

```ts
export async function pruneImages(): Promise<DockerOperationResult> {
  return postJSON<DockerOperationResult>("/docker/images/prune");
}
// ImagesView prune button (mirrors volumes-view.tsx:44 Eraser):
// <Button onClick={() => pruneMut.mutate()} confirming with X-Confirm-Destructive flow>
```

Backward compat: existing `GET /docker/images` callers unaffected; new `POST /docker/images/prune` returns `200 {nodeId,result}` per-node fan-out (same shape as `pruneVolumes`).

#### 3.1.3 Lossy container/network create — extend handler struct (no breaking change)

**Problem:** `container_admin.go:1787-1813` only binds `{name,image,cmd,env,labels}` then `ContainerCreate(...,nil,nil,nil,name)` — UI `CreateContainerRequest{ports,volumes,network,restartPolicy}` silently dropped. `handleNetworkCreate:1835-1839` ignores subnet → no IPAM.

**Beacon** `container_admin.go:1776` — extend request struct while keeping old fields optional (wire-compatible — old callers with only `name+image` still succeed):

```go
// handleContainerCreate (beacon/internal/server/container_admin.go:1776)
// Keep image+name required check at :1798; add progressive wiring.
type containerCreateRequest struct {
    Name          string            `json:"name"`
    Image         string            `json:"image"`
    Cmd           []string          `json:"cmd"`
    Env           []string          `json:"env"`
    Labels        map[string]string `json:"labels"`
    // New optional fields — honor when supplied, ignore when absent (backward compat):
    Ports         []struct{ HostPort, ContainerPort int `json:"hostPort"`; Protocol string `json:"protocol"`; HostIP string `json:"hostIp"` } `json:"ports"`
    Volumes       []struct{ HostPath, ContainerPath string `json:"hostPath"`; ReadOnly bool `json:"readOnly"` } `json:"volumes"`
    Network       string            `json:"network"`
    RestartPolicy string            `json:"restartPolicy"`
}

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
    // ... auth :1776 ...
    var req containerCreateRequest
    if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&req); err != nil { /* 400 */ }
    if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Image) == "" { /* 400 */ }

    // Build HostConfig when ports/volumes/restartPolicy present. Use existing helpers
    // from docker.go:865 dockerPorts() + :743 buildContainerMounts() helpers or inline.
    // When absent, HostConfig stays nil-like (but now correctly non-nil with defaults
    // for isolation: still need hardening review — keep nil for minimal change; add
    // isolation flags only if operator opts in via future flag).
    exposed, bindings, _ := dockerPortsFromCreateRequest(req.Ports) // new helper
    mounts := mountsFromCreateRequest(req.Volumes)                  // reuse allowedMountSource: mounts.go:64
    hostCfg := &container.HostConfig{
        PortBindings:  bindings,
        Mounts:        mounts,
        RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(req.RestartPolicy)},
    }
    networkingCfg := buildNetworkingConfigForAdmin(req.Network) // nil when empty

    config := &container.Config{
        Image:      req.Image,
        Cmd:        req.Cmd,
        Env:        req.Env,
        Labels:     req.Labels,
        ExposedPorts: exposed,
    }
    created, err := docker.ContainerCreate(r.Context(), config, hostCfg, networkingCfg, nil, req.Name)
    // ... audit logAdminAction:1802 + return id ...
}
```

Complexity kept low: first iteration honors `ports` + `restartPolicy` + `network`; `volumes` with `/proc:/sys:/var/run/docker.sock` is denied via `allowedMountSource` (`mounts.go:64`) already — surface that as `400 compose policy violation`-style error. Detailed isolation flags (`CapDrop ALL`, `ReadonlyRootfs`, `SecurityOpt no-new-privileges`) are still absent for admin containers (intentional — admin containers are not `mgp-*` game workloads). Future hardening can add them behind a flag without breaking this step.

**Network** `container_admin.go:1824` — handle subnet with validation (do not break callers who omitted `subnet`):

```go
type networkCreateRequest struct {
    Name     string            `json:"name"`
    Driver   string            `json:"driver"`
    Labels   map[string]string `json:"labels"`
    Internal bool              `json:"internal"`
    Subnet   string            `json:"subnet"` // NEW — optional CIDR like "10.20.0.0/16"
    Gateway  string            `json:"gateway"`
}

func (s *Server) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
    var req networkCreateRequest
    // ...
    opts := network.CreateOptions{
        Driver:  req.Driver,
        Labels:  req.Labels,
        Internal: req.Internal,
    }
    if strings.TrimSpace(req.Driver) == "" {
        opts.Driver = "bridge"
    }
    if s := strings.TrimSpace(req.Subnet); s != "" {
        if _, _, err := net.ParseCIDR(s); err != nil {
            writeError(w, http.StatusBadRequest, "invalid subnet CIDR: "+err.Error())
            return
        }
        ipamCfg := network.IPAMConfig{Subnet: s}
        if g := strings.TrimSpace(req.Gateway); g != "" {
            if net.ParseIP(g) == nil {
                writeError(w, http.StatusBadRequest, "invalid gateway IP")
                return
            }
            ipamCfg.Gateway = g
        }
        opts.IPAM = &network.IPAM{Config: []network.IPAMConfig{ipamCfg}}
    }
    netw, err := docker.NetworkCreate(r.Context(), req.Name, opts)
    // ...
}
```

**Frontend:** `docker.ts:159` `createNetwork` already sends `subnet` to API — no change. API `handlers_docker.go:377` `dockerCreateNetwork` forwards `map[string]any` body verbatim via `AdminNetworkCreate:2038` so new `subnet` is carried without API change. Document that `subnet` now actually takes effect.

### 3.2 `shortFormHostPort` privileged-port bypass — P1

**Root cause restated:** `beacon/internal/server/compose.go:214`

```go
func shortFormHostPort(entry string) string {
    parts := strings.Split(entry, ":")
    switch len(parts) {
    case 2: return parts[0]
    case 3: return parts[1]
    default: return "" // "80" (len 1) → "" → validateComposePorts:198 skip
    }
}
```

Plus `validateComposePorts:198` `if published=="" continue` skips `<1024` check. Single-value `"80"` was intended as a host publish (or at least defensively block privileged). `"127.0.0.1::80"` also bypasses (split `"127.0.0.1", "", "80"` → len 3 → `parts[1]==""` → `published==""` → skip). Protocol suffix `"80:80/tcp"` fails `Atoi("80/tcp")` → silent continue.

**Fix — beacon `compose.go:181-224` (no API surface change):**

```go
func shortFormHostPort(entry string) string {
    // Strip protocol suffix (/tcp, /udp, /sctp) before colon analysis.
    // "8080:80/tcp" → "8080:80", "80/tcp" → "80", "80" → "80"
    if idx := strings.Index(entry, "/"); idx >= 0 {
        entry = entry[:idx]
    }
    entry = strings.TrimSpace(entry)
    if entry == "" {
        return ""
    }
    parts := strings.Split(entry, ":")
    switch len(parts) {
    case 1:
        // Bare port like "80" — in Compose short-form this is container-only
        // on some engines, but for our security posture treat it as a candidate
        // host publish and apply the <1024 guard (defensive). This closes the
        // bypass reported in LF-06 / COMP-03.
        return parts[0]
    case 2:
        return parts[0]
    case 3:
        // "ip:hostPort:containerPort" — empty hostPort ("127.0.0.1::80") means
        // engine picks random high port, not privileged; but we must not return
        // "" then skip. Return parts[1] even when empty so caller can decide:
        // empty → skip privileged check (safe), non-empty → check. Our current
        // caller does `if published=="" continue` which for "::80" would still
        // skip — but that is correct because no fixed host port requested.
        // Keep empty → skip. For defensive explicitness, keep as-is.
        return parts[1]
    default:
        // IPv6 host IP with colons: rejected elsewhere, but defensively take last-1
        // e.g. "[::1]:8080:80" split would be 4+ parts. Prefer docker/go-connections.
        return ""
    }
}

// Alternatively, delegate to docker/go-connections/nat.ParsePortSpec for full
// fidelity (handles IPv6, protocol, range). For this plan keep the lightweight
// split fix + protocol strip; migrate to nat.ParsePortSpec in the follow-up.

func validateComposePorts(service string, value any) error {
    entries, ok := value.([]any)
    if !ok { return nil }
    for _, entry := range entries {
        var published string
        switch v := entry.(type) {
        case string:
            // Strip protocol first for cases like "80:80/tcp"
            raw := v
            if idx := strings.Index(raw, "/"); idx >= 0 {
                raw = raw[:idx]
            }
            published = shortFormHostPort(raw)
        case map[string]any:
            if raw, ok := v["published"]; ok {
                published = fmt.Sprint(raw)
                // long-form published may also have protocol suffix
                if idx := strings.Index(published, "/"); idx >= 0 {
                    published = published[:idx]
                }
            }
        default:
            continue
        }
        published = strings.TrimSpace(published)
        if published == "" {
            continue // ephemeral or no host port → nothing to guard
        }
        // Handle "8080-8082:80-82" ranges — take first host port
        if idx := strings.Index(published, "-"); idx >= 0 {
            published = published[:idx]
        }
        port, err := strconv.Atoi(published)
        if err != nil || port <= 0 {
            continue
        }
        if port < 1024 {
            return fmt.Errorf("service %q: publishing privileged host port %d is not allowed in compose deployments", service, port)
        }
    }
    return nil
}
```

**API parity:** Add same check to `forge/api/internal/services/compose/service.go:427` `ValidateComposeSecurity` (currently has zero port check). Mirror logic or extract shared package so both layers agree. This closes the API-then-Beacon mismatch where API says `valid:true` then Beacon `400 policy violation` (divergent messaging; but also fixes the case where Beacon was bypassed).

**Tests:** Add `beacon/internal/server/compose_ports_test.go` (unit — no Docker needed):

```go
func TestShortFormHostPortPrivilegedBypass(t *testing.T) {
    cases := []struct{ in, host string }{
        {"80", "80"},
        {"80:80", "80"},
        {"127.0.0.1:80:80", "80"},
        {"8080:80/tcp", "8080"},
        {"80/tcp", "80"},
        {"8080:80", "8080"},
        {"127.0.0.1::80", ""}, // random host port → empty, skip check (correct)
    }
    for _, c := range cases { ... }
}
func TestValidateComposePortsPrivileged(t *testing.T) {
    // ports: ["80"] must error
    yaml := "services:\n  web:\n    image: nginx\n    ports: [\"80\"]\n"
    if err := validateComposePolicy(yaml); err == nil { t.Fatalf("want privileged port error for [\"80\"]") }
    // ["127.0.0.1::80"] must NOT error (no fixed host port)
    // long-form published: 22 must error
}
```

Reverification python split proof gone; replaced by Go unit.

Backward compat: existing stacks with `"8080:80"` unaffected; new `"80"` deploys now correctly rejected with `400 compose policy violation: publishing privileged host port 80 is not allowed` (beacon) and `ValidateComposeSecurity` error at API. Operators needing `80` should front with `gateway/reverse-proxy` — same guidance as before.

### 3.3 Per-replica resource ceiling — P1 (quota bypass via `deploy.replicas`)

**Current:** `lifecycle.go:280`

```go
if req.MemoryMB > MaxUserStackMemoryMB || req.DiskMB > MaxUserStackDiskMB || req.CPUShares > MaxUserStackCPUShares {
    return nil, fmt.Errorf("%w: memory<=%dMB ...", ErrResourceLimitExceeded, MaxUserStackMemoryMB, ...)
}
```

Plus `scheduler.PlaceServer:293` uses `req.CPUShares/MemoryMB/DiskMB` top-level only. Per-service `deploy.resources.limits` (`parser.go:913` `normalizeResources` only `cpu`/`memory`) never multiplied by `replicas`.

**Fix — derive total requested from YAML at placement time; no schema migration required:**

1. Parse YAML via existing `ParseComposeYAML` (fallback parser) inside `DeployComposeStack:260` before placement, or reuse `service.go:145` parse result (add `ParsedCompose` to the validation path). No new dependency on compose-go loader (keep deploy-hot-path behavior consistent).

2. Helper computes totals from `ParsedCompose` (already has `DeploySummary{Replicas, Resources{Limits: map[string]string}}` via `normalizeDeploy:841`):

```go
// forge/api/internal/services/compose/quota.go — new file, pure function + tests
package compose

import (
    "fmt"
    "math"
    "strconv"
    "strings"
)

// parseMemoryToMB parses "8G", "512M", "1024", "8GiB" variants from deploy.resources.limits.memory values.
// It accepts the string stored in ResourceSummary (e.g. "8589934592" bytes from parser.go:913) as well
// as human forms from the fallback parser ("8G", "512MiB").
func parseMemoryToMB(raw string) (int64, error) {
    s := strings.TrimSpace(raw)
    if s == "" { return 0, nil }
    // Already bytes as decimal from parser.go:928 "8589934592"
    if v, err := strconv.ParseInt(s, 10, 64); err == nil {
        return v / (1024 * 1024), nil
    }
    // Human forms: 8G, 8GB, 8GiB, 512M, 512MiB, 1K etc.
    suffixes := []struct{ suffix string; mult int64 }{
        {"TiB", 1024 * 1024}, {"GiB", 1024}, {"MiB", 1}, {"KiB", 0}, // KiB below 1MiB rounds to 0 — treat as 1
        {"TB", 1024 * 1024}, {"GB", 1024}, {"MB", 1}, {"KB", 0},
        {"T", 1024 * 1024}, {"G", 1024}, {"M", 1}, {"K", 0},
    }
    upper := strings.ToUpper(s)
    for _, sf := range suffixes {
        if strings.HasSuffix(upper, sf.suffix) {
            num := strings.TrimSpace(s[:len(s)-len(sf.suffix)])
            f, err := strconv.ParseFloat(num, 64)
            if err != nil { return 0, fmt.Errorf("invalid memory %q", raw) }
            mb := int64(math.Ceil(f * float64(sf.mult)))
            if strings.HasSuffix(sf.suffix, "KIB") || strings.HasSuffix(sf.suffix, "KB") || sf.suffix == "K" {
                if mb == 0 && f > 0 { mb = 1 }
            }
            return mb, nil
        }
    }
    return 0, fmt.Errorf("unrecognized memory %q", raw)
}

func parseCPUToShares(raw string) (int64, error) {
    // raw is NanoCPUs as decimal string from parser.go:926, or human "4.0" / "4000m"
    // For simplicity map NanoCPUs → cpuShares 1024 ~= 1 cpu. Docker default 1024.
    // Reuse existing conversion: CPUShares already in req; here we just want per-service cpus.
    s := strings.TrimSpace(raw)
    if s == "" { return 0, nil }
    // Try NanoCPUs decimal first
    if v, err := strconv.ParseFloat(s, 64); err == nil && !strings.ContainsAny(s, "mM") {
        // NanoCPUs like "4000000000" (4 cpus) or maybe "4.0"? Heuristic: > 1000 is nanos
        if v >= 1000 {
            cpus := v / 1e9
            return int64(cpus * 1024), nil
        }
        return int64(v * 1024), nil
    }
    return 0, fmt.Errorf("unrecognized cpu %q", raw)
}

// TotalRequested computes the aggregate request across all services: sum(limits * replicas).
// It is used for quota enforcement, not for scheduling precision. Reserved==limits for quota.
func TotalRequested(services []ServiceSummary) (memoryMB int64, cpuShares int64, replicasTotal int, err error) {
    for _, svc := range services {
        reps := 1
        if svc.Deploy != nil && svc.Deploy.Replicas > 0 {
            reps = svc.Deploy.Replicas
        }
        var mem, cpu int64
        if svc.Deploy != nil && svc.Deploy.Resources != nil {
            if v, ok := svc.Deploy.Resources.Limits["memory"]; ok {
                m, e := parseMemoryToMB(v)
                if e != nil { return 0,0,0,e }
                mem = m
            }
            if v, ok := svc.Deploy.Resources.Limits["cpu"]; ok {
                c, e := parseCPUToShares(v)
                if e != nil { return 0,0,0,e }
                cpu = c
            }
            // Also include reservations["memory"] when limits absent (conservative: take max)
            if mem == 0 {
                if v, ok := svc.Deploy.Resources.Reservations["memory"]; ok {
                    m, e := parseMemoryToMB(v); if e==nil { mem = m }
                }
            }
            if cpu == 0 {
                if v, ok := svc.Deploy.Resources.Reservations["cpu"]; ok {
                    c, e := parseCPUToShares(v); if e==nil { cpu = c }
                }
            }
            // PidsLimit, Gpus, Ulimits ignored for quota — not enforceable via MB/shares.
        }
        memoryMB += mem * int64(reps)
        cpuShares += cpu * int64(reps)
        replicasTotal += reps
    }
    return
}
```

3. Wire into `lifecycle.go:273-283` (replace the bare per-stack check):

```go
// AFTER ValidateCompose gate (lifecycle.go:264) — new block before per-user quota:
parsed, _ := s.ParseComposeYAML([]byte(req.ComposeYAML), "", nil) // reuse parse from ValidateCompose if available
totalMem, totalCPU, totalReps, parseErr := TotalRequested(parsed.Services)
if parseErr != nil {
    return nil, fmt.Errorf("%w: resource parse: %v", ErrInvalidCompose, parseErr)
}
// Merge top-level request (legacy callers pass MemoryMB/CPUShares explicitly) with YAML-derived totals.
// Take the conservative max so old callers without deploy.resources still enforced:
effectiveMem := max(req.MemoryMB, totalMem)
effectiveCPU := max(req.CPUShares, totalCPU)
// Replicas scale only matters for derived totals; still enforce ceiling:
if effectiveMem > MaxUserStackMemoryMB || req.DiskMB > MaxUserStackDiskMB || effectiveCPU > MaxUserStackCPUShares {
    return nil, fmt.Errorf("%w: effective memory %dMB cpuShares %d + disk %dMB exceeds per-stack ceilings memory<=%d disk<=%d cpuShares<=%d (services %d, totalReplicas %d)",
        ErrResourceLimitExceeded, effectiveMem, effectiveCPU, req.DiskMB,
        MaxUserStackMemoryMB, MaxUserStackDiskMB, MaxUserStackCPUShares,
        len(parsed.Services), totalReps)
}
// Pass effective values to scheduler/reservation so placement sees YAML reality:
if s.scheduler != nil && req.NodeID == "" {
    decision, placeErr = s.scheduler.PlaceServer(ctx, domain.PlacementRequest{
        CPU:      int(effectiveCPU),
        MemoryMB: int(effectiveMem),
        DiskMB:   int(req.DiskMB),
        RegionID: "",
    })
}
 // And to reservation create:
 reservation, err = s.store.CreatePlacementReservation(ctx, store.CreatePlacementReservationRequest{
    NodeID: req.NodeID,
    CPU:    int(effectiveCPU / 100), // existing convention in lifecycle.go:328
    Memory: effectiveMem,
    Disk:   req.DiskMB,
})
```

No DB migration needed — `compose_stacks.memory_mb/cpu_shares` can continue storing the caller-supplied top-level (now effectively `max(...)`), or store `effective*` (prefer effective for audit). Either is additive. Pick storing `effective*` and log `requested vs effective` in span log.

**Backward compat:** Stacks already deployed before this fix are not retroactively re-checked. A reconcile sweep (`ListComposeStacksForReconciliation:416`) logs `WARN totalMem > MaxUserStackMemoryMB for stack <id>` but does not stop/downgrade them. Operators should be told to re-deploy if they relied on the bypass.

**Why not at Beacon?** Beacon could re-check via `validateComposePolicy` extended with resource guard, but placement must fail-fast on the panel before reservations. This plan puts the guard at the panel (authoritative) and adds a defence-in-depth beacon check if needed later behind a flag. No `buildResources`/`buildHostConfigWithSettings` bridging — that isolation tier stays game-only (`docker.go:783`/`827`).

### 3.4 Volume allowlist bifurcation — unify predicate

**Current split:**

- Game: `mounts.go:64` `allowedMountSource(source, allowed)` — `EvalSymlinks(Clean(source))` + `Rel` within `allowedMounts` (empty → deny all). Correct.
- API `service.go:595` `checkVolumesSecurity` warns on `/, /root, /etc, /home` (`sensitiveHostPaths:679`); `service.go:663` `ValidateHostMountWithAllowlist` gate exists but off-path for compose.
- Beacon compose `compose.go:226` `validateComposeVolumes` strictly `if strings.HasPrefix(source,"/") → error` with no allowlist — any absolute host bind rejected unconditionally.

Result: `POST /compose/validate` returns `valid:true` with warning for `/data`, then Beacon `400 policy violation` on same YAML (P0 bifurcation).

**Fix — plumb allowlist into Beacon and share predicate:**

#### Step A — daemon contract (`forge/api/internal/daemon/compose.go`)

Extend `ComposeDeployRequest` (currently `StackID/ComposeYAML/EnvVars`) to carry the resolved allowlist snapshot and caller privilege used at validation time:

```go
type ComposeDeployRequest struct {
    StackID       string            `json:"stackId"`
    ComposeYAML   string            `json:"composeYaml"`
    EnvVars       map[string]string `json:"envVars,omitempty"`
    RemoveOrphans bool              `json:"removeOrphans,omitempty"` // keep existing beacon doc
    // NEW — plumbs the validation predicate inputs to Beacon so validate == deploy:
    AllowedMounts []string `json:"allowedMounts,omitempty"`
    IsAdmin       bool     `json:"isAdmin,omitempty"`
}
```

Also extend `ComposeDelete` path to accept `keepVolumes` (see Step C).

#### Step B — API `lifecycle.go` → daemon call sites

In `DeployComposeStack:396` and `UpdateComposeStack:533` and `gitops.go:384` (DeployFromGit) and `RollbackToPrevious`:

```go
// Before client.ComposeDeploy — resolve node's allowed_mounts from store + caller role:
allowedMounts, _ := s.store.GetNodeAllowedMounts(ctx, req.NodeID) // or s.allowedMountsForNode(nodeID)
isAdmin := isCallerInfraAdmin(ctx) // existing helper that feeds getAdminUserInfo:1151 at Beacon
deployResp, err := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
    StackID:       stackID,
    ComposeYAML:   req.ComposeYAML,
    EnvVars:       req.EnvVars,
    RemoveOrphans: req.RemoveOrphans, // if exposed from request (default false)
    AllowedMounts: allowedMounts,
    IsAdmin:       isAdmin,
})
```

No DB schema change for `allowedMounts` — node `allowed_mounts` already stored via `SetAllowedMounts` in `server.go:129` backing; API can derive it from node metadata/store (or call `GET /api/capabilities` heartbeat snapshot).

#### Step C — Beacon `compose.go:226` — enforce with the same predicate

```go
// compose.go — new exported variant used by handleComposeDeploy

func validateComposeVolumesWithAllowlist(service string, value any, allowedMounts []string, isAdmin bool) error {
    entries, ok := value.([]any)
    if !ok { return nil }
    for _, entry := range entries {
        switch v := entry.(type) {
        case map[string]any:
            if composeString(v["type"]) == "bind" {
                return fmt.Errorf("service %q: long-form bind mounts are not allowed in compose deployments", service)
            }
        case string:
            source, _, hasSource := strings.Cut(v, ":")
            if !hasSource || source == "" { continue } // anonymous volume "/data" or bare target
            // Disallow docker.sock unconditionally (API already errors)
            lower := strings.ToLower(source)
            if strings.HasPrefix(lower, "/var/run/docker.sock") || strings.Contains(lower, ":/var/run/docker.sock") {
                return fmt.Errorf("service %q: mounting docker.sock is not allowed", service)
            }
            if strings.HasPrefix(lower, "/proc") || strings.HasPrefix(lower, "/sys") {
                return fmt.Errorf("service %q: mounting %q is not allowed", service, source)
            }
            if strings.HasPrefix(source, "/") || isPathTraversal(source) {
                // Use the unified allowlist predicate instead of blanket reject:
                if err := validateHostMount(source, isAdmin, allowedMounts); err != nil {
                    return fmt.Errorf("service %q: host bind mount source %q is not allowed: %w", service, source, err)
                }
                // If allowed, then permit (no error).
                continue
            }
        }
    }
    return nil
}

func validateHostMount(source string, isAdmin bool, allowed []string) error {
    // mirrors forge/api/internal/services/compose/service.go:663
    // ValidateHostMountWithAllowlist — must stay in sync. Extract to shared package
    // in the follow-up; for now duplicate with comment to keep in sync.
    clean := filepath.Clean(source)
    // sensitiveHostPaths: "/", "/root", "/etc", "/home" — keep in sync with service.go:679
    isSensitive := clean == "/" || strings.HasPrefix(clean, "/root/") || clean == "/root" ||
        strings.HasPrefix(clean, "/etc/") || clean == "/etc" ||
        strings.HasPrefix(clean, "/home/") || clean == "/home"
    // If not sensitive, allow even without allowlist? No — keep deny-by-default for host binds.
    // For non-sensitive host paths, check allowlist only.
    // For sensitive paths, require admin+allowlist (matches API hardening at service.go:634-654).
    _ = isSensitive
    if !isAdmin {
        return fmt.Errorf("host path mount %q requires admin", clean)
    }
    for _, a := range allowed {
        aClean := filepath.Clean(a)
        if clean == aClean || strings.HasPrefix(clean, aClean+"/") {
            return nil
        }
    }
    return fmt.Errorf("host path mount %q is not on the allowedMounts allowlist", clean)
}

// Keep old validateComposeVolumes for tests/caller compatibility; delegate:
func validateComposeVolumes(service string, value any) error {
    return validateComposeVolumesWithAllowlist(service, value, nil, false) // no allowlist → all host binds rejected (old behavior preserved for callers without allowlist)
}
```

**Wire:** In `validateComposePolicy:77` extend signature to accept `(yaml string, allowedMounts []string, isAdmin bool)` and in `handleComposeDeploy:358` call:

```go
if err := validateComposePolicyWithAllowlist(req.ComposeYAML, req.AllowedMounts, req.IsAdmin); err != nil {
    writeError(w, http.StatusBadRequest, "compose policy violation: "+err.Error())
    return
}
```

Keep `validateComposePolicy(yamlContent string) error` as a wrapper delegating to `(...,nil,false)` for test/backward compat.

**API `service.go:595` `checkVolumesSecurity`** — retain warnings but ensure `ValidateHostMountWithAllowlist` is the source of truth for compose path warnings: when a sensitive mount is outside the allowlist, `ValidateComposeSecurity` should emit `severity:error` (not warning) when `allowedMounts` would deny it, if that call has access to the node's allowlist snapshot (it currently doesn't — so keep warning at API and hard error at Beacon; after allowlist plumbing both agree when `allowedMounts` matches node's `SetAllowedMounts`).

**Delete data-loss — `compose.go:578` → opt-in `-v`:**

Current:

```go
downArgs := []string{"compose", "-f", composePath, "-p", stackID, "down", "-v"}
```

Change to (beacon hot-fix already has `removeOrphans` gate at `:574`; mirror for volumes):

```go
// DELETE /compose/{stackId}?volumes=true&removeOrphans=true
// Default: do NOT delete volumes (safe). Operators must explicitly pass volumes=true.
removeVolumes := r.URL.Query().Get("volumes") == "true" || r.URL.Query().Get("v") == "true" // accept "v" for docker parity
removeOrphans := r.URL.Query().Get("removeOrphans") == "true"

downArgs := []string{"compose", "-f", composePath, "-p", stackID, "down"}
if removeVolumes {
    downArgs = append(downArgs, "-v")
}
if removeOrphans {
    downArgs = append(downArgs, "--remove-orphans")
}
```

API `handlers_compose.go:401` `DELETE /compose/:id` currently calls `composeSvc.DeleteComposeStack` with no query forwarding. Extend:

```go
protected.Delete("/compose/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
    // ...
    volumes := c.Query("volumes") == "true" || c.Query("v") == "true"
    removeOrphans := c.Query("removeOrphans") == "true"
    if err := composeSvc.DeleteComposeStackWithOptions(ctx, c.Params("id"), DeleteOptions{RemoveVolumes: volumes, RemoveOrphans: removeOrphans}); err != nil { ... }
})
```

Add `DeleteOptions` struct; keep `DeleteComposeStack(ctx, id)` as wrapper calling `(..., DeleteOptions{})` for callers not yet migrated (backward compat — previously hard-coded `-v` now becomes opt-in, so delete without flag becomes safe where it was destructive before; that is intended and communicated).

Daemon `daemon/compose.go:85` `ComposeDelete` should forward the query params:

```go
func (c *Client) ComposeDelete(ctx context.Context, baseURL, nodeToken, stackID string, opts DeleteOptions) (ComposeOperationResponse, error) {
    endpoint := strings.TrimRight(baseURL, "/") + "/compose/" + stackID
    q := url.Values{}
    if opts.RemoveVolumes { q.Set("volumes","true") }
    if opts.RemoveOrphans { q.Set("removeOrphans","true") }
    if len(q) > 0 { endpoint += "?" + q.Encode() }
    // existing DELETE to endpoint
}
```

**Frontend:** Compose detail `DELETE` confirmation modal must now have a destructive-checkbox "Delete volumes (`down -v`) — destroys named volumes like `pgdata`" unchecked by default, requiring typing the stack name (mirrors `images prune` `X-Confirm-Destructive`). See §4.6.

### 3.5 `env_file` — fail-fast with import parity (P0)

**Current:** `service.go:92` `rawService` ignores `env_file`; `validateComposePolicy:77` has no `env_file` case; `compose.go:380` only writes `EnvVars` → `.env`. Import path `parser.go:364` `ParseComposeString` via compose-go DOES understand `env_file`, so `POST /compose/validate` can show `valid` while deploy silently omits vars.

**Fix — phase 1: fail-fast + document (no host mount of arbitrary host files):**

1. Extend `rawService` to surface the key so validation can reject it honestly:

```go
// forge/api/internal/services/compose/service.go:92
type rawService struct {
    Image       string                 `yaml:"image,omitempty"`
    Build       map[string]interface{} `yaml:"build,omitempty"`
    Ports       []interface{}          `yaml:"ports,omitempty"`
    Environment interface{}            `yaml:"environment,omitempty"`
    EnvFile     interface{}            `yaml:"env_file,omitempty"` // ADD — for validation only
    Volumes     []interface{}          `yaml:"volumes,omitempty"`
    DependsOn   interface{}            `yaml:"depends_on,omitempty"`
    Profiles    []string               `yaml:"profiles,omitempty"`
    Restart     string                 `yaml:"restart,omitempty"`
    Command     interface{}            `yaml:"command,omitempty"`
    Entrypoint  interface{}            `yaml:"entrypoint,omitempty"`
    HealthCheck map[string]interface{} `yaml:"healthcheck,omitempty"`
    Deploy      map[string]interface{} `yaml:"deploy,omitempty"`
    Secrets     []interface{}          `yaml:"secrets,omitempty"`
    Configs     []interface{}          `yaml:"configs,omitempty"`
}
```

2. In `ValidateComposeSecurity:427` add error branch (this is the ONLY place that must error — it gates `DeployComposeStack:264`):

```go
if _, hasEnvFile := svc["env_file"]; hasEnvFile {
    issues = append(issues, ValidationIssue{
        Field:    fmt.Sprintf("services.%s.env_file", name),
        Message:  "env_file is not supported in Forge managed compose deployments — declare variables via the Environment section or project-level EnvVars; host file references are rejected",
        Severity: "error", // was silently dropped; now honestly rejected
    })
}
// Also check top-level env_file if someone puts it at service-include level:
if rawMap["env_file"] != nil {
    issues = append(issues, ValidationIssue{Field: "env_file", Message: "...", Severity: "error"})
}
```

3. Beacon `validateComposePolicy:77` — add same error branch so Beacon defense-in-depth matches API (defense in depth: `validateComposePolicy` denies `env_file`, `cap_add`, etc.):

```go
case "env_file":
    return fmt.Errorf("service %q: env_file is not supported — use environment: or project EnvVars", name)
case "extends":
    return fmt.Errorf("service %q: extends is not supported — inline the base service", name)
```

4. `Parser.go` fallback vs compose-go divergence: `service.go:145` `ParseComposeYAML` and `lifecycle.go:264` `DeployComposeStack` call fallback parser. After this fix they will both return `!Valid` when `env_file` is present, so divergent `ParsedCompose` shapes no longer mask the error. Keep `parser.go:245` wrapper for `ImportComposeProject` (`parse_compose.go:364`) — its preview may still show `valid` via compose-go; that is okay if the Deploy path now honestly says `error`. For honest preview, route `POST /compose/import` validation through `ValidateCompose` (which it already does at `handlers_compose.go:133`) so preview and deploy agree.

**Phase 2 (future, not in this release): honest `env_file` support.**

If product later wants to support `env_file`, implement via Steps that preserve host confinement:

- When `env_file: ./app.env` is present, resolve `./app.env` relative to a *deployment context directory* (`composeStack.dirForID(stackID)`) not host root. On `DeployFromGit:196` `readComposeFromDir`, copy referenced `env_file` hosts (confirmed inside `repositoryPath`) into `stackDir` before `handleComposeDeploy`, then `docker compose --env-file ./app.env up -d` or rely on `.env` overlay via compose-go `WithEnvFiles` + copy. Until that is implemented, keep the error branch.

**Docs & migration:** Mark `env_file` as `INTENTIONALLY_NOT_SUPPORTED` in `docs/compose-spec.md` with workaround (use `environment:` + `EnvVars`). Add `compose:validate` UI banner explaining the error (see §4).

**No migration SQL** — `compose_stacks.compose_yaml` already contains the raw YAML; no column needed. Existing stacks that were deployed with `env_file: ./x` have a `compose_yaml` on disk that never contained `./x` content. They will continue running until next redeploy, at which point they will be told to inline vars. Safe.

### 3.6 Build context — fail-fast or tarball transfer (P2 MISSING)

**Current:** `compose.go:374` only writes `compose.yaml`+`.env`; `./web/Dockerfile` absent → `409` `ComposeOperationResponse{Error, Output}`.

**Fix — phase 1: fail-fast at validate time (no new infra):**

In `ValidateComposeSecurity:427` (or `ValidateCompose:240` summary), add:

```go
if svc["build"] != nil {
    // rawService.Build is map[string]interface{}; detect any build key
    issues = append(issues, ValidationIssue{
        Field:    fmt.Sprintf("services.%s.build", name),
        Message:  "build: {context,dockerfile,args} is not supported for managed compose deploys — build images locally, push to a registry, then reference by image: name@sha256:<digest>",
        Severity: "error",
    })
}
```

This makes the 409 at deploy time become a clear `400 error` at validate time with actionable guidance, matching `Coolify`/`Dokploy` which reject `build:` or build on a builder.

**Phase 2 (future, guarded feature flag `COMPOSE_BUILD_TARBALL=true`):** Tarball transfer

- Client `Upload` `build.context` tree via existing `daemon.Client` (reuse `HostFilesUpload` pattern `client.go:1477`). Beacon `handleComposeDeploy` streams to `stackDir/build/<service>/context.tar.gz`, `docker load` or `docker compose build` before `up -d`. Panel's `build.Service` (`build/service.go:530` `executeRemoteBuild` clones repo) would be the builder. Out of scope for this plan's P2 — keep phase 1 fail-fast only.

**Frontend:** Show `build:` error inline in compose editor (`validateCompose` result) with link to `dockerImageBuild` tab (`/admin/docker`).

### 3.7 Compose restart — wire native `docker compose restart` vs `Stop+Start`

**Current asymmetry:**

- Beacon `compose.go:506` `handleComposeRestart` is native `docker compose restart` — correct (dep-ordered `PreStop/PostStart` + `waitDependencies` at upstream `restart.go:31`).
- API `lifecycle.go:781` `RestartStack` is `if _, err := s.StopStack(ctx,stackID); err != nil {return nil,err}; return s.StartStack(ctx,stackID)` — two calls, loses restart semantics and races `restart:always` auto-restart.
- `handlers_compose.go:470` `POST /compose/:id/restart` calls `composeSvc.RestartStack` (the wrong one); `daemon/compose.go:81` `ComposeRestart` exists but is unused by that handler.
- `forge/web/lib/api/compose.ts` has no `restartComposeStack` export — admin page uses `fetch` directly.

**Fix:**

```go
// forge/api/internal/services/compose/lifecycle.go:781 — replace Stop+Start
func (s *Service) RestartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
    existing, err := s.store.GetComposeStack(ctx, stackID)
    if err != nil { return nil, ErrStackNotFound }
    stack := fromStoreComposeStack(existing)
    if stack.Status != StackStatusRunning && stack.Status != StackStatusStopped && stack.Status != StackStatusDegraded {
        return nil, ErrStackNotRunnable
    }
    node, err := s.store.GetNode(ctx, stack.NodeID)
    if err != nil { return nil, fmt.Errorf("node not found: %w", err) }
    cred, err := s.store.GetNodeDaemonCredential(ctx, stack.NodeID)
    if err != nil { return nil, fmt.Errorf("node credential not found: %w", err) }
    client := s.getClient()
    // Single native restart — respects docker compose dep-order + hooks:
    op, err := client.ComposeRestart(ctx, node.BaseURL, cred, stackID)
    if err != nil {
        return nil, fmt.Errorf("restart compose stack: %w", err)
    }
    _ = op
    // Optimistically mark awaiting_health, then WaitForHealthy to confirm:
    stack.Status = StackStatusAwaitingHealth
    stack.UpdatedAt = time.Now().UTC()
    _ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
    if err := s.WaitForHealthy(ctx, stackID, node.BaseURL, cred, 2*time.Minute); err != nil {
        stack.Status = StackStatusDegraded
        stack.Error = "health check failed after restart: " + err.Error()
        stack.UpdatedAt = time.Now().UTC()
        _ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
    } else {
        stack.Status = StackStatusRunning
        stack.Error = ""
        stack.UpdatedAt = time.Now().UTC()
        _ = s.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
    }
    if s.publisher != nil {
        _ = s.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeUpdated, "compose", "stack", stackID, map[string]any{"nodeId": stack.NodeID, "action": "restart"}))
    }
    return stack, nil
}
```

No Beacon change — `compose.go:506` handler stays.

**Frontend:** Add `restartComposeStack` export and button (see §4.5):

```ts
export function restartComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
}
```

Wire in `forge/web/app/admin/compose/[id]/page.tsx:26` `ComposeStackDetailPage` alongside stop/start. Existing `handlers_compose.go:470` route already expected `RestartStack` — now it actually restarts natively.

**Backward compat:** Stacks already in `stopped` will be restarted (previously `StopStack` would 409 `ErrStackNotRunnable` on stopped). New `RestartStack` now handles `stopped→running` via native `docker compose restart` (which itself starts stopped services). Document as intentional improvement. Old `Stop→Start` two-phase logs are replaced by one operation.

### 3.8 Resource classification — app-services vs per-stack ceilings (RT-14)

**Two tiers are real and intentional — just undocumented and incompletely enforced:**

- **Tier 1 — game workloads (`mgp-*`)** via `beacon/internal/runtime/docker.go:783` `buildResources` + `827` `buildHostConfigWithSettings` (`CapDrop ALL`, `Privileged false`, `Init true`, `ReadonlyRootfs true`, `SecurityOpt no-new-privileges:seccomp=builtin`, `UsernsMode host` if rootless, `Tmpfs /tmp`, `LogConfig json-file 10m×3` + `ensureNetwork:1065` managed bridge `managed:true`). Cgroup limits from `CreateRequest{MemoryMB, CPUShares, CPUPercent, IOWeight, PIDLimit, OOMKillDisabled}` at `docker.go:151-246` `Create/Reconcile`.
- **Tier 2 — compose workloads** via `compose.go:344` `docker compose up -d` — limits from YAML `deploy.resources.limits` directly via engine, subject only to `validateComposePolicy:77` deny-list (privileged/network_mode/pid/userns/cap_add/devices/security_opt) + new port/memory guard in §3.2/3.3. No `buildHostConfigWithSettings` bridging — compose stacks do not get `ReadonlyRootfs:true` unless in YAML.

**Fix:** Document both tiers side-by-side in `docs/compose-resources.md` tooltip and enforce ceilings per §3.3 totals. No code merges the two tiers — that separation is correct (game workloads need deterministic hardening; compose workloads need spec fidelity). Where compose workloads should inherit hardening, guide operators to add `read_only: true`, `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]` in YAML (add validator warnings if missing).

### 3.9 Multi-runtime dispatch honesty — reject phantom providers (RT phantom)

**Current:** API `multiruntime.go:45` `getRuntimeForTarget` → `defaultRuntime` fallback. `forge/api/cmd/api/main.go:393` registers 6 providers into `MultiRuntimeAdapter`; `Capabilities()` unions them. Beacon `factory.go:16` only supports docker/containerd/podman/firecracker/kubernetes (no `ProviderLXC/KVM`); `server.go:735` `create` drops `Provider` field.

**Fix — API-first rejection (no Beacon change in this step):**

```go
// forge/api/internal/runtime/multiruntime.go — new guard called from placement
// mirroring 1Panel's flat provider registry but honestly.
//
// Before scheduler picks a node, verify the requested provider is actually
// implemented by at least one registered runtime. If not, return 501 with
// a clear message rather than silently falling back to docker.
var (
    ErrUnsupportedProvider = errors.New("unsupported runtime provider")
)

var supportedProviders = map[string]bool{
    ProviderDocker:      true,
    ProviderContainerd:  true,
    ProviderPodman:      true,
    ProviderFirecracker: true,
    ProviderKubernetes:  true,
    // LXC/KVM intentionally NOT supported until beacon factory implements them
    // behind build tags + kernel stubs. Advertising them now is phantom.
}

func isSupportedProvider(p string) bool { return supportedProviders[strings.ToLower(p)] }

// In the scheduler/clustermanager path where Target.Provider is resolved (e.g.
// forge/api/internal/services/placement/* or wherever CreateServer builds Target):
if target.Provider != "" && !isSupportedProvider(target.Provider) {
    return CreateResponse{}, fmt.Errorf("%w: %q is not implemented — supported: docker, containerd, podman, firecracker, kubernetes", ErrUnsupportedProvider, target.Provider)
}
```

For compose specifically, workloads are always Docker (`ComposeDeploy` → `docker compose` shellout) so they do not go through `CreateRequest.Provider`. Ensure compose placement never advertises `lxc/kvm` as eligible.

**Beacon** `server.go:735` `create` handler option for defence-in-depth (future):

```go
if provider := req.Provider; provider != "" && provider != "docker" {
    // Reject rather than drop — honest dispatch. Only after factory.go implements it
    // should this branch accept.
    http.Error(w, fmt.Sprintf("unsupported provider %q", provider), http.StatusNotImplemented)
    return
}
```

Currently Beacon receives `Provider` from `forge/api/internal/daemon/client.go:418` `CreateRequest{Provider}` via `runtime.CreateRequest`. But compose path is shellout, not via `runtime.Create`. So this guard affects game servers only, not compose stacks — correct.

**Frontend:** Node selector (see §4.1) should filter out nodes whose `Provider` would be phantom; show `LXC/KVM (coming soon)` as disabled with tooltip.

### 3.10 Compose lifecycle gaps carried from Phase-06 (C12/C17/C18)

#### 3.10.1 Queue family — already fixed, keep

`handlers_compose.go:315` `POST /compose` already calls `DeployComposeStack` directly, not via `queue_handler.go:10` `ComposeQueueHandler` (which existed but was never enqueued). Keep direct path. Queue handler remains for future schedule/worker (no change, no delete).

#### 3.10.2 Scale — add `POST /compose/:id/scale` (P3 → P2 for this plan's spec diff)

Upstream `docker-compose/pkg/compose/scale.go:27` `ScaleOptions` delegates to `create`. Coolify/Dokploy both expose slider.

**New route:**

```go
// forge/api/internal/http/handlers_compose.go — after restart handler :470
protected.Post("/compose/:id/scale", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
    if cfg.Store == nil { return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required") }
    var req struct {
        Service string `json:"service"` // required
        Replicas int   `json:"replicas"` // required, >=0
    }
    if err := c.BodyParser(&req); err != nil { return fiber.NewError(fiber.StatusBadRequest, "invalid request body") }
    req.Service = strings.TrimSpace(req.Service)
    if req.Service == "" || req.Replicas < 0 { return fiber.NewError(fiber.StatusBadRequest, "service and replicas>=0 required") }
    ctx, cancel := requestContext()
    defer cancel()
    stack, err := composeSvc.ScaleService(ctx, c.Params("id"), req.Service, req.Replicas)
    if err != nil { return respondInternalError(c, err) }
    return c.JSON(stack)
})
```

**Service** `lifecycle.go` new method `ScaleService`:

```go
func (s *Service) ScaleService(ctx context.Context, stackID, service string, replicas int) (*ComposeStack, error) {
    existing, err := s.store.GetComposeStack(ctx, stackID)
    if err != nil { return nil, ErrStackNotFound }
    stack := fromStoreComposeStack(existing)
    // Rewrite deploy.replicas in YAML (use compose-go loader to preserve formatting, or string patch for min viable)
    // Minimal: patch YAML string via yaml.v3 decode → services[service]["deploy"]["replicas"]=replicas → re-encode
    newYAML, err := patchComposeReplicas(stack.ComposeYAML, service, replicas)
    if err != nil { return nil, err }
    // Enforce ceilings on the patched YAML (reuse TotalRequested path):
    parsed, _ := s.ParseComposeYAML([]byte(newYAML), "", nil)
    totalMem, totalCPU, _, _ := TotalRequested(parsed.Services)
    if max(totalMem, stack.MemoryMB) > MaxUserStackMemoryMB || totalCPU > MaxUserStackCPUShares {
        return nil, fmt.Errorf("%w: after scale %s=%d, effective memory %dMB cpuShares %d exceeds ceiling", ErrResourceLimitExceeded, service, replicas, totalMem, totalCPU)
    }
    return s.UpdateComposeStack(ctx, stackID, UpdateComposeRequest{ComposeYAML: newYAML, EnvVars: stack.EnvVars})
}
```

Beacon `compose.go` no new handler — `ScaleService` is just an in-place redeploy with new YAML counts; `docker compose up -d --scale svc=N` is equivalent but requires flags. Keep redeploy path for correctness.

**Frontend:** Compose detail page per-service row with `+ / −` + slider, calling `scaleComposeStack` (see §4.4).

#### 3.10.3 `DeployFromGit` fresh-ID bug — fix one line

`forge/api/internal/services/compose/gitops.go:351/384` currently:

```go
stackID := g.compose.createStackID()
if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
    _, _ = g.store.UpdatePlacementReservationStatus(ctx,reservation.ID,Cancelled)
    return nil, fmt.Errorf("create compose stack record: %w", err)
}
```

Change to:

```go
if err := g.store.CreateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
```

(`CreateComposeStack` in `store_compose.go:110` has `ON CONFLICT (id) DO UPDATE` so calling it is upsert-safe. But correctness requires CREATE semantics on fresh ID; the underlying SQL is already upsert so bug manifests as no error but silent upsert of a non-existent row being treated as update that did insert — still functionally upsert. However the intent is clearly CREATE; switch for clarity and to match `lifecycle.go:390` `CreateComposeStack` path.)

---

## 4. Frontend Plan — Docker / Compose Parity UI

### 4.1 Node / group selector for Docker views

**Files:** `forge/web/lib/api/docker.ts:77`, `forge/web/components/docker/containers-view.tsx:56`, `images-view.tsx:30`, `networks-view.tsx:18`, `volumes-view.tsx:18`, `forge/web/app/admin/docker/page.tsx:14`

**Problem:** `listContainers` fans out `nodeListOrFirst:85` across all nodes — table shows `nodeId/nodeName` flat. No way to target a single node consistently; `docker.ts` already supports `?node=` but UI never exposes it.

**Design:**

- New `NodeSelector` component (`forge/web/components/docker/node-selector.tsx`) — `useQuery(["nodes"])` from existing nodes endpoint (already used for placement). Dropdown: `All nodes` (fan-out) / `<node-name> (<id short>)`. Persisted in `localStorage["docker:nodeFilter"]`.
- `ContainersView` reads `nodeId` from selector and calls `listContainers({all, nodeId})` → API `dockerListContainers` path? Today `dockerListContainers:98` always fans out `nodeListOrFirst`. Need an optional filter: add query param `?node=` handling in `dockerListContainers`:
  ```go
  // handlers_docker.go:98 — add branch:
  func dockerListContainers(cfg Config) fiber.Handler {
      return func(c *fiber.Ctx) error {
          nodeID := c.Query("node")
          var targets []nodeAdminRequest
          if nodeID != "" {
              t, err := resolveSingleNodeTarget(cfg, nodeID)
              if err != nil { return err }
              targets = []nodeAdminRequest{*t}
          } else {
              t, err := nodeListOrFirst(cfg); if err != nil { return err }
              targets = t
          }
          // fan-out unchanged
      }
  }
  ```
  Mirror for `dockerListImages:240`, `dockerListNetworks:340`, `dockerListVolumes:417`.

- Same selector shared across `ImagesView`, `NetworksView`, `VolumesView`, `ComposeStacksPage` (`forge/web/app/admin/compose/page.tsx:32`). Compose already has per-stack `nodeId` but not list filter — add `?node=` to `listComposeStacks` (`store_compose.go:362` `ListComposeStacks(ctx,userID)` filtered by `node_id` when supplied). API `handlers_compose.go:348` `GET /compose` parses `c.Query("node")` and filters in memory (or new store method).

- Group selector (Phase 2, deferred): if `nodes` have `cluster_group_id` / `region_id`, add grouped `<optgroup>`.

**Migrations:** None. Existing fan-out without `?node=` still works (default `All nodes`).

### 4.2 Per-service scale via `POST /compose/:id/scale`

**Backend:** See §3.10.2 new route.

**Frontend:**

- `forge/web/lib/api/compose.ts` — add:
  ```ts
  export function scaleComposeService(id: string, service: string, replicas: number) {
    return postJSON(`/compose/${encodeURIComponent(id)}/scale`, { service, replicas });
  }
  ```

- `forge/web/app/admin/compose/[id]/page.tsx:26` `ComposeStackDetailPage` — each row of `services` table (`ServiceState:107` currently `Name/Image/Status/State/Ports`) gets:
  - `Replicas: number` pill (derived from `parsedConfig` → `ServiceSummary.Deploy.Replicas` or 1 when absent).
  - `−`/`+` stepper + slider (`input[type=range] min 0 max 20`) + `Apply` button. Calls `scaleComposeService` with optimistic update.

- Limits: clamp to `MaxUserStack` ceiling message surfaced from API `ErrResourceLimitExceeded`.

### 4.3 Spec diff preview before Deploy (ComposeUp pull→build→create→up→wait→prune 6-phase)

**Reference:** `komodo/bin/periphery/src/api/compose.rs:414` `ComposeUp` 6-phase — `write_stack → maybe_login_registry → pre_deploy → docker compose config → build → pull → down? → up -d → post_deploy` gated `all_logs_success`, `COMPOSE_COMMAND` wrapper, `last_project_name` rename-aware down.

Forge today does single `up -d` shellout (`compose.go:344`) with implicit pull and no `config` sanitization/build/pre/post hooks.

**This plan does NOT build full Komodo parity — it adds the operator-visible 6-phase preview without the extra infrastructure (registry login via existing `RegistryAuth`, build `fail-fast`, config diff):**

#### Phase preview modal (purely informational before deploy, P1):

- New handler `POST /compose/:id/preview` (or `POST /compose/validate?withPreview=true`) returning:
  ```json
  {
    "valid": true,
    "phases": [
      {"phase":"validate", "status":"ok"},
      {"phase":"config", "command":"docker compose -f <stackDir>/compose.yaml -p <id> config", "output":"... normalized YAML ..."},
      {"phase":"pull", "images":["nginx:alpine","postgres:16"], "strategy":"docker compose pull"},
      {"phase":"build", "blocked":"build: is not supported (see error)"},
      {"phase":"create", "services":["web","db"]},
      {"phase":"up", "command":"docker compose up -d", "orphans": false},
      {"phase":"wait", "timeout":"2m", "poll":"ComposeStatus every 5s"},
      {"phase":"prune", "note":"unused networks/volumes not pruned (opt-in later)"}
    ],
    "diff": {
      "servicesChanged": ["web.image: nginx:1.25 → nginx:1.26", "web.deploy.replicas: 2 → 3"],
      "composeHash": {"from":"ab12...", "to":"cd34..."},
      "previousYamlExcerpt":"..."
    }
  }
  ```

- Derive `diff` via existing `gitops.go:841` `computeServiceDiffs` (compares image/ports/env/volumes/restart/command/dependsOn/replicas) + `composeHash` (`computeHash:988`) already persisted per stack. Images list from `ParsedCompose.Services[].Image`.

- Beacon already has `handleComposePull:709` and `handleComposeStatus:604` but preview does NOT actually pull — it just lists what `pull` would fetch (no side effect).

- UI: `forge/web/app/admin/compose/[id]/page.tsx` Deploy button now opens `SpecDiffModal` showing `Valid + Warnings + diff.servicesChanged + phases list` with `Confirm & Deploy` CTA. Existing `deployComposeStack:89` `POST /compose/:id/deploy` remains the mutating call; preview is read-only.

- For GitOps stacks, preview also shows `GitPreviousCompose` vs new `compose.yaml` diff (reuse `TO_CHAR` already in `scanComposeStack`).

### 4.4 `build:` and `env_file:` honest preview

- `validateCompose:49` `/compose/validate` UI already shows `warnings`. After §3.5/3.6, `build:` and `env_file:` will now appear as `errors` (`!valid`). UI renders them as blocking errors with help text (link to docs/compose-spec.md).

### 4.5 Compose restart button

**Files:** `forge/web/app/admin/compose/[id]/page.tsx:26` (status poll 10s, logs 5s), `handlers_compose.go:470`

- Add method to `compose.ts`:

  ```ts
  export function restartComposeStack(id: string) {
    return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
  }
  ```

- In `ComposeStackDetailPage`, add `Restart` button next to `Stop`/`Start`/`Redeploy`:
  ```tsx
  const restartMut = useMutation({ mutationFn: () => restartComposeStack(id), ... });
  <Button variant="outline" disabled={!["running","stopped","degraded"].includes(stack.status)} onClick={() => restartMut.mutate()}>
    <RotateCw className="mr-2 h-4 w-4" /> Restart
  </Button>
  ```
- Disable condition matches `lifecycle.go:781` `RestartStack` new guard (`running/stopped/degraded`). Tooltip: "Runs `docker compose restart` (dep-ordered, not stop→start)".

### 4.6 `down -v` / `--remove-orphans` opt-in UI

- Delete flow in `compose-view.tsx` / `[id]/page.tsx` delete mutation: existing `deleteComposeStack:71` now accepts options:

  ```ts
  export function deleteComposeStack(id: string, opts?: { volumes?: boolean; removeOrphans?: boolean }) {
    const qs = new URLSearchParams();
    if (opts?.volumes) qs.set("volumes","true");
    if (opts?.removeOrphans) qs.set("removeOrphans","true");
    const suffix = qs.toString() ? `?${qs.toString()}` : "";
    return deleteJSON(`/compose/${encodeURIComponent(id)}${suffix}`);
  }
  ```

- Confirmation modal checkboxes:
  - `[ ] Delete volumes (down -v) — destroys named volumes like pgdata` (unchecked, red warning)
  - `[ ] Remove orphans (--remove-orphans) — removes containers not in new compose file` (unchecked)

### 4.7 Volume prune / image prune reconciliation

- `volumes-view.tsx:44` Eraser button already uses `pruneVolumes:185` `POST /docker/volumes/prune`. Add sibling `images-view.tsx` pruning via new `pruneImages` (§3.1.2) with `X-Confirm-Destructive` handling already at `handleImagePrune:544`.

---

## 5. Regression Tests — Ensure Fixed Chains Stay Wired (T2-T4)

All tests are unit/integration via `httptest` — no live Docker required for T2-T4 wiring, only for E2E `up -d`.

### 5.1 API `handlers_docker_test.go` — route existence (P0)

```go
// forge/api/internal/http/handlers_docker_test.go
package http_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gofiber/fiber/v2"
    apihttp "gamepanel/forge/internal/http"
)

func newTestApp() *fiber.App {
    // Wire a minimal Store=nil + Daemon=stub that returns a fake node list.
    // The stub's adminPostJSON just records that it was called.
    return apihttp.NewTestAppWithStubs() // to be added
}

func TestDockerRoutes_AdminCreate_Mounted(t *testing.T) {
    // Was 404 at beacon/server.go:440 pre-fix; now must be present.
    cases := []struct{ method, path string }{
        {"POST", "/api/admin/containers"},
        {"POST", "/api/admin/networks"},
        {"POST", "/api/admin/volumes"},
        {"POST", "/api/admin/volumes/prune"},
        {"POST", "/api/admin/images/prune"},
    }
    for _, tc := range cases {
        req := httptest.NewRequest(tc.method, tc.path, nil)
        req.Header.Set("X-Panel-Signature","stub") // whatever beacon auth mock needs
        resp, _ := newBeaconTestServer().ServeHTTP(req)
        if resp.StatusCode == http.StatusNotFound {
            t.Fatalf("%s %s got 404 — wiring regressed (was fixed at server.go:478-483)", tc.method, tc.path)
        }
        // 400/401/409 is fine (auth/validation), 404 is not.
        if resp.StatusCode == http.StatusNotFound {
            t.Fatalf("route %s %s still 404", tc.method, tc.path)
        }
    }
}

func TestDockerRoutes_NoExecMounted(t *testing.T) {
    // INTENTIONALLY_NOT_EXPOSED — must remain 404 at the /docker surface.
    app := newTestApp()
    req := httptest.NewRequest("POST","/docker/containers/abc/exec", strings.NewReader(`{"cmd":["ls"]}`))
    req.Header.Set("Content-Type","application/json")
    resp, _ := app.Test(req)
    if resp.StatusCode != http.StatusNotFound {
        t.Fatalf("exec route should be air-gapped (INTENTIONALLY_NOT_EXPOSED), got %d", resp.StatusCode)
    }
}

func TestDockerRoutes_PauseMounted_AfterFix(t *testing.T) {
    // After §3.1.1 this must be 200/4xx, not 404. Before fix it was 404.
    cases := []string{"/api/admin/containers/abc/pause","/api/admin/containers/abc/unpause"}
    for _, path := range cases {
        req := httptest.NewRequest("POST", path, nil)
        resp, _ := newBeaconTestServer().ServeHTTP(req)
        if resp.StatusCode == http.StatusNotFound {
            t.Fatalf("pause/unpause route %s must be mounted (see compose.go wire fix), got 404", path)
        }
    }
}
```

### 5.2 Beacon `container_admin` handler tests

Reuse `beacon/internal/server/server_test.go` helpers:

```go
func TestHandleContainerCreate_StillMounted(t *testing.T) {
    s, _ := NewServer(nil, t.TempDir())
    s.SetTokenGenerator(stubGenerator(infraAdmin=true))
    body := `{"name":"t","image":"alpine:3.21"}`
    req := httptest.NewRequest("POST","/api/admin/containers", strings.NewReader(body))
    signRequest(req, s.token)
    w := httptest.NewRecorder()
    s.ServeHTTP(w, req)
    // 201 or 500 with stub client is fine; 404 is not.
    if w.Code == http.StatusNotFound { t.Fatalf("POST /api/admin/containers regressed to 404") }
}
```

### 5.3 Compose policy unit tests (no Docker)

`beacon/internal/server/compose_policy_test.go` — the `shortFormHostPort` and `validateComposeVolumes` regressions:

```go
func TestShortFormHostPort_FixesPrivilegedBypass(t *testing.T) { /* §3.2 matrix */ }
func TestValidateComposePolicy_EnvFileRejected(t *testing.T) {
    yaml := "services:\n  web:\n    image: nginx\n    env_file: ./app.env\n"
    if err := validateComposePolicy(yaml); err == nil { t.Fatalf("env_file must be rejected per §3.5") }
}
func TestValidateComposePolicy_BuildRejected(t *testing.T) { ... }
func TestValidateHostMountWithAllowlist_BetweenHops(t *testing.T) {
    // API ValidateHostMountWithAllowlist("...") and beacon validateHostMount must agree
}
```

### 5.4 Per-replica quota test

`forge/api/internal/services/compose/quota_test.go`:

```go
func TestTotalRequested_ReplicasMultiplied(t *testing.T) {
    yaml := `
services:
  api:
    image: nginx
    deploy: {replicas: 10, resources: {limits: {memory: 8G}}}
  worker:
    image: nginx
    deploy: {replicas: 2, resources: {limits: {memory: 512M}}}
`
    svc := &Service{}
    parsed, _ := svc.ParseComposeYAML([]byte(yaml), "", nil)
    totalMem, _, totalReps, err := TotalRequested(parsed.Services)
    if err != nil { t.Fatal(err) }
    want := 10*8192 + 2*512
    if totalMem != int64(want) { t.Fatalf("want %d got %d", want, totalMem) }
    if totalReps != 12 { t.Fatalf("want 12 got %d", totalReps) }
    // 10*8G = 80G > 64G ceiling → Deploy must error
}
```

### 5.5 Frontend E2E (Playwright)

- `e2e/docker-admin.spec.ts` — visit `/admin/docker`, assert `Pause` button triggers `POST /docker/containers/:id/operate {action:"pause"}` and receives `200 {status:"ok"}` (mock Beacon with in-memory docker client).
- `e2e/compose-preview.spec.ts` — paste `ports: ["80"]` YAML, assert validate shows privileged port error; paste `env_file:` YAML, assert `error` blocking deploy; click Deploy preview modal shows diff.

---

## 6. Migrations & Schema — No Breaking Changes

### 6.1 No required schema migration for core fixes

- **ShortFormHostPort, volume allowlist, env_file fail-fast, build fail-fast, image prune, pause/unpause** — no new columns. `compose_stacks.compose_yaml` already carries the YAML; `compose_stacks.compose_hash` (`computeHash:988`) carries hash; `env_vars` + `env_vars_encrypted` already carry env; `git_previous_compose_yaml` carries rollback YAML. All reads via `composeStackScanExpr:90`/`composeStackCols:75` unchanged.

- **Per-replica ceiling** — derived at placement time; no `total_memory_mb` column. If auditing is desired, add a nullable `effective_memory_mb` later, but not needed for correctness.

- **Restart native switch** — no column. Status enum `StackStatusRunning/AwaitingHealth/Degraded` (`lifecycle.go:24-35`) already covers restart observation.

### 6.2 Optional additive migration — `scale_generation` for optimistic concurrency

If `ScaleService` concurrent calls matter, add one nullable column for optimistic locking so two scale requests don't race:

```sql
-- migrations/161_add_compose_scale_generation.sql
-- Subagent-05: additive, nullable, no backfill required. Existing stacks keep NULL.
ALTER TABLE compose_stacks
  ADD COLUMN IF NOT EXISTS scale_generation INTEGER NOT NULL DEFAULT 0;

-- Scale operation should WHERE id=$1 AND scale_generation=$expected
-- then SET scale_generation = scale_generation+1 RETURNING ...
-- If no row returned, concurrent update — surface 409 Conflict to caller to retry.
```

No `down.sql` required (keep column if rolled back — nullable default hides it). `scanComposeStack:195` / `toStoreComposeStack:894` mapping unchanged if omitted — `scale_generation` can be kept out of scan until feature needs it.

### 6.3 `DeleteComposeStack` `down -v` flag — no column

Query param `?volumes=true` is ephemeral per request; do not persist a `remove_volumes` column. Operators who want periodic pruning should use `pruneVolumes`/`pruneImages` explicitly.

### 6.4 Node `allowed_mounts` plumbing — no new column

Node allowed mounts already stored via `server.SetAllowedMounts` (`server.go:129` `allowedMounts []string`) and `allowedMountSources:135` with `sync.RWMutex`. API can fetch via existing node metadata (join with node table or in-memory capability heartbeat `handleGetCapabilitiesDelta:400`). If not persisted per-node, add ephemeral mapping via the heartbeat snapshot — no migration.

---

## 7. Rollout Plan — Strangler by Path, Not Flag Day

| Week | Slice | Files | Risk | Verification |
|---|---|---|---|---|
| **W1 P0** | Beacon privileged port fix + unit tests | `beacon/internal/server/compose.go:214/181` | Low — single parser, no API shape change. Grandfather existing stacks (no retroactive fail). | `go test ./beacon/internal/server -run TestShortFormHostPort` + manual `docker compose config` matrix in §3.2 |
| **W1 P0** | `env_file` + `build:` fail-fast at API + Beacon `validateComposePolicy` | `service.go:92/427` `rawService` + `compose.go:77`, `handlers_compose.go:315` path NOT changed | Low-medium — previously silent, now 400. Phase 1 error messaging only; allow header `X-Allow-Unsupported-Env-File` for 30 days if needed (defer adding header if no broken stacks found in audit). | `go test ./forge/internal/services/compose -run TestValidateEnvFileRejected`, `TestValidateBuildRejected` |
| **W1 P1** | Volume allowlist unification (plumb `allowedMounts+isAdmin`) + `down -v` opt-in | `daemon/compose.go:13` `ComposeDeployRequest`, `compose.go:226/344/550`, `lifecycle.go:396/533`, `handlers_compose.go:401`, `mounts.go:64` predicate | Medium — cross-service contract change (API↔Daemon↔Beacon). But backward compat: Beacon wrapper `validateComposeVolumes(...,nil,false)` keeps old behavior for callers without the new fields (old images continue to reject all host binds). | Manual: `POST /compose/validate` with `/data` as allowed vs not; `DELETE /compose/:id` without `?volumes` leaves volumes behind (verify via `docker volume ls`). |
| **W1 P1** | Per-replica ceiling `TotalRequested` | `lifecycle.go:280/293/328`, new `quota.go` | Low — derive from YAML at placement; if parser fails, fail closed with `ErrInvalidCompose` already. Log `effectiveMem` in span. | `go test -run TestTotalRequested_ReplicasMultiplied` + integration `DeployComposeStack` with `replicas:10 × 8G` must 400. |
| **W2 P1** | `pause`/`unpause` handler + image prune wiring | `container_admin.go:193/276`, `server.go:459-478`, `handlers_docker.go:63`, `client.go:1845` reuse, `forge/web/lib/api/docker.ts:147 + images-view.tsx:30` | Low — additive routes; old `operateContainer("pause")` callers now succeed instead of 404. No removal of start/stop/restart. | Playwright `docker-admin.spec.ts` + `handlers_docker_test.go` `TestDockerRoutes_PauseMounted` |
| **W2 P2** | Container create lossy fix (ports/network/subnet IPAM) | `container_admin.go:1776/1824`, `docker.go:743/865` helpers | Medium — touches `ContainerCreate` HostConfig. Keep nil HostConfig fallback for old callers; add validation for docker.sock. | E2E: create container with `8080:80` then `docker inspect` shows port binding; create network with subnet then `docker network inspect` shows IPAM. |
| **W2 P2** | `RestartStack` → native `ComposeRestart` | `lifecycle.go:781`, `daemon/compose.go:81` | Low — replace Stop+Start with single call; keep health gate `WaitForHealthy:188`. | Manual `docker compose ps` after restart shows same project, no orphan; logs `restart` event not `stop+start`. |
| **W2 P2** | Frontend node selector + restart button + scale endpoint + diff preview + `down -v` checkboxes | `forge/web/lib/api/docker.ts`, `compose.ts`, `containers-view.tsx:56`, `compose/[id]/page.tsx:26`, `handlers_compose.go:470`, `compose_service scale` | Low — UI only; scale is just patched redeploy (reuses Update path). | Playwright `compose-preview.spec.ts` + `docker-admin.spec.ts` |
| **W3** | `DeployFromGit` fresh-ID bug + scale generation optional migration | `gitops.go:384`, `store_compose.go:110` | Low — single line fix; idempotent because `CreateComposeStack` is upsert. | Integration `TestDeployFromGit_CreatesStack` |

**Feature flag strategy:** Only `build tarball transfer` gets a flag (`COMPOSE_BUILD_TARBALL`). Everything above ships without flags — behavior changes are either additive routes or honester errors that were previously silent wrong behavior (`env_file` drop, privileged port bypass, `down -v` wipe). Announce the two error promotions (`env_file`, `build`) 7 days ahead via `/compose/validate` warnings already visible today.

---

## 8. Backward Compatibility Matrix — "No Breaking Existing Stacks" Checklist

| Existing stack characteristic | Before this plan | After this plan | Action required on next redeploy | Notes |
|---|---|---|---|---|
| `ports: ["8080:80"]` non-privileged | `up -d` ok | `up -d` ok | none | No change |
| `ports: ["80"]` (privileged) already deployed | `up -d` ok (bypass) | Still running; `GET /status` not retroactively failed | Must change to `8080:80` or front with proxy before next `redeploy` | Logged as warning by `ListComposeStacksForReconciliation` sweep |
| `volumes: ["/opt/appdata:/data"]` with `allowed_mounts=/opt/appdata` | `POST /compose/validate` warning → `400 policy violation` at deploy (bifurcation) | Both agree: `valid` when allowlisted, `400` when not | If allowlisted, nothing; if not, add to `allowed_mounts` or use named volume | Unified predicate §3.4 |
| `volumes: ["/etc:/host-etc:ro"]` | API warning, Beacon `400` | Both `error` when allowlist denies `/etc` | Must move to named volume or request allowlist exception | No data loss — redeploy never deletes `/etc` |
| `env_file: ./app.env` | `up -d` succeeds, env missing → app crash | `POST /compose/validate` now `error`; `POST /compose` → `400` with doc link | Inline vars into `environment:` or `EnvVars` map | Old stack continues running until redeploy |
| `build: {context: ./web}` | `409 ComposeOperationResponse{Error, Output}` opaque | `POST /compose/validate` → `error: build: not supported — push image then image: …@sha256:…` | Build + push outside Forge, then `image:` digest-pin | Old stack continues |
| `deploy.replicas: 10` × `memory: 8G` | `Deploy` succeeds, host OOM after `up` | `400 ErrResourceLimitExceeded` before reservation | Reduce replicas or request per-stack ceiling increase from infra admin | Existing running stacks not killed; new placements blocked |
| `down` (DELETE) before with implicit `-v` | Volumes wiped | `DELETE /compose/:id` without `?volumes=true` now leaves volumes (safe) | Operators who relied on wipe must add `?volumes=true` or use explicit prune | Communicate in release notes; audit volumes for stacks deleted in last 30 days |
| `pause`/`unpause` | `502 Bad Gateway` (404 at Beacon) | `200 {status:"ok"}` | none | Old UI buttons now work |
| `providers: lxc/kvm` scheduled | Silently ran as Docker | `501 Unsupported provider` before placement | Use `docker` provider until `factory.go:16` implements LXC/KVM | Phantom eliminated |

---

## 9. Detailed Task List for Implementers (with file:line)

| # | Task | File:line (edit) | Type | Depends on |
|---|---|---|---|---|
| 1 | **Regression guards** `handlers_docker_test.go` + beacon `server_test.go` to freeze T2-T4 wired routes | `beacon/internal/server/server.go:478-483`, `forge/api/internal/http/handlers_docker.go:30-63` | test | — |
| 2 | **Fix `shortFormHostPort`** + `validateComposePorts` backwards-compatible strip/empty check | `beacon/internal/server/compose.go:212-224` + `beacon/internal/server/compose.go:181-209` + `forge/api/internal/services/compose/service.go:427` mirror | fix | #1 |
| 3 | **Per-replica quota** `TotalRequested` helper + wiring in `DeployComposeStack/UpdateComposeStack/ScaleService` | `forge/api/internal/services/compose/lifecycle.go:150/273-328` + new `quota.go` + `parser.go:72 Resources` | fix | #1 |
| 4 | **Volume allowlist unify** — plumb `allowedMounts+isAdmin` through `ComposeDeployRequest` | `forge/api/internal/daemon/compose.go:13` + `lifecycle.go:396`, `beacon/internal/server/compose.go:226/283/344/550`, `beacon/internal/server/mounts.go:64` | fix | #1 |
| 5 | **`down -v` opt-in + `--remove-orphans` query** | `beacon/internal/server/compose.go:550-578`, `forge/api/internal/daemon/compose.go:85`, `forge/api/internal/http/handlers_compose.go:401`, `lifecycle.go:570` | fix | #4 |
| 6 | **`env_file` / `build:` fail-fast** at both hops | `forge/api/internal/services/compose/service.go:92/427` + `beacon/internal/server/compose.go:77` | fix | #1 |
| 7 | **`pause`/`unpause` handler** + audit | `beacon/internal/server/container_admin.go:193-273` + `server.go:459-462` | fix | #1 |
| 8 | **Image prune Docker-admin wire** | `forge/api/internal/http/handlers_docker.go:60-63` + `forge/web/lib/api/docker.ts:147` | fix | #1 |
| 9 | **Lossy create/network fix** — `ports/network/restartPolicy` + `subnet` IPAM | `beacon/internal/server/container_admin.go:1776/1824` | fix | #1 |
|10 | **`RestartStack` → `ComposeRestart`** | `forge/api/internal/services/compose/lifecycle.go:781`, `forge/api/internal/daemon/compose.go:81` | fix | #1 |
|11 | **`DeployFromGit` fresh-ID bug** | `forge/api/internal/services/compose/gitops.go:384` → `CreateComposeStack` | fix | #1 |
|12 | **Frontend: node selector + docker fan-out filter** | `handlers_docker.go:98/240/340/417`, `forge/web/components/docker/node-selector.tsx` | feat | #7,#8 |
|13 | **Frontend: compose restart + scale + spec diff preview modal + `down -v` checkboxes** | `forge/web/lib/api/compose.ts`, `forge/web/app/admin/compose/[id]/page.tsx:26`, new `handlers_compose.go` `/compose/:id/scale` + `/preview`, `lifecycle.go` `ScaleService` | feat | #3,#5,#10 |
|14 | **Docs — `INTENTIONALLY_NOT_EXPOSED` for exec + `INTENTIONALLY_NOT_SUPPORTED` for env_file/build** | `docs/compose-spec.md`, `docs/runtime-tiers.md` (new) | docs | #6 |
|15 | **Optional migration `scale_generation`** | `migrations/161_add_compose_scale_generation.sql` | migration | #13 |

---

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `env_file`/`build` fail-fast breaks stacks that unknowingly relied on silent drop | Medium — some operators duplicated vars into `environment:` so drop was benign | P0 noise | Provide 30-day `X-Allow-Unsupported-*` header for redeploy only (not new deploys); keep old stacks running until redeploy; validate sweep shows how many stacks have `env_file` (grep `compose_yaml`) before merge |
| Volume allowlist plumbing changes `ComposeDeployRequest` shape — old Beacon without new fields | Low — Deploy path gracefully handles absent `allowedMounts` (wrapper `validateComposeVolumes(nil,false)` preserves old blanket-reject) | Medium | Make Beacon handler backward compat: if `req.AllowedMounts` nil and caller is non-admin, reject all host binds (old behavior) so old panels don't accidentally permit. Add version negotiation later via `X-Beacon-Version` already at `client.go:388` |
| Per-replica totals false-positives for stacks with `deploy.resources` expressed as `memory: 512m` edge cases | Medium | Low — ceil check only; false positive blocks deploy | Parser tolerant: `parseMemoryToMB` handles `M/MiB/G/GiB/T` + decimal bytes; if unparseable, return `ErrInvalidCompose` with `field: services.<name>.deploy.resources.limits.memory` so operator fixes YAML |
| Pause semantics differ Docker engine vs manager (swarm `pause` only on single host) | Low | Low | Document that `pause`/`unpause` targets only hosts from `nodeListOrFirst` single node; `All nodes` pause is not supported (return 422) |
| `compose restart` vs `stop→start` change for `restart:always` stacks | Medium | P2 Flap fix, not regression | Keep `WaitForHealthy:188` 2m after native restart; add log `restart.native=true` vs old two-phase for audit |

---

## 11. Verification — Before/After Gates

### 11.1 Must pass before merge

- `go test ./beacon/internal/server -run TestShortFormHostPort -count=1 -v` green
- `go test ./beacon/internal/server -run TestValidateCompose -v` rejects `"80"` as privileged, rejects `env_file:`, rejects `build:`
- `go test ./forge/api/internal/services/compose -run TestTotalRequested` 10×8G > ceiling → 400
- `go test ./forge/api/internal/http -run TestDockerRoutes` coverage for `pause/unpause` mounted, `pruneImages` mounted, exec remains air-gapped
- `./scripts/check-compose-spec.sh` (if exists) shows `env_file` not in `compose.yaml` Golden files
- Manual beacon sign test: `curl -H "X-Panel-Signature: ..." POST /api/admin/containers/<id>/pause` returns `200 {status:"ok"}` not 404 (mock)

### 11.2 Must pass after merge (staging)

- Existing compose stacks `GET /compose/:id/status` + `ps --format json` still returns `services{Name,Image,Status,State,Ports}` (`compose.go:604`) with same project name `stackID`
- New deploy with `ports: ["80"]` → `400 compose policy violation: privileged host port 80`
- New deploy with `/opt/appdata` bind → `200 valid` when `allowed_mounts` includes it, `400 policy violation` when not (both hops agree)
- `DELETE /compose/:id` leaves volumes behind unless `?volumes=true` (`docker volume ls | grep <project>` still present)
- `POST /compose/:id/restart` does `docker compose restart` (single log line, not two `stop`+`start`)
- `POST /compose/:id/scale {service:"web",replicas:3}` → `docker compose ps` shows 3 containers (or redeploys with `replicas=3` patch; observe via `compose.go` ps)
- `DeployFromGit` with fresh repo creates `compose_stacks` row and is `running` after `WaitForHealthy:416`
- `POST /docker/images/prune` under `/docker` returns `200` per node (same as `/portainer/images/prune`)
- `POST /docker/containers/:id/operate {action:"pause"}` + `unpause` succeed for `running`/`paused` states

---

## 12. Open Questions (Resolved for This Plan)

- **Q: Should `pause`/`unpause` be `INTENTIONALLY_NOT_EXPOSED` like `exec`?** A: No — see §3.1.1 decision. `exec` is intentionally air-gapped (allowlist footnotes); `pause` is standard Docker lifecycle with no allowlist baggage. Wire it.

- **Q: Should Beacon enforce per-replica ceilings too?** A: Not in this step. Panel `lifecycle.go:280` authoritative before reservation; Beacon `validateComposePolicy` is defense-in-depth but its `validateComposePolicy` today has no `MemoryMB` view beyond YAML. Add beacon guard later if needed.

- **Q: Tarball transfer for `build:` now?** A: Not in this release. Provide honest error (Phase 1) not half-tarball; build on registry side instead. Full 6-phase Komodo parity (`config → build → pull → up`) deferred.

- **Q: Migration SQL for `scale_generation`?** A: Provided as optional additive `161_...sql` (§6.2). Can ship after scale endpoint proves contention in prod.

---

## 13. References — File:Line Index (current checkout, authoritative)

- `beacon/internal/server/compose.go:59` `validStackID` `^[a-z0-9][a-z0-9_-]{0,127}$` — keep
- `beacon/internal/server/compose.go:77` `validateComposePolicy` deny-list — extend with `env_file`/`build`/`restart`/`extends` branches
- `beacon/internal/server/compose.go:181` `validateComposePorts` — fix (see §3.2)
- `beacon/internal/server/compose.go:212-224` `shortFormHostPort` — fix (`case 1` + protocol strip)
- `beacon/internal/server/compose.go:226` `validateComposeVolumes` — replace via `validateComposeVolumesWithAllowlist`
- `beacon/internal/server/compose.go:264` `encodeComposeEnv` — keep (used with `ComposeDeployRequest.EnvVars`)
- `beacon/internal/server/compose.go:283` `dirForID` — keep (`<dataDir>/../compose/<stackID>`)
- `beacon/internal/server/compose.go:344` `handleComposeDeploy` single `up -d` — keep, extend with allowlist param
- `beacon/internal/server/compose.go:506` `handleComposeRestart` native — keep, make API use it
- `beacon/internal/server/compose.go:550-578` `handleComposeDelete` hard-coded `down -v` — change to opt-in `volumes` + `removeOrphans`
- `beacon/internal/server/compose.go:604` `handleComposeStatus` `ps --format json` line-delimited — keep, add `scale` handling via patch path
- `beacon/internal/server/compose.go:661` `handleComposeLogs` — keep; add `Since/Until/Timestamps` later
- `beacon/internal/server/container_admin.go:29` `handleContainerList` — keep (managed-filter)
- `beacon/internal/server/container_admin.go:193` `adminContainerAction` — extend with pause/unpause
- `beacon/internal/server/container_admin.go:276` `handleContainerDelete` — keep
- `beacon/internal/server/container_admin.go:531` `handleImagePrune` — keep
- `beacon/internal/server/container_admin.go:792` `handleContainerExec` — keep air-gapped (see §1.4)
- `beacon/internal/server/container_admin.go:995` `handleContainerStats` — keep
- `beacon/internal/server/container_admin.go:1776` `handleContainerCreate` — extend (see §3.1.3)
- `beacon/internal/server/container_admin.go:1824` `handleNetworkCreate` — extend IPAM
- `beacon/internal/server/container_admin.go:1896/1961` `handleVolumeCreate/Prune` — keep
- `beacon/internal/server/server.go:408-489` multiplex — keep; add `pause/unpause` mount at `459-463` order
- `beacon/internal/server/server.go:478` `POST /api/admin/containers` — keep (FIXED)
- `beacon/internal/server/server.go:479/480` networks — keep
- `beacon/internal/server/server.go:481-483` volumes/prune — keep
- `beacon/internal/server/server.go:735` `create` drops `Provider` — add guard for future
- `beacon/internal/server/mounts.go:64` `allowedMountSource` — keep as source of truth for game; reuse predicate for compose
- `beacon/internal/runtime/docker.go:783` `buildResources` + `827` `buildHostConfigWithSettings` — keep game tier 1; compose bypass correct
- `forge/api/internal/daemon/client.go:1718/1726/1731/1756/1760/1764/1785` container Admin* — keep
- `forge/api/internal/daemon/client.go:1875/1879` `AdminContainerPause/Unpause` — keep (now hits handler)
- `forge/api/internal/daemon/client.go:1883` `AdminContainerCreate` — keep
- `forge/api/internal/daemon/client.go:2038/2047` networks — keep
- `forge/api/internal/daemon/client.go:2064/2073/2090` volumes — keep
- `forge/api/internal/daemon/client.go:1845` `AdminImagePrune` — keep, wire API route
- `forge/api/internal/daemon/client.go:2095` `AdminContainerExec` — keep air-gapped at API
- `forge/api/internal/daemon/compose.go:13` `ComposeDeployRequest` — extend with `AllowedMounts+IsAdmin+RemoveOrphans`
- `forge/api/internal/daemon/compose.go:48/73-86/81` `ComposeDeploy/Stop/Start/Restart/Delete/Pull/Status/Logs` — keep; fix `RestartStack` to use `:81`
- `forge/api/internal/http/handlers_docker.go:25-63` `registerDockerRoutes` — add `images/prune`, keep `volumes/prune`, keep `containers/operate` switch `pause/unpause`
- `forge/api/internal/http/handlers_docker.go:136` `dockerCreateContainer` — keep (forwards `map[string]any` verbatim)
- `forge/api/internal/http/handlers_docker.go:176` `pause/unpause` — keep (already forwards)
- `forge/api/internal/http/handlers_docker.go:667` `dockerPruneVolumes` — keep; add `dockerPruneImages` mirror
- `forge/api/internal/http/handlers_compose.go:66/315/413/440/453/470` compose routes — keep; fix `470` RestartStack→native, add `/scale` + `/preview`
- `forge/api/internal/services/compose/service.go:14` `MaxComposeYAMLBytes=1MB` — keep
- `forge/api/internal/services/compose/service.go:92` `rawService` — extend with `EnvFile`
- `forge/api/internal/services/compose/service.go:145/264` fallback parser — keep hot path, but add env_file error branch
- `forge/api/internal/services/compose/service.go:427` `ValidateComposeSecurity` — extend port/env_file/build checks
- `forge/api/internal/services/compose/service.go:595` `checkVolumesSecurity` — keep; add error for `/etc` already there
- `forge/api/internal/services/compose/service.go:663` `ValidateHostMountWithAllowlist` — keep and plumb
- `forge/api/internal/services/compose/service.go:765` `normalizeDependsOn` — keep (condition discarded today; engine still respects)
- `forge/api/internal/services/compose/service.go:841` `normalizeDeploy` — keep; feed quota via `Deploy.Replicas`
- `forge/api/internal/services/compose/parser.go:245` loader wrapper vs `service.go:145` fallback — divergence noted; Phase 1 parity by making both error on env_file/build
- `forge/api/internal/services/compose/parser.go:913` `normalizeResources` — keep but expand `Pids/Blkio` later
- `forge/api/internal/services/compose/lifecycle.go:139-152` ceilings + `MaxUserStacks=20` — keep; extend `280` via `TotalRequested`
- `forge/api/internal/services/compose/lifecycle.go:188` `WaitForHealthy` 5s ticker 2m — keep
- `forge/api/internal/services/compose/lifecycle.go:249/476` `Deploy/Update` — keep; add replica×limits check + allowedMounts pass
- `forge/api/internal/services/compose/lifecycle.go:570` `DeleteComposeStack` — add opt-in volumes
- `forge/api/internal/services/compose/lifecycle.go:781` `RestartStack` Stop+Start — replace with native `ComposeRestart`
- `forge/api/internal/services/compose/gitops.go:196/287/351/384` `DeployFromGit` fresh-ID bug — fix `Update→Create`
- `forge/api/internal/store/store_compose.go:75/90/110/191` `composeStackCols/scanExpr/CreateComposeStack=upsert` — keep; optional additive `scale_generation`
- `forge/api/internal/runtime/multiruntime.go:12/45` `MultiRuntimeAdapter/getRuntimeForTarget` — add unsupported provider guard
- `beacon/internal/runtime/factory.go:16` `Factory.CreateRuntime` — keep (LXC/KVM not implemented)
- `forge/web/lib/api/docker.ts:77/113/118/147/159/172/185/189/207` — keep; add `pruneImages`, `node` query param
- `forge/web/lib/api/compose.ts:49/53/89` — keep; add `restartComposeStack`, `scaleComposeService`, `validateWithPreview`
- `forge/web/app/admin/docker/page.tsx:14` + `components/docker/{containers,images,networks,volumes}-view.tsx` — add node selector
- `forge/web/app/admin/compose/page.tsx:32` + `compose/[id]/page.tsx:26` — add restart button, scale slider, preview modal, down-v checkboxes

---

## 14. Appendix — Example SQL (only if scale generation adopted)

```sql
-- audits/implementation-plan/migrations/161_add_compose_scale_generation.sql
-- Subagent-05 optional — additive, nullable+default, no backfill
BEGIN;

ALTER TABLE compose_stacks
  ADD COLUMN IF NOT EXISTS scale_generation integer NOT NULL DEFAULT 0;

-- If you later need optimistic locking for ScaleService:
-- UPDATE compose_stacks SET compose_yaml=$2, compose_hash=$3, scale_generation=scale_generation+1
-- WHERE id=$1 AND scale_generation=$expected RETURNING *

COMMENT ON COLUMN compose_stacks.scale_generation IS
  'Monotonic generation for scale mutations — optimistic concurrency for POST /compose/:id/scale (subagent-05). NULL-safe default 0.';

COMMIT;
```

For all other changes in this plan **no migration is required** — existing `compose_stacks` rows remain valid, and the `compose_projects` legacy import path (`handlers_compose.go:118` `POST /compose/import`) is unchanged except for honester `ValidateCompose` errors at preview time.
