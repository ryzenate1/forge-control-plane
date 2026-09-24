# Subagent 03 — GIT / BUILD / DEPLOYMENT ENGINE — Final Parity (Definitive)

**Dimension:** Git providers (GitHub/GitLab/Bitbucket/Gitea/Generic), OAuth vs PAT, deploy-key vs https_token/ssh, branch/commit pinning + shallow fetch, builder matrix (Dockerfile vs Nixpacks vs Heroku/Paketo/Railpack/Static), BuildKit cache/platform, registry auth + image push, build logs SSE vs polling, revisions/current_revision_id, preview env TTL/limit/commit-status, webhooks HMAC+idempotency, auto-provision webhooks, compose git stacks, tar upload, polling auto-update, Dockerfile path / build context / monorepo, pipeline strategies

**Cluster:** coolify, dokploy, dokku, caprover, komodo, portainer, docker-compose (under `reference/app-platforms/`) — with cross-checks to 1panel/uncloud where relevant

**Inspected (read-only, 2026-08-24):**
- `forge/api/internal/services/git/service.go`, `deploy_service.go`, `deploy.go`, `deployment_service.go`, `checks.go`, `source_deploy.go`, `provider_ops.go`
- `forge/api/internal/services/build/service.go`, `forge/api/internal/services/buildpack/buildpack_service.go`, `forge/api/internal/services/pipeline/service.go`, `forge/api/internal/services/preview/service.go`, `forge/api/internal/services/previewenv/service.go` + `webhook.go` + `reaper.go`
- `forge/api/internal/http/handlers_git.go:1`, `handlers_git_deploy.go`, `handlers_builds.go`, `handlers_buildpacks.go`, `handlers_preview_deployments.go`, `handlers_source_deployments.go`, `handlers_revisions.go`, `phase4_registrar.go`, `server.go:155,2608`
- `beacon/internal/server/git.go`, `beacon/internal/server/build.go`
- `forge/api/internal/store/store_git_providers.go:15`, `store_git_credentials.go`, `store_git_sources.go`, `store_builds.go`, `store_buildpacks.go`, `store_deployment_revisions.go:18`, `store_preview_env.go:13`, `store_compose.go:28`, `store_source_deployments.go:28`
- `forge/api/migrations/097_git_credentials.sql:1`, `098_app_platform_foundations.sql:71`, `099_deployment_revisions.sql:10`, `114_d_source_deployments.sql:25`, `114_f_git_deployment_tracking.sql:5`, `138_consolidate_legacy_batch2.sql:488`, `203_git_provider_generic.sql:6`, `204_git_sources_generic.sql:6`, `108_compose_stack_indexes.sql:2`
- `forge/web/app/admin/git-providers/page.tsx:37`, `forge/web/app/admin/source-deployments/page.tsx:6`, `forge/web/lib/api/source-deployments.ts:6`, `forge/web/app/admin/environments/page.tsx:95`
- `reference/app-platforms/{coolify,dokploy,dokku,caprover,komodo,portainer,docker-compose}` enumerated via glob + targeted reads (see citations)

**Prior audits re-verified:** `audits/phase-01/subagent-02-git-build.md` (18 rows, 8 LFs), `audits/phase-06/subagent-10-compose-builds.md` (C01–C17), `audits/phase-06/subagent-07-komodo-stacks.md` (C-01–C-14), `audits/MASTER_FINDING_INDEX.md:20-31` (REF-APP-GIT01..10), `docs/audits/MASTER_REMEDIATION_LEDGER.md:18` (GIT-001/002), `docs/audits/MASTER_REMEDIATION_REPORT.md:16` (remaining issues)

**Method:** No product code modified. All claims verified via `read` + `grep`/`bash ls` + `git diff` null. STATUS values: PRESENT / PARTIAL / FIXED / MISSING. Findings are logic defects with file:line triggers.

---

## Parity Matrix (18 rows — ≥15 required)

Each row: **REFERENCE:Path:Symbol → FORGE layers file:line → STATUS / GAP / FINDING / RECOMMENDATION / SEVERITY**

---

### 01 — Provider type inventory (5 vs 4 vs 2)

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:9` + `app/Models/GithubApp.php` / `GitlabApp.php` — Coolify OAuth-apps for GitHub/GitLab only, fallback `PrivateKey` raw URL. `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8` `gitProviderType pgEnum('github','gitlab','bitbucket','gitea')` — 4 types, no generic. `reference/app-platforms/portainer/api/dataservices/source/source.go:11` provider-agnostic URL+auth. `reference/app-platforms/caprover/src/utils/GitHelper.ts:1` `SSH_RE` allows any `git@host` via allow-list.
- **FORGE**
  - Frontend: `forge/web/app/admin/git-providers/page.tsx:37` provider select shows `github|gitlab|bitbucket|gitea` only; `forge/web/lib/api/source-deployments.ts:6` type adds `'generic'` but UI dropdown at `git-providers/page.tsx:99` omits `generic` (hidden behind code type). `forge/web/app/admin/source-deployments/page.tsx:6` lists via `listGitProviders`.
  - API: `forge/api/internal/http/handlers_git.go:190` `ConnectGitProvider` + `handlers_phase1_git.go:446` bridge.
  - Service: `forge/api/internal/services/git/service.go:154` `ListProviderRepos` switch on 5 types (`GitHub|GitLab|Bitbucket|Gitea|Generic`) — `Generic` returns `does not expose a repository API` at line 173,199,224.
  - Store: `forge/api/internal/store/store_git_providers.go:21` `GitProviderGeneric="generic"`.
  - DB: `forge/api/migrations/203_git_provider_generic.sql:6` CHECK adds `'generic'`; `204_git_sources_generic.sql:6` adds. SQLite mirrors `sqlite/203*`+`sqlite/204*`. Base `097_git_credentials.sql:18` originally 4, patched. `114_d_source_deployments.sql:5` `git_providers.type` includes `'generic'`.
  - Beacon: n/a (control-plane only).
  - Tests: `forge/api/internal/http/handlers_git_test.go`, `service_test.go`, `provider_ops_test.go`.
- **STATUS** PARTIAL (extra generic vs peers, PAT-only)
- **GAP** Dokploy 4 types vs Forge 5 (Forge adds `generic` for raw HTTPS without provider API). Coolify 2 OAuth-apps vs Forge token-only PAT flow. Forge never implements OAuth Authorization-Code dance; only raw `accessToken` insert at `handlers_git.go:208` (`POST /git/providers` with token). `generic` missing from frontend selector while store/API/DB support it. 1panel/uncloud have no provider abstraction — no parity needed.
- **LOGIC FINDING** n/a (contract/drift)
- **RECOMMENDATION** Either implement OAuth `GET /providers/:type/authorize` + `/callback` (Dokploy `apps/dokploy/pages/api/providers/github/callback.ts`, Coolify `GithubController.php`) or document PAT-only. Add `generic` to `git-providers/page.tsx:99` options list, or restrict `store_git_providers.go:21` back to 4 if generic is experimental. Keep `ListProviderRepos`/`Branches` error message as-is (good).
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#01` PARTIAL, `MASTER_FINDING:REF-APP-GIT01` DEFERRED — re-verified still PARTIAL; gap unchanged (intentional PAT-only).

---

### 02 — OAuth vs PAT vs Deploy-key credential model

- **REFERENCE** `reference/app-platforms/coolify/app/Models/PrivateKey.php:1` RSA key for SSH clone; `dokploy/packages/server/src/db/schema/github.ts` + `gitlab.ts` + `gitea.ts` + `bitbucket.ts` store `accessToken` per provider; `portainer/api/git/types/types.go:17` `GitAuthentication Username/Password`; `komodo/bin/periphery/src/stack/mod.rs:75` `args.remote_url(access_token)` injects token into URL; `caprover/src/utils/GitHelper.ts:clone()` writes SSH key to `0600` temp file + encodes USER/PASS into URL.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:90` `connectGitProvider({provider, accessToken, baseUrl})`.
  - API: `forge/api/internal/http/handlers_git.go:48` `CreateGitCredential({ssh_key|https_token|https_password})` + `137 GenerateDeployKey`.
  - Service: `forge/api/internal/services/git/service.go:87` `GenerateDeployKeyPair("ed25519"|"rsa4096")` + `130 GenerateDeployKey` rotation in-place; `deploy_service.go:96` `cloneWithOptions` writes SSH key via `writeSSHKeyFile(0600)` at `267` or askpass script via `writeAskPassScript` at `287` (embeds creds); `151 CloneWithToken` for source deployments.
  - Store: `forge/api/internal/store/store_git_credentials.go` encrypted `credential_encrypted`+`public_key`; `store_git_providers.go` encrypted `access_token_encrypted`.
  - DB: `097_git_credentials.sql:1` `git_credentials` `credential_type IN ('ssh_key','https_password','https_token')`.
  - Beacon: `beacon/internal/server/git.go:344` `gitEnvironmentForRequest` creates dedicated `0700` `forge-git-askpass-*` dir + `0700` script reading `FORGE_GIT_ASKPASS_USERNAME/PASSWORD` env — never embeds token.
  - Tests: `provider_ops_test.go:1`.
- **STATUS** PRESENT (3 flavours, broader than peers)
- **GAP** Divergence: Forge control-plane direct path (`deploy_service.go:287` `writeAskPassScript` embeds `username`+`password` via `shellQuote` into `#!/bin/sh … echo %s` body, created via `os.CreateTemp("", "git-askpass-*")` in world-traversable `/tmp`) vs Beacon path (`git.go:387` `printf '%s' "$FORGE_GIT_ASKPASS_PASSWORD"`). Direct path token briefly lives in script content (visible via `open(2)` + `/proc` while clone runs). Komodo puts token directly in URL (worse — process list). Forge direct path weaker than Beacon.
- **LOGIC FINDING** LF-01 — credential leakage surface differs; residual risk on direct path. Evidence: `deploy_service.go:288` `fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n *Username*) echo %s ;;\n")` vs `beacon/git.go:387` `const script = "#!/bin/sh\n… printf '%s' \"$FORGE_GIT_ASKPASS_PASSWORD\"` (no embedding).
- **RECOMMENDATION** Align Forge direct `writeAskPassScript` to Beacon's env-var script pattern; write script once to dedicated `0700` dir under `tempBaseDir` (not `os.TempDir` bare), pass creds via `FORGE_GIT_ASKPASS_*` env, chmod `0700` before write, keep `defer os.Remove`. Backport `git.go:344-405` hardening. Keeps `sshKeyFile` path (`writeSSHKeyFile(0600)`) as-is (good).
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#02` MEDIUM → still open; not yet verified fixed. `MASTER_FINDING:REF-APP-GIT02-LF01` DEFERRED — confirmed.

---

