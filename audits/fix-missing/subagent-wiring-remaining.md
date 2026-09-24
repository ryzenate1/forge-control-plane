# Subagent — Wiring Remaining Hidden Capabilities (Phase 110-03 Synthesis §4 + FINAL_PARITY_AUDIT §7 Hidden)

**Date:** 2026-08-24  
**Scope:** Live HEAD checkout at `/Users/riyaz/project/gamepanel` vs audits/reverification/synthesis.md §4 and FINAL_PARITY_AUDIT §12.3 Hidden/UNWIRED list + MASTER_FINDING_INDEX. Each item inspected file:line SOURCE_VERIFIED, 404-probed via static route + UI nav link check. Minimal code wiring applied where still write-only.

---

## Summary Verdict

| # | Checklist Item | Prior Status (audit claim) | Live Verdict 2026-08-24 | Action | 404 now? |
|---|---|---|---|---|---|
| 1 | `security_headers` table write-only | Claimed FIXED via `caddy_proxy.go:777 SecurityHeadersProvider → headers handler` (110-03-16) | **STILL WRITE-ONLY** — provider defined but never wired in `main.go` → gateway rendered defaults only | **WIRED** (`main.go` adapter + `SetSecurityHeadersProvider`) | No |
| 2 | Capabilities delta endpoint (`beacon/internal/server/capabilities.go:226`) + `CheckCapability` before scheduling (`scheduler/service.go:49`) | Claimed FIXED (phase 05-05) | **Compute CORRECT, scheduling WIRED, panel delta 404** — beacon delta correct, scheduler check present, panel `/capabilities/:nodeId/delta` missing (UNWIRED) | **WIRED** panel delta handler added | No |
| 3 | StorageLocality vocab (`scheduler/service.go:817` canonical `local_only→local`) + Nodes pill | Claimed FIXED | **FIXED confirmed** — canonical helpers + pill present | None | No |
| 4 | `server_orphan_remediations` tracking + `/admin/orphans` page (phase 05-05) | Claimed FIXED | **FIXED confirmed** — page exists, nav linked, API live | None | No |
| 5 | Pipeline queue+schedule (`store_pipeline` / `services/pipeline`) with no HTTP/UI — `/admin/pipelines` (phase 05-06) | Claimed FIXED | **FIXED confirmed** — queue loop + scheduleLoop live, phase5 registrar mounts `/api/v1/pipelines*`, UI page exists & nav linked | None | No |
| 6 | Any other hidden — MASTER_FINDING_INDEX UNWIRED remainder | Mixed | **No new UNWIRED requiring hotfix** — installer, HTTPSolver, previewenv, servicediscovery, river_queue, pre-restore-*.zip etc. already wired or intentionally gated (see §6) | None (documented) | No |

Overall: **4 of 5 checklist rows were already FIXED; 2 still required small wiring (security_headers gateway + panel delta) which is now landed. Zero 404 on hidden routes after patch. `go vet ./...` clean on both `forge/api` and `beacon`.**

---

## 1. `security_headers` — Write-Only → Gateway Wired

### Prior claim
FINAL_PARITY_AUDIT §12.3 / synthesis §4 listed `security_headers` as UNWIRED. Phase 110-03-16 claimed fix via `forge/api/internal/services/trafficmanager/caddy_proxy.go:777` `SecurityHeadersProvider` → `headers` handler. Reverification required confirming it left write-only.

### Live inspection

**Store** — `forge/api/internal/store/store_security_headers.go:13-31` `SecurityHeaderConfig` with 11 columns; `CreateSecurityHeaders:44`, `GetSecurityHeadersByDomain:89` (`WHERE domain_id=$1`), `Update/Delete` complete. Migration `forge/api/migrations/117_domains_certificates.sql:49-65` defines `security_headers(domain_id→proxy_domains.id)` with defaults (HSTS 63072000, DENY, nosniff, strict-origin…).

**Gateway Caddy code** — `forge/api/internal/services/trafficmanager/caddy_proxy.go:32-43` field `securityHeadersProvider SecurityHeadersProvider` + setter `SetSecurityHeadersProvider`. Interface `caddy_proxy.go:867-872`:

