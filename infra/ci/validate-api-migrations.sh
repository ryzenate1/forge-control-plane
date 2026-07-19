#!/usr/bin/env bash
# Bootstrap the Forge API against CI PostgreSQL, verify every migration was
# recorded, and stop the API immediately. The temporary listener is loopback-only.

set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL must point to the CI PostgreSQL service}"

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/../.." && pwd)"
api_dir="$repo_root/forge/api"
migrations_dir="${MIGRATIONS_DIR:-migrations}"
tmp_dir="$(mktemp -d)"
api_bin="$tmp_dir/forge-api"
api_log="$tmp_dir/forge-api.log"
api_pid=""

cleanup() {
    if [ -n "$api_pid" ] && kill -0 "$api_pid" 2>/dev/null; then
        kill "$api_pid" 2>/dev/null || true
        wait "$api_pid" 2>/dev/null || true
    fi
    rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

cd "$api_dir"

if [ ! -d "$migrations_dir" ]; then
    echo "Migration directory not found: $api_dir/$migrations_dir" >&2
    exit 1
fi

expected_count="$(find "$migrations_dir" -maxdepth 1 -type f -name '*.sql' | wc -l | tr -d '[:space:]')"
if [ "$expected_count" -eq 0 ]; then
    echo "No SQL migrations found in $api_dir/$migrations_dir" >&2
    exit 1
fi

api_port="$(python3 -c 'import socket; sock = socket.socket(); sock.bind(("127.0.0.1", 0)); print(sock.getsockname()[1]); sock.close()')"
api_addr="127.0.0.1:$api_port"
health_url="http://$api_addr/api/v1/health"

go build -o "$api_bin" ./cmd/api

APP_ENV=development \
API_ADDR="$api_addr" \
API_AUTH_SECRET="${API_AUTH_SECRET:-ci-api-secret-not-for-production}" \
DAEMON_NODE_TOKEN="${DAEMON_NODE_TOKEN:-0123456789abcdef.0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}" \
DATABASE_URL="$DATABASE_URL" \
MIGRATIONS_DIR="$migrations_dir" \
REDIS_ADDR= \
"$api_bin" >"$api_log" 2>&1 &
api_pid=$!

for _ in $(seq 1 60); do
    if ! kill -0 "$api_pid" 2>/dev/null; then
        echo "Forge API exited before migration validation completed:" >&2
        cat "$api_log" >&2
        exit 1
    fi

    if health_response="$(curl --fail --silent --show-error "$health_url" 2>/dev/null)"; then
        applied_count="$(printf '%s' "$health_response" | python3 -c 'import json, sys
payload = json.load(sys.stdin)
for check in payload.get("checks", []):
    if check.get("name") == "database":
        count = check.get("details", {}).get("migrationCount")
        if isinstance(count, int):
            print(count)
            raise SystemExit(0)
raise SystemExit("database migrationCount missing from API health response")')"

        if [ "$applied_count" -ne "$expected_count" ]; then
            echo "Migration bootstrap applied $applied_count of $expected_count SQL files" >&2
            cat "$api_log" >&2
            exit 1
        fi

        echo "Migration bootstrap applied all $expected_count SQL files"
        exit 0
    fi

    sleep 0.5
done

echo "Timed out waiting for Forge API migration bootstrap:" >&2
cat "$api_log" >&2
exit 1
