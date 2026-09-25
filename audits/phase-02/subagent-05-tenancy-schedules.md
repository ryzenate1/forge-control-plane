# Subagent 05 — TENANCY / RBAC / SUBUSERS / PERMISSIONS / SCHEDULES / CRON / DATABASES / MOUNTS

**Cluster:** same 6  
**Dimension:** subusers / roles / permissions / server ownership / admin vs user / node allocations / API keys / schedules & cron tasks / server databases / mounts / isolation / policy validation  
**Date:** 2026-08-23  
**Auditor:** subagent-5 (Phase 02)

---

## 1. Scope & References Inspected

### Forge (current)
- `forge/api/internal/store/permissions.go:1-228` — canonical permission constants, `AllPermissions()`, `HasPermission()`, `IsSensitivePermission()`
- `forge/api/internal/store/store_users.go:88-1303` — `Authenticate`, `ListServerSubusers`, `UpsertServerSubuser:297`, `GetServerSubuser:337`, `UserCanAccessServer:353-396`, `AuthenticateSFTP:410-542`, `normalizeSubuserPermissions:544`, `defaultSubuserPermissions:563`
- `forge/api/internal/store/store_tenancy.go:1-654` — `Organizations/Projects/Environments/TeamMembers`, `UserIsOrgMember`, `GetTeamMemberRole`, `ResolveEffectiveOrgRole`, `UserCanAccessOrgResource`, `ServerBelongsToOrg`
- `forge/api/internal/store/store_mounts_ext.go:1-534` — `ListMounts`, `CreateMount:41`, `validateMountPaths:316`, `validateMountPath:323`, `ensureMountAvailableForServer:297`, `AssignMountToServer:272`, `AllowedMountSourcesForNode`
- `forge/api/internal/store/store_schedules.go:1-725` — `ListSchedules`, `CreateSchedule:166`, `CreateScheduleTask:252`, `PatchScheduleTask:331`, `isValidScheduleTaskAction` (in `store.go:1182`), `ClaimDueSchedule` lease path
- `forge/api/internal/store/store_cron_jobs.go:1-325` — `CreateCronJob`, `UpdateCronJob`, `dispatch targetType/targetId`, `CompleteCronJobExecution`
- `forge/api/internal/store/store_allocations.go:1-391` — `AssignAllocationToServer:327`, `SetPrimaryAllocation:367`, `DeleteAllocations` atomic batch
- `forge/api/internal/store/store_database_services.go:1-406` — `CreateDatabaseService`, `UpdateDatabaseServiceServerID`
- `forge/api/internal/store/store_envvars.go:1-499` — `CreateEnvironmentVariable`, `ResolveEnvironmentVariables`, `GetTeamMemberPermissions:418`, `SetTeamMemberPermissions:470`
- `forge/api/internal/policies/server_policy.go:1-94`, `policies/policy.go:1-51`, `policies/environment_policy.go:1-77`, `policies/project_policy.go:1-73`, `policies/user_policy.go`, `policies/node_policy.go`
- `forge/api/internal/http/auth.go:430-598` — `requireRole`, `requireServerPermission`, `checkServerPermission:558`, `enforceRoleRulesForRequest`
- `forge/api/internal/http/handlers_servers.go:46-3754` — `hasServerReadEnv:52`, `redactStartupSecrets:71`, subuser CRUD `:id/users` `541-632`, allocation `454-698`, schedule CRUD `1139-1373`, `validateServerScheduleCron:3693`, backup name `91`
- `forge/api/internal/http/handlers_user_console.go:314-514` — `registerServerCronJobRoutes` (server-scoped cron, `PermCron*`)
- `forge/api/internal/http/handlers_cronjob.go:1-238` — `validateCronSchedule:15`, `registerCronJobRoutes` (admin-only `/cron-jobs`)
- `forge/api/internal/http/handlers_database_services.go:1-406` — `registerDatabaseServiceRoutes` (all admin-gated)
- `forge/api/internal/http/handlers_admin.go:1579-1871` — mount admin CRUD, `CreateMount` validation, `DeleteMount` guard, `AssignMountToServer`
- `forge/api/internal/http/schedule_runner.go:1-439` — `tick`, `runClaim`, `executeTask:386` (`power|backup|command`), `waitOffset` lease renewal
- `forge/api/internal/services/cronjob/service.go:1-305` — `runShellCommand:212`, `dispatchServerCommand:240`, `executeJob:117`, `runShellCommand` scrubbed env
- `forge/api/internal/services/dbprovisioner/service.go:1-745` — `Provision`, `RotatePassword`, `quotePostgresIdentifier`, `quoteMySQLIdentifier`
- `beacon/internal/cron/cron.go:1-122`, `beacon/internal/cron/cleanup.go:1-237`, `beacon/internal/cron/sftp.go`, `beacon/internal/rootfs/rootfs.go`

