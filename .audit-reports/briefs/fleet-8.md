# Scope 8 — Host terminal/console + SFTP + transfer protocol: BOTH TIERS (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 8; report to `.audit-reports/reports/fleet-8.md`.

The load-bearing rule in your slice: **Panel -> Beacon commands go over HTTP via `daemon/client.go`; WebSockets carry
only console/stats/log/terminal streams, never commands.** Beacon serves SFTP on :2022 and its HTTP API on :9090.

## You own (edit only these)

Beacon:
- `beacon/internal/server/hostterminal_unix.go`, `hostterminal_windows.go`, `console.go`, `ws_lifecycle.go`,
  `transfer_protocol.go`
- the SFTP-credential and route parts of `beacon/internal/server/enrollment.go`, `manager.go`, `server.go` —
  your lines only; the enrollment TOKEN logic in `enrollment.go` belongs to scope 3, do not touch it.
API:
- `forge/api/internal/http/handlers_sftp.go` (83 lines, ~5 routes; `registerSFTPRoutes` at server.go:2685)
- `forge/api/internal/store/store_sftp_config.go`
- `forge/api/internal/daemon/client.go` — only the terminal/WS URL and transfer methods (`HostTerminalWSURL`,
  `WebSocketURL`, `SignedHeaders`, `RegisterTransferCredential`, `PrepareTransferSource`, `PushTransferSource`,
  `FinalizeTransferDestination`, `RestoreTransferDestination`, `CleanupTransferSource`, `CancelTransfer`,
  `SendCommand`, `SendCommandWithOutput`, `sendCommandWithBody` and `runInstaller`). Surgical.
- the console/host-terminal WebSocket proxy — locate it by searching the WS registrations in
  `forge/api/internal/http/` and `beacon/internal/server/edge.go`, and confine edits to that handler.
Web:
- `forge/web/app/admin/terminal/page.tsx` (already modified in the working tree — improve what is there, do not
  revert other people's work)
- `forge/web/components/admin/AdminSftp.tsx`
- `forge/web/lib/api/sftp.ts` and the console/ws helpers under `forge/web/lib/api/ws/`

Read every owned file fully, plus the intent-encoding tests `beacon/internal/server/console_test.go`,
`console_recovery_test.go`, `ws_lifecycle`-related tests, `hostfiles_confinement_test.go` (read-only, jail contract)
and `install_exclusivity_test.go`.

## Hunt for, with extreme depth

**Terminal / console**
1. Authorization checked only at HTTP upgrade and never re-asserted for subsequent frames; a WS ticket or URL that is
   reusable, has no lifetime bound, or is not tied to a specific user+node+session.
2. PTY lifecycle: shell orphaned when the client disconnects (no process-group kill / `SIGHUP`), zombie children,
   fd and goroutine leaks per session, no idle timeout, no max-session cap, no context cancellation on close.
3. Output handling: unbounded buffering of a chatty program (OOM path), no write-side backpressure, a slow client
   blocking the read loop.
4. Command injection: a "terminal" that really execs a fixed command but interpolates user input into a shell string;
   `SendCommand`/`SendCommandWithOutput` building `/bin/sh -c <user input>`; resize/signal frames with unvalidated
   dimensions.
5. Reconnect semantics: a session resuming onto a different node or a dead PTY while the UI still shows connected;
   journal/sequence gaps hidden rather than surfaced; `ws_lifecycle.go` claiming liveness from a stale channel state.
6. `hostterminal_windows.go` parity with the unix path — the Windows branch is usually the one that has drifted.
7. **A dead session must not read as connected.** If the shell exited, the stream must terminate and the UI must show
   the exit reason.

**SFTP**
8. Global vs per-node config precedence: which wins, is it validated, and can a node with no explicit config fall
   back to a permissive default (anonymous auth, wide root, no chroot).
9. Credential handling: private keys or passwords stored plaintext, logged, echoed by a read endpoint, or returned in
   a list response; rotation that leaves existing sessions or the Beacon's stored config valid.
10. Chroot/confinement: whether the SFTP subsystem honours the same jail as the file API, and whether per-node config
    can widen it.
11. Transfer protocol correctness: resumability and idempotency across `Prepare*`/`Push*`/`Finalize*`/`Cancel*`; a
    partial file left visible at the destination instead of staged-then-renamed; no size or checksum verification
    before finalize; cancellation that leaves a transfer credential registered; `CleanupTransferSource` errors
    swallowed.
12. `store_sftp_config.go`: upsert correctness, org scoping, swallowed `sql.ErrNoRows`.

**Web**
13. `requestJSON`/`fetchJSON` bypassed with a bare `fetch`; a second WS client invented instead of the existing
    `lib/api/ws/` helper; reconnect loop without backoff or a give-up condition; missing distinct
    connected/reconnecting/auth-failed/closed states; untrusted terminal output rendered without sanitisation;
    secrets displayed with no explicit reveal action.

## Report requirements

Add a `## Session trust` section: how a terminal or transfer session is authenticated, what binds it to one
user+node, when that binding is re-checked, and how it dies. Add a `## Credential exposure` section for SFTP secret
handling. End with `## Cannot verify`.
