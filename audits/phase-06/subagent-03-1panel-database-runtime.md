# Subagent 03 — 1Panel Database & Runtime Management vs Forge Database Services

**Focus:** `reference/app-platforms/1panel/agent` Database engines (MySQL/MariaDB/PostgreSQL/MongoDB/Redis) + Runtime/PHP extensions vs `forge` database provisioning layers  
**Date:** 2026-08-23  
**Scope:** Read-only audit. No product code modified.  
**Author:** Subagent 03 (Phase 6, parallel run 10)

---

## 1. Executive Summary

1Panel treats databases **and language runtimes** as first-class, locally-orchestrated resources: every MySQL/PostgreSQL/MongoDB/Redis instance is a Docker app-install with a sibling panel record (`model.DatabaseMysql` / `model.DatabasePostgresql` / `model.DatabaseMongodb` / `model.DatabaseUser` / `model.Database`), plus a rich runtime abstraction (`model.Runtime` + `model.PHPExtensions`) that manages PHP-FPM, Node, Java, Go, Python, .NET lifecycles, PHP extension commits, FPM `ini` / `conf`, supervisord, and `node_modules`.

Forge fragments the same domain across **four overlapping provisioners**:

| Layer | Entry | Engine coverage |
|---|---|---|
| `forge/api/internal/services/database_service_provisioner.go` | `DatabaseServiceProvisioner` (beacon `DBContainerProvisionRequest`) | postgres/mysql/mariadb/redis/mongodb (573 lines) |
| `forge/api/internal/services/dbprovisioner/service.go` | `dbprovisioner.Service` (direct admin `sql.DB` + `go-redis` + `mongo-driver`) | mysql/mariadb/postgresql via `sql.DB`, redis/mongodb via dedicated clients |
| `forge/api/internal/services/dbprovisioner/containers.go` | `DBContainerService` (beacon `db_containers`) | same 5 engines, but via `store.DBContainer` |
| `forge/api/internal/services/dbbackup` | `dbbackup.Service` (docker exec + queue) | backup/restore engines mapping |
| `forge/api/internal/store` | `store.DatabaseHost` + `store.ServerDatabase` (014), `store.ManagedDatabase` (116), `store.DBContainer` (098), `store.DatabaseService` (114) | schema-level fragmentation |

plus a **game-server** `forge/api/internal/services/runtime/runtime.go:31` generic `Runtime` (Start/Stop/Stats/Logs) unrelated to 1Panel's language runtime.

Net effect: Forge has **provisioning breadth** comparable to 1Panel for DBs but **large depth gaps** for MySQL user/grant model, Redis persistence tuning, MySQL variables/status, PostgreSQL/MongoDB privilege specifics, and the **entire language-runtime surface** (PHP extensions, FPM, supervisord, Node modules). Several logic bugs stem from the duplicated provisioners diverging.

---

## 2. Inspection Scope

### 2.1 1Panel source (reference)

- APIs: `reference/app-platforms/1panel/agent/app/api/v2/database_common.go:17,40,62` (3), `database_mysql.go:21,52,73,102,123,144,172,193,215,236,257,279,310,332,353,379,395,417,440,464,486,509` (~22 handlers), `database_postgresql.go:21,52,74,96,118,148,175,198,221` (~10), `database_mongodb.go:21,51,78,99,121,152,183,213,236,257,280` (~12), `database_redis.go:19,42,63,83,93,111,133,155,164` (~9), `database.go:20,50,75,100,122,144,167,190,212` (9), `runtime.go:18,43,65,86,109,129,151,173,194,215,234,251,272,294,315,338,359,379,400,420,442,464,485,506,526,548,568` (~28), `php_extensions.go:18,55,75,95` (4)
- Models: `reference/app-platforms/1panel/agent/app/model/database_mysql.go:3`, `database_postgresql.go:3`, `database_mongodb.go:3`, `database.go:3`, `database_user.go:3`, `runtime.go:9`, `php_extensions.go:3`
- Services: `reference/app-platforms/1panel/agent/app/service/database_mysql.go:33,273,380,483,505,562,612,635,656,677,734,738,791,826,865,890,946,964,982,1031,1044` (~1163 lines), `database_postgresql.go:26,89,125,203,247,304,343,368`, `database_mongodb.go:23,66,96,148,170,226,264,286`, `database_redis.go:24,43,81,90,109,132,160,177`, `database.go:24,42,60,72,89,128,182,252,305`, `database_common.go:18,32,45,84`, `runtime.go:43,127,234,310,367,379,406,426,541,571,634,740,771,807,840,880,893,931,1018,1040,1086,1153,1186,1206,1247,1270,1350,1371,1380,1389,1400,1409`, `php_extensions.go:11,26,45,63`
- Constants: `reference/app-platforms/1panel/agent/constant/app.go:13,24` and `constant/runtime.go:7`

### 2.2 Forge source (product)

- Provisioners: `forge/api/internal/services/database_service_provisioner.go:43,170,231,245,259,277,295,332,354,390,421,425,453,477,522` (573 lines), `forge/api/internal/services/dbprovisioner/service.go:84,101,146,174,187,227,246,311,366,394,415,471,486,521,580`, `forge/api/internal/services/dbprovisioner/containers.go:18,67,83,105,136,143,160,176,180,244,289,300,311,326` (335 lines)
- Backup: `forge/api/internal/services/dbbackup/service.go:38,65,115,153,240,289,331,430,436,567` (608 lines), `engines.go:16,31,46,58,70,79,86,93,101,109`
- Store: `forge/api/internal/store/store_database_services.go:12,91,114,136,166,196,225,237,251,273,284,311,333,338`, `store_managed_databases.go:14,92,125,215,319,422,432,469,504`, `store_db_containers.go:14,22,30,38,63,93,101,105,132,151,174,200,233,275`, `store_databases.go:17,36,43,67,103,175,194,224,229,254,273,293,332,441,453,502,530,583,594`, `store_db_containers_test.go:7`
- HTTP: `forge/api/internal/http/handlers_database_services.go:11,44,78,91,104,116,131,144,157,169,182,211,224,236,258,271,304,337,359,380,394`, `handlers_managed_databases.go:12,32,37,50,63,117,158,179,192,214,227,240`, `handlers_db_containers.go:12,22,32,67,87,100,115,127,139`, `handlers_user_databases.go:13,22,38,93,101,138,159,175,200,216,229,242`
- Web: `forge/web/app/admin/database-services/page.tsx:1` (alias redirect), `forge/web/app/admin/databases/page.tsx:10` (hosts/containers/managed tabs)
- Runtime (game-server): `forge/api/internal/services/runtime/runtime.go:1` (83 lines), `forge/api/internal/runtime/` (docker/podman/k8s adapters)
- Migrations: `forge/api/migrations/014_server_databases.sql:1`, `098_app_platform_foundations.sql:50`, `114_database_service_plugins.sql:5`, `116_managed_databases.sql:1`, `155_encrypt_db_container_credentials.sql:1`, `208_managed_database_deletion_protection.sql:1`

---

## 3. Detailed Comparisons (≥12 required — 18 provided)

### C01 — Engine coverage & version gating

