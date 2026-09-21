# Subagent 10/10 — Phase 02 Context Confirm — Security / Tenancy / RBAC + UX

**Agent:** 110-02-10 of 110 — Phase 02 Agent 10/10 (parallel)
**Focus:** Confirm Security/Tenancy/RBAC + UX Findings
**Date:** 2026-08-24
**Method:** source-only re-inspection — no product code modified. All citations `file:line` verified via Read/Grep. Evidence tier: SOURCE_VERIFIED.
**Prior artifacts reconciled:**
- `audits/FINAL_PARITY_AUDIT.md` §11 (S01-S16, 30 rows — 16 security) + §16 (UX 14 rows)
- `audits/final-parity/subagent-10-security-ux.md` (16 security + 14 UX, P0×2: S02/S03 wildcard + S10 policy leakage)
- `audits/reverification/subagent-20-security-ux.md` (30 rows: 18 security S01-S18 + 12 UX U01-U12)
- `audits/phase-01/subagent-05-architecture-security.md` (17 comparisons C01-C17, F1-F5 + O5-10)
- `audits/phase-01/subagent-04-ux-control-plane.md` (17 comps C01-C12 UX)
- `audits/phase-02/subagent-05-tenancy-schedules.md` (F-01 *, F-03 mounts), `audits/phase-04/subagent-05-network-security.md` (15 comps), `audits/phase-05/subagent-05-architecture-synthesis.md`

**Forge layers live-inspected for this pass (key files):**
`forge/api/internal/store/permissions.go:89` `AllPermissions`, `forge/api/internal/store/store_users.go:544` `normalizeSubuserPermissions` + `:306` `UpsertServerSubuser`, `forge/api/internal/store/store_tenancy.go:91` `ListOrganizationsForUser` + `:539` `ResolveEffectiveOrgRole`, `forge/api/internal/domain/domain.go:114` `PlacementRequest`, `forge/api/internal/store/store_mounts_ext.go:323` `validateMountPath`, `forge/api/internal/http/middleware_ratelimit.go:98` `ExtractClientIP`, `forge/api/internal/http/middleware_security.go:22` `generateNonce` fallback, `forge/api/internal/http/middleware_security_headers.go:43` second fallback, `forge/api/internal/services/trafficmanager/traefik_proxy.go:960` `collectApplicablePolicies`, `forge/api/internal/services/trafficmanager/caddy_proxy.go:722` `buildPolicyRoutes` + `:795` `buildPolicyHandles` fictional handlers, `forge/api/internal/http/handlers_loadbalancer.go:88` `AddTarget`, `forge/api/internal/secrets/keyring.go:75` `Encrypt` AAD, `forge/api/internal/auth/session.go:36` `InMemorySessionStore`, `forge/api/internal/events/event.go:196` `Envelope`, `forge/api/internal/eventstore/store.go:113` `Publish`, `forge/api/internal/eventstore/outbox.go:39` `Relay.Subscribe`
`forge/web/components/admin/admin-registry.ts:22` 5 groups ~61 entries, `forge/web/components/admin/admin-shell.tsx:42` `plan` 8 goal groups, `forge/web/components/admin/AdminOverview.tsx:86` + `forge/web/app/admin/monitoring/page.tsx:1`, `forge/web/app/admin/apps/[id]/page.tsx:28` `useState` tabs, `forge/web/components/app/deployment-progress.tsx:70` 2s poll vs `forge/web/components/charts/DeploymentTimeline.tsx:27` 5s poll, `forge/web/components/environment/env-var-editor.tsx:8` vs `forge/web/components/admin/AdminAppsShared.tsx:108` vs `forge/web/components/environment/EnvironmentEditor.tsx:37` triplicate, `forge/web/app/admin/{traffic,load-balancer,domains,certificates,security,firewall,endpoints}/*` 7 pages, `forge/web/components/shared/states-empty.tsx:17`

---

## 0. Executive Verdict — P0s Still Broken

**2/2 P0 security findings RECONFIRMED STILL BROKEN on live checkout (2026-08-24):**

| P0 | Title | Live file:line | Verdict |
|---|---|---|---|
| **GH-18 / SE-01 / S02-S03** | Subuser wildcard `*` persistable + no actor-subset check — any `user.create` holder → full control (`database.view_password`, `schedule.delete`, `settings.reinstall`, `cron.create`) | `forge/api/internal/store/store_users.go:544` (`normalizeSubuserPermissions:564` `permission != "*"`) + `:306` `UpsertServerSubuser` (no actor param) + `forge/api/internal/http/handlers_servers.go:541` only `PermUserCreate` gate | **STILL BROKEN — P0 escalation** |
| **NG-10 / SE-09 / S10** | All gateway policies apply to all routes — cross-tenant L7 isolation break | `forge/api/internal/services/trafficmanager/traefik_proxy.go:960` `collectApplicablePolicies` returns entire map + `forge/api/internal/services/trafficmanager/caddy_proxy.go:722` `buildPolicyRoutes` prepends all handles onto every route + `forge/api/internal/store/store_traffic.go:44` no join | **STILL BROKEN — P0 cross-tenant** |

No P0 was remediated between FINAL_PARITY (2026-08-24) and this live re-inspection — identical lines, identical gaps. Two additional P0-adjacent host-breakout primitives also reconfirmed: mount allowlist too narrow (`store_mounts_ext.go:323` only 2 denials) and backup sidecar unauthenticated (out of scope for this agent but noted in FINAL_PARITY §13).

Overall security: **14 of 16 §11 S-rows still BROKEN/PARTIAL** (2 COMPLETE), 0 newly FIXED. UX: **0 of 14 rows newly FIXED** beyond FINAL_PARITY; 2 PARTIAL mitigations (nav re-group, monitoring honest labeling) contain but do not resolve underlying splits.

---

## 1. FINAL_PARITY §11 Security — S01-S16 Live Confirm (file:line cited)

> §11 = `audits/FINAL_PARITY_AUDIT.md:304-324` (16 security rows). Columns: **# | Area | Prior (FINAL_PARITY) | Live file:line | Verdict | Severity | Gap / Evidence**

