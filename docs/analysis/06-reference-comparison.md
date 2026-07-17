# 06 — Reference Project Comparison

## Overview Table

| Aspect | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| Language | PHP 8 / Go | PHP 8 / Go | Go (monolith) | Go (monolith) |
| Architecture | Panel + separate Wings daemon | Panel + separate Wings-compatible daemon | Single binary (panel + daemon) | Panel (forge) + separate beacon daemon |
| Frontend | React 17 SPA (admin + client) | Filament PHP (admin) + React (client) | React SPA | React SPA |
| Database | MySQL | MySQL | SQLite / MySQL / PostgreSQL / SQL Server | MySQL / PostgreSQL |
| Daemon | Wings (Go, separate binary) | Wings-compatible (Pelican fork) | Built-in OR remote Go node | Beacon (Go, separate binary) |
| Auth Model | Binary `root_admin` bool | Spatie RBAC roles | OAuth2 scope system | JWT + subuser permissions + RBAC (in progress) |
| Multi-node | Yes (Locations) | Yes (Node Tags replaces Locations) | Yes (LocalNode + remote HTTP proxy) | Yes (Regions / Clusters) |
| Pterodactyl-compatible | — | Partial (Wings protocol) | No | Partial (Egg import) |
| Single-binary | No | No | Yes | No |
| Plugin system | No | Yes (ServiceProvider + plugin.json) | No | No |
| Webhook system | No | Yes (WebhookConfiguration model) | No | Planned |
| OAuth2 | No | Social login (Socialite, consumer only) | Scope-based (full issuer) | Issuer + consumer (built-in) |

---

## Pterodactyl Panel — Key Architecture

Pterodactyl is the original reference implementation. Its architecture established the conventions that downstream projects largely follow.

**Backend:** PHP 8 / Laravel. REST APIs split into three namespaces:
- `/api/client` — end-user actions (server power, files, backups, schedules, subusers)
- `/api/application` — administrative CRUD (servers, users, nodes, eggs, nests, locations)
- `/api/remote` — panel↔Wings communication (boot configs, activity ingestion, backup completion callbacks)

**Frontend:** React 17 SPA with Easy-Peasy (Redux-based state) and SWR for data fetching. Admin UI is still Blade-rendered; only the client area is a React SPA.

**Database:** MySQL only. Schema uses a `servers` → `nests` → `eggs` hierarchy with a `service_variables` / `egg_variables` table for per-egg startup variable definitions, and `server_variables` for per-server overrides.

**Daemon:** Wings — a standalone Go binary. The panel issues a node token at creation time; Wings authenticates every request from the panel using that token. Wings manages Docker lifecycle, SFTP, file management, backups, and server transfers entirely independently of the panel.

**Authorization:**
- `root_admin` boolean on the `users` table: either you are a global admin or you are not.
- Subusers: up to 34 granular per-server permissions stored as a `permissions` JSON array on the `subusers` table.

**Egg / Nest system:**
- Nests group related eggs (e.g., "Minecraft" nest contains Paper, Vanilla, Forge eggs).
- Each egg defines: Docker image, startup command, config file locations + parsers, install script, variables with validation rules.
- Config inheritance: eggs can extend a parent egg and inherit its config/script.
- Script inheritance: separate from config; install scripts can also chain.

**WebSocket flow:** Client requests a short-lived JWT from the panel (`/api/client/servers/{id}/websocket`). Client then opens a WebSocket directly to Wings using that JWT. Wings validates the JWT independently.

**Server transfer:** Initiated by admin via panel API → panel instructs source Wings to archive the server → source Wings streams archive to destination Wings → destination Wings unpacks → panel updates the node record.

---

## Pelican Panel — What Changed vs Pterodactyl

Pelican is an actively maintained fork of Pterodactyl. It preserves Wings protocol compatibility while modernizing the stack.

### Additions

- **Filament PHP** replaces the Blade admin UI with a component-driven PHP admin panel (still server-renders).
- **Spatie Permission** replaces the `root_admin` boolean with a full RBAC role/permission system. Roles are assignable to users; permissions are assignable to roles or directly to users.
- **Plugin system** — Laravel `ServiceProvider`-based; each plugin ships a `plugin.json` manifest and registers via `plugins/{id}/src/Providers/`. The `sushi` package is used to expose the filesystem plugin list as an Eloquent model (no `plugins` DB table needed).
  - Categories: `Plugin`, `Theme`, `Language`
  - Statuses: `NotInstalled`, `Disabled`, `Enabled`, `Incompatible`, `Errored`
  - Plugins can register custom permissions via `Role::registerCustomPermissions()`.
  - Update checking polls `update_url` every 10 minutes via a scheduled job.
  - Console commands placed in `src/Console/Commands/` are auto-discovered.
