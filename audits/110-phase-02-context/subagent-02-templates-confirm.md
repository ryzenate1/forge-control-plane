# Subagent 02 — Templates / Foundry Confirmation (Phase 110-02-02)

**Date:** 2026-08-24  
**Branch inspected:** `mvp-2` (HEAD `ca06f74`, dirty working tree)  
**Prior audits reconciled:** `audits/phase-02/subagent-02-eggs-templates.md` (LF-01..LF-05), `audits/phase-06/subagent-06-templates-catalog.md` (L1..L4, C11 seeding), `audits/final-parity/subagent-01-game-hosting.md` (GH-13..GH-16), `audits/FINAL_PARITY_AUDIT.md` §12.6 DUPLICATE #5 (DB-09..DB-10, GH-16)  
**Live files inspected:** `forge/api/internal/store/store_nests.go`, `forge/api/internal/store/store_egg_variables.go:142`, `forge/api/internal/store/store_templates.go`, `forge/api/migrations/091_seed_minecraft_java.sql`, `forge/web/lib/egg-templates.ts:27`, `packages/game-templates/*` (git status), `forge/web/lib/app-templates-data.ts`, `beacon/internal/server/server.go:3254`, `forge/api/internal/store/seeder.go`, `forge/api/internal/http/handlers_remote.go`, `forge/api/internal/store/store_servers_control.go`

---

## 0. Verdict Summary

| Question from task | Prior audit claim | Live verdict | file:line proof |
|---|---|---|---|
| **93% seeding deficit fixed?** | Phase-06 L3 + Final GH-16: 1 egg seeded, 14 templates phantom (`091_seed` only) | **NOT FIXED** — still 1/14 = 7% seeded, 93% deficit | `forge/api/migrations/091_seed_minecraft_java.sql:1-29` single `INSERT ... 'Minecraft Java'` + 2 variables; `forge/api/internal/store/seeder.go:38-91` `DefaultSeeder` only `default-roles`+`default-settings`; `forge/api/migrations/*.sql:197` files, no new template seed beyond 091; `forge/api/internal/store/store.go:1349-1457` demo `Seed()` only inserts one `Minecraft Java` egg |
| **Slash bug fixed?** | Phase-02 LF-01 + Final GH-14 `BROKEN P0`: `regex:/.../` compiled with slashes, `Split("|")` breaks character classes | **NOT FIXED** — identical code | `forge/api/internal/store/store_egg_variables.go:142-183` `strings.Split(rules,"|")` + `strings.Cut(rule,":")` + `regexp.Compile(arg)` at `store_egg_variables.go:177`; `forge/web/lib/egg-templates.ts:80` still ships `rules:"required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` which will fail |
| **Config parser wired?** | Phase-02 C14 + LF-05: `config` JSON stored not applied; Final GH-15 `MISSING` | **PARTIALLY WIRED BUT BROKEN** — beacon now has `applyConfigurationFiles` but schema mismatch makes it a no-op for Forge's own templates | `beacon/internal/server/server.go:3254-3295` new function exists, but expects `config["files"].([]any)` (`server.go:3259`) while Forge stores `config.files` as **map** object (`egg-templates.ts:64-72` + `091_seed:15`); 6 wings parsers (`reference/.../parser/parser.go:114` `ConfigurationFile{File,Parser,Replace}`) not ported |
| **packages/game-templates on main now?** | Phase-06 C1: 14 templates on worktree branches, 0 on `main` checkout; Final DB-09: `0 on disk (deleted, uncommitted D)` | **TRACKED IN HEAD BUT DELETED ON DISK — phantom** | `git ls-tree -r HEAD --name-only | grep game-templates` lists 14 files (e.g. `packages/game-templates/templates/minecraft-paper.json`); `ls packages/` `shared-types,sdk` only; `git status --short` shows `D packages/game-templates/README.md` + 13 D (`index.json`, `template-schema.json`, `csgo.json` etc.) — file not present at `packages/game-templates/src/index.ts` on disk |

---

## 1. Prior Audit Claims vs Live Re-check

### 1.1 Phase-02 subagent-02 (eggs/templates)

Phase-02 identified 5 logic findings. Re-checked:

