# Subagent 04 — Git / Build / Preview — LIVE Confirmation (110-02-04 of 110)

**Agent:** 110-02-04 of 110 — Phase 02 Agent 04/10 (all 10 run in parallel)  
**Focus:** Confirm Git/Build/Preview implementations vs prior audits  
**Date:** 2026-08-24 (re-inspection live)  
**Reconciles:** `audits/110-phase-01-audit/subagent-02-web-core.md` (env-editor side) + `audits/final-parity/subagent-03-git-build.md` (18 rows, 6 LFs) + `audits/reverification/subagent-10-git-providers.md` (18 rows, 5 LFs) + `audits/reverification/subagent-11-build-pipeline.md` (15 rows, 4 findings)  
**Method:** Read-only. No code modified. Every claim below verified via `read` at cited `file:line`. Absent paths checked via `bash ls`. STATUS: FIXED / STILL BROKEN (with file:line proof).

---

## 1. Live Files Inspected (prompt-mandated)

| Prompt target | Live file:line verified |
|---|---|
| `deploy_service.go:207` shallow SHA | `forge/api/internal/services/git/deploy_service.go:207` `args := []string{"clone","--depth","1","--single-branch","--branch",branch,…}` + `forge/api/internal/services/git/deploy_service.go:228` `fetch --depth 1 origin <sha>` |
| `deploy_service.go:287` `writeAskPassScript` embeds `shellQuote` | `forge/api/internal/services/git/deploy_service.go:287` `func writeAskPassScript(username,password string)` at `deploy_service.go:288` `fmt.Sprintf("#!/bin/sh\n… echo %s …", shellQuote(username), shellQuote(password))` + `deploy_service.go:308` `shellQuote` + `deploy_service.go:289` `os.CreateTemp("", "git-askpass-*")` |
| `beacon/git.go:387` env-var | `beacon/internal/server/git.go:387` `const script = "#!/bin/sh\n… printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" … \"$FORGE_GIT_ASKPASS_PASSWORD\""` + `beacon/internal/server/git.go:344` `gitEnvironmentForRequest` + `git.go:362` `MkdirTemp(parentDir,"forge-git-askpass-*") 0700` + `git.go:398-401` `GIT_ASKPASS` + `FORGE_GIT_ASKPASS_*` env |
| `preview/service.go:42` vs `previewenv/service.go:121` at `phase4_registrar.go:37` | `forge/api/internal/services/preview/service.go:42` `func (s *Service) Create` (no TTL, `45 uuid[:8]`, `125 hardcoded https://preview-<suffix>.example.com`) vs `forge/api/internal/services/previewenv/service.go:121` `Create` (enforces `135 MaxPerOrg`, `144 per-PR scan`, `158 expiresAt=now+TTL`, `92 PreviewURL=https://pr<N>-<owner>-<repo>.<baseDomain>`) wired at `forge/api/internal/http/phase4_registrar.go:37` `previewenv.New(BaseDomain,TTL 24h,MaxPerOrg 5)` |
| `handlers_source_deployments.go:95` admission 422 | `forge/api/internal/http/handlers_source_deployments.go:95` `if req.BuildType != "dockerfile" return 422 "buildType must be \"dockerfile\" …"` (with `87-96` default + honesty comment) |
| `beacon/internal/server/build.go:54` CacheFrom/To forward | `beacon/internal/server/build.go:54` `CacheFrom []string json:"cacheFrom"` + `build.go:55` `CacheTo` + `build.go:56` `Platform` + `build.go:126-138` forwarding via `build.go:542 isSafeBuildxRef` + `build.go:554 isSafePlatform` |
| `build/service.go:610` buffered `BuildLogs` vs `daemon/build.go:156` `BuildLogsStream` | `forge/api/internal/services/build/service.go:610` `logs, err := s.daemonCli.BuildLogs(ctx,baseURL,token,resp.ID,true)` (post-hoc) + `service.go:617` `for _,l := range logs { logCh <- … }` vs `forge/api/internal/daemon/build.go:156` `func BuildLogsStream` (`io.ReadCloser` SSE `follow=true`) + `daemon/build.go:118` `BuildLogs` blocking; service never calls `BuildLogsStream` |
| `store_deployment_history` vs `git_deployments` disjoint | `forge/api/internal/store/store_deployment_history.go:55` `CreateDeploymentRecord` / `195 CreatePreviewDeployment` + `forge/api/internal/store/store_git_deployments.go:44` `CreateGitDeployment` / `194 CompleteGitDeployment`; `forge/api/internal/services/git/deploy.go:282` `handleComposeDeployment` + `395 handleDockerfileDeployment` + `forge/api/internal/services/git/source_deploy.go:81` only touch `git_deployments`/`source_deployments`, never `store_deployment_history` |

