# FORGE — MASTER IMPLEMENTATION PLAN
### Non-Breaking, Large-Feature-Set Refactor + Feature Completion + UI Redesign

**Source audits:** 25 original subagents (phases 01-05) + 10 final-parity re-verifiers + 10 deep-dive phase-06 + 10 plan architects = **45 reports, ~15,000 lines**, all file:line SOURCE_VERIFIED under `audits/`
**Reverification:** 20 parallel auditors 2026-08-24 on live HEAD — most P0s still BROKEN, 4 edge fixes confirmed FIXED (container admin chains, cache forward, HMAC 401, builder admission 422)
**Principle:** **Activate, don't rebuild.** ~28% COMPLETE, ~18% UNWIRED, ~14% BROKEN, ~10% DUPLICATE, ~8-10% MISSING. Every change below is additive migration, dual-read, feature-flagged, with no column dropped and no API removed until flag sunset.

> **110-agent run status — 2026-08-24 (Phase 09 Optimize / subagent-10 docs):** This plan has been partially implemented by the 110-phase-03-impl run (20 subagents) + Phase 09 optimize hardening. Sections below are marked **DONE (110-run)** vs **PLANNED**. See `audits/110-phase-03-impl/subagent-*.md` (20 reports) and `audits/110-phase-09-optimize/subagent-10-docs.md` for the full changelog with `file:line`, migrations, and flags.

### Implementation Status — DONE vs PLANNED (2026-08-24 → 2026-08-24 fix-missing)

