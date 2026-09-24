# Final Parity — Subagent 07 / 10 — Networking / Gateway (Reverse Proxy, TLS/ACME, Dynamic Config, Discovery, Mesh)

**Scope:** HTTP/TCP/UDP routing, route grouping, strategies/weights, health probes (HTTP vs TCP dial), WebSocket, TLS issuance (HTTP-01/DNS-01/TLS-ALPN, wildcard, renewal window), wildcard domain handling, middleware chain (ordered vs all-to-all), dry-run validation vs validate=apply, gateway delivery of certs, config authority (single vs five writers), static IP handling, observability headers, firewall persistence.

**References:** `reference/networking/caddy` (admin `/load` vs `/adapt`, `modules/caddyhttp`, `certmagic`), `reference/networking/traefik` (dynamic config `provider→router→middleware→service`, file watch), `reference/networking/nginx-proxy-manager` (proxy-host rows, `configure→test→reload`), `reference/app-platforms/uncloud` (WireGuard+Corrosion+gossip DNS+per-node Caddy)

**Forge re-verified:** `forge/api/internal/services/trafficmanager/{service.go,caddy_proxy.go,caddy_tls.go,caddy_admin.go,traefik_proxy.go,gateway_adapter.go}` · `forge/api/internal/services/loadbalancer/{service.go,dataplane.go}` · `forge/api/internal/services/crossnode/{ingress_sync.go,resolver.go,health_filter.go,routegroup.go}` · `forge/api/internal/services/servicediscovery/{registry.go,reachability.go,model.go}` · `forge/api/internal/services/domains/service.go` · `forge/api/internal/services/dns/service.go` · `forge/api/internal/services/acme/service.go` · `forge/api/internal/http/{handlers_trafficmanager.go,handlers_loadbalancer.go,handlers_domains.go,handlers_proxy_domains.go,handlers_certificates.go,handlers_certificates_ext.go,middleware_security.go,middleware_ratelimit.go,middleware_mtls.go,handlers_firewall.go}` · `forge/web/app/admin/{domains,traffic,load-balancer,certificates,firewall,endpoints,security}/page.tsx` · `forge/api/cmd/api/main.go`

**Prior audits read:** `audits/phase-04/synthesis.md` + `phase-04/subagent-01-reverse-proxy.md` + `subagent-02-tls-acme.md` + `subagent-03-network-config-discovery.md` + `subagent-04-forge-gateway.md` + `subagent-05-network-security.md` + `phase-06/subagent-08-uncloud-mesh.md` + `MASTER_FINDING_INDEX.md`

**Method:** file:line citations, no product code modified. Re-verified wiring confirms Dec 2025 prior-audit P0s remain open — five-writer race, dead adapters, policy leakage, and cert non-delivery are unchanged in current snapshot.

---

## 0. Densest P0 cluster — the entire audit's #1

The networking/gateway cluster contains **10 P0s (F-NET-01…10)** — more P0s than any other cluster, and three of them are mutually amplifying. No other phase has a five-writer single-resource race:

| Finding | Short title | Blast radius |
|---|---|---|
| **F-NET-01** `crossnode/ingress_sync.go:111,166` | Empty-sync wipes whole Caddy every 30 s | Periodic total outage of domain routes + TLS |
| **F-NET-02** `caddy_proxy.go:707-720` vs `523-593` | Asymmetric merge — traffic/TLS writers full-replace while domain writer merges | Any route sync erases the other feature's routes; TLS provision erases both |
| **F-NET-03** `caddy_proxy.go:980-1002,691-702` | `validate = apply` + snapshot-after-mutation | Rollback restores the bad config; double-apply per update; no true dry-run |
| **F-NET-04** `caddy_proxy.go:795-875` | Fictional `rate_limit` / `circuit_breaker` handlers | Enabling **any** policy makes **every** future `UpdateRoutes` fail validation and abort — feature-flag bricks the gateway |
| **F-NET-05** `traefik_proxy.go:960-972` / `caddy_proxy.go:722-793` | All policies on all routes (no rule↔policy FK) | Cross-tenant ACL leakage |
| **F-NET-07** `service.go:383-387` / `caddy_proxy.go:924-978` | TCP→HTTP mis-render; UDP half-implemented | L4 rules never match |
| **F-NET-08** `gateway_adapter.go:49` | `SetCertificate` zero callers — certs never leave Postgres | Successful ACME produces no TLS |
| **F-NET-09** `acme/service.go:52-92,157-159` | `HTTPSolver` never mounted — default HTTP-01 always times out | — |
| **F-NET-10** `traffic/page.tsx:14-22` | Traffic admin UX schema mismatch + 405 verbs | Admin surface non-functional |

Root cause single sentence: **Forge has five concurrent gateway writers on one embedded Caddy instance with no single config authority, no true dry-run, and two dead adapter paths that the working path must compensate around.**

This is the densest P0 cluster of the entire audit and the highest-priority fix sequence.

---

## 1. Parity matrix (18 rows)

Legend: STATUS `PARITY` / `PARTIAL` / `MISSING` / `BROKEN`  · SEVERITY `P0` · `P1` · `P2`

