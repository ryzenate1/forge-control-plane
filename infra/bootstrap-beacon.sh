#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$script_dir"

if [ ! -f .env ]; then echo "infra/.env is required" >&2; exit 1; fi
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
    DAEMON_NODE_ID|DAEMON_NODE_TOKEN|PANEL_API_URL|GAME_SERVERS_HOST_DIR|DAEMON_IMAGE|METRICS_TOKEN|DAEMON_SFTP_HOST_KEY_PASSPHRASE|DAEMON_ADDR|DAEMON_SFTP_BIND_ADDR|DAEMON_DATA_DIR|DAEMON_BACKUP_DIR|BEACON_DATABASE_PATH|DAEMON_ALLOW_MOCK_RUNTIME|BACKUP_ADAPTER|S3_BUCKET|S3_REGION|S3_ACCESS_KEY_ID|S3_SECRET_ACCESS_KEY|S3_ENDPOINT|S3_PREFIX|S3_USE_PATH_STYLE) ;;
    *) continue ;;
  esac
  case "$value" in
    '"'*'"') value="${value#\"}"; value="${value%\"}" ;;
    "'"*"'") value="${value#\'}"; value="${value%\'}";;
  esac
  printf -v "$key" '%s' "$value"
  export "$key"
done < ./.env
: "${DAEMON_NODE_ID:?Set DAEMON_NODE_ID to the node UUID created in the panel}"
: "${DAEMON_NODE_TOKEN:?Set DAEMON_NODE_TOKEN to the panel-issued credential}"
: "${PANEL_API_URL:?Set PANEL_API_URL to a URL reachable from this node}"

case "$PANEL_API_URL" in http://*|https://*) ;; *) echo "PANEL_API_URL must be an absolute HTTP(S) URL" >&2; exit 1;; esac
mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"
docker compose -f compose.beacon.yml --env-file .env config --quiet
docker compose -f compose.beacon.yml --env-file .env pull
docker compose -f compose.beacon.yml --env-file .env up -d --pull always
docker compose -f compose.beacon.yml --env-file .env ps
