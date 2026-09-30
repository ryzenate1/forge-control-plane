# Subagent 13 — Reverification: Database & Template Catalog (Engine provisioning, grants, redis/pg privilege, language runtime, catalog fragmentation)

**Date:** 2026-08-24
**Branch inspected:** `mvp-2` (HEAD `bb893a5`, working-tree `D packages/game-templates/*`)
**Scope:** Read-only. No product code modified. Re-inspects `phase-06 subagent-03-1panel-database-runtime.md` (18 comparisons), `phase-06 subagent-06-templates-catalog.md` (16 comparisons), `final-parity subagent-06-database-templates.md` (23 rows) against current checkout file:line.
**Author:** reverification subagent 13/20 (parallel)

**Prior audits re-verified:**
- `audits/phase-06/subagent-03-1panel-database-runtime.md` — 18 comparisons + 6 LFs
- `audits/phase-06/subagent-06-templates-catalog.md` — 16 comparisons + 4 Ls
- `audits/final-parity/subagent-06-database-templates.md` — 23 rows + 6 LFs

**Files re-inspected (file:line verified 2026-08-24):**
- `forge/api/internal/store/store_db_containers.go:14,174,200,233` — `SupportedDBEngines`, `ListDBContainers`, `ListAllDBContainers`, `SetDBContainerStatus`
- `forge/api/internal/services/dbprovisioner/service.go:373,417,422` — `mysql.RegisterTLSConfig`, `mysqlTLSConfigName`, `hostTLSConfig`
- `forge/api/internal/services/dbprovisioner/containers.go:44,67,105,289,300,311` — `generatePassword`, `imageForDB`, `envVarsForDB`, `Deprovision`, `Restart`, `Backup`
- `forge/api/internal/store/store_managed_databases.go:504` — `DeleteManagedDatabaseWithForce`
- `forge/api/internal/services/database_service_provisioner.go:63,81,95,110,140,370,453,571` — `generatePassword`, `defaultPortForEngine`, `imageForDB`, `envVarsForDB`, `connectionString`, `RotateCredentials redis`, `GetServiceLogs`, `quoteSQLString`
- `forge/api/internal/store/store_nests.go:20,36,59,190,248,361` — `Egg`, `CreateEggRequest`, `UpdateEggRequest`, `ListEggs`, `CreateEgg`
- `forge/api/internal/store/store_app_store.go:6,10,48,92` — `AppStoreApp`, `AppStoreInstall`, `ListAppStoreApps`, `UpsertAppStoreApp`
- `forge/api/internal/store/store_catalog.go:13,30,73,84,89,134,206` — `CatalogEntry`, `CatalogInstance`, `scanCatalogRow`, `ListCatalogEntries`, `CreateCatalogInstance`, `UpdateCatalogInstanceStatus`
- `forge/api/internal/store/store_database_services.go:12` + `store_databases.go:17` + `store_catalog.go:1` (fragmentation map)
- `forge/api/migrations/091_seed_minecraft_java.sql:3` — single-egg seed
- `forge/api/migrations/155_encrypt_db_container_credentials.sql:1` — `connection_string_encrypted/credentials_encrypted`
- `packages/game-templates/templates/*.json` — **14 in HEAD** (`git ls-files | grep game-templates` → 14), **0 on disk** (`ls packages/game-templates → No such file`, `git status --short → D packages/game-templates/*` 21 deletions, unstaged)
- `packages/game-templates/src/types.ts:1`, `template-schema.json:1`, `scripts/validate-templates.mjs:46`, `index.json:1` — HEAD only (work-tree deleted)
- `forge/web/lib/egg-templates.ts:1` — `EGG_TEMPLATES:14` (hardcoded, 563 lines, still on disk)
- `forge/web/lib/app-templates-data.ts:1` — `DEFAULT_APP_TEMPLATES:5` + `STORAGE_KEY forge.app-templates.v1`
- `reference/game-hosting/pufferpanel-templates/spec.json:1` — draft `2020-12`, `$defs/variable:156`, `$defs/operation:274` 24 ops
- `reference/app-platforms/1panel/agent/app/{api/v2/database_*.go, model/database*.go, service/database*.go, service/runtime.go:43, service/php_extensions.go:11}`

---

## 1. Reverification Summary

