# 110 Phase 01 — Subagent 01 — API Core Preparation Audit

**Slice:** Forge API Core — `forge/api` auth, store, migrations, config, middleware, daemon client, eventstore, placement
**Date:** 2026-08-24
**Auditor:** 110-01-01
**Mode:** Audit only — no product code modified
**Evidence:** Live file:line inspection at HEAD + `audits/FINAL_PARITY_AUDIT.md` + `audits/reverification/synthesis.md` + `audits/MASTER_FINDING_INDEX.md`

---

## 1. Current File Inventory (key files:line)

### 1.1 Top-level wiring & module

| File | Lines | Role | Notable symbols |
|---|---|---|---|
| `forge/api/go.mod:1` | 195 | Module `gamepanel/forge`, go 1.26, fiber v2, pgx v5, redis v9, jwt v5 | Requires `github.com/gofiber/fiber/v2:18`, `github.com/jackc/pgx/v5:22`, `github.com/redis/go-redis/v9:26` |
| `forge/api/cmd/api/main.go:113` | 2063 | Single process wiring: `run():120`, healthcheck `healthcheck():1754`, `masterKeyringFromEnvironment():1813`, all service constructors `344-1200`, HTTP `NewServer` + listener `1578-1591`, `shutdownServices():1603` | `readinessHealthPath:111="/api/v1/health/ready"`, `healthcheck("http://127.0.0.1"+healthcheckPort(env("API_ADDR"))+path):122`, `daemon.NewClient:244`, `eventstore.NewRelay:370`, `placement.NewEngine:373` |
| `forge/api/cmd/api/main_test.go:1` | — | Startup arg parsing tests (`--healthcheck`) | |
| `forge/api/cmd/api/river.go:13` | — | Deprecated vendored River fork — dead code per `reverification/synthesis.md:17` | |
| `forge/api/config/app.go:1`, `auth.go:1`, `database.go:1`, `helpers.go:1`, `mail.go:1`, `mtls.go:1`, `runtime.go:1`, `services.go:1` | — | Legacy `forge/config` package (separate from `internal/config`); `mtls.go` + `mtls_test.go` validate mTLS | |

### 1.2 Config (`forge/api/internal/config/`)

| File | Lines | Key contents |
|---|---|---|
| `internal/config/config.go:1` | 214 | `Manager:12`, `Config:16` (App, Server, DB, Redis, Auth, Mail, Daemon, Backup, Log), `defaults():101`, `NewManager():145` (viper, env binding `169-183`), `Validate():205` (minimal — full check is `validator.go`) |
| `internal/config/env.go:1` | 128 | `FromEnv():10` flat env reader, helpers `env():77`, `envInt():84`, `envBool():96`, `envDuration():108` |
| `internal/config/validator.go:1` | 120 | `ValidateAll():35` — production guards: `auth.secret:39`, `app.key:41`, `len>=32:44`, `redis.password:50`, `db TLS sslmode:53`, `DAEMON_ALLOW_INSECURE_NO_AUTH:56`, `MTLS_DEV_BYPASS:65`, `BCRYPT_COST>=10:73`, `server.addr:80`, `db.url:83`, `token_ttl>0:86` |
| `internal/config/config_test.go:1` | — | Validates `ValidateAll` error fields (`auth.secret:39`, `redis.password:179`) |
| `internal/config/mtls.go:1`, `mtls_test.go:1` | — | `MTLSConfig()`, `ValidateMTLS()` used at `cmd/api/main.go:1114-1123` |

**Observation:** Two config systems coexist: legacy `forge/config` (used in `cmd/api/main.go:26,1114`) and `internal/config` (Manager + FromEnv). `cmd/api/main.go:1311-1718` builds `config.Config` manually via `env()` helpers, **not** via `internal/config.NewManager()` — the Manager path is unused at startup but is the testable/validated path. Drift risk: `cmd/api/main.go:validateConfig:1877` calls `cfg.ValidateAll()` on the legacy struct, while `internal/config/config.go:205` has a weaker `Validate()`. Only one should be canonical.

### 1.3 Store

| File | Lines | Key contents |
|---|---|---|
| `internal/store/store.go:1` | ~1950+ | Core types: `Store:22{db *pgxpool.Pool, secrets *Keyring}`, `migrationAdvisoryLockID:39`, `acquireMigrationLock():45`, all domain DTOs (`User:61`, `Node:132`, `Server:446`, `Schedule:830`, `Backup:893`), `ConnectWithPoolConfig():1158` (20 retries, 500ms), `runMigrations():1226` (sort, `validateNoDuplicatePrefixes`, pg_advisory_lock, tx per file), `Rollback():1284`, `Seed():1349` (demo_seeded guard, production guard `1366`, localhost guard `1373`, random password `1399`, tx `1419`) |
| `internal/store/migration.go:1` | 319 | `MigrationRunner:12`, `Run():24` dialect override (`32-62`), `validateNoDuplicatePrefixes:271`, DDL helpers `getCreateMigrationTableSQL:288`/`getRecordMigrationSQL:308` |
| `internal/store/seeder.go:1` | 92 | `Seeder:12`, `DefaultSeeder():38` seed `roles:41` + `settings:64` via `ON CONFLICT DO NOTHING` |
| `internal/store/permissions.go:1` | — | `AdminScopes`, `ValidateApiKeyScopes`, `HasAdminScope` — consumed by `http/auth.go:513-522` |
| `internal/store/repositories.go:1` / `repository_impl.go:1` / `database.go:1` / `driver_*.go:1` / `schema_versions.go:1` | — | Repository abstractions, multi-driver support (postgres/mysql/sqlite) |
| `internal/store/migration_*.go` + `*_test.go` | — | `migration_comprehensive_test.go`, `migration_duplicate_test.go`, `migration_runner_unified_test.go` |
| `internal/store/store_*.go` (120+ files) | — | One file per domain: `store_users.go` (subuser wildcard at `564`), `store_servers*.go` (5+ files, `store_servers.go:253` lifecycle, `store_servers_control.go:95` config), `store_allocations.go`, `store_mounts_ext.go:323` (allowlist), `store_backups.go:240` (prune without advisory lock), `store_deployments.go`, `store_egg_variables.go:176` (regex slash), etc. |
| `forge/api/migrations/*.sql` | 201 files | Numbered `001_init.sql` … `112_cron_jobs.sql` with letter-suffix branches (`041_a_placement_intents.sql`, `024_a_sftp_config.sql`). `084_create_missing_tables.sql` is catch-all. Historically prefixes validated by `store/migration.go:271`. |
| `internal/store/seeder.go:38` + `migrations/091_seed_minecraft_java.sql` | — | Only 1 seeded egg vs 14 on FS (`egg-templates.ts:27`) + `gamepanel/packages/game-templates/` — seeding deficit `audits/FINAL_PARITY_AUDIT.md:DB-09` |

**Store migration runner details (`store.go:1226-1280`):**
- Session advisory lock `pg_advisory_lock(0x466F7267656D6967)` per `store.go:45` — correct single-connection lock, `context.WithoutCancel` on unlock `55`.
- Per-migration `BEGIN` `1261`, per-statement `tx.Exec` `1266`, `INSERT schema_migrations` `1271`, `COMMIT` `1275`.
- `Rollback()` `1284` requires matching `*.down.sql` under `rollbacksDir` — observed zero `.down.sql` files in `migrations/`; rollback path is non-functional unless rollbacks are maintained separately.
- Multi-statement split via `splitSQLStatements` — risk: `DO $$`, `CREATE FUNCTION`, `COMMENT ON` not tested in `store.go:1265` (handled only in `migration.go:110` SQLite path).

### 1.4 Auth (`forge/api/internal/auth/`)

| File | Lines | Key contents |
|---|---|---|
| `internal/auth/session.go:1` | 306 | `Session:14`, `SessionStore` interface `25`, `InMemorySessionStore:36{mu sync.RWMutex, sessions map, byToken map, byUser map}`, `Create:51`, `Get:68` (expiry check), `GetByToken:82` (raw `byToken[rawToken]` `86`), `Update:100` (overwrites `sessions[ID]=session:111` with raw pointer, no expiry or token-index update), `Delete:115` (delete `byToken[Token]:124`), `DeleteByUser:142`, `Cleanup:157`, `ListByUser:182`, `SessionMiddleware:231` (`GetByToken` then `sess.LastActiveAt=Now():261` then `store.Update():262`), `ValidateSessionIP:304` |
| `internal/auth/session_postgres.go:1` | — | Postgres-backed `PostgresSessionStore` used at `cmd/api/main.go:319{sessionStore *auth.PostgresSessionStore}` but **nil when `db==nil`** (dev mode): `server.go:231-253` creates fallback in-memory store per request — see Finding SC-02 |
| `internal/auth/remote_hmac.go:1` | 230 | Dual-auth comment `16-26` (mTLS panel→beacon, HMAC beacon→panel), `NonceStore:44{mu sync.Mutex, seen map, redisClient}`, `Accept:72` (Redis `SETNX` `92` + in-memory fallback + bounded `maxRemoteNonces=4096:36`), `SignHMAC:153` (`method\nURI\ntimestamp\nnonce\nbody`), `VerifyRemoteHMAC:170` (5-min skew `175`, hex nonce 16 bytes `180`, `hmac.Equal:194`, `Accept:197` with `claimed_until`), `VerifyRemoteHMACRaw:205` |
| `internal/auth/scopes.go:1` | 159 | `Scope:12`, `BuiltinScopes:20` (22 scopes including `admin:read:31`, `admin:write:32`, `node:read:33` etc.), `KnownScopes:49`, `ParseScopes:62` (space/comma split, unknown → error `76`), `Has/HasAny/HasAll:84-107`, `DefaultScopes:137` (IsDefault true) |
| `internal/auth/scopes_extended.go:1` | — | Extended scope helpers (checked at `scopes_extended_test.go`) |
| `internal/auth/session_test.go:1`, `session_integration_test.go:1`, `remote_hmac_test.go:1` | — | Session token generation (`GenerateSessionToken:196` 32 bytes rawURLEncoding, `GenerateSessionID:204` 16 bytes) |

**Wiring in `http/auth.go:258-382`:** `authMiddleware(secret, st)` → `authMiddlewareWithStore` reads session cookie `188` first (`getSessionCookie:187`), then bearer `314-318` (`Bearer ` vs `X-API-Key`), tries `parseToken:333` → `validateCurrentSession:336` (checks `IsJWTRevoked:400` + `IsUserSessionRevoked:411` + `GetUserByID:420` + `SessionVersion:424`), then OAuth `350-368`, then `ValidateApiKey:371`. Scoped admin routes re-checked at `requireRole:430` (`hasAnyAdminScope:513`).

### 1.5 HTTP Middleware (`forge/api/internal/http/middleware_*.go`)

