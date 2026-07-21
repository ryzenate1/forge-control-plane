#!/usr/bin/env bash
# =============================================================================
# GamePanel — Master Shipping Script
# Orchestrates fresh installs, upgrades, beacon deployment, monitoring, and TLS.
# Usage: ./ship.sh [--help|--beacon|--dry-run]
# =============================================================================
set -euo pipefail

SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INFRA="$(cd "$SELF/.." && pwd)"
PROJECT="$(cd "$INFRA/.." && pwd)"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*" >&2; }
err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
header(){ echo -e "\n${CYAN}━━━ $* ━━━${NC}\n"; }

BEACON_MODE=false
DRY_RUN=false
COMPOSE_FILES=(-f compose.yml -f compose.production.yml)
COMPOSE_ENV=(--env-file .env)
COMPOSE_CMD=(docker compose "${COMPOSE_FILES[@]}" "${COMPOSE_ENV[@]}")

# ---------------------------------------------------------------------------
# Parse args
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help)    sed -ne '/^# Usage:/,/^$/{s/^# //;p}' "$0"; exit 0 ;;
    --beacon)  BEACON_MODE=true ; shift ;;
    --dry-run) DRY_RUN=true     ; shift ;;
    *) err "Unknown flag: $1"; exit 1 ;;
  esac
done

# ---------------------------------------------------------------------------
# Prerequisites
# ---------------------------------------------------------------------------
check_prereqs() {
  header "Checking prerequisites"

  if [[ "$(uname -s)" != Linux ]]; then
    warn "Not running on Linux — some features may not work"
  fi

  if ! command -v docker &>/dev/null; then
    err "Docker is not installed. See https://docs.docker.com/engine/install/"
    exit 1
  fi
  ok "Docker $(docker --version)"

  if ! docker compose version &>/dev/null; then
    err "Docker Compose v2 is required"
    exit 1
  fi
  compose_ver="$(docker compose version --short 2>/dev/null || docker compose version 2>/dev/null)"
  ok "Docker Compose $compose_ver"

  major="${compose_ver%%.*}"; minor="${compose_ver#*.}"; minor="${minor%%.*}"
  if [[ "$major" -lt 2 ]] || { [[ "$major" -eq 2 ]] && [[ "$minor" -lt 24 ]]; }; then
    err "Docker Compose v2.24+ required (detected $major.$minor)"
    exit 1
  fi

  if ! command -v openssl &>/dev/null; then
    err "openssl is required for secret generation"
    exit 1
  fi
  ok "OpenSSL available"

  if ! command -v curl &>/dev/null; then
    warn "curl not found — healthchecks will be skipped"
  fi

  echo
}

# ---------------------------------------------------------------------------
# Environment detection & generation
# ---------------------------------------------------------------------------
ensure_env() {
  header "Environment setup"

  if [[ -f "$INFRA/.env" ]]; then
    info "Found existing .env — sourcing"
    set -a; source "$INFRA/.env"; set +a
    MODE="upgrade"
    warn "Upgrade mode: existing .env detected"
  else
    MODE="fresh"
    info "No .env found — generating new secrets"

    if [[ -z "${PANEL_DOMAIN:-}" ]]; then
      warn "PANEL_DOMAIN not set — defaulting to panel.example.com"
    fi

    if [[ "$DRY_RUN" == true ]]; then
      echo "Would run: PANEL_DOMAIN=${PANEL_DOMAIN:-panel.example.com} bash $INFRA/gen-env.sh $INFRA/.env"
      return
    fi

    PANEL_DOMAIN="${PANEL_DOMAIN:-panel.example.com}" bash "$INFRA/gen-env.sh" "$INFRA/.env"
    set -a; source "$INFRA/.env"; set +a
    ok ".env generated with secure secrets"
  fi

  # Validate critical secrets
  local missing=0
  for var in POSTGRES_PASSWORD API_AUTH_SECRET FORGE_MASTER_KEY DAEMON_NODE_TOKEN; do
    if [[ -z "${!var:-}" ]]; then
      err "$var is not set in .env"
      missing=1
    fi
  done
  if [[ "$missing" -eq 1 ]]; then
    exit 1
  fi

  # Auto-generate optional master key if missing
  if [[ -z "${FORGE_MASTER_KEY:-}" ]]; then
    FORGE_MASTER_KEY="$(openssl rand -hex 32)"
    FORGE_MASTER_KEY_ID=primary
    cat >> "$INFRA/.env" <<EOF

# Added by ship.sh
FORGE_MASTER_KEY=$FORGE_MASTER_KEY
FORGE_MASTER_KEY_ID=primary
EOF
    ok "Generated missing FORGE_MASTER_KEY"
  fi
}

