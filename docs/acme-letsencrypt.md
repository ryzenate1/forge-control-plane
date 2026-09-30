# ACME / Let's Encrypt runbook — 2 websites (staging → prod)

Two issuance paths exist in this repo; pick one per environment:

| Path | Gateway | When to use |
|---|---|---|
| **A. Forge ACME service (lego)** — `POST /certificates/issue` | Any (DB-stored cert, installed into gateway via `GatewayCertInstaller`) | Ubuntu prod with the API running; works behind Caddy or nginx |
| **B. Gateway-managed** — `PUT /domains/:id { certType: "letsencrypt" }` (Caddy) or Traefik `compose.tls.yml` / host nginx + certbot | Caddy / Traefik / host nginx | Caddy/Traefik prod stacks; host-nginx stack |

There is no `openapi.json` in the repo (the task brief assumed one). The
routes below are the source of truth, from
`forge/api/internal/http/handlers_acme_accounts.go`,
`handlers_certificates.go`, `handlers_certificates_ext.go`,
`handlers_proxy_domains.go`, and `server.go`:

```
GET    /acme/accounts                 list ACME accounts        -> { data: [...] }
POST   /acme/accounts                 create { email, caUrl?, privateKey? } -> { data: {...} } 201
GET    /acme/accounts/:id             get one                   -> { data: {...} }
PUT    /acme/accounts/:id             update { email?, caUrl?, privateKey?, isDefault? }
DELETE /acme/accounts/:id             204
GET    /acme/dns-accounts[?provider=] list DNS-01 accounts      -> { data: [...] }
POST   /acme/dns-accounts             create { name, provider, credentials }
DELETE /acme/dns-accounts/:id         204
POST   /certificates/issue            order   { domains[], provider?, email?, challengeType?, dnsProvider?, dnsCredentials?, autoRenew? } -> { data: cert } 201
GET    /certificates[?provider&status&wildcard&limit&offset]    -> { data: [...] }
GET    /certificates/:id              -> { data: cert } (private key never serialized, `json:"-"`)
POST   /certificates/:id/renew        -> { data: cert }
DELETE /certificates/:id              revoke + delete, 204
POST   /certificates/upload           inventory import { certificate, privateKey, chain? } -> { data: cert } 201
GET    /certificates/:id/download     PEM download
POST   /certificates/:id/export       -> { data: { certificate, privateKey } }
POST   /certificates                  domain-bound import { domainId, domains?, certificate, privateKey, issuer? } -> { data: cert } 201
PUT    /domains/:id                   attach { certType: "letsencrypt"|"custom"|"none", certData?, certKey?, autoRenew?, https? }
GET    /.well-known/acme-challenge/*  HTTP-01 solver (public, served by acme.Service.HTTPSolver)
```

All `/acme/*`, `/certificates*`, `/domains*` routes require admin role +
`certificates.read/write` or `domains.read/write` scope, session cookie +
CSRF. The frontend client lives in `forge/web/lib/api/acme.ts`; UIs are
`/admin/acme`, `/admin/certificates`, `/admin/domains`, `/admin/dns`.

## 0. Prerequisites (both domains)

- Both hostnames have public **A/AAAA records pointing at the prod host**.
  Verify first: `Admin → Domains → Check DNS`, or
  `POST /domains/:id/verify` (does a real `LookupHost`, returns
  `{ verified, addresses }`).
- **Ports 80 and 443 open** on the host firewall/security group and mapped
  in compose (`80:80`, `443:443`). HTTP-01 serves the token on port 80;
  LE follows redirects to 443 but the initial port-80 reachability is
  mandatory. DNS-01 needs no inbound ports (only outbound to the CA + DNS API).
- Outbound HTTPS to the CA directory (prod
  `https://acme-v02.api.letsencrypt.org/directory`, staging
  `https://acme-staging-v02.api.letsencrypt.org/directory`).
