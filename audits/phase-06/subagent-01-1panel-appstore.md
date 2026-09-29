# Subagent 01 — Phase 6: 1Panel App Store & Application Lifecycle vs Forge (AppStore / Catalog / Compose / AppHosting)

**Scope:** `reference/app-platforms/1panel/agent/app/api/v2/app.go`, `app_install.go`, `app_ignore_upgrade.go`, `compose_template.go` + `agent/app/model/app*.go`, `app_detail.go`, `app_tag.go` + `agent/app/service/*app*`, `dto/app*.go` vs `forge/api/internal/services/appstore`, `catalog`, `compose`, `apphosting` + `forge/api/internal/http/handlers_appstore.go`, `handlers_apphosting.go`, `handlers_compose.go` + `forge/web/app/admin/app-store` + `forge/api/internal/store/app_store*` tables + `beacon/internal/server/compose.go`

**Date:** 2026-08-23
**Auditor:** subagent 01 (parallel deep-dive)
**Method:** File reads before claims, line-cited. No product code modified.

---

## 1. Reference Inventory (1Panel)

| Area | File | Symbol / Feature |
|------|------|------------------|
| App catalog search | `reference/app-platforms/1panel/agent/app/api/v2/app.go:22` `SearchApp` | POST `/apps/search` with `request.AppSearch` (name/type/recommend/resource/arch/tags) → `appService.PageApp` |
| App sync remote | `app.go:42` `SyncApp` | `GetAppUpdate()` check `CanUpdate`/`IsSyncing` then `SyncAppListFromRemote(taskID)` async with `appStoreSyncMu` |
| App sync local | `app.go:74` `SyncLocalApp` | `go appService.SyncAppListFromLocal(taskID)` — imports filesystem local apps |
| Get app / detail | `app.go:91` `GetApp`, `app.go:115` `GetAppDetail`, `app.go:139` `GetAppDetailByID`, `app.go:252` `GetAppDetailForNode` | Key lookup, version+type, enable flag, params, docker-compose, hostMode, arch/memory, runtime PHP handling |
| Install | `app.go:162` `InstallApp` | `appService.Install(req,true)` → task `TaskInstall` |
| Tags | `app.go:181` `GetAppTags` | `GetAppTags` with i18n translation |
| Update check | `app.go:196` `GetAppListUpdate` | `GetAppUpdate()` version txt + icon existence |
| Icon | `app.go:213` `GetAppIcon` | ETag, Cache-Control 30d, 304 handling, base64 or file (`appicon`) |
| Installed lifecycle | `app_install.go:19` `SearchAppInstalled`, `54` `ListAppInstalled`, `71` `CheckAppInstalled`, `92` `LoadPort`, `113` `LoadConnInfo`, `134` `DeleteCheck`, `156` `SyncInstalled`, `173` `OperateInstalled`, `193` `GetServices`, `210` `GetUpdateVersions`, `232` `ChangeAppPort`, `252` `GetDefaultConfig`, `274` `GetParams`, `298` `UpdateInstalled`, `318` `UpdateAppConfig`, `338` `GetAppInstallInfo`, `361` `UpdateAppInstallSort` | Full CRUD + operate (rebuild/start/stop/restart/delete/sync/upgrade/reload/favorite) |
| Ignore upgrade | `app_ignore_upgrade.go:16` `ListAppIgnored`, `34` `IgnoreAppUpgrade`, `56` `CancelIgnoreAppUpgrade` | Scope `version` vs `all` |
| Compose template | `compose_template.go:18` `CreateComposeTemplate`, `40` `BatchComposeTemplate`, `62` `SearchComposeTemplate`, `87` `ListComposeTemplate`, `106` `DeleteComposeTemplate`, `128` `UpdateComposeTemplate` | CRUD for `ComposeTemplate` (name/description/content) |
| Model App | `agent/app/model/app.go:13` `App` | Fields: `Key,ShortDescZh/En,Description(i18n JSON),Icon,Type,Status,Required,CrossVersionUpdate,Limit,Resource(remote/local/custom),Architectures,MemoryRequired,GpuSupport,RequiredPanelVersion,BatchInstallSupport` + `GetAppResourcePath()` `IsLocalApp()/IsCustomApp()` |
| Model AppDetail | `app_detail.go:3` `AppDetail` | `AppId,Version,Params(JSON),DockerCompose,Status,LastModified,DownloadUrl,Update` |
| Model AppInstall | `app_install.go:11` `AppInstall` | `Name(UNIQUE),AppId,AppDetailId,Version,Param,Env,DockerCompose,Status,HttpPort/HttpsPort,ContainerName,ServiceName,WebUI,Favorite,SortOrder` + path helpers `GetPath/GetComposePath/GetEnvPath/GetAppPath` |
| Model AppTag | `app_tag.go:3` `AppTag` | Join `AppId,TagId` |
| Model AppIgnoreUpgrade | `app_ignore_upgrade.go:3` `AppIgnoreUpgrade` | `AppID,AppDetailID,Scope` |
| Model ComposeTemplate | `compose_template.go:3` `ComposeTemplate` | `Name(unique),Description,Content` |
| Service App | `agent/app/service/app.go:69` `PageApp`, `174` `GetAppTags`, `196` `GetApp`, `234` `GetAppDetail`, `331` `GetAppDetailByID`, `347` `Install`, `351` `installWithHooks`, `596` `SyncAppListFromLocal`, `828` `GetAppUpdate`, `968` `SyncAppListFromRemote`, `1023` `GetAppIcon` | Core logic: tag translation, arch filter, runtime detection, downloadUrl fallback, hostMode, limit checks, port extraction `PANEL_APP_PORT_*`, container naming `1panel-{key}-{rand4}`, network creation, sortOrder, async task install |
| Service AppInstall | `agent/app/service/app_install.go:79` `Page`, `133` `CheckExist`, `177` `LoadPort`, `185` `LoadConnInfo`, `200` `SearchForWebsite`, `246` `Operate`, `315` `UpdateAppConfig`, `327` `UpdateSort`, `336` `Update`, `472` `SyncAll`, `502` `GetServices`, `571` `GetUpdateVersions`, `636` `ChangeAppPort`, `664` `DeleteCheck`, `692` `GetDefaultConfigByKey`, `723` `GetParams`, `958` `GetAppInstallInfo` | Operate switch (rebuild/start/stop/restart/delete/sync/upgrade/reload/favorite), backup/image flags, proxy pass nginx reload, sync via `docker.ListContainersByName` |
| Service AppIgnoreUpgrade | `app_ingore_upgrade.go:24` `List`, `57` `CreateAppIgnore`, `74` `Delete` | Orphan cleanup on list, scope handling `version`/`all` |
| DTO | `agent/app/dto/app.go:9` `AppDatabase..`, `44` `AppVersion`, `50` `AppList/AppDefine/AppProperty`, `109` `AppConfigVersion`, `138` `AppForm`, `165` `AppResource` ; `dto/request/app.go:9` `AppSearch`, `19` `AppInstallCreate`, `37` `AppContainerConfig`, `55` `AppInstalledSearch`, `67` `AppInstalledInfo`, `81` `AppInstalledOperate`, `97` `AppInstallUpgrade`, `116` `AppInstalledUpdate`, `132` `PortUpdate`, `138` `AppUpdateVersion` ; `dto/response/app.go:12` `AppRes`, `17` `AppUpdateRes`, `24` `AppDTO`, `31` `AppItem`, `52` `AppInstalledCheck`, `68` `AppDetailDTO`, `90` `AppInstallDTO`, `154` `DatabaseConn`, `163` `AppService`, `171` `AppParam`, `186` `AppConfig` ; `dto/compose_template.go:5` `ComposeTemplateCreate/Batch/Update/Info` | Rich filtering, i18n description, form generation |

---

## 2. Forge Inventory

