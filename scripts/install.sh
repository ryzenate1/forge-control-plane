#!/usr/bin/env bash
# ============================================================
# GamePanel — Comprehensive Installation Script
#
# Installs the Forge control plane on a Linux server using
# Docker Compose. Performs pre-flight checks, generates secure
# configuration, starts all services, and verifies the install.
#
# Usage:
#   Interactive:
#     curl -fsSL https://.../install.sh | bash
#
#   Unattended:
#     export GAMEPANEL_FQDN=panel.example.com
#     export GAMEPANEL_ADMIN_EMAIL=admin@example.com
#     export GAMEPANEL_ADMIN_PASSWORD=MySecurePass123
#     export GAMEPANEL_DB_PASSWORD=MyDBPass123
#     ./install.sh --unattended
#
#   Options:
#     --unattended     Non-interactive mode (needs env vars)
#     --skip-checks    Skip pre-flight checks (dangerous)
#     --force          Force reinstall over existing
#     --help           Show help and exit
# ============================================================

set -euo pipefail

# --- Constants ---
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly MIN_DOCKER_VERSION=24
readonly MIN_RAM_MB=1900
readonly MIN_DISK_GB=20
readonly MIN_CPUS=2
readonly REQUIRED_PORTS=(80 443 8080 9090 3000)
readonly SUPPORTED_OS=("Ubuntu 22.04" "Ubuntu 24.04" "Debian 12")
readonly TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"

# These will be set after detection
ENV_FILE=""
COMPOSE_FILES=()
COMPOSE_CMD=()
INSTALL_EXISTING=false
UNATTENDED=false
SKIP_CHECKS=false
FORCE_REINSTALL=false

# --- Colors ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
    CYAN='\033[0;36m'; BLUE='\033[0;34m'; NC='\033[0m'
    BOLD='\033[1m'
else
    RED=''; GREEN=''; YELLOW=''; CYAN=''; BLUE=''; NC=''; BOLD=''
fi

# --- Output helpers ---
info()  { printf "  ${GREEN}[ok]${NC} %s\n" "$1"; }
warn()  { printf "  ${YELLOW}[!!]${NC} %s\n" "$1"; }
detail(){ printf "  ${BLUE}[..]${NC} %s\n" "$1"; }
fail()  { printf "  ${RED}[FAIL]${NC} %s\n" "$1"; exit 1; }
header(){ printf "\n${CYAN}${BOLD}=== %s ===${NC}\n" "$1"; }
step()  { printf "\n${BOLD}[%s/%s]${NC} %s\n" "$1" "$2" "$3"; }

# --- Cleanup / rollback state ---
ROLLBACK_STATE=""
ROLLBACK_ENV_BACKUP=""
ROLLBACK_DIRS=()
ROLLBACK_MARKER=""

cleanup_on_interrupt() {
    printf "\n${YELLOW}Installation interrupted. Cleaning up...${NC}\n"
    trigger_rollback
    exit 130
}

trigger_rollback() {
    if [ -z "$ROLLBACK_STATE" ]; then
        return 0
    fi

    header "Rollback"
    if [ -n "$ROLLBACK_ENV_BACKUP" ] && [ -f "$ROLLBACK_ENV_BACKUP" ]; then
        warn "Restoring environment backup: $ENV_FILE"
        cp "$ROLLBACK_ENV_BACKUP" "$ENV_FILE"
    elif [ -n "$ROLLBACK_MARKER" ] && [ -f "$ENV_FILE" ]; then
        warn "Removing newly created .env"
        rm -f "$ENV_FILE"
    fi

    if [ "${#ROLLBACK_DIRS[@]}" -gt 0 ]; then
        for dir in "${ROLLBACK_DIRS[@]}"; do
            warn "Removing instance directory: $dir"
            rm -rf "$dir"
        done
    fi

    if [ "${#COMPOSE_CMD[@]}" -gt 0 ] && [ -f "$ENV_FILE" ]; then
        warn "Stopping containers"
        "${COMPOSE_CMD[@]}" --env-file "$ENV_FILE" down --remove-orphans --volumes 2>/dev/null || true
    fi

    warn "Rollback complete. No persistent state remains."
}

