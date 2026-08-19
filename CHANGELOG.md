# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Canonical version source is `VERSION` file + git tag `vX.Y.Z` (see `docs/releasing.md`).
Release comparison links use the `v`-prefixed tag.

## [Unreleased] — 110-agent run (Phase 09 Optimize) — 2026-08-24

> **Scope:** 110 parallel agents across `audits/110-phase-01-audit` (10), `110-phase-02-context` (10), `110-phase-03-impl` (20), `110-phase-04-verify` (10), `110-phase-05-wiring` (10), `110-phase-06-beautify` (10), `110-phase-07-smoke` (10), `110-phase-08-tests` (20), `110-phase-09-optimize` (10). Full evidence in `audits/110-phase-*/subagent-*.md` (see `audits/FORGE_IMPLEMENTATION_PLAN.md` DONE vs PLANNED table) and `audits/MASTER_FINDING_INDEX.md` remediation column.

### Added — Migrations (all additive, `IF NOT EXISTS` / dual-read)

| Migration | File | Purpose |
|---|---|---|
| `202_demo_seed_marker` | `forge/api/migrations/202_demo_seed_marker.sql:1` | `panel_settings.demo_seeded BOOLEAN` — restart never resurrects deleted demo nodes |
| `203_git_provider_generic` | `forge/api/migrations/203_git_provider_generic.sql:1` | `git_provider_tokens.provider` allow `generic` for self-hosted git (no provider API) |
| `204_git_sources_generic` | `forge/api/migrations/204_git_sources_generic.sql:1` | `git_sources.provider` allow `generic` |
| `205_app_store_install_compose_project_text` | `forge/api/migrations/205_app_store_install_compose_project_text.sql:1` | `app_store_installs.compose_project_id UUID→TEXT` — fixes `"invalid input syntax for type uuid"` on every `cps-<id>` update |
| `206_app_store_installs_ownership` | `forge/api/migrations/206_app_store_installs_ownership.sql:1` | `app_store_installs.user_id/org_id UUID` + indexes — tenancy scoping for non-admin installs |
| `207_backup_restores_data` | `forge/api/migrations/207_backup_restores_data.sql:1` | `backup_restores.data BYTEA` — restore store/service serialized request |
| `208_managed_database_deletion_protection` | `forge/api/migrations/208_managed_database_deletion_protection.sql:1` | `managed_databases.deleted_at TIMESTAMPTZ` soft-delete + `idx_managed_databases_*` |
| `209_operation_stale_reaper` | `forge/api/migrations/209_operation_stale_reaper.sql:1` | `idx_operations_running_updated_at (status, updated_at) WHERE status='running'` — 5m stale reaper |
| `210_servers_created_at_index` | `forge/api/migrations/210_servers_created_at_index.sql:1` | `idx_servers_created_at` — cursor pagination `created_at,id` |
| `211_a_add_restoring_backup_actual_state` | `forge/api/migrations/211_a_add_restoring_backup_actual_state.sql:1` | `server_actual_state` enum `restoring_backup`/`offline`/`terminating`/`terminated` — unconditional restoring lock (GH-09 P1) |
| `211_b_backup_encryption_v2` | `forge/api/migrations/211_b_backup_encryption_v2.sql:1` | `backups.encryption_salt/version/aad` + `backup_artifacts` + `idx_backups_partial_gc` — AEAD V2 per-backup HKDF |
| `212_appstore_upgrade_guards` | `forge/api/migrations/212_appstore_upgrade_guards.sql:1` | `app_store_apps.cross_version`, `app_store_installs.ignore_upgrade/app_ignore_upgrade` + `trg_sync_app_ignore_upgrade` |
| `213_encrypt_dns_credentials` | `forge/api/migrations/213_encrypt_dns_credentials.sql:1` | `certificates.dns_credentials_encrypted TEXT` + `dns_provider_accounts.credentials_encrypted` — keyring envelope |
| `214_forge_leader` | `forge/api/migrations/214_forge_leader.sql:1` | `forge_leader(id, leader_id, elected_at, expires_at)` singleton + `idx_forge_leader_expires` — advisory-lock/TTL elector AF-2 (gates reconciler/periodic/retention/failover) |
| `215_tenant_scoping_additive` | `forge/api/migrations/215_tenant_scoping_additive.sql:1` | `events/placement_decisions/reconcile_plans/reservations/intents .tenant_id UUID` + 8 partial `WHERE tenant_id IS NOT NULL` indexes |
| `216_preview_per_pr_unique` | `forge/api/migrations/216_preview_per_pr_unique.sql:1` | `idx_preview_deployments_pr_unique (pr_number, lower(repo_owner), lower(repo_name)) WHERE status IN ('deploying','running','stopped')` + `expires_at` backfill 24h |
| `217_api_perf_indexes` | `forge/api/migrations/217_api_perf_indexes.sql:1` | `idx_*` API hot-path indexes (see `audits/110-phase-09-optimize/subagent-04-db-perf.md`) |
| `218_db_perf_fk_indexes` | `forge/api/migrations/218_db_perf_fk_indexes.sql:1` | 98 FK + composite hot-path indexes for capacity/expiry scans |
| `219_add_node_tunnel` | `forge/api/migrations/219_add_node_tunnel.sql:1` | **`nodes.tunnel_ip INET` + `mesh_pubkey TEXT` — overlay mesh** `Node.TunnelIP`/`MeshPubKey` `store.go:142`; nullable, `IF NOT EXISTS`, sqlite dialect `219_add_node_tunnel.sql` TEXT |
| `220_add_egg_install_steps` | `forge/api/migrations/220_add_egg_install_steps.sql:1` | **`eggs.install_steps JSONB DEFAULT '[]'` — typed install pipeline** `store_nests.go:198` `COALESCE(install_steps,'[]')`; additive fallback to `install_script` shell blob; sqlite `TEXT DEFAULT '[]'` |

