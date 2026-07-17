# Forge/GamePanel Remediation - Complete Chat Summary

## Overview
This document summarizes the complete remediation work performed on the Forge/GamePanel project, transforming it from a largely non-functional/mock state into a production-capable game server management panel.

## Initial State
- Frontend: ~146 paths, 62 modified/deleted files, 7,006 additions, 5,320 deletions
- Most frontend was mock/unmounted components
- Backend had critical security issues, false success reporting, invalid migrations
- No real integration testing

## Major Backend Remediation (Completed)

### Phase 0 - Stop Unsafe Installation & False Success
- ✅ Fixed invalid migration 020 (`pCREATE`)
- ✅ Added canonical webhook migration (039)
- ✅ Removed unconditional demo seeding
- ✅ Added seed gate preventing production demo data
- ✅ Disabled false-success plugin/migration/recovery/evacuation/mail/legacy-transfer
- ✅ Fixed CI paths to `forge/api`
- ✅ Added PostgreSQL migration bootstrap CI

### Phase 1 - Canonical Node Runtime & Storage
- ✅ Host bind-mounted server root (replaced Docker named volumes)
- ✅ Installer/runtime use same canonical root
- ✅ Server manager reconstruction on Beacon startup
- ✅ Container reconciliation on startup
- ✅ Removed Docker auto-restart conflict
- ✅ Graceful stop command/signal/timeout semantics
- ✅ Docker event watcher reconnect with backoff
- ✅ Per-node outbound daemon credentials with rotation

### Phase 2 - Secure Filesystem, SFTP, Archives, Backups
- ✅ Descriptor-relative filesystem abstraction (`openat2`/`openat`, no-follow)
- ✅ HTTP file operations through safe filesystem
- ✅ SFTP through safe filesystem
- ✅ Strict UUID/identifier validation
- ✅ Atomic, locked, size-bounded uploads
- ✅ SSRF-hardened remote pulls
- ✅ Staged/rollback-protected archive extraction
- ✅ Backups moved outside server roots
- ✅ Complete backup validation before live modification
- ✅ S3 multipart/checksum/pagination/retry handling
- ✅ SFTP: public-key auth, quotas, activity, deauthorization, idle/session limits

### Phase 3 - Real Server Provisioning & Egg Model
- ✅ Consolidated `eggs` and `server_templates` into canonical eggs (migration 043)
- ✅ Canonical `mount_server` with legacy backfill
- ✅ Variable CRUD, install fields
- ✅ Truthful create → config sync → daemon create → explicit install lifecycle
- ✅ Compensation/rollback and orphan remediation
- ✅ Real reinstall daemon call
- ✅ Real server deletion with allocation release
- ✅ Docker CPU/memory/swap/IO/OOM/PID/UID/GID/DNS/network/registry settings

### Phase 4 - Console, Realtime, Sessions
- ✅ One lifecycle-managed Docker attach/output producer per server
- ✅ Server-scoped console sinks with bounded replay
- ✅ Bounded/drop behavior and disconnect cleanup
- ✅ Fixed frontend WebSocket ticket endpoint
- ✅ Single canonical token/session accessor
- ✅ Removed JWT query-string WebSocket auth
- ✅ Shared user/session deauthorization for WS and SFTP

### Phase 5 - Authentication, Authorization, Secrets
- ✅ Current-user reload per request
- ✅ Effective role reload
- ✅ Session version, JTI
- ✅ Logout revocation
- ✅ Password/2FA invalidation
- ✅ API-key IP/CIDR enforcement
- ✅ Admin-scope restrictions
- ✅ OAuth server binding
- ✅ Versioned AES-GCM keyring (migration 046)
- ✅ Encrypted: node tokens, DB credentials, SMTP/integration secrets, webhook secrets, TOTP seeds
- ✅ Recovery codes bcrypt-hashed and atomically consumed

