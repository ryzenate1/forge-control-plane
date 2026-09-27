#!/usr/bin/env bash
# GamePanel Diagnostic Script
# Checks all services and reports status.
#
# Credentials and ports are parameterized (env > .dev-data/ports.env >
# defaults) — never hardcode container names or passwords here. Postgres and
# Redis containers are discovered via `docker compose ps` in infra/.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
[ -f "$ROOT/scripts/dev/ports.env" ] && . "$ROOT/scripts/dev/ports.env"
[ -f "$ROOT/.dev-data/ports.env" ] && . "$ROOT/.dev-data/ports.env"

API_PORT="${API_PORT:-8080}"
DAEMON_PORT="${DAEMON_PORT:-9090}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"
DB_PORT="${DB_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"

DB_USER="${DB_USER:-gamepanel}"
DB_NAME="${DB_NAME:-gamepanel}"
DB_PASS="${DB_PASS:-${POSTGRES_PASSWORD:-gamepanel}}"
DIAG_EMAIL="${DIAG_EMAIL:-admin@example.com}"
DIAG_PASSWORD="${DIAG_PASSWORD:-admin123}"
export PGPASSWORD="$DB_PASS"

echo "========================================"
echo "GamePanel System Diagnostic"
echo "========================================"
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check API
echo -n "API (port $API_PORT): "
if curl -s "http://localhost:${API_PORT}/api/v1/health/ready" > /dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
else
    printf "${RED}✗ Not responding${NC}\n"
fi

# Check Daemon
echo -n "Daemon (port $DAEMON_PORT): "
if curl -s "http://localhost:${DAEMON_PORT}/health" > /dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
else
    printf "${RED}✗ Not responding${NC}\n"
fi

# Check Frontend
echo -n "Frontend (port $FRONTEND_PORT): "
if curl -s "http://localhost:${FRONTEND_PORT}" > /dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
else
    printf "${RED}✗ Not responding${NC}\n"
fi

# Discover compose containers (no hardcoded docker-postgres-1 names).
POSTGRES_CID=""
REDIS_CID=""
if command -v docker >/dev/null 2>&1 && [ -f "$ROOT/infra/compose.yml" ]; then
    POSTGRES_CID="$(cd "$ROOT/infra" && docker compose ps -q postgres 2>/dev/null | head -1)"
    REDIS_CID="$(cd "$ROOT/infra" && docker compose ps -q redis 2>/dev/null | head -1)"
fi
# Fallback: any container publishing the expected port.
if [ -z "$POSTGRES_CID" ] && command -v docker >/dev/null 2>&1; then
    POSTGRES_CID="$(docker ps --filter "publish=${DB_PORT}" --format "{{.ID}}" 2>/dev/null | head -1)"
fi
if [ -z "$REDIS_CID" ] && command -v docker >/dev/null 2>&1; then
    REDIS_CID="$(docker ps --filter "publish=${REDIS_PORT}" --format "{{.ID}}" 2>/dev/null | head -1)"
fi

# Check PostgreSQL
echo -n "PostgreSQL: "
if [ -n "$POSTGRES_CID" ] && docker exec "$POSTGRES_CID" pg_isready -U "$DB_USER" > /dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
elif command -v pg_isready >/dev/null 2>&1 && pg_isready -h 127.0.0.1 -p "$DB_PORT" -U "$DB_USER" >/dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
else
    printf "${RED}✗ Not running${NC}\n"
fi

# Check Redis
echo -n "Redis: "
if [ -n "$REDIS_CID" ] && docker exec "$REDIS_CID" redis-cli ping > /dev/null 2>&1; then
    printf "${GREEN}✓ Running${NC}\n"
elif command -v redis-cli >/dev/null 2>&1 && redis-cli -h 127.0.0.1 -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG; then
    printf "${GREEN}✓ Running${NC}\n"
else
    printf "${RED}✗ Not running${NC}\n"
