# Phase 4 / Subagent 4 — Forge Gateway Abstraction vs Three Gateways (caddy · traefik · nginx-proxy-manager)

Cluster: `reference/networking/{caddy,traefik,nginx-proxy-manager}`
Scope: joint comparison of the three gateway models + audit of Forge's gateway fragmentation, and a proposal for THE Forge Gateway abstraction.
All paths relative to repo root unless noted. No product code was modified.

---

## 1. Executive summary

Forge does not have one gateway; it has **at least five overlapping "gateways"** spread across services, tables, handlers and adapters:

| # | Gateway-ish surface | Code |
|---|---------------------|------|
| 1 | TrafficManager (L7 routes + policies) | `forge/api/internal/services/trafficmanager/` |
| 2 | LoadBalancer (own L4 dataplane) | `forge/api/internal/services/loadbalancer/` |
| 3 | Domains service → Caddy `gamepanel-domains` server block | `forge/api/internal/services/domains/service.go:455-489` |
| 4 | Crossnode IngressSynchronizer (3rd writer to the same Caddy) | `forge/api/internal/services/crossnode/ingress_sync.go:111-198` |
| 5 | ProxyDomains + RedirectRules + SecurityHeaders (npm-style row model, CRUD-only) | `forge/api/internal/store/store_proxy_domains.go`, `store_redirect_rules.go`, `store_security_headers.go` |

The reference projects each have exactly **one** internal model: Traefik's `provider → router → middleware → service` chain (`reference/networking/traefik/pkg/config/dynamic/http_config.go:38-99`), Caddy's single JSON config of `routes[] → matchers → handlers` (`reference/networking/caddy/modules/caddyhttp/routes.go:31-41`), and NPM's single `proxy-hosts` row (`reference/networking/nginx-proxy-manager/backend/schema/components/proxy-host-object.json`). Forge should adopt a Traefik-shaped **declarative Gateway model** (Router/Middleware/Service tables) behind the existing `GatewayAdapter` interface — see §6.

Three logic bugs found in production wiring are called out in §5 (config wipe race, dead adapter wiring, all-policies-applied-to-all-routes).

---

## 2. The three reference gateway models

### 2.1 Traefik — provider → router → middleware → service

- Dynamic config is one tree: `HTTPConfiguration{Routers, Services, Middlewares}` (`reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46`); TCP/UDP live in sibling sections (`tcp_config.go`).
- A **Router** matches (Rule) and points at exactly one **Service**, plus an ordered **Middlewares** list (`http_config.go:84-99`). Request path: `Client → EntryPoint → Router → Middleware chain → Service → Backend` (per `reference/networking/traefik/AGENTS.md`, "Core vocabulary").
- Middleware is a *named, referenced* object; the router references it by key (`http_config.go:43`, router field `Middlewares []string` at :88). This indirection is what makes policy reuse possible.
- Services compose: `LoadBalancer`, `Weighted`, `Mirroring`, `Failover` (`http_config.go:62-69`) — LB strategy is a property of the *service*, not the route.
- Providers merely *produce* this same dynamic config (file, docker, k8s…), so adding a source never forks the model.

### 2.2 Caddy — one JSON document of routes/handlers

