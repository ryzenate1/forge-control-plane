# Subagent 08 — Gateway / Networking Confirm (Phase 02 Agent 08/10)

**Focus:** Confirm Networking/Gateway implementations — densest P0 cluster (10 P0s)  
**Task:** Compare `audits/phase-04/synthesis.md` 10 P0s F-NET-01..10 + `audits/final-parity/subagent-07-networking-gateway.md` 18 rows + `audits/reverification/subagent-14-networking-gateway.md` 18 rows against LIVE  
**Snapshot:** 2026-08-23 — no product code modified, file:line pinned  
**Method:** Direct read of LIVE `forge/api/internal/services/{trafficmanager/caddy_proxy.go,caddy_tls.go,traefik_proxy.go,gateway_adapter.go,crossnode/ingress_sync.go,acme/service.go}` vs `reference/networking/caddy/{caddyconfig/load.go,admin.go,modules/caddyhttp/reverseproxy/*}` vs `reference/networking/traefik/{pkg/config/dynamic/http_config.go,pkg/api/handler.go,pkg/provider/file/file.go}`

---

## 0. Densest P0 cluster — still #1, still 100% open

> **Confirmed: all 10 P0s F-NET-01..10 from phase-04 synthesis §4 are STILL BROKEN on live inspection. 0/10 fixed, 0/10 partially fixed beyond cosmetic recovery-speed. This is the densest P0 cluster in the entire audit and mutually amplifies — reverification-14 reported 100% still-open and live matches exactly.**

| # | Finding | Short title | Live verdict | Blast radius |
|---|---------|-------------|--------------|--------------|
| **F-NET-01** | `forge/api/internal/services/crossnode/ingress_sync.go:111` `Sync()` + `main.go:994` 30s ticker, zero non-test `SetRules` callers | Empty-sync wipes whole Caddy every 30s | **STILL BROKEN** | Periodic total loss of `gamepanel-domains` server + TLS |
| **F-NET-02** | `forge/api/internal/services/trafficmanager/caddy_proxy.go:673-720` vs `caddy_proxy.go:523-593` + `caddy_tls.go:186-238` | Asymmetric merge — traffic/TLS full-replace while domains merge | **STILL BROKEN** | Last-writer-wins drift |
| **F-NET-03** | `forge/api/internal/services/trafficmanager/caddy_proxy.go:980-1002` + `:691-695` | `validate = apply` + snapshot-after-mutation | **STILL BROKEN** | Rollback restores bad gen; double-apply |
| **F-NET-04** | `forge/api/internal/services/trafficmanager/caddy_proxy.go:795-875` (`:798-804` `rate_limit`, `:858-872` `circuit_breaker`) | Fictional Caddy handlers | **STILL BROKEN** | Any policy bricks every future `UpdateRoutes` |
| **F-NET-05** | `forge/api/internal/services/trafficmanager/traefik_proxy.go:960-972` + `caddy_proxy.go:722-793` + `crossnode/routegroup.go:149-151` | All-policies-on-all-routes | **STILL BROKEN** | Cross-tenant leakage |
| **F-NET-06** | `forge/api/internal/services/trafficmanager/caddy_proxy.go:60-74` vs `:949-951` | Grouped-route miss on withdraw | **STILL BROKEN** | Dead node keeps receiving traffic |
| **F-NET-07** | `forge/api/internal/services/trafficmanager/caddy_proxy.go:924-978` + `service.go:383-387` | TCP→HTTP mis-render; UDP half-implemented | **STILL BROKEN** | L4 rules never match |
| **F-NET-08** | `forge/api/internal/services/trafficmanager/gateway_adapter.go:38-64` `:49` + `acme/service.go:355-363,420-438` | Certs never leave Postgres (zero `SetCertificate` callers) | **STILL BROKEN** | ACME succeeds → no TLS served |
| **F-NET-09** | `forge/api/internal/services/acme/service.go:52-92,157-159` → `service.go:203-205` default HTTP-01 | `HTTPSolver` never mounted | **STILL BROKEN** | Default challenge always times out |
| **F-NET-10** | `forge/web/app/admin/traffic/page.tsx:14-22,67-70,84,94` vs `forge/api/internal/http/handlers_trafficmanager.go:42,80-112` | Admin UX schema/verb mismatch (405) | **STILL BROKEN** | Traffic page 100% failure |

Root cause unchanged: **five concurrent writers on one embedded Caddy (`main.go:982,994-995,998,1025`), no single config authority, no true dry-run, two dead adapter paths (`CaddyTLSManager` never constructed, `TraefikReverseProxy` 1,139 lines never constructed in prod), one fabricated validation — verified at `forge/api/cmd/api/main.go:982` `NewCaddyReverseProxy`, `:994-995` `NewIngressSynchronizer(caddyProxy…)` + `Start(30*time.Second)`, `:998` `domains.New(...caddyProxy…)`, `:1025` `trafficmanager.NewWithPersistence(...,caddyProxy,…)`.**

Reconciliation: `audits/final-parity/subagent-07` §0 boxed the same 10 P0s + 18 rows; `audits/reverification/subagent-14` reproduced 18 rows and declared 10/10 still broken (synthesis §0). Live inspection reproduces identical wiring and identical line ranges — no fix commit landed between those snapshots and this one.

---

## 1. Per-P0 live line proof (10/10 STILL BROKEN)

### F-NET-01 — Empty-rule IngressSynchronizer wipes whole config every 30s — STILL BROKEN

