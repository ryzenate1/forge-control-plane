# Forge smoke campaign — shared rubric (READ FIRST, IT IS BINDING)

Date: 2026-09-30. Branch `mvp-4`. Orchestrator = the main agent; you are one of ten scope agents.

## Mission

End-to-end smoke test of the running Forge stack, then **fix what is broken in the same pass**.
"Smoke test" here means: drive the real feature through its real path and prove the observable
outcome — not reading code and nodding. A feature is PASS only if you saw it work; anything else is
FAIL, or NOT-VERIFIABLE with a stated reason. Never report success for work you did not do.

## Live environment (verified by the orchestrator before dispatch)

| Thing | Value |
| --- | --- |
| API | `http://127.0.0.1:8080/api/v1` — running, healthy (pid from `.dev-pids/`) |
| Web | `http://localhost:3000` — Next dev server, compiles, `/` returns 200 |
| Beacon (node A) | `http://127.0.0.1:9090/health` → `{"ok":true,"runtime":true}` |
| Beacon SFTP | port 2022 |
| Postgres | 5432, `db=gamepanel user=gamepanel pass=gamepanel` |
| Redis | 6379, password `gamepanel` |
| Docker | Colima, `docker` CLI works directly, 10 CPUs / ~8 GB |
| Node A | `22222222-2222-2222-2222-222222222222` "Ubuntu Demo Node", **online**, baseUrl `http://127.0.0.1:9090` |
| Node B | `e43bb9aa-24c8-4900-a469-4656d45b55ad` "smoke-node-b", **offline**, baseUrl `http://192.168.31.189:9091`, sftp 2023 |
| Location | `cccccccc-cccc-cccc-cccc-cccccccccccc` "Local Lab" |
| Seeds | 3 servers, 2 apps, 0 mounts, 0 domains at start |

Node B exists so multi-node placement, reservations and cross-node flows are genuinely exercisable.
Keep it. Scope 9 owns making it online.

### Auth — source the helper, do not reinvent it

```bash
cd /Users/riyaz/forge-plane/forge-test && . .smoke/api.sh
api_probe /nodes          # "path  CODE | body-snippet"
api_body  /mounts         # body only
api_get   /servers        # body + trailing status line
api_post  /path '{"json":"body"}'
api_method PUT /path '{"json":"body"}'
api_delete /path
dbq "select count(*) from nodes"
```

Three traps that will waste your time if you ignore them (all cost the orchestrator a turn each):

1. **Use `127.0.0.1`, never `localhost`.** The cookie jar is domain-scoped to `127.0.0.1`; curl drops
   the session for `localhost` and you get a misleading `401 missing authentication`.
2. **Session cookie + `X-CSRF-Token` header go together** for anything mutating. The helper does this.
3. `go vet ./...` on `forge/api` reports failures in **test files** referencing symbols production no
   longer exports. That is test drift, not your regression. Check before "fixing".

`rtk` is documented in AGENTS.md as mandatory but **is not installed** on this machine. Run commands
plainly. A `command not found` empty result is not "the file is empty".

## Hard rules

- **Do not restart or kill** API / web / Beacon / Postgres / Redis. The running API is a prebuilt binary
  (`forge/api/api`); your source edits are NOT live until the orchestrator restarts it. Verify a fix by
  package-level test or by reasoning against the code, and tag it `needs-restart-to-verify`.
- **You may only edit files your scope brief lists as OWNED.** Need a change in someone else's file, or
  in `internal/http/server.go`, `cmd/api/main.go`, `forge/web/app/layout.tsx`, `lib/api/http.ts`, or
  `.smoke/api.sh`? Write it under **NEEDS-ORCHESTRATOR** in your report with the exact edit. These shared
  files are edit-forbidden because ten concurrent writers would clobber each other.
- Anything you create is named `smokeN-...` (N = your scope number) and deleted by you at the end.
  Report anything you cannot delete.
- Build/vet per module (`cd forge/api && go build ./...`; `cd forge/web && npx tsc --noEmit`). Never
  `go build -o <running binary path>` — that rewrites the live executable.
