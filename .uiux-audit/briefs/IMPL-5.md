# Impl scope 5 — Infrastructure A (nodes, regions, docker, mounts, locations, capabilities, tokens, cloud)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-5.md`, then
implement. Report to `.uiux-audit/impl/scope-5.md`.

You own: `app/admin/nodes/**`, `app/admin/regions/**`, `app/admin/docker/**`, `app/admin/mounts/**`,
`app/admin/locations/**`, `app/admin/capabilities/**`, `app/admin/onboarding-tokens/**`,
`app/admin/cloud/**`, `app/admin/containers/**`, `app/admin/beacons/**` + `app/admin/infra/**`
(alias stubs → `/admin/nodes`), `app/admin/storage/**` + `app/admin/volumes/**` (stubs → mounts),
`components/admin/AdminNodes.tsx`, `AdminRegions.tsx`, `AdminLocations.tsx`, `AdminCapabilities.tsx`,
`AdminOnboardingTokens.tsx`, `AdminMounts.tsx`, `AdminHealth.tsx`.

**Frozen (report only):** `node-select.tsx`, `beacon-workspace.tsx`, `telemetry-ui.tsx`.

**Already done for you:** `AdminNodes.tsx` had 42 syntax errors from a half-finished `AdminTable`
migration (JSX classes written as bare attributes) and a precedence bug where
`desiredState ?? draining ? "draining" : …` made every active node read "Desired state: draining".
It compiles now and `:367` is fixed — re-verify the table renders.

1. **Failed telemetry renders as "Offline" or as empty** — `AdminNodes.tsx:358`,
   `AdminRegions.tsx:83-84`, and the docker views under `app/admin/docker/**`
   (`containers-view.tsx:274-278`). An unreachable node is *unknown*, not down, and never "no data".
2. **Fleet fan-outs present partial data as complete.** `handlers_docker.go:113-121` drops failed
   nodes but returns 200; `lib/api/docker.ts:88-110` flattens it; "Prune Unused" is fleet-wide,
   unscoped in its confirm, and reports success on partial failure (`volumes-view.tsx:82-84`, `:170`).
   The Go side is frozen for you — so make the UI honest about partial results with what it receives,
   scope the prune to a selected node, and put the affected node in the confirm text. If the payload
   cannot distinguish partial failure, say so under "Needs central change" with the field you need.
3. **No page renders `FreshnessBadge`** despite using `useNodesQuery`, and
   `AdminNodes.tsx:331` sets `KpiDatum.live = Boolean(cap)` — a live claim derived from "we got some
   data". Bind real freshness.
4. `AdminNodes.tsx:200-213` and `AdminOnboardingTokens.tsx:314` were mid-migration to `AdminTable`;
   confirm the tables are complete, sortable where the audit expects it, and inside a scroll container.
5. Onboarding tokens are secrets: verify masking, one-time reveal, and that a revoked/expired token
   cannot read as active.
