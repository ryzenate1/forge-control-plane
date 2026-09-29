# Forge MIGRATION audit — shared brief (all agents)

Repo: `/Users/riyaz/forge-plane/forge-test`. Read this fully before editing anything.
You are one of 10 agents auditing the **database migration stream** only.

## Boundary — a DIFFERENT audit is running in this same tree

Another 10-agent audit (`.audit-reports/`, not yours) owns Go application code:
`internal/services/compose|git|gitpush|deployment|previewenv|pipeline|build|apphosting|forgefile`,
`internal/http/handlers_*.go`, `internal/daemon/*`, `beacon/internal/server/*`,
`forge/web/lib/api/*`, and store files like `store_compose.go`, `store_deployments.go`,
`store_git_*.go`, `store_preview_env.go`, `store_source_deployments.go`, `store_templates.go`.

**Never edit any of those.** Uncommitted changes there are not your business and not your
damage. Focus only on your assigned migration scope.

Your reports go in `/Users/riyaz/forge-plane/forge-test/.mig-audit/reports/`. Never write
into `.audit-reports/`.

## The machinery

- `forge/api/migrations/` — the **only** directory any runner executes at startup
  (`MIGRATIONS_DIR`, default `migrations` relative to `forge/api`; call sites
  `internal/app/container.go:132`, `cmd/api/main.go:200`). 245 canonical `.sql` now,
  plus `sqlite/`, `mysql/`, `rollbacks/`.
- `forge/api/internal/store/migration.go` — `MigrationRunner.Run`: dialect override
  resolution, ordering, `validateNoDuplicatePrefixes`, `validateMigrationHashes`,
  `migrationAliases` guard/canonical skip+backfill, `sqliteCompatibleMigration` /
  `mysqlCompatibleMigration`, `sqliteSkippedDDL`, `sqliteUntranslatableDDL`,
  `splitSQLiteAlterAdd`, `stripCastsOutsideLiterals`, `fkFollowupMigrations`,
  `retiredMigrationPrefixes`.
- `forge/api/internal/store/store.go` ~1181–1400 — `Store.runMigrations`, `Store.Rollback`,
  `isNonReversibleRollback`; `store.go:67–90` — `acquireMigrationLock`.
- `forge/api/internal/store/store_sql_split.go` — `splitSQLStatements` (Forge splits
  statements itself; the driver does not).
- `forge/api/internal/store/migrations/` — **RETIRED, read-only history**, never executed.
  Do not edit; do not assume its tables exist at runtime.

## Live databases — verify, never guess

- Postgres **16.15** on `127.0.0.1:5432`, role `gamepanel`, **trust auth (no password)**.
- `mig_audit_schema` — reference DB, whole canonical stream applied, 263 tables.
  **STRICTLY READ ONLY** (several agents query it; never write, never drop).
- Make your own scratch DB, e.g. `createdb -h 127.0.0.1 -U gamepanel mig_s3`, and drop it
  when done. `gamepanel` (that database name) is the **live dev DB** — do not touch.

## Verified baseline — established twice, on the CURRENT tree (245 files)

- Applied **once** to a fresh Postgres 16: **0 failures**. So the bugs are not "does it
  run". They are semantic: idempotency, ordering, missing/incorrect constraints,
  schema-vs-code divergence, dialect coverage.
- Applied **twice**: **4 files fail**, and they are STILL unfixed in the tree:
  - `060_region_slug_normalization.sql:11` — `regions_slug_format_check` violation on re-run.
  - `090_allocation_transport.sql:14` — `allocations_node_ip_port_protocol_key already exists`
    (unguarded unique constraint).
  - `104_a_backup_system.sql:439,452,465,478,491,504,517` — 6 `updated_at` triggers created
    with no `DROP TRIGGER IF EXISTS`; `:555,568` — views `backup_system_overview`,
    `backup_statistics` created without `OR REPLACE`.
  - `114_a_mtls_certificates.sql:1` — `CREATE TYPE cert_type` unguarded (Postgres has no
    `CREATE TYPE IF NOT EXISTS`; needs a `DO $$` + `pg_type` check, the pattern already used
    in `086_add_table_constraints.sql`).
  **These four files are ORCHESTRATOR-OWNED. Do not edit them.** If you find issues in them,
  report them. Everything else in your range is yours to fix.