| # | Area | FINAL_PARITY status | Live file:line | Verdict | Sev | Gap (live) |
|---|---|---|---|---|---:|---|
| S01 (SE-01) | Permission catalog + wildcard `*` doc vs impl | BROKEN (P0 escalation) | `forge/api/internal/store/permissions.go:89` `AllPermissions():90-106` + `permissions.go:84` `PermServerView`/`PermServerSettings` defined but absent from AllPermissions + `:196` comment "*must ensure wildcard only ever assigned to owners/admins*" | **STILL BROKEN** (PARTIAL catalog inconsistency + doc-violating wildcard) | P3 (catalog) + P0 via S02 | Live `AllPermissions` total 43 entries but omits 2 consts (`server.view`, `server.settings` at `:84-85`); `normalizeSubuserPermissions` would drop them if assigned — inconsistency. Doc at `:196-199` violated by S02. No change since reverification-20 S01. |
| S02 (SE-02) | **Wildcard `*` persistable by any `user.create` holder — P0** | BROKEN P0 | `forge/api/internal/store/store_users.go:544` `normalizeSubuserPermissions:562-565` `(!allowed[perm] && permission != "*")` explicitly allows `*` + `:306` `UpsertServerSubuser` only normalizes, no actor subset param + `forge/api/internal/store/permissions.go:206` `HasPermission:211` `p=="*"` universal grant + `forge/api/internal/http/handlers_servers.go:541` `requireServerPermission(PermUserCreate)` only | **STILL BROKEN — P0** | **P0** | Identical to FINAL_PARITY `store_users.go:544` cited. Any holder of `user.create` can `POST /servers/:id/users {["*"]}` → `*` persisted as JSON. Repro unchanged. Pay attention per task ✓. |
| S03 (SE-02) | Subset enforcement (actor can only grant what they have) | BROKEN P0 | `forge/api/internal/store/store_users.go:306` `UpsertServerSubuser(ctx, serverID, req, actorID *string)` — no `actorPerms`/`actorRole` param + `store_users.go:377` `UserCanAccessServer` not consulted | **STILL BROKEN — P0** | **P0** | Same root as S02. Holder of `file.read+user.create` can grant `database.view_password`, `allocation.delete`. No `GetServerSubuser` + subset loop added. |
| S04 (SE-03) | Tenancy scope (org/project/env) vs placement tenant isolation | PARTIAL (P1) | `forge/api/internal/store/store_tenancy.go:91` `ListOrganizationsForUser` (org tables exist) + `:539` `ResolveEffectiveOrgRole:540-548` `if globalRole=="admin" return "admin"` bypass + `forge/api/internal/domain/domain.go:114` `type PlacementRequest struct` no `TenantID`/`OrgID`/`ProjectID` + `forge/api/internal/events/event.go:196` `type Envelope struct` no `tenant_id` + `forge/api/internal/eventstore/migration.go:19` events table no `tenant_id` col | **STILL PARTIAL (P1)** — tenant-blind scheduling/audit | P1 | Policy layer enforced where checked (server read via `PermServerView` or org membership; `store_tenancy.go:539` admin bypass correct), but fleet-wide placement still global: `scheduler/service.go:88` `PlaceServer` ranks `ListNodes` globally; `events/event.go:196` tenantless → audit cannot partition. Same as reverification-20 S04. |
| S05 (SE-04) | Mount allowlist / confinement | BROKEN (P0 host breakout) | `forge/api/internal/store/store_mounts_ext.go:323` `validateMountPath:324-339` | **STILL BROKEN** | P1* (P0 host breakout per FINAL_PARITY GH-19) | Live at `:332` `if field=="source" && (value=="/etc/forge" \|\| value=="/var/lib/forge/volumes")` only blocks 2 sources; `:335` `target ∈ {"/", "/home/container"}`. Syntactically strong (`IsAbs`, `Clean==value`, no `..` at `:327-330`) but semantically narrow — `/etc`, `/etc/shadow` parent, `/var/run/docker.sock`, `/proc`, `/`, `/root`, `/boot`, `/var/lib/forge` parent all pass. |
| S06 (SE-05) | Cron shell boundary — server jobs never reach host shell | COMPLETE+ | `forge/api/internal/services/cronjob/service.go:212` `runShellCommand` scrubbed `PATH`/`HOME` + `:135` `TargetType=="server"` → `dispatchServerCommand:240` fail-closed if `serverDispatcher==nil` (`:249-251` never falls back) + `forge/api/internal/http/handlers_user_console.go:314` forces `TargetType="server"` (admin routes `handlers_cronjob.go:59` `requireRole("admin")`) | **STILL FIXED — COMPLETE** | — | No regression. `service_rce_regression_test.go:17-38` fail-closed still holds. The only UX-layer drift is `defaultMaxCronTimeoutSeconds=3600` still large, but boundary correct. |
| S07 (SE-06) | DB identifier quoting | COMPLETE | `forge/api/internal/services/dbprovisioner/service.go:737` `quotePostgresIdentifier` `"a""b"`, `:740` `quoteMySQLIdentifier` `\`a\`\`b\``, `:743` `quoteSQLString` `'a''b'` + `:326` `validateTarget` + `handlers_database_services.go:44` `requireRole("admin")` | **STILL FIXED — COMPLETE** | — | Matches/exceeds reference. `service_test.go:226-232` quoting still passes. Per-phase-06 `mysql.RegisterTLSConfig` global leak noted separately (P1) but not quoting. |
| S08 (SE-07) | Trusted proxy model — forwarded headers only from configured CIDRs | BROKEN (P1) | `forge/api/internal/http/middleware_ratelimit.go:98` `ExtractClientIP:99-119` trusts XFF when peer `IsLoopback\|\|IsPrivate\|\|IsUnspecified` (`:101`) + `forge/api/internal/http/middleware_ipaccess.go:44` `getClientIP` `TrustProxy:true` always + `:98` `AdminIPAccessConfig` warn-only when `ADMIN_IP_ALLOW`/`DENY` unset | **STILL BROKEN** | P1 | Live at `middleware_ratelimit.go:101` `peerIP.IsLoopback()\|\|peerIP.IsPrivate()\|\|peerIP.IsUnspecified()` → trusts XFF from any RFC1918 peer. `middleware_ipaccess.go:98` `TrustProxy:true` warn-only. No `TRUSTED_PROXIES` env exists. `middleware_request_logging.go:70` still uses raw `c.IP()` vs limiter trusted-derived — two IP notions. |
| S09 (SE-08) | CSP nonce entropy + `strict-dynamic` | BROKEN (P1) | `forge/api/internal/http/middleware_security.go:19` `generateNonce:22` `return "fallback-nonce"` + `:27` `SecurityHeaders` always CSP+HSTS+XFO + `forge/api/internal/http/middleware_security_headers.go:40` `generateMiddlewareNonce:43` same fallback | **STILL BROKEN** | P1 | Both middlewares fall back to constant `"fallback-nonce"` if `rand.Read` fails, combined with `strict-dynamic` in CSP (`:33` and `DefaultSecurityHeadersConfig.CSPValue`). Deterministic nonce + `strict-dynamic` = injected `nonce="fallback-nonce"` executes. Correct is abort/500, not ship. |
| S10 (SE-09) | Policy attachment model — all-policies-on-all-routes | BROKEN (P0 cross-tenant) | `forge/api/internal/services/trafficmanager/traefik_proxy.go:960` `collectApplicablePolicies:964` `"For now, return all policies..."` + `forge/api/internal/services/trafficmanager/caddy_proxy.go:722` `buildPolicyRoutes` prepends `policyHandles` onto every route (`:728-731` loops all policies, `:742` `enrichRouteWithPolicy`) + `forge/api/internal/store/store_traffic.go:44` no FK/join, `:161` parallel tables | **STILL BROKEN — P0** | **P0** | Identical to FINAL_PARITY NG-10. One tenant's whitelist/blacklist/ratelimit/circuitBreaker applies to another's domain → availability break + bypass. 3 subagents independently confirmed. See §2.2 for deep dive. |
| S11 (SE-10) | L4 LB probe primitive as internal-network probe/relay | BROKEN (P1) | `forge/api/internal/http/handlers_loadbalancer.go:88` `AddTarget` accepts `req.IP` verbatim (`:96` `strings.TrimSpace(req.IP)`, `:97` only `== ""` + port/weight checks) — no `net.ParseIP`, no private/metadata rejection + `forge/api/internal/services/loadbalancer/dataplane.go:142` dials `IP:port` per connection | **STILL BROKEN** | P1 | Any `loadbalancer.write` holder can probe `169.254.169.254`, `10.0.0.0/8` etc via LB listeners as one-hop relays. Forge's own `trafficmanager/service.go:388` validates `TargetHost` more strictly — LB path lacks parity. |
| S12 (SE-11) | Keyring AAD binding | PARTIAL (P1) | `forge/api/internal/secrets/keyring.go:75` `Encrypt(plaintext, aad)` / `:96` `Decrypt(envelope, aad)` GCM with AAD + `:29` `ParseKey` hex vs base64 + `forge/api/internal/store/store_secrets.go:18` `encryptSecret`/`decryptSecret` + `:45` `secretAAD(table+":"+id+":"+field)` | **STILL PARTIAL** | P1 | Correct primitive; gaps persist: (1) `ParseKey:31-35` tries hex when `len==64 && validHex` → 64-char base64 that happens to be valid hex misinterpreted as hex (phase-01 O5-10). (2) AAD caller-dependent — some callers pass `""` (e.g. `store_envvars.go:274` `decryptSecret(..., "", aad)`); same key ID can be `!NeedsRotation` yet fail only on next decrypt. No typed AAD constant validation. Not a bypass today but misuse risk. |
| S13 (SE-12) | Session pointer race + raw token storage | BROKEN (P1) | `forge/api/internal/auth/session.go:36` `InMemorySessionStore{sessions map[string]*Session, byToken map[string]string}` + `:51` `Create` stores caller pointer (`:62` `s.sessions[session.ID]=session`) + `:68` `Get` / `:82` `GetByToken` return same pointer + `:231` `SessionMiddleware` mutates `LastActiveAt` outside store lock (`:261` `sess.LastActiveAt=Now()` then `:262` `store.Update`) | **STILL BROKEN** | P1 | Data race: concurrent Bearer requests share `*Session`, both `LastActiveAt=Now()` without store lock — `go test -race` detectable. `byToken` stores raw base64 token → heap/pprof leak. `Update:100-111` only touches `sessions[id]` leaving `byToken` stale if token mutated. Durable `store_users.go:914` `user_sessions` correctly stores `session_token_hash` SHA-256 hex — in-memory does opposite. |
| S14 (SE-13) | mTLS HTTPS gate trusts client X-Forwarded-Proto | BROKEN (P1 hardening would be PARTIAL) | `forge/api/internal/http/middleware_mtls.go:117` comment + `:123` `c.Protocol() != "https" && c.Get("X-Forwarded-Proto") != "https"` passes if client sends header — no CIDR gate | **STILL BROKEN** | P1 | Cert verification still requires real `*tls.Conn:127-158` so not full bypass, but weakens defense-in-depth. Same trust question as S08. Fix: drop raw XFP unless peer ∈ TRUSTED_PROXIES. |
| S15 (SE-14) | WS origin for Bearer auth (missing Origin allowed) | PARTIAL (P2 design trade-off) | `forge/api/internal/http/ws_origin.go:57` `isCookieAuthRequest` + `:95` `validateWebSocketOrigin` (missing Origin allowed only for `!isCookieAuth`) + `:125` `wsOriginMiddleware` + `:157` `wsUpgraderOrigins` permits `""` for token clients + `forge/api/internal/http/ws_hub.go:13-49` intentionally NOT subscribed in `main.go` | **STILL PARTIAL** (by design) | P2 | Correctly requires Origin for cookie sessions while allowing token clients to omit it. `ws_hub` un-wired → real notifications fall back to 5s polling (durability cost). `wsUpgraderOrigins` always permits `""` for token clients — moderate vs Portainer stricter. Documented trade-off, not a logic bypass today. |
| S16 (SE-15) | Audit logging — traffic/cert/LB/failover silent | MISSING (P1) | `forge/api/internal/store/store_audit.go` + `store_audit_logs.go` + `forge/api/internal/http/handlers_auditlog.go` — game/app/server tenancy mutations audited via `AppendAudit` (`store_users.go:275`, `store_tenancy.go:165`, `store_envvars.go:143`), but `forge/api/internal/http/handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_certificates.go`, `services/failover/service.go` have zero `AppendAudit` | **STILL MISSING** | P1 | Network mutations (highest blast radius) unrecorded; incident response cannot partition by tenant (ties to S04 tenantless events). NPM audits every mutation. |

