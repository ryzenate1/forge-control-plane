#!/bin/bash
# Forge Plane - native macOS launcher.
#
# PostgreSQL and Redis run as Homebrew services, Beacon runs as a launchd
# user agent, and the API and web dashboard run as background processes.
#
#   ./native.sh start | stop | restart | status | logs

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BREW_PREFIX="$(brew --prefix)"
export PATH="$BREW_PREFIX/bin:$BREW_PREFIX/opt/postgresql@16/bin:$PATH"

DB_USER=gamepanel
DB_PASS=gamepanel
DB_NAME=gamepanel
DB_PORT=5432
REDIS_PORT=6379
API_PORT=8080
WEB_PORT=3000
BEACON_PORT=9090

PG_FORMULA=postgresql@16
BEACON_LABEL=com.gamepanel.beacon
PLIST="$HOME/Library/LaunchAgents/$BEACON_LABEL.plist"

LOG_DIR="$ROOT/.dev-logs"
PID_DIR="$ROOT/.dev-pids"
DATA_DIR="$ROOT/.dev-data"
SECRETS="$ROOT/.dev-secrets.env"

if [ -S "$HOME/.docker/run/docker.sock" ]; then
    DOCKER_SOCK="$HOME/.docker/run/docker.sock"
elif [ -S "$HOME/.colima/default/docker.sock" ]; then
    DOCKER_SOCK="$HOME/.colima/default/docker.sock"
elif [ -S "/var/run/docker.sock" ]; then
    DOCKER_SOCK="/var/run/docker.sock"
else
    DOCKER_SOCK="$HOME/.colima/default/docker.sock"
fi

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'; CYAN=$'\033[0;36m'; NC=$'\033[0m'

info() { printf '  %s\n' "$1"; }
ok()   { printf '  %s[ok]%s   %s\n' "$GREEN" "$NC" "$1"; }
warn() { printf '  %s[warn]%s %s\n' "$YELLOW" "$NC" "$1"; }
fail() { printf '  %s[fail]%s %s\n' "$RED" "$NC" "$1"; }
head_() { printf '\n%s=== %s ===%s\n' "$CYAN" "$1" "$NC"; }

port_open() { nc -z 127.0.0.1 "$1" >/dev/null 2>&1; }

# PID of whatever is listening on a port, or empty if nothing is. An open port
# is not evidence that *our* process opened it, so anything that reports a tier
# as started has to compare this against the pid we spawned.
port_owner() { lsof -nP -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null | head -1; }

# Start a process in its own session so it outlives this script and whatever
# terminal invoked it.
spawn_detached() {
    local name=$1 workdir=$2; shift 2
    python3 - "$name" "$workdir" "$PID_DIR" "$LOG_DIR" "$@" <<'PY'
import subprocess, sys, os
name, workdir, pid_dir, log_dir, *argv = sys.argv[1:]
log = open(os.path.join(log_dir, name + ".log"), "ab")
proc = subprocess.Popen(
    argv, cwd=workdir, stdout=log, stderr=subprocess.STDOUT,
    stdin=subprocess.DEVNULL, start_new_session=True,
)
with open(os.path.join(pid_dir, name + ".pid"), "w") as handle:
    handle.write(str(proc.pid))
PY
}

wait_port() {
    local port=$1 tries=${2:-30} i=0
    while [ "$i" -lt "$tries" ]; do
        port_open "$port" && return 0
        sleep 1; i=$((i + 1))
    done
    return 1
}

# Wait for the tier named $1 to serve $2, and confirm the listener belongs to
# the process we just spawned.
#
# `wait_port` alone cannot tell "our server came up" from "an orphan from an
# earlier run still owns the port". That distinction matters: a dev server that
# lost the bind keeps serving the code and config it loaded at *its* start, so
# the operator edits a file, sees the old output, and has no way to know why.
# Exit codes: 0 ours, 2 someone else's, 3 nothing listening.
confirm_listener() {
    local name=$1 port=$2 tries=${3:-30}
    local pidfile="$PID_DIR/$name.pid" ours owner
    ours="$(cat "$pidfile" 2>/dev/null || true)"

    local i=0
    while [ "$i" -lt "$tries" ]; do
        # Our own process dying is conclusive; stop waiting out the timeout.
        if [ -n "$ours" ] && ! kill -0 "$ours" 2>/dev/null; then break; fi
        port_open "$port" && break
        sleep 1; i=$((i + 1))
    done

    port_open "$port" || return 3
    owner="$(port_owner "$port")"
    # No lsof reading is not a mismatch: report success rather than inventing a
    # conflict we cannot actually see.
    [ -z "$owner" ] && return 0
    [ -n "$ours" ] && [ "$owner" = "$ours" ] && return 0

    # npm spawns next as a child, so the listener is usually in our process
    # group rather than our pid itself.
    local owner_pgid ours_pgid
    owner_pgid="$(ps -o pgid= -p "$owner" 2>/dev/null | tr -d ' ')"
    ours_pgid="$(ps -o pgid= -p "$ours" 2>/dev/null | tr -d ' ')"
    if [ -n "$owner_pgid" ] && [ "$owner_pgid" = "$ours_pgid" ]; then return 0; fi

    LISTENER_OWNER="$owner"
    return 2
}

# Report a tier's startup from what is actually listening.
report_listener() {
    local name=$1 port=$2 tries=$3 label=$4 logname=$5
    confirm_listener "$name" "$port" "$tries"
    local rc=$? owner
    owner="$(port_owner "$port")"
    case $rc in
        0) ok "$label${owner:+ (pid $owner)}" ;;
        2) fail "Port ${port} is held by pid ${LISTENER_OWNER}, which is not the ${name} we just started."
           info "That process serves its own older code and config. Stop it first:"
           info "  kill ${LISTENER_OWNER} && $0 restart"
           return 2 ;;
        *) warn "${name} has not opened port ${port} yet; see $LOG_DIR/$logname" ; return 3 ;;
    esac
}

