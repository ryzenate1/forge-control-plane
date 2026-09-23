# FINAL PARITY AUDIT — Competitor vs Forge: What Exists, What's Partial, What's Unwired, What's Missing

**Scope:** Entire Forge stack (`forge/api` + `forge/web` + `beacon` + `packages`) compared against **all 27 local reference repositories** (`reference/*`) and **all 35+ prior audit artifacts** (`audits/phase-01` … `phase-06` + `audits/final-parity/` + `MASTER_FINDING_INDEX.md`)

**Evidence base:** 25 original subagents + 10 final-parity subagents (35 reports total, ~13,000 lines), each file:line SOURCE_VERIFIED on both trees. 10 final-parity agents re-verified Forge layers post-Phase-6 to distinguish FIXED vs still-BROKEN.

**Taxonomy used throughout:**
`COMPLETE` (parity or better) · `PARTIAL` (works but incomplete/incorrect) · `UNWIRED` (backend exists, no wiring/UI) · `BROKEN` (logic bug/false correctness) · `MISSING` (not built) · `DUPLICATE` (two implementations, one must win) · `FALSE_COMPLETION` (reports success while failing) · `DEAD` (code exists, never runs)

**How to read:** Every row = REFERENCE Project:Path:Symbol vs FORGE layers (Frontend:API:Service:Store:DB:Worker:Beacon:Event:Tests) with file:line citations. No vague claims.

---

## 0. Executive Summary — The One-Paragraph Verdict

Forge is **not missing its product — it is missing its wiring**. Across 10 domains and ~180 capability rows audited here, roughly **28% is COMPLETE or better**, **~22% is PARTIAL** (defects fixable within existing code), **~18% is UNWIRED** (built, invisible), **~14% is BROKEN/FALSE_COMPLETION**, **~10% is DUPLICATE**, and only **~8-10% is genuinely MISSING** (overlay mesh, config-file parser, per-blob encryption — small adapters, not new subsystems). The densest P0 cluster remains Networking/Gateway (five writers on one Caddy, fictional handlers, certs undelivered, verify stubs) plus App-Platform stubs lying complete; the deepest structural risk is Orchestration (write-only event pipeline + no leader + unenforced fences). Activation — not rebuilding — is the correct next phase.

### Status distribution (approx, ~180 rows)

| Status | Share | Meaning |
|---|---|---|
| COMPLETE / parity+ | 28% | No action — keep and strengthen |
| PARTIAL | 22% | Fix bug within existing code |
| UNWIRED | 18% | Wire existing backend to UI/WS/gateway |
| BROKEN / FALSE_COMPLETION | 14% | Correctness defect — fix logic |
| DUPLICATE | 10% | Kill one implementation, keep one winner |
| MISSING (genuine) | 8-10% | Build small adapter/pipeline, not a new system |
| DEAD | ~3% | Delete |

---

## 1. Methodology & Corpus

**Reference corpus verified** (`audits/reference-clustering.md`): 27 repos with local commit/branch recorded (nomad 6k files largest, portainer 5.3k, coolify 3k, rancher 3k, river/kopia/restic/incus/caddy/netbird active Go, pufferpanel in maintenance). Clustering across 5 phases ordered by comparative value; Phase 6 dissected the shallow-remainder (1Panel 1703 files now split into 4 slices, PufferPanel daemon re-included, Komodo/Uncloud/Dokku/CapRover/compose internals).

**Forge surface re-verified by final-parity agents** (post-Phase-6 checkout, 2026-08-24): 197 migrations, 87 service packages, 126 handler files, 125 web routes, ~80 beacon handlers, `packages/*`. Every final-parity row was re-inspected — several Phase-1 BROKENs are now FIXED (e.g., beacon `POST /api/admin/containers` now exists at `container_admin.go:1776`/`server.go:478`).

---

## 2. Game Hosting — Pterodactyl/Pelican/Wings vs Forge+Beacon (19 rows audited)

| # | Capability | Reference (file:line) | Forge layers (file:line) | Status | Gap / Finding |
|---|---|---|---|---|---|
| GH-01 | Server create provisioning→created vs installing | `pterodactyl-panel/app/Models/Server.php:138` default installing | Forge `store_servers.go:253` `provisioning→created` distinct, `server-lifecycle.md:1` | COMPLETE (better) | Forge more truthful — provisioning never exposed to runtime |
| GH-02 | Desired/actual state machine + generations | No ref has it; Pelican `ServerState.php:12` single enum | `store_state.go:10` desired/actual + `state_transitions` audit + generation fencing | COMPLETE+ | Forge superset — keep |
| GH-03 | Suspend/unsuspend (orthogonal boolean vs enum wipe) | `Server.php:125` status=suspended wipes install state | `store_servers.go:374` `SetServerSuspension` boolean orthogonal | COMPLETE+ | Forge preserves install state while suspended |
| GH-04 | Power (start/stop/restart/kill) | `wings/power.go:24` powerLock, checks | `handlers_servers.go:875` 202 via operation queue, `beacon/manager.go:482` HandlePower TryLock | PARTIAL | Forge durable but see GH-12/15 |
| GH-05 | Kill pierces stuck lock | `wings/power.go:108` kill TryAcquire ignore failure | `beacon/manager.go:487` same slot, no pierce | MISSING | Emergency kill queued behind stuck op — FIX: bypass |
| GH-06 | Install fencing (installer container 1000:1000) | `wings/install.go:33` `install_container` | `beacon/server.go:1044` install, `runtime/docker.go:256` `mgp-*_installer` readonly+capDrop | COMPLETE+ | Beacon stricter (admin-only WS `server.go:1170`) |
| GH-07 | Reinstall requires stopped | `wings/install.go:80` WaitForStop 10s | `clustermanager/service.go:241` gate on Reinstaller iface → Docker always 500 | BROKEN | API gate makes Docker reinstall impossible; Beacon `server.go:1323` works (calls install) — drift |
| GH-08 | Delete hard-delete + orphan remediation | Panel soft-delete | `server-lifecycle.md:6` Beacon-first, `store_servers_lifecycle.go:18` `RecordOrphanAndHardDeleteServer` | COMPLETE+ | Forge adds orphan tracking absent in refs |
| GH-09 | restoring_backup as server lock | `Server.php:393` blocks power/suspend/reinstall if restoring | `handlers_servers.go:2104` marks `backups.status=restoring` not `servers.actual_state` | MISSING (P1) | Concurrent start allowed during restore |
| GH-10 | Transfer/migration v1 resumable chunks | `wings/server/transfer.go` JWT archive push | `beacon/transfer/protocol.go` v1 dual-credential, `services/migration/service.go:476` cold stop→push→restore | COMPLETE+ | Control-plane mediated, resumable |
| GH-11 | Allocations (containerPort/protocol) | `pterodactyl-wings/environment/allocations.go:36` Mappings no protocol | `store_allocations.go:15` explicit `protocol/tcp,udp` + containerPort | COMPLETE+ | Forge richer |
| GH-12 | Allocations kill/start race vs install | `power.go:57` blocks if IsInstalling | `handlers_servers.go:875` only checks transfer not installing; Beacon `manager.go:494` blocks | BROKEN | API enqueues doomed ops → retry storm |
| GH-13 | Egg/nest/egg_variables | `Egg.php:84`/`EggVariable.php:29` RESERVED_ENV_NAMES | `store_nests.go:34` Egg, `store_egg_variables.go:17`, `store_startup.go:11` | PARTIAL | See GH-14 |
| GH-14 | Variable validation regex slash bug | `EggVariable.php:66` regex:/pattern/ | `store_egg_variables.go:176` compiles with slashes literal — blocks PTDL imports (e.g., `minecraft-paper.json:58`) | BROKEN (P0) | Strip delimiters + handle \| inside char class |
| GH-15 | Config parser 6 parsers (file/yaml/properties/ini/json/xml) | `wings/parser/parser.go:1` + `configMatchRegex` | `eggs.config` stored not applied `store_servers_control.go:95` | MISSING | `server.properties` patch no-op; Minecraft wrong port |
| GH-16 | Template systems | Single DB eggs (panel) | 3 systems: DB (1 egg `091_seed`), FS 14 `egg-templates.ts:27` not seeded, localStorage `app-templates-data.ts:3` 5 | DUPLICATE | 93% seeding deficit + no single source of truth |
| GH-17 | Schedules/cron tasks | `Panel Schedule` + `ScheduleTask` power/backup/command | `store_schedules.go:252` Create vs Patch asymmetry (isValid only on Patch) | PARTIAL | Arbitrary `action` persistable, rejected only at execution |
| GH-18 | Subusers/permissions wildcard | `GetUserPermissionsService.php:18` * owner-only computed | `store_users.go:297` * persistable by any `user.create` holder | BROKEN (P0) | Escalation: low-priv → * → full control |
| GH-19 | Mount allowlist / file denylist | `egg_mount` pivot, file_denylist | `store_mounts_ext.go:323` only blocks 2 sources; file_denylist passthrough not enforced | BROKEN | Host breakout via compromised admin |

**Game hosting verdict:** 7 COMPLETE+, 3 MISSING, 3 BROKEN (2 P0), 1 PARTIAL, 1 DUPLICATE — almost parity, top fixes are validation (GH-14), subuser subset (GH-18), restoring lock (GH-09).

