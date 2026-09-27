#!/usr/bin/env bash
# GamePanel PostgreSQL backup loop.
#
# Permissions: /backups should be owned by the backup user (uid 10001 in
# compose) with mode 0700; dumps are written 0600. The compose mount
# ${POSTGRES_BACKUP_HOST_DIR}:/backups must exist on the host with matching
# ownership or the first write fails loudly (fail-closed, no silent skip).
#
# Concurrency: guarded by flock(1) on /tmp/gamepanel-backup.lock so a slow
# dump overlapping the next interval cannot corrupt the .partial file.
#
# Restore: see docs/restore-runbook.md (pg_restore --clean --if-exists
# against a scratch database first, then the target).
set -euo pipefail

interval="${POSTGRES_BACKUP_INTERVAL_SECONDS:-86400}"
retention="${POSTGRES_BACKUP_RETENTION_DAYS:-14}"

s3_bucket="${S3_BACKUP_BUCKET:-}"
s3_region="${S3_BACKUP_REGION:-}"
s3_access_key="${S3_BACKUP_ACCESS_KEY_ID:-}"
s3_secret_key="${S3_BACKUP_SECRET_ACCESS_KEY:-}"
s3_prefix="${S3_BACKUP_PREFIX:-postgres}"
compress="${POSTGRES_BACKUP_COMPRESS:-true}"

case "$interval" in *[!0-9]*|'') echo "invalid backup interval" >&2; exit 1;; esac
case "$retention" in *[!0-9]*|'') echo "invalid backup retention" >&2; exit 1;; esac

mkdir -p /backups
chmod 700 /backups 2>/dev/null || true

# Single-flight: exit (don't stack) if another backup loop holds the lock.
exec 9>/tmp/gamepanel-backup.lock
if ! flock -n 9; then
  echo "another backup process holds /tmp/gamepanel-backup.lock; exiting" >&2
  exit 1
fi

# Push a success timestamp for the PostgresBackupStale alert (scraped via
# pushgateway when PUSHGATEWAY_URL is set; harmless no-op otherwise).
push_backup_metric() {
  [ -n "${PUSHGATEWAY_URL:-}" ] || return 0
  ts="$(date +%s)"
  printf 'backup_last_success_seconds %s\n' "$ts" | \
    curl -fsS --max-time 10 --data-binary @- "${PUSHGATEWAY_URL}/metrics/job/postgres-backup" >/dev/null 2>&1 || \
    echo "WARN: failed to push backup metric" >&2
}

while true; do
  start_time="$(date +%s)"
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  partial="/backups/gamepanel-${timestamp}.dump.partial"

  if pg_dump --format=custom --file="$partial"; then
    chmod 600 "$partial" 2>/dev/null || true
    # Integrity verification in two stages:
    # 1. pg_restore --list must parse the archive (catches truncation).
    # 2. Scratch restore into a throwaway database proves the dump actually
    #    restores (catches catalog corruption --list cannot see).
    if ! pg_restore --list "$partial" >/dev/null 2>&1; then
      echo "Backup integrity check failed for $partial (archive unreadable)" >&2
      rm -f "$partial"
      sleep "$interval"
      continue
    fi
    scratch="verify_${timestamp//[^0-9]/}"
    scratch="v${scratch:0:12}$(date +%s)"
    if command -v createdb >/dev/null 2>&1 && command -v dropdb >/dev/null 2>&1; then
      if createdb "$scratch" >/dev/null 2>&1 && pg_restore --no-owner --dbname="$scratch" "$partial" >/dev/null 2>&1; then
        echo "Backup scratch-restore verified: $partial (db $scratch)"
        dropdb "$scratch" >/dev/null 2>&1 || true
      else
        echo "Backup scratch-restore FAILED for $partial" >&2
        dropdb "$scratch" >/dev/null 2>&1 || true
        rm -f "$partial"
        sleep "$interval"
        continue
      fi
    else
      echo "Backup archive-list verified (no createdb available for scratch restore): $partial"
    fi

    if [ "$compress" = "true" ]; then
      final="/backups/gamepanel-${timestamp}.dump.gz"
      gzip -f "$partial"
      mv "${partial}.gz" "$final"
    else
      final="/backups/gamepanel-${timestamp}.dump"
      mv "$partial" "$final"
    fi
    chmod 600 "$final" 2>/dev/null || true
    push_backup_metric
  else
    echo "PostgreSQL backup failed at $timestamp" >&2
    rm -f "$partial"
    sleep "$interval"
    continue
  fi

  if [ -n "$s3_bucket" ] && [ -n "$s3_access_key" ] && [ -n "$s3_secret_key" ]; then
    if command -v aws >/dev/null 2>&1; then
      s3_path="${s3_prefix}/gamepanel-${timestamp}.dump"
      if [ "$compress" = "true" ]; then s3_path="${s3_path}.gz"; fi
      export AWS_ACCESS_KEY_ID="$s3_access_key"
      export AWS_SECRET_ACCESS_KEY="$s3_secret_key"
      export AWS_DEFAULT_REGION="${s3_region:-us-east-1}"
      if aws s3 cp "$final" "s3://${s3_bucket}/${s3_path}" --no-progress; then
        echo "S3 upload succeeded: s3://${s3_bucket}/${s3_path}"
      else
        echo "S3 upload failed at $timestamp" >&2
      fi
    else
      echo "S3 credentials supplied but aws CLI is not installed" >&2
    fi
  fi

  find /backups -maxdepth 1 -type f \( -name 'gamepanel-*.dump' -o -name 'gamepanel-*.dump.gz' \) -mtime "+$retention" -delete

  end_time="$(date +%s)"
  elapsed=$((end_time - start_time))
  sleep_time=$((interval - elapsed))
  if [ "$sleep_time" -gt 0 ]; then
    sleep "$sleep_time"
  fi
done
