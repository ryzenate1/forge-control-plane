# Subagent 02 — GAME SERVER TEMPLATES / EGGS / STARTUP / VARIABLES / ALLOCATIONS / RESOURCES

**Date:** 2026-08-23
**Dimension:** eggs / nests / templates / startup commands / variables / docker images / allocations / ports / resource limits (cpu/mem/disk/io/swap/oom), PufferPanel templates vs packages/game-templates
**Cluster:** game hosting (6 projects)
**Source refs:** `reference/game-hosting/{pterodactyl-panel,pelican-panel,pterodactyl-wings,pufferpanel-templates}` vs `forge/{api,web}`, `beacon`, `packages/game-templates`, `packages/shared-types`

---

## 1. Reference Model Inventory

### 1.1 Pterodactyl Panel — canonical

| Concept | Table / File | Key fields |
|---------|--------------|------------|
| Nest (service category) | `nests` via `app/Models/Nest.php:19` | `id, uuid, author, name, description`, hasMany eggs `Nest.php:54` |
| Egg (template) | `eggs` via `app/Models/Egg.php:52` | `id, uuid, nest_id, author, name, docker_images JSON, startup, config_files/startup/logs/stop, script_install/container/entry, file_denylist, features, force_outgoing_ip, copy_script_from, config_from`, EXPORT_VERSION `PTDL_v2` `Egg.php:68` |
| EggVariable | `egg_variables` via `app/Models/EggVariable.php:29` | `id, egg_id, name, env_variable, default_value, user_viewable, user_editable, rules, required computed` |
| ServerVariable | `server_variables` (`2016_01_23_201649_add_server_variables.php:14`) | `server_id, variable_id, variable_value` |
| Server | `app/Models/Server.php:109` | `memory, swap, disk, io, cpu, threads, oom_disabled, allocation_id, nest_id, egg_id, startup, image`, validation `Server.php:162` |
| Allocations | `allocations` (`2016_01_23_195641_add_allocations_table.php:13`) | `id, node, ip, port, assigned_to` ; richer later |

### 1.2 Pelican Panel — delta on top

`reference/game-hosting/pelican-panel/app/Models/Egg.php:1` retains identical schema but adds:
- `tags: string[]` and `startup_commands: array<string,string>` (new columns)
- `children`, `mounts` relationships, `HasIcon` trait, stricter Validatable contract
- Same inheritance accessors (`getInheritConfigFilesAttribute`, etc.) preserved.

### 1.3 Pterodactyl Wings — runtime consumption

- `reference/game-hosting/pterodactyl-wings/remote/types.go:45` — `ServerConfigurationResponse` carries `settings: json.RawMessage` (the `Configuration` blob) + `ProcessConfiguration` (startup/done/stop/configs).
- `reference/game-hosting/pterodactyl-wings/server/configuration.go:22` — `Configuration` embeds `environment.Allocations`, `environment.Limits`, `environment.Variables`, `EggConfiguration`.
- `reference/game-hosting/pterodactyl-wings/server/server.go:151` — `GetEnvironmentVariables()` injects fixed `TZ/STARTUP/SERVER_MEMORY/SERVER_IP/SERVER_PORT` then appends `EnvVars` upper-cased, skipping duplicates.
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` — `Limits{MemoryLimit, Swap, IoWeight, CpuLimit, DiskSpace, Threads, OOMDisabled}` with helpers `ConvertedCpuLimit()`, `BoundedMemoryLimit()`, `ConvertedSwap()`, `AsContainerResources()` (cgroup-aware `BlkioWeight`).

### 1.4 PufferPanel Templates — typed ops model

- `reference/game-hosting/pufferpanel-templates/spec.json:1` — JSON Schema 2020-12, `$id=https://raw.githubusercontent.com/pufferpanel/templates/v3/spec.json`. Top-level keys: `id, type, display, data (variables), environment/supportedEnvironments, requirements, install[], uninstall[], run{command, pre, post, stop/stopCode, stdin/stdout, environmentVars, autostart/autorecover/autorestart}, groups, stats, query`.
- Variables: `spec.json:201` `$defs/variable` with `type ∈ {option,string,boolean,integer}`, `value, display, desc, required, userEdit, internal, options[]`; `option` type requires `options`.
- Operations: `spec.json:307` `$defs/operation` enumerates 26 typed ops (`mojangdl, paperdl, steamgamedl, javadl, forgedl, neoforgedl, writefile, alterfile, download, extract, command, archive, move, mkdir, dockerpull, curseforge, fabricdl, spongedl, resolveforgeversion, …`) each with `if: string` conditional expression and type-specific required fields.
- Example `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:2` — `type: minecraft-java`, host+docker envs, conditional `install` pipeline, `run.command[]` with per-entry `if` clauses, `environmentVars`, `supportedEnvironments[host,docker]`.

### 1.5 Packages / Forge shared catalog

- `packages/game-templates/template-schema.json:1` — draft-07 schema, `$id=https://gamepanel.io/schemas/game-template.json`, required `id,name,description,version,game,image,startup,config,ports,env,resources,install_script,supported_platforms,categories`.
- `packages/game-templates/src/types.ts:42` + `template-schema.json:25` — `GameTemplate{image, images?, startup, config{files,startup.done,stop,logs}, ports[{port,protocol,public}], env[{name,env_variable,default_value,user_viewable,user_editable,rules}], resources{cpu,memory_mb,memory_max_mb,disk_mb,cpu_shares,io_weight,swap_mb}, install_script{container,entrypoint,script}, …}`.
- `packages/game-templates/templates/minecraft-paper.json:10` — concrete example (`image=ghcr.io/pterodactyl/yolks:java_21`, `images{Java 21, Java 17}`, `startup=java … {{SERVER_JARFILE}}`, `config.files.server.properties`, `resources{cpu:100,memory_mb:2048, …}`, `env[]` with `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` etc.)

---

