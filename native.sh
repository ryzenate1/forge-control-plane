#!/bin/bash
# Forge Plane - native macOS launcher.
#
# PostgreSQL and Redis run as Homebrew services, Beacon runs as a launchd
# user agent, and the API and web dashboard run as background processes.
#
#   ./native.sh start | stop | restart | status | logs

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! command -v brew >/dev/null 2>&1; then
    echo "Homebrew is required for native mode (PostgreSQL/Redis via brew services)." >&2
    echo "Install from https://brew.sh then re-run: $0 start" >&2
    exit 1
fi
BREW_PREFIX="$(brew --prefix)"
export PATH="$BREW_PREFIX/bin:$BREW_PREFIX/opt/postgresql@16/bin:$PATH"

DB_USER=gamepanel
DB_PASS=gamepanel
DB_NAME=gamepanel
# Canonical ports live in scripts/dev/ports.env; source it when present so the
# two launchers cannot drift. WEB_PORT/BEACON_PORT are this script's legacy
# names for FRONTEND_PORT/DAEMON_PORT.
# shellcheck disable=SC1091
[ -f "$ROOT/scripts/dev/ports.env" ] && . "$ROOT/scripts/dev/ports.env"
DB_PORT="${DB_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
API_PORT="${API_PORT:-8080}"
WEB_PORT="${FRONTEND_PORT:-${WEB_PORT:-3000}}"
BEACON_PORT="${DAEMON_PORT:-${BEACON_PORT:-9090}}"

# Set by any tier that did not come up. The start continues — one dead tier is
# not a reason to withhold the others — but the banner and the exit code must
# not describe a stack that is only partly running as ready.
STACK_DEGRADED=0

PG_FORMULA=postgresql@16
BEACON_LABEL=com.gamepanel.beacon
PLIST="$HOME/Library/LaunchAgents/$BEACON_LABEL.plist"

LOG_DIR="$ROOT/.dev-logs"
PID_DIR="$ROOT/.dev-pids"
DATA_DIR="$ROOT/.dev-data"
SECRETS="$ROOT/.dev-secrets.env"

if [ -n "${DOCKER_HOST:-}" ]; then
    # Respect an explicit DOCKER_HOST (e.g. Colima, remote daemon). Only
    # unix:// sockets map to a filesystem path we can test with -S.
    case "$DOCKER_HOST" in
        unix://*)
            DOCKER_SOCK="${DOCKER_HOST#unix://}"
            ;;
        *)
            # TCP/named-pipe endpoints have no socket file; pass through.
            DOCKER_SOCK="$DOCKER_HOST"
            ;;
    esac
elif [ -S "$HOME/.docker/run/docker.sock" ]; then
    DOCKER_SOCK="$HOME/.docker/run/docker.sock"
elif [ -S "$HOME/.colima/default/docker.sock" ]; then
    DOCKER_SOCK="$HOME/.colima/default/docker.sock"
elif [ -S "/var/run/docker.sock" ]; then
    DOCKER_SOCK="/var/run/docker.sock"
else
    # Fail closed: no socket means container workloads cannot run. Do NOT
    # silently fall back to a mock runtime — the operator must opt in via
    # DAEMON_ALLOW_MOCK_RUNTIME=true, and the degraded banner below must show.
    DOCKER_SOCK=""
fi
HAVE_DOCKER_SOCK=0
if [ -n "${DOCKER_SOCK:-}" ]; then
    case "$DOCKER_SOCK" in
        tcp://*|ssh://*|npipe://*) HAVE_DOCKER_SOCK=1 ;;
        *) [ -S "$DOCKER_SOCK" ] && HAVE_DOCKER_SOCK=1 ;;
    esac
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

