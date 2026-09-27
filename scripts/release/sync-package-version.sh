#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
VERSION="$(tr -d ' \n\r' < VERSION)"
if ! echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9._-]+)?$'; then
  echo "VERSION $VERSION not semver" >&2; exit 1
fi
sync_one() {
  local file="$1"
  if [ ! -f "$file" ]; then echo "skip $file (missing)"; return; fi
  # Use node to rewrite only version field, preserving formatting via python or jq?
  # Simple: use npm-style python JSON edit (preserves order, adds newline)
  node -e "
    const fs=require('fs');
    const p='$file';
    const j=JSON.parse(fs.readFileSync(p,'utf8'));
    j.version='$VERSION';
    fs.writeFileSync(p, JSON.stringify(j,null,2)+'\n');
  "
  echo "synced $file -> $VERSION"
}
sync_one package.json
sync_one forge/web/package.json
sync_one packages/sdk/package.json 2>/dev/null || true
# Also update beacon/api docker labels are dynamic, no need
echo "Done. Verify with: cat VERSION && node -p \"require('./package.json').version\""