load_secrets() {
    if [ ! -f "$SECRETS" ]; then
        umask 077
        cat > "$SECRETS" <<EOF
API_AUTH_SECRET=dev-$(openssl rand -hex 32)
APP_KEY=base64:$(openssl rand -base64 32)
FORGE_MASTER_KEY=$(openssl rand -base64 32)
DAEMON_NODE_TOKEN=devnodetoken0001.$(openssl rand -hex 24)
DAEMON_SFTP_HOST_KEY_PASSPHRASE=dev-$(openssl rand -hex 16)
EOF
        umask 022
    fi
    set -a; . "$SECRETS"; set +a
}

export_env() {
    load_secrets
    export DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@127.0.0.1:${DB_PORT}/${DB_NAME}?sslmode=disable"
    export API_ADDR=":${API_PORT}"
    export APP_ENV=development
    export APP_CIPHER=AES-256-GCM
    # Seeds the demo admin and pairs the demo node with DAEMON_NODE_TOKEN so
    export API_SEED_DEMO=true
    export REDIS_ADDR="127.0.0.1:${REDIS_PORT}"
    export REDIS_PASSWORD="${REDIS_PASSWORD:-gamepanel}"
    export BEACON_BASE_URL="http://127.0.0.1:${BEACON_PORT}"
    export PANEL_URL="http://localhost:${WEB_PORT}"
    export SESSION_COOKIE_SECURE=false
    export FORGE_MASTER_KEY_ID=primary
    export FORGE_ALLOW_EPHEMERAL_MASTER_KEY=false
    export DAEMON_NODE_ID="22222222-2222-2222-2222-222222222222"
    export NEXT_PUBLIC_API_URL="/api/v1"
    export API_INTERNAL_URL="http://127.0.0.1:${API_PORT}"
}