Additional files read for cross-check: `forge/api/internal/http/handlers_preview_deployments.go:9`, `forge/api/internal/http/server.go:155` + `2608`, `forge/api/internal/store/store_preview_env.go:17`, `beacon/internal/server/git.go:58`+`182`+`246`.

---

## 2. Verdict Summary

| GB | Prior audits (phase-01 → final → reverify 10/11) | Live status | Disposition |
|---|---|---|---|
| **GB-02/LF-01** credential askpass divergence (`deploy_service.go:287` vs `beacon/git.go:387`) | MEDIUM OPEN → MEDIUM OPEN → MEDIUM OPEN | **STILL BROKEN** | Direct path still embeds creds |
| **GB-03/LF-02** shallow single-branch + depth 1 SHA fetch (`deploy_service.go:207` then `228`) | MEDIUM OPEN → MEDIUM OPEN → MEDIUM OPEN | **STILL BROKEN** | No fallback; beacon init path not backported |
| **GB-04** builder matrix false-completion | HIGH → FIXED (admission) → FIXED | **FIXED** | `handlers_source_deployments.go:95` 422 closes HTTP path |
| **GB-05/LF-04** BuildKit CacheFrom/To dropped by beacon | MEDIUM → FIXED → FIXED | **FIXED** | `beacon/build.go:54` now forwards |
| **GB-07/F-01** remote build logs post-hoc buffered | MEDIUM OPEN → MEDIUM OPEN → MEDIUM OPEN | **STILL BROKEN** | `build/service.go:610` buffered; `daemon/build.go:156` Stream unused |
| **GB-08/F-03** `git_deployments` vs `deployment_revisions` disjoint | MEDIUM OPEN → MEDIUM OPEN → MEDIUM OPEN | **STILL BROKEN** | Git deploy never writes `deployment_revisions` |
| **GB-09/F-02** preview duality (`preview` vs `previewenv`) | HIGH → HIGH → HIGH | **STILL BROKEN (HIGH)** | Both services live on same table; exposed admin uses legacy |
| **GB-10** webhook HMAC 200→401 | LOW → FIXED → FIXED | **FIXED** | `handlers_git.go:640` 401 verified |
| Other 10 rows (registry, compose git, polling, platform, tar intentional, provider inventory, branch validators, auto-provision, commit-status partial, Dockerfile path) | LOW/PARTIAL → unchanged | **UNCHANGED** (see §3) | No regression, no new fix needed |

**Counts:** Historic 8 LFs → 3 now FIXED (GB-04 admission, GB-05 cache, GB-10 HMAC) — all reverified FIXED live. 5 remain OPEN (GB-02, GB-03, GB-07, GB-08, GB-09). Overall build pipeline parity: 2 of 5 historic HIGHs fixed, 1 HIGH (preview duality) remains.

---

## 3. Per-Finding Confirmation with File:Line Proof

### 3.1 GB-02 / LF-01 — `writeAskPassScript` embeds `shellQuote` vs Beacon env-var — STILL BROKEN

**Prior:** `phase-01#02` MEDIUM diverg., `final-parity#02` LF-01 MEDIUM open, `reverify-10#04` LF-01 MEDIUM open — direct path leaks creds via script body, beacon hardened.

**Live — direct path (BROKEN):**
- `forge/api/internal/services/git/deploy_service.go:287` `func writeAskPassScript(username,password string) (string, error)` 
- `deploy_service.go:288` `script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n    *Username*) echo %s ;;\n    *Password*) echo %s ;;\nesac\n", shellQuote(username), shellQuote(password))`
- `deploy_service.go:308` `func shellQuote(s string) string { return "'" + strings.ReplaceAll(s,"'","'\\''") + "'"` } — correctly escapes `'` but still cleartext on disk
- `deploy_service.go:289` `os.CreateTemp("", "git-askpass-*")` — shared `/tmp` (1777 traversable), then `295 Chmod 0700`, `defer os.Remove(af)` at `119,132,158`
- Call sites `deploy_service.go:115` `writeAskPassScript(token,"x-oauth-basic")`, `127` `writeAskPassScript(user,pass)`, `154` `CloneWithToken` same
- SSH path at `deploy_service.go:267` `writeSSHKeyFile` is `0600` but also `CreateTemp("", "git-ssh-key-*")` in shared tmp (minor residual vs beacon)