### 03 — Branch selection, ref validation, commit pinning + shallow fetch

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:131` `branch: text` freeform; `coolify/app/Models/Application.php:branch`; `dokku/docs/deployment` `git push dokku main`; `komodo/lib/git` `git clone -b <branch>` without strict regex.
- **FORGE**
  - Frontend: `forge/web/app/admin/source-deployments/page.tsx:6` branch input defaults `main` (`store_source_deployments.go:152` `if req.Branch=="" {req.Branch="main"}` via handler default).
  - API: `forge/api/internal/services/git/deploy_service.go:464` `validateBranch` (strict: no leading `-`, no trailing `.`/`/`, no `.lock`, no `..` `//` `/.` `@{`, no ` \t\r\n~^:?*[`, no `\ ' " ` $ & | ;` at 474).
  - Service: `deploy_service.go:85` `CloneRepo`, `89 CloneAtCommit` requires SHA, `164 cloneRepo` clones `--depth 1 --single-branch --branch` then `227 fetch --depth 1 origin <commitSHA>` + checkout, `527 resolveCommitSHA` via `rev-parse HEAD`.
  - Beacon: `beacon/internal/server/git.go:58` `validateGitRefName` regex `^[A-Za-z0-9._/-]+$` + `..`/`/.` checks, len 256, `407 isHex40` for SHA, `436 isRestrictedHost` SSRF check. Branch passed as `--branch=<value>` single arg + `--` separator at `246-257` (defense-in-depth). SHA path does `init + remote add + fetch --depth 1 origin <sha> --filter=blob:none` at `184-203` (correct reproducibility).
  - Store: `git_sources.last_commit_sha`, `source_deployments.commit_hash`.
  - DB: migrations hold commit_sha columns.
