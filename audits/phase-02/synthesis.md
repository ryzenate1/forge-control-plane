# Phase 02 Synthesis — Game Hosting Cluster

**Cluster:** pelican-panel, pelican-wings, pterodactyl-panel, pterodactyl-wings, pufferpanel, pufferpanel-templates  
**Why this cluster next:** After Phase 1 validated Forge's app-platform lifecycle, the canonical correctness horizon for *game* servers is Wings. Every game capability (create/install/power/suspend/transfer/backup/SFTP/console/allocations/eggs/variables/schedules/subusers) has a 1:1 Forge↔Wings mapping to audit for parity, divergence, and Beacon superset. PufferPanel's `pufferpanel-templates` vs `packages/game-templates` tests the template catalog dualism surfaced in Phase 1 C11/C12.  
**Evidence:** 5 subagent reports (≥18 comparisons each) — lifecycle, eggs/templates, nodes/daemon, game UX, tenancy/schedules — file:line verified, not marketing.  
**Method:** Compare panel models (`Server.php`, `Egg.php`, `VariableValidatorService`), wings daemons (`power.go`, `install.go`, `console.go`, `filesystem`, `sftp`, `backup`), PufferPanel `spec.json` typed ops, against Forge `store_*.go`, `handlers_servers.go`, `clustermanager/*`, `operation/*`, `migration/*`, `beacon/internal/server/*`, `beacon/internal/runtime/*`, `forge/web/components/server/*`, `server-lifecycle.md`.

---

## 1. Forge equivalents at a glance