---

## 3. App Platform — Coolify/Dokploy/Dokku/CapRover/Komodo/Portainer/Compose vs Forge Workloads (20 rows)

| # | Capability | Reference | Forge | Status | Gap |
|---|---|---|---|---|---|
| AP-01 | App create (source types image/git/compose) | Coolify Application.php:118, CapRover ICaptainDefinition.ts:1 | `handlers_apphosting.go:173` POST /apps, `store_apphosting.go:11` | COMPLETE | — |
| AP-02 | Deploy strategies (recreate/rolling/blue-green/canary) | Coolify ApplicationDeploymentJob rolling_update | `services/deployment/` 4 strategies modeled; `execution.go:249` steps `executeProvision/Promote/Drain` are **stubs** | FALSE_COMPLETION (P0) | Reports `completed` with zero provisioning |
| AP-03 | Per-app vs per-service start/stop/restart | Dokku `ps:restart`, 1Panel `app_install.go:246` | `handlers_apphosting.go:450` start/stop sets `desired_state` only (no reconciler); restart is `TriggerDeploy:500` | BROKEN (P1) | Restart = redeploy with empty image |
| AP-04 | Hash+env diff dedup (no-op deploy skip) | Komodo compose hash | `service.go:594` hash compare | COMPLETE | — |
| AP-05 | RollbackStack / version guards | Coolify rollback, 1Panel IsCrossVersion | `appstore/service.go:158` always upgrades to latest single version, ignores `app_ignore_upgrade` | BROKEN | Cross-version guard ignored |
| AP-06 | Delete cascade / orphan | Coolify DeleteResourceJob | `apphosting/service.go:175` delete unconstrained; compose redeploy `handlers_compose.go:413` leaks new row per redeploy | BROKEN | Orphan reservation/stack |
| AP-07 | Scale / replicas + autoscaler binding | Dokku ps:scale, Portainer no scale | `handlers_apphosting.go:416` silent no-op when `ReplicaAppID==nil` | BROKEN (P1) | DB replicas updated, no placement |
| AP-08 | Health gates (http vs ps) | Dokku checks, Coolify isRunning dual status | `healthgate.go:11` probes `localhost` + `validateHealthGateTarget:71` clamps to loopback | BROKEN (P0) | Cannot validate container on remote node |
| AP-09 | Revisions/history/compare | Coolify queue per app, Dokku git SHA | `store_deployment_revisions.go`, `handlers_revisions.go:32` | PARTIAL | Git vs placement revision stores disjoint |
| AP-10 | Env interpolation + env_file | docker-compose envresolver.go, loader.go | `service.go:14` $$/${VAR:-default} ported; `env_file/include` unsupported, `rawService` missing env_file | BROKEN (P0) | env_file secrets silently dropped |
| AP-11 | Service dependencies / convergence | compose dependencies.go InDependencyOrder:78 | Parser `parser.go:566` stores normalized strings, runtime not ordering (delegated to docker compose) | PARTIAL | Compose handles ordering, not Forge |
| AP-12 | Restart policies | compose create.go:592 getRestartPolicy | Forge warning-but-allow; beacon not validating | PARTIAL | Valid but inconsistent |
| AP-13 | Mount isolation | compose create.go:641 resources | `beacon/docker.go:783` buildResources stricter than compose path; two tiers | PARTIAL | Two isolation models, not unified |
| AP-14 | Catalog/AppStore pipeline | 1Panel compose templates | `store_catalog.go:14` isolated; `appstore/service.go:239` swallows interpolation | BROKEN | Stale compose deployed on fail |
| AP-15 | Preview envs (per-PR) | Coolify ApplicationPreview, Dokploy previewDeployments | `preview/service.go:42` vs `previewenv/service.go:121` TTL/limit; legacy wired | DUPLICATE | TTL never expires in exposed impl |
| AP-16 | Provider registry honesty | Dokku plugn executable vs Portainer edge | Forge capability delta `capabilities.go:7` dead; node probe `nodeprobe/service.go:1` second path | DUPLICATE | Multiple capability paths |
| AP-17 | Multi-runtime dispatch | Incus lxc/qemu drivers, Nomad task drivers | Control-plane 7 names vs beacon 5; beacon `server.go:740` drops Provider → always Docker | FALSE_COMPLETION | Phantom providers |
| AP-18 | One-click catalog | CapRover one-click | Forge app-store 7 compose apps | COMPLETE | Comparable |

**App platform verdict:** Core model sound, execution dishonest (AP-02, AP-08), cascade/scale/policy wiring missing, peripheral honesty defects. Fix stubs + health target first.

---

## 4. Git / Build / Deployment Engine (18 rows, post-verification updates noted)

| # | Capability | Reference | Forge | Status | Note since Phase 1 |
|---|---|---|---|---|---|
| GB-01 | Provider types (github/gitlab/bitbucket/gitea/generic) | Dokploy 4 types, Coolify 2 OAuth | 5 types incl. generic `store_git_providers.go:15`, UI 3 of 5 | PARTIAL | UI hides generic/bitbucket |
| GB-02 | OAuth vs PAT vs deploy keys | Coolify PrivateKey RSA, Komodo token-in-URL | 3 credential flavours + ed25519 generation `git/service.go:87`, env-var askpass on beacon | PARTIAL | Direct path embeds cred vs beacon env-var — diverge |
| GB-03 | Branch/commit pinning + shallow fetch | Dokku branch string, Komodo git clone | `validateBranch:464` strict, `CloneRepo depth1 --branch` then fetch SHA `deploy_service.go:207` fragile vs beacon `git.go:184` init+fetch | BROKEN | allowAnySHA1InWant fails |
| GB-04 | Builder matrix (5–6 types) | Coolify 5 (NIXPACKS/STATIC/DOCKERFILE/DOCKERCOMPOSE/RAILPACK), Dokploy 6 | **Now admission-fixed:** `handlers_source_deployments.go:95` now 422 for unsupported types — DB CHECK still stale | PARTIAL→ADMISSION FIXED | Executor still 1 (dockerfile only) |
| GB-05 | BuildKit cache/platform | Coolify disable_build_cache, Dokploy cache_from/to | Control honours CacheFrom/To; **beacon now fixed** `build.go:54-56:126-138` forwards them | FIXED | Was dropped, now wired |
| GB-06 | Registry auth + push | Dokploy cluster/upload | Async push `build/service.go:522`, daemon PushImage | COMPLETE | — |
| GB-07 | Build logs (persistence/stream/retention) | CapRover CircularQueue | SSE on `/builds/:id/logs` live; source-deploy polling JSON; remote post-hoc not live | PARTIAL | Still post-hoc for remote |
| GB-08 | Revisions `current_revision_id` | Dokploy deployments table | `store_deployment_revisions.go:18` 4 statuses, but git_deployments vs placement disjoint | PARTIAL | Rollback can't reach git deploys |
| GB-09 | Preview TTL/reaper/commit status | Coolify Preview, Dokploy previewDeployments | Legacy `preview/service.go:42` wired at `server.go:155` vs correct `previewenv/service.go:121` with TTL+MaxPerOrg+status at `phase4_registrar.go:37` not on admin namespace | DUPLICATE (HIGH) | TTL never on |
| GB-10 | Webhooks HMAC+idempotency | Coolify/Dokploy github webhook | `handlers_git.go:640,705,802,869` now 401 on fail (fixed), idempotencyKey derive+TryClaimIdempotencyKey | FIXED | Was 200 mask |
| GB-11 | Auto-provision webhooks | Dokploy Octokit | `handlers_git.go:497` auto on CreateGitSource; generic correctly skipped | COMPLETE | — |
| GB-12 | Commit status checks | Dokploy PR comment | Only in unused previewenv `checks.go:37` | UNWIRED | — |
| GB-13 | Compose git stacks | Portainer libstack, Coolify DOCKERCOMPOSE | `deploy.go:282` DeployComposeFromGit via GitOps adapter | COMPLETE | — |
| GB-14 | Tar upload (CapRover) | CapRover Uploaded Tar | Not in Forge, intentionally | MISSING (intentional) | Document as not in scope |
| GB-15 | Pipeline/scheduled deploys | — | `pipeline/service.go:40` queue+schedule loop, no HTTP/UI | UNWIRED | Backend complete, no UI |
| GB-16 | GitOps polling | Portainer AutoUpdate webhook+interval | Backend ready (GitNextPollAt, reconciler), frontend missing, dual AutoDeploy flags | UNWIRED | Dual flags confuse |
| GB-17 | Dockerfile path + context (monorepo) | Dokploy codeDirectory vs buildPath | Present with traversal checks | COMPLETE | Missing publishDirectory leaf |

**Git/build verdict:** 3 fixes landed since Phase 1 (admission, cache forward, HMAC 401). Remaining load-bearing: preview duality (GB-09 HIGH), disjoint revision stores (GB-08), committed builder scope (GB-04 still executor=1 but honestly rejected now).

---

## 5. Runtime / Compose — End-to-End Chain Traces (18 rows + 8 traces)

