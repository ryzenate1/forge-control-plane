# Impl scope 4 — Deploy group

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-4.md`, then
implement. Report to `.uiux-audit/impl/scope-4.md`.

You own: `app/admin/deployments/**`, `app/admin/compose/**`, `app/admin/compose-templates/**`,
`app/admin/pipelines/**`, `app/admin/git/**`, `app/admin/git-providers/**`,
`app/admin/preview-environments/**`, `app/admin/preview-deployments/**`,
`app/admin/source-deployments/**`, `app/admin/zerodowntime/**`, `app/admin/deploy/**` (alias stub →
deployments), `components/admin/zerodowntime-manager.tsx`, `preview-deployments-view.tsx`,
`lib/api/git-admin.ts`.

**Already done for you:** `app/admin/git/page.tsx` was missing 22 identifiers (the whole
`admin-ui` import plus the three queries' `isLoading`/`isError`/`error`/`refetch`). It now compiles.
Verify it renders sensibly — check the tabs all have real loading/error states and that nothing else
was silently dropped when those imports went missing.

1. **Deployments' two tabs are one endpoint.** `app/admin/deployments/page.tsx:53` and
   `lib/api/apps.ts:443` both `GET /admin/deployments`, rendered twice with two status vocabularies
   and two tone derivations that disagree 40px apart (`:179` vs `:32-38`); `rolled_back` renders grey
   while `lib/api/status.ts:86` says otherwise. Collapse to one table and one status source — use the
   shared status helper, delete the page-local maps. You found **9 private status maps** in this
   slice and `sourceStatusTone`/`serverDeploymentStatusTone`/`buildStatusTone` with zero consumers —
   delete the dead ones.
2. **Frame collision is in modals**: `deployments` and `pipelines` stack `DashHeader` *inside*
   `Modal`, repeating title/status/description 2-3x. Remove the `DashHeader` from modals; a modal
   needs a plain title.
3. **Failed reads render as empty lists at 8 sites**; unmeasured shown as measured: compose shows
   `Health: Healthy` derived from status, unknown status → `failed`, preview TTL shows "uncapped"
   while loading, `0 || "—"`. Fix each to loading → error → empty precedence.
4. **Eight different time formats** across the slice. Standardise on the shared `formatDate`.
5. Detail routes: `/admin/deployments/[id]` passes a title expression (`Deployment: <8 chars of id>`)
   — that is legitimate, but make the id readable and keep the frame's `<h1>` single.