| Concern | Forge location (canonical) | Relation to reference |
|---------|----------------------------|----------------------|
| Server lifecycle | `store_state.go:10` desired/actual + `store_servers_control.go:13` power states + `store_servers_lifecycle.go:12` delete + docs `server-lifecycle.md:1` | Superset: generation fencing + `state_transitions` audit; Pterodactyl had single `status` nullable |
| Create→provision | `clustermanager/service.go:76` CreateServer (placement → DB `provisioning` → runtime Create → `created`) `store_servers.go:253` | More truthful than Pterodactyl default `status=installing` at creation; separate `provisioning` never exposed to runtime |
| Power | `handlers_servers.go:875` power (Operation Queue `202`), `beacon/manager.go:482` HandlePower (TryLock), `runtime/docker.go:151` Create/Start/Stop/Kill/Restart, `store_servers_control.go:44` powerSignalPriorStates allowlist | Durable queue vs Wings synchronous; Beacon mirrors `installing/transferring/restoring` guards + `onBeforeStart` sync |
| Suspend | `store_users.go:88` plus `store_servers.go:374` `SetServerSuspension`/`CompareAndSet`, `orchestrator/suspension.go:28` SuspendServer, `handlers_servers.go:1400` suspension endpoints (admin `suspend/unsuspend` + `suspension` action) + legacy compat | Forge keeps `suspended` boolean orthogonal to `status` — Pterodactyl/Pelican conflate `status='suspended'` wiping install/restoring |
| Install/Reinstall | `handlers_servers.go:1037` install, `1097` reinstall, `clustermanager/service.go:216` runInstaller, `beacon/server.go:1044` install + `1151` installWS, `manager.go:271` BeginInstall fencing, `runtime/docker.go:256` Install container `mgp-*_installer` 1000:1000 readonly+capDrop | Installer container security stricter; `installWS` gates `ScopeAdmin` unlike Wings panel stream; but `installer/service.go:66` 6-step workflow is dead code |
| Delete | `server-lifecycle.md:6` Beacon-first, `clustermanager/service.go:332` DeleteServer (calls runtime, orphan on `!force` error), `store_servers_lifecycle.go:17` `RecordOrphanAndHardDeleteServer`, `beacon/server.go:1425` delete | Adds `server_orphan_remediations` tracking absent in Pterodactyl; PufferPanel `IsIdle` guard mirrored |
| Backup restore lock | `store_state.go:131` `restoring_backup` ↔ `store.go:1479` but never driven by backup routes (`handlers_servers.go:2104` marks `backups.status=restoring` not server status) | Missing vs all 4 refs treat `restoring_backup` as server exclusive lock |
| Transfer/Migration | `server-lifecycle.md:7` legacy Wings transfer disabled `501`; modern `migration/service.go:102` Create/ExecuteMigration (`migration_runs`+`allocation_reservations`), `servers.transfer_state` legacy shim + `040_truthful_server_lifecycle.sql:7` clobber; Beacon `transfer_protocol.go` v1 dual credentials | Pelican transfer (token→allocation reservation→daemon peer push) replaced by control-plane mediated v1; Pterodactyl `server_transfers.successful NULL` vs Forge `migrations` |
| Eggs/Nests | `store_nests.go:16` Nest, `34` Egg, `store_egg_variables.go:17` EggVariable, `store_startup.go:11` GetServerStartup/`validateVariableValue`, `store_templates.go:12` compatibility transform `server_templates→eggs` `043_unify`: | Faithful port of Panel, plus `normalizeDockerImages` handles map-or-array more permissive, but `rules` regex `/…/` delimiter handling broken |
| Templates dualism | `packages/game-templates/template-schema.json:1` draft-07 schema + `src/types.ts:42` GameTemplate + `templates/minecraft-paper.json:10` 15 curated + `forge/web/lib/egg-templates.ts:27` static array vs DB eggs + `forge/web/lib/app-templates-data.ts` localStorage | Three systems unsynced: DB eggs (canonical), FS `game-templates` catalog (static catalog), `app-templates` localStorage (UI helpers) |
| Allocations | `store_allocations.go:15` Allocation(`inet IP, protocol tcp/udp, containerPort`), bulk Tx, `AssignAllocationToServer:327` node-affinity `FOR UPDATE` | Rips Pterodactyl but adds `protocol`+`containerPort` explicit (Pterodactyl `Mappings map[string][]int` dual tcp/udp) |
| Resources | `store_servers.go:209` + `beacon/runtime/docker.go:901` validateCreateRequest + `shared-types/api.ts:98` `cpuShares/cpuLimit/threads` | Pterodactyl `environment/settings.go:37` Limits compare shows `cpu` percent vs `cpu_shares` weight conflation across store/daemon/beacon |
| Config files | `store.go:698` `ServerProvisionTarget.Mounts` + `daemon/client.go:468` Mounts passed but `wings/parser:1` `file/yaml/properties/ini/json/xml` patching not reimplemented; game-templates `minecraft-paper.json:16` `find:server-port→{{server.build.default.port}}` inert | Gap: `config` JSON stored not applied |

---

## 2. Hidden / unwired (activation)

- `installer/service.go:66` 6-step workflow (`docker.create/filesystem.setup/download.server/script.install/config.apply/server.start`) persists rows but never executed — dead workflow engine (`REF-GAME-HIDDEN-01`).
- `server_orphan_remediations` table (`040`) tracks hard-delete failures but has no admin resolution UI (`handlers_admin.go` lacks `GET /orphans`); orphan rows accrue silently.
- `transferTargetNodeId`/`transferState` on `servers` are legacy columns superseded by `migrations` yet still drive `ensureTransferIdle:145` guards — dual state split makes `GET /transfer` read stale column (`REF-GAME-HIDDEN-02`).
- `fetchServerCrashHistory` in `CrashBanner` polls every 30s but `server_crash_events` not hydrated on `fetchServers` list — admin list has no crash badge.
- `game-templates` curated catalog has no seed job; DB eggs drift from FS templates unless admin manually imports (`REF-GAME-HIDDEN-03` — also Phase 1 C11).

---

## 3. Missing (genuine)