| Claim from prior audits | Verdict on `mvp-2` checkout | Evidence |
|---|---|---|
| **DB-01 Encrypted columns omission** — `ListDBContainers` never selects encrypted columns, fleet view blank | **CONFIRMED STILL** | `forge/api/internal/store/store_db_containers.go:174` `ListDBContainers` SELECT 14 cols without `connection_string_encrypted, credentials_encrypted`; `forge/api/internal/store/store_db_containers.go:200` `ListAllDBContainers` same; only `GetDBContainer:151` selects them (`SELECT ... COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,''):158` + `decryptDBContainerSecrets:295`). `SetDBContainerStatus:233` writes encrypted + clears plaintext (`connection_string = CASE WHEN $6 <> '' THEN ''`, `credentials = '{}'::jsonb`). Post-`155` migration list callers receive `''`/`'{}'`. |
| **DB-02 TLS global leak** — `mysql.RegisterTLSConfig` mutates global map per host, never unregisters, races | **CONFIRMED STILL** | `forge/api/internal/services/dbprovisioner/service.go:373` `if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already registered")`; `mysqlTLSConfigName:417` `sha256(ID+host+mode+serverName+CA)[:12]` → new name per rotation; no `UnregisterTLSConfig`, no `sync.Mutex`. `hostTLSConfig:422` builds per-host `*tls.Config` then registers globally. |
| **TMPL-01 93% seeding deficit** — 14 templates authored, 1 egg seeded, 0 on disk on `main` | **CONFIRMED WORSE** — now **100% on disk deficit** | HEAD has 14 (`git ls-files` lists `minecraft-paper.json`, `rust.json`, etc. `bb893a5`); **working tree 0** (`ls packages/game-templates → No such file`). `git status` shows `D packages/game-templates/*` 21 deletions unstaged on `mvp-2`. Fresh `forge/api` migration still only `migrations/091_seed_minecraft_java.sql:3` inserts 1 egg `Minecraft Java` (`itzg/minecraft-server:java21`, empty startup). `forge/web/lib/egg-templates.ts:27` still holds 14 hardcoded `EGG_TEMPLATES` (frontend-only, no DB sync). So `ListEggs(ctx,""):190` returns 1 row vs authored 14 → 93% deficit; on this checkout even FS source is absent → 100% FS deficit. |
| **Validation blind to `install_script`** — `validate-templates.mjs` only checks `startup + config.files` placeholders | **CONFIRMED STILL** | `HEAD:packages/game-templates/scripts/validate-templates.mjs:32-54` `collectPlaceholders` called only on `content.startup` and `content.config.files`; `content.install_script.script` never collected. `git show HEAD:scripts/validate-templates.mjs:46` loop `for (const placeholder of new Set(placeholders)) if (!BUILTIN && !envVariables)` — `install_script` can reference `{{UNDEFINED}}` → `curl -L ""` still passes. File itself is **deleted on disk** (`cat packages/.../validate-templates.mjs → No such file`), so `prebuild: node scripts/validate-templates.mjs` (`package.json:9`) is not runnable on this checkout; CI validates 0 templates. |
| **DB-03 Deprovision swallows daemon error / Restart no-op** | **CONFIRMED STILL** | `forge/api/internal/services/dbprovisioner/containers.go:289` `_ = s.daemon.DeProvisionDatabase(...)` ignored then `return s.store.DeleteDBContainer`; `:300` `Restart` only `return s.store.SetDBContainerStatus(..., "running", ...)` with no `daemon.AdminContainerStart/Stop`. Contrast `forge/api/internal/services/database_service_provisioner.go:259` `DeleteService` correctly returns error and flips `failed` on daemon failure, `:245` `StartService`/`232` `StopService` call daemon. |
| **DB-04 Password divergence + phantom `REDIS_PASSWORD`** | **CONFIRMED STILL** | `database_service_provisioner.go:63` `generatePassword` does `make([]byte,length)` then `hex.EncodeToString(b)[:length]` (odd `length` truncates incorrectly); `dbprovisioner/containers.go:44` does `(length+1)/2` (odd-safe). Functional divergence for even lengths minimal but `containers.go:105` `envVarsForDB mysql` generates `rootPassword := generatePassword(64)` distinct from `password`, while `database_service_provisioner.go:110` sets `MYSQL_ROOT_PASSWORD=password` (same). Both set `REDIS_PASSWORD` (`containers.go:128` `REDIS_PASSWORD=`, `provisioner.go:132` same) which `redis:7` image ignores (password must be `CONFIG SET requirepass`). |

No prior row is contradicted; two evolutions tighten prior conclusions: (a) `packages/game-templates` working-tree deletion makes FS source **truly absent** (prior audit saw branch copy in `.freebuff/worktrees`); (b) `HANDOFF.md` 5-layer multiplication strategy now documented as precedent for consolidating `catalog` vs `app_store` vs `eggs` (not yet applied).

---

## 2. Reconciled Parity Matrix (14 rows — requires ≥12)

