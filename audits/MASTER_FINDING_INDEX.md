# Master Finding Index

ID format: `PREFIX-NNN` where prefix = `REF-APP`, `REF-GAME`, `REF-BKP`, `REF-NET`, `REF-ORCH`, `FORGE-LOGIC`
Severity: P0 security/data-loss | P1 core broken | P2 unwired/integration | P3 discoverability/UX | P4 optimization
Evidence: SOURCE_VERIFIED > RUNTIME_VERIFIED > TEST_VERIFIED > DOC_DERIVED > INFERRED
Remediation: Phase 1 pass 2026-08-23 — see phase-01-remediation/finding-status.md for root causes, files, tests.
**110-run addendum 2026-08-24:** 20 impl subagents (phase-03) + 10 wiring + 10 beautify + 20 tests — see `audits/110-phase-03-impl/subagent-*.md` (20), `audits/110-phase-09-optimize/subagent-10-docs.md` (docs), `CHANGELOG.md` Unreleased (migrations 202–216 + flags + file:line), `audits/FORGE_IMPLEMENTATION_PLAN.md` DONE vs PLANNED table. Rows below with **110-FIXED** were closed in this run; others remain OPEN/DEFERRED/BLOCKED per Phase 01 ledger.

| ID | Phase | Severity | Kind | Title | First source | REMEDIATION_STATUS |
| | | | | | | |

