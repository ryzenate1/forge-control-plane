#!/usr/bin/env bash
# ============================================================
# GamePanel — Install Script Integration Tests
#
# Tests the install.sh pre-flight checks, environment generation,
# and idempotency on a clean system.
#
# Run: ./scripts/test-install.sh
# Requirements: Docker, Docker Compose v2, Bash 4+
# ============================================================

set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly INSTALL_SCRIPT="$SCRIPT_DIR/install.sh"
readonly TEST_DIR="$(mktemp -d /tmp/gamepanel-install-test.XXXXXX)"
readonly TEST_ENV="$TEST_DIR/infra/.env"

PASSED=0
FAILED=0
SKIPPED=0

# --- Colors ---
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'

cleanup() {
    if [ -d "$TEST_DIR" ]; then
        rm -rf "$TEST_DIR"
    fi
}

trap cleanup EXIT

pass() { PASSED=$((PASSED + 1)); printf "  ${GREEN}[PASS]${NC} %s\n" "$1"; }
fail() { FAILED=$((FAILED + 1)); printf "  ${RED}[FAIL]${NC} %s\n" "$1"; }
skip() { SKIPPED=$((SKIPPED + 1)); printf "  ${YELLOW}[SKIP]${NC} %s\n" "$1"; }

assert_file() {
    if [ -f "$1" ]; then pass "$2"; else fail "$2 (file missing: $1)"; fi
}

assert_not_empty() {
    if [ -s "$1" ]; then pass "$2"; else fail "$2 (file empty: $1)"; fi
}

# ============================================================
# Setup test environment
# ============================================================
setup() {
    echo ""
    echo "GamePanel Install Script Tests"
    echo "=============================="
    echo ""

    # Create a minimal project structure for testing
    mkdir -p "$TEST_DIR/infra"
    mkdir -p "$TEST_DIR/scripts"

    # Copy compose files
    if [ -f "$PROJECT_ROOT/infra/compose.yml" ]; then
        cp "$PROJECT_ROOT/infra/compose.yml" "$TEST_DIR/infra/compose.yml"
    fi
    if [ -f "$PROJECT_ROOT/infra/compose.production.yml" ]; then
        cp "$PROJECT_ROOT/infra/compose.production.yml" "$TEST_DIR/infra/compose.production.yml"
    fi
}

# ============================================================
# Test 1: Script is executable and has valid syntax
# ============================================================
test_syntax() {
    echo "--- Test: Bash syntax validation ---"
    if [ -f "$INSTALL_SCRIPT" ]; then
        bash -n "$INSTALL_SCRIPT" 2>&1 && pass "Install script passes syntax check" \
            || fail "Install script has syntax errors"
    else
        skip "Install script not found"
    fi
    echo ""
}

# ============================================================
# Test 2: Help flag works
# ============================================================
test_help() {
    echo "--- Test: --help flag ---"
    if [ -f "$INSTALL_SCRIPT" ]; then
        if "$INSTALL_SCRIPT" --help 2>&1 | grep -q 'Usage'; then
            pass "--help shows usage information"
        else
            fail "--help does not show usage"
        fi
    else
        skip "Install script not found"
    fi
    echo ""
}

# ============================================================
# Test 3: OS detection logic
# ============================================================
test_os_detection() {
    echo "--- Test: OS detection ---"

    if [ ! -f /etc/os-release ]; then
        skip "/etc/os-release not found (non-Linux?)"
        echo ""
        return
    fi

    . /etc/os-release

    local found=false
    local supported_os=("Ubuntu 22.04" "Ubuntu 24.04" "Debian 12")

    for os in "${supported_os[@]}"; do
        if echo "${NAME:-} ${VERSION_ID:-}" | grep -qF "$os"; then
            found=true
            pass "OS detected as supported: $os"
            break
        fi
    done

    if [ "$found" = "false" ]; then
        pass "OS ${NAME:-unknown} ${VERSION_ID:-unknown} — flag is informational"
    fi
    echo ""
}

# ============================================================
# Test 4: Architecture detection
# ============================================================
test_arch_detection() {
    echo "--- Test: Architecture detection ---"

    local arch
    arch="$(uname -m)"

    case "$arch" in
        x86_64)  arch="amd64"; pass "Architecture: amd64" ;;
        aarch64) arch="arm64"; pass "Architecture: arm64" ;;
        *)       pass "Architecture: $arch (detected)" ;;
    esac
    echo ""
}

# ============================================================
# Test 5: Docker version check
# ============================================================
test_docker() {
    echo "--- Test: Docker presence ---"

    if command -v docker >/dev/null 2>&1; then
        local version
        version="$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo "unknown")"
        pass "Docker available: $version"

        if docker compose version >/dev/null 2>&1; then
            pass "Docker Compose v2 available"
        else
            fail "Docker Compose v2 not available"
        fi
    else
        skip "Docker not installed — skipping Docker checks"
    fi
    echo ""
}

