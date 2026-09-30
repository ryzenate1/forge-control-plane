# Subagent 09 — Reverification: Docker-Compose Spec Fidelity (env_file / dependencies / convergence / resources / mounts / restart / secrets / scaling) — Forge Compose Service + Beacon Compose Handler

**Scope:** Reconcile `phase-06 subagent-10-compose-builds.md` (17 spec comparisons) + `phase-06 subagent-07-komodo` 6-phase `ComposeUp` + `final-parity subagent-04` RT-11..RT-15 + `MASTER_FINDING_INDEX.md` REF-P6-COMP-01..04 vs current checkout. Re-inspect reference `docker-compose pkg/compose/*` against Forge `forge/api/internal/services/compose/*`, `beacon/internal/server/compose.go`, `beacon/internal/runtime/docker.go`, `daemon/compose.go`, `forge/api/internal/services/build/*`, `beacon/internal/server/build.go`.

**Date:** 2026-08-24
**Auditor:** subagent-09 (reverification, parallel 09/20)
**Mode:** Read-only inspection; no product code modified. All citations file:line SOURCE_VERIFIED on current checkout (`f015a22`).

---

## 1. Reference Inventory (file:line verified)

| Ref | Path:Symbol | Proves |
|-----|-------------|--------|
| docker-compose loader | `reference/app-platforms/docker-compose/pkg/compose/loader.go:36` + `pkg/compose/envresolver.go` + `cmd/compose/compose.go:79` `cli.NewProjectOptions` (WithEnvFiles/WithDotEnv/WithWorkingDirectory/RemoteLoader) | Full env resolution: `.env`, `env_file`, `include.env_file`, `${VAR:-default}`, `${VAR:?err}`, `${VAR:+alt}`, `$$`, bare `$VAR` |
| docker-compose dependencies | `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:17` `InDependencyOrder`/`InReverseDependencyOrder`/`NewGraph`/`HasCycles` | DAG topological sort, cycle detection, `condition: service_started|service_healthy|service_completed_successfully`, `required:false`, disabled-service pruning, `Provider` services |
| docker-compose convergence | `reference/app-platforms/docker-compose/pkg/compose/convergence.go:88` `waitDependencies` (500 ms poll `isServiceHealthy`/`isServiceCompleted`) + `create.go:62` `create → ensureImages → ensureProjectVolumes → collectObservedState → reconcile → executePlan` | Health-gated ordering, parallel traversal with `errgroup`+`maxConcurrency` |
| docker-compose create | `reference/app-platforms/docker-compose/pkg/compose/create.go:592` `getRestartPolicy`+`mapRestartPolicyCondition:619` / `:635` `getDeployResources`+`setLimits:641`+`setReservations:728` / `:862` `buildContainerVolumes`/`buildContainerMountOptions` | Merges `restart`+`deploy.restart_policy`, maps `deploy.resources.limits.{MemoryBytes,NanoCPUs,Pids,Blkio}`+reservations+`cpu_shares/quota/period/blkio_config/ulimits/device_requests/gpus/shm_size` into `container.Resources`; volume type `bind|volume|tmpfs|image` with mount API selection |
| Komodo ComposeUp | `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:414` `ComposeUp` 6-phase pipeline | `write_stack → maybe_login_registry → pre_deploy → docker compose config → build → pull → down? → up -d → post_deploy` gated `all_logs_success`, `[[COMPOSE_COMMAND]]` wrapper |
| Komodo project naming | `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:45` `Stack::project_name(fresh)` | Fresh vs deployed project name, `last_project_name` rename-aware down |

---

## 2. Forge Chain Inventory (current, file:line)