- **LF-01 slash bug** `phase-02/subagent-02:102` — claimed `validateVariableValue` mishandles `regex:/pattern/` including `Split("|")` inside regex. **Still true.** `store_egg_variables.go:142` `for _, rule := range strings.Split(rules, "|")` will split `"required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` correctly into 2 parts only because character class `|` not present, but any `regex:/a|b/` would split incorrectly; more critically `store_egg_variables.go:176-182` does `regexp.Compile(arg)` where `arg="/^([\\w\\d._-]+)(\\.jar)$/"` includes leading/trailing `/` — `regexp.Compile` treats slashes as literals, so `server.jar` never matches. No change since `git diff -- forge/api/internal/store/store_egg_variables.go` shows only audit `mustAuditJSON` fixes at `store_egg_variables.go:80,102,114`, no logic change. Live count: `forge/web/lib/egg-templates.ts:80,136` 2 occurrences of `regex:/.../` rules that would always fail import via `POST /eggs/:id/variables` or `POST /eggs/import`.

- **LF-02 CPU conflation** still present (`phase-02/subagent-02:180`), unchanged at `store_servers.go:209` + `beacon/internal/runtime/docker.go:924`. Not in scope but confirms no collateral fix leaked into templates.

- **C14 config parser** `phase-02/subagent-02:159` — claimed `config` JSON stored not applied, beacon lacks `wings/parser`. Live: `forge/api/internal/store/store_nests.go:34` `Egg{Config json.RawMessage}` persisted via `store_nests.go:232,341` but never interpreted except pass-through `handlers_remote.go:227` `config := map[string]any{}; json.Unmarshal(target.ConfigJSON,&config)` → `buildDaemonServerConfiguration` `store_servers_control.go:95` `e.config::text` → beacon `server.go:869` `applyConfigurationFiles`. So wiring now exists but is shallow (see §3).

- **C11 template vs egg dualism** — Phase-02 noted two template systems without sync. Live still exactly two (plus third app-templates): `store_templates.go:12` shim + `egg-templates.ts:27` static.

- **Overall verdict** Phase-02 `§6 Recommendations` items 1,4,5 (fix regex, implement parser, unify sources) — **none implemented**.

### 1.2 Phase-06 subagent-06 (templates-catalog)

| Phase-06 finding | Live status |
|---|---|
| **L3 seeding divergence 93% deficit** `phase-06/subagent-06:183` fresh `ListEggs` returns 1 vs `packages/game-templates/index.json` 14 | **Unchanged.** `091_seed_minecraft_java.sql:3-29` still sole production seed; `packages/game-templates/templates/*.json` 14 files tracked but deleted on disk; `forge/web/lib/egg-templates.ts:27` still 14 `EggTemplateItem[]` frontend constants; `seeder.go:38` still no egg seeding; no `SeedDefaultApps`-style upsert for eggs (`forge/api/cmd/api/main.go:529` only seeds AppStore). `git status` D confirms packages not on disk, so `main` cannot build/validate them. |
| **L1 conditional-install expressiveness** `phase-06/subagent-06:154` 24 Puffer ops → single shell blob | **Unchanged.** `template-schema.json` (deleted on disk) + `egg-templates.ts` `installScript` remain single script (`egg-templates.ts:44,105,156` etc.), no typed ops. |
| **L2 startup variable resolution** `phase-06/subagent-06:168` validator scope narrow | **Unchanged.** `store_egg_variables.go:142` same, `validate-templates.mjs` still absent on disk (deleted). |
| **C11 seeding & bootstrap** `phase-06/subagent-06:99` "14 templates exist nowhere after migrate" | **Still true.** `ls forge/api/migrations/*.sql` shows 197 files, last template-related is `091_seed_minecraft_java.sql`; `store.go:1349` Seed still only one `Minecraft Java`. |

### 1.3 Final-Parity subagent-01 GH-13..GH-16