- **Webhooks** — `WebhookConfiguration` model stores event subscriptions + target URLs. Panel fires events (Laravel event system); a listener serializes and POSTs to each configured webhook URL.
- **OAuth2 social login** via Laravel Socialite. Routes: `GET /auth/oauth/redirect/{driver}` and `GET /auth/oauth/callback/{driver}`. Drivers are configured in `services.php`.
- **Node CPU overallocation** — nodes now store a `cpu_overallocation` percentage alongside the existing memory overallocation.
- **Node Tags** — JSON array on the node; replaces the `locations` table for grouping nodes. Tags are free-form strings.
- **Node Roles** — `node_role` pivot table assigning roles to nodes for access-scoped administration.
- **Multiple startup commands per egg** — eggs can define conditional startup variants.
- **Allocation locking** (`is_locked` flag) — prevents auto-assignment of a locked allocation to a new server.
- **Server icons** — optional icon URL stored per server.

### Removals vs Pterodactyl

| Removed | Replaced By |
|---|---|
| `nests` table | Eggs are now standalone (no parent nest) |
| `locations` table | Node Tags (free-form JSON) |
| `root_admin` column | Spatie RBAC role assigned to user |
| `name_first` / `name_last` columns | Single `username` column |
| `gravatar` column | Dropped entirely |

---

## PufferPanel — Unique Architecture

PufferPanel takes a fundamentally different approach: a single compiled Go binary that can function as panel, daemon, or both simultaneously.

### Monolith Design

- **LocalNode sentinel:** servers with `node_id = NULL` are served by the in-process daemon. No network hop, no separate process.
- **Remote nodes:** standard HTTP proxying to external PufferPanel instances acting as pure daemon nodes.
- This means a single-server deployment requires zero configuration of a separate daemon.

### Environment Abstraction

PufferPanel abstracts the execution environment behind an `Environment` interface with multiple implementations:

| Implementation | Mechanism |
|---|---|
| Docker | One container per server run; `AutoRemove: true` on exit |
| Standard (TTY) | Linux `unshare` with `pivot_root`; flags: `CLONE_NEWUSER \| CLONE_NEWNS \| CLONE_FILES \| CLONE_NEWCGROUP \| CLONE_NEWIPC \| CLONE_NEWUTS` |

Console proxy modes (per server): `stdin`, `telnet`, `RCON`, `RCON-WebSocket` — the proxy method is selected per-server in the template, not globally.

### OAuth2 Scope System

PufferPanel issues OAuth2 tokens itself (it is the authorization server). Scopes are hierarchical:

| Scope | Grants |
|---|---|
| `admin` | All operations |
| `login` | Authentication only |
| `nodes.*` | Full node management |
| `server.*` | All server operations |
| `server.admin` | Implies all `server.*` scopes |
| `users.*` | Full user management |
| `templates.*` | Template CRUD |

Scopes can be global or bound to a specific `server_id`, enabling per-server OAuth2 clients (useful for game integrations).

### Rich Install Operation Pipeline

Templates define an ordered list of operations executed at install time. 20+ built-in operation types:

| Operation | Purpose |
|---|---|
| `alterfile` | Patch a config file (JSON/YAML/Properties/etc.) |
| `archive` | Create an archive |
| `command` | Run a shell command |
| `console` | Send console input |
| `curseforge` | Download from CurseForge API |
| `dockerpull` | Pull a Docker image |
| `download` | Generic HTTP download |
| `extract` | Unpack archive |
| `fabricdl` | Download Fabric loader |
| `forgedl` | Download Forge loader |
| `javadl` | Download a JDK |
| `mkdir` | Create directory |
| `mojangdl` | Download from Mojang version manifest |
| `move` | Move / rename file |
| `neoforgedl` | Download NeoForge loader |
| `nodejsdl` | Download Node.js |
| `paperdl` | Download PaperMC build |
| `sleep` | Wait N seconds |
| `steamgamedl` | Download via SteamCMD |
| `writefile` | Write literal content to a file |

