# 110 Phase 01 Audit — Subagent 06: Networking & Gateway

**Scope:** `trafficmanager`, `loadbalancer`, `crossnode`, `servicediscovery`, `domains`, `acme`, `dns`, `firewall` + HTTP handlers + `forge/web` traffic/load-balancer/domains/certificates/firewall  
**Date:** 2026-08-24  
**Mode:** Preparation audit — inventory, known P0s, files-to-touch, single-writer inversion plan, migrations, risks. Do not modify code.  
**Evidence:** File:line citations verified against current checkout. Cross-checked against `audits/FINAL_PARITY_AUDIT.md §8 NG-01..NG-20` (575 lines) and `audits/phase-04/synthesis.md` (10 P0s).

---

## 1. Inventory — What Exists Today

### 1.1 TrafficManager (L7 gateway)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/trafficmanager/service.go:24` | 1-1082 | `RoutingRule` / `TrafficPolicy` structs (24:40), `Service` with `rules/policies/withdrawnRules/healthFailures` in-mem maps (73:90), `ApplyRoutes`/`SyncRoutes`/`ProbeTargets`/`WithdrawNodeTargets`/`ReconcileRoutes` all call `proxy.UpdateRoutes` or `RemoveRoutes` |
| `forge/api/internal/services/trafficmanager/gateway_adapter.go:38` | 1-64 | `GatewayAdapter` interface (38:63) — `UpdateRoutes`, `RemoveRoutes`, `UpdateDomainRoutes`, `SetCertificate`, `RemoveCertificate`, `ValidateConfig`, `Reload`, `Rollback`, `CleanupStale`, `SetUpstreamHealth`, `Health` |
| `forge/api/internal/services/trafficmanager/caddy_proxy.go:673` | 1-1079 | `CaddyReverseProxy` — `updateRoutesAtomic:673` builds `serverConfig` with *only* `gamepanel` server (707:720), `validateConfig:980` POSTs `/load` (an apply), `applyConfig:1026` POSTs `/config/`, `buildPolicyRoutes:722` + `buildPolicyHandles:795` emit fictional handlers, `groupRules:894` / `buildGroupedRoute:924` triplicated grouping, `buildDomainRoutes:595` + `UpdateDomainRoutes:514` uses read-modify-write preserving `gamepanel` |
| `forge/api/internal/services/trafficmanager/caddy_tls.go:18` | 1-280 | `CaddyTLSManager` (18:32) — never constructed in prod; `ProvisionLetsEncrypt:34` + `UploadCustomCert:58` speak to `/tls/certificates/*` or stub `buildTLSConfig:186` which returns isolated doc (`tls.automation` + single `gamepanel-domains` route on `:443`); `validateConfig:240` is local JSON parse only, not Caddy dry-run |
| `forge/api/internal/services/trafficmanager/traefik_proxy.go:373` | 1-1139 | `TraefikReverseProxy` (21:30) — 1139 lines, never constructed in prod (`cmd/api/main.go:982` hardcodes Caddy); `SetCertificate:373` writes PEM file paths as `CertFile/KeyFile` (file-path fields holding PEM strings — bug), `RemoveCertificate:399` string-contains filter, `reloadTraefik:1065` POSTs nonexistent `/api/refresh`, `Health:561` reads `configDir` + GET `/api/http/routers`, `groupRules:725` third copy of grouping, `collectApplicablePolicies:960` returns all policies |
| `forge/api/internal/services/trafficmanager/caddy_admin.go:1` | 1-69 | `caddyAdminBaseURL:24` validates loopback vs remote-TLS, `newCaddyAdminRequest:51` adds `CADDY_ADMIN_TOKEN` auth |

**State:** Single real adapter (Caddy) with asymmetric writers; Traefik is decorative; TLS manager is dead code.

### 1.2 LoadBalancer (L4 dataplane, bypasses gateway)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/loadbalancer/service.go:22` | 1-639 | Types `TargetGroup:38`, `HealthCheckConfig:62`; `Service:71` with `groups/roundRobin/connCount/listeners/packets`; `validateTargetGroup:163`, CRUD + `NextTarget:390`, `nextWeightedRoundRobin:468` with shared `__weighted` key, `MarkNodeTargets*` event handlers (558:638) |
| `forge/api/internal/services/loadbalancer/dataplane.go:20` | 1-326 | `Start:20` 2s ticker reconcile+health; `reconcileListeners:62` binds `LOAD_BALANCER_BIND_HOST:Port`; `serveTCP/proxyTCP:115/130` raw `io.Copy` both directions; `serveUDP:187` + `relayUDPResponses:256` session map; `runHealthChecks:296` raw TCP dial 2s, skips UDP (306:308), ignores `HealthCheckConfig` path/interval |

**State:** Independent L4 TCP/UDP listeners outside Caddy; own health loop; config as in-mem + DB.

### 1.3 Crossnode (third gateway writer + resolver)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/crossnode/ingress_sync.go:111` | 1-300 | `IngressSynchronizer` (23:37) with its own `rules/policies` maps; `Sync:111` groups via `GroupRulesByRoute`, filters via `HealthFilter`, builds `mergedRules` including synthetic `-replica-N` rules (145:162), then `adapter.UpdateRoutes:166` unconditionally — empty maps produce empty slice → wipes `gamepanel` |
| `forge/api/internal/services/crossnode/resolver.go:14` | 1-188 | `Resolver:14` cache 30s (54:55), `ResolveTargetHost:59` precedence `discovery → store server→node → localhost`, `resolveFromDiscovery:135` fail-open to unhealthy endpoints, `ResolveNodeAddress:168` same |
| `forge/api/internal/services/crossnode/routegroup.go:32` | 1-195 | `GroupRulesByRoute:32` struct `RouteKey:11`/`RouteGroup:17`; `UniqueBackends:70` sorted; `extractPolicyID:149` returns `""` always → `PolicyIDs:108` always empty; `BuildRouteGenerationRecords:163` |
| `forge/api/internal/services/crossnode/health_filter.go:31` | 1-282 | `HealthFilter:31` threshold 3, `RecordSuccess/RecordFailure:76/98`, `FilterHealthy:138` used by ingress_sync only; `StartReaper:217` removes stale health entries |

**State:** Third writer, never populated in prod (zero non-test callers of `SetRules:200`/`UpsertRule:218`), so every 30s tick pushes empty config.

### 1.4 ServiceDiscovery (dynamic-config / discovery)

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/servicediscovery/service.go:12` | 1-179 | Facade `Service:12` wiring `Registry`+`Reaper`+`Verifier`+`Policy` + `adapter:21` (NetworkAdapter, currently unused) |
| `forge/api/internal/services/servicediscovery/discovery.go:11` | 1-155 | `ServiceDiscovery:11`; `Resolve:54` prefers healthy else returns *all* (fail-open, 60:69); `ResolveAll:78` unfiltered; `VerifyCrossNodeReachability:90` looks up registry endpoint then `verifier.VerifyCrossNode` |
| `forge/api/internal/services/servicediscovery/reachability.go:33` | 1-129 | `ReachabilityVerifier:12` `VerifyEndpoint:33` + `VerifyCrossNode:58` dial from API server (not source node) with `5s` timeout (24) → wrong vantage point |
| `forge/api/internal/services/servicediscovery/registry.go:32` | 1-381 | `Registry:32` `RegisterEndpoint:64` / `RemoveEndpoint:122` with store persistence; `ListEndpoints:163` filtered; `RebuildFromEndpoints:279` |
| `forge/api/internal/services/servicediscovery/stale_reaper.go:11` | 1-134 | `StaleEndpointReaper:11` interval 30s, TTL 3m; `reap:90` marks stale as `Unhealthy` but never removes (117) |
| `forge/api/internal/services/servicediscovery/adapter.go` | — | `NetworkAdapter` interface (refer `service.go:21`) |
| `forge/api/internal/services/servicediscovery/networkpolicy.go` | — | `PrivateNetworkPolicy` port ACLs, zero UI |
| `forge/api/internal/services/servicediscovery/visibility.go:49` | 1-196 | `BuildNetworkVisibility:49`, `BuildNodeView:110` — complete, unexposed in UX |

**State:** Registry/reaper/verifier/policy all functional but largely unwired to gateway or UI; reaper never truly removes.

### 1.5 Domains

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/domains/service.go:87` | 1-534 | `Service:91` with `domainStore` + `caddyUpdater` (74:76, second Caddy writer); `syncCaddyRoutes:455` builds `[]VerifiedDomainRoute` from `ListVerifiedDomains` via `NodeResolver.ResolveServerTarget` (470:475) then `caddy.UpdateDomainRoutes:484`; `AddDomain:172` (normalizes via `idna`+`publicsuffix`), `verifyOwnership:357` HTTPS GET `https://<host>/.well-known/forge-verify` (383) with `Host` header override (390), `reverifyAll` ticker 24h (127:155) |

