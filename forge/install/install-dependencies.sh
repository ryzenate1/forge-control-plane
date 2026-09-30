#!/usr/bin/env bash
# ============================================================
# GamePanel Forge Dependencies Installation Script
#
# This script installs all required dependencies for GamePanel Forge
# on supported Linux distributions.
# ============================================================

set -euo pipefail

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

# --- Detect OS and Version ---
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME="$ID"
        OS_VERSION="$VERSION_ID"
    elif type lsb_release >/dev/null 2>&1; then
        OS_NAME=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        OS_VERSION=$(lsb_release -sr)
    elif [ -f /etc/redhat-release ]; then
        OS_NAME="rhel"
        OS_VERSION=$(cat /etc/redhat-release | head -1 | awk '{print $7}')
    else
        OS_NAME="unknown"
        OS_VERSION="unknown"
    fi

    echo "$OS_NAME"
}

# --- RHEL-family package helper (dnf preferred, yum fallback) ---
pkg_rhel() {
    if command -v dnf >/dev/null 2>&1; then
        dnf install -y "$@"
    elif command -v yum >/dev/null 2>&1; then
        yum install -y "$@"
    else
        log_error "Neither dnf nor yum found; cannot install: $*"
        exit 1
    fi
}

# --- Install Docker ---
install_docker() {
    local os_name
    os_name=$(detect_os)

    log_step "Installing Docker"

    case "$os_name" in
        ubuntu|debian)
            install_docker_ubuntu_debian
            ;;
        centos|rhel|fedora)
            install_docker_centos_rhel
            ;;
        *)
            log_error "Unsupported OS for Docker installation: $os_name"
            exit 1
            ;;
    esac

    # Verify Docker installation
    if ! command -v docker &> /dev/null; then
        log_error "Docker installation failed"
        exit 1
    fi

    log_info "Docker installed successfully: $(docker --version)"
}

install_docker_ubuntu_debian() {
    export DEBIAN_FRONTEND=noninteractive
    local distro codename
    # shellcheck disable=SC1091
    . /etc/os-release 2>/dev/null || true
    distro="${ID:-ubuntu}"
    # Debian must use the debian repo, not ubuntu (different codenames/keys).
    case "$distro" in
        debian) distro="debian" ;;
        *) distro="ubuntu" ;;
    esac
    codename="$(lsb_release -cs 2>/dev/null || echo "${VERSION_CODENAME:-stable}")"
    # Remove old Docker versions
    apt-get remove -y docker docker-engine docker.io containerd runc || true

    # Install required packages
    apt-get update
    apt-get install -y ca-certificates curl gnupg lsb-release

    # Add Docker's official GPG key for the matching distro
    mkdir -p /etc/apt/keyrings
    curl -fsSL "https://download.docker.com/linux/${distro}/gpg" | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod 644 /etc/apt/keyrings/docker.gpg

    # Set up the repository
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${distro} \
      ${codename} stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    # Install Docker Engine
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
}

install_docker_centos_rhel() {
    # Remove old Docker versions
    yum remove -y docker docker-client docker-client-latest docker-common docker-latest docker-latest-logrotate docker-logrotate docker-engine || true

    # Install required packages
    yum install -y yum-utils

    # Add Docker repository
    yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo

    # Install Docker Engine
    yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

    # Start Docker
    systemctl start docker
    systemctl enable docker
}

# --- Install Docker Compose ---
install_docker_compose() {
    log_step "Installing Docker Compose"

    # Check if Docker Compose is already installed
    if docker compose version &> /dev/null; then
        log_info "Docker Compose is already installed"
        return
    fi

    # Install Docker Compose standalone (fallback)
    # Match the host arch: the hard-coded x86_64 asset 404s/fails on arm64.
    local compose_version="v2.24.5" compose_arch machine
    machine="$(uname -m)"
    case "$machine" in
        x86_64|amd64) compose_arch="x86_64" ;;
        aarch64|arm64) compose_arch="aarch64" ;;
        armv7l|armv7*) compose_arch="armv7" ;;
        *) log_error "Unsupported architecture for compose fallback: $machine"; exit 1 ;;
    esac
    curl -SL "https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-linux-${compose_arch}" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose

    # Verify installation
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose installation failed"
        exit 1
    fi

    log_info "Docker Compose installed successfully: $(docker-compose --version)"
}

# --- Install Git ---
install_git() {
    log_step "Installing Git"

    local os_name
    os_name=$(detect_os)

    case "$os_name" in
        ubuntu|debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update
            apt-get install -y git
            ;;
        centos|rhel|fedora)
            pkg_rhel git
            ;;
        *)
            log_error "Unsupported OS for Git installation: $os_name"
            exit 1
            ;;
    esac

    # Verify Git installation
    if ! command -v git &> /dev/null; then
        log_error "Git installation failed"
        exit 1
    fi

    log_info "Git installed successfully: $(git --version)"
}

# --- Install Other Dependencies ---
install_dependencies() {
    log_step "Installing additional dependencies"

    local os_name
    os_name=$(detect_os)

    case "$os_name" in
        ubuntu|debian)
            export DEBIAN_FRONTEND=noninteractive
            apt-get update
            apt-get install -y curl wget jq htop net-tools lsof
            ;;
        centos|rhel|fedora)
            pkg_rhel curl wget jq htop net-tools lsof
            ;;
        *)
            log_error "Unsupported OS for dependencies installation: $os_name"
            exit 1
            ;;
    esac

    log_info "Additional dependencies installed"
}

# --- Configure Docker to Start on Boot ---
configure_docker_autostart() {
    log_step "Configuring Docker to start on boot"

    if command -v systemctl &> /dev/null; then
        systemctl enable docker
        systemctl start docker
    else
        log_warn "systemctl not found, Docker may not start on boot"
    fi
}

# --- Add Current User to Docker Group ---
configure_docker_group() {
    log_step "Configuring Docker group"

    # Create docker group if it doesn't exist
    if ! getent group docker > /dev/null; then
        groupadd docker
    fi

    # Add the invoking (non-root) user — under sudo $USER is root, so prefer
    # $SUDO_USER which names the human who ran sudo.
    local current_user
    current_user="${SUDO_USER:-$(whoami)}"
    # Strip domain suffix if present (e.g. DOMAIN\user handled elsewhere).
    current_user="${current_user%% *}"
    if [ "$current_user" != "root" ] && [ -n "$current_user" ]; then
        usermod -aG docker "$current_user"
        log_info "User $current_user added to docker group"
        log_info "Please log out and log back in for Docker group changes to take effect"
    elif [ -n "${SUDO_USER:-}" ]; then
        log_info "Invoked via sudo by $SUDO_USER; added to docker group"
    fi
}

# --- Main Function ---
main() {
    echo ""
    log_step "Starting GamePanel Forge dependencies installation"
    echo ""

    # Detect OS
    local os_name
    os_name=$(detect_os)
    log_info "Detected OS: $os_name"

    # Check if running as root
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi

    # Install dependencies
    install_dependencies
    install_git
    install_docker
    install_docker_compose

    # Configure Docker
    configure_docker_autostart
    configure_docker_group

    echo ""
    log_info "All dependencies installed successfully!"
    echo ""
    echo "Next steps:"
    echo "1. Log out and log back in (or run: newgrp docker)"
    echo "2. Run the GamePanel Forge installation script"
    echo ""
}

# Run main function
main "$@"
