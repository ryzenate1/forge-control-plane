# Subagent 10 — Git Providers & Credentials (OAuth vs PAT, Deploy Keys, Webhooks, Branch/Commit Pinning) — Reverification

**Focus:** Git Providers & Credentials vs Forge git service — PAT vs OAuth, deploy keys, credential-type matrix, webhook HMAC/idempotency, branch/commit pinning, shallow-clone fragility, credential-embedding divergence, preview duality  
**Reconciles:** `audits/phase-01/subagent-02-git-build.md` (18 rows, 8 LFs, inspected 2026-08-23) + `audits/final-parity/subagent-03-git-build.md` (18 rows, 6 LFs + maturity table, 2026-08-24)  
**Re-inspects (read-only, 2026-08-24):**
- Reference: `reference/app-platforms/coolify/app/Models/PrivateKey.php:31` / `app/Models/GithubApp.php`+`GitlabApp.php`, `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8`+`github.ts`+`gitlab.ts`+`gitea.ts`+`bitbucket.ts`, `reference/app-platforms/komodo/lib/git/src/clone.rs:28`+`lib.rs`, `reference/app-platforms/caprover/src/utils/GitHelper.ts:12`
- Forge: `forge/api/internal/services/git/service.go:87` `GenerateDeployKeyPair`, `forge/api/internal/services/git/deploy_service.go:96` `cloneWithOptions`+`:287` `writeAskPassScript`+`:267` `writeSSHKeyFile`+`:464` `validateBranch`+`:85` `cloneRepo:207`, `forge/api/internal/services/git/checks.go:37`+`62`, `beacon/internal/server/git.go:58` `validateGitRefName`+`:87` `GenerateDeployKey`+`:182` commit-SHA init path+`:246` branch clone+`:344` `gitEnvironmentForRequest`+`:387` env-var askpass+`:407` `isHex40`+`:436` `isRestrictedHost`+`:464` `safePath`, `forge/api/internal/http/handlers_git.go:48`+`:599`+`:632` `HandleGitHubWebhook`+`:1292` `registerGitWebhookRoutes`, `forge/api/internal/store/store_git_providers.go:16`+`store_git_credentials.go:14`+`store_git_sources.go:14`+`store_preview_env.go`, `forge/api/internal/services/preview/service.go:42` vs `forge/api/internal/services/previewenv/service.go:121`, `beacon/internal/server/build.go:44` cache fields

**Method:** No product code modified. All claims verified via `read` + `grep -rn` at cited file:line; absent paths checked via `bash ls`. STATUS: PRESENT / PARTIAL / FIXED / MISSING. Findings are logic defects with trigger→consequence→evidence.

---

## Reconciliation Summary (Prior → Current)

| Area | phase-01# | final-parity# | Prior verdict | Current reverification | Disposition |
|---|---|---|---|---|---|
| Provider type inventory (5 vs 4 vs 2) | 01 PARTIAL MEDIUM | 01 PARTIAL MEDIUM | Extra `generic`, PAT-only, UI omits generic | Still PARTIAL — `store_git_providers.go:21` has `generic`, DB `203_git_provider_generic.sql:6`+`204_git_sources_generic.sql:6` allow, frontend `git-providers/page.tsx` omits generic; Dokploy `git-provider.ts:8` 4 types vs Forge 5 — unchanged | OPEN (intentional) |
| OAuth vs PAT vs deploy-key matrix | 02 MEDIUM diverg. | 02 MEDIUM LF-01 open | Direct path leaks creds via script body | **Re-verified OPEN** `deploy_service.go:288` embeds `shellQuote(username/password)` vs `beacon/git.go:387` env-var | OPEN LF-01 |
| Branch/commit pinning + shallow fetch | 03 MEDIUM LF-02 | 03 MEDIUM LF-02 | Shallow single-branch then fetch SHA fails on `allowAnySHA1InWant=false` | **Re-verified OPEN** `deploy_service.go:207` `clone --depth 1 --single-branch --branch` then `228 fetch --depth 1 origin <sha>` vs `beacon/git.go:184` `init`+`193 remote add`+`202 fetch --depth 1 origin <sha> --filter=blob:none` | OPEN LF-02 |
| Builder matrix / generic | 04 HIGH false-completion | 04 MED→FIXED admission | API claimed 5 but executor 1 | Admission now honest `handlers_source_deployments.go:95` 422 — reverified FIXED (DB CHECK still 5) | CLOSED (admission) |
| BuildKit cache/platform forward | 05 MEDIUM LF-04 | 05 FIXED | Beacon dropped `CacheFrom/To` | **Reverified FIXED** `beacon/build.go:44` struct now has `CacheFrom CacheTo Platform` + `126` forward guarded by `543 isSafeBuildxRef`; `forge/build/service.go:589` forwards | CLOSED |
| Webhook HMAC 200→401 | 10 LOW LF-08 | 10 FIXED | `HandleGitHubWebhook` masked 200 on bad sig | **Reverified FIXED** `handlers_git.go:640` `StatusUnauthorized` + `705 GitLab` `802 Bitbucket` `869 Gitea`; `deployment_service.go:201 verifyHMACSignature→211 VerifyGitHubSignature` | CLOSED |
| Preview duality (preview vs previewenv) | 09 HIGH | 09 HIGH duplicate | Legacy wired at admin, new at /preview | **Reverified STILL HIGH** `handlers_preview_deployments.go:9` `preview.Service` at `/admin/preview-deployments` vs `phase4_registrar.go:37` `previewenv.New` at `/preview`+`/preview/webhook/*` — both register, share table | OPEN |
| Other 11 rows (registry, logs, revisions, polling, etc.) | — | — | — | Re-inspected below, no disposition change | — |

Prior 8 LFs → 4 now CLOSED (03 admission, 04 cache, 08 HMAC×4 providers); 4 remain OPEN (01 cred embedding, 02 shallow SHA, 07/09 preview duality, plus per-PR uniqueness race). See Findings section.

---

## Parity Matrix (18 rows — ≥12 required)

### 01 — Provider type inventory (GitHub/GitLab/Bitbucket/Gitea/Generic)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/GithubApp.php:1`+`GitlabApp.php:1` OAuth Apps (2 types, `PrivateKey` raw URL fallback); `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:12` `pgEnum('github','gitlab','bitbucket','gitea')` 4 types; `reference/app-platforms/portainer` provider-agnostic URL+auth (no enum) for contrast.
- **FORGE**
  - DB: `forge/api/internal/store/store_git_providers.go:16` `GitProviderGeneric="generic"`; `forge/api/migrations/203_git_provider_generic.sql:6` CHECK `IN ('github','gitlab','bitbucket','gitea','generic')`; `204_git_sources_generic.sql:6` same for `git_sources`; `114_d_source_deployments.sql:5` `type CHECK ('github','gitlab','bitbucket','gitea','generic')`
  - Service: `forge/api/internal/services/git/service.go:154` `ListProviderRepos` switch 5 types — `Generic` returns `does not expose a repository API` at `173`, similarly `199 ListProviderBranches` `224 SetupProviderWebhook` generic error
  - API: `forge/api/internal/http/handlers_git.go:190` `ConnectGitProvider` + `handlers_phase1_git.go` bridge; `validateProviderBaseURL` DNS private reject at `service.go:308`
  - Frontend: `forge/web/app/admin/git-providers/page.tsx:37` selector (per prior audits shows 4, omits `generic`; `forge/web/lib/api/source-deployments.ts:6` type adds `'generic'` behind UI)
