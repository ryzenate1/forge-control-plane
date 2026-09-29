# Phase 06 Synthesis — Remaining Reference Deep-Dive (1Panel · PufferPanel · Komodo · Uncloud · Dokku/CapRover · Docker-Compose)

**Cluster:** 1Panel (dev-v2, 1703 files, single-host VPS panel) — dissected into 4 slices — plus PufferPanel daemon, PufferPanel-templates, Komodo, Uncloud, Dokku, CapRover, Docker-Compose spec, with Coolify/Dokploy build lenses where relevant. 10 subagents run **fully in parallel** per user instruction.

**Why this phase:** Phases 1–5 covered the obvious clusters but left high-value surfaces shallow or explicitly excluded:
- 1Panel's 60+ API groups (website/openresty, SSL, databases, runtime, snapshot, monitoring, AI/MCP) were touched only as a "widest admin surface" label in Phase 1 — not file:line compared.
- PufferPanel's *daemon* (environment/operations/servers, archive paths, keepalive, permissions) was explicitly declared "architecturally unrelated" and excluded from Phase 2's Wings comparison — now re-included.
- Komodo periphery (Rust agent, compose lifecycle, builds, alerts) got one-stack.rs citation in Phase 1 — not a proper agent vs beacon comparison.
- Uncloud's quorum-less mesh, WireGuard, CRDT Corrosion store, per-node Caddy/DNS, and Docker IPAM were only in clustering rationale — not audited against Forge's Postgres-central placement.
- Docker-Compose spec fidelity (dependency graphs, env_file interpolation, convergence, resource mapping) was a "spec control" — not traced end-to-end through Forge's compose service and beacon compose handler.
- CapRover/Dokku minimal-PaaS patterns (CaptainDefinition, one-click, plugn) and coolify/dokploy build specifics deserved direct appstore/catalog/build comparison.

**Method:** 10 subagents, each ≥12 comparisons + ≥3 logic findings, all file:line verified on both trees (`reference/*` vs `forge/api` + `beacon` + `forge/web` + `packages`). No product code modified. Reports read and reconciled 2026-08-23. Total: **3112 lines** across 10 reports, **~48 logic findings**.

---

## 1. What each subagent actually inspected

| SA | Focus | Key reference files | Key Forge files |
|---|---|---|---|
| 01 | 1Panel App Store & App Lifecycle | `agent/app/api/v2/app*.go`, `model/app*.go`, `service/*app*` | `services/appstore`, `catalog`, `compose`, `handlers_appstore.go` |
| 02 | 1Panel Website/SSL/OpenResty/ACME/DNS | `website*.go`, `nginx.go` (7 files) | `services/domains`, `dns`, `acme`, `handlers_domains/proxy_domains/acme_accounts/certificates` |
| 03 | 1Panel Database & Runtime | `database*.go` ×5, `runtime.go`, `php_extensions.go` | `database_service_provisioner.go`, `dbprovisioner`, `db_containers`, `managed_databases` |
| 04 | 1Panel Infra/Monitor | `container.go`, `image*.go`, `docker.go`, `cronjob.go`, `snapshot.go`, `monitor.go`, `firewall.go`, `ssh.go`, `fail2ban.go`, `host.go`, `file.go` | `beacon/handlers_host.go`, `handlers_firewall.go`, `hostfiles.go`, `secure_files.go`, `metrics`, `handlers_host/files/firewall/cronjob` |
| 05 | PufferPanel Daemon vs Beacon | `servers/server.go:185-907`, `environment`, `operations/` (24 typed ops), `spec.json` | `beacon/server.go`, `manager.go`, `runtime/docker.go`, `clustermanager`, `orchestrator` |
| 06 | Templates Catalog (PufferPanel 44 JSONs vs Forge 14) | `pufferpanel-templates/spec.json:1`, `data.json`, `minecraft/minecraft.json` etc. | `packages/game-templates/*`, `template-schema.json`, `store_nests/templates/catalog`, `app-templates-data.ts` |
| 07 | Komodo Stacks/Builds/Periphery | `bin/core/src/api/write/stack.rs`, `periphery/src/api/compose.rs:414-783` | `services/compose/*`, `beacon/compose.go`, `services/build`, `alerting`, `observability` |
| 08 | Uncloud Mesh (WG+Corrosion+DNS+Caddy+IPAM) | `service.go:28-86`, `cluster/cluster.go:91-143`, `dns/server.go:294-326`, `caddyconfig/controller.go:92-163`, `store/store.go:64-117` | `placement/engine.go:37-77`, `heartbeatmonitor:89-313`, `servicediscovery/Registry`, `loadbalancer:22-29`, `crossnode/ingress_sync:129-164` |
| 09 | Dokku + CapRover minimal PaaS | `plugins/plugn.go:42`, `src/user/ImageMaker.ts`, `ICaptainDefinition.ts` | `services/apphosting`, `deployment`, `plugins/plugin.go:69`, `forgefile/service.go:137` |
| 10 | Docker-Compose & Build Fidelity | `pkg/compose/create.go:592/635`, `dependencies.go:17`, `loader.go:36`, `envresolver.go` | `compose/service.go:14/92/356`, `parser.go:245/913`, `beacon/compose.go:77/226/576`, `build/service.go:530`, `daemon/compose.go` |