- Challenge choice:
  - `http-01` — single hostnames only, domain must already resolve to this
    panel. Wildcards (`*.example.com`) are **rejected** with http-01.
  - `dns-01` — required for wildcards; needs a supported DNS provider plus
    credentials passed per issuance (`dnsProvider`, `dnsCredentials`) —
    lego providers under `forge/api/internal/services/dns/service.go`
    (cloudflare, route53, gandi, hetzner, …). Stored `/acme/dns-accounts`
    rows are inventory only; issuance takes credentials inline.

## 1. Staging first (both domains, avoids rate limits)

```bash
API=https://panel.example.com/api/v1   # same-origin /api/v1 in the UI
COOKIE='session cookie from login'

# 1a. Staging ACME account (CA URL selects staging; default is prod)
curl -s -b "$COOKIE" -X POST $API/acme/accounts \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","caUrl":"https://acme-staging-v02.api.letsencrypt.org/directory"}'

# 1b. Order one staging cert covering both sites (or run twice, one per site)
curl -s -b "$COOKIE" -X POST $API/certificates/issue \
  -H 'Content-Type: application/json' \
  -d '{"domains":["site1.example.com","site2.example.com"],"provider":"letsencrypt-staging","email":"admin@example.com","challengeType":"http-01","autoRenew":true}'

# 1c. Confirm expiry/issuer is the staging CA ("Fake LE")
curl -s -b "$COOKIE" $API/certificates | head -c 2000
```

UI flow: `Admin → ACME → New ACME Account` (paste the staging directory URL
— the form now hints this), then `Admin → Certificates → Request
Certificate` with `site1.example.com, site2.example.com`, challenge
`HTTP-01`, auto-renew on. Browsers will show the staging cert as untrusted —
that is expected; it proves issuance works before touching prod rate limits.

Wildcard variant (DNS-01, per site):

```bash
curl -s -b "$COOKIE" -X POST $API/certificates/issue \
  -H 'Content-Type: application/json' \
  -d '{"domains":["*.example.com"],"provider":"letsencrypt-staging","email":"admin@example.com","challengeType":"dns-01","dnsProvider":"cloudflare","dnsCredentials":{"CF_DNS_API_TOKEN":"..."},"autoRenew":true}'
```

## 2. Production (same two domains)

```bash
# 2a. Prod account (omit caUrl → defaults to LE prod)
curl -s -b "$COOKIE" -X POST $API/acme/accounts \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com"}'
ACCT=<id from response>
curl -s -b "$COOKIE" -X PUT $API/acme/accounts/$ACCT \
  -H 'Content-Type: application/json' -d '{"isDefault":true}'

# 2b. Prod order (provider defaults to letsencrypt when omitted)
curl -s -b "$COOKIE" -X POST $API/certificates/issue \
  -H 'Content-Type: application/json' \
  -d '{"domains":["site1.example.com","site2.example.com"],"email":"admin@example.com","challengeType":"http-01","autoRenew":true}'

# 2c. Attach to each proxy domain (gateway starts serving it)
#     Get domain IDs from: curl -b "$COOKIE" $API/domains | jq '.data[].id'
curl -s -b "$COOKIE" -X PUT $API/domains/<domainId-site1> \
  -H 'Content-Type: application/json' \
  -d '{"certType":"letsencrypt","autoRenew":true,"https":true}'
curl -s -b "$COOKIE" -X PUT $API/domains/<domainId-site2> \
  -H 'Content-Type: application/json' \
  -d '{"certType":"letsencrypt","autoRenew":true,"https":true}'
```

UI flow: `Admin → Certificates → Request Certificate` (no staging provider),
then per domain `Admin → Certificates → Attach to Domain` (domain UUID from
`Admin → Domains`), or set `certType: letsencrypt` when creating/editing the
proxy domain. Manual renew: row → `Renew` (`POST /certificates/:id/renew`).
Renewing an operator-uploaded (`manual`/`custom`) cert is refused with 400 —
upload a replacement instead.

## 3. Renewal

