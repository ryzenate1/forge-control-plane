# Phase 04 Synthesis — Networking Cluster (Caddy · Traefik · Nginx-Proxy-Manager)

**Cluster:** caddy (master), traefik (master), nginx-proxy-manager (develop)
**Why this cluster now:** Phase 1 exposed Forge's gateway surface scattered across 7 pages/6 services; Phase 3 showed cert/artifact storage patterns. This cluster is the definitive gateway test: three references each have exactly ONE config authority (Traefik dynamic-config tree, Caddy single JSON doc, NPM proxy-host rows) while Forge has **five overlapping gateways** writing to one Caddy instance. Findings here are the most load-bearing of the audit — multiple P0s where enabling a feature bricks route updates or wipes tenant routes.

**Evidence:** 5 subagent reports — reverse-proxy/routing/LB (18 comparisons), TLS/ACME (17), dynamic-config/discovery/UX (18), Forge-gateway joint-compare (15), network security/reliability/observability (15) — all file:line verified against both trees.

---

## 1. Reference models (what "one gateway" looks like)

| Model | Authority | Shape |
|-------|-----------|-------|
| **Traefik** | One `*dynamic.Configuration` tree | `HTTPConfiguration{Routers→Services→Middlewares}` (`pkg/config/dynamic/http_config.go:38-46`); routers reference named middlewares by key (`:88`); LB strategy belongs to Service (`Weighted/Failover/Mirroring` `:62-69`); providers merely produce this shape; API is **read-only** GET (`pkg/api/handler.go:103-133`); file provider self-heats via fsnotify (`provider/file/file.go:90,170-207`); ordered middleware chain with recursion check (`pkg/server/middleware/middlewares.go:50-83`) |
| **Caddy** | Single JSON doc via admin API | `/load` replaces whole config (`caddyconfig/load.go:68-72`); `/adapt` is the TRUE dry-run (`load.go:137-175`); targeted ops under `/id/<id>` + sub-resource paths (`admin.go:1091-1099`); strict SNI (`modules/caddyhttp/server.go:663-678`); trusted_proxies CIDR-gated XFF (`server.go:1171-1236`); passive health cooldowns (`reverseproxy/healthchecks.go:233-268`) |
| **NPM** | DB rows → nginx files | Denormalized `proxy-hosts` row (`schema/components/proxy-host-object.json:4-25`); every CRUD ends configure→test→reload with `.err` rename + meta offline on failure (`backend/internal/nginx.js:27-117`); per-host access lists; audit log on every mutation (`audit-log.js:84-103`); certbot worker (`certificate.js:22-36`) |

## 2. Forge fragmentation map

Five concurrent gateway surfaces:
1. **TrafficManager** (L7 rules+policies → own Caddy server `gamepanel`) — `services/trafficmanager/service.go:24`, wired `main.go:1025`
2. **LoadBalancer** (own L4 TCP/UDP dataplane bypassing gateways) — `loadbalancer/dataplane.go:20-49`
3. **Domains service** (writes second Caddy server `gamepanel-domains`) — `domains/service.go:455-489`
4. **Crossnode IngressSynchronizer** (third writer, 30s ticker, never populated) — `ingress_sync.go:111-198`, `main.go:994`
5. **ProxyDomains+RedirectRules+SecurityHeaders** (npm-style rows, no renderer) — `store_proxy_domains.go`

Plus four adapter abstractions (`GatewayAdapter`, legacy `ReverseProxy`, `domains.caddyUpdater`, servicediscovery `NetworkAdapter`) and a dead 1,139-line Traefik adapter constructed only from tests (`main.go:982` hardcodes Caddy). Schema duplication: `traffic_rules` created by TWO migrations with divergent columns (`internal/store/migrations/038_traffic_routing.sql:1` vs `api/migrations/083_a_traffic_rules.sql`), mapped by two Go row types (`store_traffic.go:11` vs `store_routing.go:21`). Seven tables encode what Traefik expresses in three.

## 3. Hidden / unwired / dead