Each operation supports an `if` field evaluated by a scripting engine against template variable values, enabling conditional install steps.

### Other Unique PufferPanel Features

- **Auto-restart on crash** — configurable per server; panel-side crash detection re-issues start.
- **Keep-alive command** — a command sent to the console on a timer to prevent idle disconnects (e.g., Minecraft RCON keepalive).
- **Auto-start on panel boot** — servers marked `shouldAlwaysRun` are started when the panel process starts.
- **JVM heap statistics** — uses `jcmd <pid> GC.heap_info` to report heap usage separately from container memory.
- **WebAuthn / Passkeys** — full FIDO2 registration and authentication flow.
- **Template repositories** — pull template collections from remote git repositories via the UI; templates update on sync.
- **Multiple DB dialects** — SQLite (zero-config default), MySQL, PostgreSQL, SQL Server; dialect selected via config.
- **Per-server cron scheduler** — cron expressions, concurrent execution limit, and a `limitMode` (skip or queue) when the limit is reached.

---

## Wings Reference — Complete Feature Surface

Wings is the gold-standard daemon implementation. The following catalogs everything Beacon needs to match.

### HTTP API (27+ routes)

Routes organized by resource:

| Group | Routes |
|---|---|
| Server lifecycle | `POST /api/servers`, `DELETE /api/servers/{uuid}`, `POST /api/servers/{uuid}/reinstall`, `PATCH /api/servers/{uuid}` |
| Power | `POST /api/servers/{uuid}/power` (start/stop/restart/kill) |
| Commands | `POST /api/servers/{uuid}/commands` |
| WebSocket | `GET /api/servers/{uuid}/ws` |
| Resources | `GET /api/servers/{uuid}/resources` |
| Logs | `GET /api/servers/{uuid}/logs` |
| Files | `GET/POST/PUT/DELETE /api/servers/{uuid}/files/*` (list, contents, write, delete, create-dir, rename, copy, compress, decompress, chmod, pull) |
| Backups | `POST /api/servers/{uuid}/backup`, `DELETE /api/servers/{uuid}/backup/{backup}`, `POST /api/servers/{uuid}/backup/{backup}/restore` |
| Transfers | `POST /api/servers/{uuid}/transfer`, `DELETE /api/servers/{uuid}/transfer` |
| SFTP | `POST /api/sftp/auth` |
| System | `GET /api/system`, `GET /api/servers` |

### WebSocket Protocol

**Inbound events (client → Wings):**

| Event | Purpose |
|---|---|
| `auth` | Present JWT for authentication |
| `set-state` | Power action (start/stop/restart/kill) |
| `send-command` | Send console input |
| `send-logs` | Request log replay |
| `send-stats` | Request current resource stats |

**Outbound events (Wings → client):**

| Event | Payload |
|---|---|
| `auth success` | Authentication confirmed |
| `auth error` | Authentication failed |
| `status` | Server state change |
| `console output` | Console line(s) |
| `stats` | CPU / memory / network / disk |
| `token expiring` | JWT nearing expiry |
| `token expired` | JWT expired |
| `install output` | Install script output line |
| `install started` | Install began |
| `install completed` | Install finished |
| `transfer logs` | Transfer log line |
| `transfer status` | Transfer state change |
| `backup restore completed` | Restore finished |
| `backup complete` | Backup succeeded |
| `backup error` | Backup failed |
| `deleted` | Server deleted |
| `daemon error` | Internal daemon error |
| `jwt error` | JWT validation error |

### Docker

- Full container lifecycle: create → start → attach → stop → kill → remove.
- Streaming stats via `docker stats` event stream (CPU %, memory bytes, network I/O, block I/O).
- Memory overhead multipliers applied per container to account for JVM / runtime overhead (configurable per egg).
- Stop method types: `SIGTERM`, `SIGKILL`, `stop` command (sends a string to stdin before kill).
- Image pull on container create if not present; pull progress streamed to install WebSocket.

### SFTP

- Wings runs an internal SFTP server (not system OpenSSH).
- Auth flow: SFTP client sends username/password → Wings calls back to panel `/api/remote/sftp/auth` → panel returns allowed permissions → Wings enforces them.
- 9 SFTP operations with activity logging:

| Operation | Logged? |
|---|---|
| `read` (file download) | Yes |
| `write` (file upload) | Yes |
| `create` (new file) | Yes |
| `rename` | Yes |
| `delete` | Yes |
| `mkdir` | Yes |
| `rmdir` | Yes |
| `list` | No |
| `stat` | No |

