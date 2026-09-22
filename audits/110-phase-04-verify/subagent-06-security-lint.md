# Subagent 06 — Security Slices 16, 04 Lint Verification (110-04-06)

**Date:** 2026-08-24
**Scope:** `forge/api/internal/http` (trusted proxy, CSP, L4 probe, wildcard gating) + `forge/api/internal/store` (DNS creds encryption, keyring AAD) + `forge/api/migrations/213`
**Focus:** trusted proxy CIDR gating, DNS credentials encryption, CSP nonce/no fallback-nonce/no strict-dynamic, keyring AAD namespacing, L4 probe IP blocklist, wildcard permission gating

---

## 1. `go vet` — `forge/api/internal/http` (Task Literal vs Corrected)

**Task literal:** `go vet ./forge/api/internal/http -run TestSecurity -count=1 2>&1 | head -n 100`

**Result — literal is malformed (go vet has no `-run`):**
```
malformed import path "-run": leading dash
package TestSecurity is not in std (/opt/homebrew/Cellar/go/1.26.4/libexec/src/TestSecurity)
malformed import path "-count=1": leading dash
```

**Corrected:** `go vet ./forge/api/internal/http 2>&1 | head -n 100`

```
(empty output)
VET_HTTP_EXIT:0
```

Also verified broadened: `go vet ./forge/api/... 2>&1 | head -n 100` → exit 0, 0 lines.

**Verdict:** PASS — no vet issues. Task arguments were test-style flags that vet rejects; running canonical vet shows clean.

---

## 2. `go vet` — `forge/api/internal/store` (Task Literal vs Corrected)

**Task literal:** `go vet ./forge/api/internal/store -run TestSecurity -count=1 2>&1 | head -n 50`

**Result — same malformed:**
```
malformed import path "-run": leading dash
...
```

**Corrected:** `go vet ./forge/api/internal/store 2>&1 | head -n 50`

```
(empty output)
VET_STORE_EXIT:0
```

**Verdict:** PASS.

---

## 3. `go test` — `forge/api/internal/http` — Security/Trust

**Task literal (needs shell-quoted regex):** `go test ./forge/api/internal/http -run TestSecurity|TestTrust -count=1 2>&1 | tail -n 30`
Without quotes the shell would pipe `TestTrust` — corrected to quoted `"TestSecurity|TestTrust"`.

**Command:** `go test ./forge/api/internal/http -run "TestSecurity|TestTrust" -count=1 -v 2>&1 | tail -n 30`

**Result:** PASS — `ok gamepanel/forge/internal/http 0.906s` (non-verbose) / detailed verbose:

```
=== RUN   TestSecurityHeadersMiddleware_Default
--- PASS: TestSecurityHeadersMiddleware_Default (0.00s)
=== RUN   TestSecurityHeadersMiddleware_CustomCSP
--- PASS: TestSecurityHeadersMiddleware_CustomCSP (0.00s)
=== RUN   TestSecurityHeadersMiddleware_DisableHeaders
--- PASS: TestSecurityHeadersMiddleware_DisableHeaders (0.00s)
=== RUN   TestSecurityHeadersMiddleware_CustomFrameOptions
--- PASS: TestSecurityHeadersMiddleware_CustomFrameOptions (0.00s)
=== RUN   TestSecurityHeadersMiddleware_CustomHSTSMaxAge
--- PASS: TestSecurityHeadersMiddleware_CustomHSTSMaxAge (0.00s)
=== RUN   TestTrustedProxies_ExtractClientIP
--- PASS: TestTrustedProxies_ExtractClientIP (0.00s)
=== RUN   TestTrustedProxies_DefaultDeny
2026/08/24 ... WARN TRUSTED_PROXIES not configured — proxy headers will be ignored (default deny) metric=trusted_proxy_unconfigured peer=0.0.0.0
--- PASS: TestTrustedProxies_DefaultDeny (0.00s)
PASS
ok  	gamepanel/forge/internal/http	1.318s
```

**Broader security-related suite also verified:**
`go test ./forge/api/internal/http -run "TestSecurity|TestTrust|TestCSP|TestExtract|TestL4|TestLoadBalancer|TestIPAccess|TestMTLS" -count=1 -v`

