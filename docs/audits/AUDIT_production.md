# Production Readiness Audit

**Project:** GamePanel (Forge Control Plane)
**Date:** 2026-07-17
**Audit Scope:** Security, Error Handling, Logging, Production Necessities, Deployment Config

---

## 1. Security Concerns

### 1.1 .env Files — Properly Gitignored (PASS)

- `.gitignore` correctly excludes `.env`, `.env.*`, and `!.env.example`.
- Verified: Neither `/.env` nor `/infra/.env` are tracked by git. **No secrets committed.**

### 1.2 Secrets/API Keys/Tokens in the Codebase

| File | Severity | Finding |
|------|----------|---------|
| `.env.example` (root) | LOW | Contains example `FORGE_MASTER_KEY=YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=` (base64 of all `a` bytes). The `.env.example` is intended as a template, but the presence of a real-looking key could lead to a developer copying it unchanged into production. |
| `/infra/.env` (untracked) | LOW | Contains dev secrets (`dev-api-secret`, `dev-node-token`, `gamepanel` PG password). These are local-dev defaults but serve as a reminder that real production secrets must rotate all of these. |
| `.github/workflows/ci.yml` | LOW | Hardcodes `API_AUTH_SECRET` and `DAEMON_NODE_TOKEN` for CI runs. Acceptable for ephemeral CI, but they are visible in the public repo. |
| `forge/web/lib/api.ts:44` | INFO | Default `NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1` — safe default, overridable via env. |
| `forge/api/cmd/api/main.go` | INFO | Various `http://localhost:3000` defaults — safe, overridable via env. |

**Production validation is in place:**
- `main.go` (API) rejects `API_AUTH_SECRET == ""` or `== "dev-api-secret"` in production mode (line 77).
- `main.go` (Beacon) rejects `DAEMON_NODE_TOKEN == ""` or `== "dev-node-token"` in production mode (line 100).
- `FORGE_MASTER_KEY` is required when `DATABASE_URL` is set in production (line 503-504).

### 1.3 Hardcoded URLs That Should Be Configurable

All URLs found are configurable via environment variables:
- `PANEL_URL` (default `http://localhost:3000`) — used for email links
- `NEXT_PUBLIC_API_URL` (default `http://localhost:8080/api/v1`) — frontend API target
- `API_INTERNAL_URL` (default `http://api:8080`) — web-to-API proxy
- `PANEL_API_URL` (default `http://localhost:8080/api/v1`) — daemon-to-API URL

**Conclusion:** All configurable. No hardcoded production URLs. **(PASS)**

### 1.4 Missing Environment Variable Validation at Startup

| Component | Status | Detail |
|-----------|--------|--------|
| Forge API | **PARTIAL** | `appEnv`, `authSecret`, `databaseURL`, `FORGE_MASTER_KEY` are validated. But many other env vars like `REDIS_ADDR`, `LANGS_DIR`, `PLUGINS_DIR` use silent defaults without warning. |
| Beacon (daemon) | **MOSTLY GOOD** | Validates `DAEMON_NODE_TOKEN`, `DAEMON_ALLOW_INSECURE_NO_AUTH`, and `FORGE_MASTER_KEY` equivalents. However, S3 backup adapter config (`S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET`, `S3_REGION`) is only validated when `BACKUP_ADAPTER=s3` is selected, not at startup — an operator could configure S3 but forget required env vars and only discover at backup time. |

---

## 2. Error Handling

### 2.1 Empty Catch Blocks

Found **15+ empty or near-empty catch blocks** in the TypeScript frontend:

| File | Line | Context | Risk |
|------|------|---------|------|
| `forge/web/lib/api.ts` | 65 | `catch {}` — cookie access error | Low (cookie access failures are benign) |
| `forge/web/lib/api.ts` | 138 | `catch {}` — error body reading | Low |
| `forge/web/lib/api/http.ts` | 154 | `catch {}` — getErrorMessage fallback | Low |
| `forge/web/lib/api/auth.ts` | 62 | `catch {}` — password reset email | Low |
| `forge/web/lib/api/servers.ts` | 294 | `catch {}` | Low |
| `forge/web/components/server/console-view.tsx` | 160,172,180 | `catch {}` — JSON.parse | Acceptable |
| `forge/web/components/server/stats.tsx` | 65 | `catch {}` | Low |
| `forge/web/components/server/files-view.tsx` | 103 | `catch { setError(...) }` | Acceptable |
| `forge/web/components/server/databases-view.tsx` | 35 | `catch {}` — clipboard | Acceptable |
| `forge/web/components/server/file-download-button.tsx` | 30 | `catch {}` | Low |
| `forge/web/components/server/console-charts.tsx` | 103 | `catch {}` | Low |
| `forge/web/components/branding.tsx` | 17 | `catch { return undefined; }` | Acceptable |
| `forge/web/components/ui/auth-utils.ts` | 6 | `catch {}` | Low |
| `forge/web/app/account/page.tsx` | 18 | `catch { toast(...) }` | Acceptable |

**Conclusion (PASS with caveats):** Most empty catches have explicit comments explaining why swallowing is safe. None pose a security or data-loss risk, but they do represent silent failures that could make debugging harder in production.

