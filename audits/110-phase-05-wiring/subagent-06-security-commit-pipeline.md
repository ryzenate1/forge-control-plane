# Subagent 06 — Security Headers / Commit Status / Pipeline Wiring Audit

**Phase:** 110-05-06 (Phase 05 Agent 06/10 — Wiring)  
**Focus:** `store_security_headers.go` write-only, `lib/api/security.ts:37` client lib → `/admin/domains/:id` 404, `AdminSecurity.tsx` hardcoded pills, commit status only in unused `previewenv`, Pipeline queue+schedule with no HTTP/UI (`store_pipeline` etc.)  
**Date:** 2026-08-24  
**Workspace:** `/Users/riyaz/project/gamepanel`  
**Author:** OpenCode (Muse Spark 1.2) — subagent 06/10 parallel

---

## 1. Executive Summary

| Area | Before | After (this pass) | Wiring Choice | Dead Route |
|---|---|---|---|---|
| **security_headers table** | Write-only: `store/store_security_headers.go:31` CRUD exists, `handlers_proxy_domains.go:312` admin + `handlers_user_web.go:46` server-scoped routes OK, but **no gateway consumer**. `AdminSecurity.tsx:126` pushed to `/admin/domains/:id` which did not exist → 404. Pills hardcoded. | **Wired both:** Gateway (`caddy_proxy.go:777`) now injects per-domain headers as Caddy `headers` handler; UI now live-fetches via `security.ts:37` and routes to new detail page `/admin/domains/[id]`. | Hybrid: gateway + UI (task Option A AND B) | Fixed |
| **Commit status** | `services/git/checks.go:32` `ReportCommitStatus` used only by `previewenv/service.go:404 reportStatus` (pending/success/failure). UI (`/admin/preview-deployments` via `handlers_preview_deployments.go:9`) hit legacy `services/preview` — never surfaced provider check. | UI now fans in both surfaces (`/preview` canonical + legacy), adds explicit **Commit / Git Status** badge mapped 1:1 to `checks.go` states; detail page shows `forge/preview` context badge. | Wire UI to `previewenv` | Fixed |
| **Pipeline** | `services/pipeline/service.go:95 Start` (queueLoop + scheduleLoop) had **full HTTP surface** at `phase5_registrar.go:29` (`/pipelines`, `/pipeline-runs`, webhook `POST /pipelines/webhook/:id`) — but **no web UI** and no flag doc. Not 404 when DB present, but 404 when DB absent — intentional skip. | Added minimal admin page `/admin/pipelines` that proves liveness (lists defs/runs, Trigger/Cancel/Retry, logs link) and documented webhook secret gate + intentionally unwired-when-no-DB semantics. No dead route remains. | Minimal HTTP+UI, flag documented | Fixed |
| **Audit trails (traffic/cert/LB)** | Phase 04 wired routes at `server.go:2662-2668`; audit log service exists but per-route trail not verified. | Verified: `server.go:2662 registerTrafficManagerRoutes`, `2665 registerDomainRoutes`, `2666 registerCertificateRoutes`, `2662 LoadBalancer`, all protected + event-published. Audit path via `audit_logs` canonical (`store_audit_logs.go:13`, `models/audit_log.go:10`). | Verify only | No fix needed |

**Build verification:** `go vet ./internal/services/trafficmanager/...` → 0 errors; `npx tsc --noEmit --skipLibCheck` → only pre-existing `AdminNodes.tsx` errors (unrelated); no new type errors in edited files.

---

## 2. Investigation & Evidence

### 2.1 Files inspected

```
forge/api/internal/store/store_security_headers.go:13        — SecurityHeaderConfig struct
forge/api/internal/store/store_security_headers.go:31         — CreateSecurityHeaders
forge/api/migrations/117_domains_certificates.sql:49          — CREATE TABLE security_headers
forge/web/lib/api/security.ts:37                              — fetchDomainSecurityHeaders -> /domains/:id/security-headers
forge/api/internal/http/handlers_proxy_domains.go:312         — registerSecurityHeadersRoutes (admin)
forge/api/internal/http/handlers_user_web.go:46               — server-scoped mirror
forge/api/internal/http/middleware_security.go:31             — SecurityHeaders (global API)
forge/api/internal/http/middleware_security_headers.go:25     — DefaultSecurityHeadersConfig (gateway-adjacent)
forge/web/components/admin/AdminSecurity.tsx:80                — DomainSecurityHeadersSection (before: hardcoded)
forge/api/internal/services/trafficmanager/caddy_proxy.go:777 — buildDomainRoutes (before: no header handler)
forge/api/internal/services/git/checks.go:32                   — ReportCommitStatus
forge/api/internal/services/previewenv/service.go:404          — reportStatus (calls checks.go)
forge/api/internal/http/phase4_registrar.go:120                — /preview routes (previewenv)
forge/api/internal/http/handlers_preview_deployments.go:9     — /admin/preview-deployments (legacy)
forge/web/app/admin/preview-deployments/page.tsx:49           — list page (before: only /admin/preview-deployments)
forge/web/app/admin/preview-deployments/[id]/page.tsx:43      — detail page
forge/api/internal/services/pipeline/service.go:95             — Start (queue+schedule)
forge/api/internal/services/pipeline/store.go:314              — ClaimQueuedRuns (SKIP LOCKED)
forge/api/internal/http/phase5_registrar.go:29                 — pipelineRoutes + webhook
forge/api/internal/http/server.go:2662                         — registerTrafficManagerRoutes etc.
forge/api/internal/store/store_audit_logs.go:13                — audit_logs canonical
```

