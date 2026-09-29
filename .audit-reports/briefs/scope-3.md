# Scope 3 — Git core integration (audit and FIX, write code directly)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root for layering, route
registration, and the two invariants: never report success for work not performed (unknown is not zero, a
stale reading is not healthy); never resolve an ambiguous node target silently.

## A previous pass was interrupted mid-edit
`forge/api/internal/http/handlers_git.go` was modified at 22:33 — possibly by the killed agent, possibly by the
concurrent human editor. Re-read CURRENT content of every file before judging it; repair half-applied changes
(unused or missing imports, declared-but-unused variables, a helper called that does not exist, a signature
changed at only some call sites).

## Files you own — edit ONLY these
- forge/api/internal/services/git/service.go
- forge/api/internal/services/git/checks.go
- forge/api/internal/services/git/validate.go
- forge/api/internal/http/handlers_git.go
- forge/api/internal/services/gitprovider/service.go
- forge/api/internal/store/store_git_sources.go
- forge/api/internal/store/store_git_providers.go
- forge/api/internal/store/store_git_providers_new.go
- forge/api/internal/store/store_git_credentials.go
Do NOT touch services/git/deploy.go, deploy_service.go, deployment_service.go, handlers_git_deploy.go,
handlers_gitpush.go, handlers_phase1_git.go, compose/gitops.go, store_git_deployments.go, store_gitpush.go,
store_phase1_gitlinks.go — other scopes own those.

## Historical issues to RE-VERIFY against current source (do not trust the old reports)
(a) a cross-tenant secret disclosure reachable through an env-var resolution route;
(b) a git_credentials AAD mismatch that bricked API startup.
Confirm the current code actually guards both, fix whatever is still open, and state your verdict with evidence.

## Bug classes
1. Secret handling — provider tokens, OAuth refresh tokens and SSH private keys logged, returned by a list/get
   endpoint, embedded in an activity or audit entry, or placed in a URL query. Masking must be non-reversible and
   applied on EVERY read path. Decryption must fail closed: a decrypt/AAD failure returns an error, never an empty
   or partial value a later call treats as valid. Verify encrypt and decrypt agree on nonce and associated-data
   construction, and that the encoded column width fits the ciphertext.
2. Tenancy — any query keyed only by ID with no org/workspace filter; cross-org resolution by guessed ID; one org
   reading or deleting another's credential, source or provider.
3. Provider API correctness — pagination not followed so a repo/branch/commit list is truncated and reported as
   complete; rate-limit or error responses parsed as an empty result ("no branches" hiding an API failure);
   webhook secrets compared with `==` instead of constant-time; base-URL assumptions that break self-hosted
   GitLab/Gitea/Forgejo.
4. Injection — repository names, refs, branch names, submodule URLs and .gitconfig content are hostile. Reject
   refs beginning with `-` (argument injection), validate refs against a whitelist pattern, refuse path traversal
   in clone/worktree paths, never interpolate user input into a shell command.
5. Status honesty — checks.go reporting a connection or repo verified/connected without a live round-trip; a
   stale lastVerified or commit SHA presented as current; a check that swallows its error and returns healthy.
6. Wiring — enumerate handlers in handlers_git.go and confirm each is mounted by registerGitRoutes
   (server.go:2614) or registerGitWebhookRoutes (server.go:2353). Add missing registrations; flag shadowed paths.
7. Store — missing uniqueness where a source must be one-per-repo; sql.ErrNoRows mapped to a zero value;
   unbounded listings; driver error strings leaked to clients.

Fix everything in place, matching existing style. No new dependencies or abstraction layers. Never remove or
weaken a security control, signature check, or authorization guard to make code pass. No comments explaining
what code does; one short line only when the WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.

## Deliverable — write incrementally
Report file: `.audit-reports/scope-3-git-core.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Security findings`,
`## Verdict on the two historical issues`, `## Unwired endpoints`, `## Needs elsewhere`, `## Checked clean`.
Return a summary under 250 words, leading with the security findings.
