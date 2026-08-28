#!/bin/bash

# E2E Test Validation Script
# This script validates the test infrastructure and runs comprehensive checks

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DATABASE_URL="${TEST_DATABASE_URL:-postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable}"
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_DIR="$TEST_DIR/test-reports"
LOG_FILE="$REPORT_DIR/validation-$(date +%Y%m%d-%H%M%S).log"

# Create report directory
mkdir -p "$REPORT_DIR"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
    echo "[INFO] $1" >> "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
    echo "[SUCCESS] $1" >> "$LOG_FILE"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
    echo "[WARNING] $1" >> "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    echo "[ERROR] $1" >> "$LOG_FILE"
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to run a test and capture results
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    log_info "Running test: $test_name"
    
    if eval "$test_command" >/dev/null 2>&1; then
        log_success "$test_name passed"
        return 0
    else
        log_error "$test_name failed"
        return 1
    fi
}

# Header
log_info "=========================================="
log_info "E2E Test Infrastructure Validation"
log_info "=========================================="
log_info ""

# 1. Check prerequisites
log_info "1. Checking prerequisites..."

# Check Go
if command_exists go; then
    GO_VERSION=$(go version | awk '{print $3}')
    log_success "Go is installed (version: $GO_VERSION)"
else
    log_error "Go is not installed"
    exit 1
fi

# Check PostgreSQL client
if command_exists psql; then
    log_success "PostgreSQL client is installed"
else
    log_warning "PostgreSQL client is not installed (psql)"
fi

# Check Docker (optional)
if command_exists docker; then
    log_success "Docker is installed"
else
    log_warning "Docker is not installed (optional for some tests)"
fi

# Check Git
if command_exists git; then
    GIT_VERSION=$(git --version | awk '{print $3}')
    log_success "Git is installed (version: $GIT_VERSION)"
else
    log_error "Git is not installed"
    exit 1
fi

log_info ""

# 2. Check database connectivity
log_info "2. Checking database connectivity..."

if psql "$DATABASE_URL" -c "SELECT 1" >/dev/null 2>&1; then
    log_success "Database connection successful"
else
    log_error "Database connection failed"
    log_error "Please ensure PostgreSQL is running and TEST_DATABASE_URL is set correctly"
    exit 1
fi

# Check database version
DB_VERSION=$(psql "$DATABASE_URL" -t -c "SELECT version();" | head -1)
log_info "Database version: $DB_VERSION"

log_info ""

# 3. Check test files
log_info "3. Checking test files..."

TEST_FILES=(
    "suite_test.go"
    "replicated_app_test.go"
    "health_gated_deployment_test.go"
    "node_failure_test.go"
    "gitops_compose_test.go"
    "remote_build_test.go"
    "main_test.go"
    "report.go"
)

for file in "${TEST_FILES[@]}"; do
    if [ -f "$TEST_DIR/$file" ]; then
        log_success "Test file exists: $file"
    else
        log_error "Test file missing: $file"
        exit 1
    fi
done

log_info ""

# 4. Check Go modules
log_info "4. Checking Go modules..."

cd "$TEST_DIR"

if [ -f "go.mod" ]; then
    log_success "go.mod exists"
else
    log_warning "go.mod not found in test directory"
fi

# Try to download dependencies
if go mod download 2>/dev/null; then
    log_success "Go modules downloaded successfully"
else
    log_warning "Go modules download had issues"
fi

log_info ""

# 5. Check code compilation
log_info "5. Checking code compilation..."

if go build -tags=e2e ./... 2>/dev/null; then
    log_success "Code compiles successfully"
else
    log_error "Code compilation failed"
    exit 1
fi

log_info ""

# 6. Run database migrations check
log_info "6. Checking database migrations..."

# This would normally run the migrations, but for validation we just check they exist
if [ -d "$TEST_DIR/../../../migrations" ]; then
    log_success "Migrations directory exists"
else
    log_warning "Migrations directory not found"
fi

log_info ""

# 7. Run connectivity tests
log_info "7. Running connectivity tests..."

# Test database schema creation
if psql "$DATABASE_URL" -c "CREATE SCHEMA IF NOT EXISTS test_validation;" >/dev/null 2>&1; then
    log_success "Database schema creation successful"
    psql "$DATABASE_URL" -c "DROP SCHEMA IF EXISTS test_validation CASCADE;" >/dev/null 2>&1
else
    log_error "Database schema creation failed"
    exit 1
fi

log_info ""

# 8. Run a simple test
log_info "8. Running a simple database test..."

# Create a simple test file
cat > /tmp/simple_test.go << 'EOF'
package main

import (
    "context"
    "fmt"
    "os"
    "time"
    
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    databaseURL := os.Getenv("TEST_DATABASE_URL")
    if databaseURL == "" {
        fmt.Println("TEST_DATABASE_URL not set")
        os.Exit(1)
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    pool, err := pgxpool.New(ctx, databaseURL)
    if err != nil {
        fmt.Printf("Failed to connect: %v\n", err)
        os.Exit(1)
    }
    defer pool.Close()
    
    if err := pool.Ping(ctx); err != nil {
        fmt.Printf("Ping failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Println("Database connection test passed")
}
EOF

cd /tmp
if go run simple_test.go 2>/dev/null; then
    log_success "Simple database test passed"
else
    log_error "Simple database test failed"
    exit 1
fi

rm -f /tmp/simple_test.go

log_info ""

# 9. Check test configuration
log_info "9. Checking test configuration..."

# Check if TEST_DATABASE_URL is set
if [ -n "$TEST_DATABASE_URL" ]; then
    log_success "TEST_DATABASE_URL is set"
else
    log_warning "TEST_DATABASE_URL is not set (will use default)"
fi

# Check if E2E_ENVIRONMENT is set
if [ -n "$E2E_ENVIRONMENT" ]; then
    log_success "E2E_ENVIRONMENT is set to: $E2E_ENVIRONMENT"
else
    log_info "E2E_ENVIRONMENT is not set (will default to 'local')"
fi

log_info ""

# 10. Summary
log_info "=========================================="
log_info "Validation Summary"
log_info "=========================================="

log_success "All validation checks passed!"
log_info ""
log_info "You can now run the E2E tests with:"
log_info "  cd $TEST_DIR"
log_info "  make test"
log_info ""
log_info "Or run specific scenarios with:"
log_info "  make test-replicated"
log_info "  make test-health"
log_info "  make test-node"
log_info "  make test-gitops"
log_info "  make test-build"
log_info ""
log_info "Validation log saved to: $LOG_FILE"

# Clean up
rm -f /tmp/simple_test.go

exit 0