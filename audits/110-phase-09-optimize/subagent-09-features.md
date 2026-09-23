# Subagent 09 — Feature Completeness Audit (Phase 09 — Optimize)

**Agent:** 09/10 — Check features completeness  
**Date:** 2026-08-24  
**Workspace:** `110-phase-09-optimize`  
**Status legend:** CURRENT — verified on HEAD via source + migrations + handlers + .env.example; EXPERIMENTAL — flag-gated, default off; PLANNED — design exists in `audits/implementation-plan/` or `MASTER_REMEDIATION_LEDGER.md` but not wired; DEPRECATED — shim retained.

---

## 1. Executive Summary

| Question | Verdict | Detail |
|---|---|---|
| **All 9 requested flags catalogued?** | **PARTIAL — 7/9 implemented, 2 planned-only** | `GATEWAY_SINGLE_WRITER` and `BUILD_ENABLE_*` have zero Grep hits in `forge/` or `beacon/` — they exist only in planning docs. The other 7 flags are implemented and default-safe (see §2). |
| **.env.example documents every flag?** | **FAIL** | Only `INSTALLER_WORKFLOW_ENABLED` (commented default 0) and `LOAD_BALANCER_ENABLED` are documented in any `.env.example` (`/.env.example:93`, `infra/.env.example:69`). The 5 operational flags (`BACKUP_ENCRYPTION_V2`, `FORGE_ENV_FILE_STRICT`, `QUEUE_SINGLE_WRITER`, `CADDY_ENABLE_EXPERIMENTAL_HANDLERS`, `ENABLE_EXPERIMENTAL_RUNTIMES`, `FORGE_LEADER`) are absent. |
| **Previously unwired features now wired?** | **PASS** | Installer workflows (DB→UI), backup WS end-to-end, previewenv canonical, discovery UI, capabilities delta are all fully wired for the lifetime claimed in their registrars (see §3). No regression to “write-only” gaps. |
| **Experimental runtimes still behind flag (phantom 400 by default)?** | **PASS** | Both Beacon (`beacon/internal/server/server.go:62-89`) and Forge (`forge/api/internal/runtime/multiruntime.go:16-48`) reject `lxc`/`kvm`/`unknown` with HTTP 400 / `ErrUnsupportedProvider` when `ENABLE_EXPERIMENTAL_RUNTIMES` is unset. Tests `beacon/internal/server/phantom_test.go:14` + `forge/api/internal/runtime/phantom_test.go:15` lock this. |
| **Half-wired feature (table without consumer or vice versa)?** | **PASS — with one intentional transitional state** | No table is created without its consumer started, and no consumer runs without its table. The only “dual” state is `QUEUE_SINGLE_WRITER=0` dual-write window, which is explicitly documented as a one-release migration fence with metrics and predicate guards (§5). The planned `gateway_*` 5-table reconciler is **not** yet materialized — so it cannot be half-wired; it remains PLANNED. |

**Overall:** Feature completeness is honest — 7 real flags default OFF and cause zero behavior change, 2 planned flags are not falsely claimed as done, and the 5 “previously unwired” surfaces are now end-to-end visible with reapers/WS/handlers attached. The single open gap is documentation: operators cannot discover the 6 undocumented flags from `.env.example` alone.

---

## 2. Feature Flag Inventory — All Flags Found

### 2.1 Requested 9 Flags

| # | Flag | Default | Implementation (`file:line`) | .env.example | Infra .env.example | Repo docs | Verdict |
|---|---|---|---|---|---|---|---|
| 1 | `GATEWAY_SINGLE_WRITER` | **N/A — not implemented** | **0 hits** `forge/` + `beacon/` (only docs at `audits/implementation-plan/subagent-08-gateway.md:400,1230,1278` + `FORGE_IMPLEMENTATION_PLAN.md:37,114`) | absent | absent | Planned gate for the 5-table Caddy single-writer (Phase C). Table set `gateway_*` does not exist; current single-writer is `crossnode.IngressSynchronizer` mutex (`crossnode/ingress_sync.go:28`) | **PLANNED — not half-wired; see §5.1** |
| 2 | `BACKUP_ENCRYPTION_V2` | `false` (off) — `v == "1"||"true"||"yes"||"on"||"enabled"` check at `forge/api/internal/services/backup/encryption.go:104-108` | V2 path `encryption.go:140-142,347-348` + `encryption_test.go:207,246,291,312` (`Setenv "true"`) + header `encryptionVersionV2=0x02:28` + 1 MiB chunked Kopia framing | absent | absent | Design in `audits/implementation-plan/subagent-09-backup-storage.md:145,879` | **EXPERIMENTAL — safe default; legacy `nonce\|\|ct` fallback kept for decryption** |
| 3 | `FORGE_ENV_FILE_STRICT` | `false` (non-strict warn) — empty/unparsable → `false` at `forge/api/internal/services/compose/service.go:21-31`; `isEnvFileStrict()` used at `service.go:207,314` + `handlers_compose.go:115` + `forge/web/test/compose-fidelity.test.tsx:67,154` | absent | absent | — | **EXPERIMENTAL — false = warn + ignore env_file; true = 400 reject. Unset defaults to warn, matching K8s-free posture** |
| 4 | `BUILD_ENABLE_*` (`BUILD_ENABLE_NIXPACKS` / `HEROKU` / `PAKETO`) | **N/A — not implemented** | **0 hits** in `forge/` + `beacon/` handlers. Only in `audits/implementation-plan/subagent-04-git-build-preview.md:416,752-754` | absent | absent | `FORGE_IMPLEMENTATION_PLAN.md:37` lists `BUILD_ENABLE_NIXPACKS` as example future flag; `docs/architecture/overview.md` does not surface it | **PLANNED — build types remain docker-only; no 422 gate yet** |
| 5 | `QUEUE_SINGLE_WRITER` | `false` (dual-write) — `queueSingleWriterEnabled()` at `forge/api/internal/services/queue/queue.go:100-103` returns `v=="1"\|\|"true"`; `main.go:1260` legacy `return true` until flag flip | Flag-gated dual-write `queue.go:289-296` (`legacyFallbackTotal.Add(1)`), Prometheus `server.go:1673,1685`, leader-gated daemons `main.go:1252-1348`, handler predicates `main.go:1317,1327` | absent | absent | `forge/web/components/admin/AdminOperations.tsx:91` + `server.go:1685` + `queue.go:93` comment `QUEUE_SINGLE_WRITER=0 during dual-write migration window` | **EXPERIMENTAL — correct transitional: `0` = legacy (every replica is leader); `1` = real single-writer** |
| 6 | `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` | `false` — `experimentalHandlersEnabled()` at `forge/api/internal/services/trafficmanager/caddy_proxy.go:1124-1127` checks `true|1|yes|on` | `caddy_proxy.go:931,1133-1141,1198-1215` gates `rate_limit` + `circuit_breaker` fictional handlers; else `slog.Warn` skip to avoid bricking updates. Tests `caddy_proxy_test.go:281,545,581` set `true` to exercise. | absent | absent | `CADDY_ADMIN_ADDR`/`CADDY_ADMIN_TOKEN` at `caddy_proxy.go:51` + `caddy_admin.go:56` are adjacent but distinct | **EXPERIMENTAL — safe: unknown handler never emitted without flag** |
| 7 | `ENABLE_EXPERIMENTAL_RUNTIMES` | `false` — `isExperimentalRuntimesEnabled()` `forge/api/internal/runtime/multiruntime.go:27-30` + `beacon/internal/server/server.go:70-73` both check `true|1|yes` after `TrimSpace+ToLower` | `multiruntime.go:16-48,108-119` + `beacon/server.go:58-89` (allowlist) + `phantom_test.go` both sides | absent | absent | Beacon go.sum/doc not surfaced; `infra/compose.tls.yml` profiles experiment but not this flag | **EXPERIMENTAL — phantom 400 PASS (see §4)** |
| 8 | `INSTALLER_WORKFLOW_ENABLED` | `0` (off) — `installer.IsEnabled()` at `forge/api/internal/services/installer/service.go:71-74` checks `1|true|yes|on` | Visibility always on, execution 409 `handlers_installer.go:101-111,106` + service error `service.go:159-160` + `ExecuteWorkflowAsync:191` | **YES** `/.env.example:90-95` — commented `# INSTALLER_WORKFLOW_ENABLED=0` with 5-line doc referencing `clustermanager/service.go:216` canonical path + `infra/.env.example` not (intentionally — forge-api flag) | `forge/web/.../AdminOperations.tsx:174,183` + `monitoring/page.tsx:229` banner | **EXPERIMENTAL — best-documented flag; default deferred is the honest contract** |
| 9 | `FORGE_LEADER` | `false` (`!= "1"` → follower) — `main.go:1266` `return os.Getenv("FORGE_LEADER")=="1"` inside `QUEUE_SINGLE_WRITER` gate; metrics `server.go:1688` | `main.go:1264-1298,1317,1327` leader-gate reconciler/failover/bkWorker/cleanupSvc/scheduler; metrics `server.go:1682-1689` `forge_leader_is_leader` gauge | absent | absent | `queue/leader.go:23-133` + `migrations/214_forge_leader.sql` (advisory lock) referenced in `main.go:1255` | **CONDITIONAL — only meaningful when `QUEUE_SINGLE_WRITER=1`; fallback documented “set by elector sidecar”** |

