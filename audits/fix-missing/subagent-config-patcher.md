# Fix: egg config.files 6-parser patcher — stored-not-applied

**Task:** Implement `config.files` patching at beacon pre-start to fix Minecraft wrong port (`server.properties`).

## Inspection

- `forge/api/internal/store/store_nests.go:36` `Egg.Config json.RawMessage` — stored as JSON object via `normalizeJSONObject` (`store_nests.go:394`) with `{"files":{...}}` shape. `egg-templates.ts:64` (and `packages/game-templates/templates/minecraft-paper.json:16`) stores `config.files` as **map** `{"server.properties":{"parser":"properties","find":{...}}}` — Forge correctly preserves this.
- Reference `reference/game-hosting/pelican-wings/parser/parser.go:1` defines 6 parsers: `file, yaml, properties, ini, json, xml, toml` (plus `configMatchRegex` at `helpers.go:22`). `reference/.../parser/parser.go:114` `ConfigurationFile` with `FileName`, `Parser`, `Replace`.
- `beacon/internal/server/server.go:3600` `applyConfigurationFiles` existed but expected `config["files"].([]any)` array, while Forge sends `map[string]any` — mismatch caused no-op; also used `HasSpaceForWrite` but not pre-start.
- `beacon/internal/server/manager.go:676` `onBeforeStart` checked `ConfigurationSynced` and `RootDir` but never applied config files; `beacon/internal/server/server.go:990` `syncConfiguration` did call `applyConfigurationFiles` on sync, but not on every start, and clobbered boot file.

## Changes (bare minimum)

### 1. New patcher `beacon/internal/server/config_patcher.go` (new file)

- Handles **both shapes**: `[]any` (Wings array) and `map[string]any` (Forge object) via `parseConfigFiles` (`config_patcher.go:35`).
- Array entries: `file`/`path`, `parser`, `find` map, `replace` list (with `replace_with`/`value` fallback). Map entries: key = filename, value = `{parser, find, replace}`.
- Resolves templates via `resolveValue` (`config_patcher.go:20`): `{{server.build.default.port}}` → `allocation.Port`, `{{server.build.default.ip}}` → `allocationIP`, `{{env.VAR}}` / `{{VAR}}` → `env[VAR]`, supports `|default:''` filter, uses `placeholderRegex`.
- **Parsers:** `properties`, `yaml`/`yml`, `json` implemented; others (`file`, `ini`, `xml`, `toml`) log warn (`config_patcher.go:130`) per task (3 of 6).
  - `applyPropertiesPatch` (`config_patcher.go:145`): reads existing file (or creates), preserves comments, replaces `key=value` lines, appends missing keys, `0640`, `0750` dirs, `maxConfigFileSize` 64MiB.
  - `applyYamlPatch` (`config_patcher.go:190`): `yaml.v3` unmarshal to `map[string]any`, `setNestedValue` with dot paths and `foo[0]` array support, `coerceValue` for int/bool.
  - `applyJsonPatch` (`config_patcher.go:215`): `json.Unmarshal`/`MarshalIndent`, same `setNestedValue`.
- `patchConfigurationFiles` (`config_patcher.go:80`) — shared helper takes `rootDir`, `payload`, `env`, `port`, `ip`, validates `Rel` to prevent escape, logs per-file.

### 2. Fix `beacon/internal/server/server.go:3600`

- Now checks both `[]any` and `map[string]any` for `config.files` (`server.go:3605`), extracts `port`/`ip` via `allocationPortFromConfiguration`/`allocationIPFromConfiguration`, ensures `SERVER_PORT` in env, calls `patchConfigurationFiles(root, payload, env, port, ip)` (`server.go:3638`). Keeps legacy `content`/`properties`/`json` direct file creation path for array entries that have `content` without `find`.

### 3. Wire pre-start `beacon/internal/server/manager.go:724`

- After `syncServerStateFromPanel`, calls `applyPreStartConfigPatches` (`manager.go:724`), which reads `filepath.Join(root, ".config", "server.json")`, unmarshals hybrid payload (preserves both `settings` and `config`), extracts `env`/`port`/`ip` from `State`, calls `patchConfigurationFiles` (`manager.go:760`). Logs but does not fail start (Minecraft can still boot).

### 4. Preserve config across restarts

- `beacon/cmd/daemon/main.go:597` `syncServersFromPanel` now preserves existing `config` key when writing `.config/server.json` (merge, not overwrite) to avoid clobbering `server.properties` patches on daemon restart (GH-15).
- `beacon/internal/server/server.go:1006` `syncConfiguration` now merges existing file (`settings` + `config`) before writing, so boot and sync payloads coexist.

### 5. Forge store verification

- `forge/api/internal/store/store_nests.go:36` `Egg.Config` already normalized via `normalizeJSONObject`; `forge/api/internal/http/handlers_remote.go:224` `buildDaemonServerConfiguration` correctly unmarshals `ConfigJSON` into `ServerConfiguration.Config` and `forge/api/internal/daemon/client.go:444` `ServerConfiguration.Config map[string]any` carries it to beacon via `SyncServerConfiguration` (not `CreateRequest`, which is for container create; config is sent via dedicated sync). Verified no code change needed for Forge — preservation and sync are correct. `forge/api/internal/services/clustermanager/service.go:713` `runtimeConfiguration` includes `Config`.

### 6. Duplicate registration guard

- `beacon/internal/installer/operations/operation.go:48` `Register` now idempotent (logs, returns) instead of panicking on duplicate `"extract"` (added via `operations_bootstrap.go:7` in mvp-v2). Prevents test/daemon crash when same operation is imported via multiple paths (go.work).

## Verification

- `go vet ./beacon/...` — no output (pass).
- `go test ./beacon/internal/server -run TestPatch -count=1 -v` — 5 tests pass: `TestPatchPropertiesMapShapeMinecraft`, `TestPatchArrayShape`, `TestPatchYamlAndJson`, `TestPatchCreatesIfMissing`, `TestPatchUnsupportedLogsWarn` (logs for `xml`).
- `go test ./beacon/internal/server -run TestManagerPreStart -count=1 -v` — 2 tests pass: `TestManagerPreStartPatchesViaDisk`, `TestManagerPreStartNoConfigNoOp`.
- `go test ./beacon/... -count=1` — all packages pass (server 7.5s, etc.).
- Manual check: `server.properties` with `server-port=25565` patched to `25577` via `{{server.build.default.port}}`; `query.port` also; `motd` preserved; missing file created; yaml/json dot paths work.

## Scope

Bare minimum for Minecraft `server.properties` wrong port. Other parsers (`file`, `ini`, `xml`, `toml`) log warn as required. Future work: wildcard `.*` and `IfValue`/`regex:` handling for yaml/json (currently logged/ignored), `configMatchRegex` for `{{config.docker.interface}}` (not needed for game ports).

## Files Modified

- `beacon/internal/server/config_patcher.go` (new)
- `beacon/internal/server/config_patcher_test.go` (new, verifies 3 parsers)
- `beacon/internal/server/manager_config_patcher_test.go` (new, pre-start via disk)
- `beacon/internal/server/server.go:3600`
- `beacon/internal/server/manager.go:676,760`
- `beacon/cmd/daemon/main.go:597`
- `beacon/internal/installer/operations/operation.go:48`

## Forge Check

- `forge/api/internal/daemon/client.go:444` `ServerConfiguration.Config` present — no change.
- `forge/api/internal/store/store_nests.go:36,394` preserves `config` — no change.
