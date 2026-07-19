#!/usr/bin/env bash
# Production readiness guard — exits with an error if any check fails.
# Source this file or run as a step before the application starts.

set -euo pipefail

fail() {
  echo "PRODUCTION GUARD FAILED: $*" >&2
  exit 1
}

if [ "${FORGE_ALLOW_EPHEMERAL_MASTER_KEY:-}" = "true" ]; then
  fail "FORGE_ALLOW_EPHEMERAL_MASTER_KEY must not be true in production"
fi

forge_master_key="${FORGE_MASTER_KEY:-}"
if [ "${#forge_master_key}" -lt 32 ]; then
  fail "FORGE_MASTER_KEY must contain at least 32 characters"
fi

case "${DATABASE_URL:-}" in
  *sslmode=require*|*sslmode=verify-ca*|*sslmode=verify-full*) ;;
  *@postgres:5432/*sslmode=disable*)
    # The bundled PostgreSQL service is isolated on Compose's private backend
    # network and is not published by the production override.
    ;;
  *) fail "DATABASE_URL must require TLS unless it targets the private bundled postgres service" ;;
esac

api_auth_secret="${API_AUTH_SECRET:-}"
app_key="${APP_KEY:-}"

if [ "${#api_auth_secret}" -lt 32 ] || [ "$api_auth_secret" = "dev-api-secret" ]; then
  fail "API_AUTH_SECRET must be a non-default secret of at least 32 characters"
fi

if [ "${#app_key}" -lt 32 ]; then
  fail "APP_KEY must be at least 32 characters"
fi

if [ "${APP_ENV:-}" != "production" ]; then
  fail "APP_ENV must be 'production'"
fi

echo "All production guard checks passed."
