#!/bin/bash
set -euo pipefail
# Canonical lint lives in scripts/dev/lint.sh. This shim exists so `make lint`
# and legacy callers converge on one implementation instead of drifting.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$ROOT/scripts/dev/lint.sh" "$@"
