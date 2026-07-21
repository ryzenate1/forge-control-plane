#!/bin/sh
set -eu

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
while true; do
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  partial="/backups/gamepanel-${timestamp}.dump.partial"

  if [ "$compress" = "true" ]; then
    final="/backups/gamepanel-${timestamp}.dump.gz"
    if pg_dump --format=custom --file="$partial"; then
      gzip -f "$partial"
      mv "${partial}.gz" "$final"
    else
      echo "PostgreSQL backup failed at $timestamp" >&2
      rm -f "$partial"
      sleep "$interval"
      continue
    fi
  else
    final="/backups/gamepanel-${timestamp}.dump"
    if pg_dump --format=custom --file="$partial"; then
      mv "$partial" "$final"
    else
      echo "PostgreSQL backup failed at $timestamp" >&2
      rm -f "$partial"
      sleep "$interval"
      continue
    fi
  fi

  if [ -n "$s3_bucket" ] && [ -n "$s3_access_key" ] && [ -n "$s3_secret_key" ]; then
    s3_path="${s3_prefix}/gamepanel-${timestamp}.dump${compress:+.gz}"
    export AWS_ACCESS_KEY_ID="$s3_access_key"
    export AWS_SECRET_ACCESS_KEY="$s3_secret_key"
    export AWS_DEFAULT_REGION="${s3_region:-us-east-1}"
    if aws s3 cp "$final" "s3://${s3_bucket}/${s3_path}" --no-progress; then
      echo "S3 upload succeeded: s3://${s3_bucket}/${s3_path}"
    else
      echo "S3 upload failed at $timestamp" >&2
    fi
  fi

  find /backups -maxdepth 1 -type f \( -name 'gamepanel-*.dump' -o -name 'gamepanel-*.dump.gz' \) -mtime "+$retention" -delete
  sleep "$interval"
done
