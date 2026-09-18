# Scope 8 — Git integration, webhooks, builds, buildpacks, app store, catalog, previews

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 8.

Git integration was explicitly named by the user. The whole arc is: provider credential → repo → webhook →
build → artifact → deploy → preview. Test the arc, not the endpoints in isolation.

## Owned files

`forge/api/internal/http/handlers_git.go`, `handlers_git_deploy.go`, `handlers_gitpush.go`,
`handlers_phase1_git.go`, `handlers_builds.go`, `handlers_buildpacks.go`, `handlers_forgefile.go`,
`handlers_appstore.go`, `handlers_catalog.go`, `handlers_templates.go`, `handlers_preview_deployments.go`,
`handlers_source_deployments.go`, `handlers_apphosting.go`, `handlers_revisions.go` (coordinate with
scope 7 if the bug is in the deploy step), `handlers_webhooks*.go`, `handlers_nests.go` is scope 2 — do
not edit it, `forge/api/internal/services/git/`, `gitprovider/`, `gitpush/`, `phase1git/`, `build/`,
`buildpack/`, `forgefile/`, `appstore/`, `catalog/`, `previewenv/`, `preview*`, `eggseeder/`, `webhook/`,
plus matching `_test.go`. `beacon/internal/git*` if present.
Frontend: `forge/web/lib/api/git*.ts`, `forge/web/lib/api/builds*.ts`, `forge/web/lib/api/app-store*.ts`,
`forge/web/components/admin/AdminGit*.tsx`, `AdminAppStore*.tsx`, `forge/web/app/admin/git/**`,
`app/admin/app-store/**`, `app/admin/catalog/**`.

## Smoke checklist

Provider credentials: GitHub, GitLab, Bitbucket, and generic git. Store a credential and prove it is
encrypted at rest (this repo has a history of a `git_credentials` AAD mismatch that bricks API startup —
verify a credential written by the current code can be read back by the current code, and that a bad
credential produces a usable error instead of a startup failure). **Never restart the API** — if you
suspect a credential write can brick startup, test in a scratch copy of the code path with a Go test, and
flag it loudly in NEEDS-ORCHESTRATOR.

Repo access without a token: public repos over `https://` should work without credentials; verify a
private repo attempt produces an authentication error (not a generic 500, not an empty branch list that
looks like "no branches"). Branch/commit listing against a real public repo you choose; verify pagination
and that a deleted/renamed repo is reported honestly.

Webhooks: register → the provider-side URL/secret the panel hands back → deliver a simulated push (POST to
the webhook endpoint with the correct signature, then with a bad signature). Verify: bad signature is
rejected, correct signature triggers a deployment, and the event is recorded. Verify webhook deletion
removes it. Verify a webhook for a repo belonging to another tenant does not fire.

Build pipeline: create a `smoke8-*` app from a real public repo, trigger a build, follow the log stream,
and verify: build log is real (not empty/echoed), stages are recorded, a failing build (bad Dockerfile /
missing script) ends in `failed` with the actual error preserved, and retries work. Check buildpack and
Forgefile paths separately: `/forgefile`, `/servers/:id/buildpacks`, `/builds` list + `/:buildId` detail.
Verify a build for a node that cannot run the chosen runtime is refused up-front rather than failing later.

App store / catalog: list, search, detail, install. Known trap: app-store seeding previously failed because
`tags` was sent as TEXT where the column is jsonb (or the reverse), making the list endpoint error or return
empty. Verify the list is non-empty and that tags survive a round trip; verify install actually creates a
workable server/app with correct defaults, and that installing an app whose requirements exceed node
capacity is refused honestly.

Preview deployments: create a preview for a branch/PR (simulated webhook), verify the generated subdomain,
that the preview deploys and serves, that TTL expiry actually tears it down, and that manual delete works.
Verify two previews for the same app don't collide on subdomain or port.

Source deployments / app-hosting: the known history is a **fail-closed refusal with no runtime
consistency** — i.e. the deploy path refuses and the UI shows a success. Prove which it does now. If a step
cannot do its job it must return an error; a `nil` from an unimplemented branch is S1.

## Report

`.smoke/reports/scope-8.md`. Include the exact build/preview IDs you created and their terminal states.
