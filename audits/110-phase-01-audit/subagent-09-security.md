# Subagent 09 — Security / Tenancy / RBAC / Secrets / Network Boundaries — Preparation Audit

**Phase:** 110 Phase-01 (10 parallel agents) — Agent 09/10
**Focus:** Security / Tenancy / RBAC / Secrets / Network Boundaries
**Date:** 2026-08-24
**Mode:** Preparation audit — inventory + known P0s. Do not modify code. All citations `file:line` SOURCE_VERIFIED.

**Scope inspected (per task):**
`forge/api/internal/store/permissions.go:89` `store_users.go:544` `store_tenancy.go:91` `domain/domain.go:114` `events/event.go:196` `store_mounts_ext.go:323` `store_secrets.go:45` `secrets/keyring.go:75` `auth/session.go:36` `http/middleware_security.go:22` `http/middleware_security_headers.go:40` `http/middleware_ratelimit.go:98` `http/middleware_ipaccess.go:24` `http/middleware_mtls.go:123` `http/ws_origin.go:95` `http/ws_hub.go:13` `http/handlers_ws_ticket.go:159` `http/realtime.go:20` `store/store_certificates.go:95` `store/store_acme_accounts.go:187` `eventstore/migration.go:19` + `eventstore/store.go:113` + `services/loadbalancer/*`

**Reference parity baseline:**
`audits/FINAL_PARITY_AUDIT.md §11 S01-S16` (30 rows) + `§13 Security Findings P0/P1` + `audits/phase-01/subagent-05-architecture-security.md` (17 comps C01-C17, 5 findings FORGE-05-001..005, O5-06..O5-11) + `audits/final-parity/subagent-10-security-ux.md` (S01-S16 parity matrix) + `audits/FINAL_REFERENCE_ECOSYSTEM_REPORT.md` + `reference/app-platforms/portainer/pkg/libcrypto` + `reference/game-hosting/pterodactyl-panel/app/{Models/Permission.php:18,Services/Servers/GetUserPermissionsService.php:18,Models/Subuser.php}` + `reference/networking/caddy/modules/caddyhttp/server.go:1171` (`determineTrustedProxy`) + `reference/networking/traefik/pkg/middlewares/{ipwhitelist,ratelimiter}`

---

## 1. Inventory — Security Slice Files vs Status

### 1.1 RBAC / Permissions / Subuser

| File | Symbol / Line | Finding | Status | Parity Ref |
|------|---------------|---------|--------|------------|
| `forge/api/internal/store/permissions.go:5-86` | Permission constants 40+ keys, 10 groups, `PermServerView:84` / `PermServerSettings:85` defined | Catalog superset vs Pterodactyl `Permission.php:18` (~45 constants, 10 groups) | **COMPLETE** (superset) | FINAL_PARITY S01, subagent-10 S01 |
| `forge/api/internal/store/permissions.go:89-107` | `AllPermissions():89` returns slice | Omits `PermServerView`/`PermServerSettings` at 84-86 — if ever assigned, `normalizeSubuserPermissions` drops them | **PARTIAL / BUG** P3 | FINAL_PARITY S01 footnote |
| `forge/api/internal/store/permissions.go:196-216` | `HasPermission(permissions, required):206` honors `p=="*"` | Correct for owner (`*` computed at request time in Pterodactyl `GetUserPermissionsService.php:18`). Dangerous because store now persists `*` | **BROKEN-by-caller** | FINAL_PARITY SE-01, GH-18 |
| `forge/api/internal/store/permissions.go:218-228` | `IsSensitivePermission:221` gates `database.view_password`, `server:read-env` | Used to mask secrets in responses | **COMPLETE** | subagent-10 S01 |
| `forge/api/internal/store/store_users.go:306-344` | `UpsertServerSubuser:306` calls `normalizeSubuserPermissions(req.Permissions):311`, checks owner!=subuser at 330, no actor-subset check | Any holder of `user.create`/`user.update` (handler gate `handlers_servers.go:541,574`) can grant arbitrary perms including `*` → full control | **BROKEN P0** | FINAL_PARITY GH-18/SE-01/SE-02, §13 row 1 |
| `forge/api/internal/store/store_users.go:555-572` | `normalizeSubuserPermissions:555` allowlist `defaultSubuserPermissions():574` 40+ keys at 575-588, special-case `permission!="*"` at 564 `if (!allowed[perm] && permission!="*") continue` | Persists `*` for subusers; Pterodactyl never stores `*` for subusers — owner gets `['*']` computed, never persisted | **BROKEN P0** | subagent-10 S02, FINAL_PARITY S02/S03 |
| `forge/api/internal/store/store_users.go:574-588` | `defaultSubuserPermissions:574` includes `backup.delete` but not `backup.download/restore`, includes `allocation.update` but not `create/delete`, includes `file.sftp` etc. | Asymmetric vs `permissions.go:89` catalog — some AllPermissions keys absent, some default keys absent from AllPermissions | **PARTIAL** inconsistency | subagent-10 S02 |
| `forge/api/internal/store/store_users.go:377-407` | `UserCanAccessServer:377` owner/admin true, subuser `HasPermission` for non-empty, baseline `permission==""` → `len>0` at 401-404 | Correctly blocks zero-perm rows. Comment at 206-216 warns `HasPermission("", "") == false` | **COMPLETE** | FINAL_PARITY SE-15 |
| `forge/api/internal/http/handlers_servers.go:541,574` (inferred) | `POST /servers/:id/users` `requireServerPermission(PermUserCreate)` / `PATCH` `PermUserUpdate` | Handler gates only check caller has `user.create`, not that caller owns every granted perm | **BROKEN** (missing subset enforcement) | FINAL_PARITY S03 |
| `forge/api/internal/store/store_tenancy.go:91` | `ListOrganizationsForUser:91`, `GetTeamMemberRole:427` | Org membership read | **COMPLETE** | subagent-10 S04 |
| `forge/api/internal/store/store_tenancy.go:539-548` | `ResolveEffectiveOrgRole:539` `if globalRole=="admin" return "admin"` | Global admin bypass — same at `UserCanAccessServer:378` and `UserCanAccessOrgResource:645` | **PARTIAL** — intended but widens S04 tenant-blind issue | subagent-05 C11 |
| `forge/api/internal/store/store_tenancy.go:645-653` | `UserCanAccessOrgResource:645` admin true, else `UserIsOrgMember` + `ServerBelongsToOrg` | No per-server team ACL; Portainer `ResourceControl:TeamAccesses[]/UserAccesses[]` per-resource grant missing — membership is all-or-nothing per org | **PARTIAL** | subagent-05 C11, FINAL_PARITY S04 |

### 1.2 Tenancy / Placement / Events (Tenant-Blind Core)

