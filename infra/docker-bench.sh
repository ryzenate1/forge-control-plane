#!/usr/bin/env bash
# GamePanel Docker Security Benchmark
# Checks running containers match the security posture defined in
# infra/compose.yml and reports PASS / FAIL / WARN for each check.
# Exit code 0 if all checks pass.
#
# Usage: ./infra/docker-bench.sh
set -euo pipefail

PASS=0
FAIL=0
WARN=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ---- helpers ---------------------------------------------------------------
pass() { echo -e "  ${GREEN}PASS${NC}: $1"; PASS=$((PASS+1)); }
fail() { echo -e "  ${RED}FAIL${NC}: $1"; FAIL=$((FAIL+1)); }
warn() { echo -e "  ${YELLOW}WARN${NC}: $1"; WARN=$((WARN+1)); }

check_container() {
  local c="$1"
  echo ""
  echo "--- Container: $c ---"

  # 1. Running as non-root user
  local user
  user=$(docker inspect "$c" --format '{{.Config.User}}' 2>/dev/null || echo "")
  if [ -z "$user" ] || [ "$user" = "" ] || [ "$user" = "root" ] || [ "$user" = "0" ]; then
    fail "$c runs as root"
  else
    pass "$c runs as user '$user'"
  fi

  # 2. no-new-privileges security_opt
  local sec
  sec=$(docker inspect "$c" --format '{{json .HostConfig.SecurityOpt}}' 2>/dev/null || echo "[]")
  if echo "$sec" | grep -q "no-new-privileges"; then
    pass "$c has no-new-privileges"
  else
    fail "$c missing no-new-privileges"
  fi

  # 3. Capabilities dropped (ALL)
  local cap
  cap=$(docker inspect "$c" --format '{{json .HostConfig.CapDrop}}' 2>/dev/null || echo "[]")
  if echo "$cap" | grep -q "ALL"; then
    pass "$c drops ALL capabilities"
  else
    fail "$c does not drop ALL capabilities (CapDrop: $cap)"
  fi

  # 4. Read-only root filesystem
  local ro
  ro=$(docker inspect "$c" --format '{{.HostConfig.ReadonlyRootfs}}' 2>/dev/null || echo "false")
  if [ "$ro" = "true" ]; then
    pass "$c has read-only rootfs"
  else
    warn "$c does not have read-only rootfs"
  fi

  # 5. Healthcheck configured
  local hc
  hc=$(docker inspect "$c" --format '{{json .Config.Healthcheck}}' 2>/dev/null || echo "null")
  if [ "$hc" != "null" ] && [ -n "$hc" ]; then
    pass "$c has healthcheck"
  else
    fail "$c missing healthcheck"
  fi

  # 6. Memory limit set
  local mem
  mem=$(docker inspect "$c" --format '{{.HostConfig.Memory}}' 2>/dev/null || echo "0")
  if [ -n "$mem" ] && [ "$mem" != "0" ]; then
    pass "$c has memory limit ($mem bytes)"
  else
    warn "$c has no memory limit"
  fi
}

# ---- main ------------------------------------------------------------------
echo "=========================================="
echo "  GamePanel Docker Security Benchmark"
echo "=========================================="
echo ""

# Host-level check: Docker socket permissions
echo "--- Host: Docker socket ---"
if [ -S /var/run/docker.sock ]; then
  perms=$(stat -c '%a' /var/run/docker.sock 2>/dev/null || stat -f '%OLp' /var/run/docker.sock 2>/dev/null || echo "000")
  if [ "$perms" -le 660 ] 2>/dev/null; then
    pass "Docker socket permissions ($perms)"
  else
    warn "Docker socket too permissive ($perms, should be =660)"
  fi
else
  warn "Docker socket not found at /var/run/docker.sock"
fi

# Find GamePanel containers
CONTAINERS=$(docker ps --format "{{.Names}}" 2>/dev/null \
  | grep -E '(gamepanel|forge|beacon|api|daemon|web|postgres|redis|prometheus|alertmanager|grafana)' \
  || true)

if [ -z "$CONTAINERS" ]; then
  echo ""
  warn "No matching containers found running"
  echo ""
  echo "=========================================="
  echo -e "  ${GREEN}$PASS passed${NC}"
  echo -e "  ${RED}$FAIL failed${NC}"
  echo -e "  ${YELLOW}$WARN warnings${NC}"
  echo "=========================================="
  exit 0
fi

for c in $CONTAINERS; do
  check_container "$c"
done

echo ""
echo "=========================================="
echo -e "  ${GREEN}$PASS passed${NC}"
echo -e "  ${RED}$FAIL failed${NC}"
echo -e "  ${YELLOW}$WARN warnings${NC}"
echo "=========================================="

exit $((FAIL > 0 ? 1 : 0))
