# Subagent 07 — Database & Template Catalog Confirmation

**Phase:** 110-02-07 of 110 (Phase 02, Agent 07/10, parallel run)
**Focus:** Confirm Database & Template Catalog implementations
**Date:** 2026-08-24
**Scope:** Re-verify against live worktree `forge/api`, `forge/web`, `packages/game-templates`, `forge/api/migrations` at `HEAD` + working tree.
**Prior audits compared:**
- `audits/phase-06/subagent-03-1panel-database-runtime.md` — 18 comparisons + 6 LFs
- `audits/phase-06/subagent-06-templates-catalog.md` — 16 comparisons + 4 Ls
- `audits/final-parity/subagent-06-database-templates.md` — 23 rows (DB01-DB14 + RT01-RT03 + TPL01-TPL10, collapsed to 23 in synthesis header) + 6 LFs

**Method:** Read-only. Every row re-inspected at `file:line` in live tree. Verdict per finding: **BROKEN** (still present), **FIXED** (remediated), **INTENTIONAL** (architectural divergence documented as non-goal), **PARTIAL-FIXED**.

---

## 1. Direct Live Checks Requested in Task

| Check | Live file:line | Finding | Verdict |
|-------|---------------|---------|---------|
| `store_db_containers.go:174` `ListDBContainers` omits encrypted cols | `forge/api/internal/store/store_db_containers.go:174` `SELECT ... credentials, status ...` (14 cols) vs `forge/api/internal/store/store_db_containers.go:151` `GetDBContainer` `SELECT ... COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')` (16 cols) | List path never selects `connection_string_encrypted`/`credentials_encrypted`; `decryptDBContainerSecrets:295` receives `""` in list loops, so fleet view blank post-`155_encrypt_db_container_credentials.sql` | **BROKEN** — unchanged from LF01. Same code in `:200` `ListAllDBContainers` (identical SELECT). |
| `store_db_containers.go:200` `ListAllDBContainers` omits encrypted cols | `forge/api/internal/store/store_db_containers.go:200` same 14-col SELECT; `forge/api/internal/store/store_db_containers.go:233` `SetDBContainerStatus` correctly writes encrypted cols (`COALESCE(NULLIF($6,''), connection_string_encrypted)`) so divergence is read-side only | `forge/web/app/admin/databases/page.tsx:17` containers tab calls `ListAllDBContainers` — will show empty `connectionString`/`credentials` for encrypted rows; single `GetDBContainerCredentials:275` still works | **BROKEN** |
| `dbprovisioner/service.go:373` `mysql.RegisterTLSConfig` global leak | `forge/api/internal/services/dbprovisioner/service.go:373` `cfg := mysql.NewConfig()` + `forge/api/internal/services/dbprovisioner/service.go:378` `mysql.RegisterTLSConfig(name, tlsConfig)` with `mysqlTLSConfigName:417` `sha256(host.ID+host+mode+CA)[:12]` | Global `map[string]*tls.Config` mutated per host, never unregistered, `already registered` check at `:378` races; rotation creates new name, old CA pin leaks; memory unbounded | **BROKEN** — no `sync.Mutex`/`sync.Once`/`UnregisterTLSConfig` added |
| `containers.go:289` deprovision swallows error | `forge/api/internal/services/dbprovisioner/containers.go:289` `Deprovision` ` _ = s.daemon.DeProvisionDatabase(...)` then unconditional `return s.store.DeleteDBContainer` | Orphan container/volume leak on daemon failure; inconsistent with `forge/api/internal/services/database_service_provisioner.go:259` `DeleteService` which propagates error and sets `failed` | **BROKEN** |
| `containers.go:308` (task cites `:289` but `Restart:300` is companion) `Restart` is status-only no-op | `forge/api/internal/services/dbprovisioner/containers.go:300` `Restart` `return s.store.SetDBContainerStatus(ctx, containerID, "", "running", 0, "", "", nil)` without `daemon.AdminContainerStart/Stop` | `forge/api/internal/http/handlers_db_containers.go:127` `POST /databases/containers/:id/restart` always succeeds even if container stopped/failed | **BROKEN** |
| `store_managed_databases.go:504` status race | `forge/api/internal/store/store_managed_databases.go:504` `DeleteManagedDatabaseWithForce` + `forge/api/internal/http/handlers_managed_databases.go:166` `_ = cfg.Store.UpdateManagedDatabaseStatus(ctx, id, deleting)` **before** `DeleteManagedDatabaseWithForce` | If `hasVolume && !force` at `store_managed_databases.go:526` returns `deletion protection: use force`, row already mutated to `deleting` (via `UpdateManagedDatabaseStatus:422`) and stuck forever; legacy fallback at `store_managed_databases.go:143,222,326,427,508` string-sniffs `column "deleted_at" does not exist` | **BROKEN** |
| `packages/game-templates/templates/*.json` on disk (0 files now?) | `packages/game-templates` — `ls` fails `No such file or directory` on working tree; `git ls-files \| grep game-templates` shows 14 files in `HEAD` (`packages/game-templates/templates/*.json:1` 14 entries: `7days2die`, `csgo`, `enshrouded`, `factorio`, `minecraft-bedrock`, `minecraft-paper`, `minecraft-vanilla`, `palworld`, `rust`, `satisfactory`, `teamspeak3`, `terraria`, `valheim`, `zomboid`); `git status` shows `D packages/game-templates/*` (unstaged deletion) | Working checkout ships **0** FS templates; `HEAD` ships 14; either way DB seeding sees 0 | **BROKEN** (phantom catalog) |
| `egg-templates.ts:27` 14 vs `091_seed` 1 egg | `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries (mirrors `packages/game-templates` IDs: `minecraft-paper:29`, `minecraft-vanilla:91`, `palworld:145`, `valheim:177`, `terraria:208`, `enshrouded:257`, `satisfactory:288`, `rust:319`, `csgo:355`, `factorio:390`, `7days2die:423`, `minecraft-bedrock:456`, `teamspeak3:498`, `zomboid:531`) vs `forge/api/migrations/091_seed_minecraft_java.sql:3` `INSERT INTO eggs ... 'Minecraft Java' ... ON CONFLICT DO NOTHING` (1 row) + 2 `egg_variables` `VERSION/TYPE` | `forge/api/internal/store/store_nests.go:1` + `store_templates.go:1` `ListEggs`/`ListTemplates` on fresh migrate returns 1; frontend wizard never materializes `EGG_TEMPLATES` to DB | **BROKEN** — seeding gap unchanged |
| `scripts/validate-templates.mjs:46` blind | `packages/game-templates/scripts/validate-templates.mjs:46` (in `HEAD`: `collectPlaceholders(content.startup, placeholders)` + `content.config.files` only, lines 46-54) — `install_script` never collected | Variable `{{UNDEFINED}}` inside `install_script` (most dangerous place — `minecraft-paper.json:44` `sed -e 's/{{/${/g'` and `minecraft-vanilla.json:105` same) passes validation but produces `curl -L ""` at deploy; plus file deleted on disk so `prebuild: node scripts/validate-templates.mjs` cannot run on working checkout at all | **BROKEN** |

---

## 2. Phase-06 Subagent-03 — 18 Comparisons Confirmation

Source: `audits/phase-06/subagent-03-1panel-database-runtime.md`. All file:lines re-checked live.

| # | Axis | Reference file:line | Forge live file:line | Prior status | Live verdict | Evidence |
|---|------|---------------------|----------------------|--------------|--------------|----------|
| C01 | Engine coverage & version gating | `reference/app-platforms/1panel/agent/constant/app.go:13` `AppMysql` + `reference/app-platforms/1panel/agent/app/service/database_common.go:52` cluster branches | `forge/api/internal/store/store_db_containers.go:14` `SupportedDBEngines` (5 engines pinned) + `forge/api/internal/store/store_db_containers.go:22` `DBEngineImages` + `forge/api/internal/store/store_db_containers.go:30` `DBEngineDefaultPorts` + `forge/api/internal/services/database_service_provisioner.go:95` `imageForDB` + `forge/api/internal/services/dbprovisioner/containers.go:67` `imageForDB` + `forge/api/migrations/175_catalog_entries.sql:23` 11 catalog entries | Partial | **BROKEN (unchanged)** — no `mysql-cluster`/`postgresql-cluster`/`redis-cluster` topology; no `memcached` as DB engine (catalog has `memcached` but not via `store_db_containers`); hardcoded version map requires triple change (store + `114_database_service_plugins.sql:69` seed + both `imageForDB` maps); `imageForDB` duplicated across 3 files | `store_db_containers.go:14` still sole source but not consumed by provisioners' local `imageForDB` maps |
| C02 | MySQL user model (multi-host, password rotation) | `reference/app-platforms/1panel/agent/app/api/v2/database_mysql.go:73` `CreateMysqlUser` + `:52` `ListUsers` + `:123` `UpdateMysqlUser` + `reference/app-platforms/1panel/agent/app/model/database_user.go:3` composite `idx_database_user` | `forge/api/internal/services/dbprovisioner/service.go:295` `CreateUser` `CREATE USER 'u'@'%'` + `forge/api/internal/store/store_database_services.go:44` `DatabaseServiceCredential` (no host column) + `forge/api/internal/services/database_service_provisioner.go:295` same | Missing | **BROKEN** — `store_database_services.go:44` still `{username, encrypted_password, database_name, permissions}` no `host`; no `ListUsers`/`DeleteUser`/`UpdateUser`; `splitMysqlHosts:86` equivalent absent; `loadMysqlPasswordAppTargets:168` app-env propagation absent | `grep -n "host.*TEXT" store_database_services.go` returns none |
| C03 | MySQL grant / privilege lattice | `reference/app-platforms/1panel/agent/app/api/v2/database_mysql.go:172` `ListMysqlGrants` + `:193` `GrantSummary` + `:236` `RevokeMysqlGrant` blocked for `*` | `forge/api/internal/services/dbprovisioner/service.go:332` `GrantPermissions` binary `ALL` vs `SELECT` + `forge/api/internal/store/store_database_services.go:333` `RevokeServiceCredential` soft `revoked_at` | Partial (create-only) | **BROKEN** — no `ListGrants`/`RevokeGrant` handler; `RevokeServiceCredential` never executes `REVOKE`/`DROP USER` on engine; binary permission collapses finer lattice | `handlers_database_services.go` has no grant routes |
| C04 | MySQL operational surface (status/vars/remote/format) | `reference/app-platforms/1panel/agent/app/api/v2/database_mysql.go:379` `LoadFormatOption` + `:509` `LoadVariables` + `:486` `LoadStatus` + `reference/app-platforms/1panel/agent/app/service/database_common.go:84` `UpdateDBConfByFile` | `forge/api/internal/services/database_service_provisioner.go:477` `TestConnection` (`SELECT 1`) only + `forge/api/internal/store/store_databases.go:224` host CRUD | Missing | **BROKEN** — no `SHOW GLOBAL VARIABLES/STATUS`, no `my.cnf` edit, no `LoadRemoteAccess`/`LoadFormatOption`; `adminConn` pool at `dbprovisioner/service.go:522` exists but not wired for read-only vars | No `SHOW` queries in `forge/api` beyond provision conn test |
| C05 | PostgreSQL surface (bind/superuser/password) | `reference/app-platforms/1panel/agent/app/api/v2/database_postgresql.go:52` `BindPostgresqlUser` + `:96` `ChangePostgresqlPrivileges` + `reference/app-platforms/1panel/agent/app/service/database_postgresql.go:89` `SuperUser` | `forge/api/internal/services/dbprovisioner/service.go:550` `provisionPostgreSQL` `CREATE ROLE LOGIN` (hardcoded) + `forge/api/internal/services/database_service_provisioner.go:285` `postgresql` branch | Partial | **BROKEN** — no `BindUser` idempotent upsert, no `superUser` flag, no `updateInstallInfoInDB:422` app-env propagation, no `pg_roles` existence guard in `database_service_provisioner` path | `grep -n "SuperUser\|super_user" forge/api` returns none |
| C06 | MongoDB surface (roles/bind/privilege) | `reference/app-platforms/1panel/agent/app/api/v2/database_mongodb.go:21` `isSupportedMongodbPrivilege:910` (`dbOwner/read/readWrite/userAdmin`) + `:121` `BindMongodbUser` | `forge/api/internal/services/dbprovisioner/service.go:675` `provisionMongoDB` fixed `readWrite` + `:693` deprovision + `:708` rotate | Partial | **BROKEN** — fixed `readWrite` role; no `BindUser`, no privilege selection, no `LoadPrivileges`/`ChangePrivileges`; `dbbackup/engines.go:86` only `mongodump --archive` | `grep -n "dbOwner\|userAdmin" forge/api` returns none |
| C07 | Redis surface (status/conf/persistence/CLI) | `reference/app-platforms/1panel/agent/app/api/v2/database_redis.go:19` `LoadStatus:132` parses `INFO` + `:42` `UpdateConf` + `reference/app-platforms/1panel/agent/app/service/database_redis.go:295` `redisExec` | `forge/api/internal/services/dbprovisioner/service.go:486` `provisionRedis` `CONFIG SET requirepass` + `CONFIG SET appendonly yes` + best-effort `CONFIG REWRITE:498` + `forge/api/internal/services/database_service_provisioner.go:370` dead `quoteSQLString` redis branch + `:132` `REDIS_PASSWORD` env | Partial | **BROKEN** — no `INFO`/`CONFIG GET` handlers, no persistence editor, no helper container; `database_service_provisioner.go:370` `CONFIG SET requirepass '+quoteSQLString` is dead-but-wrong (would corrupt Redis protocol if `adminConn` ever supported redis); both provisioners set non-standard `REDIS_PASSWORD` env that `redis:7` image ignores | `go-redis` `ConfigGet` imported at `service.go:20` but never exposed via HTTP |
| C08 | Language runtimes (PHP/Node/Java/Go/Python/.NET) | `reference/app-platforms/1panel/agent/app/model/runtime.go:9` `Runtime` + `reference/app-platforms/1panel/agent/app/service/runtime.go:127` `Create` + `:379` `Delete` + `constant/runtime.go:7` `RuntimePHP\|RuntimeNode\|...` | `forge/api/internal/services/runtime/runtime.go:31` game-server `Runtime` (Start/Stop/Stats/Logs) + `forge/api/internal/runtime/` adapters (docker/podman/k8s) | Missing (architectural) | **INTENTIONAL (still missing, by design)** — Forge `runtime` is game-server container infra (`gpruntime.CreateServer` at `services/clustermanager/service.go:65`), not language runtime; `forge/web/app/admin/databases/page.tsx:7` has no runtime page; `compose_projects`/`builds` (`098_app_platform_foundations.sql:72` `builder_type dockerfile/nixpacks`) is the different primitive | Documented divergence; no shim attempted — correct to keep as intentional gap |
| C09 | PHP extensions | `reference/app-platforms/1panel/agent/app/model/php_extensions.go:3` + `reference/app-platforms/1panel/agent/app/service/runtime.go:931` `InstallPHPExtension` (`docker commit`) | No file in `forge/api` or `forge/web` (grep returns none); `store_app_store.go` has no extension primitive | Missing | **INTENTIONAL (still missing)** — no `docker commit` path; `compose_projects` would be used instead | Correctly not ported |
| C10 | PHP config / FPM / supervisor / Node modules | `reference/app-platforms/1panel/agent/app/service/runtime.go:1040` `GetPHPConfig` + `:1206` `UpdateFPMConfig` + `:1371` `GetSupervisorProcess` + `:740` `GetNodePackageRunScript` | `forge/api/internal/http/handlers_database_services.go:116` `restart` + `forge/api/internal/http/handlers_db_containers.go:127` `restart` (status poke only) | Missing | **INTENTIONAL (still missing)** — inherited from C08/C09; large FPM/supervisor/node_modules surface has no Forge analogue | Correctly not ported |
| C11 | Remote database host model | `reference/app-platforms/1panel/agent/app/model/database.go:3` `Database` (union host+creds + `AppInstallID`) | `forge/api/internal/store/store_databases.go:17` `DatabaseHost` + `014_server_databases.sql:1` `database_hosts` + `087_a_parity_schema.sql:30` `database_host_node` pivot + `116_managed_databases.sql:1` `managed_databases` + `098_app_platform_foundations.sql:50` `db_containers` + `114_database_service_plugins.sql:5` `database_services` | Partial (Forge stronger TLS/nodes, fragmented) | **BROKEN (unchanged)** — 3 primitives (`server_databases` host-provisioned, `managed_databases+db_containers` container-provisioned, `database_services` legacy beacon) require caller to choose; no `From=local` equivalent; `managed_databases.server_id` FK added via `ALTER` DO-block `116:59` may be absent; `store_managed_databases.go:136,215` legacy fallbacks mask drift | Fragmentation not consolidated |
| C12 | Database lifecycle (create/sync/delete guard) | `reference/app-platforms/1panel/agent/app/service/database_mysql.go:677` `LoadFromRemote` + `:738` `DeleteCheck` (website/app `appInstallResourceRepo`) + `:791` `Delete` | `forge/api/internal/services/dbprovisioner/service.go:101` `Provision` (`pending→ready` + `SetServerDatabaseProvisioningState:118`) + `forge/api/internal/store/store_managed_databases.go:504` `DeleteManagedDatabaseWithForce` (`hasVolume && !force → 409`) + `forge/api/internal/store/store_db_containers.go:270` hard delete | Partial | **BROKEN** — `ServerDatabase` delete has no `DeleteCheck` (can delete DB still referenced by compose/website); `DBContainer.Restart:308` no-op; `DeleteManagedDatabaseWithForce` status race (see LF06) | `handlers_managed_databases.go:166` `deleting` before check still present |
| C13 | Configuration file editing (`my.cnf`/`postgresql.conf`/`redis.conf`) | `reference/app-platforms/1panel/agent/app/api/v2/database_common.go:40` `LoadDBFile` + `:62` `UpdateDBConfByFile` per `req.Type` (`postgresql-conf v18:66`) + `service/database_common.go:52,90` `..` traversal guard + `compose.Restart:111` | `forge/api/internal/services/dbbackup/service.go:455` `stageCredentialFile` (transient cred files only); `forge/api/internal/services/dbprovisioner/containers.go:105` `envVarsForDB` + `store_managed_databases.go:470` `UpdateManagedDatabase` (name/memory/cpu/version text only) | Missing (by design — immutable containers) | **INTENTIONAL (still missing)** — containers immutable; `postgresql.conf` `data/18/docker/postgresql.conf` path not exposed; `UpdateManagedDatabasePort:450` only port mutation | Document as intentional |
| C14 | Connection details, credential handling & encryption | `reference/app-platforms/1panel/agent/app/service/database_mysql.go:857` `encrypt.StringEncrypt` per-row (no AAD) | `forge/api/internal/store/store_db_containers.go:14` + `:158` `secretAAD("db_containers", id, ...):237` + `forge/api/internal/services/database_service_provisioner.go:217` `secrets.Keyring` + `store_database_services.go:12` `encrypted_password` | Forge stronger (AAD-bound) | **PARTIAL-FIXED on security, BROKEN on list** — encryption at rest is strictly stronger than 1Panel, but `ListDBContainers:174`/`ListAllDBContainers:200` never decrypt (see LF01), so fleet view broke while single `GetDBContainer:151` works | Security win intact; read-path regression still present |
| C15 | Backup / restore engine matrix | `reference/app-platforms/1panel/agent/app/service/backup_mysql.go` etc. (`mysqldump`/`pg_dump` via `utils/mysql/client`) | `forge/api/internal/services/dbbackup/service.go:65` `Backup`/`HandleQueuedBackup:115`/`runBackup:153` + `engines.go:16` `backupCommandForEngine` (`pg_dump -F c`/`pg_restore -c`, `mysqldump --single-transaction`, `mongodump --archive`, `redis-cli --rdb`) + `service.go:567` `EngineDumpCommands` duplicated | Partial (Forge better queue/checksum/storage) | **BROKEN (unchanged)** — `dbbackup/engines.go:16` vs `dbbackup/service.go:567` mapping duplicated (drift risk); `handlers_db_containers.go:115` `Backup:311` fires `BackupDatabase` without result tracking; `provisionMySQL` error paths not connected to backup lifecycle | Duplication still present |
| C16 | Observability & day-2 ops (logs, restart, status) | `reference/app-platforms/1panel/agent/app/service/database_mysql.go:982` `LoadStatus` + `reference/app-platforms/1panel/agent/app/service/runtime.go:1409` `GetFPMStatus` (FastCGI) | `forge/api/internal/services/database_service_provisioner.go:453` `GetServiceLogs` (tail 50) + `forge/api/internal/store/store_managed_databases.go:73` enum `creating\|running\|error\|deleting` + `database_service_provisioner.go:477` `TestConnection` (`SELECT 1`) | Partial | **BROKEN (unchanged)** — `DBContainerService.Restart:300` no-op; `managed_databases` status is enum not live ping; no `SHOW GLOBAL STATUS` expansion, no `INFO` for redis, no FPM/supervisor | `handlers_database_services.go:169` still 50-line tail |
| C17 | TLS / SSL handling | `reference/app-platforms/1panel/agent/app/model/database.go:16` `SSL, RootCert, ClientKey, ClientCert, SkipVerify bool` | `forge/api/internal/store/store_databases.go:311` `tls_mode/tls_ca/tls_server_name` + `:594` `validateDatabaseHostRequest` (4 modes, x509 PEM) + `forge/api/internal/services/dbprovisioner/service.go:366` `connectorForHost:372` + `:422` `hostTLSConfig` | Forge stronger | **PARTIAL-FIXED on lattice, BROKEN on leak** — 4-mode lattice (`disable/required/verify-ca/verify-full`) strictly more expressive than 1Panel `SkipVerify bool`; but `mysql.RegisterTLSConfig` global leak (LF02) remains | Lattice correct; leak unfixed |
| C18 | Deleted-but-referenced integrity & migrations shims | `reference/app-platforms/1panel/agent/app/model/database_mysql.go:3` `IsDelete bool` + `WithoutByFrom("local")` filters | `forge/api/migrations/208_managed_database_deletion_protection.sql:2` `deleted_at` + `forge/api/internal/store/store_managed_databases.go:136` `WHERE deleted_at IS NULL` + `:175` `getManagedDatabaseLegacy` string-sniff + `handlers_db_containers.go:56` `"standalone"` default + `store_managed_databases.go:117` `NULLIF($2,'')` UUID | Partial (defensive shims, fragile) | **BROKEN (unchanged)** — string-matching `column "deleted_at" does not exist` at `store_managed_databases.go:143,222,326,427,440,454,465,481,533` is fragile; `SMOKE_TEST_REPORT:105` flagged `NULLIF($2,'')` and `"standalone"` still present at `store_managed_databases.go:117` and `handlers_db_containers.go:56` | Shims mask drift rather than fail migration |

