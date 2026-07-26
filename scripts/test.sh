#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Running Go API tests ==="
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/services/i18n/...)
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/services/health/...)
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/services/activity/...)
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/services/plugins/...)
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/services/recovery/...)
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./internal/policies/...)

echo "=== Running Go Beacon tests ==="
(cd "$ROOT/beacon" && go test -v -count=1 ./...)

echo "=== Running all Go tests ==="
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./...)
(cd "$ROOT/beacon" && go test -v -cover -count=1 ./...)

echo "=== All tests passed ==="
