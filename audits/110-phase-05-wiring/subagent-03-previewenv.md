# Subagent 03 — PreviewEnv TTL / Reaper / Commit-Status Wiring (110-05-03 of 110)

**Date:** 2026-08-24  
**Agent:** 110-05-03 of 110 — Phase 05 Agent 03/10 (all 10 run in parallel)  
**Focus:** Wire previewenv TTL / reaper / commit-status (currently legacy `preview` wired, correct `previewenv` not)  
**Reconciles:** `audits/MASTER_FINDING_INDEX.md:27` REF-APP-GIT09-LF07, `FINAL_PARITY_AUDIT.md:86 AP-15`, `107 GB-09`, `110 GB-12`, `audits/110-phase-02-context/subagent-04-git-build-confirm.md:3.8-3.9`, `phase4_registrar.go:37` vs `server.go:155`

---

## 1. Live Inspection (file:line verified before edit)

| Prompt target | Live file:line | Finding |
|---|---|---|
| `preview/service.go:42` legacy Create | `forge/api/internal/services/preview/service.go:42` `func (s *Service) Create(ctx, serverID string, req *store.PreviewDeployment) (*store.PreviewDeployment, error)` at `44 suffix := uuid.NewString()[:8]` and `125 PreviewURL=https://preview-<suffix>.example.com` hardcoded, no TTL, no per-PR dedup, no per-org limit, no `expires_at`, no `reportStatus` | **DUPLICATE — exposed** |
| `previewenv/service.go:121` correct Create | `forge/api/internal/services/previewenv/service.go:121` `func (s *Service) Create` enforces `136 CountActivePreviewDeploymentsForOrg >= MaxPerOrg → ErrOrgLimitReached`, `144 ListActivePreviewDeployments scan → ErrAlreadyExists`, `158 expires := now.Add(s.opts.TTL)`, `183 SetPreviewDeploymentExpiresAt`, `404 reportStatus` via `git/checks.go:62` | **CORRECT — hidden** |
| `phase4_registrar.go:37` correct wiring | `forge/api/internal/http/phase4_registrar.go:37` `previewenv.New(cfg.Store, Options{BaseDomain: previewEnvString("PREVIEW_DOMAIN","env.example.com"), TTL: 24h, MaxPerOrg:5, Logger, Publisher, GitService, AcmeService, TrafficMgr, DomainSvc, PanelURL})` + `51 registerPreviewEnvManagementRoutes` on `protected /preview` + `54 svc.StartReaper(5*time.Minute)` | Correct but **not on admin namespace** |
| `server.go:155` legacy wiring | `forge/api/internal/http/server.go:157` `PreviewDeploymentSvc *previewsvc.Service` + `2660 registerPreviewDeploymentRoutes(protected, cfg, cfg.PreviewDeploymentSvc, ...)` mounting `protected /admin/preview-deployments` via `handlers_preview_deployments.go:9` | **Legacy on admin namespace** |
| `handlers_preview_deployments.go:9` legacy handler | `forge/api/internal/http/handlers_preview_deployments.go:9` `func registerPreviewDeploymentRoutes(..., svc *preview.Service)` with `14 protected.Group("/admin/preview-deployments", ...)` and `16 svc.ListAll`, `53 svc.Create`, `69 Deploy`, `76 Cleanup`, `83 UpdateStatus` — all on `preview.Service` | Uses legacy |
| `store_preview_env.go:17` TTL helpers | `forge/api/internal/store/store_preview_env.go:17 SetPreviewDeploymentExpiresAt`, `32 ListPreviewDeploymentsWithExpiry`, `54 CountActivePreviewDeploymentsForOrg`, `66 ListExpiredPreviewDeployments` (`expires_at < now AND status IN ('deploying','running')`), `94 ListReapablePreviewDeployments` | Correct store, but **never hit by legacy rows** (`expires_at IS NULL` never matches `ListExpired`) |
| `store_deployment_history.go:195` shared table | `forge/api/internal/store/store_deployment_history.go:195 CreatePreviewDeployment` inserts without `expires_at`; both services share `preview_deployments` table (`119_deployments_rollbacks.sql:27`) | Shared table divergent semantics |
| `migrations/119 + 180` | `119_deployments_rollbacks.sql:27 CREATE TABLE preview_deployments` + `180_preview_ttl.sql:5 ALTER ADD expires_at` + `8 idx_expiry (expires_at,status)` + `11 idx_cleaned (cleaned_at) WHERE cleaned_up` | **No per-PR unique index** before fix |
| `reaper.go:21` reaper | `forge/api/internal/services/previewenv/reaper.go:21 StartReaper(ctx, interval)` with `40 ticker 5m`, `60 ListExpiredPreviewDeployments` → `Cleanup`, `73 ListReapablePreviewDeployments` → `Delete` | Live but only reaps `previewenv` rows |
| `checks.go:62` commit status | `forge/api/internal/services/git/checks.go:62 ReportCommitStatusForUser` → `37 ReportCommitStatus` per-provider (`88 GitHub`, `108 GitLab`, `137 Bitbucket`, `168 Gitea`) and `previewenv/service.go:404 reportStatus` calls it with `forge/preview` context and `PanelURL` target | Only via `previewenv` |
| `forge/web/app/admin/preview-deployments/page.tsx` | List page at `49 fetchJSON("/admin/preview-deployments")`, no TTL/expires, no config, no reaper note | UI missing lifecycle |
| `forge/web/app/admin/preview-deployments/[id]/page.tsx` | Detail page at `43 fetchJSON("/admin/...")`, no expires, no TTL/limit | UI missing lifecycle |
| `forge/web/lib/api/preview-deployments.ts` | Type at `3 PreviewDeployment` missing `expiresAt`, source limited to `github|gitlab`, lib still hits `/admin/...` only | UI/API mismatch |

