# Phase 4 / Subagent 5 — Network Security · Reliability · Observability (caddy · traefik · nginx-proxy-manager vs Forge)

Cluster: `reference/networking/{caddy,traefik,nginx-proxy-manager}`
Dimension: network security (header injection, open redirect via proxy, cert validation, DNS SSRF), reliability (failover, circuit breaker, retry), observability (access logs, metrics).
All paths relative to repo root. No product code was modified.

---

## 1. Executive summary

Forge's networking layer implements the right *vocabulary* (rate limits, IP allow/deny, mTLS, health probes, failover policies) but several implementations diverge from the reference gateways in ways that are exploitable or self-defeating:

- **Rule/URL injection**: routing-rule `Path` is interpolated unescaped into Traefik rule expressions (`traefik_proxy.go:797`); ACME domains are interpolated into Caddy admin URL paths (`caddy_tls.go:74-75`, `caddy_proxy.go:92-93`).
- **Global policy leakage**: every traffic policy (IP whitelist/blacklist, rate limit, circuit breaker) is applied to **every** route, not just its owning domain (`traefik_proxy.go:960-972`, `caddy_proxy.go:722-747`) — one tenant's ACL restricts or unlocks another's.
- **Failover oscillation**: unhealthy upstreams are re-added by every full route sync because route builders ignore the adapters' own health maps — a self-sustaining add/remove flapping loop at the reconcile interval.
- **Cert mishandling**: the Traefik adapter writes PEM *bodies* into `certFile`/`keyFile` *path* fields so certificates silently never load; `RevokeCertificate` only deletes the DB row and never revokes with the CA; the Caddy TLS manager replaces the whole `gamepanel-domains` server per call, wiping previously provisioned domains.
- **DNS SSRF surface**: user-supplied `PDNS_API_URL` / `RFC2136_NAMESERVER` credentials are used with zero scheme/host validation, and provider construction mutates process-global env vars under a lock other goroutines can observe.
- **Observability gaps**: access logs record raw `c.IP()` while rate limiting uses a different (proxy-aware) derivation; Caddy adapter health is hardcoded `healthy`; "active connections" returns file counts.

15 comparisons in §4, 13 logic findings in §5.

---

## 2. Scope inspected

Forge:
- `forge/api/internal/http/middleware_security.go`, `middleware_security_headers.go`, `middleware_mtls.go`, `middleware_ratelimit.go`, `middleware_ipaccess.go`, `middleware_metrics.go`, `middleware_request_logging.go`, `ws_origin.go`
- `forge/api/internal/http/handlers_certificates.go`, `handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_failover.go`
- `forge/api/internal/services/acme/{service.go,providers.go}`, `forge/api/internal/services/dns/service.go`
- `forge/api/internal/services/trafficmanager/{service.go,caddy_proxy.go,caddy_tls.go,caddy_admin.go,traefik_proxy.go}`
- `forge/api/internal/services/loadbalancer/{service.go,dataplane.go}`
- `forge/api/internal/services/failover/service.go`, `crossnode/{ingress_sync.go,resolver.go}`, `heartbeatmonitor/service.go`

Reference:
- caddy: `modules/caddyhttp/server.go` (trusted proxies, strict SNI, logRequest), `modules/caddyhttp/reverseproxy/healthchecks.go`
- traefik: `pkg/server/middleware/middlewares.go` (chain building), `pkg/middlewares/{ratelimiter,ipwhitelist,snicheck,redirect}/`
- nginx-proxy-manager: `backend/internal/{access-list.js,audit-log.js,certificate.js,ip_ranges.js}`

---

## 3. Reference hardening inventory

