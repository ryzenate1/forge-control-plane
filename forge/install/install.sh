#!/usr/bin/env bash
# ============================================================
# GamePanel Forge Installation Script
# 
# This script provides comprehensive installation and setup for
# the GamePanel Forge control plane on Linux systems.
#
# Usage:
#   Interactive:  ./install.sh
#   Unattended:   ./install.sh --unattended --fqdn panel.example.com --email admin@example.com
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
DEFAULT_ADMIN_PASSWORD=""
DEFAULT_DB_PASSWORD=""

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
            --password)
                DEFAULT_ADMIN_PASSWORD="$2"
                shift 2
                ;;
            --db-password)
                DEFAULT_DB_PASSWORD="$2"
                shift 2
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
  Unattended:   ./install.sh [OPTIONS]

Options:
  --unattended       Run in non-interactive mode
  --skip-checks      Skip pre-flight checks (not recommended)
  --force            Force reinstall over existing installation
  --verbose          Show verbose output
  --fqdn            Set the FQDN for the panel (required for unattended)
  --email           Set the admin email (required for unattended)
  --password        Set the admin password (required for unattended)
  --db-password     Set the database password (required for unattended)
  --help, -h        Show this help message

Requirements:
  - Docker ${MIN_DOCKER_VERSION}+ and Docker Compose ${MIN_DOCKER_COMPOSE_VERSION}+
  - ${MIN_RAM_MB}MB RAM, ${MIN_DISK_GB}GB disk space, ${MIN_CPUS} CPUs
  - Supported OS: ${SUPPORTED_OS[*]}
  - Root or sudo access

Examples:
  # Interactive installation
  ./install.sh

  # Unattended installation
  ./install.sh --unattended \\
    --fqdn panel.example.com \\
    --email admin@example.com \\
    --password MySecurePass123 \\
    --db-password MyDBPass123
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
    
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose is not installed"
        exit 1
    fi

    local compose_version
    if command -v docker-compose &> /dev/null; then
        compose_version=$(docker-compose --version | awk '{print $3}')
    else
        compose_version=$(docker compose version | awk '{print $4}')
    fi

    log_info "Docker Compose version: $compose_version"
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

    # Check disk space
    local disk_space_gb
    disk_space_gb=$(df / --output=size | tail -1 | awk '{print $1 / 1024 / 1024}')
    
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

check_ports() {
    log_step "Checking required ports"
    
    local required_ports=(80 443 8080 9090 3000)
    local available=true
    
    for port in "${required_ports[@]}"; do
        if ss -tlnp | grep -q ":$port "; then
            log_warn "Port $port is already in use"
            available=false
        fi
    done

    if [ "$available" = false ]; then
        log_warn "Some required ports are already in use"
        if [ "$SKIP_CHECKS" = false ]; then
            read -p "Continue anyway? [y/N]: " -n 1 -r
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
            read -p "Reinstall over existing installation? [y/N]: " -n 1 -r
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
        
        if [ -z "$DEFAULT_ADMIN_PASSWORD" ]; then
            log_error "Admin password is required for unattended installation (use --password)"
            exit 1
        fi
        
        if [ -z "$DEFAULT_DB_PASSWORD" ]; then
            log_error "Database password is required for unattended installation (use --db-password)"
            exit 1
        fi
        
        FQDN="$DEFAULT_FQDN"
        ADMIN_EMAIL="$DEFAULT_ADMIN_EMAIL"
        ADMIN_PASSWORD="$DEFAULT_ADMIN_PASSWORD"
        DB_PASSWORD="$DEFAULT_DB_PASSWORD"
        
        return
    fi

    # Interactive mode
    echo
    
    # Get FQDN
    while [ -z "$FQDN" ]; do
        read -p "Enter the FQDN for your panel (e.g., panel.example.com): " FQDN
        if [ -z "$FQDN" ]; then
            log_error "FQDN cannot be empty"
        fi
    done

    # Get admin email
    while [ -z "$ADMIN_EMAIL" ]; do
        read -p "Enter admin email: " ADMIN_EMAIL
        if [ -z "$ADMIN_EMAIL" ]; then
            log_error "Admin email cannot be empty"
        fi
    done

    # Get admin password
    while [ -z "$ADMIN_PASSWORD" ]; do
        read -s -p "Enter admin password: " ADMIN_PASSWORD
        echo
        if [ -z "$ADMIN_PASSWORD" ]; then
            log_error "Admin password cannot be empty"
        fi
    done

    # Confirm admin password
    while true; do
        read -s -p "Confirm admin password: " ADMIN_PASSWORD_CONFIRM
        echo
        if [ "$ADMIN_PASSWORD" = "$ADMIN_PASSWORD_CONFIRM" ]; then
            break
        else
            log_error "Passwords do not match"
        fi
    done

    # Get database password
    while [ -z "$DB_PASSWORD" ]; do
        read -s -p "Enter database password: " DB_PASSWORD
        echo
        if [ -z "$DB_PASSWORD" ]; then
            log_error "Database password cannot be empty"
        fi
    done

    # Confirm database password
    while true; do
        read -s -p "Confirm database password: " DB_PASSWORD_CONFIRM
        echo
        if [ "$DB_PASSWORD" = "$DB_PASSWORD_CONFIRM" ]; then
            break
        else
            log_error "Database passwords do not match"
        fi
    done
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
    
    # Generate .env file
    cat > "$CONFIG_DIR/.env" << EOF
# GamePanel Forge Configuration
GAMEPANEL_FQDN=$FQDN
GAMEPANEL_ADMIN_EMAIL=$ADMIN_EMAIL
GAMEPANEL_ADMIN_PASSWORD=$ADMIN_PASSWORD
GAMEPANEL_DB_PASSWORD=$DB_PASSWORD

# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME=gamepanel
DB_USER=gamepanel

# Docker Configuration
DOCKER_REGISTRY=ghcr.io
DOCKER_NETWORK=gamepanel_network

# Application Configuration
APP_URL=https://$FQDN
APP_TIMEZONE=UTC
APP_DEBUG=false
EOF

    chmod 600 "$CONFIG_DIR/.env"
    log_info "Configuration generated"
}