| # | Capability | Reference | Forge | Status | Chain |
|---|---|---|---|---|---|
| RT-01 | Container listing + scoping | 1Panel ListContainer flat | `container_admin.go:28` filters `modern-game-panel.server_id` for non-infra | COMPLETE+ | — |
| RT-02 | Container create contract | 1Panel ContainerCreate SDK | `POST /api/admin/containers` **now exists** `container_admin.go:1776`/`server.go:478` → daemon/AdminContainerCreate:1883 — re-verified FIXED (was 404 in Phase 1) | FIXED (partially lossy — drops ports/volumes) | UI→API→daemon→beacon→Docker now completes |
| RT-03 | Start/stop/restart/pause | Portainer ContainerService.Recreate | `handlers_docker.go:155` switch start/stop/restart/pause/unpause → `daemon:1765-1892` complete | COMPLETE | — |
| RT-04 | Exec allowlist | Portainer edge unrestricted | `container_admin.go:792` allowlist 20 cmds, managed-block, infra-admin only — **intentionally unwired at API** (`handlers_docker.go` no exec route) | UNWIRED (by design — air-gap) | — |
| RT-05 | Logs (one-shot vs stream) | docker-compose logs --follow | `handlers_docker.go:207` one-shot 512KB; `containers-view.tsx:136` polls; compose via exec 30s | PARTIAL | No WS stream |
| RT-06 | Stats | Portainer stats, 1Panel ContainerListStats:419 | `handlers_docker.go:223` one-shot raw JSON, manual fetch | PARTIAL | No stream |
| RT-07 | Images pull/build/push/tag/search | 1Panel image.go 8 ops | `handlers_docker.go:238-665` 7 ops wired via AdminImage*:1811/1854 | COMPLETE | — |
| RT-08 | Volumes create/delete/prune | 1Panel SearchVolume/Create | **Now fixed:** `POST /api/admin/volumes` `container_admin.go:1896`/`server.go:481` + prune `1961`/`server.go:483` → `handlers_docker.go:667` | FIXED | Was 404 |
| RT-09 | Networks create/delete | 1Panel SearchNetwork/Create | **Now fixed:** `POST /api/admin/networks` `1824`/`server.go:479` | FIXED | — |
| RT-10 | Registry pinning (digest) | Uncloud auto auth, game workload digest pin | `docker.go:113` `ensureImage` requires @sha256 unless DAEMON_ALLOW_UNPINNED | COMPLETE | Correct asymmetry |
| RT-11 | Env interpolation + env_file | compose envresolver.go + loader.go | `service.go:220` $$/${VAR:-default} ported; env_file/include NOT handled | BROKEN | Secrets dropped silently |
| RT-12 | Service deps ordering | compose dependencies.go:78 InDependencyOrder | Parser `parser.go:566` normalized; runtime delegates to `docker compose up -d` healthcheck | PARTIAL | Compose handles ordering |
| RT-13 | Restart policies | compose create.go:592 getRestartPolicy | Forge warning but allow; beacon not validating | PARTIAL | — |
| RT-14 | Resources/isolation tiers | compose create.go:641 limits | `docker.go:783` buildResources stricter for game runtime vs compose pass-through; ceilings `lifecycle.go:139` MaxUserStack | PARTIAL | Two tiers correct |
| RT-15 | Mount confinement | 1Panel unrestricted (root agent) | `mounts.go:13` runtimeMounts EvalSymlinks+Rel within allowedMounts; compose `validateComposeVolumes:205` deny-list | COMPLETE+ | — |
| RT-16 | Multi-runtime dispatch honesty | Portainer ClientFactory per endpoint | Control 7 names vs beacon 5; create drops Provider | FALSE_COMPLETION | See also Orchestration R-01 |
| RT-17 | Traces (5 previously BROKEN) | — | **Updated:** T2 create FIXED, T4 volume prune FIXED, T5 compose deploy healthy but restart `Stop+Start` not native ComposeRestart (WIRED_BUT_WRONG), T6 exec intentional air-gap, T7 stats healthy | 5/7 FIXED since Phase 1 | — |

**Runtime verdict:** Phase 1's 4 P1 BROKEN chains (admin create, prune crossed wires, missing restart) are **3 of 4 FIXED** on re-verification — notable progress. Remaining P1s are spec-fidelity (env_file, privileged port `["80"]` bypass, resource volume ignored) and phantom runtime.

---

## 6. Single-Host Administration (18 rows) — What 1Panel Has vs What Forge Should Borrow

| # | Capability | Reference file:line | Forge | Status | Recommendation |
|---|---|---|---|---|---|
| HA-01 | Docker daemon.json editing | `1panel/.../docker.go:19` LoadDaemonJson/Update | Forge: no host docker options | MISSING | REJECT at control plane — immutable provisioning |
| HA-02 | SSH host key/mTLS | `ssh.go:17` | Forge mTLS per-node `mtls_certificates` | MISSING | REJECT — keep Forge mTLS |
| HA-03 | Fail2ban jail config | `fail2ban.go:16` | None | MISSING | REJECT — host appliance |
| HA-04 | FTP user provisioning | `ftp.go:16` | None | MISSING | REJECT — host appliance |
| HA-05 | Snapshot (panel tar+rotation) | `snapshot.go:12` | Forge S3 artifact engine | MISSING | REJECT — different durability model |
| HA-06 | File manager (allowlist+openat2) | `file.go:400` uid-permissive | `rootfs_linux.go:47` RESOLVE_BENEATH+NO_MAGICLINKS vs `secure_files.go:60`+`hostfiles.go:109` atomic | COMPLETE+ | Forge strictly stronger |
| HA-07 | Recycle bin / favorites / history | `recycle_bin.go`, `favorite.go`, `file_history.go` | Not separately modeled | PARTIAL | UX convenience, low priority |
| HA-08 | Cronjob (host cron) | `cronjob.go` | `cronjob/service.go` admin crons `requireRole("admin")` | PARTIAL | 1Panel per-host, Forge panel-level — different scopes |
| HA-09 | Host monitoring (CPU/mem/disk/GPU/load/process alive threshold) | `monitor.go`, `device.go:56`, `gpu.go`, `process.go` | `beacon/sysinfo_linux.go:94` + `store_heartbeat` + `observability` | PARTIAL | GPU visible only via capabilities delta, not page |
| HA-10 | Container/image/registry ops at host scope | `container.go:224` 8 ops, `image_repo.go` | Matched via docker admin handlers (now fixed) | COMPLETE | — |
| HA-11 | Network/firewall source CIDR strictness | `firewall.go` | `beacon/handlers_firewall.go:129` strict CIDR | COMPLETE+ | Beacon correct; API validation weaker |
| HA-12 | Upload buffering before openat2 check | — | `handlers_files.go:386` rawBody unbounded before secure_files check | BROKEN | Cap upload early |

**Host admin verdict:** Forge correctly refuses host-tool sprawl — 6 of 12 "MISSING" are intentional REJECTs. Where both operate (file manager), Forge is more hardened. Remaining defects are host-file buffering and duplicate cron micro-races.

---

## 7. Database & Template Catalog (23 rows)

| # | Capability | Reference | Forge | Status |
|---|---|---|---|---|
| DB-01 | Engine provisioning + TLS lattice | 1Panel per-engine host services, `mysql.RegisterTLSConfig` | `dbprovisioner/service.go:373` global leak/race | BROKEN |
| DB-02 | Grants/users per-host matrix | 1Panel model.Runtime:9 | `store_db_containers.go:174,200` list omits encrypted cols | BROKEN |
| DB-03 | Redis status/persistence telemetry | 1Panel redis surface | Forge missing redis conf view | PARTIAL |
| DB-04 | Pg privilege granularity | 1Panel pg roles | MISSING by design | REJECT |
| DB-05 | Language runtime (php/node/java/go/python/dotnet) | 1Panel PHPExtensions:3 | MISSING | REJECT |
| DB-06 | Host service restart (real daemon) | 1Panel restart host service | `containers.go:300` status-only no-op | BROKEN |
| DB-07 | Password generation determinism | `database_service_provisioner.go:63` vs `containers.go:44` | MISMATCH | BROKEN |
| DB-08 | Catalog isolation | 1Panel compose_template.go batch | `store_catalog.go:84` well-isolated | COMPLETE |
| DB-09 | Game templates on disk | `packages/game-templates/templates/*.json:1` 14 | Currently 0 on disk (deleted, uncommitted D), 1 seeded egg `091` | BROKEN | seed 93% deficit |
| DB-10 | Template validation (install_script) | both `template-schema.json` | `validate-templates.mjs:46-54` blind to install_script | BROKEN |
| ... | (remaining 13 rows: stored-templated dualism, app-templates-data.ts localStorage, seed job, etc.) | — | — | — |

Catalog lesson: fragmentation into DB eggs / FS game-templates / localStorage app-templates / DB app_store must collapse; game-templates should seed eggs via `DefaultSeeder` or die.

---

## 8. Networking & Gateway — Traefik/Caddy/NPM + Uncloud Mesh (18+17 rows, densest P0 cluster)

### 8.1 Reverse proxy & routing (vs Traefik one-tree / Caddy one-doc / NPM rows)

