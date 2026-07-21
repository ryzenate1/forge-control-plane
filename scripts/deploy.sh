#!/usr/bin/env bash
# ============================================================
# GamePanel - Production Deployment Script (Docker Compose)
# ============================================================
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "  ${GREEN}[ok]${NC} $1"; }
warn()  { echo -e "  ${YELLOW}[!!]${NC} $1"; }
fail()  { echo -e "  ${RED}[FAIL]${NC} $1"; exit 1; }
header(){ echo -e "\n${CYAN}=== $1 ===${NC}"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
INFRA_DIR="$PROJECT_DIR/infra"

DRY_RUN=false
VERSION="latest"
COMPOSE_FILES=(-f compose.yml -f compose.production.yml)

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    --help)
      echo "Usage: $0 [--dry-run] [--version <tag>]"
      echo ""
      echo "  --dry-run         Simulate the deployment without making changes"
      echo "  --version <tag>   Deploy a specific image tag (default: latest)"
      echo ""
      echo "Phased rollout order:"
      echo "  1. postgres"
      echo "  2. redis"
      echo "  3. api (with migrations)"
      echo "  4. web"
      echo "  5. daemon"
      echo "  6. monitoring (prometheus, alertmanager, grafana)"
      exit 0
      ;;
    *) fail "Unknown option: $1. Use --help for usage." ;;
  esac
done

header "Environment Validation"

if ! docker compose version >/dev/null 2>&1; then
  fail "Docker Compose v2 is required"
fi
info "Docker Compose v2 found"

if [ ! -f "$INFRA_DIR/.env" ]; then
  fail "infra/.env is missing. Run: cd infra && ./gen-env.sh .env"
fi
info "Environment file found"

if [ "$DRY_RUN" = true ]; then
  warn "DRY RUN MODE - no changes will be made"
fi

export COMPOSE_FILE=""
export ENV_FILE="$INFRA_DIR/.env"

compose_up() {
  local service=$1
  local label=$2
  if [ "$DRY_RUN" = true ]; then
    warn "[dry-run] Would start $label ($service)"
    return 0
  fi
  info "Starting $label..."
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" up -d --no-deps --wait "$service" 2>&1 | sed 's/^/  /'
}

healthcheck_service() {
  local service=$1
  local label=$2
  local retries=30
  local interval=5
  info "Waiting for $label to become healthy..."

  for i in $(seq 1 "$retries"); do
    local status
    status=$(docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" ps --format json "$service" 2>/dev/null | grep -o '"Status":"[^"]*"' | head -1)
    if echo "$status" | grep -qi "healthy"; then
      info "$label is healthy"
      return 0
    fi
    if [ "$i" -eq "$retries" ]; then
      fail "$label failed to become healthy after $((retries * interval))s"
    fi
    sleep "$interval"
  done
}

run_migrations() {
  if [ "$DRY_RUN" = true ]; then
    warn "[dry-run] Would run database migrations"
    return 0
  fi
  header "Database Migrations"
  info "Running database migrations..."

  local api_container
  api_container=$(docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" ps -q api 2>/dev/null)
  if [ -z "$api_container" ]; then
    warn "API container not running, skipping migrations (will run on next API start)"
    return 0
  fi

  if docker exec "$api_container" /api --migrate 2>&1; then
    info "Migrations completed successfully"
  else
    fail "Migrations failed"
  fi
}

rollback_on_failure() {
  local phase=$1
  echo ""
  warn "Deployment failed at phase: $phase"
  warn "Initiating rollback to previous state..."
  if [ -f "$INFRA_DIR/.deploy.backup" ]; then
    warn "Restoring previous Compose state..."
    docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" down --timeout 30 2>/dev/null || true
    docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" up -d 2>/dev/null || true
  fi
  fail "Deployment aborted. Manual intervention required."
}

# --- Phase 1: Postgres ---
header "Phase 1/6: PostgreSQL"
if [ "$DRY_RUN" = false ]; then
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull postgres 2>&1 | sed 's/^/  /' || warn "Could not pull postgres image, using cached"
fi
compose_up postgres "PostgreSQL" || rollback_on_failure "postgres"
healthcheck_service postgres "PostgreSQL"

# --- Phase 2: Redis ---
header "Phase 2/6: Redis"
if [ "$DRY_RUN" = false ]; then
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull redis 2>&1 | sed 's/^/  /' || warn "Could not pull redis image, using cached"
fi
compose_up redis "Redis" || rollback_on_failure "redis"
healthcheck_service redis "Redis"

# --- Phase 3: API + Migrations ---
header "Phase 3/6: API"
if [ "$DRY_RUN" = false ]; then
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull api 2>&1 | sed 's/^/  /' || warn "Could not pull api image, using cached"
fi
compose_up api "API" || rollback_on_failure "api"
healthcheck_service api "API"
run_migrations

# --- Phase 4: Web ---
header "Phase 4/6: Web"
if [ "$DRY_RUN" = false ]; then
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull web 2>&1 | sed 's/^/  /' || warn "Could not pull web image, using cached"
fi
compose_up web "Web" || rollback_on_failure "web"
healthcheck_service web "Web"

# --- Phase 5: Daemon ---
header "Phase 5/6: Daemon"
if [ "$DRY_RUN" = false ]; then
  docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull daemon 2>&1 | sed 's/^/  /' || warn "Could not pull daemon image, using cached"
fi
compose_up daemon "Daemon" || rollback_on_failure "daemon"
healthcheck_service daemon "Daemon"

# --- Phase 6: Monitoring ---
header "Phase 6/6: Monitoring"
for svc in prometheus alertmanager grafana; do
  if [ "$DRY_RUN" = false ]; then
    docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env" pull "$svc" 2>&1 | sed 's/^/  /' || warn "Could not pull $svc image, using cached"
  fi
  compose_up "$svc" "$svc" || warn "Monitoring service $svc failed to start (non-critical)"
done

header "Deployment Complete"
info "All services deployed successfully."
info "Run ./scripts/healthcheck.sh to verify full system status."

if [ "$DRY_RUN" = true ]; then
  warn "DRY RUN - no changes were made"
fi
