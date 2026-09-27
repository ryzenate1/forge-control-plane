#!/usr/bin/env bash
# ============================================================
# GamePanel - Linux Production Deployment Script
#
# This script deploys GamePanel on a Linux server with:
#   - Native PostgreSQL 16 (systemd)
#   - Native Redis 7 (systemd)
#   - Go API binary (systemd)
#   - Next.js frontend (systemd + Node.js)
#
# NO DOCKER for database/redis. Docker is ONLY for game servers.
#
# Usage:
#   chmod +x deploy-prod.sh
#   sudo ./deploy-prod.sh
# ============================================================

set -euo pipefail

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "  ${GREEN}[ok]${NC} $1"; }
warn()  { echo -e "  ${YELLOW}[!!]${NC} $1"; }
fail()  { echo -e "  ${RED}[FAIL]${NC} $1"; exit 1; }
header(){ echo -e "\n${CYAN}=== $1 ===${NC}"; }

# --- Must be root ---
if [ "$(id -u)" -ne 0 ]; then
    fail "This script must be run as root (sudo ./deploy-prod.sh)"
fi

# --- Configuration ---
PANEL_USER="gamepanel"
DB_NAME="gamepanel"
DB_PASS="$(openssl rand -base64 32 | tr -d '\n')"
API_SECRET="$(openssl rand -base64 32 | tr -d '\n')"
NODE_TOKEN="$(openssl rand -hex 8).$(openssl rand -hex 32)"
# 32 random bytes, base64-encoded (matches FORGE_MASTER_KEY contract:
# exactly 32 bytes base64/base64url). Never hex — the API rejects it.
MASTER_KEY="$(openssl rand -base64 32 | tr -d '\n')"
REDIS_PASS="$(openssl rand -base64 32 | tr -d '\n')"
INSTALL_DIR="/opt/gamepanel"

API_PORT=8080
FRONTEND_PORT=3000

echo ""
echo -e "  ${RED}+==========================================+${NC}"
echo -e "  ${RED}|   GamePanel Production Deployment        |${NC}"
echo -e "  ${RED}|   Native PostgreSQL + Redis (no Docker)  |${NC}"
echo -e "  ${RED}+==========================================+${NC}"
echo ""

# ============================================================
# Step 1: Install PostgreSQL 16 natively
# ============================================================
header "Installing PostgreSQL 16"

if command -v psql &>/dev/null; then
    info "PostgreSQL already installed: $(psql --version)"
else
    # Add PostgreSQL APT repo
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates gnupg lsb-release
    curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
    echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list
    apt-get update -qq
    apt-get install -y -qq postgresql-16
    info "PostgreSQL 16 installed"
fi

# Ensure running
systemctl enable postgresql
systemctl start postgresql
info "PostgreSQL service running (systemd)"

# Create database and user
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${PANEL_USER}'" | grep -q 1 || {
    sudo -u postgres psql -c "CREATE USER ${PANEL_USER} WITH PASSWORD '${DB_PASS}' CREATEDB;"
    info "Created PostgreSQL user: ${PANEL_USER}"
}
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1 || {
    sudo -u postgres createdb -O "${PANEL_USER}" "${DB_NAME}"
    info "Created database: ${DB_NAME}"
}

# ============================================================
# Step 2: Install Redis 7 natively
# ============================================================
header "Installing Redis"

if command -v redis-cli &>/dev/null; then
    info "Redis already installed: $(redis-cli --version)"
else
    apt-get install -y -qq redis-server
    info "Redis installed"
fi

