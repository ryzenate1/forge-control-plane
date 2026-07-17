# Daemon

The daemon is a lean Go service that runs on each game node and exposes signed HTTP/WebSocket endpoints to the panel API.

## Responsibilities

- Register/heartbeat with the panel API
- Fetch assigned server configuration from the panel's Wings-compatible `/api/remote` API on boot
- Reset transient panel-side server states after boot recovery
- Create, start, stop, restart, kill, and delete containers
- Apply Panel-provided custom bind mounts during container creation when the mount is allowed for the node
- Stream console logs
- Report Docker stats
- Serve jailed file operations for each server
- Serve native panel-authenticated SFTP sessions
- Reject unsigned panel requests when `DAEMON_NODE_TOKEN` is configured

The daemon accepts either the project-native `DAEMON_NODE_ID`, `DAEMON_NODE_TOKEN`, and `PANEL_API_URL` environment variables or Wings-like `WINGS_NODE_ID`, `WINGS_TOKEN_ID`, `WINGS_TOKEN`, and `WINGS_PANEL_URL`. When both `WINGS_TOKEN_ID` and `WINGS_TOKEN` are present, the daemon authenticates to the panel as `Bearer <token_id>.<token>`. Native SFTP listens on `DAEMON_SFTP_ADDR` or `:2022` when panel API URL and node token are configured.

## Runtime Interface

The daemon must code against a runtime interface:

- `Create`
- `Start`
- `Stop`
- `Kill`
- `Restart`
- `Stats`
- `Logs`
- `Delete`

Runtimes may also implement `WatchEvents` to stream container lifecycle events into the daemon `ServerManager`.

Docker is the first implementation. containerd and Podman are future implementations. The Docker implementation binds requested port mappings from the panel. Minecraft Java v1 maps the assigned allocation to `25565/tcp` inside the container.

Custom mounts follow the Pterodactyl/Wings shape: the panel includes allowed mount source paths in node configuration and includes assigned server mounts in server configuration. The daemon keeps `/home/container` as the default server data mount and adds approved bind mounts as extra Docker mounts.

## HTTP Surface

- `POST /servers` - create server data directory and container
- `DELETE /servers/:id` - remove the runtime container
- `GET /metrics` - Prometheus text metrics for daemon uptime, runtime availability, goroutines, and memory
- `POST /servers/:id/power` - start, stop, restart, or kill
- `GET /servers/:id/stats` - Docker stats with mock fallback for missing containers
- `GET /servers/:id/logs` - recent stdout/stderr logs, capped at 256 KiB
- `POST /servers/:id/backups` - create a ZIP archive of the jailed server directory
- `GET /servers/:id/backups` - list backup archives
- `GET /servers/:id/backups/download` - stream one backup archive
- `GET /servers/:id/ws/stats` - JSON stats snapshots every two seconds
- `GET /servers/:id/ws/logs` - recent log snapshots every three seconds
- `GET /servers/:id/ws/console` - attach to container stdin/stdout/stderr for interactive console
- `GET /servers/:id/files` - list a jailed directory
- `GET /servers/:id/files/content` - read one file, capped at 1 MiB
- `PUT /servers/:id/files/content` - stream-write one file, capped at 16 MiB for v1
- `PUT /servers/:id/files/upload` - append one upload chunk, capped at 8 MiB per chunk, then atomically moves the completed temp file into the jail when `final=true`
- `POST /servers/:id/files/mkdir` - create a jailed directory
- `PATCH /servers/:id/files/rename` - rename inside the jail
- `DELETE /servers/:id/files` - delete one jailed path; deleting the server root is rejected

## Native SFTP

The daemon runs a Go-native SSH/SFTP server using its panel remote credentials. Password login calls the panel's `/api/remote/sftp/auth` endpoint with the SFTP username, password, and client IP. The panel returns the server ID and granted file permissions, and the daemon creates a jailed handler rooted at that server's data directory.

SFTP permission mapping:

- `file.read` allows directory listing and stat calls.
- `file.read-content` allows file reads and downloads.
- `file.create` allows new file and directory creation.
- `file.update` allows existing file writes and renames.
- `file.delete` allows file and directory deletion.
- `*` allows all file actions.

The SFTP server rejects absolute paths, `..` traversal, and null bytes before touching the filesystem.

## Request Signing

When `DAEMON_NODE_TOKEN` is set, every route except `/health` requires `X-Panel-Timestamp` and `X-Panel-Signature`. The signature is HMAC-SHA256 over method, request URI, timestamp, and body. Timestamps outside a five-minute window are rejected. Streaming upload routes sign the request metadata without reading the body into daemon memory.

## Concurrency Rules

- Every request accepts `context.Context`.
- Power actions go through `ServerManager` and use one per-server lock.
- Concurrent start, stop, restart, and kill requests for the same server fail fast.
- Start and restart run `onBeforeStart` preflight before touching the runtime.
- Preflight blocks startup while install is active, before configuration sync, or when current disk usage exceeds the server disk limit.
- Expected stop/kill/restart exits are marked offline and are not treated as crashes.
- Unexpected non-zero or OOM exits from Docker events trigger automatic restart when crash detection is enabled and the server is outside the 60-second cooldown.
- Long-running streams stop when the context is canceled.
- Channels are bounded.
- Shutdown closes listeners and waits for active workers.
- WebSocket clients have ping/pong heartbeat and write deadlines.
- Console WebSockets use one writer lock per connection so output frames and heartbeat pings cannot race.