**Verdict before fix:** **STILL BROKEN — HIGH** (`GB-09`) — legacy `preview` wired on admin, correct `previewenv` wired on `/preview` but not admin; TTL never expires for exposed rows; reaper only touches `previewenv` rows; commit status only via unused `previewenv`; per-PR race (no DB index) at `previewenv/service.go:144` scan; UI shows no TTL/limit/reaper.

---

## 2. Disposition: What Was Fixed (wire correct one)

### 2.1 Canonical vs alias (keep both, correct wins)

| Route | Before | After | Handler |
|---|---|---|---|
| **Canonical** `GET /api/v1/preview/` | `phase4_registrar.go:126 preview.Get("/", ListWithExpiry)` via `previewenv.Service` (correct) | **Unchanged — canonical stays `/preview`** (phase4_registrar.go:121) | `previewenv/service.go:214 ListWithExpiry` (includes `expires_at`) |
| `GET /preview/config` | returns `ttl` as `time.Duration` int (ns) | **Fixed to `ttl.String()` / `retain.String()`** (phase4_registrar.go:149-150) so UI can `formatTTL` | `previewenv/service.go:455 Config()` |
| `GET /preview/server/:id`, `GET /preview/:id`, `POST /preview/:id/deploy`, `POST /preview/:id/cleanup`, `DELETE /preview/:id` | already via `previewenv` | **Unchanged** | `previewenv/service.go:207 List`, `199 Get`, `227 Deploy` (ACME+traffic+domain), `286 Cleanup` (withdraw route), `318 Destroy` |
| Webhooks `POST /preview/webhook/{github,gitlab,bitbucket,gitea}` | `phase4_registrar.go:68-71` via `previewenv` verifiers (`providers.go:28 verifyGithub` etc.) → `webhook.go:55 upsertAndDeploy` + `142 cleanupForPR` | **Unchanged** | `previewenv/webhook.go` + `providers.go` |
| **Alias** `GET /admin/preview-deployments/*` | `handlers_preview_deployments.go:9` via `*preview.Service` (legacy, no TTL) on `protected.Group("/admin/preview-deployments")` | **Switched to `*previewenv.Service` + Deprecation headers** (`handlers_preview_deployments.go:9-18`) — same semantics as canonical so TTL/reaper/commit-status active on alias | `previewenv/service.go` + `store_preview_env.go` |
| Alias `POST /admin/preview-deployments/` (manual create) | legacy `Create` | **Now `previewenv.Create` with TTL/limit/per-PR** (handlers:53) → `respondStoreError` maps `ErrAlreadyExists`/`ErrOrgLimitReached` to 409 via `errors.go:33` | `previewenv/service.go:121` + `isPreviewUniqueViolation` |
| Alias `GET /admin/preview-deployments/config` | did not exist | **Added** (handlers:30) proxies `svc.Config()` as strings | same as canonical |
| Alias `DELETE /admin/preview-deployments/:id` | did not exist | **Added** (handlers:96) → `svc.Destroy` | `previewenv/service.go:318` |
| Alias `POST /admin/preview-deployments/:id/status` | `svc.UpdateStatus` (legacy direct) | **Retained for backward compat** but now delegates to `svc.Cleanup` when `cleaned_up` else `store.UpdatePreviewDeploymentStatus` (handlers:104) + sets `Deprecation` header on group | `store_deployment_history.go:267` |
| Deprecation | none | **Alias group sets `Deprecation: true`, `Sunset: Thu, 31 Dec 2026 23:59:59 GMT`, `Warning: 299 - "Deprecated: use /api/v1/preview instead"`** (handlers:14-18) | RFC 8594 |

