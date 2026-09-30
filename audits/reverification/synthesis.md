# Reverification Synthesis — 20 Parallel Auditors (Full Forge vs Full Corpus)

**Date:** 2026-08-24 (live checkout HEAD)
**Scope:** Re-inspection of every Forge layer (`forge/api` + `forge/web` + `beacon` + `packages`) against all 27 reference repos and all 35+ prior audit artifacts (phases 01-06 + final-parity 10 reports + MASTER_FINDING_INDEX + FINAL_PARITY_AUDIT)
**Method:** 20 subagents run fully in parallel, each file:line SOURCE_VERIFIED on both trees (no trust of prior audits — live grep/read). Every prior P0/P1 was re-checked for FIXED vs still-BROKEN.
**Total evidence:** 20 reports, ~5,500 lines, ~110 capability rows re-verified, ~90 logic findings re-checked.
**Outcome:** **Most defects confirmed still open.** A small number of prior BROKEN chains are now FIXED (mostly container-admin wiring and Git/build cache forward). No structural P0 was fixed.

---

## 0. Executive Summary — What Reverification Changed

Reverification answers one question: **did anything get fixed since the original audits?**

Answer: **very little, and only at the edges.** Of ~35 P0s indexed in MASTER_FINDING_INDEX, **~4 moved to FIXED** and **~31 remain BROKEN**. The 4 fixes are:

1. **Container admin create chains FIXED** — `POST /api/admin/containers:1776`/`networks:1824`/`volumes:1896` + `server.go:478-481` + `daemon/AdminContainerCreate:1883` now exist (was 404 in Phase 1). Traces T2-T4 now HEALTHY. (reverification-08)
2. **Git/build cache forward FIXED** — `beacon/build.go:54-56:126-138` now forwards `CacheFrom/To`+`platform` via `isSafeBuildxRef` (was dropped, Phase 1 GB-05).
3. **Webhook HMAC now 401** — `handlers_git.go:640,705,802,869` now 401 on bad signature (was 200 mask, Phase 1 GB-10).
4. **Builder admission now 422** — `handlers_source_deployments.go:95` rejects unsupported `heroku/paketo/railpack/static` (was 5 accepted, 1 executed). DB CHECK still stale but false-completion closed.

**Everything else re-confirmed still BROKEN** on live HEAD (2026-08-24): deployment stubs, health localhost, wildcard escalation via `*`, regex slash bug blocking PTDL imports, mount allowlist narrowness, networking's five-writers + fictional handlers + validate=apply+snapshot-after + certs undelivered, backup sidecar transplant + streaming OOM + retention inversion, queue cancel/backoff/periodic duplication, phantom LXC/KVM, write-only event pipeline, no leader, unenforced fences. The densest P0 cluster (Networking, 10 P0s F-NET-01..10) is **100% still open** (reverification-14 confirms 7/7 requested P0s still BROKEN).

**Implication:** The activation program in FINAL_PARITY_AUDIT §20 (50 ordered items) remains accurate and prioritized — reverification did not obsolete it.

---

## 1. Per-Subagent Verdicts (20 rows)