# Refuse to start a tier whose port is already taken, *before* spawning it.
#
# report_listener already detects a foreign listener and says the right thing —
# but it runs after spawn_detached, and for the web tier that is too late to
# matter. `next dev` clears the manifests in .next as it boots and only then
# discovers the port is taken and exits, so a doomed second start leaves the
# server that is still serving :3000 with a gutted .next and every route
# answering 500 with `ENOENT routes-manifest.json`. A healthy process's build
# directory must not be destroyed by one that could never have started.
#
# If anything at all is listening, the spawn is doomed regardless of who owns
# the port, so refusing costs nothing that could have worked. That is why an
# unidentifiable listener refuses here rather than being waved through the way
# confirm_listener waves it through: there, a missing lsof reading must not
# invent a conflict; here, the conflict is already proven by the open port and
# only the owner's name is unknown.
#
# Exit codes: 0 free to start, 1 already running as ours, 2 held by another.
guard_port() {
    local name=$1 port=$2
    # Always reset: a stale value from an earlier tier would otherwise be
    # reported as this tier's port holder.
    LISTENER_OWNER=""
    port_open "$port" || return 0

    local owner ours owner_pgid ours_pgid
    owner="$(port_owner "$port")"
    ours="$(cat "$PID_DIR/$name.pid" 2>/dev/null || true)"
    LISTENER_OWNER="$owner"

    if [ -n "$owner" ] && [ -n "$ours" ]; then
        if [ "$owner" = "$ours" ]; then return 1; fi
        # npm spawns next as a child, so our listener is usually in our process
        # group rather than our pid itself.
        owner_pgid="$(ps -o pgid= -p "$owner" 2>/dev/null | tr -d ' ')"
        ours_pgid="$(ps -o pgid= -p "$ours" 2>/dev/null | tr -d ' ')"
        if [ -n "$owner_pgid" ] && [ "$owner_pgid" = "$ours_pgid" ]; then return 1; fi
    fi

    return 2
}

# Guard a tier's port and report why it is not being started. Returns non-zero
# when the caller must skip the spawn.
require_free_port() {
    local name=$1 port=$2
    guard_port "$name" "$port"
    local rc=$? who
    # lsof can decline to name the holder; say so rather than printing an empty
    # pid or claiming a pid we do not have.
    if [ -n "$LISTENER_OWNER" ]; then who="pid $LISTENER_OWNER"; else who="an unidentified process"; fi
    case $rc in
        0) return 0 ;;
        1) ok "${name} is already running on port ${port} (${who})"
           info "Use '$0 restart' to pick up code or config changes."
           return 1 ;;
        *) fail "Port ${port} is held by ${who}, which is not a ${name} this script started."
           info "Not starting a second ${name}: it would lose the bind, and a doomed"
           info "'next dev' wipes .next out from under the process still serving the port."
           info "Stop the holder first:"
           info "  kill ${LISTENER_OWNER:-<pid>} && $0 start"
           return 1 ;;
    esac
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
    # Restricted parser: only bare KEY=VALUE lines are honoured. No `source`,
    # no command/export expansion, no quoting games — a malicious or broken
    # secrets file cannot execute code here. Invalid lines are ignored loudly.
    while IFS= read -r line || [ -n "$line" ]; do
        line="${line%$'\r'}"
        case "$line" in ''|'#'*) continue ;; esac
        case "$line" in
            *=*) ;;
            *) warn "Ignoring malformed line in $SECRETS: $line"; continue ;;
        esac
        key="${line%%=*}"
        value="${line#*=}"
        case "$key" in
            ''|*[!A-Za-z0-9_]*|[0-9]*) warn "Ignoring bad key in $SECRETS: $key"; continue ;;
        esac
        case "$key" in
            API_AUTH_SECRET|APP_KEY|FORGE_MASTER_KEY|DAEMON_NODE_TOKEN|DAEMON_SFTP_HOST_KEY_PASSPHRASE|REDIS_PASSWORD) ;;
            *) warn "Ignoring unknown key in $SECRETS: $key"; continue ;;
        esac
        # Strip one layer of matching surrounding quotes if present.
        case "$value" in
            '"'*'"') value="${value#\"}"; value="${value%\"}" ;;
            "'"*"'") value="${value#\'}"; value="${value%\'}";;
        esac
        printf -v "$key" '%s' "$value"
        export "$key"
    done < "$SECRETS"
}