| Layer | File:line | Evidence (current checkout) |
|-------|-----------|-----------------------------|
| **API compose service (fallback parser)** | `forge/api/internal/services/compose/service.go:14` `MaxComposeYAMLBytes=1MB` / `:92` `rawService` / `:356` `interpolateEnv` (`composeVarRe:346` `\$\{([^}]+)\}\|\$\$`) / `:427` `ValidateComposeSecurity` / `:594` `checkVolumesSecurity`+`isSensitiveHostPath:679` | Custom YAML structs with ~13 keys (`image,build,ports,environment,volumes,depends_on,profiles,restart,command,entrypoint,healthcheck,deploy,secrets,configs`). No `env_file` field on service; `interpolateEnv` supports `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?msg}`, `${VAR?msg}`, `$$` only. |
| **API compose-go wrapper (unused on deploy hot-path)** | `forge/api/internal/services/compose/parser.go:245` `ParseComposeYAML` + `:364` `ParseComposeString` (via `compose-spec/compose-go/v2/loader`) / `:913` `normalizeResources` / `:897` `normalizeDependsOnFromCompose` | Compliant loader exists but `service.go:145` `ParseComposeYAML(content,workingDir,nil)` and `lifecycle.go:264` `DeployComposeStack` call **fallback** parser, not wrapper. `ImportComposeProject` does use wrapper (`import.go:96`), creating divergent validation. |
| **API lifecycle / placement / health** | `forge/api/internal/services/compose/lifecycle.go:139` `MaxUserStackMemoryMB/CPUShares/DiskMB` / `:188` `WaitForHealthy` (5 s ticker, 2 m) / `:249` `DeployComposeStack` / `:476` `UpdateComposeStack` / `:570` `DeleteComposeStack` / `:781` `RestartStack` | `DeployComposeStack:260` `ValidateCompose` gate + `273` per-user quota `MaxUserStacks=20` + `280` stack-level ceilings + `293` `scheduler.PlaceServer` via top-level `CPU/MemoryMB/DiskMB` + `390` `store.CreateComposeStack` + `396` `daemon.ComposeDeploy` + `416` `WaitForHealthy`. `RestartStack:781` is `StopStack`+`StartStack` not native restart. |
| **API queue** | `forge/api/internal/services/compose/queue_handler.go:10` `ComposeQueueHandler` / `:49` `HandleDeploy/Update/Delete/Start/Stop/Restart` | Thin JSON translators forwarding to lifecycle methods; no idempotency, no concurrency limit. Unused: `handlers_compose.go:315` deploys synchronously via `DeployComposeStack` direct. |
| **API gitops** | `forge/api/internal/services/compose/gitops.go:196` `readComposeFromDir` / `:287` `readLimitedComposeFile` / `:301` `DeployFromGit:384` / `:687` `DetectDrift` / `:755` `DetectRuntimeDrift` / `:1151` `HandleWebhook` (`webhookMu`+`GitLastDeliveryID:1176`) / `:1280` `checkAndPollStack` | Single-file read (no `include` expansion), `Update` on fresh ID bug, `webhookMu` per-process only, `DetectDrift:841` `computeServiceDiffs` on image/ports/env/volumes/restart/command/dependsOn/replicas. |
| **API daemon client** | `forge/api/internal/daemon/compose.go:13` `ComposeDeployRequest` / `:48` `ComposeDeploy` (`POST /compose/deploy`) / `:81` `ComposeRestart` / `:110` `ComposeStatus` / `:131` `ComposeLogs` | `composeAction:157` builds `/compose/<id>[/<action>]`. `ComposeDeploy` POSTs `StackID+ComposeYAML+EnvVars` only (no `RemoveOrphans` flag propagated; daemon's `ComposeDeployRequest` lacks that field vs beacon's `composeDeployRequest.RemoveOrphans:301`). |
| **Beacon compose handler** | `beacon/internal/server/compose.go:59` `validStackID` / `:77` `validateComposePolicy` / `:181` `validateComposePorts` / `:212` `shortFormHostPort` / `:226` `validateComposeVolumes` / `:264` `encodeComposeEnv` / `:344` `handleComposeDeploy` / `:418` `handleComposeStop` / `:462` `handleComposeStart` / `:506` `handleComposeRestart` / `:550` `handleComposeDelete:578` `down -v` / `:604` `handleComposeStatus` / `:661` `handleComposeLogs` / `:709` `handleComposePull` | Policy YAML `map[string]any` deny-list (privileged/network_mode/pid/userns_mode/cap_add/devices/security_opt + ports/volumes). Single-shot `docker compose -f composePath -p stackID up -d [--remove-orphans]` (10 m), writes `compose.yaml` 0640 + `.env` 0640 under `compose/<id>`. Delete always `-v`. |
| **Beacon runtime (single-container isolation)** | `beacon/internal/runtime/docker.go:783` `buildResources` / `:827` `buildHostConfigWithSettings` (`CapDrop ALL`, `Privileged false`, `Init true`, `ReadonlyRootfs true`, `SecurityOpt no-new-privileges:seccomp=builtin`, `UsernsMode host` if rootless, `Tmpfs /tmp`, `LogConfig json-file 10m×3`) / `:1065` `ensureNetwork` | Correct cgroup/host isolation for game workloads. Compose path `compose.go:395` **bypasses** this entirely — `docker compose up` sets limits from YAML, not from `buildResources`. |
| **Beacon mounts** | `beacon/internal/server/mounts.go:64` `allowedMountSource` (`EvalSymlinks`+`Rel` within `allowedMounts`) | Used for game workloads only; compose `validateComposeVolumes` ignores allowlist (see C10). |
| **Build service + beacon build** | `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` / `beacon/internal/server/build.go:54` `dockerfileBuildRequest` (`CacheFrom/To:54` `Platform:56`) / `:126` `isSafeBuildxRef` / `:136` `isSafePlatform:553` / `:44` `maxLogBufferSize 100MB` | Git-clone→`DockerfileBuild`/`NixpacksBuild` with `workspaceID` confinement (`resolveWorkspace:176`). Cache/platform now forwarded after LF-04 fix; `isSafeBuildxRef` guards injection. Not wired to compose `build:` contexts. |

---

## 3. Parity Matrix — Reference vs Forge (≥12 rows)

**Legend:** `COMPLETE` parity-or-better, `PARTIAL` subset/divergent, `BROKEN` silent wrong behaviour, `MISSING` not built, `WIRED_BUT_WRONG` semantic mismatch. Every `BROKEN` maps to a finding in §4.

