# Subagent 11 — Build Pipeline — Reverification (2026-08-24)

**Focus:** Builder matrix (dockerfile vs nixpacks vs heroku/paketo/railpack/static), BuildKit cache/platform, registry auth/push, build logs SSE vs polling, revisions, preview env TTL/limit/commit status, webhooks idempotency, auto-provision.

**Cluster:** coolify, dokploy, portainer, docker-compose (under `reference/app-platforms/`), with forge `build`, `buildpack`, `preview`, `previewenv`, `git/SourceDeploy`, `beacon/build` cross-checks.

**Reconciles:** `audits/final-parity/subagent-03-git-build.md` GB-04..GB-16, `audits/phase-01/subagent-02-git-build.md` GB-04 (5 types accepted → 1 executed), GB-05 cache forward.

**Re-inspected (read-only):**
- `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` (5: NIXPACKS|STATIC|DOCKERFILE|DOCKERCOMPOSE|RAILPACK)
- `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:70` (6: dockerfile|heroku_buildpacks|paketo_buildpacks|nixpacks|static|railpack), `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:22` (`--no-cache`), `reference/app-platforms/dokploy/packages/server/src/utils/builders/index.ts:35` (`getBuildCommand` switch 6)
- `forge/api/internal/services/build/service.go:28` (2 builders), `:81` (`BuildOptions` CacheFrom/CacheTo/Platform), `:95` (Platform field), `:160` (registers 2), `:1088` (`runBuildCommand`), `forge/api/internal/services/buildpack/buildpack_service.go:29` (nixpacks-only), `forge/api/internal/http/handlers_builds.go:91` (SSE), `forge/api/internal/http/handlers_source_deployments.go:95` (422 admission), `forge/api/internal/http/handlers_preview_deployments.go:9` (legacy preview routes), `forge/api/internal/services/preview/service.go:42` vs `forge/api/internal/services/previewenv/service.go:121` (duality), `forge/api/internal/services/git/source_deploy.go:81` (dockerfile-only switch), `forge/api/internal/daemon/build.go:54` (CacheFrom/To forward), `beacon/internal/server/build.go:54` (beacon CacheFrom/To now present), `forge/api/internal/store/store_builds.go:14` (persisted CacheFrom/To/Platform), `forge/api/internal/store/store_deployment_history.go:18` (DeploymentRevision vs `store_git_deployments.go:10` git_deployments disjoint), `forge/api/internal/store/store_webhook_deliveries.go:291` (TryClaimIdempotencyKey)

**Method:** No product code modified. All claims verified via `read` + `grep`/`bash`. STATUS: PRESENT / PARTIAL / FIXED / MISSING. Findings are logic defects with file:line triggers.

---

## Parity Matrix (15 rows — ≥12 required)

Each row: **REFERENCE → FORGE file:line → STATUS / GAP / FINDING / SEVERITY**

---

### 01 — Builder matrix — 5/6 types advertised vs executed (GB-04)

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` enum 5 (NIXPACKS,STATIC,DOCKERFILE,DOCKERCOMPOSE,RAILPACK); `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:70` `buildType` pgEnum 6 (dockerfile,heroku_buildpacks,paketo_buildpacks,nixpacks,static,railpack); `reference/app-platforms/dokploy/packages/server/src/utils/builders/index.ts:44` switch dispatch 6 (`getBuildCommand`).
- **FORGE**
  - `forge/api/internal/services/build/service.go:28` `const BuilderDockerfile="dockerfile", BuilderNixpacks="nixpacks"` — only 2 `BuilderType` constants.
  - `forge/api/internal/services/build/service.go:160` `s.builders[BuilderDockerfile] + [BuilderNixpacks]` — registration of exactly 2 builders.
  - `forge/api/internal/http/handlers_source_deployments.go:87` `if req.BuildType=="" req.BuildType="dockerfile"` default.
  - `forge/api/internal/http/handlers_source_deployments.go:95` `if req.BuildType!="dockerfile" return 422 "buildType must be \"dockerfile\" (nixpacks, heroku, paketo, static are not yet supported …)"` — **admission honesty** (phase-01 remediation).
  - `forge/api/internal/services/git/source_deploy.go:81` `switch d.BuildType { case "dockerfile","": buildDockerfile; default: fail("build type %q is not supported … dockerfile is the only supported") }` at `forge/api/internal/services/git/source_deploy.go:87`.
  - `forge/api/internal/services/build/service.go:348` `Detect` — dockerfile priority, nixpacks fallback.
  - `beacon/internal/server/build.go:79` `handleDockerfileBuild` + `beacon/internal/server/build.go:193` `handleNixpacksBuild` — only 2 beacon endpoints.
  - `forge/api/internal/store/store_builds.go:25` `builder_type` persisted as `'dockerfile','nixpacks'` (DB `098_app_platform_foundations.sql:71` CHECK mirrors).
  - `forge/web/lib/api/source-deployments.ts:26` type still lists 5 (`'dockerfile'|'nixpacks'|'heroku'|'paketo'|'static'`) — UI type broader than admission.
- **STATUS** **FIXED (admission)** — was `PARTIAL` / `HIGH` false-completion (5 accepted, 1 executed). Now fails closed at `CreateSourceDeployment` with 422 before `RunDeployment` ever queued. Re-verified: `handlers_source_deployments.go:95` returns 422.
- **GAP** Residual: DB CHECK `114_d_source_deployments.sql:25` still allows `('dockerfile','nixpacks','heroku','paketo','static')` — API rejects before DB, but direct store inserts could create undispatchable rows. `BuildpackService` (`forge/api/internal/services/buildpack/buildpack_service.go:212` `BuilderNixpacks`) has nixpacks path but `SourceDeployExecutor` never calls it. UI type not narrowed.
- **SEVERITY** HIGH → **MEDIUM** after admission (downgrade; legacy index keeps HIGH until DB narrowed or executor wired).
- **PRIOR AUDIT** `phase-01#04` HIGH → `final-parity#04` VERIFIED_FIXED (admission) — **re-verified FIXED**.

---