```go
type SecurityHeadersProvider interface {
    GetSecurityHeadersByDomain(host string) (map[string]string, error)
}
```

`securityHeadersForDomain:879-903` builds canonical defaults (mirroring `middleware_security.go:31` + `middleware_security_headers.go:26`) and overlays provider map (`delete(defaults,k)` when `v==""`). `buildDomainRoutes:788-865` attaches per-domain `headers` handler:

```go
headerSets := p.securityHeadersForDomain(dr.Domain) // 834-835
if len(headerSets)>0 { handles = append(handles, map[string]any{"handler":"headers","response":map[string]any{"set": headerSets}}) }
```

All correct — **but** `grep -r SetSecurityHeadersProvider` found **only the definition** (2 hits, both in `caddy_proxy.go` itself) and **zero callers** in `forge/api/cmd/api/main.go`. Gateway always fell back to defaults; custom rows were persisted but never emitted. `AdminSecurity.tsx:54,89-170` UI and `handlers_proxy_domains.go:312-360` CRUD (`GET/POST /domains/:domainId/security-headers` + server-scoped mirror `handlers_user_web.go:46-48`) were live, proving store was writeable via UI but read path to gateway was dead — classic write-only.

**UI** — `forge/web/components/admin/AdminSecurity.tsx:73-170` `DomainSecurityHeadersSection` lists `/domains` and per-domain badge via `fetchDomainSecurityHeaders` (`forge/web/lib/api/security.ts:35-62`). `forge/web/app/admin/domains/[id]/page.tsx:9-106` provides full editor (HSTS/CSP/frame). `forge/web/app/admin/gateways/page.tsx:487` lists "Headers / HSTS" neutral pill. So UI wiring was present.

### Fix applied (small wiring)

`forge/api/cmd/api/main.go:116-123` — new `securityHeadersAdapter` bridging `store.Store.GetSecurityHeadersByDomain(ctx, domainID)` (ID-keyed) to the hostname-keyed provider interface. Resolution: `host → GetProxyDomainByHostname → GetSecurityHeadersByDomain` with 3s timeout, wildcard `*.` fallback, and struct→`map[string]string` translation:

- HSTS: `max-age=N; includeSubDomains; preload` when enabled, `""` (deletes default) when disabled
- X-Frame-Options / X-Content-Type-Options / Referrer-Policy / Permissions-Policy when non-empty
- CSP: value when enabled, `""` when disabled (deletes default CSP)
- `CustomHeaders` merged verbatim (empty key skipped, empty value deletes)

`main.go:1056` — immediate wiring after Caddy construction:

```go
caddyProxy := trafficmanager.NewCaddyReverseProxy(env("CADDY_ADMIN_ADDR", "127.0.0.1:2019"))
caddyProxy.SetSecurityHeadersProvider(&securityHeadersAdapter{store: db}) // ← FIX
```

Nil-safe: only inside `if db != nil` block (line 385), `db` non-nil there. No new import beyond existing `context/fmt/strings/time`. `go vet ./forge/api/cmd/api` clean.

After fix: custom `security_headers` rows now flow `DB → adapter → securityHeadersForDomain → Caddy headers handler → gateway response.set`. Empty custom = honest defaults.

### No-404 proof

- API: `registerSecurityHeadersRoutes` mounted at `server.go:2974` under `protected.Group("/domains/:domainId/security-headers", adminIPAccess)` — `GET / (domains.read)`, `POST /(domains.write)`, `PUT /:id`, `DELETE /:id`. Server-scoped mirror `handlers_user_web.go:46-48` at `/servers/:id/proxy-domains/:domainId/security-headers`. `grep` shows 7 call sites — no 404 when DB present.
- Gateway: `buildDomainRoutes` always emits `headers` handler (defaults at minimum). With provider wired, per-domain overrides now reach Caddy at `POST /config/apps/http/servers/gamepanel-domains` via `UpdateDomainRoutes:701-786` (fetch-merge-preserve + sub-resource, F-NET-02/03 fixes).
- UI: `/admin/security` (registry `admin-registry.ts:111`) + per-domain editor `/admin/domains/[id]` both render without 404; empty-state copy explicitly cites `security_headers` + `store_security_headers.go` + Caddy gateway.

---

## 2. Capabilities Delta Endpoint + `CheckCapability` before Scheduling