| # | Capability | Reference Path:Symbol | Forge Parser/Validator (API) file:line | Forge Runtime/Handler (Beacon) file:line | Status | Gap / Logic (concise) | Severity |
|---|------------|-----------------------|----------------------------------------|------------------------------------------|--------|------------------------|----------|
| C01 | YAML loading: multi-file, `WorkingDir`, `.env`, `COMPOSE_FILE`, remote `include` | `loader.go:36` `cli.NewProjectOptions` + `options.go:277` `RemoteLoader` (Git/OCI + confirmation) | `service.go:145` `ParseComposeYAML` (fallback `yaml.Unmarshal` raw structs) vs `parser.go:245` loader wrapper **not** on deploy hot-path (`service.go:145`/`lifecycle.go:264`) | `compose.go:374` writes single `compose.yaml`; `gitops.go:212` `readComposeFromDir` reads single file | **BROKEN** | Deploy path never benefits from compose-go handling of `COMPOSE_FILE`, remote includes, `.env` overlay. Import (`parser.go:364`) may show valid while deploy parser accepts/rejects differently. | **P1** |
| C02 | `env_file` support | `types.ServiceConfig.EnvFiles` + `envresolver.go` | `service.go:92` `rawService` has **no** `EnvFile` field (only `rawInclude.EnvFile:134` for `include:`) — silently dropped; `ValidateComposeSecurity:427` never checks it; `ValidateCompose:240` summary omits it | `compose.go:380` writes `Req.EnvVars` to `.env` via `encodeComposeEnv:264` only; never mounts host `env_file` sources; `validateComposePolicy:92` switch omits `env_file` | **BROKEN** | `env_file: ./app.env` ignored — no error, no inclusion — containers start missing required env; compose-go import path *does* understand `env_file`, so UX diverges `valid` at import vs empty at deploy. **Reconfirms REF-P6-COMP-02 (P0) + RT-11 BROKEN** | **P0** |
| C03 | `extends` (service inheritance) | `cmd/compose/compose.go:318` `extends: {service,file}` | `service.go:92` / `parser.go:42` neither struct has `extends`; fallback discards key | `compose.go:73` no `extends` check; single `compose.yaml` written without referenced base file → `docker compose up` fails with missing file | **MISSING** | Spec incompleteness; `extends: file: common.yml` fails only at deploy `409` with opaque stderr, not at validate | **P2** |
| C04 | `include` / remote includes | `options.go:277` `WithResourceLoader` + warning banner | `service.go:81` `rawCompose.Include []rawInclude` captured with `Path/ProjectDir/EnvFile` but never resolved (no `loader.ResourceLoader`); not stored in `ParsedCompose` | `compose.go:344` single `compose.yaml` + `docker compose up -d` without include expansion; `gitops.go:212` single-file read | **MISSING** | `include: [./common.yml]` silently unreachable at beacon; remote `include:` Git/OCI warning/confirmation never implemented | **P2** |
| C05 | `profiles` activation | `loader.go:113` `WithDefaultProfiles`, `dependencies.go:272` disabled-service pruning | `service.go:272` `ValidateCompose` emits **warning** `profiles are not currently enforced by Forge` only; parser stores them (`parser.go:537`) but lifecycle never filters | `compose.go:395` invokes `docker compose -p id up -d` without `--profile`; services with `profiles:` never start; `encodeComposeEnv:264` has no `COMPOSE_PROFILES` | **PARTIAL** | Fidelity gap: gated services silently absent; no API query param for profiles. | **P2** |
| C06 | `depends_on` conditions, DAG & convergence | `dependencies.go:17` `NewGraph`/`InDependencyOrder`/`HasCycles` + `convergence.go:157` `waitDependencies` (per-`condition`, 500 ms poll, `required:false`, disabled-service pruning, `Provider`) | `parser.go:897` `normalizeDependsOnFromCompose` and `service.go:765` `normalizeDependsOn` collapse `types.DependsOnConfig{condition,restart}` → `[]string` names (condition `name:condition` serialization at `parser.go:805` is display-only); no DAG, no `HasCycles`, no `waitDependencies` | `lifecycle.go:188` `WaitForHealthy` polls `ComposeStatus` 5 s ×2 m checking only `State==running && Status==up`, ignoring `depends_on` ordering; `compose.go:344` single `docker compose up -d` delegates ordering to Docker engine | **PARTIAL** | Engine still orders `depends_on` internally at `up` time, so runtime mostly correct, but pre-flight fails to fail-fast on cycles, does not respect `required:false` / `Provider`; health gate ignores per-service health conditions. **Reconfirms RT-12 PARTIAL** | **P2** |
| C07 | `secrets` & `configs` (short vs long-form, external, driver) | `create.go:1082` `buildContainerSecretMounts`/`buildContainerConfigMounts` | `service.go:105` `rawService.Secrets []interface{}` stores string lists only; `ValidateCompose:311` only warns on `External`; no file-existence/target/`uid/gid/mode` validation; fallback discards long-form fields (parser.go via compose-go would decode `types.ServiceSecretConfig` but deploy path discards) | `compose.go` relies on `docker compose up -d` for mounts; `validateComposePolicy` does not validate secrets at all; forge never writes secret source files (only `.env`) → `secrets: {file: ./db_pass.txt}` refers to nonexistent path → `409` | **PARTIAL** | Neither fully supported nor fully rejected — partial warn-only. | **P2** |
| C08 | Restart policy fidelity | `create.go:592` `getRestartPolicy` merges `restart` + `deploy.restart_policy` (`MaxAttempts→MaximumRetryCount`), `mapRestartPolicyCondition:619` | `service.go:516` warns `restart: always may conflict` (warning not error); `parser.go:539` stores raw string; no `deploy.restart_policy` precedence, no `MaximumRetryCount` propagation | `compose.go:92` `validateComposePolicy` has **no** `restart` case (allow-all); `lifecycle.go:781` `RestartStack` is `StopStack`+`StartStack` (2 compose invocations) vs beacon `handleComposeRestart:506` native `docker compose restart` (more faithful but unused by that API handler `handlers_compose.go:470`) | **WIRED_BUT_WRONG** | API warns `always` but beacon honors it, causing `StopStack` to race host-level auto-restart; `on-failure:3` retry count lost; Swarm `deploy.restart_policy.condition` never mapped. **Reconfirms RT-13 PARTIAL + LF-05** | **P2** |
| C09 | Resource limits `deploy.resources` + quotas | `create.go:635` `getDeployResources` → `setLimits:641` (`MemoryBytes,NanoCPUs,Pids,Blkio`) + `setReservations:728` (`MemoryBytes,DeviceRequests`) + `cpu_shares/period/quota/blkio/ulimits/gpus/shm_size` | `parser.go:913` `normalizeResources` maps **only** `cpu` (`NanoCPUs`) + `memory` (`MemoryBytes`) into `map[string]string`; `Pids,BlkioWeight,Gpus,Ulimits,ShmSize,DeviceRequests` dropped; `lifecycle.go:139` stack ceilings (`MaxUserStackMemoryMB 64G/Disk500G/CPUShares8192:150`) checked at `lifecycle.go:280` per-stack only, not per-service; scheduler `PlaceServer:293` uses top-level `MemoryMB/CPUShares/DiskMB` not YAML `deploy.resources` | `compose.go:395` bypasses `docker.go:783` `buildResources` (single-container isolation: period/quota/shares/pids/memory/swap); `docker compose up` honors YAML limits but Forge placement not fed — user can request `memory: 8G` per service while `DeployComposeRequest.MemoryMB` is 512; `replicas:10` not multiplied in quota | **BROKEN** | `memory:100TiB` or `memory:8G ×10 replicas` bypasses reservation; placement over-commits. **Reconfirms RT-14 PARTIAL (two isolation tiers) — still open** | **P1** |
| C10 | Volumes: type system, binds, allowlist | `create.go:862` `buildContainerVolumes`/`buildContainerMountOptions`/`fillBindMounts`/`volumeRequiresMountAPI` (type `bind|volume|tmpfs|image`, SELinux/propagation/recursive/subpath/nocopy/labels, hash via `ensureProjectVolumes:128`) | `parser.go:880` `normalizeVolumesFromCompose` and `service.go:734` `normalizeVolumes` coerce to `source:target[:ro]` strings, dropping `type/consistency/bind.propagation/selinux/recursive/volume.nocopy/subpath/tmpfs.size/mode/image.subpath`; `service.go:595` `checkVolumesSecurity` blocks `docker.sock:620`/`/proc:/sys:627` (error) and warns on sensitive `/ /root /etc /home:679` with `ValidateHostMountWithAllowlist:663` admin+allowlist gate off-path | `compose.go:226` `validateComposeVolumes` strictly **blocks** any absolute bind source (`strings.HasPrefix(source,"/")`) + path-traversal (`isPathTraversal:251`) + any long-form `type:bind:234` (no allowlist param); diverges from `mounts.go:64` `allowedMountSource` (evalSymlinks+rel within `allowed_mounts`) used for game workloads | **BROKEN** | **Trifurcation:** API warns (`/data` warning) yet `POST /compose/validate` passes → beacon `400 policy violation` on same YAML. Game workloads respect `allowed_mounts` allowlist; **compose workloads cannot use allowlisted host binds** — extending `allowed_mounts` to `/opt/appdata` enables portal servers but not compose stacks. **Reconfirms REF-P6-COMP-01 (P0) + RT-15 BROKEN** | **P0** |
| C11 | Networks & IPAM | `create.go:132` `prepareNetworks`+`shouldCreateNetwork` with hash, IPAM `config/subnet/gateway/ip_range`, `internal/attachable/external/enable_ipv6/labels/name`, per-endpoint `ipv4_address/mac/address/aliases/gw_priority` | `service.go:109` `rawNetwork` captures only `driver/external/labels` + `parser.go:692` `normalizeNetworkToForge` (`driver/external/labels/driver_opts`); no `ipam/internal/attachable/enable_ipv6/name`, per-service aliases/IPs lost (parser types would capture if on deploy path) | `compose.go` relies on `docker compose up` defaults (`<project>_<network>` named `<stackID>_<network>`); `docker.go:1065` `ensureNetwork` (managed label+IPAM+concurrent-create race) bypassed for compose | **PARTIAL** | Compose networks isolated per-stack (good) but no pre-validation; `external:true` without pre-existing network surfaces only as docker error | **P3** |
| C12 | Lifecycle `up/down/stop/start/restart/pull/ps/logs/scale` | `up.go:44` create+start+attach, `down.go:38` dep-reverse + network/volume prune opt-in (`options.Volumes`), `restart.go:31` dep-ordered `PreStop/PostStart`+`waitDependencies`, `scale.go:27` `ScaleOptions`, `logs.go:34` `Tail/Since/Until/Timestamps`, `ps.go:32` health/mounts/networks | `lifecycle.go:396` `DeployComposeStack` merges `create+start` without `IgnoreOrphans` handling (except opt-in `RemoveOrphans` doc at `compose.go:290` but not in `daemon.ComposeDeployRequest`); `lifecycle.go:781` `RestartStack` is `Stop+Start`; `lifecycle.go:788` `PullStack` calls `ComposePull:89` unconditionally; `lifecycle.go:625` `GetStackStatus` proxies `ComposeStatus:110` | `compose.go:395` `up -d` (implicit pull), `576` `down -v` **always removes volumes** (hard-coded `-v` at `compose.go:578`), `506` native `docker compose restart` (more faithful) exists but API stop+start sequence does not use `daemon.ComposeRestart:81`; `604` `ps --format json` line-delimited JSON → `{Name,Image,Status,State,Ports}` only; `661` `logs --no-color --tail` plain text, no `Since/Until/Timestamps`; scale has no `POST /compose/:id/scale` route | **PARTIAL (data-loss)** | Delete's unconditional `-v` diverges from upstream opt-in `options.Volumes` — destroys `pgdata:/var/lib/postgresql/data` user expected persisted (Coolify/Dokploy preserve unless asked). Restart uses degraded stop+start (loses `--no-deps`/hooks/crash-relevance filtering). Scale unaffected — `deploy.replicas` not actionable. **Reconfirms prior RECON-05** | **P1** |
| C13 | Interpolation & config-hash reconciliation | `envresolver.go` + `toBakeSecrets`/`resolveAndMergeBuildArgs`/`prepareLabels` with `com.docker.compose.config-hash` via `ServiceHash:500`/`convergence.go` fast-path | `service.go:356` `interpolateEnv` regex `\$\{([^}]+)\}\|\$\$` handles `$$`, `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?msg}`, `${VAR?msg}`; **missing** `${VAR:+alt}`, `${VAR:offset:length}`, bare `$VAR`, nested expansions; `docker.go:899` `configHashLabel` exists for single containers but no compose hash; `compose.go` stacks have no hash label | Beacon `.env` written from raw `EnvVars` map only; `interpolateEnv` server-side substitution + docker compose's own expansion double-apply risk (panel substitutes via `ExpandTemplate`, beacon writes `.env` then docker compose re-expands) | **PARTIAL** | `foo: ${ENABLED:+--verbose}` always expands to `""`; `MaxComposeYAMLBytes:14` mitigates ReDoS on unbounded `[^}]`, but greedy `[^}]` 1 MB still heavy | **P2** |
| C14 | Build context per-service (`build: {context,dockerfile,args,cache_from,platforms,…}` + bake vs classic) | `docker-compose/pkg/compose/build.go:112` `BuildConfig` + `build_bake.go` (groups, platforms, `cache_from/to`, provenance/SBOM, `ssh/secrets/network`, `dockerignore`+`compress`) | `service.go:92` `rawService.Build map[string]interface{}` → `normalizeBuild:787` `(context/dockerfile/args/target)` stored as `BuildSummary` string; `lifecycle.go` never ships source tree | `compose.go:374` only writes `compose.yaml`+`.env`; `./web/Dockerfile` absent → `docker compose up` → `failed to read dockerfile: no such file` → `409` `ComposeOperationResponse{Error,Output:404}`. Generic build service (`build/service.go:530` `executeRemoteBuild`) clones repo to `gitCloneDir/workspaceID` for **non-compose** builds, not wired to compose `build.context` | **MISSING** | Structural: compose stacks with `build:` always fail unless `image:` also supplied and pre-built. Dokploy zips+tarballs context; Coolify clones repo then `docker compose build`; Forge has no tarball/context transfer nor rewrite `build`→`image`. **Reconfirms RECON-06** | **P2** |
| C15 | Build system: Dockerfile/Nixpacks/buildx, cache, platforms, build-args/secrets | Coolify: `nixpacks|railpack|static|dockerfile|dockercompose` (helper `docker/coolify-helper/Dockerfile:54` buildx+nixpacks); Upstream bake with cache/platform auto-detection | Forge builders `BuilderDockerfile|BuilderNixpacks:27` (`forge/api/internal/services/build/service.go:28`) only; `handlers_source_deployments.go` previously rejected nixpacks, now admits dockerfile only path; `executeRemoteBuild:530`→`daemon.GitClone`+`DockerfileBuild`/`NixpacksBuild` | `beacon/build.go:54` `CacheFrom/To:54` + `Platform:56` now **FIXED** after LF-04 (`beacon/internal/server/build.go:126` `isSafeBuildxRef` filter, `dockerfileBuildRequest:54`); `isSafeBuildxRef` allows `type=gha,mode=max` despite registry intent (passes `_`); platform validated via `isSafePlatform:553` | **PARTIAL/FIXED** | `Railpack/static` missing (admitted 422 now, not lie); cache forward fixed; multi-arch bake groups (`platforms:[linux/amd64,linux/arm64]`) not exposed for compose | **P2** |
| C16 | Secrets propagation (build & runtime) | `docker-compose/pkg/compose/create.go:1082` `buildContainerSecretMounts` (long `{source,target,uid,gid,mode}`, driver/template_driver, warns on `uid/gid/mode` unsupported) | `parser.go:42` `rawService.Secrets []interface{}` stores names only; `ValidateCompose:311` checks `sec.External` only; long-form `uid/gid/mode` dropped; `build/service.go:480` `maskCredentials` + beacon `build.go:400` secret scrub exist but `docker --secret` not used | `compose.go` delegates to `docker compose`; source files never written (only `.env:387` perms `0640`) → file-backed secrets fail; `ComposeOperationResponse.Output` carries raw docker stderr which may contain expanded secrets unless scrubbed | **PARTIAL** | Fidelity: secrets accepted as inventory but not honored at runtime; need masking on compose operation output | **P2** |
| C17 | Orchestration semantics: ComposeUp 6-phase vs Beacon single-shot; health & orphan semantics | Komodo `ComposeUp:414` validate→login→pre_deploy→`config` enumerate `StackServiceNames` with replica expansion (`:544`) →build (`:628`)→pull (`:668`)→`down` if `destroy_before_deploy||project_name changed:712`→`up -d`→`post_deploy`; project-name fresh vs deployed (`stack.rs:45`) | `lifecycle.go:188` `WaitForHealthy` + `DeployComposeStack` single `ComposeDeploy` (10 m+2 m health), `UpdateComposeStack:476` re-deploy via `ComposeDeploy` (no separate `create`/`down?`), `DeleteComposeStack:596` `ComposeDelete:86` | `compose.go:344` single `up -d` (no `config` sanitization, no registry login beyond `runtime.RegistryAuth` for single-container, no build/pre/post), `550` `down -v` (+`removeOrphans` opt-in doc but flag not wired through `daemon.ComposeDeployRequest`) | **PARTIAL** | Fidelity far below Komodo's 6-phase; no `pre_deploy`/`post_deploy`/`compose_cmd_wrapper` (`compose.rs:1050`), no `destroy_before_deploy` rename handling; orphan handling partially documented at `compose.go:290` but not fully wired | **P2** |
| C18 | Convergence & volume inheritance, scaling/observability | `create.go:862` `fillBindMounts`, `volumeRequiresMountAPI`/`bindRequiresMountAPI`, `tmpfs.go`, `image` mounts API≥1.48, `scale.go:27` delegating to `create` with `ScaleOptions` | `compose-view.tsx:31` read-only status view (no scale slider); `queue_handler.go:10` handlers exist but never enqueued (`handlers_compose.go:315` calls directly); `handlers_compose.go:413` `/deploy` reuses `UpdateComposeStack` (good, Phase-1 redeploy leak fixed) | `compose.go:314` `composeServiceState` only `{Name,Image,Status,State,Ports}` vs upstream `api.ContainerSummary` (health/mounts/networks); `docker.go:783` anonymous-volume inheritance from `inherit` container not relevant to compose but indicates gap | **PARTIAL** | No `ScaleStack` (`deploy.replicas` parsed but no `/scale` endpoint), no `watch`/`viz`/`hooks`; `compose-view.tsx` polls via `useQuery` not WS | **P3** |