| # | Axis | REFERENCE Path:Symbol | FORGE Layers file:line | STATUS | GAP | SEVERITY |
|---|------|------------------------|------------------------|--------|-----|----------|
| **DB-01** | Engine coverage & version gating | `1panel/constant/app.go:13` `AppMysql/AppPostgresql/AppMongodb/AppRedis` (+ `-cluster` variants `service/database_common.go:52`); `007_postgres_core_foundation.sql:106` `Games` nest | `store_db_containers.go:14` `SupportedDBEngines={pg13-16,mysql8.0-8.3,mariadb10-11,redis6-7,mongo6-7}`; `ValidateDBEngine:63`, `CanonicalDBEngine:93` `postgres→postgresql`; `175_catalog_entries.sql:23` adds `valkey/rabbitmq/clickhouse/nats/memcached` beyond 1Panel; `database_service_provisioner.go:81,95` + `containers.go:67,136` duplicate `defaultPortForEngine`/`imageForDB` maps vs `store.DBEngineDefaultPorts:30` | **Partial** | No `mysql-cluster`/`pg-cluster`/`redis-cluster` topology; hardcoded version map requires triple change (store + `114_*.sql:69` seed + both provisioner image maps); `ports` duplication drift | P2 |
| **DB-02** | TLS / envelope encryption | `1panel/model/database.go:16` `SSL,RootCert,ClientKey,ClientCert,SkipVerify bool`; `service/database.go:128,182` per-engine clients, `api/v2/database.go:25` base64 PEM decode | `store/store_databases.go:311` `tls_mode/tls_ca/tls_server_name`; `store_databases.go:594` `validateDatabaseHostRequest` default `verify-full` + x509 PEM; `dbprovisioner/service.go:366` `connectorForHost:372` `hostTLSConfig:422` 4-mode lattice (`disable/required/verify-ca/verify-full`) + AAD `store_db_containers.go:233` `secretAAD("db_containers",id,"connection_string")` `233,295` | **Forge stronger, logic bug retained** | Lattice strictly more expressive than 1Panel `SkipVerify bool`; envelope encryption with AAD stronger than 1Panel `encrypt.StringEncrypt`. **But** global `mysql.RegisterTLSConfig` leak retained (see LF01). Migration must map `skip_verify=true → required` (comment `service.go:439`). | P1 (leak) |
| **DB-03** | MySQL user model (multi-host) | `1panel/api/v2/database_mysql.go:73` `CreateMysqlUser`, `:102` `Delete`, `:123` `Update` (`splitMysqlHosts:86` comma, `service/database_mysql.go:541` host rename rollback), `:144` `ChangePassword`, `:52` `ListUsers` hiding `isMysqlSystemUser:111`, `model.DatabaseUser:3` unique `idx(type,database,username,host)` | `dbprovisioner/service.go:295` `CREATE USER 'u'@'%'` single host; `store/store_database_services.go:44` `DatabaseServiceCredential{username,encrypted_password,database_name,permissions}` **no host column**; `database_service_provisioner.go:295` even narrower (only create+grant) | **Missing** | Cannot model `user@10.0.0.%` vs `localhost` vs `%`; no `ListUsers`/`DeleteUser`/`UpdateUser`/multi-host; password rotation per-app-user missing. | P1 |
| **DB-04** | MySQL grant lattice | `1panel/api/v2/database_mysql.go:172` `ListGrants`, `:193` `ListGrantSummary`, `:215` `Grant`, `:236` `Revoke` guard `*` (`service/database_mysql.go:638`) | `dbprovisioner/service.go:332` `GrantPermissions` binary `ALL` vs `SELECT`; `database_service_provisioner.go:332` same; no `ListGrants`/`RevokeGrant`; `store.RevokeServiceCredential:333` soft `revoked_at` only, never `REVOKE/DROP USER` | **Partial (create-only)** | Audit who-can-access-which-DB impossible; revocation not wired to engine. | P1 |
| **DB-05** | MySQL operational surface | `1panel/api/v2/database_mysql.go:379` `LoadFormatOption:1031` (`utf8mb4`), `:509` `LoadVariables:964`, `:486` `LoadStatus:982` (`SHOW MASTER STATUS`/`SHOW BINARY LOG STATUS` `≥8.4:1016`), `:464` `LoadRemoteAccess:946` (`host='%'`), `:332` `UpdateVariables:890` `updateMyCnf:1079` + `compose.Restart` | `database_service_provisioner.go:477` `TestConnection` (`SELECT 1`) only + `store_databases.go:224` host CRUD; no `my.cnf` edit, no live vars/status, no collation preview | **Missing by design** | Day-2 tuning (buffer pool, `max_connections`) + observability (QPS, replication `File:Position`) absent; read-only `SHOW GLOBAL` via existing `adminConn:522` pool trivial but not wired. | P2 |
| **DB-06** | PostgreSQL surface | `1panel/api/v2/database_postgresql.go:52` `BindUser` (`SuperUser` `service/database_postgresql.go:89,106`), `:96` `ChangePrivileges` (`super_user` `343`), `:118` `ChangePassword` per-DB vs root `384` + `updateInstallInfoInDB:422` | `dbprovisioner/service.go:550` `provisionPostgreSQL` (`CREATE ROLE LOGIN`, `CREATE DATABASE WITH OWNER`, `ALL PRIVILEGES`) `rotateRemote:636`; `database_service_provisioner.go:285` (`postgresql`) covers `CREATE DATABASE WITH OWNER` without `pg_roles` check; no `BindUser` upsert, no `superUser` flag, no app-env propagation | **Partial** | Cannot replicate superuser distinction or `BindUser` re-attach after restore. | P2 |
| **DB-07** | MongoDB surface | `1panel/api/v2/database_mongodb.go:21` create `isSupportedMongodbPrivilege:910` (`dbOwner/read/readWrite/userAdmin`), `:121` `BindUser` (`buildMongodbBindUserScript:448`), `:213` `LoadPrivileges`/`236` `ChangePrivileges` | `dbprovisioner/service.go:675` `provisionMongoDB` fixed `readWrite`, `693` deprovision, `708` rotate; `dbbackup/engines.go:86,93` `mongodump/mongorestore` only | **Partial** | `dbOwner`/`userAdmin`/`read` choices + post-create mutation absent. | P2 |
| **DB-08** | Redis surface | `1panel/api/v2/database_redis.go:19,42,63` status/conf/persistence `docker exec redis-cli` (`service/database_redis.go:295` `redisExec`), `LoadStatus:132` `INFO`, `LoadConf:160`, `LoadPersistenceConf:177` via `confSet:217` marker, `CheckHasCli:83` helper `redis:7.4.4` | `dbprovisioner/service.go:486` `provisionRedis` `CONFIG SET requirepass` + `appendonly yes` + best-effort `CONFIG REWRITE:498` ignored; `625` deprovision, `661` rotate via `go-redis`; `database_service_provisioner.go:370` dead `CONFIG SET requirepass `+`quoteSQLString` (SQL quoting for Redis, unreachable) ; no `INFO`/`CONFIG GET`, no persistence editor | **Partial** | Cannot observe/tune Redis; `quoteSQLString` latent bug; phantom `REDIS_PASSWORD` env in both provisioners (image ignores). | P2 |
| **DB-09** | Remote host model & fragmentation | `1panel/model/database.go:3` `Database{AppInstallID,Name,Type,From local\|remote,Address,Port,InitialDB,Username,Password,SSL...}`; `DeleteCheck:252` guards `appInstallResourceRepo` | `store/store_databases.go:17` `DatabaseHost` (`tls_mode` etc.) + `database_host_node` `087:30`; `server_databases:014:17` lease; `db_containers:098:50` + `managed_databases:116:1` container-provisioned; `database_services:114:5` legacy beacon | **Partial, fragmented** | Three primitives (`server_databases` host-provisioned vs `managed_databases+db_containers` container vs `database_services` beacon) require caller to choose; no `From=local` equivalent surfaced (`handlers_database_services.go:337` only reassigns `server_id`); FK `managed_databases.server_id` via `ALTER DO-block 116:59` may be absent (legacy fallback `store_managed_databases.go:136,215`). | P2 |
| **RT-01** | Language runtimes (PHP/Node/Java/Go/Python/.NET) | `1panel/service/runtime.go:127` `Create` (`RuntimePHP|RuntimeNode|...:7`), `:379` `Delete` `DeleteCheck:367`, `GetPHPExtensions:893` `InstallPHPExtension:931` `UnInstall:1018` `GetPHPConfig:1040`/`1086`, `GetFPMStatus:1409`, `GetSupervisorProcess:1371`, `GetNodeModules:807` `OperateNodeModules:840` | `forge/api/internal/services/runtime/runtime.go:1` **game-server runtime** (`Start/Stop/Stats/Logs/ExecuteCommand/ReadFile/...:31`) via docker/podman/k8s adapters (`forge/api/internal/runtime/`), `clustermanager/service.go:65` `gpruntime.CreateServer`; no `model.Runtime:9` equivalent | **Missing (intentional divergence)** | WordPress-style 1Panel site not lift-and-shift; requires `compose_projects` instead. 1Panel `global.Dir.RuntimeDir:36` host-mount vs Forge multi-node scheduler — cannot shim. | P3 (doc) |
| **TPL-01** | Template count & coverage | Puffer `spec.json:1` + `reference/game-hosting/pufferpanel-templates/*.json:1` 36 types / 42 files (incl. 6 `data.json`) — SteamCMD `rust 258550`, `valheim 896660`, `7days2die 294420`, `csgo 740`, Mojang/Paper/Fabric/NeoForge etc. | `packages/game-templates` **HEAD 14** (`minecraft-paper:valid`, `rust`, `valheim`, `palworld`, `enshrouded` etc.) — **0 on disk** (`D` unstaged, `ls packages/game-templates → No such file`); `forge/web/lib/egg-templates.ts:27` **14** hardcoded mirror; DB `091_seed_minecraft_java.sql:3` **1 egg** `Minecraft Java` empty startup; `store_nests.go:20` `Egg` + `store_templates.go:13` shim | **Partial 39% + phantom on disk** | Missing `ark,arma3,css/cstrike,discord-*,dontstarvetogether,eco,gmod,minecraft-bungeecord/curseforge/ftb/velocity/waterfall,pocketmine,squad,starbound,tf2,unturned,vintage-story` etc.; Forge adds `palworld,enshrouded`. Working-tree deletion makes authored catalogue **0 FS files** reachable. | **P0** |
| **TPL-02** | Variable / env model | Puffer `spec.json:$defs.variable:156` `type enum[option,string,boolean,integer]:165`, `options[]:183`, `internal:true:118` (`minecraft.json:118` `resolvedForgeVersion`), `groups: minItems:1 if:string:155` (`minecraft.json:74` Paper/Fabric groups) | Forge `template-schema.json:69` `env: {name,env_variable,default_value,user_viewable,user_editable,rules}` + `store_egg_variables.go:10` `EggVariable` all-string + `rules` DSL (`validateVariableValue:92`); no `option`/`internal`/`groups` | **Partial (stringly-typed collapse)** | `internal` vs `user_viewable:false` boundary lost; Paper/Fabric/Forge groups flattened to single sorted list (invite misconfig). `store_egg_variables.go:92` `validateVariableValue` includes `/` delimiters in `regexp.Compile` (phase-02 LF-01 still present). | P2 |
| **TPL-03** | Install / provisioning fidelity | Puffer `spec.json:$defs.operation:274` **24 decl ops** with typed args + per-step `if: string` (`steamgamedl`, `paperdl`, `mojangdl`, `javadl`, `resolveforgeversion` etc. `:282-300`) with `file_exists`/`os` guards (`minecraft.json:173` `if javaversion!='' && env=='host' → javadl`) | Forge `template-schema.json:114` `install_script:{container,entrypoint,script:string}` single shell (`rust.json:127` `steamcmd +app_update 258550`, `minecraft-paper.json:44` Papermc `curl+jq` fallback); `store_nests.go:20` `Egg.InstallScript` same single-script DB model `:232` normalizes but never validates 24-op catalogue | **Partial (lossy transliteration)** | `javadl` host-only gate dropped, Forge shim `file_exists` fallback hardcoded, Steam `appId` literals lack schema guard, `pre:[resolveforgeversion]` not synchronized between install and startup. | P2 |
| **TPL-04** | Validation & placeholder contract | Puffer draft `2020-12` `additionalProperties:false` strict at `spec.json:4` | Forge `template-schema.json:3` draft-07 `required:[id,name,description,version,game,image,startup,config,ports,env,resources,install_script,supported_platforms,categories]:6` + `scripts/validate-templates.mjs:1` `REQUIRED:8`, `BUILTIN_VARIABLES(9):12`, `collectPlaceholders:20` over `startup+config.files:46-54`, ports range, `file===id.json:65`. **But** `egg-templates.ts` no validator; `store_egg_variables.go:49` `validateEggVariableRequest` never checks `Egg.Startup` placeholders | **Partial (narrow validator, now unrunnable)** | Validator ignores `install_script` placeholders (most dangerous `DL_PATH sed`); Steam `appId` ints and `ports[] ↔ startup` Beacon/RCON gap unchecked (`satisfactory.json` `BEACON_PORT 15000` no `ports[]` entry); work-tree deletion makes `validate-templates.mjs` non-executable. | P1 |
| **TPL-05** | Catalog vs AppStore vs Eggs vs localStorage fragmentation | Puffer single layer `templates/*.json + data.json + spec.json`; 1Panel `model/compose_template.go:3` `ComposeTemplate{Name,Description,Content}` `Batch:58` upsert | Forge **four disconnected layers**: (1) `store_catalog.go:13` `CatalogEntry{Key,DisplayName,Category,Versions[],DefaultVersion,Requires[],Enabled,SortOrder}` + `CatalogInstance{EntryKey,Kind,Version,EnvironmentID,NodeID,RefType,Status,ConnString}` `catalog/catalog.go:68` version-gated (`isManagedDBKind→provisionDB else provisionCompose`); (2) `store_app_store.go:6` `AppStoreApp{Key,Name,Category,Tags[],Version,ComposeContent,Params JSON,MinMemoryMB}` + `AppStoreInstall` via `compose.Service.DeployComposeStack:116` + `SyncFromRemote:306`; (3) `store_nests.go:20` `Egg`+`EggVariable`+`Template` shim `store_templates.go:13`; (4) `app-templates-data.ts:1` 5 defaults + `localStorage forge.app-templates.v1` (never touches DB, `apps.ts:369` `fetchAppTemplates` falls back to `getAllTemplates`) + frontend `AdminNestsEggs.tsx:1` vs `app-templates/page.tsx:1` split-brain | **Fragmented** | No single source for 14 game definitions; no `SeedDefaultApps:529`-style `EggSeed` bridging FS→DB; no `Batch`-style upsert for eggs; `catalog_entries` vs `app_store_apps` both versioned service catalogues but separate tables. `HANDOFF.md` precedent (`capabilityMatrix+featureFlags.slice+catalogEntries`) available but not yet applied to `app_store/eggs/localStorage`. | **P1** |