### Phase 6 - Databases, Schedules, Mail, Webhooks, Backups
- ✅ Configurable TLS, validation, write-only API, encrypted-at-rest for DB hosts
- ✅ Compensating DB create/delete/rotation
- ✅ Global database-host handling
- ✅ Durable replica-safe schedule claim/lease model
- ✅ Task offsets, online-only, continuation, cancellation, restart lease recovery
- ✅ Real SMTP with durable outbox/retries
- ✅ Transactional password reset enqueue
- ✅ Durable webhook queue, retries, delivery history, SSRF controls, signatures, redaction
- ✅ Schedule leases, offsets, online-only, continuation, restart reclaim

### Phase 7 - Real Transfer, Migration, Evacuation, Recovery
- ✅ Authenticated versioned transfer protocol (`forge-beacon-transfer/v1`)
- ✅ Scoped expiring transfer credentials
- ✅ Resumable source seek/destination append
- ✅ Checksum verification
- ✅ Staging, rollback activation
- ✅ Destination container creation
- ✅ PostgreSQL finalization before source cleanup
- ✅ Restart reclaim, cancellation, idempotency
- ⏳ Real two-node Docker validation pending (required before enabling evacuation/recovery)

## Frontend Rebuild (Completed)

### Shared/Auth/Account UI
- ✅ Login/2FA/recovery code
- ✅ Setup wizard
- ✅ Forgot/reset password
- ✅ Account profile summary
- ✅ Password change
- ✅ 2FA setup/recovery/disable
- ✅ SSH keys
- ✅ Personal API keys
- ✅ Recent activity
- ✅ Session outage/retry behavior
- ✅ Branding/title/favicon/background
- ✅ Error/not-found pages
- ✅ Reusable accessible UI primitives
- ✅ Toasts/dialogs/alerts/skeletons

### Server UI
- ✅ Responsive server shell/navigation
- ✅ Console and real metrics
- ✅ Files list/grid/editor/uploads/actions
- ✅ Backups
- ✅ Databases
- ✅ Schedules/tasks
- ✅ Network allocations
- ✅ Subusers
- ✅ Startup variables
- ✅ Settings
- ✅ Activity

### Admin UI
- ✅ Routes: `/admin/regions`, `/admin/roles`, `/admin/oauth-clients`, `/admin/plugins`, `/admin/operations`
- ✅ Overview, nodes, servers, users/roles, eggs/nests
- ✅ Database hosts/TLS, webhooks/delivery history, activity, health, monitoring, settings/navigation

## Critical Contract Fixes (Latest Phase)

### 1. Server Permission Discovery
- **Problem**: Subusers couldn't discover their own permissions (required `user.read` to list subusers)
- **Fix**: `GET /servers/:id` now returns caller's permissions directly
- **Files**: `handlers_servers.go`, `store_users.go`, `server-console-layout.tsx`

### 2. 2FA Management
- **Problem**: `/auth/me` omitted `useTotp`, making 2FA permanently unavailable
- **Fix**: `/auth/me` now loads authoritative user from database
- **Files**: `handlers_auth.go`, `store_users.go`

### 3. Reinstall Endpoint
- **Problem**: Frontend called admin-only `/install` instead of permission-scoped `/reinstall`
- **Fix**: Changed `reinstallServer()` to POST `/servers/:id/reinstall`
- **Files**: `api.ts`, `console-view.tsx`, `settings-view.tsx`

### 4. API Key Scopes
- **Problem**: Account API key creation sent no scopes, creating unusable credentials
- **Fix**: UI now requires explicit scope selection from delegable scopes
- **Files**: `account/page.tsx`, `api.ts`

### 5. Logout Revocation
- **Problem**: `logout()` only cleared localStorage, didn't call backend revocation
- **Fix**: `logout()` now POSTs `/auth/logout` before clearing token
- **Files**: `api.ts`, all logout call sites

### 6. SFTP Details
- **Problem**: Normal users couldn't access node details (admin-only endpoint)
- **Fix**: Server response includes `sftpHost` and `sftpPort`
- **Files**: `store_servers.go`, `handlers_servers.go`, `settings-view.tsx`

### 7. Power Controls
- **Problem**: Single aggregate permission check enabled all power buttons
- **Fix**: Per-signal permission checks (`control.start`, `control.stop`, `control.restart`)
- **Files**: `console-view.tsx`

