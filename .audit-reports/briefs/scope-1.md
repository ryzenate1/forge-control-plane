# Scope 1 — Compose engine + store (audit and FIX, write code directly)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: it defines the layering
(handlers -> services -> store) and the two invariants you must enforce —
(1) never report success for work not performed: unknown is not zero, not-reported is not zero, a stale
reading is not healthy; (2) never resolve an ambiguous node target silently. Discarded errors are bugs.

## A previous pass was interrupted mid-edit
Re-read the CURRENT content of every file before judging it. Repair half-applied changes so each file is
internally consistent: unused or missing imports, declared-but-unused variables, a helper called that does
not exist, a signature changed at only some call sites, an error now returned where a caller ignores it.

## Files you own — edit ONLY these
- forge/api/internal/services/compose/service.go
- forge/api/internal/services/compose/controller.go
- forge/api/internal/services/compose/lifecycle.go
- forge/api/internal/services/compose/parser.go
- forge/api/internal/services/compose/import.go
- forge/api/internal/services/compose/queue_handler.go
- forge/api/internal/store/store_compose.go

Do NOT touch compose/gitops.go or compose/git_deploy_adapter.go (scope 4 owns them), and do not edit anything
under internal/http (scopes 2 and 7 own those). Other agents and a concurrent human editor are in this tree;
ignore breakage outside your list.

## Known lead — confirm then fix
controller.go discards the deployment response twice: `_ = deployResp` appeared at both line 235 and line 320.
Fix both so a failed or empty deploy response changes the stack's recorded state instead of being thrown away.
Judge the neighbouring `_ = c.store.UpdateComposeStack(...)` and `_ = c.store.ReleaseComposeStackClaim(...)`
sites individually: a lost claim release or a lost state write is a bug, not best-effort noise.

## Bug classes
1. Honesty: a stack recorded active/deployed/healthy without a confirmed Beacon round-trip; container,
   replica or service counts defaulted to 0 where the truth is unknown; `unknown` collapsed into 0 or into
   success; an error assigned then discarded; a function returning nil when its work could not run.
2. Reconcile loop: claims never released on an error path; a claim released by a worker that does not own it;
   a stack stuck terminal-but-claimed forever; the loop aborting after one error instead of continuing; a
   ticker/timer never stopped; a goroutine with no shutdown path; ctx cancellation ignored; no backoff.
3. Parser fidelity — a silently dropped compose field changes runtime behavior, so it must become a validation
   error or a surfaced warning: YAML anchors/aliases, `extends`, `depends_on` long-form conditions, `build`
   (context/dockerfile/args/target), `env_file`, interpolation and defaults, `profiles`, `healthcheck`,
   `restart`, ports with protocol/range, volumes long-form and bind options, `networks`, `secrets`/`configs`,
   `command`/`entrypoint` forms, `deploy.resources`.
4. Store: lookup by ID without an org/tenant filter; an UPDATE matching zero rows that returns nil;
   sql.ErrNoRows converted to a zero value; multi-row writes without a transaction; SQL built by string
   concatenation; unbounded result sets.
5. Concurrency: maps or slices shared between the controller and HTTP handlers without synchronization.

Fix everything in place, matching the existing hand-rolled style. No new dependencies, no new abstraction
layers, no new files unless a fix truly has nowhere else to live. Never weaken a check or lower a threshold
to make code pass. No comments explaining what code does; one short line only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only — verification happens centrally
afterwards. Compensate by reading precisely: every call must match a real signature in the current file.

## Deliverable — write incrementally, not at the end
Report file: `.audit-reports/scope-1-compose-engine.md`. After you finish EACH source file, append your
findings for it immediately (Write, then Edit to extend), so an interruption cannot erase the record.
Sections: `## Half-applied repairs`, `## Fixed` (file:line — problem -> change),
`## Discarded errors reviewed` (each `_ =`, fixed or kept with reason),
`## State transitions that can still lie`, `## Needs elsewhere` (exact file, function, change),
`## Checked clean`.
Return a summary under 250 words. Do not overstate — if something is still broken, say so.