| GH | Final claim | Live re-check | file:line |
|---|---|---|---|
| **GH-13 Egg/nest/egg_variables** `PARTIAL` — `store_nests.go:34` Egg, `store_egg_variables.go:17` valid | **Still PARTIAL.** `store_nests.go:34-55` `Egg` struct unchanged; `store_nests.go:361` `normalizeDockerImages` still handles `map`+`[]string`; `store_nests.go:276-320` `normalizeJSONObject/JSONArray` unchanged. Structurally sound, but variable validation + seeding gaps keep PARTIAL. |
| **GH-14 Variable validation slash bug** `BROKEN P0` — `store_egg_variables.go:176` compiles with slashes | **Still BROKEN P0.** `store_egg_variables.go:176-182` byte-identical to audit. Repro: `POST /eggs` with `rules:"required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` + `defaultValue:"server.jar"` → `400 value does not match`. Import of `minecraft-paper.json:58` canonical `SERVER_JARFILE` variable would fail. No `REGEX` delimiter stripping, no `|`-inside-regex handling. |
| **GH-15 Config parser 6 parsers** `MISSING` — `wings/parser/parser.go:1` not reimplemented, `config` stored not applied | **Upgraded to PARTIAL/BROKEN (not COMPLETE).** Beacon now has `beacon/internal/server/server.go:3254` `applyConfigurationFiles` (60 lines) called at `server.go:869` during `PUT /servers/{id}/config` sync. But: (a) expects `files: []any` with `path/file` keys (`server.go:3259-3276`), Forge stores `files: {"server.properties": {"parser":"properties","find":{"server-port":"{{server.build.default.port}}"}}}` (`egg-templates.ts:64-72`, `minecraft-paper.json:22` style). Live `091` seed has `config: {"stop":"stop","logs":...,"startup":...}` with **no `files` key at all** (`091_seed:15`). So Forge's own seed never exercises parser; (b) only 3 synthetic file modes (`content`/`properties`/`json`) implemented (`server.go:3280-3295`), not the 6 wings parsers `file/yaml/properties/ini/json/xml` (`parser/parser.go:1` + `pelican-wings/server/config_parser.go:34`); no `configMatchRegex` (`{{config.docker.interface}}`) or `Startup` placeholder resolution beyond `renderTemplate`. Reference files like `minecraft-paper` expecting `server-port` patch to `{{SERVER_PORT}}` will stay literal `{{server.build.default.port}}` because `resolveStartupCommand` only resolves egg vars, not `server.build.*` (`store_schedules.go:14`). |
| **GH-16 Template systems DUPLICATE** — 3 systems: DB 1 egg, FS 14 not seeded, localStorage 5 | **Still DUPLICATE.** Identical: `forge/api/internal/store/store_templates.go:12` DB shim (1 egg live), `forge/web/lib/egg-templates.ts:27` 14 FS constants, `forge/web/lib/app-templates-data.ts:3` 5 localStorage defaults (`nginx:7, node:17, python:26, postgres-compose:35, redis:48`). Count verified: `wc -l egg-templates.ts:563` contains 14 `id:` entries (`minecraft-paper:29` through `zomboid:531`), `app-templates-data.ts` 5 `DEFAULT_APP_TEMPLATES`. Final audit §12.6 #5 wording "1 seeded egg vs 14 on disk `egg-templates.ts:27` vs 44 PufferPanel templates" remains accurate (44 Puffer ref vs 14 Forge). |

### 1.4 FINAL_PARITY_AUDIT §12.6 DUPLICATE #5

> `Three template systems (DB eggs / FS game-templates / localStorage app-templates) — 1 seeded egg vs 14 on disk egg-templates.ts:27 vs 44 PufferPanel templates.` `FINAL_PARITY_AUDIT.md:380` + §12.6 line 5

Live: **Confirmed duplicate persists.** See GH-16 above. Additional proof: `packages/game-templates` would be the canonical FS game-templates source, but it is deleted on disk (`git status D` 14 files) so the FS source is now actually `forge/web/lib/egg-templates.ts` only, which is **frontend-only** and never materializes via `DefaultSeeder` or migration. The DB source (`store_templates.go:12` + `store_nests.go`) is empty except `091_seed`. The localStorage source (`app-templates-data.ts:3` `STORAGE_KEY="forge.app-templates.v1"` + `loadUserTemplates:56` reads `window.localStorage`, `saveUserTemplates:66` writes, `getAllTemplates:71` merges defaults+user) is **entirely browser-local**, never touches `store_apphosting` or `nests/eggs`. `fetchAppTemplates()` fallback in `forge/web/lib/api/apps.ts:369` was noted in phase-06 as falling back to `getAllTemplates()` on network error — verified still present. No convergence work landed: `forge/api/internal/store/seeder.go:38` still lacks `egg-templates` seeding, `forge/api/migrations/` has no `092_seed_game_templates.sql`, `forge/api/cmd/api/main.go:529` only seeds AppStore.

