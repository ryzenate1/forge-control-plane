# Agent 6 — Cross-Reference General Lookup Report

> **Scope:** All five projects (GamePanel, Pterodactyl, Pelican Panel, PufferPanel, Wings)
> **Date:** 2026-07-16
> **Methodology:** Pattern-based cross-project comparison across 10 dimensions

---

## Table of Contents

1. [Configuration Patterns](#1-configuration-patterns)
2. [Error Handling Patterns](#2-error-handling-patterns)
3. [Testing Coverage](#3-testing-coverage)
4. [Dependency Analysis](#4-dependency-analysis)
5. [Docker / Deployment](#5-docker--deployment)
6. [CI/CD Pipelines](#6-cicd-pipelines)
7. [Documentation Quality](#7-documentation-quality)
8. [Security Headers / Defaults](#8-security-headers--defaults)
9. [WebSocket Implementation](#9-websocket-implementation)
10. [Database Migration Strategy](#10-database-migration-strategy)
11. [Cross-Cutting Insights](#11-cross-cutting-insights)

---

## 1. Configuration Patterns

### Summary Table

| Aspect | GamePanel (Beacon) | GamePanel (Forge API) | Pterodactyl (Wings) | PufferPanel | Pelican/Pterodactyl Panel |
|---|---|---|---|---|---|
| **Config format** | YAML | `.env` + env vars | YAML + `.envrc` | JSON (Viper) | `.env` (Laravel) |
| **Config loader** | Custom (`gopkg.in/yaml.v2`) | Env vars (Fiber) | Custom + `creasty/defaults` | `spf13/viper` | Laravel `config()` |
| **Env prefix** | None | None | None | `PUFFER_` | Laravel conventions |
| **Defaults** | Explicit `Default()` function | Go zero values + Docker args | `creasty/defaults` tags | Viper defaults | `.env.example` |
| **Hot reload** | No | No | Yes (`WriteToDisk`) | Yes (file watcher) | No (requires artisan) |
| **Thread safety** | `sync.RWMutex` | N/A | `sync.RWMutex` | N/A (Viper handles) | N/A (Laravel config cache) |
| **Config locations** | File path CLI arg | `.env` / Docker args | `/etc/pterodactyl/config.yml` | `/etc/pufferpanel/config.json` | `.env` |
| **Save to disk** | Yes (`Save()`) | No | Yes (`WriteToDisk`) | Yes (auto) | No (manual artisan) |

### Key Findings

- **Beacon** uses a clean, minimal YAML config with explicit `Default()` and thread-safe access via `sync.RWMutex`. This is a good pattern — better than Wings which mixes YAML struct tags with `creasty/defaults`.
- **Wings** has the most complex config (`376 lines`), handling TLS, backup limits, transfer limits, console throttles, openat2, log rotation, timezone, and Docker. It overloads the config struct — some of these should be separate concerns.
- **PufferPanel** is the only project using `spf13/viper` for config with automatic env var binding (`PUFFER_` prefix). This is the most mature config approach. It also searches multiple paths (`$PUFFER_CONFIG`, `/etc/pufferpanel/config.json`, `./config.json`).
- **Forge API** uses minimal env vars with no structured config file — relying on Docker `ARG`/`ENV` for deployment configuration. This is lean but lacks validation and discovery.
- **PHP Panels** (Pterodactyl, Pelican) use Laravel's `.env` convention — standard but well-understood.

### Gap: GamePanel Forge API

The Forge API lacks a structured config file or config validation layer. Unlike every reference project, it has no config loading ceremony, no defaults documentation, and no runtime config access pattern. Consider adding either a YAML config (like Beacon) or a Viper-based approach (like PufferPanel).

---

## 2. Error Handling Patterns

### Pattern Comparison

| Pattern | GamePanel (Beacon) | GamePanel (Forge API) | Wings | PufferPanel | PHP Panels |
|---|---|---|---|---|---|
| **Go error propagation** | `if err != nil` (standard) | `if err != nil` (standard) | `if err != nil` + `emperror.dev/errors` | `if err != nil` + `pkg/errors` | N/A |
| **Panic recovery** | Not found in HTTP layer | Not found in HTTP layer | `recover()` in router middleware | `recover()` in gin middleware | Laravel exception handler |
| **Error wrapping** | `fmt.Errorf` | `fmt.Errorf` | `emperror.dev/errors` with context | `pkg/errors` with stack traces | Laravel `throw` + custom exceptions |
| **Structured errors** | No custom error types | No custom error types | Custom error types in `server/errors/` | Custom error types | Laravel exception classes |
| **Logging on error** | `log.Printf` | `log.Printf` | `apex/log` structured logging | `logrus` structured logging | Laravel `Log::error()` |
| **Graceful shutdown** | `signal.Notify` context | `signal.Notify` context | `signal.Notify` context | `signal.Notify` context | Supervisor manages |

### Key Findings

- **Neither GamePanel component has panic recovery in its HTTP layer.** This is a critical gap. Wings, PufferPanel, and PHP panels all implement `recover()` middleware. A panic in a goroutine handler will crash the entire process without recovery.
- **Beacon and Forge API use the most basic error handling** — no structured error types, no error wrapping with context. Wings uses `emperror.dev/errors` and PufferPanel uses `pkg/errors` for enriched stack traces and error context.
- **All Go projects** follow the standard `if err != nil` Go idiom, which is correct.
- **PHP panels** benefit from Laravel's centralized exception handler, which provides consistent error formatting, logging, and HTTP response generation.

### Gap: No Panic Recovery

**CRITICAL:** Neither `beacon/` nor `forge/api/` has `recover()` middleware. Add panic recovery middleware to both HTTP servers immediately — a single `nil` pointer dereference in any handler will crash the entire service.

---

## 3. Testing Coverage

### Test File Counts

| Project | Test Files | Source Files (approx) | Coverage Estimate | Test Types |
|---|---|---|---|---|
| **GamePanel Beacon** | 18 | ~25 | ~72% | Unit, integration |
| **GamePanel Forge API** | 27 | ~45 | ~60% | Unit, integration, security |
| **Wings** | 15 | ~55 | ~27% | Unit |
| **PufferPanel** | 17 | ~80 | ~21% | Unit, integration |
| **Pterodactyl Panel** | 70 | ~250 | ~28% | Integration, feature |
| **Pelican Panel** | 62 | ~250 | ~25% | Integration, feature, Filament |

### Test Distribution

**GamePanel (82 test files combined)**
- Beacon: 18 files — backup (local, S3), remote client, rootfs, runtime (Docker stats), server (console, manager, secure_files, handlers), serverid, SFTP, system (activity dedup), transfer protocol
- Forge API: 27 files — HTTP handlers (auth security, file download, OAuth2, plugins, WS ticket, status, schedules, server lifecycle), daemon client, Docker runtime, keyring, DB provisioner, evacuation planner, mail SMTP, migration, webhook security/worker, store layer (API keys, databases, eggs, encryption, migration transfer, server lifecycle, SQL split)

**Reference Projects**
- Wings: 15 files — events, progress, filesystem, HTTP, console, power, locker, rate, sink pool, utils
- PufferPanel: 17 files — conditions, filesystem, models, operations (NeoForge, Steam), scopes, servers, services (token), utils
- Pterodactyl: 70 files — extensive API integration tests for all endpoints
- Pelican: 62 files — API integration tests, Filament admin tests, service tests

### Key Findings

- **GamePanel has the highest test density** of all projects — 82 test files across a relatively small codebase. The 27 Forge API tests include security-specific tests (`auth_security_test.go`, `webhook/security_test.go`) that no other project has.
- **Forge API tests are notably well-categorized**: daemon client, HTTP handlers, runtime, secrets, services (DB provisioner, evacuation planner, mail, migration, webhook), and store layer (with integration tests).
- **Wings has poor test coverage** (~27%) for a production project — only 15 test files for ~55 source files.
- **Pterodactyl/Pelican** have the most tests by count (70/62) but lower density due to the larger Laravel codebase.
- **GamePanel is unique in having security-focused test files** (`auth_security_test.go`, `handlers_ws_ticket_test.go`, `webhook/security_test.go`).

### Gap: No Frontend Tests

Neither GamePanel nor any reference project has frontend JavaScript/TypeScript test files. All projects ship frontend code without automated testing — relying entirely on type checking and linting.

---

## 4. Dependency Analysis

### Go Dependencies (Direct)

| Dependency | GamePanel Beacon | GamePanel Forge API | Wings | PufferPanel |
|---|---|---|---|---|
| **Go version** | 1.26 | 1.26 | 1.24 | 1.25 |
| **Web framework** | None (net/http) | GoFiber v2 | Gin v1 | Gin v1 |
| **WebSocket** | gorilla/websocket | gorilla/websocket + gofiber/contrib | gorilla/websocket | gorilla/websocket |
| **Database** | None | pgx (PostgreSQL) | gorm (SQLite) | gorm (SQLite/MySQL/Postgres/SQLServer) |
| **Docker SDK** | docker/docker v28 | None | docker/docker v28 | docker/docker v28 |
| **SFTP** | pkg/sftp | None | pkg/sftp | pkg/sftp |
| **JWT** | None | golang-jwt v5 | gbrlsnchs/jwt v3 | golang-jwt v5 |
| **Config** | yaml.v2 | None (env vars) | yaml.v2/v3 + ini.v1 | viper + yaml |
| **Crypto** | golang.org/x/crypto | golang.org/x/crypto | golang.org/x/crypto | golang.org/x/crypto |
| **Testing** | None (stdlib) | None (stdlib) | testify | testify |
| **Direct deps** | **12** | **9** | **52** | **58** |
| **Indirect deps** | **49** | **30** | **90** | **151** |

### PHP Dependencies

| Aspect | Pterodactyl Panel | Pelican Panel |
|---|---|---|
| **PHP version** | ^8.2 \|\| ^8.3 | ^8.3 \|\| ^8.4 \|\| ^8.5 |
| **Framework** | Laravel 11 | Laravel 13 |
| **Admin panel** | Custom (UI package) | Filament v5 |
| **Auth** | Sanctum + JWT | Sanctum + Socialite + WebAuthn |
| **Testing** | PHPUnit 10 | Pest v4 |
| **Static analysis** | PHPStan + Larastan | Larastan |
| **Direct deps** | ~35 | ~30 |
| **License** | MIT | AGPL-3.0 |

### Key Findings

- **GamePanel has the leanest dependency footprint** — 12 direct deps for Beacon, 9 for Forge API. This is a significant advantage for security (smaller attack surface) and maintainability.
- **Wings and PufferPanel have bloated dependency trees** — 52/58 direct and 90/151 indirect. PufferPanel's dependency count is nearly 4x GamePanel's.
- **GamePanel uses PostgreSQL exclusively** (pgx) while Wings/PufferPanel use SQLite via GORM. This is a modern, production-appropriate choice.
- **Wings still uses gbrlsnchs/jwt v3** (unmaintained), while GamePanel and PufferPanel have migrated to golang-jwt v5.
- **PufferPanel supports 4 database backends** (SQLite, MySQL, PostgreSQL, SQL Server) — impressive but adds complexity.
- **PufferPanel is the only Go project using swagger** (swaggo) for API documentation generation.
- **GamePanel's use of GoFiber** (forge API) vs standard `net/http` (beacon) is an interesting inconsistency within the same project.

### Gap: Framework Inconsistency

Beacon uses `net/http` directly while Forge API uses GoFiber. This means different middleware patterns, different request/response handling, and different WebSocket integration approaches within the same project. Consider standardizing.

---

## 5. Docker / Deployment

### Dockerfile Comparison

| Aspect | GamePanel Beacon | GamePanel Forge API | GamePanel Forge Web | Wings | PufferPanel | Pelican Panel | Pterodactyl Panel |
|---|---|---|---|---|---|---|---|
| **Multi-stage** | Yes (2) | Yes (2) | Yes (3) | Yes (2) | Yes (2) | Yes (4+) | Yes (2) |
| **Base image** | alpine:3.21 | alpine:3.21 | node:20-alpine | distroless/static | alpine | Custom base-php | php:8.3-fpm-alpine |
| **Non-root user** | No | Yes (appuser) | No | Yes (distroless) | No (commented out) | Yes (www-data) | Yes (nginx) |
| **CGO_ENABLED** | 0 | 0 | N/A | 0 | 1 (CGO for SQLite) | N/A | N/A |
| **Health check** | No | No | No | No | No | Yes | No |
| **Volume mounts** | No | No | No | No | 3 volumes | 1 volume | No |
| **Port exposed** | 9090, 2022 | 8080 | 3000 | 8080, 2022 | 8080, 5657 | 80, 443 | 80, 443 |
| **Security** | Root user, no healthcheck | Non-root user | Root user | Distroless (best) | Root user | www-data | nginx |

### Key Findings

- **Wings has the most secure Docker setup** — uses `gcr.io/distroless/static:latest` as the final image, which contains no shell, package manager, or unnecessary binaries. This is the gold standard for container security.
- **GamePanel Beacon and Forge API have good practices** (multi-stage build, non-root user for API) but lack health checks and running as non-root (Beacon).
- **PufferPanel's Docker user is commented out** — a clear security regression. The `#USER pufferpanel` comment suggests this was intentional for debugging but left in production.
- **Pelican Panel has the most sophisticated Dockerfile** — 4+ stages, custom base image, health checks, supervisor, and multi-platform support.
- **PufferPanel runs `db upgrade` during build** (line 90: `RUN /pufferpanel/bin/pufferpanel db upgrade`) — this is an anti-pattern as it bakes migration state into the image.
- **Only Pelican Panel has a HEALTHCHECK** — all others rely on external monitoring.

### Gap: Missing Health Checks

Neither GamePanel component has Docker `HEALTHCHECK` instructions. Add health check endpoints and Dockerfile HEALTHCHECK directives for all three services (Beacon, Forge API, Forge Web).

### Gap: Beacon Runs as Root

Beacon Dockerfile does not create a non-root user. The Forge API correctly creates `appuser` but Beacon should do the same.

---

## 6. CI/CD Pipelines

### Pipeline Comparison

| Aspect | GamePanel | Wings | PufferPanel | Pelican Panel | Pterodactyl Panel |
|---|---|---|---|---|---|
| **CI workflows** | 3 (ci.yml, docker.yml, release.yml) | 0 (no CI in repo) | 5 (build, formatter, tester, arch, run-template-tester) | 3 (docker-publish, upload/download translations) | 0 (uses Pterodactyl's CI) |
| **Linting** | golangci-lint v2 | None | None in CI | None in CI | phpstan + php-cs-fixer |
| **Build** | `go build` + `npm run build` | N/A | `go build` + `yarn build` | Composer + Yarn | Yarn + Composer |
| **Tests in CI** | `go test ./...` | N/A | `go test ./...` (in Dockerfile only) | Not in main CI | PHPUnit |
| **Docker build** | Multi-arch (amd64, arm64) via GHCR | N/A | Multi-arch via GHCR/DockerHub | Multi-arch (amd64, arm64) via GHCR | Multi-arch via GHCR |
| **Release** | Tag-triggered GitHub Release | N/A | Tag-triggered + PackageCloud (DEB/RPM) | Tag-triggered | Tag-triggered |
| **Caching** | Go module cache + GHA Docker cache | N/A | Go cache + npm cache | GHA Docker cache | N/A |
| **Secrets** | Minimal (CI-only tokens) | N/A | CurseForge, PackageCloud, Registry | GITHUB_TOKEN | N/A |

### Key Findings

- **GamePanel has the most complete CI pipeline** among the Go projects — lint, build, test, Docker build, and release, all with proper service containers (PostgreSQL) for integration tests.
- **PufferPanel has the most mature release pipeline** — builds DEB and RPM packages, pushes to PackageCloud, supports multiple registries (GHCR, DockerHub), and creates GitHub Releases with artifacts.
- **Wings has no CI pipeline in its repository** — it relies on Pterodactyl's CI or separate infrastructure.
- **GamePanel's CI correctly tests against PostgreSQL** with a service container — this is better than Wings/PufferPanel which only test against SQLite.
- **GamePanel's release workflow** generates release notes from git log — simple but functional.
- **PufferPanel's build is the most complex** — 5 workflows, multi-architecture DEB/RPM packaging, swagger generation, and template testing.

### Gap: No Frontend Testing in CI

GamePanel's `ci.yml` runs `typecheck` and `lint` for the frontend but has no test step. Consider adding frontend tests if they exist.

---

## 7. Documentation Quality

### Documentation Inventory

| Document Type | GamePanel | Wings | PufferPanel | Pelican Panel | Pterodactyl Panel |
|---|---|---|---|---|---|
| **README** | Yes (brief) | Yes (brief) | Yes (brief) | Yes (detailed, with game table) | Yes (detailed, with sponsors) |
| **CONTRIBUTING** | No | No | Yes | Yes | Yes |
| **SECURITY** | No | No (references Pterodactyl) | Yes | Yes | Yes |
| **CHANGELOG** | No | Yes | No (uses GitHub Releases) | No (uses GitHub Releases) | Yes |
| **LICENSE** | No explicit file | No explicit file | No explicit file | AGPL-3.0 (in composer.json) | MIT |
| **API docs** | No | No | Yes (`api.md` + Swagger) | Yes (Scramble auto-generated) | No (external docs site) |
| **Architecture docs** | Yes (`docs/` folder) | No | No | No | No |
| **Inline docs** | Store README, Realtime README | UFS README ("Coming Soon") | None | None | None |
| **Markdown files** | 13+ docs | 3 | 11 | 5 | 7 |

### Key Findings

- **GamePanel has the most extensive internal documentation** — the `docs/` folder contains architecture docs, planning docs, audit reports, handoff docs, and decision records. This is far more than any reference project.
- **However, GamePanel lacks standard open-source docs** — no CONTRIBUTING.md, no SECURITY.md, no LICENSE file, no CHANGELOG.
- **PufferPanel has the best API documentation** — an `api.md` file plus auto-generated Swagger docs.
- **Pelican Panel has the most polished README** — includes a detailed game/egg table, contribution guide, and sponsor acknowledgments.
- **Wings has a stub README** for its UFS package ("Coming Soon™").
- **Pterodactyl has the strongest brand presence** — badge-laden README with sponsor table, demo GIF, and links to external documentation site.

### Gap: Missing Open-Source Essentials

GamePanel needs:
- `LICENSE` file
- `CONTRIBUTING.md`
- `SECURITY.md`
- `CHANGELOG.md` (or a CHANGELOG section in releases)

---

## 8. Security Headers / Defaults

### Security Pattern Comparison

| Security Feature | GamePanel Beacon | GamePanel Forge API | Wings | PufferPanel | PHP Panels |
|---|---|---|---|---|---|
| **CORS** | Not implemented | Not implemented | `AllowedOrigins` config field | `gin-contrib/cors` (middleware) | Laravel middleware |
| **CSRF** | N/A (API only) | N/A (API only) | N/A (API only) | Session-based CSRF via gin | Laravel VerifyCsrfToken |
| **Rate limiting** | Not found | Not found | `system/rate.go` + `juju/ratelimit` | Not found | Laravel throttle middleware |
| **Security headers** | None | `auth_security_test.go` (tests only) | `X-Frame-Options` in router | None | Laravel middleware stack |
| **TLS** | Not in config | Not in config | Full TLS config (cert, key, port) | Not in config | Nginx/Caddy |
| **Trusted proxies** | Not found | Not found | `TrustedProxies` config | Not found | Laravel TrustedProxies |
| **Input validation** | `yaml.v2` tags | Fiber validator | `validator/v10` | `validator.v9` | Laravel Form Requests |
| **Secret management** | YAML file (chmod 0o600) | Env vars | YAML file + JWT tokens | Env vars + config file | `.env` file |

### Key Findings

- **CORS is missing from both GamePanel components.** Wings has `AllowedOrigins` config, PufferPanel uses `gin-contrib/cors`, and PHP panels use Laravel middleware. This must be addressed before production.
- **Rate limiting is absent from GamePanel.** Wings has a dedicated `system/rate.go` with `juju/ratelimit`. Forge API's `auth_security_test.go` suggests security is on the radar but not implemented.
- **No security headers** (X-Frame-Options, X-Content-Type-Options, Strict-Transport-Security, Content-Security-Policy) are set by any Go project — these are typically handled at the reverse proxy level (nginx/Caddy).
- **GamePanel has `auth_security_test.go`** which tests auth security — this is a positive signal that security is being thought about, even if middleware is not yet implemented.
- **Wings has the most complete TLS configuration** — configurable certificate file, key file, SSL port, and remote download disabling.
- **No Go project implements CSRF protection** — this is acceptable for pure API services but matters if serving browser-based UI.

### Gap: CORS and Rate Limiting

Both are critical for production:
1. **CORS middleware** — Add to Forge API (GoFiber has built-in CORS middleware)
2. **Rate limiting** — Add to both Beacon and Forge API, especially on auth endpoints. Wings' `system/rate.go` is a good reference pattern.

---

## 9. WebSocket Implementation

### WebSocket Comparison

| Aspect | GamePanel | Wings | PufferPanel | Pelican Panel | Pterodactyl Panel |
|---|---|---|---|---|---|
| **Library** | gorilla/websocket (Beacon), gofiber/contrib (Forge API) | gorilla/websocket | gorilla/websocket | Laravel Broadcasting (Pusher) | Laravel Broadcasting (Pusher) |
| **Files** | Beacon: server/console.go, Forge: plans only | 7 files in `router/websocket/` | In HTTP handlers | `WebsocketControllerTest.php` exists | `WebsocketControllerTest.php` exists |
| **Rate limiting** | Not found | `limiter.go` dedicated file | Not found | Via broadcast driver | Via broadcast driver |
| **Message format** | Not yet implemented | Custom `Message` struct | Not standardized | Pusher protocol | Pusher protocol |
| **Reconnection** | Not implemented | Event listeners | Not implemented | Pusher client handles | Pusher client handles |
| **Fanout** | Planned (`internal/realtime/`) | `listeners.go` | Not implemented | Redis broadcasting | Redis broadcasting |
| **Auth** | Ticket-based (`handlers_ws_ticket_test.go`) | JWT tokens | Not implemented | Laravel Echo auth | Laravel Echo auth |
| **Documentation** | Realtime README (3 lines) | No docs | No docs | No docs | No docs |

### Key Findings

- **GamePanel's WebSocket layer is planned but not implemented** — the `internal/realtime/` directory contains only a README stating "Redis pub/sub, WebSocket fanout, rate-limit counters, and cleanup rules." The test file `handlers_ws_ticket_test.go` suggests a ticket-based auth design is in progress.
- **Wings has the most mature WebSocket implementation** — 7 files covering connection management, message handling, rate limiting, listeners, and token auth. This is the reference architecture for GamePanel.
- **Both PHP panels use Laravel Broadcasting** with Pusher protocol — a push-based model where the panel pushes events to clients via Redis. GamePanel's Forge API plans to use a similar Redis pub/sub model.
- **Wings' WebSocket has a dedicated rate limiter** (`limiter.go`) — GamePanel's realtime layer should include this from the start.
- **PufferPanel has no WebSocket implementation** — it relies on HTTP polling for real-time updates.

### Gap: WebSocket Not Yet Implemented

The realtime layer is the biggest missing piece in GamePanel. Based on Wings' architecture, GamePanel should implement:
1. WebSocket upgrade handler with ticket-based auth (already designed)
2. Rate limiting per connection
3. Message serialization format
4. Reconnection handling
5. Redis pub/sub fanout for multi-node scenarios

---

## 10. Database Migration Strategy

### Migration Comparison

| Aspect | GamePanel (Forge API) | Wings | PufferPanel | Pelican Panel | Pterodactyl Panel |
|---|---|---|---|---|---|
| **Migration tool** | Raw SQL files | None (no DB) | gormigrate (GORM) | Laravel Migrations | Laravel Migrations |
| **Format** | Numbered `.sql` files (001-044) | N/A | Go code via gormigrate | PHP `Schema::create/table` | PHP `Schema::create/table` |
| **Count** | 44 migrations | 0 | Auto-migrated via GORM | 242 migrations | 195 migrations |
| **Runner** | Custom migration service + CI validation | N/A | `db upgrade` CLI command | `php artisan migrate` | `php artisan migrate` |
| **Rollback** | Not implemented | N/A | Not implemented | Yes (standard Laravel) | Yes (standard Laravel) |
| **Version tracking** | SQL file numbering | N/A | gormigrate version tracking | Laravel migration table | Laravel migration table |
| **CI validation** | `validate-api-migrations.sh` | N/A | Run in Dockerfile build | No | No |
| **Database** | PostgreSQL only | None | SQLite/MySQL/PostgreSQL/SQLServer | MySQL/MariaDB | MySQL/MariaDB |

### Key Findings

- **GamePanel has the most modern migration approach** — raw numbered SQL files (001-044) with a CI validation step. This is more transparent than ORM-generated migrations and avoids vendor lock-in.
- **The 44 GamePanel migrations** cover a comprehensive feature set: server lifecycle, transfers, schedules, databases, backups, regions, evacuation, provisioning, settings, OAuth2, plugins, webhooks, and async delivery.
- **CI validation of migrations** (`validate-api-migrations.sh`) is unique to GamePanel — no other project validates migrations in CI. This prevents broken migrations from reaching production.
- **PufferPanel uses GORM's AutoMigrate** via `gormigrate` — convenient but less controlled than explicit SQL.
- **Wings has no database** — it stores state in files and relies on the panel for persistence.
- **Laravel panels have 195-242 migrations** accumulated over 10+ years of development. Pelican has added ~47 new migrations beyond Pterodactyl's base.
- **No Go project implements rollback** — this is a gap shared by all Go projects.

### Gap: No Rollback Support

GamePanel's migration service should support rollback for failed migrations. Currently, if migration 045 fails, there's no way to cleanly revert. Consider adding a `_down.sql` file for each migration or a transactional migration wrapper.

---

## 11. Cross-Cutting Insights

### Strengths of GamePanel

1. **Test density is best-in-class** — 82 test files across a small codebase, including security-specific tests that no reference project has.
2. **Lean dependency footprint** — 12/9 direct deps for Beacon/Forge API vs 52/58 for Wings/PufferPanel.
3. **Modern tech choices** — PostgreSQL (vs SQLite), GoFiber (vs Gin), Go 1.26 (vs 1.24-1.25).
4. **CI migration validation** — Unique among all projects.
5. **Security-aware design** — `auth_security_test.go`, `handlers_ws_ticket_test.go`, ticket-based WebSocket auth.
6. **Architecture documentation** — Extensive docs/ folder with decision records and planning.

### Weaknesses of GamePanel

1. **No panic recovery** in HTTP middleware — process-crashing vulnerability.
2. **No CORS middleware** — browser clients will be blocked.
3. **No rate limiting** — vulnerable to abuse.
4. **WebSocket layer not implemented** — only planned.
5. **No health checks** — Docker and monitoring gaps.
6. **Missing open-source docs** — LICENSE, CONTRIBUTING, SECURITY files.
7. **Root user in Beacon Docker** — container security gap.
8. **Framework inconsistency** — net/http (Beacon) vs GoFiber (Forge API).

### Patterns Worth Adopting from Reference Projects

| Pattern | Source | Priority |
|---|---|---|
| Panic recovery middleware | Wings router | **CRITICAL** |
| CORS middleware | PufferPanel `gin-contrib/cors` | **HIGH** |
| Rate limiting | Wings `system/rate.go` | **HIGH** |
| Docker HEALTHCHECK | Pelican Panel | **MEDIUM** |
| Non-root Docker user | Wings distroless | **MEDIUM** |
| Swagger API docs | PufferPanel `swaggo` | **MEDIUM** |
| Security headers middleware | Any (custom) | **MEDIUM** |
| TLS configuration | Wings config | **MEDIUM** |
| Viper config with env prefix | PufferPanel | **LOW** |
| DEB/RPM packaging | PufferPanel | **LOW** |

### Maturity Ranking

1. **Pterodactyl Panel** — Most mature (10+ years), largest community, but aging codebase
2. **Pelican Panel** — Most modern PHP panel, good Docker/CI, growing fast
3. **PufferPanel** — Most packaging maturity (DEB/RPM), best config management
4. **GamePanel** — Best test coverage, leanest deps, but missing production essentials
5. **Wings** — Best WebSocket implementation, but no CI, poor test coverage

---

*Report generated by Agent 6 — Cross-Reference General Lookup*
*Methodology: File-by-file audit + grep-based pattern matching across all 5 projects*
