#!/bin/bash
set -e

echo "=== Formatting Go code ==="
cd forge/api && gofmt -w . && goimports -w . && cd ../..
cd beacon && gofmt -w . && goimports -w . && cd ../..

echo "=== Formatting TypeScript/JavaScript ==="
npx prettier --write "forge/web/**/*.{ts,tsx,js,jsx,json,css}" "packages/**/*.{ts,tsx,js,jsx,json}"

echo "=== Formatting complete ==="