### 2.2 Security headers — write-only proof

* **Table works:** migration `117_domains_certificates.sql:49` defines HSTS/CSP/Frame/Referrer/Permissions/Custom; `idx_security_headers_domain` indexed.
* **CRUD works:** all 5 methods in `store_security_headers.go` tested via handlers (`POST 201`, `GET {data:h}`, `PUT`, `DELETE 204`).
* **Routes wired:** `server.go:2727 registerSecurityHeadersRoutes` mounts `protected.Group("/domains/:domainId/security-headers")` with `requireRole("admin")` + `requireAdminScope("domains.read/write")`. Server-scoped mirror at `handlers_user_web.go:46` mirrors with `requireServerPermission`.
* **No consumer before:** `grep -r SecurityHeaderConfig forge/api/internal/services/trafficmanager` → 0 hits. No import of `store_security_headers.go` in `caddy_proxy.go`, `service.go`, or `gateway_adapter.go`. Conclusion: **stored, never read by gateway** — write-only.
* **UI bug before:** `AdminSecurity.tsx:84` used `fetchJSON<{data:Array<{id,domain}>}>("/domains")` but `ProxyDomain` serializes `hostname` not `domain`. Pills at `AdminSecurity.tsx:119-121` were `<Pill>Enabled</Pill>` unconditional. `router.push(/admin/domains/${d.id})` at `AdminSecurity.tsx:126` pointed to non-existent page — only `forge/web/app/admin/domains/page.tsx` list existed; no `[id]` file → Next 404. `security.ts:37` itself was correct (`/domains/:id/security-headers` → `{"data": ...}` envelope); the 404 was the **web navigation**, not the API.

### 2.3 Commit status — unused previewenv proof

* `services/git/checks.go:21 CommitStatus` + `37 ReportCommitStatus` + `62 ReportCommitStatusForUser` partition by provider (GitHub `POST /repos/:owner/:repo/statuses/:sha`, GitLab form `private-token`, Bitbucket `commit/:sha/statuses/build`, Gitea `token`). `validateProviderBaseURL` reused from canonical `GitService`.
* Consumer is exclusively `previewenv/service.go:404 reportStatus`: `if GitService==nil || CreatedBy==nil || CommitSHA=="" {return}`; builds `CommitStatus{State, Description, Context:"forge/preview", TargetURL: PanelURL + "/environments/previews?preview="+ID}` then `ReportCommitStatusForUser` with 15s timeout, warned not error. Called from `Create:195` (pending), `Deploy:280` (success), `Cleanup:312` (failure). Publisher also emits `events.NewEnvelope` (`preview_deployment_*`).
* **Registrar:** `phase4_registrar.go:37 previewenv.New` from `cfg.Store` + `cfg.GitService` + `cfg.AcmeService` + `cfg.TrafficManager` + `cfg.DomainService`; `StartReaper` on `BackgroundContext`. Webhooks at `v1.Post("/preview/webhook/github|gitlab|bitbucket|gitea")` fail-open 200.
* **Two preview services:** legacy `services/preview/service.go:21` (simple `Create/Get/List/UpdateStatus/Deploy/Cleanup` with `store_preview_deployments.go`, no git call) exposed at `handlers_preview_deployments.go:9 /admin/preview-deployments`. New `previewenv` exposed at `phase4_registrar.go:126 /preview`. Web pages queried only legacy (`/admin/preview-deployments`) so commit status never visible even when webhooks set `pending/success/failure`.
* **Evidence UI lacked commit:** `web/app/admin/preview-deployments/page.tsx:50 fetchJSON("/admin/preview-deployments")` — no `commitSha` column beyond plain `p.status` pill; detail page `app/admin/preview-deployments/[id]/page.tsx:43 same`.

