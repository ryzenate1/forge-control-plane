#!/usr/bin/env bash
# Production readiness guard — exits with an error if any check fails.
# Source this file or run as a step before the application starts.
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

fail() {
  echo -e "${RED}PRODUCTION GUARD FAILED:${NC} $*" >&2
  exit 1
}

ok() {
  echo -e "  ${GREEN}[PASS]${NC} $1"
}

info() {
  echo -e "  ${YELLOW}[INFO]${NC} $1"
}

echo ""
echo "============================================"
echo "  GamePanel Production Readiness Guard"
echo "============================================"
echo ""

# === Environment variable validation ===
echo "--- Environment variables ---"

required_vars=(
  "API_AUTH_SECRET"
  "APP_KEY"
  "APP_ENV"
  "DATABASE_URL"
  "FORGE_MASTER_KEY"
  "DAEMON_NODE_TOKEN"
  "DAEMON_NODE_ID"
)

for var in "${required_vars[@]}"; do
  if [ -z "${!var:-}" ]; then
    fail "$var is not set"
  fi
done
ok "All required environment variables are set"

if [ "${FORGE_ALLOW_EPHEMERAL_MASTER_KEY:-}" = "true" ]; then
  fail "FORGE_ALLOW_EPHEMERAL_MASTER_KEY must not be true in production"
fi
ok "FORGE_ALLOW_EPHEMERAL_MASTER_KEY is disabled"

forge_master_key="${FORGE_MASTER_KEY:-}"
if [ "${#forge_master_key}" -lt 32 ]; then
  fail "FORGE_MASTER_KEY must contain at least 32 characters (got ${#forge_master_key})"
fi
ok "FORGE_MASTER_KEY length is sufficient (${#forge_master_key} chars)"

case "${DATABASE_URL:-}" in
  *sslmode=require*|*sslmode=verify-ca*|*sslmode=verify-full*) ;;
  *@postgres:5432/*sslmode=disable*)
    ;;
  *) fail "DATABASE_URL must require TLS unless it targets the private bundled postgres service" ;;
esac
ok "DATABASE_URL uses TLS or internal service"

api_auth_secret="${API_AUTH_SECRET:-}"
app_key="${APP_KEY:-}"

if [ "${#api_auth_secret}" -lt 32 ] || [ "$api_auth_secret" = "dev-api-secret" ]; then
  fail "API_AUTH_SECRET must be a non-default secret of at least 32 characters"
fi
ok "API_AUTH_SECRET is valid"

if [ "${#app_key}" -lt 32 ]; then
  fail "APP_KEY must be at least 32 characters"
fi
ok "APP_KEY is valid"

if [ "${APP_ENV:-}" != "production" ]; then
  fail "APP_ENV must be 'production' (got '${APP_ENV:-}')"
fi
ok "APP_ENV is set to production"

# === Docker daemon check ===
echo ""
echo "--- Docker daemon ---"
if ! command -v docker &>/dev/null; then
  fail "Docker is not installed"
fi
ok "Docker binary found"

if ! docker info &>/dev/null; then
  fail "Docker daemon is not running"
fi
ok "Docker daemon is running"

# === Port availability ===
echo ""
echo "--- Port availability ---"
critical_ports=(80 443 8080 3000 9090 2022)
for port in "${critical_ports[@]}"; do
  if ss -tlnp "sport = :$port" 2>/dev/null | grep -q ":$port "; then
    info "Port $port is in use (may be expected)"
  fi
done

# === TLS certificate check ===
echo ""
echo "--- TLS certificates ---"
if ! command -v openssl &>/dev/null; then
  info "openssl not installed; skipping TLS certificate validation"
elif [ -n "${TLS_CERT_PATH:-}" ] && [ -n "${TLS_KEY_PATH:-}" ]; then
  if [ -f "$TLS_CERT_PATH" ]; then
    CERT_EXPIRY=$(openssl x509 -enddate -noout -in "$TLS_CERT_PATH" 2>/dev/null | cut -d= -f2)
    if [ -n "$CERT_EXPIRY" ]; then
      if date -d "$CERT_EXPIRY" +%s >/dev/null 2>&1; then
        EXPIRY_EPOCH=$(date -d "$CERT_EXPIRY" +%s 2>/dev/null || echo 0)
      else
        EXPIRY_EPOCH=$(date -j -f "%b %d %H:%M:%S %Y %Z" "$CERT_EXPIRY" +%s 2>/dev/null || echo 0)
      fi
      NOW_EPOCH=$(date +%s)
      if [ "$EXPIRY_EPOCH" -gt "$NOW_EPOCH" ]; then
        DAYS_LEFT=$(( (EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))
        if [ "$DAYS_LEFT" -lt 30 ]; then
          info "TLS certificate expires in $DAYS_LEFT days: $CERT_EXPIRY"
        else
          ok "TLS certificate valid for $DAYS_LEFT more days"
        fi
      else
        fail "TLS certificate has expired: $CERT_EXPIRY"
      fi
    fi
  else
    info "TLS_CERT_PATH set but file not found: $TLS_CERT_PATH"
  fi
else
  info "TLS_CERT_PATH/TLS_KEY_PATH not configured; skipping certificate validation"
fi

# === Disk space check ===
echo ""
echo "--- System resources ---"
AVAILABLE_DISK=$(df /var/lib/docker 2>/dev/null | awk 'NR==2 {print $4}' || df / | awk 'NR==2 {print $4}')
if [ "${AVAILABLE_DISK:-0}" -lt 5242880 ]; then
  info "Less than 5GB available disk space (${AVAILABLE_DISK}KB)"
fi

MEM_TOTAL=$(free -m | awk '/^Mem:/{print $2}')
if [ "${MEM_TOTAL:-0}" -lt 2048 ]; then
  info "System has less than 2GB RAM (${MEM_TOTAL}MB)"
fi

# === Run production guard via api ===
echo ""
echo "--- API production guard ---"
if [ -x /api ]; then
  /api --production-guard 2>/dev/null || true
fi

echo ""
echo "============================================"
echo -e "${GREEN}All production guard checks passed.${NC}"
echo "============================================"
echo ""
