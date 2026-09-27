#!/usr/bin/env bash
# Backwards-compatible wrapper — the canonical dev runner is scripts/dev/start-dev.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Single runner: canonical implementation lives in scripts/dev/.
# This wrapper exists for backwards compatibility only.
exec "$ROOT/scripts/dev/start-dev.sh" "$@"