| File | Lines | Key behavior |
|---|---|---|
| `middleware_cors.go:1` | 100 | `CORSMiddleware:34` — compiled from `CORSConfig:10`, `DefaultCORSConfig:19` (localhost:3000/3002), `originAllowed:68` exact match only (`== origin`), no wildcard when `AllowCredentials:82` panics, registered **first** at `server.go:1091` |
| `middleware_csrf.go:1` | — | `CSRFMiddleware` + `server.go:196` `LoadSessionCookieConfig` |
| `middleware_ratelimit.go:1` | 307 | `RateLimitConfig:19`, `memBucket:34` + `memRateLimiter:39` (Map+Mutex), `globalMemLimiter:44` + ticker cleanup `46-58`, `ExtractClientIP:98` (trust XFF only if `peer.IsLoopback||IsPrivate||IsUnspecified:101`, right-most valid IP `108`), `RateLimiter:142` (Redis `tryRedis:212` `INCR`+`EXPIRE` else in-memory fallback gated on `REDIS_ADDR==""` `169` + production fail-closed `174`), `GetRateLimitForEndpoint:242` (`auth:5/m`, `mutation:30/m`, `read:120/m`) |
| `middleware_mtls.go:1` | 220 | `MTLSAuthMiddleware:77` — DevBypass panic in prod `82,88`, disabled/no-op `93`, missing revocation lookup → `503:104`, CA pool `109`, require `https` (`c.Protocol()!=https && X-Forwarded-Proto!="https":123`), `tls.Conn:132`, peer cert chain verify `145-158` + `verifyMTLSRevocation:159`, identity `forge-node` URI → DNSNames → CN `169-179` |
| `middleware_security.go:1` | 62 | `SecurityHeaders(env):27` — `generateNonce:19` (16 bytes rand → base64 else `"fallback-nonce":22`), CSP `nonce+strict-dynamic` `32`, `X-Frame-Options DENY:45`, HSTS only if `production:56` |
| `middleware_security_headers.go:1` | 93 | `SecurityHeadersMiddleware(cfg):48` — configurable toggles `12-22`, `DefaultSecurityHeadersConfig:25` (CSP with `{NONCE}` placeholder `34`), `generateMiddlewareNonce:40` (same fallback-nonce), `{NONCE}` replace `71`, `X-CSP-Nonce:81` |
| `middleware_maintenance.go:1` | — | `MaintenanceModeMiddleware` tied to `FORGE_MAINTENANCE_MODE`/`config` |
| `middleware_logger.go:1` | — | `StructuredLogger(cfg.Logger)` at `server.go:1101` |
| `middleware_metrics.go:1` | — | `MetricsMiddleware(NewMetricsCollector())` at `server.go:1110`, redis/queue/health metrics |
| `middleware_requestid.go:1` | — | Request ID injection |
| `middleware_ipaccess.go:1` | — | `IPAccessControl(AdminIPAccessConfig)` + `APIIPAccessConfig` wired at `server.go:1130` before `v1.Group` |
| `middleware_i18n.go:1`, `middleware_request_logging.go:1` | — | i18n translator, request logging (with tests) |

**Registration order (`server.go:1063-1159`):** `fiberrecover` `1063` → `configLocalKey` `1068` → `CORS` `1091` → `Maintenance` `1094` → `SecurityHeaders` `1098` → `StructuredLogger` `1101` → `Metrics` `1110` → swagger/well-known/i18n → `RateLimiter` instances `1124` → `IPAccessControl`+mTLS `1160` → `v1.Group` `1160`. Notable: CORS and SecurityHeaders are **global** (outside `/api/v1`), rate/mTLS are **per-`v1`** but CSP nonce still on unprotected routes (correct).

### 1.6 Daemon Client (`forge/api/internal/daemon/client.go`)

| Segment | Lines | Key contents |
|---|---|---|
| Constants/backoff | `27-38` | `maxRetries=3:28`, `retryableStatuses:29`, `baseBackoff=500ms, maxBackoff=30s:35-36` (vars for test shrink), `ErrMissingNodeToken:39` |
| Context key + helpers | `41-102` | `commandIDContextKey:41`, `ContextWithCommandID:43`, `isRetryableStatus:53`, `isIdempotentMethod:62` (GET/HEAD/PUT/DELETE/OPTIONS/TRACE), `hasIdempotencyKey:71` (`X-Forge-Command-ID`/`Idempotency-Key`), `isRetryableRequest:91` (idempotent OR keyed), `jitter:105` (±25% via `rand.Int`) |
| `Client` struct + ctor | `117-291` | `Client:117{httpClient, defaultBaseURL, defaultNodeToken, loopback}`, `NewClient(baseURL, nodeToken):248` (empty both → placeholder client `251`, token empty → `ErrMissingNodeToken:260`, loopback detection `isLoopback:293` + `isLoopbackHost:306`, force `https` if not loopback `265`, `daemonTransport:315` TLS12 + optional mTLS `daemonMTLSConfig:340` + key perm check `363` + `GetClientCertificate` hot reload `368`), `daemonMTLSConfig:340` (CA pool, key 0600 check, `Certificates` preload) |
| Retry transport | `179-246` | `retryRoundTripper:183{base, resign:Resign func}`, `RoundTrip:188` (buffer body `195`, jitter backoff `203`, recreate body `214`, `resign:219` re-sign nonce, `base.RoundTrip:225`, retry on idempotent/keyed+5xx `226-237`, last status fmt `245`) |
| Signing | `1324-1391` | `requestSigningKey:1326`, `SignedHeaders:1333` (`newSignatureParts:1367` time RFC3339 + 16-byte hex nonce), `resignRequest:1348` (fresh timestamp/nonce, `signedHeaders:1360` with `sign()`), `signedHeaders:1375` sets `X-Panel-Timestamp/Nonce/Signature` + `X-Beacon-Version` |
| Power/install/create | `384-768` | `PowerResponse:384`, `CreateRequest:392{Provider string:418}`, `SendPower:832` (adds `X-Forge-Command-ID:843` if present), `CreateServer:731` (`create:ServerID:737` commandID), `InstallServer:689`/`ReinstallServer:693` (`runInstaller:697` with `install:ServerID:704` fallback) |
| Files/backups/transfers | `769-1532` | `BackupEntry:554`, `Transfer* 514-553`, `CreateBackup:910` (`backup:create:ServerID:name:931` key), `RestoreBackup:994` (`backup:restore:`), `PullRemoteFile:1135` (HTTPS-only `1138`, private/loopback reject `1144`, target traversal guard `1147-1152`, JSON to `/files/pull:1158`), plus `ArchiveFiles:1035`, `DeleteFiles:1224`, `HostFiles*:1397`, `WebSocketURL:1313` (http→ws, https→wss) |

**Daemon client maturity notes:**
- Loopback detection uses both `isLoopback(rawURL)` (via `net.ParseIP.IsLoopback`) and `isLoopbackHost` string forms — consistent, no `IsPrivate` confusion.
- `daemonMTLSConfig:362-365` permission check `Perm&0o077!=0` matches `sdk-go mtls.go` host hardening — good.
- `NewClient("", "") → placeholder` `251` allows dev without token (`main.go:246-250` creates `dev-placeholder`) but the placeholder client has empty `defaultBaseURL` and no TLS; callers must always supply explicit `(baseURL, nodeToken)` per `Send*` — dev-mode `ServerControlTarget` still routes via real node credential.
- 15-minute `http.Client.Timeout:254,287` matches long installs (`execution.go:78 timeout` + `main.go:579-601` 15m) — aligned.

### 1.7 Eventstore (`forge/api/internal/eventstore/`)

| File | Lines | Key contents |
|---|---|---|
| `store.go:1` | 237 | `StoredEvent:49`, `ClaimPending:67` (`FOR UPDATE SKIP LOCKED` `71-78`, `claimed_by=$3||:id` `75`), `Publish:113` (`INSERT events 125` dispatched=false), `Pending:135`, `MarkDispatched:157`, `MarkFailed:165`, `MoveToDeadLetter:175`, `Prune:191`, `OutboxPublisher:205{store, registry}`, `Publish:214` (store then `registry.Publish:218` else `"persisted but in-memory delivery failed"`), `Count:224` |
| `outbox.go:1` | 219 | `Relay:18{store, pollInterval, subscribers, maxRetries=3:34, eventTimeout=30s:35}`, `NewRelay:30(5s)`, `Subscribe:39` (append to `subscribers`), `Start:45` (once + `WithCancel` + `pollLoop`), `pollLoop:74` (ticker `pollInterval` → `processBatch:92` + hourly `Prune:95` 7d/30d), `processBatch:102` (`ClaimPending` `105` lease `11*30s=330s` + `uuid.NewString()` token), `processEvent:124` (`WithTimeout 30s:125`, `json.Unmarshal payload:130`, deliver `174`, `MarkDispatched` retry 3× 100ms backoff `161-170`), `deliverWithRetries:174` (per-handler `maxRetries=3` exp backoff 100ms×2) |
| `migration.go:1` | — | `Migrate(*sql.DB)` DDL for `events`, `events_dead_letter` — called at `main.go:194` before operational-secret migration |
| `store_test.go:1` | — | Unit tests for claim/publish |

**Wiring (`main.go:364-371`):** `es := eventstore.New(db.GetDB())` → `eventRelay := eventstore.NewRelay(es, 5s)` → `outboxPub := eventstore.NewOutboxPublisher(es, eventRegistry)`. Relay started at `1167`, `eventRegistry.Subscribe(events.WildcardEventType, obs/whSvc)` at `1076-1077`, failover `946`, trafficmanager `1026-1067` etc. **No `Subscribe` on `Relay` in production** — `Relay.Subscribe` `outbox.go:39` has zero prod callers; `EventRelay` consumers read via `eventRegistry` (in-memory) not via relay replay. See Finding EC-01.

### 1.8 Placement (`forge/api/internal/placement/`)

| File | Lines | Key contents |
|---|---|---|
| `engine.go:1` | 112 | `Engine:14{scorer, checker, logger, mu sync.Mutex:18}`, `NewEngine:21`, `Place:37` (`mu.Lock:38`, `FilterByConstraints:40`, `Score:52`, `CheckSoft:56`, `score+bonus:58`, best> `67`), `PlaceAll:79` (sorted desc `107`) |
| `constraints.go:1` | 193 | `ConstraintType:9` (affinity, anti-affinity, region, node, label), `Constraint:19{Type, Operator, Key, Values, Required}`, `ConstraintContext:27{ServerNodeMap, NodeLabels}`, `CheckHard:38` (required only), `CheckSoft:50` (`+1e12` / `-1e10:59,62` — dwarf bug), `checkSingle:69` dispatch, `checkAffinity:86`/`checkAntiAffinity:99`/`checkRegion:112`/`checkNode:130`/`checkLabel:148`, `FilterByConstraints:181` |
| `strategy.go:1` | — | `Scorer`, `Candidate`, `WorkloadRequest`, `ScoreResult`, `StrategyLeastLoaded` etc. (used at `main.go:373`) |
| `replica.go:1` | — | Replica placement extensions (used by `replicamanager`) |
| `explain.go:1`, `strategy_test.go:1` etc. | — | Test explainers |

**Wiring (`main.go:373-392`):** `placement.NewEngine(placement.NewScorer(StrategyLeastLoaded), placement.NewConstraintChecker())` → `scheduler.New(db, placeEngine, outboxPub)` with predictive/constraint/reservations adapters. `Engine.mu` serializes all `Place` calls (Finding PL-03).