| Layer | File | Symbol / Feature |
|-------|------|------------------|
| **Frontend** | `forge/web/app/admin/app-store/page.tsx:27` `AppStorePage` | browse/installed/detail views, category filter, search debounce 500ms, Grid3X3, Package, RefreshCw, install form (name/node/memory/disk/params), uninstall confirm, StatusBadge |
| **Frontend API** | `forge/web/lib/api/app-store.ts:51` `apiFetch`, `108` `listApps`, `116` `getApp`, `120` `installApp`, `128` `uninstallApp`, `134` `listInstalls`, `138` `upgradeApp`, `144` `syncRegistry` | Retry 3× with Retry-After/exp backoff, CSRF, `data` unwrapping |
| **API** | `forge/api/internal/http/handlers_appstore.go:11` `registerAppStoreRoutes` | `GET /app-store/apps`, `GET /admin/app-store-templates`, `GET /app-store/apps/:key`, `POST /app-store/install`, `POST /app-store/:id/uninstall`, `GET /app-store/installed`, `POST /app-store/:id/upgrade`, `POST /app-store/sync` |
| **API Catalog** | Forge has no `handlers_catalog.go`; catalog is accessed via `catalog` service internally; not directly exposed as HTTP in this slice — mediated via compose/db provisioners | Gap vs 1Panel's direct app routes |
| **API AppHosting** | `handlers_apphosting.go:55` `registerAppHostingRoutes` | `/organizations/:orgId/apps`, `/apps`, `/apps/:id/{deploy,services,instances,logs,domains,backups,compose,git}`, `POST /apps/:id/{start,stop,restart}`, `GET /admin/app-templates`, `GET /admin/git-branches` (git ls-remote) |
| **API Compose** | `handlers_compose.go:66` `registerComposeRoutes` | `/compose/validate`, `/compose/import`, `/compose/git/deploy|.../redeploy|check-update|preview|rollback|pull-redeploy|branch|auto-update|drift|status|last-webhook`, `POST /compose`, `GET /compose`, `GET /compose/:id{,status,logs}`, `PATCH /compose/:id`, `DELETE /compose/:id`, `POST /compose/:id/{deploy,stop,start,restart}`, legacy `/compose/projects` CRUD |
| **Service AppStore** | `forge/api/internal/services/appstore/service.go:20` `Service` `31` `New`, `41` `ListApps`, `45` `GetApp`, `53` `ListInstalls`, `74` `InstallApp`, `139` `UninstallApp`, `158` `UpgradeApp`, `204` `SyncFromRemote`, `239` `resolveTemplate` | Store+composeSvc mandatory, template expansion via `compose.ExpandTemplate`, deploy via `DeployComposeStack`, status mapping `degraded|failed→error` |
| **Service AppStore Seed** | `seed.go:27` `seedApps` `215` `SeedDefaultApps` | 7 hard-coded apps (nginx,postgres,redis,mongo,mariadb,portainer,traefik) |
| **Service Catalog** | `catalog/catalog.go:61` `Service` `119` `Provision`, `190` `ListEntries`, `198` `GetEntry`, `209` `ListInstances`, `218` `GetInstance` | Unified catalog (postgres/mysql/mariadb/redis/mongo/valkey/redis-queue/rabbitmq/clickhouse/nats/memcached) |
| **Catalog Inject** | `catalog/inject.go:62` `writeConnVars`, `111` `InjectAndInjectConnector`, `128` `Attach` | Writes `DATABASE_URL/REDIS_URL/AMQP_URL` etc as sensitive env vars + `catalog_attach_links` |
| **Catalog Retention** | `catalog/retention.go:56` `runRetentionPass`, `84` `RunRetentionNow`, `94` `StartRetentionWorker` (hourly) | Per-kind backup pruning |
| **Service Compose** | `services/compose/service.go:126` `Service` `137` `Parse`, `240` `ValidateCompose`, `352` `ExpandTemplate`, `427` `ValidateComposeSecurity` | Parsing YAML, security validation (privileged, host namespaces, cap_add, devices, docker.sock, /etc), resource quotas, scheduler placement |
| **Service Compose Lifecycle** | `services/compose/lifecycle.go:249` `DeployComposeStack`, `476` `UpdateComposeStack`, `570` `DeleteComposeStack`, `625` `GetStackStatus`, `663` `GetStackLogs`, `697` `StartStack`, `745` `StopStack`, `781` `RestartStack` | Scheduler `PlaceServer`, reservation, daemon `ComposeDeploy`, `WaitForHealthy` 2min, rollback on failure |
| **Service Compose GitOps** | `services/compose/gitops.go:301` `DeployFromGit`, `431` `CheckForUpdates`, `504` `RedeployFromGit`, `548` `PullAndRedeploy`, `595` `RollbackToPrevious`, `687` `DetectDrift`, `1151` `HandleWebhook` | HMAC `sha256=`, polling, webhook `pending→idle` async deploy, drift classification |
| **Service AppHosting** | `services/apphosting/service.go:39` `Service` `93` `CreateApp`, `125` `GetApp`, `136` `ListApps`, `140` `UpdateApp`, `175` `DeleteApp`, `199` `CreateService`, `241` `ListServices`, `252` `DeleteService`, `360` `UpdateService`, `398` `ScaleService`, `455` `GetServiceStatus`, `529` `GetServiceOverview`, `609` `PlanServiceUpdate`, `681` `TriggerDeploy` | Tenancy checks `AppBelongsToOrg`, desired_state validation, replica placement guard |
| **Store AppStore** | `store/store_app_store.go:10` `AppStoreApp`, `30` `AppStoreInstall`, `48` `ListAppStoreApps`, `81` `GetAppStoreApp`, `92` `UpsertAppStoreApp`, `112` `CreateAppStoreInstall`, `120` `GetAppStoreInstall`, `132` `ListAppStoreInstalls`, `152` `ListAppStoreInstallsForUser`, `172` `UpdateAppStoreInstallStatus` | `tags TEXT[]`, `params JSONB`, `compose_content TEXT`; user/org scoping |
| **Store Catalog** | `store/store_catalog.go:14` `CatalogEntry`, `32` `CatalogInstance`, `52` `CatalogAttachLink`, `62` `BackupRetention`, `88` `ListCatalogEntries`, `112` `GetCatalogEntry`, `134` `CreateCatalogInstance`, `158` `GetCatalogInstance`, `180` `ListCatalogInstances`, `208` `UpdateCatalogInstanceStatus`, `231` `CreateCatalogAttachLink` | 11 seeded kinds, provisioning status, conn_string, ref_type |
| **Store Compose** | `store/store_compose.go:11` `ComposeStack` (id text `cps-`), etc. | Stack + service + logs + ProjectDocument legacy |
| **Store AppHosting** | `store/store_apphosting.go:11` `Application`, `68` `AppService`, `113` `CreateApplication`, `151` `CreateApplication` | org/project/env/server linkage, desired/observed status |
| **DB** | `migrations/113_app_store.sql:1` (app_store_apps/installs), `175_catalog_entries.sql`, `177_catalog_instances.sql`, `178_backup_retention.sql`, `181_catalog_attach_links.sql`, `205_app_store_install_compose_project_text.sql`, `206_app_store_installs_ownership.sql`, `113_app_store.sql` vs `migrations/115_compose_stacks.sql` | AppStore tables are distinct from `applications`/`app_services`/`compose_stacks` |
| **Beacon** | `beacon/internal/server/compose.go:77` `validateComposePolicy`, `344` `handleComposeDeploy`, `418` `handleComposeStop`, `462` `handleComposeStart`, `506` `handleComposeRestart`, `550` `handleComposeDelete`, `604` `handleComposeStatus`, `661` `handleComposeLogs`, `709` `handleComposePull` | Policy (truthy check, host ports <1024, bind mounts), locks per stackID, docker compose up/stop/start/restart/down/ps/logs |
| **Daemon** | `forge/api/internal/daemon/compose.go:48` `ComposeDeploy`, `73` `ComposeStop/Start/Restart/Delete/Pull/Status/Logs` | HTTP to beacon `/compose/*` with nodeToken |
| **Worker** | `catalog/retention.go:94` `StartRetentionWorker` (hourly), `compose/gitops.go:1327` `StartPolling` (60s), no app-store sync worker (manual `POST /app-store/sync`) | No equivalent to 1Panel's `task.TaskSync` with `task.TaskScopeAppStore` async chain |
| **Tests** | `store/store_app_store_integration_test.go:59` `TestUpsertAppStoreAppTagsRoundTrip`, `146` `TestAppStoreInstallsRoundTrip`; `store/store_compose_integration_test.go`, `services/compose/service_test.go`, `services/apphosting/service_test.go` | Covers tags array round-trip, re-upsert idempotency, search |

---

## 3. Detailed Comparisons (≥12)