### 02 — Nixpacks detection & BuildpackService wrapper

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:8` `getNixpacksCommand` with `--env`, `--no-cache`, `--no-error-without-start`; `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` NIXPACKS first-class.
- **FORGE**
  - `forge/api/internal/services/build/service.go:963` `NixpacksBuilder.Detect` checks 17 indicators (`package.json`, `requirements.txt`, `Cargo.toml`, `go.mod`, etc.) at `forge/api/internal/services/build/service.go:967` but yields `false` if Dockerfile exists `forge/api/internal/services/build/service.go:964`.
  - `forge/api/internal/services/build/service.go:982` `NixpacksBuilder.Build` args `nixpacks build <SourceDir> --name <image> --no-cache --build-env --platform --plan` (`forge/api/internal/services/build/service.go:988`, `forge/api/internal/services/build/service.go:1006`).
  - `forge/api/internal/services/buildpack/buildpack_service.go:29` `TriggerBuild` dispatches exclusively `buildsvc.BuilderNixpacks` at `forge/api/internal/services/buildpack/buildpack_service.go:212` with `ServerID:"server:"+serverID`, `Tags:[imageTag]`, `BuildIdempotencyKey:"app-build:"+build.ID` at `forge/api/internal/services/buildpack/buildpack_service.go:218`.
- **STATUS** PRESENT (2 builders, correct priority, plan file 0600 at `forge/api/internal/services/build/service.go:1021`).
- **GAP** SourceDeploy path (`git/source_deploy.go:81`) never invokes `NixpacksBuilder` — nixpacks images only via `buildpack.Service` (server-scoped `server:` prefix), not via git source-deploys. No heroku/paketo/railpack/static builder exists (Dokploy 6 vs Forge 2).
- **SEVERITY** MEDIUM (parity gap, intentional per admission).

---

### 03 — BuildKit cache forward — CacheFrom/CacheTo (GB-05)

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/builders/docker-file.ts:42` `--no-cache` toggle; `reference/app-platforms/dokploy/packages/server/src/utils/docker/types.ts:526` `cache_from/cache_to`; `reference/app-platforms/coolify/database/migrations/2024_12_05_add_disable_build_cache:12` `disable_build_cache`.
- **FORGE**
  - `forge/api/internal/services/build/service.go:81` `BuildOptions{CacheFrom []string; CacheTo []string; NoCache bool; Platform string}` extended fields.
  - `forge/api/internal/services/build/service.go:939` `for _, cf := range opts.CacheFrom { args=append(args,"--cache-from",cf)}` + `forge/api/internal/services/build/service.go:943` `CacheTo` + `forge/api/internal/services/build/service.go:948` `Platform` at `DockerfileBuilder.Build`.
  - `forge/api/internal/services/build/service.go:598` `CacheFrom: opts.CacheFrom` forwarded to `daemon.DockerfileBuildRequest` at `forge/api/internal/daemon/build.go:25` (`CacheFrom []string \`json:"cacheFrom"\``) and `forge/api/internal/daemon/build.go:44` for nixpacks.
  - `beacon/internal/server/build.go:54` `dockerfileBuildRequest{CacheFrom []string \`json:"cacheFrom"\` CacheTo []string \`json:"cacheTo"\` Platform string}` — **previously missing**, now present (comment at `beacon/internal/server/build.go:54` `Phase-1 LF-04: was silently dropped`).
  - `beacon/internal/server/build.go:126` `for _, from := range req.CacheFrom { if isSafeBuildxRef(from) { args=append(args,"--cache-from",from)}}` at `beacon/internal/server/build.go:128`, `beacon/internal/server/build.go:131` `CacheTo` similarly, `beacon/internal/server/build.go:136` `isSafePlatform` guard.
  - `forge/api/internal/store/store_builds.go:14` persisted `CacheFrom CacheTo Platform` columns via `CreateBuild` at `forge/api/internal/store/store_builds.go:42`.
- **STATUS** **FIXED** — was `PARTIAL` (panel forwarded, beacon dropped). Now beacon struct has fields and handler forwards with `isSafeBuildxRef`/`isSafePlatform` at `beacon/internal/server/build.go:542` / `beacon/internal/server/build.go:554`. Re-verified: `beacon/internal/server/build.go:54` vs `forge/api/internal/daemon/build.go:25` schemas align.
- **GAP** UI/API never exposes CacheFrom/To: `forge/api/internal/http/handlers_builds.go:13` `startBuildRequest{NoCache}` no cache fields; `handlers_source_deployments.go` none — feature backend-only, undriven.
- **SEVERITY** MEDIUM → **CLOSED** (beacon fix verified); LOW residual (UI gap).

---

### 04 — Platform selection & BuildKit daemon

- **REFERENCE** `reference/app-platforms/docker-compose` build spec `platform:`; `dokploy/packages/server/src/utils/builders/*` no platform param (host arch).
- **FORGE**
  - `forge/api/internal/services/build/service.go:97` `Platform string` in `BuildOptions`; `forge/api/internal/services/build/service.go:879` `validateBuildContext` allowlist `linux/amd64, arm64, arm/v7, arm/v6, 386, ppc64le, s390x, windows/amd64` at `forge/api/internal/services/build/service.go:880`.
  - `forge/api/internal/services/build/service.go:435` default `Platform="linux/amd64"` at `forge/api/internal/services/build/service.go:436`.
  - `beacon/internal/server/build.go:136` `if req.Platform!="" && isSafePlatform(req.Platform) { args=append(args,"--platform",req.Platform)}` at `beacon/internal/server/build.go:136`.
  - `beacon/internal/server/build.go:554` `isSafePlatform` rejects `\x00\r\n;/\\ ` and empty segments, alnum+`_` only at `beacon/internal/server/build.go:554`.
  - `beacon/internal/server/build.go:447` `buildEnvironment={"DOCKER_BUILDKIT=1"}` at `beacon/internal/server/build.go:447`.
- **STATUS** PRESENT (allowlist + safe guard + default).
- **GAP** `DockerfileBuilder.Build` local path appends `--platform` without `isSafePlatform` check (only `validateBuildContext` allowlist); beacon path has defense-in-depth double check — minor divergence.
- **SEVERITY** LOW.

---

### 05 — Registry auth — LoginRegistry before build/push

