#!/usr/bin/env bash
# ============================================================
# GamePanel Forge Installation Script
# 
# This script provides comprehensive installation and setup for
# the GamePanel Forge control plane on Linux systems.
#
# Usage:
#   Interactive:  ./install.sh
#   Unattended:   GAMEPANEL_ADMIN_PASSWORD=... GAMEPANEL_DB_PASSWORD=... \
#                 ./install.sh --unattended --fqdn panel.example.com --email admin@example.com
#   (Passwords are taken from the environment or from hidden `read -s` prompts —
#    never from command-line arguments, which are world-readable via ps/procfs.)
# ============================================================

set -euo pipefail

# --- Constants ---
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly INSTALL_DIR="/opt/gamepanel"
readonly DATA_DIR="/var/lib/gamepanel"
readonly CONFIG_DIR="/etc/gamepanel"
readonly LOG_DIR="/var/log/gamepanel"

# Minimum requirements
readonly MIN_DOCKER_VERSION=24
readonly MIN_DOCKER_COMPOSE_VERSION=2
readonly MIN_RAM_MB=1900
readonly MIN_DISK_GB=20
readonly MIN_CPUS=2

# Supported operating systems
readonly SUPPORTED_OS=("Ubuntu 22.04" "Ubuntu 24.04" "Debian 12" "CentOS 7" "CentOS 8" "CentOS 9")

# Default configuration
DEFAULT_FQDN=""
DEFAULT_ADMIN_EMAIL=""
# Credentials are seeded from the environment, NOT from argv: a command-line
# argument is visible to every account on the host through `ps -e` and
# /proc/<pid>/cmdline, and lands in shell history and CI logs. These are the
# same keys the generated $CONFIG_DIR/.env uses, so a re-run can simply
# `set -a; . /etc/gamepanel/.env; set +a`.
DEFAULT_ADMIN_PASSWORD="${GAMEPANEL_ADMIN_PASSWORD:-}"
DEFAULT_DB_PASSWORD="${GAMEPANEL_DB_PASSWORD:-}"

# Installation flags
UNATTENDED=false
SKIP_CHECKS=false
FORCE_REINSTALL=false
VERBOSE=false

# --- Colors ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    PURPLE='\033[0;35m'
    CYAN='\033[0;36m'
    WHITE='\033[0;37m'
    BOLD='\033[1m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    PURPLE=''
    CYAN=''
    WHITE=''
    BOLD=''
    NC=''
fi

# --- Logging Functions ---
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_header() {
    echo -e "\n${BOLD}${CYAN}=== $1 ===${NC}"
}

log_step() {
    echo -e "${BOLD}${BLUE}→ $1${NC}"
}

# --- Argument Parsing ---
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --unattended)
                UNATTENDED=true
                shift
                ;;
            --skip-checks)
                SKIP_CHECKS=true
                shift
                ;;
            --force)
                FORCE_REINSTALL=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                set -x
                shift
                ;;
            --fqdn)
                DEFAULT_FQDN="$2"
                shift 2
                ;;
            --email)
                DEFAULT_ADMIN_EMAIL="$2"
                shift 2
                ;;
            --password|--db-password)
                log_error "'$1' has been removed: secrets passed as arguments are readable by any local user (ps/proc) and leak into shell history and CI logs."
                log_error "Set GAMEPANEL_ADMIN_PASSWORD and/or GAMEPANEL_DB_PASSWORD in the environment instead,"
                log_error "or drop --unattended and answer the hidden (read -s) prompts."
                exit 1
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

