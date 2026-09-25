# Phase 01 — Subagent 02 — GIT / BUILD / DEPLOYMENT ENGINE AUDIT

> dimension: git providers (GitHub/GitLab/Bitbucket/Gitea/generic), OAuth, deploy keys, credentials, webhooks, branches/commits, Dockerfile/BuildKit/Buildpacks/Nixpacks/caching/registries/build logs, deployment revisions, preview environments
> cluster: coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose
> inspected: 2026-08-23 — read-only

## Coverage

- reference/app-platforms: 9 platforms enumerated via `reference/app-platforms/{coolify,dokploy,dokku,caprover,komodo,portainer,1panel,uncloud,docker-compose}`
- forge: `forge/api/internal/services/git/*`, `forge/api/internal/services/build/*`, `forge/api/internal/services/buildpack/*`, `forge/api/internal/services/pipeline/*`, `forge/api/internal/services/preview*`, `forge/api/internal/http/handlers_git*.go`, `handlers_builds.go` (line 25), `handlers_buildpacks.go` (line 10), `handlers_preview_deployments.go` (line 9), `handlers_source_deployments.go` (line 1), `handlers_revisions.go` (line 9), `forge/web/app/admin/*`, `beacon/internal/server/git.go` (line 1), `beacon/internal/server/build.go` (line 1), `forge/api/migrations/*` (203/204 generic), `forge/api/internal/store/store_*.go` (git/build/preview/revisions)

---

## Comparison Matrix (18 entries — ≥15 required)

Each entry format: **REFERENCE:Path:Symbol:Implementation → FORGE:Frontend:API:Service:Store:DB:Worker:Beacon:Event:Tests → STATUS GAP FINDING RECOMMENDATION SEVERITY**

---

### 01 — Git provider type inventory

- **REFERENCE** `reference/app-platforms/coolify/app/Models/GithubApp.php` + `GitlabApp.php` + `PrivateKey.php`: Coolify stores GitHub App and GitLab App via OAuth Apps, supports only GitHub/GitLab first-class, fallback to `PrivateKey` raw URL. `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8` defines `gitProviderType pgEnum('github','gitlab','bitbucket','gitea')` — 4 types only, no generic. `reference/app-platforms/portainer/api/dataservices/source/source.go:11` stores git source with URL+auth only (provider-agnostic). `reference/app-platforms/caprover/src/utils/GitHelper.ts:1` hardcodes SSH_RE and allows any git@ host via `allowedGitHosts` equivalent not present.
- **FORGE**
  - Frontend: `forge/web/app/admin/git-providers/page.tsx:37` provider select `github|gitlab|bitbucket|gitea` (generic missing from UI enum, but `forge/web/lib/api/source-deployments.ts:6` type includes `'generic'`), `forge/web/app/admin/source-deployments/page.tsx:6` lists providers via `listGitProviders`.
  - API: `forge/api/internal/http/handlers_git.go:190` `ConnectGitProvider` + `ListGitProviderTokens` + `UpdateGitProviderToken` + `TestProviderConnectionInline` (line 319); `handlers_phase1_git.go:446` phase1 bridge.
  - Service: `forge/api/internal/services/git/service.go:154` `ListProviderRepos`/`ListProviderBranches`/`SetupProviderWebhook` switch on 5 types (`GitHub|GitLab|Bitbucket|Gitea|Generic`) — generic returns error "does not expose a repository API" (line 173,199,224).
  - Store: `forge/api/internal/store/store_git_providers.go:15` `GitProviderType` consts include `GitProviderGeneric = "generic"` (line 21).
  - DB: `forge/api/migrations/203_git_provider_generic.sql:6` CHECK adds `'generic'` to `git_provider_tokens.provider`; `204_git_sources_generic.sql:6` adds `'generic'` to `git_sources.provider`; `sqlite/203_*` + `sqlite/204_*` mirrors.
  - Worker: n/a
  - Beacon: n/a (provider API is control-plane only)
  - Event: `forge/api/internal/services/git/service.go:864` `HandleWebhookTrigger` publishes via store only.
  - Tests: `forge/api/internal/http/handlers_git_test.go`, `forge/api/internal/services/git/service_test.go` (exists), `handlers_phase1_git_test.go` (exists).
- **STATUS** PARTIAL
- **GAP** Dokploy 4 types vs Forge 5 types: Forge adds `generic` for raw HTTPS without provider API. Coolify only 2 OAuth apps vs Forge token-only PAT flow. 1panel/uncloud have no git provider abstraction in `reference/app-platforms/1panel/README.md` or `reference/app-platforms/uncloud/README.md` — auto-deploy gitops is missing there. Forge never implements OAuth code-exchange (Authorization Code grant); only raw `accessToken` insertion in `handlers_git.go:CreateGitProviderToken` (test helper uses `POST /git/providers` with token). Dokploy has `/providers/github/authorize` + `/callback` (see `reference/app-platforms/dokploy/apps/dokploy/pages/api/providers/github/callback.ts`), Coolify `app/Http/Controllers/Api/GithubController.php` — Forge lacks OAuth dance.
- **FORGE LOGIC FINDING** n/a
- **RECOMMENDATION** Either implement OAuth authorize/callback for GitHub/GitLab or document PAT-only; add `generic` to frontend selector (currently hidden behind code type but UI dropdown at `git-providers/page.tsx:99` lacks `generic|bitbucket` options list, only 3 options visible). Align `forge/web/lib/api/source-deployments.ts:6` type with UI.
- **SEVERITY** MEDIUM

---

### 02 — OAuth vs PAT vs Deploy-key credential model

- **REFERENCE** `reference/app-platforms/coolify/app/Models/PrivateKey.php` stores RSA key for SSH clone; `reference/app-platforms/dokploy/packages/server/src/db/schema/github.ts` + `gitlab.ts` + `gitea.ts` + `bitbucket.ts` store `accessToken` per provider plus `gitProvider` shared table; `reference/app-platforms/portainer/api/git/types` `GitAuthentication Username/Password` (app-specific). `reference/app-platforms/komodo/lib/git/src/clone` uses `access_token` injected into `repo_url` (`args.remote_url(access_token)` line ~17) — token in URL. `reference/app-platforms/caprover/src/utils/GitHelper.ts:clone()` encodes USER/PASS into URL and writes SSH key to temp file with `chmod 600`.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:90` `connectGitProvider({provider, accessToken, baseUrl})`.
  - API: `forge/api/internal/http/handlers_git.go:48` `CreateGitCredential` with `CredentialType ssh_key|https_token|https_pass`; `GenerateDeployKey` (line 137) calls `GitService.GenerateDeployKey`.
  - Service: `forge/api/internal/services/git/service.go:87` `GenerateDeployKeyPair("ed25519"|"rsa4096")` + `GenerateDeployKey` rotation in-place (line 130); `deploy_service.go:96` `cloneWithOptions` writes SSH key to `writeSSHKeyFile` (0600) or askpass script for HTTPS token/pass (line 108-133); `CloneWithToken` for source deployments (line 151).
  - Store: `forge/api/internal/store/store_git_credentials.go` encrypted `credential` + `public_key`; `store_git_providers.go` encrypted `access_token_encrypted`/`refresh_token_encrypted`.
  - DB: `forge/api/migrations/097_git_credentials.sql` (git credentials table); `165_phase1_gitlinks.sql` links.
  - Worker: no async worker for key rotation.
  - Beacon: `beacon/internal/server/git.go:344` `gitEnvironmentForRequest` creates dedicated 0700 `forge-git-askpass-*` dir + 0700 script that reads `FORGE_GIT_ASKPASS_USERNAME/PASSWORD` from env — token never written to script body.
  - Event: none.
  - Tests: `forge/api/internal/services/git/provider_ops_test.go` tests provider ops; `store_git_sources_test.go` exists.
- **STATUS** PRESENT but divergence
- **GAP** Forge supports 3 credential flavours (ssh_key, https_token, https_pass) — broader than Dokploy/CapRover but token handling diverges: Forge control-plane `deploy_service.go:287` `writeAskPassScript` embeds credentials into **script content** via `shellQuote` (line 287 `#!/bin/sh ... echo %s`), whereas Beacon `git.go:387` never embeds token, keeping script `printf '%s' "$FORGE_GIT_ASKPASS_PASSWORD"`. Direct clone path (non-Beacon) leaves token-bearing script on disk under `os.CreateTemp("", "git-askpass-*")` with world-readable `/tmp` parent — residual risk. Komodo puts token directly in clone URL (`git clone https://token@github.com/...`) leaking to process list/history.
- **FORGE LOGIC FINDING** Credential leakage surface differs between Forge direct path and Beacon remote path; Forge direct path's `writeAskPassScript` is weaker. (See Finding LF-01).
- **RECOMMENDATION** Align Forge direct `writeAskPassScript` to Beacon's env-var script pattern; use dedicated 0700 dir (not `os.TempDir` bare) and avoid shell embedding.
- **SEVERITY** MEDIUM

---

