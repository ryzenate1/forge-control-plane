# 07 — Feature Matrix

## Server Management

| Feature | Pterodactyl | Pelican | PufferPanel | GamePanel | Notes |
|---|---|---|---|---|---|
| Create server | ✅ | ✅ | ✅ | ✅ | |
| Delete server | ✅ | ✅ | ✅ | ✅ | |
| Power (start/stop/restart/kill) | ✅ | ✅ | ✅ | ✅ | |
| Console (WebSocket) | ✅ | ✅ | ✅ | ✅ | |
| Resource stats (CPU/mem/net/disk) | ✅ | ✅ | ✅ | ✅ | |
| Reinstall | ✅ | ✅ | ✅ | ✅ | |
| Server transfer (live) | ✅ | ✅ | ❌ | ✅ | PufferPanel has no cross-node transfer |
| Suspend / unsuspend | ✅ | ✅ | ❌ | ✅ | |
| Settings (rename, limits) | ✅ | ✅ | ✅ | ✅ | |
| Activity log | ✅ | ✅ | ⚠️ | ✅ | PufferPanel logs are minimal |
| Crash auto-restart | ❌ | ❌ | ✅ | ⚠️ | GamePanel: reconciler detects crash, restart not yet wired |
| Subuser / access sharing | ✅ | ✅ | ⚠️ | ✅ | PufferPanel: OAuth2 client per-server only |
| Server icon | ❌ | ✅ | ❌ | ❌ | |
| Allocation locking | ❌ | ✅ | ❌ | ❌ | |
| Evacuation / live migrate | ❌ | ❌ | ❌ | ✅ | GamePanel unique |
| Auto-recovery from node failure | ❌ | ❌ | ❌ | ✅ | GamePanel unique |
| Placement reservations | ❌ | ❌ | ❌ | ✅ | GamePanel unique |

---

## File Management

| Feature | Wings | Beacon | Gap |
|---|---|---|---|
| List directory | ✅ | ✅ | — |
| Read file | ✅ | ✅ | — |
| Write file | ✅ | ✅ | — |
| Chunked upload | ✅ | ⚠️ | Multipart not fully implemented |
| Delete (single) | ✅ | ✅ | — |
| Batch delete | ✅ | ✅ | — |
| Rename | ✅ | ✅ | — |
| Batch rename | ✅ | ✅ | — |
| Create directory | ✅ | ✅ | — |
| Copy | ✅ (files only) | ⚠️ | Directory copy not supported in Wings or Beacon |
| Compress | ✅ | ✅ | — |
| Decompress | ✅ (.zip + .tar.gz only) | ⚠️ | Same limitation as Wings; other formats not supported |
| chmod | ✅ | ⚠️ | Implemented but not surfaced in UI |
| Pull from URL | ✅ | ⚠️ | No SSRF validation (see security assessment) |
| Search (recursive) | ✅ | ❌ | Not yet implemented |
| Egg file denylist | ✅ | ❌ | Wings enforces per-egg file access restrictions; Beacon does not |
| Safe path (openat2) | ✅ | ⚠️ | Beacon uses `filepath.Rel` only; no `openat2` / `RESOLVE_BENEATH` |
| Symlink resolution | ✅ | ❌ | Wings resolves and validates symlinks; Beacon does not |
| Monaco in-browser editor | ❌ | ✅ | GamePanel unique |
| SFTP | ✅ | ✅ | — |
| SFTP activity logging | ✅ | ⚠️ | Partial; deduplication missing |

---

## Database & Backups & Schedules

### Databases

| Feature | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| Per-server databases | ✅ | ✅ | ❌ | ✅ |
| Auto-deploy DB host | ✅ | ✅ | ❌ | ✅ |
| Max connections enforced | ✅ | ✅ | ❌ | ✅ |
| Multiple DB hosts | ✅ | ✅ | ❌ | ✅ |
| DB host tagging / selection | ❌ | ❌ | ❌ | ⚠️ |

### Backups

| Feature | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| Local filesystem backup | ✅ | ✅ | ❌ | ✅ |
| S3-compatible backup | ✅ | ✅ | ❌ | ✅ |
| Backup locking (restore guard) | ✅ | ✅ | ❌ | ✅ |
| Backup checksum (SHA1) | ✅ | ✅ | ❌ | ✅ |
| Checksum reported to panel | ✅ | ✅ | ❌ | ✅ |
| Backup ignore rules | ✅ | ✅ | ❌ | ✅ |
| Write rate limiting | ✅ | ✅ | ❌ | ❌ |
| pgzip parallel compression | ✅ | ✅ | ❌ | ❌ |
| Backup restore | ✅ | ✅ | ❌ | ✅ |

### Schedules

| Feature | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| Cron-expression schedules | ✅ | ✅ | ✅ | ✅ |
| Power action task | ✅ | ✅ | ❌ | ✅ |
| Command task | ✅ | ✅ | ✅ | ✅ |
| Backup task | ✅ | ✅ | ❌ | ✅ |
| `only_when_online` guard | ✅ | ✅ | ❌ | ✅ |
| `continue_on_failure` | ✅ | ✅ | ❌ | ✅ |
| Chained task steps | ✅ | ✅ | ❌ | ✅ |
| Schedule run history in DB | ❌ | ❌ | ❌ | ✅ |
| Concurrent execution limit | ❌ | ❌ | ✅ | ❌ |

---

## Authentication & Security