# --- CLI parsing ---
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --unattended) UNATTENDED=true; shift ;;
            --skip-checks) SKIP_CHECKS=true; shift ;;
            --force) FORCE_REINSTALL=true; shift ;;
            --help|-h)
                cat <<EOF
Usage: ./install.sh [OPTIONS]

Options:
  --unattended   Non-interactive mode. Requires env vars:
                   GAMEPANEL_FQDN          Domain name
                   GAMEPANEL_ADMIN_EMAIL   Admin email
                   GAMEPANEL_ADMIN_PASSWORD Admin password
                   GAMEPANEL_DB_PASSWORD   PostgreSQL password (optional, auto-gen)
  --skip-checks  Skip pre-flight system checks (dangerous).
  --force        Force reinstallation even if existing install found.
  --help         Show this message.

Environment variables for unattended mode:
  GAMEPANEL_FQDN           Required. Domain for the panel (e.g. panel.example.com)
  GAMEPANEL_ADMIN_EMAIL    Required. Administrator email address
  GAMEPANEL_ADMIN_PASSWORD Required. Minimum 12 characters
  GAMEPANEL_DB_PASSWORD    Optional. Auto-generated if not set
  GAMEPANEL_NODE_ID        Optional. Auto-generated UUID if not set
EOF
                exit 0
                ;;
            *) fail "Unknown option: $1. Use --help for usage." ;;
        esac
    done
}

# --- Utility functions ---
have() { command -v "$1" >/dev/null 2>&1; }

new_secret() {
    local bytes="${1:-32}"
    openssl rand -hex "$bytes"
}

new_uuid() {
    local hex
    hex="$(new_secret 16)"
    printf '%s-%s-%s-%s-%s\n' \
        "${hex:0:8}" "${hex:8:4}" "${hex:12:4}" "${hex:16:4}" "${hex:20:12}"
}

port_in_use() {
    local port="$1"
    if have nc; then
        nc -z -G 1 127.0.0.1 "$port" >/dev/null 2>&1 || nc -z -w 1 127.0.0.1 "$port" >/dev/null 2>&1
        return $?
    fi
    if have ss; then
        ss -tlnH "sport = :$port" 2>/dev/null | grep -q .
        return $?
    fi
    if have lsof; then
        lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
        return $?
    fi
    (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1
}

prompt() {
    local var_name="$1" prompt_text="$2" default="${3:-}"
    local is_secret="${4:-false}"
    local input=""
    local attempt=0

    while true; do
        attempt=$((attempt + 1))
        if [ -n "$default" ]; then
            if [ "$is_secret" = "true" ]; then
                printf "  %s [******]: " "$prompt_text"
            else
                printf "  %s [%s]: " "$prompt_text" "$default"
            fi
        else
            printf "  %s: " "$prompt_text"
        fi

        if [ "$is_secret" = "true" ]; then
            read -rs input 2>/dev/null || read -r input
            printf "\n"
        else
            read -r input
        fi
        input="${input:-$default}"

        case "$var_name" in
            FQDN)
                if [ -z "$input" ]; then
                    warn "Domain is required"
                    continue
                fi
                printf '%s' "$input"; return 0
                ;;
            ADMIN_EMAIL)
                if [ -z "$input" ]; then
                    warn "Email is required"
                    continue
                fi
                printf '%s' "$input"; return 0
                ;;
            ADMIN_PASSWORD)
                if [ -z "$input" ]; then
                    warn "Password cannot be empty"
                    continue
                fi
                if [ "${#input}" -lt 12 ]; then
                    warn "Password must be at least 12 characters"
                    continue
                fi
                declare -g GAMEPANEL_ADMIN_PASSWORD="$input"
                printf '%s' "$input"; return 0
                ;;
            DB_PASSWORD)
                printf '%s' "$input"; return 0
                ;;
        esac
    done
}