| REF-APP-C01 | Phase1 | P2 | PARTIAL | CreateApp defers destination validation | store_apphosting.go:92, handlers_apphosting.go:173 || OPEN (P2) |
| REF-APP-C02-FLF01 | Phase1 | P0 | FALSE_COMPLETION | ExecuteDeployment stubs report completed with no runtime | deployment/execution.go:249-311 | FL-12 |
| REF-APP-C03-FLF02 | Phase1 | P1 | BROKEN | App start/stop desired_state lie, restart is TriggerDeploy, admin compose restart missing | handlers_apphosting.go:450/500, handlers_compose.go:442 | — |
| REF-APP-C04-FLF03 | Phase1 | P1 | BROKEN | Compose redeploy creates new row leak | handlers_compose.go:413 | — |
| REF-APP-C05-FLF04 | Phase1 | P2 | BROKEN | Rollback version race (non-version-checked UpdateDeployment) | deployment/revisions.go:147 | — |
| REF-APP-C06 | Phase1 | P2 | PARTIAL | Delete unconstrained (orphan containers/reservations) | apphosting/service.go:174, compose/lifecycle.go:570 || DEFERRED_WITH_REASON |
| REF-APP-C07 | Phase1 | P1 | PARTIAL | Scale silent no-op when ReplicaAppID nil | handlers_apphosting.go:416, replicamanager || VERIFIED_FIXED (pre-mutation validation) |
| REF-APP-C08-FLF05 | Phase1 | P0 | BROKEN | Health gate localhost probe | deployment/healthgate.go:11, rollout.go:71 || VERIFIED_FIXED (node-derived health target) |
| REF-APP-GIT01 | Phase1 | MED | PARTIAL | Generic OAuth dance missing, UI hides generic | handlers_git.go:190, git-providers/page.tsx:37 || DEFERRED_WITH_REASON |
| REF-APP-GIT02-LF01 | Phase1 | MED | BROKEN | Cred script embeds token vs Beacon env-var | git/deploy_service.go:287 vs beacon/git.go:387 || DEFERRED_WITH_REASON |
| REF-APP-GIT03-LF02 | Phase1 | MED | BROKEN | Shallow SHA fetch fragility | git/deploy_service.go:207 || DEFERRED_WITH_REASON |
| REF-APP-GIT04-LF03 | Phase1 | HIGH | FALSE_COMPLETION | 5 buildTypes accepted, 1 executed | git/source_deploy.go:81, build/service.go:28 || VERIFIED_FIXED (honest admission) |
| REF-APP-GIT05-LF04 | Phase1 | MED | UNWIRED | Beacon drops CacheFrom/CacheTo | beacon/build.go:59 vs build/service.go:95 || VERIFIED_FIXED (+platform, injection-safe) |
| REF-APP-GIT07-LF05 | Phase1 | MED | PARTIAL | Remote build logs post-hoc not live SSE | build/service.go:476 vs handlers_builds.go:91 || DEFERRED_WITH_REASON |
| REF-APP-GIT08-LF06 | Phase1 | MED | PARTIAL | Revisions vs git deployments disjoint tables | store_deployment_revisions.go vs git/deploy.go || DEFERRED_WITH_REASON |
| REF-APP-GIT09-LF07 | Phase1 | HIGH | DUPLICATE | preview vs previewenv (legacy wired, TTL hidden) | preview/service.go:42 vs previewenv/service.go:121 || DEFERRED_WITH_REASON (decision: promote previewenv) |
| REF-APP-GIT10-LF08 | Phase1 | LOW | BROKEN | Webhook HMAC 200 mask | handlers_git.go:599 || VERIFIED_FIXED (401 on bad signature) |
| REF-APP-RT-FL01 | Phase1 | P1 | BROKEN | Admin container/network/volume create 404 (no Beacon handler) | daemon/client.go:1892/2047/2073 vs beacon/server.go:440 || VERIFIED_FIXED (beacon handlers + routes) |
| REF-APP-RT-FL02 | Phase1 | P2 | BROKEN | Prune crossed wires (volume→missing, image→unwired) | daemon/client.go:1854/2099, beacon/container_admin.go:531 || VERIFIED_FIXED |
| REF-APP-RT-FL03 | Phase1 | P2 | UNWIRED | Exec allowlisted in Beacon but unwired in API/UI | beacon/container_admin.go:792 vs handlers_docker.go || INTENTIONALLY_NOT_EXPOSED |
| REF-APP-UX01 | Phase1 | P3 | DUPLICATE | Recovery nav duplicates /admin/migrations | admin-registry.ts:32 || ALREADY_FIXED |
| REF-APP-UX02 | Phase1 | P2 | FALSE_COMPLETION | App create form validates then discards (hardcodes image) | app/app-create-form.tsx:22 || VERIFIED_FIXED |
| REF-APP-UX03 | Phase1 | P1 | DUPLICATE | Three EnvVar editors incoherent | env-var-editor.tsx:8 vs AdminAppsShared:120 vs EnvironmentEditor.tsx:37 || DEFERRED_WITH_REASON |
| REF-APP-UX04 | Phase1 | P2 | BROKEN | Back href hardcodes /servers, tab state lost | app-detail.tsx:55, apps/[id]/page.tsx:28 || VERIFIED_FIXED |
| REF-APP-UX05 | Phase1 | P2 | BROKEN | EmptyState CTA dead, metrics queryKey missing node | app-list.tsx:44, metrics-chart.tsx:73 || CTA VERIFIED_FIXED; queryKey INVALIDATED |
| REF-APP-UX06 | Phase1 | P2 | DUPLICATE | Deployment polling duality 2s vs 5s | deployment-progress vs DeploymentTimeline || DEFERRED_WITH_REASON |
| REF-APP-ARCH01 | Phase1 | P1 | BROKEN | Operation reaper 5m duplicate execution | operation/service.go:164 || VERIFIED_FIXED (worker heartbeat Touch) |
| REF-APP-ARCH02 | Phase1 | P2 | BROKEN | Session pointer race + raw token storage | auth/session.go:36 || BLOCKED (concurrent rewrite) |
| REF-APP-ARCH03 | Phase1 | P2 | BROKEN | Beacon reconnect blind StateConnected defeats heartbeatmonitor | beacon/remote/reconnect.go:98 vs heartbeatmonitor || BLOCKED (concurrent edits) |
| REF-APP-ARCH04 | Phase1 | P3 | ARCHITECTURE | placement.Engine global mutex serializes scheduling | placement/engine.go:14 || DEFERRED_WITH_REASON |
| REF-APP-ARCH05 | Phase1 | P3 | BROKEN | Advisory lock blocking + seeder bypass | store/store.go:39 || DEFERRED_WITH_REASON |
| 
| REF-GAME-C01 | Phase2 | P1 | PARTIAL | Install/kill/suspend fencing races | beacon/manager.go:693, store_state.go:40 | — |
| REF-GAME-F-G-06 | Phase2 | HIGH | MISSING | restoring_backup not set as server lock (concurrent start allowed) | handlers_servers.go:2104 | **110-FIXED** `211_a_add_restoring_backup_actual_state.sql:1` enum + `store_servers.go:399` `IsServerRestoreBlocking` — `subagent-02-restoring-lock.md` |
| REF-GAME-F-G-07 | Phase2 | HIGH | BROKEN | Reinstall fails on Docker runtime gate | clustermanager/service.go:242 | — |
| REF-GAME-F-G-08 | Phase2 | HIGH | BROKEN | Regex delimiter in validateVariableValue blocks egg imports | store_egg_variables.go:177 | **110-FIXED** `store_egg_variables.go:200-326` slash + `splitValidationRules` + flags — `subagent-01-slash-seed.md` (CONFIRMED_BY: subagent02 LF-01) |
| REF-GAME-F-G-09 | Phase2 | HIGH | BROKEN | CPU cpu_shares vs CPUPercent conflation | store_servers.go:209, beacon/docker.go:924 | — |
| REF-GAME-F-G-10 | Phase2 | MED | BROKEN | user_viewable gate leak on CreateServer | store_servers_control.go:154 vs store_startup.go:58 | — |
| REF-GAME-F-G-13 | Phase2 | MED | BROKEN | Overcommit scoring healthy | noderegistry/service.go:302 | — |
| REF-GAME-F-G-15 | Phase2 | MED | BROKEN | Heartbeat history 5 truncation misclassifies recovery | heartbeatmonitor/service.go:205 | — |
| REF-GAME-F-G-17 | Phase2 | P1 | BROKEN | WS double JSON parse drops plain output | ws/websocket-manager.ts:109 | — |
| REF-GAME-F-G-18 | Phase2 | P1 | BROKEN | Network graph cumulative not delta | console-view.tsx:203 | — |
| REF-GAME-F-G-19 | Phase2 | P1 | BROKEN | Transfer dual source race flicker | transfer-view.tsx:77 | — |
| REF-GAME-F-G-22 | Phase2 | HIGH | BROKEN | Subuser * escalation (any user.create → *) | store_users.go:297 | **110-FIXED** `store_users.go:297` subset + owner/admin `*` gate — `subagent-16-security-trust.md` (CONFIRMED_BY: subagent05 F-01) |
| REF-GAME-F-G-23 | Phase2 | MED | BROKEN | CreateScheduleTask missing isValidScheduleTaskAction | store_schedules.go:252 | — |
| REF-GAME-F-G-24 | Phase2 | MED | BROKEN | Mount allowlist too narrow (host breakout) | store_mounts_ext.go:323 | — |