show_help() {
    cat << EOF
GamePanel Forge Installation Script

Usage:
  Interactive:   ./install.sh
  Unattended:   GAMEPANEL_ADMIN_PASSWORD=... GAMEPANEL_DB_PASSWORD=... ./install.sh [OPTIONS]

Options:
  --unattended       Run in non-interactive mode
  --skip-checks      Skip pre-flight checks (not recommended)
  --force            Force reinstall over existing installation
  --verbose          Show verbose output
  --fqdn            Set the FQDN for the panel (required for unattended)
  --email           Set the admin email (required for unattended)
  --help, -h        Show this help message

Environment variables (used instead of password flags, which were removed):
  GAMEPANEL_ADMIN_PASSWORD   Admin password for unattended runs
  GAMEPANEL_DB_PASSWORD      Database password for unattended runs
  When unattended and unset, the installer prompts with a hidden read -s if a
  TTY is attached; with no TTY it fails rather than accept a secret via argv.

Requirements:
  - Docker ${MIN_DOCKER_VERSION}+ and Docker Compose ${MIN_DOCKER_COMPOSE_VERSION}+
  - ${MIN_RAM_MB}MB RAM, ${MIN_DISK_GB}GB disk space, ${MIN_CPUS} CPUs
  - Supported OS: ${SUPPORTED_OS[*]}
  - Root or sudo access

Examples:
  # Interactive installation (passwords entered at hidden prompts)
  ./install.sh

  # Unattended installation — credentials come from the environment
  GAMEPANEL_ADMIN_PASSWORD='...' GAMEPANEL_DB_PASSWORD='...' \\
    ./install.sh --unattended \\
    --fqdn panel.example.com \\
    --email admin@example.com
EOF
}

# --- Pre-flight Checks ---
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi
}

check_os() {
    log_step "Checking operating system"
    
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME="$NAME"
        OS_VERSION="$VERSION_ID"
    elif type lsb_release >/dev/null 2>&1; then
        OS_NAME=$(lsb_release -si)
        OS_VERSION=$(lsb_release -sr)
    elif [ -f /etc/redhat-release ]; then
        OS_NAME="Red Hat"
        OS_VERSION=$(cat /etc/redhat-release)
    else
        OS_NAME="Unknown"
        OS_VERSION="Unknown"
    fi

    local supported=false
    for os in "${SUPPORTED_OS[@]}"; do
        if [[ "$OS_NAME $OS_VERSION" == *"$os"* ]]; then
            supported=true
            break
        fi
    done

    if [ "$supported" = false ]; then
        log_warn "Unsupported operating system: $OS_NAME $OS_VERSION"
        log_warn "Supported systems: ${SUPPORTED_OS[*]}"
        if [ "$SKIP_CHECKS" = false ]; then
            log_error "Aborting installation"
            exit 1
        fi
    else
        log_info "Operating system: $OS_NAME $OS_VERSION"
    fi
}

check_docker() {
    log_step "Checking Docker installation"
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        exit 1
    fi

    local docker_version
    docker_version=$(docker --version | awk '{print $3}' | cut -d'.' -f1)
    
    if [[ $docker_version -lt $MIN_DOCKER_VERSION ]]; then
        log_error "Docker version ${docker_version} is too old. Minimum required: ${MIN_DOCKER_VERSION}"
        exit 1
    fi

    log_info "Docker version: $(docker --version)"
}

check_docker_compose() {
    log_step "Checking Docker Compose installation"

    # Docker Compose v2 (`docker compose`) is required; v1 `docker-compose`
    # is EOL. Accept v1 only as a legacy fallback warning.
    if docker compose version &> /dev/null; then
        local compose_version
        compose_version=$(docker compose version | awk '{print $4}')
        log_info "Docker Compose version: $compose_version"
        return
    fi

    if command -v docker-compose &> /dev/null; then
        log_warn "Found legacy docker-compose v1 only; install the v2 plugin (docker-compose-plugin)"
        log_info "Docker Compose version: $(docker-compose --version)"
        return
    fi

    log_error "Docker Compose is not installed (need 'docker compose' v2)"
    exit 1
}

