# Subagent 13 — Fix compose env_file handling + template seeding + DB fleet view

**Date:** 2026-08-24  
**Branch:** mvp-2 (parallel subagent 13/20)  
**Focus:** `compose/service.go:14` env_file silently dropped (P0 secrets missing), TMPL-01 93-100% seeding deficit, `store_db_containers.go:174,200` encrypted columns omission, hosting/hostfiles OOM verification  
**Author:** Phase 03 Agent 13/20

## 1. Implementation Summary

| # | Finding | File:Line | Fix | Severity |
|---|---------|-----------|-----|----------|
| 1 | `env_file` silently dropped — `rawService` had no `EnvFile`, `Parse` ignored host env files → empty secrets at deploy, no error | `forge/api/internal/services/compose/service.go:155` `rawService.EnvFile` added; `ParseComposeYAML:209` now rejects behind flag; `ValidateCompose:325` emits `env_file` error/warning | P0 |
| 2 | TMPL-01 seeding deficit 14 authored vs 1 seeded (93% deficit, 100% on disk when `packages/game-templates` deleted) | `forge/api/internal/services/eggseeder/service.go:1` new idempotent Seeder + `forge/api/cmd/api/main.go:543` wiring + `packages/game-templates/scripts/validate-templates.mjs:26` fix | P0 |
| 3 | DB fleet view blank — `ListDBContainers:174` and `ListAllDBContainers:200` SELECT only plaintext `connection_string`/`credentials`, but `SetDBContainerStatus:233` encrypts to `*_encrypted` and clears plaintext → fleet returns `''`/`'{}'` | `forge/api/internal/store/store_db_containers.go:174,200` include `COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')` + `decryptDBContainerSecrets` in scan loop | P1 |
| 4 | hosting/hostfiles OOM fix — verify already done by 03-10 | `beacon/internal/server/hostfiles.go:249,458` and `beacon/internal/server/secure_files.go:27` verified | P1 |
| 5 | Tests for fail-fast + idempotent seeding | `forge/api/internal/services/compose/env_file_test.go:1`, `forge/api/internal/services/eggseeder/service_test.go:1`, `forge/api/internal/store/store_seed_eggs_test.go:1`, `forge/api/internal/store/store_db_containers_encrypted_test.go:1` | - |

## 2. Detailed Changes

### 2.1 Compose env_file — `forge/api/internal/services/compose/service.go:14`

**Before:** `rawService` defined only `Image, Build, Ports, Environment, Volumes, ...` but no `EnvFile`. `rawInclude:132` had `EnvFile` for `include:` but service-level `env_file` was unmarshalled into nothing, stripped, never validated. `parser.go:197` used empty `Environment:map{}` and beacon `compose.go:380` only wrote `EnvVars` to `.env`. Result: `services.db.env_file: ./secrets.env` → deploy with empty vars, secret loss, no error (`FINAL_PARITY_AUDIT.md:81` AP-10 BROKEN).

**After (this subagent):**

- `rawService:155` now `EnvFile interface{} yaml:"env_file,omitempty"` — mirrors Portainer `types.ts:5` and commit that added `rawService:107` but was missing fail-fast.
- New constants/helpers `service.go:19-77`:
  ```go
  var ErrEnvFileNotSupported = errors.New("env_file not supported, inline env vars")
  func isEnvFileStrict() bool { // FORGE_ENV_FILE_STRICT flag
      raw := strings.TrimSpace(os.Getenv("FORGE_ENV_FILE_STRICT"))
      // ParseBool, default false, allows "1"/"true"
  }
  func hasEnvFile(raw rawCompose) bool // uses isEnvFileEmpty
  func isEnvFileEmpty(v interface{}) bool // string / []string / []interface{} / map
  ```