| File | Symbol / Line | Finding | Status |
|------|---------------|---------|--------|
| `forge/api/internal/domain/domain.go:114-129` | `PlacementRequest:114` fields `ServerID, RegionID, Region, PreferredNode, RequiredNode, NodeID, AllocationID, SkipReservation, StorageLocality, MemoryMB, CPUShares, CPU, DiskMB, Runtime` — **no `TenantID`/`OrgID`/`ProjectID`/`EnvID`** | Tenant-blind scheduling: `scheduler/service.go:88` `PlaceServer` ranks global `ListNodes`; `recovery/service.go:276` enumerates all servers of a node; affinity influences fleet-wide capacity | **PARTIAL P1** FINAL_PARITY S04 / §13 SE-03, subagent-10 S04 FORGE-LOGIC-S02 |
| `forge/api/internal/domain/domain.go:149-157` | `PlacementDecision:149` `RegionID, NodeID, AllocationID, ReservationID, Manual, Score, Reasons` — no tenant | Same | **PARTIAL P1** |
| `forge/api/internal/events/event.go:196-205` | `Envelope:196` `ID, Type, Timestamp, Source, ResourceType, ResourceID, CorrelationID, Payload` — **no `TenantID`/`OrgID`** at 196-205 | Audit trails cannot be partitioned; `eventstore/migration.go:19-31` `events` table has no tenant column | **PARTIAL P1** FINAL_PARITY SE-03, subagent-10 S16 |
| `forge/api/internal/eventstore/migration.go:19-31` | `events` DDL at 19: `id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched, failure_count, last_error` + indexes 32-39, `claimed_by:36` `claimed_until:37` | Tenant column missing; `OutboxPublisher:205-222` `Publish` does DB insert then in-memory `registry.Publish` without `PublishTx` — publish-after-commit not atomic, cross-instance delivery write-only | **PARTIAL** FINAL_PARITY §14 Arch AF-1 |
| `forge/api/internal/eventstore/store.go:67-93` | `ClaimPending:67` `claimed_by=$3 || ':' || e.id` + `claimed_until NOW()+$4` | `:` in `claimToken` not escaped; `MarkFailed:165` `claimed_by=NULL` allows duplicate delivery to another relay polling `Pending:135` with `FOR UPDATE SKIP LOCKED` but `pendingQuery` bypass at 68-70 | **PARTIAL** subagent-05 O5-06 |
| `forge/api/internal/store/store_tenancy.go:550-599` | `ListServersForOrg:552`, `ServerBelongsToOrg:610`, `BackupBelongsToOrg:616`, `DeploymentBelongsToOrg:630` | Org-scoped queries exist for servers/backups/deployments but placement/events/broadcasts bypass them | **PARTIAL** |

### 1.3 Mount / Host Breakout

