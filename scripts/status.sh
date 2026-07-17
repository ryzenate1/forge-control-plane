#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT/.dev-pids"
DB_PORT="${DB_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
API_PORT="${API_PORT:-8080}"
DAEMON_PORT="${DAEMON_PORT:-9090}"
DAEMON_SFTP_PORT="${DAEMON_SFTP_PORT:-2022}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

green=$'\033[32m'
red=$'\033[31m'
yellow=$'\033[33m'
reset=$'\033[0m'

port_open() {
  local port="$1"
  if command -v nc >/dev/null 2>&1; then
    nc -z -G 1 127.0.0.1 "$port" >/dev/null 2>&1 || nc -z -w 1 127.0.0.1 "$port" >/dev/null 2>&1
    return $?
  fi
  (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1
}

docker_available() {
  (docker info >/dev/null 2>&1) &
  local pid=$!
  for _ in 1 2 3; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      wait "$pid"
      return $?
    fi
    sleep 1
  done
  kill -9 "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
  return 1
}

service_line() {
  local name="$1" port="$2" url="$3"
  if port_open "$port"; then
    printf "  %s[up]%s   %-12s %s\n" "$green" "$reset" "$name" "$url"
  else
    printf "  %s[down]%s %-12s %s\n" "$red" "$reset" "$name" "$url"
  fi
}

pid_line() {
  local name="$1"
  local file="$PID_DIR/${name}.pid"
  if [ -f "$file" ] && kill -0 "$(cat "$file")" >/dev/null 2>&1; then
    printf "  %s[pid]%s  %-12s %s\n" "$yellow" "$reset" "$name" "$(cat "$file")"
  fi
}

printf "GamePanel development status\n"
service_line Frontend "$FRONTEND_PORT" "http://localhost:${FRONTEND_PORT}"
service_line API "$API_PORT" "http://localhost:${API_PORT}/api/v1"
service_line Daemon "$DAEMON_PORT" "http://localhost:${DAEMON_PORT}"
service_line SFTP "$DAEMON_SFTP_PORT" "localhost:${DAEMON_SFTP_PORT}"
service_line Postgres "$DB_PORT" "localhost:${DB_PORT}"
service_line Redis "$REDIS_PORT" "localhost:${REDIS_PORT}"
pid_line frontend
pid_line daemon
pid_line api

if command -v docker >/dev/null 2>&1; then
  printf "\nDocker services:\n"
  if docker_available; then
    (cd "$ROOT/deploy" && docker compose ps 2>/dev/null || true)
  else
    printf "  Docker daemon is not reachable. Start Docker Desktop or use native mode.\n"
  fi
fi
