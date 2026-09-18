# Scope 10 — Web dashboard end-to-end (you are the ONLY agent allowed to drive a browser)

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 10.

The browser MCP instance is shared and single-tenant: **only you** may use Playwright/browser tools. Every
other agent tests through the API. Use that exclusivity — click through the real UI, and use the API helper
to check that what the UI shows is what the control plane actually says.

## Owned files

`forge/web/**` except `forge/web/lib/api/http.ts` and `forge/web/app/layout.tsx` (edit-forbidden shared
files — NEEDS-ORCHESTRATOR), `packages/ui/**`, `packages/shared-types/**` (coordinate: rebuilding
`shared-types` dist is required before frontend checks — run
`npm run build:packages` from the repo root if types changed; that is allowed, it does not restart services),
`lang/**` for catalog keys you must add.
Do **not** edit `forge/api/**`. When a UI defect is caused by the backend, reproduce it, name the exact
API/response that is wrong, and file it under NEEDS-ORCHESTRATOR so the owning scope's fix and yours don't
collide.

## Smoke checklist

1. **Compile sweep first.** One bad component 500s the entire dashboard at campaign start
   (`AdminMounts.tsx` had an unbalanced JSX tag). Run `cd forge/web && npx tsc --noEmit`, and a parse sweep
   over every `.tsx` so a syntax error in a file nobody visited does not hide. Report anything unparseable as
   S1 and fix it.

2. **Every admin route renders.** Enumerate the routes from `components/admin/admin-registry.ts` (that is
   the source of truth for admin navigation — do not hand-count the `app/admin` directories, and note if the
   two disagree, which is itself a finding). For each route: load it in the browser, confirm it renders a
   real page (not a 500, not a blank shell, not an endless skeleton, not a "no data" that is actually an
   swallowed error), and check the browser console for errors and the network tab for 4xx/5xx.
   Log every failure as `route → symptom → cause`. A page that renders but whose primary query is failing
   and showing an empty state is a **FAIL**, not a pass.

3. **Create → edit → delete through forms** for at least one representative resource per feature area:
   server, app/app-store install, mount, domain, node, backup schedule, cron job, notification channel,
   git credential, preview deployment, allocation/port, firewall rule, certificate request. Verify the form's
   payload matches what `lib/api/*` and the handler expect — this repo's recurring bug is a UI field name the
   backend never reads, so the save "succeeds" and the setting is lost. After every save, reload the page and
   confirm the value is still there. That reload check is the test; a toast is not proof.

4. **Capability gating and honest controls.** Buttons that cannot work for the selected node (offline node,
   unsupported runtime, missing scope) must be disabled or refuse with a reason — never present an enabled
   control that silently no-ops. Node B is offline and node A is the only online Docker node, which gives you
   a real gating case.

5. **States matrix** for the main list/detail pages: empty, loading, error, boundary (very long names, zero
   results, 100+ rows), permission-restricted, offline. Trigger them deliberately (stop trusting the seeded
   happy path): e.g. ask for a nonexistent id, use a limited user if scope 1's RBAC lets you create one.

6. **Live surfaces**: console, log stream, stats charts, notification toasts, deployment progress. Verify they
   show real data and that a stalled/absent stream is reported as disconnected rather than freezing at a stale
   value or rendering zeros as if they were measurements.

7. **Auth + navigation**: login, logout, protected-route redirects (unauthenticated `/admin/*` must redirect,
   not flash content), session expiry handling mid-page, and the sidebar reflecting actual permissions.

8. **i18n**: switch at least two locales; verify no raw keys (`common.foo_bar`) leak into the UI and that
   translated strings are not applied to technical values (ids, sizes, statuses) in a way that breaks them.

9. **Design-token compliance** (cheap to check, real debt): grep your touched files for raw hex colors and
   arbitrary Tailwind values where a `var(--token)` exists. Fix in files you own; do not mass-refactor.

## Notes

The dev server hot-reloads, so your edits take effect immediately — but the **API does not**. Frontend fixes
are verifiable by you; anything needing a backend change is NEEDS-ORCHESTRATOR. Keep the app compiling:
re-run `npx tsc --noEmit` before you finish, and make sure `curl -s -o /dev/null -w '%{http_code}'
http://localhost:3000/` is still 200 — several agents are depending on that not going red again.

## Report

`.smoke/reports/scope-10.md`. Include a route-by-route table for step 2 with the HTTP/render outcome; that
table is the artifact the orchestrator will re-run.
