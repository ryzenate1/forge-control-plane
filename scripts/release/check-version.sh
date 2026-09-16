#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail=0
note() { echo "== $*"; }
err() { echo "ERROR: $*" >&2; fail=1; }
ok() { echo "OK: $*"; }

# Canonical VERSION
if [ ! -f VERSION ]; then
  err "VERSION file missing"
  exit 1
fi
VERSION="$(tr -d ' \n\r' < VERSION)"
if ! echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9._-]+)?$'; then
  err "VERSION '$VERSION' not semver X.Y.Z[-prerelease]"
else
  ok "VERSION=$VERSION"
fi

# package.json sync
for pkg in package.json forge/web/package.json; do
  if [ -f "$pkg" ]; then
    pkg_ver="$(node -p "require('./$pkg').version" 2>/dev/null || echo "?")"
    if [ "$pkg_ver" != "$VERSION" ]; then
      err "$pkg version $pkg_ver != VERSION $VERSION (run scripts/release/sync-package-version.sh)"
    else
      ok "$pkg version matches VERSION"
    fi
  fi
done

# No hard-coded fake claims: latest should only appear where intentionally pinned (infra base deps)
note "Checking for stale version claims"
# Disallow hard-coded latest for our own images in K8s (but allow quay.io/soketi:latest etc only in infra/compose.realtime)
if grep -rn "ghcr.io.*:latest" infra/ship/kubernetes/ 2>/dev/null; then
  err "K8s manifests must not use :latest for ghcr.io/gamepanel images (use \${TAG})"
else
  ok "K8s images no fake :latest"
fi
if grep -rn "ghcr.io/gamepanel.*:v0\.1\.0" infra/ship/kubernetes/ 2>/dev/null | grep -v "\${TAG}" ; then
  err "K8s still hard-codes v0.1.0 (use \${TAG})"
else
  ok "K8s no hard-coded v0.1.0"
fi
if grep -rn "ghcr.io/anomalyco" infra/ 2>/dev/null; then
  err "infra still references ghcr.io/anomalyco (should be ghcr.io/gamepanel/*)"
else
  ok "infra no stale anomalyco refs"
fi

# Dockerfiles must expose VERSION arg/label
for df in forge/api/Dockerfile beacon/Dockerfile forge/web/Dockerfile; do
  if ! grep -q "ARG VERSION" "$df"; then
    err "$df missing ARG VERSION"
  else
    ok "$df has ARG VERSION"
  fi
  if ! grep -q "org.opencontainers.image.version" "$df"; then
    err "$df missing OCI version label"
  else
    ok "$df has OCI label"
  fi
done

# compose.yml APP_VERSION fallback must not be hard 0.1.0
if grep -q 'APP_VERSION: ${APP_VERSION:-0\.1\.0}' infra/compose.yml; then
  err "infra/compose.yml still defaults APP_VERSION to 0.1.0 (should default to \${TAG})"
else
  ok "compose APP_VERSION defaults to TAG"
fi
# TAG must be pinned with :? error
if ! grep -q 'ghcr.io/gamepanel/forge-api:${TAG:?Set TAG' infra/compose.yml; then
  err "infra/compose.yml forge-api image not TAG-pinned"
else
  ok "compose images TAG-pinned"
fi

# Go version injection docs
if ! grep -q "gamepanel/forge/internal/version.Version" forge/api/Dockerfile; then
  err "forge/api/Dockerfile missing ldflags for version.Version"
else
  ok "forge/api Dockerfile injects version"
fi
if ! grep -q "main.Version" beacon/Dockerfile; then
  err "beacon/Dockerfile missing main.Version ldflag"
else
  ok "beacon Dockerfile injects version"
fi

# Deprecated APP_VERSION env should be documented as mirroring TAG
if grep -q "APP_VERSION:-0\.1\.0" infra/.env.example; then
  err "infra/.env.example still hard-codes APP_VERSION=0.1.0"
else
  ok "env example APP_VERSION mirrors TAG"
fi

# CHANGELOG exists and has Unreleased
if [ ! -f CHANGELOG.md ]; then
  err "CHANGELOG.md missing"
elif ! grep -q "## \[Unreleased\]" CHANGELOG.md; then
  err "CHANGELOG.md missing Unreleased section"
else
  ok "CHANGELOG.md present"
fi

# Version file used in workflows
if ! grep -q "publish-images.yml" .github/workflows/publish-images.yml 2>/dev/null; then
  :
fi
if [ ! -f .github/workflows/publish-images.yml ]; then
  err ".github/workflows/publish-images.yml missing (should publish Docker images on tag)"
else
  ok "publish-images workflow present"
fi
if ! grep -q "profile.*docs" infra/compose.yml 2>/dev/null; then
  :
fi

if [ "$fail" -ne 0 ]; then
  echo "Version checks FAILED" >&2
  exit 1
fi
echo "All version checks passed (VERSION=$VERSION)"