### C01 — App Registry Search & Filtering

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `agent/app/api/v2/app.go:22` `SearchApp` → `dto/request/app.go:9` `AppSearch{PageInfo,Name,Tags,Type,Recommend,Resource,ShowCurrentArch}` → `service/app.go:69` `PageApp` (repo opts `OrderByRecommend`, `WithByLikeName`, `WithType`, `GetRecommend`, `WithResource`, `WithArch` via `DashboardService.LoadOsInfo` + kernel `aarch64→arm64`, tag join `tagRepo→appTagRepo`, php version gate `RequiredPanelVersion`, `getAppTags(lang)`, `Installed` via `appInstallRepo` or `runtimeRepo`) | `handlers_appstore.go:23` `listAppsHandler` → `services/appstore/service.go:41` `ListApps(category,search)` → `store/store_app_store.go:48` `ListAppStoreApps(category,search)` (`ILIKE name/short_desc`, `category = $1`, `ORDER BY name ASC`) ; admin shadow `GET /admin/app-store-templates` |
| **Forge Frontend** | n/a (1Panel frontend not in scope) | `forge/web/app/admin/app-store/page.tsx:28` `categories` (hard-coded 5), `40` debounce 500ms, `55` `listApps(debouncedCategory,debouncedSearch)` |
| **Forge API** | `PageApp` supports 6 dimensions + pagination gzip | `ListApps` supports only 2 dimensions (category, search) + no pagination, no recommend/resource/arch/tags filters |
| **Forge Service** | N/A | `store_app_store.go:48` simple concatenated WHERE |
| **Forge Store** | N/A | `ListAppStoreApps` — no total count returned (unlike `AppRes{Items,Total}`), no `PageResult` |
| **Forge DB** | `apps` table with `type, resource, architectures, recommend, tags` via join | `app_store_apps` flat: `category TEXT, tags TEXT[], version TEXT` single version string vs 1Panel's `AppDetail` versions array |
| **Worker/Beacon/Tests** | background sync via task | No worker ; test `TestUpsertAppStoreAppTagsRoundTrip:59` covers storage |
| **STATUS** | **PARTIAL** | Core browse works; filtering, sorting, pagination, arch awareness, installed flag incomplete |
| **GAP** | Missing: `recommend`, `resource` (remote/local/custom), `showCurrentArch`, tag-based multi-select, `limit`/`page` pagination, `type` (runtime/php/node…) filtering, `Installed` flag per card (Forge computes via separate `installedKeys` set but N+1), `BatchInstallSupport`, `gpuSupport`, `crossVersionUpdate` visibility. `ORDER BY name ASC` vs `ORDER BY recommend` changes discovery ranking. | |
| **RECOMMENDATION** | **ADAPT** — Add query params `type`, `resource`, `recommend`, `tags[]`, `showCurrentArch`, `page/pageSize` to `handlers_appstore.go:23` and extend `store_app_store.go:48` with arch filtering (reuse `runtime.GOARCH` or node label). Preserve 1Panel's `TagsKey` i18n pattern for future i18n. Do NOT copy PHP-specific version gating (`reference/agent/app/service/app.go:128`) — irrelevant to Forge. | |
| **SEVERITY** | Medium — affects discoverability but not correctness. | |

---

### C02 — App Detail / Versions / Enablement

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app.go:115` `GetAppDetail(appId,version,type)` + `app.go:139` `GetAppDetailByID` + `app.go:252` `GetAppDetailForNode` + `model/app_detail.go:3` `AppDetail{AppId,Version,Params,DockerCompose,Status,DownloadUrl,Update}` + `dto/response/app.go:68` `AppDetailDTO{Enable,Params,Image,HostMode,Architectures}` ; service `service/app.go:234` `GetAppDetail` handles runtime `data.yml`/`docker-compose.yml` from filesystem, `DownloadUrl` fallback fetch, `checkLimit(app)` → `Enable=false`, `isHostModel` detection | `handlers_appstore.go:40` `GET /app-store/apps/:key` → `store/store_app_store.go:81` `GetAppStoreApp` returns single `version` string + `composeContent` + `params JSONB` (flat). No `GetAppDetail` endpoint. Detail view in `page.tsx:368` `AppDetailView` shows only `version`, `category`, `minMemory/Disk`, `description`, `maintainer` — no param form schema, no hostMode/arch/gpu, no per-version docker-compose preview |
| **Forge Service** | `services/appstore/service.go:45` `GetApp` single path | No version list, no cross-version checks |
| **STATUS** | **PARTIAL** | Single-version catalog vs multi-version `AppDetail` rows |
| **GAP** | 1Panel stores N versions per app (`app_detail` rows) with distinct `Params` (JSON `AppForm`), `DockerCompose` per version, `Status` (normal/takedown). Forge collapses to one `version` column (`113_app_store.sql:9` `version VARCHAR(50) DEFAULT 'latest'`) — cannot represent 1Panel's `postgres 14/15/16` catalog or rollback target. Also misses `Enable` (limit check `checkLimit` in `service/app.go:323`), `HostMode` detection (`isHostModel`), `Architectures`/`MemoryRequired`/`GpuSupport` gating. Frontend `InstallFormModal:475` builds params from `app.params` defaults but ignores `type/select/apps/service` field types that 1Panel's `GetParams` handles (`service/app_install.go:782` `Type:"service"`→ resolves `AppInstall.Name`). | |
| **RECOMMENDATION** | **ADAPT** — Introduce versioned catalog: either normalize `AppStoreApp`→`app_store_app_versions` (FK app_id) mirroring `app_detail`, or at minimum store `versions TEXT[]` already in catalog_entries but not in app_store_apps. Add `GetAppVersions(key): AppVersion[]` handler mirroring `app_install.go:210` `GetUpdateVersions`. Gate install with `Limit` check (1Panel's `checkRequiredAndLimit` → `Limit==0` single-instance vs multi). | |
| **SEVERITY** | High — blocks multi-version upgrades/downgrades and accurate capacity gating. | |

---

### C03 — App Tags & Taxonomy

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `model/app.go:40` `TagsKey []string yaml:"tags"` + `model/tag.go` (Tag{Key,Translations JSON}) + `model/app_tag.go:3` `AppTag{AppId,TagId}` + `dto/app.go:117` `Tag{Key,Name,Locales}` + service `app.go:174` `GetAppTags` with `getLang(c)` translation fallback (`app.go:61` `getLang`) ; `PageApp` tag filter via `tagRepo.GetByKeys → appTagRepo.GetByTagIds → appRepo.WithByIDs` | `store/store_app_store.go:8` `AppStoreApp{Tags []string}` stored as `TEXT[]` (`113_app_store.sql:9` `tags TEXT[]`); `store_catalog.go:62` `BackupRetention` not tags. No tag table, no i18n, no join. `page.tsx:279` renders `app.tags.slice(0,3)` as chips. `handlers_appstore.go:23` passes only `category`/`search`, not tags. |
| **STATUS** | **PARTIAL** | Tags stored but not filterable via API, not normalized, no i18n |
| **GAP** | 1Panel's tag system supports `GET /apps/tags` (`app.go:181`) and search-by-tags with multi-tag AND via `AppTag` join. Forge's `ListAppStoreApps` (`store_app_store.go:48`) never filters by tags; `UpsertAppStoreApp` (`92`) accepts `tags::text[]` but `ListApps` ignores them. Divergence: `TagsKey` in 1Panel includes `ResourceLocal` auto-tag (`SyncAppListFromLocal:699` `append(TagsKey, AppResourceLocal)`). Forge's `categories` in `page.tsx:18` are categories, not tags — conflation of taxonomy levels. | |
| **RECOMMENDATION** | **INSPIRE** — Keep `TEXT[]` simplicity for now (avoids join), but wire `tags` query param: `WHERE tags && $3::text[]` (overlap) in `ListAppStoreApps`. Add `GET /app-store/tags` aggregation (`SELECT DISTINCT unnest(tags)` ) mirroring `GetAppTags`. Defer i18n until needed. | |
| **SEVERITY** | Low — discoverability only. | |

---

### C04 — Remote App Store Sync (Catalog Sync)

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app.go:42` `SyncApp` + `service/app.go:828` `GetAppUpdate` + `968` `SyncAppListFromRemote(taskID)` → checks `setting.AppStoreLastModified` vs remote `1panel.json.version.txt` (`service/app.go:839` `versionUrl`), `appStoreSyncMu` + `appStoreSyncing bool` global mutex, `task.NewTaskWithOps` with two subtasks `createSyncAppStoreTask` + `createSyncAppStoreMetaTask`, `getAppList()` downloads `1panel.json.zip` via `DownloadFileWithProxy`, decompress, JSON decode `dto.AppList{Apps []AppDefine, Extra Tags/Version}`, `deleteCustomApp()` handling, transactional `BatchCreate/Save/Delete` for apps/details/tags | `handlers_appstore.go:144` `POST /app-store/sync {registryUrl}` → `services/appstore/service.go:204` `SyncFromRemote(registryURL)` → direct `http.DefaultClient.Do`, `io.ReadAll` 10MB limit, `json.Unmarshal([]AppStoreApp)` → `for app:=range apps { UpsertAppStoreApp }` ; `seed.go:215` `SeedDefaultApps` inserts 7 hard-coded apps without remote fetch. No mutex, no version check, no task, no transactional batch. |
| **Forge Worker/Beacon/Tests** | background async `go syncTask.Execute()` with recover | Sync is synchronous inside request context (`c.Context()`), no background task, no status `IsSyncing`/`CanUpdate` feedback |
| **STATUS** | **PARTIAL** | Manual sync exists but diverged semantics |
| **GAP** | 1Panel's protocol: version.txt gate → zip download → structured `AppDefine` with `Versions []AppConfigVersion` + `ExtraProperties.Tags/Version` (panel version floor). Forge expects `[]AppStoreApp` JSON directly (different schema) — incompatible with 1Panel app repo. Missing: global mutex (concurrent sync race), `IsSyncing`/`CanUpdate` signaling (`app.go:52` handling), `SyncAppListFromRemote` async task reporting, `deleteCustomApp` pruning, icon handling (`appicon.IsIconFile` fallback in `GetAppUpdate:865`), version compatibility check (`list.Extra.Version` vs `SystemVersion`). Forge's `SyncFromRemote` returns sync error directly (500) vs 1Panel's task-tracked error with `AppStoreSyncStatus=error`. Also missing `POST /apps/checkupdate` + `GetAppListUpdate` polling endpoint. | |
| **RECOMMENDATION** | **ADAPT** — Add `AppStoreSyncMu sync.Mutex` + `appStoreSyncing` flag mirroring `service/app.go:40` to avoid concurrent fetch. Add `GET /app-store/check-update` returning `{canUpdate,isSyncing,lastModified}`. Support 1Panel's zip protocol as alternative source (detect `Content-Type: application/zip` or `.zip` suffix). Make sync async via `task` or at least goroutine + return `202 Accepted` like 1Panel's immediate `helper.Success(c)`. Add `registryUrl` default from env (`AppRepoURL` equiv). | |
| **SEVERITY** | Medium — operational parity; current manual sync suffices for dev but not production catalog freshness. | |

