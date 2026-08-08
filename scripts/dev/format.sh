#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
echo "=== Formatting Go code ==="
if command -v goimports >/dev/null 2>&1; then
    cd "$ROOT/forge/api" && gofmt -w -s . && goimports -w .
    cd "$ROOT/beacon" && gofmt -w -s . && goimports -w .
else
    cd "$ROOT/forge/api" && gofmt -w -s . 
    cd "$ROOT/beacon" && gofmt -w -s .
fi

echo "=== Formatting TypeScript/JavaScript ==="
npx prettier --write "forge/web/**/*.{ts,tsx,js,jsx,json,css}" "packages/**/*.{ts,tsx,js,jsx,json}"

echo "=== Formatting complete ==="
