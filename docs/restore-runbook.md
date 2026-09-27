# PostgreSQL Restore Runbook

How to restore the Forge control-plane database from the dumps produced by
`infra/postgres-backup.sh` (custom-format `gamepanel-*.dump[.gz]` in
`${POSTGRES_BACKUP_HOST_DIR}`, default `/var/backups/gamepanel/postgres`).

## 1. Pick a backup

```bash
ls -lt /var/backups/gamepanel/postgres | head
# choose the newest verified dump, e.g. gamepanel-20260101T000000Z.dump.gz
```

Dumps are `pg_dump --format=custom`; the `.gz` suffix is transport
compression applied *after* the dump (gunzip first, then `pg_restore`).

```bash
BACKUP=/var/backups/gamepanel/postgres/gamepanel-<ts>.dump.gz
cp "$BACKUP" /tmp/restore.dump.gz
gunzip -f /tmp/restore.dump.gz   # -> /tmp/restore.dump
chmod 600 /tmp/restore.dump
pg_restore --list /tmp/restore.dump >/dev/null && echo "archive OK"
```

## 2. Scratch-verify first (never restore straight into production)

```bash
createdb scratch_restore
pg_restore --no-owner --dbname=scratch_restore /tmp/restore.dump
# sanity-check row counts, then drop:
psql -d scratch_restore -c 'SELECT count(*) FROM schema_migrations;'
dropdb scratch_restore
```

If the scratch restore fails, stop — do not touch the live database.

## 3. Restore the target

Stop writers first (API + backup loop), then restore:

```bash
cd infra
docker compose stop api postgres-backup

# Option A: into the existing database (destructive):
pg_restore --clean --if-exists \
  --host 127.0.0.1 --username gamepanel --dbname gamepanel \
  /tmp/restore.dump

# Option B: into a fresh database, then swap:
# createdb gamepanel_restored
# pg_restore --no-owner --dbname=gamepanel_restored /tmp/restore.dump

docker compose start api postgres-backup
```

`PGPASSWORD` must be set (or `~/.pgpass`) — the password is
`POSTGRES_PASSWORD` from `infra/.env`.

## 4. Post-restore checks

- `docker compose ps` — postgres/api healthy.
- `curl -s http://localhost:8080/api/v1/health/ready`.
- `psql -c 'SELECT count(*) FROM schema_migrations;'` matches pre-restore.
- Watch Alertmanager: `PostgresBackupStale` should clear after the next
  successful backup loop iteration.

## Permissions

- Backup host dir: `0700`, dumps: `0600`, owner = backup uid (10001 in
  compose). A world-readable backup dir leaks the full database.
- Never leave `/tmp/restore.dump` behind: `shred -u /tmp/restore.dump`.