# ---------------------------------------------------------------------------
# Deploy control plane
# ---------------------------------------------------------------------------
deploy_control_plane() {
  header "Deploying control plane"

  if [[ "$DRY_RUN" == true ]]; then
    echo "Would run: bash $INFRA/bootstrap-control-plane.sh"
    return
  fi

  mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"

  info "Starting PostgreSQL and Redis..."
  "${COMPOSE_CMD[@]}" up -d postgres redis
  "${COMPOSE_CMD[@]}" exec -T postgres pg_isready -U "${POSTGRES_USER:-gamepanel}" -d "${POSTGRES_DB:-gamepanel}" --quiet
  ok "PostgreSQL is healthy"
  "${COMPOSE_CMD[@]}" exec -T redis redis-cli ping | grep -q PONG
  ok "Redis is healthy"

  info "Starting API and Web..."
  "${COMPOSE_CMD[@]}" up -d api web
  sleep 5
  "${COMPOSE_CMD[@]}" exec -T api /api --healthcheck && ok "API is healthy" || warn "API healthcheck failed — check logs"
  ok "Forge Web started"

  "${COMPOSE_CMD[@]}" ps
}

# ---------------------------------------------------------------------------
# Deploy beacon (control-plane node)
# ---------------------------------------------------------------------------
deploy_beacon() {
  header "Deploying Beacon daemon (local node)"

  if [[ "$DRY_RUN" == true ]]; then
    echo "Would run: ${COMPOSE_CMD[*]} up -d daemon"
    return
  fi

  mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"
  "${COMPOSE_CMD[@]}" up -d daemon
  sleep 3
  "${COMPOSE_CMD[@]}" exec -T daemon /daemon --healthcheck && ok "Beacon daemon is healthy" || warn "Beacon healthcheck failed"

  info "To register this node, complete setup in the panel at https://${PANEL_DOMAIN:-panel.example.com}" \
       ", create a Node, and update DAEMON_NODE_ID / DAEMON_NODE_TOKEN in .env"
}

# ---------------------------------------------------------------------------
# Deploy monitoring
# ---------------------------------------------------------------------------
deploy_monitoring() {
  header "Deploying monitoring stack"

  if [[ "$DRY_RUN" == true ]]; then
    echo "Would run: ${COMPOSE_CMD[*]} up -d prometheus alertmanager grafana"
    return
  fi

  "${COMPOSE_CMD[@]}" up -d prometheus alertmanager grafana
  sleep 5
  ok "Monitoring stack started"
}

# ---------------------------------------------------------------------------
# TLS / SSL setup
# ---------------------------------------------------------------------------
setup_tls() {
  header "TLS / SSL configuration"

  if [[ -n "${TRAEFIK_ACME_EMAIL:-}" ]] && [[ -n "${PANEL_DOMAIN:-}" ]] && [[ "${PANEL_DOMAIN:-}" != "panel.example.com" ]]; then
    info "Traefik with Let's Encrypt detected — adding TLS compose file"
    COMPOSE_FILES+=(-f compose.tls.yml)

    if [[ "$DRY_RUN" == false ]]; then
      "${COMPOSE_CMD[@]}" up -d traefik
      ok "Traefik started — TLS certificates will be obtained automatically"
    fi
  else
    warn "TRAEFIK_ACME_EMAIL or PANEL_DOMAIN not set; skipping auto-TLS."
    info "To set up manually:"
    info "  1. Install certbot: sudo apt install certbot"
    info "  2. sudo certbot certonly --standalone -d ${PANEL_DOMAIN:-panel.yourdomain.com}"
    info "  3. Edit infra/nginx.conf and replace __PANEL_FQDN__"
    info "  4. sudo cp infra/nginx.conf /etc/nginx/sites-available/gamepanel"
  fi
}

# ---------------------------------------------------------------------------
# Health verification
# ---------------------------------------------------------------------------
health_check() {
  header "Health verification"

  if [[ "$DRY_RUN" == true ]]; then
    echo "Would run health checks against running services"
    return
  fi

  local failed=0

  if command -v curl &>/dev/null; then
    if curl -sf http://127.0.0.1:8080/health >/dev/null 2>&1; then
      ok "Forge API health: OK"
    else
      err "Forge API health: FAIL"
      failed=1
    fi

    if curl -sf -o /dev/null http://127.0.0.1:3000/; then
      ok "Forge Web health: OK"
    else
      err "Forge Web health: FAIL"
      failed=1
    fi

    if curl -sf http://127.0.0.1:9091/-/healthy >/dev/null 2>&1; then
      ok "Prometheus health: OK"
    fi

    if curl -sf http://127.0.0.1:9093/-/healthy >/dev/null 2>&1; then
      ok "Alertmanager health: OK"
    fi

    if curl -sf -o /dev/null http://127.0.0.1:3001/api/health >/dev/null 2>&1; then
      ok "Grafana health: OK"
    fi
  else
    warn "curl not available — skipping HTTP health checks"
    "${COMPOSE_CMD[@]}" ps
  fi

  echo
  if [[ "$failed" -eq 1 ]]; then
    warn "Some health checks failed — review logs with: docker compose logs"
  else
    ok "All services healthy"
  fi
}