All migrations have matching `migrations/rollbacks/*.down.sql` forward-only rollback policy (`docs/migration-rollback-policy.md`).

### Added — Flags / Env (default OFF unless noted)

| Flag | Default | Location | Purpose |
|---|---|---|---|
| `BACKUP_ENCRYPTION_V2` | OFF | `forge/api/internal/services/backup/encryption.go:104` | Per-backup salt + AAD `server_id:backup_name` + 1MiB chunked AEAD seals; legacy `nonce||ct` fallback when OFF |
| `FORGE_ENV_FILE_STRICT` | OFF | `forge/api/internal/services/compose/service.go:22`, `handlers_compose.go:115` | `env_file` either resolve or 422; OFF warns + ignores `env_file`, ON rejects with clear message |
| `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` | OFF | `forge/api/internal/services/trafficmanager/caddy_proxy.go:1125` | Gates fictional `rate_limit`/`circuit_breaker` handlers (never brick route updates unless real Caddy module) |
| `QUEUE_SINGLE_WRITER` | OFF | `forge/api/cmd/api/main.go:932`, `internal/http/server.go:1129` | Dual-write `job_queue`+`operations` when OFF; single-writer `queue.Service` when ON; `legacy_fallback_total` metric |
| `FORGE_LEADER` | OFF (requires `QUEUE_SINGLE_WRITER=1`) | `forge/api/cmd/api/main.go:112` + `214_forge_leader.sql` | Advisory-lock/TTL leader election for maintenance daemons |
| `APP_ENV` | `development` | `forge/api/config/mtls.go:37`, `internal/http/server.go:443`, `cmd/api/main.go:137` | `production` panics if `MTLS_DEV_BYPASS`/`API_MTLS_DEV_BYPASS` true; guards `API_SEED_DEMO` |
| `GATEWAY_SINGLE_WRITER` | OFF | `audits/FORGE_IMPLEMENTATION_PLAN.md:13` spec | Future: sole `GatewayAdapter` vs legacy `ReverseProxy`+`caddyUpdater` (PLANNED) |

### Fixed — by file:line (110-run, grouped by audit cluster)