| # | Capability | Reference | Forge | Status |
|---|---|---|---|---|
| NG-01 | HTTP host/path routing authority | Traefik one dynamic tree / Caddy one JSON / NPM rows | Five writers on one Caddy (trafficmanager, loadbalancer, domains, crossnode IngressSynchronizer, proxyDomains) | BROKEN (P0) — empty-sync wipes config every 30s |
| NG-02 | Route grouping into pools | Traefik ServersLoadBalancer.Merge | `caddy_proxy.go:894-922` + `traefik_proxy.go:725-753` + `crossnode/routegroup.go:32-68` triplicated divergent | BROKEN |
| NG-03 | LB strategies (round_robin/least_conn/ip_hash/weighted) | Caddy selectionpolicies:41, Traefik wrr/p2c/hrw | Strategy→sticky mistranslation (least_conn→cookie), least_connections matches no Caddy policy | BROKEN |
| NG-04 | Weighted routing | Caddy weighted back on policy not upstream | Upstream JSON silently discards weight; lost on health re-add, WRR cross-groups | BROKEN |
| NG-05 | Health probes (HTTP path/status/interval) | Caddy healthchecks.go:73, Traefik healthcheck.go:483 | TCP dial only (2s fixed), HealthCheckConfig stored never executed | MISSING |
| NG-06 | WebSocket | Caddy transparent; NPM Upgrade headers | Both adapters set headers, correct | COMPLETE |
| NG-07 | TCP/UDP L4 | Traefik tcp:/udp: sections, NPM stream.conf | Caddy renders tcp as HTTP, UDP rejected yet rendered under tcp: — never matches | BROKEN (P0) |
| NG-08 | Validate-before-load & rollback | Caddy Cache-Control must-revalidate dry-run; NPM configure→test→reload | validateConfig POSTs /load (an apply!) then snapshots — wrong generation restore | BROKEN (P0) |
| NG-09 | Policy middlewares (rate_limit/circuit_breaker/ip) | Traefik named middlewares; Caddy CB namespace | `"handler":"rate_limit"` fictional for Caddy → must-revalidate fails, bricks updates | BROKEN (P0) |
| NG-10 | All-policies-on-all-routes | Traefik per-router named refs; NPM per-host | No rule↔policy join anywhere — one tenant blocks another | BROKEN (P0) |

### 8.2 TLS / ACME (vs certmagic/lego)

| # | Capability | Reference | Forge | Status |
|---|---|---|---|---|
| NG-11 | ACME client library & issuance | Traefik lego v5 EAB/key-type/chain; Caddy acmez | lego v4 hardcodes RSA2048, no EAB/chain, no timeouts; registration URI not persisted | PARTIAL |
| NG-12 | HTTP-01 solver | Traefik challenge_http.go:33 | `HTTPSolver()` never mounted — default challenge type guaranteed timeout | BROKEN (P0) |
| NG-13 | DNS-01 propagation tuning | Caddy acmeissuer:218 TTL/Delay/Resolvers | Fixed `1.1.1.1/8.8.8.8` only, no per-request resolvers/delay | PARTIAL |
| NG-14 | Wildcard FQDN/dup-SAN pruning | Traefik sanitizeDomains | Forge no `*.*` reject, no UnFqdn, no dedup of covered subdomains | PARTIAL |
| NG-15 | Renewal window | Caddy 10m scan, 1/3 lifetime; Traefik lifetime-scaled | Fixed 30-day, fixed 24h ticker, no jitter; panic kills loop permanently | BROKEN |
| NG-16 | Key reuse on renewal | Caddy generates NEW key per cert | Forge reuses stored PrivateKey forever | BROKEN |
| NG-17 | Revocation | Caddy ACME revoke; NPM certbot revoke | `RevokeCertificate` DB delete only, CA never informed — name overpromises | BROKEN |
| NG-18 | Cert storage encryption | Traefik 0600 file; Caddy pluggable StorageConverter | Leaf+ACME keys encrypted, DNS tokens (`dns_credentials`, `dns_provider_accounts.credentials`) plaintext JSONB | BROKEN (P1) |
| NG-19 | Default contact | NPM hard-blocks without user email | `admin@localhost` rejected by CA | BROKEN |
| NG-20 | Cert delivery to gateway | Traefik dynamic-config channel hot-reload; Caddy certmagic cache swap | `SetCertificate` zero callers — issued certs never serve TLS | BROKEN (P0) |

### 8.3 Discovery / Config / UX

- `validate = apply` semantics (above); Traefik `POST /api/refresh` invented (GET-only API); fingerprint dedupe on per-node Caddy but membership-blind for Uncloud DNS/Caddy.
- admin pages: traffic posts wrong schema (posts targetGroup vs domain), cert upload 405, security hardcoded pills + dead `/admin/domains/:id` link — **entire networking admin UX non-functional** (`phase-04/synthesis`).
- Firewall ephemeral pass-through, no desired-state; servicediscovery complete API, zero UI; reachability dials from panel not source node (wrong vantage).

**Networking overall:** The single most broken domain. Every policy-enabled route update fails validation; empty sync wipes domains every 30s; the default issuance path fails; certs never serve. Must be fixed as a single-writer gateway inversion, not per-handler patches.

---

## 9. Backup & Storage — Kopia/Restic/Longhorn (20 rows)

| # | Capability | Reference | Forge | Status |
|---|---|---|---|---|
| BK-01 | Repo/pack model | Kopia repo/blob/Storage, content_manager WriteManager vs Restic PackFile/chunker | Artifact zip+m_sidecar `beacon/local.go:238` + `services/backup/storage.go:15` whole-object | MISSING (intentionally — artifact model ok for <2GB) |
| BK-02 | Chunked dedup | Kopia CDC, Restic Rabin | Whole-archive only | MISSING (intentionally) |
| BK-03 | Per-blob HKDF+GCM vs single-archive nil salt/AAD | Kopia per-blob HKDF | `encryption.go:92` nil salt + nil AAD; `local.go:238` plaintext | BROKEN (P0 transplant forgery) |
| BK-04 | Streaming encryption | Kopia throttling streamed | `encryption.go:150` + `service.go:303` io.ReadAll → OOM | BROKEN (P1) |
| BK-05 | Sidecar integrity | Restic per-blob verifyCiphertext | `local.go:757` unkeyed SHA, `verification.go:31` trusted | BROKEN (P0) |
| BK-06 | Retention OR vs AND | Kopia/Rest: union OR (keep if any rule) | `beacon/retention.go:61` explicit AND → data loss/bypass | BROKEN (P1) |
| BK-07 | Prune exclusive lock | Restic LockRepo exclusive | `store_backups.go:240` SQL-only without pg_advisory_lock | BROKEN (P1 race) |
| BK-08 | GC orphan .partial | Kopia maintenance GC roots | `local.go:275` .partial never indexed + not swept | BROKEN |
| BK-09 | Distributed lock | Restic backend lock | `local.go:90` lockNamespace in-process only → split-brain | BROKEN |
| BK-10 | Scheduling (beacon gocron vs Worker policyDue) | Kopia scheduling_policy union | `beacon/scheduler.go:15 gocron` + `worker.go:152 policyDue (NextRunAt==nil→false)` conflict | CONFLICT |
| BK-11 | Restore journal + resumability | — | `local.go:532` 3-phase prepared→live-moved→activated + recoverInterruptedRestore:931 strong; control-plane `restore.go:733` vacuous pre-snapshot | PARTIAL (beacon strong, panel weak) |
| BK-12 | Staging lifecycle O_CREATE\|O_EXCL→Rename | Kopia pack commit | Beacon correct (`local.go:275`), service.go regresses to RAM-only | DIVERGED |
| BK-13 | Progress WS end-to-end | Kopia 300ms throttle, restic message_type:status | `backupProgressWS:2155` exists BUT `server.go:1995 realtimeProxy` only `stats\|logs\|console` + `backups-view.tsx:43` poll-only | UNWIRED (P2) |
| BK-14 | StorageLocality scoring | Longhorn diskSelector enforced at schedule | `scheduler/service.go:317 ±1e10` dead vocab drift, `handlers_servers.go:860` never populates | DEAD |
| BK-15 | Verification sampling | Restic check --read-data | Not scheduled | UNWIRED |
| BK-16 | Compression per-blob + throttling + manifest | Kopia throttler:26 streamed | Present but not comparable | PARTIAL |

Plus Longhorn verdict (5.04): mounts+artifacts remain correct durability; schedule-time locality checks with one vocabulary; StorageLocality advice already covered here.

---

## 10. Orchestration & Operations — Nomad/River/Incus/NetBird/Longhorn/Rancher (19+ rows + 3 structural facts)

### 10.1 Placement / Scheduling (vs Nomad)

