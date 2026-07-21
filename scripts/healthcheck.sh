#!/usr/bin/env bash
# ============================================================
# GamePanel - Comprehensive Health Check
# ============================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; overall_status=1; }
warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }
header(){ echo -e "\n${CYAN}--- $1 ---${NC}"; }

overall_status=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INFRA_DIR="$(cd "$SCRIPT_DIR/../infra" && pwd)"

COMPOSE_CMD=(docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env")

echo ""
echo "  GamePanel Health Check"
echo "  $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# ---- Section 1: Container Status ----
header "Container Status"

if ! docker compose version >/dev/null 2>&1; then
  fail "Docker Compose v2 not available"
fi

all_services=("postgres" "redis" "api" "web" "daemon" "prometheus" "alertmanager" "grafana")

for svc in "${all_services[@]}"; do
  local_status=$("${COMPOSE_CMD[@]}" ps --format json "$svc" 2>/dev/null || true)
  if [ -z "$local_status" ]; then
    fail "$svc: not found in compose"
    continue
  fi

  local_state=$(echo "$local_status" | grep -o '"Status":"[^"]*"' | head -1 || echo "")
  local_name=$(echo "$local_status" | grep -o '"Name":"[^"]*"' | head -1 || echo "")

  if echo "$local_state" | grep -qi "healthy"; then
    pass "${svc}: healthy"
  elif echo "$local_state" | grep -qi "running"; then
    warn "${svc}: running (not yet healthy)"
  elif echo "$local_state" | grep -qi "exited"; then
    fail "${svc}: exited"
  else
    fail "${svc}: $local_state"
  fi
done

# ---- Section 2: API Health Endpoint ----
header "API Health Endpoint"

API_URL="${API_URL:-http://127.0.0.1:8080}"

api_health=$(curl -sf "$API_URL/api/v1/health" 2>/dev/null || true)
if [ -n "$api_health" ]; then
  if echo "$api_health" | grep -qi '"ok"\s*:\s*true'; then
    pass "API /api/v1/health: ok"
  else
    warn "API /api/v1/health responded: $api_health"
  fi
else
  fail "API /api/v1/health: unreachable"
fi

# ---- Section 3: Daemon Health Endpoint ----
header "Daemon Health Endpoint"

DAEMON_URL="${DAEMON_URL:-http://127.0.0.1:9090}"

daemon_health=$(curl -sf "$DAEMON_URL/health" 2>/dev/null || true)
if [ -n "$daemon_health" ]; then
  if echo "$daemon_health" | grep -qi '"ok"\s*:\s*true'; then
    pass "Daemon /health: ok"
  else
    warn "Daemon /health responded: $daemon_health"
  fi
else
  fail "Daemon /health: unreachable"
fi

# ---- Section 4: Web Frontend ----
header "Web Frontend"

WEB_URL="${WEB_URL:-http://127.0.0.1:3000}"

web_status=$(curl -s -o /dev/null -w "%{http_code}" "$WEB_URL" 2>/dev/null || true)
if [ "$web_status" = "200" ] || [ "$web_status" = "301" ] || [ "$web_status" = "302" ]; then
  pass "Web frontend responds with HTTP $web_status"
else
  fail "Web frontend: HTTP $web_status"
fi

# ---- Section 5: Database Connectivity ----
header "Database Connectivity"

pg_container=$("${COMPOSE_CMD[@]}" ps -q postgres 2>/dev/null || true)
if [ -n "$pg_container" ]; then
  pg_status=$(docker exec "$pg_container" pg_isready -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" 2>/dev/null || true)
  if echo "$pg_status" | grep -qi "accepting"; then
    pass "PostgreSQL: accepting connections"
  else
    fail "PostgreSQL: $pg_status"
  fi
else
  warn "PostgreSQL container not found via compose"

  pg_direct=$(pg_isready -h 127.0.0.1 -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" 2>/dev/null || true)
  if [ -n "$pg_direct" ] && echo "$pg_direct" | grep -qi "accepting"; then
    pass "PostgreSQL (direct): accepting connections"
  else
    fail "PostgreSQL: not reachable"
  fi
fi

# ---- Section 6: Redis Connectivity ----
header "Redis Connectivity"

redis_container=$("${COMPOSE_CMD[@]}" ps -q redis 2>/dev/null || true)
if [ -n "$redis_container" ]; then
  redis_ping=$(docker exec "$redis_container" redis-cli ping 2>/dev/null || true)
  if [ "$redis_ping" = "PONG" ]; then
    pass "Redis: PONG"
  else
    fail "Redis: $redis_ping"
  fi
else
  redis_direct=$(redis-cli -h 127.0.0.1 ping 2>/dev/null || true)
  if [ "$redis_direct" = "PONG" ]; then
    pass "Redis (direct): PONG"
  else
    fail "Redis: not reachable"
  fi
fi

# ---- Section 7: Prometheus Metrics ----
header "Prometheus Metrics"

PROM_URL="${PROM_URL:-http://127.0.0.1:9091}"

prom_targets=$(curl -sf "$PROM_URL/api/v1/targets" 2>/dev/null || true)
if [ -n "$prom_targets" ]; then
  pass "Prometheus API reachable"
else
  warn "Prometheus: not reachable"
fi

# ---- Summary ----
echo ""
if [ "$overall_status" -eq 0 ]; then
  echo -e "  ${GREEN}All checks passed${NC}"
else
  echo -e "  ${RED}Some checks failed${NC}"
fi

exit "$overall_status"
