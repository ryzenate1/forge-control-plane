#!/bin/bash
# Shared Forge API helper for the smoke campaign. Source it:  . .smoke/api.sh
# All requests go to 127.0.0.1 (NOT localhost) — the cookie jar is domain-scoped to
# 127.0.0.1 and curl silently drops the session for any other host, giving 401s.
set -u
FORGE_ROOT=/Users/riyaz/forge-plane/forge-test
JAR="$FORGE_ROOT/.dev-data/smoke-jar.txt"
API=http://127.0.0.1:8080/api/v1
WEB=http://localhost:3000
NODE_A=22222222-2222-2222-2222-222222222222   # Ubuntu Demo Node, online, port 9090
NODE_B=e43bb9aa-24c8-4900-a469-4656d45b55ad   # smoke-node-b, offline, http://192.168.31.189:9091
LOCATION=cccccccc-cccc-cccc-cccc-cccccccccccc

api_login() {
  redis-cli -a gamepanel DEL api:ratelimit:127.0.0.1 >/dev/null 2>&1
  curl -s -o /dev/null -c "$JAR" -X POST "$API/auth/login" \
    -H 'Content-Type: application/json' -H 'Origin: http://localhost:3000' \
    -d '{"email":"admin@example.com","password":"admin123"}' >/dev/null
}

api_csrf() { grep -oE 'forge_csrf[[:space:]]+[^[:space:]]+' "$JAR" | awk '{print $2}'; }

# Re-login if the session died (expiry is ~24h, but revocation/rotation happens too).
api_ensure() {
  local code
  code=$(curl -s -m 10 -b "$JAR" -H 'Origin: http://localhost:3000' \
    -o /dev/null -w '%{http_code}' "$API/auth/me")
  [ "$code" = "200" ] || api_login
}

# GET <path>  -> body on stdout, trailing line is the HTTP status
api_get() {
  api_ensure
  curl -s -m 40 -b "$JAR" -H 'Origin: http://localhost:3000' \
    -H "X-CSRF-Token: $(api_csrf)" -w $'\n%{http_code}' "$API$1"
}

# POST <path> <json>  (also PUT/PATCH/DELETE via api_method)
api_post() {
  api_ensure
  curl -s -m 60 -b "$JAR" -H 'Origin: http://localhost:3000' \
    -H "X-CSRF-Token: $(api_csrf)" -H 'Content-Type: application/json' \
    -X POST -d "$2" -w $'\n%{http_code}' "$API$1"
}

api_method() { # api_method PUT /path <json>
  api_ensure
  curl -s -m 60 -b "$JAR" -H 'Origin: http://localhost:3000' \
    -H "X-CSRF-Token: $(api_csrf)" -H 'Content-Type: application/json' \
    -X "$1" -d "$3" -w $'\n%{http_code}' "$API$2"
}

api_delete() {
  api_ensure
  curl -s -m 40 -b "$JAR" -H 'Origin: http://localhost:3000' \
    -H "X-CSRF-Token: $(api_csrf)" -X DELETE -w $'\n%{http_code}' "$API$1"
}

# One-line "CODE | body" for a quick probe
api_probe() { # api_probe /path
  local out
  out=$(api_get "$1")
  printf '%-46s %s | %s\n' "$1" "$(printf '%s' "$out" | tail -1)" \
    "$(printf '%s' "$out" | sed '$d' | tr '\n' ' ' | cut -c1-110)"
}

# Status code only
api_code() { api_get "$1" | tail -1; }

# Pretty-ish JSON body without the status line
api_body() { api_get "$1" | sed '$d'; }

# Beacon (node A) direct endpoint; no auth needed for /health
beacon_get() { curl -s -m 20 -w $'\n%{http_code}' "http://127.0.0.1:9090$1"; }

# Postgres access (used to check whether an API answer matches the row)
dbq() { # dbq "select ..."
  PGPASSWORD=gamepanel psql -h 127.0.0.1 -U gamepanel -d gamepanel -tAc "$1"
}