## 2. Forge Implementation Inventory

| Layer | File(s) | Purpose |
|-------|---------|---------|
| DB schema | `forge/api/migrations/007_postgres_core_foundation.sql:87` `94` `013_startup_variables.sql:1` `043_unify_eggs_templates_mounts.sql:1` | `nests(id,name UNIQUE,description)`, `eggs(id,nest_id, docker_images JSONB, startup TEXT, config JSONB, default_memory_mb …)`, `egg_variables(egg_id, env_variable UNIQUE per egg, rules TEXT)`, `server_variables(server_id, variable_id PK)`, unify `server_templates → eggs` |
| Store: nests/eggs | `forge/api/internal/store/store_nests.go:16` `34` `98` `188` | `Nest{Name,Description,EggCount}`, `Egg{DockerImages json.RawMessage, Startup, Config, DefaultMemoryMB, InstallScript…, ConfigFrom/CopyScriptFrom, Features…}` |
| Store: egg variables | `forge/api/internal/store/store_egg_variables.go:17` `31` `129` `142` | `EggVariable{Name,EnvVariable,DefaultValue,UserViewable,UserEditable,Rules,Sort}`, `validateVariableValue()` |
| Store: startup | `forge/api/internal/store/store_startup.go:11` `54` | `GetServerStartup`, `UpdateServerStartupVariable` with `validateVariableValue` + `config_sync_pending` |
| Store: templates | `forge/api/internal/store/store_templates.go:12` `99` | `ListTemplates/GetTemplate` as **derived view over eggs** via `templateFromEgg()` |
| Store: allocations | `forge/api/internal/store/store_allocations.go:15` `125` `214` `327` `367` | `Allocation{IP,Port,ContainerPort,Protocol,Alias}`, atomic `CreateAllocations`, `AssignAllocationToServer`, `SetPrimaryAllocation` |
| Store: servers | `forge/api/internal/store/store_servers.go:180` `308` | `CreateServer` validates limits, mounts `startupVariables` via `egg_variables` rules; `GetServer` joins `eggs.docker_images` |
| Store: provision target | `forge/api/internal/store/store_servers_control.go:82` `104` | `ServerProvisionTarget` aggregates `Environment map[string]string`, `Mounts`, `Allocations[]`, resources `MemoryMB/SwapMB/CPUShares/CPULimit/DiskMB/IOWeight/Threads/OOMDisabled`, resolves `{{VAR}}` via `resolveStartupCommand()` |
| HTTP: nests/eggs | `forge/api/internal/http/handlers_admin.go:990` `1083` `1185` `1288` `1308` | CRUD `/nests`, `/eggs`, `/eggs/:id/variables`, `/eggs/:id/export`, `/eggs/import` |
| HTTP: templates compat | `forge/api/internal/http/handlers_admin.go:2037` | `/templates` routes delegate to `Store.ListTemplates` (egg-derived) |
| HTTP: allocations | `forge/api/internal/http/handlers_admin.go:1365` `1378` `handlers_servers.go:454` `634` | `/allocations`, `/allocations/nodes`, `/servers/:id/allocations`, primary alias bulk |
| Daemon client | `forge/api/internal/daemon/client.go:401` | `CreateRequest{Image, Command[], Env[], Ports[], Mounts[], MemoryMB/SwapMB/CPUShares/CPUPercent/IOWeight/OOMKillDisabled, PIDLimit, StopSignal…}` + `ServerConfiguration{Environment, Invocation, Allocations, Build, Mounts}` |
| Beacon runtime | `beacon/internal/runtime/runtime.go:55` | `CreateRequest{Image,Command,Env,Ports,Mounts,MemoryMB,SwapMB,CPUShares,CPUPercent,IOWeight,OOMKillDisabled,PIDLimit,StopSignal/Network…}`; `beacon/internal/runtime/docker.go:901` `validateCreateRequest` |
| Web admin | `forge/web/app/admin/nests/page.tsx:1` `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1` `forge/web/lib/egg-templates.ts:1` | `AdminNestsEggs` CRUD, `EggCard`, `EGG_TEMPLATES[]` (hardcoded fallback mirror), `apps-template` localStorage templates |
| Shared types | `packages/shared-types/src/api.ts:887` `896` `1262` `1318` | `ApiNest`, `ApiEgg`, `ApiStartupVariable`, `ApiTemplate`, `CreateEggInput` |

---

## 3. Detailed Comparisons (15+)

### C01 — Nests: first-class table vs unique-name contract

- **Ref panel:** `app/Models/Nest.php:20` table `nests(id,uuid,author,name)` with `HasFactory`, activity logging; name unique per Pterodactyl seeders.
- **Forge:** `store_nests.go:87` `CREATE TABLE nests(id UUID PK, name TEXT NOT NULL UNIQUE, description TEXT)` `store_nests.go:137` `CreateNest` trims name and requires non-empty. Single default `Games` nest `007:106`. No `uuid/author` columns — Forge uses plain UUID PK without separate `uuid` column. **Gap: Forge drops `author`/audit on nests; migration doesn’t preserve pelican `uuid` alias.**

### C02 — Eggs: schema parity with normalization differences

- **Ref:** `app/Models/Egg.php:84-108` columns `docker_images (array cast), config_files/startup/logs/stop (JSON strings), file_denylist (array), script_is_privileged, force_outgoing_ip, copy_script_from/config_from (FK inherits)`, validation `docker_images.* regex /^[\w#\.\/\- ]*\|?~?[\w\.\/\-:@ ]*$/` `Egg.php:134`.
- **Forge:** `store_nests.go:34` `Egg{DockerImages json.RawMessage, Config json.RawMessage, DefaultMemoryMB int, InstallScript…, FileDenylist, UpdateURL, Features}` + `store_nests.go:361` `normalizeDockerImages` accepts **either** `map[string]string` **or** legacy `[]string` and normalizes to map — more permissive than panel’s map-only expectation after `2022_05_07` migration. **Improvement: Forge handles both shapes; panel rejects array.**