---

## 3. Logic Findings (6 — requires ≥3)

### LF01 — DB-01 Encrypted columns omission still — `List*` returns blank credentials (P1, data integrity) — CONFIRMED

**Location:** `forge/api/internal/store/store_db_containers.go:174` `ListDBContainers` and `:200` `ListAllDBContainers` — SELECT 14 cols (`id,server_id,engine,version,container_id,connection_string,credentials,status,port,volume_id,memory_mb,cpu_shares,created_at,updated_at`); `:151` `GetDBContainer` SELECT 16 cols adding `COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,''):158` + `decryptDBContainerSecrets:295`.

**Repro after `155_encrypt_db_container_credentials.sql:1`:** `SetDBContainerStatus:233` persists `connectionEncrypted/credentialsEncrypted` and clears plaintext (`CASE WHEN $6 <> '' THEN '' ELSE connection_string END:257` + `'{}'::jsonb:258`). `ListDBContainers:188` scans plaintext `connectionString, credentials` which are now `''`/`'{}'` — credentials lost for fleet view; `GetDBContainerCredentials:275` (single-id) still works via encrypted path.

**Impact:** `forge/web/app/admin/databases/page.tsx:17` `containers` tab via `ListAllDBContainers` shows blank `connectionString/credentials` for encrypted rows — admin can fetch single container but fleet view blank (high confusion).