---

## 2. Known Findings in This Slice (P0/P1 that touch API core)

Findings are sourced from `audits/FINAL_PARITY_AUDIT.md:§10-11, §13-15` and `audits/reverification/synthesis.md:§2`. Only findings whose **remediation must modify files inside this slice** (or whose root cause lives there) are listed.

### 2.1 Authentication & Session (touches `internal/auth/*`, `internal/http/auth*.go`, `internal/store/store_users*.go`, `internal/store/store_apikeys*.go`, middleware)

| ID | Title | Severity | Files:line | Status in reverification |
|---|---|---|---|---|
| **GH-18 / SE-01+02** | Subuser wildcard `*` persistable by any `user.create` holder + missing subset-check (`Upsert` grants arbitrary scopes). Escalation `user → * → full control` + `database.view_password` | **P0** | `store/store_users.go:297` (`UpsertServerSubuser:297` + `normalizeSubuserPermissions:563` allowlists `*` `564`), `store/permissions.go` (`ValidateApiKeyScopes` scope gate), `http/handlers_servers.go:554` (subuser route `Upsert`) | Still BROKEN (`reverification/synthesis.md:51` + subagent-04 F-01) |
| **SE-12** | Session store pointer race + raw token keys + raw `byToken` map. `InMemorySessionStore.byToken[rawToken]:39,86` stores raw token hash-keyed; `GetByToken:82` returns `*Session` raw pointer → caller mutates `sess.LastActiveAt=Now()` at `auth/session.go:261` + `http/server.go:261` while `RWMutex` is released → data race. Concurrent `LastActiveAt` write collides with `Get` readers. + token-map leak (no hashing). | **P1** | `auth/session.go:36-98`, `auth/session.go:261-262` (`LastActiveAt` race), `auth/session_postgres.go` (postgres path avoids race but shares token shape) | Still BROKEN (`FINAL_PARITY_AUDIT.md:§11 SE-12`, reverification-20) |
| **SE-crumb** | Dual session stores drift: `InMemorySessionStore` (`auth/session.go:36`) vs `PostgresSessionStore` (`auth/session_postgres.go`) + `auth/session_integration_test.go` — two implementations with divergent expiry, indexing, and token-hash semantics. `cmd/api/main.go:319` only constructs postgres; dev `db==nil` path in `http/server.go:990-1023` constructs in-memory without Redis, unsafe for multi-replica | P1 | `auth/session.go:36`, `auth/session_postgres.go`, `cmd/api/main.go:319`, `http/server.go:992` | Open |
| **SE-13** | mTLS HTTPS gate trusts client X-Forwarded-Proto (`middleware_mtls.go:123` `c.Get("X-Forwarded-Proto")!="https"` bypass). Any private peer can inject XFP and suppress the HTTPS requirement, downgrading mTLS-protected routes to plain HTTP behind a proxy | P2 | `http/middleware_mtls.go:123` | Still BROKEN (reverification-20) |
| **SE-14** | WS origin missing for Bearer auth (`ws_origin.go:95`). `origin` not validated when `!isCookieAuth`, allows cross-origin WebSocket hijack with bearer token | P2 | `http/handlers_notifications_websocket_test.go`, `http/ws_origin.go:95` (implied) | Open |
| **AUTH-01** | `SessionMiddleware` does `store.Update` per-request (`auth/session.go:261-262`) inside cookie path **and** `http/server.go:261` JWT path similarly — write amplification + no conditional on dirty; every authenticated request writes to DB. Postgres path must throttle or use `UPDATE ... SET last_active=CASE WHEN now - last_active > interval THEN now` | P1 perf/correctness | `auth/session.go:261`, `http/server.go:261` | Open |
| **AUTH-02** | `authMiddlewareWithStore:271` tries session cookie first then raw JWT bearer `333`, then OAuth `350`, then API key `371`. Order is correct but `parseToken` uses `WithLeeway 30s` `80` and `IssuedAt` check — import `auth/session.go` IAT binding not enforced for `issue2FAConfirmationToken:631` (separate issuer `type=2fa_confirmation`) | P2 | `http/auth.go:73-114`, `http/auth.go:631` | Open |
| **AUTH-03** | `http/auth.go:298-303` admin cookie path synthesizes `apiScopes=["*"]` for `RoleAdmin`; non-admin gets `[]`. API-key/OAuth paths go through `ValidateApiKeyScopes`. The admin `*` synthesis must survive the store-layer scope check `hasAnyAdminScope:513` — currently does (explicit `scope=="*":515` short-circuit). No bug but fragile coupling | Info | `http/auth.go:299`, `http/auth.go:513` | Track |

### 2.2 Middleware & Edge (touches `internal/http/middleware_*.go`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **SE-07** | Trusted proxy fail-open: `ExtractClientIP:98` trusts `X-Forwarded-For` from any `IsPrivate`/`IsLoopback`/`IsUnspecified` peer (1.1: expands to every private container), wrong vantage (panel not source node). Any container can rotate identity → bypass rate limits. Reference: Caddy explicit CIDR allowlist | **P1** | `http/middleware_ratelimit.go:98-119` | Still BROKEN (reverification-20) |
| **SE-08** | CSP nonce fallback `fallback-nonce` with `strict-dynamic` (when `rand.Read` fails). Attacker can inject `'nonce-fallback-nonce'` → `strict-dynamic` executes it. Two identical generators `middleware_security.go:19` + `middleware_security_headers.go:40` | P2 | `http/middleware_security.go:22`, `http/middleware_security_headers.go:43` | Still BROKEN (reverification-20) |
| **SE-09/NG-10** | All traffic policies apply to all routes — cross-tenant ACL. Not in this slice's middleware but edge is `IPAccessControl` scoping at `server.go:1130` — wired but `AdminIPAccessConfig`/`APIIPAccessConfig` are env-only, no per-route attachment join | P0 (adjacent) | `http/server.go:1130`, `store/store_routing_rules.go` (if exists) | Open (not fixable inside API-core alone) |
| **MW-01** | Dual CSP middleware: `SecurityHeaders` (`middleware_security.go:27`) **and** `SecurityHeadersMiddleware` (`middleware_security_headers.go:48`) — two trees manually kept in sync (`reverification/synthesis.md:§3 SE-07`). `server.go:1098` uses only the first; second is dead except tests — duplicate | P2 | `http/middleware_security.go:1`, `http/middleware_security_headers.go:1`, `http/server.go:1098` | Open (deduplicate) |
| **MW-02** | `RateLimiter` per-endpoint `GetRateLimitForEndpoint:242` is **instance-local** when `REDIS_ADDR==""` (`server.go:1123` `requireSharedRateLimiter` only when `RedisEnabled && production`). Dev/test runs with in-memory `globalMemLimiter:44` — cross-replica bypass. `tryRedis:212` does `INCR`+`EXPIRE` non-atomically (TOCTOU window) but acceptable for coarse limits | P1 | `http/middleware_ratelimit.go:44,142,212`, `http/server.go:1123` | Open |
| **MW-03** | `ExtractClientIP` uses right-most XFF (`108-112`) but also trusts `X-Real-IP` (`114-117`) unconditionally after peer check — same private-peer expansion as XFF. Must be behind explicit `TRUSTED_PROXY_CIDRS` not `IsPrivate` | P1 | `http/middleware_ratelimit.go:114` | Open |
| **MW-04** | CORS exact-match only (`originAllowed:68` `== origin`). Correct for security but `DefaultCORSConfig:19` hardcodes localhost:3000/3002 only; production must set `API_CORS_ALLOWED_ORIGINS` else `server.go:1086` warns and falls back to localhost — blocks remote panel origins until env set (fail-closed is correct, just operationally surprising) | P3 | `http/middleware_cors.go:19,68`, `http/server.go:1077` | By design |

### 2.3 Store & Migrations (touches `internal/store/*`, `migrations/*.sql`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **GH-14** | Variable validation regex slash bug: `regexp.Compile(arg)` at `store_egg_variables.go:176` includes surrounding `/` delimiters, literal `\|` inside char class — blocks all PTDL imports (e.g., `minecraft-paper.json:58`) | **P0** | `store/store_egg_variables.go:176` | Still BROKEN (reverification-02 F-01) |
| **GH-19/SE-04** | Mount allowlist too narrow: `store_mounts_ext.go:323` only blocks 2 sources, misses `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/` — host breakout | **P0** | `store/store_mounts_ext.go:323` | Still BROKEN (reverification-03/20) |
| **DB-01** | `dbprovisioner/service.go:373` global `RegisterTLSConfig` leak/race + `store_db_containers.go:174,200` list omits encrypted cols → blank fleet view | **P1** | `internal/services/dbprovisioner/service.go:373`, `store/store_db_containers.go:174` | Open (outside core but store-layer) |
| **DB-02** | Game template seeding deficit: 93% (1 seeded `091_seed` vs 14 on disk `egg-templates.ts:27` vs 44 PufferPanel) — `DefaultSeeder:38` only seeds `roles`+`settings`, never eggs | **P1** | `store/seeder.go:38`, `migrations/091_seed_minecraft_java.sql` | Still BROKEN (reverification-13) |
| **BK-07** | Prune without advisory lock: `store_backups.go:240` SQL-only delete races across replicas, no `pg_advisory_lock` | P1 | `store/store_backups.go:240` | Open |
| **SEC-DB** | `dns_credentials` / `dns_provider_accounts.credentials` plaintext JSONB (`migrations 134/135`) vs siblings encrypted via `secrets.Keyring` (`migrations 157` leaf+ACME). `store_dns.go` / `store_acme_accounts.go` read plaintext | P1 | `store/store_dns.go`, `store/store_acme_accounts.go`, `migrations/134_*.sql`/`135_*.sql` vs `157_*.sql` | Open |
| **STORE-01** | `store.go:1265` `splitSQLStatements` splits naively on `;` — breaks `DO $$ ... $$;` blocks and `CREATE FUNCTION ...; END;`. Only SQLite shim in `migration.go:110` handles `DO $$`; postgres path does not. Risk: migrations containing plpgsql will split mid-block | P2 | `store/store.go:1265`, `store/migration.go:110` | Open |
| **STORE-02** | `Rollback():1284` assumes `.down.sql` files; none exist. Any operator attempt `store.Rollback(ctx, "migrations/rollbacks", target)` returns `"rollback file not found"` — feature dead. Must either ship down-files or delete the method and document forward-only | P2 | `store/store.go:1284-1347` | Dead |

