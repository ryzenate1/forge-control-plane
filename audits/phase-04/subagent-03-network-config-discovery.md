# Phase 4 — Networking Cluster · Subagent 03: Dynamic Config / Discovery / Network Security / Admin UX

**Dimension:** dynamic config (labels, file providers, API, hot reload, validation/reconciliation), service discovery, network security (IP filtering, middlewares, headers, auth, rate limits, redirects, ACLs), admin UX
**References:** `reference/networking/caddy`, `reference/networking/traefik`, `reference/networking/nginx-proxy-manager`
**Forge:** `forge/api/internal/services/{trafficmanager,servicediscovery,domains,configvalidator}`, `forge/api/internal/http/handlers_{servicediscovery,firewall,proxy_domains,domains,certificates,trafficmanager,loadbalancer}.go`, `forge/web/app/admin/{domains,traffic,load-balancer,firewall,endpoints,certificates,security}`
**Date:** 2026-08-23 · **Method:** static inspection of both trees; every path:line below was opened and read before citation. No product code modified.

---

## Legend

- STATUS: `PARITY` / `PARTIAL` / `MISSING` / `BROKEN` (present but non-functional)
- SEVERITY: `P0` traffic-affecting correctness · `P1` major functional gap · `P2` polish/robustness

---

## Part A - Capability Comparisons (18)

### 1. Config validation model: real dry-run vs "validate = apply"

- REFERENCE caddy: `caddyconfig/load.go:54-66` exposes both `/load` AND `/adapt`; `handleAdapt` (`load.go:137-175`) adapts a config and returns warnings WITHOUT applying. `handleLoad` docs state plainly: "replaces the entire current configuration" (`load.go:68-72`) then calls `caddy.Load(body, forceReload)` (`load.go:116`), which is a full `changeConfig(POST, "/config")` replace-with-provision (`caddy.go:115-142`). Failed loads leave the old config running.
- FORGE `internal/services/trafficmanager/caddy_proxy.go:980-1002`: `validateConfig` POSTs the candidate config to `/load` with `Cache-Control: must-revalidate` (:987-988). Because `/load` IS an apply operation, every "validation" already rewrites the live gateway; `applyConfig` (:1026-1052) then applies it a second time via `POST /config/`.
- STATUS: BROKEN (semantics) · SEVERITY: P0 (see Logic Finding L1)
- GAP: two mutations per update; no true dry-run anywhere in the pipeline.

### 2. Forge `configvalidator` never validates gateway configs

- FORGE `internal/services/configvalidator/validator.go:15-28`: validates only the API process's own env config (`cfg.ValidateAll()` on `internal/config.Config`) plus backup retention. Never invoked on routing rules, proxy domains, traffic policies, or rendered gateway output.
- REFERENCE caddy: module lifecycle Provision/Validate runs inside every load; REFERENCE traefik rejects invalid middleware configs at runtime build (`pkg/server/middleware/middlewares.go:476`).
- STATUS: MISSING · SEVERITY: P1

### 3. Hot reload: Traefik fsnotify watch vs Forge's nonexistent `/api/refresh`

- REFERENCE traefik `pkg/provider/file/file.go:36-37`: file provider supports `directory` + `watch`; watcher installed at `file.go:90`, fsnotify loop `file.go:170-207`. No reload call needed - the admin API is read-only GET (`pkg/api/handler.go:103-133`: rawdata/overview/http/tcp/udp routers+services+middlewares; no POST route exists at all).
- FORGE `traefik_proxy.go:1065-1084`: after writing `routes.yml`, `reloadTraefik` POSTs `/api/refresh`. That endpoint does not exist; every reload returns 404, so `UpdateRoutes` treats the write as failed and restores the previous file (:263-268), reverting even the correct file-level change.
- MITIGATING: the adapter is dead code - `cmd/api/main.go:982` constructs only `NewCaddyReverseProxy`; `NewTraefikReverseProxy` has zero non-test callers (1,139 lines).
- STATUS: BROKEN · SEVERITY: P1

### 4. Middleware composition: per-router chains vs "all policies on all routes"

- REFERENCE traefik `pkg/server/middleware/middlewares.go:64-98`: `BuildMiddlewareChain` resolves each named middleware from the router's own list and hard-fails with `middleware %q does not exist` (:73); recursion checked (:77).
- FORGE `traefik_proxy.go:960-972`: `collectApplicablePolicies` returns EVERY policy for EVERY rule ("no direct rule-to-policy mapping"). Caddy side identical: `enrichRouteWithPolicy` prepends all policy handles onto each route (`caddy_proxy.go:722-747, 771-793`).
- GAP: one tenant's IP blacklist or rate limit silently applies to all tenants' routes - cross-route contamination; NPM attaches access lists per host instead.
- STATUS: PARTIAL · SEVERITY: P0 (security)