check_resources() {
    log_step "Checking system resources"
    
    # Check RAM
    local total_ram_kb
    total_ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    local total_ram_mb=$((total_ram_kb / 1024))
    
    if [[ $total_ram_mb -lt $MIN_RAM_MB ]]; then
        log_error "Insufficient RAM: ${total_ram_mb}MB (minimum: ${MIN_RAM_MB}MB)"
        exit 1
    fi
    log_info "RAM: ${total_ram_mb}MB"

    # Check disk space (integer GB — df reports KiB; float output would break -lt)
    local disk_space_gb
    disk_space_gb=$(df / --output=size | tail -1 | awk '{printf "%d", $1 / 1024 / 1024}')
    
    if [[ $disk_space_gb -lt $MIN_DISK_GB ]]; then
        log_error "Insufficient disk space: ${disk_space_gb}GB (minimum: ${MIN_DISK_GB}GB)"
        exit 1
    fi
    log_info "Disk space: ${disk_space_gb}GB"

    # Check CPUs
    local cpu_count
    cpu_count=$(nproc --all)
    
    if [[ $cpu_count -lt $MIN_CPUS ]]; then
        log_error "Insufficient CPUs: ${cpu_count} (minimum: ${MIN_CPUS})"
        exit 1
    fi
    log_info "CPUs: $cpu_count"
}

# --- Port probe (ss || netstat || /dev/tcp fallback) ---
port_in_use() {
    local port=$1
    if command -v ss >/dev/null 2>&1; then
        ss -tln 2>/dev/null | grep -q ":${port} "
        return $?
    fi
    if command -v netstat >/dev/null 2>&1; then
        netstat -tln 2>/dev/null | grep -q "[:.]${port} "
        return $?
    fi
    # Bash /dev/tcp fallback — no external dependency.
    (echo >"/dev/tcp/127.0.0.1/${port}") >/dev/null 2>&1
}

check_ports() {
    log_step "Checking required ports"

    # Full stack footprint: edge (80/443), API (8080), beacon (9090),
    # web (3000), postgres (5432), redis (6379), SFTP (2022), grafana
    # (3001), prometheus (9091 host-mapped), alertmanager (9093 host-mapped).
    local required_ports=(80 443 8080 9090 3000 5432 6379 2022 3001 9091 9093)
    local available=true

    for port in "${required_ports[@]}"; do
        if port_in_use "$port"; then
            log_warn "Port $port is already in use"
            available=false
        fi
    done

    if [ "$available" = false ]; then
        log_warn "Some required ports are already in use"
        if [ "$SKIP_CHECKS" = false ]; then
            if [ ! -t 0 ]; then
                log_error "Ports in use and no TTY to confirm. Free the ports or re-run with --skip-checks."
                exit 1
            fi
            read -p "Continue anyway? [y/N]: " -n 1 -r || REPLY="n"
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                exit 1
            fi
        fi
    else
        log_info "All required ports are available"
    fi
}

check_existing_installation() {
    log_step "Checking for existing installation"
    
    if [ -d "$INSTALL_DIR" ] || [ -d "$DATA_DIR" ] || [ -d "$CONFIG_DIR" ]; then
        if [ "$FORCE_REINSTALL" = false ]; then
            log_warn "Existing installation detected"
            if [ ! -t 0 ]; then
                log_error "Existing installation found and no TTY to confirm. Re-run with --force or remove it first."
                exit 1
            fi
            read -p "Reinstall over existing installation? [y/N]: " -n 1 -r || REPLY="n"
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "Aborting installation"
                exit 0
            fi
        else
            log_info "Forcing reinstall over existing installation"
        fi
    fi
}

# --- Hidden Credential Prompts ---
# Collects a secret straight from the terminal: it never appears in argv (which
# any local user can read through ps(1) / /proc/<pid>/cmdline), is not echoed,
# and is not written to the logs. When a confirmation label is given the entry
# must be repeated before it is accepted.
prompt_secret() {
    local var_name="$1" label="$2" confirm_label="${3:-}"
    local value confirm
    while true; do
        if ! IFS= read -r -s -p "Enter ${label}: " value; then
            echo >&2
            log_error "No input available while asking for the ${label}; aborting instead of looping."
            return 1
        fi
        echo >&2
        if [ -z "$value" ]; then
            log_error "${label} cannot be empty"
            continue
        fi
        if [ -n "$confirm_label" ]; then
            if ! IFS= read -r -s -p "Confirm ${label}: " confirm; then
                echo >&2
                log_error "No input available while confirming the ${label}; aborting."
                return 1
            fi
            echo >&2
            if [ "$value" != "$confirm" ]; then
                log_error "${label}s do not match"
                continue
            fi
        fi
        printf -v "$var_name" '%s' "$value"
        return 0
    done
}