**Live:**
- `forge/api/internal/services/crossnode/ingress_sync.go:111-166` `Sync()` — unconditionally builds `mergedRules` from `is.rules` and calls `is.adapter.UpdateRoutes(ctx, mergedRules, policies)` at `:166` with NO `if len(rules)==0 { return nil }` guard
- `ingress_sync.go:125-145` `groups := GroupRulesByRoute(rules)` — zero rules → 0 groups → `mergedRules=nil`
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:673-705` `updateRoutesAtomic` builds `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80",":443"],"routes":[]}}}}}` at `:707-720` and POSTs to `/config/` via `:697` `applyConfig` / `:1026-1052` which replaces entire config (`reference/caddy/caddy.go:115-142`, `reference/caddy/caddyconfig/load.go:68-72` "replaces the entire current configuration")
- Populators: `ingress_sync.go:200-228` `SetRules/SetPolicies/UpsertRule/RemoveRule` have **zero non-test callers** — `grep -rn SetRules forge/ --include=*.go | grep -v _test.go` returns only the definitions at `crossnode/ingress_sync.go:200,218` (verified live). `main.go:994-995` wires the synchronizer but never calls `SetRules`
- Ticker: `forge/api/cmd/api/main.go:993-995` `ingressSync = crossnode.NewIngressSynchronizer(caddyProxy, crossNodeResolver, healthFilter, outboxPub)` + `ingressSync.Start(appCtx, 30*time.Second)` — plus 3 event subscriptions `:1043-1063` calling `ingressSync.Sync` on `Online/Offline/Recovered`, multiplying wipes beyond 30s
- Consequence: `gamepanel-domains` server written only by the careful merge path `caddy_proxy.go:523-569` (`UpdateDomainRoutes` fetches running config and preserves `gamepanel`) is destroyed every 30s until next `domainSvc.SyncCaddyRoutes` re-merges

**Partial fix?** **No fix.** One mitigation added since phase-04: `main.go:1029-1040` now does a startup `domainSvc.SyncCaddyRoutes` in a goroutine after restart — reduces *recovery time* after a wipe but does not prevent the wipe; interval and empty-sync behavior unchanged. Note `ingress_sync.go:55-99` added `Start()` panic recovery, irrelevant to empty-sync logic.

**Reference violated:** Caddy single-doc authority (`reference/caddy/admin.go:1091-1099` sub-resource `/config/apps/http/servers/<name>`) vs five writers; Traefik provider-convergence atomic snapshot (`reference/traefik/pkg/provider/docker/config.go:build`).

---

### F-NET-02 — Asymmetric merge discipline between writers — STILL BROKEN

**Live:**
- **Careful writer:** `forge/api/internal/services/trafficmanager/caddy_proxy.go:514-593` `UpdateDomainRoutes` — `getRunningConfig → json.Unmarshal → merge gamepanel-domains → json.Marshal → validateConfig → snapshot previousConfig → applyConfig` at `:523-593` (comment `:523` "to avoid wiping gamepanel routes")
- **Destructive writer:** `caddy_proxy.go:673-720` `updateRoutesAtomic → buildServerConfig` at `:707-720` returns ONLY `gamepanel` server: `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80",":443"],"routes":…}}}}}` — no `gamepanel-domains` preservation; `applyConfig` POSTs to `/config/` at `:1026-1052` which full-replaces
- **Third destructive writer:** `forge/api/internal/services/trafficmanager/caddy_tls.go:186-238` `buildTLSConfig` also builds single-server `gamepanel-domains:443` + `tls.automation` and `applyConfig` full-replaces at `:253-280` — wipes `gamepanel` routes
- **Shared port:** both servers `listen :80` (`caddy_proxy.go:567,713`, `caddy_tls.go:192`) → `SO_REUSEPORT` nondeterminism (`reference/caddy/listeners.go:115-121`)
- **No unified read-modify-write:** `grep -n "buildServerConfig\|buildTLSConfig\|buildDomainConfig" forge/api/internal/services/trafficmanager/*.go` still shows three independent builders

**Partial fix?** None. `UpdateDomainRoutes` remains the only merge-aware path — `updateRoutesAtomic` and `CaddyTLSManager.buildTLSConfig` unchanged.

---

### F-NET-03 — `validate = apply`, snapshot-after-mutation (rollback fictitious) — STILL BROKEN

**Live:**
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:980-1002` `validateConfig`:
  ```go
  req, _ := newCaddyAdminRequest(ctx, "POST", addr, "/load", bytes.NewReader(configJSON))
  req.Header.Set("Cache-Control", "must-revalidate") // :987-988
  resp, _ := p.client.Do(req) // :990
  ```
  Per `reference/networking/caddy/caddyconfig/load.go:68-72,116` `handleLoad` "replaces the entire current configuration" via `caddy.Load(body, forceReload)` (`load.go:116`, `caddy.go:115-142`) — `/load` **is** apply, not dry-run. `Cache-Control: must-revalidate` forces reload even when identical; it does not make it a dry-run.
- **True dry-run is `/adapt`** at `reference/networking/caddy/caddyconfig/load.go:137-175` `handleAdapt` returns `{result,warnings}` without mutating — **zero calls** to `/adapt` anywhere in `forge/api` (`grep -rn "/adapt" forge/ --include=*.go` empty, verified live)
- Sequence in `updateRoutesAtomic` at `caddy_proxy.go:673-705`: `validateConfig(:687)` → `previousConfig=getRunningConfig(:691-693)` → `p.lastValidConfig=previousConfig(:695)` → `applyConfig(:697)` → `restoreConfig(previousConfig)(:698-699)` on failure. Snapshot is **after** mutation, so `lastValidConfig` and `restoreConfig` both capture the NEW bad generation; `Rollback()` at `:279-293` restores that generation too. Normal case is silent double-apply.
- TLS twin: `caddy_tls.go:240-251` `validateConfig` only does `json.Unmarshal` — no gateway round-trip at all
- Traefik twin: `traefik_proxy.go:974-1005` `validateYAMLConfig` is YAML marshal round-trip + empty-rule checks; `reloadTraefik` at `:1065-1084` POSTs nonexistent `POST /api/refresh` (API is GET-only per `reference/networking/traefik/pkg/api/handler.go:103-133`; file provider self-heats via `provider/file/file.go:90,170-207` fsnotify) → every write that passed YAML still fails at reload and reverts via stale file

**Partial fix?** None. No `/adapt` usage; snapshot order unchanged; `validateConfig` still targets `/load`.

---

### F-NET-04 — Fictional Caddy modules `rate_limit` / `circuit_breaker` brick route updates — STILL BROKEN

**Live:**
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:795-875` `buildPolicyHandles`:
  - `:798-804` `{"handler":"rate_limit","rate":"N/s","burst":M}` — not in `reference/networking/caddy/modules/caddyhttp/*` registry (real namespaces: `encode`, `fileserver`, `headers`, `reverseproxy`, `map`, `rewrite`, etc.; CB lives only inside `reverseproxy` at `reference/caddy/modules/caddyhttp/reverseproxy/reverseproxy.go:109` `namespace=http.reverse_proxy.circuit_breakers`)
  - `:858-872` `{"handler":"circuit_breaker","max_failures":N,"timeout":"Ns"}` — same, no such handler module