**Subagent-03 LFs confirmation:**

| LF | Location | Verdict |
|----|----------|---------|
| LF01 `ListDBContainers` omits encrypted cols | `store_db_containers.go:174,200` | **BROKEN** — see §1 |
| LF02 Global `mysql.RegisterTLSConfig` leak/race | `dbprovisioner/service.go:373,378,417` | **BROKEN** — see §1 |
| LF03 Redis `quoteSQLString` in provisioner | `database_service_provisioner.go:370` `CONFIG SET requirepass '+quoteSQLString` + `:114` `REDIS_PASSWORD` env non-standard | **BROKEN** — dead-but-wrong branch unchanged; `containers.go:127` `REDIS_PASSWORD` also non-standard |
| LF04 `Deprovision` swallows error + `Restart` no-op | `containers.go:289,300` | **BROKEN** — see §1 |
| LF05 `generatePassword` divergence (root vs app) | `database_service_provisioner.go:63` `hex.EncodeToString(b)[:length]` with `make([]byte,length)` vs `containers.go:44` `(length+1)/2` + `:114` `rootPassword:=generatePassword(64)` distinct from `password` | **BROKEN** — divergence unchanged; `MYSQL_ROOT_PASSWORD` not persisted, `connectionString:140` embeds app password only, rotation never touches root |
| LF06 Managed DB soft-delete + status race + legacy fallback masking | `store_managed_databases.go:504` + `handlers_managed_databases.go:166` + `handlers_user_databases.go:146` + string-sniff at `store_managed_databases.go:143` etc. | **BROKEN** — see §1 |