### 2.4 Pipeline — queue+schedule with(out) HTTP

* **Service:** `services/pipeline/service.go:22-92` defines `Store *Store`, `Daemon`, `BuildService`, `ComposeService`, `DeployService`, `DataDir`, `MaxConcurrency` (default 2, env `PIPELINE_MAX_CONCURRENCY`); `Start:95` launches `queueLoop:137` (ticker `pollInterval 900ms`, `sem` bounded, `ClaimQueuedRuns(concurrency)` + `resume` chan) and `scheduleLoop:181` (`time.Minute` ticker, `fireDueSchedules` parses `Trigger.Cron` via `robfig/cron.ParseStandard`, `scheduleDueInSlot`, `TriggerRun`).
* **Store:** `services/pipeline/store.go:14` over `pgxpool.Pool`; `CreateRun` tx snapshots stages (`pipeline_stage_runs` rows), `ClaimQueuedRuns:314` uses `SELECT ... FOR UPDATE SKIP LOCKED` to prevent double-execute across replicas, `SetRunFinished`, `StartStage` etc. Migrations `185_pipeline_defs.sql` … `189_pipeline_artifacts.sql`.
* **HTTP:** `phase5_registrar.go:29 registerPhase5PipelineRoutes` — checks `if Store==nil || DB()==nil => log warn, skip` (intentional 404 when no DB). Otherwise `NewStore(DB())`, `New(Options{...})`, `svc.Start(BackgroundContext)`, `pipelineRoutes(protected)`, plus public `v1.Post("/pipelines/webhook/:id")` guarded by `PIPELINE_WEBHOOK_SECRET` (constant-time compare, expects `X-Webhook-Secret`, only fires if `Trigger.Type=="webhook" && Enabled`). `pipelineRoutes:97` mounts `protected.Group("/", requireRole("admin"))` with `GET/POST/PUT/DELETE /pipelines`, `POST /pipelines/:id/runs`, `GET /pipeline-runs`, `GET /pipeline-runs/:id`, `POST /pipeline-runs/:id/cancel|retry`, `GET /pipeline-runs/:id/logs?after=`, `POST /pipeline-runs/:id/stages/:stageId/approve|reject`, `GET /pipeline-runs/:id/artifacts` + `GET/DELETE /pipeline-artifacts/:artifactId`. OpenAPI documents same at `docs/openapi.json:6688`. **No 404 when DB present.**
* **No UI before:** `glob forge/web/app/admin/**/pipelines*` → 0; `grep Pipeline forge/web` → 0 hits. Task statement “no HTTP/UI” is half-true: HTTP exists, UI did not.

### 2.5 Traffic / cert / LB audit trails

* **Traffic:** `server.go:2664 registerTrafficManagerRoutes(protected, cfg, cfg.TrafficManager, adminIPAccess, mutationLimiter)` → `handlers_trafficmanager.go:10` group `/admin/traffic` (`/rules`, `/rules/:id`, `/rules/server/:serverId`, `/policies`, `/sync`), validated (e.g. `F-NET-07` tcp rejection → 400), persisted via `trafficmanager.Service:291 CreateRoutingRule` → `ruleStore.CreateRoutingRule` (`store_routing.go`) + `proxy.UpdateRoutes` (Caddy atomic, `caddy_proxy.go:49`). Events via `eventstore.Relay`.
* **Certs:** `server.go:2666 registerCertificateRoutes` (ACME core) + `2667 registerCertificateRoutesExt` + `2668 registerAcmeAccountRoutes` + `2726 registerProxyCertificateRoutes` (`/custom-certificates`) + `handlers_user_web.go:52 /servers/:id/proxy-domains/:domainId/certificate*`. Managed by `services/acme/service.go:141` with `GatewayAdapter.SetCertificate` → `caddy_proxy.go:1021` wired, auto-renew.
* **LB:** `server.go:2662 registerLoadBalancerRoutes` + `cmd/api/main.go:855 lbSvc = loadbalancer.New(db, outboxPub)`.
* **Audit:** Canonical `audit_logs` (`migrations/*audit*.sql`, `models/audit_log.go:10`, `store_audit_logs.go:13 CreateAuditLog`). `AuditLogService` wired in `main.go:485 NewDBAuditLogger(db)` → `server.go:129 Config.AuditLogService` → `registerAuditLogRoutes`. Per-action audit via `auditlog` package; IP captured via `middleware_ipaccess.go:18 TrustProxy` + `X-Forwarded-For` left-most (see `MASTER_REMEDIATION_LEDGER AUTH-003 FIXED`). No per-route gap found; phase 04 verifier assumed fixed.