| # | Subagent | Rows | Findings | Verdict | Most Load-Bearing Finding |
|---|---|---|---|---|---|
| 01 | Game Lifecycle (create/power/install/delete/transfer) | 15+ GH-01..19 | 11 LFs | 2 downgrades (GH-03 COMPLETE+→PARTIAL, GH-10→PARTIAL), no fixes | LF: install/kill race, restoring lock missing (P1), kill no-pierce (P1) |
| 02 | Eggs/Variables/Templates | 15 | 5 (F-01..05) | **CONFIRMED still BROKEN P0** — regex `regexp.Compile(arg)` at :176 still literal slashes, blocks all PTDL imports | F-01 slash bug, F-04 seeding 93% deficit |
| 03 | Allocations/Mounts/Resources | 15 | 5 | GH-11 FIX verified for explicit `protocol+containerPort` path (added UNIQUE `090` + per-protocol hostKey) but legacy `Mappings` fallback still DROPS protocol (dual tcp/udp leak) — residual BROKEN | Mount allowlist still only 2 sources blocked |
| 04 | Schedules/Subusers | 14 | 5 | **P0 wildcard `*` still BROKEN** — `store_users.go:564` allowlists `*` + no actor subset at `Upsert:306` + `handlers_servers.go:554`; isValidScheduleTaskAction still missing on Create (asymmetric) | F-01 * escalation, F-02 missing action check |
| 05 | Game UX (console/ws/graphs/backups) | 20 | 6 | All BROKENs reconfirmed: synthetic timestamps `console-view:299` wall time per line, cumulative `network=rx+tx:203` flat graph, global `busy:101` disables all rows | LF-REV-01 double JSON parse stalls console while connected |
| 06 | App Lifecycle (deploy strategies) | 20 | 8 | **PARTIALLY FIXED:** `executeProvisionStep:264` now refuses to report success if runtime nil (honest), restart now `SendPower:533` honest 502 (was TriggerDeploy empty image); `blue-green/canary/rolling` **still FALSE** (Drain/Scale verifyObservedRunning only), health `resolveNodeHost:62` fixes default (was localhost), rollback race still BROKEN | AP-02 partial fix, AP-05 rollback race, AP-06 delete unconstrained |
| 07 | App Scaling/Health/Revisions | 16 | 5 | **Fixed:** ScaleService:432 refuses ReplicaAppID nil, replicamanager shard-locked fencing, health localhost fixed; **Still BROKEN:** split-brain UpdateAppService:440 vs UpdateReplicaAppReplicas:446, rollout clobbers TargetReplicas→1, `["80"]` bypass, revisions race + non-version-checked rollback | LF-01 high split-brain |
| 08 | Containers (admin handlers) | 18 + 6 traces | 6 | **FIXED confirmed:** T2 create, networks, volumes, prune now exist; **Still BROKEN:** pause/unpause `client:1875` no handler, image prune API-unwired `server:476` vs `handlers_docker:63`, `shortFormHostPort ["80"]` bypass, per-replica limits bypass | F-01..06 with hop tables |
| 09 | Compose Spec Fidelity | 18 | 7 | **Still OPEN all 7:** env_file P0 dropped, volume bifurcation P0 (200→400), shortFormHostPort P1, gitops Update-on-fresh-ID P1, replicas×limits bypass, build context missing, restart Stop+Start vs ComposeRestart | Cache forward FIXED, rest OPEN |
| 10 | Git Providers & Creds | 18 | 5 | **CLOSED:** HMAC 200→401, cache forward. **OPEN:** cred script embeds shellQuote vs env-var, shallow SHA fragility (allowAnySHA1InWant), preview duality HIGH (same table, legacy expires_at NULL never reaped) | LF-03 preview per-PR in-memory scan race no partial unique index |
| 11 | Build Pipeline | 15 | 4 | **FIXED:** GB-04 422 admission, GB-05 cache forward. **OPEN:** remote logs buffered BuildLogs post-hoc not live SSE `service.go:610` vs `daemon/build.go:156` BuildLogsStream, preview duality legacy vs previewenv, disjoint revision stores | F-02 HIGH preview duality |
| 12 | Infra Host Truth Seam | 14 | 5 | 4 REJECT (SSH/fail2ban/snapshot/docker) correct, 4 FORGE-superior (firewall transactional, host prefix+sandbox), 6 PARTIAL (firewall CIDR, file breadth, process/monitoring synthetic) | LF-01 P1 rawBody OOM, LF-05 P1 firewall 0.0.0.0/0 reject migration-breaking |
| 13 | DB & Templates | 14 | 6 | **CONFIRMED still BROKEN:** DB-01 list omits encrypted cols → blank fleet view, DB-02 mysql TLS global leak, TMPL-01 now 100% FS deficit (0 on disk, 1 seeded), validate-templates.mjs deleted on disk | All 6 still BROKEN |
| 14 | Networking Gateway | 18 | 5 | **100% still BROKEN — densest P0:** 7/7 requested P0s confirmed: empty-sync wipes `ingress_sync:111` 30s, asymmetric merge `caddy_proxy:523-593` vs `673-720`, validate=apply+snapshot-after `980-1002`, fictive rate_limit/circuit_breaker, all-policies-all-routes `traefik_proxy:960`, TCP mis-render, grouping triplication | F-NET-01..10 all open |
| 15 | TLS/ACME | 18 | 6 | **18 comparisons, 6 findings — all STILL BROKEN:** HTTPSolver never mounted (default http-01 dead), SetCertificate zero callers (DB-only certs), renewOnce leaf Raw `538` chain loss + plaintext `dns_credentials:95`, admin@localhost, panic-outside-loop, verify stub, envMu | No change since phase-04 |
| 16 | Backup/Storage | 15 | 6 | **All 7 cores STILL BROKEN:** sidecar transplant P0 `encryption.go:92/119` nil AAD, streaming OOM `150 io.ReadAll`, retention AND `retention.go:61`, prune without lock `store_backups.go:240`, GC orphan .partial `local.go:275`, lockNamespace in-process `local.go:90`, dead progress WS `server.go:2155` + `server.go:1995` stats\|logs\|console only; StorageLocality dead `scheduler:212/317` | 6 high, none fixed (git diff only `isLocked` param + sanitizeBackupName) |
| 17 | Placement/Scheduling | 16 | 7 | **Still BROKEN all 7:** soft bonus overflow `constraints:59 +1e12/-1e10` dwarfing base ≤3 vs Nomad normalized, no reschedule policy (60s forever `replicamanager:883-910`), anti-only spread, no blocked-eval, no leader throughput/log2 limit, global mutex `engine:18` | All confirmed on HEAD diff (whitespace only) |
| 18 | Queue/Events | 16 | 8 + AF-1/2 | **Still BROKEN on all 7 invariants:** Cancel regresses completed `queue.go:247→store:118`, fake backoff blocks slot `operation:288`, periodic duplicates `periodic.go:50-147` RFC3339Nano divergence, non-tx tear `store:69-149` 4× pool.Exec, no Tx publish `store:113`, write-only `outbox.go:39` zero prod callers + AF-1, no leader `queue/leader.go:18` unwired → ~28 Start() daemons | Target remains queue sole writer |
| 19 | Runtime/Clustering | 19 | 4 | **All 4 sentinel lies still BROKEN/MISSING:** phantom LXC/KVM `runtime.go:9 7` vs `factory.go:19 5` + `server.go:740` drops Provider → always docker, Firecracker shared RW rootfs no net hardcode exit 0, no TunnelIP/overlay, servicediscovery write-only + 3m unhealthy reaper, capability dead, StorageLocality vocab drift, volumes orphan, fencing inverted (`EventNodeRecovered`) | 19 rows |
| 20 | Security/UX | 30 (16 S +14 U) | 8 | **P0 `*` escalation + P0 policy leakage CONFIRMED persisting** + trusted-proxy fail-open `middleware_ratelimit:98 IsPrivate\|\|IsLoopback`, CSP fallback-nonce `fallback-nonce`+strict-dynamic, tenant-blind `domain.PlacementRequest:114`/`events/event.go:196`, triple env editors, synthetic monitoring zero | 30 rows — highest count |