- Wings `allocations` + `eggMount` pivot is superset for most games, but PufferPanel's 26 typed install operations (`spec.json:307` `mojangdl|paperdl|steamgamedl|curseforge|fabricdl…` with `if:` conditionals) and multi-env `supportedEnvironments[host,docker]` vs Forge single script blob (`template-schema.json:95` `install_script{container,entrypoint,script}`) — gap only if targeting modded Minecraft Forge/Fabric pipeline fidelity.
- PufferPanel's cross-node live migration (absent — Forge's `migration` is actually ahead; Wings transfer is archival move, not live).
- PufferPanel `internal`/`if:` group-level conditional display per variable (`spec.json:160` `if:`/`order` per variable/group) not present in Pterodactyl model.

---

## 4. Broken / incorrect logic (FORGE LOGIC FINDINGS — 22 unique across 5 runlets)

Consolidated and de-duplicated; all file:line verified.

**P0/P1 — power/install/reinstall/lifecycle fencing:**
- **F-G-01 — Install/kill race vs suspend snapshot.** `beacon/manager.go:693` `syncServerStateFromPanel` loads `state.Suspended` under lock then races `PanelURL` fetch without fence; `store_state.go:40-42` `observed_generation` only advances when desired==actual active overlap, leaving `crashed`+desired running in reconciler loop (`reconciler/service.go:476-493`). (subagent01 LF-01/LF-02)
- **F-G-02 — HardDeleteServer allocation ordering / FK leak.** `store_servers_lifecycle.go:43-46` sets `server_id=NULL` on allocations *after* deleting server row in non-Tx order; under FK `SET NULL` race `HardDeleteServer` can violate if allocations checked after. (subagent01 LF-03)
- **F-G-03 — Hash race `docker.go:195-203` + generation fence gap.** `CreateRequest` `configHashLabel:899` sha256 of CreateRequest computed without generation seed; two Create calls with same spec but different `desired_generation` collapse to same hash and skip reconciliation. (subagent01 LF-04)
- **F-G-04 — Reinstall clobbers suspended while installing.** `store_servers_control.go:217` `installing→installed` writes `status='stopped'` unconditionally even if `suspended=true`; badge shows `stopped`+suspended dual truth vs Pterodactyl `status=suspended` persistence. (subagent01 duplicate of C08)
- **F-G-05 — Beacon 30s blocking `server.go:1380` vs Forge 202 queue divergence.** Beacon `WaitForStop` 10m vs Forge operation timeout 5m reaper (also Phase 1 F-18/F-19) — kill bypass lock divergence (Phase 2 C05 wings bypass `powerLock` for `kill`, Forge queues kill behind stuck op; also `beacon/manager.go:487` `RunningAction` same slot, no pierce).
- **F-G-06 — `restoring_backup` not set as server lock.** `handlers_servers.go:2104` marks `backups.status=restoring` not `servers.actual_state=restoring_backup`; `ensureTransferIdle:145` guards transfer only, not `restoring` nor `installing`. `POST /power start` concurrent with restore allowed, diverging from `validateCurrentState:396` and `IsRestoring` guards in all 4 refs. (subagent01 C10, severity High)
- **F-G-07 — Reinstall not supported on Docker runtime but advertised.** `clustermanager/service.go:242-243` gating `Reinstaller` interface makes pure-Docker Reinstall always fail 500, while beacon `server.go:1323` reinstall works (just calls install) — drift API vs Beacon. (subagent01 C07 HIGH)

