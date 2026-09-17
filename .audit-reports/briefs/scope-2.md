# Scope 2 — Compose HTTP + stack templates (audit and FIX, write code directly)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering
(handlers receive a `Config` struct of service pointers), route registration (about 100 `register*Routes`
calls inside `NewServer` in internal/http/server.go, plus the `RegisterPhaseRegistrar` + `init()` pattern —
a registrar only runs if some file calls RegisterPhaseRegistrar inside init()), and the two invariants:
never report success for work not performed; never resolve an ambiguous node target silently.

## A previous pass was interrupted mid-edit
Re-read the CURRENT content of every file first and repair half-applied changes: unused or missing imports,
declared-but-unused variables, a helper called but never defined, a signature changed at only some call sites.

## Files you own — edit ONLY these
- forge/api/internal/http/handlers_compose.go
- forge/api/internal/http/handlers_compose_templates.go
- forge/api/internal/http/handlers_templates.go
- forge/api/internal/services/composetemplates/service.go
- forge/api/internal/store/store_compose_templates.go
- forge/api/internal/store/store_templates.go
Read-only reference, do NOT edit: internal/http/server.go, internal/services/compose/*.go,
forge/web/lib/api/compose.ts, forge/web/lib/api/compose-templates.ts.

## Bug classes, endpoint by endpoint
1. Unwired handlers — this repo's documented failure mode is a handler that compiles and returns 404 because
   no route line was added. Enumerate every handler function in your three handler files (`grep '^func'`), then
   confirm each is mounted inside registerComposeRoutes / registerComposeTemplateRoutes / registerTemplateRoutes.
   Add the missing registrations. registerTemplateRoutes is invoked at server.go:2557 — verify, and flag any
   path registered twice where a later route shadows an earlier one.
2. Authorization gaps — a stack or template read/mutate that looks up by ID with no org/tenant filter;
   template create/edit/delete that should require an admin scope but does not; a server-bound action missing
   requireServerPermission. Stack routes carry a documented cross-tenant secret disclosure history — inspect
   env/var fields on stack responses especially closely.
3. Honest responses — 200 with an empty list on error (indistinguishable from "no stacks"); {"ok":true} after a
   swallowed service error; a nil service pointer bypassed with a silent empty result instead of a clear 503/501;
   a stale status field presented as current; wrong codes (500 for a 404/403/409 condition, 201 with no resource).
4. Boundary validation — bodies parsed with no size limit; name/slug unconstrained; a template body interpolated
   into generated YAML without escaping (template injection into a compose file is a real escalation path);
   pagination negative or unbounded; a compose file accepted whose parse result was ignored.
5. Contract drift — compare paths, verbs and JSON field names AND casing (snake_case vs camelCase is a known trap)
   against compose.ts and compose-templates.ts. Fix Go when Go is wrong; list web-side mismatches in
   "Needs elsewhere".

Fix everything in place, matching existing handler style. No new dependencies or abstraction layers. Never
weaken a validation or authorization check to make a path work. No comments explaining what code does; one
short line only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-2-compose-http-templates.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed` (file:line — problem -> change), `## Unwired endpoints`,
`## Authorization gaps`, `## Needs elsewhere` (exact file, function, change), `## Checked clean`.
Return a summary under 250 words.