- No browser/Playwright unless your brief grants it. Exactly ONE agent (scope 10) drives the UI, because
  the browser MCP is a single shared instance.
- Prefer targeted tests (`go test -race -run TestX ./internal/services/deployment`). A full `go test ./...`
  here aborts on the first panic and masks everything after it.

## What "verified working" means

For each feature in your scope, walk the full arc, not just a GET:

1. **Read**: list + detail endpoint returns 2xx with a correctly shaped body.
2. **Write**: create/update/delete through the API and observe the effect in the API response *and* in
   Postgres (`dbq`). An endpoint that returns 200 but writes nothing, or writes and returns a stale copy,
   is a FAIL.
3. **Contract**: the field names the frontend sends/receives (`forge/web/lib/api/*`, `packages/shared-types`)
   match what the handler actually reads/returns. camelCase/snake_case and `Ip` vs `IP` mismatches are real
   bugs — one silently-ignored request field usually means a feature that appears to work and does nothing.
4. **Honesty**: unimplemented, unwired, stubbed, or "returns nil without doing the job" must be reported as
   broken, not as working. `unknown ≠ zero`; a stale reading ≠ healthy. If a control exists in the UI but no
   route backs it, or a route exists but nothing calls it, that is a finding.
5. **Fail-closed**: error paths must return errors, not `nil`/empty/200. Trigger at least one failure per
   feature (bad id, missing node, wrong tenant, over-capacity) and check the status code is the *right*
   class — a validation failure returning 500 is a bug (two of those are already known, see below).

## Severity

- **S1** data loss, security/tenancy bypass, or a feature that silently does nothing while reporting success
- **S2** feature unusable end-to-end (500s, 404s, broken contract, wrong state)
- **S3** wrong-but-usable: bad status class, misleading message, stale/incorrect aggregate, UI/UX defect
- **S4** polish

Fix every S1/S2/S3 you can inside your owned files. If you cannot fix it, say exactly why in
**BROKEN-NOT-FIXED** — "too hard" is only valid with the reason (needs schema change, needs another scope's
file, needs a product decision).

## Report discipline (this survives interruption)

Append to **your own report file** after every feature area you finish — do not batch writing until the end.
Prior campaigns lost ten agents at once to a context boundary and only the on-disk increments survived.

Report file: `.smoke/reports/scope-N.md`, this exact shape:

```markdown
# Scope N — <name>
## Summary
verified-pass: X   broken-found: Y   fixed: Z   broken-not-fixed: W   not-verifiable: V
## Pass
| feature | how verified (command + observed) |
## Fixed
### <symptom>
- severity / feature / root cause / file:line changed / how verified / needs-restart-to-verify: yes|no
## Broken-not-fixed
### <symptom>  — severity, why not fixed, exact edit needed
## Needs-orchestrator
- edits to forbidden/shared files, restarts needed, cross-scope conflicts
## Not-verifiable
- what + why (missing hardware, second node offline, external service absent)
## Notes for the final re-verify pass
- the exact commands the orchestrator should re-run after restart to confirm your fixes
```

Cite `file:line` only after you have opened the file. A finding can be real while its cited line is wrong —
verify before asserting.

## Already known at dispatch (do not re-discover; find the cause and fix it)

1. `POST /api/v1/nodes` returns **500** for plain validation failures that should be 4xx — observed for
   missing/invalid `baseUrl` and for a loopback `fqdn` ("fqdn must be a publicly routable hostname,
   not a loopback address" came back as `{"error":...}` with `HTTP=500`). Missing `LocationID` correctly
   returned 422, so the bug is in how one class of validation error is mapped. Scope 2 owns this.
2. `GET /api/v1/locations` reports `nodeCount: 0` while node A is in that location/region — stale or
   unimplemented aggregate. Scope 2 owns this.
3. Whole-web 500 at campaign start: unclosed JSX in `forge/web/components/admin/AdminMounts.tsx:650`
   (`</div>` where `</AdminPageLayout>` belonged). Orchestrator fixed it. Treat "one broken component
   poisons every route" as a live risk — scope 10 should syntax-sweep the tree, not just the routes it clicks.
