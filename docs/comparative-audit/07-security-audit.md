# Security Audit

## Findings Summary

| Severity | Count | forge/api | forge/web | beacon |
|---|---|---|---|---|
| Critical | 3 | 2 | 1 | 0 |
| High | 5 | 3 | 1 | 1 |
| Medium | 6 | 4 | 1 | 1 |
| Low | 4 | 2 | 1 | 1 |

## Critical Findings

### 1. Migration 020 — Stray Byte (forge/api)
- **File:** `forge/api/migrations/020_regions_multi_node_foundation.sql:1`
- **Issue:** Line 1 reads `pCREATE TABLE IF NOT EXISTS regions (` — a stray `p` makes this invalid SQL
- **Impact:** All migrations ≥020 silently fail to apply. Regions, migration engine, heartbeats, evacuation, recovery, observability, OAuth2, S3 backups — none exist in DB
- **Evidence:** Verified by reading the file
- **Mitigation:** Remove leading `p` and re-run migrations

### 2. Webhook Table Orphaned (forge/api)
- **File:** `forge/api/internal/store/migrations/023_webhooks.sql`
- **Issue:** Webhooks DDL is in `internal/store/migrations/` (not `migrations/`), so `RunMigrations` never applies it
- **Impact:** Every webhook CRUD call returns 500 — table doesn't exist
- **Evidence:** `cmd/api/main.go:60` uses `MIGRATIONS_DIR` defaulting to `migrations/`
- **Mitigation:** Move file to `migrations/038_webhooks.sql`

### 3. SQL Injection in dbprovisioner (forge/api)
- **File:** `forge/api/internal/services/dbprovisioner/service.go:161,168,174,199,212,224,233,238,264,273,287`
- **Issue:** Identifier values (db name, username, host) interpolated via `fmt.Sprintf` with only `'` escaping — backticks and `"` pass through
- **Impact:** A crafted username like `a" --` can execute arbitrary SQL on database hosts
- **Evidence:** `escapeSQLString` only escapes single quotes
- **Mitigation:** Use `pq.QuoteIdentifier` or an `[a-zA-Z0-9_]` allowlist

## High Findings

### 4. Default Credentials in Production (forge/api)
- **File:** `forge/api/internal/store/store.go:668-748`
- **Issue:** `Store.Seed` runs unconditionally in production, inserts `admin@example.com`/`admin123` and `daemon_token='dev-node-token'` via ON CONFLICT DO UPDATE
- **Impact:** Every deploy gets known credentials. Node token overwrites real tokens
- **Evidence:** `cmd/api/main.go:63` unconditionally calls `Seed`
- **Mitigation:** Gate behind `APP_ENV == "development"`

### 5. No Inbound HMAC Verification (forge/api)
- **File:** `forge/api/internal/http/handlers_remote.go:21-39`
- **Issue:** Inbound daemon→panel calls authenticated only by static bearer token. No HMAC body/timestamp verification
- **Impact:** Replay attacks possible. No authenticity of request origin
- **Evidence:** Outbound calls use HMAC (`daemon/client.go:627-687`) but inbound doesn't
- **Mitigation:** Add HMAC verification middleware for `/api/remote/*`