# ============================================================
# Step 1: OS Detection
# ============================================================
detect_os() {
    step "1" "9" "Detecting operating system"

    if [ ! -f /etc/os-release ]; then
        fail "Cannot detect OS — /etc/os-release not found"
    fi
    . /etc/os-release

    OS_NAME="${NAME:-unknown}"
    OS_VERSION="${VERSION_ID:-unknown}"
    OS_ID="${ID:-unknown}"

    info "Detected: $OS_NAME $OS_VERSION ($OS_ID)"

    local supported=false
    for supported_os in "${SUPPORTED_OS[@]}"; do
        if echo "$OS_NAME $OS_VERSION" | grep -qF "$supported_os"; then
            supported=true
            break
        fi
    done

    if [ "$supported" = "false" ]; then
        warn "$OS_NAME $OS_VERSION is not officially supported. Supported: ${SUPPORTED_OS[*]}"
        if [ "$UNATTENDED" = "true" ]; then
            fail "Unsupported OS in unattended mode. Use --skip-checks to override."
        fi
        printf "  Continue anyway? [y/N]: "
        read -r response
        case "${response:-n}" in [Yy]*) ;; *) exit 1 ;; esac
    fi
}

# ============================================================
# Step 2: Architecture Detection
# ============================================================
detect_arch() {
    step "2" "9" "Detecting CPU architecture"

    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64)  ARCH="amd64";  info "Architecture: amd64" ;;
        aarch64) ARCH="arm64";  info "Architecture: arm64" ;;
        armv7l)  ARCH="armv7";  warn "32-bit ARM is untested; Docker may not be available" ;;
        *)       fail "Unsupported architecture: $ARCH" ;;
    esac

    # OS + architecture combined support matrix
    case "$OS_ID-$ARCH" in
        ubuntu-amd64|ubuntu-arm64|debian-amd64|debian-arm64)
            detail "OS+arch combination is fully supported"
            ;;
        *)
            warn "OS+arch combination ($OS_ID-$ARCH) is not fully tested"
            ;;
    esac
}

# ============================================================
# Step 3: Docker Version Check
# ============================================================
check_docker() {
    step "3" "9" "Checking Docker installation"

    if ! have docker; then
        warn "Docker is not installed"
        if [ "$UNATTENDED" = "true" ]; then
            detail "Attempting unattended Docker installation..."
        else
            printf "  Install Docker Engine? [Y/n]: "
            read -r response
            case "${response:-y}" in [Nn]*) fail "Docker is required to continue." ;; esac
        fi
        install_docker
    fi

    if ! have docker; then
        fail "Docker installation failed"
    fi

    local docker_version
    docker_version="$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)"
    if [ -z "$docker_version" ]; then
        fail "Docker daemon is not running. Start Docker and retry."
    fi

    local major
    major="$(echo "$docker_version" | cut -d. -f1)"
    if [ "$major" -lt "$MIN_DOCKER_VERSION" ]; then
        fail "Docker $docker_version is too old. Minimum required: $MIN_DOCKER_VERSION"
    fi
    info "Docker $docker_version"

    if ! docker compose version >/dev/null 2>&1; then
        fail "Docker Compose v2 plugin is required (docker compose version)"
    fi
    local compose_version
    compose_version="$(docker compose version --short 2>/dev/null || echo "unknown")"
    info "Docker Compose $compose_version"
}

install_docker() {
    detail "Installing Docker Engine..."
    curl -fsSL https://get.docker.com | sh
    if ! have docker; then
        fail "Docker installation via get.docker.com failed"
    fi
    if command -v systemctl >/dev/null 2>&1; then
        systemctl enable docker
        systemctl start docker
    fi
}

# ============================================================
# Step 4: Port Availability
# ============================================================
check_ports() {
    step "4" "9" "Checking port availability"

    local conflicts=()
    for port in "${REQUIRED_PORTS[@]}"; do
        if port_in_use "$port"; then
            warn "Port $port is already in use"
            conflicts+=("$port")
        else
            info "Port $port is available"
        fi
    done

    if [ "${#conflicts[@]}" -gt 0 ]; then
        fail "Ports in use: ${conflicts[*]}. Free these ports before installing."
    fi
}