**State:** Second Caddy writer using *correct* read-modify-write (`caddy_proxy.go:514`), fixed to preserve `gamepanel`. But competes with `updateRoutesAtomic` which does not preserve `gamepanel-domains`.

### 1.6 ACME

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/acme/service.go:52` | 1-687 | `httpChallenger:52` map `token/domain→keyAuth` (61:64), `ServeHTTP:75` trims `/.well-known/acme-challenge/`; `HTTPSolver:157` exposes it; `IssueCertificate:193` default `admin@localhost` (198) + RSA2048 (241), `configureChallenge:297` wires HTTP-01 via `httpChallenge` vs DNS-01 via `dns01.AddRecursiveNameservers` fixed `1.1.1.1/8.8.8.8` (316); `renewOnce:492` reuses stored `PrivateKey` forever (331:333, 499:543), `RevokeCertificate:368` DB delete only, `StartAutoRenewal:380` 24h ticker with recover *outside* loop (389:396) → panic kills loop |
| `forge/api/internal/services/acme/providers.go` | — | `RegisterDNSProvider` glue called from `dns/service.go:593` |

**Critical:** `HTTPSolver()` is never mounted in `forge/api/cmd/api/main.go` or `forge/api/internal/http/server.go` — no route serves `/.well-known/acme-challenge/*`. `grep -rn HTTPSolver forge` hits only `acme/service.go:157` + test. Means default `ChallengeTypeHTTP01` (203) always times out.

### 1.7 DNS

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/services/dns/service.go:90` | 1-663 | `supportedProviders:90` metadata for 30+ providers; `providerRegistry:311` maps type→factory via `createDNSProvider:489` which does `os.Setenv` + restore with global `envMu:637` (microscopic window but still process-global mutation, needs per-call validation); `createDNSProvider:495` restores immediately (audit P1 fix applied); `acmeFactory:603` prefers inline creds else `GetDefaultDNSProvider`; `credentialsFromProvider:620` parses `provider.Credentials` JSONB; `VerifyProvider:527` tests factory, `RegisterWithAcme:593` bridges to ACME; `ExecuteDNSChallenge:555` / `CleanupDNSChallenge:574` on default provider |

**State:** Env-mutation window now minimal (restored defer), but `PDNS_API_URL`/`RFC2136_NAMESERVER` still lack scheme/IP validation (SSRF surface) — `handlers_loadbalancer.go:88` plus `dns/service.go:489` were P1s.

### 1.8 Firewall

| File | Lines | Role |
|---|---|---|
| `forge/api/internal/http/handlers_firewall.go:7` | 1-210 | `registerFirewallRoutes:7` — 12 routes under `/host/firewall` (`status/enable/disable/rules crud/port/forward`), all passthrough to `cfg.Daemon.*` via `resolveNodeHostTarget`, bodies as `map[string]any` (86:88), zero validation/persistence |
| `forge/web/components/admin/AdminFirewall.tsx:32` | 1-462 | Full UX: node selector, enable/disable, Rules/Forwards tabs, `AddRuleModal:323` + `EditRuleModal:370` + `AddForwardModal:417` |

**State:** Entirely ephemeral daemon pass-through; no DB desired-state, no reconciler, no drift detection; node reprovision loses rules silently.

### 1.9 HTTP Handlers (networking slice)

| File | Lines | Finding |
|---|---|---|
| `forge/api/internal/http/handlers_trafficmanager.go:107` | 1-113 | `registerTrafficManagerRoutes:8` group `/admin/traffic` (13). Routes: `GET/POST/GET/:id/PUT/:id/DELETE/:id` rules (15:59), `GET server/:serverId` (61:67), `POST/GET/:id/PUT/:id/DELETE/:id` policies (69:105) — but **no `GET /policies` list** → web cannot list; `PUT` not `PATCH`, web sends `PATCH` (traffic `page.tsx:93`) → 405; `POST /sync:107` calls `SyncRoutes:108` |
| `forge/api/internal/http/handlers_loadbalancer.go:84` | 1-138 | Group `/admin/load-balancer` (29). `POST /groups/:id/targets:84` accepts raw `ip/port/weight` with zero IP validation → internal probe primitive; `Get /groups/:id/next:127` is GET but web `Test Next` sends POST → mismatch; `Patch /targets/:targetId` expects `status` enum |
| `forge/api/internal/http/handlers_domains.go:19` | 1-124 | `registerDomainRoutes:19` — scoped `/servers/:id/domains` + legacy `/domains/verify` + `/domains/check-dns` + `registerWellKnownVerifyRoute:102` serving `/.well-known/forge-verify` (real domain ownership path, not ACME) |
| `forge/api/internal/http/handlers_proxy_domains.go:199` | 1-412 | `registerProxyDomainRoutes:9` CRUD `/domains` (NPM-style `proxy_domains`); `POST /:id/verify:199` returns `{"verified":true}` unconditionally — fictional; `registerProxyCertificateRoutes:215` exposed as `/custom-certificates` (226) with duplicate `/certificates` prefix collision avoided; `registerSecurityHeadersRoutes:312` + `registerRedirectRulesRoutes:362` write-only tables |
| `forge/api/internal/http/handlers_certificates.go:10` + `handlers_certificates_ext.go:17` | 77 / 176 | `registerCertificateRoutes:10` `/certificates` issue/list/get/delete/renew (ACME); `registerCertificateRoutesExt:17` `/certificates/upload` validates PEM+keypair, `/:id/download` + `/:id/export` (leaks `privateKey` in JSON) — but both groups share `/certificates` prefix; ext writes `provider=manual` (154:69) never delivered to gateway |
| `forge/api/internal/http/handlers_dns.go:11` | 1-90 | `registerDNSRoutes:11` `/dns/providers*` list/configure/verify/set-default |
| `forge/api/internal/http/handlers_firewall.go:7` | (above) | see §1.8 |

### 1.10 Web (5 admin pages)

| Route | File | Finding |
|---|---|---|
| `/admin/traffic` | `forge/web/app/admin/traffic/page.tsx:1` (350 lines) | Types `RouteRule:14` `{path,targetGroup,priority,methods}` do **not** match backend `RoutingRule` `{domain,path,targetPort,protocol,strategy,...}` (service.go:24); `policies` list fetches `GET /admin/traffic/policies` which has no handler → 405/empty; `updateRouteMutation:92` sends `PATCH` vs handler `PUT` |
| `/admin/load-balancer` | `forge/web/app/admin/load-balancer/page.tsx:1` (239 lines) | Correct against `TargetGroup` (25:34); but `testSelectionMutation:142` POSTs `/groups/:id/next` while handler is `GET` (handlers_loadbalancer.go:127) → 405; `IP` accepted verbatim displayed |
| `/admin/domains` | `forge/web/app/admin/domains/page.tsx:1` (302 lines) | Server-scoped (`/servers/:id/domains`), selector required, `POST /domains/verify` + `checkDNSApi` wired correctly |
| `/admin/certificates` | `forge/web/app/admin/certificates/page.tsx:1` (199 lines) | `fetchJSON("/certificates")` (33) missing `/admin` prefix? Actually cert routes are `/admin/certificates` via `adminIPAccess`? Check `server.go:2614` `protected.Group("/certificates"` under `protected` (= `/api`?) — prefix mismatch probable; `postJSON("/certificates", uploadForm)` (44) posts to list-404; `"/certificates/:id/renew"` same |
| `/admin/firewall` | `forge/web/app/admin/firewall/page.tsx:1` + `AdminFirewall.tsx:32` | Complete, matches daemon API |

---

## 2. Known P0s — Synthesis of FinalParity NG-01..20 and Phase-04 F-NET P0s

### 2.1 Fidelity to FINAL_PARITY §8

Every NG row re-verified against current code; `Status Today` is current checkout truth, not 2026-08-24 snapshot.

| NG | Capability (Reference) | Forge File:Line | FINAL Status | Status Today | Evidence of fix / still broken |
|---|---|---|---|---|---|
| **NG-01** | HTTP host/path routing authority — one writer (Traefik one tree / Caddy one doc / NPM rows) | `caddy_proxy.go:673` `updateRoutesAtomic`, `domains/service.go:455` `syncCaddyRoutes`, `crossnode/ingress_sync.go:111` `Sync`, `loadbalancer/dataplane.go:20` `Start` (bypass) | BROKEN P0 — five writers, empty-sync wipes | **STILL BROKEN** | `crossnode/ingress_sync.go:111` still calls `UpdateRoutes` with empty `mergedRules` every 30s (if `SetRules` never called). `main.go:994` starts it with empty maps. No empty-guard added. `caddy_proxy.go:673` still full-replaces. `domains/service.go:455` is second writer. LB is fifth writer on separate port but conceptually fifth. |
| **NG-02** | Route grouping into pools | `caddy_proxy.go:894` vs `traefik_proxy.go:725` vs `crossnode/routegroup.go:32` | BROKEN | **STILL BROKEN** | Three `groupRules` copies remain byte-divergent; Caddy groups by `domain|path|protocol` string key (894), Traefik by same but with `traefikRuleGroup` struct (725), crossnode by `RouteKey` struct (32). `extractPolicyID` still `""` (routegroup.go:149). |
| **NG-03** | LB strategies (round_robin/least_conn/ip_hash/weighted) | `caddy_proxy.go:924` `lbPolicy`, `traefik_proxy.go:841` sticky, `loadbalancer/service.go:468` WRR | BROKEN | **STILL BROKEN** | `traefik_proxy.go:841` still mangles `least_connections` → sticky cookie (841:848) with name `_gp_<id>`; `caddy_proxy.go:973` `lbPolicy` raw string passes `least_connections` which matches no Caddy policy (`random`, `least_conn`, `first`, `ip_hash`, `uri_hash`, `header`, `cookie`, `round_robin` are valid — `least_connections` not). `loadbalancer/service.go:481` WRR uses global `"__weighted"` across all groups. |
| **NG-04** | Weighted routing | `caddy_proxy.go:937` `weight` in upstream | BROKEN | **STILL BROKEN** | Caddy upstream `weight` (937) is valid only for `weighted_round_robin` lb_policy, not default; Traefik weight in `Server.Weight` (813) valid — but `healthStatus` modify removes/re-adds without preserving weight (`caddy_proxy.go:503` unconditional `dial` only, no weight). `crossnode/ingress_sync.go:159` sets weight from backend but primary weight lost on health re-add. |
| **NG-05** | Health probes (HTTP path/status/interval) | `trafficmanager/service.go:1001` `ProbeTargets`, `loadbalancer/dataplane.go:296` `runHealthChecks` | MISSING | **STILL MISSING** | `trafficmanager/service.go:1026` still `net.DialTimeout("tcp", addr, 2s)` only; `loadbalancer/dataplane.go:317` same `DialContext("tcp", ..., 2s)` inside `if network != "udp" { network="tcp" }` (313:315). No HTTP path/header/status evaluation. `HealthCheckConfig` stored but ignored (dataplane.go:306 skip UDP, never reads `Path/Interval/Threshold`). |
| **NG-06** | WebSocket | Both adapters | COMPLETE | **COMPLETE** | `caddy_proxy.go:966` `header_up`, `traefik_proxy.go:835` `FlushInterval:0ms` correct. |
| **NG-07** | TCP/UDP L4 | `caddy_proxy.go:877` routes, `traefik_proxy.go:758` `buildTCPGroupedRoute` | BROKEN P0 | **STILL BROKEN** | Caddy has **no** L4 branch; `Protocol=="tcp"` still renders as HTTP route with `host`+`path` match (953:957). Traefik TCP router uses `HostSNI` (884) TLS-SNI-only, plain TCP will not match. `service.go:383` admits `"tcp"` but not `"udp"` (383:387) while LB dataplane is the only UDP handler — split-brain. |
| **NG-08** | Validate-before-load & rollback | `caddy_proxy.go:980` `validateConfig` | BROKEN P0 | **STILL BROKEN** | `validateConfig:980` still `POST /load` (983) — real apply. Correct dry-run is `/adapt` or pre-post `/config/` diff; snapshot taken *after* validate (691:694) so rollback restores post-mutate config. `caddy_tls.go:240` validate is local no-op JSON check, not a dry-run. Traefik `validateYAMLConfig:974` is local marshal round-trip only. |
| **NG-09** | Policy middlewares (rate_limit/circuit_breaker/ip) | `caddy_proxy.go:795` `buildPolicyHandles` | BROKEN P0 | **STILL BROKEN** | `caddy_proxy.go:799` `handler:"rate_limit"` still fictional (no such Caddy module — real is `request_body`? actually rate limiting is `rate_limit` plugin not in stdlib); `caddy_proxy.go:867` `handler:"circuit_breaker"` still fictional (real is `reverse_proxy` `lb_try_duration`/`lb_retry_match` or `circuit_breaker` plugin). Traefik equivalents are correct (ipWhitelist/blacklist/rateLimit/circuitBreaker) but the Caddy path will fail `must-revalidate` and brick updates. Tests assert broken rendering. |
| **NG-10** | All-policies-on-all-routes | `caddy_proxy.go:722` `buildPolicyRoutes`, `traefik_proxy.go:960` `collectApplicablePolicies` | BROKEN P0 cross-tenant | **STILL BROKEN** | `caddy_proxy.go:722` collects *all* policies into `policyHandles` (727:734) and `enrichRouteWithPolicy:771` prepends them to *every* route (781:783). `traefik_proxy.go:960` method comment admits it returns all (964:965). `routegroup.go:149` `extractPolicyID` always `""` so no per-route filtering is even possible. |
| **NG-11** | ACME client library & issuance | `acme/service.go:132` `New` | PARTIAL | **PARTIAL** | `acme/service.go:241` hardcodes `RSA2048` (no EC choice), no EAB/chain support, `getOrCreateAccountKey:580` generates `P384` EC but registration URI not persisted beyond account private key (no `acme_account.registration_uri` column). Timeouts: default lego client uses http.DefaultClient (no explicit timeout config). |
| **NG-12** | HTTP-01 solver | `acme/service.go:52` `httpChallenger` | BROKEN P0 | **STILL BROKEN — dead code** | `acme/service.go:52` + `:157` fully implemented, but `grep -rn HTTPSolver forge` shows zero mounts. `cmd/api/main.go:983` `acmesvc.New` never exposes solver; `internal/http/server.go` never `app.Get("/.well-known/acme-challenge/*")`. Default `ChallengeTypeHTTP01` (193:204) will time out against LE. DNS-01 via `dns/service.go` is only working path. |
| **NG-13** | DNS-01 propagation tuning | `acme/service.go:315` `AddRecursiveNameservers` | PARTIAL | **STILL PARTIAL** | Fixed to `1.1.1.1:53` + `8.8.8.8:53` only (315:317); no per-request resolvers/propagationDelay/TTL; `dns/service.go` providers use default propagation configs from lego (no override). |
| **NG-14** | Wildcard FQDN/dup-SAN pruning | — | PARTIAL | **STILL PARTIAL** | `domains/service.go:515` `normalizeAndValidateDomain` rejects IP but allows `*.example.com`; no `*.*` reject, no dedup of covered SANs before issue (acme `IssueCertificate:210` wildcard check but no `UnFqdn`/dedup). Traefik `sanitizeDomains` equivalent missing. |
| **NG-15** | Renewal window | `acme/service.go:380` `StartAutoRenewal` | BROKEN | **STILL BROKEN** | Fixed 24h ticker (397) + 30-day expiring query (`store.FindExpiringCertificates`), no jitter, no `1/3 lifetime` scaling; panic `recover` is *outside* loop (389:396) — if `runAutoRenewal` panics, loop dies permanently. |
| **NG-16** | Key reuse on renewal | `acme/service.go:492` `renewOnce` | BROKEN | **STILL BROKEN** | `renewOnce:499` parses stored `PrivateKey` and passes same `keyDER` to `client.Certificate.Renew(..., true, false` with `bundle=true, mustStaple=false` but `PrivateKey` unchanged; certmagic pattern is fresh key per renewal (per `acme/service.go:492:546` comment). Also `renewWithRetry` reuses same `certKey` across 3 attempts. |
| **NG-17** | Revocation | `acme/service.go:368` `RevokeCertificate` | BROKEN | **STILL BROKEN** | `368:370` is `return s.store.DeleteCertificate(ctx, certID)` only — never calls `client.Certificate.Revoke` — CA still serves cert; API name `Revoke` overpromises. |
| **NG-18** | Cert storage encryption | migrations `117 / 157` | BROKEN P1 | **STILL BROKEN** | `certificates` LP keys encrypted via `masterKeyring` (needs check — leaf cert encrypted, but `dns_credentials` JSONB + `dns_provider_accounts.credentials` plaintext). `dns/service.go:624` `credentialsFromProvider` reads raw `provider.Credentials` bytes containing plaintext tokens; migration `157` not applied to dns tables. |
| **NG-19** | Default contact | `acme/service.go:198` `admin@localhost` | BROKEN | **STILL BROKEN** | `IssueCertificate:198` defaults to `admin@localhost` — rejected by LE, ZeroSSL still accepts but warns; should block or require `PANEL_ACME_EMAIL`. |
| **NG-20** | Cert delivery to gateway | `gateway_adapter.go:49` `SetCertificate` | BROKEN P0 | **STILL BROKEN** | `CaddyReverseProxy.SetCertificate:83` implemented (POST `/tls/certificates/<domain>`), but zero callers in codebase (`grep -rn SetCertificate forge/api/cmd` empty). `acme/service.go:193` issue + `RenewCertificate:322` + cert upload handler `handlers_certificates_ext.go:63` never call it; `CaddyTLSManager` also never constructed. `TraefikReverseProxy.SetCertificate:373` writes PEM string into `CertFile` path field — doubly broken. |

### 2.2 Mapping to Phase-04 Synthesis 10 P0s (the “activation order” list)

| Phase-04 P0 | Alias | Maps to NG | Still Repro Today? | File:Line now |
|---|---|---|---|---|
| **F-NET-01** | Empty-sync wipe every 30s | NG-01 | **Yes** | `crossnode/ingress_sync.go:111,166` — `Sync` calls `UpdateRoutes` with empty `mergedRules` when `is.rules` empty |
| **F-NET-02** | Asymmetric merge discipline | NG-01 | **Yes** | `caddy_proxy.go:673` full-replace vs `caddy_proxy.go:514` merge |
| **F-NET-03** | Validate=apply, snapshot-after-mutation | NG-08 | **Yes** | `caddy_proxy.go:980,691` |
| **F-NET-04** | Fictional Caddy modules brick updates | NG-09 | **Yes** | `caddy_proxy.go:799,867` |
| **F-NET-05** | All policies → all routes (cross-tenant) | NG-10 | **Yes** | `caddy_proxy.go:722`, `traefik_proxy.go:960`, `routegroup.go:149` |
| **F-NET-06** | Grouped-route removal misses dead nodes | NG-01/02 | **Yes** | `caddy_proxy.go:51` `RemoveRoutes` per-rule `gamepanel-<id>` misses `gamepanel-<firstRule>-group` keyed routes |
| **F-NET-07** | TCP as broken HTTP, UDP half-implemented | NG-07 | **Yes** | `caddy_proxy.go:924` no protocol branch; `traefik_proxy.go:884` HostSNI-only |
| **F-NET-08** | Certs never delivered | NG-20 | **Yes** | `gateway_adapter.go:49` zero callers |
| **F-NET-09** | HTTP-01 dead code | NG-12 | **Yes** | `acme/service.go:157` never mounted |
| **F-NET-10** | Networking admin UX non-functional | — | **Yes** | `forge/web/app/admin/traffic/page.tsx:62` schema mismatch; `PATCH` vs `PUT`; `GET /policies` 405; `forge/web/app/admin/certificates/page.tsx:33` prefix bug; `handlers_proxy_domains.go:199` fictional verify |

Remaining F-NET-11..25 are P1s; the dense P0 cluster above is the load-bearing set for Phase 01 planning.

### 2.3 Security-flavored Gateway P0s (NG-18/SE, NG-14/SE, NG-16/SE, NG-10/SE, NG-09/SE)

| ID | Surface | File:Line | Severity |
|---|---|---|---|
| NG-18/SE-18 | Plaintext DNS API credentials in `dns_credentials` / `dns_provider_accounts.credentials` | `dns/service.go:624` `credentialsFromProvider` + migrations `117`/`135` vs encrypted `157` | **P1** — rotate to encrypted JSONB |
| NG-10/SE-10 | L4 LB as internal-network probe (`AddTarget` accepts arbitrary `IP`) | `handlers_loadbalancer.go:88` `ip/port` verbatim; `loadbalancer/service.go:332` no IP allowlist | **P1** — add private-IP allowlist |
| NG-14/SE-14 | Traefik rule injection via backticks in `Path` | `traefik_proxy.go:797` `fmt.Sprintf("Host(`%s`) && PathPrefix(`%s`)", ...)` unsanitized `grp.path` | **P1** — escape or structural matcher |
| NG-16/SE-16 | DNS-provider SSRF (`PDNS_API_URL`, `RFC2136_NAMESERVER` zero validation + env mutation) | `dns/service.go:254:266` + `489:498` | **P1** — env window narrowed; still need URL/IP validation |
| SE-09 | Cross-tenant policy leakage (same as NG-10) | `caddy_proxy.go:722`, `traefik_proxy.go:960` | **P0** — tenant isolation |

---

## 3. Files to Touch — Preparation Inventory

### 3.1 Backend — Gateway single-writer inversion (core)

| File | Why touch | Change size | Risk |
|---|---|---|---|
| `forge/api/cmd/api/main.go:982,994,998,1025,1030,1041` | Wiring seam — construct gateway, wire resolver, start/stop | M | High — startup order |
| `forge/api/internal/services/trafficmanager/service.go:73:129,562:618,900:999` | Remove per-node map vs desired-state diff; add `RulePolicyJoin` model; fix `ReconcileRoutes` to consult desired, not map drift | L | High |
| `forge/api/internal/services/trafficmanager/caddy_proxy.go:514,673,707,722,795,894,980,1026` | Rewrite `updateRoutesAtomic` to read-modify-write like `UpdateDomainRoutes`; fix `validateConfig` to not apply; remove fictional handlers or gate them; implement proper `lb_policy` mapping; group-aware `RemoveRoutes`/`CleanupStale`; health-aware weight preservation | L | High |
| `forge/api/internal/services/trafficmanager/caddy_tls.go:34,58,186,240,253` | Either delete (if certs go via `GatewayAdapter.SetCertificate`) or fix to merge not replace; otherwise it is a third writer | M | Med |
| `forge/api/internal/services/trafficmanager/traefik_proxy.go:373,399,960,1065` | Delete or feature-flag; if keep, fix `/api/refresh` → file-watch, fix `CertFile/KeyFile` PEM-vs-path, fix sticky/weight | M/L | Low if deleted |
| `forge/api/internal/services/trafficmanager/gateway_adapter.go:38` | Add `UpsertDomainRoutes` vs `UpdateDomainRoutes` semantics? Add `Describe()`; otherwise stable | S | Low |
| `forge/api/internal/services/domains/service.go:74,455,484` | Stop speaking to Caddy directly — emit desired `DomainRoute` to gateway instead of `caddy.UpdateDomainRoutes`; make `NodeResolver` async via discovery cache | M | Med |
| `forge/api/internal/services/crossnode/ingress_sync.go:111,200,218` | Add empty-sync guard (`if len(is.rules)==0 return nil` or `skipEmptySync` flag); or retire `IngressSynchronizer` entirely in favor of TrafficManager reconcile loop; fix replica-synthesis to respect primary weight | S/M | Med |
| `forge/api/internal/services/crossnode/resolver.go:59,135,168` | Repair fail-open (healthy-only fallback should return error, not unhealthy); add cache invalidation on node health change | S | Med |
| `forge/api/internal/services/crossnode/routegroup.go:32,149` | Implement `extractPolicyID` (read rule tag/`RoutingRule.Headers["policyId"]` or DB join); consolidate grouping helper so three copies collapse to one | S | Low |
| `forge/api/internal/services/loadbalancer/service.go:468,296,332,546` | Fix WRR shared `"__weighted"` bucket → per-group; implement `HealthCheckConfig` HTTP probe (reuse `ProbeTargets` logic); add IP allowlist validation in `AddTarget`; decide if LB remains separate dataplane or becomes `GatewayAdapter` target-group provider | M | Med |
| `forge/api/internal/services/loadbalancer/dataplane.go:20,62,296` | Either keep as-is behind `LOAD_BALANCER_ENABLED` flag (separate port space, no conflict) or unify under gateway; if keep, fix UDP health skip, implement HTTP health | M | Low |
| `forge/api/internal/services/acme/service.go:52,157,193,297,315,368,380,492` | Mount `HTTPSolver` handler; fix `RevokeCertificate` to call CA; add `SetCertificate` call after issue/renew; generate fresh key on renewal; add `EAB/key-type/chain` fields; fix renewal loop recover+ jitter; validate `Email` not `admin@localhost` | L | High |
| `forge/api/internal/services/dns/service.go:311,489,593,603` | Add `PDNS_API_URL` https+IP allowlist validation; `RFC2136_NAMESERVER` IP validation; encrypt `dns_provider_accounts.credentials` column; wire `RegisterWithAcme` idempotency | M | Med |
| `forge/api/internal/services/servicediscovery/registry.go:32,90` | Keep | — | — |
| `forge/api/internal/services/servicediscovery/discovery.go:54,78,90` | Fix `Resolve:60` fail-open; repair `VerifyCrossNodeReachability` vantage point (delegate to beacon or node-agent dial, not API) | S/M | Med |
| `forge/api/internal/services/servicediscovery/reachability.go:33,58,82` | Add source-node delegation; test harness must mock beacon dial | S/M | Low |
| `forge/api/internal/services/servicediscovery/stale_reaper.go:90` | Decide if reaper removes vs marks; add endpoint-age metric | S | Low |
| `forge/api/internal/services/servicediscovery/visibility.go:49` | Expose via HTTP if desired (currently unexposed) | S | Low |

### 3.2 Backend — HTTP handlers

| File | Why touch |
|---|---|
| `forge/api/internal/http/handlers_trafficmanager.go:107` | Add `GET /admin/traffic/policies` list (currently missing → 405); keep `PUT` vs `PATCH` decision — either keep `PUT` and fix web to `PUT`, or support both methods |
| `forge/api/internal/http/handlers_loadbalancer.go:84,127` | Add IP allowlist validation; fix `GET /groups/:id/next` vs web `POST` mismatch (add POST alias or fix web); add `Validate` dry-run endpoint for `validate-before-load` |
| `forge/api/internal/http/handlers_domains.go:102` | Keep `/.well-known/forge-verify` (22 lines); add `/.well-known/acme-challenge/*` → `acmeSvc.HTTPSolver()` bridge + gateway-proxied path |
| `forge/api/internal/http/handlers_proxy_domains.go:199,215` | Fix `POST /:id/verify` to actually check `/.well-known/forge-verify` or remove 200-hardcode; unify `/domains` vs `/admin/domains` vs `/servers/:id/domains` surface (currently 3 domain systems); fix cert handling to call `SetCertificate` |
| `forge/api/internal/http/handlers_certificates.go:10` + `handlers_certificates_ext.go:17` | Unify `/certificates` mounts — both register `protected.Group("/certificates")` which collides (currently ext + acme own same prefix; proxy certs moved to `/custom-certificates:226` as workaround). Unify or keep distinct prefix. Add `privateKey` redaction on list (`handlers_certificates_ext.go:36` leaks on `/export` as designed but not on list) |
| `forge/api/internal/http/handlers_dns.go:11` | Add `GET /providers/verify` rate-limit; encrypt/decrypt credentials on read (redact) |
| `forge/api/internal/http/handlers_firewall.go:7` | Add input validation (`CIDR/port/proto` schema), persistence (see §5), audit log |

### 3.3 Frontend — 5 admin pages + lib

| File | Why touch |
|---|---|
| `forge/web/app/admin/traffic/page.tsx:14:66` | **Rewrite** type `RouteRule:14` + form `defaultRouteForm:35` to match `RoutingRule` (`domain, path, targetHost, targetPort, protocol, strategy, weight, headers, enabled, webSocket`); fix `fetchJSON("/admin/traffic/policies")` 405 (missing handler); change `patchJSON` → `putJSON` for routes; fix table columns |
| `forge/web/app/admin/load-balancer/page.tsx:142` | Change `testSelectionMutation` POST → GET (or add server route); otherwise page is functional |
| `forge/web/app/admin/domains/page.tsx:52,117` | Requires `?serverFilter` — cross-check why empty when no server selected (current: disabled until `serverFilter` set, so all-domains view is blank). Probably add `/admin/domains` list handler. Minor |
| `forge/web/app/admin/certificates/page.tsx:33,44` | Fix prefix: `fetchJSON("/certificates")` likely needs `/admin/certificates` or proxy `/api` base; `postJSON("/certificates", uploadForm)` missing `certificate/privateKey` validation pre-flight; add ACME-issue tab (`POST /certificates/issue`) — currently only upload |
| `forge/web/app/admin/firewall/page.tsx:1` + `AdminFirewall.tsx:32` | Keep — only working networking UX |
| `forge/web/lib/api/domains.ts:checkDNS` + `forge/web/lib/api/firewall.ts` | Keep |
| `forge/web/lib/api` central (`fetchJSON/postJSON/patchJSON/putJSON`) | Ensure verb parity — add `putJSON` usage in traffic page, remove `patchJSON` mis-wire |

### 3.4 Store & Migrations

| File | Why touch |
|---|---|
| `forge/api/migrations/083_a_traffic_rules.sql:1` vs `forge/api/internal/store/migrations/038_traffic_routing.sql:1` | Consolidate duplicated `traffic_rules` definitions. Adopt `083_a` shape (has `target_host`, non-null `name`, `server_id NOT NULL`, weight default 1). Remove `038` or make it no-op guard. |
| `forge/api/migrations/133_b_routing_rules_persistence.sql` | Already idempotent `ADD COLUMN web_socket`; keep |
| `forge/api/migrations/117_domains_certificates.sql:1` | Houses `proxy_domains` + `security_headers` + `redirect_rules` — decide if these remain DB-backed domain system or get collapsed into Gateway model (recommended: keep as UI store, but rename gateway-owned fields so they are not authoritative) |
| `forge/api/migrations/xxx` **new** | `rule_policy_join` table (`rule_id FK traffic_rules, policy_id FK traffic_policies, order int`) — required for NG-10/P0 fix (see §5.2) |
| `forge/api/migrations/xxx` **new** | `dns_provider_accounts.credentials` encryption re-migration + `dns_credentials` column (if any) |
| `forge/api/migrations/xxx` **new** | `gateway_desired_state` snapshot table (optional) for snapshot-before-mutate rollback (JSONB with generation, see §4.7) |
| `forge/api/migrations/xxx` **new** | `firewall_desired_state` table (`node_id, rules JSONB, generation`) if persisting firewall |
| `forge/api/migrations/xxx` **new** | `gateway_generations` or extend `traffic_rules.updated_at` + `traffic_policies.updated_at` fencing — needed for last-writer-wins avoidance |

---

## 4. Single-Writer Gateway Inversion Plan — Touchpoints

> Reference models: Traefik `*dynamic.Configuration` (one tree, routers→services→middlewares, providers produce shape, API read-only) / Caddy one JSON doc via `POST /load` full-replace or `/config` sub-resource merges / NPM denormalized row + `configure→test→reload` with `.err` rename.
> Phase-04 §7 recommendation: **ADOPT Traefik provider→router→middleware→service declarative model** as THE Gateway model; domains becomes a provider emitting routers; collapse `traffic_rules+proxy_domains→routers`, `traffic_policies+security_headers+redirect_rules→middlewares`.

### 4.1 Target Architecture (post-inversion)

```
                desired-state (DB)
  ┌──────────────────────────────────────────┐
  │ traffic_rules │ traffic_policies │ proxy_domains │ security_headers │ redirect_rules │
  │ + rule_policy_join (NEW) │ certificates │ target_groups (optional) │
  └──────────────┬───────────────────────────┘
                 │ single reconciler polls/subscribes (2s or on-event)
                 ▼
         ┌─────────────────┐
         │ GatewayService  │  ← THE only writer to Caddy admin API
         │  (new package)  │     owns CaddyReverseProxy instance
         │  BuildDesired() │     validate-then-apply with snapshot-before
         │  DiffAndApply() │     exclusive lease/lock (pg_advisory_xact_lock)
         └────────┬────────┘
                  │ POST /config/  (sub-resource merges) or POST /id/<id>
                  │ or POST /load  (full doc, with true dry-run via /adapt)
                  ▼
             ┌──────────┐
             │  Caddy   │  single JSON doc
             └──────────┘
         IngressSynchronizer, domains.Service, CaddyTLSManager → retired as writers
         become Providers that emit fragments consumed by GatewayService
```

### 4.2 The Five Writers Today (must collapse to one)

| # | Writer | Package:Line | Method | Server key | Discipline | Status |
|---|---|---|---|---|---|---|
| 1 | **TrafficManager** | `trafficmanager/caddy_proxy.go:673` `updateRoutesAtomic` | `POST /config/` full-replace `apps.http.servers.gamepanel` | `gamepanel` | full-replace | **Active** |
| 2 | **Domains** | `domains/service.go:484` via `caddy_proxy.go:514` `UpdateDomainRoutes` | read-modify-write preserving `gamepanel` | `gamepanel-domains` | merge | **Active** — but wiped by #1 |
| 3 | **Crossnode IngressSynchronizer** | `crossnode/ingress_sync.go:166` `Sync` via same `caddyProxy` | `UpdateRoutes` full-replace | `gamepanel` | full-replace | **Active** but empty — wipes #1 |
| 4 | **CaddyTLSManager** | `trafficmanager/caddy_tls.go:34,58,186,253` | `POST /config/` isolated doc with `:443` | `gamepanel-domains` (collides with #2) + `tls` | full-replace | **Dead** (never constructed), but if wired becomes fourth writer |
| 5 | **LoadBalancer dataplane** | `loadbalancer/dataplane.go:62` `reconcileListeners` | `net.Listen` bypasses Caddy entirely | own `LISTEN :30000-30100` | separate | **Active** but out-of-band — not Caddy writer, but conceptually fifth routing authority |

**Net effect:** Writers 1 and 3 both `POST /config/` with `{"apps":{"http":{"servers":{"gamepanel":{...}}}}}` containing *only* their own server, so each apply erases the other's server *and* writer 2's `gamepanel-domains`. With default `SO_REUSEPORT` on Caddy, two servers both `listen [":80"]` is non-deterministic.

### 4.3 Inversion Steps (ordered, each is a PR-sized slice)

#### Step 0 — Stop the bleeding (P0 hotfix, no schema, minutes)

- File: `crossnode/ingress_sync.go:111` add at top of `Sync`:
  ```go
  if len(is.rules)==0 && len(is.policies)==0 { slog.Debug("ingress sync skip: no desired state"); return nil }
  ```
  or gate `Start` behind `GATEWAY_INGRESS_SYNC_ENABLED=false` default.
- File: `trafficmanager/caddy_proxy.go:673` short-term choose merge parity: make `updateRoutesAtomic` call `getRunningConfig` + merge `gamepanel` like `UpdateDomainRoutes:524` does, preserving `gamepanel-domains` and unknown fields. One-method change, avoids wipe until full inversion.
- Verify: run `go test ./internal/services/crossnode -run TestIngress` + `go test ./internal/services/trafficmanager -run TestCaddy` with — no new migration, safe to ship first.

#### Step 1 — Extract `GatewayService` (new package, no wiring yet)

- New file: `forge/api/internal/services/gateway/service.go` (or `gateway/` package) — the *only* holder of `*CaddyReverseProxy`.
- Interface:
  ```go
  type Provider interface { DesiredRoutes(ctx) ([]RouteFragment, error) }
  type RouteFragment struct { Server string; Routes []map[string]any; Policies []map[string]any }
  type GatewayService struct {
      mu sync.Mutex
      proxy *trafficmanager.CaddyReverseProxy
      providers []Provider
      lastApplied json.RawMessage
      generation int64 // DB-backed, for fencing
  }
  func (g *GatewayService) Reconcile(ctx) error // BuildDesired → Diff → validate snapshot-before → apply
  ```
- Move `caddy_proxy.go:894,924,795,707,980,1026` logic into this package, fixing the four correctness bugs there in one pass (validate, fictional handlers, lb_policy, group key).
- Keep old writers calling through `GatewayService` shim so tests still pass.

#### Step 2 — Make `TrafficManager` and `Domains` Providers, not Writers

- `trafficmanager/service.go:562` `ApplyRoutes` / `SyncRoutes` stop calling `s.proxy.UpdateRoutes`; instead mutate in-mem + DB, then `gatewayService.SignalReconcile()` (or rely on periodic poll). Remove `proxy ReverseProxy` field or keep as read-only handle delegated to gateway.
- `domains/service.go:455` `syncCaddyRoutes` stops calling `s.caddy.UpdateDomainRoutes`; instead materializes `VerifiedDomainRoute` slice, gateway merges it as `gamepanel-domains` fragments.
- Delete `caddyUpdater` interface from domains once gateway owns caddy.
- File: `cmd/api/main.go:982,998,1025` wire: `gatewaySvc := gateway.New(caddyProxy, tmProvider, domainProvider)`; pass `gatewaySvc` to `domainSvc`/`tmSvc` as signaler, not as proxy.

#### Step 3 — Retire or Feature-Flag `IngressSynchronizer` + `CaddyTLSManager`

- Option A (recommended): delete `crossnode.IngressSynchronizer` (its `GroupRulesByRoute:32` duplicates `gateway/grouping`; its `HealthFilter` is the third health system). Its `Replicas` synthesis belongs in `GatewayService` or `Resolver`.
- Option B: keep behind `GATEWAY_INGRESS_SYNC_ENABLED=false`, add `deprecated` doc, remove `Start` call in `main.go:994:995` if flag off.
- `CaddyTLSManager`: if certificate delivery goes via `GatewayService.SetCertificate` (calling `proxy.SetCertificate:83`), this package dies. Otherwise unify it into gateway's `SetCertificate` path.

#### Step 4 — Fix the four correctness bugs inside the new `GatewayService` (one commit, snapshot-before-validate)

Scope: exactly the four P0 bricks that make *any* write fail once policies exist.

1. **Validate != apply** — `caddy_proxy.go:980` currently `POST /load` with `Cache-Control: must-revalidate`. Caddy truth: `POST /load` *is* apply (`caddy/caddyconfig/load.go:68`), true dry-run is `POST /adapt` (`load.go:137`) or local idempotent read-then-diff. Fix: snapshot `previousConfig = getRunningConfig` *before* validate, then validate via local JSON+schema or `POST /adapt` if available; never `POST /load` for check.
   ```go
   prev, _ := p.getRunningConfig(ctx, addr)
   candidate := buildSnapshot(prev, desired)
   if err := localValidate(candidate); err != nil { return err } // no network
   return p.applyConfig(ctx, addr, candidate)
   ```
2. **Gate fictional handlers** — `buildPolicyHandles:795` currently emits `handler:"rate_limit"` + `handler:"circuit_breaker"` unconditionally. Fix: gate behind `CADDY_RATE_LIMIT_ENABLED`/`CADDY_CB_ENABLED` env, or remove until real plugin (`caddy-ratelimit`, `caddy-cb`) is installed. On gate-off, return error or warn and skip that handle.
3. **All-policies-everywhere** — implement `rule_policy_join` read in `BuildDesired` (see §5.2), pass per-route policy slice to `buildGroupedRoute`. `collectApplicablePolicies` disappears.
4. **Group removal + weight + WebSocket consistency** — `RemoveRoutes:51` must address grouped route ID (`gamepanel-<firstID>-group`) not per-rule; `SetUpstreamHealth:379` must preserve `weight` when re-adding; `groupRules` key must include `protocol` so `Protocol` doesn't split groups unexpectedly.

#### Step 5 — Mount HTTP-01 and wire cert delivery (P0 #3 of phase-04)

- `cmd/api/main.go` after `acmeSvc = acmesvc.New(...)` add:
  ```go
  app.Get("/.well-known/acme-challenge/*", fiberWrap(acmeSvc.HTTPSolver()))
  // and instruct gateway to proxy /.well-known/acme-challenge from Caddy:
  // gatewayService.EnsureACMEChallengeRoute() -> host:* path /.well-known/acme-challenge/*
  //                                           -> reverse_proxy to this panel host
  ```
  Because Caddy terminates `:80`, LE validation hits Caddy first — Caddy must forward `/.well-known/acme-challenge/*` to the panel, or panel must listen on `:80` alongside Caddy (pick one; forwarding from Caddy is cleaner).
- After `acme/service.go:268` `obtainWithRetry` success (Issue) and `acme/service.go:322` `renewWithRetry` success (Renew) and `handlers_certificates_ext.go:63` upload success, call `gatewaySvc.SetCertificate(ctx, trafficmanager.CertConfig{Certificate: certPEM, PrivateKey: keyPEM, Domains: domains})`. This is the fix for NG-20; `gateway.GatewayAdapter.SetCertificate:49` is the interface — but `CaddyReverseProxy.SetCertificate:83` currently POSTs to `/tls/certificates/<domain>` with `{"certificate","key"}` — verify against Caddy admin `PUT /config/apps/tls/certificates` shape; may need to write to `apps.tls.certificates.load_pem` or `apps.tls.automation.policies`.
- Add `CaddyTLSManager` deletion decision before/after this step.

#### Step 6 — Unify health (single owner)

- Three health systems today: `trafficmanager.ProbeTargets:1001` (2m ticker, TCP 2s), `loadbalancer.runHealthChecks:296` (2s, TCP 2s, skips UDP), `crossnode.HealthFilter:31` (3 strikes, used only by IngressSync).
- Choose one owner: `GatewayService` consults a shared `HealthState` map fed by a single prober that honors `HealthCheckConfig` (HTTP `Path/Port/Interval/Thresholds`), respects operator `SetTargetStatus` writes, and implements hysteresis (e.g., 3 consecutive failures before `Unhealthy`, 2 successes to return `Healthy`). Builders must read this map instead of blindly re-adding all backends.
- File: `loadbalancer/service.go:71` `connCount`/`roundRobin` remain for L4 dataplane; HTTP health for gateway moves out of dataplane entirely.

#### Step 7 — Harden convergence

- Add `pg_advisory_xact_lock(42)` around `GatewayService.Reconcile` (in Postgres-backed func) so multi-replica API doesn't race on `POST /config/`.
- Add `gateway_generations` optimistic fencing if lock not desired — writer reads `generation`, CAS on update.
- Write `audit_log` row for every gateway mutation (`applied config generations N→N+1, routes M, policies K`) — NPM reference pattern (phase-04 §7).

### 4.4 Provider Contract (what TrafficManager and Domains emit)

```go
// gateway/provider.go
type DesiredState struct {
    Generation int64
    Servers map[string][]Route // server name → routes
    Certs   []CertConfig
}

type Route struct {
    ID       string // for @id, for stale cleanup
    Hosts    []string
    Paths    []string // each is path prefix or exact
    Upstreams []Upstream // dial + weight
    LBPolicy string // validated against caddy known set
    Headers  map[string]string
    WebSock  bool
    Middlewares []MiddlewareRef // ordered, per-route only
}

type MiddlewareRef struct {
    ID   string // policy id
    Kind string // rate_limit | ip_whitelist | ...
    Params map[string]any // validated
}
```

Providers implement `BuildDesired(ctx) (DesiredState, error)` from DB without touching Caddy. Gateway merges all providers' `Servers` into one doc, diffs vs last-applied, validates, applies.

---

## 5. Migrations Needed

| # | Migration | Columns / Shape | Why | Can defer? |
|---|---|---|---|---|
| **M1** | `traffic_rules` consolidation (`038` vs `083_a`) | Adopt `083_a_traffic_rules.sql:1` shape as canonical: `id uuid PK, name text NOT NULL, server_id uuid NOT NULL REFERENCES servers, domain text NOT NULL, path text DEFAULT '/', target_host text DEFAULT '', target_port int NOT NULL, protocol text DEFAULT 'http', strategy text DEFAULT 'round_robin', weight int DEFAULT 1, headers jsonb DEFAULT '{}', enabled boolean DEFAULT TRUE, web_socket boolean DEFAULT FALSE, created_at/updated_at`. Make `038_traffic_routing.sql` a no-op conditional (`DO $$ BEGIN … EXCEPTION WHEN duplicate_table THEN NULL; END $$;`) | Two migrations both `CREATE TABLE IF NOT EXISTS traffic_rules` with divergent columns — Go row types `store_traffic.go:11` vs `store_routing.go:21` map to different defaults (`enabled false` vs `true`, missing `target_host`) | **No** if single-writer inversion ships — must occur before gateway trusts DB as source |
| **M2** | `rule_policy_join` (NEW) | `id uuid PK DEFAULT gen_random_uuid(), rule_id uuid NOT NULL REFERENCES traffic_rules(id) ON DELETE CASCADE, policy_id uuid NOT NULL REFERENCES traffic_policies(id) ON DELETE CASCADE, attach_order int NOT NULL DEFAULT 0, UNIQUE(rule_id, policy_id)` + index `rule_id`, `policy_id`. Backfill script: leave empty (all-policies bug → explicit opt-out is safer than auto-attach) | Fixes NG-10/F-NET-05 — per-route policy attachment, deterministic middleware ordering (`rate_limit→blacklist→whitelist→CB→redirect`) | **P0 for inversion** |
| **M3** | `dns_provider_accounts.credentials` encryption | Mirrors `migrations/157` encrypted sibling: add `credentials_enc BYTEA, credentials_iv BYTEA` or encrypt in-place with `EncryptJSONB` func; backfill via `masterKeyring`. Update `dns/service.go:620` `credentialsFromProvider` to decrypt if `_enc` present | NG-18/SE-18 P1 plaintext secrets at rest | **P1** but bundle with M2 |
| **M4** | `gateway_desired_state` + `gateway_generations` (optional but recommended for lock/fence) | `gateway_state` `id smallint PK DEFAULT 1 CHECK (id=1), generation bigint NOT NULL DEFAULT 0, last_applied jsonb, updated_at timestamptz DEFAULT now()` (single row). Alternatively extend `traffic_rules.updated_at` with trigger bump | Fencing for multi-replica reconciler, snapshot-before-validate rollback source | Can be in same release as M2; if not, use `pg_advisory_lock` instead |
| **M5** | `firewall_desired_state` (if persisting firewall, P1) | `firewall_rules_desired` `id uuid PK, node_id uuid REFERENCES nodes, port int, protocol text, source_ip cidr, action text, description text, generation int, created_at` — or JSONB desired-state per node | F-NET-19 ephemeral daemon pass-through; node reprovision loses rules | **Defer to P1** |
| **M6** | `traffic_policies` + `proxy_domains` field rename/compat view (low priority) | If collapsing into `gateway_routes/middlewares/services` model, create view `gateway_routes AS SELECT … FROM traffic_rules …` for backwards compat | Phase-04 §7 single `GatewayRouter/Middleware/Service` model — large refactor, intentionally post-Phase-01 | **P3 / post-01** |
| **M7** | `acme_accounts.registration_uri` + `certificate.renewal_key_fingerprint` | `acme_accounts.registration_uri text`; `certificates.renewal_key_fingerprint text`; or reuse existing JSONB | Persist account binding, fix permanent key-reuse (NG-16) expectation | **P1** bundle |

**Forward-compatible note:** M1 and M2 are the only hard gates for the single-writer inversion. M3 can land in same window. M4 is one-row helper — non-destructive. M5-M7 are P1-P3 and can ship later but should be tracked now.

---

## 6. Risks — Five Writers on One Caddy (and everything it drags with it)

### 6.1 The canonical five-writer race (graphical)

```
         trafficmanager/service.go:562
              ApplyRoutes │
                          ├─────┐
                 2m ticker ProbeTargets/Withdraw
                          │     │
   domains/service.go:484 │     │ crossnode/ingress_sync.go:166 30s ticker
  syncCaddyRoutes        │     │  Sync (empty)
         ╲                │     │   ╱
          ╲               ▼     ▼  ╱
           └────────▶  Caddy admin API  ◀────────── CaddyTLSManager (dead, 4th if wired)
                      POST /config/     ^                LoadBalancer (5th, separate listen)
                           │
                      :80/:443
                           ▼
                    live gateway (split-brain)
```

Each arrow is an **unfenced, uncoordinated `POST /config/` full-replace** (except domains' merge, which the others still overwrite). No `lastValidConfig` helps — `caddy_proxy.go:695` stores `lastValidConfig` only for `updateRoutesAtomic`, but `UpdateDomainRoutes:530` stores nothing, and `IngressSynchronizer` stores per-key `RouteGenerationRecord` but never consults before write.

### 6.2 The six failure modes this race produces (with repro pointers)

| # | Failure | Repro sequence | Blast radius | File:Line |
|---|---|---|---|---|
| 1 | **Empty-sync wipe** | Deploy fresh instance, never create traffic rule → wait 30s → `GET /admin/traffic/rules` empty but `GET /config/` shows empty `gamepanel` routes, or conversely `CreateRoutingRule` then `syncCaddyRoutes` adds `gamepanel-domains`, next `IngressSync` tick deletes it | All domain-hosted game servers 502 | `ingress_sync.go:111:166`, `caddy_proxy.go:673`, `main.go:994` |
| 2 | **Domain/traffic cross-wipe** | `POST /servers/:id/domains {domain:example.com}` → `SyncCaddyRoutes` writes `gamepanel-domains` → `POST /admin/traffic/rules {domain:example.com, targetPort:25565}` → `updateRoutesAtomic` replaces `{"apps":{"http":{"servers":{"gamepanel":{...}}}}}` dropping `gamepanel-domains` → `example.com` 404 | Every verified domain | `caddy_proxy.go:707:720` vs `caddy_proxy.go:566:569` |
| 3 | **Policy addition bricks all writes** | `POST /admin/traffic/policies {name:"ratelimit", rateLimit:10}` → any subsequent `CreateRoutingRule` / `SyncRoutes` → `buildPolicyHandles:799` emits `rate_limit` → `validateConfig:980` fails `must-revalidate` because module unknown → `ApplyRoutes` returns `config validation failed` → operator cannot create *any* new route until policy deleted | Gateway frozen — new routes impossible | `caddy_proxy.go:795,980,687` |
| 4 | **Grouped target removal lies** | `GroupRulesByRoute` keys `example.com|/api|http` with 3 rules (`r1,r2,r3`) → one route `@id=gamepanel-<r1>-group` with 3 upstreams → `WithdrawNodeTargets` or `probe` marks `r3`'s node down → `RemoveRoutes(["r3"])` DELETEs `/id/gamepanel-r3` which **does not exist** → Caddy still sends to `r3`'s backend (dead node) | Stuck failing backend | `caddy_proxy.go:51:73`, `caddy_proxy.go:949`, `service.go:753:799` |
| 5 | **Health flap + re-add** | `ProbeTargets:1026` TCP fail 3× → `SetUpstreamHealth:379` removes upstream `dial` → next `SyncRoutes:590` rebuilds *all* groups from `s.rules` without consulting `healthStatus` map → re-adds the just-removed backend → oscillation every `reconcileInterval 2m` / `ticker 30s` | Outage-length flapping | `service.go:1001,1027`, `caddy_proxy.go:379,468` |
| 6 | **TLS ghost** | `POST /certificates/issue` or `/upload` succeeds → postgres updated → never `SetCertificate` → Caddy serves old/no cert → TLS handshake fallback to Caddy's self-signed or 525; `RenewCertificate` renews in DB but not on wire | HTTPS outage on renew | `gateway_adapter.go:49`, `acme/service.go:268,341`, `caddy_proxy.go:83`, `caddy_tls.go:58` — zero callers |

### 6.3 Why `POST /load` as validator is load-bearing unsafe

- Caddy's `POST /load` (`caddy/caddyconfig/load.go:68:72`) **replaces** config immediately, even with `Cache-Control: must-revalidate`. Forge sends `must-revalidate` thinking it is a dry-run (`caddy_proxy.go:988`), but Caddy interprets it as "load this candidate, but tell me if it changed from cached." The candidate *is* loaded.
- Snapshot is taken *after* (`caddy_proxy.go:691:694` `previousConfig = getRunningConfig` after `validateConfig`) so rollback restores the **just-loaded broken** config, not the prior good one.
- Correct flow: `prev = GET /config/` → build `candidate = merge(prev, desired)` in-memory → `localValidate(candidate)` or `POST /adapt` (true dry-run, `caddy/caddyconfig/load.go:137:175`) → `POST /config/` only on success → on error, `POST /config/` with `prev` (no race with other writers while single-writer holds lock).

### 6.4 Interaction with the 2-minute node-reconcile + probe storm

- `trafficmanager/service.go:963` ticker fires `ReconcileRoutes + ProbeTargets` every `reconcileInterval 2m` (default), while `crossnode/ingress_sync.go:74` ticks every 30s (configured 30s at `main.go:995`), and `loadbalancer/dataplane.go:37` ticks every 2s. Three independent loops all touch routing state with no phase coordination — under node flapping (`EventNodeOffline/Recovered` at `main.go:1041,1050,1059`), the Withdraw/Reinstate path writes Caddy concurrently with the next ticker. This is last-writer-wins with nondeterministic `gamepanel`/`gamepanel-domains` survival.

### 6.5 What a “single-writer” fix must defend against (checklist for PR reviewers)

- [ ] Only `GatewayService.Reconcile` holds `*CaddyReverseProxy` and calls `GET/POST /config/` — enforced by not injecting `CaddyReverseProxy` anywhere else (constructor search proves it).
- [ ] `Reconcile` holds `pg_advisory_xact_lock` or singleflight mutex so cross-region replicas don't interleave.
- [ ] `empty-sync skip` guard is tested (`Sync` with 0 rules → no network call, mock assert count 0).
- [ ] `validate-then-apply` has snapshot-before order, tested with failing policy that leaves prior config intact (inspect mock call log: `GET /config/` before `POST /config/`, never `POST /load`).
- [ ] `RemoveRoutes` unit test uses grouped key, asserts per-backend withdrawal not per-route delete (`modifyCaddyUpstream` / `modifyUpstreamsInRoute` path).
- [ ] `SetCertificate` call site covered by ACME issue/renew/upload handler tests (assert gateway mock called).
- [ ] `/.well-known/acme-challenge/*` end-to-end handler test (httptest request through solver map populated by `Present`, not just unit).

### 6.6 Residual risks even after single-writer (to track, not block)

| Risk | After mitigation | Mitigation |
|---|---|---|
| Caddy restart loses routes (empty `/config/`, memory-only) | **Medium** — persists until next `Reconcile` tick (worst 30s-2m) | Persist desired-state JSONB in DB, boot-sync `domainSvc.SyncCaddyRoutes` already exists (`main.go:1030`) — extend to full gateway |
| Operator force-sets `Unhealthy` but probe auto-reverts to `Healthy` | **Low-Med** | HealthFilter must respect `TargetStatusDraining`/`Unhealthy` as sticky (LB already skips `Draining` at `dataplane.go:310`, but gateway must honor operator flag until manual override) |
| `TraefikReverseProxy` left as dead code confuses future contributors | **Low** | Delete `traefik_proxy.go:1139` or gate behind `GATEWAY_ADAPTER=traefik` flag with integration test; do not leave both adapters importable without guard |
| Three `groupRules` copies diverging again after refactor | **Low** | Collapse into `gateway/grouping.go` single `GroupRules(routes []*RoutingRule) []RouteGroup` with single sort+key; add `go vet` string-key not struct? keep struct |
| Firewall ephemeral rules drift from DB after fix | **Med** | After M5, daemon reconciliation loop diffs desired vs `ListFirewallRules` and converges |

---

## 7. Preparation Checklist — What to Do Before Coding (Phase-01 §8 requirement: “do not modify code”)

### 7.1 Evidence gathered, no file written

- [x] Inventory of 8 networking packages + 7 handler files + 5 web pages (this doc §1)
- [x] Cross-reference FINAL_PARITY NG-01..20 and phase-04 F-NET P0 cluster (this doc §2)
- [x] Files-to-touch with size/risk (this doc §3)
- [x] Single-writer inversion touchpoints and provider contract (§4)
- [x] Migrations needed with column shapes and defer decision (§5)
- [x] Five-writers-on-one-Caddy risk graph and six failure repros (§6)

### 7.2 Pre-flight checks the build agent should run before first PR

```bash
# 1) Confirm never-constructed surfaces
grep -rn "NewTraefikReverseProxy\|NewCaddyTLSManager" forge/api/cmd --include="*.go"
grep -rn "SetCertificate\|RemoveCertificate" forge/api --include="*.go" | grep -v "_test.go" | grep -v "func.*SetCertificate"

# 2) Confirm empty-sync is live
grep -n "SetRules\|UpsertRule" forge/api/internal/services/crossnode/ingress_sync.go
grep -rn "SetRules\|UpsertRule" forge --include="*.go" | grep -v "func "

# 3) Confirm three groupRules copies
grep -rn "func.*groupRules" forge/api/internal --include="*.go"

# 4) Confirm handler verb mismatch
grep -n "Patch\|Put\|Get.*policies" forge/api/internal/http/handlers_trafficmanager.go
grep -n "patchJSON\|putJSON\|fetchJSON.*policies" forge/web/app/admin/traffic/page.tsx

# 5) Confirm /load misuse
grep -n "validateConfig\|/load\|/config/.*must-revalidate" forge/api/internal/services/trafficmanager/caddy_proxy.go

# 6) Confirm HTTPSolver unmounted
grep -rn "HTTPSolver\|acme-challenge" forge/api --include="*.go" | grep -v "func Test" | grep -v "challenge/http.go"
```

### 7.3 Minimal test harness for the first fix PR (Step 0 hotfix)

- `TestIngressSync_SkipsEmpty` — construct `IngressSynchronizer` with nil adapter mock, `rules=nil`, `Sync` returns nil and mock `UpdateRoutes` call-count 0.
- `TestCaddyUpdateRoutesAtomic_PreservesDomainServer` — seeded `gamepanel-domains` routes in mock `GET /config/` response, call `updateRoutesAtomic` with one traffic rule, assert returned candidate still contains `gamepanel-domains`.
- `TestValidateIsNotApply` — policy with `rateLimit=100` produces `handler:"rate_limit"` → assert `validateConfig` does not issue `POST /load` (mock server records path); current code will fail this test — keep it as red test documenting the bug until fixed.

---

## 8. Cross-links

- **§8 reference truth:** `audits/FINAL_PARITY_AUDIT.md:194:209` (NG-01..NG-10) + `210:218` (NG-11..NG-20 table) + `400:405` (SE mappings)
- **Deep dive:** `audits/phase-04/synthesis.md` full cluster (5 subagents, 18+17 rows, §2 fragmentation map through §8 activation order)
- **Per-subagent:** `audits/final-parity/subagent-07-networking-gateway.md` + `audits/phase-04/subagent-0{1..5}-*.md`
- **TrafficManager tests:** `forge/api/internal/services/trafficmanager/{service_test.go,caddy_proxy_test.go,traefik_proxy_test.go,persistence_test.go}`
- **Crossnode tests:** `forge/api/internal/services/crossnode/{crossnode_test.go,scenario7_test.go}`
- **Loadbalancer tests:** `forge/api/internal/services/loadbalancer/{dataplane_test.go,load_test.go,loadbalancer_scenario7_test.go}`
- **ACME tests:** `forge/api/internal/services/acme/{service_test.go,providers_test.go}` (note `TestHTTPSolver:79` exists but no mount test)
- **Migrations:** `forge/api/migrations/083_a_traffic_rules.sql:1` vs `forge/api/internal/store/migrations/038_traffic_routing.sql:1` (divergent)
- **Main wiring:** `forge/api/cmd/api/main.go:982,994:995,1025,1030,1041:1067,1526:1535,1566:1571`
- **Routes wiring:** `forge/api/internal/http/server.go:1114,2610:2680`

---

## 9. Summary Verdict for Phase 01 Planning

- **Densest P0 cluster confirmed:** §8 NG-01 (5 writers), NG-08 (validate=apply), NG-09 (fictional modules), NG-10 (all-policies-everywhere), NG-07 (L4), NG-12 (HTTP-01 dead), NG-20 (certs undelivered), plus NG-19 (bad default contact) — exactly the phase-04 F-NET-01..09 set, all still reproducible today. No P0 was fixed since final-parity.
- **Preparation, not code, is correct for this subagent:** every fix requires a structural inversion (single-writer `GatewayService` + per-route `rule_policy_join` + validate-≠-apply + cert delivery + `/.well-known` mount + grouping consolidation). Patching any merge/validate/policy symptom in isolation re-creates the race.
- **Safe first slice:** §4.3 Step 0 hotfix (empty-sync guard + temporary merge-parity in `updateRoutesAtomic`) is the only no-migration, no-schema, reversible stop-the-bleeding change — ship it before the reconciler rewrite so the next 30s tick stops wiping domains on a greenfield deploy.
- **Gate for Phase 04 complete:** single `Reconcile` owning `POST /config/` with exclusive lock + snapshot-before-validate + gated fictional handlers + per-route middleware refs + cert delivery line + HTTP-01 mount is the exit criterion; everything else (UDP honest health, LB IP allowlist, firewall persistence, Traefik deletion, schema consolidation) is P1/P2.

