#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT/.dev-pids"
MODE="${1:-docker}"

yellow=$'\033[33m'
green=$'\033[32m'
reset=$'\033[0m'

stop_pid() {
  local name="$1"
  local file="$PID_DIR/${name}.pid"
  if [ -f "$file" ]; then
    local pid
    pid="$(cat "$file")"
    if kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
      printf "  %s[stop]%s %s pid %s\n" "$yellow" "$reset" "$name" "$pid"
    fi
    rm -f "$file"
  fi
}

stop_port() {
  local name="$1" port="$2"
  if ! command -v lsof >/dev/null 2>&1; then
    return 0
  fi
  local pids
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -z "$pids" ]; then
    return 0
  fi
  for pid in $pids; do
    kill "$pid" >/dev/null 2>&1 || true
    printf "  %s[stop]%s %s port %s pid %s\n" "$yellow" "$reset" "$name" "$port" "$pid"
  done
}

stop_pid frontend
stop_pid daemon
stop_pid api
stop_port frontend "${FRONTEND_PORT:-3000}"
stop_port daemon "${DAEMON_PORT:-9090}"
stop_port sftp "${DAEMON_SFTP_PORT:-2022}"
stop_port api "${API_PORT:-8080}"

if [ "$MODE" = "docker" ] && command -v docker >/dev/null 2>&1; then
  (cd "$ROOT/infra" && docker compose down >/dev/null 2>&1 || true)
  printf "  %s[stop]%s Docker Postgres/Redis stopped\n" "$yellow" "$reset"
fi

printf "%sGamePanel dev environment stopped.%s\n" "$green" "$reset"