- Why re-apply matters: `migration.go:170-174` and `819-822` state the runner **always**
  executes an unrecorded canonical and never backfills one from a guard row, on the stated
  basis that "every canonical is idempotent". A non-idempotent canonical breaks that
  contract — it is a real bug, not a style preference.
- Test blind spot: `TestComprehensiveMigrationValidation`'s Postgres subtests **skip**
  (`pq: role "postgres" does not exist`); only SQLite really runs. Do not trust that suite
  as evidence about the deployed dialect.

## Rules enforced by the code

- Apply order: numeric then letter suffix — bare `082` before `082_a` (`sortMigrationFiles`).
- `validateNoDuplicatePrefixes`: bare prefixes may not repeat except the grandfathered
  `{015:2, 018:2, 020:2, 044:2, 054:2, 057:3, 080:2, 082:3, 083:2, 087:2}`.
- Retired numbers, never reuse: `029`–`031`, `061`–`076`, `166`–`169`.
- `migrationAliases` (guard → canonical): `015_mounts`→`015_a_mounts`;
  `015_db_hosts_constraints`→`120_a`; `018_api_key_scopes`→`121`; `018_ssh_2fa_activity`→`018_a`;
  `020_node_expansion`→`122`; `020_regions_multi_node_foundation`→`020_a`;
  `044_cloud_node_links`→`044_a`; `054_activity_events`→`124`; `054_social_auth`→`054_a`;
  `057_job_queue`→`057_a`; `057_webauthn`→`057_b`; `057_backup_policies`→`119_z`;
  `080_recovery_execution_statuses`→`080_a`; `087_parity_schema`→`087_a`.
  A **guard file must be a pure no-op** (comments / `SELECT 1`). A guard still carrying DDL
  is Critical (double application).
- `fkFollowupMigrations` (`migration.go:851-864`) records tables first created **without FKs**
  and the later file that adds them; `target_group_targets` maps to `""` as a tracked gap —
  but `234_target_group_targets_fks.sql` now exists and may close it.

## Hard constraints

1. **NEVER rename, move, or delete an existing migration file.** The filename is the PRIMARY
   KEY of `schema_migrations`; a rename re-applies it as brand-new on every host. Marked
   CRITICAL in `migration.go`.
2. **NEVER run `git add`, `git stash`, `git checkout`, `git reset`, `git clean`, or any
   destructive git command.** Shared tree with a concurrent human and another 10-agent audit;
   their uncommitted work is in there. Read `git status`/`git diff`; edit only your own files.
3. Editing an **already-applied** file changes only fresh installs — existing hosts won't
   re-run it. Fix content anyway (fresh-install correctness is the contract), but if a fix must
   also repair data/state on hosts that already applied it, **do not create a file**; put the
   exact SQL in your report under `NEEDS NEW MIGRATION`. Numbers are allocated by the
   orchestrator. **`234` and `235` are taken; next free prefix is `236`.**
   (Exception: the parity agent, which is explicitly assigned `236+`.)
4. Only edit inside your assigned scope. Do not touch Go code unless your scope says so.

## Restart protocol — why this is the second dispatch

Five agents previously died ~40 min in (60–120 tool calls each) after writing real edits to
disk; their reports were lost and `/tmp` was purged. Therefore:

1. **Assess before acting.** `git status`/`git diff` on your own scope, and
   `find <dir> -name '*.sql' -mmin -600`, to see what a half-finished pass already changed.
   **Continue from that state — do not redo or revert it.** Verify partial work rather than
   trusting it; a half-applied edit is the likeliest cause of a broken stream. Make your files
   internally consistent first.
2. **Write your report incrementally to disk**, starting it early (before deep reading), and
   append findings as you go. If you die, the file IS the deliverable. End with
   `STATUS: complete` or `STATUS: partial — remaining: <list>`.
