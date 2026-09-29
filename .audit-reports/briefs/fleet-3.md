# Scope 3 — Capabilities, Onboarding tokens, Node probe/enrollment: BACKEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 3; report to `.audit-reports/reports/fleet-3.md`.

Capabilities and onboarding are two ends of one flow: Beacon produces host facts, the API stores and serves them,
and an onboarding token bootstraps a Beacon's node credentials. You own both sides of that flow.

## You own (edit only these)

API:
- `forge/api/internal/http/handlers_capabilities.go` (547 lines, ~13 routes; registered at `server.go:2625` as
  `registerCapabilityRoutes(protected, cfg, cfg.NodeProbe)`)
- `forge/api/internal/http/phase8_onboarding.go` (158 lines, ~6 routes; registrar `("phase8-onboarding", 170, …)`)
- the inline `POST /onboarding/exchange` handler at `forge/api/internal/http/server.go:1097-1115` — surgical edit
  only, `server.go` is a shared hotspot
- `forge/api/internal/services/onboarding/`, `internal/services/nodeprobe/`, `internal/services/registrations/`,
  `internal/services/installer/`
- `forge/api/internal/store/store_capabilities.go`
Beacon:
- `beacon/internal/server/capabilities.go`, `enrollment.go`, `install_stream.go`, `diagnostics.go`
- `beacon/internal/runtime/providers.go`, `factory.go` — ONLY for what gets reported as a capability (e.g. a
  build-tag-gated or hypervisor-requiring provider claiming support). Docker is the only verified production path.
  Anything else in those files belongs to scope 9/10 — report instead of editing.

Before editing, READ the existing tests that encode intent: `beacon/internal/server/audit_fixes_test.go`,
`phantom_test.go`, `state_test.go`, `runtime_handlers_test.go`, and `forge/api/internal/http/handlers_capabilities.go`'s
own tests if present. Also read (without editing) scope 1's `/nodes/:id/capability*` routes so the two halves agree.

## Hunt for, with extreme depth

1. **Optimistic capability reporting**: a probe that fails, times out, hits an unreachable node, or runs on a host
   that cannot support a runtime, yet reports an empty-but-successful capability set. "No capabilities" and
   "could not determine" must be different states, and the empty case must carry a reason + timestamp.
2. **Drift detection that compares a cached inventory against itself**, or that concludes "no drift" when one side
   was never fetched.
3. **Capability row writes that clobber** the whole record instead of upserting per key, so a partial probe silently
   deletes previously known facts.
4. **Stale-read-as-healthy**: `last_reported` ignored, or a capability from days ago served as current.
5. **Authz on capability endpoints**: cross-org node capability leakage; a `:id` param that is parsed but never used
   to scope the query; missing `requireAdminScope` on the remediation/refresh routes (a route that triggers work on
   a node is a mutation, not a read).
6. **Onboarding token crypto/lifecycle**: token material compared with `==` instead of a constant-time digest
   comparison; tokens not single-use; no expiry enforced; a token re-readable after redemption; secret material
   written to logs or audit records; a token created without a node/region/location/organization binding that any
   host can then redeem — verify the redeeming host's org matches the token's org, and refuse otherwise.
7. **`/onboarding/exchange` hardening**: rate limiting present and correct; already-redeemed and expired tokens
   rejected with distinct reasons; concurrent redemption of one token (double-spend) refused, which needs an atomic
   claim in the store, not a read-then-write.
8. **Revocation**: revoking a token must not leave an already-issued node credential usable; revoke must target by
   id, never by list index; verify the Beacon's stored credential actually stops working.
9. **Beacon enrollment atomicity**: node credentials persisted atomically; on a failed or ambiguous API response the
   Beacon must NOT silently keep using a stale token and report enrolled. `install_stream.go`: an installer that
   exits non-zero must not mark the operation completed.
10. **Unknown-vs-zero in Beacon diagnostics**: any metric a platform cannot provide (this dev box is macOS, so darwin
    gaps are real) must be reported unsupported, not 0.

## Report requirements

Add a `## Token lifecycle` section: create -> redeem -> rotate -> revoke, with the exact guard and store operation at
each step and whether it is atomic. End with `## Cannot verify`.