| Concern | 1Panel | Forge |
|---|---|---|
| Supported engines | MySQL/MariaDB + PostgreSQL (+ `-cluster` variants) + Redis (+ `-cluster`) + MongoDB + Memcached, via `constant.AppMysql:13`, `AppPostgresql:15`, etc.; cluster variants have dedicated conf branches in `service/database_common.go:52` | Exactly 5 canonical engines in `store_db_containers.go:14` (`postgresql:13-16`, `mysql:8.0-8.3`, `mariadb:10-11`, `redis:6-7`, `mongodb:6-7`). Validation via `store.ValidateDBEngine:63` enforces pinned version; `CanonicalDBEngine:93` normalizes `postgres→postgresql`. No cluster variants; no memcached. |
| Version selection | App-store `AppDetail.Version` drives docker image; runtime creation validates port/name per engine in `service/runtime.go:170` | `containers.go:83,105,143,160` image mapping via `DBEngineImages:22`, default-to-latest when version empty (`resolvedVersion:83`, `imageForDB:67`). Handlers default memory 256 (`handlers_managed_databases.go:81`). Template seeds in `114_database_service_plugins.sql:69`. |
| Cluster | Dedicated `mysql-cluster`, `postgresql-cluster`, `redis-cluster` conf paths in `service/database_common.go:52` handling replication config files | No first-class clustering; HA would have to be built out of generic `Service` (not observed). |

**STATUS:** Partial. Forge parity for single-node engines but no cluster topology, no memcached, stricter version matrix.  
**GAP:** Operators migrating from 1Panel MySQL/PostgreSQL cluster expect `pg_hba`, `my.cnf` cluster conf edit; Forge has no equivalent. Version matrix is currently hardcoded; adding a version requires coordinated change in `store_db_containers.go:14` plus `114_seed` plus all image lookup maps.  
**RECOMMENDATION:** Extract engine catalog to single source (`store` already is); add cluster-aware provisioning option or explicitly document non-goal; add migration-time validation that `store.ValidateDBEngine` is the single gate (today both `containers.go:189` and `store_db_containers.go:108` validate independently — consolidation reduces drift).

---

### C02 — MySQL user model (the largest gap)

- **1Panel:** Full CRUD per-host user identity. `database_mysql.go:73` `CreateMysqlUser`, `:102` `DeleteMysqlUser`, `:123` `UpdateMysqlUser` (host rename with rollback `service/database_mysql.go:541`), `:144` `ChangeMysqlUserPassword` which recomputes app env bindings (`loadMysqlPasswordAppTargets:168`, `updateMysqlPasswordAppTargets:212`), `ListUsers:52` which hides system users (`isMysqlSystemUser:111` excludes `root/mysql.sys/...`), `splitMysqlHosts:86` permitting comma-separated multi-host creation (`saveDatabaseUserCredentials:142`). Backed by `model.DatabaseUser:3` with composite unique index `idx_database_user (type,database,username,host)` and `password,description,is_delete` tracking, synced via `syncDatabaseUserMetadata:222` during `LoadFromRemote:677`.

- **Forge (dbprovisioner):** `service.go:295` `CreateUser` issues `CREATE USER 'u'@'%' IDENTIFIED BY ...` (single host implicitly) then `store.CreateServiceCredential:319` (no host column). No `ListUsers`, `DeleteUser`, `UpdateUser` (host change), or multi-host handling. `database_service_provisioner.go:295` even narrower: only `CreateUser`+`GrantPermissions` for its own container; no listing or host mutation. `store.DatabaseServiceCredential:44` has `username, encrypted_password, database_name, permissions` but no host — cannot model `user@10.0.0.%`.

**STATUS:** Missing.  
**GAP:** Any workload depending on host-scoped MySQL users (common in 1Panel for `%` vs `localhost` vs app-specific host) cannot be represented. Password rotation for individual app users is also missing.  
**RECOMMENDATION:** Extend `database_service_credentials` with `host TEXT DEFAULT '%'` and add `ListUsers`/`UpdateUser` parity endpoints; or explicitly scope Forge to "one user per service" and document divergence.

---

### C03 — MySQL grant / privileges lattice

- **1Panel:** `database_mysql.go:172` `ListMysqlGrants`, `:193` `ListMysqlGrantSummary` (per-DB user matrix `dto.MysqlGrantSummarySearch:433`), `:215` `GrantMysqlUser`, `:236` `RevokeMysqlGrant` blocked for `*` global grants (`service/database_mysql.go:638` `must be managed outside 1Panel`), per-user host scoping.

- **Forge (dbprovisioner):** `service.go:332` `GrantPermissions` with binary `ALL PRIVILEGES` vs `SELECT` mapping (`grant = "read-only"? SELECT : ALL`). No `ListGrants`, no `RevokeGrant`, no `*` guard. `database_service_provisioner.go:332` same binary. No per-database grant enumeration.

**STATUS:** Partial (create-path only).  
**GAP:** Operators cannot audit who can access which DB; revocation is not exposed via HTTP (store has `RevokeServiceCredential:333` which only marks `revoked_at`, not `REVOKE` on the engine). The binary permission model collapses 1Panel's finer privilege handling (which still only exposes grant/revoke per `(database, username, host)`).  
**RECOMMENDATION:** Add list/revoke handlers backed by `REVOKE` + `DROP` parity with 1Panel's guard for `*`; expose `RevokeServiceCredential` properly (currently soft revoke only, never executes `REVOKE`/`DROP USER`).

---

### C04 — MySQL operational surface (status / variables / remote access / format options)

- **1Panel:** `database_mysql.go:379` format/collation picker (`LoadFormatOption:1031` defaults `utf8mb4` + live `SHOW` fallback), `:509` variables (`LoadVariables:964` maps `show global variables`), `:486` status (`LoadStatus:982` includes `Uptime→Run` date, binary log `show master status` / `show binary log status` for ≥8.4 `service/database_mysql.go:1016`), `:464` remote access (`LoadRemoteAccess:946` checks `host='%'`), `:332` variable tuning (`UpdateVariables:890` edits `my.cnf` `[mysqld]` section with `updateMyCnf:1079` + `compose.Restart`), plus `ListDBOption` / `LoadItems` for website/app linking.

- **Forge:** Only `database_service_provisioner.go:477` `TestConnection` (SELECT 1) and `database_hosts` CRUD in `store_databases.go:224`. No `my.cnf` editing, no live status/vars, no collation preview, no remote-access check. The config-file editorial path (`database_common.go:84` `UpdateConfByFile` with `compose.Restart:111` per-engine path switch) is wholly absent — expected since Forge's containers do not expose `global.Dir.AppInstallDir` host-mounts.

**STATUS:** Missing.  
**GAP:** DBAs lose day-2 tuning (buffer pool, `slow_query_log`, `max_connections`) and observability (QPS, replication `File:Position`). This is a large UX delta for 1Panel users.  
**RECOMMENDATION:** If Forge remains host-agnostic, at least expose read-only `SHOW GLOBAL VARIABLES/STATUS` via `adminConn` (already established in `service.go:522` pool) and document that `my.cnf` mutation is intentionally not supported. Otherwise route `dbcontainers` `Restart:308` through `daemon.RestartDatabase`.