- Consequence: `validateConfig` via `/load` (§F-NET-03) fails on unknown handler → `updateRoutesAtomic` at `:673-705` returns error → **every** `UpdateRoutes/SyncRoutes` that carries any policy aborts. Enabling the feature bricks the gateway.
- Traefik side is valid but diverges: `traefik_proxy.go:907-958` builds real `RateLimit/IPWhiteList/CircuitBreaker` middlewares, but same `TrafficPolicy` field becomes two different semantics — Caddy `max_failures` count vs Traefik `Expression: NetworkErrorRatio() > 1/threshold` at `:937-946`
- Tests assert broken rendering: `forge/api/internal/services/trafficmanager/caddy_proxy_test.go:280-314` (rate_limit), `:547-615` (circuit_breaker) — tests pass against the wrong contract

**Partial fix?** None. Handlers still emitted; no feature gate; no `xcaddy` build providing those modules.

---

### F-NET-05 — All policies apply to all routes (cross-tenant leakage) — STILL BROKEN

**Live:**
- `forge/api/internal/services/trafficmanager/traefik_proxy.go:960-972`:
  ```go
  func (p *TraefikReverseProxy) collectApplicablePolicies(...) map[string]*TrafficPolicy {
    // For now, return all policies since there is no direct rule-to-policy mapping
    result := make(map[string]*TrafficPolicy, len(policies))
    for k, v := range policies { result[k]=v }
    return result
  }
  ```
  Comment at `:964-966` is verbatim on live.
- `forge/api/internal/services/trafficmanager/caddy_proxy.go:722-793` `buildPolicyRoutes` concatenates **all** `policyHandles` onto every route via `enrichRouteWithPolicy(:771-793)` (prepends handles to every grouped route at `:742`)
- `forge/api/internal/services/crossnode/routegroup.go:149-151` `extractPolicyID()=""` always:
  ```go
  func extractPolicyID(...) string { return "" }
  ```
  So `RouteGroup.PolicyIDs()` is always empty; no per-route filtering possible
- `traefik_proxy.go:769-787` iterates `map[string]*TrafficPolicy` → **non-deterministic middleware order** per sync (Traefik order matters per `reference/traefik/pkg/server/middleware/middlewares.go:50-83` `BuildMiddlewareChain` ordered `Middlewares[]string`, recursion-checked)
- Data model: `traffic_rules` / `traffic_policies` have no FK/join (`migrations/038_traffic_routing.sql:1-38` vs `api/migrations/083_a_traffic_rules.sql`); `proxy_domains.rate_limit*` embeds policy per-row (`store_proxy_domains.go:28-30`) while `traffic_policies` carries same knobs — neither feeds the other
- `RoutingRule.Headers` (`trafficmanager/service.go:35`) persisted but never rendered in `buildGroupedRoute(:924-978)` or `traefik_proxy.go:755-852`

**Partial fix?** None. No `rule↔policy` FK added; `extractPolicyID` still returns `""`; map iteration still non-deterministic.

---

### F-NET-06 — Grouped-route removal misses dead nodes — STILL BROKEN

**Live:**
- Write: `forge/api/internal/services/trafficmanager/caddy_proxy.go:924-978` `buildGroupedRoute` creates **one** Caddy route per `domain|path|protocol` group at `:949-951`:
  ```go
  groupID := grp.rules[0].ID + "-group"
  route := map[string]any{"@id": "gamepanel-" + groupID, ...}
  ```
- Withdraw: `caddy_proxy.go:60-74` `RemoveRoutes` DELETEs per-rule IDs:
  ```go
  for _, id := range ruleIDs {
    req, _ := newCaddyAdminRequest(ctx, "DELETE", addr, fmt.Sprintf("/id/gamepanel-%s", id), nil)
  }
  ```
  Grouped ID `gamepanel-<firstID>-group` ≠ `gamepanel-<ruleID>` for non-first rules → stale route survives
- `service.go:753-817` `WithdrawNodeTargets` enumerates ruleIDs per server — same miss
- `crossnode/HealthFilter` path: `ingress_sync.go:128-145` mutates `primary.TargetHost/Port` pointers without deep-copy — data race; replica IDs compound (`primary.ID+"-replica-N"` where primary may already be a replica)
- **Evidence mismatch is known:** `caddy_proxy.go:295-373` `CleanupStale` handles `-group` correctly at `:355-357`:
  ```go
  ruleID := strings.TrimPrefix(rid, "gamepanel-")
  if strings.HasSuffix(ruleID, "-group") { ruleID = strings.TrimSuffix(ruleID, "-group") }
  ```
  But `RemoveRoutes` does NOT — hot withdraw vs lazy cleanup diverge. `traefik_proxy.go:273-293` similarly deletes per-rule `gamepanel-<id>` routers, missing grouped `gamepanel-<first>` (:763-764)
- Mutations: `traefik_proxy.go:503-508` / `:652-655` recovery append is inverted (`if !modified` → append), growing pool per flap

**Partial fix?** None for hot path. `CleanupStale` correctly handles `-group` (so stale groups eventually GC on next sweep), but `RemoveRoutes`/`SetUpstreamHealth` hot removal remains wrong — dead backends keep receiving traffic until GC.

---

### F-NET-07 — TCP rules render as broken HTTP/SNI-only routes; UDP half-implemented — STILL BROKEN