### 2.4 Daemon Client (touches `internal/daemon/client.go`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **R-01 / SE-16** | Phantom multi-runtime: control-plane advertises 7 providers (`runtime.go:9`), beacon implements 5 (`factory.go:19`), wire drops `Provider` (`beacon/server.go:740` → always `mode:docker:841`). `daemon.CreateRequest.Provider:418` set but ignored downstream — silent lie | **P0 (phantom)** | `daemon/client.go:392-419`, `internal/runtime/*.go`, `beacon/server.go:740` | Still BROKEN (reverification-19) |
| **CL-01** | `PullRemoteFile:1135-1153` HTTPS-only + private/loopback rejection is correct, but `target+fileName` check `1147` uses `filepath.Clean` + `filepath.IsAbs` — host check is filesystem-path, not URL. No rate/host denylist beyond private ranges (no SSRF allowlist for beacon→panel direction, just inverse). Consider adding `DAEMON_PULL_ALLOWLIST` | P1 | `daemon/client.go:1135` | Open |
| **D-01** | `daemonMTLSConfig:340-382` reads `MTLS_*` env per-call, caches `GetClientCertificate` hot reload — correct. But `isDaemonMTLSEnabled:328` checks `MTLS_ENABLED` only, not `DAEMON_MTLS_*`; mismatch with `beacon/internal/tls` naming (`DAEMON_MTLS_CA_FILE`). Env naming drift | P3 | `daemon/client.go:328,340`, `forge/api/config/mtls.go` | Open |
| **D-02** | `NewClient("", ""):251` placeholder has zero `httpClient.Timeout`? Actually `254: 15 min` set — fine. But `isLoopback("")` `293` returns false, so placeholder → `daemonTransport()` attempt? No — early return `252-257` constructs direct transport `http.DefaultTransport` — no TLS — fine for dev `dev-placeholder` | Info | `daemon/client.go:248-257` | Correct |

### 2.5 Config (touches `internal/config/*`, `cmd/api/main.go:120-216`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **CF-01** | Config dualism: `forge/config` (legacy used in wiring) vs `internal/config` (Manager/FromEnv). `cmd/api/main.go:1311-1718` constructs `config.Config` via raw `env()` calls, bypassing `internal/config/NewManager:145` and its `AutomaticEnv` + `BindEnv:169`. Tests use `internal/config`, prod uses ad-hoc — validation drift | P2 | `cmd/api/main.go:1311`, `internal/config/config.go:145`, `internal/config/env.go:10` | Open |
| **CF-02** | `internal/config/config.go:198` `All():194-198` falls back to `os.Getenv` for `db.url`/`auth.secret` but `NewManager:164` already bound `AutomaticEnv` — redundant, and `env.go:13` sets `Debug` as `APP_ENV != production && APP_DEBUG==true` (nonstandard double-read) | P3 | `internal/config/config.go:194`, `internal/config/env.go:13` | Open |
| **CF-03** | `validator.go:53` `isTLSDatabaseURL:96` requires exactly one `sslmode` query value `102-105`. Breaks URLs with `sslmode=require&sslrootcert=...` (two values or additional params) — valid pg TLS rejected | P2 | `internal/config/validator.go:96-111` | Open |
| **CF-04** | Production `PANEL_URL` must be HTTPS `main.go:134` (correct) but `internal/config` has no equivalent check — Manager path would allow `http://` in prod if used | P2 | `cmd/api/main.go:133`, `internal/config/validator.go:35` | Drift |
| **SEC-CF** | `masterKeyringFromEnvironment:1813` correctly rejects `CHANGE_ME:1815`, known dev keys `1823`, requires 32-byte base64 via `secrets.New`, ephemeral only when `FORGE_ALLOW_EPHEMERAL_MASTER_KEY:1832` but warns `179`. Good. No finding — keep | — | `cmd/api/main.go:1813` | Complete |

### 2.6 Eventstore (touches `internal/eventstore/*`, `internal/events/*`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **AF-1** | Write-only durability: durable outbox `eventstore/outbox.go:39` + `store.go:113` has **zero production subscribers on `Relay`** (`outbox.go:39 Subscribe:39` zero prod callers, `EventRelay` unused, `reverification/synthesis.md:17` `Relay.Subscribe zero production callers; EventRelay unused`). No `PublishTx` — events published **after** commit (`outbox.go:214` `store.Publish` then `registry.Publish:218`), so cross-instance automation not guaranteed after crash between DB commit and outbox insert | **P0 (architecture)** | `eventstore/store.go:113`, `eventstore/outbox.go:214`, `cmd/api/main.go:371` | Still BROKEN |
| **Q-02** | `OutboxPublisher.Publish:214` inserts into `events` then delivers in-memory via `registry.Publish`. If `registry.Publish` fails, caller sees error `"persisted to DB but in-memory delivery failed:218"` but event is already leased via Relay — retry will deliver again (at-least-once). Correct semantics but misleading error — callers in `main.go:554-565` ignore result for power ops | P2 | `eventstore/store.go:214` | Open |
| **Q-03** | `Relay.processEvent:124` uses `context.WithTimeout(ctx, 30s:125)` per event but `processBatch:105` leases `11*30s=330s`. If handler hangs past 30s, event is `MarkFailed:209` then retried, but lease still held 330s — tail latency under failure is 30s per handler × subscribers, lease not shortened on fail | P2 | `eventstore/outbox.go:105,125,174` | Open |
| **Q-04** | `eventstore/migration.go:Migrate` is called at `main.go:194` with `*sql.DB` from `GetDB()` — but `store.GetDB()` returns `*pgxpool.Pool` (pool) in current `store.go:22` (pgxpool, not database/sql). Check whether `Migrate` still opens a `*sql.DB` or was migrated to pgx — mismatch will panic on type assert | **P1** | `cmd/api/main.go:194`, `eventstore/migration.go:1`, `store/store.go:22` | Open — verify |

### 2.7 Placement (touches `internal/placement/*`, `cmd/api/main.go:373`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **SCH-01** | Soft bonus dwarfs base score: `CheckSoft:59 +1e12`, `62 -1e10` vs base `Score` ≤ ~3 (`constraints.go:59` + `reverification/synthesis.md:51 SCH-01 soft bonus overflow`). Constraint count dominates fit — placement is effectively random among feasible | **P1** | `placement/constraints.go:59` | Still BROKEN |
| **SCH-02** | Global mutex serializes all placement: `Engine.mu sync.Mutex:18` `Place:38`/`PlaceAll:80`. Leader throughput is single-threaded; replicas of `scheduler`/`replicamanager`/`evacuationplanner` contend. Correctness OK, throughput P1 | **P1** | `placement/engine.go:18,38` | Still BROKEN (`FINAL_PARITY_AUDIT.md:§10.1 Leader throughput`) |
| **SCH-03** | No blocked-eval equivalent: one-shot provision vs Nomad style blocked eval queue. Freed capacity only observed on next poll | P2 | `placement/engine.go:37`, `services/scheduler/*.go` | Open |
| **SCH-04** | Spread/affinity/weighted-affinity/reschedule policy missing: only binary affinity/anti-affinity + region/node/label. No attribute-target % spread, no `Weight -100..100`, no `DelayFunctions` exponential/constant/fibonacci, no sticky fallback (`hard requiredNode fails` vs `findPreferredNode`) | P2 | `placement/constraints.go:9-25`, `placement/engine.go:40` | Open |
| **SCH-05** | `FilterByConstraints:181` collects `reasons` per excluded candidate but caller at `engine.go:40` logs only when `e.logger != nil`; no metric for exclusion rate — unobservable placement failures | P3 | `placement/engine.go:40-44` | Open |
| **DRAIN** | `services/migration` drain deadline `DrainStrategy Deadline/ForceDeadline, MaxParallel` partially modeled via `evacuationplanner Plan ledger + bounded evacuator` — placement not re-checked after drain decision | P2 | `services/evacuationplanner/*.go`, `services/migration/*.go` | Open |

### 2.8 Wiring / Health (touches `cmd/api/main.go`, `internal/http/server.go`, `internal/services/health`)

| ID | Title | Severity | Files:line | Status |
|---|---|---|---|---|
| **AP-02** | Deployment execution stubs: `services/deployment/execution.go:249` `executeProvision/Promote/Drain` were stubs reporting `completed` with zero provisioning. **Partially fixed** at `execution.go:268-274` (`runtime==nil → error` honest) + `beacon_executor.go:20 WireBeaconExecutor` at `main.go:819` + `verifyObservedRunning:338` now gates drain/scale. Recreate honest; blue-green/canary/rolling still `verifyObservedRunning` only (not `Scale`). Updated synthesis: `AP-02 PARTIALLY FIXED` | **P0** (partial) | `services/deployment/execution.go:264`, `services/deployment/beacon_executor.go:20`, `cmd/api/main.go:819`, `services/deployment/execution.go:338` | Partially fixed — blue-green/canary/rolling still false |
| **AP-08** | Health gate probes `localhost` clamped at `healthgate.go:71` `validateHealthGateTarget`. **Default fixed** at `resolveNodeHost:62` uses `NodeURL→Hostname` not localhost; explicit `Host` override still clamped correctly. Residual: `DaemonSFTPAlias` not resolved | P0 → P2 residual | `services/deployment/healthgate.go:11,71`, `services/deployment/healthgate.go:62` (fix) | Default fixed, per reverification-06 |
| **HC-01** | Health endpoints: `server.go:1300` `/health/live`, `1305` `/health/ready`, `1323` `/health` (legacy), `1340` `/health/:check`. Docker `HEALTHCHECK` at `main.go:121-122` hits `http://127.0.0.1`+`healthcheckPort`+`/api/v1/health/ready` — **localhost** but correct for container self-probe (same netns). Prior audit flag `health localhost` was about **health gate target** (`localhost` for remote node), not this probe — distinguish | Info | `cmd/api/main.go:111-122,1742`, `http/server.go:1300-1347` | Correct (self-probe locality) |
| **HC-02** | `healthcheckPort(addr):1742` parses `:8080` vs `host:port`; fallback `:8080` is correct. 3-second `nethttp.Client:1755` is short but OK for localhost | Info | `cmd/api/main.go:1742` | Correct |
| **BOOT-01** | Startup timeout 10 min `main.go:127` + unbounded service `Start` fan-out (`1150-1174`: 15+ `Start` calls) — no aggregated health gate before `app.Listen`. A slow `MigrateOperationalSecrets:197` or `RunMigrations:191` blocks 10m; service `Start` errors only warn (`slog.Error` not returned) except `cronJobSvc:1147`, `failSvc:1161` — silent partial start | P2 | `cmd/api/main.go:127,1147,1161` | Open |

---

## 3. Files That WILL Be Touched for Fixes (exact paths)

Grouped by remediation batch. No file is hypothetical — each maps to §2 finding.

