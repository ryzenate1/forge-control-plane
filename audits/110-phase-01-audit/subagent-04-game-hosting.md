# 110 Phase 01 Audit — Subagent 04/10 — Game Hosting

**Agent:** 110-01-04 of 110 · Phase 01 · Game Hosting (servers, allocations, eggs, schedules, subusers, mounts)
**Date:** 2026-08-24
**Scope:** Forge game-hosting slice vs `FINAL_PARITY_AUDIT.md` GH-01..GH-19 + `MASTER_FINDING_INDEX.md` REF-GAME-*
**Method:** Direct `read`+`grep` file:line re-inspection on current checkout; no trust of prior audits; no code modified.
**References:** `reference/game-hosting/pterodactyl-panel` · `pelican-panel` · `pterodactyl-wings` · `pufferpanel` · `pufferpanel-templates` under `reference/game-hosting/`

---

## 0. Executive summary

Forge game-hosting is **7 COMPLETE+/COMPLETE, 3 MISSING, 3 BROKEN (2 P0), 1 PARTIAL, 1 DUPLICATE** per `audits/FINAL_PARITY_AUDIT.md:44-64` (`Game hosting verdict:64`). Re-verification against live code confirms **all four named P0s remain BROKEN/MISSING** and **zero drift fixed** since 2026-08-24 synthesis:

| Named P0 (task prompt) | FINAL GH | MASTER REF | Live status | File:line |
|---|---|---|---|---|
| Slash bug — regex delimiter blocks PTDL imports | GH-14 `BROKEN P0` `audits/FINAL_PARITY_AUDIT.md:58` | REF-GAME-F-G-08 `store_egg_variables.go:177` | **BROKEN** | `forge/api/internal/store/store_egg_variables.go:142-188` esp. `176` |
| Wildcard escalation — any `user.create` → `*` | GH-18 `BROKEN P0` `audits/FINAL_PARITY_AUDIT.md:61` | REF-GAME-F-G-22 `store_users.go:297` | **BROKEN** | `forge/api/internal/store/store_users.go:306-365,555-573` |
| Restoring lock missing — concurrent start during restore | GH-09 `MISSING P1` `audits/FINAL_PARITY_AUDIT.md:49` | REF-GAME-F-G-06 `handlers_servers.go:2104` | **MISSING** | `forge/api/internal/http/handlers_servers.go:2117-2181,2170` + `store_state.go:31,131` never driven |
| Mount allowlist too narrow — host breakout | GH-19 `BROKEN` `audits/FINAL_PARITY_AUDIT.md:62` | REF-GAME-F-G-24 `store_mounts_ext.go:323` | **BROKEN** | `forge/api/internal/store/store_mounts_ext.go:323-339` |

**Net recommendation for Phase 01:** 4 surgical store/handler fixes (no new subsystem), 2 additive migrations/seed, 3 frontend patches. All are `S` scope, `P0` severity, additive-safe.

---

## 1. Inventory — files inspected

### 1.1 Store layer `forge/api/internal/store/`

| File | Lines | Responsibilities — key symbols file:line | Parity relevance |
|---|---|---|---|
| `store_servers.go` | 531 | `ListServers:31`, `ListServersForUser:56`, `ListServersPaginated:121`, `CreateServer:180` Tx validates `node/egg/allocations FOR UPDATE:234`, `GetServer:308`, `IsServerTransferBlocking:346`, `SetServerSuspension:374`, `CompareAndSetServerSuspension:385`, `SetServerSuspended:397` audit, `UpdateServer:428`, `UpdateServerGeneration:502` | GH-01,03,04,11,12 |
| `store_servers_control.go` | 275 | `SetServerPowerState:13` + `powerSignalPriorStates:44` (`start:[created,stopped,install_failed]`, `stop/kill:7-states:51`), `ServerControlTarget:57`, `ServerProvisionTarget:82` joins `docker_images/startup/install_script`, `Environment` from `egg_variables:155`, `resolveStartupCommand:179`, `Mounts+Allocations:180-198`, `SetServerProvisioned:207` `created`, `SetServerInstallState:218` `installing/installed/failed`, `MarkServerConfigSynced:244`, `SetServerStatus:266` | GH-01,04,06,12,15 |
| `store_servers_lifecycle.go` | 57 | `HardDeleteServer:12`, `RecordOrphanAndHardDeleteServer:18` requires `daemonError!=""`, `hardDeleteServer:25` Tx `FOR UPDATE:32`, `server_orphan_remediations:36`, `allocations.server_id=NULL:46`, `DELETE servers:49` | GH-08 |
| `store_nests.go` | 425 | `Nest:16`, `Egg:36`, `ListNests:100`, `GetNest:123`, `CreateNest:137`, `DeleteNest:172` guards `eggCount>0`, `ListEggs:190`, `GetEgg:224`, `CreateEgg:248` `normalizeDockerImages:253`, `UpdateEgg:292`, `normalizeDockerImages:361` handles `map` or legacy `[]`, `normalizeJSONObject:394`, `normalizeJSONArray:405`, `DeleteEgg:416` | GH-13,16 |
| `store_egg_variables.go` | 189 | `eggVariableNamePattern:15` `^[A-Z][A-Z0-9_]*$`, `EggVariable:17`, `ListEggVariables:42`, `CreateEggVariable:65`, `UpdateEggVariable:84`, `validateEggVariableRequest:129` (`name`, `envVariable`, `rules`, `validateVariableValue:139`), **`validateVariableValue:142`** `Split("|"):143` → `Cut(rule,":"):145` → `regex: Compile(arg):177` — **slash bug** | GH-13,14 `P0` |
| `store_startup.go` | 100 | `GetServerStartup:11` `COALESCE startup_command:15`, `user_viewable=true:31`, `resolveStartupCommand:50`, `UpdateServerStartupVariable:54` checks `user_editable:58`, `validateVariableValue:73`, `INSERT ON CONFLICT:81`, `config_sync_pending=true:90` | GH-13,14 |
| `store_templates.go` | 125 | `ListTemplates:14` compat over `ListEggs`, `GetTemplate:30`, `CreateTemplate:38` via `CreateEgg` under `Games` nest `SELECT nests WHERE name='Games':48`, `UpdateTemplate:70`, `templateFromEgg:99` `json.Unmarshal dockerImages:101`, sorted keys `keys[0]:110` | GH-16 DUPLICATE |
| `store_allocations.go` | 391 | `AllocationNode:15`, `ListAllocationNodes:23`, `ListAllocationsWithTotal:69`, `CreateAllocations:133` Tx `net.ParseIP:145`, `protocol tcp/udp:151`, `containerPort 0→port:155`, `pg 23505:182`, `DeleteAllocations:220` batch-atomic `assigned IS NULL:232`, `UpdateAllocation:250`, `AssignAllocationToServer:327` `node_id FOR UPDATE`, `Unassign:350` guards `primary:353`, `SetPrimaryAllocation:367` Tx `FOR UPDATE:374` + `config_sync_pending=true:377` | GH-11 `COMPLETE+` |
| `store_state.go` | 177 | `SetServerDesiredState:10` `desired_generation +1 on distinct:16`, `SetServerActualState:31` `observed_generation CASE 39` + `serverStatusFromActual:131` mapping `restoring_backup:138`, `SetNodeDesiredState:55`, `SetNodeActualState:83`, `recordStateTransition:120` → `state_transitions`, helpers `serverDesiredFromSignal:165`, `serverActualFromSignal:172` | GH-02,09 |
| `store_schedules.go` | 725 | `resolveStartupCommand:14`, `ListSchedules:26`, `CreateSchedule:166`, `PatchSchedule:208`, **`CreateScheduleTask:252`** (`action!="" :253` only, `timeOffset>=0`), **`PatchScheduleTask:331`** validates `isValidScheduleTaskAction:332`, `DeleteScheduleTask:379`, `NotifySchedulesChanged:398`, `ListDueSchedules:489` `only_when_online:500`, `CreateScheduleRun:590`, `ListScheduleRuns:624` | GH-17 |
| `store_users.go` | 1314 | `Authenticate:96` + `dummyBcryptHash:82`, `ListUsers:131`, `CreateUser:226`, **`ListServerSubusers:280`**, **`UpsertServerSubuser:306`** `normalizeSubuserPermissions:311` + `owner check:330`, **`UserCanAccessServer:377`** `HasPermission:406` vs `permission==""` baseline `401`, `DeleteServerSubuser:409`, `AuthenticateSFTP:421` + `AuthenticateSFTPPublicKey:483` + `AuthorizeSFTPSession:521`, **`normalizeSubuserPermissions:555`** `allowed[perm]||perm=="*":564` + `sort:569`, `defaultSubuserPermissions:574` | GH-18 `P0` |
| `store_mounts_ext.go` | 534 | `ListMounts:13`, `CreateMount:41` `validateMountPaths:48`, `UpdateMount:121` allowlist `177`, `AttachEggToMount:194`, `ServerMountsForMount:226`, `DeleteMount:253`, `AssignMountToServer:272` → `assignMountToServer:276` Tx `mount_server ON CONFLICT DO NOTHING:285`, **`ensureMountAvailableForServer:297`** `EXISTS JOIN mount_node + egg_mount:300-306`, **`validateMountPath:323`** `path.IsAbs:324`, `Clean==value`, `no ..:328`, **allowlist `source==/etc/forge or /var/lib/forge/volumes:332`, `target==/ or /home/container:335`** | GH-19 `P0` |
| `store.go` (shared) | 1891 | `Server` struct DTO `actual_state/desired_state`, `ServerControlTarget`, `ServerProvisionTarget`, `Mount`, `Allocation`, `isValidScheduleTaskAction:1196` `power|backup|command`, `runMigrations:1205` | GH-02,17 |
| `permissions.go` | — | `HasPermission:206` `p=="*"` wildcard, `IsSensitivePermission`, `defaultSubuserPermissions` catalog 40+ keys | GH-18 |