---

### C05 — Local / Custom App Sync

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app.go:74` `SyncLocalApp` → `service/app.go:596` `SyncAppListFromLocal(taskID)` — scans `global.Dir.LocalAppResourceDir`, reads `LocalAppAppDefine` YAML, reconciles `OldApps` vs `LocalApps` via `appRepo.WithResource(AppResourceLocal)`, marks `Status=Takedown`, preserves installed apps from deletion, transactional `BatchCreate/BatchDelete` for details + tags, `TagsKey append(AppResourceLocal)` | Forge has NO equivalent local app filesystem import. Custom apps are created as `applications` with `source_type=COMPOSE/GIT/DOCKER_IMAGE` via `handlers_apphosting.go:116` `CreateApp`, not as catalog entries. |
| **STATUS** | **MISSING** | Intentional divergence — Forge's multi-node model doesn't have per-node app resource dirs |
| **GAP** | 1Panel's local app sync handles developer-sideloaded `data.yml`/`docker-compose.yml` under a resource dir, with version subdirs. Forge's `compose/projects` (`handlers_compose.go:513`) and `compose import` allow raw compose ingestion but not catalog-style local apps. No file-system watcher, no `CustomAppResource` (`AppResourceCustom`) handling (`model/app.go:48` `IsCustomApp`). | |
| **RECOMMENDATION** | **REJECT** — Do not adopt. Forge's `AppHosting` + `Compose` import already covers custom workloads more flexibly across nodes. If parity needed for offline/air-gapped, add an explicit `POST /app-store/import` tar/zip upload rather than filesystem scan. | |
| **SEVERITY** | Low — not applicable to Forge's SaaS deployment model. | |

---

### C06 — Application Install Lifecycle (Create → Deploy)

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app.go:162` `InstallApp` → `service/app.go:347` `Install` → `351` `installWithHooks` : `docker.CreateDefaultDockerNetwork()`, `appInstallRepo.ListBy lowerName` uniqueness, `DatabaseKeys[app.Key]` remote existence guard, `checkPort(PANEL_APP_PORT_*)`, `checkRequiredAndLimit`, build `AppInstall{Name,AppId,AppDetailId,Version,Status=installing,HttpPort/HttpsPort,SortOrder}`, compose map manipulation (`editCompose` or `downloadUrl` fetch, `yaml.Unmarshal/Marshal`, service rename when `Limit==0`, `addDockerComposeCommonParam`), `createLink` DB relation, `copyData(t,app,appDetail,appInstall,req)` + `runScript(init)` + `handleSiteDir/handleOpenrestyFile` + `upApp(pullImage flag)` + `updateToolApp`, background `installTask.Execute()` with timeout `PullImageTimeout` handling (`service/app.go:586` ) | `handlers_appstore.go:51` `POST /app-store/install` → `services/appstore/service.go:74` `InstallApp` : `GetAppStoreApp`, `json.Marshal(params)`, `resolveTemplate(app.ComposeContent,params)` via `compose.ExpandTemplate` (fallback `return tmpl` on error), `CreateAppStoreInstall(status=installing)`, `DeployComposeStack` (`services/compose/lifecycle.go:249`) with scheduler placement, reservation, `ValidateCompose` security, quota `MaxUserStacks=20`, `WaitForHealthy 2m`, `UpdateAppStoreInstallComposeProject` + `UpdateAppStoreInstallStatus(running/error)` |
| **Forge Frontend** | `page.tsx:475` `InstallFormModal` → `page.tsx:501` `onInstall({appKey,name,nodeId,memoryMb,cpuShares,diskMb,params})` |  |
| **STATUS** | **PARTIAL** | Happy-path wired but semantics diverge |
| **GAP** | 1Panel's install is deeply imperative: file-system copy, init scripts, openresty site handling, container name collision check (`WithContainerName` + `checkContainerNameIsExist`), `Limit` rename (serviceName→appName when singleton), `allowPort`/`cpuQuota`/`memoryLimit` injection, local DB link creation. Forge delegates entirely to compose stack deployment (generic). Missing checks: uniqueness across `appStoreInstalls.name` — 1Panel errors `ErrAppNameExist` (`service/app.go:357`), Forge allows duplicate names (no `UNIQUE` on `app_store_installs.name` vs `AppInstall.Name gorm:"UNIQUE"`). Missing `DatabaseKeys` guard, `checkRequiredAndLimit`, `PANEL_DB_HOST` dependency validation, `crossVersionUpdate` nothing to check against (single version). `resolveTemplate` failure mode is silent fallback vs 1Panel's hard error. Resource defaults: 1Panel derives from `AppInstall` env extraction; Forge defaults in handler (`handlers_appstore.go:72` `256/512/1024`) vs actual catalog `MinMemoryMB/MinDiskMB` not consulted. | |
| **RECOMMENDATION** | **ADAPT** — Enforce `UNIQUE(name, user_id)` or at least per-user duplicate check (mirroring `ListByLowerName`). Validate required params against `app.Params` schema before `resolveTemplate` (fail fast on `${VAR:?msg}` errors instead of silent `return tmpl`). Source defaults from `app.MinMemoryMB/MinDiskMB` when handler defaults absent. Do not adopt openresty/php-specific branches. | |
| **SEVERITY** | High — duplicate-name + silent-template bypass are correctness issues. | |

---

