# Scope 9 — Source deployments, build, pipeline, buildpack, Forgefile, app-hosting (audit and FIX)

Repo: /Users/riyaz/forge-plane/forge-test. FIRST read `agents.md` at the repo root: layering, route registration,
and the invariants — never report success for work not performed; never resolve an ambiguous node target
silently. Panel->Beacon is HTTP via internal/daemon.Client.

## A previous pass was interrupted mid-edit
Re-read CURRENT content of every file first; repair half-applied changes (unused or missing imports,
declared-but-unused variables, a helper called that does not exist, a signature changed at only some call sites,
platform-specific files left inconsistent with each other).

## Files you own — edit ONLY these
- forge/api/internal/http/handlers_source_deployments.go
- forge/api/internal/store/store_source_deployments.go
- forge/api/internal/services/pipeline/service.go, actions.go, artifacts.go, model.go, scheduler.go, store.go
- forge/api/internal/services/build/service.go, types.go, id.go, proc_unix.go, proc_windows.go
- forge/api/internal/services/buildpack/buildpack_service.go
- forge/api/internal/services/forgefile/service.go
- forge/api/internal/services/apphosting/service.go
- forge/api/internal/http/handlers_builds.go, handlers_buildpacks.go, handlers_forgefile.go, handlers_apphosting.go
- forge/api/internal/store/store_builds.go, store_buildpacks.go, store_apphosting.go
Read-only, do NOT edit: internal/http/server.go, internal/daemon/client.go, beacon/internal/server/build.go,
forge/web/lib/api/source-deployments.ts.

## Wiring facts to verify
registerSourceDeploymentRoutes at server.go:2620, registerBuildRoutes at 2617, registerBuildpackRoutes at 2618,
registerAppHostingRoutes at 2636, registerForgefileRoutes at 2644. Confirm the individual routes INSIDE each of
those bodies exist for every handler function defined in the file — a handler that compiles but is never mounted
returns 404, which is this repo's documented failure mode.

## Two documented historical bugs to re-verify against current source
1. App-hosting deploy gave a fail-closed refusal while still claiming runtime consistency — verify apphosting
   never asserts a runtime agreement it did not actually check.
2. Compose deploy error details were stripped by a generic wrapper — the team explicitly requires Beacon's error
   detail to pass through to the API response. Check every wrapping fmt.Errorf / ResponseError path in your files.

## Trace, then hunt
source/app created -> build triggered (locally or remotely on Beacon) -> artifact produced -> app-hosting
workload deployed -> status, URL and logs reported.
1. Build honesty — a build marked success when the process was killed, timed out, or its exit code never checked;
   cmd.Wait() error discarded; stderr captured then dropped so the user sees "build failed" with no reason; a
   build reported complete when the artifact was never produced or pushed; log-stream failures swallowed.
2. Process handling — signal and kill-group correctness in proc_unix.go (negative PID / process group) mirrored
   in proc_windows.go; orphaned children on cancel; pipes never closed (deadlock once a buffer fills); no
   context deadline or timeout; temp dirs and build workspaces never removed (disk leak); a build directory
   reused across concurrent builds of the same app.
3. Artifact integrity — an image name, tag or artifact path that can collide across apps or orgs; digests not
   verified; a stale artifact served for a new commit.
4. Pipeline scheduler — a stage proceeding despite a failed dependency; a scheduler tick aborting after one
   error; retries unbounded or without backoff; a queued job claimed twice because there is no claim token or
   lock; cancelled jobs still executed; a per-stage timeout missing.
5. Forgefile / buildpack parsing — a field parsed but never applied IS a bug (silently ignored config must error
   or warn, per the honesty invariant); build args, env values, entrypoints, commands and mount paths
   interpolated into a generated Dockerfile, compose file or shell without escaping — a repo's contents are
   hostile input; path traversal in a build context.
6. Tenancy — build logs, app env vars and secrets are the documented cross-tenant disclosure vector. Verify every
   log-read, env-read and detail endpoint filters by org, and that list endpoints never return full secrets.
7. Ambiguous targeting — build and deploy must require an explicit node; reject when absent rather than choosing.

Prioritize: data-integrity and honesty bugs, then security and tenancy, then wiring, then resource leaks. Fix
everything in place, matching existing style. No new dependencies; no premature abstraction. Never swallow an
error or weaken a check to make a path work. No comments explaining what code does; one short line only when the
WHY is non-obvious.

## Constraints
Do NOT run go build, go vet, gofmt, tests, lint, npm or make. Code-only; verification is central afterwards.
Keep proc_unix.go and proc_windows.go consistent with each other — only one compiles on this machine.

## Deliverable — write incrementally, not at the end
Report file: `.audit-reports/scope-9-source-deploy-build.md`. Append findings after EACH file you finish.
Sections: `## Half-applied repairs`, `## Fixed`, `## Honesty and security findings`, `## Resource leaks`,
`## The two historical bugs` (resolved or still open, with evidence), `## Unwired endpoints`, `## Needs elsewhere`,
`## Checked clean`.
Return a summary under 300 words.
