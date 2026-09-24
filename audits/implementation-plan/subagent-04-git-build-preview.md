# Subagent 04 — Git / Build / Preview / Webhooks Pipeline — Implementation Plan

**Scope:** Git cloning & credential handling, Build matrix & remote builds, Preview environments (legacy vs Phase-4), Webhooks & HMAC, Deployment history/rollback coherence.  
**Date:** 2026-08-24  
**Master index refs:** `REF-APP-GIT01` … `REF-APP-GIT10/LF01-LF08`, `FORGE-LOGIC-001/002`, `audits/MASTER_FINDING_INDEX.md:20-29`  
**Status of prior fix:** Admission now returns `422` for non-dockerfile (`forge/api/internal/http/handlers_source_deployments.go:90-97` FIXED), but executor still only implements `dockerfile` — honesty contract held, capability not yet expanded. Webhook HMAC now `401` (`forge/api/internal/http/handlers_git.go:632-641`, `700-705`, `798-802` FIXED). Remainder DEFERRED_WITH_REASON.

---

## 1. Current-State Inventory (file:line-verified)

### 1.1 Git clone paths — two implementations

| Path | File | Lines | Behaviour |
|------|------|-------|-----------|
| **Forge direct (panel-local)** | `forge/api/internal/services/git/deploy_service.go:207-239` | `clone --depth 1 --single-branch --branch <branch> --no-tags --config core.symlinks=false` then if `commitSHA != ""` → `git -C <dir> fetch --depth 1 origin <sha>` → `checkout <sha>` | Fails when `uploadPack.allowAnySHA1InWant=false` or SHA not on branch tip; shallow single-branch fetch by SHA is not guaranteed. `checkGitBinary:433`, `validateBranch:464` permissive, `writeSSHKeyFile:267` (0600) + `writeAskPassScript:287` (0700) embeds credentials via `shellQuote:308` into script body; `knownHosts` required at `deploy_service.go:214-218`. |
| **Beacon remote (daemon-delegated)** | `beacon/internal/server/git.go:182-237` | `git -C <dir> init` → `remote add origin <url>` → `fetch --depth 1 origin <sha> --no-tags --filter=blob:none` → `checkout --detach FETCH_HEAD` → `rev-parse HEAD` verify `actualSHA != req.CommitSHA` → 500 | Correct SHA-pinned flow. Uses `gitEnvironmentForRequest:344-405` — creates per-request `0700` dir under `dataDir` (or `os.TempDir` fallback), `0700` script `forge-git-askpass-*.sh:373-397` that reads `FORGE_GIT_ASKPASS_USERNAME/PASSWORD` env vars — never embeds credential values in script body. Hardened `gitEnv:306-315` (`GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_LFS_SKIP_SMUDGE=1`). Validates branch via `validateGitRefName:58-75` + `--branch=<value>` single-token `git clone:246-257` with `--` separator and `-c protocol.file.allow=never`. |
| **Build-service remote clone** | `forge/api/internal/services/build/service.go:546-571` | delegates to `daemon.Client.GitClone → beacon handleGitClone` | Benefits from beacon fix when `BuilderType` path uses `executeRemoteBuild`. The `SourceDeployExecutor` path (`forge/api/internal/services/git/source_deploy.go:44-97`) uses local `GitDeployService.CloneWithToken` — therefore inherits the fragile path. |

**Validator divergence:**
- `deploy_service.go:464 validateBranch` — allows `_`/`-` hybrid, rejects `~^:?*[\` etc. but does not use allowlist regex.
- `beacon/internal/server/git.go:51 gitRefNamePattern = ^[A-Za-z0-9._/-]+$` + `validateGitRefName:58` — stricter allowlist, 256 char cap, `..`/`/.` rejection.
- `forge/api/internal/services/git/service.go:244 ErrInvalidBranch` shared constant but two validators.
- `ValidateRepoURL:485` vs `validateSSHRepoURL:312` vs beacon's `handleGitClone:109-122` SSRF check (`isRestrictedHost:436` with `restrictedNetworks:26`) — three places.

### 1.2 Preview duality — HIGH

| Implementation | File | Wired where | Characteristics |
|----------------|------|-------------|-----------------|
| **Legacy** | `forge/api/internal/services/preview/service.go:16-226` — `Service{store, publisher}`; `Create:42` generates `suffix=uuid[:8]`, no TTL, no MaxPerOrg, no expiry; `Deploy:125` hardcodes `https://preview-<suffix>.example.com` + `https://deploy-<suffix>.example.com`; `HandleWebhook:169` only handles `pull_request.opened/synchronize/closed`, no branch-push, no per-PR uniqueness enforcement beyond app-level check (none) | `forge/api/internal/http/handlers_preview_deployments.go:9` → `protected.Group("/admin/preview-deployments":14)` with `adminIPAccess` + `requireAdminScope`; registered at `forge/api/internal/http/server.go:2608` `registerPreviewDeploymentRoutes` | No TLS, no wildcard cert, no commit-status, no reaper, no wildcard domain. Admin namespace only. |
| **Phase-4 (correct)** | `forge/api/internal/services/previewenv/service.go:1-474` — `Options{BaseDomain, TTL, MaxPerOrg, RetainCleaned, Logger, Publisher, GitService, AcmeService, TrafficMgr, DomainSvc, PanelURL}`; defaults `env.example.com:60`, `TTL 24h:61`, `MaxPerOrg 10:62`; `Create:121` enforces `CountActivePreviewDeploymentsForOrg:136` (> MaxPerOrg → `ErrOrgLimitReached`) and `ListActivePreviewDeployments` scan for `(pr_number, repo_owner, repo_name)` `ErrAlreadyExists:144-155`; sets `expires_at:158-184` via `store.SetPreviewDeploymentExpiresAt`; `PreviewURL:92` builds `pr<Number>-<owner>-<repo>.<BaseDomain>` sanitized; `Deploy:227` ensures ACME cert (`ensureBaseCertificate:340`), traffic route (`ensureRoutingRule:366` → `preview-<id>`), wildcard domain (`ensureWildcardDomain:392`), commit-status `forge/preview:404-429`; reaper via `StartReaper` (phase4_registrar.go:54) | `forge/api/internal/http/phase4_registrar.go:16-57` — init priority 400 `RegisterPhaseRegistrar("phase4-preview-environments", 400, …:16)`; `registerPhase4PreviewRoutes:26` builds `previewenv.New` from `PREVIEW_DOMAIN/TTL/MAX_PER_ORG` env; mounts `v1 POST /preview/webhook/{github,gitlab,bitbucket,gitea}:67-72` (public, signature-authenticated, fail-open 200) + `protected /preview/*:120-190` (admin, `requireAdminScope deployemets.*`), **not** `/admin/*` | Backed by same table `preview_deployments` (`forge/api/migrations/119_deployments_rollbacks.sql:27`, `forge/api/migrations/180_preview_ttl.sql:5` adds `expires_at` nullable) |