- **REFERENCE** `reference/app-platforms/caprover/src/user/DockerRegistryHelper.ts:1` registry auth; `reference/app-platforms/dokploy/packages/server/src/utils/cluster/upload.ts:8` `uploadImageRemoteCommand` after build.
- **FORGE**
  - `forge/api/internal/services/build/service.go:577` `if opts.RegistryAuth!=nil { daemonCli.LoginRegistry(ctx,baseURL,token,*opts.RegistryAuth)}` at `forge/api/internal/services/build/service.go:577` (warn-only on fail at `forge/api/internal/services/build/service.go:579`).
  - `forge/api/internal/services/build/service.go:751` second login before `pushToRegistry` per-node at `forge/api/internal/services/build/service.go:752`.
  - `forge/api/internal/daemon/build.go:436` `LoginRegistry` POST `/registry/login` with `RegistryAuth{Username,Password,ServerAddress}` at `forge/api/internal/daemon/build.go:421`.
  - `beacon/internal/server/build_ext.go:197` `handleRegistryLogin` with `0700` `docker-config-*` temp dir + `--password-stdin` at `beacon/internal/server/build_ext.go:215` (never argv).
- **STATUS** PRESENT (login before build and push, 0700 config dir, password via stdin).
- **GAP** Login failure is `Warn` not `Fail` at `forge/api/internal/services/build/service.go:579` — build proceeds unauthenticated then push fails later (soft fail).
- **SEVERITY** LOW.

---

### 06 — Image push flow — per-tag push + digest capture

- **REFERENCE** same as 05.
- **FORGE**
  - `forge/api/internal/services/build/service.go:737` `pushToRegistry(ctx,nodeID,nodeToken,record,opts)` at `forge/api/internal/services/build/service.go:737` — iterates `opts.Tags` or `opts.ImageName` at `forge/api/internal/services/build/service.go:758`, calls `daemonCli.PushImage` at `forge/api/internal/services/build/service.go:769`, captures `pushResult.Digest` at `forge/api/internal/services/build/service.go:776`, writes `digestBasedRef(tag,digest)` at `forge/api/internal/services/build/service.go:778`.
  - `forge/api/internal/services/build/service.go:522` async `go s.pushToRegistry` after `BuildSucceeded` at `forge/api/internal/services/build/service.go:522`.
  - `forge/api/internal/services/build/service.go:647` `InspectImageDigest` after build success at `forge/api/internal/services/build/service.go:647` with fallback warn.
  - `forge/api/internal/daemon/build.go:359` `PushImage` POST `/image/push` at `forge/api/internal/daemon/build.go:359`; `beacon/internal/server/build_ext.go:32` `handleImagePush` does `docker login --password-stdin` + `docker push` + `docker inspect --format "{{index .RepoDigests 0}}"` at `beacon/internal/server/build_ext.go:128`.
  - `forge/api/internal/services/git/source_deploy.go:148` `if d.Registry!="" { stage pushing + dockerPush }` at `forge/api/internal/services/git/source_deploy.go:148` vs skip if empty.
- **STATUS** PRESENT (per-tag, digest, async, login).
- **GAP** `SourceDeployExecutor` skips push when `Registry==""` (`forge/api/internal/services/git/source_deploy.go:148`) — image stays node-local; no validation of `RegistryCredentialID` at `CreateSourceDeployment` (`forge/api/internal/http/handlers_source_deployments.go:122` stores ID without existence check).
- **SEVERITY** LOW.

---