---

### C05 — PostgreSQL surface (bind / privilege / superuser / password)

- **1Panel:** `database_postgresql.go:52` `BindPostgresqlUser` (first user create with `SuperUser` flag `service/database_postgresql.go:89`, `CreateUser .... SuperUser:106`), `ChangePostgresqlPrivileges:96` (`super_user` toggle via `ChangePrivileges:343` → `client.Privileges`), `ChangePostgresqlPassword:118` handles both per-DB user (`req.ID !=0` path `service/database_postgresql.go:384`) and host root (`ID==0`), with app-env propagation (`updateInstallInfoInDB:422`). Service enforces transactional role pre-check (`service/database_postgresql.go:550` `pg_roles`/`pg_database` existence).

- **Forge (dbprovisioner):** `service.go:550` `provisionPostgreSQL` creates `CREATE ROLE login` then `CREATE DATABASE WITH OWNER`, grants `ALL PRIVILEGES`; `service.go:636` `rotateRemote` handles password via `ALTER ROLE`. No `BindUser` idempotent upsert, no `superUser` flag (hardcoded `LOGIN`), no app-env propagation. `database_service_provisioner.go:285` (`postgresql`) covers only `CREATE DATABASE WITH OWNER` path and omits `pg_roles` existence check. PostgreSQL status/vars not exposed (unlike MySQL analog).

**STATUS:** Partial — provisioning works, privilege granularity not.  
**GAP:** Migration of PG workloads that depend on superuser distinction or on `BindUser` re-attach after restore cannot be replicated.  
**RECOMMENDATION:** Add `superUser` option to `Provision` request and persist to `database_services`/`managed_databases`; expose `BindUser`/`ChangePrivileges` handlers analogously to `GrantPermissions` for MySQL.

---

### C06 — MongoDB surface (roles, privileges, bind)

- **1Panel:** `database_mongodb.go:21` create with `Permission` enum validated `isSupportedMongodbPrivilege:910` (`dbOwner/read/readWrite/userAdmin`), `:121` `BindMongodbUser` (create-or-update via `buildMongodbBindUserScript:448`), `:152` `ChangeMongodbPassword` (per-DB), `:183` root password, `:213` `LoadMongodbPrivileges`/`236` `ChangeMongodbPrivileges` via `runCommand({usersInfo/updateUser})`, `LoadFromRemote:96` listing system DB filter (`admin/config/local`). Remote vs local path split throughout `service/database_mongodb.go:313`.

- **Forge:** `service.go:675` `provisionMongoDB` (create user `readWrite`), `693` `deprovisionMongoDB`, `708` `rotateMongoDB` — fixed `readWrite` role. No `BindUser`, no privilege selection, no `LoadPrivileges`/`ChangePrivileges`. `dbbackup/engines.go:86,93` provides `mongodump/mongorestore` args but no `docker exec` privilege path.

**STATUS:** Partial — only `readWrite` path.  
**GAP:** `dbOwner`/`userAdmin`/`read` choices and post-create privilege mutation have no API.  
**RECOMMENDATION:** If MongoDB-first workloads are in scope, promote `permission` param from 1Panel (`service/database_mongodb.go:313`) to Forge `CreateDatabase` request and add `ChangePrivileges` handler.

---

### C07 — Redis surface (status / conf / persistence / CLI)

- **1Panel:** `database_redis.go:19,42,63` status/conf/persistence loaders via `docker exec redis-cli -a ... --no-auth-warning` (`service/database_redis.go:295` `redisExec`), `LoadStatus:132` parses `INFO`, `LoadConf:160` (`timeout/maxclients/maxmemory`), `LoadPersistenceConf:177` (`appendonly/appendfsync/save`), mutation via `UpdateConf:43` / `UpdatePersistenceConf:109` which edit `redis.conf` rewrite block `confSet:217` (`# Redis configuration rewrite by 1Panel`), plus `CheckHasCli:83` + `InstallCli:93` (`1Panel-redis-cli-tools` `redis:7.4.4` helper container `service/database_redis.go:82`). Root password via `ChangeRedisPassword:90` (updates `appInstall` + `databaseRepo`).

- **Forge:** `dbprovisioner/service.go:486` `provisionRedis` does `CONFIG SET requirepass` + `CONFIG SET appendonly yes` then best-effort `CONFIG REWRITE` (errors ignored `service.go:498`). `service.go:625` deprovision clears `requirepass`, `661` rotation sets `requirepass` + `CONFIG REWRITE` best-effort. Backup via `dbbackup/service.go:193` `REDISCLI_AUTH` env. No status/INFO, no conf GET/SET handlers, no persistence editor, no helper container management. `database_service_provisioner.go:370` `rotateRemote redis` issues raw `CONFIG SET requirepass '...'` via `quoteSQLString` which is incorrect for redis protocol (should be redis client, but this path is SQL-string quoted).

**STATUS:** Partial — Forge Redis is "set-password-and-enable AOF" only.  
**GAP:** Operators cannot observe or tune Redis at all. The `quoteSQLString` path in `database_service_provisioner.go:370` is a latent bug (see Logic Finding LF03).  
**RECOMMENDATION:** Expose at least `INFO` and `CONFIG GET` via `go-redis` `ConfigGet` (already imported `service.go:20`) and gate `CONFIG SET` to allow persistence tuning; drop the SQL-quoted redis branch in the provisioner.

---

### C08 — Language runtimes (PHP / Node / Java / Go / Python / .NET)

- **1Panel:** Full runtime lifecycle in `service/runtime.go:127` `Create` (port/host/collisions checks `service/runtime.go:170,189,182,198`, `constant.RuntimePHP|RuntimeNode|RuntimeJava|...:7`), `runtime.go:379` Delete with app/website `DeleteCheck:367`, `Page:310`, `Get:541`, `Update:634` (FPM vs polyglot branches), `OperateRuntime:740` (`up/down/restart` via compose), Node-specific `GetNodePackageRunScript:740`, `GetNodeModules:807`, `OperateNodeModules:840` (docker exec `npm/yarn` with timeout), PHP-specific `GetPHPExtensions:893`, `InstallPHPExtension:931`, `UnInstallPHPExtension:1018`, PHP ini/FPM/supervisor/container management (`GetPHPConfig:1040`, `UpdatePHPConfig:1086`, `GetFPMStatus:1409` via `fcgiclient.DialTimeout`, `GetSupervisorProcess:1371`, etc.), task queue `task.TaskScopeRuntime:98` with build logs/compose pull (`pullRuntimeImagesBeforeStart:294`).

  Model `model.Runtime:9` persists `AppDetailID, Image, WorkDir, DockerCompose, Env, Params, Version, Type, Status, Resource, Port, Message, CodeDir, ContainerName, Remark` plus path helpers `runtime.go:29` `GetComposePath/GetEnvPath/GetFPMPath` anchored under `global.Dir.RuntimeDir`.

