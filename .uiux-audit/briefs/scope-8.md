# Scope 8 — Operations A: the day-2 primaries

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-8.md`, appending after each page.

| Registry label | href | primary | files |
| --- | --- | --- | --- |
| Operations | `/admin/operations` | yes | `components/admin/AdminOperations.tsx`, `components/admin/OperationsTimeline.tsx` |
| Backups | `/admin/backups` | yes | `app/admin/backups/page.tsx`, `components/admin/billing-manager.tsx` if referenced |
| Backup Engines | `/admin/backups/engines` | More, `parent: /admin/backups` | `app/admin/backups/engines/page.tsx` |
| Migrations | `/admin/migrations` | yes | `components/admin/AdminMigrations.tsx` |
| Cron Jobs | `/admin/cron-jobs` | yes | `app/admin/cron-jobs/**` |
| Docker Events | `/admin/docker-events` | More | `components/admin/docker-events-feed.tsx` |

Plus alias stub `app/admin/operations/advanced/**` and `app/admin/logs/**`
(`/admin/logs → /admin/activity`, owned by Scope 1 — check only that the stub behaves).

Focus areas:
- **Backups vs Backup Engines is a deliberate two-page split** (classic policies/jobs/artifacts vs
  Restic/Kopia engines; different APIs). The registry explains it; the UI must too. Can an operator
  tell which backup system a given artifact came from? Does one page's empty state look like the
  other's failure? Report whether the split is communicated or just two similar tables.
- Backup/restore is where **invented numbers do the most damage**: check size, count, last-run,
  retention and verification status against real sources. An unverified restore marked "Ready" or a
  backup whose size is unknown rendered as 0 is **S1**.
- Operations: "control-plane operation history and manual controls". Two jobs in one page — is the
  manual-controls half capable of triggering something destructive without confirmation? Does the
  timeline show job status from the queue or from a stale read?
- Migrations: workload migration/evacuation. Check progress is real (percentage of what?), that a
  stuck migration doesn't render as in-flight forever, and that it links to the affected node
  (Scope 5's `node-select`).
- Cron Jobs: cron expression authoring — is there a human-readable next-run preview, validation,
  and a real last-run/next-run pair? Report if the page accepts an invalid schedule silently.
- Docker Events is a **live stream**: check connection state is visible (connected/reconnecting/
  offline), that it doesn't claim "live" when the socket is down, that it bounds memory (event cap /
  virtualisation), and offers filtering/pause. Compare its freshness handling to `FreshnessBadge`
  (Scope 1's target frame).
- Note the timeline/feed/table idiom each page picks and whether they could share one primitive.
