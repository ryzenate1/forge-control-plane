#!/bin/bash

# Forge Control Plane - Single-command dev environment launcher
# Usage:
#   ./start-dev.sh                # Start with Docker PostgreSQL + Redis (default)
#   ./start-dev.sh --native       # Start with locally installed PostgreSQL + Redis
#   ./start-dev.sh --stop         # Stop everything

set -e

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse arguments
STOP=false
NATIVE=false

for arg in "$@"; do
    case "$arg" in
        --stop) STOP=true ;;
        --native) NATIVE=true ;;
    esac
done

# Configuration
DB_USER="gamepanel"
DB_PASS="gamepanel"
DB_NAME="gamepanel"
DB_PORT=5432
REDIS_PORT=6379
API_PORT=8080
FRONTEND_PORT=3000
BEACON_PORT=9090

# Environment variables
export DATABASE_URL="postgres://${DB_USER}:${DB_PASS}@localhost:${DB_PORT}/${DB_NAME}?sslmode=disable"
export API_ADDR=":${API_PORT}"
export API_AUTH_SECRET="dev-api-secret"
export APP_ENV="development"
export DAEMON_NODE_TOKEN="dev-node-token"
export DAEMON_NODE_ID="1"
export API_SEED_DEMO="false"
export REDIS_ADDR="localhost:${REDIS_PORT}"
export MIGRATIONS_DIR="${ROOT}/forge/api/migrations"
export NEXT_PUBLIC_API_URL="http://localhost:${API_PORT}/api/v1"
export FORGE_ALLOW_EPHEMERAL_MASTER_KEY="true"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Helper functions
write_status() {
    echo -e "  $1 ${NC}$2"
}

write_header() {
    echo -e "\n${CYAN}=== $1 ===${NC}"
}

test_port() {
    local port=$1
    local max_retries=${2:-20}
    local i=0

    while [ $i -lt $max_retries ]; do
        if nc -z 127.0.0.1 $port >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        echo -n "."
        i=$((i+1))
    done
    return 1
}

stop_process_by_port() {
    local port=$1
    local pids=$(lsof -ti :$port 2>/dev/null || echo "")

    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 $pid 2>/dev/null || true
        done
    fi
}

# Stop mode
if [ "$STOP" = true ]; then
    write_header "Stopping Forge Dev Environment"

    stop_process_by_port $API_PORT
    write_status "[x]" "API stopped" "$YELLOW"

    stop_process_by_port $FRONTEND_PORT
    write_status "[x]" "Frontend stopped" "$YELLOW"

    stop_process_by_port $BEACON_PORT
    write_status "[x]" "Beacon daemon stopped" "$YELLOW"

    if [ "$NATIVE" = false ]; then
        (cd "$ROOT/infra" && docker compose down) >/dev/null 2>&1 || true
        write_status "[x]" "Docker services stopped" "$YELLOW"
    fi

    echo -e "\n  All services stopped.\n"
    exit 0
fi

# Banner
echo ""
echo -e "  ${RED}+==========================================+${NC}"
echo -e "  ${RED}|       Forge Control Plane Dev          |${NC}"
echo -e "  ${RED}+==========================================+${NC}"
if [ "$NATIVE" = true ]; then
    echo -e "  ${YELLOW}|       Mode: NATIVE (no Docker)           |${NC}"
else
    echo -e "  |       Mode: DOCKER (default)             |"
fi
echo ""

# Step 1: Database + Redis
write_header "Starting PostgreSQL and Redis"

if [ "$NATIVE" = true ]; then
    # Native mode: expect PostgreSQL and Redis already installed and running
    echo -n "  Checking local PostgreSQL..."
    if test_port $DB_PORT 5; then
        echo -e " ${GREEN}Found!${NC}"
        write_status "[ok]" "PostgreSQL on port $DB_PORT (native)" "$GREEN"
    else
        echo -e " ${RED}NOT RUNNING${NC}"
        echo ""
        echo -e "  ${RED}PostgreSQL is not running on port $DB_PORT.${NC}"
        echo -e "  ${YELLOW}Install it natively:${NC}"
        echo "    Windows:  https://www.postgresql.org/download/windows/"
        echo "    Linux:    sudo apt install postgresql-16"
        echo ""
        echo -e "  ${YELLOW}Then create the database:${NC}"
        echo "    sudo -u postgres createuser -s gamepanel"
        echo "    sudo -u postgres createdb -O gamepanel gamepanel"
        echo "    sudo -u postgres psql -c \"ALTER USER gamepanel PASSWORD 'gamepanel';\""
        exit 1
    fi

    echo -n "  Checking local Redis..."
    if test_port $REDIS_PORT 5; then
        echo -e " ${GREEN}Found!${NC}"
        write_status "[ok]" "Redis on port $REDIS_PORT (native)" "$GREEN"
    else
        echo -e " ${YELLOW}NOT RUNNING (optional, continuing without)${NC}"
        export REDIS_ADDR=""
    fi
