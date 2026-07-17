# Competitor Behavior Map

**Phase 1 — Read competitors, document behavior, no code.**
**Date: 2026-06-17**

This document captures *what* competitors do, not *how*. Convert behavior into our stack
later. Do not copy code. Do not invent features until core panel works.

---

## AUTH

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **Login** | Email + password form. Throttled: 3 fails = 2-min lockout. Optional recaptcha. React SPA handles redirect. | Email/username + password. Rate-limited at 3 attempts/2min. Can entirely disable password login when OAuth is enabled. | Email + password. Creates UUID session token (SHA256-hashed in DB). Cookie: `puffer_auth`, 1hr expiry. | N/A (daemon only) | Email + password login. Rate-limited per IP + email via Redis (fails open without Redis). JWT token returned. |
| **2FA** | TOTP via `pragmarx/google2fa`. Login returns `complete: false` + `confirmationToken`. User posts checkpoint with TOTP code. Recovery tokens stored in DB. | App TOTP (encrypted secret in `users.mfa_app_secret`) + Email codes (`users.mfa_email_enabled`). Three enforcement levels: off / admins only / all users. Middleware redirects to profile if 2FA required but not set. | TOTP via `pquerna/otp`. Recovery codes hashed with blake2b. `UseTOTP` boolean on user. Separate `LoginPost` → `OtpPost` flow. | N/A | TOTP via standard library. Login checkpoint flow: return `confirmationToken`, user posts TOTP code. Recovery tokens. Enforcement: off / admin / all. |
| **API Keys** | Two types: Account (`ptlc_`) for client API, Application (`ptla_`) for admin API. 16-char prefix + 32-char token (encrypted in DB, plaintext shown once). Per-resource bitwise permissions (NONE=0, READ=1, WRITE=2). IP whitelist via CIDR. Expiry support. `last_used_at` tracked. | Same two-type model. Prefixes: `pacc_` (account), `papp_` (application). Permissions stored as JSON array of `{resource, level}`. `allowed_ips`, `expires_at`, `last_used_at`, `memo`. | Session-based (no API keys in same sense). OAuth2 server for daemon-to-panel auth. `Client` model with `client_id` + `hashed_client_secret` + scopes. | Bearer token: `{daemon_token_id}.{daemon_token}`. Token stored encrypted in panel DB. Wings sends it as `Authorization: Bearer <token>`. | Two types: admin (full access) and user (scoped). Scopes as string array. IP whitelist (`allowed_ips`). Expiry. `last_used_at`. SHA256-hashed in DB (prefix lookup). Token shown once on creation. |
| **OAuth/SSO** | None. | 13 providers: Discord, Steam, Authentik, GitHub, GitLab, Google, Facebook, Bitbucket, LinkedIn, Slack, X, + CommonSchema. Each has icon, settings form, enable/disable. Auto-create or link users. OAuth user IDs stored in `users.oauth` JSON. | None (has own OAuth2 server for daemon nodes, not user login). | N/A | Not in Phase 1. Essential for later (Discord, Steam minimum). |
| **WebAuthn** | None. | None. | Full passkey support via `go-webauthn`. `CredentialAssertion` challenge flow. Credentials serialized as JSON in `webauthn_credentials` table. | N/A | Not in Phase 1. |
| **Password reset** | Email reset link. `ForgotPasswordController`. Password reset tokens expire in 60 minutes. | Laravel built-in password reset. | Not visible in auth flow (admin-managed). | N/A | Password reset via email token. |
| **Session management** | Laravel session (cookie-based). Server-side state. | Laravel session (cookie-based). | UUID session token hashed with SHA256, stored in DB with 1hr expiration. `puffer_auth` cookie (secure, httpOnly). Logout deletes session row. | N/A | JWT with 24hr TTL. Stateless. Logout handled client-side (remove token). |

---