systemctl enable redis-server
systemctl start redis-server
# Require authentication: an unauthenticated localhost Redis accepts commands
# from any local user/process. Persist requirepass and restart.
if ! grep -qsE '^[[:space:]]*requirepass' /etc/redis/redis.conf 2>/dev/null; then
    # Escape backslash and double-quote for the redis.conf line.
    _rp_esc="${REDIS_PASS//\\/\\\\}"
    _rp_esc="${_rp_esc//\"/\\\"}"
    if [ -w /etc/redis/redis.conf ]; then
        printf '\n# Managed by deploy-prod.sh — Forge API auth\nrequirepass "%s"\n' "$_rp_esc" >> /etc/redis/redis.conf
        systemctl restart redis-server
    else
        warn "Cannot write /etc/redis/redis.conf — set requirepass manually, then restart redis-server"
    fi
fi
if REDISCLI_AUTH="$REDIS_PASS" redis-cli ping 2>/dev/null | grep -q PONG; then
    info "Redis AUTH verified (requirepass active)"
else
    warn "Redis AUTH check failed — verify requirepass in /etc/redis/redis.conf and that REDIS_PASSWORD matches"
fi
info "Redis service running (systemd)"

# ============================================================
# Step 3: Install Node.js 20+ (for frontend)
# ============================================================
header "Checking Node.js"

if command -v node &>/dev/null && [ "$(node -v | cut -d. -f1 | tr -d v)" -ge 20 ]; then
    info "Node.js $(node -v) already installed"
else
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y -qq nodejs
    info "Node.js $(node -v) installed"
fi

# ============================================================
# Step 4: Install Docker (for game server containers ONLY)
# ============================================================
header "Checking Docker (for game servers only)"

if command -v docker &>/dev/null; then
    info "Docker already installed: $(docker --version)"
else
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    info "Docker installed (used ONLY for game server containers)"
fi

# ============================================================
# Step 5: Create panel user and directories
# ============================================================
header "Setting up directories"

id -u "${PANEL_USER}" &>/dev/null || {
    useradd -r -m -d "${INSTALL_DIR}" -s /bin/bash "${PANEL_USER}"
    info "Created system user: ${PANEL_USER}"
}

mkdir -p "${INSTALL_DIR}"/{api,frontend,migrations,logs}
info "Directories created at ${INSTALL_DIR}"

# ============================================================
# Step 6: Build and install API
# ============================================================
header "Building Go API"

if command -v go &>/dev/null; then
    info "Go $(go version | awk '{print $3}') found"
else
    warn "Go not installed. Install from https://go.dev/dl/ and re-run."
    warn "Or copy the pre-built api binary to ${INSTALL_DIR}/api/"
fi

