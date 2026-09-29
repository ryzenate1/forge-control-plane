# Subagent 10 — Docker-Compose Spec Fidelity + Coolify/Dokploy Build Systems vs Forge Compose & Build

**Scope:** Docker Compose spec (`reference/app-platforms/docker-compose` pkg/compose/*), Coolify/Dokploy compose/network/volume + builder helpers vs Forge `forge/api/internal/services/compose/*`, `beacon/internal/server/compose.go`, `forge/api/internal/services/build/service.go`, `daemon/compose.go`, `beacon/internal/runtime/docker.go`. Verify end-to-end UI→API→queue→Beacon→docker compose including env_file, depends_on, profiles, extends, secrets, scaling.

**Date:** 2026-08-23
**Auditor:** Subagent 10 (Phase 6 parallel)

---

## 1. Execution Path Summary (Forge)

```
forge/web/components/app/compose-view.tsx  (read-only status view)
  → forge/web/lib/api/compose.ts  (frontend API client - not fully inspected but routes exposed)
  → forge/api/internal/http/handlers_compose.go:66  (registerComposeRoutes)
      POST /compose               → compose.Service.DeployComposeStack
      PUT  /compose/:id           → compose.Service.UpdateComposeStack
      DELETE /compose/:id         → compose.Service.DeleteComposeStack
      POST /compose/:id/start|stop|restart → Start/Stop/RestartStack
      GET  /compose/:id/status|logs       → GetStackStatus/Logs
      POST /compose/validate|import       → ValidateCompose / CreateComposeProject
      POST /compose/git/*                 → GitOpsService
      POST /compose/webhook/:webhookId    → GitOpsService.HandleWebhook
  → forge/api/internal/services/compose/lifecycle.go  (Deploy/Update/Delete + WaitForHealthy)
  → forge/api/internal/services/compose/queue_handler.go:10  (HandleDeploy/Update/Delete/Start/Stop/Restart)
  → forge/api/internal/daemon/compose.go:13  (HTTP client to Beacon)
  → beacon/internal/server/compose.go:344  (handleComposeDeploy/Stop/Start/Restart/Delete/Status/Logs/Pull)
      → exec.CommandContext("docker", "compose", "-f", composePath, "-p", stackId, ...)
  ← status polling via beacon ps --format json
```

Build path:
```
handlers_builds.go → build.Service.StartBuild (BuildOptions)
  → executeRemoteBuild → daemon.Client.DockerfileBuild / NixpacksBuild
  → beacon/internal/server/build.go:79/193 (handleDockerfileBuild / handleNixpacksBuild)
      → docker buildx build / nixpacks build (beacon local exec)
  → daemon.InspectImageDigest / PushImage
```

---

## 2. Detailed Comparisons (>=12)

### C01 — YAML Loading: Upstream `cli.With*` vs Forge dual parsers

- **Upstream (`reference/app-platforms/docker-compose/pkg/compose/loader.go:36-68` + `cmd/compose/compose.go:79`):** `cli.NewProjectOptions` chains `WithWorkingDirectory`, `WithOsEnv`, `WithEnvFiles`, `WithDotEnv`, `WithConfigFileEnv`, `WithDefaultConfigPath`, `WithName`, plus `RemoteLoader` (Git remote, OCI remote). Env resolution via `envResolver()` (`pkg/compose/envresolver.go:30`) which is case-insensitive on Windows. Loader merges multiple `ConfigFiles`, resolves `env_file`, interpolates `${VAR:-default}` via `compose-go`'s `loader`+`interpolate`, handles `.env` overlay, `COMPOSE_FILE` env, remote includes (with confirmation prompt in `options.go:277`).

- **Forge fallback parser (`forge/api/internal/services/compose/service.go:81-136` `rawCompose`/`rawService`):** Custom YAML structs with only ~13 service keys (`image, build, ports, environment, volumes, depends_on, profiles, restart, command, entrypoint, healthcheck, deploy, secrets, configs`). YAML unmarshal directly; env interpolation via bespoke `interpolateEnv()` (`service.go:356`). No `.env` file reading, no `COMPOSE_FILE`, no `WorkingDir` relative path resolution (except fallback project name from `filepath.Base(workingDir)` at `service.go:161`). `env_file` field absent from `rawService` — silently dropped.

- **Forge compose-go wrapper (`forge/api/internal/services/compose/parser.go:245-265` `ParseComposeYAML` and `ComposeParser.ParseComposeString`):** Uses `compose-spec/compose-go/v2/loader` correctly for that path, but `ValidateCompose`/`DeployComposeStack` **only** call the fallback `ParseComposeYAML(content, workingDir, nil)` (`service.go:145`, `lifecycle.go:264`), not the compose-go loader. Therefore the compliant parser exists but is not on the deploy hot path. `ImportComposeProject`/`GetImportPreview` do call `parser.ParseComposeString` (`import.go:96`, `import.go:353`), creating divergent validation between import and deploy.

**Fidelity gap:** Deploy path never benefits from compose-go's full spec handling (env_file, extends, interpolation completeness, .env). Import preview may show “valid” while deploy's simpler parser accepts/rejects differently.

---

### C02 — `env_file` Support

- **Upstream:** First-class at service level (`types.ServiceConfig.EnvFiles`), at project level via `cli.WithDotEnv`/`WithEnvFiles`. `cmd/compose/compose.go:92` notes `raw` format for `env_file` parsing (same parser as `docker run --env-file`). Variables from `env_file` merged before environment override.
- **Forge `service.go:92-107`:** `rawService` has no `EnvFile` field. `rawInclude` has an `EnvFile` but only for `include:` top-level (`service.go:134`). `ValidateComposeSecurity` never checks `env_file` either. Consequently `env_file: ./app.env` is silently ignored — no error, no inclusion. Preview parser (`parser.go`) via compose-go **does** understand `env_file`, so import will list no warning while deploy silently omits vars.
- **Beacon `compose.go:380-390`:** Deployment writes `Req.EnvVars` to `.env` file via `encodeComposeEnv`. It never processes `env_file` entries from original YAML, nor does it mount host env files. No validation for `env_file:` keys at beacon either (`validateComposePolicy` only checks privileged/network/mount/ports).

---

### C03 — `extends` (Service Inheritance)

- **Upstream:** `cmd/compose/compose.go:318` (`case "extends":`) and `compose-go` supports `extends: {service, file}`. The `extends` resolution happens inside `loader` before model finalization; tracing counts it (`internal/tracing/attributes.go:92` `CountExtends`).
- **Forge `service.go:92-107` + `parser.go` structs:** Neither `rawService` nor `ForgeServiceConfig` contain `extends`. Compose-go parser could handle it if used, but deploy path's fallback parser discards the key. `ValidateComposeSecurity` has no branch for `extends`. No test covers it.
- **Beacon `compose.go`:** Not checked in `validateComposePolicy`. If `extends:` YAML reaches beacon, `docker compose up` will attempt to honor it but beacon writes only a single `compose.yaml` without the referenced base file → `extends: file: common.yml` fails at `docker compose` time with missing file error, not surfaced until deploy (409 with stderr).
- **Coolify/Dokploy note:** Both Coolify and Dokploy's compose generation do not emit `extends` (they template flat YAML), so divergence is upstream-vs-Forge rather than vs peers, but spec fidelity is still incomplete.

---

### C04 — `include` / Remote Includes

- **Upstream (`reference/app-platforms/docker-compose/cmd/compose/options.go:277`):** `cli.WithResourceLoader` supports remote `include:` from Git/OCI with warning banner `Warning: This Compose project includes files from remote sources...` and user confirmation (`assume yes` / reject). `types.Project` tracks `CountIncludesLocal/Remote`.
- **Forge `service.go:81-135`:** `rawCompose.Include []rawInclude` captured with `Path`, `ProjectDir`, `EnvFile` (`service.go:131-135`), but `ParseComposeYAML` never resolves includes (no `loader.ResourceLoader`). Includes are stored nowhere in `ParsedCompose`, not validated, not forwarded to beacon. Beacon similarly never fetches includes; `handleComposeDeploy` writes single `compose.yaml` and invokes `docker compose -f composePath -p stackId up -d` without `include` expansion.
- **GitOps path (`gitops.go:212` `readComposeFromDir`):** Only reads a single file (explicit `composePath` or nearest candidate `compose.yml`...). If that file contained `include:`, beacon's later `docker compose` would try to resolve relative includes from `stackDir` — but the included files were not written (only one file written at `beacon/internal/server/compose.go:374`). So `include: [./common.yml]` fails on node.

---

### C05 — `profiles` & Activation

- **Upstream (`pkg/compose/loader.go:113` `cli.WithDefaultProfiles`, `pkg/compose/create.go:69` `project.WithServicesEnabled/WithSelectedServices`, `dependencies.go:272-276` skips disabled services):** Profiles gate service existence; `docker compose --profile foo up` enables gated services. Dependency graph removes disabled services from edges if `Required: false`.
- **Forge `service.go:272-276` `ValidateCompose`:** Only emits warning `profiles are not currently enforced by Forge` (`service.go:274`). Parser stores them (`parser.go:537` `Profiles: svc.Profiles`), but `lifecycle.go` deployment does not filter by profiles, nor does HTTP API accept `--profile` parameter. `handlers_compose.go` has no query param for profiles. Beacon's `docker compose -p project up -d` is invoked without `--profile`; services with `profiles:` never start. No `COMPOSE_PROFILES` handling in `encodeComposeEnv`.
- **Phase-1 test gap:** `parser_comprehensive_test.go` asserts `profiles` roundtrip but no integration test enables profile and checks container creation.

---

### C06 — `depends_on` Conditions & Health Orchestration

- **Upstream (`pkg/compose/dependencies.go:17-285` `Graph`, `InDependencyOrder`/`InReverseDependencyOrder`, `convergence.go:157-285` `waitDependencies`):** Strong ordering: `NewGraph` builds DAG, detects cycles (`HasCycles`), handles `condition: service_started | service_healthy | service_completed_successfully | running_or_healthy`, `required: false`, disabled-service skipping, `Provider` services, parallel traversal with `errgroup` + `maxConcurrency`, polling `isServiceHealthy`/`isServiceCompleted` every 500 ms, timeout handling, `pre_start` hooks.

- **Forge `service.go:765` `normalizeDependsOn` / `parser.go:897` `normalizeDependsOnFromCompose`:** Both collapse typed `types.DependsOnConfig` (map of service→{condition,restart}) to `[]string` (display only), discarding `condition` semantics except optional `name:condition` serialization in `parser.go:805`. No DAG built, no cycle detection (except indirectly via compose-go if that parser were used), no `waitDependencies`. `lifecycle.go` `WaitForHealthy` merely polls container `Status`/`State` via `docker compose ps --format json` on a 5 s ticker (`lifecycle.go:191-228`), ignoring `depends_on` ordering and health checks. If a DB service specifies `condition: service_healthy` and fails readiness, upstream blocks dependent web; Forge starts all services simultaneously via single `docker compose up -d` (Compose engine itself does enforce `depends_on` inside the daemon, but Forge's healthgate doesn't wait per-condition).

- **Beacon `compose.go`:** No dependency logic; relies on `docker compose up -d` to order. Forge-side pre-flight does not fail fast on cycles.

---

### C07 — Secrets & Configs (Long-Form vs Short-Form, External, Driver)

- **Upstream (`pkg/compose/create.go:1082-1188` `buildContainerSecretMounts`, `buildContainerConfigMounts`):** Mounts secrets to `/run/secrets/<name>` or explicit `target`, validates external vs file, supports `driver`/`template_driver` errors, warns on `uid/gid/mode` unsupported, handles `environment:` delivery, long-form `{source, target, uid, gid, mode}`.

- **Forge `service.go:105-106` `rawService.Secrets []interface{}` / `parser.go:42-43` `Secrets []interface{}`:** Parsed only as string lists; `normalize*` functions not present in fallback path. `ValidateCompose` checks only `sec.External` (`service.go:311-317`), otherwise just warns. No check for file existence, no target resolution, no `uid/gid/mode` warning. `parser.go` via compose-go does decode full structs (`types.ServiceSecretConfig`), but deploy path's fallback discards long-form fields.

- **Beacon `compose.go`:** `docker compose up -d` handles secrets/config mount if compose file references them and files exist in `stackDir`. Forge never writes secret source files (only `.env`), so `secrets: {file: ./db_pass.txt}` would refer to nonexistent host path → docker compose failure hidden until beacon stderr. `validateComposePolicy` does not validate secrets/configs at all.

- **Comparison to Coolify/Dokploy:** Both platforms inject secrets via env vars / UI-managed encrypted stores, not via `secrets:` Compose primitive; their validation similarly blocks arbitrary host-file secrets. Forge's gap is that it neither fully supports nor fully rejects the primitive (partial warn-only).

---

### C08 — Restart Policy Fidelity & Divergence

- **Upstream (`pkg/compose/create.go:592-633` `getRestartPolicy`, `mapRestartPolicyCondition`):** Merges `service.restart` and `deploy.restart_policy` (with `MaxAttempts` → `MaximumRetryCount`). Maps `none/no`→Disabled, `on-failure`→OnFailure, `unless-stopped`→UnlessStopped, `any/always`→Always. Honors Swarm string forms. Deploys via `container.HostConfig.RestartPolicy`.

- **Forge `service.go:516-523` security policy / `parser.go:539` `Restart: stringifyValue(svc.Restart)`:** Security validator only warns `restart: always may conflict with platform lifecycle management` (`service.go:519`) — does NOT enforce. Beacon `validateComposePolicy` does **not** check restart at all (`compose.go:92-131` switch never includes `restart`). Parser stores raw string without mapping nor `deploy.restart_policy` precedence. Beacon's runtime `docker.go:527-528` for single-container workloads uses no per-compose restart; compose deploys delegate to `docker compose up` which honors the string verbatim, so `restart: always` actually applies, diverging from API warning that claims conflict/"may". No `MaximumRetryCount` propagation.

- **Logical inconsistency:** Forge warns on `always` but lets beacon start it; lifecycle `StopStack`/`StartStack` assume `docker compose stop|start` control — a container with `restart: always` will auto-restart outside lifecycle expectations.

---

### C09 — Resource Limits: `deploy.resources` vs Docker Limits & Validations

- **Upstream (`pkg/compose/create.go:635-762` `getDeployResources` + `setLimits`, `setReservations`, `setBlkio`):** Complete mapping:
  - Limits: `MemoryBytes`, `NanoCPUs`, `Pids`, `Blkio`.
  - Reservations: `MemoryBytes`, `DeviceRequests`/`DeviceCapabilities`.
  - Swarm-agnostic fields: `cpu_percent`, `cpu_shares`, `cpu_count`, `cpu_quota`, `cpu_period`, `cpus`, `mem_limit`, `mem_swappiness`, `mem_reservation`, `shm_size`, `device_cgroup_rules`, `blkio_config`, `pids_limit`, `ulimits`, `gpus`, `devices` (with CDI detection), full `blkio_config` (weight, device, throttles).
  - Proper unit conversions (NanoCPUs `* 1e9`, memory bytes).

- **Forge fallback (`service.go:841-861` `normalizeDeploy` / `parser.go:914-943` `normalizeResources`):**
  ```go
  // parser.go:914
  if resources.Limits.NanoCPUs != 0 { result.Limits["cpu"] = fmt.Sprintf("%g", resources.Limits.NanoCPUs) }
  if resources.Limits.MemoryBytes != 0 { result.Limits["memory"] = fmt.Sprintf("%d", resources.Limits.MemoryBytes) }
  // only cpu and memory survive; pids, devices, blkio, reservations.cpu etc. dropped
  ```
  No `Pids`, no `Devices`, no `BlkioWeight`, no `Gpus`, no `Ulimits`, no `ShmSize`. `DeploySummary` only has `Limits/Reservations map[string]string` — cannot distinguish units (bytes vs MB). `lifecycle.go:280-283` per-stack ceilings check `MemoryMB`/`CPUShares`/`DiskMB` globally, but never enforces per-service `deploy.resources.limits`.

- **Beacon `docker.go:783-806` `buildResources` (single-container runtime):** Correctly builds period/quota/shares/pids/memory overhead/swap handling for non-compose workloads, but compose path (`compose.go:395-399`) bypasses it entirely — `docker compose up` computes resources itself from compose file; Forge-side accounting not fed to Docker limits. So a compose file requesting `deploy: resources: limits: cpus: '4.0'` is honored by Docker, but not reflected in Forge's quota placement (`scheduler.PlaceServer` uses only top-level `MemoryMB`/`CPUShares` from request).

- **Coolify/Dokploy:** Both cap per-service resources via UI and template-generated compose with explicit `deploy.resources.limits`; Coolify enforces hard cgroup limits at deploy; Forge's gap: silently accepts unrealistic limits (e.g., `memory: 100TiB`) but warns only in security path for some fields, not for resources.

---

### C10 — Volumes: Type System, Binds, Enforceability

- **Upstream (`pkg/compose/create.go:862-1188` `buildContainerVolumes`, `buildContainerMountOptions`, `fillBindMounts`, `volumeRequiresMountAPI`, `bindRequiresMountAPI`):** Distinguishes `type: bind|volume|tmpfs|image`, selects `Binds` string API vs `Mounts` API based on advanced options (selinux propagation, recursive, `subpath`, `nocopy`, labels). Handles anonymous volumes inheritance from `inherit` container, `tmpfs` handling (`tmpfs.go`), `image` mounts (API 1.48+). Validates absolute source, handles named volumes vs bind mounts, creates volumes via `ensureVolume`.

- **Forge `service.go:734-762` `normalizeVolumes` / `parser.go:880-893` `normalizeVolumesFromCompose`:** Both coerce to `source:target[:ro]` strings, discarding `type`, `consistency`, `bind.propagation/selinux/recursive`, `volume.nocopy/subpath`, `tmpfs.size/mode`, `image.subpath`. `ValidateComposeSecurity` (`service.go:595-657`) blocks docker.sock/proc/sys and warns on `/`, `/root`, `/etc`, `/home` mounts (error for `/etc` per `service.go:642-643`), while **beacon `validateComposeVolumes` (`compose.go:226-249`) is stricter — blocks all host bind sources (`strings.HasPrefix(source, "/") → 403) and long-form `type: bind` (`compose.go:234`).** This duality creates inconsistent UX: API warns but may still store YAML, beacon then 400-rejects same YAML on deploy.

- **Beacon `mounts.go:64-89` (non-compose runtime):** For single-container workloads validates against `allowed_mounts` allowlist with symlink resolution and `rootfs.New`. Compose volumes use **different** policy (`validateComposeVolumes`) which has no allowlist — any absolute host bind rejected unconditionally. So compose stacks cannot use host binds even on nodes where `allowed_mounts` explicitly permits `/data`.

- **Coolify/Dokploy:** Both allow host binds only via allowlisted volumes or disabled by default; Coolify maps Coolify-managed persistent volumes to named Docker volumes and prevents arbitrary `/` mounts. Forge's compose volume handling is more permissive at API warning level, stricter at beacon enforcement, but lacks named-volume vs bind disambiguation feedback before deploy.

---

### C11 — Networks & DNS/IPAM

- **Upstream (`pkg/compose/create.go:132-153` `prepareNetworks`, `132-1471` `shouldCreateNetwork` with hash, `ensureNetwork`, IPAM, dual API path for extra networks `<1.44` vs `>=1.44` `defaultNetworkSettings`):** Handles custom `driver`, `driver_opts`, `ipam: config/subnet/gateway/ip_range/aux`, `internal`, `attachable`, `external`, `enable_ipv6`, `labels`, `name` override, endpoint-specific `ipv4_address`, `ipv6`, `mac_address`, `aliases`, `gw_priority`, `link_local_ips`. Resolves external by ID/name ambiguities, reconnects dangled containers.

- **Forge `service.go:109-113` `rawNetwork` / `parser.go:692-703` `normalizeNetworkToForge`:** Captures only `driver`, `external`, `labels` (plus forge extension `driver_opts` via `mapOptionsToMap`). No `ipam`, `internal`, `attachable`, `enable_ipv6`, `name`, per-service network aliases, IP overrides. `parser.go:578` does capture full via types but again not on deploy path. `beacon/compose.go` does not implement network isolation beyond `docker compose up` defaults; no per-network hash or driver validation beyond docker compose's own error.

- **Forge `docker.go:1065-1110` `ensureNetwork` (single-container runtime):** Manages IPAM, `modern-game-panel.managed` label, concurrent-create race, validates requested `NetworkIP` inside subnet. Compose networks bypass this logic entirely.

---

### C12 — Lifecycle: `docker compose up|down|stop|start|restart|pull|ps|logs` Parity

| Upstream operation | Upstream impl | Forge API | Beacon implement | Observations |
|---|---|---|---|---|
| **create + ensure images + ensure networks/volumes + reconcile/plan + executePlan** (`pkg/compose/create.go:62-130`) | Full: check container name unicity, `ensureImagesExists`, `ensureProjectVolumes`, `collectObservedState`, `reconcile`, dependency-ordered `executePlan` | `lifecycle.go:396-412` `ComposeDeploy` directly `docker compose up -d` without separate `create`; no orphan warning logic | `beacon/compose.go:395-401` `docker compose -f composePath -p stackId up -d` | Abbreviates lifecycle; skips `ensureImagesExists` pull-policy nuance (always implicit pull), no `IgnoreOrphans` handling except via `RemoveOrphans` opt-in flag (314 lines `compose.go:301` doc). |
| **up** (`pkg/compose/up.go:44-303`) | `create` + `start` (+ attach/watch/navigation menu) | `DeployComposeStack` merges both; no separate `Create` endpoint, no dry-run, no `project.Name` interpolation checks | Same `up -d` | No `Up` attach semantics (expected; stack API is detached). |
| **down** (`pkg/compose/down.go:38-125`) | Dependency-reverse removal, RemoveOrphans handling, network/image/volume pruning, `getProjectWithResources` fallback | `DeleteComposeStack` → `lifecycle.go:596-606` `ComposeDelete` → `beacon/compose.go:576-594` `docker compose down -v [--remove-orphans]` | `compose.go:578` `down -v` **always** removes volumes (`-v` hardcoded) | Upstream `volumes` is opt-in (`options.Volumes`). Forge unconditionally deletes volumes on stack delete, potentially destroying data user expected to persist (divergence from Coolify/Dokploy which default to preserve volumes unless asked). |
| **restart** (`pkg/compose/restart.go:31-114`) | Dependency-ordered, `PreStop`/`PostStart` hooks, `waitDependencies` with `Restart` filter | `lifecycle.go:781-786` `RestartStack` = `StopStack` + `StartStack` (two compose invocations, loses ordering) | `compose.go:506-547` `docker compose restart` (in-place restart) actually more faithful than API wrapper which stop+start sequentially | API `RestartStack` loses `restart` semantics (dependency `restart: false` filtered) and does not honor `PreStop`/`PostStart`. Beacon's native restart endpoint is superior but API layer doesn't use `ComposeRestart` — it calls `daemon.ComposeRestart`? Check `lifecycle.go:781` stops then starts, not `ComposeRestart`. So `handlers_compose.go:470-481` `/restart` route triggers API stop+start, while `daemon/compose.go:81` `ComposeRestart` exists but is unused by that handler. Inconsistency. |
| **stop/start** (`pkg/compose/stop.go`, `start.go`) | `stop` via `StopOptions` timeout, `start` via `StartOptions` with dependencies | `StopStack` (`lifecycle.go:745-779`) and `StartStack` (`697-743`) call `daemon.ComposeStop/Start` | `compose.go:418-504` `docker compose stop` / `start` per project | Matches upstream semantics (no down). |
| **scale** (`pkg/compose/scale.go:27-34`) | Delegates to `create` with `ScaleOptions` | No `ScaleStack` API; `deploy.replicas` parsed but no scaling endpoint exposed; `handlers_compose.go` has no `/scale` route | N/A | Gap: `deploy.resources`/`replicas` not actionable via API. Coolify/Dokploy both expose replica slider ⇒ Forge missing feature. |
| **pull** (`pkg/compose/pull.go:47-151`) | Per-service pull-policy aware (`PullPolicyNever/Build/Missing/...`), `IgnoreBuildable`, `Quiet`, image-present detection, parallel with `maxConcurrency` | `PullStack` (`lifecycle.go:788-815`) calls `ComposePull` unconditionally for whole project | `compose.go:709-750` `docker compose pull` | Upstream no-pull when policy `never` or build-only image; Forge always pulls (may fail for private build-only services). |
| **ps** (`pkg/compose/ps.go:32-134`) | Merges inspect data, sort ports, health, networks, volumes | `GetStackStatus` (`lifecycle.go:625-661`) proxies `ComposeStatus` | `compose.go:604-659` `docker compose ps --format json` line-delimited JSON parsing (`json.Unmarshal` per line) | Upstream returns `api.ContainerSummary` with health/mounts/networks; beacon returns only `Name/Image/Status/State/Ports` (`compose.go:314-320`). Health status filtered upstream vs raw String status here. `WaitForHealthy` in `lifecycle.go` duplicates some health logic but only checks lower-cased `State==running && Status==up`. |
| **logs** (`pkg/compose/logs.go:34-149`) | Multiplexed consumer, `Tail/Since/Until/Timestamps`, TTY vs non-TTY, follow monitor via `newMonitor` | `GetStackLogs` (`lifecycle.go:663-695`) wraps `ComposeLogs` | `compose.go:661-707` `docker compose logs --no-color --tail` (plain text, not JSON), no `Since/Until/Timestamps` query support | Upstream supports advanced log filtering; Forge only `tail`+`service`. Non-TTY `stdcopy.StdCopy` handled upstream; beacon just `CombinedOutput` of docker compose. |
| **build** (vs separate build service) | `pkg/compose/build.go`+`build_bake.go` orchestrates per-service `BuildConfig` (context, dockerfile, args, labels, cache_from/to, platforms, target, ssh, secrets, network, etc.), chooses bake vs classic | Build service (`build/service.go`) is **independent** of compose (`compose.Service` stores build only as `BuildSummary` string). Compose stacks with `build:` are not auto-built — `docker compose up` will try to build if Docker has context, but beacon only received `compose.yaml`, not build context files. | `build.go` (beacon) handles explicit `DockerfileBuild`/`NixpacksBuild` for sources, not inline compose build contexts | Structural gap: compose `build:` requires source tarball transfer; Forge compose path has no build-context upload. Dokploy zips context and uploads to server; Coolify clones repo then `docker compose build`. Forge's `build:` via compose will always fail for non-prebuilt images (`docker compose up` → `failed to read dockerfile` / `no such file`). |

---

### C13 — Interpolation & `configHashLabel` / Reconciliation

- **Upstream:** `envresolver.go` + interpolation inside `compose-go` loader plus `toBakeSecrets`, `resolveAndMergeBuildArgs`, proxy config merging. Container labels include `com.docker.compose.config-hash` computed via `ServiceHash` (`pkg/compose/create.go:500-506`) + `prepareLabels`, used to decide if container needs recreation (`convergence.go` diff). `createAndStart` fast-path idempotency on hash match.

- **Forge fallback `interpolateEnv` (`service.go:356-425`):** Handles `${VAR}`, `${VAR:-def}`, `${VAR-def}`, `${VAR:?err}`, `$$` — but **not** `${VAR:+alt}`, `${VAR:offset:length}`, no escaping edge cases, no compose-go's `Environment.Resolve` nuances (case-insensitivity already handled upstream via `envResolverWithCase`). Passes `envVars` map from API request; if `envVars == nil` substitutes empty map, silently expands unknown vars to `""` (same as upstream default). However, compose-go also resolves build args via `MappingWithEquals.Resolve(envResolver(...))` — two layers, not replicated. Beacon's `.env` writing re-uses same vars, so interpolated result may double-apply.

- **Forge `docker.go:899-1030` `configHashLabel`:** Similar label for single-container workloads (`modern-game-panel.config_hash`) based on `CreateRequest` JSON hash (excluding password). No equivalent for compose stacks; `compose.go` stacks have no hash label; reconciliation relies on docker compose's own hash. No diff event emission.

---

### C14 — Build Systems: Dockerfile / Nixpacks / Buildx vs Coolify/Dokploy Patterns

| Aspect | Coolify | Dokploy | Upstream compose build | Forge build service + beacon |
|---|---|---|---|---|
| **Supported builders** | `nixpacks`, `railpack` (next-gen), `static`, `dockerfile`, `dockercompose` (openapi.yaml enums). `docker/docker-compose` build via UI. | Generic: build from `docker buildx` or direct `docker compose build` (server/utils/deploy.ts). Supports nixpacks via template evaluation. | bake (`buildx`) with groups, classic fallback, auto-detection of `buildx` plugin, platform handling, cache_from/to, provenance/SBOM/attest. | `BuilderDockerfile` + `BuilderNixpacks` only (`build/service.go:27-30`). No `railpack`, `heroku/paketo`, `static` (rejected in `handlers_source_deployments.go:92-96` with error "must be dockerfile (nixpacks not yet supported by source deployments)" — contradictory because Nixpacks builder exists for explicit builds). |
| **Build context transfer** | Helper container: `docker/coolify-helper/Dockerfile` includes `docker buildx` + `nixpacks` (`coolify/docker/coolify-helper/Dockerfile:54-60`). Clones repo to `/coolify/sources` on helper; builds via helper's Docker socket. | Schedules via SSH/tarball upload to destination server; streams `docker build` output. | Uses local filesystem paths, git URLs (`build.DetectContextType` git/http/local), `dockerignore` handling, `compress` context. | `executeRemoteBuild` (`build/service.go:530-573`) triggers `daemon.GitClone` to beacon's workspace (`s.dataDir/gitCloneDir/workspaceId`), then `DockerfileBuild` with `SourceDir=cloneDir` or `WorkspaceID`. Context never leaves node; panel never sends tarball — git clone via beacon's git service. Works for git-backed builds, but inline `docker-compose.yml: build: .` for raw YAML has no cloned dir (`SourceDir=""` path fails validation `validateBuildContext`). Compose-integrated build not supported. |
| **Cache handling** | `setup-buildx-action@v3` in CI with `cache-from: type=registry,ref=...:buildcache` + `cache-to: type=gha` (`.github/workflows/coolify-staging-build.yml:81-84`). Runtime `build.cacheFrom/cacheTo` optional per-service. | Similar `docker-cleanup.ts` prune, `type=gha` cache. | `build.go` passes `buildConfig.CacheFrom/CacheTo` to bake targets; compose file `cache_from/cache_to` respected. Beacon's `isSafeBuildxRef` guards injection. | Forge **now** forwards `CacheFrom/To` after Phase-1 LF-04 fix (`beacon/internal/server/build.go:126-135` filters via `isSafeBuildxRef`; `dockerfileBuildRequest` added fields `build.go:54-56`; `service.go:95-96`). Prior silently dropped. `useBuildCache` UI field absent? Compose `build.cache_from`/`cache_to` not surfaced to composestacks — user must manually include in YAML; no centralized cache registry config. Limits: only `type=registry` refs allowed; `gha` cache (used by Coolify CI) rejected by `isSafeBuildxRef` (starts with `type=` contains `=` but still passes? Actually `isSafeBuildxRef` checks not starting with `-` and no `;` — so `type=gha,mode=max` passes though not a registry ref, begets buildx error). |
| **Platform selection** | Multi-arch via `matrix.arch` (`buildx imagetools create`), explicit `platform` per build. | `linux/amd64` default; `linux/arm64` optional via settings. | `platforms: [linux/amd64, linux/arm64]` per `build` yields `outputs: type=image,push=...` multi-arch manifest. | Forge defaults `record.Platform = "linux/amd64"` if unset (`build/service.go:435-437`), validates limited set (`validateBuildContext:880-890`). Beacon's `isSafePlatform` (`build.go:553-572`) allows comma-separated `goos/goarch` with underscores; rejects spaces/semicolons. `docker buildx build --platform` forwarded for Dockerfile; Nixpacks `--platform` for Nixpacks. No multi-arch bake grouping (compose bake's `platforms: [..]` would cause multi-platform push) — not exposed for compose stacks. |
| **Build args handling** | UI form per-service; escaped `${`→`$${` in bake (`build_bake.go:172`). | Env injection via `additionalContexts` service references. | `resolveAndMergeBuildArgs` (`build.go:243-267`): service `build.args` → CLI `--build-arg` override → env resolution → proxy `parseProxyConfig`. | `BuildOptions.BuildArgs []string` raw strings; validated via `validateBuildVariable` (line-length, name charset). No proxy injection, no compose build args resolution. Beacon's `DockerfileBuilder.Build` passes each arg as `--build-arg arg` without resolving against `EnvVars` map. |
| **Secrets propagation** | Build secrets via `type=file`/`type=env` with helper fs mounts; restricted to is: server. | Similar restricted mount via `/run/secrets`. | `toBakeSecrets` + `buildCtx` secret sources; warns on driver mismatch. | Compose `secrets:` inside build not supported in Forge compose YAML (fallback discards). Build service masks credentials (`maskCredentials`, `credentialPatterns`), but not via Docker `--secret`. Build via compose file cannot declare build secrets because source file omitted. |
| **Preview / cache reuse** | Coolify shows "Build cache" utilization per deployment; Dokploy shows env preview. | Dokploy's `checkForUpdates` diff. | N/A | Forge `GitOpsService.CheckForUpdates` clones and diffs services (`gitops.go:431-479`), but for builds, no preview; idempotency key prevents duplicate (`buildIdempotencyKey`). |

---

## 3. Additional Satellite Comparisons (Coolify/Dokploy Network/Volume Helpers)

### C15 — Coolify/Dokploy Network Generation vs Forge `prepareNetworks`/`ensureNetwork`

- Coolify/Dokploy generate deterministic network names `coolify`, `dokploy-network`; they ensure attachable bridge with IPAM defaults via `docker network create` with project label. Upstream `prepareNetworks` (`create.go:132-139`) labels each network with `api.NetworkLabel`+`api.ProjectLabel`+`api.VersionLabel`.

- Forge Beacon's compose path relies on docker compose to create networks named `<project>_<network>` (project prefix `stackId`). No `ensureNetwork` for compose; only single-container runtime ensures `gamepanel` network (`ensureExistingNetwork`). Compose stacks thus get isolated per-stack networks (good), but cross-stack network sharing (`external: true`) requires pre-existing docker network manually; upstream resolves via `resolveExternalNetwork` gracefully, Beacon just passes docker compose error.

### C16 — Volume Lifecycle vs Coolify/Dokploy

- Coolify mounts persistent volumes via explicit volume driver opts (NFS, local); creates volumes with `com.docker.compose.volume` label and hash comparison (`ensureProjectVolumes`:128-170, `VolumeHash`), `prompt` on divergence.

- Forge `ensureProjectVolumes` only for compose via docker compose (deferred); API `Delete` uses `-v` unconditional. Dokploy similarly prunes with `-v` opt. Forge divergence: no volume hash / divergent volume handling, no `docker volume create` with driver opts (compose `driver_opts` dropped; parser keeps only label `external`). Data-loss risk on down.

### C17 — UI→API→Queue→Beacon Scaling & Observability Parity

- Upstream has `api.ScaleOptions` (scale.go), `watch` (watch.go), `viz` graph, `hook` (pre_start/post_start).

- Dokploy's UI has `Scaling` slider that POSTs to `server/api/routers/...` which executes `docker compose up --scale svc=N`. Coolify has `replicas` within `destination` settings, updates compose via re-deploy.

- Forge UI (`compose-view.tsx:31-74`) only renders `fetchAppComposeConfig` (via `packages`? Actually old `compose-view` hits `apps` API). No scale slider, no watch, no hooks. `queue_handler.go:48-115` handlers exist but no queue enqueue observed in `handlers_compose.go` — composes are executed synchronously in request handler (`DeployComposeStack` does `client.ComposeDeploy` directly within HTTP handler context with `requestContext()`). Queue integration appears aspirational; actual deploy is synchronous + `WaitForHealthy` 2 min block, risking gateway timeouts under load. Dokploy uses BullMQ; Coolify uses Jobs/Actions queue.

---

## 4. Logic Findings (≥3)

### LF-01 — **CRITICAL: `env_file` Silently Ignored → Runtime Misconfiguration**

- **Location:** `forge/api/internal/services/compose/service.go:92-107` (`rawService` missing `env_file`), `forge/api/internal/services/compose/parser.go:245` (compose-go path never used for deploy), `beacon/internal/server/compose.go:226-249` (`validateComposeVolumes` + `encodeComposeEnv` only).
- **Upstream expectation:** Compose spec treats `env_file` as first-class; docker compose loads variables from host files relative to `WorkingDir` before merging with `environment:`. Even missing file defaults to warnings but interlinked `depends_on` health may vary.
- **Forge behavior:** User-provided `env_file: ./.env.production` arrives, is parsed into nothing, stripped from summary, not written to beacon. Deployed containers lack required env (DB passwords, etc.) → silent success (`up -d`) but app crashes. `ValidateComposeSecurity` has no code path for `env_file`, so no warning/error returned to user.
- **Impact:** Data-plane failure without control-plane signal. Test `parser_comprehensive_test.go:168,486` expects `env_file` parsing (compose-go path) but deploy path won't surface equality.
- **Coolify/Dokploy contrast:** Both explicitly disallow `env_file` host references and replace with UI-managed env injections (Coolify `DATABASE_URL` templating, Dokploy `environment.test.ts` encryption). Forge neither disallows nor supports — worst of both.

**Repro sketch:**
```yaml
services:
  web:
    image: nginx:alpine
    env_file:
      - ./frontend.env
    environment:
      - PORT=3000
```
Expected `PORT` override from `frontend.env`+`environment` merge. Forge deploy: only `PORT=3000` plus any `EnvVars` map; `frontend.env` keys absent. No error.

---

### LF-02 — **HIGH: `deploy.resources.limits` & `deploy.replicas` Not Honored; Beacon Resource Enforcement Bypass**

- **Location:** `forge/api/internal/services/compose/parser.go:913-943` (`normalizeResources` discards most limits), `forge/api/internal/services/compose/lifecycle.go:280-283` (per-stack ceiling only), `beacon/internal/runtime/docker.go:783-806` vs `beacon/internal/server/compose.go:395-401`.
- **Logic:** Flask example:
  ```yaml
  services:
    api:
      image: api:latest
      deploy:
        resources:
          limits:
            cpus: '4.0'
            memory: 8G
            pids: 100
          reservations:
            memory: 2G
        replicas: 3
  ```
  Forge fallback normalizes to `Limits: {"cpu": ..., "memory": ...}` only for display; `pids`, `cpuset`, `blkio`, `ulimits`, `gpus`, `shm_size` dropped. `DeployComposeStack` placement computes `PlacementRequest{CPU: CPUShares, MemoryMB, DiskMB}` from **top-level** request fields, not per-service deploy resources. A malicious/insufficiently validated user can request `memory: 8G` inside YAML while `DeployComposeRequest.MemoryMB` is low (default 512, `import.go:132`). Reservation succeeds, but beacon's docker compose then requests more memory → host OOM or hidden over-commit. Conversely, large `replicas: 10` causes docker compose to start 10 replicas, exceeding per-stack CPU ceiling (8192) without API rejection (requested resources capped at stack-level, not multiplied by replicas). `validateComposePolicy` never checks `deploy.resources`. Scaling API missing.
- **Upstream handling:** `getDeployResources` enforces per-service `Resources` (CPUPeriod/Quota, Memory, PidsLimit), swarm scheduler respects. Docker Compose v5 will actually set cgroup limits per container.

**Evidence:**
```go
// forge/api/internal/services/compose/parser.go:914-931
if resources.Limits.NanoCPUs != 0 {
  result.Limits["cpu"] = fmt.Sprintf("%g", resources.Limits.NanoCPUs)
}
if resources.Limits.MemoryBytes != 0 {
  result.Limits["memory"] = fmt.Sprintf("%d", resources.Limits.MemoryBytes)
}
// PidsLimit, devices, etc. never mapped
```
vs
```go
// reference/app-platforms/docker-compose/pkg/compose/create.go:743-761
func setLimits(limits *types.Resource, resources *container.Resources) {
  if limits.MemoryBytes != 0 { resources.Memory = int64(limits.MemoryBytes) }
  if limits.NanoCPUs != 0 { resources.NanoCPUs = int64(limits.NanoCPUs * 1e9) }
  if limits.Pids > 0 { resources.PidsLimit = &limits.Pids }
}
```

---

### LF-03 — **HIGH: Volume Policy Bifurcation → Host Bind- Mount Inconsistency + Data-Loss on Down**

- **Location:** 
  - `forge/api/internal/services/compose/service.go:595-657` (`checkVolumesSecurity` warns on sensitive paths, blocks docker.sock/proc/sys),
  - `beacon/internal/server/compose.go:226-249` (`validateComposeVolumes` strictly blocks `source` starting with `/` or path traversal, plus `type: bind` long-form),
  - `beacon/internal/server/mounts.go:64-89` (allowlisted mounts for single-container runtime),
  - `beacon/internal/server/compose.go:576-594` (`docker compose down -v` hardcoded).

**Logic split #1 — Validation mismatch:** API security validator flags `/etc` as error, but allows e.g., `/data` as warning (requires admin+allowlist gate off-path). Beacon validator rejects `/data` unconditionally (any `"/" + source`). A stack that passes `POST /compose/validate` can then fail `POST /compose/{id}/status` deploy with `400 compose policy violation: host bind mount source "/data" not allowed`. No pre-flight consistency; user sees successful validation then failed deploy.

**Logic split #2 — Allowlist bypass inconsistency:** Single-container workloads respect `DAEMON_ALLOWED_MOUNTS` (`allowedMountSource` with `EvalSymlinks`); compose workloads cannot use allowlisted host mounts at all (beacon `validateComposeVolumes` has no `allowed` parameter). Therefore extending node `allowed_mounts` to include `/opt/appdata` enables portal servers but not compose stacks mounting `/opt/appdata/config:/config`. Dokploy compensates by letting compose bind mounts reference named volumes only; Coolify restricts to its managed storage path.

**Logic split #3 — Data-loss:** `handleComposeDelete` always `down -v` (`compose.go:578`). Upstream `down.go:112-113` makes volume removal gated on `options.Volumes` boolean. Forge's unconditional `-v` deletes named volumes created by `docker compose up` even when user expected persistence (e.g., `postgres:13` with `volumes: - pgdata:/var/lib/postgresql/data`). Coolify explicitly preserves volumes unless user checks "Delete volumes". Forge has no query param for `keepVolumes`.

**Test blind spot:** `mounts_test.go` covers `allowedMountSource` for single-container path; no beacon test covers `validateComposeVolumes` host-bind rejection vs `checkVolumesSecurity` warning matrix.

---

### LF-04 — **MEDIUM: Build Context Not Shipped for Compose `build:` Services → Silent Build Failure**

- **Location:** `forge/api/internal/services/build/service.go:530-735` (`executeRemoteBuild`), `beacon/internal/server/compose.go:374-401` (only `compose.yaml` + `.env` written), `reference/app-platforms/docker-compose/pkg/compose/build.go:112-167` (build vs image-pull decision).

**Flow:** User imports/submits:
```yaml
services:
  web:
    image: myapp:local
    build:
      context: ./web
      dockerfile: Dockerfile
      args:
        - NODE_ENV=production
    ports: ["8080:80"]
```
Forge API `ParseComposeYAML` captures `build.context` as string, stores entire compose YAML. Beacon's `handleComposeDeploy` writes `compose.yaml` with `build: {context: ./web ...}` and triggers `docker compose up -d`. On node, `stackDir` contains only `compose.yaml` + `.env`; the `./web/Dockerfile` and source are absent. Docker Compose then errors: `failed to read dockerfile: open /path/compose/<id>/web/Dockerfile: no such file`. Beacon returns `409` with `ComposeOperationResponse{Error, Output}` (`compose.go:404-408`), which `lifecycle.go:402-405` surfaces as `markFailed`. From user perspective, `docker-compose.yml` that works locally (relative context) fails on Forge with opaque `Output` string.

Upstream/Dokploy solution: Dokploy's `deploy` clones repo and preserves relative tree, so `./web` exists; Coolify's helper similarly clones whole repo before `docker compose up`. Forge's `build.Service` handles git-cloned builds for **generic build jobs**, but that path is not wired to compose stacks: `gitops.go:393-397` deploy after clone writes compose YAML from cloned directory but beacon still only writes that one file without its referenced `build.context` directory. Need to tarball/transfer context or rewrite `build.context` to `image` after pre-building.

---

### LF-05 — **MEDIUM: Restart Policy Divergence + Lifecycle Race**

- **Location:** `forge/api/internal/services/compose/service.go:516-523` (warn on `restart: always`), `beacon/internal/server/compose.go:92-131` (no check), `forge/api/internal/services/compose/lifecycle.go:781-786` (`RestartStack` as Stop+Start), `beacon/internal/server/compose.go:506-547` (`handleComposeRestart` native restart).

**Evidence:** `RestartStack` impl:
```go
// lifecycle.go:781
func (s *Service) RestartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
  if _, err := s.StopStack(ctx, stackID); err != nil { return nil, err }
  return s.StartStack(ctx, stackID)
}
```
vs upstream `restart.go:77-114` dependency-ordered `ContainerRestart` with `WaitGroup`, Pre/Post hooks.

- **Issue A (policy):** `restart: always` containers auto-restart via dockerd; `StopStack` (which does `docker compose stop`) will stop, but host-level restart policy will immediately restart the container, causing `StartStack` to see already `running` and `WaitForHealthy` to flap. Upstream docs recommend `restart: unless-stopped` for Compose-managed stacks; Forge warns `always` at API but lets beacon enforce it anyway, creating platform-managed vs compose-managed contention.

- **Issue B (lifecycle):** API handler `POST /compose/:id/restart` (`handlers_compose.go:470-481`) correctly exists but calls the composite Stop+Start sequence instead of `daemon.ComposeRestart` → beacon native `handleComposeRestart`. The single-call `docker compose restart` (which respects `--no-deps` and restart-condition filtering) is not used. Thus two consecutive deploys plus restart can race with `WaitForHealthy` health gate (no per-container restart version tracking). Dokploy/Coolify both proxy directly to `docker compose restart`.

- **Issue C (resource map miss):** `getRestartPolicy` merges `deploy.restart_policy.condition` with `restart`; Forge never maps — a compose file using Swarm style `deploy.restart_policy: {condition: on-failure, max_attempts: 3}` is stored as string but not translated by Docker (needs engine swarm mode). Upstream mapping covers this; Forge loses retry semantics.

---

### LF-06 — **MEDIUM: Interpolation Coverage Incomplete (Beaconside Env Expansion)**

- **Location:** `forge/api/internal/services/compose/service.go:356-425` (`interpolateEnv` regex `\$\{([^}]+)\}|\$\$`). Supported: `${VAR}`, `${VAR:-default}`, `${VAR-default}`, `${VAR:?msg}`, `${VAR?msg}`, `$$`. Unsupported (per compose spec): `${VAR:+alt}`, `${VAR:offset:length}`, nested expansions, quoting preserve rules. Also handles only `\${...}` form, not bare `$VAR` (without braces). Upstream compose-go uses `MappingWithEquals.Resolve(envResolver(...))` with full spec (including `+` conditional). Subtle divergence: `foo: ${ENABLED:+--verbose}` expected to emit empty or `--verbose`; Forge always interpolates to `""` (bare var not matched). Build-phase envs (multiple env sources precedence: `env_file` → `environment:` → `.env`) not replicated. Beacon `.env` written from raw `EnvVars` map only; variables defined via `env_file` never appear, causing interpolated `compose.yaml` (already substituted via `ExpandTemplate` for imported stacks? Actually `ParseComposeYAML` interpolates before yaml unmarshal, so some expansions happen server-side using `envVars` param; beacon then re-expands? docker compose will also expand using `.env` file — double substitution risk).

---

### LF-07 — **LOW (but systematic): Parity Gaps with Coolify/Dokploy UX**

| Feature | Coolify/Dokploy | Forge |
|---|---|---|
| **Preview builds / PR builds** | Coolify has preview deployments per PR, Dokploy has branch preview via `previewDeployment` (not in Forge audit scope but Phase1 noted cache handling). | `GitOpsService` supports `CheckForUpdates`/`GetUpdatePreview` but no ephemeral preview stack lifecycle (no `pr-<id>` project naming). |
| **Build logs streaming** | Coolify `docker logs -f` via Realtime helpers (`other/`, `docker/coolify-realtime`), Dokploy `listen-deployment.ts` websocket. | Forge has `GetStackLogs` (100 tail) but no streaming; build logs stored up to 100 MB then truncated (`beacon/build.go:105-109` `maxLogBufferSize` → `LOG TRUNCATED`). UI's `compose-view.tsx` polls via `useQuery` not WS. |
| **Registry & digest pinning** | Ports/registry auth stored encrypted; pinned image `name@sha256:...` enforced (`beacon/runtime/docker.go:40` `pinnedImagePattern` for single containers). | For compose, digest pinning not enforced — image `nginx:latest` pull bypasses pinned requirement path (compose runtime uses `ImagePull` via docker compose, not `ensureImage` with `pinnedImagePattern`). |
| **Resource governance** | Coolify enforces per-service `resources` via UI slider; Dokploy plans-limits middleware (`plan-limits.ts`). | Forge caps per-stack `MemoryMB/CPUShares` at API level only; per-service deploy resources unbounded, bypassing scheduler. |
| **Networking UI** | Coolify exposes service networks diagram (`viz.go` inclusion). | Forge UI shows only `containerName` color block; `parser.go` stores networks but not exposed via `ComposeView`. |

---

## 5. Security & Correctness Cross-Checks

### Secrets-in-logs scrubbing parity
- Forge `build/service.go:480-483` `maskCredentials`, beacon `build.go:400-402` per-pattern replace with `****`. Good parity.
- Compose env logs: `beacon/compose.go` outputs raw `docker compose` stderr (which may contain expanded secrets if `environment: - SECRET=x`); `encodeComposeEnv` writes secrets to `.env` at `0640` (`compose.go:387`). File perms good, but no masking in API responses (output returned via `ComposeOperationResponse.Output`). Postfix: ensure `Output` redacted.

### Queue integrity
- `queue_handler.go:10-115` assumes JSON payload decode → direct service call. No idempotency key, no dead-letter, no concurrency limit. Dokploy uses BullMQ with `concurrency.test.ts`; Forge queue journal exists (`server/queue.go` not covered here) but compose handlers don't integrate with it — deploy is direct HTTP, so queue not on hot path.

### Env interpolation SSRF / ReDoS
- `interpolateEnv` regex `\$\{([^}]+)\}|\$\$` matches greedy `[^}]` without length bound; unbounded repetitions on 1 MB YAML could be DoS. Upstream uses byte-level loader with streaming. Mitigated by `MaxComposeYAMLBytes = 1MB` (`service.go:14`).

---

## 6. Summary Risk Matrix

| ID | Severity | Title | User Impact | References |
|---|---|---|---|---|
| LF-01 | **CRITICAL** | env_file silently ignored | Deploy-time missing env, app crash | `service.go:92-107`, `compose.go:226`, `parser.go:245` vs `loader.go:36` |
| LF-03 | **HIGH** | Volume policy bifurcation & unconditional -v on down | Bind mounts rejected post-validate; data loss | `service.go:595-657`, `compose.go:226-249,578`, `mounts.go:64` |
| LF-02 | **HIGH** | deploy.resources & replicas not honored / scheduler bypass | OOM, quota overflow, hidden over-commit | `parser.go:913-943`, `lifecycle.go:280`, `create.go:635` |
| LF-04 | **MEDIUM** | build context not shipped for compose build | `build:` stacks fail, opaque 409 | `build/service.go:530`, `compose.go:374`, `build.go:112` |
| LF-05 | **MEDIUM** | restart policy + lifecycle race | Containers restart outside lifecycle, Start/Stop flaps | `service.go:516`, `lifecycle.go:781`, `compose.go:506` |
| LF-06 | **MEDIUM** | Interpolation incomplete, double expansion | Vars lost for `+` forms, PR env mismatch | `service.go:346-425`, `envresolver.go:30` |
| C05 | **MEDIUM** | Profiles not enforced | Services with `profiles:` never run | `service.go:272`, `loader.go:113` |
| C04 | **MEDIUM** | Include not resolved | Compose with includes fails on beacon | `service.go:131`, `compose.go:374`, `options.go:277` |
| C12-down | **MEDIUM** | down always -v | Persistent volumes deleted | `compose.go:578` vs `down.go:112` |
| C12-scale | **LOW** | Scale unsupported | No replica scaling API | `scale.go:27`, `handlers_compose.go` missing `/scale` |

---

## 7. Recommendations (No Code Changes — Findings Only)

1. **Unify parser:** Route deploy validation through `ComposeParser` (compose-go) — keep `rawService` only for security lint, not for summary/hash. Ensure `env_file`, `extends`, `include`, `profiles`, `secrets` long-form survive.
2. **Reject or support `env_file` explicitly:** Either error on `env_file` presence (with actionable message) or implement: read `.env` overlay, inject into `EnvVars`, or bundle env files along with `compose.yaml` on beacon. Match beacon `validateComposePolicy` to same decision.
3. **Converge mount policies:** Add `allowed_mounts` check to `validateComposeVolumes` (or at least mirror `checkVolumesSecurity` allowlist), keep `down -v` behind query param `?volumes=false` default preserve (align with upstream `options.Volumes`). Document data-path.
4. **Enforce resource limits:** Compute per-service `MemoryBytes+NanoCPUs*replicas` aggregate against `MaxUserStack*` ceilings in `ValidateCompose`; surface in `ValidateResult.Warnings/Errors`. Optionally call `buildResources` equivalent during scheduling.
5. **Fix compose build pipeline:** For stacks with `build:`, either disable compose-build path (force pre-build via `buildService` and inject resulting image digest into compose YAML stripping `build:`) or ship build context tarball to beacon via new `compose/build` endpoint. Document requirement for prebuilt images today.
6. **Wire `RestartStack` to native beacon restart** (use `daemon.ComposeRestart`) instead of Stop+Start, and block `restart: always` at API with guidance to `unless-stopped`.
7. **Support scaling:** Expose `POST /compose/:id/scale` forwarding to `docker compose up --scale svc=N` or regenerate compose with `deploy.replicas` and reconciliation; record scale in store.
8. **Logs parity:** Extend `handleComposeLogs` to accept `since/until/follow` forwarding to `docker compose logs --since --timestamps --follow` with SSE streaming, matching upstream `Logs` signature.

---

## 8. File:Line Citations Index

**Forge:**
- `forge/api/internal/services/compose/service.go:14` `MaxComposeYAMLBytes`
- `forge/api/internal/services/compose/service.go:81-136` `rawCompose`/`rawService`/`rawInclude`
- `forge/api/internal/services/compose/service.go:145-158` fallback `ParseComposeYAML`
- `forge/api/internal/services/compose/service.go:240-330` `ValidateCompose` (service lacks image/build, profiles/healthcheck/deploy warnings, secrets/configs external)
- `forge/api/internal/services/compose/service.go:346-425` `interpolateEnv` (env var regex, `:-`/`-`/`?`/` :?`)
- `forge/api/internal/services/compose/service.go:516-523` restart always warning
- `forge/api/internal/services/compose/service.go:595-657` `checkVolumesSecurity`
- `forge/api/internal/services/compose/service.go:841-861` `normalizeDeploy` (deploy resources)
- `forge/api/internal/services/compose/parser.go:245-265` compose-go `ParseComposeYAML` (unused in deploy)
- `forge/api/internal/services/compose/parser.go:913-943` `normalizeResources` (cpu/memory only)
- `forge/api/internal/services/compose/parser.go:245-324` `NormalizeToForge`
- `forge/api/internal/services/compose/lifecycle.go:26-36` `StackStatus` enum
- `forge/api/internal/services/compose/lifecycle.go:141-153` `MaxUserStack*` ceilings
- `forge/api/internal/services/compose/lifecycle.go:188-248` `WaitForHealthy` health gate
- `forge/api/internal/services/compose/lifecycle.go:249-435` `DeployComposeStack` (placement, reservation, security validate, hash)
- `forge/api/internal/services/compose/lifecycle.go:625-779` `GetStackStatus`/`GetStackLogs`/`StartStack`/`StopStack`/`RestartStack`/`PullStack`
- `forge/api/internal/services/compose/lifecycle.go:781-786` Restart as stop+start
- `forge/api/internal/services/compose/queue_handler.go:10-115` queue handlers (not on HTTP hot path)
- `forge/api/internal/services/compose/gitops.go:77-95` `DriftCheckResult`/`PreviousDeploymentManifest`
- `forge/api/internal/services/compose/gitops.go:1151-1258` webhook HMAC and async `PullAndRedeploy`
- `forge/api/internal/services/compose/import.go:76-182` `ImportComposeProject` (dual parser)
- `forge/api/internal/http/handlers_compose.go:66-481` `registerComposeRoutes` (CRUD, gitops, restart route)
- `forge/api/internal/daemon/compose.go:13-184` daemon client (deploy/stop/start/delete/status/logs/pull/restart)
- `forge/api/internal/services/build/service.go:27-31` `BuilderType` constants
- `forge/api/internal/services/build/service.go:95-102` `BuildOptions` (CacheFrom/To/Platform)
- `forge/api/internal/services/build/service.go:530-735` `executeRemoteBuild`
- `forge/api/internal/services/build/service.go:894-957` `DockerfileBuilder.Build` (args, cache, platform)
- `forge/api/internal/services/build/service.go:963-1040` `NixpacksBuilder`
- `forge/api/internal/services/build/service.go:1058-1093` `validateBuildVariable`/`runBuildCommand` allowlist
- `forge/web/components/app/compose-view.tsx:31-74` UI view (polling, no scale)

**Beacon:**
- `beacon/internal/server/compose.go:77-179` `validateComposePolicy` (privileged, cap_add, devices, ports, volumes)
- `beacon/internal/server/compose.go:226-262` `validateComposeVolumes` (strict host bind block)
- `beacon/internal/server/compose.go:264-281` `encodeComposeEnv` (newline stripping)
- `beacon/internal/server/compose.go:290-416` `handleComposeDeploy` (`docker compose up -d`, `--remove-orphans` opt-in)
- `beacon/internal/server/compose.go:418-707` `handleComposeStop/Start/Restart/Delete/Status/Logs/Pull`
- `beacon/internal/server/compose.go:576-594` `down -v` unconditional
- `beacon/internal/server/build.go:54-77` `dockerfileBuildRequest`/`nixpacksBuildRequest` (CacheFrom/To/Platform added post LF-04)
- `beacon/internal/server/build.go:79-239` `handleDockerfileBuild`/`handleNixpacksBuild` (lookup, args, cache, platform guards `isSafeBuildxRef`/`isSafePlatform`)
- `beacon/internal/server/build.go:336-572` `buildManager.startBuild` (100 MB truncate, reaper)
- `beacon/internal/server/mounts.go:64-89` `allowedMountSource` allowlist logic
- `beacon/internal/runtime/docker.go:42-76` `NewDockerRuntime` (network pin, unpinned image pattern)
- `beacon/internal/runtime/docker.go:113-149` `ensureImage` digest pinning
- `beacon/internal/runtime/docker.go:783-862` `buildResources`/`buildHostConfigWithSettings` (period/quota/pids/memory)

**Reference Upstream:**
- `reference/app-platforms/docker-compose/pkg/compose/create.go:62-130` `create` lifecycle
- `reference/app-platforms/docker-compose/pkg/compose/create.go:132-153` `prepareNetworks` labeling
- `reference/app-platforms/docker-compose/pkg/compose/create.go:592-633` `getRestartPolicy`/`mapRestartPolicyCondition`
- `reference/app-platforms/docker-compose/pkg/compose/create.go:635-762` `getDeployResources`/`setLimits`/`setReservations`/`setBlkio`
- `reference/app-platforms/docker-compose/pkg/compose/create.go:862-1188` `buildContainerVolumes`/`buildContainerSecretMounts`/`buildContainerConfigMounts`/`buildMount`
- `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:17-285` graph + cycle detection + `InDependencyOrder` parallelism
- `reference/app-platforms/docker-compose/pkg/compose/convergence.go:51-285` scale, resolve refs, `waitDependencies`/`isServiceHealthy`/`isServiceCompleted`
- `reference/app-platforms/docker-compose/pkg/compose/envresolver.go:30-66` case-insensitive resolver
- `reference/app-platforms/docker-compose/pkg/compose/loader.go:36-157` `LoadProject` with `WithWorkingDirectory`/`WithOsEnv`/`WithEnvFiles`/`WithDotEnv`/`WithResourceLoader`+remote
- `reference/app-platforms/docker-compose/pkg/compose/up.go:44-303` `Up` (create+start+sigs/watch)
- `reference/app-platforms/docker-compose/pkg/compose/down.go:38-125` `Down` (reverse deps, orphans, networks/images/volumes flag)
- `reference/app-platforms/docker-compose/pkg/compose/scale.go:27-34` `Scale`
- `reference/app-platforms/docker-compose/pkg/compose/ps.go:32-134` `Ps` (health, networks, mounts)
- `reference/app-platforms/docker-compose/pkg/compose/logs.go:34-149` `Logs` (consumer, follow, monitor)
- `reference/app-platforms/docker-compose/pkg/compose/build.go:35-303` `Build` (bake vs classic, pull policy)
- `reference/app-platforms/docker-compose/pkg/compose/build_bake.go:54-602` `doBuildBake` (targets, cache, ssh, secrets)
- `reference/app-platforms/docker-compose/pkg/compose/pull.go:47-151` `Pull` (policy-aware)
- `reference/app-platforms/docker-compose/pkg/compose/restart.go:31-114` `Restart` (ordered, hooks)
- `reference/app-platforms/docker-compose/cmd/compose/compose.go:79-92` env_file load
- `reference/app-platforms/docker-compose/cmd/compose/options.go:277-281` remote includes warning
- `reference/app-platforms/coolify/openapi.yaml:83` builder enum `nixpacks|railpack|static|dockerfile|dockercompose`
- `reference/app-platforms/coolify/docker/coolify-helper/Dockerfile:54-60` buildx+nixpacks helper

---

*Do not modify product code — audit only. Report filed as `audits/phase-06/subagent-10-compose-builds.md`.*