| REF-BKP-F-B-01 | Phase3 | P0 | BROKEN | Unauthenticated sidecar + null AAD transplant forgery | beacon/local.go:787, encryption.go:26 | **110-FIXED** `211_b_backup_encryption_v2.sql:1` + `encryption.go:104` `BACKUP_ENCRYPTION_V2` AAD `server_id:backup_name` — `subagent-09-backup-crypto.md` |
| REF-BKP-F-B-02 | Phase3 | P0 | BROKEN | EncryptReader buffers entire backup OOM | encryption.go:150 | **110-FIXED** `211_b` V2 + 1MiB chunked streaming seals (legacy `nonce||ct` fallback) — `subagent-09` |
| REF-BKP-F-B-05 | Phase3 | P1 | BROKEN | Retention AND vs OR inversion | retention.go:61 vs store_backup_policies.go:301 | **110-FIXED** union-OR + advisory-lock `FOR UPDATE SKIP LOCKED` + `flock` — `subagent-09` |
| REF-BKP-F-B-06 | Phase3 | P1 | BROKEN | Prune SQL-only orphans S3 + racy enforceRetentionBeforeBackup | store_backups.go:242 | **110-FIXED** dual-target retention `s.defaultAdapter` + per-backup routing — `subagent-09` (partial; storage routing G6 PLANNED) |
| REF-BKP-F-B-15 | Phase3 | P2 | UNWIRED | backupProgressWS dead end (beacon publishes, panel not proxies) | beacon/server.go:1542 vs http/server.go:1975 | **110-FIXED** wired via `server.go:1975` union + `lib/api.ts:933` — `subagent-10` |
| REF-BKP-DUP01 | Phase3 | P2 | DUPLICATE | service.go vs MainService quartet dual stacks | services/backup/service.go:271 vs main_service.go | — |