- Config is a single JSON doc; routes are `{match[], handle[], terminal}` arrays under `apps.http.servers.<name>.routes` (`reference/networking/caddy/modules/caddyhttp/routes.go:31-41`; consumed by Forge's adapter via `getRunningConfig` at `caddy_proxy.go:1004-1024`).
- Handlers are modules composed inline in `handle[]`; grouping/mutual-exclusion is expressed with `Group` on the route (:37).
- The admin API is transactional per-path (`POST /config/[path]`), which is why Forge's adapter can validate-then-snapshot-then-apply (`caddy_proxy.go:673-705`).

### 2.3 nginx-proxy-manager — one row per proxy host

- Everything a host needs is denormalized into a single `proxy-hosts` object: `domain_names[], forward_host/port/scheme, certificate_id, ssl_forced, hsts_enabled, http2_support, allow_websocket_upgrade, caching_enabled, block_exploits, access_list_id, locations[], advanced_config` (`backend/schema/components/proxy-host-object.json`, required list lines 4-25).
- Sibling row types for the other shapes: `redirection-hosts`, `streams` (TCP/UDP), `dead-hosts` (404 parking), `certificates`, `access-lists` (`backend/schema/paths/nginx/` directory listing).
- Nginx config is generated from rows and reloaded as a unit; there is no middleware composition, but also no ambiguity about "where does rate limiting come from".

---

## 3. Comparisons (15) — reference model vs Forge's current fragmentation

1. **Single source of truth vs five writers.** Each reference has one config authority (Traefik dynamic config; Caddy `/config/`; NPM `proxy_hosts` table). Forge has three independent writers to the *same* Caddy instance: trafficmanager (`main.go:1025`), domains service (`main.go:998` → `domains/service.go:455-489`), crossnode IngressSynchronizer (`main.go:994`), plus loadbalancer bypassing gateways entirely with its own listeners (`loadbalancer/dataplane.go:20-49`).
2. **Router concept duplication.** Traefik: one Router type. Forge has four row-level "route" concepts: `traffic_rules` (`migrations/083_a_traffic_rules.sql`), `proxy_domains.path` (`store_proxy_domains.go:24`), `redirect_rules.source_path→target_url` (`migrations/117_domains_certificates.sql:35`), and domain-routes synthesized from verified `server_domains` (`domains/service.go:455-489`). Same question ("which hostname+path goes where?") answered 4 ways.
3. **Service/backend concept duplication.** Traefik separates Service from Router. Forge splits backends across `traffic_rules.target_host/target_port` (`store_routing.go:21-37`), `target_groups` + `target_group_targets` (`migrations/082_b_target_groups.sql`), and `proxy_domains.service_id/service_type/port` (`store_proxy_domains.go:16-19`) — three backend models, none shared.
4. **Middleware/policy referencing.** Traefik routers reference middlewares by name (`http_config.go:88`). Forge's `traffic_policies` table (`038_traffic_routing.sql:21-36`) has **no FK or join to `traffic_rules`**; both adapters therefore apply every policy to every route (`traefik_proxy.go:960-972` returns all policies verbatim; `caddy_proxy.go:722-747` enriches every route with the concatenated handles of *all* policies).
5. **Policy embedded in the row (npm style) AND policy as separate table (traefik style) coexist.** `proxy_domains` embeds `rate_limit/rate_limit_burst/websocket/forward_auth_url` per-row (`store_proxy_domains.go:28-30`) while `traffic_policies` carries the same knobs (`store_traffic.go:27-42`) and neither feeds the other. NPM picks one place; Forge has two.
6. **LB strategy placement.** Traefik: strategy belongs to the Service (`Weighted`, LB options, `http_config.go:64-66`). Forge puts `strategy`/`weight` on the *route* (`traffic_rules.strategy`, `038_traffic_routing.sql:9-10`) and separately reimplements strategies inside the L4 loadbalancer service (`loadbalancer/service.go:416-427`: round_robin/least_connections/ip_hash/weighted_round_robin) with no bridge to the adapters.
7. **Redirects triplicated.** NPM: dedicated `redirection-hosts` row type. Traefik: `redirectScheme/redirectRegex` middlewares. Forge has *three*: `redirect_rules` table (write-only, see finding F3), an always-built HTTPS redirect route `gamepanel-https-redirect` when any policy has TLS (`caddy_proxy.go:736-767`), and a Traefik `-redirect` middleware per policy (`traefik_proxy.go:949-956`). None share semantics (301 global vs permanent scheme redirect).
8. **TLS/cert ownership split.** NPM ties cert to host by `certificate_id`. Forge splits certs across ACME service (`handlers_certificates.go:10`), custom-cert endpoint (`handlers_proxy_domains.go:215-310`), `proxy_domains.cert_data/cert_key/cert_type/auto_renew` columns (`store_proxy_domains.go:20-23`), adapter `SetCertificate/RemoveCertificate` (`gateway_adapter.go:49-51`), and a second Caddy client `CaddyTLSManager` that duplicates validate/apply plumbing (`caddy_tls.go:240-280` vs `caddy_proxy.go:980-1052`).
9. **Health checks.** Traefik: healthCheck declared on the service LB (`traefik_proxy.go` mirrors at `TraefikHealthCheck`, `traefik_proxy.go:85-89`). Forge runs *three independent health systems*: TCP dial probes in trafficmanager keyed `ruleID|host:port` (`service.go:1001-1082`), a second dial loop in loadbalancer (`dataplane.go:296-326`), and a third failure tracker in crossnode `HealthFilter` (`crossnode/health_filter.go`, wired `main.go:993`). Thresholds and state machines differ per system.
10. **Node-offline reaction triplicated.** One event, three subscribers with different behaviors: trafficmanager withdraws/reinstates rules (`service.go:753-898`, subscribed `main.go:1026-1027`), loadbalancer flips target statuses (`service.go:552-561`, subscribed `main.go:1049-1050`), ingressSync re-syncs whole config (`main.go:1051-1069`). Traefik equivalent = one provider recomputation.
11. **Adapter interfaces proliferation.** Forge defines four overlapping adapter abstractions: `GatewayAdapter` (`gateway_adapter.go:38-64`), legacy `ReverseProxy` (`trafficmanager/service.go:101-105`), `domains.caddyUpdater` (`domains/service.go:74-76`), and servicediscovery's unrelated `NetworkAdapter` (`servicediscovery/adapter.go:8-20`). Traefik has one Provider interface producing one config type; Caddy has one config schema.
12. **Route-grouping logic duplicated per adapter.** Identical `domain|path|protocol` grouping implemented twice: `CaddyReverseProxy.groupRules` (`caddy_proxy.go:894-922`) and `TraefikReverseProxy.groupRules` (`traefik_proxy.go:725-753`), plus a third variant `crossnode.GroupRulesByRoute` (`crossnode/routegroup.go:32-68`). Grouping is a *model* concern and belongs once, above adapters (as in Traefik's aggregation step).
13. **Config validation & rollback asymmetry.** Caddy adapter validates against the real admin API then snapshot/restores (`caddy_proxy.go:673-705`); Traefik adapter only round-trips YAML marshal/unmarshal (`traefik_proxy.go:974-1005`) and restores from a file backup (`traefik_proxy.go:449-469`); NPM validates through nginx `-t` before reload. Forge's abstraction doesn't capture "validate against the real engine" as a contract strength — `GatewayAdapter.ValidateConfig` means different rigor per kind.
14. **Observability contract mismatch.** `GetActiveConnections` counts actual upstreams by walking Caddy's running config (`caddy_proxy.go:149-230`) but merely counts `.yml` files for Traefik (`traefik_proxy.go:296-311`); `Health()` is hardcoded healthy for Caddy (`caddy_proxy.go:375-377`) but probes Traefik's API (`traefik_proxy.go:561-604`). Callers cannot rely on either method across kinds.
15. **UI/API fragmentation mirrors the model fragmentation.** Admin pages: `/admin/traffic`, `/admin/load-balancer`, `/admin/domains`, `/admin/certificates`, `/admin/dns`, `/admin/firewall` (`forge/web/app/admin/*`; nav entries `admin-registry.ts:72-73`), while `proxy_domains`, `security_headers`, `redirect_rules` are API-only CRUD (`server.go:2656-2659`, comment "distinct from per-server domains" at `server.go:2655`). NPM exposes exactly one hosts page + streams page.

---

## 4. Store/schema audit — duplicate routing concepts across Forge tables

| Table | File | Role today | Overlap |
|---|---|---|---|
| `traffic_rules` | `internal/store/migrations/038_traffic_routing.sql:1-16` **and** `api/migrations/083_a_traffic_rules.sql` (two CREATE TABLEs for the same name, different columns — `target_host` only exists in 083_a) | L7 route | duplicates `proxy_domains` rows and domain-service routes |
| `traffic_policies` | `038_traffic_routing.sql:21-36` | rate-limit/IP/TLS/CB bundle | no link to any rule; overlaps `proxy_domains.rate_limit*` |
| `target_groups` / `target_group_targets` | `082_b_target_groups.sql` | L4 pools | parallel to `traffic_rules` targets and LB service state |
| `proxy_domains` | `117_domains_certificates.sql:1-33`, struct `store_proxy_domains.go:13-33` | npm-style host row incl. embedded certs/auth/rate-limit | duplicates traffic_rules + policies + certificates |
| `redirect_rules` | `117_domains_certificates.sql:35` | path redirects | overlaps built-in HTTPS redirect + Traefik redirect middleware |
| `security_headers` | `117_domains_certificates.sql:49` | HSTS etc. per domain | write-only; no adapter consumes it (grep over `services/`, `cmd/` finds zero readers) |
| dual store files | `store_traffic.go` (`TrafficRuleRow`, no TargetHost) vs `store_routing.go` (`RoutingRuleRow`, with TargetHost) | two Go mappings of the same table | `TrafficRuleRow.CreateTrafficRule` inserts without `target_host` column list (`store_traffic.go:117-127`) — schema drift risk |

---

## 5. Logic findings (≥3 required — 6 provided)

### F1 — TrafficManager full-config apply wipes domain routes (integration bug)
`CaddyReverseProxy.updateRoutesAtomic` builds a server map containing **only** `gamepanel` and POSTs it to `/config/`, which replaces the entire Caddy config (`caddy_proxy.go:707-720` build, `:697` apply via `applyConfig` `:1026-1052`). Meanwhile `UpdateDomainRoutes` deliberately fetches running config first "to avoid wiping gamepanel routes" (`caddy_proxy.go:523-569`, writes `servers["gamepanel-domains"]`). So any trafficmanager `ApplyRoutes`/`SyncRoutes` (handler `POST /admin/traffic/sync`, `handlers_trafficmanager.go:107-112`) erases the `gamepanel-domains` server block that the domains service maintains — asymmetric merge discipline between two writers sharing one config document.

### F2 — Production wiring never installs the GatewayAdapter; half the abstraction is inert
`tmSvc = trafficmanager.NewWithPersistence(db, db, db, db, caddyProxy, outboxPub)` (`cmd/api/main.go:1025`) leaves `svc.adapter == nil` (only `NewWithAdapter*` set it, `service.go:131-141`). Consequences: `ProbeTargets` early-returns (`service.go:1001-1004`), so upstream health marking never happens; `ValidateGatewayConfig`/`ReloadGateway`/`RollbackGateway` are no-ops (`service.go:693-718`); `CleanupStaleRoutes` never runs. The richer interface (`gateway_adapter.go:38-64`) exists but only the legacy 3-method `ReverseProxy` slice (`service.go:101-105`) is exercised in prod. The Traefik adapter is constructed **only from tests** — grep shows `NewTraefikReverseProxy` exclusively in `_test.go` files; prod hardcodes `NewCaddyReverseProxy` (`main.go:982`).

### F3 — Policies are not attached to rules; every policy hits every route
There is no rule↔policy association anywhere (no column/FK in `038_traffic_routing.sql`/`083_a_traffic_rules.sql`). Both adapters paper over it: Traefik's `collectApplicablePolicies` returns the input map unchanged with a TODO-style comment ("For now, return all policies...") (`traefik_proxy.go:960-972`); Caddy concatenates all policies' handles onto **every** grouped route (`buildPolicyRoutes`/`enrichRouteWithPolicy`, `caddy_proxy.go:722-747`, `:771-793`). Creating one blacklist policy silently blacklists IPs for *all* tenants' routes. Crossnode's `extractPolicyID` even returns `""` unconditionally (`crossnode/routegroup.go:149-151`), so `RouteGroup.PolicyIDs()` is always empty.

### F4 — Strategy semantics diverge per adapter (same rule, different behavior)
A rule with `strategy=least_connections`: Caddy maps non-round_robin strategy straight to `lb_policy` (`caddy_proxy.go:944-946`, `:973-975` → correct-ish); Traefik maps *any* non-round-robin strategy to a **sticky cookie** (`stickyStrategy`, `traefik_proxy.go:806-826` → `Sticky.Cookie` `:841-849`), i.e. least_connections becomes session pinning. Additionally `TraefikServer.Weight` is emitted (`traefik_proxy.go:814-817`) although current Traefik's `dynamic.Server` has no weight field on the URL server form used here (weights belong to weighted *services*, `reference/networking/traefik/pkg/config/dynamic/http_config.go:62-69`), so weights may be dropped/warn depending on version.

### F5 — TCP rules are rendered as HTTP routes by the Caddy adapter
`RoutingRule` validation accepts `protocol=tcp` (`service.go:383-387`) and the Traefik adapter correctly branches to TCP routers with `HostSNI` (`traefik_proxy.go:758-760`, `:854-905`). The Caddy adapter has **no protocol branch**: `buildGroupedRoute` matches `host`+`path` and uses `reverse_proxy` regardless (`caddy_proxy.go:924-978`). A game-server TCP rule pushed through the Caddy path becomes a broken HTTP route — the abstraction leaks engine capability differences that the model must own (cf. NPM solving this with a separate `streams` row type).

### F6 — Two migrations create the same tables; two Go row types map them
`traffic_rules`/`traffic_policies` are created twice: `internal/store/migrations/038_traffic_routing.sql:1,21` (no `target_host`, default `weight 0`) and `api/migrations/083_a_traffic_rules.sql` (with `target_host`, `weight NOT NULL DEFAULT 1`). Which one wins depends on migration ordering across two directories. Correspondingly `TrafficRuleRow` (`store_traffic.go:11-25`) lacks `TargetHost`/`WebSocket` while `RoutingRuleRow` (`store_routing.go:21-37`) has both; `CreateTrafficRule` omits `web_socket` entirely (`store_traffic.go:117-127`). The repo even carries a test guarding duplicate prefixes because of this history (`migration_duplicate_test.go:37,52,115`).

*(Bonus observation)* **F7 — GetActiveConnections lies for Traefik** (`traefik_proxy.go:296-311` returns per-file zeros), and `CleanupStale` encodes knowledge of foreign route IDs (`gamepanel-https-redirect`, `gamepanel-domain-*`) inside the Caddy adapter (`caddy_proxy.go:350`) instead of ownership being modeled.

---

## 6. Recommendation — THE Forge Gateway model

### 6.1 Adopt a Traefik-shaped core (Routers / Middlewares / Services), stored declaratively

Traefik's shape fits Forge best because Forge already needs multi-provider reality (per-node Caddy *and* optional Traefik, plus its own L4 dataplane): providers produce one canonical dynamic config (`reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46`). Concretely:

```
GatewayRouter      id, tenant, entrypoint(:80/:443/:30000-30100/tcp/udp),
                   match {host, path, protocol}, service_id, middleware_ids[],
                   tls_policy_id, priority, enabled
GatewayMiddleware  id, type(rate_limit|ip_acl|headers|hsts|redirect|forward_auth|
                   circuit_breaker|strip_path|compress), config JSONB
GatewayService     id, lb_strategy(round_robin|least_conn|ip_hash|weighted),
                   sticky{}, health_check{}, targets -> GatewayTarget[]
GatewayTarget      service_id, server_id?, node_id?, host, port, weight
```

This collapses today's seven tables: `traffic_rules`+`proxy_domains` → `gateway_routers`; `traffic_policies`+`security_headers`+`redirect_rules` → typed `gateway_middlewares` (referenced! fixes F3/Finding 7); `target_groups(_targets)` → `gateway_services/targets`; the domains service becomes a *provider* that emits routers for verified domains (exactly Traefik's provider pattern), not a second config writer.

### 6.2 Keep one adapter interface — finish `GatewayAdapter`

- Make `GatewayAdapter` (`gateway_adapter.go:38-64`) the only abstraction; delete `ReverseProxy`, `caddyUpdater`, and fold `NetworkAdapter` concerns into discovery. `domains.Service` should depend on a narrow `RoutePublisher` fed by the new provider, not on a concrete caddy-ish updater (`domains/service.go:74-76,484`).
- Add to the interface what today diverges per engine: `ActiveBackends()` semantics (fix Finding 14), `ProtocolSupport()` so TCP rules can be rejected/refused per kind (fix F5), and a real `ValidateConfig` contract (engine-side validation like Caddy `/load` with `Cache-Control: must-revalidate`, `caddy_proxy.go:980-1002`).
- Move route grouping out of adapters into the manager (one implementation replacing `caddy_proxy.go:894-922`, `traefik_proxy.go:725-753`, `crossnode/routegroup.go:32-68`).

### 6.3 Single reconciler (desired-state loop)

One owner computes desired config and calls adapters — Traefik's watcher/NPM's renderer pattern. Today three writers race on one Caddy (F1). Merge `IngressSynchronizer.Sync` (`crossnode/ingress_sync.go:111-198`), trafficmanager `ApplyRoutes/SyncRoutes` (`service.go:562-618` — themselves near-duplicates), and `domains.syncCaddyRoutes` into one reconcile that diffs desired vs reported state. Node-offline handling stays event-driven but through the single reconciler (replacing triple subscription at `main.go:1026-1027,1049-1069`).

### 6.4 Discovery integration

Keep `servicediscovery` as the address oracle but expose it behind the existing `NodeResolver` seam — unify `trafficmanager.NodeResolver` (`gateway_adapter.go:10-12`) with `domains.NodeResolver` (`domains/service.go:87-89`) and back both with `crossnode.Resolver.ResolveTargetHost` (`resolver.go:59-83`), which already layers discovery → store → localhost fallback with caching. Health-check state should flow *into* `GatewayTarget.status` from one checker (§3 item 9), and adapters receive final target lists (as they do today via `resolveTargets`, `service.go:620-643`).

### 6.5 Migration steps (order matters)

1. Consolidate `traffic_rules` DDL to the 083_a shape; drop `store_traffic.go` row mapping (fixes F6) — pure refactor, no behavior change.
2. Introduce rule↔middleware join; stop applying all policies globally (fixes F3) — biggest correctness win.
3. Fix Caddy full-config replace to merge `gamepanel-domains` like `UpdateDomainRoutes` does, or route everything through one writer (fixes F1 immediately even pre-model-change).
4. Wire `NewWithAdapterAndPersistence` in `main.go` (or strip the unused adapter path) so health probes/rollback actually run (fixes F2).
5. Extract grouping; add `protocol` branch or rejection to the Caddy adapter (fixes F5).
6. Migrate UI: one "Gateways" section with Routers/Services/Middlewares tabs replaces `/admin/traffic` + `/admin/load-balancer` split; `proxy_domains` pages finally get a home or get deleted with their table.

---

## 7. Evidence index (key citations)

- Adapter interface: `forge/api/internal/services/trafficmanager/gateway_adapter.go:38-64`; kinds `:14-19`
- Legacy proxy iface: `trafficmanager/service.go:101-105`; nil-adapter defaults `:675-682`; probe guard `:1001-1004`
- Caddy writer: `caddy_proxy.go:44-49,673-705,707-720,722-793,894-978,149-204,295-373,375-377,523-593`
- Traefik writer: `traefik_proxy.go:243-271,725-753,755-852,854-905,907-972,974-1005,296-311,561-604`
- Second Caddy client: `caddy_tls.go:18-32,240-280`
- Domains writer: `domains/service.go:74-76,87-89,455-489`
- Crossnode third writer: `crossnode/ingress_sync.go:111-198`; grouping `routegroup.go:32-68,149-151`
- L4 dataplane: `loadbalancer/dataplane.go:20-49,115-178,187-270`; strategies `loadbalancer/service.go:416-427`
- Wiring: `cmd/api/main.go:982,993-994,998,1025-1027,1049-1069`
- Handlers: `handlers_trafficmanager.go:8-113`; `handlers_loadbalancer.go:24-138`; `handlers_proxy_domains.go:9-213,215-310,312-360,362-412`; registration `http/server.go:2593-2598,2656-2663`
- Stores: `store_routing.go:12-37,41-98,216-237`; `store_traffic.go:11-42,117-127,215-229`; `store_proxy_domains.go:13-71`; `store_redirect_rules.go:42`; `store_security_headers.go:45`
- Migrations: `internal/store/migrations/038_traffic_routing.sql:1-38`; `api/migrations/082_b_target_groups.sql`; `api/migrations/083_a_traffic_rules.sql`; `api/migrations/117_domains_certificates.sql:1,35,49`
- References: traefik `pkg/config/dynamic/http_config.go:38-99,62-69,470-478` + `AGENTS.md` vocabulary; caddy `modules/caddyhttp/routes.go:31-41`; npm `backend/schema/components/proxy-host-object.json:4-25`, `backend/schema/paths/nginx/` layout