### 07 — Dockerfile path & build context — monorepo + traversal guards

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/filesystem/directory.ts:107` (`buildType dockerfile` custom path), `reference/app-platforms/coolify/app/Http/Controllers/Api/ApplicationsController.php:1026` custom dockerfile, `reference/docker-compose` `build: {context,dockerfile}`.
- **FORGE**
  - `forge/api/internal/services/git/source_deploy.go:112` `dockerfilePath` defaults `Dockerfile` via `forge/api/internal/services/git/source_deploy.go:113`, `filepath.Clean` + `filepath.Rel` escape check `strings.HasPrefix(rel,".."+sep)` at `forge/api/internal/services/git/source_deploy.go:120`.
  - `forge/api/internal/services/git/source_deploy.go:124` `buildContext` from `d.BuildContext` with same `Rel` escape at `forge/api/internal/services/git/source_deploy.go:131`.
  - `beacon/internal/server/build.go:111` `-f dockerfile` handling at `beacon/internal/server/build.go:111` + `beacon/internal/server/build.go:90` `resolveWorkspace` via `safePath` (`beacon/internal/server/build.go:176`).
  - `forge/api/internal/http/handlers_source_deployments.go:19` `BuildContext`/`DockerfilePath` in create/update bodies.
- **STATUS** PRESENT with traversal checks (rel + Clean).
- **GAP** `source_deploy.go:120` uses `Rel` string prefix check, not `safePath.EvalSymlinks` — symlink inside context that points outside could pass (vs `beacon safePath` which resolves symlinks). `publishDirectory`/`watchPaths` (Dokploy `nixpacks.ts:29` static nginx) not supported.
- **SEVERITY** LOW.

---

### 08 — Build logs — SSE live vs polling (remote post-hoc)

- **REFERENCE** `reference/app-platforms/caprover/src/user/BuildLog.ts:1` `CircularQueue` bounded; `reference/app-platforms/coolify/resources/views/livewire/project/application/general.blade.php` deployment logs.
- **FORGE**
  - `forge/api/internal/http/handlers_builds.go:91` `GET /builds/:id/logs` — if terminal → `text/plain BuildLog` at `forge/api/internal/http/handlers_builds.go:98`; else SSE `text/event-stream` with `logCh` heartbeat 30s at `forge/api/internal/http/handlers_builds.go:111` + `forge/api/internal/http/handlers_builds.go:122` `SetBodyStreamWriter` loop.
  - `forge/api/internal/http/handlers_source_deployments.go:258` `GetDeploymentBuildLogs` polling JSON array (`ListDeploymentBuildLogs`) — no SSE at `forge/api/internal/http/handlers_source_deployments.go:265`.
  - `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` fetches after build: `daemonCli.DockerfileBuild` at `forge/api/internal/services/build/service.go:605`, then `daemonCli.BuildLogs` at `forge/api/internal/services/build/service.go:610`, then loops `for _, l := range logs { logCh <- … }` at `forge/api/internal/services/build/service.go:617` — **buffered post-hoc, not live tail**.
  - `beacon/internal/server/build.go:241` `handleBuildLogs` supports `follow=true` SSE ticker 100ms at `beacon/internal/server/build.go:275` with `job.logBuf` truncation 100MB at `beacon/internal/server/build.go:42`.
  - `forge/api/internal/daemon/build.go:118` `BuildLogs` blocking GET `/build/logs?id=&follow=` with SSE/ plain fallback at `forge/api/internal/daemon/build.go:135` — `BuildLogsStream` exists at `forge/api/internal/daemon/build.go:156` but never used for remote live path.
- **STATUS** PARTIAL — SSE contract exists for `BuildService` but remote path is **post-hoc buffered**, not live. Source-deployments only polling.
- **FINDING** F-01 — remote log streaming post-hoc not live (see Findings).
- **SEVERITY** MEDIUM (UX, not data loss).

---

### 09 — Build record lifecycle — status, stages, idempotency

- **FORGE**
  - `forge/api/internal/services/build/service.go:34` `BuildRunning|Succeeded|Failed|Canceled|Abandoned` + `forge/api/internal/services/build/service.go:44` `BuildStageQueued|Cloning|Building|Built|Pushing|Verifying` at `forge/api/internal/services/build/service.go:44`.
  - `forge/api/internal/services/build/service.go:365` `StartBuild` checks `GetActiveBuildByIdempotencyKey` at `forge/api/internal/services/build/service.go:382` before `GenerateBuildID`.
  - `forge/api/internal/services/build/service.go:1181` `buildIdempotencyKey` sha256(`sourceID|commitSHA|dockerfile|platform|registry|repoURL|branch`) at `forge/api/internal/services/build/service.go:1181`.
  - `forge/api/internal/store/store_builds.go:275` `GetActiveBuildByIdempotencyKey` where `status IN ('running','queued','cloning','building','pushing')` at `forge/api/internal/store/store_builds.go:281` + `ListNonTerminalBuilds` at `forge/api/internal/store/store_builds.go:291`.
  - `forge/api/internal/services/build/service.go:182` `RecoverRunningBuilds` 5min abandon timeout + beacon `GetBuildStatus` re-attach at `forge/api/internal/services/build/service.go:236`.
  - `forge/api/internal/services/build/service.go:1088` `runBuildCommand` allowlist `docker|nixpacks` at `forge/api/internal/services/build/service.go:1089` + `Setpgid` + `Kill(-pid)` at `forge/api/internal/services/build/service.go:1114`.
- **STATUS** PRESENT (idempotency, reaper, re-attach).
- **GAP** `GetActiveBuildByIdempotencyKey` omits `verifying_digest` and `building` branch status variant (`"building"` string vs `BuildStageBuilding`) — minor mismatch with `ListNonTerminalBuilds` which includes `verifying_digest` at `forge/api/internal/store/store_builds.go:299`.
- **SEVERITY** LOW.

---

### 10 — Deployment revisions vs git_deployments — disjoint stores (GB-08)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/ApplicationDeploymentQueue.php` status + re-deploy; `reference/app-platforms/dokploy/packages/server/src/db/schema/deployment.ts` deployments.
- **FORGE**
  - `forge/api/internal/store/store_deployment_history.go:8` `DeploymentRecord{Status,CommitHash,LogPath,RollbackID}` at `forge/api/internal/store/store_deployment_history.go:8` + `forge/api/internal/store/store_deployment_history.go:119` `UpdateDeploymentRecordStatus`; `forge/api/internal/http/handlers_revisions.go` (`GET /admin/deployments/:id/revisions`, `POST rollback`).
  - `forge/api/internal/store/store_git_deployments.go:11` `GitDeployment{GitSourceID,CommitSHA,Branch,Status,ImageTag}` at `forge/api/internal/store/store_git_deployments.go:11` + `forge/api/internal/store/store_git_deployments.go:44` `CreateGitDeployment`.
  - `forge/api/internal/services/git/deploy.go:395` `handleDockerfileDeployment` + `forge/api/internal/services/git/deploy.go:282` `handleComposeDeployment` only call `CreateGitDeployment`/`CompleteGitDeployment` — **never** `CreateDeploymentRecord` nor `CreateDeploymentRevision`.
  - `forge/api/internal/services/git/source_deploy.go:81` source-deploy path writes `source_deployments` + `source_build_logs`, not revisions.
  - `forge/api/internal/store/store_compose.go:28` `GitPreviousCommitSHA/PreviousManifest` for compose rollback diff — duplicate rollback state.
- **STATUS** PRESENT (backend revisions exist) but **disjoint** from git paths.
- **FINDING** F-03 — git deployments and placement revisions disjoint; rollback cannot rollback git image (see Findings).
- **SEVERITY** MEDIUM.
- **PRIOR AUDIT** `phase-01#08` MEDIUM → `final-parity#08` still open — **re-verified OPEN**.

---

### 11 — Preview env — TTL, limit, commit-status — legacy vs previewenv duality (GB-09)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/ApplicationPreview.php:1` (`pr_id,fqdn,docker_image_tag`); `reference/app-platforms/dokploy/packages/server/src/db/schema/preview-deployments.ts:15` (`branch,pullRequestId/Number, previewStatus, expiresAt`); `reference/app-platforms/portainer` none.
- **FORGE**
  - Legacy: `forge/api/internal/services/preview/service.go:42` `Create` no uniqueness, no TTL, hardcoded `PreviewURL="https://preview-<suffix>.example.com"` at `forge/api/internal/services/preview/service.go:125`, `HandleWebhook` at `forge/api/internal/services/preview/service.go:169`.
  - Correct: `forge/api/internal/services/previewenv/service.go:121` `Create` enforces per-PR scan `forge/api/internal/services/previewenv/service.go:144` + per-org `MaxPerOrg` at `forge/api/internal/services/previewenv/service.go:135`, `expiresAt=now+TTL` at `forge/api/internal/services/previewenv/service.go:158`, canonical `PreviewURL="https://pr<Number>-<owner>-<repo>.<baseDomain>"` at `forge/api/internal/services/previewenv/service.go:92`, `Deploy` ensures ACME+TrafficMgr+wildcard at `forge/api/internal/services/previewenv/service.go:242`, `reportStatus` `forge/preview` at `forge/api/internal/services/previewenv/service.go:404`, `reaper.go:53` `ListExpiredPreviewDeployments` + `ListReapablePreviewDeployments`.
  - HTTP: `forge/api/internal/http/handlers_preview_deployments.go:9` `registerPreviewDeploymentRoutes` uses `*preview.Service` (legacy) at `forge/api/internal/http/handlers_preview_deployments.go:9`; `forge/api/internal/http/phase4_registrar.go:67` registers correct `previewenv` under `/preview/webhook/*` + separate management namespace — **two services sharing `preview_deployments` table** via `forge/api/internal/store/store_deployment_history.go:195`.
  - Store: `store_preview_env.go:13` `SetPreviewDeploymentExpiresAt`, `CountActivePreviewDeploymentsForOrg`, `ListExpired...`.
