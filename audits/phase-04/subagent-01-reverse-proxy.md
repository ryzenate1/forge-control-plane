# Phase 4 — Networking Cluster · Subagent 01: Reverse Proxy / Routing / Load Balancing

**Dimension:** reverse proxy (HTTP/HTTPS/TCP/UDP/WebSocket), routing (upstreams, load balancing, health checks, weighted routing, failover), dynamic config
**References:** `reference/networking/caddy`, `reference/networking/traefik`, `reference/networking/nginx-proxy-manager`
**Forge:** `forge/api/internal/services/{trafficmanager,loadbalancer,servicediscovery,crossnode,dns,domains}`, `forge/api/internal/http/handlers_{trafficmanager,loadbalancer,domains,proxy_domains}.go`, `forge/api/migrations`
**Date:** 2026-08-23 · **Method:** static inspection of both trees; every path:line below was read before being cited.

---

## Legend

- STATUS: `PARITY` / `PARTIAL` / `MISSING` / `BROKEN` (present but non-functional)
- SEVERITY: `P0` traffic-affecting correctness · `P1` major functional gap · `P2` polish/robustness

---

## Capability Comparison Matrix

### 1. HTTP host/path routing → gateway routes

- REFERENCE caddy:`modules/caddyhttp/reverseproxy/reverseproxy.go`:Handler (`reverseproxy.go:100`) — JSON `reverse_proxy` handler with host/path matchers; directive registered in `caddyconfig/httpcaddyfile/directives.go:94`.
- REFERENCE traefik:`pkg/config/dynamic/http_config.go`:Router (`http_config.go:84`) — rule + entryPoints + service ref; built from labels by `pkg/provider/docker/config.go`:DynConfBuilder.build (`config.go:32`).
- REFERENCE nginx-proxy-manager:`backend/templates/proxy_host.conf` — per-host nginx server block; rendered by `backend/internal/nginx.js`:configure (`nginx.js:27-99`) after uniqueness checks in `backend/internal/proxy-host.js`:create (`proxy-host.js:21-108`).
- FORGE Frontend: none (no admin UI page; SDK `packages/sdk/src/client.ts` has zero traffic/LB/domain methods). API: `internal/http/handlers_trafficmanager.go:15-67`. Service: `internal/services/trafficmanager/service.go`:RoutingRule (`service.go:24`) with DB-first CRUD (`service.go:291-359,407-427`). Store: `internal/store/store_routing.go`:RoutingRuleRow. DB: `migrations/083_a_traffic_rules.sql` (+`133_b_routing_rules_persistence.sql`,`111_routing_rules_websocket.sql`). Worker: 2-min reconcile loop (`service.go:953-993`). Event: NodeOffline/Recovered subscriptions (`cmd/api/main.go:1025-1027`). Tests: `service_test.go`, `persistence_test.go`.
- STATUS: PARTIAL
- GAP: rules only reach the gateway when an admin calls `POST /admin/traffic/sync` (`handlers_trafficmanager.go:107-112`); Create/Update never invoke the proxy — contrast NPM where every CRUD ends in configure+reload (`proxy-host.js:88`). No route priority/ordering semantics for overlapping host+path rules.
- FINDING R1
- RECOMMENDATION: auto-invoke SyncRoutes on rule mutation (or event-driven reconciler); add explicit route priority.
- SEVERITY: P1

### 2. Route grouping into multi-upstream pools

- REFERENCE traefik:`pkg/config/dynamic/http_config.go`:ServersLoadBalancer.Merge (`http_config.go:404-425`) — dedupe-merge of servers across duplicate service definitions.
- REFERENCE caddy:`modules/caddyhttp/reverseproxy/hosts.go`:UpstreamPool (`hosts.go:29-30`) — stable pool per handler feeding selection policies.
- FORGE Service: `internal/services/trafficmanager/caddy_proxy.go`:groupRules (`caddy_proxy.go:894-922`) groups by `domain|path|protocol`; buildGroupedRoute emits N upstreams (`caddy_proxy.go:924-978`). Duplicate implementation `traefik_proxy.go:groupRules` (`traefik_proxy.go:725-753`). Third canonical version `internal/services/crossnode/routegroup.go`:GroupRulesByRoute (`routegroup.go:32-68`) + UniqueBackends dedupe (`routegroup.go:70-97`).
- STATUS: PARTIAL
- GAP: grouping triplicated with divergent behavior; crossnode normalizes empty path→"/" (`routegroup.go:44-49`) but adapters do not, so rules group differently across code paths.
- LOGIC FINDING F-A (duplicate route-group generators): three independent generators fight over one gateway config: domains.Service.syncCaddyRoutes writes server `gamepanel-domains` (`domains/service.go:455-489` → `caddy_proxy.go:514-593`); trafficmanager.SyncRoutes full-replaces server `gamepanel` (`caddy_proxy.go:673-720`); crossnode.IngressSynchronizer.Sync runs every 30 s calling UpdateRoutes again (`ingress_sync.go:166`, started `cmd/api/main.go:994`). See Findings F1/F2.
- RECOMMENDATION: single source of truth (promote `crossnode.RouteGroup`); normalize path before grouping.
- SEVERITY: P1