**Templates/variables/allocations/resources:**
- **F-G-08 — Regex `validateVariableValue` delimiter bug** (HIGH). `store_egg_variables.go:177` `regexp.Compile("/^([\\w…])(\\.jar)$/")` includes slashes → `server.jar` fails; `Split("|")` also splits regex alternation `|` inside pattern. Blocks PTDL_v2 import of stock Paper eggs (`minecraft-paper.json:58`). (subagent02 LF-01 — load-bearing)
- **F-G-09 — CPU `cpu` vs `cpu_shares` vs `CPUPercent` conflation** (HIGH). `store_servers.go:209` allows `CPULimit=0` (unlimited) with `CPUShares=1024` still set; beacon `docker.go:924` caps `CPUPercent 0..100000` but store caps `CPUShares 2..262144` — unlimited CPU still sets weight `1024` diverging from `settings.go:124` conditional. (subagent02 LF-02)
- **F-G-10 — `user_viewable` gate leaks/hides** (MEDIUM). `store_servers_control.go:154` injects all variables into provision target, while `store_startup.go:58` rejects non-viewable updates, and `store_servers.go:273` CreateServer allows any variable regardless of viewable — non-admin can set `DL_PATH` at provision to hijack installer URL, not later. (subagent02 LF-03)
- **F-G-11 — Allocation `container_port` lost in Wings compat `Mappings`** (MEDIUM). `Allocations.Mappings map[string][]int` fallback encodes both tcp+udp via `Bindings()`; Forge explicit `protocol` dropped when using legacy path → UDP-only `8211` also binds tcp. (subagent02 LF-04)
- **F-G-12 — `{{server.build.default.port}}` inert** (LOW). `store_servers_control.go:178` `resolveStartupCommand` only resolves `{{VAR}}` from Environment map, not `{{server.build.default.port}}` expected by `minecraft-paper.json:22` config patch `find:"server-port"`. Config-file parser `parser:1` not ported — Minecraft `server.properties` patch no-op. (subagent02 LF-05/C14)

**Nodes/daemon parity:**
- **F-G-13 — Overcommit scoring treats used<0 as 100** (`noderegistry/service.go:302` `if used<0 {used=0}`) — overcommitted node (`available>total`) scores perfectly healthy, defeats placement ranking. (subagent03 LF-1 Medium)
- **F-G-14 — `X-Beacon-Version` dev-bypass substring-fragile** (`server.go:1621` `strings.Contains(…,"dev-")` matches production `device-manager-…`). (subagent03 LF-2 Low)
- **F-G-15 — Heartbeat history window 5 items truncates recovery** (`heartbeatmonitor/service.go:205` limit `RecoveryThreshold+3=5` vs `classify:280` `!history[0].Success` guard). (subagent03 LF-3 Medium)
- **F-G-16 — Pre-restore snapshot metadata narrow** + SFTP sparse-file quota race (Info) — not load-bearing.

**Game UX (new in this phase, not lifecycle):**
- **F-G-17 — WS double-JSON parse drops plain console output** `websocket-manager.ts:109` tries `JSON.parse(text)` then `payload.data ?? payload.error ?? text` but on parse failure returns `text.split('\n')` else on parse success with `payload.error` truthy chooses error marker string even if data is plain text — plain output that happens to be valid JSON (e.g., `{}` from game) is swallowed as `data` then `payload.data` truthy but not string. (subagent04 LF-01 P1 SILENT)
- **F-G-18 — Network graph cumulative vs delta** `console-view.tsx:203` `network = rx+tx` monotonic, normalized `max = max(…values,1)` auto-scales flat history and ever-growing sum — visually dead / misleading. Reference `StatGraphs:61` deltas `tx-prev.tx`. (subagent04 LF-02 P1 SILENT — duplicates Phase 1 metricsChart delta sentiment but game-specific)
- **F-G-19 — Dual `transferring` source race** `transfer-view.tsx:77` `isTransferring = server.transferring` (cached) vs `transfer.transferring` (polled) — flicker between emerald/warning. (subagent04 LF-03 P1 SILENT, mirrors lifecycle C05)
- **F-G-20 — Synthetic per-line timestamps** `console-view.tsx:299` `<span>{new Date().toLocaleTimeString()}</span>` render-time clock per line, not source payload timestamp — toggling showTimestamps invents time per re-render. (subagent04 LF-04 P2 SILENT)
- **F-G-21 — Auto-max sparkline exaggeration** `console-view.tsx:92` `max = Math.max(...values,1)` — flat 2% vs 40% history look identical. (subagent04 LF-05 P2 SILENT)