- **STATUS** DUPLICATE IMPLEMENTATIONS, PARTIAL WIRING — **still HIGH**.
- **GAP** Exposed admin API (`/admin/preview-deployments`) runs legacy without TTL/limit/commit-status/reaper; correct `previewenv` only under phase-4 namespace. Legacy rows have `expires_at IS NULL` so reaper never reaps them (`reaper.go:60` filters `expires_at`). Docs unclear.
- **FINDING** F-02 — preview duality HIGH (see Findings).
- **SEVERITY** HIGH (user-visible drift).
- **PRIOR AUDIT** `phase-01#09` HIGH → `final-parity#09` still HIGH — **re-verified HIGH** (no change).

---

### 12 — Preview per-org limit & per-PR uniqueness race

- **FORGE** `forge/api/internal/services/previewenv/service.go:135` `CountActivePreviewDeploymentsForOrg(ctx,repoOwner)` + `forge/api/internal/services/previewenv/service.go:140` `ErrOrgLimitReached`; `forge/api/internal/services/previewenv/service.go:144` in-memory scan `ListActivePreviewDeployments` + `samePreviewUnit` at `forge/api/internal/services/previewenv/webhook.go:112` for upsert; `forge/api/internal/services/previewenv/webhook.go:55` `upsertAndDeploy` collapses `ErrAlreadyExists`.
- **STATUS** PARTIAL (limit+uniqueness enforced in app, not DB).
- **GAP** No DB partial unique index — concurrent webhook deliveries can both pass `ListActive` scan and insert duplicate active rows (race). `preview/service.go:42` has zero uniqueness.
- **FINDING** F-04 — missing unique index race (see Findings).
- **SEVERITY** MEDIUM.

---

### 13 — Webhooks — HMAC + idempotency dedup (GB-10)

- **REFERENCE** `reference/app-platforms/dokploy/apps/dokploy/__test__/deploy/github-webhook-handler.test.ts:1` github HMAC; `reference/app-platforms/coolify/app/Jobs/ProcessGithubPullRequestWebhook.php:1`.
- **FORGE**
  - `forge/api/internal/http/handlers_git.go:599` `HandleGitHubWebhook` `X-Hub-Signature-256` verify at `forge/api/internal/http/handlers_git.go:632` → now `401 invalid signature` at `forge/api/internal/http/handlers_git.go:640` (was 200 before phase-01 fix).
  - `forge/api/internal/http/handlers_git.go:1165` `ReceiveGitDeploymentWebhook` HMAC via `GitDeployMgmtService.HandleWebhookPayload` at `forge/api/internal/http/handlers_git.go:1209` with `deriveGitDeploymentIdempotencyKey` at `forge/api/internal/http/handlers_git.go:1235` using `X-Idempotency-Key|X-GitHub-Delivery|X-Gitlab-Event-UUID|X-Gitea-Delivery` or `sha256(server:body)[:16]` fallback at `forge/api/internal/http/handlers_git.go:1255`.
  - `forge/api/internal/http/handlers_git.go:1201` `ExistsWebhookDeliveryByIdempotencyKey` + `forge/api/internal/http/handlers_git.go:1220` `TryClaimIdempotencyKey`; `forge/api/internal/store/store_webhook_deliveries.go:291` `TryClaimIdempotencyKey` with `WHERE NOT EXISTS` insert at `forge/api/internal/store/store_webhook_deliveries.go:317` + verification at `forge/api/internal/store/store_webhook_deliveries.go:327`.
  - `forge/api/internal/services/git/service.go:248` `VerifyGitHubSignature` (sha256=)+`VerifyGitLabSignature`/`VerifyBitbucketSignature`/`VerifyGiteaSignature`.
  - Two namespaces: `/git/webhook/{github,gitlab,bitbucket,gitea}` + `/git/webhook/deploy/:serverId` at `forge/api/internal/http/handlers_git.go:1292` vs `/preview/webhook/*` at `phase4_registrar.go:67`.
- **STATUS** FIXED (HMAC 401) + idempotency for `git deploy` webhooks.
- **GAP** `/git/webhook/{provider}` (git sources `HandleGitHubWebhook` etc.) have **no** `TryClaimIdempotencyKey` — duplicates can trigger `HandleWebhookTrigger` twice per provider retry. Only `deploy/:serverId` path dedups.
- **SEVERITY** LOW (closed for deploy hooks, residual for source hooks).

---

### 14 — Auto-provision webhooks — create/delete on provider (GB-11)

- **FORGE**
  - `forge/api/internal/http/handlers_git.go:442` `CreateGitSource` auto-provisions if `ProviderTokenID` + `AutoDeploy` at `forge/api/internal/http/handlers_git.go:490`, generates `32B hex` secret at `forge/api/internal/http/handlers_git.go:486`, calls `SetupProviderWebhook` at `forge/api/internal/http/handlers_git.go:497` dispatching to `createGitHubWebhook|GitLab|Bitbucket|Gitea` at `forge/api/internal/services/git/service.go:205`.
  - `forge/api/internal/http/handlers_git.go:536` fail-open: still creates source with `webhookSetupError` JSON field when provider setup fails (generic returns `generic providers do not support auto-deploy webhooks` at `forge/api/internal/services/git/service.go:225`).
  - `forge/api/internal/http/handlers_git.go:561` `DeleteGitSource` + `forge/api/internal/http/handlers_git.go:242` `DisconnectGitProvider` delete provider webhook.