### 5. Non-existent Caddy handlers emitted for rate limits / circuit breaking

- FORGE `caddy_proxy.go:795-875`: policies render `"handler": "rate_limit"` (:799-804) and `"handler": "circuit_breaker"` (:867-872). Neither is a standard Caddy module (see `reference/networking/caddy/modules/caddyhttp/`: encode, fileserver, headers, map, push, requestbody, rewrite, staticerror, staticresp, subroute, templates, tracing, vars, reverseproxy...). Unless a custom xcaddy build ships them, any policy-bearing `/load` fails permanently.
- Also Caddyfile-only shapes emitted as JSON: `upstreams[].weight` (:937-939) and reverse_proxy `"header_up"` key (:627-632, :966-971). Caddy ignores unknown JSON keys, so websocket upgrade headers and weights are silently dropped.
- Traefik equivalents are real modules (rateLimit/ipWhiteList/ipBlackList/circuitBreaker structs already declared at `traefik_proxy.go:131-150, 188-190`).
- STATUS: BROKEN · SEVERITY: P0

### 6. Hostname format/uniqueness validation

- REFERENCE npm `backend/schema/common.json:79-90`: `domain_names` = array, minItems 1, maxItems 100, uniqueItems, char-class pattern; duplicate-host detection across host types throws "<hostname> is already in use" (`backend/internal/proxy-host.js:32-43`, update path :127-153).
- FORGE `handlers_proxy_domains.go:36-38`: create checks only non-empty hostname. No DNS grammar, no normalization, no duplicate check - `GetProxyDomainByHostname` (`store/store_proxy_domains.go:101`) is consulted only by the app-hosting path (`handlers_apphosting.go:663`). Update can blank the hostname entirely (:140-142 guards presence, not emptiness).
- Contrast: Forge's other two domain surfaces DO validate properly (`trafficmanager/service.go:361-405` IDNA/publicsuffix/IP checks; `domains/service.go:515-534`). Three domain systems, three validators.
- STATUS: PARTIAL · SEVERITY: P1

### 7. Verify endpoints: real ownership proof vs stub

- FORGE `internal/services/domains/service.go:321-453` implements genuine token-based HTTP verification (token served at `/.well-known/forge-verify`, `handlers_domains.go:107-124`; periodic reverify started `cmd/api/main.go:1168`).
- BUT the proxy-domain surface's verify is fake: `POST /domains/:id/verify` returns `"verified": true` unconditionally without any DNS/HTTP check (`handlers_proxy_domains.go:199-212`).
- STATUS: BROKEN (validation bypass) · SEVERITY: P1

### 8. Certificate upload plumbing

- FORGE web `app/admin/certificates/page.tsx:43-50` uploads via `postJSON("/certificates", ...)`; `/certificates` registers no `POST /` route - only `/issue`, `GET /`, `GET /:id`, `DELETE /:id`, `POST /:id/renew` (`handlers_certificates.go:15-77`) -> upload gets 405. The working custom-cert group is `/custom-certificates` (`handlers_proxy_domains.go:220-227`, comment documents the collision) which no UI calls.
- Storage: `store.ProxyDomain.CertData/CertKey` are opaque strings (`store/store_proxy_domains.go:21-22`), persisted verbatim (:57-66), and returned in list/get responses - `ListProxyDomains` selects `cert_data, cert_key` into the JSON-serialized struct (:130-134). Private keys ship in every admin list response; PEM never parsed (validity/expiry/key-match unchecked).
- `trafficmanager/caddy_tls.go:18-280` (`ProvisionLetsEncrypt`/`UploadCustomCert`) would push certs into Caddy but `NewCaddyTLSManager` is constructed nowhere in `server.go`/`main.go` - dead code; uploaded certs never reach any gateway.
- Traefik reference expects cert paths (`certFile/keyFile`); Forge's Traefik adapter writes PEM CONTENTS into the path fields and appends once per domain (`traefik_proxy.go:373-397`) - see L3.
- STATUS: BROKEN · SEVERITY: P0

### 9. Firewall: ephemeral daemon pass-through vs persisted desired-state