# ============================================================
# Step 5: Resource Check
# ============================================================
check_resources() {
    step "5" "9" "Checking system resources"

    local ram_mb cpus disk_gb

    # RAM
    if [ -f /proc/meminfo ]; then
        ram_mb="$(awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo)"
    elif have free; then
        ram_mb="$(free -m | awk '/Mem:/ {print $2}')"
    else
        ram_mb=0
    fi
    if [ "$ram_mb" -lt "$MIN_RAM_MB" ] && [ "$ram_mb" -gt 0 ]; then
        fail "Insufficient RAM: ${ram_mb}MB available, ${MIN_RAM_MB}MB required"
    fi
    info "RAM: ${ram_mb}MB"

    # CPUs
    cpus="$(nproc 2>/dev/null || echo 1)"
    if [ "$cpus" -lt "$MIN_CPUS" ]; then
        fail "Insufficient CPUs: $cpus available, $MIN_CPUS required"
    fi
    info "CPUs: $cpus"

    # Disk
    local install_path="${GAMEPANEL_INSTALL_DIR:-/opt/gamepanel}"
    disk_gb="$(df --output=avail -BG "$(dirname "$install_path" 2>/dev/null || echo '/')" 2>/dev/null | tail -1 | tr -dc '0-9')"
    if [ -z "$disk_gb" ]; then disk_gb=0; fi
    if [ "$disk_gb" -lt "$MIN_DISK_GB" ] && [ "$disk_gb" -gt 0 ]; then
        fail "Insufficient disk: ${disk_gb}GB available, ${MIN_DISK_GB}GB required"
    fi
    info "Disk available: ${disk_gb}GB"
}

# ============================================================
# Step 6: Interactive Configuration
# ============================================================
configure_interactive() {
    step "6" "9" "Configuration"

    if [ "$UNATTENDED" = "true" ]; then
        : "${GAMEPANEL_FQDN:?GAMEPANEL_FQDN is required for unattended install}"
        : "${GAMEPANEL_ADMIN_EMAIL:?GAMEPANEL_ADMIN_EMAIL is required for unattended install}"
        : "${GAMEPANEL_ADMIN_PASSWORD:?GAMEPANEL_ADMIN_PASSWORD is required for unattended install}"
        : "${GAMEPANEL_DB_PASSWORD:=$(new_secret 24)}"
        info "Using unattended configuration"
        info "FQDN: $GAMEPANEL_FQDN"
        info "Admin: $GAMEPANEL_ADMIN_EMAIL"
        return 0
    fi

    echo ""
    echo -e "  ${BOLD}GamePanel Setup — Interactive Configuration${NC}"
    echo ""
    echo "  You will need:"
    echo "    - A domain name pointing to this server (e.g. panel.example.com)"
    echo "    - An email address for the administrator account"
    echo "    - A strong password (min 12 characters)"
    echo ""

    GAMEPANEL_FQDN="$(prompt FQDN "Enter domain name" "${GAMEPANEL_FQDN:-}")"
    GAMEPANEL_ADMIN_EMAIL="$(prompt ADMIN_EMAIL "Enter admin email" "${GAMEPANEL_ADMIN_EMAIL:-}")"
    GAMEPANEL_ADMIN_PASSWORD="$(prompt ADMIN_PASSWORD "Enter admin password (min 12 chars)" "" true)"
    GAMEPANEL_DB_PASSWORD="${GAMEPANEL_DB_PASSWORD:-$(new_secret 24)}"

    echo ""
    info "Configuration complete"
}

