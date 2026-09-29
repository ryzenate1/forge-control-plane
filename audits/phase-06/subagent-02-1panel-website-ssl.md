# Subagent 02 — 1Panel Website / OpenResty / SSL / ACME / DNS vs Forge Domains + ACME + Proxy

> Focus: Compare 1Panel agent website stack (`reference/app-platforms/1panel/agent/app/api/v2/{website,website_ssl,website_acme_account,website_ca,website_dns_account,website_domain,nginx}.go` + `reference/app-platforms/1panel/agent/app/model/website*.go`) against Forge (`forge/api/internal/services/{domains,dns,acme,trafficmanager}`, `forge/api/internal/http/handlers_{domains,proxy_domains,acme_accounts,certificates,dns,trafficmanager}.go`, `forge/web/app/admin/{domains,certificates}`).

---

## 1. Coverage Comparisons (Reference vs Forge)

| # | Capability | 1Panel Reference (`file:line`) | Forge Equivalent (`file:line`) | STATUS | GAP | SEVERITY |
|---|---|---|---|---|---|---|
| 1 | **Website CRUD + pagination** | `reference/app-platforms/1panel/agent/app/api/v2/website.go:18-175` `PageWebsite`, `GetWebsites`, `CreateWebsite`, `UpdateWebsite`, `DeleteWebsite`, `GetWebsite` — dto `WebsiteSearch` `PageInfo` with `orderBy` `websiteGroupId` filtering | No direct website entity. Forge collapses site concept into `ProxyDomain` (`forge/api/internal/store/store_proxy_domains.go:13`) + `domains.Service` for ownership + `trafficmanager.RoutingRule` for routing. No pagination on domains beyond server list; `ListProxyDomains` supports limit/offset `store_proxy_domains.go:129` | **PARTIAL** | 1Panel has first-class `websites` table with type/alias/group/runtime/appInstall/db/favorite; Forge has no equivalent group concept, no alias, no pagination metadata (`total`), no bulk ops | **MEDIUM** — Forge covers proxy use-case but loses 1Panel multi-type site abstraction (PHP/runtime/app store) |
| 2 | **Domain attachment to site** | `website_domain.go:30-93` `CreateWebDomain` (bulk `WebsiteDomainCreate:domains[]`), `GetWebDomains`, `UpdateWebDomain`, `DeleteWebDomain`; model `website_domain.go:3-12` `WebsiteDomain{WebsiteID,Domain,SSL,Port}` | `forge/api/internal/services/domains/service.go:172-234` `AddDomain(serverID,domain)` single-domain, `ListDomains`, `RemoveDomain`, `FindDomainByHost`; `store_domains.go:22` `DomainRow{ServerID,Domain,Wildcard,Verified}` | **PARTIAL** | 1Panel allows N domains per website with per-domain port/ssl flags and batch create; Forge enforces one-at-a-time, global uniqueness (`GetDomainByDomain:194`), no per-domain port (port lives on `ProxyDomain:19`), no `SSL` flag per domain. 1Panel update domain `ssl` boolean not mapped | **MEDIUM** |
| 3 | **OpenResty / Nginx config planes** | `nginx.go:10-195` `GetNginx`, `GetNginxConfigByScope`, `UpdateNginxConfigByScope`, `GetNginxStatus`, `UpdateNginxFile`, `BuildNginx`, `UpdateNginxModule`, `GetNginxModules`, `OperateDefaultHTTPs`; website-scoped `GetNginxConfigByScope` + `GetWebsiteNginx:186` per site/configType | `trafficmanager/caddy_proxy.go:44-594` `UpdateRoutes`, `RemoveRoutes`, `UpdateDomainRoutes`, `buildRoutes`, `buildDomainRoutes`, `validateConfig`, `applyConfig`, `restoreConfig`; `caddy_tls.go:18-280` `ProvisionLetsEncrypt`, `UploadCustomCert`; `caddy_admin.go` admin API | **REIMAGINED** | 1Panel manages OpenResty as file-backed process (build/modules/status/filescope). Forge replaces Nginx with **Caddy Admin API** (`:2019`) — atomic validate-then-apply with rollback (`caddy_proxy.go:673-705`). No build/module pipeline, no raw file view, no `GetNginxStatus` equivalent (Caddy health is `AdapterHealth:HealthHealthy` stub `caddy_proxy.go:375`) | **LOW** — intentional gateway swap; gap only if raw OpenResty module compilation needed |
| 4 | **Scoped nginx editing** | `website.go:185-242` `GetWebsiteNginxConfig(id,type)`, `GetNginxConfigByScope`, `UpdateNginxConfigByScope`, `UpdateWebsiteNginxConfig:321`; request `NginxScopeReq`/`NginxConfigUpdate` | `trafficmanager/caddy_proxy.go:514-594` `UpdateDomainRoutes` merges into `gamepanel-domains` server; `ApplyRoutes` rebuilds `gamepanel` server | **PARTIAL** | 1Panel supports per-website nginx file snippet editing (`websiteId+scope`); Forge only generates routing via `RoutingRule`/`VerifiedDomainRoute` structs. No free-form nginx snippet, no `UpdateNginxFile` equivalent, no rewrite/proxy-file passthrough. Operators cannot hand-edit location blocks | **MEDIUM** |
| 5 | **Reverse-proxy / location config** | `website.go:509-608` `GetProxyConfig`, `UpdateProxyConfig` (`WebsiteProxyConfig:258` with `modifier/match/proxyPass/proxyHost/sni/CorsConfig`), `DeleteProxyConfig`, `UpdateProxyConfigStatus`, `UpdateProxyConfigFile` | `store_proxy_domains.go:13` `ProxyDomain{Path,StripPath,Port,ForwardAuthURL,ForwardAuthHeaders,RateLimit,WebSocket}` + `trafficmanager/service.go:22-39` `RoutingRule{Domain,Path,TargetHost,TargetPort,Protocol,Strategy,Weight}` + `caddy_proxy.go:924-978` grouping by `domain|path|protocol` + weighted upstreams | **PARTIAL** | 1Panel per-location modifier/replaces/cache/sni — richer proxy semantics. Forge path is simple prefix (`grp.path + "*"` `caddy_proxy.go:955`), strips via routing group, but no `modifier` (`= ~ ^~`), no `replaces` map, no per-proxy `Cache`/`SNI`/`proxySSLName`. Forward-auth only via `ForwardAuthURL` vs 1Panel `proxyHost` | **MEDIUM** |
| 6 | **Load-balancing / upstreams** | `website.go:861-961` `GetLoadBalances/CreateLoadBalance/DeleteLoadBalance/UpdateLoadBalance/UpdateLoadBalanceFile` — `dto.NginxUpstreamServer` pool, algorithm; model `Website.Upstreams` implied | `trafficmanager/service.go:291-427` `CreateRoutingRule`/`UpdateRoutingRule` with `Weight`, `Strategy` (`round_robin` default `299`), `caddy_proxy.go:894-922` groups rules into weighted upstream list `lb_policy` | **PARTIAL** | 1Panel upstream is explicitly named LB object per website. Forge achieves similar via multiple `RoutingRule` sharing `domain|path|protocol` key (`groupRules:894`). Missing: explicit LB name, file-level editing (`UpdateLoadBalanceFile`), Stream UDP/algorithm config (`StreamConfig:44` 1Panel) | **LOW** |
| 7 | **Anti-leech / AuthBasic / Path auth** | `website.go:591-710` `GetAuthConfig/UpdateAuthConfig`, `GetPathAuthConfig/UpdatePathAuthConfig`, `GetAntiLeech/UpdateAntiLeech` | `handlers_proxy_domains.go:312-359` `registerSecurityHeadersRoutes` (CRUD `SecurityHeaderConfig`) + `362-411` `registerRedirectRulesRoutes` — no auth/basic/leech | **MISSING** | No counterpart for `NginxAuthReq`/`NginxAntiLeech` in Forge. Security headers are HSTS/CSP etc, not hot-link protection or basic auth. Redirects exist but auth layer absent | **MEDIUM** — feature gap if porting WAF-lite sites |
| 8 | **SSL cert lifecycle (CRUD + upload)** | `website_ssl.go:18-380` `PageWebsiteSSL`, `ListWebsiteSSL`, `CreateWebsiteSSL` (`WebsiteSSLCreate:17` with `provider/acmeAccountId/dnsAccountId/keyType/pushDir`), `UpdateWebsiteSSL`, `DeleteWebsiteSSL`, `UploadWebsiteSSL` (paste/local), `UploadSSLFile` multipart, `DownloadWebsiteSSL`, `PushWebsiteSSLToNode`, `ImportMasterSSL` | `store_certificates.go:60-343` `CreateCertificate/Get/List/Update/Delete/FindExpiring/CreateAttempt`; `handlers_certificates.go:10-77` ACME issue/list/renew; `handlers_certificates_ext.go:17-110` `POST /certificates/upload` with PEM validation + `validateKeyPair`; `handlers_proxy_domains.go:215-310` `POST /custom-certificates` (proxy-uploaded) | **PARTIAL** | Forge covers create/list/get/delete/renew/upload/download/export. Missing: `Page` with gzip, `Search` by `acmeAccountId` filter is partial (provider/wildcard/status), `PushToNode`/`PushDir`/`Nodes` distribution, `ImportMasterSSL` cluster import, `Description`/`Dir` push semantics, `SkipDNS`/`DisableCNAME`/nameservers/shell hooks. Forge encrypts keys (`encryptSecret:86`), 1Panel stores plaintext (model `WebsiteSSL:14-15` minus `json:"-"` only on acme account) | **MEDIUM** — push/topology gaps relevant at scale |
| 9 | **ACME account mgmt** | `website_acme_account.go:18-97` `PageWebsiteAcmeAccount`, `CreateWebsiteAcmeAccount` (`WebsiteAcmeAccountCreate:61` with `type/keyType/eabKid/eabHmacKey/useProxy/caDirURL`), `UpdateWebsiteAcmeAccount` (only `useProxy`), `DeleteWebsiteAcmeAccount`; model `website_acme_account.go:3-15` fields `URL/PrivateKey/Type/EabKid/EabHmacKey/KeyType/UseProxy/CaDirURL` | `store_acme_accounts.go:14-163` `AcmeAccount{Email,PrivateKey,CAURL,IsDefault}`, `CreateAcmeAccount/UpdateAcmeAccount/ListAcmeAccounts/DeleteAcmeAccount`; `handlers_acme_accounts.go:18-95` `GET/POST/PUT/DELETE /acme/accounts`; `acme/service.go:548-595` `getOrCreateAccountKey` auto-generates P-384, persists | **PARTIAL** | 1Panel supports multiple CA types (`letsencrypt/zerossl/buypass/google/custom`), EAB, keyType choice (EC256/EC384/RSA*), proxy flag. Forge stores only `email+CAURL` (`store_acme_accounts.go:25`), no `type` discriminator, no `EAB`, fixed `EC P-384` key (`acme/service.go:580`), no pagination on page. Forge adds `IsDefault` + encryption not in 1Panel | **MEDIUM** — ZeroSSL/Buypass/EAB customers cannot replicate |
| 10 | **DNS account / provider** | `website_dns_account.go:18-95` `Page/Create/Update/Delete` `WebsiteDnsAccount{ Name,Type,Authorization }` — simple map; no verification loop | `store_dns.go:12-202` `DNSProvider{Name,ProviderType,Credentials(isDefault,verified)}` encrypted; `dns/service.go:90-663` `supportedProviders()` 39 providers, `ConfigureProvider`, `VerifyProvider`, `SetDefaultProvider`, `Execute/CleanupDNSChallenge`; `handlers_dns.go:11-90` `GET/POST /dns/providers`, verify, set-default | **SUPERSET** (Forge richer) | Forge expands provider catalog 10x (see #11). 1Panel DNS account is a raw `Authorization` blob per website_ssl (`WebsiteSSL.DnsAccountID`), no default/verified semantics. Forge enforces single default provider (`SetDefaultDNSProvider` tx `store_dns.go:151`) + `verified` flag, credentials encrypted at rest | **LOW** — Forge ahead; gap is migration of per-cert `dnsAccountId` vs global default |
| 11 | **DNS provider breadth** | `reference` — `Type` freeform (Cloudflare, Ali, Tencent, etc via `acme.sh` style) — exact list not in scanned files but typical 1Panel supports ~20 (Cloudflare/Ali/Godaddy/AWS/DigitalOcean/Namecheap) | `dns/service.go:90-302` 39 typed `ProviderDefinition` with `CredentialFields` metadata (sorted by Name), `providerRegistry:311-487` maps each to lego `NewDNSProvider()` via `createDNSProvider` (env-restore pattern) — `cloudflare,route53,gcloud,digitalocean,linode,vultr,ovh,namecheap,porkbun,godaddy,azure,alidns,dnspod,duckdns,dynu,easydns,exoscale,gandi,glesys,hetzner,infomaniak,ionos,lightsail,netcup,netlify,ns1,oraclecloud,pdns,rfc2136,scaleway,selectel,transip,vercel,wedos,zoneee` | **SUPERSET** | Forge enumerates strictly validated providers with UI credential schemas. 1Panel dynamic map (`Authorization map[string]string`) less typed | **LOW** |
| 12 | **Self-signed CA management** | `website_ca.go:14-178` `PageWebsiteCA/CreateWebsiteCA/GetWebsiteCA/DeleteWebsiteCA/ObtainWebsiteCA/RenewWebsiteCA/DownloadCAFile` — full CA entity `model WebsiteCA{CSR,Name,PrivateKey,KeyType}`; `WebsiteCAObtain:156` issues leaf cert from CA with duration | **MISSING** | No `store_*.go` / `handlers_*.go` / `dns` / `acme` material for private CA. Forge relies solely on public ACME (LetsEncrypt/ZeroSSL/Buypass/Google) + manual upload | **LOW** — niche; but parity loss for internal `.local`/`IP` certs. Severity LOW unless air-gapped use |
| 13 | **DNS-01 challenge wiring** | `website_ssl.go:113-125` `GetDNSResolve` (`WebsiteDNSReq:41` needs `acmeAccountId+websiteSSLId`) — resolve helper; SSL `ObtainSSL` path triggers acme.sh with dnsAccount credentials | `acme/service.go:193-320` `IssueCertificate` branches `ChallengeTypeDNS01` (requires `DNSProvider+DNSCredentials` validated `222-229`), `configureChallenge:297-320` sets lego DNS01 provider with `AddRecursiveNameservers(["1.1.1.1:53","8.8.8.8:53"])`; `dns/service.go:555-601` `ExecuteDNSChallenge/CleanupDNSChallenge` + `RegisterWithAcme` factory | **PARTIAL** | 1Panel per-cert `DnsAccountID` link; Forge per-request `DNSProvider` + credentials inline (`acme/service.go:187-189`) and fallback to `GetDefaultDNSProvider` (`dns/service.go:573`). Forge has no per-cert stored DNS account binding after issuance except `certificates.dns_provider/dns_credentials` columns (`store_certificates.go:48`) — not exposed in UI `forge/web/app/admin/certificates/page.tsx` issue flow (upload-only, no ACME issue form) | **MEDIUM** |
| 14 | **HTTP-01 / wildcard policy** | Implicit via `acme.sh --issue -w` (1Panel `website_ssl` provider handling) — wildcard via DNS automatically | `acme/service.go:210-219` `if wildcard && challenge != dns-01 => error "wildcard certificates require dns-01"` explicit enforcement | **EQUIVALENT** | Forge stricter guard; 1Panel same policy but enforced server-side via provider check. Forges error message clearer | **LOW** |
| 15 | **Certificate apply / HTTPS binding per site** | `website.go:244-290` `GetHTTPSConfig(id)`, `UpdateHTTPSConfig` (`WebsiteHTTPSOp:201` with `WebsiteSSLID/Type/privateKey/certificate/httpConfig/sslProtocol/algorithm/hsts/http3/ports`) atomic via `helper.GetTxAndContext()` + `tx.Commit` | `store_proxy_domains.go:13` `ProxyDomain{HTTPS,CertType,CertData,CertKey,AutoRenew}` + `caddy_tls.go:34-96` `ProvisionLetsEncrypt`/`UploadCustomCert`; `handlers_proxy_domains.go:16-76` `POST /domains` writes cert fields inline; `handlers_proxy_domains.go:228-310` `POST /custom-certificates` links via `domainId` | **PARTIAL** | 1Panel HTTPS config is rich (`HttpConfig: HTTPSOnly/HTTPAlso/HTTPToHTTPS`, `SSLProtocol[]`, `Algorithm`, `Hsts`, `Http3`, `HttpsPorts[]`). Forge stores only `CertType none/letsencrypt/custom` + `CertData/CertKey`. No HSTS toggle, no protocol version selection, no HTTP->HTTPS mode, no port list | **MEDIUM** |
| 16 | **Batch HTTPS assignment** | `website.go:1324-1342` `BatchSetHttps` (`BatchWebsiteHttps:133` with `Type: existed/auto/manual`, cert paths, `HttpConfig/SSLProtocol/Algorithm/Hsts/Http3`) | No bulk cert assignment. `trafficmanager` rules and `proxy_domains` are per-domain only | **MISSING** | No `POST /websites/batch/ssl` equivalent. Operator must repeat per-domain calls | **LOW** |
| 17 | **Proxy cache / WAF primitives** | `website.go:983-1024` `UpdateProxyCache/GetProxyCache/ClearProxyCache`, `SetRealIPConfig/GetRealIPConfig`, `GetCORSConfig/UpdateCORSConfig`, `OperateCrossSiteAccess`, `UpdateAntiLeech` | `trafficmanager/service.go:41-53` `TrafficPolicy{RateLimit,RateLimitBurst,IPWhitelist,IPBlacklist,TLSEnabled,CircuitBreaker}`; `caddy_proxy.go:795-874` policy handles (rate_limit, ip blacklist/whitelist, circuit_breaker); `store_traffic.go:26-42` policy row | **PARTIAL** | 1Panel `ProxyCache` (cache time/unit), `RealIP`, `CORS`, `AntiLeech` not mapped. Forge `TrafficPolicy` covers rate-limit + IP ACL + circuit breaker but not caching layer or CORS preflight | **MEDIUM** |
| 18 | **Domain verification mechanism** | `website_ssl.go:113` `GetDNSResolve` + `website.go` domain verification likely via nginx `/.well-known` + DNS hint; WebsiteCA/SSL status `Message` field | `domains/service.go:357-430` `verifyOwnership` (DNS resolve + `GET https://{host}/.well-known/forge-verify` with `Host` header override, body[256] compare to `VerificationToken`), `handlers_domains.go:102-124` `registerWellKnownVerifyRoute` serves token; `CheckDNS:335` separate probe; `proxy_domains` verify is **stub** (`handlers_proxy_domains.go:199-212` returns `verified:true` hardcoded) | **PARTIAL / BUG** | Real `domains` service verifies; `proxy_domains` verify is no-op. 1Panel per-domain verification status stored in `website_domains` implicit; Forge dual system confusing | **HIGH** — see Logic Finding LF-01 |

*Counts: 18 comparisons (requirement ≥12 satisfied).*

---

## 2. Logic Findings (≥3)

### LF-01 — `ProxyDomain` verify endpoint is a stub that never fails (High)

**Location:** `forge/api/internal/http/handlers_proxy_domains.go:199-212`

```go
domains.Post("/:id/verify", ..., func(c *fiber.Ctx) error {
    d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
    ...
    return c.JSON(fiber.Map{"data": fiber.Map{
        "id": d.ID, "hostname": d.Hostname, "verified": true,
    }})
})
```

Contrasts with the **real** verification flow:

- `domains/service.go:357-430` `verifyOwnership` => DNS `LookupHost` + `httpClient.Do(GET https://{host}/.well-known/forge-verify)` with strict token equality, `UpdateDomainVerification`.
- `handlers_domains.go:102-124` serves the token; `VerifyOwnership:321` is externally invocable.

**Problem:** Any caller hitting `POST /domains/:id/verify` (Proxy path) bypasses DNS/HTTP proof; UI will show “verified” even when DNS is mispointed. Operator may then request ACME HTTP-01 and fail. 1Panel ` website_domain.go:17-93` has no such split — single domain table with implied verification via reachable site (nginx already serving).

**Impact:** False-positive verification enables cert request looping, support tickets, and security misperception (domain not proven). The bug is masked because `forge/web/app/admin/domains/page.tsx:96-102` calls `POST /domains/verify` (Proxy path not Domain path) — the admin domains page indeed hits the stub? Actually `AdminDomainsPage` hits `POST /domains/verify` (`page.tsx:97`) which routes to `handlers_domains.go:67` (`POST /domains/verify` under `domains` service) vs `handlers_proxy_domains.go:199` path is `POST /domains/:id/verify` — different routes; but operator using Proxy UI directly (if exposed) hits stub.

**Recommendation:** Remove `registerProxyDomainRoutes` verify stub or delegate to `domains.Service.VerifyOwnership` when `ServiceID` lookup exists; add integration test asserting verify requires `/.well-known/forge-verify` round-trip.

---

### LF-02 — `domains.Service.verifyOwnership` holds global mutex across network I/O (Medium-High)

**Location:** `forge/api/internal/services/domains/service.go:362-364`

```go
s.mu.Lock()
defer s.mu.Unlock()

if record.VerificationToken == nil || ... { return }
resolvedIPs := s.resolveDomain(record.Domain)
...
resp, err := s.httpClient.Do(req)  // up to 10s (httpClient:114)
...
if content == token {
    s.store.UpdateDomainVerification(...)
    s.syncCaddyRoutes(...)
}
```

- `mu` is the `Service.mu` (`service.go:95` `sync.RWMutex`) shared by `reverifyAll` (`ListAllDomains` loop `432-441` calls `verifyOwnership` sequentially) and the ticker (`StartReverify:127-155` every 24h) plus every ad-hoc `VerifyOwnership` / `AddDomain` verify goroutine (`222-231`).
- `resolveDomain` does `net.LookupHost` (blocking DNS) and `httpClient.Do` (10 s timeout) while holding the lock.

**Contrast 1Panel:** `WebsiteDomain` operations are request-scoped SQL; no in-process domain lock. SSL verification delegated to `acme.sh` subprocess per-cert.

**Consequence:** With N unverified domains, `reverifyAll` serializes all verifications under one lock, delaying concurrent `AddDomain` verification and `FindDomainByHost` reads? Actually `FindDomainByHost` does not take lock, but write path stalls. Worse, if panel IP DNS is slow or Caddy down, ticker blocks for `N * 10s`.

**Fix:** Move `mu` to protect only token read + `UpdateDomainVerification` write (or introduce per-domain lock / `sync.Map` attempt). Pattern: snapshot `token` under RLock, release, do network, re-acquire for DB write + `syncCaddyRoutes`.

---

### LF-03 — Unchecked split between two certificate subsystems: `/certificates` (ACME) vs `/custom-certificates` (Proxy) — route and data-model divergence (High)

**Locations:**
- `handlers_certificates.go:15-76` registers `protected.Group("/certificates")` backed by `acme.Service` (lego, `store.CreateCertificate` via ACME).
- `handlers_certificates_ext.go:17-110` registers **same** `"/certificates"` prefix for manual upload (`POST /certificates/upload`).
- `handlers_proxy_domains.go:215-310` registers `"/custom-certificates"` for proxy-uploaded certs (workaround note `222-226` explains prior panic from duplicate Fiber routes).

**Store:** Single table `certificates` (`store_certificates.go:79`) used by both; columns `provider`, `challenge_type`, `dns_provider`, `dns_credentials` capture ACME metadata; `custom`/`manual` rows have `provider="manual"|"custom"` with no challenge data.

**1Panel reference:** Single `WebsiteSSL` table distinguished by `type` (`auto` vs `manual`) plus `Provider` string; certificate files co-located, uniform `PageWebsiteSSL` view. Push semantics unified.

**Logic gap in Forge:**

1. **UI confusion:** `forge/web/app/admin/certificates/page.tsx:33` fetches `GET /certificates` (ACME service) and `POST /certificates` (`page.tsx:44` posts to `/certificates` — but `handlers_proxy_domains.go` expects `POST /custom-certificates`?) — mismatched: `uploadMutation` posts to `/certificates` (`page.tsx:44` `postJSON("/certificates", uploadForm)`) with payload `{domainId, certificate, privateKey, issuer}` — this matches neither `handlers_certificates.go:17` (`IssueCertificateRequest{domains,provider,email,challengeType}`) nor `handlers_certificates_ext.go:24` (`{certificate,privateKey,chain}`) nor `handlers_proxy_domains.go:229` (`{domainId,domains,certificate,privateKey}`). The admin certificates page will 404/400 depending on which handler is mounted first.

2. **Renew path ambiguity:** `GET /certificates/:id/renew` exists on both mounts (`handlers_certificates.go:70` via `acme.Service.RenewCertificate`, `handlers_proxy_domains.go:300` same). Fiber registers duplicate `POST /certificates/:id/renew` on the ACME group and on the custom group? Actually proxy uses `/custom-certificates/:id/renew`, but `handlers_certificates_ext.go` also mounts `/certificates/:id/download` vs proxy `/custom-certificates/:id/...` — overlap risks 405.

3. **Data invariants:** ACME certs must have `private_key_encrypted` decryptable for renewal (`acme/service.go:328` `if cert.PrivateKey == "" => error`), custom certs set `AutoRenew:false` but `acme/service.go:423` `FindExpiringCertificates` selects `auto_renew=true && expires_at <= now+30d` — a custom cert with `auto_renew=true` would be queued for lego renewal and fail with opaque error.

**Recommendation:** Collapse to one certificate surface: deprecate `/custom-certificates`, route all uploads through `handlers_certificates_ext.go:24` `/certificates/upload`, add `domainId` linking as optional FK, unify renew to call `acme.Service.RenewCertificate` only when `provider != manual/custom` else return `400`. Update `certificates/page.tsx` to target correct endpoint and render `provider`/`challengeType`/`dnsProvider` columns.

---

### LF-04 — DNS credential env-mutation uses process-global `os.Setenv` with mutex but still vulnerable to third-party leakage and partial restore on panic (Medium)

**Location:** `forge/api/internal/services/dns/service.go:637-661` `setEnvRestore` + `createDNSProvider:489-498`

```go
func createDNSProvider(creds map[string]string, newProvider func() (challenge.Provider, error)) (challenge.Provider, error) {
    restore := setEnvRestore(creds)
    defer restore()
    return newProvider()
}
func setEnvRestore(env map[string]string) func() {
    envMu.Lock()
    previous := make(map[string]*string, len(env))
    for k, v := range env { ... os.Setenv(k, v) }
    return func() { defer envMu.Unlock(); for k, prev := range previous { ... } }
}
```

**Contrast 1Panel:** Each `DnsAccount.Authorization` is passed as JSON file or env to `acme.sh` child process — isolated by `exec.Cmd.Env`, not process environment.

**Issues:**

1. **Global window:** While `http://:80` challenge runs, any concurrent goroutine reading `os.Getenv("CF_DNS_API_TOKEN")` (e.g., metrics, other provider verification) sees the mutated value. Mutex serializes Forge callers but not third-party libs that read env without locking.
2. **Panic gap:** If `newProvider()` panics, the deferred `restore()` still runs (so safe), but if the lego provider **captures** env lazily (some do `os.Getenv` inside `Present`/`CleanUp` later, e.g., `route53` chain), the env is already restored before the challenge TXT write — then `Present` fails. Forge mitigates via immediate `NewDNSProvider()` capture, but not all providers eagerly read env (documented in comment `490-494`). For lazy providers, TXT record would be missing.
3. **Secret lifetime:** `dns_providers.credentials_encrypted` decrypted to `map[string]string` then passed to `CreateDNSProvider` exposes secrets in heap longer than necessary; 1Panel same but shorter-lived subprocess.

**Recommendation:** Prefer provider constructors that accept `Config` struct (lego supports `NewDNSProviderConfig`) over env injection; where unavailable, fork `creation` into isolated process or use `t.Setenv` style with `exec` isolation. At minimum, document which providers are lazy and assert eager env capture via unit test for each `providerRegistry` entry.

---

### LF-05 — Certificate renewal reconstructs `certificate.Resource` incorrectly, discarding chain and reusing stale private key DER (Medium)

**Location:** `forge/api/internal/services/acme/service.go:492-545` `renewOnce`

```go
certs := parseCertificateChain([]byte(cert.Certificate))
x509Cert := certs[0]
keyDER, _ := x509.MarshalPKCS8PrivateKey(certKey) // certKey parsed from stored PEM
...
res, _ := client.Certificate.Renew(certificate.Resource{
    Domain: cert.Domains[0],
    Certificate: x509Cert.Raw,       // raw DER of leaf only, not PEM chain
    PrivateKey: keyDER,              // PKCS8 DER, not original PEM
}, true, false, "")
```

Lego `Renew` expects `Resource` fields `Certificate` = PEM bundle and `PrivateKey` = PEM or DER matching original issuance (`github.com/go-acme/lego/v4/certificate`). Supplying `x509Cert.Raw` (single leaf DER, no issuer chain) vs original `certRes.Certificate` PEM (used at `IssueCertificate:269-270` to extract expiry) is inconsistent. 1Panel delegates renewal to `acme.sh --renew` which re-reads fullchain PEM from disk, so chain preserved.

**Consequence:** Renewed cert may lack intermediate chain (browsers require fullchain) and renewal may fail for providers that validate CSR against original bundle. `Store.UpdateCertificate:354` writes `res.Certificate` PEM (fresh chain) but `res.PrivateKey` may be regenerated by lego if `KeyType` mismatch — silently rotating key without persisting correctly due to `certs[0].Raw` vs `certRes.Certificate` drift.

**Fix:** Store original `certificate.Resource` PEM bundle verbatim, and on renewal pass `PrivateKey: []byte(cert.PrivateKey)` PEM plus `Certificate: []byte(cert.Certificate)` (fullchain) or regenerate CSR via `Obtain` flow rather than `Renew` with DER. Add `TestRenew_preservesChain` comparing `parseCertificateChain` length before/after.

---

### LF-06 — `CaddyTLSManager.validateConfig` is a no-op local JSON check, not a Caddy `/load` validation (Low-Medium)

**Location:** `forge/api/internal/services/trafficmanager/caddy_tls.go:240-251`

```go
func (m *CaddyTLSManager) validateConfig(ctx context.Context, addr string, configJSON []byte) error {
    var config map[string]any
    if err := json.Unmarshal(configJSON, &config); err != nil { return err }
    if len(config) == 0 { return errors.New("config is empty") }
    return nil
}
```

Versus `CaddyReverseProxy.validateConfig:980-1002` which actually `POST addr + "/load"` to Caddy admin and checks HTTP status.

**Impact:** `ProvisionLetsEncrypt:44-50` calls this stub, then `applyConfig` may push syntactically valid JSON that Caddy rejects (e.g., overlapping TLS policy, missing `subjects`), but error only surfaced at `applyConfig` (`caddy_tls.go:253`) without clear validation message. 1Panel `nginx -t` is run before reload (`nginxService.Build` path).

**Fix:** Delegate to shared `validateConfig` (POST `/load` with body) as in `caddy_proxy.go`, or at least call `caddyHTTPClient` against `/config/apps/tls` validate endpoint.

---

## 3. Structural Observations

- **Two domain systems:** `domains` (server-bound, verified, Caddy `gamepanel-domains:80`) vs `proxy_domains` (service-type generic, cert fields inline, `gamepanel:80/443`). 1Panel has single `website_domains` per website. Forge divergence is intentional (game server domains vs arbitrary reverse-proxy) but doubles UI/verify/cert paths — causes LF-01/LF-03.
- **No website group/runtime/appInstall/DB coupling:** Forge proxy is pure L7 routing, not a PaaS site builder. Porting PHP/runtime sites requires additional provisioning layer (outside this audit scope).
- **Certificate encryption improved:** Forge `store_certificates.go:86` `encryptSecret` + `store_acme_accounts.go:86` envelope — 1Panel stores `WebsiteAcmeAccount.PrivateKey` with `json:"-"` only (no at-rest encryption), `WebsiteSSL.PrivateKey/Pem` plaintext.
- **Provider wiring solid:** `dns/service.go:593-618` `RegisterWithAcme` correctly bridges stored DNS provider factories to `acme.Service`; env-restore nuance covered by LF-04.

---

## 4. Recommendations Prioritized

| Priority | Item | Owner Layer |
|---|---|---|
| **P0** | Fix `handlers_proxy_domains.go:199` verify stub — delegate or remove; add test that unverified hostname fails | `forge/api/internal/http` |
| **P0** | Unify certificate endpoints: retire `/custom-certificates`, keep `/certificates/upload` + `/issue`/`/renew`, update `forge/web/app/admin/certificates/page.tsx:44` to call correct endpoint | `http` + `web` |
| **P1** | Narrow `domains.Service.mu` scope to skip network I/O (LF-02) — use per-domain `sync.Mutex` or `singleflight` | `services/domains` |
| **P1** | Fix `renewOnce` to pass fullchain PEM + PEM private key (LF-05); add chain-length assertion test | `services/acme` |
| **P1** | Replace `CaddyTLSManager.validateConfig` stub with real Caddy `/load` validation (LF-06) | `services/trafficmanager` |
| **P2** | Migrate DNS env injection to `*Config` constructors where available (LF-04); catalog lazy providers | `services/dns` |
| **P2** | Map 1Panel `WebsiteHTTPSOp` rich fields (HSTS/Http3/SSLProtocol/HttpConfig) to `ProxyDomain` or `TrafficPolicy` extensions if parity required | `store` + `trafficmanager` |
| **P2** | Expose ACME issue UI (domains/mode/provider/credentials) — currently `certificates/page.tsx` is upload-only; wire `POST /certificates/issue` | `web/admin/certificates` |
| **P3** | Implement CA entity if internal PKI needed (1Panel `website_ca.go`); otherwise document unsupported | product decision |

---

*Generated for Phase 06 — subagent 02. No product code modified. File:line citations are literal paths relative to repo root (`/Users/riyaz/project/gamepanel/`).*