| Dimension | Nomad | Forge | Gap |
|---|---|---|---|
| Job/TaskGroup/Alloc vs App/Instance/Decision | Declarative Job→Alloc chain + Previous/Next alloc | App/Instance + PlacementDecision, no allocation chaining | PARTIAL — in-place update vs always-recreate |
| Reconcile loop | GenericScheduler.Process every trigger through reconciler diff | One-shot provision; no blocked-eval equivalent | MISSING blocked-eval; freed capacity only on next poll |
| Plan/apply two-phase | Scheduler Plan → leader applier optimistic concurrency | DB-reservation gating (`FOR UPDATE`) single-phase | Different mechanism, functionally equivalent at panel scale — acceptable |
| Constraint algebra | Operand rich (`=,!=,<,<=,>,>=`, regex, version, distinct_hosts) | Enum types 5 variants | PARTIAL — enum simpler but sufficient for game hosting |
| Scoring | Normalized bounded iterators (ScoreFitBinPack 0-18, affinity weights, spread boost) | Soft bonuses +1e12/-1e10 dwarf base ≤3 (`constraints.go:59`) | BROKEN — constraint count dominates fit |
| Spread | Attribute-target % per value + even-spread | Anti-co-location only, no attribute spread | MISSING |
| Affinity | Weight -100..100 merged across job/group/task | Binary existence-only | MISSING weighted affinity |
| Reschedule policy | Delay functions exponential/constant/fibonacci, penalty nodes | Instant 60s retry forever, no cap | MISSING |
| Sticky fallback | findPreferredNode try-preferred-then-fallback | Hard requiredNode fails | BROKEN semantics |
| Drain deadline | DrainStrategy Deadline/ForceDeadline, MaxParallel | Plan ledger + bounded evacuator | Partial parity |
| Leader throughput | Lazy iterator stacks + power-of-two choices | Global mutex serializes placement | ARCHITECTURE — remove mutex |

### 10.2 Durable Queue (vs River)

Verdict from both S1 and final-parity S2: **Forge does NOT need River.** Vendored fork dead (`cmd/api/river.go:13` deprecated, river_queue orphaned migration 137). Live engines `job_queue`+`operations` dual-write same projection tables — steal≠retry accounting in queue is actually *better* than River's uniform attempts (keep it).

| Finding | File:line | Severity |
|---|---|---|
| Cancel unsound (no status predicate) → regresses completed jobs, loses races | `queue/store.go:113-130` | P1 |
| Operation backoff fake (sleep in worker slot while retrying FIRST) + no available_at | `operation/service.go:283`, `store:42-45` | P1 |
| Per-replica periodic scheduler duplicates (RFC3339Nano keys diverge per replica) | `queue/periodic.go:142`, `queue/leader.go:23` elector unwired | P1 |
| Non-tx multi-statement transitions tear job_queue/operations/attempts | `store.go:69-149` | P1 |
| Idempotency namespace fragmentation (`forge-op:` vs `forge-job:`) silent dedup | `queue.go:238` vs `operation/service.go:332` | P1 |
| Heartbeat dies with job context on Stop() | `queue.go:198,213` vs correct `context.WithoutCancel` on sibling | P1 |

**Single-writer contract:** `queue.Service` sole execution writer (`job_queue`); `operation.Service` read-model+reaper.

### 10.3 Runtime Honesty & Clustering

- **LXC/KVM phantom providers** — control 7 names vs beacon 5; `beacon/server.go:740` drops Provider field → always `"mode":"docker":841` — silent lie across trust boundary.
- **Firecracker unsafe:** same RW rootfs per VM, no networking, hardcoded install exit 0, fabricated stats.
- **Capability system dead** — `CheckCapability` zero callers, `Snapshots:true` claimed nowhere implemented.
- **No overlay option:** Node no TunnelIP; gateway resolves public hostnames only; servicediscovery write-only + 3m reaper (unhealthy after registration).
- **Event pipeline write-only:** `Relay.Subscribe` zero production callers; `EventRelay` unused; no PublishTx — publish after commit, durability not guaranteed.
- **No leader election:** ~30 daemons/instance with only jitter; unwired elector sits in dead River fork.
- **Fencing twice, wrong edge:** `FenceNode` fires on `EventNodeRecovered` (24h lease) vs recovery 1h lease, zero enforcement in beacon, grep zero generation in beacon.

### 10.4 Storage (vs Longhorn/Rancher)

StorageLocality scoring dead + vocab drift, volumes orphan FK stub, mount eligibility after placement not schedule-time, evacuator defaults local-data to replicated→AutoReplace (dangerous with failover default-Evacuate), Rancher gateway wiring dead code.

---

## 11. Security / Tenancy / RBAC / Secrets / Network Boundaries (30 rows)

| # | Area | Reference truth | Forge | Status |
|---|---|---|---|---|
| SE-01 | Permission catalog + wildcard * | `GetUserPermissionsService.php:18` * owner-only computed | `store_users.go:544` * persistable by any `user.create` holder | BROKEN (P0 escalation) |
| SE-02 | Subset enforcement (only grant what you have) | Pterodactyl intersect+websocket injection | Missing — any `user.create` → arbitrary or * | BROKEN (P0) — same root as GH-18 |
| SE-03 | Tenancy scope (org/project/env) | Portainer ResourceControl per-endpoint | Org tables exist (`store_tenancy.go:91`), but `domain.PlacementRequest:114` no tenant field, events tenantless `events/event.go:196` | PARTIAL (P1) — tenant-blind scheduling/audit |
| SE-04 | Mount allowlist | Pterodactyl model validates absolute + not `/` | `store_mounts_ext.go:323` only blocks 2 sources, misses /etc/shadow parent, /var/run/docker.sock, /proc, / | BROKEN |
| SE-05 | Cron shell boundary | Wings never runs control-plane shell for user cron; `power/command/backup` in container | `cronjob/service.go:240` dispatchServerCommand nil→error (fail-closed) correctly | COMPLETE+ |
| SE-06 | DB identifier quoting | `DatabaseManagementService.php:106` prepared identifiers | `dbprovisioner/service.go:737` double-quote quoting correct | COMPLETE |
| SE-07 | Trusted proxy model | Caddy explicit CIDR allowlist `server.go:1171` | `ExtractClientIP:98` trusts XFF from any private peer — any container rotates identity | BROKEN |
| SE-08 | CSP nonce entropy | Traefik verbatim, constant not shipped | Both middlewares fall back to `"fallback-nonce"` with strict-dynamic if rand fails | BROKEN |
| SE-09 | Policy attachment model | Per-router named refs | All-policies-on-all-routes (`traefik_proxy.go:960`) | BROKEN (P0 cross-tenant) — same as NG-10 |
| SE-10 | L4 LB probe primitive | Traefik strict CIDR | `handlers_loadbalancer.go:88` accepts arbitrary IP, dials internal networks | BROKEN |
| SE-11 | Keyring AAD | Portainer libcrypto per-value nonce | `keyring.go:75` AAD caller-dependent, no namespace validation | PARTIAL |
| SE-12 | Session pointer race | Portainer hashes tokens | `auth/session.go:36` stores raw pointer + raw token key, concurrent LastActiveAt race | BROKEN |
| SE-13 | mTLS HTTPS gate | Caddy determineTrustedProxy | Trusts client X-Forwarded-Proto (`middleware_mtls.go:123`) | BROKEN |
| SE-14 | WS origin for Bearer auth | Validate for all upgrades | Missing Origin for !isCookieAuth (`ws_origin.go:95`) | BROKEN |
| SE-15 | Audit logging | NPM writes user_id/action/meta every mutation | Traffic/cert/LB/firewall silent | MISSING |
| SE-16 | Provider registry honesty | Incus strict driver check, Longhorn driver registry | 7 names vs 5 impls vs 1 honest | FALSE_COMPLETION |

### UX / IA (14 rows)

| # | Area | Reference truth | Forge | Status |
|---|---|---|---|---|
| UX-01 | Navigation IA (5 registry groups → 6 goal groups) | Coolify resource-centric 3-depth + global search | `admin-registry.ts:22` 5 groups ~62 entries → `admin-shell.tsx:42` 6 goal groups ~50/62 (Kubernetes/Containers now fixed but Recovery→Migrations duplicate previously hiding) | PARTIAL (P2) — Advanced largest (27) no second collapse |
| UX-02 | Dashboards split | Coolify workload inventory vs 1Panel mixed home | `AdminOverview.tsx:86` inventory/capacity + `monitoring/page.tsx:1` live metrics — correct split | COMPLETE |
| UX-03 | App detail tabs | Coolify path-based tabs `/environment/:id/applications/:id/config` | `apps/[id]/page.tsx:28` useState not routed → refresh loses tab, no deep links | BROKEN |
| UX-04 | Deployment experience (dual pollers) | Dokploy single 1s poll, Coolify configuration-diff | Two pollers 2s (`deployment-progress.tsx:46`) vs 5s (`DeploymentTimeline:27`) same endpoint — never dedup, leak risk | BROKEN (duplicate) |
| UX-05 | Status tone maps per-file | Dokploy single badgeStateColor | `admin-ui.tsx:14` + per-file tones diverge (compose 9 states) | PARTIAL |
| UX-06 | Empty/loading/error states | Dokploy per-resource EmptyState | `states-empty.tsx:17` 10 semantic empties wired; `states-error.tsx:43` rich but RateLimit never consumed, OfflineBanner duplicated | PARTIAL |
| UX-07 | Logs placement (modal vs inline) | Pterodactyl per-resource logs+terminal colocated | DeploymentLogViewer WS lives in modal, not under DeploymentTimeline | PARTIAL |
| UX-08 | Domains gated behind serverFilter | Dokploy columns + dns-helper-modal | `admin/domains/page.tsx:57` enabled !!serverFilter — empty by default, no aggregate view | BROKEN |
| UX-09 | Triple env editors | Coolify single show view with flags (multiline/literal/buildtime/runtime) | Three incompatible editors (admin EnvVarEditor, AdminAppsShared Record, EnvironmentEditor encrypted) + overloaded API | BROKEN |
| UX-10 | Tenancy cascade display-only | Coolify team→project→environment scopes lists | `fetchApps` global not filtered; protected flag show-only no enforcement | BROKEN |
| UX-11 | Docker view node-blind | Dokploy server selector | `docker/page.tsx:17` no node filter, flattens without grouping | PARTIAL |
| UX-12 | Gateway vs 7 scattered pages | Traefik one HTTPConfiguration, NPM one hosts page | IA proposal: collapse traffic+load-balancer+domains+certificates+security+firewall+endpoints → Gateways (Routers/Services/Middlewares/Certs) + Operations timeline | — IA debt, not code defect |

