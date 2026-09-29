# Scope 10 — Cross-tier contracts: daemon client <-> Beacon <-> web (audit and FIX)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root for the tier boundaries:
Panel->Beacon is HTTP via internal/daemon.Client; websockets carry console/stats/log streams only, never
commands; web has exactly ONE HTTP primitive, requestJSON/fetchJSON in forge/web/lib/api/http.ts; admin
navigation is data-driven from components/admin/admin-registry.ts. Enforce the two invariants: never report
success for work not performed; never resolve an ambiguous node target silently.

## A previous pass was INTERRUPTED MID-EDIT ON daemon/client.go — this is your first job
That file was edited at 21:49 by an agent that was killed before reporting or verifying anything. Audit it for
half-applied changes and make it internally consistent BEFORE anything else: unused or missing imports,
declared-but-unused variables, a helper called that does not exist, a signature changed but not at every call
site, an error now returned where a caller still ignores it, a guard inserted mid-block leaving unreachable code.

## Files you own — edit ONLY these
Panel client:
- forge/api/internal/daemon/client.go
- forge/api/internal/daemon/compose.go
Beacon server side:
- beacon/internal/server/compose.go
- beacon/internal/server/git.go
- beacon/internal/server/build.go
- beacon/internal/server/build_ext.go
Web clients:
- forge/web/lib/api/compose.ts, compose-templates.ts, deployments.ts, deployment-rollbacks.ts, git-admin.ts,
  git-deployments.ts, preview-deployments.ts, source-deployments.ts, resource-limits.ts
- forge/web/lib/api.ts — ONLY if a module is not re-exported
- forge/web/components/admin/admin-registry.ts — ONLY if a page exists but is missing from the nav
Do NOT edit forge/api/internal/http/*, forge/api/internal/services/*, or beacon/internal/runtime/* — other scopes
own those; report those bugs under "Needs elsewhere".

## Bug classes
1. Pair every panel->Beacon call with its Beacon handler. For each request the daemon package makes (compose
   up/down/status/logs, git clone/pull/deploy-key, build), find the serving method in beacon/internal/server/*.
   Compare HTTP method, exact path, request JSON field names AND casing, response shape, and error semantics.
   Fix whichever side is wrong. A field that unmarshals to a zero value because of a struct-tag mismatch is a
   critical bug — casing drift between snake_case payloads and camelCase Go tags is a documented trap here.
2. Client hygiene in daemon/client.go — calls with no timeout or context deadline; retries on non-idempotent
   operations; a non-2xx response parsed as success; a Beacon error body dropped instead of surfaced (error
   detail passthrough is an explicit team requirement); node credentials appearing in logs; TLS verification
   skipped; an absent node silently falling back to "the first node with credentials" — that violates the
   ambiguity invariant and must be rejected.
3. Beacon honesty — compose/git/build handlers returning ok/healthy when the underlying runtime call errored;
   status served from a stale cache with no age bound; a container that started then failed reported as running;
   build exit codes ignored; "not supported on this host" reported as success instead of an explicit refusal.
   Judge workload lifecycle by the STATE endpoint, never by the stats stream — absence of stats does not mean
   stopped.
4. Web <-> panel contract — for each lib/api module, verify path, verb and payload keys against the routes
   actually registered by forge/api/internal/http/server.go and its register*Routes bodies (read, do not edit).
   Find 404s from a wrong path, casing mismatches, a response field the UI reads that the API never sends, and
   any UI that renders 0 or empty where the API said nothing — it must show unknown instead. Fix the web side;
   list backend-only mismatches under "Needs elsewhere".
5. Nav wiring — pages that exist under forge/web/app/admin/{compose, compose-templates, deployments, git,
   git-providers, preview-deployments, preview-environments, source-deployments} but are missing from
   admin-registry.ts, or registered under a wrong path.
6. Bare fetch — any fetch() in your web modules that bypasses requestJSON/fetchJSON.

Fix everything in place, matching each tier's existing style. No new dependencies. Extend existing helpers rather
than introducing a second client or a new UI primitive. Where the truth is unknown, the code must say unknown —
never fake a value to make a contract line up. No comments explaining what code does; one short line only when
the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, npm, tsc, eslint, tests, lint or make. Code-only; verification is central
afterwards. Verify by reading: imports, signatures, and TypeScript types that genuinely line up with what you send.

## Deliverable — write incrementally, not at the end
Report file: `.audit-reports/scope-10-cross-tier.md`. Append findings after EACH file you finish so an
interruption cannot erase the record.
Sections: `## Half-applied repairs in daemon/client.go` (FIRST), `## Contract mismatches fixed` (panel call <->
beacon/web handler, the field or path that disagreed, which side you changed), `## Honesty fixes`,
`## Unpaired or dead calls` (a client method with no server handler, or a handler no client reaches, with grep
evidence), `## Needs elsewhere`, `## Checked clean`.
Return a summary under 300 words.