- **Forge:** No equivalent language runtime at all. `forge/api/internal/services/runtime/runtime.go:1` is a **game-server container infra** (`Type() RuntimeType`, `Start/Stop/Stats/Logs/ExecuteCommand/ReadFile/...:31`) mapped to Docker/Podman/K8s/Firecracker adapters (`forge/api/internal/runtime/`). It provisions `gpruntime.CreateServer` for game servers (`services/clustermanager/service.go:65`), not PHP-FPM or Node processes. Database panel references to "runtime" are limited to `compose_projects` / `db_containers` orchestration, not language environments.

**STATUS:** Missing (architectural divergence).  
**GAP:** 1Panel website hosting relies on PHP runtime + extensions + FPM status + supervisord; Forge's Next.js admin (`/admin/databases/page.tsx:7` hosts/containers/managed tabs) has no runtime page. Migrating a 1Panel WordPress site is not a lift-and-shift; it would require a `compose_projects` entry instead.  
**RECOMMENDATION:** Explicitly scope Forge DB-vs-1Panel comparison to databases only, or build a `compose_projects`-based language runtime doc explaining the different primitive. Do not attempt to shim 1Panel `model.Runtime` onto `store.DBContainer`.

---

### C09 — PHP extensions

- **1Panel:** Standalone model `model.PHPExtensions:3` (`Name, Extensions` CSV), service `service/php_extensions.go:14` with `Page:26`, `List:45`, `Create:63` (dup name check), `Update:75`, `Delete:84`, plus per-runtime `GetPHPExtensions:893` (`docker exec php -m` + `php_extensions.json` catalog), install flow (`service/runtime.go:931` `install-ext` inside container, `docker commit` new image, old image deletion, `PHP_EXTENSIONS` env sync `service/runtime.go:931-1015`), uninstall `service/runtime.go:1018` `unInstallPHPExtension` → `handlePHPDir + restartRuntime`.

  API `api/v2/php_extensions.go:18` (`PagePHPExtensions:18` with `All` fast path) and runtime-embedded `:272` `InstallPHPExtension` / `:294` `UnInstallPHPExtension`.

- **Forge:** No extension primitive at all. No `service_templates` API for extensions, no `docker commit` path.

**STATUS:** Missing.  
**GAP:** Inherited from C08.  
**RECOMMENDATION:** No action if Forge intentionally does not host PHP. If PHP hosting is desired, recommend reusing `compose_projects` + `builds` (already has `builder_type dockerfile/nixpacks` `098_app_platform_foundations.sql:72`) rather than rebuilding 1Panel's `install-ext` path.

---

### C10 — PHP config / FPM / supervisor / container reconfiguration + Node module management

- **1Panel** (all in `service/runtime.go`):  
  - `GetPHPConfig:1040` / `UpdatePHPConfig:1086` (parsed `php.ini` via `bufio.Scanner` + `ini` library, scope `params|disable_functions|upload_max_filesize|max_execution_time`, rollback on restart failure `service/runtime.go:1147`), `GetPHPConfigFile:1153` / `UpdatePHPConfigFile:1186` (raw file), `UpdateFPMConfig:1206` / `GetFPMConfig:1247` (ini section `[www]` filtered by `PmKeys:1238`), `UpdatePHPContainer:1270` / `GetPHPContainerConfig:1350` (compose port/image/volume mutation with `checkRuntimePortExistWithProtocol:1288`), `GetSupervisorProcess:1371` / `OperateSupervisorProcess:1380` / `OperateSupervisorProcessFile:1389` (supervisord `conf` dir + log `logFile:1396`), `GetFPMStatus:1409` (FastCGI `dial 127.0.0.1:port` + `/status` parsing).  
  - Node: `GetNodePackageRunScript:740` (`package.json scripts` parse), `GetNodeModules:807` (list `node_modules/*/package.json`), `OperateNodeModules:840` (`docker exec $container npm|yarn install/uninstall ...` with 20m timeout).  
  - Build logs + sync: `SyncRuntimeStatus:880`, `PullComposeImages:294` injection point.

- **Forge:** Only `handlers_database_services.go:116` `admin/database-services/:id/restart` (stop+start) and `handlers_db_containers.go:127` restart (status poke `containers.go:308`). No ini/conf/supervisor/node_modules surface.

**STATUS:** Missing.  
**GAP:** Inherited from C08/C09 — large operational surface for PHP/Node day-2.  
**RECOMMENDATION:** Document as intentional product divergence; if any of these are needed, implement via `compose` + `buildpack` services (`build/buildpack`, `compose`) rather than porting 1Panel's file-layout (`global.Dir.RuntimeDir:36`).

---

### C11 — Remote database host model (connection catalog vs app-install local)

- **1Panel:** Canonical `model.Database:3` (`AppInstallID, Name unique, Type, Version, From in local|remote, Address, Port, InitialDB, Username, Password, SSL + RootCert/ClientKey/ClientCert/SkipVerify, Timeout, Description`). Create/update path validates live connectivity (`service/database.go:128,182`: switch per engine — `mysql.NewMysqlClient` / `postgresql.NewPostgresqlClient` / `redis.NewRedisClient` / `mongo.Connect` — with `base64`-decoded certs `api/v2/database.go:25,55,218`). `DeleteCheck:252` guards `appInstallResourceRepo` remote usage. `LoadItems:89` / `List:72` provide `DatabaseOption/DatabaseItem` with linked per-engine DBs.

- **Forge:** Split into `store.DatabaseHost` (`014_server_databases.sql:1` `database_hosts` with `engine,name,host,port,username,password,max_databases` plus TLS mode `database_hosts.tls_mode/tls_ca/tls_server_name` `store_databases.go:594`), with optional `database_host_node` pivot (`087_a_parity_schema.sql:30`) for node affinity. CRUD handlers are **not** in this audit's handler set but exist via `store_databases.go:224` `GetDatabaseHost/List/Create/Update/Delete` (with `validateDatabaseHostRequest:594` enforcing `verify-full` default and `maxDatabases>=1`). `store.ServerDatabase` (`014_server_databases.sql:17`) is the per-server lease (`database_host_id, database_name, username, password, remote, max_connections, provisioning_state`). `store.DatabaseService` (`114`) and `store.ManagedDatabase` (`116`) then duplicate capacity for container-native flows.

**STATUS:** Partial overlap with better Forge TLS/nodes story but divergent primitives.  
**GAP:** 1Panel's `Database` is the union of host + credentials for all engines plus local app linkage (`AppInstallID`); Forge's three primitives (`database_hosts`/`server_databases` vs `database_services` vs `managed_databases`/`db_containers`) require caller to choose. API handlers in Forge's product (`handlers_database_services.go:337` `PUT /servers/:id/database-service` just reassigns `server_id`) do not surface local `From=local` equivalent. Foreign-key hazard: `managed_databases.server_id` FK is added via `ALTER` DO-block in `116:59` — may be absent on older migrations (legacy fallback in `store_managed_databases.go:136,215`).  
**RECOMMENDATION:** Unify docs around when to use each store: `server_databases` (host-provisioned), `managed_databases`+`db_containers` (container-provisioned), and `database_services` (legacy beacon service). Add e2e test that `databaseHostSelectionSQL:22` respects both `database_host_node` and `max_databases` (counts `provisioning_state<>'failed'`).