else
    # Docker mode
    (cd "$ROOT/infra" && \
        # Ensure .env file exists with required variables
        if [ ! -f ".env" ]; then
            echo "  Creating infra/.env file..."
            cat > .env << EOF
POSTGRES_PASSWORD=gamepanel
API_AUTH_SECRET=dev-api-secret
DAEMON_NODE_TOKEN=dev-node-token
DAEMON_NODE_ID=1
GRAFANA_ADMIN_PASSWORD=admin
DATABASE_URL=postgres://gamepanel:gamepanel@postgres:5432/gamepanel?sslmode=disable
EOF
        fi

        docker compose up -d postgres redis >/dev/null 2>&1)

    echo -n "  Waiting for PostgreSQL..."
    if test_port $DB_PORT 30; then
        echo -e " ${GREEN}Ready!${NC}"
        write_status "[ok]" "PostgreSQL on port $DB_PORT (Docker)" "$GREEN"
    else
        echo -e " ${RED}FAILED${NC}"
        exit 1
    fi

    echo -n "  Waiting for Redis..."
    if test_port $REDIS_PORT 15; then
        echo -e " ${GREEN}Ready!${NC}"
        write_status "[ok]" "Redis on port $REDIS_PORT (Docker)" "$GREEN"
    else
        echo -e " ${YELLOW}TIMEOUT${NC}"
    fi
fi

# Step 2: Go API
write_header "Starting Go API"

echo "  Building API..."
(cd "$ROOT/forge/api" && \
    if [ ! -f "go.mod" ]; then
        echo -e "  ${RED}API go.mod not found!${NC}"
        exit 1
    fi

    go build -o "$ROOT/forge/api/api" ./cmd/api >/dev/null 2>&1)

if [ $? -ne 0 ]; then
    echo -e "  ${RED}API build failed!${NC}"
    exit 1
fi

# Start API in background
"$ROOT/forge/api/api" > "$ROOT/api-dev.log" 2> "$ROOT/api-dev.err.log" &
API_PID=$!

echo -n "  Waiting for API..."
if test_port $API_PORT 20; then
    echo -e " ${GREEN}Ready!${NC}"
    write_status "[ok]" "API on http://localhost:${API_PORT} (PID: $API_PID)" "$GREEN"
else
    echo -e " ${RED}FAILED${NC}"
    echo -e "  ${RED}Check: $ROOT/api-dev.err.log${NC}"
    tail -5 "$ROOT/api-dev.err.log" 2>/dev/null || true
    kill $API_PID 2>/dev/null || true
    exit 1
fi

# Step 3: Next.js Frontend
write_header "Starting Next.js Frontend"

# Check if package.json exists
if [ ! -f "$ROOT/forge/web/package.json" ]; then
    echo -e "  ${RED}Frontend package.json not found!${NC}"
    exit 1
fi

# Start frontend in background
(cd "$ROOT/forge/web" && npm run dev > "$ROOT/frontend-dev.log" 2> "$ROOT/frontend-dev.err.log" &)
FE_PID=$!

echo -n "  Waiting for Frontend..."
if test_port $FRONTEND_PORT 30; then
    echo -e " ${GREEN}Ready!${NC}"
else
    echo -e " ${YELLOW}(still compiling)${NC}"
fi
write_status "[ok]" "Frontend on http://localhost:${FRONTEND_PORT} (PID: $FE_PID)" "$GREEN"

# Step 4: Beacon Daemon
write_header "Starting Beacon Daemon"

echo "  Building Beacon..."
(cd "$ROOT/beacon" && \
    if [ ! -f "go.mod" ]; then
        echo -e "  ${RED}Beacon go.mod not found!${NC}"
        exit 1
    fi

    go build -o "$ROOT/beacon/daemon" ./cmd/daemon >/dev/null 2>&1)

if [ $? -ne 0 ]; then
    echo -e "  ${RED}Beacon build failed!${NC}"
    exit 1
fi

# Start beacon in background with required env vars
(cd "$ROOT/beacon" && \
    DAEMON_ALLOW_INSECURE_NO_AUTH=true \
    DAEMON_DATA_DIR=/tmp/beacon-data \
    PANEL_API_URL=http://localhost:8080/api/v1 \
    ./daemon > "$ROOT/beacon-dev.log" 2> "$ROOT/beacon-dev.err.log" &)
BEACON_PID=$!

echo -n "  Waiting for Beacon..."
if test_port $BEACON_PORT 15; then
    echo -e " ${GREEN}Ready!${NC}"
    write_status "[ok]" "Beacon on http://localhost:${BEACON_PORT}/health (PID: $BEACON_PID)" "$GREEN"
else
    echo -e " ${YELLOW}(daemon may take longer to start)${NC}"
fi

# Summary
echo ""
echo -e "  ${GREEN}+==========================================+${NC}"
echo -e "  ${GREEN}|       All Services Running!              |${NC}"
echo -e "  ${GREEN}+==========================================+${NC}"
echo ""
echo "  Service          URL"
echo "  ---------------  ----------------------------"
write_status "FE " "Frontend        http://localhost:${FRONTEND_PORT}" "$CYAN"
write_status "API" "API             http://localhost:${API_PORT}/api/v1" "$CYAN"
write_status "BKN" "Beacon          http://localhost:${BEACON_PORT}/health" "$CYAN"
write_status "DB " "PostgreSQL      localhost:${DB_PORT}" "$CYAN"
write_status "RDS" "Redis           localhost:${REDIS_PORT}" "$CYAN"
echo ""
echo -e "  Setup: open http://localhost:${FRONTEND_PORT}/setup on first run"
echo ""
echo "  Logs:"
echo "    API:      tail -f $ROOT/api-dev.log"
echo "    Frontend: tail -f $ROOT/frontend-dev.log"
echo "    Beacon:   tail -f $ROOT/beacon-dev.log"
echo ""
echo -e "  Stop:  $0 --stop"
echo ""