| Surface | State |
|---------|-------|
| `GatewayAdapter.SetCertificate/RemoveCertificate` | **Zero callers** — issued/renewed ACME certs never leave Postgres (LF-2 Critical). TLS works only via separate embedded-Caddy path |
| `acme.HTTPSolver()` HTTP-01 challenger | Implemented (`acme/service.go:52-92`) but **never mounted** — HTTP-01 (the DEFAULT challenge type!) times out against LE |
| Traefik adapter (1,139 lines) | Never constructed in prod; doubly broken anyway (nonexistent `/api/refresh`; PEM bodies in path fields) |
| `CaddyTLSManager` (`caddy_tls.go`) | Never constructed; proxy-domain custom certs never reach any gateway; cert upload UI posts to unregistered route (405) |
| servicediscovery REST + `PrivateNetworkPolicy` port ACLs | Complete API, zero UI consumers; ACL policy test-only |
| crossnode `HealthFilter.RecordSuccess/RecordFailure` | Zero production call sites — filter is pass-through |
| crossnode `IngressSynchronizer.SetRules/UpsertRule` | No non-test callers — sync pushes EMPTY route set every 30s (F-NET-01) |
| `security_headers` table | Write-only; no adapter consumes it |
| Per-domain header editor (`lib/api/security.ts:37-64`) | Links to nonexistent `/admin/domains/:id` |

## 4. Broken / incorrect logic (consolidated)

### P0 — traffic-affecting correctness
- **F-NET-01 Empty-rule ingress sync wipes whole gateway config every 30s.** Nothing populates IngressSynchronizer; Sync() unconditionally calls UpdateRoutes → bare `gamepanel` server POSTed to `/config/` (total replacement) destroying domains block repeatedly (`ingress_sync.go:111-166` + `caddy_proxy.go:673-720,1026-1052`).
- **F-NET-02 Asymmetric merge discipline between writers.** UpdateDomainRoutes fetch-modify-writes preserving others (`caddy_proxy.go:523-593`) but updateRoutesAtomic/buildTLSConfig full-replace with ONLY their own server (`caddy_proxy.go:707-720`; `caddy_tls.go:186-238`). Traffic/TLS apply erases domain routes; two servers share :80 → SO_REUSEPORT nondeterminism.
- **F-NET-03 "Validate = apply", snapshot-after-mutation.** validateConfig POSTs candidate to `/load` (an apply!), THEN snapshots — rollback restores the NEW config not prior (`caddy_proxy.go:980-1002,691-702`; caddy truth: `/load`=apply `load.go:68-72` vs true dry-run `/adapt` `load.go:137-175`).
- **F-NET-04 Fictional Caddy modules brick route updates when policies enabled.** `"handler":"rate_limit"`/`"handler":"circuit_breaker"` are not real Caddy modules (`reverseproxy.go:109` shows CB is reverse_proxy namespace only). must-revalidate validation fails → EVERY UpdateRoutes/SyncRoutes errors once any policy exists. Tests assert broken rendering.
- **F-NET-05 All policies apply to all routes (cross-tenant leakage).** No rule↔policy join; collectApplicablePolicies returns everything (`traefik_proxy.go:960-972`); Caddy concatenates all handles onto every grouped route (`caddy_proxy.go:722-793`); extractPolicyID returns "" always (`routegroup.go:149-151`).
- **F-NET-06 Grouped-route removal misses dead nodes.** Withdrawal targets per-rule IDs but grouped routes keyed `<firstRule>-group` — failing node keeps receiving traffic unless its rule is first (`caddy_proxy.go:60-74,949-951`).
- **F-NET-07 TCP rules render as broken HTTP/SNI-only routes; UDP half-implemented.** No protocol branch in Caddy adapter; Traefik TCP router uses HostSNI (TLS-SNI-only); UDP rejected at admission yet rendered under Traefik `tcp:` section stock Traefik ignores.
- **F-NET-08 Certificates never delivered to data plane.** Renewal updates only Postgres; contrast traefik hot-reload post-renewal (`provider.go:800-802`), npm reload (`certificate.js:155-186`).
- **F-NET-09 HTTP-01 dead code.** Default challenge type cannot succeed; DNS-01 only working path.
- **F-NET-10 Networking admin UX non-functional.** Traffic page schema mismatch (posts `{path,targetGroup,priority,methods}` vs backend `{domain,path,targetPort,...}`), policies list 405, PATCH-vs-PUT 405, cert upload 405.