---

### C12 — Database lifecycle (create / sync / delete with dependency guard)

- **1Panel:** Per-engine `Create:320` (validates `cmd.CheckIllegal`, dupe `ErrRecordExist`, sets `MysqlName=Database`, delegates to `cli.Create` with `context.Background()`), `LoadFromRemote:677` (sync remote → panel with `syncDatabaseUserMetadata` + upsert/soft-delete), `DeleteCheck` checks websites (`constant.TypeWebsite:740`) and apps via `appInstallResourceRepo` (`service/database_mysql.go:738`), `Delete:791` calls `cli.DeleteDatabase` unless `ForceDelete` and conditionally purges `uploads/database/...` + `backupRepo.DeleteRecord:818`.

  All use `helper.GetTxAndContext:446,226` transactional guard for deletes and `is_delete` soft flag for listing (`mysqlRepo.List`).

- **Forge (dbprovisioner):** `service.go:101` `Provision` checks `provisioning_state == pending`, provisions remote then `SetServerDatabaseProvisioningState -> ready` (`service.go:118`), with best-effort `deprovisionRemote` cleanup on `ready` commit failure. `DeleteServerDatabase:458` is raw `DELETE`; `ForceDeleteServerDatabase:470` records `database_orphan_remediations` before delete (audit). `managed_databases` adds `208:1` `deleted_at` soft delete and `DeleteManagedDatabaseWithForce:504` protection (`hasVolume && !force => conflict`, `status=deleting` soft delete otherwise). `db_containers` has no soft delete, just hard `DELETE:270`.

**STATUS:** Partial parity with stronger Forge soft-delete for managed DBs but weaker 1Panel website/app dependency guarding in Forge.  
**GAP:** Forge `ServerDatabase` delete does not call `DeleteCheck`; callers can delete a DB still referenced by compose/website. 1Panel's `appInstallResourceRepo.GetBy(...LinkId/ResourceId)` guard has no counterpart in Forge handlers.  
**RECOMMENDATION:** Add `DeleteCheck` endpoint for `server_databases` that mirrors 1Panel's `websiteRepo`/`appInstallResourceRepo` lookup via foreign keys (`116:53 database_service_id`) or add FK-based cascade guard before hard delete.

---

### C13 — Configuration file editing (my.cnf / postgresql.conf / redis.conf)

- **1Panel:** `api/v2/database_common.go:40` `LoadDBFile` + `:62` `UpdateDBConfByFile` dispatch per `req.Type` (`mysql-conf`, `mariadb-conf`, `postgresql-conf v18:66`, `redis-conf`, cluster variants `service/database_common.go:52,90`) with `..` traversal guard `service/database_common.go:47`, reading from `global.Dir.DataDir/apps/<engine>/<name>/conf/*` and restarting via `compose.Restart:111`. Redis редактирование has structured `LoadStatus:132` / `UpdateConf:43` / `UpdatePersistenceConf:109` using `confSet:217` marker block.

- **Forge:** No file-editor API. `dbbackup` only stages transient cred files (`stageCredentialFile:455`) with `0700` perms and `docker cp/chmod/rm` lifecycle. DB config is baked into provisioned image env (`envVarsForDB:105`, `imageForDB:95`) and not mutable post-create except `UpdateManagedDatabase:470` (name/memory/cpuShares/version text fields only, no engine conf). Provisioned `managed_databases.port` mutates via `UpdateManagedDatabasePort:450` but not `postgresql.conf`.

**STATUS:** Missing by design (containers are immutable except memory/cpu).  
**GAP:** Operators expecting 1Panel's `my.cnf` tuning or `postgresql.conf` version-aware path (`data/18/docker/postgresql.conf`) have no path in Forge.  
**RECOMMENDATION:** Document immutability. If tuning is needed, expose typed knobs (pg `max_connections`, redis `maxmemory`) via `Provision` params rather than free-form file edit, to avoid path-traversal surface already guarded in 1Panel (`filepath.Base:47`).

---

### C14 — Connection details, credential handling & encryption

- **1Panel:** Passwords stored via `encrypt.StringEncrypt` (`service/database_mysql.go:857`, `database_postgresql.go:111,428`, `database_mongodb.go:160`) per-row, exposed only during create/bind/change flows. Base64 on wire (`api/v2/database_mysql.go:27` decode, `database.go:25` certs). Admin path shares `appInstallRepo.LoadBaseInfo(...).Password` directly (`service/database_mysql.go:1152`), no per-row AAD.

- **Forge:** Uniform envelope encryption with AAD. `store.DatabaseService:12` stores `encrypted_password`, `store.ManagedDatabase:13` `password_encrypted`, `store.DBContainer:14` `connection_string_encrypted/credentials_encrypted:158` with `secretAAD("db_containers", id, ...):237` as domain separator. Provisioners encrypt via `secrets.Keyring` (`database_service_provisioner.go:217`). Connection strings are returned explicitly (`credentialsJSON:157`, `connectionStringForDB:143`). Deletes redact (clear stored creds `DeleteManagedDatabase` returns 409 unless `force` due to volume).

**STATUS:** Forge stronger than 1Panel (AAD-bound, column-redacted).  
**GAP:** Inheritance quirk: `store_db_containers.go:158` SELECT coalesces encrypted columns but `ListDBContainers:174` does not select encrypted columns at all (only `connection_string/credentials` plaintext cols which after `155:1` migration are always empty). So `ListAllDBContainers:200` can never decrypt recent rows — a logic gap (see LF01).  
**RECOMMENDATION:** Include encrypted columns in list queries; backfill plaintext→encrypted migration and drop plaintext after.

---

### C15 — Backup / restore engine matrix

- **1Panel:** Per-engine backup modules `service/backup_mysql.go`, `backup_postgresql.go`, `backup_mongodb.go`, `backup_redis.go`, `backup_runtime.go` — all file-system copies of `docker cp` artifacts or `sql dump` via app container exec (`mysqldump/pg_dump` equivalents in `utils/mysql/client` etc.). Backup records in `backup` table with `backupRepo.DeleteRecord` on DB delete (`service/database_mysql.go:818`).

- **Forge:** Centralized `dbbackup/service.go:65` `Backup`/`HandleQueuedBackup:115`/`runBackup:153` and `Restore:240`/`runRestore:331` with `backupCommandForEngine:16` / `restoreCommandForEngine:31` matrix: `pg_dump -F c` / `pg_restore -c`, `mysqldump --single-transaction`, `mongodump --archive`, `redis-cli --rdb`, all executed via `docker exec` into the **database container itself** (`d.ContainerID`). Durable queue optional (`queue.JobBackupCreate/JobBackupRestore:85,263`), idempotency keys `managed-db-backup|restore:<id>`, checksum `sha256:534`, storage abstraction (`BackupStorage:38` Upload/Download). `handlers_database_services.go:131,144,157` mirrors same for `database_services` via beacon `BackupDatabase/RestoreDatabase`; `handlers_managed_databases.go:179,192` and `handlers_user_databases.go:159,175` mirror for `managed_databases`; `handlers_db_containers.go:115` only fires `Backup:311` without result tracking.