### 3. Load-balancing strategies / selection policies

- REFERENCE caddy:`modules/caddyhttp/reverseproxy/selectionpolicies.go` — policies registered at `selectionpolicies.go:41-51`; namespace wired at `reverseproxy.go:1651`.
- REFERENCE traefik:`pkg/server/service/loadbalancer/` — `wrr/wrr.go`, `p2c`, `hrw`, `leasttime`, `failover/failover.go`:Failover.ServeHTTP (`failover.go:73`), sticky cookie; strategy enum `http_config.go:371-382`.
- FORGE Service: `RoutingRule.Strategy` (`service.go:33`); Caddy adapter passes strategy verbatim to `lb_policy` (`caddy_proxy.go:973-975`); Traefik adapter converts any non-round_robin strategy into a sticky cookie (`traefik_proxy.go:823-825,841-849`); standalone L4 LB supports round_robin|least_connections|ip_hash|weighted_round_robin (`loadbalancer/service.go:24-29,416-428`).
- STATUS: PARTIAL
- GAP: no validation of strategy names against gateway vocabulary — Forge's own name `least_connections` matches no Caddy policy (`least_conn`), producing an unloadable config; strategy→sticky mistranslation silently changes semantics per adapter.
- RECOMMENDATION: whitelist + translate strategies per adapter; reject unknown values in validateRoutingRule (`service.go:361-405`).
- SEVERITY: P1

### 4. Weighted routing

- REFERENCE caddy:`selectionpolicies.go`:WeightedRoundRobinSelection (`selectionpolicies.go:82-98`) — weights live ON THE POLICY (`Weights []int` ordered to match upstreams); `Upstream` has only dial/max_requests (`hosts.go:35-65`).
- REFERENCE traefik:`http_config.go`:Server.Weight (`http_config.go:470-478`), WRRService.Weight (`http_config.go:278-281`).
- FORGE Service: `RoutingRule.Weight` (`service.go:34`); Caddy adapter emits `"upstreams":[{"dial":…,"weight":N}]` (`caddy_proxy.go:936-939`); Traefik adapter emits per-server weight (`traefik_proxy.go:813-818`); Weight carried in crossnode BackendAddr (`routegroup.go:142-147`).
- STATUS: BROKEN (Caddy adapter)
- GAP: Caddy's Upstream JSON schema has no `weight` field — key silently discarded, weighted rules behave unweighted on the default adapter.
- LOGIC FINDING F-B: weights dead on Caddy; additionally health re-add appends bare `{dial}` losing weight (`caddy_proxy.go:503-507`; same in Traefik `traefik_proxy.go:652-654,678-680`).
- RECOMMENDATION: emit `lb_policy weighted_round_robin` with ordered weights when any group weight>0; preserve weight on recovery.
- SEVERITY: P1

### 5. Active health checks (HTTP-aware)