### P1 — major functional/reliability gaps
- **F-NET-11 Unhealthy-upstream add/remove oscillation.** ProbeTargets removes dead upstreams; every full rebuild re-adds them ignoring health maps — permanent flap during outages.
- **F-NET-12 LB HealthCheckConfig stored but never executed.** Fixed raw TCP dial every 2s regardless of configured Path/Interval/Thresholds; single-failure flip-flop; operator-set Unhealthy force-reverted each tick; UDP skipped.
- **F-NET-13 Weighted routing silently ignored on Caddy** (weight not an Upstream field; belongs in lb_policy weighted_round_robin); weights lost on health re-add both adapters; WRR counter shared across all groups (`__weighted`).
- **F-NET-14 Strategy mistranslation.** least_connections→sticky-cookie on Traefik; name matches no Caddy policy.
- **F-NET-15 Traefik router-rule injection.** Unescaped backticks in rule Path interpolate into rule expressions — admin can hijack other domains' matching (`traefik_proxy.go:797`).
- **F-NET-16 DNS-provider SSRF.** User-supplied PDNS_API_URL/RFC2136_NAMESERVER zero scheme/IP validation; env mutation observable concurrently (`dns/service.go:489-498,639-661`).
- **F-NET-17 Permanent leaf-key reuse on renewal** (`renewOnce` passes stored PrivateKey forever — anti-pattern per certmagic `automation.go:144-151`); account decoupled from issuing identity; registration URI never persisted.
- **F-NET-18 Plaintext DNS API credentials** in `certificates.dns_credentials`+`dns_provider_accounts.credentials` while siblings encrypted in migration 157.
- **F-NET-19 Firewall ephemeral daemon pass-through.** Verbatim bodies, nothing persisted, no reconciler/drift detection — node reprovision silently loses rules; zero input validation.
- **F-NET-20 Reachability checks measure wrong vantage point.** Dial from API server not source node — overlay paths never exercised (`reachability.go:58-79`).
- **F-NET-21 Fail-open resolution.** Resolve returns unhealthy endpoints when none healthy; reaper marks stale but never removes (`discovery.go:60-69`, `stale_reaper.go:113-117`).
- **F-NET-22 Observability fiction.** Caddy Health hardcoded healthy (`caddy_proxy.go:375-377`); GetActiveConnections counts upstreams/files not connections; two client-IP notions (logs raw c.IP vs rate-limit ExtractClientIP); CSP nonce falls back to constant "fallback-nonce" with strict-dynamic on rand failure (`middleware_security.go:19-25`).
- **F-NET-23 Trusted-proxy fail-open.** ExtractClientIP trusts XFF whenever direct peer is private (vs Caddy CIDR allowlist); mTLS HTTPS gate trusts client-supplied X-Forwarded-Proto (`middleware_mtls.go:117-125`); RATE_LIMIT_TRUSTED_IPS full bypass.
- **F-NET-24 ACME revoke = local delete only** (`acme/service.go:368-370`) — cert stays valid at CA; admin@localhost default contact rejected by CAs (`service.go:198`); auto-renewal goroutine panic kills loop permanently (recover outside for-loop, `service.go:389-396`).
- **F-NET-25 L4 LB as internal-network probe primitive.** AddTarget accepts arbitrary IP verbatim — status API reveals which internal IP:port pairs accept connections; listeners relay raw bytes both ways (`handlers_loadbalancer.go:88-100`).

## 5. Duplicates

- Three route-group generators with divergent normalization (`caddy_proxy.go:894-978`, `traefik_proxy.go:725-753`, `crossnode/routegroup.go:32-97`)
- Redirects triplicated: redirect_rules table + built-in HTTPS redirect route (`caddy_proxy.go:736-767`) + per-policy Traefik redirect middleware (`traefik_proxy.go:949-956`)
- Three health systems (trafficmanager probes, LB dataplane loop, crossnode HealthFilter) with different thresholds/state machines
- Policy embedded per-row (proxy_domains.rate_limit*) AND policy-as-table (traffic_policies) coexisting, neither feeding the other
- Two security-header middlewares that must be kept in sync manually (`middleware_security.go:27-62` vs `middleware_security_headers.go:48-93`)

## 6. False completion / decorative

- Proxy-domain verify endpoint returns hardcoded `"verified": true` without checks (`handlers_proxy_domains.go:199-212`)
- Security page hardcodes HSTS/CSP/X-Frame pills per row regardless of actual config (`AdminSecurity.tsx:114-133`)
- Adapter health fabricated for Caddy deployments; connection counts fictional
- Cert upload modal appears functional but 405s; Traefik SetCertificate reports success without serving anything