---

## 2. Live File-by-File Proof

### 2.1 `forge/api/internal/store/store_nests.go`

- **Nest/Egg model:** `store_nests.go:17-50` `Nest{ID,Name,Description,EggCount}` and `Egg{ID,NestID,Name,Description,DockerImages json.RawMessage,Startup,Config json.RawMessage,DefaultMemoryMB,InstallScript,InstallContainer,InstallEntrypoint,FileDenylist,ConfigFrom,CopyScriptFrom,UpdateURL,Author,Features,StartupCommands}` — matches phase-02 inventory. No `uuid`/`author` denormalization beyond `Author string`.
- **CRUD:** `store_nests.go:98-200` `ListNests/GetNest/CreateNest/UpdateNest/DeleteNest`; `store_nests.go:188-350` `ListEggs/GetEgg/CreateEgg/UpdateEgg/DeleteEgg`. `CreateEgg:244` validates `dockerImages` via `normalizeDockerImages` and defaults `DefaultMemoryMB 1024` + `InstallContainer alpine:3.21` + `InstallEntrypoint sh` — unchanged.
- **Docker image normalization:** `store_nests.go:361-392` `normalizeDockerImages` correctly handles both `map[string]string` and legacy `[]string` array → map (`images[image]=image`). This is the one "better than reference" noted in phase-02; still present.
- **Seeding relevance:** `store_nests.go` has no seeding; eggs only created via API or `091_seed` / demo `Seed()`.

### 2.2 `forge/api/internal/store/store_egg_variables.go:142`

```go
// store_egg_variables.go:142-183
func validateVariableValue(value, rules string) error {
    for _, rule := range strings.Split(rules, "|") {          // :142
        rule = strings.TrimSpace(rule)
        name, arg, _ := strings.Cut(rule, ":")                 // :145
        switch name {
        case "", "nullable", "string":
        case "required": ...
        case "max","min": ...
        case "in": ...
        case "regex":
            pattern, err := regexp.Compile(arg)                // :177 — BUG: arg == "/^([\\w\\d._-]+)(\\.jar)$/" with slashes
            if !pattern.MatchString(value) { return ... }      // :181
        }
    }
}
```

- **Bug 1 — delimiter:** Pterodactyl `EggVariable.php:66` stores `regex:/^([\\w\\d._-]+)(\\.jar)$/` with `/` delimiters. Forge does not strip them, so `regexp.Compile("/^...$/")` requires literal `/` characters. Any import of `packages/game-templates/templates/minecraft-paper.json:58` (`rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"`) or `forge/web/lib/egg-templates.ts:80` same rule will reject valid `server.jar`.
- **Bug 2 — split:** `strings.Split(rules,"|")` naïvely splits regex alternation (`regex:/a|b/` would become 2 rules). Not exercised by current templates but latent.
- **Git diff:** `git diff -- forge/api/internal/store/store_egg_variables.go` shows only audit escaping fixes at `store_egg_variables.go:77,99,111` (`mustAuditJSON`), **no regex fix**. Live equals final-parity snapshot.

- **Call sites:** `store_egg_variables.go:129` `validateEggVariableRequest` → `validateVariableValue(req.DefaultValue, req.Rules)` at `store_egg_variables.go:139`; also `store_startup.go:73` `validateVariableValue(value, rules)` for `UpdateServerStartupVariable` with `user_viewable=true` gate (`store_startup.go:62`). So both create and update paths broken.

### 2.3 `forge/api/internal/store/store_templates.go`

- `store_templates.go:12-27` `ListTemplates` comment `compatibility transform over canonical eggs` delegates to `ListEggs(ctx,"")` then `templateFromEgg` per egg — not a separate table. Verified: no `server_templates` table access.
- `store_templates.go:38-67` `CreateTemplate` requires `name,image`, looks up `nests WHERE name='Games'` (`store_templates.go:48`), marshals `images map[string]string{image:image}` (`store_templates.go:51`), then `CreateEgg`. This is the shim used by `handlers_admin.go:2043` `GET /templates` and `handlers_admin.go:2064` `POST /templates`.
- `store_templates.go:99-124` `templateFromEgg` unmarshals `DockerImages` map, sorts keys (`sort.Strings(keys)` at `store_templates.go:108`), picks `keys[0]` lexicographically — matches phase-02 C03 but loses user's `images` label ordering.
- **No seeding:** `store_templates.go` never seeds; it only transforms existing eggs.