### 6. Hard-Coded CORS Origins (forge/api)
- **File:** `forge/api/internal/http/server.go:443-447`
- **Issue:** CORS origins hard-coded to 4 `localhost` URLs. No env override (WS origins have `API_WS_ALLOWED_ORIGINS` but HTTP CORS doesn't)
- **Impact:** Production frontend on any non-localhost origin will be blocked
- **Mitigation:** Add `API_CORS_ALLOWED_ORIGINS` env variable

### 7. Auth Token Key Mismatch (forge/web)
- **Files:** `forge/web/lib/api.ts:2433` vs `stores/use-server-store.ts:54,76,78`
- **Issue:** `getToken()` reads `forge.accessToken` but everything else uses `modern-game-panel-token`
- **Impact:** WebSocket console auth silently fails — token is `undefined` in WS sub-protocol
- **Evidence:** `console.tsx:120-122` passes token from `getToken()` to WS protocols
- **Mitigation:** Unify on a single `TOKEN_KEY` constant

## Medium Findings

### 8. No Container Runtime Hardening in Beacon
- **File:** `beacon/internal/runtime/docker.go:535-554`
- **Issue:** While beacon does use `CapDrop ALL`, `ReadonlyRootfs`, `no-new-privileges`, it lacks: `openat2` syscall, rootless containers, SSH cipher lock-down, TLS config
- **Impact:** Container breakout surface is larger than Wings
- **Mitigation:** Add `openat2` support, SSH cipher configuration, TLS

### 9. Rate Limiter Fails Open (forge/api)
- **File:** `forge/api/internal/http/middleware_ratelimit.go:42-46`
- **Issue:** If Redis is down, `c.Next()` is called — rate limiting disabled
- **Impact:** Availability prioritized over security; DoS protection lost during Redis outage
- **Mitigation:** Fall back to in-memory token bucket

### 10. Custom HMAC Token (forge/api)
- **File:** `forge/api/internal/http/auth.go:26-69`
- **Issue:** User JWT is hand-rolled `base64(payload).HMAC` — no `iss`, `aud`, `jti`, no revocation list. OAuth2 tokens have JTI revocation but user tokens don't
- **Impact:** No token revocation for user sessions. Token rotation impossible
- **Mitigation:** Migrate to `golang-jwt/v5` with full claims

### 11. Silent Body Parsing (forge/api)
- **File:** `forge/api/internal/http/handlers_remote_extra.go:185,226` and `server.go:906`
- **Issue:** `_ = c.BodyParser(&body)` silently discards malformed JSON
- **Impact:** Backups, transfers, install status silently treated as empty
- **Mitigation:** Check and return 400 on parse errors

### 12. No WebSocket Connection Limits (beacon)
- **File:** `beacon/internal/server/console.go:1-284`
- **Issue:** No per-server max WebSocket connection count. Wings limits to 30
- **Impact:** Resource exhaustion possible
- **Mitigation:** Add per-server WS limit

### 13. No WebSocket Message Rate Limiting (beacon)
- **File:** beacon internal — no rate limiter on WS messages
- **Issue:** Wings limits to 10 messages per 200ms per connection. Beacon has none
- **Impact:** Abuse of WS command channel
- **Mitigation:** Add per-connection WS message rate limiter

## Low Findings

### 14. Pre-built Binary Committed
- **File:** `forge/api/api`
- **Issue:** ELF binary checked into repo
- **Impact:** Bloat, security review bypass
- **Mitigation:** Add to `.gitignore`

### 15. Go 1.26 Toolchain
- **File:** `forge/api/go.mod:3`
- **Issue:** `go 1.26.0` may not be available on standard toolchains
- **Evidence:** `go version` returns 1.26.4, but earlier versions may not resolve

### 16. Duplicate Migration Prefixes
- **File:** `forge/api/migrations/`
- **Issue:** 015, 018, 023 prefixes duplicated
- **Impact:** Order is alphabetical, not numeric; maintainability risk

## Security Comparison Summary

| Category | forge/api | forge/web | beacon | Pterodactyl | Pelican | PufferPanel | Wings |
|---|---|---|---|---|---|---|---|
| Container hardening | ✅ | N/A | ⚠️ | ✅ | ✅ | ⚠️ | ✅ |
| Path traversal | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | ✅ (openat2) |
| SQL injection risk | ⚠️ (dbprovisioner) | N/A | N/A | ✅ (ORM) | ✅ (ORM) | ✅ (GORM) | N/A |
| CSRF | ✅ | N/A | N/A | ⚠️ | ⚠️ | ✅ | N/A |
| Rate limiting | ⚠️ | N/A | ❌ | ⚠️ | ⚠️ | ⚠️ | ⚠️ |
| CORS | ⚠️ | ✅ | N/A | ✅ | ✅ | ✅ | N/A |
| WS auth | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ | ✅ |
| TLS | ✅ (configurable) | ✅ | ❌ | ✅ (via nginx) | ✅ (via nginx) | ✅ | ✅ |
| Secret management | ⚠️ | ⚠️ | ⚠️ | ✅ | ✅ | ⚠️ | ✅ |
| Audit logging | ✅ | N/A | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Overall** | **6/10** | **6/10** | **4/10** | **9/10** | **9/10** | **6/10** | **8/10** |