### 2.2 Unhandled Promise Rejections

**No global handler** (`process.on('unhandledRejection', ...)`) exists in the web frontend or any Node.js runtime context. Unhandled rejections will crash the Next.js process in production. **ACTION REQUIRED.**

### 2.3 Global Error Handler

**Forge API:** Uses `fiber.NewError(...)` throughout but does **not** set a custom `fiber.Config.ErrorHandler`. Falls back to Fiber's default error handler, which returns HTML for non-JSON requests. The API should have a custom error handler that always returns JSON. **(MINOR ISSUE)**

**Beacon (daemon):** The main daemon server (`beacon/internal/server/server.go`) does not have panic recovery middleware. A panic in any handler will crash the daemon. **(ACTION REQUIRED)**

**Beacon (new API sub-server):** `beacon/internal/api/middleware.go` contains stubs for `RecoveryMiddleware`, `ErrorHandler`, `AuthMiddleware`, `CORSMiddleware`, `CSRFMiddleware`, `RequestIDMiddleware`, `LoggingMiddleware`, and `RateLimitMiddleware` — all are empty placeholders with `// Implement ... logic` comments. This sub-server appears to be unused (no callers found), but if wired in, it would provide **zero security** as all middleware is no-op. **(DORMANT RISK)**

---

## 3. Logging

### 3.1 Structured Logging

| Component | Status | Detail |
|-----------|--------|--------|
| Forge API | **GOOD** | Uses `log/slog` with configurable format (`json`/`text`), level (`debug`/`info`/`warn`/`error`), and output (`stdout`/`stderr`/file). Has structured request logging middleware. |
| Beacon (daemon) | **POOR** | Uses Go's standard `log.Printf` everywhere. No structured fields, no log levels (except `WARNING` prefix convention), no JSON output option. A `zap`-based structured logger exists in `beacon/internal/logging/logging.go` but is **not used anywhere** in the codebase. |

### 3.2 Sensitive Data Potentially Logged

| Scenario | Risk |
|----------|------|
| Beacon logs `"synced %d server configurations from panel"` | Safe |
| Beacon S3 adapter logs `"s3 (bucket=%s region=%s prefix=%s ...)"` | Safe (no keys) |
| Beacon SFTP rejects invalid username format | Logs username — could contain PII |
| Beacon heartbeat failure logs | Safe |
| API request logging middleware logs paths, IPs, user agents | PII concern — IPs and UAs are logged |
| API `safeAuditMeta` (server.go:1262) | Claims to sanitize audit metadata, but needs review |

**Beacon's reliance on `log.Printf` is a concern:** it cannot be tuned per-environment, and any accidentally logged sensitive data cannot be redacted at the output level. **(ACTION: migrate Beacon to structured logging)**

---

## 4. Production Necessities

### 4.1 Health Check Endpoint

| Component | Endpoint | Detail |
|-----------|----------|--------|
| Forge API | `GET /api/v1/health` | Aggregates database, cache, daemon, system, and queue checks. Supports `/health/:check`. Also has `/health/live`, `/health/ready`, `/health/startup` via `HealthHandlers`. |
| Forge API | `--healthcheck` CLI flag | Docker health check support. |
| Beacon | `GET /health` | Returns `{"ok": true, "service": "daemon", "runtime": <bool>}`. |
| Beacon | `--healthcheck` CLI flag | Docker health check support. |

**Conclusion: PASS**

### 4.2 Graceful Shutdown Handling

| Component | Detail |
|-----------|--------|
| Forge API | `signal.NotifyContext` for SIGINT/SIGTERM. Calls `app.Shutdown()` (Fiber). Waits for mail worker, webhook, queue, event relay. 30s startup context timeout. |
| Beacon | Captures SIGINT/SIGTERM. Calls `server.Shutdown()`, `shutdownManager.Shutdown()`, `httpServer.Shutdown()` with 30s timeout. Stops SFTP, cron, heartbeat, activity. |
| Web (Next.js) | Default Next.js graceful shutdown. No custom logic. |

**Conclusion: PASS**

### 4.3 Rate Limiting

| Component | Implementation | Detail |
|-----------|---------------|--------|
| Forge API | Redis-backed + in-memory fallback | Auth: 5 req/min, Mutation: 30 req/min, Read: 120 req/min. Falls back to in-memory if Redis unavailable. |
| Beacon (daemon) | In-memory token bucket per IP | Power: 30/min, WS: 60/min, Files: 120/min, Default: 240/min. `golang.org/x/time/rate`. |

**Conclusion: PASS** — Rate limiting present for both services. API's Redis-backed limiter correctly fails closed (in-memory fallback).

**Note:** API rate limiting is **only effective when Redis is configured**. Without Redis, `RateLimitConfig.Enabled` is `false` and rate limiting is completely bypassed. A startup warning would help operators.

### 4.4 CORS Configuration

