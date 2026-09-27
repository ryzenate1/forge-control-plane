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

set -a
# shellcheck disable=SC1091
. ./.env
set +a

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