### C07 — Installed App Listing, Pagination, Sorting, Favorite

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app_install.go:19` `SearchAppInstalled` with `request.AppInstalledSearch{Type,Name,Tags,Update,Unused,All,Sync,CheckUpdate}` → `service/app_install.go:79` `Page` (order `favorite DESC, sort_order ASC`, tag filter via `appTagRepo→WithAppIdsIn`, `handleInstalled(installs, update, sync, checkUpdate)` for canUpdate `GetUpdateVersions` check + status sync), `54` `ListAppInstalled` lightweight, `361` `UpdateAppInstallSort` (`AppInstallSort{Items[]{InstallID,SortOrder}}`) → `BatchUpdateBy`, `303` favorite toggle recomputes `MAX(sort_order)` | `handlers_appstore.go:114` `GET /app-store/installed` → `service/appstore/service.go:53` `ListInstalls(userID)` → `store/store_app_store.go:132` `ListAppStoreInstalls` (`ORDER BY created_at DESC`) ; `handlers_apphosting.go:102` `GET /organizations/:orgId/apps` separate. No pagination, no favorite, no sort_order, no tag filter, no `Update`/`Sync` flags. `page.tsx:116` computes `installedKeys Set` client-side |
| **STATUS** | **PARTIAL** | Basic listing complete; rich controls missing |
| **GAP** | 1Panel's `installed` search is filtered, sorted, paginated, and annotates `CanUpdate`, `Ready/Total` (health), `Path`, `AppName`, `Icon`, `Favorite`. Forge returns raw `AppStoreInstall[]` with no pagination (`ListAppStoreInstalls` no limit), no `total`, no `favorite`/`sort_order` columns (`113_app_store.sql:21` lacks them), no `canUpdate` annotation, no tag filter reuse. Sorting is fixed `created_at DESC` vs `favorite→sort_order`. Favorite/ordering APIs absent. | |
| **RECOMMENDATION** | **INSPIRE** — Add `favorite BOOLEAN DEFAULT false, sort_order INT DEFAULT 0` to `app_store_installs` (migration) and expose `PATCH /app-store/:id/favorite`, `POST /app-store/sort` mirroring 1Panel if UX demands. Otherwise keep simple list but add pagination (`?page&perPage`) and `canUpdate` computed field (compare `inst.AppVersion` vs `app.Version`). | |
| **SEVERITY** | Low — UX polish; not blocking. | |

---

### C08 — Operate Installed (Start/Stop/Restart/Rebuild/Delete/Sync/Upgrade)

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app_install.go:173` `OperateInstalled` (switch `constant.Rebuild/Start/Stop/Restart/Delete/Sync/Upgrade/Reload/Favorite`) → `service/app_install.go:246` `Operate` : `compose.Up/Stop/Restart` (`github.com/1Panel/utils/compose`), `rebuildApp`, `deleteAppInstall(deleteReq{ForceDelete,DeleteBackup,DeleteDB,DeleteImage})` with `DeleteCheck` resource guard, `syncAppInstallStatus` via `docker.ListContainersByName(containerNames)` + `synAppInstall`, `upgradeInstall`, `opNginx(NginxReload)`, favorite. `SyncAll(systemInit bool)` handles `StatusInstalling/Upgrading→Error` on restart, `WaitingRestart→Up`. | **Forge app-store path:** `handlers_appstore.go:100` `POST /:id/uninstall` → `service/appstore/service.go:139` `UninstallApp(actorId,actorRole)` (perm check `ErrInstallForbidden` if non-admin + owner mismatch) → `DeleteComposeStack` (warn-only) → `DeleteAppStoreInstall`; `126` `POST /:id/upgrade` → `service/appstore/service.go:158` `UpgradeApp` (compare versions, re-resolveTemplate, `UpdateComposeStack`). **Forge compose path**: `handlers_compose.go:440` `POST /compose/:id/stop`, `452` `POST /compose/:id/start`, `470` `POST /compose/:id/restart`, plus apphosting `handlers_apphosting.go:450` `POST /apps/:id/start/stop`, `500` `POST /apps/:id/restart` via `daemon.SendPower`. No rebuild, no reload, no favorite. **Status sync:** `services/compose/lifecycle.go:188` `WaitForHealthy` polling `ComposeStatus` from beacon. |
| **STATUS** | **PARTIAL** | Start/stop/restart/upgrade/delete wired across multiple routes; rebuild/reload/sync/favorite absent |
| **GAP** | 1Panel's `Operate` is a single endpoint with enum `constant.AppOperate`. Forge fragments lifecycle across app-store, compose, and apphosting routers — discoverability cost. `Uninstall` in Forge does `Update status uninstalling` then immediate `Delete` (`service/appstore/service.go:154-155`) even if `DeleteComposeStack` fails (swallowed warning) → orphan DB record removed but stack remains on node (opposite of 1Panel's forceDelete flag). No `DeleteCheck` guard (`app_install.go:134` checks website links + `app_install_resource` dependents) → Forge can delete app-store install that still has `catalog_attach_links`/`applications` referencing it. No `Rebuild` (1Panel recreates containers from compose + env). No `Sync` manual status re-sync (Forge relies on `WaitForHealthy` at deploy time only). No `Reload` (nginx-specific but pattern for hot-reload). Upgrade in Forge reuses latest catalog `version` unconditionally, ignoring `CrossVersionUpdate` and major-version pinning (1Panel `GetUpdateVersions:590` checks `!app.CrossVersionUpdate && IsCrossVersion` then skips, and `app.Key==mysql` major prefix filter). | |
| **RECOMMENDATION** | **ADAPT** — Add `DeleteCheck` equivalent for app-store installs: query `catalog_instances`/`applications` dependent counts before delete; surface as `409 Conflict` with `resources[]`. Make uninstall transactional: only delete DB row after `DeleteComposeStack` success (or require `?force=true`). Add `POST /app-store/:id/sync` calling `GetStackStatus` to refresh `status` from `compose_stacks.status`. Keep lifecycle enum unified (optional) — document fragment vs consolidate. | |
| **SEVERITY** | High — orphan-risk on uninstall + missing dependency guard. | |

---

### C09 — Update / Upgrade Version Selection

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app_install.go:210` `GetUpdateVersions` → `service/app_install.go:571` `GetUpdateVersions(req AppUpdateVersion{AppInstallID,UpdateVersion})` : load `install+app+details`, iterate `appDetailRepo.GetBy(WithAppId)`, filter `appIgnoreUpgradeRepo` (`scope=version`), `common.IsCrossVersion && !app.CrossVersionUpdate → skip`, `common.CompareVersion(detail.Version,install.Version)` (or `isVllmUpgradeCandidate` special case), fetch `DockerCompose` if empty via `DownloadUrl`, `getUpgradeCompose(install,detail)`, MySQL `majorVersion` prefix filter, sorted ascending | `handlers_appstore.go:126` `POST /:id/upgrade` → `service/appstore/service.go:158` `UpgradeApp(installID,actorId,actorRole)` : load `inst`, load `app` (single version), `resolveTemplate(app.ComposeContent, params)`, `UpdateAppStoreInstallStatus(upgrading)`, `UpdateComposeStack` if `ComposeProjectID != ""` else only DB update; no version choice, no ignore filter, no cross-version gate |
| **STATUS** | **PARTIAL** | Upgrade works for single-version catalog but is false-completion for multi-version semantics |
| **GAP** | 1Panel exposes explicit version picker: `POST /apps/installed/update/versions` returns `[]AppVersion{Version,DetailId,DockerCompose}` sorted, respecting ignored versions and `CrossVersionUpdate` flag. Forge's upgrade is "upgrade to whatever catalog currently says is latest" — no `detailId` payload, no candidate list. Missing: `app_ignore_upgrade` filtering, `IsCrossVersion` guard, per-DB major-version constraint (MySQL/Postgres cluster branching), `PullImage`/`Backup`/`DeleteImage` options (`AppInstallUpgrade:97`), `DockerCompose` override (`handlers_appstore.go:51` install accepts none, upgrade accepts none). Also missing rollback previous compose manifest (`GitPreviousCompose` in compose path but not in app-store path). | |
| **RECOMMENDATION** | **ADAPT** — Add `GET /app-store/:id/update-versions` mirroring 1Panel's version list (derive from `catalog_entries.versions` or new `app_store_app_versions` table). Respect future `app_ignore_upgrade` table (see C10). Gate major jumps when `crossVersionUpdate=false`. Expose `Backup`/`PullImage` flags in upgrade request. | |
| **SEVERITY** | Medium — silent upgrade to breaking major is real risk for DB-kind apps. | |

---

### C10 — Ignore Upgrade

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `model/app_ignore_upgrade.go:3` `AppIgnoreUpgrade{AppID,AppDetailID,Scope}` + `api/v2/app_ignore_upgrade.go:16` `ListAppIgnored`, `34` `IgnoreAppUpgrade(Create)`, `56` `CancelIgnoreAppUpgrade(Delete)` + `service/app_ingore_upgrade.go:24` `List` (orphan cleanup if `app`/`appDetail` missing), `57` `CreateAppIgnore` (Scope `version→AppDetailID` else `all→Delete old` ), `dto/request/app_ignore_upgrade.go` | None — Forge has no `app_ignore_upgrade` table, service, handler, or UI. `page.tsx` shows unconditional `Upgrade` button for every `running` install. |
| **STATUS** | **MISSING** | Feature absent |
| **GAP** | 1Panel allows pinning an install: ignore a specific version (`scope=version`) or all upgrades (`scope=all`), affecting `GetUpdateVersions` filtering (`service/app_install.go:586` `appIgnoreUpgradeRepo.List(WithDetailId, WithScope=version)`). Forge's `UpgradeApp` cannot be suppressed; fleet-wide version bumps force immediate upgrades or noisy `canUpdate` everywhere. No `app_store_ignored_upgrades` migration exists. | |
| **RECOMMENDATION** | **ADOPT** — Add migration `app_store_ignore_upgrade(id UUID, app_key TEXT, app_version TEXT, install_id UUID, scope TEXT)` mirroring 1Panel but keyed by `app_store_installs.id` instead of `AppDetailID` (since Forge has single-version catalog). Add `POST /app-store/:id/ignore {scope,version}` and `GET /app-store/ignored`. Filter `ListInstalls`/`GetUpdateVersions` results. Low effort (3 endpoints, 1 table). | |
| **SEVERITY** | Low — nice-to-have UX; not correctness. | |

---

### C11 — Compose Template Management

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `model/compose_template.go:3` `ComposeTemplate{Name(unique),Description,Content}` + `api/v2/compose_template.go:18` `CreateComposeTemplate`, `40` `BatchComposeTemplate`, `62` `SearchComposeTemplate(SearchWithPage)`, `87` `ListComposeTemplate`, `106` `DeleteComposeTemplate(BatchDeleteReq)`, `128` `UpdateComposeTemplate(content,description)` + `dto/compose_template.go:5` `ComposeTemplateCreate/Batch/Update/Info` + `dto/common` `BatchDeleteReq` | **Forge compose-template** is superseded by two layers: (1) full **compose stack** lifecycle (`handlers_compose.go:315` `POST /compose`, `GET /compose`, `PATCH /compose/:id`, etc. with `compose_stacks` persisted, plus `handlers_compose.go:118` `POST /compose/import` + `513` `GET /compose/projects` legacy `ProjectDocument`), (2) **app-store seed** templates (`seed.go:27`). There is no `compose_templates` table. `forge/web/lib/api/compose.ts:49` `validateCompose` + `53` `createComposeStack` map to stack, not template. |
| **STATUS** | **DUPLICATE** (superseded) | Functionality exists in richer form; direct template CRUD is duplicated then extended |
| **GAP** | 1Panel's template is a static YAML snippet library (no deployment, no node binding, no GitOps). Forge's `compose_stacks` are deployed, health-checked, GitOps-aware, per-node, per-user-quota pieces. Mapping: `CreateComposeTemplate` → `POST /compose/import` (`handlers_compose.go:118`) which validates and stores as `ProjectDocument` with `ParsedConfig`, and `POST /compose` which immediately deploys. 1Panel's `Batch` import (`compose_template.go:40`) has no Forge equivalent (single import only via `ComposeImportRequest{Name,Content}` `compose.go:66`). Conversely, Forge's `BatchComposeTemplate` is missing: no bulk import. Forge's template delete is `DELETE /compose/:id` / `DELETE /compose/projects/:id` with reservation cancellation. | |
| **RECOMMENDATION** | **REJECT** — Do not recreate 1Panel's template table. Keep `compose_stacks`/`compose_projects` as richer superset. If batch import needed, add `POST /compose/import/batch {templates:[]}` trivial loop calling `ValidateCompose`. | |
| **SEVERITY** | Info — no action required. | |

---

### C12 — Resource Linking & Delete Guard (AppInstallResource / CatalogAttachLinks)

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `model/app_install_resource.go:3` `AppInstallResource{AppInstallId,LinkId,ResourceId,Key,From(local/remote)}` + `service/app_install.go:664` `DeleteCheck(installID)` (websites + `appInstallResourceRepo.WithLinkId+local` → `dto.AppResource{Type:website/app, Name}`), `service/app.go:541` `createLink` at install time, `service/app_install.go:650` `ChangeAppPort` re-iterates linked resources to restart dependents, `442` uniqueness enforcement across `LinkId` | `store/store_catalog.go:52` `CatalogAttachLink{Id,CatalogInstanceID,EnvironmentID,VarPrefix}`, `store/store_catalog.go:231` `CreateCatalogAttachLink` (ON CONFLICT DO UPDATE var_prefix), `catalog/inject.go:128` `Attach` (+ `InjectAndInjectConnector`), `store/store_apphosting.go:718` `AppBelongsToOrg` + `AppServiceBelongsToApp` guards. App-store installs have **no** resource guard; `DeleteAppStoreInstall:182` has no dependency check. |
| **STATUS** | **PARTIAL** | Pattern re-appears as `catalog_attach_links` for catalog instances but NOT for `app_store_installs` |
| **GAP** | 1Panel's generic linking tracks any app depending on another (e.g., app using a database app's `PANEL_DB_HOST` param). Foreground Forge splits world: `app_store_installs` are standalone compose stacks with `compose_project_id TEXT` FK but no `ON DELETE RESTRICT` and no `DeleteCheck` equivalent. Deleting an app-store install that a `catalog_instance` or `application` references leaves dangling `host`/`conn_string`. Conversely, Forge's `catalog_attach_links` correctly implements link tracking for catalog-provisioned services (linking env injection) with `idempotent ON CONFLICT`. Recommendation is to apply same pattern to app-store. | |
| **RECOMMENDATION** | **ADAPT** — Before `DeleteAppStoreInstall`, query `catalog_attach_links` and `applications` referencing `compose_project_id` / `app_key`, and return `409 {resources}` mirroring 1Panel's `AppResource[]` shape. Alternatively add `app_store_install_resources` table genericizing the link. Low priority but prevents orphaned env vars. | |
| **SEVERITY** | Medium — data integrity (orphan env vars with wrong conn strings). | |

---

### C13 — Port & Connection Info Management

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app_install.go:92` `LoadPort(dto.OperationWithNameAndType{Name,Type})` → `service/app_install.go:177` `LoadPort(LoadBaseInfo)` returns `app.Port int64`; `113` `LoadConnInfo` returns `DatabaseConn{Status,Username,Password,ServiceName,Port,ContainerName}`; `232` `ChangeAppPort(PortUpdate{Key,Name,Port})` with `common.ScanPort`, `updateInstallInfoInDB` + restart of linked `app_install_resource` peers | Forge has **no** `LoadPort`/`LoadConnInfo`/`ChangeAppPort`. Port is exposed only via `ComposeStack`'s `ServiceState{Ports}` from `beacon/internal/server/compose.go:632` `docker compose ps --format json` (raw string `Ports`), and via `catalog_instances.port` (int) + `conn_string`. No mutation API. |
| **STATUS** | **MISSING** | Forge defers port/config to compose/env |
| **GAP** | 1Panel's port management rewrites `.env` (`PANEL_APP_PORT_HTTP=`), updates `param/env JSON`, and `compose.Down/Up` the app plus dependent installs. Forge's app-store `Params` are free-form `map[string]string` merged into `EnvVars` at deploy (`service/appstore/service.go:108` `EnvVars: req.Params`) and baked into compose via `ExpandTemplate`, but not re-extractable as typed `LoadPort`. Secrets like `PANEL_DB_ROOT_PASSWORD` mutation via `updateInstallInfoInDB` (`service/app_install.go:866`) is DB-specific magic strings (`PANEL_REDIS_ROOT_PASSWORD=`) — not portable. | |
| **RECOMMENDATION** | **REJECT** — Forge's compose model should expose ports via parsed `ParsedConfig.services[].ports` (already stored `ParsedConfig jsonb` in `compose_stacks`) rather than 1Panel's magic env key convention. Add `GET /app-store/:id/conn-info` that reads `compose_stacks` → `ComposeStatusResponse.Services[].Ports` if needed, but don't copy `PANEL_APP_PORT_*` indirection. | |
| **SEVERITY** | Low — not applicable; compose env/port handles it. | |