### 3.1 Auth & Session (P0/P1)

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/auth/session.go` | Fix pointer race: hash token in `byToken`, copy on `Get/GetByToken` return, atomic `LastActiveAt` or dedicated `Touch` method; gate `fallback-nonce` panic | SE-12, AUTH-01 |
| `forge/api/internal/auth/session_postgres.go` | Align expiry/index/hash semantics with in-memory, add `Touch` throttling (e.g., `UPDATE ... WHERE now - last_active > 5m`) | SE-12, AUTH-01 |
| `forge/api/internal/auth/remote_hmac.go` | No change needed for correctness but add bounded `Accept` jitter test, Redis key prefix `hmac:nonce:` already correct `84` | SE-07 adjunct |
| `forge/api/internal/auth/scopes.go` | Add wildcard deny: `ParseScopes` must reject `*` unless caller has `user.create`*? Actually fix is in `store` but scope gate stays here | GH-18 |
| `forge/api/internal/store/store_users.go` | Close wildcard: `normalizeSubuserPermissions:563` must strip `*` for non-admin, enforce actor subset (intersect with grantor's own perms) at `UpsertServerSubuser:297` | GH-18/SE-01/P0 |
| `forge/api/internal/store/permissions.go` | Add explicit `IsWildcard`/`HasWildcard` helper, ensure `ValidateApiKeyScopes` rejects `*` for non-admin | GH-18 |
| `forge/api/internal/http/auth.go` | Optional: add scope synthesis test for admin `*`, throttle `recordUserSession:555` (`sha256Hex(jti)` already safe) | AUTH-03 |

### 3.2 Middleware & CORS/CSRF/Security headers

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/http/middleware_ratelimit.go` | Replace `IsPrivate/IsLoopback/IsUnspecified` peer check with explicit `TRUSTED_PROXY_CIDRS` env; fix `tryRedis` to use `EVALSHA INCR+PEXPIRE` atomic Lua; unify `X-Real-IP` trust with same allowlist | SE-07, MW-02, MW-03 |
| `forge/api/internal/http/middleware_mtls.go` | Do not trust `X-Forwarded-Proto` from untrusted peer; require `X-Forwarded-Proto` only when `peer` in `TRUSTED_PROXY_CIDRS`; else require `c.Protocol()=="https"` strictly | SE-13 |
| `forge/api/internal/http/middleware_security.go` | On `rand.Read` failure panic or return 500 instead of `fallback-nonce` with `strict-dynamic`; deduplicate generator with `middleware_security_headers.go` | SE-08 |
| `forge/api/internal/http/middleware_security_headers.go` | Align with above; delete one of the two CSP middlewares or make `middleware_security_headers.go` canonical (configurable) and have `middleware_security.go` delegate. Remove falling fallback | MW-01, SE-08 |
| `forge/api/internal/http/middleware_cors.go` | No fix needed; verify `API_CORS_ALLOWED_ORIGINS` parsing at `server.go:1079` covers prod | MW-04 |
| `forge/api/internal/http/middleware_csrf.go` | Verify `SameSite`/`Secure` tied to `APP_ENV` not just `LoadSessionCookieConfig` | Info |
| `forge/api/internal/http/middleware_ipaccess.go` | Align `AdminIPAccessConfig`/`APIIPAccessConfig` with same `TRUSTED_PROXY_CIDRS` semantics | IP-01 |

### 3.3 Store

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/store/store_egg_variables.go` | Strip `/.../` delimiters before `regexp.Compile`, handle `\|` inside char class, normalize Ruby/Perl `(?i)` etc. | GH-14/P0 |
| `forge/api/internal/store/store_mounts_ext.go` | Expand deny-list to `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/` (or allow-list mount parents) | GH-19/P0 |
| `forge/api/internal/store/store_backups.go` | Wrap prune `DELETE ... WHERE ...` with `pg_advisory_xact_lock` or `SELECT FOR UPDATE` + `EXISTS` guard | BK-07/P1 |
| `forge/api/internal/store/store_dns.go` | Encrypt `dns_credentials` / `dns_provider_accounts.credentials` via `secrets.Keyring` (same as `047_encrypt_operational_secrets`) — new column dual-write | SEC-DB/P1 |
| `forge/api/internal/store/store_acme_accounts.go` | Same as above if separate table | SEC-DB |
| `forge/api/internal/store/seeder.go` | Extend `DefaultSeeder` to seed egg templates from `packages/game-templates` or `egg-templates.ts` manifest; or wire `Seed` at `store.go:Seed` to call egg seeder | DB-02/P1 |
| `forge/api/internal/store/store.go` | Fix `splitSQLStatements` for `DO $$`, remove or implement `Rollback` down-files, fix `DatabaseHealthDetails` error wrapping | STORE-01/02 |
| `forge/api/internal/store/migration.go` | Mirror `DO $$` fix for `MigrationRunner.Run` (already handles `DO $$:110`); keep parity with `store.go` | STORE-01 |
| `forge/api/internal/store/permissions.go` | Complement to `store_users.go` fix | GH-18 |

### 3.4 Daemon client

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/daemon/client.go` | Optional: rename env `MTLS_ENABLED` → support both `MTLS_ENABLED` and `DAEMON_MTLS_*` in `isDaemonMTLSEnabled:328`; add pull allowlist env; no logic fix needed for phantom runtime (beacon-side) | D-01, CL-01 |
| `forge/api/internal/daemon/build.go` / `compose.go` | Verify cache-forward already fixed `build.go:54-56` per reverification — no change | Info |

### 3.5 Config

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/config/validator.go` | Fix `isTLSDatabaseURL:96` to allow extra query params (`strings.Contains` not exact 1 value); add `PANEL_URL https` check for prod mirroring `main.go:134` | CF-03, CF-04 |
| `forge/api/internal/config/config.go` | Remove redundant `All():194` `os.Getenv` fallback (viper already bound), or keep as defensive but test; decide canonical config path (`internal/config.NewManager` vs `cmd/api/main.go:env()`) and deprecate the other | CF-01, CF-02 |
| `forge/api/internal/config/env.go` | Fix `Debug:13` double-read of `APP_ENV`; make `Debug` derive from `APP_DEBUG` only | CF-02 |
| `forge/api/config/mtls.go` | Align env names `MTLS_*` vs `DAEMON_MTLS_*` with `daemon/client.go:328` | D-01 |

### 3.6 Eventstore & Events

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/eventstore/store.go` | Add `PublishTx(ctx, tx, envelope)` (or `PublishWithTx`) that inserts `events` row inside caller's DB transaction `BEGIN` → work → `INSERT events` → `COMMIT` — durably atomic with state mutation | AF-1/P0 |
| `forge/api/internal/eventstore/outbox.go` | Add `Relay.Subscribe`-consumer wiring or delete `Relay` if `Registry` is the wire; otherwise document Relay as dead and remove `ClaimPending` polling (saves DB load). If kept, wire at least `observability`/`webhook`/`reconciler` via Relay not in-memory | AF-1 |
| `forge/api/internal/eventstore/migration.go` | Ensure it accepts `*pgxpool.Pool` or `pgx.Tx` (not `*sql.DB`) matching `store.GetDB()` type | Q-04/P1 |
| `forge/api/internal/events/registry.go` + `publisher.go` + `event.go` | Add `TenantID` field to `Envelope:196` (`FINAL_PARITY_AUDIT.md:SE-03`) — tenant-blind placement/audit | SE-03/P1 |

### 3.7 Placement

| Path | Why | Findings |
|---|---|---|
| `forge/api/internal/placement/constraints.go` | Replace `+1e12/-1e10` with normalized `+1.0/-1.0` or scaled `±1..10`, or move soft bonus into score normalization tier. Immediate additive fix: bound bonus `clamp(bonus, -10, +10)` then `score+bonus` | SCH-01/P1 |
| `forge/api/internal/placement/engine.go` | Replace global `sync.Mutex:18` with per-call stateless `Place` (immutable scorer/checker) or `sync.RWMutex` + sharded placement; remove `mu` entirely if scorer is pure | SCH-02/P1 |
| `forge/api/internal/placement/strategy.go` | Add spread/affinity weight iterators (move iterator logic here) if not deferred to Phase-2 | SCH-04/P2 |
| `forge/api/internal/services/scheduler/service.go` | Wire locality `±1e10` removal (same family as placement bonus) — cross-check with `handlers_servers.go:860` not populating | Extension |

### 3.8 Wiring / Health / Server

| Path | Why | Findings |
|---|---|---|
| `forge/api/cmd/api/main.go` | Do not mark subuser wildcard fix as done without `store_users.go`; add `TRUSTED_PROXY_CIDRS` validation at startup; add `PublishTx` call sites (replace `outboxPub.Publish` after commit with tx-scoped publish) | GH-18, AF-1, BOOT-01 |
| `forge/api/internal/http/server.go` | Wire `TRUSTED_PROXY_CIDRS` into `ExtractClientIP` + `MTLSAuthMiddleware` + `IPAccessControl`; deduplicate CSP middleware; throttle `LastActiveAt` update | SE-07/13, MW-01, AUTH-01 |
| `forge/api/internal/services/health/*` + `cmd/api/main.go:1316-1406` | No fix (health self-probe is correct). Add `DaemonCheck` detail `healthy/unhealthy` already present `1398` | Info |
| `forge/api/internal/services/deployment/execution.go` + `beacon_executor.go` | Blue-green/canary/rolling honesty: wire real `Scale`/`Drain` via beacon, not just `verifyObservedRunning:338` — or document as P2 defer and keep fail-fast | AP-02 residual |
| `forge/api/internal/services/deployment/healthgate.go` | Verify `resolveNodeHost:62` fix persists; add explicit test for remote node host vs `localhost` clamp | AP-08 residual |

---

## 4. Additive Migration Plan (new tables/columns, dual-read strategy)

All mutations are **additive only** (new columns/tables, new indexes, no `DROP COLUMN`/`NOT NULL` without default, no renames in-place). Existing code keeps reading old shape; new code dual-reads and single-writes new shape; backfill is idempotent; later cleanup drops old shape in a separate release.

### 4.1 Migration index map (existing runway)

- Current head: `112_cron_jobs.sql` (201 files). `091_seed_minecraft_java.sql` is the only seeded egg. `084_create_missing_tables.sql` catch-all exists — avoid overloading it; new migrations get own numbers `113_*` forward.
- Prefix validation `migration.go:271` enforces uniqueness of prefix (`113`, `113_a`, …) — choose `113` for first API-core batch.
- `schema_migrations.version` holds full filename `store.go:1271`; renames are not safe after deploy.

### 4.2 Proposed additive migrations for API-core fixes

#### M113 — Session touch throttling (no schema change, but add index if missing)

```sql
-- 113_session_touch_throttle.sql
-- No new column — last_active_at already exists on user_sessions (002, 041, 053).
-- Add partial index to avoid full scan on touch throttling predicate.
CREATE INDEX IF NOT EXISTS idx_user_sessions_touch_throttle
  ON user_sessions (id, last_active_at)
  WHERE revoked = false;
-- Code fix is in Go (sql predicate CASE), not DDL. Migration is index-only.
```

Dual-read: none. Backfill: N/A. Risk: index build `CONCURRENTLY` preferred — without it, `CREATE INDEX` locks writes for duration. On <100k rows negligible; on large fleets use `CONCURRENTLY` variant (requires separate helper since `store.go:1265` splits on `;`; add `CONCURRENTLY` statement in own transaction).

#### M114 — Event outbox durability (add `events.published_via_tx bool`)

```sql
-- 114_event_outbox_tx_marker.sql
ALTER TABLE events ADD COLUMN IF NOT EXISTS published_via_tx boolean NOT NULL DEFAULT false;
-- For observability: which events were written in-tx vs after-commit.
CREATE INDEX IF NOT EXISTS idx_events_published_via_tx
  ON events (published_via_tx, dispatched) WHERE dispatched = false;
```

