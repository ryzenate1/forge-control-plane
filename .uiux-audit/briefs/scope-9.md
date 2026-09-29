# Scope 9 — Operations B: reconciliation, autoscaling, placement

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-9.md`, appending after each page.

All `secondary: true` — this is the specialised tail, where a user is most likely to meet a page
that looks like a different product.

| Registry label | href | files |
| --- | --- | --- |
| Reconciliation | `/admin/reconciliation` | `components/admin/AdminReconciliation.tsx` (uses `AdminPageHeader`) |
| Orphaned Resources | `/admin/orphans` | `components/admin/AdminOrphans.tsx` |
| Node Drain | `/admin/drain` | `components/admin/AdminDrain.tsx` (uses `AdminPageHeader`) |
| Cleanup | `/admin/cleanup` | `components/admin/AdminCleanup.tsx` |
| Failover | `/admin/failover` | `app/admin/failover/**` (uses `AdminPageHeader`) |
| Procedures | `/admin/procedures` | `components/admin/procedures-manager.tsx` |
| Scheduler | `/admin/scheduler` | `app/admin/scheduler/**` (uses `AdminPageHeader`) |
| Workload Autoscaling | `/admin/autoscaler` | `app/admin/autoscaler/**` (uses `AdminPageHeader`) |
| Node Autoscaling | `/admin/node-autoscaler` | `components/admin/node-autoscaler-manager.tsx` |
| Placement Affinity | `/admin/env-affinity` | `components/admin/env-affinity-manager.tsx` (uses `AdminPageHeader` per census) |

Focus areas:
- **Drift/observability pages must never render a stale reading as healthy** (project rule).
  Reconciliation, Orphans, Scheduler, Failover: check every "healthy / in sync / 0 orphans /
  no drift" against whether the check actually ran. A page that shows a green "in sync" when the
  scan never executed or failed is **S1** — the single worst UI failure in this slice.
- Two near-duplicate destructive-cleanup pages: **Cleanup** (`/admin/cleanup`, GC of stale platform
  resources) and **Image & Cache Cleanup** (`/admin/docker-cleanup`, Scope 6, disk pruning) and
  **Orphaned Resources**. An operator cannot tell these apart from their titles. Judge the naming
  collision and whether each has dry-run preview + confirmation.
- **Scheduler vs Placement Affinity vs Workload Autoscaling vs Node Autoscaling**: four placement
  pages, two of which both promise "explanations". Does the placement *explain* (scoring breakdown,
  why a node lost) or is `explain.go` surfaced as raw text? Report whether these could be one
  placement surface with tabs, and what the registry grouping hides.
- Node Drain: lifecycle + evacuation progress — real progress, cancellation, and whether a drained
  node's state is visible on the Nodes page. Cross-reference Scope 5 only to note the gap.
- Procedures: runbooks/workflows — check the execution history vs definition editing, step status,
  and whether a failed step is distinguishable from a skipped one.
- Autoscaling: policy editing UX — min/max validation, current-vs-desired replica display, cooldown,
  and whether a policy that can never fire (target beyond capacity) is flagged.
- `*-manager.tsx` vs `Admin*.tsx` vs inline `page.tsx`: this slice uses all three conventions.
  Count them and show the header component each one produces.
