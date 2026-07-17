# 08 — Security Assessment

## Critical Vulnerabilities

| # | Issue | Location | Risk Level | Detail |
|---|---|---|---|---|
| 1 | **Plaintext HTTP between panel and beacon** | `beacon/cmd/daemon/main.go` | 🔴 Critical | All traffic between forge and beacon — including JWTs, file contents, install scripts, and environment variables — is transmitted over unencrypted HTTP. An on-path attacker on the node network can read or modify any of it. Wings and all reference implementations require TLS. |
| 2 | **No per-server WebSocket authorization** | `beacon/internal/server/server.go` | 🔴 Critical | The WebSocket endpoint authenticates that the connecting user is a valid panel user, but does not verify that the user has permission to access the specific server being connected to. Any authenticated user can attach to any server's console by knowing (or guessing) a server UUID. |
| 3 | **No SSRF protection on file pull** | `beacon/internal/server/server.go` | 🔴 Critical | `pullRemoteFile` calls `http.Get` with the user-supplied URL and no validation. An attacker can direct beacon to fetch from RFC1918 addresses (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), loopback (`127.0.0.1`), link-local (`169.254.0.0/16`), or the instance metadata endpoint (`169.254.169.254`) to exfiltrate cloud credentials or probe internal services. |
| 4 | **`wsTicketStore` map has no mutex** | `forge/api/internal/http/handlers_ws_ticket.go` | 🟡 Medium | The in-memory WebSocket ticket store is a plain `map[string]wsTicket` with no `sync.RWMutex`. Concurrent ticket issuance and validation from different goroutines will cause a data race. Under load this can corrupt map state or allow ticket reuse across users. |
| 5 | **`AtomicBool.SwapIf` is not a true CAS** | `beacon/internal/system/atomic.go` | 🟡 Medium | `SwapIf` is implemented as `Load` followed by a conditional `Store` without using `sync/atomic.CompareAndSwap`. The window between the two operations is a race: two goroutines can both observe the old value and both proceed through the swap, breaking single-ownership invariants (e.g., ensuring only one goroutine starts a container). |
| 6 | **`Locker.TryAcquire` goroutine leak** | `beacon/internal/system/locker.go` | 🟡 Medium | `TryAcquire` spawns a goroutine that blocks on the mutex channel. If the caller's context is cancelled before the lock is acquired, the goroutine is never cleaned up. Under sustained load with context cancellations (e.g., HTTP handler timeouts) this leaks goroutines indefinitely. |
| 7 | **Transfer authentication uses global node token** | `beacon/internal/transfer/transfer.go` | 🟡 Medium | Incoming transfer requests are authenticated only by the static per-node token, not by a per-transfer secret. Any node that knows another node's token (e.g., through a compromised node) can fabricate a transfer to that node and overwrite arbitrary server data. Wings mitigates this with a per-transfer signed token issued by the panel. |
| 8 | **`isContainerMissing` uses brittle string matching** | `beacon/internal/runtime/docker.go` | 🟡 Medium | The function checks whether a Docker error is a "not found" error by matching the string `"No such container"`. The Docker daemon's error messages vary across versions and may be localized. A mismatch causes the function to return `false`, making beacon treat a missing container as a runtime error and preventing correct state recovery on restart. |
| 9 | **`renderTemplate` uses `strings.ReplaceAll`** | `beacon/internal/server/server.go` | 🟠 Low-Medium | Config file templating substitutes variables via plain string replacement with no escaping. A variable value containing characters special to the target format (e.g., `"` or `\` in JSON, `:` in YAML) will produce syntactically invalid or semantically altered config files. In a JSON config this could silently corrupt the file; in pathological cases an attacker-controlled variable value could inject keys. |
| 10 | **No reCAPTCHA on login endpoint** | `forge/api/internal/http/handlers_auth.go` | 🟠 Low | The login endpoint has no CAPTCHA challenge. Combined with the rate-limiting being Redis-based (which may not be configured in all deployments), this leaves the endpoint open to credential-stuffing attacks. Pterodactyl and Pelican both support reCAPTCHA on login. |
| 11 | **`pullRemoteFile` has no file size limit** | `beacon` | 🟠 Low-Medium | There is no `Content-Length` check or streaming byte limit on the remote file download. An attacker can supply a URL that streams an arbitrarily large response, exhausting disk space on the daemon host. |
| 12 | **`pullRemoteFile` has no request timeout** | `beacon` | 🟠 Low | The `http.Get` call uses the default `http.Client` with no timeout. A server that responds slowly or never closes the connection will cause the goroutine to hang indefinitely, leaking a goroutine and a file descriptor per request. |

---

## Security Strengths

1. **Node→panel HMAC-SHA256 request signing** — every request from beacon to forge is signed with `HMAC-SHA256(method + uri + timestamp + body)`. The panel rejects any request whose signature does not match or whose timestamp is outside the allowed clock skew window. This prevents replay attacks and request forgery from the daemon side.

2. **Container security hardening** — Docker containers are created with `CapDrop: ["ALL"]`, `ReadonlyRootfs: true`, `NoNewPrivileges: true`, and `PidsLimit: 256`. This substantially limits what a malicious or compromised game server process can do on the host.

3. **Custom HMAC-JWT avoids algorithm confusion** — GamePanel's JWT implementation uses a symmetric HMAC keyed to the node secret rather than RS256 or a configurable algorithm field. This is not vulnerable to the `alg: none` or algorithm-confusion attacks that affect generic JWT libraries accepting multiple algorithms.

4. **OAuth2 JWT revocation list** — issued JWTs have a `jti` (JWT ID) claim tracked in the `jwt_revocations` table. Revoking an OAuth2 client or user session invalidates all outstanding tokens without waiting for expiry. This is absent from Pterodactyl and Pelican.

5. **40+ granular subuser permissions** — the permission system enumerates specific capabilities (e.g., `file.read`, `backup.create`, `database.delete`) rather than coarse roles, enabling the principle of least privilege for shared server access.

6. **Login rate limiting via Redis** — the authentication endpoint is rate-limited per email address and per IP address using a Redis-backed token bucket. This is enforced at the application layer independently of any upstream proxy.

7. **Setup wizard blocks re-initialization** — once an admin user exists, the setup/install endpoint returns an error rather than allowing an unauthenticated caller to reset credentials. This prevents post-deployment takeover via the setup route.

8. **2FA backend complete** — TOTP 2FA is fully implemented: secret generation, QR code enrollment, code validation, and single-use recovery tokens. Recovery tokens are hashed at rest.

9. **Single-use WebSocket tickets** — WebSocket connection tickets have a 60-second TTL and are HMAC-signed. A ticket can only be used once; attempting to reuse it is rejected. This prevents ticket harvesting from logs or network captures.

10. **SFTP path traversal prevention** — SFTP file operations are confined to the server's data directory using `filepath.Rel`. Paths that resolve outside the root return an error. This prevents a malicious SFTP client from reading or writing host files outside the server directory.

11. **OAuth2 token binding to `server_id`** — when issuing an OAuth2 token scoped to a specific server, the `server_id` is embedded in the token claims and validated on every request. A token issued for server A cannot be used to operate on server B.

12. **JWT revocation on OAuth2 client deletion** — deleting an OAuth2 client triggers revocation of all tokens issued to that client, not just future token issuance. This ensures that removing an integration immediately terminates its access.

---

## Recommended Security Fixes (Prioritized)

| Priority | Fix | Location | Effort |
|---|---|---|---|
| **P0** | Add TLS support to beacon HTTP listener; require TLS for all panel↔beacon communication | `beacon/cmd/daemon/main.go`, forge node config | Medium — add cert/key config + `http.ListenAndServeTLS`; update forge HTTP client to use TLS |
| **P0** | Add SSRF protection to `pullRemoteFile`; validate URL against RFC1918 / loopback / link-local / metadata IP denylists before making any request | `beacon/internal/server/server.go` | Small — IP parsing + denylist check before `http.Get` |
| **P0** | Add per-server WebSocket authorization; validate that the JWT's `server_id` claim matches the server UUID in the WebSocket URL | `beacon/internal/server/server.go` | Small — one claim check in the WS auth handler |
| **P1** | Add `sync.RWMutex` to `wsTicketStore` | `forge/api/internal/http/handlers_ws_ticket.go` | Trivial |
| **P1** | Replace `AtomicBool.SwapIf` with `sync/atomic.CompareAndSwapInt32` for true CAS semantics | `beacon/internal/system/atomic.go` | Trivial |
| **P1** | Fix `Locker.TryAcquire` goroutine leak; cancel the blocking goroutine when context is done | `beacon/internal/system/locker.go` | Small — add `select` on context done channel |
| **P2** | Issue per-transfer tokens from the panel; validate them on the receiving beacon instead of relying on the static node token | `beacon/internal/transfer/transfer.go`, forge transfer handler | Medium |
| **P2** | Fix `renderTemplate` to escape variable values for the target file format before substitution | `beacon/internal/server/server.go` | Medium — requires per-format escaping logic |
| **P2** | Add file size limit and request timeout to `pullRemoteFile`; use `http.MaxBytesReader` and a client with `Timeout` set | `beacon` | Small |
| **P2** | Add reCAPTCHA support to the login endpoint (configurable, off by default) | `forge/api/internal/http/handlers_auth.go` | Small |
| **P3** | Replace `isContainerMissing` string matching with Docker SDK error type assertion (`errdefs.IsNotFound`) | `beacon/internal/runtime/docker.go` | Trivial |
| **P3** | Add content-type validation and file extension checks on upload endpoints | `beacon` file upload handler | Small |

---

## Comparison: Security Model vs References

| Security Feature | Pterodactyl | Pelican | PufferPanel | GamePanel |
|---|---|---|---|---|
| TLS between panel and daemon | ✅ Required | ✅ Required | ✅ Built-in | ❌ HTTP only |
| HMAC request signing (daemon→panel) | ✅ | ✅ | ❌ | ✅ |
| JWT algorithm | HS256 (configurable) | HS256 | RS256 / HS256 | Custom HMAC (fixed) |
| SSRF protection on file pull | ✅ IP denylist | ✅ IP denylist | N/A | ❌ None |
| Login rate limiting | ⚠️ Optional | ✅ Redis | ⚠️ Optional | ✅ Redis |
| reCAPTCHA on login | ✅ Optional | ✅ Optional | ❌ | ❌ |
| TOTP 2FA | ✅ | ✅ | ✅ | ✅ |
| Session fixation protection | ✅ | ✅ | ⚠️ | ✅ |
| RBAC model | Binary flag | Spatie RBAC | OAuth2 scopes | In progress |
| SFTP path confinement | ✅ openat2 | ✅ openat2 | ✅ | ⚠️ filepath.Rel only |
| JWT revocation | ❌ | ❌ | ❌ | ✅ |
| Per-server WebSocket auth | ✅ | ✅ | ✅ | ❌ |
| Container capability drop | ✅ | ✅ | ✅ (unshare) | ✅ |
| WebAuthn / Passkeys | ❌ | ❌ | ✅ | ❌ |
| Single-use WS tickets | ✅ | ✅ | ❌ | ✅ |