| File | Symbol / Line | Finding | Status |
|------|---------------|---------|--------|
| `forge/api/internal/store/store_mounts_ext.go:323-339` | `validateMountPath(value, field):323` checks `path.IsAbs:324`, `Clean==value`, no `\`, no `..` at 327-331; then `field=="source"` only blocks `value ∈ {/etc/forge, /var/lib/forge/volumes}` at 332-333; `field=="target"` only blocks `{/ , /home/container}` at 335 | Syntactically strong but semantically narrow — exposes host via `source=/etc`, `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/`, any `/var/lib/forge` except 2 entries. Admin `mounts.write` + double-join `ensureMountAvailableForServer:297-314` (`mount_node ∧ egg_mount`) still yields breakout if attacker controls both pivots (both admin ops) | **BROKEN P0** FINAL_PARITY GH-19/SE-04, §13 row 3 |
| `forge/api/internal/store/store_mounts_ext.go:297-314` | `ensureMountAvailableForServer:297` `EXISTS (mount_node ∧ egg_mount)` | Correct double-join but does not re-validate `validateMountPath` on beacon side workload creation — beacon `AllowedMountSourcesForNode:367` only lists sources, no prefix check at runtime | **PARTIAL** |
| `forge/api/internal/store/store_mounts_ext.go:367-388` | `AllowedMountSourcesForNode:367` `SELECT DISTINCT m.source WHERE mn.node_id=$1` | Returns raw sources, no filtering | **COMPLETE** helper |

### 1.4 Secrets / Keyring / AAD

| File | Symbol / Line | Finding | Status |
|------|---------------|---------|--------|
| `forge/api/internal/secrets/keyring.go:29-42` | `ParseKey:29` accepts 64-char hex vs base64 `Strict().DecodeString:37` 32B | 64-char base64 string that happens to be valid hex misinterpreted as hex (O5-10). `New:44` validates `activeID` not containing `:` at 46 | **PARTIAL P3** |
| `forge/api/internal/secrets/keyring.go:75-94` | `Encrypt(plaintext, aad):75` `aes.NewCipher:79` `cipher.NewGCM:83` nonce `gcm.NonceSize()` at 87, `gcm.Seal(nil, nonce, plaintext, []byte(aad)):91`, envelope `forge:v1:<keyID>:<base64url nonce+ciphertext>` at 93 | Primitive correct (AES-256-GCM per-value nonce). AAD binding is **caller-dependent, unvalidated** — call sites vary (`store_secrets.go:45` `secretAAD(table+":"+id+":"+field)`, `store_envvars.go:274` passes `""`) | **PARTIAL P2** FINAL_PARITY SE-11, subagent-05 C13 |
| `forge/api/internal/secrets/keyring.go:96-132` | `Decrypt(envelope, aad):96` validates prefix `forge:v1:` at 100, splits `keyID:payload` at 104, base64 decode at 112, `gcm.Open(payload[:NonceSize()], payload[NonceSize():], aad)` at 127 | Authentication fails only on decrypt if AAD mismatched — `NeedsRotation` won't catch cross-AAD misuse | **PARTIAL** |
| `forge/api/internal/secrets/keyring.go:134-136` | `NeedsRotation:134` `!HasPrefix(envelope, "forge:v1:"+activeID+":")` | Only checks key-ID rotation, not AAD namespace. Ciphertext encrypted with different AAD but same keyID deemed not needing rotation yet fails on next decrypt | **PARTIAL** subagent-05 C13 |
| `forge/api/internal/store/store_secrets.go:18-45` | `encryptSecret:18` early `plaintext=="" → ""` at 19, `decryptSecret:28` `envelope=="" → plaintext` at 30 (legacy plaintext fallback until migration commits), `secretAAD:45` `table+":"+id+":"+field` | Correct envelope shape docs `forge/docs/encryption-at-rest.md:4`. `MigrateOperationalSecrets:50` idempotent rotation across ~14 tables + `migrateExpandedSettings:619` but **excludes** `dns_provider_accounts.credentials` and `certificates.dns_credentials` | **PARTIAL P1** gap below |
| `forge/api/internal/store/store_certificates.go:86-97` | `CreateCertificate:86` `encryptSecret(private_key)` with AAD `certificates:id:private_key` at 86, but `dns_provider:97` + `dns_credentials:97` stored as raw `req.DNSProvider, req.DNSCredentials` (`map[string]string→JSONB`) | Leaf + ACME keys encrypted, DNS zone tokens **plaintext JSONB** | **BROKEN P1** FINAL_PARITY NG-18/SE-18 |
| `forge/api/internal/store/store_acme_accounts.go:232-235` | `CreateDNSProviderAccount:222` `INSERT ... credentials $4` with `req.Credentials json.RawMessage` at 232-235; `Update:256-258` same; `List:187` `SELECT credentials` raw | **Plaintext DNS API credentials** — `dns_provider_accounts.credentials` JSONB holds provider tokens (Cloudflare, Route53, etc.) unencrypted vs `acme_accounts.private_key_encrypted` correctly encrypted at 86-93 | **BROKEN P1** migrations `134,135` vs encrypted `157` |
| `forge/api/internal/store/store_certificates.go:116,139,258` | `List/Get:116` `COALESCE(dns_credentials,'{}'::jsonb)` returned verbatim, no decrypt; `FindExpiring:258` same | API returns plaintext tokens to any `certificate.read` caller; `ListCertificates:135-189` selects `dns_credentials` without masking | **BROKEN P1** |
| `forge/api/migrations/134_certificate_dns_provider.sql:1-2` | `ALTER TABLE certificates ADD COLUMN dns_credentials JSONB` | Plaintext column, no `*_encrypted` counterpart | **BROKEN** |
| `forge/api/migrations/135_acme_accounts.sql:11-18` | `dns_provider_accounts.credentials JSONB` | Same — should be `credentials_encrypted TEXT` + legacy plaintext migration | **BROKEN** |
| `forge/api/migrations/096_dns_providers.sql:1-6` | `dns_providers.credentials TEXT`, `credentials_encrypted TEXT` | Legacy table has both columns but new `dns_provider_accounts` regressed to plaintext-only | **PARTIAL** historical gap |

### 1.5 Session / Auth

| File | Symbol / Line | Finding | Status |
|------|---------------|---------|--------|
| `forge/api/internal/auth/session.go:36-41` | `InMemorySessionStore:36` `sessions map[string]*Session:38`, `byToken map[string]string:39`, `byUser map[string][]string:40` + `NewInMemorySessionStore:43` | Stores raw pointers + raw token keys | **BROKEN P1** subagent-05 C14, FORGE-05-002 |
| `forge/api/internal/auth/session.go:51-65` | `Create:51` `s.sessions[session.ID]=session:62` `s.byToken[session.Token]=session.ID:63` `s.byUser[...] append:64` — stores caller's `*Session` pointer directly, no clone | Concurrent `SessionMiddleware` receives same `*Session` ref, mutates `LastActiveAt` outside lock at `session.go:261` — data race (`go test -race` detectable) | **BROKEN** |
| `forge/api/internal/auth/session.go:68-80` | `Get:68` `RLock` at 69, returns `sess` pointer at 79; `GetByToken:82` same at 84,97 | Returns same pointer without clone — half-written `LastActiveAt` visible under `RLock` while `Update` holds `Lock` | **BROKEN** |
| `forge/api/internal/auth/session.go:100-113` | `Update:100` `Lock` at 105, only checks `_, ok := s.sessions[session.ID]` at 108 then `s.sessions[session.ID]=session:111` — does not update `byToken` if `sess.Token` mutated, leaves stale key | Stale `byToken` + no `sha256Hex` hashing | **BROKEN** |
| `forge/api/internal/auth/session.go:231-269` | `SessionMiddleware:231` extracts cookie `__Host-forge_session` or `Authorization: Bearer ` at 233-238, skip via `skip(token)` at 245, `GetByToken:249`, checks `ExpiresAt` at 255, `sess.LastActiveAt=Now():261` then `store.Update(sess):262` | Mutates `sess.LastActiveAt` without holding store lock — race. Missing `byToken` hash means heap/pprof dump leaks bearer tokens (32B → 43-char base64url at `GenerateSessionToken:196`) | **BROKEN** subagent-05 C14 |
| `forge/api/internal/auth/session.go:196-210` | `GenerateSessionToken:196` `rand.Read 32` → `base64.RawURLEncoding 43 chars`, `GenerateSessionID:204` 16B | Strong entropy | **COMPLETE** |
| `forge/api/internal/store/store_users.go:914-937` | Durable `user_sessions` stores `session_token_hash` (SHA-256 hex) at 914-931 — in-memory path does opposite (raw) | Inconsistency — durable path correct, in-memory weaker | **DUPLICATE** |
| `forge/api/internal/http/server.go:1995` etc. (`http/auth.go:412` reference) | `jti` hashed via `sha256Hex(claims.JTI)` at `http/auth.go:412` | Shows pattern exists for JWT jti but not for opaque session token | **REFERENCE** for fix |

### 1.6 Middleware — Network Boundaries / Hardening

| File | Symbol / Line | Finding | Status | Ref Model |
|------|---------------|---------|--------|-----------|
| `forge/api/internal/http/middleware_security.go:19-25` | `generateNonce():19` `rand.Read 16` → `base64 StdEncoding` else `return "fallback-nonce":22` | Fallback deterministic nonce + `strict-dynamic` at 33 `script-src 'self' 'nonce-'+nonce 'strict-dynamic'` → injected `nonce="fallback-nonce"` scripts execute — worse than no CSP (publicly known nonce) | **BROKEN P1** FINAL_PARITY SE-08 |
| `forge/api/internal/http/middleware_security.go:27-62` | `SecurityHeaders(env):27` always sets `CSP+HSTS+XFO` at 31-49, `X-CSP-Nonce` at 43, `Permissions-Policy: geolocation=(),... payment()` at 48-49, HSTS `preload` without verifying domain on preload list, `report-uri /api/v1/csp-report` dead | Two overlapping middlewares must stay in sync (different `Permissions-Policy` values) | **PARTIAL** subagent-05 O5-07 |
| `forge/api/internal/http/middleware_security_headers.go:40-46` | `generateMiddlewareNonce():40` same fallback at 43 | Same `fallback-nonce` bug — duplicate code path | **BROKEN P1** FINAL_PARITY S09 |
| `forge/api/internal/http/middleware_security_headers.go:48-93` | `SecurityHeadersMiddleware(cfg):48` configurable variant, `CSPValue:34` with `{NONCE}` placeholder, injects at 71-79, `Permissions-Policy: geolocation=(), microphone=(), camera=():89` vs `payment()` etc. in other middleware | Inconsistent `Permissions-Policy` disables `payment()` may break Stripe/PayPal if shared ingress | **PARTIAL** |
| `forge/api/internal/http/middleware_ratelimit.go:95-119` | `ExtractClientIP(c):98` `peer:=c.IP():99`, `peerIP:=net.ParseIP(peer)` at 100, `if peerIP==nil || !(IsLoopback()\|\|IsPrivate()\|\|IsUnspecified()) return peer:101` else trust `X-Forwarded-For` right-most at 104-112, then `X-Real-IP` at 114-117 | Trusts XFF from **any** private peer — any RFC1918 container/LAN host can rotate rate-limit/IP-access keys by injecting `X-Forwarded-For`. Correct model: Caddy `determineTrustedProxy server.go:1171` explicit CIDR allowlist + strict levels `TrustedProxiesStrict:224`, Traefik `ipWhitelist:31` fails closed if `sourceRange` empty. Also `RateLimitConfig.TrustedIPs:27` fully bypasses limiting at 152, combined with #3 trust → spoofable behind private proxies. Observability gap: `middleware_request_logging.go:70` uses raw `c.IP()` while rate limiter uses `ExtractClientIP` — two client-IP notions | **BROKEN P1** FINAL_PARITY SE-07, subagent-10 S08 |
| `forge/api/internal/http/middleware_ratelimit.go:121-137` | `isTrustedIP:121` CIDR vs exact match | Correct helper, but fed by env `RATE_LIMIT_TRUSTED_IPS:288` which is empty by default → no bypass unless set — good | **COMPLETE** helper |
| `forge/api/internal/http/middleware_ratelimit.go:288-300` | `trustedIPsFromEnv():288` parses `RATE_LIMIT_TRUSTED_IPS` | Empty default = no bypass — ok | **COMPLETE** |
| `forge/api/internal/http/middleware_ipaccess.go:22-41` | `IPAccessControl(cfg):22` `getClientIP:25` → deny list first at 28, allow list at 34, `isIPInList:54` handles CIDR + exact | Logic correct but `AdminIPAccessConfig:98` + `APIIPAccessConfig:118` default `TrustProxy:true` at 105,125 and **warn-only** when env vars unset `ADMIN_IP_ALLOW/DENY, API_IP_ALLOW/DENY` at 108-113,128-132 — fail-open. Combined with `ExtractClientIP` peer-private trust → any container can spoof past IP allowlist | **BROKEN** FINAL_PARITY SE-07, subagent-10 S08 FORGE-LOGIC-S04 |
| `forge/api/internal/http/middleware_ipaccess.go:43-51` | `getClientIP(c, trustProxy):43` `if trustProxy return ExtractClientIP(c):46 else c.IP():50` | Delegates to vulnerable `ExtractClientIP` when true | **BROKEN** |
| `forge/api/internal/http/middleware_mtls.go:123-125` | `MTLSAuthMiddleware:77` gate `if c.Protocol()!="https" && c.Get("X-Forwarded-Proto")!="https" return 400:123` | Trusts client `X-Forwarded-Proto: https` without CIDR gate — weakens defense-in-depth. Correct: Caddy `determineTrustedProxy:1197` forwarded headers honored only from configured CIDR. Cert verification still requires real `*tls.Conn` at 127-158 so not full bypass, but false assurance behind untrusted hops. Also `DevBypass:93-100` correctly panics in production `isProductionMTLSEnvironment:73` — that part is solid | **PARTIAL P1** FINAL_PARITY SE-13 |
| `forge/api/internal/http/middleware_mtls.go:142-183` | `Verify:150-158` `clientCert.Verify` with `Roots+Intermediates`, `verifyMTLSRevocation:159` delegates to `RevocationLookup`, identity `forge-node` URI → `DNSNames[0]` → `CN` at 168-183 | Correct chain + revocation, SAN preference good | **COMPLETE** |
| `forge/api/internal/http/ws_origin.go:57-89` | `isCookieAuthRequest:57` checks `authSource` locals `authSourceCookieSession` at 62 vs `authSourceAPIKey/OAuth` at 65, else fallback to `Authorization: Bearer ` at 72, `forge_session` cookie at 78-87 | Correctly distinguishes cookie vs bearer for missing-Origin handling. Backwards-compatible but `Cookie` header contains substring check at 85 is loose | **PARTIAL** |
| `forge/api/internal/http/ws_origin.go:91-121` | `validateWebSocketOrigin:95` `if origin=="" { if isCookieAuth 403 else nil }` at 97-101, `originFromURL` normalize at 103, wildcard `*` at 109-113, `isOriginAllowed` at 114-118 | For `!isCookieAuth` (Bearer/ticket) missing Origin allowed — Traefik/Portainer validate Origin for all WS upgrades regardless. Moderate vs reference stricter. `ws_hub.go:13-49` intentionally NOT subscribed in `main.go` to avoid cross-tenant leakage — WS hub precise but **unwired** | **PARTIAL P2** FINAL_PARITY SE-14/SE-15, subagent-05 C15 |
| `forge/api/internal/http/ws_origin.go:148-166` | `wsUpgraderOrigins(allowed):157` permits `""` always for token clients at 164-165, early `*` return at 159-161 | `""` (no Origin) always permitted for token clients → compromised API key can upgrade without Origin, bypassing `wsOriginMiddleware` at 125-137 in `realtime.go:124-147` second check inside `realtimeProxy` still runs but same `isCookieAuth` false → allows missing Origin again | **PARTIAL** subagent-05 O5-09 |
| `forge/api/internal/http/realtime.go:20-113` | `getWebSocketAllowedOrigins(cfg):29` merges `API_WS_ALLOWED_ORIGINS` + `CORS AllowedOrigins` + `PanelURL/PANEL_URL/PANEL_API_URL`, normalizes via `originFromURL`, dedupes, fallback `localhost:3000/3002` in non-prod at 87-95, strips `*` in prod at 98-111 | Correct production `*` stripping at 98, localhost defaults only in non-prod — good | **COMPLETE** |
| `forge/api/internal/http/realtime.go:124-347` | `realtimeProxy:124` enforces `validateWSConnOrigin` at 143, ticket inspection at 166-174, `checkServerPermission` `websocket.connect` at 237, `control.console` for `console` stream at 250-259, consumes ticket only after binding at 262, upstream HMAC at 286, `daemon.MintWebsocketToken` at 295 | Strong ticket + permission binding, rate-limit 10/s at 339, pong handling etc. | **COMPLETE+** |
| `forge/api/internal/http/handlers_ws_ticket.go:159-198` | `IssueWSTicket:159` `checkServerPermission("websocket.connect")` at 172, `rand 16 → hex` ticket at 177-183, HMAC `forge:ws-ticket:v1\x00` at 254-258, 60s expiry | Correct short-lived single-use ticket with Lua `consumeScript:19` `get+del` atomic at 81-84 | **COMPLETE** |
| `forge/api/internal/http/ws_hub.go:13-49,83-143` | `ws_hub.go:13` header explicitly documents hub intentionally NOT subscribed in `main.go` to avoid cross-tenant leakage; `shouldDeliver:199` filters per-user | Precise tenancy filter but **unwired** — real notifications fall back to 5s polling `handlers_notifications_websocket_test.go` | **UNWIRED** subagent-05 C15 |

### 1.7 Load Balancer / L4 Probe

| File | Line | Finding | Status |
|------|------|---------|--------|
| `forge/api/internal/http/handlers_loadbalancer.go:84-105` | `POST /groups/:id/targets` at 84 accepts `req.IP` trimmed at 96, validates `req.ServerID=="" \|\| req.IP=="" \|\| Port<1..65535 \|\| Weight<1` at 97, then `svc.AddTarget:100` with no `net.ParseIP`, no private/metadata range rejection | Any LB-manager (`requireRole admin` + `loadbalancer.write`) can probe/map internal `IP:port` reachability (status observable via API) and use listeners as one-hop TCP relays into management nets including cloud metadata `169.254.169.254`. `trafficmanager/service.go:388` validates `TargetHost` more strictly — LB path lacks it | **BROKEN P1** FINAL_PARITY SE-10, L4 probe |
| `forge/api/internal/services/loadbalancer/service.go:322-363` | `AddTarget:322` `ID fmt.Sprintf("t-%s-%s-%d")` at 332, no IP validation, stores at 344 | Same | **BROKEN** |
| `forge/api/internal/services/loadbalancer/dataplane.go:130-178` | `proxyTCP:130` dials `target.IP:port` per connection at 142 `DialContext("tcp", IP:Port)` & health check `dataplane.go:317` `DialContext` per 2s | Relays raw bytes both ways at 156-172, `ReleaseConnection` at 272 | **BROKEN** (probe/relay primitive) |
| `forge/api/internal/services/loadbalancer/dataplane.go:296-325` | `runHealthChecks:296` walks all targets every 2s, TCP dial 2s timeout at 317, preserves `Draining` at 310-312 | No auth, no isolation; health failure marks `Unhealthy` but `reconcileListeners` re-adds? Actually `setTargetStatus` persists — ok | **PARTIAL** |

### 1.8 Audit / Handlers Security Gap

| File | Line | Finding | Status |
|------|------|---------|--------|
| `forge/api/internal/http/handlers_*` | `grep AppendAudit` finds zero callers in `handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_certificates.go`, `failover/service.go`, `handlers_firewall.go` | Network mutations (highest blast-radius surface) **unaudited**; game/app tenancy mutations audited via `AppendAudit` at `store_users.go:275`, `store_tenancy.go:165`, `store_envvars.go:143` | **MISSING P1** FINAL_PARITY S16, subagent-10 S16 |
| `forge/api/internal/http/handlers_security*.go` | **No file matches** `handlers_security*.go` glob — security_headers table exists but handlers for `/admin/security` traffic policies silent | `store_proxy_domains.go:13`, `store_security_headers.go`, `store_redirect_rules.go` API-only CRUD, no audit | **MISSING** |
| `forge/api/internal/models/audit_log.go`, `store/store_audit.go`, `store/store_audit_logs.go`, `http/handlers_auditlog.go` | Audit plumbing exists for game/app but not wired to networking | NPM `audit-log.js:84` `internalAuditLog.add{user_id, action, object_type/id, meta}` for every mutation — reference | **PARTIAL** |

---

## 2. Known P0s — Consolidated (from FINAL_PARITY §13 + §11 + Subagent-05)

| # | Title | File:line | Severity | Parity ID |
|---|-------|-----------|----------|-----------|
| **P0-1** | **Subuser wildcard `*` persistable + subset-check missing** — any `user.create` holder can grant `*` or any perm outside own grants (e.g. `file.read` holder → `database.view_password`, `schedule.delete`, `settings.reinstall`) → full control | `store/store_users.go:311` `UpsertServerSubuser`, `store/store_users.go:555-564` `normalizeSubuserPermissions` (`permission!="*"`), `store/permissions.go:206` `HasPermission` (`p=="*"`), `http/handlers_servers.go:541` gate only `user.create` | **P0 escalation** | GH-18/SE-01/SE-02, §13 row 1, subagent-10 S02/S03 |
| **P0-2** | **Mount allowlist too narrow** — only blocks `/etc/forge` + `/var/lib/forge/volumes` at `store_mounts_ext.go:332`; misses `/etc` parent, `/var/run/docker.sock`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/` → host breakout via compromised admin (admin `mounts.write` + `mount_node`∧`egg_mount` double-join) | `store/store_mounts_ext.go:323-339` | **P0 host breakout** | GH-19/SE-04, §13 row 3 |
| **P0-3** | **All traffic policies apply to all routes (cross-tenant ACL leak)** — `collectApplicablePolicies` returns entire map (`traefik_proxy.go:960` `caddy_proxy.go:722` "For now, return all policies…"); no `rule↔policy` join in `038_traffic_routing.sql`/`083_a_traffic_rules.sql`. One tenant's `ip_whitelist/blacklist, rateLimit, circuitBreaker` restricts/unlocks another's domain | `services/trafficmanager/traefik_proxy.go:960`, `caddy_proxy.go:722,771-793` | **P0 cross-tenant** | NG-09/SE-09, §13 row 6 |
| **P0-4** | **Backup sidecar unauthenticated + nil AAD → cross-server transplant** — `beacon/local.go:757` unkeyed SHA, `services/backup/encryption.go:92:119` nil salt + nil AAD, `local.go:238` plaintext; `verification.go:31` trusted | `beacon/local.go:238,757`, `encryption.go:92` | **P0 transplant forgery** | BK-03/SE-BK, §13 row 10 |
| **P0-5** | **Phantom runtime (LXC/KVM)** — control 7 names vs beacon 5; `beacon/server.go:740-772,841` drops Provider → always Docker, `factory.go:19-31` capability dead `CheckCapability` zero callers | `beacon/server.go:740`, `store` | **P0 phantom** | RT-17/R-01/SE-16, §13 row 2 |