export_env() {
    load_secrets
    export DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@127.0.0.1:${DB_PORT}/${DB_NAME}?sslmode=disable"
    export API_ADDR=":${API_PORT}"
    export APP_ENV=development
    export APP_CIPHER=AES-256-GCM
    # Demo seeding is disabled: first boot goes through /setup to create a
    # real admin, and every node/server is user-created (no Ubuntu Demo Node).
    export API_SEED_DEMO=false
    export REDIS_ADDR="127.0.0.1:${REDIS_PORT}"
    # Only export a Redis password when one is actually configured (local
    # Homebrew Redis ships with no password; sending a default "gamepanel"
    # breaks AUTH against it). Compose stacks that require auth set
    # REDIS_PASSWORD via .env/secrets.
    if [ -n "${REDIS_PASSWORD:-}" ]; then
        export REDIS_PASSWORD
    else
        unset REDIS_PASSWORD || true
    fi
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
        # Unguarded, a brew failure aborted the script here under `set -e` and
        # the diagnostic below never printed. Postgres really is fatal, so the
        # exit stays — but it should say why.
        brew services start "$PG_FORMULA" >/dev/null || true
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
        wait_port "$REDIS_PORT" 20 || true
    fi
    # This was `port_open "$REDIS_PORT" && ok "..."` as the function's last
    # command, which made a missing Redis the function's exit status: under
    # `set -e` cmd_start then died here, silently, before start_api — so the
    # line right above deliberately downgrading Redis to a warning could never
    # take effect, and an optional cache took the entire stack with it.
    if port_open "$REDIS_PORT"; then
        ok "Redis on 127.0.0.1:$REDIS_PORT"
    else
        warn "Redis is not running; the API will run without a cache."
        STACK_DEGRADED=1
    fi
}

start_api() {
    head_ "Forge API"
    require_free_port api "$API_PORT" || return 0
    info "Building..."
    # A bare subshell here aborted the script with no message under `set -e`,
    # which looks identical to a clean exit. Say what failed. Still fatal: the
    # alternative is spawning the previous build and calling it this one.
    if ! (cd "$ROOT/forge/api" && go build -o api ./cmd/api); then
        fail "API build failed; refusing to start the previously built binary."
        exit 1
    fi

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
    if ! (cd "$ROOT/beacon" && go build -o daemon ./cmd/daemon); then
        fail "Beacon build failed; refusing to start the previously built binary."
        STACK_DEGRADED=1
        return 0
    fi
    # launchd kills a bare `go build` binary with OS_REASON_CODESIGNING (it is
    # linker-signed as Identifier=a.out), so ad-hoc re-sign it with a stable
    # identifier before the agent tries to spawn it.
    if [ "$(uname -s)" = "Darwin" ]; then
        codesign --force --sign - --identifier com.gamepanel.beacon "$ROOT/beacon/daemon" >/dev/null 2>&1 \
            || warn "codesign of beacon/daemon failed - launchd may refuse to start Beacon"
    fi

    mkdir -p "$HOME/Library/LaunchAgents" "$DATA_DIR/beacon" "$DATA_DIR/beacon-tmp"

    local runtime_env=""
    if [ "${HAVE_DOCKER_SOCK:-0}" -eq 1 ]; then
        case "$DOCKER_SOCK" in
            unix://*|tcp://*|ssh://*|npipe://*)
                runtime_env="<key>DOCKER_HOST</key><string>$DOCKER_SOCK</string>" ;;
            *)
                runtime_env="<key>DOCKER_HOST</key><string>unix://$DOCKER_SOCK</string>" ;;
        esac
        info "Docker runtime via $DOCKER_SOCK"
    elif [ "${DAEMON_ALLOW_MOCK_RUNTIME:-false}" = "true" ]; then
        runtime_env="<key>DAEMON_ALLOW_MOCK_RUNTIME</key><string>true</string>"
        STACK_DEGRADED=1
        warn "No Docker socket found; mock runtime explicitly enabled (DAEMON_ALLOW_MOCK_RUNTIME=true)"
        warn "Container workloads will NOT run — STACK DEGRADED, dev-only."
    else
        STACK_DEGRADED=1
        fail "No Docker socket found and DAEMON_ALLOW_MOCK_RUNTIME is not 'true'."
        info "Start Colima/Docker Desktop, or export DAEMON_ALLOW_MOCK_RUNTIME=true"
        info "to run explicitly degraded (no containers). Continuing without beacon."
        return 0
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

    # bootout is not synchronous. launchd can still be tearing the old agent
    # down when bootstrap runs, and bootstrap then fails with
    # "5: Input/output error" because the label is still present in the domain.
    # Wait for the label to actually disappear before loading the new plist.
    launchctl bootout "gui/$UID/$BEACON_LABEL" 2>/dev/null || true
    local i=0
    while [ "$i" -lt 10 ] && launchctl print "gui/$UID/$BEACON_LABEL" >/dev/null 2>&1; do
        sleep 1; i=$((i + 1))
    done

    # A beacon that will not load must not abort the rest of the stack. This
    # bootstrap was unguarded under `set -e`, so a single EIO from launchctl
    # killed cmd_start before start_web ever ran: one unloadable agent took the
    # whole dashboard down with it, and the API and UI paths do not need beacon
    # to be worth serving. Failures are reported and the start continues.
    local boot_err
    if boot_err="$(launchctl bootstrap "gui/$UID" "$PLIST" 2>&1)"; then
        if wait_port "$BEACON_PORT" 30; then
            ok "Beacon on http://localhost:${BEACON_PORT}/health (launchd: $BEACON_LABEL)"
        else
            warn "Beacon has not opened port ${BEACON_PORT} yet; see $LOG_DIR/beacon.log"
            STACK_DEGRADED=1
        fi
        return 0
    fi

    STACK_DEGRADED=1
    if port_open "$BEACON_PORT"; then
        # Something is still serving the port. That is not this load succeeding:
        # it is the previous agent running the binary it loaded at its own start,
        # so a rebuild is not live and saying "ok" here would be a lie.
        warn "Beacon could not be reloaded: ${boot_err:-launchctl bootstrap failed}"
        warn "The previous agent is still serving port ${BEACON_PORT} with the code it started with."
        info "To pick up the rebuilt binary:"
        info "  launchctl bootout gui/$UID/$BEACON_LABEL && $0 start"
    else
        fail "Beacon did not load: ${boot_err:-launchctl bootstrap failed}"
        info "Continuing without beacon — the API and dashboard still work, but no"
        info "workload can be started or inspected on this host."
        info "  see $LOG_DIR/beacon.log"
    fi
    return 0
}

