# Subagent 06 — PufferPanel Templates vs Forge Game Templates vs 1Panel Catalog

> Scope: `reference/game-hosting/pufferpanel-templates` vs Forge `packages/game-templates` / `forge/web/lib/egg-templates.ts` / `forge/api/internal/store` (nests/eggs/variables) / `forge/api/internal/services/{catalog,appstore}` / `forge/web/app/admin/{nests,app-templates}` and 1Panel `compose_template.go`.
> All citations `file:line`. Read-only audit.

---

## 1. Inventory — What Exists Where

### 1.1 PufferPanel upstream

- `reference/game-hosting/pufferpanel-templates/spec.json:1` — JSON Schema draft `2020-12`, `$id: https://raw.githubusercontent.com/pufferpanel/templates/v3/spec.json`, title `PufferPanel Template`. Single source of truth for all game templates.
- `reference/game-hosting/pufferpanel-templates/*.json:1` — 44 JSON files on disk (36 game dirs + `.vscode/settings.json` + `spec.json` + 6 `data.json`). Unique game roots: `7days2die, ark, arma3, csgo, css, cstrike, discord-*, dontstarvetogether, eco, eco9, factorio, gmod, minecraft, minecraft-bedrock/bungeecord/curseforge/ftb/velocity/waterfall, pocketmine, rust, satisfactory, squad, starbound, stn, teamspeak3, terraria-*, tf2, unturned, valheim, vintage-story, zomboid`. E.g. `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:1`, `reference/game-hosting/pufferpanel-templates/valheim/valheim.json:1`, `reference/game-hosting/pufferpanel-templates/rust/rust.json:1`, `reference/game-hosting/pufferpanel-templates/7days2die/7days2die.json:1`, `reference/game-hosting/pufferpanel-templates/csgo/csgo.json:1`.
- Curated deploy catalogue in `reference/game-hosting/pufferpanel-templates/minecraft/data.json:1`, `.../rust/data.json:1`, `.../ark/data.json:1`, etc. — each `data.json` is an array of `{ name, variables, environment }` seed variants (e.g. `minecraft/data.json:2` vanilla host, `minecraft/data.json:48` `minecraft-paper-docker`, `rust/data.json:1` host vs docker). These *are* the one-click catalogue, separate from the template definition itself.

### 1.2 Forge — three overlapping catalogues

| Catalogue | Source | Count at audit | Store / seed |
|---|---|---|---|
| **Game Templates (npm package)** | `.freebuff/worktrees/*/packages/game-templates/templates/*.json` (14 files); `template-schema.json:1`, `src/types.ts:1`, `src/index.ts:1`, `scripts/validate-templates.mjs:1` | 14 (`7days2die.json`, `csgo.json`, `enshrouded.json`, `factorio.json`, `minecraft-bedrock.json`, `minecraft-paper.json`, `minecraft-vanilla.json`, `palworld.json`, `rust.json`, `satisfactory.json`, `teamspeak3.json`, `terraria.json`, `valheim.json`, `zomboid.json`) | No DB seed in `main` — package is `private:true` (`package.json:2`), consumed via import. Notably `packages/game-templates` **does not exist on `main` HEAD** (`ls /Users/riyaz/project/gamepanel/packages` shows only `sdk, shared-types`) — the 14-template package only exists on worktree branches (`.freebuff/worktrees/thmsm5f5s1nx6b/packages/game-templates`, `.../1078c899-...`). Fresh `main` checkout ships **zero** game-templates files. |
| **Egg Templates (hardcoded TS)** | `forge/web/lib/egg-templates.ts:1` `EGG_TEMPLATES: EggTemplateItem[]` | 14 entries (`minecraft-paper:15`, `minecraft-vanilla:84`, `palworld:130`, `valheim:164`, `terraria:194`, `enshrouded:234`, `satisfactory:267`, `rust:298`, `csgo:336`, `factorio:374`, `7days2die:400`, `minecraft-bedrock:426`, `teamspeak3:462`, `zomboid:493`) | Rendered into nest/egg creation wizard; IDs intentionally mirror `packages/game-templates` IDs (proven by `rust` `zomboid` etc. sharing ids). No DB seeding; pure frontend constant. |
| **Nests/Eggs (DB canonical)** | `forge/api/internal/store/store_nests.go:1` (`Nest`, `Egg`, `CreateNestRequest`, `CreateEggRequest`), `forge/api/internal/store/store_egg_variables.go:1` (`EggVariable`), `forge/api/internal/store/store_templates.go:1` (compat shim `Template ↔ Egg`), migration `forge/api/migrations/043_unify_eggs_templates_mounts.sql:1`, `091_seed_minecraft_java.sql:1` | Seeded to 1 egg on fresh install (see below) + runtime CRUD | `nests` (migration `007_postgres_core_foundation.sql:106` inserts `Games` nest `dddd...`), `eggs` (1 row from `091_*`), `egg_variables` (2 rows `VERSION/TYPE`) |
| **Catalog (managed services)** | `forge/api/internal/services/catalog/catalog.go:1`, `forge/api/internal/store/store_catalog.go:1` | DB-driven `catalog_entries` / `catalog_instances` for DBs/caches/queues | `db_containers` or `compose` backed, with `catalog_instances` status `provisioning/running/error` |
| **App Store (compose apps)** | `forge/api/internal/services/appstore/service.go:1` + `seed.go:10` (`seedApps: 7 entries`), `forge/api/internal/store/store_app_store.go:1` (`AppStoreApp`, `AppStoreInstall`) | 7 seeded compose apps (`nginx:28`, `postgres:56`, `redis:84`, `mongo:110`, `mariadb:138`, `portainer:168`, `traefik:194`) | `SeedDefaultApps` upserts on boot (`forge/api/cmd/api/main.go:529`) |
| **App Templates (localStorage UI)** | `forge/web/lib/app-templates-data.ts:1`, `forge/web/app/admin/app-templates/page.tsx:1` | 5 defaults (`nginx:7`, `node:17`, `python:26`, `postgres-compose:35`, `redis:48`) + user templates in `localStorage` key `forge.app-templates.v1` | Entirely client-side, never touches DB; `fetchAppTemplates()` (`forge/web/lib/api/apps.ts:369`) falls back to `getAllTemplates()` on network error |

**Gap:** Puffer ships a single coherent model (template + `data.json` variants). Forge fractures it into 4 disjoint layers with overlapping but disconnected seeding and no single source of truth for the 14 game definitions.