- REFERENCE caddy:`modules/caddyhttp/reverseproxy/healthchecks.go`:ActiveHealthChecks (`healthchecks.go:73-134`) — uri/port/headers/method/body/interval/timeout/passes/fails/expect_status/expect_body; timeout default 5s (`healthchecks.go:159-162`).
- REFERENCE traefik:`http_config.go`:ServerHealthCheck (`http_config.go:483-496`) — mode http|grpc, path/method/status/port/interval/unhealthyInterval/timeout/headers; executor `pkg/healthcheck/healthcheck.go`:ServiceHealthChecker.Launch (`healthcheck.go:134`); defaults 30s/5s (`http_config.go:16-19`).
- FORGE Service: no health-check fields on TrafficPolicy; only TCP dial probing via `trafficmanager.ProbeTargets` (`service.go:1001-1082`; threshold `service.go:22`; 2 s dial `service.go:1026`). LB model has HealthCheckConfig (`loadbalancer/service.go:62-69`; DB `migrations/082_b_target_groups.sql:7`) and dataplane `runHealthChecks` (`loadbalancer/dataplane.go:296-326`).
- STATUS: MISSING (HTTP probes) / BROKEN (LB config ignored)
- GAP: no path/status/method/interval-configurable probe anywhere; runHealthChecks ignores HealthCheckConfig entirely (raw TCP dial every fixed 2 s tick, `dataplane.go:36,317`); UDP targets skipped (`dataplane.go:306-308`).
- LOGIC FINDING F-C (missing health check): configured HTTP health checks are stored in DB and returned by the API but never executed.
- RECOMMENDATION: implement an HTTP prober honoring Path/Status/Interval/Timeout/thresholds; per-group interval; explicit TCP-only mode per protocol.
- SEVERITY: P1

### 6. Passive health checks / failover-on-error

- REFERENCE caddy:`healthchecks.go`:PassiveHealthChecks (`healthchecks.go:233-246`) consulted in Upstream.Healthy() (`hosts.go:84-93`); request retry/failover via LoadBalancing.Retries/TryDuration/TryInterval/RetryMatch (`reverseproxy.go:1647-1687`) and tryAgain (`reverseproxy.go:1342`).
- REFERENCE traefik:`pkg/server/service/loadbalancer/failover/failover.go` (`failover.go:16-73`) fallback handler switched by balancer status; PassiveServiceChecker (`pkg/healthcheck/healthcheck.go:343-371`).
- FORGE Service: LB proxyTCP marks a target Unhealthy on a SINGLE failed dial and drops the client without trying another target (`loadbalancer/dataplane.go:142-146`); one successful probe flips back healthy (`dataplane.go:320`). trafficmanager has no in-request failover; removal only via 2-min reconciler (`service.go:900-951`).
- STATUS: PARTIAL
- GAP: no retry-to-next-upstream (Caddy TryDuration equivalent); single-failure flip-flop; no draining grace.
- RECOMMENDATION: bounded next-target retries within one connection attempt; N-failure threshold (reuse crossnode.DefaultHealthThreshold).
- SEVERITY: P1

### 7. WebSocket support

- REFERENCE caddy: transparent upgrade handling by default; streaming flush control in `streaming.go`:flushInterval (`streaming.go:241-263`).
- REFERENCE nginx-proxy-manager:`backend/templates/proxy_host.conf:19-23,39-43` — conditional Upgrade/Connection headers + `proxy_http_version 1.1`.
- FORGE Service: `RoutingRule.WebSocket`; Caddy adapter sets header_up Connection/Upgrade (`caddy_proxy.go:966-971`); Traefik adapter sets responseForwarding.flushInterval "0ms" when WS (`traefik_proxy.go:835-839`); proxy_domains.websocket column (`migrations/117_domains_certificates.sql:17`).
- STATUS: PARITY
- GAP: minor — manual Connection/Upgrade header_up unnecessary on modern Caddy; flushInterval semantics differ from Traefik guidance (disable batching) but harmless.
- RECOMMENDATION: rely on gateway defaults; document flush behavior.
- SEVERITY: P2

### 8. TCP/UDP (L4) routing