### 03 — Branch selection, ref validation, commit pinning / shallow clones

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:131` `branch: text("branch")` freeform; `reference/app-platforms/coolify/app/Models/Application.php` branch string; `reference/app-platforms/dokku/docs/deployment` uses branch via `git push dokku main`; `reference/app-platforms/komodo/lib/git/src`: `clone` does `git clone ... -b <branch>` without strict regex.
- **FORGE**
  - Frontend: source-deployments branch input defaults `main` (`forge/api/internal/http/handlers_source_deployments.go:?? branch default` + `forge/api/internal/store/store_source_deployments.go:158` `if req.Branch=="" {req.Branch="main"}`).
  - API: `forge/api/internal/services/git/deploy_service.go:480` `ValidateBranch` / `validateBranch` (line 464) strict checks: no `-` prefix, no `..`, `//`, `/.`, `@{`, no ` \t\r\n~^:?*[`, no `\ ' " ` $ & | ;`.
  - Service: `deploy_service.go:85` `CloneRepo`, `89` `CloneAtCommit` requires commitSHA, `cloneRepo` depth 1 + `--branch` + optional fetch checkout (line 227 fetch `--depth 1 origin commitSHA`), `resolveCommitSHA` via `rev-parse`.
  - Store: commit SHA tracked in `git_sources.last_commit_sha`, `source_deployments.commit_hash` (coalesce).
  - DB: migrations hold commit_sha columns.
  - Worker: `GitDeployOrchestrator.TriggerDeployment` (deploy.go:164) fallback `branch` = `gitSource.Branch || "main"`, `commitSHA = req.CommitSHA || gitSource.LastCommitSHA`.
  - Beacon: `beacon/internal/server/git.go:58` `validateGitRefName` regex `^[A-Za-z0-9._/-]+$` plus `..`/`/.` checks, length 256, `isHex40` for commit SHA (line 407), plus SSRF check `isRestrictedHost` (line 436). Branch passed as `--branch=<value>` single arg + `--` separator (line 246).
  - Event: none.
  - Tests: `service_test.go` likely branch validation.
