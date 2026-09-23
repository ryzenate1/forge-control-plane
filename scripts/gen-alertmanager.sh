#!/usr/bin/env bash
# Render infra/alertmanager/alertmanager.yml from alertmanager.yml.tmpl using
# ALERTMANAGER_* environment variables.
#
# prom/alertmanager has no --config.expand-env support, so receiver endpoints
# must be baked into the config before the container starts. This script is
# that build step; compose.yml volume-mounts the generated file read-only:
#   ./alertmanager/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro
#
# Env inputs (all optional):
#   ALERTMANAGER_SMTP_HOST      SMTP smarthost, e.g. smtp.example.com
#   ALERTMANAGER_SMTP_PORT      default 587
#   ALERTMANAGER_SMTP_USER      SMTP auth username
#   ALERTMANAGER_SMTP_PASS      SMTP auth password
#   ALERTMANAGER_SMTP_FROM      From address (default noreply@gamepanel.local)
#   ALERTMANAGER_ADMIN_EMAIL    To address for delivered alerts
#   ALERTMANAGER_WEBHOOK_URL    POST endpoint for delivered alerts
#
# Email receiver is emitted when ADMIN_EMAIL + SMTP_HOST (+FROM) are set.
# Webhook receiver is emitted when WEBHOOK_URL is set. With nothing set the
# output is a valid blackhole config (alerts acknowledged and dropped).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPL="$ROOT/infra/alertmanager/alertmanager.yml.tmpl"
OUT="${1:-$ROOT/infra/alertmanager/alertmanager.yml}"

[ -f "$TMPL" ] || { echo "template not found: $TMPL" >&2; exit 1; }

SMTP_GLOBAL=""
SMTP_RECEIVER=""
WEBHOOK_RECEIVER=""
ROOT_RECEIVER="default"
CRITICAL_RECEIVER="default"
WARNING_RECEIVER="default"

if [ -n "${ALERTMANAGER_ADMIN_EMAIL:-}" ] && [ -n "${ALERTMANAGER_SMTP_HOST:-}" ]; then
  smtp_from="${ALERTMANAGER_SMTP_FROM:-noreply@gamepanel.local}"
  smtp_port="${ALERTMANAGER_SMTP_PORT:-587}"
  smarthost="${ALERTMANAGER_SMTP_HOST}:${smtp_port}"
  auth_lines=""
  if [ -n "${ALERTMANAGER_SMTP_USER:-}" ]; then
    auth_lines=$(printf '        auth_username: "%s"\n        auth_password: "%s"\n' \
      "${ALERTMANAGER_SMTP_USER}" "${ALERTMANAGER_SMTP_PASS:-}")
  fi
  SMTP_GLOBAL="  smtp_smarthost: \"${smarthost}\""$'\n'"  smtp_from: \"${smtp_from}\""$'\n'"  smtp_require_tls: true"
  SMTP_RECEIVER="$(cat <<EOF

  - name: "admin-email"
    email_configs:
      - to: "${ALERTMANAGER_ADMIN_EMAIL}"
        from: "${smtp_from}"
        smarthost: "${smarthost}"
${auth_lines}        send_resolved: true
EOF
)"
  ROOT_RECEIVER="admin-email"
  CRITICAL_RECEIVER="admin-email"
  WARNING_RECEIVER="admin-email"
elif [ -n "${ALERTMANAGER_ADMIN_EMAIL:-}" ] || [ -n "${ALERTMANAGER_SMTP_HOST:-}" ]; then
  echo "[gen-alertmanager] WARN: email receiver needs BOTH ALERTMANAGER_ADMIN_EMAIL and ALERTMANAGER_SMTP_HOST — skipping email receiver" >&2
fi

if [ -n "${ALERTMANAGER_WEBHOOK_URL:-}" ]; then
  WEBHOOK_RECEIVER="$(cat <<EOF

  - name: "webhook"
    webhook_configs:
      - url: "${ALERTMANAGER_WEBHOOK_URL}"
        send_resolved: true
EOF
)"
  ROOT_RECEIVER="${ALERTMANAGER_WEBHOOK_PRIMARY:-$ROOT_RECEIVER}"
  if [ "${ALERTMANAGER_WEBHOOK_FOR_CRITICAL:-true}" = "true" ]; then
    CRITICAL_RECEIVER="$ROOT_RECEIVER"
  fi
fi

awk \
  -v smtp_global="$SMTP_GLOBAL" \
  -v smtp_receiver="$SMTP_RECEIVER" \
  -v webhook_receiver="$WEBHOOK_RECEIVER" \
  -v root_receiver="$ROOT_RECEIVER" \
  -v critical_receiver="$CRITICAL_RECEIVER" \
  -v warning_receiver="$WARNING_RECEIVER" '
  {
    gsub(/@@SMTP_GLOBAL@@/, smtp_global)
    gsub(/@@SMTP_RECEIVER@@/, smtp_receiver)
    gsub(/@@WEBHOOK_RECEIVER@@/, webhook_receiver)
    gsub(/@@ROOT_RECEIVER@@/, root_receiver)
    gsub(/@@CRITICAL_RECEIVER@@/, critical_receiver)
    gsub(/@@WARNING_RECEIVER@@/, warning_receiver)
    print
  }' "$TMPL" > "$OUT"

# 0600: the rendered file can contain SMTP credentials (auth_password) and a
# webhook URL that may embed a bearer token, so it must never be world- or
# group-readable. If the alertmanager container runs as a non-root user that
# cannot read the operator's file, bind-mount it with an explicit `user:` (or
# `chown <container-uid>:<container-gid>`) instead of widening the mode back to
# 0644.
chmod 0600 "$OUT"

echo "[gen-alertmanager] wrote $OUT"
echo "[gen-alertmanager] root receiver: $ROOT_RECEIVER; critical: $CRITICAL_RECEIVER; warning: $WARNING_RECEIVER"