ensure_databases() {
    head_ "PostgreSQL and Redis (native Homebrew services)"

    if ! port_open "$DB_PORT"; then
        brew services start "$PG_FORMULA" >/dev/null
        wait_port "$DB_PORT" 30 || { fail "PostgreSQL did not start on $DB_PORT"; exit 1; }
    fi
    ok "PostgreSQL on 127.0.0.1:$DB_PORT"

    if command -v psql >/dev/null 2>&1; then
        if ! psql -h 127.0.0.1 -p "$DB_PORT" -d postgres -tAc \
            "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" 2>/dev/null | grep -q 1; then
            createuser -h 127.0.0.1 -p "$DB_PORT" -s "$DB_USER" 2>/dev/null || true
            psql -h 127.0.0.1 -p "$DB_PORT" -d postgres -c \
                "ALTER USER ${DB_USER} PASSWORD '${DB_PASS}'" >/dev/null 2>&1 || true
        fi
        if ! psql -h 127.0.0.1 -p "$DB_PORT" -d postgres -tAc \
            "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" 2>/dev/null | grep -q 1; then
            createdb -h 127.0.0.1 -p "$DB_PORT" -O "$DB_USER" "$DB_NAME" 2>/dev/null || true
        fi
        ok "Database '${DB_NAME}' owned by '${DB_USER}'"
    else
        ok "Database ready on port $DB_PORT (container / external service)"
    fi

    if ! port_open "$REDIS_PORT"; then
        brew services start redis >/dev/null 2>&1 || true
        wait_port "$REDIS_PORT" 20 || warn "Redis did not start; API will run without a cache"
    fi
    port_open "$REDIS_PORT" && ok "Redis on 127.0.0.1:$REDIS_PORT"
}

start_api() {
    head_ "Forge API"
    info "Building..."
    (cd "$ROOT/forge/api" && go build -o api ./cmd/api)

    # Migrations resolve relative to the working directory. A first run applies
    # 200+ migrations before the listener opens, so allow several minutes.
    spawn_detached api "$ROOT/forge/api" "$ROOT/forge/api/api"

    info "Waiting for migrations and startup..."
    if report_listener api "$API_PORT" 300 \
        "API on http://localhost:${API_PORT}/api/v1" api.log; then
        return 0
    fi
    # The API is what everything else talks to, so a wrong or missing listener
    # here is fatal rather than a warning.
    tail -20 "$LOG_DIR/api.log" || true
    exit 1
}

start_beacon() {
    head_ "Beacon daemon (launchd)"
    info "Building..."
    (cd "$ROOT/beacon" && go build -o daemon ./cmd/daemon)

    mkdir -p "$HOME/Library/LaunchAgents" "$DATA_DIR/beacon" "$DATA_DIR/beacon-tmp"

    local runtime_env=""
    if [ -S "$DOCKER_SOCK" ]; then
        runtime_env="<key>DOCKER_HOST</key><string>unix://$DOCKER_SOCK</string>"
        info "Docker runtime via $DOCKER_SOCK"
    else
        runtime_env="<key>DAEMON_ALLOW_MOCK_RUNTIME</key><string>true</string>"
        warn "No Docker socket at $DOCKER_SOCK; using mock runtime"
    fi

    cat > "$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$BEACON_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$ROOT/beacon/daemon</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>$HOME/.docker/bin:$BREW_PREFIX/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>APP_ENV</key>
        <string>development</string>
        <key>DAEMON_ADDR</key>
        <string>:$BEACON_PORT</string>
        <key>DAEMON_DATA_DIR</key>
        <string>$DATA_DIR/beacon</string>
        <key>DAEMON_NODE_ID</key>
        <string>$DAEMON_NODE_ID</string>
        <key>DAEMON_NODE_TOKEN</key>
        <string>$DAEMON_NODE_TOKEN</string>
        <key>DAEMON_SFTP_HOST_KEY_PASSPHRASE</key>
        <string>$DAEMON_SFTP_HOST_KEY_PASSPHRASE</string>
        <key>PANEL_API_URL</key>
        <string>http://127.0.0.1:$API_PORT/api/v1</string>
        $runtime_env
    </dict>
    <key>WorkingDirectory</key>
    <string>$DATA_DIR/beacon</string>
    <key>StandardOutPath</key>
    <string>$LOG_DIR/beacon.log</string>
    <key>StandardErrorPath</key>
    <string>$LOG_DIR/beacon.log</string>
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

    launchctl bootout "gui/$UID/$BEACON_LABEL" 2>/dev/null || true
    launchctl bootstrap "gui/$UID" "$PLIST"

    if wait_port "$BEACON_PORT" 30; then
        ok "Beacon on http://localhost:${BEACON_PORT}/health (launchd: $BEACON_LABEL)"
    else
        warn "Beacon has not opened port ${BEACON_PORT} yet; see $LOG_DIR/beacon.log"
    fi
}

start_web() {
    head_ "Forge Web"
    spawn_detached web "$ROOT/forge/web" npm run dev
    report_listener web "$WEB_PORT" 120 \
        "Web on http://localhost:${WEB_PORT}" web.log || true
}