---

## 4. Logic Findings (file:line — still open unless marked FIXED)

### LF-01 — CRITICAL: `env_file` silently ignored → runtime misconfiguration (reconfirmed still BROKEN)

- **Location:** `forge/api/internal/services/compose/service.go:92` (`rawService` missing `EnvFile`), `forge/api/internal/services/compose/parser.go:245` loader not on deploy path (`service.go:145`, `lifecycle.go:264`), `beacon/internal/server/compose.go:77` policy (`validateComposePolicy` no `env_file` case) / `:380` `encodeComposeEnv` only, `beacon/internal/server/compose.go:226` volume validation unrelated
- **Upstream expectation:** `types.ServiceConfig.EnvFiles` loaded via `cli.WithDotEnv`/`WithEnvFiles`/`envresolver.go:30` before `environment:` merge.
- **Forge behaviour:** `env_file: ./app.env` arrives, is unmarshalled into nothing, stripped from `ParsedCompose` summary, not written to beacon. `ValidateComposeSecurity:427` has no branch for it → no warning or error. Deployed containers lack required env (DB passwords) → `up -d` succeeds but app crashes. Import preview via `parser.go:364` `ParseComposeString` *does* understand `env_file`, so import shows valid while deploy silently omits vars.
- **Repro:**
  ```yaml
  services:
    web:
      image: nginx:alpine
      env_file: [./frontend.env]
      environment: [PORT=3000]
  ```
  Expected merge from `frontend.env`; Forge: only `PORT=3000` + `EnvVars` map.
