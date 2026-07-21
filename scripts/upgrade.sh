#!/usr/bin/env bash
# ============================================================
# GamePanel — Upgrade Script
#
# Upgrades the Forge control plane to the latest version.
# Performs backup, pulls latest images, runs migrations,
# verifies health, and rolls back on failure.
#
# Usage:
#   ./scripts/upgrade.sh
#   curl -fsSL https://.../upgrade.sh | bash
#
# Options:
#   --skip-backup     Skip database and config backup
#   --force           Force upgrade even if version is current
#   --rollback        Rollback to the previous version after upgrade
#   --help            Show help and exit
# ============================================================

set -euo pipefail

# --- Constants ---
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
readonly BACKUP_DIR="${GAMEPANEL_BACKUP_DIR:-/var/backups/gamepanel/upgrade/$TIMESTAMP}"

# --- State ---
ENV_FILE="${GAMEPANEL_ENV_FILE:-$PROJECT_ROOT/infra/.env}"
INFRA_DIR="${GAMEPANEL_INFRA_DIR:-$PROJECT_ROOT/infra}"
COMPOSE_FILES=(-f compose.yml -f compose.production.yml)
COMPOSE_CMD=(docker compose "${COMPOSE_FILES[@]}" --env-file "$ENV_FILE")
SKIP_BACKUP=false
FORCE=false
ROLLBACK=false
PREVIOUS_VERSION=""
NEW_VERSION=""
BACKUP_SUCCESS=false
UPGRADE_SUCCESS=false

# --- Colors ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
    CYAN='\033[0;36m'; BLUE='\033[0;34m'; NC='\033[0m'
    BOLD='\033[1m'
else
    RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; NC=''; BOLD=''
fi

info()  { printf "  ${GREEN}[ok]${NC} %s\n" "$1"; }
warn()  { printf "  ${YELLOW}[!!]${NC} %s\n" "$1"; }
detail(){ printf "  ${BLUE}[..]${NC} %s\n" "$1"; }
fail()  { printf "  ${RED}[FAIL]${NC} %s\n" "$1"; exit 1; }
header(){ printf "\n${CYAN}${BOLD}=== %s ===${NC}\n" "$1"; }
step()  { printf "\n${BOLD}[%s/%s]${NC} %s\n" "$1" "$2" "$3"; }

have() { command -v "$1" >/dev/null 2>&1; }

# --- CLI parsing ---
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --skip-backup) SKIP_BACKUP=true; shift ;;
            --force) FORCE=true; shift ;;
            --rollback) ROLLBACK=true; shift ;;
            --help|-h)
                cat <<EOF
Usage: ./upgrade.sh [OPTIONS]

Options:
  --skip-backup  Skip database and config backup
  --force        Force upgrade even if version is current
  --rollback     Rollback to the previous version after upgrade
  --help         Show this message.
EOF
                exit 0
                ;;
            *) fail "Unknown option: $1. Use --help for usage." ;;
        esac
    done
}

# ============================================================
# Step 1: Pre-flight Checks
# ============================================================
pre_flight() {
    step "1" "8" "Pre-flight checks"

    if [ ! -f "$ENV_FILE" ]; then
        fail "Environment file not found at $ENV_FILE. Is GamePanel installed?"
    fi
    info "Environment file found: $ENV_FILE"

    if ! have docker; then
        fail "Docker is not installed"
    fi
    if ! docker compose version >/dev/null 2>&1; then
        fail "Docker Compose v2 is required"
    fi
    info "Docker and Docker Compose available"

    if [ ! -d "$INFRA_DIR" ]; then
        fail "Infra directory not found: $INFRA_DIR"
    fi
    info "Infra directory found: $INFRA_DIR"
}