### 2.4 `forge/api/migrations/091_seed_minecraft_java.sql`

```sql
-- 091_seed_minecraft_java.sql:1-29
INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb, ...)
SELECT '91ec0000-0000-4000-8000-000000000001', id, 'Minecraft Java', 'Minecraft Java server powered by itzg/minecraft-server.',
       '{"Java 21":"itzg/minecraft-server:java21"}'::jsonb,
       '',  -- startup empty
       '{"stop":"stop","logs":{"custom":false},"startup":{"done":["Done ("]}}'::jsonb,
       2048, ... FROM nests WHERE name='Games' ON CONFLICT (nest_id,name) DO NOTHING;
-- + 2 egg_variables: VERSION LATEST (091:32) and TYPE PAPER (091:48), both `required|string|max:64/32`
```

- This is **the only production egg seed**. Migration count `ls forge/api/migrations/*.sql | wc -l` = 197; next migrations `092_durable_operations.sql`, `090_allocation_transport.sql` (now enforces `UNIQUE (node_id,ip,port,protocol)` at `090:12`), through `210_servers_created_at_index.sql` — none insert eggs/variables. `store.go:1349` demo `Seed()` inserts a second egg `111...` `Minecraft Java` with `itzg/minecraft-server:latest` only when `API_SEED_DEMO=true` and non-production, not production path.
- **93% deficit math:** Phase-06 counted 14 curated game-templates (`csgo, enshrouded, factorio, minecraft-bedrock, minecraft-paper, minecraft-vanilla, palworld, rust, satisfactory, teamspeak3, terraria, valheim, 7days2die, zomboid`) vs 1 seeded → 1/14 ≈ 7%, deficit 93%. Live `forge/web/lib/egg-templates.ts:27` still enumerates exactly 14 (`minecraft-paper:29` … `zomboid:531`), so deficit unchanged.

### 2.5 `forge/web/lib/egg-templates.ts:27` vs `packages/game-templates` on disk (git status D)

- **Live `egg-templates.ts`:** `egg-templates.ts:1-25` type `EggTemplateItem {id, name, description, game, author, image, images, startup, installContainer, installEntrypoint, installScript, config, env[], features, fileDenylist}`; `egg-templates.ts:27` `export const EGG_TEMPLATES: EggTemplateItem[] = [` with 14 entries verified by grep `id: "minecraft-paper"` … `id: "zomboid"` (lines 29,92,146,178,209,258,289,320,356,391,424,457,499,532). Each duplicates `packages/game-templates/templates/*.json` content at time of branch (e.g., `egg-templates.ts:80` `rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` identical to `minecraft-paper.json:58`).
- **On-disk `packages/game-templates`:** `ls packages/` returns `sdk, shared-types` only; `cat packages/game-templates` `No such file or directory`; `git ls-tree -r HEAD --name-only | grep game-templates` lists 14 tracked files (`packages/game-templates/README.md`, `index.json`, `package.json`, `scripts/validate-templates.mjs`, `src/index.ts`, `src/types.ts`, `template-schema.json`, `templates/{7days2die,csgo,enshrouded,factorio,minecraft-bedrock,minecraft-paper,minecraft-vanilla,palworld,rust,satisfactory,teamspeak3,terraria,valheim}.json`); `git status --short` shows `D packages/game-templates/README.md` plus 12 more D. Means package is **tracked in HEAD** (commit `bb893a5 feat: mvp-v2`), but **deleted in working tree** (unstaged). `git diff --stat | grep game-templates` empty (not staged), `git ls-files --deleted` lists all 14. So build will fail: `packages/game-templates/package.json:9` `prebuild: node scripts/validate-templates.mjs` cannot run; web `egg-templates.ts` is the only surviving copy, and it has **no validator** (`phase-06 L2` notes `EGG_TEMPLATES` array is unverified TS literals).
- **Consequence:** Phase-06 recommendation "Collapse dual template sources — delete either `packages/game-templates` or `egg-templates.ts` as canonical; generate the other" — **not done**; instead both are out of sync by deletion.

