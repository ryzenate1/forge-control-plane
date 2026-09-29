# Database Layer Audit — 2026-09-29

**Scope:** Static audit of the Forge control-plane database layer (`forge/api/internal/store`,
`forge/api/migrations`, connection/config wiring, CI coverage).
**Branch:** `mvp-4` (clean tree, `bce7085`)
**Method:** Static analysis only. No live database was inspected and no code was changed.
**Status:** All eight findings addressed — see [Resolution](#resolution) at the end, which also
records what the fixes were and were not verified against.

## Verification limits

Two things could not be verified in this environment. Both are environmental, not code defects,
and both bound the confidence of the findings below.

1. **The Go store tests could not be run.** `github.com/lib/pq` and `github.com/stretchr/testify`
   have only `.mod` metadata in the local module cache — no source. Downloading is blocked:
   the sandbox denies writes to `/Users/muni/go/pkg/mod` and denies outbound network to
   `proxy.golang.org` (`deny network-outbound proxy.golang.org:443`). An overlay `GOMODCACHE`
   using the local cache as a file proxy was attempted and denied for the two missing modules.
   `go build ./internal/store/` does pass with a redirected `GOCACHE`.
2. **No live schema was inspected.** The dev stack is down — nothing listening on 5432, 6379,
   8080, 9090 or 3000. The live schema was therefore not diffed against the migration set.

Every finding below is derived from source, migration files, CI config and infra templates.

## Summary

The migration *engine* is in good shape and is the strongest part of this layer: correct
ordering, deliberate rename/duplicate handling, cross-instance advisory locking, per-file
transactions, and a consistent refusal to let a skipped step look like success. No TODO,
FIXME or BUG markers exist anywhere in the store or the `dbprovisioner`/`dbbackup` services.

The problems are not in the engine. They are in **what verifies it** and in **what surrounds
it**: the production migration path is never exercised in CI, the CI guards the documentation
promises do not exist, operator-facing pool tuning is silently inert, and three parallel
database abstractions are dead but look authoritative.

| # | Finding | Severity |
| --- | --- | --- |
| 1 | Production migration path never exercised in CI | High |
| 2 | Documented CI guards do not exist | High |
| 3 | Connection-pool env vars are inert; pool hardcoded to 8 | Medium |
| 4 | No drift detection for already-applied migrations | Medium |
| 5 | Three parallel dead database abstractions | Medium |
| 6 | 20 MB compiled test binary committed to git | Medium |
| 7 | MySQL DSN does not encode credentials; defaults to no TLS | Low |
| 8 | Rollback policy doc counts are stale | Low |

---

## 1. Production migration path is never exercised in CI — High

There are **two** migration runners:

- `Store.runMigrations` (`forge/api/internal/store/store.go:1202`, entered via
  `RunMigrations:1181`) — pgx/`pgxpool`, Postgres only. **This is the production path**, called
  from `internal/app/container.go:134` during `InitDB`.
- `MigrationRunner` (`forge/api/internal/store/migration.go:33`) — the `DatabaseDriver`
  abstraction, with SQLite/MySQL translation and rollback support.

`NewMigrationRunner` is referenced **only from `_test.go` files**. It is not on the production
path at all.

The consequence in CI:

- `ci.yml` runs `go test -race -timeout 10m -count=1 ./...` with **no Postgres service container**.
- `migration_comprehensive_test.go` — the fresh-install, Batch 1→Batch 2 upgrade and FK
  validation suite — calls `t.Skipf("Skipping PostgreSQL test: %v", err)` at lines **230** and
  **236** when Postgres is unreachable. In CI it therefore skips, and the job reports green.
- The migration tests that *do* run (`migration_runner_unified_test.go:42` and peers) call
  `createDisposableDatabase(t, DatabaseSQLite)` and drive `MigrationRunner` — the test-only
  runner, on SQLite.

Net effect: **the 243 production migrations are never applied against Postgres in CI, and
`Store.runMigrations` is never executed by any test that runs there.** The code that ships is
verified by tests that skip; the code that is verified does not ship.

Note the irony against the repo's own rule in `AGENTS.md` — "Never report success for work not
performed." A silent `t.Skip` on a green job is exactly that, one level up.

**Recommendation:** add a `postgres:16` service container to the `forge-api` job (or a dedicated
`migrations` job) and set the env var the comprehensive test reads. Convert the reachability
`t.Skipf` to a hard failure whenever a CI marker (e.g. `CI=true`) is set, so a missing database
breaks the build instead of quietly passing.

## 2. Documented CI guards do not exist — High

`docs/migration-rollback-policy.md` ("CI Guards" section) makes four specific claims. None hold:

| Documented claim | Actual state |
| --- | --- |
| `scripts/test/validate_migrations.sh` "wired in `.github/workflows/ci.yml:migrations`" | Script exists (11 KB, mode 755) but is **never invoked** — zero references in `.github/`. No `migrations` job exists. |
| `ci.yml:migrations` does "static validation + `psql -f` fresh apply" | No such job. `ci.yml` has exactly four: `forge-api`, `beacon`, `packages`, `web`. |
| Integration tests "wired in `.github/workflows/ci.yml:forge-api-integration`" | No such job. |
| Tests run via `go test -tags integration` | **Zero files** in the repo use `//go:build integration`. No script, Makefile target or workflow passes `-tags integration`. |

So the duplicate-prefix, ordering and FK checks that the policy document presents as enforced
are, in practice, only enforced at API startup by `validateNoDuplicatePrefixes`
(`migration.go:1279`) — after a bad migration has already shipped.

**Recommendation:** either wire `validate_migrations.sh` into CI, or correct the document. The
current state is worse than either, because it tells a reviewer the guard is covered.

## 3. Connection-pool env vars are inert; pool hardcoded to 8 — Medium

`ConnectWithKeyring` (`store.go:1135`) hardcodes the pool:

```go
cfg.MaxConns = 8
cfg.MinConns = 1
cfg.MaxConnLifetime = time.Hour
```

Meanwhile `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS` and `DB_CONN_MAX_LIFETIME` are:

- **written into generated production `.env` files** — `infra/gen-env.sh:299-301`,
  `infra/gen-env.ps1:112-114`
- **documented to operators** — `infra/.env.example:62-64`
- **read in four code locations** — `cmd/api/main.go:1533`, `forge/api/config/database.go:41,55,63`,
  `internal/config/env.go:34`, plus defaults in `internal/store/store_pool.go:19`
- **never applied to the production pool.**

An operator who raises `DB_MAX_OPEN_CONNS=100` to address saturation gets no change and no
warning. The effective ceiling stays 8 connections for the whole control plane — shared across
~100 route groups and the background workers visible in the dev log (gitops controller,
catalog retention worker, heartbeat monitor, node-metrics collector, service discovery,
ingress sync). Eight is low for that concurrency and is not adjustable.

`internal/config/env.go:36` also stores `ConnMaxLifetime` as a bare `int` with no unit
conversion, unlike `config/database.go:42` which correctly multiplies by `time.Second`. Moot
while neither value reaches a pool — the only consumers of these fields are the dead driver
constructors (`driver_postgres.go:23-26`, `driver_mysql.go:24-27`) and `store_pool.go:30-33` —
but it is a live trap if either path is revived.

**Recommendation:** thread the configured values into `ConnectWithKeyring`, or remove the env
vars from `gen-env.*` and `.env.example`. Do not leave a documented knob wired to nothing.

## 4. No drift detection for already-applied migrations — Medium

`schema_migrations` is created as `(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ)` —
`store.go:1211-1212`, again at `1323-1324`, and `migration.go:1400-1401`. There is **no checksum
column**.

Neither runner records or compares a hash of applied content, so editing a migration file that
has already been applied is silently ignored forever: the `version` row exists, the file is
skipped, and the divergence between file and deployed schema is never surfaced.

`validateMigrationHashes` (`migration.go:1342`) is easy to mistake for this protection. It is
not — it hashes files against *each other* to catch unregistered byte-identical copies, not
against what was applied. It is also only called from `MigrationRunner.Run` (`migration.go:161`),
so it does not run in production at all; `Store.runMigrations` omits it. That asymmetry is a
second, smaller gap: an unregistered duplicate migration is caught only by the test-only runner.

**Recommendation:** add a `checksum TEXT` column (nullable, backfilled as NULL for existing
rows) and warn loudly on mismatch. Given that migration filenames are immutable primary keys
by design, drift in content is the one integrity failure the current design cannot see.

## 5. Three parallel dead database abstractions — Medium

Production connects via `store.ConnectWithKeyring(ctx, DATABASE_URL, keyring)`
(`container.go:112-128`) — raw `DATABASE_URL` into `pgxpool.ParseConfig`. Everything else in the
layer is unreachable from production:

| Component | Status |
| --- | --- |
| `config/database.go` — `DatabaseConfig()`, `Database`, `DatabaseConnection` | **Zero callers outside the file itself.** A Laravel-style scaffold (`gamepanel` naming, `Charset`, `Collation`, `Prefix`) ported from a PHP panel. Note the enclosing `gamepanel/forge/config` package *is* live — `cmd/api/main.go:25` and `internal/app/container.go:36` import it, but only for `MTLSConfig()`. The dead unit is this file, not the package. |
| `internal/store/store_pool.go` — `DefaultPoolConfig`, `ConfigurePool`, `GetPoolStats`, `HealthCheck` | Referenced only by `store_pool_test.go`. Pool config actually happens inline in each driver constructor. |
| `DBConfig`, `DBConfig.DSN()`, `NewDatabaseDriver`, `driver_postgres.go`, `driver_mysql.go`, `driver_sqlite.go` | Referenced only from `_test.go`. |
| `DBConfig.RedactedDSN()` (`database.go:107`) | **Zero references anywhere**, tests included. A credential-redaction helper that protects no log site. |

This is the root cause of findings 3 and 7: there are two pool implementations and two migration
runners, and in both cases the well-documented, well-tested one is the one that does not ship.
`store_pool.go` is actively misleading — it has passing tests, which reads as coverage of
production pool behaviour that it does not touch.

**Recommendation:** delete `config/database.go`, `store_pool.go` and `RedactedDSN` — but not the
`gamepanel/forge/config` package around them, which is live for mTLS. Mark the `DatabaseDriver`
abstraction and `MigrationRunner` explicitly as test scaffolding rather than deleting them:
finding 1 makes them the basis of the coverage the production path is missing. That intent also
belongs in a comment on `ConnectWithKeyring`, which currently bypasses the abstraction silently.

## 6. 20 MB compiled test binary committed to git — Medium

`forge/api/internal/store/store.test` is a tracked, executable Mach-O test binary
(20,076 KB, mode 100755, blob `2b62795`). It is the **largest tracked file in the repository by
roughly 19x** — the next largest is a 1,040 KB JSON asset.

`.gitignore` covers `*.exe`, `*.bin`, `*.log`, `*.rdb`, `*.tsbuildinfo` but has **no `*.test`
pattern**, so `go test -c` output in any package can be committed the same way. This is the only
such artifact currently tracked.

**Recommendation:** `git rm --cached forge/api/internal/store/store.test` and add `*.test` to
`.gitignore`. History rewriting is not warranted for a single blob, but the ignore rule should
land so it cannot recur.

## 7. MySQL DSN does not encode credentials; defaults to no TLS — Low

In `DBConfig.DSN()` (`internal/store/database.go`) the Postgres branch is careful, and says so:

```go
// Credentials are percent-encoded so special characters (@, :, /, ?,
// #) in usernames or passwords cannot corrupt the URL parse.
dsnURL := &url.URL{ ..., User: url.UserPassword(c.User, c.Password), ... }
```

The MySQL branch, immediately below, does raw string interpolation:

```go
return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s&parseTime=true",
    c.User, c.Password, c.Host, c.Port, c.Database, tls)
```

A password containing `@`, `:` or `/` corrupts the DSN parse, and because the host is positional
in MySQL DSN syntax, a crafted password could shift the parsed host. Separately, `SSLMode`
defaults to **`tls=false`** here, with none of the `APP_ENV`-based hardening the Postgres branch
applies (`disable` in development, `require` otherwise).

Severity is Low **only** because this code is unreachable in production (finding 5). It is a
real defect the moment anyone revives the driver abstraction, and it sits two lines below a
comment explaining precisely why not to do it.

## 8. Rollback policy doc counts are stale — Low

`docs/migration-rollback-policy.md` states "Ups: 197 migrations", "Downs: 14 rollbacks",
"Coverage: ~7%", and "The 14 existing down files are retained". Actual on disk:

| Metric | Documented | Actual |
| --- | --- | --- |
| Root migrations | 197 | **243** |
| Rollback files | 14 | **9** |
| Coverage | ~7% | **~3.7%** |

Rollback files have *decreased* since the document was written, so the drift is not simply a
stale high-water mark — five were removed without the policy being updated.

The forward-only decision itself is sound, well-argued and correctly implemented; only the
numbers are wrong.

---

## Verified healthy

Reported explicitly so a future reader does not re-litigate these:

- **Ordering is correct, and subtler than the docs suggest.** `sortMigrationFiles` implements
  numeric-then-suffix order so bare `082` applies before `082_a` — the base table exists before
  its suffixed companion alters it. Plain lexicographic sort would get this wrong
  (`018_a_…` sorts before `018_api_…` because `_` < `p`). Note `AGENTS.md` describes migrations
  as applied "in filename sort order", which is inaccurate — a custom comparator exists
  precisely because filename order is wrong.
- **Rename/duplicate handling is deliberate, not accidental.** 14 no-op guard files carrying
  `SELECT 1;`, paired in `migrationAliases` (`migration.go:1132`). Backfill is correctly
  **directional**: a canonical that applied backfills its guard row, but a guard row never
  implies the canonical ran, because guards are no-ops on fresh hosts. The grandfathered
  duplicate-prefix list (015, 018, 020, 044, 054, 057, 080, 082, 083, 087) is enforced by
  `validateNoDuplicatePrefixes` with the reasoning documented inline. The apparent duplicates
  found at `018_*` and `020_*` are this mechanism working, not a defect.
- **Concurrency is handled correctly.** `pg_advisory_lock` (`0x466F7267656D6967`) serializes
  migrations across horizontally scaled API instances, and in both runners the lock is pinned to
  one dedicated `*sql.Conn`/connection — with an inline explanation of why a pooled `Exec` would
  make the lock silently ineffective (lock on session A, DDL on session B). SQLite uses an
  in-process mutex; MySQL uses `GET_LOCK`.
- **Failure handling refuses to fake success.** Per-file transaction with rollback on error.
  MySQL's non-transactional DDL is documented as best-effort rather than glossed over.
  Untranslatable SQLite DDL fails loudly with the offending statement and a pointer to the
  dialect-override mechanism, instead of skipping.
- **SQLite foreign keys are enforced belt-and-braces** — `_foreign_keys=on` in the DSN for every
  pooled connection, plus a `PRAGMA foreign_keys=ON` for the opening handle.
- **`internal/store` builds clean**, and there are no TODO/FIXME/BUG markers in the store or in
  `dbprovisioner`/`dbbackup`.
- **Multi-driver coverage is thin but honestly labelled.** 10 MySQL and 25 SQLite dialect
  overrides for 243 migrations (4% / 10%). The `DatabaseMySQL` declaration in `database.go` says
  so directly: "best-effort targets… Do not claim full MySQL parity for migrations without an
  override." Not a defect for a Postgres-only deployment; recorded so the presence of three
  drivers is not mistaken for three supported databases.

## Suggested order of work

1. Add Postgres to CI and make the comprehensive migration test fail rather than skip there
   (finding 1) — this is the one change that would have caught most of the rest.
2. Reconcile `migration-rollback-policy.md` with reality, or wire the guards it claims
   (findings 2, 8).
3. Decide the pool story: thread the config through, or delete the env vars (finding 3).
4. Untrack `store.test`, add `*.test` to `.gitignore` (finding 6).
5. Delete the dead abstractions, or label them (findings 5, 7).
6. Add a `checksum` column to `schema_migrations` (finding 4).

---

## Resolution

Addressed in one commit on `mvp-4`. What changed, per finding:

| # | Change |
| --- | --- |
| 1 | New `forge-api-integration` CI job with a `postgres:16-alpine` service, and `store_migrations_postgres_test.go` — `TestProductionRunnerAgainstPostgres` drives `Store.RunMigrations` (the runner that ships) over the whole stream, asserting one `schema_migrations` row per `.sql` file, zero drift and an idempotent second run. `FORGE_TEST_REQUIRE_POSTGRES=1` turns the package's "PostgreSQL unavailable" skips into failures so the job cannot go green without doing the work; developers without a local PostgreSQL still get skips. |
| 2 | `validate_migrations.sh` is now invoked by a `migrations` CI job. Three defects in the script itself were fixed first: it failed on all ten grandfathered duplicate prefixes (wiring it in as-was would have broken every PR), `check_order` sorted the list and then asserted it was sorted (it could never fail), and `generate_summary` printed "All critical checks passed." unconditionally. The duplicate check now mirrors `allowedHistoricalDuplicates`/`knownMax` in `migration.go`, and a failing check now exits 1 and names itself. |
| 3 | `store.applyPoolEnvOverrides` honours `DB_MAX_OPEN_CONNS`, `DB_CONN_MAX_LIFETIME` and `DB_CONN_MAX_IDLE_TIME` on the pgxpool, with anything set inside `DATABASE_URL` taking precedence and `8` kept as the fallback so no deployment changes behaviour on upgrade. `DB_MAX_IDLE_CONNS` has no pgxpool equivalent — `MinConns` is a floor, not a ceiling, and mapping it would invert the meaning — so it was removed from `gen-env.sh`, `gen-env.ps1` and `.env.example` rather than approximated. The three write-only `config.DBConfig` pool fields, which made the pool look configured, were deleted along with their two assignment sites. |
| 4 | `schema_migrations` gained a `checksum` column (`ADD COLUMN IF NOT EXISTS` in the runner's own bootstrap, not a new migration file, since the table is runner-owned). `runMigrations` compares each already-applied migration's file against its recorded SHA-256 and exposes results through `Store.MigrationIntegrity()`, which `app.Container.InitDB` logs as a warning. Drift warns rather than aborts: filenames are immutable primary keys, so a legitimately edited migration is never re-run and refusing to boot would brick the upgrade without repairing the schema. Rows predating the column carry NULL and are counted `Unverified`, so "no drift among rows that could be checked" cannot be read as "no drift". `TestProductionRunnerDetectsDrift` covers it. |
| 5 | Deleted `config/database.go`, `store_pool.go`, `store_pool_test.go` and `RedactedDSN`. `DatabaseDriver` and `MigrationRunner` were kept: finding 1 makes them the basis of the multi-driver tests, so they are test scaffolding, now labelled as such. Note the `gamepanel/forge/config` *package* is live for `MTLSConfig()`; only the file was dead. |
| 6 | `store.test` untracked via `git rm --cached` (kept on disk); `*.test` added to `.gitignore`. No other tracked file matches the pattern. |
| 7 | The MySQL/MariaDB DSN is now built with `mysql.NewConfig()` + `FormatDSN()`, so credentials are escaped rather than concatenated, and `SSLMode` maps onto a real `TLSConfig` (`require`/`enable` → `true`, `skip-verify`, `disable` → `false`, unset → `preferred` outside development). |
| 8 | `migration-rollback-policy.md` corrected: Ups 197 → 243, Downs 14 → 9 (the five it listed do not exist), coverage ~7% → ~3.7%. The CI Guards section now describes the checks that exist, separates the `MigrationRunner`-driven tests from the new production-runner test, and states that no `-tags integration` build tag exists anywhere in the repository. |

### What this was verified against

- `go build ./...` and `go vet ./internal/config/ ./cmd/api/ ./internal/app/` in `forge/api`: clean.
- `gofmt -l -e` clean on every Go file touched. One pre-existing gofmt hunk in
  `container.go` (the `Container` struct's field alignment, ~lines 51–58) was left alone
  deliberately; it is not from this work.
- `validate_migrations.sh` against the real tree: exit 0, 243 migrations / 105 / 138. Against a
  scratch copy seeded with a new bare duplicate `301` and a malformed `badname.sql`: exit 1,
  both named.

**The new and changed Go test files were never compiled or executed.** The store package's test
dependencies (`lib/pq`, `testify`) cannot be fetched here for the reasons in
[Verification limits](#verification-limits), so `postgres_testenv_test.go`,
`store_migrations_postgres_test.go` and the `migration_comprehensive_test.go` changes were
checked only for parse and format. The `forge-api-integration` CI job is their first real
execution, and the checksum/drift code paths they cover have correspondingly not been run.