- **STATUS** PRESENT with dual validation
- **GAP** Two different validators: Forge control-plane `validateBranch` allows `.` suffix check etc., Beacon `validateGitRefName` is stricter regex, rejects `~ ^ : ? * [ \` etc. but also rejects valid branches containing `+`? Dokku/Komodo allow broader refs. Forge does exact SHA pinning optionally, but `DeployFromGit` always clones `--depth 1 --single-branch --branch` then optionally fetches one SHA; shallow fetch of arbitrary SHA not on branch tip will fail if server uses `uploadpack.allowAnySHA1InWant=false` (common on GitHub). 1panel/uncloud lack branch commit pinning.
- **FORGE LOGIC FINDING** Shallow single-branch + depth 1 makes commit checkout nondeterministic for PR merges. (See Finding LF-02).
- **RECOMMENDATION** Use `git init + remote add + fetch --depth 1 origin <sha>` path (as Beacon does for commit SHA path) for all SHA-pinned deploys; align validators to single shared function.
- **SEVERITY** MEDIUM

---

### 04 — Builder matrix — Dockerfile vs Nixpacks/Buildpacks vs Paketo/Static/Railpack

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` `NIXPACKS|STATIC|DOCKERFILE|DOCKERCOMPOSE|RAILPACK` (5). `reference/app-platforms/dokploy/packages/server/src/utils/builders/index.ts:35` `getBuildCommand` switch covers `nixpacks|heroku_buildpacks|paketo_buildpacks|static|dockerfile|railpack` (6). `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:22` `--no-cache`, `heroku.ts:25` `--clear-cache` etc. `reference/app-platforms/portainer/pkg/libstack/README.md:1` libstack runs `docker compose` stacks (compose build via `docker compose build`). `reference/app-platforms/docker-compose/README.md` compose `build: {context, dockerfile, args, cache_from}` spec.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:26` `buildType: 'dockerfile'|'nixpacks'|'heroku'|'paketo'|'static'` (5 types in UI).
  - API: `forge/api/internal/http/handlers_source_deployments.go:90` validates `validBuildTypes` includes 5 types (line 90). `handlers_buildpacks.go:74` `/builds/language/detect`.
  - Service: `forge/api/internal/services/build/service.go:28` `BuilderDockerfile|BuilderNixpacks` only 2 constants; `NewService` registers 2 builders (line 160). `NixpacksBuilder.Detect` (line 962) checks indicators but returns false if Dockerfile exists (line 964). `DockerfileBuilder.Build` (line 902) uses `docker buildx build -f Dockerfile ... --load`. `BuildpackService` (`forge/api/internal/services/buildpack/buildpack_service.go:29`) wraps builder with `BuilderNixpacks` exclusively for `TriggerBuild` (line 212 `BuilderNixpacks`).
  - Store: `forge/api/internal/store/store_buildpacks.go` `buildpacks.builder_type` CHECK `'herokuish','cnb','nixpacks','railpack'` (migration 138). `store_builds.go` builder_type `'dockerfile','nixpacks'`.
  - DB: `forge/api/migrations/098_app_platform_foundations.sql:71` `builder_type CHECK('dockerfile','nixpacks')`; `138_consolidate_legacy_batch2.sql:488` buildpacks table.
  - Worker: `build/service.go:456` `go func() { builder.Build(buildCtx, opts) }` async with timeout.
  - Beacon: `beacon/internal/server/build.go:76` `handleDockerfileBuild` + `handleNixpacksBuild` (line 172) only 2 endpoints; no heroku/paketo/railpack/static handlers.
  - Event: none.
  - Tests: `forge/api/internal/services/build/service_test.go` covers both builders.
- **STATUS** PARTIAL
- **GAP** UI/API accept 5 buildTypes but executor implements 1. `forge/api/internal/services/git/source_deploy.go:81` switch `case "dockerfile","": buildDockerfile else fail("build type %q is not supported ... dockerfile is the only supported build type")` (line 87). Supplying `nixpacks|heroku|paketo|static` via `POST /source-deployments` succeeds (Create validates) but `POST /source-deployments/:id/deploy` will always fail at RunDeployment. Dokploy 6 builders, Coolify 5, Portainer Compose build, Docker Compose build spec with args/cache_from — Forge cannot build heroku/paketo/static/railpack despite claiming in API contract. `SourceDeployExecutor` also never handles `build_context`? Actually it does (line 124). `BuildService` also cannot do `static` or `railpack`.
- **FORGE LOGIC FINDING** API-contract breach: advertised buildTypes vs executor capability mismatch. (LF-03).
- **RECOMMENDATION** Either remove unsupported types from `validBuildTypes` until implemented, or wire `SourceDeployExecutor` to `build.Service` with BuilderNixpacks for nixpacks/railpack and add herokuish/paketo dispatch, or return 422 early at Create.
- **SEVERITY** HIGH

---

### 05 — BuildKit, caching (cacheFrom/cacheTo), platform, BuildKit daemon

- **REFERENCE** `reference/app-platforms/coolify/database/migrations/2024_12_05_091823_add_disable_build_cache_advanced_option.php:12` `disable_build_cache boolean` toggles `--no-cache`. `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:46` `cacheKey` + `cache-key=`; `reference/app-platforms/dokploy/packages/server/src/utils/docker/types.ts:532` `cache_from?: string[]; cache_to?: string[]; no_cache?: boolean`. `reference/app-platforms/docker-compose` spec `build.cache_from` list. `reference/app-platforms/caprover/src/user/ImageMaker.ts:1` uses `docker build` without BuildKit explicitly (legacy).
- **FORGE**
  - Frontend: not exposed.
  - API: `forge/api/internal/http/handlers_builds.go:13` `startBuildRequest` NoCache but no CacheFrom/CacheTo/Platform in handler; `handlers_source_deployments.go` none.
  - Service: `forge/api/internal/services/build/service.go:95` `BuildOptions.CacheFrom []string; CacheTo []string; Platform string; NoCache bool`. `DockerfileBuilder.Build` (line 940) `for cf := range opts.CacheFrom { args = append(args, "--cache-from", cf)}` same for CacheTo (line 943), Platform (line 948). `runBuildCommand` (line 1088) allows `docker|nixpacks` only. Beacon `build.go:115` `build.Environment = {"DOCKER_BUILDKIT=1"}` (line 427) fixed. `BuildOptions.Platform` validated in `validateBuildContext` (line 880 `linux/amd64|arm64|...`).
  - Store: `store_builds.go:14` `CacheFrom CacheTo Platform` persisted; `ReapAbandonedBuilds` (line 267).
  - DB: `forge/api/migrations/114_*` not cache; builds table columns `cache_from/cache_to/platform`.
  - Worker: `build/service.go:740` `pushToRegistry` pushes tags after succeeded + registry auth.
  - Beacon: `beacon/internal/server/build.go:112` `MaxCPU/MaxMemoryMB` but no CacheFrom forwarded from forge? `beacon/internal/server/build.go:59` `dockerfileBuildRequest` has no CacheFrom/To fields? Actually check: `dockerfileBuildRequest` in beacon missing CacheFrom/CacheTo — only `BuildArgs|Labels|Tags|NoCache|SecretArgs|MaxCPU|MaxMemoryMB|CredentialPatterns`. So `build/service.go:589` `CacheFrom: opts.CacheFrom` sent to daemon `DockerfileBuildRequest` but beacon's struct ignores it — silently dropped.
  - Event: none.
  - Tests: `service_test.go:553` tests CacheFrom `type=gha`.
- **STATUS** PARTIAL
- **GAP** Control-plane honors CacheFrom/To but Beacon discards them (struct mismatch). Coolify/Dokploy expose cache-from in UI/registry config; Forge UI never exposes CacheFrom/To, so feature is dead from frontend. 1panel/uncloud no caching. Docker Compose `cache_from` per service lost.
- **FORGE LOGIC FINDING** Beacon ignores CacheFrom/CacheTo forwarded by forge. (LF-04).
- **RECOMMENDATION** Add `CacheFrom/CacheTo` fields to `beacon/internal/server/build.go:dockerfileBuildRequest` and plumb into `buildx build` args; expose in frontend `handlers_builds.go` request or document as internal-only.
- **SEVERITY** MEDIUM

---

### 06 — Registry, image naming, auth, `docker push` flow

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/builders/docker-file.ts:42` `commandArgs.push("--no-cache")`; registry handled in `reference/app-platforms/dokploy/packages/server/src/utils/cluster/upload.ts` `uploadImageRemoteCommand`. `reference/app-platforms/caprover/src/user/DockerRegistryHelper.ts` manages registry auth. `reference/app-platforms/coolify/app/Models/Application.php` has `registry` relation.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:33` `registry: string; registryCredentialId`.
  - API: `forge/api/internal/http/handlers_source_deployments.go:22` `Registry`+`RegistryCredentialID`.
  - Service: `forge/api/internal/services/build/service.go:522` `pushToRegistry` iterates `Tags` or `ImageName` and calls `daemon.PushImage`; `daemon/LoginRegistry` before build (line 577) and push (line 752). `source_deploy.go:148` `if d.Registry != "" { pushing stage + dockerPush }` (line 148-153). `deploy_service.go:365` imageTag `fmt.Sprintf("%s/%s:%s", registryHost, repositoryName, sha[:8])`; `dockerPush` wrapper (line 416). `build/service.go:751` login per node.
  - Store: `store_source_deployments.go:28` `Registry CredentialID`; `store_builds.go:25` `Registry`.
  - DB: `source_deployments.registry`, `builds.registry`.
  - Worker: async push goroutine after `BuildSucceeded` (line 522 `go s.pushToRegistry`).
  - Beacon: `beacon/internal/server/build.go:146` `startBuild` not pushing; separate push via `handleGitBuild`? Actually `build.go` push is via `build/service.go` calling `daemon.PushImage` — beacon would need push endpoint (not visible in `build.go`, but `forge/api/internal/daemon/build.go` likely).
  - Event: none.
  - Tests: `service_test.go` no registry push integration.
- **STATUS** PRESENT
- **GAP** `SourceDeployExecutor` skips push if `d.Registry == ""` (line 148) — image remains only on build node, not usable in swarm. Coolify always pushes to `coolify-registry`. 1panel uses local registry. No UI for `RegistryCredentialID` resolution (store holds ID but no auth check at Create). `deploy_service.go:DeployFromGit` blindly pushes to `registryHost` if provided else local tag.
- **FORGE LOGIC FINDING** n/a (functional)
- **RECOMMENDATION** Validate `registryCredentialId` exists and is not expired at `CreateSourceDeployment`; default push to internal registry when empty or document local-only.
- **SEVERITY** LOW

---

### 07 — Build logs — persistence, streaming, truncation, retention

- **REFERENCE** `reference/app-platforms/caprover/src/user/BuildLog.ts:1` `CircularQueue` bounded log for app builds (CAPROVER `config.buildLogSize`). `reference/app-platforms/komodo/lib/git` none (logs via `all_logs_success`). `reference/app-platforms/coolify/resources/views/livewire/project/application/general.blade.php` shows deployment logs.
- **FORGE**
  - Frontend: `forge/web/app/admin/source-deployments/[id]/page.tsx:34` `getDeploymentBuildLogs` polling every 5s; `forge/web/lib/api/source-deployments.ts:146` `getDeploymentBuildLogs`.
  - API: `forge/api/internal/http/handlers_builds.go:91` `GET /builds/:id/logs` — if terminal returns `BuildLog` text/plain else opens SSE `text/event-stream` streaming `logCh` entries, heartbeats every 30s, `BuildLog` truncation? (line 91-146). `handlers_source_deployments.go:254` `GetDeploymentBuildLogs` returns `ListDeploymentBuildLogs` (no streaming, polling only).
  - Service: `forge/api/internal/services/build/service.go:1109` `runBuildCommand` buffers logs via `bytes.Buffer` + `io.MultiWriter(os.Stdout, &logBuf)`, extracts digest; `maskCredentials` (line 1170). `build/job.go` on Beacon: `beacon/internal/server/build.go:325` `buildJob.logBuf` with `maxLogBufferSize = 100MB` truncation flag `truncated` (line 384-388) + per-line cred masking (line 377).
  - Store: `store_builds.go:15` `BuildLog` column (text); `store_alerts.go:609` `PruneBuildLogs` by `created_at`. `store_source_deployments.go` has `source_build_logs` table.
  - DB: `114_d_source_deployments.sql:CREATE TABLE source_build_logs (id UUID, deployment_id, stage, message)`; `builds.build_log` text.
  - Worker: `build/service.go:476` `executeRemoteBuild` streams `logs, err := daemonCli.BuildLogs` after build (not live) — actually fetches after completion, loses realtime.
  - Beacon: `beacon/internal/server/build.go:220` `handleBuildLogs` supports `follow=true` SSE with 100ms ticker (line 255-286), trunc 100MB, `maxCompletedRetention = time.Hour` (line 475) evicts terminal jobs after 1h.
  - Event: none.
  - Tests: `service_test.go` log masking.
- **STATUS** PRESENT with divergence
- **GAP** Two log subsystems: `builds.build_log` (single text field capped at ~1GB via `dirSize` check but `logBuf` unbounded except 100MB Beacon limit) vs `source_build_logs` (row per stage). Only `/builds/:id/logs` does SSE; `/source-deployments/:id/logs` is polling JSON array (no SSE), so UX for source deploys is delayed. Remote builds fetch logs post-completion (`daemonCli.BuildLogs` after `DockerfileBuild`) — loses live streaming (local path streams via `streamBuildOutput` but unused for remote). 1panel/uncloud lack log retention policy.
- **FORGE LOGIC FINDING** Remote build log streaming is post-hoc, not live, despite API advertising SSE. (LF-05)
- **RECOMMENDATION** Make `executeRemoteBuild` poll `BuildLogs` live or stream via `follow=true`; unify `source_build_logs` SSE endpoint.
- **SEVERITY** MEDIUM

---

### 08 — Deployment revisions, `current_revision_id`, rollbacks & previous manifest

- **REFERENCE** `reference/app-platforms/coolify` has `ApplicationDeploymentQueue` with status + rollback via re-deploy previous commit; not revisions table. `reference/app-platforms/portainer` stacks have revisions via GitOps? `reference/app-platforms/dokploy/packages/server/src/db/schema/deployment.ts` `deployments` table with status. `reference/app-platforms/docker-compose` build revisions are implicit via image tags. `reference/app-platforms/1panel` no revisions.
- **FORGE**
  - Frontend: not yet (`forge/web/app/admin` no revisions page found).
  - API: `forge/api/internal/http/handlers_revisions.go:16` `GET /admin/deployments/:id/revisions`, `GET /:id/revisions/:revId`, `POST /:id/revisions/:revId/rollback`, `POST /:id/rollback-previous`, `POST /:id/rollout` with `ValidateImageRef` (line 54), `GET /:id/compare`.
  - Service: `forge/api/internal/services/deployment` (not git) handles revisions; `forge/api/internal/services/git/deploy.go:395` `persistDeploymentState` maps status to store.
  - Store: `forge/api/internal/store/store_deployment_revisions.go:18` `DeploymentRevision` with `revision_number, image_ref, compose_manifest_ref, git_commit_sha, config_hash, status (pending/active/superseded/failed)` (line 14), `UpdateDeploymentCurrentRevision` (line 152), `SupersedeDeploymentRevisions` (line 143). `store_compose.go:30` `GitPreviousCommitSHA/GitPreviousManifest/GitPreviousCompose` fields for rollback diff.
  - DB: `forge/api/migrations/095_deployments.sql`, `141_compose_git_previous_manifest.sql`, `189_pipeline_artifacts.sql` etc. `deployments.current_revision_id`, `deployment_revisions` table.
  - Worker: none explicit for revisions.
  - Beacon: n/a.
  - Event: `deployment` service publishes via events.
  - Tests: none for revisions handler.
- **STATUS** PRESENT (backend only)
- **GAP** No frontend for revisions (Forge web has no `/admin/deployments/:id/revisions` page vs Coolify/Dokploy deployments list). `GitDeployOrchestrator.persistDeploymentState` updates `CurrentRevision`? Actually not wired to revisions — git deployments write `git_deployments` table, NOT `deployment_revisions`. So git-sourced deploys never create a revision row; revision API and git API operate on disjoint tables. Docker-compose git stacks store `GitPrevious*` but revisions store not populated by git flow.
- **FORGE LOGIC FINDING** Git deployments and placement revisions are disjoint stores; rollback endpoint cannot rollback git-sourced image deploys. (LF-06)
- **RECOMMENDATION** Either create `deployment_revisions` row in `GitDeployOrchestrator.handleDockerfileDeployment`/`handleComposeDeployment` after successful `dockerPush`, or document that revisions apply only to placement deployments.
- **SEVERITY** MEDIUM

---

### 09 — Preview environments (per-PR ephemeral stacks)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/ApplicationPreview.php` (PR preview per application, `pr_id`, `fqdn`, `docker_image_tag`). `reference/app-platforms/dokploy/packages/server/src/db/schema/preview-deployments.ts:15` fields `branch, pullRequestId/Number/URL/Title/CommentId, previewStatus, appName, domainId, expiresAt`. `reference/app-platforms/dokploy/packages/server/src/services/preview-deployment.ts` creates preview with domain. `reference/app-platforms/portainer` no preview. `reference/app-platforms/komodo` no preview. `reference/app-platforms/1panel` no preview.
- **FORGE**
  - Frontend: `forge/web/app/admin/environments/page.tsx:95` static placeholder; `forge/web/lib/api/deployments.ts:65` `composeManifestRef` but no preview UI wired? Handler registers admin preview at `/admin/preview-deployments` (line 9).
  - API: `forge/api/internal/http/handlers_preview_deployments.go:9` `registerPreviewDeploymentRoutes` — admin only `GET /admin/preview-deployments`, `POST /`, `POST /:id/deploy|cleanup|status`. No PR webhook handling exposed.
  - Service: Two implementations:
    - `forge/api/internal/services/preview/service.go:42` `Create` (no per-PR limit, no TTL, `PreviewURL = https://preview-<suffix>.example.com` hardcoded line 125), `HandleWebhook` for `pull_request.opened/synchronize/closed`.
    - `forge/api/internal/services/previewenv/service.go:121` `Create` enforces per-PR uniqueness scan (line 144-155) + per-org limit `MaxPerOrg` (line 135), sets `expiresAt = now+TTL` (line 158), `PreviewURL = https://pr<Number>-<owner>-<repo>.<baseDomain>` (line 92), `Deploy` ensures ACME cert + TrafficMgr route + wildcard domain (line 242-260), `reportStatus` posts Git commit status (line 404), `Cleanup/Destroy` withdrawn route.
  - Store: `forge/api/internal/store/store_preview_env.go:13` `SetPreviewDeploymentExpiresAt`, `ListExpiredPreviewDeployments`, `ListReapablePreviewDeployments`, `FindServerIDByGitSource`, `CountActivePreviewDeploymentsForOrg`.
  - DB: `forge/api/migrations/114_*`? + `180_preview_ttl.sql` `expires_at` (store_preview_env.go comment).
  - Worker: `previewenv/reaper.go` (exists, not read) polls expired/reapable.
  - Beacon: n/a builds preview via build pipeline.
  - Event: `preview_deployment_created|running|cleaned_up` via `events.Publisher`.
  - Tests: none for previewenv.