---

## 12. Status Roll-up — What Competitors Have That Forge…

### 12.1 COMPLETE or Parity+ (keep and strengthen) — ~50 rows

- Game desired/actual + generation fencing concept; orphan remediation; SFTP quota/rate-limit + openat2 RESOLVE_BENEATH confinement (1Panel file.go:400 uid-permissive is weaker); host allowlist pinned vs NPM cloud ranges; traffic direction via migration service not duplicate transfer logic; replica correlation IDs; heartbeat 6-state classifier richer than all six orchestration refs; scheduler's reservation row-locks; lease-outbox primitive structurally River-quality; steal≠retry accounting better than River; compose GitOps controller; Docker admin now actually handles create/network/volume/prune (post-verification fixes from Phase 1).

### 12.2 PARTIAL (fix within existing code) — ~40 rows

- Revisions (git vs placement disjoint); env interpolation missing `env_file/include`; resource limit tiers not unified; spread/affinity/reschedule policies not on records; DNS probe tuning surface; wildcard `sanitizeDomains` pruning; renewal jitter/stagger; upload buffering before secure_files check; language runtimes correctly absent but flagged as divergence not debt.

### 12.3 UNWIRED (exists, invisible) — ~32 rows

- Installation 6-step workflow engine (`installer/service.go:65` 6 workflows persist rows never executed); backupProgressWS beacon publishes panel never proxies; HTTPSolver fully implemented never mounted; previewenv TTL/reaper/commit-status; servicediscovery REST + PrivateNetworkPolicy; capabilities delta endpoint; `security_headers` table; `river_queue` migrated orphaned; StorageLocality scoring; `server_orphan_remediations` tracking; commit status checks only in unused previewenv; `Pipeline` queue+schedule with no HTTP/UI; pre-restore-*.zip hidden on FS.

### 12.4 BROKEN / FALSE_COMPLETION (logic defects, ~25 rows, highest priority)

- Deployment execution stubs report success (`execution.go:249`); health gate probes `localhost` clamped; DoS via infinite trusted-proxy identity rotation; fictional Caddy modules brick updates when any policy enabled; Rule injection via backticks (`Path` into Traefik rule expression); DNS-provider SSRF via unvalidated PDNS_API_URL; plaintext DNS zone tokens while siblings encrypted; L4 LB as internal-network probe; Traefik `POST /api/refresh` invented (GET-only API) so writes always 404+rollback; inventory disparate: appstore swallowing interpolation, compose `["80"]` privileged bypass, `env_file` dropped silently, volume uniform allowlist divergence.

### 12.5 MISSING (genuine — small adapters, not new subsystems, ~15 rows)

- Overlay mesh option (Node schema no `TunnelIP`/`MeshPubKey`, public-only resolution).
- Egg `config.files` patcher (6 parsers, `configMatchRegex`).
- HTTP-01 mount + TLS-ALPN provider.
- Typed install pipeline (PufferPanel 24 typed ops with `if:`/`groups`/`supportedEnvironments`) for modded Minecraft.
- Per-blob HKDF+GCM + per-write nonce backup envelope.
- Attribute-spread + reschedule-policy iterators in placement (move attempts/backoff onto records).
- Distributed backup namespace locks (Forge lockNamespace in-process only).
- Blocked-eval equivalent + explicit middleware-attachment join + distributed health enforcement primitive.
- Per-file backup progress transport (data exists at beacon, transport missing — see UNWIRED).

### 12.6 DUPLICATE (one must win)

1. `queue.Service` vs `operation.Service` dual-writing `operations` tables — verdict: `queue wins execution`, `operation becomes read-model`.
2. Dead vendored River fork + orphaned `river_queue` table.
3. `preview` vs `previewenv`.
4. Three EnvVar editors + two APIs.
5. Three template systems (DB eggs / FS game-templates / localStorage app-templates) — 1 seeded egg vs 14 on disk `egg-templates.ts:27` vs 44 PufferPanel templates.
6. Three runtime abstractions (`beacon/internal/runtime`, `forge/api/internal/runtime`, `services/runtime`).
7. Five gateway writers + four adapter abstractions (GatewayAdapter dead, Traefik 1,139 lines doubly broken).
8. Four retention/verification paths in backup (AND vs OR vs union).
9. Two security-header middleware trees kept in sync manually.
10. Fencing implemented twice (24h vs 1h leases) firing on wrong edge + zero enforcement.

### 12.7 DEAD

River fork, `river_queue`, `CaddyTLSManager`, legacy `servers.transfer_state` shim (but still drives ensureTransferIdle), `volumes` FK-stub table, `security_headers` consumers, legacy `/compose/projects/*`.

---

## 13. Security Findings (P0/P1 consolidated)

| ID | Title | File:line | Severity |
|---|---|---|---|
| GH-18/SE-01/02 | Subuser wildcard `*` persistable + subset-check missing (any `user.create` → full control + `database.view_password`) | `store_users.go:297`/`normalizeSubuserPermissions:563` | P0 |
| RT-17/R-01/SE-16 | LXC/KVM phantom runtime — beacon drops Provider → always Docker, capability system dead | `beacon/server.go:740-772,841` + `factory.go:19-31` | P0 (phantom) |
| GH-19/SE-04 | Mount allowlist too narrow — only blocks `/etc/forge`, misses `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/` | `store_mounts_ext.go:323` | P0 (host breakout) |
| SE-07 | Trusted proxy fail-open — `ExtractClientIP:98` trusts XFF from any private peer (Caddy requires CIDR) + XFP in mTLS gate `:123` | `middleware_ratelimit.go:98`+`middleware_mtls.go:117` | P1 |
| NG-18/SE-18 | Plaintext DNS API credentials in `dns_credentials`/`dns_provider_accounts.credentials` | migrations `134/135` vs encrypted `157` | P1 |
| NG-09/SE-09 | All traffic policies leak to all routes — cross-tenant ACL | `traefik_proxy.go:960` + `caddy_proxy.go:722` | P0 |
| NG-14/SE-14 | Traefik rule injection via unescaped backticks in `Path` | `traefik_proxy.go:797` vs `service.go:376` | P1 |
| NG-16/SE-16 | DNS-provider SSRF (`PDNS_API_URL` zero validation + process-global env mutation) | `dns/service.go:489-498,639-661` | P1 |
| NG-10/SE-10 | L4 LB as internal-network probe primitive (arbitrary IP accepted) | `handlers_loadbalancer.go:88` | P1 |
| BK-03/SE-BK | Backup sidecar unauthenticated + nil AAD → cross-server transplant | `beacon/local.go:757` + `encryption.go:92:119` | P0 |
| BK-07/SE-07 | CSP nonce fallback `fallback-nonce` with strict-dynamic | `middleware_security.go:22` + `middleware_security_headers.go:40` | P2 |
| SE-12 | Session store pointer race + raw token keys + raw byToken map | `auth/session.go:36-98` | P1 |
| SE-11 | Keyring AAD caller-dependent no namespace validation | `keyring.go:75` | P2 |
| SE-14 | WS origin missing for Bearer auth | `ws_origin.go:95` | P2 |

---

## 14. Reliability Findings (P0/P1)

- Operation reaper duplicate-execution (5m threshold vs 10–60m installs `operation/service.go:164`), lease-steal double retry-count (operation path), heartbeat history truncation (`RecoveryThreshold+3=5` stalling `Recovering`), beacon reconnect declaring connected without probe (`reconnect.go:146` StateConnected without SendNodeHeartbeat), non-tx multi-statement tear (`store.go:69-149` 4 statements), per-replica periodic duplication (nano timestamp keys `periodic.go:142` with no leader), blocking advisory migration lock (`store.go:45` vs try-lock), auto-renewal panic kills loop permanently (`acme/service.go:389`), unhealthy-upstream oscillation (probe removes, rebuild re-adds), LB HealthCheckConfig ignored + single-failure flip-flop, firewall ephemeral pass-through lost on node reprovision, synthetic-zero monitoring masking gaps.

---

## 15. Architecture Findings (cross-phase capstone)