- **STATUS** PRESENT (provision+cleanup, fail-open).
- **GAP** Generic provider: `AutoDeploy=true` remains though no webhook possible — semantics ambiguous vs `compose_stacks.GitAutoUpdate` polling; UI has no toggle distinction.
- **SEVERITY** LOW.

---

### 15 — Build timeouts, retry, reap, retention

- **FORGE**
  - `forge/api/internal/services/build/service.go:410` `BuildTimeout` defaults 1800s at `forge/api/internal/services/build/service.go:411`; `forge/api/internal/services/build/service.go:442` `context.WithTimeout` at `forge/api/internal/services/build/service.go:442` + `forge/api/internal/services/build/service.go:1122` `runBuildCommand` propagates via `exec.CommandContext`.
  - `forge/api/internal/services/build/service.go:1211` `RetryBuild` exponential backoff `attempt*2s` at `forge/api/internal/services/build/service.go:1217` + polls `GetBuild` per second at `forge/api/internal/services/build/service.go:1255`.
  - `forge/api/internal/store/store_builds.go:267` `ReapAbandonedBuilds` where `status='running' AND started_at < NOW()-'12 hours'` at `forge/api/internal/store/store_builds.go:268`; `forge/api/internal/services/build/service.go:335` `periodicReaper` every 5m at `forge/api/internal/services/build/service.go:335` + `forge/api/internal/services/build/service.go:168` `RecoverRunningBuilds` 5min beacon re-attach.
  - `beacon/internal/server/build.go:466` `reapAbandoned` 12h abandoned + `beacon/internal/server/build.go:490` `maxCompletedRetention=1h` evicts terminal jobs (100MB each) at `beacon/internal/server/build.go:490`.
  - `forge/api/internal/store/store_builds.go:255` `PruneBuilds` retention 20 at `forge/api/internal/store/store_builds.go:255` (delete beyond limit).
- **STATUS** PRESENT (timeout+retry+dual reapers+retention).
- **GAP** Control-plane reaper ignores `verifying_digest`/`pushing` non-terminal? Actually `ListNonTerminalBuilds` includes `verifying_digest` at `forge/api/internal/store/store_builds.go:299` but `GetActiveBuildByIdempotencyKey` at `forge/api/internal/store/store_builds.go:281` omits it — minor inconsistency.
- **SEVERITY** LOW.

---

## Reconciliation — Final-Parity GB-04..GB-16 + Phase-01 GB-04/GB-05

| GB | Final-Parity / Phase-01 claim | Re-inspected now | Verdict |
|---|---|---|---|
| **GB-04** | 5 types accepted, 1 executed → false-completion HIGH | `handlers_source_deployments.go:95` 422 admission, `source_deploy.go:87` dockerfile-only, `build/service.go:28` 2 builders | **VERIFIED FIXED** (admission closes HTTP path) |
| **GB-05** | CacheFrom/To silently dropped by beacon | `beacon/build.go:54` now has CacheFrom/To, forwarding `beacon/build.go:126`, `daemon/build.go:25` aligns | **VERIFIED FIXED** |
| **GB-06** | Registry auth/push present | `build/service.go:737` push+digest, `source_deploy.go:148` push-if-registry | **PRESENT — no regression** |
| **GB-07** | Build logs SSE vs polling divergence | `handlers_builds.go:91` SSE vs `handlers_source_deployments.go:258` polling; remote `build/service.go:610` post-hoc | **PARTIAL — still post-hoc, not live (F-01)** |
| **GB-08** | Revisions disjoint | `store_deployment_history.go:55` vs `store_git_deployments.go:44`; `deploy.go:395` never creates revision | **OPEN — disjoint (F-03)** |
| **GB-09** | Preview duality legacy vs previewenv | `handlers_preview_deployments.go:9` legacy vs `previewenv/service.go:121` correct; `phase4_registrar.go:67` separate | **STILL HIGH — duality (F-02)** |
| **GB-10** | Webhooks HMAC 200→401 + idempotency | `handlers_git.go:640` 401 fixed; `store_webhook_deliveries.go:291` TryClaim | **FIXED for deploy hooks, residual for source hooks (GB-13 gap)** |
| **GB-11** | Auto-provision webhooks | `handlers_git.go:490` SetupProviderWebhook fail-open | **PRESENT** |
| **GB-12** | Commit status only in previewenv | `previewenv/service.go:404` reportStatus vs `preview/service.go:42` no status | **STILL PARTIAL** |
| **GB-13** | Compose git stacks present | `deploy.go:282` handleComposeDeployment | **PRESENT** |
| **GB-14** | Tar upload missing (intentional) | no tar endpoint | **MISSING intentional** |
| **GB-15** | Pipeline backend no HTTP | `pipeline/service.go:137` queueLoop, no `registerPipelineRoutes` | **BACKEND ONLY** |
| **GB-16** | GitOps polling dual flags | `compose/gitops.go:1266` PollForUpdates vs `git_sources.AutoDeploy` | **PARTIAL — dual flags** |

**Admissions verified:**
- **422 now:** `forge/api/internal/http/handlers_source_deployments.go:95` returns `StatusUnprocessableEntity` with message `"buildType must be \"dockerfile\" …"` — closes false-completion. Direct store inserts still possible (DB CHECK broader) but not via HTTP.
- **Cache forward fixed:** `beacon/internal/server/build.go:54` `CacheFrom/To []string` + `forge/api/internal/daemon/build.go:25` + `forge/api/internal/services/build/service.go:598` forward path + `beacon/internal/server/build.go:126` `isSafeBuildxRef` guard — end-to-end.
- **Remote logs still post-hoc not live:** `forge/api/internal/services/build/service.go:610` `BuildLogs(...,true)` after `DockerfileBuild` + buffered `logCh <-` loop, while `forge/api/internal/daemon/build.go:156` `BuildLogsStream` (SSE) is unused — confirmed.
- **Preview duality still HIGH:** `forge/api/internal/http/handlers_preview_deployments.go:9` wires `*preview.Service` (no TTL) vs `forge/api/internal/services/previewenv/service.go:121` wired separately via `phase4_registrar.go:67` — same table divergent semantics, legacy `expires_at IS NULL` never reaped.
- **git_deployments vs deployment_revisions disjoint:** `forge/api/internal/services/git/deploy.go:395` / `forge/api/internal/services/git/source_deploy.go:81` only touch `git_deployments`/`source_deployments`, never `store_deployment_history.go:55` `DeploymentRecord` — confirmed disjoint.