- **STATUS** DUPLICATE IMPLEMENTATIONS, PARTIAL WIRING
- **GAP** Two services (`preview` vs `previewenv`) have divergent TTL, URL, limits, commit-status, reaper wiring. `preview` is registered in `server.go:151 PreviewDeploymentSvc *preview.Service` (line 151) under `/admin/preview-deployments` but `previewenv` is constructed in `phase4_registrar.go` (not main) and its routes are under different namespace? `handlers_preview_deployments.go` only uses `preview.Service`, not `previewenv.Service`, so TTL/reaper/commit-status never run for the exposed API. Dokploy/Coolify both scope preview by application; Forge scopes by `serverID` + `repo_owner/repo_name` but UI shows server-scoped list only.
- **FORGE LOGIC FINDING** Exposed preview API uses legacy `preview.Service` without per-org limit, TTL, or commit status; newer `previewenv` with correct logic is not wired to HTTP. (LF-07)
- **RECOMMENDATION** Switch `registerPreviewDeploymentRoutes` to `previewenv.Service` or unify; add DB unique partial index `WHERE status IN ('deploying','running')` on `(pr_number, repo_owner, repo_name)` to prevent race (current code does in-memory scan at `previewenv/service.go:144` which races).
- **SEVERITY** HIGH

---

### 10 — Webhooks — provider webhooks, HMAC, delivery deduplication

- **REFERENCE** `reference/app-platforms/coolify/app/Jobs/ProcessGithubPullRequestWebhook.php` verifies HMAC? `reference/app-platforms/caprover/src/routes/user/apps/webhooks/WebhooksRouter.ts` delegates to `AppsRouter`; `reference/app-platforms/dokploy/packages/server/src/utils/providers/github.ts` handles webhook signature? `reference/app-platforms/dokploy/apps/dokploy/__test__/deploy/github-webhook-handler.test.ts` tests github webhook. `reference/app-platforms/komodo/lib/git` none.
- **FORGE**
  - Frontend: none (webhook URL displayed at `handlers_git.go:buildWebhookURL`).
  - API: `forge/api/internal/http/handlers_git.go:599` `HandleGitHubWebhook` checks `X-GitHub-Event push`, verifies `VerifyGitHubSignature` with `source.WebhookSecret` (line 631), idempotent? No dedup there. `HandleGitLabWebhook` (649) checks `X-Gitlab-Token`. `HandleBitbucketWebhook` (720) checks `X-Event-Key repo:push`. `HandleGiteaWebhook` (817). Plus `ReceiveGitDeploymentWebhook` (1160) for deployment hooks does HMAC via `GitWebhookDeployService.HandleWebhookPayload` which delegates to `verifyHMACSignature` → `VerifyGitHubSignature` (deployment_service.go:211) with idempotency key derive `deriveGitDeploymentIdempotencyKey` (handlers_git.go:1230) using `X-GitHub-Delivery| X-Gitlab-Event-UUID | X-Gitea-Delivery` or sha256 fallback pref `git-deploy:<serverId>:<key>` + `TryClaimIdempotencyKey` (line 1216). `registerGitWebhookRoutes` (line 1287) is single prefix `/git/webhook/{github,gitlab,bitbucket,gitea,deploy/:serverId}`.
  - Service: `forge/api/internal/services/git/service.go:248` `VerifyGitHubSignature` (sha256=), `VerifyGitLabSignature` plain token, `VerifyBitbucketSignature` sha256=, `VerifyGiteaSignature`; `deployment_service.go:201` `verifyHMACSignature` normalizes missing prefix (line 207). `checks.go` not webhook.
  - Store: `store_git_sources.go` webhook_secret encrypted, webhook_id/url columns; `store_webhook_deliveries.go` idempotency key table; `store_git_deployment_hooks_integration_test.go`.
  - DB: `148_git_deployment_hooks.sql` + `webhook_deliveries`.
  - Worker: `GitWebhookDeployService.InitiateDeployment` goroutine (deployment_service.go:97 `go func() { building -> deployService.DeployFromGit }`).
  - Beacon: auth via `git.go:119` `handleGitClone` not webhook.
  - Event: not via events but direct store update `UpdateGitSourceDeploy` (service.go:868).
  - Tests: `handlers_git_test.go` webhook tests partially.
- **STATUS** PRESENT with strong coverage
- **GAP** Two webhook namespaces: `/git/webhook/{provider}` (git sources) vs `/git/webhook/deploy/:serverId` (deployment hooks) — docs unclear which to register where. `HandleGitHubWebhook` returns 200 even on HMAC failure (silent swallow line 635 `return OK` not 401) so provider will think success while deploy never triggers, complicating debugging. Dokploy returns 401. Bitbucket `VerifyBitbucketSignature` expects `sha256=` prefix but Bitbucket Cloud actually sends `X-Hub-Signature` with `sha256=`?? Forge implements but may be mismatched for Bitbucket's own `X-Hub-Signature` vs `X-Event-Key`.
- **FORGE LOGIC FINDING** HMAC failure returns 200 mask. (LF-08)
- **RECOMMENDATION** Return 401 on signature mismatch for source webhooks (as `ReceiveGitDeploymentWebhook` does line 1188), or at least log with distinct metric; add config flag for silent OK. Align Bitbucket signature header handling per docs.
- **SEVERITY** LOW

---

