# Subagent 14 — Re-verification: Networking — Reverse Proxy / Routing / Gateway

**Focus:** HTTP/TCP/UDP, grouping, LB, weights, grouping triplication, five-writers, fictive handlers  
**Reconciliation scope:** `audits/phase-04/synthesis.md` (densest P0 cluster: 10 P0s `F-NET-01..10`) + `audits/final-parity/subagent-07-networking-gateway.md` (18 rows, P0×10) + `audits/phase-06/subagent-08-uncloud-mesh.md` (mesh/discovery parity)  
**Re-inspection snapshot:** 2026-08-23 — file:line pinned, no product code modified  
**Method:** Direct read of `reference/` contracts vs `forge/` implementation; grep for zero-callers; `main.go` wiring check; file:line citations only.

---

## 0. Densest P0 cluster — still the audit's #1

> **Confirmed: the networking/gateway cluster remains the densest P0 cluster in the audit — 10 P0s — and all 10 are STILL BROKEN on re-inspection. No fix commit has landed between final-parity and this re-verification.**

| Finding | Short title | Still broken? | Blast radius if hit |
|---|---|---|---|
| **F-NET-01** `forge/api/internal/services/crossnode/ingress_sync.go:111` | Empty-sync wipes gateway every 30s | **YES** | Periodic total loss of `gamepanel-domains` routes + TLS |
| **F-NET-02** `forge/api/internal/services/trafficmanager/caddy_proxy.go:673-720` vs `caddy_proxy.go:523-593` | Asymmetric merge discipline | **YES** | Traffic/TLS sync erases domain routes (last-writer-wins) |
| **F-NET-03** `forge/api/internal/services/trafficmanager/caddy_proxy.go:980-1002` + `caddy_proxy.go:691-695` | `validate = apply` + snapshot-after | **YES** | Rollback restores bad gen; double-apply |
| **F-NET-04** `forge/api/internal/services/trafficmanager/caddy_proxy.go:795-875` | Fictional `rate_limit`/`circuit_breaker` | **YES** | Enabling any policy bricks every future `UpdateRoutes` |
| **F-NET-05** `forge/api/internal/services/trafficmanager/traefik_proxy.go:960-972` + `caddy_proxy.go:722-793` + `crossnode/routegroup.go:149-151` | All policies on all routes | **YES** | Cross-tenant ACL/rate-limit leakage |
| **F-NET-06** `forge/api/internal/services/trafficmanager/caddy_proxy.go:60-74` vs `caddy_proxy.go:949-951` | Grouped-route miss on withdraw | **YES** | Dead node keeps receiving traffic |
| **F-NET-07** `forge/api/internal/services/trafficmanager/service.go:383-387` + `caddy_proxy.go:924-978` + `traefik_proxy.go:758-760,854-905` | TCP/UDP mis-render | **YES** | L4 rules never match |
| **F-NET-08** `forge/api/internal/services/trafficmanager/gateway_adapter.go:38` zero callers | Certs never reach data plane | **YES** | ACME succeeds → no TLS served |
| **F-NET-09** `forge/api/internal/services/acme/service.go:52-92` + `:157-159` | HTTP-01 dead code | **YES** | Default challenge type always times out |
| **F-NET-10** `forge/api/internal/http/handlers_trafficmanager.go:107` + `forge/web/app/admin/traffic/page.tsx:14-22,84,94` | Admin UX schema/verb mismatch | **YES** | Traffic page 100% failure (405/400) |

Root cause single sentence (unchanged): **Forge has five concurrent gateway writers on one embedded Caddy instance with no single config authority, no true dry-run, with two dead adapter paths that the working path must compensate around — all confirmed present at `forge/api/cmd/api/main.go:982,994,1025`.**

This box reconciles phase-04 synthesis §3-5 + final-parity §0: the same five writers (TrafficManager, Domains `gamepanel-domains`, IngressSynchronizer 30s, L4 dataplane, dead-proxy-domain rows) plus four adapter abstractions (`GatewayAdapter` `gateway_adapter.go:38`, legacy `ReverseProxy` `service.go:101-105`, `domains.caddyUpdater`, servicediscovery `NetworkAdapter`) are wired identically to final-parity.

---

## 1. Parity matrix — 18 rows (≥15 required) — all re-verified STILL BROKEN/PARTIAL

Legend: STATUS `PARITY`/`PARTIAL`/`MISSING`/`BROKEN` · SEVERITY `P0`/`P1`/`P2`. Each row pins reference contract vs Forge today and verdict on still-broken.

### Row 01 — HTTP host/path routing (core)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/routes.go:31` `Route{match[],handle[],terminal,group}` · `reference/networking/traefik/pkg/config/dynamic/http_config.go:38` `HTTPConfiguration{Routers→Services→Middlewares}` / `Router{Rule,EntryPoints,Service,Middlewares[]}` (`:84-99`) · `reference/networking/nginx-proxy-manager/backend/schema/components/proxy-host-object.json:4` denormalized proxy-host row |
| **FORGE** | `forge/api/internal/services/trafficmanager/service.go:24` `RoutingRule{Domain,Path,TargetHost,TargetPort,Protocol,Strategy,Weight,Headers,WebSocket,Enabled}`; Handler `forge/api/internal/http/handlers_trafficmanager.go:15-59` CRUD; Caddy render `forge/api/internal/services/trafficmanager/caddy_proxy.go:924-978` `buildGroupedRoute` (host+path matcher, `reverse_proxy`); Traefik render `forge/api/internal/services/trafficmanager/traefik_proxy.go:755-852` `buildGroupedRoute` (`Host(`…`) && PathPrefix(`…`)`); Domains parallel surface `forge/api/internal/services/domains/service.go:455-489` `syncCaddyRoutes` → `caddy_proxy.go:514-593` `UpdateDomainRoutes` |
| **STATUS** | `PARTIAL` — still broken wiring |
| **GAP** | Four route concepts answer one question (`traffic_rules`, `proxy_domains.path`, `redirect_rules.source_path`, `server_domains` synthesized routes). No priority/ordering for overlapping host+path — Traefik has `Priority` (`http_config.go:94`), Caddy has `terminal`/`group` (`routes.go:31-41`). Rule mutation never auto-syncs — needs manual `POST /admin/traffic/sync` `handlers_trafficmanager.go:107-112` vs NPM `configure→test→reload` on every CRUD `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117`. Verified still manual. |
| **SEVERITY** | `P1` |