### 2a. Beacon delta compute `beacon/internal/server/capabilities.go:226`

**Live code** (`beacon/internal/server/capabilities.go:30-283`):

- `collectCapabilities:91-184` honest: `runtimeProvider` from factory-wired `s.runtimeProvider` (not `""`), `runtimeAvailable` via `dockerStatus()/Pinger`, build via `LookPath(docker/nixpacks)`, empty `previousCaps` not silently empty.
- `handleGetCapabilitiesDelta:226-237` — `current:=collectCapabilities(); delta:=computeCapabilityDeltaFromExternal(current); writeJSON(delta)`. Protected by `capabilitiesMu`.
- `computeCapabilityDeltaFromExternal:239-283` — first-call `previousCaps==nil → Added=current.Capabilities` (honest initial). Subsequent: map by `CapabilityType`, `Added` if new type, `Changed` if `Status!=` or `Version!=`, `Unchanged` else, `Removed` if type disappeared; then atomically updates `previousCaps`. Correct and race-safe via `capabilitiesMu` lock (lock→copy→unlock→compute→lock→store). Comment at `232-235` documents honest empty arrays as "no drift" signal.

**Mount**: `beacon/internal/server/server.go:458` `GET /api/capabilities/delta → handleGetCapabilitiesDelta` (mux). No 404 on beacon.

Verdict: **Compute CORRECT** as claimed in phase 05-05. No code change needed.

### 2b. `CheckCapability` before scheduling `forge/api/internal/services/scheduler/service.go:49`

**Live code** (`scheduler/service.go:244-267`):

```go
if s.runtimeRegistry != nil && node.RuntimeProvider != "" {
    if !s.runtimeRegistry.CheckCapability(node.RuntimeProvider, runtime.CapabilityContainers) {
        s.recordPlacementRejection()
        slog.Info("scheduler capability rejection", "node", node.ID, "provider", node.RuntimeProvider, "capability", runtime.CapabilityContainers)
        continue
    }
}
if s.runtimeRegistry != nil && req.Runtime != "" && !strings.EqualFold(req.Runtime,"auto") {
    if !s.runtimeRegistry.CheckCapability(strings.TrimSpace(req.Runtime), runtime.CapabilityContainers) { ... }
}
```

**Wiring proof**: `forge/api/cmd/api/main.go:417-430` builds `capRuntimeRegistry` (`runtime.NewRegistry()` + 7 adapters) and passes it to `scheduler.New(...).WithRuntimeRegistry(capRuntimeRegistry)`. Multi-runtime adapters also wired for `clustermanager`. Tests `forge/api/internal/runtime/registry_test.go:204-240` cover `CheckCapability` true/false/not-found/nil + metrics.

Verdict: **WIRED** exactly as claimed (`service.go:49` line number shifted to 252 after formatter, same block). No change needed.

### 2c. Panel delta endpoint — missing 404

**Gap found**: Panel had `GET /capabilities`, `GET /capabilities/:nodeId`, `GET /capabilities/:nodeId/history:56`, `POST /capabilities/:nodeId/probe:77` but **no** `GET /capabilities/:nodeId/delta`. Frontend `AdminNodes.tsx:890-1003` `NodeCapabilitiesTab` consumed `capabilities/:nodeId` + `history?limit=2` and synthesized delta locally (pill: `Δ history N snapshots`, stable vs drift via version diff). It never called a delta API, but audit classified "capabilities delta endpoint" as UNWIRED hidden — an empty-404 if anyone did call it. Synthesis §4 still listed it UNWIRED.

**Fix applied** — `forge/api/internal/http/handlers_capabilities.go:76-145` new handler `GET /capabilities/:nodeId/delta` (admin `nodes.read`):

- Reads `GetCapabilityHistory(ctx, nodeId, 2)` (most recent two snapshots, DESC).
- `len==0 → 404`; `len==1 → {added: entries[0].Capabilities, removed/changed/unchanged: []}` (initial honest).
- `len==2 → map-by-type diff` mirroring beacon logic (deep JSON equality via `json.Marshal` compare), returns `{nodeId, fetchedAt: observedAt RFC3339, added, removed, changed, unchanged}`.
- Registered inside `registerCapabilityRoutes` alongside history (between history and probe, after history to keep 3-segment route distinct from `/:nodeId`). `go vet ./forge/api/internal/http` clean.