New column defaults `false` so existing rows are not migrated. New `PublishTx` path sets `published_via_tx=true`. Old `Publish` keeps `false`. Dual-read: both `false` and `true` are valid; consumers do not filter. Backfill: N/A (append-only). Cleanup: none (column retained).

#### M115 — DNS credential encryption (add encrypted mirror columns)

```sql
-- 115_dns_credentials_encrypted.sql
ALTER TABLE dns_credentials ADD COLUMN IF NOT EXISTS credentials_encrypted bytea;
ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_encrypted bytea;
ALTER TABLE dns_credentials ADD COLUMN IF NOT EXISTS credentials_key_id text;
ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_key_id text;
CREATE INDEX IF NOT EXISTS idx_dns_credentials_needs_reencrypt
  ON dns_credentials (id) WHERE credentials_encrypted IS NULL AND credentials IS NOT NULL;
```

Dual-write: Go layer writes both `credentials` (plaintext JSONB legacy) and `credentials_encrypted` (`secrets.Keyring.Seal` with `key_id`). Dual-read: `GetDNSCredentials` tries `credentials_encrypted` first (via `Keyring.Open`), falls back to `credentials` JSONB if null, then schedules async re-encrypt. Backfill: background job `REPLACE` eligible rows (`WHERE credentials_encrypted IS NULL`) idempotent, checkpointed per row, safe to resume. Cleanup (next release): `ALTER TABLE ... DROP COLUMN credentials` after `SELECT COUNT(*) WHERE credentials_encrypted IS NULL = 0` in all tenants.

Note: `046_encrypt_operational_secrets.sql` precedent already dual-writes for `nodes.daemon_token_encrypted` etc. Reuse same `secrets.Rotation` + `MigrateOperationalSecrets:197` pattern — do not invent new envelope.

#### M116 — Tenant scoping on events and placements (add `tenant_id`)

```sql
-- 116_tenant_scoping.sql
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE placement_reservations ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE placement_intents ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS org_id uuid;
-- FK deferred — add after backfill to avoid blocking DDL on FK check.
CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_placement_reservations_tenant ON placement_reservations (tenant_id);
```

Dual-read: old rows have `NULL` tenant_id → treated as `unscoped` (legacy). New writes populate `tenant_id` from `auth claims → org_id`. Placement engine at `placement/engine.go:40` already receives `ConstraintContext`; no constraint change needed — engine just avoids cross-tenant affinity target leakage when tenant is supplied. Backfill: populate from `servers.org_id` join where server_id maps; else remain NULL until touched.

Not required for P0 close but flagged as `SE-03/P1`; ship as additive now, enforce non-null later.

#### M117 — Placement scoring observability (no column, add metrics table optional)

No migration. In-code change `constraints.go:59` clamp bonus — no DDL. If metric desired, reuse existing `observability` tables.

#### Not shipped in API-core batch (deferred)

- `river_queue` table is orphaned (`cmd/api/river.go:13`) — do not drop in this batch; coalesce with queue single-writer decision (`queue.Service` sole execution, `operation.Service` read-model) in Orchestration slice. Add `DROP TABLE IF EXISTS river_queue` as `M118` only after writer decision is landed and `queue/leader.go:18` elector wired.
- `security_headers` consumers are dead — do not drop column `panel_settings.security_headers` until Networking slice unifies Gateway.

### 4.3 Execution order & safety

1. **M114** first (no dependency, zero risk, enables `PublishTx` call sites).
2. **M115** second (depends on `secrets.Keyring` already initialized at `main.go:172`; run after `046` style).
3. **M113** third (index-only, can run concurrently with 115).
4. **M116** last (largest blast radius, optional for readiness; may be deferred to Phase-02 if timeline tight).

Each migration is a single `BEGIN` `COMMIT` per `store.go:1261`; keep each file to one `ADD COLUMN` batch + one `CREATE INDEX`. For `CONCURRENTLY` index variant, split into separate file `113_a_*` to avoid `BEGIN`/`CONCURRENTLY` conflict.

---

## 5. Risk: What Breaks If We Change Auth/Store

### 5.1 Auth changes

| Change | What breaks | Mitigation |
|---|---|---|
| Hash `byToken` (store SHA256 of raw token) | All existing session cookies / in-flight bearer tokens invalidate (token → hash mismatch `auth/session.go:86`). Users forced to re-login; WebSocket tickets bound to old token fail. | Dual-read period: look up by `SHA256(raw)` **and** by raw fallback; write new rows by hash only; after TTL (24h `auth.go:20`) raw table is cold. Or provide one-time `migrate_sessions_hash` backfill (scan `user_sessions` `token_hash` vs `byToken` map — no DDL). For DB-backed store, add column `session_token_hash` and backfill. |
| Copy-on-read `*Session` (fix pointer race `111`) | Callers mutating returned `*Session` without `Update` will see stale copy; `SessionMiddleware:261` does `sess.LastActiveAt=Now(); store.Update()` — correct after copy (write-through). External callers that relied on shared pointer aliasing to observe mutation will silently diverge. | Audit all `GetByToken`/`Get` call sites (only `SessionMiddleware:248` + `http/server.go:261`). Neither relies on shared aliasing except the race itself — safe. Add test `TestSessionConcurrentLastActiveAtRace` reproduces. |
| `LastActiveAt` throttling (`WHERE now - last_active > 5m`) | If predicate too coarse, `LastActiveAt` appears frozen for 5 minutes; audit trails that read `last_active_at` for liveness (maybe `activity` service) will lag. | Keep in-memory store eager, DB throttled. Document as intentional. Add metric `session_touches_throttled_total` to observe hit rate. |
| `CSP fallback-nonce` panic | A single `rand.Read` failure (host entropy stall) would 500 every request instead of serving with weak nonce. Availability vs XSS tradeoff shifts to availability loss. | Fail to `500` only in `production` (`validator.go:isProductionEnv`); in dev still emit fallback with warning log but no `strict-dynamic`. |
| `ExtractClientIP` CIDR tightening | Clients previously counted under XFF `X.Y.Z.W` will map to new IP after trusted-CIDR change → rate-limit buckets reset, logs show `peer` not `xff`. Ops dashboards relying on `clientIP == xff` break. | Add `TRUSTED_PROXY_CIDRS` defaulting to `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8,::1/128` to match current `IsPrivate||IsLoopback` union; behavior identical on day one, explicit override later. Log `peer` + `xff` pair for N days before cutover. |

### 5.2 Store changes

| Change | What breaks | Mitigation |
|---|---|---|
| `normalizeSubuserPermissions:563` reject `*` + actor subset | Existing rows with `permissions=["*"]` for non-admin become silently truncated → affected subusers lose access, support tickets. | On deploy, emit warning log line per affected row (`store_users.go:Upsert` scans). Do not auto-revoke existing `*` rows — instead, gate **new writes** only; add off-hours `SELECT` of rows with `'*' = ANY(permissions) AND role != 'admin'` for operator triage before second deploy that revokes. Provide one-shot `migrate_revoke_wildcard` whitelisted by admin. |
| `store_egg_variables.go:176` slash-strip fix | Eggs that were imported with literal `/pattern/` in DB will now compile as `pattern` — some previously-rejected-but-stored patterns may become valid and change user-visible validation (variables that previously errored 400 now pass). | Backfill not needed — regex compiled at request time. Add integration test importing `minecraft-paper.json:58` canonical PTDL fixture and assert `Validate` passes. Keep backward-compat: if compiled with slashes vs without both pass, accept either. |
| `store_mounts_ext.go:323` deny-list expansion | Nodes with mounts to newly-blocked parents (`/etc/shadow` ancestors, `/var/run/docker.sock` symlink target, `/proc`) will fail mount sync — existing servers with those mounts return 400 on next `UpdateServerMounts`/`Mounts` call. | Add `validateMountsDryRun` helper returning warnings not errors for 1 release; log `mount_blocked_by_new_policy` with `server_id`. Enforce only on **new** mounts; grandfather existing via `WHERE created_at < migration` whitelist flag column later. |
| `store_backups.go:240` add advisory lock | Callers that held no lock will now block 5-10s under contention — `retention.go:61` AND vs OR bug means retention delete was over-eager; adding lock without fixing AND makes lock contention last longer on the wrong rows. | Fix `retention.go:61` AND→OR semantics first (outside this slice but same transaction), then add lock. Keep lock scope minimal (`SELECT pg_advisory_xact_lock($1)` inside the same tx as `DELETE`). |
| `dns_credentials` dual-column encrypt | Old `credentials` column remains plaintext until backfill completes — at-rest exposure persists during window; `Keyring` rotation must handle both columns atomically or lose write if one fails. | Wrap dual-write in same DB tx as caller; on `credentials_encrypted` write fail, skip `credentials` clear (keep plaintext rather than lose data). Emit metric `dns_reencrypt_remaining` and alert < 0 after 7d. |

### 5.3 Config changes

| Change | Why fragile | Mitigation |
|---|---|---|
| Canonicalize config to `internal/config.NewManager` | `cmd/api/main.go:1311` env helpers and `internal/config/config.go:145` viper diverge on defaults (`app.name` GamePanel vs gamepanel, `migrations_dir` `migrations` same, but `auth.session_limit` only in internal). Switching canonical changes default `AUTH_SESSION_LIMIT` 10 vs unbounded. | Freeze defaults diff in a table in PR description; keep `cmd/api/main.go` as thin adapter calling `internal/config.FromEnv()` or `Manager.All()` for 1:1 override, not dual source. |
| Fix `isTLSDatabaseURL:96` | URLs with `sslmode=require&sslrootcert=...` would newly pass (were rejected) — prod deploys using `verify-ca` + extra params were blocked before; now unblocked. Risk is actually a fix — no rollback concern, but log `db_url_tls_check_passed` on change. | Add regression test `TestIsTLSDatabaseURL_WithExtraParams` before change. |

### 5.4 Transactional / migration risks

- `store.go:1261` per-file tx means `M115` `ALTER TABLE` + `CREATE INDEX` are one atomic tx — takes `AccessExclusiveLock` on the table for duration. On a hot `dns_credentials` table, lock will block concurrent writes. Keep each `ALTER` in its own file (`115_a`, `115_b`) if row count > 100k.
- `MigrationRunner` `migration.go:32-62` vs `store.go:1226` have divergent SQLite shims — prod is postgres (fine) but local SQLite dev (`mattn/go-sqlite3:25`) would hit different split logic. Test both paths: `go test ./internal/store -run TestMigration_Runner` on `GOOS=linux -tags sqlite`.
- `Rollback` dead — if any future `M113-117` requires down-file, ship it under `migrations/rollbacks/` and update `Rollback:1319` path; until then document forward-only.

---

## 6. Effort & Dependencies

### 6.1 Effort by batch