### Row 02 — TCP/UDP (L4) routing — **VERIFY STILL BROKEN: TCP mis-render**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/config/dynamic/tcp_config.go` + `udp_config.go` separate `tcp:`/`udp:` trees, `TCPRouter{Rule: HostSNI}` · `reference/networking/nginx-proxy-manager/backend/templates/stream.conf:8-27` nginx `stream { server { listen … } }` |
| **FORGE** | Admission `forge/api/internal/services/trafficmanager/service.go:383-387` `validateRoutingRule` accepts `protocol in ("","http","https","tcp")` — **udp rejected at admission**; Traefik adapter branches correctly `traefik_proxy.go:758-760,854-905` (`buildTCPGroupedRoute`, `HostSNI`) but **Caddy adapter has no branch** `caddy_proxy.go:924-978` (always `host`+`path`+`reverse_proxy`); LB dataplane is only real L4 `forge/api/internal/services/loadbalancer/dataplane.go:20-49,115-270` (TCP `proxyTCP` + UDP `serveUDP` with `udpSessionTTL 2m` `:16`) gated `LOAD_BALANCER_ENABLED` `loadbalancer/service.go:107` |
| **STATUS** | `BROKEN` (gateway L4) / `PARTIAL` (standalone LB) — **STILL BROKEN** |
| **GAP** | TCP rule through default Caddy becomes HTTP host route game traffic never matches; Traefik TCP uses `HostSNI(domain)` so plain-TCP (non-TLS) never matches; UDP silently rejected at admission yet half-rendered under `cfg.TCP.Services` `traefik_proxy.go:900-904` that stock Traefik ignores. Re-inspected: no protocol branch added to `caddy_proxy.go:924-978`. |
| **LOGIC FINDING** | **F-NET-07 — still open** |
| **SEVERITY** | `P0` |

### Row 03 — Route grouping / upstream pools — **grouping triplication**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/hosts.go:29` `UpstreamPool` · `reference/networking/traefik/pkg/config/dynamic/http_config.go:404-425` `ServersLoadBalancer.Merge` dedupe-merge |
| **FORGE** | **Three independent groupers** (triplication): `caddy_proxy.go:894-922` `groupRules` (`domain|path|protocol` key), `traefik_proxy.go:725-753` duplicate `groupRules`, `crossnode/routegroup.go:32-68` `GroupRulesByRoute` (normalizes empty path→`"/"` `:44-49` where adapters do not). Dead-node key `caddy_proxy.go:60-74,949-951` `<firstRuleID>-group` vs per-rule withdraw. Re-inspected 2026-08-23: all three still present, divergent normalization unchanged. |
| **STATUS** | `PARTIAL` — triplication still present |
| **GAP** | Same rules group differently per code path; grouped-route IDs vs per-rule `RemoveRoutes` miss (see Row 15 / FIND-03). |
| **SEVERITY** | `P1` (elevates to `P0` when combined with health events) |

### Row 04 — Strategies / weights (round_robin, least_conn, ip_hash, WRR)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go:41-98` `WeightedRoundRobinSelection{Weights[]int}` (weights **on policy**, not upstream) · `reference/networking/traefik/pkg/config/dynamic/http_config.go:62-69,371-382` `Service{LoadBalancer,Weighted,Failover}` + `Server.Weight` |
| **FORGE** | `service.go:33-34` `Strategy/Weight`; Caddy emit `caddy_proxy.go:934-939,944-946,973-975` (`upstreams[].weight` + `lb_policy = strategy`); Traefik emit `traefik_proxy.go:806-826` (non-round_robin → sticky cookie `traefik_proxy.go:841-849`, per-server `Weight` `traefik_proxy.go:813-818`); LB `loadbalancer/service.go:22-29,416-492` correct RR/LC/IP-hash/WRR but WRR counter shared `roundRobin["__weighted"]` across **all** groups `loadbalancer/service.go:480-482` |
| **STATUS** | `BROKEN` (Caddy weights silently lost) / `PARTIAL` (strategy mistranslation) |
| **GAP** | `weight` is not a Caddy `Upstream` field — silently discarded; health recovery re-appends bare `{"dial":…}` losing weight `caddy_proxy.go:503-507` (Traefik same `traefik_proxy.go:652-654`); `least_connections` has no Caddy name (`least_conn`) and becomes sticky-pin on Traefik. |
| **SEVERITY** | `P1` |

### Row 05 — Health probes (HTTP-aware vs TCP dial)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/healthchecks.go:73,159,233-268` `ActiveHealthChecks{uri,port,headers,method,interval,timeout,passes,fails}` + passive cooldowns · `reference/networking/traefik/pkg/config/dynamic/http_config.go:483-496` `ServerHealthCheck` + executor |
| **FORGE** | TrafficManager `service.go:1001-1082` `ProbeTargets` — fixed 2s TCP dial `service.go:1026`, threshold 3, `SetUpstreamHealth` path dead-call; LB `loadbalancer/dataplane.go:296-326` `runHealthChecks` — fixed 2s TCP dial ignoring `HealthCheckConfig` (`loadbalancer/service.go:62-69` Path/Interval/Thresholds) and skipping UDP `:306-308`; `crossnode/health_filter.go:76-124` `RecordSuccess/Failure` **zero prod callers** (pass-through filter); Traefik `TraefikHealthCheck` `traefik_proxy.go:85-89` never populated |
| **STATUS** | `MISSING` (HTTP probes) / `BROKEN` (configured health ignored) — still |
| **SEVERITY** | `P1` |

