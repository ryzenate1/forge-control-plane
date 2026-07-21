# E2E Test Makefile

# Database configuration
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)

# Test targets
.PHONY: all
all: test

.PHONY: test
test:
	@echo "Running E2E tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e ./...

.PHONY: test-replicated
test-replicated:
	@echo "Running Replicated Application tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e -run TestReplicatedApplication ./...

.PHONY: test-health
test-health:
	@echo "Running Health-Gated Deployment tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e -run TestHealthGatedDeployment ./...

.PHONY: test-node
test-node:
	@echo "Running Node Failure tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e -run TestNodeFailure ./...

.PHONY: test-gitops
test-gitops:
	@echo "Running GitOps Compose Stack tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e -run TestGitOpsComposeStack ./...

.PHONY: test-build
test-build:
	@echo "Running Remote Build tests..."
	@TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -v -tags=e2e -run TestRemoteBuild ./...

.PHONY: test-all-scenarios
test-all-scenarios: test-replicated test-health test-node test-gitops test-build

.PHONY: setup-db
setup-db:
	@echo "Setting up test database..."
	@psql $(DATABASE_URL) -c "CREATE DATABASE IF NOT EXISTS forge_e2e_test;" || true

.PHONY: cleanup-db
cleanup-db:
	@echo "Cleaning up test database..."
	@psql $(DATABASE_URL) -c "DROP DATABASE IF EXISTS forge_e2e_test;" || true

.PHONY: lint
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  test              - Run all E2E tests"
	@echo "  test-replicated   - Run Replicated Application tests"
	@echo "  test-health       - Run Health-Gated Deployment tests"
	@echo "  test-node         - Run Node Failure tests"
	@echo "  test-gitops       - Run GitOps Compose Stack tests"
	@echo "  test-build        - Run Remote Build tests"
	@echo "  test-all-scenarios - Run all scenario tests"
	@echo "  setup-db          - Setup test database"
	@echo "  cleanup-db        - Cleanup test database"
	@echo "  lint              - Run linter"
