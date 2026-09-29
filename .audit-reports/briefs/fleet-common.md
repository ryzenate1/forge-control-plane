# FLEET AUDIT 2026-09-29 — shared context for all ten scope agents

You are one of ten agents auditing disjoint slices of the same tree. This file is shared context. Your slice,
your file ownership, and your hunt list live in the scope file named in your prompt. Follow BOTH.

Repo: `/Users/riyaz/forge-plane/forge-test`, branch `mvp-4`. FIRST read `/Users/riyaz/forge-plane/forge-test/agents.md`
— it is authoritative on layout, commands, conventions and pitfalls.

## The three tiers

| Path | What it is |
| --- | --- |
| `forge/api` | Go control-plane API (Fiber v2), module `gamepanel/forge`. Authority for state, auth, orchestration. |
| `beacon` | Go per-host agent. Runs workloads, reports health, exposes host inspection, SFTP, console. |
| `forge/web` | Next.js 15 App Router dashboard (`@forge/web`). |

## Architecture facts (verified this session — do not re-derive, do not "fix" these)

- Layering is `handlers (forge/api/internal/http/handlers_*.go) -> services (internal/services/<pkg>) -> store
  (internal/store)`. Handlers receive a `Config` struct of service pointers, constructed in
  `forge/api/cmd/api/main.go`. There is no DI container; do not reintroduce one.
- Routes: ~103 `register*Routes(...)` calls inside `NewServer` (`internal/http/server.go`). Relevant line numbers:
  locations 2555, regions 2556, admin 2558, kubernetes 2571, incus 2572, nomad 2573, sftp 2585, node-autoscale
  2588, cloud 2593, capabilities 2625, docker 2647, host-files 2650, host 2675, cleanup 2680, crossnode 2694,
  netbird 2695. Onboarding exchange is inline at 1097-1115.
- A second, WORKING plugin mechanism exists: `RegisterPhaseRegistrar(name, priority, fn)`
  (`internal/http/phase_registry.go`), executed by `registerPhaseHooks` at `server.go:2698`. A registrar runs ONLY
  if some file calls it inside `func init()`. Live registrars in this audit: docker-events, docker-cleanup (215),
  app-mounts (210), container-files (220), phase8-onboarding (170), tags (200), resource-limits, preview-env (160),
  vault-provider, deployment-rollback, phase2 (2000). **I checked: none of these are dead code. Do not report an
  unwired registrar without proving the `init()` call site is missing.**
- Panel -> Beacon is **HTTP only** via `forge/api/internal/daemon/client.go` (112 exported methods). WebSockets
  carry console/stats/log/terminal streams only, never commands.
- Beacon talks back to the control plane over `/api/remote` with node credentials, not user sessions.
- Authz has three separate layers: admin scopes (`requireAdminScope`), per-server RBAC
  (`requireServerPermission`), and org tenancy. Session cookie is `__Host-forge_session`; mutations need the
  cookie + CSRF header pair.
- Store has postgres/mysql/sqlite drivers; Postgres is the deployed target.
- Beacon runtime adapters (docker, containerd, podman, kubernetes, firecracker, kvm, lxc) sit behind a registry in
  `beacon/internal/runtime`. **Docker is the only verified production path**; experimental hypervisor providers are
  build-tag gated. Never let a non-docker adapter be used as a silent fallback.
- Frontend: ONE HTTP primitive — `requestJSON`/`fetchJSON` in `forge/web/lib/api/http.ts`. Domain modules in
  `forge/web/lib/api/*.ts`, re-exported by `forge/web/lib/api.ts`. Bare `fetch` inside a component is a violation.
  Server state is `@tanstack/react-query`, client state `zustand`, query keys in `lib/api/query-keys.ts`. UI
  primitives are hand-rolled in `components/ui/` — there is NO Radix and NO shadcn; extend what exists. Admin nav
  is data-driven from `components/admin/admin-registry.ts` (each entry has
  `capability: "available" | "metadata-only"`). Design tokens are CSS variables in `app/globals.css` —
  `var(--token)`, never a raw hex. Fonts: Manrope (UI), JetBrains Mono (technical values).
- i18n catalogs live in `/Users/riyaz/forge-plane/forge-test/lang/` for eight locales, synced into web via
  `npm --workspace @forge/web run sync:locales`.