### C03 — Docker images: single deprecated string vs map

- **Ref:** `app/Models/Egg.php:19` `@property string $docker_image -- deprecated, use $docker_images` + `app/Models/Server.php:89` `whereImage` regex `^~?[\w\.\/\-:@ ]*$`. Panel kept legacy `docker_image` for back-compat.
- **Forge:** `store.go:848` `StartupDetails.DockerImages map[string]string`, `store_startup.go:14` picks `COALESCE(NULLIF(s.docker_image,''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1),'')` `store_nests.go:99` `templateFromEgg` sorts keys and picks `keys[0]`. Keeps compat but **Forge correctly prefers server override else lexicographically-first image**, matching panel intent.

### C04 — Startup command templating: {{VAR}} vs {{SERVER_MEMORY}}/{{config.…}}

- **Ref panel startup:** `app/Models/Egg.php:29` `startup` contains `{{SERVER_MEMORY}}`, `{{SERVER_PORT}}`, `{{SERVER_IP}}` etc. Resolved at server creation via `StartupModificationService` + `VariableValidatorService`.
- **Forge client-type template:** `packages/game-templates/templates/minecraft-paper.json:15` `startup: "java … {{SERVER_JARFILE}}"` (double-brace). `store_servers_control.go:178` calls `resolveStartupCommand(target.StartupCommand, target.Environment)` — which substitutes **only egg variables**, not `SERVER_MEMORY/IP/PORT`. The fixed injections (`SERVER_MEMORY` etc.) are injected as env vars, not startup placeholders.
- **Wings/Pelican:** `wings/environment/settings.go` + `server.go:151` inject `SERVER_MEMORY/SERVER_IP/SERVER_PORT` as env + `STARTUP` raw.

### C05 — Variables: reserved names & validation engine

- **Ref:** `app/Models/EggVariable.php:43` `RESERVED_ENV_NAMES = SERVER_MEMORY,SERVER_IP,SERVER_PORT,ENV,HOME,USER,STARTUP,SERVER_UUID,UUID`, regex `^[\w]{1,191}$` `notIn:RESERVED` `EggVariable.php:70`. `VariableValidatorService.php:28` filters `user_editable && user_viewable` when not admin, builds `rules['environment.'+env_variable] = variable->rules` and runs Laravel `ValidationFactory`.
- **Forge:** `store_egg_variables.go:15` `eggVariableNamePattern = ^[A-Z][A-Z0-9_]*$` (stricter, uppercase), `validateVariableValue()` `store_egg_variables.go:142` manually implements `required|nullable|string|max|min|in|regex`; `store_startup.go:54` enforces `user_viewable=true` + `user_editable` gate and re-validates with same helper. **No reserved-names list** — Forge relies on pattern + not exposing `SERVER_*` as egg variables. Shared-types `ApiStartupVariable:1262` carries `rules: string` passthrough.
- **Logic gap:** Forge’s regex rule compiles with `regexp.Compile(arg)` `store_egg_variables.go:177` without anchoring; panel uses Laravel regex which expects `/pattern/flags`. Game-templates ship `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"` `minecraft-paper.json:58` — Forge’s split on `:` would extract `/^([\w\d._-]+)(\.jar)$/` including `/` delimiters, which `regexp.Compile` will treat as literal slashes and **reject valid `server.jar`** or accept invalid. This is a logic bug (see Findings).

### C06 — PufferPanel typed variables vs Forge/Ptero string-rules

- **PufferPanel:** `spec.json:201` `variable {type: option|string|boolean|integer, value: string|number|boolean, options: [{value,display}], required, userEdit, internal}`. Groups with `if: string` conditionals `spec.json:160` and `order`.
- **Ptero/Forge:** variables are stringly-typed with `rules` pipe-string. Puffer’s `type: integer` would be `rules: "required|integer|min:1|max:32"` in Forge/Ptero (`palworld.json:82` `MAX_PLAYERS`). **Forge loses type safety** — everything is string coercion; numeric bounds checked via string length if mis-used. No `groups`/`if` conditional UX in Forge admin (see C12).

### C07 — PufferPanel operations vs Ptero/Forge single script

- **PufferPanel:** `spec.json:58` `install: operation[]` with 26 `type` values, each operation has `if:` expression (`javadl, mojangdl, paperdl, steamgamedl, writefile, move, command, download…`) `minecraft.json:188` shows 18-step conditional install pipeline.
- **Ptero:** `Egg.php:30-34` single `script_install + script_entry + script_container` (one shell script). Pelican adds `startup_commands` JSON `pelican Egg.php`.
- **Forge client templates:** `template-schema.json:95` `install_script: {container, entrypoint, script: string}` (single script) `minecraft-paper.json:124` 80-line `#!/bin/ash` curl+jq workflow. **Forge collapses Puffer’s declarative pipeline into an imperative shell blob** — loses idempotent typed operations and `if` evaluation.

### C08 — Allocations / ports: pool vs static template ports

- **Ptero:** `allocations` table global pool `(node, ip, port, assigned_to nullable)`; server `allocation_id` FK unique `Server.php:167`; `getAllocationMappings()` groups by `ip → port[]`.
- **Forge:** `store_allocations.go:125` `CreateAllocations` atomically inserts `allocations(node_id, ip, port, container_port, protocol)` with `ip` as `inet` (`007:134`), `protocol ∈ {tcp,udp}` + `container_port` defaulting to `port`, bulk transaction with `pg 23505` duplicate handling `store_allocations.go:182`. `Allocations` carry `HostIP/HostPort/ContainerPort/Protocol` to `CreateRequest.Ports[]` `daemon/client.go:438` and `runtime.PortBinding:100`.
- **Game-templates static ports:** `template-schema.json:53` `ports[{port,protocol,public}]` — **not** pool entries; e.g., `minecraft-paper.json:33` declares `25565 tcp public`. Forge never auto-provisions allocations from template `ports`; allocation pool must be pre-created via admin `/allocations`. **PufferPanel instead** embeds `portBindings: ["0.0.0.0:${port}:${port}/tcp"]` per environment `minecraft.json:398`.