## USERS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Fields** | id, external_id, uuid(36), username, email, name_first, name_last, password(bcrypt), language(char5), root_admin(bool), use_totp, totp_secret, totp_authenticated_at, gravatar | Same + oauth(JSON), customization(JSON), mfa_app_secret(encrypted), mfa_app_recovery_codes, mfa_email_enabled, is_managed_externally | Username, Email, HashedPassword(bcrypt), OtpSecret, OtpActive, AllowPasswordlessLogin | id, uuid, email, username, password(bcrypt), role(string), use_totp, totp_secret, language, timezone |
| **Admin CRUD** | Full CRUD via Application API. List with pagination. Create with validation. Update fields. Delete. | Full CRUD via Filament admin panel. Spatie roles/permissions. | Full CRUD. Search endpoint. Permissions as scope strings. | Full CRUD. Role-based (admin/user). |
| **Ownership** | User owns servers via `owner_id`. Can access servers they own OR are subuser of (via `accessibleServers` scope). | Same ownership model. RBAC via Spatie roles. | User owns servers. No subuser concept — single-owner model. | User owns servers. Subuser model adds access for non-owners. |
| **Activity** | MorphToMany on Activity model. Actor and subject tracking. | MorphToMany activity logging. | Not visible in same way. | Activity logs with actor ID, action, target, metadata, timestamp. |

---

## SUBUSERS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Model** | `subusers` table: user_id, server_id, permissions(JSON). | Same model. Permissions as JSON array. | No subuser concept. Server has single owner. Multiple users can be created, but each server has one owner. | `server_subusers` table: server_id, user_id, permissions(JSON array). |
| **Permissions** | Granular permission strings: `control.console`, `control.start`, `control.stop`, `control.restart`, `file.*`, `backup.*`, `schedule.*`, `database.*`, `allocation.*`, `startup.*`, `settings.*`, `activity.*`. Stored as JSON array. | Same permission model. | N/A (scopes on user, not per-server). | Granular string permissions. Admin bypass (owner or root_admin sees everything). |
| **CRUD** | GET `/users` (list subusers), POST (add by email), GET `/{user}` (view), POST `/{user}` (update permissions), DELETE `/{user}`. | Same API. | N/A. | Full CRUD. Add by email. Update permissions. Remove. |
| **Permission check** | `$user->can($permission, $server)` — checks root_admin, owner, subuser permissions array. | Same model + Spatie role cascade. | `ContainsScope(scope)` — checks admin scope + server admin scope + direct match. | `UserCanAccessServer(ctx, serverID, userID, role, permission)` — admin bypass, owner check, subuser permission check. |

---

## PERMISSIONS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Admin model** | `root_admin` boolean on user. Gives full access to everything. | Spatie role system (`roles` table, `model_has_roles`, `role_has_permissions`). RBAC with node-level role scoping (`node_role` table). | `ScopeAdmin` string → implies all scopes. `ScopeServerAdmin` → implies all `server.*` scopes. 50+ predefined scopes. | Role string (`admin`/`user`). Admin bypasses all permission checks. |
| **API key scopes** | Per-resource read/write: `r_servers`, `r_nodes`, `r_allocations`, `r_users`, `r_locations`, `r_nests`, `r_eggs`, `r_database_hosts`, `r_server_databases`. Values: 0=None, 1=Read, 2=Write. | Resource-level permissions as JSON: `[{resource: "servers", level: "read_write"}, ...]`. | Scope strings: `"server.view"`, `"server.edit"`, `"server.delete"`, etc. | String array of permission scopes. `"*"` = full access. Admin scopes catalog. |
| **Server-level** | Subuser permissions. Checked per-operation. | Same. | Global scopes only (not per-server). | Per-server subuser permissions + global API key scopes. |
| **Middleware** | `AuthenticateApplicationUser` (root_admin), `RequireClientApiKey` (blocks application keys on client), `AuthenticateServerAccess` (owner/subuser + state check), `ResourceBelongsToServer`. | Same middleware patterns. | `AuthMiddleware` (validates session), `RequiresPermission(scope)`, `ResolveServerPanel`, `ResolveServerNode`. | `authMiddleware` (JWT/API key), `requireRole`, `requireServerPermission`. |

---

