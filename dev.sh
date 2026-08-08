#!/bin/bash
#
# GamePanel macOS development launcher
#
#   ./dev.sh            start the full dev stack (native Postgres + Redis, API, Web, Beacon)
#   ./dev.sh status     show service status
#   ./dev.sh stop       stop everything the launcher started
#   ./dev.sh restart    stop + start
#   ./dev.sh reset-db   wipe the dev database (run if the master key changed and the API won't start)
#   ./dev.sh logs [api|web|beacon]
#
# API, Web and Beacon run as macOS launchd agents (survive terminal closes,
# restart on crash). Stop them with launchctl unload or ./dev.sh stop.
#
# Requirements: Go, Node.js/npm, Homebrew postgresql@16 and redis.
# Install missing ones with:
#   brew install postgresql@16 redis   (then: echo 'PATH="/opt/homebrew/opt/postgresql@16/bin:$PATH"' >> ~/.zshrc)

set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_DIR="$ROOT/.dev-pids"
LOG_API="$ROOT/api-dev.log"
LOG_API_ERR="$ROOT/api-dev.err.log"
LOG_FE="$ROOT/frontend-dev.log"
LOG_FE_ERR="$ROOT/frontend-dev.err.log"
LOG_BEACON="$ROOT/beacon-dev.log"
LOG_BEACON_ERR="$ROOT/beacon-dev.err.log"
SECRETS_FILE="$ROOT/.dev-secrets.env"
PLIST_DIR="$HOME/Library/LaunchAgents"
PLIST_API="$PLIST_DIR/com.gamepanel.api.plist"
PLIST_WEB="$PLIST_DIR/com.gamepanel.web.plist"
PLIST_BEACON="$PLIST_DIR/com.gamepanel.beacon.plist"
mkdir -p "$PLIST_DIR"

API_PORT=8080
FE_PORT=3000
BEACON_PORT=9090
DB_PORT=5432
REDIS_PORT=6379
DB_USER=gamepanel
DB_PASS=gamepanel
DB_NAME=gamepanel
DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@localhost:${DB_PORT}/${DB_NAME}?sslmode=disable"
REDIS_PW=CHANGE_ME

PG_DATA=/opt/homebrew/var/postgresql@16
PG_BIN=/opt/homebrew/opt/postgresql@16/bin

RED=$'\e[31m'; GRN=$'\e[32m'; YEL=$'\e[33m'; CYN=$'\e[36m'; BLD=$'\e[1m'; RST=$'\e[0m'

log_ok()   { printf "  ${GRN}[ok]${RST} %s\n" "$*"; }
log_info() { printf "  ${YEL}[..]${RST} %s\n" "$*"; }
log_err()  { printf "  ${RED}[!!]${RST} %s\n" "$*" >&2; }
fail()     { log_err "$*"; exit 1; }

port_busy() { nc -z 127.0.0.1 "$1" >/dev/null 2>&1; }
port_pid()  { lsof -ti :"$1" -sTCP:LISTEN 2>/dev/null | head -1; }

wait_for_port() { # port timeout_seconds
  local port=$1 timeout=$2 i=0
  while ! port_busy "$port" && [ "$i" -lt "$timeout" ]; do
    printf "."
    sleep 0.5
    i=$((i + 1))
  done
  [ "$i" -gt 0 ] && printf " "
}