---

## 3. Phase-06 Subagent-06 — 16 Comparisons Confirmation

Source: `audits/phase-06/subagent-06-templates-catalog.md`

| # | Axis | Puffer / 1Panel reference file:line | Forge live file:line | Prior status | Live verdict |
|---|------|--------------------------------------|----------------------|--------------|--------------|
| C1 | Template count & coverage | `reference/game-hosting/pufferpanel-templates/*.json:1` 42 files / 36 distinct types (e.g. `minecraft/minecraft.json:1`, `rust/rust.json:1`) | `packages/game-templates/templates/*.json:1` in `HEAD` 14 vs working tree 0 (`ls packages/game-templates → No such file`) + `forge/web/lib/egg-templates.ts:27` 14 + `forge/api/migrations/091_seed_minecraft_java.sql:3` 1 egg + `forge/api/migrations/007_postgres_core_foundation.sql:106` `Games` nest | Partial — 39% of Puffer, phantom on `main` | **BROKEN (worsened)** — prior audit saw 14 on branch, 0 on `main`; live now 0 on working tree (unstaged deletion) so even `main` cannot build `packages/game-templates`; `ListEggs` still 1 row |
| C2 | Schema draft & extensibility gate | `reference/game-hosting/pufferpanel-templates/spec.json:4` `additionalProperties:false` closed | `packages/game-templates/template-schema.json:3` draft-07 (in `HEAD`) + `forge/web/lib/egg-templates.ts` no validator | Partial (Forge semi-closed) | **BROKEN (unchanged)** — `template-schema.json` allows `images`/`config.files` open but `EGG_TEMPLATES` TS literals have no schema validation; `validate-templates.mjs` not runnable on working tree |
| C3 | Variable / environment model | `reference/game-hosting/pufferpanel-templates/spec.json:156` `$defs.variable` (`option\|string\|boolean\|integer`, `internal`, `options[]`, `groups:156`) + `minecraft/minecraft.json:74` groups `if: modlauncher=='forge'` + `:118` `internal:true` | `packages/game-templates/template-schema.json:69` `env: array<{env_variable, default_value: string, rules}>` + `forge/api/internal/store/store_egg_variables.go:10` `EggVariable` all-string + `forge/web/lib/egg-templates.ts:14` same | Partial (stringly-typed collapse) | **BROKEN** — no `option` type, no `internal`, no `groups`, no `display/desc` split; Forge splits Puffer `modlauncher` option into two templates (`minecraft-vanilla` vs `minecraft-paper`); `store_egg_variables.go:92` `validateVariableValue` delimiter bug (`regex:/.../` includes `/`) still present (phase-02 subagent-02 LF) |
| C4 | Install / provisioning fidelity (24 ops vs single shell) | `reference/game-hosting/pufferpanel-templates/spec.json:274` 24 declarative ops (`steamgamedl`, `paperdl`, `mojangdl`, `resolveforgeversion`, etc. `:282-300`) + `minecraft/minecraft.json:173` 16 conditional steps | `packages/game-templates/template-schema.json:114` `install_script:{container, entrypoint, script:string}` + `forge/api/internal/store/store_nests.go:20` `Egg{InstallScript...}` | Partial (lossy transliteration) | **BROKEN** — declarative→imperative loss unchanged; `javadl` host-only gate (`if env=='host'`) dropped; file-exists fallback on Forge shim naming (`minecraft/minecraft.json:219`) hardcoded; Steam `appId` literal `"258550"` no schema guard |
| C5 | Environment / runtime target (host+docker vs docker-only) | `reference/game-hosting/pufferpanel-templates/spec.json:26` `environment` + `supportedEnvironments` + `valheim/valheim.json:77` `portBindings: ["0.0.0.0:${port}:${port}/tcp"]` | `packages/game-templates/template-schema.json:24` `image + images Record<string,string>` + `supported_platforms:["docker","podman"]` (`:131`) | Partial (docker-only by design) | **INTENTIONAL (still docker-only)** — host mode dropped by design; Puffer `if env=='host' && file_exists(...)` has no Forge analogue; correctly not ported |
| C6 | Run / startup command model | `reference/game-hosting/pufferpanel-templates/spec.json:64` `run:{command string\|array<{command,if}>, pre:[], post:[], stop\|stopCode, stdin, stdout, environmentVars, autostart}` + `minecraft/minecraft.json:244` 9-entry `run.command` fallback + `rust/rust.json:59` `stdin:{rconws}` | `packages/game-templates/template-schema.json:34` `startup:string + config:{files, startup:{done}, stop, logs}` + `forge/api/internal/store/store_nests.go:20` `config jsonb` | Partial | **BROKEN** (by design) — no `pre/post`, no `stdin/stdout` (`rconws`/`telnet`/`file`), no `stopCode`, no `environmentVars` (`LD_LIBRARY_PATH` `rust/rust.json:56`), no `autostart/autorecover`; collapse to `{{VAR}}` substitution + `config.startup.done` regex | **INTENTIONAL** if wings `startup.done` is sufficient; otherwise gap |
| C7 | Port & network exposure | `reference/game-hosting/pufferpanel-templates/valheim/valheim.json:77` `portBindings` + `csgo/csgo.json:115` 3 bindings + `rust/rust.json:28` `port:27015+RConPort:28016` | `packages/game-templates/template-schema.json:47` `ports:[{port, protocol, public}]` + `forge/web/lib/egg-templates.ts` same (e.g. `valheim 2456/2457/2458 UDP`, `rust 28015/udp+28016/tcp`) | Forge more explicit | **FIXED relative to Puffer (Forge explicit typed ports)** — but `validate-templates.mjs:56` only checks range/protocol; `satisfactory.json` `BEACON_PORT` in `startup` has no matching `ports[]` entry (gap noted in L2) still present | **PARTIAL-FIXED** — explicit ports are better, but `ports[] ↔ startup` drift not linted |
| C8 | Resource contracts | `reference/game-hosting/pufferpanel-templates/spec.json:26` `requirements:{os, arch, binaries[]}` (no sizing) | `packages/game-templates/template-schema.json:95` `resources:{cpu, memory_mb, disk_mb}` required + `forge/api/internal/store/store_catalog.go` etc. | Forge explicit, orthogonal | **INTENTIONAL (still Forge explicit sizing)** — no change needed; Puffer `requirements` has no Forge counterpart and vice versa | No remediation needed |
| C9 | Validation pipeline | `reference/game-hosting/pufferpanel-templates/spec.json:1` draft 2020-12 strict + `packages/game-templates/scripts/validate-templates.mjs:1` (in `HEAD`) `REQUIRED:8`, `BUILTIN_VARIABLES(9)`, `collectPlaceholders:20` over `startup+config.files` | `packages/game-templates/scripts/validate-templates.mjs:46` narrow + `forge/web/lib/egg-templates.ts` no validator + `store_egg_variables.go:49` `validateEggVariableRequest` | Partial (narrow validator) | **BROKEN** — `validate-templates.mjs:46` still only checks `startup + config.files`, not `install_script`; `EGG_TEMPLATES` TS literals unverified; DB `store_nests.go:276` `normalizeDockerImages` validates at write but never checks `Egg.Startup` placeholders |
| C10 | Template naming / identity | Puffer `spec.json:12` `id?: string` optional, filesystem path identity | `packages/game-templates/template-schema.json:14` `id: ^[a-z0-9]+(-[a-z0-9]+)*$` + `validate-templates.mjs:65` `file === id.json` + `store_nests.go:116` UUID PK + `091_seed_minecraft_java.sql:6` hardcoded `91ec0000...` | Forge strict slug | **FIXED (Forge strict kebab)** — naming contract intact in `HEAD`; but file deleted on disk so contract not enforceable on working checkout | **PARTIAL-FIXED** |
| C11 | Seeding & bootstrap path | Puffer files on disk; `1panel/service/compose_template.go:58` `Batch` upserts | `forge/api/migrations/091_seed_minecraft_java.sql:3` 1 egg + `forge/api/internal/services/appstore/seed.go:27` 7 apps + `forge/web/lib/egg-templates.ts` 14 constants + `AdminNestsEggs.tsx:124` manual `importEgg` | Missing (1 egg seeded, no batch) | **BROKEN** — none of the 14 FS templates seeded to `eggs+egg_variables`; no `SeedDefaultApps`-style `UpsertEggTemplates`; `packages/game-templates` absent so `validate-templates.mjs:12` `prebuild` cannot run; `ListEggs` still 1 row | Same as §1 |
| C12 | Lifecycle / update signalling | `reference/game-hosting/pufferpanel-templates/spec.json:64` `run.query/stat/stdin/stdout/autostart` | `packages/game-templates/template-schema.json:40` `config:{stop, startup:{done}}` only + `catalog/catalog.go:11` `StatusProvisioning/Running/Error` at instance not template | Partial | **BROKEN (unchanged)** — no `stdin/stdout` strategy, no `stats/query`, no `autostart/autorestart` | Intentional if wings handles |
| C13 | Frontend / admin surface | Puffer no frontend | `forge/web/app/admin/nests/page.tsx:1` + `AdminNestsEggs.tsx:1` (Nest→Eggs, Docker images map `dockerImageLines:14`) + `forge/web/app/admin/app-templates/page.tsx:1` localStorage `forge.app-templates.v1` + `lib/api/apps.ts:369` `fetchAppTemplates` | Split-brain (two UIs) | **BROKEN** — `nests` (DB) and `app-templates` (localStorage) still disjoint; `fetchAppTemplates` never hits `/admin/nests`; `router.push("/admin/templates?nestId=...")` navigates to different system | No consolidation |
| C14 | Placeholder / condition syntax | `reference/game-hosting/pufferpanel-templates/spec.json` `${var}` + expression `if: javaversion != '' && env=='host'` + `file_exists()` | `packages/game-templates/template-schema.json` `{{VAR}}` + `BUILTIN_VARIABLES(9)` + `validate-templates.mjs:12` | Partial (no conditionals) | **BROKEN (unchanged)** — no conditional placeholders; conditionals only inside shell `if [ "${VER_EXISTS}" ...]` at `minecraft-paper.json:114`; `{{server.build.default.port}}` vs `{{SERVER_PORT}}` drift possible (daemon not validated) | Shell must reimplement |
| C15 | Catalog vs store vs template layering | Puffer one layer; `1panel/model/compose_template.go:3` `ComposeTemplate{Name, Description, Content}` | `forge/api/internal/store/store_catalog.go:84` `CatalogEntry` + `store_app_store.go:6` `AppStoreApp` + `store_nests.go:20` `Egg` + `store_templates.go:13` shim + `app-templates-data.ts:1` localStorage 5 defaults | Fragmented (3+4 layers) | **BROKEN** — three overlapping but disconnected seeding layers; no single source of truth for 14 game definitions; `AppStoreApp` and `CatalogEntry` both model versioned service catalogues but separate tables | No consolidation |
| C16 | Discoverability & ingestion | `reference/game-hosting/pufferpanel-templates/Dockerfile-templatetester` enumerates 36 | `packages/game-templates` `prebuild: node scripts/validate-templates.mjs` (`package.json:9`) + `AdminNestsEggs.tsx:124` manual paste import | Fragmented | **BROKEN** — `main` cannot build `packages/game-templates` (directory missing); CI validates 0 templates; runtime eggs DB single `091_*` egg; `AppStoreApp.SyncFromRemote:306` can pull remote registry but game-templates has no sync | Same as C11 |