**P1s relevant to this slice (network boundaries / secrets / session / L4):**

| # | Title | File:line | Parity ID |
|---|-------|-----------|-----------|
| P1-a | Plaintext DNS API credentials `dns_credentials`/`dns_provider_accounts.credentials` JSONB | `store/store_certificates.go:95`, `store/store_acme_accounts.go:232`, `migrations/134:1`, `135:11` | NG-18/SE-18, §13 row 5 |
| P1-b | Trusted proxy fail-open — `ExtractClientIP:98` trusts XFF from any private peer (Caddy requires CIDR) + `XFP` in mTLS gate `middleware_mtls.go:123` + `TrustProxy:true` warn-only `middleware_ipaccess.go:105,125` | `http/middleware_ratelimit.go:98`, `http/middleware_mtls.go:123`, `http/middleware_ipaccess.go:46,98` | SE-07, §13 row 4 |
| P1-c | CSP nonce fallback `fallback-nonce` with `strict-dynamic` | `http/middleware_security.go:22`, `http/middleware_security_headers.go:43` | SE-07 (table lists CSP as row 11), §13 row 11 |
| P1-d | L4 LB as internal-network probe/relay | `http/handlers_loadbalancer.go:88`, `services/loadbalancer/service.go:322`, `dataplane.go:142,317` | SE-10, §13 row 9 |
| P1-e | Session pointer race + raw token keys + stale `byToken` | `auth/session.go:36,51,68,100,231,261` | SE-12, §13 row 12 |
| P1-f | Keyring AAD caller-dependent no namespace validation + `ParseKey` hex/base64 confusion | `secrets/keyring.go:29,75`, `store/store_secrets.go:45` | SE-11, §13 row 13, subagent-05 C13/O5-10 |
| P1-g | DNS-provider SSRF (`PDNS_API_URL` zero validation + process-global env mutation) | `services/dns/service.go:489-498,639-661` | SE-16, §13 row 8 |
| P1-h | Traefik rule injection via unescaped backticks in `Path` | `services/trafficmanager/traefik_proxy.go:797` vs `service.go:376` | SE-14, §13 row 7 |
| P1-i | Audit logging silent for traffic/cert/LB/firewall mutations | handlers_* grep zero `AppendAudit` | S16, subagent-10 S16 |
| P1-j | Tenant-blind placement/events/scheduling (fleet-global) | `domain/domain.go:114`, `events/event.go:196`, `eventstore/migration.go:19` | S04/SE-03, subagent-05 C11 |