# ---------------------------------------------------------------------------
# Secrets (persisted once so encrypted demo data stays decryptable across runs)
# ---------------------------------------------------------------------------
load_secrets() {
  mkdir -p "$PID_DIR"
  if [ -f "$SECRETS_FILE" ]; then
    set -a; . "$SECRETS_FILE"; set +a
  fi
  local sftp_pw
  sftp_pw="${DAEMON_SFTP_HOST_KEY_PASSPHRASE:-}"
  if [ -z "$DAEMON_NODE_TOKEN" ]; then DAEMON_NODE_TOKEN="dev-$(openssl rand -hex 8).$(openssl rand -hex 24)"; fi
  if [ -z "$API_AUTH_SECRET" ]; then API_AUTH_SECRET="dev-$(openssl rand -hex 32)"; fi
  if [ -z "$APP_KEY" ]; then APP_KEY="base64:$(openssl rand -base64 32)"; fi
  if [ -z "$FORGE_MASTER_KEY" ]; then FORGE_MASTER_KEY="$(openssl rand -base64 32)"; fi
  if [ -z "$sftp_pw" ]; then sftp_pw="dev-$(openssl rand -hex 16)"; fi
  umask 077
  cat > "$SECRETS_FILE" <<EOF
DAEMON_NODE_TOKEN=$DAEMON_NODE_TOKEN
API_AUTH_SECRET=$API_AUTH_SECRET
APP_KEY=$APP_KEY
FORGE_MASTER_KEY=$FORGE_MASTER_KEY
DAEMON_SFTP_HOST_KEY_PASSPHRASE=$sftp_pw
EOF
  umask 022
  DAEMON_SFTP_HOST_KEY_PASSPHRASE=$sftp_pw

  export APP_ENV=development
  export APP_CIPHER=AES-256-GCM
  export API_ADDR=":$API_PORT"
  export API_AUTH_SECRET
  export API_SEED_DEMO=true
  export APP_KEY
  export DATABASE_URL
  export FORGE_MASTER_KEY
  export FORGE_MASTER_KEY_ID=primary
  export FORGE_ALLOW_EPHEMERAL_MASTER_KEY=false
  export REDIS_ADDR="localhost:$REDIS_PORT"
  export REDIS_PASSWORD="$REDIS_PW"
  export SESSION_COOKIE_SECURE=false
  export NEXT_PUBLIC_API_URL=/api/v1
  # Demo node seeded by the API (API_SEED_DEMO). Not a secret.
  export DAEMON_NODE_ID="22222222-2222-2222-2222-222222222222"
}

# ---------------------------------------------------------------------------
# Native PostgreSQL
# ---------------------------------------------------------------------------
pg_ready() { psql "$DATABASE_URL" -tAc 'select 1' >/dev/null 2>&1; }

ensure_postgres() {
  if pg_ready; then log_ok "PostgreSQL-up   $GRN  health OK on :$DB_PORT$RST"; return 0; fi
  if port_busy "$DB_PORT"; then
    fail "Port $DB_PORT is busy (PID $(port_pid $DB_PORT)) but the gamepanel DB is unreachable. Stop that process, then re-run."
  fi
  if [ ! -x "$PG_BIN/pg_ctl" ]; then
    fail "Native PostgreSQL not found. Install with: brew install postgresql@16"
  fi
  log_info "Starting native PostgreSQL..."
  "$PG_BIN/pg_ctl" -D "$PG_DATA" -l "$PG_DATA/server.log" start >/dev/null 2>&1 || fail "PostgreSQL failed to start. See $PG_DATA/server.log"
  for _ in $(seq 1 20); do pg_ready && break; sleep 0.5; done
  if ! pg_ready; then fail "PostgreSQL still not accepting connections after starting."; fi
  echo "$(date +%s) pg" > "$PID_DIR/started_pg"
  log_ok "PostgreSQL started (native)"
  # ensure role + database
  psql -d postgres -v ON_ERROR_STOP=1 -c "CREATE ROLE $DB_USER LOGIN PASSWORD '$DB_PASS' SUPERUSER;" 2>/dev/null || true
  psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1 || psql -d postgres -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;" >/dev/null
  log_ok "Database '$DB_NAME' ready"
}

# ---------------------------------------------------------------------------
# Native Redis
# ---------------------------------------------------------------------------
ensure_redis() {
  if redis-cli ping >/dev/null 2>&1; then
    REDIS_PASSWORD=""
    log_ok "Redis already running (:6379, no auth)"
    return 0
  fi
  if redis-cli -a "$REDIS_PW" --no-auth-warning ping >/dev/null 2>&1; then
    log_ok "Redis already running (:6379, CHANGE_ME auth)"
    return 0
  fi
  if port_busy "$REDIS_PORT"; then
    fail "Port :$REDIS_PORT is occupied by PID $(port_pid $REDIS_PORT) but it is not a reachable Redis. Stop it first."
  fi
  command -v redis-server >/dev/null 2>&1 || fail "Redis not installed. Install with: brew install redis"
  log_info "Starting Redis..."
  redis-server --daemonize yes --port 6379 --requirepass "$REDIS_PW" --appendonly yes
  sleep 0.5
  redis-cli -a "$REDIS_PW" --no-auth-warning ping >/dev/null 2>&1 || fail "Redis started but isn't responding on :6379."
  touch "$PID_DIR/started_redis"
  log_ok "Redis started (native, password '$REDIS_PW')"
}