- REFERENCE traefik:`http_config.go` separate top-level tcp:/udp: sections with TCPRouter/UDPRouter; docker provider builds them separately (`provider/docker/config.go:107` buildTCPServiceConfiguration, `config.go:137` buildUDPServiceConfiguration).
- REFERENCE nginx-proxy-manager:`backend/templates/stream.conf:8-27` — nginx stream blocks, TCP+UDP listeners with reuseport.
- FORGE Service: routing rules validate protocol ""|http|https|tcp only (`trafficmanager/service.go:383-387`) — udp rejected at admission; Traefik adapter anticipates udp (`traefik_proxy.go:758,878-881`) but renders UDP services under cfg.TCP.Services (`traefik_proxy.go:900-904`) which stock Traefik ignores; Caddy adapter renders tcp identically to http (buildGroupedRoute never branches on protocol, `caddy_proxy.go:924-978`) and never emits a layer4 app. Standalone LB dataplane does real TCP+UDP proxying with session reaper (`dataplane.go:115-270`), gated by LOAD_BALANCER_ENABLED (`service.go:107`, wired `cmd/api/main.go:820`).
- STATUS: BROKEN (gateway TCP) / MISSING (gateway UDP)
- LOGIC FINDING F-D (wrong L4 behavior): a `tcp` rule through the default Caddy adapter becomes an HTTP Host route that raw game traffic can never match; through the Traefik adapter the TCP router uses HostSNI(`domain`) (`traefik_proxy.go:883-888`) which only matches TLS SNI, so plain-TCP servers get no route either.
- RECOMMENDATION: drop tcp/udp from traffic rules until supported, or emit Caddy layer4 app config and correct Traefik ClientIP/CatchAllNoTLS rules + proper `udp:` section.
- SEVERITY: P0

### 9. Dynamic config apply: validate-before-load & rollback

- REFERENCE caddy:`admin.go` — `Cache-Control: must-revalidate` dry-run (`admin.go:1067-1069`); POST /config/ atomic replace; targeted ops under /id/ (`admin.go:1091-1099`).
- REFERENCE traefik: file-provider auto-watch; REST API read-only for runtime state (`pkg/api/handler.go:103-114` — GET /api/rawdata, GET /api/http/routers, ...).
- REFERENCE nginx-proxy-manager:`backend/internal/nginx.js`:configure/test/reload (`nginx.js:27-117`) — test → generate → test → on failure mark meta offline + rename .err + delete + reload; status persisted in DB meta (`nginx.js:48-90`).
- FORGE Service: Caddy adapter validates (must-revalidate) then snapshot→apply→restore (`caddy_proxy.go:980-1002`, `673-705`) — sound pattern; Traefik adapter "validates" only by YAML round-trip (`traefik_proxy.go:974-1005`) then calls a nonexistent endpoint (F3 below). Tests cover Caddy atomic reload paths (`caddy_proxy_test.go:15-278`).
- STATUS: PARTIAL
- GAP: no persisted online/offline status per host like NPM's meta.nginx_online/nginx_err; failures surface only as API errors/logs.
- RECOMMENDATION: persist last-apply error per rule/group for admin diagnosis.
- SEVERITY: P2

### 10. Domain/proxy-host virtual hosting & wildcards

- REFERENCE nginx-proxy-manager:`backend/internal/proxy-host.js` — isHostnameTaken uniqueness enforcement (`proxy-host.js:35-47,130-145`), multiple domain_names per host; per-host conf files (`nginx.js:124-129`).
- REFERENCE caddy:`caddyconfig/httpcaddyfile/addresses.go` — site address parsing/dedup incl. wildcards.
- FORGE Service: domains.Service add/list/delete/verify with IDNA+public-suffix normalization (`domains/service.go:504-534`); ownership via HTTP token at /.well-known/forge-verify (`handlers_domains.go:102-124`) and DNS A-record check (`handlers_domains.go:85-99`, `domains/service.go:VerifyOwnership` `service.go:321`). Gateway rendering adds apex+wildcard hosts (`caddy_proxy.go:595-655`). API: handlers_domains.go + handlers_proxy_domains.go.
- STATUS: PARITY (verification flow stronger than NPM) with caveats
- GAP: no hostname-uniqueness enforcement across routing_rules vs proxy_domains/domains — two routes can claim the same host across the two Caddy servers sharing :80 (see F2); syncCaddyRoutes hardcodes fallback localhost:8080 (`domains/service.go:467-468`).
- LOGIC FINDING F-I (stub verify): `POST /domains/:id/verify` returns hardcoded `"verified": true` without any check (`handlers_proxy_domains.go:199-212`) — unlike the real verification in handlers_domains.go/domains.Service.
- RECOMMENDATION: enforce global host uniqueness at admission; implement or remove the stub endpoint.
- SEVERITY: P1

### 11. TLS certificates (ACME + custom upload)