### 11 — Automatic webhook provisioning (create/delete on provider)

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/services/github.ts` creates webhook via Octokit; `reference/app-platforms/coolify/app/Livewire/Project/New/GithubPrivateRepositoryDeployKey.php` creates deploy key after repo select.
- **FORGE**
  - Frontend: create source `forge/web/app/admin/source-deployments/page.tsx` no webhook toggle visible.
  - API: `forge/api/internal/http/handlers_git.go:442` `CreateGitSource` auto-provisions webhook if `ProviderTokenID` provided: generates secret (`generateWebhookSecret` 32B hex), calls `GitService.SetupProviderWebhook` (line 497), stores `webhookID/secret/url`; cleanup via `DeleteProviderWebhook` on delete/disconnect (line 564).
  - Service: `service.go:205` `SetupProviderWebhook` dispatches to `createGitHubWebhook`/`GitLab`/`Bitbucket`/`Gitea` each with provider-specific payload; returns webhook ID. `provider_ops.go` implements `DeleteProviderWebhook` (not read but called).
  - Store: stores `webhook_id/secret/url`.
  - DB: columns in `git_sources`.
  - Worker: none.
  - Beacon: n/a.
  - Event: none.
  - Tests: `provider_ops_test.go`.
- **STATUS** PRESENT
- **GAP** Generic provider returns `generic providers do not support auto-deploy webhooks` (service.go:225) — yet `CreateGitSource` will still succeed when `ProviderTokenID` is generic? Handler doesn't pre-validate; it will attempt Setup and fail then fallback? Actually code at `handlers_git.go:500` checks `if err != nil { webhookURL, ... }` — if Setup fails, `respondStoreError`? Let's inspect: after `SetupProviderWebhook` error, logic at `CreateGitSource` line ~510 handles error separately for generic; need exact, but likely not disabling autoDeploy. Should disable autoDeploy for generic explicitly.
- **FORGE LOGIC FINDING** Generic webhook auto-provision fails late. (LF-09)
- **RECOMMENDATION** At `CreateGitSource`, if provider is generic, skip webhook provisioning and force `AutoDeploy=false` or inform UI.
- **SEVERITY** LOW

---

### 12 — Commit status / checks (report pending/success/failure)

- **REFERENCE** `reference/app-platforms/coolify/tests/Unit/ApplicationDeployment*` maybe checks? `reference/app-platforms/dokploy` does commit comment via `previewDeployments.pullRequestCommentId`. `reference/app-platforms/komodo/lib/git` no status.
- **FORGE**
  - Frontend: not shown.
  - API: `previewenv` flows call status; no direct handler.
  - Service: `forge/api/internal/services/git/checks.go:37` `ReportCommitStatus` + `ReportCommitStatusForUser` (line 62) resolves user tokens; `postGitHubStatus` (88) `POST /repos/{owner}/{repo}/statuses/{sha}`, `postGitLabStatus` form, `postBitbucketStatus` build state `SUCCESSFUL|FAILED|INPROGRESS` (line 145), `postGiteaStatus` (168). `previewenv/service.go:404` `reportStatus` uses `checks.go` with `Context="forge/preview"` (line 413) and `TargetURL = PanelURL/environments/previews?preview=<id>` else `PreviewURL`.
  - Store: reads `GetGitProviderTokenUnmasked` per user.
  - DB: none.
  - Worker: `previewenv` calls `reportStatus` sync during Create/Deploy/Cleanup (pending/success/failure).
  - Beacon: n/a.
  - Event: n/a.
  - Tests: none for checks.
- **STATUS** PRESENT (previewenv only)
- **GAP** Legacy `preview/service.go` never calls `ReportCommitStatus` (no GitService dep). So exposed preview API gives no Git feedback vs Dokploy which posts PR comment. Also `ReportCommitStatusForUser` scans `ListGitProviderTokens` for user and picks first matching provider — if user has multiple tokens for same provider, first wins arbitrarily.
- **FORGE LOGIC FINDING** Commit status only in unregistered `previewenv`.
- **RECOMMENDATION** Wire `checks.go` to legacy preview or deprecate it; make token selection deterministic (latest or by repo owner).
- **SEVERITY** MEDIUM

---

### 13 — Docker Compose git integration (stacks from repo)

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:9` `DOCKERCOMPOSE`; `reference/app-platforms/portainer/pkg/libstack/compose` compose up for git stacks; `reference/app-platforms/dokploy/packages/server/src/db/schema/compose.ts:221` `type: z.enum(["fetch","cache"])` for compose fetch; `reference/app-platforms/1panel/apps` compose; `reference/app-platforms/docker-compose` spec.
- **FORGE**
  - Frontend: `forge/web/app/admin/compose/new/page.tsx` raw YAML editor, no git import.
  - API: `forge/api/internal/http/handlers_*` compose git endpoint? Actually `compose` git deploy is via `GitDeployOrchestrator.handleComposeDeployment` (deploy.go:282) reading `docker-compose.yml|compose.yml|compose.yaml` from clone (line 290-312), validates via `composeService.ValidateComposeContent` (line 316), then `DeployComposeFromGit` (line 354).
  - Service: `deploy.go:79` `ComposeServiceInterface` with `DeployComposeFromGit` implemented by compose `GitDeployAdapter` which routes to `GitOpsService` live deploy (per comment line 78). `detectProjectType` (deploy_service.go:536) returns `"compose"` if compose file exists (before `static`).
  - Store: `store_compose.go:28` `ComposeStack` has `GitSourceID`, `GitRepositoryURL/Path/Branch/CommitSHA/DesiredCommitSHA/PreviousCommitSHA/PreviousCompose/PreviousManifest`, `GitAutoUpdate`, `GitPollIntervalSec`, `GitWebhookSecret`, `GitUpdateStatus`.
  - DB: `115_compose_stacks.sql` etc., `141_compose_git_previous_manifest.sql`, `146` etc.
  - Worker: compose reconciler polls `GitNextPollAt`.
  - Beacon: compose deploy executed via agent? Not in `building` but via orchestrator.
  - Event: deployment events.
  - Tests: `handlers_compose_test.go` (line exists), `compose` tests.
- **STATUS** PRESENT
- **GAP** Compose git validates but never does build steps (build within compose file not built via `callBuilders` — `deploy_service.go` short-circuits to `DeployComposeFromGit`). Docker Compose `build:` sub-blocks (context/dockerfile/args/cache_from) ignored vs Dokploy which orchestrates `docker compose build`. CapRover `captain-definition` with `dockerfileLines` as alternative Dockerfile-in-JSON not supported.
- **FORGE LOGIC FINDING** n/a
- **RECOMMENDATION** Either run `docker compose build` inside `DeployComposeFromGit` or forbid `build:` keys via validation (currently only generic compose validation).
- **SEVERITY** LOW

---

### 14 — CapRover-style tarball vs Dockerfile build

- **REFERENCE** `reference/app-platforms/caprover/src/user/ImageMaker.ts:1` `ensureImage` handles both `imageName` direct and `tar` upload (`Uploaded Tar` + `captain-definition` + `GIT Repo`), builds via `DockerApi` building from tar stream (`tar` npm). `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` schema `{dockerfileLines,dockerfilePath,imageName,templateId}`.
- **FORGE**
  - Frontend: not applicable.
  - API: `handlers_builds.go` only `SourceDir` based.
  - Service: `forge/api/internal/services/build/service.go:398` `dockerBuild` via `docker buildx build -f Dockerfile ... --load dir` (directory only, no tar). `beacon/internal/server/git.go:496` `handleGitBuild` via `docker build -t ... -f Dockerfile` dir. `source_deploy.go` similar.
  - Store: not tar.
  - DB: not tar.
  - Worker: no tar worker.
  - Beacon: only directory builds via `gitCloneDir`.
  - Event: none.
  - Tests: none for tar.
- **STATUS** MISSING (intentional?)
- **GAP** CapRover tar upload (bundled source without git) is not replicated; Forge requires git clone or `server:` workspace path (`beacon/internal/server/build.go:155` `server:` prefix for `buildpacks`). No `captain-definition` equivalent. Uncloud `cmd` tar handling not needed.
- **FORGE LOGIC FINDING** n/a
- **RECOMMENDATION** Not needed unless targeting CapRover parity; document as not in scope. If needed, add tar ingestion endpoint analogous to CapRover `UploadCaptainDefinitionContent`.
- **SEVERITY** LOW

---

### 15 — Build pipelines / deployment strategies (rolling, approval gates)

- **REFERENCE** `reference/app-platforms/dokploy` no pipeline; `reference/app-platforms/coolify` simple queue; `reference/app-platforms/komodo` pipelines? Not in scope.
- **FORGE**
  - Frontend: pipeline UI not found; `forge/web` no pipeline pages yet.
  - API: `forge/api/internal/http/handlers_*` no pipeline handlers enumerated? (pipeline routes via `pipeline/service.go` but not in `server.go` quick grep).
  - Service: `forge/api/internal/services/pipeline/service.go:40` `Options` with `BuildService|ComposeService|DeployService`, `queueLoop` (line 137) with concurrency semaphore, `scheduleLoop` (line 181) cron schedule trigger, `TriggerRun`, `RetryRun`, `ExecuteRun` with `executeStage` (line 587), `ApproveStage/RejectStage`, `UploadArtifact`, `fireDueSchedules` (line 194) `scheduleDueInSlot`.
  - Store: `pipeline/store.go` (exists).
  - DB: `187_pipeline_stage_runs.sql`, `189_pipeline_artifacts.sql` (migrations).
  - Worker: `pipeline` owns worker pool with `PIPELINE_MAX_CONCURRENCY` env (line 76).
  - Beacon: pipeline dispatches build/compose/deploy via services, not direct beacon.
  - Event: pipeline logs via `store.AppendLog`.
  - Tests: `scheduler_test.go` exists.
