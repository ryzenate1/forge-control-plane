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
| `forge/api/migrations` | SQL migrations (~246), applied by `Store.RunMigrations`. |
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
  `Config` struct holding every service pointer. `internal/app/container.go` is
  the DI container: `New → InitDB → InitStores → InitServices → BuildHTTP`.
- Most routes are registered by ~100 `register*Routes(...)` calls inside
  `NewServer` (`internal/http/server.go`). A smaller plugin-style hook exists:
  `RegisterPhaseRegistrar(name, priority, fn)` in
  `internal/http/phase_registry.go`, invoked from `registerPhaseHooks`.
  A registrar only runs if some file calls `RegisterPhaseRegistrar` in an
  `init()` — defining the function is not enough.
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