---

## 3. Deep Dive — Required Fix Design (Do Not Implement — Preparation Only)

### 3.1 Fix F1 — Subset Enforcement + Wildcard Gate (P0-1)

**Current behavior verified:**
- `store_users.go:311` `permissions := normalizeSubuserPermissions(req.Permissions)` — allowlist only, no actor-subset.
- `store_users.go:564` `(!allowed[permission] && permission!="*")` — `*` always passes allowlist.
- `permissions.go:206` `if p=="*" || p==required return true` — honors `*` for any subuser row containing it.
- `HasPermission` correctly documents at 196-205 "wildcard is honored only as explicit assignment (owner) – callers must ensure wildcard is only ever assigned to server owners or admins via normalized permission checks" — but no caller enforces it.
- Pterodactyl reference `GetUserPermissionsService.php:18` returns `['*']` **computed at read time** for `owner`/`root_admin`, never persisted in `subusers.permissions`; `SubuserController.php:154-168` `getDefaultPermissions()` intersects request with allowlist and always injects `websocket.connect`.

**Required shape (two-layer):**

1. **Store layer** `store_users.go:306` change signature to require actor binding:
   ```go
   // Before:
   func (s *Store) UpsertServerSubuser(ctx context.Context, serverID string, req UpsertServerSubuserRequest, actorID *string) (ServerSubuser, error)
   // After (preparation spec):
   func (s *Store) UpsertServerSubuser(ctx context.Context, serverID string, req UpsertServerSubuserRequest, actor *ActorPerms) (ServerSubuser, error)
   type ActorPerms struct { ID string; Role string; IsOwner bool; Perms []string } // Perms = effective perms (owner/admin → ["*"])
   ```
   Inside: load actor effective perms via `GetServerSubuser` or `["*"]` if `owner||admin`; then reject any `req.Permissions` not in `actorPerms` unless `actorIsOwner||actorIsAdmin`; gate `"*"` behind `actorIsOwner||actorIsAdmin` alone; remove `permission=="*"` escape in `normalizeSubuserPermissions:564` for subuser path (keep `HasPermission:211` wildcard for owner read).

2. **Handler layer** `handlers_servers.go:541` / `:574` PATCH — resolve actor before store call:
   ```go
   actorPerms, err := resolveActorPerms(ctx, serverID, actorUserID, actorRole)
   if err != nil { return err }
   // reject outside-subset before calling store
   ```
   Add test matrix: `file.read` holder cannot grant `database.view_password`; `user.create` alone cannot grant `*`; owner can grant `*`; admin can grant any.

3. **Normalization hardening** — ensure `permServerView:84` / `permServerSettings:85` added to `AllPermissions:89` or removed from defs; unify `defaultSubuserPermissions:574` vs `AllPermissions` (see 1.1 table).

**Verification plan:** Unit test `POST /servers/:id/users` with `user.create` but not `database.view_password` → grant `database.view_password` must 403; same with `*` → 403; owner → 200. `go test -run TestUpsertServerSubuser_SubsetEnforcement -race`.

**Blast radius if deferred:** Any low-priv subuser with `user.create` (common for team leads) escalates to full control (`database.view_password`, `buildpack.manage` → RCE via build job, `cron.create` → node shell).

---

### 3.2 Fix F2 — Mount Allowlist Prefix / Deny-List Expansion (P0-2)

**Current:** `validateMountPath:323` only `value ∈ {/etc/forge,/var/lib/forge/volumes}` blocked for source, `{/ , /home/container}` for target.

**Threat:** `source=/etc` exposes `/etc/shadow` parent; `/var/run/docker.sock` breaks container isolation; `/proc`/`/sys` leak host; `/` gives full host FS; `/var/lib/forge/volumes` narrow block misses `/var/lib/forge` sibling dirs.

**Fix — allowlist (preferred) or deny-list prefix (minimal):**