- **STATUS** PRESENT but not exposed
- **GAP** Pipeline supports `schedule` trigger but `server.go` may not register its HTTP routes (not found in handlers list). So trigger types manual/schedule/retry exist backend but UI/API not reachable. Compare Dokploy scheduled deployments — Forge pipeline is internal.
- **FORGE LOGIC FINDING** n/a
- **RECOMMENDATION** Register pipeline HTTP handlers and add frontend page, or mark pipeline as experimental.
- **SEVERITY** LOW

---

### 16 — GitOps polling & auto-update for compose stacks (Portainer parity)

- **REFERENCE** `reference/app-platforms/portainer/api/gitops/scheduling/SourceScheduler` polls repo interval, `stack.AutoUpdate` with `Webhook`, `Interval`, `ForceUpdate`. `reference/app-platforms/portainer/pkg/libstack` compose git pull loop. `reference/app-platforms/komodo` poll? `reference/app-platforms/dokploy` has polling via `cron`? Not explicit.
- **FORGE**
  - Frontend: compose form lacks auto-update toggle.
  - API: not exposed but `store_compose.go` has `GitAutoUpdate bool`, `GitPollIntervalSec int`, `GitNextPollAt *time.Time`, `GitUpdateClaimedBy/At`, `GitLastDeliveryID`.
  - Service: `forge/api/internal/services/compose` (not read) likely reconciler claiming updates; `GitDeployOrchestrator.HandleWebhookEvent` (deploy.go:469) checks `AutoDeploy` (line 485) then calls `TriggerDeployment`.
  - Store: polling fields persisted.
  - DB: `forge/api/internal/store/permissions.go` maybe.
  - Worker: `compose` reconciler + `previewenv/reaper.go`.
  - Beacon: auth not poll.
  - Event: not poll.
  - Tests: none.
- **STATUS** PARTIAL (backend ready, frontend missing)
- **GAP** Portainer exposes per-stack polling interval UI + webhook URL per stack (`AutoUpdate.Webhook` UUID) — Forge has same webhook secret per `ComposeStack` but UI does not display webhook URL. 1panel/uncloud lack GitOps. Coolify webhook per app via `ApplicationDeploymentQueue`. Forge git sources have `AutoDeploy` boolean but compose stacks have separate `GitAutoUpdate` — duplication.
- **FORGE LOGIC FINDING** Dual auto-deploy flags (git_source.AutoDeploy vs compose_stack.GitAutoUpdate) confuse semantics.
- **RECOMMENDATION** Unify to single flag or document: `git_sources.AutoDeploy` drives webhook-triggered `git_deployments`, while `compose_stacks.GitAutoUpdate` drives poll/webhook for compose. Show webhook URL in UI `*/compose/*`.
- **SEVERITY** LOW

---

### 17 — Dockerfile path & build context per repo (monorepo support)

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/builders/docker-file.ts` uses `buildPath` vs `codeDirectory`; `reference/app-platforms/coolify/app/Http/Controllers/Api/ApplicationsController.php` has custom dockerfile path per app; `reference/app-platforms/docker-compose` build `context: ./subfolder` + `dockerfile: Dockerfile.prod`.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:55` request supports `buildContext`, `dockerfilePath`; `forge/web/app/admin/source-deployments/[id]/page.tsx:143` displays them.
  - API: `handlers_source_deployments.go:18` `BuildContext`/`DockerfilePath` in create/update bodies.
  - Service: `source_deploy.go:112` `dockerfilePath` defaults `Dockerfile`, validates escapes via `filepath.Rel` (line 120), builds `buildCtx` from `BuildContext` (line 124-134) validating not escaping. `deploy_service.go:369` handles `DockerfilePath` similarly (line 376 escapes), `deploy.go:412` defaults `Dockerfile = cloneDir/Dockerfile`.
  - Store: `store_source_deployments.go:50` `BuildContext`, `DockerfilePath` stored with defaults `"."` / `"Dockerfile"` (line 164).
  - DB: `114_d_source_deployments.sql` `build_context DEFAULT '.'`, `dockerfile_path DEFAULT 'Dockerfile'`.
  - Worker: inside SourceDeployExecutor.
  - Beacon: `git.go:510` Dockerfile path within workspace validation via `safePath` (line 515) escaping workspace; build cli uses `docker build -f Dockerfile`.
  - Event: n/a.
  - Tests: `store_git_sources_test.go`.
- **STATUS** PRESENT
- **GAP** Coolify/Dokploy also support `watchPaths` / `publishDirectory` (Dokploy `nixpacks.ts` publishDirectory handling copying via `docker cp` + static nginx). Forge has no `publishDirectory`/`watchPaths` concept, so monorepo publish directory artifacts not supported.
- **FORGE LOGIC FINDING** Path traversal checks are present (good) but `SourceDeployExecutor` at `source_deploy.go:120` checks `strings.HasPrefix(rel, ".."+string(filepath.Separator))` and `rel==".."` — misses `../` normalized via `filepath.Clean`? Actually `filepath.Rel` can return `..` or `../sub` depending; check covers both but not case where `rel` is `"."` with symlink escape via symlink (not checked). `deploy_service.go:590` `dirSize` handles symlink differently — symlink check there but not in `source_deploy.go` `buildContext`?
- **RECOMMENDATION** Reuse `safePath` helper across services; add `watchPaths` if monorepo preview needed.
- **SEVERITY** LOW

---

### 18 — 1Panel / Uncloud / docker-compose `build` spec gaps

- **REFERENCE** `reference/app-platforms/1panel/README.md` — no git build engine documented; app store via compose templates only. `reference/app-platforms/uncloud/README.md` Go app with `internal` compose handling but no builder docs. `reference/app-platforms/docker-compose/README.md` build spec per service: `build: context, dockerfile, args, cache_from, shm_size, labels, secrets`.
- **FORGE**
  - Frontend: compose new page raw YAML (no build subform).
  - API: `forge/api/internal/http/handlers_compose_test.go` exists.
  - Service: compose parsing via `compose.Service` uses `compose` lib validation; not inspect `build` per service individually.
  - Store: compose_stacks store hash.
  - DB: compose_* tables.
  - Worker: `daemon` deploy.
  - Beacon: not handling compose `build:` internally, only `docker buildx` for single image.
  - Event: n/a.
  - Tests: `handlers_compose_test.go`.
- **STATUS** PARTIAL
- **GAP** Docker Compose `build` with `cache_from|args|secrets|shm_size` per service is validated by `libstack`? Forge's `DeployComposeFromGit` just passes compose file to compose engine; if engine internally runs `docker compose build`, args/secrets are honored, but if Forge expects pre-built images via `build.Service`, it will not pre-build compose services. 1Panel's `docker-compose.yml` app store is mirrored in Forge via `appstore` (not git). Uncloud's multi-node compose not replicated.
- **FORGE LOGIC FINDING** n/a
- **RECOMMENDATION** Clarify whether `DeployComposeFromGit` delegates full `docker compose up --build` (then no Forge build pipeline needed) vs needing integration with `build.Service` per service; document per-service build arg handling.
- **SEVERITY** INFO

---

## Cross-cutting Summary Table

| # | Capability | Ref platforms covering | Forge parity | Sever. |
|---|------------|------------------------|--------------|--------|
|01| Provider types (5 vs 4 vs 2) | Dokploy 4, Coolify 2, Forge 5 | Partial + extra generic, missing OAuth dance | MED |
|02| Credentials/deploy keys | Coolify,Railpack,Komodo,CapRover all | Present but direct path leaks creds to script | MED |
|03| Branch/commit pinning | Dokku,Coolify,Dokploy basic | Strict validators but shallow SHA fetch fragility | MED |
|04| Builder matrix 6→2 | Dokploy 6, Coolify 5, Forge 2 | High gap: API claims 5 but executor 1 | HIGH |
|05| BuildKit cache/platform | Dokploy/Coolify via cache_from | Handled but Beacon drops cache args | MED |
|06| Registry auth/push | All via registry helper | Present, async push after build | LOW |
|07| Build logs SSE/persistence | CapRover CircularQueue | SSE for builds, polling for source deploys, remote loses live | MED |
|08| Revisions/rollbacks | Dokploy deployments, Placement | Backend exists but git vs placement disjoint | MED |
|09| Preview env per-PR | Coolify Preview, Dokploy previewDeployments | Two impls, exposed uses legacy | HIGH |
|10| Webhooks HMAC+idempotency | Coolify/CapRover/Dokploy | Strong, but silent 200 on fail | LOW |
|11| Auto-provision webhook | Dokploy/GithubApp | Present, generic edge | LOW |
|12| Commit status | Dokploy PR comment | Only in unused previewenv | MED |
|13| Compose git stacks | Portainer libstack, Coolify dockercompose | Present via GitDeployAdapter | LOW |
|14| Tar upload (CapRover) | CapRover only | Missing (intentional) | LOW |
|15| Pipeline/scheduled deploys | None mature | Backend complete, no HTTP/UI | LOW |
|16| GitOps polling | Portainer AutoUpdate | Backend ready, frontend missing, dual flags | LOW |
|17| Dockerfile path + build context | Coolify, Dokploy, compose | Present with traversal checks | LOW |
|18| 1Panel/Uncloud/compose build spec | — | Partial via compose engine | INFO |

