#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "=== Running all Go tests ==="
(cd "$ROOT/forge/api" && go test -v -cover -count=1 ./...)
(cd "$ROOT/beacon" && go test -v -cover -count=1 ./...)

echo "=== All tests passed ==="