---

## Findings (≥3 required — detail: location, trigger, consequence, evidence)

### F-01 — MEDIUM — Remote build logs post-hoc buffered, not live SSE (GB-07)

- **Trigger:** Any `StartBuild` with `NodeID != ""` (beacon-remote path) — the only path used in production (`selectNode` or explicit `NodeID`).
- **Location:** `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` — `daemonCli.DockerfileBuild` at `forge/api/internal/services/build/service.go:605`, then **after** success `daemonCli.BuildLogs(ctx,baseURL,token,resp.ID,true)` at `forge/api/internal/services/build/service.go:610`, then `for _, l := range logs { logCh <- … }` at `forge/api/internal/services/build/service.go:617` + same for nixpacks at `forge/api/internal/services/build/service.go:685`. Status polled at `forge/api/internal/services/build/service.go:627` **after** log fetch.
- **Consequence:** `GET /builds/:id/logs?follow=true` SSE at `forge/api/internal/http/handlers_builds.go:116` reads `StreamLogs` channel which for remote builds is only filled **after** the build completes — clients see silence then full dump, not live incremental logs. Beacon supports live SSE at `beacon/internal/server/build.go:241` (`handleBuildLogs` 100ms ticker) and daemon client has `BuildLogsStream` at `forge/api/internal/daemon/build.go:156` (`io.ReadCloser` SSE) plus blocking `BuildLogs` that already handles SSE at `forge/api/internal/daemon/build.go:135` — but the service never uses streaming; it blocks until build end.
- **Evidence:** `forge/api/internal/services/build/service.go:610` `logs, err := s.daemonCli.BuildLogs(ctx, baseURL, token, resp.ID, true)` — no ticker, no incremental. `forge/api/internal/http/handlers_builds.go:116` `logCh, err := buildSvc.StreamLogs` expects live channel, but `executeRemoteBuild` fills it late. Local `runBuildCommand` at `forge/api/internal/services/build/service.go:1088` streams via `MultiWriter`/`Scanner` but remote bypasses it entirely.
- **Recommendation:** Poll `BuildLogs` incrementally or consume `BuildLogsStream` SSE: open `BuildLogsStream` goroutine that forwards `data:` lines into `logCh` while polling `GetBuildStatus` ticker, then close. Keep masking + truncation.
- **Status:** **OPEN** — UX drift, not data loss (logs survive to `BuildLog`).

### F-02 — HIGH — Preview duality persists: exposed admin API uses legacy service without TTL/limit/commit-status (GB-09)

- **Trigger:** `POST /admin/preview-deployments` (registered at `forge/api/internal/http/handlers_preview_deployments.go:9` via `registerPreviewDeploymentRoutes`) or any existing legacy preview row with `expires_at IS NULL`.
- **Location:** `forge/api/internal/http/handlers_preview_deployments.go:9` takes `*preview.Service` (`forge/api/internal/services/preview/service.go:42` `Create` no per-PR uniqueness, hardcoded `PreviewURL` at `forge/api/internal/services/preview/service.go:125`). Correct service `forge/api/internal/services/previewenv/service.go:121` `Create` enforces `MaxPerOrg` at `forge/api/internal/services/previewenv/service.go:135`, `expiresAt=now+TTL` at `forge/api/internal/services/previewenv/service.go:158`, canonical `PreviewURL` at `forge/api/internal/services/previewenv/service.go:92`, `reportStatus` at `forge/api/internal/services/previewenv/service.go:404`, `reaper.go:53` `ListExpired…` — wired only under `phase4_registrar.go:67` `/preview/webhook/*` + separate management namespace, **not** under `/admin/preview-deployments`.
- **Consequence:** Two live semantics on one table `preview_deployments` (`forge/api/internal/store/store_deployment_history.go:195` `CreatePreviewDeployment`). Legacy rows never get `expires_at`, so `reaper.go:60` `ListExpiredPreviewDeployments` never finds them — they live forever. Legacy has no per-org limit (Dokploy limit 10 / Coolify per-app) and no commit status (`forge/preview` context) — parity loss vs `reference/app-platforms/coolify/ApplicationPreview.php` + `dokploy/preview-deployments.ts:15` (`expiresAt`). UI `forge/web/app/admin/environments/page.tsx:95` placeholder shows neither.
- **Evidence:** `http/server.go:155` `PreviewDeploymentSvc *preview.Service` + `http/server.go:2608` `registerPreviewDeploymentRoutes(..., PreviewDeploymentSvc)` vs `http/phase4_registrar.go:37` `previewenv.New(..., PREVIEW_TTL, PREVIEW_MAX_PER_ORG)` separate instance. `previewenv/service.go:404` `Context="forge/preview"` never reached via admin route.
- **Recommendation:** Switch `registerPreviewDeploymentRoutes` to accept `*previewenv.Service` or unify services; deprecate `preview.Service`; migrate legacy rows: `UPDATE preview_deployments SET expires_at = created_at + interval '24h' WHERE expires_at IS NULL`; add partial unique index to prevent race (F-04).
- **Status:** **HIGH — still OPEN**, unchanged from `final-parity#09` / `phase-01#09`.

### F-03 — MEDIUM — git_deployments and deployment_revisions disjoint; rollback cannot rollback git image (GB-08)