**Why alias kept:** `FINAL_PARITY_AUDIT.md:544 DUPLICATE` requires deprecate-not-delete for one release so existing `forge/web` and external API clients on `/admin/preview-deployments` keep working while they migrate. The alias now has identical lifecycle guarantees as canonical.

### 2.2 Server wiring (`server.go:155` legacy → `previewenv`)

* **Config:** `forge/api/internal/http/server.go:157` `PreviewDeploymentSvc *previewsvc.Service` kept as deprecated (comment) and **added** `160 PreviewEnvService *previewenv.Service` (new canonical singleton). Import `previewenv` added at `75`.

* **Alias registration:** `server.go:2873` legacy `registerPreviewDeploymentRoutes(..., cfg.PreviewDeploymentSvc)` **replaced** with lazy singleton block (2878-2899):

  ```go
  previewEnvSvc := cfg.PreviewEnvService
  if previewEnvSvc == nil && cfg.Store != nil {
      previewEnvSvc = previewenv.New(cfg.Store, previewenv.Options{
          BaseDomain: previewEnvString("PREVIEW_DOMAIN","env.example.com"),
          TTL: previewEnvDuration("PREVIEW_TTL", 24*time.Hour),
          MaxPerOrg: previewEnvInt("PREVIEW_MAX_PER_ORG",5),
          Logger: cfg.Logger, Publisher: cfg.EventRegistry,
          GitService: cfg.GitService, AcmeService: cfg.AcmeService,
          TrafficMgr: cfg.TrafficManager, DomainSvc: cfg.DomainService, PanelURL: cfg.PanelURL,
      })
      cfg.PreviewEnvService = previewEnvSvc // reused by phase4 registrar
  }
  registerPreviewDeploymentRoutes(protected, cfg, previewEnvSvc, ...)
  ```

  Helpers `previewEnvString/Duration/Int` are visible from `phase4_registrar.go` (same `http` package). If `main.go` already wired `PreviewEnvService`, the lazy block reuses it; otherwise it constructs with identical options to the registrar so both entrypoints share TTL/limit.

* **Registrar reuse:** `forge/api/internal/http/phase4_registrar.go:26` comment updated and `37 svc := cfg.PreviewEnvService; if svc == nil { svc = previewenv.New(...) ; cfg.PreviewEnvService = svc }` — canonical and alias now share one `*previewenv.Service` and one reaper (no duplicate tickers).

* **Main wiring:** `forge/api/cmd/api/main.go:90` imports `previewenv`, `311 var previewEnvSvc *previewenv.Service`, `1678 PreviewEnvService: previewEnvSvc` in `http.Config`, and `1632 previewEnvSvc = previewenv.New(db, Options{BaseDomain: env("PREVIEW_DOMAIN"), TTL: envDuration("PREVIEW_TTL",24h), MaxPerOrg: envInt(5), Logger: slogLogger, Publisher: eventRegistry, GitService: gitSvc, AcmeService: acmeSvc, TrafficMgr: tmSvc, DomainSvc: domainSvc, PanelURL: env("PANEL_URL")})` (constructed before `http.Config` after `tmSvc`/`domainSvc`/`acmeSvc` are ready). Legacy `previewDeploySvc = previewsvc.New(db, outboxPub)` retained.

### 2.3 DB partial unique index (per-PR)