### 2.6 `forge/web/lib/app-templates-data.ts` localStorage

- `app-templates-data.ts:1-5` `import type {AppTemplate} from "@/lib/api/apps"` + `STORAGE_KEY="forge.app-templates.v1"` (`app-templates-data.ts:3`).
- `app-templates-data.ts:5-54` `DEFAULT_APP_TEMPLATES` 5 entries: `nginx:7` image, `node:17` git, `python:26` git, `postgres-compose:35` compose, `redis:48` image — each with `defaultPorts[]`, `defaultEnvVars{}`, `defaultResources{cpu,memory,disk:string}` — **orthogonal to game-templates** (`ports[]` vs `defaultPorts`, `env[]` vs `defaultEnvVars`).
- `app-templates-data.ts:56-75` `loadUserTemplates:56` reads `window.localStorage.getItem(STORAGE_KEY)`, `saveUserTemplates:66` writes, `getAllTemplates:71` merges defaults+user filtered by `defaultIDs`. Entirely client-side, never hits `store_nests.go`/`store_templates.go`/`store_apphosting.go`. `forge/web/app/admin/app-templates/page.tsx:1` renders this; `forge/web/lib/api/apps.ts:369` `fetchAppTemplates()` falls back to `getAllTemplates()` on network error — so even network failure silently serves localStorage catalog, not DB.
- **Parity:** Correctly identified as third template system in FINAL_PARITY_AUDIT §12.6; still duplicate, still unwired to backend.

---

## 3. Config Parser — Is It Wired?

### Claim to Check

Phase-02 C14 + LF-05 and Final GH-15 said `eggs.config` stored not applied, `server.properties` patch no-op, Minecraft wrong port. Live has new beacon code, so re-evaluate.

### What Was Added

- `beacon/internal/server/server.go:3254` `func (s *Server) applyConfigurationFiles(serverID string, payload map[string]any) error` — 42 lines added since phase-02. Called at `server.go:869` inside the config-sync handler (`PUT`/`POST` config sync from panel) after writing `.config/server.json`. It builds `env` map from `payload["environment"]` (`server.go:3263`), then iterates `config["files"]` (`server.go:3259`).

### Why It Is Still BROKEN

1. **Schema shape mismatch (critical):** Beacon expects `config["files"]` to be `[]any` array of objects with `path`/`file` keys (`server.go:3259` `files, _ := config["files"].([]any)` + `server.go:3270` `pathValue, _ := fileConfig["path"]` / `fileConfig["file"]`). Forge stores `config.files` as **object map** `{"server.properties": {"parser":"properties","find":{"server-port":"{{server.build.default.port}}"}}}` — from `egg-templates.ts:63-73`, `minecraft-paper`/`vanilla`/`bedrock`/`terraria` etc. When `config["files"]` is a `map[string]any`, the type assertion `([]any)` fails, `files` is `nil`, `len(files)==0` early return at `server.go:3260` — **function becomes a no-op for every Forge-authored template**. Even `091_seed` has no `files` key at all, so also no-op.

2. **Parser coverage incomplete:** Wings `reference/game-hosting/pterodactyl-wings/parser/parser.go:114` `ConfigurationFile{File string, Parser string ∈ {file,yaml,properties,ini,json,xml}, Replace []ConfigurationFileReplacement}` with `parse*File` routing and `ConfigMatchRegex {{config.docker.interface}}`, `XmlValueMatchRegex`, wildcard `.*`, array `something[1]`. Beacon reimplement at `server.go:3280-3300` only handles 3 synthetic modes: raw `content` string, `properties` map → `key=value` lines, `json` object → `MarshalIndent`. No `yaml`, `ini`, `xml`, no `find/replace` semantics, no `configMatchRegex`.

