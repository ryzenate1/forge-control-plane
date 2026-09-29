# Scope 8 — Preview environments + preview deployments (audit and FIX)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering, route registration,
and the two invariants — never report success for work not performed (unknown is not zero, not-reported is not
zero, a stale reading is not healthy); never resolve an ambiguous node target silently.

## A previous pass was interrupted mid-edit
`handlers_preview_deployments.go` (22:34) and `forge/web/app/admin/preview-deployments/page.tsx` show as
modified — possibly by the killed agent, possibly by the concurrent human editor. Re-read CURRENT content of
every file first; repair half-applied changes (unused or missing imports, declared-but-unused variables, a
helper called that does not exist, a signature changed at only some call sites).

## Files you own — edit ONLY these
- forge/api/internal/services/previewenv/service.go
- forge/api/internal/services/previewenv/previews.go
- forge/api/internal/services/previewenv/preview_deploy.go
- forge/api/internal/services/previewenv/preview_store.go
- forge/api/internal/services/previewenv/preview_webhook.go
- forge/api/internal/services/previewenv/webhook.go
- forge/api/internal/services/previewenv/providers.go
- forge/api/internal/services/previewenv/reaper.go
- forge/api/internal/http/handlers_preview_deployments.go
- forge/api/internal/store/store_preview_env.go
Read-only, do NOT edit: internal/http/server.go, internal/daemon/client.go,
forge/web/lib/api/preview-deployments.ts, forge/web/app/admin/preview-deployments/page.tsx.

## Wiring facts to verify
registerPreviewDeploymentRoutes runs at server.go:2592. handlers_preview_deployments.go:153 calls
RegisterPhaseRegistrar(previewEnvironmentRegistrarName, 160, registerPreviewEnvironmentRoutes) — confirm that
call is inside an `init()` in a built file, since a registrar defined but never registered produces silent 404s.

## Trace, then hunt
webhook event -> preview created -> deployed on a node -> URL and status reported -> expiry -> reaped.
1. Two webhook files (preview_webhook.go and webhook.go) — determine whether both are wired, whether they
   duplicate or conflict, and whether one is dead code: grep for every exported and unexported entry point.
   Signature verification must be constant-time and must FAIL CLOSED when the secret is unset. Check replay
   protection (timestamp tolerance plus nonce) and idempotency under duplicate delivery.
2. The reaper — the classic leak. Does it run at all (grep who constructs and starts it)? Does one reap error
   abort the whole pass instead of continuing? Is the DB row deleted before the workload is confirmed stopped
   (orphaned containers) or stopped without deleting (orphaned rows)? Are per-preview TTL and a global cap
   honoured? Is an UNKNOWN teardown state treated as reaped — it must not be? Are previews whose expiry
   timestamp was never written invisible forever?
3. Lifecycle honesty — a preview reported active/ready/healthy without a confirmed Beacon round-trip; a preview
   URL returned when no route was created; expiresAt never set so the reaper can never act; teardown errors
   swallowed while the row is marked removed; a failed create leaving behind what it already made (volumes,
   networks, DNS, subdomain registration, route entries).
4. Tenancy and RBAC — /api/v1/projects/:id/previews must verify the caller owns that project; deploy and
   teardown need per-server permission; look for ID-only lookups letting one org tear down another's preview.
5. Ambiguous targeting — preview create/deploy must not silently choose a node; provider selection must be
   explicit and validated.
6. Validation and injection — PR numbers, branch names and generated subdomains constrained and normalized (a
   crafted branch name producing an arbitrary host or path is a real bug); bodies size-limited; no unbounded
   listings.
7. Contract drift — paths, verbs and JSON field names AND casing (snake_case vs camelCase is a known trap)
   against lib/api/preview-deployments.ts and the preview page. Fix Go where Go is wrong; list web mismatches
   under "Needs elsewhere".

Fix everything in place, matching existing style. No new dependencies; no premature abstraction. Never weaken
validation, tenancy, or signature checks to make a path work. No comments explaining what code does; one short
line only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-8-preview-env.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Webhook and reaper verdict` (is each wired; what was broken),
`## Leak paths` (anything that can strand a running preview forever), `## Needs elsewhere`, `## Checked clean`.
Return a summary under 300 words, leading with the reaper verdict.