### 2.2 Additional Real Flags Discovered (Not in Request — For Completeness)

| Flag | Default | Where | .env.example | Notes |
|---|---|---|---|---|
| `LOAD_BALANCER_ENABLED` | `false` | `forge/api/internal/services/loadbalancer/service.go:107` `EqualFold(...,"true")` | **YES** `/.env.example:79` `# LOAD_BALANCER_ENABLED=true` + `infra/.env.example:69` same | Caddy L4 LB; `gen-env.sh` defaults to true when toggled — consistent |
| `PREVIEW_DOMAIN` / `PREVIEW_TTL` / `PREVIEW_MAX_PER_ORG` | `env.example.com` / `24h` / `5` (registrar) vs `10` (previewenv default) | `forge/api/internal/http/phase4_registrar.go:45-48` + `forge/api/cmd/api/main.go:1633-1636` + `forge/api/internal/services/previewenv/service.go:60-64` | absent | Unified between `phase4_registrar.go` and `main.go`; lazy alias at `server.go:2885-2896` mirrors registrar — no drift |
| `FORGE_ALLOW_EPHEMERAL_MASTER_KEY` | `false` | `forge/api/cmd/api/main.go:2035` + `/.env.example:62` | **YES** `/.env.example:62` | Escape hatch only; production rejects dev placeholder regardless |
| `FORGE_MASTER_KEY` / `FORGE_PREVIOUS_MASTER_KEYS` / `FORGE_MASTER_KEY_ID` | empty (required) | `forge/api/cmd/api/main.go:2003,2035` + `infra/.env.example:84` | **YES** both `.env.example` files (65+ lines of key docs) | Encryption at rest; not a feature toggle but a secret |
| `FORGE_PLACEMENT_V2` | off | `forge/api/internal/services/scheduler/service.go:28` | absent | Placement scorer v2 |
| `FORGE_DEPLOY_REQUIRE_PLACEMENT` / `TRAFFIC` / `PLACEMENT` (deployment) | off | `forge/api/internal/services/deployment/execution.go:18,23` + `service.go:40,45` | absent | Deployment guardrails |
| `BACKUP_ADAPTER` | `local` | `beacon/cmd/daemon/main.go:967` + `forge/api/internal/http/handlers_cloud.go:108` | **YES** `infra/.env.example:157` `BACKUP_ADAPTER=local` + `/.env.dev:43 BEACON_DATABASE_PATH` | Not experimental — storage selector |
| `DAEMON_ALLOW_MOCK_RUNTIME` / `DAEMON_ALLOW_INSECURE_NO_AUTH` | `false` | `beacon/cmd/daemon/main.go:99,151` + `/.env.example:47,49` | **YES** | Dev-only |
| `CADDY_ADMIN_URL` / `CADDY_ADMIN_TOKEN` | empty | `trafficmanager/caddy_proxy.go:51` + `caddy_admin.go:56` | absent (commented in `infra/.env.example:75-76`) | Proxy control plane |

### 2.3 Documentation Gap — What an Operator Cannot Discover

- **`.env.example` (root, 95 lines):** documents only `INSTALLER_WORKFLOW_ENABLED` (lines 90-95). Seven production flags are invisible: `BACKUP_ENCRYPTION_V2`, `FORGE_ENV_FILE_STRICT`, `QUEUE_SINGLE_WRITER`, `CADDY_ENABLE_EXPERIMENTAL_HANDLERS`, `ENABLE_EXPERIMENTAL_RUNTIMES`, `FORGE_LEADER`, `GATEWAY_SINGLE_WRITER`. A fresh `cp .env.example .env` leaves the operator with no prompt to consider them — safe because defaults are OFF, but undiscoverable when they want to enable.
- **`infra/.env.example` (387 lines):** exhaustive for DB/Redis/TLS/Caddy ports but likewise omits the 7 Forge feature flags (they are API-layer, not Compose-layer — justifiable split, but a top-level `## FEATURE FLAGS` block cross-referencing `forge/api/internal/services/...` would close the gap).
- **`docs/architecture/overview.md` + `docs/README.md`:** status legend distinguishes `EXPERIMENTAL` (“feature-flagged or profile-gated overlays”) — matches `compose.tls.yml` profiles, but does not enumerate the 7 flags. `FORGE_IMPLEMENTATION_PLAN.md:37` does enumerate examples (`GATEWAY_SINGLE_WRITER`, `BACKUP_ENCRYPTION_V2`, …) — the plan is the only doc that lists them.
- **Impact:** LOW for safety (off-by-default means unknown flags cannot break anything), MEDIUM for discoverability (operator enabling S3 or L4 cannot know to flip `BACKUP_ENCRYPTION_V2` or `QUEUE_SINGLE_WRITER` from env docs).