**Subagent-06 Ls confirmation:**

| L | Location | Verdict |
|---|----------|---------|
| L1 Conditional-install expressiveness gap (HIGH) | `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:173` vs `packages/game-templates/templates/minecraft-paper.json:114` | **BROKEN** — see C4 |
| L2 Startup variable resolution incompleteness (MEDIUM) | `packages/game-templates/scripts/validate-templates.mjs:46` narrow + `store_egg_variables.go:92` + `satisfactory.json` `BEACON_PORT` missing `ports[]` | **BROKEN** — validator blind to `install_script`; Beacon/RCON drift not linted |
| L3 Seeding & materialization divergence — 14 templates exist nowhere after `migrate` (HIGH) | `091_seed_minecraft_java.sql:3` 1 vs `HEAD` 14 vs disk 0 vs `egg-templates.ts:27` 14 | **BROKEN (worsened)** — working tree now 0, not 14 |
| L4 Variable type & UI contract mismatch (MEDIUM) | `reference/game-hosting/pufferpanel-templates/minecraft/minecraft.json:74` groups vs `store_egg_variables.go:10` stringly-typed + `validateVariableValue:92` regex delimiter bug | **BROKEN** — `internal` vs `user_viewable:false` boundary lost; groups flattened |

---

## 4. Final-Parity Subagent-06 — 23 Rows Confirmation

