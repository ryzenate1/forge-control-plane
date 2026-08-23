# Migration Rollback Policy — Phase 10

**Status:** Forward-only is canonical. Rollbacks are best-effort and incomplete.

## Finding

- Ups: 244 migrations (`001_init.sql` … `234_target_group_targets_fks.sql`) <!-- generated: ls forge/api/migrations/*.sql | wc -l -->
- Downs: 9 rollbacks in `forge/api/migrations/rollbacks/` (only for selected batches: `104`, `127`, `138`, `139`, `140`, `170`, `171`, `172`, `212`)
- Coverage: ~3.7% — `migrate down` cannot reconstruct the full history and was never the vetted production path.

Historical rollbacks were added ad-hoc for Batch 2 and a few risky DDLs. The majority of migrations are deliberately `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` and are idempotent forward, but their down files would require destructive `DROP TABLE/COLUMN` with data loss and have not been maintained.

## Decision: Forward-only + Backup/Restore

**Migrations are forward-only.** Operators must not rely on `rollback/*.down.sql` for production downgrade.

**Canonical recovery is backup/restore** as documented in `docs/upgrading.md:6`:

1. **Pre-upgrade backup (mandatory):** `infra/postgres-backup.sh` (nightly) + manual `pg_dump --format=custom --no-owner --no-acl` captured by `UpgradeService.createBackup` (`forge/api/internal/services/upgrade/service.go:418 backupDatabase:455`). Verify `database.dump` non-empty, `chmod 600`, off-host copy to S3 (`$S3_BACKUP_BUCKET`).
2. **Upgrade:** `docker compose pull && up -d api` — API startup runs `Store.RunMigrations` (`forge/api/internal/store/store.go` `runMigrations`, with `pg_advisory_lock` `0x466F7267656D6967`) and `eventstore.Migrate` within a 10-minute context. Rerun-safe (applied rows keyed by full filename in `schema_migrations`).
3. **Verify:** `curl /api/v1/health/ready | jq .details.migrationCount` and `SELECT count(*) FROM schema_migrations` match expected number; run `scripts/test/validate_migrations.sh` (duplicate-prefix, order, FK checks).
4. **Rollback on failure:** **Restore from backup**, not `migrate down`:
   - In-app: `UpgradeService.rollbackUpgrade:488 restoreFromBackup:514` → `pg_restore --clean --if-exists --no-owner --no-acl --dbname $PGDATABASE $dump`
   - Manual: §6.2 in `docs/upgrading.md` — stop `api/daemon/web`, `docker compose cp $BACKUP_DIR/database.dump postgres:/tmp/restore.dump && pg_restore ...`, revert `TAG` in `infra/.env`, `up -d`.

Beacon's embedded SQLite migrations (`beacon/internal/server/...`) are likewise forward-only; downgrade requires restoring `beacon.db` from the same backup window.

## Why rollbacks are not extended

- **Data loss:** Many migrations add columns with backfillable data, encrypt secrets, or create new tables. Down would drop data irreversibly.
- **Orphan history:** `schema_migrations` filenames are immutable once shipped (see `store/migration.go:261 validateNoDuplicatePrefixes`). Renaming or adding down files retroactively breaks production histories.
- **Operational reality:** No production rollback has used `rollback/*.down.sql`; all documented recoveries use `pg_restore` (see `docs/upgrading.md` §6 and `infra/compose.yml` backup service).

The 9 existing down files are retained for local dev convenience (a test can `Rollback` a single batch in a disposable DB) but are not a supported production downgrade path.

## CI Guards

- **Static:** `scripts/test/validate_migrations.sh` — duplicate-identifier, filename-shape, FK, tenancy-column, index and Batch 2 entity checks; wired in `.github/workflows/ci.yml:migrations`. Static only: it reads the `.sql` files and does not connect to a database.
- **Fresh + Upgrade integration:** `forge/api/internal/store/migration_comprehensive_test.go TestComprehensiveMigrationValidation` (fresh install, Batch 1→Batch 2 upgrade with data survival), `migration_duplicate_test.go` and `migration_runner_unified_test.go` (idempotent, duplicate-prefix) — all driving `MigrationRunner`; plus `store_migrations_postgres_test.go TestProductionRunnerAgainstPostgres`, which applies the whole stream with the runner that actually ships (`Store.RunMigrations`). Wired in `.github/workflows/ci.yml:forge-api-integration`, which provides a `postgres:16-alpine` service and sets `FORGE_TEST_REQUIRE_POSTGRES=1` so an unreachable database fails the job instead of skipping.
- **Drift:** `schema_migrations.checksum` records a SHA-256 of each migration as it is applied. A later run compares the file against it and exposes any mismatch through `Store.MigrationIntegrity`. Rows applied before the column existed carry NULL and are counted as unverifiable, not as clean.

  The check runs on the live startup path: `cmd/api/main.go` `run()` calls `MigrationIntegrity` immediately after `RunMigrations`, before `eventstore.Migrate`. Each drifted migration is logged at `WARN` with its applied and on-disk checksums, followed by a summary carrying both the drift count and the unverifiable count.

  Drift is **reported, not fatal**. Refusing to boot would not repair an already-applied migration, and the operator needs a running panel to fix it from. It must not pass silently either, which is why a non-zero `Unverified` count also logs: rows written before the checksum column existed cannot be confirmed clean, and unverifiable is not the same as verified-good.

  > **History:** this check previously existed only in `internal/app/container.go`, which nothing imported, so drift went unreported in production for as long as this document claimed otherwise. That file has been deleted; the check now lives in the composition root. If you move startup wiring, keep the call — a docs promise with no live caller behind it is worse than no check at all.

- **Advisory lock:** `store.go` `acquireMigrationLock` serializes migrations across horizontally scaled API instances; `pg_advisory_xact_lock` for the setup wizard (`store_setup.go` `setupAdvisoryLockID`).

There is no `-tags integration` build tag anywhere in this repository; no file uses `//go:build integration`. The integration job is separated by workflow job and package scope, not by build tag.

## References

- Runner (production): `forge/api/internal/store/store.go` `RunMigrations` → `runMigrations`
- Runner (tests only): `forge/api/internal/store/migration.go` `MigrationRunner.Run` — `NewMigrationRunner` is referenced only from `_test.go` files and is not on the production path
- Rollback stub: `forge/api/internal/store/store.go` `Rollback` (requires down file; fails closed if missing)
- Upgrading guide: `docs/upgrading.md:245 6.Rollback`
- Backup service: `infra/postgres-backup.sh`, `infra/compose.yml:137 postgres-backup`
- Migration 210 (Phase 10): `forge/api/migrations/210_servers_created_at_index.sql` (hot-path `servers.created_at DESC` index)