- `acme.Service.StartAutoRenewal` ticks every **24 h** and renews rows with
  `auto_renew = true` and `expires_at <= now() + 30 days`
  (`FindExpiringCertificates`). Operator-supplied certs are skipped, never
  overwritten.
- Renewed material is pushed to the gateway (`deliverCertificate`); a renewal
  whose install fails is reported as failed, not success.
- Caddy path (`PUT /domains` with `certType: letsencrypt`) renews inside
  Caddy itself via its ACME automation policy; Traefik path renews from
  `/letsencrypt/acme.json`.

## 4. Storage location

- **Source of truth: Postgres** — `certificates` table (PEM + encrypted
  private key `private_key_encrypted`, DNS creds in `dns_credentials_encrypted`),
  `acme_accounts`, `dns_provider_accounts`, `certificate_attempts`
  (per issue/renew outcome). Migrations: `094_acme_certificates.sql`,
  `117_domains_certificates.sql`, `134_certificate_dns_provider.sql`,
  `135_acme_accounts.sql`, `157_encrypt_acme_account_keys.sql`.
- **Gateway copies**: Traefik `letsencrypt` docker volume
  (`/letsencrypt/acme.json`, see `infra/compose.tls.yml`); Caddy `caddy-data`
  volume; host-nginx `/etc/letsencrypt/live/<fqdn>/{fullchain,privkey}.pem`
  (see `infra/nginx.conf`, provisioned by certbot outside compose).
- Internal service mesh TLS (`tls-certs` volume, self-signed CA) is unrelated
  to public LE certs.

## 5. Reloading after cert changes

| Stack | Reload |
|---|---|
| API-issued (`/certificates/issue`) | None — the service installs into the gateway on issue/renew. Verify with `GET /certificates/:id` (`expiresAt`) |
| Caddy (`compose.caddy.production.yml`) | None for LE automation; after editing `Caddyfile.production`: `docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile` |
| Traefik (`compose.tls.yml`) | Automatic from `acme.json`; after config edits: `docker compose -f compose.yml -f compose.tls.yml up -d traefik` |
| Host nginx (`infra/nginx.conf`) | After certbot renewal or conf edit: `sudo nginx -t && sudo systemctl reload nginx` |

## 6. macOS dev vs Ubuntu prod

- **macOS / localhost cannot obtain a public LE cert**: the CA must reach
  your host on port 80 (HTTP-01) or you must own the DNS zone (DNS-01);
  `*.local`, `forge.local`, `localhost`, and private IPs are ineligible, and
  staging issuance from a laptop still requires public reachability. Dev uses
  Caddy `local_certs` (`infra/Caddyfile`) — trusted only after installing the
  local CA, never a real LE cert.
- **Ubuntu prod**: public IP + DNS A records + ports 80/443 open is the whole
  game. Prefer staging first (this runbook §1), then prod (§2); keep the
  staging account row around for future validation.
- Never copy prod `/letsencrypt/acme.json` or DB private keys to a laptop;
  use `POST /certificates/:id/export` sparingly (admin-scoped, audited).

## Gaps / follow-ups

- No `openapi.json`/`openapi.yaml` is generated — API docs live only in code.
- `/acme/dns-accounts` (store-backed) and `/dns/providers` (dns.Service) are
  two parallel DNS-credential stores; issuance consumes inline
  `dnsCredentials`, not either stored row. Unify or document which to use.
- `PUT /domains/:id` with `certType: custom` needs `certData`/`certKey` on the
  domain row; there is no "attach existing inventory cert by ID" endpoint —
  custom binding happens only at `POST /certificates` import time.
- Frontend `forge/web/lib/api/acme.ts` previously expected bare arrays/objects
  while the API wraps everything in `{ data }`; fixed here by unwrapping both
  shapes, with `setDefaultAcmeAccount`, `importDomainCertificate`, and
  `attachCertificateToDomain` helpers plus `lib/api/acme.test.ts` (6 tests).