**Live — beacon path (HARDENED, not backported):**
- `beacon/internal/server/git.go:344` `func gitEnvironmentForRequest(req, dataDir) ([]string, func(), error)`
- `git.go:362` `os.MkdirTemp(parentDir,"forge-git-askpass-*")` with `352-356` `MkdirAll(parentDir,0700)` dedicated `0700` dir, not shared `/tmp` bare
- `git.go:377` `Chmod 0700` before write, `387` `const script = "#!/bin/sh\n… printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" … \"$FORGE_GIT_ASKPASS_PASSWORD\""` — **never embeds values**
- `git.go:398-401` `GIT_ASKPASS=scriptPath`, `FORGE_GIT_ASKPASS_USERNAME/PASSWORD` env scoped to subprocess only; `366 cleanup RemoveAll(dir)` deferred
- `git.go:306` `GIT_TERMINAL_PROMPT=0, GIT_CONFIG_NOSYSTEM=1` hardening applies to both paths

**Verdict:** **STILL BROKEN** — divergence persists. Control-plane deploys (`SourceDeployExecutor`, `GitWebhookDeployService.InitiateDeployment` goroutine when `deployService==nil` fallback, `GitDeployOrchestrator.TriggerDeployment:233` without Beacon) use weaker path. Fix requires aligning `deploy_service.go:287` to beacon `0700`-dir + env-var pattern; keep `writeSSHKeyFile:267` as-is.

---

### 3.2 GB-03 / LF-02 — Shallow single-branch clone prevents arbitrary SHA fetch — STILL BROKEN

**Prior:** MEDIUM OPEN across all three prior audits; trigger = force-push/PR rebase commit not at tip, server `uploadpack.allowAnySHA1InWant=false`.

**Live — direct path (BROKEN):**
- `forge/api/internal/services/git/deploy_service.go:164` `func cloneRepo` 
- `deploy_service.go:207` `args := []string{"clone","--depth","1","--single-branch","--branch",branch,"--no-tags","--config","core.symlinks=false"}` + `209 args append "--", repoURL, safeDir`
- `deploy_service.go:227` `if commitSHA != "" { 228 fetch --depth 1 origin <commitSHA> 234 checkout <sha> }`
- `deploy_service.go:241` `resolveCommitSHA` via `527 rev-parse HEAD`
- Service entry `deploy_service.go:85` `CloneRepo` + `89 CloneAtCommit` both funnel to `96 cloneWithOptions` sole path
- Failure logged at `deployment_service.go:118` `fetch commit …: exit 128 fatal: couldn't find remote ref <sha>`

**Live — beacon path (CORRECT, not backported):**
- `beacon/internal/server/git.go:182` commit-SHA path: `184 init` + `193 remote add origin <url>` + `202 fetch --depth 1 origin <sha> --no-tags --filter=blob:none` + `212 checkout --detach FETCH_HEAD` + `233 verify HEAD == req.CommitSHA`
- `git.go:246` branch path: `246 clone --depth 1 --single-branch --branch=<value>` with `-c filter.lfs.required=false -c protocol.file.allow=never` etc. + `--` separator `256`