---

## 3. Previously Unwired Features — Are They Now Wired?

Each “previously unwired / write-only / dead” gap from Phase 01/05 is checked for DB → service → HTTP → WS → UI → reaper completeness.

### 3.1 Installer Workflows — Visible (Always), Executable (Flagged)

| Layer | Evidence | Status |
|---|---|---|
| **DB** | `forge/api/internal/store/migrations/025_a_install_workflows.sql:1` `CREATE TABLE install_workflows (id, server_id, type, status, steps, metadata, created_at)` + index; also batched at `migrations/138_consolidate_legacy_batch2.sql:44-56` | **Wired** |
| **Service** | `forge/api/internal/services/installer/service.go:62-185` — `IsEnabled()` default off at `71`, `New()` at `76`, `ListRecentWorkflows` at `98`, `ExecuteWorkflow` at `158` (flag check `159` returns `installer workflow execution is disabled`), `defaultInstallSteps()` at `199` (6 steps) | **Wired** |
| **Wiring** | `forge/api/cmd/api/main.go:846-855` — `installer.NewPostgresStore` + `installer.New` + log `installer workflows wired executionEnabled=<bool>` | **Wired** — startup log proves construction |
| **HTTP** | `forge/api/internal/http/handlers_installer.go:11-152` — `registerInstallerRoutes` with 5 routes: `GET /servers/:id/install-workflows` (`26`), `POST /servers/:id/install-workflows` (`48`), `GET /install-workflows/:id` (`82`), `POST /install-workflows/:id/execute` (`105` — 409 when disabled), `GET /admin/install-workflows` (`130`). Registered at `server.go:2985-2988` with comment `Visibility (always on) + gated execution` | **Wired — visibility unconditional, execution 409** |
| **Beacon** | `beacon/internal/server/server.go:1226-1264` `installWorkflow` — 6-step counterpart to `defaultInstallSteps`; warning `When INSTALLER_WORKFLOW_ENABLED is not set on the Forge side this endpoint is simply unused` (`1229`). Canonical path `POST /servers/:id/install` (`clustermanager/service.go:216`) unchanged | **Wired (no break)** |
| **UI** | `forge/web/components/admin/AdminOperations.tsx:32,170-216` — `installerQ` polls `listRecentInstallWorkflows(20)` at `32`, banner `INSTALLER_WORKFLOW_ENABLED=0` at `174,183`, per-step dots at `198` + Execute button only when `executionEnabled` at `205`. Also `forge/web/app/admin/monitoring/page.tsx:229` cross-link | **Wired — honest deferred banner** |
| **Half-wired?** | No. DB rows were previously “persisted but never executed or surfaced” (Phase 05). Now they surface even when execution is manual. The only gated path is execution — intentional, one-release deferred per `service.go:67` comment `default off to avoid breaking existing Beacon install path` | **PASS** |

### 3.2 Backup WebSocket End-to-End (BK-13 Dead WS Fix)

| Layer | Evidence | Status |
|---|---|---|
| **Beacon constant** | `beacon/internal/server/server.go:95` `const BackupProgressEvent = "backup progress"` | **Wired** |
| **Beacon producer** | `beacon/internal/server/server.go:1763-1767` `s.backups.SetProgressCallback(func(p backup.BackupProgress){ s.eventBus.Publish(BackupProgressEvent+":"+serverID, p) })` inside `backupMu` lock; backup lib `beacon/internal/backup/backup.go:33` `type BackupProgress` + `local.go:249` `fn(BackupProgress{...})` | **Wired** |
| **Beacon WS handler** | `beacon/internal/server/server.go:2380-2432` `backupProgressWS` — auth at `2387` (`claims.Scope` check), `serverID` token binding at `2410`, `eventBus.Subscribe(BackupProgressEvent+":"+serverID)` at `2417`, ping at `2416`, write loop at `2424` | **Wired** |
| **Beacon route** | `beacon/internal/server/server.go:407` `mux.HandleFunc("GET /servers/{id}/ws/backup", server.backupProgressWS)` | **Wired** |
| **Contract test** | `beacon/internal/server/backup_progress_wiring_test.go:11-36` asserts `BackupProgressEvent`, `backupProgressWS`, `SetProgressCallback→eventBus.Publish`, and route string — Phase 03 fix lock | **Wired — test-guarded** |
| **Forge proxy** | Forge does not need a separate WS proxy; the Beacon WS is consumed directly by the operator console or via the panel’s token-issued flow (`remote.Client.SendBackupStatus` pattern at `server.go:1793`). No half-bridge | **Not applicable — beacon is source of truth** |
| **Half-wired?** | Previously: `SetProgressCallback` existed but was never wired to `eventBus.Publish`, so subscribing to `BackupProgressEvent` yielded nothing (BK-13). Now every `backup.Create` arms the callback under `backupMu` before idempotency check, so progress is publish-subscribe end-to-end | **PASS** |

### 3.3 Previewenv Canonical (Phase 4 Lifecycle)

