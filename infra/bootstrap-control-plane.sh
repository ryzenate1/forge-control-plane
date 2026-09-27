#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$script_dir"

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose v2 is required" >&2
  exit 1
fi

if [ ! -f .env ]; then
  echo "infra/.env is missing; run PANEL_DOMAIN=panel.example.com ./gen-env.sh .env" >&2
  exit 1
fi

# Restricted parser: only bare KEY=VALUE lines for an explicit allowlist.
# Never `source` .env — it is secret material that must not execute code.
while IFS= read -r line || [ -n "$line" ]; do
  line="$(printf '%s' "$line" | tr -d '\r')"
  case "$line" in ''|'#'*) continue ;; esac
  case "$line" in *"="*) ;; *) echo "Ignoring malformed line in .env: $line" >&2; continue ;; esac
  key="${line%%=*}"
  value="${line#*=}"
  case "$key" in
    ''|*[!A-Za-z0-9_]*|[0-9]*) echo "Ignoring bad key in .env: $key" >&2; continue ;;
  esac
  case "$key" in
    TAG|POSTGRES_DB|POSTGRES_USER|POSTGRES_PASSWORD|DATABASE_URL|REDIS_ADDR|REDIS_PASSWORD|PANEL_URL|PANEL_DOMAIN|API_DOMAIN|API_ADDR|API_AUTH_SECRET|APP_KEY|APP_ENV|APP_VERSION|FORGE_MASTER_KEY|FORGE_MASTER_KEY_ID|FORGE_PREVIOUS_MASTER_KEYS|FORGE_ALLOW_EPHEMERAL_MASTER_KEY|DAEMON_NODE_ID|DAEMON_NODE_TOKEN|DAEMON_ADDR|DAEMON_SFTP_BIND_ADDR|DAEMON_SFTP_HOST_KEY_PASSPHRASE|DAEMON_UPGRADE_PUBLIC_KEY|DAEMON_DATA_DIR|DAEMON_BACKUP_DIR|GAME_SERVERS_HOST_DIR|BEACON_DATABASE_PATH|PANEL_API_URL|METRICS_TOKEN|METRICS_TOKEN_FILE|GRAFANA_ADMIN_USER|GRAFANA_ADMIN_PASSWORD|BACKUP_ADAPTER|S3_BUCKET|S3_REGION|S3_ACCESS_KEY_ID|S3_SECRET_ACCESS_KEY|S3_ENDPOINT|S3_PREFIX|S3_USE_PATH_STYLE|S3_BACKUP_BUCKET|S3_BACKUP_REGION|S3_BACKUP_ACCESS_KEY_ID|S3_BACKUP_SECRET_ACCESS_KEY|S3_BACKUP_PREFIX|PUSHGATEWAY_URL|POSTGRES_BACKUP_COMPRESS|CADDY_ADMIN|CADDY_ADMIN_ADDR|CADDY_HTTP_BIND|CADDY_HTTPS_BIND|POSTGRES_BACKUP_HOST_DIR|POSTGRES_BACKUP_INTERVAL_SECONDS|POSTGRES_BACKUP_RETENTION_DAYS|ALERTMANAGER_SMTP_HOST|ALERTMANAGER_SMTP_PORT|ALERTMANAGER_SMTP_USER|ALERTMANAGER_SMTP_PASS|ALERTMANAGER_SMTP_FROM|ALERTMANAGER_ADMIN_EMAIL|ALERTMANAGER_WEBHOOK_URL|TRAEFIK_ACME_EMAIL|TRAEFIK_CERTIFICATE_RESOLVER|API_IMAGE|WEB_IMAGE|DAEMON_IMAGE|API_INTERNAL_URL|LOAD_BALANCER_ENABLED|LOAD_BALANCER_BIND_HOST|LOAD_BALANCER_PORT_MIN|LOAD_BALANCER_PORT_MAX) ;;
    *) continue ;;
  esac
  case "$value" in
    '"'*'"') value="${value#\"}"; value="${value%\"}" ;;
    "'"*"'") value="${value#\'}"; value="${value%\'}";;
  esac
  printf -v "$key" '%s' "$value"
  export "$key"
done < ./.env

mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"

# Single-shot full stack (postgres, redis, api, web, daemon, monitoring,
# backup, caddy). No phased bring-up: partial stacks hide dependency errors
# and leave /setup pointing at services that are not running yet.
compose=(docker compose -f compose.yml -f compose.production.yml --env-file .env)
"${compose[@]}" config --quiet
"${compose[@]}" up -d --build
"${compose[@]}" ps

echo "Control plane started (full profile, incl. Caddy on 80/443)."
echo "Complete /setup, create a node, then set DAEMON_NODE_ID/DAEMON_NODE_TOKEN"
echo "in .env and re-run this script to roll the new identity."