## NODES

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **Model fields** | id, uuid, public, name, description, location_id, fqdn, scheme, behind_proxy, maintenance_mode, memory, memory_overallocate, disk, disk_overallocate, upload_size, daemon_token_id(16), daemon_token(encrypted), daemonListen(8080), daemonSFTP(2022), daemonBase | Same model + tags, daemon_connect | id, name, publicHost, privateHost, publicPort, privatePort, sftpPort, secret (uuid) | N/A (daemon itself) | id, uuid, name, region_id, fqdn, scheme, behind_proxy, maintenance_mode, draining, memory_mb, disk_mb, upload_size_mb, daemon_base, daemon_listen, daemon_sftp, token_id, last_heartbeat_at, version, os, architecture, cpu_threads, docker_status, heartbeat_state |
| **Heartbeat** | Wings sends heartbeat via Panel API. Panel updates `last_seen_at`. | Same pattern. | Daemon sends periodic stats. Node status tracked. | Wings polls Panel for config changes, sends activity/stats periodically. No heartbeat as separate concept — communication is pull-based. | Beacon sends heartbeat every 30s with version, os, arch, cpu, docker status, uptime. Heartbeat monitor evaluates state transitions: healthy → suspected → unreachable → offline → recovering → healthy. |
| **State machine** | Simple: maintenance_mode boolean. No automatic state transitions. | Same + draining concept. | No state machine visible. | No central state machine (autonomous operation). | Desired state (active/maintenance/draining) + actual state (online/offline/degraded). State transitions tracked. |
| **Node config** | `getConfiguration()` returns full Wings config: uuid, token_id, decrypted token, API host/port/SSL, data directory, SFTP port, allowed mounts, panel URL. | Same config structure. | Ed25519 key-based JWT for daemon auth. Token auto-generated. | Config loaded via `config.yml` file: API host/token, system settings (data path, SFTP bind, allowed mounts, crash detection, activity send interval), Docker settings, remote query settings. | Node configuration returns full daemon config JSON. Token regeneration. Copyable daemon config for setup. |
| **Capacity** | `memory + (memory * overallocate/100)` and `disk + (disk * overallocate/100)`. `isViable(memory, disk)` checks against aggregates. | Same overallocation model. | No overallocation visible. | N/A | Total capacity + allocated capacity = available. Region-level aggregation. Capacity snapshot table for scheduling. |
| **Allocations** | Belongs to node. IP + port. Can be auto-assigned or manual. Dedicated IP and port range assignment. | Same allocation model. | IP + port per node. | Wings receives allocations from panel in server config. | IP + port + alias + notes. Range creation. Server assignment. Primary allocation. |

---

## ALLOCATIONS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Model** | id, node_id, ip, ip_alias, port, server_id(nullable), notes. | Same model + is_locked field. | IP + port on server records (not separate allocation entity). | id, node_id, ip, alias, port, server_id, notes, is_primary. |
| **Assignment** | Auto-assign via `AllocationSelectionService`: finds unassigned allocation on node. `FindViableNodesService` checks capacity. Manual via admin. | Same auto-assign. | Assigned per-server. | Manual assignment. Auto-assign via scheduler placement. |
| **Range creation** | Range notation: `25565-25570`. Single port: `25565`. Generated as individual rows. | Same. | Not applicable. | Single port, range (`25565-25570`), comma-separated (`25565,25566,25570-25575`). Regression-tested. |
| **Primary allocation** | `servers.allocation_id` points to default. Additional allocations via `allocations.server_id`. | Same. | Single allocation per server. | `servers.primary_allocation_id` + additional allocations. `setPrimaryServerAllocation` endpoint. |
| **Limits** | `servers.allocation_limit` (null = unlimited). Checked before assigning. | Same. | Not enforced. | Allocation limit enforced on server creation and assignment. |
| **Deletion guard** | Cannot delete allocation if assigned to server. Must unassign first. | Same + is_locked blocks deletion. | N/A. | Delete guard: cannot delete assigned allocation. |

---