# --- Interactive Input ---
gather_input() {
    if [ "$UNATTENDED" = true ]; then
        # Validate required parameters for unattended mode
        if [ -z "$DEFAULT_FQDN" ]; then
            log_error "FQDN is required for unattended installation (use --fqdn)"
            exit 1
        fi
        
        if [ -z "$DEFAULT_ADMIN_EMAIL" ]; then
            log_error "Admin email is required for unattended installation (use --email)"
            exit 1
        fi
        
        # No credential may arrive as a command-line argument. Take it from the
        # environment; if that is empty, fall back to a hidden prompt when a TTY
        # is attached, and fail closed when it is not.
        if [ -z "$DEFAULT_ADMIN_PASSWORD" ]; then
            if [ -t 0 ]; then
                log_warn "GAMEPANEL_ADMIN_PASSWORD is not set — prompting (input hidden)"
                prompt_secret DEFAULT_ADMIN_PASSWORD "admin password" ""
            else
                log_error "Admin password is required for unattended installation."
                log_error "Export GAMEPANEL_ADMIN_PASSWORD (do not pass it as an argument)."
                exit 1
            fi
        fi
        
        if [ -z "$DEFAULT_DB_PASSWORD" ]; then
            if [ -t 0 ]; then
                log_warn "GAMEPANEL_DB_PASSWORD is not set — prompting (input hidden)"
                prompt_secret DEFAULT_DB_PASSWORD "database password" ""
            else
                log_error "Database password is required for unattended installation."
                log_error "Export GAMEPANEL_DB_PASSWORD (do not pass it as an argument)."
                exit 1
            fi
        fi
        
        FQDN="$DEFAULT_FQDN"
        ADMIN_EMAIL="$DEFAULT_ADMIN_EMAIL"
        ADMIN_PASSWORD="$DEFAULT_ADMIN_PASSWORD"
        DB_PASSWORD="$DEFAULT_DB_PASSWORD"
        
        return
    fi

    # Interactive mode
    echo
    
    # Get FQDN (initialise defensively — these are unset under `set -u` otherwise)
    FQDN="${FQDN:-$DEFAULT_FQDN}"
    while [ -z "$FQDN" ]; do
        read -p "Enter the FQDN for your panel (e.g., panel.example.com): " FQDN
        if [ -z "$FQDN" ]; then
            log_error "FQDN cannot be empty"
        fi
    done

    # Get admin email
    ADMIN_EMAIL="${ADMIN_EMAIL:-$DEFAULT_ADMIN_EMAIL}"
    while [ -z "$ADMIN_EMAIL" ]; do
        read -p "Enter admin email: " ADMIN_EMAIL
        if [ -z "$ADMIN_EMAIL" ]; then
            log_error "Admin email cannot be empty"
        fi
    done

    # Get admin password — a value supplied through the environment is reused,
    # anything else is collected with a hidden, confirmed prompt.
    ADMIN_PASSWORD="${ADMIN_PASSWORD:-$DEFAULT_ADMIN_PASSWORD}"
    if [ -z "$ADMIN_PASSWORD" ]; then
        prompt_secret ADMIN_PASSWORD "admin password" "admin password"
    fi

    # Get database password (environment first, hidden confirmed prompt otherwise)
    DB_PASSWORD="${DB_PASSWORD:-$DEFAULT_DB_PASSWORD}"
    if [ -z "$DB_PASSWORD" ]; then
        prompt_secret DB_PASSWORD "database password" "database password"
    fi

    # The interactive confirmation is performed inside prompt_secret().
}