- **Write-only durability (AF-1):** durable outbox/Relay has zero production subscribers (`eventstore/outbox.go:39`, `http/server.go:148` unused), no PublishTx — cross-instance automation impossible.
- **No leader election (AF-2):** ~30 `Start()` daemons/instance with jitter as only coordination; unwired elector in dead River fork.
- **Fencing twice, wrong edge, zero enforcement (AF-3):** `FenceNode` on `EventNodeRecovered` (24h lease) vs recovery 1h lease, zero occurrences of `generation`/`fence` in beacon/power paths; NetBird fence-via-NetworkMap is the correct model.
- **Six controllers, one server, no serialization:** reconciler/health-recovery/replicamanager/crash-detector/failover/recovery all race on same server via separate leases.
- **Tenant-blind core paths:** placement `domain.PlacementRequest:114`, events `events/event.go:196` tenantless, plans tenantless — any misbehaving tenant influences fleet-wide decisions.
- **Config sprawl + dangerous default:** 4 config regimes + `matchingPolicy==nil→Evacuate` when Longhorn defaults `do-nothing`.
- **Phase 6 addition:** validation split (API vs beacon allowlists) producing `200 valid→400 violation` for same compose payload.

---

## 16. UX / Discoverability Findings

- Entire networking admin UX non-functional against real schemas (traffic posts wrong schema, cert upload 405, security hardcoded pills + 404).
- Triple env-var editor incoherence (`env-var-editor.tsx:8` vs `AdminAppsShared:120` vs `EnvironmentEditor.tsx:37`).
- Tenancy cascade display-only (`fetchApps` global not filtered), duplicates deployment polling (2s vs 5s), synthetic timestamps & cumulative network graph, offline banners duplicated, empty CTAs dead, tone maps per-file.
- New in final verification: **3 of 4 Phase-1 admin create/prune 404s now FIXED** (container/network/volume create now mounted at `container_admin.go:1776/1824/1896`+`server.go:478-481`, volume prune `1961`+`server.go:483`), but compose `["80"]` bypass, env_file drop, app create discards (hardcodes image), Adv group 27 entries no collapse, docker view node-blind remain.

---

## 17. Best Patterns to Adopt vs Reject

| Subsystem | Winner | Pattern | Forge action |
|---|---|---|---|
| Gateway model | Traefik | provider→router→middleware(named ref)→service tree | Single-writer gateway behind one reconciler |
| True dry-run | Caddy /adapt vs /load | validate must not apply | Live before snapshot |
| Apply discipline | NPM configure→test→reload w/ .err rename | per-host failure isolation + offline meta | — |
| Repo integrity | Restic index+prune / Kopia maintenance | content-addressed verify, exclusive-lock prune | — |
| Progress cadence | Restic `message_type:status` | throttled bytes_done/total_bytes | — |
| Job dedupe signal | River UniqueSkippedAsDuplicate | distinguish insert vs dedupe | — |
| Leader gating | Nomad / River TTL row | gate maintenance daemons | — |
| Reschedule policy | Nomad delay+penalty on record | attempts/backoff on instance record | — |
| Profile composition | Incus ordered profiles last-wins | eggs become composable layers | — |
| Enforcement=mechanism | NetBird NetworkMap push | fence via distribution | — |
| Access binding | Portainer ResourceControl | per-resource subset-checked grants | — |

**Reject:** Uncloud CRDT-no-quorum, Rancher fleet/K8s, Longhorn block replication, monolithic Portainer CE, Nginx string-template gen, 1Panel verbatim-YAML single-host assumptions, host-tool sprawl, Komodo token-in-URL.

---

## 18. Forge Advantages to Keep

Game+app on one scheduler; desired/actual+transitions sup; hysteresis heartbeat (6-state, best-in-corpus); lease-outbox primitive; steal≠retry accounting; destructive-confirm gates; resumable chunked v1 transfers; openat2 confinement; correlation IDs; org→project→environment tenancy; executor reuse in recovery — each ahead of its closest reference.

---

## 19. What Is Genuinely Missing vs Activation Scope

- **Genuinely missing (≈8-10%):** overlay mesh (`TunnelIP`/`MeshPubKey`), egg `config.files` patcher, HTTP-01 mount+TLS-ALPN, typed install pipeline for modded Minecraft, per-blob HKDF+GCM, attribute-spread/reschedule iterators, distributed backup locks, blocked-eval equivalent, per-file backup progress transport.
- **Actively hidden but built (≈18% unwired):** see §12.3 — installer 6-workflows, backup WS, previewenv, servicediscovery REST, capabilities delta, security_headers table, gateway cert paths.
- **Partial fixable in place (≈22%):** §12.2.

---

## 20. Activation Roadmap — The 50 Highest-Value Existing Capabilities to Wire Before Building Anything New

Ordered by dependency (no new subsystem — activation within existing tables/services). Every item reuses tables/handlers already present.

| # | Activation | Where | Effort | Value |
|---|---|---|---|---|
| 1 | Unify compose volume policy predicate (single allowlist) — end `200→400` trap | `service.go:594`/`compose.go:226` + `mounts.go:64` | S | P0 |
| 2 | Fix variable validator slash bug + handle \| inside class — unblock all PTDL imports | `store_egg_variables.go:176` | S | P0 |
| 3 | Enforce subuser subset: load actor perms, reject any req not in actor set; gate `*` behind owner/admin | `store_users.go:297`/`handlers_servers.go:541` | S | P0 |
| 4 | Wire `restoring_backup` as server lock (Set actual_state on restore, clear on done; guard power/install) | `handlers_servers.go:2104` + `ensureTransferIdle` | S | P1 |
| 5 | Close mount allowlist (allowlist prefix `/srv/forge-mounts`, deny `/etc/…/var/run/docker.sock//proc`) | `store_mounts_ext.go:323` | S | P0 |
| 6 | Fix appstore `resolveTemplate` to fail-fast on `${VAR:?}` instead of raw template | `appstore/service.go:239` | S | P1 |
| 7 | Fix `UninstallApp` to delete DB only after daemon success (confirm-then-delete) | `appstore/service.go:139` | S | P1 |
| 8 | Make `UpgradeApp` respect `app_ignore_upgrade` + CrossVersion | `appstore/service.go:158` | S | P0 |
| 9 | Delete verify stub (hardcoded true) or implement real DNS+HTTP proof | `handlers_proxy_domains.go:199` → `domains/service.go:357` | S | P1 |
| 10 | Scope DNS `os.Setenv` mutation (struct config, stop env-mutating) | `dns/service.go:637` | M | P1 |
| 11 | Fix renewOnce fullchain (PEM not leaf DER) | `acme/service.go:492` | S | P1 |
| 12 | List DB containers with encrypted columns (fleet view empty creds) | `store_db_containers.go:174` | S | P1 |
| 13 | Fix mysql RegisterTLSConfig per-dial (no global registry) | `dbprovisioner/service.go:373` | S | P1 |
| 14 | Fix `validateVariableValue` — gate internal vars at creation for non-admin | `store_servers.go:273` | S | P1 |
| 15 | Implement or remove egg `config.files` patching (6 parsers) — Minecraft wrong port until fixed | `store_servers_control.go:95` + beacon pre-start | M | P1 |
| 16 | Seed `game-templates` (14 → DB seeding job like SeedDefaultApps) + reconcile `egg-templates.ts:27` 14 vs DB | `091_seed` + new seeder | S | P2 |
| 17 | Wire gateway cert delivery: call `SetCertificate` after issue/renew; construct TLS manager | `gateway_adapter.go:49` + after `acme/service.go:355` + TLS manager | M | P0 |
| 18 | Mount `HTTPSolver` on :80 (or proxy `/.well-known/acme-challenge`) | `acme/service.go:157` handler | S | P0 |
| 19 | Fix validate=apply snapshot inversion (snapshot BEFORE validate, use /adapt dry-run) + single writer | `caddy_proxy.go:673-720,980-1002` | M | P0 |
| 20 | Stop empty ingress sync (skip when no rules); unify five writers behind one reconciler | `ingress_sync.go:111`+ `main.go:994` | S | P0 |
| 21 | Remove fictional Caddy handlers `rate_limit`/`circuit_breaker` until real modules or map to reverse_proxy CB namespace + weight→lb_policy | `caddy_proxy.go:795-875` | S | P0 |
| 22 | Add rule↔policy join; filter policies per route; deterministic order | `traefik_proxy.go:960`+`routegroup.go:149`+`caddy_proxy.go:722` | M | P0 |
| 23 | Fix grouped-route withdrawal (rebuild-minus-withdrawn group-aware) | `caddy_proxy.go:60-74` | M | P1 |
| 24 | Fix TCP→HTTP mis-render (protocol branch or reject) | `caddy_proxy.go:924` + `traefik_proxy.go:758` | M | P1 |
| 25 | Encrypt DNS credentials (mirror migration 157) | `certificates.dns_credentials` | M | P1 |
| 26 | Fix `ShortFormHostPort ["80"]` privileged-port bypass | `beacon/compose.go:213-224` | S | P1 |
| 27 | Fail-fast on `env_file` present (or mount/resolve) — stop silent drop | `compose/service.go:14/92` | S | P1 |
| 28 | Fix `DeployFromGit` Update-on-fresh-ID (use Create when no row) | `gitops.go:381-387` | S | P1 |
| 29 | Fix ScheduleTask rerun deduping — isolated verify of dedup threshold | `schedule_runner.go:139` dedup window | S | P2 |
| 30 | Fix deployment execution stubs + health gate target (probe via beacon not localhost) | `execution.go:249` + `healthgate.go:11` | M | P0 |
| 31 | Fix compose policy allowlist divergence (API warnings vs beacon hard reject with no allowlist recourse) | `checkVolumesSecurity:594` vs `validateComposeVolumes:226` | M | P1 |
| 32 | Cap upload early (rawBody OOM before secure_files) | `handlers_files.go:386` vs `secure_files.go:269` | S | P1 |
| 33 | Cap upload early (rawBody OOM before secure_files) | (duplicate consolidation — keep once) | — | — |
| 34 | Make event loop real (PublishTx + relay consumers for fencing/failover/tm/lb) | `eventstore/store.go:214`+`outbox.go:39` | M | P0 |
| 35 | Elect one leader (advisory-lock/TTL elector) gating maintenance daemons | `queue/leader.go:23`+`main.go:521` | M | P0 |
| 36 | Default Notify (remove Evacuate fallback when no policy) | `failover/service.go:181-211` + locality default | S | P0 |
| 37 | Enforce fence (single fencing impl, CAS generation on power/dispatch, beacon generation check) | `fencing.go:20` + `recovery/service.go:642` | M | P1 |
| 38 | Queue consolidation (CAS cancel, delay column not sleep, tx-wrapped transitions, unified idempotency, background heartbeat, leader-gated periodic, drop goroutine fallback) | `queue/store.go:113` etc | M | P1 |
| 39 | Provider honesty (delete/quarantine phantom lxc/kvm; capability-gate scheduling) | `server.go:740` Provider field + `capabilities.go` | S | P0 |
| 40 | Tenant-tag core records (PlacementRequest/Decision, plans, Envelope) | `domain.PlacementRequest:114` | S | P1 |
| 41 | Wire `backupProgressWS` end-to-end (add `backup` to `server.go:1995` union + `lib/api.ts:933` union; TotalBytes `Stat` early) | beacon `backupProgressWS:2155` | S | P1 |
| 42 | Fix encryption streaming + salt (StreamWriter, per-backup salt, AAD=server:backup_name) | `encryption.go:150` + sidecar rehash | M | P0 |
| 43 | Unify retention to union OR + prune inside advisory lock incl. S3 sweep | `retention.go:61` + `store_backups.go:240` | M | P1 |
| 44 | Rebuild networking admin UX against real schemas (schemas, GET /policies list, PATCH verb, cert upload path) | `app/admin/traffic/page.tsx:14` etc | M | P1 |
| 45 | Persist firewall desired state with reconciliation + payload validation | `handlers_firewall.go:78` | M | P1 |
| 46 | Wire servicediscovery self-registration (beacons carry endpoint liveness in heartbeats) | `servicediscovery/registry.go:91` | M | P1 |
| 47 | Unify client-IP derivation (logs vs limits) + real adapter health probe + CIDR trusted proxies | `middleware_ratelimit.go:98`+`caddy_proxy.go:375` | S | P1 |
| 48 | Fix synthetic-zero monitoring (render `no data` not 0) + daemon uptime with suspend subtraction | `monitoring/page.tsx:50`+`handlers_host.go:65` | S | P2 |
| 49 | Token rotation: per-write fresh leaf key (not stored PrivateKey reuse) + persist account binding + preferred chain | `acme/service.go:536` | S | P1 |
| 50 | Pre-restore snapshot metadata narrow / journal recovery on startup (persist RestoreOptions, rehash on tamper) | `local.go:757` + `RecoverRestoreJournals` | S | P1 |