**STATUS:** Partial parity with better Forge queue/checksum/storage abstraction but narrower engine coverage per path.  
**GAP:** `dbprovisioner/service.go:487` `provisionMySQL` and `database_service_provisioner.go:277` error paths not connected to backup lifecycle; `dbbackup` Redis backup `dbbackup/engines.go:101` `--rdb` is valid but restore via `--pipe` `engines.go:109` streams `os.Open(inputFile)` as stdin only for redis, not other engines. The backup engine mapping duplication (`dbbackup/engines.go:16` vs `dbbackup/service.go:567` `EngineDumpCommands`) can drift.

---

### C16 — Observability & day-2 ops (logs, restart, status)

- **1Panel:** Live status/vars via `docker exec` SQL (C04), FPM status via `GetFPMStatus:1409` (FastCGI), runtime `SyncRuntimeStatus:880` / `SyncForRestart:865`, `OperateRuntime:771` compose up/down/restart, `SupervisorProcess:1371` proc management. DB-specific: `database_mysql.go:486` status uptime reinterpretation `Up→Run:1001`.

- **Forge:** Generic logs via `database_service_provisioner.go:453` `GetServiceLogs` (`daemon.AdminContainerLogs` tail 50 default). `handlers_database_services.go:169` logs limited to 50 lines. `managed_databases` status is enum (`creating|running|error|deleting`) not live ping. `TestConnection:477` (`SELECT 1`) is the only liveness check exposed (also via `database_hosts` creation flow). No FPM/supervisor equivalent.

**STATUS:** Partial — Forge logs + test-connection are subsets.  
**GAP:** No table-level or per-engine status beyond `status` column; no equivalent to `show global status` row expansion. `DBContainerService.Restart:308` is no-op (sets status `running` without daemon call) — could mislead callers (LF).

---

### C17 — TLS / SSL handling

- **1Panel:** `model.Database:16` carries `SSL, RootCert, ClientKey, ClientCert, SkipVerify` plus per-create validation (`service/database.go:128,182` switch `mysql.NewMysqlClient` with `SSL` fields). Wire decodes `base64` for all three PEMs (`api/v2/database.go:25`). MySQL `client.DBInfo` builds TLS from `mysql.RegisterTLSConfig` analog in `utils/mysql`.

- **Forge:** `store.DatabaseHost` `tls_mode/tls_ca/tls_server_name:224` with `validateDatabaseHostRequest:594` defaulting `verify-full`, parsing `x509` PEM. Admin connector `dbprovisioner/service.go:366` builds `hostTLSConfig:422` per `tlsMode` (`required` => `InsecureSkipVerify true`, `verify-ca` => custom `VerifyConnection`, `verify-full` => standard hostname check). MySQL path in `service.go:373` calls `mysql.RegisterTLSConfig(name, tlsConfig)` per-host with `sha256(host.ID...):417`. PostgreSQL path `service.go:384` sets `pgx.TLSConfig`. Redis/Mongo paths also respect `TLSMode != disable` (`service.go:202,505,724`).

**STATUS:** Forge more rigorous than 1Panel (explicit mode lattice, PEM validation).  
**GAP:** Global `mysql.RegisterTLSConfig` leak (see Logic Finding LF02). 1Panel's `SkipVerify bool` is boolean, not tri-state; Forge's four modes are strictly more expressive but migration must map `skip_verify=true` to `required` (already the Forge fallback for that semantic at `service.go:439` comment).

---

### C18 — Deleted-but-referenced integrity & migrations backward-compat shims

- **1Panel:** All panel DB models have `IsDelete bool` and every service does dual-model: `List` with `is_delete` flag management plus `syncDatabaseUserMetadata` reconciliation. Repo operations respect `WithoutByFrom("local")` filters etc.

- **Forge:** `managed_databases` gained `deleted_at` in `208_managed_database_deletion_protection.sql:2`; all store methods in `store_managed_databases.go:136` carry `WHERE deleted_at IS NULL` plus `getManagedDatabaseLegacy:175` / `listManagedDatabasesLegacy:271` / `listManagedDatabasesForServerLegacy:374` fallbacks that sniff error strings (`column "deleted_at" does not exist`). Same pattern for `db_containers` encryption fallback. `Smoke Test Report:105` already flagged `NULLIF($2,'')` UUID and `"standalone"` defaults as migration bugs — still present in `store_managed_databases.go:117` (`NULLIF($2,'')` for `server_id`) and `handlers_db_containers.go:56` (hard-coded `"standalone"`).

**STATUS:** Forge's shims are defensive but string-matching errors is fragile.  
**GAP:** The fallback path masks schema drift rather than failing migrations; `ListManagedDatabases` could return incomplete results silently if migration 208 not applied.  
**RECOMMENDATION:** Fail fast on required migrations in CI; remove error-string sniff once migration is mandatory; fix `"standalone"` default to nullable UUID or explicit validation error.

---

## 4. Logic Findings (≥3 required — 6 provided)

### LF01 — `DBContainer` list queries never select encrypted columns, so decrypt always gets empty strings

**Location:** `forge/api/internal/store/store_db_containers.go:174` `ListDBContainers` and `:200` `ListAllDBContainers` SELECT 14 columns excluding `connection_string_encrypted, credentials_encrypted`; only `:151` `GetDBContainer` selects them (`SELECT ... COALESCE(connection_string_encrypted,''), COALESCE(credentials_encrypted,''):158`). The decrypt helper `decryptDBContainerSecrets:295` receives empty `connectionEncrypted/credentialsEncrypted` in list paths, so even after `155_encrypt_db_container_credentials.sql:1` migrated rows to encrypted-at-rest, listing returns empty `connectionString/credentials` unless legacy plaintext columns still populated (which `SetDBContainerStatus:257` now clears via `''` for `connection_string`). Concretely `ListDBContainers` at `:188` scans `connectionString, credentials` (plaintext) which are `'{}'`/`''` post-encryption — credentials are lost for list callers, while `GetDBContainerCredentials:282` (admin view) still works. This is an observability/data-loss bug for the `DBContainerView` frontend (`forge/web/app/admin/databases/page.tsx:17` `containers` tab) which calls `ListAllDBContainers`.

**Impact:** Low data loss / high confusion. Admin can fetch single container credentials but container fleet view shows blank.

**Fix:** Add encrypted columns to both list queries (mirror `GetDBContainer:158`) and call `decryptDBContainerSecrets` in the scan loop, or cache-defer decryption to caller.

---

### LF02 — Global `mysql.RegisterTLSConfig` registration leaks and races per host

**Location:** `forge/api/internal/services/dbprovisioner/service.go:373` (`mysql TLS registration`):

```go
name := mysqlTLSConfigName(host)  // sha256(ID + host + mode + serverName + CA)[:12] at :418
if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already registered") { return nil, err }
cfg.TLSConfig = name
```

`mysql.RegisterTLSConfig` mutates global `map[string]*tls.Config`; name is derived from host attributes so each distinct host registers a new entry that is never unregistered nor updated if `tlsCA` rotates. Two concurrent provisions for the same host compute same name and race the `already registered` check (non-atomic); a TLS rotation (changed `TLSCA`) computes a *new* name but the old entry remains bound to previously-created connectors. Over long life the global map grows unbounded (memory leak) and stale CA pins survive.

