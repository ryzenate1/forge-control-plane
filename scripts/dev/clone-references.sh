#!/bin/bash
# Clone read-only study checkouts of third-party projects into .reference-repos/.
#
# These are never built, imported or shipped with Forge. They exist so that
# design and architecture decisions can be checked against prior art instead of
# guessed at. Several are AGPL-licensed; study the concepts, never copy source.
#
# Clones are shallow and blobless to keep the checkout small.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DEST="$ROOT/.reference-repos"
mkdir -p "$DEST"

REPOS=(
  # Durable job/workflow engine (Go + PostgreSQL) — step and retry semantics.
  "https://github.com/riverqueue/river"
  # Scheduling and placement — blocked evaluations, placement explainability.
  "https://github.com/hashicorp/nomad"
  # PaaS deployment lifecycle states.
  "https://github.com/coollabsio/coolify"
  "https://github.com/Dokploy/dokploy"
  # Game server panels — server allocation and resource modelling.
  "https://github.com/pterodactyl/panel"
  "https://github.com/pelican-dev/panel"
  # Dense data UI, freshness indicators, empty/loading/error states.
  "https://github.com/supabase/supabase"
  "https://github.com/grafana/grafana"
)

for url in "${REPOS[@]}"; do
  # Include the org so that e.g. pterodactyl/panel and pelican-dev/panel do
  # not collide on a shared basename.
  name="$(echo "$url" | awk -F/ '{print $(NF-1)"-"$NF}')"
  target="$DEST/$name"
  if [ -d "$target/.git" ]; then
    printf '  [skip] %s already cloned\n' "$name"
    continue
  fi
  printf '  [clone] %s\n' "$name"
  rm -rf "$target"
  if git clone --quiet --depth 1 --filter=blob:none --single-branch "$url" "$target"; then
    printf '  [ok]   %s\n' "$name"
  else
    printf '  [fail] %s — resolve the current canonical URL manually\n' "$name"
  fi
done

printf '\nReference checkouts in %s\n' "$DEST"