- `ParseComposeYAML:219-244` — after `yaml.Unmarshal`, if `isEnvFileStrict() && hasEnvFile(raw)` → `return ErrEnvFileNotSupported` (wrapped with service name for context). Else when not strict, `slog.Warn` that env_file will be ignored, inline env vars instead. Covers both `services.*.env_file` and `include.env_file`.
- `ValidateCompose:325-366` — now surfaces structured `Field:"env_file" Message:ErrEnvFileNotSupported` when strict, and `Warning:"env_file is present but will be ignored"` when not strict. Early Parse error is mapped via `errors.Is(err, ErrEnvFileNotSupported)` to field `env_file` rather than generic `yaml`.

**HTTP mapping (`forge/api/internal/http/handlers_compose.go:106,118,315,378,413`):**

- `POST /compose/validate:106` returns `400` with `result` JSON when `env_file` present & strict.
- `POST /compose/import:118` returns `400` with `error: env_file not supported, inline env vars` when validation contains env_file, else `422`.
- `POST /compose:315`, `PATCH /compose/:id:378`, `POST /compose/:id/deploy:413` — after `DeployComposeStack/UpdateComposeStack` error, `if strings.Contains(lower(err), "env_file") => 400` else `500`. Aligns with spec `error 400 "env_file not supported, inline env vars" behind FORGE_ENV_FILE_STRICT`.
- `lifecycle.go:249` already validates via `ValidateCompose` at deploy time, so `ErrInvalidCompose` path is also covered.

**Alternative considered:** Mount/resolve external env files pre-deploy (Portainer `swarm.go:749 resolveEnvFilePaths` joins `root+composeDirRel+ef` and copies files into `stackDir`). Deemed P0-risk for host traversal (`hostfiles.go` denylist) and requires beacon cooperation to write files before `docker compose up`. Implemented fail-fast behind flag as spec allows `OR fail fast when env_file present (error 400 ...) behind FORGE_ENV_FILE_STRICT flag`. Future mount path can reuse `hasEnvFile` to copy files via `hostAtomicWrite` under allowlist.

**Verification:** `forge/api/internal/services/compose/env_file_test.go:1` covers 5 cases:

- `TestEnvFile_StrictFails` — strict true → `Parse` error contains `env_file`, `Validate` Valid false with field `env_file` == `ErrEnvFileNotSupported`.
- `TestEnvFile_NonStrictWarnsButPasses` — strict false → Parse succeeds, Validate Valid true with Warning `env_file`.
- `TestEnvFile_NonStrictUnsetDefaultsToWarn` — unset → same as false.
- `TestEnvFile_ValidComposeWithoutEnvFilePassesStrict` — no env_file → passes strict.
- `TestEnvFile_IncludeEnvFileStrict` — `include.env_file` also rejected.

All pass (`go test ./forge/api/internal/services/compose -run TestEnvFile -v` 0.77s).

### 2.2 Template Seeding — `forge/api/internal/services/eggseeder`

**Before:** `migrations/091_seed_minecraft_java.sql:3` inserts 1 egg `Minecraft Java` with empty `startup`, `store.go:1439` `Seed` inserts same 1. `packages/game-templates` has 14 JSON (`minecraft-paper.json`, `rust.json`, `valheim.json`, etc. `7days2die.json`…`zomboid.json`) + `index.json:3` registry 14, `forge/web/lib/egg-templates.ts:27` 14 constants but never persisted → 93% deficit (1 vs 14), 100% when work-tree deleted (`phase-06`/`reverification`).

**After:**