### C09 — Resource limits: cpu/mem/disk/io/swap/oom/threads

| Limit | Ptero `Server.php:153` + `environment/settings.go:37` | Forge `store_servers.go:209` `store_servers_control.go:120` `beacon/runtime/docker.go:901` `shared-types/api.ts:98-104` |
|-------|------------------------------------------------------|-----------------------------------------------------------------------------------|
| `memory` (MB) | `required numeric min:0` `Server.php:160`, Bounded `*OverheadMultiplier` 5/10/15% | `CreateServer` `MemoryMB <=0 → error` `store_servers.go:209`, `CreateRequest.MemoryMB int64` `runtime.go:62`, validated `MemoryMB <0 → error` `docker.go:908`, ApiServer.memoryMb |
| `swap` | `required numeric min:-1` `-1 = unlimited` `Server.php:161` `ConvertedSwap() => -1 or memory+swap` | `SwapMB < -1 → error`, `-1 allowed`, `SwapMB>0 && MemoryMB==0 → error` `docker.go:914`, `ApiServer.swapMb` |
| `cpu` | `cpu = percent 0..THREADS*100`, `ConvertedCpuLimit = cpu*1000` `settings.go:66`; `io` column is actually generic | `CPUShares` (int, 2..262144) vs `CPUPercent/CPULimit` conflation: `store_servers.go:209` checks `CPULimit <0`, `docker.go:924` checks `CPUPercent 0..100000`, but `Daemon CreateRequest` has **both** `CPUShares` and `CPUPercent` `daemon/client.go:410` while `runtime.CreateRequest` has `CPUShares/CPUPercent` `runtime.go:65` — mapping is ambiguous (see Findings) |
| `disk` | `disk required numeric min:0` MB, enforced via `filesystem.SetDiskLimit` | `DiskMB int64` in `CreateRequest:426`, validated only as `DiskMB>0` in store, no FS quota in beacon docker runtime (relies on `quota` package `beacon/internal/quota`) |
| `io_weight` | `10..1000` `Server.php:162` `BlkioWeight` conditional on cgroup v2 `settings.go:116` | `IOWeight 10..1000` `store_servers.go:209`, `docker.go:927` `IOWeight 0 or 10..1000`, `beacon/internal/runtime/docker.go:827` builds `HostConfig` weight similarly |
| `oom_disabled` | `sometimes boolean` default `true` `Server.php:137` | `OOMKillDisabled bool` `runtime.go:69` `daemon/client.go:414`, `--oom-kill-disable`  |
| `threads` | `nullable regex /^[0-9-,]+$/` `Server.php:164` → `CpusetCpus` | `CPUSet string` `runtime.go:67` `CpusetCpus` checked only if non-empty `settings.go:132`; `ApiServer.threads` |
| `swap/io/oom` coverage | complete | Forge `Server` lacks `memoryOverallocate`/`cpuOverallocate` from `ApiNode:176-197` but server-level parity is good |

### C10 — Docker images JSON shape

- **Ptero:** `Egg.php:120` cast `docker_images => array`, validation `docker_images.* regex /^[\w#\.\/\- ]*\|?~?[\w\.\/\-:@ ]*$/` `Egg.php:134`.
- **Game-templates:** `template-schema.json:29` `images: object<label,string>` plus `image: string` default.
- **Forge store:** `store_nests.go:361` handles both `map[string]string` and legacy `[]string` array; `forge/api/internal/daemon/client.go:401` `Image string` single, `ServerProvisionTarget.Image string` single. `beacon/runtime/docker.go:901` validates image non-empty but **does not validate label map** — registry auth separate.

### C11 — Template vs Egg dualism

- **Intended:** `forge/api/internal/store/store_templates.go:12` comment `ListTemplates is a compatibility transform over canonical eggs. It does not read from or create rows in the legacy server_templates table.` `043_unify_eggs_templates_mounts.sql:63` migrates `server_templates → eggs`, creates `template_id` alias `+ servers_egg_id_fkey` `043:86`.
- **Actual:** `GET /templates` and `POST /templates` (`handlers_admin.go:2037`) are **compatibility shims** that read/write eggs via `CreateEgg` with `Games` nest fallback `store_templates.go:48`. `packages/game-templates` ships a **separate file-system catalog** `templates/*.json` + `src/index.ts:6` `templateToApiTemplate()` converter that is **not** backed by DB — it’s a static bundle consumed by web `egg-templates.ts`. Web admin **does not** import via `POST /eggs/import` automatically; templates are UI helpers that pre-fill egg creation form. **Two template systems without sync.**

### C12 — App-templates (userland) vs game-templates (curated)

- **Forge app-templates:** `forge/web/app/admin/app-templates/page.tsx:1` + `forge/web/lib/app-templates-data.ts:5` — client-side `localStorage` (`forge.app-templates.v1`) with types `AppTemplate{type: image|git|compose, image|gitUrl|composeContent, defaultPorts[], defaultEnvVars{}, defaultResources{cpu,memory,disk: string}}`. Entirely **in-browser**, no API persistence, no egg binding. Used for “Create Application” wizard, not game servers.
- **Game-templates package:** `packages/game-templates/src/types.ts:42` `GameTemplate` + `template-schema.json:8` curated 15 entries `index.json:3` (minecraft-paper/vanilla, palworld, valheim, …). **No runtime API** serves this registry — web imports via `egg-templates.ts:27` static array.
- **Pterodactyl catalog:** panel has **no app-templates concept**; catalog is eggs. Forge conflates three concepts: `nests/eggs` (DB), `server_templates` shim, `game-templates` (FS), `app-templates` (localStorage).