# ---------------------------------------------------------------------------
# Go binaries (skip rebuild when up to date)
# ---------------------------------------------------------------------------
needs_rebuild() { # dir binary
  local dir=$1 bin=$2
  [ ! -x "$bin" ] && return 0
  local newest tmp
  tmp=$(find "$dir" -name '*.go' -o -name go.mod -o -name go.sum 2>/dev/null)
  newest=$(printf '%s\n' "$tmp" | xargs ls -lt 2>/dev/null | head -1 | awk '{print $NF}')
  [ -n "$newest" ] && [ "$newest" -nt "$bin" ] && return 0
  return 1
}
build_if_needed() { # label dir binary pkgpath
  local label=$1 dir=$2 bin=$3 pkg=$4
  if needs_rebuild "$dir" "$bin"; then
    log_info "Building $label..."
    ( cd "$dir" && go build -o "$bin" "$pkg" 2>"$bin.build.err" ) || {
      log_err "$label build failed:"; tail -5 "$bin.build.err" >&2; rm -f "$bin.build.err"; exit 1
    }
    rm -f "$bin.build.err"
    log_ok "Built $label"
  else
    log_info "$label up to date (skipping rebuild)"
  fi
}

# ---------------------------------------------------------------------------
# API (launchd agent)
# ---------------------------------------------------------------------------
start_api() {
  log_header "API"
  build_if_needed "API" "$ROOT/forge/api" "$ROOT/forge/api/api" ./cmd/api
  if port_busy "$API_PORT"; then
    log_ok "API already running on :$API_PORT"
    return 0
  fi
  cat > "$PLIST_API" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.gamepanel.api</string>
    <key>ProgramArguments</key>
    <array>
        <string>$ROOT/forge/api/api</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>APP_ENV</key><string>development</string>
        <key>APP_CIPHER</key><string>AES-256-GCM</string>
        <key>API_ADDR</key><string>:$API_PORT</string>
        <key>API_AUTH_SECRET</key><string>$API_AUTH_SECRET</string>
        <key>API_SEED_DEMO</key><string>true</string>
        <key>APP_KEY</key><string>$APP_KEY</string>
        <key>DATABASE_URL</key><string>$DATABASE_URL</string>
        <key>FORGE_MASTER_KEY</key><string>$FORGE_MASTER_KEY</string>
        <key>FORGE_MASTER_KEY_ID</key><string>primary</string>
        <key>FORGE_ALLOW_EPHEMERAL_MASTER_KEY</key><string>false</string>
        <key>REDIS_ADDR</key><string>localhost:$REDIS_PORT</string>
        <key>REDIS_PASSWORD</key><string>$REDIS_PASSWORD</string>
        <key>SESSION_COOKIE_SECURE</key><string>false</string>
        <key>DAEMON_NODE_ID</key><string>$DAEMON_NODE_ID</string>
        <key>DAEMON_NODE_TOKEN</key><string>$DAEMON_NODE_TOKEN</string>
        <key>BEACON_BASE_URL</key><string>http://127.0.0.1:$BEACON_PORT</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>$ROOT/forge/api</string>
    <key>StandardOutPath</key>
    <string>$LOG_API</string>
    <key>StandardErrorPath</key>
    <string>$LOG_API_ERR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict><key>Crashed</key><true/></dict>
    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
PLIST

  launchctl unload "$PLIST_API" 2>/dev/null
  sleep 0.5
  launchctl load "$PLIST_API" 2>&1 || fail "launchctl failed to load the API plist"
  log_info "Waiting for API (first run runs 161 migrations + demo seed, can take ~30s)..."
  local ok=0
  for _ in $(seq 1 90); do
    sleep 0.5
    if curl -sf "http://localhost:${API_PORT}/api/v1/health/ready" >/dev/null 2>&1; then ok=1; break; fi
  done
  if [ "$ok" -eq 1 ]; then
    log_ok "API ready at http://localhost:$API_PORT/api/v1"
  else
    log_err "API FAILED to start (launchd: com.gamepanel.api)."
    if grep -q "encrypted secret authentication failed\|decrypt stored secret" "$LOG_API_ERR" 2>/dev/null; then
      log_err "The database was encrypted with a different FORGE_MASTER_KEY than .dev-secrets.env."
      log_err "Fix: stop the stack, then run:  ./dev.sh reset-db   (wipes the demo DB and re-seeds with the current key)"
    else
      log_err "Last log lines:"; tail -8 "$LOG_API_ERR" >&2
    fi
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Frontend (launchd agent)
# ---------------------------------------------------------------------------
start_web() {
  log_header "Frontend (Next.js)"
  if [ ! -d "$ROOT/forge/web/node_modules" ]; then
    log_info "Installing frontend dependencies (one-time, may take a minute)..."
    ( cd "$ROOT/forge/web" && npm ci ) || fail "npm ci failed"
    log_ok "Dependencies installed"
  fi
  if port_busy "$FE_PORT"; then
    log_ok "Frontend already running on :$FE_PORT"
    return 0
  fi
  cat > "$PLIST_WEB" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.gamepanel.web</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/env</string>
        <string>npm</string>
        <string>run</string>
        <string>dev</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>NEXT_PUBLIC_API_URL</key><string>/api/v1</string>
        <key>API_INTERNAL_URL</key><string>http://127.0.0.1:$API_PORT</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>$ROOT/forge/web</string>
    <key>StandardOutPath</key>
    <string>$LOG_FE</string>
    <key>StandardErrorPath</key>
    <string>$LOG_FE_ERR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict><key>Crashed</key><true/></dict>
    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
PLIST

  launchctl unload "$PLIST_WEB" 2>/dev/null
  sleep 0.5
  launchctl load "$PLIST_WEB" 2>&1 || fail "launchctl failed to load the web plist"
  log_info "Waiting for Frontend (first compile can take ~30s)..."
  local ok=0
  for _ in $(seq 1 120); do
    sleep 0.5
    if curl -sf -o /dev/null "http://localhost:$FE_PORT/" 2>/dev/null; then ok=1; break; fi
  done
  if [ "$ok" -eq 1 ]; then
    log_ok "Frontend ready at http://localhost:$FE_PORT  (setup: /setup if no admin exists)"
  else
    log_err "Frontend FAILED to start (launchd: com.gamepanel.web)."
    tail -15 "$LOG_FE_ERR" >&2
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Beacon (launchd daemon)
# ---------------------------------------------------------------------------
start_beacon() {
  log_header "Beacon daemon (launchd)"
  build_if_needed "beacon" "$ROOT/beacon" "$ROOT/beacon/daemon" ./cmd/daemon
  mkdir -p /tmp/beacon-data
  mkdir -p "$HOME/Library/LaunchAgents"

  cat > "$PLIST_BEACON" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.gamepanel.beacon</string>
    <key>ProgramArguments</key>
    <array>
        <string>$ROOT/beacon/daemon</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>DAEMON_ADDR</key>
        <string>:$BEACON_PORT</string>
        <key>DAEMON_DATA_DIR</key>
        <string>/tmp/beacon-data</string>
        <key>DAEMON_NODE_ID</key>
        <string>$DAEMON_NODE_ID</string>
        <key>DAEMON_NODE_TOKEN</key>
        <string>$DAEMON_NODE_TOKEN</string>
        <key>PANEL_API_URL</key>
        <string>http://localhost:${API_PORT}/api/v1</string>
        <key>DAEMON_ALLOW_MOCK_RUNTIME</key>
        <string>true</string>
        <key>DAEMON_ALLOW_UNPINNED_IMAGES</key>
        <string>true</string>
        <key>DAEMON_SFTP_HOST_KEY_PASSPHRASE</key>
        <string>$DAEMON_SFTP_HOST_KEY_PASSPHRASE</string>
        <key>APP_ENV</key>
        <string>development</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>/tmp/beacon-data</string>
    <key>StandardOutPath</key>
    <string>$LOG_BEACON</string>
    <key>StandardErrorPath</key>
    <string>$LOG_BEACON_ERR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>Crashed</key>
        <true/>
    </dict>
    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
PLIST

  launchctl unload "$PLIST_BEACON" 2>/dev/null
  sleep 1
  launchctl load "$PLIST_BEACON" 2>&1 || fail "launchctl failed to load the beacon plist"
  local ok=0
  for _ in $(seq 1 30); do
    sleep 0.5
    if curl -sf "http://localhost:$BEACON_PORT/health" >/dev/null 2>&1; then ok=1; break; fi
  done
  if [ "$ok" -eq 1 ]; then
    log_ok "Beacon ready at http://localhost:$BEACON_PORT/health (launchd: com.gamepanel.beacon)"
  else
    log_err "Beacon FAILED to start."
    tail -15 "$LOG_BEACON_ERR" >&2
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# status
# ---------------------------------------------------------------------------
status() {
  echo ""
  echo "$BLD  GamePanel dev status:$RST"
  printf "  %-9s %-30s %s\n" "PostgreSQL" "localhost:$DB_PORT"  "$(pg_ready && printf '%s' "$GRN""ok$RST" || printf '%s' "$RED""down$RST")"
  printf '  %-9s %-30s %s\n' "API" "localhost:$API_PORT/api/v1" "$(port_busy $API_PORT && printf '%s' "$GRN""up$RST" || printf '%s' "$RED""down$RST")"
  printf '  %-9s %-30s %s\n' "Frontend" "http://localhost:$FE_PORT" "$(port_busy $FE_PORT && printf '%s' "$GRN""up$RST" || printf '%s' "$RED""down$RST")"
  printf '  %-9s %-30s %s\n' "Beacon" "http://localhost:$BEACON_PORT/health" "$(port_busy $BEACON_PORT && printf '%s' "$GRN""up$RST" || printf '%s' "$RED""down$RST")"
  echo ""
  echo "  launchd agents:  launchctl list | grep gamepanel"
}

# ---------------------------------------------------------------------------
# stop
# ---------------------------------------------------------------------------
stop() {
  log_header "Stopping dev stack"
  launchctl unload "$PLIST_API" 2>/dev/null; launchctl unload "$PLIST_WEB" 2>/dev/null; launchctl unload "$PLIST_BEACON" 2>/dev/null
  sleep 1
  log_ok "API / Web / Beacon stopped (launchd agents unloaded)"
  # postgres + redis only if the launcher started them
  if [ -f "$PID_DIR/started_redis" ]; then
    redis-cli -a "$REDIS_PW" --no-auth-warning shutdown nosave 2>/dev/null; rm -f "$PID_DIR/started_redis"
    log_ok "Redis stopped (launcher-started)"
  fi
  if [ -f "$PID_DIR/started_pg" ]; then
    "$PG_BIN/pg_ctl" -D "$PG_DATA" stop -m fast >/dev/null 2>&1 && log_ok "PostgreSQL stopped"
    rm -f "$PID_DIR/started_pg"
  else
    log_info "PostgreSQL left running (not started by this launcher)"
  fi
}

# ---------------------------------------------------------------------------
# reset-db
# ---------------------------------------------------------------------------
reset_db() {
  log_header "Resetting database (destroys demo data)"
  "$PG_BIN/pg_ctl" -D "$PG_DATA" -w start >/dev/null 2>&1
  psql -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB_NAME' AND pid <> pg_backend_pid();" >/dev/null 2>&1
  psql -d postgres -c "DROP DATABASE IF EXISTS $DB_NAME" >/dev/null || fail "Could not drop database '$DB_NAME'. Close psql/API connections and re-run."
  psql -d postgres -c "CREATE DATABASE $DB_NAME OWNER $DB_USER" >/dev/null || fail "Could not recreate database '$DB_NAME'."
  log_ok "Database '$DB_NAME' recreated. Next start re-runs migrations + demo seed."
}

# ---------------------------------------------------------------------------
# logs
# ---------------------------------------------------------------------------
cmd_logs() {
  case "${1:-all}" in
    api) tail -f "$LOG_API" ;;
    web) tail -f "$LOG_FE" ;;
    beacon) tail -f "$LOG_BEACON" ;;
    *) tail -f "$LOG_FE" "$LOG_FE_ERR" "$LOG_API" "$LOG_API_ERR" ;;
  esac
}

# ---------------------------------------------------------------------------
log_header() { printf "\n${BLD}== ${1} ==${RST}\n"; }

cmd="${1:-start}"
case "$cmd" in
  start)  load_secrets; ensure_postgres; ensure_redis; log_header "Starting dev stack"; start_api; start_web; start_beacon; echo ""; status; log_info "Logs: ./dev.sh logs    Stop: ./dev.sh stop" ;;
  status) status ;;
  stop)   stop ;;
  restart) stop; sleep 1; load_secrets; ensure_postgres; ensure_redis; start_api; start_web; start_beacon; status ;;
  reset-db) reset_db ;;
  logs)   cmd_logs "${2:-all}" ;;
  *) echo "usage: $0 {start|status|stop|restart|reset-db|logs}"; exit 1 ;;
esac