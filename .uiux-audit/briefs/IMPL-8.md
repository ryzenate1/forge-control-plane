# Impl scope 8 — Operations A (operations, backups, engines, migrations, cron, docker events)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-8.md`, then
implement. Report to `.uiux-audit/impl/scope-8.md`.

You own: `app/admin/operations/**`, `app/admin/backups/**` (including `backups/engines/`),
`app/admin/migrations/**`, `app/admin/cron-jobs/**`, `app/admin/docker-events/**`,
`app/admin/logs/**` (alias stub → `/admin/activity`), `app/admin/operations/advanced/**` (stub →
operations), `components/admin/AdminOperations.tsx`, `AdminMigrations.tsx`, `OperationsTimeline.tsx`,
`docker-events-feed.tsx`, `lib/api/admin-backups.ts`, `lib/api/backupengine*` if present.

1. **Restic snapshots shown as measured zeros.** `app/admin/backups/engines/page.tsx:521-522` renders
   size/file-count through a local `formatBytes` that maps missing → `"0 Bytes"` (`:44-50`), and the
   server only populates these for Kopia (`backupengine/service.go:1254-1262` vs `:1371`). Every
   Restic backup reads as a measured zero. Render "Not reported" for the missing case. Go is frozen —
   report the field you need.
2. **Backups KPIs invented while loading** (`app/admin/backups/page.tsx:461-486` prints `0` for all
   eight status figures during load/failure; `:660` artifact size `0 Bytes`; `:662` collapses
   unverified and failed-checks into one "No"). **Restore is offered on any artifact with no
   verification precondition and no confirm** (`:673`, `:227-256`) while every delete does confirm —
   gate restore on real verification state and confirm it with its blast radius.
3. **Cron Jobs: failure rendered as empty** (`page.tsx:340`, `:381-385` ignores `isError` → "No cron
   jobs configured"), validation is field-count only (`:39-44`), and an unschedulable job shows no
   next-run and no warning (`:405-409`). Show next-run from the API; state clearly when a schedule
   can't be parsed.
4. **No freshness anywhere in your slice, and two pages claim liveness falsely**: Docker Events shows
   a pulsing green "live 5s" beside its own error banner (`docker-events-feed.tsx:314-322`), and
   `AdminOperations.tsx:477-478` sets "Connected" before the socket resolves. Bind real connection
   state (connected / reconnecting / offline).
5. **`OperationsTimeline.tsx` has zero importers** although `/admin/operations/timeline` exists, so the
   header's "operation history" promise is false. Read the handler; wire the timeline for real or
   remove the claim.
6. Backups ⇄ Backup Engines split is unexplained — neither page names the other and `repo.artifactId`
   (the only provenance link) is fetched then dropped. Add the cross-link and show provenance.