# ============================================================
# Step 7: Generate Environment
# ============================================================
generate_environment() {
    step "7" "9" "Generating environment configuration"

    ENV_FILE="${GAMEPANEL_ENV_FILE:-$PROJECT_ROOT/infra/.env}"
    local env_dir
    env_dir="$(dirname "$ENV_FILE")"
    mkdir -p "$env_dir"

    # Backup existing .env if present
    if [ -f "$ENV_FILE" ]; then
        ROLLBACK_ENV_BACKUP="${ENV_FILE}.bak.${TIMESTAMP}"
        cp "$ENV_FILE" "$ROLLBACK_ENV_BACKUP"
        detail "Backed up existing .env to $ROLLBACK_ENV_BACKUP"
    else
        ROLLBACK_MARKER="new"
    fi

    local api_secret app_key node_token_id node_token_secret node_token
    local node_id postgres_password grafana_password master_key

    api_secret="$(new_secret 32)"
    app_key="$(new_secret 32)"
    node_token_id="$(new_secret 8)"
    node_token_secret="$(new_secret 32)"
    node_token="$node_token_id.$node_token_secret"
    node_id="${GAMEPANEL_NODE_ID:-$(new_uuid)}"
    postgres_password="${GAMEPANEL_DB_PASSWORD}"
    grafana_password="$(new_secret 24)"
    master_key="$(new_secret 32)"

    cat > "$ENV_FILE" <<EOF
# =====================================================================
# GamePanel Production Environment
# Generated by install.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Domain: $GAMEPANEL_FQDN
# NEVER commit this file.
# =====================================================================

# --- Database ---
POSTGRES_DB=gamepanel
POSTGRES_USER=gamepanel
POSTGRES_PASSWORD=$postgres_password
DATABASE_URL=postgres://gamepanel:$postgres_password@postgres:5432/gamepanel?sslmode=disable
POSTGRES_BACKUP_HOST_DIR=/var/backups/gamepanel/postgres
POSTGRES_BACKUP_INTERVAL_SECONDS=86400
POSTGRES_BACKUP_RETENTION_DAYS=14

# --- Redis ---
REDIS_ADDR=redis:6379

# --- Panel URL ---
PANEL_URL=https://$GAMEPANEL_FQDN

# --- API ---
API_ADDR=:8080
API_AUTH_SECRET=$api_secret
APP_KEY=$app_key
APP_ENV=production
LOAD_BALANCER_ENABLED=true
LOAD_BALANCER_BIND_HOST=
LOAD_BALANCER_PORT_MIN=30000
LOAD_BALANCER_PORT_MAX=30100

# --- Encryption at Rest ---
FORGE_MASTER_KEY=$master_key
FORGE_MASTER_KEY_ID=primary
FORGE_PREVIOUS_MASTER_KEYS=
FORGE_ALLOW_EPHEMERAL_MASTER_KEY=false

# --- Node ---
DAEMON_NODE_TOKEN=$node_token
DAEMON_ADDR=:9090
DAEMON_SFTP_ADDR=:2022
DAEMON_DATA_DIR=/srv/game-panel/servers
GAME_SERVERS_HOST_DIR=/srv/game-panel/servers
DAEMON_NODE_ID=$node_id
DAEMON_ALLOW_MOCK_RUNTIME=false
PANEL_API_URL=http://api:8080/api/v1
BEACON_PANEL_API_URL=https://$GAMEPANEL_FQDN/api/v1

# --- Grafana ---
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$grafana_password

# --- Backups ---
BACKUP_ADAPTER=local

# --- Admin Account ---
# These are consumed by the setup endpoint on first run
GAMEPANEL_ADMIN_EMAIL=$GAMEPANEL_ADMIN_EMAIL
GAMEPANEL_ADMIN_PASSWORD=$GAMEPANEL_ADMIN_PASSWORD
EOF

    chmod 600 "$ENV_FILE"
    info "Environment written to $ENV_FILE"

    ROLLBACK_STATE="env_generated"
}

# ============================================================
# Step 8: Start Services
# ============================================================
start_services() {
    step "8" "9" "Starting GamePanel services"

    local infra_dir="${GAMEPANEL_INFRA_DIR:-$PROJECT_ROOT/infra}"
    cd "$infra_dir"

    # Ensure host directories exist
    mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"
    mkdir -p "${POSTGRES_BACKUP_HOST_DIR:-/var/backups/gamepanel/postgres}"

    COMPOSE_CMD=(docker compose -f compose.yml -f compose.production.yml --env-file "$ENV_FILE")

    # Validate compose config
    detail "Validating Compose configuration"
    if ! "${COMPOSE_CMD[@]}" config --quiet 2>&1; then
        trigger_rollback
        fail "Compose configuration is invalid"
    fi

    # Pull images
    detail "Pulling Docker images"
    "${COMPOSE_CMD[@]}" pull postgres redis 2>&1 | grep -v '^$' || true

    # Build application images
    detail "Building application images"
    "${COMPOSE_CMD[@]}" build --quiet 2>&1 | tail -5

    # Start core services first (database + cache)
    detail "Starting core services (PostgreSQL, Redis)"
    "${COMPOSE_CMD[@]}" up -d postgres redis
    info "Core services started"

    # Wait for PostgreSQL to be healthy
    detail "Waiting for PostgreSQL to become healthy (max 60s)"
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
    if [ $pg_attempts -ge 60 ]; then
        trigger_rollback
        fail "PostgreSQL did not become healthy in time"
    fi

    # Start API and Web
    detail "Starting API and Web services"
    "${COMPOSE_CMD[@]}" up -d --build api web
    info "API and Web services started"

    # Wait for API to be healthy
    detail "Waiting for API to become healthy (max 120s)"
    local api_attempts=0
    while [ $api_attempts -lt 120 ]; do
        if "${COMPOSE_CMD[@]}" ps api 2>/dev/null | grep -q 'healthy'; then
            info "API is healthy"
            break
        fi
        sleep 2
        api_attempts=$((api_attempts + 1))
        printf "."
    done
    if [ $api_attempts -ge 120 ]; then
        warn "API did not report healthy in time — checking logs:"
        "${COMPOSE_CMD[@]}" logs --tail 30 api 2>/dev/null || true
        trigger_rollback
        fail "API health check failed"
    fi

    # Start remaining services
    detail "Starting daemon and monitoring services"
    "${COMPOSE_CMD[@]}" up -d --build daemon prometheus alertmanager grafana
    info "All services started"

    ROLLBACK_STATE="services_running"
}