### C13 — Startup variable lifecycle: create/change/sync

- **Ptero:** `VariableValidatorService.php:30` `EggVariable::where(egg_id, egg)` filtered by `user_editable/viewable` if not admin; builds `environment[env_variable]`, throws `ValidationException`. `StartupModificationServiceTest` verifies sync.
- **Forge:** `store_startup.go:11` `GetServerStartup` joins `egg_variables` filtered `user_viewable=true`, resolves `server_variables` fallback to `default_value`, injects `env[EnvVariable]=ServerValue`, then `StartupCommand = resolveStartupCommand(RawStartupCommand, env)`. `store_startup.go:54` `UpdateServerStartupVariable` checks `user_editable`, validates `validateVariableValue`, `INSERT … ON CONFLICT UPDATE` + `config_sync_pending=true` + audit `store_startup.go:90`. `store_servers.go:273` also validates `StartupVariables` at creation time via same helper.
- **Beacon sync:** `store_servers_control.go:178` `resolveStartupCommand` used for provision target; update pends `config_sync_pending` polled by reconciler (`services/reconciler`). **Parity good**, but Forge lacks panel’s per-variable `notIn:RESERVED` guard.

### C14 — Mounts / file denylist / config parser

- **Ptero:** `Mount` model via `egg_mount` pivot `015_a_mounts.sql:19`; egg `file_denylist` `Egg.php:23` `2021_01_10 migration`; `config_files` JSON parsed by `parser` package ` wings/parser/parser.go:1` supporting `file/yaml/properties/ini/json/xml` with `find/replace` rules, wildcard `.*`, array `something[1]`, `configMatchRegex {{config.docker.interface}}`, `xmlValueMatchRegex`.
- **Forge:** `store.go:698` `ServerProvisionTarget.Mounts []ServerMount` loaded via `ServerMounts(ctx, serverID)` `store_servers_control.go:179`; `Daemon ServerConfiguration.Mounts []Mount{Source,Target,ReadOnly}` `daemon/client.go:468`; `Runtime.Mount{Source,Target,ReadOnly}` `runtime.go:94`. Beacon `docker.go:808` `buildHostConfig` mounts via `mount.Mount`. **File denylist** passthrough `ServerProvisionTarget.FileDenylist string` but **no parser implementation** — `config` JSON is stored but wings parser `ConfigurationFiles []parser.ConfigurationFile` (`remote/types.go:152`) is **not reimplemented** in beacon; config file patching is missing (only env/startup).
- **Game-templates:** `minecraft-paper.json:16` `config.files["server.properties"]{parser:properties, find:{server-port: "{{server.build.default.port}}"}}` — correctly expects wings parser to patch `server-port` to `SERVER_PORT` allocation. Forge currently **does not apply** this on provision.

### C15 — Allocations API surface: bulk, pagination, scoping

- **Ptero:** `Allocation` via node IP block generation (`app/Services/Allocations/*`), single IP+port range creation.
- **Forge:** `store_allocations.go:69` `ListAllocationsWithTotal(page,perPage)`, `133` `CreateAllocations(requests[])` bulk atomic with Tx + audit per alloc, `214` `DeleteAllocations(ids[])` fails entirely if assigned. HTTP `GET /allocations?limit&offset` `handlers_admin.go:1378` paginated `50` default, `GET /allocations/nodes` `handlers_admin.go:1365`, `POST /servers/:id/allocations {allocationId}` `handlers_servers.go:634`, `DELETE …`, `POST …/primary`. **PufferPanel** has no allocation pool — ports are dynamic `portBindings` interpolated at create.

### C16 — Web admin: nests/eggs/variables/templates UI

- **Ptero/Pelican admin UI:** React (Panel) with `nests → eggs → variables` nested routes, import/export JSON (`Egg.php:68` `EXPORT_VERSION PTDL_v2`), docker image map editor.
- **Forge web:** `forge/web/app/admin/nests/page.tsx:1` `AdminNestsEggs` aggregates both nests+eggs in one view; `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1` full CRUD (name/description/dockerImages newline list, startup, stop, features, installScript/container/entrypoint) + `EggCard` with `Settings/Copy/Download/Trash` and `Browse Templates →` CTA. Variables are sub-route `…/eggs/[eggId]/variables/page.tsx:1` with `fetchEggVariables/createEggVariable/updateEggVariable/deleteEggVariable/reorderEggVariables` `forge/web/lib/api.ts:1340`. `EXPORT_VERSION` not exposed — export via `GET /eggs/:id/export` `handlers_admin.go:1288` returns `{egg,variables}` raw, not PTDL_v2 envelope.

### C17 — Shared-types: DTO divergence

- **Forge shared-types:** `ApiNest:887 {id,name,description?,eggs?,eggCount?,createdAt}`, `ApiEgg:896 {id,nestId,name,startup, startupCommand?, dockerImages?: Record<string,string>, dockerImage?, config?: any, variables?: ApiStartupVariable[], installScript/Container/Entrypoint?, fileDenylist?, defaultMemoryMb?}`, `ApiStartupVariable:1262 {id,name,envVariable,defaultValue,serverValue,rules,isEditable}`, `ApiTemplate:1318 {id,name,eggId,nestId,dockerImage?,startupCommand?,environment?}`.
- **Ptero Transformers:** `NestTransformer` exposes `uuid` not `id`; `EggTransformer` exposes `docker_images` map + `relationships.variables`. Forge exposes both camelCase and snake_case aliases (`env_variable` alias `ApiStartupVariable:1271`) for compat.

### C18 — Runtime CreateRequest: mounts/env/ports handling