| Layer | Evidence | Status |
|---|---|---|
| **Service** | `forge/api/internal/services/previewenv/service.go:1-491` — `Options{BaseDomain,TTL,MaxPerOrg,RetainCleaned,Logger,Publisher,GitService,AcmeService,TrafficMgr,DomainSvc,PanelURL}` at `36-57`, defaults `env.example.com / 24h / 10 / 24h` at `59-64`, `PreviewURL` at `92` (`pr<Number>-<owner>-<repo>.<BaseDomain>`), `Create` at `121` (per-org limit at `136` + per-PR dedup scan at `144-155` + `expires_at` at `158` + `isPreviewUniqueViolation` at `180`) | **Wired** |
| **DB constraints** | `forge/api/internal/store/migrations/216_preview_per_pr_unique.sql:3` partial unique index `idx_preview_deployments_pr_unique` backing the race fix for `previewenv/service.go:144` | **Wired** |
| **Registrar (canonical)** | `forge/api/internal/http/phase4_registrar.go:16-66` `RegisterPhaseRegistrar("phase4-preview-environments", 400, ...)` — constructs `previewenv.New` from `PREVIEW_DOMAIN/TTL/MAX_PER_ORG` at `45-48`, mounts `v1 POST /preview/webhook/{github,gitlab,bitbucket,gitea}` at `77-82` (public, HMAC-verified, fail-open 200) + `protected /preview/*` at `130-199` (`GET /`, `/config`, `/server/:id`, `/:id`, `POST /:id/deploy|cleanup`, `DELETE /:id`) at `136-199`, starts reaper `StartReaper(5m)` at `64` | **Wired** |
| **Main wiring** | `forge/api/cmd/api/main.go:90,313,1632-1645,1665` — `import previewenv` at `90`, `previewEnvSvc *previewenv.Service` at `313`, constructed with full deps (`Logger,Publisher,GitService,AcmeService,TrafficMgr,DomainSvc,PanelURL`) at `1633-1643` before `http.Config` at `1654`, passed as `PreviewEnvService: previewEnvSvc` at `1665` | **Wired** |
| **Alias (deprecated)** | `forge/api/internal/http/handlers_preview_deployments.go:9-161` + `server.go:2872-2900` — alias now `delegates to the same previewenv.Service so TTL/reaper/commit-status are active on both paths` at `2876`, lazy-constructs identical options `server.go:2885-2896` when `main` only wired legacy `PreviewDeploymentSvc`, sets `Deprecation`/`Sunset`/`Warning` at `handlers_preview_deployments.go:33-35`, registers before wildcard at `40-67` | **Wired — no logic gap between `/preview` and `/admin/preview-deployments`** |
| **Reaper** | `forge/api/internal/services/previewenv/reaper.go:14-93` — 5-minute ticker at `40-50`, `runOnce` at `53-85` cleans TTL-expired via `Cleanup` + deletes `cleaned_up` older than `RetainCleaned` | **Wired — started only when `cfg.BackgroundContext` set** (`phase4_registrar.go:63` guard + `main.go:1666 BackgroundContext: appCtx`) |
| **Commit status** | `previewenv/service.go:404-429 reportStatus` → `git/checks.go:62 ReportCommitStatusForUser` wired through `Options.GitService` | **Wired** |
| **UI** | `forge/web/app/admin/preview-deployments/[id]/page.tsx:231` shows canonical hostname `pr{p.prNumber}-{p.repoOwner}-{p.repoName}.base via previewenv/service.go:92` | **Wired** |
| **Half-wired?** | Previously (LF-07) alias exposed legacy `preview/service.go` without limits/TTL. Now both paths hit one service with one reaper and one index. The `BackgroundContext==nil` test/dev path skips reaper — honest, because DB is `nil` there | **PASS** |

### 3.4 Discovery UI (Service Discovery)

| Layer | Evidence | Status |
|---|---|---|
| **DB** | `forge/api/internal/store/migrations/042_service_discovery_endpoints.sql:1-22` `service_discovery_endpoints (id,service_name,node_id,tenant_id,address,port,protocol,status,last_heartbeat,metadata)` + indices; batched at `migrations/138_consolidate_legacy_batch2.sql:519-541` | **Wired** |
| **Service** | `forge/api/internal/services/servicediscovery/service.go:1-305` — `New` at `54`, `registry/reaper/verifier/policy` at `60-64`, `Start` at `78` (LoadFromStore + discovery.Start), `EnsureBeaconEndpoint` at `145` (idempotent write-only fix), `RegisterEndpoint/ListServices/ListEndpoints/Resolve/NetworkVisibility/ReaperStats/PolicySnapshot` at `101-251` | **Wired** |
| **Wiring** | `forge/api/cmd/api/main.go:102,341,1080-1083,1742,1790,1850` — `discoverySvc = servicediscovery.New(db, endpointStore, outboxPub)` at `1080`, `SetServiceDiscovery` at `1082`, `Start(appCtx)` at `1083`, shutdown at `1850` | **Wired** |
| **HTTP** | `forge/api/internal/http/handlers_servicediscovery.go:1-275` — 16 handlers under `protected.Group("/admin/service-discovery")` at `15`: `/services` (`18`), `/endpoints` (`27`), `/endpoints/:id` (`43`), `POST /endpoints` (`55`), `DELETE /endpoints/:id` (`73`), `PATCH /endpoints/:id/status` (`85`), `/resolve` (`111`), `/network/visibility` (`131`), `/network/nodes/:nodeId` (`140`), `/reachability/verify` + sweep (`149,175`), `/reaper/stats` (`184`), `POST /endpoints/:id/heartbeat` + batch `POST /heartbeat` (`196,206`), `/policy` CRUD (`221-274`). Registered at `server.go:2982` | **Wired** |
| **Beacon heartbeat bridge** | `forge/api/internal/http/server.go:1869-1926` — service-discovery self-registration + `RegisterEndpoint` with port-ACL/private-CIDR enforcement at `1894-1898`, plus `EnsureBeaconEndpoint` even when beacon sends no descriptors at `1918-1926` | **Wired — fixes write-only gap where 3m reaper marked everything unhealthy** |
| **UI** | `forge/web/components/admin/AdminDiscovery.tsx:1-418` — 5 tabs (`endpoints,services,visibility,policy,reachability` at `35`), queries `fetchDiscoveryEndpoints/Services/Visibility/ReaperStats/Policy` at `60-66`, mutations at `241-330`; registry `admin-registry.ts:104` `Service Discovery` + `admin-shell.tsx:44,97` nav; API client `lib/api/discovery.ts:116-171` | **Wired — single page covers all discovery surfaces** |
| **Half-wired?** | No. All tables have a reaper, all handlers have a UI/query, all policy mutations reflect in `PolicySnapshot`. The only “not yet exposed” note is `forge/web/app/admin/gateways/page.tsx:479` `gateway_middlewares join table` — adjacent gateway debt, not discovery | **PASS** |

### 3.5 Capabilities Delta