---

## 2. Detailed Comparisons (16)

### C1 — Template count & coverage

- **Puffer:** 36 distinct server types in 44 JSON payloads (42 deployable templates after deduplicating `data.json` + `spec.json`). Covers SteamCMD (`rust appId 258550`, `valheim 896660`, `7days2die 294420`, `csgo 740`), Mojang (`mojangdl`), Paper, Forge, Fabric, NeoForge, CurseForge, Sponge, Teamspeak, Discord bots, Vintage Story, Zomboid, etc.
- **Forge:** 14 templates across both `packages/game-templates` and `egg-templates.ts` — ~39% of Puffer's catalogue. Missing Puffer games with no Forge equivalent: `ark`, `arma3`, `css`, `cstrike`, `discord-jda/js/py`, `dontstarvetogether`, `eco/eco9`, `gmod`, `minecraft-bungeecord/curseforge/ftb/velocity/waterfall`, `pocketmine`, `squad`, `starbound`, `stn`, `terraria-tmodloader/tshock/vanilla` (Forge lumps Terraria into 1 template), `tf2`, `unturned`, `vintage-story`. Forge adds two games absent from upstream at these paths: `palworld` (AR-K-derived), `enshrouded`.
- **1Panel:** No game concept at all; `reference/app-platforms/1panel/agent/app/model/compose_template.go:3` `ComposeTemplate { Name, Description, Content }` is a generic compose fragment, not a game definition.

### C2 — Schema draft & extensibility gate

- **Puffer `spec.json:4` `additionalProperties: false` at root** and again on `variable:6`, `option:6`, `dockerEnv:6`, `standardEnv:6` etc. — strict closed schema. Unknown top-level keys rejected.
- **Forge `template-schema.json:3` draft-07** with `required: [id, name, description, version, game, image, startup, config, ports, env, resources, install_script, supported_platforms, categories]` (`template-schema.json:6`). Uses `additionalProperties` only via `images` map (`template-schema.json:31`) and `config.files` (`template-schema.json:40` `type: object` open). Puffer is more closed; Forge is semi-closed but allows extension via `features`/`file_denylist`.
- **1Panel `dto/compose_template.go:5` `ComposeTemplateCreate { Name validate:"required", Description, Content }`** — only one required field, no ports/env/resources/startup; `service/compose_template.go:46` `Batch` is upsert on `Name` only.

### C3 — Variable / environment model — the deepest divergence

- **Puffer `spec.json:$defs.variable:156`** — `type: enum[option,string,boolean,integer]` (`spec.json:165`), `value: number|string|boolean` (`spec.json:171`), `display, desc, required, userEdit, internal, options[]` (`spec.json:183`); `option` (`spec.json:237` `value + display`). Validation is type-aware: `spec.json:208` conditional `if type==option then required options`; `spec.json:226` `if type==integer then value:number`. Variables are a **map** (`spec.json:20` `data: patternProperties ^[0-9A-Za-z_]+$ → variable`). Groups (`spec.json:156` `groups: minItems:1, variables:string[]`) provide UI ordering/conditional display (`spec.json:182` `if: string`), e.g. `minecraft/minecraft.json:74` groups `General/Forge/NeoForge/Paper` with `if: modlauncher == 'forge'`. Internal vars like `resolvedForgeVersion` (`minecraft/minecraft.json:118` `internal:true`) and `DL_PATH` equivalents are first-class.
- **Forge `template-schema.json:69` `env: array< {name, env_variable, default_value, user_viewable, user_editable, rules} >`** + `src/types.ts:10` `GameTemplateVariable` — isomorphic to **Pterodactyl's `egg_variables`** (`store_egg_variables.go:10` `EggVariable {Name, EnvVariable, DefaultValue, UserViewable, UserEditable, Rules, Sort}` with `eggVariableNamePattern:8 ^[A-Z][A-Z0-9_]*$`). All values are **strings** (`default_value: string`), typing expressed via `rules` (`required|string|max:20`, `required|regex:/^...\\.jar$/`, `required|integer|min:1|max:100` — `validateVariableValue:92` in `store_egg_variables.go:92`). No `option` type, no `internal`, no `groups`, no `display/desc` split (single `description`). So Puffer's rich typed UI (integer spinner, option dropdown, boolean toggle, `internal` hiding) collapses to string+regex validation in Forge. Example: Puffer `minecraft/minecraft.json:26` `modlauncher { options: [{value:"",display:"None/Vanilla"}, {value:"fabric",display:"Fabric"}, ...] }` has no Forge equivalent; Forge splits that into two separate templates (`minecraft-vanilla.json` vs `minecraft-paper.json`).
- **1Panel:** No env model at all on the template; params live on `AppStoreApp` compose templating (`appstore/seed.go:28` `Params: JSON string` with `NGINX_PORT` etc.), expanded via `compose.ExpandTemplate` (`appstore/service.go:220`).

**Citation triad:**
- Puffer variable typing: `reference/game-hosting/pufferpanel-templates/spec.json:156-236`
- Puffer example with options/internal/groups: `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:14-170`
- Forge env shape: `.freebuff/.../packages/game-templates/template-schema.json:69-93`, `.freebuff/.../src/types.ts:10-18`, `forge/api/internal/store/store_egg_variables.go:8-48`

### C4 — Install / provisioning fidelity (largest logic risk)

