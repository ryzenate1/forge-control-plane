# Impl scope 9 — Operations B (reconciliation, orphans, drain, cleanup, failover, procedures, scheduler, autoscalers, affinity)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-9.md`, then
implement. Report to `.uiux-audit/impl/scope-9.md`.

You own: `app/admin/reconciliation/**`, `app/admin/orphans/**`, `app/admin/drain/**`,
`app/admin/cleanup/**`, `app/admin/failover/**`, `app/admin/procedures/**`, `app/admin/scheduler/**`,
`app/admin/autoscaler/**` (including `policy/[id]`), `app/admin/node-autoscaler/**`,
`app/admin/env-affinity/**`, `components/admin/AdminReconciliation.tsx`, `AdminOrphans.tsx`,
`AdminDrain.tsx`, `AdminCleanup.tsx`, `procedures-manager.tsx`, `node-autoscaler-manager.tsx`,
`env-affinity-manager.tsx`, `lib/api/reconciliation.ts`, `lib/api/cleanup.ts`, `lib/api/drain.ts`.

**Already done for you:** `app/admin/autoscaler/page.tsx:108` had an unterminated template literal
(`encodeURIComponent(serverId}`) that broke the whole build. Repaired.

1. **Unknown → zero/healthy is your dominant S1 class.** Reconciliation KPIs have no last-scan signal
   (`lib/api/reconciliation.ts:3-8`); `lib/api/cleanup.ts:11-27` defaults a malformed response to 0/0
   "clean"; autoscaler detail renders `?? 0` (`policy/[id]/page.tsx:217-231`); `AdminDrain.tsx:245-246`
   shows "0 remaining of 0". `AdminCleanup.tsx:58` already does it right — use it as the pattern for
   the whole slice.
2. **Cancelled drains render as fully complete** (`AdminDrain.tsx:66` marks every step done for
   `cancelled`). Use `DrainProgressStep.state` directly.
3. **"Simulate crash" performs a real failover write** (`app/admin/failover/page.tsx:273` →
   `handlers_failover.go:82`), unconfirmed, and always shows green "Crash handled". Rename the control
   to what it does, confirm it with the blast radius, and report the actual outcome.
4. **Orphaned Resources is a failed-deletion queue, not a scanner** — no discovery endpoint exists, so
   an empty list does NOT mean no orphans, and the registry description ("servers and databases with
   no owning record") is a promise no backend keeps. Check `lib/api/drain.ts:52-60` (`DetectOrphans`
   hint returning `[]`). Until a scan exists, relabel the page honestly as the queue it is *in the
   page copy* (registry is frozen — report the label change) and never render empty-as-clean.
5. **Confirmation asymmetry**: `node-autoscaler-manager.tsx:84`, `:106`, `:121` use native
   `window.confirm` — replace with `useConfirm`. Crash trigger, drain cancel, metric ingest, mid-case
   constraint deletes and fleet-wide resync all skip confirmation (`Failover:273`, `Drain:249-256`,
   `Scheduler:407`, `env-affinity:183`).
6. **Capability violations**: scheduler hardcodes docker/k3s/nomad pins (`scheduler/page.tsx:438-442`)
   instead of using its own backends list, and node-autoscaler always-enables scale-in despite a
   documented server-side failure (`node-autoscaler-manager.tsx:184-190`). Drive options from the API;
   disable with a reason.
7. **Developer vocabulary leaks to operators**: `store_reconcile.go:13` strip
   (`AdminReconciliation.tsx:262-265`), table names in card descriptions and query-string syntax in a
   heading (`node-autoscaler-manager.tsx:149`, `:239`).
