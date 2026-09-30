#!/usr/bin/env bash
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

case "$SERVICE" in
  api) show_tail "API" "$LOG_DIR/api.log"; show_tail "API errors" "$LOG_DIR/api.err.log" ;;
  daemon) show_tail "Daemon" "$LOG_DIR/daemon.log"; show_tail "Daemon errors" "$LOG_DIR/daemon.err.log" ;;
  frontend) show_tail "Frontend" "$LOG_DIR/frontend.log"; show_tail "Frontend errors" "$LOG_DIR/frontend.err.log" ;;
  all) show_tail "API" "$LOG_DIR/api.log"; show_tail "API errors" "$LOG_DIR/api.err.log"; show_tail "Daemon" "$LOG_DIR/daemon.log"; show_tail "Daemon errors" "$LOG_DIR/daemon.err.log"; show_tail "Frontend" "$LOG_DIR/frontend.log"; show_tail "Frontend errors" "$LOG_DIR/frontend.err.log" ;;
  *) printf "Usage: %s [all|api|daemon|frontend]\n" "$0"; exit 1 ;;
esac