- REFERENCE nginx-proxy-manager:`backend/internal/certificate.js` — certbot LE issuance/renewal worker with 30-day threshold (`certificate.js:22-36,49-102`); DNS plugins (`certificate.js:9`, `lib/certbot.js`).
- REFERENCE traefik:`pkg/api/handler_certificate.go` — runtime certificate surfaces; certs declared in dynamic config (Forge mirrors shape in `traefik_proxy.go:1100-1122`).
- FORGE Service: trafficmanager.CaddyTLSManager.ProvisionLetsEncrypt builds tls.automation ACME issuer config (`caddy_tls.go:186-238`); custom upload/renew/status call `/tls/certificates/<host>` endpoints (`caddy_tls.go:58-175`); separate ACME service + DNS-01 providers (`services/dns/service.go:555-591`; migration `117_domains_certificates.sql`).
- STATUS: BROKEN (custom cert endpoints)
- LOGIC FINDING F-E: Caddy's admin API exposes NO `/tls/certificates/*` resource (surface is /load, /stop, /config/**, /id/**, /pki/**, /reverse_proxy/upstreams — `admin.go`, `modules/caddyhttp/reverseproxy/admin.go:50-54`); UploadCustomCert/RemoveCert/RenewCert/CertStatus 404 against real Caddy. Additionally ProvisionLetsEncrypt posts a whole new root config containing only the gamepanel-domains server (`caddy_tls.go:186-238` + applyConfig POST /config/ `caddy_tls.go:253-280`), wiping running routes, and its validateConfig only checks JSON parseability (`caddy_tls.go:240-251`) instead of a dry-run.
- RECOMMENDATION: patch apps.tls via `/config/apps/tls` sub-resource paths with merge semantics; inject custom certs through `apps.tls.certificates.load_files`; reuse must-revalidate validation.
- SEVERITY: P0

### 12. Policy middlewares (rate limit, IP allow/deny, circuit breaker, redirect)

- REFERENCE traefik:`pkg/config/dynamic/http_config.go` middleware types (mirrored by Forge `traefik_proxy.go:104-208`) — real registered middleware modules.
- REFERENCE caddy: no standard `rate_limit` handler and no standalone `circuit_breaker` handler exist; circuit breaking is a namespace on reverse_proxy — CBRaw `namespace=http.reverse_proxy.circuit_breakers` (`reverseproxy.go:109`); IP filtering via remote_ip matcher (`matchers.go`).
- FORGE Service: TrafficPolicy (`service.go:41-54`); Caddy adapter emits `"handler":"rate_limit"` / `"handler":"circuit_breaker"` handles (`caddy_proxy.go:795-875`); Traefik adapter emits valid middlewares (`traefik_proxy.go:907-958`). Tests assert the broken rendering (`caddy_proxy_test.go:280-314,547-615`).
- STATUS: BROKEN (Caddy adapter)
- LOGIC FINDING F-F: any policy with RateLimit>0 or CircuitBreaker=true makes the emitted Caddy config reference unknown handler modules → the must-revalidate dry-run fails → EVERY UpdateRoutes/SyncRoutes errors out (enabling a policy bricks route updates). Compounding it, collectApplicablePolicies returns ALL policies for every route (`traefik_proxy.go:960-972`) because no rule↔policy link exists in the model (`crossnode/routegroup.go:extractPolicyID` `routegroup.go:149-151` returns "" always), so unrelated policies stack onto all routes.
- RECOMMENDATION: map to real Caddy constructs (remote_ip already fine; drop/gate rate_limit+circuit_breaker behind documented module builds); add policy_id FK to traffic_rules and filter per route.
- SEVERITY: P0

### 13. Health-event failover (node offline → withdraw)

- REFERENCE traefik:`pkg/server/service/loadbalancer/failover/failover.go:73-113` — auto-switch to fallback when balancer DOWN.
- REFERENCE caddy:`hosts.go`:Upstream.Healthy/Available (`hosts.go:77-98`) — per-request exclusion of unhealthy hosts.
- FORGE Service: event-driven withdraw/reinstate (`service.go:739-751,753-817,819-898`); reinstate gated on heartbeat healthy (`service.go:828-830`); LB marks node targets on events (`loadbalancer/service.go:552-639`). Note Handle() also supports NodeDegraded/DrainingStarted (`service.go:744`) but main.go only subscribes Offline/Recovered (`cmd/api/main.go:1025-1027`).
- STATUS: PARTIAL
- LOGIC FINDING F-G (targeted removal misses grouped routes): grouped routes are written with @id `gamepanel-<firstRuleID>-group` (`caddy_proxy.go:949-951`) and Traefik keys `gamepanel-<primary.ID>` (`traefik_proxy.go:763-764`), but WithdrawNodeTargets/RemoveRoutes target per-rule IDs `gamepanel-<ruleID>` (`caddy_proxy.go:60-74`; `traefik_proxy.go:279-287`). When the failing node's rule is NOT the group's first rule nothing is removed — the gateway keeps sending to the dead node until some full rebuild happens. CleanupStale special-cases `-group` suffixes (`caddy_proxy.go:350-362`), evidence the mismatch is known but unresolved for removal.
- RECOMMENDATION: make removal group-aware (rebuild-and-replace minus withdrawn IDs, or per-rule route keys).
- SEVERITY: P0