3. **Budget yourself.** Make your first verified fix within ~20 tool calls. A few
   high-severity verified findings beat an exhaustive unread sweep. This project forbids
   reporting success for work not performed — say what you did not verify.

## Per-file checklist (apply to every file in your range)

a. **Idempotency**: `CREATE TABLE`/`INDEX` without `IF NOT EXISTS`; `CREATE VIEW` without
   `OR REPLACE`; `CREATE TYPE`/`DOMAIN`/`ENUM` without a guard; `ADD CONSTRAINT` without an
   existence check; `CREATE TRIGGER` without `DROP TRIGGER IF EXISTS`; seed `INSERT` without
   `ON CONFLICT DO NOTHING`; `ADD COLUMN` without `IF NOT EXISTS`; `CREATE FUNCTION`/`POLICY`
   without guards. **Prove it by applying your file twice.**
b. **Ordering**: FK/ALTER/index on a table or column not yet created at that point in the
   real apply order.
c. **Referential integrity**: FK type match (`uuid` vs `text` vs `int`); missing `ON DELETE`;
   FK that would fail on populated data (orphans); missing FK-supporting index; cascade cycles.
d. **Constraint truthfulness**: `CHECK`/enum sets vs the literals Go actually writes;
   `NOT NULL` without `DEFAULT` on a column Go omits in `INSERT`; natural keys that should be
   tenant-scoped but are globally unique (and vice versa).
e. **Types/defaults**: `timestamptz` vs `timestamp`; `uuid` vs `text` PKs; `jsonb` vs `text`;
   money/usage numeric precision; `updated_at` maintained by trigger vs `DEFAULT now()`
   (DEFAULT fires only on INSERT).
f. **Data loss**: `ALTER COLUMN TYPE ... USING` that truncates or fails; `DROP COLUMN`/`TABLE`
   still referenced by Go reads.
g. **Statement splitting**: verify dollar-quoted bodies (`$$…$$`), semicolons inside string
   literals, and `--` comments containing apostrophes survive `splitSQLStatements` — not just
   `psql`. Real historical failure class here.
h. **Cross-dialect**: would the sqlite/mysql translators mangle it, or does it trip
   `sqliteUntranslatableDDL` (⇒ override REQUIRED)? Report to the dialect agents; don't edit
   their dirs.
i. **Security**: secret/credential columns plaintext where the `encrypt_*` family (147,
   155–160, 213) protects equivalents; missing tenant scoping; seeds with real-looking secrets.
   Project history: a `git_credentials` **AAD mismatch bricked API startup** — a migration that
   re-encrypts with different associated data than the runtime decrypts with. Also check for
   **double-encryption on re-apply** (silent corruption, not a clean failure).
j. **Honesty of state columns** (AGENTS.md): *"Unknown is not zero, not-reported is not zero,
   and a stale reading is not a healthy one."* A status/health/metric column defaulting to
   healthy or `0` is **Critical**.
k. **Performance**: missing indexes on hot lookup columns; duplicate/redundant indexes;
   unbounded `ADD UNIQUE`/`CREATE INDEX` on tables that grow large.
l. **Hygiene**: wrong table name pasted from a sibling block; duplicated statements; header
   comments contradicting the DDL; dead DDL no code uses; missing trailing newline.

## Output format (strict — the orchestrator integrates it)

Write to `.mig-audit/reports/scope-<N>.md`:

1. `SCOPE` — what you owned.
2. `PER-FILE LEDGER` — one line per file, no file skipped or lumped:
   `NNN_name.sql: CLEAN` or `NNN_name.sql: <issue> -> <FIXED | REPORTED>`.
3. `FIXES APPLIED` — per file, severity (Critical/High/Medium/Low) + one-line reason.
4. `NEEDS NEW MIGRATION` — exact SQL + why already-deployed hosts need it (or "none").
5. `NEEDS OTHER AGENTS' FILES` — issues in files you do not own, with filename:line and the fix.
6. `VERIFICATION` — exact commands and real output (fresh apply + double apply on your scratch
   DB). Never claim a fix without evidence. State plainly what you could not verify.
7. `STATUS:` complete / partial + remaining list.