After fix: `GET /api/v1/capabilities/:nodeId/delta` no longer 404; panel UI already shows delta note at `AdminNodes.tsx:935,988` ("delta via /delta + history"), now backed by real endpoint.

---

## 3. StorageLocality Vocab — Canonical `local_only → local`

**Prior fix**: `scheduler/service.go:817` canonical helpers added in 110-03-17 (referenced as `scheduler/service.go:817` in task).

**Live verification**:

- `forge/api/internal/services/scheduler/service.go:848-862` `canonicalStorageLocality(s)=strings.ToLower(TrimSpace(s)); if trimmed=="local_only" return "local"`, `storageLocalityEqual/isLocalStorageLocality/normalizeRequest` all use it.
- `service.go:234` `isLocalStorageLocality(req.StorageLocality)` gates local-only placement (rejects `RuntimeProvider != local`).
- `service.go:395-411` scoring uses `storageLocalityEqual` with normalized `+0.15` match / `-0.50` mismatch under `FORGE_PLACEMENT_V2` (clamped `±0.20` via `schedulerStorageBonus/Penalty`), matching `FORGE_IMPLEMENTATION_PLAN §3` spec.
- `service.go:811-834` `nodeToCandidate` maps `RuntimeProvider nfs/shared → shared` else `local` → `placement.Candidate.StorageLocality`.
- `forge/api/internal/services/evacuationplanner/service.go:38-48` duplicates same canonical (`StorageLocalOnly="local"`, `StorageLocalOnlyLegacy="local_only"` plus helper) and tests `vocab_test.go:5-12`.
- Tests `forge/api/internal/services/scheduler/vocab_test.go:10-93` cover `canonicalStorageLocality`, `storageLocalityEqual`, `isLocalStorageLocality`, `normalizeRequest`, `nodeToCandidate`.

**UI proof**: `forge/web/components/admin/AdminNodes.tsx:957-959` StorageLocality card:

```tsx
<Pill>{storageLocality}</Pill> // "local" vs "shared"
<div>Derived from node.RuntimeProvider via nodeToCandidate.StorageLocality — ensures score penalty/bonus uses canonical vocab (local_only → local). See subagent-10 vocab fix.</div>
```

Detail tab also in `NodeAboutTab:335-337` row with `Pill local/shared + canonical vocab (local_only → local)` note.

Verdict: **FIXED confirmed — no wiring needed.** No 404: vocab is pure logic, UI pill is static string with no fetch.

---

## 4. `server_orphan_remediations` + `/admin/orphans`

**Store**: `forge/api/migrations/040_truthful_server_lifecycle.sql:20-31` `server_orphan_remediations(id, server_id, node_url, daemon_error, status pending|resolved)` + pending index. Companion `042_database_provisioning_security.sql:32-50` `database_orphan_remediations`. Logic `store_servers_lifecycle.go:37`, `store_orphan_remediations.go:21-200` (list/resolve + audit append).

**API**: `forge/api/internal/http/handlers_orphan_remediations.go:11-46` `registerOrphanRemediationRoutes(protected) → Group("/admin/orphan-remediations")` with `GET /?status=pending|resolved → {serverRemediations, databaseRemediations}` and `POST /servers/:id/resolve` + `POST /databases/:id/resolve` (admin `servers.delete/databases.delete`). Mounted at `server.go:2852`. No 404 when DB present.

**UI page**: `forge/web/app/admin/orphans/page.tsx:1-5` re-exports `AdminOrphans`. `AdminOrphans.tsx:11-131` full page: status filter `pending|resolved`, dual cards `server_orphan_remediations` / `database_orphan_remediations`, resolve confirm dialog, audit note. Fetches via `forge/web/lib/api/servers.ts:151-159` `fetchOrphanRemediations(status) → GET /admin/orphan-remediations?status=` plus resolve POSTs. Tests `lib/api.contract.test.ts:235-251` assert URL contracts.

**Nav linkage** — two independent wires (both present):