## SERVERS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **Model** | id, external_id, uuid(36), uuidShort(8), node_id, name, description, status, skip_scripts, owner_id, memory(MB), swap(MB), disk(MB), io, cpu(%), threads, oom_disabled, allocation_id, nest_id, egg_id, startup, image, allocation_limit, database_limit, backup_limit, installed_at | Same model + docker_labels, icon | Identifier(8-char), Name, NodeID, IP, Port, Type, Icon. Server definition stored as JSON file on disk (template-driven). | Wings receives full server config from panel. | id, uuid, name, description, node_id, owner_id, egg_id, image, startup_command, memory_mb, disk_mb, cpu_limit, allocation_limit, database_limit, backup_limit, desired_state, actual_state, installed, suspended |
| **Status states** | null (ok), "installing", "install_failed", "reinstall_failed", "suspended", "restoring_backup" | Same status values. | Running process tracked in-memory. | PowerState: offline, starting, running, stopping. | desired_state + actual_state tracking. Status: installed, installing, suspended, transferring. |
| **Create flow** | 1. Validate egg variables. 2. Transaction: create server model, assign allocations, store variables. 3. Send to Wings daemon. 4. Rollback if Wings fails. | Same creation flow with deployment service. | 1. Write server JSON to disk. 2. Create environment. 3. Load scheduler from cron file. 4. Add to in-memory cache. | Wings receives `POST /api/servers` from panel, creates container, returns. | 1. Scheduler selects node/allocation. 2. Create reservation. 3. Create server in DB. 4. Confirm reservation. 5. Send config to Beacon. 6. Beacon creates container. |
| **Delete flow** | 1. Call Wings delete. 2. Transaction: delete databases, clear allocations, delete server. 3. Force delete option bypasses Wings. | Same flow. Force delete for stuck servers. | 1. Kill process. 2. Stop scheduler. 3. Destroy environment. 4. Remove files. | Wings receives `DELETE /api/servers/{uuid}`, removes container. | 1. Set desired_state=deleted. 2. Beacon deletes container. 3. Remove from DB. Force delete option. |
| **Power actions** | Start/Stop/Restart/Kill via Wings. Async — Wings handles container lifecycle. State validated before action (not suspended, not installing, node not in maintenance). | Same power model. | Start/Stop/Kill via environment. Async with queue. Auto-restart on crash with limit. | Wings: start (ContainerStart), stop (ContainerStop, 30s timeout), restart (ContainerRestart, 30s), kill (ContainerKill, SIGKILL). | Async power actions via Beacon. Mutex prevents concurrent actions. Desired state updated, actual state reconciled. |
| **Suspend** | Toggle `status = 'suspended'` or null. Calls Wings sync. Rollback if Wings fails. | Same suspend model. | Not implemented (process management instead). | Wings syncs config. | `suspended` boolean. Suspended servers cannot start. |
| **Transfer** | 1. Create transfer record (old_node, new_node, old_allocation, new_allocation). 2. Wings archives server. 3. Transfer to target node. 4. Restore. 5. Mark successful. Failure: report back to panel. | Same transfer model with rollback. | No transfer feature. | Wings: archive → scp ··· transfer → restore. Panel initiates, Wings executes. | Migration engine: planned → preparing → transferring → restoring → completed/failed. Reservation-based. Evacuation planner for full node drain. |
| **Reinstall** | Set `status = 'installing'`. Call Wings reinstall (runs egg install script again). | Same. | Template re-execution via operations pipeline. | Wings receives install config from panel, creates installer container, runs script. | Set installing state. Send install config to Beacon. Beacon runs installer container. Report result. |
| **Docker image** | `servers.image` — settable at creation and via startup update. | Same. | Specified in template JSON. | Wings receives image in server config. | `docker_image` field. Settable at creation and via settings update. |

---

## EGGS / NESTS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Nests** | Grouping for eggs. id, uuid, name, description, author. Read-only in admin (no CRUD endpoints). | Same + CRUD available. | No nest concept. Template repos serve similar purpose (git repos with JSON templates). | `nests` table: id, uuid, name, description, author. Full CRUD. |
| **Eggs** | id, uuid, nest_id, name, description, author, docker_images(JSON), startup, config_files(JSON), config_startup, config_stop, config_logs, features(JSON), force_outgoing_ip, file_denylist, script_install, script_entry, script_container, update_url. | Same model + image field, tags, config_from(parent egg linking). | Not applicable. Templates are JSON files with variables, install steps, run commands. CEL conditions for evaluation. | `eggs` table: id, uuid, nest_id, name, description, docker_images(JSON), startup_command, stop_command, install_script, install_container, install_entrypoint, config_files(JSON), features(JSON). |
| **Variables** | egg_variables table: id, egg_id, name, description, env_variable, default_value, user_viewable, user_editable, rules, is_required. Server overrides in server_variables table. | Same variable model with validation. | Template variables defined in JSON. CEL expressions in template strings. | `egg_variables` table + `server_variables` overrides. Rules for validation. User editable/viewable flags. |
| **Import/Export** | Not present in Pterodactyl (manual via DB). | Export to JSON, import from JSON. | Template repos (git clone). | Egg export to JSON. Import from JSON (create or update). |
| **Variable resolution** | `EnvironmentService` builds env map. `STARTUP` → startup command. `P_SERVER_*` → server attributes. Egg variables with server overrides. | Same resolution. | CEL template evaluation: `{{ expression }}` replaced with computed values. Conditional expressions with `if`/`else`. | Resolve `{{VAR}}` and `{{env.VAR}}` templates in startup command and config files. Server variables override egg defaults. |

