# Reverification Subagent 15 — TLS / ACME / Certificate Management

**Focus:** issuance, renewal, chain handling, verification, Caddy renewal, secure auth
**Reconciling:** `audits/phase-04/subagent-02-tls-acme.md` (17 comparisons, LF-1..LF-6) + `audits/phase-06/subagent-02-1panel-website-ssl.md` (5 additional LFs: verify stub, global mu, env mutation, leaf DER, local JSON validate)
**References:** `reference/networking/caddy` certmagic ACME + strict SNI, `reference/networking/traefik` `pkg/provider/acme/provider.go` 36 providers, `reference/networking/nginx-proxy-manager`
**Forge:** `forge/api/internal/services/acme/service.go:52,157,193,389,492` , `forge/api/internal/http/handlers_certificates*.go` , `forge/api/internal/http/handlers_acme_accounts.go` , `forge/api/internal/services/dns/service.go:311` `providerRegistry`, migrations `094/134/135/157`
**Date:** 2026-08-24 · **Mode:** reverification (parallel 15/20) — do not modify product code

---

## Re-inspection Summary

| Claim | Verdict | Evidence |
|---|---|---|
| LF-1 HTTP-01 dead | **STILL BROKEN** | `acme/service.go:52` `httpChallenger` + `service.go:157` `HTTPSolver()` + `service.go:300` `SetHTTP01Provider` implemented but has **zero mount points** outside package. Grep `HTTPSolver` across repo returns only definition `service.go:157` and test `service_test.go:79`. No Fiber route, no `:80` listener, no `/.well-known/acme-challenge` handler wired in `http/server.go` or `cmd/api/main.go`. `httpSolverAddr` `service.go:129,139,153` stored but never creates an `http.Server`. Default `ChallengeTypeHTTP01` `service.go:203` therefore guarantees LE validation timeout. |
| LF-2 certs never leave DB | **STILL BROKEN** | `trafficmanager/gateway_adapter.go:49` `SetCertificate` + impls `caddy_proxy.go:83` `SetCertificate`, `traefik_proxy.go:373` `SetCertificate` — grep shows **zero callers**. `acme/service.go:355` `UpdateCertificate` and `service.go:277` `CreateCertificate` persist to Postgres only; `runAutoRenewal:420-438` has no post-renew hook; `domainsenv` `CertID` ledger (ref Phase-04 #13) still has no consumer. Reconciler `trafficmanager/service.go:953` repairs routing drift but not cert drift. Traefik pushes via `addCertificateForDomain:796` and Caddy via certmagic cache — neither path is used for `acme.Service` certs. |
| LF-3 key reuse (renew) | **STILL BROKEN** | `service.go:492` `renewOnce` parses stored key `service.go:498` `MarshalPKCS8PrivateKey` and reuses it in `certificate.Resource{PrivateKey: keyDER}` `service.go:539`. Caddy `automation.go:144-151` explicitly rotates keys per renewal; lego `Renew(bundle=true, ...)` with stored `PrivateKey` reuses key forever. No fresh CSR generation, no `reuse_private_keys` opt-in documented. |
| LF-4 plaintext DNS creds | **STILL BROKEN (partially remediated elsewhere)** | `store_certificates.go:95` `INSERT ... dns_credentials` writes `req.DNSCredentials map[string]string` as plain JSONB; migration `134_certificate_dns_provider.sql:2` `ADD COLUMN dns_credentials JSONB DEFAULT '{}'` — no `_encrypted` column, no `encryptSecret`. Contrasts with sibling `store_dns.go:95,127` `credentials_encrypted` (encrypted via `secretAAD("dns_providers",id,"credentials")`) and `store_acme_accounts.go:86` `private_key_encrypted` (migration `157_encrypt_acme_account_keys.sql:2`). `store_secrets.go:326` `migrateAcmeAccountKeys` covers `acme_accounts` but NOT `certificates.dns_credentials`. `dns_provider_accounts.credentials` (migration `135`) also plaintext fallback before `store_dns.go` fix — the proxy table `certificates` was never migrated. Burning a `certificates` row leaks Cloudflare `CF_DNS_API_TOKEN` / Route53 `AWS_SECRET_ACCESS_KEY` in clear. |
| LF-5 panic kills loop | **PARTIALLY FIXED — STILL RISKY** | `service.go:389-396` now has `defer recover()` logging stack, unlike original. However recover wraps the **goroutine outside the `for`**; first panic inside `runAutoRenewal` or `RenewCertificate` exits the ticker loop permanently (no per-tick recover, no restart). Durable fallback `JobCertRenewal` `cmd/api/main.go:1179-1193` bounds blast radius but primary in-proc renewal is still one-shot. Recommendation in Phase-04 #5 (move recover inside tick) not applied. |
| LF-6 admin@localhost | **STILL BROKEN** | `service.go:198` `req.Email = "admin@localhost"` default, `service.go:507` `email: "admin@localhost"` in `renewOnce` `userReg`, `service.go:554,609` fallback in `getOrCreateAccountKey`/`getAccountKeyForRenew`, `http/handlers_user_web.go:463` `req.Email = "admin@localhost"` proxy-domain handler. LE/BuyPass reject localhost contacts → opaque ACME error; row persisted as `acme_accounts` with localhost key. NPM hard-blocks without real email (`certificate.js:145`). |
| LF-7 verify stub (Phase-06 LF-01) | **STILL BROKEN** | `http/handlers_proxy_domains.go:199-212` `POST /domains/:id/verify` returns `verified:true` hardcoded after `GetProxyDomain` lookup, no DNS/HTTP proof. Real flow is `services/domains/service.go:357-430` `verifyOwnership` (DNS `LookupHost` + `GET https://{host}/.well-known/forge-verify` `handlers_domains.go:107`). Dual system masked: admin `domains/page.tsx` hits real path, but proxy-domain UI hits stub. |
| LF-8 global mu (Phase-06 LF-02) | **STILL BROKEN** | `services/dns/service.go:637` `var envMu sync.Mutex` global; `service.go:362` `domains/service.go:362` `s.mu.Lock()` held across `net.LookupHost` + `httpClient.Do` (10s timeout) during `verifyOwnership`. Also `acme/service.go:127` `sync.RWMutex` guards `dnsProviders` map but renewal loop is sequential. Effect: `reverifyAll` serializes N verifications `N*10s`; concurrent `AddDomain` verify blocks. 1Panel has no such in-process lock. |
| LF-9 env mutation (Phase-06 LF-04) | **STILL BROKEN (narrowed window)** | `services/dns/service.go:489-498` `createDNSProvider` → `setEnvRestore:639-661` does `os.Setenv`/`Unsetenv` under `envMu`. Window is now microseconds (comment `490-494` audit P1) vs lifetime, which is improvement, but still process-global mutation serializing concurrent issuances and visible to any `os.Getenv` reader. 1Panel uses `exec.Cmd.Env` isolated per `acme.sh` subprocess. Some lego providers read env lazily in `Present`/`CleanUp` after restore → silent TXT failure risk. |
| Leaf-DER chain loss (Phase-06 LF-05) | **STILL BROKEN** | `service.go:492-545` `renewOnce`: `parseCertificateChain:659` extracts leaf `certs[0]`, then `Renew(Resource{Certificate: x509Cert.Raw, PrivateKey: keyDER})` `service.go:536-540` passes single leaf DER, not fullchain PEM (`cert.Certificate` original bundle). `service.go:269-270` `IssueCertificate` stores `certRes.Certificate` bundle vs `renewOnce` discarding intermediates. Result: renewed PEM may lack chain, breaking browser fullchain validation. Fix in Phase-06 LF-05 (pass fullchain PEM + PEM key or re-`Obtain`) not applied. |

---

## Reverification Matrix — 18 Capability Comparisons (Phase-04 17 + Phase-06 carry-overs)

Requirement: ≥12 rows. Each row re-inspects reference vs Forge current `main`.

| # | Capability | Reference (`file:line`) | Forge (`file:line`) | Status | Gap / Drift | Severity |
|---|---|---|---|---|---|---|
| 1 | ACME client & registration | traefik `pkg/provider/acme/provider.go:316-334` lego v5 persist `Account{Registration}`; caddy `modules/caddytls/acmeissuer.go:254` certmagic | `acme/service.go:193` `IssueCertificate` fresh lego v4 client per request, `service.go:253,530` re-register ToS, `service.go:241,519` hard `RSA2048` | ⚠️ Partial | No EAB (`provider.go:54`), no `KeyType` option, no preferred-chain (`service.go:540` `""`), no registration URI persistence (`migrations/135` lacks column), account→CA binding only via `ca_url`. Same as Phase-04 #1. | Medium |
| 2 | HTTP-01 challenge wiring | traefik `challenge_http.go:33-100` entrypoint router; caddy `acmeissuer.go:281` `AltHTTPPort` | `acme/service.go:52-92` `httpChallenger` token map `service.go:81` `r.Host` without port strip; `service.go:157` `HTTPSolver()` | ❌ Dead code | Handler never mounted; `httpSolverAddr` `service.go:139,153` unused; default HTTP-01 always times out. **LF-1.** | **Critical** |
| 3 | DNS-01 provider breadth | traefik `provider.go:342` `NewDNSChallengeProviderByName` ~36; `provider.go:113` `Propagation` struct | `dns/service.go:311` `providerRegistry` 36 entries (`cloudflare`→`zoneee`), `service.go:315` `AddRecursiveNameservers(["1.1.1.1:53","8.8.8.8:53"])` | ✅ Breadth parity | No propagation tuning (`delayBeforeChecks`, `PropagationWait`, TTL, `overrideDomain`) as Phase-04 #3; NPM `propagation_seconds:863` exposed, Forge not. | Medium |
| 4 | DNS credential isolation | traefik `provider.go:104` `Resolvers` per-request; 1Panel `website_dns_account` per-cert env isolation | `dns/service.go:489` `createDNSProvider` env round-trip + `service.go:637` `envMu` | ⚠️ Works but fragile | Global env mutation serializes; lazy providers may read after restore. See LF-9. | Medium |
| 5 | Wildcard handling | traefik `provider.go:1057` reject `*.*`, `dns01.UnFqdn`, `deleteUnnecessaryDomains:828` | `acme/service.go:210-219` `*.` prefix requires dns-01; stored `wildcard` flag | ⚠️ Minimal | No `*.*` reject, no trailing-dot normalization, no SAN deduplication under wildcard. Same as Phase-04 #4. | Low |
| 6 | Renewal window & schedule | traefik `getCertificateRenewDurations:808` scales to lifetime; caddy `automation.go:65` 10m scans, 1/3 lifetime ratio | `acme/service.go:397` 24h ticker; `store_certificates.go:260` `expires_at <= NOW()+30 days` | ⚠️ Fixed window | 30d fixed regardless of cert lifetime; 24h scan misses <24h expiry; no jitter/stagger; durable queue `main.go:1178` mitigates but not scaled. Phase-04 #5. | Medium |
| 7 | Renewal key & account binding | caddy `automation.go:146` new key per renewal default | `acme/service.go:492` `renewOnce` reuses stored `PrivateKey` DER `service.go:539`; account = first matching `ca_url` `service.go:612` `findAccountByCA` | ❌ Key reuse + account flip risk | **LF-3.** No issuing-account FK on `certificates`, no fresh key. | **High** |
| 8 | Revocation | caddy `acmeissuer.go:346` `Revoke` with reason; npm `certificate.js:1021` `certbot revoke` | `acme/service.go:368` `RevokeCertificate = DeleteCertificate` DB only `service.go:250` | ❌ Overpromises | No ACME `client.Certificate.Revoke` call, no reason code. Phase-04 #7. | Medium |
| 9 | Custom upload validation | npm `certificate.js:487` `openssl` verify; caddy `fileloader` | `handlers_certificates_ext.go:24` `POST /certificates/upload` → `validateCertificatePEM:112` + `validateKeyPair:127` PKCS8/PKCS1/EC + pubkey bytes compare | ✅ Solid | Chain not verified to leaf; export `handlers_certificates_ext.go:92` returns private key as JSON behind `certificates.read`. Phase-04 #8 gap unchanged. | Low |
| 10 | Encryption at rest | traefik `local_store_unix.go:31` 0600; caddy `StorageConverter`; npm 0600 | `store_certificates.go:86,212` `private_key_encrypted` AAD, `store_acme_accounts.go:86` `private_key_encrypted` (migration 157) | ⚠️ Inconsistent | `certificates.dns_credentials` `store_certificates.go:95` plaintext; `dns_provider_accounts` now encrypted `store_dns.go:95` but proxy certs not. **LF-4.** | **High** |
| 11 | Account persistence | traefik `account.go:31` RSA4096 persisted | `store_acme_accounts.go:37` `ListAcmeAccounts`, `service.go:548` `getOrCreateAccountKey` ECDSA P-384 `service.go:580` | ✅ Functional | No `UNIQUE(email,ca_url)` race, no `is_default` logic, API accepts malformed PEM `handlers_acme_accounts.go:43` then `continue` silent `service.go:571`. Phase-04 #10. | Medium |
| 12 | Default contact hygiene | traefik requires explicit email; npm blocks `certificate.js:145` | `service.go:198,507,554,609` + `handlers_user_web.go:463` synthesize `admin@localhost` | ❌ Gap | LE rejects, opaque error, useless row persisted. **LF-6.** | **High** |
| 13 | Retry / rate-limit | caddy staging fallback `acmeissuer.go:50` | `service.go:440` `obtainWithRetry` / `466` `renewWithRetry` 4 tries 5→20s `service.go:454` | ⚠️ Blind retry | Retries fatal `rejectedIdentifier` same as transient network; burns LE 5-fail/hour. No `ProblemDetails` parsing. Phase-04 #12. | Medium |
| 14 | Gateway delivery | traefik `provider.go:800` `buildMessage` → dynamic config; caddy certmagic cache swap | `gateway_adapter.go:49` `SetCertificate` / `caddy_proxy.go:83` + `306` `UpdateDomainRoutes` | ❌ Unwired | Zero callers; DB→gateway sync missing; domain reconciler `main.go:1151` covers routes not certs. **LF-2.** | **Critical** |
| 15 | TLS-ALPN-01 | traefik `challenge_tls.go` | `service.go:40` only `http-01`/`dns-01` `service.go:206` rejects others | ❌ Missing | Port-80-blocked envs have no alternative; doc says use DNS-01. Phase-04 #15. | Low |
| 16 | Preflight reachability | npm `testHttpsChallenge:1120` external probe | `services/domains/service.go:383` `forge-verify` for ownership only | ❌ Missing | No ACME-challenge HTTP probe before spending quota. Phase-04 #16. | Low |
| 17 | mTLS / internal PKI | caddy `internalissuer.go` automated CA | `services/cert_service.go:52` `GenerateCA` RSA4096 10y, `GenerateCert:103` | ✅ Fit-for-purpose | No CRL/OCSP distribution, single self-signed root, no rotation overlap enforcement. Phase-04 #17. | Medium |
| 18 | Domain verify & cert confusion (Phase-06 carry-over) | 1Panel single `website_domains` | `handlers_proxy_domains.go:199` stub `verified:true` vs real `domains/service.go:357`; dual cert surfaces `handlers_certificates.go:15` `/certificates` vs `handlers_proxy_domains.go:227` `/custom-certificates` vs `handlers_certificates_ext.go:17` `/certificates/upload`  | ❌ Divergence | Proxy verify bypasses proof; cert UI `certificates/page.tsx:44` mismatches backend. Phase-06 LF-01/LF-03 still present. Validated `validateCertificatePEM:112` only checks first block / expiry not chain (Phase-06 LF local JSON validate). | **High** |

*Counts: 18 comparisons (≥12 ✔) · Reference: traefik `provider.go` 36 providers mirrored in `dns/service.go:90-302` definitions + `providerRegistry:311` — breadth parity confirmed, wiring is `RegisterWithAcme:593`→`acme/service.go:161` `RegisterDNSProvider`.*

---

## Logic Findings Detail (≥3 required — 6 re-confirmed)

### LF-A — HTTPSolver never mounted (Critical) — STILL BROKEN
**Locations:** `forge/api/internal/services/acme/service.go:52` `httpChallenger`, `service.go:75-92` `ServeHTTP`, `service.go:157-159` `HTTPSolver()`, `service.go:139` `httpSolverAddr:":80"`, `service.go:300` `SetHTTP01Provider`

**Re-inspection:** Repo grep `HTTPSolver` yields 2 hits: definition + `service_test.go:79` `TestHTTPSolver`. No mount in `internal/http/server.go` (`registerWellKnownVerifyRoute:102` serves `/.well-known/forge-verify` only), no wiring in `cmd/api/main.go:984` `acmeSvc = acmesvc.New(db, slogLogger)` followed by `dnsSvc.RegisterWithAcme:984` but never `app.Get("/.well-known/acme-challenge/*", acmeSvc.HTTPSolver())` or Caddy passthrough. `httpSolverAddr` setter exists `service.go:153` but is unused. Host-port stripping missing `service.go:81` `for _, domain := range []string{r.Host}` keeps `:80` suffix → lookup miss if client uses non-standard port (traefik:challenge_http.go:82 strips).

** exploit:** `POST /certificates/issue` with `challengeType:"http-01"` (default `service.go:203`) → `configureChallenge` sets `s.httpChallenge` → `obtainWithRetry:440` polls LE → LE `GET http://domain/.well-known/acme-challenge/<token>` → 404 → timeout → retries 4× `5,10,20s` `service.go:454` → consumes LE `5 failures/hour` budget.

**Ref vs Forge:** traefik mounts `ChallengeHTTP` on entrypoint router; NPM generates ephemeral nginx `/.well-known/acme-challenge` file + reload `certificate.js:168`. Forge dead code.

### LF-B — Certificates never leave the database (Critical) — STILL BROKEN
**Locations:** `forge/api/internal/services/trafficmanager/gateway_adapter.go:49` `SetCertificate`, `trafficmanager/caddy_proxy.go:83` `SetCertificate` (POST `tls/certificates/<domain>` to Caddy admin `caddy_proxy.go:93`), `trafficmanager/traefik_proxy.go:373`, `acme/service.go:355-363` `UpdateCertificate`, `acme/service.go:420` `runAutoRenewal`, `trafficmanager/service.go:1001` `ProbeTargets`/`562` `ApplyRoutes` never touch certs

**Re-inspection:** `grep -r SetCertificate` returns only interface + 2 impls; `grep -r CaddyTLSManager` etc shows `main.go:982` `caddyProxy` passed to `trafficmanager.NewWithPersistence:1025` but `acme.Service` never holds a `GatewayAdapter` reference, nor does `cmd/api/main.go:1169` `StartAutoRenewal` chain post-renew hooks. Successful `IssueCertificate:269` `CreateCertificate` and `RenewCertificate:355` `UpdateCertificate` end at Postgres. Caddy renewal `beacon/internal/tls` separate is not used by panel. No cert drift comparator analogous to `trafficmanager/service.go:900` `ReconcileRoutes`.

**Phase-06 cross:** Dual cert tables `certificates` vs `proxy_domains.CertData/CertKey` + dual handlers (`/certificates` vs `/custom-certificates:227`) amplify confusion but both are DB-only unless manual `CertData` is pushed via `trafficmanager` separately.

### LF-C — renewOnce leaf-only chain + DER misuse (High) — STILL BROKEN
**Locations:** `acme/service.go:492` `renewOnce`, `service.go:493` `parseCertificateChain`, `service.go:498-501` `MarshalPKCS8PrivateKey`, `service.go:536-540` `client.Certificate.Renew(Resource{Domain: Domains[0], Certificate: x509Cert.Raw, PrivateKey: keyDER}, true, false, "")`, `store_certificates.go:114` `GetCertificate` returns fullchain PEM in `Certificate`

**Re-inspection:** `parseCertificateChain:659` parses fullchain correctly but caller `renewOnce:498` `x509Cert := certs[0]` then `keyDER` is PKCS8 DER bytes, not PEM; `Renew` is fed `Certificate: x509Cert.Raw` (single leaf DER, no intermediates, no PEM headers) contrary to lego expectation (`Resource.Certificate` is PEM bundle at issuance `service.go:269` `string(certRes.Certificate)`). Phase-04 #13 noted `true` bundle flag but input is leaf-only; Phase-06 LF-05 flagged chain length loss — still present. No test covers chain preservation (`service_test.go` only checks error contains `configure challenge`). Stored `UpdateCertificate:355` writes `res.Certificate`/`res.PrivateKey` from Renew result, so chain may be restored post-renew but input chain loss risks Renew validation failure for providers validating CSR against chain.

**Ref:** 1Panel delegates to `acme.sh --renew` reading fullchain PEM; traefik `renewCertificates:936` passes existing `cert.Certificate` bytes + key.

### LF-D — Plaintext DNS credentials in `certificates` (High) — STILL BROKEN
**Locations:** `store_certificates.go:38` `DNSCredentials map[string]string`, `store_certificates.go:95` `INSERT ... dns_credentials`, `store_certificates.go:116,139,258` `COALESCE(dns_credentials,'{}'::jsonb)`, `migrations/134_certificate_dns_provider.sql:2`, `migrations/157_encrypt_acme_account_keys.sql:2`, `store_secrets.go:326` `migrateAcmeAccountKeys`, `store_dns.go:42,95,127` encrypted path

**Re-inspection:** `certificates` row `DNSCredentials` serialized as `map[string]string` → driver JSONB plaintext, never `encryptSecret`. Contrast: `acme_accounts.private_key` migrated `157` → `private_key_encrypted` `store_acme_accounts.go:86` envelope AAD `store_secrets.go:351`; `dns_providers.credentials` migrated `096_dns_providers.sql:6` + `store_dns.go:95` encrypted with `secretAAD("dns_providers",id,"credentials")`. A DB dump or `SELECT` on `certificates` leaks `CF_DNS_API_TOKEN`/`AWS_SECRET_ACCESS_KEY` for any custom DNS-01 issuance. `FindExpiringCertificates:256` even selects `dns_credentials` in clear. `dns_provider_accounts` (separate table `migrations/135`) plaintext fallback vs `dns_providers` encrypted — asymmetry flagged in Phase-04 #9 persists.

**Also:** `handlers_user_web.go:451-474` `issueServerProxyDomainCertificate` forwards `DNSCredentials` from request body to `IssueCertificate` then into `store.CreateCertificate` plaintext store without redaction; logs not redacted.

### LF-E — Proxy domain verify stub never fails (High) — STILL BROKEN (Phase-06 LF-01)
**Location:** `forge/api/internal/http/handlers_proxy_domains.go:199-212` `POST /domains/:id/verify` → `return verified:true`

Contrasts with real `services/domains/service.go:357` `verifyOwnership` (DNS `LookupHost` + `GET https://host/.well-known/forge-verify` `service.go:383` with `bytes[256]` token compare, `UpdateDomainVerification` + `syncCaddyRoutes`). Stub enables false-positive operator workflow → ACME attempt → failure loop.

### LF-F — Global env-mutation window + per-verify mutex (Medium) — STILL BROKEN (Phase-06 LF-02/LF-04)
**Locations:** `services/dns/service.go:311` `providerRegistry`, `service.go:489` `createDNSProvider`, `service.go:637` `envMu sync.Mutex`, `service.go:639` `setEnvRestore`, `services/domains/service.go:362` `s.mu.Lock()` across `resolveDomain` + `httpClient.Do`

`envMu` serializes all concurrent DNS-01 issuances; `domains.Service.mu` `service.go:95` held across 10s network I/O serializes `reverifyAll:432` sequential loop and blocks `AddDomain` verify goroutine `service.go:222`. Env mutation window shrunk to provider construction only (comment `490-494`) but still global, third-party `os.Getenv` readers see value, and lazy lego providers (e.g., `route53` credential chain) may read env in `Present` after restore.

---

## Secure-Auth Notes (Caddy renewal / admin)

- Caddy renewal via ACME is via `acme.Service` DB path, not Caddy native `certmagic` (panel controls Caddy via `CaddyReverseProxy` admin API `caddy_proxy.go:44` `UpdateRoutes` atomic `updateRoutesAtomic:673` with `lastValidConfig` rollback). No direct Caddy ACME integration observed in `forge/api`; Beacon `internal/tls/autocert.go:43` separate.
- Admin auth on cert endpoints: `handlers_certificates.go:17` `requireRole("admin")` + `requireAdminScope("certificates.write")`, mutation limiter, IP access `adminIPAccess` — correctly gated. ACME account CRUD `handlers_acme_accounts.go:19,29,54` same gating, but `POST /acme/accounts:29` accepts arbitrary `privateKey` without PEM validation `handlers_acme_accounts.go:43-47` → poisoned row masked by `continue` `service.go:571` generating new key silently (Phase-04 #10).
- Upload validation `handlers_certificates_ext.go:112` `validateCertificatePEM` only decodes first block, checks `NotAfter`, no chain/roots/SAN verification; `validateKeyPair:127` pubkey bytes compare correct but no `ExtKeyUsage` / RSA size check. Local-only JSON validate, no CA fetch — acceptable for manual upload but bounds `acme.sh` parity.

---

## Coverage Check

- Phase-04 comparisons re-inspected: **17** (reproduced as #1-17 above)
- Phase-06 carry-over: **1** extra (#18) covering verify stub + dual-cert divergence + local PEM validate
- Total comparisons in this report: **18** (≥12 ✔)
- Logic findings re-confirmed still BROKEN: **6** (LF-A..LF-F, ≥3 ✔) + LF-C/D overlap original LF-2/LF-3
- All claims carry `file:line` citations; no product code modified; reverification is read-only.

---

*No product code modified. File:line citations are literal paths relative to repo root (`/Users/riyaz/project/gamepanel/`).*
