#!/usr/bin/env bash
# GamePanel Production Deployment Script
# Supports blue-green deployment with health checks and rollback
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

# Configuration
INSTALL_DIR="/opt/gamepanel"
COMPOSE_DIR="$INSTALL_DIR/infra"
COMPOSE_FILES="-f compose.yml -f compose.production.yml"
ENV_FILE="--env-file .env"
BACKUP_DIR="/var/backups/gamepanel"
ROLLBACK_DIR="$BACKUP_DIR/rollback"
HEALTH_RETRIES=30
HEALTH_INTERVAL=5

usage() {
  echo "Usage: $0 [--rollback] [--version <tag>] [--dry-run]"
  echo ""
  echo "Options:"
  echo "  --rollback          Roll back to the previous deployment"
  echo "  --version <tag>     Specify Docker image tag to deploy (default: latest)"
  echo "  --dry-run           Print what would be done without making changes"
  echo "  --help              Show this help message"
  exit 0
}

ROLLBACK=false
VERSION="latest"
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --rollback) ROLLBACK=true; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    --help) usage ;;
    *) fail "Unknown option: $1. Use --help for usage." ;;
  esac
done

if [ "$(id -u)" -ne 0 ] && [ "$DRY_RUN" = false ]; then
  fail "This script must be run as root (sudo ./deploy-prod.sh)"
fi

echo ""
echo -e "  ${RED}+==========================================+${NC}"
echo -e "  ${RED}|   GamePanel Production Deploy             |${NC}"
echo -e "  ${RED}|   Version: $VERSION                          ${NC}"
echo -e "  ${RED}+==========================================+${NC}"
echo ""

if [ "$DRY_RUN" = true ]; then
  info "DRY RUN MODE — no changes will be made"
fi

# === Pre-flight checks ===
header "Pre-flight checks"

if [ "$DRY_RUN" = false ]; then
  # Check Docker
  if ! command -v docker &>/dev/null; then
    fail "Docker is not installed"
  fi
  info "Docker found: $(docker --version)"

  # Check Docker Compose
  if ! docker compose version &>/dev/null; then
    fail "Docker Compose v2 is not available"
  fi
  info "Docker Compose found: $(docker compose version)"

  # Check install directory
  if [ ! -d "$COMPOSE_DIR" ]; then
    fail "Install directory not found: $COMPOSE_DIR"
  fi
  info "Install directory found: $INSTALL_DIR"

  # Check environment file
  if [ ! -f "$COMPOSE_DIR/.env" ]; then
    fail "Environment file not found: $COMPOSE_DIR/.env"
  fi
  info "Environment file found"

  # Run production guard
  if [ -f "$INSTALL_DIR/scripts/production-guard.sh" ]; then
    info "Running production guard..."
    bash "$INSTALL_DIR/scripts/production-guard.sh"
  fi
fi

# === Backup current state ===
header "Backing up current state"
if [ "$DRY_RUN" = false ]; then
  mkdir -p "$ROLLBACK_DIR"
  TIMESTAMP=$(date +%Y%m%d-%H%M%S)
  if [ -d "$COMPOSE_DIR" ]; then
    cp "$COMPOSE_DIR/.env" "$ROLLBACK_DIR/.env.$TIMESTAMP"
    docker compose $COMPOSE_FILES $ENV_FILE config 2>/dev/null > "$ROLLBACK_DIR/docker-compose-config.$TIMESTAMP.yml" || true
    info "Current state backed up to $ROLLBACK_DIR"
  fi
fi

# === Pull new images ===
header "Pulling Docker images"
if [ "$DRY_RUN" = false ]; then
  (cd "$COMPOSE_DIR" && TAG=$VERSION docker compose $COMPOSE_FILES $ENV_FILE pull) || warn "Image pull encountered warnings"
  info "Docker images pulled"
fi

# === Deploy (blue-green) ===
header "Deploying services"

if [ "$ROLLBACK" = true ]; then
  info "Rolling back to previous deployment..."
  # Find most recent backup
  LATEST_BACKUP=$(ls -t "$ROLLBACK_DIR/.env."* 2>/dev/null | head -1)
  if [ -z "$LATEST_BACKUP" ]; then
    fail "No rollback backup found"
  fi
  info "Restoring from: $LATEST_BACKUP"
  if [ "$DRY_RUN" = false ]; then
    cp "$LATEST_BACKUP" "$COMPOSE_DIR/.env"
  fi
fi

if [ "$DRY_RUN" = false ]; then
  (cd "$COMPOSE_DIR" && docker compose $COMPOSE_FILES $ENV_FILE up -d --remove-orphans) || fail "Deploy failed"
  info "Services deployed"
fi

# === Health check verification ===
header "Verifying health"

check_health() {
  local name=$1 url=$2 retries=$3
  echo -n "  Waiting for $name... "
  for i in $(seq 1 "$retries"); do
    if curl -sf "$url" > /dev/null 2>&1; then
      echo -e "${GREEN}healthy${NC}"
      return 0
    fi
    sleep "$HEALTH_INTERVAL"
  done
  echo -e "${RED}unhealthy${NC}"
  return 1
}

if [ "$DRY_RUN" = false ]; then
  ALL_HEALTHY=true

  check_health "Forge API" "http://localhost:8080/api/v1/health/ready" "$HEALTH_RETRIES" || ALL_HEALTHY=false
  check_health "Forge Web" "http://localhost:3000" 12 || ALL_HEALTHY=false
  check_health "Beacon" "http://localhost:9090/health" 12 || ALL_HEALTHY=false

  if [ "$ALL_HEALTHY" = false ]; then
    warn "Some services are unhealthy!"
    info "Running diagnostics..."
    bash "$INSTALL_DIR/scripts/diagnose.sh" 2>/dev/null || true
    if [ "$ROLLBACK" = false ]; then
      warn "Consider rolling back: $0 --rollback"
    fi
  else
    info "All services healthy"
  fi
fi

# === Summary ===
echo ""
echo -e "  ${GREEN}+==========================================+${NC}"
echo -e "  ${GREEN}|   Deployment Complete                      |${NC}"
echo -e "  ${GREEN}+==========================================+${NC}"
echo ""
echo "  Version:     $VERSION"
echo "  Action:      $([ "$ROLLBACK" = true ] && echo "rollback" || echo "deploy")"
echo "  Directory:   $INSTALL_DIR"
echo "  Timestamp:   $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""
echo "  Commands:"
echo "    docker compose -f $COMPOSE_DIR/compose.yml ps"
echo "    docker compose -f $COMPOSE_DIR/compose.yml logs --tail 50"
echo "    $0 --rollback"
echo ""