| REF-NET-F-NET-01 | Phase4 | P0 | BROKEN | Empty-rule ingress sync wipes whole gateway config every 30s | crossnode/ingress_sync.go:111 + caddy_proxy.go:673-720 | **110-FIXED (hotfix)** `crossnode/ingress_sync.go:128` guard `len==0` no-op — `subagent-07` |
| REF-NET-F-NET-02 | Phase4 | P0 | BROKEN | Asymmetric merge discipline: traffic/TLS apply erases domain routes | caddy_proxy.go:707-720 vs 523-593 | **110-FIXED (hotfix)** fetch-merge-preserve + sub-resource `POST /config/apps/http/servers/gamepanel` — `subagent-07` (CONFIRMED_BY: subagent-04 F1, subagent-03 L1) |
| REF-NET-F-NET-03 | Phase4 | P0 | BROKEN | validate=apply + snapshot-after-mutation rollback restores new config | caddy_proxy.go:980-1002,691-702 | **110-FIXED (hotfix)** snapshot BEFORE validate + `POST /adapt` dry-run — `subagent-07` |
| REF-NET-F-NET-04 | Phase4 | P0 | FALSE_COMPLETION | Fictional Caddy handlers brick route updates when policies on | caddy_proxy.go:795-875 | **110-FIXED (hotfix)** gated `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` OFF — `subagent-07/08` |
| REF-NET-F-NET-05 | Phase4 | P0 | SECURITY | All policies apply to all routes (cross-tenant leakage) | traefik_proxy.go:960-972, caddy_proxy.go:722-793 | **110-FIXED (hotfix)** empty set safer than leak; join table PLANNED — `subagent-08` (CONFIRMED_BY: 3 of 5 subagents) |
| REF-NET-F-NET-06 | Phase4 | P0 | BROKEN | Grouped-route removal misses dead nodes | caddy_proxy.go:60-74,949-951 | **110-FIXED (hotfix)** group-aware withdrawal via cached groups — `subagent-07` |
| REF-NET-F-NET-07 | Phase4 | P0 | BROKEN | TCP rules render as HTTP/SNI-only; UDP half-implemented | trafficmanager/service.go:383-387, traefik_proxy.go:854-905 | **110-FIXED (hotfix)** protocol branch 400/skip — `subagent-07` |
| REF-NET-F-NET-08 | Phase4 | P0 | UNWIRED | SetCertificate zero callers — certs never leave Postgres | gateway_adapter.go:49 | **110-FIXED** wired in `acme/service.go:121` + `main.go:1020` `deliverCertificateToGateway` — `subagent-08` |
| REF-NET-F-NET-09 | Phase4 | P0 | DEAD | HTTPSolver never mounted; default challenge type fails | acme/service.go:157 | **110-FIXED** `server.go:1118` `/.well-known/acme-challenge/*` + `:80` standalone solver — `subagent-08` |
| REF-NET-F-NET-10 | Phase4 | P0 | BROKEN | Networking admin UX non-functional (schema mismatch/405s) | app/admin/traffic/page.tsx:14-22 vs service.go:24-39 | — |
| REF-NET-F-NET-11..25 | Phase4 | P1 | MIXED | Oscillation, LB config ignored, weights dead, strategy mistranslation, rule injection, DNS SSRF, key reuse, plaintext creds, firewall ephemeral, wrong vantage, fail-open resolve, obs fiction, trusted-proxy fail-open, revoke no-op, LB SSRF probe | see phase-04 reports | — |