- **Minimal deny-list expansion** at `store_mounts_ext.go:332-333`:
  ```go
  if field == "source" {
    if isReservedSource(value) { return err } // block prefixes
  }
  func isReservedSource(v string) bool {
    blockedPrefixes := []string{"/etc","/var/run","/proc","/sys","/dev","/boot","/root","/var/lib/forge"}
    allowedBases := []string{"/srv/forge-mounts"} // operator-configured allowlist base
    // If allowlist mode: require HasPrefix(allowedBase) else deny.
    // Deny-list mode: if any blockedPrefix == v || strings.HasPrefix(v, blockedPrefix+"/") → deny
  }
  ```
  Or switch to **allowlist** via env `MOUNTS_ALLOWED_PREFIX` (default `/srv/forge-mounts/`) — validate `strings.HasPrefix(source, allowedBase+"/") || source==allowedBase`.

- **Beacon re-validation** at workload creation `mounts.go:13` `runtimeMounts EvalSymlinks+Rel within allowedMounts` — tighten to same prefix set; also validate at `daemon`/`beacon` container create path.

- **Test:** `CreateMount Source=/etc → error`, `=/var/run/docker.sock → error`, `=/proc → error`, `=/ → error`, `=/srv/forge-mounts/appdata → ok`.

**Risk if allowlist too strict:** Legit admin mounts like `/srv/forge-shared` would be rejected — gate via env `MOUNTS_ALLOWED_PREFIXES` comma-list.

---

### 3.3 Fix F3 — Encrypt DNS Credentials (P1-a)

**Current:** `store_certificates.go:95` `dns_credentials JSONB` plaintext; `store_acme_accounts.go:232` `credentials JSONB` plaintext. Leaf private keys correctly at `store_certificates.go:86` `secretAAD("certificates", id, "private_key")`; ACME account keys at `store_acme_accounts.go:86` same. `migrations/134:1` + `135:11` create plaintext columns, no `*_encrypted` counterpart (vs `096_dns_providers.sql:6` which has `credentials_encrypted`).

**Fix:**

1. **Schema:** Add columns + migration `156_encrypt_dns_credentials.sql`:
   ```sql
   ALTER TABLE certificates ADD COLUMN IF NOT EXISTS dns_credentials_encrypted TEXT NOT NULL DEFAULT '';
   ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '';
   ```

2. **Store:** Update `store_certificates.go:CreateCertificate:86-97` to `encryptSecret(json.Marshal(DNSCredentials), secretAAD("certificates", id, "dns_credentials"))` and store in `dns_credentials_encrypted`; keep `dns_credentials` plaintext cleared to `'{}'::jsonb` after migration (mirror `store_secrets.go:119` `persist … encrypted` pattern). Same for `store_acme_accounts.go:232-235` `CreateDNSProviderAccount` / `Update:256` — encrypt via `secretAAD("dns_provider_accounts", id, "credentials")`.

3. **Read path:** `GetCertificate:109-132` decrypt `dns_credentials_encrypted` via `secretAAD`; `List:135-189` either decrypt per-row (costly) or omit credentials from list and only return on `Get` (preferred — least exposure). Same for `ListDNSProviderAccounts:186`.

4. **Migration:** Extend `store_secrets.go:MigrateOperationalSecrets:64-79` `fields` list to include `{"certificates","id::text","dns_credentials","dns_credentials_encrypted"}` and `{"dns_provider_accounts","id::text","credentials","credentials_encrypted"}` (JSONB vs TEXT handling — marshal as string). Idempotent encrypt + clear plaintext.

5. **API masking:** Mask on read — `credentials` never returned in `List`; `Get` returns decrypted only to `requireRole admin` + `audit`.

**Verification:** `SELECT credentials, credentials_encrypted FROM dns_provider_accounts` after migration → `credentials='{}'` and encrypted non-empty. Decrypt round-trip test with same AAD only.

---

### 3.4 Fix F4 — Trusted Proxy CIDR (P1-b)

**Current:** `middleware_ratelimit.go:98` `ExtractClientIP` trusts XFF when `peerIP.IsLoopback()||IsPrivate()||IsUnspecified()` at 101 — any RFC1918 container can inject `X-Forwarded-For: 203.0.113.99` and rotate rate-limit/IP-access identity to bypass `RATE_LIMIT_TRUSTED_IPS` or `ADMIN_IP_ALLOW`. `middleware_ipaccess.go:105,125` `TrustProxy:true` default warn-only. `middleware_mtls.go:123` `X-Forwarded-Proto` trusted without CIDR gate; logging uses `c.IP()` while limit uses `ExtractClientIP` — two notions.

**Reference model:** Caddy `server.go:1171` `determineTrustedProxy` — forwarded headers honored **only** when direct peer matches explicitly configured CIDR ranges; strict levels `TrustedProxiesStrict:224` walk XFF chain skipping only trusted hops. Traefik `ipWhitelist:31` fails closed if `sourceRange` empty.

**Fix:**

1. **Config:** Introduce `TRUSTED_PROXIES` env (comma CIDRs) + fallback to `TRUSTED_PROXY_CIDRS`; parse at startup via `net.ParseCIDR`. Empty → trust **none** (only `X-Forwarded-For` ignored, `c.IP()` used) — fail-closed for prod; dev may set `127.0.0.1/32,10.0.0.0/8` explicitly.

2. **ExtractClientIP rewrite** `middleware_ratelimit.go:98`:
   ```go
   func ExtractClientIP(c *fiber.Ctx) string {
     peer := strings.TrimSpace(c.IP())
     peerIP := net.ParseIP(peer)
     if peerIP == nil || !isTrustedProxy(peerIP) { return peer } // require CIDR match
     // walk XFF right→left, skip trusted hops, return first untrusted
     // (Caddy strict model) — or simple right-most non-trusted if single hop
   }
   func isTrustedProxy(ip net.IP) bool {
     for _, cidr := range trustedProxies { if cidr.Contains(ip) { return true } }
     return false
   }
   ```

3. **IPAccess:** Change `AdminIPAccessConfig:98` / `APIIPAccessConfig:118` to `TrustProxy: isTrustedProxyConfigured()` and make `ADMIN_IP_ALLOW` fail-closed when production and not set (or document warn-only as intentional with alert — current warn at 110,130 is insufficient). Unify IP derivation for `middleware_request_logging.go:70` to use `ExtractClientIP` everywhere.

4. **mTLS gate:** Replace `middleware_mtls.go:123` raw `X-Forwarded-Proto` with `if !isTrustedProxy(net.ParseIP(c.IP())) && c.Protocol()!="https" -> 400` else allow `X-Forwarded-Proto`.

**Test:** `ExtractClientIP` with `RemoteAddr=10.0.0.5` (trusted) + `XFF: 203.0.113.1` → returns `203.0.113.1`; with `RemoteAddr=203.0.113.5` (untrusted) + same XFF → returns `203.0.113.5` (no spoof). Add `TRUSTED_PROXIES=10.0.0.0/8` env in tests.

---

### 3.5 Fix F5 — CSP Abort (P1-c)

**Current:** `middleware_security.go:22` `return "fallback-nonce"` on `rand.Read` failure, `middleware_security_headers.go:43` same. Then `script-src 'nonce-fallback-nonce' 'strict-dynamic'` makes injected `nonce="fallback-nonce"` execute.

**Fix — abort, not fallback (single line each):**

```go
func generateNonce() (string, error) { // at 19 and 40
  b := make([]byte, 16)
  if _, err := rand.Read(b); err != nil {
    return "", fmt.Errorf("generate CSP nonce: %w", err)
  }
  return base64.StdEncoding.EncodeToString(b), nil
}
// In handler:
nonce, err := generateNonce()
if err != nil {
  // Fail closed: do not ship deterministic CSP; abort or omit CSP header
  return fiber.NewError(fiber.StatusInternalServerError, "security header unavailable")
}
// Or minimal: if err != nil { return c.Next() } without setting CSP (never set fallback-nonce)
```