### Row 06 — Passive health / failover & oscillation

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/healthchecks.go:233-246` passive + `reverseproxy.go:1342,1647-1687` `TryDuration/RetryMatch` · `reference/networking/traefik/pkg/server/service/loadbalancer/failover/failover.go:16-73` |
| **FORGE** | LB `dataplane.go:142-146` marks unhealthy on first dial fail, drops client with no next-target retry; `trafficmanager/service.go:900-951` `ReconcileRoutes` only removes never retries; Caddy recovery re-add fight `caddy_proxy.go:503-508` + full rebuild `caddy_proxy.go:673-720` ignoring `healthStatus` map; re-verified: no `RetryMatch` added |
| **STATUS** | `PARTIAL` — oscillation still possible |
| **SEVERITY** | `P1` (P0 under sustained outage) |

### Row 07 — WebSocket

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/reverseproxy/streaming.go:241-263` transparent upgrade; `reference/networking/nginx-proxy-manager/backend/templates/proxy_host.conf:19-23` conditional `Upgrade`/`Connection` |
| **FORGE** | `service.go:37` `WebSocket`; Caddy `caddy_proxy.go:966-971` + `domains/service.go:627-632` `header_up Connection/Upgrade`; Traefik `traefik_proxy.go:835-839` `flushInterval="0ms"`; still present |
| **STATUS** | `PARITY` |
| **SEVERITY** | `P2` |

### Row 08 — TLS issuance: HTTP-01 / DNS-01 / TLS-ALPN-01

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddytls/acmeissuer.go:218-319` DNS-01 solver; `reference/networking/traefik/pkg/provider/acme/provider.go:342-372` DNS provider + `challenge_http.go:33-100` http-01 map+ServeHTTP |
| **FORGE** | `forge/api/internal/services/acme/service.go:52-92` `httpChallenger{token map[token/domain]keyAuth}+ServeHTTP` + `HTTPSolver() http.Handler` `service.go:157-159` — **zero callers** (grep `forge/` finds only definition); `service.go:193-295` `IssueCertificate` defaults `ChallengeTypeHTTP01` `service.go:203-205`; DNS-01 via `forge/api/internal/services/dns/service.go:311-487,639-661` ~36 providers but env-mutation under `envMu` `dns/service.go:489-498` |
| **STATUS** | `BROKEN` (HTTP-01 dead) / `PARTIAL` (DNS-01) — **STILL BROKEN** |
| **SEVERITY** | `P0` |

### Row 09 — Wildcard domain handling

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:828-880,1057-1062` `sanitizeDomains` (reject `*.*`, wildcard-covered-SAN pruning) |
| **FORGE** | `acme/service.go:210-219` wildcard=`strings.HasPrefix(d,"*.")`+require dns-01; stored `wildcard` flag; `domains/service.go:504-534` IDNA+publicsuffix, Caddy apex+wildcard `caddy_proxy.go:595-655`; but **no** `*.*` reject, no trailing-dot normalization, no SAN dedupe; still as before |
| **STATUS** | `PARTIAL` |
| **SEVERITY** | `P1` |

### Row 10 — TLS renewal window & storage

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:808-823` lifetime-scaled window; `reference/networking/caddy/.../certmagic/storage.go` 1/3-lifetime window |
| **FORGE** | `acme/service.go:380-438` `StartAutoRenewal` 24h ticker + `store/store_certificates.go:260` `expires_at <= now()+30d` + queue fallback `cmd/api/main.go:1178-1192`; encryption partial: `store_certificates.go:86,212` AES-sealed but plaintext `certificates.dns_credentials` + `dns_provider_accounts.credentials` (migration 134/135) while siblings encrypted (157); leaf key reuse `service.go:536-540` |
| **STATUS** | `PARTIAL` |
| **SEVERITY** | `P1` (P0 security for key reuse + creds) |

### Row 11 — Middleware chain: ordered vs all-to-all — **VERIFY STILL BROKEN: fictive handlers + all-policies-all-routes**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/server/middleware/middlewares.go:50-83` ordered `Middlewares[]string` recursion-checked; `reference/networking/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go` |
| **FORGE** | `traefik_proxy.go:960-972` `collectApplicablePolicies` returns **all** policies (comment `For now, return all`) + `crossnode/routegroup.go:149-151` `extractPolicyID()=""` always; `caddy_proxy.go:722-793` concatenates all `policyHandles` onto every route; `traefik_proxy.go:769-787` iterates Go map → **non-deterministic order**; handlers `caddy_proxy.go:795-875` emit `{"handler":"rate_limit","rate":"N/s"}` and `{"handler":"circuit_breaker"}` — **not in Caddy's registry** (CB is `reverse_proxy` namespace `reverseproxy.go:109`) |
| **STATUS** | `BROKEN` — **STILL BROKEN: fictive handlers brick updates; all-policies-all-routes leaks** |
| **GAP** | Cross-tenant: one tenant's IP blacklist/rate-limit applies to all; order flips per sync; any policy-enabled route fails `validateConfig` (via `/load`) and aborts; `RoutingRule.Headers` `service.go:35` persisted but never rendered. Tests still assert broken rendering `caddy_proxy_test.go:280-314,547-615`. |
| **LOGIC FINDING** | **F-NET-04 + F-NET-05 — still open** |
| **SEVERITY** | `P0` |