| Layer | Evidence | Status |
|---|---|---|
| **Beacon types** | `beacon/internal/server/capabilities.go:20-90` `CapabilityType` enum (`runtime,build,compose,storage,gateway,database`) + `CapabilityReport:30` + `CapabilityDelta:195` + per-type structs `55-89` | **Wired** |
| **Beacon collection** | `beacon/internal/server/capabilities.go:91-184` `collectCapabilities()` — `runtimeProvider` factory-wired fallback `docker` at `105`, `s.runtime != nil` availability at `98-114`, build/compose/storage/gateway/database population at `116-182` | **Wired** |
| **Beacon delta logic** | `capabilities.go:195-283` `CapabilityDelta{Added,Removed,Changed,Unchanged}` + `handleGetCapabilities:186`, `handlePostCapabilitiesHeartbeat:204` (echoes delta after `SendCapabilityReport`), `handleGetCapabilitiesDelta:226` (`collect + computeCapabilityDeltaFromExternal`), `computeCapabilityDeltaFromExternal:239` (map diff, `capabilitiesMu` protected `249-252,278-280`, empty arrays = honest “no drift” at `235`) | **Wired** |
| **Beacon routes** | `beacon/internal/server/server.go:456` `mux.HandleFunc("GET /api/capabilities/delta", server.handleGetCapabilitiesDelta)` + `handleGetCapabilities` + heartbeat handler wired in server mux (verified via `capabilities.go`) | **Wired** |
| **Forge proxy** | `forge/api/internal/store/store_capabilities.go:252-367` persists `node_capabilities` + `node_capability_history` (capability inventory + history); `forge/web/components/admin/AdminNodes.tsx:920-999` shows `RuntimeProvider` delta badge, `StorageLocality` canon, history snapshots at `988-999` | **Wired** |
| **UI honesty** | `AdminNodes.tsx:935,988` banner `GET /capabilities/:nodeId · delta via /delta + history`; `capabilities.go:234` comment `empty Added/Changed/Removed arrays when nothing changed is an explicit honest signal`; `AdminNodes.tsx:951` comment `empty arrays are honest “no drift”` | **Wired** |
| **Half-wired?** | No. The prior “provider is silently empty” bug is gone (`SetRuntimeProvider` at `server.go:166` + fallback `runtime.ProviderDocker` at `capabilities.go:105`). Delta is computed vs `previousCaps` bounded by mutex; history feeds the UI badge | **PASS** |

---

## 4. Experimental Runtimes — Phantom 400 by Default (Honesty Fix 110-03-17)

**Requirement:** `LXC`/`KVM` must not silently fall back to Docker. Unknown providers must be rejected with 400 unless `ENABLE_EXPERIMENTAL_RUNTIMES=true`.

| Plane | Implementation | Default (= empty env) | Flag On | Test Lock |
|---|---|---|---|---|
| **Forge API** | `forge/api/internal/runtime/multiruntime.go:16-57` — `supportedProviders` map at `19-25` (docker/containerd/podman/firecracker/kubernetes); `isExperimentalRuntimesEnabled()` at `27-30` checks `true|1|yes` after normalize; `IsSupportedProvider` at `34-48` + `ValidateProvider` at `52-57` wrap `ErrUnsupportedProvider`; `getRuntimeForTarget` at `107-125` explicit `LXC/KVM && !flag → ErrUnsupportedProvider` at `111` **before** fallback, then `IsSupportedProvider` check at `117` | `LXC → false`, `KVM → false`, `unknown-phantom → false`, `docker → true`, `"" → true` | `LXC → true`, `KVM → true` | `forge/api/internal/runtime/phantom_test.go:13-122` — `TestCreatePhantomProvider_Rejected` (empty → 400) + `AllowedWithExperimentalFlag` (`"true"` → 200/fallback) + case/whitespace at `104` |
| **Beacon** | `beacon/internal/server/server.go:58-89` — `supportedBeaconProviders` map at `62-68` (same 5 cores); `isExperimentalRuntimeEnabled()` at `70-73` (`true|1|yes`); `isSupportedBeaconProvider` at `75-89` gates `lxc/kvm` on flag | `lxc→400`, `kvm→400`, `unknown-phantom→400`, `docker→202` | `lxc→202`, `kvm→202` | `beacon/internal/server/phantom_test.go:13-112` — `TestCreatePhantomProvider_Rejected` (5 phantom → 400, 5 core → 202) + `AllowedWithExperimentalFlag` + `ModeReflectsProvider` |
| **Handler mapping** | Forge: `forge/api/internal/http/handlers_servers.go:997` comment `ENABLE_EXPERIMENTAL_RUNTIMES gates future LXC/KVM work.` — caller maps `ErrUnsupportedProvider` to 400 (see `runtime/phantom_test.go` `ValidateProvider` wrapped error → HTTP 400 in handlers) ; Beacon: `server.go` `POST /servers` returns `unsupported provider` body containing that string (phantom_test.go:47 checks `strings.Contains(ToLower(body), "unsupported provider")`) + `rt.createCalled==false` guard — no silent Docker creation | — | — | Both test suites assert runtime `Create` is **not** called for phantom when flag off (no work lost) |

**Edge cases verified:**

- Empty provider `""` is allowed on both planes (means “use default” — `multiruntime.go:36`, `server.go:77`) — tested `phantom_test.go:33,72` (`""` in allowlist).
- Case/whitespace tolerance: `" Docker "`, `"LXC"`, `" KVM "` normalized via `ToLower+TrimSpace` — `multiruntime.go:35`, `server.go:76` — tested `multiruntime/phantom_test.go:108-115` + `beacon/phantom_test.go:19-23`.
- `.env.example` absence is **intentional for safety**: default off means a fresh env without the flag cannot accidentally admit phantom runtimes. But discoverability suffers — see §2.3.
- No `BUILD_ENABLE_*` overlap: `BUILD_ENABLE_*` governs buildpacks/builder selection, not runtime provider. They are orthogonal — neither flag bleeds into the other’s check.

**Verdict: PASS.** Phantom 400 is intact on both planes, flag-gated, case-insensitive, tested, and default-off safe.

---

## 5. Half-Wiring Audit — Would a Table Exist Without Its Consumer (or Vice Versa)?

### 5.1 Gateway 5 Tables + Reconciler — Intentionally Not Half-Wired

**Claim to check:** “gateway 5 tables created but reconciler not started” (classic half-wiring dread).

