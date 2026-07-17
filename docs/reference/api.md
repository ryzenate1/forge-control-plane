# Panel API

Base path: `/api/v1`

## Auth

- `POST /auth/login` - email/password login; returns an HMAC-signed bearer token
- `POST /auth/logout` - invalidate session
- `GET /auth/me` - current user and role
- All routes except `/health` and `/auth/login` require `Authorization: Bearer <token>` when `API_AUTH_SECRET` is configured.
- `GET /metrics` - Prometheus text metrics for API uptime, configured dependencies, goroutines, and memory

## Nodes

- `GET /nodes`
- `POST /nodes` - creates a Pterodactyl-style node with UUID, FQDN/scheme, daemon listen/SFTP ports, base data path, upload limit, and a `token_id.token` daemon credential
- `GET /nodes/:id`
- `GET /nodes/:id/configuration` - returns a Wings-style node configuration payload containing `uuid`, `token_id`, `token`, `api`, `system`, `remote`, and allowed origins/mounts
- `POST /nodes/:id/rotate-token` - rotates the Wings-style daemon credential

## Wings Remote API

Base path: `/api/remote`. These routes are authenticated with `Authorization: Bearer <token_id>.<token>`, matching Wings' panel-to-remote convention.

- `GET /servers` - list server configurations assigned to the authenticated node
- `POST /servers/reset` - clear transient install/restore states after daemon boot
- `GET /servers/:id` - return `settings` and `process_configuration` for one server
- `GET /servers/:id/install` - return install container, entrypoint, and script
- `POST /servers/:id/install` - accept install success/failure callbacks from the daemon
- `POST /servers/:id/transfer/success`
- `POST /servers/:id/transfer/failure`

## Allocations

- `GET /allocations` - list node IP/port allocations and assigned servers

## Database Hosts

- `GET /database-hosts` - admin-only list of PostgreSQL hosts available for server databases
- `POST /database-hosts` - admin-only PostgreSQL host creation
- `DELETE /database-hosts/:id` - admin-only deletion when no server databases are assigned

## Mounts

- `GET /mounts` - admin-only list of custom bind mounts
- `POST /mounts` - admin-only mount creation with node and egg assignment
- `DELETE /mounts/:id` - admin-only mount deletion

## Servers

- `GET /servers` - lists admin-visible servers, or only servers owned by/currently shared with the authenticated user
- `POST /servers` - create a server record and assign an allocation
- `GET /servers/:id`
- `POST /servers/:id/power` with `signal`: `start`, `stop`, `restart`, or `kill`
- `POST /servers/:id/install` - ask the node daemon to create the server container and data directory
- `DELETE /servers/:id` - ask the daemon to remove the container and mark the server deleted
- `GET /servers/:id/stats`
- `GET /servers/:id/logs`
- `GET /servers/:id/backups` - list ZIP backups created by the daemon
- `POST /servers/:id/backups` - admin-only backup creation
- `GET /servers/:id/backups/download` - stream a backup ZIP by `name`
- `GET /servers/:id/files`
- `GET /servers/:id/files/content`
- `PUT /servers/:id/files/content`
- `PUT /servers/:id/files/upload` - chunked upload with `path`, `uploadId`, `offset`, and `final`; proxies the request body as a stream
- `DELETE /servers/:id/files`
- `POST /servers/:id/files/mkdir`
- `PATCH /servers/:id/files/rename`
- `GET /servers/:id/databases` - list database records for a server
- `POST /servers/:id/databases` - create a server PostgreSQL database record on an available database host
- `POST /servers/:id/databases/:databaseId/rotate-password` - rotate and return the generated database password
- `DELETE /servers/:id/databases/:databaseId` - delete a server database record
- `GET /servers/:id/mounts` - list active custom mounts for a server
- `POST /servers/:id/mounts` - admin-only assign an allowed mount to a server
- `DELETE /servers/:id/mounts/:mountId` - admin-only remove a mount from a server
- `GET /servers/:id/schedules` - list server schedules and tasks
- `POST /servers/:id/schedules` - create a schedule
- `PATCH /servers/:id/schedules/:scheduleId` - update schedule timing/enabled state
- `DELETE /servers/:id/schedules/:scheduleId` - delete a schedule
- `POST /servers/:id/schedules/:scheduleId/tasks` - add a schedule task
- `PATCH /servers/:id/schedules/:scheduleId/tasks/:taskId` - update a schedule task
- `DELETE /servers/:id/schedules/:scheduleId/tasks/:taskId` - delete a schedule task
- `GET /servers/:id/schedules/:scheduleId/runs` - list recent schedule runs
- `POST /servers/:id/schedules/:scheduleId/run` - manually execute a schedule

The schedule runner wakes from PostgreSQL `LISTEN/NOTIFY` on schedule or task changes, keeps a timer for the nearest `next_run_at`, and falls back to minute polling if notifications are unavailable.

Server-scoped routes enforce granular permissions for subusers. Admins and server owners bypass these checks. Subusers must have the matching permission such as `file.read`, `file.update`, `backup.create`, `schedule.update`, `database.delete`, `control.start`, `control.stop`, or `control.restart`.

The frontend file workspace uses Monaco Editor on top of the jailed file API. It can browse the selected server directory, read file content, edit it, save it back through the API, and upload large files through the chunked upload route.

## Transfer

The development Docker stack exposes an SFTP sidecar on host port `2222`, mounted to the same `game-servers` volume used by the daemon. The frontend Transfer view shows SFTP and rsync-over-SSH commands for the selected server directory.

## Templates

- `GET /templates`
- `POST /templates` - admin-only template creation
- `GET /templates/:id`

## Users

- `GET /users` - admin-only user list
- `POST /users` - admin-only user creation with `admin` or `user` role

## Audit

- `GET /audit` - admin-only global audit stream

## Database

Migrations are tracked in `schema_migrations`. Startup applies only pending SQL files and records each version after its transaction succeeds.

The first migration creates users, nodes, server templates, servers, and audit events. Minecraft Java should be inserted as the first `server_templates` record when seed data is added.

Install requests include the assigned allocation when one exists. For Minecraft Java v1, the panel publishes container port `25565/tcp` to the assigned host IP and port.

## Daemon Trust

Panel-to-daemon HTTP requests are signed with `X-Panel-Timestamp` and `X-Panel-Signature`. The signature is HMAC-SHA256 over method, request URI, timestamp, and request body using the node token. Streaming upload chunks sign method, request URI, and timestamp without buffering the body, so upload integrity is enforced by the trusted daemon channel plus ordered chunk offsets.

Daemon-to-panel remote API requests use Pterodactyl/Wings-style bearer credentials: `Authorization: Bearer <token_id>.<token>`. The panel resolves the node by `token_id` and validates the token before returning remote server data.

Panel user tokens are HMAC-signed bearer tokens issued by `/auth/login`. For production, set `API_AUTH_SECRET` to a strong secret and replace the dev default.

## Realtime

- `GET /servers/:id/ws/console`
- `GET /servers/:id/ws/stats`
- `GET /servers/:id/ws/logs`

WebSocket connections use the same bearer token as REST. Browser clients may pass `?token=<bearer>` because the WebSocket API cannot set custom headers. The API gateway signs its upstream daemon WebSocket handshake with the node token. Console text frames are forwarded to container stdin; daemon output frames are forwarded back to the browser.

Server WebSocket connections require `websocket.connect` unless the user is an admin or server owner. The proxy uses a shared cancellation context for both pumps and closes both sockets when either side disconnects.
