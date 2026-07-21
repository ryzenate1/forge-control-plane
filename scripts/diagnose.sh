#!/usr/bin/env bash
# GamePanel Diagnostic Script — comprehensive system health check
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

ok()  { echo -e "  ${GREEN}[OK]${NC} $1"; }
fail() { echo -e "  ${RED}[FAIL]${NC} $1"; }
info() { echo -e "  ${YELLOW}[INFO]${NC} $1"; }

DIAG_DIR=$(mktemp -d)
REPORT="/tmp/diagnostic-report-$(date +%Y%m%d-%H%M%S).txt"

echo "" | tee -a "$REPORT"
echo "========================================" | tee -a "$REPORT"
echo "  GamePanel System Diagnostic" | tee -a "$REPORT"
echo "  $(date -u '+%Y-%m-%d %H:%M:%S UTC')" | tee -a "$REPORT"
echo "========================================" | tee -a "$REPORT"
echo "" | tee -a "$REPORT"

# === Service health checks ===
echo "--- Service health ---" | tee -a "$REPORT"

check_endpoint() {
  local name=$1 url=$2
  echo -n "  $name: " | tee -a "$REPORT"
  if curl -sf "$url" > /dev/null 2>&1; then
    ok "$name ($url)" | tee -a "$REPORT"
  else
    fail "$name ($url)" | tee -a "$REPORT"
  fi
}

check_endpoint "Forge API" "http://localhost:8080/api/v1/health/ready"
check_endpoint "Beacon" "http://localhost:9090/health"
check_endpoint "Forge Web" "http://localhost:3000"
check_endpoint "Prometheus" "http://localhost:9091/-/ready"
check_endpoint "Alertmanager" "http://localhost:9093/-/ready"
check_endpoint "Grafana" "http://localhost:3001/api/health"

# === Docker container status ===
echo "" | tee -a "$REPORT"
echo "--- Docker containers ---" | tee -a "$REPORT"
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null | tee -a "$REPORT" || info "Docker not available"

# === PostgreSQL check ===
echo "" | tee -a "$REPORT"
echo "--- PostgreSQL ---" | tee -a "$REPORT"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q postgres; then
  PG_CONTAINER=$(docker ps --format '{{.Names}}' 2>/dev/null | grep postgres | head -1)
  if docker exec "$PG_CONTAINER" pg_isready -U gamepanel > /dev/null 2>&1; then
    ok "PostgreSQL is ready"
    # Query database stats
      info "Migrations applied: $(docker exec "$PG_CONTAINER" psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM schema_migrations" 2>/dev/null | tr -d ' ' || echo 'N/A')"
    info "Servers: $(docker exec "$PG_CONTAINER" psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM servers" 2>/dev/null | tr -d ' ' || echo 'N/A')"
    info "Nodes: $(docker exec "$PG_CONTAINER" psql -U gamepanel -d gamepanel -t -c "SELECT COUNT(*) FROM nodes" 2>/dev/null | tr -d ' ' || echo 'N/A')"
  else
    fail "PostgreSQL is not responding"
  fi
else
  fail "PostgreSQL container not found"
fi

# === Redis check ===
echo "" | tee -a "$REPORT"
echo "--- Redis ---" | tee -a "$REPORT"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q redis; then
  REDIS_CONTAINER=$(docker ps --format '{{.Names}}' 2>/dev/null | grep redis | head -1)
  if docker exec "$REDIS_CONTAINER" redis-cli ping 2>/dev/null | grep -q PONG; then
    ok "Redis is responding"
    info "Redis info: $(docker exec "$REDIS_CONTAINER" redis-cli INFO server 2>/dev/null | grep -E '^(redis_version|uptime_in_seconds|used_memory_human):' | tr '\n' ', ')"
  else
    fail "Redis is not responding"
  fi
else
  fail "Redis container not found"
fi

# === Disk space check ===
echo "" | tee -a "$REPORT"
echo "--- Disk space ---" | tee -a "$REPORT"
df -h / /var/lib/docker /srv/game-panel 2>/dev/null | tee -a "$REPORT"

# === Memory usage ===
echo "" | tee -a "$REPORT"
echo "--- Memory usage ---" | tee -a "$REPORT"
free -h | tee -a "$REPORT"

echo "" | tee -a "$REPORT"
echo "--- Top memory consumers ---" | tee -a "$REPORT"
ps aux --sort=-%mem 2>/dev/null | head -10 | tee -a "$REPORT" || ps aux -o pid,user,%mem,rss,command 2>/dev/null | head -10 | tee -a "$REPORT"

# === Network connectivity ===
echo "" | tee -a "$REPORT"
echo "--- Network connectivity ---" | tee -a "$REPORT"
# Check internal DNS
if host api 2>/dev/null || getent hosts api 2>/dev/null; then
  ok "Internal DNS resolution works"
else
  info "Internal DNS resolution skipped (not in Docker network)"
fi

# Check external connectivity
if curl -sf --max-time 5 https://google.com > /dev/null 2>&1; then
  ok "External internet connectivity"
else
  fail "No external internet connectivity"
fi

# === Log collection ===
echo "" | tee -a "$REPORT"
echo "--- Recent logs ---" | tee -a "$REPORT"
for logfile in api-dev.log api-dev.err.log beacon-dev.err.log frontend-dev.log; do
  if [ -f "$logfile" ]; then
    echo "  Last 10 lines of $logfile:" | tee -a "$REPORT"
    tail -10 "$logfile" 2>/dev/null | sed 's/^/    /' | tee -a "$REPORT"
  fi
done

# Docker logs
for service in api daemon web; do
  if docker logs "$service" --tail 5 2>/dev/null; then
    echo "  Last 5 lines of $service container:" | tee -a "$REPORT"
    docker logs "$service" --tail 5 2>/dev/null | sed 's/^/    /' | tee -a "$REPORT"
  fi
done

# === Collect docker-compose ps ===
echo "" | tee -a "$REPORT"
echo "--- Docker Compose status ---" | tee -a "$REPORT"
if [ -d infra ]; then
  (cd infra && docker compose -f compose.yml ps 2>/dev/null) | tee -a "$REPORT" || info "Docker Compose not available"
fi

# === Report summary ===
echo "" | tee -a "$REPORT"
echo "========================================" | tee -a "$REPORT"
echo "  Diagnostic Report Saved" | tee -a "$REPORT"
echo "  $REPORT" | tee -a "$REPORT"
echo "========================================" | tee -a "$REPORT"
echo "" | tee -a "$REPORT"
echo "Next steps:" | tee -a "$REPORT"
echo "  1. Review any [FAIL] results above" | tee -a "$REPORT"
echo "  2. Check container logs: docker compose logs --tail 200" | tee -a "$REPORT"
echo "  3. Run production guard: ./scripts/production-guard.sh" | tee -a "$REPORT"
echo "" | tee -a "$REPORT"

echo "  Report saved to: $REPORT"