# ============================================================
# Step 9: Post-Install Verification
# ============================================================
verify_installation() {
    step "9" "9" "Verifying installation"

    # Wait a moment for remaining services to stabilise
    sleep 5

    # Service status
    header "Service Status"
    "${COMPOSE_CMD[@]}" ps

    # API health check
    header "API Health"
    local api_health
    api_health="$(curl -fsS --max-time 10 http://127.0.0.1:8080/api/v1/health/ready 2>&1 || echo "FAILED")"
    if [ "$api_health" != "FAILED" ]; then
        info "API health: $api_health"
    else
        warn "API health check returned: $api_health"
        detail "API may still be migrating. Check logs: ${COMPOSE_CMD[*]} logs api"
    fi

    # Web UI check
    header "Web UI"
    if curl -fsS --max-time 10 -o /dev/null http://127.0.0.1:3000 2>/dev/null; then
        info "Web UI reachable on http://127.0.0.1:3000"
    else
        warn "Web UI not responding yet (may still be building)"
    fi

    # Beacon daemon check
    header "Beacon Daemon"
    if curl -fsS --max-time 10 -o /dev/null http://127.0.0.1:9090/health 2>/dev/null; then
        info "Beacon daemon reachable on http://127.0.0.1:9090/health"
    else
        warn "Beacon daemon not responding yet"
    fi

    # Clear rollback state — installation succeeded
    ROLLBACK_STATE=""
}

# ============================================================
# Final Output
# ============================================================
print_summary() {
    echo ""
    echo -e "  ${GREEN}${BOLD}+============================================================+${NC}"
    echo -e "  ${GREEN}${BOLD}|          GamePanel Installed Successfully!                  |${NC}"
    echo -e "  ${GREEN}${BOLD}+============================================================+${NC}"
    echo ""
    echo -e "  ${BOLD}Service URLs${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Web Dashboard:    ${CYAN}https://$GAMEPANEL_FQDN${NC}"
    echo -e "  Setup (first run): ${CYAN}https://$GAMEPANEL_FQDN/setup${NC}"
    echo -e "  API:              ${CYAN}https://$GAMEPANEL_FQDN/api/v1${NC}"
    echo -e "  Grafana:          ${CYAN}https://$GAMEPANEL_FQDN:3001${NC} (loopback only)"
    echo ""
    echo -e "  ${BOLD}Admin Account${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Email:    ${GREEN}$GAMEPANEL_ADMIN_EMAIL${NC}"
    echo -e "  Password: ${GREEN}(as entered)${NC}"
    echo ""
    echo -e "  ${BOLD}Management Commands${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  Status:      cd $PROJECT_ROOT/infra && ${COMPOSE_CMD[*]} ps"
    echo -e "  Logs:        cd $PROJECT_ROOT/infra && ${COMPOSE_CMD[*]} logs -f"
    echo -e "  Restart:     cd $PROJECT_ROOT/infra && ${COMPOSE_CMD[*]} restart"
    echo -e "  Stop:        cd $PROJECT_ROOT/infra && ${COMPOSE_CMD[*]} down"
    echo ""
    echo -e "  ${BOLD}Next Steps${NC}"
    echo "  -----------------------------------------------------------------"
    echo "  1. Configure Nginx + TLS for $GAMEPANEL_FQDN"
    echo "     (see docs/operations/production-deployment.md)"
    echo "  2. Open https://$GAMEPANEL_FQDN/setup in your browser"
    echo "  3. Create a node in Admin → Nodes"
    echo "  4. Update DAEMON_NODE_ID and DAEMON_NODE_TOKEN in $ENV_FILE"
    echo "  5. Deploy your first game server"
    echo ""
    echo -e "  ${BOLD}Configuration${NC}"
    echo "  -----------------------------------------------------------------"
    echo -e "  .env file:     $ENV_FILE"
    echo -e "  DB Password:   ${YELLOW}$GAMEPANEL_DB_PASSWORD${NC}"
    echo ""
    echo -e "  ${YELLOW}Store $ENV_FILE securely — it contains all secrets.${NC}"
    echo ""

    if [ -n "$ROLLBACK_ENV_BACKUP" ] && [ -f "$ROLLBACK_ENV_BACKUP" ]; then
        echo -e "  ${YELLOW}Previous .env backed up to: $ROLLBACK_ENV_BACKUP${NC}"
        echo ""
    fi
}

