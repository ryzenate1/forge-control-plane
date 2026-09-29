# Subagent 09 — Dokku + CapRover Minimal PaaS Patterns vs Forge App Deployment

**Scope:** `reference/app-platforms/dokku` (plugins/*, scheduler-docker-local, git handling) and `reference/app-platforms/caprover` (ImageMaker, ICaptainDefinition, AppsDataStore, ServiceManager, DockerRegistryHelper, one-click) vs `forge/api/internal/services/apphosting,deployment,build,git` + `forge/web/app/admin/apps,deployments,preview-deployments` + plugin model + forgefile/forge_manifest.

**Date:** 2026-08-23 | **Author:** subagent-09 (read-only audit)

---

## 1. Executive Summary

Dokku and CapRover are single-host, opinionated, git-push-to-deploy PaaS systems. Dokku achieves extensibility via a shell+Go **plugn** trigger bus with ~60 discrete plugins; CapRover uses a monolithic TypeScript `ServiceManager` + `ImageMaker` pipeline centered on a single `captain-definition` file. Forge re-implements the app-hosting layer as a multi-tenant, multi-node control plane (Postgres-backed `applications`/`app_services`/`deployments` + `build` records) with a rich `forgefile` (forge.yaml) manifest. The gap analysis surfaces that Forge's plugin system is currently **metadata-only** (no runtime), its build/deploy paths are honest but more complex than the minimal PaaS models, and it inverts several simplicity trade-offs that make Dokku/CapRover trivial to operate.

---

## 2. Comparative Matrix (14 comparisons)

### C01 — Plugin Architecture: Dokku `plugn` trigger bus vs Forge `plugins` service vs CapRover's zero-plugin monolith

| Aspect | Dokku | CapRover | Forge |
|---|---|---|---|
| Mechanism | `plugn trigger <name> <args>` shell dispatch; every plugin is a directory with executable hook files (`plugins/20_events/*`, `plugins/apps/triggers.go:10`, etc.) | No plugin system at all; all behaviour in `ServiceManager.ts:50` + `ImageMaker.ts:77` | In-process Go `PluginStore` + `HookHandler` map (`forge/api/internal/services/plugins/plugin.go:69-77`) + HTTP import handlers (`forge/api/internal/http/handlers_plugins.go:48`) |
| Discovery | `plugins/plugin/plugin.go:30-51` lists via `plugn list` + enriches git source; filesystem is truth | N/A | `plugin.go:229-283` discovers `manifest.json` directories; DB is truth (`store_plugins.go:51`) |
| Extensibility | Fully open: `dokku plugin:install https://github.com/...` clones repo, enables triggers | One-click templates only (`OneClickAppDeployManager.ts:33`) | Manifest import from file/URL (`handlers_plugins.go:84,106`) but no execution |

**Takeaway:** Dokku's plugin model is shell-deployable and composes via ordered trigger files; Forge attempted to mirror it but stopped at metadata import (see Logic Finding LF-01).

### C02 — App Definition / Identity

- **Dokku** `apps/apps.go:5` — filesystem truth: `$DOKKU_ROOT/<app>` directory + `VHOST`/`ENV`/`DOKKU_SCALE` files. No DB. `apps` plugin `DefaultProperties` is 3 keys (`deploy-source`, `disable-autocreation`). Creation is `TriggerAppCreate` → `createApp()` → `mkdir $DOKKU_ROOT/app` (`plugins/apps/triggers.go:10`). Name validation `common/common.go:994` regex `^[a-z0-9][a-z0-9.-]*$`.
- **CapRover** `models/AppDefinition.ts:45` (`IAppDefinitionBase`) — JSON blob persisted via `configstore` (`AppsDataStore.ts:305-348`). Richer than Dokku: `instanceCount`, `captainDefinitionRelativeFilePath`, `volumes`, `ports`, `customNginxConfig`, `preDeployFunction`, `serviceUpdateOverride`, `appDeployTokenConfig`. Name regex `AppsDataStore.ts:30` `^[a-z0-9\-]+$` (no dots, no `--`). Persistent vs non-persistent flag (`hasPersistentData: boolean`).
- **Forge** `forge/api/internal/services/apphosting/service.go:51-60` — `CreateAppRequest` with `SourceType ∈ {GIT,DOCKER_IMAGE,COMPOSE}` (`service.go:81-85`), org-tenancy aware (`AppBelongsToOrg`), Postgres-backed `applications` + `app_services` rows. `forge/web/app/admin/apps/page.tsx:16` exposes `AppType ∈ {image, git, compose, game_server}` vs CapRover's `hasPersistentData` boolean and Dokku's untyped app. Forge separates Application → AppService (Compose-like) whereas Dokku/CapRover are 1 app = 1 service (+ sidecars via volumes).

### C03 — Build / Builder Detection

- **Dokku**: Multi-builder chain via `builder-detect` trigger. `plugins/builder/builder.go:5` tracks `build-dir`, `selected`. `common/plugn.go:42` `CallPlugnTrigger("builder-detect",...)` iterates `builder-dockerfile`, `builder-herokuish`, `builder-nixpacks`, `builder-pack`, `builder-lambda`. `git/functions: git_trigger_build` falls back `BUILDER="herokuish"` → `pack` on arm64. Output is a Docker image tag `dokku/<app>:latest`.
- **CapRover**: Single `ImageMaker.ts:386-431` `getCaptainDefinition()` enforces `schemaVersion === 2` and **exactly one** of `{templateId,imageName,dockerfilePath,dockerfileLines}` (`ImageMaker.ts:416-427`). If `imageName` set, pulls image directly (`ImageMaker.ts:154-181`); otherwise converts captain-definition → `Dockerfile` via `convertCaptainDefinitionToDockerfile` (`ImageMaker.ts:433-479`) → tars directory (`ImageMaker.ts:586-600`) → `dockerApi.buildImageFromDockerFile` → `retagAndPushIfDefaultPushExist` (`DockerRegistryHelper.ts:25-110`). Fallback to plain `Dockerfile` if `captain-definition` missing (`ImageMaker.ts:482-535`).
- **Forge**: `forge/api/internal/services/build/service.go:140-163` struct with `builders map[BuilderType]Builder` containing `DockerfileBuilder` + `NixpacksBuilder` (`service.go:160-161`). `Detect()` checks for `Dockerfile` existence, else nixpacks indicators (`service.go:348-356`). Node-aware via `NodeCapabilitySelector` (`service.go:1272-1317`) and `selectNode` (`service.go:358-363`). Remote execution via Beacon (`executeRemoteBuild:530-734`) with `GitClone` → `DockerfileBuild`/`NixpacksBuild` → `InspectImageDigest`. Honest failure when `RuntimeExecutor` nil (contrast Dokku/CapRover which assume local docker).

### C04 — Deployment / Release Pipeline

- **Dokku**: `release-and-deploy` trigger is the central deploy transaction: `common/plugn.go:42` calls `release-and-deploy` → builder `builder-release` → `scheduler-docker-local` deploys container + `proxy-build-config`. `ps/ps.go:78-114` `Rebuild`/`Restart` are thin wrappers over `CallPlugnTrigger("receive-app")` / `"deploy"`. No revision table; `DOKKU_SCALE` + image tags are state.
- **CapRover**: `ServiceManager.ts:112-231` `scheduleDeployNewVersion()` → `createNewVersion()` (bump `versions[]`, prune `maxVersionHistory`) (`AppsDataStore.ts:591-634`) → `imageMaker.ensureImage()` → `setDeployedVersionAndImage()` (`AppsDataStore.ts:541-589`) → `ensureServiceInitedAndUpdated()` which creates/updates a Docker Swarm service (`DockerApi.createServiceOnNodeId` / `updateService`) and `reloadLoadBalancer()` (`ServiceManager.ts:888-1000`). Queuing via `activeOrScheduledBuilds` + `queuedBuilds[]` (`ServiceManager.ts:73-75,118-172`) — at most one build globally (`isAnyBuildRunning`).
- **Forge**: DAG-based `deployment` service (`service.go:113-120`, `execution.go:22-157`). `StartRollout()`/`recreateRollout()`/`rollingRollout()`/`blueGreenRollout()`/`canaryRollout()` (`rollout.go:129-320`) validate image digest (`validateImageRef:322-330` requires `@sha256:`), claim execution lease (`execution.go:23-30`), create `DeploymentStep` rows (`steps.go`), then `ExecuteDeployment` loops `executeStep` → `markStepStarted`/`markStepCompleted` with progress + timeout. `RuntimeExecutor` interface (`execution.go:254-262`) is the only bridge to real runtime. Supports blue-green / canary / rolling / recreate; Dokku/CapRover only have recreate (Dokku's docker-local) + swarm rolling (CapRover).

### C05 — Git Handling / Push-to-Deploy

- **Dokku**: Git receive is the **primary** ingress. `plugins/git/functions: git_build_app_repo()` → `fn-git-setup-build-dir` → `git_trigger_build` → `core-post-extract` → `builder-detect` → `dokku_receive`. Hook is `plugins/git/subcommands/default` via SSH `git-receive-pack` (authorized keys + `git:initialize`). `deploy-source-set` records `apps:deploy-source` property (`plugins/apps/triggers.go:18-23`). No provider OAuth, no webhook.
- **CapRover**: Git is **secondary** source. `ImageMaker.ts:336-349` clones via `GitHelper.ts:35-114` using `simple-git` with HTTPS (embed `user:pass`) or SSH (temp key file + `ssh-keyscan` to `known_hosts`, `GitHelper.ts:47-78`). `RepoInfo` stored encrypted (`AppsDataStore.ts:223-232`). Alternative sources: tarball, captain-definition content (`extractContentIntoDestDirectory:305-368`). No GitHub/GitLab API, no webhook; app is built per-deploy, not auto on push.
- **Forge**: `forge/api/internal/services/git/service.go:231` is canon: `GenerateDeployKeyPair` (ed25519/RSA), `ListProviderRepos/Branches` for GitHub/GitLab/Bitbucket/Gitea (`service.go:154-229`), `SetupProviderWebhook` (`service.go:205-229`), HMAC verification (`VerifyGitHubSignature:248`, `VerifyGitLabSignature:262`, `VerifyGiteaSignature:320`, `computeHMAC:858`). Provider URL SSRF protection (`validateProviderURL:303-318` checks HTTPS + DNS → no loopback/private). Webhook → `HandleWebhookTrigger:864-868` → `UpdateGitSourceDeploy`. This is a full provider-integrated gitops layer neither Dokku nor CapRover attempts.

### C06 — Proxy / Routing / vhosts (dokku proxy / nginx-vhosts vs CapRover LoadBalancerManager vs Forge trafficmanager)

- **Dokku**: `proxy/proxy.go:29-91` `BuildConfig`/`ClearConfig`/`Enable`/`Disable` + `IsAppProxyEnabled`. Pluggable backends: `nginx-vhosts`, `caddy-vhosts`, `haproxy-vhosts`, `traefik-vhosts`, `openresty-vhosts` (see `plugins/nginx-vhosts/*`, `plugins/caddy-vhosts/*`). Per-app `VHOST` file (`domains/report.go:42`). `proxy-build-config` trigger is called on every deploy (`ps/ps.go:223-227` after `Start`).
- **CapRover**: Single nginx config via `LoadBalancerManager` (`ServiceManager.ts:996-1000` `reloadLoadBalancer()`) → `LoadBalancerManager.rePopulateNginxConfigFile()`. Template `template/server-block-conf.ejs` + `root-nginx-conf.ejs`. SSL via `CertbotManager` + `enableSslForApp` (`ServiceManager.ts:337-388`) requiring root SSL first (`ERROR_FIRST_ENABLE_ROOT_SSL:33`).
- **Forge**: Modular `trafficmanager` supporting `caddy_proxy`, `traefik_proxy`, `gateway_adapter` (`api/internal/services/trafficmanager/{caddy_proxy.go,traefik_proxy.go,gateway_adapter.go}`) + `loadbalancer` dataplane (`loadbalancer/dataplane.go`). More nodes/tenants aware; not just per-app nginx reload.

### C07 — Scheduler / Runtime Backend

- **Dokku**: `scheduler-docker-local` is default (`common/common.go:468` `return "docker-local"`). Others: `scheduler-k3s`, `scheduler-null`. Scheduler interface is trigger-based (`plugins/20_events/*` lists ~20 scheduler hooks: `scheduler-deploy`, `scheduler-stop`, `scheduler-is-deployed`, `scheduler-enter`, etc.). `ps/ps.go:148-249` `Restore`/`Start`/`Stop` delegate to `scheduler-pre-restore`/`scheduler-stop`/`proxy-*`. Storage exec integration (`scheduler-docker-local/storage_exec.go`).
- **CapRover**: Docker Swarm only (`DockerApi.ts` swarm init `initSwarm`, `createServiceOnNodeId`, `updateService` with `IDockerUpdateOrders`, `IDockerUpdateOverride` via `preDeployFunction`). Node pinning via `nodeId` (`ServiceManager.ts:669-702` persistent apps require nodeId else infer from running service).
- **Forge**: Multi-runtime `forge/api/internal/runtime/{docker.go, kubernetesadapter.go, nomad..., lxc..., firecrackeradapter.go, ...}` + `scheduler/{scheduler_k3s.go,scheduler_nomad.go,scheduler_factory.go}`. Build node selection via capabilities (`build/service.go:1272-1317`). Placement engine (`placement/*`), evac planner, etc. Architecturally furthest from minimal PaaS.

### C08 — Configuration / Environment Variables

- **Dokku**: `config/config.go:33-105` `SetMany`/`UnsetMany`/`UnsetAll` with `loadAppOrGlobalEnv` merging global+app env files (`Env` type). Writes to filesystem (`0600`), triggers `post-config-update` + optional `release-and-deploy` if `ps:restore != false` (`config.go:134-156`).
- **CapRover**: `AppDefinition.envVars: IAppEnvVar[]` (`AppDefinition.ts:9`) mutated via `updateAppDefinitionInDb:677-832` (trims, drops empties). Injected into build (`ImageMaker.ts:131-140` adds `CAPROVER_GIT_COMMIT_SHA`) and service update (`ServiceManager:dockerApi.updateService(..., app.envVars, ...)` `ServiceManager.ts:967-971`).
- **Forge**: `envvars`, `envgroups`, `envmanifest`, `configvalidator` services; `forgefile` `deploy[].env` (`forgefile/service.go:78`). Current `forgefile` persists secrets in manifest content column (DB `forge_manifests.content`) with no masking separation — see LF-03.

### C09 — Domain / TLS / Certs

- **Dokku**: `domains` plugin (`plugins/domains/*`) manages `VHOST` lines; `certs` plugin (`plugins/certs/*`) `add`/`remove`/`show` cert files. No auto-ACME in core (letsencrypt is a community plugin). `proxy-type` + `app-urls` triggers.
- **CapRover**: `AppsDataStore.ts:410-429` `enableCustomDomainSsl` → `DomainResolveChecker.requestCertificateForDomain` → `saveApp` + `reloadLoadBalancer`. Root SSL gate (`ServiceManager.ts:249-256`). Custom domain validation via `Utils.checkCustomDomain`.
- **Forge**: Dedicated `acme` service (`api/internal/services/acme/{service.go,providers.go,accounts.go}`) + `certificates`/`domains` services + `deployment` `domains` TLS handling. `forgefile` `domainFor`/`urlFor:502-519` synthesizes URLs deterministically. Managed cert/mysql etc more elaborate than either reference.

### C10 — One-Click / App Store

- **CapRover**: `OneClickAppDeployManager.ts:33` template substitution (`$$cap_appname` → app name), dependency-ordered sort (`createAppsArrayInOrder:196-249`), per-service 3-step pipeline (`OneClickAppDeploymentHelper.ts`) → `ServiceManager` register → configure → deploy. Transition state via callback `onDeploymentStateChanged` (`OneClickAppDeployManager.ts:39`).
- **Dokku**: No app store; provisioning is manual or community plugin.
- **Forge**: `appstore` service (`service.go`) + `catalog`/`templates` (`web/app/admin/app-store/page.tsx`, `app-templates/page.tsx`) + `forge/web/app/admin/compose/*`. Forge maps more to CapRover's one-click than to Dokku; but `forgefile` `database:` block is recorded-only (`forgefile/service.go:354-356` warns "database targets are recorded but provisioned by the platform databases service") vs CapRover which actually registers volumes/services for DBs.

### C11 — Process / Scaling — `ps:scale` vs `instanceCount` vs `desiredReplicas`

- **Dokku**: `ps` plugin `Formation` (`ps/ps.go:57-76`) + `ps:scale` + `DOKKU_SCALE` file; per-process-type counts. Restart behavior (`ps:restore`, `ps:restart-policy:18` default `on-failure:10`, `stop-timeout-seconds:30`).
- **CapRover**: `instanceCount: number` (`AppDefinition.ts:77`) single integer for the whole app; Swarm `replicas` in `updateService` (`ServiceManager.ts:965-976`); volume apps forced `instanceCount=1`? (`ServiceManager:704-708` rejects volumes for non-persistent).
- **Forge**: `AppService.Replicas + Mode (replicated/global) + UpdateConfig + HealthCheck + Resources` (`apphosting/service.go:274-288`) plus separate `deployment.TargetReplicas` (`deployment/service.go:80`). `ScaleService:398-453` now validates `ReplicaAppID != nil` before mutating metadata (fix for silent no-op). Health via `ServiceStatusView`/`ServiceOverview` aggregation (`ComputeServiceHealth:323-358`, `GetServiceOverview:529-592`).

### C12 — CaptainDefinition vs forge.yaml vs Dockerfile / app.json / Procfile

- **CapRover**: `ICaptainDefinition.ts:1` `{schemaVersion,dockerfileLines,dockerfilePath,imageName,templateId}` - 5 fields, mutually exclusive exactly-one (`ImageMaker.ts:416-427`). Plus fallback to `Dockerfile`.
- **Dokku**: Detects: `app.json`, `Procfile`, `Dockerfile`, `buildpacks` file, `nixpacks.toml`, `railpack.json`, `lambda.yml` (see `docs/appendices/file-formats/*`, `plugins/common` `CorePostExtract`), with `Procfile` validated via `procfile-util check` (`ps/triggers.go:60-70`).
- **Forge**: `forgefile/service.go:44-97` `Manifest{Project,Deploy[],Database,Environments}`; `knownTopLevel`/`knownDeployKeys` (`service.go:137-149`) soft-validate with warnings not errors (`Validate:153-172`, `checkMinimal:222-256`). Transformations `normalize()` (`service.go:174-186`). `MaxManifest:27` 256 KiB. Encoded as JSON inside YAML column (`persist:374-392` double-encodes `content` as `json.Marshal(content)` Stored as JSON string in Postgres).

### C13 — Registry / Image Distribution

- **CapRover**: `DockerRegistryHelper.ts:25-110` `retagAndPushIfDefaultPushExist` — local builds are `img-<ns>-<app>:<ver>` then retagged `registry/pre/<img>:<ver>` and pushed if a default push registry exists (`DockerRegistryHelper.ts:48-53`). Auth map `createDockerRegistryConfig:172-204` includes docker.io special key `https://index.docker.io/v1/`. Pull-through for `imageName` captain-definition (`ImageMaker.ts:167-176`).
- **Dokku**: Local registry not required; `registry` plugin optionally configures remote registry, patches `docker-options` (`--registry-*`). Images remain local `dokku/<app>:<tag>`.
- **Forge**: Remote builds push via Beacon `ImagePushRequest` (`build/service.go:737-782` `pushToRegistry`), digest capture `digestImageRef:1204-1209`, `validateImageRef:322-330` refusing non-digested refs. Registry auth via `Daemon.RegistryAuth` per-node `LoginRegistry` (`build/service.go:586-589,752-754`). More security-conscious but heavier.

### C14 — Preview / Ephemeral Environments

| System | Primitive | Scope | Lifecycle |
|---|---|---|---|
| Dokku | None built-in; users hack `apps:clone`/git branch or external CI | — | — |
| CapRover | None; branch is just a build input (`RepoInfo.branch`) | — | — |
| Forge | First-class `preview` + `previewenv` services (`preview/service.go`, `previewenv/service.go`) + HTTP `handlers_preview_deployments.go` exposing `/admin/preview-deployments` CRUD/deploy/cleanup/status | Per-`ServerID` + `PRNumber` with `UniqueSuffix`, `IsIsolated` (`preview/service.go:36-51`) | `Status ∈ {deploying,...}` + `Reaper` to GC TTL |

Forge's preview deployments are strictly more ambitious than either reference; CapRover/Dokku punt to CI.

---

## 3. Supplementary Detailed Comparisons

### Git clone safety (CapRover vs Forge)

CapRover `GitHelper.ts:47-78` writes SSH key to `CaptainConstants.captainRootDirectoryTemp` (`/captain/.../uuid`), `chmod 600`, appends `ssh-keyscan -p ${SSH_PORT} -H ${DOMAIN} >> /root/.ssh/known_hosts` — **no host-key verification**, no revocation, appends to global `known_hosts` on every clone, and SSH key lifetime is manual `fs.remove`. Forge never shells out for git on the control plane; clone happens on Beacon via `GitClone` API with encrypted credential resolution on the node, and provider URL is DNS-validated (`service.go:303-318`).

### Build context size + TLS verification (CapRover gap, Forge closes)

CapRover has no build-context size guard (tar is streamed unbounded). Forge `validateBuildContext:851-891` caps at 1 GiB (`service.go:873-876`) and validates platform enum. CapRover's `ssh-keyscan >> known_hosts` is TOFU without pinning; Forge's `validateProviderURL` does DNS resolution check for private/loopback at request time.

### App lifecycle state machines

- Dokku: filesystem-derived (`IsDeployed:677-696` reads `common/deployed` property, else `scheduler-is-deployed` probe; retired via `scheduler-register-retired`/`scheduler-retire` events).
- CapRover: `AppsDataStore.versions[]` + `deployedVersion` index; deployed image at `versions[deployedVersion].deployedImageName`.
- Forge: `deployment` state machine `StatusPending → StatusInProgress (Provisioning/AwaitingHealth/Promoting) → StatusCompleted` with `RollbackPending/RollingBack/RolledBack`, versioned updates (`UpdateDeploymentStatusVersioned`), execution lease (`ClaimExecutionLease` 5 min), progress %, timeout, `ResumeDeployments` on restart (`execution.go:428-479`). Strictly larger.

---

## 4. Logic Findings (≥3)

### LF-01 — Forge Plugin Model Is Metadata-Only (No Runtime) — **CONFIRMED REAL PLUGIN GAP**

**Evidence:**
- `forge/api/internal/http/handlers_plugins.go:48-51` header comment: *"Manifest metadata can be imported and queried. Lifecycle operations remain unavailable until Forge has a plugin runtime that can apply their effects."*
- `forge/api/internal/services/plugins/plugin.go:206-227` `ExecuteHook` merely iterates an in-memory `map[string][]HookHandler` populated only by in-process `RegisterHook` calls (`plugin.go:194-204`) — there is no out-of-process execution, no `plugn` fork, no WASM, no sidecar.
- `store_plugins.go:51` `CreatePlugin` hard-codes `installed=false, enabled=false`; no installer ever flips them to true except manual `UpdatePluginState`.
- `handlers_plugins.go:200-224` `InstallPlugin` delegates to `PluginService.Install` which only validates manifest + creates directory + DB row (`plugin.go:98-161`); it does not fetch code, build, or register hooks.
- Dokku contrast: every plugin is an *executable* (`plugins/apps/triggers.go`, `plugins/proxy/proxy.go:29`, `plugins/ps/ps.go:78`, `plugins/common/plugn.go:42-70` fork+exec `plugn trigger`). CapRover contrast: no plugin system advertised, so no expectation gap.

**Impact:** Users importing from URL/file get a stored manifest that never influences scheduling, proxy, build, or checks. UI marketplace/discover endpoints return rows but cannot enact Dokku-like `dokku plugin:install` semantics. This is a product-scope decision but should be labelled **experimental / metadata catalog only** to avoid false parity.

**Recommendation:** Either (a) mark the feature as *catalog* and rename `marketplace`→`catalog` (avoid Dokku naming), or (b) implement a minimal runtime: `Discover` → verify signature → extract to `pluginsDir` → register `HookHandler` from `Manifest.Hooks` mapping, with 10 s timeout preserved.

### LF-02 — Forge `forgefile` ↔ CapRover `captain-definition` Equivalence Is Partial — Secrets & Build Semantics Diverge

**Evidence:**
- CapRover `ImageMaker.ts:416-427` strictly enforces mutual exclusivity of `templateId/imageName/dockerfilePath/dockerfileLines` and `schemaVersion===2`; Dokku-equivalent validation is trigger-based (`builder-detect`). Forge `forgefile/service.go:137-172` only emits *warnings* for unknown keys (`checkKeys:189-220`, `checkMinimal:222-256`) and never rejects `builder` values unknown to `knownBuilders` as errors, and treats `deploy[].env` as plain `map[string]string` persisted opaquely.
- CapRover encrypts provider secrets at rest (`AppsDataStore.ts:224-232` `encryptor.encrypt(password)` + decrypt on read `AppsDataStore.ts:333-341`). Forge `forgefile/service.go:374-392` `persist` does `json.Marshal(content)` (raw YAML bytes re-encoded as JSON string) into `forge_manifests.content` with **no field-level encryption** for `deploy[].env` values; `GetOwnedManifest:263-282` returns the full manifest including env to the owner/admin, and `GetManifestMetadata:287-305` correctly strips it for public but nothing prevents admin dump. Contrast to CapRover's explicit `passwordEncrypted`/`sshKeyEncrypted` split.
- CapRover `ImageMaker.ts:482-535` fallback: if no captain-definition but `Dockerfile` exists, synthesize `captainDefinitionDefault{ dockerfilePath: "./Dockerfile" }`. Forge `Validate:162-164` requires YAML to be a mapping node else silent warning; there is no Dockerfile auto-synthesis path.

**Impact:** Low immediate outage risk, but inconsistent secret handling vs CapRover and weaker validation than either reference. Forge manifests that omit `project.slug` show warnings but still attempt to `persist` with empty slug (unique conflict).

**Recommendation:** Elevate `checkMinimal` slug/type violations to errors, add `json` tag `env` redaction on read (or store env separately encrypted), and add Dockerfile-adjacent fallback similar to CapRover's.

### LF-03 — Unbounded Build Parallelism vs CapRover Single-Build Gate vs Dokku In-Process Serialization

**Evidence:**
- CapRover `ServiceManager.ts:116-172` `scheduleDeployNewVersion` holds `activeOrScheduledBuilds: IHashMapGeneric<boolean>` and `queuedBuilds: QueuedBuild[]`; `isAnyBuildRunning()` scans the map and refuses concurrency per-capitan-namespace; at most **one** build runs, extra builds for the same app replace queued entry.
- Dokku serialization is implicit: `dokku_receive` is run under a per-app lock (`plugins/ps/ps.go:38` `RetireLockFailed` path) and `scheduler-docker-local` deploys run serialized by the trigger bus (single `plugn` process tree).
- Forge `build/service.go:365-528` `StartBuild` creates a `BuildRecord` + in-memory `active[buildID]=cancel` map (`service.go:443-444`) but has **no global concurrency gate** beyond `NodeCapabilitySelector` node availability; multiple builds for the same `sourceID` are deduplicated only by `GetActiveBuildByIdempotencyKey` if `CommitSHA` is present (`service.go:381-385`), else all builds launch. No per-tenant semaphore, no per-node build slot accounting beyond node capability polling.

**Impact:** Forge can burst dozens of concurrent remote builds, each `GitClone` + `buildx` on the same node, saturating disk/CPU and hitting Beacon concurrent-limit; CapRover/Dokku remain stable under load precisely because they throttle.

**Recommendation:** Add a per-org and per-node inflight limit (e.g., `MaxConcurrentBuildsPerNode=2`, `MaxConcurrentBuildsPerOrg=3`) in `NodeCapabilitySelector.SelectBuildNode` or `StartBuild` gate, matching CapRover's UX while cheaper than Swarm global queue.

### LF-04 — Execution Honesty: Forge's `RuntimeExecutor` Nil-Guard vs Dokku/CapRover Local-Assumption

**Evidence:**
- Forge `execution.go:268-274` `executeProvisionStep` returns error if `s.runtime == nil`; `verifyObservedRunning:338-350` fails any lifecycle step that cannot confirm `VerifyRunning`. Comments explicitly cite Phase-1 findings (`F-01`/`F-05`/`FORGE-LOGIC-001`, `execution.go:249-253,350, healthgate.go:10-13`). History correctly handled via `healthgate.go:6-43` `resolveNodeHost` which fails the gate when target node is offline (contrast naive localhost fallback that made gates pass/fail spuriously).
- Dokku `ps/ps.go:148-182` `Restore` assumes containers exist locally; `IsDeployed:677-696` caches `common/deployed` property after first probe and trusts it afterward until explicit `post-deploy` clears.
- CapRover `ServiceManager.ts:888-993` `ensureServiceInitedAndUpdated` `isServiceRunningByName` check then `createServiceOnNodeId` with placeholder image, then `updateService` — no lease, no digest validation.

**Verdict:** Forge's design here is **strictly stronger** than both references. The nil-guard + digest requirement (`rollout.go:322-330`) + node-host resolution prevents the "deploy reports success with zero containers" class of bugs Dokku/CapRover still permit under failure. Cite as positive deviation.

### LF-05 — Drift Between Building Blocks: `apphosting` Scale vs `deployment` Rollout Replicas

**Evidence:**
- `apphosting/service.go:398-453` `ScaleService` now correctly refuses when `ReplicaAppID==nil` (fix for Phase-1 F-13) and mirrors to `UpdateReplicaAppReplicas`. `apphosting/service.go:681-727` `TriggerDeploy` creates a `store.Deployment` with `Strategy=recreate` and validates `resolveDeployImage` non-empty.
- `deployment/service.go:42-90` + `rollout.go:92-117` `applyRolloutRequest` separately tracks `TargetReplicas` (default 1) and `RolloutStrategy`. Health gate threshold/interval defaults differ (`DefaultHealthGateThreshold=3` vs `ServiceManager.websocketSupport` etc.).
- No shared source of truth links `AppService.Replicas` to `Deployment.TargetReplicas`; a user scaling via `/apps/:id/services/:svcId/scale` vs triggering a rollout with different `targetReplicas` yields divergent desired counts. Dokku/CapRover have single count (`CapRover instanceCount`, `Dokku DOKKU_SCALE`).

**Impact:** Low but observable: dashboard `GetServiceOverview` reads `AppService.Replicas` (`apphosting/service.go:556-557`) while deployment history shows `TargetReplicas`; they can disagree post-rollback.

**Recommendation:** Make `TriggerDeploy`/`StartRollout` source replicas from `ListAppServices` (or require caller to pass service ID), or emit a warning when counts mismatch.

---

## 5. Prescriptive Delta Checklist (Forge → Minimal PaaS Parity)

- [ ] Label plugin marketplace as **catalog-only** until runtime exists, or implement hook runtime (LF-01).
- [ ] Promote `forgefile` minimal-check warnings to errors and add Dockerfile fallback (LF-02).
- [ ] Encrypt `deploy[].env` values at rest separately from manifest body (LF-02).
- [ ] Add per-org/per-node build concurrency gates (LF-03).
- [ ] Unify replicas source of truth between apphosting service and deployment rollout (LF-05).
- [ ] Consider Dokku/CapRover UX wins to port: Dokku's `apps:report/Builder:null` pattern for diagnostics, CapRover's single-build queuing UX (`queuedBuilds` replacement message), Dokku's `letsencrypt:auto-renew` via cron template (`scheduler-docker-local/functions.go:120-184`) as precedent for job scheduling (Forge has `cronjob` service but not yet wired to deployments).

---

## 6. File Reference Index

**Reference — Dokku**
- `reference/app-platforms/dokku/plugins/common/plugn.go:42-70` — `CallPlugnTrigger` bus
- `reference/app-platforms/dokku/plugins/common/common.go:419-469,677-696,995-1005` — scheduler detection, `IsDeployed`, `IsValidAppName`
- `reference/app-platforms/dokku/plugins/apps/apps.go:5-15`, `plugins/apps/triggers.go:10-60` — app lifecycle
- `reference/app-platforms/dokku/plugins/ps/ps.go:5-31,78-256` — formation, restart/restore/start, `RestartProcess`
- `reference/app-platforms/dokku/plugins/proxy/proxy.go:29-91`, `plugins/domains/*`, `plugins/nginx-vhosts/*`, `plugins/caddy-vhosts/*`, `plugins/traefik-vhosts/*` — proxy
- `reference/app-platforms/dokku/plugins/builder/builder.go:5-18`, `plugins/builder-dockerfile/*`, `builder-herokuish/*`, `builder-nixpacks/*` — builders
- `reference/app-platforms/dokku/plugins/scheduler-docker-local/functions.go:120-184`, `storage_exec.go`, `triggers.go:4-6` — cron, scheduler
- `reference/app-platforms/dokku/plugins/config/config.go:33-168` — env management
- `reference/app-platforms/dokku/plugins/checks/report.go:7-50` — health checks
- `reference/app-platforms/dokku/plugins/git/functions:1-80` (shell) — git receive pipeline
- `reference/app-platforms/dokku/plugins/plugin/plugin.go:30-51` — plugin discovery

**Reference — CapRover**
- `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1-7` — captain-definition schema
- `reference/app-platforms/caprover/src/models/AppDefinition.ts:9-116` — `IAppDef`, `RepoInfo`, `AppDeployTokenConfig`
- `reference/app-platforms/caprover/src/user/ImageMaker.ts:55-601` — build pipeline
- `reference/app-platforms/caprover/src/user/ServiceManager.ts:50-1003` — service manager, queued builds, `ensureServiceInitedAndUpdated`, SSL, custom domains
- `reference/app-platforms/caprover/src/user/DockerRegistryHelper.ts:25-363` — registry retag/push/auth
- `reference/app-platforms/caprover/src/datastore/AppsDataStore.ts:29-963` — name/volume/port validation, version history, encryption
- `reference/app-platforms/caprover/src/handlers/users/apps/appdefinition/AppDefinitionHandler.ts:26-373` — register/update/patch
- `reference/app-platforms/caprover/src/user/oneclick/OneClickAppDeployManager.ts:33-309` — one-click orchestration
- `reference/app-platforms/caprover/src/utils/GitHelper.ts:12-156` — git clone helpers
- `reference/app-platforms/caprover/src/docker/DockerApi.ts:1-150` — swarm primitives

**Forge**
- `forge/api/internal/services/apphosting/service.go:51-754` — app/service CRUD, scaling, health, deploy trigger
- `forge/api/internal/services/deployment/service.go:17-488`, `execution.go:22-626`, `rollout.go:42-346`, `healthgate.go:10-80`, `steps.go` — deployment DAG
- `forge/api/internal/services/build/service.go:33-1317` — builders, node selection, remote build, validation, push
- `forge/api/internal/services/git/service.go:231-869` — provider integration, HMAC, webhook
- `forge/api/internal/services/forgefile/service.go:44-523` — manifest parse/validate/persist/materialize
- `forge/api/internal/services/plugins/plugin.go:20-299`, `forge/api/internal/http/handlers_plugins.go:48-330`, `handlers_plugins_extended.go:1-50`, `forge/api/internal/store/store_plugins.go:51-72` — plugin subsystem
- `forge/api/internal/http/handlers_apphosting.go:1-200`, `handlers_preview_deployments.go:1-60` — HTTP surface
- `forge/web/app/admin/apps/page.tsx:1-204`, `web/app/admin/deployments/*`, `web/app/admin/preview-deployments/*` — admin UI

---

## 7. Verdict

Forge is architecturally **superset** to Dokku+CapRover in node count, tenancy, deployment strategy richness, git provider integration, and execution honesty (LF-04). The minimal-PaaS references remain valuable as **simplicity benchmarks**: their single-build gate, filesystem-truth, plugn composability, and `captain-definition` exactly-one rule are UX and operability properties Forge should consciously preserve rather than re-derive. The only structural deficit versus references is the **non-functional plugin runtime**; the remaining deltas are scope decisions and minor consistency gaps.