### 1.2 HTTP layer `forge/api/internal/http/`

| File | Lines | Responsibilities file:line | Relevance |
|---|---|---|---|
| `handlers_servers.go` | 3767 | `ensureTransferIdle:145` `IsServerTransferBlocking:151`, `registerServerRoutes:161`, `POST /servers:789` `clusterManager.CreateServer:838`, `POST /servers/:id/power:888` `ensureTransferIdle:889` → `DispatchPower:915` deterministic `uuid.NewSHA1("forge-op:"+key)`, `POST /servers/:id/install:1050` `DispatchInstall:1070`, `POST /servers/:id/reinstall:1110` `DispatchReinstall:1129`, `suspension:1421` (`suspend/unsuspend` + legacy `1450/1462`), `DELETE /servers/:id:1474` via `clusterManager.DeleteServer:332`, `POST /servers/:id/backups/restore:2117` `Check completed:2145` → `DispatchBackupRestore:2155` **or fallback `MarkBackupStatus restoring:2170`** + `daemon.RestoreBackup:2173`, `GET /backups/verify:2183` checksum path, `GET /users:501` `requireServerPermission PermUserCreate` but no subset, `POST /users:554`, `GET /allocations:454` | GH-01..09,11,17,18 |
| `handlers_admin.go` | 93417 total (partial read) | `GET /nests:990`, `GET /nests/:id:1003`, `POST /nests:1016` `mutationLimiter + requireRole("admin")`, `PATCH /nests/:id:1043`, `DELETE /nests/:id:1067` guards `eggCount>0`, `GET /eggs:1085`, `GET /nests/:nestId/eggs:1098`, `GET /eggs/:id:1111`, `POST /eggs:1124`, `PATCH /eggs/:id:1156`, `GET /eggs/:id/variables:1185`, `POST /eggs/:id/variables:1198`, `PATCH /eggs/:id/variables/:variableId:1226`, `DELETE /eggs/:id/variables/:variableId:1254`, `GET /eggs/:id/export:1288`, `POST /eggs/import:1308` loop `CreateEggVariable` — **hits slash bug**, `GET /allocations/nodes:1365`, `GET /allocations:1378` paginated `50`, `GET /mounts:1579`, `POST /mounts:1605`, `DELETE /mounts:1638`, `PATCH /mounts:1687`, `POST /mounts/:id/eggs:1744`, `DELETE /mounts/:id/eggs/:eggId:1764`, `POST /mounts/:id/nodes:1776`, `POST /mounts/:id/servers:1810`, `GET /allocations bulk:1965` | GH-11,13,16,19 |

### 1.3 Docs, migrations, packages, web