### 14. Cross-node backend resolution & discovery-backed routing

- REFERENCE traefik:`provider/docker/config.go`:addServer/getIPAddress (`config.go:288-359`) — per-backend container IP resolution.
- REFERENCE caddy:`modules/caddyhttp/reverseproxy/upstreams.go` — pluggable dynamic upstream sources; cleanup semantics discussed at `healthchecks.go:56-66`.
- FORGE Service: servicediscovery registry with statuses healthy|unhealthy|unknown|draining (`servicediscovery/model.go:8-14`), stale reaper marks Unhealthy (`stale_reaper.go:114`), reachability sweep (`service.go:111`); crossnode.Resolver prefers discovery→store→localhost with 30 s TTL cache (`resolver.go:59-111,135-166`); IngressSynchronizer filters backends via HealthFilter.FilterHealthy and omits all-unhealthy routes (`ingress_sync.go:128-142`); cache cleared on node events (`cmd/api/main.go:1045-1069`). API: handlers_crossnode.go resolve/cache endpoints.
- STATUS: PARTIAL
- LOGIC FINDING F-H (dead health producer): HealthFilter.RecordSuccess/RecordFailure have zero production call sites (definitions only, `crossnode/health_filter.go:76-124`; repo-wide grep shows no callers outside tests) → FilterHealthy always passes everything; the "no healthy backends" branch can only trigger from missing data. Also Sync() mutates shared primary.TargetHost/Port pointers without coordination (`ingress_sync.go:139-140`) while Sync runs concurrently from the 30 s ticker AND event handlers (`main.go:994,1045-1069`) — data race.
- RECOMMENDATION: feed HealthFilter from LB dataplane + ProbeTargets results; deep-copy rules before mutation.
- SEVERITY: P1

### 15. Ingress synchronizer lifecycle (empty-state wipe)

- REFERENCE traefik: providers converge to one complete dynamic-config snapshot applied atomically by the configuration watcher (provider model per `pkg/provider/docker/config.go:build` returning a full `*dynamic.Configuration`).
- REFERENCE caddy:`admin.go` — replacement is total at `/config/`; partial edits use `/config/<path>` sub-resources or targeted `/id/<id>` ops (`admin.go:1091-1099`).
- FORGE Service: crossnode.IngressSynchronizer started every 30 s (`cmd/api/main.go:993-994`) and invoked on node events; it calls adapter.UpdateRoutes unconditionally (`ingress_sync.go:166`) even when its rule set is empty.
- STATUS: BROKEN
- GAP: nothing in production ever populates the synchronizer — SetRules/SetPolicies/UpsertRule have no non-test callers (repo-wide grep; only definitions `ingress_sync.go:200-228`). Sync therefore always pushes an EMPTY route set.
- LOGIC FINDING F1 (empty-sync wipes gateway config, 30 s cadence): with zero rules, UpdateRoutes → updateRoutesAtomic builds a bare `gamepanel` server and POSTs it to /config/, REPLACING THE ENTIRE RUNNING CONFIG — including the `gamepanel-domains` server written by domain sync — every 30 seconds. The Caddy admin API's total-replacement semantics make an empty full-push destructive.
- RECOMMENDATION: skip empty syncs (no-op when no rules); populate IngressSynchronizer from trafficmanager/domains state or delete the component; switch adapters to sub-resource merges (/config/apps/http/servers/gamepanel) instead of root replacement.
- SEVERITY: P0

### 16. Full-config replacement asymmetry (routes vs domains)