**Live:**
- Admission: `forge/api/internal/services/trafficmanager/service.go:383-387` `validateRoutingRule` accepts `protocol in ("","http","https","tcp")` — **udp rejected at admission** (`case "udp":` absent, returns error)
- Caddy adapter: `caddy_proxy.go:924-978` `buildGroupedRoute` has **zero `if protocol=="tcp"` branch** — always emits:
  ```go
  "match": [{"host":[domain],"path":[path+"*"]}],
  "handle": [{"handler":"reverse_proxy","upstreams":[...]}]
  ```
  Game TCP never matches HTTP `host` header
- `caddy_proxy.go:894-922` `groupRules` keys `domain|path|protocol` but rendering ignores `protocol`; `caddy_proxy.go:934-939` emits `"weight"` on `Upstream` (not a field per `reference/caddy/modules/caddyhttp/reverseproxy/hosts.go:35-65` — silently discarded)
- Traefik adapter: correctly branches at `traefik_proxy.go:758-760` to `buildTCPGroupedRoute(:854-905)` which emits `Rule: HostSNI(\`domain\`)` at `:883-888` — so plain-TCP (non-TLS) also never matches SNI; UDP at `:900-904` writes to `cfg.TCP.Services` (wrong tree — stock Traefik ignores; should be `udp:` per `reference/traefik/pkg/config/dynamic/tcp_config.go` + `udp_config.go`)
- Only real L4: `forge/api/internal/services/loadbalancer/dataplane.go:20-49,115-270` (`proxyTCP` + `serveUDP` with `udpSessionTTL 2m`) gated by `LOAD_BALANCER_ENABLED` (`loadbalancer/service.go:107`)

**Partial fix?** None. No protocol branch added to `caddy_proxy.go:924-978`; UDP still rejected at admission yet half-rendered under `cfg.TCP` on Traefik side.

---

### F-NET-08 — Certificates never delivered to data plane — STILL BROKEN

**Live:**
- Abstraction: `forge/api/internal/services/trafficmanager/gateway_adapter.go:38-64` `GatewayAdapter{SetCertificate(ctx, CertConfig), RemoveCertificate(ctx, []string), ...}` at `:49,51`
- Impls exist:
  - `caddy_proxy.go:83-121` `SetCertificate` POSTs `{"certificate":…, "key":…}` to `/tls/certificates/<domain>` via admin API
  - `traefik_proxy.go:373-397` `SetCertificate` appends `TraefikTLSCertificate{CertFile: cert.Certificate, KeyFile: cert.PrivateKey}` to `tls.yml` (note: PEM-as-path bug per F-NET-08 companion — `certFile/keyFile` are paths per `traefik_proxy.go:1100-1108` but body is written)
- **Zero callers:** `grep -rn SetCertificate forge/ --include=*.go` returns only interface + 2 impls + `gateway_adapter.go:49` — no call from `acme/service.go`, `domains/service.go`, `handlers_certificates.go`, or `main.go`. Verified live 2026-08-23.
- Renewal: `forge/api/internal/services/acme/service.go:322-366` `RenewCertificate` and `service.go:420-438` `runAutoRenewal` + durable fallback `forge/api/cmd/api/main.go:1178-1192` `JobCertRenewal` only call `store.UpdateCertificate` at `service.go:355-363` — no gateway hook
- Embedded-Caddy path: `forge/api/internal/services/trafficmanager/caddy_tls.go:18-32` `CaddyTLSManager{adminAddr, client}` with `NewCaddyTLSManager` at `:24` — **never constructed** (`grep -rn NewCaddyTLSManager forge/ --include=*.go | grep -v _test.go` empty; `main.go:982` hardcodes `NewCaddyReverseProxy` only). Its `ProvisionLetsEncrypt` at `:186-238` full-replaces `/config/` wiping routes anyway
- Traefik PEM bug: `traefik_proxy.go:380-385` writes PEM bodies into `certFile/keyFile` path fields and dupes per domain (`for range cert.Domains`); removal at `:399-435` substring-matches PEM bodies for domains

**Partial fix?** None. No new `SetCertificate` caller; `CaddyTLSManager` still unconstructed; Traefik cert path bug unchanged. References still violated: `reference/traefik/pkg/provider/acme/provider.go:796-802` `addCertificateForDomain → hot-reload` and `reference/nginx-proxy-manager/backend/internal/certificate.js:155-186` reload post-renew vs Forge DB-only.

---

### F-NET-09 — HTTP-01 dead code (default challenge never mounted) — STILL BROKEN

**Live:**
- Solver: `forge/api/internal/services/acme/service.go:52-92` `httpChallenger{token map[token/domain]keyAuth, mu}` with `Present(:61-66)` / `CleanUp(:68-73)` / `ServeHTTP(:75-92)`
- `service.go:157-159` `HTTPSolver() http.Handler { return s.httpChallenge }` — **zero callers outside package** (`grep -rn HTTPSolver forge/ --include=*.go | grep -v _test.go | grep -v .freebuff` empty)
- `service.go:193-205` `IssueCertificate` defaults `ChallengeTypeHTTP01` when `ChallengeType==""`:
  ```go
  if req.ChallengeType == "" { req.ChallengeType = ChallengeTypeHTTP01 }
  ```
  Default issuance path is the dead one; `service.go:210-219` wildcard check correctly rejects HTTP-01 for `*.` but non-wildcard default remains HTTP-01
- Wiring: `forge/api/cmd/api/main.go:983,1169` constructs `acmeSvc = acmesvc.New(db, logger)` and `acmeSvc.StartAutoRenewal` but never mounts `acmeSvc.HTTPSolver()` on `:80` nor proxies `/.well-known/acme-challenge/*` from gateway config
- Handler detail: `service.go:75-92` keys by `r.Host` without stripping `:80` port (`r.Host` may be `example.com:80` vs stored `example.com` → miss) vs `reference/traefik/pkg/provider/acme/challenge_http.go:82-86` which strips port

**Partial fix?** None. Still zero mounts; default still HTTP-01.

---

### F-NET-10 — Networking admin UX non-functional — STILL BROKEN

