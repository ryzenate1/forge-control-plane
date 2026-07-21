#!/usr/bin/env bash
# ============================================================
# GamePanel Forge Uninstallation Script
# 
# This script removes GamePanel Forge and its components from the system.
# ============================================================

set -euo pipefail

# --- Constants ---
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly INSTALL_DIR="/opt/gamepanel"
readonly DATA_DIR="/var/lib/gamepanel"
readonly CONFIG_DIR="/etc/gamepanel"
readonly LOG_DIR="/var/log/gamepanel"

# --- Colors ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}→ $1${NC}"
}

# --- Confirmation ---
confirm_uninstall() {
    echo ""
    log_error "WARNING: This will remove all GamePanel Forge components and data!"
    echo ""
    echo "The following will be removed:"
    echo "  - Installation directory: $INSTALL_DIR"
    echo "  - Data directory: $DATA_DIR"
    echo "  - Configuration directory: $CONFIG_DIR"
    echo "  - Log directory: $LOG_DIR"
    echo "  - Docker containers and images"
    echo "  - Database data"
    echo ""
    
    read -p "Are you sure you want to uninstall GamePanel Forge? [y/N]: " -n 1 -r
    echo
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Uninstallation cancelled"
        exit 0
    fi
}

# --- Stop Services ---
stop_services() {
    log_step "Stopping GamePanel Forge services"
    
    if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/docker-compose.yml" ]; then
        cd "$INSTALL_DIR"
        
        # Stop containers
        if docker-compose down; then
            log_info "Services stopped"
        else
            log_warn "Failed to stop services with docker-compose, trying docker directly"
            docker stop gamepanel-api gamepanel-web gamepanel-db gamepanel-proxy || true
        fi
    else
        log_warn "Installation directory not found, trying to stop containers directly"
        docker stop gamepanel-api gamepanel-web gamepanel-db gamepanel-proxy || true
    fi
}

# --- Remove Containers ---
remove_containers() {
    log_step "Removing Docker containers"
    
    local containers
    containers=$(docker ps -a --filter "name=gamepanel-*" --format "{{.Names}}" || true)
    
    if [ -n "$containers" ]; then
        docker rm -f $containers || true
        log_info "Containers removed"
    else
        log_info "No GamePanel containers found"
    fi
}

# --- Remove Images ---
remove_images() {
    log_step "Removing Docker images"
    
    local images
    images=$(docker images --filter "reference=ghcr.io/gamepanel/*" --format "{{.Repository}}:{{.Tag}}" || true)
    
    if [ -n "$images" ]; then
        docker rmi -f $images || true
        log_info "Images removed"
    else
        log_info "No GamePanel images found"
    fi
}

# --- Remove Volumes ---
remove_volumes() {
    log_step "Removing Docker volumes"
    
    local volumes
    volumes=$(docker volume ls --filter "name=gamepanel_*" --format "{{.Name}}" || true)
    
    if [ -n "$volumes" ]; then
        docker volume rm $volumes || true
        log_info "Volumes removed"
    else
        log_info "No GamePanel volumes found"
    fi
}

# --- Remove Networks ---
remove_networks() {
    log_step "Removing Docker networks"
    
    local networks
    networks=$(docker network ls --filter "name=gamepanel_*" --format "{{.Name}}" || true)
    
    if [ -n "$networks" ]; then
        docker network rm $networks || true
        log_info "Networks removed"
    else
        log_info "No GamePanel networks found"
    fi
}

# --- Remove Directories ---
remove_directories() {
    log_step "Removing installation directories"
    
    # Remove install directory
    if [ -d "$INSTALL_DIR" ]; then
        rm -rf "$INSTALL_DIR"
        log_info "Install directory removed"
    else
        log_info "Install directory not found"
    fi
    
    # Remove data directory
    if [ -d "$DATA_DIR" ]; then
        rm -rf "$DATA_DIR"
        log_info "Data directory removed"
    else
        log_info "Data directory not found"
    fi
    
    # Remove config directory
    if [ -d "$CONFIG_DIR" ]; then
        rm -rf "$CONFIG_DIR"
        log_info "Config directory removed"
    else
        log_info "Config directory not found"
    fi
    
    # Remove log directory
    if [ -d "$LOG_DIR" ]; then
        rm -rf "$LOG_DIR"
        log_info "Log directory removed"
    else
        log_info "Log directory not found"
    fi
}

# --- Remove Configuration Files ---
remove_config_files() {
    log_step "Removing configuration files"
    
    # Remove systemd service files
    if [ -f /etc/systemd/system/gamepanel-api.service ]; then
        rm -f /etc/systemd/system/gamepanel-api.service
        systemctl daemon-reload || true
        log_info "Systemd service files removed"
    fi
    
    # Remove cron jobs
    if crontab -l 2>/dev/null | grep -q gamepanel; then
        crontab -l 2>/dev/null | grep -v gamepanel | crontab - 2>/dev/null || true
        log_info "Cron jobs removed"
    fi
}

# --- Clean Docker System ---
clean_docker_system() {
    log_step "Cleaning Docker system"
    
    # Remove unused Docker objects
    docker system prune -f || true
    log_info "Docker system cleaned"
}

# --- Show Summary ---
show_summary() {
    echo ""
    log_info "GamePanel Forge has been uninstalled"
    echo ""
    echo "Removed components:"
    echo "  ✓ Docker containers"
    echo "  ✓ Docker images"
    echo "  ✓ Docker volumes"
    echo "  ✓ Docker networks"
    echo "  ✓ Installation directories"
    echo "  ✓ Configuration files"
    echo ""
    echo "Note: Database data in Docker volumes may still exist."
    echo "To completely remove all data, run: docker system prune -a"
    echo ""
}

# --- Main Function ---
main() {
    echo ""
    log_step "Starting GamePanel Forge uninstallation"
    echo ""
    
    # Check if running as root
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi
    
    # Confirm uninstallation
    confirm_uninstall
    
    # Perform uninstallation
    stop_services
    remove_containers
    remove_images
    remove_volumes
    remove_networks
    remove_directories
    remove_config_files
    clean_docker_system
    
    # Show summary
    show_summary
    
    log_step "Uninstallation complete"
}

# Run main function
main "$@"