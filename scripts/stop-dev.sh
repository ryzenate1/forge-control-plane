#!/usr/bin/env bash
# Backwards-compatible wrapper — the canonical dev stopper is scripts/dev/stop-dev.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$ROOT/scripts/dev/stop-dev.sh" "$@"
