# Scope 1 — Auth, sessions, users, RBAC, tenancy

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 1.

Auth is the load-bearing wall of this campaign: every other agent logs in the way you validate. If you
break session handling, ten agents lose their ability to test. Backward compatibility is not optional —
`__Host-forge_session`, the `forge_csrf` cookie + `X-CSRF-Token` pair, and Bearer/API-key paths must all
keep working, because the orchestrator's helper (`.smoke/api.sh`, forbidden for you to edit) depends on them.

## Owned files (`forge/api/` unless noted)

`internal/http/auth.go`, `internal/http/handlers_auth.go`, `handlers_account_recovery.go`,
`handlers_password_reset.go`, `handlers_roles.go`, `handlers_users.go`, `handlers_tenancy.go`,
`handlers_oauth2.go`, `handlers_social_auth.go`, `handlers_webauthn.go`, `handlers_setup.go`,
`handlers_ws_ticket.go`, `handlers_permissions*.go` if present, `internal/services/tenancy/`,
`internal/services/webauthn/`, `internal/services/onboarding/`, `internal/services/config/`, and the
matching `_test.go` files. Frontend: `forge/web/app/(auth)/**`, `forge/web/app/setup/**`,
`forge/web/lib/api/auth*.ts`, `forge/web/lib/api/users*.ts`, `forge/web/lib/api/roles*.ts`,
`forge/web/stores/*auth*` / session stores.

## Smoke checklist

Login / logout / `/auth/me` / session refresh / session revocation (does killing a session actually stop
the old cookie from working?), password change, password reset flow, account recovery, TOTP /
two-factor gating (`requireTwoFactorAuthentication` — verify a user with TFA on cannot skip it and one
without it is not blocked), CSRF enforcement (a mutating request with cookie but no header must be
rejected; with both must pass; wrong header must be rejected), rate limiting (login limiter — note the
Redis key shape `api:ratelimit:127.0.0.1`), OAuth2 provider flows, social auth, WebAuthn registration +
assertion, first-run `/setup`.

Authorization layers — this is the heart of your scope, verify each independently and prove they are
really three layers and not one that shadows the others:

1. admin scopes (`requireAdminScope`) — an admin session must get `["*"]`; a non-admin must get `[]` and be
   refused. Test that a non-admin session cannot reach any `/admin/*` route.
2. per-server RBAC (`requireServerPermission`) — create/obtain two users, one with `server.view` only,
   one full. Prove the limited user can read and cannot mutate, and cannot reach the *other* user's server.
3. org tenancy (`handlers_tenancy.go`, `services/tenancy`) — cross-org read/write attempts must be refused
   with 403/404, not 200 with filtered data, and not 500.

Also verify: `handlers_ws_ticket.go` issues a ticket that the WS endpoints accept, and that a ticket is
single-use / scoped (a ticket for server X must not authorize the console stream of server Y).
Verify API keys: create one with least privilege and prove the scope list is actually enforced.

Watch specifically for the known failure mode in this repo: a handler that reads a request field the
client never sends (or the reverse), so the operation "succeeds" and does nothing. Compare the JSON tags
in the Go request structs against what `forge/web/lib/api/*` actually sends, field by field, for login,
user create/update, role assignment and tenancy mutations.

## Report

`.smoke/reports/scope-1.md`. In `Notes for the final re-verify pass`, give the orchestrator the exact
commands to re-run after the API restart — especially any auth change, since it can invalidate the
shared session for every other agent. If a fix of yours requires re-login, say so loudly.