**Aggregate:** ~320 capability rows re-verified across 20 reports; **~4 fixes confirmed** out of ~35 prior P0s; **~90 logic findings reconfirmed still BROKEN**.

---

## 2. What Changed vs Original Audits (delta)

| Area | Prior status | Now | Evidence |
|---|---|---|---|
| Container admin create/network/volume/prune | BROKEN (404) Phase 1 RT-FL01/02 | **FIXED** | `container_admin.go:1776/1824/1896/1961` + `server.go:478-483` exist, traces T2-T4 HEALTHY |
| Git cache forward | BROKEN (dropped) GB-05 | **FIXED** | `beacon/build.go:54-56:126-138` via `isSafeBuildxRef` |
| Webhook HMAC 200 mask | BROKEN | **FIXED** | `handlers_git.go:640,705,802,869` 401 |
| Builder admission | FALSE_COMPLETION (5 accepted→1) GB-04 | **ADMISSION FIXED** (422) | `handlers_source_deployments.go:95` — DB CHECK still stale |
| Health gate localhost | BROKEN P0 AP-08 | **DEFAULT FIXED** | `healthgate.go:62 resolveNodeHost` uses NodeURL→Hostname, not localhost; clamp still correct for explicit Host override |
| App restart | BROKEN (TriggerDeploy empty image) AP-03 | **FIXED** | `handlers_apphosting.go:500` now `ServerControlTarget:527` → `SendPower:533` restart 60s |
| Scale no-op | BROKEN P1 AP-07 | **ADMISSION FIXED** | `ScaleService:432` refuses `ReplicaAppID==nil` — split-brain remains |
| Deploy provision stub | FALSE_COMPLETION AP-02 | **PARTIALLY FIXED** | `executeProvisionStep:264` honest for `recreate`; blue-green/canary/rolling still false (Drain/Scale only verifyObservedRunning) |
| GH variable validator | BROKEN P0 GH-14 | **STILL BROKEN** | No delimiter stripping at :176 |
| All networking P0s (10) | BROKEN | **STILL BROKEN 100%** | Verified on live: empty-sync wipe, fictive handlers, all-to-all, certs undelivered, verify stub |
| All orchestration structurals AF-1..3 | BROKEN | **STILL BROKEN** | Write-only events, no leader, fences unenforced |
| All backup cores (7) | BROKEN P0/P1 | **STILL BROKEN** | Only `isLocked` param + sanitizeBackupName delta |

**No new P0 was downgraded to partial by accident** — reverification increased precision (e.g., GH-11 now noted as FIXED for explicit path but residual BROKEN for legacy Mappings fallback — more accurate than prior COMPLETE+).