## 7. Architecture lessons

| Reference pattern | Forge today | ADOPT / ADAPT / REJECT |
|---|---|---|
| Traefik provider→router→middleware→service declarative model | 5 writers, 7 tables, 4 adapters | **ADOPT as THE Gateway model**: GatewayRouter/Middleware/Service tables; domains becomes a *provider* emitting routers; collapse traffic_rules+proxy_domains→routers, traffic_policies+security_headers+redirect_rules→typed middlewares (referenced!), target_groups→services/targets |
| One reconciler owning desired-state convergence | 3 event subscriptions + manual sync + 2min loop | **ADOPT** single reconcile diffing desired vs reported state |
| True dry-run before apply (Caddy /adapt; nginx -t) | validate=apply then snapshot-after | **ADOPT** snapshot BEFORE mutation; use sub-resource merges or read-modify-write like UpdateDomainRoutes does |
| Named middleware references per-router | all-policies-everywhere | **ADOPT** rule↔middleware join; deterministic order (rate-limit→blacklist→whitelist→CB→redirect) |
| CIDR-gated trusted proxies (Caddy) | peer-private-IP trust + XFP acceptance | **ADAPT** explicit TRUSTED_PROXIES config; drop raw XFP in mTLS gate |
| Passive health inside proxy process (Caddy/Traefik) | out-of-band probes fighting rebuilders | **ADAPT** single owner for upstream health; builders must consult health state |
| NPM one-page host editor | 5 broken/split pages | **ADAPT** single Gateways section with Routers/Services/Middlewares/Certs tabs |
| Audit log on every networking mutation (NPM) | none on traffic/cert/LB/firewall | **ADOPT** |

## 8. Recommended activation order

1. **P0 Stop the bleeding:** skip empty ingress syncs; make updateRoutesAtomic merge like UpdateDomainRoutes (or route ALL writes through ONE writer); snapshot BEFORE validate (use /adapt dry-run semantics); never emit rate_limit/circuit_breaker handlers until real modules exist (gate features explicitly).
2. **P0 Wire the dead cert path:** mount HTTPSolver on :80 (or proxy /.well-known/acme-challenge from gateway config); call SetCertificate after issue/renew; construct a TLS manager so uploads reach gateways; rewire cert upload UI to the registered route; redact cert_key from list responses.
3. **P0 Fix policy attachment:** add rule↔policy FK/join; filter policies per route; deterministic middleware ordering; reject backticks/control chars in Path (or emit matchers structurally).
4. **P1 Rebuild networking admin UX against real schemas:** traffic page (RoutingRule fields), GET /policies list, PATCH verb, fix LB Test Next, delete-or-implement /admin/domains/:id.
5. **P1 Single health owner:** builders consult adapter health maps; implement HTTP prober honoring HealthCheckConfig; hysteresis thresholds; respect operator statuses; group-aware withdrawal (rebuild-minus-withdrawn).
6. **P1 Encrypt DNS credentials** (mirror migration 157); validate DNS-provider URLs (scheme+IP allowlist); fresh key per renewal + persist account binding + preferred chain.
7. **P1 Persist firewall desired state** in DB with node reconciliation + payload validation.
8. **P2 Unify client-IP derivation** (logs vs limits); real adapter health probe for Caddy; honest metric names; per-request CSP nonce abort-on-failure; CIDR-based trusted proxies; reject private/link-local LB target IPs unless allowed.
9. **P3 Decide Traefik adapter fate:** fix reload (file-watch model, drop /api/refresh), write real cert paths, wire into prod behind config flag — else delete the 1,139 lines.
10. **P3 Consolidate schema:** adopt 083_a traffic_rules shape, drop store_traffic.go mapping, unify migrations.

## 9. Handoff to Phase 05

Phase 1 established queue/placement debt; Phase 4 shows what happens when desired-state has multiple writers. Phase 5 (Nomad/Incus/River/NetBird/Longhorn/Rancher vs placement/scheduler/runtime/clustering/queue) should treat F-NET-01/02's single-writer lesson as axiomatic when evaluating Nomad's eval-broker and River's tx-bound enqueue, and should verify whether Forge's queue/operation duality produces the same last-writer-wins drift seen here.