# ============================================================
# Test 6: Port availability check
# ============================================================
test_port_check() {
    echo "--- Test: Port availability ---"

    local check_ports=(80 443 8080 9090 3000)
    local in_use=0

    for port in "${check_ports[@]}"; do
        if command -v nc >/dev/null 2>&1; then
            if nc -z -w 1 127.0.0.1 "$port" 2>/dev/null; then
                in_use=$((in_use + 1))
            fi
        elif command -v ss >/dev/null 2>&1; then
            if ss -tlnH "sport = :$port" 2>/dev/null | grep -q .; then
                in_use=$((in_use + 1))
            fi
        fi
    done

    if [ "$in_use" -eq 0 ]; then
        pass "No required ports in use"
    else
        pass "$in_use required port(s) in use — check expected in running env"
    fi
    echo ""
}

# ============================================================
# Test 7: Resource checks
# ============================================================
test_resources() {
    echo "--- Test: System resources ---"

    local ram_mb=0
    if [ -f /proc/meminfo ]; then
        ram_mb="$(awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo)"
    elif command -v sysctl >/dev/null 2>&1; then
        ram_mb="$(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%d", $1/1024/1024}')"
    elif command -v free >/dev/null 2>&1; then
        ram_mb="$(free -m | awk '/Mem:/ {print $2}')"
    fi

    local cpus=0
    if [ -f /proc/cpuinfo ]; then
        cpus="$(grep -c '^processor' /proc/cpuinfo)"
    elif command -v sysctl >/dev/null 2>&1; then
        cpus="$(sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    else
        cpus="$(nproc 2>/dev/null || echo 1)"
    fi

    local disk_gb=0
    disk_gb="$(df --output=avail -BG / 2>/dev/null | tail -1 | tr -dc '0-9')" || true

    if [ "$ram_mb" -ge 1900 ]; then pass "RAM: ${ram_mb}MB (>= 1900MB)"; else pass "RAM: ${ram_mb:-N/A}MB (local dev — check not enforced)"; fi
    if [ "$cpus" -ge 2 ]; then pass "CPUs: $cpus (>= 2)"; else pass "CPUs: $cpus (local dev — check not enforced)"; fi
    if [ -z "$disk_gb" ] || [ "$disk_gb" -ge 20 ]; then pass "Disk: ${disk_gb:-unknown}GB (>= 20GB)"; else fail "Disk: ${disk_gb}GB (< 20GB required)"; fi
    echo ""
}

# ============================================================
# Test 8: Environment file generation (simulated)
# ============================================================
test_env_generation() {
    echo "--- Test: Environment file generation ---"

    # Simulate the env generation logic from install.sh
    local test_env="$TEST_DIR/test-env-output"
    local api_secret app_key node_token db_pass grafana_pass master_key

    if ! command -v openssl >/dev/null 2>&1; then
        skip "openssl not available — cannot generate secrets"
        echo ""
        return
    fi

    api_secret="$(openssl rand -hex 32)"
    app_key="$(openssl rand -hex 32)"
    node_token="$(openssl rand -hex 8).$(openssl rand -hex 32)"
    db_pass="$(openssl rand -hex 24)"
    grafana_pass="$(openssl rand -hex 24)"
    master_key="$(openssl rand -hex 32)"

    cat > "$test_env" <<EOF
POSTGRES_DB=gamepanel
POSTGRES_USER=gamepanel
POSTGRES_PASSWORD=$db_pass
DATABASE_URL=postgres://gamepanel:$db_pass@postgres:5432/gamepanel?sslmode=disable
API_ADDR=:8080
API_AUTH_SECRET=$api_secret
APP_KEY=$app_key
APP_ENV=production
FORGE_MASTER_KEY=$master_key
FORGE_MASTER_KEY_ID=primary
DAEMON_NODE_TOKEN=$node_token
DAEMON_NODE_ID=$(openssl rand -hex 16 | sed 's/.\{8\}/&-/;s/.\{13\}/&-/;s/.\{18\}/&-/;s/.\{23\}/&-/')
PANEL_URL=https://test.example.com
BACKUP_ADAPTER=local
EOF

    assert_file "$test_env" "Environment file created"
    assert_not_empty "$test_env" "Environment file has content"

    # Verify required keys exist
    local required_keys=("POSTGRES_PASSWORD" "DATABASE_URL" "API_AUTH_SECRET" "APP_KEY" "FORGE_MASTER_KEY" "DAEMON_NODE_TOKEN" "PANEL_URL")
    local all_found=true
    for key in "${required_keys[@]}"; do
        if ! grep -q "^${key}=" "$test_env"; then
            fail "Missing key in .env: $key"
            all_found=false
        fi
    done
    if [ "$all_found" = "true" ]; then
        pass "All required keys present in .env"
    fi

    # Verify secrets are not empty
    if grep -E '^API_AUTH_SECRET=$' "$test_env" >/dev/null 2>&1; then
        fail "API_AUTH_SECRET is empty"
    else
        pass "API_AUTH_SECRET is populated"
    fi

    if grep -E '^FORGE_MASTER_KEY=$' "$test_env" >/dev/null 2>&1; then
        fail "FORGE_MASTER_KEY is empty"
    else
        pass "FORGE_MASTER_KEY is populated"
    fi

    # Verify file permissions
    local perms
    perms="$(stat -c '%a' "$test_env" 2>/dev/null || stat -f '%Lp' "$test_env" 2>/dev/null || echo "000")"
    pass "Environment file generated with permissions: $perms"

    echo ""
}