Source: `audits/final-parity/subagent-06-database-templates.md` (23-row definitive parity matrix, DB01-DB14 + RT01-RT03 + TPL01-TPL09 collapsed). All file:lines re-verified live; no contradictions with phase-06 audits — only evolution is working-tree deletion (phantom → truly absent) and `HANDOFF.md` 5-layer multiplication strategy now available as precedent but not applied.

| # | Axis | Reference file:line | Forge live file:line | Status in final-parity audit | Live re-check |
|---|------|---------------------|----------------------|------------------------------|---------------|
| DB01 | Engine coverage & version gating | `1panel/constant/app.go:13` + `service/database_common.go:52` cluster | `store_db_containers.go:14,22,30` + `database_service_provisioner.go:95` + `containers.go:67` + `175_catalog_entries.sql:23` | Partial | **BROKEN (unchanged)** — `store_db_containers.go:14` still sole source but duplicated helpers remain |
| DB02 | TLS / SSL handling + envelope encryption | `1panel/model/database.go:16` `SSL bool` | `store_databases.go:311,594` + `dbprovisioner/service.go:366,373,417,422` + `database_service_provisioner.go:539` | Forge stronger (lattice) + leak P1 | **PARTIAL-FIXED / BROKEN** — 4-mode lattice intact and correct (`store_databases.go:594` `verify-full` default, x509 PEM); leak at `service.go:373` still present |
| DB03 | MySQL user model (multi-host) | `1panel/api/v2/database_mysql.go:73,52,123` + `model/database_user.go:3` | `dbprovisioner/service.go:295` + `store_database_services.go:44` no host | Missing | **BROKEN** |
| DB04 | MySQL grant lattice | `1panel/api/v2/database_mysql.go:172,193,215,236` + `service/database_mysql.go:638` `*` guard | `dbprovisioner/service.go:332` binary grant + `store_database_services.go:333` soft revoke | Partial (create-only) | **BROKEN** |
| DB05 | MySQL operational surface | `1panel/api/v2/database_mysql.go:379,509,486,464,332` + `service/database_common.go:84` | `database_service_provisioner.go:477` `TestConnection` only | Missing | **BROKEN** — `adminConn` pool exists but not exposed for `SHOW GLOBAL` |
| DB06 | PostgreSQL surface (bind/superuser) | `1panel/api/v2/database_postgresql.go:52,96,118` + `service/database_postgresql.go:89,343,384` | `dbprovisioner/service.go:550,636` + `database_service_provisioner.go:285` | Partial | **BROKEN** |
| DB07 | MongoDB surface (roles/bind) | `1panel/api/v2/database_mongodb.go:21,121,213,236` + `service/database_mongodb.go:313` `isSupportedMongodbPrivilege` | `dbprovisioner/service.go:675,693,708` fixed `readWrite` | Partial | **BROKEN** |
| DB08 | Redis surface (status/conf/persistence) | `1panel/api/v2/database_redis.go:19,42,63` + `service/database_redis.go:295` | `dbprovisioner/service.go:486,625,661` + `database_service_provisioner.go:370,114` | Partial | **BROKEN** — `go-redis` `ConfigGet` never exposed; dead `quoteSQLString` branch remains |
| DB09 | Remote DB host model (TLS, node affinity) | `1panel/model/database.go:3` union host | `store_databases.go:17,311` + `014_server_databases.sql:1` + `087_a_parity_schema.sql:30` + `116_managed_databases.sql:1` etc. | Partial (Forge stronger nodes) | **BROKEN (fragmented)** — 3 primitives remain |
| DB10 | Database lifecycle (guard, soft-delete) | `1panel/service/database_mysql.go:738` `DeleteCheck` + `:677` `LoadFromRemote` | `dbprovisioner/service.go:101,458,470` + `store_managed_databases.go:504` + `store_db_containers.go:270` | Partial (soft-delete stronger, guard weaker) | **BROKEN** — `DeleteCheck` missing for `server_databases`; `deleting` status race present; `Restart:308` no-op |
| DB11 | Config file editing | `1panel/api/v2/database_common.go:40,62` + `service/database_common.go:52` `filepath.Base:47` traversal guard | `dbbackup/service.go:455` transient cred files only; `envVarsForDB:105` immutable | Missing (by design) | **INTENTIONAL** — immutability documented as divergence |
| DB12 | Credential handling & encryption | 1Panel `encrypt.StringEncrypt` | `store_db_containers.go:14,158,233,237` AAD + `database_service_provisioner.go:217` `secrets.Keyring` | Forge stronger | **PARTIAL-FIXED / BROKEN** — AAD encryption stronger, but list query regression (LF01) makes fleet view blank |
| DB13 | Backup / restore matrix + queue | 1Panel per-engine backup modules | `dbbackup/service.go:65,115,153,240,331` + `engines.go:16` + `service.go:567` `EngineDumpCommands` | Partial (better queue, drift risk) | **BROKEN** — `engines.go:16` vs `service.go:567` duplication persists; `DBContainerService.Backup:311` no result tracking |
| DB14 | Observability & day-2 ops | 1Panel `LoadStatus:982` + `GetFPMStatus:1409` | `database_service_provisioner.go:453` `GetServiceLogs` tail 50 + `managed_databases` enum + `TestConnection:477` | Partial | **BROKEN** — status enum vs live ping conflated; `Restart` no-op |
| RT01 | Language runtimes (PHP/Node/Java/Go/Python/.NET) | `1panel/service/runtime.go:127,379,740,807,893,931,1040,1371,1409` + `model/runtime.go:9` | `services/runtime/runtime.go:31` game-server + `runtime/` adapters | Missing (intentional) | **INTENTIONAL** — correctly not ported; `compose_projects` + `buildpack` is different primitive |
| RT02 | PHP extensions | `1panel/model/php_extensions.go:3` + `service/php_extensions.go:11` + `runtime.go:931` `install-ext`+`docker commit` | No file (none) | Missing | **INTENTIONAL** |
| RT03 | PHP config / FPM / supervisor / Node modules | `1panel/service/runtime.go:1040,1153,1206,1270,1371,1409` (FastCGI `dial`) + `840` npm/yarn | `handlers_database_services.go:116` + `handlers_db_containers.go:127` status poke only | Missing | **INTENTIONAL** |
| TPL01 | Template count & coverage | `reference/game-hosting/pufferpanel-templates/*.json:1` 42/36 types | `HEAD` 14 vs disk 0 vs `egg-templates.ts:27` 14 vs `091_seed:3` 1 | Partial — phantom | **BROKEN (worsened)** — disk 0 now, not 14 |
| TPL02 | Schema draft & extensibility gate | `spec.json:4` closed `additionalProperties:false` | `HEAD` `template-schema.json:3` draft-07 + `egg-templates.ts` no validator | Partial | **BROKEN** — `EGG_TEMPLATES` still unverified TS literals |
| TPL03 | Variable / environment model (typed vs stringly, groups/internal lost) | `spec.json:156` variable+`groups` + `minecraft/minecraft.json:74,118` | `template-schema.json:69` `env[]` + `store_egg_variables.go:10` + `egg-templates.ts` | Partial | **BROKEN** — `validateVariableValue:92` regex delimiter bug still present |
| TPL04 | Install / provisioning fidelity (24 ops vs shell) | `spec.json:274` 24 ops `:282-300` | `template-schema.json:114` single shell + `store_nests.go:20` | Partial (lossy) | **BROKEN** |
| TPL05 | Environment / runtime target (host+docker vs docker-only) | `spec.json:26` `environment` + `supportedEnvironments` | `template-schema.json:24` `image+images` + `supported_platforms` | Partial (docker-only) | **INTENTIONAL** — docker-only by design |
| TPL06 | Run / startup command model | `spec.json:64` `run` fallback `command[]+if`, `pre/post`, `stdin`, `stop\|stopCode` | `template-schema.json:34` `startup` + `config:{stop, done}` | Partial | **INTENTIONAL** if wings `done` regex sufficient |
| TPL07 | Validation pipeline & placeholder contract | `spec.json:3` strict + `HEAD` `validate-templates.mjs:1` `REQUIRED:8`/`BUILTIN_VARIABLES:9`/`collectPlaceholders:20` | `HEAD` `validate-templates.mjs:46` narrow + `store_egg_variables.go:49` | Partial (narrow) | **BROKEN** — `install_script` blind spot remains; `EGG_TEMPLATES` no validator |
| TPL08 | Seeding & bootstrap (phantom catalog) | Puffer files; `1panel/service/compose_template.go:58` `Batch` | `007_postgres_core_foundation.sql:106` `Games` nest + `091_seed:3` 1 egg + `appstore/seed.go:27` 7 apps + `AdminNestsEggs.tsx:124` manual import | Missing | **BROKEN** — no FS→DB seeding job; disk 0 templates cannot be seeded |
| TPL09 | Catalog vs AppStore vs Eggs vs localStorage fragmentation | Puffer one layer; `1panel/compose_template.go:3` | `store_catalog.go:84` 11 `catalog_entries` + `store_app_store.go:6` 7 `app_store_apps` + `store_nests.go:20` eggs + `app-templates-data.ts:1` 5 localStorage | Fragmented | **BROKEN** — 4 layers remain disconnected; `HANDOFF.md` 5-layer multiplication precedent not applied |
| TPL10 | ComposeTemplate subset (generic fragment vs game definition) | `1panel/model/compose_template.go:3` `ComposeTemplate` | `store_app_store.go:1` superset + `store_catalog.go:1` + `app-templates-data.ts:1` localStorage | Forge superset on server, fragmented on client | **BROKEN** — `AppStoreApp` and `CatalogEntry` separate tables; `HANDOFF.md` registry multiplication not implemented |