### Row 01 — HTTP host/path routing

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/routes.go:31` `Route{match[],handle[],terminal,group}` · `reference/networking/traefik/pkg/config/dynamic/http_config.go:38,84` `HTTPConfiguration{Routers,Services,Middlewares}` / `Router{Rule,EntryPoints,Service,Middlewares[]}` · `reference/networking/nginx-proxy-manager/backend/schema/components/proxy-host-object.json:4` denormalized `proxy_host` row |
| **FORGE** | Service `forge/api/internal/services/trafficmanager/service.go:24` `RoutingRule{Domain,Path,TargetHost,TargetPort,Protocol,Strategy,Weight,Headers,WebSocket,Enabled}`; Handler `forge/api/internal/http/handlers_trafficmanager.go:15-59` CRUD; Caddy render `forge/api/internal/services/trafficmanager/caddy_proxy.go:924-978` `buildGroupedRoute` (host+path matcher, `reverse_proxy`); Traefik render `forge/api/internal/services/trafficmanager/traefik_proxy.go:755-852` `buildGroupedRoute` (`Host(`…`) && PathPrefix(`…`)`); Domains parallel surface `forge/api/internal/services/domains/service.go:455-489` `syncCaddyRoutes` → `caddy_proxy.go:514-593` `UpdateDomainRoutes` |
| **STATUS** | `PARTIAL` |
| **GAP** | Four route concepts answer one question (`traffic_rules`, `proxy_domains.path`, `redirect_rules.source_path`, `server_domains` synthesized routes — `phase-04/subagent-04 §2-3`). No priority/ordering for overlapping host+path — Traefik has `Priority` (`http_config.go:49`), Caddy has `terminal`/`group`. Rule mutation never auto-syncs — needs manual `POST /admin/traffic/sync` (`handlers_trafficmanager.go:107`) vs NPM configure→test→reload on every CRUD (`backend/internal/nginx.js:27-117`). |
| **LOGIC FINDING** | — |
| **RECOMMENDATION** | Collapse to one router model (see §3); auto-sync on mutation via single reconciler; add explicit priority. |
| **SEVERITY** | `P1` |

### Row 02 — TCP/UDP (L4) routing

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/config/dynamic/tcp_config.go` + `udp_config.go` separate `tcp:`/`udp:` trees, `TCPRouter{Rule: HostSNI}` · `reference/networking/nginx-proxy-manager/backend/templates/stream.conf:8-27` nginx `stream { server { listen ... } }` with `reuseport` |
| **FORGE** | Admission `forge/api/internal/services/trafficmanager/service.go:383-387` validates `protocol in ("","http","https","tcp")` — **udp rejected at admission**; Traefik adapter branches correctly `traefik_proxy.go:758-760,854-905` (`buildTCPGroupedRoute`, `HostSNI`) but Caddy adapter has **no branch** `caddy_proxy.go:924-978` (always `host`+`path`+`reverse_proxy`); LB dataplane is the only real L4 path `forge/api/internal/services/loadbalancer/dataplane.go:115-270` (TCP `proxyTCP` + UDP `serveUDP` with `udpSessionTTL 2m`) gated `LOAD_BALANCER_ENABLED` (`loadbalancer/service.go:107`) |
| **STATUS** | `BROKEN` (gateway L4) / `PARTIAL` (standalone LB) |
| **GAP** | TCP rule through default Caddy becomes an HTTP host route game traffic never matches; Traefik TCP uses `HostSNI(domain)` so plain-TCP (non-TLS) also never matches; UDP silently rejected yet half-rendered under `cfg.TCP.Services` (`traefik_proxy.go:900-904`) stock Traefik ignores. |
| **LOGIC FINDING** | **F-NET-07** — protocol-declared rules render incorrectly on both gateway adapters. |
| **RECOMMENDATION** | Drop `tcp`/`udp` from traffic-rule admission until supported, or emit Caddy `layer4` app and Traefik `ClientIP`/`CatchAll` + proper `udp:` section; keep L4 LB as the canonical L4 surface. |
| **SEVERITY** | `P0` |

### Row 03 — Route grouping / upstream pools

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/hosts.go:29` `UpstreamPool` · `reference/networking/traefik/pkg/config/dynamic/http_config.go:404-425` `ServersLoadBalancer.Merge` dedupe-merge |
| **FORGE** | Three independent groupers: `caddy_proxy.go:894-922` `groupRules` (`domain|path|protocol` key), `traefik_proxy.go:725-753` duplicate, `crossnode/routegroup.go:32-68` `GroupRulesByRoute` (normalizes empty path→"/" `routegroup.go:44-49` where adapters do not) · dead-node key `caddy_proxy.go:60-74,949-951` `<firstRuleID>-group` vs per-rule withdraw |
| **STATUS** | `PARTIAL` |
| **GAP** | Divergent normalization → same rules group differently per code path; grouped-route IDs vs per-rule withdraw miss (see Row 15). |
| **LOGIC FINDING** | **F-NET-06** |
| **RECOMMENDATION** | Single `GroupRulesByRoute` owned above adapters; normalize before keying; make withdraw group-aware. |
| **SEVERITY** | `P1` (elevates to `P0` when combined with health events) |

### Row 04 — Strategies / weights (round_robin, least_conn, ip_hash, WRR)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go:41-51,82-98` `WeightedRoundRobinSelection{Weights[]int}` (weights **on policy**, not upstream) · `reference/networking/traefik/pkg/config/dynamic/http_config.go:62-69,371-382,470-478` `Service{LoadBalancer,Weighted,Failover}` + `Server.Weight` |
| **FORGE** | `service.go:33-34` `RoutingRule.Strategy/Weight`; Caddy emit `caddy_proxy.go:934-939,944-946,973-975` (`upstreams[].weight` + `lb_policy = strategy`); Traefik emit `traefik_proxy.go:806-826,813-818` (non-round_robin → sticky cookie `traefik_proxy.go:841-849`, per-server `Weight`); LB `loadbalancer/service.go:22-29,416-428,468-492` correct RR/LC/IP-hash/WRR · but WRR counter shared `roundRobin["__weighted"]` across **all** groups `service.go:480-482` |
| **STATUS** | `BROKEN` (Caddy weights) / `PARTIAL` (strategy mistranslation) |
| **GAP** | `weight` is not a Caddy `Upstream` field — silently discarded, weighted Caddy routes behave unweighted; health recovery re-appends bare `{"dial":…}` losing weight `caddy_proxy.go:503-507` (Traefik same `traefik_proxy.go:652-654`); `least_connections` has no Caddy name match (`least_conn`) and becomes session pinning on Traefik. |
| **LOGIC FINDING** | — (see `phase-04/subagent-01 §4` F-B) |
| **RECOMMENDATION** | Emit `weighted_round_robin` policy with ordered `Weights` when any weight>0; translate names per adapter (`least_connections→least_conn`); per-group WRR counters; preserve weight on recovery. |
| **SEVERITY** | `P1` |

### Row 05 — Health probes (HTTP-aware vs TCP dial)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/healthchecks.go:73,159,233-268` `ActiveHealthChecks{uri,port,headers,method,body,interval,timeout,passes,fails,expect_status}` + passive cooldowns + `hosts.go:84-93` `Upstream.Healthy()` · `reference/networking/traefik/pkg/config/dynamic/http_config.go:483-496` `ServerHealthCheck{path,method,status,interval,unhealthyInterval,timeout}` + `pkg/healthcheck/healthcheck.go:134` executor |
| **FORGE** | TrafficManager `service.go:1001-1082` `ProbeTargets` — fixed 2 s TCP dial `service.go:1026`, threshold 3, dead-code `SetUpstreamHealth`; LB `loadbalancer/dataplane.go:36,296-326` `runHealthChecks` — fixed 2 s TCP dial every tick ignoring `HealthCheckConfig` (`loadbalancer/service.go:62-69` Path/Interval/Thresholds) and skipping UDP `dataplane.go:306-308`; `crossnode/health_filter.go:76-124` `RecordSuccess/Failure` has **zero prod callers** (pass-through filter `subagent-01 §14` F-H); Traefik `TraefikHealthCheck` declared `traefik_proxy.go:85-89` never populated |
| **STATUS** | `MISSING` (HTTP probes) / `BROKEN` (configured health ignored) |
| **GAP** | No path/status/method/interval-configurable probe anywhere; configured LB health is dead storage; single-failure flip-flop; no hysteresis. |
| **LOGIC FINDING** | **F-BRK-HEALTH-CONFIG-IGNORED** |
| **RECOMMENDATION** | Implement HTTP prober honoring `HealthCheckConfig`; per-group interval; hysteresis via `crossnode.HealthFilter` threshold; make builders consult health maps; respect operator-set statuses. |
| **SEVERITY** | `P1` |