# Stop the process recorded for a tier. Exit codes: 0 signalled, 1 no pidfile,
# 2 pidfile present but the process was already gone.
kill_pidfile() {
    local name=$1 file="$PID_DIR/$1.pid"
    [ -f "$file" ] || return 1
    local pid; pid="$(cat "$file")" rc=2
    if kill -0 "$pid" 2>/dev/null; then
        # npm spawns next as a child; take down the whole process group.
        kill -TERM -"$(ps -o pgid= "$pid" | tr -d ' ')" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
        rc=0
    fi
    rm -f "$file"
    return $rc
}

# Stop a tier and report what actually happened.
#
# The pidfile is a record of what we last started, not of what is running now:
# a dev server can be replaced, re-bind, or outlive the pidfile that named it.
# Killing the recorded pid and printing "stopped" therefore claims work that may
# not have been done — and leaves an orphan serving stale code on the port. The
# port is the ground truth, so it is what gets checked and reported.
stop_tier() {
    local name=$1 port=$2 label=$3 rc owner
    kill_pidfile "$name"; rc=$?

    local i=0
    while [ "$i" -lt 10 ] && port_open "$port"; do sleep 1; i=$((i + 1)); done

    if port_open "$port"; then
        owner="$(port_owner "$port")"
        fail "${label} is still listening on ${port}${owner:+ (pid $owner)} after stop."
        info "It is not the process ${0##*/} recorded, so it was started outside this script."
        [ -n "$owner" ] && info "  kill ${owner}"
        return 1
    fi

    case $rc in
        0) ok "${label} stopped" ;;
        1) info "${label} was not running (no pidfile)" ;;
        2) info "${label} was not running (recorded process already gone)" ;;
    esac
}

cmd_start() {
    mkdir -p "$LOG_DIR" "$PID_DIR" "$DATA_DIR"
    export_env
    ensure_databases
    start_api
    start_beacon
    start_web

    head_ "Ready"
    printf '  %-16s %s\n' "Web dashboard" "http://localhost:${WEB_PORT}"
    printf '  %-16s %s\n' "Forge API" "http://localhost:${API_PORT}/api/v1"
    printf '  %-16s %s\n' "API health" "http://localhost:${API_PORT}/api/v1/health/ready"
    printf '  %-16s %s\n' "Beacon health" "http://localhost:${BEACON_PORT}/health"
    printf '  %-16s %s\n' "PostgreSQL" "127.0.0.1:${DB_PORT}"
    printf '  %-16s %s\n' "Redis" "127.0.0.1:${REDIS_PORT}"
    printf '\n  Demo login: admin@example.com / admin123\n'
    printf '  Logs: %s\n\n' "$LOG_DIR"
}

cmd_stop() {
    head_ "Stopping"
    launchctl bootout "gui/$UID/$BEACON_LABEL" 2>/dev/null && ok "Beacon unloaded" || warn "Beacon was not loaded"
    stop_tier api "$API_PORT" "API" || true
    stop_tier web "$WEB_PORT" "Web" || true
    info "PostgreSQL and Redis are left running (brew services stop ${PG_FORMULA} redis)"
    echo
}

cmd_status() {
    head_ "Status"
    for entry in "PostgreSQL:$DB_PORT" "Redis:$REDIS_PORT" "API:$API_PORT" "Beacon:$BEACON_PORT" "Web:$WEB_PORT"; do
        local name=${entry%%:*} port=${entry##*:} owner
        if port_open "$port"; then
            # The pid is printed because "something is listening" and "the
            # process this script started is listening" are different facts,
            # and only the second one means the port serves current code.
            owner="$(port_owner "$port")"
            ok "$name listening on $port${owner:+ (pid $owner)}"
        else
            fail "$name not listening on $port"
        fi
    done
    echo
    launchctl print "gui/$UID/$BEACON_LABEL" 2>/dev/null \
        | grep -E -o 'state = [a-z]+|pid = [0-9]+' | sed 's/^/  beacon /' || info "beacon: not loaded in launchd"
    echo
}

cmd_logs() { tail -f "$LOG_DIR"/*.log; }

case "${1:-start}" in
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    restart) cmd_stop; cmd_start ;;
    status)  cmd_status ;;
    logs)    cmd_logs ;;
    *) echo "usage: $0 {start|stop|restart|status|logs}" >&2; exit 1 ;;
esac
