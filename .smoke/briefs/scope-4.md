# Scope 4 — Networking I: domains, proxy, routes, ingress, load balancer, DNS, mTLS

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 4.

## Owned files (`forge/api/`)

`internal/http/handlers_domains.go`, `handlers_proxy_domains.go`, `handlers_redirects.go`,
`handlers_endpoints.go`, `handlers_trafficmanager.go`, `handlers_loadbalancer.go`, `handlers_gateways*.go`
if present, `handlers_dns.go`, `handlers_dns_providers*.go` if present, `handlers_mtls.go`,
`handlers_servicediscovery.go`, `handlers_external.go`, `handlers_cloudflare*.go` if present,
`internal/services/domains/`, `internal/services/domainsenv/`, `internal/services/redirects/`,
`internal/services/trafficmanager/`, `internal/services/loadbalancer/`, `internal/services/dns/`,
`internal/services/servicediscovery/`, `internal/services/mtls_migrator.go`,
`forge/api/internal/daemon/` client pieces for **route sync only** (compose pieces belong to scope 7),
plus matching `_test.go`.

You do **not** own certificates/ACME/firewall — that is scope 5. Do not edit
`internal/services/acme`, `handlers_certificates*.go`, `handlers_acme_accounts.go`, `handlers_firewall.go`.

## Smoke checklist

Domains: create `smoke4-<n>.example.test` on a real server, list, get, patch, delete. Prove the domain
row, the panel's view of it, and the **gateway's actual route table** agree. If route sync to the gateway
is skipped, stubbed, or returns success without pushing anything, that is S1 — the UI will show a domain
that routes nothing.

Routes: add/remove routes for a workload; verify path-based and host-based matching, that a route pointing
at a stopped workload reports honestly, and that deleting a workload cleans up its routes (no dangling
backend). Check the former IDOR surface: the env-var-resolved endpoint route was org-guarded — re-prove
the guard still holds and a request for another tenant's domain is refused, not silently filtered.

Ingress: `/api/v1/.../ingress/rules`, `/policies`, `/backends`, `/route-groups`, `POST /ingress/sync`,
`GET /ingress/health/stats`, `GET /ingress/stats`, `POST /ingress/cleanup`. Verify sync is idempotent
(two syncs produce one set of rules, not duplicates), that `cleanup` removes only orphans and refuses to
remove live ones, and that stats are fresh (not a cached zero).

Load balancer: backends, weights/algorithm, health-based ejection (an unreachable backend must leave the
pool and be reported as ejected, not "healthy"), and re-add on recovery. Verify cross-node backends
resolve — with node B offline, LB must not silently include it as a live target.

DNS: provider CRUD, record add/list/delete, propagation status reporting. A provider that cannot be
reached must produce a visible error, never a fake "verified". If DNS provider credentials are required
and absent, that is a NOT-VERIFIABLE item — record it as such and verify the fail-closed path instead
(what does the panel show when the provider call fails?).

mTLS: certificate-pinning/verification settings per endpoint; verify an insecure/absent-client-cert
connection is rejected and that the setting round-trips through the API (send it, read it back, check the
gateway config actually changed).

Service discovery: `/api/v1/.../resolve`, `/services`, `GET /describe/:host/:port`,
`POST /cache/clear`, `POST /cache/ttl`. Prove cache invalidation actually invalidates (change a backend,
clear cache, see the new answer) and that TTL is honoured rather than ignored.

Cross-cutting honesty check: for every networking endpoint, confirm "unknown" is not reported as zero.
A domain with no observed traffic must not read as healthy-and-serving.

## Report

`.smoke/reports/scope-4.md`.