### 3.1 Caddy
- **trusted_proxies**: client IP derived from forwarded headers *only* when the direct peer matches explicitly configured CIDR ranges; otherwise the socket address wins (`modules/caddyhttp/server.go:1171-1236`, `determineTrustedProxy`). Strictness levels (`TrustedProxiesStrict`, server.go:224) walk the XFF chain skipping only trusted hops (`strictUntrustedClientIp`).
- **strict SNI**: when enabled (auto-enabled for servers with TLS ClientAuth, `app.go:282-286`), `enforcementHandler` compares TLS SNI to HTTP Host and answers `409 MisdirectedRequest` + closes the connection on mismatch (`server.go:663-678`) — defeats "benign SNI, restricted Host" bypasses on shared listeners.
- **Access logs**: centralized `logRequest` with per-server `ServerLogConfig` (`server.go:233-236, 981+`); logs use the trusted-derived client IP.
- **Health checks**: active checks with interval/port/expectation plus passive checks with fail-duration cooldown (`reverseproxy/healthchecks.go:67, 230-268`).

### 3.2 Traefik
- **Middleware chain ordering**: routers carry ordered `Middlewares []string`; `BuildMiddlewareChain` appends constructors in declared order with recursion detection (`pkg/server/middleware/middlewares.go:50-83`) — order explicit, deterministic, validated.
- **ipWhiteList/ipAllowlist**: fails closed if `sourceRange` empty (`ipwhitelist/ip_whitelist.go:31-34`), rejects 403, evaluates against per-router `ip.Strategy` (depth/excludedIPs).
- **rateLimit**: token bucket (`golang.org/x/time/rate`) with `SourceCriterion` and `maxSources = 65536` cap on tracked buckets (`ratelimiter/rate_limiter.go:24, 44-52`) — bounds memory against source-spoofing floods.
- **snicheck**: `421 MisdirectedRequest` when router TLS options differ from connection SNI (`snicheck/snicheck.go:27-42`).
- **redirect**: rewritten URL parsed via `url.Parse` before emitting `Location`; method-aware status codes (`redirect/redirect.go:56-77`).

### 3.3 nginx-proxy-manager
- **Access lists**: per-proxy-host lists combining basic-auth items *and* client allow/deny directives with `satisfy_any`; create/update regenerates nginx config (`access-list.js:29-80`).
- **Audit log**: `internalAuditLog.add` records `user_id`, `action`, `object_type/id`, `meta` for every mutation (`audit-log.js:84-103`); access-list creation writes audit entries (`access-list.js:78-81`).
- **Certificate handling**: uploads restricted to `allowedSslFiles` (`certificate.js:47-48`); certbot-managed issuance/renewal timer; DNS provider credentials masked in API responses (`omissions()` includes `meta.dns_provider_credentials`, certificate.js:41).
- **Trusted proxy ranges**: Cloudflare/CloudFront ranges fetched over pinned HTTPS URLs on a timer → nginx `real_ip` config (`ip_ranges.js:12-16, 47-60`) — curated, not "any private IP".

---

## 4. Comparison matrix (Forge vs references)