**Tenancy/schedules/databases/mounts (Phase2-05):**
- **F-G-22 — Subuser `*` escalation** `store_users.go:297` `UpsertServerSubuser` only allowlist-checks `allowed[perm]||perm=="*"` without actor-subset check; `handlers_servers.go:541` checks `user.create` but not that requested `perms ⊆ actorPerms` (contrast `SubuserController:154 getDefaultPermissions` intersect + `websocket.connect` inject in Pterodactyl). Any `user.create` holder can grant `*` or arbitrary `database.view_password` etc. (subagent05 F-01 HIGH — load-bearing, new vs Phase 1 which had generic RBAC but not subuser-specific)
- **F-G-23 — Schedule task action asymmetry** `store_schedules.go:252` CreateScheduleTask checks `action != ""` only, while `PatchScheduleTask:331` validates `isValidScheduleTaskAction` (`power|backup|command` `store.go:1182`) — arbitrary action persists, only rejected at `schedule_runner:386`. Pollutes schedule store. (subagent05 F-02 MEDIUM)
- **F-G-24 — Mount reserved list too narrow** `store_mounts_ext.go:323` only blocks `source ∈ {/etc/forge,/var/lib/forge/volumes}` + `target ∈ {/,/home/container}`; leaves `/etc`, `/var/run/docker.sock`, `/proc`, `/`, `/root` mountable via compromised admin. Forge's daemon `AllowedMountSourcesForNode:367` is second gate but panel allowlist short. (subagent05 F-03 MEDIUM-HIGH duplicate of F-03 in Phase1 but escalation to host breakout)
- **F-G-25 — Server-scoped cron listing scans whole table then Go-filters** `handlers_user_console.go:315` `ListCronJobs` global then `for j … if TargetType!="server" continue` — O(N) overhead / timing leak vs SQL `WHERE target_type='server' AND target_id=$1`. (subagent05 F-04 INFO)
- **F-G-26 — `GetTeamMemberPermissions` OR-merge re-enables explicit false** `store_envvars.go:418` `if !perms.CanCreateProjects { perms.CanCreateProjects = defaultPerms… }` overwrites stored `false` indistinguishable from absent zero-value. (subagent05 F-05 LOW-MEDIUM)

---

## 5. Duplicates (update)

Add to MASTER_FINDING_INDEX: `REF-GAME-DUP-01` — template catalog three-way split (DB eggs vs FS game-templates vs localStorage app-templates) — confirms Phase 1 C11/C12. `REF-GAME-DUP-02` — config-file parser expected (`config.files` in `eggs.config` + `minecraft-paper.json:16`) is duplicate expectation: game-templates carry `config.files` inert, and PufferPanel's typed alters duplicate the expectation.

---

## 6. False completion / decorative (update)

- Buildpacks view `builds-view.tsx:110` Build button permanently `disabled` with title claiming executor absent — correctly honest but decorative; should hide or link to git/compose flows.
- Transfer `progress` field (`legacy` shim) may be null always — bar `width clamp(progress)` may never move despite `server.transferState` text changing (Phase2-04 C05).

---

## 7. Architecture lessons