**Game lifecycle / hosting (GH-*)**
- `forge/api/internal/store/store_egg_variables.go:142-326` — `validateVariableValue` slash fix: `findRegexTokenEnd` respects `\/`, `[...]`, flags; `splitValidationRules` placeholder `\x1f`; `regex:/^...$/` flags `i/m/s` → `(?im)`; `nullable` empty check; `integer` case — fixes `minecraft-paper.json:58` `SERVER_JARFILE` import (REF-GAME-F-G-08) — `subagent-01-slash-seed.md`
- `forge/api/internal/store/store_servers.go:209,399` + `forge/api/internal/store/store_servers_control.go:154` + `beacon/internal/server/manager.go:693` + `forge/api/migrations/211_a_*.sql` — `restoring_backup` enum + `IsServerRestoreBlocking` + DTO scoping — `subagent-02-restoring-lock.md`
- `forge/api/internal/store/store_db_containers.go:174,200,295` — `ListDBContainers`/`ListAllDBContainers` now `COALESCE(connection_string_encrypted, credentials_encrypted)` + `decryptDBContainerSecrets` — fleet view no longer blank after `155_encrypt_db_container_credentials` — `subagent-13-envfile-seeding.md`
- `forge/api/internal/store/store_users.go:297` + `handlers_servers.go:541` — subuser `*` escalation closed: subset check + owner/admin gate `*` — `subagent-16-security-trust.md`
- `forge/api/internal/store/store_mounts_ext.go:323` + `beacon/internal/server/mounts.go:64` — mount allowlist additive prefix `/srv/forge-mounts` deny `/etc/.../var/run/docker.sock` — `subagent-04-mount-allowlist.md`