| Check | Result |
|---|---|
| `gateway_*` tables exist? | **No.** Grep `gateway_`/`gwy_`/`gateway_routes` in `forge/api/internal/store/migrations/*.sql` and `forge/api/migrations/*.sql` returns **0 hits** (verified via `bash grep -rn gateway_ --include=*.sql`). `038_traffic_routing.sql` only defines `traffic_rules` + `traffic_policies`. No `gateway_routers/middlewares/services/certificates/reported_state` tables have been created. |
| `GATEWAY_SINGLE_WRITER` flag exists? | **No.** Grep `GATEWAY_SINGLE_WRITER` in `forge/` + `beacon/` returns **0 hits**. The flag is documented only in `audits/implementation-plan/subagent-08-gateway.md:128,400,1230,1278` + `audits/FORGE_IMPLEMENTATION_PLAN.md:37,114` as the **Phase C** canary flag (“when true, reconciler is the only Caddy writer; legacy writers deleted”). |
| Reconciler exists / started? | **No dedicated `gateway.Reconciler` package exists yet.** The current reconciliation stack is `forge/api/internal/services/reconciler/service.go` (general drift + health) + `crossnode.IngressSynchronizer` at `forge/api/cmd/api/main.go:1087-1088` + `trafficmanager.CaddyReverseProxy` at `main.go:1120`. The planned `gateway/{adapter,store,reconciler,render_caddy}` package at `implementation-plan/subagent-08-gateway.md:1265 B2` is still “code exists but is not wired as writer yet.” |
| Current single-writer mechanism | `crossnode.IngressSynchronizer` (`crossnode/ingress_sync.go:28`) protects `rules/policies/tracking/syncCount` with `sync.RWMutex`, serializes gateway writes via `Sync:111-215`, and has empty-sync guard at `ingress_sync.go:128-181` + integration test `forge/api/internal/http/integration_gateway_test.go:14` (9 tests PASS). This is the **interim** single-writer for the legacy whole-doc `POST /config/` path — not the planned `gateway_*` pipeline. |
| Half-wired risk | **None.** No table without reconciler, no reconciler without table. The “5-table inversion” is still in planning (`subagent-08-gateway.md:1267` “B4 dual-read helper” etc.). The existing `traffic_rules` + `traffic_policies` remain the **single source of truth** consumed by `CaddyReverseProxy.buildPolicyHandles` and `buildRoutes` (`caddy_proxy.go`). Upgrades see no data loss if the plan ships later. |

**Verdict: PASS.** Gateway single-writer remains intentional technical debt, not a half-wire. The implementation plan explicitly calls out reversibility at `subagent-08-gateway.md:1320` (`GATEWAY_SINGLE_WRITER=false` returns to legacy without data loss).

### 5.2 Other “Could Be Half-Wired” Surfaces

| Surface | Table / State | Consumer / Reaper / Handler | Half? | Notes |
|---|---|---|---|---|
| **Previewenv reaper** | `preview_deployments.expires_at` (`119_deployments_rollbacks.sql:27` + `180_preview_ttl.sql:5`) | `previewenv/reaper.go:21 StartReaper(5m)` via `phase4_registrar.go:64` **only when `cfg.BackgroundContext != nil`**. `main.go:1666` sets `BackgroundContext: appCtx`, so prod starts it. Tests/dev with `cfg==nil` or `Store==nil` skip (`handlers_preview_deployments.go:28`, `phase4_registrar.go:36-40`). | **No** | Reaper lifecycle bound to `appCtx`; `Stop()` via `done` channel at `reaper.go:88`. `server.go` lazy alias mirrors options so both `/preview` + `/admin/preview-deployments` share expiry semantics. |
| **Service discovery reaper** | `service_discovery_endpoints.last_heartbeat` | `servicediscovery/service.go:78 Start` → `discovery.Start(ctx)` → `StaleEndpointReaper` + `registry.LoadFromStore` at `81`. `main.go:1083 discoverySvc.Start(appCtx)` + `healthFilter.StartReaper(appCtx,5m)` at `1086` + `ingressSync.Start(appCtx,30s)` at `1088` | **No** | `EnsureBeaconEndpoint` at `server.go:1918-1926` fixes prior write-only gap where endpoints were never heartbeat-refreshed and 3m reaper marked everything unhealthy. |
| **Queue single-writer + leader** | `job_queue` + `operations` dual tables + `forge_leader` (`214_forge_leader.sql`) | `queue.Service:289-301` dual-write + `main.go:1259-1348` `isForgeLeader()` predicate wrapping `rec.Start`, `failSvc.Start`, `bkWorker.Start`, `cleanupSvc.Start`, `periodicScheduler.Start` + per-job predicates at `main.go:1317,1327` (`backup.retention`, `cert.renewal`) + metrics `server.go:1673-1689` | **Intentional transitional — not half-wired** | `QUEUE_SINGLE_WRITER=0` = legacy (every replica is leader, `main.go:1261 return true`) counts toward `legacy_fallback_total` but still durable. When `1`, `queueSingleWriterEnabled()==true` at `queue.go:294` stops incrementing, sidelines `operations`, and predicate gates follower handlers (idempotent via `ON CONFLICT`). One-release window documented at `queue.go:289` + `AdminOperations.tsx:91` (`dual-write for one release`). |
| **Backup encryption** | `backups` rows store `bytes` + legacy `Nonce` column (`forge/api/migrations` not re-encrypted) | `backup/encryption.go:131-165` `Encrypt` (V2 flag-branch), `Decryption` auto-detects V2 header (`Decrypt:232, DecryptWithAAD:267`) + streaming decryptor at `364-457` handling both V2 chunked + legacy chunked + legacy single-shot. Old ciphertexts remain decryptable without V2 enabled. | **No** | Header `0x02` at `encryption.go:29` disambiguates. `isV2StructuralError` at `281` prevents false fallback. `EncryptWithAAD` at `164` always uses `encryptV2` with AAD for newer uploads (outside flag). Flag only gates `Encrypt(data,key)` generic path at `140`. |
| **Caddy experimental handlers** | `traffic_policies.rate_limit / circuit_breaker` rows | `caddy_proxy.go:1129-1218` `buildPolicyHandles` — `experimentalHandlersEnabled()` gate at `1133,1198`; `else slog.Warn skip` at `1140,1213` prevents emitting fictional `rate_limit`/`circuit_breaker` handlers that brick `POST /config` updates. `traffic_policies` IP allow/deny path at `1145-1195` is always on (subroute + static_response). | **No** | Skipping is honest — the route is still emitted, just without the experimental handle. Upgrade to `CADDY_ENABLE_EXPERIMENTAL_HANDLERS=1` is additive, never destructive. |
| **Reconciler wiring** | `reconcile_plans` + `servers/nodes` | `reconciler/service.go:134 New`, `174 Start` (jitter at `189`, ticker at `195`, `RunOnce` at `202`), `SetQueueService/GitOpsService/DomainSyncer` at `158,166,170`, wired at `main.go:1230-1273` **before** `Start`, leader-gated at `1274-1278` | **No** | `main.go:1272` comment “Reconciler must be started after tmSvc so autoReconcileDrifts can observe gateway state” — ordering correct. `autoReconcile` default `true` at `147` but gated by leader check at `1274`. No zero-table start. |
| **Installer + queue interplay** | `install_workflows` (server_id FK) | `main.go:846-855` wires store+service even when `db==nil`? No — `if cfg.Store==nil return` at `handlers_installer.go:17` + `server.go:2988` `registerInstallerRoutes(protected,cfg)` requires `InstallerService != nil && Store != nil` else skip with `dev mode` comment | **No** | Dev mode (no DB) correctly omits routes rather than half-emitting 500s. |