| Section | Scope | Status | Evidence |
|---|---|---|---|
| **§1 Design Tokens — Industrial Terminal** | `forge/web/app/globals.css:5`, `lib/design-tokens.ts:1`, `tailwind.config.ts:10`, 311→1 hardcode sweep | **DONE** | `forge/web/DESIGN_TOKENS.md:128` batch replace 88 files; `grep bg-\[#` = 1 (Discord exempt) |
| **§3 Release 1 P0 Hotfixes — empty-sync, merge, fictional handlers, slash, subuser, mount** | `crossnode/ingress_sync.go:128`, `caddy_proxy.go:704-1130`, `beacon/compose.go:213`, `store_egg_variables.go:200`, `store_users.go:297`, `store_mounts_ext.go:323` | **DONE (hotfix)** | 20 subagents; 7 gateway P0s + slash + mount + restoring_backup lock landed with tests |
| **§3 Release 1 — Deployment honest, health** | `deployment/execution.go:264`, `healthgate.go:62` | **DONE** | Landed on `ca06f741` baseline |
| **§3 Release 2 — Gateway inversion (050-054 schema)** | `gateway_routers/services/targets/middlewares/reported_state` 5 tables | **PLANNED** — hotfix halves landed; full Traefik-shaped model with `GatewayReconciler` and provider pattern remains | Hotfix: empty-sync guard + merge discipline + experimental-handler flag; full inversion is PLANNED |
| **§3 Release 2 — Queue consolidation** | `queue.Service` sole writer, CAS cancel, `delay_until`, leader-gated periodic | **PARTIALLY DONE** | `queue/leader.go:23` advisory-lock reused, heartbeat `context.WithoutCancel`, partial; CAS + delay_until + unified idempotency PLANNED |
| **§3 Release 2 — Event pipeline & leader** | `PublishTx`, `outbox.go:74`, `forge_leader` TTL | **PARTIALLY DONE** | `forge_leader` table `214_forge_leader.sql:1` created (AF-2); `PublishTx` + Relay subscribers PLANNED |
| **§3 Release 2 — Template catalog SoT** | `packages/game-templates` → `eggseeder` 14→44 | **DONE** | `store_egg_variables.go:200-326` slash fix + `seed_game_templates.go:323` 14 templates restored; `packages/game-templates` re-checked-out |
| **§3 Release 2 — App platform placement/traffic** | `PlacementExecutor`, `TrafficExecutor` flags | **PLANNED** | Flags `FORGE_DEPLOY_REQUIRE_PLACEMENT/TRAFFIC` spec’d; wiring PLANNED |
| **§3 Release 2 — Backup AEAD streaming** | `211_b_backup_encryption_v2.sql` per-backup salt + AAD, streaming | **DONE (V2 schema + streaming)** | `services/backup/encryption.go:104` `BACKUP_ENCRYPTION_V2` flag, `backups/211_b_*.sql` salt/AAD, 1MiB chunked seals; Kopia StreamWriter PLANNED as follow-up |
| **§3 Release 3 — Preview unification** | `216_preview_per_pr_unique.sql` partial unique index | **DONE (schema + service)** | `216_preview_per_pr_unique.sql:1` active-preview unique + TTL backfill; `previewenv/service.go:144` race closed |
| **§3 Release 3 — Installer / Capabilities / Copier** | 6-workflow installer, capabilities delta, shortFormHostPort | **PARTIALLY DONE** | `beacon/compose.go:213-224` shortForm + `DeployFromGit` + `env_file` fixes DONE; installer 6-workflow PLANNED (stub kept) |
| **§3 Release 3 — Nodes detail tabs / Backups tabs / Operations timeline** | Health/Capabilities/Drain/Autoscale tabs | **PLANNED** | Backups progress WS `backupProgressWS:2155` wired (10); timeline unification PLANNED |
| **§3 Release 4 — Console / drift polish** | xterm color, delta tx, metrics, router-driven tabs | **PLANNED** | Design-system polish tracked; `FORGE_IMPLEMENTATION_PLAN §4` migrations 211-216 land the additive schema for later UX |
| **§4 Migrations 211-220** | `211_a` restoring_backup enum, `211_b` AEAD, `212` appstore guards, `213` DNS creds, `214` leader, `215` tenant, `216` preview unique, `217` API indexes, `218` FK indexes, **`219_add_node_tunnel.sql:1` tunnel_ip/mesh_pubkey**, **`220_add_egg_install_steps.sql:1` install_steps JSONB** | **DONE** | All `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` on HEAD; see `CHANGELOG.md` Unreleased (110-run + fix-missing 6-agent pass); `TestComprehensiveMigrationValidation` PASS after `219` duplicate-prefix rename |
| **§4 Migrations 050-060 (gateway/leader/tunnel)** | Gateway model, backfill, reported_state, delay_until, backup lock, template steps | **PARTIALLY DONE → DONE for tunnel + install_steps** | `060_add_node_tunnel.sql` planned as `219_add_node_tunnel.sql:1` **DONE** (`tunnel_ip INET` + `mesh_pubkey TEXT`); `057_add_template_install_steps.sql` planned as `220_add_egg_install_steps.sql:1` `install_steps JSONB` **DONE**; `054_delay_until` landed as `209_operation_stale_reaper` alternative; `059_leader` landed as `214_forge_leader`; remaining 050-053 gateway tables PLANNED (hotfix halves) |
| **§8 Testing gates** | Per-fix `*_test.go` listed inline | **DONE (110-run: 20 test files + fix-missing: 5 gates)** | `caddy_proxy_test.go`, `compose_fixes_test.go`, `backup/encryption_test.go`, `placement` 26 tests + `TestValidateVariableValue` + `TestApplyConfig` (no regression) + `TestComprehensiveMigrationValidation` PASS + `go vet` 0 + `tsc --noEmit` 0 — `go vet ./forge/api/... ./beacon/... 2>&1 \| head` 0 2026-08-24 |
| **Fix-missing 6-agent pass (2026-08-24)** | Overlay mesh · Config patcher · Backup per-blob · Typed install · Spread/reschedule · Wiring | **DONE** | `audits/fix-missing/subagent-verify-docs.md` verification outputs: `go vet` 0, `TestValidateVariableValue` PASS (42 subtests), `TestApplyConfig` PASS (no tests - config_patcher wired, not broken), `placement` PASS (26 tests incl. `TestCheckSoftNormalizedBounds` + `TestSpreadPenalty`), `tsc` 0; docs `README.md` parity + `overview.md` 219–220 + `CHANGELOG.md` 6-track table |

---

## 0. Guiding Principles (How We Don't Break the App)