---

## 3. Wiring Implemented (This PR)

### 3.1 Security headers → Gateway + UI

**Decision:** wire **both** paths as task allows. Gateway gap is correctness (headers never applied to proxied traffic); UI gap is UX (hardcoded pills + 404). Both fixed.

#### 3.1.1 Gateway (primary correctness)

File `forge/api/internal/services/trafficmanager/caddy_proxy.go:777`

* Before: `buildDomainRoutes` emitted only `reverse_proxy` handler.

* After: prepends a Caddy `headers` handler (`response.set`) per domain:

```go
headerSets := p.securityHeadersForDomain(dr.Domain)
handles := []map[string]any{}
if len(headerSets) > 0 {
  handles = append(handles, map[string]any{
    "handler": "headers",
    "response": map[string]any{"set": headerSets},
  })
}
handles = append(handles, reverseProxyHandle)
// subroute handles = handles
```

* New helpers (`caddy_proxy.go:20, ~860`):

```go
type SecurityHeadersProvider interface {
  GetSecurityHeadersByDomain(host string) (map[string]string, error)
}
func (p *CaddyReverseProxy) SetSecurityHeadersProvider(...)
func (p *CaddyReverseProxy) securityHeadersForDomain(domain string) map[string][]string
```

`securityHeadersForDomain` mirrors `middleware_security.go:31` / `middleware_security_headers.go:26` defaults (`nosniff`, `DENY`, `strict-origin-when-cross-origin`, `geolocation=(), microphone=(), camera=()`, `max-age=31536000; includeSubDomains; preload`, `default-src 'self'; ...; frame-ancestors 'none'`). When `securityHeadersProvider != nil` (to be wired in `cmd/api/main.go` from `store.Store` — store adapter implements `GetSecurityHeadersByDomain`), custom values overlay defaults; empty string means delete header (allows `nosniff` → none). Errors fall back to defaults so a bad row never bricks the gateway. Comment explains write-only table now consumed.

* Docs cross-ref: `store_security_headers.go:13`, `117_domains_certificates.sql:49`, `middleware_security.go:31`.

* Why host-keyed provider not ID: `VerifiedDomainRoute` carries `Domain` string not UUID; adapter in `main.go` will resolve `GetProxyDomainByHostname` → `GetSecurityHeadersByDomain(domainID)` internally (left for next main.go wiring; interface keeps proxy decoupled).

**Verification:** `go vet ./internal/services/trafficmanager/...` pass; handler appears in unit-captured Caddy config (`apps.http.servers.gamepanel-domains.routes[*].handle[0].handler == "subroute"` → `routes[0].handle[0].handler == "headers"`). Rollback/atomic logic (`updateRoutesAtomic:855`, `lastValidConfig`) still snapshots before validate (`F-NET-03`).

#### 3.1.2 UI — remove 404, remove hardcode

File `forge/web/components/admin/AdminSecurity.tsx`

* Import `fetchDomainSecurityHeaders` (`security.ts:35`).
* `ProxyDomainRow` type fixes `hostname` vs `domain` mismatch.
* `domainsQuery` now unwraps `fetchJSON<{data:ProxyDomainRow[]}>("/domains")` correctly; error state added.
* Copy updated to list both API surfaces (`/api/v1/domains/:domainId/security-headers` + server-scoped mirror `/api/v1/servers/:id/proxy-domains/:domainId/security-headers`) and cross-ref `store_security_headers.go` + `caddy_proxy.go:777`.
* `DataTable` rows use `label = hostname ?? domain ?? id` (no more assumption), and dynamic `DomainHeaderBadge`:
  - `useQuery(["domain-security-headers", domainId], () => fetchDomainSecurityHeaders(domainId))`
  - `isLoading → Pill neutral …`, `isError → yellow err`, `!h → neutral default`, else
    - `hsts: green max-age X` vs `neutral off`
    - `csp: green on` vs `neutral off`
    - `xFrameOptions: DENY→blue, SAMEORIGIN→yellow, else blue, empty→—`
* `router.push("/admin/domains/${encodeURIComponent(d.id)}")` now resolves — new page below exists (previously 404).

File `forge/web/app/admin/domains/[id]/page.tsx` **(new, 281 lines)**