**Store layer:**
- `forge/api/internal/store/store_deployment_history.go:195-302` — `CreatePreviewDeployment:195`, `Get/List/Update:203-274`, `ListActivePreviewDeployments:281` uses `WHERE status NOT IN ('cleaned_up','failed')` (includes `deploying|running|stopped` — but `stopped` not in registrar's reaper filter).
- `forge/api/internal/store/store_preview_env.go:10-162` — `SetPreviewDeploymentExpiresAt:17`, `ListPreviewDeploymentsWithExpiry:24`, `CountActivePreviewDeploymentsForOrg:54` uses `status IN ('deploying','running','stopped')`, `ListExpiredPreviewDeployments:66` (`IN ('deploying','running') AND expires_at < $1`), `ListReapablePreviewDeployments:96`.
- **No DB constraint** prevents two concurrent `deploying` rows for same PR. `previewenv/service.go:144` scans in Go — race under concurrent webhooks.

**Migration gap:** No partial unique index `WHERE status IN ('deploying','running')`.

### 1.3 Build matrix

| Layer | Files | Matrix |
|-------|-------|--------|
| **DB** | `forge/api/migrations/114_d_source_deployments.sql:25` `build_type IN ('dockerfile','nixpacks','heroku','paketo','static')` — 5 values; `forge/api/internal/store/migrations/041_buildpack_support.sql:6` `herokuish/cnb/nixpacks/railpack`; `forge/api/migrations/098_app_platform_foundations.sql:71` `dockerfile/nixpacks` | 5 accepted at DB level |
| **Admission (FIXED)** | `forge/api/internal/http/handlers_source_deployments.go:90-97` `if req.BuildType != "dockerfile" → 422` | Honest; prevents false completion |
| **Executor** | `forge/api/internal/services/git/source_deploy.go:81-87` `switch d.BuildType { case "dockerfile","": buildDockerfile } default: fail("not supported")` | Only 1 executes |
| **Build service (generic)** | `forge/api/internal/services/build/service.go:28-31` `BuilderDockerfile="dockerfile"`, `BuilderNixpacks="nixpacks"`; `builders map:160-162` registers only those two; `StartBuild:369` returns `unknown builder type` for others; `executeRemoteBuild:584-735` dispatches `BuilderDockerfile` via `daemon.DockerfileBuild` and `BuilderNixpacks` via `daemon.NixpacksBuild`; `NixpacksBuilder.Detect:963` + `Build:982` local path exists (runs `nixpacks build` directly). `Forgefile` validators at `forge/api/internal/services/forgefile/service.go:34 BuilderNixpacks="nixpacks"` and `144 knownBuilders dockerfile/nixpacks/heroku/static` — partially overlaps. | Generic pipeline can do dockerfile+nixpacks; source-deployments not wired |
| **Beacon** | `beacon/Dockerfile:49-55` bundles `nixpacks`; `beacon/internal/server/capabilities.go:64 NixpacksEnabled`, `111-115` detects `nixpacks` binary; `beacon/internal/server/build.go:64 nixpacksBuildRequest`, `193 handleNixpacksBuild`, `79 handleDockerfileBuild` (now honours `CacheFrom/CacheTo/Platform:44-56,126-138` after LF-04 fix); `beacon/internal/server/server.go:430` mounts both `/build/dockerfile` and `/build/nixpacks` | Both builders available at edge |
| **UI** | `forge/web/lib/api/source-deployments.ts:25 buildType: 'dockerfile'|'nixpacks'|'heroku'|'paketo'|'static'` + `forge/web/app/admin/source-deployments/page.tsx:176` select shows nixpacks/paketo; `forge/api/migrations/138_consolidate_legacy_batch2.sql:488` | UI offers what DB allows |
| **Gap** | `REF-APP-GIT04/LF03` — 5 admitted, 1 executed (now admission masks but executor unchanged). No `herokuish/paketo` dispatch, no `railpack` support, no `static` path. `CacheFrom/CacheTo/Platform` was previously dropped (`REF-APP-GIT05/LF04` fixed at beacon `build.go:54-56` + daemon `build.go:25-27` + `forge/api/internal/services/build/service.go:93-97` but `SourceDeployExecutor` still calls `dockerBuild` directly (`source_deploy.go:144`) without those fields — bypasses beacon cache/platform UI. |
| **Feature flag** | None yet — need `PREVIEW_BUILDER_*` / `BUILD_ENABLE_NIXPACKS` env gate before admitting new types. |

### 1.4 Logs — post-hoc buffered vs live SSE

| Path | File | Behaviour |
|------|------|-----------|
| **Beacon** | `beacon/internal/server/build.go:241 handleBuildLogs` | If `job.isTerminal() \|\| !follow` → `text/plain` full `logBuf.Bytes()`; else `text/event-stream` streaming: 100ms ticker, `data:` frames, `event: done` on terminal, `Flusher`. Also supports plain `GET /build/logs?id=&follow=true`. |
| **Daemon client** | `forge/api/internal/daemon/build.go:118 BuildLogs(ctx, baseURL, token, buildID, follow bool)` — buffered: `GET /build/logs?id=&follow=true/false`, scans response, returns `[]BuildLogLine` fully buffered; `156 BuildLogsStream(ctx, …, follow=true)` — returns `io.ReadCloser` SSE stream (not yet used by panel). | Panel can pass `follow=true` to stream, but current call sites use `follow` as bulk fetch. |
| **Build service** | `forge/api/internal/services/build/service.go:609-624` (dockerfile) + `685-699` (nixpacks) | Calls `s.daemonCli.BuildLogs(ctx, baseURL, token, resp.ID, true)` **buffered** — blocks until build finishes server-side (or returns all buffered logs at that moment), then `BuildLogLine` → `logBuf` + optional `logCh`. Afterwards polls `GetBuildStatus:626`. No live tail to UI until build completes. |
| **Panel HTTP** | `forge/api/internal/http/handlers_builds.go:91 GET /builds/:id/logs` | If `IsTerminal:98` → returns stored `BuildLog` text/plain; else if `follow != "true":107` returns record; else SSE `text/event-stream` streaming from `buildSvc.StreamLogs:116` (which is the **in-memory `logStream` map `build/service.go:147`** fed only via `logCh` inside `executeRemoteBuild` — which itself is populated from the buffered `BuildLogs` **after** completion → never streams live for remote builds). |
| **Source deployments logs** | `forge/api/internal/http/handlers_source_deployments.go:258 GetDeploymentBuildLogs` | `store.ListDeploymentBuildLogs:309` from `source_build_logs` table — append-only stages (`cloning`, `building`, `pushing`, `completed`, `failed`). Written via `store.CreateDeploymentBuildLog` in `source_deploy.go:54`, `142,149,157`, `handlers_source_deployments.go:211,253`. No `follow` param; no streaming; polling required. `store_source_deployments.go:309` selects ordered `ASC`. UI at `forge/web/lib/api/source-deployments.ts:146 getDeploymentBuildLogs` polls `GET /source-deployments/:id/logs`. | Post-hoc, fine for short builds, poor for 30-min builds. |
| **Observed fix state** | `REF-APP-GIT07/LF05 DEFERRED_WITH_REASON` | Correct — still buffered. |

### 1.5 Webhook namespaces & HMAC

| Namespace | File | Auth | Route |
|-----------|------|------|-------|
| **Provider hooks (generic, push)** | `forge/api/internal/http/handlers_git.go:12-15 canonical comment`, `1292-1296 registerGitWebhookRoutes`, `599 HandleGitHubWebhook`, `654 HandleGitLabWebhook`, `725 HandleBitbucketWebhook`, `822 HandleGiteaWebhook` | Delegates to `git.VerifyGitHubSignature:248`, `VerifyGitLabSignature:263`, `VerifyBitbucketSignature:273`, `VerifyGiteaSignature:320` (canonical in `service.go`). Now returns `401` on bad signature (`Was 200 mask, REF-APP-GIT10/LF08 FIXED`). Looks up source via `FindGitSourceByRepoAndBranch` → `HandleWebhookTrigger:864` (`UpdateGitSourceDeploy` if `AutoDeploy`). | `POST /api/v1/git/webhook/github`, `/gitlab`, `/bitbucket`, `/gitea` (public, no session) |
| **Server-scoped deploy hook** | `forge/api/internal/http/handlers_git.go:1165 ReceiveGitDeploymentWebhook` | Header `X-Hub-Signature-256` or `X-Hub-Signature`, loads `GetGitSourceByServerIDUnmasked:1191` (needs `WebhookSecret`), verifies via `GitWebhookDeployService.HandleWebhookPayload:1209` (delegated HMAC). Idempotency via `deriveGitDeploymentIdempotencyKey:1235` (`X-Idempotency-Key` / `X-GitHub-Delivery` / `X-Gitlab-Event-UUID` / `X-Gitea-Delivery` / hash of `server+body`) + `TryClaimIdempotencyKey:1221`. On fail → `401`. | `POST /api/v1/git/webhook/deploy/:serverId` (public) — distinct namespace. |
| **Preview env hooks** | `forge/api/internal/http/phase4_registrar.go:67 registerPreviewEnvWebhookRoutes` → `v1 POST /preview/webhook/{github,gitlab,bitbucket,gitea}` → `previewenv/webhook.go` handlers (`HandleGitHubWebhook` et al. — see `previewenv/service.go` webhook files, not separately routed server.go). They call `resolveSource:31` which re-uses `git.Verify*` helpers via `verify` closure, fail-open `200` on mismatch (intentional — `phase4_registrar.go:74-82` logs warn, still 200 to avoid provider retries). | `POST /api/v1/preview/webhook/*` — third namespace, overlapping providers. |
| **Compose webhooks** | `forge/api/internal/http/handlers_compose.go:78` | Mirrors `/git/webhook/*` pattern, separate path `/compose/webhook/*` | Fourth namespace — must not collide. |
| **Confusion risk** | `handlers_git.go:1-15` comments already warn: *"registerGitWebhookRoutes is the single public /git/webhook/* registrar; previewenv and compose use their own /preview/webhook/* and /compose/webhook/* namespaces"* — correct, but not documented externally, no OpenAPI grouping, and legacy Forge docs (reference/clustering) conflate them. `buildWebhookURL:889-898` builds `/api/v1/git/webhook/<provider>` only — preview webhooks never auto-registered via `SetupProviderWebhook` (only git sources). Operator must manually point preview webhooks separately. | Needs docs + UI hint. |

### 1.6 Deployment history — disjoint rollback domains

| Table | File | Purpose |
|-------|------|---------|
| `git_deployments` | `forge/api/migrations/114_f_git_deployment_tracking.sql:5`, `forge/api/internal/store/store_git_deployments.go:1` | Per `git_sources` history: `id, git_source_id, commit_sha, branch, status (pending/completed/failed…), image_tag, build_log, deploy_log, started_at, completed_at`. Managed by `CreateGitDeployment:44`, `UpdateGitDeployment:162`, `Complete/Fail:193-223`. Exposed at `handlers_git.go:938-1037` (`/git/servers/:id/deployments`). No linkage to `deployment_revisions`. |
| `deployment_revisions` | `forge/api/migrations/099_deployment_revisions.sql:4`, `forge/api/internal/store/store_deployment_revisions.go:1` | Per `deployments` rollout history: `id, deployment_id, revision_number (unique per deployment:21), image_ref, compose_manifest_ref, git_commit_sha, config_hash, status (pending/active/superseded/failed:10-16), deployed_at`. Supports rollback via `GetPreviousDeploymentRevision:92`, `SupersedeDeploymentRevisions:143`, `UpdateDeploymentCurrentRevision:152`. Backed by `deployments` table (`rollout_strategy:161`). |
| `deployment_history` + `rollbacks` | `forge/api/migrations/119_deployments_rollbacks.sql:1-25` | Higher-level audit (`deployment_history` with `revision_id, release_id FK`), `rollbacks` per `deployment_history`. |
| `source_deployments` + `source_build_logs` | `forge/api/migrations/114_d_source_deployments.sql:19`, `forge/api/internal/store/store_source_deployments.go:1` | Admin “Source Deployments” — isolated lifecycle (`pending/queued/cloning/building/pushing/completed/failed/canceled`) with `BuildLog[]`. Executed by `SourceDeployExecutor` locally, not via `build.Service`. Not linked to `deployment_revisions` either. |
| **Disjoint** | `REF-APP-GIT08/LF06` | A git push can produce a `git_deployments` row *and* a `deployment_revisions` row via separate code paths (`git/deploy.go` orchestrator vs `deployment` service) with no FK. Rollback of `deployment_revisions` does not know which `git_deployments.commit_sha` it corresponds to, and vice versa. `source_deployments` is third island. `preview_deployments` reuses `deployment_history` table namespace but not FK to revisions. |

---

## 2. Target Architecture

### 2.1 Principles

1. **Single source of truth per concern** — one clone routine, one HMAC verifier, one preview service, one build dispatch. Additive migrations, no table drops this phase.
2. **Honest admission until wired** — keep `422` gate for unsupported `build_type` until executor + beacon + UI all support it. Gate controlled by feature flag, not ad-hoc `if`.
3. **DB-enforced invariants** — partial unique indexes for per-PR uniqueness, not Go scans; idempotency via `webhook_deliveries`/`TRY_CLAIM` pattern already used elsewhere.
4. **Env-var askpass everywhere** — no credential interpolation into script bodies; `0700` dedicated dir + `0600` file per beacon pattern.
5. **Streaming-first logs** — SSE `follow=true` for remote builds; `source_build_logs` gains `follow` cursor API for gradual migration off poll.
6. **Backward compat** — all route changes add, do not remove; legacy `/admin/preview-deployments` kept as 301→`/preview` alias for one release; `build_type` allowlist expansion is flag-gated.

### 2.2 Component map (after)

```
handlers_git.go (canonical HMAC)
  ├─ VerifyGitHubSignature / VerifyGitLab / VerifyBitbucket / VerifyGitea  (service.go)
  ├─ /git/webhook/{provider}  (push → git_sources)
  ├─ /git/webhook/deploy/:serverId (server-scoped idempotent)
  ├─ /preview/webhook/{provider} (previewenv, same verifiers, fail-open 200)
  └─ /compose/webhook/* (separate)

services/git/
  ├─ service.go (canonical verifiers + provider API)
  ├─ deploy_service.go (unified clone: init→remote add→fetch SHA or clone branch)
  └─ source_deploy.go (delegates build to build.Service, not local docker)

services/build/
  ├─ BuilderDockerfile (local + beacon/daemon.DockerfileBuild)
  ├─ BuilderNixpacks   (local + beacon/daemon.NixpacksBuild)
  ├─ BuilderHerokuish  (future: herokuish via docker, gated)
  └─ BuilderPaketo     (future: paketo via buildpack, gated)

services/previewenv (canonical)
  ├─ store: preview_deployments with partial unique index
  ├─ lifecycle: Create→Deploy→Cleanup→Destroy + reaper
  └─ wiring: ACME / trafficmanager / domains / commit-status (fail-open)

store/
  ├─ git_deployments (legacy, retained, linked via view)
  ├─ deployment_revisions (canonical rollout)
  └─ source_deployments (admin, bridged to build.Service)
```

---

## 3. Detailed Plan

### 3.1 P0 — Unify preview (single service, single route set, DB constraint)

**Goal:** Eliminate duality where `server.go:2608` wires `preview.Service` at `/admin/preview-deployments` and `phase4_registrar.go:37-51` wires `previewenv.Service` at `/preview/*`. Promote `previewenv` as canonical; keep legacy as deprecated alias.

**3.1.1 Choose canonical**

Decision per `REF-APP-GIT09/LF07 DEFERRED_WITH_REASON (decision: promote previewenv)` at `MASTER_FINDING_INDEX.md:28` → keep `previewenv`.

*File:* `forge/api/internal/services/preview/service.go:16-32` legacy stays but handler is switched.

**3.1.2 HTTP wiring**

- Edit `forge/api/internal/http/server.go:2608`
  ```go
  // BEFORE
  registerPreviewDeploymentRoutes(protected, cfg, cfg.PreviewDeploymentSvc, adminIPAccess, mutationLimiter)
  // AFTER — switch to canonical; legacy alias retained for compat
  registerPreviewDeploymentRoutes(protected, cfg, cfg.PreviewDeploymentSvc, adminIPAccess, mutationLimiter) // now delegates to previewenv adapter — see 3.1.3
  // AND in phase4_registrar.go: keep registerPreviewEnvManagementRoutes but mount at canonical namespace
  ```

Preferred: Change `registerPreviewDeploymentRoutes` signature to accept `*previewenv.Service` and delegate; or introduce adapter.

*Option A (minimal, recommended):* Keep function name `registerPreviewDeploymentRoutes` for backwards compat but change its body to proxy to `previewenv.Service`:
- `forge/api/internal/http/handlers_preview_deployments.go:9` — add import `previewenv`, change first param `svc *previewenv.Service`, map endpoints 1:1:
  - `GET /admin/preview-deployments/` → `svc.ListWithExpiry` (instead of `ListAll`)
  - `GET /admin/preview-deployments/:id` → `svc.Get`
  - `POST /admin/preview-deployments/` → `svc.Create` (needs `RepoOwner/RepoName` validation, maps to previewenv's org-limit/TTL)
  - `POST /:id/deploy` → `svc.Deploy`
  - `POST /:id/cleanup` → `svc.Cleanup`
  - `POST /:id/status` → **deprecate** (previewenv doesn't expose arbitrary status; keep as no-op returning 410)
  - `GET /admin/preview-deployments/server/:serverId` → `svc.List(serverId)`
- Add deprecation header `Deprecation: true` + `Sunset: <date>` + WARN log.
- Also keep `phase4_registrar.go:120 registerPreviewEnvManagementRoutes` mounted at `protected.Group("/preview", …)` — new preferred path. Both coexist; docs point to `/preview`.

*Option B (cleaner long-term):* Inline legacy handler out, delete `preview/service.go` in next major. For this phase, alias is safer.

- In `forge/api/internal/http/server.go:155 Config.PreviewDeploymentSvc` — change type to `*previewenv.Service` (or keep both fields, init `PreviewDeploymentSvc` → `previewenv.New` when `phase4_registrar` constructs, passed via `Config` from `main.go`). If `main.go` already builds `preview.Service` there, add bridging initializer: when `PreviewDeploymentSvc == nil && PreviewEnvSvc != nil`, assign. Otherwise wire adapter.

- Ensure route ordering: static segments before wildcard
  - `phase4_registrar.go:121-126` already correct: `/config`, `/server/:serverId` before `/:id`. Mirror in alias.

- Admin namespace clarification: after migration, `protected.Group("/preview", requireRole("admin"), requireAdminScope(...))` (phase-4) is the canonical admin namespace; legacy `/admin/preview-deployments` is alias. Do not move preview routes to public namespace — both are admin-protected.

**3.1.3 Service adaptation**

- `previewenv.Options` already expects `BaseDomain` from `PREVIEW_DOMAIN` etc. at `phase4_registrar.go:37-48`. Ensure `server.go` construction mirrors same env lookup when building alias, so both paths share TTL/MaxPerOrg. Extract helper `previewEnvOptions(cfg)` shared between `server.go` and `phase4_registrar.go`.

- Handle field mapping divergence:
  - Legacy `Create:42-64` builds `UniqueSuffix = uuid[:8]` + hardcoded URL — canonical builds per-PR host (`PreviewURL:92`). Alias must translate: if legacy client expects `preview-<suffix>.example.com` format, return canonical URL anyway (documented breaking change, mitigated by deprecation window).
  - Legacy `HandleWebhook:169` expects `payload["pr_number"] int` — previewenv's handlers parse provider payloads properly (`previewenv/webhook_*.go` — not separately shown but wired). Alias webhook path not used for creation; keep previewenv webhooks only.

**3.1.4 DB — per-PR partial unique index (race fix)**

*Migration:* additive, no downtime.

File `forge/api/migrations/211_preview_per_pr_unique.sql` (new):

```sql
-- Fix race in previewenv/service.go:144-155 scan-then-create.
-- Enforces exactly one active preview per (pr_number, repo_owner, repo_name) where active.
-- Lower-case repo_owner/repo_name normalized by service (sanitizeHostPart) but index must match query:
-- queries use strings.EqualFold comparison; make functional index on lower() to be sound.

CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_unique_active_pr
ON preview_deployments (
  pr_number,
  lower(repo_owner),
  lower(repo_name)
)
WHERE status IN ('deploying', 'running');

-- Optional: also guard per-branch preview path used by upsertAndDeploy (prNumber derived from hash)
-- so same branch+owner+repo cannot duplicate while active:
CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_unique_active_branch
ON preview_deployments (
  lower(repo_owner),
  lower(repo_name),
  lower(branch)
)
WHERE status IN ('deploying', 'running') AND pr_number > 0;
-- Note: second index may be too strict for legitimate concurrent PRs on same repo with same branch name on different forks.
-- Prefer only first index + application-level branch guard for this phase; add second behind flag if needed.

-- Backfill check (should be no violation after reaper): verify before deploy.
-- No data migration required.

COMMENT ON INDEX idx_preview_unique_active_pr IS 'prevents concurrent duplicate preview rows for same PR (previewenv/service.go:144 race)';
```

**Why `WHERE status IN ('deploying','running')`:** matches `store_preview_env.go:75 ListExpiredPreviewDeployments` and counting scope. Include `stopped` if desired — but canonical active set per `previewenv/service.go` and `store_preview_env.go:59` is `('deploying','running','stopped')`. The requested spec says `WHERE status IN ('deploying','running')` — follow spec, document divergence: `stopped` previews are paused, should still block new PR? Decision: include `stopped` in unique index as well (`IN ('deploying','running','stopped')`) to be consistent with `CountActivePreviewDeploymentsForOrg:59`. For minimal diff, create index with `('deploying','running')` and second index covering `stopped` if needed. Recommend spec-exact `('deploying','running')` then extend if race persists on stopped.

**Error handling:** On `Create` violation (`pq: duplicate key idx_preview_unique_active_pr`), service must translate to `ErrAlreadyExists` (already at `previewenv/service.go:179-180`). Add detection:

```go
// in Create: after s.store.CreatePreviewDeployment
if err != nil && isUniqueViolation(err) {
    return nil, ErrAlreadyExists
}
```

Add `isUniqueViolation` helper checking `pgErr.Code == "23505"` and constraint name.

**TTL & retention wiring:**

- Ensure `phase4_registrar.go:37-48` defaults `TTL 24h` and `RetainCleaned 24h` are persisted. Verify `forge/api/internal/store/store_preview_env.go:17 SetPreviewDeploymentExpiresAt` is called from `Create:182` and `Deploy` does not reset TTL incorrectly.
- Validate migration `180_preview_ttl.sql:5` column is present before index creation (ordering).

**3.1.5 Reaper & retention**

- `previewenv.Service.StartReaper` (not shown but called at `phase4_registrar.go:53-55`) should poll `ListExpiredPreviewDeployments` and `ListReapablePreviewDeployments` on `5m` tick, transition `deploying|running` → `cleaned_up` via `Cleanup`, then after `RetainCleaned` → `Delete`. Ensure `store_deployment_history.go:267 UpdatePreviewDeploymentStatus` sets `cleaned_at` correctly; previewenv's reaper should `UPDATE ... SET status='cleaned_up', cleaned_at=now()` not just `Delete`.

- Add Prometheus metric: `preview_deployments_active{owner}`, `preview_reaper_expired_total`.

**3.1.6 Deprecation timeline**

- Phase N: both routes live, legacy logs WARN.
- Phase N+1: legacy returns `410` with `Gone` + helpful message pointing to `/api/v1/preview`.
- Phase N+2: remove `forge/api/internal/services/preview/*`.

**Backward compat guarantees:** Existing `GET /admin/preview-deployments` clients continue to receive 200 with new shape (`expires_at` additive field, URL format changed). Clients must handle additive field; if they strict-validate URL regex `preview-[a-z0-9]{8}.example.com`, they'll break — document in changelog, provide feature flag `PREVIEW_LEGACY_URL_FORMAT=true` to emit legacy host for N days (controlled via `previewenv.Options.BaseDomain` override path — not recommended, at most config switch).

---

### 3.2 P1 — Align validators, fix shallow SHA fragility, unify cred handling

**3.2.1 Single shared validator**

Create `forge/api/internal/services/git/validate.go` (new, additive):

```go
package git

import (
  "regexp"
  "strings"
  "fmt"
)

var gitRefNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

func ValidateBranchShared(name string) error {
  if name == "" { return fmt.Errorf("%w: branch is required", ErrInvalidBranch) }
  if len(name) > 256 { return fmt.Errorf("%w: branch name too long", ErrInvalidBranch) }
  if strings.HasPrefix(name, "-") { return fmt.Errorf("%w: must not start with '-'", ErrInvalidBranch) }
  if !gitRefNamePattern.MatchString(name) { return fmt.Errorf("%w: disallowed characters", ErrInvalidBranch) }
  if strings.Contains(name, "..") || strings.Contains(name, "/.") || strings.Contains(name, "//") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".lock") {
    return fmt.Errorf("%w: disallowed sequence", ErrInvalidBranch)
  }
  if strings.Contains(name, "@{") || strings.ContainsAny(name, " \t\r\n~^:?*[\\'\"`$&|;") {
    return fmt.Errorf("%w: disallowed characters in branch", ErrInvalidBranch)
  }
  return nil
}
func ValidateRepoURLShared(repoURL string) error {
  // delegates to existing ValidateRepoURL but adds SSRF guard via net.LookupIP or webhook.ValidateURL
  // Keep allowedHost map unified.
}
func IsHex40(s string) bool { /* move from beacon/git.go:407 */ }
```

- Change `deploy_service.go:464 validateBranch(branch) bool` to `ValidateBranchShared(branch) == nil`.
- Change `beacon/internal/server/git.go:58 validateGitRefName` to call same package via import or duplicate regex synced via test — but beacon is separate module (`gamepanel/beacon` vs `gamepanel/forge`); cannot import forge's `git` package directly without circular deps. Therefore: **extract shared validation into `packages/gitvalidate` or duplicate with contract test**. Recommendation: create `beacon/internal/gitvalidate` + `forge/internal/services/git/validate.go` with identical source (copy + test vector shared via `go vet` or golden file). Add `go test ./...` cross-check: `gitvalidate_test.go` asserts same valid/invalid cases in both repos.

- Unify `ValidateSSHRepoURL` at `deploy_service.go:312` — keep allowlist `allowedGitHosts` (there vs beacon's different allowed set). Beacon currently rejects all SSH (`handleGitClone:109-111` “SSH URLs are not supported”), while Forge direct path allows SSH if credential is `GitCredentialSSHKey`. After unification: Forge direct path keeps SSH support, beacon path remains HTTPS-only (documented). Provide validator that takes `allowSSH bool`.

**3.2.2 Clone flow unification — SHA-pinned**

*Current fragile path:*
`deploy_service.go:207 clone --depth 1 --single-branch --branch <branch>` then `fetch --depth 1 origin <sha>` — fails when SHA not advertised as want (`allowAnySHA1InWant=false`), or when depth 1 doesn't contain parent needed for checkout.

*Target (beacon-correct, adopt everywhere):*

For `commitSHA != ""`:
```go
// in cloneWithOptions or new helper cloneAtCommitShared
initCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "init")
remoteCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "remote", "add", "origin", repoURL)
fetchCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "fetch", "--depth", "1", "origin", commitSHA, "--no-tags", "--filter=blob:none")
checkoutCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "checkout", "--detach", "FETCH_HEAD")
revCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "rev-parse", "HEAD")
 // verify actualSHA == requested