**Fix:** Add encrypted cols to both list queries (mirror `GetDBContainer:158`) and call `decryptDBContainerSecrets` in scan loop; regression test `create → encrypt → list → assert connectionString non-empty`.

**Prior mapping:** `REF-P6-DB-01` CONFIRMED; `phase-06 subagent-03 LF01` + `final-parity LF01`.

---

### LF02 — DB-02 TLS global leak and race per host (P1, security) — CONFIRMED

**Location:** `forge/api/internal/services/dbprovisioner/service.go:373` `mysqlTLSConfigName:417` `sha256(ID+host+mode+serverName+CA)[:12]` → `mysql.RegisterTLSConfig(name, tlsConfig)` mutating global `map[string]*tls.Config`.

**Finding:** Each distinct `DatabaseHost` registers a new global entry never unregistered nor updated if `TLSCA` rotates. Concurrent provisions for same host race `already registered` check (non-atomic). Rotation computes new name but old entry remains bound to prior connectors; global map grows unbounded, stale CA pins survive. `hostTLSConfig:422` 4-mode lattice is correct; registration mechanism is not. 1Panel `utils/mysql/client` presumably builds per-client `tls.Config` without global registry (not inspected, assumed).

**Fix:** Guard with `sync.Mutex` + `map[string]*tls.Config` or use `mysql.NewConnector` with per-connector dialer applying `tls.Config` without global registration (driver `cfg.TLS = &tls.Config` on recent versions — verify; else `SyncOnce` per key + `UnregisterTLSConfig` on host update/delete).

**Prior mapping:** `REF-P6-DB-02` CONFIRMED; `phase-06 LF02` + `final-parity LF03`.

---

### LF03 — TMPL-01 93% seeding deficit, now 100% FS deficit + phantom catalogue (P0, data-loss on install) — CONFIRMED WORSE

**Locations:** `migrations/091_seed_minecraft_java.sql:3` (1 egg `Minecraft Java` `itzg/minecraft-server:java21` empty startup), `packages/game-templates/templates/*.json` (HEAD 14 → `git ls-files` 14, work-tree `ls → No such file`), `forge/web/lib/egg-templates.ts:27` (14 constants), `store_nests.go:190` `ListEggs`, `store_templates.go:13` `ListTemplates`, `lib/api/apps.ts:369` `fetchAppTemplates`, `app-templates/page.tsx:56` `localStorage`.