| Batch | Items | Effort | Calendar | Dependency |
|---|---|---|---|---|
| **B1 Auth wildcard + session race** | `store_users.go:563` + `permissions.go` + `auth/session.go` copy/race + `LastActiveAt` throttle | **S** (1-2 days, 3 files, ~80 lines) | Low risk, parallelizable | None. Must land before any RBAC UI work. |
| **B2 CSP deduplicate + fallback** | `middleware_security.go:22` + `middleware_security_headers.go:43` dedup, `server.go:1098` single wire | **S** (<1 day, 3 files) | — | None. |
| **B3 Trusted proxy CIDR** | `middleware_ratelimit.go:98`, `middleware_mtls.go:123`, `middleware_ipaccess.go`, `server.go:1130`, `cmd/api/main.go:120-140` env plumbing + `TRUSTED_PROXY_CIDRS` | **M** (2-3 days, tests for `ExtractClientIP` matrix) | — | Coordinate with infra (ALB/Caddy `XFF` shape). Not on critical path for correctness except rate limits. |
| **B4 Store: egg regex + mount allowlist + backup lock** | `store_egg_variables.go:176`, `store_mounts_ext.go:323`, `store_backups.go:240` + `retention.go:61` AND→OR semantics fix (cross-slice) | **M** (3-4 days, fixtures `minecraft-paper.json`) | — | Needs PTDL fixture corpus; mount test needs symlink eval cases. |
| **B5 DNS credential dual-enc** | `M115` DDL + `store_dns.go`/`store_acme_accounts.go` dual-write/fallback + `MigrateOperationalSecrets` reuse + backfill job | **M** (3-5 days, Keyring rotation interaction) | — | Requires `secrets.Keyring` existing header `FORGE_MASTER_KEY` + `FORGE_PREVIOUS_MASTER_KEYS` documented rotation procedure. |
| **B6 Config canonicalization** | `internal/config/*` unification, `validator.go:96` TLS fix, `main.go:1813` already correct | **S** (1-2 days, mostly tests/docs) | — | None. |
| **B7 Eventstore PublishTx** | `eventstore/store.go:113` `PublishTx` + `outbox.go:214` `PublishTx` overload + all `outboxPub.Publish(ctx, ...)` call sites in `cmd/api/main.go:554-669` (power/restore/file) converted to tx-scoped publish | **M** (3-4 days, every `db` write site touching `Publish` must be audited) | **Must batch with call-site sweep** — 12+ sites in `main.go` (e.g., `576-601` install, `639-670` restore). Each site needs `tx, _ := db.Begin(ctx)` refactor. | Depends on `store.GetDB` returning `*pgxpool.Pool` typed for `Begin` (currently does `22`). Decide: `BeginTx` helper on `Store` wrapping `db.Begin`. |
| **B8 Placement soft-bonus clamp + mutex removal** | `constraints.go:59` (`1e12`→`1.0`), `engine.go:18` remove `mu`, normalize scorer ≤3 | **S-M** (1-3 days, benchmark `placement/load_test.go`) | — | Needs `placement/load_test.go` baseline before/after to prove throughput gain. |
| **Overall** | B1-B8 | **M-L** (2-3 engineer-weeks if B3/B5/B7 in parallel; S-M if B7 deferred) | — | Critical path: B1 → B4 → B7. B3/B5 can parallel. |

### 6.2 Dependencies between batches

```
B1 (wildcard/race)  ── independent ──► can land first, no prereq
B2 (CSP)            ── independent
B3 (trusted CIDR)   ── needs infra CIDR list ──► maybe after B1
B4 (store)          ── PTDL fixtures + mount policy doc
B5 (DNS encrypt)    ── needs M115 merged first + Keyring cred docs
B6 (config)         ── independent, land after B1
B7 (PublishTx)      ── needs Store interface change + call-site sweep ──► last
B8 (placement)      ── independent, benchmark before merge
```

Cross-slice dependencies (outside API core):
- `queue/leader.go:18` elector (Orchestration slice) — B7's `Relay` single-writer decision competes with Orchestration's `queue.Service` vs `operation.Service` winner-take-all. Do not wire `Relay` consumers until that decision lands.
- `services/deployment/beacon_executor.go:20` honesty — AP-02 residual needs beacon side (`beacon/server.go:740` Provider passthrough) shipped in Runtime slice.
- Networking five-writers (`caddy_proxy.go:894`) — not touched here; API-core must not add a sixth gateway writer (keep `traefik_proxy.go` dead).

---

## 7. Implementation Checklist (ordered steps — execute B1→B8)

Checklist is ordered for **audit-safe incremental rollout** (each item is independently testable, shippable, reversible). Do not squash.

### Step 0 — Scaffolding (pre-flight, no code change)

- [ ] 0.1 Create migrations directory lock: verify `migrations/113_*` prefix free (`store/migration.go:271` check via `go test -run TestValidateNoDuplicatePrefixes`).
- [ ] 0.2 Add regression fixtures to repo: copy `reference/pterodactyl-panel/tests/fixtures/minecraft-paper.json:58`-like PTDL egg into `forge/api/testdata/ptdl/paper.json`; add `TestEggVariableRegexWithDelimiters`.
- [ ] 0.3 Baseline placement throughput: `go test ./internal/placement -run TestLoad -count 5` records p50 ms; save as `placement_load_baseline.json`.
- [ ] 0.4 Inventory all `outboxPub.Publish` call sites: `rg -n 'outboxPub\.Publish|\.Publish\(.*events\.' forge/api --glob '*.go'` → must enumerate 12 sites at `main.go:554,583,652,670,860,945,1100,...` for B7 sweep.

### Step 1 — B1: Close subuser wildcard + fix session race (P0/P1)