---

### C14 — Params / Configuration & Docker Compose Editing (Advanced Mode)

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app_install.go:252` `GetDefaultConfig({Type,Name})` (reads `conf/my.cnf`/`redis.conf`/`nginx.conf` from app resource dir) + `274` `GetParams(appInstallId)` → `response.AppConfig{Params []AppParam, RawCompose, AppContainerConfig, HostMode, RestartPolicy, WebUI}` via `json Unmarshal(detail.Params→AppForm)`, env merge, `getAppCommonConfig`, `isHostModel`; `298` `UpdateInstalled(AppInstalledUpdate{Params,AppContainerConfig(editCompose,dockerCompose,containerName,cpuQuota...)})` → rewrites `.env`, `docker-compose.yml`, `rebuildApp` with rollback on failure, plus proxy pass nginx reconciliation (`service/app_install.go:431` `hasAppInstallProxyPassChanged` → `nginx.WriteConfig` + `nginxCheckAndReload`) | **Forge app-store:** `page.tsx:554` `app.params` rendered as form (`InstallFormModal`) but read-only after install; no `GetParams` equivalent. **Forge compose:** `handlers_compose.go:378` `PATCH /compose/:id` → `services/compose/lifecycle.go:476` `UpdateComposeStack` (hash check, `DeployComposeWaitForHealthy`, `rollbackStack` on failure). **Forge apphosting compose:** `handlers_apphosting.go:865` `PUT /apps/:id/compose {sourceConfig}` validates via `ValidateCompose` then `UpdateApplication(SourceConfig)` — previously bypassed security (now fixed `handlers_apphosting.go:891` ValidateCompose check). |
| **STATUS** | **PARTIAL** | Compose editing exists and is safer; DB-config `GetDefaultConfig` absent; proxy reconciliation absent |
| **GAP** | 1Panel's `UpdateInstalled` is the most complex operation: it handles container rename collision, port change validation, docker-compose common param injection (`addDockerComposeCommonParam` adds `cpus/memory/limits/gpu/ip`), `.env` merge (`godotenv.Read` + `handleMap`), atomic rollback on `rebuildApp` failure (backup maps/compose), and nginx proxy auto-update for website-linked apps. Forge's `UpdateComposeStack` is simpler but stronger on the core loop (hash dedupe, `WaitForHealthy`, `rollbackStack` with health). Missing pieces in Forge app-store context: `GetDefaultConfigByKey` for DB configs, `HostMode`/`RestartPolicy` surfacing, `ContainerName` collision guard (Forge generates `cps-<12>` stable IDs, not user-supplied names, so collision not an issue), `AllowPort`/`CpuQuota`/`MemoryLimit` injection via compose `deploy.resources` (Forge validates but doesn't synthesize). | |
| **RECOMMENDATION** | **ADAPT — selectively.** Add `GET /app-store/:id/params` exposing `AppDetailDTO.params` raw schema + current `AppStoreInstall.params` values (mirrors `GetParams`). Do NOT adopt `GetDefaultConfig` filesystem conf reads (Forge compose contents are self-contained). For advanced compose editing, reuse `PATCH /compose/:id` validation+rollback pattern — do NOT add nginx proxy reconciliation (Forge handles via `proxy_domains` separate). | |
| **SEVERITY** | Low — power-user feature; standard install path complete. | |

---

### C15 — Icon Serving & Caching

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `app.go:213` `GetAppIcon` → `service/app.go:1023` `GetAppIcon(key)` : if `appicon.IsIconFile(app.Icon)` → `ReadIconFile` with `ETag`, else `base64.StdEncoding.DecodeString`; handler sets `Cache-Control: public, max-age=2592000`, `304 NotModified` on `If-None-Match==etag`, `ContentTypePNG`, `204 NoContent` when empty | `store/store_app_store.go:6` `AppStoreApp.Icon` is plain URL string (`https://cdn.jsdelivr.net/npm/simple-icons/...` in `seed.go:32`). `page.tsx:272` renders `<Image src={app.icon} unoptimized>` remote URL directly — no API serving, no caching headers, no ETag. `handlers_appstore.go` has no `/apps/icon/:key` endpoint. |
| **STATUS** | **UNWIRED** (present differently) | Icons served externally, not via API |
| **GAP** | 1Panel's icon path supports both file-backed icons (`appicon.IconFileExists` check in `GetAppUpdate:869` triggers `CanUpdate`) and inline base64. Forge's CDN URL avoids storage but introduces SPOF (cdn.jsdelivr) and no offline/air-gap support; also `Image` unoptimized flag disables Next.js optimization. Cache semantics missing: 1Panel's 30-day `Cache-Control` + `ETag` reduces app-list bandwidth (gzip flag `SuccessWithDataGzipped` at `app.go:32` — not mirrored in Forge's `ListApps` JSON). | |
| **RECOMMENDATION** | **INSPIRE** — Keep URL approach for now (simpler). Add `GET /app-store/apps/:key/icon` proxy+cache if air-gapped deployments required: fetch icon URL, cache in `object_storage` or local disk, serve with `ETag`/`Cache-Control` mirroring 1Panel. Otherwise no action. | |
| **SEVERITY** | Info — cosmetic/operations. | |