### Row 12 — Dry-run validation vs `validate=apply` — **VERIFY STILL BROKEN**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/caddyconfig/load.go:68-72,116,137-175` — `/load` **is apply** (`caddy.Load(body,forceReload)`), `/adapt` is TRUE dry-run; `reference/networking/caddy/admin.go:1067-1069` `Cache-Control: must-revalidate` transaction; `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117` `configure→test→reload` with `.err` rename |
| **FORGE** | `caddy_proxy.go:980-1002` `validateConfig` POSTs candidate to **`/load`** (apply) `caddy_proxy.go:981-983`; `updateRoutesAtomic` `caddy_proxy.go:673-705` sequence `validate → snapshot → apply → restore` where **snapshot is AFTER mutation** `caddy_proxy.go:691-695` `previousConfig=getRunningConfig` post-validate → stores NEW config as `lastValidConfig`; TLS path `caddy_tls.go:240-251` only checks JSON parse; Traefik `traefik_proxy.go:974-1005` YAML round-trip + nonexistent `POST /api/refresh` `traefik_proxy.go:1065-1084` (Traefik API is GET-only `pkg/api/handler.go:103-133`; file provider self-heats via `provider/file/file.go:90,170-207` `fsnotify`) |
| **STATUS** | `BROKEN` — **STILL BROKEN** |
| **GAP** | Every "validation" mutates live gateway; rollback restores bad generation; Traefik writes revert via stale file. |
| **LOGIC FINDING** | **F-NET-03 — still open** |
| **SEVERITY** | `P0` |

### Row 13 — Gateway delivery of certs (data-plane) — **gateway_adapter zero callers**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/provider/acme/provider.go:796-802` `addCertificateForDomain → hot-reload`; `reference/networking/nginx-proxy-manager/backend/internal/certificate.js:155-186` reload post-renew |
| **FORGE** | Abstraction `gateway_adapter.go:38-64` `GatewayAdapter{SetCertificate,RemoveCertificate,ValidateConfig,Reload,Rollback,CleanupStale,Health,SetUpstreamHealth}`; impls `caddy_proxy.go:83-121` + `traefik_proxy.go:373-397` exist; **grep `SetCertificate` across `forge/` finds only interface+impls — zero callers** (verified 2026-08-23 `grep -rn SetCertificate forge/`). Renewal `acme/service.go:355-363,420-438` + queue fallback `main.go:1178-1192` only call `store.UpdateCertificate`. `CaddyTLSManager` `caddy_tls.go:18-32` `NewCaddyTLSManager` never constructed (`main.go:982` hardcodes `NewCaddyReverseProxy`); Traefik writes PEM **bodies** into path fields `certFile/keyFile` `traefik_proxy.go:380-385` |
| **STATUS** | `BROKEN` — **STILL BROKEN: certificates never leave Postgres** |
| **SEVERITY** | `P0` |

### Row 14 — Config authority (single vs five writers) — **five-writers**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46` **one** `*dynamic.Configuration`; `reference/networking/caddy/admin.go:1091-1099` single JSON doc (`/config/[path]` + `/id/<id>`); `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` per-node Caddy fingerprint-cache, never writes invalid (`:188-189`) |
| **FORGE** | **Five writers on one Caddy** wired at same address: 1) `trafficmanager.Service` `main.go:1025` `NewWithPersistence(..., caddyProxy)`, 2) `domains.Service` second server `gamepanel-domains` `domains/service.go:455-489` + `main.go:998` same `caddyProxy`, 3) `crossnode.IngressSynchronizer` third writer 30s ticker `main.go:994-995` same `caddyProxy`, 4) L4 `loadbalancer.Service` direct listeners `loadbalancer/dataplane.go:62-92` bypassing gateway, 5) `proxy_domains/redirect_rules/security_headers` rows no renderer. Shared `:80` (`caddy_proxy.go:567,713` both servers `listen:80`) → `SO_REUSEPORT` nondeterminism `reference/networking/caddy/listeners.go:115-121`. Re-verified: wiring unchanged. |
| **STATUS** | `BROKEN` — **STILL BROKEN: last-writer-wins by design** |
| **SEVERITY** | `P0` |

### Row 15 — Gateway route lifecycle (RemoveRoutes / CleanupStale) — **grouped-route miss**

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/admin.go:1091-1099` targeted `/id/<id>` ops; `reference/networking/traefik/pkg/provider/file/file.go:90,170-207` atomic file replace + `fsnotify` |
| **FORGE** | `caddy_proxy.go:51-74` `RemoveRoutes` DELETEs `/id/gamepanel-<id>` per ruleID; but `buildGroupedRoute` `caddy_proxy.go:949-951` creates **one** Caddy route per group `@id=gamepanel-<firstRuleID>-group`; withdrawal `service.go:753-798` `WithdrawNodeTargets` iterates `ruleIDs` per server — grouped route with non-first rule stale survives. `CleanupStale` `caddy_proxy.go:295-373` handles `-group` suffix (`:355-357`) but `RemoveRoutes` does not; Traefik side `traefik_proxy.go:273-293,471-559` strips per-rule routers only, grouped `gamepanel-<first>` miss. Still present. |
| **STATUS** | `BROKEN` — **STILL BROKEN: dead backends keep receiving traffic** |
| **SEVERITY** | `P0` when combined with `F-NET-01`/`HealthFilter` |

### Row 16 — Static IP handling / target addressing

| field | value |
|---|---|
| **REFERENCE** | `reference/app-platforms/uncloud/internal/machine/cluster/ipam.go:21-81` deterministic `/24` per host; `reference/networking/traefik/pkg/provider/docker/config.go:288-359` per-backend IP resolution |
| **FORGE** | `trafficmanager/service.go:620-673` `resolveTargets→resolveTargetHost` (server→node→`PublicHostname/FQDN` vs inline `TargetHost`); `crossnode/resolver.go:59-83` caches `localhost` fallback failures for 30s (`:73-82`); `loadbalancer/service.go:62-69` advertises static IP `Target{IP,Port}` but `dataplane.go:142` dials verbatim; `handlers_loadbalancer.go:88-100` accepts arbitrary `IP` without `net.ParseIP`/private-range reject → internal probe primitive |
| **STATUS** | `PARTIAL` |
| **SEVERITY** | `P1` (P0 if metadata-exposed) |

### Row 17 — Observability headers / security hardening

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/caddy/modules/caddyhttp/server.go:1171-1236` CIDR-gated `trusted_proxies`; `server.go:663-678` `StrictSNIHost` |
| **FORGE** | Two middlewares `middleware_security.go:27-62` vs `middleware_security_headers.go:48-93` diverging; CSP nonce fallback `"fallback-nonce"` `middleware_security.go:19-25`; `middleware_ratelimit.go:98-119` trusts XFF when peer is any private; `caddy_proxy.go:375-377` `Health()` hardcoded healthy; `caddy_proxy.go:149-204` `GetActiveConnections` counts upstreams/files; `AdminSecurity.tsx:114-133` hardcodes pills |
| **STATUS** | `PARTIAL` |
| **SEVERITY** | `P1` |