- **Cross-refs:** `REF-P6-COMP-02` **P0** BROKEN still open; `final-parity RT-11` BROKEN still open; `phase-06 C02` CRITICAL — all reconfirmed on current checkout; Coolify/Dokploy explicitly disallow host `env_file` and replace with UI-managed env injections — Forge neither rejects nor supports.
- **Fix (either):** (a) Route `DeployComposeStack`/`DeployFromGit` through compose-go loader (`parser.go:245`) with `WithEnvFiles` and copy referenced `env_file` hosts into `stackDir` before `handleComposeDeploy`, **or** (b) fail-fast: reject `env_file` at `ValidateCompose:240` with `severity:error` and document as unsupported. On beacon, ensure `docker compose` can see written `env_file` files under `stackDir`.
- **Phase-06 synthesis note:** Explicitly called out as "LF-01 Critical — env_file silently dropped. `env_file:` interpolated via `loader.go:36` + `.env` + `${VAR:-default}`. Forge implements `${VAR}` subset, but `env_file` entries remain in YAML and are never mounted/resolved." — **no remediation detected.**

---

### LF-02 — HIGH: `deploy.resources.limits`/`reservations` + `replicas` not multiplied in placement; beacon enforcement bypass

- **Location:** `forge/api/internal/services/compose/parser.go:913` `normalizeResources` (only `cpu` `NanoCPUs` + `memory` `MemoryBytes`; `Pids/Devices/Blkio/Gpus/Ulimits/ShmSize` dropped), `forge/api/internal/services/compose/lifecycle.go:139` ceilings + `:280` stack cap check + `:293` `PlaceServer{CPU:CPUShares, MemoryMB, DiskMB}` top-level only, `beacon/internal/runtime/docker.go:783` `buildResources` (single-container path) vs `beacon/internal/server/compose.go:395` compose bypass
- **Logic:** Example
  ```yaml
  services:
    api:
      image: api:latest
      deploy:
        resources:
          limits: {cpus: '4.0', memory: 8G, pids: 100}
          reservations: {memory: 2G}
        replicas: 3
  ```
  Forge fallback normalizes to `Limits:{"cpu","memory"}` for display only; `pids/cpuset/blkio/ulimits/gpus/shm` dropped. Placement uses `DeployComposeRequest{MemoryMB,CPUShares,DiskMB}` globals, never per-service `deploy.resources`. A user can request `memory:8G` per replica while `DeployComposeRequest.MemoryMB` is low (default 512, `import.go:132`) → reservation succeeds but `docker compose` requests more → host OOM or hidden over-commit. `replicas:10 × memory:8G` exceeds per-stack ceiling `8192` shares not checked. `validateComposePolicy:92` never checks `deploy.resources`.