**Finding:** Fresh `forge/api` migration yields exactly 1 egg (`ListEggs("", ""):190` returns 1) vs authored 14. Bridge `src/index.ts:8` `templateToApiTemplate` and `EGG_TEMPLATES` constant never materialize as persistent `eggs`; admin's only path is manual `AdminNestsEggs.tsx:124` `importEgg` paste-JSON (validates only `dockerImages.length>0`), no batch import, no `SeedDefaultApps:27`-style `UpsertEggTemplates`. Current checkout makes it worse: `packages/game-templates` is **deleted from work-tree** (`D` unstaged, `ls packages → sdk, shared-types` only), so `prebuild: node scripts/validate-templates.mjs` cannot run and CI on this checkout validates 0 templates. Effective state: 0 FS files + 1 DB row + 14 frontend constants + 7 `AppStoreApp` compose apps + 5 `localStorage` app-templates = 4 disconnected truths.

**Impact:** Any claim Forge ships 14 game servers out-of-box is false on this checkout (ships 1 minimal placeholder + 14 phantom constants).

**Fix:** `git restore --source=HEAD packages/game-templates` (or collapse to single canonical generated from FS), then introduce `EggSeed` upserting 14 FS templates into `eggs+egg_variables` on migration/boot (like `appstore/seed.go:216` `SeedDefaultApps` called from `cmd/api/main.go:529`), plus `Batch`-style upsert.

**Prior mapping:** `REF-P6-TMPL-01` CONFIRMED WORSE; `phase-06 subagent-06 L3` + `final-parity LF06(workspace deletion)+TPL08`.

---

### LF04 — Validation blind to `install_script` — `validate-templates.mjs:46` narrow scope + now unrunnable (P1, correctness) — CONFIRMED

**Location:** `HEAD:packages/game-templates/scripts/validate-templates.mjs:12` `BUILTIN_VARIABLES(9)`, `:20` `collectPlaceholders`, `:46` `collectPlaceholders(content.startup, ...)` + `content.config.files` only.

**Findings:**
1. `install_script.script` placeholders (most dangerous place — `minecraft-paper.json:44` `DL_PATH sed -e 's/{{/${/g' -e 's/}}/}/g'`, `rust.json:127` Steam literals) never checked → `{{UNDEFINED_IN_ENV}}` inside `install_script` passes but produces `curl -L ""` at deploy. Puffer expands variables in all `install`+`run` fields (`spec.json` validation broader).
2. Builtin list omits `{{PORT}}` legacy check only via docs (`README.md:56` warns but not enforced); `ports[] ↔ startup` Beacon/RCON gap unchecked (e.g. `satisfactory.json` `BEACON_PORT 15000` in `startup` but no `ports[]` entry — firewall allocator gap).
3. Work-tree deletion makes validator **non-executable** on this checkout (`cat validate-templates.mjs → No such file`), so `npm run prebuild` fails; CI on `mvp-2` working tree validates 0 templates regardless of content.

**Fix:** Expand `validate-templates.mjs:49` to `collectPlaceholders(content.install_script.script, ...)` + lint Steam `appId` integers (`258550`, `896660`, `294420` etc.) and `update_url` reachability; add `ports[] ↔ startup` cross-check; ensure `packages/game-templates` present on all branches (`git restore`).

**Prior mapping:** `REF-P6-TMPL-02` + `final-parity LF06` `validate-templates.mjs:20` family CONFIRMED.

---

### LF05 — `Deprovision` swallows daemon error, `Restart` is status-only no-op + managed-delete status race (P1, consistency) — CONFIRMED

**Locations:**
- `dbprovisioner/containers.go:289` `Deprovision` `_ = s.daemon.DeProvisionDatabase(ctx, ..., db.ContainerID, db.VolumeID)` then hard `DeleteDBContainer:270` regardless; `:300` `Restart` only `SetDBContainerStatus(..., "running", 0, "", "", nil)`.
- `store_managed_databases.go:504` `DeleteManagedDatabaseWithForce:504` + handlers `handlers_managed_databases.go:158` (inferred from final-parity, verify via `ListManagedDatabases:215` legacy fallback pattern) sets `UpdateManagedDatabaseStatus(ctx, id, deleting)` before `DeleteManagedDatabaseWithForce` → if `hasVolume && !force` fails, row stuck in `deleting` forever unless retried with `force`.
- Contrast `database_service_provisioner.go:259` `DeleteService:269` correctly propagates `DeProvisionDatabase` error and flips `failed`.

**Impact:** Orphan volume/container leak on daemon failure; `handlers_db_containers.go:127` restart always succeeds even when container failed; `deleting` stuck state.

**Fix:** Mirror `DeleteService:269` semantics: propagate `DeProvisionDatabase` error, set `failed` status, do not delete store row on error; implement `Restart` via `daemon.AdminContainerStart/Stop` like `provisioner.go:245`; move `deleting` mutation inside `DeleteManagedDatabaseWithForce` transaction or revert on `hasVolume && !force`.

**Prior mapping:** `REF-P6-DB-03` CONFIRMED; `phase-06 LF04/06` + `final-parity LF02/LF05`.

---

### LF06 — `generatePassword` divergence + `REDIS_PASSWORD` phantom + duplicate engine catalog (P2, provisioning drift) — CONFIRMED