### Row 18 — Firewall persistence / service discovery (uncloud mesh parity)

| field | value |
|---|---|
| **REFERENCE** | `reference/networking/nginx-proxy-manager/backend/internal/proxy-host.js:21-108` + `audit-log.js:84-103`; `reference/app-platforms/uncloud/internal/machine/dns/server.go:44-54,259-326` per-node DNS `*.internal.` + locality; `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92-200` per-node Caddy watches container stream |
| **FORGE** | `handlers_firewall.go:78-193` verbatim `map[string]any` to daemon — no DB, no reconciler, no validation (vs NPM typed schema `access_list.js:24-60`); node reprovision loses rules; `servicediscovery/registry.go:32-40` in-memory + `EndpointStore`; `PrivateNetworkPolicy` test-only; `reachability.go:58-79` dials from API not source node; UI zero for discovery endpoints; re-verified still unwired |
| **STATUS** | `MISSING` (firewall persistence) / `PARTIAL` (discovery) |
| **SEVERITY** | `P1` |

**Reconciliation note:** Phase-04 synthesis reported 18 comparisons; final-parity subagent-07 reported 18 rows (P0×10). This matrix reproduces 18 rows, confirms the same 10 P0s remain open, and adds cross-check against phase-06 subagent-08 uncloud mesh §C10/C06/C14 (per-node single-writer Caddy vs Forge five-writer central).

---

## 2. Logic findings (5 — ≥4 required, each file:line pinned, STILL BROKEN)

### FIND-01 — Empty-rule IngressSynchronizer replaces whole Caddy config every 30s (P0, destructive) — **STILL BROKEN**

**Where:** `forge/api/internal/services/crossnode/ingress_sync.go:111-198` `Sync() → adapter.UpdateRoutes` unconditional; `ingress_sync.go:200-228` `SetRules/UpsertRule/RemoveRule` have **zero non-test callers** (repo-wide grep `grep -rn SetRules forge/api` returns only tests + definition); `forge/api/cmd/api/main.go:993-995` `ingressSync = crossnode.NewIngressSynchronizer(caddyProxy,…)` + `ingressSync.Start(appCtx, 30*time.Second)`; `forge/api/internal/services/trafficmanager/caddy_proxy.go:673-720,1026-1052` `updateRoutesAtomic → buildServerConfig({gamepanel:…}) → POST /config/` full replacement (`caddy/caddy.go:115-142`, `caddyconfig/load.go:68-72`).

**Re-verification:** Read `ingress_sync.go:111-166` — no guard `if len(rules)==0 {return nil}`. Grepped `main.go:982,994,1025` — same `caddyProxy` instance is passed to **three** services: `ingressSync`, `domainSvc`, `tmSvc`. Event subscriptions `main.go:1041-1067` additionally call `ingressSync.Sync` on `Online/Offline/Recovered`, multiplying wipes beyond the 30s ticker.

**Logic:** Synchronizer holds empty `rules` map forever → `GroupRulesByRoute([])` yields 0 groups → `mergedRules=nil` → `caddyProxy.updateRoutesAtomic` builds `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80",":443"],"routes":[]}}}}}` and POSTs to `/config/` which **replaces entire running config**. The `gamepanel-domains` server written by `domains.syncCaddyRoutes` (the only writer doing `getRunningConfig`+merge `caddy_proxy.go:523-569`) is destroyed every 30s until next domain event re-syncs it. If `validateConfig` (via `/load`) fails first, no wipe occurs that tick — but success wipes deterministically.

**Reference violated:** Caddy single-doc authority (`admin.go:1091-1099` sub-resource vs full replace) + Traefik provider-convergence atomic snapshot vs Forge five writers.

**Still broken evidence:** No `len(rules)==0` early-return; no population from `trafficmanager`/`domains`; `HealthFilter` has zero producers (`health_filter.go:76-124` not called outside tests) so `FilterHealthy` is pass-through, making `Sync` effectively `UpdateRoutes(nil, policies)` every 30s.

**Recommendation:** Skip `Sync` when `len(is.rules)==0` (no-op); populate synchronizer from single source of truth or delete it; make all writers use sub-resource merges (`/config/apps/http/servers/<name>`) — `UpdateDomainRoutes` pattern.

---

### FIND-02 — Asymmetric merge + validate=apply+snapshot-after makes rollback fictitious (P0, safety) — **STILL BROKEN**

