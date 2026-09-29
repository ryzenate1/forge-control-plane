# Scope 7 — Networking group (12 routes)

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-7.md`, appending after each page.

| Registry label | href | primary | files |
| --- | --- | --- | --- |
| Domains | `/admin/domains` | yes | `app/admin/domains/page.tsx` (297 lines, `AdminPageLayout`+`AdminTable`), `domains/[id]/`, `components/admin/AdminEndpointDetail.tsx` |
| Certificates | `/admin/certificates` | yes | `app/admin/certificates/**` |
| Gateways | `/admin/gateways` | yes | `app/admin/gateways/**` |
| Firewall | `/admin/firewall` | yes | `components/admin/AdminFirewall.tsx` |
| DNS Providers | `/admin/dns` | More | `app/admin/dns/**` |
| ACME Accounts | `/admin/acme` | More | `app/admin/acme/**` |
| Endpoints | `/admin/endpoints` | More | `components/admin/AdminEndpoints.tsx`, `endpoints/[id]/` |
| IP Allocations | `/admin/allocations` | More | `components/admin/AdminAllocations.tsx` |
| Load Balancer | `/admin/load-balancer` | More | `app/admin/load-balancer/**` |
| Traffic Policies | `/admin/traffic` | More | `app/admin/traffic/**` |
| Service Discovery | `/admin/discovery` | More | `components/admin/AdminDiscovery.tsx` |
| Cross-Node Routing | `/admin/crossnode` | More | `components/admin/AdminCrossnode.tsx` |

Plus alias stubs `app/admin/network/**`, `app/admin/infra/networking/**`,
`app/admin/traffic-policies/**`, `app/admin/dns-providers/**`, `app/admin/acme-accounts/**`, and the
deep-link `/admin/networking → /admin/endpoints`.

Focus areas:
- **Twelve pages, one domain.** Is there any shared visual language for a hostname / URL / IP:port /
  certificate? Report every distinct rendering of the same concept (mono vs sans, truncation, copy
  affordance, protocol prefix shown or not).
- **Status semantics**: verified/unverified domain, issued/expiring/expired/failed certificate,
  active/stale endpoint, healthy/unhealthy upstream. Do these use one `Pill` tone vocabulary with
  consistent colours and text, or does each page invent its own? Check thresholds ("expires in N
  days" → warning) are real and not hardcoded per page.
- **This group had a cross-tenant secret disclosure in the env-var resolved route** (since fixed).
  Re-verify tenancy guards are visible in the UI: does an endpoint/domain page state which
  organization/server it belongs to, or does it present a fleet-wide list an operator can't scope?
- Gateways vs Traffic Policies vs Load Balancer vs Cross-Node Routing vs Service Discovery: five
  overlapping "how traffic moves" pages. Judge whether an operator could find where to add a rate
  limit, or a redirect, or an upstream. Report the IA confusion concretely, with the labels as
  written.
- DNS Providers and ACME Accounts are credential-bearing (API tokens, private keys). Check masking,
  and whether a broken credential is shown as valid.
- FireWall: rule creation form quality — port ranges, protocol select, source CIDR validation, and
  whether an invalid rule is blocked before submit.
- Domains page: `checkDNS` + `verifyDomain` + `removeServerDomain` mutations — check pending/failed
  states are shown, that removal is confirmed, and that the page honestly reports which server a
  domain is attached to. Does it require a server selection first?
- Endpoints detail (`AdminEndpointDetail.tsx`) is shared with `/admin/endpoints/[id]` — check it
  reads as a detail of Endpoints and not a peer page.