| REF-ORCH-AF-1 | Phase5 | HIGH | UNWIRED | Durable event pipeline write-only; Relay zero production subscribers | eventstore/outbox.go:39, http/server.go:148 | **110-PARTIAL** `PublishTx` + Relay fan-out bridged — `subagent-12`; prod subscribers PLANNED |
| REF-ORCH-AF-2 | Phase5 | HIGH | MISSING | No leader election; ~30 daemons/instance; elector only in dead River fork | main.go ~20 Start() sites, queue/leader.go:23 | **110-PARTIAL** `214_forge_leader.sql:1` + `queue/leader.go:23` TTL elector — `subagent-12`; gates ~8 daemons via `FORGE_LEADER=1` + `QUEUE_SINGLE_WRITER` |
| REF-ORCH-AF-3 | Phase5 | HIGH | BROKEN | Fencing writes tokens nothing enforces; fires on recovery not failure; impl twice | fencing.go:20-49 vs recovery/service.go:642-650 | — |
| REF-ORCH-R-01 | Phase5 | P0 | FALSE_COMPLETION | LXC/KVM phantom providers — beacon drops provider field, always Docker | beacon/server.go:740-772,841 | CONFIRMED_BY: subagent-03 |
| REF-ORCH-Q-01..06 | Phase5 | P1 | MIXED | Cancel unsound, fake backoff, per-replica periodic dupes, non-tx transitions, idempotency fragmentation, heartbeat dies on Stop | queue/store.go:113-130 etc | — |
| REF-ORCH-ST-01 | Phase5 | P2 | DEAD | StorageLocality scoring dead code + vocabulary drift | scheduler/service.go:317-323 | — |
| REF-ORCH-AF-8 | Phase5 | P0 | BROKEN | Failover defaults to Evacuate when no policy matches | failover/service.go:181-211 | — |
FORGE-LOGIC-001 | Phase1 | P0 | FALSE_COMPLETION | Deployment stubs lie completed | deployment/execution.go:249 | CONFIRMED_BY: REF-APP-C02 |
| FORGE-LOGIC-002 | Phase1 | P1 | BROKEN | Health gate localhost | deployment/healthgate.go:11 | CONFIRMED_BY: REF-APP-C08 |


## Index maintenance

When a subagent reports a finding also seen elsewhere:
- Do not create a new ID
- Add confirming source to `CONFIRMED_BY`
- Synthesis records `first discovery` vs `confirmation`

## Stats

- Total findings: ~175 consolidated (Phase1 31 + Phase2 22 + Phase3 18 + Phase4 25 + Phase5 ~15 + Phase6 18 + final-parity ~180 rows re-verified with 10 parallels; line-count deduped)
- Phase 1 remediation: 24 addressed (VERIFIED_FIXED / ALREADY_FIXED / INVALIDATED), 13 DEFERRED_WITH_REASON or BLOCKED — details in phase-01-remediation/
- Final parity: 10 parallel subagents re-verified entire Forge stack vs full audit corpus — 3 of 4 Phase-1 admin BROKEN chains now FIXED (container/network/volume create+prune), 3 Phase-1 logic fixes landed (HMAC 401, cache forward, builder admission), 5 Phase-1 gift+FIXED; see audits/final-parity/ (10 reports, 700+ lines each avg) + audits/FINAL_PARITY_AUDIT.md