---

## STARTUP VARIABLES

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Storage** | `server_variables` table: server_id, variable_id, variable_value. Egg variables are source of truth, server overrides stored here. | Same storage model. | Variables are part of server JSON definition. Updated via operations pipeline. | Egg variables define defaults. Server variables store overrides. |
| **Display** | Client API returns merged list (egg default + server override). User can edit `user_editable` variables. | Same display. | Template editor in frontend shows all variables. | Startup page shows: docker image selector, resolved startup command, editable variables (with descriptions, validation rules). |
| **Validation** | `VariableValidatorService`. Per-variable rules (required, regex, min/max). | Same validation. | Template validation at parse time. CEL condition evaluation. | Per-variable rules. Required. Type checking. Regex validation. |
| **Docker image** | `docker_images` JSON on egg (multiple images, e.g., Java 8/11/16/17). User selects via `PUT /startup/variable`. | Same image selection. | Single image per template. | Docker image selector from egg's image list. |

---

## DATABASES

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Host model** | `database_hosts`: id, node_id, host, port, username, password(encrypted), max_databases. | Same model with belongsToMany nodes. | Not implemented (no DB provisioning). | `database_hosts`: id, name, host, port, username, password(encrypted), engine(mysql/postgres). |
| **Server DB model** | `databases`: id, server_id, database_host_id, database, username, password(encrypted), remote, max_connections. | Same model. | N/A. | `server_databases`: id, server_id, database_host_id, database_name, username, password(encrypted). |
| **Provisioning** | Actually creates databases on MySQL host. `DatabaseManagementService` creates database + user + grants. Password rotation endpoint. Connection testing. | Same provisioning. | N/A. | Currently returns 501. MUST implement: actual MySQL/PostgreSQL host connection and provisioning. Database CRUD, password rotation, per-server limit enforcement, connection testing. |
| **Limits** | `servers.database_limit` (null = unlimited). Checked before creation. | Same limit enforcement. | N/A. | Database limit per server. |
| **Rotation** | `POST /rotate-password` generates new password, updates MySQL user. | Same rotation. | N/A. | Password rotation endpoint. |

---

## BACKUPS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **Model** | id, server_id, uuid, name, ignored_files, disk, sha256_hash, bytes, completed_at, is_successful, checksum, upload_id, locked. Soft-deletable. | Same model. | name, fileName, serverID, createdAt (simple model). | Wings creates backup: archives server directory to ZIP, streams to S3 or local. Reports status back to panel. | id, server_id, uuid, name, path, size_bytes, checksum(sha256), status, locked, created_at, completed_at. |
| **Creation** | Panel tells Wings to create. Wings: archive server dir → upload to S3 (if configured) or store locally. Report status via callback. | Same flow. | Local file copy with name + timestamp. | 1. Walk server root, skip .uploads/.backups/symlinks. 2. Create ZIP with deflate compression. 3. SHA256 checksum. 4. Return metadata. 5. If S3 configured: upload via multipart. | Panel triggers backup. Beacon creates ZIP from server directory (exclude .backups, .uploads, symlinks). SHA256 checksum. Return metadata. Local storage for now. S3/rclone for Phase 2. |
| **S3 storage** | AWS S3, MinIO via `aws-sdk-php`. Multipart upload. Configurable per node. | Same S3 support. | No S3 support. | Wings downloads backup from S3 for restore. Uploads multipart with progress. | S3/rclone adapter planned for Phase 2. |
| **Restore** | Panel tells Wings to restore. Wings: download from S3 or use local, extract to server dir. | Same restore flow. | Not implemented. | Extract ZIP to server root (preserving .backups/.uploads dirs). Path traversal protection on every entry. | Download backup ZIP. Extract to server root. Path traversal protection. Preserve backup/uploads dirs. |
| **Locking** | `locked` boolean. Locked backups cannot be deleted. `POST /backups/{id}/lock` toggles. | Same locking. | Not implemented. | N/A. | Backup lock flag. Locked backups protected from deletion. Lock/unlock endpoints. |
| **Limits** | `servers.backup_limit`. Count completed backups. Enforced at creation. | Same limit. | Not enforced. | N/A. | Backup limit per server. Count completed backups. Block creation when limit reached. |
| **Download** | Generates download URL. For S3: signed URL. For local: streams file. | Same download handling. | Local file download. | Streams ZIP file with `Content-Type: application/zip`. | Download endpoint. Streams file. For future S3: signed URLs. |
| **Retention** | Auto-cleanup when creating new backup and limit exceeded. | Same retention. | Manual cleanup only. | N/A. | Retention: when creating new backup and limit exceeded, delete oldest. |