### Pterodactyl Panel (reference)
- `reference/game-hosting/pterodactyl-panel/app/Models/Subuser.php:1-95` — `$casts permissions=>array`, guarded fields
- `reference/game-hosting/pterodactyl-panel/app/Models/Permission.php:1-219` — 10 groups, 45+ constants `ACTION_*`, `permissions()` map, `RESOURCE_NAME=subuser_permission`
- `reference/game-hosting/pterodactyl-panel/app/Models/Server.php:1-419` — `owner_id`, `subusers()`, `validateCurrentState()`, `Mounts` via `MountServer`
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/GetUserPermissionsService.php:1-34` — owner/admin => `['*']`, else subuser row
- `reference/game-hosting/pterodactyl-panel/app/Services/Databases/DatabaseManagementService.php:1-177` — `MATCH_NAME_REGEX s{serverId}_`, `TooManyDatabasesException`, remote create with rollback
- `reference/game-hosting/pterodactyl-panel/app/Services/Schedules/ProcessScheduleService.php:1-86` — `is_processing` lock, `only_when_online` daemon state check
- `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/SubuserController.php:1-169` — `getDefaultPermissions()` intersect+websocket, implicit owner/permission-subset enforcement, `RevokeSftpAccessJob`
- `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/ScheduleController.php:1-199` — `authorizeTasks():158` per-task permission check, `getNextRunAt()`

### Wings / Beacon & PufferPanel
- Beacon `internal/rootfs` + `internal/cron` provides interval jobs, not Pterodactyl Wings' Docker-isolated schedule runner; Wings permission checks are daemon-side `ServerPermission` middleware.
- PufferPanel reference not vendored; comparison inferred from PufferPanel `server.reload` / daemon config sync pattern reproduced in Forge `POST /servers/:id/reload` (`handlers_servers.go:438`).

---

## 2. Comparison Matrix (18 comparisons)

| # | Aspect | Pterodactyl Panel | Forge | Assessment |
|---|--------|-------------------|-------|------------|
| 1 | **Permission catalog** | `Permission.php:18-66` defines 10 groups (websocket/control/user/file/backup/allocation/startup/database/schedule/settings/activity) with 45 constants. Source of truth is `Permission::permissions()` map `101-209`. | `permissions.go:5-86` reproduces all 10 plus adds `cron.read/create/update/delete/run` (`28-33`), `buildpack.manage` (`38`), `mount.read/update` (`79-80`), `server:read-env` (`72`), `server.view/settings`, `settings.description/change-icon`. `AllPermissions()` enumerates 40+ keys. | **Parity+** — Forge strictly superset. No missing Pterodactyl permission. Extra groups are intentional (cron replaces Pterodactyl's schedule.* at node level, mount and buildpack are new). Risk: `PermServerView`/`PermServerSettings` are defined but absent from `AllPermissions()` — normalize will drop them if ever assigned via subuser flow. LOW. |
| 2 | **Wildcard `*` semantics** | `GetUserPermissionsService.php:18` returns `['*']` exclusively for `root_admin` or `owner_id === user.id`. Never stored in `subusers.permissions`; computed at request time. | `permissions.go:206-215` `HasPermission` honors `p=="*"` as wildcard. `store_users.go:544-561` `normalizeSubuserPermissions` explicitly allows `permission=="*"` (`!allowed[permission] && permission != "*"` continue). Owner/admin path in `UserCanAccessServer:373` short-circuits before row check; subusers can hold `*` in DB if assigned. | **Divergence — HIGH.** Pterodactyl never persists `*` for subusers. Forge persists it if any `user.update/create` caller supplies it. See Finding F-01. |
| 3 | **Sensitive-value masking** | `DatabaseController` gates `view_password` via `Permission::ACTION_DATABASE_VIEW_PASSWORD`; Blade redacts otherwise. Env values not exposed via separate scope. | `permissions.go:218` `IsSensitivePermission` flags `database.view_password` + `server:read-env`. Handlers redacting: `handlers_servers.go:52` `hasServerReadEnv` + `71` `redactStartupSecrets` masks `ServerValue`/`StartupCommand` to `"********"`; DB routes call `hasServerDatabaseViewPassword:3726`. | **Improvement.** Forge adds explicit `server:read-env` (no Pterodactyl equivalent). Correctly fails closed (owner/admin bypass, else require explicit grant). |
| 4 | **Subuser permission subset enforcement** | `SubuserController.php:154-168` `getDefaultPermissions()` intersects request with `Permission::permissions()` allowlist, always adds `websocket.connect`. Service layer `SubuserCreationService` checks actor can only grant permissions they themselves possess and cannot edit own row (`"They will never be able to edit their own account, or assign permissions they do not have themselves."` — `Permission.php:120`). | `store_users.go:297-335` `UpsertServerSubuser` only calls `normalizeSubuserPermissions` (allowlist + `*`). `handlers_servers.go:541` `POST /servers/:id/users` checks `requireServerPermission(PermUserCreate)` then `CheckUserCanCreateSubuser` (cap), but never validates `req.Permissions ⊆ caller.Permissions`. Same for `PATCH :581` (`PermUserUpdate`). | **Missing — HIGH.** No subset check. Any holder of `user.create`/`user.update` can escalate to arbitrary permissions including `*`. See F-01. |
| 5 | **Owner-as-subuser guard** | `SubuserCreationService` throws `UserIsServerOwnerException`. | `store_users.go:321-322` `if userID == ownerID { return "server owner cannot be added as a subuser" }`. | **Parity.** Both reject. |
| 6 | **Allocation node-affinity** | `Allocation` model validates `node_id` matches server node before attach; panel/UI enforces same. | `store_allocations.go:327-348` `AssignAllocationToServer` loads server `node_id`, allocation `node_id`+`server_id`, rejects `already assigned` and `does not belong to server node`. `store_servers.go:227-243` same checks at server creation with `FOR UPDATE` lock. | **Parity+.** Forge adds `FOR UPDATE` on allocation rows at create, closes TOCTOU race present in Pterodactyl. |
| 7 | **Primary allocation protection** | Pterodactyl prevents unassigning primary allocation; UI warns. | `store_allocations.go:350-365` `UnassignAllocationFromServer` pre-loads `primary_allocation_id` and rejects if `== allocationID`. `store_allocations.go:214-248` `DeleteAllocations` only deletes where `server_id IS NULL`, so primary or any assigned allocation cannot be bulk-deleted. | **Parity, stricter.** Forge also makes bulk delete atomic (transaction rolls back entire batch on one failure `238`) vs Pterodactyl per-row. |
| 8 | **Mount path traversal / confinement** | Pterodactyl `Mount` model validates `source`/`target` are absolute, not `/`, not overlapping (`/home/container`). Wings re-validates on daemon. | `store_mounts_ext.go:316-339` `validateMountPaths` / `validateMountPath`: `path.IsAbs`, `path.Clean(value)==value`, no `\\`, no `..` segment, reserved `source ∈ {/etc/forge,/var/lib/forge/volumes}` blocked, `target ∈ {/,/home/container}` blocked. `ensureMountAvailableForServer:297` double-join `mount_node ∧ egg_mount` required. Beacon `AllowedMountSourcesForNode:367` allowlists sources per node; `cleanup.go:139-162` `safeToRemove` adds symlink/ownership checks. | **Partial parity — MEDIUM gap.** Reserved list is too narrow (does not block `/etc`, `/var/run/docker.sock`, `/proc`, `/`). See F-03. Double-join eligibility is stronger than Pterodactyl's simple mount assignment. |
| 9 | **Mount delete lifecycle** | Pterodactyl deletes mount then pings Wings to unmount; if wings down, orphan mount remains. | `handlers_admin.go:1638-1684` `DELETE /mounts/:id` first checks `CountServersUsingMount>0` → 400, then loads nodes, deletes DB row via `DeleteMount:253` (no FK cascade check), then best-effort `Daemon.CleanupMount` per node with token lookup. Order is correct (DB first, then daemon) but delete does not run in same tx as count check — race window where concurrent attach could slip after check. | **Near parity, minor race.** Pattern matches Pterodactyl but adds per-node cleanup logging. Recommend `SELECT FOR UPDATE` or FK restrict. LOW-MEDIUM. |
| 10 | **Schedule cron admission validation** | `ScheduleController.php:185-198` `getNextRunAt()` calls `Utilities::getScheduleNextRunDate` and throws `DisplayException` → 422 if cron invalid (5 fields). | `handlers_servers.go:1170` `POST /schedules` calls `validateServerScheduleCron:3693` using `robfig/cron` parser before `CreateSchedule`. PATCH path validates if any cron field present (`1207-1233`). `handlers_cronjob.go:15-34` `validateCronSchedule` does same for global cron jobs plus min-interval `* *` reject. | **Parity+.** Both validate at admission with 422. Forge's cron validator additionally rejects `* *` (every minute) which Pterodactyl allows; intentional rate-limit. |
| 11 | **Schedule task action validation** | `ScheduleTaskController` validates `action ∈ {power,command,backup}` at store time. | `store.go:1182` `isValidScheduleTaskAction` covers `power|backup|command`. `store_schedules.go:332` `PatchScheduleTask` enforces it, but `CreateScheduleTask:252` does NOT call it — only checks `action != ""`. | **Defect — MEDIUM.** Asymmetry allows storing arbitrary actions (e.g. `power; rm -rf`). Execution `schedule_runner.go:386-438` rejects at run time (`unsupported task action`), so not RCE, but persistence of invalid state is a logic bug and fools `ListDueSchedules` counts. See F-02. |
| 12 | **`only_when_online` gate** | `ProcessScheduleService.php:45-68` loads daemon state via `DaemonServerRepository->getDetails()`; skips if `offline|stopping`, treats `DaemonConnectionException` specially (quiet fail). | `store_schedules.go:489-502` `ListDueSchedules` filters SQL `ss.only_when_online=FALSE OR s.status='running'` (DB status, not live daemon). `schedule_runner.go:139-187` `tick` → `ListDueSchedules` uses same filter; no live daemon probe at claim time. `ClaimDueSchedule` path transactional. | **Weaker fidelity.** Forge uses DB `status` column (eventually consistent) rather than live daemon state. Under rapid power transitions a schedule may run while daemon reports offline or be skipped while starting. LOW-MEDIUM behavioral difference, not security bypass. |
| 13 | **Cron job shell execution confinement (RCE boundary)** | Pterodactyl Wings never executes control-plane shell for user-supplied cron; only `power/command/backup` tasks run in container via daemon. | `cronjob/service.go:117-145` `executeJob` branches: `TargetType=="server"` → `dispatchServerCommand:240` (node console channel), else `runShellCommand:212` (`sh -c`). `dispatchServerCommand` fails closed if `serverDispatcher==nil` (`249-251` never falls back to shell). Server-scoped routes in `handlers_user_console.go:314-514` force `TargetType="server"`, `TargetID=serverID`; admin routes `handlers_cronjob.go:59` are `requireRole("admin")` and may create `Type!="server"` shell jobs. `service_rce_regression_test.go:17-38` proves fail-closed. | **Stronger isolation than reference.** Correctly implements SEC-5.1 boundary. `runShellCommand` scrubs env to `PATH`+`HOME` and hard timeouts `defaultMaxCronTimeoutSeconds=3600`. Matches Wings isolation intent. |
| 14 | **Database identifier quoting / priv escalation** | `DatabaseManagementService.php:106-113` creates DB+user via `DatabaseRepository` using prepared identifiers; name must match `s{serverId}_` prefix (`86-88`). Panel user gets `u{serverId}_` prefix. | `dbprovisioner/service.go:521-534` `provisionMySQL`/`provisionPostgreSQL:550` quote identifiers via `quoteMySQLIdentifier:740` (backticks) / `quotePostgresIdentifier:737` (double quotes) and string literals via `quoteSQLString:743`. `ValidateDatabaseIdentifier` enforced in `validateTarget:326`. `DatabaseService` routes are `requireRole("admin")` only. Server DB provision does not expose direct SQL. | **Parity+.** Quoting discipline matches or exceeds reference. Engine allowlist (`postgresql/mysql/mariadb/redis/mongodb`) explicit. |
| 15 | **Env var scope resolution hierarchy** | Pterodactyl startup variables are per-server, flat. | `store_envvars.go:240-284` `ResolveEnvironmentVariables` walks `organization → project → environment → service` inheritance, later scopes override earlier. `normalizeScope` defaults to `environment`. `GetTeamMemberPermissions:418-468` merges stored JSON permissions with `DefaultGranularPermissions(role)` by OR-ing defaults when stored false. | **New.** No Pterodactyl equivalent; behavior is intentional but the OR-merge bug (see §3) can silently re-enable permissions a caller tried to revoke for `member` role. |
| 16 | **Tenancy: owner vs org-member access** | `Server.php` has no `org_id`; access is `owner_id==user.id OR subuser row`. No organization concept. | `store_users.go:365-396` `UserCanAccessServer` keeps Pterodactyl semantics (admin bypass → owner → subuser row → `HasPermission`). Org layer is parallel: `policies/server_policy.go:22-54` checks `PermServerView`/`PermServerSettings` then falls back to `checkOrgMembership:66` (`UserIsOrgMember` on `servers.org_id`) or `checkOrgAdmin:81` (`owner|admin` role). `store_tenancy.go:609-654` `UserCanAccessOrgResource` requires admin OR (member ∧ `ServerBelongsToOrg`). Tests `store_tenancy_test.go:324-447` prove cross-tenant rejection. | **Superset.** Forge preserves Pterodactyl server access while adding mandatory org membership for policy-gated reads/updates. Interaction is additive, not replacement — a user who is a subuser but not an org member can still READ via `HasPermission` path even if not org member (intentional for invite flows). Documented correctly. |
| 17 | **Server policy Delete always-denies** | Pterodactyl allows server delete via admin API only. | `policies/server_policy.go:48-49` `ActionDelete: return false` (even admin hits early `user.Role=="admin"` return true at `23`, so reachable only for non-admins). Non-admin delete is always denied regardless of `PermServerSettings` or org admin role. | **Intentional hard-deny.** Non-admin server delete is blocked; only `requireRole("admin")` routes can delete (e.g. `DELETE /users`, `DELETE /servers` not exposed to users). Matches Pterodactyl admin-only delete. |
| 18 | **SFTP authorization gate** | Pterodactyl SFTP daemon checks `file.sftp` permission. | `store_users.go:410-542` `AuthenticateSFTP:464` and `AuthorizeSFTPSession:536` both iterate `permissions` and allow only if `*` or `file.sftp` present; suspended/installing servers rejected. Parity. | **Parity.** |

---

## 3. Logic Defects & Findings

### F-01 — Subuser Permission Escalation via Unrestricted `UpsertServerSubuser` (HIGH)

**Location**
- `forge/api/internal/store/store_users.go:297-335` (`UpsertServerSubuser`), `544-561` (`normalizeSubuserPermissions`)
- `forge/api/internal/store/permissions.go:206-215` (`HasPermission` honors `*`)
- `forge/api/internal/http/handlers_servers.go:541-632` (subuser routes)

**Reference**
- `reference/.../Permission.php:120` ("They will never be able to ... assign permissions they do not have themselves.")
- `reference/.../SubuserController.php:154-168` (`getDefaultPermissions` + service-level subset check, always injects `websocket.connect`)

**Defect**
Forge's allowlist in `normalizeSubuserPermissions` builds `allowed = map[defaultSubuserPermissions()]` and then for each input `perm` keeps it if `allowed[perm] || perm=="*"`. No check that the *actor* possesses `perm`. Any caller who passes `requireServerPermission(PermUserCreate)` (or `PermUserUpdate` for PATCH) can grant an arbitrary allowlisted permission or `*` to any other user:

```go
// store_users.go:552-554  — only allowlist, no actor-subset check
if permission == "" || seen[permission] || (!allowed[permission] && permission != "*") {
    continue
}
```

`HasPermission` then treats `*` as universal grant, so a single `user.create` grant is sufficient to escalate to `database.view_password`, `schedule.delete`, `allocation.delete`, `settings.reinstall`, or full `*`.

Pterodactyl explicitly forbids this; Forge's `UpsertServerSubuser` comment trail does not mention the guard, and no code enforces it. The existing `CheckUserCanCreateSubuser` only enforces *count* limits, not permission subset.

A second facet: `getDefaultPermissions` in Pterodactyl silently drops unknown keys via `array_intersect`; Forge does same but *adds* `*` as a permitted token, which Pterodactyl never stores for subusers. This widens the escalation from "any permission" to "every permission at once."

**Impact**
Vertical privilege escalation within a server. A low-privileged collaborator (e.g. granted only `file.read` + `user.create` to invite teammates) can self-escalate or escalate a confederate to full control, read DB passwords (`database.view_password`), trigger reinstalls, or create cron jobs that dispatch commands to the node (`cron.create` — still server-scoped but node-touching). Confidentiality + integrity boundary between subusers is broken.

**Reproduction sketch**
1. `owner` creates `alice` with `permissions=["user.create","file.read"]`.
2. `alice` calls `POST /servers/:id/users` with `{"email":"bob@example.com","permissions":["*"]}` → succeeds (handler only checks `user.create`).
3. `bob` now has `*` and can `GET /servers/:id/startup` with secrets visible, `POST /servers/:id/schedules` etc.

**Recommendation**
- Enforce subset: at `UpsertServerSubuser` (or handler), load actor's effective permissions via `GetServerSubuser(ctx, serverID, actorID)` (or `['*']` for owner/admin) and reject any `req.Permissions` not in actor's set unless actor is owner/admin. Signature change to `UpsertServerSubuser(ctx, serverID string, req UpsertServerSubuserRequest, actorID *string, actorPerms []string)`.
- Remove `permission=="*"` from the subuser allow path, or gate it behind `actorIsOwnerOrAdmin`. At minimum, strip `*` from `defaultSubuserPermissions` allow branch and treat it as admin-only token.
- Add test: subuser with `user.create` cannot grant `database.view_password`.

**Severity:** HIGH — permission bypass / wrong scope.

---

### F-02 — Missing `isValidScheduleTaskAction` Check on `CreateScheduleTask` (MEDIUM)

**Location**
- `forge/api/internal/store/store_schedules.go:252-329` — `CreateScheduleTask` validates `action != ""` and `timeOffset>=0` but never calls `isValidScheduleTaskAction`
- `forge/api/internal/store/store.go:1182-1189` — `isValidScheduleTaskAction` exists (power/backup/command)
- `forge/api/internal/store/store_schedules.go:331-333` — `PatchScheduleTask` *does* validate: `if req.Action != nil && !isValid(...) { return "unsupported..." }`

**Reference**
- `reference/.../ScheduleController.php:40-47` — schedule tasks created via typed repository methods; Wings runner only handles known actions.

**Defect**
Create vs Patch asymmetry. An attacker with `schedule.update` (which gates `POST .../tasks` in `handlers_servers.go:1275`) can persist `action="powershell"` or `action="command; rm -rf /"` with arbitrary `payload`. The runner `schedule_runner.go:386-438` correctly rejects at execution (`default: return "unsupported task action"`), so no host RCE, but the DB now holds invalid rows that:
- break `ListDueSchedules` expectations (counts/tasks include un-runnable entries, wasting lease cycles),
- allow stored XSS-like payload reflection if any UI renders `action` without escaping,
- constitute a validation bypass that Pterodactyl does not have.

**Impact**
Logic defect / wrong validation scope. Low direct security impact (no execution), but violates fail-at-admission principle and pollutes schedule state. Combined with F-01 it amplifies an attacker's ability to plant confusing schedule tasks that later confuse operators reviewing audit trails.

**Recommendation**
Add at top of `CreateScheduleTask`:
```go
if !isValidScheduleTaskAction(req.Action) {
    return ScheduleTask{}, fmt.Errorf("unsupported task action: %s", strings.TrimSpace(req.Action))
}
```
Add unit test for symmetry with `Patch`.

**Severity:** MEDIUM — missing validation.

---

### F-03 — Mount Path Reserved Allowlist Too Narrow → Host Path Exposure (MEDIUM-HIGH)

**Location**
- `forge/api/internal/store/store_mounts_ext.go:323-339` `validateMountPath`
- `forge/api/internal/http/handlers_admin.go:1605-1635` `POST /mounts` (admin-only, `mounts.write`)

**Reference**
- Pterodactyl `Mount` docs: source must not be `/`, must be absolute, target must not be `/`; Forge adds explicit blocks for `/etc/forge`, `/var/lib/forge/volumes`, `/`, `/home/container`.

**Defect**
`validateMountPath` blocks only:
- `source ∈ {/etc/forge, /var/lib/forge/volumes}`
- `target ∈ {/, /home/container}`

Dangerous host paths remain allowed: `source=/etc`, `source=/etc/shadow` parent dir, `source=/var/run/docker.sock`, `source=/proc`, `source=/` (source-side `/` is *not* blocked — only target), `source=/root`. An admin who is compromised or a confused-deputy API key with `mounts.write` can create a mount exposing the host's credential material or Docker socket into every container whose egg/node eligibility matches. The double-join `ensureMountAvailableForServer` does not help if the attacker also attaches the egg and node (both admin ops).

Other edge cases correctly handled: `..` segments are blocked (`328-330`), `path.Clean != value` rejects `//` or trailing `/.`, and absolute-path requirement is enforced. So classic traversal is closed; the gap is the *semantic* allowlist, not the syntactic check.