Also **deduplicate** to one middleware (API vs web). Keep `X-CSP-Nonce` only if SSR needs it; otherwise drop header. Unify `Permissions-Policy` values between the two files (currently `geolocation=(), microphone=(), camera=(), payment()...` vs `geolocation=(), microphone=(), camera=()`).

**Verification:** Unit test `generateNonce` failure path (mock `rand.Reader` error) → no `fallback-nonce` in response, either 500 or no `Content-Security-Policy`.

---

### 3.6 Fix F6 — Keyring AAD Namespace (P1-f)

**Current:** `keyring.go:75` `Encrypt(plaintext, aad string)` / `96` `Decrypt(envelope, aad)` use `[]byte(aad)` as GCM additional data but call sites vary: `store_secrets.go:45` `table+":"+id+":"+field`, some callers use node ID vs project ID vs `""` (`store_envvars.go:274` passes `""`). No cross-check that AAD matches envelope's intended resource type; `NeedsRotation:134` only checks key ID.

**Fix:**

1. **Constant per usage** — enforce caller to supply namespaced AAD via helper, add unit test that `Encrypt`→`Decrypt` round-trips only with same AAD:
   ```go
   // In store: always use secretAAD(table, id, field) — already exists at 45
   // Add vet: forbid s.secrets.Encrypt(..., "") via linter or wrapper
   func (s *Store) encryptSecretNamespaced(plaintext, table, id, field string) (string, error) {
     return s.encryptSecret(plaintext, secretAAD(table, id, field))
   }
   ```

2. **ParseKey fix** `keyring.go:29-42` — try base64 first or require explicit prefix to avoid 64-char base64 string that happens to be valid hex being misinterpreted as hex (O5-10). Either try base64 before hex, or require hex strings to be explicitly prefixed `hex:`.

3. **Test:** `Encrypt("x", "nodes:1:token")` then `Decrypt(envelope, "projects:1:token")` must fail authentication; `NeedsRotation` test with same AAD different key ID.

---

### 3.7 Additional Fixes Required in Slice (not in prompt subset but P1 for completeness)

- **F7 Session pointer race** `auth/session.go:36` — Clone on `Create`/`Get` (`*sess` copy), store `sha256Hex(token)` as map key (as done for `jti` at `http/auth.go:412`), update `byToken` atomically in `Update`/`Delete`. Test with `-race` concurrent `GetByToken` + `Update LastActiveAt`.
- **F8 L4 probe hardening** `handlers_loadbalancer.go:88` / `services/loadbalancer/service.go:322` — `net.ParseIP(req.IP)` reject `nil`, reject `IsLoopback()||IsPrivate()||IsUnspecified()||IsLinkLocalUnicast()` unless operator allowlist `LOAD_BALANCER_ALLOW_PRIVATE=true`, reject `169.254.169.254` metadata, add `TargetHost` validation parity with `trafficmanager/service.go:388`, audit `AppendAudit` for every LB mutation.
- **F9 Tenant tagging** `domain/domain.go:114` + `events/event.go:196` — add `TenantID`/`ProjectID` to `PlacementRequest`, `PlacementDecision`, `Envelope` now while volumes small (phase-05 AF-7); add `tenant_id` column to `events` table (`eventstore/migration.go:19`), filter candidates per project quotas later. Document retro migration cost if delayed.
- **F10 Audit wiring** — add `AppendAudit` to `handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_certificates.go`, `failover/service.go`, `handlers_firewall.go`; extend `Envelope` with `tenant_id` (ties to F9).
- **F11 mTLS XFP** `middleware_mtls.go:123` — drop raw `X-Forwarded-Proto` unless peer in `TRUSTED_PROXIES` (see F4).
- **F12 WS origin tightening** `ws_origin.go:95` — optionally require Origin even for Bearer when allow-list non-empty; document ticket-scoped WS as intended; consider `isTrustedProxy` gate for `X-Forwarded-Proto` at `realtime.go:132-145`.
- **F13 AllPermissions divergence** `permissions.go:89` — add `PermServerView`/`PermServerSettings` to `AllPermissions` or remove defs.

---

## 4. Files to Touch — Ordered by P0→P1

| Priority | File:line | Change Summary | Est. LOC | Risk |
|----------|-----------|---------------|----------|------|
| **P0** | `forge/api/internal/store/store_users.go:306,555` | `UpsertServerSubuser` signature + subset check + `normalizeSubuserPermissions` remove `*` escape, unify catalog | 40-60 | Medium — authz gate, needs handler coupling |
| **P0** | `forge/api/internal/http/handlers_servers.go:541,574` | Resolve `actorPerms` before store call, pass to `UpsertServerSubuser`, reject subset violation 403 | 20-30 | Low |
| **P0** | `forge/api/internal/store/permissions.go:84,89` | Add `PermServerView/Settings` to `AllPermissions` or remove defs; keep `HasPermission` wildcard but never persist `*` | 5 | Low |
| **P0** | `forge/api/internal/store/store_mounts_ext.go:323-339` | Expand `validateMountPath` to deny-list prefixes `/etc,/var/run,/proc,/sys,/dev,/boot,/root,/,/var/lib/forge` or switch to allowlist `MOUNTS_ALLOWED_PREFIX` | 25-35 | Low |
| **P0** | `beacon/*` (runtime mount re-validation) | Mirror prefix check at workload creation `mounts.go:13` + `daemon` | 10 | Low |
| **P1** | `forge/api/internal/store/store_certificates.go:86,95,116` | Encrypt `dns_credentials` via `dns_credentials_encrypted`, decrypt on `Get`, omit from `List`, mask | 30-40 | Medium — migration + read path |
| **P1** | `forge/api/internal/store/store_acme_accounts.go:187,232,256` | Encrypt `credentials` via `credentials_encrypted`, same pattern | 30-40 | Medium |
| **P1** | `forge/api/migrations/156_*.sql` | Add `*_encrypted` columns, copy `store_secrets.go` migration | 10 | Low |
| **P1** | `forge/api/internal/store/store_secrets.go:64,119` | Extend `MigrateOperationalSecrets` fields to include cert/DNS creds | 10 | Low |
| **P1** | `forge/api/internal/http/middleware_ratelimit.go:98,121,288` | `TRUSTED_PROXIES` CIDR config, `ExtractClientIP` CIDR gate, `isTrustedProxy` helper | 30-40 | Medium — network boundary change, needs rollout + docs |
| **P1** | `forge/api/internal/http/middleware_ipaccess.go:43,98,118` | `TrustProxy` CIDR-gated, fail-closed in prod when `ADMIN_IP_ALLOW` unset | 15 | Low |
| **P1** | `forge/api/internal/http/middleware_mtls.go:123` | XFP only from trusted proxy CIDR | 5 | Low |
| **P1** | `forge/api/internal/http/middleware_security.go:19,27` + `middleware_security_headers.go:40,48` | `generateNonce` return error, abort/omit CSP on failure, dedupe middlewares, unify `Permissions-Policy` | 15-20 | Low |
| **P1** | `forge/api/internal/secrets/keyring.go:29,75,96,134` | `ParseKey` base64-first, `Encrypt/Decrypt` AAD namespace enforcement, test | 20 | Low |
| **P1** | `forge/api/internal/auth/session.go:36,51,68,100,231` | Clone on `Create`/`Get`, `sha256Hex(token)` key, atomic `byToken` | 30 | Medium — concurrency |
| **P1** | `forge/api/internal/http/handlers_loadbalancer.go:84` + `services/loadbalancer/service.go:322` + `dataplane.go:142,317` | `net.ParseIP` + private/metadata reject, allowlist, audit | 20 | Low |
| **P1** | `forge/api/internal/domain/domain.go:114` | Add `TenantID/ProjectID` to `PlacementRequest/Decision` | 10 | Medium — API shape change, migration |
| **P1** | `forge/api/internal/events/event.go:196` | Add `TenantID` to `Envelope` | 5 | Medium |
| **P1** | `forge/api/internal/eventstore/migration.go:19` | Add `tenant_id` column to `events` table + index | 5 | Medium |
| **P1** | `forge/api/internal/http/handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_certificates.go`, `handlers_firewall.go`, `services/failover/service.go` | Add `AppendAudit` per mutation | 20 | Low |
| **P1** | `forge/api/internal/http/ws_origin.go:95,148` | Optional: require Origin even for bearer when allow-list non-empty; doc ticket intent | 10 | Low |
| **P2** | `forge/api/internal/http/middleware_security.go:58` | HSTS `preload` only when verified on preload list — or drop `preload` | 2 | Low |