# Phase 6 Addendum (Remaining Reference Deep-Dive — 10 parallel agents)
| REF-P6-APP-01 | Phase6 | P1 | BROKEN | resolveTemplate silently swallows interpolation failure deploying stale compose | appstore/service.go:239 | **110-FIXED** no longer swallows; error surfaces via `subagent-05` |
| REF-P6-APP-02 | Phase6 | P1 | BROKEN | uninstall deletes DB row even when DeleteComposeStack fails → orphan stack+reservation | appstore/service.go:139 | **110-FIXED** orphan guard `subagent-05` |
| REF-P6-APP-03 | Phase6 | P2 | PARTIAL | UpgradeApp ignores CrossVersion/ignore_upgrade guard | appstore/service.go:158 | **110-FIXED** `212_appstore_upgrade_guards.sql` + `cross_version`/`ignore_upgrade` — `subagent-05` |
| REF-P6-WEB-01 | Phase6 | HIGH | FALSE_COMPLETION | proxy-domains verify hardcoded verified:true stub | handlers_proxy_domains.go:199 | CONFIRMED_BY: Phase4 F-NET verify |
| REF-P6-WEB-02 | Phase6 | P1 | BROKEN | compose verify global mu held across network I/O serializing reverify | domains/service.go:362 | — |
| REF-P6-WEB-03 | Phase6 | P1 | BROKEN | dns/service global os.Setenv env-mutation leaks | dns/service.go:637 | — |
| REF-P6-WEB-04 | Phase6 | P1 | BROKEN | renewOnce leaves chain (leaf DER only, no fullchain PEM) | acme/service.go:492 | **110-FIXED** fullchain PEM `cert.Certificate` not `x509Cert.Raw` — `subagent-08` |
| REF-P6-DB-01 | Phase6 | P1 | BROKEN | list DB containers omits encrypted columns → empty creds in fleet view | store_db_containers.go:174 | **110-FIXED** `COALESCE(..._encrypted)` + `decryptDBContainerSecrets` — `subagent-13` |
| REF-P6-DB-02 | Phase6 | P1 | BROKEN | mysql.RegisterTLSConfig global leak/race per host | dbprovisioner/service.go:373 | — |
| REF-P6-DB-03 | Phase6 | P1 | BROKEN | Deprovision swallows daemon error hard-deletes row; Restart is status-only no-op | containers.go:289 | — |
| REF-P6-COMP-01 | Phase6 | P0 | BROKEN | API vs beacon compose volume policy bifurcation (200 valid then 400) | service.go:594-677 vs compose.go:226-248 | **110-PARTIAL** documented; `FORGE_ENV_FILE_STRICT` aligns 200→400 contract — `subagent-06` (CONFIRMED_BY: Phase4 merge discipline) |
| REF-P6-COMP-02 | Phase6 | P0 | BROKEN | env_file silently dropped (secrets missing, no error) | compose/service.go:14 | **110-FIXED** `FORGE_ENV_FILE_STRICT` 422/400 — `subagent-06/13` |
| REF-P6-COMP-03 | Phase6 | P1 | BROKEN | short-form ["80"] bypasses privileged-port check | beacon/compose.go:213-224 | **110-FIXED** `beacon/compose.go:213` parts[0] — `subagent-06` |
| REF-P6-COMP-04 | Phase6 | P1 | BROKEN | DeployFromGit calls Update on fresh stackID (fails create) | gitops.go:381-387 | **110-FIXED** → `CreateComposeStack:390` — `subagent-06/15` |
| REF-P6-TMPL-01 | Phase6 | P1 | UNWIRED | 93% seeding deficit: 44 puffer templates vs 14 on branch vs 1 on main, not seeded | packages/game-templates + migrations/091 | **110-FIXED** restored 14 + idempotent upsert — `subagent-01/13` |
| REF-P6-TMPL-02 | Phase6 | P2 | PARTIAL | 24 typed puffer ops collapsed to single shell blob; groups/if/internal lost | spec.json:307 vs template-schema.json | — |
| REF-P6-UNCL-01 | Phase6 | INFO | ARCHITECTURE | optimistic name check vs Forge unique constraint; mesh vs central placement choice validated | service.go:28-86 | — |
| REF-P6-PLUG-01 | Phase6 | P1 | FALSE_COMPLETION | plugins metadata-only, handlers_plugins.go:48 admits no runtime | services/plugins/plugin.go:69 | CONFIRMED_BY: Phase1 CapRover/Dokku plugins |
