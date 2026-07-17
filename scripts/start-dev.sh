#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT/.dev-pids"
LOG_DIR="$ROOT/.dev-logs"
MODE="${1:-docker}"

DB_USER="${DB_USER:-gamepanel}"
DB_PASS="${DB_PASS:-gamepanel}"
DB_NAME="${DB_NAME:-gamepanel}"
DB_PORT="${DB_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
API_PORT="${API_PORT:-8080}"
DAEMON_PORT="${DAEMON_PORT:-9090}"
DAEMON_SFTP_PORT="${DAEMON_SFTP_PORT:-2022}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
# Load environment variables from local .env file if it exists
if [ -f "$ROOT/.env" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    # Remove carriage returns (Windows compat)
    line="${line//$'\r'/}"
    # Strip leading/trailing whitespace
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    # Skip empty lines and comments
    if [ -z "$line" ] || [ "${line:0:1}" = "#" ]; then
      continue
    fi
    # Only export if it has an equals sign
    if [[ "$line" == *"="* ]]; then
      key="${line%%=*}"
      value="${line#*=}"
      # Remove surrounding quotes if present
      value="${value#[\"\']}"
      value="${value%[\"\']}"
      export "$key"="$value"
    fi
  done < "$ROOT/.env"
fi

export DATABASE_URL="${DATABASE_URL:-postgres://${DB_USER}:${DB_PASS}@localhost:${DB_PORT}/${DB_NAME}?sslmode=disable}"
export API_ADDR="${API_ADDR:-:${API_PORT}}"
export API_AUTH_SECRET="${API_AUTH_SECRET:-dev-api-secret}"
export APP_ENV="${APP_ENV:-development}"
export DAEMON_NODE_TOKEN="${DAEMON_NODE_TOKEN:-devnodetoken0001.dev-node-token}"
export API_DEMO_MODE="${API_DEMO_MODE:-false}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:${REDIS_PORT}}"
export MIGRATIONS_DIR="${MIGRATIONS_DIR:-$ROOT/forge/api/migrations}"
export NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://localhost:${API_PORT}/api/v1}"
export SEED_NODE_BASE_URL="${SEED_NODE_BASE_URL:-http://localhost:${DAEMON_PORT}}"
export FORGE_MASTER_KEY="${FORGE_MASTER_KEY:-YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=}"
export FORGE_MASTER_KEY_ID="${FORGE_MASTER_KEY_ID:-primary}"
export FORGE_ALLOW_EPHEMERAL_MASTER_KEY="${FORGE_ALLOW_EPHEMERAL_MASTER_KEY:-false}"


mkdir -p "$PID_DIR" "$LOG_DIR"

red=$'\033[31m'
green=$'\033[32m'
yellow=$'\033[33m'
cyan=$'\033[36m'
reset=$'\033[0m'

info() { printf "%s==>%s %s\n" "$cyan" "$reset" "$1"; }
ok() { printf "  %s[ok]%s %s\n" "$green" "$reset" "$1"; }
warn() { printf "  %s[warn]%s %s\n" "$yellow" "$reset" "$1"; }
fail() { printf "  %s[error]%s %s\n" "$red" "$reset" "$1"; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

port_open() {
  local port="$1"
  if have nc; then
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

wait_port() {
  local name="$1" port="$2" attempts="${3:-30}"
  printf "  Waiting for %s on port %s" "$name" "$port"
  for _ in $(seq 1 "$attempts"); do
    if port_open "$port"; then printf " %sready%s\n" "$green" "$reset"; return 0; fi
    printf "."
    sleep 1
  done
  printf " %stimeout%s\n" "$yellow" "$reset"
  return 1
}

kill_stale_ports() {
  info "Cleaning up stale processes on critical ports"
  local ports=("${API_PORT:-8080}" "${DAEMON_PORT:-9090}" "${FRONTEND_PORT:-3000}" "${DAEMON_SFTP_PORT:-2022}")
  for port in "${ports[@]}"; do
    if have lsof; then
      local pids
      pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
      if [ -n "$pids" ]; then
        for pid in $pids; do
          kill -9 "$pid" >/dev/null 2>&1 || true
          printf "  %s[killed]%s stale process pid %s on port %s\n" "$yellow" "$reset" "$pid" "$port"
        done
      fi
    fi
  done
  for port in "${ports[@]}"; do
    for _ in 1 2 3 4 5; do
      port_open "$port" || break
      sleep 1
    done
    if port_open "$port"; then
      fail "port $port is still in use after cleanup"
    fi
  done
}

ensure_port_free() {
  local name="$1" port="$2"
  if port_open "$port"; then
    fail "$name port $port is already in use. Run './scripts/stop-dev.sh' or stop the existing process before starting GamePanel."
  fi
}

start_docker_services() {
  have docker || fail "Docker is required for default mode. Use './scripts/start-dev.sh native' to use local Postgres/Redis."
  docker_available || fail "Docker is installed but the daemon is not reachable. Start Docker Desktop or run './scripts/start-dev.sh native' with local Postgres/Redis."
  info "Starting PostgreSQL and Redis with Docker Compose"
  (cd "$ROOT/infra" && docker compose up -d postgres redis >/dev/null)
  wait_port "PostgreSQL" "$DB_PORT" 30 || fail "PostgreSQL did not become reachable."
  wait_port "Redis" "$REDIS_PORT" 15 || warn "Redis did not become reachable; continuing."
}

check_native_services() {
  info "Checking native PostgreSQL and Redis"
  wait_port "PostgreSQL" "$DB_PORT" 5 || fail "PostgreSQL is not running on port $DB_PORT."
  if wait_port "Redis" "$REDIS_PORT" 5; then
    ok "Redis available on port $REDIS_PORT"
  else
    warn "Redis unavailable; API will run without Redis if supported."
    export REDIS_ADDR=""
  fi
}

start_api() {
  have go || fail "Go is required to start the API."
  info "Starting API"
  ensure_port_free "API" "$API_PORT"
  (
    cd "$ROOT/forge/api"
    nohup go run ./cmd/api >"$LOG_DIR/api.log" 2>"$LOG_DIR/api.err.log" &
    pid=$!
    echo "$pid" >"$PID_DIR/api.pid"
    disown "$pid" 2>/dev/null || true
  )
  wait_port "API" "$API_PORT" 30 || {
    printf "\n  %s--- API stdout (last 30 lines) ---%s\n" "$yellow" "$reset"
    tail -n 30 "$LOG_DIR/api.log" 2>/dev/null || true
    printf "\n  %s--- API stderr (last 30 lines) ---%s\n" "$red" "$reset"
    tail -n 30 "$LOG_DIR/api.err.log" 2>/dev/null || true
    printf "\n"
    fail "API did not become reachable. Check migration or startup errors above."
  }
  ok "API: http://localhost:${API_PORT}"
}

start_daemon() {
  have go || fail "Go is required to start the daemon."
  info "Starting daemon"
  ensure_port_free "Daemon" "$DAEMON_PORT"
  mkdir -p "$ROOT/.dev-data/servers"
  (
    cd "$ROOT/beacon"
    nohup env \
      DAEMON_ADDR=":${DAEMON_PORT}" \
      DAEMON_SFTP_ADDR=":${DAEMON_SFTP_PORT}" \
      DAEMON_DATA_DIR="$ROOT/.dev-data/servers" \
      DAEMON_NODE_ID="${DAEMON_NODE_ID:-}" \
      DAEMON_NODE_TOKEN="$DAEMON_NODE_TOKEN" \
      PANEL_API_URL="http://localhost:${API_PORT}/api/v1" \
      APP_ENV="$APP_ENV" \
      DAEMON_ALLOW_MOCK_RUNTIME="${DAEMON_ALLOW_MOCK_RUNTIME:-false}" \
      go run ./cmd/daemon >"$LOG_DIR/daemon.log" 2>"$LOG_DIR/daemon.err.log" &
    pid=$!
    echo "$pid" >"$PID_DIR/daemon.pid"
    disown "$pid" 2>/dev/null || true
  )
  wait_port "Daemon" "$DAEMON_PORT" 30 || {
    printf "\n  %s--- Daemon stdout (last 30 lines) ---%s\n" "$yellow" "$reset"
    tail -n 30 "$LOG_DIR/daemon.log" 2>/dev/null || true
    printf "\n  %s--- Daemon stderr (last 30 lines) ---%s\n" "$red" "$reset"
    tail -n 30 "$LOG_DIR/daemon.err.log" 2>/dev/null || true
    printf "\n"
    fail "Daemon did not become reachable."
  }
  ok "Daemon: http://localhost:${DAEMON_PORT}"
  wait_port "SFTP" "$DAEMON_SFTP_PORT" 15 || fail "SFTP did not become reachable."
  ok "SFTP: localhost:${DAEMON_SFTP_PORT}"
}

start_frontend() {
  have npm || fail "Node/npm is required to start the frontend."
  info "Starting frontend"
  ensure_port_free "Frontend" "$FRONTEND_PORT"
  (
    cd "$ROOT/forge/web"
    nohup env NEXT_PUBLIC_API_URL="$NEXT_PUBLIC_API_URL" npm run dev -- -p "$FRONTEND_PORT" >"$LOG_DIR/frontend.log" 2>"$LOG_DIR/frontend.err.log" &
    pid=$!
    echo "$pid" >"$PID_DIR/frontend.pid"
    disown "$pid" 2>/dev/null || true
  )
  wait_port "Frontend" "$FRONTEND_PORT" 45 || warn "Frontend may still be compiling. Check $LOG_DIR/frontend.log."
  ok "Frontend: http://localhost:${FRONTEND_PORT}"
}

wait_http() {
  local name="$1" url="$2" attempts="${3:-30}"
  printf "  Waiting for %s at %s" "$name" "$url"
  for _ in $(seq 1 "$attempts"); do
    if have curl; then
      if curl -fsS --max-time 2 -o /dev/null "$url" 2>/dev/null; then
        printf " %sready%s\n" "$green" "$reset"
        return 0
      fi
    elif have wget; then
      if wget -q --timeout=2 -O /dev/null "$url" 2>/dev/null; then
        printf " %sready%s\n" "$green" "$reset"
        return 0
      fi
    fi
    printf "."
    sleep 1
  done
  printf " %stimeout%s\n" "$yellow" "$reset"
  return 1
}

verify_health() {
  info "Running final health checks"
  if have pg_isready; then
    pg_isready -h localhost -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" >/dev/null || fail "PostgreSQL health check failed."
    ok "Health check passed for PostgreSQL"
  else
    wait_port "PostgreSQL" "$DB_PORT" 10 || fail "PostgreSQL health check failed."
    ok "Health check passed for PostgreSQL"
  fi
  if [ -n "${REDIS_ADDR:-}" ]; then
    if have redis-cli; then
      redis-cli -h localhost -p "$REDIS_PORT" ping 2>/dev/null | grep -q '^PONG$' || fail "Redis health check failed."
      ok "Health check passed for Redis"
    else
      wait_port "Redis" "$REDIS_PORT" 10 || fail "Redis health check failed."
      ok "Health check passed for Redis"
    fi
  fi
  local services=(
    "API" "http:http://localhost:${API_PORT}/api/v1/health"
    "Daemon" "http:http://localhost:${DAEMON_PORT}/health"
    "Frontend" "http:http://localhost:${FRONTEND_PORT}/"
    "SFTP" "tcp:localhost:${DAEMON_SFTP_PORT}"
  )

  for ((i=0; i<${#services[@]}; i+=2)); do
    local name="${services[i]}"
    local spec="${services[i+1]}"
    local proto="${spec%%:*}"
    local target="${spec#*:}"
    local port="${target##*:}"
    if [ "$proto" = "http" ]; then
      port="${port%%/*}"
      if wait_http "$name" "$target" 15; then
        ok "Health check passed for $name"
      else
        fail "Health check failed for $name on $target"
      fi
    else
      if wait_port "$name" "$port" 10; then
        ok "Health check passed for $name"
      else
        fail "Health check failed for $name on $target"
      fi
    fi
  done
}

verify_tracked_processes() {
  info "Verifying managed dev processes are still alive"
  local names=("api" "daemon" "frontend")
  sleep 2
  for name in "${names[@]}"; do
    local file="$PID_DIR/${name}.pid"
    if [ ! -f "$file" ]; then
      fail "$name pid file was not created"
    fi
    local pid
    pid="$(cat "$file")"
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      fail "$name process exited after startup; inspect $LOG_DIR/${name}.log and $LOG_DIR/${name}.err.log"
    fi
    ok "$name process is alive (pid $pid)"
  done
}

case "$MODE" in
  docker|"") kill_stale_ports; start_docker_services ;;
  native) kill_stale_ports; check_native_services ;;
  *) fail "Unknown mode '$MODE'. Use 'docker' or 'native'." ;;
esac

start_api
start_daemon
start_frontend
verify_health
verify_tracked_processes

printf "\n%sGamePanel dev environment is running.%s\n" "$green" "$reset"
printf "  Frontend: http://localhost:%s\n" "$FRONTEND_PORT"
printf "  API:      http://localhost:%s/api/v1\n" "$API_PORT"
printf "  Daemon:   http://localhost:%s\n" "$DAEMON_PORT"
printf "  Logs:     ./scripts/logs.sh\n"
printf "  Stop:     ./scripts/stop-dev.sh\n"