- **Cross-refs:** `phase-06 C09` HIGH, `final-parity RT-14` PARTIAL still open. **Evidence still holds** (inspected `parser.go:914-943` vs `create.go:743` `setLimits` including `PidsLimit`).
- **Fix:** Enforce `sum(per-service limits × replicas)` ≤ `MaxUserStack*` at `ValidateCompose` or placement time; document dual-tier isolation (game `ReadonlyRootfs/CapDrop` vs compose pass-through) in admin UI.

---

### LF-03 — HIGH: Volume policy bifurcation → host bind-mount inconsistency + data-loss on `down`

- **Location:** `forge/api/internal/services/compose/service.go:595` `checkVolumesSecurity` (warns `/, /root, /etc, /home:679`; `ValidateHostMountWithAllowlist:663` admin+allowlist gate off-path) vs `beacon/internal/server/compose.go:226` `validateComposeVolumes` (blocks any absolute host bind `strings.HasPrefix(source,"/")` + `type:bind` + `isPathTraversal:251`) plus `beacon/internal/server/mounts.go:64` `allowedMountSource` allowlist for game path, `beacon/internal/server/compose.go:576` `down -v` hard-coded.
- **Split #1 — Validation mismatch:** API security flags `/etc` as error (`service.go:642`), allows `/data` as warning → `ValidateCompose` still `Valid:true`. Beacon rejects `/data` unconditionally → `POST /compose/validate` can pass with `valid:true` then deploy fails `400 compose policy violation: host bind mount source "/data" not allowed`. No allowlist recourse (beacon ignores `AllowedMounts`).
- **Split #2 — Allowlist bypass inconsistency:** Game workloads respect `DAEMON_ALLOWED_MOUNTS` (`mounts.go:64` `EvalSymlinks`); compose workloads cannot use allowlisted host mounts — `validateComposeVolumes` has no `allowed` param.
- **Split #3 — Data-loss:** `handleComposeDelete:576` always `down -v` (`compose.go:578`) unconditional vs upstream `down.go:112` opt-in `options.Volumes`. Forge deletes named volumes (e.g., `pgdata:/var/lib/postgresql/data`) even when user expected persistence. Coolify explicitly preserves volumes unless checked "Delete volumes".
- **Cross-refs:** `REF-P6-COMP-01` **P0** BROKEN (bifurcation `200 valid→400`) still open; `final-parity RT-15` BROKEN still open; `phase-06 C10` HIGH + `C12 down` data-loss still open. **No remediation detected** (inspected `compose.go:226-248` still no allowlist plumbing; `compose.go:578` still `-v` hard-coded).
- **Fix:** Unify: plumb `allowedMounts+isAdmin` through `composeDeployRequest` into `validateComposeVolumes(..., allowed, isAdmin)` or make API reject all absolute mounts (`error`) matching beacon until plumbing exists; add `?keepVolumes` query for `DeleteComposeStack` and make `-v` opt-in.

---

### LF-04 — HIGH: Build context not shipped for compose `build:` services → silent build failure

- **Location:** `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` (git clone → `DockerfileBuild`/`NixpacksBuild` for generic builds only), `beacon/internal/server/compose.go:374` only `compose.yaml`+`.env` written, `forge/api/internal/services/compose/parser.go:566` stores `Build` as `BuildSummary`, `daemon/compose.go:48` no tarball/context fields.
- **Flow:** Stack
  ```yaml
  services:
    web:
      image: myapp:local
      build: {context: ./web, dockerfile: Dockerfile, args: [NODE_ENV=production]}
  ```
  API `ParseComposeYAML:145` captures `build.context` as string, persists YAML. Beacon writes `compose.yaml` with `build: {context: ./web}` into `compose/<id>` and runs `docker compose up -d`. `stackDir` lacks `./web/Dockerfile`+source → docker errors `failed to read dockerfile: open .../compose/<id>/web/Dockerfile: no such file` → beacon `409` `ComposeOperationResponse{Error, Output:404}` → `lifecycle.go:402` `markFailed`. From user perspective, locally-valid compose fails with opaque `Output`.
- **Cross-refs:** `phase-06 C18` & `LF-04`, `final-parity C18` MISSING still open; Dokploy zips+tarballs context+uploads, Coolify clones repo before `docker compose build`; Forge `build.Service` generic path not wired to compose stacks (`gitops.go:196` single-file read).
- **Fix:** Either tarball/transfer build context via existing `daemon.Client` + pre-build images and rewrite `build.context→image` before deploy, **or** fail-fast: reject `build:` at `ValidateCompose` (`severity:error`) unless `image:` also present.

---

### LF-05 — MEDIUM: Restart policy divergence + `RestartStack` lifecycle race

- **Location:** `forge/api/internal/services/compose/service.go:516` warn `restart:always` only; `beacon/internal/server/compose.go:92` no restart check; `forge/api/internal/services/compose/lifecycle.go:781` `RestartStack` as `StopStack`+`StartStack`; `beacon/internal/server/compose.go:506` `handleComposeRestart` native `docker compose restart` (correct) **not** called by API; `forge/api/internal/http/handlers_compose.go:470` `POST /compose/:id/restart` calls API `RestartStack` (stop+start).
- **Evidence:**
  ```go
  // lifecycle.go:781
  func (s *Service) RestartStack(ctx context.Context, stackID string) (*ComposeStack, error) {
    if _, err := s.StopStack(ctx, stackID); err != nil { return nil, err }
    return s.StartStack(ctx, stackID)
  }
  ```
  vs upstream `restart.go:77` dep-ordered `ContainerRestart` with `PreStop/PostStart` + `waitDependencies`.