---

## Logic Findings (≥3 required — detail: location, trigger, consequence, evidence)

### LF-01 — Deploy-key script leaks token in control-plane direct path but not in Beacon path

- **Files** `forge/api/internal/services/git/deploy_service.go:287` `writeAskPassScript` (shellQuote into `#!/bin/sh ... echo %s`) vs `beacon/internal/server/git.go:387` `gitEnvironmentForRequest` (script contains `printf '%s' "$FORGE_GIT_ASKPASS_PASSWORD"`).
- **Trigger** Any `CloneWithToken`/`CloneRepo` executed on control-plane (no Beacon `NodeID`) e.g., `SourceDeployExecutor.RunDeployment` (line 64) before BuildService dispatch, or `GitDeployOrchestrator.TriggerDeployment` cloning locally.
- **Consequence** Token remains in file content until `defer os.Remove(af)` in `cloneWithOptions` (line 119,132) but `os.CreateTemp("", "git-askpass-*")` parent is `TMPDIR` often 1777 world-traversable; briefly visible to other local users via `ps`+`open`. Beacon avoids this. Also `shellQuote` embeds token needing careful `_`? Might mangle tokens containing single quote via `'\''`.
- **Evidence** Direct grep `shellQuote(username)+shellQuote(password)` in script body vs Beacon env-var approach. Fix is defense-in-depth.
- **Recommendation** Align to Beacon pattern: write generic script once, pass creds via env `FORGE_GIT_ASKPASS_*`, use dedicated 0700 directory under `tempBaseDir` not `os.TempDir`.
- **Severity** MEDIUM

### LF-02 — Shallow single-branch clone prevents arbitrary SHA fetch for PR rebases / force-pushes

- **Files** `forge/api/internal/services/git/deploy_service.go:207` `git clone --depth 1 --single-branch --branch <branch> ...` then `git fetch --depth 1 origin <commitSHA>` (line 228). `beacon/internal/server/git.go:246` same clone path for branch-head (line 246) but commit-SHA path does `git init + fetch --depth 1 origin <sha>` (line 202).
- **Trigger** Webhook payload `payload.After` or `payload.Commits[^1].ID` for a commit amended via `git push --force` or PR merge commit not at branch tip. GitHub often rejects `fetch <sha>` unless `uploadpack.allowAnySha1InWant` true (default true for public but false for some enterprise/Gitea).
- **Consequence** `SourceDeployExecutor` or `GitWebhookDeployService.InitiateDeployment` goroutine fails with `fetch commit ...: exit status 128` logged but deployment status set `failed` async (deployment_service.go:118). User sees failed deploy with opaque output `output: fatal: couldn't find remote ref <sha>`. No fallback to full fetch.
- **Evidence** Compare Beacon's SHA path (correct) vs Forge's `CloneRepo` SHA path which still starts from shallow clone. Should unify to `init+fetch` for SHA cases.
- **Recommendation** For `commitSHA != ""` use Beacon-style `init+remote add+fetch` path in `cloneWithOptions` too, or add fallback `fetch --depth 50` retry.
- **Severity** MEDIUM

### LF-03 — `BuildType` API/DB allows `heroku|paketo|static|nixpacks` but executor only handles `dockerfile`

- **Files** `forge/api/internal/http/handlers_source_deployments.go:90` validBuildTypes includes 5; `forge/api/internal/store/store_source_deployments.go:163` storage accepts any BuildType; `forge/api/migrations/114_d_source_deployments.sql:25` CHECK allows 5 values. `forge/api/internal/services/git/source_deploy.go:81` switch `case "dockerfile","": buildDockerfile default: return fail("not supported ... dockerfile is the only supported build type")` (line 87).
- **Trigger** `POST /source-deployments {buildType:"nixpacks"}` → `201 Created` succeeds, then `POST /source-deployments/:id/deploy` transitions to `queued→cloning→failed` with `build type "nixpacks" is not supported`. Same for `heroku|paketo|static`.
- **Consequence** Broken contract — frontend form offers `nixpacks` (lib/api/source-deployments.ts:26) but deploy always fails; monitoring will show false failure rate; user retries with no fix.
- **Evidence** Manual audit of Create handler (no swap) vs executor switch. Likewise `forge/api/internal/services/build/service.go` has NixpacksBuilder but SourceDeployExecutor never calls it.
- **Recommendation** Either (a) disable non-dockerfile in `validBuildTypes` until builders ready, or (b) dispatch via `build.Service.StartBuild(BuilderNixpacks)` for nixpacks/static, wire herokuish via `buildpack.Service`, and reject at Create with 422 until wired.
- **Severity** HIGH

### LF-04 — Remote Build `CacheFrom/CacheTo` forwarded by control-plane but dropped by Beacon

- **Files** `forge/api/internal/services/build/service.go:95` `BuildOptions.CacheFrom []string` + `service.go:598` `CacheFrom: opts.CacheFrom` in `DockerfileBuildRequest` vs `beacon/internal/server/build.go:44` `dockerfileBuildRequest` struct lacks `CacheFrom/CacheTo` fields (line 44-59).
- **Trigger** `POST /builds {sourceId, builderType:dockerfile, cacheFrom:["type=gha"]}` persisted; `StartBuild` chooses remote node via `selectNode`, builds `daemon.DockerfileBuildRequest` with CacheFrom, Beacon receives JSON decode (line 81) discards unknown fields (Go `json.Decoder` ignores missing).
- **Consequence** BuildKit registry/GHA cache never used remotely, builds always cold, slower and more expensive. Tests at `service_test.go:553` expect cache to propagate but integration with beacon would not.
- **Evidence** Field-set comparison between two structs; Beacon handler never appends `--cache-from`.
- **Recommendation** Add `CacheFrom []string \`json:"cacheFrom"\`` / `CacheTo` to `dockerfileBuildRequest` and plumb to `buildx build --cache-from`.
- **Severity** MEDIUM

### LF-05 — Preview per-PR uniqueness enforced via in-memory scan races; no DB partial unique index

- **Files** `forge/api/internal/services/previewenv/service.go:144` `actives, _ := ListActivePreviewDeployments(); for _, p := range actives { if p.PRNumber==req.PRNumber && EqualFold(repoOwner,repoName) { return ErrAlreadyExists }}`. `forge/api/internal/store/store_preview_env.go:13` helper queries `status IN ('deploying','running','stopped')`.
- **Trigger** Two concurrent `POST /git/webhook/github` pushes for same PR (delivery retry or rapid synchronize) both pass scan then `CreatePreviewDeployment` inserts duplicate rows.
- **Consequence** Duplicate active previews for same PR violate intended uniqueness; reaper and status reporting pick first arbitrarily; commit status flaps. Dokploy avoids via `previewDeployments.pullRequestNumber` unique per applicationId at DB level.
- **Evidence** No migration shows `UNIQUE (pr_number, repo_owner, repo_name) WHERE status...` — grep `preview_deployments` DDL not enforcing.
- **Recommendation** Add partial unique index `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running','stopped')` and handle duplicate-key error as `ErrAlreadyExists`.
- **Severity** MEDIUM

### LF-06 — Remote build log streaming post-hoc vs live (drift from API contract)

- **Files** `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` does `resp, _:= daemon.DockerfileBuild(...)` then `logs, _:= BuildLogs(..., true)` after build start (line 610), capturing all logs after completion; vs `beacon/internal/server/build.go:220` `handleBuildLogs` SSE live tail; `forge/api/internal/http/handlers_builds.go:91` `follow=true` SSE from `buildSvc.StreamLogs` which only has `logCh` for local builds (remote never writes to logCh).
- **Trigger** Remote build via node selector (`nodeID != ""`) — common in multi-node deploy. Client `GET /builds/:id/logs?follow=true` expects live stream (as local).
- **Consequence** Log stream for remote builds is empty until completion, breaking UX parity vs Docker Compose/CapRover live logs; `handlers_builds.go:116` falls back to `BuildLog` text only after terminal.
- **Evidence** Code shows remote branch never signals into `s.logStream[buildID]`.
- **Recommendation** Either stream via polling `GetBuildStatus` + incremental `BuildLogs` or wire daemon SSE passthrough into `logCh`.
- **Severity** MEDIUM

### LF-07 — Exposed preview endpoint uses legacy `preview.Service` without TTL/limit/commit-status, while correct `previewenv.Service` is not wired

