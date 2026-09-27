#!/usr/bin/env bash
# Canonical log viewer — single runner with the dev scripts.
# Logs live in .dev-logs/ (written by scripts/dev/start-dev.sh and native.sh).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT/.dev-logs"
SERVICE="${1:-all}"

show_tail() {
  local name="$1" file="$2"
  printf "\n== %s ==\n" "$name"
  if [ -f "$file" ]; then
    tail -n 80 "$file"
  else
    printf "No log file at %s\n" "$file"
  fi
}

# "frontend" and "web" are aliases: native.sh writes web.log, the dev
# runners write frontend.log. Show whichever exists.
show_frontend() {
  if [ -f "$LOG_DIR/frontend.log" ] || [ -f "$LOG_DIR/frontend.err.log" ]; then
    show_tail "Frontend" "$LOG_DIR/frontend.log"; show_tail "Frontend errors" "$LOG_DIR/frontend.err.log"
  else
    show_tail "Web" "$LOG_DIR/web.log"
  fi
}

case "$SERVICE" in
  api) show_tail "API" "$LOG_DIR/api.log"; show_tail "API errors" "$LOG_DIR/api.err.log" ;;
  daemon|beacon) show_tail "Daemon" "$LOG_DIR/daemon.log"; show_tail "Daemon errors" "$LOG_DIR/daemon.err.log"; show_tail "Beacon" "$LOG_DIR/beacon.log" ;;
  frontend|web) show_frontend ;;
  all) show_tail "API" "$LOG_DIR/api.log"; show_tail "API errors" "$LOG_DIR/api.err.log"; show_tail "Daemon" "$LOG_DIR/daemon.log"; show_tail "Daemon errors" "$LOG_DIR/daemon.err.log"; show_tail "Beacon" "$LOG_DIR/beacon.log"; show_frontend ;;
  *) printf "Usage: %s [all|api|daemon|frontend|web]\n" "$0"; exit 1 ;;
esac