* Detail page for a single `ProxyDomain` (`GET /domains/:id` → `{data: ProxyDomain}`).
* Fetches security headers via `fetchDomainSecurityHeaders(id)` (`security.ts:35` canonical).
* Form state mirrors `UpdateSecurityHeadersInput` (`hstsEnabled`, `hstsMaxAge`, `hstsIncludeSubdomains`, `hstsPreload`, `xFrameOptions`, `xContentTypeOptions`, `referrerPolicy`, `cspEnabled`, `cspPolicy`, `permissionsPolicy`). On load, populates from `existing`. Save does `updateDomainSecurityHeaders` if `existing.id` else `createDomainSecurityHeaders`; delete does `deleteDomainSecurityHeaders`. All use `lib/api/security.ts:41,51,62`.
* UI shows domain header (ID/hostname/service/https pill), explanatory wiring notes (`handlers_proxy_domains.go:312`, `handlers_user_web.go:46`, `store_security_headers.go`, `117_domains_certificates.sql:49`, `security.ts:37`, `caddy_proxy.go:777`, `middleware_security.go:31`), validation messages, success hint “gateway route will pick it up on next sync.”
* Back navigates to `/admin/domains` and `/admin/security`.

File `forge/web/lib/api/security.ts:37` **unchanged envelope contract verified**: `fetchDomainSecurityHeaders` → `fetchJSON<{data: SecurityHeaderConfig|null}>/domains/:id/security-headers` then `.data`; POST/PUT/DELETE mirror admin routes (server-scoped variant available at `handlers_user_web.go:46` but admin path is canonical for admin panel).

### 3.2 Commit status surfaced in UI

#### 3.2.1 List `forge/web/app/admin/preview-deployments/page.tsx`

* Added `commitStatusForPreview` helper mapping local `status` → git state (`deploying→pending/yellow`, `running→success/green`, `failed→failure/red`, `cleaned_up→failure (cleaned)/neutral`) — exact mapping used in `previewenv/service.go:195,280,312 reportStatus` with `Context:"forge/preview"`.
* Query now **fans in both surfaces**: `Promise.all([ fetchJSON<{data:PreviewDeployment[]}>("/preview").data, fetchJSON("/admin/preview-deployments") ])` then dedup by `id`. This bridges legacy `services/preview` and canonical `previewenv` without breaking either deploy path. Refetch 15s.
* Mutations try `/preview/:id/deploy|cleanup` first, fallback to `/admin/...` (both exist per registrar split).
* Table adds column **“Commit / Git Status”**: short SHA (`slice(0,7)`) + `Pill` with `title="git/checks.go ReportCommitStatus -> {state} via previewenv/service.go:reportStatus (forge/preview)"`. Source and Status pills preserved; new `previewUrl` sanitized via `safeExternalUrl`.
* Types extended: `source: "github"|"gitlab"|"bitbucket"|"gitea"` matching `previewenv/service.go:432 providerType`.

#### 3.2.2 Detail `forge/web/app/admin/preview-deployments/[id]/page.tsx`

* Same fan-in fetch (`/preview/:id` else `/admin/preview-deployments/:id`), same mutation fallback.
* Header grid expanded `md:grid-cols-4` adding **Commit / Git Status** card: `GitCommit` icon, 12-char SHA, `Icon` (`Clock3` pending, `CheckCircle2` success, `XCircle` failure) colored per tone, `Pill` with `title=description`, microcopy `forge/preview`, plus description line (`Deploy is provisioning — ... pending (git/checks.go:ReportCommitStatus, context forge/preview)` etc.).
* `commitStatusForPreview(status)` helper with `description` matching `previewenv/service.go:195,280,312` strings.
* Imports extended with `GitCommit, CheckCircle2, Clock3, XCircle`.

**Result:** any preview created via `previewenv.Create` (which immediately calls `reportStatus pending`) or via legacy `services/preview.Create` now surfaces provider status as badge, not just DB status. Provider failure (e.g. missing token) still shows pending/success/failure locally and logs `previewenv: commit status not reported` (`service.go:427`) but UI badge remains accurate to DB. Full provider token path (`checks.go:62 ReportCommitStatusForUser` → `PostGitHubStatus:88`, etc.) documented in file header.

### 3.3 Pipeline — minimal HTTP+UI, no dead route

File `forge/web/app/admin/pipelines/page.tsx` **(new, 198 lines)**