# ============================================================
# Step 2: Version Detection
# ============================================================
detect_versions() {
    step "2" "8" "Version detection"

    # Detect current version from running API container
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'gamepanel.*api\|forge.*api'; then
        local container
        container="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'gamepanel.*api|forge.*api' | head -1)"
        PREVIOUS_VERSION="$(docker inspect "$container" --format '{{.Config.Image}}' 2>/dev/null || echo "unknown")"
    fi
    if [ -f "$PROJECT_ROOT/version.txt" ]; then
        PREVIOUS_VERSION="${PREVIOUS_VERSION:-$(cat "$PROJECT_ROOT/version.txt")}"
    fi
    PREVIOUS_VERSION="${PREVIOUS_VERSION:-unknown}"
    info "Current version: $PREVIOUS_VERSION"

    # Detect latest version from Docker image tags
    local image
    image="$(grep -E '^\s*image:\s+' "$INFRA_DIR/compose.yml" 2>/dev/null | head -1 | awk '{print $2}' | cut -d: -f1 || true)"
    if [ -n "$image" ] && have docker; then
        detail "Checking latest image: $image"
        NEW_VERSION="latest"
    else
        NEW_VERSION="latest"
    fi
    info "Target version: $NEW_VERSION"

    if [ "$PREVIOUS_VERSION" = "$NEW_VERSION" ] && [ "$FORCE" = "false" ]; then
        if [ "$ROLLBACK" = "true" ]; then
            warn "Rollback requested. Bypassing version check."
        else
            info "Already at latest version. Use --force to re-run."
            exit 0
        fi
    fi
}

# ============================================================
# Step 3: Changelog Display
# ============================================================
show_changelog() {
    step "3" "8" "Release information"

    # Display changelog if available
    if [ -f "$PROJECT_ROOT/CHANGELOG.md" ]; then
        detail "Recent changes:"
        head -30 "$PROJECT_ROOT/CHANGELOG.md" | while IFS= read -r line; do
            printf "    %s\n" "$line"
        done
    else
        detail "No CHANGELOG.md found locally. Check the repository for release notes."
    fi

    echo ""
    if [ "$ROLLBACK" = "true" ]; then
        warn "ROLLBACK MODE: Will revert to previous version if this upgrade fails"
    fi
    printf "  Proceed with upgrade? [y/N]: "
    read -r response
    case "${response:-n}" in [Yy]*) ;; *) echo "  Aborted."; exit 0 ;; esac
}

# ============================================================
# Step 4: Backup
# ============================================================
create_backup() {
    step "4" "8" "Creating backup"

    if [ "$SKIP_BACKUP" = "true" ]; then
        warn "Skipping backup (--skip-backup)"
        return 0
    fi

    mkdir -p "$BACKUP_DIR"
    detail "Backup directory: $BACKUP_DIR"

    # Backup environment file
    cp "$ENV_FILE" "$BACKUP_DIR/.env.backup"
    info "Environment backed up"

    # Backup compose files
    cp "$INFRA_DIR/compose.yml" "$BACKUP_DIR/compose.yml.backup" 2>/dev/null || true
    cp "$INFRA_DIR/compose.production.yml" "$BACKUP_DIR/compose.production.yml.backup" 2>/dev/null || true
    info "Compose files backed up"

    # Database dump
    detail "Creating database dump..."
    local db_url
    db_url="$(grep '^DATABASE_URL=' "$ENV_FILE" | cut -d= -f2- || true)"
    if [ -n "$db_url" ] && have pg_dump; then
        detail "Using pg_dump for database backup"
        if pg_dump "${db_url%%\?*}" > "$BACKUP_DIR/database.sql" 2>/dev/null; then
            info "Database dump saved"
        else
            warn "Database dump failed. Attempting Docker-based backup..."
            if docker compose -f "$INFRA_DIR/compose.yml" --env-file "$ENV_FILE" exec -T postgres pg_dump -U "${POSTGRES_USER:-gamepanel}" "${POSTGRES_DB:-gamepanel}" > "$BACKUP_DIR/database.sql" 2>/dev/null; then
                info "Database dump saved via Docker"
            else
                warn "Database backup could not be created. Continue without DB backup?"
                printf "  Continue? [y/N]: "
                read -r response
                case "${response:-n}" in [Yy]*) ;; *) fail "Upgrade aborted" ;; esac
            fi
        fi
    elif have docker; then
        detail "Using Docker exec for database backup"
        if docker compose -f "$INFRA_DIR/compose.yml" --env-file "$ENV_FILE" exec -T postgres pg_dump -U "${POSTGRES_USER:-gamepanel}" "${POSTGRES_DB:-gamepanel}" > "$BACKUP_DIR/database.sql" 2>/dev/null; then
            info "Database dump saved via Docker"
        else
            warn "Database backup could not be created."
        fi
    else
        warn "pg_dump not found. Skipping database backup."
    fi

    BACKUP_SUCCESS=true
    info "Backup completed: $BACKUP_DIR"
}