### Row 06 — Passive health / failover-on-error & oscillation

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/healthchecks.go:233-246` passive + `reverseproxy.go:1342,1647-1687` `TryDuration/RetryMatch` · `reference/networking/traefik/pkg/server/service/loadbalancer/failover/failover.go:16-73` fallback handler |
| **FORGE** | LB `dataplane.go:142-146` marks unhealthy on first dial fail, drops client with no next-target retry; `trafficmanager/service.go:900-951` `ReconcileRoutes` only removes, never retries; Caddy recovery re-add fight (`caddy_proxy.go:503-508` + full rebuild `673-720` ignoring `healthStatus`) |
| **STATUS** | `PARTIAL` |
| **GAP** | No retry-to-next-upstream; permanent add/remove flap during outage — each 2 min full rebuild re-adds dead backends until probe removes them (`phase-04/subagent-05 F3`). |
| **LOGIC FINDING** | **F-OSCILLATION** (`phase-04/synthesis F-NET-11`) |
| **RECOMMENDATION** | Bounded next-target retry; N-failure threshold; builders must check health state before re-adding. |
| **SEVERITY** | `P1` |

### Row 07 — WebSocket

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/streaming.go:241-263` transparent upgrade + `flushInterval` · `reference/networking/nginx-proxy-manager/backend/templates/proxy_host.conf:19-23,39-43` conditional `Upgrade`/`Connection` + `proxy_http_version 1.1` |
| **FORGE** | `service.go:37` `RoutingRule.WebSocket`; Caddy `caddy_proxy.go:966-971` + `domains/service.go:627-632` `header_up Connection/Upgrade`; Traefik `traefik_proxy.go:835-839` `responseForwarding.flushInterval="0ms"`; `proxy_domains.websocket` (`migrations/117_domains_certificates.sql:17`) |
| **STATUS** | `PARITY` |
| **GAP** | Minor — manual `header_up` unnecessary on modern Caddy; flush semantics harmless. |
| **RECOMMENDATION** | Rely on gateway defaults. |
| **SEVERITY** | `P2` |

