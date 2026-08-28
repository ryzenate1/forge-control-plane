# RETIRED — do not add migrations here

The canonical migration stream lives in `forge/api/migrations/` and is the
only directory any runner reads (`MIGRATIONS_DIR`, default `migrations`
relative to `forge/api`). No code path executes the files in this directory;
they are retained as read-only history for archaeology (old Batch fragments,
pre-consolidation numbering).

## Rules

- NEVER rename an already-applied migration file. The filename is the primary
  key in `schema_migrations`; a rename re-applies as a brand new migration.
  Historical renames are handled through `migrationAliases` in
  `internal/store/migration.go` (alias-aware skip + backfill), never renames.
- New migrations go in `forge/api/migrations/` with a unique numeric prefix.
  Use a letter suffix (`NNN_a_...`) when extending a family, or bump to a new
  highest number. Never reuse a retired number.
- Filenames that duplicated canonical files byte-for-byte
  (`041_a_placement_intents.sql`, `114_e_zero_downtime_deploy.sql`) were
  removed from this directory to keep a single migration dir. Their content
  lives on unchanged under the same names in `forge/api/migrations/`.

## Retired numbers (never reuse)

- `029`, `030`, `031`: retired auth fragments, superseded by the 027-028
  series (copies remain in this directory as history only).
- `061`-`076`: never shipped; the gap between `060` and `077` is intentional.
- `166`-`169`: retired phase fragments, superseded by the 165/170-172 series.
- Enforced by `retiredMigrationPrefixes` in `internal/store/migration.go`:
  a new file reusing a retired number fails validation.