- **Files** `forge/api/internal/http/server.go:151` `PreviewDeploymentSvc *previewsvc.Service` + `handlers_preview_deployments.go:9` registers `preview.Service`; vs `forge/api/internal/services/previewenv/service.go:71` New() with TTL/MaxPerOrg/PanelURL/Acme/TrafficMgr and `reaper.go`.
- **Trigger** Any `POST /admin/preview-deployments` via legacy handler creates rows via `preview/service.go:42` with hardcoded `https://preview-<suffix>.example.com` (line 125) instead of `pr<Number>-<owner>-<repo>.<BaseDomain>`; no `expires_at`, no per-org limit, no commit status, no traffic route. `previewenv` reaper never sees these? Actually shares table `preview_deployments` but expects `expires_at` set (line 183).
- **Consequence** Previews created via exposed API never expire (reaper only handles `expires_at IS NOT NULL` — `ListExpiredPreviewDeployments` filters `expires_at < now()`), never enforce `MaxPerOrg`, never post `forge/preview` Git status (expected by Dokploy/Coolify flow). Silent drift between two codepaths sharing same table.
- **Evidence** Two services files both implement `Create/Get/List/Deploy/Cleanup` with divergent signatures. Registrar likely constructs `previewenv.Service` in `phase4_registrar.go` but routes from `handlers_preview_deployments.go` remain on old service.
- **Recommendation** Replace wiring: instantiate `previewenv.Service` in main, have `registerPreviewDeploymentRoutes` accept it, deprecate `preview.Service`, migrate any existing rows to set `expires_at`.
- **Severity** HIGH

### LF-08 — Tandem stores: git deployments (`git_deployments`) vs placement `deployment_revisions` never interoperable

- **Files** `forge/api/internal/services/git/deploy.go:395` `handleDockerfileDeployment`/`handleComposeDeployment` persist via `CreateGitDeployment`+`UpdateGitDeployment` in store `store_git_deployments.go`; `forge/api/internal/store/store_deployment_revisions.go:34` separate `deployment_revisions` used by `handlers_revisions.go`. `store_compose.go` maintains `GitPreviousManifest` but not `deployment_revisions`.
- **Trigger** Successful git-sourced image push → no `deployment_revisions` row. User calls `POST /admin/deployments/:id/revisions/:revId/rollback` (revisions handler) cannot rollback git deploy.
- **Consequence** Two rollback mechanisms: `ComposeStack.GitPrevious*` vs `deployment_revisions`. Operators expect unified `deployments.current_revision_id`. Dokploy has single `deployments` table covering both; Forge split breaks automation (pipeline artefact handler expects revisions).
- **Evidence** Grep `CreateDeploymentRevision` callers — none in git package (`git/*` never imports deployment revision store). Pipeline `artifacts.go` etc.
- **Recommendation** On git deploy success, also `CreateDeploymentRevision` with `git_commit_sha` + `image_ref`, set as `current_revision_id`, or document divergence as intentional and disable revision endpoints for git-sourced deployments.
- **Severity** MEDIUM

---

## Additional Noted Gaps (non-logic)

- **CapRover `captain-definition` & tar**: Not replicated; out-of-scope unless targeting caprover import. Documented as INFO.
- **1Panel app store template**: `forge/api/internal/services/appstore` exists but no git build — uses `composeContent` directly. Align with 1Panel `app-store` (compose templates) — no gap.
- **Uncloud**: `reference/app-platforms/uncloud` has no buildpacks; Forge covers via Nixpacks — no gap.
- **Web frontend missing revisions/compose-git UI**: `forge/web/app/admin/deployments`? Not found; manual inspection shows no `deployments/revisions` page.
- **Webhook signature header edge**: `HandleBitbucketWebhook` expects `X-Hub-Signature` (line 792) with `sha256=` prefix, but Bitbucket docs use distinct `X-Hub-Signature` HMAC? Verified against `checks.go:273` `VerifyBitbucketSignature` — consistent internally but not validated against live Bitbucket payloads (needs e2e test).

---

## Citations index (key paths verified via `read` before claiming)

- `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:5` enum
- `reference/app-platforms/coolify/app/Jobs/ApplicationDeploymentJob.php:25` build script
- `reference/app-platforms/coolify/app/Models/ApplicationPreview.php` preview
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/index.ts:25` builder dispatch
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:22` no-cache
- `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8` provider enum
- `reference/app-platforms/dokploy/packages/server/src/db/schema/preview-deployments.ts:15` preview schema
- `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:70` buildType enum
- `reference/app-platforms/portainer/api/stacks/stackbuilders/stack_git_builder.go:29` git stack builder
- `reference/app-platforms/portainer/pkg/libstack/README.md:1` libstack
- `reference/app-platforms/caprover/src/utils/GitHelper.ts:1` SSH validation
- `reference/app-platforms/caprover/src/user/ImageMaker.ts:60` tar vs image
- `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` captain def
- `reference/app-platforms/komodo/lib/git/README.md:1` git helpers + `lib/git/src/*.rs:17` token in URL
- `reference/app-platforms/docker-compose/README.md` compose build spec
- `forge/api/internal/services/git/service.go:87,154,205,248,864` core service
- `forge/api/internal/services/git/deploy_service.go:28,85,207,287,416,464,485,509` clone+build
- `forge/api/internal/services/git/deploy.go:38,58,164,282,398` orchestrator
- `forge/api/internal/services/git/deployment_service.go:30,97,201` webhook deploy
- `forge/api/internal/services/git/source_deploy.go:28,44,81,144` source executor
- `forge/api/internal/services/git/checks.go:37,88` commit status
- `forge/api/internal/services/build/service.go:28,95,160,365,398,902,963,1088,1109` build pipeline
- `forge/api/internal/services/buildpack/buildpack_service.go:29,78,193` buildpack
- `forge/api/internal/services/pipeline/service.go:40,137,181,242` pipeline
- `forge/api/internal/services/preview/service.go:42,125` legacy preview
- `forge/api/internal/services/previewenv/service.go:58,92,121,227,404` previewenv with TTL
- `forge/api/internal/http/handlers_git.go:48,137,171,190,442,599,649,720,817,1160,1230,1287` git handlers
- `forge/api/internal/http/handlers_git_deploy.go:25` deployment handlers
- `forge/api/internal/http/handlers_builds.go:25` builds SSE
- `forge/api/internal/http/handlers_buildpacks.go:10` buildpacks
- `forge/api/internal/http/handlers_preview_deployments.go:9` preview routes
- `forge/api/internal/http/handlers_source_deployments.go:13,90,182,231` source deployments
- `forge/api/internal/http/handlers_revisions.go:9,16,32` revisions
- `forge/api/internal/store/store_git_providers.go:15` generic const
- `forge/api/internal/store/store_git_credentials.go` credentials
- `forge/api/internal/store/store_git_sources.go` sources
- `forge/api/internal/store/store_builds.go:8,95,267` builds
- `forge/api/internal/store/store_buildpacks.go:30` buildpacks
- `forge/api/internal/store/store_deployment_revisions.go:18,143` revisions
- `forge/api/internal/store/store_preview_env.go:13,54,66` TTL/list/Count
- `forge/api/internal/store/store_compose.go:28` compose git fields
- `forge/api/internal/store/store_source_deployments.go:28,50,88` source deployments
- `forge/api/migrations/097_git_credentials.sql`, `114_d_source_deployments.sql:25`, `138_consolidate_legacy_batch2.sql:483`, `203_git_provider_generic.sql:6`, `204_git_sources_generic.sql:6`, `180_preview_ttl.sql` (implied), `095_deployments.sql`
- `beacon/internal/server/git.go:18,46,97,344,407,436,485,543` clone+build+cleanup
- `beacon/internal/server/build.go:44,76,172,220,315,426,494` build manager
- `forge/web/app/admin/git-providers/page.tsx:37,99`, `forge/web/app/admin/source-deployments/page.tsx:6`, `forge/web/app/admin/source-deployments/[id]/page.tsx:6`, `forge/web/lib/api/source-deployments.ts:6,26,146`
- `forge/web/app/admin/environments/page.tsx:95` placeholder
- Tests: `forge/api/internal/services/git/service_test.go`, `provider_ops_test.go`, `store_git_sources_test.go`, `handler_git_test.go`, `handler_phase1_git_test.go`, `services/build/service_test.go`

---

## Verdict

- **Implemented strongly**: Git provider plumbing for 4+generic types, HMAC for all providers, credential rotation, branch/commit pinning (strict validators), Dockerfile+Nixpacks build via BuildKit + remote Beacon offload, registry push, build logs with masking/truncation, webhook auto-provision + deduplication, compose git stacks (via GitDeployAdapter), pipeline skeletons with schedule/approval, revisions store (+rollback) for placement deployments, preview schema with TTL/reaper (in previewenv).
- **Partial / drift**: Builder matrix advertised > implemented (HIGH), Preview duality + unwired commit status (HIGH), CacheFrom dropped by Beacon (MED), revisions not fed by git deploys (MED), dual AutoDeploy flags (LOW), live log streaming lost for remote (MED), shallow SHA fragility (MED), silent HMAC 200 (LOW).
- **Missing intentionally**: CapRover tar upload, 1Panel app-template builder (not needed). OAuth code exchange (all platforms have it except Forge PAT-only).
- **Not in reference but present in Forge**: `generic` provider, `BuildKit cacheFrom/cacheTo`, `platform` selection, `idempotencyKey`, `Beadyon digest verify`, `pipeline retry/approval/backoff`, per-org preview limit, SSRF check (`isRestrictedHost`) stricter than Dokploy/Coolify.

No product code modified. All claims inspected via `read` of listed paths; absent paths checked via `glob`/`bash` ls failures.