**Where:** `caddy_proxy.go:514-593` `UpdateDomainRoutes` (fetch-modify-write preserving others) vs `caddy_proxy.go:673-720` `updateRoutesAtomic` + `caddy_proxy.go:980-1002` `validateConfig` + `caddy_proxy.go:687-702` snapshot ordering; `caddy_tls.go:186-238` `buildTLSConfig` full-replace + `caddy_tls.go:240-251` JSON-only validate.

**Re-verification:** Read `caddy_proxy.go:523-593` — `UpdateDomainRoutes` does `getRunningConfig → json.Unmarshal → merge gamepanel-domains → json.Marshal → validate → snapshot → apply`. `updateRoutesAtomic` does `buildServerConfig` (only `gamepanel` `caddy_proxy.go:707-720`) → `validateConfig` → `previousConfig=getRunningConfig` → `applyConfig`. Order is inverted. `validateConfig` `caddy_proxy.go:980-1002` POSTs to `/load` with `Cache-Control: must-revalidate` — per `caddyconfig/load.go:68-116` this **replaces** config (forceReload path) — not `/adapt` (`load.go:137-175` is the true dry-run returning `{result,warnings}`).

**Logic:** Step 1 already mutates. Step 2 snapshots the *new* config as `lastValidConfig` (`:691-695`), so `restoreConfig(:697-702)` and `Rollback()` (`:279-293`) replay the bad generation. The `must-revalidate` header does not make `/load` a dry-run; it forces reload even when identical.

**Reference violated:** Caddy `/adapt` contract (`load.go:137-175`) and NPM `configure→test→reload` guard (`backend/internal/nginx.js:27-117` `test()` before/after, `.err` rename, `meta offline` on failure).

**Still broken evidence:** No call to `/adapt` anywhere in `forge/api` (`grep -rn "/adapt" forge/` empty); `validateConfig` still targets `/load`; snapshot still after validate.

**Recommendation:** Snapshot **before** mutation; replace `validateConfig` with `POST /adapt` or sub-resource `POST /config/apps/http/servers/<name>` dry-run; persist `lastValidConfig` durably or drop memory-only rollback.

---

### FIND-03 — Fictive Caddy handlers + all-policies-all-routes + grouped-route miss (P0/P1 cluster) — **STILL BROKEN**

Three sub-bugs share one path; all verified still present:

**3a. Fictive handlers** `caddy_proxy.go:795-875` `buildPolicyHandles` emit `{"handler":"rate_limit","rate":"N/s","burst":M}` and `{"handler":"circuit_breaker","max_failures":…}` — not in Caddy's `modules/caddyhttp/*` registry (CB lives in `reverse_proxy` namespace `reverseproxy.go:109`). Validation via `/load` fails → **every** `UpdateRoutes/SyncRoutes` with any policy errors out. Tests still assert broken rendering (`caddy_proxy_test.go:280-314` `rate_limit`, `caddy_proxy_test.go:547-615` `circuit_breaker`).

**3b. All-policies-all-routes** `traefik_proxy.go:960-972` `collectApplicablePolicies`:
```go
// For now, return all policies since there is no direct rule-to-policy mapping
result := make(map[string]*TrafficPolicy, len(policies))
for k, v := range policies { result[k]=v }
```
plus `crossnode/routegroup.go:149-151` `extractPolicyID() = ""` always, and `caddy_proxy.go:722-793` `buildPolicyRoutes` concatenates every `policyHandles` onto every grouped route. Go map iteration `traefik_proxy.go:769-787` makes middleware order non-deterministic (Traefik order matters `pkg/server/middleware/middlewares.go:50-83`). `RoutingRule.Headers` `service.go:35` persisted but never rendered.

**3c. Grouped-route miss** `caddy_proxy.go:60-74` `RemoveRoutes` DELETEs `/id/gamepanel-<id>` per ruleID; but grouped routes are `@id=gamepanel-<firstRuleID>-group` `caddy_proxy.go:949-951`. `WithdrawNodeTargets` `service.go:753-798` enumerates ruleIDs per server — grouped route with non-first stale rule survives. `CleanupStale` `caddy_proxy.go:350-362` handles `-group` suffix (`:355-357`) so stale *cleanup* eventually catches it but **hot withdraw does not**. Traefik side `traefik_proxy.go:273-293` per-rule router keys miss grouped `gamepanel-<first>` similarly.

**Still broken:** No `rule↔policy` FK/join added; `extractPolicyID` still `return ""`; no deterministic ordering; no `-group` handling in `RemoveRoutes`.

**Recommendation:** Add `rule↔policy` join (`gateway_middlewares` referenced by routers — Traefik shape `http_config.go:38-46`); deterministic order `rate-limit→blacklist→whitelist→CB→redirect`; gate `rate_limit`/`circuit_breaker` behind real module or map to `remote_ip`/`subroute` constructs; make withdraw group-aware.

---

### FIND-04 — TCP rules render as broken HTTP/SNI-only routes; UDP silently rejected but half-implemented (P0) + weights mishandled — **STILL BROKEN**