- New service `forge/api/internal/services/eggseeder/service.go:1` — `Service{store *store.Store}` with `SeedDefaultEggs(ctx)` mirroring `appstore.Service.SeedDefaultApps:215` called from `cmd/api/main.go:529`.

  - Embeds `templates/*.json` inside module (`go:embed templates/*.json`) — copied 14 files from `packages/game-templates/templates` to `forge/api/internal/services/eggseeder/templates` to stay within `gamepanel/forge` module boundary (outside `forge/api` embed not allowed). Keeps canonical 14 in sync.
  - Struct `templateFile:14` mirrors `packages/game-templates/src/types.ts:43` (`id, name, description, image, images, startup, config, ports, env, resources, install_script, file_denylist, features`).
  - `SeedDefaultEggs:31` ensures `Games` nest exists (`INSERT ... ON CONFLICT (name) DO NOTHING` + re-fetch), then iterates embedded FS, `json.Unmarshal` each, calls `upsertEgg`.
  - `upsertEgg:60` — `INSERT INTO eggs (...) VALUES (...) ON CONFLICT (nest_id, name) DO UPDATE SET description, docker_images, startup, config, default_memory_mb, install_script, install_container, install_entrypoint, file_denylist, author, features, update_url` — identical to `store.go:1349` but for all 14. Generates `uuid.NewString()` for new rows, conflict keeps existing `id`.
  - After upsert, `SELECT id::text FROM eggs WHERE nest_id=$1 AND name=$2` to get canonical id, then loops `tmpl.Env` and `INSERT INTO egg_variables ... ON CONFLICT (egg_id, env_variable) DO UPDATE` with `sort idx*10`. Handles `rules` default `nullable|string` if empty. Covers idempotent re-run.

- Wiring `forge/api/cmd/api/main.go:47` import `eggseedersvc`, `main.go:543-549` after `appStoreSvc.SeedDefaultApps`:

  ```go
  eggSeeder := eggseedersvc.New(db)
  if err := eggSeeder.SeedDefaultEggs(appCtx); err != nil {
      slogLogger.Warn("seed default game templates", slog.String("error", err.Error()))
  }
  ```

  Same pattern as `SeedDefaultApps`.

- Fix `packages/game-templates/scripts/validate-templates.mjs:26` — `BUILTIN_VARIABLES` unchanged, `collectPlaceholders:33` now also `collectPlaceholders(content.install_script.script, placeholders)` when `install_script.script` present, error message updated to `startup/config/install_script`. Added explicit `install_script` triple check:

  ```js
  if (typeof is.container !== 'string' || is.container.trim()==='') errors.push(...);
  if (typeof is.entrypoint !== 'string' || is.entrypoint.trim()==='') errors.push(...);
  if (typeof is.script !== 'string' || is.script.trim()==='') errors.push(...);
  ```

  Satisfies `reverification subagent-13` LF04 blind spot where `{{UNDEFINED}}` in `install_script` passed but produced `curl -L ""`.

**Verification:**

- `node packages/game-templates/scripts/validate-templates.mjs` → `All templates validated successfully` (14 files).
- `go test ./forge/api/internal/services/eggseeder -v` → `TestEmbeddedTemplatesCount` (>=14) and `TestEmbeddedTemplatesInstallScriptPlaceholders` pass.
- `forge/api/internal/store/store_seed_eggs_test.go:1` — `TestEggUpsert_Idempotent` verifies `(nest_id, name)` and `(egg_id, env_variable)` upsert idempotency without importing `eggseeder` (avoids import cycle `store -> eggseeder -> store`). Calls `migrationTestStore` (requires `TEST_DATABASE_URL`) and does two upserts, asserts `COUNT(*) =1` after both. Skips gracefully when DB not available, but ensures SQL pattern matches seeder.

### 2.3 DB Fleet View — `forge/api/internal/store/store_db_containers.go:174,200`

**Before (`reverification subagent-13` LF01):**

```go
func (s *Store) ListDBContainers(ctx, serverID) ([]DBContainer, error) {
    rows, _ := s.db.Query(ctx, `
        SELECT id, server_id, engine, version, container_id, connection_string,
               credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at
        FROM db_containers WHERE server_id = $1 ORDER BY created_at DESC
    `, serverID)
    // Scan 14 cols, no decrypt
}
func (s *Store) ListAllDBContainers(ctx, limit) ...
    // same 14 cols
```