fi

echo ""
echo "----------------------------------------"
echo "Testing API Authentication"
echo "----------------------------------------"

psql_query() {
    local sql="$1"
    if [ -n "$POSTGRES_CID" ]; then
        docker exec -e PGPASSWORD="$DB_PASS" "$POSTGRES_CID" psql -U "$DB_USER" -d "$DB_NAME" -t -c "$sql" 2>/dev/null | tr -d ' '
    else
        psql -h 127.0.0.1 -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "$sql" 2>/dev/null | tr -d ' '
    fi
}

# Test login (credentials from DIAG_EMAIL/DIAG_PASSWORD, never hardcoded in logic)
echo -n "Login endpoint: "
LOGIN_RESPONSE=$(curl -s -X POST "http://localhost:${API_PORT}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${DIAG_EMAIL}\",\"password\":\"${DIAG_PASSWORD}\"}")

if echo "$LOGIN_RESPONSE" | grep -q "token"; then
    printf "${GREEN}✓ Working${NC}\n"
    TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
    echo "   Token: ${TOKEN:0:50}..."

    # Test authenticated endpoint
    echo -n "Authenticated request: "
    ME_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" "http://localhost:${API_PORT}/api/v1/auth/me")
    if echo "$ME_RESPONSE" | grep -q "$DIAG_EMAIL"; then
        printf "${GREEN}✓ Working${NC}\n"
    else
        printf "${RED}✗ Failed${NC}\n"
        echo "   Response: $ME_RESPONSE"
    fi
else
    printf "${RED}✗ Failed${NC}\n"
    echo "   Response: $LOGIN_RESPONSE"
fi

echo ""
echo "----------------------------------------"
echo "Database Status"
echo "----------------------------------------"

# Count migrations
MIGRATIONS=$(psql_query "SELECT COUNT(*) FROM schema_migrations" || true)
if [ -n "$MIGRATIONS" ]; then
    echo "Applied migrations: ${GREEN}$MIGRATIONS${NC}"
else
    printf "Migrations: ${RED}Unable to query${NC}\n"
fi

# Count servers
SERVERS=$(psql_query "SELECT COUNT(*) FROM servers" || true)
if [ -n "$SERVERS" ]; then
    echo "Servers in database: ${GREEN}$SERVERS${NC}"
fi

# Count nodes
NODES=$(psql_query "SELECT COUNT(*) FROM nodes" || true)
if [ -n "$NODES" ]; then
    echo "Nodes in database: ${GREEN}$NODES${NC}"
fi

# Count allocations
ALLOCATIONS=$(psql_query "SELECT COUNT(*) FROM allocations" || true)
if [ -n "$ALLOCATIONS" ]; then
    echo "Allocations in database: ${GREEN}$ALLOCATIONS${NC}"
fi

echo ""
echo "----------------------------------------"
echo "Recent Logs"
echo "----------------------------------------"

echo ""
echo "Last 5 API log entries:"
tail -5 "$ROOT/.dev-logs/api.log" 2>/dev/null || echo "No API logs found"

echo ""
echo "Last 5 API errors:"
tail -5 "$ROOT/.dev-logs/api.err.log" 2>/dev/null || echo "No API errors"

echo ""
echo "Last 5 Daemon errors:"
tail -5 "$ROOT/.dev-logs/daemon.err.log" 2>/dev/null || echo "No Daemon errors"
tail -5 "$ROOT/.dev-logs/beacon.log" 2>/dev/null || true

echo ""
echo "========================================"
echo "Diagnostic Complete"
echo "========================================"
echo ""
echo "Next steps:"
echo "1. If all services are green, open http://localhost:${FRONTEND_PORT}"
echo "2. Open browser DevTools (F12) → Console tab"
echo "3. Login with ${DIAG_EMAIL}"
echo "4. Check for errors in browser console"
echo "5. Report any red errors you see"
echo ""
