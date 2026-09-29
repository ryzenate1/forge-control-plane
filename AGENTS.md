# Forge — agent working notes

Forge is a control plane for deploying and operating game servers and app
workloads across distributed infrastructure. Forge decides what should happen;
**Beacon**, the per-host agent, makes it happen on a machine.

## Layout

| Path | What it is |
| --- | --- |
| `forge/api` | Go control-plane API (Fiber v2). The authority for state, auth and orchestration. |
| `beacon` | Go per-host agent. Runs workloads, reports health, exposes host inspection, SFTP, console. |
| `forge/web` | Next.js 15 App Router dashboard (`@forge/web`). The product UI. |
| `packages/*` | npm workspace packages: `shared-types`, `sdk`, `ui`, `game-templates`. |
| `forge/api/migrations` | SQL migrations (244), applied by `Store.RunMigrations`. |
| `forge/install` | `install.sh` / `uninstall.sh` / `install-dependencies.sh` for production hosts. |
| `lang/` | Translation catalogs for eight locales; synced into web via `npm --workspace @forge/web run sync:locales`. |
| `infra/` | Compose stacks, Caddy/Nginx, Prometheus/Grafana/Alertmanager, bootstrap and backup scripts. |
| `docs/` | Architecture, operations and development docs. |
| `audits/` | Historical phase audits and findings indexes. Read-only history, not specs. |
| `scripts/` | dev, deploy, release, diagnostics and cleanup helpers. |

`go.work` spans `./beacon` and `./forge/api` (Go 1.26). npm workspaces span
`forge/web` and `packages/*`.

## Commands

```bash
# Everything
make build            # go build both modules + next build
make test             # go test -race both modules + vitest
make lint             # scripts/dev/lint.sh
make format           # scripts/dev/format.sh
make api-test         # Go tests, forge/api only
make beacon-test      # Go tests, beacon only
make web-test         # vitest only

# Go, per module
cd forge/api && go build ./... && go vet ./...
cd beacon    && go build ./... && go vet ./...
go test -race -run TestName ./internal/services/deployment   # single test
golangci-lint run     # config in .golangci.yml

# Frontend, from forge/web
npx tsc --noEmit                    # typecheck
npx eslint .                        # lint
npx vitest run                      # unit tests
npx vitest run path/to/file.test.ts # single test file
npx vitest run -t "test name"       # single test by name
npx playwright test                 # e2e (spins up its own mock API on :8080)

# Root npm scripts
npm run build:packages  # must run before web typecheck/build after package changes
```

## Running a dev stack

`./native.sh start|stop|restart|status|logs` is the macOS runner: Postgres and
Redis as Homebrew services, Beacon as a launchd user agent, API and web as
background processes. Logs land in `.dev-logs/`, pids in `.dev-pids/`, state in
`.dev-data/`. `scripts/dev/start-dev.sh` is the container-based equivalent
(`npm run dev:start|dev:stop|dev:status|dev:logs`).

Docker on this machine is Colima (`~/.colima/default/docker.sock`). If Colima is
stopped, Beacon runs but cannot start containers — API and UI still work against
seeded Postgres data, so HTTP and UI paths remain verifiable while container
lifecycle does not.

Ports: API `8080` (`/api/v1`, docs at `/api/docs`), web `3000` (`/setup` on
first run), Beacon `9090` (`/health`), Beacon SFTP `2022`, Postgres `5432`,
Redis `6379`.

## Conventions that are actually in use

**Backend**

- Layering is `handlers (internal/http/handlers_*.go) → services
  (internal/services/<pkg>) → store (internal/store)`. Handlers receive a
  `Config` struct holding every service pointer (`internal/http/server.go`,
  123 exported fields).
- **There is no DI container, by decision.** The canonical architecture is an
  explicit composition root: `cmd/api/run()` (`cmd/api/main.go`) constructs the
  entire dependency graph and injects it into the HTTP layer as a single
  `http.Config` value. `internal/app/container.go` used to be described here as
  "the DI container" — it never was. Nothing imported
  `gamepanel/forge/internal/app`, and its `InitServices`/`BuildHTTP` were
  `return nil` no-ops. It has been **deleted**, and its one genuinely useful
  behaviour (the migration-drift check) was ported into `run()`. Do not
  reintroduce a parallel container: add wiring to the composition root, or to a
  focused constructor a service package owns.

  It was also a stale, divergent fork of `run()` — its `InitDB` built a
  **plaintext** Redis client *after* passing the production `REDIS_TLS` guard,
  and its `demoSeedEnabled` blocked only the literal string `"production"`
  where `run()` uses a development/local/test allowlist. Worth remembering as
  the failure mode of speculative scaffolding kept "for later".

  Known debt, not yet addressed: `run()` is ~1,600 lines and `http.Config` has
  123 exported fields, so the composition root is honest but not yet readable.
  The intended direction is to decompose `run()` into focused init functions
  and to narrow `Config` toward per-domain dependency groups. Do this
  incrementally, with a compiler and the test suite available.