| Pattern | Source | ADOPT / INSPIRE / REJECT |
|---------|--------|--------------------------|
| Desired/actual split with `observed_generation`/`desired_generation` + `state_transitions` audit | Forge addition (no ref had it) vs Pelican `ServerState` enum | **KEEP — adopt as canonical** — Pterodactyl single `status` is weaker |
| `suspended` boolean orthogonal to `status` vs Ptero `status='suspended'` wiping install state | Forge vs Panel | **KEEP Forge** — clarifies tri-state; add UI badge combo `stopped+suspended` |
| Beacon chown `1000:1000` on `MkdirAll` `server.go:1088` + read-only installer + capDrop | Forge vs Wings | **KEEP** |
| `HasSpaceAvailable` / `HasSpaceForWrite` pre-checks vs Wings `HasSpaceErr` post-check | Beacon vs Wings | **KEEP** |
| Typed install pipeline (26 ops with `if:` conditionals) vs single script blob | PufferPanel `spec.json:307` vs Forge single `install_script` | **INSPIRE for modded Minecraft** — keep Forge single-script for most games but consider typed pipeline for Fabric/CurseForge heavy installs |
| `config.files` parser (6 parsers + `configMatchRegex`) patching `server.properties` at boot | Wings `parser:1` vs Forge inert | **ADAPT** — either implement pre-start patch in `beacon/manager.go:onBeforeStart` or strip `config.files` from templates and document unsupported |
| `validateVariableValue` Laravel `ValidationFactory` vs hand-rolled `validateVariableValue` | Pterodactyl vs Forge | **ADAPT** — strip `regex:/…/` delimiters and respect `|` inside regex char classes, or reuse compiled Laravel-compatible validator |
| Multi-env `supportedEnvironments[host,docker]` per PufferPanel vs single Docker | PufferPanel `minecraft.json:398` vs Forge Docker-only | **REJECT** — host env unsupported intentionally |

---

## 8. Recommended activation order (existing capability → wired)

Without new subsystem (all within existing tables/services):

1. **P0 Fix `validateVariableValue` regex delimiters** `store_egg_variables.go:177` — unblocks all PTDL_v2 imports (F-G-08).
2. **P0 Gate subuser escalation** `store_users.go:297` subset check + `*` behind owner/admin — close vertical privilege boundary (F-G-22).
3. **P0/P1 Wire `restoring_backup` as server lock** `handlers_servers.go:2104` set `actual_state=restoring_backup` on restore, clear on completion; guard `power/install/reinstall` via `ensureRestoreIdle` alongside `ensureTransferIdle` — closes concurrent start/restore (F-G-06).
4. **P1 Harden mount allowlist** `store_mounts_ext.go:323` switch to `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` or deny `/etc|/proc|/sys|/dev|/var/run` — mitigates host breakout (F-G-24).
5. **P1 Fix `CreateScheduleTask` action validation symmetry** `store_schedules.go:252` call `isValidScheduleTaskAction` — remove invalid persistence (F-G-23).
6. **P1 Fix allocation/CPU conflation determinism** `store_servers.go:209`/`docker.go:901` — align `cpu` percent vs `cpu_shares` weight semantics, gate weight on `CPULimit>0` like wings `settings.go:124`.
7. **P2 Fix WS/network/sparkline fidelity** `websocket-manager.ts:109` plain output handling, `console-view.tsx:203` delta `tx-prev.tx`, `92` fixed max vs limit — restores operator truth (F-G-18-21).
8. **P2 Unify templates** — seed `game-templates/*.json` into eggs on `DefaultSeeder` or expose DB-backed import path; deprecate localStorage drift (F-G hidden 03).
9. **P3 Fix heartbeat history window 5** → `max(RecoveryThreshold*3,20)` and align `LastSeenAt` vs `ObservedAt` (F-G-15).
10. **P3 Move `handlers_user_console.go:315` filter into SQL** — capping O(N) scan.
11. **P3 Reject reinstall when running more clearly** — make `POST /servers/:id/reinstall` 409 message instruct `stop first` consistently (like beacon does) rather than marking `install_failed`.
12. **P2/P3 Merge transfer single source** `transfer-view.tsx:31` derive `isTransferring` from `transfer?.transferring ?? server.transferring` with invalidate on change; remove flicker (F-G-19).

---

## 9. Handoff to Phase 3 (Backup)

Phase 1 already flagged `backup/restore` worker divergence and preview/git plumbing; Phase 2 consolidates server `restoring_backup` and heartbeatmonitor gaps. Phase 3 (Kopia/Restic vs `services/backup` + `beacon/internal/backup`) should avoid repeating lifecycle fencing and focus on chunked deduplication, storage atomicity, verification, and retention semantics that Phase 1/2 did not go deep on.