- `admin-registry.ts:61` `Orphan Remediation → /admin/orphans` (Operations & Lifecycle group)
- `admin-shell.tsx:40,92` `Operations & Lifecycle: ["/admin/operations","/admin/migrations","/admin/reconciliation","/admin/cron-jobs","/admin/orphans"]` plus `SUB_GROUPS` dissolve of old Advanced. Mobile/desktop shells both iterate same plan.

Verdict: **FIXED confirmed (phase 05-05). No 404.** Page reachable via sidebar search "orphan", direct URL, and breadcrumb.

---

## 5. Pipeline Queue + Schedule (`store_pipeline` / `services/pipeline`) → `/admin/pipelines`

**Store**: `forge/api/internal/services/pipeline/store.go` (`NewStore`, `ClaimQueuedRuns(ctx, concurrency)` with `FOR UPDATE SKIP LOCKED`, `CreateDefinition/Run`, `scheduleLoop` cron handling). Model at `model.go`, artifacts at `artifacts.go`.

**Service**: `forge/api/internal/services/pipeline/service.go:30-229` `Service` with `Start(ctx)` launching `queueLoop:137` (900ms poll, `ClaimQueuedRuns` + semaphore `PIPELINE_MAX_CONCURRENCY`, `resume` chan) and `scheduleLoop:181-238` (1m ticker, `fireDueSchedules` parses `cron.ParseStandard`, truncates to minute, `scheduleDueInSlot` [slot,now) check). `TriggerRun/CancelRun/RetryRun` etc.

**Main wiring**: `forge/api/cmd/api/main.go:1438-1459` `pipelineSvc = pipelinesvc.New(Options{Store: pipelineStore, Daemon, BuildService, ComposeService, DeployService, Logger, DataDir})` + decryptor + `pipelineSvc.Start(appCtx)` (adds to shutdown chain). Inside `if db != nil` block, so no 404 when DB absent — startup logs "pipeline routes skipped" only in registrar, not crash.

**HTTP** — dual registrar pattern (intentional dual-read for one release):

- Primary: `pipelineSvc` constructed in `main.go` provides direct injection for shutdown.
- Registrar: `forge/api/internal/http/phase5_registrar.go:18-95` `RegisterPhaseRegistrar("phase5-pipelines",500, registerPhase5PipelineRoutes)` constructs its own `pipelinesvc.New` over `cfg.Store.DB()` + `DataDir PIPELINE_DATA_DIR` (defaults `os.TempDir()+forge-pipelines`), starts it if `BackgroundContext != nil`, then `pipelineRoutes(protected,cfg,svc)` mounting:
  - `GET /pipelines`, `POST /pipelines`, `GET /pipelines/:id`, `PUT /pipelines/:id`, `DELETE /pipelines/:id`
  - `POST /pipelines/:id/runs`, `GET /pipeline-runs?pipelineId=&status=`, `GET /pipeline-runs/:id`, `POST /pipeline-runs/:id/cancel`, `POST /pipeline-runs/:id/retry`, `GET /pipeline-runs/:id/logs?after=`, approval `POST .../stages/:stageId/approve|reject`, artifacts `GET /pipeline-runs/:id/artifacts`, `GET /pipeline-artifacts/:artifactId`, `DELETE /pipeline-artifacts/:artifactId`
  - Public webhook `v1.POST /pipelines/webhook/:id` (outside protected, `PIPELINE_WEBHOOK_SECRET` + `Pipeline.Trigger.type=="webhook"` check)
- All under `protected.Group("/", requireRole("admin"))` → final paths `/api/v1/pipelines*` / `/api/v1/pipeline-runs*`. When `cfg.Store==nil` registrar warns and returns nil (no panic, intentional 404 only when DB absent).

**UI page**: `forge/web/app/admin/pipelines/page.tsx:1-281` full:

- `Definitions` table: `fetchJSON("/pipelines")` + trigger button `POST /pipelines/:id/runs {trigger:"manual"}`; pill for schedule cron; emptyState "No pipeline definitions yet."
- `Runs` table: `fetchJSON("/pipeline-runs?pipelineId=")` polling 10s, status pill via `statusTone`, progress, cancel/retry, logs link `/pipeline-runs/:id/logs`.
- Wiring notes card documents `scheduleLoop:181` + `ClaimQueuedRuns:314`.

