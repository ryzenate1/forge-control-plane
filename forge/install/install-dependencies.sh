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
    # Remove old Docker versions
    apt-get remove -y docker docker-engine docker.io containerd runc || true
    
    # Install required packages
    apt-get update
    apt-get install -y ca-certificates curl gnupg lsb-release
    
    # Add Docker's official GPG key
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    
    # Set up the repository
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    
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
    local compose_version="v2.24.5"
    curl -SL https://github.com/docker/compose/releases/download/${compose_version}/docker-compose-linux-x86_64 -o /usr/local/bin/docker-compose
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
            apt-get update
            apt-get install -y git
            ;;
        centos|rhel|fedora)
            yum install -y git
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
            apt-get update
            apt-get install -y curl wget jq htop net-tools lsof
            ;;
        centos|rhel|fedora)
            yum install -y curl wget jq htop net-tools lsof
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
    
    # Add current user to docker group
    local current_user
    current_user=$(whoami)
    if [ "$current_user" != "root" ]; then
        usermod -aG docker "$current_user"
        log_info "User $current_user added to docker group"
        log_info "Please log out and log back in for Docker group changes to take effect"
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