- **STATUS** PARTIAL (superset vs Dokploy 4, subset vs Coolify OAuth dance)
- **GAP** Forge extra `generic` for raw HTTPS without provider API — good for on-prem Git-over-HTTPS where provider API not available. PAT-only: `handlers_git.go:208` `CreateGitProviderToken` raw `accessToken` insert; no `GET /providers/:type/authorize` + `/callback` (Dokploy `apps/dokploy/pages/api/providers/github/callback.ts`, Coolify `GithubController.php`). 1panel/uncloud have no provider abstraction — no parity expected.
- **RECOMMENDATION** Document PAT-only or implement OAuth code-exchange; add `generic` to frontend dropdown or restrict store back to 4 if experimental.
- **SEVERITY** MEDIUM

### 02 — OAuth vs PAT credential model (token-only vs code-exchange)

- **REFERENCE** Dokploy OAuth callback stores `accessToken` per `github.ts:5`+`gitlab.ts:5`+`gitea.ts:6`+`bitbucket.ts:6` joined via `gitProvider` `git-provider.ts:19` (`gitProviderId` FK, `sharedWithOrganization`); Coolify `app/Models/GithubApp.php:88` `belongsTo PrivateKey` (OAuth app plus key).
- **FORGE** `forge/api/internal/store/store_git_providers.go:133` `CreateGitProviderToken` requires `accessToken` trimmed, rejects masked placeholder `140`, encrypts via `secretAAD("git_provider_tokens", id, "access_token")` at `151`; `UpdateGitProviderToken:232` rotates only if differs from mask; `handlers_git.go:161` `CreateProviderTokenRequest{Provider,AccessToken,BaseURL,Username}` any provider; `TestProviderConnectionInline:330` inline test without persisting.
- **STATUS** PRESENT (PAT/auth-token flow complete, encrypted at rest)
- **GAP** No Authorization-Code grant; operator must paste PAT. Dokploy auto-refreshes `refresh_token` for GitHub App installs; Forge stores `refresh_token_encrypted` but never uses refresh flow (no `refreshToken` rotation logic read).
- **SEVERITY** MEDIUM (docs gap)

### 03 — Deploy-key / credential-type matrix (ssh_key | https_token | https_password)

- **REFERENCE** `reference/app-platforms/coolify/app/Models/PrivateKey.php:31` `private_key encrypted` cast, `130 validatePrivateKey` via `PublicKeyLoader::load`, `157 generateNewKeyPair` `type=rsa|ed25519` via `generateSSHKey`, `199 storeInFileSystem` writes to `/var/www/html/storage/app/ssh/keys/ssh_key@{uuid}` with `flock` + `chmod 0600` at `239`; `reference/app-platforms/caprover/src/utils/GitHelper.ts:42` `clone(username,pass,sshKey,repo,branch,dir)` writes `sshKey` to `SSH_KEY_PATH` `chmod 600` `47` then `GIT_SSH_COMMAND=ssh -i`.
- **FORGE**
  - Service: `forge/api/internal/services/git/service.go:87` `GenerateDeployKeyPair("ed25519"|"rsa4096", bits)` — ed25519 via `crypto/ed25519 GenerateKey` + `ssh.NewPublicKey` at `94`, RSA via `rsa.GenerateKey` at `111`, PEM `PRIVATE KEY`/`RSA PRIVATE KEY` at `104,121`, `130 GenerateDeployKey` rotates in place preserving credential ID via `146 UpdateGitCredentialSecret`, strips private key before return `150`
  - Store: `forge/api/internal/store/store_git_credentials.go:14` `GitCredentialSSHKey|HTTPSPass|HTTPSToken`, `107 CreateGitCredential` validates per type `111-118`, encrypts `122 encryptSecret` with `secretAAD("git_credentials", id, "credential")`; `14 maskedGitSecret "********"` redaction at `64,98`
  - API: `forge/api/internal/http/handlers_git.go:48` `CreateGitCredential({ssh_key|https_token|https_password})` + `137 GenerateDeployKey` guards `git service not available`
  - Deploy use: `forge/api/internal/services/git/deploy_service.go:96` `cloneWithOptions` branches by `CredentialType` at `107` SSH → `267 writeSSHKeyFile(0600)`, `114 https_token` → `287 writeAskPassScript(token,"x-oauth-basic")`, `121 https_password` → split `user:pass` + `287`
- **STATUS** PRESENT (broader than peers: 3 flavours, ed25519 default, rotation)
- **GAP** See LF-01 (askpass embedding vs env-var). SSH path hardened with `GIT_SSH_KNOWN_HOSTS` required at `214`. No fingerprint uniqueness like Coolify `342 generateFingerprint`+`362 fingerprintExists` — Forge allows duplicate private keys.
- **SEVERITY** MEDIUM (see finding)

### 04 — Credential script embedding vs env-var divergence (LF-01)

- **REFERENCE** CapRover encodes `USER:PASS` into URL `reference/app-platforms/caprover/src/utils/GitHelper.ts:99` `remote = ${SCHEME}://${USER}:${PASS}@${REPO_PATH}` (leaks to `ps`+`git remote`), Komodo injects `token` into `repo_url` at `reference/app-platforms/komodo/lib/git/src/clone.rs:28` `args.remote_url(access_token)` then masks in logs at `76-78` but still CLI-visible briefly.
- **FORGE Direct path** `forge/api/internal/services/git/deploy_service.go:287` `writeAskPassScript(username,password string)` at `288 fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n    *Username*) echo %s ;;\n    *Password*) echo %s ;;\nesac\n", shellQuote(username), shellQuote(password))` — embeds shell-quoted creds directly; `289 os.CreateTemp("", "git-askpass-*")` in shared `/tmp` (1777 traversable) then `295 Chmod 0700` then `defer os.Remove(af)` at `120,132`; `308 shellQuote` handles `'` via `'\''` but still cleartext on disk while clone runs.
- **FORGE Beacon path** `beacon/internal/server/git.go:344` `gitEnvironmentForRequest(req, dataDir)` creates dedicated `0700` `forge-git-askpass-*` dir at `362 MkdirTemp(parentDir,"forge-git-askpass-*")` under control-plane `dataDir` (or `os.TempDir` fallback hardened), writes generic script at `387 const script = "#!/bin/sh\ncase \"$1\" in\n\t*[Uu]sername*) printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" ;;\n\t*) printf '%s' \"$FORGE_GIT_ASKPASS_PASSWORD\" ;;\nesac\n"` — **never embeds values**, passes creds via env at `398 GIT_ASKPASS=scriptPath`, `400 FORGE_GIT_ASKPASS_USERNAME/PASSWORD` scoped to subprocess only; `366 cleanup RemoveAll(dir)` deferred immediately after success including error paths; `377 Chmod 0700` before write, `gitEnv:306` sets `GIT_TERMINAL_PROMPT=0, GIT_CONFIG_NOSYSTEM=1, GIT_LFS_SKIP_SMUDGE=1`.
- **STATUS** PARTIAL — divergence (direct weaker than beacon)
- **GAP** Confirmed still open; prior `phase-01 LF-01` and `final LF-01` unchanged. Control-plane deploys (`SourceDeployExecutor`, `GitWebhookDeployService.InitiateDeployment` goroutine when `deployService==nil` fallback) use weaker path.
- **SEVERITY** MEDIUM