3. **Placeholder resolution incomplete:** `server.go:3298` `renderTemplate` replaces `{{KEY}}`, `{{ KEY }}`, `{{env.KEY}}`, `{{KEY|default:''}}` from `payload["environment"]` only. Forge's `resolveStartupCommand` (`store_schedules.go:14`) replaces `{{VAR}}` and `{{lower(v)}}` from egg variables, but **neither** resolves `{{server.build.default.port}}` or `{{server.build.default.ip}}` which appear in `egg-templates.ts:69-70` (`"server-port": "{{server.build.default.port}}"`). Those are Pterodactyl server-build placeholders that should be `allocationPort`/`allocationIP` (`handlers_remote.go:126` `SERVER_PORT`, `clustermanager/service.go:636,682` same). Beacon's `effectiveEnvList` (`server.go:3302`) does inject `SERVER_PORT`/`SERVER_IP`/`SERVER_MEMORY`/`STARTUP` into `renderTemplate` via `state.EnvVars`, but `applyConfigurationFiles` at `server.go:3263` reads `payload["environment"]` which is `buildDaemonServerConfiguration`'s `environment` map (`handlers_remote.go:233-239`) that **does** contain those — so `{{SERVER_PORT}}` would resolve, but `{{server.build.default.port}}` will not (key mismatch). The templates mix both syntaxes (`minecraft-paper` uses `server.build.default.port`, `terraria` uses `{{server.build.default.port}}` vs `{{MAX_PLAYERS}}`).

4. **Forge-side wiring missing:** `forge/api/internal/store/store_servers_control.go:95` loads `e.config::text` as `target.ConfigJSON` string, then `handlers_remote.go:227` `json.Unmarshal([]byte(target.ConfigJSON),&config)` → `buildDaemonServerConfiguration` `handlers_remote.go:277` `Config: config` passed to beacon via `daemon.ServerConfiguration{Config config}` (`handlers_remote.go:255,277`). So the `config` blob **does** reach beacon, but beacon's `applyConfigurationFiles` is the only consumer, and it is shape-incompatible as above. No fallback to `wings/parser` or `server.UpdateConfigurationFiles()` (`pelican-wings/server/config_parser.go:34`).

