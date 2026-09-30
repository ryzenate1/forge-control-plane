#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

usage() {
  cat <<USAGE
Usage: $0 [--list|--delete-local|--delete-remote]
  --list          List local tags matching freebuff-snapshot/* and any other
                  snapshot-like tags not matching ^v[0-9]+\\.[0-9]+\\.[0-9]+
  --delete-local  Delete those local tags (no remote)
  --delete-remote Delete remote tags as well (requires confirmation, no dry-run on remote)
  --help          This help

No git delete happens without an explicit --delete flag. Remote deletion requires typing
"yes" and will run: git push origin --delete <tag>
USAGE
}

mode="list"
case "${1:-}" in
  --list|"") mode="list" ;;
  --delete-local) mode="local" ;;
  --delete-remote) mode="remote" ;;
  --help|-h) usage; exit 0 ;;
  *) echo "unknown arg $1" >&2; usage; exit 2 ;;
esac

# Find snapshot tags: freebuff-snapshot/* or any tag not matching release semver vX.Y.Z
tags="$(git tag --list | grep -E '^freebuff-snapshot/|^snapshot/|^tmp/' || true)"
# also consider any tag that is not vX.Y.Z
other="$(git tag --list | grep -vE '^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9._-]+)?$' || true)"
# Merge unique
all_tags="$(echo -e "${tags}\n${other}" | sed '/^$/d' | sort -u)"
if [ -z "$all_tags" ]; then
  echo "No snapshot/non-release tags found."
  exit 0
fi

echo "Found $(echo "$all_tags" | wc -l | tr -d ' ') snapshot/non-release tag(s):"
echo "$all_tags" | sed 's/^/  /'

if [ "$mode" = "list" ]; then
  echo ""
  echo "Run with --delete-local to remove locally, or --delete-remote for remote (careful)."
  exit 0
fi

if [ "$mode" = "local" ]; then
  echo ""
  echo "Deleting locally..."
  echo "$all_tags" | xargs -r -n1 git tag -d
  echo "Done. Push deletion separately with --delete-remote if needed."
  exit 0
fi

# remote
echo ""
echo "WARNING: This will delete the above tags from origin (remote)!"
printf "Type 'yes' to confirm: "
read -r ans
if [ "$ans" != "yes" ]; then
  echo "Aborted." >&2; exit 1
fi
for t in $all_tags; do
  echo "Deleting remote $t ..."
  git push origin --delete "$t" || echo "  (failed or not on remote) $t"
done
echo "Done."
