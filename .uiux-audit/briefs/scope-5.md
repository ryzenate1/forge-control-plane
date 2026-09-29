# Scope 5 — Infrastructure A: fleet, placement, storage, capabilities

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-5.md`, appending after each page.

| Registry label | href | primary | files to open |
| --- | --- | --- | --- |
| Nodes | `/admin/nodes` | yes | `components/admin/AdminNodes.tsx` (1291 lines, `DashHeader` — a reference page) + all `app/admin/nodes/**` detail routes |
| Regions | `/admin/regions` | yes | `components/admin/AdminRegions.tsx` |
| Containers | `/admin/docker` | yes | `app/admin/docker/**` |
| Storage Mounts | `/admin/mounts` | yes | `components/admin/AdminMounts.tsx` |
| Locations | `/admin/locations` | More | `components/admin/AdminLocations.tsx` |
| Node Capabilities | `/admin/capabilities` | More | `components/admin/AdminCapabilities.tsx` |
| Onboarding Tokens | `/admin/onboarding-tokens` | More | `components/admin/AdminOnboardingTokens.tsx` |
| Cloud Instances | `/admin/cloud` | More | `app/admin/cloud/**` |

Plus alias stubs `app/admin/containers/**`, `app/admin/beacons/**`, `app/admin/infra/**`,
`app/admin/storage/**`, `app/admin/volumes/**`. Also `components/admin/node-select.tsx` and
`components/admin/beacon-workspace.tsx` (shared node pickers — used across the whole product).

Focus areas:
- **Nodes is the most important page in the product and the model for honest telemetry.** Audit it
  properly: heartbeat/online/offline derived from what exactly; capacity shown as absolute or
  percentage; does an unreachable node render as healthy or as unknown? Is drift between desired and
  reported capability visible? Copy the `FreshnessBadge` binding pattern in detail — Scope 1 is
  writing the target frame and will judge everyone against it.
- Nodes ↔ Regions ↔ Locations is a triple of "where things run". Three near-empty CRUD pages with
  different tables? Report whether they could be one page with tabs, and whether the registry's
  split is defensible.
- Containers (`/admin/docker`): node selector, images/networks/volumes sub-views. Does it require
  picking a node before showing anything, and does it say so? (Project rule: never resolve an
  ambiguous target silently — a node-less global view that silently picks node 1 is **S1**.)
- Node Capabilities: "capability inventory and drift" — check the disabled-with-reason convention
  (rubric M) is honoured here, since this page is what should *feed* capability gating everywhere
  else. If this page lies, gating lies everywhere.
- Onboarding Tokens: token creation is a secret — masking, one-time reveal, expiry, approve/revoke
  flow, and whether a revoked token can still be mistaken for active.
- Cloud Instances: provider credentials, instance state vs provisioning state, and whether
  unsupported providers are hidden or disabled-with-reason.
- `node-select.tsx` / `beacon-workspace.tsx` are shared: judge whether they present consistent
  node naming (hostname? id? region?) — inconsistency here propagates to every page that uses them.