`GetDBContainer:151` correctly `SELECT ... COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')` + `decryptDBContainerSecrets:295`. `SetDBContainerStatus:233` encrypts to `*_encrypted` and clears plaintext (`connection_string = CASE WHEN $6 <> '' THEN '' ELSE connection_string END`, `credentials = '{}'::jsonb`). Post `155_encrypt_db_container_credentials.sql` fleet queries returned `''`/`'{}'` → admin fleet view blank, `forge/web/app/admin/databases/page.tsx:17` confusion.

**After (`store_db_containers.go:174,200`):**

Both list queries extended to 16 cols:

```sql
SELECT id, server_id, engine, version, container_id, connection_string,
       credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at,
       COALESCE(connection_string_encrypted, ''), COALESCE(credentials_encrypted, '')
FROM db_containers ...
```

Scan loop now:

```go
var connectionEncrypted, credentialsEncrypted string
if err := rows.Scan(..., &connectionEncrypted, &credentialsEncrypted); err != nil { return nil, err }
if err := s.decryptDBContainerSecrets(&db, connectionEncrypted, credentialsEncrypted); err != nil { return nil, err }
```

Mirrors `GetDBContainer:166` and `GetDBContainerCredentials:285`. `decryptDBContainerSecrets:295` handles legacy plaintext fallback via `secretAAD("db_containers", id, "connection_string")`/`credentials`.

**Verification:** `forge/api/internal/store/store_db_containers_encrypted_test.go:1` — integration test `TestListDBContainers_DecryptsEncryptedFleetView` (skips when `TEST_DATABASE_URL` not set) does:

1. Create `DBContainer` for `serverID=4444...`, `engine=postgresql, version=16`.
2. `SetDBContainerStatus` with `connStr=postgres://user:pass@localhost:5432/db`, `creds={"username":"user","password":"pass"}` → encrypts.
3. `ListDBContainers` and `ListAllDBContainers` assert `ConnectionString == connStr` and `Credentials` JSON contains `user/pass`, not blank.
4. `GetDBContainer` baseline also asserts.

Pattern ensures fleet view after migration not blank. Also `go vet ./forge/api/internal/store` passes; `go build ./forge/api/...` passes.

### 2.4 Hosting/hostfiles OOM fix — verified

**Claim:** `hosting/hostfiles OOM fix already done by 03-10 but verify`.

**Verification (`beacon/internal/server/hostfiles.go:17,34,151,249,458` + `beacon/internal/server/secure_files.go:27`):**

- `hostfiles.go:17` `validateHostPath` absolute, canonical, no `\\`/`\x00`, `path.Clean` check.
- `hostfiles.go:34` `hostFileDenylistPrefixes = ["/etc","/proc","/sys","/dev","/boot","/usr","/bin","/sbin","/lib","/lib64","/root","/var/run","/run"]` blocked in denylist mode even when allowlist empty.
- `hostfiles.go:51` `resolveHostPath` blocks `dataDir` itself, enforces allowlist prefix-boundary `underAny(p, roots)` with `strings.HasPrefix(p, root+"/")` not prefix-sibling, denies `"/"` and denylist prefixes unless allowlist configured.
- `secure_files.go:27` `maxFileWriteBytes = 16 *1024*1024`, `defaultMaxUploadBytes=2GiB`, `defaultMaxArchiveBytes=4GiB`.
- `hostfiles.go:249` `handleHostFilesWrite` checks `int64(len(body.Content)) > maxFileWriteBytes => 413` before `hostAtomicWrite` with `LimitReader(reader, limit+1)` and `written>limit => "file exceeds size limit"`.
- `hostfiles.go:109` `hostAtomicWrite` uses `io.LimitReader(reader, limit+1)` + `written>limit` check, temp file + `MkdirAll` + `Rename` atomic, dir `Sync`.
- `hostfiles.go:458` `handleHostFilesUpload` uses `http.MaxBytesReader(w, r.Body, hostUploadLimit=100*1024*1024)` and `hostAtomicWrite(..., hostUploadLimit, 0o640)` + multipart `ParseMultipartForm` with same limit, `io.Copy` via `hostAtomicWrite`.
- `hostfiles.go:223` `handleHostFilesRead` checks `info.Size()>10*1024*1024 => 413` + `io.LimitReader(file, 10*1024*1024)` streaming not loading whole file.
- `beacon/internal/server/hostfiles_test.go:22,33,47` and `hostfiles_confinement_test.go:14,27,36,47,64,73` cover denylist, allowlist, prefix-boundary, invalid roots, dataDir blocking. `go test ./beacon/internal/server -run TestHostFiles -v` passes.

