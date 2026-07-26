#!/bin/bash
set -euo pipefail

echo "=== Formatting Go code ==="
if command -v goimports >/dev/null 2>&1; then
    cd forge/api && gofmt -w -s . && goimports -w . && cd ../..
    cd beacon && gofmt -w -s . && goimports -w . && cd ../..
else
    cd forge/api && gofmt -w -s . && cd ../..
    cd beacon && gofmt -w -s . && cd ../..
fi

echo "=== Formatting TypeScript/JavaScript ==="
npx prettier --write "forge/web/**/*.{ts,tsx,js,jsx,json,css}" "packages/**/*.{ts,tsx,js,jsx,json}"

echo "=== Formatting complete ==="