---

## 2. Cross-cutting inventory: what 1Panel actually is (for Forge readers)

1Panel is **not a PaaS like Coolify** — it is a **single-host sysadmin console** that happens to also run Docker apps via OpenResty/Nginx site management. Its 60+ v2 API groups (`app/api/v2/*.go`) span:
- App store via in-panel catalog (not Git-driven)
- Website management backed by OpenResty with per-site nginx conf generation, proxy cache, WAF, per-domain or IP binding
- Per-engine database provisioning with TLS lattices (mysql.RegisterTLSConfig), user-grant matrices, php extension supermarkets, supervisor-managed FPM, node_modules lifecycle
- Battery of host appliances: firewall (firewalld/ufw), fail2ban, SSH, host_tool, FTP, disk, GPU, device tree, snapshot (panel tar with rotation), clamav, recyle_bin, ai/mcp_server
Forge is the opposite pole: **multi-node, Postgres-central, scheduler-mediated**. The comparison is therefore not "who is better" but **where a single-host panel solves host truth better than a control-plane can**, and where host tools should deliberately stay out of Forge.

Key divergence table:

| Concern | 1Panel | Forge | Verdict |
|---|---|---|---|
| App lifecycle | In-panel catalog + compose templates + BatchComposeTemplate | apphosting + catalog + compose + GitOps | Different origins, converge on compose |
| Website routing | OpenResty nginx conf per site with per-site WAF/cache/proxy | Traefik-shaped Gateway routers + Caddy JSON | Forge's gateway is the right multi-node equivalent; 1Panel's per-site nginx is correct for single host |
| SSL | Host-local OpenResty + acme.sh-style accounts | Central ACME + central CA + mTLS migrator | Forge correctly centralizes; but see LF-02..05 |
| Databases | Per-engine host services with host-local TLS | Central provisioner with DB-per-node pools | Both are host-local execution; Forge's encryption at rest stronger |
| Snapshots | Host tar with cron rotation | S3 artifact backup engine | Different durability models — not comparable |
| Firewall/SSH/fail2ban/FTP | Direct host appliance config | AllowedMountSources + hostfiles allowlist only | Forge correctly avoids host appliance sprawl |

---

## 3. Consolidated findings by theme

### 3.1 App Store / Catalog — Forge is stricter than 1Panel where it matters

1Panel App Store (`service/app_install.go:246` Operate, `571` GetUpdateVersions, `IsCrossVersion`) handles cross-version update guards, ignore-upgrade lists (`app_ignore_upgrade.go`), and compose template batch (`compose_template.go`). Forge's catalog (`store/store_catalog.go:14`) is well-isolated but appstore has two instructive defects:

- **LF-01 (service.go:239 resolveTemplate) — template interpolation silently swallows `${VAR:?err}` failures** and deploys stale compose — vs `service/app.go:347` which fails fast. A mis-templated upgrade deploys whatever string was produced.
- **LF-02 (service.go:139) — uninstall deletes DB row even when DeleteComposeStack fails**, orphaning stack+reservation on host — vs `service/app_install.go:246` ForceDelete guard that retains row on daemon failure.
- **LF-03 (UpgradeApp:158) — always upgrades to latest single version**, ignoring `CrossVersionUpdate`/`IsCrossVersion`/`app_ignore_upgrade` guard that 1Panel enforces via `GetUpdateVersions:571`.