# ============================================================
# Existing Installation Detection
# ============================================================
detect_existing() {
    local env="${GAMEPANEL_ENV_FILE:-$PROJECT_ROOT/infra/.env}"
    local infra_dir="${GAMEPANEL_INFRA_DIR:-$PROJECT_ROOT/infra}"

    if [ ! -f "$env" ]; then
        return 0
    fi

    # Check for running containers
    local running=false
    if have docker && docker compose version >/dev/null 2>&1; then
        if (cd "$infra_dir" && docker compose -f compose.yml --env-file "$env" ps --services --filter "status=running" 2>/dev/null | grep -q .); then
            running=true
        fi
    fi

    if [ "$FORCE_REINSTALL" = "true" ]; then
        warn "Force-reinstalling over existing installation"
        INSTALL_EXISTING=true
        return 0
    fi

    header "Existing Installation Detected"
    echo ""
    echo "  Environment found at: $env"
    if [ "$running" = "true" ]; then
        echo "  Running containers detected"
    fi
    echo ""

    if [ "$UNATTENDED" = "true" ]; then
        warn "Existing installation found in unattended mode — aborting"
        echo "  Use --force to overwrite."
        exit 1
    fi

    echo "  Options:"
    echo "    [U] Update   — Restart services with existing configuration"
    echo "    [R] Reinstall — Generate new configuration and start fresh"
    echo "    [A] Abort    — Exit without changes (default)"
    echo ""
    printf "  Choose [u/R/a]: "
    read -r choice
    case "${choice:-a}" in
        [Uu]*)
            INSTALL_EXISTING=true
            ENV_FILE="$env"
            COMPOSE_CMD=(docker compose -f compose.yml -f compose.production.yml --env-file "$ENV_FILE")
            COMPOSE_FILES=(-f compose.yml -f compose.production.yml)
            detail "Updating existing installation..."
            cd "$infra_dir"
            "${COMPOSE_CMD[@]}" pull
            "${COMPOSE_CMD[@]}" up -d --build
            "${COMPOSE_CMD[@]}" ps
            info "Update complete"
            print_summary
            exit 0
            ;;
        [Rr]*)
            INSTALL_EXISTING=true
            detail "Reinstalling in place..."
            ;;
        *)
            echo "  Aborted."
            exit 0
            ;;
    esac
}

# ============================================================
# Main Entry Point
# ============================================================
main() {
    trap cleanup_on_interrupt INT TERM

    parse_args "$@"

    echo ""
    echo -e "  ${RED}${BOLD}+============================================================+${NC}"
    echo -e "  ${RED}${BOLD}|           GamePanel · Forge Control Plane                    |${NC}"
    echo -e "  ${RED}${BOLD}|           Production Installation Script                    |${NC}"
    echo -e "  ${RED}${BOLD}+============================================================+${NC}"
    echo ""

    # Detect existing installation before doing work
    detect_existing

    if [ "$SKIP_CHECKS" = "false" ]; then
        detect_os
        detect_arch
        check_docker
        check_ports
        check_resources
    else
        warn "Skipping pre-flight checks (--skip-checks)"
    fi

    configure_interactive
    generate_environment
    start_services
    verify_installation
    print_summary

    echo -e "  ${GREEN}${BOLD}Installation completed at $(date -u +"%Y-%m-%dT%H:%M:%SZ")${NC}"
    echo ""
}

main "$@"