generate_docker_compose() {
    log_step "Generating Docker Compose configuration"
    
    cat > "$INSTALL_DIR/docker-compose.yml" << 'EOF'
version: '3.8'

services:
  api:
    image: ghcr.io/gamepanel/forge-api:latest
    container_name: gamepanel-api
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - gamepanel_data:/data
      - gamepanel_config:/config
    environment:
      - TZ=UTC
      - PUID=1000
      - PGID=1000
    networks:
      - gamepanel_network
    depends_on:
      - database

  database:
    image: postgres:15-alpine
    container_name: gamepanel-db
    restart: unless-stopped
    ports:
      - "5432:5432"
    volumes:
      - gamepanel_db_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_DB=gamepanel
      - POSTGRES_USER=gamepanel
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - TZ=UTC
    networks:
      - gamepanel_network

  web:
    image: ghcr.io/gamepanel/forge-web:latest
    container_name: gamepanel-web
    restart: unless-stopped
    ports:
      - "3000:3000"
    volumes:
      - gamepanel_data:/data
    environment:
      - API_URL=http://api:8080
      - APP_URL=https://${FQDN}
      - TZ=UTC
    networks:
      - gamepanel_network
    depends_on:
      - api

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

    log_info "Docker Compose configuration generated"
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
        ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
        ssl_prefer_server_ciphers on;

        # Security headers
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header X-XSS-Protection "1; mode=block" always;

        location / {
            proxy_pass http://web;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_set_header Upgrade \$http_upgrade;
            proxy_set_header Connection "upgrade";
        }

        location /api/ {
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
    
    # Pull the latest images
    log_info "Pulling Docker images..."
    docker-compose pull
    
    # Start the services
    log_info "Starting containers..."
    docker-compose up -d
    
    # Wait for services to be healthy
    log_info "Waiting for services to start..."
    sleep 10
    
    # Check service status
    docker-compose ps
}

verify_installation() {
    log_step "Verifying installation"
    
    # Check if containers are running
    local container_count
    container_count=$(docker ps --filter "name=gamepanel-*" --format "{{.Names}}" | wc -l)
    
    if [[ $container_count -lt 3 ]]; then
        log_error "Not all containers are running"
        docker-compose logs
        exit 1
    fi
    
    log_info "All containers are running"
    
    # Test API connectivity
    local api_health
    api_health=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/health || echo "000")
    
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
    echo "   Start:      cd $INSTALL_DIR && docker-compose up -d"
    echo "   Stop:       cd $INSTALL_DIR && docker-compose down"
    echo "   Restart:    cd $INSTALL_DIR && docker-compose restart"
    echo "   Logs:       cd $INSTALL_DIR && docker-compose logs -f"
    echo "   Update:     cd $INSTALL_DIR && docker-compose pull && docker-compose up -d"
    echo ""
    echo "⚠️  Important Notes:"
    echo "   - SSL certificates are not automatically configured"
    echo "   - Please set up SSL certificates in $INSTALL_DIR/ssl/"
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
    generate_nginx_config
    start_services
    verify_installation
    
    # Show summary
    show_summary
    
    log_header "Installation Complete"
}

# Run main function with all arguments
main "$@"