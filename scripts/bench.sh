#!/bin/bash
set -e

echo "=== Running Go Benchmarks ==="
cd forge/api

echo "--- i18n benchmarks ---"
go test -bench=. -benchmem -count=5 ./internal/services/i18n/...

echo "--- Activity benchmarks ---"
go test -bench=. -benchmem -count=5 ./internal/services/activity/...

echo "--- Health benchmarks ---"
go test -bench=. -benchmem -count=5 ./internal/services/health/...

echo "--- Policy benchmarks ---"
go test -bench=. -benchmem -count=5 ./internal/policies/...

cd ../..
echo "=== Benchmarks complete ==="