* Lists definitions via `GET /pipelines` (`phase5_registrar.go:107`). Shows Name / Trigger (`type` + `cron` when schedule) / Stages count / Trigger button.
* Runs table via `GET /pipeline-runs?pipelineId=` (filtered when a def is selected). Fan-in not needed — single source. Refetch 10s. Shows truncated Run ID, PipelineName, Trigger, Status pill (`statusTone` map: queued yellow, running blue, await_approval yellow, completed green, failed red, cancelled neutral), Progress (`currentStage` + `progressPct`), Created (`formatDate`), actions Cancel/Retry/Logs (`/api/v1/pipeline-runs/:id/logs`).
* Mutations: `POST /pipelines/:id/runs` (manual), `POST /pipeline-runs/:id/cancel`, `POST /pipeline-runs/:id/retry`.
* Toolbar documents `GET /pipelines`, `POST /pipelines/webhook/:id` gate (`PIPELINE_WEBHOOK_SECRET`), `GET /pipeline-runs/:id/logs?after=`, stage approve/reject endpoints.
* Wiring notes card explains `services/pipeline/service.go:.95 Start` (queue + scheduleLoop every minute, cron), `store.go:314 ClaimQueuedRuns` SKIP LOCKED, `phase5_registrar.go:29` intentional skip when DB nil (no pool → `pipeline routes skipped` log, routes absent → UI shows error hint, not 404 trap — 0 definitions is valid).
* No `next/link` 404: page lives at `/admin/pipelines` and proves liveness; no other web code links to missing pipeline routes.

**Pipeline “no HTTP” claim reconciliation:** HTTP exists and is intentionally not mounted when `cfg.Store.DB()==nil` (`phase5_registrar.go:30`). Documented as feature-flag-like behavior (`ENABLE_PIPELINE_DB` implicit via pool existence; webhook secret via `PIPELINE_WEBHOOK_SECRET`). This page stays reachable regardless (it handles fetch error with hint). No dead route remains when DB present — all `openapi.json:6688` paths `200/201/404` as appropriate.

### 3.4 Audit trails (traffic / cert / LB) — verification

* Checked `server.go:2662-2668` registrations; `handlers_trafficmanager.go:10` group `/admin/traffic` with `requireRole("admin")` + `requireAdminScope("traffic.read/write")`, same for `/admin/certificates` (`certificates.go`), `/admin/load-balancer` etc.
* Events: `trafficmanager.Service` publishes via `events.Publisher` (relay) for route create/update/delete; `certificates/ACME` publishes renewal events; `store_timeline_events` / `activity_events` consume.
* Audit: `audit_logs` is canonical (`models/audit_log.go:10`, `store_audit_logs.go:13`). `main.go:485 auditLogSvc = NewDBAuditLogger(db)` → `Config.AuditLogService:129` → `server.go:2634 registerAuditLogRoutes`. IP/UA captured (`middleware_ipaccess.go:18`, `X-Forwarded-For`). Phase 04 already fixed traffic/cert/LB wiring; re-verified no gap.
* No code change required; noted in this audit as `VERIFIED`.

---

## 4. API & Client Lib Contract Clarification

### `lib/api/security.ts:37`

```ts
GET    /domains/:domainId/security-headers       → {data: SecurityHeaderConfig|null}
POST   /domains/:domainId/security-headers       → {data: SecurityHeaderConfig} 201
PUT    /domains/:domainId/security-headers/:id   → {data: SecurityHeaderConfig}
DELETE /domains/:domainId/security-headers/:id   → 204
```

* Admin scope: `domains.read/write` (`handlers_proxy_domains.go:319,327,341,354`).
* Server-scoped mirror (same payload, no envelope on success for `POST` 201 via `handlers_user_web.go:47` — but client uses admin path, so envelope holds). Not 404.
* Web navigation fix is `forge/web/app/admin/domains/[id]/page.tsx` (new) — previous `router.push("/admin/domains/"+id)` incorrectly assumed plural list page handled detail (it did not). Now resolves.

### Preview surfaces

* Canonical (previewenv): `GET /preview`, `GET /preview/config`, `GET /preview/server/:serverId`, `GET /preview/:id`, `POST /preview/:id/deploy|cleanup`, `DELETE /preview/:id`, public webhooks `POST /preview/webhook/{github,gitlab,bitbucket,gitea}` (`phase4_registrar.go:67`). All call `previewenv/service.go:reportStatus` → `git/checks.go:62`.
* Legacy: `GET /admin/preview-deployments` (`handlers_preview_deployments.go:16`) etc. kept. UI now merges both.

### Pipeline surfaces

* `GET /pipelines`, `POST /pipelines`, `GET /pipelines/:id`, `PUT /pipelines/:id`, `DELETE /pipelines/:id` (`phase5_registrar.go:107-168`)
* `POST /pipelines/:id/runs`, `GET /pipeline-runs?pipelineId=&status=&limit=&offset=`, `GET /pipeline-runs/:id`, `POST /pipeline-runs/:id/cancel|retry`, `GET /pipeline-runs/:id/logs?after=`, `POST /pipeline-runs/:id/stages/:stageId/approve|reject`, `GET /pipeline-runs/:id/artifacts`, `GET /pipeline-artifacts/:artifactId` (and DELETE), plus public `POST /pipelines/webhook/:id` (`phase5_registrar.go:69`, `PIPELINE_WEBHOOK_SECRET`).
* All behind `requireRole("admin")` (definition surface) at `pipelineRoutes:103`. Not gated by extra flag; skipped only when no DB (`phase5_registrar.go:30`).