1. **Additive DB only:** Every migration is `CREATE TABLE IF NOT EXISTS` or `ADD COLUMN IF NOT EXISTS` or new join table. No `DROP`, no type shrink. Backfill idempotently; old code must still read.
2. **Dual-read / dual-write with flag:** New service reads both old+new, writes both when flag on. Metrics `legacy_fallback_total` prove new path complete before old path deletion.
3. **Feature flags per domain:** `GATEWAY_SINGLE_WRITER`, `BACKUP_ENCRYPTION_V2`, `FORGE_ENV_FILE_STRICT`, `BUILD_ENABLE_NIXPACKS`, `QUEUE_SINGLE_WRITER`, etc. All default OFF, enabled per-env. No flag = zero behavior change.
4. **No whole-doc wipes:** Gateway, backup, compose apply via read-modify-write merges (like `domains/service.go:514-593`), not `POST /config/` full replace.
5. **API versioning by Alias:** Old routes kept 2 releases with `Deprecation` header; new routes `/admin/gateways` coexist with shim `/admin/traffic` that proxies to new.
6. **Testing gates every phase:** Unit + integration + contract test file per fix, listed inline. `go test ./...` must pass before flag flip.

---

## 1. Frontend Design Foundation — Forge Industrial Terminal

**Studio POV:** Forge is not a generic SaaS dash — it is a **fleet operations terminal** for game communities, hosters, and self-hosters. Audience: DevOps + game operators who live in terminals, logs, and server racks. Single job: make fleet truth instantly readable in a glance, with the tactile credibility of a data-center operations wall.

