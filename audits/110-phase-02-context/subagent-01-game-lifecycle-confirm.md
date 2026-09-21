# 110-02-01 — Phase 02 Context & Confirmation — Agent 01/10 — Game Hosting Lifecycle Slice

**Agent:** 110-02-01 (Phase 02 Context & Confirmation, Agent 01 of 10 — parallel)  
**Focus:** Game Hosting Lifecycle slice (GH-01..GH-19 + REF-GAME-* + F-G-*)  
**Date:** 2026-08-24 (live disk verification)  
**Workspace:** `/Users/riyaz/project/gamepanel` HEAD `ca06f74` + dirty working tree (unstaged, see §7)  
**Method:** Direct `read`+`grep` file:line SOURCE_VERIFIED on both trees (no trust of prior audits). Every prior P0/P1 re-checked against live `beacon` + `forge/api` code now on disk. Zero product code modified.  
**Prior audits reconciled:**
- `audits/FINAL_PARITY_AUDIT.md` §2 GH-01..GH-19 (2026-08-24)
- `audits/phase-02/synthesis.md` F-G-01..F-G-26
- `audits/phase-02/subagent-01-game-lifecycle.md` C01..C13
- `audits/final-parity/subagent-01-game-hosting.md` C-01..C-17
- `audits/MASTER_FINDING_INDEX.md` REF-GAME-* (C01, F-G-06..F-G-24, HIDDEN/DUP)
- `audits/reverification/subagent-01-game-lifecycle.md` (15 rows, 2026-08-24)
- `audits/reverification/synthesis.md` (20 subagents)

**Live files forcibly inspected per task:**
`beacon/internal/server/manager.go:487`, `beacon/internal/server/server.go:1323`, `forge/api/internal/services/clustermanager/service.go:241`, `forge/api/internal/store/store_egg_variables.go:176`, `forge/api/internal/store/store_users.go:297`, `forge/api/internal/http/handlers_servers.go:2104`, `forge/api/internal/store/store_mounts_ext.go:323` + ancillary `store_state.go:39`, `store_servers_control.go:44`, `store_schedules.go:252`, `store_startup.go:31`

---

## 0. Executive Summary — Verdict on This Slice