* **Migration `216_preview_per_pr_unique.sql`:** `forge/api/migrations/216_preview_per_pr_unique.sql`

  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_deployments_pr_unique
      ON preview_deployments (pr_number, lower(repo_owner), lower(repo_name))
      WHERE status IN ('deploying', 'running', 'stopped');
  UPDATE preview_deployments
      SET expires_at = created_at + INTERVAL '24 hours'
      WHERE expires_at IS NULL AND status IN ('deploying','running','stopped');
  ```

  Closes the race at `previewenv/service.go:144` where `ListActivePreviewDeployments` scan is non-atomic; two concurrent webhook deliveries can both pass the scan and insert duplicate active rows. The index makes it DB-atomic; the service maps the violation via new `isPreviewUniqueViolation` (service.go:462-470) to `ErrAlreadyExists` (409) matching `errors.go:33` domainStatus.

  *SQLite dialect* at `forge/api/migrations/sqlite/216_preview_per_pr_unique.sql` uses `datetime(created_at,'+24 hours')` for `sqliteCompatibleMigration` parity (migration runner `store/migration.go:88` replaces `TIMESTAMPTZ` etc., but not `INTERVAL`).

* **Backfill:** Legacy rows (`expires_at IS NULL`) from `preview/service.go:42` now get `expires_at = created_at + 24h` so `store_preview_env.go:75 ListExpiredPreviewDeployments` (`expires_at < now AND status IN ('deploying','running')`) will eventually reap them. Without this, the reaper (`reaper.go:60`) would never find `expires_at IS NULL` rows.

* **Prefix check:** `forge/api/internal/store/migration.go:245 migrationPrefix` → `216` not previously used (`215_tenant_scoping_additive.sql` was max); `go test -run TestNoDuplicatePrefixes` passes.

### 2.4 Reaper (cron for TTL expiry)

* **Already at** `phase4_registrar.go:54 svc.StartReaper(cfg.BackgroundContext, 5*time.Minute)` — `reaper.go:21` ticker `5m`, `53 runOnce` does:

  1. `66 ListExpiredPreviewDeployments(now)` → `65 Cleanup` per row (sets `cleaned_up`, withdraws traffic route if `TrafficMgr` set, publishes, reports commit status `failure`), then
  2. `73 ListReapablePreviewDeployments(now - RetainCleaned)` → `DeletePreviewDeployment` per row.

  Because canonical and alias now share one `*previewenv.Service` singleton (see 2.2), there is exactly one reaper goroutine (started by the registrar). `main.go` does not start a second; `server.go` lazy path stores back into `cfg.PreviewEnvService` so registrar reuses.

* **Verified:** `reaper.go:14 type reaper struct` with `done chan`, `30 loop` select on `ctx.Done()` vs `ticker.C`, `53 runOnce` logs at `Warn` if store errors. Interval `5m` matches task "cron for TTL expiry" and `FINAL_PARITY_AUDIT.md:12.3` installer 6-workflow / previewenv TTL requirement.

### 2.5 Commit status via `checks.go`

* **Wiring:** `previewenv/service.go:54 PanelURL`, `57 GitService *git.Service` passed through `Options` from `main.go:1640` / `server.go:2891` / `phase4_registrar.go:43-47`. On `Create` (service.go:195) `reportStatus(pending, "preview environment is being provisioned")`, on `Deploy` success (280) `reportStatus(success, "preview environment is running at "+previewURL)`, on `Cleanup` (312) `reportStatus(failure, "preview environment was cleaned up")`.

* **Implementation:** `service.go:404 reportStatus` → `git.Checks.go:62 ReportCommitStatusForUser(ctx, *p.CreatedBy, providerType(p.Source), p.RepoOwner, p.RepoName, p.CommitSHA, st)` where `st` is `CommitStatus{State, Context:"forge/preview", TargetURL: PanelURL+"/environments/previews?preview="+p.ID or p.PreviewURL}`. `checks.go:37 ReportCommitStatus` validates token, `validateProviderBaseURL`, then per-provider `88 postGitHubStatus`, `108 postGitLabStatus`, `137 postBitbucketStatus`, `168 postGiteaStatus`. Fail-open: if `GitService` nil or `CreatedBy` nil/empty or no token, it silently returns (service.go:405-409).

* **Result:** Every preview lifecycle transition now surfaces on the PR as a GitHub/GitLab/Bitbucket/Gitea status check `forge/preview` (GB-12 FIXED).

---

## 3. UI — TTL, limit, status, reaper

### 3.1 Types (`forge/web/lib/api/preview-deployments.ts:1`)

* `PreviewDeployment` extended: `source` now `"github"|"gitlab"|"bitbucket"|"gitea"|"manual"` (was 2), added `expiresAt?: string` (from `store_deployment_history.go:52` + `store_preview_env.go:24` `ListWithExpiry`), kept `cleanedAt`.
* Added `PreviewConfig {baseDomain, ttl, retain, maxPerOrg}` mirroring `phase4_registrar.go:146 /config`.
* **Canonical vs alias:** `fetchPreviewDeployments()` tries `GET /preview` first (canonical) then fallback `/admin/preview-deployments` (alias) — both now return `ListWithExpiry` with `expires_at`. Same pattern for `fetchPreviewDeployment`, `fetchPreviewConfig`, `deployPreview`, `cleanupPreview`, plus new `destroyPreview` (`DELETE /preview/:id` → `DELETE /admin/...` fallback). `createPreviewDeployment` stays on alias (canonical creates via webhooks).

### 3.2 List page (`forge/web/app/admin/preview-deployments/page.tsx`)

* **Data:** `useQuery(["admin","preview-deployments"], fetch /preview || /admin/...)` returns `PreviewDeployment[]` with `expiresAt`; `useQuery(["preview","config"], /preview/config || /admin/.../config)` returns `PreviewConfig` (TTL, retain, maxPerOrg, baseDomain).
* **Lifecycle card (new):** 4-tile grid — **TTL** (`formatTTL(ttl)` default `24h`, note `auto-expire → reaper cleanup`), **Limit** (`activeCount / limit` with `atLimit ? red : nearLimit ? amber : slate`, text `PREVIEW_MAX_PER_ORG` and `409 limit reached` warning), **Retain** (`retain` default `24h`, `cleaned_up → row delete`), **Reaper** (`every 5m`, `cron: TTL expiry + row reaping; commit status via checks.go`). Also shows canonical vs alias badges and `base → pr<N>-<owner>-<repo>.base` hint.
* **Limit banner:** When `activeCount >= limit` shows red `AlertTriangle` banner: `Org limit reached — new preview creations return 409 until a slot is cleaned up (reaper or manual cleanup). Partial unique index idx_preview_deployments_pr_unique also prevents duplicate active PRs (per-PR 409).`
* **Table:** New column **TTL / Expires** — `timeLeft(expiresAt)` (`"<24h" → "5h 12m"`, `">24h" → "2d 3h"`, `expired → "expired — reaper pending"` red, `cleaned_up → cleaned <date>` slate). `safeExternalUrl` still guards `previewUrl`. Existing columns (PR, Title, Branch, Source `Pill`, Status `Pill`, Created) unchanged.
* **Mutations:** `deploy`/`cleanup` try canonical `/preview/:id/...` then fallback alias, invalidate `["admin","preview-deployments"]`.

### 3.3 Detail page (`forge/web/app/admin/preview-deployments/[id]/page.tsx`)

* Extended `PreviewDeployment` with `expiresAt`, fetched via canonical `/preview/:id` → alias fallback.
* Added `configQuery` for `PreviewConfig`.
* New **Lifecycle** card (3 tiles): **TTL** (`timeLeft`, `expires: <date>` + `reaper pending` if expired, `configured TTL: <ttl>`), **Status** (`Pill` + `commit status: forge/preview via checks.go` + `per-PR unique: idx_preview_deployments_pr_unique WHERE active`), **Retain** (`cleaned` date or `not cleaned`, `retain: 24h then row delete — base/limit`).
* Header actions now include **Destroy** (`DELETE /preview/:id` → alias fallback) in addition to Deploy/Cleanup, wired via `useMutation(deleteJSON)`.
* Details grid adds **Expires** (`formatDate(expiresAt) (left)`) and **Reaper** (`5m cron`, `TTL expiry → cleaned_up → row delete`) tiles.

---

## 4. Files Changed (10)

| File | Change | Lines |
|---|---|---|
| `forge/api/internal/services/previewenv/service.go` | Map unique violation to `ErrAlreadyExists` via `isPreviewUniqueViolation` (handles postgres `duplicate key` + sqlite `UNIQUE constraint failed`) at `179-181`; add helper `462 isPreviewUniqueViolation`; keep reportStatus wiring | `179-181`, `462-470`, `455 Config()` unchanged |
| `forge/api/migrations/216_preview_per_pr_unique.sql` | **New** partial unique index + backfill `expires_at` | `9-11` index, `14-16` UPDATE |
| `forge/api/migrations/sqlite/216_preview_per_pr_unique.sql` | **New** sqlite dialect (datetime) | same |
| `forge/api/internal/http/handlers_preview_deployments.go` | **Switch** `*preview.Service` → `*previewenv.Service`; add `Deprecation/Sunset/Warning` middleware; `GET /` → `ListWithExpiry`, `GET /config` added, `POST /` → `previewenv.Create` with `respondStoreError` (409), `POST /:id/deploy`/`cleanup` via `respondStoreError`, `DELETE /:id` → `Destroy`, retained `POST /:id/status` as deprecated compat (maps `cleaned_up` → `Cleanup` else direct store status) | `1-7` imports, `9-18` func+dep, `20-40` GET, `96-100` DELETE |
| `forge/api/internal/http/server.go` | Import `previewenv`, add `PreviewEnvService` field, replace legacy alias registration with lazy canonical singleton block that mirrors `phase4_*` options and stores back into `cfg` | `75`, `160-161`, `2873-2899` |
| `forge/api/internal/http/phase4_registrar.go` | Reuse `cfg.PreviewEnvService` singleton if present, else construct and store back; update comment to document TTL/reaper/commit-status wiring; make `/config` return `ttl.String()`/`retain.String()` for UI | `26-55`, `146-154` |
| `forge/api/cmd/api/main.go` | Import `previewenv`, add `previewEnvSvc` var, set `PreviewEnvService` in `http.Config`, construct canonical `previewenv.New` with `BaseDomain/PREVIEW_DOMAIN, TTL/PREVIEW_TTL 24h, MaxPerOrg/PREVIEW_MAX_PER_ORG 5, Logger, Publisher:eventRegistry, GitService:gitSvc, AcmeService:acmeSvc, TrafficMgr:tmSvc, DomainSvc:domainSvc, PanelURL` before `http.Config` | `90`, `311`, `1632-1645`, `1699` |
| `forge/web/lib/api/preview-deployments.ts` | Extend type with `expiresAt` + 5-source union, add `PreviewConfig`, make fetchers canonical-first (`/preview`) with alias fallback, add `fetchPreviewConfig`, `destroyPreview`, keep `create` on alias | `1-2`, `17-60` |
| `forge/web/app/admin/preview-deployments/page.tsx` | Fetch canonical→alias, fetch config, 4-tile lifecycle card (TTL/limit/retain/reaper + canonical/alias badges + atLimit banner), new TTL/Expires column with `timeLeft` + `reaper pending` | `4-12`, `14-35`, `45-120`, `106-180` |
| `forge/web/app/admin/preview-deployments/[id]/page.tsx` | Same fetch/config, 3-tile lifecycle, Destroy action, Expires + Reaper detail tiles | `5-11`, `14-50`, `60-140` |

**Untouched but verified:** `forge/api/internal/store/store_preview_env.go` (TTL helpers), `forge/api/internal/services/previewenv/reaper.go` (5m cron), `forge/api/internal/services/git/checks.go` (status), `forge/api/internal/http/handlers_preview_deployments_test.go` (nil-service 404 still passes), `store_deployment_history.go` (shared table).

---

## 5. Verification

```bash
go vet ./forge/api/internal/http          # EXIT 0
go vet ./forge/api/internal/services/previewenv  # EXIT 0
go vet ./forge/api/cmd/api                # EXIT 0
go vet ./forge/api/...                    # EXIT 0
go test ./forge/api/internal/http -run TestPreview -v
  TestPreviewDeploymentRoutes_NilService          PASS (4 subtests)
  TestPreviewDeploymentRoutes_NonAdmin            PASS
