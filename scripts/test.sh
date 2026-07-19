#!/bin/bash
set -e

echo "=== Running Go API tests ==="
cd forge/api
go test -v -cover -count=1 ./internal/services/i18n/...
go test -v -cover -count=1 ./internal/services/health/...
go test -v -cover -count=1 ./internal/services/activity/...
go test -v -cover -count=1 ./internal/services/plugins/...
go test -v -cover -count=1 ./internal/services/recovery/...
go test -v -cover -count=1 ./internal/policies/...
cd ../..

echo "=== Running Go Beacon tests ==="
cd beacon && go test -v -count=1 ./... && cd ..

echo "=== Running all Go tests ==="
cd forge/api && go test -v -cover -count=1 ./... && cd ../..
cd beacon && go test -v -cover -count=1 ./... && cd ..

echo "=== All tests passed ==="
