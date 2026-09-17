# Scope 10 — Cross-tier contracts: daemon client ↔ Beacon ↔ web

Repo: `/Users/riyaz/forge-plane/forge-test`. Audit + fix pass. No builds/tests run
(per brief); verification is by reading imports, signatures, struct tags and routes.

## Half-applied repairs in daemon/client.go

`forge/api/internal/daemon/client.go` was being written while this pass ran
(mtime moved 18:45 → 18:50:37 → 18:52:00 → 19:02, size 73,955 → 76,662 bytes), so
the audit was done against the live state and re-checked after each change.

Deltas found on disk came from two overlapping edits: an earlier one that widened
timeouts and keyed `CreateServer`/transfer-push on idempotency, and a later one
that narrowed the retry policy to the routes Beacon actually dedupes.

Checked and consistent (each item verified by reading, not by compiling):

- **Imports**: all 17 imports are referenced — `math/big` (`jitter`),
  `path/filepath` (`PullRemoteFile`), `net` (`isLoopback`, `PullRemoteFile`),
  `crypto/tls` (`daemonTransport`), `encoding/hex` (`newSignatureParts`). No
  missing import for the new code paths.
- **Helpers called but not defined**: none. `commandIDFromContext` (:49),
  `beaconDedupedCommandRoute` (:175), `readErrorDetails` (:199),
  `transferJSONWithTimeout` (:646), `WithTimeout` (:96), `validateNodeURL`
  (:1680) all exist. Every `c.<method>(` call in package `daemon` resolves to a
  `func (c *Client)` definition (set-difference over the whole package is empty).
- **Signatures**: `transferJSON` kept its old signature and delegates to the new
  `transferJSONWithTimeout(..., 0, "")`, so all six existing transfer call sites
  stay valid; the only caller passing the new args is `PushTransferSource`.
- **Locals**: `RoundTrip`'s new `lastStatus`/`lastDetails`/`sawStatus` are all
  read on every exit path; the old "read status after closing the body" ordering
  bug is gone.
- **Guards**: the three inserted `validateNodeURL` calls (`transferJSONWithTimeout`,
  `HostFilesUpload`, `AdminContainerFilesUpload`) sit after request construction
  and before `Do`, so nothing downstream became unreachable, and each returns the
  error instead of proceeding.
- **Comments vs behaviour**: the earlier edit claimed the idempotency header made
  a create/push safe to replay; the later edit corrected `CreateServer` and
  `PushTransferSource` to say the opposite. I confirmed the corrected claim
  against Beacon: `beacon/internal/server/server.go:1512-1516` (`power`) is the
  only handler that reads `X-Forge-Command-ID` (falling back to `Idempotency-Key`)
  and dedupes through `operations.EnqueueCommand`. `pushTransferSource` does *not*
  dedupe — it copies `idempotencyKey` from the JSON body onto the outbound
  `Idempotency-Key` header it sends to the destination
  (`beacon/internal/server/transfer_protocol.go:143`), so the panel's header on the
  push request is bookkeeping, not a replay licence. `forge/api/internal/daemon/
  client_retry_test.go` was updated in step with the narrowed policy (it now
  asserts the power route replays and `/servers` + `X-Idempotency-Key` do not), so
  no test drift remains from this change.

Repaired in this pass:

1. **Dead declaration removed** — `retryableStatuses = "429,502,503,504"` in the
   const block was referenced nowhere in the module (`isRetryableStatus` hardcodes
   the four codes). Left over from the retry rework; an unreferenced const is a lie
   about a shared source of truth, so it is gone.

## Contract mismatches fixed

_TBD — panel↔beacon pairing in progress._

## Honesty fixes

_TBD._

## Unpaired or dead calls

_TBD._

## Needs elsewhere

_TBD._

## Checked clean

_TBD._
