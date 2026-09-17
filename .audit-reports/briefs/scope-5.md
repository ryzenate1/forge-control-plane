# Scope 5 — gitpush receiver + phase1 git links (audit and FIX, write code directly)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering, route
registration (including `RegisterPhaseRegistrar` called from an `init()` — defining a registrar is not enough),
and the two invariants: never report success for work not performed; never resolve an ambiguous node target
silently.

## A previous pass was interrupted mid-edit
`forge/api/internal/http/phase1_registrar.go` is currently modified in git. Re-read CURRENT content of every
file first; repair half-applied changes (unused or missing imports, declared-but-unused variables, a helper
called that does not exist, a signature changed at only some call sites).

## Files you own — edit ONLY these
- forge/api/internal/services/gitpush/service.go
- forge/api/internal/http/handlers_gitpush.go
- forge/api/internal/store/store_gitpush.go
- forge/api/internal/services/phase1git/service.go
- forge/api/internal/services/phase1git/browse.go
- forge/api/internal/services/phase1git/deploykey.go
- forge/api/internal/services/phase1git/oauth.go
- forge/api/internal/services/phase1git/store.go (only if it exists)
- forge/api/internal/http/handlers_phase1_git.go
- forge/api/internal/store/store_phase1_gitlinks.go
Do NOT edit services/git/*, handlers_git.go, compose/gitops.go, store_git_deployments.go — other scopes own those.

## Context
gitpush accepts a `git push` over HTTP and mounts the pushed content. phase1git is the older git-link surface
(browse, deploy keys, OAuth). Both expose a PUBLIC node-facing webhook route on the `v1` router plus an admin
surface on the authenticated `protected` router. Known wiring points to VERIFY: registerGitPushRoutes at
server.go:2615, registerGitPushWebhookRoutes at server.go:2358, and phase1_registrar.go:16 calling
RegisterPhaseRegistrar("phase1-git", 40, ...) — confirm that call sits inside an `init()` in a built file.

## Bug classes
1. Unauthenticated push surface — the highest-risk code you own. Every mutating endpoint must be authenticated
   by node credentials or a verified signature. Signature verification must be constant-time (hmac.Equal /
   crypto/subtle) and must FAIL CLOSED when the secret is unset. Verify the signed bytes are exactly the body
   received, not a re-marshaled version. Reject refs and repo names used as filesystem paths or shell arguments:
   path traversal via a ref, branch or filename is the classic bug; also reject refs beginning with `-`.
2. OAuth — the state/nonce parameter generated, persisted and verified on callback (CSRF on the callback is a
   real bug class here); authorization code redeemed once; redirect URI matched EXACTLY, not by prefix; tokens
   stored encrypted and never returned to the client; tokens kept out of URL queries so they cannot reach logs.
3. Deploy keys — generated with adequate strength; the private key never returned after creation; a key
   installed on a host is revoked when the link is deleted, and a revocation failure is REPORTED not swallowed.
4. Browse — path components normalized and validated before use (no `..` escape, no absolute path); no unbounded
   tree read; no listing that exposes secrets.
5. Honesty — a git link reported connected/verified without a live check; a stale lastVerified shown as current;
   an empty listing returned when the upstream errored.
6. Wiring — enumerate handler functions in handlers_gitpush.go and handlers_phase1_git.go and confirm each is
   actually mounted. A compiled-but-404 endpoint is this repo's known failure mode.
7. Store — missing tenancy filters; cross-org lookup by bare ID; missing uniqueness where a link must be
   one-per-repo; unbounded result sets; driver errors leaked to clients.

Fix everything in place, matching existing style. No new dependencies; no premature abstraction. Never remove or
weaken a security control to make code pass. No comments explaining what code does; one short line only when the
WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-5-gitpush-phase1git.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Security findings` (push surface, OAuth, deploy keys,
traversal), `## Unwired endpoints`, `## Needs elsewhere`, `## Checked clean`.
Return a summary under 250 words, leading with the worst security finding.
