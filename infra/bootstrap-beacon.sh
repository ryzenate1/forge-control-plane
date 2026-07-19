#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$script_dir"

if [ ! -f .env ]; then echo "infra/.env is required" >&2; exit 1; fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a
: "${DAEMON_NODE_ID:?Set DAEMON_NODE_ID to the node UUID created in the panel}"
: "${DAEMON_NODE_TOKEN:?Set DAEMON_NODE_TOKEN to the panel-issued credential}"
: "${PANEL_API_URL:?Set PANEL_API_URL to a URL reachable from this node}"

case "$PANEL_API_URL" in http://*|https://*) ;; *) echo "PANEL_API_URL must be an absolute HTTP(S) URL" >&2; exit 1;; esac
mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"
docker compose -f compose.beacon.yml --env-file .env config --quiet
docker compose -f compose.beacon.yml --env-file .env up -d --build
docker compose -f compose.beacon.yml --env-file .env ps