---

## SCHEDULES

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Model** | `schedules`: id, server_id, name, cron_day_of_week, cron_month, cron_day_of_month, cron_hour, cron_minute, is_active, is_processing, only_when_online, last_run_at, next_run_at. `tasks`: id, schedule_id, sequence_id, action, payload, time_offset, is_queued, continue_on_failure. | Same model. | Cron tasks stored as `{serverId}.cron` JSON file. Uses `go-co-op/gocron/v2`. Tasks dict with schedule, enabled, operations. | id, server_id, name, cron_expression, is_active, only_when_online, next_run_at. Tasks: id, schedule_id, action, payload, sequence_id, continue_on_failure, time_offset. |
| **Task actions** | Three actions: `command` (send console command), `power` (start/stop/restart/kill), `backup` (create backup). Payload varies by action type. | Same three actions. | Operations pipeline: `command`, `download`, `extract`, `move`, `archive`, `sleep`, `writefile`, `mkdir`, `dockerpull`, etc. Extensible. | Three actions: `command`, `power`, `backup`. Matching Pterodactyl/Pelican action set. |
| **Task chaining** | Multiple tasks per schedule. `sequence_id` determines order. `continue_on_failure` (if false, stops chain on failure). `time_offset` (seconds between tasks). | Same chaining model. | Not chain-based — each cron entry is independent. | Task sequence with `sequence_id`. `continue_on_failure` flag. `time_offset` support. |
| **Execution** | Laravel cron runs Artisan command every minute. Checks `cron_*` fields match current time. Sets `is_processing=true`. Executes tasks in sequence. Records last_run_at, next_run_at. | Same execution model. | gocron scheduler runs in-process. | Schedule runner polls due schedules. Executes tasks in sequence order. Records run history. |
| **Run history** | `schedule_runs` + `schedule_task_runs`: track each execution with success/failure, output, timestamps. | Same history tables. | Not visible. | Schedule run history: run ID, schedule ID, status, started_at, completed_at. Task run history per task in run. |
| **Run now** | `POST /schedules/{id}/execute` triggers immediate execution. | Same. | Not visible. | Run now endpoint: executes all tasks in schedule immediately. |

---

## FILES

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **List** | GET directory listing. Returns files/folders with name, size, modified, mode. | Same. | FileServer interface. List with name, size, mode. | `GET /servers/{id}/files?directory=` — `os.ReadDir` with size/modTime. | List directory contents: name, size, modified, mode, type (file/dir/symlink). |
| **Read** | GET file contents. Max 4MB (`files.max_edit_size`). | Same limit. | FileServer read. | `GET /servers/{id}/files/content?file=` — `os.Open`, `io.Copy` capped at 1MB. | Read file contents. Configurable size limit. |
| **Write** | POST file contents. Creates or overwrites. | Same. | FileServer write. | `PUT /servers/{id}/files/content?file=` — `os.OpenFile(O_CREATE|O_TRUNC|O_WRONLY, 0640)`. Capped at 16MB. | Write file contents. Create or overwrite. Size limit. |
| **Upload** | Chunked upload. Upload ID, offset, final flag. | Same chunked upload. | Not chunked. | `PUT /servers/{id}/files/upload?path=&uploadId=&offset=&final=` — 8MB max chunk. Temp files in `.uploads/`. Offset validation. Atomic rename on final. | Chunked upload. 8MB chunks. Upload ID tracking. Offset validation. Atomic finalization via os.Rename. |
| **Delete** | POST delete (file or directory). | Same. | FileServer delete. | `DELETE /servers/{id}/files?path=` — removes files. | Delete file or directory. Recursive for directories. |
| **Mkdir** | POST create-folder. | Same. | FileServer mkdir. | `POST /servers/{id}/files/mkdir?path=` — `os.MkdirAll(0755)`. | Create directory. |
| **Rename** | PUT rename (source → target). | Same. | FileServer rename. | `PATCH /servers/{id}/files/rename` — `os.Rename`. | Rename file or directory. |
| **Copy** | POST copy (source → destination). | Same. | FileServer copy. | Not implemented in Wings baseline. | Copy file. Block copy-into-self and copy-into-descendant. |
| **Archive** | POST compress (tar.gz of selected files). | Same. | Operations pipeline: archive. | `POST /servers/{id}/files/archive` — tar.gz streaming. Skips symlinks. | Archive files to tar.gz. |
| **Decompress** | POST decompress (zip, tar.gz). | Same. | Operations pipeline: extract. | `POST /servers/{id}/files/decompress` — supports .zip, .tar.gz, .tgz. Checks expanded size against disk limit. | Decompress archive (zip, tar.gz). Size check against disk limit before extraction. |
| **Chmod** | POST chmod (file + permissions). | Same. | Not visible. | Not implemented in Wings baseline. | Chmod. Local filesystem only. |
| **Pull from URL** | POST pull (download file from URL to server). | Same. | Operations pipeline: download. | Not implemented in Wings baseline. | Download file from URL to server directory. URL scheme validation. Cap unknown remote pulls. |
| **Path safety** | Chroot per server via filesystem paths. | Same. | Chroot via FileServer interface. | `safePath()`: reject null bytes, absolute paths, `..` traversal, escape detection via `filepath.Rel`. Symlink resolution. | `safePath()`: null byte rejection, absolute path rejection, `..` rejection, `filepath.Rel` escape check, symlink resolution. Same pattern as Wings. |
| **Disk enforcement** | Docker volume size limit (kernel-enforced). | Same. | Filesystem walk check. | Wings: Docker volume limits. GamePanel Beacon currently: filesystem walk (O(n), must replace with Docker volumes or more efficient check). | Phase 1: filesystem walk (current). Phase 2: Docker volume size limits. |