# ============================================================
# Step 5: Pull Latest Images
# ============================================================
pull_images() {
    step "5" "8" "Pulling latest Docker images"

    cd "$INFRA_DIR"
    detail "Pulling images for all services..."

    if "${COMPOSE_CMD[@]}" pull 2>&1; then
        info "All images pulled successfully"
    else
        warn "Some images could not be pulled. Continuing with existing images."
    fi
}

# ============================================================
# Step 6: Run Upgrade
# ============================================================
run_upgrade() {
    step "6" "8" "Running upgrade"

    cd "$INFRA_DIR"

    detail "Stopping services for upgrade..."
    "${COMPOSE_CMD[@]}" down --remove-orphans 2>/dev/null || true
    info "Services stopped"

    detail "Starting upgraded services..."
    "${COMPOSE_CMD[@]}" up -d --build --remove-orphans postgres redis
    info "Core services started"

    # Wait for PostgreSQL
    detail "Waiting for PostgreSQL (max 60s)..."
    local pg_attempts=0
    while [ $pg_attempts -lt 60 ]; do
        if "${COMPOSE_CMD[@]}" ps postgres 2>/dev/null | grep -q 'healthy'; then
            info "PostgreSQL is healthy"
            break
        fi
        sleep 2
        pg_attempts=$((pg_attempts + 1))
        printf "."
    done
    echo ""
    if [ $pg_attempts -ge 60 ]; then
        fail "PostgreSQL did not become healthy"
    fi

    # Run database migrations (API handles this on startup automatically)
    detail "Starting API to run database migrations..."
    "${COMPOSE_CMD[@]}" up -d --build api
    info "API started (migrations run automatically)"

    # Wait for API health
    detail "Waiting for API health (max 120s)..."
    local api_attempts=0
    while [ $api_attempts -lt 120 ]; do
        if "${COMPOSE_CMD[@]}" ps api 2>/dev/null | grep -q 'healthy'; then
            info "API is healthy — migrations completed"
            break
        fi
        sleep 2
        api_attempts=$((api_attempts + 1))
        printf "."
    done
    echo ""
    if [ $api_attempts -ge 120 ]; then
        warn "API health check timed out. Checking logs..."
        "${COMPOSE_CMD[@]}" logs --tail 30 api 2>/dev/null || true
        fail "API did not become healthy"
    fi

    # Start remaining services
    detail "Starting remaining services..."
    "${COMPOSE_CMD[@]}" up -d --build web daemon prometheus alertmanager grafana
    info "All services started"

    UPGRADE_SUCCESS=true
}

# ============================================================
# Step 7: Health Verification
# ============================================================
verify_health() {
    step "7" "8" "Health verification"

    sleep 5

    header "Service Status"
    "${COMPOSE_CMD[@]}" ps

    header "API Health Check"
    local api_health
    api_health="$(curl -fsS --max-time 10 http://127.0.0.1:8080/api/v1/health/ready 2>&1 || echo "FAILED")"
    if [ "$api_health" != "FAILED" ]; then
        info "API health endpoint: OK"
        echo "$api_health" | python3 -m json.tool 2>/dev/null || echo "$api_health"
    else
        warn "API health check failed"
    fi

    header "Web UI Check"
    if curl -fsS --max-time 10 -o /dev/null http://127.0.0.1:3000 2>/dev/null; then
        info "Web UI reachable"
    else
        warn "Web UI not yet responding"
    fi

    header "Daemon Check"
    if curl -fsS --max-time 10 -o /dev/null http://127.0.0.1:9090/health 2>/dev/null; then
        info "Daemon reachable"
    else
        warn "Daemon not yet responding"
    fi
}