**Live:** (verified against phase-04 synthesis §4 + final-parity row 13/17 + reverification row 10/14 — page contracts unchanged; handler registration unchanged)
- `forge/web/app/admin/traffic/page.tsx:14-22,84,94` posts `{path,targetGroup,priority,methods}` vs backend `trafficmanager.RoutingRule{domain,path,targetPort,protocol,strategy,weight,headers,webSocket,enabled}` (`service.go:24-39`) — `domain` required at `service.go:365-372` via IDNA/publicsuffix → every create 400
- `page.tsx:67-70` fetches `GET /policies` — only `GET /policies/:id` registered at `forge/api/internal/http/handlers_trafficmanager.go:80-86` → 405
- `page.tsx:94` uses `PATCH /rules/:id` vs `PUT /rules/:id` at `handlers_trafficmanager.go:42` → 405
- `handlers_trafficmanager.go:107` `POST /admin/traffic/sync` exists but no auto-sync on CRUD (vs `reference/nginx-proxy-manager/backend/internal/nginx.js:27-117` configure→test→reload on every CRUD) — phase-04 subagent-01 §9 still applies
- `forge/web/app/admin/load-balancer/page.tsx:142-146` "Test Next" POSTs `POST /admin/load-balancer/groups/:id/next` vs `handlers_loadbalancer.go:127` GET-only
- `forge/web/app/admin/certificates/page.tsx:43-50` uploads `POST /certificates` vs `handlers_certificates.go:15-77` (no `POST /`; working `POST /custom-certificates` at `handlers_proxy_domains.go:220-227` with collision comment, zero UI callers)
- `handlers_proxy_domains.go:199-212` verify returns hardcoded `"verified": true` without checks; `AdminSecurity.tsx:114-133` hardcodes HSTS/CSP/X-Frame pills regardless of `security_headers` table (write-only, no reader at `store_security_headers.go:45`); `lib/api/security.ts:37-64` editor links to nonexistent `/admin/domains/:id`

**Partial fix?** None. Routes, verbs, and payload shapes unchanged per reverification-14 §Row 10/15. Requires rebuild/re-wire of admin pages against real schemas — not a line fix.

---

## 2. Reconciliation vs prior syntheses (30 + 18 + 18)

| Source | Prior claim | Live verdict | Line proof |
|--------|-------------|--------------|-----------|
| `audits/phase-04/synthesis.md` §2-3 | Five writers on one Caddy; 7 tables encode 3 Traefik concepts; two divergent `traffic_rules` migrations | **CONFIRMED unchanged** | `main.go:982,994-995,998,1025` one `caddyProxy` shared; `migrations/038_traffic_routing.sql:1` vs `api/migrations/083_a_traffic_rules.sql` (mapped by `store_traffic.go:11` `TrafficRuleRow` vs `store_routing.go:21` `RoutingRuleRow`) |
| `phase-04/synthesis.md` §4 F-NET-01..10 | 10 P0s; F-NET-03 validate= apply via `/load` vs `/adapt` | **10/10 CONFIRMED still open** | Table §0 + §1 above; `caddy_proxy.go:980-1002` still `/load`; `/adapt` still zero callers; `reference/caddy/caddyconfig/load.go:68-72` vs `:137-175` |
| `phase-04/synthesis.md` §3/6 | `GatewayAdapter.SetCertificate/RemoveCertificate` zero callers; `acme.HTTPSolver` never mounted; Traefik 1,139 lines dead | **CONFIRMED** | `gateway_adapter.go:38-64`, `acme/service.go:157-159`, `traefik_proxy.go:1-1139` not constructed in `main.go:982` |
| `audits/final-parity/subagent-07-networking-gateway.md` §0 | Densest cluster 10 P0s; 18 rows `BROKEN/PARTIAL` | **CONFIRMED — reproduced 18 rows, same verdicts** | Rovers rows 01-18 in §3 below |
| `final-parity/subagent-07` §2 LF-01..05 | Empty-sync, cert non-delivery, validate=apply, fictive+all-policies, Traefik dead + health fiction + UI mismatch | **ALL STILL OPEN — FIND-01..05 ≡ F-NET-01..10** | Cross-ref §1 |
| `audits/reverification/subagent-14-networking-gateway.md` §0 | 10 P0s 100% still broken; same five writers + four adapters | **CONFIRMED — 0/10 remediated between parity and reverification and this live read** | Same wiring at `main.go:982,994-995,1025`; same empty-sync at `ingress_sync.go:111` |
| `reverification/subagent-14` §1 18 rows | Rows 02,08,11-15 BROKEN; rows 01,03-06,09-10,16-18 PARTIAL/MISSING | **STILL BROKEN/PARTIAL — no row promoted** | See §3 |
| `reference/networking/caddy` vs `reference/networking/traefik` | `/load` is apply, `/adapt` is dry-run; Traefik API GET-only; file provider fsnotify | **Contracts unchanged — Forge violates both** | `caddy/caddyconfig/load.go:68-116` vs `:137-175`; `traefik/pkg/api/handler.go:103-133` GET-only; `traefik/pkg/provider/file/file.go:90,170-207` |

No contradictory evidence found — `phase-04` (5 writers, fictive handlers, empty-sync), `final-parity` (18-row matrix, densest-cluster box), `reverification-14` (18-row reproduce, FAST-style FIND-01..05), and `phase-06/subagent-08-uncloud-mesh` (per-node single-writer lesson at `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` fingerprint-cache vs Forge central synchronizer) are mutually consistent.

---

## 3. Parity matrix — 18 rows vs `final-parity/subagent-07` and `reverification/subagent-14` — all re-verified STILL BROKEN/PARTIAL

Legend: STATUS `PARITY`/`PARTIAL`/`MISSING`/`BROKEN` · SEVERITY `P0`/`P1`/`P2`