---

### C16 — Container/Stack Status Sync & Observability

| Dimension | 1Panel Reference | Forge |
|-----------|-----------------|-------|
| **Reference** | `service/app_install.go:472` `SyncAll(systemInit bool)` (handles `StatusInstalling/Upgrading/Rebuilding/Uninstalling → Error` on init, `WaitingRestart→Up`, else `syncAppInstallStatus`), `service/app_install.go:835` `syncAppInstallStatus` → `docker.NewClient().ListContainersByName(containerNames)` + `synAppInstall(containersMap, appInstall, force)` ; `GetAppInstallInfo:958` calls `syncAppInstallStatus` before return | **Forge compose:** `beacon/internal/server/compose.go:604` `handleComposeStatus` (`docker compose ps --format json`), `services/compose/lifecycle.go:188` `WaitForHealthy` polling `ComposeStatus` every 5s for 2m with restart-count bump guard; `services/catalog/catalog.go:168` `UpdateCatalogInstanceStatus` maps to `ref.Status`. **Forge app-store:** `UpdateAppStoreInstallStatus` stores `error_message` but `ListInstalls` never re-syncs live container state; `GetAppInstallInfo` analogue missing (no live status). |
| **STATUS** | **PARTIAL** | Health polling exists for compose; app-store install status is last-write-wins |
| **GAP** | 1Panel's `SyncAll` reconciles DB status with Docker reality on `SyncInstalled` and on startup (`systemInit`). Forge's `app_store_installs.status` is only updated at install/upgrade/uninstall time (`service/appstore/service.go:118` `error→running|error`). No periodic reconciler: if beacon restarts container out-of-band, `app_store_installs.status` drifts. `WaitForHealthy` is stronger than 1Panel's best-effort `syncAppInstallStatus` (checks `Names[0]` map) but only runs at deploy/update time, not as background loop. | |
| **RECOMMENDATION** | **ADAPT** — Add `GET /app-store/:id/sync` → `GetStackStatus` → update `app_store_installs.status` (mirrors `app_install.go:289` `Sync` op). Optionally schedule `catalog retention worker` style periodic sync for app-store installs (`ListAppStoreInstalls` + `GetStackStatus` per `compose_project_id`). | |
| **SEVERITY** | Medium — operational drift under failure. | |

---

## 4. FORGE LOGIC FINDINGS (≥3)

All findings are evidenced — each cites Forge vs 1Panel lines.

### LF-01 — Silent Template Bypass on Interpolation Failure (Deploy-With-Stale Compose)

- **Forge:** `services/appstore/service.go:239` `resolveTemplate`:
  ```go
  func resolveTemplate(tmpl string, params map[string]string) string {
      out, err := compose.ExpandTemplate([]byte(tmpl), params)
      if err != nil { return tmpl } // ← fallback to raw template
      return string(out)
  }
  ```
  `services/compose/service.go:388` `${VAR:?msg}` and `${VAR?msg}` raise `required variable "X" is unset` when var missing. The app-store path swallows this and deploys the unexpanded template (e.g., `postgres:16` stays `${POSTGRES_VERSION:-16}` literal or, worse, `${REQUIRED:?}` literal). The subsequent `DeployComposeStack` (`lifecycle.go:264` `ValidateCompose`) may still pass because it sees the default syntax as valid YAML, so a mis-configured install reaches beacon as a broken compose.
- **Reference:** `services/app.go:347` `Install` fails fast on param/compose errors (`yaml.Unmarshal` error → `return err`), never silently substituting. Also `dto/app.go:138` `AppForm.Required bool` is enforced at API via `helper.CheckBindAndValidate`.
- **Impact:** Mis-typed param keys or missing `required` vars produce a compose that deploys but with wrong image (`postgres:-16`) or fails later with obscure docker error instead of immediate 422.
- **Severity:** High (correctness)
- **Fix:** Change `resolveTemplate` to return `([]byte,error)` and surface `?`/` :?` errors as `422 Unprocessable Entity`. Alternatively validate required params before expansion: `for _, f := range app.Params.FormFields { if f.Required && params[f.EnvKey]=="" → 400 }`.

### LF-02 — Uninstall Orphan Race (DB Delete Before Node Confirmation)

- **Forge:** `services/appstore/service.go:139` `UninstallApp`:
  ```go
  if inst.ComposeProjectID != "" {
      if err := s.composeSvc.DeleteComposeStack(ctx, inst.ComposeProjectID); err != nil {
          slog.Warn("uninstall: delete compose stack", ...) // swallow
      }
  }
  _ = s.store.UpdateAppStoreInstallStatus(ctx, installID, "uninstalling", "")
  return s.store.DeleteAppStoreInstall(ctx, installID) // always deletes row
  ```
  Delete succeeds even when `DeleteComposeStack` fails (node down, beacon 409 with `output`, network partition). Row gone but containers remain on node, with lingering volume `postgres-{name}-data` and reservation `reservation_id` not cancelled (cancelled inside `DeleteComposeStack:607` only on success).
- **Reference:** `service/app_install.go:246` `Operate(Delete)` builds `request.AppInstallDelete{ForceDelete,DeleteBackup,...}` and `deleteAppInstall(deleteReq)` is a transactional task with `ForceDelete` guard; `app_install.go:134` `DeleteCheck` prevents deletion when dependents exist. `app_install.go:257` `Delete` case returns error when not `ForceDelete`.
- **Impact:** "uninstall" appears successful (200 `{data:"ok"}` in `handlers_appstore.go:111`) while stack still consumes reservation/memory on node; subsequent re-install with same `name` may collide on volume or `postgress-{name}` service name. No idempotent retry (row gone).
- **Severity:** High (data/lease leak)
- **Fix:** Only `DeleteAppStoreInstall` after `DeleteComposeStack` success; on failure persist `error` on install row and return `502 Bad Gateway` (retain row for retry). Add `?force=true` query param mirroring `ForceDelete` to allow forced row delete after user confirmation. Also call `cancelReservation` on orphan path.

### LF-03 — Upgrade Ignores Version Semantics & Scope (Single-Version Assumption)

- **Forge:** `services/appstore/service.go:158` `UpgradeApp`:
  ```go
  app, _ := s.store.GetAppStoreApp(ctx, inst.AppKey) // single row
  resolvedCompose := resolveTemplate(app.ComposeContent, params) // always latest
  ```
  No `AppDetail` lookup, no `crossVersionUpdate` guard, no `app_ignore_upgrade` check, no `version` pinning. Caller cannot choose target version (e.g., `postgres 15 → 16` major bump blocked by 1Panel when `CrossVersionUpdate=false`). Also no `PullImage`/`Backup` flags.
- **Reference:** `service/app_install.go:571` `GetUpdateVersions` filters:
  ```go
  ignores, _ := appIgnoreUpgradeRepo.List(WithDetailId(detail.ID), WithScope("version"))
  if len(ignores)>0 { continue }
  if common.IsCrossVersion(install.Version, detail.Version) && !app.CrossVersionUpdate { continue }
  if app.Key==mysql && !strings.HasPrefix(detail.Version, getMajorVersion(install.Version)) { continue }
  ```
  plus `model/app.go:24` `CrossVersionUpdate bool`.