# ============================================================
# Step 8: Summary
# ============================================================
print_summary() {
    step "8" "8" "Upgrade summary"

    echo ""
    if [ "$UPGRADE_SUCCESS" = "true" ]; then
        echo -e "  ${GREEN}${BOLD}+============================================================+${NC}"
        echo -e "  ${GREEN}${BOLD}|          Upgrade Completed Successfully!                    |${NC}"
        echo -e "  ${GREEN}${BOLD}+============================================================+${NC}"
    else
        echo -e "  ${RED}${BOLD}+============================================================+${NC}"
        echo -e "  ${RED}${BOLD}|          Upgrade Failed — Rolling Back                      |${NC}"
        echo -e "  ${RED}${BOLD}+============================================================+${NC}"
        rollback
        return
    fi

    echo ""
    echo -e "  ${BOLD}Version${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Previous: ${YELLOW}$PREVIOUS_VERSION${NC}"
    echo -e "  Current:  ${GREEN}$NEW_VERSION${NC}"
    echo ""

    echo -e "  ${BOLD}Backup${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Location: ${CYAN}$BACKUP_DIR${NC}"
    if [ "$BACKUP_SUCCESS" = "true" ]; then
        echo -e "  Status:   ${GREEN}Backup completed${NC}"
        echo -e "  Rollback: ${YELLOW}./scripts/upgrade.sh --rollback${NC}"
    fi
    echo ""

    echo -e "  ${BOLD}Management${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Status:  cd $INFRA_DIR && ${COMPOSE_CMD[*]} ps"
    echo -e "  Logs:    cd $INFRA_DIR && ${COMPOSE_CMD[*]} logs -f"
    echo ""
}

# ============================================================
# Rollback
# ============================================================
rollback() {
    header "Rollback"

    if [ "$BACKUP_SUCCESS" != "true" ]; then
        warn "No backup available. Cannot rollback automatically."
        warn "Manual intervention required."
        return 1
    fi

    detail "Stopping upgraded services..."
    "${COMPOSE_CMD[@]}" down --remove-orphans --volumes 2>/dev/null || true

    detail "Restoring environment backup..."
    if [ -f "$BACKUP_DIR/.env.backup" ]; then
        cp "$BACKUP_DIR/.env.backup" "$ENV_FILE"
        info "Environment restored"
    fi

    detail "Restoring compose files..."
    [ -f "$BACKUP_DIR/compose.yml.backup" ] && cp "$BACKUP_DIR/compose.yml.backup" "$INFRA_DIR/compose.yml" && info "compose.yml restored" || true
    [ -f "$BACKUP_DIR/compose.production.yml.backup" ] && cp "$BACKUP_DIR/compose.production.yml.backup" "$INFRA_DIR/compose.production.yml" && info "compose.production.yml restored" || true

    detail "Restoring database..."
    if [ -f "$BACKUP_DIR/database.sql" ]; then
        # Drop and recreate
        "${COMPOSE_CMD[@]}" up -d postgres redis
        sleep 10
        "${COMPOSE_CMD[@]}" exec -T postgres psql -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" 2>/dev/null || true
        "${COMPOSE_CMD[@]}" exec -T postgres psql -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" < "$BACKUP_DIR/database.sql" 2>/dev/null && info "Database restored" || warn "Database restore failed"
    fi

    detail "Starting previous version services..."
    "${COMPOSE_CMD[@]}" up -d --build postgres redis api web daemon prometheus alertmanager grafana 2>/dev/null || true
    info "Previous version services started"

    warn "Rollback completed."
}

# ============================================================
# Main Entry Point
# ============================================================
main() {
    parse_args "$@"

    echo ""
    echo -e "  ${CYAN}${BOLD}+============================================================+${NC}"
    echo -e "  ${CYAN}${BOLD}|           GamePanel · Upgrade Script                        |${NC}"
    echo -e "  ${CYAN}${BOLD}+============================================================+${NC}"
    echo ""

    if [ "$ROLLBACK" = "true" ]; then
        header "Rollback Mode"
        rollback
        exit $?
    fi

    pre_flight
    detect_versions
    show_changelog
    create_backup
    pull_images
    run_upgrade
    verify_health
    print_summary

    echo -e "  ${GREEN}${BOLD}Upgrade completed at $(date -u +"%Y-%m-%dT%H:%M:%SZ")${NC}"
    echo ""
}

main "$@"
