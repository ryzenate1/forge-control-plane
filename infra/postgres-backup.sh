#!/bin/sh
set -eu

interval="${POSTGRES_BACKUP_INTERVAL_SECONDS:-86400}"
retention="${POSTGRES_BACKUP_RETENTION_DAYS:-14}"

case "$interval" in *[!0-9]*|'') echo "invalid backup interval" >&2; exit 1;; esac
case "$retention" in *[!0-9]*|'') echo "invalid backup retention" >&2; exit 1;; esac

mkdir -p /backups
while true; do
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  partial="/backups/gamepanel-${timestamp}.dump.partial"
  final="/backups/gamepanel-${timestamp}.dump"
  if pg_dump --format=custom --file="$partial"; then
    mv "$partial" "$final"
    find /backups -maxdepth 1 -type f -name 'gamepanel-*.dump' -mtime "+$retention" -delete
  else
    echo "PostgreSQL backup failed at $timestamp" >&2
  fi
  sleep "$interval"
done