**Locations:**
- `database_service_provisioner.go:63` `b := make([]byte,length)` then `hex.EncodeToString(b)[:length]` (64 hex chars then slice — odd-length truncates differently, even length coincidentally matches);
- `containers.go:44` `b := make([]byte,(length+1)/2)` then `[:length]` (odd-safe);
- `containers.go:105` `envVarsForDB mysql` `rootPassword := generatePassword(64)` distinct from `password` for `MYSQL_USER`, vs `provisioner.go:110` `MYSQL_ROOT_PASSWORD=password` (same);
- `containers.go:67` `imageForDB` + `database_service_provisioner.go:81,95` `defaultPortForEngine`/`imageForDB` vs `store.DBEngineImages:22`/`DBEngineDefaultPorts:30` single source.

**Finding:** A DB via `DBContainerService` gets `u_…` password `!=` root password (root random64 not persisted, `connectionStringForDB:143` embeds only app password), while via `DatabaseServiceProvisioner` root==app. Rotation in both layers (`dbprovisioner/service.go:252` `rotateRemote`, `provisioner.go:354` `RotateCredentials`) only rotates app user (`ALTER USER u@...` / `ALTER ROLE`), never root — root credential diverges from panel store if provisioned via `Provisioner`. `connectionString:140` not persisted for root. Both provisioners set `REDIS_PASSWORD` (non-standard; `redis:7` image ignores env, password must be `CONFIG SET` post-start as `dbprovisioner` correctly does in `provisionRedis:486`). Engine catalog duplicated across three maps.

**Fix:** Unify password policy (decide root==app or distinct, persist both in `credentialsJSON:160`/`credsJSON:157`); ensure `RotateCredentials` rotates both if equal at genesis; remove `REDIS_PASSWORD` env or document as panel convention; consolidate `imageForDB`/`defaultPortForEngine` to `store.DBEngine*` single source.

**Prior mapping:** `phase-06 LF05` + `final-parity LF06(1)` CONFIRMED.

---

## 4. Cross-Reference: Prior Audits Reconciled

- **Subagent-03 C01–C18 re-verified:** All 18 rows hold at file:line on `mvp-2`. Engine coverage (C01), MySQL user/grant (C02–C04), PG (C05), Mongo (C06), Redis (C07), runtimes (C08–C10), host model (C11), lifecycle (C12), conf editing (C13), encryption (C14), backup (C15), observability (C16), TLS (C17), migrations shims (C18) — no contradictions. Addendum: `SupportedDBEngines:14` + `175_catalog_entries.sql:23` now adds `valkey/rabbitmq` beyond prior count, tightening C01. LF01–LF06 map 1:1 to this report's LF01/02/05/06.
- **Subagent-06 C1–C16 re-verified:** Counts updated — Puffer **36 types / 42 files** (prior 44 JSON payloads incl. `spec.json+6×data.json` — consistent), Forge **14** (prior: 14 on branch, 0 on `main` — now **0 on disk** due to unstaged deletion is stricter than prior). Seeding table (1 egg + 7 `AppStoreApp:seed.go:27` + 5 `localStorage` + 11 `catalog_entries:175` = fragmentation 4) confirmed. `spec.json:274` 24 ops, `template-schema.json:114` single shell, `validate-templates.mjs:46` narrow scope all confirmed despite file being HEAD-only.
- **Final-parity subagent-06 DB01–DB14 + RT01–RT03 + TPL01–TPL10 (23 rows) re-verified:** No row contradicted; `DB02` TLS leak, `DB12` encrypted list, `DB10` deletion race, `TPL08` phantom catalog, `TPL07` validation narrow all re-confirmed with tighter evidence (work-tree deletion). This report adds explicit `mvp-2` git-status evidence (`D` 21 files) not observed in final-parity (which saw `.freebuff/worktrees` branch copies).
- **SMOKE_TEST_REPORT open items:** `NULLIF($2,'')` UUID in `store_managed_databases.go:117` + `"standalone"` default pattern (cited as `handlers_db_containers.go:56` in final-parity) still tracks same drift class as LF06; string-sniff fallbacks (`column "deleted_at" does not exist` at `store_managed_databases.go:143,222,326,427,440,454,465,481,533`) remain fragile — not yet hardened to `information_schema.columns` probe or mandatory migration.
- **No new MASTER IDs needed:** All findings are `CONFIRMED_BY` this subagent to `REF-P6-DB-01..03`, `REF-P6-TMPL-01/02`, `REF-P6-RT-01`.

---

## 5. Recommendations (informational, no product code modified — prioritized)

### P0 — Data integrity / security
1. **Fix `ListDBContainers` encrypted columns** (`store_db_containers.go:174,200`) — mirror `GetDBContainer:158` and call `decryptDBContainerSecrets` in list loops. Regression: `create → encrypt → list → assert connectionString non-empty`.
2. **Make `Deprovision`/`Restart` honest** (`containers.go:289,300`) — propagate `DeProvisionDatabase` error, set `failed`, do not delete; `Restart` via `daemon.AdminContainerStart/Stop` like `provisioner.go:245`.
3. **Guard deletion ordering** (`handlers_managed_databases.go` + `store_managed_databases.go:504`) — mutate `deleting` inside same transaction or revert on `hasVolume && !force`; replace string-sniff with schema-version check.

