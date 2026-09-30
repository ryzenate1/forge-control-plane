# Scope 5 — Networking II: certificates, ACME, firewall, gateway delivery, Netbird

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 5.

## Owned files (`forge/api/`)

`internal/http/handlers_certificates.go`, `handlers_certificates_ext.go`, `handlers_acme_accounts.go`,
`handlers_firewall.go`, `handlers_gateways*.go` if present else the gateway handlers under
`handlers_admin_extras.go` (coordinate via NEEDS-ORCHESTRATOR, that file is scope 2's),
`internal/services/acme/`, `internal/services/cert_service.go`, `internal/services/firewall/` (if present),
`internal/services/cleanup/` for cert-cleanup paths only, `internal/gateway/**` if present,
`internal/services/netbird/`, `internal/http/handlers_netbird.go`, plus matching `_test.go`.
Infra: `infra/**` Caddy/Nginx/Alertmanager config for the TLS path — read freely, but any edit to shared
infra files goes to NEEDS-ORCHESTRATOR.

Not yours: domains/routes/LB/DNS/mTLS (scope 4), mounts and backups (scope 6).

## Smoke checklist

ACME accounts: create, list, patch, delete; prove the private key is stored encrypted and is never
returned in a response body; prove deleting an account in use is refused or cascades as documented — pick
whichever the code intends and make the two agree.

Certificate lifecycle: request issuance for `smoke5-<n>.example.test`, track order/challenge status,
renew, revoke, list, get detail. Real public ACME issuance against `*.example.test` will not work; that is
fine and expected — but the failure must be **visible and correctly classified** (challenge failed vs
DNS could not resolve vs provider unreachable), never a 200 with an empty cert, and never a stuck
"pending" that no endpoint can explain.

**Delivery to the gateway is the part most likely to be broken.** Known context: ACME certificates are
supposed to reach the gateway via `SetCertificate` after issuance. Verify the whole chain: issuance →
store → `SetCertificate` call → gateway actually serving the cert. If issuance succeeds and delivery is
absent, stubbed, or error-swallowed, that is S1. Check the delivery error is propagated to the API
response and to the certificate's status rather than being logged and dropped.

Renewal: prove a cert near expiry is renewed automatically (find the renewal job/timer), that renewal
failure marks the cert unhealthy, and that expiry dates shown in the API are the real ones from the cert,
not a computed guess.

Firewall: rules list/add/delete/toggle; port allow/deny; per-node application. Prove a rule reaches the
host firewall (`pfctl`/`iptables` equivalent is not available on macOS — check the Beacon side and the
recorded state, and mark host enforcement NOT-VERIFIABLE with the reason if the platform cannot do it).
Verify the API does not claim "applied" when the node is offline.

Gateway TLS settings: HTTPS enforcement, redirect settings, min TLS version, ALPNs — send, read back, and
confirm the gateway config file/actual admin endpoint reflects the change.

Netbird: peers list/sync (`/admin/netbird/peers`). Without a Netbird server the honest result is an error
or a clearly-skipped state; verify it is not reported as an empty-but-healthy peer list.

Alerting hooks: cert-about-to-expire should emit a notification event. Check the event is produced by the
renewal path (it may belong to scope 8's engine — if so, verify the *emit* side and note the dependency).

## Report

`.smoke/reports/scope-5.md`. Be explicit about which parts you could not do because macOS/Colima cannot
act as a real firewall host or reach a live ACME CA, and what you proved instead about the failure paths.