**Where:** `service.go:383-387` `validateRoutingRule` allows `tcp` but rejects `udp`; `caddy_proxy.go:924-978` `buildGroupedRoute`/**has zero `if protocol=="tcp"` branch** (always `host`+`path`+`reverse_proxy`); `traefik_proxy.go:758-760,854-905` `buildTCPGroupedRoute` correctly branches but Caddy does not; `caddy_proxy.go:934-939` `upstreams[].weight` (weight not a Caddy `Upstream` field — `hosts.go:29`), `caddy_proxy.go:503-507` health re-add loses weight.

**Re-verification:** Inspected `caddy_proxy.go:894-978` — no `protocol` check. `service.go:386` switch explicitly rejects `udp` before it can reach either adapter. `traefik_proxy.go:900-904` UDP remnant writes to `cfg.TCP.Services` that stock Traefik ignores (no `udp:` tree).

**Logic:** TCP rule through Caddy becomes `{"match":{"host":[…],"path":[…]}}` + `reverse_proxy` — game TCP never matches HTTP host header. Traefik TCP uses `HostSNI(`domain`)` so plain-TCP (non-TLS) never matches SNI either. UDP never reaches dataplane correctly via gateway.

**Reference violated:** Traefik separate `tcp:`/`udp:` trees `tcp_config.go`/`udp_config.go`; Caddy `layer4` app pattern; NPM `stream.conf` separation.

**Recommendation:** Drop `tcp`/`udp` from `validateRoutingRule` until supported, or emit Caddy `layer4` app and Traefik proper `udp:` section + `ClientIP`/`CatchAll` for raw TCP; keep L4 LB `loadbalancer/dataplane.go:20-49` as canonical L4.

---

### FIND-05 — GatewayAdapter zero callers + five-writer amplification + admin UX non-functional (P0) — **STILL BROKEN**

**Where:**
- `gateway_adapter.go:38-64` interface (includes `SetCertificate/RemoveCertificate`); `grep -rn SetCertificate forge/` → only interface+impls (`caddy_proxy.go:83-121` + `traefik_proxy.go:373-397` + `gateway_adapter.go:49`).
- Renewal `acme/service.go:355-363` + `main.go:1178-1192` `JobCertRenewal` only call `store.UpdateCertificate`; no gateway hook.
- `caddy_tls.go:18-32` `NewCaddyTLSManager` never constructed (`grep -rn NewCaddyTLSManager forge/` empty; `main.go:982` hardcodes `NewCaddyReverseProxy`).
- `traefik_proxy.go:380-385` writes PEM **bodies** into `certFile/keyFile` **paths** (`TraefikTLSCertificate` `traefik_proxy.go:1100-1108` expects paths).
- `forge/web/app/admin/traffic/page.tsx:14-22` posts `{path,targetGroup,priority,methods}` vs backend `RoutingRule{domain,path,targetPort,…}` `service.go:24-39`; `handlers_trafficmanager.go:80-86` only `GET /policies/:id` (no `GET /policies` list `page.tsx:67-70` →405), `PATCH` `page.tsx:94` vs `PUT` `handlers_trafficmanager.go:42` →405; cert upload `POST /certificates` 405 (`certificates/page.tsx:43-50` vs `handlers_certificates.go:15-77` no `POST /`).

**Logic:** Forge's ACME rinnova persists STAR-certs but listeners never receive `tls/certificates/<sni>` material — "Verified" domain status is a Postgres flag, not a handshake proof. Typed UI vs typed API mismatch makes traffic page 100% failure without curl work-arounds. `TraefikProxy` 1,139 lines `traefik_proxy.go:1-1139` is constructed only from tests (`grep NewTraefikReverseProxy` outside tests empty).

**Reference violated:** Traefik `addCertificateForDomain → hot-reload` (`provider.go:796-802`), NPM reload post-renew (`certificate.js:155-186`), Caddy certmagic swap.

**Still broken:** Zero new callers added; wiring unchanged at `main.go:982-1028`; UI types unchanged.

**Recommendation:** Wire `SetCertificate` after `Create/Update/RenewCertificate`; fix Traefik cert paths (write `*.pem` files); construct or delete `CaddyTLSManager`; rebuild admin against real schemas (already documented `phase-04/subagent-03 §13-15`); decide Traefik adapter fate: file-watch model `provider/file/file.go:90,170-207` `fsnotify` or delete 1,139 lines.

---

## 3. Reconciliation statement (phase-04 × final-parity × phase-06)

| Source | Claim | Re-verified verdict |
|---|---|---|
| `phase-04/synthesis.md` §2-3 | Five overlapping gateways writing to one Caddy; 7 tables for 3 concepts; two migrations with divergent columns | **CONFIRMED** — `main.go:982,994-995,998,1025` share one `caddyProxy`; migrations still divergent (`038_traffic_routing.sql` vs `083_a_traffic_rules.sql` mapped by `store_traffic.go:11` vs `store_routing.go:21`) |
| `phase-04/synthesis.md` §8-9 | 10 P0s (F-NET-01..10); F-NET-03 validate=apply via `/load` vs true `/adapt` | **CONFIRMED** — `caddy_proxy.go:980-1002` still POSTs `/load`; `caddyconfig/load.go:68-72,137-175` confirms `/load`=apply, `/adapt`=dry-run |
| `final-parity/subagent-07` §0 | Densest P0 cluster — 10 P0s — single-writer lesson | **CONFIRMED** — unchanged; no new single-writer reconciler introduced |
| `final-parity/subagent-07` §1 | 18 rows; rows 11-14 carry the P0 cluster (middleware chain, dry-run, cert delivery, config authority) | **CONFIRMED** — reproduced 18 rows; same 4 P0-bearing rows remain BROKEN |
| `final-parity/subagent-07` §2 LF-01..05 | Empty-sync wipes, cert non-delivery, validate=apply, fictive+all-policies, Traefik dead + health fiction + UI mismatch | **ALL STILL OPEN** — detailed FIND-01..05 above |
| `phase-06/subagent-08` §C10/C06/C14 | Per-node single-writer Caddy (`caddyconfig/controller.go:92,148-200` fingerprint-cache `never writes invalid`) vs Forge central IngressSynchronizer pushing full set each tick | **CONFIRMED** — Uncloud lesson not yet applied; Forge still full-replace per tick `ingress_sync.go:166` `UpdateRoutes(mergedRules)` |
| `phase-06/subagent-08` LF-02 | Membership-blind data plane risk (TODO in Uncloud DNS/Caddy) — Forge explicitly couples heartbeat → LB marks but crossnode HealthFilter producer-less | **CONFIRMED** — `health_filter.go:76-124` zero prod callers; `servicediscovery/reachability.go:58-79` dials from API not source node (vantage mismatch per `phase-04/synthesis` F-NET-20) |

**Net:** No contradictory evidence found. Phase-04 + final-parity + phase-06 are mutually consistent; current snapshot shows zero remediation of the five-writer / empty-sync / fictive-handler / cert-delivery P0 chain.

---

## 4. Evidence index (representative file:line — inspected this pass)

- Wiring: `forge/api/cmd/api/main.go:982` `NewCaddyReverseProxy`, `:994-995` `IngressSynchronizer.Start(30s)`, `:1025` `NewWithPersistence(...,caddyProxy)`, `:1041-1067` 7 subscriptions (ingress×3 + tmSvc + lbSvc), `main.go:1178-1192` `JobCertRenewal`
- Caddy adapter: `caddy_proxy.go:44-49` struct, `:51-74` `RemoveRoutes`, `:83-121` `SetCertificate`, `:149-204` `GetActiveConnections`, `:234-293,295-373` `ValidateConfig/CleanupStale`, `:375-377` `Health()` hardcoded, `:379-512` `SetUpstreamHealth` (append-bug `:503-508`, `Contains` `:457`), `:514-593` `UpdateDomainRoutes` merge, `:595-655` wildcard, `:673-720` `updateRoutesAtomic` (validate-before-snapshot), `:707-720` `buildServerConfig` (`gamepanel` only), `:722-793` `buildPolicyRoutes`, `:795-875` fictive handlers, `:894-978` grouping+`buildGroupedRoute` (`-group` `:949-951`, `weight` `:937-939`, `lb_policy` `:973-975`), `:980-1052` `validate/getRunning/apply/restore`
- TLS second client: `caddy_tls.go:18-32,58-175,186-238,240-251` (never constructed)
- Traefik adapter 1,139 lines: `traefik_proxy.go:243-293` `UpdateRoutes` (round-trip validate), `:273-293` `RemoveRoutes`, `:373-397` `SetCertificate` (PEM-as-path), `:471-559` `CleanupStale`, `:561-604` `Health`, `:606-697` `SetUpstreamHealth`, `:725-753,755-905` grouping+TCP branch, `:907-972` `buildMiddlewares/collectApplicablePolicies` (returns all), `:974-1005` `validateYAMLConfig`, `:1065-1084` `reloadTraefik POST /api/refresh` dead, `:1100-1108` TLS paths
- Gateway abstraction: `gateway_adapter.go:10-64` `GatewayAdapter`; legacy `service.go:101-105` `ReverseProxy`, `domains/service.go:74-76` `caddyUpdater`; zero callers `gateway_adapter.go:38`
- Traffic service: `service.go:24-54` models, `:361-405` `validateRoutingRule`, `:291-359,562-618,753-951,1001-1082` CRUD/Apply/Sync/withdraw/probe; `service.go:620-673` `resolveTargets`
- LB: `loadbalancer/service.go:22-69,480-482` `__weighted` shared counter, `:552-639` node-mark; `dataplane.go:20-49,62-92,115-270,296-326` listeners + `runHealthChecks` fixed 2s
- Crossnode: `ingress_sync.go:23-37,111-198,200-228` empty-sync; `routegroup.go:32-97,149-151` grouping+`extractPolicyID=""`; `health_filter.go:42-56,76-124` threshold with no producers; `resolver.go:59-111,135-166` 30s `localhost` cache
- Discovery: `servicediscovery/registry.go:32-40,103-108`; `reachability.go:58-79` API-vantage dial; `networkpolicy.go:35-69` test-only
- References verified: `reference/networking/caddy/caddyconfig/load.go:68-72,116,137-175` `/load` vs `/adapt`; `reference/networking/caddy/admin.go:1067-1069,1091-1099` `must-revalidate` + `/id/<id>`; `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46,62-99` router→middleware→service; `reference/networking/traefik/pkg/provider/file/file.go:90,170-207` `fsnotify` watch; `reference/networking/traefik/pkg/api/handler.go:103-133` GET-only API; `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117` `configure→test→reload` guard

---

## 5. Recommended activation order (unchanged — densest P0 first)

1. **Stop the bleeding:** guard empty `IngressSync.Sync` (`len(rules)==0 → return nil`); route ALL Caddy writes through ONE writer (or sub-resource merges); snapshot **before** validate; `POST /adapt` not `/load`.
2. **Gate fictive handlers:** never emit `rate_limit`/`circuit_breaker` until real modules exist; fix `all-policies-all-routes` via `rule↔policy` FK + deterministic order.
3. **Make grouped-route lifecycle consistent:** `RemoveRoutes` must handle `-group` suffix like `CleanupStale` does; make `SetUpstreamHealth` preserves weight and idempotent.
4. **Fix or drop TCP/UDP gateway path:** either remove `tcp` from admission or emit `layer4`/`udp:` correctly; keep `loadbalancer/dataplane.go` as canonical L4.
5. **Wire cert delivery:** call `GatewayAdapter.SetCertificate` after every issue/renew/upload; fix Traefik PEM-as-path; construct or delete `CaddyTLSManager`; mount `acme.HTTPSolver()` on `:80` or proxy `/.well-known/acme-challenge/*`.
6. **Rebuild admin against real schemas** (`handlers_trafficmanager.go:107`, `service.go:24-39`).
7. **Decide Traefik adapter fate** — file-watch model `provider/file/file.go:90` or delete 1,139 lines.

*Re-verification produced with file:line citations only; no product code modified.*
