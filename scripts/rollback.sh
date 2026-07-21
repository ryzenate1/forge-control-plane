#!/usr/bin/env bash
# ============================================================
# GamePanel - Rollback Script
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
INFRA_DIR="$(cd "$SCRIPT_DIR/../infra" && pwd)"
BACKUP_DIR="${POSTGRES_BACKUP_HOST_DIR:-/var/backups/gamepanel/postgres}"

COMPOSE_CMD=(docker compose -f "$INFRA_DIR/compose.yml" -f "$INFRA_DIR/compose.production.yml" --env-file "$INFRA_DIR/.env")

list_versions() {
  header "Available Backup Snapshots"
  if [ -d "$BACKUP_DIR" ]; then
    local files
    files=$(find "$BACKUP_DIR" -maxdepth 1 -name 'gamepanel-*.dump' -type f 2>/dev/null | sort -r)
    if [ -z "$files" ]; then
      warn "No database backups found in $BACKUP_DIR"
    else
      echo "$files" | while read -r f; do
        local ts
        ts=$(basename "$f" .dump | sed 's/gamepanel-//')
        local size
        size=$(du -h "$f" | cut -f1)
        echo "  $ts  ($size)"
      done
    fi
  else
    warn "Backup directory $BACKUP_DIR does not exist"
  fi

  header "Available Docker Image Tags"
  local tags
  tags=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep "ghcr.io" | grep -E "(forge-api|forge-web|beacon)" | sort -u | head -20)
  if [ -z "$tags" ]; then
    warn "No GHCR images found locally"
  else
    echo "$tags" | sed 's/^/  /'
  fi
}

restore_database() {
  local backup_file=$1
  if [ ! -f "$backup_file" ]; then
    fail "Backup file not found: $backup_file"
  fi

  info "Restoring database from $(basename "$backup_file")..."

  local pg_container
  pg_container=$("${COMPOSE_CMD[@]}" ps -q postgres 2>/dev/null)
  if [ -z "$pg_container" ]; then
    fail "PostgreSQL container is not running"
  fi

  local db_name="${POSTGRES_DB:-gamepanel}"
  local db_user="${POSTGRES_USER:-gamepanel}"

  docker exec -i "$pg_container" pg_restore --clean --if-exists --dbname="postgres://${db_user}@localhost:5432/${db_name}" < "$backup_file" 2>&1 | sed 's/^/  /'
  info "Database restore completed"
}

rollback_images() {
  local target_tag=$1
  header "Rolling Back Docker Images to $target_tag"

  for service in forge-api forge-web beacon; do
    local full_image="ghcr.io/${GITHUB_REPOSITORY_OWNER:-gamepanel}/gamepanel/${service}:${target_tag}"
    info "Pulling $full_image..."
    docker pull "$full_image" 2>&1 | sed 's/^/  /'
    docker tag "$full_image" "infra-${service}:latest"
  done
  info "Images rolled back to $target_tag"
}

verify_health() {
  header "Post-Rollback Health Verification"
  local all_healthy=true

  for service in postgres redis api web daemon; do
    local status
    status=$("${COMPOSE_CMD[@]}" ps --format json "$service" 2>/dev/null | grep -o '"Status":"[^"]*"' | head -1)
    if echo "$status" | grep -qi "healthy"; then
      info "$service: healthy"
    elif echo "$status" | grep -qi "running"; then
      warn "$service: running (not yet healthy)"
      all_healthy=false
    else
      warn "$service: not running"
      all_healthy=false
    fi
  done

  "$SCRIPT_DIR/healthcheck.sh" || all_healthy=false

  if [ "$all_healthy" = true ]; then
    info "All services healthy after rollback"
    return 0
  else
    warn "Some services may not be fully healthy"
    return 1
  fi
}

usage() {
  echo "Usage: $0 [command]"
  echo ""
  echo "Commands:"
  echo "  list                         List available backup versions and image tags"
  echo "  db <backup-file>             Restore database from a backup dump file"
  echo "  images <tag>                 Roll back Docker images to a specific tag"
  echo "  full <tag> [backup-file]     Full rollback: images + database + restart"
  echo ""
  echo "Examples:"
  echo "  $0 list"
  echo "  $0 db /var/backups/gamepanel/postgres/gamepanel-20240721T120000Z.dump"
  echo "  $0 images v0.1.0"
  echo "  $0 full v0.1.0 /var/backups/gamepanel/postgres/gamepanel-20240721T120000Z.dump"
}

case "${1:-help}" in
  list)
    list_versions
    ;;
  db)
    if [ -z "${2:-}" ]; then fail "Usage: $0 db <backup-file>"; fi
    restore_database "$2"
    verify_health
    ;;
  images)
    if [ -z "${2:-}" ]; then fail "Usage: $0 images <tag>"; fi
    rollback_images "$2"
    info "Recreating services with rolled-back images..."
    "${COMPOSE_CMD[@]}" up -d --force-recreate --wait 2>&1 | sed 's/^/  /'
    verify_health
    ;;
  full)
    if [ -z "${2:-}" ]; then fail "Usage: $0 full <tag> [backup-file]"; fi
    rollback_images "$2"
    if [ -n "${3:-}" ]; then
      restore_database "$3"
    fi
    info "Recreating all services..."
    "${COMPOSE_CMD[@]}" up -d --force-recreate --wait 2>&1 | sed 's/^/  /'
    verify_health
    info "Full rollback to $2 completed"
    ;;
  help|--help|-h)
    usage
    ;;
  *)
    fail "Unknown command: $1. Use '$0 help' for usage."
    ;;
esac