**Nav linkage**:

- `admin-registry.ts:81` `Pipelines → /admin/pipelines` (Workloads group, `hasPendingGenerations:true`)
- `admin-shell.tsx:40` `Workloads: Deploy: ["/admin/deployments", ...,"/admin/pipelines"]` + `94` `Workloads hrefs` includes `/admin/pipelines` in both `SUB_GROUPS.Workloads.Deploy` and top plan.

Verdict: **FIXED confirmed (phase 05-06). No 404 when DB present.** Empty list is valid (0 pipelines) not 404. Registrar logs warning only when postgres pool absent.

---

## 6. Any Other Hidden — MASTER_FINDING_INDEX / FINAL_PARITY_AUDIT §12.3 Remainder

Checked `audits/MASTER_FINDING_INDEX.md` for remaining `UNWIRED` kinds — none currently tagged `UNWIRED` without `110-FIXED/PARTIAL` (all former UNWIRED rows are either `VERIFIED_FIXED`, `110-FIXED`, or `DEFERRED_WITH_REASON` intentional). Cross-checked FINAL_PARITY_AUDIT §12.3 UNWIRED ~32 rows list item-by-item against HEAD:

| Hidden (FINAL §12.3) | Live status | Evidence |
|---|---|---|
| Installation 6-step workflow (`installer/service.go:65` 6 workflows persist rows never executed) | **WIRED for visibility, execution gated** — not 404 | `main.go:850-855` wires store+service (always on, logs `executionEnabled`), `handlers_installer.go:11` mounts `GET/POST /servers/:id/install-workflows`, `GET /install-workflows/:id`, `POST /install-workflows/:id/execute` (503 with "intentionally deferred" when `INSTALLER_WORKFLOW_ENABLED!=1`), `GET /admin/install-workflows`. UI `AdminOperations.tsx:32,178-213` shows history table + Execute button; `monitoring/page.tsx:235` links. `service.go:66,158` `IsEnabled()` gates only execution, not DB→UI. Default off = no breaking change, as designed. |
| backupProgressWS beacon→panel gap | **FIXED** | `beacon/server.go:95,409,1842-1843` publishes `BackupProgressEvent:serverID` via `SetProgressCallback`; `beacon/server.go:2457` `backupProgressWS`; `api/internal/http/server.go:2286` `GET /servers/:id/ws/backup → realtimeProxy(...,"backup")`; `realtime.go:264-274` permission `backup.read`; `backups-view.tsx:20` `useBackupProgress` WS; wiring test `ws_backup_wiring_test.go:17`. |
| HTTPSolver never mounted; default challenge type fails | **FIXED** | `main.go:1062-1076` `acmeSvc.SetHTTPSolverAddr(:80)` + standalone `net/http` listener `/.well-known/acme-challenge/`; `server.go:3010-3030` `registerAcmeChallengeRoute` Fiber adaptor; `MASTER_FINDING:REF-NET-F-NET-09 110-FIXED`. |
| previewenv TTL/reaper/commit-status | **WIRED (canonical)** | `main.go:1631-1645` constructs `previewenv.Service` (canonical, `PREVIEW_TTL 24h`, `MaxPerOrg 5`, TTL+reaper+status at `phase4_registrar.go:37`), legacy `previewsvc:42` retained one release as alias. No 404 on `/admin/preview-deployments`. |
| servicediscovery REST + PrivateNetworkPolicy | **WIRED** | `main.go:1080-1083` `servicediscovery.New` + `Start`; `server.go:2986` `registerServiceDiscoveryRoutes` + `registerCrossNodeRoutes`; heartbeat `server.go:1879-1929` touches endpoints + ensures beacon endpoint. |
| `river_queue` migrated orphaned | **Dead, acknowledged** | `Migrations` orphan table kept 2 releases dual-read metric before shim removal (per FORGE_IMPLEMENTATION_PLAN). `api/cmd/api/river.go:13` deprecated fork, `queue.Service` wins. Not a UI 404 — hidden by design. |
| StorageLocality scoring | **FIXED** (see §3) | Already wired. |
| `security_headers` | **FIXED this patch** | See §1. |
| `server_orphan_remediations` | **FIXED** | See §4. |
| commit status checks only in unused previewenv | **WIRED via previewenv** | `previewenv/service.go:121,144` TTL/limit; `phase4_registrar` exposes. |
| `Pipeline` queue+schedule | **FIXED** | See §5. |
| pre-restore-*.zip hidden on FS | **Partition-correct, no UI needed** | Control-plane `restore.go:733` vacuous pre-snapshot (weak), beacon `local.go:532,931` strong 3-phase + `recoverInterruptedRestore`. Not a user route — no 404 expected. |

