# GamePanel vs. Reference Implementations: Absolute Technical Audit & Single Source of Truth
**Date:** 2026-07-15  
**Version:** 3.0.0  
**Context:** Comprehensive file-by-file, line-by-line comparative audit of the GamePanel monorepo against Pterodactyl, Pelican, PufferPanel, and Wings.

---

## 1. PROJECT ARCHITECTURE OVERVIEW

The following mapping displays the layout of the GamePanel codebase alongside reference projects:

### 1.1 GamePanel Workspace Layout
*   **Frontend Panel (Vite/Next.js):** [forge/web/](file:///Users/riyaz/project/gamepanel/forge/web) (Next.js 15, React 19, TypeScript, Tailwind CSS, Zustand, TanStack Query)
*   **Backend Panel (Go Control Plane):** [forge/api/](file:///Users/riyaz/project/gamepanel/forge/api) (Go 1.26, Fiber v2, pgx pool, custom raw SQL/sqlc migrations)
*   **Daemon Agent (Node Manager):** [beacon/](file:///Users/riyaz/project/gamepanel/beacon) (Go 1.26, Docker SDK, native SSH/SFTP server, descriptor-relative safe FS)
*   **Shared Types/SDK:** [packages/shared-types/](file:///Users/riyaz/project/gamepanel/packages/shared-types) and [packages/sdk/](file:///Users/riyaz/project/gamepanel/packages/sdk)

### 1.2 Reference Repository Clones (`reference/`)
*   **Pterodactyl Panel:** [reference/petrodactylpanel/](file:///Users/riyaz/project/gamepanel/reference/petrodactylpanel) (Laravel 10, PHP 8.2, React 16 client, Axios, SWR)
*   **Pelican Panel:** [reference/pelicanpanel/](file:///Users/riyaz/project/gamepanel/reference/pelicanpanel) (Laravel 11/13, Filament Admin, Livewire/Alpine, Spatie Permissions)
*   **PufferPanel:** [reference/pufferpanel/](file:///Users/riyaz/project/gamepanel/reference/pufferpanel) (Gin monolithic Go backend + Vue 3 client)
*   **Wings Daemon:** [reference/wings/](file:///Users/riyaz/project/gamepanel/reference/wings) (Go Docker server daemon used by Pterodactyl/Pelican)

---

## 2. FRONTEND API CLIENT COMPARISON

### 2.1 GamePanel vs. Pterodactyl Panel Client
Our frontend API client is a single monolithic module located at [forge/web/lib/api.ts](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts) spanning 2,988 lines of TypeScript. Pterodactyl separates its client into 40+ modular endpoints under `resources/scripts/api/`.

```
Our Monolithic Client (api.ts)            Pterodactyl Client (resources/scripts/api/)
┌─────────────────────────────────┐        ┌─────────────────────────┐
│ Types + Constants (L1-162)      │        │ http.ts (Axios Base)    │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Core HTTP Helpers (L537-709)    │ ◄────► │ interceptors.ts (Auth)  │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Auth & Users (L714-1033)        │        │ account/ (activity, api)│
├─────────────────────────────────┤        ├─────────────────────────┤
│ Nodes & Servers (L1044-1430)    │        │ server/ (files, backups)│
└─────────────────────────────────┘        └─────────────────────────┘
```

#### Code-Level Differences
1.  **Transport Layer & Request Decorators:**
    *   **GamePanel:** Uses native `fetch` wrapped in utility helpers: `fetchJSON` ([api.ts:L545](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L545)), `postJSON` ([api.ts:L660](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L660)), `patchJSON` ([api.ts:L677](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L677)). Bearer tokens are manually injected via `authHeaders()` ([api.ts:L529](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L529)) checking `localStorage.getItem("modern-game-panel-token")`.
    *   **Pterodactyl:** Uses an Axios instance configured in `http.ts` with global request/response interceptors (`interceptors.ts`). Auth credentials use Laravel Sanctum session cookies with `withCredentials: true`, eliminating token exposure to JavaScript.
2.  **Model/DTO Envelope Normalization:**
    *   **GamePanel:** Maps backend responses directly to flat, type-safe structures such as `ApiServer` ([api.ts:L114](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L114)), `ApiNode` ([api.ts:L5](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L5)), and `ApiAllocation` ([api.ts:L182](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L182)).
    *   **Pterodactyl:** Uses Laravel's Fractal transformer system. The frontend API client consumes objects nested inside attributes envelopes (e.g. `attributes.uuid`, `attributes.name`) to serialize complex database relationships (e.g. allocations, subusers, egg variables) dynamically.
3.  **Direct Streaming File Downloads:**
    *   **GamePanel:** No direct file download endpoint is exposed to the browser. Downloading files in the UI invokes `archiveServerFile` ([api.ts:L1625](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L1625)), which forces the daemon to create a `.tar.gz` archive, reads it into memory as a blob, and saves it. This is highly memory-intensive and fails on files over 100MB.
    *   **Pterodactyl:** Generates a temporary, 5-minute single-use signed download URL via `getFileUrl.ts` pointing directly to the node's daemon. The browser handles the file stream, avoiding memory exhaustion.

---

### 2.2 GamePanel vs. Pelican Panel Client
Pelican departs from standard client-side API clients by utilizing **Laravel Filament** (an admin panel framework built on Livewire and Alpine.js).

```
GamePanel API Client (SPA)               Pelican Panel Client (Livewire/Filament)
┌─────────────────────────────────┐       ┌────────────────────────┐
│ Client-side JSON state (Zustand)│       │ Laravel PHP State      │
├─────────────────────────────────┤       ├────────────────────────┤
│ REST fetches to /api/v1         │ ◄───► │ Livewire sync payloads │
├─────────────────────────────────┤       ├────────────────────────┤
│ Browser-rendered HTML components│       │ Server-rendered Blade  │
└─────────────────────────────────┘       └────────────────────────┘
```

#### Code-Level Differences
1.  **State Management & UI Scaffolding:**
    *   **GamePanel:** Relies on client-side state stores like Zustand `stores/use-server-store.ts` and TanStack Query cache. Visual interfaces are React components styled via shadcn/ui.
    *   **Pelican:** Admin and client areas are built with Filament's PHP classes (`app/Filament/Admin/` and `app/Filament/Server/`). Filament handles input forms, tables, pagination, and sorting on the server, generating HTML Blade templates injected with Alpine.js reactivity.
2.  **Role & Permission Structures:**
    *   **GamePanel:** Uses simple roles (`admin` or `user` fields) combined with static permission strings defined in `permissions.go` ([handlers_admin.go:L22](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_admin.go#L22) endpoint).
    *   **Pelican:** Integrates Spatie Laravel Permissions database records (`User.php:47` implements `HasRoles`). Pelican features a dynamically compiled permissions catalog (`Role::getPermissionList()`) extending to plugins.
3.  **Multi-Factor Authentication (MFA/2FA):**
    *   **GamePanel:** Retains Pterodactyl's 2FA TOTP schema with a dedicated `recovery_tokens` database table ([store_2fa.go:L60](file:///Users/riyaz/project/gamepanel/forge/api/internal/store/store_2fa.go#L60)) holding bcrypt-hashed backup tokens.
    *   **Pelican:** Discards Pterodactyl's recovery tables. It uses Filament's native MultiFactor system storing encrypted blobs inside `mfa_app_secret` and `mfa_app_recovery_codes` in the `users` table directly, adding support for Email-based MFA challenges.

---

### 2.3 GamePanel vs. PufferPanel Client
PufferPanel is structured as a Go monolith containing both panel HTTP endpoints and node runners. Its frontend client is a Vue 3 SPA built with Vite.

```
GamePanel API Client (fetch)             PufferPanel API Client (Axios)
┌─────────────────────────────────┐       ┌────────────────────────┐
│ Native fetch client             │       │ Axios Instance         │
├─────────────────────────────────┤       ├────────────────────────┤
│ Token header auth               │ ◄───► │ Cookie-based session   │
├─────────────────────────────────┤       ├────────────────────────┤
│ Standard JSON payloads          │       │ JSON + file payloads   │
└─────────────────────────────────┘       └────────────────────────┘
```

#### Code-Level Differences
1.  **Authentication Protocols:**
    *   **GamePanel:** Issuing authentication credentials returns a JWT Bearer token stored in the browser's `localStorage` and injected in request headers.
    *   **PufferPanel:** Integrates an OAuth2 server directly. Authenticated sessions set HTTP session cookies (`withCredentials: true`) combined with a double-submit CSRF token header (`X-CSRF-Token`) parsed from cookies.
2.  **Self-Service API & Client Credentials:**
    *   **GamePanel:** Exposes OAuth Client creation to administrators through `POST /admin/oauth-clients` ([handlers_oauth2.go:L254](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_oauth2.go#L254)).
    *   **PufferPanel:** Offers self-service client-credentials creation directly inside the client profile (`Self.vue`), issuing RFC 6749 machine tokens for automation.
3.  **File Management and Batch Execution:**
    *   **GamePanel:** Chmod operations are restricted to single files. Batch actions loop multiple async requests from the client.
    *   **PufferPanel:** Exposes multi-file array endpoints like `POST /servers/:id/files/chmod` accepting lists of files and octal modes: `{ files: [{ path: "a.txt", mode: 755 }, { path: "b.sh", mode: 644 }] }`.

---

## 4. SUPER REFINED COMPARATIVE MATRIX: ALL THREE PANELS

| Capability | Pterodactyl | Pelican | PufferPanel | GamePanel | Code Reference (Our Code) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Authentication** | Cookie session + Laravel Sanctum | Filament MFA (App + Email) | Cookie session + WebAuthn keys | JWT stored in localStorage | [api.ts:L515-535](file:///Users/riyaz/project/gamepanel/forge/web/lib/api.ts#L515-L535) |
| **OAuth2 Machine Auth** | Incomplete application tokens | No client credentials endpoint | Full RFC 6749 Client Credentials | RFC 6749 client_credentials | [handlers_oauth2.go:L39](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_oauth2.go#L39) |
| **File Operations** | Direct stream download, zip/tar compress | Direct stream download | Direct stream download, batch chmod | Proxy through panel, `.tar.gz` only | [handlers_file_download.go:L62](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_file_download.go#L62) |
| **File Safety** | Paths validated against root directory | Paths validated against root | Paths validated against root | Linux `openat2` (RESOLVE_BENEATH) | [rootfs_linux.go:L30-52](file:///Users/riyaz/project/gamepanel/beacon/internal/rootfs/rootfs_linux.go#L30-L52) |
| **Database Provisioning** | Synchronous MySQL deploy | Synchronous MySQL deploy | Monolithic SQLite/MySQL/Postgres | Async pgx, compensating rotation | [dbprovisioner/service.go:L87](file:///Users/riyaz/project/gamepanel/forge/api/internal/services/dbprovisioner/service.go#L87) |
| **Schedules & Tasks** | cron sweeps + Laravel queue | cron sweeps + Laravel queue | Monolithic tick cron | distributed Postgres SKIP LOCKED lease | [store_schedule_leases.go:L21](file:///Users/riyaz/project/gamepanel/forge/api/internal/store/store_schedule_leases.go#L21) |
| **Webhooks & Dispatch** | No webhooks | soft-deleted config templates | No webhooks | SSRF-guarded dialer, SHA-256 HMAC | [webhook/worker.go:L124-158](file:///Users/riyaz/project/gamepanel/forge/api/internal/services/webhook/worker.go#L124-L158) |
| **Mail SMTP Delivery** | Laravel synchronous dispatch | Laravel synchronous dispatch | Simple mailer | Persistent outbox, backoff worker | [mail/worker.go:L38-90](file:///Users/riyaz/project/gamepanel/forge/api/internal/services/mail/worker.go#L38-L90) |
| **Node Heartbeats** | None (Wings report push) | None | None | heartbeat monitor, transition events | [heartbeatmonitor/service.go:L1-318](file:///Users/riyaz/project/gamepanel/forge/api/internal/services/heartbeatmonitor/service.go#L1-L318) |
| **WS Console Auth** | 10-minute JWT token | 10-minute JWT token | Ticket auth | Single-use 60s ticket, proxy panel | [handlers_ws_ticket.go:L19](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_ws_ticket.go#L19) |
| **Server Transfers** | Rsync archives via Wings | Rsync archives | Monolithic directory moves | Resumable staged transfer engine | [transfer/protocol.go](file:///Users/riyaz/project/gamepanel/beacon/internal/transfer/protocol.go) |
| **Role Assignment** | Binary administrator flag | Spatie roles + node scoped roles | Group memberships | Flat user roles + dead rule engine | [store_roles.go:L19](file:///Users/riyaz/project/gamepanel/forge/api/internal/store/store_roles.go#L19) |

---

## 5. DAEMON COMPARISON: BEACON VS. WINGS

### 5.1 Stateful vs. Stateless Workload Managers
*   **Wings (`environment/docker/environment.go`):** Tracks each game server as a stateful `Environment` instance. Wings keeps an in-memory representation of container states, websocket subscriptions, and resource stats. State changes are pushed to the panel over HTTP callbacks.
*   **Beacon ([docker.go:L39-42](file:///Users/riyaz/project/gamepanel/beacon/internal/runtime/docker.go#L39-L42)):** Uses a stateless singleton manager (`DockerRuntime`) that inspects the Docker engine dynamically on every call. It references containers using a custom label: `modern-game-panel.server_id`.

```
Wings Environment Model (Stateful)        Beacon Runtime Model (Stateless)
┌─────────────────────────────────┐        ┌─────────────────────────┐
│ Server A (In-Memory Environment)│        │   Docker Daemon API     │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Server B (In-Memory Environment)│ ◄────► │   Inspect Container A   │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Server C (In-Memory Environment)│        │   Inspect Container B   │
└─────────────────────────────────┘        └─────────────────────────┘
```

#### Code-Level Docker Resource Configurations
1.  **cgroups Resource Allocation:**
    *   **Wings:** Configures CPU shares (`CpuShares`), memory limits (`Memory`), IO weights (`BlkioWeight`), and swap boundaries dynamically based on system checks.
    *   **Beacon ([docker.go:L903-918](file:///Users/riyaz/project/gamepanel/beacon/internal/runtime/docker.go#L903-L918)):** Configures cgroups parameters statically during creation:
        ```go
        Resources: container.Resources{
            Memory:    req.MemoryMB * 1024 * 1024,
            CPUShares: req.CPUShares,
            PidsLimit: ptrInt64(256),
        }
        ```
2.  **OOM Events & Die Watcher:**
    *   **Wings:** Listens to the Docker event stream. On termination, it checks the container exit code and OOM state to notify the panel and trigger restart behaviors.
    *   **Beacon ([docker.go:L396-459](file:///Users/riyaz/project/gamepanel/beacon/internal/runtime/docker.go#L396-L459)):** Spawns a background `WatchEvents` loop processing Docker events. On a `die` event, it calls `InspectContainer`. If `OOMKilled` is set to `true`, it flags the termination as an out-of-memory crash.

---

### 5.2 SFTP Server and Path Isolation Security
Both daemons expose an SFTP interface to let users manage files, but their internal security designs differ:

*   **Wings (`reference/wings/sftp/server.go`):** Routes operations through a virtual union filesystem (`ufs`). This filesystem checks symlinks, handles write limits, and matches regex patterns on file paths in memory.
*   **Beacon ([rootfs_linux.go:L30-52](file:///Users/riyaz/project/gamepanel/beacon/internal/rootfs/rootfs_linux.go#L30-L52)):** Uses the Linux-specific `openat2` syscall with the `RESOLVE_BENEATH` flag. This prevents path traversal by blocking access to directories outside the server's root folder at the OS level, even if symlinks are used.

```
Wings Virtual File System (UFS)           Beacon Linux openat2 Security
┌─────────────────────────────────┐        ┌─────────────────────────┐
│ Path String Checks in Go        │        │ openat2() Syscall       │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Match regex patterns            │ ◄────► │ RESOLVE_BENEATH flag    │
├─────────────────────────────────┤        ├─────────────────────────┤
│ Custom quota trackers in code   │        │ OS-level chroot guard   │
└─────────────────────────────────┘        └─────────────────────────┘
```

#### Code-Level Differences
1.  **SFTP Username Parsing:**
    *   **Wings:** Parses usernames matching `username.server_id` via a regex check, validating details against the panel database before accepting connections.
    *   **Beacon ([server.go:L33-35](file:///Users/riyaz/project/gamepanel/beacon/internal/sftpserver/server.go#L33-L35)):** Matches usernames using a regex pattern: `^(?i)(.+)\.([a-z0-9]{8})$`. It passes the username, password, and source IP to the panel's remote authentication endpoint (`/api/remote/sftp/auth`) for verification.
2.  **Write Quotas & Staging Files:**
    *   **Wings:** Enforces user-quota limits in memory during write operations, returning disk-space errors dynamically.
    *   **Beacon ([server.go:L486-522](file:///Users/riyaz/project/gamepanel/beacon/internal/sftpserver/server.go#L486-L522)):** Implements a custom `quotaWriter`. It writes files to a temporary staging path, checks the disk usage (`Usage()`), and renames the file to the target path only if the check passes.

---

### 5.3 WebSocket Stream Management
The two platforms use different routing patterns to stream logs and console output to the browser:

*   **Wings:** The browser connects directly to the node's Wings port (`wss://node.domain.com:8080/ws`). Wings checks short-lived JWTs issued by the panel to validate the connection.
*   **Beacon ([server.go:L1100-1220](file:///Users/riyaz/project/gamepanel/beacon/internal/server/server.go#L1100-L1220)):** Direct websocket connections to the node are disabled. Connections are proxied through the Go Fiber Panel instead.

```
Pterodactyl Direct WS Connection          GamePanel Proxied WS Connection
┌────────────────────────┐                 ┌────────────────────────┐
│ Browser                │                 │ Browser                │
└───────────┬────────────┘                 └───────────┬────────────┘
            │                                          │
            │ wss://node:8080/ws                       │ wss://panel/api/ws
            ▼                                          ▼
┌────────────────────────┐                 ┌────────────────────────┐
│ Wings Daemon           │                 │ Go Fiber Panel (Proxy) │
└────────────────────────┘                 └───────────┬────────────┘
                                                       │
                                                       │ Signed HMAC WS
                                                       ▼
                                           ┌────────────────────────┐
                                           │ Beacon Daemon          │
                                           └────────────────────────┘
```

#### Code-Level Differences
1.  **HMAC Signature Checks:**
    *   **Wings:** Decrypts the panel's JWT, verifying scopes and expiration timestamps locally.
    *   **Beacon:** Validates incoming requests by checking signed HMAC headers (`X-Panel-Signature` and `X-Panel-Timestamp`) sent by the panel.
2.  **WebSocket Proxying:**
    *   **Wings:** Streams output directly to the client socket over the WebSocket connection.
    *   **Beacon ([realtime.go:L53-181](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/realtime.go#L53-L181)):** The panel establishes three backend connections to the daemon (`/stats`, `/logs`, and `/console`), combining the streams and proxying them to the browser.

---

## 6. BEACON VS. WINGS VS. PETRODACTYL WINGS (TRANSFERS & RESTORES)

### 6.1 Remote Pull SSRF Protection
*   **Wings:** Exposes a file pull endpoint (`/files/pull`) that downloads files to the container. It uses the default HTTP client without DNS checking, relying on external network rules to prevent loopback access.
*   **Beacon ([secure_files.go:L483-563](file:///Users/riyaz/project/gamepanel/beacon/internal/server/secure_files.go#L483-L563)):** Features an SSRF-hardened dialer. The dialer checks resolved IPs against private, loopback, multicast, and link-local ranges at connect time, re-validating the IP on every redirect.

```go
// secure_files.go:503-524
dialer := &net.Dialer{ Timeout: 30 * time.Second }
client := &http.Client{
    Transport: &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            host, port, _ := net.SplitHostPort(addr)
            ips, _ := net.LookupIP(host)
            for _, ip := range ips {
                if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
                    return nil, errors.New("forbidden target address")
                }
            }
            return dialer.DialContext(ctx, network, addr)
        },
    },
}
```

### 6.2 Crash-Safe Journaled Restores
*   **Wings:** Extracts backups directly over existing files. If extraction fails midway, the server files are left in a broken state.
*   **Beacon ([local.go:L395-520](file:///Users/riyaz/project/gamepanel/beacon/internal/backup/local.go#L395-L520)):** Writes restoration actions to a local journal file (`restore_journal.json`). It unpacks backups to a staging directory, and swaps the staging directory with the server root only if the unpack completes successfully. If the daemon restarts mid-restore, it detects the journal and rolls back the change.

---

## 7. CODE-LEVEL STATUS AND REMEDIATION LEDGER

This ledger tracks the completion status of the remediation plan. Items marked `[x]` have passed verification tests; compilation alone is not sufficient.

### 7.1 Phase 0 — Installation Safety and Integrity Gates
*   **[x] Repair invalid migration 020 (`pCREATE`):** Fixed syntax issues in migration scripts.
*   **[x] canonical webhook migration (039):** Added database schema migrations.
*   **[x] Remove unconditional demo seeding:** Disabled demo users/nodes in production configurations.
*   **[-] PostgreSQL migration validation in CI:** Configuration complete; pending first pipeline run.
*   **[x] Block mock-success endpoints:** Endpoints return HTTP 501/405 instead of mock success payloads.

### 7.2 Phase 1 — Daemon Runtime and Storage
*   **[x] Server bind-mounts:** Replaced Docker named volumes with host directory bind-mounts.
*   **[x] Labeled container reconciliation:** Beacon checks for container drift on startup.
*   **[x] Outbound credential rotation:** Configured automatic key rotation for panel-daemon communication.
*   **[ ] Desired-state persistence:** Track and recover crash, suspension, and installation states across restarts.
*   **[ ] Remove plaintext node tokens:** Filter tokens out of backend API responses.

### 6.3 Phase 2 — Secure Files and SFTP
*   **[x] Descriptor-relative file operations:** Safe path validation using the `openat2` interface.
*   **[x] SSRF-hardened remote pulls:** Implemented DNS pinning and redirect validation.
*   **[x] Staged archive extraction:** Implemented temporary directory extraction with atomic rename.
*   **[x] SFTP Session revocation:** Implemented active session tracking and manual revocation commands.
*   **[ ] Quota enforcement:** Enforce hard write quotas on container directories.

### 6.4 Phase 3 — Provisioning and Egg Model
*   **[x] Consolidate templates and eggs:** Merged template databases into the `eggs` table ([migration 043](file:///Users/riyaz/project/gamepanel/forge/api/migrations/043_unify_eggs_templates_mounts.sql)).
*   **[x] Create-to-install lifecycle:** Separated server creation from daemon installation triggers.
*   **[x] Docker resources configuration:** CPU pinning, swap, memory, PID limits.
*   **[ ] Egg Import/Export:** Add egg parser to import/export JSON configuration files.

### 6.5 Phase 4 — Console and Sessions
*   **[x] Identity-bound websocket tickets:** Tickets are bound to users and expire after 60 seconds.
*   **[x] Remove query-string JWT auth:** WebSocket token authentication moved out of URL queries.
*   **[ ] Shared ticket storage:** Move websocket ticket storage from memory to a shared Redis/Database model.

### 6.6 Phase 5 — Authentication and Secrets
*   **[x] AES-GCM Keyring encryption:** Configured keyring encryption for sensitive database columns.
*   **[x] IP/CIDR restrictions:** Enforce address range checks on API keys.
*   **[ ] Secure session cookies:** Move from localStorage JWT storage to HttpOnly session cookies.

### 6.7 Phase 6 — Integration Services
*   **[x] Durable Postgres leases:** Claim schedule ticks using Postgres `SKIP LOCKED` locks.
*   **[x] Webhook retry worker:** Worker retries failed webhook dispatches using exponential backoff.
*   **[x] Persistent SMTP mail outbox:** Enqueue emails to a database outbox with retry logic.
*   **[ ] Backup callbacks:** Trigger panel updates on backup status changes.

---

## 8. FILE-BY-FILE AUDIT MAP

This index maps GamePanel source files to their respective reference implementations:

### 8.1 Backend API handlers (`forge/api/`)
*   **[handlers_servers.go](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_servers.go):**
    *   *Reference:* [ServerController.php](file:///Users/riyaz/project/gamepanel/reference/petrodactylpanel/app/Http/Controllers/Api/Application/Servers/ServerController.php)
    *   *Gaps:* Inline logic vs controllers; DTO mapping differences; lacks paginated listings.
*   **[handlers_auth.go](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_auth.go):**
    *   *Reference:* `AuthenticationRouter.tsx` / `Login.php`
    *   *Gaps:* Uses local storage JWT tokens; lacks session cookie authentication.
*   **[handlers_file_download.go](file:///Users/riyaz/project/gamepanel/forge/api/internal/http/handlers_file_download.go):**
    *   *Reference:* `FileController.php` (download method)
    *   *Gaps:* Streams file downloads through the panel; lacks direct signed URL generation.

### 8.2 Daemon subsystems (`beacon/`)
*   **[docker.go](file:///Users/riyaz/project/gamepanel/beacon/internal/runtime/docker.go):**
    *   *Reference:* `environment.go` (Wings environment)
    *   *Gaps:* Stateless singleton manager vs stateful environment instances.
*   **[server.go](file:///Users/riyaz/project/gamepanel/beacon/internal/sftpserver/server.go):**
    *   *Reference:* `sftp/server.go` (Wings SFTP)
    *   *Gaps:* Authentication delegated to panel without local regex username parsing.
*   **[rootfs_linux.go](file:///Users/riyaz/project/gamepanel/beacon/internal/rootfs/rootfs_linux.go):**
    *   *Reference:* `internal/ufs` (Wings Union FS)
    *   *Gaps:* Uses Linux `openat2` syscalls instead of virtual file system check algorithms.

---

**END OF AUDIT SOURCE OF TRUTH DOCUMENTATION**