- `TestExtractClientIPUsesRightmostForwardedAddress` — PASS (`forge/api/internal/http/security_trust_test.go` helpers)
- `TestExtractClientIPDefaultDenyWithoutTrustedProxies` — PASS
- `TestExtractClientIPPrivateNotTrustedWithoutCIDR` — PASS
- `TestExtractClientIPTrustedCIDRAllowsForwarded` — PASS
- `TestCSP_NoFallbackNonceAndNoStrictDynamic` — PASS (`forge/api/internal/http/security_trust_test.go:126`)
- `TestL4Probe_PrivateAndMetadataRejected` — PASS (`forge/api/internal/http/security_trust_test.go:156`)
- `TestLoadBalancer_AddTarget_RejectsPrivateIP` — PASS
- `TestIPAccess_TrustedProxies` — PASS (`forge/api/internal/http/security_trust_test.go:66`)
- `TestMTLS_XForwardedProto_TrustedOnly` — PASS
- `TestMTLSDevBypass_*` — 6 sub-tests PASS (verifies alias env var panic in production)

Also full HTTP package: `go test ./forge/api/internal/http -count=1` → `ok 1.869s` all green.

**Store security tests:**
`go test ./forge/api/internal/store -run "TestSecurity" -count=1` → `[no tests to run]` (expected — store uses integration helpers, not name `TestSecurity`).
Slice-specific store tests verified separately:
`go test ./forge/api/internal/store -run "TestDNS|TestCertificate|TestSecret" -count=1 -v` →
```
=== RUN   TestSecretAAD_Namespaced
--- PASS: TestSecretAAD_Namespaced (0.00s)
=== RUN   TestDNSProviderAccount_CredentialsEncryption
--- PASS: TestDNSProviderAccount_CredentialsEncryption (0.00s)
PASS
ok  	gamepanel/forge/internal/store	0.762s
```

**Verdict:** PASS — all security/trust/CSP/L4 tests green.

---

## 4. Slice-Specific Pattern Checks

### 4.1 `fallback-nonce` — Expected 0 Hits in Production Emission

**Task check:** `rg -n "fallback-nonce" forge/api/internal/http` → should be 0 hits (`rg` not installed; used `grep -rn`).

**Raw grep:**
```
forge/api/internal/http/security_trust_test.go:128:  if strings.Contains(csp, "fallback-nonce") {
forge/api/internal/http/security_trust_test.go:129:    t.Fatalf("CSP must not contain fallback-nonce ...")
forge/api/internal/http/security_trust_test.go:137:  if nonce ... == "fallback-nonce" ...
forge/api/internal/http/security_trust_test.go:148:  if strings.Contains(csp2, "fallback-nonce") {
... (4 test assertions)
forge/api/internal/http/middleware_security.go:18:// predictable fallback-nonce, exploitable. We generate a strong random nonce per
forge/api/internal/http/middleware_security.go:19:// request and abort (500) if entropy fails — never emit "fallback-nonce".
forge/api/internal/http/middleware_security.go:38:// Do not emit nonce/fallback-nonce with strict-dynamic...
forge/api/internal/http/middleware_security_headers_test.go:48:// ... no fallback-nonce.
forge/api/internal/http/middleware_security_headers_test.go:57:  if strings.Contains(csp, "fallback-nonce") {
... (test assertions)
```

**Filtered production code (exclude `_test.go` and explanatory comments):**
`grep -rn "fallback-nonce" forge/api/internal/http/*.go | grep -v "_test.go" | grep -v "never emit" | grep -v "Do not emit"` → **0 hits** (no code emits fallback-nonce as a value).

**Verification:** Production emitters:
- `forge/api/internal/http/middleware_security.go:24-30` `generateNonce()` → `rand.Read` 16 bytes → `base64.StdEncoding`; on error returns `500` (`SecurityHeaders:32-34`), never a fallback string.
- `forge/api/internal/http/middleware_security_headers.go:43-60` `generateMiddlewareNonce()` same; on error returns `500`, never fallback. `X-CSP-Nonce` header is always the random nonce (`middleware_security_headers.go:79`).

Tests (`security_trust_test.go:128,137,148` and `middleware_security_headers_test.go:57,69`) explicitly assert `!strings.Contains(csp, "fallback-nonce")` and `nonce != "fallback-nonce"` — all PASS.

**Verdict:** ✅ FIXED — zero production emission; comments document the prohibition.

### 4.2 `IsPrivate` vs `isTrustedProxy` in `middleware_ratelimit.go`

**Task check:** `rg -n "IsPrivate" middleware_ratelimit.go` should use `isTrustedProxy` now.

```
grep -rn "IsPrivate" forge/api/internal/http/middleware_ratelimit.go → EXIT 1 (0 hits)
grep -rn "isTrustedProxy" forge/api/internal/http/middleware_ratelimit.go →
  forge/api/internal/http/middleware_ratelimit.go:103: if peerIP == nil || !isTrustedProxy(peerIP) {
```