- **Issue A — policy:** `restart:always` containers auto-restart via `dockerd`; `StopStack` (`docker compose stop:443`) will stop but host policy immediately restarts, so `StartStack` sees already running and `WaitForHealthy:188` flaps. Upstream guidance: `restart: unless-stopped` for Compose-managed stacks; Forge warns at API but lets beacon enforce.
- **Issue B — lifecycle:** API `/restart` calls stop+start (two compose invocations, losing `restart: false` filtering and `PreStop/PostStart` hooks) instead of single `docker compose restart` (which respects `--no-deps` semantics). `daemon.ComposeRestart:81` exists but is unused by this handler.
- **Issue C — `MaximumRetryCount`:** `getRestartPolicy:619` merges `deploy.restart_policy.condition`+`max_attempts`→`container.RestartPolicy{Mode,MaximumRetryCount}`; Forge never maps — Swarm-style `deploy.restart_policy: {condition:on-failure, max_attempts:3}` stored but not translated.
- **Cross-refs:** `phase-06 C08` WIRED_BUT_WRONG + `LF-05`, `final-parity RT-13` PARTIAL still open. **No remediation detected** (inspected `lifecycle.go:781` still stop+start; `compose.go:506` native handler still not called from API path).
- **Fix:** Call `daemon.ComposeRestart:81` from `RestartStack` (single native `docker compose restart`), add `validateComposePolicy` warning for `restart:always`, and map `MaximumRetryCount` for `on-failure:N` or enforce `unless-stopped` default with audit note.

---

### LF-06 — MEDIUM: `shortFormHostPort` privileged-port bypass (host port hijack)

- **Location:** `beacon/internal/server/compose.go:212` `shortFormHostPort` + `:181` `validateComposePorts`
  ```go
  // compose.go:214
  func shortFormHostPort(entry string) string {
    parts := strings.Split(entry, ":")
    switch len(parts) {
    case 2: return parts[0]
    case 3: return parts[1]
    default: return "" // ← handles "80" (len1) and "80:80/tcp" protocol suffix incorrectly
    }
  }
  // compose.go:198 validateComposePorts: if published=="" { continue } → skip check
  ```
- **Effect:** `ports: ["80"]` (or `["80:80"]` interpreted as ephemeral vs fixed depending on engine), `["127.0.0.1::80"]`, or long-form `published:80` still caught but short-form single-value falls through `published==""` → **no privileged-port error** → attacker binds host `80`/`22` on shared nodes. API `ValidateComposeSecurity:427` has **no** privileged-port check at all; only beacon does, and it skips.
- **Cross-refs:** `REF-P6-COMP-03` **P1** BROKEN (`short-form ["80"] bypasses privileged-port check`) still open; `final-parity RT-05/C05` noted same. **No remediation detected** (inspected `compose.go:212-224` still `default: return ""`).
- **Fix:** Handle `len==1` as `parts[0]` after stripping `/tcp`/`/udp` suffix and using `nat.ParsePortSpec` or compose-go port parsing; validate long-form `published` for single-value strings; add API-side check parity.
- **Test sketch:** `["80"]`, `["80:80"]`, `["8080:80/tcp"]`, long-form `{target:80, published:22}`, varying protocols; assert `400 policy violation`.

---

### LF-07 — MEDIUM: `DeployFromGit` persists new stack via `UpdateComposeStack` instead of `CreateComposeStack` (fresh-ID upsert assumption)

- **Location:** `forge/api/internal/services/compose/gitops.go:351` `stackID := g.compose.createStackID()` → `:384` `if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil { _, _ = g.store.UpdatePlacementReservationStatus(ctx,reservation.ID,Cancelled); return nil, fmt.Errorf("create compose stack record: %w", err) }` vs correct `forge/api/internal/services/compose/lifecycle.go:390` `store.CreateComposeStack`
- **Logic:** Allocates fresh ID and calls **Update** on a row that does not exist. Unless the store implements upsert, this returns `not found`/`0 rows` and surfaces `"create compose stack record"` while reservation is cancelled but no stack persisted. Idempotent caller sees generic error; retry leaks reservations. `redeployFromGit`/`pullAndRedeploy` correctly use update because they mutate existing rows. Reconfirmed on current checkout: `gitops.go:384` still calls `UpdateComposeStack`.
- **Cross-refs:** `REF-P6-COMP-04` **P1** BROKEN (`DeployFromGit calls Update on fresh stackID`) still open; `phase-06 C17`/`LF-02` same. **No remediation detected.**
- **Fix:** Change `gitops.go:384` to `CreateComposeStack`. Add integration test that `ListComposeStacks` contains the new stack after `POST /compose/git/deploy`.

---

## 5. Convergence / Scaling / Secrets — Remaining Spec Details

- **Convergence (`convergence.go`):** `volumesFrom: container:ID` rewiring + anonymous-volume inheritance from `inherit` container — **not** relevant to Forge compose (isolated projects) but indicates Forge's volume-isolation via `ensureProjectVolumes` is delegated to `docker compose`. No gap beyond delegated behaviour.
- **Scaling:** Upstream `scale.go:27` `ScaleOptions` delegates to `create` with replica counts. Forge has **no `ScaleStack` API**; `parser.go:566` stores `deploy.replicas` but no `/scale` route (`handlers_compose.go` has none). Both `DeployComposeStack` ceilings and scheduler placement never multiply by replicas (see LF-02). Coolify/Dokploy both expose replica sliders.
- **Secrets pull vs build:** `ref-P6-COMP-05..PLUG-01` not in scope here; secret scrubbing at build logs is now **FIXED** (`beacon/build.go:400` per-pattern `****`, `build/service.go:480` `maskCredentials`) but compose operation output (`compose.go:404-408` → `lifecycle.go:402` → stack `Error` field) still carries raw `docker compose` stderr — should redact if it contains `environment:` values expanded into logs (`encodeComposeEnv:264` writes `.env 0640` correctly, but API response `Output` should be scrubbed).

---

## 6. Reverification Summary — Still Open vs Fixed