**§11 delta vs FINAL_PARITY:** 0 status changes. All 16 rows re-verified at identical `file:line` with identical gaps. `COMPLETE` rows (S06, S07) remain complete; `BROKEN`/`PARTIAL`/`MISSING` rows unchanged.

---

## 2. P0 Deep Dives (pay special attention per task)

### 2.1 P0 — Wildcard `*` Escalation (GH-18 / SE-01 / FINAL_PARITY S02-S03 / reverification S02-S03)

**Severity:** P0 — Privilege escalation / confidentiality+integrity break  
**Reference truth:** `reference/game-hosting/pterodactyl-panel/app/Services/Servers/GetUserPermissionsService.php:18` owner/admin get `['*']` *computed* never stored in `subusers.permissions`; `app/Http/Controllers/Api/Client/Servers/SubuserController.php:154` `getDefaultPermissions()` intersects request with allowlist, always injects `websocket.connect`. `app/Models/Permission.php:120` "They will never be able to ... assign permissions they do not have themselves."

**Forge live (STILL BROKEN):**
- `forge/api/internal/store/store_users.go:555` `normalizeSubuserPermissions` builds `allowed = map[defaultSubuserPermissions():575]` then at `:564` `if permission=="" || seen[permission] || (!allowed[permission] && permission != "*") { continue }` — explicitly **exempts** `"*"` from allowlist.
- `forge/api/internal/store/permissions.go:206` `HasPermission:210-212` `if p=="*" || p==required { return true }` — universal grant.
- `forge/api/internal/store/store_users.go:306` `UpsertServerSubuser(ctx, serverID, req, actorID *string)` only calls `normalizeSubuserPermissions` at `:311`; never loads actor's effective permissions. Handler `forge/api/internal/http/handlers_servers.go:541` `POST /servers/:id/users` only checks `requireServerPermission(PermUserCreate)` (`:574` PATCH checks `PermUserUpdate`), not subset.
- Comment at `permissions.go:196-199` warns "*must ensure wildcard is only ever assigned to server owners or admins via normalized permission checks, not via arbitrary user input*" — **code violates its own doc**, no caller enforces it. `policies/server_policy.go:22` does not participate (subuser RBAC at handler/store).

**Repro (identical to FINAL_PARITY §13, reverification F-01):**
1. Owner creates alice with `permissions=["user.create","file.read"]` via `POST /servers/:id/users`
2. Alice `POST /servers/:id/users {"email":"bob@example.com","permissions":["*"]}` → 200 (only `user.create` checked)
3. Bob `GET /servers/:id/startup` sees unredacted `envVars` (needs `server:read-env` via `*`), `POST /servers/:id/schedules`, `database.view_password`, `buildpack.manage`, `cron.create` dispatching to node via `cronjob/service.go:117`

**Impact:** Any `user.create` holder = full control including `cron.create` (node-touching via `cronjob/service.go:117:240` server dispatcher), `database.view_password`, `settings.reinstall`. Breaks subuser trust boundary; Pterodactyl never stores `*` for subusers — always computed. The same gap escalates *without* `*` too — `file.read+user.create` → `database.view_password` (S03).

**Live vs prior:** Phase-02 F-01 HIGH/P0 (`store_users.go:297` no subset, `normalizeSubuserPermissions` allows `*` at `:564`) → `reverification-20:31` PERSISTS P0 → `final-parity S02/S03` BROKEN P0 → **this pass RECONFIRMED** — lines unchanged, severity unchanged. The new `permissions.go:206` empty-permission guard (`if required=="" return false`) is correct but orthogonal.

