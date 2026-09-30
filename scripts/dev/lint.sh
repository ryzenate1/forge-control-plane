#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

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
# Web typecheck resolves workspace packages from their dist output, so build
# them first (same ordering as `npm run typecheck` at the repo root). This
# explicit call is load-bearing: npm runs `prebuild` before `build`, but it
# does NOT run `prestypecheck` before `typecheck` (verified on npm 11), so
# there is no hook to delegate to here — and because the hook never fires,
# this call cannot double-build either.
(cd "$ROOT" && npm run build:packages)
(cd "$ROOT/forge/web" && npm run typecheck)
if [ -d "$ROOT/packages/sdk" ] && grep -q '"lint"' "$ROOT/packages/sdk/package.json" 2>/dev/null; then (cd "$ROOT/packages/sdk" && npm run lint); fi
if [ -d "$ROOT/packages/shared-types" ] && grep -q '"lint"' "$ROOT/packages/shared-types/package.json" 2>/dev/null; then (cd "$ROOT/packages/shared-types" && npm run lint); fi
if [ -d "$ROOT/packages/ui" ] && grep -q '"lint"' "$ROOT/packages/ui/package.json" 2>/dev/null; then (cd "$ROOT/packages/ui" && npm run lint); fi
if [ -d "$ROOT/packages/game-templates" ] && grep -q '"lint"' "$ROOT/packages/game-templates/package.json" 2>/dev/null; then (cd "$ROOT/packages/game-templates" && npm run lint); fi

echo "=== Running Prettier check ==="
(cd "$ROOT" && npx prettier --check "forge/web/**/*.{ts,tsx,js,jsx,json,css}")

echo "=== All checks passed ==="