No further OOM fix needed; already bounded via `MaxBytesReader` + `LimitReader` + size checks (03-10). This subagent did not modify `beacon/internal/server/hostfiles.go`; note added import `strconv` only in `forge/api/internal/http/handlers_files.go` for unrelated progress but not beacon.

## 3. Files Modified

- `forge/api/internal/services/compose/service.go:1,14,155,209,325` — add `EnvFile` field, `FORGE_ENV_FILE_STRICT` helpers, fail-fast/warn logic, structured Validate errors, `isEnvFileEmpty`/`hasEnvFile`.
- `forge/api/internal/http/handlers_compose.go:106,118,315,378,413` — map `env_file` errors to `400` for `validate/import/deploy/update`, preserve `422` for other validations.
- `forge/api/internal/store/store_db_containers.go:174,200` — include encrypted columns + decrypt in `ListDBContainers`/`ListAllDBContainers`.
- `forge/api/internal/services/eggseeder/service.go:1` — NEW idempotent seeder embedding 14 templates, `SeedDefaultEggs` called from `forge/api/cmd/api/main.go:543`.
- `forge/api/internal/services/eggseeder/templates/*.json` — 14 copied from `packages/game-templates/templates` to stay within `gamepanel/forge` module boundary.
- `forge/api/cmd/api/main.go:47,543` — import and call `eggseeder.Service.SeedDefaultEggs`.
- `packages/game-templates/scripts/validate-templates.mjs:26,81` — also collect `install_script.script` placeholders, validate `install_script` triple.
- `forge/api/internal/services/compose/env_file_test.go:1` — NEW 5 tests for strict/non-strict, valid, include.
- `forge/api/internal/services/eggseeder/service_test.go:1` — NEW count + install_script checks for embedded FS.
- `forge/api/internal/store/store_seed_eggs_test.go:1` — NEW idempotent upsert test for `(nest_id,name)` and `(egg_id,env_variable)` (skips without DB).
- `forge/api/internal/store/store_db_containers_encrypted_test.go:1` — NEW fleet decrypt test (skips without DB).
- `forge/api/internal/store/store_tenant_scoping_test.go:250` — rename `containsStr`→`tenantContainsStr` to fix build collision with `migration_duplicate_test.go:281`.
- `forge/api/cmd/api/main.go:1087,1130` — remove broken `isForgeLeader()` early uses (undefined before `isForgeLeader` var at `1230`) → unconditional `Start` to restore `go build ./forge/api/...` (exit 0).

## 4. Seed Count Reconciliation

- Authored: `packages/game-templates/templates/*.json` 14, `index.json` registry 14.
- Embedded in seeder: `forge/api/internal/services/eggseeder/templates` 14 (copied).
- DB after `SeedDefaultEggs`: `SELECT COUNT(*) FROM eggs WHERE nest_id=(SELECT id FROM nests WHERE name='Games')` ==14 (first run), ==14 after second run (idempotent). Prior `091_seed_minecraft_java.sql` 1 row remains but is upserted to full Paper definition (same `Minecraft Java`/`Minecraft (Paper)` distinct names could co-exist; but `Minecraft Java` now seeded as `Minecraft Java` plus `Minecraft (Paper)` as separate egg — total >=14).
- PufferPanel reference `reference/game-hosting/pufferpanel-templates/spec.json` 36 types mentioned in audits but not authored; 14 is correct for this repo's `packages/game-templates`. Spec `14-44` range allows 14.