**Verdict: PASS.** No “table without ticker” or “ticker without table” instance found. The sole transitional duality (queue) is fully documented, metric’d, predicate-guarded, and time-boxed to one release.

---

## 6. Cross-Cutting Findings

### 6.1 Defaults Are Zero-Break

Every real flag defaults to **OFF/false/empty → safe legacy behavior**:

- `ENABLE_EXPERIMENTAL_RUNTIMES="" → false` → phantom 400 (no Docker fallback)
- `BACKUP_ENCRYPTION_V2="" → false` → legacy `nonce||ct` encrypt, V2 decrypt still works via header detection
- `FORGE_ENV_FILE_STRICT="" → false` → warn + ignore env_file, not 400
- `CADDY_ENABLE_EXPERIMENTAL_HANDLERS="" → false` → skip fictional handlers, emit static routes
- `INSTALLER_WORKFLOW_ENABLED="" → false` → visibility on, execute 409 `intentionally deferred`
- `QUEUE_SINGLE_WRITER="" → false` → dual-write `job_queue`+`operations`, `isForgeLeader()==true` on every replica (legacy), metric increments
- `FORGE_LEADER != "1" → follower` → but ignored until `QUEUE_SINGLE_WRITER=1`

No flag introduces a breaking change on upgrade — matches `FORGE_IMPLEMENTATION_PLAN.md:37` principle “No flag = zero behavior change.”

### 6.2 Planned-Only Flags — Honest Gap, Not Drift

| Flag | Why It’s Still Planned | Evidence It’s Not Claimed as Done |
|---|---|---|
| `GATEWAY_SINGLE_WRITER` | Full 5-table inversion + `gateway/` package + `gateway_reported_state` + cert-delivery reconciler. Cost is a data-model cutover; interim `IngressSynchronizer` mutex already provides single-writer for `POST /config/` path | Zero code refs; `infra/compose.yml` + `Caddyfile` still on whole-doc `POST /config`; `forge/web/app/admin/gateways/page.tsx:479` calls out missing `gateway_router_middlewares` join table + `GET /policies` |
| `BUILD_ENABLE_*` | Builder matrix (`nixpacks|herokuish|paketo`) behind `source-deployments` API; requires buildpack service + cache/platform UI | Zero code refs; `forge/api/internal/services/build` etc. exist but no `BUILD_ENABLE_*` predicate; `audits/implementation-plan/subagent-04` marks `buildType` 422 honesty contract intentionally retained |

Neither is half-wired — they simply haven’t been cut.

### 6.3 Test Coverage Snapshots

| Area | Test File(s) | Outcome on HEAD (no DB) |
|---|---|---|
| Experimental runtimes (Beacon) | `beacon/internal/server/phantom_test.go` | 3 tests, phantom 400 locked |
| Experimental runtimes (Forge) | `forge/api/internal/runtime/phantom_test.go` | 3 tests, `ErrUnsupportedProvider` ↔ 400 |
| Backup WS wiring | `beacon/internal/server/backup_progress_wiring_test.go` | Strings-asserts `BackupProgressEvent`, `backupProgressWS`, route |
| Backup crypto V2 | `forge/api/internal/services/backup/encryption_test.go` | `BACKUP_ENCRYPTION_V2=true` round-trips + streaming cases |
| Compose env_file strict | `forge/api/internal/services/compose/env_file_test.go` + `handlers_apphosting_reverification_test.go:512` | Both `true` → 400, `false`/`""` → warn/pass |
| Caddy handlers | `forge/api/internal/services/trafficmanager/caddy_proxy_test.go` | `CADDY_ENABLE_EXPERIMENTAL_HANDLERS=true` exercised; default skips |
| Preview per-PR | `forge/api/migrations/216_preview_per_pr_unique.sql` + `previewenv/service.go:144` scan + `handlers_git_reverification_test.go:309` dual-path 409 | Partial unique index + in-memory scan (DB-level + service-level) |
| Gateway guard | `forge/api/internal/http/integration_gateway_test.go` | 9 tests PASS — empty-sync + no-wipe invariants |

All of the above were ran as part of `go test ./... -run Test` without `TEST_DATABASE_URL` during Phase 09 verification (DB-dependent suites SKIP cleanly).

---

## 7. Recommendations

### P1 — Document the Hidden Flags in `.env.example` (One-PR Fix)

Add a `## FEATURE FLAGS (all default OFF — zero behavior change)` section to **root `.env.example`** after the `INSTALLER_WORKFLOW_ENABLED` block and mirror a `## FORGE FEATURE FLAGS (api-layer, not compose)` note at the bottom of `infra/.env.example`:

```env
# Feature flags — all default OFF (unset or 0/false). No flag = zero behavior change.
# See audits/FORGE_IMPLEMENTATION_PLAN.md:37 and each file’s header for semantics.

# Gate LXC/KVM phantom providers. When false (default) they return HTTP 400 via
# forge/api/internal/runtime/multiruntime.go:27 + beacon/internal/server/server.go:70
# Tests: forge/api/internal/runtime/phantom_test.go / beacon/internal/server/phantom_test.go
# ENABLE_EXPERIMENTAL_RUNTIMES=0

# Gate per-backup salt + 1 MiB chunked AES-GCM-Kopia framing. Legacy nonce||ct remains
# decryptable when flag is 0. See forge/api/internal/services/backup/encryption.go:104
# BACKUP_ENCRYPTION_V2=0

# When true, compose env_file is rejected with HTTP 400. Default false = warn + ignore.
# See forge/api/internal/services/compose/service.go:21
# FORGE_ENV_FILE_STRICT=0

# Gate experimental Caddy rate_limit/circuit_breaker handlers. When false (default) those
# policies are skipped with slog.Warn to avoid bricking POST /config updates.
# See forge/api/internal/services/trafficmanager/caddy_proxy.go:1124
# CADDY_ENABLE_EXPERIMENTAL_HANDLERS=0

# Queue consolidation: 0 = dual-write job_queue+operations (every replica is leader, one release
# migration window, counts toward game_panel_api_legacy_fallback_total). 1 = single-writer
# job_queue only, predicate-gated handlers, leader-gated daemons.
# See forge/api/internal/services/queue/queue.go:100 + forge/api/cmd/api/main.go:1252
# QUEUE_SINGLE_WRITER=0

# Only meaningful when QUEUE_SINGLE_WRITER=1. Set by the elector sidecar after
# pg_try_advisory_lock(hashtext('forge_leader')) (migrations/214_forge_leader.sql).
# Controls reconciler/failover/retention/cleanup/scheduler leader gates + metrics
# game_panel_api_forge_leader_is_leader. See forge/api/cmd/api/main.go:1264
# FORGE_LEADER=0
```