| Path | What was inspected file:line | Finding |
|---|---|---|
| `forge/api/docs/server-lifecycle.md` | 16 lines: `5` `provisioning` + `installed=false`, `6` Beacon-first delete + `server_orphan_remediations`, `12` `transfer/archive 501 Not Implemented` | **Doc lie** vs `handlers_servers.go:713` `POST /servers/:id/transfer` returns `202` via `MigrationService`. Update doc to reflect `forge-beacon-transfer/v1` `services/migration/service.go:102`. |
| `forge/api/migrations/ 043_unify_eggs_templates_mounts.sql` | 163 lines: `5-10` `eggs` parity columns, `14` `docker_images array→map`, `20` `Legacy Templates` nest, `30-61` `server_templates→eggs` sync, `63-91` `servers egg_id/template_id FK`, `143-163` trigger `sync_server_egg_identifiers` | GH-13 canonicalization correct; `eggs.docker_images` `jsonb_array_elements_text` migration prevents legacy crash. |
| `forge/api/migrations/ 089_egg_parity_fields.sql` | 7 lines `1-6` adds `config_from, copy_script_from, update_url, author, features, startup_commands` + `egg_variables.sort` | GH-13 parity fields present, not used by `ServerProvisionTarget` (only `config`/`features`/`startup_commands` passthrough via `Egg` read). |
| `forge/api/migrations/ 091_seed_minecraft_java.sql` | 55 lines `3-27` single `eggs` `Minecraft Java` `itzg/minecraft-server:java21`, `31-54` 2 vars `VERSION, TYPE` | **Seeding deficit:** 1/14 curated. Fresh install has 1 egg; `forge/web/lib/egg-templates.ts:27` 14 static vs `main` missing `packages/game-templates` (see below). `REF-GAME` `DUPLICATE+MISSING`. |
| `forge/api/migrations/ 015_a_mounts.sql` | 33 lines `1-11` `mounts` + `13-29` pivots `mount_node/egg_mount/mount_server` | GH-19 mounts schema correct; `090_allocation_transport.sql` fixes unique to `(node_id,ip,port,protocol)` (`090:8-15`). |
| `forge/api/migrations/ 090_allocation_transport.sql` | `ADD CONSTRAINT allocations_node_ip_port_protocol_key UNIQUE (node_id,ip,port,protocol)` | GH-11 DB uniqueness now correct — code assumption `store_allocations.go:178` `23505` now protocol-distinct; legacy `Mappings` fallback still drops protocol. |
| `forge/api/migrations/ 040_truthful_server_lifecycle.sql` | `7` clobbers stale `queued/running→failed`, `18` `server_orphan_remediations` `status pending|resolved` | GH-08,10 dual-state split: modern `migrations` not projected to legacy `servers.transfer_state`. |
| `packages/game-templates` | **Absent on `main`** (`ls packages → sdk, shared-types`); exists in worktrees `.freebuff/.../packages/game-templates` + `templates/*.json` 14 curated + `template-schema.json` | **Phantom package** on worktrees; `main` cannot build `packages/game-templates`. `FORGE-LOGIC` `DUPLICATE`. Single source of truth broken. |
| `forge/web/lib/egg-templates.ts` | `27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries; e.g. `minecraft-paper` `63-84` `config.files["server.properties"]{parser:properties, find:{server-port→{{server.build.default.port}}}}`, env `rules:"required|regex:/^([\w\d._-]+)(\.jar)$/"` (`egg-templates.ts:58` variant) | GH-14 regex sample that triggers slash bug; GH-15 `config.files` inert (parser MISSING). |
| `forge/web/app/admin/nests/*` | `app/admin/nests/page.tsx` aggregates nests+eggs; `[nestId]/eggs/page.tsx` CRUD `dockerImages` newline list + `startup/stop/features/installScript`; `[eggId]/variables/page.tsx` CRUD | GH-13 UI exists but hits `CreateEggVariable` slash bug on import. `CreateNest:137` requires name; `DeleteNest:172` guards `eggCount>0` but race without `FOR UPDATE`. |
| `forge/web/app/admin/app-templates/page.tsx` | `formToTemplate` freeform `host:container` CSV → `localStorage STORAGE_KEY forge.app-templates.v1` 5 defaults (`app-templates-data.ts:3`) | GH-16 localStorage catalog disconnected from DB — `DUPLICATE`. |
| `forge/web/components/server/console-view.tsx` | 315 lines; `37-50` `InstallBanner` when `status==='installing'`, `52-71` `TransferBanner` when `transferring`, `91` `Chart` sparkline, `125` `canConsole/hasServerPermission`, `146-187` WS install `WebSocketManager`, `189-212` stats WS `network = rx+tx:203` → `networkHistory:206` **cumulative** (`REF-GAME-F-G-18`), `229` `blocked = suspended||transferring||status==='installing'` (no `restoring_backup`), `242-249` `Chart` CPU/memory/network + uptime, `154` double-parse `try {JSON.parse(text) as {data?:string}}` vs `websocket-manager.ts:107` already parses | GH-04,05,06,09 + `REF-GAME-F-G-18` network cumulative not delta, `F-G-17` double JSON parse, `F-G-10` `user_viewable` leak via `ServerProvisionTarget`. |
| `forge/web/components/server/transfer-view.tsx` | `31` polls `GET /servers/:id/transfer` 5s if `transferring`, `77` `isTransferring = server.transferring` vs `transfer.transferring` flicker | `REF-GAME-F-G-19` dual source race. |
| `forge/web/components/server/users-view.tsx` | `25` `selectedPermissions` default `["websocket.connect","control.console","file.read","file.sftp"]`, `36` `Array.from(new Set(["websocket.connect",...]))` | GH-18 UI always injects `websocket.connect` — matches `REF-GAME` but store allows `*` bypass. |
| `forge/web/lib/api/ws/websocket-manager.ts` | `23` `WebSocketManager` `maxRetries 10`, `104` `onmessage = (event)=>{const data = JSON.parse(event.data); onMessage(data)}`, `109-112` else warn non-JSON | **Double-parse bug:** manager parses to object then `console-view.tsx:154` does `String(data)`→`"[object Object]"`→`JSON.parse` fails→plain daemon output dropped. |
| `forge/api/internal/daemon/client.go` (referenced) | `438` `Port{HostIP,HostPort,ContainerPort,Protocol}` + `Mappings` compat | GH-11 compat drops protocol. |
| `beacon/internal/server/manager.go` (referenced) | `271` `BeginInstall` under `TryLock`, `487` `HandlePower` `RunningAction` slot, `748` `syncServerStateFromPanel` race | GH-03,04,05 |

---

## 2. Reconciliation vs GH-01..GH-19 + REF-GAME-*

### 2.1 GH matrix (FINAL_PARITY_AUDIT §2.1)

| GH | Title — FINAL status `FINAL_PARITY_AUDIT.md:44-62` | MASTER REF | Re-verification now (live) | Delta |
|---|---|---|---|---|
| GH-01 | Create `provisioning→created` vs `installing` — `COMPLETE (better)` | — | **COMPLETE** `store_servers.go:253` `provisioning` + `store_servers_control.go:207` `created` + `server-lifecycle.md:5` | CONFIRMED |
| GH-02 | Desired/actual + generations — `COMPLETE+` | REF-GAME-C01 `F-G-13` | **COMPLETE** `store_state.go:10,31,39` fence, `state_transitions:120` — livelock `crashed+desired running` still (`store_state.go:40` only `running↔running|stopped↔stopped`) | CONFIRMED |
| GH-03 | Suspend orthogonal bool — `COMPLETE+` | — | **PARTIAL** downgraded: `handlers_servers.go:1413` `POST /suspension` lacks `ensureTransferIdle:145`, `store_servers.go:374` raw `SetServerSuspension` bypass CAS, `manager.go:748` race | FINAL overstated; matches `final-parity C-03 PARTIAL` |
| GH-04 | Power start/stop/restart — `PARTIAL` | — | **PARTIAL** `handlers_servers.go:888` `ensureTransferIdle` only transfer, `store_servers_control.go:44` allowlist; Beacon `manager.go:494` blocks installing | CONFIRMED |
| GH-05 | Kill pierces stuck lock — `MISSING` | — | **MISSING** `manager.go:487,581` same `RunningAction` slot; `docker.go:494` `workloadLock[64]` sharded but not bypass | CONFIRMED |
| GH-06 | Install fencing 1000:1000 — `COMPLETE+` | — | **COMPLETE** `server.go:1052 install`, `manager.go:271`, `docker.go:256` `User 1000:1000` `ReadonlyRootfs:302` `CapDrop:ALL:299`, `installWS:1178 ScopeAdmin` | CONFIRMED |
| GH-07 | Reinstall requires stopped / Docker gate — `BROKEN` | REF-GAME-F-G-07 `clustermanager/service.go:242` | **BROKEN** `clustermanager/service.go:241` `Reinstaller` gate → 500 on Docker; `beacon/server.go:1331` works (calls `install`) — API/Beacon drift | CONFIRMED not FIXED |
| GH-08 | Delete hard-delete + orphan — `COMPLETE+` | — | **COMPLETE** `store_servers_lifecycle.go:18` + `server-lifecycle.md:6` | CONFIRMED |
| GH-09 | `restoring_backup` lock — `MISSING P1` | REF-GAME-F-G-06 `handlers_servers.go:2104` | **MISSING** `handlers_servers.go:2170` marks `backups.status=restoring` not `servers.actual_state`; `store_state.go:137` mapping never driven | CONFIRMED P1 (P0 for data corruption) |
| GH-10 | Transfer/migration v1 — `COMPLETE+` | — | **PARTIAL** downgraded: `server-lifecycle.md:12` says 501 but `handlers_servers.go:713` `202` via `migration/service.go:127`; `store_servers.go:346` `IsServerTransferBlocking` reads legacy `servers.transfer_state` not `migrations` | FINAL overstated; matches `final-parity C-10 PARTIAL` |
| GH-11 | Allocations containerPort/protocol — `COMPLETE+` | REF-GAME-F-G-09? | **COMPLETE** `store_allocations.go:15,133` + `090` unique `(node_id,ip,port,protocol)` fixed; `Mappings` compat still drops protocol `clustermanager/service.go:698` | CONFIRMED |
| GH-12 | Allocations kill/start vs install — `BROKEN` | — | **BROKEN** `store_servers_control.go:51` allows `stop/kill` during `installing`, API only `ensureTransferIdle` not installing, Beacon `manager.go:494` blocks → enqueued doomed op retries 3× | CONFIRMED |
| GH-13 | Egg/nest/egg_variables — `PARTIAL` | — | **PARTIAL** `store_nests.go:34` + `store_egg_variables.go:17` schema ok but GH-14 bug + `DeleteNest:172` `COUNT(*)` race | CONFIRMED |
| GH-14 | Variable validation slash bug — `BROKEN P0` | REF-GAME-F-G-08 `store_egg_variables.go:177` | **BROKEN P0** `store_egg_variables.go:176` `regexp.Compile(arg)` with slashes, `143` `Split("|")` splits alternation | CONFIRMED not FIXED |
| GH-15 | Config parser 6 parsers — `MISSING` | — | **MISSING** `eggs.config` stored `store_servers_control.go:95` not applied; `beacon/server.go:869` no parser; `resolveStartupCommand:14` only `{{VAR}}` | CONFIRMED |
| GH-16 | Template systems — `DUPLICATE` | REF-P6-TMPL-01 | **DUPLICATE** `091` 1 egg vs `egg-templates.ts:27` 14 static vs `app-templates-data.ts:3` 5 localStorage | CONFIRMED |
| GH-17 | Schedules cron tasks — `PARTIAL` | REF-GAME-F-G-23 `store_schedules.go:252` | **PARTIAL** `CreateScheduleTask:252` `action!=""` only vs `PatchScheduleTask:331` `isValidScheduleTaskAction` — asymmetry allows arbitrary action | CONFIRMED |
| GH-18 | Subusers wildcard — `BROKEN P0` | REF-GAME-F-G-22 `store_users.go:297` | **BROKEN P0** `normalizeSubuserPermissions:555` allows `*` without subset, `UpsertServerSubuser:306` no actor check | CONFIRMED not FIXED |
| GH-19 | Mount allowlist / file denylist — `BROKEN` | REF-GAME-F-G-24 `store_mounts_ext.go:323` | **BROKEN** `validateMountPath:323` only `2+2` reserved (`/etc/forge,/var/lib/forge/volumes` + `/,/home/container`) | CONFIRMED |

### 2.2 MASTER_FINDING_INDEX REF-GAME-* drill (open game findings)

| REF | Title `MASTER_FINDING_INDEX.md:44-57` | Maps to GH | Live evidence | Severity |
|---|---|---|---|---|
| REF-GAME-C01 | Install/kill/suspend fencing races — `beacon/manager.go:693, store_state.go:40` | GH-02,03,05 | `store_state.go:40` fence livelock `crashed`; `manager.go:748` `syncServerStateFromPanel` loads `Suspended:611` releases then re-acquires `748` — `HandlePower start` interleaves | P1 |
| REF-GAME-F-G-06 | restoring_backup not set as server lock | GH-09 | `handlers_servers.go:2117` `MarkBackupStatus restoring:2170` not `SetServerActualState restoring_backup`; `store_schedules.go:14` `Blocked` misses `restoring_backup` | HIGH/MISSING P1 |
| REF-GAME-F-G-07 | Reinstall fails on Docker gate | GH-07 | `clustermanager/service.go:242` `runtime does not support reinstall` vs `beacon/server.go:1331` works | HIGH/BROKEN P1 |
| REF-GAME-F-G-08 | Regex delimiter blocks imports | GH-14 | `store_egg_variables.go:177` `regexp.Compile(arg)` with `/^.../` literal | HIGH/BROKEN P0 |
| REF-GAME-F-G-09 | CPU cpu_shares vs CPUPercent conflation | GH-12 | `store_servers.go:209` allows `CPULimit 0` + `CPUShares 1024`; `beacon/docker.go:924` `CPUPercent 0..100000`, `store_servers_control.go:96` `cpu_shares/cpu_limit` both loaded but `buildResources:783` `CPUPercent→Quota` vs `CPUShares` weight gating `settings.go:124` diverge | HIGH/BROKEN P2 |
| REF-GAME-F-G-10 | user_viewable gate leak on CreateServer | GH-14 | `store_servers.go:273` `CreateServer StartupVariables` validates `rules` but not `user_editable/viewable`; `store_servers_control.go:154` `ServerProvisionTarget:154` loads **all** vars; `store_startup.go:31` filters `user_viewable=true` — non-admin can set `DL_PATH` at provision to hijack installer | MED/BROKEN P2 |
| REF-GAME-F-G-13 | Overcommit scoring healthy | — | `noderegistry/service.go:302` (not re-inspected but cited) — deferred; out of slice but noted | MED |
| REF-GAME-F-G-15 | Heartbeat history 5 truncation misclassifies recovery | — | `heartbeatmonitor/service.go:205` — out of slice deferred | MED |
| REF-GAME-F-G-17 | WS double JSON parse drops plain output | — | `websocket-manager.ts:107` parses to object → `console-view.tsx:154` `String(data)`→`"[object Object]"`→second `JSON.parse` fails→plain daemon output dropped | P1/BROKEN |
| REF-GAME-F-G-18 | Network graph cumulative not delta | — | `console-view.tsx:203` `network = rx+tx` cumulative → `networkHistory:206` never delta; sparkline trends total bytes not throughput | P1/BROKEN |
| REF-GAME-F-G-19 | Transfer dual source race flicker | GH-10 | `transfer-view.tsx:77` `server.transferring` (cached) vs `transfer.transferring` (polled) distinct sources | P1/BROKEN |
| REF-GAME-F-G-22 | Subuser * escalation | GH-18 | `store_users.go:306,555` `normalize` allows `*` | HIGH/BROKEN P0 |
| REF-GAME-F-G-23 | CreateScheduleTask missing `isValidScheduleTaskAction` | GH-17 | `store_schedules.go:252` only `action!=""` | MED/BROKEN P2 |
| REF-GAME-F-G-24 | Mount allowlist too narrow (host breakout) | GH-19 | `store_mounts_ext.go:323` | MED→P0/BROKEN |

### 2.3 Cross-check `reverification/subagent-01-game-lifecycle.md`

Reverification 01/20 re-checked same slice and matches this preparation audit on every GH row (no newly-fixed rows, zero drift). Deltas: GH-03 and GH-10 downgraded from `COMPLETE+` to `PARTIAL` in `final-parity` and confirmed here — FINAL overstated.

---

## 3. Known P0s — deep dive (reproduction + blast radius + fix)

### 3.1 P0 — `store_egg_variables.go:176` slash bug blocks PTDL imports

**Blast:** `POST /nests`, `POST /eggs`, `POST /eggs/import`, `POST /eggs/:id/variables`, `POST /servers` (via startup vars), `PATCH /servers/:id` startup. Import of **every** stock PTDL egg with `regex:/.../` fails.

**Reproduction:**
1. `POST /eggs/import` with `minecraft-paper.json` `SERVER_JARFILE` `rules:"required|regex:/^([\w\d._-]+)(\.jar)$/"` → `CreateEggVariable:65` → `validateVariableValue:142` → `rules.Split("|")[143]` yields `regex:/^([\w\d._-]+)(\.jar)$/` → `Cut(":")[145]` → `arg=/^([\w\d._-]+)(\.jar)$/` → `regexp.Compile(arg)[177]` compiles literal slashes → `server.jar` does **not** match `/^.../` → 400 `value does not match required pattern`. Also `Split("|")` splits `regex:/^(foo|bar)$/` `in:easy,normal` composites.
2. `minecraft-paper.json:58` variant `regex:/^([\w\d._-]+)(\.jar)$/` stored in `egg-templates.ts:58` same shape.

**Evidence:**
- `store_egg_variables.go:15` `eggVariableNamePattern ^[A-Z][A-Z0-9_]*$`
- `store_egg_variables.go:129` `validateEggVariableRequest` calls `validateVariableValue` on `DefaultValue`
- `store_egg_variables.go:142-188` `validateVariableValue`; `143` `strings.Split(rules,"|")`, `145` `strings.Cut(rule,":")`, `176` `regexp.Compile(arg)` — no delimiter strip
- `store_egg_variables.go:176-183` `if !pattern.MatchString(value) → "value does not match"`
- `handlers_admin.go:1308` `POST /eggs/import` loops `CreateEggVariable` per var — fails on slash
- `store_servers.go:273` `CreateServer StartupVariables` same helper — provision-time var also fails
- PTDL ref `reference/game-hosting/pterodactyl-panel/... EggVariable.php:66` `regex:/pattern/` Laravel style

**Fix scope (additive, no migration):**
- `forge/api/internal/store/store_egg_variables.go:142` — strip `/` delimiters and optional flags `i,m` before `Regexp.Compile`; make `Split` respect `regex:` payload (collect remainder after first `regex:` or use state machine). Reuse Laravel semantics or add unit test `minecraft-paper` rules.
- `forge/api/internal/store/store_egg_variables.go:129,142` add `RESERVED_ENV_NAMES` guard (`SERVER_MEMORY/SERVER_IP/SERVER_PORT/ENV/HOME/USER/STARTUP/SERVER_UUID/UUID`) matching `EggVariable.php:43`.
- Tests: `store_eggs_integration_test.go` — add cases for slash-delimited regex, `|` inside class, `in:` + `regex:` combos.

**Files to touch:** `store_egg_variables.go:142-188`, `store_egg_variables_test.go` (new), `handlers_admin.go:1308` (no change, caller benefits), `forge/web/lib/egg-templates.ts:58` (no change, data now valid).

**Risk if not fixed:** `GUARDED` — imports fail closed (safe) but blocks all PTDL seeding; `091` seed survives only because it has no regex vars; `egg-templates.ts` 14 cannot be imported.

---

### 3.2 P0 — `store_users.go:297,555` wildcard `*` + missing subset escalation

**Blast:** Any holder of `user.create` (subuser `user.create` suffices per `permissions.go:28`) can `POST /servers/:id/users {email:victim, perms:["*"]}` → victim gains full control including `database.view_password`, `file.sftp`, `allocation.update`, `settings.reinstall`, `mount.update`. Also admin SFTP `AuthenticateSFTP:464` & `AuthorizeSFTPSession:521` honor `*`.

**Reproduction:**
- `POST /servers/:id/users` (`handlers_servers.go:554`) checks `requireServerPermission(PermUserCreate)` but `UpsertServerSubuser:306` does `normalizeSubuserPermissions:311` which allows `*` (`564` `allowed[perm]||perm=="*"`), no actor-subset check.
- `normalizeSubuserPermissions:555` sorts but does not intersect with actor perms.
- `UserCanAccessServer:377` `HasPermission:406` treats `*` as wildcard — persisted `*` grants all.
- `GetUserPermissionsService.php:18` ref says `*` owner-only computed, never stored.

**Evidence:**
- `store_users.go:306` `UpsertServerSubuser` `permissions := normalizeSubuserPermissions(req.Permissions):311`
- `store_users.go:555` `normalizeSubuserPermissions:563` `if permission==""||seen[permission]||(!allowed[permission]&&permission!="*") continue`
- `store_users.go:574` `defaultSubuserPermissions` catalog 40+ keys
- `store_users.go:362` `UserCanAccessServer:377` comment `permission==""` baseline vs wildcard `HasPermission` `permissions.go:206` `p=="*"` — correct distinction for read but not write gate
- `handlers_servers.go:541,554` permission check only `user.create`, not `perms ⊆ actorPerms`
- `permissions.go:206` `HasPermission` wildcard

**Fix scope:**
- `store_users.go:306` `UpsertServerSubuser` — add `actorID` param (like `handlers_servers.go:554` already has `actorID` via `c.Locals("user")`), load actor perms (`ListServerSubusers` for actor or `UserCanAccessServer` expansion + `AllPermissionsForActor`), reject if `requested ⊈ actor ∪ {admin/owner}`; gate `*` behind `role=="admin" || actorIsOwner`.
- `permissions.go` — optionally add `IsOwnerOrAdmin` helper.
- `handlers_servers.go:554` — pass actorID through, translate 403 `cannot grant permission you do not hold`.
- Additive, no migration; but audit existing rows: `SELECT server_id,user_id,permissions FROM subusers WHERE permissions::text LIKE '%*%'` — flag for admin review.

**Files to touch:** `store_users.go:306,555`, `handlers_servers.go:554,594`, `permissions.go`, `forge/web/components/server/users-view.tsx:36`.

**Frontend impact:** `users-view.tsx:36` already injects `websocket.connect`; after fix, subuser attempting `*` will 403 — UI should surface error toast and disable `*` checkbox for non-owner. `users-view.tsx:25` `selectedPermissions` picker should hide `*` unless owner/admin.

**Risk if not fixed:** Cross-tenant privilege escalation within same panel — `P0` security boundary violation; `SE-01/SE-02` `MASTER_FINDING_INDEX.md:58`.

---

### 3.3 P1→P0 — `handlers_servers.go:2117` restoring lock missing (data corruption race)

**Blast:** Concurrent `POST /servers/:id/power {signal:start}` + `POST /servers/:id/backups/restore {name,truncate}` both succeed at API; Beacon `restoreBackup:1691` `backupMu` serialize + `pre-restore-*.zip` snapshot but `manager.go:30` has no `Restoring` state; container `Start` races `Extract` on same `RootDir` → half-written files, crash loop, or leaked snapshot.

**Reproduction:**
- `POST /backups/restore` (`handlers_servers.go:2117`) `resolveBackupName:2141`, `Check completed:2145`, `DispatchBackupRestore:2155` **or** `MarkBackupStatus restoring:2170` (per-backup row `store_backups.go:115`), no `SetServerActualState restoring_backup`.
- `POST /power` (`handlers_servers.go:888`) only `ensureTransferIdle:889` (`IsServerTransferBlocking:151` reads `servers.transfer_state` not `backups`), no restoring guard — 202 accepted, worker `sendPower:532` sets `desired=running` → `runtime.Start`.
- `store_state.go:31` `SetServerActualState` exists with `restoring_backup` mapping `131-138` but zero callers from backup path.

**Evidence:**
- `handlers_servers.go:2117-2181` restore handler; `2170` `MarkBackupStatus(ctx,target.ServerID,backup.Name,"restoring",actorID)` — wrong table
- `handlers_servers.go:145` `ensureTransferIdle` only transfer
- `store_state.go:31` `SetServerActualState`, `131` `serverStatusFromActual restoring_backup`
- `store_backups.go:115` `MarkBackupStatus` per-backup
- `beacon/internal/server/server.go:1691` `restoreBackup` `backupMu`
- `console-view.tsx:229` `blocked = suspended||transferring||installing` — missing `restoring_backup`
- `store_servers_control.go:51` `powerSignalPriorStates` lists `restoring_backup` in stop/kill allowlist but API never sets it, so `start` incorrectly allowed during restore (should be blocked)

**Fix scope (additive migration optional):**
- `handlers_servers.go:2117` — on `DispatchBackupRestore` also call `store.SetServerActualState(ctx, serverID, ServerActualStateRestoringBackup, "backup restore started"):31`; on completion/failure (via `operation/service.go` backup worker `MarkBackupStatus restore_failed/restored` + `backup/service.go:577` `restoring`→`restore_failed`) clear to `stopped` or prior `actual_state`. Need `operation` callback to clear — add `SetServerActualState` in `backup/service.go:577` success path.
- Add `ensureRestoreIdle` guard alongside `ensureTransferIdle:145` for `POST /power`, `POST /install`, `POST /reinstall`, `POST /suspension` — check `SELECT actual_state FROM servers WHERE id=$1 = 'restoring_backup'` or `EXISTS backups WHERE server_id=$1 AND status='restoring'`.
- Migration (additive, non-breaking): optional index `CREATE INDEX backups_restoring_idx ON backups(server_id) WHERE status='restoring'` to make `ensureRestoreIdle` cheap. No `ALTER servers` needed — `actual_state` already enum.
- `console-view.tsx:229` `blocked` add `restoring_backup` badge + banner mirroring `InstallBanner`.

**Files to touch:** `handlers_servers.go:145,888,1050,1110,2117`, `store_state.go:31`, `services/backup/service.go:577`, `beacon/server.go:1691` (optional `Restoring` flag), `forge/web/components/server/console-view.tsx:229`.

**Risk if not fixed:** `P1 USER_VISIBLE` data corruption — spec violation `Server.php:393 validateCurrentState` `restoring_backup` semantics. Fix is fencing, not data loss recovery — still `P1` but worth `P0` for race window.

---

### 3.4 P0 — `store_mounts_ext.go:323` allowlist too narrow (host breakout)

**Blast:** `POST /mounts` (`handlers_admin.go:1605`) requires `mounts.write` (admin scope) — compromised admin or stolen `mounts.write` API key can `source=/etc` `target=/host-etc` → `ServerMounts:513` → `beacon/runtime/docker.go:743` `buildContainerMounts` bind-mounts host dir into container → container breakout via `/proc`, `/var/run/docker.sock`, `/`, `/root`, `/etc/shadow`. Current allowlist only blocks 2+2 paths.

**Evidence:**
- `store_mounts_ext.go:316` `validateMountPaths`, `323` `validateMountPath:324` `path.IsAbs`, `324` `Clean==value`, `328` `..` check, **`332` `source == "/etc/forge" || source == "/var/lib/forge/volumes"`**, **`335` `target == "/" || target == "/home/container"`** — only 4 denied.
- `store_mounts_ext.go:297` `ensureMountAvailableForServer` requires both `mount_node` + `egg_mount` `300-306` — correct second gate but admin can `POST /mounts/:id/nodes:1776` + `/mounts/:id/eggs:1744` to link任意 node/egg.
- `store_mounts_ext.go:367` `AllowedMountSourcesForNode` `DISTINCT source` — panel not filtering host-sensitive sources.
- `handlers_admin.go:1605` `POST /mounts` → `CreateMount:41` → `validateMountPaths:48` → `290: 2014` (no collation), then pivots `mount_node:66`, `egg_mount:72` — no prefix check.
- `beacon/runtime/docker.go:743` `buildContainerMounts` trusts panel `source` verbatim — no daemon-side allowlist.

**Fix scope (defense-in-depth, additive):**
- Panel `store_mounts_ext.go:323` — deny-prefix list `/etc,/proc,/sys,/dev,/var/run,/run,/root,/boot,/usr,/home/container` or **allowlist prefix** `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts,/mnt/forge` (env-driven, default deny all outside). Require `source` starts with allowlist; keep existing reserved checks as fallback.
- Add migration (additive, non-breaking): `ALTER TABLE mounts ADD COLUMN IF NOT EXISTS allowlist_checked BOOLEAN DEFAULT true` is unnecessary — just code gate. Optionally `CREATE TABLE mount_allowlist(prefix TEXT PRIMARY KEY)` seeded via env.
- Beacon `docker.go:743` — add daemon-side `validateMountSource` second gate (same deny list) — prevents compromised panel DB.
- `handlers_admin.go:1605` 409 message should guide operator to prefix.

**Files to touch:** `store_mounts_ext.go:323-339`, `beacon/runtime/docker.go:743`, `handlers_admin.go:1605` error mapping, `docs/server-lifecycle.md` (document mount policy), `forge/web/components/server/mounts-view.tsx` (surface validation error).

**Frontend impact:** `mounts-view.tsx` + admin mounts list will show 400 with allowlist guidance when mounting `/etc` — no schema change, just error toast.

**Risk if not fixed:** Host breakout primitive without shell — `P0` if admin token stolen (`P1 OPERATOR_VISIBLE` otherwise → upgrade to `P0` for defense-in-depth).

---

## 4. Remainder of GH slice — status, gaps, files to touch, additive migrations, frontend impact, risks

### 4.1 Completeness quick ref

| GH | Status | Touch? | Additive migration? | Frontend? | Risk if deferred |
|---|---|---|---|---|---|
| GH-01 provisioning | COMPLETE keep | No | No | Add `provisioning` badge `lifecycle-machines.ts:41` `P3` | None |
| GH-02 desired/actual fence | COMPLETE+ fix fence | `store_state.go:39` widen to `crashed` | No | Two-dot badge `desired|actual` | reconciler livelock `P2` |
| GH-03 suspend | PARTIAL | `handlers_servers.go:1413` add `ensureTransferIdle`, `store_servers.go:374` CAS, `console-view.tsx:229` combined pill | No | Combined `stopped+suspended` pill | illegal `suspended+transferring` `P1` |
| GH-04 power | PARTIAL | `handlers_servers.go:888` fast 409 for `installing` before enqueue, docs | No | Document cancellation semantics | misleading 202 → async fail `P1` |
| GH-05 kill pierce | MISSING | `manager.go:487,581` separate `killMu`, `operation/service.go:368` bypass | No | No | emergency kill fails `P1` |
| GH-06 install | COMPLETE add `skip_scripts` | `clustermanager/service.go:232`, `beacon/server.go:727` honor `skip_scripts` | No | No | divergence `P2` |
| GH-07 reinstall | BROKEN fix fallback | `clustermanager/service.go:241` fallback `InstallServer` | No | Inline `stop first` 409 | reinstall always 500 `P1` |
| GH-08 delete+orphan | COMPLETE add UI | `clustermanager/service.go:348` phantom orphan when `runtime==nil` → `HardDeleteServer` | No | `GET /admin/orphans` `P3` | silent orphan table |
| GH-09 restoring | MISSING fencing | `handlers_servers.go:2117` + `store_state.go:31` | Optional `backups_restoring_idx` | `console-view.tsx:229` `RestoringBanner` | data corruption `P1` |
| GH-10 transfer | PARTIAL sync | `handlers_servers.go:145` query `migrations` or project `migrations.status→servers.transfer_state`, `server-lifecycle.md:12` | No | Unify `transfer-view.tsx:77` source | stale guard + UI flicker `P2` |
| GH-11 allocations | COMPLETE | Remove `Mappings` compat `clustermanager/service.go:698` or encode protocol | Already `090` | No | UDP also binds tcp `P2` |
| GH-12 start vs install race | BROKEN | `handlers_servers.go:888` `IsServerInstalling` guard | No | No | retry storm `P1` |
| GH-13 egg model | COMPLETE | `store_nests.go:172` `FOR UPDATE` on delete | No | No | FK race `P4` |
| GH-14 slash bug | **BROKEN P0** | `store_egg_variables.go:142` | No | No | blocks imports `P0` |
| GH-15 config parser | MISSING | `beacon/parser` port or strip `config.files` | No | Remove `config.files` from `egg-templates.ts` or doc unsupported | Minecraft wrong port `P2` |
| GH-16 templates | DUPLICATE | `SeedGameTemplates` boot upsert 14 | `092_seed_game_templates.sql` idempotent or boot seed (like `appstore/seed.go:216`) | Deprecate `app-templates` localStorage | 93% deficit `P1` |
| GH-17 schedules | PARTIAL | `store_schedules.go:252` add `isValidScheduleTaskAction` | No | No | arbitrary action persist `P2` |
| GH-18 wildcard | **BROKEN P0** | `store_users.go:306,555` subset+`*` gate | Audit `permissions` `*` rows | `users-view.tsx:25` hide `*` unless owner | escalation `P0` |
| GH-19 mounts | **BROKEN P1→P0** | `store_mounts_ext.go:323` allowlist | `MOUNTS_ALLOWED_PREFIX` env or `mount_allowlist` table | `mounts-view.tsx` error toast | host breakout `P0` |

### 4.2 Non-P0 but load-bearing gaps to call out (for Phase 01 scoping — defer or fold)

**GH-07 BROKEN — `clustermanager/service.go:241` reinstall gate**
- Drifter: `if !ok → return errors.New("runtime does not support reinstall")` makes Docker reinstall always 500, marks `install_failed` leaving `POWER_MACHINE` unusable. Beacon `server.go:1331` succeeds (calls `install`). Fix: one-line fallback.
- Files: `forge/api/internal/services/clustermanager/service.go:241-243`
- Frontend: `console-view.tsx:313` `Reinstall` button already `disabled` when `installing`; after fix, show `stop first` 409 guidance.
- Risk: `P1 USER_VISIBLE` — default reinstall path dead.

**GH-17 asymmetry — `store_schedules.go:252`**
- `CreateScheduleTask:252` only `action!=""` vs `Patch:331` `isValidScheduleTaskAction:1196` (`power|backup|command`). Arbitrary `action: "rm -rf"` persistable, rejected only at `schedule_runner.go:386`.
- Files: `store_schedules.go:252`, `store.go:1196`
- Fix: one-line `if !isValidScheduleTaskAction(req.Action) → error` additive; no migration.
- Risk: `P2` store pollution.

**REF-GAME-F-G-17/18 — console-view WS + network**
- `websocket-manager.ts:107` double parse drops plain daemon output; `console-view.tsx:203` cumulative `rx+tx` not delta. Both `console-view.tsx` fixes, no store change.
- Files: `forge/web/lib/api/ws/websocket-manager.ts:104-112`, `forge/web/components/server/console-view.tsx:148-156,197-206`
- Fix: manager should pass raw `event.data` string or object tag; console handler should branch `typeof data === "string" ? data : data.data ?? data.error`.

**REF-GAME-F-G-10 — `user_viewable` leak**
- `store_servers.go:273` allows non-admin `CreateServer StartupVariables` any `env_variable`; `store_startup.go:59` enforces `user_editable` for updates. Attacker can set `DL_PATH` at provision to hijack installer URL (`DL_PATH` `user_viewable:false`).
- Files: `store_servers.go:273-295`, `store_servers_control.go:154` vs `store_startup.go:31`
- Fix: gate CreateServer startup vars by `user_editable/viewable` for non-admin, or require `requireRole("admin")` for blind vars.

---

## 5. Files to touch (ordered, no new subsystem, minimal diff)

| Order | Severity | File:line | Change | Size |
|---|---|---|---|---|
| 1 | **P0** | `forge/api/internal/store/store_egg_variables.go:142-188` | Strip `regex:/…/` delimiters + flags before `regexp.Compile`; make `Split("|")` regex-aware (`regex:` payload consumes remainder). Add `RESERVED_ENV_NAMES` guard. | S ~40 LoC + tests |
| 2 | **P0** | `forge/api/internal/store/store_users.go:306,555` + `forge/api/internal/store/permissions.go:206` | Add `actorID` to `UpsertServerSubuser`, load actor perms, reject `requested ⊈ actor` + gate `*` behind owner/admin. | S ~60 LoC |
| 3 | **P0** | `forge/api/internal/http/handlers_servers.go:554,594` | Pass `actorID` (already `c.Locals("user")`), map 403; keep `requireServerPermission(PermUserCreate)` | S ~15 LoC |
| 4 | **P1→P0** | `forge/api/internal/store/store_mounts_ext.go:323-339` | Replace 2+2 allowlist with deny-prefix `/etc,/proc,/sys,/dev,/var/run,/run,/root,/boot,/usr,/var/lib/forge` or allowlist `MOUNTS_ALLOWED_PREFIX` env. | S ~30 LoC |
| 5 | **P1→P0** | `beacon/internal/runtime/docker.go:743` `buildContainerMounts` | Daemon-side second gate same deny-prefix — prevents compromised panel. | S ~25 LoC |
| 6 | **P1** | `forge/api/internal/http/handlers_servers.go:145,2117` + `forge/api/internal/store/store_state.go:31` + `forge/api/internal/services/backup/service.go:577` | `ensureRestoreIdle` guard + `SetServerActualState restoring_backup` on restore start/clear on completion; `MarkBackupStatus` stays per-backup. | S ~50 LoC |
| 7 | **P1** | `forge/web/components/server/console-view.tsx:229,37` | Add `RestoringBanner` + `blocked` includes `actual_state==='restoring_backup'` + `backups.status==='restoring'` check via `server` prop (or new `isRestoring`). | S ~30 LoC |
| 8 | **P1** | `forge/api/internal/services/clustermanager/service.go:241` | Fallback `if !ok → runtime.InstallServer` (mirror `beacon/server.go:1331`) or implement `DockerRuntime.Reinstall` alias. | S ~5 LoC |
| 9 | **P2** | `forge/api/internal/store/store_schedules.go:252` | `if !isValidScheduleTaskAction(req.Action) → error` one-line. | S 1 LoC |
| 10 | P2 | `forge/api/internal/store/store_state.go:39` | Widen `observed_generation` CASE to also converge on `crashed`+desired `running` → `running`. | S 2 LoC |
| 11 | P2 | `forge/api/internal/http/handlers_servers.go:888` | Fast 409 `IsServerInstalling` guard before `DispatchPower` (alongside `ensureTransferIdle`). | S ~10 LoC |
| 12 | P2 | `forge/web/lib/api/ws/websocket-manager.ts:104` + `forge/web/components/server/console-view.tsx:148` | Fix double parse: manager passes raw string or `{raw, parsed}`; console branches. | S ~20 LoC |
| 13 | P2 | `forge/web/components/server/console-view.tsx:197-206` | Delta network: store `prevRx/Tx`, `delta = (rx+tx) - prevTotal`. | S ~15 LoC |
| 14 | P3 | `forge/api/docs/server-lifecycle.md:12` | Replace 501 note with migration engine `forge-beacon-transfer/v1` + `migrations` table + `allocation_reservations`. | Doc |
| 15 | P3 | `forge/web/lib/api/servers.ts` + `forge/web/components/server/transfer-view.tsx:77` | Unify `transferring` source to single poll (`GET /transfer` superset). | S ~10 LoC |

No file outside game-hosting slice is required for P0s. `migrations` for mounts/restoring are optional additive indices.

---

## 6. Additive migrations (no destructive, no rewrite)

| Migration | File (proposed) | SQL / code | Why additive-safe |
|---|---|---|---|
| **OPTIONAL — mount allowlist env doc not table** | `forge/api/migrations/1xx_mount_allowlist_policy.sql` **or** env-only | `CREATE TABLE IF NOT EXISTS mount_allowlist(prefix TEXT PRIMARY KEY); INSERT INTO mount_allowlist(prefix) VALUES ('/srv/forge-mounts') ON CONFLICT DO NOTHING;` — OR keep env `MOUNTS_ALLOWED_PREFIX` and code gate only (preferred: no table). | Additive table if added; env-only has zero DB impact. Existing rows with `/etc/forge` still pass allowlist if prefix includes it — but they are legacy risky and will 400 until admin re-creates under new prefix (intentional). |
| **OPTIONAL — restoring cheap guard** | `forge/api/migrations/1xx_backups_restoring_idx.sql` | `CREATE INDEX CONCURRENTLY IF NOT EXISTS backups_restoring_idx ON backups(server_id) WHERE status='restoring';` | `CONCURRENTLY` no lock; queried by `ensureRestoreIdle:145` alternative path. No data change. |
| **REQUIRED — game templates seed** | `forge/api/migrations/1xx_seed_game_templates.sql` **or** boot `SeedGameTemplates` (preferred like `appstore/seed.go:216`) | Idempotent `INSERT ... SELECT ... WHERE name='Games' ON CONFLICT (nest_id,name) DO NOTHING` for 14 curated eggs from `forge/web/lib/egg-templates.ts:27` + vars; **or** Go `SeedGameTemplates` called at `cmd/api/main.go:529` after `SeedDefaultApps`. Include `normalizeDockerImages` shape `map`. | New rows only; existing `091` `Minecraft Java` variant untouched if `ON CONFLICT DO NOTHING` keyed on `(nest_id,name)`. If boot seed, same guarantee via `SELECT COUNT` guard. |
| **NOT NEEDED** | — | `allocations` unique already `090`: `UNIQUE (node_id,ip,port,protocol)` | Already additive-fixed. |
| **NOT NEEDED** | — | `eggs` parity `089` already present | Already additive. |

**Seeding deficit detail:** `091` inserts 1 egg with 2 vars (`VERSION`, `TYPE`). `egg-templates.ts:27` 14 entries include `minecraft-paper`, `minecraft-vanilla`, `minecraft-forge`, `minecraft-fabric`, `valheim`, `rust`, `ark`, `terraria`, `factorio`, `cs2`, `gmod`, `unturned`, `arkse`, `teamspeak` (example set varies by worktree) — each with `config_files` + `env[]` with `regex:/.../` rules that hit slash bug. Until P0 slash fix lands, seeding via migration that calls `CreateEggVariable` path will 400; migration raw `INSERT INTO egg_variables` bypasses `validateVariableValue` but still stores correct `rules` — future `UpdateServerStartupVariable` will then fail unless slash fix lands. Order: **fix slash bug first, then seed.**

---

## 7. Frontend impact

| Area | Path file:line | Impact if P0 fixed | Change size | UX note |
|---|---|---|---|---|
| **Nests/eggs CRUD** | `forge/web/app/admin/nests/page.tsx`, `[nestId]/eggs/page.tsx`, `[eggId]/variables/page.tsx`, `forge/api/internal/http/handlers_admin.go:990-1310` | Imports succeed; `CreateEggVariable` with `regex:/.../` now 201 not 400; PTDL `minecraft-paper` import works. | 0 FE (store fix) | After seed, wizard dropdown populates 14 not 1 — verify `GET /nests/:nestId/eggs` pagination. |
| **App-templates duality** | `forge/web/app/admin/app-templates/page.tsx` `app-templates-data.ts:3` | After single-source seed, deprecate `localStorage 5` `forge.app-templates.v1` or unify under DB `ListEggs` — `GET /eggs` becomes canonical for both game + app. | S | Show migration banner `Local templates migrated to DB`. |
| **Users/subusers** | `forge/web/components/server/users-view.tsx:25,36` + `handlers_servers.go:554` | Non-owner attempting `*` now 403 `cannot grant permission you do not hold`; `*` checkbox hidden unless `owner||admin`. Existing `*` rows remain `*` until admin audit — UI should flag `*` rows with warning pill. | S | Add error toast `You cannot grant *`. |
| **Console** | `forge/web/components/server/console-view.tsx:37-71,125,148-212,229,242-249` | `InstallBanner` + `TransferBanner` + new `RestoringBanner` render; `blocked` includes `restoring_backup` → power buttons disabled correctly; network sparkline trends delta throughput; daemon plain output no longer dropped (double-parse fix). | S-M | `console-view.tsx:229` `blocked` single source — add `server.actualState==='restoring_backup'` (requires DTO expose) or `server.status==='restoring_backup'`. |
| **Transfer** | `forge/web/components/server/transfer-view.tsx:31,77` + `forge/web/lib/api/servers.ts:394` | Unified `transferring` source stops flicker emerald/warning. `server-lifecycle.md:12` updated stops doc/UX lie. | S | Poll interval 5s unchanged; unify to `GET /transfer` presence. |
| **Schedules** | `forge/web/components/server/schedules-view.tsx:37` `TaskEditor` | `CreateScheduleTask` now rejects arbitrary `action` with 400 before list pollutes store — `TaskEditor` should validate `action ∈ {power,backup,command}` client-side before POST. | S | Add select not freeform. |
| **Mounts admin** | `forge/web/components/server/mounts-view.tsx` + `forge/web/app/admin/mounts/page.tsx` | Mount creation under `/etc`, `/proc`, `/var/run/docker.sock` now 400 with allowlist guidance; `AllowedMountSourcesForNode:367` still shows node sources but creation gate prevents breakout. | S | Error toast `source outside allowed prefix /srv/forge-mounts`. |
| **Server detail tabs** | `forge/web/components/server/server-tabs.tsx:20` `server-nav.tsx:17` | `suspended`+`transferring`+`restoring_backup` now exclusive locks — nav disables `Startup`, `Files`, `Databases` correctly. | S | Add `Restoring` pill badge alongside `Installing`/`Transferring`. |

---

## 8. Risks

### 8.1 If P0s ship together (recommended)
- **Regression — regex fix widens acceptance:** old 400 now 201 could persist previously-blocked rule strings that are malformed (e.g., `regex:/unterminated`). Mitigate: compile test `regexp.Compile` after strip — still 400 on genuinely invalid regex, not swallow.
- **Regression — wildcard gate:** existing `*` rows keep `*` — no data migration forces downgrade, so old escalation persists until admin audit. Mitigate: add `GET /admin/audits/subuser-wildcards` report query `SELECT ... WHERE permissions @> '["*"]'` and ops runbook.
- **Mount allowlist:** existing mounts with `/etc` already persisted will still mount in Beacon until next `ServerProvisionTarget` sync — but `validateMountPath` only guards `POST/PATCH`, not existing rows. Mitigate: add startup audit log `WARN mounts with prefix outside allowlist: 3 rows` at `CreateMount` read path, and `ListMounts:13` flag for admin UI.
- **Restoring lock:** `SetServerActualState restoring_backup` adds row to `state_transitions` + `observed_generation` fencing (`store_state.go:39`) — no extra migration, but `operation/service.go` backup worker must clear even on `RestoreBackup` exception path to avoid stuck `restoring_backup`. Mitigate: `defer clearActualState` in restore handler + operation handler.

### 8.2 If deferred
- **Slash bug deferred → GH-16 seed blocked:** Cannot fix `DUPLICATE` temple deficit without fixing validator — `POST /eggs/import` will keep 400, `SeedGameTemplates` via Go store path also calls `validateVariableValue` → 500 on boot. Must ship 3.1 before 6.x seed.
- **Wildcard deferred → `SE-01/SE-02` open:** `MASTER_FINDING_INDEX.md:58` `REF-GAME-F-G-22` remains `BROKEN P0` — low-priv subuser → `database.view_password`/`file.sftp` → secret exfil. Pen-test finding stays open.
- **Restoring deferred → data race window:** `P1` but trivial to hit via API (automation `restore + start` in same workflow). Half-written `server.properties` + running JVM reads corrupt world.
- **Mount allowlist deferred → host breakout:** `P0` if admin token stolen — but token theft is already `P0` elsewhere; fixing mount is defense-in-depth with near-zero cost — no reason to defer.

### 8.3 Sequencing constraint
1. `store_egg_variables.go:142` slash fix **first** — unblocks imports and seeding.
2. `store_mounts_ext.go:323` allowlist **second** — one-line, no dependency.
3. `store_users.go:306` wildcard subset **third** — needs `store_egg_variables` not blocking review queue.
4. `handlers_servers.go:2117` restoring lock **fourth** — needs `store_state` understanding but independent.
5. `SeedGameTemplates` **last** — depends on 1.

---

## 9. Verification checklist (for Phase 01 implementer — not this audit)

| P0 | Test |
|---|---|
| Slash bug | `go test ./internal/store -run TestCreateEggVariable // regex:/^([\w\d._-]+)(\.jar)$/ server.jar → ok` + `regex:/^(foo|bar)$/ "foo" → ok` + `in: easy,normal,hard` + legacy `POST /eggs/import minecraft-paper.json → 201` |
| Wildcard | `POST /servers/:id/users {perms:["*"]} as subuser with user.create → 403` + `as owner → 201` + `authenticate SFTP with * → fails for non-owner` |
| Restoring | `POST /backups/restore {name,truncate} → GET /servers/:id actual_state=restoring_backup` + `POST /power start during → 409 restoring_backup` + daemon `restoreBackup: failure → actual_state cleared` |
| Mount allowlist | `POST /mounts {source:/etc,target:/host-etc} → 400 "mount source must be under /srv/forge-mounts"` + `source:/srv/forge-mounts/data target:/data → 201` + `AssignMountToServer` with node/egg not in allowlist → 400 |
| GH-07 reinstall | `POST /servers/:id/reinstall while running → 409 must be stopped` + `while stopped Docker → 202` + `install_failed` not left |
| GH-17 schedules | `POST /schedules/:id/tasks {action:"rm -rf"} → 400` |
| Seed | Fresh `go run ./cmd/api --migrate` → `SELECT COUNT(*) FROM eggs → 14` + `ListEggs("")` returns `DockerImages` maps `images: {"Java 21": ...}` |

---

## 10. Appendix — file:line citational index (for reviewer click-through)

- `store_egg_variables.go:15` pattern, `17` struct, `42` List, `65` Create, `84` Update, `129` validateRequest, `142` validateVariableValue, `143` `Split("|")`, `145` `Cut(":")`, `176` `regexp.Compile(arg)`, `177-183` match
- `store_startup.go:11` `GetServerStartup:15`, `31` `user_viewable=true`, `50` `resolveStartupCommand`, `54` `UpdateServerStartupVariable:58 user_editable`, `73` validate, `90` `config_sync_pending`
- `store_servers.go:253` `provisioning`, `180` `CreateServer`, `234` `FOR UPDATE` allocations, `273` `StartupVariables` loop, `308` `GetServer`, `346` `IsServerTransferBlocking`, `374` `SetServerSuspension`, `397` `SetServerSuspended`
- `store_servers_control.go:13` `SetServerPowerState`, `44` `powerSignalPriorStates`, `51` `stop/kill allow installing`, `57` `ServerControlTarget`, `82` `ServerProvisionTarget:95 ConfigJSON`, `154` `Environment all vars`, `179` `resolveStartupCommand`, `207` `SetServerProvisioned`, `218` `SetServerInstallState`, `244` `MarkServerConfigSynced`
- `store_nests.go:16` Nest, `34` Egg, `100` ListNests, `172` DeleteNest `COUNT(*)`, `190` ListEggs, `248` CreateEgg, `292` UpdateEgg, `361` `normalizeDockerImages`, `394` JSONObject
- `store_templates.go:14` ListTemplates compat, `38` CreateTemplate via `Games` nest `48`, `99` `templateFromEgg`
- `store_allocations.go:15` AllocationNode, `23` `ListAllocationNodes`, `133` `CreateAllocations:145 ParseIP,151 protocol,155 containerPort,182 pg23505`, `220` `DeleteAllocations`, `327` `AssignAllocationToServer`, `350` `Unassign`, `367` `SetPrimaryAllocation:374 FOR UPDATE`
- `store_state.go:10` `SetServerDesiredState:16 generation`, `31` `SetServerActualState:39 observed_generation`, `131` `serverStatusFromActual restoring_backup`
- `store_schedules.go:14` `resolveStartupCommand`, `26` List, `166` Create, `208` Patch, `252` `CreateScheduleTask:253 action!=""`, `331` `PatchScheduleTask:332 isValid`, `398` `NOTIFY schedule_events`, `489` `ListDueSchedules:500 only_when_online`
- `store_users.go:96` Authenticate, `131` ListUsers, `226` CreateUser, `280` `ListServerSubusers`, `306` `UpsertServerSubuser:311 normalize,330 owner check`, `362` `UserCanAccessServer:377 HasPermission:406`, `421` `AuthenticateSFTP:464 * or file.sftp`, `483` `AuthenticateSFTPPublicKey`, `521` `AuthorizeSFTPSession`, `555` `normalizeSubuserPermissions:564 allows *`, `574` `defaultSubuserPermissions`
- `store_mounts_ext.go:13` List, `41` Create `48 validate`, `121` Update `177 allowlist`, `194` `AttachEggToMount`, `226` `ServerMountsForMount`, `253` `DeleteMount`, `272` `AssignMountToServer:276 Tx`, `297` `ensureMountAvailableForServer:300 JOIN`, `316` `validateMountPaths`, `323` `validateMountPath:332 source reserved,335 target reserved`, `367` `AllowedMountSourcesForNode`
- `handlers_servers.go:145` `ensureTransferIdle:151 IsServerTransferBlocking`, `161` `registerServerRoutes`, `501` `GET /servers/:id/users`, `554` `POST /users`, `789` `POST /servers:838 CreateServer`, `888` `POST /power:889 ensureTransferIdle`, `1050` `POST /install:1051 ensureTransferIdle`, `1110` `POST /reinstall:1129 DispatchReinstall`, `1413` `POST /suspension`, `2117` `POST /backups/restore:2170 MarkBackupStatus restoring`
- `handlers_admin.go:990` `GET /nests`, `1016` `POST /nests`, `1043` `PATCH /nests`, `1067` `DELETE /nests`, `1085` `GET /eggs`, `1098` `GET /nests/:nestId/eggs`, `1111` `GET /eggs/:id`, `1124` `POST /eggs`, `1156` `PATCH /eggs`, `1185` `GET /eggs/:id/variables`, `1198` `POST /eggs/:id/variables`, `1226` `PATCH .../variables/:variableId`, `1254` `DELETE`, `1288` `GET /eggs/:id/export`, `1308` `POST /eggs/import` loop, `1365` `GET /allocations/nodes`, `1378` `GET /allocations`, `1579` `GET /mounts`, `1605` `POST /mounts`, `1687` `PATCH /mounts`, `1744` `POST /mounts/:id/eggs`, `1776` `POST /mounts/:id/nodes`, `1810` `POST /mounts/:id/servers`
- `forge/api/docs/server-lifecycle.md:5` provisioning, `6` orphan, `12` `501` lie
- `forge/api/migrations/043_unify_eggs_templates_mounts.sql:5` parity, `14` array→map, `20` Legacy Templates, `63` `egg_id` FK, `143` trigger
- `089_egg_parity_fields.sql:1` `config_from…features`
- `091_seed_minecraft_java.sql:3` 1 egg, `31` 2 vars
- `015_a_mounts.sql:13` `mount_node`, `19` `egg_mount`, `25` `mount_server`
- `090_allocation_transport.sql:8` `UNIQUE (node_id,ip,port,protocol)`
- `forge/web/lib/egg-templates.ts:27` 14 static, `58` `regex:/^([\w\d._-]+)(\.jar)$/`, `63` `config.files`
- `forge/web/app/admin/nests/page.tsx`, `[nestId]/eggs/page.tsx`, `[eggId]/variables/page.tsx`
- `forge/web/app/admin/app-templates/page.tsx` `app-templates-data.ts:3` localStorage 5
- `forge/web/components/server/console-view.tsx:37` `InstallBanner`, `52` `TransferBanner`, `91` `Chart`, `125` `canConsole`, `148` `WebSocketManager`, `154` double-parse, `197` stats `network:203 cumulative`, `229` `blocked`, `242` charts
- `forge/web/lib/api/ws/websocket-manager.ts:104` `JSON.parse(event.data)` + `107` `onMessage(data)` object
- `store.go:1196` `isValidScheduleTaskAction`

---

## 11. What was NOT inspected (out of slice — noted for other subagents)

- `beacon/internal/runtime/docker.go` full `validateCreateRequest:901` CPU/memory/IO gating — skimmed only for GH-05,12 mapping; full cgroup v1/v2 parity is runtime-compose slice (Agent 05).
- `services/noderegistry`, `heartbeatmonitor`, `reconciler`, `clustermanager` placement scoring — only referenced for `F-G-13/F-G-15`.
- `services/migration` dual-credential protocol `beacon/transfer/protocol.go` — only surface via `migration/service.go:102` `forge-beacon-transfer/v1`.
- `internal/store/store_database_services.go` / `managed_databases` — database-templates slice (Agent 08).

---

*End of preparation audit. Do not modify code per task. File:line citations above are the implementation hooks for Phase 01 execution.*