# Copy migrations
cp -r apps/api/migrations/* "${INSTALL_DIR}/migrations/" 2>/dev/null || true
info "Migrations copied to ${INSTALL_DIR}/migrations/"

# Build API (if source available)
if [ -d "panel" ] && command -v go &>/dev/null; then
    cd forge
    CGO_ENABLED=0 go build -o "${INSTALL_DIR}/api/panel-api" ./cmd/api
    cd ../..
    info "API binary built: ${INSTALL_DIR}/api/panel-api"
fi

# ============================================================
# Step 7: Build frontend
# ============================================================
header "Building Frontend"

if [ -d "web" ]; then
    cd web
    npm ci --production=false
    npx next build
    cd ../..
    cp -r web/{.next,public,package.json,node_modules} "${INSTALL_DIR}/frontend/" 2>/dev/null || true
    info "Frontend built and installed"
fi

# ============================================================
# Step 8: Create environment file
# ============================================================
header "Creating environment config"

DB_PASS_ENC="$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote_plus(sys.argv[1]))' "$DB_PASS")"

cat > "${INSTALL_DIR}/.env" << EOF
# GamePanel Production Configuration
# Generated: $(date -Iseconds)
# Permissions: 0600 ${PANEL_USER}:${PANEL_USER}. Never print or log this file.

# Database (native PostgreSQL - NOT Docker)
# sslmode=require: configure PostgreSQL with SSL (ssl=on + certs) before use.
# A `disable` value would send the DB password in cleartext.
DATABASE_URL=postgres://${PANEL_USER}:${DB_PASS_ENC}@localhost:5432/${DB_NAME}?sslmode=require

# API
API_ADDR=:${API_PORT}
API_AUTH_SECRET=${API_SECRET}
APP_ENV=production
MIGRATIONS_DIR=${INSTALL_DIR}/migrations

# Redis (native - NOT Docker, AUTH required — see requirepass above)
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=${REDIS_PASS}

# Daemon
DAEMON_NODE_TOKEN=${NODE_TOKEN}
API_DEMO_MODE=false

# Encryption at Rest (Required when DATABASE_URL is set)
# 32 bytes base64 — loss = permanent data loss.
FORGE_MASTER_KEY=${MASTER_KEY}
FORGE_MASTER_KEY_ID=primary
EOF


chmod 600 "${INSTALL_DIR}/.env"
chown "${PANEL_USER}:${PANEL_USER}" -R "${INSTALL_DIR}"
info "Environment config: ${INSTALL_DIR}/.env"

# ============================================================
# Step 9: Create systemd services
# ============================================================
header "Creating systemd services"

# API service
cat > /etc/systemd/system/forge-api.service << EOF
[Unit]
Description=GamePanel API Server
After=network.target postgresql.service redis-server.service
Requires=postgresql.service

[Service]
Type=simple
User=${PANEL_USER}
WorkingDirectory=${INSTALL_DIR}/api
EnvironmentFile=${INSTALL_DIR}/.env
ExecStart=${INSTALL_DIR}/api/panel-api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=forge-api

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${INSTALL_DIR}/logs
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
info "Created forge-api.service"

# Frontend service
cat > /etc/systemd/system/panel-frontend.service << EOF
[Unit]
Description=GamePanel Frontend (Next.js)
After=network.target forge-api.service

[Service]
Type=simple
User=${PANEL_USER}
WorkingDirectory=${INSTALL_DIR}/frontend
Environment=NODE_ENV=production
Environment=PORT=${FRONTEND_PORT}
ExecStart=/usr/bin/npx next start -p ${FRONTEND_PORT}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=plane-frontend

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
info "Created plane-frontend.service"

# Reload and enable
systemctl daemon-reload
systemctl enable forge-api plane-frontend
systemctl start forge-api
sleep 3
systemctl start plane-frontend

info "Services enabled and started"

# ============================================================
# Summary
# ============================================================
echo ""
echo -e "  ${GREEN}+==========================================+${NC}"
echo -e "  ${GREEN}|   GamePanel Deployed Successfully!       |${NC}"
echo -e "  ${GREEN}+==========================================+${NC}"
echo ""
echo "  Architecture (NO Docker for infrastructure):"
echo "  -----------------------------------------------"
echo -e "  ${CYAN}PostgreSQL 16${NC}  Native systemd   port 5432"
echo -e "  ${CYAN}Redis 7${NC}        Native systemd   port 6379"
echo -e "  ${CYAN}API${NC}            Native Go binary  port ${API_PORT}"
echo -e "  ${CYAN}Frontend${NC}       Node.js           port ${FRONTEND_PORT}"
echo -e "  ${CYAN}Docker${NC}         Game servers ONLY"
echo ""
echo "  Credentials saved to: ${INSTALL_DIR}/.env (mode 600, owner ${PANEL_USER})"
echo "  Secrets are never printed to the console or logs."
echo "  Retrieve them with: sudo cat ${INSTALL_DIR}/.env"
echo "  Back up ${INSTALL_DIR}/.env offline — loss of FORGE_MASTER_KEY = permanent data loss."
echo ""
echo "  Commands:"
echo "    systemctl status forge-api"
echo "    systemctl status plane-frontend"
echo "    journalctl -u forge-api -f"
echo "    journalctl -u plane-frontend -f"
echo ""
echo -e "  ${YELLOW}Back up ${INSTALL_DIR}/.env now (offline, chmod 600). It will not be shown again.${NC}"
echo ""
""