| # | Area | Reference behavior | Forge behavior | Verdict |
|---|------|--------------------|----------------|---------|
| 1 | Security header defaults | Caddy adds nothing implicitly; Traefik `headers` middleware opt-in/explicit | Two overlapping middlewares (`middleware_security.go:27-62` sets CSP+HSTS+XFO always; `middleware_security_headers.go:48-93` configurable variant) that must be kept in sync | Divergent |
| 2 | CSP nonce entropy | Traefik CSP set verbatim | Both Forge middlewares fall back to constant `"fallback-nonce"` if `crypto/rand` fails (`middleware_security.go:21-25`, `middleware_security_headers.go:40-46`) | Weaker |
| 3 | Trusted proxy model | Caddy: explicit CIDR allowlist required before honoring XFF (`server.go:1171-1236`); NPM: curated vendor ranges (`ip_ranges.js`) | `ExtractClientIP` trusts forwarded headers whenever direct peer is *any* loopback/private/unspecified IP (`middleware_ratelimit.go:98-119`) — any RFC1918 container/LAN host can rotate identities | Weaker |
| 4 | Admin/API IP access | Traefik ipWhiteList fails closed on empty range | `AdminIPAccessConfig`/`APIIPAccessConfig` default `TrustProxy:true` and no-op when env vars unset (`middleware_ipaccess.go:98-135`) — warn-only fail-open | Divergent |
| 5 | Strict SNI / host-SNI binding | Caddy `StrictSNIHost` (`server.go:663-678`); Traefik `snicheck` (`snicheck.go:27-42`) | No equivalent: gateway routes match Host only; SNI↔Host mismatch never checked | Missing |
| 6 | Rate limiter internals | Token bucket, bounded `maxSources=65536`, delay shaping (`rate_limiter.go:24-70`) | Fixed-window Redis INCR+EXPIRE with TOCTOU-safe overflow rejection (`middleware_ratelimit.go:212-228` — good), but in-memory fallback buckets are unbounded per key (`middleware_ratelimit.go:39-93`) keyed by attacker-influenced IP strings | Mixed |
| 7 | Trusted-IP bypass | Traefik has no implicit bypass | `RATE_LIMIT_TRUSTED_IPS` fully bypasses limiting (`middleware_ratelimit.go:151-154`); combined with #3, spoofable behind private proxies | Weaker |
| 8 | Middleware ordering semantics | Explicit ordered list, recursion-checked (`middlewares.go:50-83`) | Adapter builds middleware list by iterating a Go **map** of policies (`traefik_proxy.go:769-787`) → non-deterministic execution order across syncs | Weaker |
| 9 | Policy attachment model | Per-router named references (Traefik dynamic config router.Middlewares); NPM access lists bound per proxy host | `collectApplicablePolicies` returns ALL policies for EVERY rule (`traefik_proxy.go:960-972`); Caddy path prepends all policy handles onto all routes (`caddy_proxy.go:722-747,771-793`) | Broken |
| 10 | HTTPS redirect placement | Caddy catch-all redirect route on :80 (Forge's own Caddy adapter has this: `caddy_proxy.go:749-769`); Traefik redirectScheme belongs on the HTTP router | Traefik adapter attaches `redirectScheme` only to routers already on `websecure` entrypoint (`traefik_proxy.go:783-794,949-956`) — HTTP traffic never reaches it; plain-HTTP requests 404 instead of redirecting | Inconsistent across adapters |
| 11 | Redirect target validation | Traefik parses rewritten URL before Location (`redirect.go:56-77`) | Caddy adapter emits `https://{http.request.host}{http.request.uri}` placeholder (Caddy-sanitized, acceptable); WS origin checks solid (`ws_origin.go:95-121`); no central review of proxy-generated Locations otherwise | Partial |
| 12 | Access lists per domain | NPM per-host basic-auth + client directives, satisfy_any | `IPAccessControl` is global env-based (ADMIN_IP_ALLOW/DENY) with no per-domain binding; per-domain intent only exists via broken policy path (#9) | Missing |
| 13 | Audit logging | NPM audits every mutation with user_id/meta (`audit-log.js:84-103`) | Traffic rules/policies, certificate issue/renew/revoke, LB groups/targets, failover handlers write **no audit events** (no audit references found in those handlers) | Missing |
| 14 | Cert validation/install | NPM file-type whitelist + certbot lifecycle; Caddy native API; Traefik real paths | Traefik adapter stores PEM bodies in path fields (`traefik_proxy.go:380-385`); ACME revoke is local delete (`acme/service.go:368-370`); domains flow unchecked into admin URL paths (`caddy_tls.go:74-75`) | Broken |
| 15 | Health check execution & retry | Caddy active/passive thresholds+cooldown (`healthchecks.go:67,230-268`); Traefik retry middleware attempts | LB health checks TCP-dial-only, ignore `HealthCheckConfig`, flip state on first failure, overwrite operator statuses every 2s (`dataplane.go:296-326`); `proxyTCP` closes client conn on dial failure with no retry to another target (`dataplane.go:138-146`) | Weaker |

Observability cross-check: Forge's RED metrics middleware bounds label cardinality well (`middleware_metrics.go:12-50`). But request logs use raw `c.IP()` (`middleware_request_logging.go:70`) while rate limiting uses `ExtractClientIP` — two different client-IP notions in one product (Caddy logs the trusted-derived IP). Caddy adapter `Health()` is hardcoded healthy (`caddy_proxy.go:375-377`) while the Traefik adapter probes its API (`traefik_proxy.go:561-604`); `GetActiveConnections()` returns upstream/file counts, not connections (`caddy_proxy.go:149-204`, `traefik_proxy.go:296-311`). Positive: `caddy_admin.go:24-69` enforces HTTPS + token for remote admin endpoints, matching Caddy's own admin-origin hardening posture.

---

## 5. Logic findings

Severity: CRITICAL / HIGH / MEDIUM / LOW. Static-analysis findings; each cites Forge file:line and the reference contract violated.

### F1 — Traefik router-rule injection via routing-rule `Path` (HIGH)
`traefik_proxy.go:797`: `Rule: fmt.Sprintf("Host(\`%s\`) && PathPrefix(\`%s\`)", grp.domain, grp.path)`.
`validateRoutingRule` (`trafficmanager/service.go:376-382`) accepts any `url.ParseRequestURI`-parseable path starting with `/` and not `//` — including backticks, spaces, parens. A path like `` /x` || Host(`victim.example `` closes the PathPrefix string literal inside the Traefik rule expression and appends arbitrary matchers — an admin-scoped rule author can hijack traffic for other domains on the same Traefik instance. The domain half is IDNA-validated (`service.go:365-372`, blocks backticks), the path half is not escaped. Traefik itself never builds rules by interpolating user strings; chain construction only validates recursion (`middlewares.go:62-69`). Fix: reject backticks/control chars in `Path` or emit matchers structurally.

### F2 — Cross-domain traffic-policy leakage (HIGH)
No rule→policy mapping exists in the data model or API (`handlers_trafficmanager.go` has no policy-binding field), so adapters apply **all** policies to **all** routes: `collectApplicablePolicies` returns a copy of the entire map ("For now, return all policies…", `traefik_proxy.go:960-972`) and the Caddy path collects every policy's handles onto every grouped route (`caddy_proxy.go:722-747`, `enrichRouteWithPolicy` :771-793). Consequences:
- An IPWhitelist meant for one admin panel applies to every customer domain → availability break.
- An IPBlacklist on domain A silently blocks listed clients on domain B → policy collision/bypass.
Traefik attaches middlewares per-router by name; NPM binds access lists per proxy host. (Overlaps the wiring bug noted in subagent-04 §5; confirmed here from the security dimension.)

### F3 — Unhealthy-upstream add/remove oscillation loop (HIGH, reliability)
Two subsystems disagree about who owns health state:
1. `ProbeTargets` (`trafficmanager/service.go:1001-1082`) TCP-dials targets and after 3 failures calls `adapter.SetUpstreamHealth(..., false)`; the Caddy adapter removes the dial from the running config (`caddy_proxy.go:486-512`).
2. Every full sync (`ApplyRoutes`/`SyncRoutes`/`ReinstateNodeTargets`, `service.go:562-587,871-880`) rebuilds routes via `buildRoutes`/`buildGroupedRoute` (`caddy_proxy.go:877-885,924-978`) **without consulting `p.healthStatus`**, re-adding every known upstream.
Each reconcile tick (2 min, `service.go:963-992`) re-adds dead backends until the next probe removes them — a permanent flap loop that periodically routes real requests at dead nodes (defeating the probe) and churns gateway reloads. Similarly `ReconcileRoutes` moves rules into `withdrawnRules` (`service.go:900-951`) while `ProbeTargets` still iterates `s.rules`. Caddy avoids out-of-band divergence by keeping passive health inside the proxy process (`healthchecks.go:230-268`). Not strictly infinite, but self-sustaining for the lifetime of an outage.

### F4 — ACME: no domain validation, revoke is a no-op, domains injected into admin URLs (HIGH)
- `IssueCertificateRequest.Domains` flows from `/certificates/issue` (`handlers_certificates.go:17-27`) straight into lego with zero normalization (no IDNA/lowercasing; only a blanket wildcard⇒dns-01 check, `acme/service.go:210-229`) and **no ownership check** that requested domains belong to this panel. Contrast Caddy on-demand TLS "ask" gating and NPM tying certs to existing proxy hosts.
- `RevokeCertificate` deletes the store row only (`acme/service.go:368-370`); the cert remains valid at the CA until expiry while the panel reports it revoked. NPM shells to `certbot revoke`.
- Those unvalidated domains become Caddy admin API path segments: `"tls/certificates/%s"` (`caddy_proxy.go:92-93`; `caddy_tls.go:74-75,103-104,123-124,148-149`). A stored domain containing `/` redirects DELETE/GET to arbitrary admin endpoints; nothing percent-encodes the hostname.
- Minor: HTTP-01 solver keys tokens by `token+"/"+domain` with domain = `r.Host` (`acme/service.go:75-92`) — a `Host: example.com:80` header misses the lookup and fails challenges.

### F5 — DNS-provider SSRF + process-global env mutation (MEDIUM-HIGH)
`dns/service.go` exposes credential fields `PDNS_API_URL` (line 256) and `RFC2136_NAMESERVER` (line 266) that lego will point at any URL/host with zero scheme/IP-range validation (no metadata/RFC1918/link-local blocking anywhere in the file). A configured provider turns the panel into an authenticated SSRF client carrying its stored API key toward internal services. Additionally `createDNSProvider` swaps credentials into **process-global environment variables** under `envMu` (`setEnvRestore`, dns/service.go:489-498,637-661): concurrent goroutines reading those vars can observe another configuration's secrets during the window, and the global mutex serializes provider construction on the renewal path. References avoid this class: NPM passes credentials as files to certbot subprocesses; Caddy uses structured issuer JSON.

### F6 — mTLS "HTTPS required" gate trusts client-supplied `X-Forwarded-Proto` (MEDIUM)
`middleware_mtls.go:117-125`: the HTTPS requirement passes if the client sends `X-Forwarded-Proto: https` — no trusted-proxy gating (contrast Caddy `determineTrustedProxy`, forwarded headers honored only from configured ranges, `server.go:1197-1220`). Cert verification still requires a real `*tls.Conn` (:127-158), so this weakens defense-in-depth rather than fully bypassing mTLS — but the check gives false assurance behind untrusted hops. Same trust question applies to all `ExtractClientIP` consumers (#3/#7).

### F7 — LB health checks ignore their own config; no hysteresis; operator statuses reverted (HIGH, reliability)
`runHealthChecks` (`dataplane.go:296-326`):
- Never reads `group.HealthCheck` (Path/IntervalSeconds/TimeoutSeconds/HealthyThreshold/UnhealthyThreshold advertised at `service.go:62-69` and persisted); checks run every fixed 2s (`dataplane.go:36-47`).
- Single TCP connect flips status both directions — no thresholds → flap under transient loss (Caddy pairs active expectations with passive cooldowns, `healthchecks.go:67,233-266`).
- Marks targets Healthy on dial success even when an operator set them Unhealthy via `SetTargetStatus` (`service.go:502-528`) — Draining is skipped (:310-311) but Unhealthy is force-reverted every 2 seconds; manual removal from rotation is impossible.
- The advertised HTTP health-check path is never exercised (Forge defines but never populates `TraefikHealthCheck` either, `traefik_proxy.go:85-89`).
Also: `MarkNodeTargetsHealthy` marks every node target Healthy on heartbeat recovery alone — blind trust, no probe (`service.go:600-639`); `nextWeightedRoundRobin` shares counter key `"__weighted"` across all groups (`service.go:480-482`) so group weights perturb each other.

### F8 — Non-deterministic middleware ordering + duplicate upstream append on recovery (MEDIUM)
- `buildGroupedRoute` iterates a policy map to build ordered middleware lists (`traefik_proxy.go:769-787`): with ≥2 policies the whitelist/blacklist/ratelimit/circuit-breaker order changes run-to-run. In Traefik list position defines execution order (`middlewares.go:53-57`) — blacklist-before-whitelist vs reverse yields different outcomes for IPs matching both.
- Recovery append duplication: `SetUpstreamHealth(healthy=true)` appends `targetURL` whenever `modified == false` (`traefik_proxy.go:652-655`) — i.e., whenever the URL was already present — growing the server pool each flap/recover cycle; same pattern in the Caddy adapter (`caddy_proxy.go:503-508`).
- Health maps keyed `ruleID|host:port` are never cleaned on rule deletion (`traefik_proxy.go:607-621`, `caddy_proxy.go:380-394`), and route matching by `strings.Contains(rid, ruleID)` (`caddy_proxy.go:457`) hits sibling rules whose IDs share prefixes (e.g. `abc` matches `gamepanel-abc2-group`) — mutating the wrong route's upstreams.

### F9 — Failover engine details + crossnode pointer mutation (MEDIUM)
- `HandleNodeOffline` synthesizes fallback `Policy{ID:""}` when none configured (`failover/service.go:188-210`); incidents persist `policyID=""` (:220,252-266) and cooldown keys on `"node:"+nodeID` (:442-445) — functional but incident records lose the acting policy.
- If final `CreateFailoverEvent` fails the error is swallowed and success returned (`failover/service.go:523-525`): completed evacuation without durable record lets `hasActiveIncidentLocked` (:235-248) admit a duplicate plan within the window.
- On the "infinite failover loop" hypothesis: the heartbeat monitor gates recovery behind thresholds and a Reconciling phase (`heartbeatmonitor/service.go:300-315,337-352`), and cooldown is claimed *before* the executor runs (`failover/service.go:450-455`), so event retries cannot storm evacuations. The genuine loop risk is F3, not policy-level re-trigger.
- `IngressSynchronizer.Sync` mutates shared `*RoutingRule` pointers after releasing the RLock (`primary.TargetHost = ...`, `crossnode/ingress_sync.go:128-140`) — data race with `SetRules/UpsertRule` — and replica IDs compound across syncs (`primary.ID+"-replica-N"` where primary may itself be a prior replica fed back via `SetRules`, :144-162).
- `Resolver.ResolveTargetHost` caches failure results (including the `"localhost"` fallback) for cacheTTL (`crossnode/resolver.go:73-82,110`) — transient store errors misroute traffic to the wrong host for up to 30s.

### F10 — L4 load balancer as internal-network probe primitive (MEDIUM-HIGH)
`AddTarget` accepts `req.IP` verbatim — no `net.ParseIP`, no private/metadata range rejection (`handlers_loadbalancer.go:88-100`). The dataplane dials `target.IP:port` per connection (`dataplane.go:142`) and per health check (:317), relaying raw bytes both ways. Anyone permitted to manage LB targets can map which internal IP:port pairs accept connections (status observable via API) and use listeners as one-hop TCP relays into management networks (cloud metadata included). Compare Traefik strict CIDR-or-reject parsing (`ip_whitelist.go:31-40`) and Forge's own stricter `validateRoutingRule.TargetHost` checks (`trafficmanager/service.go:388-403`) which the LB path lacks entirely.

### F11 — CaddyTLSManager full-config replacement wipes sibling domains (HIGH, reliability)
`ProvisionLetsEncrypt` POSTs a complete config containing a single-domain `gamepanel-domains` server to `/config/` (`caddy_tls.go:34-56,186-238,253-280`). Caddy treats `POST /config/` as whole-config replacement: provisioning domain B erases domain A's routes, TLS automation policy, and connection policies. The fetch-modify-apply-with-rollback discipline used elsewhere (`caddy_proxy.go:514-593,673-705`) is absent here. Its `validateConfig` is local-only JSON parsing (`caddy_tls.go:240-251`) unlike the real `/load` dry-run used by the routes adapter (`caddy_proxy.go:980-1002`), so invalid TLS configs reach production. `RemoveCert` ignores response status entirely (`caddy_tls.go:109-115`) — failed removals look successful.

### F12 — Constant CSP nonce fallback (LOW-MEDIUM)
If `crypto/rand.Read` fails both middlewares emit `'nonce-fallback-nonce' 'strict-dynamic'` (`middleware_security.go:19-25`, `middleware_security_headers.go:40-46`) — a publicly-known nonce which, combined with `strict-dynamic`, lets injected scripts execute. Correct behavior is aborting the request rather than shipping a deterministic CSP. Also `X-CSP-Nonce` echoes the nonce in a response header (`middleware_security.go:43`, `middleware_security_headers.go:81`) — acceptable only because it is per-request.

### F13 — Traefik adapter cannot install certificates at all (HIGH, cert mishandling)
`SetCertificate` marshals `cert.Certificate`/`cert.PrivateKey` — PEM **contents** — into `certFile`/`keyFile`, which Traefik's file provider interprets as filesystem **paths** (`traefik_proxy.go:373-397`, types :1100-1108). Traefik attempts to open files named after PEM bodies and fails; the API reports success regardless. Net effect: custom certs never serve on the Traefik gateway. Removal compounds it: `RemoveCertificate` substring-matches stored PEM bodies for domain names (`traefik_proxy.go:410-423`). Reference contract: NPM writes real files consumed by nginx (`certificate.js` allowedSslFiles pipeline); Traefik expects paths; Caddy takes content uploads (which `caddy_proxy.go:83-121` does correctly).

### Additional low-severity notes
- `resolveTargets` drops rules whose node is unhealthy instead of failing over to any remaining healthy target — traffic 404s during partial outages even when replicas exist (`trafficmanager/service.go:620-643`; contrast crossnode's replica fan-out, `ingress_sync.go:128-163`).
- Routing rules accept `Headers` maps that are persisted but never rendered into either gateway config (`service.go:35,283`; absent from `buildGroupedRoute`/traefik service config) — silent no-op feature.
- Circuit-breaker semantics differ per adapter for the same policy field: Caddy gets `max_failures` count, Traefik gets `NetworkErrorRatio() > 1/threshold` expression (`caddy_proxy.go:858-872` vs `traefik_proxy.go:937-946`) — identical policies behave differently depending on gateway.

---

## 6. Recommendations (priority order)

1. **Bind policies to rules explicitly** (F2, #9): add `policyId[]` to routing rules; attach only referenced policies per route; sort middleware application deterministically (rate-limit → blacklist → whitelist → circuit breaker → redirect) mirroring Traefik's ordered-list contract.
2. **Escape/reject rule interpolation inputs** (F1): forbid backticks, `${`, control chars in `Path`; consider structural matcher emission instead of string templates.
3. **Single owner for upstream health** (F3, F7): make route builders consult adapter health maps (or move health into the gateway like Caddy/Traefik do); add threshold hysteresis and respect operator-set statuses in the LB dataplane; honor `HealthCheckConfig`.
4. **Fix certificate lifecycle** (F13, F11, F4): write real key/cert files for the Traefik adapter; merge-not-replace in `CaddyTLSManager`; implement CA revocation (or rename endpoint); validate ACME domains (IDNA + ownership against proxy domains) before issuance; percent-encode hostnames in admin URL paths.
5. **Harden trust boundaries** (F5, F6, #3, F10): validate DNS-provider endpoint URLs against scheme+IP allowlists; replace "peer is private" proxy trust with explicit CIDR config (Caddy model); drop raw `X-Forwarded-Proto` acceptance in mTLS gate unless peer is trusted; validate LB target IPs (reject link-local/private unless explicitly allowed).
6. **Close observability gaps** (#15, obs cross-check): unify client-IP derivation for logs and limits; implement real adapter health probes for Caddy; report actual connection counts or rename the metric; add audit events for traffic/cert/LB/failover mutations (NPM model).