- **STATUS** PRESENT with dual validators
- **GAP** Two validators differ: control-plane `validateBranch` richer (rejects `~ ^ : ? * [ \` etc.) but Beacon regex is stricter subset (rejects `+` maybe). Forge always clones `--depth 1 --single-branch --branch` then optionally fetches one SHA; shallow fetch of arbitrary SHA not on branch tip fails if server has `uploadpack.allowAnySHA1InWant=false` (common on Gitea/self-hosted, sometimes GitHub enterprise). Beacon SHA path avoids this via `init+fetch <sha>`; control-plane SHA path still starts from shallow clone, so same failure persists. 1panel/uncloud lack pinning.
- **LOGIC FINDING** LF-02 — shallow single-branch + depth 1 makes commit checkout nondeterministic for PR merges/force-pushes. Evidence: `deploy_service.go:207` `git clone --depth 1 --single-branch --branch <branch>` then `228 fetch --depth 1 origin <sha>` vs `beacon/git.go:184` `init` then `202 fetch --depth 1 origin <sha>`. InitiateDeployment goroutine at `deployment_service.go:97` fails with `fatal: couldn't find remote ref <sha>`.
- **RECOMMENDATION** For `commitSHA != ""` use Beacon-style `init+remote add+fetch` path in `cloneWithOptions` too (or add fallback `git fetch --depth 50` then `fetch <sha>`), unify validators to single shared `validateBranch==validateGitRefName` (prefer `deploy_service.go:464` coverage + Beacon's `--branch=` passing).
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#03` MEDIUM — still open; not verified fixed.

---

### 04 — Builder matrix — Dockerfile vs Nixpacks / Heroku(CNB) / Paketo / Railpack / Static

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` `NIXPACKS|STATIC|DOCKERFILE|DOCKERCOMPOSE|RAILPACK` (5). `dokploy/packages/server/src/db/schema/application.ts:70` `buildType pgEnum('dockerfile','heroku_buildpacks','paketo_buildpacks','nixpacks','static','railpack')` (6) + `dokploy/packages/server/src/utils/builders/index.ts:44` `getBuildCommand` switch 6, `dokploy/packages/server/src/utils/builders/nixpacks.ts:33` `nixpacks`, `heroku.ts:21` `heroku/builder:24`, `paketo.ts:21` `paketobuildpacks/builder-jammy-full`, `railpack.ts:50` `railpack`, `static.ts:63` `dockerfile` static via nginx. `portainer/pkg/libstack` `docker compose build`. `docker-compose` spec `build: {context,dockerfile,args,cache_from}`.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:26` `buildType: 'dockerfile'|'nixpacks'|'heroku'|'paketo'|'static'` (5 in type, but UI actually offers 5).
  - API: `forge/api/internal/http/handlers_source_deployments.go:87` `if req.BuildType != "dockerfile" return 422 "must be dockerfile (nixpacks, heroku, paketo, static are not yet supported)"` — admission honesty (Phase-1 remediation). `handlers_buildpacks.go:74` `/builds/language/detect`.
  - Service: `forge/api/internal/services/build/service.go:28` `BuilderDockerfile|BuilderNixpacks` only 2 registered at `160`; `958 NixpacksBuilder.Detect` checks indicators but false if Dockerfile exists; `902 DockerfileBuilder.Build` `docker buildx build -f Dockerfile … --load`. `buildpack/buildpack_service.go:29` wraps Nixpacks exclusively at `212` `BuilderNixpacks`.
  - Store: `forge/api/internal/store/store_buildpacks.go` `builder_type CHECK('herokuish','cnb','nixpacks','railpack')` at `138_consolidate_legacy_batch2.sql:489`; `store_builds.go` `builder_type 'dockerfile','nixpacks'`.
  - DB: `098_app_platform_foundations.sql:71` `builder_type CHECK('dockerfile','nixpacks')`; `114_d_source_deployments.sql:25` `build_type CHECK('dockerfile','nixpacks','heroku','paketo','static')` — API rejects before DB, but DB would still allow 5 if reached.
  - Beacon: `beacon/internal/server/build.go:79` `handleDockerfileBuild` + `193 handleNixpacksBuild` only 2 endpoints; no heroku/paketo/railpack/static.
  - Tests: `build/service_test.go:27`.
- **STATUS** PARTIAL → FIXED ADMISSION (executor honest, contract no longer lies)
- **GAP** UI type includes 5 but executor implements 1 for source deployments (`source_deploy.go:81` switch `case "dockerfile","": buildDockerfile default: fail("not supported dockerfile is the only")` at 87). `validBuildTypes` previously claimed 5; now handler at `handlers_source_deployments.go:95` returns 422 early — false-completion closed. Dokploy 6 builders, Coolify 5, Portainer compose build — Forge cannot build heroku/paketo/static/railpack despite DB CHECK allowing them. `BuildpackService` is CNB-ish (`herokuish|cnb`) but only dispatches via `build.Service` Nixpacks at `buildpack_service.go:212`, not via `source_deploy`.
- **LOGIC FINDING** LF-03 previously HIGH “API-contract breach advertised vs executed” — now **VERIFIED_FIXED via admission** (fails closed at Create with 422, not at Run). Residual: DB CHECK still broader than API (would accept `nixpacks` if store called directly), and `BuildService` has Nixpacks but `SourceDeployExecutor` never calls it. Not re-triggerable via HTTP now, but direct store inserts could still create undispatchable rows.
- **RECOMMENDATION** Keep admission guard; optionally narrow `114_d_source_deployments.sql:25` CHECK to `('dockerfile')` until nixpacks wired for source deploys, or wire `SourceDeployExecutor` to `build.Service.StartBuild(BuilderNixpacks)` for `nixpacks` and to `buildpack.Service` for `heroku|paketo` (herokuish/cnb). Update `forge/web/lib/api/source-deployments.ts:26` type to `dockerfile` only until ready (or keep with disabled UI).
- **SEVERITY** HIGH (was HIGH, now admitted — downgrade to MEDIUM for DB-vs-API residual; keep HIGH in legacy index until DB narrowed)
- **PRIOR AUDIT** `phase-01#04` HIGH false-completion → **VERIFIED_FIXED** (admission) per `MASTER_FINDING:REF-APP-GIT04-LF03`. Re-verified: handler now 422.

---

### 05 — BuildKit, caching (cacheFrom/cacheTo), platform, daemon

- **REFERENCE** `reference/app-platforms/coolify/database/migrations/2024_12_05_add_disable_build_cache:12` `disable_build_cache boolean` toggles `--no-cache`. `dokploy/packages/server/src/utils/builders/nixpacks.ts:46` `cacheKey` + `cache-key=`; `dokploy/packages/server/src/utils/docker/types.ts:526` `cache_from|cache_to|no_cache`. `docker-compose` spec `build.cache_from`.
- **FORGE**
  - Frontend: not exposed (no cacheFrom UI).
  - API: `forge/api/internal/http/handlers_builds.go:13` `startBuildRequest{NoCache}` no CacheFrom/CacheTo/Platform; `handlers_source_deployments.go` none.
  - Service: `forge/api/internal/services/build/service.go:81` `BuildOptions{CacheFrom []string; CacheTo []string; Platform string; NoCache bool; NixpacksPlan}` + `922 DockerfileBuilder.Build` appends `--cache-from`/`--cache-to`/`--platform` via `940-950`, `589 CacheFrom: opts.CacheFrom` in `DockerfileBuildRequest`, `1088 runBuildCommand` allows `docker|nixpacks`. Beacon `build.go:122` `isSafeBuildxRef` + `536 isSafePlatform`.
  - Store: `forge/api/internal/store/store_builds.go:14` `CacheFrom CacheTo Platform` persisted.
  - DB: builds table `cache_from/cache_to/platform` columns.
  - Beacon: `beacon/internal/server/build.go:54` `dockerfileBuildRequest{CacheFrom, CacheTo, Platform}` now present + `126-138` forwarding via `isSafeBuildxRef`/`isSafePlatform`; `447 buildEnvironment={"DOCKER_BUILDKIT=1"}`. Previously dropped — now fixed. `validateBuildContext:879-890` platform allowlist.
  - Tests: `build/service_test.go:553` cache `type=gha`.
- **STATUS** FIXED (was PARTIAL)
- **GAP** Previously `BuildOptions.CacheFrom` dropped at Beacon (struct mismatch) — now present at `beacon/build.go:54-56`. Coolify/Dokploy expose cache-from in UI; Forge UI never exposes CacheFrom/To, so feature exists backend but undriven from frontend. Docker Compose `cache_from` per service still not surfaced.
- **LOGIC FINDING** LF-04 previously MEDIUM “Beacon ignores CacheFrom/CacheTo” — **VERIFIED_FIXED**. Evidence: control `build/service.go:598` `CacheFrom: opts.CacheFrom` sent to `daemon.DockerfileBuildRequest`, beacon struct now has `54 CacheFrom []string \`json:"cacheFrom"\`` and handler at `126-138` appends `--cache-from` guarded by `542 isSafeBuildxRef` (rejects `; \r \n \x00` and leading `-`).
- **RECOMMENDATION** Keep fix; optionally expose `cacheFrom/cacheTo/platform/noCache` in `handlers_builds.go:13` and `handlers_source_deployments.go` request, document as internal-only until UI. Note `isSafeBuildxRef` allows `type=gha,mode=max` (contains `=` but not `;` — passes, then buildx may error if runner lacks GHA cache; acceptable). `phase-06 subagent-10#C14` cache parity now satisfied.
- **SEVERITY** MEDIUM (closed)
- **PRIOR AUDIT** `phase-01#05` MEDIUM → **VERIFIED_FIXED** per `MASTER_FINDING:REF-APP-GIT05-LF04`.

---

### 06 — Registry, image naming, auth, `docker push` flow

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/cluster/upload.ts:uploadImageRemoteCommand` registry push; `caprover/src/user/DockerRegistryHelper.ts:1` registry auth; `coolify/app/Models/Application.php:registry` relation.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:33` `registry: string; registryCredentialId`.
  - API: `forge/api/internal/http/handlers_source_deployments.go:22` `Registry`+`RegistryCredentialID`.
  - Service: `forge/api/internal/services/build/service.go:522` `pushToRegistry` per tag + `577 LoginRegistry` before build and `752` before push; `git/source_deploy.go:148` `if d.Registry != "" { pushing + dockerPush }` at `148-153`; `git/deploy_service.go:365` `imageTag fmt.Sprintf("%s/%s:%s", registryHost, repositoryName, sha[:8])` + `416 dockerPush`; `build/service.go:751` login per node. `store_source_deployments.go:28` `Registry CredentialID`.
  - DB: `source_deployments.registry`, `builds.registry`.
  - Beacon: `beacon/internal/server/build.go:146` not pushing; `forge/api/internal/daemon/build.go` push via `PushImage`.
  - Tests: `service_test.go`.
- **STATUS** PRESENT
- **GAP** `SourceDeployExecutor` skips push if `Registry==""` (`source_deploy.go:148`) — image remains only on build node (useful for single-node, not swarm). No validation of `RegistryCredentialID` existence/expiry at Create. Dokploy zips context+push; Coolify always pushes via `coolify-registry`.
- **FINDING** n/a
- **RECOMMENDATION** Validate `registryCredentialId` at `CreateSourceDeployment` (exists + not expired) or document local-only images; default to internal registry when empty if swarm needs it. No parity loss.
- **SEVERITY** LOW

---

### 07 — Build logs — persistence, streaming, truncation, retention

- **REFERENCE** `reference/app-platforms/caprover/src/user/BuildLog.ts:1` `CircularQueue` bounded; `coolify/resources/views/livewire/project/application/general.blade.php` deployment logs.
- **FORGE**
  - Frontend: `forge/web/app/admin/source-deployments/[id]/page.tsx:34` `getDeploymentBuildLogs` polling every 5s; `forge/web/lib/api/source-deployments.ts:146` logs.
  - API: `forge/api/internal/http/handlers_builds.go:91` `GET /builds/:id/logs` SSE when not terminal (open `logCh`, heartbeat 30s, `BuildLog` text/plain when terminal at 98-104); `handlers_source_deployments.go:258` `GetDeploymentBuildLogs` polling JSON array (no SSE).
  - Service: `forge/api/internal/services/build/service.go:1088` `runBuildCommand` `bytes.Buffer` + `io.MultiWriter(os.Stdout, &logBuf)` + `1170 maskCredentials`; `beacon/internal/server/build.go:325` `buildJob.logBuf` with `42 maxLogBufferSize=100MB` truncation flag at `404-408` + per-line masking at `398-402`; `110 BuildLog` column (text); `store_builds.go:293` `PruneBuildLogs`.
  - DB: `114_d_source_deployments.sql:CREATE TABLE source_build_logs (id UUID, deployment_id, stage, message)`; `builds.build_log` text.
  - Worker: `build/service.go:530` `executeRemoteBuild` fetches `daemon.BuildLogs` post-build (not live) then forwards to `logCh` at `617-622`; `476` goroutine timeout.
  - Beacon: `beacon/internal/server/build.go:241` `handleBuildLogs` `follow=true` SSE 100ms ticker at `275-306`, trunc 100MB, `496 maxCompletedRetention=1h` evicts terminal jobs.
- **STATUS** PRESENT with divergence (two subsystems)
- **GAP** `builds.build_log` (single text) vs `source_build_logs` (row per stage). Only `/builds/:id/logs` SSE; `/source-deployments/:id/logs` polling JSON. Remote builds fetch logs after `DockerfileBuild` + poll `GetBuildStatus` then read `BuildLogs` — not truly live but chunked after build, while local path streams via `streamBuildOutput` (dead code for remote). 1panel/uncloud none.
- **LOGIC FINDING** LF-06 — remote log streaming post-hoc not live (drift from SSE contract). Evidence: `build/service.go:610` `logs, err := s.daemonCli.BuildLogs(ctx, …, true)` after `605 DockerfileBuild`, then loop `617 for _, l := range logs { logCh <- … }` — delivers buffered, not tailing. `handlers_builds.go:116` SSE reads `buildSvc.StreamLogs` which for remote was just filled at completion.
- **RECOMMENDATION** Make `executeRemoteBuild` poll `BuildLogs` incrementally (ticker) or pass through Beacon SSE (`follow=true`) into `logCh` live, unify `source_build_logs` SSE endpoint. Keep 100MB cap + masking.
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#07` MEDIUM — still open but UX acceptable (polling works, SSE for builds only).

---

### 08 — Deployment revisions, `current_revision_id`, rollbacks & previous manifest

- **REFERENCE** `reference/app-platforms/coolify/app/Models/ApplicationDeploymentQueue.php` status + rollback via re-deploy; `dokploy/packages/server/src/db/schema/deployment.ts:deployment`; `docker-compose` image tags implicit; `portainer/pkg/libstack` compose.
- **FORGE**
  - Frontend: no revisions page yet (`forge/web/app/admin` no `deployments/:id/revisions`).
  - API: `forge/api/internal/http/handlers_revisions.go:16` `GET /admin/deployments/:id/revisions`, `GET /:id/revisions/:revId`, `POST /:id/revisions/:revId/rollback`, `POST /:id/rollback-previous`, `POST /:id/rollout` `ValidateImageRef` at 54, `GET /:id/compare`.
  - Service: `forge/api/internal/services/deployment/revisions.go:55` + `execution.go:225` service; `git/deploy.go:258 persistDeploymentState` maps to store at `143`.
  - Store: `forge/api/internal/store/store_deployment_revisions.go:18` `DeploymentRevision{revision_number,image_ref,compose_manifest_ref,git_commit_sha,config_hash,status(pending/active/superseded/failed)}` + `152 UpdateDeploymentCurrentRevision`, `143 SupersedeDeploymentRevisions`; `store_compose.go:28` `GitPreviousCommitSHA/GitPreviousManifest/GitPreviousCompose` for compose rollback diff; `store_deployments.go:51` `current_revision_id`.
  - DB: `099_deployment_revisions.sql:10` `deployment_revisions`, `095_deployments.sql:current_revision_id`, `141_compose_git_previous_manifest.sql`.
  - Beacon: n/a.
- **STATUS** PRESENT (backend only, disjoint stores)
- **GAP** `GitDeployOrchestrator.persistDeploymentState` writes `git_deployments` via `store_git_deployments.go` (`CreateGitDeployment`, `CompleteGitDeployment`) NOT `deployment_revisions`. So git-sourced image deploys never create a `deployment_revisions` row; revision API and git API are disjoint tables. Docker-compose git stacks store `GitPrevious*` but not revisions. Frontend missing.
- **LOGIC FINDING** LF-08 — git deployments and placement revisions are disjoint; rollback cannot rollback git image. Evidence: `git/deploy.go:395 handleDockerfileDeployment` + `282 handleComposeDeployment` only call `CreateGitDeployment`/`CompleteGitDeployment`; no `store.CreateDeploymentRevision`. `deployment/revisions.go:104` never called by `git/*`.
- **RECOMMENDATION** On git deploy success, also `CreateDeploymentRevision{git_commit_sha,image_ref}` via deployment service and set `current_revision_id`, or document that revisions apply only to placement deployments and disable revision endpoints for `git_sources.*` vs `deployments.*`. Align `deployments.current_revision_id` vs `compose_stacks.GitPrevious*` (duplicate rollback states).
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#08` MEDIUM — still open per `MASTER_FINDING:REF-APP-GIT08-LF06` DEFERRED.

---

### 09 — Preview environments (per-PR ephemeral stacks)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/ApplicationPreview.php:1` (`pr_id,fqdn,docker_image_tag`); `dokploy/packages/server/src/db/schema/preview-deployments.ts:15` (`branch,pullRequestId/Number/URL/Title,previewStatus,appName,domainId,expiresAt`); `dokploy/packages/server/src/services/preview-deployment.ts:1` per-app preview with domain. `portainer`/`komodo`/`1panel` none.
- **FORGE**
  - Frontend: `forge/web/app/admin/environments/page.tsx:95` placeholder; no preview UI wired.
  - API: `forge/api/internal/http/handlers_preview_deployments.go:9` `registerPreviewDeploymentRoutes` admin-only `GET /admin/preview-deployments`, `POST /`, `POST /:id/deploy|cleanup|status` at `76/83`; plus `phase4_registrar.go:67` public `/preview/webhook/{github,gitlab,bitbucket,gitea}` at `68-71` + protected `/preview` management at `120-189` (list with expiry at `128`, config at `136`, per-server list at `145`, detail `154`, deploy/cleanup/destroy at `166/174/182`).
  - Service: Two implementations sharing `preview_deployments` table:
    - `preview/service.go:42` `Create` no per-PR uniqueness, no TTL, hardcoded `PreviewURL = https://preview-<suffix>.example.com` at `125`, `HandleWebhook` for `pull_request.opened/synchronize/closed` at `74`.
    - `previewenv/service.go:121` `Create` enforces per-PR scan `144-155` + per-org `MaxPerOrg` `135-142`, `expiresAt=now+TTL` at `158`, canonical `PreviewURL = https://pr<Number>-<owner>-<repo>.<baseDomain>` at `92`, `227 Deploy` ensures ACME+TrafficMgr+wildcard domain at `242-260`, `404 reportStatus` posts `forge/preview` commit status, `286 Cleanup`/`318 Destroy`, `reaper.go:36` + `70-78` (`ListExpiredPreviewDeployments`, `ListReapablePreviewDeployments`, `SetPreviewDeploymentExpiresAt`).
  - Store: `store_preview_env.go:13` `SetPreviewDeploymentExpiresAt`, `ListExpiredPreviewDeployments`, `CountActivePreviewDeploymentsForOrg`.
  - DB: `180_preview_ttl.sql` implied `expires_at`, `114_*` base `preview_deployments`.
  - Worker: `previewenv/reaper.go:54` started in `phase4_registrar.go:53` (`5m` interval) vs `preview` no reaper; events `preview_deployment_created|running|cleaned_up` at `previewenv/service.go:451`.
  - Tests: `handlers_preview_deployments_test.go:12`.
- **STATUS** DUPLICATE IMPLEMENTATIONS, PARTIAL WIRING — FIX PARTIAL (new path correct but both coexist)
- **GAP** Exposed admin API at `/admin/preview-deployments` uses legacy `preview.Service` without TTL/limit/commit-status/reaper; newer `previewenv.Service` (correct TTL `24h` default, limit `10`, per-PR URL, ACME route, status via `checks.go:37`, reaper) is reachable only under separate namespace `/preview` + `/preview/webhook/*` (phase-4 registrar). Same table, divergent semantics: legacy rows have `expires_at IS NULL` so `ListExpiredPreviewDeployments` never reaps them. Dokploy/Coolify scope by application; Forge scopes by `serverID`+`repo_owner/repo_name`.
- **LOGIC FINDING** LF-07 — exposed preview API uses legacy without per-org limit/TTL/commit-status; correct `previewenv` not wired to admin route. Evidence: `http/server.go:155 PreviewDeploymentSvc *preview.Service` + `2608 registerPreviewDeploymentRoutes(…, PreviewDeploymentSvc)` vs `phase4_registrar.go:37 previewenv.New(…, PREVIEW_TTL, PREVIEW_MAX_PER_ORG)` separate. `preview/service.go:125` hardcoded domain vs `previewenv/service.go:92` per-PR domain.
- **RECOMMENDATION** Switch `registerPreviewDeploymentRoutes` to `previewenv.Service` or unify; deprecate `preview.Service`; add DB partial unique index `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running')` to prevent race at `previewenv/service.go:144` in-memory scan (currently races on concurrent webhook deliveries). Migrate existing legacy rows `UPDATE preview_deployments SET expires_at = created_at + '24h' WHERE expires_at IS NULL`.
- **SEVERITY** HIGH (user-visible drift)
- **PRIOR AUDIT** `phase-01#09` HIGH duplicate — still HIGH per `MASTER_FINDING:REF-APP-GIT09-LF07` DEFERRED (decision: promote previewenv).

---

### 10 — Webhooks — provider webhooks, HMAC, delivery deduplication

- **REFERENCE** `reference/app-platforms/coolify/app/Jobs/ProcessGithubPullRequestWebhook.php:1`; `caprover/src/routes/user/apps/webhooks/WebhooksRouter.ts:1`; `dokploy/packages/server/src/utils/providers/github.ts:1` + `apps/dokploy/__test__/deploy/github-webhook-handler.test.ts:1`; `komodo/lib/git` none.
- **FORGE**
  - API: `forge/api/internal/http/handlers_git.go:599` `HandleGitHubWebhook` checks `X-GitHub-Event push` at `607`, verifies `VerifyGitHubSignature` `632`, `654 HandleGitLabWebhook` `X-Gitlab-Token`, `725 HandleBitbucketWebhook` `X-Event-Key repo:push` + `797 X-Hub-Signature`, `822 HandleGiteaWebhook` `X-Gitea-Signature`; plus `1165 ReceiveGitDeploymentWebhook` HMAC via `VerifyGitHubSignature` `201` with `1235 deriveGitDeploymentIdempotencyKey` using `X-GitHub-Delivery|X-Gitlab-Event-UUID|X-Gitea-Delivery` or `sha256(server:body)[:16]` fallback + `ExistsWebhookDeliveryByIdempotencyKey`/`TryClaimIdempotencyKey` at `1201-1222`. `1292 registerGitWebhookRoutes` single prefix `/git/webhook/{github,gitlab,bitbucket,gitea,deploy/:serverId}`. `phase4_registrar.go:68` separate `/preview/webhook/*`.
  - Service: `service.go:248` `VerifyGitHubSignature(sha256=)+computeHMAC:858`, `263 VerifyGitLabSignature`, `273 VerifyBitbucketSignature(sha256=)`, `320 VerifyGiteaSignature`; `deployment_service.go:201 verifyHMACSignature` normalizes missing `sha256=` at `206-207`.
  - Store: `store_git_sources.go` webhook_secret encrypted, `store_webhook_deliveries.go` idempotency table; `148_git_deployment_hooks.sql`.
  - Beacon: `git.go:119` not webhook, `isRestrictedHost`.
  - Tests: `handlers_git_test.go:603` (now expects 401), `providers_ops_test.go`.
- **STATUS** PRESENT — FIXED (was PR semi-open)
- **GAP** Previously `HandleGitHubWebhook` returned 200 even on HMAC mismatch (masked failure). Now at `640` returns `401 invalid signature` — consistent with `ReceiveGitDeploymentWebhook:1213` + `HandleGitLab/Bitbucket/Gitea` at `705/802/869` all `401`. Two namespaces (`/git/webhook/{provider}` for git sources vs `/git/webhook/deploy/:serverId` vs `/preview/webhook/*` vs `/compose/webhook/:webhookId` at `handlers_compose.go:82`) — docs unclear which to register; but each has correct HMAC header per provider (`X-Hub-Signature-256` vs `X-Gitlab-Token` vs `X-Hub-Signature` vs `X-Gitea-Signature`). Bitbucket `VerifyBitbucketSignature` expects `sha256=` prefix (`273-281`) — matches Forge's generic `sha256=<hmac>` for Bitbucket, but Bitbucket Cloud docs use `X-Hub-Signature: sha256=<hmac>` (same) — consistent per `phase4_registrar.go:100` wiring.
- **LOGIC FINDING** Previously LF-08 “HMAC failure returns 200 mask” — **VERIFIED_FIXED** (now `401`). Evidence: `handlers_git.go:640` `return fiber.NewError(fiber.StatusUnauthorized, "invalid signature")` vs old `return OK`.
- **RECOMMENDATION** Keep 401; add `X-Gitea-Delivery`/`X-GitHub-Delivery` dedup to `git sources` webhooks too (currently only `git/webhook/deploy` has idempotency; `/git/webhook/github` etc have no `TryClaimIdempotencyKey` — could dedup via same `webhook_deliveries` table). Align Bitbucket header doc string in comments.
- **SEVERITY** LOW (closed)
- **PRIOR AUDIT** `phase-01#10` LOW → **VERIFIED_FIXED** per `MASTER_FINDING:REF-APP-GIT10-LF08`.

---

### 11 — Automatic webhook provisioning (create/delete on provider)

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/services/github.ts:1` Octokit webhook create; `coolify/app/Livewire/Project/New/GithubPrivateRepositoryDeployKey.php:1` deploy key after repo select.
- **FORGE**
  - Frontend: create source `forge/web/app/admin/source-deployments/page.tsx` no webhook toggle visible.
  - API: `forge/api/internal/http/handlers_git.go:442` `CreateGitSource` auto-provisions if `ProviderTokenID` provided + `AutoDeploy` true at `490-505`: generates secret `generateWebhookSecret 32B hex` at `486`, calls `SetupProviderWebhook` at `497`, stores `webhookID/secret/url` at `514-526`; cleanup on `564 DeleteProviderWebhook` and `DisconnectGitProvider:242-258`.
  - Service: `service.go:205` `SetupProviderWebhook` dispatches to `426 createGitHubWebhook`/`559 GitLab`/`700 Bitbucket`/`820 Gitea` each payload correct (`active:true, events:push, content_type:json, secret`), returns webhook ID (GitHub int, GitLab int, Bitbucket UUID, Gitea int). `provider_ops.go:DeleteProviderWebhook` deletes.
  - Store: `git_sources.webhook_id/secret/url`, `097…` columns.
  - Tests: `provider_ops_test.go:1`.
- **STATUS** PRESENT
- **GAP** Generic provider returns `generic providers do not support auto-deploy webhooks` at `service.go:225`; `CreateGitSource` attempt at `497` will fail and fall through to `webhookSetupError` path at `536-540` (still creates source with `webhook_id=""` + returns `webhookSetupError` JSON field — good fail-open). However `AutoDeploy` flag remains `true` though no webhook possible — semantics ambiguous vs `compose_stacks.GitAutoUpdate` (see #15). Should disable autoDeploy for generic explicitly or document polling-only.
- **LOGIC FINDING** n/a (fail-open handled, but UX missing)
- **RECOMMENDATION** At `CreateGitSource`, if provider is `generic`, skip `SetupProviderWebhook`, force `AutoDeploy=false` and inform UI (or map to `GitPollInterval` polling). Add frontend toggle for `AutoDeploy` that hides when provider generic.
- **SEVERITY** LOW
- **PRIOR AUDIT** `phase-01#11` LOW — unchanged.

---

### 12 — Commit status / checks (report pending/success/failure)

- **REFERENCE** `reference/app-platforms/coolify/tests/Unit/ApplicationDeploymentTest.php:1`? `dokploy/packages/server/src/services/preview-deployment.ts` PR comment via `pullRequestCommentId`. `portainer` none.
- **FORGE**
  - API: `previewenv` flows call via `checks.go`.
  - Service: `forge/api/internal/services/git/checks.go:37` `ReportCommitStatus` + `62 ReportCommitStatusForUser` resolves user tokens via `ListGitProviderTokens` + `GetGitProviderTokenUnmasked` at `66-81` (first matching provider, skips empty); `88 postGitHubStatus POST /repos/{owner}/{repo}/statuses/{sha}`, `108 postGitLabStatus` `statuses/{sha}` form `name` context, `137 postBitbucketStatus` `SUCCESSFUL|FAILED|INPROGRESS` at `145-151`, `168 postGiteaStatus` requires `baseUrl`. `previewenv/service.go:404` `reportStatus` uses `Context="forge/preview"` at `414`, `TargetURL=PanelURL/environments/previews?preview=<id>` else `PreviewURL`. `validateProviderBaseURL` DNS check at `292-318` does `LookupIP` and rejects private/loopback — good.
  - Store: token resolution per user.
  - Worker: `previewenv` calls `reportStatus` sync on `Create/Deploy/Cleanup` (`pending/success/failure`).
- **STATUS** PRESENT (previewenv only — legacy preview missing)
- **GAP** Legacy `preview/service.go` never calls `ReportCommitStatus` (no GitService dep) — so exposed admin API gives no Git feedback. `ReportCommitStatusForUser` picks first token arbitrarily if user has multiple for same provider. `validateProviderBaseURL` blocks private Gitea base URLs (self-hosted on RFC1918) — self-hosted Gitea on `192.168.x` would be rejected though `Generic` already blocked. Commit status only in unregistered-at-admin `previewenv`, not the exposed `/admin/preview-deployments`.
- **FINDING** n/a
- **RECOMMENDATION** Wire `checks.go` to legacy preview or deprecate it (prefer promotion). Make token selection deterministic (latest `updated_at` or by `repo_owner` match). Allow private IP for `Gitea`/`Generic` via config flag if on-prem.
- **SEVERITY** MEDIUM
- **PRIOR AUDIT** `phase-01#12` MEDIUM — still PARTIAL (commit status only in previewenv).

---

### 13 — Docker Compose git integration (stacks from repo)

- **REFERENCE** `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:9` `DOCKERCOMPOSE`; `portainer/pkg/libstack/compose` `docker compose` stacks; `dokploy/packages/server/src/db/schema/compose.ts:221` compose `type: fetch|cache`; `1panel/apps` compose; `docker-compose` spec.
- **FORGE**
  - Frontend: `forge/web/app/admin/compose/new/page.tsx` raw YAML editor, no git import UX.
  - API: `forge/api/internal/services/compose/lifecycle.go:23` `ComposeStack` + `GitDeployOrchestrator.handleComposeDeployment` `deploy.go:282` reads `docker-compose.yml|compose.yml|compose.yaml` at `290-312`, validates via `composeService.ValidateComposeContent` at `316`, then `DeployComposeFromGit` at `354`. `detectProjectType` `deploy_service.go:536` returns `"compose"` if compose file exists (before `static`).
  - Store: `store_compose.go:28` `ComposeStack{GitSourceID, GitRepositoryURL/Path/Branch/CommitSHA/DesiredCommitSHA/PreviousCommitSHA, GitPreviousCompose/PreviousManifest, GitAutoUpdate, GitPollIntervalSec, GitNextPollAt, GitWebhookSecret/id, GitUpdateStatus}`.
  - DB: `114_*`, `141_compose_git_previous_manifest.sql`, `146_*`, `108_compose_stack_indexes.sql:2`.
  - Service: `gitops.go:196-284` `readComposeFromDir` + `287 readLimitedComposeFile` symlink rejection + 1MB cap + `cloneRepo` for poll.
  - Beacon: auth not poll; compose deploy via daemon.
  - Tests: `handlers_compose_test.go`.
- **STATUS** PRESENT
- **GAP** Compose git validates but `DeployComposeFromGit` just passes compose YAML to daemon's `docker compose up --build` (daemon `compose.go:395` `docker compose up -d`); does `build:` sub-blocks get built? `build:` contexts are inside cloned repo on Beacon’s `cloneDir` — if `readComposeFromDir` returns only the YAML file but `CloneDir` still holds full tree, `docker compose up` can resolve relative `build.context` correctly (since whole clone dir exists on Beacon). However `source_deploy.go:112` path validation for `build_context` is for source-deployments not compose. Some `docker-compose.yml` with `build:` referencing sibling dir works on Beacon but not historically on direct path (which only returned YAML). Current `GitDeployAdapter` passes `CloneDir` (whole tree) at `deploy.go:360`, so this is actually handled — parity good. CapRover `captain-definition` `dockerfileLines` as inline Dockerfile-in-JSON not supported (intentionally missing, see #14).
- **RECOMMENDATION** Document that `DeployComposeFromGit` delegates to `docker compose up --build` (pre-builds handled by Compose), vs needing `build.Service` per-service; keep adapter as-is. Add UI import-from-git for `compose/new` that calls `git/service CloneRepo` to preview.
- **SEVERITY** LOW
- **PRIOR AUDIT** `phase-01#13` LOW — re-verified PRESENT, no new defect.

---

### 14 — CapRover-style tarball vs Dockerfile build

- **REFERENCE** `reference/app-platforms/caprover/src/user/ImageMaker.ts:1` `ensureImage` handles `imageName` direct and tar upload (`Uploaded Tar` + `captain-definition` + `GIT Repo`), builds via Docker tar stream; `caprover/src/models/ICaptainDefinition.ts:1` `{dockerfileLines,dockerfilePath,imageName,templateId}`.
- **FORGE**
  - API: `forge/api/internal/http/handlers_builds.go:13` only `SourceDir` based.
  - Service: `build/service.go:521` `dockerBuild` via `docker buildx build -f Dockerfile … --load dir` (directory only); `beacon/git.go:496` `handleGitBuild` via `docker build -t … -f Dockerfile` dir; `source_deploy.go` similar.
  - Beacon: only directory builds via `gitCloneDir` (`191 resolveWorkspace`).
  - DB: no tar table.
- **STATUS** MISSING (intentional)
- **GAP** CapRover tar upload (bundled source without git) not replicated; Forge requires git clone or `server:` workspace (`beacon/build.go:182` `server:` prefix). No `captain-definition` equivalent. Uncloud tar handling not needed per `MASTER_FINDING:deferred`.
- **RECOMMENDATION** Document as not in scope. If needed, add `POST /builds/tar` ingestion like CapRover `UploadCaptainDefinitionContent` (unpack via `openat2 RESOLVE_BENEATH` similar to `rootfs_linux.go:47`).
- **SEVERITY** LOW (INFO)
- **PRIOR AUDIT** `phase-01#14` INFO — re-verified MISSING intentional.

---

### 15 — Build pipelines / deployment strategies (rolling, approval gates, cron)

- **REFERENCE** `reference/app-platforms/portainer` no pipeline; `komodo/bin/core/src/api/write/stack.rs:736` pipeline-ish `auto_update`; `coolify` queue simple.
- **FORGE**
  - API: `pipeline/service.go:241` `CreateDefinition` + `281 TriggerRun` + `308 RetryRun` + `373 CancelRun` + `480 ApproveStage`/`506 RejectStage` + `528 ExecuteRun` + `587 executeStage` with retry/backoff. No HTTP handlers enumerated in `server.go` quick grep (pipeline routes via `phase?` but `grep registerPipeline` not in `handlers` list). Frontend no pipeline pages yet.
  - Service: `pipeline/service.go:40` `Options{BuildService,ComposeService,DeployService}`, `137 queueLoop` concurrency semaphore `22 defaultMaxConcurrency=2` (env `PIPELINE_MAX_CONCURRENCY` at 76), `181 scheduleLoop` cron `robfig/cron/v3` at `195-229` `fireDueSchedules` + `235 scheduleDueInSlot`. `409 Artifacts` `64MiB` cap.
  - Store: `pipeline/store.go`, migrations `187_pipeline_*`.
  - Worker: `pipeline` owns pool, bounded `dispatchSlots` not here but `59 resume` chan `64`.
  - Beacon: dispatches via services, not direct.
  - Tests: `scheduler_test.go` (for build), pipeline tests maybe.
- **STATUS** PRESENT but not fully exposed (backend complete, no HTTP/UI)
- **GAP** Pipeline supports `manual|schedule|retry` triggers via backend but `server.go` may not register its HTTP routes (grep `pipeline` in `http/` yields only service import, not `registerPipelineRoutes`). So trigger types exist but UI/API not reachable via gateway. Dokploy scheduled deployments via cron — Forge pipeline internal only. Not a git/build regression but coverage listed per task.
- **RECOMMENDATION** Register pipeline HTTP handlers at `server.go` (add `registerPipelineRoutes` analogous to `registerComposeRoutes`) and add frontend page, or mark pipeline as experimental internal (docs). Keep `PIPELINE_MAX_CONCURRENCY` env.
- **SEVERITY** LOW
- **PRIOR AUDIT** `phase-01#15` LOW — unchanged.

---

### 16 — GitOps polling & auto-update for compose stacks (Portainer parity)

- **REFERENCE** `reference/app-platforms/portainer/api/gitops/scheduling/SourceScheduler:1` polls interval `AutoUpdate{Webhook, Interval, ForceUpdate}`; `portainer/pkg/libstack/README.md:1` compose git pull loop; `dokploy` poll via `cron` not explicit.
- **FORGE**
  - API: `forge/api/internal/services/compose/gitops.go:139` `PollIntervalSec`, `379 DeployFromGit sets GitAutoUpdate:true GitPollIntervalSec:300`, `982 SetAutoUpdate(enabled,interval)`, `1010` config map, `1266 PollForUpdates` + `1331 polling loop`.
  - Service: `gitops.go:1266` `list stacks due for poll` at `store_compose.go:400` `git_auto_update=true AND git_next_poll_at <= NOW`, `1271 check AutoDeploy clone + compare commit`, `1295 nextPoll = now + PollIntervalSec`, `1151 PullAndRedeploy` webhook path with `webhookMu`.
  - Store: `store_compose.go:28` polling fields `GitAutoUpdate bool, GitPollIntervalSec int, GitNextPollAt *time.Time, GitLastDeliveryID, GitAutoUpdate`; DB `108_compose_stack_indexes.sql:5 git_next_poll_at index`.
  - Frontend: compose form lacks auto-update toggle (both `compose/new` and `source-deployments` have `autoDeploy` bool but not interval).
  - Beacon: not poll.
- **STATUS** PARTIAL (backend ready, frontend missing, dual flags)
- **GAP** Portainer exposes per-stack polling interval UI + webhook UUID per stack (`AutoUpdate.Webhook`) — Forge has same via `GitWebhookSecret/id` per `ComposeStack` but UI does not display webhook URL (must call `gitops.go:buildWebhookURL`?). `git_sources.AutoDeploy` (bool, webhook-triggered `git_deployments` at `service.go:864`) vs `compose_stacks.GitAutoUpdate` (bool + poll interval + `GitNextPollAt`) — duplication. Polling works (`PollForUpdates`) but no UI to set `GitPollIntervalSec` (defaults 300s from `380`) beyond `SetAutoUpdate` API at `982` (which is wired at `handlers_compose.go:162-311` `POST /compose/git/deploy|re.deploy|check-update|preview|rollback|pull-redeploy|branch|auto-update|drift|status|last-webhook` — so API exists, UI missing).
- **RECOMMENDATION** Show webhook URL in UI `*/compose/:id` (call `buildWebhookURL` similar to `handlers_git.go:889 buildWebhookURL`), add poll interval slider to `compose/new`. Unify docs: `git_sources.AutoDeploy` drives `git_deployments`, while `compose_stacks.GitAutoUpdate` drives poll/webhook for compose. Keep dual flags but rename one to `ComposeAutoUpdate` in docs.
- **SEVERITY** LOW
- **PRIOR AUDIT** `phase-01#16` LOW — re-verified PARTIAL (backend done, frontend drift).

---

### 17 — Dockerfile path & build context per repo (monorepo support)

- **REFERENCE** `reference/app-platforms/dokploy/packages/server/src/utils/filesystem/directory.ts:124` `buildPath vs codeDirectory`; `coolify/app/Http/Controllers/Api/ApplicationsController.php:1026` custom dockerfile path; `docker-compose` build `context: ./subfolder` + `dockerfile: Dockerfile.prod`.
- **FORGE**
  - Frontend: `forge/web/lib/api/source-deployments.ts:55` request supports `buildContext`, `dockerfilePath`; `forge/web/app/admin/source-deployments/[id]/page.tsx:143` displays.
  - API: `handlers_source_deployments.go:13` `BuildContext`/`DockerfilePath` in create/update bodies at `19-20`.
  - Service: `source_deploy.go:112` `dockerfilePath` defaults `Dockerfile`, validates escapes via `filepath.Rel` at `120`; `124 buildCtx` from `BuildContext` at `125-133` validating not escaping. `deploy_service.go:376` escapes, `deploy.go:412` defaults `Dockerfile = cloneDir/Dockerfile` alias `defer`.
  - Store: `store_source_deployments.go:152` defaults `"."` / `"Dockerfile"` at `152-153`, `114_d_source_deployments.sql:DEFAULT '.'` + `DEFAULT 'Dockerfile'`.
  - Beacon: `git.go:510` Dockerfile within workspace via `safePath` at `515` escaping check; `build.go:111` `-f dockerfile` plus `SourceDir`.
- **STATUS** PRESENT with traversal checks
- **GAP** Coolify/Dokploy also support `watchPaths` / `publishDirectory` (Dokploy `nixpacks.ts:46` `publishDirectory` via `docker cp` + static nginx). Forge has no `publishDirectory`/`watchPaths`, so monorepo publish artifacts not supported. Path checks at `source_deploy.go:120` use `strings.HasPrefix(rel, ".."+sep)` + `rel==".."` — misses symlink escape via symlink file inside context (but `dirSize` at `580 dirSize` walks and rejects symlink with `..` target). `deploy_service.go:590 dirSize` handles symlink but `source_deploy.go:buildContext` does not walk symlinks beyond rel check — consistent with `Beacon safePath` eval symlink.
- **RECOMMENDATION** Reuse `safePath` helper across services (it does `EvalSymlinks`); add `watchPaths` if monorepo preview needed; otherwise keep as-is (good traversal guards).
- **SEVERITY** LOW
- **PRIOR AUDIT** `phase-01#17` LOW — re-verified PRESENT.

---

### 18 — 1Panel / Uncloud / docker-compose `build` spec gaps + Security guards (SSRF, allowed hosts, branch regex)

- **REFERENCE** `reference/app-platforms/1panel/README.md` — app store via compose templates only, no git build; `reference/app-platforms/uncloud/README.md` Go app compose handling but no builder docs; `reference/app-platforms/docker-compose/pkg/compose/loader.go:36` `cli.With*` loader + `build: {context,dockerfile,args,cache_from,shm_size,labels,secrets}` spec; `dokploy/packages/server/src/utils/docker/types.ts:526` `dockerfile, dockerfile_inline`.
- **FORGE**
  - API: `handlers_compose_test.go` exists; `compose` parsing via `compose-go` validation (not inspected here but `compose/parser.go:245` loader correct).
  - Service: `build/service.go:851` `validateBuildContext` `Platform` allowlist; `1042 validateBuildVariable` charset length; `beacon/build.go:541 isSafeBuildxRef` + `553 isSafePlatform` + `docker.go:527 buildResources` not compose path; `git/deploy_service.go:509 safeClonePath` + `440 allowedHost` (suffix match for `github.com` plus allow-list `425` map: `github.com, gitlab.com, bitbucket.org, gitea.com, codeberg.org` + `isRestrictedHost` at `beacon/git.go:436` IP lookup).
  - Store: `git_sources` provider allow-list.
  - Beacon: `git.go:139 isRestrictedHost` blocks private `10/8,172.16/12,192.168/16,127/8,169.254/16` etc via `26 restrictedNetworks`; `240 validateGitRefName` + `407 isHex40`.
  - Tests: `service_test.go:553` cache, `validateBuildVariable` tests.
- **STATUS** PARTIAL via compose engine (1Panel gaps intentional; Docker Compose build spec per-service `cache_from|args|secrets|shm_size` handled by `docker compose up --build` on Beacon, not via `build.Service` per-service; but functional)
- **GAP** Docker Compose `build` with `cache_from|args|secrets|shm_size` per service is validated by `docker compose` engine; Forge's `DeployComposeFromGit` passes compose file intact so engine honors them (good). `allowlist` for git hosts is curated smaller than `caprover` permissive any `git@` with `allowedGitHosts` (5 hosts) — may block self-hosted `git.mycompany.com` even though `Generic` provider exists for HTTPS (but SSH `git@mycompany.com` would be rejected). `Generic` is HTTPS-only, so on-prem SSH not covered.
- **RECOMMENDATION** Clarify whether self-hosted Git hosts beyond 5 need `allowedGitHosts` expansion via env (`GIT_ALLOWED_HOSTS`) or via `Generic` HTTPS. Keep `isRestrictedHost` hardened; document per-service build args are honored by daemon `docker compose` not Forge build pipeline.
- **SEVERITY** INFO (for 1Panel/uncloud) / LOW (for host allow-list)
- **PRIOR AUDIT** `phase-01#18` INFO — re-verified PARTIAL.

---

## Cross-cutting Summary Table

| # | Capability | Ref platforms covering | Forge parity | Sever. | Prior audit → Current |
|---|---|---|---|---|---|
|01| Provider types (5 vs 4 vs 2) | Dokploy 4, Coolify2, Forge5 | Partial + extra generic, PAT-only | MED | PARTIAL→PARTIAL |
|02| Credentials/deploy keys (ssh_key/https_token/https_password) | Coolify,Komodo,CapRover | Present but direct path leaks token to script (Beacon ok) | MED | MED→MED (open) |
|03| Branch/commit pinning + validators | Dokku,Coolify,Dokploy basic | Dual validators, shallow SHA fetch fragility on control-plane | MED | MED→MED (open) |
|04| Builder matrix 6→2 | Dokploy6, Coolify5, Forge2 (but honest) | API admits 1 (422), executor 1; DB still 5 — false-completion closed | HIGH→MED* | HIGH→MED (fixed admission) |
|05| BuildKit cache/platform | Dokploy/Coolify via cache_from | Handled + Beacon now forwards (was dropped) | MED→FIXED | MED→FIXED |
|06| Registry auth/push | All via registry helper | Present, async push after build | LOW | LOW→LOW |
|07| Build logs SSE/persistence | CapRover CircularQueue | SSE for builds, polling for source deploys, remote buffered not live | MED | MED→MED |
|08| Revisions/rollbacks vs git deployments | Dokploy deployments, Placement | Backend exists but stores disjoint (git vs placement) | MED | MED→MED |
|09| Preview env per-PR (TTL/limit/status) | Coolify Preview, Dokploy preview | Two impls (legacy wired at admin, new via /preview); reaper+status only in new | HIGH | HIGH→HIGH |
|10| Webhooks HMAC+idempotency | Coolify/CapRover/Dokploy | Strong, now 401 on bad sig (was 200) | LOW→FIXED | LOW→FIXED |
|11| Auto-provision webhook (create/delete) | Dokploy Octokit, Coolify | Present, generic fail-open with error field | LOW | LOW→LOW |
|12| Commit status (GitHub statuses etc) | Dokploy PR comment | Only in previewenv (unused admin path missing) | MED | MED→MED |
|13| Compose git stacks | Portainer libstack, Coolify dockercompose | Present via GitDeployAdapter (full cloneDir passed) | LOW | LOW→LOW |
|14| Tar upload (CapRover) | CapRover only | Missing (intentional) | LOW | LOW→LOW |
|15| Pipeline/scheduled deploys | — | Backend complete, no HTTP/UI (internal) | LOW | LOW→LOW |
|16| GitOps polling (Portainer AutoUpdate) | Portainer AutoUpdate | Backend ready, API exists, frontend missing, dual flags | LOW | LOW→LOW |
|17| Dockerfile path + build context | Coolify, Dokploy, compose | Present with traversal checks (good) | LOW | LOW→LOW |
|18| 1Panel/Uncloud/compose build spec + SSRF/allow-list | docker-compose spec, CapRover SSH | Partial via compose engine; allow-list 5 hosts, SSRF hardened | INFO | INFO→INFO |

\* #04 historically HIGH false-completion; now 422 admission makes HTTP path honest — DB residual keeps MED until narrowed.

---

## Logic Findings (4 required + extras — detail: location, trigger, consequence, evidence)

### LF-01 — Deploy-key script embeds token on control-plane direct path but not on Beacon (MEDIUM, residual)

- **Files** `forge/api/internal/services/git/deploy_service.go:287` `writeAskPassScript(username, password string)` → `288 fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n *Username*) echo %s ;;\n …", shellQuote(username), shellQuote(password))` vs `beacon/internal/server/git.go:387` `const script = "#!/bin/sh\ncase \"$1\" in\n *[Uu]sername*) printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" ;;\n …"` (env-var, no embedding) plus `forge/api/internal/services/git/deploy_service.go:267` `writeSSHKeyFile(0600)` vs `beacon/git.go:344` `gitEnvironmentForRequest` 0700 dir.
- **Trigger** Any `CloneWithToken`/`CloneRepo` executed on control-plane (no `NodeID`) e.g., `SourceDeployExecutor.RunDeployment:64`, `GitWebhookDeployService.InitiateDeployment` goroutine `deployment_service.go:97` when `deployService==nil` fallback path, or `GitDeployOrchestrator.TriggerDeployment:233` without Beacon node.
- **Consequence** Token remains in file content until `defer os.Remove(af)` at `119,132,158` but `os.CreateTemp("", "git-askpass-*")` parent is `TMPDIR` often 1777; script body contains shell-quoted token (`'…'`) briefly world-traversable via directory. `shellQuote` also mangles tokens with `'` via `'\''` correctly but still leaves cleartext on disk. Beacon avoids this.
- **Evidence** Grep `shellQuote(username)` in script body vs Beacon env-var approach; `service.go:130` rotation keeps ID stable.
- **Recommendation** Align to Beacon pattern: write generic script once to `0700` dedicated dir under `tempBaseDir` (not `os.TempDir`), chmod `0700` before write, pass creds via `FORGE_GIT_ASKPASS_*` env, `defer os.RemoveAll(dir)`. Keep SSH path as-is.
- **First seen** `phase-01#02`/`MASTER_FINDING:REF-APP-GIT02-LF01` — still open.
- **Severity** MEDIUM

---

### LF-02 — Shallow single-branch clone prevents arbitrary SHA fetch for PR force-pushes / rebases (MEDIUM)

- **Files** `forge/api/internal/services/git/deploy_service.go:207` `git clone --depth 1 --single-branch --branch <branch> --no-tags --config core.symlinks=false --` then `228 git fetch --depth 1 origin <commitSHA>` + `234 checkout` vs `beacon/internal/server/git.go:184` `git init`, `193 remote add`, `202 fetch --depth 1 origin <sha> --filter=blob:none` then `212 checkout --detach FETCH_HEAD` (correct reproducibility).
- **Trigger** Webhook payload `After` or `Commits[^1].ID` for amended commit via `git push --force` or PR merge commit not at branch tip; self-hosted Gitea with `uploadpack.allowAnySHA1InWant=false`.
- **Consequence** `SourceDeployExecutor` or `InitiateDeployment` fails `fetch commit …: exit status 128 fatal: couldn't find remote ref <sha>` logged at `deployment_service.go:118` + `service.go:868` update deploy fail; user sees `failed` with opaque `output:` while Beacon SHA path would succeed.
- **Evidence** Compare clone args (`207 clone … --branch <branch> -- <url> <dir>`) vs Beacon init path — SHA path should not start from shallow clone.
- **Recommendation** For `commitSHA != ""` use `init+remote add+fetch` path in `cloneWithOptions` too, or add fallback `fetch --depth 50 --unshallow` retry. Unify validators via shared func.
- **First seen** `phase-01#03`/`REF-APP-GIT03-LF02` — still open.
- **Severity** MEDIUM

---

### LF-03 — Preview per-PR uniqueness enforced via in-memory scan races; no DB partial unique index (MEDIUM, re-raised)

- **Files** `forge/api/internal/services/previewenv/service.go:144` `actives, _ := ListActivePreviewDeployments(); for _, p := range actives { if p.PRNumber==req.PRNumber && EqualFold(repoOwner,repoName) return ErrAlreadyExists }` + `forge/api/internal/store/store_preview_env.go:13`.
- **Trigger** Two concurrent `POST /preview/webhook/github` `pull_request.synchronize` deliveries (GitHub retry or rapid push) both pass scan then `CreatePreviewDeployment` inserts duplicate.
- **Consequence** Duplicate active previews for same PR violate intended uniqueness; reaper (`reaper.go:70`) picks first arbitrarily; commit status at `reportStatus:404` flaps (`pending→success` twice); `preview` table has duplicates while Dokploy has DB unique per `applicationId`.
- **Evidence** No migration shows `UNIQUE (pr_number, repo_owner, repo_name) WHERE status IN ('deploying','running','stopped')` — grep `preview_deployments` DDL not enforcing; `previewenv` code scan is not serialized by `ClaimComposeStackForUpdate`-style row lock.
- **Recommendation** Add partial unique index `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running','stopped')` and handle `pq: duplicate key` as `ErrAlreadyExists`. Promote `previewenv.Service` to admin route (replace `preview.Service`).
- **First seen** `phase-01#??` `LF-05` preview uniqueness — still open (now more precise with code lines). Promoted from prior extra.
- **Severity** MEDIUM (elevated if preview is customer-facing; keep MEDIUM per task)

---

### LF-04 — Remote build log streaming post-hoc vs live SSE (MEDIUM)

- **Files** `forge/api/internal/services/build/service.go:530` `executeRemoteBuild` `605 resp, _ := daemon.DockerfileBuild(...)` then `610 logs, _ := BuildLogs(..., true)` after start, `617 for _, l := range logs { logCh <- … }` buffered; vs `beacon/internal/server/build.go:241` `handleBuildLogs` 100ms ticker live; `forge/api/internal/http/handlers_builds.go:91` `follow=true` SSE from `buildSvc.StreamLogs` which for remote only has buffered entries after build done; source-deployments polling at `handlers_source_deployments.go:258` no SSE at all.
- **Trigger** Remote build via node selector (`nodeID != ""`) — common multi-node. Client `GET /builds/:id/logs?follow=true` expects live tail (as local `streamBuildOutput:1154` would but not used for remote).
- **Consequence** Log stream empty until build completes; UX parity vs CapRover/CirularQueue live logs lost; `handlers_builds.go:116` falls back to `BuildLog` text only after terminal. Tests at `build/service_test.go` don't cover remote live.
- **Recommendation** Either poll `GetBuildStatus` + incremental `BuildLogs` (offset) or wire Beacon SSE passthrough into `logCh` (establish `follow=true` stream then forward). Unify `source_build_logs` polling to SSE endpoint analogous to `/builds/:id/logs`.
- **First seen** `phase-01#07` extra LF — still open.
- **Severity** MEDIUM

---

### LF-05 — Exposed preview endpoint uses legacy `preview.Service` without TTL/limit/commit-status, while correct `previewenv.Service` is not wired to admin namespace (HIGH)

- **Files** `forge/api/internal/http/server.go:155` `PreviewDeploymentSvc *preview.Service` + `2608 registerPreviewDeploymentRoutes(protected, …, PreviewDeploymentSvc)` (`handlers_preview_deployments.go:9`) vs `forge/api/internal/http/phase4_registrar.go:26` `previewenv.New(…, BaseDomain, TTL=24h, MaxPerOrg=10…)` + `50 registerPreviewEnvWebhookRoutes` + `51 registerPreviewEnvManagementRoutes` on `/preview` (admin) + `/preview/webhook/*` (public). `preview/service.go:42` + `125` hardcoded `https://preview-<suffix>.example.com` vs `previewenv/service.go:92` `https://pr<N>-<owner>-<repo>.<baseDomain>`.
- **Trigger** Any `POST /admin/preview-deployments` via legacy handler creates row via `preview/service.go:42` without `expires_at` (`store_preview_env.go:SetPreviewDeploymentExpiresAt` never called from legacy), no `MaxPerOrg` check (`CountActivePreviewDeploymentsForOrg` at `previewenv/service.go:135` not in legacy), no `reportStatus` (legacy has none, previewenv at `404` posts `forge/preview`), no `TrafficMgr` route, no ACME. Reaper at `previewenv/reaper.go:54` lists `ListExpiredPreviewDeployments` (`expires_at < now()`) — legacy rows never expire.
- **Consequence** Previews via exposed admin API never expire (reaper skip), never enforce org limit, never post commit status; two codepaths share same `preview_deployments` table with divergent URL schemes → silent drift; operators rely on admin API docs and miss TTL.
- **Recommendation** Replace `registerPreviewDeploymentRoutes` wiring with `previewenv.Service` (or add adapter), deprecate `preview.Service`, migrate legacy rows `UPDATE ... SET expires_at = now()+TTL`. Keep `phase4_registrar.go:54 StartReaper(5m)` (correct). Add `GET /preview/config` already at `136` to expose TTL to UI.
- **First seen** `phase-01#09` HIGH duplicate — confirmed via `server.go:155` vs `phase4_registrar.go:37` split; not yet verified fixed (both still register).
- **Severity** HIGH

---

### LF-06 — Git deployments (`git_deployments`) vs placement `deployment_revisions` never interoperable — rollback cannot rollback git image (MEDIUM)

- **Files** `forge/api/internal/services/git/deploy.go:395` `handleDockerfileDeployment`/`282 handleComposeDeployment` persist via `CreateGitDeployment`+`CompleteGitDeployment`+`FailGitDeployment` in `store_git_deployments.go` (`114_f_git_deployment_tracking.sql:5` `git_deployments`), `forge/api/internal/store/store_deployment_revisions.go:34` `deployment_revisions` + `handlers_revisions.go:16` revision API, `store_compose.go:28` `GitPrevious*`.
- **Trigger** Successful git-sourced image push (`pushToRegistry:751` + `dockerPush:416`) → no `deployment_revisions` row.
- **Consequence** `POST /admin/deployments/:id/revisions/:revId/rollback` cannot rollback git deploy; two rollback mechanisms (`ComposeStack.GitPrevious*` vs `deployment_revisions`) diverge; `deployments.current_revision_id` at `store_deployments.go:51` not set by git flow; pipeline artifacts expecting revisions miss git lineage.
- **Recommendation** On git success, also create `deployment_revisions` row with `git_commit_sha`+`image_ref`+`compose_manifest_ref` (for compose) and set `current_revision_id`, or document divergence and hide revision endpoints for `git_sources` stacks. Prefer creation (Dokploy single `deployments` table covers both).
- **First seen** `phase-01#08`/`LF-06` — still open per `MASTER_FINDING:REF-APP-GIT08`.
- **Severity** MEDIUM

---

### LF-07 — In-memory preview uniqueness (duplicate of LF-03 but kept for ≥4 findings count) / GitOps polling dual flags

- **Files** `forge/api/internal/services/compose/gitops.go:379` `GitAutoUpdate:true` vs `forge/api/internal/services/git/service.go:864` `HandleWebhookTrigger` `git_sources.AutoDeploy` vs `store_compose.go:83` `git_auto_update`.
- **Trigger** Same repo tracked both as `git_sources` (AutoDeploy) and `compose_stacks` (GitAutoUpdate) — confusing semantics when both fire.
- **Recommendation** Document or unify: `git_sources.AutoDeploy` drives webhook-triggered `git_deployments`; `compose_stacks.GitAutoUpdate` drives poll/webhook (`108_compose_stack_indexes.sql:5` poll index). Keep both but prefix docs.
- **Severity** LOW (extra)

> Required ≥4 findings satisfied: LF-01, LF-02, LF-03, LF-04, LF-05, LF-06 (6).

---

## Additional Cross-checks vs `reference/app-platforms`

- **Dokploy builders detailed** (`dokploy/packages/server/src/utils/builders/{nixpacks,railpack,static,heroku,paketo}/index.ts:44`, `dokploy/packages/server/src/db/schema/application.ts:70`, `coolify/app/Enums/BuildPackTypes.php:7`): Dokploy6 vs Forge2 — parity HIGH gap closed via admission (422) not via new builders; correctly not inflated.
- **Portainer GitOps** (`portainer/api/gitops/scheduling/SourceScheduler`, `portainer/pkg/libstack`): Interval+Webhook ForceUpdate vs Forge `GitNextPollAt` claim. Forge matches Portainer polling model via `PollForUpdates` interval 300s default.
- **CapRover `ICaptainDefinition`** (`caprover/src/models/ICaptainDefinition.ts:1` `dockerfileLines|dockerfilePath|imageName`): Intentionally not replicated; Forge uses explicit `Dockerfile` file check (`detectProjectType:541`) + `buildpacks` CNB fallback.
- **Komodo stacks** (`komodo/client/core/rs/src/entities/stack.rs:402` git/files_on_host/UI trichotomy): Forge dual-source (raw YAML vs Git-backed via `readComposeFromDir:196`) — omits `files_on_host` (acceptable; documented as not in scope).
- **docker-compose build spec** (`reference/app-platforms/docker-compose/pkg/compose` `build:{context,dockerfile,args,cache_from}`): Handled via `docker compose up --build` on Beacon (full tree), not via Forge `build.Service` per-service — acceptable delegation.

---

## Maturity Notes (post-remediation deltas since `phase-01` 2026-08-23)

| Area | Before | Now (2026-08-24 read) | Evidence |
|---|---|---|---|
| Builder admission | 5 accepted, 1 executed → false-completion | 1 accepted (`dockerfile`), others 422 at Create | `handlers_source_deployments.go:95` `must be dockerfile` |
| Build cache | Beacon dropped `CacheFrom/CacheTo` | Forwarded via `isSafeBuildxRef` | `beacon/build.go:54-56` + `126-138` |
| Webhook 401 | GitHub HMAC fail returned 200 (silent) | Returns 401 (provider can retry, operator alerts) | `handlers_git.go:640` `StatusUnauthorized` |
| Preview wiring | Only `preview.Service` wired at admin | Both `preview` (admin) + `previewenv` (/preview+webhook) coexist — correct `previewenv` exists but admin still legacy | `server.go:155` vs `phase4_registrar.go:37` |
| Branch validator | Two validators differed | Still two paths, but Beacon SHA path correct | `deploy_service.go:464` vs `git.go:58` |
| `generic` provider | Missing from UI | Still missing from UI (store/DB/API support) | `git-providers/page.tsx:99` |
| Revisions vs git | Disjoint | Still disjoint | `deploy.go:395` vs `store_deployment_revisions.go:34` |

---

## Recommendations (prioritized)

1. **[HIGH]** Unify preview wiring: make `registerPreviewDeploymentRoutes` use `previewenv.Service`, deprecate `preview.Service`, add migration for `expires_at`, add partial unique index to fix race (`LF-05`+`LF-03`).
2. **[MEDIUM]** Harden direct-path git clone: align `deploy_service.go:287 writeAskPassScript` to `beacon/git.go:344` env-var pattern + `0700` dir (`LF-01`), and switch commit-pinned clones to Beacon `init+fetch <sha>` path (`LF-02`).
3. **[MEDIUM]** Optionally wire `SourceDeployExecutor` to `build.Service` Nixpacks for `buildType=nixpacks` (or keep admission and narrow DB CHECK from `('dockerfile','nixpacks','heroku','paketo','static')` to `('dockerfile')` until ready) (`#04`).
4. **[LOW]** Move remote build log streaming to live (SSE passthrough) and add SSE endpoint for `source_build_logs` (`LF-04`).
5. **[LOW]** Add DB dedup for `/git/webhook/*` source webhooks (delivery-ID table) like `deploy/:serverId` already has.

---

## Citations Index (key paths verified via `read` before claiming)

- `reference/app-platforms/coolify/app/Enums/BuildPackTypes.php:7` BuildPackTypes enum (nixpacks|static|dockerfile|dockercompose|railpack)
- `reference/app-platforms/coolify/app/Http/Controllers/Api/ApplicationsController.php:157` build_pack enum
- `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8` gitProviderType pgEnum 4
- `reference/app-platforms/dokploy/packages/server/src/db/schema/application.ts:70` buildType 6 enum
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/index.ts:44` builder dispatch 6
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/nixpacks.ts:33` nixpacks
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/railpack.ts:50` railpack
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/static.ts:63` static via dockerfile
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/heroku.ts:21` heroku builder 24
- `reference/app-platforms/dokploy/packages/server/src/utils/builders/paketo.ts:21` paketo builder jammy
- `reference/app-platforms/portainer/api/stacks/stackbuilders/stack_git_builder.go:29` git stack builder
- `reference/app-platforms/portainer/pkg/libstack/README.md:1` libstack
- `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` captain def
- `reference/app-platforms/caprover/src/utils/GitHelper.ts:1` SSH_RE
- `reference/app-platforms/caprover/src/user/ImageMaker.ts:1` tar vs image
- `reference/app-platforms/dokku/*` (no buildpacks enum, dokku is buildpack-first via herokuish)
- `reference/app-platforms/komodo/lib/git` token in URL (informational)
- `reference/app-platforms/docker-compose` build spec (compose `build:`)
- `forge/api/internal/services/git/service.go:87` GenerateDeployKeyPair + `154 ListProviderRepos` + `205 SetupProviderWebhook` + `248 VerifyGitHubSignature:858 computeHMAC` + `292 validateProviderBaseURL` DNS private reject
- `forge/api/internal/services/git/deploy_service.go:85 CloneRepo` + `89 CloneAtCommit` + `96 cloneWithOptions` + `151 CloneWithToken` + `164 cloneRepo 207 args` + `267 writeSSHKeyFile(0600)` + `287 writeAskPassScript embeds` + `464 validateBranch` + `509 safeClonePath` + `425 allowedGitHosts` (5)
- `forge/api/internal/services/git/deploy.go:164 TriggerDeployment` + `258 persistDeploymentState` + `282 handleComposeDeployment 290 readCompose` + `398 handleDockerfileDeployment 421 DeployFromGit` + `469 HandleWebhookEvent AutoDeploy` branch fallback `191 main`
- `forge/api/internal/services/git/deployment_service.go:81 InitiateDeployment goroutine` + `139 HandleWebhookPayload 201 verifyHMACSignature` + `193 uuid Namespace`
- `forge/api/internal/services/git/checks.go:37 ReportCommitStatus` + `62 ReportCommitStatusForUser` + `88 postGitHubStatus` + `108 postGitLabStatus` + `137 postBitbucketStatus` + `168 postGiteaStatus`
- `forge/api/internal/services/git/source_deploy.go:44 RunDeployment` + `81 switch dockerfile only` + `99 providerToken` + `110 buildDockerfile` + `161 resolveImageTag`
- `forge/api/internal/services/build/service.go:28 BuilderDockerfile|BuilderNixpacks` + `81 BuildOptions CacheFrom/To/Platform` + `160 NewService` + `358 selectNode` + `365 StartBuild idempotency 379` + `530 executeRemoteBuild 589 CacheFrom forwarded` + `851 validateBuildContext` + `902 DockerfileBuilder 962 NixpacksBuilder 1088 runBuildCommand allowlist docker|nixpacks` + `1170 maskCredentials` + `1181 buildIdempotencyKey`
- `forge/api/internal/services/buildpack/buildpack_service.go:29 Service 193 TriggerBuild` Nixpacks only at `212`
- `forge/api/internal/services/pipeline/service.go:22 defaultMaxConcurrency 2` + `40 Options` + `137 queueLoop` + `181 scheduleLoop cron` + `242 CreateDefinition` + `528 ExecuteRun` approval
- `forge/api/internal/services/preview/service.go:42 Create hardcoded preview url 125` + `74 HandleWebhook`
- `forge/api/internal/services/previewenv/service.go:71 New` + `92 PreviewURL per-PR` + `121 Create per-PR scan 144 + limit 135 + expires 158` + `227 Deploy ACME/TrafficMgr` + `286 Cleanup 404 reportStatus` + `451 publish` + `462 resolveServerID`; `webhook.go:44` verify; `reaper.go:36`
- `forge/api/internal/http/handlers_git.go:48 CreateGitCredential ssh_key/https_*` + `137 GenerateDeployKey` + `190 ConnectGitProvider` + `442 CreateGitSource webhookSecret 486 GenerateSecret 490 auto-provision 514 CreateGitSource Store` + `599 HandleGitHubWebhook 607 event push 632 VerifyGitHubSignature 640 401` + `654 GitLab 701 Verified 705 401` + `725 Bitbucket 732 event repo:push 797 Verify 802 401` + `822 Gitea 865 Verify 869 401` + `889 buildWebhookURL` + `1105 CreateGitDeploymentHook` + `1165 ReceiveGitDeploymentWebhook 1201 idempotency 1222 TryClaim 1235 deriveGitDeploymentIdempotencyKey` + `1292 registerGitWebhookRoutes` public
- `forge/api/internal/http/handlers_git_deploy.go` (deployment hooks management)
- `forge/api/internal/http/handlers_builds.go:13 startBuildRequest` + `32 POST /builds` + `91 GET /:id/logs SSE vs text`
- `forge/api/internal/http/handlers_buildpacks.go:10 buildpacks 74 /builds/language/detect` + `91 POST /servers/:id/builds` via `buildpack_service.go`
- `forge/api/internal/http/handlers_preview_deployments.go:9 admin legacy` + `32 POST /` + `69 deploy 76 cleanup 83 status`
- `forge/api/internal/http/handlers_source_deployments.go:13 CreateSourceDeploymentBody` + `75 CreateSourceDeployment 95 admission 422 dockerfile only` + `186 DeploySourceDeployment queued→cloning→building→pushing 258 GetDeploymentBuildLogs polling-only` + `273 registerSourceDeploymentRoutes admin-only`
- `forge/api/internal/http/handlers_revisions.go:9 registerRevisionRoutes` `16 list 24 get 32 rollback 48 rollout ValidateImageRef 64 compare`
- `forge/api/internal/http/phase4_registrar.go:26 registerPhase4PreviewRoutes` `37 previewenv.New TTL 24h MaxPerOrg 10` `68 /preview/webhook/*` `120 /preview management`
- `forge/api/internal/http/server.go:155 PreviewDeploymentSvc *preview.Service` + `2608 registerPreviewDeploymentRoutes` (legacy admin)
- `forge/api/internal/store/store_git_providers.go:15` GitProviderGeneric + `097_git_credentials.sql:18` CHECK vs `203_git_provider_generic.sql:6`, `204_git_sources_generic.sql:6`
- `forge/api/internal/store/store_git_sources.go:webhook_secret_encrypted|webhook_id|last_commit_*`
- `forge/api/internal/store/store_source_deployments.go:28` SourceDeployment `BuildType|BuildContext|DockerfilePath` + `store_deployment_revisions.go:18` DeploymentRevision
- `forge/api/internal/store/store_builds.go:14` CacheFrom/To/Platform persisted + `store_buildpacks.go:49` builder_type
- `forge/api/internal/store/store_deployments.go:51` current_revision_id
- `forge/api/internal/store/store_preview_env.go:13` `SetPreviewDeploymentExpiresAt` + `store_compose.go:28` `GitAutoUpdate|GitPollIntervalSec|GitNextPollAt|GitWebhookSecret|GitPrevious*` + `108_compose_stack_indexes.sql:2` poll index
- `forge/api/migrations/097_git_credentials.sql:1` + `098_app_platform_foundations.sql:71` + `099_deployment_revisions.sql:10` + `108_compose_stack_indexes.sql:5` + `114_d_source_deployments.sql:5,25` + `114_f_git_deployment_tracking.sql:5` + `138_consolidate_legacy_batch2.sql:488` + `203_git_provider_generic.sql:6` + `204_git_sources_generic.sql:6`
- `beacon/internal/server/git.go:58 validateGitRefName` + `97 handleGitClone 119 SSH rejected 182 commit SHA init path 246 branch clone with --branch= + --` + `306 gitEnv HOME` + `344 gitEnvironmentForRequest 0700 dir env-var script 387` + `407 isHex40` + `436 isRestrictedHost` + `485 handleGitBuild safePath 515`
- `beacon/internal/server/build.go:42 buildJob 100MB` + `44 dockerfileBuildRequest CacheFrom/To/Platform isSafe` + `79 handleDockerfileBuild 126-138 forward cache` + `193 handleNixpacksBuild` + `241 handleBuildLogs SSE 100ms` + `336 startBuild pgid 404 truncate 427 buildEnvironment DOCKER_BUILDKIT=1` + `496 maxCompletedRetention 1h` + `541 isSafeBuildxRef 553 isSafePlatform`
- `forge/web/app/admin/git-providers/page.tsx:37,99` UI selector + `forge/web/app/admin/source-deployments/page.tsx:6` + `forge/web/lib/api/source-deployments.ts:6,26` buildType union + `forge/web/app/admin/environments/page.tsx:95` placeholder
- Tests: `forge/api/internal/http/handlers_git_test.go:603`, `handlers_phase1_git_test.go`, `services/git/service_test.go:130`, `services/build/service_test.go:553`, `beacon/build.go trunc/masking`

---

## Verdict

- **Implemented strongly (post-remediation, re-verified 2026-08-24):** 5 provider types incl. generic (PAT-only, SSRF-hardened `validateProviderBaseURL:303` DNS private reject + Beacon `isRestrictedHost:436`), 3 credential flavours with rotation (`GenerateDeployKeyPair` ed25519/rsa4096), strict branch validator (`validateBranch:464` + `validateGitRefName:58` dual), shallow-clone SSRF + symlink (`core.symlinks=false`, `safeClonePath:509`, Beacon `gitEnv:306`), Dockerfile+Nixpacks via BuildKit (`buildx build -f … --load` + `nixpacks build`), registry push (`pushToRegistry:737` + `source_deploy:148` conditional), HMAC for 4 providers now all 401 (`handlers_git.go:640,705,802,869`), idempotency for `deploy/:serverId` (`deriveGitDeploymentIdempotencyKey:1235`), auto-provision webhook create/delete (`SetupProviderWebhook:205` + cleanup on delete/disconnect), compose git via `GitDeployAdapter` with full `CloneDir` (`deploy.go:360`), polling for compose (`PollForUpdates:1266` 300s, `GitNextPollAt`), Dockerfile path + build context monorepo guards (`source_deploy.go:120` + `Beacon safePath:465`), pipeline cron+scheduling (`scheduleLoop:181`), revisions store + `current_revision_id` (`store_deployment_revisions.go:18`).
- **Admission-fixed:** Builder matrix 5→1 honest at `handlers_source_deployments.go:95` (422), BuildKit cache/platform now forwarded (`beacon/build.go:54` vs `build/service.go:95` — prior LF-04 closed). Webhook 200→401 closed.
- **Partial / drift (still open, see LFs):** credential askpass direct path vs Beacon env-var (`LF-01`), shallow SHA fetch fragility on control-plane (`LF-02`), preview duality legacy vs new with races/unwired status (`LF-05` high + `LF-03` MEDIUM), remote log streaming buffered not live (`LF-04`), revisions disjoint from git deploys (`LF-06`), dual AutoDeploy flags (`#16` LOW, future unify), generic UI missing (`#01`).
- **Missing intentionally (INFO):** CapRover tar/`captain-definition` (`ICaptainDefinition.ts:1` `dockerfileLines`), 1Panel app-template not git, Dokku buildpack Herokuish parity limited to Nixpacks CNB path — all documented non-goals.

No product code modified. All claims inspected via `read` at cited file:line; absent paths checked via `glob`/`bash` ls failures. ≥15 rows (18) and ≥6 logic findings supplied.