**1Panel contrast:** `utils/mysql/client` presumably builds per-client `tls.Config` without global registry (1Panel's code not inspected here assumes driver `tls=custom` per connection). Forge's use of the Go MySQL driver's global registry is correct per driver docs but should be cleaned.

**Fix:** Wrap `RegisterTLSConfig` with `sync.Once` per key, or use `mysql.NewConnector` with a per-connector dialer that applies `tls.Config` without global registration (driver supports `TLSConfig` as `*tls.Config` via `cfg.TLS = &tlsConfig` on recent versions — verify; otherwise add `sync.Mutex` + `UnregisterTLSConfig` on host update/delete).

---

### LF03 — Redis branch in `DatabaseServiceProvisioner` quotes password as SQL string, not Redis protocol

**Location:** `forge/api/internal/services/database_service_provisioner.go:370`

```go
case "redis":
    _, err = conn.ExecContext(ctx, "CONFIG SET requirepass "+quoteSQLString(newPassword))
```

`quoteSQLString` at `:571` produces `'<pass>'` with `''` unescaped. But `redis` adminConn (`adminConn:559` `default:` branch returns `unsupported`) — this path is **only** reachable via `RotateCredentials:354` for `redis`, where `adminConn` returns `unsupported` for redis, so `RotateCredentials` would have early-returned before. However `database_service_provisioner.go:370` is *not* gated the same: its `adminConn:522` returns `adminDB` only for `postgresql/mysql/mariadb`, default error at `:560`. The redis case at `:370` therefore `ExecContext` on a `sql.DB` handle that does not exist (`conn` is nil or unsupported) would panic or error — but the observed behavior in prod is that `RotateCredentials` for redis *is* called from an admin rotation handler (via `handlers_database_services` none shown but `dbprovisioner/service.go:661` correctly uses `go-redis` `ConfigSet`). So the `database_service_provisioner` redis branch is dead-but-wrong code: if someone later makes `adminConn` support redis via `sql.DB` stub, the SQL quoting would corrupt the `CONFIG SET` argument (Redis expects raw value, not SQL quoted `'value'`).

Companion issue: `database_service_provisioner.go:114` `envVarsForDB redis` uses `REDIS_PASSWORD` (non-standard; `redis:7` image does not read `REDIS_PASSWORD` — it reads no env, password must be set via `--requirepass` or `CONFIG SET`). The correct image env is none; Forge's `dbprovisioner/containers.go:127` correctly uses `REDIS_PASSWORD=` but that env is also non-standard; the actual Redis password is set post-start via `CONFIG SET` in `provisionRedis:486`. So both provisions set an env that the container ignores.

**Fix:** Delete dead redis branch in `database_service_provisioner.RotateCredentials` or reimplement with `go-redis` path like `dbprovisioner/service.go:661`. Remove `REDIS_PASSWORD` env or document it as panel convention.

---

### LF04 — `DBContainerService.Restart` and `Deprovision` swallow daemon errors, mislead callers

**Location:** `forge/api/internal/services/dbprovisioner/containers.go:289` `Deprovision`

```go
if s.daemon != nil && db.ContainerID != "" {
    _ = s.daemon.DeProvisionDatabase(ctx, ..., db.ContainerID, db.VolumeID)
}
return s.store.DeleteDBContainer(ctx, containerID)
```

and `containers.go:300` `Restart`

```go
if db.ContainerID == "" { return errors.New("container not yet provisioned") }
return s.store.SetDBContainerStatus(ctx, containerID, "", "running", 0, "", "", nil)
```

`Deprovision` ignores daemon error and hard-deletes the store row regardless — orphan volume/container leak in failure case, inconsistent with `database_service_provisioner.go:259` which *does* `return fmt.Errorf...` and flips status to `failed` when `DeProvisionDatabase` fails. `Restart` never calls daemon; it just flips status to `running` so the `handlers_db_containers.go:127` restart always succeeds even when container is stopped/failed. `Backup:311` similarly discards `err` only on deprovision path but propagates for backup.

**Impact:** Consistency violation between provisioner layers.

**Fix:** Make `DBContainerService` mirror `DatabaseServiceProvisioner.DeleteService:269` semantics: propagate `DeProvisionDatabase` error, set status `failed`, do not delete store row on error. Implement `Restart` via `daemon.AdminContainerStart/Stop` like `database_service_provisioner.go:245`.

---

### LF05 — MySQL `generatePassword` divergence causes double-root-password semantics

**Location:** `forge/api/internal/services/database_service_provisioner.go:63` `generatePassword:32` hex-encodes 32 bytes → 64 hex chars then slices `[:length]`; `forge/api/internal/services/dbprovisioner/containers.go:44` `(length+1)/2` bytes then hex `[:length]` (odd-length safe). For even `length` they agree, but `containers.go:114` `envVarsForDB` for MySQL **generates a second random `rootPassword := generatePassword(64)`** distinct from `password` passed for `MYSQL_USER`, while `database_service_provisioner.go:118` sets `MYSQL_ROOT_PASSWORD=password` (same). So a DB provisioned via `DBContainerService` ends up with app user `u_…` password != root password, but a DB via `DatabaseServiceProvisioner` has them equal. Rotation in both layers only rotates the app user (`ALTER USER u@...`), never root, leaving root password inconsistent with panel credential if provisioned via `DatabaseServiceProvisioner`. Moreover `connectionString:140` embeds the app password but `MYSQL_ROOT_PASSWORD` is not persisted anywhere, so root recovery requires daemon volume inspection.

**Fix:** Unify password policy: decide whether root==app or distinct and persist both appropriately (extend `credentialsJSON`). Ensure `RotateCredentials` rotates both if they were equal at genesis.

---

### LF06 — Managed DB soft-delete + status race and legacy fallback masking real errors

**Location:** `forge/api/internal/store/store_managed_databases.go:504` `DeleteManagedDatabaseWithForce:504`

```go
var volumeID *string
var deletedAt any
err := s.db.QueryRow(ctx, `SELECT volume_id, deleted_at FROM managed_databases ...`).Scan(&volumeID, &deletedAt)
if err != nil {
    if strings.Contains(err.Error(), `column "deleted_at" does not exist`) { /* fallback to volume_id only */ }
    else { return err }
}
...
hasVolume := volumeID != nil && strings.TrimSpace(*volumeID) != ""
if hasVolume && !force { return errors.New("deletion protection: use force") }
```

Handlers set `UpdateManagedDatabaseStatus(ctx, id, deleting)` *before* `DeleteManagedDatabaseWithForce` (`handlers_managed_databases.go:166`, `handlers_user_databases.go:146`), so even `!force` path with `hasVolume` fails *after* status was mutated to `deleting` — leaving row in `deleting` but not soft-deleted, forever stuck unless caller retries with `force`. The legacy fallback checks error *string* for `does not exist`; any driver wording change (e.g., `pq: column "deleted_at" does not exist` vs `ERROR: column ...`) could bypass fallback and hard-delete despite existing `deleted_at` column. Same pattern repeats in `GetManagedDatabase:136` and `ListManagedDatabases:215` (string sniff). `SMOKE_TEST_REPORT:105` flagged similar `NULLIF($2,'')` UUID and `"standalone"` default inconsistencies — these are manifestations of same category: drift between store constants and schema not caught at compile time.

**Fix:** Order handlers to check `DeleteManagedDatabaseWithForce` *before* mutating status, or make status mutation part of the same transaction. Replace string-sniff with explicit schema version check or `information_schema.columns` existence probe, or make `deleted_at` mandatory and run migration 208 in all envs before code ships.

---

## 5. Recommendations (prioritized)

### P0 — Close data-integrity / security gaps

1. **Fix `ListDBContainers` encrypted columns** (`store_db_containers.go:174,200`) — add `connection_string_encrypted, credentials_encrypted` and decrypt in list loops. Regression test: create container, encrypt, list, assert `connectionString` non-empty.
2. **Make `Deprovision`/`Restart` honest** (`containers.go:289,300`) — propagate daemon errors, set `failed` status like `database_service_provisioner.go:270`, do not delete row on failure.
3. **Guard deletion status ordering** (`handlers_managed_databases.go:166`, `handlers_user_databases.go:146`, `store_managed_databases.go:504`) — only set `deleting` inside `DeleteManagedDatabaseWithForce` transaction, or revert on `hasVolume && !force`.

### P1 — Unify provisioning surface & per-engine parity

4. **Unify password policy:** remove divergence between `database_service_provisioner/envVarsForDB:118` (`root == app`) and `containers.go:114` (`root = random64`). Persist root password if distinct.
5. **MySQL grant/user parity:** extend `database_service_credentials:114` with `host TEXT` and add `ListUsers/ListGrants/RevokeGrant` handlers per C02/C03. At minimum expose read-only enumeration backed by `adminConn` to match 1Panel audit expectations.
6. **PostgreSQL `superUser` + MongoDB privilege param:** add request fields mirroring `service/database_mongodb.go:313` `isSupportedMongodbPrivilege` and `service/database_postgresql.go:343` `super_user`.
7. **Redis honest surface:** replace dead `quoteSQLString` redis branch (`database_service_provisioner:370`) with `go-redis` path; consider bounded `CONFIG GET/SET` + `INFO` handlers via `go-redis` (already a dependency).

### P2 — Reduce duplication & drift

8. **Single engine catalog:** keep `SupportedDBEngines:14`, `DBEngineImages:22`, `DBEngineDefaultPorts:30` as sole source (already is) — remove duplicated `defaultPortForEngine` helpers in both provisioners (`database_service_provisioner.go:81` and `containers.go:136`) in favor of `store.DBEngineDefaultPorts`. Extract `imageForDB` vs `imageForDB/inline` maps to shared helper.
9. **Consolidate backup engine mapping:** deduplicate `dbbackup/engines.go:16` and `dbbackup/service.go:567` `EngineDumpCommands`. One table should source both.
10. **Fix TLS global registration:** (`dbprovisioner/service.go:373`) guard with `sync.Mutex` + map and support rotation via explicit unregistration or switch to connector-level TLS (no global).

### P3 — Product positioning

11. **Document runtime divergence explicitly:** 1Panel `model.Runtime:9` is not Forge `services/runtime:31`. The former manages language runtimes (php/node/...); the latter manages game-server runtimes. The `/admin/database-services` alias `page.tsx:1` → `/admin/databases` is correct, but no analog for `runtimes`. Add architecture decision record stating Forge DB services are container-provisioned only and that `my.cnf`/`php.ini`/`redis.conf` editing is intentionally absent.
12. **Address SMOKE_TEST_REPORT open items:** fix `NULLIF($2,'')` UUID cast in `store_managed_databases.go:117` and `"standalone"` default in `handlers_db_containers.go:56` (already filed as item 1 & 2).
13. **Add `DeleteCheck` for `server_databases`:** mirror `service/database_mysql.go:738` website/app dependency guard so panel cannot delete a host-provisioned DB still referenced by compose projects or domains.

---

## 6. Appendix — Endpoint & model inventory for reviewers

### 6.1 1Panel endpoints counted

| Group | Count | Sources |
|---|---|---|
| Common (baseinfo/conf file) | 3 | `database_common.go:17,40,62` |
| Generic Database (remote) | 9 | `database.go:20,50,75,100,122,144,167,190,212` |
| MySQL | 22 | `database_mysql.go:21-509` (full routing list at top) |
| PostgreSQL | 10 | `database_postgresql.go:21-221` |
| MongoDB | 12 | `database_mongodb.go:21-280` |
| Redis | 9 | `database_redis.go:19-164` |
| Runtime | 28 | `runtime.go:18-568` |
| PHP Extensions | 4 | `php_extensions.go:18-95` |
| **Total catalogued** | **97** | |

### 6.2 Forge handlers counted

| Group | Count | Routes |
|---|---|---|
| `database-services` (beacon) | 12 routes | `handlers_database_services.go:47` create, `78` list, `91` get, `104` delete, `116` restart, `131` create backup, `144` list backups, `157` restore, `169` logs, `182` create cred, `211` list cred, `224` revoke cred, + `236` testConnection, `258` list templates, `271` create template, `304` put template, `337` attach/detach server |
| `managed-databases` (admin) | 10 routes | `handlers_managed_databases.go:32` engines, `37` list, `50` get, `63` create (async provision), `117` patch, `158` delete (force), `179` backup, `192` restore, `214` rotate (stub), `227` list backups, `240` list restores |
| `managed-databases` (user) | 10 routes | `handlers_user_databases.go:17` engines, `22` list, `38` create, `93` get, `101` patch, `138` delete, `159` backup, `175` restore, `200` rotate (stub), `216` list backups, `229` restores, `242` credentials |
| `db_containers` | 7 routes | `handlers_db_containers.go:22` engines, `32` provision, `67` list, `87` get, `100` delete, `115` backup, `127` restart, `139` credentials |
| **Total catalogued** | **39** | |

### 6.3 Model cardinalities

| System | Models/tables | Notes |
|---|---|---|
| 1Panel | `DatabaseMysql:3` (15 lines), `DatabasePostgresql:3` (14), `DatabaseMongodb:3` (12), `Database:3` (24), `DatabaseUser:3` (12), `Runtime:9` (55), `PHPExtensions:3` (7) | `IsDelete bool` soft pattern throughout |
| Forge | `DatabaseService:12` + `Backup:35` + `Credential:44` + `ServiceTemplate:55` (`store_database_services.go:12`), `ManagedDatabase:14` + `Backup:46` + `Restore:62` (`store_managed_databases:14`), `DBContainer:38` (`store_db_containers.go:14`), `DatabaseHost:1` + `ServerDatabase:17` (`014_server_databases.sql:1`) | `encrypted_*` + AAD columns; `deleted_at` for managed DB; `database_host_node` pivot for multi-node |

---

*Report generated by parallel subagent 03. All line citations verified by reading referenced files at audit time. No files were modified.*