| Component | Status | Detail |
|-----------|--------|--------|
| Forge API | **GOOD** | Configured via `API_CORS_ALLOWED_ORIGINS` env var. Default allows `localhost:3000`, `127.0.0.1:3000`, `localhost:3002`, `127.0.0.1:3002`. Allows credentials. |
| Beacon (daemon HTTP) | **NONE** | Main daemon HTTP server does not set CORS headers. Only WebSocket origin checked (via `DAEMON_WS_ALLOWED_ORIGINS`). Acceptable since daemon is API-facing, not browser-facing. |
| Beacon (API sub-server) | **STUB** | `CORSMiddleware` is a no-op. Unused. |

**Conclusion: PASS with caveats**

### 4.5 Security Headers

| Component | Headers | Detail |
|-----------|---------|--------|
| Forge API | **COMPREHENSIVE** | CSP, X-Frame-Options: DENY, X-Content-Type-Options: nosniff, X-XSS-Protection, Referrer-Policy, Permissions-Policy, HSTS (conditional). Applied via `SecurityHeaders()` middleware. |
| Beacon (daemon) | **NONE** | No security headers on daemon HTTP responses. |
| Web (Next.js) | **NONE** | No custom security headers in `next.config.ts` or middleware. |

**Conclusion: PARTIAL.** API is well-protected. Beacon and Web frontend need headers.

### 4.6 CSRF Protection

| Component | Status | Detail |
|-----------|--------|--------|
| Forge API | **GOOD** | Double-submit cookie pattern. Validates `X-CSRF-Token` header against `__Host-forge_csrf` cookie. Also validates `Origin` and `Sec-Fetch-Site`. |
| Beacon | **NONE** | No CSRF (acceptable for daemon-to-API HMAC auth). |

---

## 5. Docker & Deployment Configuration

### 5.1 Dockerfiles

| Component | Status | Issues |
|-----------|--------|--------|
| `forge/api/Dockerfile` | **GOOD** | Multi-stage build, distroless runtime (`alpine:3.21`), non-root user (`appuser`). Copies migrations. Minimal. |
| `forge/web/Dockerfile` | **PARTIAL** | Multi-stage build, standalone output. **Missing `USER node`** — runs as root. |
| `beacon/Dockerfile` | **PARTIAL** | Multi-stage build. **Runs as root** — no `USER` directive. |

### 5.2 Docker Compose

`infra/compose.yml` is comprehensive:
- Postgres 16 + Redis 7 with health checks
- API with health checks, dependency ordering, `VAR:?error` env validation
- Daemon with health checks, Docker socket mount
- Prometheus + Alertmanager + Grafana for monitoring
- Web frontend with build args
- Separate frontend/backend networks
- Named volumes

**Issues:**
- Compose file is in `infra/` directory, not project root — operators must know where to run it.

### 5.3 Missing .dockerignore

| Directory | Has .dockerignore? |
|-----------|-------------------|
| `forge/api/` | **NO** |
| `forge/web/` | **NO** |
| `beacon/` | **NO** |

Without `.dockerignore`, builds may include `node_modules/`, `.git/`, `.next/`, bloating context and potentially leaking secrets.

### 5.4 CI/CD

GitHub Actions workflows present:
- `ci.yml` — Lint, build, test for all 3 components
- `docker.yml` — Multi-arch Docker build/push to GHCR
- `deploy.yml` — Deployment
- `release.yml` — Release
- `dependabot.yml` — Dependency updates

**Conclusion: PASS**

---

## 6. Summary of Key Issues

### CRITICAL (Fix before production)

1. **Beacon daemon has no panic recovery middleware** — A panic in any HTTP handler will crash the daemon process. Add `recover()` middleware to the beacon's HTTP server.
2. **Web frontend missing `process.on('unhandledRejection')`** — Unhandled promise rejections will crash Next.js in production.

### HIGH (Fix before production)

3. **Beacon Dockerfile runs as root** — Add `USER` directive.
4. **Web Dockerfile runs as root** — Add `USER node` to runner stage.
5. **Beacon uses unstructured `log.Printf` everywhere** — Structured logging (`slog` or `zap`) needed. A zap interface exists but is unused.

### MEDIUM (Fix soon after launch)

6. **Beacon API sub-server has all-stub middleware** — `beacon/internal/api/middleware.go` contains no-op implementations for Auth, CORS, CSRF, Rate Limit, Logging, Recovery. Either implement or remove this dead code.
7. **Beacon lacks security headers** — No CSP, X-Frame-Options, etc. on daemon HTTP.
8. **Web frontend lacks security headers** — No CSP in Next.js config or middleware.
9. **Forge API lacks custom error handler** — Default Fiber error handler returns HTML for non-JSON responses.

### LOW

10. **Missing `.dockerignore` files** in `forge/api/`, `forge/web/`, `beacon/`.
11. **Empty catch blocks** (15+) in frontend — collectively make debugging harder.
12. **S3 backup config not validated at startup** — Misconfigured S3 settings only surface at backup time.
13. **pprof endpoint on daemon** — Could leak info if accidentally enabled in production.
14. **`.env.example` contains real-looking master key** — Consider using a clearly placeholder value like `CHANGE_ME`.
15. **Rate limiting bypassed when Redis is absent** — No startup warning if Redis is missing.