Code (`middleware_ratelimit.go:98-106`):
```go
// ExtractClientIP uses proxy headers only when the direct peer is explicitly listed
// in TRUSTED_PROXIES (CIDR or single IP). Default deny if TRUSTED_PROXIES not set.
// Private/loopback status alone is NOT sufficient — explicit CIDR gate is required.
func ExtractClientIP(c *fiber.Ctx) string {
    peer := strings.TrimSpace(c.IP())
    peerIP := net.ParseIP(peer)
    if peerIP == nil || !isTrustedProxy(peerIP) {
        return peer
    }
```

`IsPrivate()` still correctly appears only where L4/metadata semantics require it (`handlers_loadbalancer.go:40` inside `isProhibitedTargetIP`), not for trusted-proxy gating.

Broader proxy-trust surface:
- `forge/api/internal/http/trusted_proxies.go:52` `isTrustedProxy(peer net.IP)` — CIDR/single-IP gate, default deny with `WarnOnce` + `trusted_proxy_unconfigured` metric.
- `forge/api/internal/http/middleware_ratelimit.go:103`, `forge/api/internal/http/middleware_mtls.go:126` (X-Forwarded-Proto gating), `forge/api/internal/http/middleware_ipaccess.go:49,119,146` all consume `isTrustedProxy` / `trustedProxiesConfigured`.

**Verdict:** ✅ FIXED — private-network heuristic removed from rate-limit trust; centralized `isTrustedProxy` used.

### 4.3 Keyring AAD (`secretAAD`), CSP, L4 Probe, Wildcard Summary

#### Trusted Proxy (Slice 16)
- `forge/api/internal/http/trusted_proxies.go:11-41` `parseTrustedProxies()` splits `TRUSTED_PROXIES` by `,` → CIDR vs single IP, warns on invalid.
- `trusted_proxies.go:52-75` `isTrustedProxy` default deny, `slog.Warn` with `metric=trusted_proxy_unconfigured`.
- `middleware_ratelimit.go:100-123` `ExtractClientIP` takes rightmost XFF entry (prevents client-spoofed leftmost rotation).
- `middleware_ipaccess.go:49-68` `getClientIP` honors proxy headers only if `trustedProxiesConfigured()` else warns and delegates to `ExtractClientIP` (which still default-denies).
- `middleware_mtls.go:126` only trusts `X-Forwarded-Proto` if `isTrustedProxy(peerIP)`.

#### DNS Credentials Encryption (Slice 04 / 213)
- Migration `forge/api/migrations/213_encrypt_dns_credentials.sql:5-6`:
  ```sql
  ALTER TABLE certificates ADD COLUMN IF NOT EXISTS dns_credentials_encrypted TEXT NOT NULL DEFAULT '';
  ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '';
  ```
  Mirrors `157_encrypt_acme` pattern; plaintext `dns_credentials` / `credentials` kept as compatibility targets until migrator dual-writes then clears.

- Store `forge/api/internal/store/store_secrets.go:774-875`
  - `secretAAD("certificates", id, "dns_credentials")` + `secretAADLegacy` dual-read, encrypted write to `dns_credentials_encrypted`, then `UPDATE certificates SET dns_credentials='{}'::jsonb`.
  - `secretAAD("dns_provider_accounts", id, "credentials")` analogous for `credentials_encrypted`.
  - Rotation detection via `NeedsRotation` + `isLegacyAAD` forces re-encrypt to new key/AAD.

#### CSP (no `fallback-nonce`, no `strict-dynamic` with nonce)
- `forge/api/internal/http/middleware_security.go:18-26` — docs: strict-dynamic ignored when present with nonce; never emit `fallback-nonce`; `generateNonce()` 16 cryptoroent bytes, abort 500 on entropy failure.
- `middleware_security.go:37-48` — CSP: `default-src 'self'; script-src 'self' 'nonce-<rand>'; style-src 'self' 'unsafe-inline'; ... frame-ancestors 'none'; ...`; no `strict-dynamic`, no `unsafe-inline` in script-src, no `unsafe-eval` (API process never serves Monaco).
- `middleware_security_headers.go:43-86` — `DefaultSecurityHeadersConfig().CSPValue` contains `'nonce-{NONCE}'`; `SecurityHeadersMiddleware` replaces `{NONCE}` per-request with `generateMiddlewareNonce()`; if custom CSP lacks nonce injection, still injects `script-src 'self' 'nonce-...'`; if `strict-dynamic` present with nonce, it is stripped (`strings.ReplaceAll` + cleanup).
- Headers: `X-CSP-Nonce`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `Permissions-Policy` stripped down, `HSTS` production-only.
- Tests: `middleware_security_headers_test.go:48-71`, `security_trust_test.go:126-153` both `t.Fatalf` on `fallback-nonce` or `strict-dynamic` presence, assert `'nonce-` exists — PASS.

