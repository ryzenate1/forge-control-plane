#!/usr/bin/env bash
# Canonical dev stopper (scripts/start-dev.sh delegates here).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PID_DIR="$ROOT/.dev-pids"
MODE="${1:-docker}"

# shellcheck disable=SC1091
[ -f "$ROOT/scripts/dev/ports.env" ] && . "$ROOT/scripts/dev/ports.env"
# Recorded runtime ports win over defaults when present.
[ -f "$ROOT/.dev-data/ports.env" ] && . "$ROOT/.dev-data/ports.env"

yellow=$'\033[33m'
green=$'\033[32m'
reset=$'\033[0m'

# Stop only the PID recorded in $PID_DIR (i.e. a process WE started), and take
# down its whole process group like native.sh (kill -TERM -pgid) so `npm` /
# `go run` children die with their parent instead of orphaning.
stop_pid() {
  local name="$1"
  local file="$PID_DIR/${name}.pid"
  if [ -f "$file" ]; then
    local pid pgid
    pid="$(cat "$file")"
    if kill -0 "$pid" >/dev/null 2>&1; then
      pgid="$(ps -o pgid= "$pid" 2>/dev/null | tr -d ' ')"
      if [ -n "$pgid" ]; then
        kill -TERM -"$pgid" >/dev/null 2>&1 || kill -TERM "$pid" >/dev/null 2>&1 || true
      else
        kill "$pid" >/dev/null 2>&1 || true
      fi
      printf "  %s[stop]%s %s pid %s\n" "$yellow" "$reset" "$name" "$pid"
    fi
    rm -f "$file"
  fi
}

# Report (never kill) foreign listeners: a port held by a PID we did not
# record is not ours to signal, and `kill -9` on it could take down an
# unrelated application.
report_port() {
  local name="$1" port="$2"
  if ! command -v lsof >/dev/null 2>&1; then
    return 0
  fi
  local pids
  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    # shellcheck disable=SC2086
    for pid in $pids; do
      printf "  %s[held]%s %s port %s still held by pid %s (not started by dev scripts; not signalling)\n" "$yellow" "$reset" "$name" "$port" "$pid"
    done
  fi
}

stop_pid frontend
stop_pid daemon
stop_pid api
report_port frontend "${FRONTEND_PORT:-3000}"
report_port daemon "${DAEMON_PORT:-9090}"
report_port sftp "${DAEMON_SFTP_PORT:-2022}"
report_port api "${API_PORT:-8080}"

if [ "$MODE" = "docker" ] && command -v docker >/dev/null 2>&1; then
  (cd "$ROOT/infra" && docker compose down >/dev/null 2>&1 || true)
  printf "  %s[stop]%s Docker Postgres/Redis stopped\n" "$yellow" "$reset"
fi

printf "%sGamePanel dev environment stopped.%s\n" "$green" "$reset"