**Final-parity LFs re-checked:**

| LF | File:line | Final-parity severity | Live verdict |
|----|-----------|----------------------|--------------|
| LF01 List omits encrypted cols | `store_db_containers.go:174,200` | P1 | **BROKEN** |
| LF02 Deprovision swallows + Restart no-op | `containers.go:289,300` | P1 | **BROKEN** |
| LF03 `mysql.RegisterTLSConfig` global leak | `dbprovisioner/service.go:373,378` | P1 | **BROKEN** |
| LF04 Redis `quoteSQLString` + `REDIS_PASSWORD` phantom | `database_service_provisioner.go:370,114` | P2 | **BROKEN** |
| LF05 Managed DB soft-delete + status race + string-sniff | `store_managed_databases.go:504` + `handlers_managed_databases.go:166` | P1 | **BROKEN** |
| LF06 `generatePassword` divergence + phantom + validator blind + workspace deletion | `database_service_provisioner.go:63` vs `containers.go:44,114` + `validate-templates.mjs:46` + `git status D packages/game-templates` | P1 | **BROKEN** — all 3 sub-parts still present (password divergence, validator blind, disk deletion) |

No contradictions between phase-06 audits and final-parity synthesis; final-parity's two evolutions confirmed live:
- **Working-tree deletion:** prior phase-06 saw 14 on branch / 0 on `main`; live now 0 on working tree (all `D`) — strictly worse, CI validates 0 templates.
- **`HANDOFF.md` multiplication strategy:** referenced at `final-parity/subagent-06` as precedent for consolidating `catalog_entries + featureFlags + capabilityMatrix(N)` from single `GamePanelRegistry` — not yet applied to database/template catalogs (still 4 layers).