- **Wings:** `ServerConfiguration{Build: Limits, Allocations: {Mappings: ip→port[]}, Environment: Variables, Mounts[], Container: {Image}, Egg}` → `environment/docker.go` builds `HostConfig = {Memory, MemorySwap, CpuQuota/Period/Shares, BlkioWeight, CpusetCpus, Binds, PortBindings}` with `BlkioWeight` gated `settings.go:142`.
- **Forge API → Beacon:** `store_servers_control.go:82` `ServerProvisionTarget{Image, StartupCommand (resolved), Environment map, Mounts[], MemoryMB/SwapMB/CPUShares/CPULimit/DiskMB/IOWeight/Threads/OOMDisabled, Allocations[]}` → `Daemon CreateRequest{Image, Command[], Env: []string ("KEY=VALUE"), Ports: []Port{HostIP,HostPort,ContainerPort,Protocol}, Mounts: []Mount{Source,Target,ReadOnly}}` `daemon/client.go:401`. **Env is flattened to `KEY=VALUE` strings**; beacon `docker.go:1003` rebuilds `container.Config{Env, Cmd, Image, StopSignal, Labels{"modern-game-panel.server_id": id, hash}}`.
- **PufferPanel:** `run.environmentVars: map[string,string>` + `run.command: string|{command,if}[]` with expression language; Forge’s single `StartupCommand` string must be shell-tokenized externally — no array `command[]` support.

---

## 4. Logic Findings (≥3)

### LF-01 — [HIGH] Regex-rule parser incompatibility breaks imported Pterodactyl eggs — `validateVariableValue` mis-handles `regex:/pattern/`

**Files:**
- `packages/game-templates/templates/minecraft-paper.json:58` `rules: "required|regex:/^([\w\d._-]+)(\.jar)$/"`
- `forge/api/internal/store/store_egg_variables.go:142-188` `validateVariableValue`
- `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:66` `rules` (Laravel validation)
- `forge/api/migrations/013_startup_variables.sql:26` seed `required|string|max:64`

**Finding:** Pterodactyl stores `rules` as Laravel strings with optional `regex:/pattern/` delimiters (including `/` and optional flags). Forge `validateVariableValue` splits on `|`, then `strings.Cut(rule, ":")` to extract `name, arg`. For `regex:/^([\w\d._-]+)(\.jar)$/` this yields `name=regex`, `arg=/^([\w\d._-]+)(\.jar)$/` — including the surrounding `/`. It then does `regexp.Compile(arg)` `store_egg_variables.go:177`, which compiles a pattern that **literally expects leading and trailing slashes**, so `server.jar` fails validation (must be `/server.jar/`). Conversely `/`-less rules would behave differently. Valid eggs imported via `POST /eggs/import` `handlers_admin.go:1308` loop that calls `CreateEggVariable` will **reject all variables with regex rules**, blocking import of any stock Paper/Vanilla egg. The seed in `013_startup_variables.sql:26` uses `max:64` which is fine, but any regex import will error.

**Impact:** Cannot import Pterodactyl `PTDL_v2` eggs that contain regex-validated variables (most Minecraft eggs).

**Expected:** Strip leading `/` and trailing `/[flags]` before `regexp.Compile`, or reuse a Laravel-compatible validator. Also handle `regex:/.../` with pipes inside character classes (naïve `Split("|")` splits regex alternation).

**Repro:** `POST /eggs` with `rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` and `defaultValue: "server.jar"` → `400 value does not match the required pattern`.

---

### LF-02 — [HIGH] CPU limits conflation — `cpu` vs `cpu_shares` vs `CPUPercent` mismatch across store, daemon, beacon

**Files:**
- `forge/api/internal/store/store_servers.go:209` `if req.CPULimit < 0` + `CPUShares <=0` check
- `forge/api/internal/daemon/client.go:401` `CreateRequest{CPUPercent int64, CPUShares int64}`
- `beacon/internal/runtime/runtime.go:65` `CreateRequest{CPUPercent int64, CPUShares int64, IOWeight int64}`
- `beacon/internal/runtime/docker.go:924` `if req.CPUPercent < 0 || req.CPUPercent > 100000`
- `reference/game-hosting/pterodactyl-wings/environment/settings.go:37` `Limits{CpuLimit int64 (percent 0..Threads*100)}`
- `packages/shared-types/src/api.ts:98` `cpuLimit?: number, cpuShares?: number`

**Finding:** Three different CPU concepts are conflated:
1. Pterodactyl `cpu` is **percent** (0 = unlimited, else threads*100) → `CpuLimit` → `CPUQuota = CpuLimit*1000` `settings.go:125`.
2. Forge `ApiServer.cpuShares`/`CPUShares` is **weight** 2..262144, while `cpu`/`CPULimit`/`CPUPercent` is percent.
3. `store_servers.go:209` validates `CPULimit <0` but **does not cap at 100000** unlike beacon `docker.go:924`; `CreateRequest` carries **both** `CPUShares` and `CPUPercent` but `store_servers_control.go:96` only plumbs `CPUShares`/`CPULimit` → daemon `CreateRequest` mapping is undefined. `store_servers_control.go:120` loads `s.cpu_shares, s.cpu_limit` but daemon `client.go` `CreateRequest` has `CPUShares/CPUPercent` — caller must decide which `cpu` field maps. Current code in `forge/api/internal/services` (cluster manager) maps `store.CPULimit → CreateRequest.CPUPercent` and `store.CPUShares → CPUShares`, but **store validation allows `CPUShares=1024` default alongside `CPULimit=0` (unlimited) which would set Docker `CPUShares=1024` even when unlimited** — diverges from wings `settings.go:124` which only sets `CPUShares=1024` when `CpuLimit>0`.

**Impact:** Servers may run with unintended CPU weight or be rejected differently by API vs beacon.

**Expected:** Align validation (`CPULimit 0..threads*100`, `CPUShares 2..262144`), and make `AsContainerResources`-like logic conditional on `CPULimit>0` in beacon.

---

### LF-03 — [MEDIUM] `user_viewable` gate leaks/hides variables inconsistently — startup vs allocation substitution

