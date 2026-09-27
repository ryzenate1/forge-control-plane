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

# --- Options ---
# Default is --keep-data: volumes, images and host directories that may hold
# database / server data are left alone unless --purge-data is given.
PURGE_DATA=false

parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --purge-data)
                PURGE_DATA=true
                shift
                ;;
            --keep-data)
                PURGE_DATA=false
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [--keep-data|--purge-data]"
                echo "  --keep-data    Preserve Docker volumes/images and data dirs (default)"
                echo "  --purge-data   Remove volumes, images and data dirs after a backup prompt"
                exit 0
                ;;
            *)
                echo "Unknown option: $1" >&2
                exit 1
                ;;
        esac
    done
}

# --- Confirmation ---
confirm_uninstall() {
    echo ""
    if [ "$PURGE_DATA" = true ]; then
        log_error "WARNING: This will remove GamePanel Forge AND ALL DATA (volumes, images, data dirs)!"
    else
        log_error "WARNING: This will remove GamePanel Forge components (data volumes preserved)."
    fi
    echo ""
    echo "The following will be removed:"
    echo "  - Installation directory: $INSTALL_DIR"
    echo "  - Configuration directory: $CONFIG_DIR"
    echo "  - Log directory: $LOG_DIR"
    echo "  - Docker containers"
    if [ "$PURGE_DATA" = true ]; then
        echo "  - Data directory: $DATA_DIR (PURGE)"
        echo "  - Docker volumes, images and networks (PURGE)"
    else
        echo "  - (kept by default: $DATA_DIR, Docker volumes/images)"
    fi
    echo ""

    read -p "Are you sure you want to uninstall GamePanel Forge? [y/N]: " -n 1 -r
    echo

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Uninstallation cancelled"
        exit 0
    fi

    if [ "$PURGE_DATA" = true ]; then
        echo ""
        log_warn "You asked to permanently delete all data."
        log_warn "Take a database backup first (infra/postgres-backup.sh, see docs/restore-runbook.md)."
        read -p "Have you backed up your data and still want to PURGE everything? [y/N]: " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Purge not confirmed — switching to --keep-data mode."
            PURGE_DATA=false
        fi
    fi
}

# --- Stop Services ---
stop_services() {
    log_step "Stopping GamePanel Forge services"

    if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/docker-compose.yml" ]; then
        cd "$INSTALL_DIR"

        # Stop containers (Compose v2)
        if docker compose down; then
            log_info "Services stopped"
        else
            log_warn "Failed to stop services with docker compose, trying docker directly"
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

    local containers=()
    while IFS= read -r line; do
        [ -n "$line" ] && containers+=("$line")
    done < <(docker ps -a --filter "name=gamepanel-*" --format "{{.Names}}" || true)

    if [ "${#containers[@]}" -gt 0 ]; then
        docker rm -f "${containers[@]}" || true
        log_info "Containers removed"
    else
        log_info "No GamePanel containers found"
    fi
}

# --- Remove Images ---
remove_images() {
    log_step "Removing Docker images"

    if [ "$PURGE_DATA" = false ]; then
        log_info "Keeping images (--purge-data not given)"
        return
    fi

    local images=()
    while IFS= read -r line; do
        [ -n "$line" ] && images+=("$line")
    done < <(docker images --filter "reference=ghcr.io/gamepanel/*" --format "{{.Repository}}:{{.Tag}}" || true)

    if [ "${#images[@]}" -gt 0 ]; then
        docker rmi -f "${images[@]}" || true
        log_info "Images removed"
    else
        log_info "No GamePanel images found"
    fi
}

# --- Remove Volumes ---
remove_volumes() {
    log_step "Removing Docker volumes"

    if [ "$PURGE_DATA" = false ]; then
        log_info "Keeping volumes (--purge-data not given)"
        return
    fi

    local volumes=()
    while IFS= read -r line; do
        [ -n "$line" ] && volumes+=("$line")
    done < <(docker volume ls --filter "name=gamepanel_*" --format "{{.Name}}" || true)

    if [ "${#volumes[@]}" -gt 0 ]; then
        docker volume rm -f "${volumes[@]}" || true
        log_info "Volumes removed"
    else
        log_info "No GamePanel volumes found"
    fi
}

# --- Remove Networks ---
remove_networks() {
    log_step "Removing Docker networks"

    local networks=()
    while IFS= read -r line; do
        [ -n "$line" ] && networks+=("$line")
    done < <(docker network ls --filter "name=gamepanel_*" --format "{{.Name}}" || true)

    if [ "${#networks[@]}" -gt 0 ]; then
        docker network rm "${networks[@]}" || true
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

    # Data directory holds database/server data — keep unless purging.
    if [ "$PURGE_DATA" = true ]; then
        if [ -d "$DATA_DIR" ]; then
            rm -rf "$DATA_DIR"
            log_info "Data directory removed"
        else
            log_info "Data directory not found"
        fi
    else
        log_info "Keeping data directory $DATA_DIR (--purge-data not given)"
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
# NOTE: never run a bare `docker system prune -f` here — it deletes images,
# volumes and networks belonging to OTHER applications on the host. Only
# GamePanel-labelled objects are touched (see remove_* above). This hook is
# kept for an explicit scoped cleanup in --purge-data mode.
clean_docker_system() {
    log_step "Cleaning Docker system (scoped to GamePanel only)"

    if [ "$PURGE_DATA" = false ]; then
        log_info "Keeping shared Docker objects (--purge-data not given)"
        return
    fi

    docker image prune -f --filter "label!=keep" >/dev/null 2>&1 || true
    log_info "Scoped Docker cleanup done (no global prune performed)"
}

# --- Show Summary ---
show_summary() {
    echo ""
    log_info "GamePanel Forge has been uninstalled"
    echo ""
    echo "Removed components:"
    echo "  ✓ Docker containers"
    echo "  ✓ Docker networks"
    echo "  ✓ Installation directories"
    echo "  ✓ Configuration files"
    if [ "$PURGE_DATA" = true ]; then
        echo "  ✓ Docker volumes (purged)"
        echo "  ✓ Docker images (purged)"
        echo "  ✓ Data directory $DATA_DIR (purged)"
    else
        echo "  - Docker volumes (kept — re-run with --purge-data to remove)"
        echo "  - Docker images (kept — re-run with --purge-data to remove)"
        echo "  - Data directory $DATA_DIR (kept — re-run with --purge-data to remove)"
    fi
    echo ""
    if [ "$PURGE_DATA" = false ]; then
        echo "Note: database and server data were preserved."
        echo "To remove them after a backup, re-run: $0 --purge-data"
    else
        echo "Note: all GamePanel data was purged. Only GamePanel-labelled"
        echo "Docker objects were touched; nothing else was pruned."
    fi
    echo ""
}

# --- Main Function ---
main() {
    parse_arguments "$@"
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