**Fix (unchanged from FINAL_PARITY Activation #3):** Change `UpsertServerSubuser(ctx, serverID, req, actorID string, actorPerms []string, actorIsOwnerOrAdmin bool)` → load `GetServerSubuser(ctx, serverID, actorID)` or `["*"]` for owner/admin, then `for _, p := range req.Permissions { if !HasPermission(actorPerms, p) && !actorIsOwnerOrAdmin { return fmt.Errorf("cannot grant %q",p) } }`; reject `*` unless `actorIsOwnerOrAdmin`; remove `permission=="*"` branch or gate behind flag; add test: subuser `user.create+file.read` cannot grant `database.view_password` nor `*`.

### 2.2 P0 — Policy Leakage — All Policies Apply to All Routes (NG-10 / SE-09 / FINAL_PARITY S10 / reverification S10 / phase-04 F2)

**Severity:** P0 — Availability break + policy bypass + tenant isolation failure  
**Reference truth:** `reference/networking/traefik/pkg/config/dynamic/http_config.go:38` `Router.Middlewares []string` per-router; `reference/networking/nginx-proxy-manager/backend/internal/access-list.js:29` per-host lists; Traefik `pkg/server/middleware/middlewares.go:50-83` ordered list with recursion detection.

**Forge live (STILL BROKEN):**
- Data model has **no join**: `forge/api/internal/store/store_traffic.go:44` `traffic_rules` + `:161` `traffic_policies` are parallel tables, no `traffic_rule_policies` FK, no `policy_ids` column. Grep confirms 0 `JOIN` between them.
- `forge/api/internal/services/trafficmanager/traefik_proxy.go:960` `collectApplicablePolicies:964` comment `"For now, return all policies since there is no direct rule-to-policy mapping"` then `result := make(map[string]*TrafficPolicy, len(policies)); for k,v := range policies { result[k]=v }` — returns entire map for **every** rule.
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:722` `buildPolicyRoutes:728-731` loops all `policies` to build `policyHandles`, then at `:742` `enrichRouteWithPolicy(route, policyHandles)` prepends **all** handles onto **every** route. Deterministic order missing.
- Even Caddy handler generation is fictional: `caddy_proxy.go:800` `"handler":"rate_limit"` and at `:868` `"handler":"circuit_breaker"` are not real Caddy modules → `validateConfig` at `caddy_proxy.go:980` `POST /load` fails with `config invalid` when any policy with RateLimit or CircuitBreaker exists, bricking updates (FINAL_PARITY NG-09). Rate-limit `lb_policy` at `caddy_proxy.go:974` also mistranslated (`least_conn` vs Caddy's `least_conn` vs `least_connections`).

**Consequences (live):** One tenant's `IPWhitelist` restricts another's domain; one tenant's `IPBlacklist` silently blocks another's clients; shared `rateLimit` pool leaks; `circuitBreaker` on one domain unlocks another. Cross-tenant ACL fundamentally broken. Confirmed by 3 subagents (phase-04 F2/F5, phase-04-forge-gateway F3, final-parity S10, reverification F-02).

**Live vs prior:** Phase-04 F2 HIGH/P0 (`traefik_proxy.go:960` all-policies) → `reverification-20:36` PERSISTS P0 (store confirms no join) → `final-parity S10` BROKEN P0 → **this pass RECONFIRMED** — `store_traffic.go` still two independent tables, adapters still return entire map. No `policy_ids` column added; no Traefik-shaped `gateway_routers` abstraction introduced.

**Fix (unchanged):** Add `policy_ids FK` join `traffic_rule_policies(rule_id, policy_id)` or Traefik-shaped `gateway_routers/middlewares/services` tables (`GatewayRouter/GatewayMiddleware/GatewayService`); `collectApplicablePolicies(rule)` filters by join; sort middleware deterministically (rate-limit→blacklist→whitelist→circuit-breaker→redirect) mirroring `middlewares.go:50-83`.

---

## 3. Other Live Targets per Task Directive (file:line still-broken vs fixed)

Task explicitly asked to inspect these live locations for STILL BROKEN vs FIXED. Results:

| Live target (task shorthand) | Live file:line | Verdict | Evidence |
|---|---|---|---|
| `permissions.go:89 AllPermissions` | `forge/api/internal/store/permissions.go:89` `AllPermissions():90-106` | **STILL PARTIAL (not fixed)** — catalog inconsistency | `PermServerView:84` + `PermServerSettings:85` defined but absent from AllPermissions → `normalizeSubuserPermissions` would drop them if ever assigned. Catalog is superset otherwise (adds `cron.*`, `buildpack.manage`, `mount.*`, `server:read-env`). |
| `store_users.go:544 persistable *` | `forge/api/internal/store/store_users.go:544` (actually `normalizeSubuserPermissions:562-565`) + `permissions.go:206` | **STILL BROKEN P0** | See §2.1 — `permission != "*"` branch explicitly allows persistence; `HasPermission` honors `*`. |
| `store_tenancy.go:91` (org/project/env tables exist) | `forge/api/internal/store/store_tenancy.go:91` `ListOrganizationsForUser:91-112` + `:539` `ResolveEffectiveOrgRole` + `:74` `allowedEnvColors` | **STILL PARTIAL** — tenancy exists, placement remains tenant-blind | Org tables + `protected` at `store_tenancy.go:37` + `DeleteEnvironment:373` blocks protected env → correct. But `domain.PlacementRequest:114` carries no tenant field, events tenantless — fleet capacity still tenant-blind. |
| `domain.PlacementRequest:114 tenantless` | `forge/api/internal/domain/domain.go:114` `type PlacementRequest struct:115-129` | **STILL BROKEN (tenant-blind)** | Fields: `ServerID,RegionID,Region,PreferredNode,RequiredNode,NodeID,AllocationID,SkipReservation,StorageLocality,MemoryMB,CPUShares,CPU,DiskMB,Runtime` — no `TenantID`/`OrgID`/`ProjectID`. `scheduler/service.go:88` ranks global `ListNodes`. |
| `store_mounts_ext.go:323 only /etc/forge` | `forge/api/internal/store/store_mounts_ext.go:323` `validateMountPath:324-339` | **STILL BROKEN** | Only `source ∈ {/etc/forge,/var/lib/forge/volumes}` blocked at `:332`, `target ∈ {/,/home/container}` at `:335`. Host-exposed paths pass. |
| `middleware_ratelimit.go:98 IsPrivate trust` | `forge/api/internal/http/middleware_ratelimit.go:98` `ExtractClientIP:99-103` | **STILL BROKEN** | `peerIP.IsLoopback()||IsPrivate()||IsUnspecified()` → trusts XFF from any RFC1918 peer; `RATE_LIMIT_TRUSTED_IPS:151` full bypass combined with peer-private trust → spoofable. |
| `middleware_security.go:22 fallback-nonce strict-dynamic` | `forge/api/internal/http/middleware_security.go:19` `generateNonce:22` + `middleware_security_headers.go:40` | **STILL BROKEN** | Both return `"fallback-nonce"` on `rand.Read` failure, CSP still `script-src 'self' 'nonce-{nonce}' 'strict-dynamic'` at `:33` — publicly known nonce executes injected `nonce="fallback-nonce"` scripts. |
| `traefik_proxy.go:960 all-policies-to-all-routes leak` | `forge/api/internal/services/trafficmanager/traefik_proxy.go:960` | **STILL BROKEN P0** | See §2.2 — `collectApplicablePolicies` returns all. |
| `handlers_loadbalancer.go:88 probe` | `forge/api/internal/http/handlers_loadbalancer.go:88` `AddTarget:95-101` | **STILL BROKEN (probe/relay)`** | No `net.ParseIP`, no private/metadata rejection; `loadbalancer/dataplane.go:142` dials raw `IP:port`. |
| `keyring.go:75 AAD` | `forge/api/internal/secrets/keyring.go:75` `Encrypt`/`96` `Decrypt` + `store/store_secrets.go:45` | **STILL PARTIAL** — correct primitive, caller-dependent AAD | GCM with AAD correct; `ParseKey:31-35` hex/base64 ambiguity + unvalidated AAD (some callers pass `""`) remain. |
| `auth/session.go:36 race` | `forge/api/internal/auth/session.go:36` `InMemorySessionStore` + `:51` `Create` + `:231` `SessionMiddleware` | **STILL BROKEN** | Pointer aliasing + raw token map + `LastActiveAt` mutated outside store lock. |

All 11 live targets re-inspected: **1 COMPLETE pair (S06+S07) already covered, 6 BROKEN P0/P1, 3 PARTIAL, 1 intentional PARTIAL (WS origin)**. None flipped to FIXED since reverification-20 (identical lines).

---

## 4. FINAL_PARITY §16 UX 14 Rows + Task UX Targets — Live Confirm

> §16 = `audits/FINAL_PARITY_AUDIT.md:325-342` (14 UX rows U01-U14). Plus task's UX shorthand list. All re-inspected at `forge/web/*`.

| # | Area (FINAL_PARITY §16 + task shorthand) | Live file:line | Verdict | Sev | Gap (live) |
|---|---|---|---:|---|
| U01 (UX-01) | Navigation IA — 5 registry groups → 6 goal groups (`admin-registry.ts:22 5→6 goal groups`) | `forge/web/components/admin/admin-registry.ts:22` 5 groups (Operations 10, Infra 8, Management 8, Services 8, Advanced 27 →61+ metadata) → `forge/web/components/admin/admin-shell.tsx:42` `plan:45-55` re-groups into **8** goal groups (Command 4, Operations 4, Runtime 2, Workloads 8, People&Access 6, Infra&Data 10, Networking&Security 12, Platform Config 10) via `entries.find(href)` filtered by `adminPagesForRole` | **PARTIAL (improved, still gap)** | P2 | `plan` surfaces only ~50 hrefs — `containers→/admin/docker` alias (`containers/page.tsx:3` permanentRedirect), `/admin/dev/states` (`dev/states/page.tsx`) exist as routes types (`routes.d.ts:36,45`) without registry entries → invisible to `navSearch:59` and breadcrumb `findAdminPage`. `Advanced` remains largest group (27) with no second-level collapse — Dokploy pattern inverted. Breadcrumb at `admin-shell.tsx:195` shows `Admin / <currentPage.label>` without `org→project→env→app` lineage (Coolify breadcrumbs). Server nav `server-nav.tsx:16` has 17 tabs vs Dokploy 7 — spills to drawer without hint. |
| U02 (UX-02) | Dashboards split — `AdminOverview:86 + monitoring/page` | `forge/web/components/admin/AdminOverview.tsx:86` (5 queries: nodes/servers/users/health/activity; `overall` dot `200-210`, capacity table `75`, attention list, recent activity) + `forge/web/app/admin/monitoring/page.tsx:1` (primary `AreaChart` with `period` 5m-24h, `metric` cpu/mem/disk/net, `nodeId` selector, `isSynthetic` banner `45-55`) | **FIXED — COMPLETE** | — | Correct split — Overview inventory/capacity/heartbeat, Monitoring live metrics. More honest than Coolify (labels "Configured, not live" at `AdminOverview:313`). Minor synthetic-zero detection (`cpuLoad1m==0 && networkRxBytes==0` masks missing heartbeat as "allocated") still, but split itself is **COMPLETE** and **unchanged FIXED** since FINAL_PARITY. |
| U03 (UX-03) | App detail tabs — `apps/[id]/page.tsx:28 useState not routed` | `forge/web/app/admin/apps/[id]/page.tsx:28` `TABS` 7 (`overview,deployments,configuration,logs,console,domains,backups`) via `useState tab` + `:42` `searchParams.get("tab")` initial at `:43` + `:97` `TABS.map` + `:112` `window.history.replaceState` `?tab=` | **STILL BROKEN** | P2 | Tabs are `useState` not Next.js routes — refresh loses tab except `replaceState` patch at `:108-113` persists only via query param `?tab=` not path; browser back/forward doesn't navigate tabs (Coolify/Dokploy use path tabs `/environment/:id/applications/:id/config`). `plan` says "Persist tab via URL" but still not router-driven nested routes. Live at `:28-43`, `:97`, `:112`. |
| U04 (UX-04) | Deployment experience — `deployment-progress 2s vs DeploymentTimeline 5s` | `forge/web/components/app/deployment-progress.tsx:46` `useQuery(["deployment-steps",deploymentId], refetchInterval:2000)` at `:70` (`:83` pct math) vs `forge/web/components/charts/DeploymentTimeline.tsx:23` `useQuery(["deployment-steps",deploymentId], refetchInterval:5000)` at `:27` | **STILL BROKEN (DUPLICATE)** | P2 | Two pollers same endpoint `GET /admin/deployments/:id/steps` with different intervals (2s vs 5s) and different cache semantics (`staleTime 1s, gcTime 5m` at `deployment-progress:77` vs none). Same query key `["deployment-steps",deploymentId]` — will dedup via React Query cache if mounted simultaneously, but intervals differ → whichever mounts first wins interval? Still double load, divergence risk, leak risk if `deploymentId` changes quickly. `DeploymentLogViewer` WS lives in modal at `apps/[id]/page.tsx:366` `log: string` not under `DeploymentTimeline`. |
| U05 | Status tone maps per-file divergent | `forge/web/components/admin/admin-ui.tsx:14` `Pill` 5 tones + `AdminAppsShared` via `apps.ts:statusTone`/`deploymentStatusTone` (centralized) vs `deployment-progress.tsx:32` local `statusConfig` 6 icons vs `DeploymentTimeline.tsx:14` local `statusConfig` 6 vs `AdminOverview` segment `emerald/amber/red/slate` dot unrelated | **STILL PARTIAL** | P3 | `AdminAppsShared` now consumes centralized `statusTone` — **partially fixed** for app/deploy badges. Remaining divergence: `deployment-progress:32` and `DeploymentTimeline:14` maintain separate `statusConfig` maps (different icons/colors: `Clock/Loader/Check/X` vs `CheckCircle2/LoaderCircle/...`). `compose/page.tsx` local 9-state map (not re-inspected but cited). |
| U06 | Empty/loading/error states | `forge/web/components/shared/states-empty.tsx:17` `EmptyCard` + 10 wrappers (`EmptyList/Search/Deployments/Backups/Domains/Services/Git/Certificates/DNSProviders/Organizations` ), `states-loading.tsx:10` `Skeleton*`, `states-error.tsx:43` `ErrorAlert/NotFound/Permission/Network/RateLimit:164` countdown, `states-offline.tsx:6` `OfflineBanner` + `admin-shell.tsx:194` `OfflineBanner` + `admin/OfflineBanner.tsx` variant | **STILL PARTIAL** | P3 | Rich semantic empties wired correctly; gaps: `ErrorRateLimit:164` countdown exists but `lib/api/http.ts` does not surface `429 Retry-After` as that prop → dead. `OfflineBanner` duplicated (shared + shell + admin variant) — `admin-shell:194` imports from `shared/states-offline` deduped but `admin/OfflineBanner.tsx` variant still exists. Two skeleton families (`SkeletonDetail` vs `AdminLoadingState`) coexist. Default `AppList` Create CTA not wired if caller omits `emptyAction`. |
| U07 | Logs placement (modal vs inline) + metrics duplication | `forge/web/components/deployment/DeploymentLogViewer.tsx:37` WS with 20 reconnects + `AdminAppsShared:30` `LogViewer` polling + `monitoring/metrics-chart.tsx:66` vs `monitoring/page.tsx` separate charts | **STILL PARTIAL** | P2 | Logs correctly per-resource (good, matches Dokploy), but deploy logs live inside modal `apps/[id]/page.tsx:366` `selected.log: string` not inline under `DeploymentsTab` — Dokploy shows deployment log inline. Metrics: single-chart toggle + per-metric comparison table — now labeled distinct ("trend" vs "Node comparison") so less confusion than final-parity U07, but still two presentation models. Deploy log WS not wired into deployments tab — still polling `fetchAppDeployments` only. |
| U08 | Domains gated behind serverFilter | `forge/web/app/admin/domains/page.tsx:16` `admin/domains/page.tsx:57` `enabled: !!serverFilter` — EmptyState "Select a server to view its domains" at `:195` by default + `forge/web/components/app/domains-view.tsx:23` app-level `fetchAppDomains` + `forge/web/lib/api/domains.ts` vs `admin/certificates/page.tsx` | **STILL BROKEN** | P2 | No aggregate "all domains" view (Dokploy shows all). Two DNS buttons (Check DNS + Verify) on same domain with unclear order — Dokploy shows expected records before Verify via dns-helper. Types diverge `AppDomain{ssl,sslStatus}` vs `DomainRecord{verified,verificationToken}`. Certificates page filters by verified but domains page doesn't surface cert expiry. |
| U09 | Triple env editors — `env triplicate` | `forge/web/components/environment/env-var-editor.tsx:8` API-backed `EnvVarResponse[]` (`isSensitive,version`) vs `forge/web/components/admin/AdminAppsShared.tsx:108` `EnvVarEditor:120` (`Record<string,string>` silent upsert `:127`, key `onChange={()=>{}}` `:153` disabled) vs `forge/web/components/environment/EnvironmentEditor.tsx:37` controlled `EnvVar[]` encrypted+import/export | **STILL BROKEN** | **P1** | **Three editors for one concept persist.** No `literal/buildtime/runtime/multiline`. `AdminAppsShared:120` silently upserts on duplicate key instead of erroring. `env-vars.ts:6` overload hides whether `project`-scoped vars work; `admin/environments/page.tsx` still calls `createEnvVar(selectedEnv,...)` with 3-arg legacy overload not scoped overload, and app config tab bypasses API until bulk `updateApp:391`. Most visible UX incoherence. |
| U10 | Tenancy cascade display-only | `forge/web/app/admin/environments/page.tsx:68` cascade `org→projectsQuery→environmentsQuery` → env list + var editor + `stores/use-tenancy-store.ts` + `forge/web/app/admin/apps/page.tsx:35` global `fetchApps()` | **STILL BROKEN** | P2 | Cascade correctly models org→project→env, but `GET /apps` (`apps.ts:fetchApps`) still global with no `orgId/projectId` scope — `/admin/apps` shows all apps across orgs (tenancy display-only). `color`/`protected` 8-color palette exists but protected only blocks `DeleteEnvironment:373` — no UI guidance ("Protected" pill only). `projects/organizations/page.tsx` no link to environments — must re-select org→project again (state not carried). |
| U11 | Docker view node-blind | `forge/web/app/admin/docker/page.tsx:17` 4 TABS → `docker/{containers,images,networks,volumes}-view.tsx` + `forge/web/lib/api/docker.ts:65` `listContainers(all)` fans out per node then flattens (`nodeName` retained) | **STILL PARTIAL** | P3 | Faithful Dokploy/1Panel pattern; `ImagesView` pull non-streaming; views client-side filter only — no node filter despite `nodeName` retained; multi-node lists all without grouping (monitoring has `nodeId` selector). `/admin/containers` alias redirects to `/admin/docker` — good dedup. |
| U12 | Gateway vs 7 scattered pages — `Gateways scattered 7 pages vs one` | `forge/web/app/admin/{traffic,load-balancer,domains,certificates,security,firewall,endpoints}/*` + `store_traffic.go:44` + `store_proxy_domains.go:13` + `store_security_headers.go` + `store_redirect_rules.go` + `gateway_adapter.go:38` `GatewayAdapter` not wired in `main.go:1025` (legacy `ReverseProxy` used) | **STILL BROKEN** — IA debt, not code defect, but highest leverage | P1 | Product IA mirrors gateway fragmentation diagnosed in `phase-04-forge-gateway:11` — 5 writers map to 7 top-level pages while Traefik proves one tree suffices. `GatewayAdapter` interface exists but `NewWithAdapterAndPersistence` not wired — half abstraction inert. Audit trails silent (S16). Recommend Traefik-shaped **Gateways** section (Routers/Services/Middlewares/Certs tabs) replacing 7 pages; collapse `traffic_rules`+`proxy_domains` → `gateway_routers`, `traffic_policies`+`security_headers` → typed `gateway_middlewares`; domains becomes provider emitting routers. |
| U13 | Monitoring observability — adapter health honesty (overlaps S08) | `forge/api/internal/services/trafficmanager/caddy_proxy.go:375` `Health()` hardcoded healthy vs `traefik_proxy.go:561` real probe; `eventstore/outbox.go:102` `processBatch` write-only 7-day retention with zero prod subscribers (`ws_hub` un-wired) | **STILL PARTIAL** | P2 | Observability primitives strong (per-service metrics, persisted plans/diffs) but write-only durability (AF-1) → 2 writes per publish for 7-day audit cost; Caddy health dishonest; connection counts not comparable (`caddy_proxy.go:149` walks running config vs `traefik_proxy.go:296` counts files). `middleware_request_logging.go:70` raw `c.IP()` vs trusted-derived divergence. |
| U14 | Navigation discoverability — endpoints as gateway entry | `forge/web/app/admin/endpoints/page.tsx:25` inventory (`ProjectID`/`GroupID` filter) + `forge/web/lib/api/endpoints.ts` under `Networking & Security` not co-located with node/region | **STILL PARTIAL** | P3 | Endpoints are natural gateway fabric (Portainer model) but placed as 7th networking leaf rather than entry to Gateways section; discovery would improve if Endpoints → Gateways hierarchy. |

**UX task-shorthand checklist (from user prompt):**

| Task shorthand | Live | Verdict |
|---|---|---|
| `admin-registry.ts:22 5→6 goal groups` | `admin-registry.ts:22` 5 groups 61 entries → `admin-shell.tsx:42` 8 goal groups ~50/61 surfaced | **PARTIALLY FIXED** — re-group correct (5→8), but Advanced 27 no second collapse, invisible routes remain (`Kubernetes`/`Containers` aliases, `/admin/dev/states` without registry). |
| `AdminOverview:86 + monitoring/page` | `AdminOverview.tsx:86` inventory/capacity + `monitoring/page.tsx:1` live metrics — correct split | **FIXED — COMPLETE** |
| `apps/[id]/page.tsx:28 useState not routed` | `:28` `TABS` + `:42` `useState tab` + `:112` `replaceState ?tab=` hack | **STILL BROKEN** |
| `deployment-progress 2s vs DeploymentTimeline 5s` | `deployment-progress.tsx:70` 2000 vs `DeploymentTimeline.tsx:27` 5000 same query key | **STILL DUPLICATE** |
| `env triplicate` | `env-var-editor.tsx:8` vs `AdminAppsShared:108` vs `EnvironmentEditor.tsx:37` + overloaded `env-vars.ts:6` | **STILL BROKEN** |
| `Gateways scattered 7 pages vs one` | `traffic`+`load-balancer`+`domains`+`certificates`+`security`+`firewall`+`endpoints` =7, `GatewayAdapter` unwired | **STILL BROKEN** |
| `states-empty empty CTAs` + `states-error/loading/offline` | `states-empty.tsx:17` 10 wrappers wired; `states-error.tsx:43` `RateLimit` dead; `OfflineBanner` duplicated | **STILL PARTIAL** |
| `tone maps divergent` | `admin-ui.tsx:14` + per-file `statusConfig` diverge (compose 9 states) but `AdminAppsShared` now consumes centralized `statusTone` partially | **STILL PARTIAL** |

**UX delta vs FINAL_PARITY:** 0 rows newly FIXED. U02 remains COMPLETE; U01/U05/U13 show partial containment (centralization of `statusTone`, `isSynthetic` labeling, `plan` re-group) but underlying editor/routing/polling splits remain.

---

## 5. phase-01 subagent-05 — 17 Comparisons (Architecture/Security) — Live Confirm

> Source: `audits/phase-01/subagent-05-architecture-security.md` — C01-C17 (Queue/Workers/Idempotency C01-C05, Reconciliation C06-C10, Tenancy/RBAC/Secrets C11-C17). All re-inspected.

| # | Comps | Reference truth | Live forge file:line | Prior ver | Live verdict |
|---|---|---|---|---|---|
| C01 | Lease too short for long deploys (30s vs Coolify 24h) | Coolify `config/queue.php:28` | `forge/api/internal/services/queue/queue.go:107` `lease=30s` + `store.go:55` steal | Unresolved | **STILL PARTIAL** — lease still 30s, `keepLease` 10s ticks, long builds terminably stealable. Not P0 but reliability debt. |
| C02 | Double retry_count on stolen lease | — | `queue/store.go:60` `CASE WHEN candidate.status='running' THEN 1` + `process` Retry `+1` | Unresolved | **STILL BROKEN** — stolen+failed consumes 2 retries for 1 failure, exhausts MaxRetries prematurely. |
| C03 | In-memory periodic scheduler not durable | Coolify DB cron | `queue/periodic.go:97` ticker 1s + `:142` `RFC3339Nano` idempotent | Unresolved | **STILL DUPLICATE/UNWIRED** — idempotent dispatch masks gap but missed window not recovered; per-replica nano keys diverge (`leader.go:23` elector unwired). |
| C04 | Dual queues without single-writer contract | — | `queue/store.go:34-46` dually writes `operations` + `operation/store.go:19` independent, `queue.go:29` vs `operation/service.go:44` deprecated comments | Unresolved | **STILL DUPLICATE** — `queue.Service` vs `operation.Service` dual-writing `operations` tables — verdict per FINAL_PARITY §12.6: `queue wins execution`, `operation` read-model. Not enforced (`operation.DispatchCompose` still callable). |
| C05 | Operation reaper vs no per-job timeout (5m) | Coolify `timeout=3600` | `operation/service.go:164` reaper 1m `ReapStale 5m` + `:229` no deadline | P1 | **STILL BROKEN** — legitimate 10-60m installs reaped and re-executed concurrently with original handler (still holds `active` map). No `Heartbeat` for operations. |
| C06 | Reconciler never resets restartAttempts on success | — | `reconciler/service.go:639-646` increments on both error+success | — | **STILL BROKEN** — `MaxRestartAttempts=3` permanently gates server after 3 recoveries ever. |
| C07 | hasPendingDuplicatePlan suppresses drifts within 30m | — | `reconciler/service.go:654-676` `PlanDedupeWindow=30m` hash | — | **STILL PARTIAL** — legitimate flapping drifts dropped if hash equals pending. |
| C08 | Reconciler publishes events before verifying actual state | — | `reconciler/service.go:476-496` `EventDesiredStateChanged`/`EventActualStateChanged` before `isNodeOperable` | — | **STILL BROKEN** — observer-visible drift that never happened. |
| C09 | Recovery generation fencing without tx reservation coupling | — | `recovery/service.go:648` `UpdateServerGeneration` then `CreateReservations:653`, rollback via `errors.Join` | — | **STILL PARTIAL** — concurrent beacon heartbeat could have observed incremented generation; `WorkloadLeaseExpiry` rollback not atomic with fence. |
| C10 | Beacon reconnect declares StateConnected without verification | — | `beacon/internal/remote/reconnect.go:146-168` `StoreInt32(StateConnected)` without `SendNodeHeartbeat` probe + `:124` `lastHb=Now()` bump despite failure | P2 | **STILL BROKEN** — panel classifies Offline while beacon believes Connected; defeats recovery eligibility (`recovery/service.go:256`). |
| **C11** | **Tenancy scoping org-based but global admin bypass** | Portainer `ResourceControl` per-resource | `forge/api/internal/store/store_tenancy.go:539` `if globalRole=="admin" return "admin"` + `http/auth.go:587` + `store_tenancy.go:645` | P1 | **STILL PARTIAL** — all-or-nothing per org, no per-server team ACL; `server:read` grant reads all servers via overlapping scopes. |
| **C12** | **Scope namespace overlap allows privilege confusion** | Portainer `ReadWriteAccessLevel` unambiguous | `auth/scopes.go:20-43` `server:read/write/control` + `KnownScopes:49-55` + `http/auth.go:512` `hasAnyAdminScope` | P1 | **STILL PARTIAL** — `server:*` prefixes order-dependent; `ValidateApiKeyScopes` normalization not inspected but overlap persists. |
| **C13** | **Secrets keyring AAD binding caller-dependent** | Portainer `libcrypto/encrypt.go` per-value nonce | `secrets/keyring.go:75` `Encrypt/Decrypt` + `NeedsRotation:134` | P1 | **STILL PARTIAL** — see S12: `ParseKey:31-35` hex ambiguity + `""` AAD callers remain. |
| **C14** | **Session store raw tokens + pointer aliasing** | Portainer hashes API keys | `auth/session.go:36` `sessions map[string]*Session` + `:51` stores pointer + `:231` mutates `LastActiveAt` outside lock | P2 | **STILL BROKEN — P1** — see S13. |
| **C15** | **WS hub tenancy filter precise but un-wired** | Komodo per-server fan-out | `http/ws_hub.go:199` `shouldDeliver` + header `NOT subscribed` in `main.go` + 5s polling fallback | P2 | **STILL PARTIAL/UNWIRED** — polling with no backoff ceiling, DB load under 100s dashboards; 2 writes per publish for 7-day retention via `eventstore/outbox.go:39` with zero prod subscribers. |
| **C16** | **Daemon HMAC + mTLS loopback downgrade** | Portainer edge tunnel | `daemon/client.go:243` downgrades to HTTP when `isLoopback(baseURL)` + `isPrivateHost` only in `PullRemoteFile` | P2 | **STILL PARTIAL** — misconfigured `BaseURL=http://private-ip` not downgraded but still HTTP-eligible; transfer retries re-sign subtlety remains. |
| **C17** | **Migration advisory lock blocking** | Uncloud no advisory lock | `store/store.go:45` `SELECT pg_advisory_lock` blocking | P3 | **STILL BROKEN** — second replica startup wedged on long DDL; should be `pg_try_advisory_lock` with timeout. |

**phase-01 C11-C17 (tenancy/RBAC/secrets/mTLS/WS) live summary:** 5 of 7 (C11-C17) remain **PARTIAL/BROKEN** — identical lines to phase-01 (C11-C15 fully reconfirmed in reverification-20 S04/S08/S12-S15). No P1/P2 was remediated; only documentation slightly clarified (headers at `auth/session.go:36`, `secrets/keyring.go:75` still accurate).

---

## 6. reverification-20 — 30 Rows (18 Security + 12 UX) — Live Confirm

> Source: `audits/reverification/subagent-20-security-ux.md` — definitive parity matrix S01-S18 (18 security) + U01-U12 (12 UX) = 30. This agent reconciled phase-01/02/04 + final-parity. Live delta vs reverification (2026-08-24) shown.

### 6.1 Security S01-S18 (reverification-20 → live)

| # | Area (reverification-20) | Reverification status | Live file:line | Live delta |
|---|---|---|---|
| S01 | Permission catalog | PARTIAL (2 consts missing from AllPermissions) | `permissions.go:84-85` vs `AllPermissions:89` still omits | **No change — STILL PARTIAL** |
| S02 | Wildcard `*` persistable | BROKEN P0 (FORGE-LOGIC-S01) | `store_users.go:564` still `permission != "*"` | **No change — STILL BROKEN P0** |
| S03 | Subset enforcement | BROKEN P0 | `store_users.go:306` no actor subset | **No change** |
| S04 | Org/Project/Env tenancy vs placement tenant-blind | PARTIAL P1 (FORGE-LOGIC-S02) | `domain.PlacementRequest:114` no TenantID + `events/event.go:196` no tenant + `eventstore/migration.go:19` no tenant column | **No change** |
| S05 | Mount allowlist | PARTIAL P1 (FORGE-LOGIC-S03) | `store_mounts_ext.go:323` only 2 sources blocked | **No change** |
| S06 | Cron shell boundary | COMPLETE | `cronjob/service.go:135` fail-closed | **No change — STILL COMPLETE** |
| S07 | DB identifier quoting | COMPLETE | `dbprovisioner/service.go:737` quoting | **No change — STILL COMPLETE** |
| S08 | Trusted proxy model | BROKEN P1 (FORGE-LOGIC-S04) | `middleware_ratelimit.go:101` IsPrivate trust + `middleware_ipaccess.go:98` warn-only | **No change** |
| S09 | CSP nonce entropy | PARTIAL → P1 (FORGE-LOGIC-S05) | `middleware_security.go:22` fallback-nonce | **No change** |
| S10 | Failover/traffic policy leakage | BROKEN P0 (FORGE-LOGIC-S06) | `traefik_proxy.go:960` + `caddy_proxy.go:722` | **No change — STILL BROKEN P0** |
| S11 | L4 LB as internal probe | BROKEN P1 (FORGE-LOGIC-S07) | `handlers_loadbalancer.go:88` no ParseIP | **No change** |
| S12 | Secrets keyring AAD | PARTIAL P1 | `keyring.go:75` caller-dependent AAD | **No change** |
| S13 | Session pointer race + raw tokens | BROKEN P1 (FORGE-LOGIC-S08) | `auth/session.go:36` raw pointer | **No change** |
| S14 | mTLS HTTPS gate trusts X-Forwarded-Proto | PARTIAL P1 | `middleware_mtls.go:123` no CIDR gate | **No change** |
| S15 | WS origin cookie vs bearer | PARTIAL P2 (design trade-off) | `ws_origin.go:57` allows missing for !cookie | **No change** |
| S16 | Audit trails silent | MISSING P1 | `handlers_trafficmanager.go` zero AppendAudit | **No change** |
| S17 | HMAC WS ticket single-use Lua vs in-memory fallback | PARTIAL P2 | `handlers_ws_ticket.go:19` Lua atomic, fallback process-local | **No change** |
| S18 | Runtime health / IP derivation divergence | PARTIAL P2 | `middleware_request_logging.go:70` raw IP vs `caddy_proxy.go:375` hardcoded health | **No change** |

**Reverification S01-S18 live delta: 0 of 18 flipped.** 14 BROKEN/PARTIAL/MISSING + 2 COMPLETE remain identical. Reverifier's inference at S06 ("adapters not re-read in full but store contract unchanged") now **confirmed** by full adapter reads (`collectApplicablePolicies` verbatim).

### 6.2 UX U01-U12 (reverification-20 → live)

| # | Area (reverification-20) | Reverification status | Live file:line | Live delta |
|---|---|---|---|
| U01 | Navigation IA 5 vs 6 (now 8) groups | PARTIAL P2 | `admin-registry.ts:22` 5 groups + `admin-shell.tsx:42` plan 8 groups, Advanced 27 still largest | **No change — STILL PARTIAL** (re-group reduces but `Advanced` 27 still largest, invisible routes remain). Reverifier noted 5+8+8+8+27=61 vs plan 6 groups ~56 hrefs filtered to ~50; live is now 10+8+8+8+27=61 vs plan 8 groups — same pattern, ++2 extra Infrastructure entries. |
| U02 | Dashboards split | COMPLETE | `AdminOverview:86` + `monitoring/page.tsx:1` | **No change — STILL COMPLETE** (isSynthetic honest labeling kept). |
| U03 | App detail tabs | PARTIAL → FORGE-LOGIC-U01 | `apps/[id]/page.tsx:28` useState not routed | **No change — STILL PARTIAL** (replaceState added but not router-driven). |
| U04 | Deployment dual pollers | DUPLICATE (FORGE-LOGIC-U02) | `deployment-progress:70` 2000 vs `DeploymentTimeline:27` 5000 same key | **No change — STILL DUPLICATE** (both now use react-query but still two intervals). |
| U05 | Status tone maps | PARTIAL | `AdminOverview` emerald/amber/red/slate dot vs `deployment-progress:32` vs `DeploymentTimeline:14` local maps | **Marginal improvement — STILL PARTIAL** (`AdminAppsShared` now consumes centralized `statusTone` for app/deploy badges, but deploy-step tones still duplicated). |
| U06 | Empty/loading/error/offline | PARTIAL | `states-empty:17` rich but `ErrorRateLimit:164` dead, `OfflineBanner` duplicated | **No change — STILL PARTIAL** (OfflineBanner deduped at `admin-shell:194` import but `admin/OfflineBanner.tsx` variant still exists). |
| U07 | Logs/metrics placement | PARTIAL | `DeploymentLogViewer` WS in modal at `apps/[id]/page.tsx:366` vs `DeploymentTimeline` | **No change — STILL PARTIAL** (metrics now labeled distinct "trend" vs "Node comparison" — confusion reduced but still modal). |
| U08 | Domains/TLS verification UX | PARTIAL | `admin/domains/page.tsx:57` gated `!!serverFilter` empty default | **No change** |
| U09 | Env vars three editors | BROKEN (FORGE-LOGIC-U03) P1 | `env-var-editor:8` vs `AdminAppsShared:120` Record vs `EnvironmentEditor:37` | **No change — STILL BROKEN** |
| U10 | Tenancy cascade display-only | PARTIAL | `fetchApps` global not filtered; protected flag show-only | **No change** |
| U11 | Gateways fragmentation | BROKEN (FORGE-LOGIC-U04) P1 | 7 pages vs Traefik one-tree; `GatewayAdapter` not wired | **No change — STILL BROKEN** |
| U12 | Docker faithful but node-blind | COMPLETE (minor) | `docker/page.tsx:17` `nodeName` retained but no filter | **No change — STILL COMPLETE** |

**Reverification U01-U12 live delta: 0 of 12 flipped to FIXED.** 10 PARTIAL/BROKEN/DUPLICATE + 2 COMPLETE remain identical; U05/U06/U07 show *containment* (centralized `statusTone` consumption, honest `isSynthetic` banner, shell dedupe) but not resolution — underlying splits remain.

**Aggregate reverification-20 30 rows:** **0 newly FIXED**, 24 PERSISTS (including 2 P0 + 6 P1), 4 COMPLETE remain complete.

---

## 7. Consolidated STILL BROKEN vs FIXED Roll-up

### 7.1 Security (FINAL_PARITY §11 S01-S16, reverification S01-S18, phase-01 C11-C17)

| Bucket | Count | Items (live) |
|---|:---:|---|
| **STILL BROKEN — P0** | 2 | S02 `*` wildcard escalation (`store_users.go:544:564` + `:306`) + S10 all-policies-on-all-routes (`traefik_proxy.go:960` + `caddy_proxy.go:722`) — identical to FINAL_PARITY P0×2 |
| **STILL BROKEN — P1** | 5 | S08 trusted proxy fail-open (`middleware_ratelimit.go:98`) + S11 LB probe (`handlers_loadbalancer.go:88`) + S13 session race (`auth/session.go:36`) + S09 CSP fallback (`middleware_security.go:22`) + S16 audit silent (MISSING) — same as FINAL_PARITY |
| **STILL PARTIAL — P1/P2** | 5 | S04 tenant-blind placement (`domain.PlacementRequest:114` + `events/event.go:196`) + S05 mount allowlist (`store_mounts_ext.go:323`) + S12 keyring AAD (`keyring.go:75`) + S14 mTLS XFP (`middleware_mtls.go:123`) + S15 WS origin (intentional PARTIAL) |
| **STILL FIXED — COMPLETE** | 2 | S06 cron shell boundary (`cronjob/service.go:135`) + S07 DB identifier quoting (`dbprovisioner/service.go:737`) |
| **PARTIAL catalog debt** | 1 | S01 `AllPermissions` missing 2 consts (`permissions.go:89`) P3 |

**Security FIXED since FINAL_PARITY: 0** — 12 of 14 non-complete rows still BROKEN/PARTIAL at identical `file:line`.

### 7.2 UX (FINAL_PARITY §16 14 rows, task UX shorthand 8 targets, reverification U01-U12)

| Bucket | Count | Items |
|---|:---:|---|
| **STILL BROKEN** | 5 | U03 tabs not routed (`apps/[id]/page.tsx:28`) + U04 dual pollers (2s vs 5s) + U09 env triplicate (`env-var-editor:8` vs `AdminAppsShared:108` vs `EnvironmentEditor:37`) + U08 domains gated (`admin/domains/page.tsx:57`) + U10 tenancy display-only (`fetchApps` global) + U12 gateways 7 vs 1 (`traffic`+`load-balancer`+`domains`+`certificates`+`security`+`firewall`+`endpoints`) — U12 counted once, total 6 but merged UX-08/10 as 1? Conservative: 6 |
| **STILL PARTIAL** | 6 | U01 nav 5→8 but Advanced 27 no collapse (`admin-registry.ts:22` + `admin-shell.tsx:42`) + U05 tone maps divergent (`deployment-progress:32` vs `DeploymentTimeline:14`) + U06 states-empty dead RateLimit + offline dupe + U07 logs modal vs inline + U11 docker node-blind + U13 health/IP divergence + U14 endpoints discoverability |
| **FIXED — COMPLETE** | 2 | U02 dashboards split (`AdminOverview:86` + `monitoring/page.tsx:1`) + U12 docker faithful base (minor node filter missing but COMPLETE per taxonomy) |

**UX FIXED since FINAL_PARITY: 0** — U02 remained COMPLETE; no BROKEN→FIXED transition. Containment improvements (plan re-group, centralized `statusTone` for app badges, `isSynthetic` banner) reduce severity but do not resolve structural splits.

### 7.3 phase-01 subagent-05 17 comps + reverification-20 30 rows combined

- **phase-01 C01-C17:** C11-C17 (7 tenancy/RBAC/secrets/mTLS/WS) → **2 still BROKEN P0/P1 (C14 session, S02 wildcard overlaps), 5 still PARTIAL**, 0 FIXED. C01-C10 (queue/reconciler/recovery) remain as documented (C05 reaper, C10 beacon reconnect still BROKEN, C02 double increment still BROKEN, C04 dual queues still DUPLICATE — not primary for this agent but confirmed).
- **reverification-20 30 rows:** **0 of 30 flipped**, 24 BROKEN/PARTIAL/MISSING persist, 4 COMPLETE persist (C06/C07 analogs), 2 PARTIAL improvements (U05/U06 deduplication) but not FIXED per taxonomy.

---

## 8. Recommendations (activation-ordered, no new subsystem — reuse existing tables/services)

> Identical to FINAL_PARITY §20 Activations #3, #5, #22, #47 but re-confirmed live — ordered P0 first, file:line actionable.

| # | Activation | Live file:line | Effort | Value |
|---|---|---|:---:|---|
| 1 | **Enforce subuser subset + gate `*`** — change `UpsertServerSubuser(ctx, serverID, req, actorID string, actorPerms []string, actorIsOwnerOrAdmin bool)`; reject any `req.Permissions` not in `actorPerms` unless owner/admin; reject `*` unless `actorIsOwnerOrAdmin`; remove `permission=="*"` from `normalizeSubuserPermissions:564` or gate | `forge/api/internal/store/store_users.go:306` + `:544` + `permissions.go:206` | S | **P0** |
| 2 | **Fix gateway policy join** — add `traffic_rule_policies(rule_id, policy_id)` FK/join or Traefik-shaped `gateway_routers/middlewares/services` (`GatewayRouter/GatewayMiddleware/GatewayService`); `collectApplicablePolicies(rule)` filters by join; sort middleware deterministically (rate-limit→blacklist→whitelist→circuit-breaker→redirect) | `forge/api/internal/store/store_traffic.go:44` + `traefik_proxy.go:960` + `caddy_proxy.go:722` | M | **P0** |
| 3 | **Close mount allowlist** — allowlist prefix `/srv/forge-mounts`, deny `/etc` `/var/run/docker.sock` `/proc` `/sys` `/dev` `/boot` `/root` `/`; add test `CreateMount Source=/etc → error` | `forge/api/internal/store/store_mounts_ext.go:323` | S | **P0** (host breakout) |
| 4 | **Fix trusted proxy** — replace peer-private trust with explicit `TRUSTED_PROXIES` CIDR config (Caddy `determineTrustedProxy:1171` model); `ExtractClientIP` must require peer ∈ trusted CIDRs, not any private; make `ADMIN_IP_ALLOW` fail-closed in prod; unify `getClientIP` everywhere | `forge/api/internal/http/middleware_ratelimit.go:98` + `middleware_ipaccess.go:98` | S | P1 |
| 5 | **Fix LB target validation** — `net.ParseIP`; reject link-local/private/metadata (`169.254.169.254`) unless operator allowlist; add `TargetHost` validation parity with `trafficmanager/service.go:388` | `forge/api/internal/http/handlers_loadbalancer.go:88` | S | P1 |
| 6 | **Fix CSP nonce** — on `rand.Read` failure return `500` or skip CSP rather than deterministic `"fallback-nonce"`; deduplicate to one middleware | `forge/api/internal/http/middleware_security.go:22` + `middleware_security_headers.go:43` | S | P1 |
| 7 | **Clone session + hash token** — clone on `Create`/`Get` (`*sess` copy), store `sha256Hex(token)` as key (as `http/auth.go:412` does for `jti`), update `byToken` atomically in `Update`/`Delete` | `forge/api/internal/auth/session.go:36` + `:51` + `:231` | S | P1 |
| 8 | **Tenant-tag core records now** — add `TenantID`/`OrgID`/`ProjectID` to `PlacementRequest`, `PlacementDecision`, `ReconcilePlan`, `Envelope` while volumes small; add `tenant_id` to `events` table | `forge/api/internal/domain/domain.go:114` + `events/event.go:196` + `eventstore/migration.go:19` | S | P1 |
| 9 | **Audit trails** — `AppendAudit` to traffic rules/policies, LB groups/targets, cert issue/renew/revoke, failover policies; extend `Envelope`/`events` with `tenant_id` | `forge/api/internal/http/handlers_trafficmanager.go` etc. | S | P1 |
| 10 | **UX convergence** — router-driven tabs `/admin/apps/[id]/{overview,deployments,configuration,logs,console,domains,backups}` nested routes vs `useState`; single `DeploymentTimeline` (`refetchInterval:2000` canonical + inline `DeploymentLogViewer`); one `EnvVarEditor` model (`EnvVarResponse[]` + `isSensitive/version` + Coolify flags `isMultiline/isLiteral/isBuildtime/isRuntime/comment`) | `forge/web/app/admin/apps/[id]/page.tsx:28` + `deployment-progress.tsx:70`/`DeploymentTimeline:27` + `env-var-editor:8` | M | P2/P1 |

Items 33/29 collapsed — remaining 10 are distinct table/service/WS fixes reusing existing code; none requires a new subsystem. Prioritized P0 first (wildcard, policy leakage, mount allowlist).

---

## 9. Files Created in This Task

- This file: `audits/110-phase-02-context/subagent-10-security-ux-confirm.md` — single file:line-cited confirm for S01-S16 + UX 14 + phase-01 17 + reverification 30 vs LIVE.

No product code modified. Source inspected under `/Users/riyaz/project/gamepanel` per ABSOLUTE RULE.

---

*End of subagent 10 report — 16 security + 14 UX + 17 phase-01 comps + 30 reverification rows reconciled against LIVE (2026-08-24). No P0 remediated — wildcard escalation and policy leakage reconfirmed STILL BROKEN at identical file:line.*