- [ ] 1.1 `internal/store/store_users.go:563` `normalizeSubuserPermissions` — reject `*` when `actorRole != admin` and require `input ⊆ actor.Permissions` (load actor's perms via `GetUser`+`ListServerSubusers` or cached set). Add helper `filterToSubset(input, actorSet) error`.
- [ ] 1.2 `internal/store/permissions.go` — add `HasWildcard(scopes) bool`, `ValidateSubuserPermissions(scopes, isAdmin) error`; wire into `store_users.go` `UpsertServerSubuser` and `CreateSubuserInvitation` (`store/store_subuser_invitations.go` successor).
- [ ] 1.3 `internal/store/store_users_test.go` (new) — cases: `*` rejected for non-admin, subset enforced, empty input allowed, admin can grant `*`.
- [ ] 1.4 `internal/auth/session.go:36-98` — store `byToken[sha256Hex(token)]`, store `*Session` deep copy (not raw pointer), `Get/GetByToken` return `copySession(s)` (deep copy of `Permissions` slice). Add `sha256Hex` helper (already in `http/auth.go:555` style — dedupe).
- [ ] 1.5 `internal/auth/session.go:100-113` `Update` — update `byToken` index when `Token` rotates; copy-in `sessions[id]=copySession(session)`.
- [ ] 1.6 `internal/auth/session_postgres.go` — add `UPDATE user_sessions SET last_active_at = now() WHERE id=$1 AND now() - last_active_at > interval '5 minutes'` fast-path; expose `Touch(ctx, id) error`.
- [ ] 1.7 `internal/auth/session.go:261-262` + `internal/http/server.go:261` — gate `Update/Touch` behind `time.Since(sess.LastActiveAt) > 1*minute` to cut write amplification (AUTH-01).
- [ ] 1.8 Tests: `go test ./internal/auth -run TestSessionConcurrentLastActiveAt -count 100 -race` must pass.

**Done when:** `rg '\*\".*wildcard|byToken\[sess\.Token' internal/auth/session.go` returns 0 hits for raw token key; `TestSubuserWildcard` enforces P0.

### Step 2 — B2: Deduplicate CSP middleware (P2)

- [ ] 2.1 Pick canonical: `middleware_security_headers.go:48 SecurityHeadersMiddleware` (configurable) is more flexible. Keep it, make `middleware_security.go:27 SecurityHeaders(env)` delegate to it with `DefaultSecurityHeadersConfig()` (or delete delegator and update `server.go:1098` to call `SecurityHeadersMiddleware` directly).
- [ ] 2.2 `middleware_security*.go:19,40` `generateNonce` — on `rand.Read` error: in `production` return `error` and have middleware return `500`; in dev return fallback **without** `strict-dynamic` and log Warn. Remove `fallback-nonce` with `strict-dynamic`.
- [ ] 2.3 `internal/http/middleware_security_headers_test.go` — assert nonce length 24 (16 bytes base64), assert no `fallback-nonce` substring, assert `X-CSP-Nonce` header mirrors CSP nonce.
- [ ] 2.4 Update `server.go:1098` single CSP wire.

**Done when:** one nonce generator, no `fallback-nonce"+"strict-dynamic"` pair in codebase.

### Step 3 — B3: Trust proxy CIDR (P1)

- [ ] 3.1 Add env `TRUSTED_PROXY_CIDRS` (comma-separated CIDRs, default private+loopback union) at `internal/config/env.go:38` (`TrustedProxyCIDRs []string`) + `cmd/api/main.go:120-140` plumbing.
- [ ] 3.2 `middleware_ratelimit.go:98` `ExtractClientIP(c, trustedCIDRs []string)` — `net.ParseIP(peer)` must be inside `cidr.Contains(peerIP)` for some entry in `trustedCIDRs`, not `IsPrivate`. Remove `X-Real-IP` fallback or tie it to same allowlist.
- [ ] 3.3 `middleware_mtls.go:123` — guard `X-Forwarded-Proto` similarly: only trust when `peer` in `trustedCIDRs`; else `c.Protocol()=="https"` strict.
- [ ] 3.4 `tryRedis:212` — replace `INCR`+`EXPIRE` with Lua `EVALSHA "local c=redis.call('INCR',KEYS[1]); if c==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return c" 1 key windowMs`.
- [ ] 3.5 Tests `middleware_ratelimit_test.go` — matrix: `peer=10.0.0.5 XFF=1.2.3.4 → 1.2.3.4`; `peer=203.0.113.5 XFF=1.2.3.4 → peer`; right-most vs X-Real-IP precedence.

**Done when:** `rg 'IsPrivate' internal/http/middleware_ratelimit.go` returns 0 hits outside comments.

### Step 4 — B4: Store correctness (P0/P1)

- [ ] 4.1 `store_egg_variables.go:176` — `sanitizeRegexPattern(raw string) string { raw=TrimSpace(raw); if len>=2 && raw[0]=='/' && last=='/' { inner:=raw[1:len-1]; lastSlash:=strings.LastIndex(inner,"/"); if lastSlash>=0 && isFlags(inner[lastSlash+1:]) inner=inner[:lastSlash]; raw=inner } raw=strings.ReplaceAll(raw, "\\|","|"); return raw }` then `regexp.Compile(sanitized)`. Add `isFlags` helper (`i,m,s`).
- [ ] 4.2 `store_mounts_ext.go:323` — replace narrow `source==/etc/forge` style with `blockedPrefixes := {"/proc","/","/var/run/docker.sock","/etc/shadow","/boot","/sys"}` plus parent check (`if source==p || strings.HasPrefix(source, p+"/")` or normalized via `filepath.Clean`). Keep allow-list path: `allowedMounts` evaluation at `beacon/mounts.go:13 EvalSymlinks+Rel` remains enforcement-in-depth.
- [ ] 4.3 `store_backups.go:240` — wrap retention/prune delete in `SELECT pg_advisory_xact_lock($1)` where lock ID = `hashtext('retention_prune:'||server_id)`. Add `retention.go:61` semantics fix (AND→OR) via join with RetentionService slice (mark deprecated path).
- [ ] 4.4 `store/migration.go:110` + `store.go:1265` — make `store.go:runMigrations` skip `DO $$` blocks (same as `migration.go:110`) or use the same `MigrationRunner` everywhere and delete `store.go:runMigrations` duplicate.

**Done when:** PTDL import `minecraft-paper` passes; `TestMountAllowlist` rejects `/proc/self/environ` symlink variant; `TestBackupPruneConcurrent` parallel 10 workers no duplicate delete.

### Step 5 — B5: DNS credential encryption (P1)

- [ ] 5.1 Author `migrations/115_dns_credentials_encrypted.sql` (see §4.2 M115).
- [ ] 5.2 `store/store_dns.go` — inject `*secrets.Keyring` (from `cmd/api/main.go:172` already available at store construction). `Create/Update` dual-write `credentials` + `credentials_encrypted=keyring.Seal(json)`, `Get` dual-read (encrypted first via `Open`, fallback plaintext), `List` redacts.
- [ ] 5.3 Background backfill at `cmd/api/main.go:197` after `MigrateOperationalSecrets`: call `db.BackfillEncryptedDNS(ctx)` (idempotent, paginated 100 rows, `WHERE credentials_encrypted IS NULL`).
- [ ] 5.4 Metrics: expose `dns_credentials_unencrypted_total` via `internal/http/middleware_metrics.go` gauge.

**Done when:** `SELECT count(*) FROM dns_credentials WHERE credentials_encrypted IS NULL AND credentials IS NOT NULL` == 0 after backfill; `SELECT pg_column_size(credentials_encrypted) > 0` for new rows.

### Step 6 — B6: Config canonicalization (P2)

- [ ] 6.1 Choose canonical: `internal/config` (`Manager`+`FromEnv`). Deprecate `forge/config` package by making `forge/config/app.go` re-export `internal/config.Config`.
- [ ] 6.2 `internal/config/validator.go:96` `isTLSDatabaseURL` — change to `for _, v := range parsed.Query()["sslmode"] { if in(v, "require","verify-ca","verify-full") return true }` (any-of not exactly-one).
- [ ] 6.3 `internal/config/validator.go:71` add `PANEL_URL https` check mirror of `main.go:134` (url.Parse + `Scheme=="https"` + `Hostname()!=""` + `User==nil`).
- [ ] 6.4 `internal/config/env.go:13` fix `Debug` to `envBool("APP_DEBUG", false)` without `APP_ENV` coupling; add `TrustedProxyCIDRs: splitEnv("TRUSTED_PROXY_CIDRS")`.

**Done when:** `go test ./internal/config -run Validator` covers TLS-with-extra-params + panel_url https.

### Step 7 — B7: Eventstore PublishTx (P0/architecture, largest blast radius)

- [ ] 7.1 `internal/eventstore/store.go` — add `func (s *EventStore) PublishTx(ctx context.Context, tx pgx.Tx, envelope events.Envelope) error` that `Exec` insert on the tx, not pool. Keep old `Publish(ctx, envelope)` for callers not in tx (logs only).
- [ ] 7.2 `internal/eventstore/outbox.go` — add `type TxPublisher struct{ store *EventStore; tx pgx.Tx }` or `PublishTx` method on `OutboxPublisher` that calls `store.PublishTx`. Document at-least-once vs exactly-once trade.
- [ ] 7.3 `internal/store/store.go` — add helper `func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) { return s.db.Begin(ctx) }` (or expose `DB() *pgxpool.Pool` already at `store.go:1192`).
- [ ] 7.4 Sweep every `outboxPub.Publish` call site in `cmd/api/main.go` that is adjacent to a DB mutation: `resMgr` (placement reservation `383`), `sched` placement intents `385`, `cm.ProvisionRecoveredServer` `410`, `opSvc` handlers `553-786`, `deploySvc` `816`, `healthCheckRunner` bridge `840-850`, failover `896-946`, `lbSvc/trafficmanager` `1036` etc. Convert pattern:
  ```go
  tx, _ := db.Begin(ctx)
  if err := doWorkTx(ctx, tx); err != nil { tx.Rollback(); return err }
  if err := outboxPub.PublishTx(ctx, tx, envelope); err != nil { tx.Rollback(); return err }
  if err := tx.Commit(ctx); err != nil { return err }
  ```
- [ ] 7.5 `internal/eventstore/migration.go` — verify type is `*pgxpool.Pool` not `*sql.DB`; change signature if needed (or add adapter `sqlDBFromPool`).
- [ ] 7.6 Decide `Relay` fate: if `registry` remains in-memory (single-process), `Relay` remains dead — delete `eventstore/outbox.go:Relays` code and `main.go:370,1167 eventRelay.Start`. If cross-replica needed, wire at least one consumer (e.g., `observability`) via `Relay.Subscribe` not `eventRegistry.Subscribe(Wildcard)` and add leader election (`queue/leader.go:18` or new `internal/leader`).

**Done when:** `rg 'outboxPub\.Publish\(' cmd/api/main.go` count == number of `PublishTx` sites (no naked Publish adjacent to DB write); `Relay.Subscribe` either has ≥1 prod subscriber or is deleted; `go vet` passes.

### Step 8 — B8: Placement hardening (P1)

- [ ] 8.1 `internal/placement/constraints.go:50` `CheckSoft` — replace `bonus += 1e12 / -= 1e10` with normalized `bonus += 1.0` / `bonus -= 1.0` (or `±0.5`). Add `func normalizeBonus(raw float64) float64 { return math.Max(-10, math.Min(10, raw)) }` then `score+bonus` at `engine.go:58`.
- [ ] 8.2 `internal/placement/engine.go:18` — remove `mu sync.Mutex:18` and `Lock/Unlock:38,80` entirely; make `Place/PlaceAll` stateless (requires `Scorer` and `ConstraintChecker` to be goroutine-safe — verify `Scorer` at `strategy.go` has no mutable state). If checker has mutable cache, make it `sync.RWMutex` local.
- [ ] 8.3 `internal/placement/strategy.go` — add comment documenting score range `base ≤3` and soft bonus range `[-1,+1]` so future values stay bounded.
- [ ] 8.4 Benchmark: `go test ./internal/placement -bench BenchmarkPlace -benchmem` before/after; expect removal of mutex to allow parallel `Place` (run with `-parallel 8`).

**Done when:** `constraints.go` contains no `1e12` / `1e10` literal; `engine.go` has no `sync.Mutex`; load test p50 drops >50% under concurrent placement.

### Acceptance gates (run after each step)

- [ ] `go test ./internal/auth -race -run TestSession`
- [ ] `go test ./internal/store -run TestMigration_Runner -count 1 -tags pgx` (plus `TestEggValidation`, `TestMountAllowlist`)
- [ ] `go test ./internal/http -run TestRateLimit|TestMTLS|TestSecurityHeaders -count 1`
- [ ] `go test ./internal/config -run Validator -count 1`
- [ ] `go test ./internal/eventstore -run TestPublish -count 1`
- [ ] `go test ./internal/placement -bench . -count 1` (for B8)
- [ ] `rg 'fallback-nonce' --glob '*.go' forge/api` returns 0 hits (after B2)
- [ ] `rg '1e12|1e10' internal/placement` returns 0 hits (after B8)
- [ ] `golangci-lint run ./...` 0 new warnings (esp. `gosec` for key perm `0600:daemon/client.go:363`)

---

## 8. Open Questions / Assumptions

- **Daemon pull allowlist (CL-01):** `PullRemoteFile:1135` SSRF guard is inverse (reject private), not allowlist. If beacons are expected to pull from private registries (e.g., `registry.internal:5000`), current reject of `IsPrivate` will block legitimate pulls. Confirm with runtime slice whether private pulls are needed; if yes, add `DAEMON_PULL_PRIVATE_ALLOWLIST` env.
- **Eventstore pool type (Q-04):** `store.GetDB()` at `store.go:1192` returns `*pgxpool.Pool`; `eventstore.New(pool)` at `outbox.go:30` expects `*pgxpool.Pool` (correct after recent refactor). But `eventstore/migration.go:Migrate` legacy signature `Migrate(*sql.DB)` would need adaptor — verify it was already migrated to pgx or add `pgxpool` variant. Without check, `main.go:194` will fail at startup with type mismatch panic.
- **River queue:** `Migrations/057_a_job_queue.sql` creates `job_queue`; `operations` tables are `092_durable_operations.sql`. Both are live. `river_queue` is orphaned `river.go:13`. Do not drop until `queue.Service` vs `operation.Service` single-writer decision is signed (Orchestration slice). Assumed deferred.
- **MTLS env naming:** `cmd/api/main.go:1114-1128` uses `forgecfg.MTLSConfig()` (reads `MTLS_*`) while `daemon/client.go:328` reads `MTLS_ENABLED` not `DAEMON_MTLS_*`. Beacon expects `DAEMON_MTLS_CA_FILE`. Assumed drift is intentional (panel→beacon optional); document mapping in `docs/mTLS.md`.
- **Placement scoring base:** `Scorer` implementation not inspected in this audit beyond `NewScorer(StrategyLeastLoaded)` — assumed base score range ≤3 per `constraints.go:59` comment; verify in `strategy.go` `Score(ctx, Candidate, WorkloadRequest) (float64, []string, error)` implementation before choosing clamp bounds.

---

## 9. Sources

- `audits/FINAL_PARITY_AUDIT.md:1-600` (35 reports synthesized, 180 rows) — §10-11 security findings SE-01..16, §10 orchestration placement/queue, §14 reliability, §15 architecture AF-1..3
- `audits/reverification/synthesis.md:1-132` (20 subagents, ~5.5k lines) — §2 delta table (4 fixed, ~31 still broken), §5 security P0s still open, §6 activation order
- `audits/MASTER_FINDING_INDEX.md:1-150` (index of P0/P1 per slice)
- `audits/reverification/subagent-04-schedules-subusers.md` (F-01 wildcard), `subagent-17-placement-scheduling.md` (SCH-01), `subagent-18-queue-events.md` (Q-01..08), `subagent-20-security-ux.md` (SE-07/08/12)
- Live inspection: `forge/api/go.mod:1`, `cmd/api/main.go:1-2063`, `internal/config/config.go:1-214`, `validator.go:1-120`, `env.go:1-128`, `internal/auth/session.go:1-306`, `scopes.go:1-159`, `remote_hmac.go:1-230`, `internal/http/middleware_ratelimit.go:1-307`, `middleware_mtls.go:1-220`, `middleware_security*.go:1-93`, `internal/daemon/client.go:1-1532`, `internal/eventstore/store.go:1-237`, `outbox.go:1-219`, `internal/placement/engine.go:1-112`, `constraints.go:1-193`, `internal/store/store.go:1-1450`, `migration.go:1-319`, `seeder.go:1-92`, `internal/http/auth.go:1-768`, `internal/http/server.go:1-1391`

---

*End of audit — 110-01-01. No product code modified. File:line citations mandatory where checked.*