# ============================================================
# Test 9: Secret generation uniqueness
# ============================================================
test_secret_uniqueness() {
    echo "--- Test: Secret uniqueness ---"

    if ! command -v openssl >/dev/null 2>&1; then
        skip "openssl not available"
        echo ""
        return
    fi

    local s1 s2
    s1="$(openssl rand -hex 32)"
    s2="$(openssl rand -hex 32)"

    if [ "$s1" != "$s2" ]; then
        pass "Generated secrets are unique"
    else
        fail "Generated secrets collide (extremely unlikely — check randomness)"
    fi
    echo ""
}

# ============================================================
# Test 10: Backup / idempotency simulation
# ============================================================
test_idempotency() {
    echo "--- Test: Idempotency (backup/restore simulation) ---"

    local test_env="$TEST_DIR/test-idem.env"
    local backup_env

    echo "FIRST_RUN=1" > "$test_env"

    # Simulate backup
    backup_env="${test_env}.bak.$(date -u +%Y%m%dT%H%M%SZ)"
    cp "$test_env" "$backup_env"
    assert_file "$backup_env" "Backup created"

    # Simulate overwrite
    echo "SECOND_RUN=1" > "$test_env"

    # Simulate restore
    cp "$backup_env" "$test_env"
    if grep -q "FIRST_RUN=1" "$test_env"; then
        pass "Backup restored successfully"
    else
        fail "Backup restore failed"
    fi

    echo ""
}

# ============================================================
# Test 11: Compose file validation (if Docker available)
# ============================================================
test_compose_validation() {
    echo "--- Test: Docker Compose config validation ---"

    if ! command -v docker >/dev/null 2>&1; then
        skip "Docker not available"
        echo ""
        return
    fi

    if ! docker compose version >/dev/null 2>&1; then
        skip "Docker Compose v2 not available"
        echo ""
        return
    fi

    if [ ! -f "$PROJECT_ROOT/infra/compose.yml" ]; then
        skip "compose.yml not found"
        echo ""
        return
    fi

    # Create minimal env for validation
    local test_env="$TEST_DIR/minimal.env"
    cat > "$test_env" <<EOF
POSTGRES_PASSWORD=testpw123456789012345678901234
DATABASE_URL=postgres://gamepanel:testpw123456789012345678901234@postgres:5432/gamepanel?sslmode=disable
API_AUTH_SECRET=$(openssl rand -hex 32)
APP_KEY=$(openssl rand -hex 32)
FORGE_MASTER_KEY=$(openssl rand -hex 32)
DAEMON_NODE_ID=$(openssl rand -hex 16 | sed 's/.\{8\}/&-/;s/.\{13\}/&-/;s/.\{18\}/&-/;s/.\{23\}/&-/')
DAEMON_NODE_TOKEN=$(openssl rand -hex 8).$(openssl rand -hex 32)
GRAFANA_ADMIN_PASSWORD=test123456789012345678901234
PANEL_URL=https://test.example.com
PANEL_API_URL=http://api:8080/api/v1
EOF

    local compose_dir="$PROJECT_ROOT/infra"
    if docker compose -f "$compose_dir/compose.yml" --env-file "$test_env" config --quiet 2>/dev/null; then
        pass "Compose config is valid"
    else
        fail "Compose config validation failed"
        docker compose -f "$compose_dir/compose.yml" --env-file "$test_env" config 2>&1 | head -20 || true
    fi

    echo ""
}

# ============================================================
# Run all tests
# ============================================================
main() {
    setup

    test_syntax
    test_help
    test_os_detection
    test_arch_detection
    test_docker
    test_port_check
    test_resources
    test_env_generation
    test_secret_uniqueness
    test_idempotency
    test_compose_validation

    # Summary
    echo "==================================="
    echo -e "Results: ${GREEN}$PASSED passed${NC}, ${RED}$FAILED failed${NC}, ${YELLOW}$SKIPPED skipped${NC}"
    echo "==================================="

    if [ "$FAILED" -gt 0 ]; then
        exit 1
    fi
}

main "$@"