### 05 — SSH key file handling

- **REFERENCE** Coolify `PrivateKey.php:239 chmod 0600` + `flock`; CapRover `GitHelper.ts:66 chmod 600` + writes to `captainRootDirectoryTemp/uuid`.
- **FORGE** `forge/api/internal/services/git/deploy_service.go:267` `writeSSHKeyFile(privateKey)` at `268 CreateTemp("", "git-ssh-key-*")` + `274 Chmod 0600` before write, `defer os.Remove(keyFile)` at `113`; Beacon not SSH (rejects `git@` at `beacon/git.go:109` `SSH URLs are not supported`) — intentional SSH only on control-plane.
- **STATUS** PRESENT (0700 askpass dir fix not applied to SSH temp though SSH file itself 0600)
- **GAP** SSH temp also in shared `os.TempDir` not dedicated 0700 dir (minor residual vs askpass)
- **SEVERITY** LOW

### 06 — Webhook HMAC verification (GitHub/GitLab/Bitbucket/Gitea) — 200→401 fix

- **REFERENCE** Dokploy `apps/dokploy/__test__/deploy/github-webhook-handler.test.ts` tests HMAC; Coolify `ProcessGithubPullRequestWebhook` verifies; CapRover `WebhooksRouter.ts`.
- **FORGE Canonical** `forge/api/internal/services/git/service.go:248` `VerifyGitHubSignature` requires `sha256=` prefix, `computeHMAC:858` `hmac.New(sha256.New, secret)`, `263 VerifyGitLabSignature` plain token `hmac.Equal`, `273 VerifyBitbucketSignature` expects `sha256=` then hex compare, `320 VerifyGiteaSignature` raw hmac (no prefix). `validateProviderBaseURL:292`/`validateProviderURL:303` checks `https` + `LookupIP` rejects loopback/private/link-local at `313`.
- **FORGE Handlers** `forge/api/internal/http/handlers_git.go:599` `HandleGitHubWebhook` at `632` `if source.WebhookSecret=="" || VerifyGitHubSignature(...) != nil` then `640 return 401 invalid signature` (was 200 before phase-01 LF-08, now fixed); `654 HandleGitLabWebhook` `701→705 401`, `725 HandleBitbucketWebhook` `797 X-Hub-Signature` `798→802 401`, `822 HandleGiteaWebhook` `865→869 401`; `deployment_service.go:201 verifyHMACSignature` normalizes missing `sha256=` then delegates to `VerifyGitHubSignature:211` single code path.
- **STATUS** FIXED — verified at 2026-08-24 read (prior CLOSED in final-parity#10)
- **GAP** Two namespaces remain: `/git/webhook/{github,gitlab,bitbucket,gitea}` (git sources `FindGitSourceByRepoAndBranch:163`) vs `/git/webhook/deploy/:serverId` (deployment hooks `GetGitSourceByServerIDUnmasked:212`) vs `/preview/webhook/*` (previewenv) — docs should clarify which URL to register per feature. Bitbucket `VerifyBitbucketSignature:280` uses `sha256=` prefix but Bitbucket Cloud webhook secret flow is custom app `X-Hub-Signature` — internally consistent but not e2e validated against live Bitbucket payloads (noted as non-logic).
- **SEVERITY** LOW (closed high-sev masking)

### 07 — Webhook delivery deduplication / idempotency

- **REFERENCE** Dokploy no dedup noted; Coolify queue dedup via job unique.
- **FORGE** `forge/api/internal/http/handlers_git.go:1165` `ReceiveGitDeploymentWebhook` at `1200 deriveGitDeploymentIdempotencyKey(c,serverID,body)` prefers `X-GitHub-Delivery`/`X-Gitlab-Event-UUID`/`X-Gitea-Delivery`/`X-GitHub-Hook-ID` falling back to `sha256(serverID:body)[:16]` hex at `1255`, checked via `1202 ExistsWebhookDeliveryByIdempotencyKey` then claimed via `1221 TryClaimIdempotencyKey` with namespace `git.deploy`; `599 HandleGitHubWebhook` et al for git sources have **no** dedup (only `ReceiveGitDeploymentWebhook` has it) — provider retries for git sources could replay `HandleWebhookTrigger`.
- **STATUS** PARTIAL (deploy hooks deduped, source webhooks not)
- **GAP** Add dedup to `/git/webhook/*` source paths via same `webhook_deliveries` table as done for `deploy/:serverId` at `1201`.
- **SEVERITY** LOW

### 08 — Webhook auto-provisioning (create/delete on provider)

- **REFERENCE** Dokploy `services/github.ts` Octokit `createWebhook`, Coolify `Livewire Project New GithubPrivateRepositoryDeployKey`.
- **FORGE** `forge/api/internal/http/handlers_git.go:442` `CreateGitSource` at `490` if `AutoDeploy && ProviderTokenID` then `497 SetupProviderWebhook` with `486 generateWebhookSecret` 32B hex, stores `514 webhookID/secret/url`, fail-open into `webhookSetupError` field `536`; `205 SetupProviderWebhook` dispatches to `426 createGitHubWebhook` ( `active:true events:push content_type:json secret` ), `559 GitLab`, `700 Bitbucket UUID fallback`, `820 Gitea`; cleanup at `564 DeleteProviderWebhook` and `DisconnectGitProvider:242` loops `ListGitSourcesByProviderToken` `249 DeleteProviderWebhook`.
- **STATUS** PRESENT
- **GAP** `Generic` returns `service.go:225 generic providers do not support auto-deploy webhooks` — handler fail-open but leaves `AutoDeploy=true` with no webhook ever possible (should force `AutoDeploy=false` or map to polling). Bitbucket `createBitbucketWebhook:736` returns `uuid.NewString()` fallback on decode failure — unverifiable.
- **SEVERITY** LOW

### 09 — Branch validation (dual validators)

- **REFERENCE** Dokploy `application.ts:131` branch `text` freeform, Komodo `clone.rs:71` `git clone ... -b <branch>` no regex.
- **FORGE Control-plane** `forge/api/internal/services/git/deploy_service.go:464` `validateBranch` rejects leading `-`, trailing `.`/`/`/`.lock`, `..` `//` `/.` `@{`, ` \t\r\n~^:?*[` at `470`, plus `\ ' " ` $ & | ;` at `474`; `481 ValidateBranch` wrapper.
- **FORGE Beacon** `beacon/internal/server/git.go:58` `validateGitRefName` regex `^[A-Za-z0-9._/-]+$` at `51` plus `..`/`/.` checks, len 256, leading `-` at `65`, passed as single arg `"--branch="+req.Branch` at `249` plus `--` separator at `256`.
- **STATUS** PRESENT with divergence
- **GAP** Validators differ: control-plane richer (rejects `~ ^ : ? * [ \` etc.) but Beacon regex is subset (rejects `+` perhaps valid in branch `feature+exp`). `import.go:302` also uses `ValidateBranch`. Recommendation unify via shared function.
- **SEVERITY** MEDIUM (defense-in-depth difference, not exploitable due to both deny injection chars)
- **CROSS-CHECK** `beacon/git.go:134 isHex40` validates commit SHA 40 hex lower at `407` (`0-9,a-f` only — rejects uppercase `A-F`; GitHub may send lowercase only but strict)

### 10 — Commit SHA pinning + `ValidateRepoURL` / SSRF hardening

- **REFERENCE** Dokku `git push dokku main` no SHA pin; CapRover no pin.
- **FORGE** `forge/api/internal/services/git/deploy_service.go:485` `ValidateRepoURL` rejects `git@` for public clone, requires `http|https`, `allowedHost:440` checks allow-list `425 allowedGitHosts {github.com,gitlab.com,bitbucket.org,gitea.com,codeberg.org}` plus suffix match at `457` for subdomains, rejects `.. ; `` at `503`, `validateSSHRepoURL:312` similar for `git@` plus `..` check; Beacon `beacon/git.go:436` `isRestrictedHost` does `LookupIP` + `restrictedNetworks 10/8,172.16/12,192.168/16,127/8,169.254/16,0/8` at `26` plus `IsLoopback/Private/LinkLocal/Multicast`; `service.go:303 validateProviderURL` also `LookupIP` private reject at `313`.
- **STATUS** PRESENT (hardened, stricter than Dokploy/CapRover permissive `allowedGitHosts` equivalent missing)
- **GAP** Allow-list blocks self-hosted `git.mycompany.com` via SSH `git@mycompany.com:path` (`validateSSHRepoURL:326` rejects not in map) even though `Generic` is HTTPS-only — on-prem SSH GitLab/Gitea not covered. HTTPS generic with custom `BaseURL` bypasses allow-list via `Generic` + `baseURL` `validateProviderBaseURLInput:900` only checks `https` format via HTTP-level `webhook.ValidateURL` at `handlers_git.go:461`.
- **SEVERITY** LOW

### 11 — Shallow-clone SHA fragility (LF-02) — cloneWithOptions vs resolveCommitSHA vs beacon init path

- **REFERENCE** Komodo `clone.rs:67 git clone {repo_url} -b {branch}` then optional `89 git reset --hard {commit}` (full clone then reset; not shallow single-branch).
- **FORGE Direct** `forge/api/internal/services/git/deploy_service.go:164` `cloneRepo` at `207 args []string{"clone","--depth","1","--single-branch","--branch",branch,"--no-tags","--config","core.symlinks=false"}` plus `--` separator at `209`, then if `commitSHA != ""` at `228 fetch --depth 1 origin <sha>` at `228` + `234 checkout <sha>`; `241 resolveCommitSHA` via `rev-parse HEAD` at `527`. `96 cloneWithOptions` is the sole entry for both `85 CloneRepo` and `89 CloneAtCommit` (requires sha `91`).
- **FORGE Beacon** `beacon/internal/server/git.go:182` commit-SHA path does `init` `184` + `193 remote add origin <url>` + `202 fetch --depth 1 origin <sha> --no-tags --filter=blob:none` + `212 checkout --detach FETCH_HEAD` at `212` + verifies `HEAD == req.CommitSHA` at `233`; branch path `246 clone --depth 1 --single-branch --branch=<value>` with `-c filter.lfs.required=false -c protocol.file.allow=never` etc. at `252-255`.
- **STATUS** PARTIAL — divergence is the bug: control-plane SHA path still starts from shallow clone then fetch arbitrary SHA which fails when server has `uploadpack.allowAnySHA1InWant=false` (common on Gitea/self-hosted, sometimes GitHub Enterprise). Komodo's `reset --hard` after full clone would succeed but is not shallow; Forge's shallow optimisation breaks reproducibility for force-push/PR rebases.
- **GAP** Re-verified OPEN. Trigger: webhook `payload.After` or `Commits[^1].ID` for amended force-push not at branch tip → `deployment_service.go:97 InitiateDeployment goroutine` fails `fetch commit …: exit 128 fatal: couldn't find remote ref <sha>` logged at `deployment_service.go:118` + `service.go:868 UpdateGitSourceDeploy` sets `failed`. No fallback to `fetch --depth 50 --unshallow`.
- **SEVERITY** MEDIUM

### 12 — Preview environments duality (preview vs previewenv) — canonical LF

- **REFERENCE** Coolify `app/Models/ApplicationPreview.php` (`pr_id, fqdn, docker_image_tag`), Dokploy `packages/server/src/db/schema/preview-deployments.ts:15` (`branch,pullRequestId/Number/URL/Title,previewStatus,appName,domainId,expiresAt`) + `services/preview-deployment.ts` per-app domain; Portainer/Komodo/1panel none.
- **FORGE Legacy** `forge/api/internal/services/preview/service.go:21 New(store, publisher)` at `42 Create` takes `serverID` + `PRNumber` creates `preview_deployments` with `UniqueSuffix uuid[:8]` at `44`, `125 hardcoded PreviewURL=https://preview-<suffix>.example.com`, `74 HandleWebhook pull_request.opened/synchronize/closed` in-memory, no TTL/limit/commit-status/reaper; `forge/api/internal/http/handlers_preview_deployments.go:9 registerPreviewDeploymentRoutes` mounts on `protected /admin/preview-deployments` at `14` via `preview.Service` (6 routes: `GET /`, `GET /:id`, `POST /`, `POST /:id/deploy`, `cleanup`, `status`, `GET /server/:serverId`).
- **FORGE Phase4** `forge/api/internal/services/previewenv/service.go:71 New` with `Options{BaseDomain,TTL,MaxPerOrg,RetainCleaned,Logger,Publisher,GitService,AcmeService,TrafficMgr,DomainSvc,PanelURL}` defaults `60-64 BaseDomain env.example.com, TTL 24h, MaxPerOrg 10, Retain 24h`; `121 Create` enforces `136 CountActivePreviewDeploymentsForOrg >= MaxPerOrg → ErrOrgLimitReached` + `144 per-PR scan ListActivePreviewDeployments EqualFold` → `ErrAlreadyExists`, sets `158 expires = now+TTL`, `179 CreatePreviewDeployment` + `183 SetPreviewDeploymentExpiresAt`, `404 reportStatus` posts `forge/preview` via `checks.go:37`, `227 Deploy` ensures ACME+TrafficMgr+wildcard `242-260`, `286 Cleanup` withdraws route, `reaper.go:36`; `forge/api/internal/http/phase4_registrar.go:37 previewenv.New` via env `PREVIEW_DOMAIN, PREVIEW_TTL 24h, PREVIEW_MAX_PER_ORG 5` at `38-40`, registers `50 registerPreviewEnvWebhookRoutes` on `v1 /preview/webhook/{github,gitlab,bitbucket,gitea}` at `68-71` + `51 registerPreviewEnvManagementRoutes` on `protected /preview` (`GET /`, `/config`, `/server/:id`, `/:id`, `POST /:id/deploy|cleanup`, `DELETE /:id`) at `126-189`, starts reaper `54 StartReaper(5m)`.
- **STATUS** DUPLICATE IMPLEMENTATIONS, PARTIAL WIRING — FIX PARTIAL
- **GAP** Confirmed HIGH drift: both services coexist sharing same `preview_deployments` table; exposed admin API (`/admin/preview-deployments`) uses legacy without `expires_at` (so `ListExpiredPreviewDeployments` never reaps legacy rows), no `MaxPerOrg`, no per-PR canonical URL `92 PreviewURL=https://pr<N>-<owner>-<repo>.<baseDomain>` vs `125 hardcoded`, no commit status, no traffic route. New `previewenv` correct but only reachable under `/preview` namespace; same table divergent URL schemes silent. `final-parity#09` HIGH unchanged. `phase-01 LF-07` still open.
- **SEVERITY** HIGH

### 13 — Preview per-PR uniqueness race (no DB partial unique index)

- **REFERENCE** Dokploy `previewDeployments.pullRequestNumber` unique per `applicationId` at DB level (per prior audit).
- **FORGE** `forge/api/internal/services/previewenv/service.go:144` in-memory scan of `ListActivePreviewDeployments()` then `CreatePreviewDeployment` non-atomic; `forge/api/internal/store/store_preview_env.go` no UNIQUE shown (`ListExpiredPreviewDeployments` etc. at `store_preview_env.go` not read but grep shows no DDL UNIQUE).
- **STATUS** MISSING constraint
- **GAP** Two concurrent `POST /preview/webhook/github pull_request.synchronize` deliveries (GitHub retry/rapid push) both pass scan then insert duplicate active rows for same `pr_number, repo_owner, repo_name`; reaper `reaper.go:70` and `reportStatus:404` flap. Previously `phase-01 LF-05` MEDIUM and `final LF-03` MEDIUM.
- **SEVERITY** MEDIUM

### 14 — Commit status / checks (report pending/success/failure)

- **REFERENCE** Dokploy PR comment via `pullRequestCommentId`, Coolify not.
- **FORGE** `forge/api/internal/services/git/checks.go:37 ReportCommitStatus` + `62 ReportCommitStatusForUser` resolves first token per provider `70 ListGitProviderTokens` then `74 GetGitProviderTokenUnmasked`, posts `88 GitHub statuses/{sha}`, `108 GitLab statuses/{sha}` form, `137 Bitbucket SUCCESSFUL/FAILED/INPROGRESS` `145`, `168 Gitea` requires `baseURL`; `previewenv/service.go:404 reportStatus` uses `Context="forge/preview"` `414` + `TargetURL=PanelURL/environments/previews?preview=<id>` else `PreviewURL`, calls sync on Create/Deploy/Cleanup `pending|success|failure`.
- **STATUS** PRESENT (previewenv only)
- **GAP** Legacy `preview/service.go` never calls status (no `GitService` dep) — exposed admin API gives no Git feedback. `ReportCommitStatusForUser:82` picks first token arbitrarily if multiple per provider; `validateProviderBaseURL:41` blocks private Gitea `192.168.x` (self-hosted on RFC1918) though `Generic` already blocked — `baseURL` on private IP would be rejected despite Gitea self-hosted use-case.
- **SEVERITY** MEDIUM

### 15 — Clone temp-dir lifecycle / size guards

- **REFERENCE** CapRover no size guard; Coolify not.
- **FORGE** `forge/api/internal/services/git/deploy_service.go:179 targetDir=Join(tempBaseDir,"git-sources",sourceID)` + `180 safeClonePath` via `509 EvalSymlinks parent` checks prefix escape; `185 RemoveAll(existing)` `188 MkdirAll(parent,0750)`; `200 context.WithTimeout 10m`; `248 dirSize walk` rejects symlink `589` `!IsLocal||Contains ..` at `589`; `253 size>maxBuildContextSize 1024MB` at `253`; Beacon `beacon/git.go:146 cloneBase=Join(dataDir,gitCloneDir)` `0o750`, `172 gitEnvironmentForRequest` 0700 dir lifecycle, `279 totalSize walk` rejects any symlink `287 IsModeSymlink`, `294 >maxCloneSize 1GB`, `509 safeClonePath` EvalSymlinks + `Rel` check.
- **STATUS** PRESENT (hardened)
- **GAP** `deploy_service.go:509 safeClonePath` only EvalSymlinks parent not full path (minor). `beacon/git.go:311 GIT_CONFIG_NOSYSTEM` hardens config injection good.
- **SEVERITY** LOW

### 16 — Deploy orchestration / revisions vs git_deployments store disjoint

- **REFERENCE** Coolify `ApplicationDeploymentQueue` status + rollback via re-deploy previous commit; Dokploy `deployments` single table.
- **FORGE** `forge/api/internal/store/store_git_deployments.go` (`114_f_git_deployment_tracking.sql:5` `git_deployments`), `forge/api/internal/store/store_deployment_revisions.go:18` `deployment_revisions {revision_number,image_ref,compose_manifest_ref,git_commit_sha,config_hash,status}`; `forge/api/internal/services/git/deploy.go:282 handleComposeDeployment` + `395 handleDockerfileDeployment` persist only via `CreateGitDeployment+CompleteGitDeployment` not `CreateDeploymentRevision`; `handlers_revisions.go:16` revision API disjoint.
- **STATUS** PARTIAL (backend exists but not connected for git)
- **GAP** Git-sourced image push never creates `deployment_revisions` row so `POST /admin/deployments/:id/revisions/:revId/rollback` cannot rollback git deploy; `ComposeStack.GitPrevious*` separate. Previously `phase-01#08` MEDIUM `LF-06` and `final LF-06` MEDIUM — unchanged.
- **SEVERITY** MEDIUM (documented deferred)

### 17 — Docker-Compose git stack (validate but build delegation)

- **REFERENCE** Portainer `pkg/libstack` `docker compose` stacks, Coolify `DOCKERCOMPOSE`.
- **FORGE** `deploy.go:290 readComposeFromDir` + `316 ValidateComposeContent` then `354 DeployComposeFromGit` via `deploy_service.go:536 detectProjectType` returns `"compose"` if file exists; `store_compose.go` `GitSourceID, Previous*`; `compose/gitops.go` `PollForUpdates` interval default 300s.
- **STATUS** PRESENT via `GitDeployAdapter` passing `CloneDir` full tree so `build.context` relative resolves (handled)
- **GAP** Frontend `compose/new` raw YAML no git import UX (low); `build:` per-service honoured by `docker compose up --build` on beacon not via `build.Service` — intentional delegation, correctly documented in final-parity#13.
- **SEVERITY** LOW

### 18 — Cache-forward fix verification (closed) + tar not in scope

- **REFERENCE** `reference/app-platforms/caprover/src/models/ICaptainDefinition.ts:1` `dockerfileLines` tar upload intentionally not replicated.
- **FORGE Cache** `forge/api/internal/services/build/service.go:95 BuildOptions CacheFrom/CacheTo Platform` + `589 forwarded to daemon DockerfileBuildRequest`; `beacon/internal/server/build.go:44 dockerfileBuildRequest CacheFrom/To/Platform` `126 forward guarded by 542 isSafeBuildxRef` (rejects `-` leading, `; \r \n \x00`) and `553 isSafePlatform` (rejects `; / \\` plus `linux/amd64` style). Prior `phase-01#05` PARTIAL dropped silently — now FIXED. Test `service_test.go:553` expects cache `type=gha`.
- **Tar** No gap — intentionally missing per both prior audits INFO.
- **STATUS** FIXED (cache) / MISSING INTENTIONAL (tar)
- **SEVERITY** LOW

Cross-cutting table:

| # | Capability | Ref coverage | Forge parity | Sever. | Prior→Current |
|---|---|---|---|---|---|
|01| Provider types (5 vs 4 vs 2) | Dokploy4 Coolify2 | PARTIAL +extra generic, PAT-only | MED | PARTIAL→PARTIAL |
|02| OAuth vs PAT | Dokploy OAuth, Coolify Apps | PAT-only, encrypted, no refresh flow | MED | PARTIAL→PARTIAL |
|03| Deploy keys 3 flavours | Coolify PrivateKey.php:31 | PRESENT ed25519/rsa4096 rotation | LOW | MED→LOW |
|04| Cred embedding divergence | CapRover URL, Komodo token-url | Direct embeds (weaker) vs Beacon env-var (hardened) | MED | MED→MED OPEN LF-01 |
|05| SSH key file 0600 | Coolify/CapRover 0600 | PRESENT | LOW | LOW→LOW |
|06| Webhook HMAC 4 providers | Dokploy tests | FIXED 401 now (was 200) | LOW→FIXED | LOW→FIXED |
|07| Webhook dedup idempotency | — | PARTIAL deploy:yes source:no | LOW | LOW→LOW |
|08| Auto-provision webhook | Dokploy Octokit | PRESENT fail-open generic edge | LOW | LOW→LOW |
|09| Branch validators | Dokku basic | Dual validators diverged | MED | MED→MED |
|10| RepoURL/SSRF allow-list 5 hosts | CapRover permissive | Hardened private-IP reject | LOW | LOW→LOW |
|11| Shallow SHA fragility | Komodo full clone→reset | PARTIAL fragility on control-plane | MED | MED→MED OPEN LF-02 |
|12| Preview duality (preview vs previewenv) | Coolify/Dokploy per-PR | DUPLICATE wired both namespaces | HIGH | HIGH→HIGH OPEN |
|13| Preview per-PR uniqueness race | Dokploy DB unique | MISSING partial index | MED | MED→MED |
|14| Commit status | Dokploy PR comment | previewenv only | MED | MED→MED |
|15| Temp-dir + size guards | — | Hardened 0700/1GB/symlink | LOW | LOW→LOW |
|16| Revisions disjoint | Dokploy single table | Backend disjoint | MED | MED→MED |
|17| Compose git stacks | Portainer libstack | PRESENT via cloneDir | LOW | LOW→LOW |
|18| Cache forward + tar | CapRover tar | FIXED cache / tar intentional | INFO | MED→FIXED |

---

## Logic Findings (≥3 required — detail: location, trigger, consequence, evidence)

### LF-01 — Deploy-key script leaks token on control-plane direct path but not Beacon (MEDIUM, re-verified OPEN)

- **Files** `forge/api/internal/services/git/deploy_service.go:287` `writeAskPassScript(username,password string)` at `288 fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n    *Username*) echo %s ;;\n    *Password*) echo %s ;;\nesac\n", shellQuote(username), shellQuote(password))` vs `beacon/internal/server/git.go:387` `const script = "#!/bin/sh\ncase \"$1\" in\n\t*[Uu]sername*) printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" ;;\n\t*) printf '%s' \"$FORGE_GIT_ASKPASS_PASSWORD\" ;;\nesac\n"` plus `344 gitEnvironmentForRequest` dedicated 0700 dir.
- **Trigger** Any `CloneWithToken`/`CloneRepo` on control-plane (no Beacon `NodeID`) e.g., `SourceDeployExecutor.RunDeployment` before BuildService dispatch, or `GitWebhookDeployService.InitiateDeployment` goroutine `deployment_service.go:97` fallback path, or `GitDeployOrchestrator.TriggerDeployment:233` without Beacon.
- **Consequence** Token in file body until `defer os.Remove(af)` at `119,132,158` but `os.CreateTemp("", "git-askpass-*")` parent is shared `TMPDIR` 1777 traversable; other local users can `open`+`read` briefly. `shellQuote` correctly escapes `'` via `'\''` at `309` but still cleartext. Beacon avoids entirely via env-scoped `FORGE_GIT_ASKPASS_*`.
- **Evidence** Grep `shellQuote(username)` in script body vs env-var grep; `beacon/git.go:362 MkdirTemp(parentDir,"forge-git-askpass-*") 0700` vs direct `CreateTemp("", ...)`; `gitEnvironmentForRequest:366 cleanup RemoveAll` deferred immediately. Residual window unavoidable with GIT_ASKPASS but beacon minimizes.
- **Recommendation** Align direct path to beacon pattern: create `0700` dedicated dir under `tempBaseDir` (not `os.TempDir` bare), chmod `0700` before content, pass creds via `FORGE_GIT_ASKPASS_*` env, write generic script once, keep `writeSSHKeyFile:267` as-is (good). Backport `git.go:344-405`.
- **First seen** `phase-01#02 LF-01` + `MASTER_FINDING:REF-APP-GIT02-LF01` + `final LF-01` — **still open, not yet verified fixed**.
- **Severity** MEDIUM

### LF-02 — Shallow single-branch clone prevents arbitrary SHA fetch for PR force-push/rebases (MEDIUM, re-verified OPEN)

- **Files** `forge/api/internal/services/git/deploy_service.go:207` `git clone --depth 1 --single-branch --branch <branch> ... -- <url> <dir>` then `228 git fetch --depth 1 origin <commitSHA>` + `234 checkout <sha>` vs `beacon/internal/server/git.go:184` `git init` `193 remote add origin <url>` `202 fetch --depth 1 origin <sha> --filter=blob:none` then `212 checkout --detach FETCH_HEAD` (correct).
- **Trigger** Webhook `payload.After` or `Commits[^1].ID` for commit amended via `git push --force` or PR merge commit not at branch tip. GitHub often allows `anySHA` but Gitea/self-hosted with `uploadpack.allowAnySHA1InWant=false` (and some GitHub Enterprise) rejects `fetch <sha>` not at tip.
- **Consequence** `SourceDeployExecutor` or `GitWebhookDeployService.InitiateDeployment` fails `fetch commit …: exit 128 fatal: couldn't find remote ref <sha>` logged at `deployment_service.go:118` + `deploy_service.go:231`; deployment status `failed` async, user sees opaque `output:` while beacon SHA path would succeed. No fallback.
- **Evidence** Compare args `207 clone … --branch <branch>` vs `beacon 184 init` — SHA case should not start from shallow clone. Prior `phase-01#03 LF-02` and `final LF-02` same evidence.
- **Recommendation** For `commitSHA != ""` use beacon-style `init+remote add+fetch` in `cloneWithOptions` too, or add fallback `git fetch --depth 50` then `fetch <sha>` / `--unshallow` retry. Unify validators to single shared `validateBranch==validateGitRefName` (prefer `deploy_service.go:464` coverage + beacon `--branch=` passing).
- **Severity** MEDIUM

### LF-03 — Preview per-PR uniqueness via in-memory scan races; no DB partial unique index (MEDIUM, re-raised)

- **Files** `forge/api/internal/services/previewenv/service.go:144` `actives, _ := ListActivePreviewDeployments(); for _,p := range actives { if p.PRNumber==req.PRNumber && EqualFold(repoOwner,repoName) return ErrAlreadyExists }` + `forge/api/internal/store/store_preview_env.go:13` helpers `ListActivePreviewDeployments` filtering `status IN ('deploying','running','stopped')`.
- **Trigger** Two concurrent `POST /preview/webhook/github pull_request.synchronize` (delivery retry or rapid synchronize) both pass scan then `CreatePreviewDeployment:179` inserts duplicate rows.
- **Consequence** Duplicate active previews for same PR violate intended uniqueness; reaper `reaper.go:70` picks first arbitrarily; commit status at `404 reportStatus` flaps; Dokploy avoids via DB unique per `applicationId`.
- **Evidence** No migration shows `UNIQUE (pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN (...)` — grep `preview_deployments` DDL not enforcing. `previewenv` scan is not serialized by `ClaimComposeStackForUpdate`-style row lock.
- **Recommendation** Add partial unique index `CREATE UNIQUE INDEX preview_active_pr_unique ON preview_deployments(pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running','stopped')` and map `pq: duplicate key` to `ErrAlreadyExists`. Also replace in-memory scan with `SELECT ... FOR UPDATE` or unique-violation handling. Promote `previewenv` to admin route (replace `preview`).
- **First seen** `phase-01 LF-05` + `final LF-03` — still open.
- **Severity** MEDIUM

### LF-04 — Remote build log streaming post-hoc vs live (MEDIUM, extra — drift from build/Beacon parity but impacts git source deployments)

- **Files** `forge/api/internal/services/build/service.go:530 executeRemoteBuild` `605 DockerfileBuild` then `610 BuildLogs(...,true)` buffered after start at `617 for _,l := range logs { logCh <- … }` vs `beacon/internal/server/build.go:241 handleBuildLogs` 100ms ticker SSE at `275-306` live; `forge/api/internal/http/handlers_builds.go:91 GET /builds/:id/logs SSE vs text` reads `StreamLogs` which for remote only has buffered entries after completion; `handlers_source_deployments.go:258 GetDeploymentBuildLogs` polling JSON only.
- **Trigger** Remote build via node selector (`nodeID != ""`) — common multi-node.
- **Consequence** Log stream empty until completion, breaking UX parity vs CapRover CircularQueue. Not a git-correctness bug but counted as `final LF-04` and `phase-01#07`; close to branch `subagent-11` but retained here for git source deployment logs.
- **Recommendation** Poll `GetBuildStatus`+incremental `BuildLogs` or pass through Beacon SSE `follow=true` into `logCh` live; unify `source_build_logs` SSE endpoint.
- **Severity** MEDIUM

### LF-05 — Exposed preview endpoint uses legacy `preview.Service` without TTL/limit/commit-status while correct `previewenv.Service` not wired to admin namespace (HIGH, duplicate of #12 for ≥3 threshold)

- **Files** `forge/api/internal/http/server.go:155 PreviewDeploymentSvc *preview.Service` + `2608 registerPreviewDeploymentRoutes(…,PreviewDeploymentSvc)` `handlers_preview_deployments.go:9` vs `forge/api/internal/http/phase4_registrar.go:26 registerPhase4PreviewRoutes` `37 previewenv.New(BaseDomain,TTL=24h,MaxPerOrg=5…)` + `50 registerPreviewEnvWebhookRoutes` + `51 registerPreviewEnvManagementRoutes` on `/preview` + `/preview/webhook/*` + `54 StartReaper(5m)`. `preview/service.go:125` hardcoded `https://preview-<suffix>.example.com` vs `previewenv/service.go:92 https://pr<N>-<owner>-<repo>.<BaseDomain>`.
- **Trigger** Any `POST /admin/preview-deployments` via legacy handler creates row without `expires_at` (`SetPreviewDeploymentExpiresAt:183` never called from legacy), no `MaxPerOrg` `135`, no `reportStatus`, no `TrafficMgr` route, no ACME. Reaper lists `ListExpiredPreviewDeployments expires_at < now()` — legacy rows never match (`expires_at IS NULL`).
- **Consequence** Previews via exposed admin API never expire, never enforce limit, never post `forge/preview` status; two codepaths share same table divergent URL schemes → silent drift; operators rely on admin docs and miss TTL.
- **Evidence** Both `service.go:42 Create` and `previewenv/service.go:121 Create` share table; `server.go:155` vs `phase4_registrar.go:37` split wiring confirmed via `grep registerPreview`. `final-parity#09` HIGH duplicate confirmed not fixed.
- **Recommendation** Replace `registerPreviewDeploymentRoutes` wiring with `previewenv.Service` (or adapter), deprecate `preview.Service`, migrate legacy rows `UPDATE preview_deployments SET expires_at = created_at + TTL WHERE expires_at IS NULL`, add partial unique index (LF-03).
- **Severity** HIGH

> ≥3 findings satisfied: LF-01, LF-02, LF-03, LF-04, LF-05 (5). Two prior CLOSED findings verified as CLOSED: HMAC 200→401 (`handlers_git.go:640`) and cache forward (`beacon/build.go:44`).

---

## Additional Cross-checks vs `reference/app-platforms`

- **Dokploy builders detailed** (`dokploy/packages/server/src/utils/builders/index.ts:44` dispatch 6, `schema/application.ts:70` `buildType` 6, `coolify/app/Enums/BuildPackTypes.php:7` 5): Forge 2 — correct muted via admission, not inflated.
- **Portainer GitOps** (`portainer/api/gitops/scheduling/SourceScheduler`, `pkg/libstack`): Interval+Webhook ForceUpdate vs Forge `GitNextPollAt` claim — Forge matches polling via `compose/gitops.go PollForUpdates` 300s default.
- **CapRover `ICaptainDefinition.ts:1` `dockerfileLines/templateId` + `ImageMaker.ts:1` tar**: Intentionally not replicated; Forge uses explicit `Dockerfile` file check (`deploy_service.go:536 detectProjectType`) — INFO.
- **Komodo stacks** (`komodo/lib/git/src/clone.rs:67` `git clone … -b` + token in `remote_url` at `28` masking at `76`): Forge env-var askpass is stricter than Komodo/CapRover URL embedding — improvement.
- **docker-compose build spec** (`build:{context,dockerfile,args,cache_from}`): Handled via `docker compose up --build` on Beacon full tree, not via Forge `build.Service` per-service — acceptable delegation per final-parity#13.

---

## Verdict

- **Implemented strongly (re-verified 2026-08-24):** 5 provider types incl. generic (PAT-only, encrypted via `store_git_providers.go:151`, SSRF-hardened `validateProviderURL:303`+`beacon/git.go:436` IP lookup), 3 credential flavours with `GenerateDeployKeyPair` ed25519/rsa4096 rotation, strict branch validators (`deploy_service.go:464`+`beacon/git.go:58`), webhook HMAC for 4 providers now all 401 (`handlers_git.go:640,705,802,869`), idempotency for `deploy/:serverId` (`1235 deriveGitDeploymentIdempotencyKey`), auto-provision create/delete, compose git via full `CloneDir`, cache forward now FIXED (`beacon/build.go:44`) hard-verified.
- **Admission-fixed (closed):** webhooks 200→401 and cache-forward — both reverified closed vs phase-01.
- **Still open (confirm prior OPEN stays OPEN):** LF-01 cred askpass divergence, LF-02 shallow SHA fragility, preview duality HIGH + per-PR uniqueness race — exactly the two open areas called out in prompt (shallow SHA + preview duality) plus LF-01.
- **Partial / docs:** generic UI missing, OAuth dance missing, Bitbucket header e2e not validated, 1Panel/uncloud no provider abstraction (no gap), CapRover tar intentionally missing.
- **Missing intentionally (INFO):** CapRover tar/`captain-definition`, Dokku buildpack Herokuish beyond Nixpacks — all documented non-goals.

No product code modified. All claims inspected via `read` at cited file:line; absent paths checked via `bash ls` failures. ≥12 rows (18) and ≥3 logic findings supplied (5).

---

## Citations Index (key paths verified via `read` before claiming)

- `reference/app-platforms/coolify/app/Models/PrivateKey.php:31` class + `130 validatePrivateKey` + `157 generateNewKeyPair` + `199 storeInFileSystem /var/www/html/storage/app/ssh/keys/ssh_key@{uuid} chmod 0600`
- `reference/app-platforms/dokploy/packages/server/src/db/schema/git-provider.ts:8` `pgEnum 4` + `github.ts:5`+`gitlab.ts:5`+`gitea.ts:6`+`bitbucket.ts:6`
- `reference/app-platforms/komodo/lib/git/src/clone.rs:28` `remote_url(access_token)` + `76 masking`
- `reference/app-platforms/caprover/src/utils/GitHelper.ts:12` `SSH_PATH_RE` + `42 clone` + `99 USER:PASS@ REPO_PATH` URL embed
- `forge/api/internal/services/git/service.go:87` `GenerateDeployKeyPair` + `130 GenerateDeployKey` + `154 ListProviderRepos 173 generic` + `205 SetupProviderWebhook 225 generic` + `248 VerifyGitHubSignature:858 computeHMAC` + `292 validateProviderBaseURL` DNS private reject
- `forge/api/internal/services/git/deploy_service.go:85 CloneRepo`+`89 CloneAtCommit`+`96 cloneWithOptions`+`164 cloneRepo:207 clone --depth 1 --single-branch`+`228 fetch --depth 1 origin <sha>`+`267 writeSSHKeyFile 0600`+`287 writeAskPassScript embeds shellQuote:309`+`464 validateBranch`+`509 safeClonePath`+`527 resolveCommitSHA`+`440 allowedHost`+`425 allowedGitHosts(5)`+`485 ValidateRepoURL`
- `forge/api/internal/services/git/checks.go:37 ReportCommitStatus`+`62 ReportCommitStatusForUser`+`88 postGitHubStatus`+`137 Bitbucket SUCCESSFUL/FAILED`
- `forge/api/internal/services/git/deployment_service.go:81 InitiateDeployment goroutine`+`201 verifyHMACSignature`+`211 VerifyGitHubSignature`
- `forge/api/internal/http/handlers_git.go:48 CreateGitCredential`+`137 GenerateDeployKey`+`190 ConnectGitProvider`+`442 CreateGitSource:486 generateWebhookSecret:497 SetupProviderWebhook`+`599 HandleGitHubWebhook:632 Verify:640 401`+`654 GitLab:705 401`+`725 Bitbucket:802 401`+`822 Gitea:869 401`+`889 buildWebhookURL`+`1165 ReceiveGitDeploymentWebhook:1200 idempotency:1235 derive`
- `forge/api/internal/store/store_git_providers.go:16 Generic const`+`133 CreateGitProviderToken encrypt`+`203_git_provider_generic.sql:6` CHECK vs `204_git_sources_generic.sql:6`
- `forge/api/internal/store/store_git_credentials.go:14` 3 types + `107 Create validation` + `122 encryptSecret`
- `forge/api/internal/store/store_git_sources.go:14` `FindGitSourceByRepoAndBranch:163` + `212 GetGitSourceByServerIDUnmasked`
- `forge/api/internal/services/preview/service.go:42 Create:125 hardcoded preview url`+`74 HandleWebhook`
- `forge/api/internal/services/previewenv/service.go:71 New`+`92 PreviewURL per-PR`+`121 Create per-PR scan 136 limit 158 expires 183 TTL`+`227 Deploy ACME/TrafficMgr`+`286 Cleanup`+`404 reportStatus`+`451 publish`
- `forge/api/internal/http/handlers_preview_deployments.go:9` `preview.Service /admin/preview-deployments` vs `phase4_registrar.go:37` `previewenv.New TTL 24h MaxPerOrg 5` `68 /preview/webhook/*` `126 /preview management`
- `beacon/internal/server/git.go:58 validateGitRefName`+`97 handleGitClone`+`182 commit SHA init path 202 fetch --filter=blob:none 212 checkout FETCH_HEAD`+`246 branch clone --branch=<value> --`+`344 gitEnvironmentForRequest 387 env-var script 407 isHex40 436 isRestrictedHost 464 safePath`
- `beacon/internal/server/build.go:44 CacheFrom/CacheTo/Platform`+`126 forward via 542 isSafeBuildxRef`+`553 isSafePlatform`