go test ./forge/api/internal/store -run TestNoDuplicatePrefixes -v  # PASS (216 not duplicate)
npx tsc --noEmit (forge/web)              # EXIT 0 (no new type errors; PreviewDeployment now includes expiresAt)
```

*Manual checks:*

- `curl -i GET /api/v1/admin/preview-deployments` → `Deprecation: true`, `Sunset: Thu, 31 Dec 2026 23:59:59 GMT`, `Warning: 299 ...`, body `data: [...]` with `expiresAt` populated (TTL `created_at+24h` even for legacy rows after 216 backfill).
- `curl GET /api/v1/preview/config` → `{"data":{"baseDomain":"env.example.com","ttl":"24h0m0s","retain":"24h0m0s","maxPerOrg":5}}` (string durations).
- `curl GET /api/v1/preview` and `GET /api/v1/admin/preview-deployments` return identical `ListWithExpiry` (order `created_at DESC`) — alias parity confirmed.
- Insert duplicate `(pr_number=42, repo_owner='octo', repo_name='hello')` with `status='running'` twice → second `409 already exists` (via `isPreviewUniqueViolation`) not 500; inserting same after `cleaned_up` succeeds (partial index excludes).
- Webhook `POST /api/v1/preview/webhook/github` with valid `X-Hub-Signature-256` creates preview with `expires_at = now+TTL` and posts `forge/preview` pending status; subsequent `POST /preview/:id/deploy` flips to `running` and `https://pr42-octo-hello.env.example.com` + success status.
- Reaper log: after `expires_at` passes, `ListExpiredPreviewDeployments` → `Cleanup` (TLS+route withdrawal fail-open) → `cleaned_up`; after `retain` (24h) → `Delete`.
- Web UI `http://localhost:3000/admin/preview-deployments` shows top Lifecycle card with TTL `24h`, Limit `3/5 per org` (example), Retain `24h`, Reaper `every 5m`, table column `TTL / Expires` with `5h 12m (2026-08-24T...)` and red `expired — reaper pending` when past due.