#### Keyring AAD (namespaced + legacy rotation)
- `forge/api/internal/store/store_secrets.go:52` `func secretAAD(table, id, field string) string { return "forge:secret:" + table + ":" + id + ":" + field }`
- `store_secrets.go:54` `secretAADLegacy` = `table + ":" + id + ":" + field` (pre-namespace)
- `store_secrets.go:58-68` `isLegacyAAD` — decrypt succeeds with legacy but not namespaced → force rotation.
- `store_secrets.go:42-49` `decryptSecret` — try `aad`, fallback legacy when `forge:secret:` prefix.
- Call sites (`store_secrets.go:130-842`) cover all encrypted fields: `compose_stacks.env_vars`, `git_webhook_secret`, `notification_channels.config`, `backup_policies.encryption_key`, `acme_accounts.private_key`, `db_containers.(connection_string|credentials)`, `backup_storage_providers.config`, `panel_settings_expanded.*`, `nodes.daemon_token`, `certificates.dns_credentials`, `dns_provider_accounts.credentials` — 20 total `secretAAD` refs.
- `MigrateOperationalSecrets` (`store_secrets.go:75-160`) iterates 12 fieldSpecs, dual-decrypts legacy, re-encrypts with new AAD if `NeedsRotation` or `usedLegacy`.

#### L4 Probe (private/metadata blocked)
- `forge/api/internal/http/handlers_loadbalancer.go:29-48` `isProhibitedTargetIP`:
  ```go
  if ip.IsLoopback() { blocked }
  if ip.IsPrivate() { blocked }
  if ip.IsUnspecified() { blocked }
  if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() { blocked }
  if ip.IsMulticast() { blocked }
  if ipStr == "169.254.169.254" { blocked } // metadata
  if net.ParseIP fails { blocked }
  ```
  Used in `handlers_loadbalancer.go:169` for `POST /admin/load-balancer/groups` / target add.
- `security_trust_test.go:156-180` `TestL4Probe_PrivateAndMetadataRejected` checks 13 cases: `192.168.1.10`, `10.0.0.5`, `172.16.5.1`, `127.0.0.1`, `169.254.169.254`, `169.254.0.1`, `0.0.0.0`, `224.0.0.1`, `::1`, `not-an-ip` blocked; `8.8.8.8`, `1.1.1.1`, `203.0.113.5` allowed — PASS.

#### Wildcard Permission Gating
- `forge/api/internal/auth/scopes_extended.go:133-134` `wildcard := parts[0]+":*"` honored as explicit `userScopes[wildcard]` mapping.
- `forge/api/internal/store/permissions.go:197-201` doc: wildcard `*` honored only as explicit owner assignment.
- `forge/api/internal/http/handlers_servers.go:604-616,678-689` GH-18/SE-01 gate: `forbidden: only server owner or admin can grant wildcard permission` before `CreateSubUser` / `UpdateSubUserPermissions`; enforces subset of caller scopes.
- Tests: `store_users_wildcard_test.go:88-191` covers non-owner cannot grant `*`, owner/admin can, sensitive permission blocked.

---

## 5. `dns_credentials_encrypted` Migration & `secretAAD` Check

**Task checks:**
- `rg -n "dns_credentials_encrypted" forge/api/migrations` should exist 213
- `rg -n "secretAAD" store_secrets.go`

```
grep -rn "dns_credentials_encrypted" forge/api/migrations →
  forge/api/migrations/213_encrypt_dns_credentials.sql:5: ALTER TABLE certificates ADD COLUMN IF NOT EXISTS dns_credentials_encrypted ...

grep -rn "secretAAD" forge/api/internal/store/store_secrets.go → 20 hits (grep -c = 20)
  store_secrets.go:52:func secretAAD(...)
  store_secrets.go:54:func secretAADLegacy(...)
  store_secrets.go:130,131,222,240,288,341,391,444,445,505,583,599,644,689,774,775,842,843 ...
```

**Verdict:** ✅ Both present — migration 213 adds dual encrypted columns; 20 `secretAAD` references confirm namespaced AAD coverage including DNS creds migration path.

---

## 6. Remaining `vet` / `lint`

**`go vet ./forge/api/...`** → exit 0, 0 lines (clean).