- FORGE `handlers_firewall.go:78-97, 115-155, 174-193`: rule/port-forward bodies arrive as `map[string]any` and go verbatim to the node daemon. Nothing persists in the panel DB - no desired-state record, no reconciler, no drift detection. Node reprovision = silent rule loss.
- REFERENCE npm: DB is source of truth; nginx config regenerated from DB through configure->test->reload (`backend/internal/nginx.js:15-31, 104-115`), so state is always reproducible.
- STATUS: MISSING (persistence) · SEVERITY: P1

### 10. Firewall input validation bypass

- FORGE `handlers_firewall.go:86-92, 123-129, 144-150, 182-188`: zero validation between BodyParser and daemon RPC - ports out of range, arbitrary `action` strings, malformed sources all pass; sole guards are role+scope. UI adds client-side required checks only for forwards (`AdminFirewall.tsx:441`), none for port ranges/CIDR syntax (:351-360).
- REFERENCE npm inserts schema-typed access-list clients (`backend/internal/access-list.js:24-60`).
- STATUS: PARTIAL (validation bypass) · SEVERITY: P1

### 11. Service discovery: capable API, invisible UI, decorative enforcement

- API surface complete: endpoints CRUD/status, resolve, network visibility, node views, reachability verify/sweep, reaper stats (`handlers_servicediscovery.go:18-193`); registry with Postgres persistence + startup reload (`servicediscovery/service.go:48-56`, `registry.go:265-294`, `store.go:28-96`); stale-endpoint reaper (`stale_reaper.go:90-128`).
- UI: ZERO references - grep of `forge/web` for service-discovery/servicediscovery returns nothing. Contrast references: Traefik embeds a React dashboard (webui/) reading its read-only API (`pkg/api/handler.go:103-133`); npm lists hosts directly.
- Enforcement decorative: `PrivateNetworkPolicy.AllowPort/RevokePort/IsPortAllowed/AddPrivateCIDR/RemovePrivateCIDR` (`networkpolicy.go:35-69, 96-134`) are called ONLY from tests (`servicediscovery_test.go:334-471`). Port allow-lists and custom private CIDRs cannot be set or enforced anywhere.
- STATUS: PARTIAL (API) / MISSING (UI, enforcement) · SEVERITY: P1

### 12. Reachability checks measure the wrong vantage point

- FORGE `discovery.go:90-110`: `VerifyCrossNodeReachability` resolves the target locally and calls `VerifyCrossNode(ctx, sourceNodeID, targetNodeID, ...)`; the dial happens FROM THE API SERVER (`reachability.go:58-79` uses `net.Dialer` directly). SourceNodeID is bookkeeping; node-to-node paths (overlay/WG isolation) are never exercised.
- REFERENCE traefik: health checks run inside the serving proxy's data path, so results reflect actual forwarding.
- STATUS: PARTIAL (results misattributed) · SEVERITY: P1

### 13. Traffic page bound to a schema the backend does not have

- Web `app/admin/traffic/page.tsx:14-22` models RouteRule as `{path, targetGroup, priority, methods}` and posts exactly that (:84, :94). Backend binds `trafficmanager.RoutingRule{domain, path, targetPort, protocol, strategy, weight...}` (`service.go:24-39`); empty domain trips `validateRoutingRule` -> "invalid routing domain" (`service.go:365-372`). Creating/editing routes from this page cannot succeed; table columns (:178-192) render fields the API never returns.
- Policies tab fetches `GET /admin/traffic/policies` (:67-70) - no list route registered (`handlers_trafficmanager.go:69-105` has only `GET /policies/:id`) -> 405.
- Edit uses patchJSON (:94) but only PUT `/rules/:id` exists (`handlers_trafficmanager.go:42`) -> 405.
- Policy form posts `{type, config:{}}` (:108-121) while backend expects flat rateLimit/ipWhitelist fields (`service.go:41-54`) -> zero-value policies even if created.
- STATUS: BROKEN (entire page) · SEVERITY: P0

### 14. Load-balancer page: one broken verb away

- Web `app/admin/load-balancer/page.tsx:142-146`: "Test Next" issues POST `/admin/load-balancer/groups/:id/next`; the route is GET-only (`handlers_loadbalancer.go:127`). Other group/target verbs match (:29-135).
- Same JSX copy-paste defect as #16: `<OfflineBanner/>` nested inside the "Create Target Group" button (:158-159).
- STATUS: PARTIAL · SEVERITY: P2

### 15. Security page: hardcoded pills and a dead link

- `web/components/admin/AdminSecurity.tsx:114-133`: every domain row hardcodes HSTS Enabled / CSP Active / X-Frame DENY pills regardless of actual security_headers rows (:119-121). "View" navigates to `/admin/domains/${d.id}` (:126) - route does not exist (`app/admin/domains/` contains only page.tsx) -> 404. Stale copy "Advanced -> Domains" (:99) vs nav group "Networking & Security" (`admin-shell.tsx:53`). Global header section (:8-70) is read-only documentation, not editable config.
- STATUS: BROKEN (dead link) · SEVERITY: P1