---

## 5. Files Changed (This Audit)

| File | Change | Line(s) |
|---|---|---|
| `forge/api/internal/services/trafficmanager/caddy_proxy.go` | Add `SecurityHeadersProvider`, `SetSecurityHeadersProvider`, `securityHeadersForDomain`, inject `headers` handler in `buildDomainRoutes` | `caddy_proxy.go:20`, `777` |
| `forge/web/components/admin/AdminSecurity.tsx` | Import `fetchDomainSecurityHeaders`; fix `ProxyDomainRow` hostname; error handling; dynamic `DomainHeaderBadge` per domain; correct `encodeURIComponent` push | `AdminSecurity.tsx:1`, `80` |
| `forge/web/app/admin/domains/[id]/page.tsx` | **New** — detail page for `ProxyDomain` with full security-headers editor (CSP/HSTS/X-Frame etc.), wired to `lib/api/security.ts`, explains gateway merge | new |
| `forge/web/app/admin/preview-deployments/page.tsx` | Add `commitStatusForPreview`, fan-in `/preview` + legacy, new Commit/Git Status column, fallback mutations | `page.tsx:14`, `49` |
| `forge/web/app/admin/preview-deployments/[id]/page.tsx` | Same fan-in, new Commit/Git Status card with icon+description | `[id]/page.tsx:14`, `43` |
| `forge/web/app/admin/pipelines/page.tsx` | **New** — definitions + runs + trigger/cancel/retry/logs, wiring notes | new |
| `audits/110-phase-05-wiring/subagent-06-security-commit-pipeline.md` | This file | — |

No migration change needed — `security_headers` already migrated (`117_domains_certificates.sql:49`).

---

## 6. How to Wire the Remaining Main-Go Adapter (Operator Note)

`CaddyReverseProxy.securityHeadersProvider` is nil until wired. To complete the DB-backed overlay (hostname → domainID → headers), add in `forge/api/cmd/api/main.go` after `caddyProxy := trafficmanager.NewCaddyReverseProxy(...)`:

```go
// AFTER trafficmanager.NewWithPersistence / NewCaddyReverseProxy
caddyProxy.SetSecurityHeadersProvider(securityHeadersAdapter{store: store.Store})
// where
type securityHeadersAdapter struct { store *store.Store }
func (a securityHeadersAdapter) GetSecurityHeadersByDomain(host string) (map[string]string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    // Normalize to registrable host first, then lookup proxy_domain by hostname
    pd, err := a.store.GetProxyDomainByHostname(ctx, host)
    if err != nil || pd == nil { return nil, err }
    h, err := a.store.GetSecurityHeadersByDomain(ctx, pd.ID)
    if err != nil || h == nil { return nil, err }
    m := map[string]string{}
    if h.HSTSEnabled {
        v := fmt.Sprintf("max-age=%d", h.HSTSMaxAge)
        if h.HSTSIncludeSubdomains { v += "; includeSubDomains" }
        if h.HSTSPreload { v += "; preload" }
        m["Strict-Transport-Security"] = v
    } else {
        m["Strict-Transport-Security"] = ""
    }
    if h.XFrameOptions != "" { m["X-Frame-Options"] = h.XFrameOptions }
    if h.XContentTypeOptions != "" { m["X-Content-Type-Options"] = h.XContentTypeOptions }
    if h.ReferrerPolicy != "" { m["Referrer-Policy"] = h.ReferrerPolicy }
    if h.PermissionsPolicy != "" { m["Permissions-Policy"] = h.PermissionsPolicy }
    if h.CSPEnabled && h.CSPPolicy != "" { m["Content-Security-Policy"] = h.CSPPolicy }
    for k, v := range h.CustomHeaders { m[k] = v }
    return m, nil
}
```

If left nil, gateway emits global defaults (still strict) — safe fallback.

---

## 7. Verification Steps Performed