- Activity log deduplication: repeated identical SFTP operations within a window are collapsed to one log entry.
- Session cancellation: if a server is deleted while an SFTP session is active, the session is forcibly terminated.

### File Management

15+ operations exposed over HTTP:

| Operation | Notes |
|---|---|
| List directory | Returns metadata array |
| Read file | Streaming response |
| Write file | Full replacement |
| Chunked upload | Multipart; used for large files |
| Delete file | Single |
| Batch delete | Array of paths |
| Rename | Single |
| Batch rename | Array of from/to pairs |
| Create directory | Recursive |
| Copy | File only (not directory) |
| Compress | Creates `.tar.gz` |
| Decompress | Supports `.zip` and `.tar.gz` |
| chmod | Numeric mode |
| Pull from URL | Async download into server dir |
| Search | Recursive filename/content search |

SSRF protection: Wings validates pull URLs against a denylist (RFC1918 ranges, loopback, link-local). Path safety: `openat2` syscall with `RESOLVE_BENEATH` flag prevents symlink traversal outside the server root.

### Backups

- **Local:** writes to a configurable backup directory; `pgzip` for parallel gzip compression; SHA1 checksum on completion reported to panel.
- **S3:** multipart upload; parts uploaded concurrently; checksum on completion.
- Write rate limiting: configurable bytes/sec to prevent backup I/O from starving the host.
- Backup locking: panel sets a lock flag before restore; Wings refuses deletion of locked backups.

### Transfer

- **Outgoing node:** archives the server data directory → streams archive over HTTP to destination node → sends SHA1 checksum to panel on completion.
- **Incoming node:** receives stream → verifies checksum → creates new server environment → extracts archive → starts server if it was running.

### Config File Parsers

Wings can patch game server config files during startup with dynamic variable values:

| Format | Parser |
|---|---|
| JSON | `gjson` / `sjson` |
| YAML | `go-yaml` |
| INI | Custom |
| Properties (Java) | Custom |
| XML | `etree` |

### Activity Logging

- Daemon-side activity stored in a local SQLite database.
- Batched POST to panel `/api/remote/activity` every N seconds.
- SFTP activity is deduplicated before batching.

### State Persistence

- Server runtime states written to `states.json` every 60 seconds.
- On Wings restart: reads `states.json` and re-attaches to running containers; starts servers marked as `running` that are not.

### Console Throttling

- Token bucket algorithm: 2000 lines per 100 ms window.
- Lines beyond the limit are dropped and a throttle warning is emitted on the WebSocket.

---

## What GamePanel Has That References Don't

| Feature | Description |
|---|---|
| **Region / Cluster hierarchy** | Two-level grouping above nodes: Regions contain Clusters, Clusters contain Nodes — more granular than Pterodactyl Locations or Pelican Node Tags. |
| **Evacuation planner** | Automated node drain: schedules live migrations of all servers off a node before maintenance, with configurable concurrency and dry-run mode. |
| **Recovery coordinator** | Detects node failure via heartbeat classifier → automatically migrates servers to healthy nodes based on capacity scoring. |
| **5-state heartbeat classifier** | Node health states: `Healthy`, `Degraded`, `Unhealthy`, `Unreachable`, `Unknown` — derived from rolling heartbeat windows rather than binary up/down. |
| **Placement reservations** | Capacity is reserved (soft-held) during the server creation window to prevent double-allocation when multiple servers are created concurrently. |
| **Reconciler** | Continuous desired-vs-actual state loop: compares panel DB state to beacon-reported state and issues corrective actions. |
| **Observability timeline events** | Structured event log (not just console lines): captures state transitions, migration events, placement decisions with timestamps and metadata. |
| **Node capacity snapshots** | Historical time-series records of node CPU/memory/disk usage stored in DB — enables trend analysis and capacity planning. |
| **Schedule run history** | Each schedule execution is recorded in the DB with outcome, duration, and any error — not just last-run timestamp. |
| **OAuth2 token issuer** | GamePanel issues OAuth2 tokens as an authorization server (not just a social-login consumer), enabling third-party integrations and service-to-service auth. |
| **Monaco in-browser editor** | Full Monaco editor (VS Code engine) embedded in the file manager for in-browser code editing with syntax highlighting. |