### Row 08 — TLS issuance: HTTP-01 / DNS-01 / TLS-ALPN-01

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddytls/acmeissuer.go:218-229,254-319` `DNS01Solver{TTL,PropagationDelay,Resolvers}` + `makeIssuerTemplate`; `reference/networking/traefik/pkg/provider/acme/provider.go:342-372` `dns.NewDNSChallengeProviderByName` + `Propagation{disableChecks,…}` (`provider.go:104-118`) + `pkg/provider/acme/challenge_http.go:33-100` http-01 map+ServeHTTP |
| **FORGE** | `forge/api/internal/services/acme/service.go:52-92` `httpChallenger{token map[token/domain]keyAuth}` + `Present/CleanUp/ServeHTTP` + `HTTPSolver() http.Handler` (`service.go:157-159`) — **zero callers** (grep `forge/api` finds only definition); `service.go:193-295` `IssueCertificate` defaults `ChallengeTypeHTTP01` (`service.go:203-205`); DNS-01 `service.go:315-317` `dns01.AddRecursiveNameservers(1.1.1.1:53,8.8.8.8:53)` via `forge/api/internal/services/dns/service.go:311-487,593-601` providerRegistry (~36 providers) but env-mutation under `envMu` `dns/service.go:489-498,639-661` |
| **STATUS** | `BROKEN` (HTTP-01 dead) / `PARTIAL` (DNS-01) / `MISSING` (TLS-ALPN-01) |
| **GAP** | Default challenge type cannot succeed — token URL never answered; DNS-01 lacks propagation tuning (`propagation_seconds`, `resolvers`, `TXT TTL`, `override domain` — cf. NPM `certificate.js:863-868`, Traefik `provider.go:113-118`); env-mutation serializes concurrent issuance. TLS-ALPN-01 not implemented (`ChallengeTypeHTTP01/DNS01` only `service.go:40-41`). |
| **LOGIC FINDING** | **F-NET-09 / LF-1** Dead HTTP-01 solver. |
| **RECOMMENDATION** | Mount `acmeSvc.HTTPSolver()` on `:80` (or proxy `/.well-known/acme-challenge/*` from gateway config); strip port from `r.Host` (`acme/service.go:81-83`); add propagation tuning; scope DNS creds via constructors not global env. |
| **SEVERITY** | `P0` |

### Row 09 — Wildcard domain handling

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:828-880,1057-1062` `deleteUnnecessaryDomains`, `sanitizeDomains` (reject `*.*`, `dns01.UnFqdn`, wildcard-covered-SAN pruning) · `reference/networking/caddy/caddyconfig/httpcaddyfile/addresses.go` site address parsing/dedup including wildcards |
| **FORGE** | `acme/service.go:210-219` wildcard=`strings.HasPrefix(d,"*.")` + require dns-01; stored `wildcard` flag (migration `094`); domain validation `domains/service.go:504-534` (`isWildcardDomain`, IDNA+publicsuffix `service.go:515-529`), Caddy render apex+wildcard `caddy_proxy.go:595-655` `hosts=[domain, TrimPrefix("*.",domain)]`; but **no** `*.*` reject, no FQDN trailing-dot normalization, no dedupe of SANs covered by wildcard; `handlers_proxy_domains.go:36-38` lacks any DNS grammar |
| **STATUS** | `PARTIAL` |
| **GAP** | `POST /certificates/issue` accepts `*.*.example.com` and redundant SANs wasting rate-limit; duplicate-host check missing across `traffic_rules` vs `proxy_domains`/`server_domains` on shared `:80`. |
| **LOGIC FINDING** | — |
| **RECOMMENDATION** | Port `sanitizeDomains` checks; normalize via `UnFqdn`; enforce global host uniqueness (`proxy-host.js:32-47`). |
| **SEVERITY** | `P1` |

### Row 10 — TLS renewal window & storage

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:808-823` `getCertificateRenewDurations` (scales window + check interval to lifetime; 90-day→30d window, hourly check) · `reference/networking/caddy/caddyconfig/httpcaddyfile/../certmagic/storage.go:27-35` + `automation.go:65,78,129` 10m scan, 1/3-lifetime window · `reference/networking/nginx-proxy-manager/backend/internal/certificate.js:22-36,49-102` 30-day threshold + sequential |
| **FORGE** | `acme/service.go:380-438` `StartAutoRenewal` 24h ticker + `store/store_certificates.go:260` `expires_at <= now()+30d` (`auto_renew=true`) + durable queue fallback `JobCertRenewal` (`cmd/api/main.go:1178-1192`); encryption `store_certificates.go:86,212` + `store_acme_accounts.go:77-101` AES-sealed, but plaintext `certificates.dns_credentials` (`migration 134`) + `dns_provider_accounts.credentials` (`135`) while siblings encrypted (`157`); leaf key reused forever `service.go:536-540` `PrivateKey=storedDER` (anti-pattern `certmagic/automation.go:144-151`); `service.go:389-396` panic `recover` outside loop kills loop permanently; `store/store_certificates.go:277-282` decrypt failure silent-skip |
| **STATUS** | `PARTIAL` |
| **GAP** | Fixed 30d window breaks short-lived certs (<30d validity); 24h scan gives one shot for <24h-to-expiry; leaf key reuse; account decoupled from issuer; cert retrieval not retried inside renewal. |
| **LOGIC FINDING** | **F-NET-17 / LF-3** key reuse; plaintext DNS creds (`F-NET-18`) |
| **RECOMMENDATION** | Scale window off actual `NotBefore` lifetime; move `recover` inside tick; fresh key per renewal; persist issuing `accountID` + `registration_uri`; encrypt DNS creds (mirror `157`); record failed attempt row on decrypt skip. |
| **SEVERITY** | `P1` (key reuse + creds = `P0` security if zone-takeover scoped) |

### Row 11 — Middleware chain: ordered vs all-to-all

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/server/middleware/middlewares.go:50-83` `BuildMiddlewareChain` (ordered `Middlewares[]string`, recursion-checked) · `reference/networking/traefik/pkg/middlewares/{ratelimiter,ipwhitelist}/…` · `reference/networking/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go:41` |
| **FORGE** | `traefik_proxy.go:960-972` `collectApplicablePolicies` returns **all** policies (comment "For now, return all") + `routegroup.go:149-151` `extractPolicyID()=""` always; `caddy_proxy.go:722-793` concatenates all `policyHandles` onto every route; `traefik_proxy.go:769-787` iterates a Go map → **non-deterministic order**; real Caddy handlers `rate_limit`/`circuit_breaker` are fictional (`caddy_proxy.go:795-875`; Caddy has no such modules — CB lives in `reverse_proxy` namespace `reverseproxy.go:109`) |
| **STATUS** | `BROKEN` |
| **GAP** | Cross-tenant leakage: one tenant's IP blacklist/rate-limit applies to all; order flips per sync; policy-enabled routes fail validation and brick updates; `Headers` on `RoutingRule` (`service.go:35`) persisted but never rendered. |
| **LOGIC FINDING** | **F-NET-04 + F-NET-05** |
| **RECOMMENDATION** | Add `rule↔policy` FK/join (`gateway_middlewares` referenced by routers — Traefik shape); deterministic order (`rate-limit→blacklist→whitelist→CB→redirect`); reject backticks in `Path` or emit matchers structurally (`traefik_proxy.go:797` injection — `subagent-05 F1`); gate `rate_limit`/`circuit_breaker` behind real module build or map to `remote_ip`/`subroute` constructs. |
| **SEVERITY** | `P0` |

### Row 12 — Dry-run validation vs validate=apply

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/caddyconfig/load.go:68-72,137-175` `/load` **is** apply (replaces whole config, `caddy.go:115-142`), `/adapt` is the TRUE dry-run; `reference/networking/caddy/admin.go:1067-1069` `Cache-Control: must-revalidate` transaction; `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117` `configure→test→reload` with `.err` rename + `meta offline` (`nginx.js:48-90`) |
| **FORGE** | `caddy_proxy.go:980-1002` `validateConfig` POSTs candidate to `/load` (apply), then `updateRoutesAtomic` `caddy_proxy.go:673-705` snapshots **after** mutation (`previousConfig=getRunningConfig` post-validate → stores NEW config as `lastValidConfig` `:691-695`); TLS path `caddy_tls.go:240-251` only checks JSON parse; Traefik "validation" `traefik_proxy.go:974-1005` is YAML round-trip + nonexistent `POST /api/refresh` (`traefik_proxy.go:1065-1084` — API is GET-only `pkg/api/handler.go:103-133`; file provider self-heats via `provider/file/file.go:90,170-207`) |
| **STATUS** | `BROKEN` |
| **GAP** | Every "validation" mutates live gateway; rollback restores bad generation; Traefik writes always revert via stale file restore (`traefik_proxy.go:263-268`). |
| **LOGIC FINDING** | **F-NET-03 / L1** |
| **RECOMMENDATION** | Snapshot **before** mutation; use sub-resource merges (`/config/apps/http/servers/<name>`) or read-modify-write (as `UpdateDomainRoutes` already does `caddy_proxy.go:523-593`); Traefik via file-watch (drop `/api/refresh` or make advisory). |
| **SEVERITY** | `P0` |

### Row 13 — Gateway delivery of certs (data-plane)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:796-802` `addCertificateForDomain` → hot-reload via dynamic-config channel · `reference/networking/caddy/modules/caddytls/...` certmagic swap+OCSP (`automation.go:55-59`) · `reference/networking/nginx-proxy-manager/backend/internal/certificate.js:155-186` `reload` post-renew |
| **FORGE** | Abstraction `gateway_adapter.go:49` `SetCertificate/RemoveCertificate` has **zero callers** (repo-wide grep); `acme/service.go:355-363` renewal updates only Postgres; `acme/service.go:420-438` post-renew has no gateway hook; `caddy_proxy.go:83-121` + `traefik_proxy.go:373-397` impls exist; `caddy_tls.go:58-175` is the **separate embedded-Caddy** path (`ProvisionLetsEncrypt` `caddy_tls.go:186-238` full-replaces `/config/` wiping routes) — `NewCaddyTLSManager` never constructed (`cmd/api/main.go` no call); Traefik impl writes PEM **bodies** into path fields `certFile/keyFile` (`traefik_proxy.go:380-385`, `1100-1108`) — never loads |
| **STATUS** | `BROKEN` |
| **GAP** | ACME pipeline produces rows, not TLS — successful issuance never reaches any gateway unless the undocumented embedded-Caddy path is exclusively used; custom upload `POST /certificates` 405s (`handlers_certificates.go:15-77` has no `POST /`, working path is `/custom-certificates` `handlers_proxy_domains.go:227` no UI calls `certificates/page.tsx:43-50`). |
| **LOGIC FINDING** | **F-NET-08 / LF-2** |
| **RECOMMENDATION** | Wire `SetCertificate` after `Create/UpdateCertificate` + `RenewCertificate`; cert-presence drift reconciler; instantiate TLS manager or delete it; write real `*.pem` files for Traefik (`certFile/keyFile` are paths). |
| **SEVERITY** | `P0` |

### Row 14 — Config authority (single vs five writers)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46` **one** `*dynamic.Configuration`; `reference/networking/caddy/admin.go:1091-1099` single JSON doc (`/config/[path]` + `/id/<id>` targeted ops); `reference/networking/nginx-proxy-manager/backend/internal/proxy-host.js:21-108` single `proxy_hosts` table; `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` per-node Caddy watches container stream, fingerprint-cache skips churn, **never writes invalid config** (`:188-189`) |
| **FORGE** | Five writers on one Caddy: `trafficmanager.Service` (`service.go:24`, wired `main.go:1025`), `domains.Service` second server `gamepanel-domains` (`domains/service.go:455-489`, wired `main.go:998`), `crossnode.IngressSynchronizer` third writer 30 s ticker never populated (`ingress_sync.go:111-198,200-228` `main.go:994`), L4 `loadbalancer.Service` (`loadbalancer/dataplane.go:20-49`) bypassing gateways, `proxy_domains/redirect_rules/security_headers` npm-style rows with no renderer (`store_proxy_domains.go`); `caddy_admin.go:24-69` hardens remote admin but writers still share `:80` → `SO_REUSEPORT` nondeterminism (`listeners.go:115-121`; `caddy_proxy.go:567,713`) |
| **STATUS** | `BROKEN` |
| **GAP** | Last-writer-wins drift by design; empty IngressSync bare-server push destroys domain block repeatedly; three health systems with different thresholds (`service.go:1001-1082` vs `dataplane.go:296-326` vs `health_filter.go:42-56`). |
| **LOGIC FINDING** | **F-NET-01 + F-NET-02** |
| **RECOMMENDATION** | Single reconciler owning desired-state convergence (Traefik watcher / NPM renderer pattern); domains becomes a *provider* emitting routers, not a second writer; merge `IngressSynchronizer`, `ApplyRoutes/SyncRoutes` (`service.go:562-618`), `domains.syncCaddyRoutes` into one read-modify-write; skip empty syncs. |
| **SEVERITY** | `P0` |

### Row 15 — Static IP handling (target addressing & promotion)

| field | value |
|---|---|
| **REFERENCE** | `reference/app-platforms/uncloud/internal/machine/cluster/ipam.go:21-81` `IPAM` `/24` per host deterministic + `internal/machine/network/wireguard.go:12-26` `WireGuardNetwork.Configure` with `Keepalive 25s`, `internal/machine/docker/controller_linux.go:76-106` bridge `uncloud` MTU-matched + iptables allow `wg→br-*` · `reference/networking/traefik/pkg/provider/docker/config.go:288-359` `addServer/getIPAddress` per-backend IP resolution |
| **FORGE** | `trafficmanager/service.go:620-673` `resolveTargets` → `resolveTargetHost` (server→node→`PublicHostname/FQDN` vs inline `TargetHost`); cache? No — but `crossnode/resolver.go:59-83,110` caches `localhost` fallback failures for 30 s (`resolver.go:73-82`); `domains/service.go:467-468,595-655` hardcodes `localhost:8080`; `loadbalancer/service.go:62-69` advertises static IP via `Target{IP,Port}` but `dataplane.go:142` dials verbatim · `handlers_loadbalancer.go:88-100` accepts arbitrary `IP` with no `net.ParseIP`/private-range check → internal probe primitive |
| **STATUS** | `PARTIAL` |
| **GAP** | No static-IP promotion guarantee: resolved host flips between discovery→store→localhost (`resolver.go:135-166`); failure cache misroutes for 30 s; LB listeners relay raw bytes both ways (`dataplane.go:115-270`) without allowlist; no IPAM — LB port pool `30000-30100` (`loadbalancer/service.go:104-108`) + traffic `target_host` free-form share no allocation ledger (contrast Uncloud `/24` exhaustion at 256 hosts `ipam.go:44-65`). |
| **LOGIC FINDING** | **F10 / LB SSRF probe** (`phase-04/subagent-05 F10`) |
| **RECOMMENDATION** | Validate LB `IP` (`net.ParseIP`, reject link-local/private/metadata unless allowed); unify resolution behind `crossnode.Resolver` with no failure-cache of `localhost`; make `domains.TargetHost` authoritative (remove `localhost:8080` fallback); consider port/IP allocation ledger. |
| **SEVERITY** | `P1` (security `P0` if metadata-exposed) |

### Row 16 — Observability headers & security-hardening headers

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/server.go:1171-1236,663-678,981+` `trusted_proxies` CIDR-gated XFF (`TrustedProxiesStrict`, `strictUntrustedClientIp`), `StrictSNIHost` (`:663-678` 409), `logRequest` with per-server `ServerLogConfig` · `reference/networking/traefik/pkg/middlewares/{ratelimiter,ipwhitelist,snicheck,headers,redirect}/…` |
| **FORGE** | Two overlapping middlewares `middleware_security.go:27-62` vs `middleware_security_headers.go:48-93` (must be kept in sync); CSP nonce fallback to constant `"fallback-nonce"` `middleware_security.go:19-25` (`strict-dynamic` makes it exploitable); `middleware_ratelimit.go:98-119` `ExtractClientIP` trusts XFF when peer is **any** private/loopback/unspecified (vs Caddy CIDR allowlist) — two client-IP notions: logs use raw `c.IP()` `middleware_request_logging.go:70` vs limits use proxy-aware IP; `middleware_mtls.go:117-125` mTLS HTTPS gate trusts client-supplied `X-Forwarded-Proto`; `caddy_proxy.go:618-624` emits `X-Forwarded-Host/Proto` placeholders; `caddy_proxy.go:375-377` `Health()` hardcoded healthy, `149-230` `GetActiveConnections` counts upstreams/files not connections; `AdminSecurity.tsx:114-133` hardcodes HSTS/CSP/X-Frame pills regardless of `security_headers` table (write-only, no reader `store_security_headers.go:45`) |
| **STATUS** | `PARTIAL` |
| **GAP** | Trusted-proxy fail-open; observability fiction; CSP nonce abort missing; header duplication. |
| **LOGIC FINDING** | **F-NET-22 + F-NET-23** |
| **RECOMMENDATION** | Unify header policy (one middleware); CIDR-based `TRUSTED_PROXIES`; per-request nonce aborts on `rand` failure; unify client-IP derivation; real Caddy health probe (`/config/` fetch); audit `security_headers` rows actually consumed. |
| **SEVERITY** | `P1` (`P0` security for nonce + proxy trust) |

### Row 17 — Firewall persistence (desired-state)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117` DB is source of truth, `configure→test→reload` reproducible (`:104-115`) · `reference/app-platforms/uncloud/internal/machine/docker/controller_linux.go:31-72,129-168` `EnsureUncloudNetwork` declarative + iptables `DOCKER-USER` + `UNCLoud-INPUT` NAT-skip |
| **FORGE** | `forge/api/internal/http/handlers_firewall.go:78-193` bodies `map[string]any` verbatim to node daemon — no DB persistence, no reconciler, no drift detection; `forge/web/app/admin/firewall/page.tsx` client-side only; node reprovision = silent rule loss; zero validation (`handlers_firewall.go:86-92,123-129` — ports/CIDR/action unchecked) vs NPM `access_list.js:24-60` typed schema |
| **STATUS** | `MISSING` (persistence) |
| **GAP** | Ephemeral pass-through; Drift window infinite; same pattern as `security_headers` write-only. |
| **LOGIC FINDING** | **F-NET-19** |
| **RECOMMENDATION** | Persist firewall desired state in DB; node reconciler with payload validation (port ranges, CIDR grammar, action enum); port-forward narrowing. |
| **SEVERITY** | `P1` |

### Row 18 — Service discovery & mesh vs central registry (uncloud parity)

| field | value |
|---|---|
| **REFERENCE** | `reference/app-platforms/uncloud/internal/machine/dns/server.go:44-54,259-326` per-node DNS on `managementIP:53` (`internal/machine/cluster.go:222-229`), `ClusterResolver` `resolver.go:39-103` `serviceName→[]netip.Addr` with `nearest.<svc>.internal.` locality sort + forward to upstreams (`maxConcurrentForwards=1024`, `forwardingTimeout=3s`) · `internal/corrosion/client.go:51-66,148-248` CRDT + `internal/machine/cluster.go:371-483` `waitStoreSync` + `pkg/client/connector/wireguard.go:37-81` `WireGuardConnector.Connect` via `tun.DialContext` · `internal/machine/caddyconfig/controller.go:92-200` per-node Caddy watching container subscribe |
| **FORGE** | `servicediscovery/registry.go:32-40,103-108` in-memory `endpoints`/`services` + `EndpointStore` Postgres + `servicediscovery/reachability.go:33-55` TCP dial `VerifyEndpoint` + `crossnode/resolver.go:135-166` `resolveFromDiscovery`→`ResolutionStore`→`localhost`; `crossnode/health_filter.go` filter; `loadbalancer/service.go:62-69` target status; but UI zero (`handlers_servicediscovery.go:18-193` complete API, `forge/web` grep zero refs); `PrivateNetworkPolicy` port ACLs `servicediscovery/networkpolicy.go:35-134` test-only (`subagent-03 §11`); reachability dials from **API server** not source node `reachability.go:58-79` vs proxy data-path health; resolver discovery TODO membership-blind `uncloud/internal/machine/dns/resolver.go:46` mirrored — Forge correctly couples `heartbeatmonitor→loadbalancer.MarkNodeTargetsUnhealthy` but `crossnode.HealthFilter` is still producer-less |
| **STATUS** | `PARTIAL` |
| **GAP** | Capable API, invisible UI, decorative enforcement; vantage-point misattribution; no encrypted overlay (Uncloud mesh `/24` within `10.210.0.0/16` `ipam.go:13`) — Forge uses public `PublicHostname/FQDN` (`servicediscovery/registry.go:356-381` `SelectNodeAddress`); no locality modes (`nearest/rr`) `dns/server.go:294-326`. |
| **LOGIC FINDING** | **LF-02 membership-blind data plane** (uncloud parity risk) + **F-NET-21 fail-open resolve** (`discovery.go:60-69`) |
| **RECOMMENDATION** | Wire discovery into an admin page or strip it; fix reachability vantage (per-node agent probe, not API dial); add locality modes when mesh exists; gate discovery results through `heartbeatmonitor` state. |
| **SEVERITY** | `P1` |

---

## 2. Logic findings (5 — ≥4 required, each file:line pinned)

### LF-01 — Empty-rule IngressSynchronizer replaces whole Caddy config every 30 s (P0, destructive)

**Where:** `forge/api/internal/services/crossnode/ingress_sync.go:111-166` `Sync() → adapter.UpdateRoutes` unconditional; `ingress_sync.go:200-228` `SetRules/UpsertRule` have **zero non-test callers** (repo-wide grep); `forge/api/cmd/api/main.go:993-994` `ingressSync.Start(appCtx, 30*time.Second)`; `forge/api/internal/services/trafficmanager/caddy_proxy.go:673-720,1026-1052` `updateRoutesAtomic → buildServerConfig ({gamepanel:…}) → POST /config/` full replacement per `caddy/caddy.go:115-142`.

**Logic:** Synchronizer holds empty `rules` map at startup forever → `GroupRulesByRoute([])` yields 0 groups → `mergedRules = nil` → `caddyProxy.updateRoutesAtomic` builds `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80",":443"],"routes":[]}}}}}` and POSTs to `/config/` which **replaces the entire running config** (Caddy semantics `caddyconfig/load.go:68-72`). The `gamepanel-domains` server written by `domains.syncCaddyRoutes` (`caddy_proxy.go:514-593` — the only writer that merges) is destroyed every 30 s until next domain event re-syncs it. Event subscriptions (`main.go:1041-1067`) also call `ingressSync.Sync` on `Online/Offline/Recovered`, so node churn multiplies wipes.

**Reference violated:** Caddy single-doc authority + Traefik provider-convergence atomic snapshot (`reference/networking/traefik/pkg/provider/docker/config.go:build`) vs Forge five writers.

**Recommendation:** Skip `Sync` when `len(rules)==0` (no-op); populate synchronizer from `trafficmanager+domains` or delete it; make all writers use sub-resource merges (`/config/apps/http/servers/<name>`) — `UpdateDomainRoutes` pattern `caddy_proxy.go:523-569`.

---

### LF-02 — Certificates never leave Postgres (P0, data-plane gap)

**Where:** `forge/api/internal/services/trafficmanager/gateway_adapter.go:49` `SetCertificate/RemoveCertificate`; `forge/api/internal/services/trafficmanager/caddy_proxy.go:83-121` + `forge/api/internal/services/trafficmanager/traefik_proxy.go:373-397` impls; **zero callers** (grep `SetCertificate` across `forge/` finds only interface + impls).

Renewal `forge/api/internal/services/acme/service.go:322-366` `RenewCertificate` + `service.go:420-438` `runAutoRenewal` only call `store.UpdateCertificate` (`service.go:355-363`); durable fallback `main.go:1178-1192` same. Even the primary LE path via embedded Caddy (`caddy_tls.go:34-56` `ProvisionLetsEncrypt` → `buildTLSConfig` `caddy_tls.go:186-238`) is inert — `NewCaddyTLSManager` never constructed (`main.go` has no such `New`).

**Logic:** Forge's ACME rinnova and persists the star-cert `*.game.internal` plus 90d leaf, but the listeners serving HTTPS (`gamepanel-domains` + group upstreams) never receive `tls/certificates/<sni>` material. Operators observe "Verified" domain status yet TLS handshake serves self-signed or expired — the row count is not the delivery criterion. Compare Traefik `addCertificateForDomain` → dynamic-config hot-reload (`provider.go:796-802`) and NPM reload post-renew (`certificate.js:155-186`).

**Recommendation:** After every `CreateCertificate/UpdateCertificate/RenewCertificate`, call `adapter.SetCertificate` (and `RemoveCertificate` on delete); add cert `expires_at`/presence drift to reconciler; wire `CaddyTLSManager` or delete it.

---

### LF-03 — `validate = apply` + snapshot-after-mutation makes rollback fictitious (P0, safety)

**Where:** `caddy_proxy.go:980-1002` `validateConfig` POSTs to `/load` with `Cache-Control: must-revalidate`; `caddy_proxy.go:673-705` `updateRoutesAtomic` sequence `validate → snapshot → apply → restore`; `caddy_tls.go:240-251` TLS validate is JSON-only.

**Logic:** Per `caddy/caddyconfig/load.go:68-72,116` `/load` **replaces** running config even with `must-revalidate` — it is not `/adapt` (`load.go:137-175`). So step 1 already mutates. Step 2 then snapshots the *new* config as `lastValidConfig` (`:691-695`), so on subsequent `applyConfig` failure `restoreConfig(:697-702)` replays the bad generation; `Rollback()` (`:279-293`) has same generation. Normal case is silent double-apply, not validation.

**Reference violated:** Caddy `/adapt` contract and NPM `configure→test→reload` guard (`nginx.js:48-90`); Forge's abstraction `GatewayAdapter.ValidateConfig` means different rigor per kind (`traefik_proxy.go:974-1005` YAML-only).

**Recommendation:** Snapshot **before** mutation; replace gateway-level `validateConfig` with `POST /adapt` or `POST /config/` sub-resource dry-run pattern; persist `lastValidConfig` or drop memory-only rollback.

---

### LF-04 — Fictional handlers + append-bug + injection surface (P0, security + liveness)

Three sub-bugs share one MW chain path:

1. **Fictional handlers** `caddy_proxy.go:795-875` emit `{"handler":"rate_limit","rate":"N/s","burst":M}` and `{"handler":"circuit_breaker","max_failures":…}` — not in Caddy's `modules/caddyhttp/*` registry (CB is a `reverse_proxy` namespace `reverseproxy.go:109`); validation fails → **every** `UpdateRoutes` with any policy errors out, and tests assert the broken rendering (`caddy_proxy_test.go:280-314,547-615`). Traefik equivalents are valid but the same policy field becomes two different semantics: Caddy `max_failures` count vs Traefik `NetworkErrorRatio() > 1/threshold` `traefik_proxy.go:937-946`.

2. **Recovery duplication** `caddy_proxy.go:503-508` / `traefik_proxy.go:652-655` — `SetUpstreamHealth(healthy=true)` appends `targetURL` whenever `!modified` (already absent vs already present inverted), growing server pool per flap; map-iteration order `traefik_proxy.go:769-787` randomizes execution order across syncs (Traefik order matters `middlewares.go:53-57`), and `strings.Contains(rid, ruleID)` `caddy_proxy.go:457` matches sibling IDs (`abc` hits `abc-v2`).

3. **Injection** `traefik_proxy.go:797` `Rule: fmt.Sprintf("Host(\`%s\`) && PathPrefix(\`%s\`)", grp.domain, grp.path)` — domain half is IDNA-validated (`service.go:365-372`) but `Path` `service.go:376-382` accepts any `url.ParseRequestURI` starting with `/` — backticks escape `PathPrefix` and append arbitrary matchers (admin can hijack other domains — `phase-04/subagent-05 F1`).

**Recommendation:** Gate `rate_limit`/`circuit_breaker` behind documented module builds or map to real constructs (`remote_ip` subroutes — already correct for blacklists); fix append guard (`if healthy && modified==false && !alreadyPresent`); reject backticks/control chars or emit matchers structurally.

---

### LF-05 — Traefik adapter dead but doubly broken, Caddy health fabricated, observability headers divergent (P1/P0 cluster)

**Where:**

- `traefik_proxy.go:1065-1084` `reloadTraefik` `POST /api/refresh` — stock Traefik has **no such endpoint** (API is GET-only `pkg/api/handler.go:103-114`; file provider watches `provider/file/file.go:90,170-207`) → every write returns 404 then `restorePreviousConfig` (`:263-268`) reverts the correct file; `main.go:982` hardcodes `NewCaddyReverseProxy`, `NewTraefikReverseProxy` (1,139 lines `traefik_proxy.go`) never constructed in prod.

- `caddy_tls.go:186-238` `buildTLSConfig` posts a single-domain `gamepanel-domains` server to `/config/` — wipes sibling domains' routes + TLS automation; `validateConfig` `caddy_tls.go:240-251` is parse-only; `traefik_proxy.go:380-385` writes PEM bodies into `certFile/keyFile` **paths** and dupes per domain (`for range cert.Domains`).

- `caddy_proxy.go:375-377` `Health()` hardcoded `HealthHealthy` ignores gateway reachability (vs `traefik_proxy.go:561-604` probing `GET /api/http/routers`); `caddy_proxy.go:149-204` `GetActiveConnections` counts upstreams/files, not conns; `middleware_request_logging.go:70` vs `middleware_ratelimit.go:98-119` two client-IP derivations.

- `forge/web/app/admin/traffic/page.tsx:14-22,84,94` posts `{path,targetGroup,priority,methods}` vs backend `{domain,path,targetPort,…}` (`service.go:24-39`) → admin traffic page 100 % failure; `GET /policies/:id` only (`handlers_trafficmanager.go:80-86`) no `GET /policies` list (`page.tsx:67-70` 405) + `PATCH` (`:94`) vs `PUT` (`:42`) 405; cert upload `POST /certificates` 405s (`certificates/page.tsx:43-50` vs `handlers_certificates.go:15-77` no `POST /`, working `POST /custom-certificates` nowhere called); `handlers_proxy_domains.go:199-212` `verified:true` unconditional; `AdminSecurity.tsx:114-133` pills hardcoded; `lib/api/security.ts:37-64` editor links to nonexistent `/admin/domains/:id`.

**Logic:** Half the abstraction is decorative — dead adapters consume TypeScript surface while prod paths lack validation, health, and mutual exclusion. Certified-failure pattern: enabling an optional feature (policy/TLS/Traefik wiring) silently degrades the common path.

**Recommendation:** Decide Traefik fate — fix (file-watch model, real cert paths) or delete 1,139 lines; construct `CaddyTLSManager` or delete; make `Health()` honest; rebuild admin against real schemas (schemas already documented in `phase-04/subagent-03 §13-15`).

---

## 3. Cross-cutting recommendations (in open-order)

See densest-cluster box (†). After P0 stop-the-bleed:

4. **P1 wire servicediscovery or strip it** — `handlers_servicediscovery.go:18-193` fully capable, zero UI consumers; `networkpolicy.go:35-69` test-only; reachability `reachability.go:58-79` dials from API server — proxy path never exercised (`subagent-03 §12`).

5. **P1 unify resolution + health:** builders consult `healthFilter`/`healthStatus`; TTL for failure-cache (`resolver.go:73-82`) must not cache `localhost`; `stale_reaper.go:113-117` marks but never removes — blackholed backends handed out (`subagent-03 L6`).

6. **P1 encrypt DNS credentials** (`migration 157` pattern) + validate DNS-provider URLs (scheme+IP allowlist) + fresh key per renewal + persist `registrationURI`/`accountID` + `preferredChain`.

7. **P1 persist firewall desired state** with node reconciliation + payload validation (port ranges/CIDR/action).

8. **P2** unify client-IP, per-request CSP nonce abort, CIDR trusted proxies, `RATE_LIMIT_TRUSTED_IPS` bypass removal or explicit CIDR.

---

## 4. Reference vs Forge mapping (compact)

| Reference contract | Forge today | ADOPT / ADAPT / REJECT |
|---|---|---|
| Traefik `provider→router→middleware→service` declarative (one `*dynamic.Configuration`) | 5 writers, 7 tables, 4 adapters | **ADOPT** as THE model: `GatewayRouter/Middleware/Service` tables; `domains` becomes a *provider* emitting routers; collapse `traffic_rules+proxy_domains→routers`, `traffic_policies+security_headers+redirect_rules→typed middlewares` (referenced), `target_groups→services/targets` |
| One reconciler owning desired vs reported diff | 3 subscriptions + manual sync + 2 min loop + 30 s empty sync | **ADOPT** single reconciler; skip empty; sub-resource merges |
| True dry-run before apply (Caddy `/adapt`; nginx `-t`) | validate=apply + snapshot-after | **ADOPT** snapshot-before + `/adapt` semantics |
| Named middleware references per-router | all-policies-everywhere | **ADOPT** `rule↔middleware` join; deterministic order |
| CIDR-gated trusted proxies (Caddy `trusted_proxies`) | peer-private-IP trust + XFP acceptance | **ADAPT** explicit `TRUSTED_PROXIES` |
| Passive health inside proxy (Caddy/Traefik) | out-of-band probes fighting rebuilders | **ADAPT** single health owner |
| NPM one-page host editor + audit log | 5 fragmented pages + no audit on traffic/cert/LB/firewall | **ADOPT** single `Gateways` section (Routers/Services/Middlewares/Certs tabs) + audit rows |

---

## 5. Evidence index (representative file:line — inspected in this pass)

- Wiring: `forge/api/cmd/api/main.go:982` `NewCaddyReverseProxy`, `:994` `ingressSync.Start(30*time.Second)`, `:1025-1027` `EventNodeOffline/Recovered→tmSvc`, `:1039-1072` 7 subscriptions (LB + ingress×3), `main.go:1178-1192` `JobCertRenewal`, `main.go:1151-1156` domain drift reconciler comment
- Caddy adapter: `caddy_proxy.go:44-49` struct, `:673-705` `updateRoutesAtomic` (validate-before-snapshot), `:707-720` `buildServerConfig` (only `gamepanel`), `:722-793` `buildPolicyRoutes/enrichRouteWithPolicy`, `:795-875` `buildPolicyHandles` (fictional handlers), `:894-922,924-978` grouping + `buildGroupedRoute` (`@id gamepanel-<first>-group`, `upstreams[].weight`, `header_up`, `lb_policy`), `:60-74,457,503-512,949-951,350-362` withdraw/group cleanup vs Contains, `:149-204,375-377` `GetActiveConnections`/`Health`, `:980-1052` `validateConfig/getRunningConfig/applyConfig/restoreConfig`
- TLS second Caddy client: `caddy_tls.go:18-32,58-175,186-238,240-280` (never constructed)
- Traefik adapter 1,139 lines: `traefik_proxy.go:223-237,243-293,373-397,549-604,606-697,725-753,755-852,854-905,907-972,974-1005,1065-1084,1100-1108` (dead code; `/api/refresh`; PEM-as-path)
- Gateway abstraction: `gateway_adapter.go:10-12,14-19,38-64` `GatewayAdapter` (+ legacy `service.go:101-105` `ReverseProxy`, `domains/service.go:74-76` `caddyUpdater`)
- Traffic service: `trafficmanager/service.go:24-54` models, `:361-405` `validateRoutingRule`, `:291-359,562-618,753-951,1001-1082` CRUD/Apply/Sync/withdraw/reinstate/probe (+ `main.go:1025` nil-adapter hiding probes — `service.go:131-141` only `NewWithAdapter*` sets it)
- LB: `loadbalancer/service.go:22-29,62-69,416-492,480-482,552-639` algorithms + `__weighted` bug + node-mark; `dataplane.go:20-49,115-270,296-326` listeners UDP reaper + `runHealthChecks` fixed 2 s
- Crossnode: `ingress_sync.go:23-37,111-198,200-228` empty-sync; `routegroup.go:32-97,149-151` grouping + `extractPolicyID=""`; `health_filter.go:42-56,76-124,203-282` threshold/reaper with no producers; `resolver.go:59-111,135-166` 30 s `localhost` cache + discovery fallback
- Discovery: `servicediscovery/registry.go:32-40,103-108,356-381` `SelectNodeAddress`; `reachability.go:33-55,58-79` API-vantage dial
- Domains/DNS/ACME: `domains/service.go:74-76,87-89,455-489,504-534` + hardcode `localhost:8080` (`:467-468`); `dns/service.go:311-487,489-498,555-591,639-661` 36 providers + env mutation; `acme/service.go:52-92,157-159,193-295,315-317,368-370,389-438,492-546,612-638` solver dead + key reuse + revoke no-op + 24h ticker + recover outside loop
- Handlers/UI: `handlers_trafficmanager.go:8-113`, `handlers_loadbalancer.go:24-138`, `handlers_domains.go:102-124`, `handlers_proxy_domains.go:9-413` (verify stub `:199-212`, `/custom-certificates` `:227`), `handlers_certificates.go:15-77` vs `handlers_certificates_ext.go`-collision comment, `handlers_servicediscovery.go:18-193`, `handlers_firewall.go:78-193`; web `traffic/page.tsx:14-22,67-70,84,94,108-121`, `load-balancer/page.tsx:142-146,158-159`, `certificates/page.tsx:43-50,76-79`, `domains/page.tsx:127-129`, `security` (`AdminSecurity.tsx:114-133,126`)
- References: `reference/networking/caddy/caddyconfig/load.go:54-175`, `admin.go:1067-1069,1091-1099`, `modules/caddyhttp/routes.go:31-41`, `modules/caddyhttp/server.go:1171-1236,663-678,981+`, `modules/caddyhttp/reverseproxy/healthchecks.go:67,159,233-268`, `modules/caddyhttp/reverseproxy/hosts.go:29,77-98`, `modules/caddyhttp/reverseproxy/selectionpolicies.go:41-98`; `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-99,371-382,483-496`, `pkg/api/handler.go:103-133`, `pkg/provider/file/file.go:90,170-207`, `pkg/server/middleware/middlewares.go:50-83`; `reference/networking/nginx-proxy-manager/backend/schema/components/proxy-host-object.json:4-25`, `backend/internal/{proxy-host.js:21-108,nginx.js:27-117,certificate.js:22-36,access-list.js:24-60,audit-log.js:84-103}`; `reference/app-platforms/uncloud/internal/machine/{cluster.go:39-74,cluster/cluster.go:209-238,cluster/ipam.go:21-81,network/wireguard.go:12-26,dns/server.go:44-54,259-326,dns/resolver.go:39-103,caddyconfig/controller.go:92,148-200,docker/controller_linux.go:31-168}, pkg/client/{service.go:21-86,connector/wireguard.go:37-81}`

---

## 6. Handoff

Phase 04 established that Forge has **seven tables encoding three Traefik concepts** and **five writers on one Caddy**; Phase 06 showed Uncloud's per-node Caddy avoids the problem by making each node the single writer for its own listeners. The networking cluster is therefore the load-bearing finding of the audit — fixing the single-writer + true-dry-run + cert-delivery path unblocks TLS, policy, and L4 features simultaneously. Adopt the Traefik shape as the canonical Gateway model and treat `ingress_sync.go` + `caddy_tls.go` construction as the first code change, not a backlog item.

*Generated for final parity — file:line citations point to inspected snapshot; no product code modified.*