- REFERENCE caddy:`admin.go` — merge-by-path semantics exist precisely to avoid whole-config stomping.
- REFERENCE nginx-proxy-manager:`backend/internal/nginx.js`:configure — regenerates ONLY the affected host file then reloads; other hosts untouched (`nginx.js:37-40,124-129`).
- FORGE Service: UpdateDomainRoutes carefully MERGES the existing config "to avoid wiping gamepanel routes" (comment `caddy_proxy.go:523`, implementation 524-592), but the inverse path updateRoutesAtomic builds serverConfig containing ONLY the gamepanel server (`caddy_proxy.go:707-720`) and applyConfig posts it to /config/ as a FULL replacement (`caddy_proxy.go:1026-1052`). Same pattern in buildTLSConfig (`caddy_tls.go:186-238`).
- STATUS: PARTIAL
- LOGIC FINDING F2: any ApplyRoutes/SyncRoutes call destroys domain routes and TLS app config (and vice versa is protected). Two servers also both listen on :80 (`caddy_proxy.go:567,713`); Caddy binds overlapping sockets via SO_REUSEPORT for graceful swaps (`listeners.go:115-121`), so coexisting servers sharing :80 yield nondeterministic request distribution rather than clean host separation.
- RECOMMENDATION: always read-modify-write the running config (as UpdateDomainRoutes does); give each logical vhost set its own listen ports or fold into ONE server with ordered routes.
- SEVERITY: P0

### 17. Traefik reload mechanism

- REFERENCE traefik:`pkg/api/handler.go:103-114` — API exposes GET endpoints only (rawdata/routers/services/middlewares); there is no refresh/reload endpoint; file provider picks up changes by watching.
- FORGE Service: TraefikReverseProxy.reloadTraefik POSTs to `/api/refresh` on the admin endpoint (`traefik_proxy.go:1065-1084`).
- STATUS: BROKEN
- LOGIC FINDING F3: stock Traefik has no `/api/refresh` — every write path (UpdateRoutes, RemoveRoutes, SetUpstreamHealth, cert ops) ends in `HTTP 404` → errCount++ → UpdateRoutes triggers restorePreviousConfig (`traefik_proxy.go:263-268`). Against real Traefik the adapter can never successfully apply a route change; tests pass only because they stub reload behavior.
- RECOMMENDATION: rely on file-provider watch (drop the reload call or make it advisory/non-fatal); validate against the live instance via GET /api/rawdata comparison like Traefik's own runtime model.
- SEVERITY: P0

### 18. Standalone L4 load balancer dataplane

- REFERENCE traefik:`pkg/server/service/tcp`, `udp` + `pkg/server/service/loadbalancer/wrr/wrr.go` — production L4 balancing with sticky/weights.
- REFERENCE nginx-proxy-manager:`backend/templates/stream.conf` — TCP/UDP stream hosts with reuseport.
- FORGE Service: loadbalancer.Service with target groups persisted (`store_target_groups.go`; migration `082_b_target_groups.sql`), algorithms RR/LC/IP-hash/WRR (`service.go:416-492`), real TCP+UDP proxying incl. UDP session TTL reaper (`dataplane.go:115-270`), listener reconciliation loop (`dataplane.go:20-92`), event-driven node health marking (`service.go:552-639`). Tests: `dataplane_test.go`, `load_test.go`, `loadbalancer_scenario7_test.go`. API: handlers_loadbalancer.go:24-138 incl. `/next` selection preview.
- STATUS: PARTIAL
- GAP: disabled by default (env-gated, `service.go:107`); weighted WRR counter shared across ALL groups via single "__weighted" key (`service.go:480-482`) coupling group sequences; least_connections counters only decremented by dataplane release (`dataplane.go:271-277`) so API-driven NextTarget previews permanently inflate counts; no connection draining on target removal (RemoveTarget just deletes, `service.go:365-388`).
- RECOMMENDATION: per-group weighted counters; decrement on preview or separate dry-run selector; drain-then-remove semantics.
- SEVERITY: P2

---

## Consolidated Logic Findings (ranked)