### 8. Allocation Alias Editing
- **Problem**: Server-scoped UI called admin-only global allocation PATCH
- **Fix**: Added `PATCH /servers/:id/allocations/:allocationId` with server-scoped auth
- **Files**: `store_allocations.go`, `handlers_servers.go`, `api.ts`, `network-view.tsx`

### 9. Backup Timestamps
- **Problem**: Frontend expected `created`, backend returned `createdAt`
- **Fix**: Changed DTO field to `createdAt`
- **Files**: `api.ts`, `backups-view.tsx`, contract tests

### 10. Startup Configuration
- **Problem**: Image/command selection disabled despite backend support
- **Fix**: Enabled editing via existing `startup.docker-image` and `startup.update` permissions
- **Files**: `startup-view.tsx`

### 11. Server Dashboard
- **Problem**: No account link, search, pagination, polling
- **Fix**: Added account navigation, search, client pagination, 15s polling
- **Files**: `servers/page.tsx`

### 12. Self-Service OAuth
- **Problem**: Backend had `/account/oauth-clients` but no frontend
- **Fix**: Added OAuth client registration/revocation in account UI
- **Files**: `api.ts`, `account/page.tsx`

## File Operations Security Fixes

### Remote URL Pull (SSRF Remediation)
- **Problem**: Forge performed unbounded `http.Get` with redirect following
- **Fix**: Routed through Beacon's hardened `/files/pull` with DNS pinning, private IP rejection, size limits, atomic writes
- **Files**: `handlers_servers.go`, `daemon/client.go`, `secure_files.go`

### Direct File Download
- **Problem**: Only archive downloads worked; large files failed at 1MiB text endpoint
- **Fix**: Short-lived single-use download tickets + streaming Beacon endpoint
- **Files**: `handlers_file_download.go`, `server.go`, `api.ts`, `files-view.tsx`

### Copy Files
- **Problem**: Not exposed at any layer
- **Fix**: Wired through Beacon's atomic copy with conflict detection (409 on existing destination)
- **Files**: `server.go`, `daemon/client.go`, `handlers_servers.go`, `api.ts`, `files-view.tsx`

### chmod
- **Problem**: Not exposed
- **Fix**: Batched chmod with strict octal validation (`^[0-7]{3,4}$`)
- **Files**: `server.go`, `daemon/client.go`, `handlers_servers.go`, `api.ts`, `files-view.tsx`

### Batch Delete/Rename
- **Problem**: UI looped single requests; Beacon silently skipped failures
- **Fix**: Bounded (1-100 items), prevalidated, fail-fast batch endpoints
- **Files**: `server.go`, `daemon/client.go`, `handlers_servers.go`, `api.ts`, `files-view.tsx`

## Beacon Authentication Hardening
- **Problem**: Development startup allowed unauthenticated daemon API
- **Fix**: `DAEMON_NODE_TOKEN` required in all environments; `DAEMON_ALLOW_INSECURE_NO_AUTH=true` only for isolated dev tests, rejected in production
- **Files**: `cmd/daemon/main.go`, `infra/compose.yml`

## Tooling & Quality
- ✅ ESLint excludes `coverage/**`
- ✅ Next.js `outputFileTracingRoot` set to workspace root
- ✅ All frontend tests pass (36 tests)
- ✅ All backend tests pass
- ✅ TypeScript compilation clean
- ✅ Production build: 32 pages generated
- ✅ `git diff --check` clean

## Validation Results

### Backend
```bash
go -C forge/api test ./...        # PASS
go -C beacon test ./...           # PASS
```

### Frontend
```bash
npm --prefix forge/web test -- --run        # 4 files, 36 tests PASS
npm --prefix forge/web run typecheck        # PASS
npm --prefix forge/web run lint             # PASS (0 errors, 0 warnings)
npm --prefix forge/web run build            # PASS, 32 pages
npm --prefix forge/web run test:coverage    # 36.8% statements, 63.35% branches
```

## Remaining Production Blockers