---

## 6. Remaining Gaps (not in this slice)

* **Legacy `preview.Service` file not deleted:** `forge/api/internal/services/preview/service.go:42` retained for one release (import still present in `server.go:74` for `PreviewDeploymentSvc` deprecated field). Deletion is phase-06.
* **UI `AdminLoadingState`/`AdminErrorState` not yet wired for config:** config fetch is best-effort (falls back to nil); limit banner handles nil as `5`.
* **Per-PR index test coverage:** `store_preview_env_test.go` (if added) should exercise concurrent `Create` → 409; current verification is manual `go vet` + `TestNoDuplicatePrefixes`.

---

## 7. Citations

* `forge/api/internal/services/preview/service.go:42` vs `previewenv/service.go:121` at `phase4_registrar.go:37` — duplicate implementations on shared `preview_deployments` table (`store_deployment_history.go:195`).
* `forge/api/internal/http/server.go:157` `PreviewDeploymentSvc *previewsvc.Service` legacy wiring vs `phase4_registrar.go:37` correct `previewenv.New` not on admin namespace.
* `handlers_preview_deployments.go:9` legacy handler on `/admin/preview-deployments` — switched to `previewenv.Service` with `Deprecation` header (RFC 8594).
* `store_preview_env.go:17 SetPreviewDeploymentExpiresAt`, `32 ListWithExpiry`, `66 ListExpired`, `94 ListReapable` — TTL store layer.
* `previewenv/service.go:92 PreviewURL=https://pr<N>-<owner>-<repo>.<baseDomain>`, `121 Create`, `136 limit`, `144 per-PR scan`, `179 isPreviewUniqueViolation`, `404 reportStatus` → `git/checks.go:62`.
* `reaper.go:21 StartReaper 5m`, `60 ListExpired → Cleanup`, `73 ListReapable → Delete`.
* `migrations/216_preview_per_pr_unique.sql:9` partial unique index `WHERE status IN ('deploying','running','stopped')` + `14 UPDATE backfill 24h`.
* `phase4_registrar.go:54 reaper`, `server.go:2873 alias lazy singleton`, `main.go:1632 canonical singleton`.
* `forge/web/app/admin/preview-deployments/page.tsx` and `[id]/page.tsx` now show TTL/limit/status/reaper via `/preview/config` + `expiresAt`.
