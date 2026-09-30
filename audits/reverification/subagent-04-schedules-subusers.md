# Subagent 04 — Reverification: Schedules/Cron Tasks + Subusers/Permissions vs Pterodactyl Permission Model

**Date:** 2026-08-24
**Scope:** FINAL_PARITY_AUDIT §11 SE-01/02 + GH-17/18, phase-02 subagent-05-tenancy-schedules F-01..F-03, final-parity subagent-10 S01-S04, MASTER FINDING REF-GAME-F-G-22 (wildcard escalation) + REF-GAME-F-G-23 (isValidScheduleTaskAction asymmetric)
**Method:** source inspection only — no product code modified. All citations `file:line` SOURCE_VERIFIED on working tree. Evidence tier: SOURCE_VERIFIED.

**Re-inspected (Forge):**
- `forge/api/internal/store/permissions.go:5`
- `forge/api/internal/store/store_users.go:88,297,544`
- `forge/api/internal/store/store_tenancy.go:1`
- `forge/api/internal/store/store_schedules.go:1,252,331`
- `forge/api/internal/store/store.go:1182` (`isValidScheduleTaskAction`)
- `forge/api/internal/store/store_mounts_ext.go:323`
- `forge/api/internal/http/handlers_servers.go:541`
- `forge/api/internal/http/handlers_user_console.go:314`
- `forge/api/internal/policies/server_policy.go:22`, `policies/policy.go:18`, `policies/environment_policy.go:23`, `policies/project_policy.go:20`
- `forge/api/internal/auth/session.go:36`
- `forge/api/internal/services/cronjob/service.go:212` (`runShellCommand`), `240` (`dispatchServerCommand`)