start_web() {
    head_ "Forge Web"
    # Checked before the spawn, not after: a doomed `next dev` wipes .next on
    # its way out and takes the running server's manifests with it.
    require_free_port web "$WEB_PORT" || return 0
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
    # Reset per-run: `restart` runs cmd_stop then cmd_start in one process, so
    # a degraded flag from an earlier start must not leak into this one.
    STACK_DEGRADED=0
    mkdir -p "$LOG_DIR" "$PID_DIR" "$DATA_DIR"
    export_env
    ensure_databases
    start_api
    start_beacon
    start_web

    # The banner is a claim about what is serving, so it is built from what is
    # actually listening. Printing a flat "Ready" with a Beacon health URL under
    # it while beacon failed to load tells the operator to go look at something
    # that is not there.
    if [ "$STACK_DEGRADED" -ne 0 ]; then
        head_ "Started with problems"
    else
        head_ "Ready"
    fi
    local tier
    for tier in "Web dashboard:$WEB_PORT:http://localhost:${WEB_PORT}" \
                "Forge API:$API_PORT:http://localhost:${API_PORT}/api/v1" \
                "API health:$API_PORT:http://localhost:${API_PORT}/api/v1/health/ready" \
                "Beacon health:$BEACON_PORT:http://localhost:${BEACON_PORT}/health" \
                "PostgreSQL:$DB_PORT:127.0.0.1:${DB_PORT}" \
                "Redis:$REDIS_PORT:127.0.0.1:${REDIS_PORT}"; do
        local label=${tier%%:*} rest=${tier#*:}
        local port=${rest%%:*} value=${rest#*:}
        if port_open "$port"; then
            printf '  %-16s %s\n' "$label" "$value"
        else
            printf '  %-16s %s (not listening)\n' "$label" "$value"
        fi
    done
    printf '\n  Demo login: admin@example.com (dev seed only — use /setup credentials in production)\n'
    printf '  Logs: %s\n\n' "$LOG_DIR"
    [ "$STACK_DEGRADED" -eq 0 ]
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