Items 33/29 collapsed — remaining 50 are distinct table/service/WS fixes reusing existing code; none requires a new subsystem. Prioritized: P0 first (gw wipes, empty sync, fictional handlers, verify stub, subuser `*`, mount allowlist, deployment stubs, health localhost, cert delivery, HTTP-01 mount, retention inversion, sidecar transplant, phantom providers); then P1 consolidation; then UX activations.

---

## 21. How Much Is Genuinely Missing vs Already Built but Disconnected?

Approx decomposition of ~180 capability rows audited here (full phase 1–6 synthesis + final-parity re-verification):

| Bucket | Share | Example |
|---|---|---|
| **COMPLETE / parity+** | 28% | game desired/actual, SFTP quota+openat2, heartbeat 6-state, placement FOR UPDATE, compose GitOps, transfer v1 resumable |
| **PARTIAL (fix in place)** | 22% | revision/CPU conflation, env interpolation missing env_file, wildcard domain pruning |
| **Implemented but UNWIRED** | 18% | installer 6-workflows, backup WS, HTTPSolver, previewenv, servicediscovery REST, capabilities delta, StorageLocality, orphan center |
| **Implemented but BROKEN** | 14% | stubs lying complete, localhost health probes, vendor deregister, certs undelivered, loyalty tests |
| **DUPLICATE (two winners, one must survive)** | 10% | queue vs operation, preview pair, three EnvVar editors, three template systems, five gateway writers |
| **Genuinely MISSING (≈ build small adapter, not rebuild platform)** | 8–10% | overlay mesh, egg config.files patcher, per-blob HKDF, typed install pipeline, spread/reschedule iterators, distributed backup locks |
| **DEAD / decorative** | ~3% | River fork, CaddyTLSManager, volumes stub, legacy shim columns |

**Therefore:** the next engineering phase is an **activation program across the ordered 50 above**, sequenced P0 wiring/bugfixes first (no new tables), not new subsystem construction. Every P0 is fixable within existing services and tables — the corpus's best patterns (§17 of the prior final report) slot directly into Forge's existing seams because Forge drew those seams correctly in the first place.

---

## 22. Files Created in This Final Task

- 10 parallel parity reports: `audits/final-parity/subagent-01-game-hosting.md` (792 lines, 19 rows, 12 LFs), `subagent-02-app-platform.md` (20 rows, 6 LFs), `subagent-03-git-build.md` (18 rows, 6 LFs), `subagent-04-runtime-compose.md` (18 rows, 8 traces, 7 LFs), `subagent-05-host-admin.md` (18 rows, 6 LFs), `subagent-06-database-templates.md` (23 rows, 6 LFs), `subagent-07-networking-gateway.md` (18 rows, P0×10), `subagent-08-backup-storage.md` (20 rows, 9 findings), `subagent-09-orchestration-queue.md` (19 rows, 6 structural), `subagent-10-security-ux.md` (30 rows)
- This file: `audits/FINAL_PARITY_AUDIT.md` — single clear-cut synthesis for executive + engineering hand-off.
- Prior final report remains at `audits/FINAL_REFERENCE_ECOSYSTEM_REPORT.md` with Phase 6 addendum.

No product code modified. Source inspected under `/Users/riyaz/project/gamepanel` per ABSOLUTE RULE.


---

## Appendix — Final-Parity Re-Verification Notes (what changed since original audits)

This appendix records where re-verification by the 10 parallel final-parity agents **changed** a prior verdict, so the matrix above is authoritative vs stale phase syntheses.

- **Runtime admin chains FIXED since Phase 1:** `POST /api/admin/containers` now `container_admin.go:1776`/`server.go:478` → `daemon/AdminContainerCreate:1883`; `POST /api/admin/networks` `1824`/`server.go:479`; `POST /api/admin/volumes` `1896`/`server.go:481` + `POST /api/admin/volumes/prune` `1961`/`server.go:483` → `handlers_docker.go:667`. Phase 1 marked these BROKEN/404; they are now FIXED (RT-02/08/09). Image prune `server.go:476` still dead at handler (`handlers_docker.go:63` never calls `daemon/AdminImagePrune:1854`).
- **Git/build cache fix:** `beacon/build.go:54-56:126-138` now forwards `CacheFrom/CacheTo`+`platform` — was BROKEN dropped (Phase 1 GB-05). Webhook HMAC now 401 (`handlers_git.go:640,705,802,869`) — was 200 mask (Phase 1 GB-10). Builder admission now 422 at `handlers_source_deployments.go:95` for unsupported types (Phase 1 GB-04 HIGH false-completion → now honestly rejected, DB CHECK still stale).
- **Compose restart mounted:** `handlers_compose.go:470` → `lifecycle.go:781` `RestartStack` now live (was missing admin route), but impl is `Stop+Start` not native `daemon/ComposeRestart:81` → `compose.go:506` `docker compose restart` (WIRED_BUT_WRONG — T8).
- **Host admin divergence clarified:** 6 of 12 "MISSING" are intentional REJECTs (daemon.json, SSH, fail2ban, FTP, snapshot host appliances). File confinement (`rootfs_linux.go:47` RESOLVE_BENEATH) is strictly stronger than 1Panel `file.go:400`.
- **Phase 6 novelties carried forward:** template seeding deficit (`egg-templates.ts:27` 14 vs 1 seeded egg vs 44 PufferPanel templates), `env_file` dropped silently, `["80"]` privileged-port bypass, `DeployFromGit` Update-on-fresh-ID — these were not in original FINAL_REFERENCE_ECOSYSTEM_REPORT.md; they are in this FINAL_PARITY_AUDIT.

No other P0 reclassified as fixed — networking's five-writers, fictive handlers, all-policies-all-routes, certs undelivered, verify stub, plus orchestration's write-only events/no-leader/fence gaps all **still BROKEN** on re-verification.