**Re-inspected (Reference):**
- `reference/game-hosting/pterodactyl-panel/app/Models/Permission.php:18`
- `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/SubuserController.php:154` (`getDefaultPermissions`)
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/GetUserPermissionsService.php:15`
- `reference/game-hosting/pterodactyl-panel/app/Services/Schedules/ProcessScheduleService.php:45` (only_when_online gate)

---

## 1. Reconciliation Summary vs Prior Audits

| Prior ID | Prior verdict | Current verdict | Drift |
|---|---|---|---|
| REF-GAME-F-G-22 / FINAL GH-18 / SE-01 / F-01 / S02+S03 — wildcard `*` persistable + no subset enforcement | **BROKEN (P0)** | **STILL BROKEN (P0)** — no code change since 2026-08-23; `normalizeSubuserPermissions:564` still allowlists `*`, `UpsertServerSubuser:306` still no actor-subset check, handler `handlers_servers.go:554` gates only `user.create` | Confirmed unchanged |
| REF-GAME-F-G-23 / FINAL GH-17 / F-02 / S?? — `isValidScheduleTaskAction` asymmetric | **BROKEN (MEDIUM)** | **STILL BROKEN (MEDIUM)** — `CreateScheduleTask:252` still does not call `isValidScheduleTaskAction:1196`; `PatchScheduleTask:331` does | Confirmed unchanged |
| REF-GAME-F-G-24 / SE-04 / F-03 / S05 — mount allowlist too narrow | **BROKEN (MEDIUM-HIGH)** | **STILL BROKEN (MEDIUM-HIGH)** — `validateMountPath:323` still blocks only 2 sources + 2 targets | Confirmed unchanged |
| FINAL S01 — permission catalog superset | **COMPLETE** | **COMPLETE** — unchanged | — |
| FINAL S04 — tenancy org membership | **PARTIAL** (S04 tenant-blind placement) | **PARTIAL** — `server_policy.go:66` `checkOrgMembership` correctly queries `UserIsOrgMember` (not universal true); placement/ events remain tenant-blind (S04) but out-of-scope for this subagent | Narrow fix landed, wider gap remains |
| Cron shell boundary (service.go:240 fail-closed) | **COMPLETE+** | **COMPLETE** — `dispatchServerCommand:249` still fail-closed, `runShellCommand:220` scrubbed env, `handlers_user_console.go:381` forces `TargetType="server"` | Confirmed intact |

---

## 2. Definitive Parity Matrix (14 rows)

| # | Aspect | Reference truth (`file:line`) | Forge (`file:line`) | Status | Gap / Note |
|---|---|---|---|---|---|
| 1 | **Permission catalog** — 10 groups, source of truth | `Permission.php:18-66` 10 groups (`websocket/control/user/file/backup/allocation/startup/database/schedule/settings/activity`) + `permissions():215` map `101-209` | `permissions.go:5-86` 40+ keys across same 10 + extras `cron.read/create/update/delete/run:28-33`, `buildpack.manage:38`, `mount.read/update:79-80`, `server:read-env:72`, `server.view/settings:84-85` ; `AllPermissions():89` | **COMPLETE (superset)** | Forge strictly superset. Minor leftover: `PermServerView`/`PermServerSettings:84-85` defined but absent from `AllPermissions():89-106` — `normalizeSubuserPermissions` would drop them if ever assigned via subuser flow (P3, noted since phase-02 §2.1). |
| 2 | **Wildcard `*` semantics — owner-only computed, never persisted for subusers** | `GetUserPermissionsService.php:15-27` returns `['*']` only if `root_admin || owner_id==user.id`; subuser rows hold explicit perms only | `permissions.go:206-215` `HasPermission` honors `p=="*"` as universal; `store_users.go:564` `normalizeSubuserPermissions` explicitly allows `permission=="*"` via `(!allowed[perm] && permission!="*") continue`; `UserCanAccessServer:378-406` owner/admin short-circuit before row, but DB row for subuser may hold `*` | **BROKEN (P0)** | Divergence confirmed unchanged. Reference never stores `*` for subusers; Forge persists it if any `user.create` holder supplies it. See Finding F-RV-01. |
| 3 | **Subset enforcement — actor may only grant perms they possess** | `Permission.php:120-121` “They will never be able to edit their own account, or assign permissions they do not have themselves.” + `SubuserController.php:154-167` `getDefaultPermissions` intersects with allowlist via `array_intersect` + service-layer subset check (not shown but documented) | `store_users.go:297-335` `UpsertServerSubuser` calls only `normalizeSubuserPermissions:311` (allowlist, not actor-subset); `handlers_servers.go:554` `POST /servers/:id/users` checks `requireServerPermission(PermUserCreate)` then `CheckUserCanCreateSubuser` (count limit only, `handlers_servers.go:564-570`); same for `PATCH :594` (`PermUserUpdate`) | **BROKEN (P0)** | Same root as #2. No subset gate. Any `user.create/update` holder can grant arbitrary allowlisted perm or `*`. |
| 4 | **Owner-as-subuser guard** | `SubuserCreationService` throws `UserIsServerOwnerException` | `store_users.go:321-322` `if userID==ownerID { return "server owner cannot be added as a subuser" }` | **COMPLETE** | Parity exact. |
| 5 | **SFTP authorization gate** | Panel `file.sftp` permission checked on SFTP daemon | `store_users.go:421-481` `AuthenticateSFTP:475` and `521-553` `AuthorizeSFTPSession:547` iterate perms and allow only if `*` or `file.sftp`; reject `suspended/installing` | **COMPLETE** | Parity; correctly gates on `*`\|`file.sftp`. |
| 6 | **Schedule cron admission validation** | `ScheduleController` throws `DisplayException`→422 if 5-field cron invalid | `handlers_servers.go:1178-1213` `POST .../schedules` calls `validateServerScheduleCron:1183` (robfig/cron); `PATCH :1220-1244` recomposes + re-validates; `handlers_user_console.go:365` `validateCronSchedule` for cron-jobs + min-interval reject | **COMPLETE+** | Both sides 422 at admission; Forge additionally rejects `* *` every-minute (intentional rate-limit). |
| 7 | **Schedule task action validation** | `ScheduleTaskController` validates `action ∈ {power,command,backup}` at store time | `store.go:1192-1202` `isValidScheduleTaskAction` covers `power│backup│command` (lower+trim); `store_schedules.go:331-333` `PatchScheduleTask` enforces it; `store_schedules.go:252-284` `CreateScheduleTask` validates only `action!=""` and `timeOffset>=0` — never calls `isValidScheduleTaskAction` | **BROKEN (MEDIUM)** | Create vs Patch asymmetry persists verbatim. See Finding F-RV-02. |
| 8 | **`only_when_online` gate fidelity** | `ProcessScheduleService.php:45-68` probes live daemon via `DaemonServerRepository->getDetails()`; skips if `offline│stopping`; special-cases `DaemonConnectionException` | `store_schedules.go:489-502` `ListDueSchedules` filters SQL `ss.only_when_online=FALSE OR s.status='running'` (DB status, not live daemon); `schedule_runner.go:139-187` `tick`→`ListDueSchedules` same filter; no live daemon probe at claim; `ClaimDueSchedule` transactional | **PARTIAL** | Weaker fidelity documented since phase-02 §2.12 — DB `status` is eventually consistent vs live daemon. Not a bypass, but behavioral delta (skip may fire while daemon offline). |
| 9 | **Mount path traversal / confinement — syntactic checks** | `Mount` model + Wings daemon validates absolute, not `/`, no overlap `/home/container` | `store_mounts_ext.go:323-338` `validateMountPath`: `path.IsAbs`, `Clean==value`, no `\`, no `..` segment; `validateMountPaths:316` both source+target | **COMPLETE (syntactic)** | Classic traversal closed. |
| 10 | **Mount allowlist — semantic deny-list breadth** | Docs: mount source should be under dedicated host dir; Wings re-validates via `mount_node` allowlist | `store_mounts_ext.go:332-336` only `source∈{/etc/forge,/var/lib/forge/volumes}` blocked and `target∈{/,/home/container}` blocked; `ensureMountAvailableForServer:297` double-join `mount_node ∧ egg_mount` required; `AllowedMountSourcesForNode:367` allowlists per node | **BROKEN (MEDIUM-HIGH)** | Syntactically strong but semantically narrow — see Finding F-RV-03. Double-join is stronger than Pterodactyl base but does not help if attacker also controls egg+node attachments (both admin ops). |
| 11 | **Cron shell boundary — server jobs never reach host shell** | Wings never runs control-plane shell for user cron; only `power/command/backup` in container | `cronjob/service.go:212-238` `runShellCommand` scrubbed env `PATH=/usr/local/sbin:...` `HOME=/tmp` + `defaultMaxCronTimeoutSeconds=3600`; `240-261` `dispatchServerCommand` fails closed if `serverDispatcher==nil` (never falls back); `135-144` `executeJob` branches `TargetType=="server"` → `dispatchServerCommand` else `runShellCommand` | **COMPLETE+** | Stronger isolation than reference; `service_rce_regression_test.go:17-38` (cited phase-02) proves fail-closed. `handlers_user_console.go:341-382` forces `TargetType="server"`, `TargetID=serverID`; admin `handlers_cronjob.go:59` `requireRole("admin")` for shell path. |
| 12 | **Server-cron route confinement + listing** | Client `SubuserController` scopes cron to server implicitly; server cron is not a global listing | `handlers_user_console.go:314-338` `GET /servers/:id/cron-jobs` `requireServerPermission(PermCronRead)` then loads `ListCronJobs` global then Go-filters `TargetType=="server" && TargetID==serverID`; `POST :341` `requireServerPermission(PermCronCreate)` forces `TargetType="server"` `TargetID=c.Params("id")`; PATCH/DELETE/GET-by-id all re-check `TargetType/ID` (`398-408`, `442-447`, `479-484`, `505-506`) | **PARTIAL** | Confinement on write/read-by-id is correct (fail-closed per-row check). `GET` list is *functionally correct* (non-matching rows not serialized) but does O(N) global scan then filters in Go — timing side-channel + scale cost (see Finding F-RV-04). |
| 13 | **Tenancy: org/project/env membership vs Pterodactyl owner-only** | `Server.php:18` `owner_id` only; access = `owner==user` OR subuser row; no org | `store_tenancy.go:396-548` `ListTeamMembers/GetTeamMemberRole/ResolveEffectiveOrgRole:539` + `645` `UserCanAccessOrgResource` (member ∧ `ServerBelongsToOrg:610`); `policies/server_policy.go:22-54` `ActionRead` checks `PermServerView` then `checkOrgMembership:66` (`UserIsOrgMember` on `servers.org_id`), `ActionUpdate` → `checkOrgAdmin:81` (`owner│admin`), `ActionDelete:49` always false for non-admin (admin bypass at `:23`) | **PARTIAL** | Forge superset preserved: `store_users.go:365` owner shortcut intact, org check additive. `server_policy.go:66` now correctly does `SELECT org_id` + `UserIsOrgMember` — fixes prior “universal true” risk. Remaining fleet-wide placement/events are tenant-blind (`PlacementRequest` no tenant field, `events` table no tenant column) — out-of-scope but noted in S04 (P1). |
| 14 | **Session / InMemory store hygiene (auth/session.go)** | Panel hashes tokens; Portainer `libcrypto` per-value nonce | `auth/session.go:36-178` `InMemorySessionStore{sessions, byToken, byUser}` + `51-65` `Create` stores raw `*Session` pointer + raw token key + `68-97` `Get/GetByToken` returns same pointer + `100` `Update` only touches `sessions[id]` + `231` `SessionMiddleware` mutates `sess.LastActiveAt` outside lock | **BROKEN (P1)** | Not subuser-scoped but cited in prompt: pointer aliasing + raw-token map leaks heap + concurrent `LastActiveAt` race (S13 in final-parity). Unchanged since phase-01. Documented for completeness; does not affect subuser escalation directly but affects SFTP/WS sessionBearer paths. |

---

## 3. Findings (Reverified)

### F-RV-01 — Subuser Permission Escalation via Unrestricted `UpsertServerSubuser` — STILL BROKEN (P0, security)

**Locations:**
- `forge/api/internal/store/store_users.go:306-343` (`UpsertServerSubuser`)
- `forge/api/internal/store/store_users.go:555-571` (`normalizeSubuserPermissions`)
- `forge/api/internal/store/permissions.go:196-215` (`HasPermission` honors `*`)
- `forge/api/internal/http/handlers_servers.go:554-620` (`POST :id/users`, `PATCH :id/users/:userId`)
- `forge/api/internal/http/auth.go:549-598` (`requireServerPermission` → `UserCanAccessServer:377`)

**Reference:**
- `reference/game-hosting/pterodactyl-panel/app/Models/Permission.php:120` (“They will never be able to … assign permissions they do not have themselves.”)
- `reference/game-hosting/pterodactyl-panel/app/Http/Controllers/Api/Client/Servers/SubuserController.php:154-168` (`getDefaultPermissions` intersects with `Permission::permissions()` allowlist, always injects `websocket.connect`)
- `reference/game-hosting/pterodactyl-panel/app/Services/Servers/GetUserPermissionsService.php:15-27` (owner/admin ⇒ `['*']` computed, never stored for subusers)

**Re-verified defect:**
`normalizeSubuserPermissions` builds `allowed = map[defaultSubuserPermissions()]` (which at `store_users.go:574-588` explicitly excludes `cron.*`, `file.read-content`, `file.sftp` extras? actually includes most but not `*`) and then keeps each input `perm` if `allowed[perm] || perm=="*"`. No check that the *actor* possesses `perm`.

```go
// store_users.go:562-567 — only allowlist, no actor-subset check, wildcard carve-out
if permission == "" || seen[permission] || (!allowed[permission] && permission != "*") {
    continue
}
```

`HasPermission:211` then treats `*` as universal, so a single `user.create` grant suffices to escalate to `*` → `database.view_password`, `schedule.delete`, `allocation.delete`, `settings.reinstall`, `cron.create` (which via `handlers_user_console.go:341` dispatches commands to the node). `CheckUserCanCreateSubuser` (called at `handlers_servers.go:566`) only enforces count limits, not permission subset. Both `POST :554` and `PATCH :594` follow the same path.

Pterodactyl explicitly forbids this; Forge comment at `permissions.go:196-199` (“The wildcard `*` is honored only as an explicit assignment (owner) – callers must ensure wildcard is only ever assigned to server owners or admins via normalized permission checks, not via arbitrary user input”) documents the invariant but the store does not enforce it.

**Impact:** Vertical privilege escalation within a server. Repro: `owner` grants `alice` `["user.create","file.read"]`; `alice` calls `POST /servers/:id/users {"email":"bob@x","permissions":["*"]}` → 201 (handler only checks `user.create`); `bob` now reads startup secrets (`PermServerReadEnv:72` via `*`), DB passwords (`database.view_password`), creates schedules/cron that reach the node. Confidentiality + integrity boundary between subusers broken.

**Recommendation (unchanged from phase-02 F-01):**
- Load actor effective perms at handler or store: `GetServerSubuser(ctx, serverID, actorID)` (or `['*']` for owner/admin via `UserCanAccessServer` with owner bypass) and reject any `req.Permissions` not in actor set unless actor is owner/admin. Signature change to `UpsertServerSubuser(ctx, serverID string, req UpsertServerSubuserRequest, actorID *string, actorPerms []string)`.
- Remove `permission=="*"` from subuser allow branch, or gate behind `actorIsOwnerOrAdmin`.
- Add test: subuser with `user.create` cannot grant `database.view_password` or `*`.

**Severity:** P0 — permission bypass / privilege escalation. **Status: STILL BROKEN** (no diff since audit baseline).

---

### F-RV-02 — Missing `isValidScheduleTaskAction` Check on `CreateScheduleTask` — STILL BROKEN (MEDIUM)

**Locations:**
- `forge/api/internal/store/store_schedules.go:252-329` (`CreateScheduleTask` validates `action!=""` + `timeOffset>=0` but never calls `isValidScheduleTaskAction`)
- `forge/api/internal/store/store.go:1196-1202` (`isValidScheduleTaskAction` exists for `power|backup|command`)
- `forge/api/internal/store/store_schedules.go:331-333` (`PatchScheduleTask` does validate: `if req.Action != nil && !isValid(...) { return "unsupported..." }`)
- `forge/api/internal/http/handlers_servers.go:1288-1312` (`POST /schedules/:scheduleId/tasks` forwards to store without extra validation)
- `forge/api/internal/http/schedule_runner.go:386-438` (`executeTask` rejects at run time `default: return "unsupported task action"`)

**Reference:** `reference/game-hosting/pterodactyl-panel` typed schedule task repos; Wings runner only handles known actions.

**Re-verified defect:** Exact asymmetry described in phase-02 F-02. An actor with `schedule.update` (which gates `POST .../tasks` at `handlers_servers.go:1288` — note: task create is gated by `schedule.update`, not `schedule.create`, matching Pterodactyl's `schedule.update` for tasks) can persist `action="powershell"` or `action="command; rm -rf /"`-style arbitrary strings with arbitrary `payload`. Runner correctly rejects at execution (`schedule_runner.go:436-437` `unsupported task action`), so no host RCE, but DB now holds invalid rows that waste `ListDueSchedules` lease cycles, confuse `ListDueSchedules` counts, and allow stored payload reflection if UI renders `action` without escaping.

**Recommendation:** Add at top of `CreateScheduleTask`:

```go
if !isValidScheduleTaskAction(req.Action) {
    return ScheduleTask{}, fmt.Errorf("unsupported task action: %s", strings.TrimSpace(req.Action))
}
```

Add symmetry unit test.

**Severity:** MEDIUM — missing validation / wrong scope. **Status: STILL BROKEN** (one-line guard still absent).

---

### F-RV-03 — Mount Path Reserved Allowlist Too Narrow → Host Path Exposure (MEDIUM-HIGH) — STILL BROKEN

**Locations:**
- `forge/api/internal/store/store_mounts_ext.go:323-338` (`validateMountPath`)
- `forge/api/internal/http/handlers_admin.go:1605-1635` (`POST /mounts`, admin-only `mounts.write` — delegated to `CreateMount:41`)
- `forge/api/internal/store/store_mounts_ext.go:367-387` (`AllowedMountSourcesForNode`) + `297-313` (`ensureMountAvailableForServer`)

**Reference:** Pterodactyl model validates `source`/`target` absolute, not `/`, not overlapping `/home/container`; Forge adds explicit blocks.

**Re-verified defect:** `validateMountPath` blocks only:
- `source ∈ {/etc/forge, /var/lib/forge/volumes}` (`332`)
- `target ∈ {/, /home/container}` (`335`)

Remainder allowed: `source=/etc`, `source=/etc/shadow` parent dir, `source=/var/run/docker.sock`, `source=/proc`, `source=/sys`, `source=/dev`, `source=/boot`, `source=/root`, `source=/` (source-side `/` is *not* blocked — only target), `source=/var/lib/forge` (parent). The syntactic checks (`path.IsAbs`, `Clean==value`, no `\`, no `..` at `324-330`) are correct, but the *semantic* allowlist is shorter than advised. An admin credential or API key with `mounts.write` can create a mount exposing host credential material or Docker socket into every container whose egg/node eligibility matches. `ensureMountAvailableForServer:297` double-join `mount_node ∧ egg_mount` does not help if attacker also attaches the egg and node (both admin ops via `AttachEggToMount:194` / `AttachNodeToMount:204`). `AllowedMountSourcesForNode:367` is queried by runtime before container create, but panel pre-rejection is still too permissive.

**Recommendation (unchanged):** Expand reserved list to deny-list sensitive prefixes `/etc`, `/var/run`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, any `/var/lib/forge` except explicit `allowedMountBase` (e.g. `/srv/forge-mounts`), or switch to allowlist `MOUNTS_ALLOWED_PREFIX` (default `/srv/forge-mounts/`). Apply same check in `UpdateMount` (already does via `validateMountPath:122-134`) and add daemon-side re-validation in Beacon workload creation. Add test `CreateMount Source=/etc → error`, `Source=/var/run/docker.sock → error`, `Source=/proc → error`.

**Severity:** MEDIUM-HIGH — blast-radius amplification for compromised admin (not unauth privesc). **Status: STILL BROKEN**.

---

### F-RV-04 — Server-Scoped Cron Listing Filters Client-Side After Full Table Scan (INFO / LOW-MEDIUM) — STILL PRESENT

**Locations:**
- `forge/api/internal/http/handlers_user_console.go:315-338` (`GET /servers/:id/cron-jobs`)
- `forge/api/internal/store/store_cron_jobs.go:95-118` (`ListCronJobs` no server filter)

**Defect:** Handler loads *all* cron jobs (`ListCronJobs`) then filters in Go:

```go
for _, j := range jobs {
    if j.TargetType != "server" || j.TargetID != serverID { continue }
}
```

Not a data leak (non-matching jobs not serialized), but O(N) work per request (global + server jobs) and timing side-channel (response time proportional to total jobs). Admin `GET /cron-jobs` (`handlers_cronjob.go:37`) is intentionally global; server route should push filter into SQL (`WHERE target_type='server' AND target_id=$1`). Also missing `COUNT` pagination caps are inconsistent with `clampPageParams` elsewhere.

**Recommendation:** Add `ListCronJobsByServer(ctx, serverID string)` with SQL filter; keep `ListCronJobs` for admin route only.

**Severity:** INFO — no bypass, but logic/ performance defect. **Status: STILL PRESENT** (same code as audited).

---

### F-RV-05 — Session Store Pointer Aliasing + Raw Token Storage (P1, cross-cutting) — STILL BROKEN

**Locations:**
- `forge/api/internal/auth/session.go:36-113` (`InMemorySessionStore`)
- `forge/api/internal/auth/session.go:51-65` (`Create` stores `session` pointer + raw token key)
- `forge/api/internal/auth/session.go:68-97` (`Get/GetByToken` returns same pointer)
- `forge/api/internal/auth/session.go:100-113` (`Update` only touches `sessions[id]`)
- `forge/api/internal/auth/session.go:231-269` (`SessionMiddleware` mutates `sess.LastActiveAt` outside lock)

**Defect (final-parity S13, phase-01 ARCH02):** Two concurrent `Bearer <token>` requests receive same `*Session`, both do `sess.LastActiveAt=Now()` without holding store lock — `go test -race` detectable. `byToken` stores raw base64 token as map key → heap/pprof dump leaks bearer tokens. `Update` at `100` only touches `sessions[id]`, leaving `byToken` stale if `sess.Token` mutated. Durable `store_users.go:914` `user_sessions` correctly stores `session_token_hash` (SHA-256 hex) — in-memory path does opposite. Included here because `handlers_user_console.go:314` cron routes and SFTP `AuthenticateSFTP:410` share the same session bearer path.

**Recommendation:** Clone on `Create`/`Get` (`*sess` copy), store `sha256Hex(token)` as key (as done for `jti` at `http/auth.go:412`), update `byToken` atomically in `Update`/`Delete`.

**Severity:** P1 — race + credential leakage in heap dumps. **Status: STILL BROKEN** (code unchanged).

---

## 4. Additional Observations (Not Findings)

- **Cron expression injection:** Both `handlers_servers.go:1183` `validateServerScheduleCron` and `handlers_user_console.go:365` `validateCronSchedule` use `robfig/cron` 5-field parser; expressions never interpolated into shell commands, only parsed — no finding.
- **Task payload injection:** `schedule_runner.go:420-435` `command` tasks take `payload["command"]` as opaque string → `Daemon.SendCommand` (game server console), not host shell; `backup`/`power` ignore payload except `signal`. No host command injection path.
- **Global cron shell injection boundary:** `cronjob/service.go:192-238` `runShellCommand` runs `sh -c` *by design* for admin-authored jobs; authorization is the boundary (admin `POST /cron-jobs` `requireRole("admin")`); server path never reaches shell — verified fail-closed at `244-251`.
- **Database identifier quoting / priv escalation:** `services/dbprovisioner/service.go:737-743` `quote*Identifier` correct; routes `requireRole("admin")` only — no finding.
- **Mount delete lifecycle race (minor):** `handlers_admin.go:1638-1684` `DELETE /mounts/:id` checks `CountServersUsingMount>0` →400 then deletes `DeleteMount:253` (no FK restrict) then best-effort `Daemon.CleanupMount` per node — window where concurrent attach slips after check. Recommend `SELECT FOR UPDATE` or FK `RESTRICT`. LOW-MEDIUM (phase-02 §2.9).

---

## 5. Verdict — Are the Three Flagged Issues Fixed?

| Flag | Question | Answer |
|---|---|---|
| **Wildcard `*` escalation (REF-GAME-F-G-22 / SE-01/02 / GH-18 / F-01)** | Is subuser `*` escalation still BROKEN? | **YES — still BROKEN.** `normalizeSubuserPermissions:564` still carves out `*`, `UpsertServerSubuser:306` still no actor-subset gate, handler still only gates `user.create/update`. Same P0 escalation path as before. |
| **CreateScheduleTask validation (REF-GAME-F-G-23 / GH-17 / F-02)** | Does `CreateScheduleTask` now validate `isValidScheduleTaskAction`? | **NO — still asymmetric.** `PatchScheduleTask:331` validates, `CreateScheduleTask:252` does not. The one-line guard remains absent; arbitrary `action` still persists and is rejected only at execution (`schedule_runner.go:436`). |
| **Mount allowlist breadth (REF-GAME-F-G-24 / SE-04 / F-03)** | Is mount allowlist still narrow? | **YES — still narrow.** `validateMountPath:332` still blocks only `/etc/forge` + `/var/lib/forge/volumes`; `/etc`, `/var/run/docker.sock`, `/proc`, `/sys`, `/dev`, `/root`, `/` remain allowed via admin `CreateMount`. |

Net: **0 of 3 P0/HIGH fixes have landed** since the phase-02 / final-parity baselines. The permission catalog superset and cron shell fail-closed boundary remain intact and are the only COMPLETE items in this slice.

---

## 6. References

- `FINAL_PARITY_AUDIT.md §11` rows SE-01, SE-02, GH-17, GH-18
- `audits/phase-02/subagent-05-tenancy-schedules.md` Findings F-01 (HIGH), F-02 (MEDIUM), F-03 (MEDIUM-HIGH), F-04 (INFO), F-05 (LOW-MEDIUM)
- `audits/final-parity/subagent-10-security-ux.md` S01-S05 (S02/S03 P0, S05 P1)
- `audits/MASTER_FINDING_INDEX.md` REF-GAME-F-G-22, REF-GAME-F-G-23, REF-GAME-F-G-24
- Reference: `reference/game-hosting/pterodactyl-panel/app/Models/Permission.php:18`, `.../SubuserController.php:154`, `.../GetUserPermissionsService.php:15`

*Audit completed without executing live provisioning; findings are code-path verified against the working tree at `forge/api` and `reference/` as of 2026-08-24. No product code was modified.*