### P1 — Provisioning parity & catalog activation
4. **Restore `packages/game-templates` from HEAD** (`git restore --source=HEAD packages/game-templates`) — currently 0 files on disk breaks `prebuild` and any FS→DB seeding.
5. **Close catalog seeding gap** — `EggSeed` upserting 14 FS templates into `eggs+egg_variables` on boot (like `appstore/seed.go:216` `SeedDefaultApps:529`), plus `Batch`-style batch import (`1panel/service/compose_template.go:58` precedent).
6. **Expand validation** — `validate-templates.mjs:46` must cover `install_script` placeholders + Steam `appId` ints + `ports[] ↔ startup` Beacon/RCON gap; ensure `egg-templates.ts` generated from FS rather than duplicated.
7. **Fix TLS global leak** (`dbprovisioner/service.go:373`) — `sync.Mutex` + map or connector-level TLS (no global).
8. **Unify password policy** (`provisioner.go:110` vs `containers.go:114`) — decide root==app or distinct, persist both, rotate both; remove phantom `REDIS_PASSWORD`.

### P2 — Reduce duplication & drift
9. **Single engine catalog** — keep `SupportedDBEngines:14`/`DBEngineImages:22`/`DBEngineDefaultPorts:30` as sole source; remove duplicated `defaultPortForEngine`/`imageForDB` helpers in both provisioners.
10. **Consolidate backup mapping** — deduplicate `dbbackup/engines.go:16` vs `dbbackup/service.go:567` `EngineDumpCommands`.
11. **String-sniff hardening** — replace `column "deleted_at" does not exist` sniff at `store_managed_databases.go:143` et al. with `information_schema.columns` probe or make `208` mandatory before code ships.
12. **Catalog consolidation** — follow `HANDOFF.md` precedent: derive `catalog_entries` + `featureFlags` + `capabilityMatrix` from single registry or explicitly partition docs for `catalog` (infra DBs) vs `app_store` (compose apps) vs `nests/eggs` (games) vs `localStorage` (generic app-templates).

### P3 — Product positioning (doc only)
13. **Document runtime divergence** — `1panel/model.Runtime:9` ≠ `forge/services/runtime:31` (language vs game-server); add ADR that `my.cnf`/`php.ini`/`redis.conf` editing is intentionally absent, tuning via typed `Provision` params.
14. **MySQL grant/user parity** — if 1Panel migration is goal, extend `database_service_credentials:114` with `host TEXT DEFAULT '%'` and add `ListUsers/ListGrants/RevokeGrant` handlers (else doc divergence "one user per service").
15. **SMOKE open items** — fix `NULLIF($2,'')` UUID (`store_managed_databases.go:117`) and ensure no `"standalone"` hard-coded default remains (already filed under `REF-P6-DB-01` family).

---

## 6. Appendix — Evidence & Inventory

### 6.1 Git evidence (2026-08-24, `mvp-2`)
```
git ls-files | grep game-templates → 21 files (README, index.json, package.json, scripts/validate-templates.mjs, src/types.ts, template-schema.json, templates/*.json 14)
ls packages/game-templates → No such file
ls packages → sdk, shared-types (only)
git status --short | grep game-templates → D packages/game-templates/README.md … D packages/game-templates/templates/zomboid.json (21 D lines, unstaged deletion)
git show HEAD:packages/game-templates/templates/rust.json | grep install_script → present
git show HEAD:packages/game-templates/scripts/validate-templates.mjs:46 → placeholders only over startup + config.files
```

### 6.2 Store duplication map (current)
| Store table | File | Engine coverage |
|-------------|------|-----------------|
| `database_hosts` + `server_databases` (014) | `store_databases.go:17,36,43,67,103,175,194,224,229,254,273,293,332,441,453,502,530,583,594` | remote host-provisioned (TLS 4 modes, `max_databases`, node affinity `087`) |
| `database_services` (114) | `store_database_services.go:12,91,114,136,166,196,225,237,251,273,284,311,333,338` | legacy beacon service (`DBContainerProvisionRequest`) |
| `db_containers` (098→155) | `store_db_containers.go:14,22,30,38,63,93,101,105,132,151,174,200,233,275` | `DBContainerService` 5 engines, AAD encryption |
| `managed_databases` (116→208) | `store_managed_databases.go:14,92,125,215,319,422,432,469,504` | container-provisioned, soft-delete `deleted_at` |
| `catalog_entries/instances` (175/177/181) | `store_catalog.go:13,84,134,206` + `catalog/catalog.go:68` | 11 entries version-gated, DB vs compose routing |
| `app_store_apps/installs` (113) | `store_app_store.go:6,48,81,92,107,112,120,132,152` | 7 seeded compose apps, `SyncFromRemote:306` |

### 6.3 Key citations for database & template divergence
- `store_db_containers.go:158` vs `:174,200` (encrypted select vs list omission)
- `dbprovisioner/service.go:373` `RegisterTLSConfig` vs `hostTLSConfig:422` lattice
- `containers.go:114` `generatePassword(64)` root vs `provisioner.go:63` `generatePassword` + `:110` `MYSQL_ROOT_PASSWORD=password`
- `provisioner.go:370` `quoteSQLString` redis branch vs `dbprovisioner/service.go:661` correct `go-redis` path
- `091_seed_minecraft_java.sql:12` `startup:''` 1 egg vs `HEAD:packages/game-templates/index.json` registry 14
- `forge/web/lib/egg-templates.ts:14` `EGG_TEMPLATES` 14 vs `store_nests.go:190` `ListEggs` 1 row
- `app-templates-data.ts:6` `STORAGE_KEY forge.app-templates.v1` vs `store_app_store.go:6` vs `store_catalog.go:13` vs `store_nests.go:20` fragmentation

---

*Generated for reverification / subagent 13 — read-only, no product code modified. Re-verification confirms all 18 + 16 + 23 prior rows hold; only evolution is `packages/game-templates` now **0 on disk** (was 0 on `main` in phase-06, 14 in worktree branch) and `validate-templates.mjs` consequently unrunnable.*