- Migrations: `forge/api/migrations/NNN_description.sql`, applied in filename sort order, unique numeric prefix
  required (use a letter suffix like `235_a_...` if the number is taken). **Never rename an applied migration — its
  name is a primary key in `schema_migrations`.** Latest existing is `234_target_group_targets_fks.sql`.

## Non-negotiable invariants (these are the spec, not suggestions)

1. **Never report success for work not performed.** A step that cannot do its job returns an error, not `nil`.
   Unknown is not zero. Not-reported is not zero. A stale reading is not a healthy one. This holds in the API, in
   Beacon, and in the UI.
2. **Never resolve an ambiguous target silently.** If a request omits the node/cluster/instance it refers to,
   reject it with 400. Do not iterate and pick the first node that happens to have credentials. Watch specifically
   for `SetDefaultNode` being used as a tie-breaker.
3. **Fail closed on missing configuration.** An unconfigured client or absent credential must produce an explicit
   "not configured" error, never an empty list that masquerades as "nothing exists".

User decision for this run: where an audited page or endpoint turns out to be a stub, unwired, or dishonestly
reporting success — **make it actually work**, and leave nav items marked `available` only once the path genuinely
works end to end. If you truly cannot make it work in your slice, return an explicit unavailable error rather than
fabricating data, and say so in your report.

## Hard constraints

- **Edit only files your scope file lists as yours.** Nine other agents are editing other files right now. If you
  find a defect in a file you do not own, DO NOT touch it — write it into your report as `[OUT-OF-SCOPE]` with
  `file:line`.
- Shared hotspots that several agents may legitimately need: `forge/api/internal/http/server.go`,
  `forge/api/cmd/api/main.go`, `forge/api/internal/store/store.go`, `forge/api/internal/store/database.go`,
  `forge/api/internal/daemon/client.go`, `beacon/internal/server/server.go`, `forge/web/lib/api.ts`,
  `forge/web/lib/api/query-keys.ts`, `forge/web/lib/api/http.ts`, `forge/web/components/admin/admin-registry.ts`.
  In these, make the **smallest possible surgical edit to your own lines only**. Never reformat, reorder, re-align
  or rewrite such a file. Never run a formatter over a whole shared file.
- Do NOT touch: `reference/` (a 1.9 GB vendored upstream checkout, includes netbird source), `graphify-out/` or any
  `**/graphify-out/`, `node_modules`, `.next`, `dist`, `forge/web/vitest-*.txt` (captured logs, not sources),
  `audits/`, or `.audit-reports/` except your own report file.
- Do NOT run builds or tests: no `go build ./...`, `go test`, `make`, `golangci-lint`, `tsc`, `eslint`, `vitest`,
  `next build`, `npm run build`, or the locale sync script. The tree is under concurrent edit so any output is
  meaningless, and `go vet ./...` in `forge/api` additionally reports pre-existing test-file drift that is not a
  regression. `gofmt -e <one of your files>` is allowed. Verify correctness by reading.
- Comments: default to none. One short line maximum, only when the WHY is non-obvious (a hidden constraint, a
  subtle invariant, a workaround). Never write a comment that references this audit, this scope, a task number, or
  "used by" callers.
- Do not add abstractions, helpers, feature flags, or backwards-compat shims beyond what the fix requires. Do not
  do opportunistic refactors of your slice.

## Reporting protocol (required — this is what makes an interrupted run recoverable)

Your report file: `/Users/riyaz/forge-plane/forge-test/.audit-reports/reports/fleet-N.md` (N = your scope number).

1. Create it with a `# Scope N` header and a `## Status` line BEFORE you edit anything, then **append a finding
   line immediately after you finish each source file.** Do not accumulate findings and write them at the end. If
   you are killed mid-run, the file must already show what you did.
2. Each finding is one line: `- [FIXED] file:line — what was wrong -> what you changed`, or
   `[OUT-OF-SCOPE] file:line — defect you saw but did not touch`, or
   `[DELIBERATELY-UNCHANGED] file:line — looked wrong, is actually correct, and why`.
3. Track your files as a checklist at the top and tick each off as fully read + audited.
4. Finish by updating `## Status` to `complete` (or `partial: <what remains>`) and adding a `## Cannot verify`
   section listing anything unprovable from code alone, with the reason.
5. Return to the caller a summary under 250 words pointing at the report file. Do not paste diffs.