| MASTER / Final-Parity ID | Title | Previous Severity | Current Verdict (reverified) |
|---------------------------|-------|-------------------|------------------------------|
| **REF-P6-COMP-01** `service.go:594 vs compose.go:226` | Volume policy bifurcation (`200 valid→400`) + `-v` data-loss | **P0** | **STILL BROKEN** — API warning vs beacon hard-reject mismatch; `compose.go:578` `-v` still hard-coded; `validateComposeVolumes` still no allowlist |
| **REF-P6-COMP-02** `compose/service.go:14` | `env_file` silently dropped | **P0** | **STILL BROKEN** — `rawService:92` still no `EnvFile`; deploy path still fallback parser; beacon `encodeComposeEnv:264` only |
| **REF-P6-COMP-03** `beacon/compose.go:213` | Short-form `["80"]` bypasses privileged port check | **P1** | **STILL BROKEN** — `shortFormHostPort:214` still `default: return ""` |
| **REF-P6-COMP-04** `gitops.go:381` | `DeployFromGit` calls `Update` on fresh ID | **P1** | **STILL BROKEN** — `gitops.go:384` still `UpdateComposeStack` on create path |
| **RT-11** (final-parity) | Env interpolation+`env_file` | BROKEN | **STILL BROKEN** (LF-01) |
| **RT-12** | Service deps ordering | PARTIAL (delegated) | **STILL PARTIAL** — delegated to engine, pre-flight cycle fail-fast missing |
| **RT-13** | Restart policies | PARTIAL (inconsistent) | **STILL PARTIAL/WIRED_BUT_WRONG** — LF-05 still stop+start vs native restart |
| **RT-14** | Resources / isolation tiers | PARTIAL (two tiers) | **STILL PARTIAL** — LF-02 per-service limits×replicas not enforced |
| **RT-15** | Mount confinement | BROKEN (bifurcated) | **STILL BROKEN** — LF-03 |
| **phase-06 C14** | Build context missing | MISSING | **STILL MISSING** — LF-04 |
| **phase-06 C15/C16** | Build cache/platform | PARTIAL→FIXED (LF-04) | **VERIFIED FIXED** — `beacon/build.go:126` now forwards `CacheFrom/To` with `isSafeBuildxRef`; platform via `isSafePlatform:553` |
| **phase-06 Komodo 6-phase** | 6-phase `ComposeUp` parity | — | **STILL DELEGATED** — Forge single-shot `up -d` is single-phase; no `config` sanitization/build/pull/pre/post/wrapper |

No product files modified. No new P0s beyond the four already indexed; recomputed risk unchanged. Recommended fix order: **LF-01 (P0 env_file) → LF-03 (P0 volume bifurcation) → LF-02 (P1 limits×replicas) → LF-06 (P1 port bypass) → LF-07 (P1 fresh-ID upsert) → LF-04 (P2 build context) → LF-05 (P2 restart lifecycle).**

---

## 7. File:Line Index (key citations, current checkout)

- `reference/app-platforms/docker-compose/pkg/compose/create.go:592` `getRestartPolicy` + `:619` `mapRestartPolicyCondition`
- `reference/app-platforms/docker-compose/pkg/compose/create.go:635` `getDeployResources` + `:641` `setLimits` + `:728` `setReservations` + `:862` volume helpers
- `reference/app-platforms/docker-compose/pkg/compose/dependencies.go:17` `NewGraph` + `InDependencyOrder:78`
- `reference/app-platforms/docker-compose/pkg/compose/convergence.go:88` `waitDependencies`
- `reference/app-platforms/docker-compose/pkg/compose/loader.go:36` + `pkg/compose/envresolver.go`
- `forge/api/internal/services/compose/service.go:14` `MaxComposeYAMLBytes` + `:92` `rawService` (no `env_file`) + `:356` `interpolateEnv` + `:346` `composeVarRe` + `:427` `ValidateComposeSecurity` + `:595` `checkVolumesSecurity` + `:663` `ValidateHostMountWithAllowlist` + `:679` `sensitiveHostPaths` + `:516` `restart:always` warning + `:765` `normalizeDependsOn` + `:841` `normalizeDeploy`
- `forge/api/internal/services/compose/parser.go:245` `ParseComposeYAML` (compose-go wrapper) + `:364` `ParseComposeString` + `:566` `normalizeDependsOn` display + `:913` `normalizeResources` (cpu+memory only)
- `forge/api/internal/services/compose/lifecycle.go:139` ceilings + `:188` `WaitForHealthy` + `:249` `DeployComposeStack:280` ceiling+placement + `:476` `UpdateComposeStack` + `:570` `DeleteComposeStack` + `:781` `RestartStack` stop+start + `:188` 2 m health gate
- `forge/api/internal/services/compose/gitops.go:196` `readComposeFromDir` + `:287` `readLimitedComposeFile` + `:301` `DeployFromGit:384` `UpdateComposeStack` bug + `:687` `DetectDrift:841` + `:755` `DetectRuntimeDrift` + `:1151` `HandleWebhook:1176` delivery dedup + `:1280` `PollForUpdates`
- `forge/api/internal/services/compose/queue_handler.go:49` `HandleDeploy/Update/Delete/Start/Stop/Restart` (unused on hot path)
- `forge/api/internal/daemon/compose.go:13` `ComposeDeployRequest` + `:48` `ComposeDeploy` + `:81` `ComposeRestart` + `:110` `ComposeStatus` + `:131` `ComposeLogs`
- `beacon/internal/server/compose.go:59` `validStackID` + `:77` `validateComposePolicy` + `:181` `validateComposePorts` + `:212` `shortFormHostPort` (bypass) + `:226` `validateComposeVolumes` (no allowlist) + `:251` `isPathTraversal` + `:264` `encodeComposeEnv` + `:283` `dirForID` + `:344` `handleComposeDeploy` + `:418` `handleComposeStop` + `:462` `handleComposeStart` + `:506` `handleComposeRestart` + `:550` `handleComposeDelete:578` `down -v` + `:604` `handleComposeStatus` + `:661` `handleComposeLogs` + `:709` `handleComposePull`
- `beacon/internal/server/mounts.go:64` `allowedMountSource` (game-path allowlist, not used by compose)
- `beacon/internal/runtime/docker.go:783` `buildResources` + `:827` `buildHostConfigWithSettings` (isolation tier for game workloads)
- `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` (clone→build, not wired to compose `build.context`)
- `beacon/internal/server/build.go:54` `dockerfileBuildRequest.CacheFrom/To` + `:126` `isSafeBuildxRef` + `:553` `isSafePlatform` + `:44` `maxLogBufferSize`
- `forge/api/internal/http/handlers_compose.go:66` `registerComposeRoutes` + `:106` `POST /compose/validate` + `:315` `POST /compose` + `:470` `POST /compose/:id/restart` (stop+start wrapper)

---

*Generated by subagent 09 (reverification, parallel 09/20). No product files modified. Re-inspected 2026-08-24 against `phase-06 subagent-10-compose-builds.md`, `phase-06 subagent-07-komodo`, `final-parity subagent-04 RT-11..RT-15`, `MASTER FINDING INDEX REF-P6-COMP-01..04`.*
