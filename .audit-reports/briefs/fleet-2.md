# Scope 2 — Nodes, Regions, Locations, Capabilities, Onboarding: FRONTEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first — tier boundaries, the three invariants, shared-file rules,
reporting protocol. You are scope 2; report to `.audit-reports/reports/fleet-2.md`.

## You own (edit only these)

- `forge/web/components/admin/AdminNodes.tsx`, `AdminRegions.tsx`, `AdminLocations.tsx`, `AdminCapabilities.tsx`,
  `AdminOnboardingTokens.tsx`, `onboarding-manager.tsx`, `node-select.tsx`, `node-autoscaler-manager.tsx`
- `forge/web/app/admin/nodes/page.tsx`, `app/admin/nodes/[id]/page.tsx`, `regions/page.tsx`, `locations/page.tsx`,
  `capabilities/page.tsx`, `onboarding/page.tsx`, `onboarding-tokens/page.tsx`, `autoscaler/page.tsx`
- `forge/web/lib/api/status.ts`, `capabilities.ts`, `onboarding.ts`, `discovery.ts`, `nodeautoscale.ts`
- the node/region/location/capability/onboarding/autoscaler sections of `lib/api/query-keys.ts` and the re-exports in
  `lib/api.ts` — surgical, your lines only.

Read every owned file completely before editing. Then READ (do not edit) the backend to verify the contract:
`forge/api/internal/http/handlers_admin.go` (nodes/regions/locations — scope 1's files), `handlers_locations.go`,
`handlers_regions.go`, `handlers_capabilities.go` and `phase8_onboarding.go` (scope 3's files), plus
`forge/api/internal/daemon/client.go` where relevant.

## Hunt for, with extreme depth

1. **Contract drift**: a client calling a path or verb the API never registers; wrong param name; a node/region id
   string-concatenated into a URL without `encodeURIComponent`; response shape mismatch, including the known
   snake_case-vs-camelCase decoding hazard.
2. **Erased failures** — the top priority here. Every `?? []`, `.catch(() => [])`, `|| []`, `?? 0`, `|| 0` and
   `if (error) return null` in your files. An errored or unloaded query must never render as "0 nodes", "no
   capabilities" or "no drift". Each list/detail surface needs distinct empty / loading / error / permission-denied /
   node-unreachable rendering.
3. **Honest UI numbers**: capacity bars or usage percentages that render 0 for a metric the node never reported;
   a node shown `online` because a boolean defaulted false; capability drift tables that show nothing rather than
   "probe failed / not reported"; an onboarding token UI implying success without the returned secret being shown
   exactly once. **This directly contradicts the repo rule "unknown is not zero".**
4. **Nav/registry honesty**: `admin-registry.ts` marks all 17 audited entries `capability: "available"` (90 of 91
   entries are `available`, one is `metadata-only`). For YOUR entries, verify against the backend whether the page
   genuinely works end to end; if it does not, do not silently flip the label — implement what is missing where you
   can, and otherwise report the mismatch precisely with the evidence.
5. **react-query correctness**: missing or shared query keys; no `enabled` guard when a node id is absent from
   route state; aggressive `refetchInterval` on detail pages; mutations that never invalidate the list they changed;
   optimistic updates with no rollback on error; subscriptions/timers not cleaned up on unmount.
6. **Ambiguous node targeting in the UI**: `node-select.tsx` — check whether it silently defaults to the first node
   when none is chosen. The API rejects ambiguous targets, so a control that fires with no explicit node is a bug.
   Destructive actions (drain, evacuate, rotate token, delete node) need an explicit confirmation naming the node.
7. **Access control**: admin-only mutations rendered enabled for non-admins; secrets rendered without an explicit
   reveal action.
8. **Style compliance**: raw hex colours, magic px inline styles, duplicated table/badge/card markup that should use
   `components/ui/`, new UI strings without `labelKey`/`descriptionKey` i18n keys.

## Report requirements

Add a `## Contract matrix` section: each client function you touched -> the backend route it targets -> verified
present/absent -> response-shape match yes/no. End with `## Cannot verify`.