- **Puffer `spec.json:$defs.operation:274`** — **24 declarative operation types** with typed args and per-step `if: string` conditionals: `alterfile, archive, command, console, curseforge, dockerpull, download, extract, fabricdl, forgedl, javadl, mkdir, mojangdl, move, neoforgedl, nodejsdl, paperdl, resolveforgeversion, resolveneoforgeversion, sleep, spongedl, stdin, steamgamedl, writefile` (`spec.json:282-300`). Each has required fields (e.g. `steamgamedl → appId`, `paperdl → build+minecraftVersion+target`, `mojangdl → version+target`). Steps can be conditional (`minecraft/minecraft.json:173` `if: javaversion != '' && env == 'host' → javadl`, `minecraft/minecraft.json:184` `if: modlauncher == 'paper' → paperdl`, etc.) and combine domain-aware helpers (version resolution, auth).
- **Forge `template-schema.json:114` `install_script: {container, entrypoint, script: string}`** (`src/types.ts:18-22`) — **single shell script string**. All 24 Puffer ops are manually transliterated to bash: `rust.json:127` `script: "#!/bin/bash\ncurl ... steamcmd... +app_update 258550 +quit"`, `minecraft-paper.json:114` inlines Papermc API curl+jq with fallback logic. No static validation of Steam appIds, no `if` branching (script must implement with `[ -n "$VAR" ]`), no typed arg checks. The declarative→imperative translation is lossy; every Puffer `paperdl/fabricdl/forgedl/mojangdl/curseforge/javadl/steamgamedl` is re-implemented as ad-hoc shell.
- **Forge eggs DB `store_nests.go:20` `Egg {InstallScript, InstallContainer, InstallEntrypoint, Startup, Config jsonb}`** — same single-script model at the DB layer; `store_nests.go:232` `CreateEgg` normalizes but never validates against the 24-op catalogue.
- **1Panel** — no install concept at all for `ComposeTemplate`; `appstore` compose installs go via `compose.Service.DeployComposeStack` (`appstore/service.go:116` `DeployComposeStack`) — similar single-compose blob.

### C5 — Environment / runtime target handling

- **Puffer:** Dual fields (`spec.json:26` `environment: {type: enum[docker, host]}` + `supportedEnvironments: array<environment> minItems:1`). Each env entry carries its own `image` + `portBindings: array<string>` (`spec.json:$defs.dockerEnv:350` `image required, portBindings[]`), or host `disableUnshare/mounts` (`spec.json:$defs.standardEnv:384`). **Per-template branching** uses `if: env == 'host'` vs `env == 'docker'` (`minecraft/minecraft.json:169`, `rust/rust.json:39-49`), plus `if: file_exists(...)`, `if: modlauncher == 'forge' && file_exists(...)`, `if: os != 'windows'` (`7days2die/7days2die.json:24-30`). `portBindings: ["0.0.0.0:${port}:${port}/tcp"]` (`valheim/valheim.json:77`) maps the variable port to container. Daemon chooses the matching environment; variables like `javaversion` only apply when `env == 'host'` (`minecraft/minecraft.json:173` `javadl if env=='host'`).
- **Forge game-template:** Two image fields: `image: string` (primary Docker image) + `images: Record<string,string>` (`template-schema.json:24-31`, `minecraft-vanilla.json:12` `images: { "Java 21": ..., "Java 17": ... }`), plus `supported_platforms: ["docker","podman"]` (`template-schema.json:131`). No `environment` / `supportedEnvironments` distinction; host mode does not exist — all templates are Docker-only. No `portBindings` — ports are declared as `ports: [{port, protocol, public}]` (`template-schema.json:47`). Forge egg-templates `forge/web/lib/egg-templates.ts:18` similarly expose `image + images` but no env switching. The Puffer host-vs-Docker split is dropped entirely (Forge lines like `if env=='host' && file_exists('fabric-server-launch.jar')` in Puffer `run.command[]:6` have no Forge analogue).
- **Forge catalog/appstore compose** — `store_catalog.go:84` `CatalogEntry` / `compose_template.go:3` don't model env types either.

### C6 — Run / startup command model

- **Puffer `spec.json:64` `run: { command: string|array<{command,if}>, pre:[], post:[], stop|stopCode (oneOf required), stdin, stdout, environmentVars, autostart, autorecover, autorestart, expectedExitCode }`** (`spec.json:64-153`). `run.command` is either a single string (Rust `rust/rust.json:51` `"./RustDedicated ... +rcon.password ${RConPassword}"`) or an **ordered fallback list with conditions** (Minecraft `minecraft/minecraft.json:244` 9 entries: fabric host/docker, server.jar host/docker, Forge new/legacy, NeoForge, final fallback). `environmentVars: {LD_LIBRARY_PATH}` (`rust/rust.json:56`), `stdin: {type: rconws, port:${RConPort}, password:${RConPassword}}` (`rust/rust.json:59`), `stop: "quit"` vs `stopCode: 2` (Valheim `valheim/valheim.json:65` `stopCode:2`, Rust `stop:"quit"`). `pre: [resolveforgeversion, resolveneoforgeversion]` (`minecraft/minecraft.json:306`).
- **Forge `template-schema.json:34` `startup: string` + `config: {files, startup:{done}, stop, logs}`** (`src/types.ts:14-30` `GameTemplateConfig`, `template-schema.json:40` `config: required [startup, stop, logs]`). No `pre/post`, no `stdin`/`stdout` config, no `stopCode`, no `environmentVars`, no `autostart`/`autorecover`/`autorestart`/`expectedExitCode`. All runtime semantics collapse to: `startup` string with `{{VAR}}` substitution (e.g. `rust.json:10` `"./RustDedicated -batchmode +server.port {{SERVER_PORT}} ... +rcon.password {{RCON_PASSWORD}}"`, `valheim.json:10` `"./valheim_server.x86_64 -name \"{{SERVER_NAME}}\" -port {{SERVER_PORT}} ..."`) + `config.stop` + `config.startup.done` regex. Puffer's RCON/telnet/file stdout routing and `LD_LIBRARY_PATH` injection are absent at schema level.

### C7 — Port & network exposure

- **Puffer:** Implicit via variable `${port}` + `supportedEnvironments[].portBindings: ["0.0.0.0:${port}:${port}/tcp"]` (`valheim/valheim.json:77`, `csgo/csgo.json:115-119` three bindings TCP for `port/clientport/tvport`). Puffer also exposes multiple logical variables per service (`rust/rust.json:28` `port:27015` + `RConPort:28016`, `csgo/csgo.json:14` `port:27015 + clientport:27016 + tvport:27017`, `valheim 2456` range).
- **Forge:** Explicit `ports: [{port, protocol: tcp|udp, public, description}]` (`template-schema.json:47-59`, `validate-templates.mjs:56` checks `port 1..65535` + `protocol in tcp/udp`). E.g. `valheim.json:21` expands Valheim's single Puffer port into 3 UDP ports `2456/2457/2458`; `rust.json:20` splits game+RCON into `28015/udp + 28016/tcp`; `factorio/templates/7days2die` use single `34197/udp`, `26900/udp`. No `portBindings` concept — mapping is assumed external (Pterodactyl `allocation → SERVER_PORT` injection).
- **1Panel:** No port model on `ComposeTemplate`; ports are freeform inside `Content` YAML.

### C8 — Resource contracts

