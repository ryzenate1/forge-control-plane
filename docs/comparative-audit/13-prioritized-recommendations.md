# Prioritized Recommendations

## Status: ALL P0 BLOCKERS AND MOST P1/P2 ITEMS RESOLVED

See `docs/fix-tracking.md` for the complete fix log.

## Remaining Recommendations

### P1 — Should be addressed

| # | Improvement | Complexity | Impact |
|---|---|---|---|
| P1-1 | JWT WebSocket auth on beacon (replace bearer token in URL) | Week | Security |

### P2 — Important improvements

| # | Improvement | Complexity | Impact |
|---|---|---|---|
| P2-1 | Backup rename/lock API endpoints | Hours | API parity |
| P2-2 | i18n support (next-intl) | Weeks | Internationalization |
| P2-3 | Light/dark theme toggle | Days | UX |
| P2-4 | Accessibility pass (aria, keyboard, Radix) | Weeks | Compliance |
| P2-5 | Rate limiter in-memory fallback when Redis is down | Day | Reliability |
| P2-6 | openat2 support in beacon (kernel-level path safety) | Week | Security |
| P2-7 | Memory overhead compensation in beacon (JVM OOM prevention) | Day | Reliability |
| P2-8 | Per-server WebSocket connection limit in beacon | Hour | Security |
| P2-9 | WebSocket message rate limiting in beacon | Hour | Security |
| P2-10 | Notifications table + endpoints | Week | User experience |
| P2-11 | External ID fields for users/servers | Day | API parity |
| P2-12 | Split store.go into domain-specific files | Days | Maintainability |

### P3 — Optional enhancements

| # | Enhancement | Complexity |
|---|---|---|
| P3-1 | Cursor-based pagination | Day |
| P3-2 | Query caching (Redis) | Week |
| P3-3 | Standard JWT (RFC 9068) migration | Week |
| P3-4 | OpenTelemetry tracing | Week |
| P3-5 | OAuth social login (Discord, GitHub, Google) | Week |
| P3-6 | Fix duplicate migration prefixes | Hour |
| P3-7 | Fix 35 gofmt issues | Minute |
| P3-8 | Fix 43 ESLint warnings | Day |
| P3-9 | Consolidate duplicate console components | Hour |
| P3-10 | Down-migration support | Week |
| P3-11 | Deployment documentation | Day |