Status: Compose validation in Forge **stricter than 1Panel** (`beacon/compose.go:77` validateComposePolicy). Catalog is `PARTIAL` but cleanly isolated.

### 3.2 Website / SSL / ACME / DNS — the densest new P0 cluster in this phase

1Panel website subsystem (7 API files, OpenResty conf generation) has no direct Forge equivalent — Forge centralizes at gateway. But the centralization has gaps surfaced by 1Panel's host-local wiring:

- **LF-01 (handlers_proxy_domains.go:199) — verify is hardcoded `verified:true` stub** vs real `domains/service.go:357` token DNS+HTTP verification — validation bypass if proxy-domain path is ever used for routing.
- **LF-02 (domains/service.go:362) — global mu held across net.LookupHost + httpClient.Do (10s)** in reverify ticker — serializes reverify across domains (P1 throughput bottleneck).
- **LF-03 — Split /certificates (ACME lego) vs /custom-certificates (proxy upload) duplicate routes**, UI mismatch (`certificates/page.tsx:44` posts wrong prefix), renewal ambiguity.
- **LF-04 (dns/service.go:637) — global os.Setenv env-mutation for lego providers** leaks beyond mutex, fails for lazy-env providers — concurrency hazard.
- **LF-05 (acme/service.go:492 renewOnce) — passes x509Cert.Raw DER leaf-only + PKCS8 DER key** instead of fullchain PEM — discards chain.
- **LF-06 (caddy_tls.go:240) — validateConfig is local JSON check only** vs real Caddy /load dry-run (`caddy_proxy.go:980-1002`) — weak validation.

Phase 4 already showed certs undelivered; Phase 6 adds: even if delivered, chain is wrong, verify is stubbed, and DNS creds are mutated globally.

### 3.3 Database & Runtime — strongest InventoryDiff in this phase (18 comparisons)

1Panel's per-engine surface (MySQL common + mysql clause, postgres/mongo/redis) vs Forge's central provisioner:

- MySQL multi-host/user + grant matrices, Redis status/persistence, Postgres row-level privilege granularity, plus the **entire `model.Runtime:9` / `PHPExtensions:3` language-runtime surface** (php/node/java/go/python/dotnet, supervisor, node_modules) are `MISSING` in Forge — **by design divergence**, not a gap to chase at gateway control-plane.
- TLS lattice, `TestConnection:477`, encrypted-at-rest `secretAAD` are `PARTIAL/STRONGER` in Forge.
- **Six logic findings** (all P1 syndromes in the centralization seam):
  - **LF01 (store_db_containers.go:174,200)** — list queries omit encrypted columns → fleet view returns empty credentials (only GetDBContainer selects them).
  - **LF02 (dbprovisioner/service.go:373,418)** — global mysql.RegisterTLSConfig leak/race per host, stale CA pin under concurrent provision.
  - **LF03 (database_service_provisioner.go:370)** — redis quoteSQLString dead-but-wrong branch + ignored REDIS_PASSWORD env.
  - **LF04 (containers.go:289,300)** — Deprovision swallows daemon error and hard-deletes row; Restart is status-only no-op vs real daemon path.
  - **LF05 — password generation divergence** (root==app vs root!=app inconsistency, rotation gap).
  - **LF06 (store_managed_databases.go:504)** — status-before-delete race + string-sniff legacy fallback masking schema drift.

Lesson: host-local TLS and encryption posture is fine; host-local runtime supervision belongs in 1Panel's domain — Forge should **not** import language-runtime management.

### 3.4 Monitoring / Files / Cron / Firewall — the "host truth" seam

Revisits Phase 2 and Phase 4 host stories with deeper file/host audit:

- Firewall `beacon/handlers_firewall.go:129` source-CIDR strictness vs Forge's Cloudflare-range fetch already noted — Phase 6 confirms allowlist `hostfiles.go:51` properly pinned, but `handlers_files.go:386` allows buffering unbounded uploads before openat2 check.
- **LF: cron retry-blocking & trigger duplicate** — `cronjob/service.go:158` retry blocks worker slot; `:263` scheduling trigger `triggerDuplicateCheck` uses wall clock, not monotonic, so NTP jump duplicates dispatch (mirrors Phase 5's per-replica periodic duplication theme).
- **LF: monitoring synthetic-zero fallback** — `monitoring/page.tsx:62` renders "0" as zero rather than "no data" for missing heartbeat window — masks gaps (consistent with synthetic timestamps finding in Phase 2).
- **LF: daemon uptime mislabel** — `handlers_host.go:65` reports `uptimeSeconds` from `time.Since(started)` without subtracting suspend intervals.
- Gaps intentionally omitted: docker daemon.json editing (host docker options), SSH host key rotation, fail2ban jail config, FTP user provisioning, snapshot rotation — all belong in 1Panel's host domain, not Forge fleet.

### 3.5 PufferPanel Daemon — the previously-excluded comparison, now corrected

Phase 2 excluded PufferPanel daemon as "architecturally unrelated." SA-05 proves overlap:

- Daemon vs Beacon split: both separate panel from node agent, but PufferPanel's operations are **24 typed ops** (`spec.json:307` `mojangdl|paperdl|steamgamedl|javadl|fabricdl|…+if:` conditionals) executed against a `data` map, while Forge's daemon is generic docker/runtime with single shell script blob.
- Environment vs multi-runtime: PufferPanel `environment` carries `supportedEnvironments[host,docker]` with per-env type; Forge multi-runtime is `docker|containerd|podman|firecracker|kubernetes` with phantom lxc/kvm.
- State machine: PufferPanel `installing/running` flags + long-poll `keepAlive` ticker vs Forge `desired/actual` + manager HandlePower fencing — Forge strictly stronger but PufferPanel's crash KeepAlive is close.
- Scheduling: PufferPanel has **none** (single-node panel assumption) — Forge placement/reservations correctly central.
- Filesystem/archive: PufferPanel `internal/operations` path handling vs Beacon `secure_files.go` openat2 — Beacon harder but PufferPanel's archive extraction respects `if:` guards per step.
- Idempotency/reconcile: PufferPanel tasks are per-server operation log vs Forge `job_queue`/`operations` duality — PufferPanel's operation log is actually simpler than Forge's dual-write projection.

Three instructive misses: Forge had no PufferPanel-style typed install pipeline for modded Minecraft, no operation log visibility comparable to PufferPanel's per-server keepalive channel, and scheduling is correctly central (no missing capability).

### 3.6 PufferPanel Templates vs Forge Game Templates vs 1Panel Catalog

- **Scale gap:** PufferPanel ships **36 game types / 44 JSONs** with **24 typed install ops**, `data` map typed as `option|string|boolean|integer` with `groups` ordering, `supportedEnvironments[host,docker]`, `run:{command[]+if, pre/post, stop||stopCode, stdin/stdout}` (`spec.json:1`). Forge ships **14 templates only on worktree branches (absent on main — `packages` on main is `sdk, shared-types` is empty dir)**, duplicated in `forge/web/lib/egg-templates.ts:1` (14 EGG_TEMPLATES) but **never seeded** — fresh DB has 1 egg (`migrations/091_seed_minecraft_java.sql:3`) = **93% seeding deficit**.
- **Model gap:** PufferPanel `data:{port:{type:integer, value:25565, required, userEdit}}` with `if:` conditionals; Forge collapses to `GameTemplate.env[]: {name,env_variable,default_value,string+rules}` (`template-schema.json:69`, `store_egg_variables.go:10`) and `install_script:{container,entrypoint,script}` single shell blob vs PufferPanel's pipeline — loses `internal/groups` conditional UX and typed `options` select.
- **Catalog split:** `store_catalog.go:84` (DBs/caches), `store_app_store.go:6` (7 compose apps), `store_nests.go:20` (eggs), `app-templates-data.ts:1` (5 localStorage defaults) — **no single source of truth**. 1Panel's `compose_template.go:3` `ComposeTemplate{Name,Description,Content}` + Batch is a strict subset and not a reference to chase.
- **Validation placeholder:** `scripts/validate-templates.mjs:46` covers only `startup+config.files` and `BUILTIN_VARIABLES:12` — `install_script` unchecked, no `appId/groups/stdin` checks.

Logic findings L1-L4: env-gated javadl/steamgamedl loss, startup↔ports drift (Beacon RCON), seeding deficit, typed→stringly `internal/groups` loss (reference `minecraft/minecraft.json:74` vs `store_egg_variables.go:92`).

### 3.7 Komodo Periphery — the cleanest agent comparison in the corpus

Rust periphery vs Go beacon: core/periphery channel (gRPC-like) vs Forge's HMAC-signed daemon client. Key discriminators:

- **Compose lifecycle is 6-phase** in Komodo (`periphery/src/api/compose.rs:414-783`: pull → build → create → up → wait-healthy → prune) vs single `docker compose up -d` shellout in Forge (`beacon/compose.go:344-416`). Health waits identically, but Komodo distinguishes build/pull phases for artifact caching — Forge does not cache builds per-stack.
- **File sourcing is trifecta** (compose content + env file + .env) vs Forge raw+git only.
- **Dual-layer security validation** is present in both (`compose/service.go:427-528` vs `compose.go:73-184`) — Forge **already stricter** on privileged mounts.
- **Queue/controller duality** exists in both (Komodo `stack/execute.rs:77-84` vs Forge `controller.go:130-163` + `queue_handler.go`).
- **GitOps at image-digest vs commit-SHA**: Komodo caches `image_digest`, Forge polls `commit-SHA` — digest is correct for docker-image deploys, SHA for git deploys; Komodo's singleflight monitor (`monitor/mod.rs:85-90`) vs Forge `observability/service.go:40` jitter — Komodo more efficient.
- **Alerter with maintenance-window dispatch** vs Forge threshold+SSRF-guard — Komodo strictly stronger for on-call.

Logic findings (6): `compose.go:213-224` shortFormHostPort `["80"]` bypasses privileged-port check; `gitops.go:381-387` DeployFromGit calls UpdateComposeStack on fresh stackID instead of Create; API vs beacon allowlist bifurcation (`checkVolumesSecurity:594-677` warning-only vs `validateComposeVolumes:226-248` hard reject → `200 valid` then `400 policy violation` with no recourse); webhook GitLastDeliveryID race across replicas.

### 3.8 Uncloud Mesh — the architectural counter-reference

Uncloud is the **anti-Forge**: decentralized WireGuard mesh (`network/wireguard.go:12-26`), CRDT Corrosion store with version vector + gap tracking (`store/store.go:64-117`, `cluster.go:371-483`), per-node embedded DNS `*.internal.` with `nearest/rr` locality (`dns/server.go:294-326`), per-node Caddy watching SubscribeContainers with fingerprint dedupe (`caddyconfig/controller.go:92-163`), Docker bridge EnsureUncloudNetwork with MTU/IPAM deferral (`docker/controller_linux.go:31-72`), fat-client broadcast orchestration (`service.go:28-86,93-115`). Forge is centralized Postgres `FOR UPDATE` reservations + scorer, Suspected→Unreachable→Offline hysteresis, in-memory servicediscovery keyed `tenant/service`, multi-algorithm LB with node-offline marking, central IngressSynchronizer.

Bifurcation:
- Uncloud placement is **optimistic + gossip-reconciled**; Forge is **pessimistic + DB-serialized**. Uncloud wins on partition tolerance; Forge wins on global correctness (no split-brain placement).
- Uncloud reachability is from each peer's vantage; Forge's VerifyCrossNode dials **from the panel** (`reachability.go:33-56`, `VerifyCrossNode:58-79`) — measures panel→target, mislabeled as cross-node (LF corollary of Phase 4's wrong-vantage finding).
- Uncloud IPAM is embedded (/16, 60k containers); Forge reservations are global and never double-allocate but can pre-assign the same host port via racy optimistic name check (`service.go:28-86` fatal de-dupe) — LF-01.
- DNS/Caddy membership-blind: Forge resolves through stale cache keyed by tenant/service while Uncloud propagates node membership into DNS+Caddy via subscription.

Lesson: Uncloud proves a mesh can work, but for Forge's fleet (game servers with Postgres-as-single-writer correctness requirements), **central placement is the right choice** — borrow only the per-node service discovery pattern and the "no healthy == 502 not 404" semantic difference.

### 3.9 Dokku + CapRover — minimal PaaS as a design lab

Dokku `plugn` plugin bus (`plugins/plugn.go:42`) is **executable** — `services/plugins/plugin.go:69` in Forge is **metadata-only** (`handlers_plugins.go:48` comment admits lifecycle unavailable). CapRover `CaptainDefinition` vs Forge `forgefile`/`forge_manifest*` (`forgefile/service.go:137`): forgefile warns but doesn't reject, leaks deploy[].env unencrypted vs CapRover AppsDataStore.ts:224 encryption.

Other instructives: build concurrency — Forge queue has no bound (`operation/service.go:158` retry blocks slot) vs CapRover single-build gate (`ServiceManager.ts:116`); RuntimeExecutor nil-guard in Forge deployment execution is **strictly stronger than both refs**; replicas drift between apphosting and deployment engine.

Status: plugin system decorative, one-click catalog pattern consumable, forgefile semantics weaker than CaptainDefinition.

### 3.10 Docker-Compose & Build — spec-fidelity audit (17 comparisons, 7 findings)

Deepest fidelity report of the program:

- **Coverage:** dependency graph `InDependencyOrder` vs Forge parser storing DependsOn as normalized strings (`parser.go:566`) but runtime not enforcing order (delegated to `docker compose up -d` health via DependsOn healthcheck); convergence `volumesFrom/networkMode` rewiring; env interpolation `$$/${VAR:-default}/${VAR:?err}` ported vs `env_file`/`include.env_file` unsupported; restart policy `getRestartPolicy:592` vs Forge warning-but-allow; resources `Memory/NanoCPUs/PidsLimit` mapping vs Forge isolation tiers; mounts/extension; compose spec builds per service (context/dockerfile/args/cache_from ignored — `DeployComposeFromGit` just passes file to compose engine).

This is the spec-fidelity anchor for the whole audit. Findings:

- **LF-01 Critical — env_file silently dropped.** Docker-Compose `env_file:` interpolated via `loader.go:36` + `envresolver.go` + `.env` + `include` + `${VAR:-default}`. Forge implements `${VAR}` subset, but `env_file` entries remain in YAML and are never mounted/resolved — they are resolved *by Docker Compose at deploy time* via generated `.env` sidecar only. For any compose repo relying on `env_file` for secrets, Forge silently runs with empty vars.
- **LF-02 — Resource limit bypass before placement.** Forge `DeployComposeStack:150` quota `MaxUserStackMemoryMB/Disk/CPUShares` enforced before placement, but beacon compose does not enforce ceilings — validated values not propagated to `docker compose` except as reservation accounting.
- **LF-03 — Volume policy bifurcation → data loss.** `checkVolumesSecurity:594-677` allows `/root` as warning + ValidateHostMountWithAllowlist gate, while `validateComposeVolumes:226-248` rejects all absolute host mounts unconditionally → `200 valid` at API then `400 policy violation` at beacon with no allowlist recourse; named volumes vs bind mounts mismatch can drop data.
- **LF-04 — Build context missing.** `DeployComposeFromGit` validates but never runs per-service `build: context/dockerfile/args/cache_from` — if Forge expects pre-built images via build.Service, compose services with `build:` will fail at `docker compose up` building on node without cache.
- **LF-05 — Restart/lifecycle race.** `RestartStack` shellouts `docker compose restart` while Forge queue holds reservation for `updating` — status field (`updating`) vs actual container restart not reconciled.
- Others: convergence rewiring unverified, scale per-service vs compose spec mapping ambiguous.

---

## 4. Hidden / Unwired / Dead — Phase 6 additions

| Surface | State |
|---|---|
| 1Panel AI/MCP server (`mcp_server.go`) | No Forge equivalent; correctly out-of-scope — control plane should not embed host AI tooling |
| PufferPanel typed install pipeline (`spec.json` 24 ops) | Forge single shell blob; richer pipeline exists only as pufferpanel-templates JSON, not as Forge execution |
| Komodo maintenance-window Alerter dispatch | Forge has threshold+SSRF guard but not windowed dispatch |
| Komodo buildx on periphery | Forge staged build service exists separately; not per-stack buildx on beacon |
| Docker daemon.json editing, SSH host key rotation, fail2ban jail config, FTP provisioning | 1Panel capabilities Forge intentionally avoids — no activation needed |
| Uncloud embedded DNS/Caddy per node | Correct for mesh; Forge discovery correctly central — not a missing capability but a chosen architecture |
| Dokku `plugn` executable bus | Forge plugin system metadata-only — honest decoration |
| Docker-Compose `env_file/include/extends/profiles` and per-service `build: secrets/shm_size` | Validated by libstack but not orchestrated; dead surface if not composed via engine |

---

## 5. Broken / incorrect logic (Phase 6 — consolidated, 30+ across 10 subagents)

P0: template resolve silently swallows interpolation failure deploying stale compose; upload valid then beacon-reject (API warning vs beacon hard reject) with no recourse — users see `200 valid` then `400` on same payload.

P1: uninstall DB-delete on failed daemon leaves orphan reservation+stack; UpgradeApp ignores CrossVersion guards; verify stub cross-tenant leakage; global mu across network I/O serializes reverify; DNS env mutation visible concurrently; hosts file buffering before openat2; synthetic-zero monitoring; daemon uptime without suspend subtraction; typed install pipeline absent; env_file dropped silently; volume policy bifurcation.

P2: DB container list omits credentials; mysql TLS registration global leak/race; redis dead branch ignored password; deprovision swallows error hard-deleting row; status-before-delete race; compose short-form `["80"]` bypasses privileged-port check; DeployFromGit creates via Update on fresh ID fails unless store upserts; Compose stack size/SGX host-mount warnings diverge.

Grouped, the dominant defect in this phase is **validation split across two enforcers** (API vs beacon) with different policy strictness for the same compose payload — a user-valid input's validity depends on which hop you ask.

---

## 6. Duplicates — Phase 6 additions

- Compose validation duplicated (`service.go:427-528` vs `compose.go:73-184`) with divergent allowlists.
- Template catalog fragmented into four surfaces (DB eggs / FS game-templates / localStorage app-templates / DB app_store) plus 1Panel `compose_template` as subset — no single source of truth.
- Firewall/host notion: `allowedMountSources` per node vs hostfiles allowlist vs API gate — same host path notion repeated.
- Build: staged `build/service.go` + compose's `Build` on periphery/beacon split (Komodo builds on agent side; Forge builds centrally then pushes to image ref — two paths for same image).

---

## 7. Architecture lessons — what to adopt vs reject from this phase

| Reference pattern | Forge today | ADOPT / INSPIRE / REJECT |
|---|---|---|
| 1Panel per-site OpenResty nginx conf generation | Central gateway with one reconciler | INSPIRE for template rendering discipline; ADOPT centrally but borrow per-site isolation semantics |
| 1Panel in-panel CA/DNS account registry | Central DNS service + ACME | KEEP central; FIX wiring (§3.2 LFs) |
| PufferPanel 24 typed install ops with `if:` conditionals | Single shell blob | INSPIRE for modded Minecraft/Fabric heavy installs; keep blob for simpler games |
| PufferPanel Catalogue `data.json` + provider pattern | `store_catalog.go` | KEEP Forge catalog well-isolated; unify sources |
| Komodo 6-phase ComposeUp (pull→build→create→up→wait→prune) | Single `up -d` | ADOPT phase decomposition for caching and per-stack buildx |
| Komodo singleflight monitor | `observability/service.go:40` jitter | INSPIRE singleflight deduplication |
| Komodo maintenance-window Alerter | Threshold + SSRF guard | ADOPT windowing |
| Uncloud central vs mesh placement | Postgres FOR UPDATE | KEEP central; BORROW per-node service discovery subscription pattern for servicediscovery self-registration |
| Uncloud nearest/rr DNS + per-node Caddy | Central ingress | INSPIRE nearest locality for DNS-resolved replica routing |
| Docker-Compose dependency graph `InDependencyOrder` | Normalized string storage | ADOPT explicit ordering or delegate verification that Compose respects it with healthcheck conditions |
| Dokku `plugn` executable bus | Metadata-only | REJECT at gateway control-plane — keep plugins metadata-only honestly or make pluggable workloads own execution |
| CapRover CaptainDefinition fail-fast | forgefile warn-but-allow | ADOPT fail-fast on interpolation errors; encrypt deploy env |
| CapRover single-build gate | Unbounded queue | ADOPT bound (1 per server) with backoff |

---

## 8. Recommended activation order (Phase 6 — existing capability → wired, no new subsystem)

1. **P0 Fix `resolveTemplate` to fail-fast on `${VAR:?}`/`${VAR?}`-style required errors** — `service/appstore/service.go:239`; surface parse error instead of raw template. Unblocks template fidelity trust.
2. **P0 Unify compose volume policy between API and beacon** — make `checkVolumesSecurity:594-677` and `validateComposeVolumes:226-248` share a single allowlist predicate (`ValidateHostMountWithAllowlist`) with consistent error codes; make `/root`-class mounts hard-error both places or explicitly gated behind admin+allowlist mount check end-to-end. Fixes the `200 valid` → `400 policy violation` trap.
3. **P1 Wire correct uninstall contract** — `UninstallApp:126-150` only deletes DB row after `DeleteComposeStack` success (or Force flag); cancel reservation idempotently; currently orphans.
4. **P1 Make UpgradeApp respect IgnoreUpgrade/CrossVersion** — query `app_ignore_upgrade` and `CrossVersionUpdate` before `UpgradeApp:158`; currently auto-max upgrades break 1Panel parity.
5. **P1 Fix verify stub leakage** — either delete `handlers_proxy_domains.go:199` hardcoded true or implement real token check against `domains/service.go:357` DNS+HTTP proof; cross-tenant route leakage already in Phase 4 F-NET-05 — verify stub worsens it with unauthenticated route takeover.
6. **P1 Eliminate DNS env global mutation** — `dns/service.go:637` global `os.Setenv` must become scoped config construction (lego providers now support struct config; stop env-mutating).
7. **P1 Fix database container list projection** — `store_db_containers.go:174,200` list must select encrypted columns (or explicit per-row fetch) — fleet view returns empty creds today.
8. **P1 Fix TLS race** — `dbprovisioner/service.go:373` mysql.RegisterTLSConfig per-host must not use global registry (use custom tls.Config per dial instead).
9. **P1 Wire correct env_file handling** — either implement `env_file` interpolation in `compose/service.go:14/92` (`interpolateEnv`) by mounting/resolving external env files pre-deploy, or **fail fast** when `env_file` is present in uploaded compose (document as unsupported and reject), rather than silently running with missing secrets.
10. **P2 Seed game-templates** — `packages/game-templates/*.json` on branches but absent on main: add `SeedGameTemplates` idempotent seeder (like `SeedDefaultApps:27` at `cmd/api/main.go:529`) covering EGG_TEMPLATES 14; unify `app-templates-data.ts:1` localStorage with egg-backed persistence or deprecate one catalog surface.
11. **P2 Fix short-form `["80"]` privileged-port bypass** — `beacon/compose.go:213-224` shortFormHostPort returns "" for len1 → `validateComposePorts:198-206` skips check; fix to parse `"<hostPort>"` as `hostPort==port`.
12. **P2 Fix DeployFromGit on fresh ID** — `gitops.go:381-387` calls UpdateComposeStack on fresh stackID → fails; use CreateComposeStack ( `lifecycle.go:390`) when no existing row, or make store upsert.
13. **P2 Add missing database observables** — Redis status/conf/persistence telemetry, Pg privilege granularity, supervisor/node_modules lifecycle if targeting language-hosting — otherwise document as out-of-scope.
14. **P3 Implement or remove host-tool window dressing** — Either mark freeze for fail2ban/SSH/FTP/snapshot host tools (document as 1Panel-only), or wire host appliance desired-state tables with reconciliation — do NOT halfway.

---

## 9. Handoff & update to final report

Phase 6 proves a thesis implicit since Phase 1: Forge's biggest advantage is its **multi-node, Postgres-central scheduler**. 1Panel wins host truth on a single box; Forge correctly refuses host-tool sprawl. The corollary: PufferPanel's typed install pipeline, Komodo's singleflight monitor + windowed alerts + per-phase compose, and Docker-Compose's dependency/env graph are the borrowable host/runtime refinements; Uncloud's mesh proves partition tolerance vs Forge's serializable correctness — Forge's choice is correct for its fleet.

Phase 6 findings extend the final report's §6 parity matrix: Template/Mount/Monitoring rows worsen Phase-5 verdicts, Networking TLS/Verify rows confirm Phase-4 P0s from a second reference family, and Runtime correctly quarantines firecracker while leaving lxc/kvm as metadata. No new subsystem is needed; every high-value item above is activation within existing tables/services.