Rationale: Resolves the sole FAIL in §1 without changing runtime defaults. Operators can `grep FEATURE .env.example` and immediately know the flag names, defaults, file references, and test locks.

### P2 — Add `GATEWAY_SINGLE_WRITER` to the Plan’s `.env.example` When Phase C Lands (Defer)

Do **not** add `GATEWAY_SINGLE_WRITER` to `.env.example` now — the 5-table cut has no code to read it. When `gateway/{store,reconciler}` lands (per `subagent-08-gateway.md:1278` C1), add:

```env
# GATEWAY_SINGLE_WRITER=0 — Phase C canary. When true, the gateway reconciler
# is the ONLY Caddy writer; legacy whole-doc POST /config path is removed.
# Keep false for one canary cycle; flip fleet-wide after dry-run validation.
# See forge/api/internal/services/gateway/reconciler.go
```

### P3 — Consider Exposing `BUILD_ENABLE_*` Defaults Now (Optional)

Since `BUILD_ENABLE_*` remains PLANNED, it’s reasonable to leave it out of `.env.example` until the builder matrix is wired. If early discoverability is valued, add a commented stub:

```env
# BUILD_ENABLE_NIXPACKS=0 — allow buildType=nixpacks|railpack (planned,
# handler will return 422 when off). See forge/api/internal/http/handlers_source_deployments.go
```

### P4 — Keep the One-Release QUEUE Single-Writer Window Time-Boxed

The `QUEUE_SINGLE_WRITER=0` dual-write window is healthy but must not linger. Track the fleet’s `game_panel_api_legacy_fallback_total` (via `server.go:1673` metric). Once `QUEUED` p99 converges and all followers are on the predicate-gated code, flip to `1` and file the follow-up to delete the dual-write fallback in the next release — otherwise the “one release” promise becomes permanent debt.

---

## 8. Evidence & Reproduction

```bash
# 1. Feature flag census (the 9 requested)
grep -R --include='*.go' 'GATEWAY_SINGLE_WRITER\|BACKUP_ENCRYPTION_V2\|FORGE_ENV_FILE_STRICT\|BUILD_ENABLE_\|QUEUE_SINGLE_WRITER\|CADDY_ENABLE_EXPERIMENTAL\|ENABLE_EXPERIMENTAL_RUNTIMES\|INSTALLER_WORKFLOW_ENABLED\|FORGE_LEADER' forge beacon

# 2. Full flag census (all os.Getenv)
grep -R --include='*.go' 'os\.Getenv' forge --include='*.go' | grep -i 'env\|queue\|caddy\|gateway\|backup\|forge_\|build_enable\|installer'

# 3. Gateway table census (should return 0)
grep -R --include='*.sql' 'gateway_' forge/api/internal/store/migrations forge/api/migrations

# 4. Lock the phantom 400 contract
go test ./beacon/internal/server -run Phantom -count=1 -v
go test ./forge/api/internal/runtime -run Phantom -count=1 -v

# 5. Verify previously-unwired surfaces are wired (DB→UI)
grep -R 'StartReaper\|registerPreviewEnv\|registerServiceDiscovery\|backupProgressWS\|BackupProgressEvent\|handleGetCapabilitiesDelta' forge beacon
grep -R 'install_workflows\|service_discovery_endpoints\|preview_deployments' forge/api/internal/store/migrations

# .env.example diff
cat .env.example          # only INSTALLER_WORKFLOW_ENABLED documented (90-95)
cat infra/.env.example | grep -A2 -B2 'LOAD_BALANCER\|BACKUP_ADAPTER'  # compose-doc'd flags
```

---

## 9. Verdict Matrix

| Feature / Flag | Implemented? | Defaults | Documented in .env.example? | Wired end-to-end? | Half-wired? | Overall |
|---|---|---|---|---|---|---|
| `GATEWAY_SINGLE_WRITER` | No — PLANNED | — | No | No (intended) | **No — no table, no reconciler** | **PLANNED — not half-wired** |
| `BACKUP_ENCRYPTION_V2` | Yes | `false` — `encryption.go:104` | **No** | Yes (streaming encrypt+header detect fallback) | No | **PASS (undoc)** |
| `FORGE_ENV_FILE_STRICT` | Yes | `false` — `service.go:21` | **No** | Yes (400 vs warn branches tested) | No | **PASS (undoc)** |
| `BUILD_ENABLE_*` | No — PLANNED | — | No | No (intended 422 honesty) | No | **PLANNED** |
| `QUEUE_SINGLE_WRITER` | Yes | `false` — `queue.go:100` | **No** | Yes (dual-write + predicate gates + leader-gated daemons) | **No — intentional one-release window** | **PASS (undoc)** |
| `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` | Yes | `false` — `caddy_proxy.go:1124` | **No** | Yes (skip w/ warn, never bricks) | No | **PASS (undoc)** |
| `ENABLE_EXPERIMENTAL_RUNTIMES` | Yes | `false` — both planes | **No** | Yes (phantom→400 both planes, 202 when on) | No | **PASS (undoc)** |
| `INSTALLER_WORKFLOW_ENABLED` | Yes | `0` — `service.go:71` | **Yes** — `/.env.example:93` + 5-line doc | Yes (DB→UI always; 409 deferred; beacon 6-step endpoint present) | No | **PASS** |
| `FORGE_LEADER` | Yes (conditional) | `0` — `main.go:1266` | **No** | Yes (only when `QUEUE_SINGLE_WRITER=1`, via elector sidecar) | No | **PASS (undoc)** |
| Installer workflows visible | Yes | — | — | DB+service+4 route groups+UI+beacon endpoint; execution intentionally deferred | No | **PASS** |
| Backup WS e2e | Yes | — | — | `SetProgressCallback → eventBus.Publish → WS /servers/{id}/ws/backup` | No | **PASS** |
| Previewenv canonical | Yes | — | — | `previewenv.Service` canonical + deprecated alias (one service, one reaper), TTL+reaper+commit-status+unique index | No | **PASS** |
| Discovery UI | Yes | — | — | `service_discovery_endpoints` + 16 handlers + beacon self-registration + 5-tab UI | No | **PASS** |
| Capabilities delta | Yes | — | — | `collectCapabilities` + `CapabilityDelta` diff mutex + `GET /api/capabilities/delta` + history UI | No | **PASS** |
| No gateway half-wire | N/A | — | — | No table created, no reconciler running — so cannot be half-wired | **No** | **PASS** |

*Subagent 09 — 110-09-09 of 110 — Phase 09 Agent 09/10. All file:line citations verified on HEAD 2026-08-24.*