**Impact**
If an admin credential is stolen, mount creation becomes a host-breakout primitive without needing shell access. Pterodactyl's Wings validates mounts at the daemon as well and documents that mounts should point to dedicated host directories; Forge's panel-side list is shorter than advised.

**Recommendation**
- Expand reserved list to deny-list sensitive prefixes: `/etc`, `/var/run`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, and any path under `/var/lib/forge` except an explicit `allowedMountBase` (e.g. `/srv/forge-mounts`). Alternatively switch to allowlist: require `source` to be under a configured `MOUNTS_ALLOWED_PREFIX` (default `/srv/forge-mounts/` or `/var/lib/forge/mounts/`).
- Apply same check to `UpdateMount` already (it does, via `validateMountPath`), and add daemon-side re-validation in Beacon's allowed-sources check (`AllowedMountSourcesForNode` is allowlist, but workload creation should reject if `source` not in that node's allowlist — already true via `Rejected` path, but panel should pre-reject).
- Add test covering `CreateMount Source=/etc` → error.

**Severity:** MEDIUM-HIGH — mount traversal / host exposure (requires admin scope, so not unauth privilege escalation, but blast-radius amplification).

---

### F-04 — Server-Scoped Cron Listing Filters Client-Side After Full Table Scan (INFO / LOW-MEDIUM)