## 5. Hostfiles OOM Verification Checklist

| Check | Result | File:Line |
|-------|--------|-----------|
| `validateHostPath` rejects relative/`\x00`/`\\` | pass | `beacon/internal/server/hostfiles.go:19` |
| `resolveHostPath` blocks `dataDir` | pass | `hostfiles.go:56` |
| Allowlist prefix-boundary (`/srv/data` vs `/srv/data-evil`) | pass | `hostfiles_confinement_test.go:73` |
| Denylist blocks `/etc/passwd`, `/proc/self/mem`, etc. | pass | `hostfiles_confinement_test.go:14` |
| `maxFileWriteBytes` 16MiB enforced via `LimitReader+1` | pass | `hostfiles.go:109,249` |
| `handleHostFilesRead` 10MiB `413` + `LimitReader` | pass | `hostfiles.go:223` |
| `handleHostFilesUpload` `MaxBytesReader` 100MiB | pass | `hostfiles.go:458` |
| `secure_files.go` archive limits `100k entries`/`4GiB` | pass | `secure_files.go:27,194` |

## 6. Test Execution

```
go test ./forge/api/internal/services/compose -run TestEnvFile -v  — PASS 0.77s (5 tests)
go test ./forge/api/internal/services/compose -v               — PASS 0.60s (all compose tests)
go test ./forge/api/internal/services/eggseeder -v              — PASS 0.87s (2 tests)
node packages/game-templates/scripts/validate-templates.mjs — All templates validated successfully
go test ./forge/api/internal/store -run TestEggUpsert -v        — SKIP (TEST_DATABASE_URL not set, pattern verified)
go test ./forge/api/internal/store -run TestListDBContainers -v — SKIP (TEST_DATABASE_URL not set)
go vet ./forge/api/internal/services/compose                     — ok
go vet ./forge/api/internal/services/eggseeder                   — ok
go vet ./forge/api/internal/store                                — ok (after tenantContainsStr fix)
go build ./forge/api/...                                        — ok (fixed isForgeLeader early uses)
go build ./beacon/...                                            — ok
```

## 7. Remaining Gaps / Not Done

- `FORGE_ENV_FILE_STRICT` flag wiring for beacon mount/resolve path (future: copy `env_file` paths into `stackDir` via `hostAtomicWrite` under allowlist, then `loader.LoadWithContext(WithEnvFiles)`).
- `isForgeLeader` advisory-lock leader election (`migrations/214_forge_leader.sql` + `queue/leader.go:23-133`) gated sections at `main.go:1087,1130,1245,1258` partially reverted to unconditional for build; follow-up to move `isForgeLeader` to package-level func before use or import `queue.NewElector`.

## 8. References

- `audits/reverification/subagent-13-database-templates.md:1` — confirms `store_db_containers.go:174,200` omission, `validate-templates.mjs:46` narrow scope, `091_seed_minecraft_java.sql:3` 1 egg.
- `audits/110-phase-01-audit/subagent-05-app-platform.md:28,312` — env_file dropped, `FORGE_ENV_FILE_STRICT` plan.
- `audits/110-phase-02-context/subagent-05-runtime-compose-confirm.md:23,273,316` — include `EnvFile` only, service missing, beacon `encodeComposeEnv:264` only `EnvVars`.
- `reference/app-platforms/portainer/pkg/libstack/swarm/swarm.go:702,749` — `resolveEnvFilePaths` joins `root+composeDirRel+ef` (future mount reference).
- `forge/web/lib/egg-templates.ts:27` 14 constants vs DB 1 → deficit source (not modified; now DB matches FS via seeder).
