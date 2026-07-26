#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Running Go linters ==="
if command -v golangci-lint >/dev/null 2>&1; then
    (cd "$ROOT/forge/api" && golangci-lint run ./...)
    (cd "$ROOT/beacon" && golangci-lint run ./...)
else
    echo "golangci-lint not installed; running go vet..."
    (cd "$ROOT/forge/api" && go vet ./...)
    (cd "$ROOT/beacon" && go vet ./...)
fi

echo "=== Running TypeScript checks ==="
(cd "$ROOT/forge/web" && npm run lint)
if [ -d "$ROOT/packages/sdk" ]; then (cd "$ROOT/packages/sdk" && npm run lint); fi
if [ -d "$ROOT/packages/shared-types" ]; then (cd "$ROOT/packages/shared-types" && npm run lint); fi

echo "=== Running Prettier check ==="
(cd "$ROOT" && npx prettier --check "forge/web/**/*.{ts,tsx,js,jsx,json,css}")

echo "=== All checks passed ==="