---

## 5. Aggregated Verdict

| Bucket | Total rows/findings | **BROKEN** | **INTENTIONAL** (architectural divergence, no fix expected) | **PARTIAL-FIXED** (security win but read-path regression) | **FIXED** |
|--------|---------------------|------------|-------------------------------------------------------------|-----------------------------------------------------------|-----------|
| Phase-06 DB 18 comparisons | 18 | 11 (C01,C02,C03,C04,C05,C06,C07,C11,C12,C15,C16,C18) + C14 read-path counts as broken — effectively 13 | 5 (C08,C09,C10,C13 + C17 lattice) | 2 (C14 encryption, C17 lattice) | 0 |
| Phase-06 DB 6 LFs | 6 | 6 | 0 | 0 | 0 |
| Phase-06 TPL 16 comparisons | 16 | 10 (C1,C2,C3,C4,C6,C9,C11,C13,C14,C15,C16) | 4 (C5,C6-in-part,C8,C12) | 2 (C7,C10) | 0 |
| Phase-06 TPL 4 Ls | 4 | 4 | 0 | 0 | 0 |
| Final-parity 23 rows (collapsed) | 23 | 15 | 6 | 2 | 0 |
| **Direct checks (§1)** | 9 | 9 | 0 | 0 | 0 |

**Net:** **0 of the logic bugs are fixed.** All 6+6 LFs remain. No intentional regressions were remediated; all security/data-integrity P0s remain. The only live evolution is the working-tree deletion of `packages/game-templates`, which worsens the prior phantom-catalog finding from "14 in HEAD, 0 on checkout" to "0 on live checkout — validator not runnable".

---

## 6. Detailed Evidence (file:line snapshots)

### 6.1 `store_db_containers.go` — encrypted columns omission

```
151: func (s *Store) GetDBContainer(...) — SELECT ... COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')  // correct
174: func (s *Store) ListDBContainers — SELECT id, server_id, engine, version, container_id, connection_string, credentials, status ... // missing _encrypted
200: func (s *Store) ListAllDBContainers — same 14-col SELECT // missing _encrypted
233: func (s *Store) SetDBContainerStatus — connection_string_encrypted = COALESCE(NULLIF($6,''), connection_string_encrypted) // write correct
275: func (s *Store) GetDBContainerCredentials — SELECT credentials, connection_string, COALESCE(credentials_encrypted,''), COALESCE(connection_string_encrypted,'') // correct single path
295: func (s *Store) decryptDBContainerSecrets — decryptSecret(connectionEncrypted, db.ConnectionString, secretAAD(...)) // helper exists but starved in list paths
```

Post-`155_encrypt_db_container_credentials.sql:1` (`ALTER TABLE db_containers ADD COLUMN IF NOT EXISTS connection_string_encrypted ...`), `List*` scans plaintext `connection_string`/`credentials` which `SetDBContainerStatus:257` now clears to `''`/`'{}'` — fleet view blank.

### 6.2 `dbprovisioner/service.go` — TLS global leak

```
366: func connectorForHost(host store.DatabaseHost, password string) (driver.Connector, error)
373:     cfg := mysql.NewConfig(); cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = ...
378:         if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already registered") {
381:             cfg.TLSConfig = name
417: func mysqlTLSConfigName(host store.DatabaseHost) string — sha256(ID+host+mode+serverName+CA)[:12]
422: func hostTLSConfig(host store.DatabaseHost) (*tls.Config, error) — switches required/verify-ca/verify-full
```

No `sync.Mutex`, `sync.Map`, `UnregisterTLSConfig`, or connector-level `*tls.Config` path. Correct lattice at `:422` does not mitigate leak at `:378`.

### 6.3 `dbprovisioner/containers.go` — deprovision + restart + password divergence

```
44: func generatePassword(length int) — (length+1)/2 bytes then hex[:length]  // odd-length safe
67: func imageForDB(engine, version string) — local map, not store.DBEngineImages
105: func envVarsForDB — postgresql/mysql/mariadb/mongodb/redis branches
114:     case "mysql","mariadb": rootPassword := generatePassword(64) // distinct from password param
127:     case "redis": return []string{"REDIS_PASSWORD="+password} // non-standard, image ignores
289: func (s *DBContainerService) Deprovision — _ = s.daemon.DeProvisionDatabase(...) // swallow
300: func (s *DBContainerService) Restart — return s.store.SetDBContainerStatus(..., "running", 0, "", "", nil) // no daemon call
```

Contrast `database_service_provisioner.go:63` `generatePassword` — `make([]byte,length)` then `hex.EncodeToString(b)[:length]` (even-only safe, truncates differently on odd). `database_service_provisioner.go:118` `MYSQL_ROOT_PASSWORD=password` (same), vs `containers.go:114` random64 distinct and not persisted anywhere; `connectionStringForDB:143` + `credentialsJSON:160` embed only app password.

### 6.4 `database_service_provisioner.go` — redis dead branch

```
114: envVarsForDB redis → "REDIS_PASSWORD="+password  // non-standard
370: case "redis": _, err = conn.ExecContext(ctx, "CONFIG SET requirepass "+quoteSQLString(newPassword)) // SQL-quoted, but adminConn:522 returns error for redis (unsupported), so dead path
522: func (p *DatabaseServiceProvisioner) adminConn — switch postgresql/mysql/mariadb only, default error
571: func quoteSQLString — "'" + strings.ReplaceAll(value, "'", "''") + "'"
```

Correct redis path is `dbprovisioner/service.go:661` `rotateRedis` via `go-redis` `ConfigSet` + `ConfigRewrite`. Provisioner branch would corrupt `CONFIG SET` if `adminConn` ever supported redis.

### 6.5 `store_managed_databases.go` + handlers — deletion protection race

```
504: func (s *Store) DeleteManagedDatabaseWithForce(ctx context.Context, id string, force bool) error
508:     err := s.db.QueryRow(ctx, `SELECT volume_id, deleted_at FROM managed_databases WHERE id=$1`).Scan(&volumeID, &deletedAt)
510:         if strings.Contains(err.Error(), `column "deleted_at" does not exist`) { fallback to volume_id only }
525:     hasVolume := volumeID != nil && strings.TrimSpace(*volumeID) != ""
526:     if hasVolume && !force { return errors.New("deletion protection: use force") }
531:     tag, err := s.db.Exec(ctx, `UPDATE managed_databases SET deleted_at = NOW(), status=$2 ... WHERE deleted_at IS NULL`, id, ManagedDBStatusDeleting)
143/222/326/427/440/454/465/481/533: string-sniff "column \"deleted_at\" does not exist" repeated 8×
```

Handlers:
```
forge/api/internal/http/handlers_managed_databases.go:166 — _ = cfg.Store.UpdateManagedDatabaseStatus(ctx, id, store.ManagedDBStatusDeleting) // before DeleteManagedDatabaseWithForce
forge/api/internal/http/handlers_user_databases.go:146 — same pattern (not shown above, same race)
```

