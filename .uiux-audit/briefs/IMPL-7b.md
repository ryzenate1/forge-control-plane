# Impl scope 7 (continued) — Networking, remaining 9 routes

A previous agent worked this slice and was cut off by a turn limit after **3 of 12 routes**. Its
report is `.uiux-audit/impl/scope-7.md` — READ IT FIRST. It is high quality and its decisions are
settled; do not re-litigate or redo what it already did.

**Already complete — do not touch:** `app/admin/dns/page.tsx`, `app/admin/dns/**`,
`app/admin/domains/page.tsx`, `app/admin/domains/**` (including `[id]`). Its findings there stand,
including the verified conclusion that `/admin/domains` (server-scoped `DomainRecord`) and
`/admin/domains/[id]` (`store.ProxyDomain`) are **different backend resources**, proven by reading
`handlers_domains.go:41` and `handlers_proxy_domains.go:121` — `ProxyDomain` has no `serverId`,
`organizationId` or `verified` column. Do not try to "make the detail match the list".

**First job: make the tree internally consistent.** I already added the missing
`FreshnessBadge`/`sourceState` imports to `dns/page.tsx`. Still outstanding:
`app/admin/domains/[id]/page.tsx:162` — `form.cspPolicy` is possibly undefined. Fix that without
changing the validation behaviour the previous agent built (CSP cannot be enabled with an empty policy).

## Your remaining routes

| Route | files | state pattern to copy |
| --- | --- | --- |
| Certificates | `app/admin/certificates/page.tsx` | `AdminFirewall.tsx:161-168` |
| Gateways | `app/admin/gateways/**` | idem |
| Firewall | `components/admin/AdminFirewall.tsx` | already correct; verify, don't rewrite |
| ACME Accounts | `app/admin/acme/**` | idem |
| Endpoints | `components/admin/AdminEndpoints.tsx` | idem |
| Endpoint detail | `components/admin/AdminEndpointDetail.tsx`, `app/admin/endpoints/[id]` | **was mid-edit when the agent died — check it compiles and is coherent before extending it** |
| IP Allocations | `components/admin/AdminAllocations.tsx` | idem |
| Load Balancer | `app/admin/load-balancer/**` | idem |
| Traffic Policies | `app/admin/traffic/**` | idem |
| Service Discovery | `components/admin/AdminDiscovery.tsx` | idem |
| Cross-Node Routing | `components/admin/AdminCrossnode.tsx` | idem |

Plus the alias stub pages your slice owns, which do not exist yet: `app/admin/networking/**`,
`app/admin/network/**`, `app/admin/traffic-policies/**`, `app/admin/dns-providers/**`,
`app/admin/acme-accounts/**`, `app/admin/infra/networking/**`. Each must be a real redirect page so
`test/route-integrity.test.ts` ("every alias source is served by a real page") passes — that test
currently fails on exactly these. Copy an existing stub (`app/admin/containers/page.tsx`) rather than
inventing a form.

## Carried-over priorities from the audit

1. **Ten of twelve routes report a failed read as an empty state, or an unmeasured value as 0** —
   `traffic:193`/`:251`, `AdminAllocations.tsx:175-179`/`:213`, `AdminEndpointDetail.tsx:102`
   (`0`-while-loading, swallowed errors, `undefined MB`, `NaN%`). This is the S1 list.
2. **Gateways and Traffic both GET `/admin/traffic/rules` and render non-overlapping field sets**
   (`gateways:258` vs `traffic:66`); Gateways has no create path. Do not merge them (product decision) —
   but stop them claiming the same data differently, and give Gateways a real create or remove the claim.
3. Consistent hostname / IP:port / URL cell rendering is a scope-level pattern the previous agent
   deferred as central — you may add a small local helper in your own files, but flag the shared
   primitive in "Needs central change"; do not edit `admin-ui.tsx`.
4. Secrets: ACME private keys and provider tokens masked, never in a `title` attribute or DOM text.

Follow `.uiux-audit/briefs/IMPL-shared.md` exactly (frozen files, honesty rules, token rules).
Append to `.uiux-audit/impl/scope-7.md` under a `## Continued` heading after each page.