| ID | Summary | Where | Severity |
|----|---------|-------|----------|
| F1 | Empty-rule ingress sync replaces whole Caddy config with bare server every 30 s (rules never populated) | `crossnode/ingress_sync.go:111-166` + no SetRules callers + `cmd/api/main.go:994`; `caddy_proxy.go:673-720,1026-1052` | P0 |
| F-G | Withdraw/remove targets per-rule IDs while grouped routes are keyed `<firstRule>-group` → dead-node traffic persists after node offline | `caddy_proxy.go:60-74,949-951`; `traefik_proxy.go:279-287,763-764` | P0 |
| F-F | rate_limit/circuit_breaker are not real Caddy handlers → policy-enabled configs fail validation and brick all route updates; all policies stack onto all routes (no rule↔policy link; extractPolicyID returns "") | `caddy_proxy.go:798-804,858-872` vs caddy `reverseproxy.go:109`; `traefik_proxy.go:960-972`; `crossnode/routegroup.go:149-151` | P0 |
| F-E | TLS manager calls nonexistent Caddy endpoints `/tls/certificates/*` and full-replaces config on ACME provisioning; fake JSON-only validation | `caddy_tls.go:58-175,186-251,253-280` vs caddy `admin.go`, `reverseproxy/admin.go:50-54` | P0 |
| F-D | TCP routing rules render as HTTP routes (Caddy) or SNI-only TCP routers (Traefik); UDP rejected at admission yet half-implemented under Traefik `tcp:` section | `trafficmanager/service.go:383-387`; `caddy_proxy.go:924-978`; `traefik_proxy.go:758,854-904` | P0 |
| F2 | Route updates wipe domain/TLS config (full replace vs careful merge asymmetry); two servers share :80 | `caddy_proxy.go:523-592 vs 707-720`; `caddy_tls.go:186-238` | P0 |
| F3 | Traefik adapter depends on nonexistent `/api/refresh` → every apply fails against stock Traefik | `traefik_proxy.go:1065-1084` vs `pkg/api/handler.go:103-114` | P0 |
| F-B | Weighted routing silently ignored on Caddy (weight not an upstream field); weights lost on health re-add both adapters | `caddy_proxy.go:936-939,503-507`; `hosts.go:35-65`; `selectionpolicies.go:82-98`; `traefik_proxy.go:652-680` | P1 |
| F-C | LB HealthCheckConfig stored but never executed (fixed raw TCP dial, 2 s tick, UDP skipped); no HTTP probes anywhere | `loadbalancer/dataplane.go:36,296-326`; `loadbalancer/service.go:62-69` | P1 |
| F-H | HealthFilter has no producers → FilterHealthy is a pass-through; IngressSynchronizer mutates shared rule pointers (race) | `crossnode/health_filter.go:76-124` (no callers); `crossnode/ingress_sync.go:139-140` | P1 |
| F-A | Triplicated route grouping with divergent normalization; three generators fight over one gateway config | `caddy_proxy.go:894-978`; `traefik_proxy.go:725-753`; `crossnode/routegroup.go:32-97` | P1 |
| R1 | Routing rules require manual `/admin/traffic/sync` to reach gateway; no auto-apply on CRUD | `handlers_trafficmanager.go:107-112`; NPM contrast `proxy-host.js:88` | P1 |
| F-I | Proxy-domain verify endpoint is a hardcoded stub (`verified: true`) | `handlers_proxy_domains.go:199-212` | P1 |

Frontend gap (dimension-wide): none of these capabilities has UI surface — `web/app` contains no traffic/load-balancer/proxy-domain pages and `packages/sdk/src/client.ts` exposes none of the `/admin/traffic`, `/admin/load-balancer`, `/domains*` endpoints.

## Test coverage notes

- Strong: atomic reload/rollback for Caddy (`trafficmanager/caddy_proxy_test.go:15-278`), persistence round-trips (`persistence_test.go`, `service_test.go`), Traefik rendering incl. failure restore (`traefik_proxy_test.go:21-341`), crossnode grouping/scenarios (`crossnode_test.go`, `scenario7_test.go`), LB algorithms under load (`loadbalancer/load_test.go`).
- Missing: no test renders a policy through the real Caddy module registry (would catch F-F); none exercises removal of a non-first grouped rule (F-G); none asserts config preservation across mixed route/domain updates (F1/F2); none runs against real Traefik API semantics (F3).

## Verification commands used during audit

- Forge wiring: `grep -n "trafficmanager\|loadbalancer\|IngressSynchronizer" forge/api/cmd/api/main.go`
- Dead code checks: repo-wide grep for `SetRules|UpsertRule|RecordSuccess|RecordFailure` outside `_test.go`
- Reference API surfaces: `reference/networking/caddy/admin.go:1067`, `.../reverseproxy/admin.go:50-54`, `reference/networking/traefik/pkg/api/handler.go:103-114`
