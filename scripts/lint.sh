#!/bin/bash
set -e

echo "=== Running Go linters ==="
cd forge/api && golangci-lint run ./... && cd ../..
cd beacon && golangci-lint run ./... && cd ..

echo "=== Running TypeScript checks ==="
cd forge/web && npm run lint && cd ../..
cd packages/sdk && npm run lint && cd ../..
cd packages/shared-types && npm run lint && cd ../..

echo "=== Running Prettier check ==="
npx prettier --check "forge/web/**/*.{ts,tsx,js,jsx,json,css}" "packages/**/*.{ts,tsx,js,jsx,json}"

echo "=== All checks passed ==="