**Location**
- `forge/api/internal/http/handlers_user_console.go:315-339` `GET /servers/:id/cron-jobs`
- `forge/api/internal/store/store_cron_jobs.go:95-118` `ListCronJobs` (no server filter)

**Defect**
Handler loads *all* cron jobs (`ListCronJobs`) then filters in Go:
```go
for _, j := range jobs {
    if j.TargetType != "server" || j.TargetID != serverID { continue }
}
```
This is not a data leak (non-matching jobs are not serialized), but it incurs O(N) work per request (global + server jobs) and, if the table grows large, creates a timing side-channel (response time proportional to total jobs, not just this server's). The admin `GET /cron-jobs` route (`handlers_cronjob.go:37`) is intentionally global, but the server-scoped route should push the filter into SQL (`WHERE target_type='server' AND target_id=$1`). Also missing `COUNT` pagination caps are inconsistent with `clampPageParams` elsewhere.

**Impact**
Performance / minor information disclosure via timing. Not an auth bypass.

**Recommendation**
Add `ListCronJobsByServer(ctx, serverID string)` with SQL filter; keep `ListCronJobs` for admin route only.

**Severity:** INFO — no bypass, but logic defect.

---

### F-05 — `GetTeamMemberPermissions` OR-Merge Silently Re-enables Explicitly Denied Permissions (LOW-MEDIUM)

**Location**
- `forge/api/internal/store/store_envvars.go:418-468` `GetTeamMemberPermissions`
```go
if !perms.CanCreateProjects && role != "" {
    perms.CanCreateProjects = defaultPerms.CanCreateProjects
}
...
if !perms.CanDeleteProjects {
    perms.CanDeleteProjects = defaultPerms.CanDeleteProjects
}
```

**Defect**
Stored `permissions` JSON is unmarshalled into `GranularPermissions` (zero-value false for missing keys). The merge then does `if !stored.X { stored.X = default }`. For `member`/`viewer` defaults where many fields are false, this is benign. For `member` where `CanCreateProjects=true` by default, an explicit stored `false` (operator intentionally revoked create) is indistinguishable from "field absent / zero value" and will be overwritten to `true`. The check cannot distinguish "admin set false" from "field was omitted in old JSON." This re-enables a permission the admin tried to revoke.

**Recommendation**
Store permissions as `*bool` or use `map[string]bool` presence check before defaulting. Or store full struct on write so missing fields are never ambiguous.

**Severity:** LOW-MEDIUM — policy validation error, not exploitable without an admin first setting a permission then trying to revoke it.

---

## 4. Detailed Observations

### Schedules / Cron — Injection & Validation

- **Cron expression injection:** Both `validateServerScheduleCron:3693` and `validateCronSchedule:15` use `robfig/cron` parser with `Minute|Hour|Dom|Month|Dow` fields, rejecting 6-field seconds or `* *` every-minute (`parts[0]=="*" && parts[1]=="*"`). Forge's `PATCH` path recomposes the 5-field expression from partial updates (`1207-1233`) and re-validates — correct. No shell injection via cron fields; expressions are never interpolated into shell commands, only parsed.

- **Task payload injection:** `schedule_runner.go:420-435` `command` tasks take `payload["command"]` as opaque string and pass it to `Daemon.SendCommand` (game server console), not host shell. `backup` and `power` tasks ignore payload except `signal`. No host command injection path.

- **Global cron shell injection:** `cronjob/service.go:212-238` `runShellCommand` runs `sh -c` *by design* for admin-authored jobs. Documented in comments `192-211` that sanitization is intentionally absent. Authorization is the boundary (admin-only `POST /cron-jobs` `handlers_cronjob.go:59` `requireRole("admin")`). Server cron path never reaches `runShellCommand` — verified by `service_rce_regression_test.go:17-38` fail-closed test. No finding.

### Databases — Privilege & Isolation

- **Database provisioning:** `dbprovisioner/service.go:471-483` and `594-623` `deprovisionRemote` correctly branches on `Engine ∈ {mysql,mariadb,postgresql,redis,mongodb}` and uses quoted identifiers. `handlers_database_services.go:44-113` all routes are `requireRole("admin")` + `requireAdminScope("databases.*")`, so no user can provision arbitrary DB engines. Server DB routes (`GET /servers/:id/database-services:380`) require `PermDatabaseRead` — correct scoping per server.

- **Database limits:** `DatabaseManagementService.php:80` checks `database_limit`; Forge `CreateServerRequest` now carries `DatabaseLimit` with validation in `store_servers.go:210` and handler `handlers_servers.go:817` `CheckUserCanCreateServer`. No bypass found.

### Mounts — Confinement

- Beacon's `AllowedMountSourcesForNode:367` is queried by the runtime adapter before container creation; only sources explicitly linked via `mount_node` are allowed. This is defense-in-depth over panel validation. The remaining gap is the admin allowlist breadth (F-03).

### API Keys / Scopes

- `store_apikeys.go:54` `AdminScopes` includes `mounts.read/write/delete`, `allocations.write`, etc. `auth.go:525-543` `requireAdminScope` correctly checks `HasAdminScope`. Server access via API key also checks `checkServerPermission` which enforces both OAuth/IP scope *and* subuser permission — dual gate. No finding.

### Tenancy

- `policies/server_policy.go:22` admin bypass is correct. The `store_users.go:365` owner shortcut is preserved; org check is additive, not replacing. This means a user who owns a server in org A but is not a member of org B cannot access B's servers even if they are a subuser there — but subuser path still works without org membership, which is intentional for cross-org collaboration invites before org join (documented in comment `store_tenancy.go:642-654`).

### Beacon Cron vs Forge Scheduler

- Beacon `cron.go:39-83` is a simple interval scheduler for cleanup/health; it does not execute game schedules. Game schedules run on the control plane (`schedule_runner.go:139-187` lease-based `ClaimDueSchedule` with `ScheduleLeaseDuration`), which prevents multi-replica double-execution via `pg_advisory_lock` + `SELECT FOR UPDATE SKIP LOCKED` style claim (implementation in `store_schedule_leases.go`). Wings daemon previously executed tasks directly; Forge moves execution to control plane + daemon RPC — architecturally sound, but note the `only_when_online` fidelity difference (§2.12).

---

## 5. Recommendations Summary

| Priority | Item | Location | Fix |
|----------|------|----------|-----|
| P0 | Enforce permission-subset on subuser upsert; remove `*` from subuser allowlist | `store_users.go:297`, `handlers_servers.go:541` | Load actor perms, reject grants outside actor set; gate `*` behind owner/admin |
| P1 | Add `isValidScheduleTaskAction` to `CreateScheduleTask` | `store_schedules.go:252` | One-line guard + test |
| P1 | Harden mount reserved list / add allowed-prefix | `store_mounts_ext.go:323` | Deny `/etc`, `/var/run`, `/proc`, `/sys`, `/dev`, `/`; or allowlist prefix |
| P2 | Push cron server filter into SQL | `handlers_user_console.go:322` | `WHERE target_type='server' AND target_id=$1` |
| P2 | Fix `GetTeamMemberPermissions` OR-merge to respect explicit false | `store_envvars.go:418` | Use pointer/presence map |
| P2 | Make mount delete count check race-free | `handlers_admin.go:1653` | `SELECT ... FOR UPDATE` or FK `RESTRICT` |
| P3 | Add `PermServerView`/`PermServerSettings` to `AllPermissions` or remove defs | `permissions.go:84-106` | Consistency |

---

## 6. Conclusion

Forge's tenancy refactor is a strict superset of Pterodactyl's subuser model with meaningful hardening: scrubbed cron shell env, fail-closed server dispatcher, `FOR UPDATE` on allocations, explicit `server:read-env` masking, and org-scoped policy layer with proven cross-tenant tests (`store_tenancy_test.go:324-447`). The counted 18 comparisons show no missing Pterodactyl permission and stronger isolation on allocations and RCE boundaries.

Three logic defects remain load-bearing:

- **F-01 (HIGH)** breaks the subuser trust boundary — any `user.create` holder can become `*`.
- **F-02 (MEDIUM)** allows invalid schedule task actions to persist.
- **F-03 (MEDIUM-HIGH)** leaves sensitive host paths mountable by a compromised admin.

F-04/F-05 are lower severity but indicate incomplete validation / policy merge logic. All are fixable with localized store/handler patches and covered by existing test harnesses (`service_rce_regression_test.go`, `store_tenancy_test.go`, `store_mounts_delete_integration_test.go`).

*Audit completed without executing live provisioning; findings are code-path verified against the working tree at `forge/api` and `beacon/` as of 2026-08-23.*
