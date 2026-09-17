#!/bin/bash
# Double-apply harness for scope 3 (migrations 098-114) on scratch DB mig_s3.
# Mirrors the runner's real apply order: numeric prefix, then letter suffix
# (bare sorts first). Applies every canonical migration <= 114 so real
# dependencies exist, then re-applies the whole prefix to expose
# non-idempotent canonicals.
#
# The real runner (Store.runMigrations, store.go:1508-1524) executes every
# split statement of a file inside ONE tx and commits with the
# schema_migrations row; a failing file rolls back whole. --single-transaction
# reproduces that; plain per-statement psql would leave half-applied files.
#
# usage: apply-scope3.sh <db> <pass-label> <up-to-number>
set -uo pipefail
DB="${1:-mig_s3}"
LABEL="${2:-pass1}"
UPTO="${3:-114}"
DIR=/Users/riyaz/forge-plane/forge-test/forge/api/migrations
OUT=/Users/riyaz/forge-plane/forge-test/.mig-audit/out
mkdir -p "$OUT"

ls "$DIR"/*.sql | sed "s|$DIR/||" \
  | grep -E '^[0-9]{3}(_[a-z0-9_]+)?\.sql$' \
  | awk -v max="$UPTO" '{ n = substr($0,1,3) + 0; if (n <= max) { sfx = ($0 ~ /^[0-9]{3}_[a-z]/) ? substr($0,5,1) : "0"; printf "%03d %s %s\n", n, sfx, $0 } }' \
  | sort -k1,1n -k2,2 -k3,3 | awk '{print $3}' > "$OUT/${LABEL}.order"

total=$(wc -l < "$OUT/${LABEL}.order" | tr -d ' ')
echo "pass=$LABEL files=$total db=$DB"
: > "$OUT/${LABEL}.tsv"
while IFS= read -r f; do
  err=$(psql -X -q -1 -v ON_ERROR_STOP=1 -h 127.0.0.1 -U gamepanel -d "$DB" \
        -f "$DIR/$f" 2>&1 | grep -E 'ERROR:|FATAL:' | head -4)
  if [ -n "$err" ]; then
    printf '%s\tFAIL\t%s\n' "$f" "$(printf '%s ' $err | tr '\n' ' ' | cut -c1-300)" >> "$OUT/${LABEL}.tsv"
  else
    printf '%s\tOK\t\n' "$f" >> "$OUT/${LABEL}.tsv"
  fi
done < "$OUT/${LABEL}.order"
echo "pass=$LABEL done: $(grep -c '	FAIL	' "$OUT/${LABEL}.tsv") failures -> $OUT/${LABEL}.tsv"