| Feature | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| Session-based auth | ✅ | ✅ | ✅ | ✅ |
| HMAC-JWT (panel↔daemon) | ✅ | ✅ | ❌ | ✅ |
| OAuth2 issuer (built-in) | ❌ | ❌ | ✅ | ✅ |
| OAuth2 social login (consumer) | ❌ | ✅ | ❌ | ⚠️ |
| WebAuthn / Passkeys | ❌ | ❌ | ✅ | ❌ |
| API keys (user-generated) | ✅ | ✅ | ✅ | ✅ |
| TOTP 2FA | ✅ | ✅ | ✅ | ✅ |
| 2FA enforcement (admin-required) | ✅ | ✅ | ❌ | ⚠️ |
| Recovery tokens | ✅ | ✅ | ❌ | ✅ |
| SSH public keys | ❌ | ❌ | ✅ | ❌ |
| RBAC roles | ❌ | ✅ | ❌ | ⚠️ |
| Binary admin flag | ✅ | ❌ | ❌ | ⚠️ |
| Granular subuser permissions | ✅ (34) | ✅ (40+) | ❌ | ✅ (40+) |
| Node HMAC request signing | ✅ | ✅ | ❌ | ✅ |
| Login rate limiting | ⚠️ | ✅ | ⚠️ | ✅ |
| reCAPTCHA | ✅ | ✅ | ❌ | ❌ |
| TLS between panel and daemon | ✅ | ✅ | ✅ | ❌ |
| JWT revocation list | ❌ | ❌ | ❌ | ✅ |
| Single-use WebSocket tickets | ✅ | ✅ | ❌ | ✅ |

---

## GamePanel-Exclusive Orchestration Features

| Feature | Status | What It Does | Missing / Gap |
|---|---|---|---|
| Region / Cluster hierarchy | ✅ Implemented | Two-level grouping above nodes for geographic/logical organization | UI for region-level management incomplete |
| Heartbeat classifier (5-state) | ✅ Implemented | Derives `Healthy/Degraded/Unhealthy/Unreachable/Unknown` from rolling beacon ping windows | Degraded→Unhealthy threshold not yet tunable via UI |
| Placement reservations | ✅ Implemented | Holds node capacity during server creation to prevent race-condition double-allocation | Reservation TTL cleanup job not yet running |
| Reconciler | ✅ Implemented | Continuously diffs desired DB state vs actual beacon-reported state; issues corrective actions | Reconcile interval not yet configurable per-cluster |
| Node capacity snapshots | ✅ Implemented | Stores time-series capacity records in DB for trend analysis | Retention policy / pruning job not implemented |
| Evacuation planner | ⚠️ Partial | Drains servers off a node before maintenance; supports dry-run and concurrency limit | Migration sequencing and rollback not complete |
| Recovery coordinator | ⚠️ Partial | Detects node failure → scores candidate nodes → initiates server migration | Scoring algorithm not yet accounting for NUMA/locality |
| Observability timeline events | ⚠️ Partial | Structured event log for state transitions and orchestration decisions | Event schema not finalized; UI not built |
| Schedule run history | ✅ Implemented | Records each schedule execution (outcome, duration, error) in DB | UI for browsing run history not complete |
| OAuth2 token issuer | ✅ Implemented | GamePanel acts as authorization server; issues tokens for third-party service integrations | Scope management UI not built |

---

## Frontend Completeness

| Feature Area | Admin UI | Server UI | API (Client) | Complete? |
|---|---|---|---|---|
| Server create / delete | ✅ | — | ✅ | ✅ |
| Server power controls | — | ✅ | ✅ | ✅ |
| Console (WebSocket) | — | ✅ | ✅ | ✅ |
| Resource stats | — | ✅ | ✅ | ✅ |
| File manager (list/read/write) | — | ✅ | ✅ | ✅ |
| Monaco file editor | — | ✅ | ✅ | ✅ |
| File upload | — | ✅ | ✅ | ⚠️ Chunked incomplete |
| File pull from URL | — | ✅ | ✅ | ⚠️ No SSRF guard |
| File compress / decompress | — | ✅ | ✅ | ✅ |
| SFTP credentials display | — | ✅ | ✅ | ✅ |
| Databases | — | ✅ | ✅ | ✅ |
| Backups (list/create/delete) | — | ✅ | ✅ | ✅ |
| Backup restore | — | ✅ | ✅ | ✅ |
| Schedules (CRUD) | — | ✅ | ✅ | ✅ |
| Schedule run history | — | ❌ | ✅ | ❌ UI missing |
| Subusers / permissions | — | ✅ | ✅ | ✅ |
| Activity log | — | ✅ | ✅ | ✅ |
| Server settings | — | ✅ | ✅ | ✅ |
| Server reinstall | — | ✅ | ✅ | ✅ |
| Server transfer | ✅ | — | ✅ | ✅ |
| Server suspend | ✅ | — | ✅ | ✅ |
| Node management (CRUD) | ✅ | — | ✅ | ✅ |
| Node capacity snapshots | ❌ | — | ✅ | ❌ UI missing |
| Evacuation planner UI | ❌ | — | ✅ | ❌ UI missing |
| Recovery coordinator UI | ❌ | — | ⚠️ | ❌ UI missing |
| Region / Cluster management | ⚠️ | — | ✅ | ⚠️ Partial |
| Egg management (CRUD) | ✅ | — | ✅ | ✅ |
| Egg variable editor | ✅ | — | ✅ | ✅ |
| User management | ✅ | — | ✅ | ✅ |
| RBAC role management | ❌ | — | ⚠️ | ❌ Not built |
| OAuth2 client management | ❌ | — | ✅ | ❌ UI missing |
| API key management | ✅ | — | ✅ | ✅ |
| 2FA setup | — | ✅ | ✅ | ✅ |
| Observability timeline | ❌ | ❌ | ⚠️ | ❌ Not built |
| reCAPTCHA config | ❌ | — | — | ❌ Not implemented |
| Webhook configuration | ❌ | — | — | ❌ Not implemented |