**Files:**
- `forge/api/internal/store/store_startup.go:28` `WHERE … AND ev.user_viewable = true`
- `forge/api/internal/store/store_startup.go:58` `WHERE … AND ev.user_viewable = true` (update)
- `forge/api/internal/store/store_servers_control.go:154` `SELECT ev.env_variable … FROM egg_variables` **without** `user_viewable` filter
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/VariableValidatorService.php:31` `where('user_editable', true)->where('user_viewable', true)` (non-admin)
- `reference/game-hosting/pterodactyl-wings/server/server.go:162` loops `EnvVars` without viewable gate (runtime needs all)

**Finding:** Forge’s `GetServerStartup` correctly filters `user_viewable=true` for UI display `store_startup.go:31`, but `ServerProvisionTarget` query `store_servers_control.go:154` **does not filter** and injects **all** variables (including `user_viewable=false` like `DL_PATH` in `minecraft-paper.json:112`). This is intentional for runtime (all env needed), but `UpdateServerStartupVariable` `store_startup.go:58` rejects updates to `user_viewable=false` variables, while `store_servers.go:273` `CreateServer` **allows** setting any `StartupVariables` key that exists, regardless of `user_viewable/editable` — so a non-admin can set `DL_PATH` at creation (privilege escalation to override download URL) but cannot change it later. Pterodactyl’s `VariableValidatorService` would block non-admin entirely from such variables at creation time too.

**Impact:** Inconsistent authorization — internal variables writable at provision time, read-only thereafter; may allow sandbox escape via installer script URL injection if `copy_script_from` not validated.

**Expected:** Gate `CreateServer` startup variables by `user_editable && user_viewable` for non-admin principals, matching `VariableValidatorService`.

---

### LF-04 — [MEDIUM] Allocation `container_port` / `protocol` dropped in Wings-compat mappings — `Allocations.mappings ip→port[]` loses protocol

**Files:**
- `reference/game-hosting/pterodactyl-wings/environment/allocations.go:36` `Mappings map[string][]int` — **no per-port protocol**.
- `forge/api/internal/store/store_allocations.go:155` `containerPort` default, `protocol ∈ {tcp,udp}`
- `forge/api/internal/daemon/client.go:438` `Port{HostIP,HostPort,ContainerPort,Protocol}`
- `beacon/internal/runtime/runtime.go:100` `PortBinding{HostIP,HostPort,ContainerPort,Protocol}` + `docker.go:978` `dockerPorts`
- `reference/game-hosting/pterodactyl-wings/environment/allocations.go:38` `Bindings()` creates **both** `tcp` and `udp` bindings for every port.

**Finding:** Forge correctly models `protocol` per allocation and per `PortBinding`, but when Wings `Allocations.Mappings` is used (legacy `Configuration.Allocations`), Forge still must populate `Mappings map[string][]int` for backward compat (sent in `ServerConfiguration.Allocations` `daemon/client.go:460`). Forge’s `ServerProvisionTarget.Allocations []ServerRuntimeAllocation:725` does carry `Protocol`, but the **legacy** `Allocations map[string]any` (`daemon/client.go:460`) built from `ListServerAllocations` would drop protocol if derived from old path. Wings `Bindings()` then **binds every port as both tcp and udp** — so a UDP-only game port (e.g., `palworld.json:45` `8211 udp`) would incorrectly also bind `tcp`, exposing extra surface; conversely a tcp-only port would also get udp. Forge’s daemon/runtime path avoids this (explicit `protocol`), but any fallback to wings-era allocation serialization would be wrong.

**Impact:** Low today (Forge uses explicit ports), but import of old server configs or `server.build.default.port` substitution could mis-bind.

---

### LF-05 — [LOW] `resolveStartupCommand` undefined `{{server.build.default.port}}` vs `{{SERVER_PORT}}` drift

**Files:**
- `packages/game-templates/templates/minecraft-paper.json:22` `find: {"server-port": "{{server.build.default.port}}"}` (legacy panel placeholder)
- `reference/game-hosting/pterodactyl-wings/parser` `configMatchRegex {{config.*}}`
- `forge/api/internal/store/store_servers_control.go:178` `resolveStartupCommand`

**Finding:** Game-templates `config.files[].find` values contain `{{server.build.default.port}}` — a Pterodactyl **config-file** interpolation that Wings parser resolves against `configMatchRegex` at boot via `ConfigurationFileReplacement.LookupConfigurationValue`. Forge’s `resolveStartupCommand` only resolves `{{VAR}}` from `Environment` map; it does **not** resolve `{{server.build.default.port}}` (which should be the primary allocation port). At provision time, server.properties will be written with literal `{{server.build.default.port}}` if config-file patching is not implemented (which it isn’t — see C14). Even for startup commands, `palworld.json:14` uses `{{SERVER_PORT}}` which **is** resolved, but `config.files` entries rely on the other syntax.

**Impact:** Config-file parser not ported; game servers requiring `server.properties` patching (Minecraft) will start on wrong port.

---

## 5. Coverage Matrix — Resource & Template Parity

| Feature | Pterodactyl Panel | Pelican | PufferPanel | Forge (API+Beacon) | Gap |
|---------|-------------------|---------|-------------|--------------------|-----|
| `docker_images` map | ✓ `Egg.php:120` | ✓ + `tags` | `supportedEnvironments[].image` per env `spec.json:395` | ✓ `store_nests.go:361` normalize | — |
| `startup` templating | `{{VAR}}` + `SERVER_*` fixed | same | `run.command: string|{command,if}[]` | ✓ `resolveStartupCommand` but single string only | lacks array `if` |
| Variables CRUD | `EggVariable.php` + `VariableValidatorService` | same + `HasValidation` | `data: {var: {type,…}}` typed | ✓ `store_egg_variables.go:42` + `store_startup.go:54` | regex bug LF-01 |
| Variable `internal`/`user_viewable=false` | ✓ | ✓ | `internal: boolean` `spec.json:233` | ✓ (runtime leaks per LF-03) | auth gate missing |
| Resource limits full set | ✓ `Server.php:160-175` | ✓ | `requirements{os,arch,binaries}` only | ✓ `store_servers.go:209` + `runtime.go:55` | cpu conflation LF-02 |
| OOM / PID limit | `oom_disabled`, `threads` | same | `stats: jcmd`, `expectedExitCode` | ✓ `OOMKillDisabled`, `PIDLimit` `runtime.go:69-71` | — |
| Allocations pool | ✓ single IP/port | ✓ + `force_outgoing_ip` | `portBindings` inline | ✓ `store_allocations.go:125` with `protocol/containerPort` | — |
| Mounts | `egg_mount` pivot | + `mounts` relation | `environment.type=host → mounts: string[]` | ✓ `ServerMounts` → `Daemon Mounts[]` | readOnly honored, but `file_denylist` not enforced |
| Install pipeline | single script | same + `startup_commands` | 26 typed ops + `if` | ✗ single `script` blob `template-schema.json:95` | no typed ops |
| Config file parser | `parser/` 6 parsers + `configMatchRegex` | same | `alterfile/file` ops | ✗ `config` JSON stored but not applied `store_servers_control.go:95` | LF-05 |
| Nest/Egg import/export | `PTDL_v2` `Egg.php:68` `handlers_admin.go:1288/1308` | same | `spec.json` validated, `template.json` per game | ✓ `GET /eggs/:id/export` raw, `POST /eggs/import` `handlers_admin.go:1308` but not PTDL_v2 | not compatible with panel exports |
| Web admin | React | React | N/A (file) | ✓ `AdminNestsEggs` + `EGG_TEMPLATES` static `web/lib/egg-templates.ts:27` + `app-templates` localStorage | two template systems C11/C12 |
| Tests | `VariableValidatorServiceTest` `BuildModificationServiceTest` | same | `spec.json` schema validate | `store_eggs_integration_test.go` exists, but no variable validator test covers regex case | — |

---

## 6. Summary & Recommendations

**Overall:** Forge’s egg/nest/variable core is a faithful port of Pterodactyl Panel with Go+Postgres, adding stricter UUID handling, bulk allocation Tx, and `game-templates` static catalog. The CLI/package templates are a convenience layer on top of the DB eggs, and the `server_templates` → `eggs` unification `043_unify_eggs_templates_mounts.sql:43` is clean. Allocations and resource limits are richer than reference (per-protocol `containerPort`). Beacon’s `CreateRequest` is more explicit than Wings’ `Environment.Limits/Allocations` pair.

**Critical gaps / logic bugs requiring fix:**

1. **Fix `validateVariableValue` regex parsing** (`store_egg_variables.go:177`) — strip `/…/` delimiters and handle `|` inside regex; otherwise no Pterodactyl egg with regex variables can be imported.
2. **Align CPU limit semantics** (`store_servers.go:209` + `beacon/internal/runtime/docker.go:924` + `daemon/client.go:401`) — decide `cpu` = percent vs shares, gate `CPUShares` on `CPULimit>0` like `settings.go:124`.
3. **Gate internal variables on creation** (`store_servers.go:273`) by `user_editable/viewable` + add `RESERVED_ENV_NAMES` check (`EggVariable.php:43`) to prevent `SERVER_*` override.
4. **Implement or explicitly drop config-file parser** — `config.files` JSON currently inert; either wire `wings/parser` logic into beacon pre-start or document as unsupported and remove `config.files` from templates.
5. **Unify template sources** — `packages/game-templates` FS catalog and DB eggs diverge; add `seed` job `store/seeder.go` that upserts eggs from `templates/*.json` on startup, or deprecate `egg-templates.ts` static mirror.
6. **Allocation protocol fidelity** — ensure legacy `Allocations.Mappings` compat path encodes `protocol` or is removed; document that Wings-era dual tcp/udp binding is not reproduced.

**What Forge does better than reference:**
- `normalizeDockerImages` handles both `map` and `[]string` inputs (`store_nests.go:361`) — more robust than panel’s map-only after 2022 migration.
- Atomic bulk allocation create/delete (`store_allocations.go:134` `169`) with `23505` handling and audit per-row — Pterodactyl creates allocations one-by-one via service layer.
- `container_port` + `protocol` per allocation (`store_allocations.go:155`) — Pterodactyl allocations lacked `container_port` and assumed `tcp`+`udp` (`allocations.go:54`).

---
*Audited files (selected):* `reference/game-hosting/pterodactyl-panel/app/Models/Egg.php:84` `EggVariable.php:43` `app/Services/Servers/VariableValidatorService.php:28` `app/Models/Server.php:153` `database/migrations/2016_01_23_195641_add_allocations_table.php:13` `reference/game-hosting/pterodactyl-wings/server/server.go:151` `environment/settings.go:37` `environment/allocations.go:36` `parser/parser.go:1` `reference/game-hosting/pufferpanel-templates/spec.json:1` `minecraft/minecraft.json:2` `valheim/valheim.json:1` `packages/game-templates/template-schema.json:1` `src/types.ts:42` `templates/minecraft-paper.json:10` `packages/shared-types/src/api.ts:887` `forge/api/internal/store/store_nests.go:16` `store_egg_variables.go:17` `store_startup.go:11` `store_templates.go:12` `store_allocations.go:15` `store_servers.go:180` `store_servers_control.go:82` `forge/api/internal/daemon/client.go:401` `beacon/internal/runtime/runtime.go:55` `beacon/internal/runtime/docker.go:901` `forge/api/internal/http/handlers_admin.go:990` `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1` `forge/web/lib/egg-templates.ts:1` `forge/api/migrations/007_postgres_core_foundation.sql:87` `013_startup_variables.sql:1` `043_unify_eggs_templates_mounts.sql:1`