# --- Installation Functions ---
create_directories() {
    log_step "Creating directories"
    
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$DATA_DIR"
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$LOG_DIR"
    
    chmod 750 "$INSTALL_DIR"
    chmod 750 "$DATA_DIR"
    chmod 750 "$CONFIG_DIR"
    chmod 755 "$LOG_DIR"
    
    log_info "Directories created"
}

generate_configuration() {
    log_step "Generating configuration"

    # Service secrets: generated once per install, never logged. Reuse
    # existing values when re-running against $CONFIG_DIR/.env so a
    # reinstall does not invalidate encrypted data.
    if [ -f "$CONFIG_DIR/.env" ]; then
        # Restricted load: bare KEY=VALUE allowlist only, never sourced.
        while IFS= read -r line || [ -n "$line" ]; do
            line="$(printf '%s' "$line" | tr -d '\r')"
            case "$line" in ''|'#'*) continue ;; esac
            case "$line" in *"="*) ;; *) continue ;; esac
            _k="${line%%=*}" _v="${line#*=}"
            case "$_k" in API_AUTH_SECRET|APP_KEY|FORGE_MASTER_KEY|FORGE_TAG) ;;
                *) continue ;;
            esac
            case "$_v" in '"'*'"') _v="${_v#\"}"; _v="${_v%\"}" ;;
                "'"*"'") _v="${_v#\'}"; _v="${_v%\'}";;
            esac
            printf -v "$_k" '%s' "$_v"
        done < "$CONFIG_DIR/.env"
    fi
    : "${API_AUTH_SECRET:=$(openssl rand -base64 32 | tr -d '\n')}"
    : "${APP_KEY:=base64:$(openssl rand -base64 32 | tr -d '\n')}"
    : "${FORGE_MASTER_KEY:=$(openssl rand -base64 32 | tr -d '\n')}"
    : "${FORGE_TAG:=${FORGE_RELEASE_TAG:-v1.0.0}}"
    export API_AUTH_SECRET APP_KEY FORGE_MASTER_KEY FORGE_TAG

    # Secrets are shell-quoted with %q so values containing spaces, $, quotes
    # or newlines survive a later `set -a; . /etc/gamepanel/.env` re-source.
    # Never log or echo these values.
    {
        printf '# GamePanel Forge Configuration\n'
        printf 'GAMEPANEL_FQDN=%q\n' "$FQDN"
        printf 'GAMEPANEL_ADMIN_EMAIL=%q\n' "$ADMIN_EMAIL"
        printf 'GAMEPANEL_ADMIN_PASSWORD=%q\n' "$ADMIN_PASSWORD"
        printf 'GAMEPANEL_DB_PASSWORD=%q\n' "$DB_PASSWORD"
        printf 'API_AUTH_SECRET=%q\n' "$API_AUTH_SECRET"
        printf 'APP_KEY=%q\n' "$APP_KEY"
        printf 'FORGE_MASTER_KEY=%q\n' "$FORGE_MASTER_KEY"
        printf 'FORGE_MASTER_KEY_ID=%q\n' "primary"
        printf 'FORGE_TAG=%q\n' "$FORGE_TAG"
        cat << 'STATIC'
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME=gamepanel
DB_USER=gamepanel

# Docker Configuration
DOCKER_REGISTRY=ghcr.io
DOCKER_NETWORK=gamepanel_network

# Application Configuration
STATIC
        printf 'APP_URL=%q\n' "https://$FQDN"
        printf 'APP_TIMEZONE=%q\n' "UTC"
        printf 'APP_DEBUG=%q\n' "false"
    } > "$CONFIG_DIR/.env"

    chmod 600 "$CONFIG_DIR/.env"
    log_info "Configuration generated"
}