**Total touch surface:** ~16 files, ~320-420 LOC net, no new subsystems — all fixes are within existing code.

---

## 5. Risks — If Deferred or Done Wrong

| Risk | Trigger | Impact | Mitigation |
|------|---------|--------|------------|
| **Subuser escalation remains open** | F1 not shipped | Any `user.create` holder (common) → `*` or `database.view_password` → secret exfil, `cron.create` → node shell, `settings.reinstall` → wipe | Ship F1 first, add handler+store test, gate `*` behind owner/admin, retro-scan existing `subusers.permissions` containing `*` and downgrade |
| **Host breakout via mount** | F2 narrow | Compromised admin or stolen `mounts.write` key + `mount_node`/`egg_mount` double-join → mount `/var/run/docker.sock` → container breakout → host root | Expand deny-list + allowlist base, re-validate in beacon; audit existing mounts for blocked sources |
| **DNS token exfil** | F3 plaintext | `GET /admin/certificates` or `dns_provider_accounts` SELECT leaks Cloudflare/Route53 tokens to any `certificate.read` caller; DB backup/snapshot exfil targets zone takeover | Encrypt + clear plaintext, scope read to admin, mask in List, rotate any tokens that were stored plaintext |
| **XFF identity rotation** | F4 fail-open | Any RFC1918 container spoofs `X-Forwarded-For` → rotates rate-limit key → bypass 5/30/120 req/min limits (auth 5/min is auth-bruteforce protection), bypasses `ADMIN_IP_ALLOW` | Ship CIDR gate, require `TRUSTED_PROXIES`; after deploy, verify via `curl -H "X-Forwarded-For: 1.2.3.4"` from untrusted peer does not change `ExtractClientIP` |
| **CSP nonce bypass** | F5 fallback | Deterministic `fallback-nonce` + `strict-dynamic` → injected `script nonce="fallback-nonce"` executes — worse than no CSP (attacker knows nonce) | Abort on `rand.Read` failure; add test that no response contains `fallback-nonce` |
| **AAD misuse** | F6 caller-dependent | Ciphertext encrypted with wrong AAD (e.g. `""`) decrypts only with same wrong AAD on same host — cross-resource transplant not detected until next rotation; `NeedsRotation` misses it | Enforce `secretAAD(table:id:field)` wrapper, test cross-AAD failure |
| **Session race / token heap leak** | F7 raw pointer + raw token | Concurrent WS probes leak `LastActiveAt` race (crash under `-race`), heap dump exposes 43-char bearer tokens | Clone + hash, `go test -race` |
| **L4 probe as SSRF** | F8 arbitrary IP | LB-manager probes internal `172.16.x.x` / `169.254.169.254` metadata → credential theft → relay via TCP listener | Validate IP, reject private/metadata, audit |
| **Tenant-blind retro cost** | F9 tenantless | Placement/event tables accumulate tenant-less rows; adding `tenant_id` later requires expensive backfill and dual-write; one tenant's scaling influences fleet capacity; audit trails unpartitionable for incident response (NetBird account scoping is data property) | Tag now while volumes small; add column with default empty, dual-write new code, backfill async |
| **Audit gap** | F10 silent | Firewall/traffic/LB/cert rotations have no `AppendAudit` — compliance/incident cannot attribute `proxy_domains` or `security_headers` mutations; `GatewayAdapter` half-wired (`NewWithAdapterAndPersistence` never called at `main.go:1025`) | Wire audit before next network change window |
| **Validation drift** | Bead — not fixing mount prefix in beacon too | API rejects `source=/etc/shadow` but beacon `AllowedMountSourcesForNode` still lists it if pre-existing row, runtime mounts outside FS confinement | Validate in both control-plane and beacon; add integration test `deploy with old mount source must fail` |

**Cross-cutting risk:** F1, F2, F3, F4 are independently exploitable but combine into privilege chain: low-priv → `*` (F1) → create mount `/var/run/docker.sock` (F2) → attach to server → exec via `cron.create` or `control.console` → exfiltrate DNS tokens (F3) → rotate identity via XFF (F4) to hide audit gap (F10). Fix order should be **F1 → F2 → F3 → F4 → F5 → F6 → F7 → F8 → F9/F10** (P0 first, then network boundaries, then depth).

---

## 6. Verification Checklist (pre-implementation, runnable now)

- [ ] `go vet` — confirm `permission == "*"` special-case at `store_users.go:564` exists and is the only persist path for wildcard
- [ ] `rg -n 'normalizeSubuserPermissions|UpsertServerSubuser|HasPermission.*\*'` — enumerate all wildcard grant paths
- [ ] `rg -n 'validateMountPath|validateMountPaths'` — confirm only 2 blocked sources for `source`, 2 for `target`
- [ ] `rg -n 'dns_credentials|dns_provider_accounts.*credentials'` — confirm plaintext JSONB at `134,135` migrations and `store_certificates.go:95` / `store_acme_accounts.go:232`
- [ ] `rg -n 'ExtractClientIP|isTrustedIP|isIPInList|getClientIP'` — confirm 3 call sites, two IP notions
- [ ] `rg -n 'fallback-nonce'` — confirm 2 occurrences at `middleware_security.go:22` + `middleware_security_headers.go:43`
- [ ] `rg -n 'ParseKey|Encrypt.*aad|NeedsRotation'` — confirm AAD caller spread
- [ ] `rg -n 'InMemorySessionStore|byToken|LastActiveAt'` — confirm pointer sharing at `session.go:62,79,97,111,261`
- [ ] `go test ./forge/api/internal/http -run 'TestExtractClientIP|TestWSOrigin' -count=1` — baseline pass before F4/F12
- [ ] `go test ./forge/api/internal/auth -run TestSession -race -count=1` — expect race if concurrent Get+Update
- [ ] `psql -c "SELECT count(*) FROM subusers WHERE permissions::text LIKE '%*%'"` — scan existing wildcard grants (if DB available)
- [ ] `psql -c "SELECT source FROM mounts WHERE source IN ('/etc','/var/run/docker.sock','/proc','/')"` — scan mounts outside allowlist

---

## 7. References — How to Read Related Audits

- **If fixing mounts:** also see `phase-02/subagent-04` + `phase-05/synthesis` for `file_denylist` passthrough not enforced (`store_mounts_ext.go:323` vs `egg.file_denylist`).
- **If fixing DNS creds:** also see `phase-04/subagent-05-network-security.md` for `PDNS_API_URL` SSRF + env mutation.
- **If fixing trusted proxy:** also see `phase-01/subagent-05-architecture-security.md` C11-C17 and `middleware_request_logging.go:70` dual IP notion.
- **If fixing CSP:** also see `final-parity/subagent-10 S09` duplicate middleware analysis — dedupe, don't patch both separately.
- **If adding tenant columns:** coordinate with `domain/PlacementRequest` + `events.Envelope` + `eventstore` migration in one atomic PR to avoid drift between layers.

---

*End of subagent-09 preparation audit — 17 inspected symbols/files, 5 P0 + 10 P1 known issues inventoried, 13 fix specs (F1-F13) with file:line, 16 files to touch (~320-420 LOC), risks and verification checklist provided. No code modified per task.*