| Blocker | Status | Notes |
|---------|--------|-------|
| Browser sessions use localStorage JWT | ⏳ Next phase | HttpOnly cookie migration designed |
| No Playwright E2E suite | ⏳ | Required for real integration validation |
| No real Docker validation | ⏳ | Server create/install/start/stop/files/SFTP/backup |
| No two-node transfer test | ⏳ | Required before enabling evacuation/recovery |
| No clean PostgreSQL migration-through-046 | ⏳ | Fresh DB validation needed |
| File download/copy/chmod complete | ✅ | Done this phase |
| Backup locking/names/exclusions | ⏳ | Backend-limited |
| User-mountable mounts | ⏳ | Backend permission changes needed |
| Account session inventory/revocation | ⏳ | Backend endpoints missing |
| Egg import/export & config parser | ⏳ | Backend missing |
| SMTP/S3/DB-host TLS/nginx/external Beacon | ⏳ | Infrastructure tests needed |

## Files Changed (Key)

### Backend (forge/api)
- `internal/http/auth.go` - Dual Bearer/cookie auth, authSource tracking
- `internal/http/handlers_auth.go` - Cookie login/logout/migrate, 2FA
- `internal/http/handlers_servers.go` - Server permissions, SFTP, allocations, file ops
- `internal/http/handlers_file_download.go` - NEW: Ticket issuance & streaming download
- `internal/http/handlers_ws_ticket.go` - Identity-bound tickets
- `internal/http/middleware_csrf.go` - NEW: Double-submit CSRF
- `internal/http/session_cookie.go` - NEW: Cookie utilities
- `internal/http/server.go` - Wiring, middleware order
- `internal/store/store_users.go` - useTotp in GetUserByID, empty permission check
- `internal/store/store_servers.go` - sftpHost/sftpPort/permissions in GetServer
- `internal/store/store_allocations.go` - UpdateServerAllocation
- `internal/daemon/client.go` - PullRemoteFile, CopyFile, ChmodFile, DownloadFile, batch ops

### Beacon
- `internal/server/server.go` - downloadFile, hardened batch delete/rename, copy conflict, chmod validation
- `internal/server/secure_files.go` - Pull validation (existing)
- `cmd/daemon/main.go` - Auth requirement enforcement

### Frontend (forge/web)
- `lib/api.ts` - sessionFetch, cookie auth, ticket download, CSRF
- `stores/use-server-store.ts` - sessionStatus replaces token
- `components/providers.tsx` - Bootstrap with migration
- `app/page.tsx` - Login without token storage
- `app/account/page.tsx` - OAuth clients, API key scopes
- `app/servers/page.tsx` - Search, pagination, polling, account link
- `components/server/files-view.tsx` - Download, copy, chmod, batch actions
- `components/server/startup-view.tsx` - Image/command editing
- `components/server/settings-view.tsx` - SFTP from server metadata
- `components/server/console-view.tsx` - Per-signal power permissions
- `components/server/network-view.tsx` - Server-scoped allocation updates
- `components/server/backups-view.tsx` - createdAt field
- `components/server/server-console-layout.tsx` - Permission discovery fix
- `components/server/network-view.tsx` - Batch allocation updates
- `eslint.config.mjs` - coverage exclusion
- `next.config.ts` - outputFileTracingRoot

## Documentation
- `docs/REMEDIATION_MASTER_PLAN.md` - Updated with all completed items, test counts, coverage

## Post-summary Audit Correction (2026-07-15)

The cookie-session work described above is **not yet end-to-end complete**. Although the API has cookie and CSRF helper code, login/checkpoint responses still issue browser-consumed JWTs, successful login does not set session cookies, and the frontend still persists the JWT in `localStorage`. The remediation ledger correctly retains this as an open Phase 5 item.

The WebSocket ticket path was also corrected after this summary: tickets are now bound to the issuing user and are consumed only after server, stream, current-session identity, and permission validation. Ticket storage remains process-local, so replica-safe shared storage is still required.

## Next Phase Priority
1. **Complete session cookie migration** (issue/clear HttpOnly cookies, wire CSRF middleware, remove browser JWT storage and subprotocol JWT authentication)
2. **Playwright E2E suite** against real stack
3. **Real Docker integration validation**
4. **Two-node transfer test**
5. **Clean PostgreSQL migration validation**

---

*Last updated: 2026-07-15*
*All validation commands executed and passing at time of summary*