generate_docker_compose() {
    log_step "Generating Docker Compose configuration"

    # Unquoted EOF: installer values (FQDN, DB_PASSWORD) are baked in at
    # generate time. Secrets are double-quoted in YAML so special characters
    # ($, :, #, spaces) do not break parsing; embedded double-quotes and
    # backslashes are escaped first.
    # Images are pinned to $FORGE_TAG (never :latest) for reproducible
    # installs. Internal ports bind loopback-only; the public entrypoint is
    # the nginx proxy on 80/443.
    local db_pw_esc fqdn_esc api_secret_esc app_key_esc master_key_esc tag_esc
    db_pw_esc=${DB_PASSWORD//\\/\\\\}
    db_pw_esc=${db_pw_esc//\"/\\\"}
    fqdn_esc=${FQDN//\\/\\\\}
    fqdn_esc=${fqdn_esc//\"/\\\"}
    api_secret_esc=${API_AUTH_SECRET//\\/\\\\}
    api_secret_esc=${api_secret_esc//\"/\\\"}
    app_key_esc=${APP_KEY//\\/\\\\}
    app_key_esc=${app_key_esc//\"/\\\"}
    master_key_esc=${FORGE_MASTER_KEY//\\/\\\\}
    master_key_esc=${master_key_esc//\"/\\\"}
    tag_esc=${FORGE_TAG//\\/\\\\}
    tag_esc=${tag_esc//\"/\\\"}

    cat > "$INSTALL_DIR/docker-compose.yml" << EOF
version: '3.8'

services:
  api:
    image: ghcr.io/gamepanel/forge-api:"$tag_esc"
    container_name: gamepanel-api
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - gamepanel_data:/data
      - gamepanel_config:/config
    environment:
      - TZ=UTC
      - PUID=1000
      - PGID=1000
      - API_AUTH_SECRET="$api_secret_esc"
      - APP_KEY="$app_key_esc"
      - FORGE_MASTER_KEY="$master_key_esc"
      - FORGE_MASTER_KEY_ID=primary
      - APP_ENV=production
      - DATABASE_URL=postgres://gamepanel:$db_pw_esc@database:5432/gamepanel?sslmode=disable
    networks:
      - gamepanel_network
    depends_on:
      - database
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/api/v1/health/ready > /dev/null 2>&1 || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL

  database:
    image: postgres:15-alpine
    container_name: gamepanel-db
    restart: unless-stopped
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - gamepanel_db_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_DB=gamepanel
      - POSTGRES_USER=gamepanel
      - POSTGRES_PASSWORD="$db_pw_esc"
      - TZ=UTC
    networks:
      - gamepanel_network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gamepanel -d gamepanel"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    cap_add:
      - CHOWN
      - DAC_OVERRIDE
      - FOWNER
      - SETGID
      - SETUID

  web:
    image: ghcr.io/gamepanel/forge-web:"$tag_esc"
    container_name: gamepanel-web
    restart: unless-stopped
    ports:
      - "127.0.0.1:3000:3000"
    volumes:
      - gamepanel_data:/data
    environment:
      - API_URL=http://api:8080
      - APP_URL=https://$fqdn_esc
      - TZ=UTC
    networks:
      - gamepanel_network
    depends_on:
      - api
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:3000/ > /dev/null 2>&1 || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL

  proxy:
    image: nginx:alpine
    container_name: gamepanel-proxy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    networks:
      - gamepanel_network
    depends_on:
      - api
      - web
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- https://127.0.0.1/ -k > /dev/null 2>&1 || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    cap_add:
      - CHOWN
      - DAC_OVERRIDE
      - NET_BIND_SERVICE
      - SETGID
      - SETUID

networks:
  gamepanel_network:
    driver: bridge

volumes:
  gamepanel_data:
    driver: local
  gamepanel_config:
    driver: local
  gamepanel_db_data:
    driver: local
EOF

    log_info "Docker Compose configuration generated (images pinned to $FORGE_TAG, loopback-only internal ports)"
}

ensure_ssl_certs() {
    log_step "Ensuring TLS certificates"
    # The nginx proxy mounts ./ssl and requires fullchain.pem + privkey.pem.
    # With no certs the proxy crash-loops and verify_installation fails on a
    # missing gamepanel-proxy. Generate a self-signed pair as a bootstrap so
    # the stack comes up; replace with real (Let's Encrypt) certs after.
    mkdir -p "$INSTALL_DIR/ssl"
    if [ -f "$INSTALL_DIR/ssl/fullchain.pem" ] && [ -f "$INSTALL_DIR/ssl/privkey.pem" ]; then
        log_info "TLS certificates already present in $INSTALL_DIR/ssl/"
        return 0
    fi
    if ! command -v openssl >/dev/null 2>&1; then
        log_error "openssl is required to bootstrap TLS certificates"
        exit 1
    fi
    openssl req -x509 -newkey rsa:2048 -sha256 -days 90 -nodes \
        -keyout "$INSTALL_DIR/ssl/privkey.pem" \
        -out "$INSTALL_DIR/ssl/fullchain.pem" \
        -subj "/CN=${FQDN}" \
        -addext "subjectAltName=DNS:${FQDN}" 2>/dev/null
    chmod 600 "$INSTALL_DIR/ssl/privkey.pem"
    chmod 644 "$INSTALL_DIR/ssl/fullchain.pem"
    log_warn "Self-signed certificate generated for ${FQDN} (bootstrap only)"
    log_warn "Replace $INSTALL_DIR/ssl/ with real certificates, then: cd $INSTALL_DIR && docker compose restart proxy"
}

generate_nginx_config() {
    log_step "Generating Nginx configuration"

    cat > "$INSTALL_DIR/nginx.conf" << EOF
worker_processes auto;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # Rate limiting: generic API bucket + strict auth bucket.
    limit_req_zone \$binary_remote_addr zone=api:10m rate=10r/s;
    limit_req_zone \$binary_remote_addr zone=auth:10m rate=5r/m;
    limit_req_status 429;

    upstream api {
        server api:8080;
    }

    upstream web {
        server web:3000;
    }

    server {
        listen 80;
        listen [::]:80;
        server_name $FQDN;

        # Redirect HTTP to HTTPS
        return 301 https://\$host\$request_uri;
    }

    server {
        listen 443 ssl http2;
        listen [::]:443 ssl http2;
        server_name $FQDN;

        ssl_certificate /etc/nginx/ssl/fullchain.pem;
        ssl_certificate_key /etc/nginx/ssl/privkey.pem;
        ssl_session_timeout 1d;
        ssl_session_cache shared:SSL:50m;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305;
        ssl_prefer_server_ciphers on;

        # Security headers (HSTS + CSP included; X-XSS-Protection disabled —
        # legacy header that re-enables exploitable XSS auditors).
        add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
        add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' wss: ws:; frame-ancestors 'none'" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "0" always;
        add_header Referrer-Policy "strict-origin-when-cross-origin" always;
        add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;

        client_max_body_size 64m;

        location / {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://web;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_set_header Upgrade \$http_upgrade;
            proxy_set_header Connection "upgrade";
        }

        location /api/ {
            limit_req zone=api burst=20 nodelay;
            proxy_pass http://api;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
        }

        location /api/v1/auth/ {
            limit_req zone=auth burst=5 nodelay;
            proxy_pass http://api;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
        }

        location /ws/ {
            proxy_pass http://api;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_set_header Upgrade \$http_upgrade;
            proxy_set_header Connection "upgrade";
        }
    }
}
EOF

    log_info "Nginx configuration generated"
}

start_services() {
    log_step "Starting services"

    cd "$INSTALL_DIR"

    # Docker Compose v2 only (`docker compose`); the standalone
    # `docker-compose` v1 binary is EOL and must not be used.
    if ! docker compose version >/dev/null 2>&1; then
        log_error "Docker Compose v2 is required (docker compose version)"
        exit 1
    fi

    # Pull the latest images
    log_info "Pulling Docker images..."
    docker compose pull

    # Start the services
    log_info "Starting containers..."
    docker compose up -d

    # Wait for services to be healthy
    log_info "Waiting for services to start..."
    sleep 10

    # Check service status
    docker compose ps
}

verify_installation() {
    log_step "Verifying installation"

    # Assert all four expected containers by exact name — a bare count can
    # pass while the wrong set is running.
    local expected_containers=(gamepanel-api gamepanel-db gamepanel-web gamepanel-proxy)
    local missing=0 name
    for name in "${expected_containers[@]}"; do
        if docker ps --filter "name=^${name}$" --format "{{.Names}}" | grep -qx "$name"; then
            log_info "Container running: $name"
        else
            log_error "Container not running: $name"
            missing=$((missing + 1))
        fi
    done

    if [[ $missing -gt 0 ]]; then
        log_error "$missing of ${#expected_containers[@]} expected containers are not running"
        docker compose logs --tail=100
        exit 1
    fi

    log_info "All containers are running"

    # Test API connectivity (readiness, not just liveness)
    local api_health
    api_health=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/health/ready || echo "000")
    
    if [[ "$api_health" != "200" ]]; then
        log_warn "API health check failed (HTTP $api_health)"
    else
        log_info "API health check passed"
    fi
    
    # Test web connectivity
    local web_health
    web_health=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 || echo "000")

    if [[ "$web_health" != "200" ]]; then
        log_warn "Web health check failed (HTTP $web_health)"
    else
        log_info "Web health check passed"
    fi

    # Test proxy TLS (self-signed bootstrap cert is expected on first install;
    # -k skips chain verification, failure here is a warning, not fatal).
    local proxy_health
    proxy_health=$(curl -sk -o /dev/null -w "%{http_code}" https://localhost/ || echo "000")

    if [[ "$proxy_health" != "200" && "$proxy_health" != "404" && "$proxy_health" != "502" ]]; then
        log_warn "Proxy health check failed (HTTPS $proxy_health)"
    else
        log_info "Proxy health check passed (HTTPS $proxy_health)"
    fi
}

show_summary() {
    log_header "Installation Summary"
    
    echo ""
    echo "GamePanel Forge has been installed successfully!"
    echo ""
    echo "📋 Configuration:"
    echo "   FQDN:       $FQDN"
    echo "   Admin Email: $ADMIN_EMAIL"
    echo ""
    echo "🚀 Services:"
    echo "   API:        http://$FQDN/api"
    echo "   Web:       http://$FQDN"
    echo "   Database:   localhost:5432"
    echo ""
    echo "📁 Directories:"
    echo "   Install:    $INSTALL_DIR"
    echo "   Data:       $DATA_DIR"
    echo "   Config:     $CONFIG_DIR"
    echo "   Logs:       $LOG_DIR"
    echo ""
    echo "🔧 Management Commands:"
    echo "   Start:      cd $INSTALL_DIR && docker compose up -d"
    echo "   Stop:       cd $INSTALL_DIR && docker compose down"
    echo "   Restart:    cd $INSTALL_DIR && docker compose restart"
    echo "   Logs:       cd $INSTALL_DIR && docker compose logs -f"
    echo "   Update:     cd $INSTALL_DIR && docker compose pull && docker compose up -d"
    echo ""
    echo "⚠️  Important Notes:"
    echo "   - A self-signed TLS bootstrap cert was generated in $INSTALL_DIR/ssl/"
    echo "   - Replace it with real certificates, then: cd $INSTALL_DIR && docker compose restart proxy"
    echo "   - Default credentials: $ADMIN_EMAIL / [your password]"
    echo ""
}

# --- Main Installation Function ---
main() {
    parse_arguments "$@"
    
    log_header "GamePanel Forge Installation"
    
    # Run pre-flight checks
    if [ "$SKIP_CHECKS" = false ]; then
        check_root
        check_os
        check_docker
        check_docker_compose
        check_resources
        check_ports
        check_existing_installation
    else
        log_warn "Skipping pre-flight checks"
    fi
    
    # Gather input
    gather_input
    
    # Perform installation
    create_directories
    generate_configuration
    generate_docker_compose
    ensure_ssl_certs
    generate_nginx_config
    start_services
    verify_installation
    
    # Show summary
    show_summary
    
    log_header "Installation Complete"
}

# Run main function with all arguments
main "$@"
