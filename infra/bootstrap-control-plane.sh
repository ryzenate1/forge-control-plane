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

mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"

compose=(docker compose -f compose.yml -f compose.production.yml --env-file .env)
"${compose[@]}" config --quiet
"${compose[@]}" up -d --build postgres redis api web
"${compose[@]}" ps

echo "Control plane started. Complete /setup, create a node, replace"
echo "DAEMON_NODE_ID and DAEMON_NODE_TOKEN in .env, then run:"
echo "  ${compose[*]} up -d --build daemon prometheus alertmanager grafana"