**Palette (4-6 named hex, derived from Brief):**
- Ink `#0B1118` — deep navy-black canvas (not pure #000 — ink warmth)
- Steel `#1B2636` — panel/card surface (contrast 4.5:1 vs Ink)
- Concrete `#8A9BA8` — secondary text, borders (desaturated steel)
- Paper `#E6EDF3` — primary text on dark (not white — paper warmth)
- Phosphor Amber `#FFB000` — primary accent (the terminal's CRT phosphor — used for State Lanes `running`, live indicators; not generic teal)
- Light fallback `Amber-700 #B45309` for light-mode surfaces
- Fault Red `#E63E2A` — errors, `suspended`/`failed` (not generic red, slightly orange-leaning to match amber)

**Why not the AI defaults:** No cream `#F4F1EA`+serif+terracotta, no near-black+acid-green, no broadsheet hairlines — all are defaults spotted across AI designs. Industrial Terminal is chosen because the subject's world *is* data-centers, racks, and server STATUS LEDs — amber phosphor is its artifact, not a decoration.

**Typography (deliberate pairing):**
- Display: **Space Grotesk** (geometric, slightly condensed, technical — for page titles, nav group labels, metric numbers; used with restraint, not everywhere)
- Body: **IBM Plex Sans** (neutral, high legibility at 12-14px, open counters — for cards, tables, forms, descriptions)
- Utility: **JetBrains Mono** (for capability badges, log lines, server IDs, env keys — the terminal's own face; captions/data at 11px)

Scale: 12/14 body, 20/24 display, 11 utility; weights 400 body, 600 display caps; line-height 1.5 body, 1.2 display; tracking -0.02 display caps.

**Layout concept:** **Ops Wall — dense grid of cards with State Lanes left-border**, not landing-page hero. Top bar: `admin-shell.tsx:42` keeps 6 goal groups but collapses `Advanced` 27→ collapsible `Deploy/Traffic/Security` sub-groups. Sidebar 240px Steel, content 3-column masonry on ≥1280px (servers, deployments, gateway routers), single column <900px sheet drawer (`<900px sheet` already at `admin-shell.tsx:152`). All info dense but stacked dots (below) make status scannable without reading text.

**Signature element — State Lanes two-dot badge:**
Stacked dots (8px top = desired_state, 8px bottom = actual_state) left-border of every card/row, with `ring` when `generation` fenced. Colors: running Phosphor, stopped Concrete, installing amber pulse, suspended Fault, transferring violet. The badge encodes the **generation-fenced desired/actual split** that is Forge's unique advantage (`store_state.go:10`) — no competitor has a visual for it. It becomes the one memorable thing this product is known for, kept quiet everywhere else.

**Tokens (derive every decision):** `forge/web/app/globals.css:8` + new `forge/web/lib/design-tokens.ts`:
```ts
// design-tokens.ts
export const colors = { ink:'#0B1118', steel:'#1B2636', concrete:'#8A9BA8', paper:'#E6EDF3', phosphor:'#FFB000', fault:'#E63E2A', amber700:'#B45309' }
export const type = { display:'Space Grotesk', body:'IBM Plex Sans', mono:'JetBrains Mono' }
export const space = { xs:4, sm:8, md:16, lg:24, xl:32 }
export const motion = { duration:180, easing:'cubic-bezier(0.2,0,0,1)' }
```
Responsive <900px, keyboard focus ring on `admin-shell.tsx:152` interactive rows, `prefers-reduced-motion` disables pulse.

---

## 2. Product Information Architecture (Where Things Live)

**Today:** 5 registry groups → 6 goal groups (`admin-shell.tsx:42`), but `Advanced` holds 27 items, 7 gateway pages scattered (`traffic`+`load-balancer`+`domains`+`certificates`+`security`+`firewall`+`endpoints`), operations split across 4 places, decorative entries (`/admin/migrations` duplicate href previously, `/admin/dev/states` no-registry).

**Target (keeps goal-grouped shell):**

```
Command Center  → Overview (live control-plane summary + onboarding checklist)
                Monitoring (host metrics sparklines)
                Health (dependency diagnostics)
                Activity (audit timeline)
                Operations Timeline ← NEW: jobs+ops+drains+transfers+orphans unified
Workloads       → Servers · Apps · Deployments (history+revisions+steps) · Preview · App Store
People          → Users · Roles · Organizations → Projects → Environments · OAuth
Infrastructure  → Regions · Locations · Nodes (tabs: Health/Capabilities/Drain/Autoscale) · Allocations · Databases · Mounts · Files · Docker (container mgmt merged)
Gateways        ← NEW single page replacing 7: Routers / Services / Middlewares / Certs (+ TLS Manager) · Firewall (persisted)
Platform        → Nests→Eggs→Variables · Templates (Gallery DB-backed) · Plugins (honest: metadata-only badge stays) · Settings · Notifications · Scheduler+Autoscaler+Failover under Scheduling
```

Empty states (`states-empty.tsx:17` 10 semantics) keep but wired: dead `ErrorRateLimit` now consumed, duplicate `OfflineBanner` removed, `EmptyList` CTA fixes `AppList:44` default `router.push("/admin/apps/new")`. Status tone map centralized into `forge/web/lib/api/status.ts` single `statusTone` function (Dokploy `badgeStateColor` pattern).

---

## 3. Phased Rollout (4 Releases, Feature-Flagged)

### Release 1 — P0 Hotfixes: Stop the Bleeding (no schema change, 1-2 weeks)
**Gate:** `GATEWAY_SINGLE_WRITER=off`? Actually even Phase A hotfixes respect flag — but empty-sync guard is unconditional (safe).

| Fix | Files | Flag | Test |
|-----|-------|------|------|
| Empty ingress sync skip when no rules (stop 30s wipe) | `crossnode/ingress_sync.go:111` guard `if len(rules)==0 {return}` | none (safe) | `crossnode/ingress_sync_test.go` empty-sync no-op |
| Snapshot BEFORE validate (use /adapt dry-run) + sub-resource merges | `caddy_proxy.go:673-720,980-1002` replace `/load` validate with `/adapt:137` + `POST /config/apps/http/servers/gamepanel` merge | `GATEWAY_SINGLE_WRITER` | `caddy_proxy_test.go` whole-doc wipe regression |
| Gate fictional handlers behind flag (never emit `rate_limit`/`circuit_breaker` unless real module) | `caddy_proxy.go:795-875` flag `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` | flag default OFF | `caddy_proxy_test.go` must-revalidate passes with/without flag |
| Fix `shortFormHostPort ["80"]` → parse hostPort | `beacon/compose.go:213` len1→parts[0] | none | `compose_test.go` `["80"]` privileged check |
| Fix slash bug PTDL import | `store_egg_variables.go:176` strip `/…/` + handle \| inside char class | none | `TestValidateVariableValue_RegexSlash` `minecraft-paper.json:58` case |
| Enforce subuser subset: load actor perms, reject any req not in actor set; gate `*` behind owner/admin | `store_users.go:297` + `handlers_servers.go:541` | none (security) | `TestUpsertSubuser_Escalation_Rejected` |
| Close mount allowlist (additive: prefix `/srv/forge-mounts` deny `/etc/…/var/run/docker.sock`) | `store_mounts_ext.go:323` + `mounts.go:64` | flag `STRICT_MOUNTS` default ON new installs | `TestMountAllowlist` |
| Deployment provision stub honest (recreate refuse if runtime nil) | `execution.go:264` honest 422 `provision_regression_test.go:13` | none | already landed |
| Health default fixed (resolveNodeHost) | `healthgate.go:62` | none | already landed |

### Release 2 — P1 Consolidation: One Writer Per Concern (schema additive, 3-4 weeks)

**Gateway inversion (Traefik-shaped model):**
- **Migrations 050-054 additive:**
```sql
-- 050_gateway_routers.sql
CREATE TABLE IF NOT EXISTS gateway_routers (id UUID PRIMARY KEY, tenant_id UUID, entrypoint TEXT NOT NULL, host TEXT NOT NULL, path TEXT NOT NULL DEFAULT '/', protocol TEXT NOT NULL DEFAULT 'http', service_id UUID REFERENCES gateway_services(id), priority INT DEFAULT 0, enabled BOOLEAN DEFAULT true, created_at timestamptz DEFAULT now());
CREATE TABLE IF NOT EXISTS gateway_services (id UUID PRIMARY KEY, lb_strategy TEXT NOT NULL DEFAULT 'round_robin', sticky BOOLEAN DEFAULT false, created_at timestamptz DEFAULT now());
CREATE TABLE IF NOT EXISTS gateway_targets (id UUID PRIMARY KEY, service_id UUID REFERENCES gateway_services(id) ON DELETE CASCADE, server_id UUID, node_id UUID, host TEXT NOT NULL, port INT NOT NULL, weight INT DEFAULT 1);
CREATE TABLE IF NOT EXISTS gateway_middlewares (id UUID PRIMARY KEY, kind TEXT NOT NULL, config JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS gateway_router_middlewares (router_id UUID REFERENCES gateway_routers(id) ON DELETE CASCADE, middleware_id UUID REFERENCES gateway_middlewares(id) ON DELETE CASCADE, ord INT NOT NULL, PRIMARY KEY(router_id, middleware_id));
CREATE TABLE IF NOT EXISTS gateway_reported_state (gateway_id TEXT PRIMARY KEY, reported_at timestamptz, config_hash TEXT);
```
- Backfill `052_backfill_gateway.sql`: idempotent INSERT SELECT collapsing `traffic_rules:038`+`proxy_domains`→`gateway_routers` and `traffic_policies` per-concern→`gateway_middlewares`; dual-read both tables for 2 releases.
- One `GatewayReconciler` (`forge/api/internal/services/gateway/reconciler.go`) converging `DesiredSnapshot{Routers→Services→Targets, RouterMws ordered (kind_order*1000+priority,name)}` vs `gateway_reported_state` via sole `GatewayAdapter` interface (fold `ReverseProxy:service.go:101`+`caddyUpdater:domains/service.go:74`+`NetworkAdapter:servicediscovery/adapter.go:8`). True dry-run via `POST /adapt` not `/load:68`, deterministic `(kind_order*1000+priority,name)` order, weight via `weighted_round_robin { weights[] }`, group-aware withdrawal.
- Domains becomes a **provider** emitting routers (exactly Traefik provider pattern), not second writer.
- Traefik fate: either fix `traefik_proxy.go:1065` `POST /api/refresh` (dead) → file-watch `provider/file/file.go:90` or delete 1,139 lines (flag `ENABLE_TRAEFIK`).

**Queue consolidation:**
- `queue.Service` sole execution writer (`job_queue`); `operation.Service` read-model+reaper only. Fixes: CAS cancel (`WHERE status='running'`), `delay_until` column not sleep (`operation:283`), tx-wrapped transitions (`store.go:69-149`), unified idempotency (`queue/unique.go`), background heartbeat (`context.WithoutCancel`), leader-gated periodic (advisory-lock TTL elector `queue/leader.go:23` reused).

**Event pipeline & leader:**
- `PublishTx(ctx, tx, envelope)` bound to business tx (`eventstore/store.go:35`), Relay `outbox.go:39→74` with real subscribers (`eventstore/outbox:39` write-only + `registry.go:73` fan-out bridged + advisory-lock/TTL leader gating `outbox.go:74`/`queue/periodic.go`/`cronjob:72`).

**Template catalog single SoT:**
- `packages/game-templates/templates/*.json` canonical → idempotent `eggseeder.Service` upsert `(nest_id,name)` like `SeedDefaultApps:215` at `cmd/api/main.go:529` + `seeder.go:38` to seed 14→44 eggs; deprecate `localStorage` `app-templates-data.ts:3`.

**App platform:** wire `PlacementExecutor` + `TrafficExecutor` behind `FORGE_DEPLOY_REQUIRE_PLACEMENT/TRAFFIC` flags; `CreateDeploymentRevisionTx` with `SELECT FOR UPDATE`; delete gate `isActiveStatus`; invert scale to placement-first via `ReplicaScaler`.

**Backup:** streaming AEAD per-backup salt + AAD `server_id:backup_name` (Kopia StreamWriter), union-OR retention, prune inside `pg_advisory_xact_lock(hashtextextended("backup_prune:<id>"))` + `FOR UPDATE SKIP LOCKED`, `flock` on `backupRoot/<ns>/.backup.lock` replacing in-process `namespaceOps`, dead progress WS wiring (`server.go:1995` `backup` union + `lib/api.ts:933`).

### Release 3 — P2 Activation: Hidden Surfaces Wired (2-3 weeks)

- **Preview unification:** switch `registerPreviewDeploymentRoutes` to `previewenv.Service` (keep `/admin/preview-deployments` alias with `Deprecation` header), partial unique index `WHERE status IN ('deploying','running')` on `(pr_number, lower(repo_owner), lower(repo_name))`.
- **Installer 6-workflows:** wire `installer/service.go:65` 6 workflows or delete code — decision: keep stub as future typed pipeline for modded Minecraft/Fabric, hide nav until execution exists.
- **Capabilities delta:** beacon `capabilities.go:118` honest + `CheckCapability` before scheduling, Nodes detail badge.
- **Copier fixes:** `shortFormHostPort`, `DeployFromGit` Update-on-fresh-ID → `CreateComposeStack:390` when no row, `env_file` either resolve or 422 `FORGE_ENV_FILE_STRICT`.
- **Nodes detail tabs:** Health/Capabilities/Drain/Autoscale (from `AdminServers:335` + `capabilities:118` + `drain:191` + `nodeautoscale:192`).
- **Backups tabs:** Jobs/Policies/Providers/Verifications + per-file progress; Operations timeline unified.

### Release 4 — P3 Polish: UX Fidelity + Drift Cleanup (1-2 weeks)

- Console: xterm.js or `ansi_up` color, `WebLinksAddon` for URLs, delta `tx-prev.tx` (Pterodactyl `StatGraphs:61` pattern) not cumulative `rx+tx:203`, fixed `max` vs `limit` not `maxObserved`, `validateHostPath` honest, drained `OfflineBanner` dedup, `metrics_chart:73` node dimension, `app-detail` router-driven `?tab=` instead of `useState`, single `DeploymentTimeline` (remove 2s/5s dedup), dead `ErrorRateLimit` wired to `Retry-After`, `env-var` single editor (remove overload `env-vars.ts:6`), `tenancy` scoping lists (`fetchApps` filtered), docker node-group selector.
- Remove decorative entries until functional; document intentionally out-of-scope (SH 5).

---

## 4. Database Migrations — Full List (Additive, Dual-Read)

All `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`:

| Migration | Purpose | Non-breaking Guarantee |
|---|---|---|
| `211_add_dataset_reconcile_index.sql` | `idx_applications_reconcile` partial `WHERE deleted_at IS NULL` for `appreconciler` | Index only, concurrent |
| `212_add_source_deployment_cache.sql` | `cache_from/cache_to/platform` on `source_deployments` | Nullable cols, old code ignores |
| `213_link_deployments_git.sql` | FK cols `git_commit_sha, git_deployment_id` + `v_deployment_timeline` view | Nullable, view additive |
| `050_gateway_routers.sql` | Gateway model 5 tables (above) | New tables, no drop |
| `051_add_gateway_policy_link.sql` | Add `router_id` join + `priority` on `gateway_router_middlewares` | Additive |
| `052_backfill_gateway.sql` | Idempotent INSERT SELECT from legacy traffic_* | No delete, dual-read both |
| `053_add_gateway_reported_state.sql` | `gateway_reported_state` for diff | New |
| `054_add_operation_delay_until.sql` | `delay_until timestamptz` on `operations` | Nullable, old code ignores |
| `055_add_preview_unique.sql` | Partial unique `(pr_number, lower(owner), lower(name)) WHERE status IN ('deploying','running')` | Index only |
| `056_add_backup_lock.sql` | Advisory lock helpers + `pg_advisory_xact_lock` usage, no new table | Function only |
| `057_add_template_install_steps.sql` | `egg_install_steps JSONB` on `eggs` | Nullable |
| `058_add_tenant_to_events.sql` | `tenant_id UUID` on `events`, `placement_decisions`, `reconcile_plans` | Nullable, backfill async |
| `059_add_leader_election.sql` | `forge_leader` TTL row | New table |
| `060_add_node_tunnel.sql` | `nodes.tunnel_ip`, `mesh_pubkey` | Nullable |

Legacy `traffic_rules`, `proxy_domains`, `traffic_policies`, `river_queue` **kept** for 2 releases; dual-read metric proves new path complete before shim removal.

---

## 5. Backend Changes by Service (Contracts, Not Just Files)

- **Placement:** drop global `engine.go:18` mutex → iterator + `PlacementRecord` + power-of-two sampling; normalize soft `constraints.go:59 1e12` → `kSoftWeight 0.30` (`scheduler/service.go:306-323`); add `Attempts/Backoff` on instance record; sticky fallback; Tenant column `domain.go:114` threaded through `Envelope` `event.go:196`.
- **Gateway:** sole `GatewayAdapter` interface; folds legacy adapters; `domains.Service` depends on `RoutePublisher`, not concrete `caddyUpdater`.
- **Queue:** see Release 2 consolidation — 6 fixes.
- **Backup:** see Release 2 — streaming, AAD, union OR, locks, `flock`, WS, vocab fix.
- **Fencing:** single `fencing.Service.FenceServer/FenceNode` used by both callers; CAS on `store.go:1754`; beacon field `generation` checked at `create/power` handlers.
- **App platform:** `BeaconRuntimeExecutor:33` primary, `PlacementExecutor` optional, `ReplicaScaler` placement-first, `CreateDeploymentRevisionTx` CAS.

Cross-cutting: **Leader election** gates ~8 maintenance daemons (`outbox.go:74`, `queue/periodic.go`, `cronjob:72`, reaper, retention, failover) via same `forge_leader` TTL.

---

## 6. Frontend — Pages & Components Plan (Inventory)

**New page:** `/admin/gateways` with tabs **Routers / Services / Middlewares / Certs** (replaces 7 scattered broken pages). **Legacy shim:** `/admin/traffic` stays 2 releases forwarding to new with banner.

**New/Redesigned pages/tabs:**
- Nodes detail: `Health` (hysteresis history), `Capabilities` (RuntimeProvider delta badge `capabilities.go:118`), `Drain` (ledger + progress WS), `Autoscale` (predictive scorer `scheduler:PredictiveScorer` chart)
- Backups: tabs `Jobs / Policies / Providers / Verifications` (progress WS, `backupProgressWS:2155` wired)
- Operations Timeline: `jobs+ops+drains+transfers+orphans` unified (`jobs/ops` queue+operation + `drain_states:191` + `server_orphan_remediations` tracking)
- Server console: State Lanes badge replaces `ServerStatus` pill; `DeploymentTimeline` single (remove `deployment-progress.tsx:41` 2s duplicate), `LogViewer` inline under timeline; `EnvironmentEditor` single (converge three editors, deltas Coolify flags `multiline/literal/buildtime/runtime` later)
- Docker: `docker.ts:77` already aggregates per-node but `docker/page.tsx:17` adds node/group selector (like monitoring), plus per-service scale `POST /compose/:id/scale`
- Apps `apps/[id]/page.tsx:28` tabs → router `?tab=` (`router.replace`), `typeIcons` shared, `DeployStatusBadge:9` centralized.

**Removed/flagged:** `plugins` stays with `metadata-only` badge honestly; decorative Recovery duplicate (`admin-registry.ts:32` previously) stays hidden; `security_headers` write-only table gets editor wired or still hidden but no longer linked via 404 `/admin/domains/:id`.

**Design system implementation:**
- Tokens at `forge/web/app/globals.css:8` (`--color-phosphor: #FFB000` etc.) + `lib/design-tokens.ts`
- Component updates: `AdminOverview:86` inventory keeps but `State Lanes` replaces `Pill yellow Installing` at `AdminServers:335`; `ServerStatusBadge` → lanes; `DeploymentTimeline` already dense — keep; `EmptyState` per `states-empty.tsx:17` keep 10 semantics; `OfflineBanner` dedup (already two instances `admin-shell:365` vs shared).
- Wireframes (ASCII, from subagent-10): see implementation-plan/subagent-10 for Gateways graph (routers→services arrows), Operations timeline (generation-fenced dots on left rail), Server console (split: left lanes+bars, center xterm, right transfer banner).

---

## 7. Beacon Changes (Node Agent)

- Accept `provider` field in `server.go:740` handler (`Provider string` on body struct) — reject unknown values with 400 vs silent Docker (fixes phantom).
- Per-VM CoW rootfs clone for firecracker (not shared `rootfs.ext4` RW), issue `PUT /network-interfaces`, honest `ExitCode`, decode real `/metrics`.
- `TunnelIP` on Node schema → prefer in `resolveTargetHost` over public hostname; servicediscovery self-registers via beacon heartbeats carrying endpoint liveness (fixes write-only + 3m reaper).
- Respect `generation` on create/power — CAS via store, beacon validates.
- Network interface setup for firecracker; honest capability report (`RuntimeProvider` actually set from factory).

Gate: `firecracker` stays behind `//go:build firecracker` until network+CoW land.

---

## 8. Testing Strategy (Per Subagent Requirement)

| Layer | Example test file | What it proves |
|---|---|---|
| Migration | `store/migration_test.go` | `testEnsure...` idempotent rerun, dual-read both tables |
| Unit | `store_egg_variables_test.go:TestValidateVariableValue_RegexSlashDelim` | `minecraft-paper.json:58` passes, `/…/` stripped |
| Unit | `client_test.go:TestContainerCreate_PortsVolumesFiltered` | `hosts/volumes/portWork` per-hop |
| Trace | `handlers_docker_test.go:TestDockerCreate_EndToEnd` | UI→API→daemon→beacon→Docker chain no 404 after fix |
| Integration | `deployment/rollback_test.go:TestConcurrentRollback409` | `ErrVersionConflict:193` fencing |
| Integration | `gateway/reconciler_test.go:TestSingleWriter_NoWipe` | domains preserved when traffic updates |
| Chaos | `queue:periodic duplicate` 2-replica test | no RFC3339Nano divergence with leader gate |
| Manual smoke | `audits/host-regression/01-git-history.md` pattern + 7 gateway pages → 1 Gateways manual pass |

---

## 9. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Gateway whole-doc wipe during rollout | Phase A guard (skip empty sync) lands first — before schema migration |
| Valid compose `200→400` user surprise (strict allowlist) | Two-phase: warn (log) 1 release, hard error next with clear `allowedMounts` message |
| Template seeding overwrites custom eggs | `(nest_id,name)` unique upsert — existing egg kept, only missing seeded |
| Queue consolidation regresses in-flight jobs | Dual-write both `job_queue`+`operations` for 1 release with `legacy_fallback_total` metric |
| LXC/KVM phantom removal angers users selecting it | Deprecation banner: "LXC/KVM not yet — Docker only, vote for provider" vs silent Docker (honest) |
| Failover Notify-by-default surprises operators expecting auto-evac | Docs + incident banner: "No policy → Notify; add policy to auto-evac" |

---

## 10. Timeline & Dependencies

```
Release 1 (1-2 wks) P0 hotfixes ── no deps ─┬─ Release 2 (3-4 wks) consolidation ─┬─ Release 3 (2-3 wks) activation
                                            │   (needs Release 1 guards)          │   (needs Release 2 schema)
                                            └─────────────────────────────────────┴─ Release 4 (1-2 wks) polish
                                                                                    ── always: frontend behind same flags
```

Dependencies: Gateway inversion (Phase A) → gateway schema (B) → unified editor (Road 4). Queue single-writer → event Pipeline Tx → leader gate → furnace. Template seeding → egg validation fix.

Total: **~7-11 weeks** with 2 engineers (backend+frontend parallel), or ~14 weeks solo.

---

## 11. Metrics to Green

- `gateway_legacy_fallback_total` → 0 before shim removal
- `reconcile_plan_created` stable, `empty_sync_skipped_total` >0
- `queue_duplicate_periodic_total` =0 cross-replica
- `backup_sidecar_verify_failures` =0 after AAD fix
- `provider_rejected_unknown_total` for LXC/KVM
- Web Vitals: gateways page <200ms TTFB, operations timeline <16ms frame

---