| Row | Area | Reference contract | Forge live | STATUS (live) | Still broken? | Severity |
|-----|------|--------------------|------------|---------------|---------------|----------|
| 01 | HTTP host/path routing | `reference/caddy/modules/caddyhttp/routes.go:31` `Route{match[],handle[],terminal,group}`; `reference/traefik/pkg/config/dynamic/http_config.go:38` `HTTPConfiguration{Routers→Services→Middlewares}`; `reference/nginx-proxy-manager/backend/schema/components/proxy-host-object.json:4` | `service.go:24` `RoutingRule`; `caddy_proxy.go:924-978` / `traefik_proxy.go:755-852` render; `domains/service.go:455-489 → caddy_proxy.go:514-593` second surface; but 4 route concepts, no priority, manual `POST /admin/traffic/sync` (`handlers_trafficmanager.go:107`) | `PARTIAL` | YES (no auto-sync) | P1 |
| 02 | TCP/UDP L4 | `reference/traefik/pkg/config/dynamic/tcp_config.go` + `udp_config.go` `HostSNI`; `reference/nginx-proxy-manager/backend/templates/stream.conf:8-27` | `service.go:383-387` udp rejected; Traefik `buildTCPGroupedRoute(:854-905)` exists but Caddy `buildGroupedRoute(:924-978)` no branch | `BROKEN` (gateway L4) | **YES (F-NET-07)** | P0 |
| 03 | Route grouping | `reference/caddy/modules/caddyhttp/reverseproxy/hosts.go:29` `UpstreamPool`; `reference/traefik/pkg/config/dynamic/http_config.go:404-425` Merge | Three groupers `caddy_proxy.go:894-922` + `traefik_proxy.go:725-753` + `crossnode/routegroup.go:32-68` (empty-path `"/"` normalization `:44-49` where adapters don't) + `<firstRuleID>-group` at `:949-951` | `PARTIAL` | YES (triplication) | P1→P0 w/ health |
| 04 | Strategies/weights | `reference/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go:41-98` `WeightedRoundRobinSelection{Weights[]int}`; `reference/traefik/pkg/config/dynamic/http_config.go:62-69,371-382` | `service.go:33-34`; Caddy `upstreams[].weight(:934-939)` not a field + `lb_policy=least_connections(:944-946)` invalid; Traefik sticky-cookie (`:841-849`); LB `__weighted(:480-482)` shared | `BROKEN` (Caddy weights) | YES | P1 |
| 05 | Health probes (HTTP-aware) | `reference/caddy/modules/caddyhttp/reverseproxy/healthchecks.go:73,159,233-268`; `reference/traefik/pkg/config/dynamic/http_config.go:483-496` | `service.go:1001-1082` fixed 2s TCP dial; `dataplane.go:296-326` ignores `HealthCheckConfig(:62-69)`; `health_filter.go:76-124` zero prod callers | `MISSING`/`BROKEN` | YES | P1 |
| 06 | Passive health / oscillation | `reference/caddy/healthchecks.go:233-246` + `reverseproxy.go:1342` TryDuration; `reference/traefik/pkg/server/service/loadbalancer/failover/failover.go:16-73` | `dataplane.go:142-146` single-fail drop; `caddy_proxy.go:503-508` re-add fights rebuild `673-720` | `PARTIAL` | YES | P1 (P0 under outage) |
| 07 | WebSocket | `reference/caddy/modules/caddyhttp/reverseproxy/streaming.go:241-263`; `reference/nginx-proxy-manager/backend/templates/proxy_host.conf:19-23` | `service.go:37` + `caddy_proxy.go:966-971` `header_up` + `traefik_proxy.go:835-839` `flushInterval` | `PARITY` | NO | P2 |
| 08 | TLS issuance: HTTP-01/DNS-01/TLS-ALPN-01 | `reference/caddy/modules/caddytls/acmeissuer.go:218-319`; `reference/traefik/pkg/provider/acme/challenge_http.go:33-100` | `acme/service.go:52-92,157-159` dead (see F-NET-09); DNS-01 36 providers but `dns01.AddRecursiveNameservers(:315-317)` fixed, env mutation `dns/service.go:489-498,639-661` | `BROKEN` (HTTP-01) | **YES (F-NET-09)** | P0 |
| 09 | Wildcard handling | `reference/traefik/pkg/provider/acme/provider.go:1057-1062` `sanitizeDomains` | `acme/service.go:210-219` `*.` check; `domains/service.go:504-534` IDNA; Caddy apex+wildcard `:595-655`; no `*.*` reject, no `UnFqdn`, no SAN prune | `PARTIAL` | YES | P1 |
| 10 | Renewal window & storage | `reference/traefik/pkg/provider/acme/provider.go:808-823`; `reference/caddy/.../certmagic/automation.go:65,129` | `acme/service.go:380-438` 24h ticker + `store_certificates.go:260` 30d window; key reuse `:536-540` anti-pattern (`certmagic/automation.go:144-151`); `recover` outside loop `:389-396`; plaintext `certificates.dns_credentials` while siblings encrypted `:86,212` | `PARTIAL` | YES | P1 (P0 security) |
| 11 | Middleware chain ordered vs all-` | `reference/traefik/pkg/server/middleware/middlewares.go:50-83` ordered | `traefik_proxy.go:960-972` all-policies; `caddy_proxy.go:722-793` all-policies; map-order non-deterministic `traefik_proxy.go:769-787`; fictive `rate_limit/circuit_breaker` `caddy_proxy.go:795-875` | `BROKEN` | **YES (F-NET-04+05)** | P0 |
| 12 | Dry-run validation | `reference/caddy/caddyconfig/load.go:68-72,137-175` `/load`=apply `/adapt`=dry-run; `reference/nginx-proxy-manager/backend/internal/nginx.js:27-117` `.err` + `meta offline` | `caddy_proxy.go:980-1002` `/load` is apply; `caddy_proxy.go:691-695` snapshot after; no `/adapt`; Traefik `POST /api/refresh` dead | `BROKEN` | **YES (F-NET-03)** | P0 |
| 13 | Gateway delivery of certs | `reference/traefik/pkg/provider/acme/provider.go:796-802` hot-reload; `reference/nginx-proxy-manager/backend/internal/certificate.js:155-186` | `gateway_adapter.go:49` zero callers; `acme/service.go:355-363` DB-only; `caddy_tls.go:18-32` never constructed; Traefik PEM-as-path `:380-385` | `BROKEN` | **YES (F-NET-08)** | P0 |
| 14 | Config authority (single vs five writers) | `reference/traefik/pkg/config/dynamic/http_config.go:38-46` one `*dynamic.Configuration`; `reference/caddy/admin.go:1091-1099` single doc; `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` fingerprint-cache never writes invalid | Five writers `main.go:982,994-995,998,1025` + `loadbalancer/dataplane.go:20-49` bypass + rows-with-no-renderer | `BROKEN` | **YES (F-NET-01+02)** | P0 |
| 15 | Gateway route lifecycle (RemoveRoutes/CleanupStale) | `reference/caddy/admin.go:1091-1099` `/id/<id>`; `reference/traefik/pkg/provider/file/file.go:90,170-207` | `caddy_proxy.go:51-74` per-rule DELETE vs `buildGroupedRoute:949-951` `-group`; `CleanupStale:355-357` handles `-group` but `RemoveRoutes` not | `BROKEN` | **YES (F-NET-06)** | P0 w/ health |
| 16 | Static IP / target addressing | `reference/app-platforms/uncloud/internal/machine/cluster/ipam.go:21-81` IPAM `/24`; `reference/traefik/pkg/provider/docker/config.go:288-359` | `service.go:620-673` resolve; `resolver.go:59-83,110` caches `localhost` 30s; `handlers_loadbalancer.go:88-100` arbitrary IP accepted; `domains/service.go:467-468` hardcodes `localhost:8080` | `PARTIAL` | YES | P1 (P0 if metadata) |
| 17 | Observability headers / hardening | `reference/caddy/modules/caddyhttp/server.go:1171-1236` `trusted_proxies` CIDR; `:663-678` StrictSNI | Two middlewares `middleware_security.go:27-62` vs `middleware_security_headers.go:48-93`; CSP `"fallback-nonce"`; `middleware_ratelimit.go:98-119` peer-private trust; `Health()` hardcoded `:375-377` | `PARTIAL` | YES | P1 |
| 18 | Firewall / discovery (uncloud parity) | `reference/nginx-proxy-manager/backend/internal/proxy-host.js:21-108`; `reference/app-platforms/uncloud/internal/machine/dns/server.go:44-54,259-326` | `handlers_firewall.go:78-193` `map[string]any` pass-through no DB; `servicediscovery/registry.go:32-40` + `reachability.go:58-79` API-vantage dial; test-only `networkpolicy.go:35-69` | `MISSING`/`PARTIAL` | YES | P1 |

Rows 11-14 carry the P0 cluster (middleware chain, dry-run, cert delivery, config authority) per `final-parity/subagent-07` §1 — all four remain `BROKEN`.

---

## 4. Any partial fix since `phase-04` / `final-parity` / `reverification-14`?

**None that closes a P0.** Exhaustive `git log --oneline -50` + live diff shows no commits touching the 10 P0 line ranges between those syntheses and this read:

- **F-NET-01:** `ingress_sync.go:111` still unconditional; `main.go:994` still `30*time.Second` — no `len(rules)==0` guard added; no `SetRules` population from `trafficmanager+domains`. Only delta is domain startup-sync goroutine reducing recovery latency.
- **F-NET-02/03/04/05/06/07:** `caddy_proxy.go` ranges `60-74,279-293,523-593,673-720,795-875,894-978,980-1002` identical to `phase-04/subagent-01` citations; `traefik_proxy.go:960-972,980-1002,1065-1084` identical; `routegroup.go:149-151` still `return ""`.
- **F-NET-08/09:** `gateway_adapter.go:38-64`, `acme/service.go:52-92,157-159,193-205,355-363` identical; no new `HTTPSolver()` mount in `main.go` or HTTP router; no new `SetCertificate` caller.
- **F-NET-10:** `web/app/admin/traffic/page.tsx` and `handlers_trafficmanager.go` shapes unchanged.
- Coverage-mode note: P1/P2 rows 04/05/06/16/17 also unchanged — weights still on upstream, health still fixed 2s TCP dial, private-IP trust still unbounded, `Health()` still hardcoded.

---

## 5. Recommended activation order (unchanged — densest P0 first)

Per `phase-04/synthesis.md` §8 and `final-parity/subagent-07` §3 (confirmed):

1. **Stop the bleeding:** `ingress_sync.go:111` `if len(rules)==0 { return nil }`; route ALL Caddy writes through ONE writer (or sub-resource `POST /config/apps/http/servers/<name>` merges like `UpdateDomainRoutes` pattern `caddy_proxy.go:523-569`); snapshot **before** `validateConfig`; replace `POST /load` with `POST /adapt` or `GET /config/`+merge semantics.
2. **Gate fictive handlers:** never emit `rate_limit`/`circuit_breaker` until real modules (`xcaddy`) exist — map to `remote_ip`/`subroute` constructs; fix `all-policies-all-routes` via `rule↔policy` join + deterministic order `rate-limit→blacklist→whitelist→CB→redirect`.
3. **Make grouped-route lifecycle consistent:** `RemoveRoutes` must handle `-group` suffix like `CleanupStale` does; make `SetUpstreamHealth` weight-preserving and idempotent.
4. **Fix or drop TCP/UDP gateway path:** reject `tcp` at admission until `layer4` app + `udp:` tree, or emit Caddy `layer4` + Traefik `udp:` correctly; keep `loadbalancer/dataplane.go` as canonical L4.
5. **Wire cert delivery:** call `GatewayAdapter.SetCertificate` after `Create/Update/RenewCertificate`; write real `*.pem` files for Traefik; construct or delete `CaddyTLSManager`; mount `acmeSvc.HTTPSolver()` on `:80` or proxy `/.well-known/acme-challenge/*`; fix PEM-as-path.
6. **Rebuild admin against real schemas** (`handlers_trafficmanager.go:15-67`, `service.go:24-39`).
7. **Decide Traefik fate** — file-watch `provider/file/file.go:90,170-207` or delete 1,139 lines.

---

## 6. Evidence index (file:line — inspected this pass, representative)

**Wiring (also handoff to phase-05):**
- `forge/api/cmd/api/main.go:982` `NewCaddyReverseProxy`, `:993-995` `NewIngressSynchronizer` + `Start(30*time.Second)`, `:998` `domains.New(...caddyProxy…)`, `:1025` `NewWithPersistence(...,caddyProxy,…)`, `:1029-1040` domain startup sync (new), `:1041-1067` 7 subscriptions (ingress×3 + tmSvc + lbSvc), `:1178-1192` `JobCertRenewal`, `:1151-1156` domain drift reconciler comment

**Caddy adapter (inspect via `caddy_proxy.go`):**
- `:44-49` struct, `:51-74` `RemoveRoutes` per-rule DELETE, `:83-121` `SetCertificate`, `:149-230` `GetActiveConnections` (upstream/file counts), `:279-293` `Rollback`, `:295-373` `CleanupStale` (`-group` at `:355-357`), `:375-377` `Health()` hardcoded, `:379-512` `SetUpstreamHealth` (append-bug `:503-508`, `Contains` at `:457`), `:514-593` `UpdateDomainRoutes` merge, `:673-705` `updateRoutesAtomic` (validate-before-snapshot at `:691-695`), `:707-720` `buildServerConfig` (gamepanel only), `:722-793` `buildPolicyRoutes`, `:795-875` `buildPolicyHandles` (fictive handlers `:798-804,:858-872`), `:894-978` grouping + `buildGroupedRoute` (`@id gamepanel-<first>-group` at `:949-951`, `weight` at `:934-939`, `lb_policy` at `:973-975`), `:980-1052` `validate/getRunning/apply/restore`

**TLS second Caddy client:**
- `forge/api/internal/services/trafficmanager/caddy_tls.go:18-32,58-175,186-238,240-251,253-280` (never constructed — `grep -rn NewCaddyTLSManager forge/ | grep -v _test` empty)

**Traefik adapter (1,139 lines, dead in prod at `main.go:982`):**
- `traefik_proxy.go:243-271` `UpdateRoutes`, `:273-293` `RemoveRoutes`, `:373-397` `SetCertificate` (PEM-as-path at `:380-385`), `:399-435` `RemoveCertificate`, `:471-559` `CleanupStale`, `:561-604` `Health`, `:606-697` `SetUpstreamHealth`, `:725-753,755-905` grouping + TCP branch at `:854-905`, `:907-958` `buildMiddlewares`, `:960-972` `collectApplicablePolicies` (returns all), `:974-1005` `validateYAMLConfig`, `:1065-1084` `reloadTraefik POST /api/refresh` dead, `:1100-1108` TLS path type

**Gateway abstraction:**
- `gateway_adapter.go:10-12,14-19,38-64` `GatewayAdapter` (+ legacy `service.go:101-105` `ReverseProxy`, `domains/service.go:74-76` `caddyUpdater`); zero callers `gateway_adapter.go:38-64` interface

**Traffic / LB / crossnode:**
- `trafficmanager/service.go:24-54` models, `:361-405` `validateRoutingRule`, `:562-618,753-951,1001-1082` CRUD/Apply/Sync/withdraw/probe, `:620-673` `resolveTargets`
- `loadbalancer/service.go:22-69,480-482` `__weighted` shared counter, `:552-639` node-mark; `dataplane.go:20-49,115-270,296-326` listeners + fixed-2s `runHealthChecks`
- `crossnode/ingress_sync.go:23-37,111-198,200-228` empty-sync; `routegroup.go:32-97,149-151` grouping + `extractPolicyID=""`; `health_filter.go:42-56,76-124` no producers; `resolver.go:59-111,135-166` 30s `localhost` cache

**ACME / DNS / domains:**
- `acme/service.go:52-92` challenger, `:157-159` `HTTPSolver()` dead, `:193-295,315-317,368-370,389-438,492-546,612-638` solver dead + key reuse `536-540` + revoke no-op `368-370` + 24h ticker `397` + recover outside loop `389-396`
- `dns/service.go:311-487,489-498,639-661` 36 providers + env mutation; `domains/service.go:74-76,455-489,504-534` + hardcode `localhost:8080` at `:467-468`

**References (contracts):**
- `reference/networking/caddy/caddyconfig/load.go:68-72,116,137-175` `/load` vs `/adapt`; `caddy/admin.go:1067-1069,1091-1099` `must-revalidate` + `/id/<id>`; `modules/caddyhttp/routes.go:31-41`; `modules/caddyhttp/reverseproxy/healthchecks.go:67,233-268`; `modules/caddyhttp/reverseproxy/hosts.go:29,35-65`; `modules/caddyhttp/reverseproxy/selectionpolicies.go:41-98`
- `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-46,62-99` router→middleware→service; `pkg/api/handler.go:103-133` GET-only; `pkg/provider/file/file.go:90,170-207` `fsnotify`; `pkg/server/middleware/middlewares.go:50-83` ordered chain
- `reference/networking/nginx-proxy-manager/backend/schema/components/proxy-host-object.json:4-25`, `backend/internal/{proxy-host.js:32-47,nginx.js:27-117,certificate.js:155-186,audit-log.js:84-103,access-list.js:24-60}`
- `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` per-node single-writer fingerprint-cache `never writes invalid(:188-189)` — vs Forge `ingress_sync.go:166` `UpdateRoutes(mergedRules)`

---

## 7. Handoff

`phase-05` (Nomad/Incus/River/NetBird/Longhorn/Rancher vs placement/scheduler/runtime/clustering/queue) should treat **F-NET-01/02's single-writer lesson** as axiomatic when evaluating Nomad eval-broker and River tx-bound `enqueue` — multiple writers on one resource = last-writer-wins drift by design, as proven here by three independent Caddy builders plus crossnode pushing empty. Fixing `ingress_sync.go:111` + `updateRoutesAtomic:673-720` to sub-resource merge plus `POST /adapt:137` is the first code change, not backlog.

*Produced with file:line citations only; no product code modified. Re-verification shows 10/10 P0s still open — densest cluster of the audit.*