# ---------------------------------------------------------------------------
# Print post-install summary
# ---------------------------------------------------------------------------
print_summary() {
  header "Deployment summary"

  local domain="${PANEL_DOMAIN:-panel.example.com}"
  local scheme="http"

  if [[ -n "${TRAEFIK_ACME_EMAIL:-}" ]] || [[ -f "/etc/nginx/sites-enabled/gamepanel" ]]; then
    scheme="https"
  fi

  echo
  echo -e "  ${CYAN}GamePanel${NC}                   ${scheme}://${domain}"
  echo -e "  ${CYAN}Forge API${NC}                   http://127.0.0.1:8080"
  echo -e "  ${CYAN}Forge Web${NC}                   http://127.0.0.1:3000"
  echo -e "  ${CYAN}Grafana${NC}                     http://127.0.0.1:3001  (admin / ${GRAFANA_ADMIN_PASSWORD:-<set in .env>})"
  echo -e "  ${CYAN}Prometheus${NC}                  http://127.0.0.1:9091"
  echo -e "  ${CYAN}Alertmanager${NC}                http://127.0.0.1:9093"
  echo
  echo -e "  ${YELLOW}Next steps:${NC}"
  echo -e "  1. Open ${scheme}://${domain}/setup in your browser"
  echo -e "  2. Create the admin account"
  echo -e "  3. Add nodes via Nodes -> Create Node"
  echo -e "  4. Update DAEMON_NODE_ID and DAEMON_NODE_TOKEN in infra/.env"
  echo -e "  5. Re-run this script to deploy the Beacon daemon and monitoring"
  echo
  echo -e "  ${YELLOW}Useful commands:${NC}"
  echo -e "  docker compose -f compose.yml -f compose.production.yml --env-file .env logs -f"
  echo -e "  docker compose -f compose.yml -f compose.production.yml --env-file .env ps"
  echo -e "  docker compose -f compose.yml -f compose.production.yml --env-file .env down"
  echo

  if [[ "$MODE" == "fresh" ]]; then
    warn "IMPORTANT: Back up the .env file — it contains secrets that cannot be recovered!"
    echo -e "  cp infra/.env ~/gamepanel.env.backup"
  fi
}

# ---------------------------------------------------------------------------
# Beacon-only mode (remote node)
# ---------------------------------------------------------------------------
beacon_mode() {
  header "Beacon node deployment"

  check_prereqs

  if [[ ! -f "$INFRA/.env" ]]; then
    err "infra/.env is required on beacon nodes"
    err "Copy it from the control plane or create it with DAEMON_NODE_ID, DAEMON_NODE_TOKEN, and PANEL_API_URL"
    exit 1
  fi

  set -a; source "$INFRA/.env"; set +a
  : "${DAEMON_NODE_ID:?Set DAEMON_NODE_ID in .env}"
  : "${DAEMON_NODE_TOKEN:?Set DAEMON_NODE_TOKEN in .env}"
  : "${PANEL_API_URL:?Set PANEL_API_URL in .env}"

  mkdir -p "${GAME_SERVERS_HOST_DIR:-/srv/game-panel/servers}"

  if [[ "$DRY_RUN" == true ]]; then
    echo "Would run: docker compose -f compose.beacon.yml --env-file .env up -d --build"
    return
  fi

  docker compose -f "$INFRA/compose.beacon.yml" --env-file "$INFRA/.env" config --quiet
  docker compose -f "$INFRA/compose.beacon.yml" --env-file "$INFRA/.env" up -d --build
  ok "Beacon daemon deployed"

  echo
  info "Beacon node deployed. Verify with:"
  echo "  docker compose -f compose.beacon.yml --env-file .env ps"
  echo "  docker compose -f compose.beacon.yml --env-file .env logs daemon"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo -e "${CYAN}"
  echo "  ╔══════════════════════════════════════════════╗"
  echo "  ║           GamePanel — Ship Script             ║"
  echo "  ╚══════════════════════════════════════════════╝"
  echo -e "${NC}"

  if [[ "$BEACON_MODE" == true ]]; then
    beacon_mode
    exit 0
  fi

  check_prereqs
  ensure_env

  if [[ "$MODE" == "fresh" ]]; then
    deploy_control_plane
    setup_tls
    deploy_monitoring
    deploy_beacon
  else
    warn "Upgrade detected — pulling latest images and rebuilding"
    "${COMPOSE_CMD[@]}" pull
    "${COMPOSE_CMD[@]}" up -d --build --remove-orphans
  fi

  health_check
  print_summary
}

main