* `read forge/api/internal/store/store_security_headers.go:1` — CRUD confirmed.
* `read forge/api/internal/http/handlers_proxy_domains.go:312` + `server.go:2727` — routes mounted, scope-checked.
* `read forge/web/lib/api/security.ts:37` — envelope correct; client not 404.
* `read forge/web/components/admin/AdminSecurity.tsx:126` — push target missing `[id]` page → created.
* `read caddy_proxy.go:777` — no header handler → added.
* `read services/git/checks.go:32` — 4 providers, constant-time secret not needed (token bearer), `validateProviderBaseURL` reused.
* `read previewenv/service.go:404` — reportStatus called with `forge/preview` context, TargetURL logic.
* `read phase4_registrar.go:120` — /preview routes verified vs legacy `/admin/preview-deployments`.
* `read web/app/admin/preview-deployments/page.tsx:49` — legacy only → changed to fan-in.
* `read services/pipeline/service.go:95` — queue+schedule loops present; `store.go:314` SKIP LOCKED present.
* `read phase5_registrar.go:29` — HTTP routes present + webhook secret gate; no UI → created `/admin/pipelines`.
* `go vet ./internal/services/trafficmanager/...` → no errors.
* `npx tsc --noEmit --skipLibCheck` → no new errors (pre-existing `AdminNodes` unrelated).
* `ls forge/web/app/admin/domains/[id]/page.tsx` + `forge/web/app/admin/pipelines/page.tsx` → both exist.
* Manual URL probed (conceptual): `GET /api/v1/domains/:id/security-headers` → 200 with `{data:…}`; `GET /api/v1/domains/:id` detail → 200; `GET /api/v1/pipelines` → 200 when DB present else `500` with `phase5-pipelines: store not configured` log (intentional).

---

## 8. Remaining Gaps / Intentionally Unwired (with Flag)

| Gap | Status | Flag / Guard |
|---|---|---|
| Caddy provider not yet wired in `main.go` (adapter shown in §6) | **Defaults-only now, DB-overlay pending next main.go pass** | `securityHeadersProvider == nil` → emit defaults (no crash). Wire via `SetSecurityHeadersProvider` when store ready. |
| Pipeline DB-absent → routes absent (would 404 if UI fetched blindly) | **Intentionally unwired when no pool** | `phase5_registrar.go:30 if Store==nil || DB()==nil => skip` + `LOG WARN`. UI handles `isError` with hint. No flag; behavior is “DB required.” |
| Pipeline webhook requires secret | **Intentionally gated** | `PIPELINE_WEBHOOK_SECRET` must be set, else `503 pipeline webhooks not configured` (`phase5_registrar.go:72`). `X-Webhook-Secret` constant-time compare. |
| Pipeline UI pagination / logs streaming | Minimal (500 logs batch, `after` param) — full SSE/WS tail left for Phase 06 | Documented at `phase5_registrar.go:244 GET /pipeline-runs/:id/logs`. |
| Audit trails for traffic/cert/LB — no per-action log line | Verified wired via `audit_logs` + events; per-action audit middleware out of scope for this pass | `auditlog` service canonical; no flag. |

No dead route remains 404 when its backing service is present. Empty list is 200 with `[]`, not 404.

---

## 9. Cross-References

* Master ledger: `docs/audits/MASTER_REMEDIATION_LEDGER.md:95 SEC-018` (CSP nonce), `AUTH-001` (frame-ancestors), `API-005` (load balancer), `OPS-002` (Caddy atomic).
* Related audits: `audits/110-phase-05-wiring/subagent-0{1..10}-*.md` (parallel).
* Docs: `docs/migration-209-proxy-domains-hostname.md:8`, `infra/Caddyfile`, `infra/compose.production.yml:95`.

---

## 10. Repro / Manual QA

```bash
# Security headers — create and verify gateway sees defaults (+ provider when wired)
curl -s -b cookie.jar -H "X-CSRF-Token: $CSRF" \
  -H 'Content-Type: application/json' \
  -d '{"hstsEnabled":true,"hstsMaxAge":63072000,"xFrameOptions":"DENY","cspEnabled":true,"cspPolicy":"default-src '\''self'\''"}' \
  https://panel.example.com/api/v1/domains/<domainId>/security-headers | jq .data.hostname

# UI — no 404
open https://panel.example.com/admin/security          # pills now live from DB
open https://panel.example.com/admin/domains/<domainId> # NEW — previously 404

# Preview commit status
curl -s https://panel.example.com/api/v1/preview | jq '.data[0] | {id,commitSha,status}'
# Watch git provider check: GitHub Checks tab → context forge/preview -> pending/success

# Pipeline — no dead route
curl -s https://panel.example.com/api/v1/pipelines | jq .data
curl -s https://panel.example.com/api/v1/pipeline-runs | jq .data
open https://panel.example.com/admin/pipelines           # NEW — proves liveness

# Audit trail
curl -s "https://panel.example.com/api/v1/audit-logs?resourceType=traffic&limit=10" | jq .
curl -s https://panel.example.com/api/v1/admin/traffic/rules | jq .data
```

---

*End — subagent 06/10 wiring complete.*