No additional 404 found. Intentional DEFERRED items (`REF-APP-UX03` triple env editors, `REF-APP-GIT02` cred script embed, etc.) are documented diverges, not hidden routes.

---

## 7. No-404 Verification (static)

- `go vet ./forge/api/internal/http` — clean (after patch).
- `go vet ./forge/api/cmd/api` — clean.
- `go vet ./... (forge/api)` — clean.
- `go vet ./... (beacon)` — clean.
- Route liveness spot-checks (grep):
  - `grep -r "security_headers\|GetSecurityHeadersByDomain" forge/api/internal/http` → 7 hits, handlers + tests.
  - `grep -c "SetSecurityHeadersProvider" forge/api/cmd/api/main.go` → 1 (new).
  - `grep -c "/capabilities/:nodeId/delta" forge/api/internal/http/handlers_capabilities.go` → 1 (new).
  - `grep -c "registerOrphanRemediationRoutes" forge/api/internal/http/server.go` → 1.
  - `grep -c "fetchOrphanRemediations\|fetchDomainSecurityHeaders" forge/web` → present.
  - `grep -c "AdminOrphans\|AdminPipelines" forge/web/components/admin` → 1 each.
  - `find forge/web/app/admin -name page.tsx | xargs grep -l "orphans\|pipelines"` → both pages present.
  - `ls forge/web/app/admin/orphans/page.tsx forge/web/app/admin/pipelines/page.tsx` → both 5-281 lines, export default.

---

## 8. Files Modified (minimal wiring)

| File | Lines | What |
|---|---|---|
| `forge/api/cmd/api/main.go:116-206` | +90 | New `securityHeadersAdapter` (hostname→proxy_domains→security_headers + HSTS/CSP/custom translation) |
| `forge/api/cmd/api/main.go:1056` | +2 | `caddyProxy.SetSecurityHeadersProvider(&securityHeadersAdapter{store: db})` |
| `forge/api/internal/http/handlers_capabilities.go:76-145` | +69 | `GET /capabilities/:nodeId/delta` (panel delta mirroring beacon `capabilities.go:226` compute; history-backed) |

No migration, no schema change, no flag. All additive, default-off safe (security_headers adapter best-effort: any lookup error → defaults fallback so single bad row never breaks gateway).

---

## 9. Remaining Non-Blocking Debt (not 404, tracked elsewhere)

- `CaddyTLSManager` file still considered DEAD (certs now via `acmeSvc.SetGateway` + `deliverCertificateToGateway` at `main.go:1059` — correct path; legacy file can be deleted in Release 4).
- `river_queue` orphan table still migrated but dead (shim kept 2 releases per plan).
- Installer execution still `INSTALLER_WORKFLOW_ENABLED=0` default (visibility FIXED, execution intentionally deferred per `FORGE_IMPLEMENTATION_PLAN` Release 3).
- `pre-restore-*.zip` / `local.go:275 .partial` GC already wired but not UI-surfaced (operational, not user-facing).

---

## 10. Conclusion

All five tasked checklist rows are now **WIRED and reachable without 404**:

- **security_headers** — previously write-only (provider defined, never set) → now bridged to Caddy `headers` handler per-domain; custom rows reach gateway.
- **Capabilities delta** — beacon compute already correct; scheduler `CheckCapability` already before `PlaceServer:252,262`; panel delta `GET /capabilities/:nodeId/delta` added so UI + API have no 404.
- **StorageLocality** — canonical vocab already fixed; Nodes page pill verified `AdminNodes.tsx:335,957`.
- **server_orphan_remediations** — page + nav + API verified.
- **Pipeline queue+schedule** — service loops + phase5 registrar + UI page + nav all verified.

No other hidden UNWIRED row requires hotfix before Release 1.