**Gateway / networking (F-NET-01..10)**
- `forge/api/internal/services/crossnode/ingress_sync.go:128-135,175-182` — empty-rule `len(mergedRules)==0` no-op (stops 30s whole-gateway wipe) — `subagent-07-gateway-hotfix.md`
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:704-722,946-982,1211-1250` — fetch-merge-preserve + sub-resource `POST /config/apps/http/servers/gamepanel` (no whole-doc wipe) — `subagent-07`
- `caddy_proxy.go:704-713,1086-1155,514:580-592` — snapshot BEFORE validate; dry-run `POST /adapt` fallback + `must-revalidate` — `subagent-07`
- `caddy_proxy.go:1058-1082,1084-1155` + `traefik_proxy.go:1122-1129` + `caddy_proxy_test.go:280-415` — fictional handlers gated `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` OFF — `subagent-07/08`
- `caddy_proxy.go:690-702,1030-1045` + `trafficmanager/service.go:383-393` + `handlers_trafficmanager.go:23-32` — TCP/UDP branch reject/skip (no HTTP host+path for tcp) — `subagent-07`
- `caddy_proxy.go:26-27,904-919` + `traefik_proxy.go:22-30,246-265` — grouped-route withdrawal group-aware via cached groups — `subagent-07`
- `caddy_proxy.go:999-1012` + `traefik_proxy.go:1122-1129` — all-policies-all-routes leak hotfix: empty set safer than cross-tenant leakage (join table PLANNED) — `subagent-08`
- `forge/api/internal/services/acme/service.go:121-180,340-358,421-432,605-610,157` + `forge/api/internal/http/server.go:1118-1145` + `cmd/api/main.go:1020-1050` — `SetCertificate` wired after create/update/renew; fullchain PEM not leaf DER; `HTTPSolver` mounted at `/.well-known/acme-challenge/*` + standalone `:80` solver; `deliverCertificateToGateway` + `acmeGatewayAdapter` — `subagent-08`
- `acme/service.go:242-255,570-596` + `store/store_certificates.go:80-150` + `migrations/213_encrypt_dns_credentials.sql:1` + `store/store_secrets.go:746-810` — `dns_credentials_encrypted` envelope + `isValidEmail` reject `admin@localhost` — `subagent-08`

**App platform / compose (REF-P6-COMP-*)**
- `beacon/internal/server/compose.go:213-224` — `shortFormHostPort ["80"]` → `parts[0]` privileged-port check — `subagent-06-compose-fixes.md`
- `forge/api/internal/services/compose/service.go:22,207,314` + `handlers_compose.go:115` — `env_file` strict `FORGE_ENV_FILE_STRICT` 400 vs warn — `subagent-06/13`
- `forge/api/internal/compose/gitops.go:381-387` — `DeployFromGit` Update-on-fresh-ID → `CreateComposeStack:390` — `subagent-06/15`
- `forge/api/internal/services/appstore/service.go:139,158,239` + `migrations/212_appstore_upgrade_guards.sql` — `uninstall` orphan guard, `UpgradeApp` `crossVersion`/`ignore_upgrade`, `resolveTemplate` interpolation failure no longer swallowed — `subagent-05-appstore-fixes.md`
- `forge/api/internal/services/compose/service.go:594-677` vs `beacon/compose.go:226-248` — volume policy bifurcation documented; compose-go includes/profiles (`G1`) audit — `subagent-06`

**Backup / crypto (REF-BKP-*)**
- `forge/api/internal/services/backup/encryption.go:104` + `migrations/211_b_backup_encryption_v2.sql:1` — `BACKUP_ENCRYPTION_V2` per-backup 16B salt + AAD `server_id:backup_name` + 1MiB chunked seals — `subagent-09-backup-crypto.md`
- `forge/api/internal/services/backup/retention.go:61` + `store/store_backup_policies.go:301` — retention OR not AND (union) + `pg_advisory_xact_lock(hashtextextended("backup_prune:<id>"))` + `FOR UPDATE SKIP LOCKED` + `flock` `backupRoot/<ns>/.backup.lock` — `subagent-09`
- `beacon/internal/backup/local.go:787` + `forge/api/internal/services/backup/adapters/s3.go` — S3 multipart/presigned `FAILED` not lost; sidecar verified vs transplant forgery guard — `subagent-09/10`
- `beacon/server.go:1542` vs `http/server.go:1975` — `backupProgressWS` dead end wired via `backup` union + `lib/api.ts:933` — `subagent-10-backup-progress.md`

**Deployment / execution**
- `forge/api/internal/deployment/execution.go:249-311` + `deployment/healthgate.go:11` — `ExecuteDeployment` honest `completed` only via `BeaconRuntimeExecutor`; health gate node-derived `resolveTargetHost` not localhost — `subagent-11-deployment-exec.md`

**Queue / events / leader (AF-2, Q-01..06)**
- `forge/api/internal/queue/leader.go:23` + `migrations/214_forge_leader.sql:1` + `cmd/api/main.go:112,932` — `forge_leader` TTL elector gates ~8 daemons (`outbox.go:74`, `queue/periodic.go`, `cronjob:72`) — `subagent-12-queue-events.md`
- `forge/api/internal/eventstore/outbox.go:39,74` + `registry.go:73` + `store.go:35` — `PublishTx(ctx, tx, envelope)` bound to business tx; Relay fan-out bridged — `subagent-12`
- `forge/api/internal/queue/store.go:113-130` — queue consolidation: CAS cancel `WHERE status='running'`, `delay_until` column, tx-wrapped transitions, unified idempotency, `context.WithoutCancel` heartbeat — `subagent-12`

**Seeding / templates**
- `forge/api/internal/store/seed_game_templates.go:323` + `migrations/091_seed_minecraft_java.sql` — 44→14 egg seeding idempotent `(nest_id,name)` upsert at `cmd/api/main.go:529` — `subagent-01/13`

**Cron / schedules**
- `forge/api/internal/http/handlers_cronjob.go:validateCronSchedule` + `handlers_servers.go:validateServerScheduleCron` + `handlers_procedures.go:validateProcedureCron` — `robfig/cron` parser 422 at admission (5-field + interval <1m) not at runner — `subagent-14-cron-placement.md:Phase9_API_SEMANTICS_AUDIT.md`
- `forge/api/internal/http/errors.go:domainErrorStatus` + `envelope.go:109` — status normalization: conflict→409, validation→422, `respondStoreError` bulk ~90 sites — `subagent-14`

**Tenancy / scoping**
- `migrations/215_tenant_scoping_additive.sql:1` — additive `tenant_id` on `events/placement_decisions/reconcile_plans/reservations/intents` + partial indexes — `subagent-19-tenancy.md`
- `forge/api/internal/http/handlers_apphosting.go:416` — scale no-op when `ReplicaAppID nil` now 400 — `subagent-19`

**Security / trust**
- `forge/api/config/mtls.go:37` + `internal/http/server.go:443` + `cmd/api/main.go:137` — `MTLS_DEV_BYPASS`/`API_MTLS_DEV_BYPASS` panic in `APP_ENV=production` — `subagent-16-security-trust.md`
- `forge/api/internal/auth/remote_hmac.go:38-124` — `NonceStore` Redis `SETNX` + 5-min skew enforced on all `/api/remote/*` — `subagent-16`

### Changed
- `forge/api` config (`APP_VERSION`) and `infra/compose.yml` now default to `TAG` (single source; `0.1.0` fallback removed).
- `beacon/Dockerfile`, `forge/api/Dockerfile`, `forge/web/Dockerfile` carry OCI `org.opencontainers.image.version` labels from `VERSION`.
- Beacon `update` dev detection now handles both `dev` and `beacon-dev`.
- `forge/api/internal/http/envelope.go:109` pagination now canonical `PaginationMeta{total, page, per_page, total_pages, has_next}` cursor+offset compat.
- `forge/api/migrations/113_app_store.sql:31` `compose_project_id UUID→TEXT`, `137_river_queue_features.sql` deprecated header, `172_phase2_envdomains.sql:16` `cert_id VARCHAR(36)`.

### Added — Missing fixes (fix-missing 6-agent pass — 2026-08-24)

> **Scope:** 6 parallel fix tracks closing the last genuinely MISSING 8–10% (`FINAL_PARITY_AUDIT.md:469`): overlay mesh `TunnelIP`/`MeshPubKey`, egg `config.files` patcher, per-blob HKDF+GCM, typed install pipeline, attribute-spread/reschedule, wiring. Verified via `go vet` 0 + `TestValidateVariableValue` PASS + `TestApplyConfig` PASS (no regression) + `placement` 26 tests PASS + `tsc --noEmit` 0 + `TestComprehensiveMigrationValidation` PASS after duplicate-prefix rename. Report: `audits/fix-missing/subagent-verify-docs.md`.

| Track | Migration / file:line | What was MISSING → now CURRENT | Wiring |
|---|---|---|---|
| **Overlay mesh** | `219_add_node_tunnel.sql:1` `tunnel_ip INET` + `mesh_pubkey TEXT` → `store/store.go:142` `TunnelIP`/`MeshPubKey` + `store_nodes.go:94,202` SELECT `tunnel_ip::text`, `mesh_pubkey` + `UpdateNodeHeartbeat:770` `mesh_pubkey = CASE WHEN $12<>''` + `NodeHeartbeatRequest:396` `TunnelIP/MeshPubKey` | `crossnode/resolver.go:103` `if node.TunnelIP != nil && *node.TunnelIP != "" { return *node.TunnelIP }` + `103,121,135` canonical check + `trafficmanager/service.go:653` `resolveTargetHost` + `cmd/api/main.go:2228` overlay precedence (`TunnelIP` over `publicHostname`/`FQDN`) + `store/store.go:142` JSON `tunnelIp`/`meshPubKey` + `http/server.go:1867` patch input | Private WireGuard east-west now available; nullable → fallback to `publicHostname/FQDN` when not set; heartbeat-distributed via `UpdateNodeHeartbeat`; `go vet` 0 after fixing duplicate `219` → `220` prefix (renamed `219_add_egg_install_steps.sql` → `220`) |
| **Config patcher** | `beacon/internal/server/config_patcher.go:233` `patchConfigurationFiles` (new 548-line file; fixed `bufio` unused import) | `manager.go:754` `applyPreStartConfigPatches` pre-start hook reads `.config/server.json` + `server.go:1013` `applyConfigurationFiles` sync path; supports Forge map `config.files["server.properties"].find.server-port={{server.build.default.port}}` + Wings array `replace` shape + `properties`/`yaml`/`json` parsers + `resolveValue:23` handles `{{VAR}}`/`{{env.VAR}}`/`{{server.build.default.port}}`/`\|default:''` + `..` escape guard + 64 MiB limit + 0640 writes | Fixes `FINAL_PARITY` GH-15 P1 Minecraft wrong port (previously `eggs.config` stored-not-applied `store_servers_control.go:95`); unsupported parsers `file`/`ini`/`xml`/`toml` log warn not fail; `go vet beacon` now 0 |
| **Backup per-blob HKDF+GCM** | `211_b_backup_encryption_v2.sql:1` already `encryption_salt/version/aad` + `backup_artifacts` (reused) | `backup/encryption.go:121` per-blob HKDF hierarchy: `master --HKDF(salt, purposeEncryptionKey)--> per-backup key --HKDF(nil, backup-chunk:<n>)--> per-chunk subkey(32B)` `deriveChunkKey:143` + `chunkSize=1<<20` 1 MiB chunked `GCM.Seal` per chunk `encryption.go:231` `gcm.Seal(nil,nonce,chunk,aad)` + per-chunk 12B random `nonce` + `AAD=server_id:backup_name` binding + `Decrypt:343` seen-nonce map + empty-plaintext single-chunk framing; fallback `encryption.go:372` single-key decrypt for legacy backups | Kopia `content-manager` pattern; transplant forgery sealed via salt+AAD+per-chunk domain separation; `BACKUP_ENCRYPTION_V2` flag gates V2 vs legacy `nonce||ct` |
| **Typed install pipeline** | `220_add_egg_install_steps.sql:1` `eggs.install_steps JSONB DEFAULT '[]'` (`sqlite:220` `TEXT`) | `store_nests.go:198,234` `COALESCE(install_steps,'[]')` + `store/store.go:727` `Template.InstallSteps json.RawMessage` + `store/store.go:764` + `daemon/client.go:482` `InstallSteps` + `runtime/runtime.go:83` `InstallSteps` + `installer/service.go:199` `defaultInstallSteps()` + `server.go:1305` interpreter dispatch comment; additive — `[]` or missing → fallback to `install_script` shell blob (PufferPanel 24 ops `mojangdl|paperdl|fabricdl` typed vs Forge single script) | `220` idempotent `IF NOT EXISTS`/`ADD COLUMN IF NOT EXISTS`; existing shell installs unaffected; `go vet` + migration `TestComprehensiveMigrationValidation` PASS |
| **Spread / reschedule** | `placement/strategy.go:30` `StrategySpread` + `placement/replica.go:172` `spreadPenalty 0.1*count` + `placement/constraints.go:50` `kSoftWeight=0.30` | `placement/strategy.go:42-62` `Spread *SpreadConfig` (`Attribute`, `Weight`, `Targets`, `SpreadCounts`, `SpreadTotal`) + `replica.go:180+` `evenSpreadScoreBoost`/`targetSpreadScore`/`desiredCountsForSpread` bounded `±0.30` via `FORGE_PLACEMENT_V2` (legacy `FORGE_PLACEMENT_V2=false` restores `+1e12/-1e10` for rollback) + `placement/constraints.go:59` normalized `CheckSoft` + `scheduler/replica_test.go:235` spread prefers emptier node + `services/cronjob/service.go:121` `RescheduleJob` (if `!Enabled` `removeJob` else `scheduleJob`, `service_test.go:77` 3 tests: Disables/Enabled/Reschedules) + `queue/queue_test.go:92` rescheduled in place | 26 placement tests PASS (`TestCheckSoftNormalizedBounds`, `TestLeastLoadedScorerNormalized`, `TestEnginePlaceSoftBonusDoesNotDwarfBase`, `TestSpreadPenalty`, etc.); no reschedule policy forever-loop (`replicamanager:883` 60s) replaced by `RescheduleJob` idempotent |
| **Wiring** | All above `go vet ./forge/api/... ./beacon/...` 0 (fixed `beacon/config_patcher.go:4` `"bufio"` unused + `manager.go:727` `applyPreStartConfigPatches` wiring) + `go test placement` 0 + `tsc --noEmit` 0 | `crossnode/resolver.go:104,121,135` canonical `TunnelIP` fallback + `store_nodes.go:147` `Valid && String != ""` → `Node.TunnelIP = &v` + `main.go:2242` `TunnelIP: node.TunnelIP` DTO + `http/server.go:941` `tunnelIp` JSON + `domain/domain.go:308` `ConnectionModeTunnel`; migration duplicate-prefix fix: `219_add_egg_install_steps.sql` → `220` (sqlite mirrored) so `validateNoDuplicatePrefixes` no longer `FAIL` `TestComprehensiveMigrationValidation` | Full wiring chain `Node` schema → `SELECT` → `Heartbeat` → `Resolver` → `TrafficManager` → `API` verified; `go vet` clean, `tsc` clean |

### Fixed — prior baseline (pre-110, retained)
- Removed fake `latest` claims in K8s deployments (now `IfNotPresent` + explicit `${TAG}`).
- `store_egg_variables.go:176` slash bug, `store_users.go:297` subuser `*`, `store_mounts_ext.go:323` mount allowlist — now tracked above as 110-run FIXED.

---

## [0.1.0] - 2026-08-23

### Added
- Initial Forge Control Plane MVP (API, Web, Beacon, compose, K8s ship).

[Unreleased]: https://github.com/ryzenate1/forge-control-plane/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ryzenate1/forge-control-plane/releases/tag/v0.1.0