---

## CONSOLE

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Wings behavior | Our target |
|---|---|---|---|---|---|
| **WebSocket** | Client connects to Wings daemon via WebSocket. Token-based auth. Bidirectional: sends commands, receives output. | Same WebSocket model. | WebSocket tracker/socket pattern. Multiple clients receive broadcast. | Wings: `GET /servers/{id}/ws` — upgrades to WebSocket. Streams container stdout/stderr. Receives stdin. 1KB read limit, 60s read deadline with pong refresh, 10s write deadline. | WebSocket to Beacon. Auth via token in query param. Bidirectional: commands in, output out. Same timeout/deadline pattern. |
| **Command history** | Not built-in. Client-side implementation. | Not built-in. | Not built-in. | Wings receives stdin via WebSocket, writes to container. | Client-side command history. localStorage or in-memory. |
| **Autoscroll** | Client-side toggle. | Client-side toggle. | Client-side. | N/A (server-side streaming). | Autoscroll toggle in UI. |
| **Search** | xterm.js addon-search. | xterm.js addon-search. | Vue component search. | N/A. | xterm.js addon-search for searching console output. |
| **Clear** | Client-side clear. | Client-side clear. | Client-side clear. | N/A. | Clear console button. |
| **Reconnect** | Auto-reconnect on disconnect. | Auto-reconnect. | Reconnect logic. | N/A (server handles). | Reconnect with backoff. Show connection status (connected/disconnected/error). |
| **State** | Wings reports server state. Panel shows. | Same. | State from server process. | Wings tracks: offline, starting, running, stopping. | Power state from Beacon. Console badge shows connected/disconnected. |

---

## ACTIVITY LOGS

| Aspect | Pterodactyl behavior | Pelican behavior | PufferPanel behavior | Our target |
|---|---|---|---|---|
| **Model** | `activity_logs`: id, batch(uuid), event, ip, description, actor(morph), properties(JSON), timestamp. `activity_log_subjects`: subject(morph). | Same activity model (morph-based). | Not implemented as structured activity log. | id, actor_id, actor_type, action, target_type, target_id, metadata(JSON), ip_address, created_at. |
| **Admin view** | Application API: `GET /activity` with pagination + filters. | Same + Filament viewer. | Not visible. | List all activity logs. Filter by action, actor, target, date range. |
| **Server view** | Client API: `GET /servers/{id}/activity`. Scoped to server. | Same. | Not visible. | List activity logs for specific server. Filter by action. |
| **Events logged** | Login/logout, server create/delete, power actions, install/reinstall, suspend/unsuspend, backup create/delete/restore, file operations, database operations, subuser add/remove, settings changes. | Same event coverage. | Not visible. | All mutation operations: server lifecycle, power, files, backups, databases, schedules, subusers, settings. |