```

For `commitSHA == ""` (branch tip):
```go
cloneCmd := exec.CommandContext(cloneCtx, "git", "clone",
  "--depth", "1",
  "--single-branch",
  "--branch="+branch, // single token defense-in-depth
  "--no-tags",
  "--config", "core.symlinks=false",
  "-c", "filter.lfs.required=false",
  "-c", "protocol.file.allow=never",
  "-c", "protocol.ext.allow=never",
  "-c", "core.gitProxy=none",
  "--",
  repoURL, safeDir)
```

File changes:

- `forge/api/internal/services/git/deploy_service.go:164-265`
  - Extract `cloneRepoSHA`, `cloneRepoBranch` helpers.
  - Replace `deploy_service.go:207` branch-clone + `228-238` SHA fetch with above.
  - Propagate `credEnv` (env var askpass) instead of `cmd.Env` built from shell interpolation.
- `forge/api/internal/services/git/service.go` — add (optional) `CloneOptions{RepoURL, Branch, CommitSHA, CredentialID, Token}` helper used by both `GitDeployService` and `build.Service`.
- Keep timeouts `cloneCtx 10m` (`deploy_service.go:200`), add per-fetch timeout (already shared ctx).

**Defense-in-depth:** Always pass `--branch=<value>` as single arg, always include `--` before positional `repoURL`/`safeDir`, always set `GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`, etc. already done in beacon.

**3.2.3 Credential handling — Beacon env-var askpass pattern for Forge direct path**

*Current Forge direct path (`deploy_service.go:108-133`, `287-310`):*
```go
writeAskPassScript(username, password string) // 0700 script: #!/bin/sh\ncase "$1" in *Username*) echo '<quoted user>';; *Password*) echo '<quoted pass>';; esac
// uses shellQuote:308  "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
// then cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_ASKPASS=%s", askPassFile))
```
Risk: token embedded in script body → briefly on-disk with value; shellQuote correct but still disk exposure; `os.CreateTemp("", "git-askpass-*")` uses shared `/tmp` (world-readable parent).

*Beacon correct pattern (`beacon/internal/server/git.go:344-405`):*
- Dedicated `0700` per-request directory `forge-git-askpass-*` under `dataDir` (not shared `/tmp`).
- Script contains **no secrets**: `printf '%s' "$FORGE_GIT_ASKPASS_USERNAME"` / `"$FORGE_GIT_ASKPASS_PASSWORD"` — values live only in `env`.
- `Chmod 0700` before write, `0644` not used.
- Cleanup via deferred `os.RemoveAll(askPassDir)` on every exit.
- Documented residual window note (line 342-343) acknowledged.

**Plan for Forge (`deploy_service.go`):**

Implement `writeAskPassEnvDir(tempBaseDir, username, password string) (env []string, cleanup func(), error)` matching beacon:

```go
func gitEnvWithAskPass(baseEnv []string, tempBaseDir, username, password string) ([]string, func(), error) {
  if username == "" && password == "" { return baseEnv, func(){}, nil }
  parent := tempBaseDir
  if parent == "" { parent = os.TempDir() }
  if err := os.MkdirAll(parent, 0o700); err != nil { return nil, func(){}, err }
  askPassDir, err := os.MkdirTemp(parent, "forge-git-askpass-*")
  if err != nil { return nil, func(){}, err }
  cleanup := func(){ _ = os.RemoveAll(askPassDir) }
  scriptPath := filepath.Join(askPassDir, "askpass.sh")
  // Open with 0600 before write, chmod 0700
  const script = "#!/bin/sh\ncase \"$1\" in\n\t*[Uu]sername*) printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" ;;\n\t*) printf '%s' \"$FORGE_GIT_ASKPASS_PASSWORD\" ;;\nesac\n"
  f, err := os.OpenFile(scriptPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o700)
  // write, chmod, close
  env := append(baseEnv, "GIT_ASKPASS="+scriptPath, "FORGE_GIT_ASKPASS_USERNAME="+username, "FORGE_GIT_ASKPASS_PASSWORD="+password)
  return env, cleanup, nil
}
```

Update `cloneWithOptions:96-147` to use this:

```go
baseEnv := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", …)
credEnv, cleanup, err := gitEnvWithAskPass(baseEnv, d.tempBaseDir, username, password)
defer cleanup()
cmd.Env = credEnv
```

For SSH: keep `writeSSHKeyFile:267` but ensure parent dir is `0700` and `Chmod 0600` before write (already does). Additionally, ensure `knownHosts` validation not bypassed when using askpass path — currently SSH path requires `GIT_SSH_KNOWN_HOSTS:214-218`, non-SSH doesn't. Keep.

For token path `CloneWithToken:151-162` — same `gitEnvWithAskPass`.

Add test `deploy_service_test.go` asserting temp dir permissions `0700`, script contains no token substring.

**Backward compat:** Old `writeAskPassScript` remains (unused) or is replaced; callers unaffected. No migration.

---

### 3.3 P1 — Build matrix: honest 422 now, wire next

**3.3.1 Keep honest until wired — no revert**

`handlers_source_deployments.go:90-97` is correct. Do not regress to `5 accepted, 1 executed` (FALSE_COMPLETION). Keep 422 with message: `"buildType must be \"dockerfile\" (nixpacks, heroku, paketo, static are not yet supported by source deployments)"`.

**3.3.2 Feature-flagged expansion**

Introduce env feature flags (additive, no code change to default behavior):

```go
// in handlers_source_deployments.go:95
enabledBuildTypes := map[string]bool{"dockerfile": true}
if strings.EqualFold(os.Getenv("BUILD_ENABLE_NIXPACKS"), "true") {
  enabledBuildTypes["nixpacks"] = true
}
if strings.EqualFold(os.Getenv("BUILD_ENABLE_HEROKU"), "true") { // herokuish
  enabledBuildTypes["heroku"] = true
  enabledBuildTypes["herokuish"] = true // alias
}
if strings.EqualFold(os.Getenv("BUILD_ENABLE_PAKETO"), "true") {
  enabledBuildTypes["paketo"] = true
  enabledBuildTypes["cnb"] = true // alias from 041_buildpack_support.sql
}
if !enabledBuildTypes[req.BuildType] {
  return fiber.NewError(422, "buildType %q not enabled; set BUILD_ENABLE_*")
}
```

Same gate in store (optional) — DB CHECK already allows all 5, so handler gate is sufficient.

**3.3.3 Wire execution**

When flag enabled, `source_deploy.go:81` dispatch must route to `build.Service`:

*Current dispatch (local docker only):*
```go
switch d.BuildType {
case "dockerfile","": if err := e.buildDockerfile(execCtx, d, clone.Dir, clone.CommitSHA); err != nil ...
default: fail("not supported")
}
```
`buildDockerfile` runs `dockerBuild`/`dockerPush` directly in panel process — bypasses beacon, bypasses `CacheFrom/CacheTo/Platform`.

*Target dispatch (when flag on):*

```go
switch d.BuildType {
case "dockerfile","":
  // if build.Service available, delegate; else fallback local (compat)
  if e.buildSvc != nil {
    // Use build.Service: needs SourceDeployExecutor to hold *build.Service
    opts := build.BuildOptions{
      SourceDir:    clone.Dir,
      Dockerfile:   filepath.Join(clone.Dir, d.DockerfilePath),
      ImageName:    imageTag, // from resolveImageTag
      CacheFrom:    /* parse from d.Registry? or new fields CacheFrom/CacheTo */ ,
      Platform:     /* new field Platform */ ,
      RepositoryURL: d.Repository, Branch: d.Branch, CommitSHA: clone.CommitSHA,
      TenantID:     /* derive from d.CreatedBy or ServerID */,
    }
    _, err := e.buildSvc.StartBuild(execCtx, d.ID, build.BuilderDockerfile, opts, nil)
    // wait or async? retain current synchronous 30m timeout pattern — poll GetBuild until terminal
  } else {
    e.buildDockerfile(...)
  }
case "nixpacks","railpack": // railpack treated as nixpacks (railwayapp/nixpacks)
  if e.buildSvc != nil {
    opts := build.BuildOptions{ SourceDir: clone.Dir, ImageName: imageTag, BuildArgs: ..., TenantID: ... }
    _, err := e.buildSvc.StartBuild(execCtx, d.ID, build.BuilderNixpacks, opts, nil)
  } else {
    fail("nixpacks requires build service")
  }
case "heroku","herokuish","cnb","paketo":
  // herokuish/paketo are CNB-style; panel has no herokuish binary — use beacon's nixpacks as fallback or herokuish builder
  // Option: dispatch to BuilderDockerfile with generated Dockerfile (herokuish shim) or new BuilderPaketo
  // For this phase: return 422 with guidance until herokuish binary bundled
}
```

**Required store changes:** `source_deployments` currently has no `cache_from`, `cache_to`, `platform` columns. Add additive migration:

`forge/api/migrations/212_source_build_cache_platform.sql`:

```sql
ALTER TABLE source_deployments ADD COLUMN IF NOT EXISTS cache_from TEXT[] DEFAULT '{}';
ALTER TABLE source_deployments ADD COLUMN IF NOT EXISTS cache_to   TEXT[] DEFAULT '{}';
ALTER TABLE source_deployments ADD COLUMN IF NOT EXISTS platform   TEXT DEFAULT '';
ALTER TABLE source_deployments ADD COLUMN IF NOT EXISTS builder_config JSONB DEFAULT '{}';
```

Update `store_source_deployments.go:14-60` structs: `CacheFrom []string`, `CacheTo []string`, `Platform string`, `BuilderConfig json.RawMessage`. Update `Create/Update/List` queries to include them. Allowlist in `UpdateSourceDeployment:244` add columns.

Update `CreateSourceDeploymentBody:13` (handlers_source_deployments.go) with same fields; validate via `isSafeBuildxRef`/`isSafePlatform` (beacon logic `build.go:541-571`) before persisting.

**UI:** `forge/web/lib/api/source-deployments.ts:25` already lists types; add fields `cacheFrom/cacheTo/platform` to form at `forge/web/app/admin/source-deployments/page.tsx:176`. Hide cache/platform when flag off; show when `BUILD_ENABLE_*` exposed via `GET /config` endpoint (or just always show — beacon honors only when forwarding).

**Beacon side:** Already supports `CacheFrom/CacheTo/Platform` after LF-04 fix (`beacon/internal/server/build.go:54-56,126-138`). No change needed except ensuring `nixpacks` path also forwards cache/platform — currently `nixpacksBuildRequest:64-77` lacks `CacheFrom/CacheTo` (only `Platform` not yet — check `beacon/internal/server/build.go:64` missing those fields while `dockerfile` has them). Add them to `nixpacksBuildRequest` for parity (even if nixpacks ignores cache, platform matters). See 3.3.5.

**Railpack:** Treat as alias to `nixpacks` (`forge/api/internal/services/forgefile/service.go:144` already accepts both). When `build_type='railpack'` normalize to `nixpacks` at handler (`handlers_source_deployments.go:87 req.BuildType = normalizeBuildType(req.BuildType)` where `railpack→nixpacks`, `cnb→paketo`, `herokuish→heroku`).

**3.3.4 Execution path convergence**

Current `SourceDeployExecutor` clones via local `GitDeployService` (which itself may clone locally) then `dockerBuild` locally. This duplicates beacon's `build.Service.executeRemoteBuild` git clone + build remoting.

Target for remote-capable deployments: `SourceDeployExecutor.RunDeployment` should **delegate entirely to `build.Service.StartBuild` with `RepositoryURL/Branch/CommitSHA`**, letting `build.Service` choose node via `NodeCapabilitySelector` and execute on beacon (which then clones via correct SHA init fetch). Keep local path as fallback when `buildSvc == nil` or flag off.

Add to `SourceDeployExecutor` struct:

```go
type SourceDeployExecutor struct {
  store     *store.Store
  deploySvc *GitDeployService
  buildSvc  *build.Service
  logger    *slog.Logger
}
```

Wire in `main.go` where `NewSourceDeployExecutor` is called — inject `build.Service` (already has `DaemonClient`).

**3.3.5 Align build option forwarding**

File matrix:

- `forge/api/internal/daemon/build.go:16-54` already defines `DockerfileBuildRequest{CacheFrom,CacheTo,Platform:25-27,45-46}` correctly (LF04 fixed).
- `beacon/internal/server/build.go:44 dockerfileBuildRequest` has `CacheFrom/CacheTo/Platform:54-56` — fixed.
- Missing: `nixpacksBuildRequest:64` lacks those fields. Add:
  ```go
  type nixpacksBuildRequest struct {
    ...
    CacheFrom []string `json:"cacheFrom,omitempty"`
    CacheTo   []string `json:"cacheTo,omitempty"`
    Platform  string   `json:"platform,omitempty"`
  }
  ```
  And in `handleNixpacksBuild:193-239` forward platform/cache similarly (platform at least).
- `forge/api/internal/services/build/service.go:93-97 BuildOptions{CacheFrom,CacheTo,Platform}` and `589-604` forwards correctly. No diff.

Add test that `executeRemoteBuild` Nixpacks path includes `Platform` when set.

---

### 3.4 P1 — Logs: live SSE vs post-hoc buffered

**3.4.1 Problem**

- `forge/api/internal/services/build/service.go:609-624` (and 685-699) does `BuildLogs(..., true)` buffered → `GetBuildStatus` → `logBuf`. UI sees logs only after build completes (30-min wait).
- `forge/api/internal/daemon/build.go:118 BuildLogs` with `follow=true` would stream but still buffered client-side via `bufio.Scanner` loop `137-152`.
- `forge/api/internal/daemon/build.go:156 BuildLogsStream` exists but no caller uses it for panel SSE.
- `forge/api/internal/http/handlers_builds.go:91` tries to stream via `buildSvc.StreamLogs` (in-memory `logStream` map `build/service.go:147` fed only when `logCh` passed to `StartBuild` and `executeRemoteBuild` writes `logCh <-` after buffered fetch — too late).
- `source_deployments` logs via `source_build_logs` table are append-only; `GetDeploymentBuildLogs:258` returns all rows without cursor; UI polls.

**3.4.2 Target**

- For **generic builds** (`/builds/:id/logs`): support `?follow=true` live streaming that tails beacon logs via `BuildLogsStream`.
- For **source deployments** (`/source-deployments/:id/logs`): add `?follow=true` SSE that tails `source_build_logs` + optionally live build logs via `build.Service` when delegated.

**3.4.3 Plan — generic builds**

Files: `forge/api/internal/services/build/service.go`, `forge/api/internal/daemon/build.go`, `forge/api/internal/http/handlers_builds.go`, `beacon/internal/server/build.go`

Option A — **use `DaemonClient.BuildLogsStream` for remote builds** (simpler, lower risk):

1. In `build/service.go:executeRemoteBuild` after `DockerfileBuild`/`NixpacksBuild` gets `resp.ID`, **poll or stream**:
   - Change `BuildLogs(ctx, baseURL, token, resp.ID, true)` + separate `GetBuildStatus` to:
     ```go
     stream, err := s.daemonCli.BuildLogsStream(ctx, baseURL, token, resp.ID)
     defer stream.Close()
     // Scan SSE data: lines as daemon returns
     scanner := bufio.NewScanner(stream)
     for scanner.Scan() {
       line := scanner.Text()
       if strings.HasPrefix(line, "data: ") { line = strings.TrimPrefix(line, "data: ") }
       if strings.HasPrefix(line, "event: done") { break }
       logBuf.WriteString(line+"\n")
       if logCh != nil { select { case logCh <- BuildLogEntry{BuildID: buildID, Line: line}: case <-ctx.Done(): return } }
       // Also persist incremental? store.UpdateBuild log?
     }
     status, _ := s.daemonCli.GetBuildStatus(ctx, baseURL, token, resp.ID)
     ```
   - Beacon's `handleBuildLogs:241` already supports SSE (`text/event-stream` when `follow=true` and not terminal). The buffered `BuildLogs` with `follow=true` also streams but client buffers; `BuildLogsStream` returns raw `resp.Body` so caller can forward real-time.

2. In `handlers_builds.go:91 GET /builds/:id/logs`, keep existing SSE logic but feed from `buildSvc.StreamLogs` which now actually receives live lines (since `executeRemoteBuild` writes live). Verify `build/service.go:446-448` stores `logStream[buildID]=logCh` before launching goroutine; `StreamLogs:811-819` returns it. That channel currently only receives after buffered fetch — with streaming fix, it becomes live.

3. Ensure `build/service.go:452-526 StartBuild` propagates `logCh` to `executeRemoteBuild` (already does at `475`). For non-remote (local) `builder.Build`, `runBuildCommand:1088` currently buffers to `bytes.Buffer` not `logCh` — add `streamBuildOutput` helper usage for local path (exists `1154` but unused). Wire it: local `DockerfileBuilder.Build` should also stream via `logCh` when channel supplied.

Option B — **direct proxy to beacon** (alternative, even simpler for panel HTTP to beacon SSE passthrough):

- Add new handler `GET /builds/:id/logs/stream?follow=true` that directly proxies `daemon.Client.BuildLogsStream` to client without Storing. Less store load, but loses persisted replay. Recommend Option A (reuse `logCh`).

Add `follow` semantics:

- `handlers_builds.go:106 follow := c.Query("follow","true")` — if `follow != "true"` return stored `BuildLog` text; else start SSE writer that reads from `logCh` + polls terminal every 30s (current ticker `123`). Change ticker from 30s to 1s for lower latency.

Add tests:

- `forge/api/internal/daemon/build_test.go:10 TestBuildLogsParsesPlainTextAndEventStreams` already covers parsing; add `TestBuildLogsStreamProxiesSSE` with httptest server returning `text/event-stream` and `data:` frames.

**3.4.4 Plan — source deployment logs**

- Extend `handlers_source_deployments.go:258 GetDeploymentBuildLogs` to support `?follow=true` SSE:

```go
func GetDeploymentBuildLogs(cfg Config) fiber.Handler {
  return func(c *fiber.Ctx) error {
    // existing list ...
    if c.Query("follow") != "true" {
      logs, _ := cfg.Store.ListDeploymentBuildLogs(ctx, c.Params("id"))
      return c.JSON(logs)
    }
    // SSE: tail source_build_logs + live build logs if buildSvc has active build
    c.Set("Content-Type","text/event-stream")
    c.Set("Cache-Control","no-cache")
    // poll loop: SELECT ... WHERE deployment_id=$1 AND id > $last AND created_at ASC every 500ms, emit data: JSON(log)
    // also if cfg.SourceDeploymentSvc → buildSvc.StreamLogs for same deploymentId? Map deploymentId→buildId via store.GetActiveBuildByIdempotencyKey or new lookup.
  }
}
```

- Add store helper `ListDeploymentBuildLogsAfter(ctx, deploymentID, afterTime, afterID)` for cursor pagination (add index on `(deployment_id, created_at)` which already exists `114_d:58`). Or simple `WHERE deployment_id=$1 AND created_at > $2`.

- UI at `forge/web/lib/api/source-deployments.ts:146 getDeploymentBuildLogs` — add `getDeploymentBuildLogsStream(id, onLine)` that opens `EventSource` when `follow` supported; fallback to polling for old servers (feature-detect via 406).

- Wire `SourceDeployExecutor.stage` to also `Publish` via `EventRegistry` if present so logs fan out via `events.Publisher` to notification WS — matches `previewenv` publishing pattern.

**3.4.5 Retention**

- `source_build_logs` grows unbounded; add reaper similar to `build` pruning (`store.PruneBuilds`): migration + cron that prunes logs older than 30 days or terminal deployments after 7 days. Not P0 but note.

**Backward compat:** `GET /source-deployments/:id/logs` without `?follow` retains JSON array shape (`BuildLog[]`). `GET /builds/:id/logs` without `follow` retains existing branching (`terminal` check at `handlers_builds.go:98`). Streaming is additive.

---

### 3.5 P1 — Webhook namespaces clarification & idempotency

**3.5.1 Docs & OpenAPI grouping**

- Update `forge/api/internal/http/handlers_git.go:1-15` comments to table above (four namespaces). Expose in Swagger tags:
  ```yaml
  tags:
    - name: git-webhooks-push
      description: POST /git/webhook/{github|gitlab|bitbucket|gitea} — provider push, auto-deploy via git_sources.webhookSecret, HMAC 401 on mismatch
    - name: git-webhooks-deploy
      description: POST /git/webhook/deploy/:serverId — server-scoped deploy, X-Hub-Signature-256, idempotencyKey from X-GitHub-Delivery etc, 401 on missing signature
    - name: preview-webhooks
      description: POST /preview/webhook/* — preview lifecycle, fail-open 200, reuses Verify* helpers
    - name: compose-webhooks
  ```

**3.5.2 Handler alignment**

- Ensure all four namespaces use **same verifier** (`git.VerifyGitHubSignature:248` etc.) — they do. Add shared test vector file `testdata/webhook_signatures.json` exercised by `handlers_git_test.go` and previewenv tests.

- For preview webhooks: current `phase4_registrar.go:74-82` handlers return `200` even on signature mismatch (fail-open, prevents provider retries hammering). Document this difference: `git` handlers return `401` (authenticated surface), preview handlers return `200` (drop). Add comment why.

- Add `GET /git/webhook/deploy/:serverId` health check returning 405 — ensures route not confused with `GET`.

**3.5.3 Idempotency clarification**

- `handlers_git.go:1235 deriveGitDeploymentIdempotencyKey` is correct. Document that `X-GitHub-Delivery`, `X-Gitlab-Event-UUID`, `X-Gitea-Delivery`, `X-Idempotency-Key` all map to `git-deploy:<serverId>:<deliveryID>`. If absent, hash `sha256(serverId + ":" + body)[:16]` hex. Ensure `webhook_deliveries` table (implied via `TryClaimIdempotencyKey` at `handlers_git.go:1221`) has `UNIQUE(idempotency_key)`.

- For provider push hooks: `handlers_git.go:630` currently **no idempotency** — could create duplicate `git_deployments` if provider retries quickly before `HandleWebhookTrigger` commits. Add same `TryClaimIdempotencyKey` guard for provider hooks (`push` events) keyed `git-push:<sourceId>:<afterSHA>`.

- For preview webhooks: use `previewenv/webhook.go: resolveSource` already drops on verify fail; add dedup via `preview_deployments.idempotency`? Not needed because preview's `idx_preview_unique_active_pr` plus `upsertAndDeploy:55-107` handles duplicate pushes (checks `p.CommitSHA == req.CommitSHA` → skip). Document.

**3.5.4 Registration hygiene**

- Confirm no duplicate `registerGitWebhookRoutes` call — `handlers_git.go:1292` defines it, called once from `server.go:2628` via `registerGitRoutes`? Actually `registerGitWebhookRoutes` is standalone, not via `registerGitRoutes`. Ensure `NewServer:2636 registerPhaseHooks` doesn't re-register same prefix. Check `server.go:2636` ordering: `registerPhaseHooks` after `registerGitRoutes:2628`, so `v1 POST /git/webhook/github` would 409 if registered twice — currently only once (`handlers_git.go:1292`). Add guard: `registerGitWebhookRoutes` idempotent check via `app.GetRoute` if available, or document as single registrar per comment.

---

### 3.6 P1 — Disjoint deployment history → unified rollback

**3.6.1 Problem**

- `git_deployments` is git-centric, lacks rollout orchestration.
- `deployment_revisions` is rollout-centric, stores `git_commit_sha` but no FK to `git_deployments.id`.
- `source_deployments` is admin source-centric.
- Rollback API (`handlers` for revisions) operates on `deployments.current_revision_id` (`store_deployment_revisions.go:152`) — git-driven deploys never set `current_revision_id`, so rollback cannot target them.

**3.6.2 Minimal additive unification (no table drops)**

Add nullable FK columns + view, not restructuring:

Migration `213_link_deployments_revisions.sql`:

```sql
-- Link git_deployments to deployment_revisions for rollback visibility.
ALTER TABLE git_deployments ADD COLUMN IF NOT EXISTS revision_id UUID REFERENCES deployment_revisions(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_git_deployments_revision ON git_deployments(revision_id);

-- Link source_deployments to deployment_revisions
ALTER TABLE source_deployments ADD COLUMN IF NOT EXISTS revision_id UUID REFERENCES deployment_revisions(id) ON DELETE SET NULL;

-- Link deployment_revisions back to git_deployments (bidirectional navigation, one nullable)
ALTER TABLE deployment_revisions ADD COLUMN IF NOT EXISTS git_deployment_id UUID REFERENCES git_deployments(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_deployment_revisions_git_deployment ON deployment_revisions(git_deployment_id);

-- Unifying view for history UI
CREATE OR REPLACE VIEW v_deployment_timeline AS
SELECT 'git'::text AS kind, gd.id, gd.git_source_id::text AS deployment_ref, gd.commit_sha, gd.status, gd.created_at
  FROM git_deployments gd
UNION ALL
SELECT 'revision'::text, dr.id, dr.deployment_id::text, dr.git_commit_sha, dr.status, dr.created_at
  FROM deployment_revisions dr
UNION ALL
SELECT 'source'::text, sd.id, sd.server_id::text, sd.commit_hash, sd.status, sd.created_at
  FROM source_deployments sd
ORDER BY created_at DESC;
```

Alternative lighter: only add `git_deployment_id` to `deployment_revisions` and keep `git_deployments.revision_id` as secondary.

**3.6.3 Code wiring**

- In `store_deployment_revisions.go:34 CreateDeploymentRevision`, accept `GitDeploymentID *string` param (new field). After `CreateGitDeployment:44` succeeds, create corresponding `DeploymentRevision` with same `git_commit_sha` and store link, then `SupersedeDeploymentRevisions:143` + `UpdateDeploymentCurrentRevision:152`.

- In `services/git/deploy.go` orchestrator (not shown but `GitDeployOrchestrator` per `service.go:8`) — upon `CompleteGitDeployment`, also create revision row via `store.CreateDeploymentRevision` (if not already created by revision service). Idempotent: check existing via `GetDeploymentRevision` by `git_deployment_id`.

- In `SourceDeployExecutor:RunDeployment` after `stage("completed")`, also create `git_deployments` row (if linking to `git_sources`) and `deployment_revisions` row so timeline view shows it.

- Add handler `GET /deployments/:id/timeline` and `GET /git/servers/:id/timeline` that queries `v_deployment_timeline` filtered by `deployment_ref`/`git_source_id`.

- Rollback path `deployment/revisions.go:147 UpdateDeployment` is non-version-checked → race (`REF-APP-C05/FLF04`). Add `WHERE version = $expected` or `revision_number = $expected AND status='active'` optimistic lock. But that is separate concern — note here and patch while linking: change `store.UpdateDeploymentCurrentRevision` to return `RowsAffected` and error if 0 → `409 Conflict`.

**3.6.4 Deprecation**

- Mark `git_deployments` + `source_deployments` as **observability** tables, `deployment_revisions` as **rollout** authority. New `GET /deployments/:id/revisions` includes derived field `git_deployment_id` so UI can jump to git log. Keep `ListGitDeployments` endpoint but document it proxies underlying `v_deployment_timeline` filtered by kind=git.

---

### 3.7 Migrations additive checklist

| Migration | File | Content | Backward compat |
|-----------|------|---------|-----------------|
| 211 | `migrations/211_preview_per_pr_unique.sql` | partial unique idx `idx_preview_unique_active_pr` + optional `idx_preview_unique_active_branch` | no backfill; fails if duplicate active rows exist → pre-migration job must `CLEANUP` duplicates (choose latest) |
| 212 | `migrations/212_source_build_cache_platform.sql` | `cache_from TEXT[]`, `cache_to TEXT[]`, `platform TEXT`, `builder_config JSONB` on `source_deployments` | nullable/additive, default values |
| 213 | `migrations/213_link_deployments_revisions.sql` | FK cols + view `v_deployment_timeline` | nullable, no data move |
| — | `migrations/214_preview_builder_flags.sql` (optional) | no DB; env flags only | — |
| — | (existing) `180_preview_ttl.sql` already additive | — | — |

All migrations use `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` (Postgres 9.6+). Order matters: 211 after 180 (needs `status` column).

---

### 3.8 Feature flags

| Flag | Env | Default | Effect | Handler |
|------|-----|---------|--------|---------|
| `BUILD_ENABLE_NIXPACKS` | bool | `false` | allow `build_type=nixpacks|railpack` | `handlers_source_deployments.go:90` |
| `BUILD_ENABLE_HEROKU` | bool | `false` | allow `heroku|herokuish` | same |
| `BUILD_ENABLE_PAKETO` | bool | `false` | allow `paketo|cnb` | same |
| `PREVIEW_LEGACY_URL_FORMAT` | bool | `false` | emit `preview-<suffix>.example.com` for alias path only | `previewenv.Options` wrapper |
| `PREVIEW_MAX_PER_ORG` | int | `10` (phase4 registrar default 5, previewenv default 10) | lifecycle limit | `phase4_registrar.go:40` already env-driven; unify to single source |
| `PREVIEW_TTL` | duration | `24h` | expiry | same |
| `BUILD_LOG_STREAM_FOLLOW` | bool | `true` | enable SSE follow on logs | `handlers_builds.go:106` |

Expose flags via `GET /config` or `GET /builds/capabilities` (beacon already `capabilities.go:64`).

---

### 3.9 Step-by-step implementation sequence (5 workstreams, additive)

**Workstream A — Preview unification (2 days)**

1. Create migration `211_preview_per_pr_unique.sql` + apply in dev (`make migrate`).
2. Add `isUniqueViolation` helper in `store/preview_env.go`.
3. Edit `previewenv/service.go:179` to handle duplicate violation → `ErrAlreadyExists`.
4. Edit `handlers_preview_deployments.go:9` to accept `*previewenv.Service` + alias mapping; keep old import for deprecation shim if needed.
5. Edit `server.go:2608` to inject `previewenv.Service` into alias; ensure `phase4_registrar.go:26` remains canonical.
6. Extract `previewEnvOptions(cfg)` helper shared.
7. Add tests: concurrent `Create` same PR → second gets `ErrAlreadyExists` + DB constraint violated not panic.
8. Verify `phases final-parity`: both `GET /admin/preview-deployments` and `GET /preview` return same rows (via `ListWithExpiry`).

**Workstream B — Git clone + validator unification (2 days)**

1. Create `forge/api/internal/services/git/validate.go` + `beacon/internal/gitvalidate/validate.go` (duplicate + shared test vectors).
2. Add `ValidateBranchShared`, `IsHex40`, `ValidateRepoURLShared`.
3. Change `deploy_service.go:464` & `beacon git.go:58` to call shared.
4. Create `gitEnvWithAskPass` (0700 dir) per `beacon/git.go:344` pattern; replace `writeAskPassScript` usage in `deploy_service.go:115,154,219-222` + `writeAskPassScript:287`.
5. Refactor `deploy_service.go:207-239` → `cloneAtCommit` (`init+remote add+fetch SHA --filter=blob:none+checkout FETCH_HEAD+rev-parse verify`) vs branch path.
6. Add unit test `TestCloneAtCommit_AllowAnySHA1Off` mocked git (or `git init` inline test with local fixture repo).
7. Verify `SourceDeployExecutor` uses new clone path (since it delegates to `GitDeployService`).

**Workstream C — Build matrix (3 days)**

1. Add migration `212_source_build_cache_platform.sql`.
2. Extend `store_source_deployments.go` structs + queries + allowlist `UpdateSourceDeployment:244`.
3. Extend `handlers_source_deployments.go:13 CreateSourceDeploymentBody` + validation via `isSafeBuildxRef/isSafePlatform`.
4. Add `BUILD_ENABLE_*` flag gating in handler.
5. Extend `SourceDeployExecutor` struct to hold `*build.Service`; inject in `main.go`.
6. Wire `RunDeployment:81` switch to delegate to `build.Service` when flag on (nixpacks/heroku/paketo) else 422 path.
7. Patch `beacon/internal/server/build.go:64 nixpacksBuildRequest` to include `CacheFrom/CacheTo/Platform` + forward.
8. Add `normalizeBuildType` helper (railpack→nixpacks aliases).
9. Tests: `handlers_source_deployments_test.go` asserts 422 vs 201 based on flag; e2e `remote_build_test.go:194` covers nixpacks remote.

**Workstream D — Live logs (2 days)**

1. In `build/service.go:executeRemoteBuild` replace buffered `BuildLogs(...,true)` with `BuildLogsStream` scan loop + `logCh` writes.
2. Add `streamBuildOutput` usage for local builders.
3. In `handlers_builds.go:91` reduce ticker 30s→1s, ensure `Flush()` after each `data:` line.
4. Extend `handlers_source_deployments.go:258 GetDeploymentBuildLogs` to support `?follow=true` SSE + cursor poll + `Flush`.
5. Add `ListDeploymentBuildLogsAfter` helper (optional).
6. UI: add `getDeploymentBuildLogsStream` in `forge/web/lib/api/source-deployments.ts:146` + poll fallback.

**Workstream E — Webhooks & history linking (1 day)**

1. Update `handlers_git.go:1-15` header + Swagger tags for four namespaces.
2. Add idempotency guard to provider push handlers (optional `TryClaimIdempotencyKey` for `git-push:<sourceId>:<after>`).
3. Add migration `213_link_deployments_revisions.sql` + view.
4. Wire `CreateDeploymentRevision` link fields + update orchestrators.
5. Add `GET /deployments/:id/timeline` handler.

---

### 3.10 File-change map (all paths absolute from repo root)

```
# Preview
forge/api/internal/services/preview/service.go          — KEEP but dep shim (or delete N+2)
forge/api/internal/services/previewenv/service.go:121-197 — add unique-violation handling
forge/api/internal/services/previewenv/webhook.go:31-49  — no change (already correct)
forge/api/internal/http/handlers_preview_deployments.go:9-103 — rewrite to proxy previewenv
forge/api/internal/http/phase4_registrar.go:16-57,67-72,120-190 — keep canonical, share Options helper
forge/api/internal/http/server.go:155,2608               — change Config type, route wiring
forge/api/internal/store/store_deployment_history.go:195-302 — no direct change (keep)
forge/api/internal/store/store_preview_env.go:10-92      — add isUniqueViolation helper
forge/api/migrations/211_preview_per_pr_unique.sql       — NEW partial unique indexes
forge/api/migrations/180_preview_ttl.sql:5               — prerequisite

# Git / clone / creds
forge/api/internal/services/git/deploy_service.go:207-239,267-310,464-507 — refactor clone + askpass + validator
forge/api/internal/services/git/validate.go              — NEW shared validator
beacon/internal/server/git.go:51-75,182-237,344-405      — sync validator, keep askpass pattern
beacon/internal/gitvalidate/validate.go                  — NEW mirror (or copy)
forge/api/internal/services/git/service.go:244            — add shared error vars if needed
forge/api/internal/services/git/source_deploy.go:44-213   — add buildSvc, delegate path

# Build
forge/api/internal/http/handlers_source_deployments.go:13-43,75-135 — add fields, flag gate, normalize
forge/api/internal/store/store_source_deployments.go:14-60,144-188,244-255 — add cache/platform fields
forge/api/migrations/212_source_build_cache_platform.sql  — NEW columns
forge/api/internal/services/build/service.go:28-31,93-97,160-162,365-528,546-735,963-1040 — extend dispatch, stream logs
forge/api/internal/daemon/build.go:16-54,93-116,118-172  — already has cache/platform; add nixpacks fields if missing
beacon/internal/server/build.go:44-77,126-138,193-239    — add cache/platform to nixpacks request
beacon/internal/server/capabilities.go:64-115            — no change (already reports)
forge/api/internal/http/handlers_builds.go:25-154       — live tail wiring
forge/api/internal/services/forgefile/service.go:34-145  — no change (already knows nixpacks)

# Webhooks
forge/api/internal/http/handlers_git.go:1-15,599-886,889-898,1165-1298 — docs + idempotency + namespace comments
forge/api/internal/services/previewenv/webhook.go:1-230 — docs
forge/api/internal/http/handlers_compose.go:78           — docs only

# History / rollback
forge/api/internal/store/store_git_deployments.go:1-414  — keep
forge/api/internal/store/store_deployment_revisions.go:1-168 — add GitDeploymentID field + link methods
forge/api/migrations/213_link_deployments_revisions.sql — NEW FK cols + view
forge/api/internal/store/store_deployment_history.go:55-143 — no change

# Tests
forge/api/internal/http/handlers_preview_deployments_test.go:16,57 — update to previewenv.Service
forge/api/internal/http/handlers_git_test.go:99-238    — add webhook idempotency + HMAC 401 assertions
forge/api/internal/services/git/service_test.go          — add ValidateBranchShared vectors
beacon/internal/server/git_test.go / gitvalidate_test.go — add same vectors
forge/api/internal/daemon/build_test.go:10-43            — add BuildLogsStream SSE test
forge/api/internal/services/e2e/remote_build_test.go:194-218 — extend to nixpacks+platform
```

---

### 3.11 Backward-compatibility matrix

| Change | Breaks? | Mitigation |
|--------|---------|------------|
| Preview route alias (`/admin/preview-deployments` → also `/preview`) | No — both live | Add `Deprecation` header, keep alias 1 release, Swagger marks old as `deprecated: true` |
| Preview URL format `preview-<suffix>.example.com` → `pr<N>-<owner>-<repo>.<domain>` | Possibly for hardcoded parsers | `PREVIEW_LEGACY_URL_FORMAT` toggle on alias only; changelog entry; tests assert new format |
| DB partial unique index | No for valid data; duplicates pre-existing would fail migration | Pre-migration query to deduplicate: `SELECT repo_owner,repo_name,pr_number FROM preview_deployments WHERE status IN ('deploying','running') GROUP BY … HAVING count>1` → clean via `UPDATE status='cleaned_up'` for older rows before `CREATE INDEX`. Index `IF NOT EXISTS` safe to re-run |
| `source_deployments.cache_from/cache_to/platform` columns | No — nullable, defaults `{} / ''` | Queries add `COALESCE`; old clients POST without fields still 201 |
| `build_type` allowlist expansion flag-gated | No — default false keeps 422 | Flag on requires beacon capability (`capabilities.go:115 NixpacksEnabled`) → 503 if beacon missing `nixpacks` |
| `deploy_service.go` askpass dir change (`os.TempDir` generic → `tempBaseDir/forge-git-askpass-*` 0700) | No — env var names same (`FORGE_GIT_ASKPASS_*`) | Cleanup still `defer RemoveAll`; script path changes only |
| Build log `follow=true` SSE | No — new query param, old `GET` without param unchanged | Content-Type changes from `application/json` array to `text/event-stream` when `follow=true` — documented; fallback poll remains |
| `deployment_revisions.git_deployment_id` FK | No — nullable | View `v_deployment_timeline` read-only; existing rollback APIs unchanged |
| Validator stricter (`^[A-Za-z0-9._/-]+$`) | May reject previously accepted branch names containing `[]~^:` etc. | Those characters were already rejected by old `validateBranch:469` but via blocklist; shared validator is stricter but compatible — add test to enumerate previously valid set and assert still valid or document as hardening |

---

### 3.12 Risks & backout

| Risk | Impact | Mitigation |
|------|--------|------------|
| Partial unique index creation fails due to duplicate active rows | Migration blocks deploy | Pre-migration job: `SELECT … HAVING count>1` → deduplicate; make migration non-transactional chunk or `CREATE INDEX CONCURRENTLY` (Postgres) wrapped in `DO $$` |
| Beacon `fetch --filter=blob:none` not supported on old git | `git fetch` fails on old beacon image | Beacon `build.go:59` already correct for git; add feature detection: if fetch fails with `unknown option` fallback to without `--filter` |
| Naked `GO` shared validator drift between `forge` and `beacon` (separate modules) | Branch accepted on panel but rejected on beacon, or vice versa | Share test vectors file `forge/api/testdata/branch_ref_vectors.json` copied to beacon via `go generate`; CI asserts both repos' validator passes identical cases |
| Streaming SSE load on beacon (many clients `follow=true`) | Beacon log buffer `maxLogBufferSize 100MB` per `beacon/build.go:42` → memory pressure | Limit concurrent `follow` subscribers per build (e.g., 32), add `GET /build/logs?follow=true` rate limit `readLimiter`; set `builds.active` eviction `maxCompletedRetention 1h:496` |
| Source deploy now delegates to beacon `build.Service` but beacon offline | `RunDeployment` fails with node unavailability → status `failed`, not `canceled` | Retry logic `RetrysBuild:1211` with backoff; `selectNode:358` returns error; caller surfaces as `failed` with message "no available build node" — acceptable; allow manual retry; add `HealthService` guard |

Backout: all migrations additive with `IF NOT EXISTS` and nullable columns — drop with `DROP INDEX IF EXISTS idx_preview_unique_active_pr`, `ALTER TABLE … DROP COLUMN IF EXISTS` if needed. Feature flags off reverts build matrix to dockerfile-only.

---

### 3.13 Observability & verification

- **Metrics**
  - `preview_deployments_active{owner}`, `preview_reaper_runs_total`, `preview_unique_conflicts_total` (from `isUniqueViolation` counts)
  - `git_clone_duration_seconds{kind=branch|sha, result=succeeded|failed}`, `git_askpass_parent_mode{mode=dedicated_0700}`
  - `build_type_admission_total{buildType, result=201|422}`, `build_remote_stream_active`
  - `webhook_hmac_rejects_total{namespace, provider}` (preview vs git, both)
  - `deployment_timeline_mismatch_total` (view detects disjoint)

- **Logs**
  - Preview creation: `"previewenv: created {id, pr, owner/repo, url, expires_at}"`
  - Unique conflict: `"previewenv: duplicate pr blocked {pr, owner/repo}"`
  - Webhook drop: `"previewenv: webhook signature verification failed {repo, branch}"` vs `"github webhook signature verification failed"` already at `handlers_git.go:634`
  - Clone fallback: `"git: fetch blob:none unsupported, retrying without filter"`

- **Alerts**
  - `preview_unique_conflicts_total` rate spike → webhook storm or index missing
  - `git_clone_duration > 5m` p99 → beacon git issue
  - `build_remote_stream_active > 100` → capacity

- **Verification checklist (run locally before merge)**
  ```bash
  # 1 — migrations up + index exists
  psql $DATABASE_URL -c "\d preview_deployments" | grep idx_preview_unique_active_pr
  # 2 — clone paths
  go test ./forge/api/internal/services/git -run TestCloneAtCommit -count=1
  go test ./beacon/internal/server -run TestGitClone -count=1
  # 3 — preview duality
  curl -s http://panel/api/v1/admin/preview-deployments | jq . # legacy alias
  curl -s http://panel/api/v1/preview | jq .                  # canonical same rows
  curl -X POST http://panel/api/v1/preview/webhook/github -H "X-Github-Event: pull_request" --data @pr.json -v  # 200 even on bad sig (preview), 401 on bad sig for /git/webhook/github
  # 4 — build matrix
  curl -X POST http://panel/api/v1/source-deployments -H "Authorization: Bearer $TOKEN" -d '{"repository":"https://github.com/org/repo","buildType":"nixpacks"}' -> expect 422 when BUILD_ENABLE_NIXPACKS=false, 201 when true
  # 5 — logs streaming
  curl -N http://panel/api/v1/builds/$BUILD_ID/logs?follow=true --output -  # SSE frames
  curl -N http://panel/api/v1/source-deployments/$SD_ID/logs?follow=true    # SSE or JSON fallback
  # 6 — timeline view
  psql -c "select * from v_deployment_timeline limit 3"
  ```

---

### 3.14 Ordering & dependencies

```
211_preview_per_pr_unique.sql  ─┐
                               ├─> previewenv race fix
forge/api/internal/http/server.go:2608 alias ─┘

validate.go + gitEnvWithAskPass ─> deploy_service.go SHA fix
                                 └─> source_deploy.go delegates

212_source_build_cache_platform.sql ─> handler flag gate ─> SourceDeployExecutor wire ─> beacon nixpacks platform field

build/service.go streaming fix ─> handlers_builds.go SSE ─> source_deploy logs?follow

213_link_deployments_revisions.sql ─> orchestrator links ─> timeline view
```

No cross-WS blocking except `212` must land before enabling flags; `211` must land before enabling concurrent preview webhook load tests.

---

### 3.15 What is explicitly NOT done in this phase

- No `herokuish`/`paketo` binary bundling at beacon — delegated behind flag, execution remains 422 until binaries proven (prevents false completion resurgence).
- No deletion of legacy tables (`git_deployments`, `preview` service file) — retained for timeline view.
- No `deployment_revisions` sharding or dropping `git_deployments` — view suffices.
- No provider-specific default-branch auto-detection beyond `previewenv/webhook.go:214 findDefaultBranch` probe — keep fail-open.

---

## 4. Acceptance criteria

- [ ] `POST /source-deployments` with `buildType=nixpacks|heroku|paketo` → `422` when flag off, `201` + successful clone→build→push via `build.Service` when flag on; `dockerfile` always 201.
- [ ] `CacheFrom/CacheTo/Platform` round-trips through `source_deployments` → beacon `buildx` args → image built with those flags (verified via beacon `handleDockerfileBuild:126-138` forwarding).
- [ ] `deploy_service.go:207` no longer `clone --depth1 --branch` + `fetch origin <sha>`; unified flow `init+remote add+fetch <sha> --filter=blob:none` for SHA, branch path uses `--branch=<value>` single token. Both paths use env-var askpass `0700` dir.
- [ ] Validator single function: `ValidateBranchShared` used by both `deploy_service.go:ValidateBranch` and `beacon git.go:validateGitRefName` (or vector-synced). No duplicate regex drifting.
- [ ] `preview` alias at `handlers_preview_deployments.go:14` returns same dataset as `phase4_registrar.go:126` (`/preview`) with TTL and wildcard URL; hardcoded `preview-<suffix>.example.com` no longer emitted on alias.
- [ ] Partial unique index `idx_preview_unique_active_pr` exists in `psql \d`; concurrent `Create` same `(pr,owner,repo)` second caller gets `ErrAlreadyExists`, not duplicate row.
- [ ] `GET /builds/:id/logs?follow=true` and `GET /source-deployments/:id/logs?follow=true` emit SSE `data:` frames live, not only post-hoc. `BuildLogsStream` path exercised.
- [ ] Webhook docs list four namespaces, HMAC returns `401` on `/git/*` and `200` drop on `/preview/*`; idempotency key derived from delivery headers logged.
- [ ] `v_deployment_timeline` view exists, `deployment_revisions.git_deployment_id` FK populated for new git-driven rollouts; rollback targets revision correctly.

---

## 5. Appendix — exact file:line anchors (for review)

- `forge/api/internal/services/git/deploy_service.go:207` (`clone --depth 1 …`), `228` (`fetch origin <sha>`), `267 writeSSHKeyFile`, `287 writeAskPassScript`, `308 shellQuote`, `464 validateBranch`, `214-218 GIT_SSH_KNOWN_HOSTS`, `312 validateSSHRepoURL`, `433 checkGitBinary`, `485 ValidateRepoURL`
- `beacon/internal/server/git.go:26 restrictedNetworks`, `51 gitRefNamePattern`, `58 validateGitRefName`, `109-122 SSRF`, `182-237 init+fetch correct`, `306 gitEnv`, `344-405 gitEnvironmentForRequest 0700 dir + env-var script`, `407 isHex40`, `246-258 clone --branch=<value> single-token + -- separator`
- `forge/api/internal/services/preview/service.go:16 Service`, `42 Create no TTL`, `125 hardcoded preview-<suffix>.example.com`, `169 HandleWebhook`
- `forge/api/internal/services/previewenv/service.go:1 phase4 header`, `60 defaultBaseDomain`, `92 PreviewURL`, `121 Create`, `136 CountActive`, `144-155 scan race`, `158-184 expires_at`, `227 Deploy TLS/routing/commit-status`, `340 ensureBaseCertificate`, `366 ensureRoutingRule`, `404 reportStatus`
- `forge/api/internal/services/previewenv/webhook.go:31 resolveSource`, `55 upsertAndDeploy`, `127 cleanupForPR`, `214 findDefaultBranch`
- `forge/api/internal/http/handlers_preview_deployments.go:9 registerPreviewDeploymentRoutes`, `14 /admin/preview-deployments`, `32-66 POST create`
- `forge/api/internal/http/phase4_registrar.go:16 phase4 registrar`, `37 previewenv.New`, `53 StartReaper`, `67 registerPreviewEnvWebhookRoutes`, `120 registerPreviewEnvManagementRoutes`, `126 protected.Group("/preview")`
- `forge/api/internal/http/server.go:155 PreviewDeploymentSvc`, `2608 registerPreviewDeploymentRoutes`, `2628 registerGitRoutes`, `1292 registerGitWebhookRoutes`
- `forge/api/internal/http/handlers_git.go:1 canonical comment`, `599 HandleGitHubWebhook`, `634 401 on bad sig`, `889 buildWebhookURL`, `1165 ReceiveGitDeploymentWebhook`, `1235 deriveGitDeploymentIdempotencyKey`, `1292 registerGitWebhookRoutes`
- `forge/api/internal/http/handholders_source_deployments.go:13 CreateSourceDeploymentBody`, `90-97 honest 422`, `182-211 stage + go RunDeployment`, `258 GetDeploymentBuildLogs post-hoc`, `273 registerSourceDeploymentRoutes`
- `forge/api/internal/services/git/source_deploy.go:29 SourceDeployExecutor`, `44 RunDeployment`, `81 switch dockerfile only`, `110-159 buildDockerfile direct dockerBuild`
- `forge/api/internal/services/build/service.go:28 BuilderDockerfile/Nixpacks`, `93 BuildOptions{CacheFrom,CacheTo,Platform}`, `160 builders map`, `369 StartBuild unknown builder`, `546 executeRemoteBuild`, `589 DockerfileBuildRequest forward`, `609 BuildLogs buffered`, `669 NixpacksBuildRequest`, `1154 streamBuildOutput unused`
- `forge/api/internal/daemon/build.go:16 DockerfileBuildRequest`, `38 NixpacksBuildRequest`, `93 NixpacksBuild URL`, `118 BuildLogs buffered`, `156 BuildLogsStream`
- `beacon/internal/server/build.go:44 dockerfileBuildRequest CacheFrom/CacheTo/Platform`, `64 nixpacksBuildRequest (missing cache)`, `79 handleDockerfileBuild CacheForward`, `126-138 isSafeBuildxRef/Platform`, `193 handleNixpacksBuild`, `241 handleBuildLogs SSE`
- `forge/api/internal/http/handlers_builds.go:25-154 registerBuildRoutes`, `91 GET /:id/logs buffered/stream`
- `forge/api/internal/store/store_deployment_history.go:195 CreatePreviewDeployment`, `281 ListActivePreviewDeployments`, `267 UpdatePreviewDeploymentStatus`
- `forge/api/internal/store/store_preview_env.go:17 SetPreviewDeploymentExpiresAt`, `54 CountActivePreviewDeploymentsForOrg`, `75 ListExpired`, `96 ListReapable`
- `forge/api/migrations/119_deployments_rollbacks.sql:27 preview_deployments`, `180_preview_ttl.sql:5 expires_at`, `106_deployment_unique_active.sql:1 partial unique pattern`
- `forge/api/migrations/114_d_source_deployments.sql:25 build_type CHECK 5 values`, `114_f_git_deployment_tracking.sql:5 git_deployments`, `099_deployment_revisions.sql:4 deployment_revisions`, `041_buildpack_support.sql:6 builder_type`

---

## 6. Notes for reviewer

- All SQL in this plan is `IF NOT EXISTS` additive — safe for hot deploy.
- Feature flags keep the shipped default (`BUILD_ENABLE_*=false`) identical to today's `422` honesty contract; no breaking change on upgrade.
- The most review-sensitive change is `deploy_service.go` askpass rewrite — verify via `go test -run TestAskpassNoEmbeddedSecret` + `ls -ld $tempDir/forge-git-askpass-*` shows `0700`.
- The partial unique index name `idx_preview_unique_active_pr` is referenced in error-translation — do not rename between DB and Go `isUniqueViolation`.
- Beacon's `nixpacks --filter=blob:none` fallback is intentionally handled; do not fail closed on old daemon.