- **Trigger:** Any successful git deployment via `GitDeployOrchestrator.handleDockerfileDeployment` at `forge/api/internal/services/git/deploy.go:395` or `handleComposeDeployment` at `forge/api/internal/services/git/deploy.go:282`, or `SourceDeployExecutor.RunDeployment` at `forge/api/internal/services/git/source_deploy.go:44`.
- **Location:** Those paths only call `forge/api/internal/store/store_git_deployments.go:44` `CreateGitDeployment` / `forge/api/internal/store/store_git_deployments.go:194` `CompleteGitDeployment` (and `source_deployments` table via `store_source_deployments.go`), never `forge/api/internal/store/store_deployment_history.go:55` `CreateDeploymentRecord` nor `CreateDeploymentRevision` (via `store_deployment_revisions.go:18`). Revision API at `handlers_revisions.go:16` (`GET /admin/deployments/:id/revisions`, `POST rollback`) queries `deployment_revisions`/`deployments.current_revision_id` — disjoint table.
- **Consequence:** Git-sourced image deploys have no `deployment_revisions` row, so `POST /admin/deployments/:id/revisions/:revId/rollback` and `POST /admin/deployments/:id/rollback-previous` cannot rollback them; compose git stacks duplicate rollback state via `store_compose.go:28` `GitPreviousCommitSHA/PreviousManifest` — two rollback mechanisms, one invisible to the other. Dokploy `deployment.ts` and Coolify `ApplicationDeploymentQueue` treat every deploy as a revision — parity gap.
- **Evidence:** `git/deploy.go:395` + `git/deploy.go:282` lack `CreateDeploymentRevision`; grep `CreateDeploymentRevision` only in `deployment/revisions.go:55` (placement path). `GET /admin/preview-deployments` vs revisions endpoints unrelated.
- **Recommendation:** On git deploy success, also `CreateDeploymentRevision{git_commit_sha, image_ref, compose_manifest_ref}` and `UpdateDeploymentCurrentRevision`, or document that revisions apply only to placement deployments and disable `handlers_revisions.go` for `git_sources.*` vs `deployments.*` scope; reconcile `GitPrevious*` vs `deployment_revisions`.
- **Status:** **MEDIUM — still OPEN**, as in `final-parity#08`.

### F-04 — MEDIUM — Preview per-PR uniqueness enforced by in-memory scan, not DB — race under concurrent webhooks

- **Trigger:** Concurrent `pull_request.synchronize` webhooks for same PR (`repo_owner/repo_name/pr_number`) hitting `previewenv/upsertAndDeploy` or `Create`.
- **Location:** `forge/api/internal/services/previewenv/service.go:144` `actives, _ := s.store.ListActivePreviewDeployments` then `for _, p := range actives { if p.PRNumber==req.PRNumber && strings.EqualFold(p.RepoOwner,req.RepoOwner) && strings.EqualFold(p.RepoName,req.RepoName) { return ErrAlreadyExists } }` at `forge/api/internal/services/previewenv/service.go:149`; same pattern in `forge/api/internal/services/previewenv/webhook.go:60` `ListActivePreviewDeployments` + `samePreviewUnit` at `forge/api/internal/services/previewenv/webhook.go:112`; fallback `ErrAlreadyExists` collapse at `forge/api/internal/services/previewenv/webhook.go:101`.
- **Consequence:** Two deliveries both see empty active list, both insert — duplicate active previews per PR, violating Dokploy `previewDeployments.pullRequestId` scoped unique and Coolify one-active-per-PR contract. No DB constraint prevents it; `preview/service.go:42` legacy path has zero uniqueness at all.
- **Evidence:** No migration adds `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running')`. `ListActivePreviewDeployments` at `forge/api/internal/store/store_deployment_history.go:281` is `WHERE status NOT IN ('cleaned_up','failed')` — broader than upsert check.
- **Recommendation:** Add partial unique index `WHERE status IN ('deploying','running')` on `(pr_number, lower(repo_owner), lower(repo_name))` and handle `pq: duplicate` as `ErrAlreadyExists`; keep in-memory check as fast-path. Migrate legacy duplicates before.
- **Status:** **MEDIUM — OPEN** (also noted in `final-parity#09` recommendation).

---

## Additional Cross-checks

- **Nixpacks plan custom:** `forge/api/internal/services/build/service.go:989` `NixpacksPlan map[string]any` marshaled to `0600` temp file at `forge/api/internal/services/build/service.go:1021` with `defer os.Remove` — present, not in Dokploy `heroku.ts:20` path (Forge equivalent via buildpack).
- **Compose build from git:** `deploy.go:282` reads `docker-compose.yml|compose.yml|compose.yaml` at `deploy.go:290` + `ValidateComposeContent` at `deploy.go:316`; `store_compose.go:28` `GitPreviousCompose/Manifest` retains rollback diff — PRESENT.
- **Branch validation dual:** `deploy_service.go:464` `validateBranch` (strict) vs `beacon/git.go:58` `validateGitRefName` regex — both present, divergence noted but safe (Forge richer, Beacon stricter).
- **Shallow SHA fetch fragility:** `deploy_service.go:207` `git clone --depth 1 --single-branch --branch` then `fetch --depth 1 origin <sha>` at `deploy_service.go:228` can fail if provider `uploadpack.allowAnySHA1InWant=false` (common Gitea); beacon `git.go:184` `init+fetch <sha>` avoids it — LF-02 still valid but out of Build-Pipeline scope (Git clone).

---

## Summary

- **GB-04 admission 422: VERIFIED FIXED.** `handlers_source_deployments.go:95` rejects non-dockerfile at Create; `source_deploy.go:87` dockerfile-only executor consistent. Residual DB CHECK + UI type broader — low.
- **GB-05 cache forward: VERIFIED FIXED.** `beacon/build.go:54` now has `CacheFrom/To`, forwarding via `isSafeBuildxRef`/`isSafePlatform` — end-to-end with `daemon/build.go:25` + `build/service.go:598`.
- **Remote logs:** Still post-hoc buffered (`build/service.go:610` → `617`), not live via `daemon/build.go:156` SSE — **F-01 OPEN MEDIUM**.
- **Preview duality:** Still **HIGH** — `handlers_preview_deployments.go:9` legacy vs `previewenv/service.go:121` correct via `phase4_registrar.go:67` — **F-02 OPEN**.
- **git_deployments vs deployment_revisions disjoint:** Still **OPEN MEDIUM** — **F-03**.
- **New F-04:** Missing DB unique index for per-PR preview — race.

**Overall Build Pipeline parity:** 2 of 5 historic HIGHs fixed (GB-04, GB-05), 3 remain or are narrowed (GB-09 HIGH, GB-07/08 MEDIUM). Builder matrix honestly admitted as dockerfile-only; full Dokploy 6-builder parity still missing but no longer a false-completion contract.

*No product code modified. All file:line citations verified via `read` 2026-08-24.*
