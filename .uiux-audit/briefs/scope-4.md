# Scope 4 — Deploy group

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-4.md`, appending after each page.

| Registry label | href | primary | files to open |
| --- | --- | --- | --- |
| Deployments | `/admin/deployments` | yes | `app/admin/deployments/page.tsx` (402 lines, uses BOTH `AdminPageLayout` *and* `DashHeader`/`InfoCard`), `app/admin/deployments/new/page.tsx`, history/revisions sub-routes |
| Compose Stacks | `/admin/compose` | yes | `app/admin/compose/page.tsx`, `compose/new/`, `compose/[id]/` |
| Pipelines | `/admin/pipelines` | yes | `app/admin/pipelines/page.tsx` (uses `DashHeader` **and** `AdminPageHeader` — both) |
| Git Integrations | `/admin/git` | yes | `app/admin/git/**` |
| Stack Templates | `/admin/compose-templates` | More | `app/admin/compose-templates/**` |
| Preview Environments | `/admin/preview-environments` | More | `app/admin/preview-environments/**` |
| Preview Deployments | `/admin/preview-deployments` | More | `components/admin/preview-deployments-view.tsx`, `[id]/page.tsx` |
| Source Deployments | `/admin/source-deployments` | More | `app/admin/source-deployments/**`, `[id]/page.tsx` |
| Zero-Downtime Releases | `/admin/zerodowntime` | More | `components/admin/zerodowntime-manager.tsx` |

Plus alias stubs: `app/admin/git-providers/**`, `app/admin/deploy/**`.

This is the slice with the most severe frame collisions — **three files already import both header
systems in the same file** (`deployments/page.tsx`, `pipelines/page.tsx`, `compose/*`). Document
exactly what the operator sees when two headers stack.

Focus areas:
- Deployment **status honesty**: the registry promises "Rolling and blue-green deployments,
  revisions and history". Check status pills map to a real observed state, that `pending` /
  `in_progress` / `completed` / `failed` / `rolled_back` are distinguishable by more than colour,
  and that a failed deploy surfaces the *reason* (project memory: compose deploy error details are
  stripped by `ResponseError` — look for a UI that shows "Deployment failed" with no message).
- Four near-identical "list of build/deploy records" pages: Deployments, Preview Deployments,
  Source Deployments, Preview Environments. Do they share a table, filters, time formatting,
  status badge? Or does each hand-roll one? Name every divergence.
- Compose: YAML editing/validation affordance, import flow, and the per-service view — is a compose
  stack presented as a document or as a set of services?
- Git Integrations: provider credentials masking, webhook status, branch lists. Does a connection
  failure show as healthy?
- Pipelines: run history vs pipeline definition — two concerns in one page? Check the run detail
  route and step logs.
- Zero-Downtime Releases: "health gates" — are gates shown as pass/fail/pending with real evidence,
  or as static labels?
- Time handling across the slice: grep `formatDate`, `toLocaleString`, relative-time helpers —
  inconsistent time formats within one group is a classic drift; report the distinct formats used.
