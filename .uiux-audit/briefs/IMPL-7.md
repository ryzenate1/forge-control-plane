# Impl scope 7 — Networking (12 routes)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-7.md`, then
implement. Report to `.uiux-audit/impl/scope-7.md`. Your slice produced the most findings
(43 S1 / 95 S2) — work the S1 list first, then copy drift, then tokens.

You own: `app/admin/domains/**`, `app/admin/certificates/**`, `app/admin/gateways/**`,
`app/admin/firewall/**`, `app/admin/dns/**`, `app/admin/acme/**`, `app/admin/endpoints/**`,
`app/admin/allocations/**`, `app/admin/load-balancer/**`, `app/admin/traffic/**`,
`app/admin/discovery/**`, `app/admin/crossnode/**`, plus the alias stubs `app/admin/networking/**`,
`app/admin/network/**`, `app/admin/infra/networking/**` (→ endpoints/domains),
`app/admin/traffic-policies/**`, `app/admin/dns-providers/**`, `app/admin/acme-accounts/**`,
`components/admin/AdminFirewall.tsx`, `AdminEndpoints.tsx`, `AdminEndpointDetail.tsx`,
`AdminAllocations.tsx`, `AdminDiscovery.tsx`, `AdminCrossnode.tsx`.

**Already done for you:** `app/admin/certificates/page.tsx` and `app/admin/domains/page.tsx` had 9
mismatched `</td>` closing tags against `<AdminTd>` openers — repaired, they compile now.

1. **DNS provider credentials render in plaintext.** `Input` accepts `type` and the field descriptor
   carries `type: "password"`, but the create form only uses it to pick a font
   (`app/admin/dns/page.tsx:153-162`) — API tokens display unmasked under copy claiming "stored
   encrypted" (`:149`). Fix the input type and masking. Check ACME accounts and Registries-style token
   fields for the same.
2. **`/admin/domains/[id]` is a different resource from its list and is orphaned.** The list manages
   `{serverId, domain, verified, wildcard}`; the detail renders `{hostname, serviceId, https}` with no
   `verified` and **no server/org attribution** (`:187-206`), reachable only from Security Headers.
   Decide from the Go handlers which resource is real and make the detail route consistent, or remove
   it and report. Add attribution either way — this group previously had a cross-tenant disclosure.
3. **10 of 12 routes report errors as empty states, or 0 as unmeasured** (`domains/page.tsx:153`,
   `traffic:193`/`:251`, `AdminAllocations.tsx:175-179`, `:213`, `AdminEndpointDetail.tsx:102`).
   `AdminFirewall.tsx:161-168` is your correct minority — copy that.
4. **Gateways and Traffic GET the same `/admin/traffic/rules`** with non-overlapping fields
   (`gateways:258` vs `traffic:66`); Gateways has no create path and calls four siblings "Legacy".
   Fix the false duplication and make each page's controls real. Do not merge pages (that's an IA
   decision) — but stop both claiming the same data differently.
5. **All six Networking aliases 404** — you found this; now create the stub pages you own (list above).