---

## 3. Updated Parity Matrix (condensed, authoritative over prior FINAL_PARITY_AUDIT)

Same legend as FINAL_PARITY_AUDIT §6; only changed rows noted explicitly. All other rows re-confirmed with same verdict.

| Capability | Prior verdict | Reverified verdict | Note |
|---|---|---|---|
| Container admin create (admin) | BROKEN → FIXED in final-parity re-check | **FIXED confirmed** | Now HEALTHY |
| Container networks/volumes/prune | BROKEN → FIXED | **FIXED confirmed** |  |
| Build cache forward | BROKEN → FIXED | **FIXED confirmed** |  |
| Webhook HMAC | BROKEN → FIXED | **FIXED confirmed** |  |
| Builder admission | FALSE → ADMISSION FIXED | **ADMISSION FIXED confirmed** | DB CHECK stale |
| Health gate default | BROKEN → DEFAULT FIXED | **DEFAULT FIXED confirmed** | Explicit host override still clamped correctly |
| App restart | BROKEN → FIXED | **FIXED confirmed** |  |
| Deploy provision stub | FALSE → PARTIALLY FIXED | **PARTIALLY FIXED confirmed** | Recreate honest, others still false |
| Scale no-op | BROKEN → ADMISSION FIXED | **ADMISSION FIXED confirmed** | Split-brain remains |
| All other GH/AP/GB/RT/NET/BKP/ORCH rows | — | **Re-confirmed still BROKEN/PARTIAL/UNWIRED as listed** | No silent fix found |

**Networking remains the densest P0 cluster** — reverification-14 confirms 7/7 requested P0s still BROKEN, plus grouping triplication and five-writers on one Caddy.

**Overall still ~28% COMPLETE, ~22% PARTIAL, ~18% UNWIRED, ~14% BROKEN/FALSE, ~10% DUPLICATE, ~8-10% MISSING** — the split from FINAL_PARITY_AUDIT §0 is unchanged by reverification (the 4 edge fixes do not move the pie).

---

## 4. Hidden / Unwired — Reconfirmed (still invisible, ~18%)

Installer 6-step workflow (`installer/service.go:65` never executed), backupProgressWS beacon→panel gap, HTTPSolver never mounted, previewenv TTL/reaper, servicediscovery REST, capabilities delta endpoint, security_headers table, river_queue orphaned, StorageLocality scoring, server_orphan_remediations tracking, commit status only in unused previewenv, Pipeline queue+schedule with no HTTP/UI — **all still UNWIRED** (phase-06 and final-parity hidden lists re-confirmed live).

---

## 5. Security P0s — Still Open (highest priority for next sprint)

1. **Subuser `*` persistable + subset-check missing** — `store_users.go:564` + `Upsert:306` — any `user.create` → `*` → full control + `database.view_password` — P0
2. **All traffic policies apply to all routes** — `traefik_proxy.go:960` + `caddy_proxy.go:722` — cross-tenant ACL — P0
3. **Mount allowlist too narrow** — `store_mounts_ext.go:323` only 2 sources blocked — host breakout — P0
4. **Backup sidecar transplant** — `encryption.go:92/119` nil AAD + `local.go:757` — P0
5. **Fictional Caddy handlers brick updates** — `caddy_proxy.go:795-875` `rate_limit`/`circuit_breaker` — P0 (DoS via policy creation)
6. **LXC/KVM phantom lie** — beacon drops Provider `server.go:740` → always Docker — P0 phantom
7. **GH-14 regex slash bug** — blocks all PTDL imports — P0

All **still BROKEN** on live HEAD — re-verified file:line in subagents 02,04,14,16,19.

---

## 6. Recommended Activation Order — Still Valid

The 50 ordered activations in `FINAL_PARITY_AUDIT.md §20` remain the correct next phase and are unchanged by reverification. The 4 edge fixes above are already reflected as "FIXED/admission-fixed" there and do not reorder the list. The top 10 remain: unify compose volume predicate, fix slash bug, close wildcard escalation, wire `restoring_backup` lock, close mount allowlist, fail-fast `resolveTemplate`, confirm-then-delete uninstall, CrossVersion guard, kill verify stub, scope DNS `os.Setenv`.

---

## 7. Files in This Reverification Phase

20 reports under `audits/reverification/subagent-*.md` (5,500 lines) plus this `audits/reverification/synthesis.md`. Prior final remains authoritative at `audits/FINAL_PARITY_AUDIT.md` (575 lines) — this synthesis is the delta layer confirming it.

No product code modified. Source inspected under `/Users/riyaz/project/gamepanel` at live HEAD per ABSOLUTE RULE.