- Most routes are registered by ~100 `register*Routes(...)` calls inside
  `NewServer` (`internal/http/server.go`). A smaller plugin-style hook
  exists: `RegisterPhaseRegistrar(name, priority, fn)` in
  `internal/http/phase_registry.go`, invoked from `registerPhaseHooks`.
  A registrar only runs if some file calls `RegisterPhaseRegistrar` in an
  `init()` — defining the function is not enough. There are **16** such
  registrars (not the 8 some older comments claim).
- **Phase registration fails closed.** A registrar has exactly three
  outcomes, and "not mounted" can never look like "mounted":

  | Return | Meaning | Effect |
  | --- | --- | --- |
  | `nil` | every route mounted | recorded `Mounted` |
  | `fmt.Errorf("%w: reason", ErrPhaseSkipped)` | optional dependency absent, deliberately not mounted | logged `WARN`, recorded `Skipped`, startup continues |
  | any other error | could not mount | logged `ERROR`, aggregated, **startup panics** |

  Returning `nil` without mounting the routes is a bug — use `ErrPhaseSkipped`.
  `RegisterPhaseRegistrar` panics on a nil registrar, a duplicate name, or a
  duplicate priority (equal priorities would let mount order depend on the
  order the compiler runs `init()` in). `registerPhaseHooks` runs every
  registrar even after one fails, then returns the joined error; logging falls
  back to `slog.Default()` so a nil `cfg.Logger` cannot hide a failure.
  `PhaseRegistrationReport()` exposes the per-phase outcome for assertions.
  Tests: `internal/http/phase_registry_test.go`.
- Auth is `authMiddleware` in `internal/http/auth.go`; session cookie is
  `__Host-forge_session`. Authorization has three layers: admin scopes
  (`requireAdminScope`), per-server RBAC (`requireServerPermission`), and org
  tenancy. Beacon talks back over `/api/remote` with node credentials, not user
  sessions.
- Panel → Beacon is **HTTP** via `internal/daemon.Client`. WebSockets carry
  console/stats/log streams only, never commands.
- Placement/scheduling lives in `internal/placement`, `internal/scheduler` and
  `internal/runtime` (strategy, scoring, constraints, `explain.go`). Workload
  runtime adapters (docker, containerd, podman, kubernetes, firecracker, kvm,
  lxc) are in `beacon/internal/runtime` behind a registry; Docker is the only
  verified production path.
- Store has multiple drivers (`driver_postgres.go`, `driver_mysql.go`,
  `driver_sqlite.go`); Postgres is the deployed target.

**Frontend**

- One HTTP primitive: `requestJSON` / `fetchJSON` in `lib/api/http.ts`. Domain
  modules live in `lib/api/*` and are re-exported through `lib/api.ts`. Do not
  add a second client.
- Server state is `@tanstack/react-query`; client state is `zustand` (`stores/`).
  Bare `fetch` belongs in `lib/api/*`, not in components.
- UI primitives are hand-rolled in `components/ui/`. There is **no** Radix or
  shadcn dependency — extend what exists rather than introducing a second
  primitive set.
- Admin navigation is data-driven from `components/admin/admin-registry.ts`.
  Add a page there, not by hand-editing the sidebar.
- Fonts are Manrope (UI) and JetBrains Mono (technical values), self-hosted via
  `next/font` in `app/fonts.ts`.
- Design tokens are CSS variables in `app/globals.css` (documented in
  `forge/web/DESIGN_TOKENS.md`). Use `var(--token)`, never a raw hex.

## Pitfalls

- **Migrations**: numbered `NNN_description.sql`, applied in filename sort
  order. New files need a unique prefix — use a letter suffix (`221_a_...`) if
  the number is taken. Historical duplicate bare prefixes (`015 018 020 044 054
  057 080 082 083 087`) are grandfathered in `validateNoDuplicatePrefixes`
  (`internal/store/migration.go`); never rename an already-applied migration,
  its name is a primary key in `schema_migrations`.
- **`go vet ./...` in `forge/api` reports failures in test files** that
  reference symbols production code no longer exports. Production packages
  build clean. Check whether a failure is test drift before assuming a
  regression.
- **Never report success for work not performed.** A step that cannot do its
  job must return an error, not `nil`. Unknown is not zero, not-reported is not
  zero, and a stale reading is not a healthy one — this holds in the API, in
  Beacon and in the UI.
- **Never resolve an ambiguous target silently.** If a request omits the node it
  refers to, reject it; do not pick the first one that happens to have
  credentials.
- `forge/web/vitest-*.txt` are large captured test logs, not sources. Don't
  grep them for code.

@RTK.md