- **Puffer `spec.json:26` `requirements: {os, arch, binaries[]}`** — e.g. `valheim/valheim.json:82` `{os:linux, arch:amd64, binaries:[unzip]}`, `rust/rust.json:65` `{os:linux, arch:amd64}`. No CPU/memory/disk sizing at template level (sizing is server-level / `data.json` `port` only).
- **Forge `template-schema.json:95` `resources: {cpu, memory_mb, memory_max_mb, disk_mb, cpu_shares, io_weight, swap_mb}`** required `cpu,memory_mb,disk_mb` — explicit quota per template (e.g. `rust.json:84` `cpu:200,memory_mb:4096,disk_mb:20480`, `minecraft-vanilla.json:78` `cpu:100,memory_mb:1024,disk:5120`, `valheim: cpu200/mem4096`). Puffer's `requirements` has no Forge counterpart; Forge's `resources` has no Puffer counterpart. They are orthogonal.
- **1Panel** — no resource fields on `ComposeTemplate`; app store uses `MinMemoryMB/MinDiskMB` (`store_app_store.go:12` `MinMemoryMB, MinDiskMB`).

### C9 — Validation pipeline

- **Puffer:** Runtime JSON-Schema validation against `spec.json:3` `$schema draft 2020-12` via `pufferpanel/pufferpanel` daemon engine (`spec.json:1` `$id .../v3/spec.json`). The file alone is the validator; no extra script at the templates repo (tests use `go` engine's `spec.json` directly). Strict: unknown fields rejected.
- **Forge game-templates:** Two-layer: **JSON Schema** (`template-schema.json:1` draft-07, `pattern: ^[a-z0-9]+(-[a-z0-9]+)*$` for `id`, `^\\d+\\.\\d+\\.\\d+$` for `version`, etc.) **plus** imperative script `scripts/validate-templates.mjs:1`: (a) `REQUIRED:8` list check (`validate-templates.mjs:12`), (b) registry membership (`index.json` `registeredIds`), (c) `env_variable ^[A-Z][A-Z0-9_]*$` (`validate-templates.mjs:35`), (d) **placeholder contract** — `collectPlaceholders:20` over `startup + config.files` and assert every `{{VAR}}` ∈ `BUILTIN_VARIABLES (32)` ∪ `env[]` (`validate-templates.mjs:46-54`), (e) ports range/protocol (`validate-templates.mjs:56-60`), (f) filename matches `id` (`validate-templates.mjs:65`). **Duplicated in `forge/web/lib/egg-templates.ts` there is no validator at all** — `EGG_TEMPLATES` array is unverified TS literals.
- **Forge eggs DB** (`store_nests.go:276` `normalizeDockerImages`, `normalizeJSONObject/JSONArray:303/320`) validates images/config at DB write time, but **has no placeholder validation** — `store_egg_variables.go:49` `validateEggVariableRequest` checks `EnvVariable` pattern + `validateVariableValue` against `rules` string (`required|min|max|in|regex|string|nullable`), but never checks that `Egg.Startup` placeholders are resolvable (that check only lives in `validate-templates.mjs`).
- **1Panel:** `dto/compose_template.go:5` `validate:"required"` on `Name` only; `service/compose_template.go:46` `Create` rejects duplicate name but never validates YAML.

### C10 — Template naming / identity

- **Puffer:** No top-level `id` convention at template-filename level; identity is **filesystem path** (`minecraft/minecraft.json` `type: minecraft-java`, `7days2die/7days2die.json` `type: srcds`). `type` and `display` are descriptive, not slug-enforced (`spec.json:12` `id?: string` optional). Deploy catalogue IDs come from `data.json` `name` fields (`minecraft/data.json:3` `minecraft-vanilla`, `minecraft-vanilla-docker`, etc. — not validated).
- **Forge:** Strict kebab `id: ^[a-z0-9]+(-[a-z0-9]+)*$` (`template-schema.json:14`), validated in `validate-templates.mjs:45` `file === id.json` and `registeredIds.has(id)`. All 14 templates' filenames exactly match `id`. **Egg-templates.ts** mirrors this (`egg-templates.ts:16` `id:"minecraft-paper"` etc.) but via TS string literals, not validated against the schema.
- **Forge nests/eggs DB** — `store_nests.go:116` `Eggs.nest_id → nests.id` FK, `store_nests.go:48` `GetNest`/`ListNests` use UUID `id`. Eggs use UUID PK, not slug; `091_seed_minecraft_java.sql:6` hardcodes `91ec0000-0000-4000-8000-000000000001` for `Minecraft Java`.

### C11 — Seeding & bootstrap path

- **Puffer:** No DB seeding — templates are **files on disk** consumed at import time; `data.json` is the implicit catalogue. Tests download & validate via `Dockerfile-templatetester`.
- **Forge DB seeding (production):**
  - `migrations/007_postgres_core_foundation.sql:106` `INSERT nests ('Games') ON CONFLICT DO NOTHING`
  - `migrations/091_seed_minecraft_java.sql:3` inserts 1 `eggs` row `Minecraft Java` (image `itzg/minecraft-server:java21`, startup empty, `config: {stop:"stop", startup:{done:["Done ("]}}`) + 2 `egg_variables` rows `VERSION/TYPE` (`091:20,32`). This is the **only** production egg seed. So a fresh Forge install exposes 1 egg, not 14.
  - `forge/api/internal/services/appstore/seed.go:27` 7 `AppStoreApp` compose upserts, called on every boot `cmd/api/main.go:529` `SeedDefaultApps`
  - `DefaultSeeder` (`store/seeder.go:40`) registers only `default-roles` + `default-settings` — no game content
  - `migrations/043_unify_eggs_templates_mounts.sql:20` creates `Legacy Templates` nest and backfills legacy `server_templates → eggs` preserving UUIDs — migration-time only
  - **Missing:** None of the 14 game-templates / 14 egg-templates are seeded to `nests/eggs`/`egg_variables` at bootstrap. The bridge `src/index.ts:8` `templateToApiTemplate` (maps `GameTemplate → ApiTemplate` via env defaults) and `EGG_TEMPLATES` constant are **frontend-only** and never materialize as persistent `eggs`. The admin must manually import via `AdminNestsEggs: importEgg` (`AdminNestsEggs.tsx:124`) paste-JSON import.
- **1Panel:** `service/compose_template.go:58` `Batch` upserts compose fragments by `Name`; no game seed.

### C12 — Lifecycle / update signalling

- **Puffer:** `run.stat: {type: "jcmd"}` (`minecraft/minecraft.json:330` absent), `run.query: {type:"minecraft"}` for UDP query probe, `run.stdin` (`minecraft: stdin`, `rust: rconws`, `7days2die: telnet:8081` `7days2die.json:42`), `run.stdout` (`telnet/rcon/rconws/file/stdout`), `run.autostart/autorecover/autorestart/expectedExitCode`. Explicit server state machine.
- **Forge game-template:** `config: {stop, startup:{done}, logs:{}}` only (`minecraft-vanilla.json:20` `stop:"stop", startup:{done:")! For help, type \""}`, `valheim: stop:"^C", done:"Game server connected"`). No `stdin/stdout` strategy, no `stats/query`, no `autostart`/`autorestart`. Equivalence is assumed by the underlying Pterodactyl `wings` `startup.done` regex. Forge `catalog` entries use `StatusProvisioning/Running/Error` (`catalog/catalog.go:11`) but at the instance level, not template.

### C13 — Frontend / admin surface

- **Puffer templates repo:** No frontend — static JSON for daemon consumption.
- **Forge `forge/web/app/admin/nests/page.tsx:1` + `forge/web/components/admin/AdminNestsEggs.tsx:1`** — hierarchical Nest→Eggs table with Docker images list, startup truncation, Variables link (`/admin/nests/[nestId]/eggs/[eggId]/variables`), clone/download/import/export modals, Docker image map normalization (`AdminNestsEggs.tsx:14` `dockerImageLines` handles both `Record<string,string>` and `string[]`). `AdminNestsEggs.tsx:69` `dockerImageLines` is actually the compatibility layer for `store_nests.go:276` `normalizeDockerImages` array→map migration.
- **Forge `forge/web/app/admin/app-templates/page.tsx:1`** — **entirely separate** UI for generic app templates (`AppTemplate {image|git|compose}` from `lib/api/apps.ts:36` `AppType`), backed by `localStorage` (`app-templates-data.ts:3` `STORAGE_KEY: forge.app-templates.v1`). `app-templates/page.tsx:56` `formToTemplate` encodes freeform port/env strings via `host:container` and `KEY=value` CSV parsing — no relation to game-template `env[]`/`ports[]` semantics. The two template systems do not share validation or storage.
- **Forge `forge/web/lib/api/apps.ts:369` `fetchAppTemplates()`** — tries `/admin/app-templates` then falls back to local; never hits `/admin/nests` or game-templates.
- **1Panel** — `api/v2/compose_template.go:1` `dto.ComposeTemplateCreate` CRUD + search paging; `service/compose_template.go:27` `List/SearchWithPage` — plain compose fragment list.

### C14 — Placeholder / condition syntax

- **Puffer:** `${var}` (`minecraft/minecraft.json:201` `"${version}"`, `"${javaversion}"`) — lower_snake derived from `data` keys (e.g. `version, javaversion, forgebuild, port, eula`). Conditions are **expression strings** (`minecraft/minecraft.json:174` `if: javaversion != '' && env == 'host'`, `minecraft/minecraft.json:219` `if: file_exists('forge-' + resolvedForgeVersion + '-shim.jar')`, `minecraft/minecraft.json:302` `if: file_exists('fabric-server-launch.jar')`, `7days2die: os != 'windows'`, `satisfactory: os == 'linux'` etc.) — evaluated by the daemon type engine. Condition language supports `==, !=, &&, file_exists(), env, os`.
- **Forge:** `{{VAR}}` (`minecraft-vanilla.json:16` `"java ... -jar {{SERVER_JARFILE}}"`, `rust.json:10` `"+server.port {{SERVER_PORT}}"`) plus the Pterodactyl allocation alias `{{server.build.default.port}}` (`minecraft-paper.json:22`). Builtins enumerated in `validate-templates.mjs:12` `BUILTIN_VARIABLES = {SERVER_PORT,SERVER_IP,SERVER_MEMORY,SERVER_UUID,P_SERVER_UUID,STARTUP, server.build.default.*}`. **No conditional placeholders** — conditionals exist only for Paper version fallback inside the shell script (`minecraft-paper.json:114` `if [ "${VER_EXISTS}" = "true" ] ...`), not declaratively.

### C15 — Catalog vs store vs template layering (1Panel contrast)

- **Puffer:** One layer — `templates/*.json` + `data.json` + `spec.json`. The daemon applies templates directly; no DB `catalog_entries` or `app_store_apps` table.
- **Forge:** Three layers that overlap but don't connect:
  1. `store_catalog.go:84` `CatalogEntry{Key, DisplayName, Category, Versions[], DefaultVersion, Requires[], Enabled, SortOrder}` + `CatalogInstance{EntryKey, Kind, Version, EnvironmentID, NodeID, RefType, Status, ConnString}` — general DB/cache/queue catalogue, version-gated (`catalog/catalog.go:68` `requireVersion`), provisioned via `DBProvider` or `ComposeProvider` (`catalog/catalog.go:95` `isManagedDBKind → provisionDB else provisionCompose`).
  2. `store_app_store.go:6` `AppStoreApp{Key, Name, Category, Tags[], Version, ComposeContent, Params JSON, MinMemoryMB, SourceURL}` + `AppStoreInstall{AppKey, ComposeContent, Params, Status, ComposeProjectID}` — compose apps, provisioned via `compose.Service.DeployComposeStack` (`appstore/service.go:116`), with remote-sync capability (`appstore/service.go:306` `SyncFromRemote` pulls `AppStoreApp[]` from URL).
  3. `store_nests.go:20` `Egg{NestID, DockerImages jsonb, Startup, Config jsonb, InstallScript, ...}` + `EggVariable` + `Template` compat shim (`store_templates.go:13` `ListTemplates` delegates to `ListEggs`). Game-templates live here if materialized.
- **1Panel** (`reference/app-platforms/1panel/agent/app/model/compose_template.go:3` `ComposeTemplate{Name, Description, Content}`) is equivalent only to Forge's **layer 2** `AppStoreApp` (compose blob) minus versioning/tags/resources. Compared to Puffer it lacks all four higher-level axes (data/variable schema, env, install ops, run model).

### C16 — Discoverability & ingestion

- **Puffer:** Files are directly testable (`Dockerfile-templatetester` spins each template's `supportedEnvironments` both `host` and `docker`); CI enumerates all 36.
- **Forge:** `packages/game-templates` has `prebuild: node scripts/validate-templates.mjs` (`package.json:9`) but `main` lacks the package entirely. Consumption paths:
  - Build-time validation via `validate-templates.mjs` checks placeholder contract only for the integer in `packages/game-templates`.
  - Runtime eggs via DB `eggs` table — single `091_*` egg.
  - Frontend `EGG_TEMPLATES` static array (`forge/web/lib/egg-templates.ts:1`) — 14 entries but no validator and no automatic DB sync.
  - `AppStoreApp.SyncFromRemote` (`appstore/service.go:306`) can pull a remote registry as `AppStoreApp[]` but game-templates have no such sync.
  - Manual admin paste import (`AdminNestsEggs.tsx:124` `importEgg` parses JSON, validates only that `dockerImages.length>0`, then `createEgg`).
- **1Panel:** `compose_template.go: Batch` import takes `ComposeTemplateBatch{Templates []ComposeTemplateCreate}` (`dto/compose_template.go:8`) and upserts both new and existing by name (`service/compose_template.go:58`).

---

## 3. Logic Findings (4 — exceeds ≥3 minimum)

### L1 — Conditional-install expressiveness gap (HIGH)

**Location:** `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:173-241` (16 conditional install steps with domain-aware ops) vs Forge `*-templates/templates/minecraft-paper.json:114, minecraft-vanilla.json:114, valheim.json:*, rust.json:127` (single shell blob).

**Finding:** Puffer's declarative conditionals encode critical cross-product safety that Forge's imperative shell transliteration silently drops:

- **Puffer guards `javadl` to host only** (`minecraft/minecraft.json:173` `if: javaversion != '' && env == 'host'`) — on Docker, `javadl` must not run because the image already carries `eclipse-temurin:${javaversion}` (`minecraft/minecraft.json:322` `supportedEnvironments: {type:docker, image: eclipse-temurin:${javaversion}}`). Forge's `minecraft-vanilla.json:114` script runs `curl ... mojang manifest | jq ...` unconditionally in `ghcr.io/pterodactyl/installers:alpine` — harmless for Minecraft, but the pattern has no `env=='host'` gate to disable flows that only make sense on bare-metal. For templates ported naively, the absence of environment-gated steps can cause double-download or missing-JDK failures.
- **Puffer file-exists fallback on Forge/NeoForge output naming** (`minecraft/minecraft.json:219` `if file_exists('forge-' + resolvedForgeVersion + '-shim.jar') → move shim.jar`; next step `if file_exists('forge-...jar') → move`). This handles the historical Forge shim vs non-shim jar naming divergence. Forge `minecraft-paper.json:114` and equivalent scripts hardcode the Paper curl URL with no shim handling — if upstream changes naming, Forge breaks where Puffer degrades gracefully.
- **Puffer `mojangdl` + `fabricdl` + `paperdl` + `neoforgedl` + `steamgamedl` operate with typed arg validation** (`spec.json:416,439,466,483,588` required `appId, version, minecraftVersion+build+target` etc.). Forge scripts embed `"258550"`, `"896660"`, `"294420"` etc. as magic literals with no schema guard — a typo like `"25855"` would pass `template-schema.json` (which has no `appId` field) and only fail at container runtime.
- **Puffer `resolveforgeversion` / `resolveneoforgeversion` outputVariable (`spec.json:531`)** → `run.pre` resolves the canonical version before startup (`minecraft/minecraft.json:306`). Forge has no `pre` hook — any version resolution must be duplicated inside `install_script` and `startup` independently, risking drift (startup uses `{{MINECRAFT_VERSION}}` but the jar on disk may reflect a different resolved value if install and start are not synchronized).

**Impact:** Game templates that appear to port correctly can silently fail on edge cases (Forge shim rename, Java version matrix, container vs host env) that Puffer's typed model handles. The relevant Forge code never observes these failures until the user's install container exits non-zero.

---

### L2 — Startup variable resolution incompleteness (MEDIUM)

**Location:** `.freebuff/.../scripts/validate-templates.mjs:32-54` (`BUILTIN_VARIABLES` + `collectPlaceholders` over `startup + config.files`) + `forge/api/internal/store/store_egg_variables.go:92` `validateVariableValue` + `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:244-304` (9-entry `run.command` fallback) + `forge/web/lib/egg-templates.ts:38` `config.files` etc.

**Findings:**

1. **Validator scope is narrow.** `validate-templates.mjs:49` only validates `{{VAR}}` in `startup` and `config.files`. Variables consumed inside `install_script` (the most dangerous place — `DL_PATH` expansion `sed -e 's/{{/${/g' -e 's/}}/}/g'` in `minecraft-vanilla.json:114, minecraft-paper.json:114`) are **not checked**. A template could reference `{{UNDEFINED_IN_ENV}}` inside `install_script` and pass validation but produce `curl ... -L ""` at deploy time. Puffer's variables are expanded in **all** `install` and `run` fields by the daemon type engine, so its `spec.json` validation is inherently broader.

2. **Builtin list is incomplete vs Pterodactyl allocation semantics.** `validate-templates.mjs:12` declares 9 builtins but omits aliases observed in Forge's own `egg-templates.ts` `config.files` such as `{{server.build.default.ip}}` vs `{{server.build.default.ip_alias}}` (both listed, correct) but guidance (`packages/game-templates/README.md:32`) documents `{{SERVER_IP}}` vs `{{SERVER_PORT}}` while Puffer's `7days2die`, `satisfactory` etc. originally used `${ip}`/`${port}` style — migration to `{{SERVER_PORT}}` happened in Forge but `validate-templates.mjs` hardcodes `SERVER_PORT` and would reject a legacy `{{PORT}}` (correctly warns `README.md:56` "Do not use {{PORT}}"), yet never guards that the daemon actually provides `SERVER_PORT` vs `server.build.default.port` consistently across all Deploy paths. `store_templates.go:48` `templateFromEgg` and the wings mapping are unvalidated.

3. **Port vs startup drift on multi-port services.** Forge `rust.json:20` declares `28015/udp + 28016/tcp` (game + RCON) but `startup: "... +rcon.port {{RCON_PORT}}"` expects a variable-port while Puffer `rust/rust.json:51` binds `+rcon.port ${RConPort}` where `RConPort` itself is a variable defaulting to `28016` (data field, user-non-editable). Forge correctly models `RCON_PORT` as an `env` entry (`rust.json:52` `RCON_PORT: default 28016, user_viewable:false`), but `satisfactory.json`'s `BEACON_PORT` (`satisfactory.json` `env: BEACON_PORT:15000`) has **no matching `ports[]` entry** — Beacon port is exposed in startup but never allocated as a `ports` row, so a firewall/allocator that strictly uses `ports[]` will not open it. Puffer's Satisfactory model (`satisfactory/satisfactory.json:35` `run.command: -ServerQueryPort=${port}`) actually only declared a single port `15777` and omitted Beacon entirely — Forge widens startup without widening `ports[]`, introducing a mismatch.

---

### L3 — Seeding & materialization divergence — 14 templates exist nowhere after `migrate` (HIGH)

**Location:** `forge/api/migrations/091_seed_minecraft_java.sql:3` (one-egg seed) vs `.freebuff/.../packages/game-templates/templates/*.json` (14 on branch, 0 on main) vs `forge/web/lib/egg-templates.ts:1` (14 constants) vs `store_templates.go:13` `ListTemplates` + `store_nests.go:116` `ListEggs` vs `forge/web/lib/api/apps.ts:369` `fetchAppTemplates` + `AdminNestsEggs.tsx:124` `importEgg`.

**Finding:** Fresh `forge/api` migration completes with exactly **1 egg** (`Minecraft Java` variant of `itzg/minecraft-server:java21` with empty `startup` — `091_seed_minecraft_java.sql:12` `startup: ''`). The 14 richer Forge-authored templates (`minecraft-paper: PaperMC build resolution`, `rust: RCON, LD_LIBRARY_PATH`, `valheim: crossplay, savedir`, etc. identical in `packages/game-templates/templates/*.json` and `forge/web/lib/egg-templates.ts`) **are never seeded to the DB**. They exist as:
- branch-only files not present on `main` (`packages/game-templates` absent on `main` `ls packages → sdk, shared-types`),
- frontend constants (`EGG_TEMPLATES`) used only to populate a creation wizard, not persisted.

Consequences:
- `ListEggs(ctx, "")` (`store_nests.go:116`) on a fresh system returns 1 row; the Browse Templates button in `AdminNestsEggs.tsx:124` (`router.push("/admin/templates?nestId=...")`) navigates to a **different system** (app-templates `localStorage`, not eggs).
- `store_templates.go:13` `ListTemplates` (compat over eggs) likewise returns 1 template; code assuming parity with `packages/game-templates/index.json:1` `registry {minecraft-paper, minecraft-vanilla, palworld, valheim, terraria, enshrouded, satisfactory, rust, csgo, factorio, 7days2die, minecraft-bedrock, teamspeak3, zomboid}` (14) will see a 93% deficit.
- The admin's only path to the missing 13 eggs is manual JSON paste import (`AdminNestsEggs.tsx:124` `importEgg` — validates only `dockerImages.length>0`), with no batch import, no `seedApps`-style `UpsertEggTemplates` analogous to `appstore/seed.go:216` `SeedDefaultApps`, and no `compose_template.go:58` `Batch`-style upsert. This is a **data-loss on install** bug: the catalogue that is authored, validated, and documented does not ship.
- Worktree `validate-templates.mjs:12` `prebuild` runs only at `packages/game-templates` build time, but `main` cannot build that package (directory missing), so CI on `main` never validates game-templates at all.

**Impact:** Any claim that Forge ships 14 game servers out of the box is false on `main`; it ships 1 minimal Minecraft placeholder. The 14-template catalogue is phantom data gated behind non-`main` branches and manual admin action.

---

### L4 — Variable type & UI contract mismatch between game-templates and eggs (MEDIUM)

**Location:** `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:74-105` (groups + `option`/`boolean`/`integer` types) vs `forge/api/internal/store/store_egg_variables.go:10` (`EggVariable` all-string + `rules` DSL) vs `.freebuff/.../packages/game-templates/template-schema.json:69` (`env: default_value: string`) vs `forge/web/components/admin/AdminNestsEggs.tsx:284` (`EggVariable` editor) and `AdminNestsEggs.tsx: distributed pages for variables`.

**Finding:**
- Puffer's `data: {port: {type:integer, value:25565}, eula:{type:boolean, value:false}, modlauncher:{type:option, options:[{value:"", display:"None/Vanilla"}, ...]}}` gives the UI a typed widget (number input, toggle, dropdown). Forge `GameTemplate.env[].default_value: string` (`template-schema.json:79`) + `rules: "required|integer|min:1|max:64"` is **stringly-typed** — the frontend must infer the widget from the `rules` regex, which it does not (the admin eggs variable editor is freeform text). Example: `zomboid.json` `Admin Password: rules:"required|string|min:4|max:32"` vs Puffer `rcon password` style — Puffer can enforce length at the schema level (`spec.json:208` conditional type checks), Forge defers to `store_egg_variables.go:92` `validateVariableValue` which only checks `string` rules at write time, not before deploy.
- Puffer `internal:true` (`minecraft/minecraft.json:118` `resolvedForgeVersion internal:true`) and `userEdit:false` (`rust/rust.json:39` `port userEdit:false`) semantics are emulated in Forge as `user_viewable:false, user_editable:false` (`rust.json:52` `SERVER_IDENTITY user_viewable:false, user_editable:false` and `RCON_PASSWORD user_viewable:false, user_editable:true`). However Puffer's `internal` (never shown to user, never selectable) has no Forge analogue — a Forge template cannot mark `DL_PATH` as internal at the schema level; it must be hidden only via `user_viewable:false`. The distinction matters for audit: `internal`-vs-hidden have different threat boundaries.
- Puffer `groups` (`spec.json:155` `groups: minItems:1, variables:string[], if:string, order` ) provide ordered conditional sections (Minecraft's Paper vs Fabric groups). Forge has **no groups** — `store_egg_variables.go:15` `Sort` (`int`) plus `UserViewable/UserEditable` control order only by numeric sort, with no conditional display. After importing `minecraft/minecraft.json`, the Fabric-vs-Paper-vs-Forge group structure is flattened into one sorted list; the admin sees `forgebuild`, `neobuild`, `paperbuild` variables simultaneously with no context, inviting misconfiguration.

---

## 4. Side-by-Side Normative Summary

| Axis | PufferPanel | Forge game-templates / eggs | 1Panel ComposeTemplate | Verdict |
|---|---|---|---|---|
| **Schema** | `spec.json:4` closed `additionalProperties:false`, 24 ops | `template-schema.json:3` draft-07 + `validate-templates.mjs:46` placeholder contract | `dto/compose_template.go:5` `validate:"required"` on `Name` only | Puffer » Forge » 1Panel |
| **Variables** | Map `data: option|string|boolean|integer + options + internal + userEdit + groups + if` | List `env[]: EnvVariable/Rules string` (`template-schema.json:69`, `store_egg_variables.go:10`) | None on template | Puffer typed, Forge stringly |
| **Install** | 24 decl ops + `if` + `file_exists` | Single shell `install_script` string | None | Puffer > Forge |
| **Runtime** | `run: {command[]+if, pre/post, stop||stopCode, stdin:telnet/rcon/rconws/stdin/file, stdout, envVars, autostart}` | `startup: string + config:{stop, done, logs}` | `Content` YAML blob | Puffer » Forge » 1Panel |
| **Env** | `environment + supportedEnvironments[{host,docker image portBindings mounts}]` | `image + images + supported_platforms` | None | Puffer host+docker; Forge docker-only |
| **Ports** | `portBindings: ["0.0.0.0:${port}:..."]` + variable ports | `ports: [{port, protocol, public}]` typed | Inline in YAML | Forge more explicit |
| **Coverage** | 36 game types | 14 (phantom on main) | 0 games | Puffer >> Forge |
| **Seeding** | Files | 1 egg seeded (`091_*`); 7 app-store apps; 5 localStorage templates | `Batch` upsert | Fragmented |
| **Frontend** | None | Two disjoint UIs: `nests` (DB) + `app-templates` (localStorage) (`AdminNestsEggs.tsx:1`, `app-templates/page.tsx:1`) | Single CRUD | Split-brain |

---

## 5. Recommendations (informational, no code change)

1. **Close the seeding gap** — introduce an `appstore/seed.go:216`-style `EggSeed` that upserts the 14 game-templates from `packages/game-templates/templates/*.json` (or from `egg-templates.ts` as canonical) into `eggs + egg_variables` on migration or via `cmd/api/main.go:529`-style idempotent boot. Until then, document that `main` ships 1 egg, not 14.
2. **Collapse the dual template sources** — delete either `packages/game-templates` or `forge/web/lib/egg-templates.ts` as canonical; generate the other. Current duplication (already drifted: `zomboid.json` author strings differ, etc.) guarantees divergence.
3. **Expand `validate-templates.mjs:49` to cover `install_script` placeholders** and validate Steam `appId` integers and `update_url` reachability; add `ports[]` ↔ `startup` Beacon/RCON gap lint (Satisfactory/Teamspeak/Valheim range checks).
4. **Materialize `groups` or an equivalent `sort + visibility-if`** so Paper/Fabric/Forge variables are not co-visible. Even a simple `if: env == ...` field on `EggVariable` would restore Puffer's conditional-group intent without full operation-type semantics.
5. **Align `packages/game-templates` presence with `main`** — either move templates to a package reachable on `main` (`forge/packages/game-templates → packages/`) or remove the phantom `packages/` reference from docs so CI on `main` can actually run `validate-templates.mjs`.

---

## 6. File Map (key citations)

- Puffer spec: `reference/game-hosting/pufferpanel-templates/spec.json:1` (full 700-line schema)
- Puffer examples: `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:1` (Paper/Fabric/Forge/NeoForge), `.../rust/rust.json:1`, `.../valheim/valheim.json:1`, `.../7days2die/7days2die.json:1`, `.../csgo/csgo.json:1`, `.../satisfactory/satisfactory.json:1`, `.../minecraft/data.json:1`
- Forge game-templates (branch-only): `.freebuff/worktrees/thmsm5f5s1nx6b/packages/game-templates/template-schema.json:1`, `.../src/types.ts:1`, `.../src/index.ts:1`, `.../scripts/validate-templates.mjs:1`, `.../templates/minecraft-paper.json:1`, `.../templates/minecraft-vanilla.json:1`, `.../templates/rust.json:1`, `.../templates/valheim.json:1`, `.../templates/7days2die.json:1`, `.../templates/csgo.json:1`, `.../index.json:1`
- Forge eggs DB: `forge/api/internal/store/store_nests.go:1` (`CreateEgg:232`, `normalizeDockerImages:276`), `forge/api/internal/store/store_egg_variables.go:1` (`validateVariableValue:92`), `forge/api/internal/store/store_templates.go:1` (`templateFromEgg:48`), `forge/api/migrations/007_postgres_core_foundation.sql:106` (`Games` nest), `forge/api/migrations/043_unify_eggs_templates_mounts.sql:1`, `forge/api/migrations/091_seed_minecraft_java.sql:3`
- Forge frontend catalogue glue: `forge/web/lib/egg-templates.ts:1` (`EGG_TEMPLATES:14`), `forge/web/lib/app-templates-data.ts:1`, `forge/web/lib/api/apps.ts:369` (`fetchAppTemplates`), `forge/web/components/admin/AdminNestsEggs.tsx:1` (`dockerImageLines:14`, `importEgg:124`), `forge/web/app/admin/nests/page.tsx:1`, `forge/web/app/admin/app-templates/page.tsx:1`
- Forge service seams: `forge/api/internal/services/catalog/catalog.go:1`, `forge/api/internal/store/store_catalog.go:1`, `forge/api/internal/services/appstore/service.go:1` + `.../seed.go:27` + `.../seed.go:216`, `forge/api/internal/store/store_app_store.go:1`, `forge/api/cmd/api/main.go:529` (`SeedDefaultApps`)
- 1Panel: `reference/app-platforms/1panel/agent/app/model/compose_template.go:3`, `reference/app-platforms/1panel/agent/app/dto/compose_template.go:5`, `reference/app-platforms/1panel/agent/app/service/compose_template.go:27` (`List:27`, `Batch:58`), `reference/app-platforms/1panel/agent/app/provider/catalog.go:1`

---

*Generated for Phase 6 / Subagent 06 — read-only, no product code modified. Cross-check with Subagent 02 (Forge eggs) and Subagent 10 (migration/seed integrity) for combined coverage of catalogue drift.*
