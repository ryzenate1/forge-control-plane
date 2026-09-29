#!/bin/bash
# .mig-audit/s5_apply.sh — apply canonical migrations to a scratch DB, mimicking
# MigrationRunner.Run for the Postgres path: files in sort order, each file in a
# single transaction, abort-on-error per file, continue to next file on failure.
# usage: s5_apply.sh <dbname> <pass-label> [firstN]
set -u
export PGHOST=127.0.0.1 PGUSER=gamepanel
DB="$1"; LABEL="${2:-pass}"
DIR=/Users/riyaz/forge-plane/forge-test/forge/api/migrations
fail=0; total=0
out=/tmp/s5_${LABEL}.log
: > "$out"
for f in $(cd "$DIR" && ls *.sql | LC_ALL=C sort); do
  total=$((total+1))
  if ! psql -q -v ON_ERROR_STOP=1 -1 -d "$DB" -f "$DIR/$f" >>"$out" 2>&1; then
    echo "FAIL $f" | tee -a "$out"
    fail=$((fail+1))
  fi
done
echo "PASS[$LABEL] files=$total failures=$fail"