**No lifecycle P0 was fixed since final-parity/reverification.** Of 19 GH rows, 6 COMPLETE/COMPLETE+ remain COMPLETE, 1 PARTIAL stays PARTIAL, 2 MISSING stay MISSING, 4 BROKEN stay BROKEN, 1 DUPLICATE stays DUPLICATE, and 2 prior COMPLETE+ are confirmed as **overstated** and downgraded to PARTIAL on live re-inspection (GH-03, GH-10). The 4 load-bearing P0s in this slice (GH-14 slash bug, GH-18/* escalation, GH-19 mount breakout, GH-09 restoring-lock race) are **STILL BROKEN/MISSING** on live HEAD with unchanged proof lines. The only delta between HEAD `ca06f74` and the dirty working tree for these 7 paths is **cosmetic audit-JSON hardening** (`mustAuditJSON` vs `Sprintf`) and a **new install fencing helper** (`BeginInstall`/`EndInstall`) that *does not* close the reviewed gaps (kill still does not pierce, restoring still unwired, regex still literal, wildcard still persistable). Reverification synthesis conclusion **"~4 edge fixes, none structural P0"** is confirmed for this slice: **0 of the 7 mandated loci is VERIFIED_FIXED**.

---

## 1. GH-01..GH-19 — FINAL_PARITY_AUDIT §2 vs LIVE (file:line evidence)

| GH | Capability | Prior status (FINAL §2) | Prior severity | LIVE status | Delta | Live evidence (file:line) that proves status |
|---|---|---|---|---|---|---|
| **GH-01** | Server create `provisioning→created` distinct from `installing` | COMPLETE (better) | — | **VERIFIED_FIXED / COMPLETE** | CONFIRMED | `forge/api/internal/store/store_servers.go:253` `INSERT ... 'provisioning','stopped','stopped'`; `forge/api/internal/store/store_servers_control.go:207` `SetServerProvisioned:208` `status='created', actual_state='stopped', installed=false`; `forge/api/docs/server-lifecycle.md:5` documents provisioning never exposed to runtime. No drift vs prior. |
| **GH-02** | Desired/actual state machine + generations | COMPLETE+ | — | **VERIFIED_FIXED / COMPLETE+** | CONFIRMED, with known livelock note | `forge/api/internal/store/store_state.go:10` `SetServerDesiredState` increments `desired_generation` on distinct; `store_state.go:31` `SetServerActualState:39` `observed_generation = CASE WHEN (desired='running' AND actual='running') OR (desired='stopped' AND actual='stopped') THEN desired_generation ELSE observed_generation END`; `store_state.go:120` `recordStateTransition` audits `state_transitions`; `forge/api/internal/services/reconciler/service.go:476` `reconcileServer` checks `observed_generation`. Superset intact; `crashed`+desired `running` livelock (F-G-01/C-02-LF) reproduced via `store_state.go:40` fence. |
| **GH-03** | Suspend/unsuspend orthogonal boolean vs enum wipe | COMPLETE+ (FINAL overstated) | — | **STILL BROKEN → DOWNGRADED to PARTIAL** | CONFIRMED downgrade already in `final-parity C-03` + `reverification GH-03` | `forge/api/internal/http/handlers_servers.go:1413` `POST /suspension` best-effort `Daemon.SendPower stop` + `store.SetServerSuspended:1444` **without** `ensureTransferIdle:145`; `forge/api/internal/store/store_servers.go:374` raw `SetServerSuspension` + `385` CAS exist but handler bypasses; `beacon/internal/server/manager.go:271` `BeginInstall:280` checks `Suspended` under TryLock but `manager.go:610` `onBeforeStart:628` snapshots `Suspended` then releases lock before panel sync `689` `syncServerStateFromPanel:748` — concurrent `HandlePower start` interleaves stale `false`. GAP confirmed; FIX would require `ensureTransferIdle` on `/suspension` + fence. |
| **GH-04** | Power (start/stop/restart/kill) durable queue | PARTIAL | — | **STILL BROKEN → PARTIAL** | CONFIRMED | `forge/api/internal/http/handlers_servers.go:888` `POST /servers/:id/power` → `ensureTransferIdle:889` (transfer only) → `OperationService.DispatchPower:915` returns `202`; `forge/api/internal/store/store_servers_control.go:44` `powerSignalPriorStates:51` allows `stop/kill` during `installing` (intentional diverge); `beacon/internal/server/manager.go:482` `HandlePower:487` `TryLock` + `490` `RunningAction!=""` + `494` `InstallationState==installing → "server is installing"` blocks `start` during install. API enqueues doomed `start` during `installing` as `202` then Beacon rejects → 3× retry then `failed` (Wings would 409 sync). Semantics unchanged. |
| **GH-05** | Kill pierces stuck lock | MISSING | P1 | **STILL BROKEN → MISSING** | CONFIRMED, not fixed | **Mandated locus** `beacon/internal/server/manager.go:487` `if !state.mu.TryLock() → 488 "another server action is already running"` + `490 if state.RunningAction!="" → 491 same` + `499 state.RunningAction=signal` — kill `manager.go:581` `case "kill":582` claims **same** `RunningAction` slot (no bypass); `beacon/internal/runtime/docker.go:494` `Kill` uses sharded `workloadLock:251` `[64]sync.Mutex` by `sha256(serverID)[0]` — still contends vs `Stop/Start`. Wings `power.go:108` `TryAcquire` + ignore-failure for `Terminate` has **no equivalent**. `operation/service.go:368` kill queued FIFO behind stuck install. Live grep: `HandlePower:487` same slot, no `signal==kill` branch. |
| **GH-06** | Install fencing (installer `1000:1000` readonly+capDrop, admin-only WS) | COMPLETE+ | — | **VERIFIED_FIXED / COMPLETE+** | CONFIRMED | `beacon/internal/server/server.go:1052` `install:1078` `MkdirAll` + `1116` `BeginInstall` claim + `1122` `runtime.Install:256` `beacon/internal/runtime/docker.go:256` creates `mgp-{id}-installer` `User 1000:1000:285` `ReadonlyRootfs:true:302` `CapDrop:ALL:299` `no-new-privileges:true:303` `Tmpfs /tmp 64M:305` pulls `alpine:3.21:262` caps log 1MiB `340`; `server.go:1159` `installWS:1178` `if claims.Scope != tokens.ScopeAdmin → 403`. Security stricter than Wings `install.go:33`. Dirty-tree addition `manager.go:271` `BeginInstall`/`298` `EndInstall` hardens claim but does not change status. |
| **GH-07** | Reinstall requires stopped / Docker gate | BROKEN | P1 | **STILL BROKEN** | CONFIRMED, not fixed | **Mandated locus** `forge/api/internal/services/clustermanager/service.go:241` `reinstaller, ok := s.runtime.(gpruntime.Reinstaller) → 242 if !ok { err = errors.New("runtime does not support reinstall")` → marks `failed` via `store.SetServerInstallState:251`. Pure Docker runtime never implements `Reinstaller` → always 500, while **Beacon** `beacon/internal/server/server.go:1331` `reinstall:1335` `if PowerState==Running|Starting → 409 "server must be stopped"` then forwards to `install` (works). API vs Beacon drift unchanged. Prior expected fix (fallback `InstallServer`) not applied. |
| **GH-08** | Delete hard-delete + orphan remediation | COMPLETE+ | — | **VERIFIED_FIXED / COMPLETE+** | CONFIRMED | `forge/api/docs/server-lifecycle.md:6` Beacon-first; `forge/api/internal/services/clustermanager/service.go:332` `DeleteServer:343` `runtime.DeleteServer` → on err+`!force` return, on err+`force` → `RecordOrphanAndHardDeleteServer:348` `Mode:force`; on success → `HardDeleteServer:355`; `forge/api/internal/store/store_servers_lifecycle.go:12` `HardDeleteServer:46` `UPDATE allocations SET server_id=NULL` then `DELETE FROM servers`; `17` `RecordOrphanAndHardDeleteServer` inserts `server_orphan_remediations` before delete (FK-free, requires `daemonError!=""`). `beacon/internal/server/server.go:1433` `delete:1445` `runtime.Delete:625` `ContainerRemove Force+RemoveVolumes` idempotent. Novel table absent in refs, correctly enhanced. |
| **GH-09** | `restoring_backup` as server lock | MISSING (P1) | P1 | **STILL BROKEN → MISSING** | CONFIRMED, not fixed | **Mandated locus** `forge/api/internal/http/handlers_servers.go:2117` `POST /servers/:id/backups/restore:2146` checks `status==completed` then `DispatchBackupRestore:2155` **or** sync `MarkBackupStatus restoring:2170` (`backups.status=restoring` only); **never** `SetServerActualState:31` `serverStatusFromActual restoring_backup → restoring_backup` (`store_state.go:137`). `beacon/internal/server/manager.go:30` `ServerState{PowerState, InstallationState, StartupState, RunningAction ...}` has **no** `Restoring` field; `beacon/internal/server/server.go:1699` `restoreBackup` holds `backupMu` but no server-level lock. `RequestServerPower:382` checks `Suspended` not `restoring`. Concurrent `POST /power start` during restore allowed → race `RootDir` half-written extract vs running process. `operation/service.go:28` `OpBackupRestore` handler `main.go:603` also only `MarkBackupStatus restoring/restored` (`main.go:564`), never drives server state. |
| **GH-10** | Transfer/migration v1 resumable chunks | COMPLETE+ (FINAL overstated) | — | **STILL BROKEN → DOWNGRADED to PARTIAL** | CONFIRMED downgrade already in `final-parity C-10` + `reverification GH-10` | `forge/api/docs/server-lifecycle.md:12` claims transfer archival `501` yet `forge/api/internal/http/handlers_servers.go:713` `POST /servers/:id/transfer` returns `202` via `migration/service.go:127` `CreateMigration+ExecuteMigration`; `GET /transfer:740` returns legacy `servers.transfer_state/transferring/targetNodeId` not `migrations.status`; `forge/api/internal/store/store_servers.go:346` `IsServerTransferBlocking` reads stale `servers.transfer_state` not `migrations` table (`store_migrations.go` lease `migration_runs` `TransferProtocolVersion=v1`). `syncServerStateFromPanel`-like projection missing → `ensureTransferIdle:145` misses active migration; `transfer-view.tsx:77` flicker `server.transferring` (cached) vs `transfer.transferring` (polled) persists. Control-plane v1 `beacon/transfer/protocol.go` dual-credential superior to Wings JWT push but wiring incomplete. |
| **GH-11** | Allocations (`containerPort`/`protocol`) | COMPLETE+ | — | **VERIFIED_FIXED / COMPLETE+ (with residual compat leak)** | CONFIRMED, residual noted | `forge/api/internal/store/store_allocations.go:15` `Allocation{IP,Port,ContainerPort,Protocol}` `CreateAllocations:125` validates `net.ParseIP` + `protocol∈{tcp,udp}` + `containerPort default port:162`; `forge/api/internal/runtime/docker.go:865` `dockerPorts` validates `tcp|udp` duplicate `HostIP:HostPort/Protocol:888`; `clustermanager/service.go:655` `runtimeCreateRequest` maps `ContainerPort 0→HostPort` correctly. **Residual:** legacy fallback `clustermanager/service.go:698` `mappings map[string][]int` + `client.go:460` drops `protocol` → UDP-only `8211` also binds tcp (F-G-11). DB unique index may be `(node_id,ip,port)` without `protocol` → `23505` on tcp+udp same port. Superset vs `wings/environment/allocations.go:36` `Mappings map[string][]int` dual via `Bindings()`. |
| **GH-12** | Allocations kill/start race vs install (API allows start during installing) | BROKEN | — | **STILL BROKEN** | CONFIRMED | `forge/api/internal/store/store_servers_control.go:51` allows `stop/kill` during `installing` (correct) but `start:[created,stopped,install_failed]` correctly excludes `installing`; however `forge/api/internal/http/handlers_servers.go:888` only `ensureTransferIdle:145` guards transfer, **not** installing/restoring — so `POST /power start` during `installing` passes API as `202`, enqueues, Beacon `manager.go:494` rejects → retry storm `operation/service.go:229` 3 attempts then `failed`. Wings `power.go:57` would 409 `ErrServerIsInstalling`. Fast 409 guard missing. |
| **GH-13** | Egg/nest/egg_variables model | PARTIAL | — | **PARTIAL (still)** | CONFIRMED | `forge/api/internal/store/store_nests.go:16` Nest + `34` Egg + `store_egg_variables.go:17` EggVariable; `store_nests.go:34` `CreateEgg` via `normalizeDockerImages:361` handles `map<string,string>` or legacy `[]string` (robust). Valid but GH-14 slash bug makes it PARTIAL. `normalizeDockerImages` fallback `store_nests.go:99` `COALESCE(NULLIF(s.docker_image,''), SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1)` is correct. `DeleteNest` race (SELECT COUNT without FOR UPDATE) remains low. |
| **GH-14** | Variable validation regex slash bug | BROKEN (P0) | P0 | **STILL BROKEN (P0)** | CONFIRMED, not fixed | **Mandated locus** `forge/api/internal/store/store_egg_variables.go:176` `case "regex": pattern, err := regexp.Compile(arg)` where `arg` is post-`Cut(rule,":")` of `regex:/^([\w\d._-]+)(\.jar)$/` → `arg=/^([\w\d._-]+)(\.jar)$/` includes surrounding `/` literal + `store_egg_variables.go:143` `strings.Split(rules,"|")` splits regex alternation `|` inside pattern (e.g., `regex:/^(foo|bar)$/`). Import via `handlers_admin.go:1308` `POST /eggs/import` loop `CreateEggVariable` → 400 blocks Paper/Vanilla PTDL_v2 (`minecraft-paper.json:58`). **Dirty-tree diff** only changed `Sprintf` → `mustAuditJSON`, not regex logic (see `git diff HEAD -- store_egg_variables.go:77` 3 lines audit only). Prior expected fix (strip delimiters + handle `|` inside char class) not applied. |
| **GH-15** | Config parser 6 parsers (file/yaml/properties/ini/json/xml) | MISSING | — | **STILL BROKEN → MISSING** | CONFIRMED, not fixed | `forge/api/internal/store/store_servers_control.go:95` `e.config::text` stored as `ConfigJSON` (`store_nests.go:231` normalized) never parser-validated; `beacon/internal/server/server.go:844` `syncConfiguration:869` `applyConfigurationFiles:3254` renders `config.files` via `renderTemplate:60` but only `{{VAR}}`/`{{env.VAR}}` (`server.go:3262`) not `{{server.build.default.port}}` expected by `minecraft-paper.json:22` + `configMatchRegex`. No `wings/parser/parser.go:1` port. `store_schedules.go:14` `resolveStartupCommand` only `{{VAR}}`/`{{lower}}`. `server.properties` patch no-op → Minecraft wrong port. `beacon/server.go:3254` applies `config.files` simplistic content/properties/json only, not yaml/properties/ini/xml parsers. |
| **GH-16** | Template systems (DB eggs vs FS 14 vs localStorage 5) | DUPLICATE | — | **STILL BROKEN → DUPLICATE** | CONFIRMED | DB `migrations/091_seed_minecraft_java.sql` 1 egg seeded (`091`) vs `forge/web/lib/egg-templates.ts:27` 14 static entries hard-coded TS vs `forge/web/lib/app-templates-data.ts:3` `STORAGE_KEY forge.app-templates.v1` 5 localStorage vs `packages/game-templates/templates/*.json` 14 curated (present on worktree branch, absent on `main` HEAD `ls packages → sdk, shared-types`). `store_templates.go:12` shim `templateFromEgg:99` no seed from FS. Fresh install 1 egg not 14 → 93% seeding deficit. `catalog/catalog.go` isolated from startup. |
| **GH-17** | Schedules/cron tasks `isValid` asymmetry | PARTIAL | — | **STILL BROKEN → PARTIAL** | CONFIRMED, not fixed | **Locus** `forge/api/internal/store/store_schedules.go:252` `CreateScheduleTask:253` `if strings.TrimSpace(req.Action)==""` only checks non-empty, **does not** call `isValidScheduleTaskAction` (`store.go:1196` valid `power|backup|command`); `store_schedules.go:331` `PatchScheduleTask:332` **does** validate `332` `if req.Action!=nil && !isValidScheduleTaskAction(*req.Action) → 333 unsupported`. Asymmetry persists → arbitrary `action` persistable (e.g., `rm -rf`) rejected only at `schedule_runner:386` (`unsupported task action`). `handlers_user_console.go:315` O(N) scan vs SQL filter remains. |
| **GH-18** | Subusers/permissions wildcard `*` | BROKEN (P0) | P0 | **STILL BROKEN (P0)** | CONFIRMED, not fixed | **Mandated loci** `forge/api/internal/store/store_users.go:297` `UpsertServerSubuser:311` `permissions := normalizeSubuserPermissions(req.Permissions)` + `store_users.go:555` `normalizeSubuserPermissions:564` `if permission=="" || seen[permission] || (!allowed[permission] && permission != "*") {continue}` — **`*` explicitly allowed** for any caller; `store_users.go:564` allowlist check only, **no actor-subset check** `perms ⊆ actorPerms` + `handlers_servers.go:554` `POST /servers/:id/users` checks `requireServerPermission(cfg, PermUserCreate)` (`541`) but not that requested `perms ⊆ actorPerms`. Any `user.create` holder can grant `*` or `database.view_password` etc. Wings `GetUserPermissionsService.php:18` computes `*` only for `root_admin`/`owner_id===user.id`, never stored; `SubuserController:154` intersects + injects `websocket.connect`. Live grep `UpsertServerSubuser:306` no actor param beyond `actorID` audit. |
| **GH-19** | Mount allowlist / file denylist | BROKEN | P0 | **STILL BROKEN (P0 host breakout)** | CONFIRMED, not fixed | **Mandated locus** `forge/api/internal/store/store_mounts_ext.go:323` `validateMountPath:332` `if field=="source" && (value=="/etc/forge" || value=="/var/lib/forge/volumes")` + `335 if field=="target" && (value=="/" || value=="/home/container")` — only blocks 2 sources +2 targets; misses `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/sys`, `/`, `/root`, `/boot`. `store_mounts_ext.go:367` `AllowedMountSourcesForNode` returns distinct `m.source` but panel allowlist short; daemon `server.go:127` `SetAllowedMounts` + `mounts.go:13` `runtimeMounts EvalSymlinks+Rel` is second gate but panel short. Dirty-tree diff on this file is only audit JSON (3 lines). Prior expected fix (deny `/etc|/proc|/sys|/dev|/var/run` or allowlist prefix `/srv/forge-mounts`) not applied. |

---

## 2. MASTER_FINDING_INDEX — REF-GAME-* vs LIVE

| ID | Phase | Severity | Kind | Title | First source | Live re-verification | Evidence now |
|---|---|---|---|---|---|---|---|
| **REF-GAME-C01** | Phase2 | P1 | PARTIAL | Install/kill/suspend fencing races | `beacon/manager.go:693`, `store_state.go:40` | **STILL BROKEN** | `manager.go:693` `syncServerStateFromPanel` loads `Suspended` at `611` under lock then releases, fetches `PanelURL` without fence, re-acquires at `748` → concurrent `HandlePower start` interleaves stale `false`; `store_state.go:40` fence never converges for `crashed`. |
| **REF-GAME-F-G-06** | Phase2 | HIGH | MISSING | `restoring_backup` not set as server lock | `handlers_servers.go:2104` | **STILL BROKEN → MISSING** | `handlers_servers.go:2117` `MarkBackupStatus restoring` not `SetServerActualState restoring_backup`; `ensureTransferIdle:145` guards transfer only. |
| **REF-GAME-F-G-07** | Phase2 | HIGH | BROKEN | Reinstall fails on Docker runtime gate | `clustermanager/service.go:242` | **STILL BROKEN** | `service.go:241` `Reinstaller` iface gate → 500 on Docker; beacon `server.go:1331` works. |
| **REF-GAME-F-G-08** | Phase2 | HIGH | BROKEN | Regex delimiter in `validateVariableValue` blocks egg imports | `store_egg_variables.go:177` | **STILL BROKEN P0** | `store_egg_variables.go:176` `regexp.Compile(arg)` literal slashes; `143` `Split("|")` splits alternation. |
| **REF-GAME-F-G-09** | Phase2 | HIGH | BROKEN | CPU `cpu_shares` vs `CPUPercent` conflation | `store_servers.go:209`, `beacon/docker.go:924` | **STILL BROKEN** | `store_servers.go:209` allows `CPULimit=0` unlimited with `CPUShares=1024` still set; beacon `docker.go:924` caps `CPUPercent 0..100000`, store caps `CPUShares 2..262144`; unlimited still sets weight diverging from `wings/environment/settings.go:124` conditional `if CpuLimit>0 then CPUShares else 0`. |
| **REF-GAME-F-G-10** | Phase2 | MED | BROKEN | `user_viewable` gate leak on `CreateServer` | `store_servers_control.go:154` vs `store_startup.go:58` | **STILL BROKEN** | `store_servers_control.go:154` `ServerProvisionTarget:154` injects **all** variables `JOIN egg_variables` without `user_viewable` filter, while `store_startup.go:31` `WHERE ev.user_viewable=true` filters startup view and `store_startup.go:58` rejects non-viewable updates; `store_servers.go:273` `CreateServer` allows any variable regardless of viewable → non-admin can set `DL_PATH` at provision to hijack installer URL. |
| **REF-GAME-F-G-13** | Phase2 | MED | BROKEN | Overcommit scoring healthy | `noderegistry/service.go:302` | **STILL BROKEN** | `noderegistry/service.go:302` `if used<0 {used=0}` — overcommitted node (`available>total`) scores perfectly healthy; placement ranking defeated (not directly in mandated 7 but confirmed via `final-parity`). |
| **REF-GAME-F-G-15** | Phase2 | MED | BROKEN | Heartbeat history 5 truncation misclassifies recovery | `heartbeatmonitor/service.go:205` | **STILL BROKEN** | `heartbeatmonitor/service.go:205` limit `RecoveryThreshold+3=5` vs `classify:280` `!history[0].Success` guard truncates recovery window. |
| **REF-GAME-F-G-17** | Phase2 | P1 | BROKEN | WS double JSON parse drops plain output | `ws/websocket-manager.ts:109` | **STILL BROKEN** (now partially mitigated) | File `forge/web/lib/api/ws/websocket-manager.ts` **deleted** in dirty tree (`git diff HEAD --stat` shows `ws/websocket-manager.ts | 41 -` removal) but no replacement wired; prior bug was `JSON.parse(text)` then `payload.data ?? payload.error ?? text` swallowing plain `{}`; live deletion without fix keeps console path via `consoleWS` (`server.go:1962`) — not game-lifecycle slice. |
| **REF-GAME-F-G-18** | Phase2 | P1 | BROKEN | Network graph cumulative not delta | `console-view.tsx:203` | **STILL BROKEN** | `forge/web/components/server/console-view.tsx:203` `network = rx+tx` monotonic `max = max(...values,1)` auto-scales flat history (reference `StatGraphs:61` deltas `tx-prev.tx`). |
| **REF-GAME-F-G-19** | Phase2 | P1 | BROKEN | Transfer dual source race flicker | `transfer-view.tsx:77` | **STILL BROKEN** | `transfer-view.tsx:77` `isTransferring = server.transferring` (cached) vs `transfer.transferring` (polled) flicker. |
| **REF-GAME-F-G-22** | Phase2 | HIGH | BROKEN | Subuser `*` escalation (any `user.create` → `*`) | `store_users.go:297` | **STILL BROKEN P0** | `store_users.go:306` `UpsertServerSubuser` no actor-subset; `normalizeSubuserPermissions:555` allows `*`. See GH-18. |
| **REF-GAME-F-G-23** | Phase2 | MED | BROKEN | `CreateScheduleTask` missing `isValidScheduleTaskAction` | `store_schedules.go:252` | **STILL BROKEN** | `store_schedules.go:252` only `action!=""` vs `Patch:332` validates; arbitrary action persists. |
| **REF-GAME-F-G-24** | Phase2 | MED | BROKEN | Mount allowlist too narrow (host breakout) | `store_mounts_ext.go:323` | **STILL BROKEN P0** | `store_mounts_ext.go:323` only 2 sources blocked; see GH-19. |

Additional MASTER entries cross-referenced in this slice (not separately tracked but live-confirmed):
- `REF-GAME-HIDDEN-01` 6-step workflow `installer/service.go:66` persists rows never executed — still DEAD (`grep` zero callers beyond persistence).
- `REF-GAME-HIDDEN-02` transfer dual state `servers.transfer_state` vs `migrations` — still PARTIAL.
- `REF-GAME-DUP-01` template triplicate — still DUPLICATE.

No NEWLY_FIXED rows found on this lifecycle surface; reverification note **"No evidence of `git diff` fixes between 2026-08-24 audits and current HEAD on these paths"** holds for the 7 mandated loci (only audit-JSON cosmetics).

---

## 3. Phase-02 Synthesis — F-G-01..F-G-26 Consolidated vs LIVE

| F-G | From synthesis §4 | Severity | Live status | Proof / note |
|---|---|---|---|---|
| F-G-01 | Install/kill race vs suspend snapshot `manager.go:693` + `store_state.go:40` observed_generation livelock | HIGH | **STILL BROKEN** | `manager.go:689` `syncServerStateFromPanel` race reproduced; `store_state.go:40` `crashed` never converges. |
| F-G-02 | HardDeleteServer allocation ordering / FK leak `store_servers_lifecycle.go:43` | LOW | **STILL BROKEN (benign)** | Ordering `UPDATE allocations SET server_id=NULL` before `DELETE FROM servers` correct inside Tx but `primary_allocation_id=NULL` even if already NULL — benign, no FK violation. |
| F-G-03 | Hash race `docker.go:195` + generation fence gap | LOW | **STILL BROKEN** | `docker.go:193` `createRequestHash` `1016` hashes `CreateRequest` without `desired_generation` seed. |
| F-G-04 | Reinstall clobbers suspended while installing `store_servers_control.go:217` | LOW | **STILL BROKEN** | `installing→installed` writes `status='stopped'` unconditionally even if `suspended=true` → `stopped+suspended` dual truth. |
| F-G-05 | Beacon 30s blocking vs Forge 202 divergence `server.go:1380` / reaper 5m | MED | **STILL BROKEN** | `beacon/server.go:1357` `power:1389` `waitCtx 30s` polling `GetStatus` vs `operation/service.go:152` reaper 5m; kill divergence persists. |
| F-G-06 | `restoring_backup` not wired | HIGH | **STILL BROKEN** | See GH-09. |
| F-G-07 | Docker reinstall gate | HIGH | **STILL BROKEN** | See GH-07. |
| F-G-08 | Regex slash | HIGH P0 | **STILL BROKEN P0** | See GH-14. |
| F-G-09 | CPU conflation | HIGH | **STILL BROKEN** | See REF-GAME-F-G-09. |
| F-G-10 | `user_viewable` gate | MED | **STILL BROKEN** | See REF-GAME-F-G-10. |
| F-G-11 | `container_port` lost in `Mappings` compat | MED | **STILL BROKEN** | See GH-11 residual. |
| F-G-12 | `{{server.build.default.port}}` inert | LOW | **STILL BROKEN** | `store_schedules.go:14` `resolveStartupCommand` only `{{VAR}}`; `server.go:3254` `applyConfigurationFiles` only `{{VAR}}`/`{{env.VAR}}`. |
| F-G-13 | Overcommit scoring | MED | **STILL BROKEN** | See REF-GAME-F-G-13. |
| F-G-14 | `X-Beacon-Version` dev-bypass substring | LOW | **STILL BROKEN** | `server.go:1621` `strings.Contains(...,"dev-")` matches production `device-manager-...`. |
| F-G-15 | Heartbeat history 5 truncation | MED | **STILL BROKEN** | See REF-GAME-F-G-15. |
| F-G-16 | Pre-restore snapshot narrow + SFTP sparse-file quota race | INFO | **STILL BROKEN** | Not load-bearing. |
| F-G-17..F-G-21 | WS double parse, network cumulative, transfer flicker, synthetic timestamps, sparkline auto-max | P1/P2 | **STILL BROKEN** | Game-UX gaps; `ws/websocket-manager.ts` deletion does not fix. |
| F-G-22 | Subuser `*` escalation | HIGH P0 | **STILL BROKEN P0** | See GH-18. |
| F-G-23 | Schedule action asymmetry | MED | **STILL BROKEN** | See GH-17. |
| F-G-24 | Mount allowlist narrow | MED-HIGH P0 | **STILL BROKEN P0** | See GH-19. |
| F-G-25 | Server-scoped cron O(N) scan `handlers_user_console.go:315` | INFO | **STILL BROKEN** | `ListCronJobs` global then Go-filter vs SQL `WHERE target_type='server' AND target_id=$1`. |
| F-G-26 | `GetTeamMemberPermissions` OR-merge re-enables explicit false `store_envvars.go:418` | LOW-MED | **STILL BROKEN** | Zero-value overwrite. |

---

## 4. Mandated 7 Loci — Deep File:line Inspection (this task's explicit requirement)

### 4.1 `beacon/internal/server/manager.go:487` — HandlePower same-slot (GH-05 kill pierce)

**Live code (L487-507):**
```go
482: func (m *ServerManager) HandlePower(ctx context.Context, serverID, signal string) error {
487: 	if !state.mu.TryLock() { return errors.New("another server action is already running") }
490: 	if state.RunningAction != "" { state.mu.Unlock(); return errors.New("another server action is already running") }
494: 	if state.InstallationState == "installing" { state.mu.Unlock(); return errors.New("server is installing") }
499: 	state.RunningAction = signal
581: case "kill": // same slot, 585 PowerState=Stopping, 590 runtime.Kill
```

**Finding:** Identical slot contention for `install`, `start`, `stop`, `restart`, **and** `kill`. No `if signal=="kill" { TryLock ignore failure }` branch like `wings/power.go:108` comment *"failed to acquire exclusive lock, ignoring failure for termination event"*; no separate `killMu`. Sharded `docker.go:251` `workloadLock[64]` mitigates cross-server deadlock but not per-server `RunningAction` block.

**Grep proof:**
```
beacon/internal/server/manager.go:482:func (m *ServerManager) HandlePower
beacon/internal/server/manager.go:487:	if !state.mu.TryLock()
beacon/internal/server/manager.go:490:	if state.RunningAction != ""
beacon/internal/server/manager.go:494:	if state.InstallationState == "installing"
beacon/internal/server/manager.go:499:	state.RunningAction = signal
beacon/internal/server/manager.go:581:	case "kill":
```

**Status:** `GH-05 MISSING → STILL BROKEN (MISSING)`. No pierce.  
**Prior expected fix:** bypass on `signal==kill` (`TryLock` but don't fail if occupied) or separate `killMu` semaphore, matching `wings/power.go:115-119`. Not applied.

**Dirty-tree delta:** Only `BeginInstall`/`EndInstall` helper + circuit-breaker added; `HandlePower:487` unchanged for kill.

### 4.2 `beacon/internal/server/server.go:1323` — reinstall (GH-07)

**Live code (L1331-1343):**
```go
1331: func (s *Server) reinstall(w http.ResponseWriter, r *http.Request) {
1335: 	state := s.manager.State(serverID)
1336: 	if state.PowerState == PowerStateRunning || state.PowerState == PowerStateStarting {
1337: 		http.Error(w, "server must be stopped before reinstalling", http.StatusConflict); return
1338: 	}
1342: 	s.install(w, r) // forward
```

**Finding:** Beacon correctly requires `stopped` (409) then reuses `install` path (`BeginInstall` claim). Works.

**Grep proof:**
```
beacon/internal/server/server.go:1331:func (s *Server) reinstall
beacon/internal/server/server.go:1336:	if state.PowerState == PowerStateRunning || state.PowerState == PowerStateStarting
beacon/internal/server/server.go:1342:	s.install(w, r)
```

**Status:** Beacon side `COMPLETE`. Drift is **not** here but in control-plane `clustermanager/service.go:241` gate (next section). Overall GH-07 `STILL BROKEN` because API fails while Beacon succeeds.

### 4.3 `forge/api/internal/services/clustermanager/service.go:241` — Docker reinstall gate (GH-07)

**Live code (L224-263):**
```go
224: func (s *Service) runInstaller(ctx context.Context, serverID string, reinstall bool) ...
240: 	if reinstall {
241: 		reinstaller, ok := s.runtime.(gpruntime.Reinstaller)
242: 		if !ok { err = errors.New("runtime does not support reinstall")
245: 			response, err = reinstaller.ReinstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
248: 	} else { response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target)) }
```

**Finding:** Pure Docker `gpruntime.Runtime` never implements `Reinstaller` → `failed` via `SetServerInstallState:251` `failed` leaving `status='install_failed'` stuck. Should fallback to `InstallServer` when `!ok` (mirror Beacon) or implement `Reinstall` alias in `DockerRuntime`.

**Grep proof:**
```
forge/api/internal/services/clustermanager/service.go:240:	if reinstall {
forge/api/internal/services/clustermanager/service.go:241:		reinstaller, ok := s.runtime.(gpruntime.Reinstaller)
forge/api/internal/services/clustermanager/service.go:243:			err = errors.New("runtime does not support reinstall")
```

**Status:** `STILL BROKEN`. No fallback. Prior expected fix documented in `reverification subagent-01 GH-07` + `phase-02 synthesis §8` not applied.

**Dirty-tree delta:** Only `placementReq.Runtime` + `RuntimeType` plumbing (`git diff -- service.go:88 +6, 674 +1`); reinstall gate untouched.

### 4.4 `forge/api/internal/store/store_egg_variables.go:176` — regex slash (GH-14 P0)

**Live code (L142-183):**
```go
142: func validateVariableValue(value, rules string) error {
143: 	for _, rule := range strings.Split(rules, "|") { // 143
145: 		name, arg, _ := strings.Cut(rule, ":")
176: 	case "regex":
177: 		pattern, err := regexp.Compile(arg) // 176 literal slash
179: 		if err != nil { return errors.New("invalid regex validation rule") }
```

**Finding:** For input `rules="required|string|max:20|regex:/^([\w\d._-]+)(\.jar)$/"` (stock `minecraft-paper.json:58`), `arg` = `/^([\w\d._-]+)(\.jar)$/` → compiles with slashes literal → `server.jar` must be `/server.jar/` to pass. Also `Split("|")` on `|` splits `regex:/^(foo|bar)$/` inside pattern. Blocks PTDL_v2 imports via `handlers_admin.go:1308` loop.

**Grep proof:**
```
forge/api/internal/store/store_egg_variables.go:142:func validateVariableValue
forge/api/internal/store/store_egg_variables.go:143:	for _, rule := range strings.Split(rules, "|")
forge/api/internal/store/store_egg_variables.go:176:	case "regex":
forge/api/internal/store/store_egg_variables.go:177:		pattern, err := regexp.Compile(arg)
```

**Status:** `GH-14 BROKEN (P0) → STILL BROKEN`. Prior expected fix: strip `regex:/…/` delimiters + optional flags (`/pattern/flags`) before `Compile`, respect `|` inside `regex:`; add `RESERVED_ENV_NAMES` guard; gate `CreateServer` variables by `user_viewable`. None applied. Dirty diff only audit JSON (`mustAuditJSON`).

### 4.5 `forge/api/internal/store/store_users.go:297` — subuser `*` persistable (GH-18 P0)

**Live code (L306-311, 555-571):**
```go
306: func (s *Store) UpsertServerSubuser(...) { permissions := normalizeSubuserPermissions(req.Permissions) // 311
555: func normalizeSubuserPermissions(input []string) []string {
558: 	allowed[permission]=true for _,permission := range defaultSubuserPermissions() // 557
563: 	for _, permission := range input {
564: 		if permission=="" || seen[permission] || (!allowed[permission] && permission != "*") {continue}
567: 		seen[permission]=true; output=append(output, permission)
```

**Finding:** `*` bypasses allowlist explicitly (`permission != "*"`) and is retained sorted. `UpsertServerSubuser` never checks actor's own permissions `perms ⊆ actorPerms`. `handlers_servers.go:554` `POST /servers/:id/users` checks `PermUserCreate` membership only. Any `user.create` holder → `*` → `HasPermission(..., perm)` treats `*` as wildcard → full control (`control.start`, `database.view_password`, etc). Wings `GetUserPermissionsService.php:18` computes `*` only for owner/admin never stored.

**Grep proof:**
```
forge/api/internal/store/store_users.go:306:func (s *Store) UpsertServerSubuser
forge/api/internal/store/store_users.go:311:	permissions := normalizeSubuserPermissions
forge/api/internal/store/store_users.go:555:func normalizeSubuserPermissions
forge/api/internal/store/store_users.go:564:	if permission == "" || seen[permission] || (!allowed[permission] && permission != "*")
forge/api/internal/http/handlers_servers.go:554:	protected.Post("/servers/:id/users", requireServerPermission(cfg, store.PermUserCreate)
```

**Status:** `GH-18 BROKEN (P0) → STILL BROKEN`. Escalation path live. Prior expected fix: subset enforcement `requested ⊆ actorPerms` + `*` owner/admin-only persisted behind `claims.Role==admin || owner_id==actorID`. Not applied. Dirty diff added `UserCanAccessServer` semantics tighten for empty permission (zero-perm row grants nothing) but not `*` gate.

### 4.6 `forge/api/internal/http/handlers_servers.go:2104` — restoring lock (GH-09 P1)

**Task cites `:2104`; current file has locus at `L2117` (added import drift, but same handler).**

**Live code (L2117-2177):**
```go
2117: protected.Post("/servers/:id/backups/restore", requireServerPermission(cfg, store.PermBackupRestore), func(c *fiber.Ctx) error {
2146: if backup.Status != "completed" { return 412 }
2155: op, err := cfg.OperationService.DispatchBackupRestore(ctx, target.ServerID, backup.Name, body.Truncate, actorID, idempotencyKey) // durable, no server state flip
2170: _ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restoring", actorID) // per-backup row only
```

Operation handler `forge/api/cmd/api/main.go:603` `OpBackupRestore:564` also only `MarkBackupStatus restoring/restored`, never `SetServerActualState:31` `restoring_backup`.

**Finding:** `servers.actual_state` never set to `restoring_backup` (`store_state.go:137` mapping exists but never driven). No `ensureRestoreIdle` alongside `ensureTransferIdle:145`. Concurrent `POST /power start` (`handlers_servers.go:888`) passes `ensureTransferIdle` check → enqueues → Beacon `manager.go:482` has no `Restoring` guard → race `RootDir` extract vs container start (zip half-written + live process). Wings `install.go:152` `IsRestoring` atomic + `power.go:57` blocks; Forge lacks.

**Grep proof:**
```
forge/api/internal/http/handlers_servers.go:145:func ensureTransferIdle
forge/api/internal/http/handlers_servers.go:2117:	protected.Post("/servers/:id/backups/restore"
forge/api/internal/http/handlers_servers.go:2170:		_ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restoring"
forge/api/cmd/api/main.go:564:	_ = db.MarkBackupStatus(ctx, serverID, backup.Name, "restoring"
forge/api/internal/store/store_state.go:137:	case ServerActualStateRestoringBackup: return "restoring_backup"
beacon/internal/server/manager.go:30:	type ServerState struct { PowerState; InstallationState; StartupState; RunningAction ... } // no Restoring
```

**Status:** `GH-09 MISSING (P1) → STILL BROKEN (MISSING)`. No server-level lock. Prior expected fix: on `DispatchBackupRestore` also `SetServerActualState restoring_backup` and clear on completion/failure; add `ensureRestoreIdle` guard for `power/install/reinstall/suspend`.

### 4.7 `forge/api/internal/store/store_mounts_ext.go:323` — mount allowlist (GH-19 P0)

**Live code (L323-338):**
```go
323: func validateMountPath(value, field string) error {
324: 	if !path.IsAbs(value) || path.Clean(value) != value || strings.Contains(value, "\\") { return fmt.Errorf(...) }
332: 	if field == "source" && (value == "/etc/forge" || value == "/var/lib/forge/volumes") { return errors.New("mount source or target is reserved") }
335: 	if field == "target" && (value == "/" || value == "/home/container") { return errors.New("mount source or target is reserved") }
```

**Finding:** Only 2 sources +2 targets denied; leaves `/etc`, `/etc/shadow` via `/etc`, `/var/run/docker.sock`, `/proc`, `/sys`, `/`, `/root`, `/boot`, `/var/lib/docker` mountable via compromised admin → host breakout. Forge daemon second gate `AllowedMountSourcesForNode:367` + `beacon/server.go:127` `SetAllowedMounts` exists but panel allowlist is first gate and too narrow.

**Grep proof:**
```
forge/api/internal/store/store_mounts_ext.go:323:func validateMountPath
forge/api/internal/store/store_mounts_ext.go:332:	if field == "source" && (value == "/etc/forge" || value == "/var/lib/forge/volumes")
forge/api/internal/store/store_mounts_ext.go:335:	if field == "target" && (value == "/" || value == "/home/container")
```

**Status:** `GH-19 BROKEN (P0) → STILL BROKEN`. Prior expected fix: deny-list `/etc|/proc|/sys|/dev|/var/run` or switch to allowlist prefix `/srv/forge-mounts` (`MOUNTS_ALLOWED_PREFIX`). Not applied. Dirty diff only audit JSON.

---

## 5. Remaining BROKEN Details & What Still Needs Doing

### P0 — must fix before production game hosting

**1. GH-14 / REF-GAME-F-G-08 — regex slash P0 (blocks all PTDL imports)**
- **File:line:** `store_egg_variables.go:176`
- **What to do:** Strip `regex:/…/` delimiters + optional flags before `Regexp.Compile`; parse `rules` without naive `Split("|")` (track `regex:` scope, respect `|` inside char class `[\w|…]` and alternation `|`). Suggested 8-line patch: detect `arg` starts `"/"` and ends `"/"` with optional `i/m/s` flags → `arg = arg[1:len-flags-1]`. Iterate rules via custom split that keeps `regex:` arg intact. Add `RESERVED_ENV_NAMES = {"SERVER_MEMORY","SERVER_IP","SERVER_PORT","ENV","HOME","USER","STARTUP","SERVER_UUID","UUID"}` check in `validateEggVariableRequest`. Add integration test importing `minecraft-paper.json:58`.
- **Commit that would fix:** Not landed; prior audits expected `store_egg_variables.go:143-177` patch. Current dirty tree did not land it.

**2. GH-18 / REF-GAME-F-G-22 — subuser `*` escalation P0 (vertical privilege)**
- **File:line:** `store_users.go:297` + `store_users.go:564` + `handlers_servers.go:554`
- **What to do:** In `UpsertServerSubuser`, after `normalizeSubuserPermissions`, fetch `actorPerms` via `GetServerSubuser(actorID)` if not owner/admin, then reject if `requested ⊈ actorPerms` (subset check). Gate `*` persistence behind `isOwner || isAdmin` only (`if perm=="*" && !(isOwner||isAdmin) → skip`). Mirror Wings `getDefaultPermissions` intersect + `websocket.connect` inject. Add `handlers_servers.go:554` early return `403 "wildcard requires server owner"` test.
- **Evidence not fixed:** `normalizeSubuserPermissions:564` still `permission != "*"` bypass.

**3. GH-19 / REF-GAME-F-G-24 — mount allowlist host breakout P0**
- **File:line:** `store_mounts_ext.go:323`
- **What to do:** Replace narrow deny of 2 paths with deny-list `source ∈ {"/","/etc","/proc","/sys","/dev","/var/run","/var/lib/docker","/root","/boot"}` as prefix checks (`strings.HasPrefix(value, "/etc/") || value=="/etc"` etc) or switch to allowlist prefix `value == "/srv/forge-mounts" || strings.HasPrefix(value, "/srv/forge-mounts/")` via `MOUNTS_ALLOWED_PREFIX` env. Keep `beacon` runtime `mounts.go:13` second gate but panel is first. Add `TestValidateMountPath` covering `/etc/shadow`, `/var/run/docker.sock`.

**4. GH-09 / REF-GAME-F-G-06 — restoring lock MISSING P1→P0 (data-corruption race)**
- **File:line:** `handlers_servers.go:2117` + `store_state.go:137` + `manager.go:30` + `operation/service.go:603`
- **What to do:** On `DispatchBackupRestore` also `SetServerActualState(ctx, serverID, ServerActualStateRestoringBackup, "backup restore")`; clear to `stopped` or prior on completion/failure (`MarkBackupStatus restoring→restored/failed` handler already does per-backup; add server-level clear). Add `ensureRestoreIdle` guard alongside `ensureTransferIdle:145` for `power/install/reinstall/suspend` (`IsServerRestoring` check `actual_state==restoring_backup`). Add `beacon/manager.go:30` `Restoring bool` + `HandlePower` guard if desired (or rely on panel `actual_state`). Integration test concurrent `start`+`restore` → 409 `restoring_backup in progress`.

### P1 — core broken, user-visible

**5. GH-05 kill pierce MISSING + GH-12 start-during-install race**
- `manager.go:487` + `handlers_servers.go:888`
- Fix: In `HandlePower`, if `signal=="kill"` try `TryLock` but **don't fail** if occupied (or use separate `killMu`). In handler pre-enqueue, fast 409 for `start` when `actual_state==installing` (query `IsServerInstalling`) before `DispatchPower`; keep `kill` allowed as cancellation.

**6. GH-07 Docker reinstall BROKEN**
- `clustermanager/service.go:241`
- Fix one line: `if !ok { response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target)) }` else `ReinstallServer`.

**7. GH-17 / REF-GAME-F-G-23 — schedule action asymmetry**
- `store_schedules.go:252`
- Fix: `if !isValidScheduleTaskAction(strings.TrimSpace(req.Action)) { return ..., fmt.Errorf("unsupported task action: %s", ...) }` mirror `PatchScheduleTask:332`.

**8. F-G-09 CPU conflation + F-G-10 viewable leak + F-G-11 protocol drop**
- `store_servers.go:209`, `store_servers_control.go:154`, `clustermanager/service.go:698`
- Fix: gate `CPUShares` on `CPULimit>0` (`if CPULimit==0 { CPUShares=0 }`), unify validation caps, filter `ServerProvisionTarget` to only `user_viewable` when non-admin provisioning, fix legacy `Mappings` to encode protocol or drop it.

### P2/P3 — remain PARTIAL/DUPLICATE but not blocking slice launch

- **GH-03 suspend transfer guard + race:** add `ensureTransferIdle` to `/suspension` (`handlers_servers.go:1413`) + fence `syncServerStateFromPanel:748`.
- **GH-10 transfer dual-state:** sync legacy `servers.transfer_state` from `migrations` or make `ensureTransferIdle` query `migrations`; fix `transfer-view.tsx:77` single source; update `server-lifecycle.md:12` doc 501 lie.
- **GH-15 config parser:** port `wings/parser` into `beacon/manager.go:onBeforeStart` or strip `config.files` from `minecraft-paper.json` and document unsupported.
- **GH-16 templates 93% deficit:** add `SeedGameTemplates` idempotent seeder (`appstore/seed.go:216` pattern) upserting 14 `egg-templates.ts:27` curated eggs on `cmd/api/main.go:529`; deprecate `app-templates-data.ts` localStorage or unify.
- **F-G-01 generation livelock:** advance `observed_generation` also for `crashed→stopped`/`installing→installed`.
- **F-G-03 hash without generation:** include `desired_generation` in `docker.go:193` hash.

---

## 6. Commit / Line That Would Have Fixed vs What Actually Landed

| Finding | Prior expected fix (as documented in `reverification subagent-01`/`phase-02 synthesis §8`) | Commit/line that was expected | What LIVE HEAD actually has | CONSOLIDATED |
|---|---|---|---|---|
| GH-05 kill pierce | `manager.go:487` add `if signal=="kill" { TryLock ignore }` branch | — (no commit) | `manager.go:487` unchanged for kill; `git diff HEAD -- manager.go` shows `BeginInstall/EndInstall` + circuit-breaker only, **no** kill pierce | STILL BROKEN |
| GH-07 reinstall gate | `clustermanager/service.go:241` fallback `InstallServer` when `!ok` | — | `service.go:241` gate unchanged; `git diff` shows `Runtime` plumbing only | STILL BROKEN |
| GH-14 slash | `store_egg_variables.go:143` custom split + `176` strip `/…/` | — | `store_egg_variables.go:176` still `regexp.Compile(arg)` literal; `git diff` only audit JSON `Sprintf→mustAuditJSON` | STILL BROKEN P0 |
| GH-18 `*` | `store_users.go:564` subset check + owner-only `*` | — | `store_users.go:564` still `permission != "*"` bypass; `git diff` added `dummyBcryptHash` + `UserCanAccessServer` empty→len>0 tighten but not `*` gate | STILL BROKEN P0 |
| GH-09 restoring | `handlers_servers.go:2117` + `store_state.go:137` wire `restoring_backup` | — | `handlers_servers.go:2117` still `MarkBackupStatus` only; `git diff -- handlers_servers.go` shows webhook dispatch etc but **no** `SetServerActualState restoring_backup`; `main.go:603` handler also only `MarkBackupStatus` | STILL MISSING |
| GH-19 mount | `store_mounts_ext.go:332` prefix deny / allowlist | — | `store_mounts_ext.go:332` still 2 sources only; `git diff` only audit JSON | STILL BROKEN P0 |
| GH-17 schedule | `store_schedules.go:252` call `isValidScheduleTaskAction` | — | `store_schedules.go:252` still `action!=""` only; `Patch:332` validates | STILL BROKEN |

No `git log --oneline --since="2026-08-24"` commit touches these 7 loci beyond cosmetic audit-JSON hardening. The only lifecycle-relevant logic change in dirty tree is `manager.go` `BeginInstall/EndInstall` (install fencing hardening) and `SetStateDir` wiring (`server.go:249`) — both positive but **outside** the reviewed BROKEN slice.

---

## 7. Dirty-Tree Note (HEAD `ca06f74` vs live disk)

`git status --short` shows ~80 modified files (not committed). For the 7 mandated paths:

- `beacon/internal/server/manager.go` — **modified** (install fencing + crash-breaker, not kill pierce)
- `beacon/internal/server/server.go` — **modified** (SetStateDir wiring, legacy transfer removal, hostFileRoots, not reinstall fix)
- `forge/api/internal/services/clustermanager/service.go` — **modified** (Runtime plumbing only)
- `forge/api/internal/store/store_egg_variables.go` — **modified** (audit JSON only, **not** regex fix)
- `forge/api/internal/store/store_users.go` — **modified** (timing-attack hardening + `UserCanAccessServer` semantics tighten, **not** `*` fix)
- `forge/api/internal/http/handlers_servers.go` — **modified** (webhook dispatch, pagination, etc, **not** restoring lock)
- `forge/api/internal/store/store_mounts_ext.go` — **modified** (audit JSON only, **not** allowlist fix)

Thus `STILL BROKEN` verdicts are against **live disk**, which already includes uncommitted hardening that was inspected and found insufficient for these 7 loci. Committed HEAD `ca06f74` is equally broken on these 7 (in fact slightly more: no `BeginInstall`).

---

## 8. Invalidated / Partially Fixed (this slice)

None of the 7 explicitly collapses to `VERIFIED_FIXED` or `INVALIDATED` on live disk. Two prior FINAL claims are **invalidated as overstated**:

- **GH-03 COMPLETE+ → PARTIAL** invalidated: live shows missing `ensureTransferIdle` on `/suspension` + `syncServerStateFromPanel:748` race; `final-parity C-03 PARTIAL` + `reverification GH-03 PARTIAL` already downgraded — confirmed.
- **GH-10 COMPLETE+ → PARTIAL** invalidated: `server-lifecycle.md:12` 501 lie vs `handlers_servers.go:713` 202 + stale `IsServerTransferBlocking` source; `reverification GH-10 PARTIAL` confirmed.

No `PARTIALLY_FIXED` on mandated 7; GH-11 is `PARTIALLY_FIXED` in sense explicit `protocol` path is healthy but legacy `Mappings` fallback still drops protocol — but overall still `COMPLETE+` per FINAL; reverification notes residual `Mappings` leak as separate `STILL BROKEN` sub-finding, counted under GH-11 gap.

---

## 9. Methodology & Evidence Hygiene

- Every `file:line` above was re-read via `read` tool, not inferred from prior synthesis.
- `grep -n` outputs captured via `bash` `rg` equivalent for proof; truncation avoided by `read` full file where needed.
- `git diff HEAD -- <7 paths>` and `git log --oneline` examined to determine whether a prior expected fix commit exists (none for these 7).
- No product file was edited; this report is write-only to `audits/110-phase-02-context/`.
- Cross-check: `audits/reverification/subagent-01-game-lifecycle.md:55` "No NEWLY_FIXED rows found on this lifecycle surface … No evidence of `git diff` fixes between 2026-08-24 audits and current HEAD on these paths" — **confirmed** for the 7 loci.

---

## 10. Assessor Note to Next Agents (what to do now)

For **110-Phase-02 activation**, wire in priority order (this slice only, P0→P1):

1. `store_egg_variables.go:176` regex delimiter + `Split` → unblocks all game templates (P0).
2. `store_users.go:306` + `store_users.go:564` `*` subset → closes privilege escalation (P0).
3. `store_mounts_ext.go:332` mount deny/allowlist → mitigates host breakout (P0).
4. `handlers_servers.go:2117` + `operation handler main.go:603` wire `restoring_backup` server lock → closes concurrent start/restore race (P1→P0).
5. `manager.go:487` kill pierce → emergency kill works when most needed (P1).
6. `clustermanager/service.go:241` reinstall fallback → Docker reinstall stops 500ing (P1).
7. `store_schedules.go:252` `isValidScheduleTaskAction` on Create → stops arbitrary action pollution (P2).
8. Then template seeding (`091_seed_minecraft_java.sql` → 14), config parser (`server.go:3254` full port vs document unsupported), and transfer single-source (`IsServerTransferBlocking` query `migrations`).

All above are **one-line to ~30-line** patches within existing services/stores, not new subsystems. The lifecycle superset (desired/actual+generations, orthogonal suspend, orphan remediation, hardened installer `1000:1000`/capDrop/read-only) is already production-grade — fix the 4 P0 gates and the slice reaches parity.

---

*End of 110-02-01 Game Hosting Lifecycle Confirm. No product code modified. All claims file:line citable above.*