**Conclusion:** Status moves from `MISSING` (phase-02) to `PARTIAL/BROKEN` (now has code, but dead for Forge's own templates). Final parity GH-15 remains effectively MISSING for operational purposes; the new function is not exercised by any real template.

---

## 4. Seeding Deficit Deep Dive

### Production Path

- `forge/api/migrations/007_postgres_core_foundation.sql:106` seeds `nests` `Games`.
- `091_seed_minecraft_java.sql:3-29` seeds 1 egg. No other migration inserts into `eggs`/`egg_variables`. Verified `grep -rn "INSERT INTO eggs" forge/api/migrations/*.sql` returns only `091_seed` + demo `Seed()` path.
- `forge/api/internal/store/seeder.go:38-91` `DefaultSeeder` registers only `default-roles` and `default-settings`. No game content. `forge/api/cmd/api/main.go:212` `connected.Seed(ctx)` is demo seed (gated by `API_SEED_DEMO`), not production. `main.go:529` `SeedDefaultApps` seeds 7 AppStore compose apps (`forge/api/internal/services/appstore/seed.go:27` 7 entries: `nginx, postgres, redis, mongo, mariadb, portainer, traefik`) — not eggs.

### What Would Fix It (not done)

Phase-06 recommended `EggSeed` analogous to `appstore/seed.go:216` upserting `packages/game-templates/templates/*.json` or `egg-templates.ts` into `eggs+egg_variables` via `cmd/api/main.go:529`-style boot. No such code exists: `grep -rn "EGG_TEMPLATES\|game-templates\|SeedDefault.*Egg\|UpsertEgg" forge/api --include="*.go"` hits only `handlers_admin.go:990` CRUD handlers, not seeding.

### Impact

Fresh `ListEggs(ctx,"")` (`store_nests.go:188`) returns 1 row; `ListTemplates` (`store_templates.go:14`) same; `AdminNestsEggs` UI (`forge/web/app/admin/nests/[nestId]/eggs/page.tsx:1`) shows 1 card; Browse Templates CTA (`AdminNestsEggs.tsx: importEgg:124`) is manual paste-JSON import validating only `dockerImages.length>0`, no batch import, no `Batch`-style upsert like `1panel/.../service/compose_template.go:58`. Any claim Forge ships 14 game servers OOTB is false on `main`-equivalent install.

---

## 5. Cross-Check: Is Anything Better Than Reported?

- **Docker image map:** Phase-02 noted Forge `normalizeDockerImages` handles both map and array, better than panel. Still true (`store_nests.go:361`). No regression.
- **Allocation transport:** `090_allocation_transport.sql:12` now correctly enforces `UNIQUE (node_id,ip,port,protocol)` and `CHECK protocol IN ('tcp','udp')`, fixing the `F-G-11` uniqueness bug noted in `master-finding`. This is unrelated to templates but confirms DB evolved.
- **Audit JSON escaping:** `store_egg_variables.go:77,99,111` + `store_schedules.go:197,226,242,309,356,388` now use `mustAuditJSON` (`store_audit.go:116`) instead of `fmt.Sprintf` injection — narrow fix landed, not template logic.
- **Beacon config sync:** `beacon/server.go:869,3254` new `applyConfigurationFiles` is progress vs `MISSING`, but shape-mismatch limits value.

No template-seeding, regex, or parser fix landed beyond these.

---

## 6. Required Proof Index (file:line)

- Egg/Nest model: `forge/api/internal/store/store_nests.go:17,34,98,188,244,361`
- Egg variable validation bug: `forge/api/internal/store/store_egg_variables.go:15,42,129,142-183` (slash bug at `store_egg_variables.go:177`)
- Template shim: `forge/api/internal/store/store_templates.go:12,38,99`
- Production seed: `forge/api/migrations/091_seed_minecraft_java.sql:1,3,15,32,48`
- Demo seed (single egg, not 14): `forge/api/internal/store/store.go:1349,1440,202`
- DefaultSeeder (no eggs): `forge/api/internal/store/seeder.go:38,41,64,91`
- AppStore seed (7 apps, not eggs): `forge/api/cmd/api/main.go:529` + `forge/api/internal/services/appstore/seed.go:27`
- Egg templates frontend (14): `forge/web/lib/egg-templates.ts:1,27,29,80,135,146,178,209,258,289,320,356,391,424,457,499,532,563`
- Game-templates on disk deleted: `git status --short` `D packages/game-templates/...` (14 D), `git ls-tree -r HEAD` lists them, `ls packages/` shows `sdk,shared-types` only, `packages/game-templates/src/index.ts` absent
- App templates localStorage: `forge/web/lib/app-templates-data.ts:3,5,56,66,71`
- Startup resolution: `forge/api/internal/store/store_schedules.go:14` `resolveStartupCommand`, `forge/api/internal/store/store_startup.go:11,50,62,73`
- Provision target (loads all vars, no parser): `forge/api/internal/store/store_servers_control.go:82,95,154,179`
- Remote payload (passes config blob): `forge/api/internal/http/handlers_remote.go:99,120,176,224,255,277`
- Beacon config parser (new but shape-mismatched): `beacon/internal/server/server.go:869,3254,3259,3270,3280,3302`
- Reference parser (6 parsers): `reference/game-hosting/pterodactyl-wings/parser/parser.go:114,240,282` + `pelican-wings/server/config_parser.go:34`
- Reference egg variable: `reference/game-hosting/pterodactyl-panel/app/Models/EggVariable.php:29,43,66`
- Reference spec (24 ops): `reference/game-hosting/pufferpanel-templates/spec.json:1,274` + `minecraft/minecraft.json:173,244`

---

## 7. Confirmation Statement

- **Phase-02 LF-01 (slash bug):** CONFIRMED STILL BROKEN — `store_egg_variables.go:142` unchanged.
- **Phase-06 L3 (93% seeding deficit):** CONFIRMED STILL BROKEN — `091_seed` sole egg, `seeder.go` no template seed, `packages/game-templates` deleted on disk.
- **Final GH-14:** CONFIRMED BROKEN P0 — same file:line.
- **Final GH-15:** CONFIRMED still BROKEN/MISSING operationally — beacon `applyConfigurationFiles` exists but is no-op for Forge's `config.files` map shape; 6-parser semantics not ported.
- **Final GH-16 + §12.6 DUPLICATE #5:** CONFIRMED STILL DUPLICATE — three disjoint catalogs (DB 1, `egg-templates.ts:27` 14 constants, `app-templates-data.ts:3` 5 localStorage) with no single source of truth and no seeding bridge.
- **packages/game-templates on main:** CONFIRMED phantom — tracked in HEAD (`git ls-tree`), deleted on disk (`git status D` ×14), not buildable.

No evidence any template/egg/seeding recommendation from prior audits was implemented beyond narrow audit-JSON escaping (`mustAuditJSON`). The template subsystem is structurally unchanged since final-parity snapshot.