### 16. One-page-editor parity (npm proxy-host modal vs Forge sprawl)

- REFERENCE npm: one dialog covers domains/target/SSL/cert/websockets/access-lists/advanced snippets (`proxy-host.js:21-108`) then configure->test->reload (`nginx.js:27-115`).
- FORGE: capabilities scattered across `/admin/domains` (drives server domains `/servers/:id/domains` only; requires server selection, shows nothing otherwise - `page.tsx:52-62, 144-145`), broken `/admin/security` (#15), broken `/admin/certificates` (#8), broken `/admin/traffic` (#13), plus API-only surfaces with no screen: security headers (`handlers_proxy_domains.go:312-360`), redirect rules (:362-412), forwardAuthUrl field (:28); client lib `lib/api/security.ts:37-64` has no consumer page.
- Cosmetic-but-telling: OfflineBanner accidentally nested inside header action buttons in domains (`page.tsx:127-129`), certificates (:76-79), load-balancer (:158-159); traffic renders it twice (:140,151).
- STATUS: PARTIAL · SEVERITY: P1

### 17. Audit trails

- REFERENCE npm: audit entries on every host/list mutation (`proxy-host.js:96, 175-199, 299`; access-list imports audit-log at `access-list.js:11`), browsable via audit-log objects (`schema/components/audit-log-object.json`).
- FORGE: proxy-domain, security-header, redirect, firewall, certificate handlers emit no events and no audit rows (zero publisher calls in those files - contrast `registry.go:110-117,142-147`, `trafficmanager/service.go:801-813`).
- STATUS: MISSING · SEVERITY: P1

### 18. Manual sync as the only routing-rule reconciliation

- FORGE: rule/policy CRUD persists DB+cache but never touches the gateway (`service.go:291-359, 407-427, 468-538`); propagation only via manual `POST /admin/traffic/sync` (`handlers_trafficmanager.go:107-112`), node events (`main.go:1026-1027`), or the 2-min loop - and `ReconcileRoutes` only REMOVES unhealthy-node routes (`service.go:900-951`); it never converges drifted gateway state back to DB truth.
- REFERENCES converge automatically: caddy atomic replace-on-load (`load.go:68-72`), traefik watch latency (`file.go:90`), npm apply-on-save.
- STATUS: PARTIAL (drift window by design) · SEVERITY: P1

---

## Part B - Logic Findings (6)

### L1 - "Validate" applies the config, and the rollback snapshot is taken afterwards [P0]

Sequence in `caddy_proxy.go:updateRoutesAtomic` (:673-705):
1. `validateConfig` POSTs the entire new config to `/load` - replacing the running config on success (`load.go:68-72,116`; `caddy.go:136`).
2. Only THEN is `previousConfig` snapshotted via `getRunningConfig` (:691-693) and stored as `lastValidConfig` (:695) - i.e., the snapshot contains the NEW config.
3. On `applyConfig` failure the "restore" replays the new config, not the prior one (:697-702); `Rollback()` (:279-293) likewise restores the wrong generation. Common case is silent double-apply, not clean rollback.

Compounding drift: the body applied is a root config containing ONLY the `gamepanel` server (`buildServerConfig`, :707-720). Under `/load` replace semantics this deletes the `gamepanel-domains` server that `UpdateDomainRoutes` maintains (:514-593 merges and preserves other servers). Asymmetric merge between two writers on one gateway guarantees last-writer-wins drift between the domains feature and LB/traffic routes. `lastValidConfig` is memory-only (lost on restart) and never cleared after a successful restore.

### L2 - Traefik adapter invents `/api/refresh`, then reverts its own correct write [P1]

`reloadTraefik` POSTs `/api/refresh` (`traefik_proxy.go:1066-1070`); Traefik's API is GET-only (`pkg/api/handler.go:103-133`) and the file provider self-heats via fsnotify (`file.go:90,170-207`). If wired: every update writes valid YAML, receives 404, then `restorePreviousConfig` overwrites the good file with stale content (:263-268). Currently moot because `main.go:982` instantiates Caddy exclusively - 1,139 lines of dead, doubly-broken code (with L3).

### L3 - Certificate material written where file paths belong; duplicated per domain [P1]

`SetCertificate` (`traefik_proxy.go:373-397`) sets `CertFile: cert.Certificate`, `KeyFile: cert.PrivateKey`. Traefik `certFile/keyFile` are filesystem PATHS; `CertConfig` carries PEM contents (proven by Caddy usage `caddy_proxy.go:92-104`). Result: YAML with multi-line PEM blobs in path fields, appended once per domain (`for range cert.Domains`, :380-385) producing N identical entries; `RemoveCertificate` substring-matches blob contents (:410-422).

### L4 - Upstream health flips corrupt pools and match by substring [P1]

- Caddy: recovery re-adds the dial as bare `{"dial": ...}`, losing `weight` (:486-509); route matching uses `strings.Contains(rid, ruleID)` (:457) so rule id `abc` also mutates `xabc`, `abc-v2`.
- Traefik: recovery appends weightless server (:652-654); guard `EntryPoints[0] != "tcp"` compares an HTTP router's entrypoint to a TCP value (:652) - meaningless; flap dedupe relies on in-memory `healthStatus` that resets on restart and diverges from the file.

### L5 - Adapter health is fabricated [P1]

`CaddyReverseProxy.Health` hardcodes Healthy without contacting the gateway (:375-377); `GetActiveConnections` counts configured upstreams, not connections (:149-204); Traefik variant maps filenames to zero-count pseudo-metrics (:296-311). Consumers of `Service.AdapterHealth` (`service.go:684-691`) display fiction for Caddy deployments; the honest probe (:588-601) lives only on the dead adapter.

### L6 - Fail-open resolution + cosmetic policy classification [P1]

- `Resolve` returns ALL endpoints, including unhealthy, when none are healthy (`discovery.go:60-69`); the reaper only marks stale endpoints, never removes (`stale_reaper.go:113-117`) - blackholed backends keep being handed out during partial outages.
- `classifySimple` prefix-matches strings: `"172."` flags public 172.0-15.x and 172.32-255.x as private (true range 172.16-31), misses CGNAT/link-local (`visibility.go:175-188`). Shadowed today because policy is always non-nil (`visibility.go:80-85`) but live for any future nil-policy path.
- `svcViewEndpointsAccess` collapses mixed visibility to `public` (`visibility.go:150-173`) - one public endpoint marks a whole tenant service public.

---

## Part C - Dead / orphaned admin surfaces

| Surface | File | State |
|---|---|---|
| Service-discovery REST | `handlers_servicediscovery.go` (whole) | No UI consumes it; zero web references |
| PrivateNetworkPolicy port ACLs & CIDR mgmt | `networkpolicy.go:35-134` | Test-only; no handler/enforcement |
| Traefik adapter | `traefik_proxy.go` (1,139 lines) | Never constructed (`main.go:982` builds Caddy only); broken reload (L2) |
| CaddyTLSManager | `caddy_tls.go:18-280` | Never constructed; proxy-domain certs never reach gateway |
| Cert upload modal | `certificates/page.tsx:43-50` | Posts to unregistered POST /certificates -> 405 |
| Per-domain security-header editor | `lib/api/security.ts:37-64` | Client lib exists; linked page `/admin/domains/:id` missing (`AdminSecurity.tsx:126`) |
| Proxy-domain redirects & forward-auth | `handlers_proxy_domains.go:362-412, :28` | API-only; no UI, no gateway renderer |

---

## Priority Recommendations

1. P0 - Replace validate-as-load with a true dry-run; snapshot BEFORE mutating; unify the two Caddy writers into one merge pipeline keyed off DB state (L1, #18 drift).
2. P0 - Stop emitting fictional Caddy handlers (`rate_limit`, `circuit_breaker`) and Caddyfile-only JSON (`weight`, `header_up`); implement via standard constructs or gate features explicitly (#5).
3. P0 - Scope policies to specific routes (rule-policy mapping) so IP filters/rate limits stop applying globally (#4).
4. P0 - Rewire certificates page to `/custom-certificates`, redact `certKey` from responses, parse/validate PEM server-side, instantiate a TLS manager so certs reach the gateway (#8).
5. P1 - Rebuild traffic page against real RoutingRule/TrafficPolicy schemas; add GET /policies list + PATCH if kept; fix LB Test Next verb; delete or implement `/admin/domains/:id` (#13-#15).
6. P1 - Persist firewall/port-forward desired state in DB with node reconciliation; validate payloads before daemon hop (#9, #10).
7. P1 - Wire servicediscovery into an admin page or strip it; same decision for the Traefik adapter and CaddyTLSManager (#11, #3, #8).
8. P1 - Emit audit events from proxy-domain/firewall/certificate mutations (#17).