- **Impact:** For DB kinds (`postgres`, `mysql` 8.0→8.3) Forge will silently apply breaking major upgrades; no way to ignore a bad release. Also `upgrade` overwrites `inst.AppVersion = app.Version` before confirming compose deploy success — partial failure leaves DB row claiming new version while containers still run old image (no rollback manifest like `GitPreviousCompose` in compose GitOps).
- **Severity:** Medium (correctness for stateful workloads)
- **Fix:** Introduce `app_store_app_versions` or at minimum enforce `app.CrossVersionUpdate` analogue per catalog entry (add `cross_version_update BOOLEAN` to `app_store_apps`). Add `UpgradeRequest{targetVersion}` and list endpoint `GET /app-store/:id/update-versions`. Gate `IsCrossVersion(appVersion, targetVersion)` check.

### LF-04 — App-Store Install Lacks Ownership Check on `ListInstalls` Pagination Leak (bonus, medium)

- **Forge:** `services/appstore/service.go:53` `ListInstalls`:
  ```go
  if userID == "" { return s.store.ListAppStoreInstalls(ctx) } // admins get all
  return s.store.ListAppStoreInstallsForUser(ctx, userID)
  ```
  `handlers_appstore.go:114` determines `userID=""` for admins (`claims.Role==RoleAdmin → userID remains ""`), so admins see all installs. Non-admin `userID` is set, but `InstallApp` (`handlers_appstore.go:69` `claims.Sub`) allows any user to install with arbitrary `UserID` override if request body contains `userId` — handler only fills when `req.UserID==""`. An attacker with `user` role can set `{"userId":"victim-id", ...}` to create install as another user, then victim cannot delete it (perm check `inst.UserID != actorID` in `UninstallApp:144` fails). Also `ListAppStoreInstallsForUser` partition leaks no org scoping (vs `applications` which use `org_id`).
- **Reference:** `app_install.go:19` `SearchAppInstalled` uses tenant-agnostic list but is single-tenant panel; Forge must enforce org isolation. 1Panel has no multi-tenant leak surface.
- **Severity:** Medium (authz)
- **Fix:** In `handlers_appstore.go:51` overwrite `req.UserID = claims.Sub` unconditionally (ignore client-sent value). Add `OrgID` enforcement via `resolveDefaultOrg` pattern used in `handlers_apphosting.go:31`.

### LF-05 — Category/Search SQL Injection-Adjacent Concatenation Pattern (bonus, low)

- **Forge:** `store/store_app_store.go:48` builds query via `fmt.Sprintf("%d", argIdx)` and `args` — safe as parametric. Not a vulnerability, but `search` does `"%"+search+"%"` without escaping `%`/`_` wildcards, so a search for `100%` matches `100X`. 1Panel uses `repo.WithByLikeName(strings.TrimSpace(req.Name))` with library `ilike ?` with escaped pattern (assumed). Minor.
- **Severity:** Low.

---

## 5. Cross-Cutting Observations

- **Two catalogs, one truth problem:** Forge maintains `app_store_apps` (simple `version` string, `compose_content` blob, `tags TEXT[]`) and `catalog_entries` (rich `versions JSONB`, `default_version`, `requires JSONB`, `sort_order`) as parallel catalogs with overlapping keys (`postgres`, `redis`, `mysql`). They are not kept in sync (`seed.go:27` vs `175_catalog_entries.sql` seeds diverge: `app_store` seeds `postgres 16` + `redis 7` etc., `catalog_entries` seeds `postgres 16/15/14` + `redis 7/6` etc.). Consider consolidating: derive `app_store_apps` from `catalog_entries` + `composeTemplateFor` (as `catalog/catalog.go:241` does) rather than maintaining duplicate seed lists.
- **Compose is the unified runtime:** 1Panel's `ComposeTemplate` is a library snippet; Forge's `compose_stacks` already unify app-store and catalog provisioning (both call `DeployComposeStack`). This is architecturally superior — do not reintroduce app-specific deploy paths.
- **Status model convergence:** 1Panel uses string statuses `StatusInstalling/Running/Error/Uninstalling` inline on `AppInstall`; Forge splits `app_store_installs.status` + `compose_stacks.status` (`deploying/awaiting_health/running/stopped/degraded/updating/rolling_back/deleting`). Mapping `degraded|failed→error` (`service/appstore/service.go:129`) is correct. Add `awaiting_health` propagation.
- **Beacon re-validation is defense-in-depth done right:** `beacon/internal/server/compose.go:77` `validateComposePolicy` is stricter than `services/compose/service.go:427` `ValidateComposeSecurity` on `cap_add`/`devices`/`bind mounts` (beacon rejects all non-empty vs API warns). This mirrors 1Panel's lack of beacon-side re-validation gap — keep.

---

## 6. Recommendations Summary

| # | Recommendation | Priority |
|---|----------------|----------|
| R01 | Make `resolveTemplate` fail-fast on `?` required-var errors | P0 |
| R02 | Fix uninstall orphan race: confirm node delete before DB delete; add `?force` | P0 |
| R03 | Add `GET /app-store/:id/update-versions` + `crossVersionUpdate` guard; version-pin upgrade | P1 |
| R04 | Enforce server-side `UserID=claims.Sub`, add org scoping to `ListInstalls` | P1 |
| R05 | Add `AddVersion` history or consolidate `app_store_apps` ← `catalog_entries` | P1 |
| R06 | Wire tag filter `WHERE tags && $3` + `GET /app-store/tags` | P2 |
| R07 | Add pagination (`page/perPage` → `PageResult{Items,Total}`) to `ListApps`/`ListInstalls` | P2 |
| R08 | Add `DeleteCheck` dependency guard for `app_store_installs` | P1 |
| R09 | Add `app_store_ignore_upgrade` table if pinning UX needed | P2 |
| R10 | Add async sync task + `GET /check-update` with `CanUpdate/IsSyncing` | P2 |
| R11 | Expose `GET /app-store/:id/params` + `GET /app-store/:id/sync` health re-sync | P2 |
| R12 | Document fragment vs unify lifecycle routes (keep but document) | P3 |

---

## 7. Status Legend

- **COMPLETE** — Par or superset of 1Panel feature.
- **PARTIAL** — Works for happy path, gaps on edge/filtering/validation.
- **UNWIRED** — UI/API/service exists but not connected.
- **BROKEN** — Exists but violates invariant (see LF).
- **MISSING** — Feature absent; no table/route/service.
- **DUPLICATE** — Feature superseded by richer generic (keep generic).
- **FALSE_COMPLETION** — Endpoint returns 200 but silently fails invariant.

Overall: Compose subsystem is **COMPLETE** (and superior to 1Panel's template library); Catalog is **PARTIAL** but well-designed; App Store is **PARTIAL** with **2 BROKEN** edges (LF-01, LF-02) and several **MISSING** controls intentionally omitted (favorites, ignore-upgrade).

---

## 8. File Line Map (Quick Reference)

- `reference/app-platforms/1panel/agent/app/api/v2/app.go:22` SearchApp / `42` SyncApp / `74` SyncLocalApp / `91` GetApp / `115` GetAppDetail / `162` InstallApp / `181` GetAppTags / `196` GetAppListUpdate / `213` GetAppIcon
- `reference/.../app_install.go:19` SearchAppInstalled … `173` OperateInstalled
- `reference/.../app_ignore_upgrade.go:16` ListAppIgnored `34` IgnoreAppUpgrade
- `reference/.../compose_template.go:18` CreateComposeTemplate
- `reference/.../model/app.go:13` App struct `44` IsLocalApp
- `reference/.../model/app_detail.go:3` AppDetail
- `reference/.../model/app_install.go:11` AppInstall
- `reference/.../service/app.go:69` PageApp `196` GetApp `234` GetAppDetail `347` Install `596` SyncAppListFromLocal `828` GetAppUpdate `968` SyncAppListFromRemote
- `reference/.../service/app_install.go:79` Page `177` LoadPort `246` Operate `571` GetUpdateVersions
- `forge/api/internal/http/handlers_appstore.go:11` registerAppStoreRoutes `23` listAppsHandler `40` GetApp `51` Install `100` Uninstall `114` ListInstalls `126` Upgrade `144` Sync
- `forge/api/internal/services/appstore/service.go:20` Service `41` ListApps `45` GetApp `74` InstallApp `139` UninstallApp `158` UpgradeApp `204` SyncFromRemote `239` resolveTemplate
- `forge/api/internal/store/store_app_store.go:10` AppStoreApp `30` AppStoreInstall `48` ListAppStoreApps
- `forge/api/internal/services/catalog/catalog.go:61` Service `119` Provision
- `forge/api/internal/services/compose/lifecycle.go:249` DeployComposeStack `476` Update `570` Delete
- `forge/api/internal/daemon/compose.go:48` ComposeDeploy
- `forge/beacon/internal/server/compose.go:77` validateComposePolicy `344` handleComposeDeploy
- `forge/web/app/admin/app-store/page.tsx:27` AppStorePage
- `forge/web/lib/api/app-store.ts:51` apiFetch

---

*No product code modified. All claims inspected before writing.*