Failure after `:166` leaves row in `deleting` but not soft-deleted.

### 6.6 `packages/game-templates` — phantom catalog

```
HEAD: packages/game-templates/templates/*.json:1 — 14 files (git ls-files: 7days2die, csgo, enshrouded, factorio, minecraft-bedrock, minecraft-paper, minecraft-vanilla, palworld, rust, satisfactory, teamspeak3, terraria, valheim, zomboid)
HEAD: packages/game-templates/index.json:1 — registry 14 (minecraft-paper, minecraft-vanilla, palworld, valheim, terraria, enshrouded, satisfactory, rust, csgo, factorio, 7days2die, minecraft-bedrock, teamspeak3, zomboid)
HEAD: packages/game-templates/scripts/validate-templates.mjs:1 — REQUIRED 14 keys, BUILTIN_VARIABLES 9, VARIABLE_PATTERN /\{\{([^{}]+)\}\}/g, collectPlaceholders:20, registry check:35, env_variable pattern:35, placeholder check:46 (startup+config.files only), ports range:56
Working tree: packages/game-templates — No such file or directory (git status D * — 22 deletions)
forge/web/lib/egg-templates.ts:27 — EGG_TEMPLATES 14 (same IDs, TS literals, no validator)
forge/api/migrations/091_seed_minecraft_java.sql:3 — 1 egg 'Minecraft Java' (itzg/minecraft-server:java21, startup '', config stop/startup) + 2 variables VERSION/TYPE
forge/api/migrations/007_postgres_core_foundation.sql:106 — INSERT nests 'Games' ON CONFLICT DO NOTHING
DB state fresh: SELECT count(*) FROM eggs → 1 (plus Games nest)
```

`validate-templates.mjs:46` blind spot:
```
46: collectPlaceholders(content.startup, placeholders)
47: if (content.config && content.config.files) { collectPlaceholders(content.config.files, placeholders) }
48: for (const placeholder of new Set(placeholders)) { if !BUILTIN && !envVariables has placeholder → error }
```
Never collects `content.install_script`; `minecraft-paper.json:44` / `minecraft-vanilla.json:105` `sed -e 's/{{/${/g' -e 's/}}/}/g'` expansion inside `install_script` can reference `{{UNDEFINED}}` and pass.

### 6.7 Other drift still present

```
store_managed_databases.go:117 — NULLIF($2,'') for server_id (uuid TEXT vs '' — UUID cast quirk, SMOKE_REPORT:105)
handlers_db_containers.go:56 — serverID := c.Query("serverId"); if serverID=="" { serverID="standalone" } // hardcoded non-UUID default
store_db_containers.go:158 — write path redacts plaintext: connection_string = CASE WHEN $6<>'' THEN '' ELSE ... — correct but list never reads encrypted back
forge/api/internal/store/store_database_services.go:12 — DatabaseService.EncryptedPass stored but DatabaseServiceCredential:44 has no host
```

---

## 7. Cross-Reference to MASTER_FINDING_INDEX

| MASTER ID (if present) | Finding | Live status |
|------------------------|---------|-------------|
| `REF-P6-DB-01` (store_db_containers list) | LF01 encrypted list | **CONFIRMED BROKEN** `store_db_containers.go:174,200` |
| `REF-P6-DB-02` (global TLS leak) | LF02 `mysql.RegisterTLSConfig` | **CONFIRMED BROKEN** `service.go:378` |
| `REF-P6-DB-03` (deprovision/restart) | LF04 `Deprovision`/`Restart` | **CONFIRMED BROKEN** `containers.go:289,300` |
| `REF-P6-TMPL-01` (93% deficit, 1 egg not 14) | L3 seeding, TPL01/TPL08 | **CONFIRMED BROKEN** `091_seed:3` 1 vs `HEAD` 14 vs disk 0 |
| `REF-P6-TMPL-02` (validator narrow) | L2/LF06 `validate-templates.mjs:46` | **CONFIRMED BROKEN** `validate-templates.mjs:46` |

No new MASTER IDs needed — all findings map to existing index entries as `CONFIRMED` (not `RE-OPENED`, since never closed).

---

## 8. Recommendations (prioritized, no code changed)

**P0 — data integrity / security (fix next):**
1. `store_db_containers.go:174,200` — add `COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,'')` and call `decryptDBContainerSecrets` in scan loops (mirror `GetDBContainer:158`).
2. `containers.go:289` — propagate `DeProvisionDatabase` error, set `failed` status, do not `DeleteDBContainer` on error (mirror `database_service_provisioner.go:269`).
3. `containers.go:300` — implement `Restart` via `daemon.AdminContainerStop/Start` (or `daemon.RestartDatabase` if available) like `database_service_provisioner.go:232,245`.
4. `handlers_managed_databases.go:166` + `store_managed_databases.go:504` — move `deleting` status mutation inside `DeleteManagedDatabaseWithForce` transaction; revert on `hasVolume && !force` conflict.
5. `dbprovisioner/service.go:373` — guard `RegisterTLSConfig` with `sync.Mutex` + map and `UnregisterTLSConfig` on host update/delete, or switch to per-connector `*tls.Config` (no global).

**P1 — provisioning parity & catalog activation:**
6. Unify password policy: decide `root==app` vs distinct; persist both in `credentialsJSON`/`connection_string` if distinct; make `RotateCredentials` rotate both if equal at genesis (`containers.go:114` vs `database_service_provisioner.go:118`).
7. `validate-templates.mjs:46` (in `HEAD`) — expand `collectPlaceholders` to cover `install_script` and lint Steam `appId` integers (`258550`, `896660`, etc.) and `ports[] ↔ startup` Beacon/RCON gaps (e.g. `satisfactory` `BEACON_PORT`).
8. Restore `packages/game-templates` from `HEAD` (`git restore --source=HEAD packages/game-templates`) or collapse to single canonical (`egg-templates.ts` generated from FS); add `EggSeed` that upserts 14 FS templates into `eggs+egg_variables` on boot (like `appstore/seed.go:216` `SeedDefaultApps`).
9. Redis: delete dead `quoteSQLString` branch at `database_service_provisioner.go:370` or reimplement with `go-redis` path (`dbprovisioner/service.go:661`); remove `REDIS_PASSWORD` env or document as panel convention; expose `INFO` + `CONFIG GET/SET` via `go-redis`.

**P2 — reduce duplication & drift:**
10. Single engine catalog: keep `SupportedDBEngines:14`/`DBEngineImages:22`/`DBEngineDefaultPorts:30` as sole source; remove duplicated `defaultPortForEngine` helpers in both provisioners (`database_service_provisioner.go:81` vs `containers.go:136`).
11. Consolidate backup mapping: deduplicate `dbbackup/engines.go:16` vs `dbbackup/service.go:567` `EngineDumpCommands` to one table.
12. Replace `column "deleted_at" does not exist` string-sniff (`store_managed_databases.go:143,222,326,427,440,454,465,481,533`) with `information_schema.columns` probe or make `208_managed_database_deletion_protection.sql` mandatory.
13. Catalog consolidation: follow `HANDOFF.md` precedent — derive `catalog_entries` + `featureFlags` + `capabilityMatrix(N)` from single `GamePanelRegistry` or explicitly partition docs for each catalogue's purpose.

**P3 — docs only:**
14. Document `1panel/model.Runtime:9` ≠ `forge/services/runtime:31` (language vs game-server) and that `my.cnf`/`php.ini`/`redis.conf` editing is intentionally absent; expose typed `Provision` knobs (`max_connections`, `maxmemory`) instead of free-form file edit.
15. Fix `store_managed_databases.go:117` `NULLIF($2,'')` UUID quirk and `handlers_db_containers.go:56` `"standalone"` default.

---

*Generated for 110-02-07 — read-only confirmation, no product code modified. All citations `file:line` verified live 2026-08-24 against working tree + HEAD.*