**Divergent validators (not exploitable, but drift):**
- `deploy_service.go:464` `validateBranch` richer (rejects leading `-`, trailing `.`/`/`/`.lock`, `.. // /. @{`, ` \t\r\n~^:?*[`, plus `\ ' " ` $ & | ;` at `474`)
- `beacon/git.go:58` `validateGitRefName` regex `^[A-Za-z0-9._/-]+$` + `..`/`/.` at `51-73`, passed as single arg `"--branch="+value` at `249` + `--` at `256` — defense-in-depth

**Verdict:** **STILL BROKEN** — `cloneWithOptions` SHA path still starts from shallow single-branch clone. Recommendation: for `commitSHA != ""` use beacon-style `init+remote add+fetch` in `cloneWithOptions` too, or fallback `fetch --depth 50`/`--unshallow` retry. Unify validators to shared helper.

---

### 3.3 GB-04 — Builder matrix: 5 types accepted, 1 executed (false-completion) — FIXED (admission)

**Prior:** `phase-01#04` HIGH false-completion (admit `dockerfile|nixpacks|heroku|paketo|static` then executor only `dockerfile`), `final#04` VERIFIED_FIXED via admission, `reverify-11#01` FIXED.

**Live — FIXED at admission:**
- `forge/api/internal/http/handlers_source_deployments.go:87` `if req.BuildType=="" {req.BuildType="dockerfile"}` default
- `handlers_source_deployments.go:90-96` comment `Admission honesty (Phase-1 REF-APP-GIT04 / LF-03)` + `95 if req.BuildType != "dockerfile" { return 422 "buildType must be \"dockerfile\" (nixpacks, heroku, paketo, static are not yet supported…)" }` — fails closed before `RunDeployment`
- `forge/api/internal/services/git/source_deploy.go:81` `switch d.BuildType { case "dockerfile","": buildDockerfile; default: fail("build type %q is not supported…") }` at `87` consistent
- `forge/api/internal/services/build/service.go:28` `BuilderDockerfile|BuilderNixpacks` only 2 constants + `160` registers 2; beacon `beacon/build.go:79` `handleDockerfileBuild` + `193 handleNixpacksBuild` only 2 endpoints

**Residual (not re-triggerable via HTTP, but store-level):**
- `store_builds.go` / `098_app_platform_foundations.sql:71` `builder_type CHECK('dockerfile','nixpacks')` — API narrower; DB still allows `nixpacks` if `store.CreateSourceDeployment` called directly (e.g., test seeding). `114_d_source_deployments.sql:25` CHECK allows 5 values (`dockerfile|nixpacks|heroku|paketo|static`) though HTTP now blocks 4 — direct inserts could create undispatchable rows. `forge/web/lib/api/source-deployments.ts:26` type still lists 5 (`'dockerfile'|'nixpacks'|'heroku'|'paketo'|'static'`) broader than admission — UI shows disabled options.

**Verdict:** **FIXED** as contract — no HTTP client can reach the unsupported executor path. Keep admission; optionally narrow DB CHECK or wire `SourceDeployExecutor` to `build.Service(BuilderNixpacks)` / `buildpack.Service` for future `nixpacks` support.

---

### 3.4 GB-05 / LF-04 — BuildKit CacheFrom/CacheTo silently dropped by Beacon — FIXED

**Prior:** `phase-01#05` MEDIUM dropped, `final#05` VERIFIED_FIXED, `reverify-11#03` FIXED.

**Live — end-to-end forwarded:**
- `forge/api/internal/services/build/service.go:81` `BuildOptions{CacheFrom []string; CacheTo []string; Platform string; …}` 
- `build/service.go:939` `for _,cf := range opts.CacheFrom { args append "--cache-from", cf}` + `943 CacheTo` + `948 Platform` (`DockerfileBuilder.Build`)
- `build/service.go:598` `CacheFrom: opts.CacheFrom` → `daemon.DockerfileBuildRequest` at `forge/api/internal/daemon/build.go:25` `CacheFrom []string json:"cacheFrom"` + `26 CacheTo` + `27 Platform` (and `daemon/build.go:44` for nixpacks)
- `beacon/internal/server/build.go:54` `CacheFrom []string json:"cacheFrom"` + `55 CacheTo` + `56 Platform` — **previously missing, now present** (comment `Phase-1 LF-04: was silently dropped`)
- `beacon/build.go:126` `for _,from := range req.CacheFrom { if isSafeBuildxRef(from){ args append "--cache-from", from}}` + `131 CacheTo` + `136 Platform && isSafePlatform`
- Guards `beacon/build.go:542` `isSafeBuildxRef` (rejects leading `-`, `\x00\r\n;`) + `554 isSafePlatform` (rejects `\x00\r\n;/\\ `, empty segments, non-alnum) — correctly allows `type=gha,mode=max` (contains `=` but not `;`)
- `forge/api/internal/store/store_builds.go:14` persisted `CacheFrom CacheTo Platform` via `CreateBuild:42`

**Residual UI gap (non-bug):** `forge/api/internal/http/handlers_builds.go:13` `startBuildRequest{NoCache}` no `CacheFrom/To/Platform` — feature backend-only, undriven from frontend (intentional until UI exposes it).

**Verdict:** **FIXED** — `daemon/build.go:25` ↔ `beacon/build.go:54` schemas aligned, forwarding guarded.

---

### 3.5 GB-06 — Registry auth & image push — PRESENT (no regression)

**Prior:** PRESENT across audits; no LF.

**Live:** 
- `build/service.go:577` `LoginRegistry` before build (warn-only at `579`), `752` before push; `source_deploy.go:148` `if d.Registry != "" { pushing + dockerPush }`; `daemon/build.go:436` `LoginRegistry` + `beacon/build_ext.go:197` `handleRegistryLogin` `0700 docker-config-*` + `--password-stdin` (never argv); `build/service.go:737` `pushToRegistry` per-tag + `776` `digestBasedRef` at `778` + `647 InspectImageDigest`

**Verdict:** **PRESENT — no change** vs prior. Gap unchanged: `SourceDeployExecutor` skips push if `Registry==""` (node-local image, ok for single-node swarm), no `RegistryCredentialID` existence check at `handlers_source_deployments.go:122`.

---

### 3.6 GB-07 / F-01 — Build logs: remote post-hoc buffered, not live SSE — STILL BROKEN

**Prior:** `phase-01#07` MEDIUM divergence (polling works, SSE only for builds), `final#07` LF-06 post-hoc, `reverify-11#08` F-01 MEDIUM open.

**Live — contract exists but remote path post-hoc:**
- `forge/api/internal/http/handlers_builds.go:91` `GET /builds/:id/logs` — if terminal `98 text/plain BuildLog`, else SSE `text/event-stream` heartbeat 30s `111` + `122 SetBodyStreamWriter` reading `logCh` from `buildSvc.StreamLogs` — **live contract**
- `handlers_source_deployments.go:258` `GetDeploymentBuildLogs` polling JSON `ListDeploymentBuildLogs` — no SSE (UX gap)
- `build/service.go:530` `executeRemoteBuild` — `605 DockerfileBuild` then **after** success `610 BuildLogs(ctx,baseURL,token,resp.ID,true)` (blocking GET `daemon/build.go:118` that handles `text/event-stream` vs plain but is called synchronously after build start), then `617 for _,l := range logs { logCh <- BuildLogEntry{…} }` — delivers **buffered, after build**, not tailing; status polled at `627 GetBuildStatus` after logs
- `daemon/build.go:118` `BuildLogs` blocking scanner (SSE aware at `135 eventStream` + `139 data:` prefix) + `156 BuildLogsStream` `io.ReadCloser` SSE exists **but never called** from `build/service.go` — `grep BuildLogsStream` only at `daemon/build.go:156`
- Beacon `beacon/build.go:241` `handleBuildLogs` `follow=true` SSE 100ms ticker `275-306` live, `42 maxLogBufferSize 100MB` trunc `404-408` + per-line masking `398-402`; `110 build log` column + `store_builds.go:293 PruneBuildLogs`

**Consequence:** `GET /builds/:id/logs?follow=true` SSE reads channel only filled post-completion for remote builds — clients see silence then dump, not live incremental. `beacon/build.go:447 buildEnvironment={"DOCKER_BUILDKIT=1"}` fine.

**Verdict:** **STILL BROKEN** (MEDIUM, UX not data loss — logs persist to `BuildLog`). Recommendation: poll `BuildLogs` incrementally or consume `daemon.BuildLogsStream` SSE goroutine forwarding `data:` into `logCh` while polling `GetBuildStatus` ticker, keep masking + 100MB trunc.

---

### 3.7 GB-08 / F-03 — Deployment revisions vs `git_deployments` disjoint — STILL BROKEN

**Prior:** `phase-01#08` MEDIUM, `final#08` LF-08 MEDIUM, `reverify-11#10` F-03 MEDIUM open — git deployments and placement revisions share no row.

**Live — disjoint proven:**
- `forge/api/internal/store/store_deployment_history.go:8` `DeploymentRecord{Status,CommitHash,LogPath,RollbackID}` + `store_deployment_history.go:195` `CreatePreviewDeployment` (same table for previews) vs `store_git_deployments.go:11` `GitDeployment{GitSourceID,CommitSHA,Branch,Status,ImageTag}` + `44 CreateGitDeployment`, `194 CompleteGitDeployment`, `210 FailGitDeployment`
- `forge/api/internal/services/git/deploy.go:282` `handleComposeDeployment` + `395 handleDockerfileDeployment` only call `CreateGitDeployment`/`CompleteGitDeployment` — **never** `CreateDeploymentRecord` nor `CreateDeploymentRevision` (`store_deployment_revisions.go:18`); `handlers_revisions.go:16` revision API (`GET /admin/deployments/:id/revisions`, `POST rollback`) queries `deployment_revisions` disjoint
- `store_compose.go:28` `GitPreviousCommitSHA/GitPreviousManifest` duplicate rollback state for compose stacks
- `source_deploy.go:81` source-deploy path writes `source_deployments` + `source_build_logs`, not revisions

**Consequence:** Git-sourced image deploys never create `deployment_revisions` row so `POST /admin/deployments/:id/revisions/:revId/rollback` cannot rollback git image; rollback only via `GitPrevious*` (compose) — two mechanisms invisible to each other.

**Verdict:** **STILL BROKEN** (MEDIUM, documented deferred). Already flagged `MASTER_FINDING:REF-APP-GIT08-LF06` DEFERRED. Recommendation: on git deploy success also `CreateDeploymentRevision{git_commit_sha,image_ref}` via deployment service and set `current_revision_id`, or document revisions apply only to placement deployments.

---

### 3.8 GB-09 / F-02 — Preview environments duality (`preview` vs `previewenv`) — STILL BROKEN (HIGH)

**Prior:** `phase-01#09` HIGH duplicate, `final#09` HIGH duplicate, `reverify-10#12` HIGH + `12` per-PR race, `reverify-11#11` HIGH — both services sharing `preview_deployments` table.

**Live — duplicate implementations coexist:**

*Legacy (exposed):*
- `forge/api/internal/services/preview/service.go:21` `New(store,publisher)` + `42 Create` takes `serverID` + `PRNumber`, `44 suffix uuid[:8]`, no per-PR uniqueness, no TTL, no limit, hardcoded `125 PreviewURL=https://preview-<suffix>.example.com`, `74 HandleWebhook pull_request.opened/synchronize/closed` in-memory, no `reportStatus` dep, no reaper
- `forge/api/internal/http/handlers_preview_deployments.go:9` `registerPreviewDeploymentRoutes` mounts on `protected /admin/preview-deployments` at `14` via `*preview.Service` (6 routes: `GET /`, `GET /:id`, `POST /`, `POST /:id/deploy|cleanup|status`, `GET /server/:serverId`) — **the exposed admin API**
- `forge/api/internal/http/server.go:155` `PreviewDeploymentSvc *preview.Service` + `2608 registerPreviewDeploymentRoutes(…,PreviewDeploymentSvc)` — wired to HTTP

*Correct (Phase-4, not wired to exposed admin):*
- `forge/api/internal/services/previewenv/service.go:71` `New` with `Options{BaseDomain,TTL,MaxPerOrg,RetainCleaned,Logger,Publisher,GitService,AcmeService,TrafficMgr,DomainSvc,PanelURL}` defaults `60-64 BaseDomain env.example.com, TTL 24h, MaxPerOrg 10, Retain 24h`; registrar overrides `phase4_registrar.go:38-40` `PREVIEW_DOMAIN, PREVIEW_TTL 24h, PREVIEW_MAX_PER_ORG 5`
- `previewenv/service.go:92` `PreviewURL=https://pr<N>-<owner>-<repo>.<baseDomain>` via `98 sanitizeHostPart`; `121 Create` enforces `136 CountActivePreviewDeploymentsForOrg >= MaxPerOrg → ErrOrgLimitReached` + `144 per-PR scan ListActivePreviewDeployments → ErrAlreadyExists`, sets `158 expires=now+TTL`, `183 SetPreviewDeploymentExpiresAt`, `404 reportStatus` posts `forge/preview` commit status, `227 Deploy` ensures ACME+TrafficMgr+wildcard at `242-260`, `286 Cleanup` withdraws route, `reaper.go:36`+`70-78`
- `forge/api/internal/http/phase4_registrar.go:37` `previewenv.New` via `registerPhase4PreviewRoutes` + `50 registerPreviewEnvWebhookRoutes` on `v1 /preview/webhook/{github,gitlab,bitbucket,gitea}` at `68-71` + `51 registerPreviewEnvManagementRoutes` on `protected /preview` at `126-189` (list with expiry at `128`, config at `136`, per-server list `145`, detail `154`, deploy/cleanup/destroy `166/174/182`) + `54 StartReaper(5m)`
- `forge/api/internal/store/store_preview_env.go:13` `SetPreviewDeploymentExpiresAt`, `CountActivePreviewDeploymentsForOrg`, `66 ListExpiredPreviewDeployments` (`expires_at IS NOT NULL AND expires_at < now()`), `96 ListReapablePreviewDeployments`

**Shared table divergent semantics:**
- Both write same `preview_deployments` table (`store_deployment_history.go:195` `CreatePreviewDeployment` shared)
- Legacy rows have `expires_at IS NULL` so `store_preview_env.go:75` `ListExpiredPreviewDeployments` never finds them → **never reaped**
- Legacy `PreviewURL` `https://preview-<suffix>.example.com` vs `previewenv` canonical `https://pr<N>-…` — silent drift
- `previewenv/reportStatus` uses `forge/preview` context at `404` + `PanelURL` target; legacy never reports

**Verdict:** **STILL BROKEN — HIGH** (unchanged from final-parity). Reaper live at `phase4_registrar.go:54` but only reaps `previewenv` rows. Recommendation: switch `registerPreviewDeploymentRoutes` to `previewenv.Service` (or unify), deprecate `preview.Service`, migrate legacy rows `UPDATE preview_deployments SET expires_at = created_at + '24h' WHERE expires_at IS NULL`, add partial unique index (see 3.9).

---

### 3.9 GB-09 companion — Per-PR uniqueness race (no DB index) — STILL BROKEN

**Live:** `previewenv/service.go:144` `ListActivePreviewDeployments` scan + `149 if PrNumber==req.PrNumber && EqualFold(owner/repo) → ErrAlreadyExists` non-atomic; `store_preview_env.go` no DDL shows `UNIQUE`; `previewenv/webhook.go:60`+`112 samePreviewUnit` same pattern. Two concurrent `POST /preview/webhook/github pull_request.synchronize` both pass scan then insert duplicate active rows.

**Verdict:** **STILL BROKEN** (MEDIUM). Requires `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running')` and `pq: duplicate key → ErrAlreadyExists` handling. Also legacy `preview/service.go:42` has zero uniqueness.

---

### 3.10 GB-10 — Webhooks HMAC + idempotency — FIXED (was PR semi-open; now 401)

**Prior:** `final#10` previously masked 200 on bad sig → now `401`.

**Live:**
- `forge/api/internal/http/handlers_git.go:632` `if source.WebhookSecret=="" || Verify… != nil` then `640 return 401 invalid signature` (was 200 before phase-01 LF-08, now fixed)
- `handlers_git.go:705 GitLab 401`, `802 Bitbucket 401`, `869 Gitea 401`; `service.go:248 VerifyGitHubSignature` + `858 computeHMAC`
- `handlers_git.go:1165 ReceiveGitDeploymentWebhook` `201 verifyHMACSignature → 211 VerifyGitHubSignature`, idempotency `1200 deriveGitDeploymentIdempotencyKey` prefers `X-GitHub-Delivery`/`X-Gitlab-Event-UUID`/`X-Gitea-Delivery` fallback `1255 sha256(server:body)[:16]`, `1202 ExistsWebhookDeliveryByIdempotencyKey` + `1221 TryClaimIdempotencyKey`
- Two namespaces remain: `/git/webhook/{github,gitlab,bitbucket,gitea}` vs `/git/webhook/deploy/:serverId` vs `/preview/webhook/*` — docs unclear but HMAC per header consistent

**Verdict:** **FIXED** — all 4 providers verified `401` (live read: `handlers_git.go:640,705,802,869`). Residual: `HandleGitHubWebhook` et al for git sources have no `TryClaimIdempotencyKey` dedup (only `deploy/:serverId` path deduped) — LOW gap remains but HMAC masking closed.

---

### 3.11 Remaining GB rows (no disposition change vs final-parity)

| GB | Live evidence | Status |
|---|---|---|
| **GB-01** Provider inventory (5 vs 4 vs 2) | `store_git_providers.go:16 Generic="generic"`, `203_git_provider_generic.sql:6` CHECK 5, `handlers_git.go:190 ConnectGitProvider`, `web/git-providers/page.tsx:37` selector omits generic (frontend shows 4) — superset PAT-only `handlers_git.go:208` raw `accessToken` | **PARTIAL (intentional)** — unchanged |
| **GB-11** Auto-provision webhooks | `handlers_git.go:490 CreateGitSource → 497 SetupProviderWebhook`, `service.go:205 dispatch` 4 providers, `225 generic error` fail-open `536 webhookSetupError` + `webhook_id=""` | **PRESENT** — unchanged |
| **GB-12** Commit status | `checks.go:37 ReportCommitStatus` + `62 ReportCommitStatusForUser`, `previewenv/service.go:404 reportStatus forge/preview` (previewenv only); legacy `preview/service.go` never calls status | **PARTIAL** — previewenv only |
| **GB-13** Compose git stacks | `deploy.go:290 readComposeFromDir` + `316 ValidateComposeContent` then `354 DeployComposeFromGit` passing `CloneDir` whole tree so `build.context` resolves; `store_compose.go:28 GitPrevious*` | **PRESENT** |
| **GB-14** CapRover tar | No tar endpoint by design; `build/service.go:521 docker buildx --load` directory only | **MISSING intentional** |
| **GB-15** Pipeline strategies | `pipeline/service.go:137 queueLoop concurrency 2`, `181 scheduleLoop` cron; no HTTP registered under `server.go` | **BACKEND ONLY** |
| **GB-16** GitOps polling | `compose/gitops.go:1266 PollForUpdates` + `1295 nextPoll`, `982 SetAutoUpdate`, `379 default 300s`; dual flags `git_sources.AutoDeploy` vs `compose_stacks.GitAutoUpdate` | **PARTIAL** — backend ready, UI missing |
| **GB-17** Dockerfile path / build context monorepo | `source_deploy.go:112 dockerfilePath defaults Dockerfile + 120 Rel escape`, `124 buildCtx 131 Rel`; `beacon/build.go:111 -f dockerfile` + `90 safePath` EvalSymlinks | **PRESENT with traversal checks** — `source_deploy` Rel vs beacon `safePath` symlink gap LOW |
| **GB-18** SSRF/branch regex | `deploy_service.go:440 allowedHost` allow-list 5 hosts + `485 ValidateRepoURL` + `464 validateBranch` rich vs `beacon/git.go:58 validateGitRefName` regex + `436 isRestrictedHost LookupIP` | **PRESENT (hardened)** |

---

## 4. What Phase-01 Subagent-02 Said vs Live Now

`110-phase-01-audit/subagent-02-web-core.md` (Forge Web Core) is web-shell scope, not git/build, but its `§16` UX findings flagged `AdminShell` 60 pages, env-editor triple duplication (`components/environment/*`), dual pollers same endpoint — **none directly git/build**. The only intersecting finding is §14 grep `generic` UI omission still true per `git-providers/page.tsx:37`. No git/build false-completion was in 02 — that was `final-parity/subagent-03`. Therefore no contradiction between 02 and live for GB rows; 03's LFs remain authoritative.

**Reconciliation of numbers:** Prior claim `BuildLogs buffered` vs `BuildLogsStream` was already in `final#07` and `reverify-11#08`; live confirms `build/service.go:610` buffered and `daemon/build.go:156` Stream exists but unused — not fixed since final-parity, still accurate.

---

## 5. Fix Tracker (actionable)

| LF | Severity | Fix applied | Still needed |
|---|---|---|---|
| **LF-01** askpass divergence `deploy_service.go:287` | MEDIUM | Beacon hardened `git.go:344-405` | Backport to `deploy_service.go:287`: create `0700` dedicated dir under `tempBaseDir` (not bare `os.TempDir`), `Chmod 0700` before write, generic script reading `FORGE_GIT_ASKPASS_*`, pass creds via env |
| **LF-02** shallow SHA `deploy_service.go:207` | MEDIUM | — | For `commitSHA != ""` use `init+remote add+fetch <sha>` path (skip single-branch clone), or `fetch --depth 50`/`--unshallow` fallback; unify `validateBranch` |
| **LF-03** preview race | MEDIUM | — | `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running')`, handle duplicate as `ErrAlreadyExists` |
| **LF-04** remote logs post-hoc `build/service.go:610` | MEDIUM | — | Consume `daemon.BuildLogsStream:156` SSE + `GetBuildStatus` ticker forwarding into `logCh` live |
| **LF-05** preview duality HIGH | HIGH | `phase4_registrar.go:37` correct service exists | Replace `server.go:2608 registerPreviewDeploymentRoutes` wiring with `previewenv.Service`, deprecate `preview.Service`, migrate `expires_at IS NULL` rows |

**Already FIXED, no action:** `handlers_source_deployments.go:95` 422 admission, `beacon/build.go:54` CacheFrom/To, `handlers_git.go:640` HMAC 401 — verified closed.

---

## 6. Citations Index

- `forge/api/internal/services/git/deploy_service.go:207` `clone --depth 1 --single-branch --branch` + `228 fetch --depth 1 origin <sha>` + `287 writeAskPassScript embeds shellQuote` + `308 shellQuote` + `267 writeSSHKeyFile 0600` + `464 validateBranch` + `509 safeClonePath` + `440 allowedGitHosts(5)` + `485 ValidateRepoURL`
- `beacon/internal/server/git.go:58 validateGitRefName ^[A-Za-z0-9._/-]+$` + `182 commit SHA init 202 fetch --filter=blob:none 212 checkout FETCH_HEAD 233 verify` + `246 branch clone --branch=<value> --` + `344 gitEnvironmentForRequest 362 MkdirTemp 0700 387 env-var script` + `407 isHex40` + `436 isRestrictedHost` + `464 safePath`
- `forge/api/internal/services/preview/service.go:42 Create:44 uuid[:8] 125 hardcoded preview url` vs `previewenv/service.go:71 New Options 92 PreviewURL per-PR 121 Create 135 limit 144 scan 158 expires 404 reportStatus` + `phase4_registrar.go:37 previewenv.New TTL 24h MaxPerOrg 5 50-51 webhooks+management 54 StartReaper(5m)`
- `forge/api/internal/http/handlers_source_deployments.go:95` admission 422 + `handlers_preview_deployments.go:9 registerPreviewDeploymentRoutes preview.Service /admin/preview-deployments` vs `phase4_registrar.go:67` + `server.go:155 PreviewDeploymentSvc *preview.Service 2608 registerPreviewDeploymentRoutes`
- `beacon/internal/server/build.go:54 CacheFrom/To/Platform 126 forward 542 isSafeBuildxRef 554 isSafePlatform 447 DOCKER_BUILDKIT=1` + `forge/api/internal/daemon/build.go:25 CacheFrom 118 BuildLogs 156 BuildLogsStream` + `build/service.go:81 BuildOptions 589 forward 598 forwarded 610 BuildLogs buffered 617 loop 939 cache appends 1088 runBuildCommand`
- `store_deployment_history.go:8 DeploymentRecord 55 CreateDeploymentRecord 195 CreatePreviewDeployment` vs `store_git_deployments.go:11 GitDeployment 44 CreateGitDeployment 194 CompleteGitDeployment` + `git/deploy.go:282 handleComposeDeployment 395 handleDockerfileDeployment only git_deployments` + `store_deployment_revisions.go:18` disjoint
- `handlers_git.go:632 VerifyGitHubSignature 640 401` + `service.go:248 VerifyGitHubSignature 858 computeHMAC 273 Bitbucket 320 Gitea`