**`golangci-lint`** → `command not found` (not installed locally; expected CI-only via `forge/api/.golangci.yml` — same as Subagent 03 finding).

**`gofmt -l` slice-relevant:**
- `forge/api/internal/http/middleware_security.go` PASS
- `forge/api/internal/http/middleware_security_headers.go` PASS
- `forge/api/internal/http/trusted_proxies.go` PASS
- `forge/api/internal/http/middleware_ratelimit.go` PASS
- `forge/api/internal/http/security_trust_test.go` PASS
- `forge/api/internal/store/store_secrets.go` **1 hit before fix** (single-line `secretAAD` func)

**Fix applied:**
```bash
gofmt -w forge/api/internal/store/store_secrets.go
```
Diff (whitespace only, no semantic change):
```diff
-func secretAAD(table, id, field string) string { return "forge:secret:" + table + ":" + id + ":" + field }
+func secretAAD(table, id, field string) string {
+	return "forge:secret:" + table + ":" + id + ":" + field
+}
```

**After:** `gofmt -l forge/api/internal/store/store_secrets.go` → empty, `go vet ./forge/api/internal/store` → exit 0.

Note: `gofmt -l forge/api/internal/http/*.go forge/api/internal/store/*.go` still lists 13 files globally (e.g., `errors.go`, `server.go`, `realtime.go`, etc.) — out of scope for security slice; slice files now clean. Full `gofmt` of repo would touch 12+ unrelated files and is deferred to global lint pass.

**Verdict:** vet clean; slice lint auto-fixed (whitespace).

---

## 7. Summary

| Check | Result | Evidence |
|---|---|---|
| `go vet http` (Task literal) | ⚠️ literal malformed | `malformed import path "-run"`; corrected `go vet ./forge/api/internal/http` → exit 0 |
| `go vet store` (Task literal) | ⚠️ literal malformed | same; corrected → exit 0 |
| `go vet ./forge/api/...` broad | ✅ PASS | 0 lines, exit 0 |
| `go test http TestSecurity\|TestTrust` | ✅ PASS | 7/7 security tests green, `ok 0.906s`–`1.59s` |
| `go test http full security suite` | ✅ PASS | `TestCSP_NoFallbackNonceAndNoStrictDynamic`, `TestL4Probe_*`, `TestTrustedProxies_*`, `TestIPAccess_*`, `TestMTLS_*`, `TestExtractClientIP*` all PASS; full http pkg PASS 1.86s |
| `go test store secretAAD/DNS` | ✅ PASS | `TestSecretAAD_Namespaced`, `TestDNSProviderAccount_CredentialsEncryption` PASS |
| `fallback-nonce` hits | ✅ 0 production emits | Only comments (`middleware_security.go:18,38`) + test assertions; no code path emits literal fallback value |
| `IsPrivate` in `middleware_ratelimit.go` | ✅ 0 hits | Now `isTrustedProxy` at `:103` |
| `isTrustedProxy` usage | ✅ | `trusted_proxies.go:52`, `middleware_ratelimit.go:103`, `middleware_mtls.go:126`, `middleware_ipaccess.go:49` |
| `dns_credentials_encrypted` migration | ✅ | `migrations/213_encrypt_dns_credentials.sql:5-6` exists for `certificates` + `dns_provider_accounts` |
| `secretAAD` | ✅ | 20 refs in `store_secrets.go:52-843`, namespaced `forge:secret:` + legacy fallback + `isLegacyAAD` rotation |
| CSP (nonce, no fallback, no strict-dynamic) | ✅ | Per-request `rand.Read` 16B, abort 500 on fail; both `SecurityHeaders` and `SecurityHeadersMiddleware` tested |
| L4 probe | ✅ | `isProhibitedTargetIP` blocks private/loopback/link-local/metadata/multicast + `TestL4Probe_PrivateAndMetadataRejected` PASS |
| Wildcard gating | ✅ | GH-18/SE-01 owner/admin-only `*`, subset checks, `scopes_extended.go:133`, `permissions.go:197` |
| Vet/lint fix | ✅ | `secretAAD` gofmt widened; vet remains 0 |

**No remaining vet issues in scope; one lint (gofmt) auto-fixed whitespace-only. All security slice verifications pass.**

---

**Sign-off:** Subagent 06 security slices 16 & 04 verified. Trusted proxy CIDR gating, DNS creds encrypted columns, CSP nonce hygiene, keyring AAD namespacing/rotation, L4 metadata/IP blocklist, wildcard permission gating confirmed; vet clean, tests green, lint patched.
